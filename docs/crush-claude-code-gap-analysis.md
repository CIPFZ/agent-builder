# Crush 与 Claude Code 全模块差距梳理

本文基于当前 `main` 分支的 Agent Builder/Crush 代码，以及本地 Claude Code 快照：

```text
C:\Users\ytq\work\ai\myclaw\claude-code
```

目标不是照搬 Claude Code，而是识别 Claude Code 已经沉淀出的 agent runtime primitives，并判断哪些应该在 Crush/Agent Builder 中补齐，以支撑“以 Crush 为底座、以 Claude Code 为设计蓝图、最终转为 Codex 类客户端”的方向。

## 总体结论

Crush 是一个干净、可改造的 Go agent runtime 底座，但当前仍偏向“单主 agent + tools + session + TUI/API”的结构。Claude Code 已经是完整产品级 agent runtime：权限策略、工具调度、上下文经济、子 agent 生命周期、记忆、远程/SDK/结构化 IO、插件、feature gate、telemetry、恢复机制都更成熟。

两者差距不是某几个工具缺失，而是运行时层级差距。

Agent Builder 当前主线已经走在正确方向上：

- 保留 Crush Go runtime 作为真实执行层。
- 用 Wails v3 做桌面薄壳。
- 用 React 做客户端交互层。
- 通过 `AgentRuntime` facade 隔离 Wails 与未来 HTTP/SSE runtime API。
- 将 session、turn、message、permission、skill、MCP、capability、audit、event stream 逐步暴露给客户端。

但要达到 Claude Code/Codex 级别，核心工作仍在 runtime primitives，而不是 UI 表层。

## 代码库规模与形态

### Crush / Agent Builder

当前仓库约 1000 个可见源码/文档/测试文件，主体为 Go：

- 根目录 CLI/TUI 入口：`main.go`
- Go runtime：`internal/backend`、`internal/app`、`internal/agent`、`internal/agent/tools`
- 持久化：`internal/db`、`internal/session`、`internal/message`
- 权限：`internal/permission`
- MCP/skills/hooks/LSP：`internal/agent/tools/mcp`、`internal/skills`、`internal/hooks`、`internal/lsp`
- 桌面壳：`desktop`
- React 客户端：`client`
- 新 runtime API 契约：`internal/runtimeapi`

### Claude Code 快照

本地快照约 1950 个文件，主体为 TypeScript/TSX：

- 入口与装配：`src/main.tsx`
- 主查询引擎：`src/QueryEngine.ts`
- 工具注册：`src/tools.ts`
- 工具协议：`src/Tool.ts`
- 权限策略：`src/utils/permissions/*`
- 子 agent 与任务：`src/tools/AgentTool/*`、`src/tasks/*`
- 上下文/记忆：`src/context.ts`、`src/utils/claudemd.ts`、`src/memdir/*`
- CLI/SDK/结构化 IO：`src/cli/*`
- remote/bridge/server：`src/remote`、`src/bridge`、`src/server`
- plugins/skills/MCP：`src/utils/plugins/*`、`src/skills/*`、`src/services/mcp/*`
- UI/REPL：`src/components/*`、`src/screens/REPL.tsx`、`src/ink/*`

Claude Code 的模块拆分更偏完整产品运行时；Crush 的模块更简洁，更适合作为 Go runtime 底座继续演进。

## 全模块对照

