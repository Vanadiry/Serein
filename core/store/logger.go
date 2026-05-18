package store

import (
	"log"
	"os"
	"path/filepath"
	"sort"
	"sync"
	"time"
)

var (
	logMu     sync.Mutex
	logFile   *os.File
	logInited bool
)

// InitLogger 初始化日志，写入 HOME/logs/ 目录。
func InitLogger(home string) error {
	logMu.Lock()
	defer logMu.Unlock()

	if logInited {
		return nil
	}

	logDir := filepath.Join(home, "logs")
	if err := os.MkdirAll(logDir, 0755); err != nil {
		return err
	}

	files, _ := filepath.Glob(filepath.Join(logDir, "*.log"))
	sort.Sort(sort.Reverse(sort.StringSlice(files)))
	for i, f := range files {
		if i >= 9 {
			os.Remove(f)
		}
	}

	name := time.Now().Format("20060102_150405") + ".log"
	path := filepath.Join(logDir, name)
	f, err := os.Create(path)
	if err != nil {
		return err
	}

	multi := ioMultiWriter(os.Stdout, f)
	log.SetOutput(multi)
	log.SetFlags(log.LstdFlags)
	logFile = f
	logInited = true
	return nil
}

func ioMultiWriter(writers ...interface{ Write([]byte) (int, error) }) *multiWriter {
	return &multiWriter{writers: writers}
}

type multiWriter struct {
	writers []interface{ Write([]byte) (int, error) }
}

func (m *multiWriter) Write(p []byte) (int, error) {
	for _, w := range m.writers {
		w.Write(p)
	}
	return len(p), nil
}

// Logf 格式化日志
func Logf(format string, args ...any) {
	log.Printf(format, args...)
}
