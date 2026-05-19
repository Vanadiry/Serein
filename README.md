# Serein

软件更新追踪器。

与其他软件更新器不同的是，Serein 不提供中心化的检查更新接口。  
所有更新规则依赖用户贡献的规则表，直接从对应软件官方源获取版本信息。  
规则表去中心化分发，任何人都可独立维护和分发，无需信任中心化服务。

内置的规则表：[Vanadiry/SereinRulesList](https://github.com/Vanadiry/SereinRulesList)

<img src="/docs/image/readme-01.png" width="500"/>
<img src="/docs/image/readme-02.png" width="500"/>

## 快速开始

下载对应系统的二进制文件后运行即可，macOS/Linux 请先赋予可执行权限。  
运行时，请保持终端窗口打开。

<details><summary>macOS</summary>

1. 打开终端，输入 `chmod +x`，然后将二进制文件拖入终端，点击回车。
2. 再将二进制文件拖入终端，点击回车。（若提示“移动到废纸篓”，继续第三步）
3. 在终端输入 `xattr -cr`，然后将二进制文件拖入终端，点击回车。
4. 再执行一遍第二步，程序应当启动。

</details>

<details><summary>Linux</summary>

1. 打开终端，输入 `chmod +x`，然后将二进制文件拖入终端，点击回车。
2. 再将二进制文件拖入终端，点击回车。

</details>

<details><summary>Windows</summary>

直接运行 exe 即可。如果被 Defender 拦截，需要手动放行。

</details>
<br />

程序会在本地启动 Go 后端，并自动拉起前端。  
如果 WebUI 没有自动打开，请访问 `http://127.0.0.1:12510`。

首次启动，请按照 WebUI 首屏的教程，更新一下默认规则源。

详细使用方法，请阅读 [docs/guide](/docs/guide.md)。
