# Client Runtime Architecture Review

本文是当前 docs 审计、客户端 runtime 架构确认，以及 Crush/fantasy 与
Claude Code QueryEngine 语义对齐的汇总入口。结论基于当前仓库状态：
TUI/CLI 主路径已经移除或降级，React/Wails 桌面客户端是产品界面，Go
runtime 是事实来源。

Status update, 2026-05-24: this review remains useful for architecture
boundaries, but its original P0/P1/P2 gap list has been overtaken by the
runtime implementation on `main`. For current Claude Code parity and execution
order, use:

- `docs/claude-code-runtime-parity-audit.md`
- `docs/claude-code-alignment-next-roadmap.md`

## 结论摘要

Agent Builder 不应继续沿用 Crush 的 TUI/CLI 产品形态，也不应复制 Claude
Code 的 terminal UI。当前目标是：

```text
React Client -> AgentRuntime interface -> Wails/HTTP adapter
  -> Go RuntimeService -> TurnEngine -> Agent/fantasy/Tools
  -> RuntimeEventBus/AuditStore -> SSE/Wails events -> React
```

`charm.land/fantasy` 已经承担 provider/model/tool stream abstraction。后续
不要重复实现 provider adapter、message/tool provider protocol 或 LLM
streaming engine。Agent Builder 需要实现的是 fantasy 之上的 runtime
orchestration：turn lifecycle、tool lifecycle、permission policy、event/audit、
state recovery、task/subagent、capability inventory 和客户端可恢复状态。

## Docs 审计结果

### Active Architecture Docs

这些文档仍应作为当前架构输入：

| 文档 | 状态 | 说明 |
| --- | --- | --- |
| `docs/client-runtime-architecture-review.md` | active | 本文，作为当前总览入口。 |
| `docs/claude-code-runtime-parity-audit.md` | active | 当前 Agent Builder runtime 与 Claude Code runtime 的全量 parity 审计。 |
| `docs/claude-code-alignment-next-roadmap.md` | active | 当前下一阶段执行路线；已取代旧 P0/P1 scheduler-first 计划。 |
| `docs/claude-code-alignment-module-priority.md` | active pointer | 短入口，指向最新 audit/roadmap。 |
| `docs/client-architecture-and-core-flow.md` | active | 客户端主架构和核心流程；方向正确。 |
| `docs/desktop-runtime-boundary.md` | active | 明确 React thin UI、Wails adapter、Go runtime source of truth。 |
| `docs/archive/phase-2-runtime-api-boundary.md` | historical baseline | Phase 2 runtime API、SSE、skills、MCP、audit baseline；大部分已实现，不能再作为当前 execution plan。 |
| `docs/turn-task-run-model.md` | active | Turn/Task/Run 数据模型边界。 |
| `docs/tool-scheduler-design.md` | active | Tool Scheduler 目标职责和生命周期。 |
| `docs/permission-policy-model.md` | active | Permission/Policy 模型升级方向。 |
| `docs/frontend-runtime-ui-technical-plan.md` | active | 前端重构、Codex-like workbench、Ant Design X、API-as-truth、event cursor、active turn 恢复。 |
| `docs/client-information-architecture.md` | active | 客户端信息架构，conversation-first。 |
| `docs/legacy-crush-inventory.md` | active reference | 清理结果和遗留 surface 说明，需局部更新旧 wording。 |
| `docs/archive/dev-baseline.md` | historical reference | 本机 baseline 记录；不再是执行路线入口。 |
| `docs/architecture-decisions.md` | partial active | ADR-004 到 ADR-008 仍有效；ADR-001 和旧 next steps 已过期。 |

### Archived Historical Docs

以下文档已移动到 `docs/archive/`，作为历史背景保留，不再作为当前执行入口：

| 文档 | 原因 |
| --- | --- |
| `docs/archive/client-ui-plan.md` | Phase 1 mock UI、DeepSeek、SSH demo 路线已过期。 |
| `docs/archive/implementation-roadmap.md` | 仍描述 Phase 0/1 mock、保留 TUI、Phase 6 Wails 等历史阶段。 |
| `docs/archive/phase-1-acceptance-test.md` | Phase 1 desktop acceptance 是历史验收文档。 |
| `docs/archive/project-structure-refactor-plan.md` | 目录整理/TUI 去除计划已完成或被当前架构替代。 |
| `docs/archive/root-cleanup-review.md` | 根目录清理审计属于已完成阶段。 |
| `docs/archive/tui-removal-plan.md` | TUI removal 已完成，保留为历史执行记录。 |

