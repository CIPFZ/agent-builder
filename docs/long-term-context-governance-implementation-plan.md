# Long-Term Context Governance Implementation Plan

## Purpose

This document defines the implementation plan for replacing Agent Builder's
current partial context compression behavior with a production-grade long-term
conversation governance system.

The target is not a small hardening pass. The target is a runtime-owned context
system that can support long engineering conversations, long-running tool loops,
session resume, compact/snip boundaries, prompt-too-long recovery, and clear UI
explainability.

This plan intentionally does not preserve compatibility with the current
incomplete compact behavior. If existing code conflicts with this design, delete
or rewrite it. Do not add shims for legacy compact semantics.

## Current Reality

The current implementation has useful pieces, but they are not yet a complete
long-term conversation system.

Implemented today:

- `internal/agent/tool_result_guard.go` applies single tool result truncation,
  disk persistence, and turn-level output budget spill.
- `internal/contextmgr/microcompact.go` records projection-level microcompact
  boundaries and content replacements without mutating canonical messages.
- `internal/agent/agent.go` no longer runs legacy auto summarize as the primary
  long-context mechanism.
- `internal/runtime/runtime_compact.go` is now limited to legacy compact
  boundary read APIs; new compact state is owned by `internal/contextmgr`.
- `internal/runtime/runtime_prompt_assembly.go` records prompt assembly
  diagnostics.
- `client/src/features/diagnostics/ContextDiagnosticsPanel.tsx` displays budget,
  context source, tool, and compact boundary diagnostics.

Important shortcomings:

- Runtime full compact records a boundary after a turn, but does not generate a
  real summary and does not replace the next model input history.
- Runtime micro compact updates tool call records after a turn, but does not own
  model input projection.
- Agent microcompact affects model input, but is not persisted as a durable
  projection or compact boundary.
- Legacy summarize is coarse and session-level. It is not a structured compact
  projection pipeline.
- There is no snip compact with projected view, replay, and runtime history
  reduction.
- There is no reactive compact retry path for prompt-too-long or context length
  provider failures.
- Tool result budget decisions are not persisted as stable per-tool-call
  replacement records for resume and prompt cache stability.
- Frontend compact visibility is mostly diagnostic-panel based. Compact
  boundaries are not first-class timeline events.

## Design Principles

- Runtime is the source of truth for context governance.
- React renders context state; it does not infer compression, budget, or compact
  boundaries from prose or local reducers.
- Every model call must pass through a single context input builder.
- The canonical session history, model input projection, and visible timeline
  are separate concepts.
- Tool-use and tool-result API invariants must never be broken by compression.
- Compact decisions must be durable, replayable, and explainable.
- Prompt-too-long errors should trigger runtime recovery before surfacing as
  user-facing failure.
- Context governance should be conservative by default and aggressively tested.
- Remove old incomplete compact paths instead of wrapping them with compatibility
  code.

## Target Architecture

```text
User input
  -> Runtime turn start
  -> Load canonical session history
  -> ContextManager.BuildModelInput()
       1. validate canonical history invariants
       2. reconstruct prior content replacement state
       3. apply per-message tool result budget
       4. apply microcompact
       5. apply active snip projection
       6. apply auto/full compact when threshold is reached
       7. reinject required context/read-file state
       8. assemble system prompt, messages, tools, context sources
       9. persist projection, budget, boundaries, warnings
  -> Agent model call
  -> Tool loop
  -> On prompt-too-long:
       ReactiveCompact()
       rebuild projection
       retry within bounded limits
  -> Persist events/audit/projection
  -> React hydrates from runtime DTOs
```

## New Backend Package

Create a new runtime package:

```text
internal/contextmgr/
  manager.go
  types.go
  model_input.go
  projection.go
  invariants.go
  budget.go
  tool_result_budget.go
  tool_result_refs.go
  microcompact.go
  compact.go
  compact_prompt.go
  snip.go
  reactive.go
  read_state.go
  store.go
  events.go
  test_fixtures.go
```

Keep package ownership narrow:

- `internal/contextmgr` owns model input projection and context governance
  decisions.
- `internal/runtime` owns transport DTOs, runtime events, audit, and service
  wiring.
