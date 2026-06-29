# 模块详细梳理

本文件是全代码详细梳理的入口。每个模块按同一结构记录：职责、关键文件、核心类型、数据流、已实现能力、缺口、测试线索。

## 1. Runtime 服务层

### 范围

- `internal/runtime`
- `internal/runtimeapi`
- `internal/workbench`
- `internal/apitypes`

### 职责

`internal/runtime` 是当前 client-first runtime 的核心服务层。它把 Agent Builder runtime 包装成 transport-neutral 的 `RuntimeService`，同时服务 Wails bridge 和 loopback HTTP/dev adapter。它负责结构化管理项目、会话、turn、run、tool、permission、event、audit、skills、MCP、worktree、terminal 等状态。

`internal/runtimeapi` 负责 runtime API/event 契约定义和校验。`internal/workbench` 和 `internal/apitypes` 是旧 Agent Builder runtime 和协议 DTO 边界，runtime 当前仍依赖它们。

### 关键文件

- `internal/runtime/runtime_service.go`：`NewRuntimeService()` 创建服务实例。
- `internal/runtime/runtime_service_types.go`：`RuntimeService` 接口和 `runtimeService` 状态容器。
- `internal/runtime/runtime_contract_types.go`：所有对 UI 暴露的 `Runtime*` DTO。
- `internal/runtime/runtime_lifecycle.go`：workspace/runtime 启动、配置和恢复。
- `internal/runtime/runtime_http.go`：loopback HTTP API、SSE、terminal WebSocket、dev fallback。
- `internal/runtime/runtime_events.go`：runtime event 消费、存储、发布和 cursor replay。
- `internal/runtime/runtime_turns.go`：chat/turn/cancel/interrupted lifecycle。
- `internal/runtime/runtime_sessions.go`：session/message/activity hydration。
- `internal/runtime/runtime_tool_calls.go`：tool call 记录和查询。
- `internal/runtime/runtime_permissions.go`、`runtime_policy.go`：permission 和 policy。
- `internal/runtime/runtime_runs.go`、`runtime_run_projection.go`、`runtime_run_scheduler_*.go`：run、checkpoint、projection、scheduler。
- `internal/runtime/runtime_skills.go`、`runtime_mcp.go`、`runtime_capabilities.go`：skills/MCP/capability。
- `internal/runtime/runtime_agent_tasks.go`、`runtime_worktrees.go`、`runtime_terminal.go`：agent task、worktree、terminal。
- `internal/runtimeapi/contract.go`：endpoint 和 event contract。
- `internal/workbench/workbench.go`：旧 runtime 聚合入口。
- `internal/apitypes/*.go`：旧 apitypes DTO。

### 核心数据流

```text
RuntimeBridge 或 HTTP route
  -> RuntimeService method
  -> ensureWorkspaceStarted
  -> workbench.Runtime / app services / agent coordinator
  -> runtime stores + db migrations
  -> runtime events / audit
  -> SessionActivity hydration
```

### 已实现能力

- 项目管理、模型/provider 配置、会话、输入、turn、tool、permission、event、audit、replay、recovery。
- run projection/checkpoint/scheduler/transition。
- skills/plugins/MCP/capability/tool search。
- agent task/worktree/terminal。
- context source、prompt assembly、budget、compact/read-files。

### 缺口线索

- `runtimeapi.Endpoints` 与 `runtime_http.go` route 清单疑似漂移。
- `run.checkpoint.resumed` 事件类型可能未进入 `runtimeapi.EventTypes`。
- `runtime_refs.go` 有 reference-aware GC TODO。
- runtime 仍深度依赖 `workbench.Service` 和 `apitypes` DTO。

### 测试线索

- `internal/runtime/runtime_http_test.go`
- `internal/runtime/runtime_*_test.go`
- `internal/runtimeapi/contract_test.go`
- `desktop/runtime_bridge_test.go`

## 2. 持久化、配置、会话和消息

### 范围

- `internal/db`
- `internal/session`
- `internal/message`
- `internal/config`
- `internal/projects`
- `internal/workspace`
- `internal/filetracker`

