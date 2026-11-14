package server

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/vanadiry/serein/core/store"
)

func waitCheckUnlocked(t *testing.T, s *Server) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for s.checkRunning.Load() {
		if time.Now().After(deadline) {
			t.Fatal("检查结束后未释放运行标记")
		}
		time.Sleep(10 * time.Millisecond)
	}
}

// TestCheckLock 同一时间只能有一个检查任务
func TestCheckLock(t *testing.T) {
	s := newTestServer(t.TempDir())
	list := []checkAllTracker{{
		id: "demo", name: "demo", typ: "app",
		entries: []store.TrackerEntry{{AppID: "nonexistent-app"}},
	}}

	rec := httptest.NewRecorder()
	s.startCheckList(rec, list, true, true)
	if rec.Code != http.StatusOK {
		t.Fatalf("首次检查应 200，得到 %d body=%s", rec.Code, rec.Body)
	}

	rec2 := httptest.NewRecorder()
	s.startCheckList(rec2, list, true, true)
	if rec2.Code != http.StatusConflict {
		t.Fatalf("并发检查应 409，得到 %d body=%s", rec2.Code, rec2.Body)
	}

	waitCheckUnlocked(t, s)
}

// TestCheckEmptyListNoLock 没有可检查条目时不占用运行标记
func TestCheckEmptyListNoLock(t *testing.T) {
	s := newTestServer(t.TempDir())
	rec := httptest.NewRecorder()
	s.startCheckList(rec, []checkAllTracker{{id: "A", name: "A", typ: "app"}}, true, true)
	if rec.Code != http.StatusOK {
		t.Fatalf("空列表应 200，得到 %d", rec.Code)
	}
	if s.checkRunning.Load() {
		t.Fatal("空列表不应占用运行标记")
	}
}