- `internal/agent` streams model/tool execution, but should not own compact
  policy.
- `internal/message` remains canonical message persistence, not projection
  policy.

## Core Interfaces

Define the manager as the single model-input builder:

```go
type Manager interface {
    BuildModelInput(ctx context.Context, req BuildInputRequest) (BuildInputResult, error)
    ReactiveCompact(ctx context.Context, req ReactiveCompactRequest) (ReactiveCompactResult, error)
    ManualCompact(ctx context.Context, req ManualCompactRequest) (CompactResult, error)
    ManualSnip(ctx context.Context, req ManualSnipRequest) (SnipResult, error)
}
```

`BuildInputRequest` should include:

- workspace ID
- session ID
- turn ID
- model/provider metadata
- current user input
- canonical persisted messages
- current tool list and selected/omitted tool metadata
- context source inventory
- read-file state
- policy mode
- source query kind, such as main turn, compact, subagent, task
- compact configuration

`BuildInputResult` should include:

- model messages
- file attachments
- selected tool schemas
- projection DTO
- budget report before and after governance
- compact boundaries applied during this build
- snip boundaries applied during this build
- content replacement records
- read-state reinjection records
- warnings
- audit payload

## Canonical History, Projection, And Timeline

Implement three separate concepts.

### Canonical History

Canonical history is the durable session record:

- user messages
- assistant messages
- tool calls
- tool results
- permission evidence
- compact boundary markers
- snip boundary markers
- content replacement decisions
- read-file state snapshots

Canonical history should remain inspectable and auditable. Do not physically
delete user-visible history as the default compact behavior.

### Model Input Projection

Projection is what the next model call actually sees. It may contain:

- compact summary messages
- preserved recent messages
- replacement previews for large tool results
- cleared old tool results
- omitted/snipped middle history
- reinjected read-file/context state
- current user input

Projection must be persisted with enough metadata to reconstruct the exact
decision set after restart.

### Timeline View

Timeline view is what React renders. It should show:

- normal user/assistant/tool rows
- compact boundary rows
- snip boundary rows
- microcompact rows
- reactive compact retry rows
- tool result replacement indicators

React must not infer these rows. They come from runtime activity DTOs.

## Store And Schema

Do not migrate old compact data. Add new schema and use it for new sessions.

Recommended tables:

```text
runtime_context_projections
runtime_context_projection_messages
runtime_context_boundaries
runtime_context_content_replacements
runtime_context_snip_boundaries
runtime_context_read_state_snapshots
runtime_context_reinjections
runtime_context_warnings
runtime_context_reactive_attempts
```

Minimum fields for `runtime_context_projections`:

- id
- session_id
- turn_id
- step
- provider
- model
- source
- status
- canonical_message_count
- projected_message_count
- budget_before_json
- budget_after_json
- created_at
- completed_at
- error

Minimum fields for `runtime_context_boundaries`:

- id
- session_id
- turn_id
- projection_id
- kind: `micro`, `full`, `auto`, `manual`, `reactive`, `snip`
- trigger
- status
- summary_message_id
- summary_ref
- message_refs_json
- tool_call_refs_json
- reinjected_refs_json
- budget_before_json
- budget_after_json
- created_at
- completed_at
- error

Minimum fields for `runtime_context_content_replacements`:

- id
- session_id
- turn_id
- projection_id
- tool_call_id
- tool_name
- kind: `tool_result`
- original_ref
- replacement_text
- original_size_bytes
- original_estimated_tokens
- replacement_estimated_tokens
- reason
- created_at

Minimum fields for `runtime_context_snip_boundaries`:

- id
- session_id
- turn_id
- projection_id
- removed_message_refs_json
- preserved_head_ref
- preserved_tail_ref
- summary_ref
- reason
- created_at

## Tool Result Budget

Replace the current turn-counter-only budget with stable per-message aggregate
budgeting.

Requirements:

- Group tool results according to model API wire-message boundaries.
- Do not treat every persisted local message as an independent budget group if
  the provider will merge them.
- Persist replacement decisions by tool call ID.
- Reapply the same replacement byte-for-byte on resume.
- Once a result is seen and left inline, do not replace it later unless a
  higher-level compact/snippet boundary changes the projection.
