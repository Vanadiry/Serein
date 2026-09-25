package checker

import (
	"testing"

	"github.com/vanadiry/serein/core/store"
)

func TestStripVersionAffixes(t *testing.T) {
	oldP, oldS := versionPrefixes, versionSuffixes
	t.Cleanup(func() { versionPrefixes, versionSuffixes = oldP, oldS })

	// 前缀按长度优先（ver 先于 v），后缀同理
	SetVersionPrefixes([]string{"v", "ver"})
	SetVersionSuffixes([]string{"a", "-beta"})

	cases := map[string]string{
		"v1.2.3":      "1.2.3",
		"ver1.2.3":    "1.2.3",
		"1.2.3-beta":  "1.2.3",
		"v1.2.3-beta": "1.2.3",
		"2.0.0":       "2.0.0",
		"":            "",
	}
	for in, want := range cases {
		if got := stripVersionAffixes(in); got != want {
			t.Errorf("stripVersionAffixes(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestResolveDirectURL(t *testing.T) {
	if got, err := resolveDirectURL("https://x/a.bin", "1.0"); err != nil || got != "https://x/a.bin" {
		t.Fatalf("无占位应原样返回: %q %v", got, err)
	}
	if got, err := resolveDirectURL("https://x/{version}/a.bin", "1.0"); err != nil || got != "https://x/1.0/a.bin" {
		t.Fatalf("替换失败: %q %v", got, err)
	}
	if _, err := resolveDirectURL("https://x/{version}/a.bin", ""); err == nil {
		t.Fatal("版本为空且含占位应报错")
	}
}

func TestExtractValue(t *testing.T) {
	// json 单路径
	if v, err := extractValue([]byte(`{"s":"x","n":12}`), "json", []any{"s"}, "", "", ""); err != nil || toString(v) != "x" {
		t.Fatalf("json: %v %v", v, err)
	}
	// json 多路径拼接
	pos := []any{[]any{"a"}, []any{"b"}}
	if v, err := extractValue([]byte(`{"a":"1","b":"2"}`), "json", pos, "-", "", ""); err != nil || v != "1-2" {
		t.Fatalf("json join: %v %v", v, err)
	}
	// xml + #text
	if v, err := extractValue([]byte(`<r><v>1.2</v></r>`), "xml", []any{"r", "v", "#text"}, "", "", ""); err != nil || v != "1.2" {
		t.Fatalf("xml: %v %v", v, err)
	}
	// regex
	if v, err := extractValue([]byte("ver=3.4.5"), "regex", `ver=([0-9.]+)`, "", "", ""); err != nil || v != "3.4.5" {
		t.Fatalf("regex: %v %v", v, err)
	}
	// html_selector + baseurl
	body := []byte(`<html><body><a class="dl" href="/files/x.bin">x</a></body></html>`)
	pos2 := map[string]any{"selector": ".dl", "attr": "href"}
	if v, err := extractValue(body, "html_selector", pos2, "", "https://h", ""); err != nil || v != "https://h/files/x.bin" {
		t.Fatalf("selector: %v %v", v, err)
	}
	// 未知类型 → nil, nil
	if v, err := extractValue(nil, "nope", nil, "", "", ""); err != nil || v != nil {
		t.Fatalf("未知类型: %v %v", v, err)
	}
}

func TestMatchRegexErrors(t *testing.T) {
	if _, err := matchRegex("abc", "a(b"); err == nil {
		t.Fatal("非法正则应报错")
	}
	if _, err := matchRegex("abc", "abc"); err == nil {
		t.Fatal("无捕获组应报错")
	}
}

func TestNewPlatformCheckConfig(t *testing.T) {
	pc := store.PlatConfig{
		Type: "json", URL: "https://x", DownloadName: "a-{version}.zip",
		DownloadViaProxy: true, AllowPrerelease: true, VPosition: []any{"v"},
	}
	got := NewPlatformCheckConfig("windows", "App", "0.9", pc)
	if got.OS != "windows" || got.Label != "App" || got.CurrentVersion != "0.9" {
		t.Fatalf("基本字段: %+v", got)
	}
	if !got.AllowPrerelease || !got.DownloadViaProxy || got.DownloadName != "a-{version}.zip" {
		t.Fatalf("透传字段: %+v", got)
	}
}
