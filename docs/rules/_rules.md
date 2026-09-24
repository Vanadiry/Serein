# 规则

## 创建规则表

首先，请前往 Serein 主目录的 `rules` 目录中，创建一个 `_local` 子目录。  
你的自定义规则表将存放在这里。

规则支持多种解析方式，并支持前置请求功能。  
首先，请阅读 [rules/toml](_toml.md) 来查看规则文件格式。

规则支持如下匹配方式，请前往对应的文档查看：

- [JSON](json.md)
- [XML](xml.md)
- [正则表达式](regex.md)
- [HTML 选择器](html_selector.md)
- [GitHub Release](github.md)
- [直通](direct.md)（仅用于 `v_type` / `d_type`）

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
