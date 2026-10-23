

<!-- fribench-shared-policy:start -->
## Fribench 公共规范引用

- 此节只适用于 Fribench 接入及部署；保留本文件其他项目约束，其他环境按原项目流程执行。
- 每次任务先定位 ops：优先 FRIBENCH_OPS_ROOT，本机默认 D:/code_CPL/fribench_ops；不得依赖工作树的相对位置。准源为私有 https://github.com/Max0314/fribench_ops 。
- 必须实际读取 ops 最新 GitHub main 的 AGENTS.md、docs/workflow.md、docs/contracts.md，不能只读本链接或依赖会话记忆。优先运行 `python <ops>/scripts/read_shared_policy.py`；也可在核实准源后 `git fetch --no-tags <GitHub remote> main`，记录 FETCH_HEAD 的 SHA，逐个用 `git show <SHA>:<文件>` 读取。不要切换、重置或覆盖 ops 的现有工作树。
- 记录本次规范 SHA；修改公共接口、接入组件及发布前再次检查变化。网络失败须标记未核实最新，不能静默用旧缓存批准公共接口或发布。未发布本地规则必须明确标注。
- 规范按最新主线读取，公共组件依赖按明确版本使用；遇到本项目原规则与公共规则冲突，列出差异和适用环境，不静默覆盖。完整引用流程见 ops 的 docs/shared-policy.md。
- 本阶段 work 各服务与私人主页面保持现状；本轮仅补充规范引用，不改变业务、不部署，也不更新其运行依赖。
<!-- fribench-shared-policy:end -->
