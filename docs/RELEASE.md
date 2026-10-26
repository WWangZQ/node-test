节点测评 v1.3.1：导入整个订阅，批量测速、检测服务可用性与 IP 质量，以彩色表格展示结果。

## 下载

- Apple Silicon Mac：选择 macos-arm64.zip，解压后打开节点测评.app。
- Intel Mac：选择 macos-x86_64.zip，解压后打开节点测评.app。
- Windows 10 / 11 x64：选择 windows-x64.zip，完整解压后打开 NodeTest.exe。

首次打开遇到系统拦截时：Mac 在系统设置 → 隐私与安全性中选择仍要打开；Windows 在提示窗口中选择更多信息 → 仍要运行。

## 功能

- 导入订阅链接或本地文件，自动遍历所有节点。
- 默认检测 16 个常用服务，支持按类别选择更多项目。
- 汇总多个来源的网络类型、代理识别、滥用记录和原始评分。
- 导出 PNG 长图、CSV、JSON 和 HTML。

[使用说明](https://github.com/WWangZQ/node-test#使用) · [检测规则](https://github.com/WWangZQ/node-test/blob/main/docs/DETECTION.md)

source-with-dependencies.tar.gz 包含项目源码和 Go 依赖源码，SHA256SUMS.txt 用于校验下载文件。项目及组件许可证见 [开源说明](https://github.com/WWangZQ/node-test/blob/main/THIRD_PARTY_NOTICES.md)。
