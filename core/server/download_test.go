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

// 连续下载后不应留下僵尸进程。
//
// 只测自定义下载器这条路径：未识别的下载器会回退到 OpenBrowser，而 OpenBrowser
// 在 macOS 上是 `open <url>`，测试里跑它会真的用默认浏览器打开测试 URL。
// OpenBrowser 用的同一个 startDetached 在下面单独测。
func TestDownloadDoesNotLeakZombies(t *testing.T) {
	exe, err := exec.LookPath("true")
	if err != nil {
		t.Skip("没有 true 命令")
	}
	cfg := store.Config{}
	// 带 {url} 才会被判定为 dlCustom，否则走 OpenBrowser 回退
	cfg.Download.Downloader = exe + " {url}"
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

	t.Logf("下载 %d 次：僵尸 %d → %d", n, before, after)
	if after > before {
		t.Errorf("僵尸进程增加了 %d 个：Start 之后没有 Wait 回收", after-before)
	}
}

// startDetached 本身：不经过 handler，不会触发任何真实的外部程序
func TestStartDetachedReaps(t *testing.T) {
	exe, err := exec.LookPath("true")
	if err != nil {
		t.Skip("没有 true 命令")
	}
	// 预热
	for i := 0; i < 3; i++ {
		_ = startDetached(exec.Command(exe))
	}
	settle(t)
	before := countZombies(t)

	const n = 40
	for i := 0; i < n; i++ {
		if err := startDetached(exec.Command(exe)); err != nil {
			t.Fatalf("startDetached: %v", err)
		}
	}
	settle(t)
	after := countZombies(t)

	t.Logf("启动 %d 次：僵尸 %d → %d", n, before, after)
	if after > before {
		t.Errorf("僵尸进程增加了 %d 个：Start 之后没有 Wait 回收", after-before)
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
	req := httptest.NewRequest("POST", "/api/call/downloader", bytes.NewReader(body))
	req.RemoteAddr = "127.0.0.1:12345" // isLoopback 要求
	rec := httptest.NewRecorder()
	s.handleCallDownloader(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("code = %d body = %s", rec.Code, rec.Body.String())
	}
}
