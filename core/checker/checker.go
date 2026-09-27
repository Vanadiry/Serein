// Checker 入口：PlatformCheckConfig、PlatformResult、CheckPlatform、HTTP 客户端
package checker

import (
	"context"
	"fmt"
	"net/http"
	"sort"
	"strings"

	"github.com/vanadiry/serein/core/httpx"
	"github.com/vanadiry/serein/core/store"
)

// PlatformResult 单个平台的检查结果
type PlatformResult struct {
	LatestVersion string
	URL           any // string 或 []string（GitHub 多 asset）
}

var versionPrefixes []string
var versionSuffixes []string

// SetVersionPrefixes 设置版本前缀。内部拷贝后再排序，不修改调用方切片
func SetVersionPrefixes(prefixes []string) {
	cp := append([]string(nil), prefixes...)
	sort.Slice(cp, func(i, j int) bool { return len(cp[i]) > len(cp[j]) })
	versionPrefixes = cp
}

// SetVersionSuffixes 设置版本后缀。内部拷贝后再排序，不修改调用方切片
func SetVersionSuffixes(suffixes []string) {
	cp := append([]string(nil), suffixes...)
	sort.Slice(cp, func(i, j int) bool { return len(cp[i]) > len(cp[j]) })
	versionSuffixes = cp
}

func stripVersionAffixes(ver string) string {
	for _, p := range versionPrefixes {
		if len(p) > 0 && strings.HasPrefix(ver, p) {
			ver = ver[len(p):]
			break
		}
	}
	for _, s := range versionSuffixes {
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

	// 前置校验解析器类型，必须早于任何网络请求：否则未知类型会先发一次请求，
	// 拿到的是网络错误（请求成功后才炸出 "<nil>" 版本号），
	// 真实原因被掩盖，还白白浪费一次往返。
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

	return vr, nil
}

// resolveDirectURL 直通模式：d_url 含 {version} 时替换；版本号为空则报错。
// 不补协议：//host 原样透出，浏览器与外部下载器都认，Go 客户端由调用方取数前解析。
func resolveDirectURL(durl, version string) (string, error) {
	if !strings.Contains(durl, "{version}") {
		return durl, nil
	}
	if version == "" {
		return "", fmt.Errorf("d_url 包含 {version} 但未能获取到版本号")
	}
	return strings.ReplaceAll(durl, "{version}", version), nil
}

// extractValue 根据 type 从响应体中提取一个值（版本号或下载链接）。
// join 为空 → 单路径；非空 → 多路径拼接。
//
// 未知类型必须报错，不能返回 (nil, nil)：那会让调用方的 toString(nil) 产出字面量
// "<nil>"，被当成合法版本号写进结果并最终持久化进 user/software.json。
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
		// 这两类不经由本函数取值：github 由 CheckGitHub 处理，
		// direct 的"值"就是配置里的字面量（见 RunPlatformCheck）
		return nil, fmt.Errorf("提取类型 %q 不经由 extractValue 取值", typ)
	default:
		return nil, fmt.Errorf("未知提取类型 %q", typ)
	}
}
