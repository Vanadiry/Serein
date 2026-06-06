package server

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os/exec"
	"strings"

	"github.com/gorilla/websocket"
	"github.com/vanadiry/serein/core/store"
)

func (s *Server) handleDownload(w http.ResponseWriter, r *http.Request) {
	var body struct {
		URL string `json:"url"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.URL == "" {
		writeError(w, http.StatusBadRequest, "missing url")
		return
	}
	dl := strings.TrimSpace(s.config.Download.Downloader)
	store.Logf("[download] %s (downloader=%q)", body.URL, dl)

	switch {
	case dl == "":
		OpenBrowser(body.URL)
		writeJSON(w, http.StatusOK, map[string]string{"status": "ok", "message": "已在浏览器中打开"})

	case dl == "ndm":
		err := sendToNDM(body.URL)
		if err != nil {
			store.Logf("[download] ndm: %v, fallback to browser", err)
			OpenBrowser(body.URL)
			writeJSON(w, http.StatusOK, map[string]string{"status": "ok", "message": "Neat Download Manager 未运行，已在浏览器中打开"})
			return
		}
		writeJSON(w, http.StatusOK, map[string]string{"status": "ok", "message": "已发送到 Neat Download Manager"})

	case strings.Contains(dl, "{url}"):
		cmd := strings.ReplaceAll(dl, "{url}", body.URL)
		parts := strings.Fields(cmd)
		if _, err := exec.LookPath(parts[0]); err != nil {
			store.Logf("[download] %s not found, fallback to browser", parts[0])
			OpenBrowser(body.URL)
			writeJSON(w, http.StatusOK, map[string]string{"status": "ok", "message": fmt.Sprintf("%s 未找到，已在浏览器中打开", parts[0])})
			return
		}
		go exec.Command(parts[0], parts[1:]...).Start()
		writeJSON(w, http.StatusOK, map[string]string{"status": "ok", "message": "已调用 " + parts[0]})

	default:
		store.Logf("[download] unknown downloader %q, fallback to browser", dl)
		OpenBrowser(body.URL)
		writeJSON(w, http.StatusOK, map[string]string{"status": "ok", "message": "下载器配置未知，已在浏览器中打开"})
	}
}

func sendToNDM(url string) error {
	dialer := websocket.Dialer{Subprotocols: []string{"neatextension.v1"}}
	conn, _, err := dialer.Dial("ws://127.0.0.1:10007/download", nil)
	if err != nil {
		return err
	}
	defer conn.Close()
	msg := fmt.Sprintf("1:GET\r\n2:%s\r\n6:normal\r\nReferer: %s\r\n", url, url)
	return conn.WriteMessage(websocket.TextMessage, []byte(msg))
}
