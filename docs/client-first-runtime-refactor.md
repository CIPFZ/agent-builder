# 客户端优先的 Runtime 改造与 CLI/TUI 裁剪方案

本文补充 `docs/crush-claude-code-gap-analysis.md` 中没有展开的一点：Agent Builder 的目标不是继续做一个 CLI/TUI agent，而是做一个客户端产品。因此，Crush 和 Claude Code 中大量为 CLI、TUI、终端 REPL、结构化 stdio、命令行交互适配的部分，不应该原样保留在产品主路径中。

目标是：

```text
React/Wails Client -> Runtime API + Event Stream -> Go Agent Runtime
```

而不是：

```text
Terminal UI -> CLI command loop -> Agent Runtime
```

## 核心判断

Crush 和 Claude Code 都是从 CLI/TUI 场景成长出来的。它们的很多模块解决的是终端产品问题，而不是桌面客户端问题。

对 Agent Builder 来说，需要保留的是后台 agent runtime 能力：

- session
- turn
- message
- tool call
- permission
- policy
- memory/context
- MCP
- skills
- hooks
- subagent/task
- audit/event
- model/provider
- workspace/sandbox

需要移除、隔离或降级为兼容层的是 CLI/TUI 适配：

- 终端渲染
- Bubble Tea / Ink UI 状态机
- slash command 的终端交互形态
- keybinding / vim input
- terminal prompt overlay
- stdout/stderr 进度文本协议
- CLI-only setup wizard
- CLI structured IO 作为产品主通信协议
- 依赖 tea.Msg / React Ink component 的 runtime event

客户端产品应该把 UI 状态和 runtime 状态彻底分开：Go runtime 只发布结构化事件，React 负责渲染和交互。

## 当前 Crush 中需要区分的部分

### 应保留为 runtime core

这些模块应继续作为 Go 后台能力的基础：

| 模块 | 保留理由 |
| --- | --- |
| `internal/backend` | transport-agnostic workspace/session/agent 后端入口 |
| `internal/app` | 当前 Crush workspace service 聚合点，但需要剥离 TUI 事件依赖 |
| `internal/agent` | agent 主循环、provider、tools、subagent 基础 |
| `internal/agent/tools` | 内置工具实现 |
| `internal/permission` | approval 服务基础，后续升级为 policy engine |
| `internal/session` | session 持久化基础 |
| `internal/message` | message 持久化基础 |
| `internal/db` | SQLite 存储基础 |
| `internal/config` | provider/model/MCP/skills 配置基础 |
| `internal/skills` | skill discovery 与 prompt 注入基础 |
| `internal/hooks` | runtime extension 基础 |
| `internal/lsp` | LSP diagnostics 与代码上下文能力 |
| `internal/server` | 可作为 legacy/API 参考，但不应直接成为客户端主协议 |
| `internal/runtimeapi` | 新客户端 runtime contract，应继续强化 |
| `desktop/runtime_*` | 当前 Wails/HTTP/SSE runtime bridge 基础 |

### 应隔离或逐步删除的 CLI/TUI 适配

| 模块/概念 | 建议处理 |
| --- | --- |
| `internal/ui/*` | 保留短期 TUI 兼容，长期从客户端主路径移除 |
| `tea.Msg` 作为事件载体 | 替换为 runtime-specific event schema |
| `internal/app.Subscribe(program *tea.Program)` | 保留 TUI adapter，runtime core 不应依赖它 |
| `internal/cmd/*` CLI command | 保留 legacy CLI，客户端不应通过 CLI command 调 runtime |
| `internal/commands` slash/custom command 终端形态 | 拆成 runtime command primitives 与 client command UI |
| TUI-specific dialog/model/list/chat | 不进入 Agent Builder 主路径 |
| terminal compact/transparent settings | 客户端无关，移出核心 config 或标记为 TUI-only |

当前明显的问题是 `internal/app` 和 `internal/backend/events.go` 仍以 `tea.Msg` 作为事件类型。这对 TUI 合理，但对客户端 runtime 不合理。客户端需要的是稳定 JSON event schema，而不是终端 UI 消息。

## Claude Code 中不应照搬的部分

Claude Code 很多能力值得借鉴，但以下模块主要服务 CLI/terminal/REPL，不应该成为 Agent Builder 主路径：

| Claude Code 领域 | 不应照搬的原因 |
| --- | --- |
| Ink renderer / terminal layout | Agent Builder 使用 React DOM + Wails WebView |
| REPL screen state | 客户端有自己的 screen/router/state |
| keybindings / vim input state machine | 可在客户端重新设计，不应复制终端模型 |
| CLI command registry UI | 客户端命令应表现为菜单、command palette、buttons、panels |
| structured stdio 作为主协议 | 桌面客户端应使用 local API + event stream |
| terminal permission dialogs | 客户端应有自己的 permission modal/drawer |
| terminal progress message collapsing | 客户端应消费 structured progress events |
| terminal prompt overlays | 客户端由 React component 承载 |
| stdout/stderr transcript rendering | 客户端应渲染 RuntimeMessage/ToolCall/Event |

