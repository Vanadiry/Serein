package httpx

import (
	"context"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"
	"testing"
	"time"
)

func TestCheckStatus(t *testing.T) {
	ok := &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(""))}
	if err := CheckStatus(ok); err != nil {
		t.Fatalf("200 不应报错: %v", err)
	}
	bad := &http.Response{StatusCode: 404, Body: io.NopCloser(strings.NewReader("nope"))}
	err := CheckStatus(bad)
	if err == nil || !strings.Contains(err.Error(), "404") || !strings.Contains(err.Error(), "nope") {
		t.Fatalf("404 错误信息 = %v", err)
	}
}

func TestRequestKey(t *testing.T) {
	mk := func(ua string, h map[string]string) *http.Request {
		r, _ := http.NewRequest("GET", "https://a.com/x", nil)
		r.Header.Set("User-Agent", ua)
		for k, v := range h {
			r.Header.Set(k, v)
		}
		return r
	}
	k1 := requestKey(mk("A", map[string]string{"X": "1"}))
	k2 := requestKey(mk("A", map[string]string{"X": "1"}))
	if k1 != k2 {
		t.Fatal("相同请求的 key 应一致")
	}
	if requestKey(mk("B", map[string]string{"X": "1"})) == k1 {
		t.Fatal("不同 UA 的 key 应不同")
	}
	if requestKey(mk("A", map[string]string{"X": "2"})) == k1 {
		t.Fatal("不同 header 的 key 应不同")
	}
}

func TestClientTimeouts(t *testing.T) {
	if got := NewClient().Timeout; got != 30*time.Second {
		t.Fatalf("NewClient timeout = %v", got)
	}
	if got := StreamClient().Timeout; got != 0 {
		t.Fatalf("StreamClient timeout = %v", got)
	}
}

func TestIsAbsoluteURL(t *testing.T) {
	if !isAbsoluteURL("https://a.com/x") {
		t.Error("绝对地址应识别")
	}
	if isAbsoluteURL("/x") || isAbsoluteURL("b.zip") {
		t.Error("相对地址不应识别为绝对")
	}
}

func TestIsBlockedIP(t *testing.T) {
	for _, s := range []string{"127.0.0.1", "10.0.0.1", "192.168.1.1", "::1", "169.254.1.1"} {
		if !isBlockedIP(net.ParseIP(s)) {
			t.Errorf("%s 应被拦截", s)
		}
	}
	if !isBlockedIP(nil) {
		t.Error("nil IP 应被拦截")
	}
	if isBlockedIP(net.ParseIP("8.8.8.8")) {
		t.Error("8.8.8.8 不应被拦截")
	}
}

func TestResolveAndCheck(t *testing.T) {
	if _, err := resolveAndCheck("127.0.0.1"); err == nil {
		t.Fatal("回环地址应报错")
	}
	ips, err := resolveAndCheck("8.8.8.8")
	if err != nil || len(ips) != 1 {
		t.Fatalf("公网 IP 应通过: %v %v", ips, err)
	}
}

func TestRequestBlockedPrivate(t *testing.T) {
	if _, err := Request(context.Background(), NewClient(), "http://127.0.0.1/x", "", nil); err == nil {
		t.Fatal("私网地址应被 Request 拦截")
	}
	if _, err := PostRequest(context.Background(), NewClient(), "http://127.0.0.1/x", "", nil, nil); err == nil {
		t.Fatal("私网地址应被 PostRequest 拦截")
	}
}

type captureRT struct{ hdr http.Header }

func (c *captureRT) RoundTrip(req *http.Request) (*http.Response, error) {
	c.hdr = req.Header.Clone()
	return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader("")), Header: http.Header{}}, nil
}

func TestAuthTransport(t *testing.T) {
	cap := &captureRT{}
	tr := &authTransport{base: cap, targets: []AuthTarget{{Host: "api.github.com", Token: "T"}}}

	req, _ := http.NewRequest("GET", "https://api.github.com/x", nil)
	if _, err := tr.RoundTrip(req); err != nil {
		t.Fatal(err)
	}
	if got := cap.hdr.Get("Authorization"); got != "Bearer T" {
		t.Fatalf("注入 = %q", got)
	}

	req2, _ := http.NewRequest("GET", "https://other.com/x", nil)
	if _, err := tr.RoundTrip(req2); err != nil {
		t.Fatal(err)
	}
	if cap.hdr.Get("Authorization") != "" {
		t.Fatal("非目标 host 不应注入")
	}

	req3, _ := http.NewRequest("GET", "https://api.github.com/x", nil)
	req3.Header.Set("Authorization", "token existing")
	if _, err := tr.RoundTrip(req3); err != nil {
		t.Fatal(err)
	}
	if got := cap.hdr.Get("Authorization"); got != "token existing" {
		t.Fatalf("已有认证不应覆盖: %q", got)
	}
}

func TestSetProxy(t *testing.T) {
	old := proxyURL
	t.Cleanup(func() { proxyURL = old })

	SetProxy(&url.URL{Scheme: "http", Host: "127.0.0.1:7890"})
	if newTransport().Proxy == nil {
		t.Fatal("设置代理后 transport 应有 Proxy")
	}
	SetProxy(nil)
	if newTransport().Proxy != nil {
		t.Fatal("nil 代理不应启用 Proxy")
	}
}