| 领域 | Crush 当前 | Claude Code 当前 | 差距等级 |
| --- | --- | --- | --- |
| 启动装配 | `main.go` -> `cmd.Execute()`，结构简洁 | `main.tsx` 装配 MDM、keychain、feature flags、MCP、plugins、skills、LSP、remote、telemetry、sandbox | 很大 |
| 主循环 | `internal/agent/sessionAgent.Run` 基于 fantasy agent stream | `QueryEngine` 独立管理 turn、messages、permissions、usage、file cache、SDK events、snip/replay | 很大 |
| 工具系统 | `internal/agent/tools/*`，Go 工具集合，权限由工具主动请求 | `Tool.ts` + `services/tools/toolExecution.ts`，统一 validation/progress/hooks/permission/telemetry/result storage | 很大 |
| 权限 | `permission.Service`: allow / allow_session / deny / skip | rule engine + permission mode + classifier + hooks + sandbox override + working dir policy + denial tracking | 极大 |
| Plan Mode | 暂无完整模式语义 | `EnterPlanModeTool`、`ExitPlanModeTool`、`prePlanMode`、plan approval | 极大 |
| Shell 安全 | Crush shell + permission prompt | Bash/PowerShell parser、dangerous pattern、subcommand analysis、read-only validation、sandbox decision | 极大 |
| Subagent | `agent` tool，可创建 task session，一次性并行子 agent | AgentTool 支持 agent definitions、background、progress、worktree/remote/cwd isolation、resume、team/swarm | 极大 |
| Background task | 有 background shell `job_output` / `job_kill` | agent task / shell task / remote task / cron / monitor / sleep / task notification | 很大 |
| 上下文 | prompt template + `context_paths` + git status + skills | system/user context、CLAUDE.md 分层、include、frontmatter globs、memory cache、read-file state、history snip | 极大 |
| 记忆 | skills 和 context paths 为主 | user/project/local/managed memory、agent memory、team memory、snapshot/sync/drift prevention | 极大 |
| Session | SQLite sessions/messages/todos，父子 session 初步存在 | JSONL transcript、parent chain、subagent sidecars、remote metadata、resume/replay、compact boundary | 很大 |
| API/Transport | HTTP server + workspace API；桌面新增 runtime API/SSE | CLI structured IO、SDK control protocol、SSE/WS/hybrid transport、bridge/remote/direct connect | 很大 |
| 插件 | MCP、skills、hooks 已有基础 | plugins marketplace/loading/policy/versioning/cache，plugin commands/agents/hooks/skills/MCP | 很大 |
| UI | Crush TUI + Agent Builder React/Wails 初版 | Ink REPL 深度状态机、permissions UI、agent panels、diff、teams、bridge/remote UI | 很大 |
| Model/provider | Catwalk/fantasy 多 provider | Anthropic-first + provider routing/capability restrictions/model policy | 中到大 |
| Audit/telemetry | Agent Builder 已有 runtime audit 雏形 | OTel/session tracing/analytics/growthbook/provider restriction audit | 很大 |
| Testing/eval | Go tests，runtime API tests | harness/eval/runtime、SDK/headless/structured IO 测试体系 | 大 |

## 关键差距详解

### 1. Runtime 主循环

Crush 的主执行路径集中在：

- `internal/agent/agent.go`
- `internal/agent/coordinator.go`
- `internal/backend/agent.go`

当前 `SessionAgent.Run` 负责：

- 获取 session messages。
- 创建 user message。
- 准备 prompt/history/files。
- 调用 fantasy agent stream。
- 写 assistant/tool message。
- 自动摘要。
- session 级队列与 cancel。

Claude Code 则通过 `QueryEngine` 把 conversation lifecycle 独立成更稳定的运行时对象：

- 一个 conversation 一个 `QueryEngine`。
- 每次 `submitMessage` 是一个 turn。
- turn 内管理 permission denials、usage、read file cache、messages、SDK status。
- 支持 headless/SDK/REPL 多入口复用。
- 支持 history snip/replay、compact boundary、partial messages。

Agent Builder 需要从当前 `SessionAgent.Run` 外围抽出独立 turn engine，而不是继续把所有能力塞进 agent 方法。

建议目标：

```text
Session -> Turn -> ToolCall -> PermissionRequest -> Event -> Audit
```

成为稳定 runtime spine。

### 2. 工具执行协议

Crush 当前工具是 Go 函数式工具集合，能力已经不少：

- bash/shell
- view/write/edit/multiedit
- grep/glob/ls
- web_fetch/web_search/sourcegraph
- MCP tools/resources/prompts
- todos
- diagnostics/LSP
- agent/agentic_fetch
- background shell job output/kill

但工具执行协议仍偏分散：工具自己验证、自己请求 permission、自己组织输出。

Claude Code 的 `Tool.ts` 和 `services/tools/toolExecution.ts` 更像统一 scheduler：

- tool schema
- validation
- permission check
- hook before/after
- progress event
- telemetry
- result storage / compression
- MCP server metadata
- error classification
- tool output budget
- SDK event queue

Agent Builder 应补一个中心化 tool scheduler。目标生命周期：

```text
requested
validated
policy_checked
approval_requested
running
streaming
completed
failed
cancelled
```

### 3. 权限与安全策略

Crush 当前权限模型在 `internal/permission/permission.go`：

- `Request`
- `Grant`
- `GrantPersistent`
- `Deny`
- `SkipRequests`
- `AutoApproveSession`
- allowed tools 简单 allowlist
- hook approval 可短路一次请求

这是一个可用的 prompt/approval 服务，但不是完整 policy engine。

Claude Code 权限体系明显更成熟：

- permission modes：default、plan、accept edits、bypass、auto 等。
- allow/deny/ask 规则。
- 多来源规则：CLI、session、user settings、project settings、policy settings。
- Bash/PowerShell 规则解析。
- dangerous permission 检测。
- auto mode classifier。
- hook decision。
- sandbox override。
- working directory policy。
- denial tracking。
- async agent 无交互场景 fail closed。

