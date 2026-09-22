// Auth 注入：按注册的目标（host）给请求自动加上认证头
// 仅在实际匹配目标、且请求未自带 Authorization 时注入
package httpx

import "net/http"

// AuthTarget 一个需要注入认证的目标
type AuthTarget struct {
	Host   string // 精确匹配 hostname（如 "api.github.com"）
	Scheme string // 认证方案，空则 "Bearer"
	Token  string // 凭据
}

// authTargets 由 SetAuthTargets 在启动时设置一次，之后只读
var authTargets []AuthTarget

// SetAuthTargets 设置认证目标
func SetAuthTargets(targets []AuthTarget) { authTargets = targets }

// authTransport 在底层 RoundTripper 上按目标注入认证头
// 每跳（含重定向）都会经过，能补回重定向被剥离的 Authorization
type authTransport struct {
	base    http.RoundTripper
	targets []AuthTarget
}

func (t *authTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	for _, tg := range t.targets {
		if req.URL.Hostname() == tg.Host && !hasAuth(req.Header) {
			req = req.Clone(req.Context())
			injectAuth(req.Header, tg)
			break
		}
	}
	return t.base.RoundTrip(req)
}

func hasAuth(h http.Header) bool { return h.Get("Authorization") != "" }

func injectAuth(h http.Header, t AuthTarget) {
	scheme := t.Scheme
	if scheme == "" {
		scheme = "Bearer"
	}
	h.Set("Authorization", scheme+" "+t.Token)
}
