# 从源码构建

## 环境

- Go 1.24+、Python 3.11+。
- macOS：Xcode Command Line Tools。
- Windows：Node.js 24+。

依赖版本由 backend/go.mod、go.sum 和 package-lock.json 固定。构建产物统一写入 dist。

## macOS

```sh
python3 scripts/build-macos.py --arch arm64
python3 scripts/package-macos.py --arch arm64
# Intel
python3 scripts/build-macos.py --arch x86_64
python3 scripts/package-macos.py --arch x86_64
```

输出为 `dist/macos-<arch>/节点测评.app`。原生窗口使用 Swift / WebKit，编译目标为 macOS 13.0。构建脚本使用 ad-hoc 签名。打包脚本生成 DMG，挂载后校验版本、签名和 Applications 入口。

## Windows

在 PowerShell 中运行：

```powershell
New-Item -ItemType Directory -Force bin
cd backend
go build -trimpath -o ../bin/node-worker.exe ./cmd/local-worker
go build -trimpath -o ../bin/node-updater.exe ./cmd/local-updater
cd ..
npm ci
npm run package:windows
./scripts/build-windows-installer.ps1
```

输出为 `dist/NodeTest-win32-x64/`。窗口使用 Electron，检测引擎为 Go worker。安装程序位于 `dist/NodeTest-<version>-windows-x64-Setup.exe`，使用 Inno Setup 7.1.0 构建。脚本下载固定版本编译器并核对 SHA256。开发时运行 `npm start`。

## 测试与发布

Go 测试覆盖服务判定、风险字段、缓存和缺失值处理；JS 测试覆盖来源计数和任务调度。本地代理夹具验证实际下载、订阅解析和连接失败处理。Windows 打包测试调用实际 worker，检查表格更新、系统加密存储以及四种导出。安装程序测试覆盖安装、快捷方式、应用启动与卸载。

GitHub Actions 在 macos-15 和 windows-latest 环境构建应用。推送 `v*` 标签后，通过检查的应用包与完整依赖源码包会上传到 Release，并生成 SHA256SUMS.txt。

完整源码包可通过以下命令生成：

```sh
cd backend
go mod vendor
cd ..
python3 scripts/source-archive.py
```

## 应用内更新

更新助手读取本仓库最新正式 Release，按系统架构选择安装包，并核对 GitHub 提供的 SHA256 摘要和文件长度。Mac 在同一文件系统暂存新版，验证应用身份、版本与代码完整性后替换；替换失败时恢复原应用。Windows 调用 Inno Setup 升级安装并重启应用。

发布流程继续使用现有安装包命名和正式版本标签，后续版本自动进入更新通道。
