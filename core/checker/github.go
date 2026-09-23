// GitHub Release 解析器。一次 /releases 请求，返回最新（非预发布）版本。
// 复用 JSON 步进引擎：[0, "tag_name"] 为最新。
package checker

import (
	"context"
	"fmt"
	"net/http"
	"regexp"

	"github.com/vanadiry/serein/core/events"
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
	Owner     string
	Repo      string
	PerPage   int
	UA        string
	Headers   map[string]string
	DPosition any
}

// CheckGitHub 请求 /releases，返回最新非预发布版本及其下载链接。
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

	var latest PlatformResult
	var latestFound bool

	for i := range arr {
		tag, err := extractGitHubVersion(root, i)
		if err != nil {
			continue
		}
		if isGitHubPrerelease(root, i) {
			continue
		}
		latest = PlatformResult{
			LatestVersion: tag,
			URL:           extractGitHubAssets(root, i, cfg.DPosition),
		}
		latestFound = true
		break
	}

	if !latestFound && len(arr) > 0 {
		events.Emit("warn", "[github]", fmt.Sprintf("未在前 %d 个 release 中找到非预发布版本，可增大 per_page", perPage))
	}

	return latest, nil
}

func extractGitHubVersion(root any, idx int) (string, error) {
	ver, err := stepJSON(root, []any{int64(idx), "tag_name"}, "")
	if err != nil {
		return "", err
	}
	return stripVersionAffixes(fmt.Sprintf("%v", ver)), nil
}

func extractGitHubAssets(root any, idx int, dPosition any) any {
	if dPosition == nil {
		return nil
	}
	assetReStr, ok := dPosition.(string)
	if !ok {
		return nil
	}
	assetRe, err := regexp.Compile(assetReStr)
	if err != nil {
		events.Emit("error", "[github]", fmt.Sprintf("规则正则表达式编译失败: %v", err))
		return nil
	}

	assets, err := stepJSON(root, []any{int64(idx), "assets"}, "")
	if err != nil {
		events.Emit("error", "[github]", fmt.Sprintf("GitHub assets JSON 解析失败: %v", err))
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

func isGitHubPrerelease(root any, idx int) bool {
	pr, err := stepJSON(root, []any{int64(idx), "prerelease"}, "")
	if err != nil {
		events.Emit("warn", "[github]", fmt.Sprintf("无法判断 release #%d 是否为预发布: %v", idx, err))
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
