package store

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestSaveLoadUserData(t *testing.T) {
	home := t.TempDir()
	os.MkdirAll(filepath.Join(home, "user"), 0755)

	ud := UserData{"a": {"macos": "1.0"}}
	if err := SaveUserData(home, ud); err != nil {
		t.Fatal(err)
	}
	got, err := LoadUserData(home)
	if err != nil {
		t.Fatal(err)
	}
	if got["a"]["macos"] != "1.0" {
		t.Fatalf("got=%+v", got)
	}

	// 覆盖写
	ud["a"]["macos"] = "2.0"
	if err := SaveUserData(home, ud); err != nil {
		t.Fatal(err)
	}
	got, _ = LoadUserData(home)
	if got["a"]["macos"] != "2.0" {
		t.Fatalf("覆盖写失败: %+v", got)
	}

	// 不应残留临时文件
	entries, _ := os.ReadDir(filepath.Join(home, "user"))
	for _, e := range entries {
		if strings.Contains(e.Name(), ".tmp") {
			t.Fatalf("残留临时文件: %s", e.Name())
		}
	}
}