- Persist full content to runtime refs before replacing it.
- Preserve image/media tool results with a separate policy.
- Allow tool-specific exemptions and limits.
- Expose replacement records to prompt assembly and UI diagnostics.

Replacement text should contain:

- original size
- persisted ref URI
- safe preview
- instruction to use a narrower read/view when detail is needed

Acceptance:

- A single huge tool result is replaced before the next model call.
- Multiple parallel tool results that merge into one API user message are
  budgeted together.
- Resume reconstructs the same replacements without reading persisted files.
- The model never receives a tool result without a matching tool use.

## Microcompact

Implement microcompact as projection-level context cleanup.

Triggers:

- time-based idle gap after last assistant message
- compactable tool result count threshold
- compactable tool result token threshold

Rules:

- Keep at least the most recent N compactable tool results.
- Only compact safe tool result content.
- Do not compact failed tool results unless policy explicitly allows it.
- Preserve original output through runtime refs.
- Record a microcompact boundary and content replacement records.
- Do not mutate canonical message content.

Compactable tool classes:

- shell output
- grep/glob/search output
- file read output
- web fetch/search output
- MCP resource reads when text-only and ref-backed

Acceptance:

- Old high-cost tool results are cleared from projection.
- Recent tool results remain available.
- Original output is recoverable through refs.
- Microcompact shows in context diagnostics and timeline.

## Full Compact

Implement full compact as real prompt-history replacement before model calls.

Triggers:

- manual compact
- auto compact threshold
- reactive compact fallback
- explicit runtime action

Flow:

1. Build compact input from canonical history or current projection.
2. Strip nonessential large attachments before summary generation.
3. Generate compact summary with the configured model.
4. Preserve recent API rounds verbatim.
5. Reinject read-file state, project context, skills, MCP instructions, active
   plan/task state, and selected durable refs.
6. Persist compact boundary, summary, projection, and budget delta.
7. Use the compacted projection for the current or next model call.

Summary prompt must preserve:

- current user objective
- files read or edited
- code snippets that are still load-bearing
- commands run and important outputs/errors
- tool decisions and permissions that affect future work
- todo/task state
- unresolved questions
- current work state immediately before compaction

Prompt-too-long during compact summary:

- retry by dropping the oldest API-round groups from the compact summary input
- keep the current task tail protected
- fail with a recorded compact error after bounded retries

Acceptance:

- After full compact, the next model call does not include the full old history.
- The compact summary is part of the model input projection.
- Preserved recent messages remain in valid tool-use/tool-result order.
- Read-file state needed for edits is reinjected or marked unavailable.

## Auto Compact

Auto compact must run before model calls, not only after turns.

Thresholds:

- effective context window
- output reserve
- warning buffer
- auto compact buffer
- blocking buffer

Recommended config:

```text
auto_compact_enabled: true
output_reserve_tokens: 16000
warning_buffer_tokens: 20000
auto_compact_buffer_tokens: 13000
blocking_buffer_tokens: 3000
max_consecutive_auto_compact_failures: 3
```

Guards:

- do not auto compact compact-summary calls
- do not auto compact session-memory/forked helper calls
- do not recurse during reactive compact
- honor disable flags for tests and recovery
- circuit-break repeated failures

Acceptance:

- Large contexts compact before provider rejection.
- Auto compact produces visible runtime events and timeline rows.
- Repeated compact failures stop after a bounded number of attempts and surface
  a clear warning.

## Snip Compact

Snip compact is required for long-lived sessions where UI scrollback and runtime
input must diverge.

Semantics:

- UI can retain old history.
- Model input projection excludes snipped middle history.
- Runtime resume can replay snip boundaries.
- Removed messages are represented by refs, not silently forgotten.

Snip boundary must record:

- removed message refs
- preserved segment head/tail
- reason
- token delta estimate
- read-file state impact
- tool discovery/capability state impact

Projection behavior:

- keep protected system/context messages
- keep compact/snip boundary messages
- keep latest task tail
- keep tool-use/tool-result groups intact
- replace snipped span with a boundary marker and optional summary

Acceptance:

- A long session can keep visible scrollback while projection size drops.
- Resume uses snip boundaries to reconstruct projected history.
- Tool-use/tool-result pairs remain valid after snip.
- Read-file state is restored or marked stale.

