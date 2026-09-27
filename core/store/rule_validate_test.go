package store

import (
	"os"
	"path/filepath"
	"sort"
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

// official_website 会被前端直接送进 openUrl。new URL() 对 "javascript:..." 不抛错，
// 浏览器分支一旦无校验就会执行规则里带来的脚本。
func TestValidateRejectsNonHTTPScheme(t *testing.T) {
	cases := []struct {
		name  string
		value string
		level string
	}{
		{"javascript", "javascript:alert(1)", "error"},
		{"data", "data:text/html,<script>alert(1)</script>", "error"},
		{"vbscript", "vbscript:msgbox(1)", "error"},
		{"file", "file:///etc/passwd", "error"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			r := Rule{Info: RuleInfo{AppID: "a", Name: "A", Platforms: []string{"windows"},
				OfficialWebsite: tc.value}}
			found := ""
			for _, is := range r.Validate() {
				if strings.Contains(is.Message, "official_website") {
					found = is.Level
				}
			}
			if found != tc.level {
				t.Errorf("level = %q, want %q（issues: %+v）", found, tc.level, r.Validate())
			}
		})
	}
}

// 合法的官网地址不得被误伤，含协议相对与 http
func TestValidateAcceptsNormalWebsite(t *testing.T) {
	for _, w := range []string{
		"https://example.com",
		"http://example.com/x",
		"//example.com/x", // 协议相对，交由 normalizeURL / 前端补全
		"",
	} {
		r := Rule{Info: RuleInfo{AppID: "a", Name: "A", Platforms: []string{"windows"},
			OfficialWebsite: w}}
		for _, is := range r.Validate() {
			if strings.Contains(is.Message, "official_website") {
				t.Errorf("%q 被误判: %s", w, is.Message)
			}
		}
	}
}

// config 里的 url / v_url / d_url 同样不得使用 javascript: 之类
func TestValidateRejectsNonHTTPSchemeInConfig(t *testing.T) {
	for _, field := range []string{"url", "v_url", "d_url"} {
		cfg := PlatConfig{Type: "json", VPosition: []any{"v"}, DPosition: []any{"d"}}
		switch field {
		case "url":
			cfg.URL = "javascript:alert(1)"
		case "v_url":
			cfg.VURL = "javascript:alert(1)"
		case "d_url":
			cfg.DURL = "javascript:alert(1)"
		}
		found := false
		for _, is := range validatePlatConfig("config", cfg) {
			if is.Level == "error" && strings.Contains(is.Message, field) {
				found = true
			}
		}
		if !found {
			t.Errorf("%s = javascript: 未报 error: %+v", field, validatePlatConfig("config", cfg))
		}
	}
}

// config 的 url / v_url / d_url 是服务端取数目标，必须是绝对 http(s)：
// 无 scheme 与协议相对都会被 http.NewRequest 拒绝
func TestValidateConfigURLMustBeAbsolute(t *testing.T) {
	for _, u := range []string{"example.com/api", "/relative", "not a url at all"} {
		cfg := PlatConfig{Type: "json", VPosition: []any{"v"}, DPosition: []any{"d"}, URL: u}
		found := false
		for _, is := range validatePlatConfig("config", cfg) {
			if is.Level == "warn" && strings.Contains(is.Message, "url") {
				found = true
			}
		}
		if !found {
			t.Errorf("config.url = %q 应报 warn（取数目标必须是绝对 http(s)）: %+v",
				u, validatePlatConfig("config", cfg))
		}
	}
	// 空字段不报
	cfg := PlatConfig{Type: "json", VPosition: []any{"v"}, DPosition: []any{"d"}}
	for _, is := range validatePlatConfig("config", cfg) {
		if strings.Contains(is.Message, "url") {
			t.Errorf("未设置 url 不应报错: %s", is.Message)
		}
	}
	// 协议相对地址合法：消费方是浏览器与下载器，两者都认 //host
	cfg.URL = "//example.com/x"
	for _, is := range validatePlatConfig("config", cfg) {
		if strings.Contains(is.Message, "url") {
			t.Errorf("协议相对 url 不应报错: %s", is.Message)
		}
	}
}

