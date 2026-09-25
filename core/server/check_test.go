package server

import (
	"context"
	"errors"
	"net/http/httptest"
	"strconv"
	"sync/atomic"
	"testing"

	"github.com/vanadiry/serein/core/checker"
)

func TestRunConcurrent(t *testing.T) {
	var ran, recorded, errored atomic.Int32
	results := runConcurrent(context.Background(), []int{1, 2, 3, 4, 5}, 2,
		func(ctx context.Context, it int) (checker.CheckResponse, error) {
			ran.Add(1)
			if it == 3 {
				return checker.CheckResponse{}, errors.New("boom")
			}
			return checker.CheckResponse{AppID: strconv.Itoa(it)}, nil
		},
		func(it int, err error) { errored.Add(1) },
		func(it int, resp checker.CheckResponse) { recorded.Add(1) },
	)
	if ran.Load() != 5 || recorded.Load() != 4 || errored.Load() != 1 {
		t.Fatalf("ran=%d recorded=%d errored=%d", ran.Load(), recorded.Load(), errored.Load())
	}
	if len(results) != 4 {
		t.Fatalf("results=%d, want 4", len(results))
	}
}

func TestRunConcurrentCancelledNotReported(t *testing.T) {
	var errored atomic.Int32
	results := runConcurrent(context.Background(), []int{1, 2}, 1,
		func(ctx context.Context, it int) (checker.CheckResponse, error) {
			return checker.CheckResponse{}, context.Canceled
		},
		func(it int, err error) { errored.Add(1) },
		func(it int, resp checker.CheckResponse) {},
	)
	if errored.Load() != 0 || len(results) != 0 {
		t.Fatalf("取消不应上报为错误: errored=%d results=%d", errored.Load(), len(results))
	}
}

func TestMergeResults(t *testing.T) {
	in := []checker.CheckResponse{
		{AppID: "a", Platforms: map[string]checker.CheckPlatform{"macos": {LatestVersion: "1"}}},
		{AppID: "b", Platforms: map[string]checker.CheckPlatform{"macos": {LatestVersion: "2"}}},
		{AppID: "a", Platforms: map[string]checker.CheckPlatform{"windows": {LatestVersion: "3"}}},
	}
	out := mergeResults(in)
	if len(out) != 2 {
		t.Fatalf("应合并为 2 条，得到 %d", len(out))
	}
	var a *checker.CheckResponse
	for i := range out {
		if out[i].AppID == "a" {
			a = &out[i]
		}
	}
	if a == nil || len(a.Platforms) != 2 {
		t.Fatalf("a 的平台未合并: %+v", a)
	}
	if out := mergeResults(nil); out == nil || len(out) != 0 {
		t.Fatalf("nil 输入应返回非 nil 空切片: %v", out)
	}
}

func TestProxyName(t *testing.T) {
	resp := &checker.CheckResponse{AppID: "pub.ext"}
	if got := proxyName(resp, "msvsix", checker.CheckPlatform{LatestVersion: "1.2.3"}); got != "pub.ext-1.2.3.vsix" {
		t.Fatalf("msvsix 命名 = %q", got)
	}
	if got := proxyName(resp, "openvsx", checker.CheckPlatform{}); got != "pub.ext.vsix" {
		t.Fatalf("openvsx 无版本命名 = %q", got)
	}
	if got := proxyName(resp, "windows", checker.CheckPlatform{DownloadName: "app-{version}.zip", LatestVersion: "9"}); got != "app-9.zip" {
		t.Fatalf("download_name 替换 = %q", got)
	}
}

func TestSignURLs(t *testing.T) {
	secret, _ := newProxySecret()
	s := &Server{proxySecret: secret}

	if got := s.signURLs("https://x/a", "a.bin"); got == nil {
		t.Fatal("string 应生成签名地址")
	}
	if got := s.signURLs("", "a.bin"); got != nil {
		t.Fatal("空串应返回 nil")
	}
	got := s.signURLs([]any{"https://x/a", 123, "https://x/b"}, "a.bin")
	arr, ok := got.([]string)
	if !ok || len(arr) != 2 {
		t.Fatalf("[]any 应过滤非字符串: %T %v", got, got)
	}
	if got := s.signURLs([]any{123}, "a.bin"); got != nil {
		t.Fatalf("无非字符串外元素应返回 nil: %v", got)
	}
}

func TestIsLoopback(t *testing.T) {
	cases := map[string]bool{
		"127.0.0.1:1234": true,
		"[::1]:1234":     true,
		"127.0.0.1":      true,
		"10.0.0.1:1234":  false,
		"example.com:80": false,
	}
	for addr, want := range cases {
		r := httptest.NewRequest("GET", "/", nil)
		r.RemoteAddr = addr
		if got := isLoopback(r); got != want {
			t.Errorf("isLoopback(%q) = %v, want %v", addr, got, want)
		}
	}
}
