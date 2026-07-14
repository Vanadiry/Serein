package store

import (
	"io"
	"log"
	"os"
	"path/filepath"
	"sort"
	"sync"
	"time"
)

const (
	maxLogSize = 10 * 1024 * 1024
	maxLogs    = 10
)

var (
	logMu       sync.Mutex
	logDir      string
	infoLogger  *log.Logger
	warnLogger  *log.Logger
	errorLogger *log.Logger
	logFile     *os.File
	logInited   bool
)

func InitLogger(home string) error {
	logMu.Lock()
	defer logMu.Unlock()

	if logInited {
		return nil
	}

	logDir = filepath.Join(home, "logs")
	if err := os.MkdirAll(logDir, 0755); err != nil {
		return err
	}

	rotateIfNeeded()

	name := time.Now().Format("20060102_150405") + ".log"
	path := filepath.Join(logDir, name)
	f, err := os.Create(path)
	if err != nil {
		return err
	}

	multi := io.MultiWriter(os.Stdout, f)
	infoLogger = log.New(multi, "[INFO]  ", log.LstdFlags)
	warnLogger = log.New(multi, "[WARN]  ", log.LstdFlags)
	errorLogger = log.New(multi, "[ERROR] ", log.LstdFlags)
	logFile = f
	logInited = true
	return nil
}

func rotateIfNeeded() {
	files, _ := filepath.Glob(filepath.Join(logDir, "*.log"))
	sort.Sort(sort.Reverse(sort.StringSlice(files)))
	for i, f := range files {
		if i >= maxLogs-1 {
			os.Remove(f)
		}
	}
}

func rotateOnSize() {
	if logFile == nil {
		return
	}
	fi, err := logFile.Stat()
	if err != nil || fi.Size() < maxLogSize {
		return
	}
	logFile.Close()

	name := time.Now().Format("20060102_150405") + ".log"
	path := filepath.Join(logDir, name)
	f, err := os.Create(path)
	if err != nil {
		return
	}

	rotateIfNeeded()

	multi := io.MultiWriter(os.Stdout, f)
	infoLogger.SetOutput(multi)
	warnLogger.SetOutput(multi)
	errorLogger.SetOutput(multi)
	infoLogger.SetFlags(log.LstdFlags)
	warnLogger.SetFlags(log.LstdFlags)
	errorLogger.SetFlags(log.LstdFlags)
	logFile = f
}

func Logf(format string, args ...any) {
	logfInfo(format, args...)
}

func LogfInfo(format string, args ...any) {
	logfInfo(format, args...)
}

func LogfWarn(format string, args ...any) {
	logfWarn(format, args...)
}

func LogfError(format string, args ...any) {
	logfError(format, args...)
}

func logfInfo(format string, args ...any) {
	if infoLogger == nil {
		return
	}
	logMu.Lock()
	defer logMu.Unlock()
	infoLogger.Printf(format, args...)
	rotateOnSize()
}

func logfWarn(format string, args ...any) {
	if warnLogger == nil {
		return
	}
	logMu.Lock()
	defer logMu.Unlock()
	warnLogger.Printf(format, args...)
	rotateOnSize()
}

func logfError(format string, args ...any) {
	if errorLogger == nil {
		return
	}
	logMu.Lock()
	defer logMu.Unlock()
	errorLogger.Printf(format, args...)
	rotateOnSize()
}
