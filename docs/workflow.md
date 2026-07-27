# 日常工作流程

## 一次性配置

### 服务器

1. 使用 Docker Compose 部署服务；
2. 配置 HTTPS 或私有 VPN；
3. 管理员登录管理网页；
4. 为办公室电脑和公司电脑分别创建 API 令牌。

### 办公室电脑

```powershell
continuity init `
  --server https://continuity.example.com `
  --token ct_办公室令牌 `
  --root D:\code_CPL `
  --device 办公室电脑
```

保存首次显示的加密密钥。

### 公司电脑

```powershell
continuity init `
  --server https://continuity.example.com `
  --token ct_公司令牌 `
  --root D:\code_CPL `
  --device 公司电脑 `
  --key 办公室电脑生成的加密密钥
```

## 离开一台电脑前

在 PowerShell 执行一次：

```powershell
continuity publish --target 公司电脑
```

这条命令会：

1. 扫描 `D:\code_CPL` 下所有 Git 项目；
2. 记录分支、提交、dirty 状态、变更文件和有限补丁；
3. 找出工作目录位于该根目录下的相关 Codex 会话；
4. 按扫描时的文件长度保存会话只读快照；
5. 生成 `HANDOFF.md` 和 `manifest.json`；
6. 压缩并在本机加密；
7. 上传密文到个人服务器。

不需要：

- 逐项目打开 Codex；
- 逐对话发送“发布交接”；
- 在 Codex 中 `@MCP`；
- 把源代码仓库完整上传到个人服务器。

## 到另一台电脑后

先同步 GitHub/GitLab 中的正常代码：

```powershell
continuity list
continuity takeover
```

接管完成后，客户端会输出一个本地 `HANDOFF.md` 路径。进入需要继续的项目，在 Codex 中发送：

```text
请读取交接目录中的 HANDOFF.md 和 manifest.json，核对当前 Git 状态；
需要时查看 projects 下补丁和 sessions 下相关只读会话快照；
总结上一台电脑做到哪里，然后从未完成事项继续。不要覆盖现有未提交修改。
```

## 后续托盘版目标

当前 MVP 是 CLI。下一阶段可以增加：

- 系统托盘“发布整机交接”；
- 退出 Codex 后自动快速快照；
- 断网队列和恢复后补传；
- 另一台电脑发现新交接时通知；
- 冲突时才要求用户确认。

届时日常操作可以缩减为一次托盘点击；自动化仍由同一个客户端核心完成，不需要改变服务端。
