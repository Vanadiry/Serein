// 检查运行器：整合规则、用户数据、Checker，组装统一的 API 返回格式。
package checker

import (
	"context"
	"errors"
	"net/http"
	"strings"

	"github.com/vanadiry/serein/core/httpx"
)

// isCanceled 判断错误是否来自 context 取消，取消时不应作为检查错误上报
func isCanceled(err error) bool {
	return errors.Is(err, context.Canceled)
}

// CheckRequest 一次检查的请求参数
type CheckRequest struct {
	AppID           string
	Name            string
	OfficialWebsite string
	RuleType        string // github / json / xml / ...
	Owner           string
	Repo            string
	PerPage         int
	Platforms       []PlatformCheckConfig
}

// PlatformCheckConfig 单个平台的检查配置（已合并）
type PlatformCheckConfig struct {
	OS              string
	Type            string
	URL             string
	UA              string
	Headers         map[string]string
	BaseURL         string
	VURL            string
	VType           string
	DURL            string
	DType           string
	VPosition       any
	DPosition       any
	VJoin           string
	DJoin           string
	CurrentVersion  string
	ForceDownloader bool
}

// CheckResponse API 返回的检查结果
type CheckResponse struct {
	AppID           string                   `json:"app_id"`
	Name            string                   `json:"name"`
	OfficialWebsite string                   `json:"official_website,omitempty"`
	Platforms       map[string]CheckPlatform `json:"platforms"`
}

// CheckPlatform 检查结果中单个平台的数据
type CheckPlatform struct {
	CurrentVersion  string `json:"current_version,omitempty"`
	LatestVersion   string `json:"latest_version,omitempty"`
	URL             any    `json:"url,omitempty"`
	Error           string `json:"error,omitempty"`
	ForceDownloader bool   `json:"force_downloader"`
}

// RunCheck 对一个软件执行检查，返回统一的 CheckResponse。
func RunCheck(ctx context.Context, req CheckRequest) (CheckResponse, error) {
	resp := CheckResponse{
		AppID:           req.AppID,
		Name:            req.Name,
		OfficialWebsite: req.OfficialWebsite,
		Platforms:       make(map[string]CheckPlatform),
	}

	client := httpx.NewClient()

	if req.RuleType == "github" {
		return runGitHubCheck(ctx, req, client)
	}

	for _, pc := range req.Platforms {
		pr, err := RunPlatformCheck(ctx, pc, client)
		if err != nil {
			cp := CheckPlatform{
				CurrentVersion:  pc.CurrentVersion,
				ForceDownloader: pc.ForceDownloader,
			}
			if !isCanceled(err) {
				cp.Error = err.Error()
			}
			resp.Platforms[pc.OS] = cp
			continue
		}
		resp.Platforms[pc.OS] = CheckPlatform{
			CurrentVersion:  pc.CurrentVersion,
			LatestVersion:   pr.LatestVersion,
			URL:             pr.URL,
			ForceDownloader: pc.ForceDownloader,
		}
	}
	return resp, nil
}

func runGitHubCheck(ctx context.Context, req CheckRequest, client *http.Client) (CheckResponse, error) {
	resp := CheckResponse{
		AppID:           req.AppID,
		Name:            req.Name,
		OfficialWebsite: req.OfficialWebsite,
		Platforms:       make(map[string]CheckPlatform),
	}

	if len(req.Platforms) == 0 {
		return resp, nil
	}

	cfg := GitHubConfig{
		Owner:   req.Owner,
		Repo:    req.Repo,
		PerPage: req.PerPage,
		Label:   req.Name,
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
		pr, err := CheckGitHub(ctx, cfg, client)
		if err != nil {
			cp := CheckPlatform{
				CurrentVersion:  pc.CurrentVersion,
				ForceDownloader: pc.ForceDownloader,
			}
			if !isCanceled(err) {
				cp.Error = err.Error()
			}
			resp.Platforms[pc.OS] = cp
			continue
		}
		if pc.DType == "direct" {
			dl := pc.DURL
			if strings.Contains(dl, "{version}") {
				if pr.LatestVersion == "" {
					resp.Platforms[pc.OS] = CheckPlatform{
						CurrentVersion:  pc.CurrentVersion,
						LatestVersion:   pr.LatestVersion,
						Error:           "d_url 包含 {version} 但未能获取到版本号",
						ForceDownloader: pc.ForceDownloader,
					}
					continue
				}
				dl = strings.ReplaceAll(dl, "{version}", pr.LatestVersion)
			}
			pr.URL = dl
		}
		resp.Platforms[pc.OS] = CheckPlatform{
			CurrentVersion:  pc.CurrentVersion,
			LatestVersion:   pr.LatestVersion,
			URL:             pr.URL,
			ForceDownloader: pc.ForceDownloader,
		}
	}
	return resp, nil
}
