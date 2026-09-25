package server

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/vanadiry/serein/core/checker"
	"github.com/vanadiry/serein/core/progress"
	"github.com/vanadiry/serein/core/store"
)

func newTestServer(home string) *Server {
	return &Server{
		home:    home,
		config:  store.Config{},
		mux:     http.NewServeMux(),
		results: map[string]map[string]checker.CheckResponse{},
	}
}

func TestFormatSourceURL(t *testing.T) {
	cases := map[string]string{
		"https://raw.githubusercontent.com/Vanadiry/SereinRulesList/main/_source.json": "Serein 官方源",
		"https://github.com/foo/bar/x":                     "foo/bar (GitHub)",
		"https://raw.githubusercontent.com/foo/bar/main/x": "foo/bar (GitHub Raw)",
		"https://example.com/x":                            "https://example.com/x",
	}
	for in, want := range cases {
		if got := formatSourceURL(in); got != want {
			t.Errorf("formatSourceURL(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestHandleConfig(t *testing.T) {
	cases := []struct{ dl, want string }{
		{"", "浏览器"},
		{"browser", "浏览器"},
		{"ndm", "Neat Download Manager"},
		{"aria2c {url}", "自定义命令"},
		{"aria2c", "无（未识别）"},
	}
	for _, c := range cases {
		s := newTestServer(t.TempDir())
		s.config.Download.Downloader = c.dl
		rec := httptest.NewRecorder()
		s.handleConfig(rec, httptest.NewRequest("GET", "/api/config", nil))
		if rec.Code != http.StatusOK {
			t.Fatalf("code=%d body=%s", rec.Code, rec.Body)
		}
		var out struct {
			Config   map[string]string `json:"config"`
			FirstRun bool              `json:"first_run"`
		}
		if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
			t.Fatal(err)
		}
		if out.Config["下载器"] != c.want {
			t.Errorf("downloader=%q 下载器=%q, want %q", c.dl, out.Config["下载器"], c.want)
		}
		if !out.FirstRun {
			t.Errorf("默认 first_run 应为 true")
		}
	}
}

func TestHandleRulesEmpty(t *testing.T) {
	home := t.TempDir()
	os.MkdirAll(filepath.Join(home, "rules"), 0755)
	s := newTestServer(home)
	rec := httptest.NewRecorder()
	s.handleRules(rec, httptest.NewRequest("GET", "/api/rules", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("code=%d body=%s", rec.Code, rec.Body)
	}
	if got := strings.TrimSpace(rec.Body.String()); got != "[]" {
		t.Fatalf("空规则源应为 []，得到 %q", got)
	}
}

func TestFormatRuleList(t *testing.T) {
	rules := []store.Rule{
		{Info: store.RuleInfo{AppID: "b", Name: "Beta", Platforms: []string{"macos"}}, SourceID: "S1"},
		{Info: store.RuleInfo{AppID: "a", Name: "alpha", Platforms: []string{"windows"}}, SourceID: "S2"},
	}
	out := formatRuleList(t.TempDir(), rules)
	if len(out) != 2 || out[0].Name != "alpha" || out[1].Name != "Beta" {
		t.Fatalf("应按名称排序: %+v", out)
	}
	if out[1].Name != "Beta" {
		t.Fatal("大小写不敏感排序失败")
	}
}

func TestHandleRulesCheck(t *testing.T) {
	home := t.TempDir()
	os.MkdirAll(filepath.Join(home, "rules"), 0755)
	s := newTestServer(home)

	// 未配置 dev 源 → 400
	rec := httptest.NewRecorder()
	s.handleRulesCheck(rec, httptest.NewRequest("POST", "/api/rules/check", strings.NewReader(`{"dev":true}`)))
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("未配置 dev 源应 400，得到 %d", rec.Code)
	}

	// 普通检查 → 200
	rec2 := httptest.NewRecorder()
	s.handleRulesCheck(rec2, httptest.NewRequest("POST", "/api/rules/check", strings.NewReader(`{}`)))
	if rec2.Code != http.StatusOK {
		t.Fatalf("普通检查应 200，得到 %d body=%s", rec2.Code, rec2.Body)
	}
}

func TestCheckResultsCache(t *testing.T) {
	s := newTestServer(t.TempDir())
	s.setCheckResult("k", checker.CheckResponse{AppID: "x", Platforms: map[string]checker.CheckPlatform{"macos": {LatestVersion: "1"}}})
	got := s.getCheckResults("k")
	if len(got) != 1 || got[0].AppID != "x" {
		t.Fatalf("set/get = %+v", got)
	}
	s.setCheckResults("k", []checker.CheckResponse{{AppID: "y"}})
	got = s.getCheckResults("k")
	if len(got) != 1 || got[0].AppID != "y" {
		t.Fatalf("setCheckResults 应整桶替换: %+v", got)
	}
	if len(s.getCheckResults("nope")) != 0 {
		t.Fatal("不存在的桶应为空")
	}
}

func TestHandleCheckTemp(t *testing.T) {
	s := newTestServer(t.TempDir())
	s.setCheckResults("t1", []checker.CheckResponse{{AppID: "a"}})

	rec := httptest.NewRecorder()
	r := httptest.NewRequest("GET", "/api/check/temp/t1", nil)
	r.SetPathValue("tracker_id", "t1")
	s.handleCheckTemp(rec, r)
	if rec.Code != http.StatusOK {
		t.Fatalf("code=%d", rec.Code)
	}
	var out struct {
		Results []checker.CheckResponse `json:"results"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
		t.Fatal(err)
	}
	if len(out.Results) != 1 || out.Results[0].AppID != "a" {
		t.Fatalf("results=%+v", out.Results)
	}

	rec2 := httptest.NewRecorder()
	r2 := httptest.NewRequest("GET", "/api/check/temp/a/b", nil)
	r2.SetPathValue("tracker_id", "a/b")
	s.handleCheckTemp(rec2, r2)
	if rec2.Code != http.StatusBadRequest {
		t.Fatalf("含斜杠的 tracker_id 应 400，得到 %d", rec2.Code)
	}
}

func TestBuildCheckList(t *testing.T) {
	home := t.TempDir()
	os.MkdirAll(filepath.Join(home, "tracker"), 0755)
	body := "display_name = \"Apps\"\n[[tracker]]\napp_id = \"a\"\n[[tracker]]\napp_id = \"b\"\n"
	if err := os.WriteFile(filepath.Join(home, "tracker", "apps.toml"), []byte(body), 0644); err != nil {
		t.Fatal(err)
	}
	s := newTestServer(home)

	list := s.buildCheckList(nil, nil)
	if len(list) != 1 || list[0].id != "apps" || len(list[0].entries) != 2 {
		t.Fatalf("全量列表: %+v", list)
	}

	list = s.buildCheckList(nil, map[string][]string{"apps": {"a"}})
	if len(list) != 1 || len(list[0].entries) != 1 || list[0].entries[0].AppID != "a" {
		t.Fatalf("按 id 过滤: %+v", list)
	}

	if l := s.buildCheckList([]string{"nope"}, nil); len(l) != 0 {
		t.Fatalf("不存在的 tracker 应为空: %+v", l)
	}
}

func TestHandleCheckConfirm(t *testing.T) {
	home := t.TempDir()
	os.MkdirAll(filepath.Join(home, "user"), 0755)
	s := newTestServer(home)

	rec := httptest.NewRecorder()
	s.handleCheckConfirm(rec, httptest.NewRequest("POST", "/api/check/confirm", strings.NewReader(`{"app_id":"a","macos":"1.2"}`)))
	if rec.Code != http.StatusOK {
		t.Fatalf("code=%d body=%s", rec.Code, rec.Body)
	}
	ud, err := store.LoadUserData(home)
	if err != nil {
		t.Fatal(err)
	}
	if ud["a"]["macos"] != "1.2" || ud["a"]["_confirmed_at"] == "" {
		t.Fatalf("user data = %+v", ud)
	}

	rec2 := httptest.NewRecorder()
	s.handleCheckConfirm(rec2, httptest.NewRequest("POST", "/api/check/confirm", strings.NewReader(`{}`)))
	if rec2.Code != http.StatusBadRequest {
		t.Fatalf("缺 app_id 应 400，得到 %d", rec2.Code)
	}
}

func TestSameOriginGuard(t *testing.T) {
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(http.StatusOK) })
	h := sameOriginGuard(next)

	serve := func(method, host string, hdr map[string]string) int {
		r := httptest.NewRequest(method, "http://"+host+"/x", nil)
		for k, v := range hdr {
			r.Header.Set(k, v)
		}
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, r)
		return rec.Code
	}

	if code := serve("GET", "host", map[string]string{"Sec-Fetch-Site": "cross-site"}); code != http.StatusOK {
		t.Fatalf("GET 不应被拦截: %d", code)
	}
	if code := serve("POST", "host", map[string]string{"Sec-Fetch-Site": "cross-site"}); code != http.StatusForbidden {
		t.Fatalf("跨站 POST 应 403: %d", code)
	}
	if code := serve("POST", "host", map[string]string{"Origin": "http://evil.com"}); code != http.StatusForbidden {
		t.Fatalf("来源不匹配应 403: %d", code)
	}
	if code := serve("POST", "host", map[string]string{"Origin": "http://host"}); code != http.StatusOK {
		t.Fatalf("同源 POST 应放行: %d", code)
	}
}

func TestWithCORS(t *testing.T) {
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(http.StatusOK) })
	h := withCORS()(next)

	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest("OPTIONS", "http://host/x", nil))
	if rec.Code != http.StatusNoContent {
		t.Fatalf("OPTIONS 应 204，得到 %d", rec.Code)
	}
	if got := rec.Header().Get("Access-Control-Allow-Origin"); got != "http://host" {
		t.Fatalf("Allow-Origin = %q", got)
	}
}

func TestServeAppJS(t *testing.T) {
	webFS := fstest.MapFS{
		"assets/app.min.js": &fstest.MapFile{Data: []byte(`var a="__DL__";var b="__DL_TYPE__";`)},
	}
	s := &Server{webFS: webFS, config: store.Config{}}
	s.config.Download.Downloader = "ndm"

	rec := httptest.NewRecorder()
	if !s.serveAppJS(rec, "/assets/app.min.js") {
		t.Fatal("应处理 app.min.js")
	}
	body := rec.Body.String()
	if !strings.Contains(body, `"Neat Download Manager（内建）"`) || !strings.Contains(body, `"ndm"`) {
		t.Fatalf("注入结果 = %q", body)
	}
	if s.serveAppJS(httptest.NewRecorder(), "/other.js") {
		t.Fatal("其它路径不应处理")
	}
}

func TestServeIndexWithFirstRun(t *testing.T) {
	webFS := fstest.MapFS{"index.html": &fstest.MapFile{Data: []byte("first=__FIRST_RUN__")}}
	s := &Server{webFS: webFS, config: store.Config{}}

	rec := httptest.NewRecorder()
	if !s.serveIndexWithFirstRun(rec, "/") {
		t.Fatal("应处理 /")
	}
	if got := rec.Body.String(); got != "first=true" {
		t.Fatalf("默认 = %q", got)
	}

	f := false
	s.config.Serein.FirstRun = &f
	rec2 := httptest.NewRecorder()
	s.serveIndexWithFirstRun(rec2, "/index.html")
	if got := rec2.Body.String(); got != "first=false" {
		t.Fatalf("关闭后 = %q", got)
	}
}

func TestServeSSE(t *testing.T) {
	ch := make(chan string, 1)
	ch <- "hello"
	close(ch)
	rec := httptest.NewRecorder()
	serveSSE(rec, httptest.NewRequest("GET", "/x", nil), ch)
	if got := rec.Header().Get("Content-Type"); got != "text/event-stream" {
		t.Fatalf("Content-Type = %q", got)
	}
	if !strings.Contains(rec.Body.String(), "data: hello\n\n") {
		t.Fatalf("body = %q", rec.Body.String())
	}
}

func TestHandleProgressCancel(t *testing.T) {
	p := progress.NewProgress(1)
	defer p.Close()

	rec := httptest.NewRecorder()
	r := httptest.NewRequest("POST", "/api/check/cancel/"+p.ID, nil)
	r.SetPathValue("task_id", p.ID)
	handleProgressCancel(rec, r)
	if rec.Code != http.StatusOK {
		t.Fatalf("code=%d", rec.Code)
	}
	if p.Context().Err() == nil {
		t.Fatal("应取消任务")
	}

	rec2 := httptest.NewRecorder()
	r2 := httptest.NewRequest("POST", "/api/check/cancel/nope", nil)
	r2.SetPathValue("task_id", "nope")
	handleProgressCancel(rec2, r2)
	if rec2.Code != http.StatusNotFound {
		t.Fatalf("不存在应 404，得到 %d", rec2.Code)
	}
}

func TestHandleTrackerNew(t *testing.T) {
	home := t.TempDir()
	os.MkdirAll(filepath.Join(home, "tracker"), 0755)
	s := newTestServer(home)

	rec := httptest.NewRecorder()
	s.handleTrackerNew(rec, httptest.NewRequest("POST", "/api/tracker/new", strings.NewReader(`{"id":"mine"}`)))
	if rec.Code != http.StatusCreated {
		t.Fatalf("新建应 201，得到 %d body=%s", rec.Code, rec.Body)
	}

	rec2 := httptest.NewRecorder()
	s.handleTrackerNew(rec2, httptest.NewRequest("POST", "/api/tracker/new", strings.NewReader(`{"id":"mine"}`)))
	if rec2.Code != http.StatusConflict {
		t.Fatalf("重复应 409，得到 %d", rec2.Code)
	}

	rec3 := httptest.NewRecorder()
	s.handleTrackerNew(rec3, httptest.NewRequest("POST", "/api/tracker/new", strings.NewReader(`{"id":"bad/name"}`)))
	if rec3.Code != http.StatusBadRequest {
		t.Fatalf("非法 id 应 400，得到 %d", rec3.Code)
	}
}

func TestHandleTrackerAddAndApps(t *testing.T) {
	home := t.TempDir()
	s := newTestServer(home)

	rec := httptest.NewRecorder()
	s.handleTrackerAdd(rec, httptest.NewRequest("POST", "/api/tracker/add", strings.NewReader(`{"app_id":"a","platforms":["macos"]}`)))
	if rec.Code != http.StatusOK {
		t.Fatalf("code=%d body=%s", rec.Code, rec.Body)
	}
	entries, err := store.LoadTrackerFile(home, store.DefaultTrackerID)
	if err != nil || len(entries) != 1 || entries[0].AppID != "a" {
		t.Fatalf("entries=%+v err=%v", entries, err)
	}

	rec2 := httptest.NewRecorder()
	s.handleTrackerAdd(rec2, httptest.NewRequest("POST", "/api/tracker/add", strings.NewReader(`{}`)))
	if rec2.Code != http.StatusBadRequest {
		t.Fatalf("缺 app_id 应 400，得到 %d", rec2.Code)
	}

	rec3 := httptest.NewRecorder()
	s.handleTrackerApps(rec3, httptest.NewRequest("GET", "/api/tracker/apps", nil))
	var apps []string
	if err := json.Unmarshal(rec3.Body.Bytes(), &apps); err != nil {
		t.Fatal(err)
	}
	if len(apps) != 1 || apps[0] != "a" {
		t.Fatalf("apps=%v", apps)
	}
}

func TestHandleTrackerListAll(t *testing.T) {
	home := t.TempDir()
	os.MkdirAll(filepath.Join(home, "tracker"), 0755)
	body := "display_name = \"Apps\"\n[[tracker]]\napp_id = \"a\"\n"
	if err := os.WriteFile(filepath.Join(home, "tracker", "apps.toml"), []byte(body), 0644); err != nil {
		t.Fatal(err)
	}
	s := newTestServer(home)
	s.setCheckResults("apps", []checker.CheckResponse{{
		AppID: "a",
		Platforms: map[string]checker.CheckPlatform{
			"macos": {LatestVersion: "2", CurrentVersion: "1"},
		},
	}})

	rec := httptest.NewRecorder()
	s.handleTrackerListAll(rec, httptest.NewRequest("GET", "/api/tracker/list/all", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("code=%d", rec.Code)
	}
	var out []map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
		t.Fatal(err)
	}
	if len(out) != 1 {
		t.Fatalf("out=%v", out)
	}
	if out[0]["checked"] != true || out[0]["updated"] != float64(1) {
		t.Fatalf("checked/updated 错误: %v", out[0])
	}
}
