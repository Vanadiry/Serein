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
	if GetProgress(p.ID) != nil {
		t.Fatal("Close 后应从注册表移除")
	}

	// 幂等，且 Close 后 Send 不 panic
	p.Close()
	p.Send("app", "B", 2, 2)
}
