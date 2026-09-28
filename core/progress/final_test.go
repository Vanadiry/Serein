package progress

import (
	"encoding/json"
	"testing"
)

// 结束事件只能有一条。调用方自行 SendMap("done") 会与 Close 发的收尾 done 重复
// 客户端只取到其中一条，而重复的那条若载荷丢失只剩 {"step":"done"}
// 前端会把它渲染成“同步完成”，把失败静默吞掉
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

// 只有“已结束且缓冲区读空”才需要补发结束事件
// 否则缓冲区里还有事件时会与正常读取路径重复送出同一帧
func TestDrainedOnlyWhenClosedAndEmpty(t *testing.T) {
	p := NewProgress(0)
	if p.Drained() {
		t.Error("任务进行中不该判为 drained")
	}
	p.Send("app", "a", 1, 1)
	if p.Drained() {
		t.Error("缓冲区还有事件时不该判为 drained")
	}
	p.Close()
	if p.Drained() {
		t.Error("缓冲区仍有未读事件（含 done）时不该判为 drained")
	}
	// 读空缓冲区
	for range p.Channel {
	}
	if !p.Drained() {
		t.Error("已结束且读空后应判为 drained")
	}
	if p.Replay() == "" {
		t.Error("drained 后必须有可补发的结束事件")
	}
}

// Close 时缓冲区满导致 done 被丢弃，Drained 仍应为 true（靠 Replay 兜住）
func TestDrainedRecoversDroppedDone(t *testing.T) {
	p := NewProgress(0)
	for i := 0; i < 100; i++ { // 超过 cap 64，撑满缓冲区
		p.Send("app", "f", i, 100)
	}
	p.Close()
	for range p.Channel {
	}
	if !p.Drained() {
		t.Fatal("应判为 drained")
	}
	var m map[string]any
	if err := json.Unmarshal([]byte(p.Replay()), &m); err != nil {
		t.Fatal(err)
	}
	if m["step"] != "done" {
		t.Errorf("补发的应是 done，实际 %v", m["step"])
	}
}
