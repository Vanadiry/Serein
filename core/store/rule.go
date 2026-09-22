package store

import (
	"bytes"
	"encoding/json"
	"fmt"
	"hash/fnv"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strconv"
	"strings"

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

// Position 在 TOML 中可为 []any（层级数组）、[][]any（多路径）、
// string（正则）或 map[string]any（html_selector）。
// 解析后存为 any，由 checker 运行时判断。
type Position = any

// PlatConfig 单个平台的最终配置
type PlatConfig struct {
	URL             string            `toml:"url,omitempty"`
	Type            string            `toml:"type,omitempty"`
	UA              string            `toml:"ua,omitempty"`
	Headers         map[string]string `toml:"headers,omitempty"`
	BaseURL         string            `toml:"baseurl,omitempty"`
	Owner           string            `toml:"owner,omitempty"`
	Repo            string            `toml:"repo,omitempty"`
	PerPage         int               `toml:"per_page,omitempty"`
	VURL            string            `toml:"v_url,omitempty"`
	VType           string            `toml:"v_type,omitempty"`
	DURL            string            `toml:"d_url,omitempty"`
	DType           string            `toml:"d_type,omitempty"`
	VPosition       Position          `toml:"v_position,omitempty"`
	DPosition       Position          `toml:"d_position,omitempty"`
	VJoin           string            `toml:"v_join,omitempty"`
	DJoin           string            `toml:"d_join,omitempty"`
	ForceDownloader bool              `toml:"force_downloader,omitempty"`
}

// rawPreStep 前置请求的原始 TOML 结构（position 字段名不同）
type rawPreStep struct {
	URL      string            `toml:"url"`
	Type     string            `toml:"type"`
	UA       string            `toml:"ua,omitempty"`
	Headers  map[string]string `toml:"headers,omitempty"`
	BaseURL  string            `toml:"baseurl,omitempty"`
	Position Position          `toml:"position"`
}

// PreRequestStep 运行时的一个前置请求步骤
type PreRequestStep struct {
	URL      string
	Type     string
	UA       string
	Headers  map[string]string
	BaseURL  string
	Position Position
}

// Rule 解析后的完整规则
type Rule struct {
	Info        RuleInfo
	SourceID    string                               // 所属规则源 source_id
	Config      PlatConfig                           // 共享配置
	Platforms   map[string]PlatConfig                // 各平台特有配置
	PreRequests map[string]map[string]PreRequestStep // id → platform(空串=通用) → step
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

	err := filepath.WalkDir(ruleDir, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if d.IsDir() || filepath.Ext(path) != ".toml" {
			return nil
		}
		// 提取 source_id：从 .toml 文件向上查找最近的 _source.json 所在目录
		sourceID := findNearestSourceID(ruleDir, path)

		rule, ruleIssues, parseErr := ParseRuleFile(path)
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

func ParseRuleFile(path string) (Rule, []RuleIssue, error) {
	var raw map[string]any
	if _, err := toml.DecodeFile(path, &raw); err != nil {
		return Rule{}, nil, err
	}

	label := filepath.Base(path)
	var issues []RuleIssue

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
		Platforms:   make(map[string]PlatConfig),
		PreRequests: make(map[string]map[string]PreRequestStep),
	}

	// info
	if infoRaw, ok := raw["info"]; ok {
		infoMap, ok := infoRaw.(map[string]any)
		if !ok {
			return Rule{}, issues, fmt.Errorf("%s: info: 期望表结构", label)
		}
		info, unknown, err := decodeSection[RuleInfo](infoMap, label+": info", nil)
		issues = appendUnknown(issues, label+": info", unknown)
		if err != nil {
			return Rule{}, issues, err
		}
		rule.Info = info
	}

	// config + config.{os}
	if cfgRaw, ok := raw["config"]; ok {
		cfgMap, ok := cfgRaw.(map[string]any)
		if !ok {
			return Rule{}, issues, fmt.Errorf("%s: config: 期望表结构", label)
		}
		// [config] 允许基字段 + 各平台子表
		cfg, unknown, err := decodeSection[PlatConfig](cfgMap, label+": config", rule.Info.Platforms)
		issues = appendUnknown(issues, label+": config", unknown)
		if err != nil {
			return Rule{}, issues, err
		}
		rule.Config = cfg
		for key, val := range cfgMap {
			if !isPlatformKey(key, rule.Info.Platforms) {
				continue
			}
			vm, ok := val.(map[string]any)
			if !ok {
				return Rule{}, issues, fmt.Errorf("%s: config.%s: 期望表结构", label, key)
			}
			pc, unknown, err := decodeSection[PlatConfig](vm, label+": config."+key, nil)
			issues = appendUnknown(issues, label+": config."+key, unknown)
			if err != nil {
				return Rule{}, issues, err
			}
			rule.Platforms[key] = pc
		}
	}

	// pre_request.{id} + pre_request.{id}.{os}
	if prRaw, ok := raw["pre_request"]; ok {
		prMap, ok := prRaw.(map[string]any)
		if !ok {
			return Rule{}, issues, fmt.Errorf("%s: pre_request: 期望表结构", label)
		}
		for id, val := range prMap {
			steps := make(map[string]PreRequestStep)
			stepMap, ok := val.(map[string]any)
			if !ok {
				return Rule{}, issues, fmt.Errorf("%s: pre_request.%s: 期望表结构", label, id)
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
						return Rule{}, issues, fmt.Errorf("%s: pre_request.%s.%s: 期望表结构", label, id, k)
					}
					rs, unknown, err := decodeSection[rawPreStep](vm, label+": pre_request."+id+"."+k, nil)
					issues = appendUnknown(issues, label+": pre_request."+id+"."+k, unknown)
					if err != nil {
						return Rule{}, issues, err
					}
					steps[k] = rawToStep(rs)
				}
				if len(stray) > 0 {
					sort.Strings(stray)
					issues = append(issues, RuleIssue{Level: "warn", Message: fmt.Sprintf("%s: pre_request.%s: 未知字段 %s", label, id, strings.Join(stray, ", "))})
				}
			} else {
				rs, unknown, err := decodeSection[rawPreStep](stepMap, label+": pre_request."+id, nil)
				issues = appendUnknown(issues, label+": pre_request."+id, unknown)
				if err != nil {
					return Rule{}, issues, err
				}
				steps[""] = rawToStep(rs)
			}
			rule.PreRequests[id] = steps
		}
	}

	return rule, issues, nil
}

