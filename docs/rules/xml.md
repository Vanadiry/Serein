# XML 规则

将 XML 转换为与 JSON 同构的树，使用同一套步进引擎。
属性以 `-` 前缀存储。

## 示例

规则表：

```toml
[config]
type = "xml"
url = "https://example.com/updates.xml"
v_position = [
  ["Version", "Major"],
  ["Version", "Minor"]
]
v_join = "."
d_position = ["channel", "item", "enclosure", "-url~dmg", "-url"]
```

对应 XML：

```xml
<updates>
  <Version>
    <Major>26</Major>
    <Minor>4</Minor>
  </Version>
  <channel>
    <item>
      <enclosure url="https://example.com/app.dmg"/>
      <enclosure url="https://example.com/app.exe"/>
    </item>
  </channel>
</updates>
```

等效 JSON：

```json
{
  "Version": {
    "Major": { "#text": "26" },
    "Minor": { "#text": "4" }
  },
  "channel": {
    "item": {
      "enclosure": [
        { "-url": "https://example.com/app.dmg" },
        { "-url": "https://example.com/app.exe" }
      ]
    }
  }
}
```

版本号拼接为 `"26.4"`，下载链接通过 `-type~dmg` 筛选出 dmg。

## position 规则

与 [JSON](json.md) 相同。  
属性用 `-` 前缀，如 `"-url"` 取 `url` 属性。  
多路径拼接同样用 `v_join` / `d_join` 组合多个 path，规则同上。

## 须知

- 最外层标签不需要写入 position，直接从下一级开始。
- XML 在同层级有多个同名元素时，会转为数组，需要额外一步 `~` 或 `=` 或下标来进入目标。

详细参考等效 JSON。
