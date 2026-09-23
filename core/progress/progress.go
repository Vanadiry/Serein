// SSE 进度系统：轻量版 Progress，用于推送实时进度
package progress

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"sync"
)

// Progress 一次操作的进度状态
type Progress struct {
	ID        string
	Channel   chan string
	Total     int
	Done      int
	Name      string // 当前正在处理的名称
	ctx       context.Context
	cancel    context.CancelFunc
	mu        sync.Mutex
	closed    bool
	cancelled bool
}

var (
	progressMap = map[string]*Progress{}
	progressMu  sync.Mutex
)

// NewProgress 创建一个进度追踪器，total 为总数
func NewProgress(total int) *Progress {
	b := make([]byte, 4)
	rand.Read(b)
	ctx, cancel := context.WithCancel(context.Background())
	p := &Progress{
		ID:      hex.EncodeToString(b),
		Channel: make(chan string, 64),
		Total:   total,
		ctx:     ctx,
		cancel:  cancel,
	}
	progressMu.Lock()
	progressMap[p.ID] = p
	progressMu.Unlock()
	p.Channel <- progressEvent("start", "", 0, total)
	return p
}

// Context 返回该任务的 context，供子任务响应取消
func (p *Progress) Context() context.Context { return p.ctx }

// Cancel 取消该任务（幂等）
func (p *Progress) Cancel() {
	p.mu.Lock()
	p.cancelled = true
	p.mu.Unlock()
	p.cancel()
}

// Send 发送进度事件。step: "app" / "list" / "file" 等
func (p *Progress) Send(step, name string, done, total int) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.closed {
		return
	}
	p.Done = done
	p.Name = name
	// select 带 default，不会阻塞，持锁发送安全
	select {
	case p.Channel <- progressEvent(step, name, done, total):
	default:
	}
}

// SendMap 发送任意 JSON 事件
func (p *Progress) SendMap(m map[string]any) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.closed {
		return
	}
	data, _ := json.Marshal(m)
	select {
	case p.Channel <- string(data):
	default:
	}
}

// Close 结束进度追踪，可重复调用
// 关闭后 Send/SendMap 变为 no-op，避免与并发 Send 撞上 send-on-closed-channel
func (p *Progress) Close() {
	p.mu.Lock()
	if p.closed {
		p.mu.Unlock()
		return
	}
	p.closed = true
	data, _ := json.Marshal(map[string]any{"step": "done", "cancelled": p.cancelled})
	select {
	case p.Channel <- string(data):
	default:
	}
	close(p.Channel)
	p.mu.Unlock()

	p.cancel()

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
