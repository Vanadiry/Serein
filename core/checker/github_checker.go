// GitHub Release 解析器。一次 /releases 请求，同时返回最新版本和历史版本。
// 复用 JSON 步进引擎：[0, "tag_name"] 为最新，后续下标为历史。
package checker

import (
	"fmt"
	"net/http"
	"regexp"
	"strings"
)

const githubAPI = "https://api.github.com/repos"

// CheckGitHubAll 请求 /releases，返回最新版本和所有历史版本。
// 一次 HTTP 请求，两种结果：latest 来自 [0, ...]，versions 来自全部。
func CheckGitHubAll(cfg CheckConfig, client *http.Client) (PlatformResult, []PlatformResult, error) {
	url := fmt.Sprintf("%s/%s/%s/releases", githubAPI, cfg.Owner, cfg.Repo)
	body, err := doRequest(client, url, cfg.UA, mergeHeaders(cfg.Headers, "application/vnd.github+json"))
	if err != nil {
		return PlatformResult{}, nil, fmt.Errorf("github: %w", err)
	}

	root, err := parseJSON(body)
	if err != nil {
		return PlatformResult{}, nil, fmt.Errorf("github: %w", err)
	}

	arr, ok := root.([]any)
	if !ok {
		// 尝试解析错误消息
		if errMap, ok := root.(map[string]any); ok {
			if msg, ok := errMap["message"].(string); ok {
				return PlatformResult{}, nil, fmt.Errorf("github: %s", msg)
			}
		}
		return PlatformResult{}, nil, fmt.Errorf("github: unexpected response format")
	}

	var latest PlatformResult
	var versions []PlatformResult

	for i := range arr {
		tag, err := extractGitHubVersion(root, i)
		if err != nil {
			continue
		}
		urls := extractGitHubAssets(root, i, cfg.DPosition)

		pr := PlatformResult{
			LatestVersion: tag,
			URL:           urls,
		}
		versions = append(versions, pr)

		if i == 0 {
			latest = pr
		}
	}

	return latest, versions, nil
}

func extractGitHubVersion(root any, idx int) (string, error) {
	ver, err := stepJSON(root, []any{int64(idx), "tag_name"}, "")
	if err != nil {
		return "", err
	}
	return strings.TrimPrefix(fmt.Sprintf("%v", ver), "v"), nil
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
		return nil
	}

	assets, err := stepJSON(root, []any{int64(idx), "assets"}, "")
	if err != nil {
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

func mergeHeaders(headers map[string]string, accept string) map[string]string {
	merged := make(map[string]string, len(headers)+1)
	for k, v := range headers {
		merged[k] = v
	}
	merged["Accept"] = accept
	return merged
}
