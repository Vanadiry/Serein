package server

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"

	"github.com/vanadiry/serein/core/progress"
)

// POST /api/check/cancel/{task_id}
// 终止端点。跨站请求由 server 的 sameOriginGuard 统一拦截
func handleProgressCancel(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(map[string]string{"status": "cancelled"})
	os.Exit(0)
}

// GET /api/progress/{task_id} SSE 端点
func handleProgressSSE(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("task_id")
	p := progress.GetProgress(id)
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
	ctx := r.Context()
	for {
		select {
		case <-ctx.Done():
			// 客户端断开，退出协程
			return
		case event, ok := <-p.Channel:
			if !ok {
				return
			}
			fmt.Fprintf(w, "data: %s\n\n", event)
			flusher.Flush()
		}
	}
}
