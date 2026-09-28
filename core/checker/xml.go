// XML 解析器，position 为层级数组，- 前缀取属性
// 与 JSON 共用同一套步进引擎，支持单路径和多路径拼接
package checker

import "fmt"

func extractXMLValue(body []byte, pos any, join string) (any, error) {
	root, err := parseXML(body)
	if err != nil {
		return nil, err
	}

	if join != "" {
		paths, ok := pos.([]any)
		if !ok {
			return nil, fmt.Errorf("xml 多路径的 position 值为 %T，不合法，应当为 [][]any", pos)
		}
		var typedPaths [][]any
		for _, p := range paths {
			typed, ok := p.([]any)
			if !ok {
				return nil, fmt.Errorf("xml 多路径的每一项值为 %T，不合法，应当为 []any", p)
			}
			typedPaths = append(typedPaths, typed)
		}
		return StepMulti(root, typedPaths, join)
	}

	path, ok := pos.([]any)
	if !ok {
		return nil, fmt.Errorf("xml 的 position 值为 %T，不合法，应当为 []any", pos)
	}
	return Step(root, path)
}
