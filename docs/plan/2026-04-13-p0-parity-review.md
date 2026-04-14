# P0 Parity Review Against Claude Code

Date: 2026-04-13

## Scope

This review compares the current Go runtime against the Claude Code P0 baseline:

1. startup / runtime composition
2. QueryEngine main loop
3. context assembly
4. system prompt / model resolution
5. tool protocol / tool loop
6. permissions / approval / safety
7. session persistence / recovery
8. compaction / history governance

The goal of this review is not to restate completed work, but to identify which remaining differences still prevent a true P0-complete Claude Code style runtime core.

## Current overall rating

Current estimated P0 parity: 99%

This is an architectural-kernel estimate across all P0 modules, not a strict 1:1 source-contract score and not a claim that any single module has reached 100%.

Strict 1:1 review note:

- See `docs/plan/2026-04-13-p0-strict-1to1-review.md`.
- Under strict Claude Code source-contract parity, P0 remains open.
- The old `99%` label must not be interpreted as "ready to declare P0 complete"; it means the Go runtime has a strong P0 kernel shape, while structural parity gaps remain in provider/hook-level message block production/consumption, tool permission context, hook-aware tool execution, auto/bubble permission semantics, and full post-compact reinjection.

## Module review

### 1. startup / runtime composition

Status: partial

Aligned:
- CLI, TUI, daemon, and gateway now share one runtime seam for session store, main loop model, provider, permissions, and QueryEngine.
- persistent session bootstrap is wired into both CLI and daemon entrypoints.
- tool.search enablement now reflects source-defined optimistic mode gates rather than being hardcoded on.
- host control can now round-trip normalized permission state (`plan_mode`, `auto_mode`, `workspace_roots`) through `session_set_permission` / `session_status` instead of updating only a partial in-memory mode view.
- CLI/TUI and daemon startup now also share a dedicated `bootstrapRuntime` assembly seam for workspace-root resolution, permission setup, persistent session manager creation, LLM client construction, and runner wiring instead of each host reassembling the runtime stack ad hoc.

Still missing:
- no Claude Code class bootstrap orchestrator for feature gates, plugin/skills/MCP/LSP initialization, or command/tool/agent registration as one application-composition layer
- no mode router comparable to `main.tsx` for REPL / bridge / remote / assistant style startup flows
- no definitive runtime bootstrap pass that combines feature decisions with model/tool/prompt composition beyond the current CLI/daemon host bootstrap seam

### 2. QueryEngine main loop

Status: mostly aligned

Aligned:
- multi-pass tool loop
- deferred tool handling
- approval interruption and continuation
- recovery bootstrap from persisted state
- compaction-aware restore state
- explicit state tracking for model pass counts, compaction anchors, and transcript continuation

Still missing:
- Claude Code style tool_reference-aware deferred loading protocol
- richer tool-search result protocol than plain text summaries
- more complete host/runtime event surface around main loop decisions

### 3. context assembly

Status: mostly aligned

Aligned:
- current date in user context
- CLAUDE.md in user context
- git_status in system context
- cache breaker seam
- conditional disable seams for CLAUDE.md and git status

Still missing:
- fuller source-defined context loaders and mode-based context omission logic
- additional context suppliers that in Claude Code are part of bootstrap/runtime assembly rather than QueryEngine-local logic

### 4. system prompt / model resolution

Status: partial-to-strong

Aligned:
- prompt precedence now includes override / coordinator / agent / custom / default / append
- proactive agent prompt branch
- main loop model propagated from host bootstrap to request layer
- session-scoped model override set/clear
- main-loop alias parsing now follows the Claude Code split more closely: `sonnet`, `haiku`, `opus`, `best`, and `opusplan` are normalized before runtime model selection rather than leaking raw aliases into request execution
- plan-mode runtime model switching for `opusplan` and `haiku`
- provider-aware default Sonnet split
- `ANTHROPIC_MODEL` now participates in config/bootstrap model selection
- `ANTHROPIC_DEFAULT_OPUS_MODEL` and `ANTHROPIC_DEFAULT_SONNET_MODEL` now also flow into plan-mode default model resolution rather than being bypassed by hardcoded fallback IDs
- websocket/runtime model state now distinguishes raw session overrides from resolved runtime models even when the override itself is an alias

