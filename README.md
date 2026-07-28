# Codex Continuity

在两台 Windows 电脑之间自动同步固定工作区根目录关联的 Codex 会话，并提供可靠的跨设备续接入口。

当前实现采用一个仓库、两个部署单元：

- `Codex Continuity`：运行在办公室电脑和公司电脑上的 Tauri 桌面客户端，内置 Go 同步核心；
- `continuity-server`：运行在个人 Linux 云服务器上的服务端，随 Docker Compose 部署，并提供管理网页。

服务端不调用 Codex 或 OpenAI，也不要求海外网络。会话快照在客户端使用 AES-256-GCM 分块加密，服务端只保存密文。

## 已实现

- 用户名/密码注册、登录、恢复密钥重置密码和用户隔离；
- 短期访问令牌、轮换刷新令牌与旧 API 令牌原地迁移（迁移后自动撤销旧令牌）；
- 基于账号与主物理网卡 MAC 摘要的稳定设备 ID，设备名可独立修改；
- 整个固定工作区根目录统一扫描，无需逐项目操作；
- Git 分支、提交、变更文件和有限补丁采集；
- 与工作区根目录相关的 Codex 原始会话只读快照；
- 变化检测、后台自动同步、失败持久化队列与恢复重试；
- 会话列表、搜索、筛选、导入导出与跨设备续接；
- `bi_center` 风格管理端；
- `bi_center` 风格 Windows 桌面主窗口；
- 系统托盘右键快捷窗口、开机后台启动和单实例；
- 图形化配置、连通测试与加密上传测试；
- Windows 凭据管理器保护本机登录令牌和账号同步密钥；
- 蓝色、青绿、紫色主题切换；
- Windows NSIS/MSI/便携版和 Linux 服务端构建；
- Docker Compose 服务端部署。

## 快速启动服务端

服务器可以联网拉取构建依赖时：

```powershell
Copy-Item .env.example .env
# 编辑 .env，至少替换 CONTINUITY_ADMIN_PASSWORD
docker compose up -d --build
```

浏览器打开 `http://服务器地址:8787`。

服务器没有海外网络时，不要在服务器构建。先在有网络的构建机或 GitHub Actions 生成
`codex-continuity-image-linux-amd64.tar.gz`，上传后执行：

```bash
gzip -dc codex-continuity-image-linux-amd64.tar.gz | docker load
docker compose up -d --no-build
```

此后服务运行不访问 Codex/OpenAI 或外部 CDN。

## 启动桌面客户端

普通用户无需打开 PowerShell：下载安装程序后，从开始菜单打开“Codex Continuity”，注册或登录同步账号，
随后在“设置”页确认设备名称和工作区根目录。

开发环境从仓库根目录运行：

```powershell
.\scripts\dev-desktop.ps1
```

构建 Windows 安装包：

```powershell
.\scripts\build-desktop.ps1
```

详细说明见 [桌面客户端](docs/desktop-client.md)。

## 构建服务端与兼容命令行核心

Windows PowerShell：

```powershell
.\scripts\build.ps1
```

发布物会写入 `release/`：

- `continuity-windows-amd64.exe`
- `continuity-server-linux-amd64`
- `SHA256SUMS.txt`

有 Docker 的构建机还可以执行 `.\scripts\export-image.ps1`，生成供离线服务器导入的镜像包。

## 图形客户端基本流程

1. 第一台电脑注册用户名和密码，并离线保存首次显示的恢复密钥；
2. 在“设置”确认 `D:\code_CPL` 和设备名称；
3. 第二台电脑登录同一账号，客户端自动解锁账号同步密钥；
4. 两台电脑分别完成“连接测试”和“加密上传测试”；
5. 保持自动同步开启；到另一台电脑后在“会话”页选择目标任务并点击“在此设备继续”。

客户端默认扫描整个 `D:\code_CPL`，无需逐项目打开 Codex 任务，也不需要 `@MCP`。兼容 CLI 仍保留给脚本和自动化使用。

## 重要边界

本工具不会覆盖目标电脑的 Codex 内部数据库，也不会承诺把运行中的原任务 ID 原样迁移。它保存会话变化后的加密快照，并在另一台电脑生成 `HANDOFF.md` 作为可靠继续入口。

详细资料见 [docs/README.md](docs/README.md)。
