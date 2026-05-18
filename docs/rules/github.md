# GitHub 规则

GitHub 规则是一个预设的 JSON 规则，复用 JSON 步进引擎。  
除了 toml 必须的字段外，仅需要 `owner`、`repo`、`d_position` 即可。

## config 写法

规则表：

```toml
[config]
type = "github"
owner = "Vanadiry"
repo = "Seshat"

[config.macos]
d_position = "darwin-arm64"

[config.windows]
d_position = "exe"
```

对应 GitHub Release：

```text
seshat-v0.2.5-darwin-arm64
seshat-v0.2.5-windows-amd64.exe
```

## 注意事项

- `v_position` 不需要写，版本号始终从 `tag_name` 提取，并自动去 `v` 前缀
- `d_position` 是为正则，匹配 asset 文件名
- 同平台匹配多个 asset 时全部返回
- 不需要 `url`，由 `owner/repo` 自动拼接
