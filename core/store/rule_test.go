package store

import (
	"os"
	"path/filepath"
	"testing"
)

func TestPlatformConfigMerge(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "m.toml")
	body := `[info]
app_id = "m"
name = "M"
platforms = ["windows", "macos"]

[config]
type = "json"
url = "https://shared"
allow_prerelease = true

[config.windows]
url = "https://win"
allow_prerelease = false

[config.macos]
type = "xml"
`
	if err := os.WriteFile(p, []byte(body), 0644); err != nil {
		t.Fatal(err)
	}
	rule, issues, err := ParseRuleFile(p, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(issues) != 0 {
		t.Fatalf("不应有问题: %+v", issues)
	}

	// windows：url/allow_prerelease 覆盖，type 继承
	win := rule.MergedConfig("windows")
	if win.URL != "https://win" || win.Type != "json" || win.AllowPrerelease {
		t.Fatalf("windows 合并错误: %+v", win)
	}
	// macos：type 覆盖，其余继承（含 presence 覆盖：false 需生效）
	mac := rule.MergedConfig("macos")
	if mac.URL != "https://shared" || mac.Type != "xml" || !mac.AllowPrerelease {
		t.Fatalf("macos 合并错误: %+v", mac)
	}
	// 未定义的平台回落到共享配置
	other := rule.MergedConfig("linux")
	if other.URL != "https://shared" || !other.AllowPrerelease {
		t.Fatalf("回落共享配置错误: %+v", other)
	}
}

func TestParseRuleStatus(t *testing.T) {
	if st, bad := ParseRuleStatus(nil); st.Message != "" || st.Level != "" || bad {
		t.Fatalf("空: %+v %v", st, bad)
	}
	if st, bad := ParseRuleStatus([]string{"维护中"}); st.Message != "维护中" || st.Level != "" || bad {
		t.Fatalf("仅消息: %+v %v", st, bad)
	}
	if st, bad := ParseRuleStatus([]string{"停更", "Removed"}); st.Level != "removed" || bad {
		t.Fatalf("等级应小写解析: %+v %v", st, bad)
	}
	if st, bad := ParseRuleStatus([]string{"x", "nope"}); st.Level != "warn" || !bad {
		t.Fatalf("未知等级应按 warn: %+v %v", st, bad)
	}
}

func TestValidTrackerName(t *testing.T) {
	ok := []string{"a", "my-tracker", "a.b_1", "默认"}
	for _, n := range ok {
		if !ValidTrackerName(n) {
			t.Errorf("%q 应合法", n)
		}
	}
	bad := []string{"", ".", "..", "a/b", `a\b`, "/abs"}
	for _, n := range bad {
		if ValidTrackerName(n) {
			t.Errorf("%q 应非法", n)
		}
	}
}

func TestPlatformsFor(t *testing.T) {
	entry := TrackerEntry{AppID: "a", Platforms: []string{"windows"}}
	if got := PlatformsFor(entry, []string{"macos"}); len(got) != 1 || got[0] != "windows" {
		t.Fatalf("条目平台应优先: %v", got)
	}
	entry.Platforms = nil
	if got := PlatformsFor(entry, []string{"macos"}); len(got) != 1 || got[0] != "macos" {
		t.Fatalf("无条目平台应用配置平台: %v", got)
	}
}

func TestSourceNames(t *testing.T) {
	home := t.TempDir()
	rules := filepath.Join(home, "rules")
	write := func(rel, body string) {
		p := filepath.Join(rules, rel)
		if err := os.MkdirAll(filepath.Dir(p), 0755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(body), 0644); err != nil {
			t.Fatal(err)
		}
	}
	write("s1/_source.json", `{"source_id":"S1","name":"源一","type":"rules","files":["A.toml"]}`)
	write("s2/_source.json", `{"source_id":"S2","name":"源二"}`)
	write("s3/_source.json", `{"source_id":"S3","name":"列表","type":"list","files":[]}`)

	got := SourceNames(home)
	if got["S1"] != "源一" || got["S2"] != "源二" {
		t.Fatalf("SourceNames = %+v", got)
	}
	if _, ok := got["S3"]; ok {
		t.Fatalf("type=list 应被排除: %+v", got)
	}

	// findNearestSourceID 向上查找最近的 _source.json 目录名
	write("s1/sub/B.toml", "")
	if id := findNearestSourceID(rules, filepath.Join(rules, "s1", "sub", "B.toml")); id != "s1" {
		t.Fatalf("findNearestSourceID = %q, want s1", id)
	}
}