## Reactive Compact

Reactive compact handles provider prompt-too-long/context-length failures.

Flow:

1. Detect provider context-length error.
2. Record reactive attempt.
3. Try low-risk projection reductions:
   - tool result budget reapplication
   - microcompact
   - snip
4. If still too large, run full compact.
5. Rebuild model input.
6. Retry the model request.
7. Stop after bounded attempts.

Acceptance:

- A prompt-too-long provider error can recover without user intervention.
- Failed recovery produces a visible runtime error with attempted actions.
- No tool-use/tool-result invariant is broken during retry.

## Prompt Assembly

Rewrite prompt assembly recording around actual projection output.

`RuntimePromptAssembly` should record:

- projection ID
- canonical message count
- projected message count
- selected/omitted messages
- selected/omitted tools
- tool result replacements
- compact boundaries
- snip boundaries
- reactive attempts
- context sources
- read-state reinjections
- budget before/after
- warnings
- redacted hashes of system prompt and summary content

Prompt assembly must represent what the model actually saw.

## Runtime Events

Add event types:

```text
context.projection.started
context.projection.completed
context.projection.failed
context.budget.updated
context.tool_result.replaced
context.microcompact.completed
context.compact.started
context.compact.completed
context.compact.failed
context.snip.completed
context.reactive.started
context.reactive.retrying
context.reactive.completed
context.reactive.failed
context.read_state.reinjected
context.read_state.stale
```

Events should include IDs and summary metadata only. Large summaries and raw
outputs should be stored as refs.

## Runtime DTOs

Expose transport-neutral DTOs from `internal/runtime`.

Required DTOs:

- `RuntimeContextProjection`
- `RuntimeContextProjectionMessage`
- `RuntimeContextBoundary`
- `RuntimeContentReplacement`
- `RuntimeSnipBoundary`
- `RuntimeReactiveCompactAttempt`
- `RuntimeContextWarning`
- `RuntimeReadStateReinjection`
- `RuntimeContextGovernanceState`

`SessionActivity` should include timeline-ready compact/snip/context markers.

## Frontend Implementation

React must display runtime facts without owning context state.

### Timeline

Add timeline item types:

- `compact_boundary`
- `snip_boundary`
- `microcompact_marker`
- `reactive_compact_retry`
- `tool_result_replacement`

Rows should show:

- trigger
- status
- messages before/after
- tokens before/after
- replacements count
- refs count
- error when failed

Rows should be expandable for detail, but compact by default.

### Context Panel

Replace the current diagnostics-only panel with a governance panel:

- current projection ID
- context usage and threshold state
- auto compact status
- active compact/snip boundaries
- tool result budget savings
- reactive retry history
- read-state reinjection status
- model-visible summary

### Composer/Header Status

Show:

- context remaining
- warning threshold
- compacting state
- reactive retry state
- manual compact action
- manual snip action

Manual actions must call runtime APIs, not mutate frontend state.

### Settings

Add context governance settings:

- auto compact enabled
- auto compact threshold
- microcompact interval
- microcompact keep recent
- tool result budget
- snip enabled
- reactive compact retry limit

All settings write through runtime/config APIs.

## Integration Points To Change

### Agent

Change `internal/agent` so it receives prepared model input from runtime context
manager, or calls a context-manager interface injected by runtime.

Remove:

- prompt-local microcompact in `preparePrompt`
- context governance decisions inside `agent.go`
- legacy summarize as automatic long-context mechanism

Keep:

- streaming model/tool loop
- tool result callbacks
- orphan tool-call/result repair as a final safety net

### Runtime

Runtime should:

- own `ContextManager`
- call `BuildModelInput` before each model step
- persist projection and boundary records
- expose context governance DTOs
- handle reactive retry orchestration

### Message

Message service should remain canonical. Do not put projection policy in
`internal/message`.

### Config

Move context governance config into a dedicated config section:

```json
{
  "context_governance": {
    "auto_compact_enabled": true,
    "tool_result_budget_chars": 200000,
    "max_single_tool_result_chars": 16000,
    "microcompact_interval": "60m",
    "microcompact_keep_recent": 3,
    "snip_enabled": true,
    "reactive_retry_limit": 2
  }
}
```

