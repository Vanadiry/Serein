package server

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/vanadiry/serein/core/store"
)

func waitSyncUnlocked(t *testing.T, s *Server) {
	t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	for s.syncRunning.Load() {
		if time.Now().After(deadline) {
			t.Fatal("同步结束后未释放运行标记")
		}
		time.Sleep(10 * time.Millisecond)
	}
}

// TestSyncRulesLock 同一时间只能有一个规则同步任务
func TestSyncRulesLock(t *testing.T) {
	block := make(chan struct{})
	released := false
	release := func() {
		if !released {
			released = true
			close(block)
		}
	}
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		<-block
		http.NotFound(w, r)
	}))
	defer ts.Close()
	defer release()

	s := newTestServer(t.TempDir())
	s.config = store.Config{
		RuleSources: []store.RuleSource{{URL: ts.URL + "/_source.json"}},
		Download:    store.DownloadConfig{Concurrency: 2},
	}

	rec := httptest.NewRecorder()
	s.syncRules(rec)
	if rec.Code != http.StatusOK {
		t.Fatalf("首次同步应 200，得到 %d body=%s", rec.Code, rec.Body)
	}

	rec2 := httptest.NewRecorder()
	s.syncRules(rec2)
	if rec2.Code != http.StatusConflict {
		t.Fatalf("并发同步应 409，得到 %d body=%s", rec2.Code, rec2.Body)
	}

	release()
	waitSyncUnlocked(t, s)
}

// TestSyncRulesNoSources 未配置规则源时不占用运行标记
func TestSyncRulesNoSources(t *testing.T) {
	s := &Server{}
	rec := httptest.NewRecorder()
	s.syncRules(rec)
	if rec.Code != http.StatusOK {
		t.Fatalf("无规则源应 200，得到 %d", rec.Code)
	}
	if s.syncRunning.Load() {
		t.Fatal("无规则源不应占用运行标记")
	}
}
