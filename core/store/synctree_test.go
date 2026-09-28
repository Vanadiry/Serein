package store

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/vanadiry/serein/core/progress"
)

// writeJSON 写一个 JSON 文件（源树夹具用）
func writeJSON(t *testing.T, path string, v any) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		t.Fatal(err)
	}
	b, err := json.Marshal(v)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, b, 0644); err != nil {
		t.Fatal(err)
	}
}

// seedUpstream 造一个本地源树：list 顶层 + 若干 rules 子源
func seedUpstream(t *testing.T, dir string, top SourceInfo, subs map[string]SourceInfo) {
	t.Helper()
	writeJSON(t, filepath.Join(dir, sourceFileName), top)
	for name, s := range subs {
		writeJSON(t, filepath.Join(dir, name, sourceFileName), s)
	}
}

// 走树：list 顶层 + 3 个 rules 子源，其中一个子源本身是 list（嵌套）
func TestWalkSourceBuildsTree(t *testing.T) {
	home := t.TempDir()
	up := filepath.Join(home, "up")
	seedUpstream(t, up,
		SourceInfo{ID: "T", Type: "list", SubSources: []string{"a/_source.json", "nested/_source.json"}},
		map[string]SourceInfo{
			"a":      {ID: "a", Files: map[string]string{"x.toml": "1"}},
			"nested": {ID: "nested", Type: "list", SubSources: []string{"deep/_source.json"}},
		},
	)
	writeJSON(t, filepath.Join(up, "nested", "deep", sourceFileName),
		SourceInfo{ID: "deep", Files: map[string]string{"z.toml": "1"}})

	claimed := &idClaimer{ids: make(map[string]bool)}
	p := progress.NewProgress(0)
	defer p.Close()

	root, fails := walkSource(context.Background(), mustFetch(t, filepath.Join(up, sourceFileName)),
		nil, filepath.Join(up, sourceFileName), "T", 4, claimed, p)
	if len(fails) != 0 {
		t.Fatalf("fails = %+v", fails)
	}
	if root == nil || !root.info.IsList() || len(root.children) != 2 {
		t.Fatalf("根节点应有 2 个子节点: %+v", root)
	}
	leaves := flattenLeaves(root)
	if len(leaves) != 2 {
		t.Fatalf("叶子数 = %d, want 2: %+v", len(leaves), leaves)
	}
	// 深度优先：a 在前，nested/deep 在后
	if leaves[0].id != "a" || leaves[0].destDir != filepath.Join("T", "a") {
		t.Errorf("第 1 个叶子 = %+v", leaves[0])
	}
	if leaves[1].id != "deep" || leaves[1].destDir != filepath.Join("T", "nested", "deep") {
		t.Errorf("第 2 个叶子 = %+v", leaves[1])
	}
}

// 子源 source_id 与目录名不一致时记失败且不认领，否则无法确定落盘位置
func TestWalkSourceRejectsIDMismatch(t *testing.T) {
	home := t.TempDir()
	up := filepath.Join(home, "up")
	seedUpstream(t, up,
		SourceInfo{ID: "T", Type: "list", SubSources: []string{"dir/_source.json"}},
		map[string]SourceInfo{"dir": {ID: "other", Files: map[string]string{"x.toml": "1"}}},
	)

	claimed := &idClaimer{ids: make(map[string]bool)}
	p := progress.NewProgress(0)
	defer p.Close()

	root, fails := walkSource(context.Background(), mustFetch(t, filepath.Join(up, sourceFileName)),
		nil, filepath.Join(up, sourceFileName), "T", 4, claimed, p)
	if len(fails) != 1 || !strings.Contains(fails[0].Error, "与目录名") {
		t.Fatalf("fails = %+v, want 1 条目录名不一致", fails)
	}
	if leaves := flattenLeaves(root); len(leaves) != 0 {
		t.Errorf("不应产出叶子: %+v", leaves)
	}
	if claimed.claim("other") != true {
		t.Error("ID 不一致的子源不应被认领，否则会挡住后续同名源")
	}
}

