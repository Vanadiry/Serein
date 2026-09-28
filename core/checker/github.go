// GitHub Release 解析器。一次 /releases 请求，返回最新（非预发布）版本
// 复用 JSON 步进引擎：[0, "tag_name"] 为最新
package checker

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"regexp"

	"github.com/vanadiry/serein/core/httpx"
)

const (
	githubAPI = "https://api.github.com/repos"
	// repos API 默认返回 30 条数据，可能触发相应上限
	// 以 defaultPerPage 限制返回 3 条，可以覆盖大多数仓库，规则可用 per_page 覆盖
	defaultPerPage = 3
)

// GitHubConfig GitHub Release 检查参数
type GitHubConfig struct {
	Owner           string
	Repo            string
	PerPage         int
	UA              string
	Headers         map[string]string
	DPosition       any
	AllowPrerelease bool   // true 时不过滤 prerelease，直接取最新一条
	Label           string // 事件标题里的标识，为空时回退 owner/repo
}

// CheckGitHub 请求 /releases，返回最新非预发布版本及其下载链接
func CheckGitHub(ctx context.Context, cfg GitHubConfig, client *http.Client) (PlatformResult, error) {
	perPage := cfg.PerPage
	if perPage <= 0 {
		perPage = defaultPerPage
	}
	url := fmt.Sprintf("%s/%s/%s/releases?per_page=%d", githubAPI, cfg.Owner, cfg.Repo, perPage)
	headers := mergeHeaders(cfg.Headers, "application/vnd.github+json")
	body, err := httpx.Request(ctx, client, url, cfg.UA, headers)
	if err != nil {
		return PlatformResult{}, fmt.Errorf("github: %w", err)
	}

	root, err := parseJSON(body)
	if err != nil {
		return PlatformResult{}, fmt.Errorf("github: %w", err)
	}

	arr, ok := root.([]any)
	if !ok {
		// 尝试解析错误消息
		if errMap, ok := root.(map[string]any); ok {
			if msg, ok := errMap["message"].(string); ok {
				return PlatformResult{}, fmt.Errorf("github: %s", msg)
			}
		}
		return PlatformResult{}, fmt.Errorf("github: unexpected response format")
	}

	return pickLatestRelease(root, arr, cfg, perPage)
}

// pickLatestRelease 从已解析的 releases 数组里选出目标 release，取出版本号与下载链接
// 与 HTTP 无关，便于单测，githubAPI 是硬编码常量，没法指向测试服务
// warn 收集到 PlatformResult.Warnings，由 server 归入本次检查的错误列表
// 不走事件总线，那是程序运行期的问题通道，不该混进某一次 task 的产出
// 必须是局部状态，本函数会被并发调用
func pickLatestRelease(root any, arr []any, cfg GitHubConfig, perPage int) (PlatformResult, error) {
	var warns []string
	collect := func(msg string) {
		for _, w := range warns {
			if w == msg {
				return
			}
		}
		warns = append(warns, msg)
	}

	var latest PlatformResult
	var latestFound bool
	for i := range arr {
		tag, err := extractGitHubVersion(root, i)
		if err != nil {
			continue
		}
		if !cfg.AllowPrerelease && isGitHubPrerelease(root, i, collect) {
			continue
		}
		latest = PlatformResult{}
		latest.LatestVersion = tag
		latest.URL = extractGitHubAssets(root, i, cfg.DPosition, collect)
		latestFound = true
		break
	}

	if !latestFound {
		// 版本号与下载链接同时取不到时根因唯一，即没有可用的 release
		// 拆成"未能提取版本"与"未能提取链接"两条会让用户以为是两个问题
		reason := "仓库没有任何 release"
		if len(arr) > 0 {
			if cfg.AllowPrerelease {
				reason = fmt.Sprintf("前 %d 个 release 都取不到 tag_name", perPage)
			} else {
				reason = fmt.Sprintf("前 %d 个 release 全是预发布版，可增大 per_page", perPage)
			}
		}
		latest.Warnings = warns
		return latest, fmt.Errorf("未找到可用的 release：%s", reason)
	}

	// 取到版本号但没有下载链接：同样算失败。否则 UI 会显示"有更新"
	// 用户点确认后版本落盘，软件却永远装不上，且没有任何提示
	if urlEmpty(latest.URL) {
		why := "未能匹配到 d_position 的下载链接"
		if cfg.DPosition == nil {
			why = "规则缺少 d_position"
		}
		latest.Warnings = warns
		return latest, errors.New(why)
	}

	latest.Warnings = warns
	return latest, nil
}

func extractGitHubVersion(root any, idx int) (string, error) {
	ver, err := stepJSON(root, []any{int64(idx), "tag_name"}, "")
	if err != nil {
		return "", err
	}
	return stripVersionAffixes(fmt.Sprintf("%v", ver)), nil
}

func extractGitHubAssets(root any, idx int, dPosition any, warn func(string)) any {
	if dPosition == nil {
		return nil
	}
	assetReStr, ok := dPosition.(string)
	if !ok {
		return nil
	}
	assetRe, err := regexp.Compile(assetReStr)
	if err != nil {
		warn(fmt.Sprintf("规则正则表达式编译失败：%v", err))
		return nil
	}

	assets, err := stepJSON(root, []any{int64(idx), "assets"}, "")
	if err != nil {
		warn(fmt.Sprintf("GitHub assets JSON 解析失败：%v", err))
		return nil
	}
	assetArr, ok := assets.([]any)
	if !ok {
		return nil
	}

	var urls []string
	for _, a := range assetArr {
		asset, ok := a.(map[string]any)
		if !ok {
			continue
		}
		name, _ := asset["name"].(string)
		if assetRe.MatchString(name) {
			if dl, ok := asset["browser_download_url"].(string); ok {
				urls = append(urls, dl)
			}
		}
	}
	if len(urls) == 1 {
		return urls[0]
	}
	if len(urls) > 1 {
		return urls
	}
	return nil
}

func isGitHubPrerelease(root any, idx int, warn func(string)) bool {
	pr, err := stepJSON(root, []any{int64(idx), "prerelease"}, "")
	if err != nil {
		warn(fmt.Sprintf("无法判断 release #%d 是否为预发布：%v", idx, err))
		return false
	}
	b, ok := pr.(bool)
	return ok && b
}

func mergeHeaders(headers map[string]string, accept string) map[string]string {
	merged := make(map[string]string, len(headers)+1)
	for k, v := range headers {
		merged[k] = v
	}
	merged["Accept"] = accept
	return merged
}
