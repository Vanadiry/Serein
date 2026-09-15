package server

import (
	"encoding/json"
	"net/http"
	"net/url"
	"os/exec"
	"runtime"

	"github.com/vanadiry/serein/core/store"
)

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
	limitBody(w, r)
	if !store.HasAccess(r) {
		writeError(w, http.StatusForbidden, "missing "+store.AccessHeader)
		return
	}
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
	OpenBrowser(u.String())
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}
