# 入门

请确保至少读完此处的内容，再继续使用 Serein。

Serein 依赖 Tracker 和规则表来运行。  
Tracker 是一个列表，记录了你需要检查更新的软件 ID。  
程序会根据 ID，在规则表中查找，并使用找到的规则，来获取新版本的版本号和下载链接。

默认的规则表 [SereinRulesList](https://github.com/Vanadiry/SereinRulesList) 由 Vanadiry 维护。  
如果你需要新的规则，可以自行创建，或者在默认规则表的仓库中提交 Issue 或 PR。

## 配置

程序默认将 `~/.vSoft/Serein/` 视为程序的主目录。你可以通过环境变量 `SEREIN_HOME` 覆盖。  
配置文件位于主目录的 `config.toml` 文件。

Serein 首次启动时，会生成默认配置：

```toml
[serein]
host = "127.0.0.1"     # 监听地址，设为 0.0.0.0 则允许局域网访问
port = 12510           # 监听端口

[tracker]
platforms = ["macos", "windows"]  # 全局平台偏好，可被 Tracker 中的记录覆盖

[download]
concurrency = 8        # 检查更新时的并发数，最大允许 64
# downloader = "ndm"   # 下载器：空=浏览器，ndm=NeatDM，也可填入命令行（{url}替换为链接）

[access]
# github_token = "github_pat_xxx" # GitHub 令牌，用于提升请求限制

[[rule_sources]]                  # 规则源，默认为 Vanadiry 维护的规则源。你可以添加新的 url 来指定更多
url = "https://111.xx/"
[[rule_sources]]                  # 规则源 2
url = "https://222.xx/"
```

### GitHub 令牌

如果你追踪的位于 GitHub Release 的软件比较多，那么建议你配置这个。  
因为 GitHub 对未认证请求限制 60 次/小时，配置 Token 后提升至 5000 次/小时。

访问 [GitHub Personal access tokens](https://github.com/settings/personal-access-tokens)，点击 Generate new token。  
设置名称和你想要的 Expiration 过期时间，其他全部默认即可，然后创建。  
创建后，将 Token 写入到配置文件的 `[access]` 节，即可提升请求次数上限。

Serein 不会收集你的 Token，请勿将 Token 外泄。

### 下载器

在 `[download]` 节配置 `downloader`：

- 留空：下载链接在浏览器中打开，使用浏览器的下载
- `"ndm"`：Neat Download Manager（内建 WebSocket 支持）
- 自定义命令：如 `"aria2c {url}"`、`"IDMan.exe /d {url}"`，`{url}` 会被 Serein 替换为下载链接

### 规则源

规则源可以配置多个，每条都必须指向一个 `_source.json` 文件，远端或本地皆可。  
如果你是规则编写者，请查看 [rules/source](rules/_source.md)。

## Tracker

Tracker 文件存放在主目录的 `tracker` 目录下，每个 `.toml` 文件为一个 Tracker，你可以创建多个。

基本格式为：

```toml
display_name = "Tracker Name"  # 可选，仅供前端展示
order = 1                      # 可选，排序（数字小的在前）

[[tracker]]
app_id = "vanadiry-seshat"
platforms = ["macos"]          # 追踪平台。

[[tracker]]
app_id = "vanadiry-serein"
platforms = ["windows", "linux"]
```

Tracker 中的 `platforms` 优先级高于配置文件中的 `platforms`。  
当在配置文件中设置了追踪 A 和 B 平台，但 Tracker 中仅设置了 A，则最终只会检查 A 平台的更新。  
不写此字段，将继承 `[tracker]` 节中的 `platforms` 设置。

可以从规则页面搜索规则并添加到默认 Tracker。  
推荐你根据 Tracker 格式，手动创建和管理。

## 备份

Serein 依赖 Tracker 来取得你希望追踪的 AppID，依赖 Rules 中的规则来检查更新。  
你只需要备份主目录下的 `tracker` 文件夹即可。如果你自己写了规则，则还应备份你写的规则文件。

规则是独立于 Serein 本体的。  
如果你希望备份规则源中的内容，可以备份主目录下的 `rules` 文件夹，也可以克隆规则源的分发仓库。
