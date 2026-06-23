# Reference Project Comparison

本文基于以下四份分析文档整理横向结论：

- [Agent Builder](./agent-builder.md)
- [Claude Code](./claude-code.md)
- [Codex](./codex.md)
- [Gemini CLI](./gemini-cli.md)

目标不是选择“最强项目”，而是决定如何基于 Agent Builder 快速落地一个面向企业
产品操作场景的 agentic operations client。

## 总体判断

Agent Builder 适合作为主开发底座。Claude Code、Codex、Gemini CLI 都不应直接替代
Agent Builder，而应作为 runtime 设计蓝图：

- Agent Builder 提供 Go runtime、provider、tools、MCP、skills、hooks、session、
  SQLite、pub/sub、TUI 和初步 client/server 边界。
- Claude Code 展示了成熟 agentic CLI 如何把工具、权限、plan mode、
  子代理、后台任务、上下文压缩和恢复语义组合成完整执行协议。
- Codex 展示了更清晰的 core API、typed protocol、app-server、sandbox、
  permission profile、subagent-as-thread 和多客户端边界。
- Gemini CLI 展示了 policy engine、central scheduler、declarative tools、
  extension packaging、MCP 生命周期、subagent registry 和 OpenTelemetry。

因此后续路线应该是：

```text
Agent Builder 作为代码底座
  + Claude Code 的 runtime 产品语义
  + Codex 的 core protocol / app-server / sandbox 思路
  + Gemini CLI 的 scheduler / policy / extension / event stream 思路
```

## 横向能力对比

| 能力面 | Agent Builder | Claude Code | Codex | Gemini CLI | 对我们的启发 |
| --- | --- | --- | --- | --- | --- |
| 实现语言 | Go | TypeScript/Bun | Rust + TS/Python | TypeScript/Node | 继续用 Agent Builder/Go 做底座，参考其他项目设计。 |
| 产品形态 | TUI/CLI，初步 client/server | CLI/TUI + SDK/remote | TUI/exec/app-server/MCP/SDK | CLI/headless/ACP/A2A | 客户端应建立在 headless runtime API 上。 |
| Provider | fantasy 多 provider | Anthropic 为主，多 provider 分支 | OpenAI Responses 为主 | Gemini 为主 | 保留 Agent Builder 的 provider 优势，新增模型能力层。 |
| Session | SQLite session/message | JSONL transcript + rich metadata | thread store + SQLite/log DB | JSONL chat recording + event trajectory | Agent Builder 应扩展 operation/run 状态，而不是替换 SQLite。 |
| Tool | Go tool registry + fantasy | 工具即执行协议 | typed tools + central orchestrator | declarative tools + scheduler | 需要在 Agent Builder 中引入 central scheduler/orchestrator。 |
| Permission | 本地 approval + allow list | mode/rule/plan/danger detection | permission profile + sandbox/approval | policy engine + TOML rules | Agent Builder 需要从 approval 升级到 policy。 |
| Hooks | PreToolUse | 多处 hooks + tool/runtime 语义 | hook runtime before tool/permission | BeforeAgent/AfterAgent/BeforeTool 等 | hooks 应成为 runtime extension point。 |
| MCP | 已支持 stdio/http/sse | first-class MCP + OAuth/resource/tool | connection manager + approvals | MCP manager + restrictive merge | Agent Builder MCP 基础好，但需补生命周期与来源治理。 |
| Skills | SKILL.md discovery/injection | skills with metadata/tool hints | core-skills + provenance/policy | skills + activation tool + extensions | skills 应逐步具备 allowed tools、model、paths、hooks。 |
| Plugins | MCP/skills/hooks 组合，没有包格式 | plugin package 很宽 | marketplace/plugin roots | extension package 很完整 | 先稳定 primitives，后做插件包。 |
| Subagents | read-only task agent | AgentTool + background + isolation | subagents as threads | invoke_agent + registry + local/remote | 先把子 agent 变成持久任务，再做 teams。 |
| Scheduler | 较弱 | AgentTool/task 语义强 | tools parallel/orchestrator | Scheduler 很清晰 | Agent Builder 需要任务/工具调度层。 |
| Sandbox | 基本无 OS sandbox | sandbox/worktree/remote/cwd 分离 | 多平台 sandbox | full-process + tool-level sandbox | 先做 tool-level sandbox 和 worktree，避免一步到位。 |
| API 边界 | workspace/server 已有雏形 | QueryEngine/structured IO/remote | JSON-RPC app-server | AgentProtocol/event stream | Agent Builder 应优先稳定 runtime API。 |
| Observability | logs/PostHog/pubsub | rich telemetry/cost/tool metrics | tracing/otel/log DB | OpenTelemetry + evals | operation audit/event log 是必须项。 |