// list 型但子源全都没抓到时不视为叶子，否则会拿空 manifest 覆盖整棵树
func TestFlattenLeavesListWithoutChildren(t *testing.T) {
	n := &sourceNode{info: &SourceInfo{ID: "T", Type: "list"}}
	if leaves := flattenLeaves(n); leaves != nil {
		t.Errorf("无子节点的 list 不应产出叶子: %+v", leaves)
	}
	if flattenLeaves(nil) != nil {
		t.Error("nil 节点应返回 nil")
	}
}

// 顶层源串行，ID 认领按配置顺序，先声明的赢
func TestIDClaimerFirstWins(t *testing.T) {
	c := &idClaimer{ids: make(map[string]bool)}
	if !c.claim("a") {
		t.Error("首次认领应成功")
	}
	if c.claim("a") {
		t.Error("重复认领应失败")
	}
	if !c.claim("b") {
		t.Error("不同 ID 应认领成功")
	}
}

// 端到端：本地源树落盘后，二次同步全部跳过
func TestSyncLocalSourceTreeEndToEnd(t *testing.T) {
	home := t.TempDir()
	up := filepath.Join(home, "up")
	seedUpstream(t, up,
		SourceInfo{ID: "T", Type: "list", SubSources: []string{"a/_source.json", "b/_source.json"}},
		map[string]SourceInfo{
			"a": {ID: "a", Files: map[string]string{"x.toml": "1", "y.toml": "1"}},
			"b": {ID: "b", Files: map[string]string{"z.toml": "1"}},
		},
	)
	writeFixture(t, filepath.Join(up, "a", "x.toml"), "X1")
	writeFixture(t, filepath.Join(up, "a", "y.toml"), "Y1")
	writeFixture(t, filepath.Join(up, "b", "z.toml"), "Z1")

	sources := []RuleSource{{URL: filepath.Join(up, sourceFileName)}}
	rulesDir := filepath.Join(home, "rules")

	// 首次同步
	var reloaded1 bool
	p1 := progress.NewProgress(0)
	SyncAllSourcesAsync(home, sources, 4, p1, func() { reloaded1 = true })

	d := &doneEvent{}
	collectDone(p1, d)
	if d.updated != 2 || d.skipped != 0 {
		t.Errorf("首次同步 updated=%d skipped=%d, want 2/0", d.updated, d.skipped)
	}
	if d.failed != 0 || d.fileErrors != 0 {
		t.Errorf("首次同步不应有失败: %+v", d)
	}
	if !reloaded1 {
		t.Error("有更新时应回调 onDone 以重载规则")
	}
	for path, want := range map[string]string{
		filepath.Join(rulesDir, "T", "a", "x.toml"): "X1",
		filepath.Join(rulesDir, "T", "a", "y.toml"): "Y1",
		filepath.Join(rulesDir, "T", "b", "z.toml"): "Z1",
	} {
		if got := mustRead(t, path); got != want {
			t.Errorf("%s = %q, want %q", path, got, want)
		}
	}
	if got := loadLocalFileTokens(filepath.Join(rulesDir, "T", "a")); got["x.toml"] != "1" {
		t.Errorf("a 的 marker token 表 = %v", got)
	}

	// 二次同步时 token 未变，全部跳过且零下载
	var reloaded2 bool
	p2 := progress.NewProgress(0)
	SyncAllSourcesAsync(home, sources, 4, p2, func() { reloaded2 = true })
	d2 := &doneEvent{}
	collectDone(p2, d2)
	if d2.skipped != 2 || d2.updated != 0 {
		t.Errorf("二次同步 skipped=%d updated=%d, want 2/0", d2.skipped, d2.updated)
	}
	if d2.files != 0 {
		t.Errorf("二次同步不应下载任何文件，files=%d", d2.files)
	}
	if reloaded2 {
		t.Error("无更新时不应回调 onDone")
	}

	// 改一个文件的 token 时只有那一个源需要更新，且只下载一个文件
	writeJSON(t, filepath.Join(up, "a", sourceFileName),
		SourceInfo{ID: "a", Files: map[string]string{"x.toml": "2", "y.toml": "1"}})
	writeFixture(t, filepath.Join(up, "a", "x.toml"), "X2")

	p3 := progress.NewProgress(0)
	SyncAllSourcesAsync(home, sources, 4, p3, nil)
	d3 := &doneEvent{}
	collectDone(p3, d3)
	if d3.updated != 1 || d3.skipped != 1 {
		t.Errorf("第三次同步 updated=%d skipped=%d, want 1/1", d3.updated, d3.skipped)
	}
	if d3.files != 1 {
		t.Errorf("只应下载 1 个文件，files=%d", d3.files)
	}
	if got := mustRead(t, filepath.Join(rulesDir, "T", "a", "x.toml")); got != "X2" {
		t.Errorf("x.toml = %q, want X2", got)
	}
	// 未变的文件必须仍在（整树换入时靠 carry 保住）
	if got := mustRead(t, filepath.Join(rulesDir, "T", "a", "y.toml")); got != "Y1" {
		t.Errorf("未变更的 y.toml 丢失或被改写: %q", got)
	}
	if got := mustRead(t, filepath.Join(rulesDir, "T", "b", "z.toml")); got != "Z1" {
		t.Errorf("其他源不应受影响: %q", got)
	}
}

