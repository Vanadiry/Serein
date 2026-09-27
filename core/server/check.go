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
	replace := true     // 整表检查：整桶替换；单条/按 id 检查改为 upsert
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
		replace = false // 只 upsert，保留该桶里其它 app 的结果
	default:
		writeError(w, http.StatusBadRequest, "invalid scope: must be all|trackers|ids")
		return
	}
	s.startCheckList(w, list, savePartial, replace)
}

// startCheckList 为一个 tracker 列表启动异步检查任务（total 为条目数）
func (s *Server) startCheckList(w http.ResponseWriter, list []checkAllTracker, savePartial, replace bool) {
	total := 0
	for _, t := range list {
		total += len(t.entries)
	}
	if total == 0 {
		writeJSON(w, http.StatusOK, map[string]string{"task_id": "", "total": "0"})
		return
	}
	log.Logf("[check] %d trackers, %d apps (savePartial=%v, replace=%v)", len(list), total, savePartial, replace)
	httpx.ClearURLCache()
	p := progress.NewProgress(total)
	go s.runCheckAllAsync(list, p, savePartial, replace)
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
		writeError(w, http.StatusBadRequest, "missing app_id")
		return
	}

	// body 里除 app_id 外的都是「平台 → 版本号」
	versions := make(map[string]string, len(body))
	for k, v := range body {
		if k == "app_id" {
			continue
		}
		versions[k] = v
	}
	confirmed, err := s.confirmVersion(appID, versions)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	// 同步内存检查缓存里的 current_version，避免切 tab 时读回旧值、又重新显示更新箭头
	s.syncCachedCurrent(appID, versions)
	log.Logf("[confirm] %s: %v", appID, confirmed)
	writeJSON(w, http.StatusOK, map[string]any{
		"app_id":    appID,
		"status":    "ok",
		"platforms": confirmed,
	})
}

// cloneCheckPlatforms 深拷贝平台结果表；nil 进 nil 出（保持 JSON 形状不变）
func cloneCheckPlatforms(in map[string]checker.CheckPlatform) map[string]checker.CheckPlatform {
	if in == nil {
		return nil
	}
	out := make(map[string]checker.CheckPlatform, len(in))
	for k, v := range in {
		out[k] = v
	}
	return out
}

// syncCachedCurrent 把确认后的版本号写回所有包含该 app 的内存检查缓存
func (s *Server) syncCachedCurrent(appID string, versions map[string]string) {
	if len(versions) == 0 {
		return
	}
	s.resultsMu.Lock()
	defer s.resultsMu.Unlock()
	for _, bucket := range s.results {
		r, ok := bucket[appID]
		if !ok {
			continue
		}
		// 复制后整体换入，绝不原地修改：已发布的表可能正被 JSON 编码读取，
		// 而 Go 对 map 的并发读写是 fatal error（无法 recover）
		cp := cloneCheckPlatforms(r.Platforms)
		if cp == nil {
			cp = make(map[string]checker.CheckPlatform)
		}
		for os, v := range versions {
			p := cp[os]
			p.CurrentVersion = v
			cp[os] = p
		}
		r.Platforms = cp
		bucket[appID] = r
	}
}

// 异步检查（后台 goroutine，通过 SSE 推送进度）

// checkMaxErrors 单次检查返回的错误条数上限。超出后只累加计数，
// 由 done 事件里的 overflow 告知前端「还有更多」，避免超长响应。
// 声明为 var 以便测试覆盖
var checkMaxErrors = 2000

// CheckError 一次检查里的一条错误。随 done 事件返回给前端一次性展示，
// 不写入结果缓存——缓存存的是版本号与下载链接，不是错误。
type CheckError struct {
	Tracker string `json:"tracker,omitempty"` // 所属 Tracker，前端按它分组
	AppID   string `json:"app_id,omitempty"`
	Name    string `json:"name,omitempty"`
	OS      string `json:"os,omitempty"` // 空 = 整条目级或任务级
	Message string `json:"message"`
}

// checkErrs 收集一次检查的错误。四种 scope 共用 runCheckAllAsync，
// 所以收集逻辑只有这一份。
type checkErrs struct {
	mu       sync.Mutex
	items    []CheckError
	overflow int
}

