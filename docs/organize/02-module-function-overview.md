# 模块与功能概述

## 顶层目录

| 路径 | 当前职责 |
| --- | --- |
| `client/` | React 桌面产品 UI，包含 workbench shell、timeline、composer、settings、plugins、diagnostics、permissions、terminal 等前端功能。 |
| `desktop/` | Wails 3 桌面 shell，注册 runtime bridge，加载前端资产，提供桌面窗口和 Wails service。 |
| `internal/runtime/` | Client-first runtime 服务层，负责对 UI 暴露结构化 API 和事件。 |
| `internal/runtimeapi/` | Runtime API/event 契约定义和冻结测试。 |
| `internal/workbench/` | Agent Builder runtime 聚合层，仍被 runtime 用来启动 workspace、agent、session、permission、event 等能力。 |
| `internal/apitypes/` | 旧 API DTO/protocol 类型。 |
| `internal/agent/` | Agent loop、provider/model、prompt、tool execution、subagent/task、tool result guard。 |
| `internal/agent/tools/` | 内置工具实现和 MCP tool/resource/prompt 适配。 |
| `internal/tools/scheduler/` | Tool call 生命周期状态记录和事件基础。 |
| `internal/permission/` | Permission service、policy mode、risk classifier、scoped policy rules。 |
| `internal/db/` | SQLite 连接、migration、sqlc query 和 typed model。 |
| `internal/session/` | 会话 CRUD、scope/project、todos、usage、task session。 |
| `internal/message/` | 消息 CRUD、content parts、tool call/result/reasoning/image/binary 映射。 |
| `internal/config/` | 配置加载、写入、provider/model/MCP/LSP/skills/defaults、OAuth token、recent models。 |
| `internal/projects/` | 全局 projects.json 项目索引。 |
| `internal/workspace/` | 本地/远端 workspace 门面接口。 |
| `internal/filetracker/` | 记录 session 读过的文件及读取状态。 |
| `internal/server/`、`internal/client/` | legacy HTTP/API 兼容路径和客户端实现。 |
| `internal/app/` | Agent Builder app 聚合服务，仍是 runtime 依赖的一部分。 |
| `internal/shell/` | Shell 命令执行、内建命令、后台任务、输出编码。 |
| `internal/lsp/` | LSP client/manager/handlers，用于诊断和代码上下文能力。 |
| `internal/skills/` | Skill discovery、catalog、manager、内置 skill。 |
| `internal/hooks/` | Hook runner、input、execution 支持。 |

## 关键功能链路

### 启动与工作区

1. 桌面启动 `desktop/main.go`。
2. Wails 注册 `RuntimeBridge`。
3. `RuntimeBridge` 创建 `runtime.NewRuntimeService()`。
4. 前端通过 `wailsWorkbenchAdapter.loadInitialViewModel()` 拉取 status、sessions、models、policy、skills/MCP 等初始数据。
5. runtime 在需要时通过 `ensureWorkspaceStarted` 启动 runtime workspace、配置 store、DB store 和事件消费。

### 用户输入到 turn

1. 前端 composer 提交 prompt 或结构化 input。
2. adapter 映射为 `RuntimeChatRequest` 或 `RuntimeUserInputRequest`。
3. runtime 归一化用户输入，生成 normalized input/user message/attachment evidence。
4. runtime 创建或选择 session，创建 turn，调用 runtime/agent。
5. agent 写入 user/assistant/tool messages，执行工具。
6. runtime 消费 runtime events，记录 turns、tool calls、permissions、audit/events。
7. 前端通过 runtime event 触发 hydration，从 `SessionActivity` 还原 timeline。

### Tool call 与 permission

1. agent 通过工具执行产生 tool call。
2. `schedulerTool` wrapper 向 recorder 发送 policy/start/output/completed/failed/cancelled 记录。
3. permission policy 对工具风险、shell 命令、MCP/skill/capability scope 做评估。
4. 需要用户审批时 runtime 发布 permission request。
5. 前端 PermissionGate 决策后调用 `DecidePermission`。
6. runtime 写入 audit/event，并继续或拒绝工具执行。

