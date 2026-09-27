package store

import (
	"bytes"
	"encoding/json"
	"fmt"
	"hash/fnv"
	"os"
	"path/filepath"
	"reflect"
	"regexp"
	"sort"
	"strconv"
	"strings"

	"github.com/vanadiry/serein/core/httpx"

	"github.com/BurntSushi/toml"
)

// 基础类型

type RuleInfo struct {
	AppID           string   `toml:"app_id"`
	Name            string   `toml:"name"`
	Description     string   `toml:"description,omitempty"`
	OfficialWebsite string   `toml:"official_website,omitempty"`
	Status          []string `toml:"status,omitempty"`
	Platforms       []string `toml:"platforms"`
}

// RuleStatus 规则状态：消息 + 可选等级（warn | error | removed）
type RuleStatus struct {
	Message string `json:"message"`
	Level   string `json:"level,omitempty"`
}

var validStatusLevels = map[string]bool{"warn": true, "error": true, "removed": true}

// ParseRuleStatus 解析 status 数组：下标 0 恒为消息，下标 1（可选）为等级
// 返回解析结果，以及等级是否未知（未知时按 warn 处理）
func ParseRuleStatus(s []string) (RuleStatus, bool) {
	var st RuleStatus
	if len(s) > 0 {
		st.Message = s[0]
	}
	if len(s) > 1 {
		lv := strings.ToLower(strings.TrimSpace(s[1]))
		if validStatusLevels[lv] {
			st.Level = lv
		} else {
			st.Level = "warn"
			return st, true
		}
	}
	return st, false
}

// Position 在 TOML 中可为 []any（层级数组）、[][]any（多路径）、
// string（正则）或 map[string]any（html_selector）。
// 解析后存为 any，由 checker 运行时判断。
type Position = any

// PlatConfig 单个平台的最终配置
type PlatConfig struct {
	URL              string            `toml:"url,omitempty"`
	Type             string            `toml:"type,omitempty"`
	UA               string            `toml:"ua,omitempty"`
	Headers          map[string]string `toml:"headers,omitempty"`
	BaseURL          string            `toml:"baseurl,omitempty"`
	Owner            string            `toml:"owner,omitempty"`
	Repo             string            `toml:"repo,omitempty"`
	PerPage          int               `toml:"per_page,omitempty"`
	AllowPrerelease  bool              `toml:"allow_prerelease,omitempty"`
	VURL             string            `toml:"v_url,omitempty"`
	VType            string            `toml:"v_type,omitempty"`
	DURL             string            `toml:"d_url,omitempty"`
	DType            string            `toml:"d_type,omitempty"`
	VPosition        Position          `toml:"v_position,omitempty"`
	DPosition        Position          `toml:"d_position,omitempty"`
	VJoin            string            `toml:"v_join,omitempty"`
	DJoin            string            `toml:"d_join,omitempty"`
	DownloadMethod   string            `toml:"download_method,omitempty"`
	DownloadViaProxy bool              `toml:"download_via_proxy,omitempty"`
	DownloadName     string            `toml:"download_name,omitempty"`
}

// PreRequestStep 一个前置请求步骤（TOML 解析与运行时共用）
type PreRequestStep struct {
	URL      string            `toml:"url"`
	Type     string            `toml:"type"`
	UA       string            `toml:"ua,omitempty"`
	Headers  map[string]string `toml:"headers,omitempty"`
	BaseURL  string            `toml:"baseurl,omitempty"`
	Position Position          `toml:"position"`
}

// Rule 解析后的完整规则
type Rule struct {
	Info          RuleInfo
	Status        RuleStatus                           // 由 Info.Status 解析
	MissingValues []string                             // 规则引用但 config 未配置的规则变量
	SourceID      string                               // 所属规则源 source_id
	Config        PlatConfig                           // 共享配置（[config] 基字段）
	Platforms     map[string]PlatConfig                // 各平台最终配置（解析时已与 Config 合并）
	PreRequests   map[string]map[string]PreRequestStep // id → platform(空串=通用) → step
}

// 解析

// RuleIssue 解析规则时发现的问题（不直接上报，由调用方决定如何展示）
type RuleIssue struct {
	Level   string `json:"level"` // "warn" / "error"
	Message string `json:"message"`
}