### Existing Archive Docs

这些文档继续保留在 archive，用作参考分析，不应作为当前状态来源：

| 文档 | 用途 |
| --- | --- |
| `docs/archive/crush-claude-code-gap-analysis.md` | 历史 gap analysis，当前 review 已吸收核心结论。 |
| `docs/archive/reference-analysis/claude-code.md` | Claude Code reference analysis。 |
| `docs/archive/reference-analysis/crush.md` | Crush reference analysis，含早期 TUI 状态。 |
| `docs/archive/reference-analysis/comparison.md` | 多项目横向比较，部分 TUI wording 已过期。 |
| `docs/archive/reference-analysis/codex.md` | Codex reference analysis。 |
| `docs/archive/reference-analysis/gemini-cli.md` | Gemini CLI reference analysis。 |

### Merge / Delete 建议

- `docs/client-architecture-and-core-flow.md` 和本文有重叠，但前者应保留为
  runtime/client 主流程细节，本文作为 review/roadmap 汇总。
- `docs/client-first-runtime-refactor.md` 与
  `docs/archive/tui-removal-plan.md` 的旧隔离/裁剪步骤有重复。前者仍有
  价值，但需要更新“未来 TUI adapter”、“不立刻删除”等 wording；后者已归档。
- `docs/architecture-decisions.md` 应后续拆分：保留 runtime API、plugin、
  permission、Web 预留、命名 ADR；把 SSH MVP 和 Phase 1 mock next steps
  标记 historical 或移入 archive。
- 暂不建议删除 docs。当前更安全的处理是归档，避免丢失 refactor 背景。

## 当前客户端整体架构确认

### 分层边界

| 层 | 职责 | 不应承担 |
| --- | --- | --- |
| React client | 展示、输入、审批交互、设置表单、timeline/detail rendering、UI-only state。 | session/message/tool/permission/audit 的权威状态。 |
| AgentRuntime TS interface | 屏蔽 Wails/HTTP transport 差异，提供稳定客户端 contract。 | 业务规则、权限判断、状态推断。 |
| Wails adapter | 桌面窗口、菜单、native bootstrap、本地 runtime token/endpoint 传递。 | 长期协议或业务 runtime。 |
| HTTP/SSE adapter | 本地 loopback API、event stream、headless/Web 复用入口。 | 独立业务逻辑。 |
| Go RuntimeService | session/turn/message/tool/permission/skill/MCP/capability/audit API 的事实来源。 | provider protocol 细节和 UI 渲染。 |
| TurnEngine | 创建、运行、取消、恢复 turn，绑定 message/tool/permission/audit。 | 具体 provider adapter。 |
| ToolScheduler | tool lifecycle、permission gate、output normalization、cancellation、audit。 | LLM provider abstraction。 |
| PermissionPolicy | allow/ask/deny、mode、risk、rule、headless 行为。 | UI modal 决策。 |
| internal/agent + fantasy | model loop、provider stream、prompt/history/tool call protocol。 | 客户端状态恢复、audit store、policy engine。 |

### Runtime Source of Truth

必须由 Go runtime 作为权威来源：

- workspace/config/provider/model
- sessions/messages/turns
- tool calls and tool results
- permission requests and decisions
- skills/MCP servers/MCP tools/capabilities
- active/running/waiting/cancelled/failed status
- usage/cost
- audit events/artifacts
- recovery state/event cursor

React 只拥有 UI 状态：

- 当前 route/tab/drawer/modal
- 输入框草稿
- 选择中的 UI item
- 折叠、排序、过滤、局部 loading state
- optimistic visual hint，但必须可由 runtime refresh 覆盖

## Crush / fantasy 能力基线

当前项目通过 `charm.land/fantasy v0.25.0` 使用以下底层能力：

- `fantasy.Provider` / `LanguageModel`：统一 OpenAI、Anthropic、OpenRouter、
  Vercel、Azure、Bedrock、Google、OpenAI-compatible、Hyper 等 provider。
- `fantasy.NewAgent` 和 `Agent.Stream`：负责 model step loop、streaming、
  reasoning/text/tool callbacks、stop conditions、provider retry/error shape。
- `fantasy.AgentTool` / `NewAgentTool` / `NewParallelAgentTool`：统一 tool
  schema、tool call input、tool response、parallel tool execution contract。
- `fantasy.Message` / `MessagePart` / `ToolCallPart` / `ToolResultPart`：统一
  model-facing transcript representation。
