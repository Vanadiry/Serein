//go:build windows

package store

import (
	"errors"
	"os"
	"syscall"
	"unsafe"
)

// Windows 无 flock，用 LockFileEx / UnlockFileEx 走 stdlib 的 LazyDLL
// 避免为此引入 golang.org/x/sys 依赖
var (
	kernel32         = syscall.NewLazyDLL("kernel32.dll")
	procLockFileEx   = kernel32.NewProc("LockFileEx")
	procUnlockFileEx = kernel32.NewProc("UnlockFileEx")
)

const (
	lockfileExclusiveLock   = 0x00000002
	lockfileFailImmediately = 0x00000001
	// 锁住整个文件：偏移 0、长度到文件末尾
	lockRangeLow  = 0
	lockRangeHigh = 0xFFFFFFFF
	// ERROR_LOCK_VIOLATION：别的进程已持有同区域排他锁
	errLockViolation = syscall.Errno(33)
	// ERROR_IO_PENDING：FAIL_IMMEDIATELY 下偶发的重试信号，同样按"被占用"处理
	errIOPending = syscall.Errno(997)
)

func tryLockFile(f *os.File) (bool, error) {
	ol := new(syscall.Overlapped)
	r1, _, err := procLockFileEx.Call(
		f.Fd(),
		uintptr(lockfileExclusiveLock|lockfileFailImmediately),
		0,
		uintptr(lockRangeLow),
		uintptr(lockRangeHigh),
		uintptr(unsafe.Pointer(ol)),
	)
	if r1 != 0 {
		return true, nil
	}
	if errors.Is(err, errLockViolation) || errors.Is(err, errIOPending) {
		return false, nil
	}
	if err == nil {
		err = errors.New("LockFileEx 调用失败但未返回错误")
	}
	return false, err
}

func unlockFile(f *os.File) error {
	ol := new(syscall.Overlapped)
	r1, _, err := procUnlockFileEx.Call(
		f.Fd(),
		0,
		uintptr(lockRangeLow),
		uintptr(lockRangeHigh),
		uintptr(unsafe.Pointer(ol)),
	)
	if r1 != 0 {
		return nil
	}
	if err == nil {
		return errors.New("UnlockFileEx 调用失败但未返回错误")
	}
	return err
}
