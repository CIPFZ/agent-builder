# P0 Strict 1:1 Review Against Claude Code

Date: 2026-04-13

## Scope

This review compares the current Go runtime against the Claude Code P0 runtime source with a strict 1:1 standard. It does not treat "similar behavior" as complete when the source-level runtime contract is still different.

Primary Claude Code sources checked:

- `claude-code/src/QueryEngine.ts`
- `claude-code/src/query.ts`
- `claude-code/src/Tool.ts`
- `claude-code/src/services/tools/toolExecution.ts`
- `claude-code/src/services/tools/toolHooks.ts`
- `claude-code/src/services/compact/sessionMemoryCompact.ts`
- `claude-code/src/context.ts`
- `claude-code/src/utils/model/model.ts`
- `claude-code/src/utils/permissions/PermissionMode.ts`
- `claude-code/src/types/permissions.ts`

Primary Go sources checked:

- `internal/queryengine/queryengine.go`
- `internal/compaction/service.go`
- `internal/model/message.go`
- `internal/tools/registry.go`
- `internal/permissions/policy.go`
- `internal/prompt/builder.go`
- `internal/session/recovery.go`
- `internal/app/bootstrap.go`
- `internal/llm/openai_compatible.go`
- `internal/protocol/ws/message.go`

## Bottom Line

P0 is not 100% complete under strict 1:1 review.

The Go runtime has a strong P0 kernel and many source-aligned seams, but several remaining gaps are structural, not cosmetic. The biggest strict-parity blockers are:

1. `model.Message` now has optional block metadata, and QueryEngine tool execution now emits linked `tool_use` / `tool_result` blocks for actual tool calls; the remaining gap is broader LLM/provider/hook block production and consumption.
2. The tool execution contract is still registry/function-call oriented rather than Claude Code's full `Tool` + `ToolUseContext` + `PermissionDecision` + hook pipeline, but the Go runtime now has initial per-tool and PermissionRequest-hook `updatedInput` execution paths.
3. Permissions recognize most modes and rule decisions, but do not yet clone the full `ToolPermissionContext`, hook-resolved permission updates, classifier-backed `auto`, or internal `bubble`.
4. Compaction implements the main session-memory branches, but not the full `adjustIndexToPreserveAPIInvariants` block semantics or post-compact attachment/reinjection economics.
5. Startup/bootstrap is centralized in Go, but not yet equivalent to Claude Code's feature-gated app composition layer.

## Findings

### P0-1: Go message model is below Claude Code block parity

Claude Code source:

- `claude-code/src/services/compact/sessionMemoryCompact.ts:232` defines `adjustIndexToPreserveAPIInvariants`.
- `claude-code/src/services/compact/sessionMemoryCompact.ts:324` defines `calculateMessagesToKeepIndex`.
- `claude-code/src/services/compact/sessionMemoryCompact.ts:514` defines `trySessionMemoryCompaction`.

Go source:

- `internal/model/message.go:5` defines `Message` as `ID`, `SessionID`, `Role`, `Content`, `CreatedAt`.
- `internal/compaction/service.go:404` currently has a simplified `adjustToBoundaryFloor`.

Strict parity result:

- Not aligned.

Claude Code preserves API invariants across structured content blocks:

- user `tool_result` blocks must retain matching assistant `tool_use` blocks by `tool_use_id`.
- assistant streaming fragments with the same provider `message.id` must not be split because normalization later merges them.
- thinking blocks are preserved as part of that same provider-message invariant.

The Go implementation currently only handles a simplified `tool` role preceded by `assistant`. That is useful, but it is not a 1:1 replica. The correct next step is to add optional block metadata to Go messages and implement the same invariant algorithm against that metadata while keeping string-only messages backward compatible.

### P0-2: Tool execution contract is still not 1:1

Claude Code source:

- `claude-code/src/Tool.ts:123` defines `ToolPermissionContext`.
- `claude-code/src/Tool.ts:362` defines the `Tool` contract.
- `claude-code/src/Tool.ts:500` requires `checkPermissions`.
- `claude-code/src/Tool.ts:556` defines `toAutoClassifierInput`.
- `claude-code/src/services/tools/toolExecution.ts:800` runs pre-tool hooks.
- `claude-code/src/services/tools/toolExecution.ts:921` resolves hook permission decisions.
- `claude-code/src/services/tools/toolExecution.ts:1130` applies `permissionDecision.updatedInput`.

