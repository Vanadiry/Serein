package server

import (
	"net/http"
	"sort"
	"strings"

	"github.com/vanadiry/serein/core/store"
)

// GET /api/search?q=  跨 Tracker 搜索应用 + 规则（匹配 name / app_id / 描述）
func (s *Server) handleSearch(w http.ResponseWriter, r *http.Request) {
	q := strings.TrimSpace(r.URL.Query().Get("q"))
	if q == "" {
		writeJSON(w, http.StatusOK, map[string]any{"apps": []any{}, "rules": []any{}})
		return
	}
	ql := strings.ToLower(q)
	rules := s.getRules()

	// 规则命中
	var ruleHits []store.Rule
	for _, rule := range rules {
		if textMatches(ql, rule.Info.Name, rule.Info.AppID, rule.Info.Description) {
			ruleHits = append(ruleHits, rule)
		}
	}
	ruleItems := formatRuleList(s.home, ruleHits)

	// 应用命中（遍历所有 Tracker）
	type appHit struct {
		AppID          string            `json:"app_id"`
		Name           string            `json:"name"`
		Description    string            `json:"description,omitempty"`
		TrackerID      string            `json:"tracker_id"`
		TrackerName    string            `json:"tracker_name"`
		CurrentVersion map[string]string `json:"current_version"`
	}
	infos, _ := store.LoadAllTrackerInfo(s.home)
	userData, _ := store.LoadUserData(s.home)
	var appItems []appHit
	for _, ti := range infos {
		entries, err := store.LoadTrackerFile(s.home, ti.ID)
		if err != nil {
			continue
		}
		for _, e := range entries {
			name := e.AppID
			desc := ""
			if rule, ok := rules[e.AppID]; ok {
				name = rule.Info.Name
				desc = rule.Info.Description
			}
			if !textMatches(ql, e.AppID, name, desc) {
				continue
			}
			cv := map[string]string{}
			for _, p := range store.PlatformsFor(e, s.config.Tracker.Platforms) {
				if v := userData[e.AppID][p]; v != "" {
					cv[p] = v
				}
			}
			appItems = append(appItems, appHit{
				AppID:          e.AppID,
				Name:           name,
				Description:    desc,
				TrackerID:      ti.ID,
				TrackerName:    ti.DisplayName,
				CurrentVersion: cv,
			})
		}
	}
	sort.SliceStable(appItems, func(i, j int) bool {
		return strings.ToLower(appItems[i].Name) < strings.ToLower(appItems[j].Name)
	})

	const limit = 30
	if len(appItems) > limit {
		appItems = appItems[:limit]
	}
	if len(ruleItems) > limit {
		ruleItems = ruleItems[:limit]
	}
	if appItems == nil {
		appItems = []appHit{}
	}
	if ruleItems == nil {
		ruleItems = []ruleListItem{}
	}
	writeJSON(w, http.StatusOK, map[string]any{"apps": appItems, "rules": ruleItems})
}

// textMatches 任一字段（非空）包含 ql（均转小写）
func textMatches(ql string, fields ...string) bool {
	for _, f := range fields {
		if f != "" && strings.Contains(strings.ToLower(f), ql) {
			return true
		}
	}
	return false
}
