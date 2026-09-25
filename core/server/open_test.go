package server

import "testing"

func TestNormalizeURL(t *testing.T) {
	cases := map[string]string{
		"//cdn.com/x":        "https://cdn.com/x",
		"  https://a.com/x ": "https://a.com/x",
		"https://a.com/x":    "https://a.com/x",
		"":                   "",
	}
	for in, want := range cases {
		if got := normalizeURL(in); got != want {
			t.Errorf("normalizeURL(%q) = %q, want %q", in, got, want)
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
	bad := []string{"", "ftp://a.com", "a.com/x", "javascript:alert(1)"}
	for _, u := range bad {
		if _, err := parseHTTPURL(u); err == nil {
			t.Errorf("parseHTTPURL(%q) 应报错", u)
		}
	}
	// 协议相对地址应补上 https 并返回规范化结果
	if got, err := parseHTTPURL("//cdn.com/x"); err != nil || got != "https://cdn.com/x" {
		t.Fatalf("协议相对: %q %v", got, err)
	}
}
