// 检查运行器：整合规则、用户数据、Checker，组装统一的 API 返回格式。
package checker

import (
	"net/http"
	"strings"
)

// CheckGitHub 单平台便捷调用（包装 CheckGitHubAll）。
func CheckGitHub(cfg CheckConfig, client *http.Client) (PlatformResult, error) {
	latest, _, err := CheckGitHubAll(cfg, client)
	return latest, err
}

// CheckRequest 一次检查的请求参数
type CheckRequest struct {
	AppID            string
	Name            string
	OfficialWebsite string
	RuleType        string // github / json / xml / ...
	Owner           string
	Repo            string
	GithubToken     string
	Platforms       []PlatformCheckConfig
}

// PlatformCheckConfig 单个平台的检查配置（已合并）
type PlatformCheckConfig struct {
	OS             string
	Type           string
	URL            string
	UA             string
	Headers        map[string]string
	BaseURL        string
	VURL           string
	VType          string
	DURL           string
	DType          string
	VPosition      any
	DPosition      any
	VJoin          string
	DJoin          string
	CurrentVersion string
}

// CheckResponse API 返回的检查结果
type CheckResponse struct {
	AppID            string                   `json:"app_id"`
	Name            string                   `json:"name"`
	OfficialWebsite string                   `json:"official_website,omitempty"`
	Platforms       map[string]CheckPlatform `json:"platforms"`
}

// CheckPlatform 检查结果中单个平台的数据
type CheckPlatform struct {
	CurrentVersion string `json:"current_version,omitempty"`
	LatestVersion  string `json:"latest_version,omitempty"`
	URL            any    `json:"url,omitempty"`
	Error          string `json:"error,omitempty"`
}

// RunCheck 对一个软件执行检查，返回统一的 CheckResponse。
func RunCheck(req CheckRequest) (CheckResponse, error) {
	resp := CheckResponse{
		AppID:            req.AppID,
		Name:            req.Name,
		OfficialWebsite: req.OfficialWebsite,
		Platforms:       make(map[string]CheckPlatform),
	}

	client := NewClient()

	if req.RuleType == "github" {
		return runGitHubCheck(req, client)
	}

	for _, pc := range req.Platforms {
		cfg := CheckConfig{
			URL:       pc.URL,
			Type:      pc.Type,
			UA:        pc.UA,
			Headers:   pc.Headers,
			BaseURL:   pc.BaseURL,
			VURL:      pc.VURL,
			VType:     pc.VType,
			DURL:      pc.DURL,
			DType:     pc.DType,
			VPosition: pc.VPosition,
			DPosition: pc.DPosition,
			VJoin:     pc.VJoin,
			DJoin:     pc.DJoin,
		}

		pr, err := RunPlatformCheck(cfg, client)
		if err != nil {
			resp.Platforms[pc.OS] = CheckPlatform{
				CurrentVersion: pc.CurrentVersion,
				Error:          err.Error(),
			}
			continue
		}
		resp.Platforms[pc.OS] = CheckPlatform{
			CurrentVersion: pc.CurrentVersion,
			LatestVersion:  pr.LatestVersion,
			URL:            pr.URL,
		}
	}
	return resp, nil
}

func runGitHubCheck(req CheckRequest, client *http.Client) (CheckResponse, error) {
	resp := CheckResponse{
		AppID:            req.AppID,
		Name:            req.Name,
		OfficialWebsite: req.OfficialWebsite,
		Platforms:       make(map[string]CheckPlatform),
	}

	if len(req.Platforms) == 0 {
		return resp, nil
	}

	cfg := CheckConfig{
		Owner: req.Owner,
		Repo:  req.Repo,
			GithubToken: req.GithubToken,
	}
	if len(req.Platforms) > 0 {
		cfg.UA = req.Platforms[0].UA
		cfg.Headers = req.Platforms[0].Headers
	}

	for _, pc := range req.Platforms {
		if pc.DType == "direct" {
			cfg.DPosition = nil
		} else {
			cfg.DPosition = pc.DPosition
		}
		pr, err := CheckGitHub(cfg, client)
		if err != nil {
			resp.Platforms[pc.OS] = CheckPlatform{
				CurrentVersion: pc.CurrentVersion,
				Error:          err.Error(),
			}
			continue
		}
		if pc.DType == "direct" {
			dl := pc.DURL
			if strings.Contains(dl, "{version}") {
				if pr.LatestVersion == "" {
					resp.Platforms[pc.OS] = CheckPlatform{
						CurrentVersion: pc.CurrentVersion,
						LatestVersion:  pr.LatestVersion,
						Error:          "d_url 包含 {version} 但未能获取到版本号",
					}
					continue
				}
				dl = strings.ReplaceAll(dl, "{version}", pr.LatestVersion)
			}
			pr.URL = dl
		}
		resp.Platforms[pc.OS] = CheckPlatform{
			CurrentVersion: pc.CurrentVersion,
			LatestVersion:  pr.LatestVersion,
			URL:            pr.URL,
		}
	}
	return resp, nil
}
