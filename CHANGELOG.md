# Changelog

## [v1.0.0] - 2026-06-07

### 破坏性更新

- **配置文件结构变更**：`config.toml` 从原来的 `[serein]` 单一节拆分为四个独立节。请将旧配置迁移：
  ```toml
  # 旧
  [serein]
  host = "127.0.0.1"
  port = 12510
  platforms = ["macos", "windows"]
  concurrency = 8
  github_token = "xxx"

  # 新
  [serein]
  host = "127.0.0.1"
  port = 12510

  [tracker]
  platforms = ["macos", "windows"]

  [download]
  concurrency = 8

  [access]
  github_token = "xxx"
  ```

- **REPL 模式已删除**：`serein cli` 不再可用，仅保留 WebUI 模式。

- **API 变更**：检查更新和同步规则现在均返回 `{"task_id": "xxx"}`，通过 `GET /api/progress/{task_id}` 获取 SSE 进度。旧的同步返回 `{synced, skipped, errors}` 格式已移除。

### Added

- **Tauri 桌面壳**：支持 macOS / Windows 原生桌面应用，Go 后端作为 sidecar 运行，窗口管理、菜单栏、优雅退出
- **SSE 实时进度**：批量检查和规则同步通过全屏进度弹窗展示实时进度，支持终止操作
- **深浅色模式**：自动跟随系统配色，CSS 变量驱动，所有组件适配
- **下载器集成**：支持 Neat Download Manager（内建 WebSocket），或自定义命令行下载器
- **外部链接弹窗**：桌面环境下点击官网 / 非文件链接时弹窗确认，支持复制链接或在浏览器打开
- **Token 迁移**：GitHub Token 移至 `[access]` 节
- **Tracker 排序**：支持 `order` 字段，数字小的在前
- **前端全面动画**：弹窗渐入渐出、视图切换过渡、toast 背景闪烁、悬停浮窗动画
- **GitHub Actions 构建**：自动构建 Desktop + Server 产物
- **深浅色图标适配**：浅色模式下自动翻转暗色图标
- **下载链接白名单**：文件类型自动识别，走下载器；非文件弹出确认后浏览器打开
- **终止功能**：支持终止正在运行的 SSE 任务
- **配置节拆分**：`[serein]`、`[tracker]`、`[download]`、`[access]` 独立配置

### Changed

- 前端全页面切换加入过渡动画
- 弹窗按钮统一为左右等宽大按钮
- 一键复制链接功能
- 列表切换时不再闪烁
- 搜索规则移入侧栏，规则页默认展示搜索
- 重要操作加入二次确认弹窗
- 规则列表按名称排序
- 悬浮提示优先单行展示
- 图标系统重构，未知平台不再显示多余 tooltip

### Fixed

- 修正前端一直显示默认 `_serein` Tracker 的问题
- 修正悬浮窗文字太窄反复换行
- 修正全选误选大量内容
- 修正长 toast 贴左侧边缘
- 修正规则源重复时版本跳过逻辑丢失
- 修正网络阻断时同步仍显示成功

### Removed

- REPL 交互终端
- 旧的同步 API 返回格式

## [v0.2.5] - 2025-05-21

### Added

- 规则同步支持并发拉取，复用现有 concurrency 配置，默认 8。
- `_source.json` 新增 `version` 字段，同步时未变更版本的源会直接跳过。
- `source_id` 加入字符校验，仅允许大小写字母、数字、下划线和连字符。
- list 模式子源的同步状态现在独立展示，不再统一计父源。

## [v0.2.4] - 2025-05-20

### Added

- 版本号与下载链接支持独立配置请求地址和解析器。
- 新增直通模式 `direct`，允许直接指定版本号或下载链接字符串。`d_url` 支持 `{version}` 占位符，自动替换为当前提取到的版本号。
- 优化前端版本展示，现在不同平台的相同版本会合并展示。

### Fixed

- 修正前端检查单个应用时，不会弹出错误 toast 的问题。
- 修正前端遇到 403 错误中返回的 HTML 代码而导致的错位问题。
- 修正用于显示提示的窗口在内容过长时会超出窗口的问题。
