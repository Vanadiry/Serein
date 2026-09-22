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
	if base == "" {
		return ref
	}
	if isAbsoluteURL(ref) {
		return ref
	}
	return strings.TrimRight(base, "/") + "/" + strings.TrimLeft(ref, "/")
}

// isAbsoluteURL 判断是否为绝对地址（含 scheme，或协议相对 //host）
func isAbsoluteURL(s string) bool {
	if strings.HasPrefix(s, "//") {
		return true
	}
	u, err := url.Parse(s)
	return err == nil && u.IsAbs()
}
