# 动态配置（Profile）

`profile.json` 用于在不更新 Serein 自身的情况下，热更新前端行为和后端处理逻辑。

配置文件的地址在 `config.toml` 的 `[profile]` 段指定。推荐和规则源放在同一个仓库中，只需不将 `profile.json` 列入 `_source.json` 的 `files` 列表，它就不会被作为规则拉取。

## 格式

```json
{
  "version": 1,
  "known_extensions": ["exe", "zip", "dmg"],
  "version_prefixes": ["v", "V", "Version "],
  "version_suffixes": ["\n"]
}
```

Serein 内置了一组简单的动态配置，在首次启动时会写入。  

### version

整数，远端版本号大于本地时才会更新。内置的动态配置，`version` 为 0。

### known_extensions

字符串数组，列出可被下载器识别的文件扩展名。页面加载时前端从后端读取此列表，动态构造 `DOWNLOAD_EXTS` 正则用于判断下载链接是否可直接发送到下载器。
对于未能匹配到的内容，Serein 会询问是否在浏览器中打开。

### version_prefixes

字符串数组，列出需要从版本号开头去除的前缀。后端在每次检查更新后，自动对提取到的版本号字符串执行前缀裁剪。

**匹配规则：** 按字符串长度降序匹配，较长的前缀优先。

### version_suffixes

字符串数组，列出需要从版本号末尾去除的后缀。用于清理提取到的版本号中的多余字符（如换行符、空格）。

## 自定义和分发

这会改变 Serein 的行为，因此请谨慎修改。

若要自定义，只需要修改 `SEREIN_HOME/user/profile.json` 即可。  
若要分发，你需要自行上传这个文件，然后修改 `config.toml` 的 `[profile]` 段指向它。
