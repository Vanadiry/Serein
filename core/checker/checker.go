// Checker 入口：PlatformCheckConfig、PlatformResult、CheckPlatform、HTTP 客户端
package checker

import (
	"fmt"
	"net/http"
	"sort"
	"strings"

	"github.com/vanadiry/serein/core/events"
	"github.com/vanadiry/serein/core/httpx"
)

// PlatformResult 单个平台的检查结果
type PlatformResult struct {
	LatestVersion string
	URL           any // string 或 []string（GitHub 多 asset）
}

var versionPrefixes []string
var versionSuffixes []string

func SetVersionPrefixes(prefixes []string) {
	sort.Slice(prefixes, func(i, j int) bool { return len(prefixes[i]) > len(prefixes[j]) })
	versionPrefixes = prefixes
}

func SetVersionSuffixes(suffixes []string) {
	sort.Slice(suffixes, func(i, j int) bool { return len(suffixes[i]) > len(suffixes[j]) })
	versionSuffixes = suffixes
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

// RunPlatformCheck 对单个平台执行检查
func RunPlatformCheck(cfg PlatformCheckConfig, client *http.Client) (PlatformResult, error) {
	var vr PlatformResult

	// 提取版本号
	vURL := cfg.URL
	vType := cfg.Type
	if cfg.VURL != "" {
		vURL = cfg.VURL
	}
	if cfg.VType != "" {
		vType = cfg.VType
	}

	if vType == "direct" {
		vr.LatestVersion = stripVersionAffixes(vURL)
	} else if cfg.VPosition != nil {
		body, err := httpx.Request(client, vURL, cfg.UA, cfg.Headers)
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
	dURL := cfg.URL
	dType := cfg.Type
	if cfg.DURL != "" {
		dURL = cfg.DURL
	}
	if cfg.DType != "" {
		dType = cfg.DType
	}

	if dType == "direct" {
		dl := dURL
		if strings.Contains(dl, "{version}") {
			if vr.LatestVersion == "" {
				return vr, fmt.Errorf("d_url 包含 {version} 但未能获取到版本号")
			}
			dl = strings.ReplaceAll(dl, "{version}", vr.LatestVersion)
		}
		vr.URL = dl
	} else if cfg.DPosition != nil {
		body, err := httpx.Request(client, dURL, cfg.UA, cfg.Headers)
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

// extractValue 根据 type 从响应体中提取一个值（版本号或下载链接）。
// join 为空 → 单路径；非空 → 多路径拼接。
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
	default:
		events.Emit("warn", "[checker]", fmt.Sprintf("未知提取类型: %s", typ))
		return nil, nil
	}
}
