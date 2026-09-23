// 规则变量：规则里用 {{name}} 引用用户在 config 的 [rule_values] 里配置的私有值
// 按 app_id 作用域，无全局回落；未定义的变量直接报错
package store

import (
	"fmt"
	"regexp"
	"strings"
)

var ruleValueRe = regexp.MustCompile(`\{\{([A-Za-z_][A-Za-z0-9_]*)\}\}`)

// SubstituteRuleValues 替换字符串里的 {{name}}。出现未定义变量时返回错误
func SubstituteRuleValues(s string, values map[string]string) (string, error) {
	if !strings.Contains(s, "{{") {
		return s, nil
	}
	var missing string
	out := ruleValueRe.ReplaceAllStringFunc(s, func(m string) string {
		name := m[2 : len(m)-2]
		v, ok := values[name]
		if !ok {
			missing = name
			return m
		}
		return v
	})
	if missing != "" {
		return "", fmt.Errorf("未配置规则变量 %s", missing)
	}
	return out, nil
}

// ApplyRuleValues 替换 PlatConfig 中允许出现变量的字段（url/v_url/d_url/baseurl/headers）
// 不修改入参的 map（headers 会复制）。position 字段不替换
func (c PlatConfig) ApplyRuleValues(values map[string]string) (PlatConfig, error) {
	var err error
	if c.URL, err = SubstituteRuleValues(c.URL, values); err != nil {
		return c, err
	}
	if c.VURL, err = SubstituteRuleValues(c.VURL, values); err != nil {
		return c, err
	}
	if c.DURL, err = SubstituteRuleValues(c.DURL, values); err != nil {
		return c, err
	}
	if c.BaseURL, err = SubstituteRuleValues(c.BaseURL, values); err != nil {
		return c, err
	}
	if c.Headers != nil {
		nh := make(map[string]string, len(c.Headers))
		for k, v := range c.Headers {
			nv, e := SubstituteRuleValues(v, values)
			if e != nil {
				return c, e
			}
			nh[k] = nv
		}
		c.Headers = nh
	}
	return c, nil
}

// ApplyRuleValues 替换 PreRequestStep 中允许出现变量的字段（url/baseurl/headers）
func (s PreRequestStep) ApplyRuleValues(values map[string]string) (PreRequestStep, error) {
	var err error
	if s.URL, err = SubstituteRuleValues(s.URL, values); err != nil {
		return s, err
	}
	if s.BaseURL, err = SubstituteRuleValues(s.BaseURL, values); err != nil {
		return s, err
	}
	if s.Headers != nil {
		nh := make(map[string]string, len(s.Headers))
		for k, v := range s.Headers {
			nv, e := SubstituteRuleValues(v, values)
			if e != nil {
				return s, e
			}
			nh[k] = nv
		}
		s.Headers = nh
	}
	return s, nil
}
