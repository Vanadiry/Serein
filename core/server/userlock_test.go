package server

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync"
	"testing"

	"github.com/vanadiry/serein/core/store"
)

func confirm(t *testing.T, s *Server, body string) {
	t.Helper()
	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/check/confirm", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	s.mux.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("确认失败 %d: %s", rr.Code, rr.Body.String())
	}
}

// 并发确认不能互相覆盖：两个请求各自加载再保存，后写的会用自己的旧快照
// 把先写的那份整个盖掉，先确认的版本就永久丢了
func TestConcurrentConfirmKeepsBoth(t *testing.T) {
	home := t.TempDir()
	s := newTestServer(home)
	s.registerRoutes()

	const n = 20
	var wg sync.WaitGroup
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			confirm(t, s, `{"app_id":"app`+itoa(i)+`","windows":"1.0.0"}`)
		}(i)
	}
	wg.Wait()

	ud, err := store.LoadUserData(home)
	if err != nil {
		t.Fatal(err)
	}
	if len(ud) != n {
		t.Errorf("%d 个并发确认后应保留 %d 个 app，实际 %d 个：%v", n, n, len(ud), keysOf(ud))
	}
	for i := 0; i < n; i++ {
		id := "app" + itoa(i)
		if ud[id]["windows"] != "1.0.0" {
			t.Errorf("%s 丢失：%v", id, ud[id])
		}
	}
}

// 同一个 app 的并发确认：平台不能互相清掉
func TestConcurrentConfirmSameApp(t *testing.T) {
	home := t.TempDir()
	s := newTestServer(home)
	s.registerRoutes()

	var wg sync.WaitGroup
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			confirm(t, s, `{"app_id":"a","os`+itoa(i)+`":"1.0"}`)
		}(i)
	}
	wg.Wait()

	ud, err := store.LoadUserData(home)
	if err != nil {
		t.Fatal(err)
	}
	if len(ud["a"]) != 9 { // 8 个平台 + _confirmed_at
		t.Errorf("应保留 8 个平台 + _confirmed_at，实际 %d 个：%v", len(ud["a"]), ud["a"])
	}
}

// 读和写必须互斥：读到写了一半的文件会被误判成损坏而留档
// 写方的数据就跟着进了 .corrupt- 文件，正式路径反而空了
func TestReadDuringWriteDoesNotQuarantine(t *testing.T) {
	home := t.TempDir()
	if err := store.SaveUserData(home, store.UserData{"a": {"windows": "1.0"}}); err != nil {
		t.Fatal(err)
	}
	s := newTestServer(home)
	s.registerRoutes()

	var wg sync.WaitGroup
	wg.Add(1)
	go func() { // 持续确认，制造持续的写入窗口
		defer wg.Done()
		for i := 0; i < 60; i++ {
			confirm(t, s, `{"app_id":"a","windows":"1.0"}`)
		}
	}()
	for i := 0; i < 60; i++ {
		ud := s.loadUserData() // 读
		if ud["a"]["windows"] != "1.0" {
			t.Fatalf("读到不完整的数据: %v", ud)
		}
	}
	wg.Wait()

	entries, _ := readDirNames(home + "/user")
	for _, e := range entries {
		if strings.HasPrefix(e, "software.json.corrupt-") {
			t.Errorf("读写并发不应触发留档，却留下了 %s（目录：%v）", e, entries)
		}
	}
	ud, err := store.LoadUserData(home)
	if err != nil {
		t.Fatalf("数据应完好: %v", err)
	}
	if ud["a"]["windows"] != "1.0" {
		t.Errorf("数据应完好: %v", ud)
	}
}

// 响应体仍应返回确认后的版本
func TestConfirmResponseShape(t *testing.T) {
	home := t.TempDir()
	s := newTestServer(home)
	s.registerRoutes()

	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/check/confirm",
		strings.NewReader(`{"app_id":"a","windows":"9.9"}`))
	req.Header.Set("Content-Type", "application/json")
	s.mux.ServeHTTP(rr, req)

	var got struct {
		AppID     string            `json:"app_id"`
		Status    string            `json:"status"`
		Platforms map[string]string `json:"platforms"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if got.Platforms["windows"] != "9.9" {
		t.Errorf("响应应返回确认后的版本: %+v", got)
	}
	if _, ok := got.Platforms["_confirmed_at"]; !ok {
		t.Errorf("响应应带 _confirmed_at: %+v", got)
	}
}

func itoa(i int) string {
	b, _ := json.Marshal(i)
	return string(b)
}

func keysOf(ud store.UserData) []string {
	out := make([]string, 0, len(ud))
	for k := range ud {
		out = append(out, k)
	}
	return out
}

func readDirNames(dir string) ([]string, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}
	names := make([]string, 0, len(entries))
	for _, e := range entries {
		names = append(names, e.Name())
	}
	return names, nil
}
