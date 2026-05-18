# HTML 选择器规则

CSS 选择器定位元素，取第一个匹配项。

## 示例

规则表：

```toml
[config]
type = "html_selector"
baseurl = "https://example.com"

[config.macos]
url = "https://example.com/download"
v_position = { selector = ".version", regex = "(\\d+\\.\\d+\\.\\d+)" }
d_position = { selector = ".mac-download", attr = "href" }
```

对应 HTML 片段：

```html
<span class="version">Version 1.2.3</span>
<a class="mac-download" href="/mac/app.dmg">macOS 下载</a>
```

`v_position` 选中 `.version` 文本，正则提取 `1.2.3`。  
`d_position` 选中 `.mac-download` 的 `href`。  
`baseurl` 拼接为 `https://example.com/mac/app.dmg`。

## position 字段

| 写法 | 行为 |
| ---- | ---- |
| `{ selector }` | 取元素文本 |
| `{ selector, attr }` | 取属性值（默认 `href`） |
| `{ selector, regex }` | 取文本后正则捕获组 |