func LoadRules(home string) (map[string]Rule, []RuleIssue, error) {
	rules := make(map[string]Rule)
	var issues []RuleIssue
	ruleDir := filepath.Join(home, "rules")

	// 规则变量来自 config.toml；读不到就当作未配置
	var ruleValues map[string]map[string]string
	if cfg, cfgErr := LoadConfig(home); cfgErr == nil {
		ruleValues = cfg.RuleValues
	}

	err := filepath.WalkDir(ruleDir, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if d.IsDir() || filepath.Ext(path) != ".toml" {
			return nil
		}
		// 提取 source_id：从 .toml 文件向上查找最近的 _source.json 所在目录
		sourceID := findNearestSourceID(ruleDir, path)

		rule, ruleIssues, parseErr := ParseRuleFile(path, ruleValues)
		issues = append(issues, ruleIssues...)
		if parseErr != nil {
			issues = append(issues, RuleIssue{Level: "error", Message: fmt.Sprintf("解析规则文件失败 %s: %v", path, parseErr)})
			return nil
		}
		rule.SourceID = sourceID
		if _, exists := rules[rule.Info.AppID]; !exists {
			rules[rule.Info.AppID] = rule
		}
		return nil
	})
	if err != nil {
		return nil, issues, fmt.Errorf("walk rule dir: %w", err)
	}
	return rules, issues, nil
}

// RulesFingerprint 计算 rules/ 目录的轻量指纹（路径+大小+mtime）
// 用于判断是否需要重新解析规则，避免每次访问都全量读盘。
func RulesFingerprint(home string) string {
	ruleDir := filepath.Join(home, "rules")
	h := fnv.New64a()
	filepath.WalkDir(ruleDir, func(path string, d os.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return nil
		}
		info, err := d.Info()
		if err != nil {
			return nil
		}
		fmt.Fprintf(h, "%s|%d|%d;", path, info.Size(), info.ModTime().UnixNano())
		return nil
	})
	return strconv.FormatUint(h.Sum64(), 16)
}

// ruleSections 规则文件的合法顶层段
var ruleSections = map[string]bool{"info": true, "config": true, "pre_request": true}

func ParseRuleFile(path string, ruleValues map[string]map[string]string) (Rule, []RuleIssue, error) {
	var raw map[string]any
	if _, err := toml.DecodeFile(path, &raw); err != nil {
		return Rule{}, nil, err
	}

	label := filepath.Base(path)
	var issues []RuleIssue

	// 规则变量替换：按 app_id 作用域，遍历所有字符串值（不改 key）
	appID, _ := raw["info"].(map[string]any)
	appIDStr := ""
	if appID != nil {
		appIDStr, _ = appID["app_id"].(string)
	}
	missingSet := make(map[string]bool)
	raw = substituteRaw(raw, ruleValues[appIDStr], missingSet).(map[string]any)
	if appID != nil && appIDStr != "" {
		appID["app_id"] = appIDStr // 身份键，不参与替换
	}
	var missing []string
	for m := range missingSet {
		missing = append(missing, m)
	}
	sort.Strings(missing)
	for _, m := range missing {
		issues = append(issues, RuleIssue{Level: "warn", Message: fmt.Sprintf("%s: 未配置规则变量 %s", label, m)})
	}

	var unknownTop []string
	for k := range raw {
		if !ruleSections[k] {
			unknownTop = append(unknownTop, k)
		}
	}
	if len(unknownTop) > 0 {
		sort.Strings(unknownTop)
		issues = append(issues, RuleIssue{Level: "warn", Message: fmt.Sprintf("%s: 未知段 %s", label, strings.Join(unknownTop, ", "))})
	}

	rule := Rule{
		MissingValues: missing,
		Platforms:     make(map[string]PlatConfig),
		PreRequests:   make(map[string]map[string]PreRequestStep),
	}

	if err := parseInfo(&rule, raw, label, &issues); err != nil {
		return Rule{}, issues, err
	}
	if err := parseConfig(&rule, raw, label, &issues); err != nil {
		return Rule{}, issues, err
	}
	if err := parsePreRequests(&rule, raw, label, &issues); err != nil {
		return Rule{}, issues, err
	}

	for _, is := range rule.Validate() {
		is.Message = label + ": " + is.Message
		issues = append(issues, is)
	}
	return rule, issues, nil
}

