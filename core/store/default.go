package store

// 默认 config.toml 模板
const DefaultConfigTOML = `# Serein 配置
[serein]
host = "127.0.0.1"                # 监听地址，设为 0.0.0.0 则允许局域网访问
port = 12510                      # 监听端口
platforms = ["macos", "windows"]  # 全局平台偏好，可被 Tracker 中的记录覆盖
concurrency = 8                   # 检查更新时的并发数，最大允许 64
# github_token = "github_xxx"     # GitHub 令牌，用于提升请求限制

[[rule_sources]]                  # 规则源，默认为 Vanadiry 维护的规则源。你可以添加新的 url 来指定更多
url = "https://raw.githubusercontent.com/Vanadiry/SereinRulesList/refs/heads/main/_source.json"
# [[rule_sources]                 # 规则源 2
# url = "https://xxx.xx/"
`

// tracker/*.toml 格式说明，每个文件可含多条 [[tracker]]
const DefaultTrackerTOML = `# Serein Tracker
display_name = "新建追踪列表"    # 可选，仅供前端展示

# [[tracker]]
# app_id = "abc-123"
# platforms = ["macos"]        # 未指定则使用config.toml中的platforms

# [[tracker]]
# app_id = "def-456"
`
