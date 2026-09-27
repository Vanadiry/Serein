package progress

import (
	"encoding/json"
	"testing"
)

// 结束事件只能有一条。调用方自行 SendMap("done") 会与 Close 发的收尾 done 重复，
// 客户端只取到其中一条——而重复的那条若载荷丢失只剩 {"step":"done"}，
// 前端会把它渲染成「同步完成」，把失败静默吞掉。
func TestCloseSendsExactlyOneDone(t *testing.T) {
	p := NewProgress(0)
	p.Send("app", "a", 1, 2)
	p.Close()
	p.Close() // 幂等

	var dones []map[string]any
	for line := range p.Channel {
		var m map[string]any
		if err := json.Unmarshal([]byte(line), &m); err != nil {
			t.Fatalf("事件不是合法 JSON: %q", line)
		}
		if m["step"] == "done" {
			dones = append(dones, m)
		}
	}
	if len(dones) != 1 {
		t.Fatalf("done 事件 = %d 条, want 1: %v", len(dones), dones)
	}
}

// SetFinalEvent 的载荷必须出现在 done 事件里，且 step/cancelled 由 Close 统一写入
func TestSetFinalEventPayload(t *testing.T) {
	p := NewProgress(0)
	payload := map[string]any{"sources_updated": 3, "sources_failed": 1}
	p.SetFinalEvent(payload)
	// 调用方之后仍持有该 map：Close 复制后再写 step/cancelled，不应回写调用方的 map
	p.Close()

	if _, ok := payload["step"]; ok {
		t.Error("Close 不应把 step 写回调用方持有的 map")
	}
	if _, ok := payload["cancelled"]; ok {
		t.Error("Close 不应把 cancelled 写回调用方持有的 map")
	}

	var done map[string]any
	for line := range p.Channel {
		var m map[string]any
		if json.Unmarshal([]byte(line), &m) == nil && m["step"] == "done" {
			done = m
		}
	}
	if done == nil {
		t.Fatal("未收到 done 事件")
	}
	if done["sources_updated"] != float64(3) || done["sources_failed"] != float64(1) {
		t.Errorf("载荷未透传: %v", done)
	}
	if done["cancelled"] != false {
		t.Errorf("cancelled = %v, want false", done["cancelled"])
	}
}

func TestSetFinalEventCancelled(t *testing.T) {
	p := NewProgress(0)
	p.SetFinalEvent(map[string]any{"files": 7})
	p.Cancel()
	p.Close()

	for line := range p.Channel {
		var m map[string]any
		if json.Unmarshal([]byte(line), &m) == nil && m["step"] == "done" {
			if m["cancelled"] != true {
				t.Errorf("cancelled = %v, want true", m["cancelled"])
			}
			if m["files"] != float64(7) {
				t.Errorf("载荷未透传: %v", m)
			}
			return
		}
	}
	t.Fatal("未收到 done 事件")
}

// Close 之后的 Send / SendMap / SetFinalEvent 均为 no-op
func TestSendAfterCloseIsNoop(t *testing.T) {
	p := NewProgress(0)
	p.Close()
	p.Send("app", "x", 1, 1)
	p.SendMap(map[string]any{"step": "file", "name": "y"})
	p.SetFinalEvent(map[string]any{"late": true})
	p.Close()

	for line := range p.Channel {
		var m map[string]any
		if err := json.Unmarshal([]byte(line), &m); err != nil {
			t.Fatalf("事件不是合法 JSON: %q", line)
		}
		if m["step"] == "done" {
			if _, ok := m["late"]; ok {
				t.Error("Close 之后的 SetFinalEvent 不应影响已发出的 done")
			}
		}
	}
}
