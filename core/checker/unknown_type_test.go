package checker

import (
	"context"
	"net/http"
	"strings"
	"testing"
)

// 核心回归：未知提取类型不得产出字面量 "<nil>" 作为版本号。
// 旧实现 extractValue 的 default 分支返回 (nil, nil)，调用方 toString(nil)
// 得到 "<nil>"，被当成合法版本号写进 CheckPlatform，并最终持久化进
// user/software.json。全程无任何错误。
func TestUnknownTypeNeverYieldsLiteralNil(t *testing.T) {
	// URL 指向一个必然失败的地址：前置校验必须早于任何网络请求，
	// 否则拿到的是网络错误而不是「未知类型」，真实原因被掩盖。
	cfg := PlatformCheckConfig{
		OS:             "windows",
		Type:           "jsno", // 拼写错误
		URL:            "http://127.0.0.1:1/x.json",
		VPosition:      []any{"version"},
		DPosition:      []any{"url"},
		DownloadName:   "x",
		CurrentVersion: "1.0.0",
	}
	pr, err := RunPlatformCheck(context.Background(), cfg, &http.Client{})
	if err == nil {
		t.Fatalf("未知类型应报错，实际 nil（版本=%q 链接=%v）", pr.LatestVersion, pr.URL)
	}
	if strings.Contains(pr.LatestVersion, "<nil>") {
		t.Fatalf("版本号里出现了字面量 <nil>: %q", pr.LatestVersion)
	}
	if strings.Contains(err.Error(), "<nil>") {
		t.Errorf("错误信息本身含 <nil>: %v", err)
	}
	if !strings.Contains(err.Error(), "jsno") {
		t.Errorf("错误信息应指出出错的类型: %v", err)
	}
	t.Logf("拒绝: %v", err)

	// 经 newCheckPlatform 组装后，Error 字段必须被填上（而不是记成检查成功）
	cp := newCheckPlatform(cfg, pr, err)
	if cp.Error == "" {
		t.Error("cp.Error 为空，该平台会被记成检查成功")
	}
	if strings.Contains(cp.Error, "<nil>") || strings.Contains(cp.LatestVersion, "<nil>") {
		t.Errorf("组装结果含字面量 <nil>: %+v", cp)
	}
	if cp.LatestVersion != "" {
		t.Errorf("出错时不应带版本号: %q", cp.LatestVersion)
	}
}

// toString(nil) 兜底：即使将来有路径返回 (nil, nil)，也不能产出 "<nil>"
func TestToStringNilIsEmpty(t *testing.T) {
	if got := toString(nil); got != "" {
		t.Fatalf("toString(nil) = %q, want 空串", got)
	}
	if strings.Contains(toString(nil), "nil") {
		t.Error("toString(nil) 不应包含 nil 字样")
	}
	// 正常值不受影响
	if got := toString("1.2.3"); got != "1.2.3" {
		t.Errorf("toString 正常值受影响: %q", got)
	}
	if got := toString(map[string]any{"#text": "v"}); got != "v" {
		t.Errorf("toString #text 受影响: %q", got)
	}
	if got := toString(12); got != "12" {
		t.Errorf("toString 数字受影响: %q", got)
	}
}

// github / direct 误入 extractValue 时必须报错，而不是静默返回 nil
func TestExtractValueRejectsNonExtractableTypes(t *testing.T) {
	for _, typ := range []string{"github", "direct"} {
		if _, err := extractValue(nil, typ, nil, "", ""); err == nil {
			t.Errorf("%s 应被拒绝", typ)
		}
	}
}

// 直通模式不补协议：//host 与无协议地址原样透出，交给浏览器 / 外部下载器处理。
// 真正需要绝对地址的只有 Go 的 http.Client，由取数前显式做（httpx.ResolveScheme）。
func TestResolveDirectURLPassesSchemeThrough(t *testing.T) {
	cases := []struct{ durl, version, want string }{
		{"//cdn.example.com/f.zip", "", "//cdn.example.com/f.zip"},
		{"cdn.example.com/f.zip", "", "cdn.example.com/f.zip"},
		{"https://cdn.example.com/f.zip", "", "https://cdn.example.com/f.zip"},
		{"//cdn.example.com/{version}.zip", "1.2.3", "//cdn.example.com/1.2.3.zip"},
		{"cdn.example.com/{version}.zip", "1.2.3", "cdn.example.com/1.2.3.zip"},
		{"https://cdn.example.com/{version}.zip", "1.2.3", "https://cdn.example.com/1.2.3.zip"},
	}
	for _, c := range cases {
		got, err := resolveDirectURL(c.durl, c.version)
		if err != nil {
			t.Errorf("resolveDirectURL(%q, %q) 报错: %v", c.durl, c.version, err)
			continue
		}
		if got != c.want {
			t.Errorf("resolveDirectURL(%q, %q) = %q, want %q（不应补协议）", c.durl, c.version, got, c.want)
		}
	}
	// {version} 但版本为空仍要报错
	if _, err := resolveDirectURL("cdn.example.com/{version}.zip", ""); err == nil {
		t.Error("版本为空时应报错")
	}
}
