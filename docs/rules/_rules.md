# 规则

## 创建规则表

首先，请前往 Serein 主目录的 `rules` 目录中，创建一个 `_local` 子目录。  
你的自定义规则表将存放在这里。

规则支持多种解析方式，并支持前置请求功能。  
首先，请阅读 [rules/toml](_toml.md) 来查看规则文件格式。

规则支持如下解析器类型，请前往对应的文档查看：

- [JSON 规则](json.md)
- [XML 规则](xml.md)
- [正则表达式规则](regex.md)
- [HTML 选择器规则](html_selector.md)
- [GitHub 规则](github.md)
- [直通模式](direct.md)（仅用于 `v_type` / `d_type`）

内置规则，无须规则表：

- [VSCode VSIX 扩展](vsix.md)

## 创建规则源

创建规则源，可以方便的组织和分发规则表，并支持在 Serein 中订阅。

规则源通过 `_source.json` 配置，允许远端或本地来源。  
请查看[规则源](_source.md)来了解 `_source.json` 格式。

## 动态配置

动态配置能热更新 Serein 的部分行为，例如版本号前后缀去除等。

动态配置通过 `profile.json` 实现，允许远端或本地来源。  
请查看[动态配置](_profile.md)来了解 `profile.json` 格式。  
推荐与规则源放在同一仓库中。

## 本地开发校验

规则编写期间，可以在 `config.toml` 中，配置指向你开发目录的 `_source.json` 文件。

```toml
[serein]
rule_source_dev = "~/Project/SereinRulesList/_source.json"
# 路径支持 ~ 展开，仅支持本地路径
```

配置后，可以在规则页面顶栏，点击「检查规则 [Dev]」按钮，来方便的检查规则是否编写错误。  
此功能仅作规则表的检查，不会与已有的规则源合并，也不会展示在规则列表中。

这会检查：

1. `_source.json` 的 `files` 中列出的文件是否存在
2. 是否有未列入 `files` 的多余 `.toml`
3. 每个规则表的结构与语义问题。
