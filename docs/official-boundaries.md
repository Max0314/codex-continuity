# 官方能力边界

本文档记录设计依赖的 Codex 官方能力，以及本项目作出的工程推论。

## 已确认的官方能力

### Hooks

Codex Hooks 支持：

- `Stop`
- `SessionStart`
- `SessionEnd`
- 其他工具和生命周期事件

Hook 可以将聊天发送到自定义日志系统，也可以用于持久记忆和校验。

约束：

- 当前实际执行的是 command handler；
- 异步 command Hook 尚不支持；
- `SessionEnd` 默认时间很短，最大支持时间也有限；
- Hook 提供 `transcript_path`，但 transcript 格式不是稳定接口；
- 非托管 Hook 首次或发生变化后需要审查和信任。

来源：[Codex Hooks](https://learn.chatgpt.com/docs/hooks)

### Plugin

Codex Plugin 可以将 Skills、Hooks、MCP 配置、脚本和其他扩展能力作为一个安装单元交付。

来源：[Build plugins](https://developers.openai.com/plugins/build/plugins)

### MCP

Codex 本地客户端支持 stdio 和 Streamable HTTP MCP，并可以通过 MCP 使用结构化工具。

来源：[Model Context Protocol](https://learn.chatgpt.com/docs/extend/mcp)

### 本地状态

Codex 将本地配置和状态存储在 `CODEX_HOME`，默认是 `~/.codex`。本地 memory 也属于具体 Codex host，不是跨主机全局状态。

来源：[Configuration](https://learn.chatgpt.com/docs/config-file/config-basic)、[Memories](https://learn.chatgpt.com/docs/customization/memories)

### 远程连接

Codex 支持连接远程主机、继续远程主机上的对话，以及在符合条件的主机之间 Handoff。

来源：[Remote connections](https://learn.chatgpt.com/docs/remote-connections)

### App Server

Codex app-server 提供认证、对话历史、审批和流式事件，适合深度客户端集成。远程 WebSocket 能力不应作为本项目 MVP 的核心依赖。

来源：[Codex app-server](https://learn.chatgpt.com/docs/app-server)

## 本项目的工程推论

以下是基于官方能力作出的架构选择，不是 Codex 官方兼容承诺：

- MCP 不能替代独立后台 Agent。
- Hook 应只入队，不直接执行网络上传。
- transcript 只作为 opaque 加密备份。
- 跨电脑本地执行时，通过新对话 + 交接包恢复，而不是复制 thread。
- 工作目录使用 project identity 和相对路径映射，不依赖绝对路径一致。
