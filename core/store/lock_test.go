package store

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestFileLockMutualExclusion(t *testing.T) {
	home := t.TempDir()
	ctx := context.Background()

	first, err := AcquireFileLock(ctx, home, time.Second, 10*time.Millisecond)
	if err != nil {
		t.Fatalf("首次加锁应成功: %v", err)
	}
	if first.Path() != filepath.Join(home, lockFileName) {
		t.Errorf("锁路径 = %q", first.Path())
	}

	// 已被占用：应在 timeout 内失败并给出可读原因，一直阻塞会让进度遮罩挂死
	start := time.Now()
	if _, err := AcquireFileLock(ctx, home, 200*time.Millisecond, 10*time.Millisecond); err == nil {
		t.Fatal("锁被占用时第二次加锁应失败")
	} else if !strings.Contains(err.Error(), "另一个 Serein 实例") {
		t.Errorf("错误信息应说明是另一个实例占用: %v", err)
	}
	if el := time.Since(start); el > 3*time.Second {
		t.Errorf("加锁超时耗时 %v，未按 timeout 返回", el)
	}

	// 释放后应能重新取得
	if err := first.Release(); err != nil {
		t.Fatalf("释放失败: %v", err)
	}
	again, err := AcquireFileLock(ctx, home, time.Second, 10*time.Millisecond)
	if err != nil {
		t.Fatalf("释放后应能重新加锁: %v", err)
	}
	if err := again.Release(); err != nil {
		t.Fatalf("释放失败: %v", err)
	}
	// 重复释放应安全
	if err := again.Release(); err != nil {
		t.Errorf("重复释放应无副作用: %v", err)
	}
}

func TestFileLockRespectsContext(t *testing.T) {
	home := t.TempDir()
	held, err := AcquireFileLock(context.Background(), home, time.Second, 10*time.Millisecond)
	if err != nil {
		t.Fatalf("加锁失败: %v", err)
	}
	defer held.Release()

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	// timeout 给足 5s，若实现忽略 ctx 这里会等满 5s 才返回
	_, err = AcquireFileLock(ctx, home, 5*time.Second, 10*time.Millisecond)
	if err == nil {
		t.Fatal("ctx 已取消时不应取得锁")
	}
}

// 锁文件必须在 home 下、且不被清理（删掉它等于放弃互斥）
func TestFileLockFileLocation(t *testing.T) {
	home := t.TempDir()
	rules := filepath.Join(home, "rules")
	if err := os.MkdirAll(rules, 0755); err != nil {
		t.Fatal(err)
	}
	l, err := AcquireFileLock(context.Background(), home, time.Second, 10*time.Millisecond)
	if err != nil {
		t.Fatal(err)
	}
	l.Release()

	lock := filepath.Join(home, lockFileName)
	if _, err := os.Stat(lock); err != nil {
		t.Fatalf("锁文件应存在于 home 下: %v", err)
	}
	if strings.HasPrefix(lock, rules+string(os.PathSeparator)) {
		t.Error("锁文件不得放在 rules/ 内，否则会被规则遍历当成源目录")
	}
	// 锁文件不应被 LoadAllTrackerInfo / SourceNames 之类的遍历当成规则或源
	if _, err := os.Stat(filepath.Join(lock)); err != nil {
		t.Errorf("释放后锁文件应保留: %v", err)
	}
}
