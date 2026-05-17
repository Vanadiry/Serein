package checker

import (
	"bytes"
	"encoding/json"
	"encoding/xml"
	"fmt"
	"regexp"
	"strings"

	"github.com/PuerkitoBio/goquery"
)

// ── JSON 解析 ──

func parseJSON(body []byte) (any, error) {
	var root any
	d := json.NewDecoder(bytes.NewReader(body))
	d.UseNumber()
	if err := d.Decode(&root); err != nil {
		return nil, fmt.Errorf("json: %w", err)
	}
	// 将 json.Number 转换，方便后续处理
	root = convertNumbers(root)
	return root, nil
}

func convertNumbers(v any) any {
	switch val := v.(type) {
	case map[string]any:
		for k, vv := range val {
			val[k] = convertNumbers(vv)
		}
		return val
	case []any:
		for i, vv := range val {
			val[i] = convertNumbers(vv)
		}
		return val
	case json.Number:
		if i, err := val.Int64(); err == nil {
			return float64(i)
		}
		if f, err := val.Float64(); err == nil {
			return f
		}
		return val.String()
	default:
		return v
	}
}

// ── XML 解析 ──

func parseXML(body []byte) (any, error) {
	decoder := xml.NewDecoder(bytes.NewReader(body))
	root, err := decodeXMLElement(decoder, "")
	if err != nil {
		return nil, fmt.Errorf("xml: %w", err)
	}
	return root, nil
}

func decodeXMLElement(decoder *xml.Decoder, stopAt string) (any, error) {
	var children []any
	attrs := make(map[string]any)

	for {
		tok, err := decoder.Token()
		if err != nil {
			// EOF
			break
		}

		switch t := tok.(type) {
		case xml.StartElement:
			// 收集属性，以 "-" 前缀存储
			elAttrs := make(map[string]any)
			for _, a := range t.Attr {
				elAttrs["-"+a.Name.Local] = a.Value
			}

			child, err := decodeXMLElement(decoder, t.Name.Local)
			if err != nil {
				return nil, err
			}

			if childMap, ok := child.(map[string]any); ok {
				for k, v := range elAttrs {
					childMap[k] = v
				}
			}

			children = append(children, map[string]any{
				t.Name.Local: child,
			})

		case xml.EndElement:
			if t.Name.Local == stopAt {
				if len(children) == 0 && len(attrs) > 0 {
					return attrs, nil
				}
				return buildXMLResult(children, attrs), nil
			}

		case xml.CharData:
			text := strings.TrimSpace(string(t))
			if text != "" {
				children = append(children, text)
			}
		}
	}

	return buildXMLResult(children, attrs), nil
}

func buildXMLResult(children []any, attrs map[string]any) any {
	var nonText []any
	var texts []string
	for _, c := range children {
		if s, ok := c.(string); ok {
			texts = append(texts, s)
		} else {
			nonText = append(nonText, c)
		}
	}

	if len(nonText) == 0 {
		result := make(map[string]any)
		for k, v := range attrs {
			result[k] = v
		}
		if len(texts) == 1 {
			result["#text"] = texts[0]
		} else if len(texts) > 1 {
			var t []any
			for _, s := range texts {
				t = append(t, s)
			}
			result["#text"] = t
		}
		return result
	}

	// 合并同类元素为数组
	merged := make(map[string][]any)
	var order []string
	for _, c := range nonText {
		if m, ok := c.(map[string]any); ok {
			for k, v := range m {
				if _, exists := merged[k]; !exists {
					order = append(order, k)
				}
				merged[k] = append(merged[k], v)
			}
		}
	}

	result := make(map[string]any)
	for k, v := range attrs {
		result[k] = v
	}
	for _, k := range order {
		vals := merged[k]
		if len(vals) == 1 {
			result[k] = vals[0]
		} else {
			result[k] = vals
		}
	}
	if len(texts) > 0 {
		result["#text"] = strings.Join(texts, " ")
	}
	return result
}

// ── HTML 解析 ──

func parseHTML(body []byte) (*goquery.Document, error) {
	doc, err := goquery.NewDocumentFromReader(bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("html: %w", err)
	}
	return doc, nil
}

// ── 正则 ──

func matchRegex(body, pattern string) (string, error) {
	re, err := regexp.Compile(pattern)
	if err != nil {
		return "", fmt.Errorf("regex: %w", err)
	}
	matches := re.FindStringSubmatch(body)
	if len(matches) < 2 {
		return "", fmt.Errorf("regex %q no match", pattern)
	}
	return matches[1], nil
}

func matchRegexString(text, pattern string) (string, error) {
	return matchRegex(text, pattern)
}

// ── XPath ──

func evalXPath(doc *goquery.Document, expr string) (string, error) {
	// XPath 简化实现：支持少量常用语法
	// 完整 XPath 需引入第三方库，此处用 goquery 的 CSS + 简单解析兜底
	sel, attr, err := parseXPathExpr(expr)
	if err != nil {
		// 兜底：将 XPath 当 CSS 选择器用
		el := doc.Find(expr).First()
		if el.Length() == 0 {
			return "", fmt.Errorf("xpath: no match for %q", expr)
		}
		return strings.TrimSpace(el.Text()), nil
	}
	el := doc.Find(sel).First()
	if el.Length() == 0 {
		return "", fmt.Errorf("xpath: no match for %q", expr)
	}
	if attr != "" {
		val, _ := el.Attr(attr)
		return val, nil
	}
	return strings.TrimSpace(el.Text()), nil
}

// parseXPathExpr 将简单 XPath 转为 CSS 选择器 + 属性名。
// 仅支持: //tag[@attr='val']/@attr, //tag/text()
func parseXPathExpr(expr string) (cssSel, attr string, err error) {
	// 去掉 /text() 和 /@xxx 尾部
	if strings.HasSuffix(expr, "/text()") {
		return strings.TrimSuffix(expr, "/text()"), "", nil
	}
	parts := strings.Split(expr, "/@")
	if len(parts) == 2 {
		return parts[0], parts[1], nil
	}
	return expr, "", nil
}
