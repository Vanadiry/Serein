// SSE 进度系统：向 EventSource 推送任务进度
package progress

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"sync"
	"sync/atomic"
	"time"
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
	finished  bool           // 结束事件已产出，可补发
	final     map[string]any // 预设的结束事件载荷
	last      atomic.Value   // string：最后一条事件帧
}

// progressRetain 任务结束后保留条目的时长
// 客户端可能在任务结束后才连上 SSE（连得晚、或 EventSource 重连），那时
// 缓冲区已读空，只能靠快照补发；立即删除会让那种订阅直接 404
// 任务在界面上就永远停着。声明为 var 以便测试覆盖
var progressRetain = 60 * time.Second

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
	p.mu.Lock()
	p.emit(progressEvent("start", "", 0, total))
	p.mu.Unlock()
	return p
}

// emit 记录并投递一条事件。调用方必须持有 p.mu
// 缓冲区满时只丢这一帧，客户端消费不过来时中间帧只影响进度条精度
// last 仍会更新，供迟到的订阅者补发
func (p *Progress) emit(data string) {
	p.last.Store(data)
	select {
	case p.Channel <- data:
	default:
	}
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
	p.emit(progressEvent(step, name, done, total))
}

// SendMap 发送任意 JSON 事件
func (p *Progress) SendMap(m map[string]any) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.closed {
		return
	}
	data, _ := json.Marshal(m)
	p.emit(string(data))
}

// SetFinalEvent 预设结束事件的载荷，由 Close 发出
// 调用方往往直到收尾才知道汇总数字，而结束事件只能在 Close 里发，所以让它能携带载荷
// 调用方自己先发一条 done 会与 Close 发的收尾 done 重复，客户端会把重复的那条当成两次结束
// 前端在第一条就 close，这个 bug 才没有暴露出来
// 载荷丢失的那条只剩 {"step":"done"}，客户端会把它渲染成“同步完成”，把失败静默吞掉
func (p *Progress) SetFinalEvent(payload map[string]any) {
	p.mu.Lock()
	p.final = payload
	p.mu.Unlock()
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
	payload := p.final
	if payload == nil {
		payload = map[string]any{}
	} else {
		// 复制一份，避免把调用方持有的 map 与后续 Close 竞争
		cp := make(map[string]any, len(payload)+2)
		for k, v := range payload {
			cp[k] = v
		}
		payload = cp
	}
	payload["step"] = "done"
	payload["cancelled"] = p.cancelled
	data, _ := json.Marshal(payload)
	p.last.Store(string(data))
	p.finished = true

	// 结束事件必须送达：先把缓冲区排空腾出位置
	// 排掉的是客户端来不及看的中间帧，而 done 丢了客户端会永远等不到任务结束
	// 进度遮罩就永久卡住（前端只在收到 done 时才 pm.close()）
	for drained := false; !drained; {
		select {
		case <-p.Channel:
		default:
			drained = true
		}
	}
	// 缓冲区已空、容量 64、且此刻持有 p.mu 并且 p.closed 已置位，不会有并发 Send
	// 因此这次发送不可能阻塞
	p.Channel <- string(data)
	close(p.Channel)
	p.mu.Unlock()

	p.cancel()
	p.retain()
}

// retain 延迟从注册表删除，让迟到的订阅者仍能补发到结束事件
func (p *Progress) retain() {
	time.AfterFunc(progressRetain, func() {
		progressMu.Lock()
		delete(progressMap, p.ID)
		progressMu.Unlock()
	})
}

// Drained 报告“任务已结束且缓冲区已被读空”，此时新订阅者再读通道会立刻
// 拿到关闭信号，一条事件都收不到，必须靠 Replay 补发结束事件
// 任务仍在进行、或缓冲区里还有未读事件时都要返回 false
// 那些事件会由正常的读取路径送出，此时再补发会让同一帧被消费两次
func (p *Progress) Drained() bool {
	p.mu.Lock()
	closed := p.closed
	p.mu.Unlock()
	return closed && len(p.Channel) == 0
}

// Replay 返回可补发给迟到订阅者的最后一条事件；不需要补发时返回 ""
// 仅在任务已结束后非空，任务仍在进行时补发会让同一帧被消费两次
func (p *Progress) Replay() string {
	p.mu.Lock()
	finished := p.finished
	p.mu.Unlock()
	if !finished {
		return ""
	}
	s, _ := p.last.Load().(string)
	return s
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
