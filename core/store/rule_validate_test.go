package store

import (
	"strings"
	"testing"
)

func TestRuleSemanticValidation(t *testing.T) {
	// v_type / d_type 覆盖时无需 type
	if is := parseIssues(t, `[info]
app_id = "p"
name = "P"
platforms = ["windows"]
[config]
v_url = "https://x/v"
v_type = "regex"
v_position = "v([0-9.]+)"
d_url = "https://x/d"
d_type = "html_selector"
d_position = { selector = "a", attr = "href" }
`); len(is) != 0 {
		t.Fatalf("v_type/d_type 覆盖时不应报错: %+v", is)
	}

	// github 缺 owner
	if is := parseIssues(t, `[info]
app_id = "g"
name = "G"
platforms = ["macos"]
[config]
type = "github"
repo = "r"
`); !hasIssue(is, "error", "缺少 owner") {
		t.Fatalf("应报 github 缺少 owner: %+v", is)
	}

	// regex 类型但 position 非字符串
	if is := parseIssues(t, `[info]
app_id = "r"
name = "R"
platforms = ["macos"]
[config]
type = "regex"
url = "https://x"
v_position = { selector = "a" }
`); !hasIssue(is, "warn", "应为正则字符串") {
		t.Fatalf("应报 position 形态错误: %+v", is)
	}

	// 正则无法编译
	if is := parseIssues(t, `[info]
app_id = "r2"
name = "R2"
platforms = ["macos"]
[config]
type = "regex"
url = "https://x"
v_position = "([0-9"
`); !hasIssue(is, "warn", "正则无法编译") {
		t.Fatalf("应报正则无法编译: %+v", is)
	}

	// url 缺少 scheme
	if is := parseIssues(t, `[info]
app_id = "u"
name = "U"
platforms = ["macos"]
[config]
type = "json"
url = "example.com/api"
`); !hasIssue(is, "warn", "不是 http(s) 链接") {
		t.Fatalf("应报 url scheme: %+v", is)
	}
}

// 拼写错误的解析器类型必须在规则检查阶段就报出来。
// 旧实现一路放行，直到运行时才炸出一个字面量 "<nil>" 的"版本号"。
func TestValidateRejectsUnknownParserType(t *testing.T) {
	cases := []struct {
		name string
		cfg  PlatConfig
		want string
	}{
		{"type 拼错", PlatConfig{Type: "jsno", VPosition: []any{"v"}, DPosition: []any{"d"}}, "jsno"},
		{"v_type 拼错", PlatConfig{Type: "json", VType: "xml1", VPosition: []any{"v"}, DPosition: []any{"d"}}, "xml1"},
		{"d_type 拼错", PlatConfig{Type: "json", DType: "regexp", VPosition: []any{"v"}, DPosition: []any{"d"}}, "regexp"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			issues := validatePlatConfig("config", tc.cfg)
			found := false
			for _, is := range issues {
				if is.Level == "error" && strings.Contains(is.Message, tc.want) {
					found = true
				}
			}
			if !found {
				t.Errorf("未报出未知类型 %q: %+v", tc.want, issues)
			}
		})
	}
}

// 合法的 6 种解析器不得被误伤
func TestValidateAcceptsAllParserTypes(t *testing.T) {
	for _, typ := range []string{"json", "xml", "regex", "html_selector", "github", "direct"} {
		cfg := PlatConfig{Type: typ, VPosition: []any{"v"}, DPosition: []any{"d"}}
		if typ == "github" {
			cfg.Owner, cfg.Repo = "o", "r"
		}
		if typ == "html_selector" {
			cfg.VPosition = map[string]any{"selector": ".v"}
			cfg.DPosition = map[string]any{"selector": ".d"}
		}
		for _, is := range validatePlatConfig("config", cfg) {
			if is.Level == "error" && strings.Contains(is.Message, "解析器") {
				t.Errorf("%s 被误判: %s", typ, is.Message)
			}
		}
	}
}

// 未知类型不应在 validatePosition 里重复报错
func TestValidatePositionSkipsUnknownType(t *testing.T) {
	if got := validatePosition("config", "v_position", []any{"v"}, "jsno"); len(got) != 0 {
		t.Errorf("未知类型应由 validatePlatConfig 报 error，validatePosition 不应重复: %+v", got)
	}
}

// IsValidParserType 与 validatePlatConfig 必须用同一份枚举
func TestIsValidParserType(t *testing.T) {
	for _, typ := range []string{"json", "xml", "regex", "html_selector", "github", "direct"} {
		if !IsValidParserType(typ) {
			t.Errorf("%s 应为合法类型", typ)
		}
	}
	for _, typ := range []string{"", "nope", "JSON", "Github", "jsonn"} {
		if IsValidParserType(typ) {
			t.Errorf("%q 不应被视为合法类型", typ)
		}
	}
	if len(validParserTypes) != 6 {
		t.Errorf("枚举大小 = %d，改动时记得同步 checker 侧与文档", len(validParserTypes))
	}
}
