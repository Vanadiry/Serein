package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestSereinHome(t *testing.T) {
	t.Setenv("SEREIN_HOME", "/tmp/custom-serein")
	if got := sereinHome(); got != "/tmp/custom-serein" {
		t.Fatalf("SEREIN_HOME = %q", got)
	}

	os.Unsetenv("SEREIN_HOME")
	dir, err := os.UserHomeDir()
	if err != nil {
		t.Skip("无法获取用户主目录")
	}
	want := filepath.Join(dir, ".vSoft", "Serein")
	if got := sereinHome(); got != want {
		t.Fatalf("默认 home = %q, want %q", got, want)
	}
}