func (c *checkErrs) add(e CheckError) {
	if e.Message == "" {
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if len(c.items) < checkMaxErrors {
		c.items = append(c.items, e)
	} else {
		c.overflow++
	}
}

// scoped 返回一个绑定到某个 Tracker 的 report，用于交给取数阶段的各函数
func (c *checkErrs) scoped(tracker string) func(CheckError) {
	return func(e CheckError) {
		if e.Tracker == "" {
			e.Tracker = tracker
		}
		c.add(e)
	}
}

func (c *checkErrs) snapshot() ([]CheckError, int) {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.items, c.overflow
}

// recordResultErrors 把一次取数结果里的错误与告警收进收集器。
// 直接遍历内存里的 results（不是结果缓存），所以天然只含本次的错误。
func recordResultErrors(ce *checkErrs, tracker string, results []checker.CheckResponse) {
	for _, r := range results {
		for os, cp := range r.Platforms {
			if cp.Error != "" {
				ce.add(CheckError{Tracker: tracker, AppID: r.AppID, Name: r.Name, OS: os, Message: cp.Error})
			}
			for _, w := range cp.Warnings {
				ce.add(CheckError{Tracker: tracker, AppID: r.AppID, Name: r.Name, OS: os, Message: w})
			}
		}
	}
}

type checkJob struct {
	req  checker.CheckRequest
	name string
}

// loadUserData 读「已确认安装到哪个版本」。
//
// 出错不中断：software.json 坏了不该让检查、搜索、列表整个打不开，用户还能把
// 留档文件手工改回来。LoadUserData 出错时也会返回可用的空数据，并且已经把问题
// 通过事件总线推给前端了，这里不用再报一遍。
func (s *Server) loadUserData() store.UserData {
	ud, _ := s.loadUserDataErr()
	return ud
}

func (s *Server) loadUserDataErr() (store.UserData, error) {
	s.userMu.RLock()
	defer s.userMu.RUnlock()
	return store.LoadUserData(s.home)
}

// confirmVersion 确认或修改某个 app 的版本号，返回该 app 确认后的平台版本。
//
// 整段 load-modify-save 都在写锁里。两个并发确认各自加载、各自保存，后写的会
// 用自己那份旧快照覆盖掉先写的那份——先确认的版本就永久丢了。UI 上「确认更新」
// 和「手动改版本」是两个入口，很容易连着点。
func (s *Server) confirmVersion(appID string, versions map[string]string) (map[string]string, error) {
	s.userMu.Lock()
	defer s.userMu.Unlock()

	ud, _ := store.LoadUserData(s.home)
	if ud[appID] == nil {
		ud[appID] = make(map[string]string)
	}
	for k, v := range versions {
		ud[appID][k] = v
	}
	ud[appID]["_confirmed_at"] = strconv.FormatInt(time.Now().UTC().Unix(), 10)
	if err := store.SaveUserData(s.home, ud); err != nil {
		return nil, err
	}
	return ud[appID], nil
}

func (s *Server) buildCheckJobs(ctx context.Context, entries []store.TrackerEntry, report func(CheckError)) ([]checkJob, int) {
	rules := s.getRules()
	userData, udErr := s.loadUserDataErr()
	if udErr != nil {
		report(CheckError{Message: fmt.Sprintf("加载用户数据失败: %v", udErr)})
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
						report(CheckError{Name: jobName, Message: fmt.Sprintf("%s 前置请求失败: %v", jobName, err)})
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
			platCfgs = append(platCfgs, checker.NewPlatformCheckConfig(os, jobName, currentVer, platCfg))
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
	// 与其它取值路径同一标准：尝试过却取不到就报错。
	// openvsx 的 files.download 缺失时 URL 会是空串，此前一路静默。
	if strings.TrimSpace(pr.LatestVersion) == "" {
		return checker.CheckResponse{}, fmt.Errorf("%s: 未取到版本号", typ)
	}
	if checker.URLEmpty(pr.URL) {
		return checker.CheckResponse{}, fmt.Errorf("%s: 未取到下载链接", typ)
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
				CurrentVersion:   currentVer,
				LatestVersion:    pr.LatestVersion,
				URL:              pr.URL,
				DownloadViaProxy: true,
			},
		},
	}, nil
}

