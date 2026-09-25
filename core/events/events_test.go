package events

import (
	"encoding/json"
	"testing"
	"time"
)

func recvUntil(t *testing.T, ch chan []byte, msg string, d time.Duration) Event {
	t.Helper()
	deadline := time.After(d)
	for {
		select {
		case data := <-ch:
			var e Event
			if err := json.Unmarshal(data, &e); err != nil {
				t.Fatalf("json: %v", err)
			}
			if e.Message == msg {
				return e
			}
		case <-deadline:
			t.Fatalf("未收到消息 %q", msg)
		}
	}
}

func TestEmitSubscribe(t *testing.T) {
	ch := Subscribe()
	defer Unsubscribe(ch)
	Emit("warn", "ctx", "hello-test")
	e := recvUntil(t, ch, "hello-test", time.Second)
	if e.Level != "warn" || e.Context != "ctx" || e.ID == "" || e.TS == 0 {
		t.Fatalf("事件字段: %+v", e)
	}
}

func TestHistoryReplay(t *testing.T) {
	Emit("info", "ctx", "replay-test")
	ch := Subscribe()
	defer Unsubscribe(ch)
	recvUntil(t, ch, "replay-test", time.Second)
}

func TestUnsubscribeCloses(t *testing.T) {
	ch := Subscribe()
	Unsubscribe(ch)
	deadline := time.After(time.Second)
	for {
		select {
		case _, ok := <-ch:
			if !ok {
				return
			}
		case <-deadline:
			t.Fatal("取消订阅后通道应关闭")
		}
	}
}
