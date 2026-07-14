package server

import (
	"encoding/json"
	"net/http"
	"os/exec"
	"runtime"
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
	var body struct {
		URL string `json:"url"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.URL == "" {
		writeError(w, http.StatusBadRequest, "missing url")
		return
	}
	OpenBrowser(body.URL)
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}
