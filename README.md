# Serein

规则去中心化的软件更新追踪器。

与其他软件更新器不同的是，Serein 不提供中心化的检查更新接口。  
所有更新规则依赖用户贡献的规则表，直接从对应软件官方源获取版本信息。  
规则表去中心化分发，任何人都可独立维护和分发，无需信任中心化服务。

内置的规则表：[Vanadiry/SereinRulesList](https://github.com/Vanadiry/SereinRulesList)

<img src="/docs/image/readme-01.png" width="500"/>
<img src="/docs/image/readme-02.png" width="500"/>

## 使用

请阅读 [docs/guide](/docs/guide.md) 来学习 Serein 的使用方法。

Serein 提供 Desktop 和 Server 两种运行方式。
二者功能和行为完全一样，只是 Desktop 版本多了 Tauri 壳，可以作为桌面应用。
Server 使用浏览器显示前端。

macOS 和 Windows 建议使用 Desktop。Linux 请使用 Server。
（~~Linux 没有 Desktop，因为我懒了QvQ，对不起呜呜。~~）  

### Serein Desktop

只需要下载对应平台的安装包，安装后即可运行。

对于 macOS，打开程序时可能会提示“未打开，无法验证...”，这是因为我没钱买一年 99 美元的苹果证书。  
安装好后，请打开终端，输入 `xattr -cr ` 后，将 Serein 程序拖入终端窗口，然后点击回车即可。

对于 Windows，首次打开安装程序或 Server 程序时会弹出 Defender 蓝窗口，这是因为我也没钱买微软的证书...  
只需要点击“更多信息”，然后点击“仍要运行”即可。

### Serein Server

对于 macOS 和 Linux，运行之前需要执行 `chmod +x`，然后通过 `./serein-server-xxx` 运行。  
运行时，请保持终端窗口打开。

程序启动后，会自动拉起浏览器。  
如果浏览器没有自动打开，请访问 `http://127.0.0.1:12510`。

## 构建

分发包已转向 GitHub Actions。从源码构建需要 `go`、`rust`、`pnpm`。

```bash
pnpm install
python3 scripts/build.py server    # Serein Server
python3 scripts/build.py desktop   # Serein Desktop
```

## 配置

程序首次启动时会在 `~/.vSoft/Serein/` 生成 `config.toml`。  
通过环境变量 `SEREIN_HOME` 可自定义主目录。

首次启动请前往规则页点击「同步规则」拉取内置规则表。
