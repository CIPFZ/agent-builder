# Error Recovery 机制完整实施方案

本文承接 `docs/organize/` 的项目梳理，以及对 `cc-haha`、`DeepSeek-GUI`、`myclaw/claude-code` 的 error recovery 对比结论。

目标不是照搬 Claude Code 的 transcript-first 恢复实现，而是在 Agent Builder 当前架构中吸收成熟机制：

- Go runtime / SQLite 继续作为恢复事实来源。
- React、Wails、HTTP/dev transport 只消费 runtime DTO 和 action metadata。
- Runtime events 只作为刷新触发和审计线索，不作为前端业务状态。
- 不自动重放可能有副作用的旧 tool call。
- 发现旧的不合理结构时直接删除、重构和替换，不保留迁移兼容层。
- 恢复动作必须结构化、可审计、可回放诊断。

## 总体目标

把 Agent Builder 的 error recovery 从当前的“启动后保守收尾”升级为完整的四层恢复体系：

```text
Durable Recovery
  启动时结构化收尾 stale turn / tool call / task / hook / permission / MCP / worktree。

Request Hygiene
  每次 provider 请求前修复非法 history，避免 tool_use/tool_result、空消息、重复 ID 等协议错误永久锁死会话。

Continuation Recovery
  对 interrupted turn 提供显式继续动作，创建新 turn 并保留 resume metadata，不复活旧 turn。

Reactive Recovery
  对 context length、rate limit、network、auth、model capability 等可分类错误执行有限 retry、compact retry 或进入可恢复状态。
```

最终产品表现：

- 用户能在 UI 中看到当前 workspace 是否有可恢复项。
- 用户能对 interrupted turn 执行 `continue`、`mark done`、`discard`、`resume checkpoint`。
- provider 请求不会因为历史中一个坏 tool pair 永久失败。
- prompt-too-long 能自动触发 compact/retry，失败时给出明确恢复建议。
- 所有恢复动作都有 runtime event、audit、run transition 和测试覆盖。

## 当前基线判断

### 已有能力

Agent Builder 当前已经有比较好的 durable recovery 基础：

- `internal/runtime/runtime_lifecycle.go`
  - `ensureWorkspaceStarted` 启动 runtime 后执行统一恢复。
- `internal/runtime/runtime_turn_store.go`
  - `InterruptUnfinished` 将未完成 turn 标记为 `interrupted`。
- `internal/runtime/runtime_tool_calls.go`
  - `cancelUnfinishedRuntimeToolCalls` 将未完成 tool call 标记为 `cancelled`。
- `internal/runtime/runtime_agent_task_store.go`
  - `InterruptUnfinished` 将未完成 AgentTask 标记为 `interrupted`。
- `internal/runtime/runtime_hooks.go`
  - `InterruptRunning` 将 running hook 标记为失败/中断。
- `internal/runtime/runtime_permissions.go`
  - `reconcilePendingPermissions` / `expireInvalidPendingPermissions` 清理失效 pending permission。
- `internal/runtime/runtime_mcp_request_store.go`
  - `CancelActionableOnStartup` 清理 stale MCP request。
- `internal/runtime/runtime_recovery.go`
  - `RecoveryStatus` 对外暴露恢复状态。
- `internal/runtime/runtime_turns.go`
  - `MarkInterruptedDone` 将 interrupted turn 显式转为 cancelled。
- `internal/runtime/runtime_runs.go`
  - `ResumeRunCheckpoint` 从 checkpoint 创建新 turn 继续。
- `internal/agent/agent.go`
  - `preparePrompt` 已有孤儿 tool result 过滤和孤儿 tool call synthetic result 注入。

这些能力应保留，但需要重构成更清晰的 recovery domain，而不是继续把恢复逻辑散落在 lifecycle、turn、run、agent prompt 中。

### 主要缺口

当前缺口按优先级排序：

1. Request hygiene 覆盖不完整。
   - 已处理 orphan tool result 和 orphan tool call。
   - 未系统处理 duplicate tool_use id、duplicate tool_result、空 assistant、thinking-only 残片、provider-specific block reorder、unsupported media recovery。
2. Interrupted continuation 只有 checkpoint resume 和 mark done。
   - 没有一等 `ResumeInterruptedTurn`。
   - 没有区分 `interrupted_prompt`、`interrupted_generation`、`interrupted_tool_call`、`interrupted_external_request`。