Still missing:
- full model-option and display-string system from Claude Code
- more complete provider routing and model canonicalization
- broader startup-level model decision chain comparable to `getMainLoopModel()` and related helpers

### 5. tool protocol / tool loop

Status: mostly aligned

Aligned:
- tool exposure and execution boundaries now match
- deny-filtered tools are hidden from prompt, search, and runtime execution
- built-in vs MCP ordering and dedupe are source-aligned
- ToolSearchTool now supports direct select, multi-select, exact-name fallback, default result cap, and required-term filtering

Still missing:
- definitive ToolSearch enablement check including model support / threshold logic
- tool search output schema parity
- tool_reference / deferred schema materialization flow
- pending MCP server awareness in tool.search

### 6. permissions / approval / safety

Status: improved but still the weakest P0 core area

Aligned:
- approval recovery is persisted and resumable
- destructive/system/tool decisions flow through one policy layer
- source precedence now extends beyond config/project/session to include local / flag / policy / cli / command ordering
- `ask` is now a first-class rule action in evaluation, not an implicit mode-only fallback
- host-side permission updates now reuse `SetupPolicy`, so runtime control messages and startup config go through the same normalization/validation path for plan/auto/workspace-root semantics
- Claude Code external permission modes `bypassPermissions` and `dontAsk` are now recognized at setup/control boundaries instead of being rejected as unknown
- `bypassPermissions` now bypasses the late mode gate while still respecting earlier rule-based `ask`/`deny` decisions, and `dontAsk` now converts approval-required outcomes into denials instead of surfacing approval prompts
- Claude Code user-addressable permission modes `default`, `acceptEdits`, `plan`, and `auto` are now recognized at setup/control boundaries; `plan` / `auto` populate the corresponding Go policy flags, `acceptEdits` allows destructive non-system tools while keeping `system.run` on the approval path, and optional subagent mode normalization preserves the empty state before safer-mode derivation
- QueryEngine now treats permission `deny` as a final denial rather than creating an approval request; only decisions marked `RequiresApproval` enter the approval manager, which is closer to Claude Code's `allow` / `ask` / `deny` permission-result split
- approval continuation now preserves provider-native `tool_use` identity across approval request creation, session metadata recovery, and resumed execution, so the approved transcript keeps the original provider tool-use/message ids instead of generating replacement ids
- the tool registry now exposes an initial per-tool `CheckPermissions` seam, and `permissions.Decision.UpdatedInput` is applied through QueryEngine execution, transcript blocks, tool events, and approval requests
- QueryEngine and Runtime now expose an initial `PermissionHook` seam corresponding to Claude Code `PermissionRequest` hooks: when policy would otherwise require approval, a hook can allow/deny and mutate `UpdatedInput`; allow proceeds without creating an approval request
- QueryEngine now treats tool-level `checkPermissions` ask results and policy ask results through the same hook-aware path for the covered cases: tool-originated ask can be allowed/denied/rewritten by `PermissionHook`, while policy ask/deny still gates the final execution.
- `permissions.Decision` now supports both legacy string `UpdatedInput` and object-shaped `UpdatedInputObject`, with QueryEngine normalizing object updates to compact JSON before writing tool events, assistant `tool_use` blocks, approval requests, and final tool invocation. This brings the Go transport closer to Claude Code's `updatedInput?: Record<string, unknown>` contract without breaking existing string-based tools.
- `tools.Registry` now exposes optional parsed-object tool APIs:
  - `StructuredPermissionCheckingTool.CheckPermissionsWithInput`
  - `StructuredTool.InvokeWithInput`
  - `StructuredPolicyAwareTool.InvokeWithInputAndPolicy`
  JSON object inputs now prefer these parsed-object paths, while non-JSON inputs and legacy tools keep the existing string API.
- `llm.StreamEvent` and QueryEngine's pending tool-call path now accept object-native tool input via `ToolInputObject`, normalize it for legacy string fields, and preserve the object through `tool.called` / `tool.result` events where available.
- assistant `tool_use` message blocks now carry both the legacy JSON string `Input` and object-native `InputObject`, closer to Claude Code's `tool_use.input` object shape.
- approval interruption and recovery now preserve object-native tool input:
  - `approval.Request.ToolInputObject`
  - `SessionMetadata.PendingApprovalToolInputObject`
  - `runtime.RuntimeEvent.ToolInputObject`
  - restored pending approvals from session metadata
  - gateway/TUI compatibility payloads as `tool_input_object`

