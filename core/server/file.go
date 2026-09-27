package server

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"mime"
	"net/http"
	"net/url"
	"path"
	"strconv"
	"strings"
	"time"

	"github.com/vanadiry/serein/core/events"
	"github.com/vanadiry/serein/core/httpx"
	"github.com/vanadiry/serein/core/log"
)

// proxyExpiry 下载代理签名有效期
const proxyExpiry = 24 * time.Hour

// maxProxyName 落盘文件名长度上限（字节）
const maxProxyName = 200

// newProxySecret 生成进程级随机密钥；重启即全部失效
func newProxySecret() ([]byte, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return nil, err
	}
	return b, nil
}

// signProxy 对 (url, name, exp) 计算 HMAC-SHA256
func signProxy(secret []byte, rawURL, name string, exp int64) string {
	mac := hmac.New(sha256.New, secret)
	fmt.Fprintf(mac, "%s\n%s\n%d", rawURL, name, exp)
	return hex.EncodeToString(mac.Sum(nil))
}

// proxyURL 生成带签名的下载代理地址（供检查结果透传）
func (s *Server) proxyURL(rawURL, name string) string {
	name = sanitizeFilename(name)
	exp := time.Now().Add(proxyExpiry).Unix()
	q := url.Values{}
	q.Set("url", rawURL)
	if name != "" {
		q.Set("name", name)
	}
	q.Set("exp", strconv.FormatInt(exp, 10))
	q.Set("sig", signProxy(s.proxySecret, rawURL, name, exp))
	return "/api/file?" + q.Encode()
}

// verifyProxy 校验签名与有效期
func (s *Server) verifyProxy(rawURL, name, expStr, sig string) bool {
	exp, err := strconv.ParseInt(expStr, 10, 64)
	if err != nil || exp < time.Now().Unix() {
		return false
	}
	want := signProxy(s.proxySecret, rawURL, name, exp)
	return hmac.Equal([]byte(want), []byte(sig))
}

// handleFile 流式下载代理：校验签名后转发上游，由响应头统一命名
func (s *Server) handleFile(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	rawURL := q.Get("url")
	name := q.Get("name")
	if rawURL == "" {
		writeError(w, http.StatusBadRequest, "missing url")
		return
	}
	if !s.verifyProxy(rawURL, name, q.Get("exp"), q.Get("sig")) {
		writeError(w, http.StatusForbidden, "invalid or expired signature")
		return
	}
	rawURL, err := parseHTTPURL(rawURL)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	if err := httpx.BlockPrivate(rawURL); err != nil {
		writeError(w, http.StatusForbidden, "url not allowed")
		return
	}

	req, err := http.NewRequestWithContext(r.Context(), http.MethodGet, rawURL, nil)
	if err != nil {
		writeError(w, http.StatusBadRequest, "bad request")
		return
	}
	// 显式 identity：避免 Go 自动解压破坏 Content-Length / Content-Range
	req.Header.Set("Accept-Encoding", "identity")
	if rng := r.Header.Get("Range"); rng != "" {
		req.Header.Set("Range", rng)
	}
	if ir := r.Header.Get("If-Range"); ir != "" {
		req.Header.Set("If-Range", ir)
	}

	resp, err := httpx.StreamClient().Do(req)
	if err != nil {
		if r.Context().Err() != nil {
			return // 客户端已断开
		}
		log.LogfWarn("[file] fetch %s: %v", rawURL, err)
		writeError(w, http.StatusBadGateway, "failed to fetch upstream")
		return
	}
	defer resp.Body.Close()

	if !writeUpstream(w, resp, name, rawURL) {
		return
	}
	if _, err := io.Copy(w, resp.Body); err != nil && r.Context().Err() == nil {
		log.LogfWarn("[file] copy %s: %v", rawURL, err)
	}
}

