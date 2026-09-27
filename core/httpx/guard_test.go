package httpx

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync"
	"testing"
)

// SSRF 校验必须在 RoundTrip 层：任何经由 httpx 客户端的请求都绕不过，
// 包括 core/store 的 getHTTP（它的 URL 来自远程规则源的 baseurl）。
func TestGuardBlocksPrivateTargets(t *testing.T) {
	// 私网目标由 httptest 监听 127.0.0.1 提供，用来证明「确实被守卫拦下」
	// 而不是「本来就连不上」
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("SECRET"))
	}))
	defer srv.Close()
	if !strings.Contains(srv.URL, "127.0.0.1") {
		t.Fatalf("测试前提不成立：%s", srv.URL)
	}

	req, err := http.NewRequest(http.MethodGet, srv.URL, nil)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := NewClient().Do(req); err == nil {
		t.Fatal("私网目标应被守卫拦下")
	} else if !strings.Contains(err.Error(), "private address") {
		t.Errorf("错误信息应说明是私网地址: %v", err)
	}
}

// 配了代理时也必须校验目标。
//
// 这是 #10 的核心：safeDialContext 在配置了代理时会主动放弃拨号层校验
// （那一层看到的只是代理地址），校验责任落到上层——而 core/store 的 getHTTP
// 恰好没做，于是恶意规则源的 baseurl 可以经代理把内网响应取回来。
//
// 断言用「代理是否被要求去取内网地址」，而不是只看有没有报错：配了代理后
// 真实代理可能刚好不可达而报错，那与 SSRF 无关，会让测试失去意义。
func TestGuardBlocksPrivateTargetsWithProxy(t *testing.T) {
	var mu sync.Mutex
	var asked []string
	proxySrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		asked = append(asked, r.Host+r.URL.Path)
		mu.Unlock()
		_, _ = w.Write([]byte("INTERNAL-METADATA"))
	}))
	defer proxySrv.Close()

	var targetHit int
	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		targetHit++
		_, _ = w.Write([]byte("INTERNAL-METADATA"))
	}))
	defer target.Close()

	pu, err := url.Parse(proxySrv.URL)
	if err != nil {
		t.Fatal(err)
	}
	old := proxyURL
	proxyURL = pu
	t.Cleanup(func() { proxyURL = old })

	req, err := http.NewRequest(http.MethodGet, target.URL+"/latest/meta-data", nil)
	if err != nil {
		t.Fatal(err)
	}
	for name, c := range map[string]*http.Client{
		"NewClient":     NewClient(),
		"StreamClient":  StreamClient(),
		"DefaultClient": DefaultClient(),
	} {
		if _, err := c.Do(req); err == nil {
			t.Errorf("%s: 私网目标应被拦下", name)
		}
	}
	mu.Lock()
	defer mu.Unlock()
	for _, a := range asked {
		t.Errorf("代理被要求取内网地址，SSRF 成立: %s", a)
	}
	if targetHit != 0 {
		t.Errorf("内网目标被直接命中 %d 次", targetHit)
	}
}

// 守卫在注入认证头之前生效：被拦下的请求不该带上凭据
func TestGuardRunsBeforeAuthInjection(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Error("请求不该到达服务端")
	}))
	defer srv.Close()

	oldT := authTargets
	authTargets = []AuthTarget{{Host: "127.0.0.1", Token: "secret-token"}}
	t.Cleanup(func() { authTargets = oldT })

	req, _ := http.NewRequest(http.MethodGet, srv.URL, nil)
	_, err := NewClient().Do(req)
	if err == nil {
		t.Fatal("应被拦下")
	}
	if strings.Contains(err.Error(), "secret-token") {
		t.Error("错误信息里不应出现凭据")
	}
}

// 放行正常目标：守卫不能误伤
func TestGuardAllowsPublicTargets(t *testing.T) {
	// 用一个「解析到公网地址」的方式避免测试依赖外网：
	// 直接验证 BlockPrivate 的判定，而不是真的发请求
	for _, u := range []string{
		"https://raw.githubusercontent.com/a/b",
		"https://api.github.com/repos/o/r/releases",
		"http://example.com/x",
	} {
		if err := BlockPrivate(u); err != nil {
			t.Errorf("%s 不应被拦: %v", u, err)
		}
	}
	for _, u := range []string{
		"http://127.0.0.1/x",
		"http://169.254.169.254/latest/meta-data",
		"http://10.0.0.5/",
		"http://[::1]/",
		"http://192.168.1.1/admin",
	} {
		if err := BlockPrivate(u); err == nil {
			t.Errorf("%s 应被拦", u)
		}
	}
}
