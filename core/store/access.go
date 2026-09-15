package store

import "net/http"

// AccessHeader 是需要防跨站的状态变更端点要求的自定义头
const AccessHeader = "X-Serein-ACCESS"

// HasAccess 报告请求是否带有有效的访问头
func HasAccess(r *http.Request) bool {
	return r.Header.Get(AccessHeader) == "1"
}