// 上游删掉一个文件时，整树换入后应消失且不计入下载数
func TestSyncRemovesDeletedFile(t *testing.T) {
	home := t.TempDir()
	up := filepath.Join(home, "up")
	seedUpstream(t, up,
		SourceInfo{ID: "T", Type: "list", SubSources: []string{"a/_source.json"}},
		map[string]SourceInfo{"a": {ID: "a", Files: map[string]string{"keep.toml": "1", "gone.toml": "1"}}},
	)
	writeFixture(t, filepath.Join(up, "a", "keep.toml"), "K")
	writeFixture(t, filepath.Join(up, "a", "gone.toml"), "G")
	sources := []RuleSource{{URL: filepath.Join(up, sourceFileName)}}
	rulesDir := filepath.Join(home, "rules")

	p1 := progress.NewProgress(0)
	SyncAllSourcesAsync(home, sources, 2, p1, nil)
	collectDone(p1, &doneEvent{})

	writeJSON(t, filepath.Join(up, "a", sourceFileName),
		SourceInfo{ID: "a", Files: map[string]string{"keep.toml": "1"}})
	p2 := progress.NewProgress(0)
	SyncAllSourcesAsync(home, sources, 2, p2, nil)
	collectDone(p2, &doneEvent{})

	if _, err := os.Stat(filepath.Join(rulesDir, "T", "a", "gone.toml")); !os.IsNotExist(err) {
		t.Error("上游已删除的文件仍留在盘上")
	}
	if got := mustRead(t, filepath.Join(rulesDir, "T", "a", "keep.toml")); got != "K" {
		t.Errorf("keep.toml = %q", got)
	}
}

// writeFixture 写一个夹具文件（自动建目录）
func writeFixture(t *testing.T, path, body string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(body), 0644); err != nil {
		t.Fatal(err)
	}
}

func mustFetch(t *testing.T, path string) *SourceInfo {
	t.Helper()
	s, _, err := fetchSourceInfo(context.Background(), path)
	if err != nil {
		t.Fatalf("读取 %s 失败: %v", path, err)
	}
	return s
}

// doneEvent 同步结束时上报的计数
type doneEvent struct {
	updated, skipped, failed, files, fileErrors int
	failures                                    []syncFailure
	cancelled                                   bool
}

// collectDone 消费 progress 通道直到关闭，取出第一个 done 事件
// SyncAllSourcesAsync 先发一个带完整计数的 done，defer p.Close() 又发一个裸的 done
// 前端在第一个就关闭连接，所以这里也只取第一个
func collectDone(p *progress.Progress, d *doneEvent) {
	for line := range p.Channel {
		var m struct {
			Step           string        `json:"step"`
			SourcesUpdated int           `json:"sources_updated"`
			SourcesSkipped int           `json:"sources_skipped"`
			SourcesFailed  int           `json:"sources_failed"`
			Files          int           `json:"files"`
			FileErrors     int           `json:"file_errors"`
			Failures       []syncFailure `json:"failures"`
			Cancelled      bool          `json:"cancelled"`
		}
		if json.Unmarshal([]byte(line), &m) != nil || m.Step != "done" {
			continue
		}
		d.updated, d.skipped, d.failed = m.SourcesUpdated, m.SourcesSkipped, m.SourcesFailed
		d.files, d.fileErrors = m.Files, m.FileErrors
		d.failures, d.cancelled = m.Failures, m.Cancelled
		return
	}
}

