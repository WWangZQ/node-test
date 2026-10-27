节点测评 v1.4.0：应用内更新、备用测速源与 IP 分类修复。

## 更新内容

- 启动后自动检查新版，点击下载并重启完成升级。
- Cloudflare 连接失败时依次尝试 OVH、Vultr SG，显示实际测速来源。
- 分开显示地址用途与 ISP、Fixed ISP、Wireless 等接入信息。
- 修复 IP 查询失败问题，完善 API 配置错误提示。

## 安装

- [Apple Silicon Mac](https://github.com/WWangZQ/node-test/releases/download/v1.4.0/NodeTest-1.4.0-macos-arm64.dmg)
- [Intel Mac](https://github.com/WWangZQ/node-test/releases/download/v1.4.0/NodeTest-1.4.0-macos-x86_64.dmg)
- [Windows 10 / 11 x64](https://github.com/WWangZQ/node-test/releases/download/v1.4.0/NodeTest-1.4.0-windows-x64-Setup.exe)

Mac 打开 DMG 后将应用拖入 Applications；Windows 双击安装。1.3.x 用户先安装这次更新，之后可在应用内升级。测试记录与设置继续保留。

Windows 另附 Portable.zip 便携版。完整源码与依赖见 source-with-dependencies.tar.gz，文件校验见 SHA256SUMS.txt。
