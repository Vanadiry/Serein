# 规则源

规则源是一个 JSON 文件，文件名统一为 `_source.json`。记录规则集的元信息和获取方式。  

## 格式

```json
{
  "source_id": "规则源唯一标识，推荐使用 UUID",
  "name": "显示名称（可选）",
  "description": "简介（可选）",
  "type": "解析模式：rules | list",
  "baseurl": "基路径（可选）",
  "files": ["文件1", "文件2..."]
}
```

`name` 和 `description` 可选，填写后可被前端渲染。

API 拉取规则时，程序逐个下载 `{baseurl}/{files[i]}` 到 Serein 主目录的 `rules/{source_id}/`。

若规则源 json 的位置，和文件位置不同，可以配置 `baseurl` 指定基目录。  
不写时，自动取 `_source.json` 的父目录。

## 递归解析

解析模式使用 `type` 设置，允许为 `rules`（默认）或 `list`。

当设置为 `list` 时，会将 `files` 中的文件视为子源，递归处理。子源的 json 名称依然需要是 `_source.json`。

```text
├── _source.json      # { "type": "list", "files": ["001/_source.json", "002/_source.json"] }
├── 001/
│   ├── _source.json  # { "source_id": "001", "type": "rules", "files": [...] }
│   └── app1.toml
└── 002/
    ├── _source.json  # { "source_id": "002", "type": "rules", "files": [...] }
    └── app2.toml
```

递归解析需要注意下面几点：

- 子源的 `source_id` 必须等于所在目录名。
- `baseurl` 继承父级。
- API 只展示 `type = "rules"` 的叶子源（`001`、`002`），跳过 `type = "list"` 的 json 数据。

## ID 冲突

多个规则源 `source_id` 相同时，按 `config.toml` 中 `[[rule_sources]]` 顺序取前者，后者将跳过。