3. Reactive provider recovery 不成体系。
   - 没有统一错误分类。
   - prompt-too-long 没有自动 compact/retry 闭环。
   - rate limit/network/auth/fallback 处理分散在 provider 或下层库中，runtime 不可观测。
4. 前端 recovery 可见性不足。
   - `RecoveryStatus` 后端已有，但 UI 没有完整 recovery center。
   - 用户很难理解哪些项可继续、哪些项已取消、哪些项需要人工处理。
5. 事件和审计语义需要统一。
   - 目前已有 event/audit，但恢复动作类型没有完整枚举和 contract。
   - run transition 与 recovery action 之间缺少统一来源标识。

## 参考项目取舍

### 借鉴 Claude 系列

应借鉴的成熟能力：

- transcript resume 中断识别的分类思想。
- `ensureToolResultPairing` 式 request-boundary hygiene。
- prompt-too-long 时 compact/truncate/retry 的闭环。
- API retry 的错误分类和 bounded retry。
- 恢复后向模型提供明确 continuation intent。

不应照搬的实现：

- 不用 JSONL transcript 作为真相源。
- 不通过静默插入普通用户消息来自动续跑。
- 不自动复活旧 turn 或重放旧 tool call。
- 不让前端从消息文本推断恢复状态。

### 借鉴 DeepSeek-GUI / Kun

应借鉴：

- append-only log 的“稳定历史不被请求优化改写”原则。
- send-time hygiene 不回写 persisted canonical history。
- bounded tool result / argument compaction。
- inflight tracker 必须在 `finally` 清理。

结合 Agent Builder 的落点：

- canonical history 仍为 runtime/message/tool call 数据。
- request hygiene 产物只用于 provider request，可记录 diagnostics，但默认不改写原始消息。
- 如果需要修复持久化状态，必须通过显式 recovery action 和 audit 记录。

### 借鉴 cc-haha

应借鉴：

- bridge/session stale binding 清理思想。
- websocket/adapter stale message guard。
- provider block reorder 针对 Bedrock/Claude 类工具块排序问题。
- unsupported image/model 错误后的降级 retry。

结合 Agent Builder 的落点：

- Wails/HTTP/dev adapter 不应维护权威会话绑定。
- stale adapter message 只能触发 refresh，不改变 runtime 状态。
- provider block reorder 放在 request hygiene，而不是 UI 或 adapter。

## 目标架构

新增 `internal/runtime/recovery` 概念边界，但不拆 Go module。推荐以 runtime 包内文件组织开始，避免过早包拆分。

```text
internal/runtime/
  runtime_recovery.go                  现有 status 入口，保留并扩展
  runtime_recovery_actions.go          新增恢复动作：resume interrupted、discard、retry recoverable error
  runtime_recovery_classification.go   新增中断和错误分类
  runtime_recovery_events.go           新增 event/audit/action metadata helper
  runtime_recovery_store.go            如需持久化 recovery records，再新增 store

internal/agent/
  history_hygiene.go                   从 agent.go 拆出 provider request hygiene
  history_hygiene_test.go              覆盖所有非法 history 场景
  provider_recovery.go                 provider 错误分类和 retry hints

internal/runtime/
  runtime_context_recovery.go          prompt-too-long -> compact/retry 运行时编排
```

原则：

- 先重构已有 `preparePrompt` 中的 hygiene 逻辑到独立纯函数，再扩展行为。
- 先让 runtime action 清晰，再接前端。
- 不增加旧字段兼容映射；如果 DTO 命名不合理，直接统一为 camelCase view model 和 snake_case runtime JSON 的明确边界。
- 测试先覆盖纯函数和 store，再覆盖 runtime service flow，最后覆盖 frontend adapter。

## 数据和契约设计

### RuntimeRecoveryStatus 扩展

当前 `RuntimeRecoveryStatus` 保留，但字段命名应在新前端 view model 中统一映射为 camelCase。

后端结构建议扩展：

```go
type RuntimeRecoveryStatus struct {
	RuntimeStartedAt   string                     `json:"runtime_started_at"`
	LastEventSequence  int64                      `json:"last_event_sequence"`
	ActiveTurns        []RuntimeTurn              `json:"active_turns"`
	InterruptedTurns   []RuntimeRecoveredTurn     `json:"interrupted_turns"`
	RecoverableErrors  []RuntimeRecoverableError  `json:"recoverable_errors,omitempty"`
	InterruptedTasks   []RuntimeAgentTask         `json:"interrupted_tasks,omitempty"`
	CompactBoundaries  []RuntimeCompactBoundary   `json:"compact_boundaries,omitempty"`
	Worktrees          []RuntimeWorktree          `json:"worktrees,omitempty"`
	HookExecutions     []RuntimeHookExecution     `json:"hook_executions,omitempty"`
	PendingPermissions []RuntimePermissionRequest `json:"pending_permissions"`
	PendingMCPRequests []RuntimeMCPRequest        `json:"pending_mcp_requests,omitempty"`
	Actions            []RuntimeRecoveryAction    `json:"actions,omitempty"`
	SnapshotRequired   bool                       `json:"snapshot_required,omitempty"`
}
```

