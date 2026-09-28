package server

import (
	"encoding/json"
	"net/http"

	"github.com/vanadiry/serein/core/progress"
)

// POST /api/progress/cancel/{task_id}
// 取消任务：中断其 context，由任务自身收尾。跨站请求由 server 的 sameOriginGuard 统一拦截
func handleProgressCancel(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("task_id")
	p := progress.GetProgress(id)
	if p == nil {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "task not found"})
		return
	}
	p.Cancel()
	writeJSON(w, http.StatusOK, map[string]string{"status": "cancelled"})
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
	// 只在“任务已结束且缓冲区已读空”时补发结束事件：这时读通道会立刻返回关闭
	// 客户端一条都收不到。否则不补发，缓冲区里还有事件时会与正常读取重复
	var replay string
	if p.Drained() {
		replay = p.Replay()
	}
	serveSSE(w, r, p.Channel, replay)
}
