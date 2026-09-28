// 正则解析器：对 HTTP 响应全文跑正则，捕获组取值
package checker

import "fmt"

func extractRegexValue(body []byte, pos any) (any, error) {
	pattern, ok := pos.(string)
	if !ok {
		return nil, fmt.Errorf("regex 的 position 值为 %T，不合法，应当为字符串", pos)
	}
	return matchRegex(string(body), pattern)
}
