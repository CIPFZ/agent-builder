# Claude Code 与 Go 复刻版全模块语义对比报告

日期：2026-04-26  
Go 复刻 worktree：`C:\Users\ytq\work\ai\agent-builder\.worktrees\claude-code-semantic-review`  
Go 复刻分支：`codex/claude-code-semantic-review`  
Claude Code 源码：`C:\Users\ytq\work\ai\agent-builder\claude-code`  
基准分支：`main @ 1e88b900f19b80a4d630bb7ef8ba32df2275f456`

## 结论摘要

当前 Go 复刻版已经不是空壳，具备一个可运行的 Claude Code 风格 runtime 内核：会话、QueryEngine、工具循环、权限审批、LLM provider、compaction、memory、subagent、MCP 局部能力、TUI 和 daemon/gateway 都有实现。

但如果目标是“1:1 复刻真实 Claude Code 的全模块、全文件、全语义行为”，当前仍处于早期到中期之间。按功能语义估算，Go 版本大约覆盖真实 Claude Code 的 15% 到 25%；按源码规模估算，Go 生产代码约 39,325 行，而 Claude Code `src/` 约 512,742 行，Go 生产代码规模约为 7.7%。规模不是唯一指标，但这里的差距与模块缺口基本一致。

最核心的差距不是 Go 代码质量，而是产品边界不同：真实 Claude Code 是一个 Anthropic-first 的完整 agent runtime、TUI/React Ink 应用、SDK/structured IO host、remote/bridge/transport 平台、插件/技能/MCP/LSP 扩展平台、telemetry/feature gate/policy 治理系统。Go 复刻版目前更像一个“最小可验证 Claude Code runtime 内核 + agent builder 起点”。

## 审阅范围与索引

本次对比覆盖：

- Go 复刻：`cmd/`、`internal/` 下所有 Go 源码与测试文件。
- Claude Code：`claude-code/src/` 下所有 `.ts`、`.tsx`、`.js` 文件，以及 `claude-code/docs/` 研究文档。
- 输出索引：`docs/_go_file_semantic_index.csv` 记录每个 Go 文件的行数与顶层定义；`docs/_claude_code_file_index.csv` 记录 Claude Code 每个源码文件的目录、路径、行数与扩展名。

注意：真实 Claude Code 源码体量约 1,902 个 TS/TSX/JS 文件，无法把每一行语义逐字复制到 Markdown 主报告中。这里采用“全文件索引 + 模块级语义审阅 + 关键行为逐项对照”的方式落地，避免把报告变成不可维护的源码转录。

## 规模对比

| 对象 | 文件数 | 行数 | 说明 |
| --- | ---: | ---: | --- |
| Go 生产代码 | 105 | 39,325 | `cmd/` + `internal/`，排除 `_test.go` |
| Go 测试代码 | 约 60 | 约 39k | 测试体量接近生产代码，说明很多能力是测试驱动补齐的 |
| Claude Code 源码 | 1,902 | 512,742 | `src/` 下 TS/TSX/JS |
| Claude Code 文档 | 48 | 未计入源码行 | 已有大量源码研究文档 |

Claude Code 最大目录：

| Claude 目录 | 文件数 | 行数 | Go 对应情况 |
| --- | ---: | ---: | --- |
| `utils` | 564 | 180,314 | 仅少量等价：permissions、workspace、sandbox、model/provider、filesystem 部分 |
| `components` | 389 | 81,853 | Go TUI 有基础替代，但不具备 React Ink 组件级等价 |
| `services` | 130 | 53,645 | Go 仅覆盖 LLM、MCP 局部、memory/session store，缺 analytics/policyLimits/remote services |
| `tools` | 184 | 50,806 | Go 有核心工具与部分 MCP/Skill/Agent，但缺 LSP、AskUserQuestion、Task v2、复杂 Bash/Edit 行为 |
| `commands` | 207 | 26,514 | Go 只实现少量 CLI/TUI 命令，slash command 体系远未等价 |
| `bridge` | 31 | 12,583 | Go gateway/WebSocket 是简化控制面，不等价于 bridge/remote/IDE |
| `cli` | 19 | 12,337 | Go daemon/gateway 不等价于 structured IO + transports |

Go 最大生产包：

