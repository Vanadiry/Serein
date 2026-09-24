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

	"github.com/vanadiry/serein/core/events"
	"github.com/vanadiry/serein/core/httpx"
	"github.com/vanadiry/serein/core/progress"
)

const maxFetchBytes = 8 << 20 // 8MB

// SourceJSON 规则源的元信息（完整 _source.json 内容）
type SourceJSON struct {
	ID          string   `json:"source_id"`
	Name        string   `json:"name,omitempty"`
	Description string   `json:"description,omitempty"`
	Type        string   `json:"type,omitempty"` // "rules"（默认）或 "list"
	BaseURL     string   `json:"baseurl,omitempty"`
	Version     int      `json:"version,omitempty"`
	Files       []string `json:"files"`
}

// readURLOrFile 读取内容：http(s) 走网络（带 ctx / 状态码校验 / 大小上限），否则读本地文件
func readURLOrFile(ctx context.Context, rawURL string, limit int64) ([]byte, error) {
	if !strings.HasPrefix(rawURL, "http://") && !strings.HasPrefix(rawURL, "https://") {
		return os.ReadFile(rawURL)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
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

func fetchSourceJSON(ctx context.Context, url string) (*SourceJSON, []byte, error) {
	if err := ctx.Err(); err != nil {
		return nil, nil, err
	}
	body, err := readURLOrFile(ctx, url, maxFetchBytes)
	if err != nil {
		return nil, nil, err
	}

	var s SourceJSON
	if err := json.Unmarshal(body, &s); err != nil {
		return nil, nil, fmt.Errorf("解析 _source.json: %w", err)
	}
	if s.ID == "" {
		return nil, nil, fmt.Errorf("_source.json 缺少 source_id")
	}
	if !regexp.MustCompile(`^[a-zA-Z0-9_-]+$`).MatchString(s.ID) {
		return nil, nil, fmt.Errorf("source_id %q 包含非法字符，仅允许大小写字母、数字、下划线和连字符", s.ID)
	}
	return &s, body, nil
}

func loadLocalSourceVersion(dir string) int {
	data, err := os.ReadFile(filepath.Join(dir, "_source.json"))
	if err != nil {
		return 0
	}
	var s SourceJSON
	if json.Unmarshal(data, &s) != nil {
		return 0
	}
	return s.Version
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
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, fmt.Errorf("下载 %s: %w", url, err)
	}
	resp, err := httpx.DefaultClient().Do(req)
	if err != nil {
		return nil, fmt.Errorf("下载 %s: %w", url, err)
	}
	defer resp.Body.Close()
	if err := httpx.CheckStatus(resp); err != nil {
		return nil, fmt.Errorf("下载 %s: %w", url, err)
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, maxFetchBytes))
	if err != nil {
		return nil, err
	}
	return body, nil
}

// 异步同步（带进度）

