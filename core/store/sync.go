package store

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/vanadiry/serein/core/events"
	"github.com/vanadiry/serein/core/httpx"
	"github.com/vanadiry/serein/core/log"
	"github.com/vanadiry/serein/core/progress"
)

const maxFetchBytes = 8 << 20 // 8MB

// stagingDirName 规则提交的暂存根目录名（home 下的隐藏目录，rules/ 的同级）
const stagingDirName = ".sync-staging"

// 提交阶段等待跨进程锁的参数：重试间隔与最长等待。
// 取得锁后临界区只有毫秒级的 rename，超时给得宽是为了容忍另一个实例正在跑完整同步。
const (
	commitLockRetry   = 100 * time.Millisecond
	commitLockTimeout = 30 * time.Second
)

// getHTTP GET 一个 URL，校验状态码并限长读取
func getHTTP(ctx context.Context, url string, limit int64) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	resp, err := httpx.DefaultClient().Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if err := httpx.CheckStatus(resp); err != nil {
		return nil, err
	}
	return io.ReadAll(io.LimitReader(resp.Body, limit))
}

// readURLOrFile 读取内容：http(s) 走网络（带 ctx / 状态码校验 / 大小上限），否则读本地文件
func readURLOrFile(ctx context.Context, rawURL string, limit int64) ([]byte, error) {
	if !strings.HasPrefix(rawURL, "http://") && !strings.HasPrefix(rawURL, "https://") {
		return os.ReadFile(rawURL)
	}
	return getHTTP(ctx, rawURL, limit)
}

// fetchSourceInfo 抓取并解析一个 _source.json，顺带剔除结构不合法的 files 条目。
// 结构问题以 warn 事件上报而非报错：上游一个笔误不该挡住整个源。
func fetchSourceInfo(ctx context.Context, url string) (*SourceInfo, []byte, error) {
	if err := ctx.Err(); err != nil {
		return nil, nil, err
	}
	body, err := readURLOrFile(ctx, url, maxFetchBytes)
	if err != nil {
		return nil, nil, err
	}

	var s SourceInfo
	if err := json.Unmarshal(body, &s); err != nil {
		return nil, nil, fmt.Errorf("解析 %s: %w", sourceFileName, err)
	}
	if s.ID == "" {
		return nil, nil, fmt.Errorf("%s 缺少 source_id", sourceFileName)
	}
	if !regexp.MustCompile(`^[a-zA-Z0-9_-]+$`).MatchString(s.ID) {
		return nil, nil, fmt.Errorf("source_id %q 包含非法字符，仅允许大小写字母、数字、下划线和连字符", s.ID)
	}
	for _, is := range validateSourceFiles(&s) {
		events.Emit("warn", "[sync]", is.Message)
		log.LogfWarn("[sync] %s", is.Message)
	}
	return &s, body, nil
}

// loadLocalFileTokens 读取本地已落盘的 marker，返回该源上次接受的 token 表。
// 这就是比对基线——不需要额外的索引文件：本地 marker 天然记录了上次接受什么。
// 读取失败（不存在 / 解析失败 / 格式不符）返回 nil 表示「无可信基线」，
// 后果仅是本轮多下一些文件，不会损坏数据。
func loadLocalFileTokens(dir string) map[string]string {
	data, err := os.ReadFile(filepath.Join(dir, sourceFileName))
	if err != nil {
		return nil
	}
	var s SourceInfo
	if json.Unmarshal(data, &s) != nil {
		return nil
	}
	return s.Files
}

// safeRelPath 校验源内相对路径
func safeRelPath(base, rel string) (string, bool) {
	if rel == "" || filepath.IsAbs(rel) {
		return "", false
	}
	cleaned := filepath.Clean(rel)
	if cleaned == "." {
		return "", false
	}
	target := filepath.Join(base, cleaned)
	r, err := filepath.Rel(base, target)
	if err != nil || r == ".." || strings.HasPrefix(r, ".."+string(filepath.Separator)) {
		return "", false
	}
	return cleaned, true
}

// readLeafFile 读取子规则源的一个文件内容：web 源走网络，本地源读文件
func readLeafFile(ctx context.Context, l leafSrc, rel string) ([]byte, error) {
	if !l.isWeb {
		return os.ReadFile(filepath.Join(l.baseURL, rel))
	}
	url := strings.TrimSuffix(l.baseURL, "/") + "/" + filepath.ToSlash(rel)
	body, err := getHTTP(ctx, url, maxFetchBytes)
	if err != nil {
		return nil, fmt.Errorf("下载 %s: %w", url, err)
	}
	return body, nil
}

