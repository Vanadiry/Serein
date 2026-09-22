// Package httpx 提供检查与规则拉取共用的 HTTP 客户端、SSRF 防护、认证注入与请求缓存。=
package httpx

import (
	"crypto/tls"
	"fmt"
	"net/http"
	"net/url"
	"sync"
	"time"
)

// proxyURL 上游请求使用的代理；由 SetProxy 在启动时设置
var proxyURL *url.URL

// SetProxy 配置上游请求使用的代理（nil 表示不使用代理）
func SetProxy(u *url.URL) { proxyURL = u }

// NewClient 创建 HTTP 客户端（含 SSRF 拨号守卫、代理、auth 注入）
func NewClient() *http.Client {
	tr := &http.Transport{
		TLSClientConfig: &tls.Config{InsecureSkipVerify: false},
		// 拨号前校验目标 IP，并用已校验 IP 连接（防 SSRF / DNS rebinding）
		DialContext: safeDialContext,
	}
	if proxyURL != nil {
		tr.Proxy = http.ProxyURL(proxyURL)
	}
	return &http.Client{
		Timeout:   30 * time.Second,
		Transport: &authTransport{base: tr, targets: authTargets},
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			if len(via) >= 10 {
				return fmt.Errorf("stopped after 10 redirects")
			}
			return blockPrivate(req.URL.String())
		},
	}
}

var (
	defaultOnce   sync.Once
	defaultClient *http.Client
)

// DefaultClient 返回共享的 HTTP 客户端（首次使用时按当前配置创建）
// 供规则源 / 动态配置拉取等复用，避免每次请求新建客户端
func DefaultClient() *http.Client {
	defaultOnce.Do(func() { defaultClient = NewClient() })
	return defaultClient
}
