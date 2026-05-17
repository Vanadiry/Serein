// JSON 解析器：position 为层级数组，复用通用步进引擎。
// 支持单路径和多路径拼接（v_join/d_join）。
package checker

import "fmt"

func extractJSONValue(body []byte, pos any, join string) (any, error) {
	root, err := parseJSON(body)
	if err != nil {
		return nil, err
	}
	return stepJSON(root, pos, join)
}

// stepJSON 对已解析的 JSON 树执行步进（供 GitHub checker 复用）。
func stepJSON(root any, pos any, join string) (any, error) {
	if join != "" {
		paths, ok := pos.([]any)
		if !ok {
			return nil, fmt.Errorf("json multi-path: want [][]any, got %T", pos)
		}
		var typedPaths [][]any
		for _, p := range paths {
			typed, ok := p.([]any)
			if !ok {
				return nil, fmt.Errorf("json multi-path: inner must be []any, got %T", p)
			}
			typedPaths = append(typedPaths, typed)
		}
		return StepMulti(root, typedPaths, join)
	}

	path, ok := pos.([]any)
	if !ok {
		return nil, fmt.Errorf("json position must be []any, got %T", pos)
	}
	return Step(root, path)
}
