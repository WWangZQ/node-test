# 节点测评 · Node Test

本地全订阅测速、服务可用性与多来源 IP 画像。导入一个订阅，应用按顺序测试所有节点，并生成可对照的彩色表格。支持 macOS 和 Windows，无需安装 Codex、Go、Python 或其他代理客户端。

[下载发行版](https://github.com/WWangZQ/node-test/releases/latest) · [检测说明](docs/DETECTION.md) · [开源与第三方许可](THIRD_PARTY_NOTICES.md)

![界面预览，数据为合成示例](docs/preview.png)

## 下载与启动

| 系统 | 文件 | 启动方式 |
|---|---|---|
| Apple Silicon Mac，macOS 13+ | NodeTest-1.3.1-macos-arm64.zip | 解压，把节点测评.app 拖到应用程序，再双击 |
| Intel Mac，macOS 13+ | NodeTest-1.3.1-macos-x86_64.zip | 同上 |
| Windows 10 / 11，64 位 | NodeTest-1.3.1-windows-x64.zip | 完整解压到一个目录，双击 NodeTest.exe |

macOS 使用原生 Swift / WebKit 窗口，Windows 使用 Electron 窗口，两者调用同一 Go 检测引擎和同一套表格界面。应用无需启动本地 HTTP 服务。Windows 目录里的资源、DLL 和 exe 需放在一起。

目前发布包没有付费开发者签名：Mac 包采用 ad-hoc 签名，未做 Apple 公证。首次打开可能需要在系统设置 → 隐私与安全性中允许打开。Windows 可能显示未知发布者提示。请核对本仓库发行页及 SHA256SUMS.txt；单位管理设备需遵循其安装策略。

macOS 最低版本在编译目标与 Info.plist 中均设为 13.0；兼容性验证范围见每次发布说明。不同系统的字体与细节会有差异。

## 使用

1. 粘贴订阅链接，或导入 UTF-8 本地订阅文件，支持 Clash / Mihomo YAML 与常见节点链接订阅（含 Base64）。文件最大 5 MB。
2. 选择速度、服务检测或两者；默认测试常用 16 项，可展开选择更多项目。选择会应用于整个订阅。
3. 点击开始测试。应用逐个调用节点，无需手动切换当前代理。测试期间可停止，已完成结果保留。
4. 按分类查看结果，点击节点查看检测依据。可保存 PNG 长图、CSV、JSON 或独立 HTML。

每节点测速默认最多下载 64 MB。100 个节点一次完整下载测试最多约 6.4 GB，另有服务和 IP 查询流量；仅服务检测可避开大文件下载。测速会占用本机带宽。

系统 TUN、VPN 和路由仍可能影响底层连接。Mac 可选择物理网卡；Windows 默认跟随系统路由，可选择可用网卡。独立 worker 不会接管系统代理设置。

常用项目：OpenAI、Gemini、Claude、Grok、Perplexity、Copilot、Meta AI；Disney+、Netflix、YouTube、Spotify、Apple TV+ 网页、TikTok；Steam、Wikipedia；Google。Google 和 Gemini 独立探测。

## IP 数据与凭据

接入 IPPure、ProxyCheck、NCGY、ipapi.is、Scamalytics、AbuseIPDB、IPQualityScore。前三项可在接口允许时匿名查询，其余项目需在 IP 数据源设置中填写自己申请的 API 密钥。无密钥、额度不足、限流、超时均会独立显示，不会转为零风险。

网络类型按来源多数显示，评估采用最保守类型；冲突标黄并显示住宅(3)、机房(2)等独立计数。代理识别、滥用记录和各家原始评分分列保留。0 分不代表安全，不会覆盖机房或代理警告。

订阅和节点凭据仅在运行内存中处理；订阅链接导入成功后从输入框清除。历史报告保存在本机，包含节点名称、出口 IP、测试结果和证据，分享报告前请自行检查。API 密钥在 Mac 存入系统钥匙串，在 Windows 使用 DPAPI 加密后保存。应用没有遥测和上传报告功能。检测会向目标服务和数据提供方发起网络请求。

公开版的数据目录独立于早期自用版：Mac 为 `~/Library/Application Support/NodeTestPublic`，Windows 为 `%APPDATA%\NodeTestPublic`。两者启动时均需重新导入订阅。应用菜单可打开结果文件夹。

## 从源码构建

需要 Go 1.24+、Python 3.11+。Windows 外壳还需要 Node.js 24+。依赖版本固定在 go.mod / go.sum 和 package-lock.json。所有构建输出到 dist，不会安装或覆盖现有应用。

macOS 需 Xcode Command Line Tools：

```sh
python3 scripts/build-macos.py --arch arm64
# Intel:
python3 scripts/build-macos.py --arch x86_64
```

Windows PowerShell：

```powershell
New-Item -ItemType Directory -Force bin
cd backend
go build -trimpath -o ../bin/node-worker.exe ./cmd/local-worker
cd ..
npm ci
npm run package:windows
```

输出位于 `dist/NodeTest-win32-x64/`。开发时可运行 `npm start`。

GitHub Actions 自动验证并生成两个 Mac ZIP 和 Windows ZIP。推送 `v*` 标签后，全部构建和测试成功才会发布 Release，并附完整源码、Go 依赖源码和 SHA256 校验文件。复用到自己的仓库时可直接使用这套流程。

## 验证与边界

- Go 测试验证服务判定、风险字段、缓存与缺失值处理；JS 测试验证多来源计数和任务调度。
- Mac 本地代理夹具覆盖实际下载、订阅解析和不可达节点；Windows 桌面冒烟测试调用实际 worker，验证两个不可达节点均失败、表格更新及四种导出。
- 第三方网页和接口会变化，检测表示当次请求取得的网络与地区证据。登录、付费播放、模型调用和账号安全需要相应业务验证。CF 验证仅说明到达验证页。
- 需要 API 密钥的来源使用本地响应夹具验证，真实账号接入仍取决于使用者的密钥和套餐。

## 开源

整体发行版采用 **AGPL-3.0-only**；上游 GPL-3.0 与其他组件保留原许可。完整来源、修改说明和版权文件见 [THIRD_PARTY_NOTICES.md](THIRD_PARTY_NOTICES.md) 与 LICENSES。Release 中的 `source-with-dependencies` 压缩包提供与版本对应的源码和 Go 依赖。

界面使用系统字体，本项目独立开发，与 Anthropic 等服务提供方无隶属关系。