Agent Builder 的第一阶段不需要直接实现 classifier，但至少要补：

- permission mode
- allow/deny/ask rule
- tool capability metadata
- headless ask 行为
- plan/read-only mode
- shell command risk classification

### 4. Plan Mode

Crush 当前没有 Claude Code 级别 plan mode。Claude Code 的 plan mode 不是 UI 标签，而是 runtime policy：

- agent 进入 plan mode 后只能规划、读取、分析。
- 修改文件/执行危险动作需要 exit plan 或 approval。
- `prePlanMode` 记录进入前权限模式。
- `EnterPlanModeTool` / `ExitPlanModeTool` 让模型显式切换模式。
- UI 呈现 mode-specific permission request。

Agent Builder 应把 plan mode 作为第一批 policy mode，而不是前端状态。

### 5. Shell / PowerShell 安全

Crush 有 portable shell 和 background shell，适合做本地执行。但安全策略较粗。

Claude Code 在 shell 上有大量专门逻辑：

- Bash parser。
- PowerShell parser。
- destructive command warning。
- read-only validation。
- mode validation。
- path validation。
- git safety。
- command semantics。
- prefix/spec matching。
- subcommand results。
- output redirection 分析。
- sandbox 判断。

这块是 Codex/Claude Code 类客户端最关键的本地安全边界之一。Agent Builder 不宜一开始全量实现，但需要先建立 shell policy 层。

### 6. Subagent / Task

Crush 已有 `internal/agent/agent_tool.go`：

- `agent` tool。
- 构建 task agent。
- 创建子 session。
- 并行执行。

也有 `agentic_fetch` 这种特定用途子 agent。

但 Claude Code 的 AgentTool 已经远远超过“一次性子调用”：

- agent definitions。
- built-in/custom/plugin agents。
- frontmatter 配置。
- allowed tools / disallowed tools。
- skills。
- MCP server requirements。
- model/effort/permission mode。
- background。
- isolation：worktree/remote/cwd override。
- progress tracker。
- token/tool activity summary。
- output file。
- pending messages。
- resume metadata。
- agent transcript。
- team/swarm。

Agent Builder 需要把 subagent 从 tool call 升级成 task：

```text
AgentTask {
  id
  parent_session_id
  child_session_id
  type/name/role
  prompt
  model
  allowed_tools
  cwd/worktree
  status
  progress
  result
  output_artifact
  audit_events
}
```

### 7. 上下文与记忆

Crush 当前 prompt assembly 在：

- `internal/agent/prompts.go`
- `internal/agent/prompt/prompt.go`

主要来源：

- prompt template
- working dir
- git status
- configured context paths
- skills XML
- MCP instructions

Claude Code 的上下文系统更完整：

- system context 与 user context 分离。
- git status snapshot。
- CLAUDE.md 层级加载。
- managed/user/project/local memory。
- `.claude/rules/*.md`。
- `@include` 指令。
- frontmatter path globs。
- HTML comment stripping。
- memory file cache。
- read file state cache。
- nested memory attachment。
- session memory scheduling。
- agent memory snapshot/sync。

Agent Builder 应优先补“分层指令文件加载”，这比复杂 memory 更基础。

建议先支持：

```text
managed:  organization/system instructions
user:     user global instructions
project:  AGENTS.md / CLAUDE.md / .agents/rules/*.md
local:    AGENTS.local.md / CLAUDE.local.md
```

### 8. Session Persistence / Recovery

Crush 使用 SQLite 保存 session/message/todos，结构更数据库化。

Claude Code 使用 JSONL transcript 与 sidecar metadata：

- transcript parent chain。
- progress 不参与 chain。
- compact boundary。
- subagent transcript。
- remote agent metadata。
- worktree session metadata。
- file history snapshot。
- attribution snapshot。
- resume/replay。

Crush 的 SQLite 是优势，但需要补足 equivalent concepts：

- turn event log
- tool call log
- permission log
- compact boundary
- subagent metadata
- resumable task state
- artifact/output storage

不要照搬 JSONL，但要吸收它的 replay/recovery 语义。

### 9. API / Transport

Crush 已有 `internal/server` workspace API，Agent Builder 又新增了 `internal/runtimeapi` 和桌面 local HTTP/SSE。

Claude Code transport 更复杂：

- structured IO。
- SDK control protocol。
- permission control request/response。
- SSE/WebSocket/hybrid transports。
- remote session manager。
- bridge/direct connect。
- upstream proxy。

Agent Builder 当前方向正确：

```text
React UI -> AgentRuntime -> Wails adapter / HTTP adapter -> Go RuntimeService -> Crush runtime
```

需要继续固化 HTTP + SSE API，让 Wails 只是 adapter。