### 职责

这一组模块提供 runtime 的基础事实存储和配置能力：SQLite、session/message model、provider/model/MCP/skills 配置、项目索引、workspace 门面和 read file tracking。

### 关键文件

- `internal/db/connect.go`：SQLite connect/release/reset pool。
- `internal/db/migrations`：数据库 schema 演进。
- `internal/db/sql` 和 `internal/db/*.sql.go`：sqlc query。
- `internal/session/session.go`：session service。
- `internal/message/message.go`、`internal/message/content.go`：message service 和 content parts。
- `internal/config/config.go`、`load.go`、`store.go`、`provider.go`、`resolve.go`：配置模型、加载、写入、provider 解析。
- `internal/projects/projects.go`：全局 projects.json。
- `internal/workspace/workspace.go`、`app_workspace.go`、`client_workspace.go`：本地/远端 workspace 门面。
- `internal/filetracker/service.go`：read_files 记录。

### 核心数据流

```text
runtime/workbench/app
  -> session.Service / message.Service
  -> db.Queries
  -> SQLite migrations

runtime/config UI
  -> config.ConfigStore
  -> global/workspace config JSON
  -> reload / provider model resolution
```

### 已实现能力

- SQLite 连接池按 data dir 复用，WAL、foreign keys、busy timeout，自动 migration。
- session CRUD、project scope、task session、todos、usage/title。
- message CRUD、content parts、provider input conversion。
- config global/workspace merge、default、provider/model、OAuth token、recent models、MCP/LSP resolution。
- projects.json 项目索引。
- local/client workspace interface。
- read_files 状态记录。

### 缺口线索

- runtime 表 access 分散在 runtime store，不在 sqlc query 中集中体现。
- `session.EstimatedUsage` 非持久。
- session 删除和 message service 删除的事件语义不一致。
- filetracker 写库失败不可回传，relative path 依赖 process cwd。
- projects register 并发读改写和非 atomic write。

### 测试线索

- `internal/db/connect_test.go`
- `internal/session/session_test.go`
- `internal/message/content_test.go`
- `internal/config/*_test.go`
- `internal/projects/projects_test.go`
- `internal/workspace/client_workspace_test.go`
- `internal/filetracker/service_test.go`

## 3. Agent、工具、权限和扩展能力

### 范围

- `internal/agent`
- `internal/agent/tools`
- `internal/tools/scheduler`
- `internal/permission`
- `internal/hooks`
- `internal/skills`

### 职责

- `internal/agent` 提供 `Coordinator` 和 `SessionAgent`，负责 provider/model、prompt assembly、session message、tool execution、queue/cancel、summarize、skills refresh、agent task execution。
- `internal/permission` 提供 policy mode、risk classifier、scoped rules 和 permission service。
- `internal/agent/tools` 提供内置工具和 MCP tool/resource/prompt 适配。
- `internal/tools/scheduler` 提供 runtime tool call 生命周期记录基础。
- `internal/hooks` 运行用户配置的 shell hook，支持 `PreToolUse`、`PostToolUse`、`UserPromptSubmit` 等事件。
- `internal/skills` 实现 skill discovery、parse/validate、dedup、enabled filter、prompt XML 注入和 loaded tracking。

### 关键文件

- `internal/agent/coordinator.go`
- `internal/agent/agent.go`
- `internal/agent/scheduler_tool.go`
- `internal/agent/hooked_tool.go`
- `internal/agent/tool_search.go`
- `internal/agent/prompt_assembly.go`
- `internal/agent/tool_result_guard.go`
- `internal/contextmgr/microcompact.go`
- `internal/agent/task_tools.go`
- `internal/agent/tools/*.go`
- `internal/permission/policy.go`
- `internal/permission/permission.go`
- `internal/tools/scheduler/scheduler.go`
- `internal/hooks/*.go`
- `internal/skills/*.go`

### 核心入口

