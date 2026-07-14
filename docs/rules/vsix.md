# VSIX 扩展

Serein 内置支持检查 VSCode 扩展的更新，无需编写规则表。

## 支持的市场

- **VS 插件市场**：Visual Studio Marketplace（`type: "msvsix"`）
- **OpenVSX**：Open VSX Registry（`type: "openvsx"`）

两者使用方式完全一致，仅下载源不同。  
通常而言，msvsix 适合 VSCode 用户，openvsx 适合 VSCodium 等开源构建。

## Tracker

只需要创建如下的 Tracker 列表即可，不需要写平台。

```toml
display_name = "名称"
type = "msvsix"

[[tracker]]
app_id = "ms-python.python"

[[tracker]]
app_id = "ms-vscode.cpptools"
```

`type` 为你想要连接的插件市场代号，为 `msvsix` 或 `openvsx`。

`app_id` 为插件的 ID。  
你可以在 VSCode 等软件的插件页中，右键一个插件，然后选择“复制 ID”。  
或者，在插件的详情页面右侧栏中，查看“标识符”部分。  
插件的 ID 是 `publisher.extension` 格式的。