// worker pool 的边界：任务数为 0 / 少于并发数时不得死锁或起多余的 worker
func TestDownloadLeavesDegenerateTaskCounts(t *testing.T) {
	home := t.TempDir()
	rulesDir := filepath.Join(home, "rules")
	up := filepath.Join(home, "up")
	if err := os.MkdirAll(filepath.Join(up, "a"), 0755); err != nil {
		t.Fatal(err)
	}
	writeFixture(t, filepath.Join(up, "a", "one.toml"), "ONE")

	for _, tc := range []struct {
		name   string
		conc   int
		leaves []leafSrc
	}{
		{"无任务", 4, nil},
		{"任务数少于并发数", 8, []leafSrc{{
			id: "a", destDir: "a", baseURL: filepath.Join(up, "a"), isWeb: false,
			files: map[string]string{"one.toml": "1"},
			need:  map[string]string{"one.toml": "1"},
		}}},
		{"并发数为 0", 0, []leafSrc{{
			id: "a", destDir: "a", baseURL: filepath.Join(up, "a"), isWeb: false,
			files: map[string]string{"one.toml": "1"},
			need:  map[string]string{"one.toml": "1"},
		}}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			p := progress.NewProgress(0)
			defer p.Close()
			pg := &progState{}
			contents, failed, fails, fileErrors := downloadLeaves(
				context.Background(), rulesDir, tc.leaves, tc.conc, p, pg)
			if fileErrors != 0 || len(fails) != 0 {
				t.Errorf("不应有失败: %v %d", fails, fileErrors)
			}
			for i := range failed {
				if failed[i] {
					t.Errorf("leaf %d 被标记失败", i)
				}
			}
			if len(tc.leaves) > 0 && string(contents[0]["one.toml"]) != "ONE" {
				t.Errorf("内容 = %q", contents[0]["one.toml"])
			}
			if pg.done != len(tc.leaves) {
				t.Errorf("done = %d, want %d", pg.done, len(tc.leaves))
			}
		})
	}
}

// 取消：不得死锁（派发端与 worker 都必须能从 ctx 撤出），且不返回任何内容
func TestDownloadLeavesCancelDoesNotDeadlock(t *testing.T) {
	home := t.TempDir()
	up := filepath.Join(home, "up")
	if err := os.MkdirAll(filepath.Join(up, "a"), 0755); err != nil {
		t.Fatal(err)
	}
	need := make(map[string]string, 200)
	for i := 0; i < 200; i++ {
		name := "f" + strconv.Itoa(i) + ".toml"
		writeFixture(t, filepath.Join(up, "a", name), "X")
		need[name] = "1"
	}
	leaves := []leafSrc{{
		id: "a", destDir: "a", baseURL: filepath.Join(up, "a"),
		files: need, need: need,
	}}

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // 一开始就取消：最坏情况是派发端先撞上满队列

	p := progress.NewProgress(0)
	defer p.Close()
	done := make(chan struct{})
	go func() {
		defer close(done)
		downloadLeaves(ctx, filepath.Join(home, "rules"), leaves, 4, p, &progState{})
	}()
	select {
	case <-done:
	case <-time.After(10 * time.Second):
		t.Fatal("取消后 downloadLeaves 未返回（死锁）")
	}
}

