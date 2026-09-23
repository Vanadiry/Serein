// 通用 HTTP 请求：GET/POST、UA/headers、响应大小限制、URL 缓存
package httpx

import (
	"fmt"
	"io"
	"net/http"
	"sort"
	"strings"
	"sync"
)

// defaultUA 程序默认 User-Agent
const defaultUA = "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/131.0.0.0 Safari/537.36"

const (
	maxRespBytes    = 8 << 20 // 响应体上限 8MB
	maxErrBodyBytes = 8 << 10 // 错误响应体上限 8KB
)

var (
	urlCache = make(map[string][]byte)
	cacheMu  sync.Mutex
)

// ClearURLCache 清空 URL 请求缓存。check-all 调用前清一次，批量内共享去重
func ClearURLCache() {
	cacheMu.Lock()
	urlCache = make(map[string][]byte)
	cacheMu.Unlock()
}

// CheckStatus 校验响应状态码，>= 400 时读取有限的错误响应体并返回错误
func CheckStatus(resp *http.Response) error {
	if resp.StatusCode < 400 {
		return nil
	}
	body, _ := io.ReadAll(io.LimitReader(resp.Body, maxErrBodyBytes))
	return fmt.Errorf("HTTP %d: %s", resp.StatusCode, string(body))
}

// Request 发起 GET 请求，带 SSRF 校验与响应大小限制，并缓存结果
func Request(client *http.Client, rawURL, ua string, headers map[string]string) ([]byte, error) {
	if err := blockPrivate(rawURL); err != nil {
		return nil, err
	}

	req, err := http.NewRequest("GET", rawURL, nil)
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

	// 缓存 key 纳入 UA 与 headers，避免同 URL 不同请求头命中错误缓存
	key := requestKey(req)
	cacheMu.Lock()
	if body, ok := urlCache[key]; ok {
		cacheMu.Unlock()
		return body, nil
	}
	cacheMu.Unlock()

	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if err := CheckStatus(resp); err != nil {
		return nil, err
	}

	body, err := io.ReadAll(io.LimitReader(resp.Body, maxRespBytes+1))
	if err != nil {
		return nil, err
	}
	if len(body) > maxRespBytes {
		return nil, fmt.Errorf("response exceeds limit of %d bytes", maxRespBytes)
	}

	cacheMu.Lock()
	urlCache[key] = body
	cacheMu.Unlock()

	return body, nil
}

// requestKey 生成请求缓存 key：method + URL + 规范化后的 headers（含 UA）
func requestKey(req *http.Request) string {
	keys := make([]string, 0, len(req.Header))
	for k := range req.Header {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	var b strings.Builder
	b.WriteString(req.Method)
	b.WriteByte(' ')
	b.WriteString(req.URL.String())
	for _, k := range keys {
		b.WriteByte('\x00')
		b.WriteString(k)
		b.WriteByte(':')
		b.WriteString(strings.Join(req.Header[k], ","))
	}
	return b.String()
}

// PostRequest 发起 POST 请求，带 SSRF 校验与响应大小限制
func PostRequest(client *http.Client, rawURL, ua string, headers map[string]string, bodyJSON []byte) ([]byte, error) {
	if err := blockPrivate(rawURL); err != nil {
		return nil, err
	}
	req, err := http.NewRequest("POST", rawURL, strings.NewReader(string(bodyJSON)))
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

	if err := CheckStatus(resp); err != nil {
		return nil, err
	}

	body, err := io.ReadAll(io.LimitReader(resp.Body, maxRespBytes+1))
	if err != nil {
		return nil, err
	}
	if len(body) > maxRespBytes {
		return nil, fmt.Errorf("response exceeds limit of %d bytes", maxRespBytes)
	}
	return body, nil
}
