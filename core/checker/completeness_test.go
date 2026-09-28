package checker

import (
	"context"
	"net/http"
	"strings"
	"testing"
)

// "配了却取不到"必须报错：平台判失败时用户知道要重试，"有更新"却装不上最难查
// 直接测判定函数，避免走网络（SSRF 守卫会拦 httptest 的 127.0.0.1）
func TestCheckCompleteness(t *testing.T) {
	const anyPos = 1 // 任意非 nil 的 position
	cases := []struct {
		name    string
		vr      PlatformResult
		vType   string
		vPos    any
		dType   string
		dPos    any
		wantSub string // "" = 不该报错
	}{
		{"都取到", PlatformResult{LatestVersion: "1.0", URL: "https://x/y.zip"},
			"json", anyPos, "json", anyPos, ""},
		{"版本空", PlatformResult{URL: "https://x/y.zip"},
			"json", anyPos, "json", anyPos, "未能提取版本号"},
		{"版本只有空白", PlatformResult{LatestVersion: "  ", URL: "https://x/y.zip"},
			"json", anyPos, "json", anyPos, "未能提取版本号"},
		{"链接 nil", PlatformResult{LatestVersion: "1.0"},
			"json", anyPos, "json", anyPos, "未能提取下载链接"},
		{"链接空串", PlatformResult{LatestVersion: "1.0", URL: ""},
			"json", anyPos, "json", anyPos, "未能提取下载链接"},
		{"链接空数组", PlatformResult{LatestVersion: "1.0", URL: []string{}},
			"json", anyPos, "json", anyPos, "未能提取下载链接"},
		{"两个都空", PlatformResult{},
			"json", anyPos, "json", anyPos, "未能提取版本号"},
		// 规则没要求某个字段时不算失败
		{"只要求版本", PlatformResult{LatestVersion: "1.0"},
			"json", anyPos, "json", nil, ""},
		{"只要求链接", PlatformResult{URL: "https://x/y.zip"},
			"json", nil, "json", anyPos, ""},
		{"都没要求", PlatformResult{}, "json", nil, "json", nil, ""},
		// direct 恒算尝试过：值就是配置里的字面量，为空即配置缺失
		{"direct 版本字面量为空", PlatformResult{URL: "https://x"},
			"direct", nil, "direct", nil, "未能提取版本号"},
		{"direct 链接字面量为空", PlatformResult{LatestVersion: "1.0"},
			"direct", nil, "direct", nil, "未能提取下载链接"},
		{"direct 都齐", PlatformResult{LatestVersion: "1.0", URL: "https://x/1.0.zip"},
			"direct", nil, "direct", nil, ""},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			err := checkCompleteness(c.vr, c.vType, c.vPos, c.dType, c.dPos)
			if c.wantSub == "" {
				if err != nil {
					t.Fatalf("不该报错: %v", err)
				}
				return
			}
			if err == nil {
				t.Fatal("应报错")
			}
			if !strings.Contains(err.Error(), c.wantSub) {
				t.Errorf("错误信息应含 %q，实际 %v", c.wantSub, err)
			}
		})
	}
}

// direct 模式不发 HTTP，可以端到端跑：验证 d_url 缺省会回落到 url
func TestDirectLinkFallsBackToURL(t *testing.T) {
	// 只填 url：链接回落到 url，版本 = url
	pr, err := RunPlatformCheck(context.Background(), PlatformCheckConfig{
		OS: "windows", Type: "direct", URL: "https://cdn/1.0",
	}, http.DefaultClient)
	if err != nil {
		t.Fatalf("不该报错: %v", err)
	}
	if pr.URL != "https://cdn/1.0" || pr.LatestVersion != "https://cdn/1.0" {
		t.Errorf("回落结果 = %+v", pr)
	}
	// d_url 有值时优先用它，含 {version} 且版本非空时替换
	pr, err = RunPlatformCheck(context.Background(), PlatformCheckConfig{
		OS: "windows", Type: "direct", URL: "1.0", DType: "direct", DURL: "https://cdn/{version}.zip",
	}, http.DefaultClient)
	if err != nil {
		t.Fatalf("不该报错: %v", err)
	}
	if pr.URL != "https://cdn/1.0.zip" || pr.LatestVersion != "1.0" {
		t.Errorf("替换结果 = %+v", pr)
	}
	// 只填 v_url 时链接无来源，报错
	if _, err := RunPlatformCheck(context.Background(), PlatformCheckConfig{
		OS: "windows", Type: "direct", VURL: "1.0", DType: "direct", DURL: "",
	}, http.DefaultClient); err == nil {
		t.Error("只有 v_url 而无 url/d_url 时，链接无来源，应报错")
	}
}

