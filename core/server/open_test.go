package server

import "testing"

// 校验层只确认 host 可用，不改写协议。协议相对地址原样放行，因为消费方是
// 浏览器与外部下载器，两者都认 //host
// 由调用方在取数前显式补，见 httpx.ResolveScheme
func TestParseHTTPURL(t *testing.T) {
	pass := []struct{ in, want string }{
		{"https://a.com/x", "https://a.com/x"},
		{"http://a.com/x", "http://a.com/x"},
		{"https://a.com", "https://a.com"},
		{"  https://a.com/x ", "https://a.com/x"}, // 空白被裁掉
		{"//cdn.com/x", "//cdn.com/x"},
		{"//cdn.com/x?a=b:c", "//cdn.com/x?a=b:c"},
		{"//cdn.example.com/f.zip", "//cdn.example.com/f.zip"},
		{"//abc.com/", "//abc.com/"},
		{"//a.com/x?y=z", "//a.com/x?y=z"},
	}
	for _, c := range pass {
		got, err := parseHTTPURL(c.in)
		if err != nil {
			t.Errorf("parseHTTPURL(%q) 应通过: %v", c.in, err)
		} else if got != c.want {
			t.Errorf("parseHTTPURL(%q) = %q, want %q", c.in, got, c.want)
		}
	}

	// 有协议但非 http(s) 必须仍被拒绝，不能借“补前缀”一并放行
	// 空与只剩斜杠同理，没有 host 可校验
	reject := []string{
		"", "   ", "//", "///",
		"ftp://a.com",
		"javascript:alert(1)",
		"data:text/html,x",
		"vbscript:msgbox(1)",
		"file:///etc/passwd",
	}
	for _, in := range reject {
		if got, err := parseHTTPURL(in); err == nil {
			t.Errorf("parseHTTPURL(%q) 应拒绝，得到 %q", in, got)
		}
	}
}
