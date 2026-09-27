package server

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os/exec"
	"strings"
	"testing"
	"time"

	"github.com/vanadiry/serein/core/store"
)

// countZombies 数当前有多少僵尸进程。Start 之后不 Wait 回收时，
// 每下载一次就留下一个，直到 Serein 自己退出。
func countZombies(t *testing.T) int {
	t.Helper()
	out, err := exec.Command("ps", "-axo", "stat=").Output()
	if err != nil {
		t.Skipf("ps 不可用: %v", err)
	}
	n := 0
	for _, f := range strings.Fields(string(out)) {
		if strings.HasPrefix(f, "Z") {
			n++
		}
	}
	return n
}

// 连续下载后不应留下僵尸进程
func TestDownloadDoesNotLeakZombies(t *testing.T) {
	if _, err := exec.LookPath("true"); err != nil {
		t.Skip("没有 true 命令")
	}
	// 两条路径都会起子进程：自定义下载器，以及未识别的下载器回退到 OpenBrowser。
	// 两条都要试——OpenBrowser 被点链接、下载回退、错误页调用，频率更高。
	for _, tc := range []struct{ name, downloader string }{
		{"自定义下载器", "/usr/bin/true {url}"},
		{"回退到 OpenBrowser", "/usr/bin/true"}, // 无 {url} → dlUnknown → OpenBrowser
	} {
		t.Run(tc.name, func(t *testing.T) {
			cfg := store.Config{}
			cfg.Download.Downloader = tc.downloader
			s := &Server{config: cfg}

			// 预热，避免把首次调度的噪声算进来
			for i := 0; i < 3; i++ {
				doDownloadReq(t, s)
			}
			settle(t)
			before := countZombies(t)

			const n = 40
			for i := 0; i < n; i++ {
				doDownloadReq(t, s)
			}
			settle(t)
			after := countZombies(t)

			t.Logf("触发 %d 次：僵尸 %d → %d", n, before, after)
			if after > before {
				t.Errorf("僵尸进程增加了 %d 个：Start 之后没有 Wait 回收", after-before)
			}
		})
	}
}

// settle 等 Wait 的 goroutine 把僵尸回收完（最多 3 秒）
func settle(t *testing.T) {
	t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		if countZombies(t) == 0 {
			return
		}
		time.Sleep(50 * time.Millisecond)
	}
}

func doDownloadReq(t *testing.T, s *Server) {
	t.Helper()
	body, _ := json.Marshal(map[string]string{"url": "https://example.com/x.zip"})
	req := httptest.NewRequest("POST", "/api/download", bytes.NewReader(body))
	req.RemoteAddr = "127.0.0.1:12345" // isLoopback 要求
	rec := httptest.NewRecorder()
	s.handleDownload(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("code = %d body = %s", rec.Code, rec.Body.String())
	}
}