值得吸收的是其后台语义：

- QueryEngine/turn lifecycle
- tool execution protocol
- permission rules/modes
- plan mode
- agent task lifecycle
- memory loading
- session recovery
- plugin/capability model
- remote/bridge 的协议思想

不值得吸收的是终端交互实现。

## 推荐目标架构

### 分层结构

```text
desktop
  Wails shell
  window/menu/native integration
  local runtime bootstrap

client/
  React UI
  AgentRuntime interface
  Wails adapter
  HTTP adapter
  SSE event subscription

internal/runtime
  runtime service
  session service
  turn engine
  tool scheduler
  permission policy
  event bus
  audit log

internal/agent
  model loop
  prompt/context assembly
  tools implementation
  subagent execution

internal/adapters
  tui adapter
  cli adapter
  http adapter
  wails adapter
```

Crush 当前还没有 `internal/runtime` / `internal/adapters` 这样明确的边界。建议后续逐步演进，而不是一次性重构。

### 主路径

客户端主路径应该是：

```text
React UI
  -> AgentRuntime
  -> WailsRuntime or HTTPRuntime
  -> RuntimeService
  -> TurnEngine
  -> ToolScheduler
  -> Agent/Tools/Permission/DB
  -> RuntimeEventBus
  -> SSE/Wails event push
  -> React UI update
```

这条路径中不应出现：

- `tea.Msg`
- TUI model
- CLI command parsing
- terminal renderer
- stdout text protocol

这些只能存在于 legacy adapter 或开发调试入口。

## 后台 Runtime 与客户端 UI 的结合方式

### 1. Runtime 是 source of truth

React 只负责展示和发命令，不负责推断核心业务状态。

客户端状态必须可从以下来源恢复：

- runtime status
- sessions
- messages
- turns
- tool calls
- permissions
- capabilities
- audit events
- event history

如果刷新窗口或重开客户端，UI 应该能通过 runtime API 恢复，而不是依赖 React 内存状态。

### 2. 所有长流程变成 Turn 或 Task

用户在客户端输入一句话，不应该只是一次 `Chat()` 调用，而应该创建一个 turn：

```text
POST /v1/sessions/{session_id}/turns
```

turn 下挂：

- user message
- assistant message
- tool calls
- permission requests
- audit events
- usage
- artifacts
- child tasks

后续复杂任务再抽象成 task/run：

```text
Turn
  ToolCall
  PermissionRequest
  AgentTask
  Artifact
  AuditEvent
```

### 3. UI 通过事件流驱动

客户端不应该轮询所有状态来猜测进度。runtime 应发布结构化事件：

```text
turn.started
message.created
message.updated
tool.call.started
tool.call.output
permission.requested
permission.decided
tool.call.completed
turn.completed
audit.recorded
```

React 收到事件后再按需拉取最新详情。

事件是通知，API 是事实来源。

### 4. Permission 是客户端交互的一等对象

权限请求不应该是终端 prompt，也不应该是工具返回的一段文本。它应是 runtime object：

```json
{
  "id": "...",
  "session_id": "...",
  "turn_id": "...",
  "tool_call_id": "...",
  "tool_name": "bash",
  "action": "execute",
  "risk": "write",
  "description": "...",
  "input": {},
  "options": ["allow_once", "allow_session", "deny"]
}
```

客户端渲染 permission modal 或 side panel，用户选择后调用：

```text
POST /v1/permissions/{permission_id}/decision
```

### 5. Tool output 是结构化内容

CLI/TUI 常把工具输出压成文本。客户端需要结构化数据：

```text
ToolCall
  id
  name
  status
  input
  output
  stderr/stdout
  diff
  files
  artifacts
  started_at
  completed_at
```

React 可以根据 tool 类型渲染不同 UI：

- shell output
- file diff
- search results
- MCP resource
- permission
- subagent progress

### 6. CLI 变成 adapter，不是产品核心

未来可以保留 CLI，但 CLI 应该调用同一 runtime API：

```text
CLI -> Runtime API -> RuntimeService
TUI -> Runtime API/Event Stream -> RuntimeService
Desktop -> Runtime API/Event Stream -> RuntimeService
```

不要再让 CLI/TUI 拥有独立业务逻辑。

## 建议的裁剪策略

### 第一阶段：隔离

不立刻删除 TUI/CLI，先建立边界：