// 异步同步（带进度）

type leafSrc struct {
	id      string
	rawBody []byte
	destDir string
	baseURL string
	files   map[string]string // 远端 manifest 全量条目：文件名 → token
	need    map[string]string // 本次需要下载的条目（files 的子集）
	carry   map[string][]byte // token 未变、沿用本地内容的条目（并入提交以免整树替换时被清掉）
	isWeb   bool
}

// readUnchangedFiles 读入 token 未变的本地文件，并入提交内容。
// 两个边界都必须处理：
//   - 只遍历远端 manifest。遍历本地表会把上游已删除的文件复活——它们正等着被整树换入清掉。
//   - token 未变但文件已不在盘上（上次同步被中断、手工删除）时退回待下载集合，
//     否则整树换入会连它一起清掉，造成规则永久缺失。
func readUnchangedFiles(rulesDir string, l *leafSrc, local map[string]string) {
	dir := filepath.Join(rulesDir, l.destDir)
	for name, token := range l.files {
		if _, downloading := l.need[name]; downloading {
			continue
		}
		body, err := os.ReadFile(filepath.Join(dir, name))
		if err != nil {
			l.need[name] = token
			continue
		}
		if l.carry == nil {
			l.carry = make(map[string][]byte, len(l.files))
		}
		l.carry[name] = body
	}
}

// syncFailure 一次同步中某个规则文件失败的信息
type syncFailure struct {
	Source string `json:"source"`
	File   string `json:"file"`
	Error  string `json:"error"`
}

// SyncAllSourcesAsync 两阶段同步：
// 1. 遍历源树，收集所有子规则源 → 发 list 事件
// 2. 并发下载所有规则文件 → 发 file 事件（done/total）
// onDone 在同步完成后（进度关闭后）调用，可用于重载规则缓存
func SyncAllSourcesAsync(home string, sources []RuleSource, concurrency int, p *progress.Progress, onDone func()) {
	reload := false
	defer func() {
		p.Close()
		if reload && onDone != nil {
			onDone()
		}
	}()

	ctx := p.Context()
	rulesDir := filepath.Join(home, "rules")
	stagingRoot, err := prepareStagingRoot(home)
	if err != nil {
		log.LogfWarn("[sync] 准备暂存目录失败，本次同步不提交: %v", err)
		p.SendMap(map[string]any{
			"step":            "done",
			"sources_total":   len(sources),
			"sources_skipped": 0,
			"sources_updated": 0,
			"sources_failed":  len(sources),
			"files":           0,
			"file_errors":     0,
			"failures":        []syncFailure{{Source: "暂存目录", Error: err.Error()}},
			"cancelled":       ctx.Err() != nil,
		})
		return
	}

	// Phase 1: 遍历
	leaves, gatherFailures := gatherLeaves(sources, concurrency, p)
	sourcesTotal := len(leaves) + len(gatherFailures)
	if len(leaves) == 0 {
		// gather 阶段全部失败：明确汇总，避免静默“同步完成”
		p.SendMap(map[string]any{
			"step":            "done",
			"sources_total":   sourcesTotal,
			"sources_skipped": 0,
			"sources_updated": 0,
			"sources_failed":  len(gatherFailures),
			"files":           0,
			"file_errors":     0,
			"failures":        gatherFailures,
			"cancelled":       ctx.Err() != nil,
		})
		return
	}

	// 逐文件比对 token：本地 marker 即上次接受的基线。
	// 整源无差异则完全跳过；有差异时把 token 未变的条目读进 carry 一并提交——
	// 提交是整树换入，只带变化的文件会把未变的旧文件一起清掉。
	var fresh []leafSrc
	var skipped []leafSrc
	for _, l := range leaves {
		local := loadLocalFileTokens(filepath.Join(rulesDir, l.destDir))
		l.need = make(map[string]string, len(l.files))
		for name, token := range l.files {
			if local[name] != token {
				l.need[name] = token
			}
		}
		if len(l.need) == 0 {
			p.SendMap(map[string]any{
				"step":  "skip",
				"name":  l.id,
				"files": len(l.files),
			})
			skipped = append(skipped, l)
			continue
		}
		readUnchangedFiles(rulesDir, &l, local)
		fresh = append(fresh, l)
	}
	leaves = fresh
	sourcesSkipped := len(skipped)
	sourcesUpdated := 0

	totalFiles := 0
	for _, l := range leaves {
		totalFiles += len(l.need)
	}

	if totalFiles > 0 {
		for _, l := range leaves {
			p.SendMap(map[string]any{
				"step":  "source",
				"name":  l.id,
				"files": len(l.need),
			})
		}
		p.Send("start", "", 0, totalFiles)
	}

	fileErrors := 0
	sourcesFailed := len(gatherFailures)
	failures := append([]syncFailure(nil), gatherFailures...)

	if totalFiles > 0 {
		contents, leafFailed, dlFailures, dlErrors := downloadLeaves(ctx, rulesDir, leaves, concurrency, p, totalFiles)
		failures = append(failures, dlFailures...)
		fileErrors += dlErrors
		for i := range leaves {
			if leafFailed[i] {
				sourcesFailed++
			}
		}
		// 取消时不提交
		if ctx.Err() == nil {
			// 临界区：整个 rules/ 树的写入与清理。跨进程互斥——两个实例并发写同一棵
			// 树会交错出「文件已删而 marker 声称最新」的永久缺失（见 lock.go）。
			// 拿不到锁就整体放弃本次提交，不做任何修改。
			lock, err := AcquireFileLock(ctx, home, commitLockTimeout, commitLockRetry)
			if err != nil {
				sourcesFailed += len(leaves)
				failures = append(failures, syncFailure{Source: "文件锁", Error: err.Error()})
			} else {
				updated, commitFailures := commitLeaves(rulesDir, stagingRoot, leaves, contents, leafFailed)
				sourcesUpdated = updated
				failures = append(failures, commitFailures...)
				sourcesFailed += len(commitFailures)
				_ = lock.Release()
			}
		}
	}

	if sourcesUpdated > 0 {
		reload = true
	}
	p.SendMap(map[string]any{
		"step":            "done",
		"sources_total":   sourcesTotal,
		"sources_skipped": sourcesSkipped,
		"sources_updated": sourcesUpdated,
		"sources_failed":  sourcesFailed,
		"files":           totalFiles,
		"file_errors":     fileErrors,
		"failures":        failures,
		"cancelled":       ctx.Err() != nil,
	})
}

