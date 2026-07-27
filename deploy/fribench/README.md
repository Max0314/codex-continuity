# fribench 临时部署

此部署专用于 `fribench`：

- 容器后端：`127.0.0.1:24001`
- 临时公网入口：`http://1.14.72.50:24001`
- UFW 接口：`eth0`
- 公网持续时间：48 小时
- 到期动作：删除 UFW 规则并移除临时 Nginx 站点

容器继续在回环地址运行，临时入口关闭后可通过再次执行 `open` 重新开放。

发布目录使用
`/srv/fribench/apps/internal/codex-continuity/releases/<YYYYMMDD-HHMMSS>`，
共享数据、下载包和 `.env` 保存在
`/srv/fribench/apps/internal/codex-continuity/shared`。`activate-release.sh`
负责校验发布编号、解压前端、首次生成随机管理员密码并原子切换 `current` 链接。

首次安装需要管理员执行一次：

```bash
sudo bash /tmp/codex-continuity-root/install-root.sh
```

之后 `cpl` 用户只能免密运行固定的 Compose 文件和三个入口控制命令：

```bash
sudo docker compose \
  -f /srv/fribench/apps/internal/codex-continuity/current/compose.yaml \
  up -d --build
sudo /usr/local/sbin/fribench-codex-continuity open
sudo /usr/local/sbin/fribench-codex-continuity status
sudo /usr/local/sbin/fribench-codex-continuity close
```

首次登录账号为 `admin@fribench.local`。管理员密码只保存在服务器权限为 `0600`
的 `.env` 中，可在服务器终端按需读取，不写入仓库或部署日志。

这是临时 HTTP 入口，不用于长期或正式商用环境。
