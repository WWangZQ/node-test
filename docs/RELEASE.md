节点测评 v1.3.2：提供 Mac DMG 和 Windows 安装程序。

## 下载与安装

- [Apple Silicon Mac](https://github.com/WWangZQ/node-test/releases/download/v1.3.2/NodeTest-1.3.2-macos-arm64.dmg)：双击 DMG，将节点测评拖入 Applications。
- [Intel Mac](https://github.com/WWangZQ/node-test/releases/download/v1.3.2/NodeTest-1.3.2-macos-x86_64.dmg)：双击 DMG，将节点测评拖入 Applications。
- [Windows 10 / 11 x64](https://github.com/WWangZQ/node-test/releases/download/v1.3.2/NodeTest-1.3.2-windows-x64-Setup.exe)：双击安装，通过桌面或开始菜单启动。

Windows 另附 Portable.zip 便携版，解压后运行 NodeTest.exe。

首次打开遇到系统拦截时：Mac 在系统设置 → 隐私与安全性中选择仍要打开；Windows 在提示窗口中选择更多信息 → 仍要运行。

## 功能

- 导入整个订阅，批量测速、检测服务可用性与 IP 质量。
- 默认检测 16 个常用服务，支持按类别选择更多项目。
- 汇总多个来源的网络类型、代理识别、滥用记录和原始评分。
- 以彩色表格展示结果，导出 PNG 长图、CSV、JSON 和 HTML。

[使用说明](https://github.com/WWangZQ/node-test#使用) · [检测规则](https://github.com/WWangZQ/node-test/blob/main/docs/DETECTION.md)

source-with-dependencies.tar.gz 包含项目源码和 Go 依赖源码，SHA256SUMS.txt 用于校验下载文件。项目及组件许可证见 [开源说明](https://github.com/WWangZQ/node-test/blob/main/THIRD_PARTY_NOTICES.md)。
