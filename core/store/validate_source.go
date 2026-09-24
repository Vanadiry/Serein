package store

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// ValidateDevSource 校验本地开发规则源
func ValidateDevSource(path string, ruleValues map[string]map[string]string) ([]RuleIssue, error) {
	p, err := expandHome(path)
	if err != nil {
		return nil, err
	}
	info, err := loadSourceJSONFile(p)
	if err != nil {
		return nil, fmt.Errorf("读取 %s 失败: %w", path, err)
	}
	var issues []RuleIssue
	validateSourceDir(filepath.Dir(p), "", info, ruleValues, &issues)
	if issues == nil {
		issues = []RuleIssue{}
	}
	return issues, nil
}

func expandHome(p string) (string, error) {
	if p == "~" || strings.HasPrefix(p, "~/") {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", err
		}
		p = filepath.Join(home, strings.TrimPrefix(p, "~"))
	}
	return filepath.Abs(p)
}

func loadSourceJSONFile(path string) (*SourceInfo, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var s SourceInfo
	if err := json.Unmarshal(data, &s); err != nil {
		return nil, err
	}
	return &s, nil
}

func fileExists(p string) bool {
	st, err := os.Stat(p)
	return err == nil && !st.IsDir()
}

// validateSourceDir 递归校验；display 为该目录相对根的展示路径
func validateSourceDir(dir, display string, s *SourceInfo, ruleValues map[string]map[string]string, issues *[]RuleIssue) {
	if s.Type == "list" {
		for _, f := range s.Files {
			rel := filepath.ToSlash(f)
			child := filepath.ToSlash(filepath.Join(display, filepath.Dir(filepath.FromSlash(f))))
			if child == "." {
				child = ""
			}
			full := filepath.Join(dir, filepath.FromSlash(f))
			if !fileExists(full) {
				*issues = append(*issues, RuleIssue{Level: "error", Message: filepath.ToSlash(filepath.Join(display, rel)) + ": 文件不存在"})
				continue
			}
			sub, err := loadSourceJSONFile(full)
			if err != nil {
				*issues = append(*issues, RuleIssue{Level: "error", Message: filepath.ToSlash(filepath.Join(display, rel)) + ": 解析失败 " + err.Error()})
				continue
			}
			validateSourceDir(filepath.Dir(full), child, sub, ruleValues, issues)
		}
		return
	}

	// rules：逐条校验 + 多余文件检查
	listed := make(map[string]bool, len(s.Files))
	for _, f := range s.Files {
		listed[filepath.Base(filepath.FromSlash(f))] = true
		mp := filepath.ToSlash(filepath.Join(display, f))
		full := filepath.Join(dir, filepath.FromSlash(f))
		if !fileExists(full) {
			*issues = append(*issues, RuleIssue{Level: "error", Message: mp + ": 文件不存在"})
			continue
		}
		_, is, err := ParseRuleFile(full, ruleValues)
		if err != nil {
			*issues = append(*issues, RuleIssue{Level: "error", Message: mp + ": 解析失败 " + err.Error()})
			continue
		}
		for _, x := range is {
			x.Message = mp + ": " + x.Message
			*issues = append(*issues, x)
		}
	}

	entries, err := os.ReadDir(dir)
	if err != nil {
		return
	}
	for _, e := range entries {
		if e.IsDir() || filepath.Ext(e.Name()) != ".toml" {
			continue
		}
		if !listed[e.Name()] {
			*issues = append(*issues, RuleIssue{Level: "warn", Message: filepath.ToSlash(filepath.Join(display, e.Name())) + ": 未在该规则源中列出"})
		}
	}
}
