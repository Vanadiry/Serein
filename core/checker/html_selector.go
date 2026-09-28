// HTML CSS 选择器解析器：CSS 选择器定位元素，可选 regex 提取文本，可选 attr 取属性
package checker

import (
	"fmt"
	"strings"

	"github.com/vanadiry/serein/core/httpx"
)

func extractSelectorValue(body []byte, pos any, baseURL string) (any, error) {
	doc, err := parseHTML(body)
	if err != nil {
		return nil, err
	}
	posMap, ok := pos.(map[string]any)
	if !ok {
		return nil, fmt.Errorf("html_selector 的 position 值为 %T，不合法，应当为表", pos)
	}

	selector, _ := posMap["selector"].(string)
	attr, _ := posMap["attr"].(string)
	regexPat, _ := posMap["regex"].(string)

	el := doc.Find(selector).First()
	if el.Length() == 0 {
		return nil, fmt.Errorf("未找到选择器 %q", selector)
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

	return httpx.JoinURL(baseURL, val), nil
}