| Go 包 | 文件数 | 行数 | 语义定位 |
| --- | ---: | ---: | --- |
| `internal/tools` | 15 | 10,056 | 工具抽象、文件工具、MCP、Skill、AgentTool、搜索工具 |
| `internal/tui` | 31 | 6,914 | Bubble Tea TUI 外壳和交互状态 |
| `internal/queryengine` | 4 | 4,937 | 会话主循环、工具调用、权限、hooks、MCP 注入 |
| `internal/llm` | 7 | 2,469 | Anthropic/OpenAI-compatible/provider routing |
| `internal/gateway` | 3 | 2,267 | HTTP/WebSocket 控制面 |
| `internal/runtime` | 5 | 1,838 | runtime runner、queue、session compaction、worktree |
| `internal/config` | 2 | 1,341 | 配置加载、环境变量展开、provider 配置 |
| `internal/permissions` | 2 | 924 | permission mode、rules、setup、policy evaluation |

## 架构语义总览

| 真实 Claude Code 语义 | Claude 关键源码/目录 | Go 复刻对应 | 覆盖判断 |
| --- | --- | --- | --- |
| 顶层应用装配、feature gate、模式分流 | `src/main.tsx`、`src/replLauncher.tsx` | `internal/app/bootstrap.go`、`internal/app/cli.go`、`cmd/myclaw` | 低。Go 有装配，但缺真实启动期 prefetch、auth、feature gate、plugin/MCP/LSP/remote 装配复杂度 |
| 会话主循环与工具回流 | `src/QueryEngine.ts` | `internal/queryengine/queryengine.go` | 中。核心循环已存在，但上下文、缓存、恢复、SDK 消息协议差距大 |
| Tool 协议 | `src/Tool.ts`、`src/tools.ts`、`src/tools/` | `internal/tools/registry.go`、`internal/tools/*` | 中低。抽象相似，具体工具行为缺口很大 |
| 权限与审批 | `src/types/permissions.ts`、`src/utils/permissions/` | `internal/permissions/*`、`internal/approval/*` | 中。模式/规则/审批已有，但远程 host、UI、managed policy、危险规则完整度不足 |
| TUI / React Ink UI | `src/components/`、`src/screens/`、`src/ink/` | `internal/tui/*` | 低。Go TUI 可验证 runtime，但不是组件级复刻 |
| Commands / slash commands | `src/commands.ts`、`src/commands/` | `internal/tui/commands.go`、`internal/app/cli.go` | 很低。真实命令数百个，Go 只有最小集合 |
| Session persistence / transcript | `src/history.ts`、`src/assistant/`、`src/state/` | `internal/session`、`internal/store`、`internal/model/claude_transcript.go` | 中低。Go 有持久化和 Claude transcript 模型，但缺完整历史投影、snip/replay、UI 状态恢复 |
| Context / CLAUDE.md / memory | `src/context.ts`、`src/memdir/`、`src/utils/*memory*` | `internal/workspace`、`internal/memory`、`internal/prompt` | 中低。基础存在，真实递归加载、缓存、团队记忆、agent memory 复杂度缺失 |
| Compaction | `src/services/compact/`、`src/utils/context` | `internal/compaction`、`internal/runtime/session_compaction.go` | 中。Go 有压缩策略与测试，但不等价于真实 history snip/replay/context cache |
| Subagent / tasks | `src/tools/AgentTool/`、`src/tasks/`、`src/commands/tasks` | `internal/agent`、`internal/tools/agent_tool.go` | 中低。Go 有生命周期和前后台雏形，缺内建 agent 组织、task UI、worktree/remote isolation 完整语义 |
| MCP / plugins / skills / LSP | `src/services/mcp`、`src/plugins`、`src/skills`、`src/services/lsp` | `internal/tools/mcp_*`、`skill_*`、`bundled_skills.go` | 低到中低。Go MCP/Skill 有实质雏形，但缺插件市场、LSP、完整 MCP client lifecycle |
| Structured IO / transports / SDK | `src/cli/structuredIO.ts`、`src/cli/transports`、`src/entrypoints/sdk` | `internal/gateway`、`internal/protocol/ws` | 很低。Go 有 WebSocket 控制面，不等价于 SDK host control protocol |
| Bridge / remote / IDE | `src/bridge`、`src/remote`、`src/upstreamproxy` | `internal/gateway`、`internal/app/daemon.go` | 很低。Go 没有真实 bridge、trusted device、remote bridge session |
| Provider routing / Anthropic client | `src/services/api`、`src/utils/model` | `internal/llm` | 中低。Go 支持 Anthropic 和 OpenAI-compatible，但真实 Claude Code 主要是 Claude provider family 和服务能力限制 |
| Telemetry / analytics / GrowthBook | `src/services/analytics` | 无完整对应 | 接近 0。Go 只有 diagnostics logger，缺 telemetry/feature experiment 治理 |
| Policy / managed settings | `src/services/policyLimits`、settings 相关 utils | `internal/config`、`internal/permissions` | 低。Go 有本地配置规则，缺企业/远程/托管设置 |

