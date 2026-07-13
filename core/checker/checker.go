// Checker 入口：CheckConfig、PlatformResult、CheckPlatform、HTTP 客户端。
package checker

import (
	"crypto/tls"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/vanadiry/serein/core/store"
)

// CheckConfig 一次检查的配置
type CheckConfig struct {
	URL         string
	Type        string
	UA          string
	Headers     map[string]string
	BaseURL     string
	Owner       string // GitHub
	Repo        string // GitHub
	GithubToken string
	VURL        string
	VType       string
	DURL        string
	DType       string
	VPosition   any
	DPosition   any
	VJoin       string
	DJoin       string
}

// PlatformResult 单个平台的检查结果
type PlatformResult struct {
	LatestVersion string
	URL           any // string 或 []string（GitHub 多 asset）
}

// NewClient 创建 HTTP 客户端
func NewClient() *http.Client {
	return &http.Client{
		Timeout: 30 * time.Second,
		Transport: &http.Transport{
			TLSClientConfig: &tls.Config{InsecureSkipVerify: false},
		},
	}
}

// RunPlatformCheck 对单个平台执行检查
func RunPlatformCheck(cfg CheckConfig, client *http.Client) (PlatformResult, error) {
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
		vr.LatestVersion = vURL
	} else if cfg.VPosition != nil {
		body, err := doRequest(client, vURL, cfg.UA, cfg.Headers)
		if err != nil {
			return vr, err
		}
		ver, err := extractValue(body, vType, cfg.VPosition, cfg.VJoin, "")
		if err != nil {
			return vr, err
		}
		vr.LatestVersion = toString(ver)
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
		body, err := doRequest(client, dURL, cfg.UA, cfg.Headers)
		if err != nil {
			return vr, err
		}
		dl, err := extractValue(body, dType, cfg.DPosition, cfg.DJoin, "")
		if err != nil {
			return vr, err
		}
		if cfg.BaseURL != "" {
			dl = cfg.BaseURL + toString(dl)
		} else {
			dl = toString(dl)
		}
		vr.URL = dl
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
	case "html_xpath":
		return extractXPathValue(body, pos, baseURL)
	default:
		store.Emit("warn", "[checker]", fmt.Sprintf("未知提取类型: %s", typ))
		return nil, nil
	}
}
