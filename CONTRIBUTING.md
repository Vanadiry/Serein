# 贡献

## 功能开发

目前不接受大功能的 PR，有需求请先提交 Issue。

## 规则贡献

内置规则源的仓库，位于 [Vanadiry/SereinRulesList](https://github.com/Vanadiry/SereinRulesList)。  
若要贡献规则或修正错误，请前往上述仓库。

## 对接 Serein

[docs/api/openapi.yaml](/docs/api/openapi.yaml) 面向对接 Serein 的开发者。

## 构建

GitHub Release 的软件包，由 GitHub Actions 构建。  
本地构建，需要 `go`、`rust`、`pnpm`、`uv`。

```bash
pnpm install
uv run scripts/build.py server/desktop/sdev/dev
```
