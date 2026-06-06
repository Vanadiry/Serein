// SSE 进度系统：轻量版 Progress，用于检查更新时推送实时进度。
package store

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"sync"
)

// Progress 一次操作的进度状态
type Progress struct {
	ID      string
	Channel chan string
	Total   int
	Done    int
	Name    string // 当前正在检查的 app 名称
	mu      sync.Mutex
}

var (
	progressMap = map[string]*Progress{}
	progressMu  sync.Mutex
)

// NewProgress 创建一个进度追踪器，total 为待检查的 app 总数。
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

// Send 发送进度事件。done 为已完成数（累计），name 为刚完成的 app 名称。
func (p *Progress) Send(name string, done, total int) {
	p.mu.Lock()
	p.Done = done
	p.Name = name
	p.mu.Unlock()
	select {
	case p.Channel <- progressEvent("app", name, done, total):
	default:
	}
}

// Close 结束进度追踪。
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

// HandleProgressCancel 终止端点：POST /api/check/cancel/{task_id}
func HandleProgressCancel(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(map[string]string{"status": "cancelled"})
	os.Exit(0)
}

// HandleProgressSSE SSE 端点：GET /api/check/progress/{task_id}
func HandleProgressSSE(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("task_id")
	p := GetProgress(id)
	if p == nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusNotFound)
		json.NewEncoder(w).Encode(map[string]string{"error": "task not found"})
		return
	}
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	flusher, ok := w.(http.Flusher)
	if !ok {
		return
	}
	for event := range p.Channel {
		fmt.Fprintf(w, "data: %s\n\n", event)
		flusher.Flush()
	}
}