新增类型：

```go
type RuntimeRecoveredTurn struct {
	RuntimeTurn
	InterruptionKind string                  `json:"interruption_kind"`
	ResumeEligible   bool                    `json:"resume_eligible"`
	DiscardEligible  bool                    `json:"discard_eligible"`
	MarkDoneEligible bool                    `json:"mark_done_eligible"`
	Reason           string                  `json:"reason,omitempty"`
	ResumeHint       string                  `json:"resume_hint,omitempty"`
	OpenToolCalls    []RuntimeToolCall       `json:"open_tool_calls,omitempty"`
	Checkpoints       []RuntimeRunCheckpoint  `json:"checkpoints,omitempty"`
}

type RuntimeRecoverableError struct {
	ID             string                 `json:"id"`
	Kind           string                 `json:"kind"`
	Severity       string                 `json:"severity"`
	SessionID      string                 `json:"session_id,omitempty"`
	TurnID         string                 `json:"turn_id,omitempty"`
	RunID          string                 `json:"run_id,omitempty"`
	Provider       string                 `json:"provider,omitempty"`
	Model          string                 `json:"model,omitempty"`
	Message        string                 `json:"message"`
	RetryEligible  bool                   `json:"retry_eligible"`
	CompactEligible bool                  `json:"compact_eligible"`
	UserAction     string                 `json:"user_action,omitempty"`
	Details        map[string]any         `json:"details,omitempty"`
	CreatedAt      string                 `json:"created_at"`
}

type RuntimeRecoveryAction struct {
	ID          string   `json:"id"`
	Label       string   `json:"label"`
	Kind        string   `json:"kind"`
	SessionID   string   `json:"session_id,omitempty"`
	TurnID      string   `json:"turn_id,omitempty"`
	RunID       string   `json:"run_id,omitempty"`
	CheckpointID string `json:"checkpoint_id,omitempty"`
	Destructive bool     `json:"destructive,omitempty"`
	StartsWorker bool    `json:"starts_worker,omitempty"`
	Evidence    []string `json:"evidence,omitempty"`
}
```

`RuntimeTurn` 原结构仍可用于普通 turn 响应，但 recovery status 中应使用 `RuntimeRecoveredTurn`，避免前端自己推断是否可继续。

### 新增恢复动作 API

HTTP route：

```text
GET  /v1/recovery/status
POST /v1/recovery/turns/{turn_id}/resume
POST /v1/recovery/turns/{turn_id}/discard
POST /v1/recovery/errors/{error_id}/retry
```

Wails bridge：

```go
RecoveryStatus(ctx context.Context) (RuntimeRecoveryStatus, error)
ResumeInterruptedTurn(ctx context.Context, turnID string, req RuntimeResumeInterruptedTurnRequest) (RuntimeTurnResponse, error)
DiscardInterruptedTurn(ctx context.Context, turnID string) (RuntimeTurnResponse, error)
RetryRecoverableError(ctx context.Context, errorID string) (RuntimeRecoveryRetryResponse, error)
```

Runtime service interface：

```go
ResumeInterruptedTurn(context.Context, string, RuntimeResumeInterruptedTurnRequest) (RuntimeTurnResponse, error)
DiscardInterruptedTurn(context.Context, string) (RuntimeTurnResponse, error)
RetryRecoverableError(context.Context, string) (RuntimeRecoveryRetryResponse, error)
```

Request DTO：

```go
type RuntimeResumeInterruptedTurnRequest struct {
	Mode      string            `json:"mode,omitempty"` // continue, retry_last_step, summarize_and_continue
	Prompt    string            `json:"prompt,omitempty"`
	Metadata  map[string]string `json:"metadata,omitempty"`
}
```

规则：

- `ResumeInterruptedTurn` 必须创建新 turn。
- 新 turn 的 user message metadata 必须包含：
  - `recovery_source_turn_id`
  - `recovery_action=resume_interrupted_turn`
  - `recovery_mode`
  - `recovery_interruption_kind`
