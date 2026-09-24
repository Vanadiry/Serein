package httpx

import (
	"net/http"
	"testing"
)

func TestJoinURL(t *testing.T) {
	cases := []struct{ base, ref, want string }{
		{"https://a.com/x", "b.zip", "https://a.com/x/b.zip"},
		{"https://a.com/x/", "/b.zip", "https://a.com/x/b.zip"},
		{"https://a.com/x", "https://c.com/d", "https://c.com/d"},
		{"https://a.com/x", "//cdn.com/d", "https://cdn.com/d"},
		{"", "b", "b"},
		{"https://a.com/x", "", "https://a.com/x"},
	}
	for _, c := range cases {
		if got := JoinURL(c.base, c.ref); got != c.want {
			t.Errorf("JoinURL(%q, %q) = %q, want %q", c.base, c.ref, got, c.want)
		}
	}
}

func TestBlockPrivate(t *testing.T) {
	blocked := []string{
		"http://127.0.0.1/a",
		"http://10.0.0.1/",
		"http://192.168.1.1/",
		"http://169.254.1.1/",
		"http://[::1]/",
	}
	for _, u := range blocked {
		if err := BlockPrivate(u); err == nil {
			t.Errorf("%s 应被拦截", u)
		}
	}
	allowed := []string{"http://8.8.8.8/", "https://1.1.1.1/"}
	for _, u := range allowed {
		if err := BlockPrivate(u); err != nil {
			t.Errorf("%s 不应被拦截: %v", u, err)
		}
	}
	// 非 http(s) 不校验
	if err := BlockPrivate("ftp://127.0.0.1/"); err != nil {
		t.Errorf("非 http(s) 应返回 nil，得到 %v", err)
	}
}

func TestInjectAuth(t *testing.T) {
	h := http.Header{}
	if hasAuth(h) {
		t.Fatal("空 header 不应有 auth")
	}
	injectAuth(h, AuthTarget{Host: "api.github.com", Token: "t"})
	if got := h.Get("Authorization"); got != "Bearer t" {
		t.Fatalf("默认方案 = %q", got)
	}
	h2 := http.Header{}
	injectAuth(h2, AuthTarget{Scheme: "token", Token: "abc"})
	if got := h2.Get("Authorization"); got != "token abc" {
		t.Fatalf("自定义方案 = %q", got)
	}
}
