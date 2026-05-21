// Rules API handlers
package server

import (
	"net/http"
	"strings"

	"github.com/vanadiry/serein/core/store"
)

// ── POST /api/rules/sync ──

func (s *Server) handleRulesSync(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "POST required")
		return
	}
	if len(s.config.RuleSources) == 0 {
		writeJSON(w, http.StatusOK, map[string]any{
			"synced": []string{},
			"errors": []string{},
		})
		return
	}
	store.Logf("[sync] %d sources", len(s.config.RuleSources))
	result := store.SyncAllSources(s.home, s.config.RuleSources, s.config.Serein.Concurrency)
	store.Logf("[sync] done, %d synced, %d skipped, %d errors", len(result.Synced), len(result.Skipped), len(result.Errors))
	writeJSON(w, http.StatusOK, result)
}

// ── GET /api/rules/list/all ──

func (s *Server) handleRulesListAll(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "GET required")
		return
	}
	list, err := store.ListAllSourceInfos(s.home)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if list == nil {
		list = []store.SourceWithID{}
	}
	writeJSON(w, http.StatusOK, list)
}

// ── GET /api/rules/list/{source_id} ──

func (s *Server) handleRulesListBySource(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "GET required")
		return
	}
	sourceID := strings.TrimPrefix(r.URL.Path, "/api/rules/list/")
	if sourceID == "" || sourceID == "all" || sourceID == "search" {
		writeError(w, http.StatusBadRequest, "missing source_id")
		return
	}

	rules, _ := store.LoadRules(s.home)
	var filtered []store.Rule
	for _, rule := range rules {
		if rule.SourceID == sourceID {
			filtered = append(filtered, rule)
		}
	}
	writeJSON(w, http.StatusOK, formatRuleList(s.home, filtered))
}

// ── GET /api/rules/list/search?q=xxx ──

func (s *Server) handleRulesListSearch(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "GET required")
		return
	}
	q := strings.ToLower(r.URL.Query().Get("q"))
	if q == "" {
		writeError(w, http.StatusBadRequest, "missing q param")
		return
	}
	rules, _ := store.LoadRules(s.home)
	var filtered []store.Rule
	for _, rule := range rules {
		if strings.Contains(strings.ToLower(rule.Info.Name), q) {
			filtered = append(filtered, rule)
		}
	}
	writeJSON(w, http.StatusOK, formatRuleList(s.home, filtered))
}

// ── 共享 ──

type ruleListItem struct {
	AppID           string   `json:"app_id"`
	Name            string   `json:"name"`
	Description     string   `json:"description,omitempty"`
	OfficialWebsite string   `json:"official_website,omitempty"`
	Status          []string `json:"status,omitempty"`
	Platforms       []string `json:"platforms"`
	SourceID        string   `json:"source_id"`
	SourceName      string   `json:"source_name,omitempty"`
}

func formatRuleList(home string, rules []store.Rule) []ruleListItem {
	var result []ruleListItem
	for _, rule := range rules {
		item := ruleListItem{
			AppID:           rule.Info.AppID,
			Name:            rule.Info.Name,
			Description:     rule.Info.Description,
			OfficialWebsite: rule.Info.OfficialWebsite,
			Status:          rule.Info.Status,
			Platforms:       rule.Info.Platforms,
			SourceID:        rule.SourceID,
		}
		if si, err := store.LoadSourceInfo(home, rule.SourceID); err == nil && si != nil {
			item.SourceName = si.Name
		}
		result = append(result, item)
	}
	if result == nil {
		result = []ruleListItem{}
	}
	return result
}
