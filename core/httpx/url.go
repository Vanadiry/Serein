// URL 拼接辅助
package httpx

import (
	"net/url"
	"strings"
)

// URLScheme 返回 raw 的协议名（小写、不含冒号）；没有协议时返回 ""。
// 按 scheme 语法逐字符判断，而不是「第一个冒号之前」——否则 "https://x" 的
// scheme 冒号会被误当成路径里的冒号。判据与浏览器一致：
// new URL(raw, base).protocol 对同一字符串必须给出同一个协议名。
func URLScheme(raw string) string {
	i := strings.IndexByte(raw, ':')
	if i <= 0 {
		return ""
	}
	for j := 0; j < i; j++ {
		c := raw[j]
		switch {
		case c >= 'a' && c <= 'z', c >= 'A' && c <= 'Z':
		case j > 0 && (c >= '0' && c <= '9' || c == '+' || c == '-' || c == '.'):
		default:
			return "" // 冒号不在 scheme 位置（路径 / 查询串里）
		}
	}
	return strings.ToLower(raw[:i])
}

// ResolveScheme 保证输出可交给 http.Client / 外部下载器：没有协议时补 https。
//
// 与 URLScheme 配套——那是「判断」，这是「补齐」，两者分开是为了让调用方
// 显式决定何时补：把 URL 交给浏览器时不需要补（浏览器自己认 //host），
// 交给 Go 的 http.Client 时必须补（否则报 unsupported protocol scheme）。
// 已带协议的地址原样返回，因此 javascript: 之类仍会带着协议交给上层拒绝。
func ResolveScheme(raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" || URLScheme(raw) != "" {
		return raw
	}
	return "https://" + strings.TrimLeft(raw, "//")
}

// JoinURL 将相对链接拼接到 base 上，兼容两端斜杠的有无
func JoinURL(base, ref string) string {
	if ref == "" {
		return base
	}
	// 协议相对 //host/：继承 base 的协议（同源），base 也没协议时兜底 https
	if strings.HasPrefix(ref, "//") {
		if s := URLScheme(base); s != "" {
			return s + ":" + ref
		}
		return ResolveScheme(ref)
	}
	if isAbsoluteURL(ref) {
		return ref
	}
	if base == "" {
		return ref
	}
	return strings.TrimRight(base, "/") + "/" + strings.TrimLeft(ref, "/")
}

// isAbsoluteURL 判断是否为绝对地址（含 scheme）
func isAbsoluteURL(s string) bool {
	u, err := url.Parse(s)
	return err == nil && u.IsAbs()
}