// parseInfo 解析 [info]
func parseInfo(rule *Rule, raw map[string]any, label string, issues *[]RuleIssue) error {
	infoRaw, ok := raw["info"]
	if !ok {
		return nil
	}
	infoMap, ok := infoRaw.(map[string]any)
	if !ok {
		return fmt.Errorf("%s: info: 期望表结构", label)
	}
	*issues = append(*issues, validateSection("info", label+": info", infoMap)...)
	info, _, err := decodeSection[RuleInfo](infoMap, label+": info")
	if err != nil {
		return err
	}
	rule.Info = info
	st, badLevel := ParseRuleStatus(info.Status)
	rule.Status = st
	if badLevel {
		*issues = append(*issues, RuleIssue{Level: "warn", Message: fmt.Sprintf("%s: info: 未知状态等级 %q，按 warn 处理", label, info.Status[1])})
	}
	return nil
}

// parseConfig 解析 [config] + [config.{os}]，解析时即合并（平台按 key 覆盖共享）
func parseConfig(rule *Rule, raw map[string]any, label string, issues *[]RuleIssue) error {
	cfgRaw, ok := raw["config"]
	if !ok {
		return nil
	}
	cfgMap, ok := cfgRaw.(map[string]any)
	if !ok {
		return fmt.Errorf("%s: config: 期望表结构", label)
	}
	// 共享基字段（排除平台子表）
	base := make(map[string]any, len(cfgMap))
	for k, v := range cfgMap {
		if isPlatformKey(k, rule.Info.Platforms) {
			continue
		}
		base[k] = v
	}
	*issues = append(*issues, validateSection("plat", label+": config", base)...)
	cfg, _, err := decodeSection[PlatConfig](base, label+": config")
	if err != nil {
		return err
	}
	rule.Config = cfg

	for key, val := range cfgMap {
		if !isPlatformKey(key, rule.Info.Platforms) {
			continue
		}
		vm, ok := val.(map[string]any)
		if !ok {
			return fmt.Errorf("%s: config.%s: 期望表结构", label, key)
		}
		merged := make(map[string]any, len(base)+len(vm))
		for k, v := range base {
			merged[k] = v
		}
		for k, v := range vm {
			merged[k] = v
		}
		*issues = append(*issues, validateSection("plat", label+": config."+key, merged)...)
		pc, _, err := decodeSection[PlatConfig](merged, label+": config."+key)
		if err != nil {
			return err
		}
		rule.Platforms[key] = pc
	}
	return nil
}

// parsePreRequests 解析 [pre_request.{id}] + [pre_request.{id}.{os}]
func parsePreRequests(rule *Rule, raw map[string]any, label string, issues *[]RuleIssue) error {
	prRaw, ok := raw["pre_request"]
	if !ok {
		return nil
	}
	prMap, ok := prRaw.(map[string]any)
	if !ok {
		return fmt.Errorf("%s: pre_request: 期望表结构", label)
	}
	for id, val := range prMap {
		steps := make(map[string]PreRequestStep)
		stepMap, ok := val.(map[string]any)
		if !ok {
			return fmt.Errorf("%s: pre_request.%s: 期望表结构", label, id)
		}
		hasPlatform := false
		for k := range stepMap {
			if isPlatformKey(k, rule.Info.Platforms) {
				hasPlatform = true
				break
			}
		}
		if hasPlatform {
			var stray []string
			for k, v := range stepMap {
				if !isPlatformKey(k, rule.Info.Platforms) {
					stray = append(stray, k)
					continue
				}
				vm, ok := v.(map[string]any)
				if !ok {
					return fmt.Errorf("%s: pre_request.%s.%s: 期望表结构", label, id, k)
				}
				*issues = append(*issues, validateSection("pre_step", label+": pre_request."+id+"."+k, vm)...)
				rs, _, err := decodeSection[PreRequestStep](vm, label+": pre_request."+id+"."+k)
				if err != nil {
					return err
				}
				steps[k] = rs
			}
			if len(stray) > 0 {
				sort.Strings(stray)
				*issues = append(*issues, RuleIssue{Level: "warn", Message: fmt.Sprintf("%s: pre_request.%s: 未知字段 %s", label, id, strings.Join(stray, ", "))})
			}
		} else {
			*issues = append(*issues, validateSection("pre_step", label+": pre_request."+id, stepMap)...)
			rs, _, err := decodeSection[PreRequestStep](stepMap, label+": pre_request."+id)
			if err != nil {
				return err
			}
			steps[""] = rs
		}
		rule.PreRequests[id] = steps
	}
	return nil
}

