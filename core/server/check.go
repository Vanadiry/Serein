// Check API handlers
package server

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/vanadiry/serein/core/checker"
	"github.com/vanadiry/serein/core/events"
	"github.com/vanadiry/serein/core/httpx"
	"github.com/vanadiry/serein/core/log"
	"github.com/vanadiry/serein/core/progress"
	"github.com/vanadiry/serein/core/store"
)

// POST /api/check/ids

func (s *Server) handleCheckIDs(w http.ResponseWriter, r *http.Request) {
	limitBody(w, r)
	var body struct {
		Type      string   `json:"type"`
		TrackerID string   `json:"tracker_id"`
		IDs       []string `json:"ids"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid body")
		return
	}
	if body.Type == "" {
		body.Type = "app"
	}
	ids := body.IDs

	if body.Type == "msvsix" || body.Type == "openvsx" {
		s.handleDirectCheckIDs(r.Context(), w, ids, body.Type)
		return
	}

	// 指定 tracker_id 时只在该 Tracker 记录内检查，平台以 Tracker 为准，否则回退到全量
	var allEntries []store.TrackerEntry
	if body.TrackerID != "" {
		if !store.ValidTrackerName(body.TrackerID) {
			writeError(w, http.StatusBadRequest, "invalid tracker_id")
			return
		}
		entries, err := store.LoadTrackerFile(s.home, body.TrackerID)
		if err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
		allEntries = entries
	} else {
		entries, err := store.LoadTracker(s.home)
		if err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
		allEntries = entries
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
	httpx.ClearURLCache()
	jobs, _ := s.buildCheckJobs(r.Context(), entries)
	var results []checker.CheckResponse
	for _, job := range jobs {
		resp, err := checker.RunCheck(r.Context(), job.req)
		if err != nil {
			if !errors.Is(err, context.Canceled) {
				events.Emit("error", "[check]", fmt.Sprintf("%s: %v", job.name, err))
			}
			continue
		}
		results = append(results, resp)
	}
	if results == nil {
		results = []checker.CheckResponse{}
	}
	// 单条/按 id 检查的结果并入该 Tracker 的缓存
	if body.TrackerID != "" {
		for _, r := range results {
			s.setCheckResult(body.TrackerID, r)
		}
	}
	writeJSON(w, http.StatusOK, results)
}

// POST /api/check/tracker

func (s *Server) handleCheckTracker(w http.ResponseWriter, r *http.Request) {
	limitBody(w, r)
	var body struct {
		TrackerID string `json:"tracker_id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.TrackerID == "" {
		writeError(w, http.StatusBadRequest, "missing tracker_id")
		return
	}
	if !store.ValidTrackerName(body.TrackerID) {
		writeError(w, http.StatusBadRequest, "invalid tracker_id")
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
	if store.GetTrackerType(s.home, body.TrackerID) == "msvsix" {
		s.handleDirectCheck(w, entries, "msvsix")
		return
	}
	if store.GetTrackerType(s.home, body.TrackerID) == "openvsx" {
		s.handleDirectCheck(w, entries, "openvsx")
		return
	}
	log.Logf("[check/tracker] %s", body.TrackerID)
	httpx.ClearURLCache()
	p := progress.NewProgress(len(entries))
	go s.runTrackerChecksAsync(entries, p, body.TrackerID)
	writeJSON(w, http.StatusOK, map[string]string{"task_id": p.ID, "total": strconv.Itoa(len(entries))})
}

// POST /api/check/confirm

func (s *Server) handleCheckConfirm(w http.ResponseWriter, r *http.Request) {
	limitBody(w, r)
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
	log.Logf("[confirm] %s: %v", appID, userData[appID])
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

func (s *Server) buildCheckJobs(ctx context.Context, entries []store.TrackerEntry) ([]checkJob, int) {
	rules := s.getRules()
	userData, udErr := store.LoadUserData(s.home)
	if udErr != nil {
		events.Emit("error", "[check]", fmt.Sprintf("加载用户数据失败: %v", udErr))
	}

	conc := s.config.Download.Concurrency

	var jobs []checkJob
	for _, entry := range entries {
		rule, ok := rules[entry.AppID]
		if !ok {
			continue
		}
		if rule.Status.Level == "removed" {
			continue
		}
		if len(rule.MissingValues) > 0 {
			continue
		}
		jobName := rule.Info.Name
		platforms := store.PlatformsFor(entry, s.config.Tracker.Platforms)

		var platCfgs []checker.PlatformCheckConfig
		for _, os := range platforms {
			platCfg := rule.MergedConfig(os)

			preSteps := rule.PreRequestChain(os)
			if len(preSteps) > 0 {
				preURL, err := checker.RunPreRequests(ctx, preSteps, httpx.NewClient())
				if err != nil {
					if !errors.Is(err, context.Canceled) {
						events.Emit("error", "[check]", fmt.Sprintf("%s 前置请求失败: %v", jobName, err))
					}
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
				PerPage:         rule.Config.PerPage,
				Platforms:       group,
			}, name: jobName})
		}
	}
	return jobs, conc
}

// selectDirectCheckFn 返回 typ 对应的 direct（vsix）检查函数
func selectDirectCheckFn(typ string) func(context.Context, string, *http.Client) (checker.PlatformResult, error) {
	if typ == "openvsx" {
		return checker.CheckOpenVSX
	}
	return checker.CheckMSVSIX
}

// directCheckResponse 执行一次 direct 检查并组装 CheckResponse
func directCheckResponse(ctx context.Context, checkFn func(context.Context, string, *http.Client) (checker.PlatformResult, error), client *http.Client, appID, typ string, userData store.UserData) (checker.CheckResponse, error) {
	pr, err := checkFn(ctx, appID, client)
	if err != nil {
		return checker.CheckResponse{}, err
	}
	currentVer := ""
	if ud, ok := userData[appID]; ok {
		currentVer = ud[typ]
	}
	return checker.CheckResponse{
		AppID: appID,
		Name:  appID,
		Platforms: map[string]checker.CheckPlatform{
			typ: {
				CurrentVersion:  currentVer,
				LatestVersion:   pr.LatestVersion,
				URL:             pr.URL,
				ForceDownloader: true,
			},
		},
	}, nil
}

// runAppJobs 并发执行 app 检查任务；每完成一个（含失败）调用 record(appID, name)
// 取消时返回已完成部分
func (s *Server) runAppJobs(ctx context.Context, jobs []checkJob, conc int, record func(appID, name string)) []checker.CheckResponse {
	if conc < 1 {
		conc = 1
	}
	sem := make(chan struct{}, conc)
	var mu sync.Mutex
	var results []checker.CheckResponse
	var wg sync.WaitGroup
	for _, job := range jobs {
		wg.Add(1)
		go func(j checkJob) {
			defer wg.Done()
			select {
			case <-ctx.Done():
				return
			case sem <- struct{}{}:
			}
			defer func() { <-sem }()
			resp, err := checker.RunCheck(ctx, j.req)
			if err != nil {
				if errors.Is(err, context.Canceled) {
					return
				}
				events.Emit("error", "[check]", fmt.Sprintf("%s: %v", j.name, err))
				record(j.req.AppID, j.name)
				return
			}
			mu.Lock()
			results = append(results, resp)
			mu.Unlock()
			record(resp.AppID, j.name)
		}(job)
	}
	wg.Wait()
	return results
}

// runDirectEntries 并发执行 direct（vsix）检查；每完成一个（含失败）调用 record(appID, name)
func (s *Server) runDirectEntries(ctx context.Context, entries []store.TrackerEntry, typ string, conc int, record func(appID, name string)) []checker.CheckResponse {
	if conc < 1 {
		conc = 1
	}
	checkFn := selectDirectCheckFn(typ)
	client := httpx.NewClient()
	userData, _ := store.LoadUserData(s.home)
	sem := make(chan struct{}, conc)
	var mu sync.Mutex
	var results []checker.CheckResponse
	var wg sync.WaitGroup
	for _, e := range entries {
		wg.Add(1)
		go func(e store.TrackerEntry) {
			defer wg.Done()
			select {
			case <-ctx.Done():
				return
			case sem <- struct{}{}:
			}
			defer func() { <-sem }()
			resp, err := directCheckResponse(ctx, checkFn, client, e.AppID, typ, userData)
			if err != nil {
				if errors.Is(err, context.Canceled) {
					return
				}
				events.Emit("error", "["+typ+"]", fmt.Sprintf("%s: %v", e.AppID, err))
				record(e.AppID, e.AppID)
				return
			}
			mu.Lock()
			results = append(results, resp)
			mu.Unlock()
			record(resp.AppID, e.AppID)
		}(e)
	}
	wg.Wait()
	return results
}

func (s *Server) runTrackerChecksAsync(entries []store.TrackerEntry, p *progress.Progress, trackerID string) {
	defer p.Close()

	ctx := p.Context()
	jobs, conc := s.buildCheckJobs(ctx, entries)
	total := len(jobs)
	if total == 0 {
		return
	}

	var mu sync.Mutex
	var done int
	record := func(_, name string) {
		mu.Lock()
		done++
		d := done
		mu.Unlock()
		p.Send("app", name, d, total)
	}

	results := s.runAppJobs(ctx, jobs, conc, record)
	s.setCheckResults(trackerID, mergeResults(results))
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
	if strings.ContainsAny(trackerID, "/\\") {
		writeError(w, http.StatusBadRequest, "invalid tracker_id")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"results": s.getCheckResults(trackerID)})
}

