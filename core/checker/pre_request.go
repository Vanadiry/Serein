// 前置请求链执行器。按编号升序执行，上一步提取 URL 自动传入下一步。
// 支持 UA/headers/baseurl 注入，最后一步输出作为 config 的请求 URL。
package checker

import (
	"context"
	"fmt"
	"net/http"

	"github.com/vanadiry/serein/core/httpx"
	"github.com/vanadiry/serein/core/store"
)

// RunPreRequests 执行前置请求链，返回最终 URL（作为 config 的请求地址）
func RunPreRequests(ctx context.Context, steps []store.PreRequestStep, client *http.Client) (string, error) {
	if len(steps) == 0 {
		return "", nil
	}

	var nextURL string
	for i, step := range steps {
		url := step.URL
		if url == "" {
			url = nextURL
		}
		if url == "" {
			return "", fmt.Errorf("pre_request %d: no url", i)
		}
		switch step.Type {
		case "json", "xml", "regex", "html_selector":
		default:
			return "", fmt.Errorf("pre_request %d: unknown type %q", i, step.Type)
		}

		respBody, err := httpx.Request(ctx, client, url, step.UA, step.Headers)
		if err != nil {
			return "", fmt.Errorf("pre_request %d: %w", i, err)
		}

		// 复用通用提取（与版本/下载解析器同一套逻辑）
		v, err := extractValue(respBody, step.Type, step.Position, "", step.BaseURL, "")
		if err != nil {
			return "", fmt.Errorf("pre_request %d: %w", i, err)
		}
		nextURL = httpx.JoinURL(step.BaseURL, toString(v))
	}
	return nextURL, nil
}