// decodeSection 校验未知字段并解码为强类型。未知字段随返回值交给调用方决定如何处理；
// 类型错误返回 error。
func decodeSection[T any](raw map[string]any, section string) (T, []string, error) {
	var result T
	unknown := unknownKeys(raw, reflect.TypeOf((*T)(nil)).Elem())
	var buf bytes.Buffer
	if err := toml.NewEncoder(&buf).Encode(raw); err != nil {
		return result, unknown, fmt.Errorf("%s: %w", section, err)
	}
	if _, err := toml.NewDecoder(&buf).Decode(&result); err != nil {
		return result, unknown, fmt.Errorf("%s: %w", section, err)
	}
	return result, unknown, nil
}

// unknownKeys 返回 raw 中不在结构体 toml tag 内的字段名。
func unknownKeys(raw map[string]any, typ reflect.Type) []string {
	known := knownTOMLFields(typ)
	var unknown []string
	for k := range raw {
		if !known[k] {
			unknown = append(unknown, k)
		}
	}
	sort.Strings(unknown)
	return unknown
}

// knownTOMLFields 收集结构体各字段的 toml tag 名。
func knownTOMLFields(typ reflect.Type) map[string]bool {
	fields := make(map[string]bool)
	for typ.Kind() == reflect.Ptr {
		typ = typ.Elem()
	}
	if typ.Kind() != reflect.Struct {
		return fields
	}
	for i := 0; i < typ.NumField(); i++ {
		name := strings.Split(typ.Field(i).Tag.Get("toml"), ",")[0]
		if name == "" || name == "-" {
			continue
		}
		fields[name] = true
	}
	return fields
}

// 合并

// MergedConfig 返回某平台的最终配置。Platforms 在解析时已与共享 [config] 合并好
func (r Rule) MergedConfig(os string) PlatConfig {
	if plat, ok := r.Platforms[os]; ok {
		return plat
	}
	return r.Config
}

// Validate 语义校验（解析后调用）：平台、type、github 必填、url scheme、position 与正则。
// 返回的 Message 不含文件名，由调用方补前缀。
func (r Rule) Validate() []RuleIssue {
	var issues []RuleIssue
	if len(r.Info.Platforms) == 0 {
		issues = append(issues, RuleIssue{Level: "error", Message: "info: 至少需要一个平台"})
	}
	// official_website 会被前端直接送进 openUrl。new URL() 对 "javascript:..." 不抛错，
	// 浏览器分支一旦无校验就会执行规则里带来的脚本（core/store/rule.schema.json 不存在，
	// 只能在这里校验）。协议相对 //host 交由 normalizeURL / 前端补全，不算非法。
	if w := strings.TrimSpace(r.Info.OfficialWebsite); w != "" {
		if level, msg := checkURLScheme("info: official_website", w, true); level != "" {
			issues = append(issues, RuleIssue{Level: level, Message: msg})
		}
	}
	for _, p := range r.Info.Platforms {
		if strings.TrimSpace(p) == "" {
			issues = append(issues, RuleIssue{Level: "error", Message: "info: 平台名不能为空"})
		}
	}
	for _, os := range r.Info.Platforms {
		if strings.TrimSpace(os) == "" {
			continue
		}
		issues = append(issues, validatePlatConfig("config."+os, r.MergedConfig(os))...)
	}
	return issues
}

// checkURLScheme 校验一个 URL 字段的协议。
//
// allowRelative 按字段的消费方区分，不能一刀切：
//   - config 的 url / v_url / d_url 是**服务端取数的目标**，必须是绝对 http(s)，
//     否则 http.NewRequest 直接失败——无 scheme、协议相对都要报出来。
//   - official_website 是**前端链接**，会被 absoluteUrl() 相对 origin 补全，
//     所以无 scheme 与 //host 都是合法的。
//
// 两边共同的一条：javascript: 之类必须 error —— new URL() 对它不抛错，
// 前端 openUrl 若无白名单就会执行规则里带来的脚本。
func checkURLScheme(field, u string, allowRelative bool) (level, msg string) {
	if u == "" {
		return "", "" // 字段未设置
	}
	// 协议相对地址原样放行：消费方是浏览器与下载器，两者都认 //host
	if strings.HasPrefix(strings.TrimSpace(u), "//") {
		return "", ""
	}
	scheme := httpx.URLScheme(u)
	if scheme == "http" || scheme == "https" {
		return "", ""
	}
	if scheme == "" {
		if allowRelative {
			return "", ""
		}
		return "warn", field + " 不是 http(s) 链接"
	}
	if scheme == "javascript" || scheme == "data" || scheme == "vbscript" || scheme == "file" {
		return "error", field + " 使用了非 http(s) 协议（" + scheme + ":），可能被用于注入脚本"
	}
	return "warn", field + " 不是 http(s) 链接"
}

