// Check API handlers
package server

import (
	"encoding/json"
	"net/http"

	"github.com/vanadiry/serein/core/checker"
	"github.com/vanadiry/serein/core/store"
)

// ── POST /api/check/all ──

func (s *Server) handleCheckAll(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "POST required")
		return
	}
	entries, err := store.LoadTracker(s.home)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	checker.ClearURLCache()
	result := runTrackerChecks(s.home, entries)
	writeJSON(w, http.StatusOK, result)
}

// ── POST /api/check/ids ──

func (s *Server) handleCheckIDs(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "POST required")
		return
	}
	var ids []string
	if err := json.NewDecoder(r.Body).Decode(&ids); err != nil {
		writeError(w, http.StatusBadRequest, "invalid body: expected array of rule ids")
		return
	}
	allEntries, err := store.LoadTracker(s.home)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	var entries []store.TrackerEntry
	for _, e := range allEntries {
		for _, id := range ids {
			if e.RuleID == id {
				entries = append(entries, e)
				break
			}
		}
	}
	result := runTrackerChecks(s.home, entries)
	writeJSON(w, http.StatusOK, result)
}

// ── POST /api/check/tracker ──

func (s *Server) handleCheckTracker(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "POST required")
		return
	}
	var body struct {
		TrackerID string `json:"tracker_id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.TrackerID == "" {
		writeError(w, http.StatusBadRequest, "missing tracker_id")
		return
	}
	entries, err := store.LoadTrackerFile(s.home, body.TrackerID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	result := runTrackerChecks(s.home, entries)
	writeJSON(w, http.StatusOK, result)
}

// ── POST /api/check/confirm ──

func (s *Server) handleCheckConfirm(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "POST required")
		return
	}
	var body map[string]string
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid body")
		return
	}
	uuid, ok := body["uuid"]
	if !ok || uuid == "" {
		writeError(w, http.StatusBadRequest, "missing uuid")
		return
	}

	userData, err := store.LoadUserData(s.home)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if userData[uuid] == nil {
		userData[uuid] = make(map[string]string)
	}
	for k, v := range body {
		if k == "uuid" {
			continue
		}
		userData[uuid][k] = v
	}
	if err := store.SaveUserData(s.home, userData); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, userData[uuid])
}

// ── 共享检查逻辑 ──

func runTrackerChecks(home string, entries []store.TrackerEntry) []checker.CheckResponse {
	cfg, _ := store.LoadConfig(home)
	rules, _ := store.LoadRules(home)
	userData, _ := store.LoadUserData(home)

	var results []checker.CheckResponse
	for _, entry := range entries {
		rule, ok := rules[entry.RuleID]
		if !ok {
			continue
		}
		platforms := store.PlatformsFor(entry, cfg.Platforms)

		var platCfgs []checker.PlatformCheckConfig
		for _, os := range platforms {
			platCfg := rule.MergedConfig(os)
			if platCfg.URL == "" && platCfg.Type != "github" {
				continue
			}
			currentVer := ""
			if ud, ok := userData[entry.RuleID]; ok {
				currentVer = ud[os]
			}
			platCfgs = append(platCfgs, checker.PlatformCheckConfig{
				OS:             os,
				Type:           platCfg.Type,
				URL:            platCfg.URL,
				UA:             platCfg.UA,
				Headers:        platCfg.Headers,
				BaseURL:        platCfg.BaseURL,
				VPosition:      platCfg.VPosition,
				DPosition:      platCfg.DPosition,
				VJoin:          platCfg.VJoin,
				DJoin:          platCfg.DJoin,
				CurrentVersion: currentVer,
			})
		}

		if len(platCfgs) == 0 {
			continue
		}

		req := checker.CheckRequest{
			UUID:            rule.Info.UUID,
			Name:            rule.Info.Name,
			OfficialWebsite: rule.Info.OfficialWebsite,
			RuleType:        rule.Config.Type,
			Owner:           rule.Config.Owner,
			Repo:            rule.Config.Repo,
			Platforms:       platCfgs,
		}

		resp, err := checker.RunCheck(req)
		if err != nil {
			continue
		}
		results = append(results, resp)
	}
	if results == nil {
		results = []checker.CheckResponse{}
	}
	return results
}