// runAppJobs 并发执行 app 检查任务；每完成一个（含失败）调用 record(appID, name)
// 取消时返回已完成部分
// runConcurrent 以并发上限 conc 执行 items：run 成功收集结果；失败（非取消）交 onError
// 每个成功项交 record。ctx 取消则跳过未开始的任务
func runConcurrent[T any](
	ctx context.Context,
	items []T,
	conc int,
	run func(context.Context, T) (checker.CheckResponse, error),
	onError func(T, error),
	record func(T, checker.CheckResponse),
) []checker.CheckResponse {
	if conc < 1 {
		conc = 1
	}
	sem := make(chan struct{}, conc)
	var mu sync.Mutex
	var results []checker.CheckResponse
	var wg sync.WaitGroup
	for _, it := range items {
		wg.Add(1)
		go func(it T) {
			defer wg.Done()
			select {
			case <-ctx.Done():
				return
			case sem <- struct{}{}:
			}
			defer func() { <-sem }()
			resp, err := run(ctx, it)
			if err != nil {
				if errors.Is(err, context.Canceled) {
					return
				}
				onError(it, err)
				return
			}
			mu.Lock()
			results = append(results, resp)
			mu.Unlock()
			record(it, resp)
		}(it)
	}
	wg.Wait()
	return results
}

// runAppJobs 并发执行 app 检查任务；每完成一个（含失败）调用 record(appID, name)
func (s *Server) runAppJobs(ctx context.Context, jobs []checkJob, conc int, record func(appID, name string), report func(CheckError)) []checker.CheckResponse {
	return runConcurrent(ctx, jobs, conc,
		func(ctx context.Context, j checkJob) (checker.CheckResponse, error) {
			return checker.RunCheck(ctx, j.req)
		},
		func(j checkJob, err error) {
			report(CheckError{Name: j.name, Message: fmt.Sprintf("%s: %v", j.name, err)})
			record(j.req.AppID, j.name)
		},
		func(j checkJob, resp checker.CheckResponse) {
			record(resp.AppID, j.name)
		},
	)
}

