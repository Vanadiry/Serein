package server

import "testing"

func TestDownloaderKind(t *testing.T) {
	cases := map[string]string{
		"":             dlBrowser,
		"  browser ":   dlBrowser,
		"ndm":          dlNDM,
		"aria2c {url}": dlCustom,
		"aria2c":       dlUnknown,
	}
	for in, want := range cases {
		if got := downloaderKindOf(in); got != want {
			t.Errorf("downloaderKindOf(%q) = %q, want %q", in, got, want)
		}
	}

	// 未知值：类型按浏览器处理、描述统一「无（未识别）」
	if got := parseDownloaderType("aria2c"); got != "browser" {
		t.Errorf("unknown type = %q, want browser", got)
	}
	if got := parseDownloaderDesc("aria2c"); got != `"无（未识别）"` {
		t.Errorf("unknown desc = %q", got)
	}
	if got := parseDownloaderDesc(""); got != `"浏览器"` {
		t.Errorf("empty desc = %q", got)
	}
}
