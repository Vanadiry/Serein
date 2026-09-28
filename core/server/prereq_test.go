package server

import (
	"context"
	"errors"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/vanadiry/serein/core/store"
)

// 写一条带 pre_request 的规则
func writePreRequestRule(t *testing.T, home, appID, pageURL string) {
	t.Helper()
	dir := filepath.Join(home, "rules", "test")
	if err := os.MkdirAll(dir, 0755); err != nil {
		t.Fatal(err)
	}
	body := `[info]
app_id = "` + appID + `"
name = "测试应用"
platforms = ["windows"]

[config]
type = "html_selector"
url = "` + pageURL + `"
position = { selector = "h1" }

[pre_request.page]
url = "https://example.com/redirect"
type = "html_selector"
position = { selector = "a", attr = "href" }
`
	if err := os.WriteFile(filepath.Join(dir, "r.toml"), []byte(body), 0644); err != nil {
		t.Fatal(err)
	}
}

func newPreReqServer(t *testing.T, home string) *Server {
	t.Helper()
	s := newTestServer(home)
	s.config.Tracker.Platforms = []string{"windows"}
	return s
}

// 预请求失败时绝不能拿规则里的原始 URL 去检查
// 那并非版本页，请求它多半拿到一个错误页，解析器会从里面抠出一个版本号
// （比如 "v2.1.0 not found" 里的 2.1.0）当成成功结果。用户点一下“确认这个更新”
// 错的值就写进 software.json，之后这个应用再也不会提示更新
func TestPreRequestFailureSkipsPlatform(t *testing.T) {
	home := t.TempDir()
	// 原始 URL 是一个语法合法的地址，错误页也能通过 HTTP 200 返回
	writePreRequestRule(t, home, "test.app", "https://example.com/fallback-not-a-real-page")

	s := newPreReqServer(t, home)
	s.runPreRequests = func(context.Context, []store.PreRequestStep, *http.Client) (string, error) {
		return "", errors.New("目标站点 503")
	}

	var errs []CheckError
	jobs, _ := s.buildCheckJobs(t.Context(),
		[]store.TrackerEntry{{AppID: "test.app"}},
		func(e CheckError) { errs = append(errs, e) })

	if len(jobs) != 0 {
		t.Errorf("预请求失败后不应产生任何 job（否则会拿原始 URL 去查），实际 %d 个: %+v", len(jobs), jobs)
	}
	// 错误必须上报，否则用户只看到“没检查”却不知道为什么
	if len(errs) == 0 {
		t.Fatal("应上报预请求失败")
	}
	joined := ""
	for _, e := range errs {
		joined += e.Message
	}
	if !strings.Contains(joined, "前置请求失败") {
		t.Errorf("错误信息应说明是前置请求失败: %s", joined)
	}
	if !strings.Contains(joined, "503") {
		t.Errorf("错误信息应带上游原因: %s", joined)
	}
}

// 预请求成功时照常替换 URL 并产生 job
func TestPreRequestSuccessStillChecks(t *testing.T) {
	home := t.TempDir()
	writePreRequestRule(t, home, "test.app", "https://example.com/fallback")

	s := newPreReqServer(t, home)
	s.runPreRequests = func(context.Context, []store.PreRequestStep, *http.Client) (string, error) {
		return "https://example.com/real-page", nil
	}

	var errs []CheckError
	jobs, _ := s.buildCheckJobs(t.Context(),
		[]store.TrackerEntry{{AppID: "test.app"}},
		func(e CheckError) { errs = append(errs, e) })

	if len(jobs) != 1 {
		t.Fatalf("预请求成功应产生 1 个 job，实际 %d 个（错误：%v）", len(jobs), errs)
	}
	// 替换后的 URL 必须真的进了 job，替换没生效的话查的还是原始地址
	found := false
	for _, pc := range jobs[0].req.Platforms {
		if strings.Contains(pc.URL, "/real-page") {
			found = true
		}
	}
	if !found {
		t.Errorf("预请求结果应替换 URL: %+v", jobs[0].req.Platforms)
	}
}

// 取消时直接停止构建，不该继续往下堆任务
func TestPreRequestCancelStopsBuilding(t *testing.T) {
	home := t.TempDir()
	writePreRequestRule(t, home, "test.app", "https://example.com/fallback")

	s := newPreReqServer(t, home)
	s.runPreRequests = func(context.Context, []store.PreRequestStep, *http.Client) (string, error) {
		return "", context.Canceled
	}

	jobs, _ := s.buildCheckJobs(t.Context(),
		[]store.TrackerEntry{{AppID: "test.app"}},
		func(CheckError) {})

	if len(jobs) != 0 {
		t.Errorf("取消后不应产生 job，实际 %d 个", len(jobs))
	}
}

// 没有 pre_request 的规则不受影响
func TestRuleWithoutPreRequestUnaffected(t *testing.T) {
	home := t.TempDir()
	dir := filepath.Join(home, "rules", "test")
	if err := os.MkdirAll(dir, 0755); err != nil {
		t.Fatal(err)
	}
	body := `[info]
app_id = "plain.app"
name = "普通应用"
platforms = ["windows"]

[config]
type = "html_selector"
url = "https://example.com/page"
position = { selector = "h1" }
`
	if err := os.WriteFile(filepath.Join(dir, "p.toml"), []byte(body), 0644); err != nil {
		t.Fatal(err)
	}

	s := newPreReqServer(t, home)
	s.runPreRequests = func(context.Context, []store.PreRequestStep, *http.Client) (string, error) {
		t.Error("没有 pre_request 就不该调用预请求")
		return "", errors.New("不该被调用")
	}

	jobs, _ := s.buildCheckJobs(t.Context(),
		[]store.TrackerEntry{{AppID: "plain.app"}},
		func(CheckError) {})

	if len(jobs) != 1 {
		t.Errorf("普通规则应正常产生 1 个 job，实际 %d 个", len(jobs))
	}
}
