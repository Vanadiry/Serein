package store

import (
	"os"
	"path/filepath"
	"testing"
)

func writeTrackerFile(t *testing.T, home, name, body string) {
	t.Helper()
	os.MkdirAll(filepath.Join(home, "tracker"), 0755)
	if err := os.WriteFile(filepath.Join(home, "tracker", name+".toml"), []byte(body), 0644); err != nil {
		t.Fatal(err)
	}
}

func TestVsixTrackerNewFormat(t *testing.T) {
	home := t.TempDir()
	writeTrackerFile(t, home, "vsix", `display_name = "VSIX"
type = "msvsix"

[tracker]
app_id = [
    "a.b",
    "c.d",
]
`)
	entries, err := LoadTrackerFile(home, "vsix")
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 2 || entries[0].AppID != "a.b" {
		t.Fatalf("entries=%+v", entries)
	}
	infos, _ := LoadAllTrackerInfo(home)
	if len(infos) != 1 || infos[0].Type != "msvsix" || infos[0].Count != 2 {
		t.Fatalf("infos=%+v", infos)
	}
}

func TestAppTrackerFormat(t *testing.T) {
	home := t.TempDir()
	writeTrackerFile(t, home, "apps", `display_name = "App"
[[tracker]]
app_id = "a"
platforms = ["macos"]
[[tracker]]
app_id = "b"
`)
	entries, err := LoadTrackerFile(home, "apps")
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 2 || entries[0].AppID != "a" || entries[0].Platforms[0] != "macos" {
		t.Fatalf("entries=%+v", entries)
	}
}

func TestVsixLegacyFormatSkipped(t *testing.T) {
	home := t.TempDir()
	writeTrackerFile(t, home, "old", `type = "msvsix"
[[tracker]]
app_id = "a.b"
`)
	// 不再兼容旧格式：解析失败 → 该文件被跳过
	if entries, _ := LoadTrackerFile(home, "old"); len(entries) != 0 {
		t.Fatalf("旧格式应被跳过，得到 %+v", entries)
	}
}

func TestAddToDefaultTracker(t *testing.T) {
	home := t.TempDir()
	if err := AddToTracker(home, DefaultTrackerID, TrackerEntry{AppID: "x", Platforms: []string{"macos"}}); err != nil {
		t.Fatal(err)
	}
	// 自动创建默认 Tracker，且带 display_name
	infos, _ := LoadAllTrackerInfo(home)
	if len(infos) != 1 || infos[0].ID != DefaultTrackerID || infos[0].Count != 1 {
		t.Fatalf("infos=%+v", infos)
	}
	if infos[0].DisplayName == "" || infos[0].DisplayName == DefaultTrackerID {
		t.Fatalf("默认 Tracker 应有 display_name，得到 %q", infos[0].DisplayName)
	}

	// 同 app_id 再添加 → 合并平台，不新增条目
	if err := AddToTracker(home, DefaultTrackerID, TrackerEntry{AppID: "x", Platforms: []string{"macos", "windows"}}); err != nil {
		t.Fatal(err)
	}
	entries, _ := LoadTrackerFile(home, DefaultTrackerID)
	if len(entries) != 1 || len(entries[0].Platforms) != 2 {
		t.Fatalf("平台应合并去重，得到 %+v", entries)
	}
}