- 旧 turn 保持 `interrupted` 或转为 `cancelled` 要明确：
  - 默认保持 `interrupted`，并记录已 resume 到新 turn。
  - 用户执行 `MarkInterruptedDone` 或 `DiscardInterruptedTurn` 才转 `cancelled`。
- 不允许复用旧 requestID。
- 不允许重新执行旧 unfinished tool call。

### InterruptionKind

新增标准枚举：

```text
interrupted_prompt
interrupted_generation
interrupted_tool_call
interrupted_external_request
interrupted_permission
interrupted_mcp
interrupted_hook
unknown
```

分类来源：

- turn status/error
- unfinished/cancelled tool calls
- pending permission
- pending MCP requests
- hook execution
- latest message role/content
- run transition

分类逻辑放在 `runtime_recovery_classification.go`，不放到前端。

## 实施阶段

## 阶段 0：契约冻结和旧结构清理

### 目标

统一 recovery 的语义和入口，先移除不合理的隐式推断，避免后续继续堆兼容层。

### 后端任务

文件：

- `internal/runtime/runtime_service_types.go`
- `internal/runtime/runtime_contract_types.go`
- `internal/runtime/runtime_recovery.go`
- `internal/runtime/runtime_http.go`
- `internal/runtimeapi/contract.go`
- `desktop/runtime_bridge.go`
- `desktop/runtime_bridge_test.go`

任务：

1. 在 `RuntimeService` 接口加入新增 recovery action。
2. 在 `runtime_contract_types.go` 增加新的 DTO。
3. 在 `runtimeapi/contract.go` 增加新增 HTTP endpoint。
4. 在 `runtime_http.go` 增加 route。
5. 在 `desktop/runtime_bridge.go` 增加 Wails bridge 方法。
6. 删除或重构任何只为旧 recovery 展示服务的临时字段。
7. 确认 `runtime_recovery.go` 不再只返回裸 `RuntimeTurn`，而是返回带 action eligibility 的结构。

### 前端任务

文件：

- `client/src/runtime/workbenchTypes.ts`
- `client/src/runtime/wailsWorkbenchAdapter.ts`
- `client/src/runtime/staticWorkbenchAdapter.tsx`

任务：

1. 新增 `RecoveryStatusViewModel`、`RecoveredTurnViewModel`、`RecoverableErrorViewModel`。
2. adapter 只接收 runtime recovery DTO，不从 messages/timeline 推断 recovery 状态。
3. 删除旧的临时 recovery display 逻辑；如果诊断面板仍直接根据 interrupted text 展示，改为消费 view model。

### 测试

- `internal/runtime/runtime_http_test.go`
- `desktop/runtime_bridge_test.go`
- `client/src/runtime/*.test.ts`

验收：

- HTTP contract test 覆盖新 recovery endpoints。
- Wails bridge 转发测试通过。
- 前端 adapter 能 hydrate 空 recovery、interrupted turn、recoverable error 三种状态。

## 阶段 1：Request Hygiene 重构和补齐

### 目标

把 provider 请求前的 history 修复从 `agent.go` 中拆成纯函数管线，并补齐 Claude 系成熟场景。

### 后端任务

新增文件：

- `internal/agent/history_hygiene.go`
- `internal/agent/history_hygiene_test.go`

修改文件：

- `internal/agent/agent.go`
- `internal/agent/agent_test.go`

任务：

1. 将以下函数从 `agent.go` 移到 `history_hygiene.go`：
   - `filterOrphanedToolResults`
   - `syntheticToolResultsForOrphanedCalls`
   - `filterFileParts`
2. 新增 `PrepareProviderHistory` 或 `ApplyHistoryHygiene` 纯函数：

```go
type HistoryHygieneOptions struct {
	SupportsImages bool
	Strict         bool
	Provider       string
	Model          string
}

type HistoryHygieneResult struct {
	Messages    []fantasy.Message
	Diagnostics []HistoryHygieneDiagnostic
	Changed     bool
}

type HistoryHygieneDiagnostic struct {
	Kind       string `json:"kind"`
	MessageID  string `json:"message_id,omitempty"`
	ToolCallID string `json:"tool_call_id,omitempty"`
	ToolName   string `json:"tool_name,omitempty"`
	Action     string `json:"action"`
	Reason     string `json:"reason,omitempty"`
}
```