## Implementation Phases

### Phase 1: Context Manager Skeleton And Schema

Deliver:

- new `internal/contextmgr` package
- new stores and migrations
- core DTOs
- projection records
- no behavior change beyond recording a no-op projection

Tests:

- store CRUD
- projection DTO JSON
- runtime service can create a no-op projection

### Phase 2: Model Input Ownership

Deliver:

- all model calls route through `ContextManager.BuildModelInput`
- no-op projection is used as the source for prompt assembly
- prompt assembly records projection ID

Tests:

- model input generated from projection
- prompt assembly matches projection counts
- no direct compact policy remains in `agent.preparePrompt`

### Phase 3: Tool Result Budget

Deliver:

- stable per-message aggregate tool result budget
- persisted replacement records
- runtime refs for original outputs
- resume reconstruction

Tests:

- single huge result replacement
- parallel tool result grouping
- byte-identical resume replacement
- tool-use/tool-result invariant preservation

### Phase 4: Microcompact

Deliver:

- projection-level microcompact
- time/count/token triggers
- boundary records
- frontend marker DTOs

Tests:

- old compactable results cleared from projection
- recent results preserved
- original refs recoverable
- canonical messages unchanged

### Phase 5: Full Compact

Deliver:

- real summary-generation compact
- compact projection replaces old history before model call
- preserved tail
- reinjected context/read-file refs

Tests:

- next model input excludes full old history
- summary and tail are included
- compact summary prompt-too-long retry
- read-file reinjection states

### Phase 6: Auto Compact

Deliver:

- pre-call threshold detection
- warning/blocking states
- circuit breaker
- runtime events and audit

Tests:

- compact triggers before provider rejection
- compact skipped for compact helper calls
- repeated failures circuit-break

### Phase 7: Snip Projection And Replay

Deliver:

- snip boundary
- projected view
- resume replay
- UI scrollback separation

Tests:

- projection excludes snipped messages
- UI activity still contains snip markers
- resume reconstructs projected view
- tool pairs remain valid

### Phase 8: Reactive Compact

Deliver:

- provider prompt-too-long detection
- bounded reactive retry
- reactive attempts persisted
- frontend retry marker

Tests:

- fake provider context-length error recovers
- bounded failure reports clear error
- retry uses rebuilt projection

### Phase 9: Frontend Governance UI

Deliver:

- timeline markers
- context governance panel
- composer/header context state
- manual compact/snip actions
- settings integration

Tests:

- adapter mapping
- marker rendering
- manual action transport
- browser smoke

### Phase 10: Remove Legacy Paths

Deliver:

- remove legacy auto summarize as primary context mechanism
- remove runtime post-turn fake full compact path
- remove agent-local microcompact
- update docs and tests

Tests:

- `go test ./...`
- `cd client && npm run build`
- long-session smoke
- packaged Wails smoke when available

## Test Matrix

Required backend tests:

- context manager no-op projection
- budget calculation before/after replacements
- tool result replacement persistence
- replacement resume reconstruction
- microcompact projection only
- full compact summary generation
- compact prompt-too-long retry
- auto compact threshold
- reactive compact retry
- snip replay
- read-file reinjection
- tool-use/tool-result invariant validation
- runtime events and audit payloads

Required frontend tests/smoke:

- context panel renders projection state
- timeline renders compact/snip/reactive markers
- manual compact action calls runtime
- manual snip action calls runtime
- adapter maps all DTOs
- refresh restores the same governance state from runtime

Required end-to-end smoke:

- long conversation with many tool calls
- huge shell output
- huge file read output
- auto compact then continue
- prompt-too-long fake provider then reactive compact retry
- snip then resume
- restart after compact then continue

## Acceptance Criteria

The refactor is complete only when all of these are true:

- Every model call uses a persisted context projection.
- Tool result budget decisions survive restart.
- Full compact changes the next model input, not only diagnostics.
- Auto compact happens before provider rejection under normal conditions.
- Reactive compact can recover from a simulated context-length provider error.
- Snip can separate UI history from model input projection.
- Prompt assembly reflects the exact projection sent to the model.
- Frontend displays compact/snip/reactive context events from runtime DTOs.
- Old incomplete compact paths are deleted or removed from the main product
  path.

