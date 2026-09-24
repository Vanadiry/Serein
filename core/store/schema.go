package store

import (
	"embed"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"sync"

	"github.com/santhosh-tekuri/jsonschema/v6"
	"github.com/santhosh-tekuri/jsonschema/v6/kind"
)

//go:embed rule.schema.json
var ruleSchemaFS embed.FS

var (
	ruleSchemaOnce sync.Once
	ruleSchemaErr  error
	ruleSchemas    map[string]*jsonschema.Schema
)

func loadRuleSchemas() error {
	ruleSchemaOnce.Do(func() {
		data, err := ruleSchemaFS.ReadFile("rule.schema.json")
		if err != nil {
			ruleSchemaErr = err
			return
		}
		var doc any
		if err := json.Unmarshal(data, &doc); err != nil {
			ruleSchemaErr = err
			return
		}
		c := jsonschema.NewCompiler()
		if err := c.AddResource("rule.schema.json", doc); err != nil {
			ruleSchemaErr = err
			return
		}
		ruleSchemas = map[string]*jsonschema.Schema{}
		for _, name := range []string{"info", "plat", "pre_step"} {
			sch, err := c.Compile("rule.schema.json#/$defs/" + name)
			if err != nil {
				ruleSchemaErr = err
				return
			}
			ruleSchemas[name] = sch
		}
	})
	return ruleSchemaErr
}

// validateSection 用 rule.schema.json 校验一个段，返回告警/错误
// name 为 $defs 下的定义名（info / plat / pre_step）
func validateSection(name, section string, raw map[string]any) []RuleIssue {
	if err := loadRuleSchemas(); err != nil {
		return nil // schema 出问题不阻塞规则解析
	}
	sch := ruleSchemas[name]
	if sch == nil {
		return nil
	}
	// 规范化为标准 JSON 值，避免 TOML 的 int64 等类型差异
	b, err := json.Marshal(raw)
	if err != nil {
		return nil
	}
	var v any
	if err := json.Unmarshal(b, &v); err != nil {
		return nil
	}
	if err := sch.Validate(v); err != nil {
		var ve *jsonschema.ValidationError
		if errors.As(err, &ve) {
			var issues []RuleIssue
			collectSchemaIssues(ve, section, &issues)
			return issues
		}
	}
	return nil
}

func collectSchemaIssues(e *jsonschema.ValidationError, section string, out *[]RuleIssue) {
	kw := ""
	if e.ErrorKind != nil {
		kw = strings.Join(e.ErrorKind.KeywordPath(), "/")
	}
	// oneOf 等只报一条，避免展开各分支的噪声
	if strings.HasPrefix(kw, "oneOf") || strings.HasPrefix(kw, "anyOf") || strings.HasPrefix(kw, "not") {
		*out = append(*out, schemaIssue(section, e.InstanceLocation, "warn", "结构不合法"))
		return
	}
	if len(e.Causes) > 0 {
		for _, c := range e.Causes {
			collectSchemaIssues(c, section, out)
		}
		return
	}

	level := "warn"
	var msg string
	switch k := e.ErrorKind.(type) {
	case *kind.Required:
		level = "error"
		msg = "缺少必填字段 " + strings.Join(k.Missing, ", ")
	case *kind.AdditionalProperties:
		msg = "未知字段 " + strings.Join(k.Properties, ", ")
	case *kind.Enum:
		msg = fmt.Sprintf("取值 %v 不在允许范围内", k.Got)
	case *kind.Type:
		msg = "类型不符，期望 " + strings.Join(k.Want, " 或 ")
	case *kind.MinItems:
		level = "error"
		msg = fmt.Sprintf("至少需要 %d 项", k.Want)
	case *kind.Minimum:
		msg = fmt.Sprintf("不能小于 %v", k.Want)
	case *kind.Maximum:
		msg = fmt.Sprintf("不能大于 %v", k.Want)
	default:
		msg = kw + " 校验失败"
	}
	*out = append(*out, schemaIssue(section, e.InstanceLocation, level, msg))
}

func schemaIssue(section string, loc []string, level, msg string) RuleIssue {
	p := section
	if len(loc) > 0 {
		p += "." + strings.Join(loc, ".")
	}
	return RuleIssue{Level: level, Message: p + ": " + msg}
}
