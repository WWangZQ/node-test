# 从源码构建

## 环境

- Go 1.24+、Python 3.11+。
- macOS：Xcode Command Line Tools。
- Windows：Node.js 24+。

依赖版本由 backend/go.mod、go.sum 和 package-lock.json 固定。构建产物统一写入 dist。

## macOS

```sh
python3 scripts/build-macos.py --arch arm64
# Intel
python3 scripts/build-macos.py --arch x86_64
```

输出为 `dist/macos-<arch>/节点测评.app`。原生窗口使用 Swift / WebKit，编译目标为 macOS 13.0。构建脚本使用 ad-hoc 签名。

## Windows

在 PowerShell 中运行：

```powershell
New-Item -ItemType Directory -Force bin
cd backend
go build -trimpath -o ../bin/node-worker.exe ./cmd/local-worker
cd ..
npm ci
npm run package:windows
```

输出为 `dist/NodeTest-win32-x64/`。窗口使用 Electron，检测引擎为 Go worker。开发时运行 `npm start`。

## 测试与发布

Go 测试覆盖服务判定、风险字段、缓存和缺失值处理；JS 测试覆盖来源计数和任务调度。本地代理夹具验证实际下载、订阅解析和连接失败处理。Windows 打包测试调用实际 worker，检查表格更新、系统加密存储以及四种导出。

GitHub Actions 在 macos-15 和 windows-latest 环境构建应用。推送 `v*` 标签后，通过检查的应用包与完整依赖源码包会上传到 Release，并生成 SHA256SUMS.txt。

完整源码包可通过以下命令生成：

```sh
cd backend
go mod vendor
cd ..
python3 scripts/source-archive.py
```
