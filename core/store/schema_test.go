package store

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func parseIssues(t *testing.T, body string) []RuleIssue {
	t.Helper()
	dir := t.TempDir()
	p := filepath.Join(dir, "r.toml")
	if err := os.WriteFile(p, []byte(body), 0644); err != nil {
		t.Fatal(err)
	}
	_, issues, err := ParseRuleFile(p, nil)
	if err != nil {
		t.Fatalf("parse error: %v", err)
	}
	return issues
}

func hasIssue(issues []RuleIssue, level, sub string) bool {
	for _, is := range issues {
		if is.Level == level && strings.Contains(is.Message, sub) {
			return true
		}
	}
	return false
}

func TestSchemaValidation(t *testing.T) {
	// 合法
	if is := parseIssues(t, `[info]
app_id = "x"
name = "X"
platforms = ["macos"]
[config]
type = "json"
url = "https://x"
`); len(is) != 0 {
		t.Fatalf("合法规则不应有告警: %+v", is)
	}

	// 未知字段
	if is := parseIssues(t, `[info]
app_id = "x"
name = "X"
platforms = ["macos"]
[config]
type = "json"
url = "https://x"
force_downloader = true
`); !hasIssue(is, "warn", "未知字段 force_downloader") {
		t.Fatalf("应报未知字段: %+v", is)
	}

	// 枚举非法
	if is := parseIssues(t, `[info]
app_id = "x"
name = "X"
platforms = ["macos"]
[config]
type = "json"
download_method = "downlaoder"
`); !hasIssue(is, "warn", "download_method") {
		t.Fatalf("应报非法取值: %+v", is)
	}

	// 必填缺失
	if is := parseIssues(t, `[info]
name = "X"
platforms = ["macos"]
`); !hasIssue(is, "error", "缺少必填字段 app_id") {
		t.Fatalf("应报必填缺失: %+v", is)
	}

	// 数值超范围
	if is := parseIssues(t, `[info]
app_id = "x"
name = "X"
platforms = ["macos"]
[config]
type = "github"
owner = "o"
repo = "r"
per_page = 0
`); !hasIssue(is, "warn", "不能小于") {
		t.Fatalf("应报范围错误: %+v", is)
	}
}
