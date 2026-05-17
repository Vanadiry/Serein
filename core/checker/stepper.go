package checker

import (
	"fmt"
	"regexp"
	"strings"
)

// Step 在数据树中按 position 数组逐层步进，返回最终值。
// 支持 JSON（encoding/json 解出的 any）和 XML（转换后同构的树）。
func Step(root any, position []any) (any, error) {
	cur := root
	for i, step := range position {
		var err error
		cur, err = stepOne(cur, step)
		if err != nil {
			return nil, fmt.Errorf("step %d (%v): %w", i, step, err)
		}
	}
	return cur, nil
}

func stepOne(cur any, step any) (any, error) {
	switch s := step.(type) {
	case string:
		return stepString(cur, s)
	case int64:
		return stepIndex(cur, int(s))
	case int:
		return stepIndex(cur, s)
	case float64:
		return stepIndex(cur, int(s))
	default:
		return nil, fmt.Errorf("unsupported step type %T", step)
	}
}

func stepString(cur any, s string) (any, error) {
	// 含 = 或 ~ → 数组筛选
	if idx := strings.IndexByte(s, '='); idx >= 0 {
		key, val := s[:idx], s[idx+1:]
		return filterArray(cur, key, val, false)
	}
	if idx := strings.IndexByte(s, '~'); idx >= 0 {
		key, pattern := s[:idx], s[idx+1:]
		return filterArray(cur, key, pattern, true)
	}

	// 普通 key
	m, ok := cur.(map[string]any)
	if !ok {
		return nil, fmt.Errorf("want object for key %q, got %T", s, cur)
	}
	v, ok := m[s]
	if !ok {
		return nil, fmt.Errorf("key %q not found", s)
	}
	return v, nil
}

func stepIndex(cur any, idx int) (any, error) {
	arr, ok := cur.([]any)
	if !ok {
		return nil, fmt.Errorf("want array for index %d, got %T", idx, cur)
	}
	if idx < 0 {
		idx = len(arr) + idx
	}
	if idx < 0 || idx >= len(arr) {
		return nil, fmt.Errorf("index %d out of range (len %d)", idx, len(arr))
	}
	return arr[idx], nil
}

func filterArray(cur any, key, val string, useRegex bool) (any, error) {
	arr, ok := cur.([]any)
	if !ok {
		return nil, fmt.Errorf("want array for filter %q=%q, got %T", key, val, cur)
	}

	var re *regexp.Regexp
	if useRegex {
		var err error
		re, err = regexp.Compile(val)
		if err != nil {
			return nil, fmt.Errorf("regex %q: %w", val, err)
		}
	}

	for _, item := range arr {
		m, ok := item.(map[string]any)
		if !ok {
			continue
		}
		itemVal, exists := m[key]
		if !exists {
			continue
		}
		strVal := fmt.Sprintf("%v", itemVal)
		if useRegex {
			if re.MatchString(strVal) {
				return item, nil
			}
		} else {
			if strVal == val {
				return item, nil
			}
		}
	}
	return nil, fmt.Errorf("no match for filter %q", key)
}

// StepMulti 多路径拼接模式：逐条取值后用 join 拼接。
func StepMulti(root any, paths [][]any, join string) (string, error) {
	var parts []string
	for _, path := range paths {
		v, err := Step(root, path)
		if err != nil {
			return "", err
		}
		parts = append(parts, fmt.Sprintf("%v", v))
	}
	return strings.Join(parts, join), nil
}