// downloadLeaves 并发下载各子规则源的文件（内存暂存）；返回失败标记、失败明细与文件错误数
func downloadLeaves(ctx context.Context, rulesDir string, leaves []leafSrc, concurrency int, p *progress.Progress, totalFiles int) ([]map[string][]byte, []bool, []syncFailure, int) {
	sem := make(chan struct{}, concurrency)
	var mu sync.Mutex
	var done int
	var wg sync.WaitGroup

	contents := make([]map[string][]byte, len(leaves))
	leafFailed := make([]bool, len(leaves))
	for i := range leaves {
		// 先并入 token 未变的本地内容：提交是整树换入，缺了它们旧文件会被清掉
		contents[i] = make(map[string][]byte, len(leaves[i].need)+len(leaves[i].carry))
		for rel, body := range leaves[i].carry {
			contents[i][rel] = body
		}
	}

	fileErrors := 0
	var failures []syncFailure

	for i := range leaves {
		l := leaves[i]
		base := filepath.Join(rulesDir, l.destDir)
		for f := range l.need {
			rel, ok := safeRelPath(base, f)
			if !ok {
				events.Emit("warn", "[sync]", fmt.Sprintf("跳过非法文件路径 %q（源 %s）", f, l.id))
				mu.Lock()
				leafFailed[i] = true
				fileErrors++
				failures = append(failures, syncFailure{Source: l.id, File: f, Error: "非法文件路径"})
				mu.Unlock()
				continue
			}
			wg.Add(1)
			go func(i int, l leafSrc, rel string) {
				defer wg.Done()
				sem <- struct{}{}
				defer func() { <-sem }()

				body, err := readLeafFile(ctx, l, rel)
				mu.Lock()
				done++
				name := rel
				if err != nil {
					leafFailed[i] = true
					fileErrors++
					failures = append(failures, syncFailure{Source: l.id, File: rel, Error: err.Error()})
					name += " (失败)"
				} else {
					contents[i][rel] = body
				}
				p.Send("file", name, done, totalFiles)
				mu.Unlock()
			}(i, l, rel)
		}
	}
	wg.Wait()
	return contents, leafFailed, failures, fileErrors
}