// #3 回归：上游返回空 files 而本地已有规则文件时，必须拒绝提交而非把规则清光
// 触发条件常见：上游 manifest 被截断、临时故障、站点正在重建
func TestSyncRejectsEmptyManifest(t *testing.T) {
	home := t.TempDir()
	up := filepath.Join(home, "up")
	seedUpstream(t, up,
		SourceInfo{ID: "T", Type: "list", SubSources: []string{"a/_source.json"}},
		map[string]SourceInfo{"a": {ID: "a", Files: map[string]string{"x.toml": "1", "y.toml": "1"}}},
	)
	writeFixture(t, filepath.Join(up, "a", "x.toml"), "X")
	writeFixture(t, filepath.Join(up, "a", "y.toml"), "Y")
	sources := []RuleSource{{URL: filepath.Join(up, sourceFileName)}}
	rulesDir := filepath.Join(home, "rules")

	p1 := progress.NewProgress(0)
	SyncAllSourcesAsync(home, sources, 2, p1, nil)
	collectDone(p1, &doneEvent{})

	// manifest 被清空
	writeJSON(t, filepath.Join(up, "a", sourceFileName), SourceInfo{ID: "a"})
	p2 := progress.NewProgress(0)
	SyncAllSourcesAsync(home, sources, 2, p2, nil)
	d := &doneEvent{}
	collectDone(p2, d)

	if got := mustRead(t, filepath.Join(rulesDir, "T", "a", "x.toml")); got != "X" {
		t.Errorf("规则文件被清掉: x.toml = %q", got)
	}
	if got := mustRead(t, filepath.Join(rulesDir, "T", "a", "y.toml")); got != "Y" {
		t.Errorf("规则文件被清掉: y.toml = %q", got)
	}
	if d.updated != 0 || d.failed != 1 {
		t.Errorf("updated=%d failed=%d, want 0/1（应报失败而非成功）", d.updated, d.failed)
	}
	if len(d.failures) != 1 || !strings.Contains(d.failures[0].Error, "未列出任何规则文件") {
		t.Errorf("failures = %+v", d.failures)
	}
}

// 空的全新源（本地本来就没有规则文件）应当正常提交，不被保护误伤
func TestSyncAllowsEmptyManifestOnFreshSource(t *testing.T) {
	home := t.TempDir()
	up := filepath.Join(home, "up")
	seedUpstream(t, up,
		SourceInfo{ID: "T", Type: "list", SubSources: []string{"a/_source.json"}},
		map[string]SourceInfo{"a": {ID: "a"}},
	)
	sources := []RuleSource{{URL: filepath.Join(up, sourceFileName)}}

	p := progress.NewProgress(0)
	SyncAllSourcesAsync(home, sources, 2, p, nil)
	d := &doneEvent{}
	collectDone(p, d)

	if d.failed != 0 {
		t.Errorf("全新空源不应被判失败: %+v", d)
	}
	// marker 仍应落盘，源在界面上可见
	if got := loadLocalFileTokens(filepath.Join(home, "rules", "T", "a")); got != nil {
		t.Errorf("marker 应已落盘, got %v", got)
	}
	if _, err := os.Stat(filepath.Join(home, "rules", "T", "a", sourceFileName)); err != nil {
		t.Errorf("marker 未落盘: %v", err)
	}
}

// 不变量 C：上游从 list 里删掉某个子源后，本地孤儿子源目录应被清除
func TestPruneOrphanSubSources(t *testing.T) {
	home := t.TempDir()
	rulesDir := filepath.Join(home, "rules")
	up := filepath.Join(home, "up")
	seedUpstream(t, up,
		SourceInfo{ID: "T", Type: "list", SubSources: []string{"keep/_source.json"}},
		map[string]SourceInfo{"keep": {ID: "keep", Files: map[string]string{"k.toml": "1"}}},
	)
	writeFixture(t, filepath.Join(up, "keep", "k.toml"), "K")
	sources := []RuleSource{{URL: filepath.Join(up, sourceFileName)}}

	p1 := progress.NewProgress(0)
	SyncAllSourcesAsync(home, sources, 2, p1, nil)
	collectDone(p1, &doneEvent{})

	// 上游移除 keep，新增 gone；本地额外留一个用户手放的目录
	writeJSON(t, filepath.Join(up, sourceFileName),
		SourceInfo{ID: "T", Type: "list", SubSources: []string{"gone/_source.json"}})
	writeJSON(t, filepath.Join(up, "gone", sourceFileName),
		SourceInfo{ID: "gone", Files: map[string]string{"g.toml": "1"}})
	writeFixture(t, filepath.Join(up, "gone", "g.toml"), "G")
	if err := os.MkdirAll(filepath.Join(rulesDir, "T", "usermanual"), 0755); err != nil {
		t.Fatal(err)
	}
	// 先手工补一个"上游已删但本地还在"的孤儿子源
	writeJSON(t, filepath.Join(rulesDir, "T", "orphan", sourceFileName),
		SourceInfo{ID: "orphan", Files: map[string]string{"o.toml": "1"}})
	writeFixture(t, filepath.Join(rulesDir, "T", "orphan", "o.toml"), "O")

	p2 := progress.NewProgress(0)
	SyncAllSourcesAsync(home, sources, 2, p2, nil)
	collectDone(p2, &doneEvent{})

	if _, err := os.Stat(filepath.Join(rulesDir, "T", "orphan")); !os.IsNotExist(err) {
		t.Error("孤儿子源目录未被清除")
	}
	if got := mustRead(t, filepath.Join(rulesDir, "T", "gone", "g.toml")); got != "G" {
		t.Errorf("新子源未落盘: %q", got)
	}
	// 门禁 2：无 marker 的用户目录不得被动
	if _, err := os.Stat(filepath.Join(rulesDir, "T", "usermanual")); err != nil {
		t.Errorf("用户手放目录被误删: %v", err)
	}
}

