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

// newTransport 构造带 SSRF 守卫与代理的底层 transport
func newTransport() *http.Transport {
	tr := &http.Transport{
		TLSClientConfig: &tls.Config{InsecureSkipVerify: false},
		// 拨号前校验目标 IP，并用已校验 IP 连接（防 SSRF / DNS rebinding）
		DialContext: safeDialContext,
		// 上游迟迟不返回响应头时不要一直挂着（不影响响应体传输）
		ResponseHeaderTimeout: 30 * time.Second,
	}
	if proxyURL != nil {
		tr.Proxy = http.ProxyURL(proxyURL)
	}
	return tr
}

// newClient 按总超时构造客户端；其余守卫（拨号校验 / 代理 / auth / 重定向）一致
func newClient(timeout time.Duration) *http.Client {
	return &http.Client{
		Timeout:   timeout,
		Transport: &authTransport{base: newTransport(), targets: authTargets},
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			if len(via) >= 10 {
				return fmt.Errorf("stopped after 10 redirects")
			}
			return blockPrivate(req.URL.String())
		},
	}
}

// NewClient 创建 HTTP 客户端（含 SSRF 拨号守卫、代理、auth 注入），总超时 30s
func NewClient() *http.Client { return newClient(30 * time.Second) }

// StreamClient 与 NewClient 同一套守卫，但总超时为 0，供大文件流式转发使用
// 大文件会在 30s 被掐断；改为依赖请求 ctx 与逐次读写
func StreamClient() *http.Client { return newClient(0) }

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
