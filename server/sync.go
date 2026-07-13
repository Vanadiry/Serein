package server

import (
	"encoding/json"
	"net/http"

	"github.com/vanadiry/serein/core/store"
)

func (s *Server) handleSync(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Type string `json:"type"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid body")
		return
	}

	switch body.Type {
	case "rules":
		s.syncRules(w)
	case "profile":
		s.syncProfile(w)
	default:
		writeError(w, http.StatusBadRequest, "unknown sync type: must be 'rules' or 'profile'")
	}
}

func (s *Server) syncRules(w http.ResponseWriter) {
	if len(s.config.RuleSources) == 0 {
		writeJSON(w, http.StatusOK, map[string]string{"task_id": ""})
		return
	}
	store.Logf("[sync] %d sources", len(s.config.RuleSources))
	p := store.NewProgress(0)
	go store.SyncAllSourcesAsync(s.home, s.config.RuleSources, s.config.Download.Concurrency, p)
	writeJSON(w, http.StatusOK, map[string]string{"task_id": p.ID})
}

func (s *Server) syncProfile(w http.ResponseWriter) {
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
