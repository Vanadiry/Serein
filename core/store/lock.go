// 跨进程文件锁：同一 SEREIN_HOME 下的多个实例（桌面壳 + go run）并发写 rules/ 时的互斥
// 为什么必须有：规则提交若是 rename 换入，临界区只有两次系统调用，接近原子
// 两个实例并发时会出现"一个失败、另一个赢"。改成按文件原地写之后，临界区
// 放大成 N 次写 + M 次删，可以交错出"文件已被 A 删除、marker 却由 B 写成
// 声称该文件已最新"的永久缺失，下次同步据此跳过，该规则文件再也不会回来
package store

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

// lockFileName 锁文件名，位于 home 下（与 rules/ 同级，不参与规则遍历）
const lockFileName = ".lock"

// FileLock 已持有的排他锁
type FileLock struct {
	f    *os.File
	path string
}

// AcquireFileLock 取得 home 上的排他锁
// 先立即尝试；已被占用则按 retry 间隔重试至 timeout。ctx 取消或超时即放弃
// 不会阻塞调用方（同步是后台任务，阻塞会让进度遮罩一直挂着且无任何提示）
// 锁文件不删除：删掉它等于放弃互斥。进程退出时 flock 由内核自动释放
func AcquireFileLock(ctx context.Context, home string, timeout, retry time.Duration) (*FileLock, error) {
	if retry <= 0 {
		retry = 100 * time.Millisecond
	}
	path := filepath.Join(home, lockFileName)
	f, err := os.OpenFile(path, os.O_RDWR|os.O_CREATE, 0600)
	if err != nil {
		return nil, fmt.Errorf("打开锁文件失败：%w", err)
	}
	deadline := time.Now().Add(timeout)
	for {
		locked, err := tryLockFile(f)
		if err != nil {
			f.Close()
			return nil, fmt.Errorf("加锁失败：%w", err)
		}
		if locked {
			return &FileLock{f: f, path: path}, nil
		}
		if ctx.Err() != nil {
			f.Close()
			return nil, ctx.Err()
		}
		if !time.Now().Before(deadline) {
			f.Close()
			return nil, fmt.Errorf("另一个 Serein 实例正在写入 %s，本次未做任何修改", home)
		}
		select {
		case <-ctx.Done():
			f.Close()
			return nil, ctx.Err()
		case <-time.After(retry):
		}
	}
}

// Release 释放锁。可重复调用
func (l *FileLock) Release() error {
	if l == nil || l.f == nil {
		return nil
	}
	err := unlockFile(l.f)
	if cerr := l.f.Close(); err == nil {
		err = cerr
	}
	l.f = nil
	return err
}

// Path 返回锁文件路径（诊断用）
func (l *FileLock) Path() string {
	if l == nil {
		return ""
	}
	return l.path
}