- 新增 runtime-native event bus。
- 将 `tea.Msg` 限制在 TUI adapter 内。
- 桌面 runtime 不直接消费 TUI message。
- RuntimeService 暴露所有客户端需要的状态。
- React 不构造 user/assistant/tool message，只展示 runtime 返回的数据。

### 第二阶段：替换

把核心能力从 CLI/TUI 形态迁移到 runtime API：

- slash command -> runtime command definition + client command palette。
- TUI permission prompt -> permission API + React modal。
- TUI progress -> runtime event + React timeline。
- TUI session picker -> sessions API + sidebar。
- TUI diff view -> tool call diff payload + React diff viewer。

### 第三阶段：删除或降级

当桌面客户端主路径稳定后：

- `internal/ui` 变成 legacy TUI package。
- CLI command 只作为 thin adapter。
- TUI-only config 移出 core config。
- tea event bus 从 `app.App` core 中移除。
- runtime tests 不依赖 TUI types。

## 具体重构建议

### 1. 新增 runtime event 类型

当前 `internal/app` 事件以 `pubsub.Broker[tea.Msg]` 为核心。建议新增：

```go
type RuntimeEvent struct {
    ID         string
    Type       string
    CreatedAt  time.Time
    SessionID  string
    TurnID     string
    MessageID  string
    ToolCallID string
    Payload    map[string]any
}
```

`internal/runtimeapi` 已经有类似结构，应把它提升为 runtime core event，而不是只存在 contract 包。

### 2. 新增 adapter 层

目标：

```text
runtime core event -> TUI adapter -> tea.Msg
runtime core event -> HTTP/SSE adapter -> JSON
runtime core event -> Wails adapter -> frontend event
```

这样 TUI 可以继续存在，但不污染 runtime core。

### 3. RuntimeService 前移

当前 `desktop/runtime_service.go` 已经实现了很多客户端 runtime 能力，但它在 desktop module 下。

建议后续迁移为：

```text
internal/runtime/service.go
```

然后：

- Wails 调用它。
- HTTP 调用它。
- CLI/TUI 也可以调用它。

### 4. Tool Scheduler 前移

当前工具执行仍在 agent/tools 内部散落。建议新增：

```text
internal/runtime/toolscheduler
```

负责：

- validation
- permission policy
- lifecycle event
- audit
- cancellation
- output normalization

### 5. Client command 模型替代 slash command

CLI slash command 不是客户端最终形态。客户端需要：

```text
CommandDefinition {
  id
  title
  description
  category
  input_schema
  source
  enabled
}
```

React 可以把它渲染成 command palette、菜单或按钮。

### 6. Permission UI 与 Policy 解耦

Runtime 负责判断是否需要 approval；React 负责呈现。

不要让 UI 参与策略判断。

### 7. 保留 headless/automation 能力

“删除 CLI/TUI 适配”不等于不能 headless。Headless 应该走 runtime API 或 SDK protocol，而不是复用终端 UI。

推荐：

```text
headless client -> HTTP/JSON + SSE
automation -> runtime API
desktop -> same API
```

## 客户端化后的模块归属

| 能力 | Runtime | Client |
| --- | --- | --- |
| provider/model config | source of truth | 表单与展示 |
| session/message | source of truth | sidebar/thread 渲染 |
| turn lifecycle | source of truth | timeline/status 渲染 |
| tool execution | source of truth | tool card 渲染 |
| permission policy | source of truth | modal/drawer 决策入口 |
| memory/context loading | source of truth | 配置/诊断展示 |
| skills/MCP/plugins | source of truth | 启用、禁用、详情页 |
| audit log | source of truth | audit drawer/report |
| diff/artifact | source of truth | viewer/editor |
| layout/theme/interaction | 无 | source of truth |

## 与现有文档的关系

本文与现有文档关系：

- `desktop-runtime-boundary.md` 已说明 React 是薄展示层。
- `phase-2-runtime-api-boundary.md` 已说明 HTTP/SSE 是长期边界。
- `crush-claude-code-gap-analysis.md` 已说明 Crush 与 Claude Code 的能力差距。
- 本文进一步明确：CLI/TUI 适配不是 Agent Builder 的产品主路径，应隔离并逐步从 runtime core 中剥离。

## 最小下一步

建议下一步优先做三件事：

1. 将 runtime event 从 `tea.Msg` 依赖中剥离，建立 runtime-native event bus。
2. 将 `desktop/runtime_service` 的通用部分迁移规划到 `internal/runtime`。
3. 定义 client-first `Turn` / `ToolCall` / `PermissionRequest` API，并让 React 只消费这些结构。

完成这三步后，Agent Builder 才会真正从“Crush 的桌面包装”转向“客户端优先的 agent runtime 产品”。

