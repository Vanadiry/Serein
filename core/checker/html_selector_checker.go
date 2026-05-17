// HTML CSS 选择器解析器：CSS 选择器定位元素，可选 regex 提取文本，可选 attr 取属性。
package checker

import (
	"fmt"
	"strings"
)

func extractSelectorValue(body []byte, pos any, baseURL string) (any, error) {
	doc, err := parseHTML(body)
	if err != nil {
		return nil, err
	}
	posMap, ok := pos.(map[string]any)
	if !ok {
		return nil, fmt.Errorf("html_selector position must be map, got %T", pos)
	}

	selector, _ := posMap["selector"].(string)
	attr, _ := posMap["attr"].(string)
	regexPat, _ := posMap["regex"].(string)

	el := doc.Find(selector).First()
	if el.Length() == 0 {
		return nil, fmt.Errorf("selector %q not found", selector)
	}

	var val string
	if attr != "" {
		val, _ = el.Attr(attr)
	} else {
		val = strings.TrimSpace(el.Text())
	}

	if regexPat != "" {
		val, err = matchRegexString(val, regexPat)
		if err != nil {
			return nil, err
		}
	}

	return applyBaseURL(val, baseURL), nil
}