// 门禁 1：本轮有子源抓取失败时，不变量 C 一律不执行
// 无法区分"上游删掉了它"与"这次没抓到"
func TestPruneSkippedWhenWalkFailed(t *testing.T) {
	home := t.TempDir()
	rulesDir := filepath.Join(home, "rules")
	up := filepath.Join(home, "up")
	seedUpstream(t, up,
		SourceInfo{ID: "T", Type: "list", SubSources: []string{"keep/_source.json", "flaky/_source.json"}},
		map[string]SourceInfo{
			"keep":  {ID: "keep", Files: map[string]string{"k.toml": "1"}},
			"flaky": {ID: "flaky", Files: map[string]string{"f.toml": "1"}},
		},
	)
	writeFixture(t, filepath.Join(up, "keep", "k.toml"), "K")
	writeFixture(t, filepath.Join(up, "flaky", "f.toml"), "F")
	sources := []RuleSource{{URL: filepath.Join(up, sourceFileName)}}

	p1 := progress.NewProgress(0)
	SyncAllSourcesAsync(home, sources, 2, p1, nil)
	collectDone(p1, &doneEvent{})

	// 上游删掉 flaky 且弄坏它的 marker 时抓取失败
	os.Remove(filepath.Join(up, "flaky", sourceFileName))
	writeJSON(t, filepath.Join(up, sourceFileName),
		SourceInfo{ID: "T", Type: "list", SubSources: []string{"keep/_source.json", "flaky/_source.json"}})
	// 改一下 keep 的 token，确保本轮确实会提交（否则清理逻辑压根不会走到）
	writeJSON(t, filepath.Join(up, "keep", sourceFileName),
		SourceInfo{ID: "keep", Files: map[string]string{"k.toml": "2"}})
	writeFixture(t, filepath.Join(up, "keep", "k.toml"), "K2")

	p2 := progress.NewProgress(0)
	SyncAllSourcesAsync(home, sources, 2, p2, nil)
	d := &doneEvent{}
	collectDone(p2, d)

	if len(d.failures) == 0 {
		t.Fatal("本应有子源抓取失败")
	}
	// 门禁 1：flaky 已从上游消失，但不得因为"没抓到"就当成孤儿删掉
	if _, err := os.Stat(filepath.Join(rulesDir, "T", "flaky", "f.toml")); err != nil {
		t.Errorf("门禁 1 未生效，flaky 被当成孤儿删除: %v", err)
	}
}