// commitLeaves 原子提交：逐个子规则源先在暂存目录里写完整棵树，全部文件写成功才换入
// 目标目录。任一文件写失败即放弃本次提交——目标目录保持原样，版本标记也不推进，
// 因此不会出现「_source.json 已记录新版本、规则文件却缺失」的永久损坏状态。
// 返回成功换入的源数与失败明细。
func commitLeaves(rulesDir, stagingRoot string, leaves []leafSrc, contents []map[string][]byte, leafFailed []bool) (int, []syncFailure) {
	updated := 0
	var failures []syncFailure
	for i := range leaves {
		if leafFailed[i] {
			continue
		}
		l := leaves[i]
		dest := filepath.Join(rulesDir, l.destDir)
		if err := commitLeafDir(stagingRoot, dest, contents[i], l.rawBody); err != nil {
			events.Emit("error", "[sync]", fmt.Sprintf("提交 %s 失败: %v", l.id, err))
			log.LogfWarn("[sync] 提交 %s 失败: %v", l.id, err)
			failures = append(failures, syncFailure{Source: l.id, Error: err.Error()})
			continue
		}
		updated++
	}
	return updated, failures
}

// commitLeafDir 把一个子规则源的文件树原子换入 dest。
// 步骤：暂存目录写全部文件（逐个 fsync）→ 写版本标记 → fsync 目录 →
// 旧目录改名让位 → 新目录改名就位 → 删除旧目录。
// 换入失败会回滚旧目录；进程在两次 rename 之间被杀时，dest 可能短暂缺失，
// 但版本标记随之消失，下次同步 loadLocalSourceVersion 得到 0 会重新拉取，不会跳过。
func commitLeafDir(stagingRoot, dest string, files map[string][]byte, rawBody []byte) error {
	stage, err := os.MkdirTemp(stagingRoot, "leaf-")
	if err != nil {
		return err
	}
	defer os.RemoveAll(stage) // 换入成功后路径已不存在，删除为 no-op

	for rel, body := range files {
		target := filepath.Join(stage, rel)
		if err := os.MkdirAll(filepath.Dir(target), 0755); err != nil {
			return fmt.Errorf("创建目录 %s: %w", filepath.Dir(target), err)
		}
		if err := writeFileSync(target, body, 0644); err != nil {
			return fmt.Errorf("写入 %s: %w", target, err)
		}
	}
	// 版本标记最后写：下次同步据此判断是否跳过本源
	if err := writeFileSync(filepath.Join(stage, "_source.json"), rawBody, 0644); err != nil {
		return fmt.Errorf("写入 _source.json: %w", err)
	}
	syncDir(stage)

	if err := os.MkdirAll(filepath.Dir(dest), 0755); err != nil {
		return fmt.Errorf("创建目录 %s: %w", filepath.Dir(dest), err)
	}
	trash, err := os.MkdirTemp(stagingRoot, "trash-")
	if err != nil {
		return err
	}
	// MkdirTemp 建出来的空目录要先让出位置给 rename 用
	trashPath := filepath.Join(trash, "old")
	hadOld := false
	if _, err := os.Lstat(dest); err == nil {
		if err := os.Rename(dest, trashPath); err != nil {
			os.RemoveAll(trash)
			return fmt.Errorf("让位旧目录 %s: %w", dest, err)
		}
		hadOld = true
	}
	if err := os.Rename(stage, dest); err != nil {
		if hadOld { // 回滚，避免目标目录空缺
			_ = os.Rename(trashPath, dest)
		}
		os.RemoveAll(trash)
		return fmt.Errorf("换入新目录 %s: %w", dest, err)
	}
	syncDir(filepath.Dir(dest))
	os.RemoveAll(trash)
	return nil
}

// prepareStagingRoot 准备暂存根目录并清掉上次残留（崩溃时留下的半成品 / 旧目录备份）
// 放在 rules/ 的同级而非其内部：规则加载会遍历 rules/ 下的所有 .toml，
// 暂存目录混进去会被当成规则读进来。
func prepareStagingRoot(home string) (string, error) {
	root := filepath.Join(home, stagingDirName)
	if err := os.RemoveAll(root); err != nil {
		return "", err
	}
	if err := os.MkdirAll(root, 0755); err != nil {
		return "", err
	}
	return root, nil
}

