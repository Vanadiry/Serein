// 响应体解析器：JSON/XML 转 tree、HTML 转 goquery、正则捕获
package checker

import (
	"bytes"
	"encoding/json"
	"encoding/xml"
	"fmt"
	"io"
	"regexp"
	"strings"

	"github.com/PuerkitoBio/goquery"
)

// JSON 解析

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

// XML 解析
// encoding/json 内建 10000 层嵌套限制，encoding/xml 没有，而 decodeXMLElement 是逐元素递归的
// 实测 2026-09-27 本机，深度 100 万、7MB 输入直接 fatal error: stack overflow
// 该错误不可 recover，整进程死。规则的 url 来自远程规则源，可远程触发，因此深度必须有上限
// 规则的 url 来自远程规则源，可远程触发，因此深度必须有上限
// 节点数同理：每个元素都会分配一个 map[string]any，实测 62 倍堆放大
// （8MB 输入放大成 478MB 堆）。两个上限声明为 var 以便测试覆盖
var (
	// xmlMaxDepth 嵌套深度上限
	// 真实 feed 都在个位数层（RSS/Atom 3~5、VSIX manifest 3~5），OPML 嵌套 outline 可到几十层
	// 256 有 50~80 倍余量，距实测崩溃点约 4000 倍
	xmlMaxDepth = 256
	// xmlMaxNodes 单个文档的元素总数上限，实测每节点 478 字节
	// 该上限把单份文档的堆占用钉在 100MB 上下，不设限时同一输入是 478MB
	xmlMaxNodes = 200000
)

// xmlBudget 单个 XML 文档的配额（一次 parseXML 一份）
type xmlBudget struct {
	nodes int
}

func parseXML(body []byte) (any, error) {
	decoder := xml.NewDecoder(bytes.NewReader(body))
	root, err := decodeXMLElement(decoder, "", 0, &xmlBudget{})
	if err != nil {
		return nil, fmt.Errorf("xml: %w", err)
	}
	return root, nil
}

// decodeXMLElement 解析一个元素及其子树。depth 为当前嵌套深度（根为 0）
// b 为整篇文档共享的配额。超限返回错误而非继续递归
func decodeXMLElement(decoder *xml.Decoder, stopAt string, depth int, b *xmlBudget) (any, error) {
	if depth > xmlMaxDepth {
		return nil, fmt.Errorf("嵌套深度超过上限 %d", xmlMaxDepth)
	}
	var children []any
	attrs := make(map[string]any)

	for {
		tok, err := decoder.Token()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("xml token: %w", err)
		}

		switch t := tok.(type) {
		case xml.StartElement:
			b.nodes++
			if b.nodes > xmlMaxNodes {
				return nil, fmt.Errorf("元素总数超过上限 %d", xmlMaxNodes)
			}
			// 收集属性，以 "-" 前缀存储
			elAttrs := make(map[string]any)
			for _, a := range t.Attr {
				elAttrs["-"+a.Name.Local] = a.Value
			}

			child, err := decodeXMLElement(decoder, t.Name.Local, depth+1, b)
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

// HTML 解析

func parseHTML(body []byte) (*goquery.Document, error) {
	doc, err := goquery.NewDocumentFromReader(bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("html: %w", err)
	}
	return doc, nil
}

// 正则

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