// 不变量 D：manifest 声明的文件数超上限时整个源被拒绝，不得逐条同步
func TestValidateRejectsTooManyFiles(t *testing.T) {
	old := maxFilesPerSource
	maxFilesPerSource = 5
	t.Cleanup(func() { maxFilesPerSource = old })

	files := make(map[string]string, 8)
	for i := 0; i < 8; i++ {
		files["f"+strconv.Itoa(i)+".toml"] = "1"
	}
	s := SourceInfo{ID: "big", Files: files}
	issues, rejected := validateSourceFiles(&s)
	if !rejected {
		t.Fatal("超过上限应整体拒绝")
	}
	if len(issues) == 0 || issues[len(issues)-1].Level != "error" {
		t.Errorf("应报 error 级问题: %+v", issues)
	}
	// 端到端：被拒绝的源不应落盘任何内容
	home := t.TempDir()
	up := filepath.Join(home, "up")
	seedUpstream(t, up,
		SourceInfo{ID: "T", Type: "list", SubSources: []string{"big/_source.json"}},
		map[string]SourceInfo{"big": {ID: "big", Files: files}},
	)
	for name := range files {
		writeFixture(t, filepath.Join(up, "big", name), "X")
	}
	p := progress.NewProgress(0)
	SyncAllSourcesAsync(home, []RuleSource{{URL: filepath.Join(up, sourceFileName)}}, 2, p, nil)
	d := &doneEvent{}
	collectDone(p, d)
	if d.updated != 0 || d.failed == 0 {
		t.Errorf("被拒绝的源不应更新，应报失败: %+v", d)
	}
	if _, err := os.Stat(filepath.Join(home, "rules", "T", "big")); !os.IsNotExist(err) {
		t.Error("被拒绝的源仍被落盘")
	}
}

// 不变量 D：本轮下载体积超上限时放弃该源，保留本地现有规则
func TestDownloadBudgetAbandonsSource(t *testing.T) {
	home := t.TempDir()
	up := filepath.Join(home, "up")
	need := make(map[string]string, 6)
	for i := 0; i < 6; i++ {
		name := "f" + strconv.Itoa(i) + ".toml"
		writeFixture(t, filepath.Join(up, "a", name), strings.Repeat("X", 40))
		need[name] = "1"
	}
	seedUpstream(t, up,
		SourceInfo{ID: "T", Type: "list", SubSources: []string{"a/_source.json"}},
		map[string]SourceInfo{"a": {ID: "a", Files: need}},
	)
	rulesDir := filepath.Join(home, "rules")
	// 先建立本地基线：正常同步一次
	p1 := progress.NewProgress(0)
	SyncAllSourcesAsync(home, []RuleSource{{URL: filepath.Join(up, sourceFileName)}}, 1, p1, nil)
	collectDone(p1, &doneEvent{})
	if _, err := os.Stat(filepath.Join(rulesDir, "T", "a", "f0.toml")); err != nil {
		t.Fatalf("首次同步未落盘: %v", err)
	}

	// 全部 token 变新时本轮需重下，此时才调低预算以触发超限
	old := maxSourceBytes
	maxSourceBytes = 64 // 每个测试文件 40 字节，第 3 个就超
	t.Cleanup(func() { maxSourceBytes = old })
	for name := range need {
		need[name] = "2"
		writeFixture(t, filepath.Join(up, "a", name), strings.Repeat("Y", 40))
	}
	writeJSON(t, filepath.Join(up, "a", sourceFileName), SourceInfo{ID: "a", Files: need})
	p2 := progress.NewProgress(0)
	SyncAllSourcesAsync(home, []RuleSource{{URL: filepath.Join(up, sourceFileName)}}, 1, p2, nil)
	d := &doneEvent{}
	collectDone(p2, d)

	if d.updated != 0 || d.failed == 0 {
		t.Errorf("超预算的源不应更新，应报失败: %+v", d)
	}
	found := false
	for _, f := range d.failures {
		if strings.Contains(f.Error, "超过上限") {
			found = true
		}
	}
	if !found {
		t.Errorf("失败原因应说明超限: %+v", d.failures)
	}
	// 放弃该源时 marker 不推进，本地内容原样保留
	if got := loadLocalFileTokens(filepath.Join(rulesDir, "T", "a")); got["f0.toml"] != "1" {
		t.Errorf("marker 被推进: %v", got)
	}
	if got := mustRead(t, filepath.Join(rulesDir, "T", "a", "f0.toml")); got[:1] != "X" {
		t.Errorf("本地内容被改写: %q", got)
	}
}
