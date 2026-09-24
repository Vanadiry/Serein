package server

import (
	"net/http"
	"net/url"
	"testing"
)

func TestProxySignVerify(t *testing.T) {
	secret, _ := newProxySecret()
	s := &Server{proxySecret: secret}

	u := s.proxyURL("https://example.com/a/vspackage", "pub.ext-1.2.3.vsix")
	pu, err := url.Parse(u)
	if err != nil {
		t.Fatal(err)
	}
	q := pu.Query()
	if q.Get("name") != "pub.ext-1.2.3.vsix" {
		t.Fatalf("name = %q", q.Get("name"))
	}
	if !s.verifyProxy(q.Get("url"), q.Get("name"), q.Get("exp"), q.Get("sig")) {
		t.Fatal("自签 URL 应通过校验")
	}
	if s.verifyProxy(q.Get("url"), "tampered.vsix", q.Get("exp"), q.Get("sig")) {
		t.Fatal("篡改 name 应失败")
	}
	if s.verifyProxy(q.Get("url")+"x", q.Get("name"), q.Get("exp"), q.Get("sig")) {
		t.Fatal("篡改 url 应失败")
	}
	if s.verifyProxy(q.Get("url"), q.Get("name"), "1", q.Get("sig")) {
		t.Fatal("过期应失败")
	}
}

func TestSanitizeFilename(t *testing.T) {
	cases := map[string]string{
		`a/b\c:d*e?f"g<h>i|j`: "a_b_c_d_e_f_g_h_i_j",
		"  ..  ":              "",
		"normal-1.2.3.zip":    "normal-1.2.3.zip",
	}
	for in, want := range cases {
		if got := sanitizeFilename(in); got != want {
			t.Errorf("sanitizeFilename(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestFilenameFromResponse(t *testing.T) {
	resp := &http.Response{Header: http.Header{}}
	resp.Header.Set("Content-Disposition", `attachment; filename="up.bin"`)
	if got := filenameFromResponse(resp, "https://x/y/vspackage"); got != "up.bin" {
		t.Fatalf("按 Content-Disposition = %q", got)
	}
	empty := &http.Response{Header: http.Header{}}
	if got := filenameFromResponse(empty, "https://x/y/foo.dmg"); got != "foo.dmg" {
		t.Fatalf("回退 URL 末段 = %q", got)
	}
}
