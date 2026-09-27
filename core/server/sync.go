package server

import (
	"encoding/json"
	"net/http"

	"github.com/vanadiry/serein/core/checker"
	"github.com/vanadiry/serein/core/log"
	"github.com/vanadiry/serein/core/progress"
	"github.com/vanadiry/serein/core/store"
)

func (s *Server) handleSync(w http.ResponseWriter, r *http.Request) {
	limitBody(w, r)
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
		s.syncProfile(w, r)
	default:
		writeError(w, http.StatusBadRequest, "unknown sync type: must be 'rules' or 'profile'")
	}
}

func (s *Server) syncRules(w http.ResponseWriter) {
	if len(s.config.RuleSources) == 0 {
		writeJSON(w, http.StatusOK, map[string]string{"task_id": ""})
		return
	}
	log.Logf("[sync] %d sources", len(s.config.RuleSources))
	p := progress.NewProgress(0)
	goSafe("sync", p, func() {
		store.SyncAllSourcesAsync(s.home, s.config.RuleSources, s.config.Download.Concurrency, p, s.reloadRules)
	})
	writeJSON(w, http.StatusOK, map[string]string{"task_id": p.ID})
}

func (s *Server) syncProfile(w http.ResponseWriter, r *http.Request) {
	url := s.config.Profile.URL
	if url == "" {
		writeJSON(w, http.StatusOK, map[string]any{"updated": false, "message": "未配置 profile URL"})
		return
	}

	p, updated, err := store.SyncProfile(r.Context(), s.home, url)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	// 同步后立刻应用新的前后缀：否则设置页显示的是新列表，实际检查仍用旧列表。
	// （handleConfig 每次请求都重读 profile 文件，所以那个不一致是可见的。）
	checker.SetVersionPrefixes(p.VersionPrefixes)
	checker.SetVersionSuffixes(p.VersionSuffixes)
	writeJSON(w, http.StatusOK, map[string]any{
		"updated":          updated,
		"known_extensions": p.KnownExtensions,
	})
}
