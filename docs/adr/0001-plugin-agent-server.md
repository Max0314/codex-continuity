# ADR-0001：采用 Plugin + Agent + Server（已被取代）

状态：Superseded by ADR-0002
日期：2026-07-27

## 背景

需要在两台本地 Codex 电脑之间连续工作。个人服务器不能访问 Codex/OpenAI，因此不能作为远程 Codex 执行主机。

需要决定：

- 只做 Codex Plugin；
- 只做 MCP 服务；
- 做 Plugin + 存储服务；
- 做 Plugin + 本地 Agent + 存储服务。

## 决策

采用：

```text
Codex Plugin
  + lifecycle Hooks
  + Skills
  + 内置 MCP 适配器
  + 独立本地后台 Agent
  + 无 OpenAI 依赖的存储服务
```

对用户交付为两套：

1. 客户端安装包；
2. 服务端部署包。

## 理由

### Plugin

提供一致安装、Skill、Hook、MCP 配置和版本升级。

### MCP

为 Codex提供结构化的发布、查询和恢复工具，但它不是后台同步器。

### Agent

负责 Hook 之外的长任务：加密、上传、下载、重试、托盘和本地队列。

### Server

负责密文存储、设备、版本、lease 和审计，不需要访问 OpenAI。

## 被否决方案

### 只同步 `.codex`

包含凭据、缓存、机器路径和非稳定状态；并发写入风险高。

### 只做 MCP

MCP 依赖 Codex主动调用，无法在 Codex关闭时继续上传或处理断网队列，也不适合系统托盘和开机启动。

### 只做 Plugin Hook

Hook 不适合长时间网络任务；异步 command Hook 当前不可用，`SessionEnd` 时间也很短。

### 服务器运行 Codex

个人服务器没有可用的 Codex/OpenAI 网络，不满足执行主机要求。

## 后果

正面：

- 服务端部署简单；
- 不依赖海外网络；
- 客户端可离线工作；
- Plugin 与 CLI 都可使用；
- 安全边界清晰。

代价：

- 需要维护 Windows Agent；
- 不能保证原 thread ID 跨机恢复；
- 首次安装需要信任 Hook；
- 需要处理设备密钥和 lease。