// validParserTypes 合法的解析器类型。type / v_type / d_type 都必须落在这个集合里。
// 拼写错误（如 v_type = "jsno"）过去能通过全部校验，直到运行时才炸成一个
// 字面量 "<nil>" 的"版本号"。core/store/rule.schema.json 并不存在，
// 所以这个 enum 由代码定义并在规则检查时校验。
var validParserTypes = map[string]bool{
	"json": true, "xml": true, "regex": true,
	"html_selector": true, "github": true, "direct": true,
}

// IsValidParserType 报告 t 是否为受支持的解析器类型。
// 规则检查与运行时检查共用这一份定义，避免两处枚举漂移。
func IsValidParserType(t string) bool { return validParserTypes[t] }

func validatePlatConfig(name string, c PlatConfig) []RuleIssue {
	var issues []RuleIssue
	add := func(level, msg string) {
		issues = append(issues, RuleIssue{Level: level, Message: name + ": " + msg})
	}

	vType := c.VType
	if vType == "" {
		vType = c.Type
	}
	dType := c.DType
	if dType == "" {
		dType = c.Type
	}
	if vType == "" {
		add("error", "缺少 type（版本号解析器）")
	} else if !validParserTypes[vType] {
		add("error", fmt.Sprintf("未知版本号解析器 %q，可选 %s", vType, parserTypeList()))
	}
	if dType == "" {
		add("error", "缺少 type（下载解析器）")
	} else if !validParserTypes[dType] {
		add("error", fmt.Sprintf("未知下载解析器 %q，可选 %s", dType, parserTypeList()))
	}

	if c.Type == "github" {
		if c.Owner == "" {
			add("error", "github 规则缺少 owner")
		}
		if c.Repo == "" {
			add("error", "github 规则缺少 repo")
		}
	}

	checkURL := func(field, u string) {
		if level, msg := checkURLScheme(field, u, false); level != "" {
			add(level, msg)
		}
	}
	// 规则里的 URL 会被前端拿去过 scheme 白名单（openUrl），但 static 校验
	// 提前挡住更省事：core/store/rule.schema.json 并不存在，只能在代码里校验
	checkURL("url", c.URL)
	checkURL("v_url", c.VURL)
	checkURL("d_url", c.DURL)

	issues = append(issues, validatePosition(name, "v_position", c.VPosition, vType)...)
	issues = append(issues, validatePosition(name, "d_position", c.DPosition, dType)...)
	issues = append(issues, validateDeadFields(name, c, vType, dType)...)
	return issues
}

// parserTypeList 合法解析器类型的可读列表，用于错误提示
func parserTypeList() string {
	names := make([]string, 0, len(validParserTypes))
	for k := range validParserTypes {
		names = append(names, k)
	}
	sort.Strings(names)
	return strings.Join(names, " | ")
}

// configuredFields 本配置里作者显式配了哪些字段（零值不算「配了」）
func configuredFields(c PlatConfig) map[string]bool {
	m := map[string]bool{}
	for f, v := range map[string]string{
		"url": c.URL, "v_url": c.VURL, "v_type": c.VType,
		"d_url": c.DURL, "d_type": c.DType,
		"v_join": c.VJoin, "d_join": c.DJoin,
		"baseurl": c.BaseURL, "ua": c.UA,
		"owner": c.Owner, "repo": c.Repo,
	} {
		if strings.TrimSpace(v) != "" {
			m[f] = true
		}
	}
	if c.VPosition != nil {
		m["v_position"] = true
	}
	if c.DPosition != nil {
		m["d_position"] = true
	}
	if len(c.Headers) > 0 {
		m["headers"] = true
	}
	if c.PerPage != 0 {
		m["per_page"] = true
	}
	if c.AllowPrerelease {
		m["allow_prerelease"] = true
	}
	return m
}