- `fantasy.ProviderOptions` / provider-specific options：承载 cache control、
  reasoning、Responses API、OpenAI-compatible extra body 等 provider 差异。
- `fantasy.Usage` / `ProviderMetadata` / `ProviderError`：用于 usage/cost、
  retry logging、provider error normalization。

这意味着 Agent Builder 不应重复实现：

- OpenAI/Anthropic/Gemini/etc. provider clients。
- model-facing message/tool call/tool result protocol。
- provider streaming callback adapter。
- provider retry/error abstraction。
- basic tool schema/function-call abstraction。

Agent Builder 可以在 fantasy 上方保留的 provider policy 层包括：

- provider/model config ownership and redaction
- model capability display
- per-mode model choice
- usage/audit accounting
- provider health and verification API
- product policy around which providers/models are allowed

## fantasy 与 QueryEngine 对齐

Claude Code `QueryEngine` 的价值不是 provider abstraction，而是 runtime
orchestration。映射关系如下：

| Claude Code QueryEngine 语义 | 当前 Crush/fantasy 基线 | Agent Builder 需要补齐 |
| --- | --- | --- |
| conversation/turn lifecycle | `SessionAgent.Run` + session busy map | 持久 `Turn` object、status、cancel/resume API。 |
| model streaming | fantasy `Agent.Stream` | 保留 fantasy，不重复实现。 |
| model/tool protocol | fantasy messages/tools/results | 保留 fantasy；在外层记录 ToolCall runtime object。 |
| permission denials/modes | `internal/permission` allow/deny/session grant | `PermissionPolicy`、mode、risk、audit、headless behavior。 |
| tool execution protocol | fantasy tool `Run` + current hooks | `ToolScheduler` 包装 lifecycle、validation、permission、audit。 |
| context/memory | prompt template、context paths、skills XML、summary | layered instruction/memory loading、read-file state、compact lifecycle。 |
| subagent/background task | `agent` tool creates child task session | persisted `AgentTask` with status/progress/cancel/resume/artifact。 |
| event stream | app/backend pubsub + desktop runtime events | runtime-native event bus with sequence/cursor and stable schema。 |
| recovery/resume | SQLite sessions/messages + orphan repair | turn/tool/permission/audit store and interrupted recovery scan。 |
| audit/telemetry | logs, session usage, desktop audit bridge | append-only runtime audit store as product contract。 |

## Runtime 差距状态

The original gap list below described the path before the current
`internal/runtime` spine landed. It is retained as historical context, not as
the current execution order. Current status is summarized in
`docs/claude-code-runtime-parity-audit.md`.

### Historical P0 Gap: Runtime Spine

Resolved as a foundation. Runtime turns, tool calls, events, audit, recovery,
capability inventory, policy baseline, and AgentTask records now exist under
`internal/runtime` and related packages. The stable main chain is:

```text
Turn -> ToolCall -> PermissionRequest -> Event -> Audit
```

Remaining work is hardening, not creating the spine:

- Full resume/continuation beyond interrupted marking.
- Richer replay/debug event persistence.
- Compact boundary and prompt budget integration.

### Historical P1 Gap: Tool Scheduler

Implemented as a foundation through `internal/tools/scheduler`,
`internal/agent/scheduler_tool.go`, and
`internal/runtime/runtime_scheduler_recorder.go`. Remaining scheduler work:

- tool search/discovery,
- explicit deadlock and recursion limits,
- per-source concurrency policy,
- durable output/artifact refs,
- richer result normalization.

### Historical P2 Gap: Permission Policy

Implemented as a partial foundation. `ask`, `auto_read`, `plan`, and
`deny_all` modes exist, and plan mode blocks non-read tool calls through
runtime policy. Remaining work:

- scoped policy rules for tool/MCP/skill/subagent/cwd/shell,
- richer Bash/PowerShell safety,
- explicit headless profiles,
- policy regression scenarios,
- model-assisted advisor as advisory-only, never self-approval.

### Historical P3 Gap: Recovery

Implemented as a foundation. Event cursors, recovery status, interrupted turns,
pending permissions, and startup recovery paths exist. Remaining work:

- persisted event replay/export beyond bounded runtime memory,
- richer audit diagnostics,
- compact-aware recovery and reinjection.

### Current P1 Gap: Context, Memory, Compact, Subagent

The largest current gaps are the long-session primitives identified in the
2026-05-24 parity audit:

- compact lifecycle: micro, full, session memory, auto trigger, reinjection,
- prompt/tool budget and model-facing tool search,
- scoped policy/shell safety,
- AgentTask roles, scopes, parent/child communication, artifacts,
- worktree/sandbox/remote isolation,
- local scenario/eval harnesses.

## Historical Route And Current Route

The phase route below is historical. It explains why the runtime boundary was
built, but the current implementation roadmap has moved to compact/tool
governance and is maintained in
`docs/claude-code-alignment-next-roadmap.md`.

### Historical Phase 1: Freeze Runtime Contract

范围：

- 将 active docs 作为当前 contract。
- 更新过期 docs 引用。
- 明确 Wails/HTTP 共享 RuntimeService。
- 确认 React 只消费 AgentRuntime shapes。

验收：

- docs 不再把 Phase 1 mock/TUI 保留作为当前路线。
- runtime API/event schema 有单一入口。

### Historical Phase 2: Durable Turn Store

范围：

- 新增或固化 `runtime_turns`。
- `POST /sessions/{id}/turns` 创建 turn。
- `GET /turns/{id}` 和 active turn query。
- cancel 以 turn 为单位。
- 当前 `requestID == turnID` 可作为迁移兼容。

验收：

- 客户端刷新后能恢复 active/waiting/cancelled/completed turn。
- audit 能按 turn 查询最小信息。

### Historical Phase 3: Runtime Event Bus and Cursor

范围：

- runtime event 增加 sequence/cursor。
- SSE 支持 `after`。
- event payload 只携带摘要；事实数据通过 API 拉取。
- restart/reconnect 支持 snapshot-required。

验收：

- SSE 断线重连不会丢关键 permission/tool/turn 状态。
- React 不依赖轮询作为主机制。

### Historical Phase 4: Tool Scheduler

范围：

- 在 fantasy tool 执行外层加入 scheduler wrapper。
- 生成 ToolCall runtime object。
- 记录 status、input summary、output summary、metadata、error。
- 初期保留现有 fantasy tools，不重写工具实现。

验收：

- tool lifecycle 可通过 API 查询。
- tool events 不再完全依赖 message part 反推。
- permission 和 audit 关联 tool_call_id。

### Historical Phase 5: PermissionPolicy

范围：

- 在现有 `internal/permission` 上新增 policy evaluation。
- 增加 risk/mode/status/decision audit。
- `allow` 命名收敛为 `allow_once`。
- headless ask 行为明确。

验收：

- UI 只提交 decision，不判断风险。
- pending permission 可恢复、可过期、可审计。
- plan/read-only mode 能阻止写入/执行类 tool。

### Historical Phase 6: RuntimeService Migration

范围：

- 将 `desktop/runtime_*` 通用部分迁到 `internal/runtime`。
- Wails 和 HTTP 只做 adapter。
- 保持 TypeScript `AgentRuntime` contract 稳定。

验收：

- desktop 不拥有业务 runtime。
- HTTP smoke tests 和 Wails client 使用同一 service。

### Historical Phase 7: Task/Subagent and Context

范围：

- 将 `agent` tool child session 升级为 persisted AgentTask。
- 增加 task status/progress/cancel/result/artifact。
- 分层 instruction/memory loading。
- 后续再加入 worktree isolation。

验收：

- subagent 可见、可取消、可恢复。
- task output 可审计并汇总回 parent turn。

## 风险点

- 一次性迁移 `desktop/runtime_*`、tool execution 和 permission 会扩大 blast
  radius；应先固化 Turn/Event，再切 ToolScheduler。
- 如果 React 继续通过事件拼接事实状态，recovery 会不可靠；必须坚持 API
  是事实来源。
- 如果在 fantasy 旁边再造 provider abstraction，会导致 provider option、
  streaming、tool result 语义重复且容易漂移。
- Policy 不应一次性做完整企业 RBAC；先做 mode/risk/rule/decision/audit。
- Archive 文档仍包含旧 TUI/Phase 1 wording；引用时必须看当前 review 和
  active docs。

## Current Recommended Priority

1. Compact lifecycle foundation.
2. Micro compact for tool outputs.
3. Prompt/tool budget accounting.
4. Tool search / capability discovery.
5. Scoped policy rules and shell safety.
6. AgentTask scope, role definitions, and parent/child messaging.
7. MCP/skills scoped activation.
8. Background job entity and output/artifact refs.
9. Worktree isolation.
10. Scenario/eval harnesses and replay diagnostics.