## 逐模块语义审阅

### 1. 启动与应用装配

Claude Code 的 `main.tsx` 是完整应用装配器：早期 profiling、MDM/keychain 预取、配置和鉴权、feature gate、plugins/skills/MCP/LSP、commands/tools、REPL/remote/bridge/assistant 模式分流都会在启动期被拼起来。

Go 复刻的 `internal/app/bootstrap.go`、`internal/app/cli.go`、`internal/app/daemon.go` 已经能装配配置、LLM、session、runtime、TUI、daemon/gateway，但语义上更接近“本地 runtime bootstrap”。缺口包括：

- 没有与真实 Claude Code 等价的 feature gate 和 GrowthBook 实验控制。
- 没有 MDM/keychain/auth/account/organization 级别启动依赖。
- 没有 plugins/LSP/remote/bridge 的启动期完整装配。
- `cmd/myclaw` 和 `cmd/myclawd` 是薄入口，不等价于 Claude 的多模式 CLI。

判断：启动链路覆盖约 20%。

### 2. QueryEngine 与主循环

Go 的 `internal/queryengine/queryengine.go` 是复刻中最接近 Claude Code 核心语义的部分。它已经实现：

- session message append 和 mutable message 维护。
- user/system context provider。
- model pass 与 tool call 循环。
- tool called / tool result / permission required 等事件。
- permission hook、pre/post tool hook、tool failure hook。
- approval required flow。
- compaction hook、max turns、usage estimate、MCP inventory、skill state recovery 等扩展点。

与 Claude Code `src/QueryEngine.ts` 相比，主要缺口：

- Claude 的 `mutableMessages`、`readFileState`、`discoveredSkillNames`、`loadedNestedMemoryPaths`、context cache、history snip/replay 语义更完整。
- Claude 的 QueryEngine 同时服务 REPL、SDK/headless、structured IO、remote host；Go 当前主要服务本地 TUI/runtime/gateway。
- Claude 对 streaming、partial SDK events、tool result block、usage、cache、telemetry 的类型语义更细。
- Go 的 `queryengine.go` 很大，很多行为集中在单文件，后续维护风险高；Claude 虽然也复杂，但周边服务拆分更细。

判断：核心 loop 覆盖约 40% 到 50%，周边状态治理覆盖约 20%。

### 3. Tool 协议与工具实现

Go 的 `internal/tools/registry.go` 已有一个较完整的工具协议：

- `Definition`、`Tool`、`StructuredTool`、`ContextualTool`、`ToolUseContext`。
- policy-aware invocation。
- permission checking interface。
- tool exposure/search/deferred/always-load。
- MCP resource/tool/prompt/skill 相关抽象。
- progress、notification、elicitation、conversation id 等回调。

这说明 Go 版本不是简单 function calling，而是在复刻 Claude Code “工具是受控执行协议”的核心思想。

缺口集中在具体工具语义：

- `Read`/`Write`/`Edit`/`MultiEdit`/`Glob`/`Grep`/`LS` 有实现，但不等价于 Claude 的完整 path normalization、partial read、read cache、diff rendering、notebook、权限提示、IDE diff 语义。
- `Bash`/`PowerShell`/`system.run` 有 shell 执行能力，但缺 Claude 的复杂 shell snapshot、persistent shell、sandbox/network policy、command classifier、background bash/task 语义。
- `Agent`/`Task` 有雏形，但 Claude 的 AgentTool 输入更丰富：`model`、`run_in_background`、`name`、`team_name`、`mode`、`isolation`、`cwd` 等。
- Go 有 `Skill` 和 `tool.search`，但真实 Claude 还有 local/remote/MCP skill、canonical skill、plugin command、allowed tools、hooks、shell/context/agent frontmatter 等复杂语义。
- Go 缺 LSP tool、AskUserQuestion、TaskCreate/TaskGet/TaskUpdate/TaskList、完整 WebFetch/WebSearch 行为、插件工具链、官方/built-in MCP 差异。

判断：工具协议抽象覆盖约 45%，具体工具行为覆盖约 20% 到 30%。

### 4. 权限、审批与安全边界

