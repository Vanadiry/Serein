// HTML XPath 解析器：XPath 表达式定位取值。
package checker

import "fmt"

func extractXPathValue(body []byte, pos any, baseURL string) (any, error) {
	doc, err := parseHTML(body)
	if err != nil {
		return nil, err
	}
	expr, ok := pos.(string)
	if !ok {
		return nil, fmt.Errorf("html_xpath position must be string, got %T", pos)
	}
	val, err := evalXPath(doc, expr)
	if err != nil {
		return nil, err
	}
	return applyBaseURL(val, baseURL), nil
}
