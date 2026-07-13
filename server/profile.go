package server

import (
	"net/http"

	"github.com/vanadiry/serein/core/store"
)

func (s *Server) handleProfile(w http.ResponseWriter, r *http.Request) {
	p, err := store.LoadProfile(s.home)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"known_extensions": p.KnownExtensions,
	})
}

func (s *Server) handleProfileSync(w http.ResponseWriter, r *http.Request) {
	cfg, err := store.LoadConfig(s.home)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if cfg.Profile.URL == "" {
		writeJSON(w, http.StatusOK, map[string]any{"updated": false, "message": "未配置 profile URL"})
		return
	}

	p, updated, err := store.SyncProfile(s.home, cfg.Profile.URL)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"updated":          updated,
		"known_extensions": p.KnownExtensions,
	})
}
