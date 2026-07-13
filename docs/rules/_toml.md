# 规则源格式

每条规则一个 `.toml` 文件，程序递归加载 `rules/` 下所有内容。  

## 结构

```toml
[info]
app_id = "vanadiry-seshat"         # 唯一标识，更推荐直接用 UUID。重复时自动取先读到的
name = "Seshat"                    # 名称，会在前端显示
platforms = ["macos", "windows"]   # 平台
description = "番组计划 Tracker 管理工具"   # 可选，描述
status = ["ABC"]                    # 可选，状态。若有值，前端将展示一个黄点
official_website = "https://github.com/Vanadiry/Seshat"   # 可选，官网

[config]
type = "github"
url = "https://..."
v_position = "..."

[config.macos]
d_position = "..."

[config.windows]
d_position = "..."
```

## [config]

| 字段 | 说明 |
| ---- | ---- |
| `type` | 解析器类型 |
| `url` | 请求地址（前置请求或各平台有 URL 时可省略） |
| `ua` | 自定义 UA（可选） |
| `headers` | 自定义请求头内联字典（可选） |
| `baseurl` | 拼接相对路径（仅 d_position 非直通模式时生效） |
| `v_position` | 版本号定位（各解析器格式不同，详见各解析器文档） |
| `d_position` | 下载链接定位 |
| `v_join` | 版本号多路径拼接分隔符 |
| `d_join` | 下载链接多路径拼接分隔符 |
| `v_url` | 版本号独立请求地址，覆盖 `url`（可选） |
| `v_type` | 版本号独立解析器类型，覆盖 `type`（可选） |
| `d_url` | 下载链接独立请求地址，覆盖 `url`（可选） |
| `d_type` | 下载链接独立解析器类型，覆盖 `type`（可选） |

`[config.{os}]` 中同名字段覆盖 `[config]`，未覆盖则继承。

## 分开请求版本号与下载链接

当版本号和下载链接位于不同页面时，使用 `v_url` 和 `d_url` 分别指定：

```toml
[config]
type = "json"
url = "https://api.example.com/releases/latest"
v_position = ["tag_name"]

# 下载链接从另一个页面获取
d_url = "https://example.com/download"
d_type = "html_selector"
d_position = { selector = ".link", attr = "href" }
```

未设置 `v_url` / `d_url` 时，回退到 `url`。未设置 `v_type` / `d_type` 时，回退到 `type`。

## 直通模式

当下载链接（或版本号）有固定规律、不需要解析页面时，使用 `d_type = "direct"`（或 `v_type = "direct"`）。

程序不会发起 HTTP 请求，直接将 `d_url` 字符串作为下载链接返回。字符串中的 `{version}` 会被替换为当前提取到的版本号。

```toml
[config]
type = "json"
url = "https://api.example.com/releases/latest"
v_position = ["tag_name"]

d_type = "direct"
d_url = "https://cdn.example.com/releases/{version}/app.dmg"
```

> **限制**：`direct` 只能用于 `v_type` 或 `d_type`，不能作为主 `type`。`{version}` 无法替换时（如版本号提取失败），下载链接直接报错。

## 解析器类型

- [JSON](json.md)
- [XML](xml.md)
- [正则表达式](regex.md)
- [HTML 选择器](html_selector.md)
- [GitHub Release](github.md)
- 直通（仅用于 `v_type` / `d_type`，详见上方[直通模式](#直通模式)）

## 前置请求

当需要多步跳转时，用 `[pre_request.NNN]` 定义链式请求：

```toml
[pre_request.001]
url = "https://example.com/downloads"
type = "html_selector"
position = { selector = ".latest a", attr = "href" }

[pre_request.002]
type = "json"
position = ["data", "download_url"]
```

上一步提取的 URL 自动成为下一步的请求地址。`position` 只提取一个链接。

支持分平台：`[pre_request.001.macos]`、`[pre_request.001.windows]`。

## 格式规范

### 合并规则

程序处理每个平台时，会根据 `platforms` 记录的平台列表，返回每个平台的数据。  
你可以在 `[config.{os}]` 中，分别设置每个平台的所有 config。

在 `[config]` 中记录的内容，是所有平台共用的内容。`[config.{os}]` 记录的是单独这个平台使用的内容。  
若每个平台的下载地址、版本都一模一样，全部写到 `[config]` 中即可。（这种情况应该很少吧）

因此，你可以给每个平台都设置不同的解析器类型。

## 提示

当遇到 403 错误以及需要人机验证的页面，可以尝试使用 `curl`。  
如果 `curl` 能够获取到前端代码，则在 `[config]` 中加入：`ua = "curl/8.0.0"` 大概率可以绕过。