Go source:

- `internal/tools/registry.go:26` defines `Tool`.
- `internal/tools/registry.go:38` defines `PolicyAwareTool`.
- `internal/queryengine/queryengine.go:1283` executes the Go tool loop.
- `internal/queryengine/queryengine.go:1365` invokes tools through `InvokeWithPolicy`.

Strict parity result:

- Partially aligned, not 1:1.

The Go runtime has policy-aware invocation and prompt/execution filtering, but Claude Code's tool execution is a richer protocol:

- tool-specific `checkPermissions` runs before execution.
- hooks can allow, deny, ask, or mutate `updatedInput`.
- permission decisions have richer reasons and content blocks.
- `toAutoClassifierInput` supplies auto-mode classifier material.
- tool progress and result shaping feed transcript semantics.

Go now has an initial equivalent for per-tool `checkPermissions` returning `updatedInput`, and QueryEngine uses that updated input for transcript blocks, tool events, approval requests, and execution. QueryEngine also has an initial PermissionRequest-hook seam that can allow a previously approval-bound tool call and mutate `updatedInput` before execution. It is still not source-complete: rich decision reasons/content blocks, permission updates, full hook execution/racing semantics, and auto-classifier input remain open.

### P0-3: Permission modes are recognized, but permission context is not cloned

Claude Code source:

- `claude-code/src/utils/permissions/PermissionMode.ts:21` defines mode schemas.
- `claude-code/src/Tool.ts:123` defines `ToolPermissionContext`.
- `claude-code/src/types/permissions.ts:419` defines `ToolPermissionRulesBySource`.

Go source:

- `internal/permissions/policy.go:19` defines `Policy`.
- `internal/permissions/policy.go:96` defines `Evaluate`.
- `internal/queryengine/queryengine.go:1308` evaluates permission decisions.

Strict parity result:

- Improved but still not 1:1.

Aligned:

- `default`, `acceptEdits`, `plan`, `auto`, `bypassPermissions`, and `dontAsk` are now recognized.
- `ask` is distinct from `deny`.
- final `deny` no longer creates approval requests.
- subagent safer-mode derivation exists.

Not aligned:

- no `additionalWorkingDirectories` map equivalent in permission context.
- no `alwaysAllowRules` / `alwaysDenyRules` / `alwaysAskRules` object model equivalent.
- no `strippedDangerousRules`.
- no `shouldAvoidPermissionPrompts` or `awaitAutomatedChecksBeforeDialog`.
- no `prePlanMode`.
- no classifier-backed `auto`.
- no internal `bubble` propagation.

The current Go policy is a valid simplified policy engine, but not a 1:1 clone of Claude Code's permission context model.

### P0-4: Session-memory compact is behaviorally close but not source-complete

Claude Code source:

- `claude-code/src/services/compact/sessionMemoryCompact.ts:232` adjusts start index for block invariants.
- `claude-code/src/services/compact/sessionMemoryCompact.ts:571` computes preserved `messagesToKeep`.

Go source:

- `internal/compaction/service.go:138` implements `CompactWithSessionMemory`.
- `internal/compaction/service.go:340` computes `sessionMemoryStartIndex`.
- `internal/compaction/service.go:404` applies simplified boundary adjustment.

Strict parity result:

- Partially aligned, not 1:1.

Aligned:

- session-memory summary branch exists.
- missing summarized anchor falls back to traditional compact.
- resumed-session branch exists.
- minimum token/text-block tail expansion exists.
- compact-boundary floor exists.
- old compact boundaries are filtered out.

Not aligned:

- no block-level `tool_use` / `tool_result` id matching.
- no provider `message.id` grouping for thinking/tool fragments.
- no plan attachment creation.
- no hook result reinjection from `processSessionStartHooks('compact')`.
- no transcript path and session-memory truncation note equivalent.
- no post-compact threshold recheck against auto-compact threshold.
- no discovered-tool metadata annotation on boundary.

