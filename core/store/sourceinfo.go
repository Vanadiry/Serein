// 规则源元信息（_source.json）的格式定义、解析与结构校验。
//
// files 按 type 分形态：
//
//	type = "rules"（默认）：{"文件名": "token"}
//	type = "list"：["<name>/_source.json", ...]，子源 marker 列表
//
// token 是不透明字符串，程序只比较它与本地已接受的值是否相同，不同即重新拉取。
// 版本精度到单文件，因此不再有顶层 version 字段。
//
// 两条结构约束（违反者剔除条目并报 warn，不阻断整个源）：
//
//	list  的条目必须匹配 ^[^/\\]+/_source\.json$（恰好一层）
//	rules 的 key 必须是裸文件名，且不得为 _source.json
//
// 这两条约束是「删除 manifest 未列出文件」能安全执行的前提：没有它们，
// 一个子源的目录可以成为另一个子源目录的祖先，后者扫描未列出文件时会
// 删掉前者的 marker，使其永久失去被更新与被列出的资格。
package store

import (
	"encoding/json"
	"fmt"
	"regexp"
)

// sourceFileName 规则源的版本标记文件名
const sourceFileName = "_source.json"

var (
	// list 型条目：恰好一层 <name>/_source.json
	listEntryRe = regexp.MustCompile(`^[^/\\]+/` + sourceFileName + `$`)
	// rules 型 key：裸文件名，不含路径分隔符
	bareNameRe = regexp.MustCompile(`^[^/\\]+$`)
)

// SourceInfo 规则源的元信息
type SourceInfo struct {
	ID          string            `json:"source_id"`
	Name        string            `json:"name,omitempty"`
	Description string            `json:"description,omitempty"`
	Type        string            `json:"type,omitempty"` // "rules"（默认）或 "list"
	BaseURL     string            `json:"baseurl,omitempty"`
	Files       map[string]string `json:"files,omitempty"` // rules 型：文件名 → token
	SubSources  []string          `json:"-"`               // list 型：files 数组
}

// IsList 报告是否为 list 型（列出子源而非规则文件）
func (s *SourceInfo) IsList() bool { return s.Type == "list" }

// UnmarshalJSON 按 type 分派 files 的形态：list 解析为数组，其余解析为对象
func (s *SourceInfo) UnmarshalJSON(b []byte) error {
	type alias struct {
		ID          string          `json:"source_id"`
		Name        string          `json:"name,omitempty"`
		Description string          `json:"description,omitempty"`
		Type        string          `json:"type,omitempty"`
		BaseURL     string          `json:"baseurl,omitempty"`
		Files       json.RawMessage `json:"files"`
	}
	var a alias
	if err := json.Unmarshal(b, &a); err != nil {
		return err
	}
	s.ID, s.Name, s.Description = a.ID, a.Name, a.Description
	s.Type, s.BaseURL = a.Type, a.BaseURL
	s.Files, s.SubSources = nil, nil

	if len(a.Files) == 0 {
		return nil
	}
	if a.Type == "list" {
		if err := json.Unmarshal(a.Files, &s.SubSources); err != nil {
			return fmt.Errorf("type=list 的 files 必须是字符串数组: %w", err)
		}
		return nil
	}
	if err := json.Unmarshal(a.Files, &s.Files); err != nil {
		return fmt.Errorf("type=%s 的 files 必须是 \"文件名\": \"token\" 对象: %w", sourceTypeName(a.Type), err)
	}
	return nil
}

func sourceTypeName(t string) string {
	if t == "" {
		return "rules"
	}
	return t
}

// MarshalJSON 与 UnmarshalJSON 对称：按 type 输出对应形态的 files。
// 没有它则 SubSources（json:"-"）无法往返，任何 marshal 出的 list 型源都会丢掉子源列表。
func (s SourceInfo) MarshalJSON() ([]byte, error) {
	out := struct {
		ID          string `json:"source_id"`
		Name        string `json:"name,omitempty"`
		Description string `json:"description,omitempty"`
		Type        string `json:"type,omitempty"`
		BaseURL     string `json:"baseurl,omitempty"`
		Files       any    `json:"files,omitempty"`
	}{
		ID:          s.ID,
		Name:        s.Name,
		Description: s.Description,
		Type:        s.Type,
		BaseURL:     s.BaseURL,
	}
	if s.IsList() {
		if len(s.SubSources) > 0 {
			out.Files = s.SubSources
		}
	} else if len(s.Files) > 0 {
		out.Files = s.Files
	}
	return json.Marshal(out)
}

// validateSourceFiles 就地剔除结构不合法的 files 条目，返回问题列表与「是否整体拒绝」。
// 单条结构问题只剔除不报错：上游一个笔误不该挡住整个源，其余条目仍应正常同步。
// 但条目数超限会整体拒绝——此时剔除一部分会让规则集变得不完整，比整体失败更难排查。
func validateSourceFiles(s *SourceInfo) ([]RuleIssue, bool) {
	var issues []RuleIssue
	if s.IsList() {
		kept := s.SubSources[:0]
		for _, f := range s.SubSources {
			if !listEntryRe.MatchString(f) {
				issues = append(issues, RuleIssue{Level: "warn", Message: fmt.Sprintf(
					"%s: 子源条目 %q 不符合 <name>/%s，已忽略（子源只支持一层；若上游已改为规则表，请把 type 从 list 改成 rules）",
					s.label(), f, sourceFileName)})
				continue
			}
			kept = append(kept, f)
		}
		s.SubSources = kept
		return issues, false
	}

	kept := make(map[string]string, len(s.Files))
	for name, token := range s.Files {
		if !bareNameRe.MatchString(name) || name == "." || name == ".." {
			issues = append(issues, RuleIssue{Level: "warn", Message: fmt.Sprintf(
				"%s: 规则条目 %q 含路径分隔符，已忽略（files 只接受裸文件名）", s.label(), name)})
			continue
		}
		if name == sourceFileName {
			issues = append(issues, RuleIssue{Level: "warn", Message: fmt.Sprintf(
				"%s: files 中出现 %s，已忽略（它会覆盖本源的版本标记）", s.label(), sourceFileName)})
			continue
		}
		kept[name] = token
	}
	s.Files = kept
	if len(s.Files) > maxFilesPerSource {
		// 不是 warn 而是直接拒绝：条目数本身就是攻击面（每个条目一次请求 + 一份内存），
		// 而剔除一部分会让本源的规则集变得不完整，比整体失败更难排查。
		return append(issues, RuleIssue{Level: "error", Message: fmt.Sprintf(
			"%s: 声明了 %d 个规则文件，超过上限 %d，已拒绝该源",
			s.label(), len(s.Files), maxFilesPerSource)}), true
	}
	return issues, false
}

// validateSourceFiles 的第二返回值报告该源是否被整体拒绝

func (s *SourceInfo) label() string {
	if s.ID != "" {
		return s.ID
	}
	return "(未命名源)"
}
