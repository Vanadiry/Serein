// 前置请求链执行器。按编号升序执行，上一步提取 URL 自动传入下一步。
// 支持 UA/headers/baseurl 注入，最后一步输出作为 config 的请求 URL。
package checker

import (
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
)

// defaultUA 程序默认 User-Agent
const defaultUA = "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/131.0.0.0 Safari/537.36"

var (
	urlCache = make(map[string][]byte)
	cacheMu  sync.Mutex
)

// ClearURLCache 清空 URL 请求缓存。check-all 调用前清一次，批量内共享去重。
func ClearURLCache() {
	cacheMu.Lock()
	urlCache = make(map[string][]byte)
	cacheMu.Unlock()
}

// PreStep 前置请求的一个步骤（checker 内部类型）
type PreStep struct {
	URL      string
	Type     string
	UA       string
	Headers  map[string]string
	BaseURL  string
	Position any
}

// RunPreRequests 执行前置请求链，返回最终 URL（作为 config 的请求地址）。
func RunPreRequests(steps []PreStep, client *http.Client) (string, error) {
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

		respBody, err := doRequest(client, url, step.UA, step.Headers)
		if err != nil {
			return "", fmt.Errorf("pre_request %d: %w", i, err)
		}

		extracted, err := extractURL(respBody, step.Type, step.Position, step.BaseURL)
		if err != nil {
			return "", fmt.Errorf("pre_request %d: %w", i, err)
		}
		nextURL = extracted
	}
	return nextURL, nil
}

// extractURL 根据 type 从响应体中提取一个链接。
func extractURL(body []byte, typ string, pos any, baseURL string) (string, error) {
	switch typ {
	case "json":
		return extractFromJSON(body, pos, baseURL)
	case "xml":
		return extractFromXML(body, pos, baseURL)
	case "regex":
		return extractFromRegex(body, pos)
	case "html_selector":
		return extractFromHTMLSelector(body, pos, baseURL)
	default:
		return "", fmt.Errorf("unknown pre_request type: %s", typ)
	}
}

func doRequest(client *http.Client, url, ua string, headers map[string]string) ([]byte, error) {
	cacheMu.Lock()
	if body, ok := urlCache[url]; ok {
		cacheMu.Unlock()
		return body, nil
	}
	cacheMu.Unlock()

	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, err
	}
	uaVal := ua
	if uaVal == "" {
		uaVal = defaultUA
	}
	req.Header.Set("User-Agent", uaVal)
	for k, v := range headers {
		req.Header.Set(k, v)
	}

	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("HTTP %d: %s", resp.StatusCode, string(body))
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	cacheMu.Lock()
	urlCache[url] = body
	cacheMu.Unlock()

	return body, nil
}

// JSON

func extractFromJSON(body []byte, pos any, baseURL string) (string, error) {
	root, err := parseJSON(body)
	if err != nil {
		return "", err
	}
	path, ok := pos.([]any)
	if !ok {
		return "", fmt.Errorf("json position must be array, got %T", pos)
	}
	v, err := Step(root, path)
	if err != nil {
		return "", err
	}
	return applyBaseURL(fmt.Sprintf("%v", v), baseURL), nil
}

// XML

func extractFromXML(body []byte, pos any, baseURL string) (string, error) {
	root, err := parseXML(body)
	if err != nil {
		return "", err
	}
	path, ok := pos.([]any)
	if !ok {
		return "", fmt.Errorf("xml position must be array, got %T", pos)
	}
	v, err := Step(root, path)
	if err != nil {
		return "", err
	}
	return applyBaseURL(fmt.Sprintf("%v", v), baseURL), nil
}

// 正则

func extractFromRegex(body []byte, pos any) (string, error) {
	pattern, ok := pos.(string)
	if !ok {
		return "", fmt.Errorf("regex position must be string, got %T", pos)
	}
	return matchRegex(string(body), pattern)
}

// html_selector

func extractFromHTMLSelector(body []byte, pos any, baseURL string) (string, error) {
	sel, err := parseHTML(body)
	if err != nil {
		return "", err
	}
	posMap, ok := pos.(map[string]any)
	if !ok {
		return "", fmt.Errorf("html_selector position must be map, got %T", pos)
	}
	selector, _ := posMap["selector"].(string)
	attr, _ := posMap["attr"].(string)
	regexPat, _ := posMap["regex"].(string)

	el := sel.Find(selector).First()
	if el.Length() == 0 {
		return "", fmt.Errorf("selector %q not found", selector)
	}

	var val string
	if attr != "" {
		val, _ = el.Attr(attr)
	} else {
		val = strings.TrimSpace(el.Text())
	}

	if regexPat != "" {
		val, err = matchRegexString(val, regexPat)
		if err != nil {
			return "", err
		}
	}
	return applyBaseURL(val, baseURL), nil
}

// 辅助

func applyBaseURL(val, baseURL string) string {
	if baseURL == "" || !strings.HasPrefix(val, "/") {
		return val
	}
	if strings.HasPrefix(val, "http://") || strings.HasPrefix(val, "https://") {
		return val
	}
	return strings.TrimSuffix(baseURL, "/") + val
}