The immediate strict-parity task should be block metadata plus invariant-preserving start-index calculation.

### P0-5: QueryEngine is structurally similar, but not source-equivalent

Claude Code source:

- `claude-code/src/QueryEngine.ts:184` defines the class runtime.
- `claude-code/src/QueryEngine.ts:416` processes user input.
- `claude-code/src/QueryEngine.ts:476` mutates `toolPermissionContext` from input processing.
- `claude-code/src/QueryEngine.ts:681` hands tool-use context into query execution.
- `claude-code/src/query.ts:412` applies microcompact before autocompact.
- `claude-code/src/query.ts:453` invokes autocompact.

Go source:

- `internal/queryengine/queryengine.go:195` defines `QueryEngine`.
- `internal/queryengine/queryengine.go:562` runs the model pass.
- `internal/queryengine/queryengine.go:605` invokes session-memory compact.
- `internal/queryengine/queryengine.go:1283` executes the tool loop.

Strict parity result:

- Strong skeleton, not 1:1.

Aligned:

- long-lived session engine exists.
- multi-pass tool loop exists.
- approval interruption and continuation exist.
- context/prompt/model inputs are explicit.
- compaction and microcompact are integrated.
- recovery state hydration exists.

Not aligned:

- no complete `processUserInput` equivalent.
- no read-file state / discovered skills / file history / attribution state parity.
- no full `ToolUseContext` object passed through tools.
- no complete query event/checkpoint/usage tracking surface.
- no structured tool-use/result block model.
- no source-equivalent auto-compact tracking and consecutive-failure circuit behavior.

### P0-6: Context and prompt assembly are partially aligned, but source context loaders are incomplete

Claude Code source:

- `claude-code/src/context.ts:36` gets git status.
- `claude-code/src/context.ts:170` loads CLAUDE.md.
- `claude-code/src/context.ts:186` emits current date.

Go source:

- `internal/queryengine/context_provider.go` supplies default context providers.
- `internal/prompt/builder.go:14` defines build input.
- `internal/prompt/builder.go:73` builds system prompt.

Strict parity result:

- Partially aligned.

Aligned:

- user/system context lanes exist.
- current date exists.
- CLAUDE.md is in user context.
- git status is in system context.
- cache breaker and disable seams exist.
- prompt precedence includes override/coordinator/proactive-agent branches.

Not aligned:

- CLAUDE.md loader precedence and include semantics are incomplete.
- memory-file filtering and cached ClaudeMd semantics are incomplete.
- bare-mode/add-dir context omission semantics are only approximated.
- context suppliers are still mostly QueryEngine-local rather than full bootstrap-driven app state.

### P0-7: Model resolution is source-inspired, not a full clone

Claude Code source:

- `claude-code/src/utils/model/model.ts:92` defines `getMainLoopModel`.
- `claude-code/src/utils/model/model.ts:145` defines `getRuntimeMainLoopModel`.
- `claude-code/src/utils/model/model.ts:445` defines `parseUserSpecifiedModel`.

Go source:

- `internal/queryengine/model_resolution.go`
- `internal/queryengine/queryengine.go:799` resolves session main-loop model.
- `internal/llm/openai_compatible.go:35` applies request-level model override.

Strict parity result:

- Partial-to-strong, not 1:1.

Aligned:

- request-level model override exists.
- session override set/clear exists.
- aliases are parsed.
- plan-mode `opusplan`/`haiku` behavior exists.
- env-backed default Opus/Sonnet overrides exist.

Not aligned:

- no full model option/display-name system.
- provider routing is narrower.
- no full startup-level model setting chain.
- no complete model capability checks such as model/tool-search compatibility and 1M context feature decisions.

### P0-8: Startup/runtime composition is centralized but not equivalent

Claude Code source:

- `claude-code/src/setup.ts`
- `claude-code/src/bootstrap/state.ts`
- `claude-code/src/bridge/*`
- `claude-code/src/services/plugins/*`
- `claude-code/src/services/mcp/*`

Go source:

