package store

import "testing"

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
