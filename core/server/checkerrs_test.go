package server

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"sync"
	"testing"

	"github.com/vanadiry/serein/core/checker"
	"github.com/vanadiry/serein/core/progress"
)

// 错误必须随 done 事件返回，不另开接口、不写入结果缓存
func TestCheckErrorsRideOnDoneEvent(t *testing.T) {
	ce := &checkErrs{}
	tracker := "v-github"
	report := ce.scoped(tracker)
	report(CheckError{Name: "整条目失败", Message: "boom"})
	// 结果里的 per-platform 错误与告警也要收进来
	recordResultErrors(ce, tracker, []checker.CheckResponse{{
		AppID: "HMCL", Name: "HMCL",
		Platforms: map[string]checker.CheckPlatform{
			"windows": {Error: "HTTP 500"},
			"macos":   {LatestVersion: "1.0", Warnings: []string{"未找到非预发布版本"}},
			"linux":   {LatestVersion: "1.0"},
		},
	}})

	p := progress.NewProgress(0)
	items, overflow := ce.snapshot()
	if items == nil {
		items = []CheckError{}
	}
	p.SetFinalEvent(map[string]any{"errors": items, "overflow": overflow})
	p.Close()

	var done map[string]any
	raw := p.Replay()
	if raw == "" {
		t.Fatal("Replay 应返回结束事件")
	}
	if err := json.Unmarshal([]byte(raw), &done); err != nil {
		t.Fatal(err)
	}
	if done["step"] != "done" {
		t.Fatalf("不是 done 事件: %v", done)
	}
	if _, ok := done["cancelled"]; !ok {
		t.Error("cancelled 应由 Close 一并写入")
	}
	errs, _ := done["errors"].([]any)
	if len(errs) != 3 {
		t.Fatalf("done 应带 3 条错误，实际 %d: %v", len(errs), errs)
	}
	// 每条都要能定位到来源：Tracker + app
	for _, it := range errs {
		m := it.(map[string]any)
		if m["tracker"] != tracker {
			t.Errorf("缺少 Tracker 归属: %v", m)
		}
		if m["message"] == "" {
			t.Errorf("缺少错误内容: %v", m)
		}
	}
	// 断线重连靠的就是这条，payload 必须带上错误列表
	if !strings.Contains(raw, "errors") {
		t.Error("错误列表不在结束事件里，重连就拿不到了")
	}
}

// Tracker 分组：scoped 绑定的名字不能被单条覆盖
func TestCheckErrsScopedTracker(t *testing.T) {
	ce := &checkErrs{}
	a := ce.scoped("Tracker A")
	b := ce.scoped("Tracker B")
	a(CheckError{Message: "1"})
	b(CheckError{Message: "2"})
	a(CheckError{Message: "3"})
	items, _ := ce.snapshot()
	if len(items) != 3 {
		t.Fatalf("应有 3 条: %+v", items)
	}
	got := map[string]int{}
	for _, e := range items {
		got[e.Tracker]++
	}
	if got["Tracker A"] != 2 || got["Tracker B"] != 1 {
		t.Errorf("分组错误: %v", got)
	}
}

// 四种 scope 共用同一个收集器：并发 add 不丢且不乱序
func TestCheckErrsConcurrent(t *testing.T) {
	ce := &checkErrs{}
	var wg sync.WaitGroup
	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			ce.add(CheckError{Message: "e", Tracker: "t"})
		}(i)
	}
	wg.Wait()
	items, _ := ce.snapshot()
	if len(items) != 50 {
		t.Errorf("并发下应收到 50 条，实际 %d", len(items))
	}
}

// 空消息不进列表；超上限只计数
func TestCheckErrsLimitAndEmpty(t *testing.T) {
	old := checkMaxErrors
	checkMaxErrors = 2
	t.Cleanup(func() { checkMaxErrors = old })

	ce := &checkErrs{}
	ce.add(CheckError{Message: ""})
	ce.add(CheckError{Message: "a"})
	ce.add(CheckError{Message: "b"})
	ce.add(CheckError{Message: "c"})
	items, overflow := ce.snapshot()
	if len(items) != 2 {
		t.Errorf("应截断到 2 条，实际 %d", len(items))
	}
	if overflow != 1 {
		t.Errorf("overflow = %d, want 1", overflow)
	}
	for _, e := range items {
		if e.Message == "" {
			t.Error("空消息不应进入列表")
		}
	}
}

// msvsix / openvsix 走 directCheckResponse，同样要判定“取到了没有”
func TestDirectCheckResponseRejectsEmpty(t *testing.T) {
	ok := func(ctx context.Context, appID string, c *http.Client) (checker.PlatformResult, error) {
		return checker.PlatformResult{LatestVersion: "1.0", URL: "https://x/1.0.vsix"}, nil
	}
	noVer := func(ctx context.Context, appID string, c *http.Client) (checker.PlatformResult, error) {
		return checker.PlatformResult{URL: "https://x/1.0.vsix"}, nil
	}
	noURL := func(ctx context.Context, appID string, c *http.Client) (checker.PlatformResult, error) {
		return checker.PlatformResult{LatestVersion: "1.0"}, nil
	}
	ctx := context.Background()
	c := &http.Client{}
	if _, err := directCheckResponse(ctx, ok, c, "a.b", "msvsix", nil); err != nil {
		t.Errorf("正常情况不该报错: %v", err)
	}
	if _, err := directCheckResponse(ctx, noVer, c, "a.b", "msvsix", nil); err == nil {
		t.Error("未能提取版本号应报错")
	}
	if _, err := directCheckResponse(ctx, noURL, c, "a.b", "openvsx", nil); err == nil {
		t.Error("未能提取下载链接应报错")
	}
}
