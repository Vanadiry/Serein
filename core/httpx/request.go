// 通用 HTTP 请求：GET/POST、UA/headers、响应大小限制、URL 缓存
package httpx

import (
	"fmt"
	"io"
	"net/http"
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

// Request 发起 GET 请求，带 SSRF 校验与响应大小限制，并缓存结果
func Request(client *http.Client, rawURL, ua string, headers map[string]string) ([]byte, error) {
	if err := blockPrivate(rawURL); err != nil {
		return nil, err
	}
	cacheMu.Lock()
	if body, ok := urlCache[rawURL]; ok {
		cacheMu.Unlock()
		return body, nil
	}
	cacheMu.Unlock()

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

	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, maxErrBodyBytes))
		return nil, fmt.Errorf("HTTP %d: %s", resp.StatusCode, string(body))
	}

	body, err := io.ReadAll(io.LimitReader(resp.Body, maxRespBytes+1))
	if err != nil {
		return nil, err
	}
	if len(body) > maxRespBytes {
		return nil, fmt.Errorf("response exceeds limit of %d bytes", maxRespBytes)
	}

	cacheMu.Lock()
	urlCache[rawURL] = body
	cacheMu.Unlock()

	return body, nil
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

	if resp.StatusCode >= 400 {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, maxErrBodyBytes))
		return nil, fmt.Errorf("HTTP %d: %s", resp.StatusCode, string(body))
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
