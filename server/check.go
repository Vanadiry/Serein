// Check API handlers
package server

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"strconv"
	"sync"
	"time"

	"github.com/vanadiry/serein/core/checker"
	"github.com/vanadiry/serein/core/store"
)

// POST /api/check/ids

func (s *Server) handleCheckIDs(w http.ResponseWriter, r *http.Request) {
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
		writeJSON(w, http.StatusOK, []checker.CheckResponse{})
		return
	}
	checker.ClearURLCache()
	jobs, _ := s.buildCheckJobs(entries)
	var results []checker.CheckResponse
	for _, job := range jobs {
		resp, err := checker.RunCheck(job.req)
		if err != nil {
			store.Emit("error", "[check]", fmt.Sprintf("%s: %v", job.name, err))
			continue
		}
		results = append(results, resp)
	}
	if results == nil {
		results = []checker.CheckResponse{}
	}
	writeJSON(w, http.StatusOK, results)
}

// POST /api/check/tracker

func (s *Server) handleCheckTracker(w http.ResponseWriter, r *http.Request) {
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
	go s.runTrackerChecksAsync(entries, p, body.TrackerID)
	writeJSON(w, http.StatusOK, map[string]string{"task_id": p.ID, "total": strconv.Itoa(len(entries))})
}

// POST /api/check/confirm

func (s *Server) handleCheckConfirm(w http.ResponseWriter, r *http.Request) {
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
	userData[appID]["_confirmed_at"] = strconv.FormatInt(time.Now().UTC().Unix(), 10)
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
	cfg, cfgErr := store.LoadConfig(s.home)
	if cfgErr != nil {
		store.Emit("error", "[check]", fmt.Sprintf("加载配置失败: %v", cfgErr))
	}
	rules, rulesErr := store.LoadRules(s.home)
	if rulesErr != nil {
		store.Emit("error", "[check]", fmt.Sprintf("加载规则失败: %v", rulesErr))
	}
	userData, udErr := store.LoadUserData(s.home)
	if udErr != nil {
		store.Emit("error", "[check]", fmt.Sprintf("加载用户数据失败: %v", udErr))
	}

	conc := store.ClampConcurrency(cfg.Download.Concurrency)

	var jobs []checkJob
	for _, entry := range entries {
		rule, ok := rules[entry.AppID]
		if !ok {
			continue
		}
		jobName := rule.Info.Name
		platforms := store.PlatformsFor(entry, cfg.Tracker.Platforms)

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
				if err != nil {
					store.Emit("error", "[check]", fmt.Sprintf("%s 前置请求失败: %v", jobName, err))
				} else if preURL != "" {
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
				OS:              os,
				Type:            platCfg.Type,
				URL:             platCfg.URL,
				UA:              platCfg.UA,
				Headers:         platCfg.Headers,
				BaseURL:         platCfg.BaseURL,
				VURL:            platCfg.VURL,
				VType:           platCfg.VType,
				DURL:            platCfg.DURL,
				DType:           platCfg.DType,
				VPosition:       platCfg.VPosition,
				DPosition:       platCfg.DPosition,
				VJoin:           platCfg.VJoin,
				DJoin:           platCfg.DJoin,
				CurrentVersion:  currentVer,
				ForceDownloader: platCfg.ForceDownloader,
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
				GithubToken:     cfg.Access.GithubToken,
				Platforms:       group,
			}, name: jobName})
		}
	}
	return jobs, conc
}

func (s *Server) runTrackerChecksAsync(entries []store.TrackerEntry, p *store.Progress, trackerID string) {
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
				store.Emit("error", "[check]", fmt.Sprintf("%s: %v", j.name, err))
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
	saveCheckTemp(s.home, trackerID, final)
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

// GET /api/check/temp/{tracker_id}

func (s *Server) handleCheckTemp(w http.ResponseWriter, r *http.Request) {
	trackerID := r.PathValue("type")
	if trackerID == "" {
		writeError(w, http.StatusBadRequest, "missing tracker_id")
		return
	}
	cache, err := store.LoadTrackerTemp(s.home, trackerID)
	if err != nil {
		if os.IsNotExist(err) {
			writeJSON(w, http.StatusOK, map[string]any{"results": []store.TempCheckResult{}, "expired": false})
			return
		}
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	expired := store.IsExpired(cache)
	writeJSON(w, http.StatusOK, map[string]any{
		"results": cache.Results,
		"expired": expired,
	})
}

// 辅助

func saveCheckTemp(home, trackerID string, results []checker.CheckResponse) {
	var temp []store.TempCheckResult
	for _, r := range results {
		platforms := make(map[string]store.TempCheckPlatform)
		for os, p := range r.Platforms {
			platforms[os] = store.TempCheckPlatform{
				CurrentVersion:  p.CurrentVersion,
				LatestVersion:   p.LatestVersion,
				URL:             p.URL,
				Error:           p.Error,
				ForceDownloader: p.ForceDownloader,
			}
		}
		temp = append(temp, store.TempCheckResult{
			AppID:     r.AppID,
			Name:      r.Name,
			Platforms: platforms,
		})
	}
	if err := store.SaveTrackerTemp(home, trackerID, temp); err != nil {
		store.Emit("error", "[check]", fmt.Sprintf("保存检查结果缓存失败: %v", err))
	}
}
