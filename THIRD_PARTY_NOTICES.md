# 来源、修改与许可

节点测评公开发行版采用 AGPL-3.0-only，见根目录 LICENSE。2026-10-26 的公开版加入 Windows 应用外壳、系统字体、独立数据目录、分架构构建和自动验证。桌面外壳、UI、本地 worker 和聚合规则的修改源码随仓库提供。上游文件继续适用各自原有许可证及版权声明；整体发行许可不抹去上游权利。

| 项目 | 使用范围 | 来源版本 | 许可证 |
|---|---|---|---|
| Clash SpeedTest Pro | backend 保留的 Go 源码、工具与类型，本地 worker 所在基础项目 | 5d863847706169605a47f210ac822269c27602fc | GPL-3.0，LICENSES/GPL-3.0.txt |
| Mihomo | worker 直接链接的代理协议与网络适配器 | v1.19.10 | GPL-3.0，依赖源代码内保留 LICENSE |
| RegionRestrictionCheck | 服务检测规则和接口流程参考；适配实现集中于 worker 的 media.go、ai.go、services.go | ef841d0a2910a44a6ca565e3a03de0f2282d83b3 | AGPL-3.0，根目录 LICENSE |
| miaospeed | 服务探测规则参考，未嵌入其可执行文件 | 0234a01d9daa5248bc9cf1a7bd63937b7cde2c68 | AGPL-3.0，LICENSES/miaospeed.txt |
| unlock-checker | 解锁检测行为参考，未嵌入其运行时 | c964d8dfcc8316159200b52c3f3e18e2b832c864 | GPL-3.0，LICENSES/unlock-checker.txt |
| Electron | Windows 桌面运行时，包含 Chromium、Node.js 等组件 | package-lock.json 固定版本 | MIT 及随二进制提供的 LICENSE、LICENSES.chromium.html |
| Go 及间接模块 | worker 编译器与链接依赖 | backend/go.mod、go.sum | 各模块许可证随完整依赖源码包保留 |

上游链接：
- https://github.com/zhsama/clash-speedtest
- https://github.com/MetaCubeX/mihomo
- https://github.com/1-stream/RegionRestrictionCheck
- https://github.com/miaokobot/miaospeed
- https://github.com/shenguanqing/unlock-checker
- https://github.com/electron/electron

发布版基于 GPL-3.0 组件，并包含参考 AGPL-3.0 项目实现的检测规则，因此整体按 AGPL-3.0 提供；GPL-3.0 第 13 节允许相应组合。没有将参考规则当作闭源内容发布。上游作者版权归其各自所有。

Releases 提供与标签对应的完整项目源码及 Go 依赖源码压缩包，保留依赖目录中的许可证和版权文件。Electron ZIP 内保留上游运行时版权声明；Electron 精确版本源码可从 https://github.com/electron/electron/releases 获取，其第三方组件路径由 LICENSES.chromium.html 标注。Node.js、Chromium 与其他 Electron 运行时组件适用其各自许可证。

公开版只调用系统字体，未分发 Anthropic 字体、商标或标识。本项目为独立社区软件。

IP 数据与服务探测请求访问第三方服务，其接口配额、服务条款和数据权限由相应提供方决定。代码许可证不授予付费 API 访问资格。