// deadFields 返回在给定 (type, v_type, d_type) 下**不会被读取**的字段 → 原因。
// 只列出作者确实配了的字段。
//
// 依据实际读取点，而不是「文档里有没有写」：
//   - core/checker/checker.go:128-186  RunPlatformCheck
//   - core/checker/github.go:35-117    CheckGitHub / pickLatestRelease
//   - core/checker/runner.go:166-168   runGitHubCheck 的 direct 分支
//
// 两个容易搞错的地方：
//   - github 的 d_position 不是路径定位，而是 asset 文件名正则（github.go:93），
//     所以它在 github 下是活的，不能按「路径类字段」一律判死。
//   - baseurl 只在下载侧、且 d_type != direct 时用于 JoinURL（checker.go:181）。
func deadFields(c PlatConfig, vType, dType string) map[string]string {
	set := configuredFields(c)
	dead := map[string]string{}
	kill := func(f, why string) {
		if set[f] {
			dead[f] = why
		}
	}
	if c.Type == "github" {
		// 版本号隐式取 [release 序号, "tag_name"]，请求地址由 owner/repo 拼出
		const tagOnly = "github 的版本号固定从 tag_name 提取，请求地址由 owner/repo 拼接"
		for _, f := range []string{"url", "v_url", "v_type", "v_position", "v_join"} {
			kill(f, tagOnly)
		}
		kill("d_join", "github 的 d_position 是 asset 文件名正则，没有多路径拼接")
		kill("baseurl", "github 不做相对地址拼接")
		if dType != "direct" {
			kill("d_url", "github 的下载链接取自 asset，只有 d_type=direct 时才读 d_url")
		}
		return dead
	}
	const onlyGitHub = "仅 github 规则使用"
	for _, f := range []string{"owner", "repo", "per_page", "allow_prerelease"} {
		kill(f, onlyGitHub)
	}
	if vType == "direct" {
		kill("v_position", "v_type=direct，版本号直接取 v_url/url，不需要 v_position")
		kill("v_join", "v_type=direct，不做多路径拼接")
	}
	if dType == "direct" {
		kill("d_position", "d_type=direct，下载链接直接取 d_url/url，不需要 d_position")
		kill("d_join", "d_type=direct，不做多路径拼接")
		kill("baseurl", "baseurl 只用于拼接下载链接的相对地址，d_type=direct 时用不到")
	}
	// url 是 v_url / d_url 的回落来源：两者都单独指定了，url 就不会被读到
	if c.VURL != "" && c.DURL != "" {
		kill("url", "v_url 与 d_url 都已单独指定，url 不会作为回落地址被读到")
	}
	// ua / headers 只在真的要发请求时用得上
	if (vType == "direct" || c.VPosition == nil) &&
		(dType == "direct" || c.DPosition == nil) {
		kill("ua", "本次检查不发请求（版本号与下载链接都不需要提取）")
		kill("headers", "本次检查不发请求（版本号与下载链接都不需要提取）")
	}
	return dead
}

// validateDeadFields 报出「配了但当前 type 下不生效」的字段。
// 报 warn 而非 error：属于规则的整洁度问题，不影响检查能否进行，
// 且规则集里数量极少（332 个平台配置中 2 处）。
func validateDeadFields(name string, c PlatConfig, vType, dType string) []RuleIssue {
	dead := deadFields(c, vType, dType)
	keys := make([]string, 0, len(dead))
	for f := range dead {
		keys = append(keys, f)
	}
	sort.Strings(keys)
	issues := make([]RuleIssue, 0, len(keys))
	for _, f := range keys {
		issues = append(issues, RuleIssue{Level: "warn", Message: fmt.Sprintf(
			"%s: %s 在 type=%s / v_type=%s / d_type=%s 下不会被读取（%s）",
			name, f, typeOrNone(c.Type), typeOrNone(vType), typeOrNone(dType), dead[f])})
	}
	return issues
}

func typeOrNone(s string) string {
	if s == "" {
		return "无"
	}
	return s
}

