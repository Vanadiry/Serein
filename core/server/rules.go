package server

import (
	"net/http"
	"sort"
	"strings"

	"github.com/vanadiry/serein/core/store"
)

func (s *Server) handleRules(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query().Get("q")
	source := r.URL.Query().Get("source")

	if q != "" {
		rules, _ := store.LoadRules(s.home)
		var filtered []store.Rule
		ql := strings.ToLower(q)
		for _, rule := range rules {
			if strings.Contains(strings.ToLower(rule.Info.Name), ql) {
				filtered = append(filtered, rule)
			}
		}
		writeJSON(w, http.StatusOK, formatRuleList(s.home, filtered))
		return
	}

	if source != "" {
		rules, _ := store.LoadRules(s.home)
		var filtered []store.Rule
		for _, rule := range rules {
			if rule.SourceID == source {
				filtered = append(filtered, rule)
			}
		}
		writeJSON(w, http.StatusOK, formatRuleList(s.home, filtered))
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
	sort.Slice(rules, func(i, j int) bool {
		return strings.ToLower(rules[i].Info.Name) < strings.ToLower(rules[j].Info.Name)
	})
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