3. 覆盖以下 hygiene rules：
   - drop empty assistant messages with no text, reasoning, or tool calls。
   - drop orphan tool_result。
   - inject synthetic error tool_result for orphan tool_call。
   - remove duplicate tool_result for same tool_call_id after first valid result。
   - dedupe duplicate tool_use IDs in same provider request。
   - ensure assistant tool_use is followed by tool_result before next user-visible turn。
   - remove thinking-only assistant fragment if it cannot be paired into valid provider message。
   - strip file/image parts when model does not support images。
   - provider-specific reorder：把 assistant content 中 tool_use block 移到 text block 后的合法位置，保持 tool_use 相对顺序。
4. `sessionAgent.preparePrompt` 只负责：
   - 加 system reminder。
   - 调用 hygiene pipeline。
   - 组装 attachments。
5. 如果发现旧测试依赖非法 history 原样发送，删除旧断言并改成新 hygiene 行为。

### 诊断落库

第一版不强制落库每条 hygiene diagnostic，避免污染 canonical history。

但是当发生 synthetic tool_result 或丢弃 orphan block 时，需要：

- `slog.Warn` 保留。
- 在当前 turn diagnostics 中暴露摘要。

可选新增：

- `runtime_prompt_assemblies` 中保存 `history_hygiene_diagnostics` JSON。

### 测试

新增测试场景：

- orphan tool_result 被删除。
- orphan tool_call 被 synthetic error result 补齐。
- duplicate tool_result 被去重。
- duplicate tool_use id 被处理。
- empty assistant 被删除。
- thinking-only assistant fragment 被删除。
- unsupported image model 会剥离历史图片。
- provider block reorder 保持 tool_use 相对顺序。
- strict mode 下非法 pairing 返回错误，用于测试/训练数据路径。

验收：

- `go test ./internal/agent` 通过。
- 任意非法 tool pairing 不会进入 provider request。

## 阶段 2：Interrupted Turn 继续能力

### 目标

提供 Agent Builder 自己的一等 continuation recovery：显式新 turn 继续，而不是复活旧 turn 或静默插入普通用户消息。

### 后端任务

新增文件：

- `internal/runtime/runtime_recovery_actions.go`
- `internal/runtime/runtime_recovery_classification.go`
- `internal/runtime/runtime_recovery_events.go`

修改文件：

- `internal/runtime/runtime_turns.go`
- `internal/runtime/runtime_runs.go`
- `internal/runtime/runtime_run_transition_writer.go`
- `internal/runtime/runtime_recovery.go`

任务：

1. 实现 `classifyInterruptedTurn(ctx, turn RuntimeTurn) RuntimeRecoveredTurn`。
2. 实现 `ResumeInterruptedTurn`：
   - 校验 turn 存在且 status 为 `interrupted`。
   - 计算 interruption kind。
   - 生成 resume prompt。
   - 创建新 Chat turn。
   - link old turn -> new turn。
   - 写 runtime event。
   - 写 audit。
   - 写 run transition。
3. resume prompt 生成规则：

```text
You are resuming an interrupted Agent Builder turn.

The previous turn was interrupted before it completed.
Continue from the last reliable state. Do not assume unfinished tool calls succeeded.
If a missing tool result is needed, rerun the relevant read-only or safe operation.
If an operation may have side effects, ask for confirmation or inspect state first.
```

如果用户提供 `Prompt`，追加为：

```text
User recovery instruction:
{prompt}
```

4. 实现 `DiscardInterruptedTurn`：
   - 只能作用于 `interrupted` turn。
   - 将状态转为 `cancelled`。
   - reason 为 `interrupted_turn_discarded`。
   - 写 event/audit/run transition。
5. `MarkInterruptedDone` 保留，但语义收窄为“已知无需继续，按完成处理为 cancelled”。
6. `ResumeRunCheckpoint` 与 `ResumeInterruptedTurn` 共享 action metadata helper，但保持 API 独立。

### 持久化

优先使用现有表：

- `runtime_turns`
- `runtime_events`
- `runtime_audit`
- `runtime_run_transitions`

如果需要 link old/new turn，新增 migration：

```sql
CREATE TABLE runtime_recovery_links (
  id TEXT PRIMARY KEY,
  source_turn_id TEXT NOT NULL,
  resumed_turn_id TEXT NOT NULL,
  action TEXT NOT NULL,
  mode TEXT NOT NULL,
  interruption_kind TEXT NOT NULL,
  created_at TEXT NOT NULL
);
CREATE INDEX idx_runtime_recovery_links_source_turn ON runtime_recovery_links(source_turn_id);
CREATE INDEX idx_runtime_recovery_links_resumed_turn ON runtime_recovery_links(resumed_turn_id);
```

