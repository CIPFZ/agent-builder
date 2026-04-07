# myclaw

`myclaw` 是一个用 Go 实现的 claw 实验项目，目标是把 `claude-code` 的 agent runtime 内核和 `openclaw` 的常驻控制面能力组合到一起。

第一版已经不是单纯的骨架，而是一个可运行的最小系统：

- WebSocket gateway / daemon
- 多轮 session transcript
- Claude-Code 风格的 tool loop
- 可配置权限模式
- 最小上下文压缩
- 最小 subagent 生命周期
- CLI 与 mock / OpenAI-compatible LLM 接入

这一版的重点是“内核先行，控制面留口”。也就是先把会话主循环、权限、压缩、subagent 跑通，再为长期监听、远程消息入口和多 agent 编排继续扩展。

## 建议目录

```text
myclaw/
├── cmd/
│   ├── myclaw/                # CLI 入口
│   └── myclawd/               # 守护进程 / gateway 入口
├── configs/
│   └── workspace/             # 本地工作区配置样例
├── docs/
│   ├── notes/                 # 每日源码对标笔记
│   └── plan/                  # 阶段规划、模块拆解
├── internal/
│   ├── agent/                 # agent loop、多代理协作
│   ├── app/                   # 应用装配、依赖注入
│   ├── cli/                   # /status /new /reset 等命令
│   ├── config/                # 项目配置与加载
│   ├── event/                 # 内部事件、消息分发
│   ├── gateway/               # WebSocket 网关与连接管理
│   ├── llm/                   # 模型接入、streaming 处理
│   ├── model/                 # 核心领域模型
│   ├── node/                  # node.list / describe / invoke
│   ├── prompt/                # system prompt、模板拼接
│   ├── protocol/
│   │   └── ws/                # WebSocket 消息协议
│   ├── runtime/               # 运行时编排
│   ├── sandbox/
│   │   └── docker/            # Docker 沙盒执行
│   ├── session/               # Session 生命周期与路由
│   ├── store/                 # 持久化层
│   ├── tools/
│   │   ├── sessions/          # 跨会话工具
│   │   └── system/            # system.run 等宿主工具
│   └── workspace/             # ~/.openclaw/workspace 配置读取
├── pkg/
│   └── types/                 # 可复用公共类型
├── scripts/                   # 开发辅助脚本
└── testdata/                  # 测试数据
```

更详细的职责说明见 [docs/project-structure.md](/home/ytq/work/ai/myclaw/docs/project-structure.md)。

## 为什么这样拆

- `cmd/` 只放启动入口，避免 main 函数变成大泥球。
- `internal/` 承载真正业务逻辑，和你的学习阶段一一对应。
- `gateway`、`session`、`tools`、`sandbox` 分开，是为了后面做主会话 / 非主会话权限隔离时不互相污染。
- `prompt`、`workspace`、`llm` 独立出来，是为了第 2-3 天做 system prompt 注入和流式输出时更清楚。
- `node` 与 `tools` 分离，是为了区分“协议层能力描述”和“具体工具实现”。
- `docs/notes` 单独存在，方便你每天记录 openclaw 对标结论，而不是把学习笔记散落在 issue 或聊天记录里。

## 阶段到目录的映射

### 阶段一：控制平面与 AI 大脑

- 第 1 天：`internal/gateway`、`internal/protocol/ws`
- 第 2 天：`internal/llm`、`internal/runtime`、`internal/agent`
- 第 3 天：`internal/prompt`、`internal/workspace`、`configs/workspace`

### 阶段二：会话管理与本地执行权限

- 第 4 天：`internal/session`、`internal/store`、`internal/model`
- 第 5-6 天：`internal/tools/system`、`internal/node`

### 阶段三：Docker 安全沙盒

- 第 7-8 天：`internal/sandbox`、`internal/sandbox/docker`
- 第 9-10 天：`internal/tools`、`internal/config`

### 阶段四：多代理协同与 CLI

- 第 11-13 天：`internal/tools/sessions`、`internal/agent`、`internal/event`
- 第 14 天：`cmd/myclaw`、`internal/cli`

## 下一步建议

如果你愿意，我下一步可以继续帮你补两样东西：