// appendUnknown 把未知字段名转成告警 issue
func appendUnknown(issues []RuleIssue, section string, unknown []string) []RuleIssue {
	if len(unknown) == 0 {
		return issues
	}
	return append(issues, RuleIssue{Level: "warn", Message: fmt.Sprintf("%s: 未知字段 %s", section, strings.Join(unknown, ", "))})
}

// decodeSection 校验未知字段并解码为强类型。未知字段随返回值交给调用方决定如何处理；
// 类型错误返回 error。extra 为额外允许的字段名。
func decodeSection[T any](raw map[string]any, section string, extra []string) (T, []string, error) {
	var result T
	unknown := unknownKeys(raw, reflect.TypeOf((*T)(nil)).Elem(), extra)
	var buf bytes.Buffer
	if err := toml.NewEncoder(&buf).Encode(raw); err != nil {
		return result, unknown, fmt.Errorf("%s: %w", section, err)
	}
	if _, err := toml.NewDecoder(&buf).Decode(&result); err != nil {
		return result, unknown, fmt.Errorf("%s: %w", section, err)
	}
	return result, unknown, nil
}

// unknownKeys 返回 raw 中不在结构体 toml tag 内的字段名（含 extra）。
func unknownKeys(raw map[string]any, typ reflect.Type, extra []string) []string {
	known := knownTOMLFields(typ)
	for _, e := range extra {
		known[e] = true
	}
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

// rawToStep 将原始前置请求转为 PreRequestStep
func rawToStep(raw rawPreStep) PreRequestStep {
	return PreRequestStep{
		URL:      raw.URL,
		Type:     raw.Type,
		UA:       raw.UA,
		Headers:  raw.Headers,
		BaseURL:  raw.BaseURL,
		Position: raw.Position,
	}
}

// 合并

func (r Rule) MergedConfig(os string) PlatConfig {
	cfg := r.Config
	if plat, ok := r.Platforms[os]; ok {
		cfg = mergePlatConfig(cfg, plat)
	}
	return cfg
}

func mergePlatConfig(base, plat PlatConfig) PlatConfig {
	if plat.URL != "" {
		base.URL = plat.URL
	}
	if plat.Type != "" {
		base.Type = plat.Type
	}
	if plat.UA != "" {
		base.UA = plat.UA
	}
	if plat.Headers != nil {
		base.Headers = plat.Headers
	}
	if plat.BaseURL != "" {
		base.BaseURL = plat.BaseURL
	}
	if plat.Owner != "" {
		base.Owner = plat.Owner
	}
	if plat.Repo != "" {
		base.Repo = plat.Repo
	}
	if plat.VPosition != nil {
		base.VPosition = plat.VPosition
	}
	if plat.DPosition != nil {
		base.DPosition = plat.DPosition
	}
	if plat.VJoin != "" {
		base.VJoin = plat.VJoin
	}
	if plat.DJoin != "" {
		base.DJoin = plat.DJoin
	}
	if plat.VURL != "" {
		base.VURL = plat.VURL
	}
	if plat.VType != "" {
		base.VType = plat.VType
	}
	if plat.DURL != "" {
		base.DURL = plat.DURL
	}
	if plat.DType != "" {
		base.DType = plat.DType
	}
	if plat.ForceDownloader {
		base.ForceDownloader = true
	}
	return base
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

// LoadSourceInfo 读取 rules 下 _source.json，返回源元信息。
func LoadSourceInfo(home, sourceID string) (*SourceJSON, error) {
	path := findSourceJSON(home, sourceID)
	if path == "" {
		return nil, fmt.Errorf("_source.json not found for %s", sourceID)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var s SourceJSON
	if err := json.Unmarshal(data, &s); err != nil {
		return nil, err
	}
	return &s, nil
}

// findSourceJSON 在 rules/ 下查找指定 source_id 的 _source.json
func findSourceJSON(home, sourceID string) string {
	ruleDir := filepath.Join(home, "rules")
	entries, err := os.ReadDir(ruleDir)
	if err != nil {
		return ""
	}
	// 先查 rules/{source_id}/_source.json（子源位于 list 源之下）
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		candidate := filepath.Join(ruleDir, e.Name(), sourceID, "_source.json")
		if _, err := os.Stat(candidate); err == nil {
			return candidate
		}
	}
	// 再查 rules/{source_id}/_source.json（顶层源）
	candidate := filepath.Join(ruleDir, sourceID, "_source.json")
	if _, err := os.Stat(candidate); err == nil {
		return candidate
	}
	return ""
}

// ListAllSourceInfos 遍历 rules/ 下所有 _source.json，跳过 type=list。
// 返回叶子源（type=rules）的元信息。
func ListAllSourceInfos(home string) ([]SourceWithID, error) {
	ruleDir := filepath.Join(home, "rules")
	var result []SourceWithID

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
		var s SourceJSON
		if json.Unmarshal(data, &s) != nil {
			return nil
		}
		if s.Type == "list" {
			return nil
		}
		result = append(result, SourceWithID{
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

// SourceWithID 源的汇总信息
type SourceWithID struct {
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
