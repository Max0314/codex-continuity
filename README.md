# Codex Continuity（Codex 工作接力）

在两台 Windows 电脑之间安全交接整个工作根目录的 Git 状态与 Codex 会话快照。

当前实现采用一个仓库、两套发布物：

- `continuity-windows-amd64.exe`：运行在办公室电脑和公司电脑上的客户端；
- `continuity-server`：运行在个人 Linux 云服务器上的服务端，随 Docker Compose 部署，并提供管理网页。

服务端不调用 Codex 或 OpenAI，也不要求海外网络。交接内容在客户端使用 AES-256-GCM 分块加密，服务端只保存密文。

## 已实现

- 管理员/成员登录和用户隔离；
- 每台设备独立 API 令牌；
- 设备注册和在线状态；
- 整个固定工作根目录一次发布；
- Git 分支、提交、变更文件和有限补丁采集；
- 与工作根目录相关的 Codex 原始会话只读快照；
- 加密上传、列表、下载、解密与接管；
- `bi_center` 风格管理端；
- 蓝色、青绿、紫色主题切换；
- Windows 单文件客户端和 Linux 服务端构建；
- Docker Compose 服务端部署。

## 快速启动服务端

```powershell
Copy-Item .env.example .env
# 编辑 .env，至少替换 CONTINUITY_ADMIN_PASSWORD
docker compose up -d --build
```

浏览器打开 `http://服务器地址:8787`。

## 构建

Windows PowerShell：

```powershell
.\scripts\build.ps1
```

发布物会写入 `release/`：

- `continuity-windows-amd64.exe`
- `continuity-server-linux-amd64`
- `SHA256SUMS.txt`

## 客户端基本流程

先在管理网页创建“办公室电脑”的 API 令牌：

```powershell
.\continuity-windows-amd64.exe init `
  --server https://continuity.example.com `
  --token ct_xxx `
  --root D:\code_CPL `
  --device 办公室电脑
```

第一次会生成一份用户加密密钥。安全复制到第二台电脑，并在第二台执行：

```powershell
.\continuity-windows-amd64.exe init `
  --server https://continuity.example.com `
  --token ct_另一台设备令牌 `
  --root D:\code_CPL `
  --device 公司电脑 `
  --key 第一台电脑生成的密钥
```

发布和接管：

```powershell
continuity publish --target 公司电脑
continuity list
continuity takeover
```

`publish` 默认扫描整个 `D:\code_CPL`，无需逐项目打开 Codex 任务，也不需要 `@MCP`。

## 重要边界

本工具不会覆盖目标电脑的 Codex 内部数据库，也不会承诺把运行中的原任务 ID 原样迁移。它保存“发布瞬间”的工作状态，接管后生成 `HANDOFF.md` 作为新任务的可靠继续入口。发布后新产生的消息需要再次发布才能进入下一份快照。

详细资料见 [docs/README.md](docs/README.md)。
