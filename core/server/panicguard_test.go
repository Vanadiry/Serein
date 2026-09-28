package server

import (
	"context"
	"encoding/json"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/vanadiry/serein/core/checker"
	"github.com/vanadiry/serein/core/events"
	"github.com/vanadiry/serein/core/progress"
)

// goSafe 必须兜住 panic：net/http 的 recover 只覆盖"处理请求的那个 goroutine"
// handler 自己 spawn 出来的后台任务 panic 一次就是整个进程静默退出，连日志都没有
// 这个测试跑在测试进程里，panic 一旦逃出去整个测试就崩，所以它同时就是回归证明
func TestGoSafeRecovers(t *testing.T) {
	p := progress.NewProgress(1)
	done := make(chan struct{})
	goSafe("test", p, func() {
		defer close(done)
		panic("解析器炸了")
	})

	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("任务没结束：panic 逃出去了")
	}

	// 进度必须被关闭，否则前端一直转圈等一个永远不来的 done 事件
	// （done 事件是靠 p.Close() 发的）
	deadline := time.After(2 * time.Second)
	for {
		if r := p.Replay(); r != "" {
			if !strings.Contains(r, `"step":"done"`) {
				t.Errorf("结束事件应为 done: %s", r)
			}
			return
		}
		select {
		case <-deadline:
			t.Fatal("panic 后进度未关闭，前端会一直转圈")
		case <-time.After(20 * time.Millisecond):
		}
	}
}

func TestGoSafePassesThrough(t *testing.T) {
	p := progress.NewProgress(1)
	ran := false
	done := make(chan struct{})
	goSafe("test", p, func() {
		defer close(done)
		ran = true
	})
	<-done
	if !ran {
		t.Error("正常任务应原样执行")
	}
}

func TestGoSafeNilProgress(t *testing.T) {
	done := make(chan struct{})
	goSafe("test", nil, func() {
		defer close(done)
		panic("没有进度对象也要兜住")
	})
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("任务没结束")
	}
}

// panic 后要推事件，用户看到的应该是"任务异常终止"而非窗口静默消失
func TestGoSafeEmitsEvent(t *testing.T) {
	ch := events.Subscribe()
	defer events.Unsubscribe(ch)
	drainSubscribed(ch) // 订阅会回放历史，先排空

	goSafe("check", progress.NewProgress(1), func() { panic("炸了") })

	deadline := time.After(2 * time.Second)
	for {
		select {
		case data := <-ch:
			var evt struct {
				Level   string `json:"level"`
				Message string `json:"message"`
			}
			if err := json.Unmarshal(data, &evt); err != nil {
				t.Fatal(err)
			}
			if evt.Level == "error" && strings.Contains(evt.Message, "异常终止") {
				return
			}
		case <-deadline:
			t.Fatal("panic 后应推送 error 事件")
		case <-time.After(20 * time.Millisecond):
		}
	}
}

// runConcurrent 的 worker panic 不该带走同批次的其他条目
func TestRunConcurrentWorkerPanicIsolated(t *testing.T) {
	items := []int{1, 2, 3, 4}
	var mu sync.Mutex
	var errs []string

	got := runConcurrent(context.Background(), items, 4,
		func(_ context.Context, v int) (checker.CheckResponse, error) {
			if v == 2 {
				panic("这一条炸了")
			}
			return checker.CheckResponse{AppID: string(rune('0' + v))}, nil
		},
		func(v int, err error) {
			mu.Lock()
			errs = append(errs, err.Error())
			mu.Unlock()
		},
		func(int, checker.CheckResponse) {},
	)

	if len(got) != 3 {
		t.Errorf("其余 3 条应正常完成，实际 %d 条: %v", len(got), got)
	}
	mu.Lock()
	defer mu.Unlock()
	if len(errs) != 1 || !strings.Contains(errs[0], "panic") {
		t.Errorf("panic 的条目要记成错误，不能静默消失: %v", errs)
	}
}

func drainSubscribed(ch chan []byte) {
	timeout := time.After(100 * time.Millisecond)
	for {
		select {
		case <-ch:
		case <-timeout:
			return
		}
	}
}