// 检查结果缓存：按 Tracker（或 direct 的 type）分桶，内层按 app_id；随进程释放

func (s *Server) setCheckResults(key string, results []checker.CheckResponse) {
	m := make(map[string]checker.CheckResponse, len(results))
	for _, r := range results {
		m[r.AppID] = r
	}
	s.resultsMu.Lock()
	s.results[key] = m
	s.resultsMu.Unlock()
}

func (s *Server) setCheckResult(key string, r checker.CheckResponse) {
	s.resultsMu.Lock()
	if s.results[key] == nil {
		s.results[key] = make(map[string]checker.CheckResponse)
	}
	s.results[key][r.AppID] = r
	s.resultsMu.Unlock()
}

func (s *Server) getCheckResults(key string) []checker.CheckResponse {
	s.resultsMu.RLock()
	defer s.resultsMu.RUnlock()
	out := make([]checker.CheckResponse, 0, len(s.results[key]))
	for _, r := range s.results[key] {
		out = append(out, r)
	}
	return out
}

func (s *Server) handleDirectCheck(w http.ResponseWriter, entries []store.TrackerEntry, typ string) {
	log.Logf("[check/%s] %d entries", typ, len(entries))
	httpx.ClearURLCache()
	p := progress.NewProgress(len(entries))
	go s.runDirectChecksAsync(entries, p, typ)
	writeJSON(w, http.StatusOK, map[string]string{"task_id": p.ID, "total": strconv.Itoa(len(entries))})
}

