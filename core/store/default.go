package store

const DefaultConfigTOML = `# Serein 配置
[serein]
host = "127.0.0.1"                # 监听地址，设为 0.0.0.0 则允许局域网访问
port = 12510                      # 监听端口

[tracker]
platforms = ["macos", "windows"]  # 全局平台偏好，可被 Tracker 中的记录覆盖

[download]
concurrency = 8                   # 检查更新时的并发数，最大允许 64
# downloader = ""                 # 下载器：空=浏览器，ndm=NeatDM，也可填入命令行（{url}替换为链接）

[access]
# github_token = "github_xxx"     # GitHub 令牌，用于提升请求限制

[[rule_sources]]                  # 规则源，默认为 Vanadiry 维护的规则源。你可以添加新的 url 来指定更多
url = "https://raw.githubusercontent.com/Vanadiry/SereinRulesList/refs/heads/main/_source.json"

[profile]                          # 动态配置，指向一个 profile.json 文件，用于热更新前端参数
url = "https://raw.githubusercontent.com/Vanadiry/SereinRulesList/refs/heads/main/profile.json"
# [[rule_sources]]                # 规则源 2
# url = "https://xxx.xx/"
`

// tracker/*.toml 格式说明，每个文件可含多条 [[tracker]]
const DefaultTrackerTOML = `# Serein Tracker
display_name = "新建追踪列表"    # 可选，仅供前端展示
# order = 1                    # 可选，排序（数字小的在前）

# [[tracker]]
# app_id = "abc-123"
# platforms = ["macos"]        # 未指定则使用config.toml中的platforms

# [[tracker]]
# app_id = "def-456"
`

const DefaultProfileJSON = `{
  "version": 0,
  "known_extensions": [
    "exe",
    "zip",
    "dmg",
    "pkg",
    "msi",
    "apk",
    "ipa",
    "rar",
    "7z"
  ]
}
`
