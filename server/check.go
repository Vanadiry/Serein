// Check API handlers
package server

import (
	"encoding/json"
	"net/http"
	"sync"

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
	store.Logf("[check/all] %d entries", len(entries))
	result := runTrackerChecks(s.home, entries)
	saveCheckTemp(s.home, "all", result)
	store.Logf("[check/all] done, %d results", len(result))
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
			if e.AppID == id {
				entries = append(entries, e)
				break
			}
		}
	}
	store.Logf("[check/ids] %v", ids)
	result := runTrackerChecks(s.home, entries)
	saveCheckTemp(s.home, "ids", result)
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
	store.Logf("[check/tracker] %s", body.TrackerID)
	checker.ClearURLCache()
	result := runTrackerChecks(s.home, entries)
	saveCheckTemp(s.home, "tracker", result)
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
	appID, ok := body["app_id"]
	if !ok || appID == "" {
		writeError(w, http.StatusBadRequest, "missing appID")
		return
	}

	userData, err := store.LoadUserData(s.home)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if userData[appID] == nil {
		userData[appID] = make(map[string]string)
	}
	for k, v := range body {
		if k == "app_id" {
			continue
		}
		userData[appID][k] = v
	}
	if err := store.SaveUserData(s.home, userData); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	store.Logf("[confirm] %s: %v", appID, userData[appID])
	writeJSON(w, http.StatusOK, map[string]any{
		"app_id":   appID,
		"status":   "ok",
		"platforms": userData[appID],
	})
}

// ── 共享检查逻辑 ──

func runTrackerChecks(home string, entries []store.TrackerEntry) []checker.CheckResponse {
	cfg, _ := store.LoadConfig(home)
	rules, _ := store.LoadRules(home)
	userData, _ := store.LoadUserData(home)

	conc := cfg.Serein.Concurrency
	if conc < 1 {
		conc = 1
	}
	if conc > 64 {
		conc = 64
	}

	type checkJob struct {
		req checker.CheckRequest
	}
	var jobs []checkJob
	for _, entry := range entries {
		rule, ok := rules[entry.AppID]
		if !ok {
			continue
		}
		platforms := store.PlatformsFor(entry, cfg.Serein.Platforms)

		var platCfgs []checker.PlatformCheckConfig
		for _, os := range platforms {
			platCfg := rule.MergedConfig(os)
			if platCfg.URL == "" && platCfg.Type != "github" {
				continue
			}

			// 执行前置请求链，获取最终 URL
			preSteps := rule.PreRequestChain(os)
			if len(preSteps) > 0 {
				var checkerSteps []checker.PreStep
				for _, ps := range preSteps {
					checkerSteps = append(checkerSteps, checker.PreStep{
						URL:      ps.URL,
						Type:     ps.Type,
						UA:       ps.UA,
						Headers:  ps.Headers,
						BaseURL:  ps.BaseURL,
						Position: ps.Position,
					})
				}
				preURL, err := checker.RunPreRequests(checkerSteps, checker.NewClient())
				if err == nil && preURL != "" {
					platCfg.URL = preURL
				}
			}

			currentVer := ""
			if ud, ok := userData[entry.AppID]; ok {
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

		jobs = append(jobs, checkJob{req: checker.CheckRequest{
			AppID:           rule.Info.AppID,
			Name:            rule.Info.Name,
			OfficialWebsite: rule.Info.OfficialWebsite,
			RuleType:        rule.Config.Type,
			Owner:           rule.Config.Owner,
			Repo:            rule.Config.Repo,
				GithubToken:     cfg.Serein.GithubToken,
			Platforms:       platCfgs,
		}})
	}

	if len(jobs) <= 1 {
		var results []checker.CheckResponse
		for _, job := range jobs {
			resp, err := checker.RunCheck(job.req)
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

	sem := make(chan struct{}, conc)
	var mu sync.Mutex
	var results []checker.CheckResponse
	var wg sync.WaitGroup

	for _, job := range jobs {
		wg.Add(1)
		go func(j checkJob) {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()

			resp, err := checker.RunCheck(j.req)
			if err != nil {
				return
			}
			mu.Lock()
			results = append(results, resp)
			mu.Unlock()
		}(job)
	}
	wg.Wait()

	if results == nil {
		results = []checker.CheckResponse{}
	}
	return results
}

// saveCheckTemp 将检查结果转为 TempCheckResult 并写入缓存。
func saveCheckTemp(home, typ string, results []checker.CheckResponse) {
	var temp []store.TempCheckResult
	for _, r := range results {
		platforms := make(map[string]store.TempCheckPlatform)
		for os, p := range r.Platforms {
			platforms[os] = store.TempCheckPlatform{
				CurrentVersion: p.CurrentVersion,
				LatestVersion:  p.LatestVersion,
				URL:            p.URL,
				Error:          p.Error,
			}
		}
		temp = append(temp, store.TempCheckResult{
			AppID:     r.AppID,
			Name:      r.Name,
			Platforms: platforms,
		})
	}
	_ = store.SaveCheckTemp(home, typ, temp)
}

// ── GET /api/check/temp/{type} ──

func (s *Server) handleCheckTemp(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "GET required")
		return
	}
	typ := r.PathValue("type")
	if typ != "ids" && typ != "all" && typ != "tracker" {
		writeError(w, http.StatusBadRequest, "invalid type: must be ids, all, or tracker")
		return
	}
	results, err := store.LoadLatestCheckTemp(s.home, typ)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if results == nil {
		results = []store.TempCheckResult{}
	}
	writeJSON(w, http.StatusOK, results)
}
