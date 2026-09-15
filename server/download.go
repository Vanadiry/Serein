package server

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"os/exec"
	"strings"

	"github.com/gorilla/websocket"
	"github.com/vanadiry/serein/core/store"
)

func (s *Server) handleDownload(w http.ResponseWriter, r *http.Request) {
	limitBody(w, r)
	var body struct {
		URL string `json:"url"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.URL == "" {
		writeError(w, http.StatusBadRequest, "missing url")
		return
	}
	u, err := url.Parse(body.URL)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
		writeError(w, http.StatusBadRequest, "only http/https URLs are allowed")
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
			store.LogfWarn("[download] ndm: %v, fallback to browser", err)
			OpenBrowser(body.URL)
			writeJSON(w, http.StatusOK, map[string]string{"status": "ok", "message": "Neat Download Manager 未运行，已在浏览器中打开"})
			return
		}
		writeJSON(w, http.StatusOK, map[string]string{"status": "ok", "message": "已发送到 Neat Download Manager"})

	case strings.Contains(dl, "{url}"):
		// 先按空白切分模板，再逐参数替换 {url}，避免 URL 中的空格被拆成额外参数
		args := strings.Fields(dl)
		if len(args) == 0 {
			writeError(w, http.StatusBadRequest, "下载器命令为空")
			return
		}
		for i := range args {
			args[i] = strings.ReplaceAll(args[i], "{url}", body.URL)
		}
		if _, err := exec.LookPath(args[0]); err != nil {
			store.LogfWarn("[download] %s not found, fallback to browser", args[0])
			OpenBrowser(body.URL)
			writeJSON(w, http.StatusOK, map[string]string{"status": "ok", "message": fmt.Sprintf("%s 未找到，已在浏览器中打开", args[0])})
			return
		}
		go exec.Command(args[0], args[1:]...).Start()
		writeJSON(w, http.StatusOK, map[string]string{"status": "ok", "message": "已调用 " + args[0]})

	default:
		store.LogfWarn("[download] unknown downloader %q, fallback to browser", dl)
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
