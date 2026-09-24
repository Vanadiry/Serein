# 直通模式

仅能在 `v_type` 和/或 `d_type` 中配置直通模式。不能作为主 `type`。

当版本号或下载链接有固定规律、不需要请求页面解析时使用。  
程序会直接把字符串当作结果返回，而不对配置直通的部分发起 HTTP 请求。

## 下载链接直通

`d_type = "direct"` 时，`d_url` 直接作为下载链接。  
如果下载链接中需要拼接当前版本号，可以使用 `{version}` 来替换为提取到的版本号。

```toml
[config]
type = "json"
url = "https://api.example.com/releases/latest"
v_position = ["tag_name"]

d_type = "direct"
d_url = "https://cdn.example.com/releases/{version}/app.dmg"
```

## 版本号直通

`v_type = "direct"` 时，`v_url` 直接作为版本号。  
需要注意的是，直通模式下的版本号，仍会被[动态配置](_profile.md)中的规则处理。

```toml
[config]
type = "json"
v_type = "direct"
v_url = "1.2.3"

d_type = "direct"
d_url = "https://cdn.example.com/app-{version}.zip"
```

通常情况下，不建议对版本号设置直通。  
版本号直通下，检查结果不会随实际发布变化，Serein 前端将不能很好的展示更新。