1. 初始化 `go.mod` 与最小可运行的 `cmd/myclaw` / `cmd/myclawd`。
2. 先把第 1 天的 WebSocket gateway 骨架写出来。

## 当前可运行入口

```bash
go run ./cmd/myclaw
go run ./cmd/myclaw version
go run ./cmd/myclawd
```

默认情况下，`myclawd` 会监听 `127.0.0.1:18080`，并提供：

- `/`
- `/healthz`
- `/statusz`
- `/ws`

`/ws` 当前支持的基础请求包括：

- `connect`
- `send_message`
- `spawn_subagent`

系统会通过事件流返回：

- `hello`
- `message.created`
- `assistant.delta`
- `tool.called`
- `tool.result`
- `run.error`
- `subagent.completed`

## 第一版能力

### 1. Agent Runtime 内核

- 会话消息会写入 transcript
- 模型可以发起 tool call，运行时会执行工具后再继续回答
- 当前内置了 `text.upper` 与 `system.run`
- subagent 已经具备最小 spawn / wait / result 回传能力

### 2. 安全准入模型

当前支持三种权限模式：

- `ask`：敏感工具直接拦下，要求批准
- `workspace-write`：仅工作区根目录内自动放行，越界则要求批准
- `danger-full-access`：完全放行

环境变量：

```bash
MYCLAW_PERMISSION_MODE=workspace-write
MYCLAW_PERMISSION_WORKSPACE_ROOTS=/abs/path/one;/abs/path/two
MYCLAW_PERMISSION_RULES_JSON=[{"tool_name":"system.run","action":"deny","match":{"command_contains":["rm -rf"]}}]
```

如果没有显式配置 `MYCLAW_PERMISSION_WORKSPACE_ROOTS`，daemon 会默认把 `configs/workspace` 当作工作区根。
`MYCLAW_PERMISSION_RULES_JSON` 用于补充更细的 allow/deny 规则，当前支持：

- `tool_name`
- `action`
- `match.command_contains`
- `match.workdir_prefixes`

### 3. 上下文压缩

第一版已经有最小 compaction 入口：

- transcript 超过阈值后会压缩旧消息
- 压缩结果会以 `summary` 消息写回 session 历史
- 新一轮 prompt 会在压缩后的历史上继续运行

这还不是完整的 `claude-code` 级别压缩体系，但已经把接口和主流程打通了，后续可以继续扩成 token 估算、分段摘要、session memory、autoDream。

当前已经接入最小 `session memory`：

- compaction 生成 `summary` 后会同步写入 memory service
- runtime 会发出 `memory.saved` 事件
- 这为后续长期记忆抽取和 autoDream 留好了主流程挂点

## 真实模型接入

当前支持通过环境变量接入一个 OpenAI 兼容接口。默认示例配置为 LongCat 兼容端点。

```bash
$env:MYCLAW_LLM_API_KEY="your_key_here"
$env:MYCLAW_LLM_BASE_URL="https://api.longcat.chat/openai/v1/chat/completions"
$env:MYCLAW_LLM_MODEL="LongCat-Flash-Chat"
go run ./cmd/myclawd
```

如果没有设置 `MYCLAW_LLM_API_KEY`，系统会自动回退到内置 mock client。

## LLM Config File

`myclaw` 现在支持通过配置文件管理 LLM 厂商、API Key 和模型参数。

默认读取：

`configs/myclaw.json`

可以直接从这个示例复制：

`configs/myclaw.example.json`

最小示例：

```json
{
  "llm": {
    "provider": "openai-compatible",
    "base_url": "https://api.longcat.chat/openai/v1/chat/completions",
    "api_key": "${MYCLAW_LLM_API_KEY}",
    "model": "LongCat-Flash-Chat"
  }
}
```

说明：

- `myclaw` 和 `myclawd` 都会读取 `configs/myclaw.json`
- 配置文件中的 `${ENV_VAR}` 会自动展开
- 环境变量仍然可以覆盖配置文件
- 当前先支持 `openai-compatible`

如果你想改默认位置，可以设置：

```bash
MYCLAW_CONFIG_FILE=/abs/path/to/myclaw.json
```

## 目录说明

