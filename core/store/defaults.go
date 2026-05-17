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
