package httpx

import "testing"

// URLScheme 必须按 scheme 语法判断，不能用「第一个冒号」。
// 判据是与浏览器一致：new URL(raw, base).protocol 对同一字符串必须给出同一个协议名。
func TestURLScheme(t *testing.T) {
	cases := map[string]string{
		"https://example.com":    "https",
		"http://x/y":             "http",
		"javascript:alert(1)":    "javascript",
		"JaVaScRiPt:x":           "javascript",
		"data:text/html,x":       "data",
		"//example.com/x":        "",
		"/rules?a=b":             "",
		"relative/path":          "",
		"https://x/?a=b:c":       "https", // 第二个冒号在查询串里，不影响
		"example.com:8080/x":     "example.com",
		"?x=javascript:alert(1)": "",
		"":                       "",
	}
	for in, want := range cases {
		if got := URLScheme(in); got != want {
			t.Errorf("URLScheme(%q) = %q, want %q", in, got, want)
		}
	}
}

// ResolveScheme 输出恒可交给 http.Client；已有协议原样保留（javascript: 之类仍交给上层拒绝）
func TestResolveScheme(t *testing.T) {
	cases := map[string]string{
		"//cdn.example.com/f.zip": "https://cdn.example.com/f.zip",
		"cdn.example.com/f.zip":   "https://cdn.example.com/f.zip",
		"www.a.com/x":             "https://www.a.com/x",
		"  https://a.com/x ":      "https://a.com/x",
		"https://a.com/x":         "https://a.com/x",
		"http://a.com/x":          "http://a.com/x",
		"javascript:alert(1)":     "javascript:alert(1)", // 不补，仍能被上层拒掉
		"data:text/html,x":        "data:text/html,x",
		"":                        "",
	}
	for in, want := range cases {
		if got := ResolveScheme(in); got != want {
			t.Errorf("ResolveScheme(%q) = %q, want %q", in, got, want)
		}
	}
}