Go `internal/permissions/policy.go` 和 `setup.go` 覆盖了重要模式：

- `default`、`ask`、`acceptEdits`、`plan`、`auto`、`workspace-write`、`danger-full-access`、`bypassPermissions`、`dontAsk`。
- allow/ask/deny rules。
- workspace roots。
- dangerous command patterns。
- subagent safer mode。
- permission updates。
- plan mode 和 auto mode 冲突校验。

`internal/approval/manager.go` 则持有 approval request、tool metadata、decision reason、content blocks、status transition。

与 Claude Code 相比：

- Go 的 policy 是本地、同步、结构化的；Claude 还包含 settings/policy/managed config/CLI arg/session/command 多来源规则和 UI/SDK/remote approval 转发。
- Go 的 dangerous command 检测是字符串模式；Claude 有更复杂的安全分类、permission prompt tool、sandbox override、working directory reasoning。
- Claude 的 plan mode 是 UI、permission、file persistence、host approval 的组合；Go 的 plan mode 主要在 policy/tool app state 中体现。
- Go 缺企业 managed settings、policy limits、remote host permission prompt 和完整 persisted permission update 语义。

判断：本地权限内核覆盖约 40%，真实产品安全治理覆盖约 20%。

### 5. TUI 与用户交互

Go `internal/tui` 使用 Bubble Tea 实现了：

- model/state/update/render。
- input、history search、global search、quick open、picker、dialog。
- approval dialog。
- tool progress、tool expand、message actions。
- session resume、transcript search、prompt stash、external editor。

这说明 Go 复刻的 TUI 已经是可验证 shell，不是纯 CLI。

但真实 Claude Code 的 UI 层包含 `components`、`screens`、`ink`、大量 hooks 和 design-system，语义远超 Go TUI：

- React Ink 组件树、hooks、上下文 provider、AppStateStore、FPS/stats provider。
- 复杂 prompt input mode、paste/truncate、vim/keybindings、wizard、feedback、trust dialog、managed settings dialog。
- task/agent 前后台切换、structured diff、file permission dialog、MCP components。
- telemetry、feedback、feature hints、team/swarm/banner。

判断：Go TUI 是 runtime 验证面，非 1:1 UI 复刻；覆盖约 10% 到 20%。

### 6. Commands 与 slash commands

Claude `src/commands/` 有 207 个文件，覆盖 auth、plugins、mcp、memory、model、permissions、plan、review、resume、rewind、tasks、voice、vim、hooks、doctor、desktop、remote、usage 等。

Go 当前命令体系主要是：

- `cmd/myclaw` / `internal/app/cli.go` 的 CLI 分发。
- TUI 内部少量 slash command 行为。
- daemon status/health/websocket。

缺口非常大：

- 没有完整 `commands.ts` 可见性/过滤/alias/feature gate 注册表。
- 没有多数 slash command 的实现。
- 没有命令与 auth/provider/user type/remote/enterprise 的可见性联动。

判断：覆盖约 5% 到 10%。

### 7. Session、transcript、history、recovery

Go 具备：

- `internal/session`：session manager、message、recovery。
- `internal/store/file` 和 `internal/store/memory`。
- `internal/model/claude_transcript.go`：Claude transcript 结构。
- `internal/runtime/session_compaction.go`：session compaction snapshot。

缺口：

- Claude 的 history snip/replay/projected view、read file state、context cache、session memory scheduling 更复杂。
- Claude 的 transcript 与 UI/SDK/analytics/tool lifecycle 强耦合；Go 更偏本地 persistence。
- Go 的 recovery 有恢复能力，但缺完整 UI state、task state、plugin/skill state、remote state 恢复。

判断：基础持久化覆盖约 35%，真实长期会话治理覆盖约 20%。

### 8. Context、memory、CLAUDE.md

Go 的 `internal/workspace/loader.go`、`internal/prompt/builder.go`、`internal/memory/service.go` 已经能把 workspace、history、memory、system prompt 拼成模型上下文。

Claude 的语义更强：

- `CLAUDE.md` 加载、禁用、额外目录、递归/嵌套 memory、team memory、agent memory、drift prevention。
- read file state 与 context cache 会影响后续上下文预算。
- memory 不只是文本拼接，而是长期会话卫生和 agent 协作协议的一部分。

判断：覆盖约 25% 到 35%。

### 9. Compaction 与上下文预算

Go `internal/compaction/service.go` 是一个实质模块，支持：

- threshold / token budget 分析。
- compaction reason。
- summary message。
- memory save。
- dynamic model limits。
- replay/cleanup hook。