// writeUpstream 写上游响应的状态与响应头，成功返回 true 由调用方继续转发响应体。
// 返回 false 表示已自行应答，调用方不要再写任何东西。
func writeUpstream(w http.ResponseWriter, resp *http.Response, name, rawURL string) bool {
	// 上游 4xx/5xx 的响应体是错误页面（HTML 错误页、JSON 报错），不是 VSIX。
	// 原样透传的话浏览器会照着 Content-Disposition 把它存下来，用户得到一个
	// 几百字节的损坏 .vsix，装的时候才报错，还以为是扩展本身有问题。
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		// 读掉一点再丢弃，让连接能进复用池，不必每次都重新握手
		_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 4096))
		log.LogfWarn("[file] 上游返回 %d: %s", resp.StatusCode, rawURL)
		writeError(w, proxyErrorStatus(resp.StatusCode),
			fmt.Sprintf("上游返回 %d，文件没有下载到", resp.StatusCode))
		// 拿这个 URL 的要么是浏览器标签页、要么是下载器（浏览器扩展拦截），
		// 两者都不经过前端的 api()，所以 Serein 窗口只能靠事件总线知道出事了。
		// 少了这条，用户点了下载之后界面毫无反应，得自己切到浏览器看报错。
		label := sanitizeFilename(name)
		if label == "" {
			label = rawURL
		}
		events.Emit("error", "[下载]", fmt.Sprintf("%s 下载失败：上游返回 %d", label, resp.StatusCode))
		return false
	}

	// 响应头白名单（不整包透传，避免 Set-Cookie / CSP / hop-by-hop 等）
	for _, h := range []string{"Content-Type", "Content-Length", "Content-Range", "Accept-Ranges", "Last-Modified", "ETag"} {
		if v := resp.Header.Get(h); v != "" {
			w.Header().Set(h, v)
		}
	}
	if w.Header().Get("Content-Type") == "" {
		w.Header().Set("Content-Type", "application/octet-stream")
	}
	if name = sanitizeFilename(name); name == "" {
		name = filenameFromResponse(resp, rawURL)
	}
	w.Header().Set("Content-Disposition", mime.FormatMediaType("attachment", map[string]string{"filename": name}))
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.WriteHeader(resp.StatusCode)
	return true
}

// proxyErrorStatus 上游失败时该回给客户端的状态码。
// 4xx 是上游明确拒绝（版本已下架、403 等），原样透传才看得出原因；
// 5xx 是上游自己坏了，那是我们这道网关没办成，回 502。
func proxyErrorStatus(upstream int) int {
	if upstream >= 400 && upstream < 500 {
		return upstream
	}
	return http.StatusBadGateway
}

// sanitizeFilename 清洗落盘文件名：去控制字符 / 路径分隔 / Windows 非法字符，限长
func sanitizeFilename(name string) string {
	name = strings.Map(func(r rune) rune {
		switch {
		case r < 0x20 || r == 0x7f:
			return -1
		case strings.ContainsRune(`"\/:*?<>|`, r):
			return '_'
		default:
			return r
		}
	}, name)
	name = strings.TrimSpace(name)
	name = strings.TrimRight(name, ".")
	if name == "" || name == "." || name == ".." || strings.Trim(name, "_") == "" {
		return ""
	}
	if len(name) > maxProxyName {
		ext := path.Ext(name)
		if len(ext) > 0 && len(ext) < 20 {
			base := strings.TrimSuffix(name, ext)
			if len(base) > maxProxyName-len(ext) {
				base = base[:maxProxyName-len(ext)]
			}
			name = base + ext
		} else {
			name = name[:maxProxyName]
		}
	}
	return name
}

// filenameFromResponse 未显式指定 name 时，回退上游 Content-Disposition 或 URL 末段
func filenameFromResponse(resp *http.Response, rawURL string) string {
	if cd := resp.Header.Get("Content-Disposition"); cd != "" {
		if _, params, err := mime.ParseMediaType(cd); err == nil {
			if fn := sanitizeFilename(params["filename"]); fn != "" {
				return fn
			}
		}
	}
	if u, err := url.Parse(rawURL); err == nil {
		if base := sanitizeFilename(path.Base(u.Path)); base != "" && base != "." {
			return base
		}
	}
	return "download"
}
