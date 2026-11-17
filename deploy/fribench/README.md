# fribench 部署

此部署专用于 `fribench`：

- 容器后端：`127.0.0.1:24001`
- 正式公网入口：`https://continuity.fribench.cn`
- TLS 与反向代理：fribench 域名 Nginx 站点
- Cookie：仅通过 HTTPS 传输

容器只在回环地址运行，不直接暴露公网端口。仓库中的临时入口控制命令仅保留作
应急诊断用途，不是正式客户端入口。

发布目录使用
`/srv/fribench/tools/codex-continuity/releases/<YYYYMMDD-HHMMSS>`，
共享数据、下载包和 `.env` 保存在
`/srv/fribench/tools/codex-continuity/shared`。`activate-release.sh`
负责校验发布编号、解压前端、首次生成随机管理员密码并原子切换 `current` 链接。

首次安装需要管理员执行一次：

```bash
sudo bash /tmp/codex-continuity-root/install-root.sh
```

之后 `cpl` 用户只能免密运行固定的 Compose 文件。临时入口控制命令只用于
管理员明确授权的应急诊断：

```bash
sudo docker compose \
  -f /srv/fribench/tools/codex-continuity/current/compose.yaml \
  up -d --build
sudo /usr/local/sbin/fribench-codex-continuity open
sudo /usr/local/sbin/fribench-codex-continuity status
sudo /usr/local/sbin/fribench-codex-continuity close
```

首次登录账号为 `admin@fribench.local`。管理员密码只保存在服务器权限为 `0600`
的 `.env` 中，可在服务器终端按需读取，不写入仓库或部署日志。

桌面客户端和网页统一使用正式 HTTPS 域名，不再使用公网 IP 与 `24001` 端口。