核心目录仍然保持按职责拆分：

- `internal/gateway`：WebSocket control plane
- `internal/runtime`：请求运行时编排
- `internal/engine`：新的会话内核抽象
- `internal/permissions`：安全准入判定
- `internal/compaction`：上下文压缩
- `internal/agent`：subagent 生命周期
- `internal/session` / `internal/store`：session 与 transcript
- `internal/tools`：工具注册与执行

更细的目录职责见 [docs/project-structure.md](/Users/ytq/work/ai/myclaw/myclaw/docs/project-structure.md)。

## Recent Additions

- Session-level permission policy is now tracked inside the runtime instead of only using one global daemon mode.
- Subagents derive a safer permission mode by default:
  - `danger-full-access -> workspace-write`
  - `workspace-write -> ask`
  - `ask -> ask`
- You can override the child-agent mode explicitly with `MYCLAW_PERMISSION_SUBAGENT_MODE`.
- You can tune dangerous shell-command gating with `MYCLAW_PERMISSION_DANGEROUS_COMMANDS`.
- The control plane now supports `session_set_permission`, so a live session can be downgraded or tightened without restarting the daemon.
- `session_set_permission` supports `cascade_subagents`, which propagates the updated parent policy down to already existing child sessions.
- `session_status` now returns `permission_mode`, which makes the control plane aware of the current session safety level.
- `session_status` also returns `subagent_mode`, so the control plane can inspect how future child agents will be derived.
- `memory_list` now returns typed memory entries, including `summary`, `task`, and `instruction`.
- permission-denied tool requests now create approval records and emit `permission.required`.
- the control plane exposes pending approval requests through `approval_list`.
- approval records can now be updated through `approval_approve` and `approval_reject`.
- `approval_approve` now continues the previously blocked tool execution and lets the run finish through the normal tool/result/assistant flow.
- `approval_list` now supports filtering by approval status, and approval decisions emit `approval.updated` audit events.
- terminal approval records can now be cleaned up through `approval_clear`, which emits `approval.cleared`.
- subagent control flow now emits `subagent.updated` for steer/stop/resume operations.
- the control plane now supports `subagent_status` for per-run detail queries including control messages.
- runtime and gateway now support a lightweight orchestration hook, so key events can be forwarded into future coordinator logic without coupling that logic into the core loop.
- the server now keeps a built-in coordinator over those hook events, and exposes `orchestration_status` so run state can be queried per session.
- coordinator state now includes lightweight `dispatcher`, `reviewer`, and `executor` suggestions plus a `next_action`, giving the control plane a minimal institution-style view without forcing automatic decisions.
- `orchestration_status` now also exposes structured `recommended_role` and `recommended_action` fields so future coordinators can consume guidance programmatically instead of parsing text.
- coordinator snapshots now include a structured decision record: `decision_type`, `decision_reason`, and `auto_executable`.
- coordinator snapshots now also include `decision_priority`, so remote controllers can rank whether a run is just observational, needs review, or is sitting on a higher-urgency branch like approval/failure.
- explicit subagent control operations now also emit `orchestration.updated`, so remote controllers can react to orchestration-state changes without polling after steer/stop/resume.
- the control plane now supports `orchestration_history`, which returns the per-run decision timeline for audit, replay, and future policy learning.
- `orchestration_history` now supports `status` and `decision_priority` filtering, and returns a small `summary` block so remote controllers can quickly tell whether they are looking at a narrowed approval/failure slice or the full run timeline.
- the control plane now also exposes `orchestration_summary`, which aggregates current run count, status distribution, decision-priority distribution, and recommended-action distribution for a session.
- the control plane now also exposes `orchestration_evaluate`, which turns current coordinator state into a sorted list of structured suggestions for approvals, failures, monitoring, and close-out decisions without auto-executing anything.
- `orchestration_evaluate` now supports server-side filtering by `category`, `priority`, and `blocking_only`, so remote controllers can directly ask for just the actionable approval/failure slice instead of re-filtering client-side.
- this keeps the evaluator usable as a thin policy layer: the server ranks and filters suggestions, but still does not auto-execute them.
- the control plane now also exposes `orchestration_plan`, which turns filtered suggestions into an ordered step list and short summary so a human or remote controller can execute the plan deliberately.
- plan responses now include per-step `action_kind` and a plan-level `groups` summary, so controllers can quickly see whether a plan is mostly approvals, failures, reviews, or close-out work.
- plan steps now also carry a stable `action_id`, and plan responses include `priority_sections`, making it easier for remote controllers to reference a step and render the plan in high/medium/low lanes.
- the control plane now also exposes `orchestration_plan_step_update`, so a remote controller can persist human execution progress like `completed` and a short result note back onto a referenced plan step.
- `orchestration_plan_step_update` now emits `orchestration.plan_step.updated`, so step-level execution writes show up as their own audit event instead of only being visible on the next plan reload.
- plan step updates now apply basic state-transition guards, which prevents obviously unsafe jumps like taking a still-`blocked` step straight to `completed`.
- when a plan step is marked `completed`, later dependent steps now unlock automatically on the next `orchestration_plan` read instead of staying permanently `blocked`.
- the control plane now also exposes `orchestration_plan_step_history`, so execution-state writes for a step can be reviewed as a small audit timeline.
- `orchestration_plan_step_history` now supports filtering by `state`, and returns a small summary block with record count, per-state counts, and the latest state so controllers can inspect execution progress without re-aggregating client-side.
- the control plane now also exposes `orchestration_plan_overview`, which summarizes plan execution progress for a session with total steps, completed steps, blocking steps, percent complete, and per-state counts.
- `orchestration_plan_overview` now also includes `active_steps` and `terminal_steps`, so a control surface can quickly tell whether a session is still being worked, is waiting, or has mostly converged to terminal outcomes.
- `orchestration_plan_overview` now also exposes `failed_steps`, so failure volume can be consumed directly without recomputing it from `state_counts`.
- `orchestration_plan_overview` now also exposes `pending_steps` and `in_progress_steps`, so dashboards can separate queued blocking work from actively executing work without unpacking raw state buckets.
- `orchestration_plan_overview` now also exposes `ready_steps`, so immediately executable work can be surfaced directly instead of inferred from generic active-state counts.
- `orchestration_plan_overview` now also exposes `latest_ready_action` and `latest_in_progress_action`, so a control surface can tell apart the next runnable step from the step currently being worked without loading the full plan graph.
- `orchestration_plan_overview` now also exposes `latest_pending_action`, so approval-waiting or queued blocking work can be surfaced directly instead of being inferred from the broader blocked-action signal.
- both `orchestration_plan_overview` and `orchestration_plan_execution_history` now include a most-recent timestamp field (`last_updated_at` / `last_recorded_at`) so dashboards can refresh incrementally without diffing entire payloads.
- `orchestration_plan_execution_history` now also supports a `since` timestamp filter, so control surfaces can ask for only the execution records newer than their last checkpoint.
- `orchestration_plan_execution_history` now also supports an `until` timestamp filter, so the same API can be used for bounded replay windows as well as live incremental tails.
- invalid `until` timestamps are rejected with a structured websocket error, matching the existing `since` validation behavior.
- invalid `since` timestamps are rejected with a structured websocket error instead of silently returning an unfiltered history slice.
- the control plane now also exposes `orchestration_plan_execution_history`, which returns the session-wide plan-step execution timeline plus a small summary of total records, per-state counts, and per-action counts.
- `orchestration_plan_execution_history` now also supports filtering by `state`, and its summary includes `latest_active_action`, so a control surface can quickly focus on the currently active execution thread.
- `orchestration_plan_execution_history` summary now also includes `latest_completed_action` and `latest_failed_action`, so recent terminal outcomes can be surfaced directly without replaying the full execution timeline client-side.
- `orchestration_plan_execution_history` summary now also includes `latest_ready_action`, `latest_in_progress_action`, and `latest_pending_action`, so execution timelines can distinguish queued, runnable, and actively running work without recomputing those signals from raw records.
- `orchestration_plan_execution_history` summary now also includes `latest_terminal_action`, so a control surface can highlight the most recent terminal outcome without choosing between completed and failed records on the client.
- `orchestration_plan_execution_history` summary now also includes `latest_terminal_state`, so the client can tell whether that most-recent terminal outcome ended in `completed` or `failed` without looking up the underlying record.
- `orchestration_plan_execution_history` summary now also includes `latest_terminal_result`, so the newest terminal note/output can be surfaced directly alongside the latest terminal action and state.
- `orchestration_plan_execution_history` summary now also includes `latest_completed_result` and `latest_failed_result`, so the latest success/failure note can be shown directly next to the latest completed/failed action without replaying terminal records client-side.
- `orchestration_plan_execution_history` summary now also includes `latest_completed_state`, so the latest completed slot follows the same `action/state/at/result` shape as the other execution summary entries.
- `orchestration_plan_execution_history` summary now also includes `latest_failed_state`, so the latest failed slot follows the same `action/state/at/result` shape as the other execution summary entries.
- `orchestration_plan_execution_history` summary now also includes `latest_completed_at` and `latest_failed_at`, so control surfaces can timestamp the newest success/failure without scanning the raw execution timeline.
- `orchestration_plan_execution_history` summary now also includes `latest_terminal_at`, so the newest terminal transition can be timestamped directly without comparing completed/failed timestamps on the client.
- `orchestration_plan_execution_history` summary now also includes `latest_active_at`, so the freshest non-terminal step can be highlighted without replaying the execution history on the client.
- `orchestration_plan_execution_history` summary now also includes `latest_in_progress_at`, so the controller can timestamp the most recent actively running step without scanning raw records.
- `orchestration_plan_execution_history` summary now also includes `latest_ready_at`, so the controller can timestamp when the newest runnable step became ready without replaying raw history.
- `orchestration_plan_execution_history` summary now also includes `latest_pending_at`, so the controller can timestamp when the newest waiting/pending step entered that state without replaying raw history.
- `orchestration_plan_execution_history` summary now also includes `latest_active_result`, so the newest note/output from the current non-terminal step can be surfaced directly without replaying raw history.
- `orchestration_plan_execution_history` summary now also includes `latest_active_state`, so the controller can tell whether the freshest non-terminal step is still pending, already ready, or actively running without recomputing it from raw records.
- `orchestration_plan_execution_history` summary now also includes `latest_in_progress_result`, so the newest note/output from the actively running step can be surfaced directly without replaying raw history.
- `orchestration_plan_execution_history` summary now also includes `latest_in_progress_state`, so actively running work now has the same `action/state/at/result` shape as the rest of the execution summary.
- `orchestration_plan_execution_history` summary now also includes `latest_ready_result`, so the newest note/output attached to the most recently runnable step can be surfaced directly without replaying raw history.
- `orchestration_plan_execution_history` summary now also includes `latest_ready_state`, so the controller can treat the newest runnable step with the same state-rich shape used by the active and recorded summaries.
- `orchestration_plan_execution_history` summary now also includes `latest_pending_result`, so the newest note/output attached to the most recently waiting step can be surfaced directly without replaying raw history.
- `orchestration_plan_execution_history` summary now also includes `latest_pending_state`, so waiting work can be exposed with the same `action/state/at/result` shape used for the rest of the execution summary.
- `orchestration_plan_execution_history` summary now also includes `latest_recorded_action`, so incremental dashboards can tell which action changed most recently without replaying or diffing the full execution list.
- `orchestration_plan_execution_history` summary now also includes `latest_recorded_state`, so the client can tell what that most-recent write actually did without scanning the history slice again.
- `orchestration_plan_execution_history` summary now also includes `latest_recorded_result`, so the newest execution note/output can be surfaced directly alongside the latest recorded action and state.
- plan steps now also expose `phase` and `depends_on`, so the draft plan is no longer just ordered prose: it carries a minimal execution graph that remote controllers can honor while still keeping a human in the loop.
- plan steps now also expose an initial execution `state` (`pending`, `blocked`, `ready`), which gives later human-control flows a stable place to attach progress without changing the plan schema again.
- plans now also expose `phase_sections`, and each step carries `result` plus `updated_at`, so later execution records can attach to the existing schema instead of forcing another shape change.
- this gives the plan schema enough room for future human-control execution tracking without needing to redesign the orchestration draft format again.
