package server

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"os/exec"
	"runtime"
	"strings"
)

// parseHTTPURL 只做校验，不改写输入。
//
// 协议相对地址（//host/x）原样放行：它的消费方是浏览器与外部下载器，两者都认
// //host，无需在此补协议。真正需要绝对地址的是 Go 的 http.Client，那一步由调用方
// 在取数前显式做（见 file.go 的 httpx.ResolveScheme）——校验层不替它猜。
func parseHTTPURL(raw string) (string, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return "", fmt.Errorf("empty url")
	}
	// 协议相对：交给下游自己解析，这里只确认 host 部分非空
	if strings.HasPrefix(raw, "//") {
		if strings.Trim(raw[2:], "/") == "" {
			return "", fmt.Errorf("only http/https URLs are allowed")
		}
		return raw, nil
	}
	u, err := url.Parse(raw)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
		return "", fmt.Errorf("only http/https URLs are allowed")
	}
	return raw, nil
}

func OpenBrowser(url string) {
	var c *exec.Cmd
	switch runtime.GOOS {
	case "darwin":
		c = exec.Command("open", url)
	case "windows":
		c = exec.Command("rundll32", "url.dll,FileProtocolHandler", url)
	default:
		c = exec.Command("xdg-open", url)
	}
	_ = c.Start()
}

func (s *Server) handleOpenURL(w http.ResponseWriter, r *http.Request) {
	if !isLoopback(r) {
		writeError(w, http.StatusForbidden, "仅本机可用")
		return
	}
	limitBody(w, r)
	var body struct {
		URL string `json:"url"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.URL == "" {
		writeError(w, http.StatusBadRequest, "missing url")
		return
	}
	u, err := parseHTTPURL(body.URL)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	OpenBrowser(u)
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}