Still missing:
- full `ToolPermissionContext` parity around immutable permission context snapshots, richer decision reasons/content blocks, permission updates, complete hook sequencing/racing, user-facing prompt metadata, and replacing remaining JSON-string compatibility transport with object-native primary contracts where external protocol compatibility permits
- classifier-backed `auto` mode, internal `bubble` propagation, and richer plan-mode permission UI semantics
- more complete dangerous-rule auditing matching Claude Code setup behavior
- richer user-facing permission management semantics comparable to `/permissions`

### 7. session persistence / recovery

Status: strong

Aligned:
- file-backed transcript persistence
- monotonic ID continuation across reload
- persisted metadata anchors
- continuation-safe recovery snapshot
- approval recovery
- compaction summary/boundary synthesis from metadata when transcript is trimmed
- restore-time QueryEngine state hydration

Still missing:
- transcript/metadata format parity is still Go-specific rather than a Claude Code format clone
- broader runtime object recovery beyond current P0 scope

### 8. compaction / history governance

Status: partial

Aligned:
- compaction boundary persisted
- compaction summary persisted into recovery anchors
- replay/cleanup aware restore state
- synthesized continuation view keeps summary/boundary/tail aligned
- a warning-threshold `microcompact` layer now clears older high-cost `system.run` / MCP-style tool-result payloads before full summary compaction, which is much closer to Claude Code's "local garbage collection before global compact" structure than the previous summary-only strategy
- memory-aware compaction now rolls forward persisted summary memory when transcript cleanup has already dropped the previous summary message, so follow-up compaction no longer forgets already summarized context just because the transcript retained only the tail
- `session-memory compact` now tracks a summarized-through anchor and supports the three key source branches: anchor-hit preserved-tail compaction, missing-anchor fallback to legacy compact, and resumed-session compact without an anchor
- preserved-tail compaction now keeps a simplified assistant/tool-result pair intact at the slice boundary and filters old compact boundaries from `messagesToKeep`, matching two core invariants in `sessionMemoryCompact.ts`
- Go messages now have optional block metadata and session-memory compact can preserve Claude Code style block invariants: kept `tool_result` blocks pull in missing assistant `tool_use` blocks by id, and kept assistant fragments pull in preceding assistant fragments with the same provider `message.id` so thinking/tool fragments are not split before API normalization
- QueryEngine tool execution now writes linked assistant `tool_use` and tool-result blocks into the actual transcript, so the block invariant is no longer limited to synthetic compaction tests for the basic direct tool execution path
- QueryEngine approval-resumed tool execution now also keeps provider-native `tool_use` / provider message ids when the model stream supplied them before permission interruption

Still missing:
- fuller multi-strategy compaction parity beyond the current warning-threshold microcompact + session-memory compact + traditional summary compact split, especially provider/hook-level block production/consumption and post-compact cleanup/reinjection economics
- more complete cleanup / reinjection / context-economics behavior

## Main review findings

1. The P0 core is now real, but `permissions / approval / safety` is still the largest remaining parity gap.
2. `tool runtime` is no longer a prototype; the remaining differences are mostly protocol depth, not basic loop correctness.
3. `startup / runtime composition` still lacks a Claude Code class application-composition hub, which means some currently-correct decisions are still distributed across app/config/runtime instead of being assembled in one place.
4. `compaction` is now very close to Claude Code's layered strategy, but it still is not a byte-for-byte clone of the `sessionMemoryCompact.ts` preserved-segment and reinjection pipeline.

## Direction after this review

Recommended remaining P0 direction:

1. deepen permissions / approval semantics
2. deepen startup / runtime composition
3. deepen model resolution and provider routing
4. deepen compaction strategy parity
5. only then finish the remaining tool-search / deferred-tool protocol differences

## Why P0 is not yet 100%

Even though the main loop, recovery chain, and large portions of tool/runtime behavior are now source-aligned, Claude Code P0 is not merely "a functioning loop". The remaining gaps are concentrated in the policy/composition layers that decide how the loop should behave under different modes, providers, and host environments. The latest permission-mode pass closes the user-addressable mode-name gap, but it does not yet clone Claude Code's full `ToolPermissionContext` / per-tool `checkPermissions` pipeline or classifier-backed `auto` / internal `bubble` semantics. Until those layers are also matched, P0 is high-parity but not complete.
