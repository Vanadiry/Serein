package events

import (
	"encoding/json"
	"strings"
	"testing"
)

// EmitSticky 的事件必须带 sticky:true，前端据此决定 toast 不自动消失。
// 之前是前端按 context === "[rules]" 猜，后端与前端两处枚举会漂移。
func TestEmitSticky(t *testing.T) {
	const mineS = "sticky-test-persistent"
	const mineP = "sticky-test-plain"
	EmitSticky("warn", "[rules]", mineS)
	Emit("warn", "[sync]", mineP)

	var sticky, plain map[string]any
	for _, ev := range drain(t) {
		switch ev["message"] {
		case mineS:
			sticky = ev
		case mineP:
			plain = ev
		}
	}
	if sticky == nil || plain == nil {
		t.Fatalf("两条事件都没收到: %+v", drain(t))
	}
	if sticky["sticky"] != true {
		t.Errorf("EmitSticky 应带 sticky=true，实际 %+v", sticky)
	}
	// Emit 不带 sticky，零值会被 omitempty 省略
	if _, ok := plain["sticky"]; ok {
		t.Errorf("Emit 不应带 sticky 字段（零值省略）: %+v", plain)
	}
	if raw := string(mustJSON(t, plain)); strings.Contains(raw, `"sticky"`) {
		t.Errorf("普通事件的 JSON 不应出现 sticky 键: %s", raw)
	}
}

// level 与 context 都原样透传，前端不靠 context 猜语义
func TestEmitKeepsLevelAndContext(t *testing.T) {
	const mine = "sticky-test-唯一标记"
	EmitSticky("error", "[rules]", mine)
	// ring 是进程级的，可能有其它用例留下的事件，按自己的 message 定位
	for _, ev := range drain(t) {
		if ev["message"] != mine {
			continue
		}
		if ev["level"] != "error" || ev["context"] != "[rules]" || ev["sticky"] != true {
			t.Errorf("level/context/sticky 被改动: %+v", ev)
		}
		return
	}
	t.Fatal("没找到自己发出的事件")
}

func mustJSON(t *testing.T, v any) []byte {
	t.Helper()
	b, err := json.Marshal(v)
	if err != nil {
		t.Fatal(err)
	}
	return b
}

func drain(t *testing.T) []map[string]any {
	t.Helper()
	ch := Subscribe()
	defer Unsubscribe(ch)
	var out []map[string]any
	// 回放 + 实时：把当前 ring 抽干即可（本测试只关心内容与字段）
	for {
		select {
		case b := <-ch:
			var m map[string]any
			if err := json.Unmarshal(b, &m); err != nil {
				t.Fatalf("非法事件: %s", b)
			}
			out = append(out, m)
			continue
		default:
		}
		return out
	}
}
