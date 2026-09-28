// Checker 入口：PlatformCheckConfig、PlatformResult、CheckPlatform、HTTP 客户端
package checker

import (
	"context"
	"fmt"
	"net/http"
	"sort"
	"strings"
	"sync/atomic"

	"github.com/vanadiry/serein/core/httpx"
	"github.com/vanadiry/serein/core/store"
)

// PlatformResult 单个平台的检查结果
type PlatformResult struct {
	LatestVersion string
	URL           any      // string 或 []string（GitHub 多 asset）
	Warnings      []string // 非致命的异常（不影响结果可用性）
}

// "配了却取不到"必须报错。判定依据是"本次是否尝试提取该字段"
// 规则没要求某个字段（position 为 nil）不算失败；要求了却拿到空值才算
// direct 模式没有"提取"这回事：值就是配置里的字面量，所以它恒算尝试过
// 为空即配置缺失。隐式取值的 github 不走这里，由 runGitHubCheck 负责
func vAttempted(vType string, vPos any) bool {
	if vType == "direct" {
		return true
	}
	return vPos != nil
}

func dAttempted(dType string, dPos any) bool {
	if dType == "direct" {
		return true
	}
	return dPos != nil
}

// URLEmpty 判断下载链接是否为空。URL 可能是 string / []string / []any / nil
// 导出供 server 侧（directCheckResponse）对 msvsix / openvsix 用同一套判定
func URLEmpty(u any) bool { return urlEmpty(u) }

func urlEmpty(u any) bool {
	switch v := u.(type) {
	case nil:
		return true
	case string:
		return strings.TrimSpace(v) == ""
	case []string:
		for _, s := range v {
			if strings.TrimSpace(s) != "" {
				return false
			}
		}
		return true
	case []any:
		for _, x := range v {
			if s, ok := x.(string); ok && strings.TrimSpace(s) != "" {
				return false
			}
		}
		return true
	}
	return false
}

// Warn 记录一条非致命异常，重复的只留一条
func (pr *PlatformResult) Warn(msg string) {
	for _, w := range pr.Warnings {
		if w == msg {
			return
		}
	}
	pr.Warnings = append(pr.Warnings, msg)
}

// 前后缀列表。拉取动态配置时会在运行时被 Set* 改写，而检查任务正并发读
// 所以走 atomic.Pointer 整体替换，裸切片赋值会被判定为数据竞争
var (
	versionPrefixes atomic.Pointer[[]string]
	versionSuffixes atomic.Pointer[[]string]
)

func affixes(in []string) *[]string {
	cp := append([]string(nil), in...)
	sort.Slice(cp, func(i, j int) bool { return len(cp[i]) > len(cp[j]) })
	return &cp
}

// list 读出当前的前后缀列表。atomic.Pointer 的零值是 nil
// 未调用过 Set* 时 Load() 返回 nil 指针，解引用会 panic
func list(p *atomic.Pointer[[]string]) []string {
	if v := p.Load(); v != nil {
		return *v
	}
	return nil
}

// SetVersionPrefixes 设置版本前缀。内部拷贝后再排序，不修改调用方切片
func SetVersionPrefixes(prefixes []string) { versionPrefixes.Store(affixes(prefixes)) }

// SetVersionSuffixes 设置版本后缀。内部拷贝后再排序，不修改调用方切片
func SetVersionSuffixes(suffixes []string) { versionSuffixes.Store(affixes(suffixes)) }

func stripVersionAffixes(ver string) string {
	for _, p := range list(&versionPrefixes) {
		if len(p) > 0 && strings.HasPrefix(ver, p) {
			ver = ver[len(p):]
			break
		}
	}
	for _, s := range list(&versionSuffixes) {
		if len(s) > 0 && strings.HasSuffix(ver, s) {
			ver = ver[:len(ver)-len(s)]
			break
		}
	}
	return ver
}

// checkParserTypes 校验版本号与下载链接两侧的解析器类型
func checkParserTypes(vType, dType string) error {
	if !store.IsValidParserType(vType) {
		return fmt.Errorf("未知版本号解析器 %q", vType)
	}
	if !store.IsValidParserType(dType) {
		return fmt.Errorf("未知下载解析器 %q", dType)
	}
	return nil
}