不要把 link 信息塞进 message 文本。

### 测试

- interrupted prompt resume 创建新 turn。
- interrupted tool call resume 不重新执行旧 tool call。
- resume 后 source turn 仍可审计。
- discard 将 interrupted turn 转 cancelled。
- 非 interrupted turn 调用 resume/discard 返回错误。
- run projection 包含 recovery transition。

验收：

- 用户能通过 API 对 interrupted turn 显式继续。
- 旧 turn 和新 turn 关系可查询、可审计。

## 阶段 3：Reactive Provider Recovery

### 目标

对 provider/API 错误做分类和有限恢复，避免把可恢复错误直接变成最终失败。

### 后端任务

新增文件：

- `internal/agent/provider_recovery.go`
- `internal/agent/provider_recovery_test.go`
- `internal/runtime/runtime_context_recovery.go`

修改文件：

- `internal/agent/agent.go`
- provider 调用相关文件，按实际代码位置调整。
- `internal/runtime/runtime_turns.go`
- `internal/runtime/runtime_compact*.go`

错误分类：

```text
context_length_exceeded
rate_limited
overloaded
network_transient
auth_expired
model_not_found
model_capability_unsupported
permission_required
policy_denied
provider_fallback_available
unknown
```

新增类型：

```go
type ProviderErrorClassification struct {
	Kind            string
	Retryable       bool
	CompactEligible bool
	RefreshAuth     bool
	FallbackEligible bool
	UserAction       string
	RetryAfter       time.Duration
	Details          map[string]any
}
```

任务：

1. 统一 provider error classifier。
2. rate limit / overloaded / network transient 做 bounded retry。
3. retry 必须有最大次数和最大 wall-clock 限制。
4. auth/model/config 错误不自动 retry，进入 recoverable error。
5. fallback 只在 provider/model 配置明确允许时触发。
6. 所有 retry attempt 写入 diagnostics，不写成普通 assistant message。

### Prompt-too-long / Compact Retry

流程：

```text
provider request
  -> context_length_exceeded
  -> classify as compactEligible
  -> create compact boundary / compact summary
  -> rebuild provider history
  -> re-run request once or bounded times
  -> success: turn completed with recovery diagnostics
  -> fail: turn failed with recoverable error
```

实现要求：

- compact retry 不删除 canonical messages。
- compact boundary 必须进入 `RuntimeRecoveryStatus.CompactBoundaries` 或 turn diagnostics。
- retry 次数默认 1，最多 2。
- 如果 compact 本身失败，原 turn 标记为 recoverable failed，不继续盲目 retry。
- 如果 provider 返回明确 max_tokens 问题，可先降低 output token budget，再 compact。

### Unsupported Media Recovery

规则：

- 如果 provider 明确返回 model 不支持 image/file part：
  - 先重建 history，剥离历史 media part。
  - 当前用户附件如果是必需输入，不自动丢弃，返回 recoverable error。
  - 历史图片降级必须写 hygiene diagnostic。

### 测试

- context_length_exceeded 触发 compact retry。
- compact retry 成功后 turn completed。
- compact retry 失败后 recoverable error 可见。
- rate limit 按 retry-after 或 backoff 重试。
- network transient 重试 bounded 次。
- auth/model/config 错误不重试。
- unsupported historical image 可剥离重试。
- current prompt image 不支持时要求用户处理。

验收：

- 可恢复错误不会直接把会话永久锁死。
- 所有 retry 可在 diagnostics/audit 中解释。

## 阶段 4：Recovery Status 和事件审计统一

### 目标

让所有 recovery 行为都有统一事件、审计和 action metadata。

### 后端任务

文件：

- `internal/runtime/runtime_recovery_events.go`
- `internal/runtime/runtime_audit.go`
- `internal/runtime/runtime_run_transition_writer.go`
- `internal/runtimeapi/contract.go`

新增事件类型：

```text
recovery.status.changed
recovery.turn.resumed
recovery.turn.discarded
recovery.error.classified
recovery.retry.started
recovery.retry.completed
recovery.retry.failed
recovery.history_hygiene.applied
recovery.context.compact_retry_started
recovery.context.compact_retry_completed
recovery.context.compact_retry_failed
```

Action metadata：

