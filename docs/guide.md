# 入门

请确保至少读完此处的内容，再继续使用 Serein。

Serein 依赖 Tracker 和规则表来运行。  
Tracker 是一个列表，记录了你需要检查更新的软件 ID。  
程序会根据 ID，在规则中查找，并使用找到的规则，来获取新版本的版本号和下载链接。

默认的规则源 [SereinRulesList](https://github.com/Vanadiry/SereinRulesList) 由 Vanadiry 维护。  
如果你需要新的规则，可以自行创建，或者在默认规则源的仓库中提交 Issue 或 PR。

## 配置

程序默认将 `~/.vSoft/Serein/` 视为程序的主目录。你可以通过环境变量 `SEREIN_HOME` 覆盖。  
配置文件位于主目录的 `config.toml` 文件。

Serein 首次启动时，会生成默认配置：

```toml
# Serein 配置
[serein]
host = "127.0.0.1"     # 监听地址，设为 0.0.0.0 则允许局域网访问
port = 12510           # 监听端口
first_run = true       # 首次运行时展示欢迎指引，设为 false 可永久关闭

[tracker]
platforms = ["macos", "windows"]  # 全局平台偏好，可被 Tracker 中的记录覆盖

[download]
concurrency = 8        # 检查更新时的并发数
# downloader = "browser"   # 下载器：browser（默认/空）| ndm | 自定义命令（{url}替换为链接）

[access]
# github_token = "github_xxx" # GitHub 令牌，用于提升请求限制，对所有发往 api.github.com 的请求生效

[rule_values]          # 规则变量，用于填充规则中的自定义变量
app_id.values = "xxx"
app_id2.values2 = "xxx"

[proxy]
host = ""              # HTTP 代理，留空则不启用
port = 0

[[rule_sources]]       # 规则源，默认为 Vanadiry 维护的规则源。你可以添加新的 url 来指定更多
url = "https://raw.githubusercontent.com/Vanadiry/SereinRulesList/refs/heads/main/_source.json"
# [[rule_sources]]     # 规则源 2
# url = "https://xxx.xx/"

[profile]              # 动态配置，指向一个 profile.json 文件，用于热更新前端参数
url = "https://raw.githubusercontent.com/Vanadiry/SereinRulesList/refs/heads/main/profile.json"
```

### GitHub 令牌

如果你追踪的位于 GitHub Release 的软件比较多，那么建议你配置这个。  
因为 GitHub 对未认证请求限制 60 次/小时，配置 Token 后提升至 5000 次/小时。

访问 [GitHub Personal access tokens](https://github.com/settings/personal-access-tokens)，点击 Generate new token。  
设置名称和你想要的 Expiration 过期时间，其他全部默认即可，然后创建。  
创建后，将 Token 写入到配置文件的 `[access]` 节，即可提升请求次数上限。

Serein 不会收集你的 Token，请勿将 Token 外泄。

### 下载器

在 `[download]` 节配置 `downloader`，留空则为默认：

- `browser`（默认）：在浏览器中打开下载链接，调用浏览器下载
- `ndm`：Neat Download Manager
- 自定义命令：如 `"aria2c {url}"`、`"IDMan.exe /d {url}"`，`{url}` 会被 Serein 替换为下载链接（程序路径请勿含空格）

远程访问（非本机）时，为避免在服务器上触发下载，仅支持在浏览器中下载。  
发送到下载器与拉起浏览器仅在运行 Serein 的本机可用。

### 规则源

规则源可以配置多个，每条都必须指向一个 `_source.json` 文件，远端或本地皆可。  
如果你是规则编写者，请查看 [rules/source](rules/_source.md)。

### 动态配置

Serein 从远端或本地拉取 `profile.json` 来自动更新前端行为，例如何种链接被下载器捕获、版本前后缀去除。  
你可以在规则页面右上角拉取动态配置。

### 规则变量

部分规则中，声明了自定义的变量。  
在 `[rule_values]` 中填写之后，Serein 就能自动替换这些内容。  

```toml
[rule_values]
FreeFileSync_Donation_Private.token = "abcdefg"
# 均为 app_id.value_name 这样的键名
AnotherApp.value = "yyy"
```

这项功能允许你持久地储存 Token 等数据，而不必每次更新规则都重新修改。  
若有你未配置的变量，程序会跳过这条检查，并报出提示。

### 代理

如果你的网络无法直接访问上游（规则源、GitHub Release 等），可以为后端配置 HTTP 代理。  
需要同时填写 `host`（IP 或域名）与 `port`，留空 `host` 即不启用代理。例如：

```toml
[proxy]
host = "127.0.0.1"
port = 7890
```

该代理作用于后端的检查更新、拉取规则源与动态配置，以及下载代理（`/api/file`）的上游请求。  
由浏览器或下载器直接发起的下载不受影响。

如果你的代理工具支持 TUN 模式，启动它，将无须单独为 Serein 配置代理。  
（视工具不同，此模式可能叫作：虚拟网卡模式、增强模式、接管所有流量、透明代理等）

## Tracker

Tracker 文件存放在主目录的 `tracker` 目录下，每个 `.toml` 文件为一个 Tracker，你可以创建多个。

基本格式为：

```toml
display_name = "Tracker Name"  # 可选，仅供前端展示
order = 1                      # 可选，排序（数字小的在前）
type = "app"                   # 可选，Tracker 类型（默认为 app，或 msvsix / openvsx）

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

在规则页选中规则源后，可为规则勾选平台：  
点「追踪」把它加入默认 Tracker，或点「复制」复制一段 `[[tracker]]` 片段，自行粘贴到你的 Tracker 文件。  
推荐你根据 Tracker 格式，手动创建和管理。

对于 VSCode 使用的 VSIX 扩展，Serein 内置了一套规则。  
这使得无须规则表即可追踪来自“VS 插件市场”和“OpenVSX”的内容。

如何编写 Tracker，请查看 [rules/vsix](rules/vsix.md)

## 备份

主目录下共有 3 个子目录、1 个文件存放重要数据。

- `rules/`：规则
- `tracker/`：追踪列表
- `user/`：已追踪软件版本、确认时间，以及动态配置
- `config.toml`：Serein 主配置

通常，你只需要备份 `tracker/` 和 `user/profile.json` 即可。  
你可以按需求决定。
