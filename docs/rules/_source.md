# 规则源

规则源是一个 JSON 文件，文件名统一为 `_source.json`。记录规则源的元信息和获取方式。  

## 格式

`files` 的形态随 `type` 变化，两者不可混用。

```jsonc
// type = "rules"（默认）
{
  "source_id": "规则源唯一标识",
  "name": "显示名称（可选）",
  "description": "简介（可选）",
  "type": "rules",
  "baseurl": "基路径（可选）",
  "files": {
    "app1.toml": "3",
    "app2.toml": "1"
  }
}
```

```jsonc
// type = "list"
{
  "source_id": "父源唯一标识",
  "type": "list",
  "baseurl": "基路径（可选）",
  "files": [
    "001/_source.json",
    "002/_source.json"
  ]
}
```

`name` 和 `description` 可选，填写后可被前端渲染。

API 拉取规则时，程序逐个下载 `{baseurl}/{files键}` 到 Serein 主目录的 `rules/{source_id}/`。

若规则源 json 的位置，和文件位置不同，可以配置 `baseurl` 指定基目录。  
不写时，自动取 `_source.json` 的父目录。

## 版本

`rules` 型源里每个文件对应一个版本字符串。  
Serein 会将其与本地已记录的值比较，若不同，则视为又更新，会以远端覆盖。  
版本字符串可以为任何内容，通常建议为任意哈希值，或者从 1 开始的递增数字。

## 递归解析

当 `type = "list"` 时，该源即为父规则源，其下嵌套规则子源，递归处理。  
规则子源的 json 名称依然是 `_source.json`。

```text
├── _source.json
├── 001/
│   ├── _source.json
│   └── app1.toml
└── 002/
    ├── _source.json
    └── app2.toml
```

递归解析需要注意下面几点：

- 规则子源的 `source_id` 必须等于所在目录名，不一致则该规则子源被忽略并报错。
- `baseurl` 继承父级。
- `list` 型源自身的 `_source.json` 不落盘。
- API 只展示 `type = "rules"` 的规则子源（`001`、`002`），跳过 `type = "list"` 的父规则源。

## 体积上限

| 上限 | 值 | 触发后 |
| --- | --- | --- |
| 单文件 | 8 MB | 该文件失败 |
| 单源文件数 | 20000 | 该源整体拒绝 |
| 单源本轮下载体积 | 64 MB | 放弃该源，保留本地现有规则 |

## ID 冲突

多个规则源 `source_id` 相同时，按 `config.toml` 中 `[[rule_sources]]` 顺序取前者，后者将跳过。