### Event 与 hydration

1. runtime 把内部事件映射成 `runtimeapi.Event`。
2. 事件写入内存和 SQLite event store，并推送给 SSE server。
3. 前端通过 EventSource、polling 或 HTTP fallback 获取事件。
4. 事件只作为 refresh trigger；UI 状态仍从 `SessionActivity`、`TurnActivity`、`Permissions` 等 API 重建。

### Settings 与配置

1. Settings 面板通过 adapter 读取 provider catalog、configured providers、model config、policy、skills、MCP。
2. 保存 provider/model/MCP/skill/policy 时调用 runtime API。
3. runtime 写入 config store 或 runtime stores，发布相关事件。
4. 前端按 action refresh targets 做局部 hydration。

### Run、checkpoint 与 scheduler

runtime 已暴露 run projection、run store、checkpoint marker、transition history、scheduler plan 和 task execution。前端已有 RunProjection、scheduler candidates 的 view model 和诊断面板入口。

已知前端现状：

- Run projection preview、checkpoint resume 控件、scheduler candidate list、execute task 入口已接入工作区。
- Run summaries、checkpoint markers、transition history 在 adapter/HTTP 层有线索，但未作为主要 hydration/UI state 消费。

### Skills / MCP / capability

runtime 提供：

- skills list/refresh/create/add path/toggle
- plugins list
- MCP server save/toggle/refresh/retry
- MCP tools/resources/prompts list
- MCP request list/detail/decision
- capability list/refresh
- tool search

前端 settings/plugin surfaces 已经有 skills/MCP/provider 管理入口，但 capability inventory 和 tool search 的产品化暴露程度仍不明确。

### Terminal

runtime 提供 session terminal create/list/input/resize/delete 和 terminal event subscription。HTTP 层有 terminal WebSocket。前端有 `TerminalPane`、xterm 和 terminal view model。

### Legacy server/client

`internal/server` 仍有一套旧 `/v1` server，支持 health/version/config/workspace/session/message/agent/permission/LSP/filetracker/skills/MCP/swagger 等路由。当前 desktop-first 主路径没有直接启动它；root `serve-http` 命令启动的是 `runtime.NewRuntimeService().ServeHTTP(...)`。

### Agent tools

内置工具已覆盖：

- 文件：`view`、`write`、`edit`、`multiedit`
- Shell/job：`bash`、`job_output`、`job_kill`
- 搜索/导航：`grep`、`glob`、`ls`、`sourcegraph`、LSP diagnostics/references/restart
- 网络：`fetch`、`download`、`web_fetch`、`web_search`、`agentic_fetch`
- MCP：动态 MCP tools、`list_mcp_resources`、`read_mcp_resource`
- 运行态信息：`agent_builder_info`、`agent_builder_logs`
- Todos：`todos` / span 更新

工具装配集中在 `internal/agent/coordinator.go`，外层还会套 scheduler recorder 和 hooked tool。

## 测试覆盖概览

- Runtime：HTTP route、SSE、event cursor、turn/tool/audit/recovery、run projection/scheduler、worktree、terminal、MCP、skills 等测试较多。
- Runtime API：有 contract freeze 和 event validation 测试。
- Desktop：`runtime_bridge_test.go` 覆盖大量 bridge 方法转发。
- Agent/tools/permission/hooks/skills：单元测试覆盖较强，包括 scheduler lifecycle、policy matrix、shell risk、hook stdout/exit code、skills discovery/tracker，以及多个内置工具。
- Config：测试覆盖较广，包括 provider/model/config store/OAuth/recent models/MCP/LSP。
- DB：连接池和 migration 基础测试。
- Session/message/filetracker：有单元测试，但 session 业务覆盖相对薄。
- Client：主要通过 `client/scripts/*smoke.mjs` 和 runtime browser harness 测试，常规 TS 单测线索较少。