## Non-Goals

- Do not preserve old compact boundary records.
- Do not migrate old sessions.
- Do not implement React-owned context state.
- Do not infer compact state from assistant prose.
- Do not build a full Run scheduler as part of this work.
- Do not introduce CLI/TUI compatibility as a design constraint.

## New Session Implementation Prompt

Use the following prompt in a new Codex session to start implementation.

```text
你现在在 C:\Users\ytq\work\ai\agent-builder 仓库中工作。

请先完整阅读并遵守：

- AGENTS.md
- docs/organize/README.md
- docs/organize/01-project-overview.md
- docs/organize/02-module-function-overview.md
- docs/organize/03-feature-gap-inventory.md
- docs/organize/04-module-deep-dive.md
- docs/long-term-context-governance-implementation-plan.md
- docs/frontend-runtime-integration-notes.md
- docs/react-agent-phase-04-context-prompt-compact-memory.md
- docs/tool-result-guard-design.md

目标：

将 Agent Builder 当前不完整的上下文压缩机制重构为生产级长期会话治理系统。不要做临时实现，不要为了兼容旧 compact 语义添加 shim，不迁移旧 compact 数据。如果旧实现和目标架构冲突，直接删除或重构。

核心要求：

1. 新增 runtime-owned ContextManager，所有模型调用前必须经过 ContextManager.BuildModelInput()。
2. 明确区分 canonical session history、model input projection、frontend timeline view。
3. 实现持久化 context projection，prompt assembly 必须记录模型实际看到的 projection。
4. 重构 tool result budget：
   - 按 API wire message 分组；
   - 稳定 replacement；
   - replacement 决策持久化；
   - resume 后 byte-identical 重放；
   - 原始输出通过 runtime ref 保存。
5. 实现 projection-level microcompact：
   - time/count/token trigger；
   - 保留最近 N 个工具结果；
   - 不修改 canonical message；
   - 记录 boundary 和 replacement。
6. 实现真正 full compact：
   - 模型调用前触发；
   - 生成 compact summary；
   - 下一轮 model input 使用 summary + preserved tail + reinjected context；
   - 不是 turn 结束后只做诊断记录。
7. 实现 auto compact：
   - 基于 effective context window、output reserve、warning/auto/blocking thresholds；
   - 带递归 guard 和 consecutive failure circuit breaker。
8. 实现 snip compact：
   - snip boundary；
   - projected view；
   - replay；
   - UI scrollback 与 model input 分离；
   - tool_use/tool_result 配对不被破坏。
9. 实现 reactive compact：
   - 捕获 prompt-too-long/context-length provider error；
   - 先尝试低风险 projection reduction；
   - 必要时 full compact；
   - bounded retry；
   - 失败时记录清晰 runtime event。
10. 前端只展示 runtime DTO：
    - timeline compact/snip/micro/reactive markers；
    - context governance panel；
    - composer/header context state；
    - manual compact/snip actions 通过 runtime API。

请按 docs/long-term-context-governance-implementation-plan.md 中的 Phase 1 开始实施，不要一次性做完整十个阶段。当前 session 的目标是完成 Phase 1，并为 Phase 2 留下清晰边界。

Phase 1 交付物：

- 新建 internal/contextmgr 包。
- 定义 ContextManager 核心接口、请求/响应类型、projection/boundary/replacement/warning DTO。
- 新增 context governance store 与数据库 migration。
- Runtime 能创建并持久化 no-op projection。
- Prompt assembly 可以引用 projection ID，但不要求本阶段改变模型输入行为。
- 添加 focused tests 覆盖 store CRUD、projection DTO JSON、runtime no-op projection 记录。
- 不要实现临时 compact 逻辑，不要把策略散落到 agent.go。

验证：

- go test ./internal/contextmgr ./internal/runtime
- go test ./...
- 如触及 client，再运行 cd client && npm run build

工作方式：

- 先阅读代码和文档，再给出 Phase 1 的具体修改计划。
- 实施时保持 runtime 是事实来源，React 不拥有上下文状态。
- 使用现有项目风格和测试方式。
- 修改完成后总结改动文件、验证结果、Phase 2 下一步。
```