func TestURLEmpty(t *testing.T) {
	empty := []any{nil, "", "   ", []string{}, []string{"", " "}, []any{""}, []any{nil}}
	for i, u := range empty {
		if !URLEmpty(u) {
			t.Errorf("case %d: %#v 应判为空", i, u)
		}
	}
	filled := []any{"https://x", []string{"https://x"}, []string{"", "https://x"}, []any{"", "https://x"}}
	for i, u := range filled {
		if URLEmpty(u) {
			t.Errorf("case %d: %#v 不应判为空", i, u)
		}
	}
}

// github 的版本与链接同时取不到时合并成一条，一个根因一条错
func TestGitHubNoUsableReleaseIsOneError(t *testing.T) {
	for _, body := range []string{
		`[]`, // 仓库没有任何 release
		`[{"tag_name":"v1","prerelease":true,"assets":[]}]`, // 全是预发布版
		`[{"prerelease":false,"assets":[]}]`,                // 都取不到 tag_name
	} {
		_, err := pickLatestRelease(mustParse(t, body), mustArr(t, body),
			GitHubConfig{Owner: "o", Repo: "r", DPosition: "exe"}, 3)
		if err == nil {
			t.Errorf("%s 应报错", body)
			continue
		}
		if !strings.Contains(err.Error(), "未找到可用的 release") {
			t.Errorf("%s → 错误信息 = %v", body, err)
		}
		if strings.Contains(err.Error(), "未能提取版本号") || strings.Contains(err.Error(), "未能提取下载链接") {
			t.Errorf("应合并成一条而非拆成两条: %v", err)
		}
	}
}

// github 取到版本但 d_position 匹配不到 asset 时报错，属于"有更新却装不上"的典型
func TestGitHubVersionWithoutAsset(t *testing.T) {
	body := `[{"tag_name":"v1.0","prerelease":false,"assets":[{"name":"a.dmg","browser_download_url":"https://x/a.dmg"}]}]`
	_, err := pickLatestRelease(mustParse(t, body), mustArr(t, body),
		GitHubConfig{Owner: "o", Repo: "r", DPosition: "exe"}, 3)
	if err == nil {
		t.Fatal("取到版本但无 asset 应报错")
	}
	if !strings.Contains(err.Error(), "未能匹配到 d_position 的下载链接") {
		t.Errorf("错误信息应说明是 d_position 没匹配上: %v", err)
	}
	// 规则没写 d_position 与匹配不上是两回事，提示要能分辨
	_, err = pickLatestRelease(mustParse(t, body), mustArr(t, body),
		GitHubConfig{Owner: "o", Repo: "r"}, 3)
	if err == nil || !strings.Contains(err.Error(), "缺少 d_position") {
		t.Errorf("缺 d_position 应报错: %v", err)
	}
}

// github：正常情况不受影响
func TestGitHubOK(t *testing.T) {
	body := `[{"tag_name":"v1.0","prerelease":false,"assets":[{"name":"a.exe","browser_download_url":"https://x/a.exe"}]}]`
	pr, err := pickLatestRelease(mustParse(t, body), mustArr(t, body),
		GitHubConfig{Owner: "o", Repo: "r", DPosition: "exe"}, 3)
	if err != nil {
		t.Fatalf("不该报错: %v", err)
	}
	if pr.LatestVersion != "v1.0" || pr.URL != "https://x/a.exe" {
		t.Errorf("结果 = %+v", pr)
	}
	// runGitHubCheck 对 direct 会把 DPosition 置 nil、链接改由 d_url 决定
	// 此时必须报错，给出"有版本没链接"会掩盖配置缺失
	pr, err = pickLatestRelease(mustParse(t, body), mustArr(t, body),
		GitHubConfig{Owner: "o", Repo: "r"}, 3)
	if err == nil || !strings.Contains(err.Error(), "缺少 d_position") {
		t.Errorf("无 d_position 应报错: %v", err)
	}
	_ = pr
}

func mustParse(t *testing.T, body string) any {
	t.Helper()
	v, err := parseJSON([]byte(body))
	if err != nil {
		t.Fatal(err)
	}
	return v
}

func mustArr(t *testing.T, body string) []any {
	t.Helper()
	arr, ok := mustParse(t, body).([]any)
	if !ok {
		t.Fatal("不是数组")
	}
	return arr
}
