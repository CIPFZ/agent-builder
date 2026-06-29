# Agent Builder 项目梳理索引

本目录保存 Agent Builder 当前代码现状的分层梳理结果。目标是理解系统、建立功能地图，并为后续功能查漏补缺提供依据。

## 文档结构

- [01-project-overview.md](01-project-overview.md)：项目整体概述、主运行链路、核心边界。
- [02-module-function-overview.md](02-module-function-overview.md)：按模块和功能链路整理职责、入口、已实现能力。
- [03-feature-gap-inventory.md](03-feature-gap-inventory.md)：功能查漏补缺清单，按“已实现、部分实现、可疑缺口、测试缺口”归类。
- [04-module-deep-dive.md](04-module-deep-dive.md)：分模块全代码详细梳理入口，记录关键文件、核心类型、数据流和测试线索。
- [05-todo-subagent-comparison.md](05-todo-subagent-comparison.md)：Todo Write 与 Subagent 能力对比，包含参考项目差异和补齐顺序。
- [06-todo-subagent-implementation-plan.md](06-todo-subagent-implementation-plan.md)：Todo 与 Subagent 的分阶段实施方案。
- [07-hooks-comparison.md](07-hooks-comparison.md)：Hook 机制梳理对比，覆盖后端 runtime、事件、持久化、前端展示和参考项目差异。
- [08-hooks-frontend-implementation-plan.md](08-hooks-frontend-implementation-plan.md)：Hook 前端展示与可观测性的完整实施方案。
- [09-system-prompt-comparison-and-plan.md](09-system-prompt-comparison-and-plan.md)：System Prompt 机制梳理对比，以及按 Claude 式 section graph 拆分的目标方案和路线。
- [10-error-recovery-implementation-plan.md](10-error-recovery-implementation-plan.md)：Error Recovery 机制完整实施方案，覆盖 runtime 恢复、request hygiene、显式续跑、reactive retry、前端 Recovery Center 和旧结构删除。

## 梳理原则

- 以代码事实为准，现有规划文档只作为背景。
- 按“功能能力”组织结论，避免只按目录罗列文件。
- 区分主产品路径、兼容路径、开发/测试路径和遗留路径。
- 重点标出前端有入口但后端链路不足、后端有能力但前端未暴露、代码有痕迹但能力不完整的地方。
- 不输出重构路线图；只输出现状地图和补缺依据。

## 本轮覆盖范围

- 根入口与桌面入口：`main.go`、`desktop/main.go`、`desktop/runtime_bridge.go`
- Runtime 主边界：`internal/runtime`、`internal/runtimeapi`
- 后端兼容边界：`internal/workbench`、`internal/apitypes`
- 持久化与配置：`internal/db`、`internal/session`、`internal/message`、`internal/config`、`internal/projects`、`internal/workspace`、`internal/filetracker`
- Agent、工具、权限、hooks、skills：`internal/agent`、`internal/agent/tools`、`internal/tools/scheduler`、`internal/permission`、`internal/hooks`、`internal/skills`
- 桌面和兼容服务：`desktop`、`internal/server`、`internal/client`、`internal/cmd`
- 前端：`client/src/app`、`client/src/runtime`、`client/src/features`、`client/scripts`
- 现有架构文档：`README.md`、`docs/README.md`、`docs/frontend-runtime-ui-technical-plan.md`、`docs/frontend-runtime-integration-notes.md`

## 产物状态

这些文档是第一轮全局梳理结果，已经覆盖主产品路径和主要兼容路径。若继续深入，建议按 `04-module-deep-dive.md` 中的模块逐个扩展到逐文件级别。
