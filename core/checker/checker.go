// Checker 入口：CheckConfig、PlatformResult、CheckPlatform、HTTP 客户端。
package checker

import (
	"crypto/tls"
	"net/http"
	"time"
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
	// 前置请求链由外部执行，此处 cfg.URL 已是最终请求地址
	body, err := doRequest(client, cfg.URL, cfg.UA, cfg.Headers)
	if err != nil {
		return PlatformResult{}, err
	}

	var vr PlatformResult

	// 提取版本号（不受 baseURL 影响）
	if cfg.VPosition != nil {
		ver, err := extractValue(body, cfg.Type, cfg.VPosition, cfg.VJoin, "")
		if err != nil {
			return vr, err
		}
		vr.LatestVersion = toString(ver)
	}

	// 提取下载链接
	if cfg.DPosition != nil {
		dl, err := extractValue(body, cfg.Type, cfg.DPosition, cfg.DJoin, "")
		if err != nil {
			return vr, err
		}
		// baseURL 直接与 d_position 结果拼接
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
		return nil, nil
	}
}
