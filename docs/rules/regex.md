# 正则表达式规则

正则适用于任何 HTTP 响应，会直接对响应全文做正则匹配，适合结构不规则的页面。

## 示例

规则表：

```toml
[config]
type = "regex"
baseurl = "https://example.com"

[config.windows]
url = "https://example.com/download"
v_position = "<b>(\\d+\\.\\d+\\.\\d+\\.\\d+)</b>"
d_position = "<a href=\"(/example\\.zip)\""
```

对应 HTML 片段：

```html
<b>1.5.3.170</b>
<b>1.5.2</b>
<b>1.4.3</b>
<a href="/example.zip">ZIP</a>
```

## 注意事项

- 正则必须包含一个捕获组 `()`，取第一个捕获组的值
- 对于示例中 `v_position` 这样多个符合的结果，取第一个
