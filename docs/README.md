# Codex Continuity 文档

Codex Continuity 是一套服务于 Codex 的外部工作接力工具。它解决两台电脑、多个项目、多个 Codex 任务之间的整体交接问题。

## 当前结论

- 采用“外部客户端 + 自有服务端”，第一阶段不依赖 Skill、Plugin 或 MCP；
- 一个 Git 仓库，构建两套发布物；
- 服务端用 Docker Compose 运行，包含 API、SQLite、密文文件存储和管理网页；
- 客户端为 Windows 单文件 EXE；
- 一个用户可以注册多台设备，一个服务端可以扩展给多名同事；
- 用户数据、API 令牌、设备和交接记录按用户隔离；
- 服务器不访问 Codex/OpenAI，因此没有海外网络也能工作；
- 发布按“整台设备的固定工作根目录”聚合，不需要逐项目或逐任务操作。

## 文档导航

- [架构与技术选型](architecture.md)
- [部署指南](deployment.md)
- [日常工作流程](workflow.md)
- [多项目和边界情况](multi-project-handoff.md)
- [数据与安全](data-security.md)
- [实施计划与当前进度](PLAN.md)
- [产品需求](product-requirements.md)
- [官方能力边界](official-boundaries.md)
- [ADR-0002：外部客户端 + 服务端](adr/0002-external-client-server.md)

## 设计稿与实现截图

- `design/login-concept.png`：登录页布局参考；
- `design/dashboard-concept-bi-center.png`：按 `bi_center` 视觉语言生成的总览参考；
- `design/login-implementation.png`：浏览器实测登录页；
- `design/dashboard-implementation.png`：浏览器实测管理端总览。