func writeFileSync(path string, data []byte, perm os.FileMode) error {
	f, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, perm)
	if err != nil {
		return err
	}
	if _, err := f.Write(data); err != nil {
		f.Close()
		return err
	}
	if err := f.Sync(); err != nil {
		f.Close()
		return err
	}
	return f.Close()
}

// syncDir fsync 目录本身，使其中的文件名增删落盘（POSIX 语义；Windows 上会失败，忽略）
func syncDir(dir string) {
	d, err := os.Open(dir)
	if err != nil {
		return
	}
	_ = d.Sync()
	d.Close()
}

func gatherLeaves(sources []RuleSource, concurrency int, p *progress.Progress) ([]leafSrc, []syncFailure) {
	sem := make(chan struct{}, concurrency)
	var mu sync.Mutex
	var wg sync.WaitGroup
	usedIDs := make(map[string]bool)
	var leaves []leafSrc
	var failures []syncFailure

	for _, src := range sources {
		wg.Add(1)
		go func(src RuleSource) {
			defer wg.Done()
			sem <- struct{}{}
			si, raw, err := fetchSourceInfo(p.Context(), src.URL)
			<-sem
			if err != nil {
				p.Send("error", src.URL+" 获取失败", 0, 0)
				log.LogfWarn("[sync] 获取 %s 失败: %v", src.URL, err)
				mu.Lock()
				failures = append(failures, syncFailure{Source: src.URL, Error: err.Error()})
				mu.Unlock()
				return
			}
			mu.Lock()
			if usedIDs[si.ID] {
				mu.Unlock()
				return
			}
			usedIDs[si.ID] = true
			mu.Unlock()

			p.Send("list", si.ID, 0, 0)
			sub, subFails := resolveLeaves(si, raw, src.URL, si.ID, usedIDs, sem, &mu, p)
			mu.Lock()
			leaves = append(leaves, sub...)
			failures = append(failures, subFails...)
			mu.Unlock()
		}(src)
	}
	wg.Wait()
	return leaves, failures
}

func resolveLeaves(s *SourceInfo, rawBody []byte, sourceURL, destRel string, usedIDs map[string]bool, sem chan struct{}, mu *sync.Mutex, p *progress.Progress) ([]leafSrc, []syncFailure) {
	isLocal := !strings.HasPrefix(sourceURL, "http://") && !strings.HasPrefix(sourceURL, "https://")
	baseURL := s.BaseURL
	if baseURL == "" {
		baseURL = filepath.Dir(sourceURL)
		if !isLocal {
			if idx := strings.LastIndex(sourceURL, "/"); idx >= 0 {
				baseURL = sourceURL[:idx]
			}
		}
	}
	if !s.IsList() {
		return []leafSrc{{id: s.ID, rawBody: rawBody, destDir: destRel, baseURL: baseURL, files: s.Files, isWeb: !isLocal}}, nil
	}
	var result []leafSrc
	var failures []syncFailure
	// 结构非法的条目已在 fetchSourceInfo → validateSourceFiles 中剔除并报 warn
	for _, f := range s.SubSources {
		subDir := filepath.Dir(f)
		var subURL string
		if isLocal {
			subURL = filepath.Join(baseURL, f)
		} else {
			subURL = strings.TrimSuffix(baseURL, "/") + "/" + f
		}
		sem <- struct{}{}
		subInfo, subRaw, err := fetchSourceInfo(p.Context(), subURL)
		<-sem
		if err != nil {
			p.Send("error", subURL+" 获取失败", 0, 0)
			log.LogfWarn("[sync] 获取 %s 失败: %v", subURL, err)
			failures = append(failures, syncFailure{Source: subURL, Error: err.Error()})
			continue
		}
		if subInfo.ID != subDir {
			continue
		}
		mu.Lock()
		if usedIDs[subInfo.ID] {
			mu.Unlock()
			continue
		}
		usedIDs[subInfo.ID] = true
		mu.Unlock()
		sub, subFails := resolveLeaves(subInfo, subRaw, subURL, filepath.Join(destRel, subDir), usedIDs, sem, mu, p)
		result = append(result, sub...)
		failures = append(failures, subFails...)
	}
	return result, failures
}
