// Check API handlers
package server

import (
	"encoding/json"
	"net/http"
	"strconv"
	"sync"

	"github.com/vanadiry/serein/core/checker"
	"github.com/vanadiry/serein/core/store"
)

// POST /api/check/all

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
	if len(entries) == 0 {
		writeJSON(w, http.StatusOK, map[string]string{"task_id": ""})
		return
	}
	checker.ClearURLCache()
	p := store.NewProgress(len(entries))
	go s.runTrackerChecksAsync(entries, p, "all")
	writeJSON(w, http.StatusOK, map[string]string{"task_id": p.ID, "total": strconv.Itoa(len(entries))})
}

// POST /api/check/ids

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
	if len(entries) == 0 {
		writeJSON(w, http.StatusOK, map[string]string{"task_id": ""})
		return
	}
	// 单个检查直接返回，避免 SSE 竞态
	if len(entries) <= 1 {
		store.Logf("[check/ids/direct] %v", ids)
		checker.ClearURLCache()
		result := s.runTrackerChecksSync(entries)
		saveCheckTemp(s.home, "ids", result)
		writeJSON(w, http.StatusOK, result)
		return
	}
	store.Logf("[check/ids] %v", ids)
	checker.ClearURLCache()
	p := store.NewProgress(len(entries))
	go s.runTrackerChecksAsync(entries, p, "ids")
	writeJSON(w, http.StatusOK, map[string]string{"task_id": p.ID, "total": strconv.Itoa(len(entries))})
}

// POST /api/check/tracker

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
	if len(entries) == 0 {
		writeJSON(w, http.StatusOK, map[string]string{"task_id": ""})
		return
	}
	store.Logf("[check/tracker] %s", body.TrackerID)
	checker.ClearURLCache()
	p := store.NewProgress(len(entries))
	go s.runTrackerChecksAsync(entries, p, "tracker")
	writeJSON(w, http.StatusOK, map[string]string{"task_id": p.ID, "total": strconv.Itoa(len(entries))})
}

// POST /api/check/confirm

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
		"app_id":    appID,
		"status":    "ok",
		"platforms": userData[appID],
	})
}

// 异步检查（后台 goroutine，通过 SSE 推送进度）

type checkJob struct {
	req  checker.CheckRequest
	name string
}

func (s *Server) buildCheckJobs(entries []store.TrackerEntry) ([]checkJob, int) {
	cfg, _ := store.LoadConfig(s.home)
	rules, _ := store.LoadRules(s.home)
	userData, _ := store.LoadUserData(s.home)

	conc := cfg.Serein.Concurrency
	if conc < 1 {
		conc = 1
	}
	if conc > 64 {
		conc = 64
	}

	var jobs []checkJob
	for _, entry := range entries {
		rule, ok := rules[entry.AppID]
		if !ok {
			continue
		}
		jobName := rule.Info.Name
		platforms := store.PlatformsFor(entry, cfg.Serein.Platforms)

		var platCfgs []checker.PlatformCheckConfig
		for _, os := range platforms {
			platCfg := rule.MergedConfig(os)

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

			if platCfg.URL == "" && platCfg.VURL == "" && platCfg.DURL == "" && platCfg.Type != "github" {
				continue
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
				VURL:           platCfg.VURL,
				VType:          platCfg.VType,
				DURL:           platCfg.DURL,
				DType:          platCfg.DType,
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

		typeGroups := make(map[string][]checker.PlatformCheckConfig)
		for _, pc := range platCfgs {
			typeGroups[pc.Type] = append(typeGroups[pc.Type], pc)
		}
		for typ, group := range typeGroups {
			jobs = append(jobs, checkJob{req: checker.CheckRequest{
				AppID:           rule.Info.AppID,
				Name:            rule.Info.Name,
				OfficialWebsite: rule.Info.OfficialWebsite,
				RuleType:        typ,
				Owner:           rule.Config.Owner,
				Repo:            rule.Config.Repo,
				GithubToken:     cfg.Serein.GithubToken,
				Platforms:       group,
			}, name: jobName})
		}
	}
	return jobs, conc
}

func runChecksSync(jobs []checkJob, conc int) []checker.CheckResponse {
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
		return mergeResults(results)
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
	return mergeResults(results)
}

func (s *Server) runTrackerChecksSync(entries []store.TrackerEntry) []checker.CheckResponse {
	jobs, conc := s.buildCheckJobs(entries)
	return runChecksSync(jobs, conc)
}

func (s *Server) runTrackerChecksAsync(entries []store.TrackerEntry, p *store.Progress, tempType string) {
	defer p.Close()

	jobs, conc := s.buildCheckJobs(entries)

	total := len(jobs)
	if total == 0 {
		return
	}

	sem := make(chan struct{}, conc)
	var mu sync.Mutex
	var results []checker.CheckResponse
	var doneCount int
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
			doneCount++
			current := doneCount
			name := j.name
			mu.Unlock()
			p.Send("app", name, current, total)
		}(job)
	}
	wg.Wait()

	final := mergeResults(results)
	saveCheckTemp(s.home, tempType, final)
}

func mergeResults(results []checker.CheckResponse) []checker.CheckResponse {
	merged := make(map[string]*checker.CheckResponse)
	for i := range results {
		r := &results[i]
		if existing, ok := merged[r.AppID]; ok {
			for os, pl := range r.Platforms {
				existing.Platforms[os] = pl
			}
		} else {
			merged[r.AppID] = r
		}
	}
	var final []checker.CheckResponse
	for _, r := range merged {
		final = append(final, *r)
	}
	if final == nil {
		final = []checker.CheckResponse{}
	}
	return final
}

// GET /api/check/temp/{type}

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

// 辅助

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
