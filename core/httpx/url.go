// URL 拼接辅助
package httpx

import (
	"net/url"
	"strings"
)

// JoinURL 将相对链接拼接到 base 上，兼容两端斜杠的有无
func JoinURL(base, ref string) string {
	if ref == "" {
		return base
	}
	// 协议相对//host/，补上 scheme
	if strings.HasPrefix(ref, "//") {
		if bu, err := url.Parse(base); err == nil && bu.Scheme != "" {
			return bu.Scheme + ":" + ref
		}
		return "https:" + ref
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
