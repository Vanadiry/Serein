package store

// 默认 config.toml 模板
const DefaultConfigTOML = `# Serein 配置文件
host = "127.0.0.1"
port = 12510
platforms = ["macos", "windows"]

# 规则源列表（可选）
# [[rule_sources]]
# url = "https://raw.githubusercontent.com/Vanadiry/SereinRulesList/refs/heads/main/source.json"
`

// tracker/*.toml 格式说明，每个文件可含多条 [[tracker]]
const DefaultTrackerTOML = `# display_name = "我的追踪列表"   # 可选，前端展示用

# [[tracker]]
# rule_id = "abc-123"
# platforms = ["macos"]   # 未指定则使用 config.toml 中的 platforms

# [[tracker]]
# rule_id = "def-456"
`