- `internal/agent/coordinator.go`：`NewCoordinator` 组装配置、权限、skills、recorder、agent；`Run` 是会话运行入口；`buildTools` 装配内置工具、MCP 工具、tool_search、scheduler wrapper、hook wrapper。
- `internal/agent/agent.go`：`sessionAgent.Run` 创建 fantasy agent，注入 system prompt 和 tools，处理消息、标题、总结和结果保护。
- `internal/agent/scheduler_tool.go`：包装工具调用并向 recorder 发送 policy/start/output/completed/failed/cancelled。
- `internal/agent/hooked_tool.go`：工具前后运行 hooks。
- `internal/permission/policy.go`：policy mode、risk、scoped policy rule、tool/shell risk classifier。
- `internal/permission/permission.go`：pending permission、grant/deny/session grant、notifications。
- `internal/hooks/runner.go`：匹配、去重、并行运行 hooks。
- `internal/skills/skills.go`、`manager.go`、`tracker.go`：discover、prompt XML、loaded tracking。

### 已实现能力

- 会话级 agent 运行、busy 检查、队列、取消、总结、标题生成。
- 模型/provider 初始化和大小模型切换。
- MCP server instructions 注入 system prompt。
- skill prompt 注入，`deny_all` 模式不注入 skills。
- 工具结果 guard、持久化、截断；microcompact 由 runtime context manager 负责。
- 子 agent / started task execution。
- 内置工具覆盖文件、shell/job、搜索/导航、网络、MCP、运行态信息、todos。
- permission 支持 `ask`、`auto_read`、`full_access`、`plan`、`deny_all`。
- headless/task/recovery profile 下 ask fail-closed。
- scoped policy rule 支持 builtin、MCP、skill、subagent、task scope、cwd/path prefix、shell prefix/regex。
- hooks 支持 exit code 2 阻断当前工具、exit code 49 halt turn，JSON stdout 支持 decision/halt/reason/context/input rewrite。
- skills 支持 builtin/user/project source、frontmatter validate、同名覆盖、disabled filter、catalog/read。

### 缺口线索

- `internal/tools/scheduler` 名称像 scheduler，但当前更像生命周期 store/事件模型。
- scheduler 默认 memory store；持久化恢复依赖 runtime recorder/store，不在该包内完成。
- permission policy 更新并发保护不明显。
- hooks 并行执行对外部副作用的顺序语义需要明确。
- hooks `updated_input` 是浅合并。
- 新工具接入权限分类、metadata、plan read-only 白名单需要额外检查。
- skills tracker 按 name 而不是 file path 跟踪。

### 测试线索

- `internal/tools/scheduler/scheduler_test.go`
- `internal/permission/permission_test.go`
- `internal/permission/policy_test.go`
- `internal/hooks/hooks_test.go`
- `internal/skills/skills_test.go`
- `internal/skills/diagnostics_test.go`
- `internal/skills/tracker_test.go`
- `internal/agent/scheduler_tool_test.go`
- `internal/agent/hooked_tool_test.go`
- `internal/agent/tool_discovery_test.go`
- `internal/agent/tools/*_test.go`

## 4. Desktop/Wails/HTTP 适配

### 范围

- `desktop`
- `internal/server`
- `internal/client`
- `internal/cmd`
- root `main.go`

### 职责

- `desktop/main.go` 创建 Wails app 和窗口，注册 `RuntimeBridge`。
- `desktop/runtime_bridge.go` 通过 type alias 暴露 runtime DTO，通过转发方法委托 `RuntimeService`。
- root `main.go` 仍保留 CLI adapter 入口。
- `internal/cmd/root.go` 默认提示 desktop-first，`serve-http` 启动 runtime HTTP API。
- `internal/server` 是 legacy HTTP/SSE server，支持 TCP、Unix socket、Windows named pipe。
- `internal/client` 是 legacy server client，目前主要被 `internal/workspace/client_workspace.go` 使用。

### 已实现能力

- Wails bridge 覆盖项目、模型、provider、chat、user input、terminal、turn/run/tool call、hooks、refs、compact/prompt assembly、worktree、agent task、session/message/activity、permissions/policy、events/audit/replay、skills/plugins、MCP、capabilities、tool search、HTTP endpoint。
- legacy server 覆盖 health/version/config/control、workspace、sessions/history/messages、agent、permissions、config、project init、LSP、file tracker、skills、MCP、Swagger。
- platform listen/dial 覆盖 Windows named pipe 和非 Windows listener。

