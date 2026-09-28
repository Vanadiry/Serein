package server

import (
	"bufio"
	"net"
	"net/http"
	"testing"
	"time"
)

// 零值 http.Server 的三项超时都是"无限制"，慢速滴灌 header 的连接
// 可永久占用 goroutine 与 fd，而本服务无鉴权、没有限流手段可用
func TestServerTimeoutsAreSet(t *testing.T) {
	srv := newHTTPServer(http.NewServeMux())
	if srv.ReadHeaderTimeout <= 0 {
		t.Error("ReadHeaderTimeout 未设置：慢速滴灌 header 的连接不会被断开")
	}
	if srv.ReadTimeout <= 0 {
		t.Error("ReadTimeout 未设置：慢速滴灌 body 的连接不会被断开")
	}
	if srv.IdleTimeout <= 0 {
		t.Error("IdleTimeout 未设置：空闲 keep-alive 连接不会被回收")
	}
	// IdleTimeout 为 0 时会回落到 ReadTimeout，两者都为 0 即无任何上限
	if srv.IdleTimeout == 0 && srv.ReadTimeout == 0 {
		t.Error("IdleTimeout 与 ReadTimeout 同时为 0，等于没有任何连接超时")
	}
	if srv.MaxHeaderBytes <= 0 {
		t.Error("MaxHeaderBytes 未显式设置")
	}
	// WriteTimeout 必须留空，否则 SSE 长连接会被掐断
	if srv.WriteTimeout != 0 {
		t.Errorf("WriteTimeout = %v，SSE（/api/events、/api/progress）会被掐断", srv.WriteTimeout)
	}
}

// 真实 slowloris：只发一半 header 然后停住，服务端必须在 ReadHeaderTimeout 内断开
// 修复前该连接会一直挂着，占住一个 goroutine 与 fd
func TestSlowlorisConnectionIsClosed(t *testing.T) {
	old := readHeaderTimeout
	readHeaderTimeout = 300 * time.Millisecond
	t.Cleanup(func() { readHeaderTimeout = old })

	srv := newHTTPServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Error("header 没有发全，请求不该被完全处理")
	}))
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	go srv.Serve(ln)
	t.Cleanup(func() { srv.Close() })

	conn, err := net.Dial("tcp", ln.Addr().String())
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()

	// 发到一半就停住：不发结尾的空行，server 会一直等
	if _, err := conn.Write([]byte("GET /api/check HTTP/1.1\r\nHost: x\r\n")); err != nil {
		t.Fatal(err)
	}

	// 服务端主动断开时读回 EOF 或 connection reset
	_ = conn.SetReadDeadline(time.Now().Add(5 * time.Second))
	start := time.Now()
	br := bufio.NewReader(conn)
	_, err = br.ReadString('\n')
	elapsed := time.Since(start)

	if err == nil {
		t.Fatal("连接未被断开：slowloris 仍可永久占用 goroutine 与 fd")
	}
	if ne, ok := err.(net.Error); ok && ne.Timeout() {
		t.Fatalf("5 秒内服务端未断开连接（读超时）")
	}
	if elapsed > 3*time.Second {
		t.Errorf("断开耗时 %v，远超 ReadHeaderTimeout=%v", elapsed, readHeaderTimeout)
	}
	t.Logf("服务端在 %v 后断开（ReadHeaderTimeout=%v）", elapsed.Round(time.Millisecond), readHeaderTimeout)
}

// 正常请求不应被这些超时误伤（含一个耗时 200ms 的处理器）
func TestNormalRequestNotBroken(t *testing.T) {
	srv := newHTTPServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(200 * time.Millisecond)
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("ok"))
	}))
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	go srv.Serve(ln)
	t.Cleanup(func() { srv.Close() })

	c := &http.Client{Timeout: 5 * time.Second}
	resp, err := c.Get("http://" + ln.Addr().String() + "/x")
	if err != nil {
		t.Fatalf("正常请求失败: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Errorf("status = %d", resp.StatusCode)
	}
}
