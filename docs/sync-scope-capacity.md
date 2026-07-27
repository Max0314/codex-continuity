# 同步范围、容量与带宽设计

更新时间：2026-07-27

## 现有问题与修复

v0.3.0 虽然把服务端上传上限设置成 512 MiB，但 HTTP 处理器使用 `ParseMultipartForm(16 MiB)`。大于 16 MiB 的 multipart 文件会写入容器 `/tmp`，而生产 Compose 只给 `/tmp` 分配了 64 MiB。结果是 219.26 MiB 的真实首次同步包在解析阶段失败，数据尚未写入持久化目录。

v0.3.1 改成逐 part 解析：

1. metadata 最多 8 MiB，在内存中解析；
2. blob 直接流式写入 `/data/blobs/<uuid>.ccx`；
3. 写入过程中计算实际字节数，超过 500 MiB 即停止并删除半成品；
4. SQLite 记录创建成功后，快照才对客户端可见；
5. 任意验证或数据库错误都会删除未完成 blob。

容器仍可保持只读根文件系统和 64 MiB `/tmp`，因为上传正文不再依赖临时目录。

## 服务端如何存储

生产环境数据目录：

```text
/srv/fribench/apps/internal/codex-continuity/shared/data/
  continuity.db
  continuity.db-wal
  continuity.db-shm
  blobs/
    <handoff-id>.ccx
```

- SQLite：用户、Web 会话、API 令牌摘要、设备和快照元数据；
- 文件系统：AES-256-GCM 加密的 `.ccx` 正文；
- 数据库只保存 blob 相对路径、大小、状态和 manifest；
- 服务端没有共享加密密钥，不能读取会话正文；
- 当前每次成功同步保存完整快照，尚未做跨版本去重。

## 客户端同步范围

默认策略：

| 配置 | 默认值 | 说明 |
|---|---:|---|
| 最近更新时间 | 7 天 | 可选 2、5、7 天或不限制 |
| 项目 | 全部 | 可取消勾选不需要的项目 |
| 已归档会话 | 关闭 | 开启后扫描 `~/.codex/archived_sessions` |
| 单会话上限 | 128 MiB | 防止单一异常文件占满快照 |
| 会话原始数据软上限 | 450 MiB | 为补丁、manifest 和 ZIP 开销留余量 |
| 最终密文包硬上限 | 500 MiB | 超过即不上传 |
| 单次上传超时 | 30 分钟 | 适配低带宽或跨公网传输 |

项目筛选使用工作区根目录下的相对路径，因此两台电脑绝对路径可以不同，但项目相对结构应一致。

## 带宽判断

当前服务器是腾讯云 CVM。腾讯云文档说明：固定带宽不高于 10 Mbps 时，实例公网入带宽上限为 10 Mbps；固定带宽高于 10 Mbps 时，入带宽上限与购买带宽一致。实际购买值仍需在腾讯云控制台查看 `InternetMaxBandwidthOut`，仅从服务器操作系统无法可靠判断套餐带宽。

按 10 Mbps 理论入带宽估算：

- 219 MiB：约 3.1 分钟；
- 500 MiB：约 7.0 分钟；
- 考虑 TCP、TLS、multipart、重传和带宽波动，实际时间会更长。

因此 500 MiB 技术上可行，但不适合频繁上传完整快照。默认一周、项目筛选和自动增量触发能减少日常流量；分片续传与内容寻址仍是商用稳定性的下一阶段。

参考：

- [腾讯云 CVM 使用限制总览](https://cloud.tencent.com/document/product/213/15379)
- [腾讯云调整实例带宽上限](https://cloud.tencent.com/document/product/213/15721)
