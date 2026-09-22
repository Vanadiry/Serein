// 前置请求链执行器。按编号升序执行，上一步提取 URL 自动传入下一步。
// 支持 UA/headers/baseurl 注入，最后一步输出作为 config 的请求 URL。
package checker

import (
	"context"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"
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

func doRequest(client *http.Client, rawURL, ua string, headers map[string]string) ([]byte, error) {
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

func doPostRequest(client *http.Client, rawURL, ua string, headers map[string]string, bodyJSON []byte) ([]byte, error) {
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

var privateCIDRs = []string{
	// IPv4
	"0.0.0.0/8",
	"10.0.0.0/8",
	"100.64.0.0/10",
	"127.0.0.0/8",
	"169.254.0.0/16",
	"172.16.0.0/12",
	"192.0.0.0/24",
	"192.0.2.0/24",
	"192.168.0.0/16",
	"198.18.0.0/15",
	"198.51.100.0/24",
	"203.0.113.0/24",
	"224.0.0.0/4",
	"240.0.0.0/4",
	// IPv6
	"::/128",
	"::1/128",
	"fc00::/7",
	"fe80::/10",
	"ff00::/8",
	"2001:db8::/32",
}

var privateNets []*net.IPNet

func init() {
	for _, c := range privateCIDRs {
		if _, n, err := net.ParseCIDR(c); err == nil {
			privateNets = append(privateNets, n)
		}
	}
}

func isBlockedIP(ip net.IP) bool {
	if ip == nil {
		return true
	}
	if v4 := ip.To4(); v4 != nil {
		ip = v4
	}
	for _, n := range privateNets {
		if n.Contains(ip) {
			return true
		}
	}
	return false
}

// resolveAndCheck 解析 host，并确保其所有 IP 都不是内网/回环地址
func resolveAndCheck(host string) ([]net.IP, error) {
	if ip := net.ParseIP(host); ip != nil {
		if isBlockedIP(ip) {
			return nil, fmt.Errorf("blocked: %s is a private address", host)
		}
		return []net.IP{ip}, nil
	}
	ips, err := net.DefaultResolver.LookupIP(context.Background(), "ip", host)
	if err != nil {
		return nil, err
	}
	if len(ips) == 0 {
		return nil, fmt.Errorf("no IP for %s", host)
	}
	for _, ip := range ips {
		if isBlockedIP(ip) {
			return nil, fmt.Errorf("blocked: %s resolves to private address %s", host, ip)
		}
	}
	return ips, nil
}

func blockPrivate(rawURL string) error {
	u, err := url.Parse(rawURL)
	if err != nil {
		return fmt.Errorf("invalid url: %w", err)
	}
	if u.Scheme != "http" && u.Scheme != "https" {
		return nil
	}
	_, err = resolveAndCheck(u.Hostname())
	return err
}

// safeDialContext 拨号前校验目标 IP，并直接用已校验的 IP 连接，消除 DNS rebinding 的 TOCTOU
func safeDialContext(ctx context.Context, network, addr string) (net.Conn, error) {
	d := net.Dialer{Timeout: 15 * time.Second}
	if proxyURL != nil {
		// 走代理时拨号目标是代理本身（常为 127.0.0.1），跳过私网校验；
		// 目标地址的 SSRF 校验由 doRequest 里的 blockPrivate 负责
		return d.DialContext(ctx, network, addr)
	}
	host, port, err := net.SplitHostPort(addr)
	if err != nil {
		return nil, err
	}
	ips, err := resolveAndCheck(host)
	if err != nil {
		return nil, err
	}
	var lastErr error
	for _, ip := range ips {
		conn, err := d.DialContext(ctx, network, net.JoinHostPort(ip.String(), port))
		if err == nil {
			return conn, nil
		}
		lastErr = err
	}
	if lastErr == nil {
		lastErr = fmt.Errorf("no address for %s", host)
	}
	return nil, lastErr
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