### 缺口线索

- 当前产品入口没有直接启动 `internal/server.Server`。
- `internal/server.Server.ListenAndServe` listener ownership 可疑。
- runtime LSP/MCP state 查询有忽略 workspaceID 的路径。
- desktop `SelectProjectDirectory` 标题有 mojibake。

### 测试线索

- `desktop/runtime_bridge_test.go`
- `desktop/runtime_bridge_live_test.go`
- `desktop/webview_test_options_test.go`
- `internal/server/recover_test.go`
- `internal/server/events_test.go`
- `internal/client/config_test.go`
- `internal/apitypes/*_test.go`
- `internal/workbench/agent_task_executor_test.go`

## 5. React Client

### 范围

- `client/src`
- `client/scripts`
- `client/package.json`

### 职责

- `client/src/app/App.tsx` 加载 runtime view model 并渲染 `WorkbenchShell`。
- `client/src/runtime/workbenchTypes.ts` 定义 UI view model。
- `client/src/runtime/wailsWorkbenchAdapter.ts` 声明 runtime DTO、transport fallback、DTO 到 view model 映射、hydration、event subscription。
- `client/src/features` 包含 workspace、sidebar、composer、timeline、tools、permissions、settings、plugins、diagnostics 等功能组件。

### 关键文件

- `client/src/main.tsx`：React 入口、Ant Design `ConfigProvider zh_CN`、error boundary。
- `client/src/app/shell/WorkbenchShell.tsx`：主 shell，持有 `WorkbenchViewModel`、当前 mode、侧栏布局、事件刷新和动作分发。
- `client/src/features/workspace/Workspace.tsx`：会话标题、timeline/chat、composer、右侧 panel、terminal tab。
- `client/src/features/composer/Composer.tsx`：Ant Design X `Sender`，模型、权限、项目范围、发送/停止。
- `client/src/features/timeline/Timeline.tsx`：message/thinking/tool/permission/progress/diagnostic/agent task/terminal marker。
- `client/src/features/settings/SettingsPanel.tsx`：providers、skills、MCP、common/general。
- `client/src/features/plugins/PluginCenter.tsx`：plugins/skills 浏览和管理。

### 已实现能力

- 项目/会话导航：侧栏、项目 CRUD、会话 CRUD、新建/选择/重命名/删除、busy 标记。
- 对话工作区：结构化 timeline 优先，bubble fallback，标题重命名。
- Composer：prompt、busy cancel、model selector、permission selector、project/standalone target、branch 展示入口。
- Timeline：用户/助手消息、thinking、tool call card/group、permission gate、turn progress、diagnostic warning、agent task row、terminal marker、复制。
- 右侧 workspace：review/files/terminal tab，可调整宽度；terminal 使用 xterm。
- 诊断：TurnDiagnostics、ContextDiagnostics、ReactCallchain、RunProjectionPreview、AgentTaskPanel。
- Settings：provider catalog/configured provider、模型发现/测试/延迟、默认模型、skills refresh/toggle、MCP server/tool/resources/prompts。
- Plugin Center：plugins/skills search/filter/detail/manage/toggle。

### 缺口线索

- 当前未实现目标栈里的 TanStack Query/Zustand 分层，仍是 shell `useState` + adapter hydration。
- Run summaries、checkpoint markers、transition history 不作为主 UI state。
- Settings nav 中多个 key 走默认 General。
- refs/artifacts center、audit/replay、worktree/diff detail、usage/cost、memory/context editor、computer use、automations 仍不完整或未产品化。
- 部分中文文案有 mojibake。

### 测试线索

- `client/package.json`：`dev`、`build`、`lint`、`preview`。
- `client/scripts/*smoke.mjs`：context diagnostics、runtime rendering、input normalization、tool loop、resume control、scheduler、provider completion、packaged WebView、action refresh、run status/read summary/checkpoint marker、terminal ownership 等。