func validatePosition(name, field string, pos any, typ string) []RuleIssue {
	if pos == nil || typ == "" || typ == "direct" {
		return nil
	}
	var issues []RuleIssue
	add := func(msg string) {
		issues = append(issues, RuleIssue{Level: "warn", Message: name + ": " + field + " " + msg})
	}
	if !validParserTypes[typ] {
		// 未知类型在 validatePlatConfig 已报 error，这里不重复
		return nil
	}
	switch typ {
	case "json", "xml":
		arr, ok := pos.([]any)
		if !ok {
			add("应为数组")
			return issues
		}
		checkPositionRegex(arr, add)
	case "regex":
		s, ok := pos.(string)
		if !ok {
			add("应为正则字符串")
			return issues
		}
		if _, err := regexp.Compile(s); err != nil {
			add("正则无法编译：" + err.Error())
		}
	case "html_selector":
		m, ok := pos.(map[string]any)
		if !ok {
			add("应为对象（含 selector / attr / regex）")
			return issues
		}
		if sel, _ := m["selector"].(string); sel == "" {
			add("缺少 selector")
		}
		if re, _ := m["regex"].(string); re != "" {
			if _, err := regexp.Compile(re); err != nil {
				add("regex 无法编译：" + err.Error())
			}
		}
	case "github":
		s, ok := pos.(string)
		if !ok {
			add("应为匹配文件名用的正则字符串")
			return issues
		}
		if _, err := regexp.Compile(s); err != nil {
			add("正则无法编译：" + err.Error())
		}
	}
	return issues
}

// checkPositionRegex 校验 json/xml 数组 position 中 `name~正则` 段的正则
func checkPositionRegex(arr []any, add func(string)) {
	for _, v := range arr {
		switch x := v.(type) {
		case []any:
			checkPositionRegex(x, add)
		case string:
			if i := strings.Index(x, "~"); i >= 0 {
				if _, err := regexp.Compile(x[i+1:]); err != nil {
					add("内联正则无法编译（" + x + "）：" + err.Error())
				}
			}
		}
	}
}

func (r Rule) PreRequestChain(os string) []PreRequestStep {
	ids := make([]string, 0, len(r.PreRequests))
	for id := range r.PreRequests {
		ids = append(ids, id)
	}
	sort.Strings(ids)

	var chain []PreRequestStep
	for _, id := range ids {
		steps := r.PreRequests[id]
		if step, ok := steps[os]; ok {
			chain = append(chain, step)
		} else if step, ok := steps[""]; ok {
			chain = append(chain, step)
		}
	}
	return chain
}

// SourceNames 返回 rules/ 下子规则源的 source_id → 名称映射（一次遍历）
func SourceNames(home string) map[string]string {
	summaries, err := ListSourceInfos(home)
	if err != nil {
		return map[string]string{}
	}
	m := make(map[string]string, len(summaries))
	for _, s := range summaries {
		m[s.SourceID] = s.Name
	}
	return m
}

// ListSourceInfos 遍历 rules/ 下所有 _source.json，跳过 type=list。
// 返回子规则源（type=rules）的元信息。
func ListSourceInfos(home string) ([]SourceSummary, error) {
	ruleDir := filepath.Join(home, "rules")
	var result []SourceSummary

	err := filepath.WalkDir(ruleDir, func(path string, d os.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return nil
		}
		if filepath.Base(path) != "_source.json" {
			return nil
		}
		data, readErr := os.ReadFile(path)
		if readErr != nil {
			return nil
		}
		var s SourceInfo
		if json.Unmarshal(data, &s) != nil {
			return nil
		}
		if s.Type == "list" {
			return nil
		}
		result = append(result, SourceSummary{
			SourceID:    s.ID,
			Name:        s.Name,
			Description: s.Description,
			AppCount:    len(s.Files),
		})
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("walk rules: %w", err)
	}
	return result, nil
}

// SourceSummary 源的汇总信息
type SourceSummary struct {
	SourceID    string `json:"source_id"`
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
	AppCount    int    `json:"app_count"`
}

// findNearestSourceID 从 .toml 文件向上查找最近的 _source.json 所在目录，返回目录名。
func findNearestSourceID(ruleDir, tomlPath string) string {
	dir := filepath.Dir(tomlPath)
	for {
		if _, err := os.Stat(filepath.Join(dir, "_source.json")); err == nil {
			return filepath.Base(dir)
		}
		parent := filepath.Dir(dir)
		if parent == dir || parent == ruleDir {
			break
		}
		dir = parent
	}
	// 兜底：使用第一级目录名
	rel, _ := filepath.Rel(ruleDir, filepath.Dir(tomlPath))
	return strings.SplitN(rel, string(filepath.Separator), 2)[0]
}

// 辅助

func isPlatformKey(s string, platforms []string) bool {
	for _, p := range platforms {
		if p == s {
			return true
		}
	}
	return false
}
