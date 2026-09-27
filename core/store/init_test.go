package store

import (
	"os"
	"path/filepath"
	"testing"
)

func modeOf(t *testing.T, path string) os.FileMode {
	t.Helper()
	fi, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	return fi.Mode().Perm()
}

// config.toml 里有 GitHub token，目录与文件都应仅当前用户可访问
func TestInitPermissions(t *testing.T) {
	if os.Getenv("GOOS") == "windows" {
		t.Skip("Windows 上权限位无效")
	}
	home := t.TempDir()
	// 模拟旧版本留下的宽权限
	if err := os.Chmod(home, 0755); err != nil {
		t.Fatal(err)
	}
	for _, sub := range []string{"rules", "tracker", "user", "logs"} {
		if err := os.MkdirAll(filepath.Join(home, sub), 0755); err != nil {
			t.Fatal(err)
		}
	}
	// 旧版创建的 config.toml 是 0644
	cfg := filepath.Join(home, "config.toml")
	if err := os.WriteFile(cfg, []byte(DefaultConfigTOML), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(cfg, 0644); err != nil {
		t.Fatal(err)
	}

	if err := Init(home); err != nil {
		t.Fatal(err)
	}

	if m := modeOf(t, home); m != 0700 {
		t.Errorf("home 权限 = %o, want 700", m)
	}
	for _, sub := range []string{"rules", "tracker", "user", "logs"} {
		p := filepath.Join(home, sub)
		if m := modeOf(t, p); m != 0700 {
			t.Errorf("%s 权限 = %o, want 700", sub, m)
		}
	}
	if m := modeOf(t, cfg); m != 0600 {
		t.Errorf("config.toml 权限 = %o, want 600", m)
	}
}

// 首次创建时也应是 0600
func TestInitConfigPermOnCreate(t *testing.T) {
	if os.Getenv("GOOS") == "windows" {
		t.Skip("Windows 上权限位无效")
	}
	home := t.TempDir()
	if err := Init(home); err != nil {
		t.Fatal(err)
	}
	cfg := filepath.Join(home, "config.toml")
	if m := modeOf(t, cfg); m != 0600 {
		t.Errorf("新建的 config.toml 权限 = %o, want 600", m)
	}
	// 内容仍是可用的默认配置，没被 chmod 破坏
	b, err := os.ReadFile(cfg)
	if err != nil || len(b) == 0 {
		t.Fatalf("config.toml 内容异常: %v", err)
	}
}
