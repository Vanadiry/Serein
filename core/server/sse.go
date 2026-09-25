package server

import (
	"fmt"
	"net/http"
)

// serveSSE 将通道中的数据以 SSE 形式持续写出，直到客户端断开或通道关闭
// 不负责通道的订阅/清理，由调用方处理
func serveSSE[T ~string | ~[]byte](w http.ResponseWriter, r *http.Request, ch <-chan T) {
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
			return
		case data, ok := <-ch:
			if !ok {
				return
			}
			fmt.Fprintf(w, "data: %s\n\n", string(data))
			flusher.Flush()
		}
	}
}
