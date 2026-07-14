# 规则

## 创建规则表

首先，请前往 Serein 主目录的 `rules` 目录中，创建一个 `_local` 子目录。  
你的自定义规则表将存放在这里。

规则支持多种解析方式，并支持前置请求功能。  
请前往 [rules/toml](_toml.md) 查看规则文件格式。

规则支持如下匹配方式，请前往对应的文档查看：

- [JSON](json.md)
- [XML](xml.md)
- [正则表达式](regex.md)
- [HTML 选择器](html_selector.md)
- [GitHub Release](github.md)

内置规则，无须规则表：

- [VSCode VSIX 扩展](vsix.md)

## 创建规则源

规则源通过 `_source.json` 配置，允许远端或本地源。  
请查看 [Source](_source.md) 来了解 `_source.json` 格式。

## 动态配置

Serein 支持热更新前端行为，通过 `profile.json` 实现。  
请查看 [Profile](_profile.md) 来了解 `profile.json` 格式。推荐与规则源放在同一仓库中。
