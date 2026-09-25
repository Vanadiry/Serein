package store

import "testing"

func TestFirstRunEnabled(t *testing.T) {
	if !(SereinConfig{}).FirstRunEnabled() {
		t.Fatal("未配置应默认开启")
	}
	f := false
	if (SereinConfig{FirstRun: &f}).FirstRunEnabled() {
		t.Fatal("显式 false 应关闭")
	}
	tr := true
	if !(SereinConfig{FirstRun: &tr}).FirstRunEnabled() {
		t.Fatal("显式 true 应开启")
	}
}

func TestProxyURL(t *testing.T) {
	if u := (Config{}).ProxyURL(); u != nil {
		t.Fatalf("未配置应返回 nil，得到 %v", u)
	}
	if u := (Config{Proxy: ProxyConfig{Host: "h", Port: 0}}).ProxyURL(); u != nil {
		t.Fatalf("端口非法应返回 nil，得到 %v", u)
	}
	if u := (Config{Proxy: ProxyConfig{Host: "h", Port: 70000}}).ProxyURL(); u != nil {
		t.Fatalf("端口越界应返回 nil，得到 %v", u)
	}
	u := (Config{Proxy: ProxyConfig{Host: "127.0.0.1", Port: 7890}}).ProxyURL()
	if u == nil || u.String() != "http://127.0.0.1:7890" {
		t.Fatalf("proxy = %v", u)
	}
}

func TestConfigValidate(t *testing.T) {
	valid := Config{
		Serein:   SereinConfig{Host: "127.0.0.1", Port: 8080},
		Download: DownloadConfig{Concurrency: 4},
	}
	if errs := valid.Validate(); len(errs) != 0 {
		t.Fatalf("合法配置不应报错: %v", errs)
	}

	bad := Config{
		Download: DownloadConfig{Concurrency: 0},
		Proxy:    ProxyConfig{Port: 1080}, // 有 port 无 host
	}
	errs := bad.Validate()
	// serein.host 空 + serein.port 非法 + concurrency<1 + proxy.port 无 host
	if len(errs) != 4 {
		t.Fatalf("应报 4 个错误，得到 %d: %v", len(errs), errs)
	}
}
