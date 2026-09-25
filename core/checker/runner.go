// 检查运行器：整合规则、用户数据、Checker，组装统一的 API 返回格式。
package checker

import (
	"context"
	"errors"
	"net/http"

	"github.com/vanadiry/serein/core/httpx"
	"github.com/vanadiry/serein/core/store"
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
	Platforms       []PlatformCheckConfig
}

// PlatformCheckConfig 单个平台的检查配置（已合并）
type PlatformCheckConfig struct {
	OS               string
	Type             string
	URL              string
	UA               string
	Headers          map[string]string
	BaseURL          string
	Owner            string
	Repo             string
	PerPage          int
	VURL             string
	VType            string
	DURL             string
	DType            string
	VPosition        any
	DPosition        any
	VJoin            string
	DJoin            string
	CurrentVersion   string
	DownloadMethod   string
	DownloadViaProxy bool
	DownloadName     string
	AllowPrerelease  bool
	Label            string // 事件标题里的标识
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
	CurrentVersion   string `json:"current_version,omitempty"`
	LatestVersion    string `json:"latest_version,omitempty"`
	URL              any    `json:"url,omitempty"`
	Error            string `json:"error,omitempty"`
	DownloadMethod   string `json:"download_method,omitempty"`
	DownloadViaProxy bool   `json:"download_via_proxy,omitempty"`
	DownloadName     string `json:"download_name,omitempty"`
	ProxyURL         any    `json:"proxy_url,omitempty"`
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
		resp.Platforms[pc.OS] = newCheckPlatform(pc, pr, err)
	}
	return resp, nil
}

// newCheckPlatform 组装单个平台的检查结果；err 非空时仅带错误（取消除外）
func newCheckPlatform(pc PlatformCheckConfig, pr PlatformResult, err error) CheckPlatform {
	cp := CheckPlatform{
		CurrentVersion:   pc.CurrentVersion,
		DownloadMethod:   pc.DownloadMethod,
		DownloadViaProxy: pc.DownloadViaProxy,
		DownloadName:     pc.DownloadName,
	}
	if err != nil {
		if !isCanceled(err) {
			cp.Error = err.Error()
		}
		return cp
	}
	cp.LatestVersion = pr.LatestVersion
	cp.URL = pr.URL
	return cp
}

// NewPlatformCheckConfig 由规则平台配置组装检查配置
func NewPlatformCheckConfig(os, label, currentVer string, pc store.PlatConfig) PlatformCheckConfig {
	return PlatformCheckConfig{
		OS:               os,
		Type:             pc.Type,
		URL:              pc.URL,
		UA:               pc.UA,
		Headers:          pc.Headers,
		BaseURL:          pc.BaseURL,
		Owner:            pc.Owner,
		Repo:             pc.Repo,
		PerPage:          pc.PerPage,
		VURL:             pc.VURL,
		VType:            pc.VType,
		DURL:             pc.DURL,
		DType:            pc.DType,
		VPosition:        pc.VPosition,
		DPosition:        pc.DPosition,
		VJoin:            pc.VJoin,
		DJoin:            pc.DJoin,
		CurrentVersion:   currentVer,
		DownloadMethod:   pc.DownloadMethod,
		DownloadViaProxy: pc.DownloadViaProxy,
		DownloadName:     pc.DownloadName,
		AllowPrerelease:  pc.AllowPrerelease,
		Label:            label,
	}
}

func runGitHubCheck(ctx context.Context, req CheckRequest, client *http.Client) (CheckResponse, error) {
	resp := CheckResponse{
		AppID:           req.AppID,
		Name:            req.Name,
		OfficialWebsite: req.OfficialWebsite,
		Platforms:       make(map[string]CheckPlatform),
	}

	for _, pc := range req.Platforms {
		cfg := GitHubConfig{
			Owner:           pc.Owner,
			Repo:            pc.Repo,
			PerPage:         pc.PerPage,
			UA:              pc.UA,
			Headers:         pc.Headers,
			AllowPrerelease: pc.AllowPrerelease,
			Label:           req.Name,
		}
		if pc.DType == "direct" {
			cfg.DPosition = nil
		} else {
			cfg.DPosition = pc.DPosition
		}
		pr, err := CheckGitHub(ctx, cfg, client)
		if err != nil {
			resp.Platforms[pc.OS] = newCheckPlatform(pc, pr, err)
			continue
		}
		if pc.DType == "direct" {
			dl, err := resolveDirectURL(pc.DURL, pr.LatestVersion)
			if err != nil {
				resp.Platforms[pc.OS] = newCheckPlatform(pc, PlatformResult{}, err)
				continue
			}
			pr.URL = dl
		}
		resp.Platforms[pc.OS] = newCheckPlatform(pc, pr, nil)
	}
	return resp, nil
}
