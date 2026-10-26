首个公开发行版，保留 1.3 的检测规则与彩色表格，加入独立 Windows 应用及分架构 Mac 构建。

- Mac：Apple Silicon 选择 arm64；Intel 选择 x86_64。完整解压后打开节点测评.app。
- Windows 10/11 x64：完整解压 windows-x64.zip，打开目录内 NodeTest.exe。
- 导入订阅 → 开始测试 → 按分类查看 → 导出 PNG / CSV / JSON / HTML。
- IP 数据源设置支持个人 API 密钥；没有密钥时显示未配置。

发布包未做 Apple 公证或 Windows 发布者签名，系统可能提示确认来源。SHA256SUMS.txt 用于核对下载文件完整性。

验证：Go 判定测试、JS 聚合及调度测试、本地代理真实下载、两个不可达节点失败语义；Windows 包装后的应用自动验证表格更新和四种导出。Mac 编译目标为 macOS 13.0，未在 macOS 13 真机执行兼容性测试。Windows 验证在 GitHub 托管 Windows runner 完成，实际网络 / VPN 环境仍需使用者实测。

所有示例均为合成数据。没有包含个人订阅、真实报告或 API 密钥。接口规则会随第三方服务变化，判定范围见仓库检测说明。

源码与 Go 依赖打包在 source-with-dependencies.tar.gz，AGPL-3.0 及各组件许可见仓库 THIRD_PARTY_NOTICES.md。
