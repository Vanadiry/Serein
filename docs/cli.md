# Serein CLI

## 命令

`serein`：启动后端和 Web 服务，并进入 WebUI。
`serein cli`：启动后端和 Web 服务，并进入 REPL 交互终端。

## REPL

### 检查更新

```bash
>>> check all                       # 检查全部追踪软件
>>> check app <id> [<id> ...]       # 检查指定软件
>>> check tracker <id>              # 检查指定 tracker
>>> check confirm <id> <version>    # 写入版本（所有平台）
>>> check confirm <id> macos:1.0    # 写入版本（区分平台）
```

`-temp` 参数查看最近检查结果：

```bash
>>> check all -temp                 # 查看最近全量检查结果
>>> check tracker <id> -temp        # 查看最近 tracker 检查结果
```

### AUTO 模式

检查后逐项交互确认，适合批量处理。

```bash
>>> check all -auto
>>> check app <id> -auto
>>> check tracker <id> -auto
```

| 输入 | 效果 |
| ---- | ---- |
| `ok` | 确认所有平台 |
| `ok [platform] <platform> ...` | 确认指定平台，其余 wait |
| Enter | 全部 wait，跳过此项 |
| `skip` | 此项和后续全部 wait |
| `stop` | 停止 AUTO，后续全部 wait |

AUTO 结束后会汇总 wait 列表并输出。

### 规则管理

```bash
>>> rule sync                       # 同步规则源
>>> rule list                       # 列出所有规则源
>>> rule list <source_id>           # 列出指定源的规则
>>> rule list -s <query>            # 搜索规则
```

### 追踪管理

```bash
>>> tracker add <id> [-platform os]...  # 添加到 _serein
>>> tracker new <id>                    # 新建 tracker
>>> tracker list                        # 列出全部追踪
>>> tracker list <id>                   # 列出 tracker 详情
```

### 其他

```bash
>>> help      # 显示帮助
>>> exit      # 退出
```
