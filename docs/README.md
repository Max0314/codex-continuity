# Codex Continuity 文档

Codex Continuity 是一套“Windows 常驻客户端 + 私有存储服务”，用于在多台电脑之间安全同步 Codex 会话快照并延续工作上下文。

## 当前结论

- 主链路使用外部客户端，不依赖 Skill、Plugin 或 MCP；
- 一个 Git 仓库维护 Windows 客户端和 Linux 服务端；
- 服务端通过 Docker Compose 运行，包含 API、SQLite、密文存储和管理页面；
- 客户端使用 Tauri 2 + React + Rust，并内置 Go 同步核心；
- 服务器不访问 Codex/OpenAI，没有海外网络也可以工作；
- 一个用户可注册多台设备，一个服务端可扩展给多名同事；
- 所有会话正文均在客户端加密，用户数据严格隔离；
- 项目代码继续使用 GitHub/GitLab，Continuity 不替代 Git。

## 文档导航

- [产品需求](product-requirements.md)
- [架构与技术选型](architecture.md)
- [日常工作流程](workflow.md)
- [Windows 桌面客户端](desktop-client.md)
- [部署指南](deployment.md)
- [数据与安全](data-security.md)
- [实施计划](PLAN.md)
- [官方能力边界](official-boundaries.md)
- [多项目与边界情况](multi-project-handoff.md)
- [产品范围审计](audit/continuity-scope-2026-07-27/README.md)

## v0.3.0 视觉资料

- `design/desktop-main-v2-concept.png`：主窗口视觉目标；
- `design/desktop-main-v3-implementation.png`：主窗口最终实现；
- `design/desktop-tray-v3-implementation.png`：500×790 托盘最终实现；
- `design/server-login-v3-implementation.png`：服务端登录页最终实现。
