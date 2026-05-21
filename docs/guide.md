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
host = "127.0.0.1"                # 监听地址，设为 0.0.0.0 则允许局域网访问
port = 12510                      # 监听端口
platforms = ["macos", "windows"]  # 全局平台偏好，可被 Tracker 中的记录覆盖
concurrency = 8                   # 检查更新时的并发数，最大允许 64
github_token = "github_xxx"       # GitHub 令牌，用于提升请求限制

[[rule_sources]]                  # 规则源，默认为 Vanadiry 维护的规则源。你可以添加新的 url 来指定更多
url = "https://111.xx/"
[[rule_sources]]                  # 规则源 2
url = "https://222.xx/"
```

### GitHub 令牌

如果你追踪的位于 GitHub Release 的软件比较多，那么建议你配置这个。  
因为GitHub 对未认证请求限制 60 次/小时，配置 Token 后提升至 5000 次/小时。

访问 [GitHub Personal access tokens](https://github.com/settings/personal-access-tokens)，点击 Generate new token」。  
设置名称和你想要的 Expiration 过期时间，其他全部默认即可，然后创建。  
创建后，将 Token 写入到配置文件，即可提升请求次数上限。

Serein 不会收集你的 Token，请勿将 Token 外泄。

### 规则源

规则源可以配置多个，每条都必须指向一个 `_source.json` 文件，远端或本地皆可。  
如果你是规则编写者，请查看 [rules/source](rules/_source.md)。

## Tracker

Tracker 文件存放在主目录的 `tracker` 目录下，每个 `.toml` 文件为一个 Tracker，你可以创建多个。

基本格式为：

```toml
display_name = "Tracker 名称"   # 可选，仅供前端展示

[[tracker]]
app_id = "vanadiry-seshat"
platforms = ["macos"]          # 追踪平台。

[[tracker]]
app_id = "vanadiry-serein"
platforms = ["windows", "linux"]
```

Tracker 中的 `platforms` 优先级高于配置文件中的 `platforms`。  
当在配置文件中设置了追踪 A 和 B 平台，但 Tracker 中仅设置了 A，则最终只会检查 A 平台的更新。  
不写此字段，将继承配置文件中的 `platforms` 设置。

`_serein.toml` 为前端自动写入的默认 Tracker。  
为了便于你管理，更推荐你手动创建和编辑一个新的 Tracker。

## REPL

通过执行 `serein cli` 可以启用交互终端。  
这并非推荐的用法。如有需要，请查看 [CLI](cli.md)。