- `internal/app/bootstrap.go:28` centralizes runtime bootstrap.
- `internal/app/cli.go`
- `internal/app/daemon.go`
- `internal/runtime/runner.go`

Strict parity result:

- Partially aligned.

Aligned:

- CLI/TUI and daemon share a runtime bootstrap seam.
- permissions, store, LLM client, workspace roots, and runner are composed centrally.

Not aligned:

- no full feature gate bootstrap.
- no plugin/skills/MCP/LSP initialization in the same composition layer.
- no Claude Code style mode router for REPL / bridge / remote / assistant / SDK-hosted paths.
- no bootstrap-state equivalent for global session flags and runtime transitions.

### P0-9: Session persistence and recovery are good but format/protocol differ

Go source:

- `internal/session/recovery.go:3` defines `RecoverySnapshot`.
- `internal/session/recovery.go:94` computes continuation state.
- `internal/session/recovery.go:210` synthesizes compaction anchors.

Strict parity result:

- Strong but not 1:1.

Aligned:

- file-backed persistence exists.
- monotonic IDs survive reload.
- metadata anchors exist.
- pending approvals recover.
- compaction summary/boundary anchors can be synthesized.
- QueryEngine can hydrate state from recovery metadata.

Not aligned:

- transcript format is Go-specific.
- message block format is missing.
- broader runtime object recovery is incomplete.
- worktree/fork/task/output-file state parity is outside current P0 kernel.

## Strict P0 Direction

The next work should not chase broad new modules. It should close the remaining source-contract gaps in this order:

1. Continue evolving Go's `PermissionDecision` toward Claude Code's full model: richer decision reasons, content blocks, permission updates, and `ToolPermissionContext` snapshots.
2. Extend `Tool` / `PolicyAwareTool` beyond the current initial `CheckPermissions` + `UpdatedInput` path toward source-like `toAutoClassifierInput` and hook-aware input mutation.
3. Continue wiring Go message blocks through LLM/provider adapters and hook flows, beyond the current QueryEngine direct and approval-resumed tool execution transcript paths.
4. Move `auto` from workspace-root approximation toward classifier/hook-backed semantics.
5. Expand post-compact reinjection: session-start hook results, plan attachment, transcript path/session-memory truncation notes, discovered-tool boundary metadata, and post-compact threshold recheck.
6. Deepen bootstrap into a source-like app-composition layer that owns feature gates and extension/runtime capability assembly.

## Progress Update: Block-Level Session-Memory Compact

Implemented after this strict review:

- `internal/model/message.go` now includes optional `ProviderMessageID` and `Blocks []MessageBlock`.
- `internal/compaction/service.go` now upgrades `adjustToBoundaryFloor` into a source-aligned invariant-preserving adjustment:
  - scans all kept messages for `tool_result` block `tool_use_id`s
  - scans kept assistant blocks for already-preserved `tool_use` ids
  - walks backward to include missing assistant `tool_use` blocks
  - walks backward to include assistant messages sharing the same provider `message.id`, preserving thinking/tool fragments that later API normalization would merge
  - keeps the old role-level assistant/tool fallback for legacy string-only messages
- `internal/compaction/service_test.go` now covers the two Claude Code bug scenarios described in `sessionMemoryCompact.ts`: orphan `tool_result` references and same-provider-message thinking block loss.

Remaining block-level gap:

- LLM/provider adapters, hook flows, and richer thinking/text block streaming still need to populate and consume these blocks. The compaction layer now has the data model and invariant algorithm, and QueryEngine's direct tool execution transcript now emits linked `tool_use` / `tool_result` blocks, but strict full parity requires all runtime transcript producers to emit provider ids, tool_use ids, tool_result ids, and thinking/text blocks naturally.

## Progress Update: QueryEngine Tool Transcript Blocks

Implemented after the initial block-level compaction pass:

