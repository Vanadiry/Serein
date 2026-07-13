package store

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
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
	URL       string            `toml:"url,omitempty"`
	Type      string            `toml:"type,omitempty"`
	UA        string            `toml:"ua,omitempty"`
	Headers   map[string]string `toml:"headers,omitempty"`
	BaseURL   string            `toml:"baseurl,omitempty"`
	Owner     string            `toml:"owner,omitempty"`
	Repo      string            `toml:"repo,omitempty"`
	VURL      string            `toml:"v_url,omitempty"`
	VType     string            `toml:"v_type,omitempty"`
	DURL      string            `toml:"d_url,omitempty"`
	DType     string            `toml:"d_type,omitempty"`
	VPosition Position          `toml:"v_position,omitempty"`
	DPosition Position          `toml:"d_position,omitempty"`
	VJoin     string            `toml:"v_join,omitempty"`
	DJoin     string            `toml:"d_join,omitempty"`
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
	SourceID    string                                          // 所属规则源 source_id
	Config      PlatConfig                                      // 共享配置
	Platforms   map[string]PlatConfig                           // 各平台特有配置
	PreRequests map[string]map[string]PreRequestStep             // id → platform(空串=通用) → step
}

// 解析

func LoadRules(home string) (map[string]Rule, error) {
	rules := make(map[string]Rule)
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

		rule, parseErr := ParseRuleFile(path)
		if parseErr != nil {
			Emit("error", "[rules]", fmt.Sprintf("解析规则文件失败 %s: %v", path, parseErr))
			return nil
		}
		rule.SourceID = sourceID
		if _, exists := rules[rule.Info.AppID]; !exists {
			rules[rule.Info.AppID] = rule
		}
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("walk rule dir: %w", err)
	}
	return rules, nil
}

func ParseRuleFile(path string) (Rule, error) {
	var raw map[string]any
	if _, err := toml.DecodeFile(path, &raw); err != nil {
		return Rule{}, err
	}

	rule := Rule{
		Platforms:   make(map[string]PlatConfig),
		PreRequests: make(map[string]map[string]PreRequestStep),
	}

	// info
	if infoRaw, ok := raw["info"]; ok {
		info := parseInto[RuleInfo](infoRaw)
		if info != nil {
			rule.Info = *info
		}
	}

	// config + config.{os}
	if cfgRaw, ok := raw["config"]; ok {
		rule.Config = *parseInto[PlatConfig](cfgRaw)
		if cfgMap, ok := cfgRaw.(map[string]any); ok {
			for key, val := range cfgMap {
				if isPlatformKey(key, rule.Info.Platforms) {
					rule.Platforms[key] = *parseInto[PlatConfig](val)
				}
			}
		}
	}

	// pre_request.{id} + pre_request.{id}.{os}
	if prRaw, ok := raw["pre_request"]; ok {
		if prMap, ok := prRaw.(map[string]any); ok {
			for id, val := range prMap {
				steps := make(map[string]PreRequestStep)
				if stepMap, ok := val.(map[string]any); ok {
					hasPlatform := false
					for k, v := range stepMap {
						if isPlatformKey(k, rule.Info.Platforms) {
							hasPlatform = true
							rs := parseInto[rawPreStep](v)
							steps[k] = rawToStep(rs)
						}
					}
					if !hasPlatform {
						rs := parseInto[rawPreStep](val)
						steps[""] = rawToStep(rs)
					}
				}
				rule.PreRequests[id] = steps
			}
		}
	}

	return rule, nil
}

// parseInto 将 any 编解码为指定类型
func parseInto[T any](v any) *T {
	var buf bytes.Buffer
	if err := toml.NewEncoder(&buf).Encode(v); err != nil {
		return nil
	}
	var result T
	if _, err := toml.NewDecoder(&buf).Decode(&result); err != nil {
		return nil
	}
	return &result
}

// rawToStep 将原始前置请求转为 PreRequestStep
func rawToStep(raw *rawPreStep) PreRequestStep {
	if raw == nil {
		return PreRequestStep{}
	}
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
