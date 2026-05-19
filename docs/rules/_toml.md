# 规则源格式

每条规则一个 `.toml` 文件，程序递归加载 `rules/` 下所有内容。  

## 结构

```toml
[info]
app_id = "vanadiry-seshat"         # 唯一标识，更推荐直接用 UUID。重复时自动取先读到的
name = "Seshat"                    # 名称，会在前端显示
platforms = ["macos", "windows"]   # 平台
description = "番组计划 Tracker 管理工具"   # 可选，描述
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
| `baseurl` | 拼接相对路径（仅 d_position 生效） |
| `v_position` | 版本号定位（各解析器格式不同，详见各解析器文档） |
| `d_position` | 下载链接定位 |
| `v_join` | 版本号多路径拼接分隔符 |
| `d_join` | 下载链接多路径拼接分隔符 |

`[config.{os}]` 中同名字段覆盖 `[config]`，未覆盖则继承。

## 合并规则

程序处理每个平台时，会根据 `platforms` 记录的平台列表，返回每个平台的数据。  
你可以在 `[config.{os}]` 中，分别设置每个平台的所有 config。

在 `[config]` 中记录的内容，是所有平台共用的内容。`[config.{os}]` 记录的是单独这个平台使用的内容。  
若每个平台的下载地址、版本都一模一样，全部写到 `[config]` 中即可。（这种情况应该很少吧）

因此，你可以给每个平台都设置不同的解析器类型。

## 解析器类型

- [JSON](json.md)
- [XML](xml.md)
- [正则表达式](regex.md)
- [HTML 选择器](html_selector.md)
- [HTML XPath](html_xpath.md)
- [GitHub Release](github.md)

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
