package server

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/vanadiry/serein/core/events"
)

func upstreamResp(status int, body string) *http.Response {
	return &http.Response{
		StatusCode: status,
		Header:     http.Header{"Content-Type": []string{"text/html"}},
		Body:       io.NopCloser(strings.NewReader(body)),
	}
}

// 上游 4xx/5xx 的错误页面绝不能被当附件存下来：那会得到一个损坏的 .vsix
// 用户装的时候才报错，还以为是扩展本身有问题
func TestUpstreamErrorNotSavedAsAttachment(t *testing.T) {
	for _, status := range []int{400, 401, 403, 404, 410, 429, 500, 502, 503} {
		page := "<html><body><h1>" + strconv.Itoa(status) + " Not Found</h1></body></html>"
		rr := httptest.NewRecorder()

		ok := writeUpstream(rr, upstreamResp(status, page), "pub.ext-1.2.3.vsix", "https://example.com/a.vsix")
		if ok {
			t.Errorf("%d: 不应继续转发响应体", status)
		}
		if cd := rr.Header().Get("Content-Disposition"); cd != "" {
			t.Errorf("%d: 不应设置附件名，实际 %q", status, cd)
		}
		if strings.Contains(rr.Body.String(), "Not Found") {
			t.Errorf("%d: 不应把上游错误页面回给浏览器: %s", status, rr.Body.String())
		}
		// 状态码要能说明原因：上游 4xx 透传，5xx 归为网关错误
		want := status
		if status >= 500 {
			want = http.StatusBadGateway
		}
		if rr.Code != want {
			t.Errorf("%d: 状态码应为 %d，实际 %d", status, want, rr.Code)
		}
	}
}

// 正常文件照旧走附件
func TestUpstreamOKStillAttachment(t *testing.T) {
	for _, status := range []int{200, 206} {
		rr := httptest.NewRecorder()
		ok := writeUpstream(rr, upstreamResp(status, "PK\x03\x04fake"), "pub.ext-1.2.3.vsix", "https://example.com/a.vsix")
		if !ok {
			t.Errorf("%d: 应继续转发响应体", status)
		}
		if cd := rr.Header().Get("Content-Disposition"); !strings.Contains(cd, "attachment") ||
			!strings.Contains(cd, "pub.ext-1.2.3.vsix") {
			t.Errorf("%d: 应作为附件下载，实际 %q", status, cd)
		}
		if rr.Code != status {
			t.Errorf("%d: 状态码应透传，实际 %d", status, rr.Code)
		}
	}
}

// 错误路径不能把上游的 Content-Type / Content-Length 带出去
// 否则浏览器会按上游的类型解释这段错误提示
func TestUpstreamErrorNoUpstreamHeaders(t *testing.T) {
	resp := upstreamResp(404, "<html>404</html>")
	resp.Header.Set("Content-Length", "15")
	rr := httptest.NewRecorder()

	writeUpstream(rr, resp, "a.vsix", "https://example.com/a.vsix")

	if v := rr.Header().Get("Content-Length"); v == "15" {
		t.Error("不应透传上游的 Content-Length")
	}
	if strings.Contains(rr.Header().Get("Content-Type"), "text/html") {
		t.Errorf("不应透传上游的 Content-Type: %q", rr.Header().Get("Content-Type"))
	}
}

func TestProxyErrorStatus(t *testing.T) {
	cases := map[int]int{
		400: 400, 403: 403, 404: 404, 429: 429,
		500: http.StatusBadGateway, 502: http.StatusBadGateway, 503: http.StatusBadGateway,
	}
	for up, want := range cases {
		if got := proxyErrorStatus(up); got != want {
			t.Errorf("上游 %d → %d，期望 %d", up, got, want)
		}
	}
}

// 拿 /api/file 的要么是浏览器标签页、要么是下载器，都不经过前端 api()
// 事件总线因此是 Serein 窗口唯一能知道“下载失败了”的途径
func TestUpstreamErrorEmitsEvent(t *testing.T) {
	ch := events.Subscribe()
	defer events.Unsubscribe(ch)
	drainSubscribed(ch) // 订阅会回放历史

	rr := httptest.NewRecorder()
	writeUpstream(rr, upstreamResp(404, "<html>404</html>"), "pub.ext-1.2.3.vsix", "https://example.com/a.vsix")

	deadline := time.After(2 * time.Second)
	for {
		select {
		case data := <-ch:
			var evt struct {
				Level   string `json:"level"`
				Context string `json:"context"`
				Message string `json:"message"`
			}
			if err := json.Unmarshal(data, &evt); err != nil {
				t.Fatal(err)
			}
			if evt.Context != "[下载]" {
				continue
			}
			if evt.Level != "error" {
				t.Errorf("级别应为 error: %+v", evt)
			}
			// 提示里要能看出是哪个文件、上游报了什么
			if !strings.Contains(evt.Message, "pub.ext-1.2.3.vsix") {
				t.Errorf("提示应含文件名: %s", evt.Message)
			}
			if !strings.Contains(evt.Message, "404") {
				t.Errorf("提示应含上游状态码: %s", evt.Message)
			}
			return
		case <-deadline:
			t.Fatal("上游出错时应推送事件，否则 App 窗口毫无反应")
		case <-time.After(20 * time.Millisecond):
		}
	}
}

// 下载成功时不该刷事件
func TestUpstreamOKNoEvent(t *testing.T) {
	ch := events.Subscribe()
	defer events.Unsubscribe(ch)
	drainSubscribed(ch)

	rr := httptest.NewRecorder()
	writeUpstream(rr, upstreamResp(200, "PK\x03\x04"), "a.vsix", "https://example.com/a.vsix")

	select {
	case data := <-ch:
		t.Errorf("成功时不该有事件: %s", data)
	case <-time.After(300 * time.Millisecond):
	}
}
