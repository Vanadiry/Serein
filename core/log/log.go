// 日志：三级输出 + 按大小/数量轮转
package log

import (
	"io"
	stdlog "log"
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

// sanitizingWriter 把日志内容里的换行/回车替换为空格（保留结尾换行），防止伪造日志行
type sanitizingWriter struct{ w io.Writer }

func (s sanitizingWriter) Write(p []byte) (int, error) {
	b := make([]byte, len(p))
	for i, c := range p {
		if (c == '\n' || c == '\r') && !(c == '\n' && i == len(p)-1) {
			b[i] = ' '
		} else {
			b[i] = c
		}
	}
	if _, err := s.w.Write(b); err != nil {
		return 0, err
	}
	return len(p), nil
}

var (
	logMu       sync.Mutex
	logDir      string
	infoLogger  *stdlog.Logger
	warnLogger  *stdlog.Logger
	errorLogger *stdlog.Logger
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
	// 0700：日志里可能有下载 URL，限制为仅当前用户可访问
	if err := os.MkdirAll(logDir, 0700); err != nil {
		return err
	}
	// 已存在时 MkdirAll 不改权限，补一次
	if err := os.Chmod(logDir, 0700); err != nil {
		return err
	}

	rotateIfNeeded()

	name := time.Now().Format("20060102_150405") + ".log"
	path := filepath.Join(logDir, name)
	f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0600)
	if err != nil {
		return err
	}

	multi := sanitizingWriter{io.MultiWriter(os.Stdout, f)}
	infoLogger = stdlog.New(multi, "[INFO]  ", stdlog.LstdFlags)
	warnLogger = stdlog.New(multi, "[WARN]  ", stdlog.LstdFlags)
	errorLogger = stdlog.New(multi, "[ERROR] ", stdlog.LstdFlags)
	logFile = f
	logInited = true
	return nil
}

// CloseLogger 关闭日志文件并复位
// Windows 上文件被打开时无法删除，测试的 TempDir 清理会失败
func CloseLogger() {
	logMu.Lock()
	defer logMu.Unlock()
	if !logInited {
		return
	}
	// 先置空，Close 之后再有日志调用会因 infoLogger 为 nil 直接返回
	infoLogger, warnLogger, errorLogger = nil, nil, nil
	if logFile != nil {
		logFile.Close()
		logFile = nil
	}
	logInited = false
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

// rotateOnSize 在日志超过上限时切换新文件
func rotateOnSize() {
	if logFile == nil {
		return
	}
	fi, err := logFile.Stat()
	if err != nil || fi.Size() < maxLogSize {
		return
	}

	name := time.Now().Format("20060102_150405") + ".log"
	f, err := os.OpenFile(filepath.Join(logDir, name), os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0600)
	if err != nil {
		return
	}

	logFile.Close()
	logFile = f
	rotateIfNeeded()

	multi := sanitizingWriter{io.MultiWriter(os.Stdout, f)}
	infoLogger.SetOutput(multi)
	warnLogger.SetOutput(multi)
	errorLogger.SetOutput(multi)
}

// Logf 输出 info 级日志。判空与写入都在锁内，避免与 InitLogger/rotateOnSize 竞争
func Logf(format string, args ...any) {
	logMu.Lock()
	defer logMu.Unlock()
	if infoLogger == nil {
		return
	}
	infoLogger.Printf(format, args...)
	rotateOnSize()
}

func LogfWarn(format string, args ...any) {
	logMu.Lock()
	defer logMu.Unlock()
	if warnLogger == nil {
		return
	}
	warnLogger.Printf(format, args...)
	rotateOnSize()
}

func LogfError(format string, args ...any) {
	logMu.Lock()
	defer logMu.Unlock()
	if errorLogger == nil {
		return
	}
	errorLogger.Printf(format, args...)
	rotateOnSize()
}
