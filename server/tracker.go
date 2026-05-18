// Tracker API handlers
package server

import (
	"encoding/json"
	"net/http"
	"strings"

	"github.com/vanadiry/serein/core/store"
)

// ── GET /api/tracker/list/all ──

func (s *Server) handleTrackerListAll(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "GET required")
		return
	}
	list, err := store.LoadAllTrackerInfo(s.home)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if list == nil {
		list = []store.TrackerInfo{}
	}
	writeJSON(w, http.StatusOK, list)
}

// ── GET /api/tracker/list/{id} ──

func (s *Server) handleTrackerListByID(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "GET required")
		return
	}
	id := strings.TrimPrefix(r.URL.Path, "/api/tracker/list/")
	if id == "" {
		writeError(w, http.StatusBadRequest, "missing tracker id")
		return
	}

	entries, err := store.LoadTrackerFile(s.home, id)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	rules, _ := store.LoadRules(s.home)
	userData, _ := store.LoadUserData(s.home)

	type detail struct {
		AppID          string            `json:"app_id"`
			Description     string            `json:"description,omitempty"`
		Name            string            `json:"name"`
		OfficialWebsite string            `json:"official_website,omitempty"`
			Status          []string          `json:"status,omitempty"`
		SourceID        string            `json:"source_id,omitempty"`
		SourceName      string            `json:"source_name,omitempty"`
		CurrentVersion  map[string]string `json:"current_version"`
		RuleMissing     bool              `json:"rule_missing,omitempty"`
	}

	var result []detail
	for _, entry := range entries {
		d := detail{
			AppID:         entry.AppID,
			CurrentVersion: make(map[string]string),
		}
		rule, ok := rules[entry.AppID]
		if !ok {
			d.Name = entry.AppID
			d.RuleMissing = true
		} else {
			d.Name = rule.Info.Name
			d.Description = rule.Info.Description
			d.OfficialWebsite = rule.Info.OfficialWebsite
			d.Status = rule.Info.Status
			d.SourceID = rule.SourceID
			if si, err := store.LoadSourceInfo(s.home, rule.SourceID); err == nil && si != nil {
				d.SourceName = si.Name
			}
		}
		platforms := entry.Platforms
		if len(platforms) == 0 {
			platforms = s.config.Serein.Platforms
		}
		ud := userData[entry.AppID]
			for _, p := range platforms {
				if ud != nil {
					d.CurrentVersion[p] = ud[p]
				} else {
					d.CurrentVersion[p] = ""
				}
			}
		result = append(result, d)
	}
	if result == nil {
		result = []detail{}
	}
	writeJSON(w, http.StatusOK, result)
}

// ── POST /api/tracker/new ──

func (s *Server) handleTrackerNew(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "POST required")
		return
	}
	var body struct {
		ID string `json:"id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.ID == "" {
		writeError(w, http.StatusBadRequest, "missing id")
		return
	}
	if store.TrackerExists(s.home, body.ID) {
		writeError(w, http.StatusConflict, "tracker already exists")
		return
	}
	if err := store.CreateTrackerFile(s.home, body.ID); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusCreated, map[string]string{"id": body.ID})
}

// ── POST /api/tracker/add ──

func (s *Server) handleTrackerAdd(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "POST required")
		return
	}
	var body struct {
		TrackerID string   `json:"tracker_id"`
		AppID     string   `json:"app_id"`
			Description     string            `json:"description,omitempty"`
		Platforms []string `json:"platforms"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid body")
		return
	}
	if body.AppID == "" {
		writeError(w, http.StatusBadRequest, "missing app_id")
		return
	}
	if body.TrackerID == "" {
		body.TrackerID = "_serein"
	}

	if err := store.AddToTracker(s.home, body.TrackerID, store.TrackerEntry{
		AppID:    body.AppID,
		Platforms: body.Platforms,
	}); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{
		"tracker_id": body.TrackerID,
		"app_id":     body.AppID,
	})
}

// ── GET /api/tracker/apps ──

func (s *Server) handleTrackerApps(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "GET required")
		return
	}
	entries, err := store.LoadTracker(s.home)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	seen := make(map[string]bool)
	var apps []string
	for _, e := range entries {
		if !seen[e.AppID] {
			seen[e.AppID] = true
			apps = append(apps, e.AppID)
		}
	}
	if apps == nil {
		apps = []string{}
	}
	writeJSON(w, http.StatusOK, apps)
}