```go
const (
	runtimeRecoveryActionResumeInterruptedTurn = "resume_interrupted_turn"
	runtimeRecoveryActionDiscardInterruptedTurn = "discard_interrupted_turn"
	runtimeRecoveryActionRetryRecoverableError = "retry_recoverable_error"
)
```

每个 action 必须声明：

- `Accepted`
- `Reason`
- `RefreshTargets`
- `Source.Kind`
- `Source.Action`
- `Source.StartsWorker`
- `Source.IdempotentBy`
- `Source.Evidence`

不要使用裸 string map 临时拼 action。

### 测试

- 每个 recovery action 都返回 action metadata。
- 每个 recovery action 都写 runtime event。
- 每个 recovery action 都写 audit。
- run transition history 能看到 recovery source。

验收：

- 前端能只靠 action metadata 决定刷新目标。
- audit export 能解释恢复链路。

## 阶段 5：Frontend Recovery Center

### 目标

把 recovery 从隐藏诊断变成一等产品能力，但前端不持有事实状态。

### 前端任务

新增文件：

- `client/src/features/recovery/RecoveryCenter.tsx`
- `client/src/features/recovery/RecoveryCenter.module.css`
- `client/src/features/recovery/RecoveryTurnCard.tsx`
- `client/src/features/recovery/RecoveryErrorCard.tsx`
- `client/src/features/recovery/RecoveryActions.tsx`

修改文件：

- `client/src/app/WorkbenchShell.tsx`
- `client/src/runtime/workbenchTypes.ts`
- `client/src/runtime/wailsWorkbenchAdapter.ts`
- `client/src/features/diagnostics/TurnDiagnosticsPanel.tsx`
- `client/src/features/timeline/Timeline.tsx`

UI 原则：

- 使用 Ant Design components 和 theme tokens。
- Recovery Center 是工作台内的诊断/行动面板，不做营销式页面。
- interrupted turn 用紧凑卡片展示：
  - session / turn
  - interruption kind
  - reason
  - open tool calls
  - checkpoint
  - actions
- recoverable error 用明确状态展示：
  - kind
  - provider/model
  - retry/compact eligibility
  - user action
- 按钮：
  - Continue：调用 `ResumeInterruptedTurn`
  - Mark done：调用 `MarkInterruptedDone`
  - Discard：调用 `DiscardInterruptedTurn`
  - Retry：调用 `RetryRecoverableError`
  - Resume checkpoint：调用现有 `ResumeRunCheckpoint`

### Adapter 行为

- 初始 hydration 加载 `RecoveryStatus`。
- runtime event 命中 `recovery.*`、`turn.*`、`run.*`、`permission.*`、`mcp.*` 时刷新 recovery。
- Vite/browser dev 下必须走 HTTP/dev transport fallback。
- Wails binding 不可用时不报错卡死 UI。

### 删除旧 UI

如果存在以下旧逻辑，直接删除或重构：

- 根据 assistant error text 推断 interrupted recovery。
- timeline 中用普通 message row 伪装 recovery action。
- diagnostics panel 内部自己拼 recovery status。

### 测试

- adapter hydrate recovery status。
- Recovery Center 空状态。
- interrupted turn card actions。
- recoverable error card actions。
- event 后刷新。
- Wails unavailable 时 HTTP fallback。

验收：

- 用户能从 UI 完成 continue / mark done / discard / retry。
- UI 不解析消息文本推断恢复状态。

## 阶段 6：Store、Run Projection 和 Diagnostics 加固

### 目标

让恢复链路在 run projection、turn diagnostics、audit export 中都能解释。

### 后端任务

文件：

- `internal/runtime/runtime_run_projection.go`
- `internal/runtime/runtime_turn_diagnostics.go`
- `internal/runtime/runtime_audit.go`
- `internal/runtime/runtime_replay.go`
- `internal/runtime/runtime_prompt_assemblies.go`

任务：

1. Run projection 增加 recovery section：
   - interrupted source turns
   - resumed turns
   - recovery transitions
   - retry attempts
   - compact retry boundaries
2. Turn diagnostics 增加：
   - history hygiene diagnostics
   - provider error classification
   - retry attempts
   - compact retry result
3. Audit export 增加 recovery event grouping。
4. Replay export 能复现恢复状态，不需要重放副作用 tool。

测试：

- run projection 包含 recovery links。
- diagnostics 包含 hygiene/retry/compact 信息。
- audit export 包含 recovery action。
- replay export 不要求重放 tool call 也能解释 interrupted 状态。

验收：

- 出现恢复问题时可以从 runtime 数据解释完整过程。