### 10. 插件 / Skills / MCP

Crush 已经有：

- MCP tools/resources/prompts。
- skills discovery。
- hooks。
- config。

Claude Code 更完整：

- plugin loader。
- marketplace。
- plugin policy。
- plugin cache/versioning。
- plugin commands。
- plugin agents。
- plugin hooks。
- plugin skills。
- plugin MCP integration。
- managed plugins。
- startup checks。

Agent Builder 不需要先做 marketplace，但应该定义 capability package：

```text
capability package:
  tools
  MCP servers
  skills
  hooks
  agents
  policies
  UI metadata
```

### 11. UI 与客户端

Crush 原始 UI 是 TUI。Agent Builder 现在有 React/Wails，已经开始转向 Codex 类客户端。

当前 React 已有：

- chat workspace
- session sidebar
- model settings
- runtime status
- usage readout
- permission modal
- audit drawer
- runtime panels for skills/MCP/capabilities

Claude Code 的 UI 是终端 REPL，但状态机更深：

- message list virtualization。
- permission-specific UI。
- diff UI。
- agent progress panel。
- team status。
- task notification。
- resume conversation。
- bridge/remote dialogs。
- keybinding/vim state。

Agent Builder 的 UI 不应照搬终端 UI，而应该消费 runtime events，把 Codex 类桌面控制面做好。

## Crush 的优势

Crush 不是空白。它已经具备很多合适的底座：

- Go 实现，适合本地优先 runtime。
- `backend/app/agent/session/message/db` 分层清楚。
- 多 provider 抽象已经存在。
- tools、MCP、skills、hooks、SQLite、LSP、TUI 都已有。
- Agent Builder 已经把 Wails/React 接到真实 Crush backend。
- `internal/runtimeapi` 已开始定义 transport-neutral contract。

这些是值得保留和强化的部分。

## 迁移原则

不要直接把 Claude Code 搬进 Crush。应该把 Claude Code 拆成 runtime primitives，再逐步用 Go/Crush 风格实现：

```text
Turn Engine
Tool Scheduler
Permission Policy
Context Memory
Agent Task
Isolation
Plugin Package
Advanced Client UI
```

每一步都要保持：

- TUI 不被破坏。
- Desktop client 通过 runtime API 消费状态。
- Go runtime 是 source of truth。
- React 不拥有核心业务状态。
- 所有高风险动作可审计。

## 建议落地路线

### Phase A: Turn Engine

目标：从 `SessionAgent.Run` 外围抽出稳定 turn model。

交付：

- turn id。
- turn status。
- turn event log。
- cancel/resume API。
- usage delta。
- latest assistant/tool state。

### Phase B: Tool Scheduler

目标：所有工具调用通过统一 scheduler。

交付：

- tool lifecycle。
- tool metadata。
- validation result。
- permission decision。
- progress event。
- audit event。

### Phase C: Permission Policy

目标：从简单 approval service 升级为 policy engine。

交付：

- permission mode。
- allow/deny/ask rules。
- plan/read-only behavior。
- headless ask behavior。
- tool capability risk metadata。

### Phase D: Context and Memory

目标：补齐 Claude Code 风格分层指令与记忆入口。

交付：

- managed/user/project/local memory。
- AGENTS.md / CLAUDE.md 层级加载。
- include。
- frontmatter path globs。
- read-file cache 基础。

### Phase E: Agent Task System

目标：把 subagent 从 tool call 升级为 task。

交付：

- agent definitions。
- allowed tools。
- background task。
- progress tracker。
- child session。
- output artifact。
- cancel/resume。

### Phase F: Worktree Isolation

目标：先用 git worktree 做高风险变更隔离。

交付：

- create/remove worktree。
- isolated cwd。
- branch metadata。
- task/worktree audit。
- merge/apply review path。

### Phase G: Runtime API Stabilization

目标：Wails 和 HTTP 共享同一 runtime service。

交付：

- stable HTTP endpoints。
- SSE event stream。
- token auth。
- API contract tests。
- desktop client 只依赖 `AgentRuntime`。

### Phase H: Capability Package

目标：统一 MCP、skills、hooks、agents、policies。

交付：

- package manifest。
- loader。
- enable/disable。
- source/version/risk metadata。
- UI capability panel。

## 当前优先级建议

短期不要先大改 UI。优先把下面这条链路稳定：

```text
Turn -> ToolCall -> Permission -> Event -> Audit
```

这条链路稳定之后，再做：

```text
Plan Mode -> Agent Task -> Worktree Isolation -> Memory -> Plugin Package
```

这样 Agent Builder 才会从“Crush 桌面壳”变成真正接近 Codex/Claude Code 的客户端。

