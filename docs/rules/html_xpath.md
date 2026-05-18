# HTML XPath 规则

XPath 表达式定位取值。  
`/text()` 取文本，`/@attr` 取属性。

## 示例

规则表：

```toml
[config]
type = "html_xpath"
baseurl = "https://example.com"

[config.macos]
url = "https://example.com/download"
v_position = "//span[@class='version']/text()"
d_position = "//div[@class='dl']//a[contains(@href, 'dmg')]/@href"
```

对应 HTML 片段：

```html
<span class="version">1.2.3</span>
<div class="dl">
  <a href="/mac/app.dmg">macOS</a>
  <a href="/mac/app.exe">Windows</a>
</div>
```

`v_position` 取到 `1.2.3`。  
`d_position` 按 `href` 包含 `dmg` 筛选。  
`baseurl` 拼接为 `https://example.com/mac/app.dmg`。

## 适用场景

仅当 CSS 选择器无法满足时使用，如按元素文本定位或向上查找父节点。  
绝大多数场景用 [html_selector](html_selector.md) 即可。