这部分与 Claude Code “上下文是稀缺资源”的设计方向一致。

缺口：

- Claude 的 session memory scheduling、history snip/replay、partial view、read cache mechanics 更完整。
- Go 的 token estimate 和 compaction 触发相对简化。
- 缺多 provider/model capability 下的完整 context window 策略。

判断：覆盖约 35% 到 45%。

### 10. Subagent、任务与隔离

Go `internal/agent/manager.go` 与 `internal/tools/agent_tool.go` 支持：

- spawn/list/status/wait/resume/steer/stop。
- running/completed/failed/stopped/closed 状态。
- child session id/key。
- structured AgentTask input 中的 `description`、`prompt`、`subagent_type`、`run_in_background`、`isolation`。
- executor injection。

缺口：

- Go manager 当前主要是内存管理；Claude 有后台任务、output file、foreground/background、UI `/tasks`、agent name/team registry。
- Go 没有真实内建 agent 类型和角色约束体系的完整复刻。
- Go `isolation` 字段存在，但没有完整 worktree/remote/cwd override 生命周期。
- Claude 的 agent 体系与 prompt、permissions、tools allowlist、memory、context economy 深度耦合。

判断：生命周期内核覆盖约 35%，真实多 agent 产品语义覆盖约 20%。

### 11. MCP、plugins、skills、LSP

Go 已有：

- MCP tool/resource/prompt 类型。
- MCP dynamic tool registration。
- MCP OAuth 雏形。
- MCP prompt skill / MCP skill command。
- local skill discovery/frontmatter/arguments。
- bundled skills。
- `tool.search` 与 Claude-like tool search。

缺口：

- Claude 的 plugin 体系、marketplace/install/reload/plugin commands、allowed tools、hooks、skill shell/context/agent 语义远更完整。
- Go 没有 LSP service/tool。
- MCP client lifecycle、server status、auth reconnect、resource template、elicitation/progress 只覆盖局部。
- Claude 中扩展能力与 commands/tools/UI/permissions/provider/analytics 交织，Go 仍偏工具层。

判断：MCP/Skill 抽象覆盖约 25%，插件/LSP 覆盖接近 0。

### 12. Structured IO、transport、SDK、remote/bridge

真实 Claude Code 不是单纯 REPL，它有：

- `src/cli/structuredIO.ts` 的 host control protocol。
- SSE/WebSocket/Hybrid transport。
- remote IO。
- SDK control schemas/types。
- bridge、trusted device、direct connect、upstream proxy、remote session。

Go 的 `internal/gateway` 和 `internal/protocol/ws` 提供 HTTP/WebSocket 控制面、status、session 操作、部分 orchestration/permission hook 测试，但这不等价于 Claude 的 structured IO 和 remote host protocol。

缺口：

- 没有 SDK-compatible stdin/stdout control protocol。
- 没有 SSE/Hybrid transport、retry/liveness/permanent rejection 语义。
- 没有 bridge/remote/trusted device/session runner。
- 没有 external host permission prompt forwarding 的完整协议。

判断：覆盖约 5% 到 15%。

### 13. LLM provider、模型解析与能力限制

Go `internal/llm` 支持：

- Anthropic client。
- OpenAI-compatible client。
- model catalog。
- provider config / env override / proxy。
- model resolution。

这与真实 Claude Code 有一个方向差异：Claude Code 的 provider 主要是 Claude family 的 `firstParty`、`bedrock`、`vertex`、`foundry`，并围绕 Anthropic API、betas、capabilities、policy limits 做治理。Go 则更像通用 LLM runtime，支持 OpenAI-compatible。

缺口：

- 缺真实 Claude provider family 的 capability restrictions。
- 缺 beta/feature/model support overrides 的完整策略。
- 缺 first-party services、policy limits、managed settings。
- 缺 API telemetry、request id、previous request id、usage/cache semantics。

判断：LLM 接入能力覆盖约 35%，Claude 产品 provider 语义覆盖约 15%。

### 14. Telemetry、analytics、feature gate

真实 Claude Code 有完整 telemetry/analytics/GrowthBook：

- Datadog 白名单和 first-party only。
- 1P event logging。
- `_PROTO_*` privileged fields。
- metadata 裁剪、tool input 限深限长。
- GrowthBook targeting attributes。
- API query/error metadata。
- 用户显式 feedback / bug report。

Go 只有 `internal/diagnostics/logger.go` 级别的 JSONL diagnostics，未见等价 telemetry、experimentation、feature rollout、provider reporting rules。

