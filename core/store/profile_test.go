package store

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadProfile(t *testing.T) {
	home := t.TempDir()
	os.MkdirAll(filepath.Join(home, "user"), 0755)
	p := filepath.Join(home, "user", "profile.json")

	// 正常
	os.WriteFile(p, []byte(`{"version":1,"known_extensions":["dmg"]}`), 0644)
	got, err := LoadProfile(home)
	if err != nil {
		t.Fatal(err)
	}
	if got.Version != 1 || len(got.KnownExtensions) != 1 {
		t.Fatalf("profile=%+v", got)
	}

	// 损坏的 JSON → 应返回错误（不再静默吞掉）
	os.WriteFile(p, []byte(`{"version":1,`), 0644)
	if _, err := LoadProfile(home); err == nil {
		t.Fatal("损坏的 profile.json 应返回错误")
	}

	// 文件缺失 → 返回错误
	os.Remove(p)
	if _, err := LoadProfile(home); err == nil {
		t.Fatal("缺失的 profile.json 应返回错误")
	}
}