## Agent Builder 的优势

Agent Builder 的最大优势是适合继续演进为本地 runtime：

- Go 实现，部署和长期运行比 TS/Python 更可控。
- 代码结构已经服务化：`internal/app`、`internal/agent`、
  `internal/session`、`internal/message`、`internal/permission`、
  `internal/hooks`、`internal/skills`、`internal/workspace`。
- SQLite/sqlc 已经存在，适合继续扩展 operation/run、approval、audit、
  artifact 等表。
- `workspace.Workspace` 和 `internal/server` 已经提供 client/runtime 分离雏形。
- MCP、skills、hooks、LSP、tools 已经存在，不需要从零搭建扩展体系。
- 当前 TUI 可以继续作为一个客户端，而不是阻碍 headless runtime 演进。

## Agent Builder 的主要短板

对目标产品来说，Agent Builder 当前短板集中在 runtime 治理层：

- 没有 durable operation/run/task 状态机。
- permission 还停留在本地工具 approval 和 allow list。
- 没有 mode-aware policy engine。
- 没有 central tool scheduler/orchestrator。
- subagent 主要是 read-only task agent，没有完整生命周期和调度。
- 没有生产级 sandbox、worktree isolation 或网络/secret 边界。
- client/server API 还偏 TUI/编码会话，不是面向 operation client 的协议。
- audit 不是一等模型，消息和工具结果还不能完整表达操作链路。
- 插件还不是可分发、可治理、可审计的 capability package。

这些短板不意味着 Agent Builder 不适合。相反，它们正是后续改造清单。

## 应该借鉴 Claude Code 的部分

Claude Code 最值得借鉴的是 runtime 产品语义：

- tool 不是函数注册表，而是受权限、上下文、UI、transcript 和恢复语义共同
  约束的执行协议。
- permission mode 不只是 UI 开关，plan mode 应该是运行时权限状态。
- AgentTool 应该支持角色、模型、上下文范围、后台运行、隔离方式和 cwd。
- background task 应该可见、可切换、可恢复。
- worktree、remote、cwd override、sandbox 是不同 isolation 语义，不能混为
  一个布尔值。
- 上下文压缩应当是生命周期机制，包括 tool-result 压缩、summary、session
  memory 和必要附件回注。

不建议照搬的部分：

- Anthropic 产品耦合、feature gate、订阅策略和 telemetry。
- 大型全局 singleton 状态。
- 过宽的插件能力一次性开放。

## 应该借鉴 Codex 的部分

Codex 最值得借鉴的是 core protocol 和执行边界：

- core runtime 使用 submission queue + event queue，适合 TUI、API、SDK 和
  headless 客户端共用。
- JSON-RPC app-server 按 `thread/*`、`turn/*`、`mcp/*`、`plugin/*`、
  `skills/*` 等能力分组，边界清晰。
- central tool orchestrator 统一处理 approval、sandbox、retry、network
  denial、hook、telemetry。
- subagents as threads，复用同一套 session/turn/tool/permission/event 机制。
- permission profile 比简单 allow/deny 更适合企业场景。
- sandbox 应作为 tool execution 的一等决策。

不建议照搬的部分：

- OpenAI Responses API 中心化 provider 设计。
- 过大的 app-server 和产品账户体系。
- 多套持久化系统同时作为权威状态。
- 一开始就追求完整跨平台 sandbox parity。

## 应该借鉴 Gemini CLI 的部分

Gemini CLI 最值得借鉴的是 scheduler、policy 和 extension：

- DeclarativeTool 将 schema、validation、confirmation、execution、display、
  model response 分开。
- Scheduler 负责工具批处理、并发、policy、hooks、confirmation、live update、
  sandbox expansion、retry 和 final response。
- Policy engine 用规则表达 allow/deny/ask，支持 mode、tool、MCP、subagent、
  command prefix/regex、interactive/headless 等维度。
- extension package 可同时携带 MCP、commands、skills、agents、policies、
  hooks、settings。
- MCP manager 能处理 trust folder、admin allowlist、extension/user config
  merge 和 context refresh。