// 死字段：配了但当前 type 下不会被读取
func TestValidateDeadFields(t *testing.T) {
	cases := []struct {
		name string
		cfg  PlatConfig
		want []string // 期望报出的字段
	}{
		{"github 下 url/v_url/v_position 全死", PlatConfig{
			Type: "github", Owner: "o", Repo: "r", DPosition: "exe",
			URL: "https://x", VURL: "https://y", VPosition: []any{"tag_name"},
			VJoin: "-", DJoin: "-", BaseURL: "https://b",
		}, []string{"baseurl", "d_join", "url", "v_join", "v_position", "v_url"}},
		{"github 下 d_position 是活的（asset 名正则）", PlatConfig{
			Type: "github", Owner: "o", Repo: "r", DPosition: "exe",
		}, nil},
		{"github + d_type=direct 时 d_url 活", PlatConfig{
			Type: "github", DType: "direct", Owner: "o", Repo: "r", DURL: "https://x/{version}.zip",
		}, nil},
		{"github + 非 direct 时 d_url 死", PlatConfig{
			Type: "github", Owner: "o", Repo: "r", DPosition: "exe", DURL: "https://x",
		}, []string{"d_url"}},
		{"非 github 下 owner/repo/per_page/allow_prerelease 死", PlatConfig{
			Type: "json", URL: "https://x", VPosition: []any{"v"}, DPosition: []any{"u"},
			Owner: "o", Repo: "r", PerPage: 5, AllowPrerelease: true,
		}, []string{"allow_prerelease", "owner", "per_page", "repo"}},
		{"v_type=direct 时 v_position/v_join 死", PlatConfig{
			Type: "regex", VType: "direct", URL: "1.0", DPosition: []any{"u"},
			VPosition: []any{"v"}, VJoin: "-",
		}, []string{"v_join", "v_position"}},
		{"d_type=direct 时 d_position/d_join/baseurl 死", PlatConfig{
			Type: "regex", URL: "https://x", VPosition: []any{"v"},
			DType: "direct", DURL: "https://x/{version}.zip",
			DPosition: []any{"u"}, DJoin: "-", BaseURL: "https://b",
		}, []string{"baseurl", "d_join", "d_position"}},
		{"两侧都 direct 时 ua/headers 死", PlatConfig{
			Type: "direct", URL: "1.0", UA: "x", Headers: map[string]string{"A": "b"},
		}, []string{"headers", "ua"}},
		{"v_url 与 d_url 都指定时 url 死", PlatConfig{
			Type: "json", URL: "https://shared", VURL: "https://v", DURL: "https://d",
			VPosition: []any{"v"}, DPosition: []any{"u"},
		}, []string{"url"}},
		{"只指定 v_url 时 url 仍是回落来源（活）", PlatConfig{
			Type: "json", URL: "https://shared", VURL: "https://v", DPosition: []any{"u"},
		}, nil},
		{"什么都没配多余的", PlatConfig{
			Type: "json", URL: "https://x", VPosition: []any{"v"}, DPosition: []any{"u"},
		}, nil},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			vType := tc.cfg.VType
			if vType == "" {
				vType = tc.cfg.Type
			}
			dType := tc.cfg.DType
			if dType == "" {
				dType = tc.cfg.Type
			}
			got := deadFields(tc.cfg, vType, dType)
			if len(got) != len(tc.want) {
				t.Fatalf("死字段 = %v, want %v", keysOf(got), tc.want)
			}
			for _, w := range tc.want {
				if _, ok := got[w]; !ok {
					t.Errorf("缺少 %s（实际 %v）", w, keysOf(got))
				}
			}
			// 报出的 level 必须是 warn，且带上原因
			for _, is := range validateDeadFields("config", tc.cfg, vType, dType) {
				if is.Level != "warn" {
					t.Errorf("level 应为 warn: %+v", is)
				}
				if !strings.Contains(is.Message, "不会被读取") {
					t.Errorf("消息应说明不会被读取: %s", is.Message)
				}
			}
		})
	}
}

func keysOf(m map[string]string) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

// 端到端：PPSSPP 的 android 把 type 覆盖成 html_selector 后，owner/repo 变成死字段。
// 这是真实规则集里唯一的两处死字段，用它守住判据不误报。
func TestValidateRealWorldDeadFields(t *testing.T) {
	p := filepath.Join(os.Getenv("HOME"),
		".vSoft/Serein/rules/SereinRulesList_Official/v-github/PPSSPP.toml")
	if _, err := os.Stat(p); err != nil {
		t.Skip("本机没有规则集，跳过")
	}
	_, issues, err := ParseRuleFile(p, nil)
	if err != nil {
		t.Fatal(err)
	}
	found := map[string]bool{}
	for _, is := range issues {
		t.Logf("  %s: %s", is.Level, is.Message)
		if strings.Contains(is.Message, "config.android") &&
			(strings.Contains(is.Message, "owner") || strings.Contains(is.Message, "repo")) {
			found[is.Message] = true
		}
	}
	if len(found) != 2 {
		t.Errorf("应报出 android 的 owner/repo 两条死字段，实际 %d 条: %+v", len(found), issues)
	}
	// 整份规则集不该有别的死字段误报
	warns := 0
	for _, is := range issues {
		if is.Level == "warn" {
			warns++
		}
	}
	if warns != 2 {
		t.Errorf("warn 总数 = %d, want 2（判据若误报会更多）", warns)
	}
}

// github 的 owner/repo 会被拼进 API 路径，不校验字符集就能用 ? & 重塑查询串
func TestValidateRejectsBadGitHubSlug(t *testing.T) {
	bad := []string{"a?per_page=100&", "a/b", "a b", "a#frag", "../evil", "a%2Fb"}
	for _, s := range bad {
		cfg := PlatConfig{Type: "github", Owner: s, Repo: "r", DPosition: "exe"}
		found := false
		for _, is := range validatePlatConfig("config", cfg) {
			if is.Level == "error" && strings.Contains(is.Message, "owner") {
				found = true
			}
		}
		if !found {
			t.Errorf("owner = %q 应报 error", s)
		}
	}
	// 合法值不得误伤（含 GitHub 真实存在的 . 和 -）
	for _, s := range []string{"vanadiry", "Vanadiry", "some.user", "a-b", "a_b", "A1"} {
		cfg := PlatConfig{Type: "github", Owner: s, Repo: s, DPosition: "exe"}
		for _, is := range validatePlatConfig("config", cfg) {
			if is.Level == "error" {
				t.Errorf("owner/repo = %q 被误判: %s", s, is.Message)
			}
		}
	}
}
