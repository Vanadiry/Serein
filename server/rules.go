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
	result := store.SyncAllSources(s.home, s.config.RuleSources)
	writeJSON(w, http.StatusOK, result)
}

// ── GET /api/rules/list/all ──

func (s *Server) handleRulesListAll(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "GET required")
		return
	}
	// 遍历 rules/ 子目录，读取 info.json 获取源元信息
	type sourceItem struct {
		SourceID   string `json:"source_id"`
		Name       string `json:"name"`
		AppCount   int    `json:"app_count"`
	}

	rules, _ := store.LoadRules(s.home)
	// 统计每个 source 下的规则数
	counts := make(map[string]int)
	sourceNames := make(map[string]string)
	for _, rule := range rules {
		counts[rule.SourceID]++
		if _, ok := sourceNames[rule.SourceID]; !ok {
			if si, err := store.LoadSourceInfo(s.home, rule.SourceID); err == nil && si != nil {
				sourceNames[rule.SourceID] = si.Name
			}
		}
	}

	var result []sourceItem
	for id, count := range counts {
		result = append(result, sourceItem{
			SourceID: id,
			Name:     sourceNames[id],
			AppCount: count,
		})
	}
	if result == nil {
		result = []sourceItem{}
	}
	writeJSON(w, http.StatusOK, result)
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