- OpenTelemetry 覆盖 startup、model、tool、policy、hook、subagent、compression。

不建议照搬的部分：

- Gemini-specific provider 和 Node 启动/sandbox 逻辑。
- 复杂 policy 一次性全部引入。
- extension 安装生态过早产品化。
- JSONL 作为主要状态存储。

## 改造优先级

### P0：建立 runtime 边界

第一阶段目标是让 Agent Builder 从 TUI-first 变成 runtime-first：

- 梳理并稳定 `workspace.Workspace` / `internal/server` 作为客户端边界。
- 定义 machine-readable event stream。
- 明确 session、turn、message、tool call、permission request 的协议模型。
- 保留 TUI 作为现有客户端，同时为未来桌面端/API 客户端预留稳定接口。

### P1：引入 operation/run 模型

企业产品操作不能只用 chat session 表达。需要新增 operation/run：

- run id、title、goal、status、risk level。
- step/task 状态。
- tool call 关联。
- approval 关联。
- artifact 和报告。
- checkpoint 和 rollback plan。
- final summary。

消息 transcript 仍是证据，但 operation event log 应成为操作审计主线。

### P2：集中 tool scheduler/orchestrator

把工具执行统一收敛到一层：

- 参数验证。
- hooks。
- policy。
- permission request。
- sandbox/worktree/环境选择。
- live progress。
- cancellation。
- retry。
- output truncation。
- telemetry。
- model-visible result。
- audit event。

这一步是后续权限、沙箱、插件和 agent teams 的基础。

### P3：权限升级为 policy

当前 permission service 可以保留，但需要演进：

- mode：plan/read-only/default/auto-edit/bypass/headless。
- rule：tool、agent、MCP server、resource、path、command、args pattern。
- decision：allow/ask/deny。
- scope：session/project/user/org。
- risk：read/write/destructive/secret/network。
- approval：是否需要用户确认或外部审批。

早期不需要完整 RBAC，但数据模型应预留 subject/resource/action。

### P4：子 Agent 与任务持久化

先不要追求复杂 agent teams，先让子 agent 成为可管理任务：

- parent session / child session。
- role/name/model。
- allowed tools / allowed MCP。
- cwd/worktree/isolation。
- status/progress/output。
- timeout/cancel/retry。
- final report。

之后再加动态创建 agent、并发调度和 agent teams。

### P5：沙箱与 workspace isolation

建议分阶段实现：

1. worktree isolation。
2. tool-level env/path/network policy。
3. command risk classification。
4. scoped permission expansion。
5. 容器或 OS sandbox。

不要一开始就追求 Codex/Gemini 的完整跨平台 sandbox。

### P6：插件包与企业 capability

先稳定 primitives：

- built-in tools。
- MCP server。
- skills。
- hooks。
- policy。
- agent definition。

之后再定义 plugin/capability package，把这些能力打包、签名、启用、禁用、
审计和分发。

## 推荐实现路线

```text
阶段 1：Runtime API 与事件流
  - 明确 session/turn/tool/permission/event 协议
  - 梳理 server/client 边界
  - TUI 继续跑在现有能力上

阶段 2：Operation/Run 审计模型
  - 新增 operation/run 表和事件表
  - 将工具调用、approval、artifact、report 关联到 run

阶段 3：Tool Scheduler
  - 集中 tool call lifecycle
  - 接入 hooks、permission、output、telemetry、audit

阶段 4：Policy Engine
  - mode-aware allow/ask/deny
  - 支持 headless、plan、default、bypass 等模式

阶段 5：Subagent Task
  - 子 agent 变成持久任务
  - 支持 role、tool scope、context scope、取消和结果汇总

阶段 6：Sandbox / Worktree / Plugin Package
  - 按风险逐步增强
```

## 最终结论

继续使用 Agent Builder 作为底座是正确的。它已经具备足够多的 agent runtime 基础，
而且 Go 技术栈符合本地优先、低环境依赖和长期服务化的方向。

Claude Code、Codex、Gemini CLI 共同证明：真正难的不是聊天 UI，而是
runtime 的执行治理。后续 Agent Builder 的核心改造应围绕：

- runtime API。
- operation/run 状态。
- tool scheduler。
- policy/permission。
- subagent task。
- sandbox/workspace isolation。
- plugin/capability package。
- audit/event/observability。

这些能力稳定后，桌面客户端、企业产品插件和 agent teams 才有可靠基础。
