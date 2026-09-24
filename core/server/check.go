// Check API handlers
package server

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"sort"
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

// POST /api/check
func (s *Server) handleCheck(w http.ResponseWriter, r *http.Request) {
	limitBody(w, r)
	var body struct {
		Scope      string              `json:"scope"`
		TrackerIDs []string            `json:"tracker_ids"`
		IDs        map[string][]string `json:"ids"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid body")
		return
	}

	var list []checkAllTracker
	savePartial := true // 指定范围检查时，取消也保留已完成部分
	switch body.Scope {
	case "all":
		list = s.buildCheckList(nil, nil)
		savePartial = false // 全部检查：取消不覆盖未完成桶
	case "trackers":
		if len(body.TrackerIDs) == 0 {
			writeError(w, http.StatusBadRequest, "missing tracker_ids")
			return
		}
		for _, id := range body.TrackerIDs {
			if !store.ValidTrackerName(id) {
				writeError(w, http.StatusBadRequest, "invalid tracker_id: "+id)
				return
			}
		}
		list = s.buildCheckList(body.TrackerIDs, nil)
	case "ids":
		if len(body.IDs) == 0 {
			writeError(w, http.StatusBadRequest, "missing ids")
			return
		}
		for id := range body.IDs {
			if !store.ValidTrackerName(id) {
				writeError(w, http.StatusBadRequest, "invalid tracker_id: "+id)
				return
			}
		}
		list = s.buildCheckList(nil, body.IDs)
	default:
		writeError(w, http.StatusBadRequest, "invalid scope: must be all|trackers|ids")
		return
	}
	s.startCheckList(w, list, savePartial)
}

// startCheckList 为一个 tracker 列表启动异步检查任务（total 为条目数）
func (s *Server) startCheckList(w http.ResponseWriter, list []checkAllTracker, savePartial bool) {
	total := 0
	for _, t := range list {
		total += len(t.entries)
	}
	if total == 0 {
		writeJSON(w, http.StatusOK, map[string]string{"task_id": "", "total": "0"})
		return
	}
	log.Logf("[check] %d trackers, %d apps (savePartial=%v)", len(list), total, savePartial)
	httpx.ClearURLCache()
	p := progress.NewProgress(total)
	go s.runCheckAllAsync(list, p, savePartial)
	writeJSON(w, http.StatusOK, map[string]string{"task_id": p.ID, "total": strconv.Itoa(total)})
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
				Owner:           platCfg.Owner,
				Repo:            platCfg.Repo,
				PerPage:         platCfg.PerPage,
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
				AllowPrerelease: platCfg.AllowPrerelease,
				Label:           jobName,
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

type checkAllTracker struct {
	id      string
	name    string
	typ     string
	entries []store.TrackerEntry
}

// buildCheckList 构造检查列表
func (s *Server) buildCheckList(trackerIDs []string, idFilter map[string][]string) []checkAllTracker {
	infos, err := store.LoadAllTrackerInfo(s.home)
	if err != nil {
		return nil
	}
	byID := make(map[string]store.TrackerInfo, len(infos))
	for _, ti := range infos {
		byID[ti.ID] = ti
	}

	var order []string
	switch {
	case len(idFilter) > 0:
		for id := range idFilter {
			order = append(order, id)
		}
		sort.Strings(order)
	case len(trackerIDs) > 0:
		seen := make(map[string]bool, len(trackerIDs))
		for _, id := range trackerIDs {
			if !seen[id] {
				seen[id] = true
				order = append(order, id)
			}
		}
	default:
		for _, ti := range infos {
			order = append(order, ti.ID)
		}
	}

	var list []checkAllTracker
	for _, id := range order {
		ti, ok := byID[id]
		if !ok {
			continue
		}
		typ := ti.Type
		if typ == "" {
			typ = "app"
		}
		all, err := store.LoadTrackerFile(s.home, id)
		if err != nil {
			continue
		}
		var entries []store.TrackerEntry
		if idFilter != nil {
			if typ == "msvsix" || typ == "openvsx" {
				// direct：按给定 id 直接检查，不要求在 Tracker 中
				for _, x := range idFilter[id] {
					entries = append(entries, store.TrackerEntry{AppID: x})
				}
			} else {
				want := make(map[string]bool, len(idFilter[id]))
				for _, x := range idFilter[id] {
					want[x] = true
				}
				for _, e := range all {
					if want[e.AppID] {
						entries = append(entries, e)
					}
				}
			}
		} else {
			entries = all
		}
		if len(entries) == 0 {
			continue
		}
		list = append(list, checkAllTracker{id: ti.ID, name: ti.DisplayName, typ: typ, entries: entries})
	}
	return list
}

func (s *Server) runCheckAllAsync(list []checkAllTracker, p *progress.Progress, savePartial bool) {
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

		// savePartial：单个/指定 Tracker 检查时，取消也保留已完成的部分
		// 全部检查时不覆盖未完成的桶
		if ctx.Err() == nil || savePartial {
			s.setCheckResults(t.id, mergeResults(results))
		}
		if ctx.Err() != nil {
			break
		}
	}
}
