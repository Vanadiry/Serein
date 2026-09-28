package server

import "testing"

// 校验层不改写输入：协议相对地址与无协议地址都原样放行
// 消费方是浏览器与外部下载器，两者都认 //host
// 需要绝对地址的只有 Go 的 http.Client，由调用方在取数前显式做，见 httpx.ResolveScheme
func TestParseHTTPURLPassesThroughUnchanged(t *testing.T) {
	cases := map[string]string{
		"//cdn.com/x":        "//cdn.com/x",
		"//cdn.com/x?a=b:c":  "//cdn.com/x?a=b:c",
		"  https://a.com/x ": "https://a.com/x",
		"https://a.com/x":    "https://a.com/x",
		"http://a.com/x":     "http://a.com/x",
	}
	for in, want := range cases {
		got, err := parseHTTPURL(in)
		if err != nil {
			t.Errorf("parseHTTPURL(%q) 应通过: %v", in, err)
			continue
		}
		if got != want {
			t.Errorf("parseHTTPURL(%q) = %q, want %q（校验层不应改写输入）", in, got, want)
		}
	}
	// 空与只剩斜杠仍要拒绝
	for _, in := range []string{"", "   ", "//", "///"} {
		if got, err := parseHTTPURL(in); err == nil {
			t.Errorf("parseHTTPURL(%q) 应报错，得到 %q", in, got)
		}
	}
}

// 有协议但非 http(s) 的必须仍被拒绝，不能借“补前缀”一并放行
func TestParseHTTPURLStillRejectsBadScheme(t *testing.T) {
	for _, u := range []string{
		"javascript:alert(1)",
		"data:text/html,x",
		"vbscript:msgbox(1)",
		"file:///etc/passwd",
	} {
		if got, err := parseHTTPURL(u); err == nil {
			t.Errorf("parseHTTPURL(%q) 应拒绝，得到 %q", u, got)
		}
	}
}

func TestParseHTTPURL(t *testing.T) {
	ok := []string{"http://a.com/x", "https://a.com", "//cdn.com/x"}
	for _, u := range ok {
		if _, err := parseHTTPURL(u); err != nil {
			t.Errorf("parseHTTPURL(%q) 应通过: %v", u, err)
		}
	}
	// "a.com/x"与"//cdn.com/x"曾在此处被列为非法。现按“校验层不改写
	// 协议相对原样放行”处理；完整用例见 TestParseHTTPURLPassesThroughUnchanged
	bad := []string{"", "ftp://a.com", "javascript:alert(1)"}
	for _, u := range bad {
		if _, err := parseHTTPURL(u); err == nil {
			t.Errorf("parseHTTPURL(%q) 应报错", u)
		}
	}
	// 协议相对地址原样返回，不补协议
	if got, err := parseHTTPURL("//cdn.com/x"); err != nil || got != "//cdn.com/x" {
		t.Fatalf("协议相对应原样放行: %q %v", got, err)
	}
}

// Serein 不解析协议相对地址：校验层原样放行，取数层也不改写
// 交给浏览器 / 下载器处理
func TestParseHTTPURLLeavesProtocolRelativeUntouched(t *testing.T) {
	for _, in := range []string{"//cdn.example.com/f.zip", "//abc.com/", "//a.com/x?y=z"} {
		got, err := parseHTTPURL(in)
		if err != nil {
			t.Errorf("parseHTTPURL(%q) 应通过: %v", in, err)
			continue
		}
		if got != in {
			t.Errorf("parseHTTPURL(%q) = %q，应原样返回", in, got)
		}
	}
}
