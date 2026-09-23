// 规则变量：规则里用 {{name}} 引用用户在 config 的 [rule_values] 里配置的私有值
// 按 app_id 作用域，无全局回落。替换在规则加载时进行，遍历所有字符串值
package store

import (
	"regexp"
	"strings"
)

var ruleValueRe = regexp.MustCompile(`\{\{([A-Za-z_][A-Za-z0-9_]*)\}\}`)

// substituteString 替换字符串里的 {{name}}，返回结果与未定义的变量名
func substituteString(s string, values map[string]string) (string, []string) {
	if !strings.Contains(s, "{{") {
		return s, nil
	}
	var missing []string
	out := ruleValueRe.ReplaceAllStringFunc(s, func(m string) string {
		name := m[2 : len(m)-2]
		v, ok := values[name]
		if !ok {
			missing = append(missing, name)
			return m
		}
		return v
	})
	return out, missing
}

// substituteRaw 递归替换 raw 中所有字符串值里的 {{name}}（不改动 map 的 key）
// 未定义的变量名收集到 missing
func substituteRaw(v any, values map[string]string, missing map[string]bool) any {
	switch t := v.(type) {
	case map[string]any:
		for k, vv := range t {
			t[k] = substituteRaw(vv, values, missing)
		}
		return t
	case []any:
		for i, vv := range t {
			t[i] = substituteRaw(vv, values, missing)
		}
		return t
	case string:
		out, miss := substituteString(t, values)
		for _, m := range miss {
			missing[m] = true
		}
		return out
	default:
		return v
	}
}
