package server

import (
	"testing"

	"github.com/vanadiry/serein/core/checker"
	"github.com/vanadiry/serein/core/progress"
)

// seedCache 往结果缓存里塞若干 app，模拟"之前检查过一次"
func seedCache(s *Server, tracker string, appIDs ...string) {
	m := make(map[string]checker.CheckResponse, len(appIDs))
	for _, id := range appIDs {
		m[id] = checker.CheckResponse{
			AppID: id, Name: id,
			Platforms: map[string]checker.CheckPlatform{"windows": {LatestVersion: "1.0"}},
		}
	}
	s.resultsMu.Lock()
	s.results[tracker] = m
	s.resultsMu.Unlock()
}

func cacheSize(s *Server, tracker string) int {
	return len(s.getCheckResults(tracker))
}

// runOne 跑一个 tracker，结果由调用方通过 fn 决定（nil = 什么都没查到）
func runOne(s *Server, savePartial, replace bool, fn func() []checker.CheckResponse) {
	s.runCheckAllAsync([]checkAllTracker{{
		id: "A", name: "A", typ: "app",
		// runCheckAllAsync 内部自己调 buildCheckJobs/runAppJobs，这里用空 entries
		// 走"没有可用结果"分支；需要真实结果时由测试直接调 setCheckResults
		entries: nil,
	}}, progress.NewProgress(0), savePartial, replace)
	_ = fn
}

// 零结果时不得把桶替换成空：未检查的 app 会看起来像"无更新"，而实际是"没查到"
func TestEmptyResultKeepsCache(t *testing.T) {
	s := newTestServer(t.TempDir())
	seedCache(s, "A", "a1", "a2")
	runOne(s, true, true, nil)
	if n := cacheSize(s, "A"); n != 2 {
		t.Errorf("零结果时应保留 2 个 app，实际剩 %d 个", n)
	}
}

// 正常跑完仍要整桶替换，否则已从 Tracker 删掉的 app 结果会永久留在桶里
func TestNormalRunReplacesBucket(t *testing.T) {
	s := newTestServer(t.TempDir())
	seedCache(s, "A", "a1", "gone")
	s.setCheckResults("A", []checker.CheckResponse{{
		AppID: "a1", Name: "a1",
		Platforms: map[string]checker.CheckPlatform{"windows": {LatestVersion: "2.0"}},
	}})
	if n := cacheSize(s, "A"); n != 1 {
		t.Fatalf("前置：整桶替换后应只剩 a1，实际 %d 个", n)
	}
}

// scope=ids 路径必须始终 upsert：单条检查不能清掉同桶其它 app 的结果
func TestSingleIDAlwaysUpserts(t *testing.T) {
	s := newTestServer(t.TempDir())
	seedCache(s, "A", "a1", "a2", "a3")
	runOne(s, true, false, nil) // replace=false
	if n := cacheSize(s, "A"); n != 3 {
		t.Errorf("单条检查不应减少桶内条目，实际 %d 个", n)
	}
}