- `internal/session.Manager` now supports appending messages with provider message id and block metadata while preserving the existing string-only `AppendMessage` API.
- `internal/llm.StreamEvent` now carries optional `ToolUseID` and `ProviderMessageID`.
- `internal/queryengine` now emits an assistant `tool_use` block and a linked tool-result block for real tool execution, using a generated tool-use id when the model stream does not provide one.
- `internal/queryengine/queryengine_test.go` now verifies that a normal tool call persists linked `tool_use` / `tool_result` blocks in the transcript.
- Approval interruption now preserves provider-native `ToolUseID` and `ProviderMessageID` through `approval.Request`, pending approval session metadata, restored runner state, and `ApproveAndContinue`, so approval-resumed tool execution no longer falls back to generated ids when the model already supplied provider ids.
- `internal/tools.Registry` now has an initial per-tool `CheckPermissions` seam, and `permissions.Decision` can carry `UpdatedInput`; QueryEngine applies the updated input to tool-use blocks, tool events, approval requests, and final tool invocation.
- `internal/queryengine.PermissionHook` and `runtime.Options.PermissionHook` now provide an initial Claude Code `PermissionRequest` hook analogue: when policy would require approval, a hook can return allow/deny plus `UpdatedInput`; allow executes the updated input without creating an approval request.
- Permission parity was tightened after review:
  - tool-level `checkPermissions` ask decisions can now flow through `PermissionHook`, not only policy-originated asks.
  - tool-level allow no longer bypasses policy ask gating in the covered QueryEngine path.
  - hook allow resolves the tool-originated ask but still allows later policy deny checks to block the final execution.
  - `permissions.Decision` now carries both legacy string `UpdatedInput` and object-shaped `UpdatedInputObject`; QueryEngine serializes object-shaped updates to compact JSON before writing events, transcript blocks, approval requests, and invoking tools.
  - `tools.Registry` now supports optional parsed-object permission and invocation APIs for tools that opt in, while preserving the old string APIs for existing tools and non-object inputs.
  - `llm.StreamEvent` and QueryEngine pending tool calls now carry `ToolInputObject`; QueryEngine preserves that object into tool events and assistant `tool_use` blocks while still emitting legacy JSON-string input for existing consumers.
  - approval/runtime/gateway/session metadata now preserve object-native tool input through approval interruption, pending approval recovery, runtime event conversion, and compatibility payloads.

Remaining gap:

- The generated id path is still an adapter fallback. Full parity still requires every streaming/provider adapter to emit provider-native ids where available, and hook paths still need richer permission-result handling: decision reasons, content blocks, permission updates, hook sequencing/racing, complete hook lifecycle events, and object-native primary protocol contracts where external compatibility permits.

## Rating Interpretation

The earlier `99%` document is too optimistic if interpreted as strict 1:1 source parity. It is better read as "P0 kernel shape is close and many functional seams exist."

Under strict 1:1 review, P0 should remain open until the structural gaps above are closed. The most accurate status is:

- functional P0 kernel: high
- strict P0 source-contract parity: not complete
- full Claude Code runtime parity including P1/P2/P3: far from complete, intentionally deferred by the current P0 plan

## Progress Update: ToolUseContext Runtime Context

The previous strict finding that QueryEngine had "no full `ToolUseContext` object passed through tools" is now substantially reduced. Go now has a source-like `tools.ToolUseContext` with abort context, session/tool/tool_use identity, object-native input, policy, exposed tools, agent/model/provider metadata, message history, app state setter, tool decisions, file/glob limits, MCP clients/resources, requestPrompt, progress callback, and contextModifier. QueryEngine now constructs this context through a single `toolUseContext` path and passes it to contextual permission checks.

Validation added:

- `internal/queryengine/queryengine_test.go::TestQueryEnginePassesToolUseContextToContextualPermissionTool` now verifies configured file/glob limits, MCP clients/resources, requestPrompt, progress callback, abort context, toolUseID, current messages, exposed tools, and tool decision mutation.
- `go test ./... -count=1` passes after the ToolUseContext update.

Strict parity caveat:

- This closes the core P0 `ToolUseContext` contract gap for the Go runtime path, but it is still not a byte-for-byte TS clone of every UI/session callback in `claude-code/src/Tool.ts`. Remaining work belongs to deeper tool/runtime UI surfaces and later MCP/LSP/subagent integrations, not the central P0 permission context path.