type leafSrc struct {
	id      string
	rawBody []byte
	destDir string
	baseURL string
	files   []string
	isWeb   bool
	version int
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
	if onDone != nil {
		defer onDone()
	}
	defer p.Close()

	ctx := p.Context()
	rulesDir := filepath.Join(home, "rules")

	// Phase 1: 遍历
	leaves := gatherLeaves(sources, concurrency, p)
	if len(leaves) == 0 {
		return
	}

	sourcesTotal := len(leaves)

	// 过滤版本未变的子规则源
	var fresh []leafSrc
	var skipped []leafSrc
	for _, l := range leaves {
		if l.version > 0 && loadLocalSourceVersion(filepath.Join(rulesDir, l.destDir)) == l.version {
			p.SendMap(map[string]any{
				"step":  "skip",
				"name":  l.id,
				"files": len(l.files),
			})
			skipped = append(skipped, l)
			continue
		}
		fresh = append(fresh, l)
	}
	leaves = fresh
	sourcesSkipped := len(skipped)
	sourcesUpdated := 0

	totalFiles := 0
	for _, l := range leaves {
		totalFiles += len(l.files)
	}

	if totalFiles > 0 {
		for _, l := range leaves {
			p.SendMap(map[string]any{
				"step":  "source",
				"name":  l.id,
				"files": len(l.files),
			})
		}
		p.Send("start", "", 0, totalFiles)
	}

	fileErrors := 0
	sourcesFailed := 0
	var failures []syncFailure

	if totalFiles > 0 {
		sem := make(chan struct{}, concurrency)
		var mu sync.Mutex
		var done int
		var wg sync.WaitGroup

		// 内存暂存：每个子规则源的文件内容，仅当该源全部文件成功时才提交
		contents := make([]map[string][]byte, len(leaves))
		leafFailed := make([]bool, len(leaves))
		for i := range leaves {
			contents[i] = make(map[string][]byte)
		}

		for i := range leaves {
			l := leaves[i]
			base := filepath.Join(rulesDir, l.destDir)
			for _, f := range l.files {
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

		for i := range leaves {
			if leafFailed[i] {
				sourcesFailed++
			}
		}

		// 提交：仅整体替换全部文件成功的源；取消时不提交
		if ctx.Err() == nil {
			for i := range leaves {
				if leafFailed[i] {
					continue
				}
				l := leaves[i]
				dest := filepath.Join(rulesDir, l.destDir)
				if err := os.RemoveAll(dest); err != nil {
					events.Emit("error", "[sync]", fmt.Sprintf("清理目录失败 %s: %v", dest, err))
					continue
				}
				if err := os.MkdirAll(dest, 0755); err != nil {
					events.Emit("error", "[sync]", fmt.Sprintf("创建目录失败 %s: %v", dest, err))
					continue
				}
				for rel, body := range contents[i] {
					target := filepath.Join(dest, rel)
					if err := os.MkdirAll(filepath.Dir(target), 0755); err != nil {
						events.Emit("error", "[sync]", fmt.Sprintf("创建目录失败 %s: %v", filepath.Dir(target), err))
						continue
					}
					if err := os.WriteFile(target, body, 0644); err != nil {
						events.Emit("error", "[sync]", fmt.Sprintf("写入文件失败 %s: %v", target, err))
					}
				}
				if err := os.WriteFile(filepath.Join(dest, "_source.json"), l.rawBody, 0644); err != nil {
					events.Emit("error", "[sync]", fmt.Sprintf("写入 _source.json 失败 %s: %v", l.destDir, err))
				}
				sourcesUpdated++
			}
		}
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

func gatherLeaves(sources []RuleSource, concurrency int, p *progress.Progress) []leafSrc {
	sem := make(chan struct{}, concurrency)
	var mu sync.Mutex
	var wg sync.WaitGroup
	usedIDs := make(map[string]bool)
	var leaves []leafSrc

	for _, src := range sources {
		wg.Add(1)
		go func(src RuleSource) {
			defer wg.Done()
			sem <- struct{}{}
			js, raw, err := fetchSourceJSON(p.Context(), src.URL)
			<-sem
			if err != nil {
				p.Send("error", src.URL+" 获取失败", 0, 0)
				return
			}
			mu.Lock()
			if usedIDs[js.ID] {
				mu.Unlock()
				return
			}
			usedIDs[js.ID] = true
			mu.Unlock()

			p.Send("list", js.ID, 0, 0)
			sub := resolveLeaves(js, raw, src.URL, js.ID, usedIDs, sem, &mu, p)
			mu.Lock()
			leaves = append(leaves, sub...)
			mu.Unlock()
		}(src)
	}
	wg.Wait()
	return leaves
}

func resolveLeaves(s *SourceJSON, rawBody []byte, sourceURL, destRel string, usedIDs map[string]bool, sem chan struct{}, mu *sync.Mutex, p *progress.Progress) []leafSrc {
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
	if s.Type != "list" {
		return []leafSrc{{id: s.ID, rawBody: rawBody, destDir: destRel, baseURL: baseURL, files: s.Files, isWeb: !isLocal, version: s.Version}}
	}
	var result []leafSrc
	for _, f := range s.Files {
		if filepath.Base(f) != "_source.json" {
			continue
		}
		subDir := filepath.Dir(f)
		var subURL string
		if isLocal {
			subURL = filepath.Join(baseURL, f)
		} else {
			subURL = strings.TrimSuffix(baseURL, "/") + "/" + f
		}
		sem <- struct{}{}
		subJSON, subRaw, err := fetchSourceJSON(p.Context(), subURL)
		<-sem
		if err != nil {
			p.Send("error", subURL+" 获取失败", 0, 0)
			continue
		}
		if subJSON.ID != subDir {
			continue
		}
		mu.Lock()
		if usedIDs[subJSON.ID] {
			mu.Unlock()
			continue
		}
		usedIDs[subJSON.ID] = true
		mu.Unlock()
		result = append(result, resolveLeaves(subJSON, subRaw, subURL, filepath.Join(destRel, subDir), usedIDs, sem, mu, p)...)
	}
	return result
}
