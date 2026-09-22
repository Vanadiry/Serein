// SSE 进度系统：轻量版 Progress，用于推送实时进度
package progress

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"sync"
)

// Progress 一次操作的进度状态
type Progress struct {
	ID      string
	Channel chan string
	Total   int
	Done    int
	Name    string // 当前正在处理的名称
	mu      sync.Mutex
}

var (
	progressMap = map[string]*Progress{}
	progressMu  sync.Mutex
)

// NewProgress 创建一个进度追踪器，total 为总数
func NewProgress(total int) *Progress {
	b := make([]byte, 4)
	rand.Read(b)
	p := &Progress{
		ID:      hex.EncodeToString(b),
		Channel: make(chan string, 64),
		Total:   total,
	}
	progressMu.Lock()
	progressMap[p.ID] = p
	progressMu.Unlock()
	p.Channel <- progressEvent("start", "", 0, total)
	return p
}

// Send 发送进度事件。step: "app" / "list" / "file" 等
func (p *Progress) Send(step, name string, done, total int) {
	p.mu.Lock()
	p.Done = done
	p.Name = name
	p.mu.Unlock()
	select {
	case p.Channel <- progressEvent(step, name, done, total):
	default:
	}
}

// SendMap 发送任意 JSON 事件
func (p *Progress) SendMap(m map[string]any) {
	data, _ := json.Marshal(m)
	select {
	case p.Channel <- string(data):
	default:
	}
}

// Close 结束进度追踪
func (p *Progress) Close() {
	select {
	case p.Channel <- `{"step":"done"}`:
	default:
	}
	close(p.Channel)
	progressMu.Lock()
	delete(progressMap, p.ID)
	progressMu.Unlock()
}

func progressEvent(step, name string, done, total int) string {
	data, _ := json.Marshal(map[string]any{
		"step":  step,
		"name":  name,
		"done":  done,
		"total": total,
	})
	return string(data)
}

// GetProgress 根据 ID 获取进度
func GetProgress(id string) *Progress {
	progressMu.Lock()
	defer progressMu.Unlock()
	return progressMap[id]
}
