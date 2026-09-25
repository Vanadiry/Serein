package log

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestSanitizingWriter(t *testing.T) {
	var buf bytes.Buffer
	w := sanitizingWriter{&buf}
	if _, err := w.Write([]byte("a\nb\r\n")); err != nil {
		t.Fatal(err)
	}
	if got := buf.String(); got != "a b \n" {
		t.Fatalf("sanitize = %q", got)
	}
}

func TestInitAndWrite(t *testing.T) {
	home := t.TempDir()
	logDir := filepath.Join(home, "logs")
	if err := os.MkdirAll(logDir, 0755); err != nil {
		t.Fatal(err)
	}
	// 预置超过上限的旧日志，验证轮转
	for i := 0; i < maxLogs+2; i++ {
		name := filepath.Join(logDir, "20200101_0000"+string(rune('a'+i))+".log")
		if err := os.WriteFile(name, []byte("old"), 0644); err != nil {
			t.Fatal(err)
		}
	}

	if err := InitLogger(home); err != nil {
		t.Fatal(err)
	}
	Logf("hello %d", 1)
	LogfWarn("warn %s", "x")
	LogfError("err")

	entries, err := os.ReadDir(logDir)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) > maxLogs {
		t.Fatalf("轮转后日志数 %d 超过上限 %d", len(entries), maxLogs)
	}

	found := false
	for _, e := range entries {
		data, _ := os.ReadFile(filepath.Join(logDir, e.Name()))
		s := string(data)
		if strings.Contains(s, "hello 1") {
			if !strings.Contains(s, "[INFO]") || !strings.Contains(s, "[WARN]") || !strings.Contains(s, "[ERROR]") {
				t.Fatalf("缺少日志级别前缀: %q", s)
			}
			found = true
		}
	}
	if !found {
		t.Fatal("未找到写入的日志内容")
	}
}
