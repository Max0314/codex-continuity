# Codex Continuity 桌面端

Windows 桌面客户端采用 Tauri 2 + React + TypeScript，复用仓库中的 Go 客户端作为 sidecar 核心。

## 用户能力

- 可视化填写服务端地址、设备名称、固定工作区根目录、API 令牌和共享加密密钥；
- 一键连通测试与 64 KB 加密上传测试；
- 扫描固定根目录下的多个 Git 工作区和关联的 Codex 会话；
- 后台自动同步、失败持久化队列与手动立即同步；
- 会话搜索、状态筛选、加密归档导入导出与跨设备续接；
- 系统托盘右键快捷窗口；
- `Ctrl+Alt+P` 全局快捷同步；
- 开机后台启动、单实例、窗口状态记忆；
- API 令牌和共享密钥保存到 Windows 凭据管理器。

## 本地开发

从仓库根目录运行：

```powershell
.\scripts\dev-desktop.ps1
```

脚本会先构建 Go sidecar，再启动 Vite 与 Tauri。开发机需要：

- Go 1.26 或更高版本；
- Node.js 24 或更高版本；
- Rust stable；
- Visual Studio 2022 Build Tools，包含 Desktop development with C++。

只验证 React 界面：

```powershell
Set-Location .\desktop
npm ci
npm run dev
```

浏览器模式使用本地模拟数据，不会写入系统凭据。

## 构建安装包

```powershell
.\scripts\build-desktop.ps1
```

产物复制到仓库的 `release/`：

- `codex-continuity_<版本>_windows-x64-setup.exe`：标准安装版。优先使用 Windows 已有的 WebView2；缺少时由安装程序从微软官方获取；
- `codex-continuity_<版本>_windows-x64-offline-setup.exe`：完整离线版，内置 WebView2 Evergreen Runtime；
- `codex-continuity_<版本>_windows-x64-portable.zip`：包含桌面程序和 Go sidecar 的便携包；
- `desktop-release.json`：供服务端下载页读取的版本、体积、下载地址和 SHA-256 清单。

默认一次构建全部版本，也可以单独构建：

```powershell
.\scripts\build-desktop.ps1 -PackageMode Standard
.\scripts\build-desktop.ps1 -PackageMode Offline
```

正式对外分发前必须配置 Windows 代码签名。升级通道、灰度发布与证书轮换属于商业发布阶段，不应使用未签名安装包直接大范围分发。