## 阶段 7：旧代码删除和结构收敛

### 目标

避免同时维护两套恢复路径。新机制完成后删除旧的不合理分支。

### 删除/重构清单

必须删除或替换：

- `agent.go` 中直接内联的 history hygiene 逻辑。
- 前端任何基于 message text 的 recovery 推断。
- 测试中为旧非法 history 行为保留的兼容断言。
- 零散拼接 recovery event/action metadata 的 helper。
- 任何把 resume link 写入普通消息文本的尝试。

需要保留但收窄语义：

- `MarkInterruptedDone`
  - 保留为用户确认无需继续。
- `ResumeRunCheckpoint`
  - 保留为 run checkpoint 恢复，不混同 interrupted turn resume。
- startup `InterruptUnfinished`
  - 保留为 durable recovery 第一层。

不做兼容：

- 不兼容旧前端字段名。
- 不保留旧 recovery UI 入口。
- 不为旧错误文本分类写 fallback parser。
- 不迁移历史中的非法 tool pairing；只在 request boundary 修复。

## 端到端验收场景

必须覆盖以下场景：

1. Runtime crash during assistant generation。
   - 重启后 turn 为 interrupted。
   - Recovery Center 显示可 continue。
   - continue 创建新 turn。
2. Runtime crash after assistant tool_use but before tool_result。
   - 重启后 tool call cancelled。
   - 请求下一轮时 synthetic tool_result 注入。
   - 不发生 provider pairing error。
3. Runtime crash during tool execution。
   - tool call cancelled。
   - resume 不自动重放 tool。
   - 模型收到明确 missing result 信息。
4. Pending permission during restart。
   - stale permission expired/cancelled。
   - RecoveryStatus 可见。
5. Pending MCP request during restart。
   - stale MCP request cancelled。
   - RecoveryStatus 可见。
6. Prompt too long。
   - provider error 被分类为 context_length_exceeded。
   - compact retry 执行。
   - 成功则 turn completed，失败则 recoverable error。
7. Rate limit。
   - bounded retry。
   - retry attempt 可诊断。
8. Unsupported historical image。
   - history hygiene 剥离历史 media。
   - 当前附件不支持时进入用户可处理错误。
9. Duplicate tool_use id。
   - request hygiene 修复或 strict mode 报错。
10. Duplicate/orphan tool_result。
   - request hygiene 修复。
11. Recovery event replay。
   - 刷新 UI 后 recovery status 与数据库一致。

## 测试命令

后端：

```text
go test ./internal/agent
go test ./internal/runtime
go test ./desktop
go test ./internal/runtimeapi
go test ./...
```

前端：

```text
cd client && npm run build
```

如存在前端测试命令，补充运行：

```text
cd client && npm test
```

Lint：

```text
task lint
```

## 推荐执行顺序

一次性纳入任务范围，但实现顺序建议如下：

```text
阶段 0：契约冻结和旧结构清理
  -> 阶段 1：Request Hygiene 重构和补齐
  -> 阶段 2：Interrupted Turn 继续能力
  -> 阶段 3：Reactive Provider Recovery
  -> 阶段 4：Recovery Status 和事件审计统一
  -> 阶段 5：Frontend Recovery Center
  -> 阶段 6：Store、Run Projection 和 Diagnostics 加固
  -> 阶段 7：旧代码删除和结构收敛
```

不要把阶段理解成“只做一部分”。本方案的所有阶段都属于同一轮 error recovery 目标，只是为了降低实现风险而排序。

## 非目标

本方案明确不做：

- 自动重放有副作用的 unfinished tool call。
- 将 Claude Code transcript recovery 原样搬到 Agent Builder。
- 前端本地保存 recovery 状态作为事实来源。
- 为旧 UI 或旧 DTO 保留长期兼容层。
- 把恢复提示隐藏在普通 assistant 文本里。
- 无上限 retry。

## 最终完成标准

完成后应满足：

- Runtime 能结构化识别和暴露所有中断状态。
- Provider 请求前 history hygiene 覆盖 Claude 系主要协议错误场景。
- Interrupted turn 能通过显式 action 创建新 turn 继续。
- Prompt-too-long 能 compact/retry。
- 可恢复 provider 错误有分类、retry 策略和用户动作。
- 前端有 Recovery Center，并且不解析消息文本。
- audit、runtime events、run projection、turn diagnostics 能解释恢复链路。
- 老的不合理 recovery 推断和重复逻辑已删除。
