# 部署指南

## 1. 服务端要求

- Linux x86_64 云服务器；
- Docker Engine 与 Docker Compose v2；
- 两台办公电脑能通过 HTTPS 或私有 VPN 访问服务器；
- 服务器本身不需要海外网络，不访问 Codex/OpenAI。

建议至少 1 核 CPU、512 MB 内存，并根据交接包数量准备磁盘。

## 2. Docker Compose 部署

### 2.1 服务器可以访问构建依赖

```bash
git clone <你的仓库地址> codex-continuity
cd codex-continuity
cp .env.example .env
```

修改 `.env`：

```dotenv
CONTINUITY_PORT=8787
CONTINUITY_PUBLIC_URL=https://continuity.example.com
CONTINUITY_ADMIN_EMAIL=admin@example.com
CONTINUITY_ADMIN_NAME=系统管理员
CONTINUITY_ADMIN_PASSWORD=一段足够长的随机密码
CONTINUITY_COOKIE_SECURE=true
CONTINUITY_MAX_UPLOAD_MIB=500
```

启动：

```bash
docker compose up -d --build
docker compose ps
docker compose logs -f continuity
```

健康检查：

```bash
curl http://127.0.0.1:8787/api/v1/health
```

### 2.2 服务器没有海外网络

推荐使用“联网构建、离线导入”，不要让个人服务器执行 `docker compose build`。

在有 Docker 和正常构建网络的 Windows 电脑：

```powershell
.\scripts\export-image.ps1
```

或者从 GitHub Release 下载：

```text
codex-continuity-image-linux-amd64.tar.gz
```

把以下内容上传到个人服务器：

```text
docker-compose.yml
.env
release/
codex-continuity-image-linux-amd64.tar.gz
```

服务器执行：

```bash
gzip -dc codex-continuity-image-linux-amd64.tar.gz | docker load
docker image inspect codex-continuity:local
docker compose up -d --no-build
```

更新时重复导入新镜像，再执行：

```bash
docker compose up -d --no-build --force-recreate
```

容器运行时只使用本机 SQLite、密文文件和静态网页，不访问海外网络。

## 3. HTTPS

生产环境应在容器前放置 Caddy、Nginx、Traefik 或公司现有网关：

```text
浏览器/客户端
  -> HTTPS :443
  -> 反向代理
  -> http://127.0.0.1:8787
```

如果服务器不便公开暴露，可以使用 Tailscale、WireGuard 或公司 VPN。不要把 8787 端口直接暴露到公网且长期使用 HTTP。

## 4. 管理员与同事

首次启动时，服务端只在数据库没有用户时创建环境变量指定的管理员。

管理员登录后：

1. 在“用户”页面为每位同事创建账号；
2. 同事登录自己的账号；
3. 每台电脑分别创建一个 API 令牌；
4. 每位用户使用自己的加密密钥初始化两台电脑。

用户之间的设备、令牌和交接记录相互隔离。管理员可以创建用户，但管理网页不会显示任何交接正文，因为服务器没有解密密钥。

## 5. 客户端下载

在 Windows 构建机执行桌面构建：

```powershell
.\scripts\build-desktop.ps1
```

将下列文件放在服务器仓库的 `release/` 目录，再重启 Compose：

```text
codex-continuity_0.3.1_windows-x64-setup.exe
codex-continuity_0.3.1_windows-x64-offline-setup.exe
codex-continuity_0.3.1_windows-x64-portable.zip
desktop-release.json
SHA256SUMS.txt
```

管理网页“桌面客户端”会读取 `/downloads/desktop-release.json`，展示真实版本、体积和下载地址。标准版在缺少 WebView2 时从微软官方获取运行时；完整离线版不依赖安装时联网。GitHub Release 工作流会自动构建这些 Windows 产物和 Linux 服务端离线镜像。

正式推广给同事前，必须配置组织代码签名证书和稳定更新通道。

## 6. 备份

Docker 命名卷 `continuity-data` 包含：

```text
continuity.db
continuity.db-wal
continuity.db-shm
blobs/*.ccx
```

备份时应同时备份 SQLite 和 `blobs/`。最稳妥做法是短暂停止容器，备份整个卷，再启动容器。