func (s *Server) handleDirectCheckIDs(ctx context.Context, w http.ResponseWriter, ids []string, typ string) {
	checkFn := selectDirectCheckFn(typ)
	client := httpx.NewClient()
	userData, _ := store.LoadUserData(s.home)
	var results []checker.CheckResponse
	for _, id := range ids {
		resp, err := directCheckResponse(ctx, checkFn, client, id, typ, userData)
		if err != nil {
			if !errors.Is(err, context.Canceled) {
				events.Emit("error", "["+typ+"]", fmt.Sprintf("%s: %v", id, err))
			}
			continue
		}
		results = append(results, resp)
	}
	if results == nil {
		results = []checker.CheckResponse{}
	}
	// 单条/按 id 检查的结果并入该 type 的缓存
	for _, r := range results {
		s.setCheckResult(typ, r)
	}
	writeJSON(w, http.StatusOK, results)
}

func (s *Server) runDirectChecksAsync(entries []store.TrackerEntry, p *progress.Progress, typ string) {
	defer p.Close()

	ctx := p.Context()
	total := len(entries)
	var mu sync.Mutex
	var done int
	record := func(_, name string) {
		mu.Lock()
		done++
		d := done
		mu.Unlock()
		p.Send("app", name, d, total)
	}

	results := s.runDirectEntries(ctx, entries, typ, 4, record)
	s.setCheckResults(typ, mergeResults(results))
}

// POST /api/check/all：一次性检查所有 Tracker

type checkAllTracker struct {
	id      string
	name    string
	typ     string
	entries []store.TrackerEntry
}

func (s *Server) handleCheckAll(w http.ResponseWriter, r *http.Request) {
	limitBody(w, r)
	infos, err := store.LoadAllTrackerInfo(s.home)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	var list []checkAllTracker
	total := 0
	for _, ti := range infos {
		entries, err := store.LoadTrackerFile(s.home, ti.ID)
		if err != nil || len(entries) == 0 {
			continue
		}
		typ := ti.Type
		if typ == "" {
			typ = "app"
		}
		list = append(list, checkAllTracker{id: ti.ID, name: ti.DisplayName, typ: typ, entries: entries})
		total += len(entries)
	}
	if total == 0 {
		writeJSON(w, http.StatusOK, map[string]string{"task_id": "", "total": "0"})
		return
	}
	log.Logf("[check/all] %d trackers, %d apps", len(list), total)
	httpx.ClearURLCache()
	p := progress.NewProgress(total)
	go s.runCheckAllAsync(list, p)
	writeJSON(w, http.StatusOK, map[string]string{"task_id": p.ID, "total": strconv.Itoa(total)})
}

func (s *Server) runCheckAllAsync(list []checkAllTracker, p *progress.Progress) {
	defer p.Close()
	ctx := p.Context()
	conc := s.config.Download.Concurrency
	if conc < 1 {
		conc = 1
	}
	totalApps := p.Total
	doneApps := 0

	for _, t := range list {
		if ctx.Err() != nil {
			break
		}
		seen := make(map[string]bool)
		var recMu sync.Mutex
		record := func(appID, name string) {
			recMu.Lock()
			if !seen[appID] {
				seen[appID] = true
				doneApps++
			}
			d := doneApps
			recMu.Unlock()
			p.Send("app", t.name+"："+name, d, totalApps)
		}

		var results []checker.CheckResponse
		if t.typ == "msvsix" || t.typ == "openvsx" {
			results = s.runDirectEntries(ctx, t.entries, t.typ, conc, record)
		} else {
			jobs, _ := s.buildCheckJobs(ctx, t.entries)
			results = s.runAppJobs(ctx, jobs, conc, record)
		}

		if ctx.Err() != nil {
			// 取消：只写已完成的桶，不覆盖未完成桶
			break
		}
		s.setCheckResults(t.id, mergeResults(results))
	}
}