// RunPlatformCheck 对单个平台执行检查
func RunPlatformCheck(ctx context.Context, cfg PlatformCheckConfig, client *http.Client) (PlatformResult, error) {
	var vr PlatformResult

	// 版本号与下载链接的请求地址 / 解析器：d_url、d_type 覆盖 url、type
	vURL, vType := cfg.URL, cfg.Type
	if cfg.VURL != "" {
		vURL = cfg.VURL
	}
	if cfg.VType != "" {
		vType = cfg.VType
	}
	dURL, dType := cfg.URL, cfg.Type
	if cfg.DURL != "" {
		dURL = cfg.DURL
	}
	if cfg.DType != "" {
		dType = cfg.DType
	}

	// 前置校验解析器类型，必须早于任何网络请求：否则未知类型会先发一次请求
	// 拿到的是网络错误（请求成功后才炸出 "<nil>" 版本号）
	// 真实原因被掩盖，还白白浪费一次往返
	if err := checkParserTypes(vType, dType); err != nil {
		return vr, err
	}

	// 提取版本号
	if vType == "direct" {
		vr.LatestVersion = stripVersionAffixes(vURL)
	} else if cfg.VPosition != nil {
		body, err := httpx.Request(ctx, client, vURL, cfg.UA, cfg.Headers)
		if err != nil {
			return vr, err
		}
		ver, err := extractValue(body, vType, cfg.VPosition, cfg.VJoin, "")
		if err != nil {
			return vr, err
		}
		vr.LatestVersion = stripVersionAffixes(toString(ver))
	}

	// 提取下载链接
	if dType == "direct" {
		dl, err := resolveDirectURL(dURL, vr.LatestVersion)
		if err != nil {
			return vr, err
		}
		vr.URL = dl
	} else if cfg.DPosition != nil {
		body, err := httpx.Request(ctx, client, dURL, cfg.UA, cfg.Headers)
		if err != nil {
			return vr, err
		}
		dl, err := extractValue(body, dType, cfg.DPosition, cfg.DJoin, "")
		if err != nil {
			return vr, err
		}
		vr.URL = httpx.JoinURL(cfg.BaseURL, toString(dl))
	}

	// 尝试过却取不到时报错
	// 平台判失败时用户知道要重试，"有更新"却装不上最难查
	// 前者列表里不显示版本号、没有下载按钮；后者用户会以为有更新并去确认
	if err := checkCompleteness(vr, vType, cfg.VPosition, dType, cfg.DPosition); err != nil {
		return vr, err
	}
	return vr, nil
}

// checkCompleteness 判定"尝试过却取不到"的字段。规则没要求某个字段
// （position 为 nil）不算失败，那种情况是配置只关心另一半
func checkCompleteness(vr PlatformResult, vType string, vPos any, dType string, dPos any) error {
	if vAttempted(vType, vPos) && strings.TrimSpace(vr.LatestVersion) == "" {
		return fmt.Errorf("未取到版本号：%s 没能取到内容", vType)
	}
	if dAttempted(dType, dPos) && urlEmpty(vr.URL) {
		return fmt.Errorf("未取到下载链接：%s 没能取到内容", dType)
	}
	return nil
}

// resolveDirectURL 直通模式：d_url 含 {version} 时替换；版本号为空则报错
// 不补协议：//host 原样透出，浏览器与外部下载器都认，Go 客户端由调用方取数前解析
func resolveDirectURL(durl, version string) (string, error) {
	if !strings.Contains(durl, "{version}") {
		return durl, nil
	}
	if version == "" {
		return "", fmt.Errorf("d_url 包含 {version} 但未能获取到版本号")
	}
	return strings.ReplaceAll(durl, "{version}", version), nil
}

// extractValue 根据 type 从响应体中提取一个值（版本号或下载链接）
// join 为空走单路径，非空走多路径拼接
// 未知类型必须报错，不能返回 (nil, nil)：那会让调用方的 toString(nil) 产出字面量
// "<nil>"，被当成合法版本号写进结果并最终持久化进 user/software.json
func extractValue(body []byte, typ string, pos any, join, baseURL string) (any, error) {
	switch typ {
	case "json":
		return extractJSONValue(body, pos, join)
	case "xml":
		return extractXMLValue(body, pos, join)
	case "regex":
		return extractRegexValue(body, pos)
	case "html_selector":
		return extractSelectorValue(body, pos, baseURL)
	case "github", "direct":
		// 这两类不经由本函数取值：github 由 CheckGitHub 处理
		// direct 的"值"就是配置里的字面量（见 RunPlatformCheck）
		return nil, fmt.Errorf("提取类型 %q 不经由 extractValue 取值", typ)
	default:
		return nil, fmt.Errorf("未知提取类型 %q", typ)
	}
}
