# JSON 规则

请求 URL，解析 JSON，使用层级数组步进取值。

## 示例

规则表：

```toml
[config]
type = "json"
url = "https://api.example.com/releases/latest"
v_position = [
  [0, "version", "major"],
  [0, "version", "minor"],
  [0, "version", "patch"]
]
v_join = "."

[config.macos]
d_position = [0, "assets", "name~darwin-arm64", "url"]
```

对应 JSON：

```json
[
  {
    "version": { "major": 1, "minor": 2, "patch": 3 },
    "assets": [
      { "name": "app-win64.exe", "url": "https://example.com/win64" },
      { "name": "app-darwin-arm64.dmg", "url": "https://example.com/darwin-arm64" }
    ]
  }
]
```

## position 规则

| 值 | 当前类型 | 行为 |
| --- | ------- | ---- |
| 字符串（无 `=` / `~`） | 对象 | 取 key |
| 字符串 `key=value` | 数组 | 精确筛选 |
| 字符串 `key~regex` | 数组 | 正则筛选 |
| 整数 >= 0 | 数组 | 下标 |
| 整数 < 0 | 数组 | 倒数（-1 = 最后一个） |

当最终存在多个匹配项时，取第一个。

### 多路径拼接

传入多个 `position`，并设置拼接分隔符 `v_join`，可以将多个字符串的内容拼接为一个。

在本页首的示例中，版本号将被拼接为 `1.2.3`。
