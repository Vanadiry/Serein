package progress

import (
	"encoding/json"
	"testing"
	"time"
)

func readEvent(t *testing.T, ch chan string) map[string]any {
	t.Helper()
	select {
	case s := <-ch:
		var m map[string]any
		if err := json.Unmarshal([]byte(s), &m); err != nil {
			t.Fatalf("json: %v (%q)", err, s)
		}
		return m
	case <-time.After(time.Second):
		t.Fatal("等待进度事件超时")
		return nil
	}
}

func TestProgressLifecycle(t *testing.T) {
	p := NewProgress(2)
	if p.ID == "" || GetProgress(p.ID) != p {
		t.Fatal("应注册并可查询")
	}
	if p.Context().Err() != nil {
		t.Fatal("初始 context 不应取消")
	}

	start := readEvent(t, p.Channel)
	if start["step"] != "start" || start["total"] != float64(2) {
		t.Fatalf("start = %v", start)
	}

	p.Send("app", "A", 1, 2)
	ev := readEvent(t, p.Channel)
	if ev["step"] != "app" || ev["name"] != "A" || ev["done"] != float64(1) {
		t.Fatalf("app = %v", ev)
	}
	if p.Done != 1 || p.Name != "A" {
		t.Fatalf("状态未更新: %+v", p)
	}

	p.Cancel()
	if p.Context().Err() == nil {
		t.Fatal("取消后 context 应结束")
	}

	p.Close()
	done := readEvent(t, p.Channel)
	if done["step"] != "done" || done["cancelled"] != true {
		t.Fatalf("done = %v", done)
	}
	if _, ok := <-p.Channel; ok {
		t.Fatal("Close 后通道应关闭")
	}
	// 条目不立即移除：迟到的订阅者要能拿到快照（见 progressRetain）
	if GetProgress(p.ID) != p {
		t.Fatal("Close 后条目应暂时保留，供迟到的订阅者补发")
	}
	if r := p.Replay(); r == "" {
		t.Fatal("已结束的任务应能补发最后一条事件")
	}

	// 幂等，且 Close 后 Send 不 panic
	p.Close()
	p.Send("app", "B", 2, 2)
}

func TestProgressRemovedAfterRetain(t *testing.T) {
	old := progressRetain
	progressRetain = 10 * time.Millisecond
	t.Cleanup(func() { progressRetain = old })

	p := NewProgress(1)
	p.Close()
	for i := 0; i < 100; i++ {
		if GetProgress(p.ID) == nil {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("超过保留时长后应从注册表移除")
}

// 核心回归：缓冲区被塞满时 Close 仍必须把 done 送达
// 丢���的后果是前端永远等不到 done，进度遮罩永久卡住
func TestCloseDeliversDoneWhenBufferFull(t *testing.T) {
	p := NewProgress(0)
	// 不消费，把 64 槽缓冲区填满
	for i := 0; i < 200; i++ {
		p.Send("file", "f", i, 200)
	}
	p.SetFinalEvent(map[string]any{"files": 200})
	p.Close()

	// 通道已关闭，但缓冲里必须能读到 done
	var got map[string]any
	for s := range p.Channel {
		var m map[string]any
		if json.Unmarshal([]byte(s), &m) != nil {
			t.Fatalf("非法事件 %q", s)
		}
		if m["step"] == "done" {
			got = m
		}
	}
	if got == nil {
		t.Fatal("缓冲区满时 done 被丢弃了")
	}
	if got["files"] != float64(200) {
		t.Errorf("done 载荷丢失: %v", got)
	}
}

// 进行中的任务不补发：客户端会从缓冲区拿到后续所有帧
// 补发会让同一帧被消费两次（进度计数翻倍）
func TestReplayEmptyWhileRunning(t *testing.T) {
	p := NewProgress(3)
	if r := p.Replay(); r != "" {
		t.Errorf("未结束的任务不应补发: %q", r)
	}
	p.Close()
	if r := p.Replay(); r == "" {
		t.Error("已结束的任务应能补发")
	}
}