// runDirectEntries 并发执行 direct（vsix）检查；每完成一个（含失败）调用 record(appID, name)
func (s *Server) runDirectEntries(ctx context.Context, entries []store.TrackerEntry, typ string, conc int, record func(appID, name string), report func(CheckError)) []checker.CheckResponse {
	checkFn := selectDirectCheckFn(typ)
	client := httpx.NewClient()
	userData := s.loadUserData()
	return runConcurrent(ctx, entries, conc,
		func(ctx context.Context, e store.TrackerEntry) (checker.CheckResponse, error) {
			return directCheckResponse(ctx, checkFn, client, e.AppID, typ, userData)
		},
		func(e store.TrackerEntry, err error) {
			report(CheckError{AppID: e.AppID, Name: e.AppID, Message: fmt.Sprintf("%s: %v", e.AppID, err)})
			record(e.AppID, e.AppID)
		},
		func(e store.TrackerEntry, resp checker.CheckResponse) {
			record(resp.AppID, e.AppID)
		},
	)
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

// attachProxyURL 为需要改名的平台补上签名后的下载代理地址（服务端统一处理）
func (s *Server) attachProxyURL(resp *checker.CheckResponse) {
	for os, cp := range resp.Platforms {
		if !cp.DownloadViaProxy || cp.URL == nil {
			continue
		}
		if u := s.signURLs(cp.URL, proxyName(resp, os, cp)); u != nil {
			cp.ProxyURL = u
			resp.Platforms[os] = cp
		}
	}
}

// proxyName 计算代理落盘名：VSIX/OpenVSX 用标准名，其余用 download_name（替换 {version}）
func proxyName(resp *checker.CheckResponse, os string, cp checker.CheckPlatform) string {
	if os == "msvsix" || os == "openvsx" {
		if cp.LatestVersion != "" {
			return fmt.Sprintf("%s-%s.vsix", resp.AppID, cp.LatestVersion)
		}
		return resp.AppID + ".vsix"
	}
	return strings.ReplaceAll(cp.DownloadName, "{version}", cp.LatestVersion)
}

// signURLs 镜像 URL 的形状（string / []string / []any），逐个生成签名代理地址
func (s *Server) signURLs(u any, name string) any {
	switch v := u.(type) {
	case string:
		if v == "" {
			return nil
		}
		return s.proxyURL(v, name)
	case []string:
		out := make([]string, 0, len(v))
		for _, x := range v {
			out = append(out, s.proxyURL(x, name))
		}
		if len(out) == 0 {
			return nil
		}
		return out
	case []any:
		out := make([]string, 0, len(v))
		for _, x := range v {
			if str, ok := x.(string); ok {
				out = append(out, s.proxyURL(str, name))
			}
		}
		if len(out) == 0 {
			return nil
		}
		return out
	}
	return nil
}

// GET /api/check/temp/{tracker_id}

func (s *Server) handleCheckTemp(w http.ResponseWriter, r *http.Request) {
	trackerID := r.PathValue("tracker_id")
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

// 检查结果缓存：按 Tracker（或 direct 的 type）分桶，内层按 app_id；随进程释放。
// 不变式：桶内的 Platforms 表发布后不再原地修改，读写两侧都做深拷贝。

func (s *Server) setCheckResults(key string, results []checker.CheckResponse) {
	m := make(map[string]checker.CheckResponse, len(results))
	for _, r := range results {
		r.Platforms = cloneCheckPlatforms(r.Platforms)
		m[r.AppID] = r
	}
	s.resultsMu.Lock()
	s.results[key] = m
	s.resultsMu.Unlock()
}

func (s *Server) setCheckResult(key string, r checker.CheckResponse) {
	r.Platforms = cloneCheckPlatforms(r.Platforms)
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
		// 必须在锁内深拷贝：调用方会在锁外用 json.Encoder 遍历这些表
		r.Platforms = cloneCheckPlatforms(r.Platforms)
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

func (s *Server) runCheckAllAsync(list []checkAllTracker, p *progress.Progress, savePartial, replace bool) {
	// 错误随 done 事件一次性返回：收集器只是本次调用的局部状态，
	// 不注册、不出接口、不写入结果缓存。
	ce := &checkErrs{}
	defer func() {
		items, overflow := ce.snapshot()
		if items == nil {
			items = []CheckError{}
		}
		p.SetFinalEvent(map[string]any{"errors": items, "overflow": overflow})
		p.Close()
	}()
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

		report := ce.scoped(t.name)
		var results []checker.CheckResponse
		if t.typ == "msvsix" || t.typ == "openvsx" {
			results = s.runDirectEntries(ctx, t.entries, t.typ, conc, record, report)
		} else {
			jobs, _ := s.buildCheckJobs(ctx, t.entries, report)
			results = s.runAppJobs(ctx, jobs, conc, record, report)
		}
		// per-platform 错误与告警在此收集，**不能挂在提交阶段**：
		// 取消时提交不执行，挂在那里会一条都收不上。遍历的是内存里的 results，
		// 不是结果缓存，所以天然只含本次的错误。
		recordResultErrors(ce, t.name, results)

		// savePartial：指定范围检查时，取消也保留已完成的部分；全部检查时不覆盖未完成的桶
		// replace：整表检查整桶替换；单条/按 id 检查只 upsert（保留该桶其它 app 的结果）
		if ctx.Err() == nil || savePartial {
			merged := mergeResults(results)
			for i := range merged {
				s.attachProxyURL(&merged[i])
			}
			switch {
			case !replace:
				// 单条 / 按 id：只覆盖本次查到的 app
				for _, r := range merged {
					s.setCheckResult(t.id, r)
				}
			case len(merged) == 0:
				// 跑了但一个结果都没有：规则全被过滤（removed / 变量缺失 / 无可用平台）、
				// 或全部整条目失败。此时整桶替换成空 = 未检查的 app 看起来像「无更新」，
				// 而实际是「没查到」。保留旧结果，错误由本次的汇总弹窗报出。
				log.LogfWarn("[check] %s 未产出任何结果，保留原缓存", t.id)
			case savePartial && ctx.Err() != nil:
				// 取消：只 upsert 已完成的部分，其余 app 保留历史结果。
				// 整桶替换会把未检查的 150 个 app 一起清掉，侧栏全部回退成「有更新」。
				for _, r := range merged {
					s.setCheckResult(t.id, r)
				}
			default:
				s.setCheckResults(t.id, merged)
			}
		}
		if ctx.Err() != nil {
			break
		}
	}
}