判断：覆盖接近 0。

## 关键缺口清单

| 优先级 | 缺口 | 为什么重要 |
| --- | --- | --- |
| P0 | 完整 command/slash-command 注册与可见性系统 | 真实 Claude 的产品能力大量由命令层暴露，Go 当前无法对齐用户操作面 |
| P0 | structured IO / SDK control protocol | 没有它，Go runtime 不能等价成为可被外部 host 接管的 agent runtime |
| P0 | 工具行为精确化：Bash、Read/Edit/Write、Todo、Agent、MCP | 当前抽象相似，但用户感知和模型行为取决于具体工具语义 |
| P1 | AppState / UI state / session recovery 一体化 | Claude 的 TUI/SDK/remote 都依赖持久可恢复状态 |
| P1 | CLAUDE.md / memory / read cache / context cache 完整语义 | 这是长会话质量和工程上下文正确性的核心 |
| P1 | subagent task background + worktree/remote isolation | 当前 agent 只是生命周期内核，还不是 Claude 的多执行单元系统 |
| P1 | plugins/skills/MCP/LSP 扩展平台 | Claude 的可扩展性不是单一工具注册表 |
| P2 | provider capability restrictions / feature gates | 影响模型选择、beta、auto mode、安全策略 |
| P2 | telemetry/analytics/diagnostics 分层 | 影响真实产品治理、实验、问题定位 |
| P2 | managed settings / enterprise / policy limits | 影响企业环境等价性 |

## “差多少”的量化判断

| 维度 | Go 当前成熟度 | 1:1 所需成熟度 | 差距 |
| --- | ---: | ---: | --- |
| runtime core | 45% | 100% | 仍需补上下文、恢复、SDK、复杂工具回流 |
| tool protocol | 45% | 100% | 抽象接近，具体工具行为差距大 |
| concrete tools | 25% | 100% | 缺大量工具、细节和 UI/permission 联动 |
| TUI | 15% | 100% | 目前是 Go 原生验证 UI，不是 React Ink 组件复刻 |
| command system | 8% | 100% | 缺绝大多数 slash commands |
| permissions | 40% | 100% | 本地内核存在，产品治理不足 |
| session/history/memory | 30% | 100% | 基础存在，长期会话语义不足 |
| subagents/tasks | 25% | 100% | 生命周期雏形存在，协作/隔离/任务 UI 不足 |
| MCP/skills/plugins/LSP | 20% | 100% | MCP/Skill 局部存在，plugins/LSP 大缺失 |
| SDK/remote/bridge | 10% | 100% | gateway 不能替代 structured IO/bridge |
| telemetry/feature gates | 0% | 100% | 基本未复刻 |

总体判断：按用户可见行为和源码语义综合，Go 复刻距离真实 Claude Code 1:1 仍差约 75% 到 85%。

## 建议路线图

1. 先定义“1:1”的验收边界：如果目标是复刻 Claude Code 产品，必须引入 command registry、structured IO、tool parity tests；如果目标是 agent-builder runtime，则不应追求全部 UI/telemetry/enterprise 语义。
2. 建立 parity test harness：用 Claude Code 源码文档中的关键行为为 golden spec，逐项生成 Go 测试，而不是继续靠手工判断。
3. 先补工具和命令：`Bash`、`Read/Edit/Write/MultiEdit`、`TodoWrite`、`Agent`、`Skill`、`MCP`、`/permissions`、`/model`、`/memory`、`/resume`、`/tasks`。
4. 再补上下文与恢复：CLAUDE.md 递归加载、read file state、context cache、history snip/replay、session transcript 兼容。
5. 最后补平台层：structured IO、transport、bridge/remote、telemetry、feature gate、managed settings。

## 验证记录

- 已从 `main` 创建 worktree：`C:\Users\ytq\work\ai\agent-builder\.worktrees\claude-code-semantic-review`。
- 已创建新分支：`codex/claude-code-semantic-review`。
- 已执行 `go mod download`：成功。
- 首次执行 `go test ./...`：124 秒超时，未得到通过结论。
- 复跑 `go test ./...`，超时设置 600 秒：exit 0；所有 Go 包通过或无测试文件。

## 附录文件

- `docs/_go_file_semantic_index.csv`：Go 全文件语义索引，含每个文件的行数与顶层定义。
- `docs/_claude_code_file_index.csv`：Claude Code 全源码文件索引，含目录、路径、行数和扩展名。
