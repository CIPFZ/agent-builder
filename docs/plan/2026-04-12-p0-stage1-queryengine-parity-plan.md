# P0 Stage 1 QueryEngine Parity Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** move the Go runtime from a minimal stateful loop toward Claude Code style QueryEngine parity by tightening the main loop, context assembly, prompt assembly inputs, and compaction/persistence seams around the QueryEngine.

## Progress notes

- Added a shared `bootstrapRuntime` seam so CLI/TUI and daemon now normalize workspace roots, permissions, persistent sessions, LLM client construction, and runner creation through one startup path instead of host-local assembly.
- Added a warning-threshold `microcompact` layer that clears older high-cost `system.run` / `mcp__*` tool results before full summary compaction.
- Added memory-aware compaction roll-forward: when transcript history no longer contains the prior compact summary, QueryEngine now uses persisted summary memory as the next compact summary and preserves a post-anchor tail instead of discarding already summarized context.
- Added `last_summarized_message_id` tracking and source-aligned session-memory compact branches for:
  - summarized-anchor hit -> summary memory plus preserved tail after anchor
  - missing anchor -> fallback to traditional compact
  - resumed session without anchor -> summary memory plus minimum preserved tail from the end
  - preserved-tail invariant -> assistant/tool-result pairs are not split at the slice boundary
  - old compact boundaries are filtered out of the preserved tail before the new compact boundary is appended

**Architecture:** treat `internal/queryengine` as the center of the runtime and deepen it before adding higher-order agent features. This stage keeps scope inside the single-agent runtime core and explicitly avoids `AgentTool` parity, fork semantics, coordinator/swarm, and remote-host protocol work. The work should preserve the current package layout while making the QueryEngine depend on clearer context-provider and recovery-ready seams.

**Tech Stack:** Go, standard library testing

---

## Stage scope

This stage covers only these P0 modules:

- QueryEngine session execution kernel
- Context assembly system
- Prompt assembly and model-resolution inputs needed by the QueryEngine
- Tool-loop semantics around permission, compaction, and memory writeback

This stage explicitly does not cover:

- transcript persistence/recovery implementation
- full CLAUDE.md/session-memory/agent-memory layering
- agent definition loading
- AgentTool parity
- forked subagents
- coordinator/swarm/team runtime
- host protocol parity

## First module to replicate

The first formal replication target is:

**`QueryEngine` main-loop semantics**

Reason:

- it is the dependency center of the entire runtime
- prompt/context/tool/permission/compact interactions are all forced through it
- getting it wrong would force rework in every later module

## Files expected in this stage

**Likely modify:**
- `C:\Users\ytq\work\ai\agent-builder\internal\queryengine\queryengine.go`
- `C:\Users\ytq\work\ai\agent-builder\internal\queryengine\queryengine_test.go`
- `C:\Users\ytq\work\ai\agent-builder\internal\runtime\runner.go`
- `C:\Users\ytq\work\ai\agent-builder\internal\runtime\runner_test.go`
- `C:\Users\ytq\work\ai\agent-builder\internal\prompt\builder.go`
- `C:\Users\ytq\work\ai\agent-builder\internal\prompt\builder_test.go`
- `C:\Users\ytq\work\ai\agent-builder\internal\compaction\service.go`
- `C:\Users\ytq\work\ai\agent-builder\internal\compaction\service_test.go`
- `C:\Users\ytq\work\ai\agent-builder\internal\memory\service.go`
- `C:\Users\ytq\work\ai\agent-builder\internal\session\manager.go`

**Possible create:**
- `C:\Users\ytq\work\ai\agent-builder\internal\queryengine\context_provider.go`
- `C:\Users\ytq\work\ai\agent-builder\internal\queryengine\context_provider_test.go`
- `C:\Users\ytq\work\ai\agent-builder\internal\queryengine\prompt_inputs.go`

---

## Task 1: Freeze the baseline before edits

**Files:**
- Test only: `internal/queryengine/queryengine_test.go`
- Test only: `internal/runtime/runner_test.go`
- Test only: `internal/prompt/builder_test.go`
- Test only: `internal/compaction/service_test.go`

- [ ] **Step 1: Run baseline tests for the current QueryEngine slice**

Run:

```powershell
go test ./internal/queryengine ./internal/runtime ./internal/prompt ./internal/compaction -count=1
```

Expected:

- PASS
- current behavior snapshot is known before replication starts

- [ ] **Step 2: Record the baseline in working notes**

Capture:

- whether tests pass
- how many tests run
- any flaky or suspicious behavior that should not be misread as a replication regression

---

## Task 2: Introduce explicit context-provider seams around QueryEngine

**Files:**
- Create: `internal/queryengine/context_provider.go`
- Create: `internal/queryengine/context_provider_test.go`
- Modify: `internal/queryengine/queryengine.go`

- [ ] **Step 1: Write failing tests for context sourcing**

Add tests proving:

- QueryEngine builds user context separately from system context
- context providers can be overridden in tests
- system context includes permission/workspace execution boundary data
- user context remains session-oriented and does not collapse into system context

- [ ] **Step 2: Run targeted tests to verify they fail**

Run:

```powershell
go test ./internal/queryengine -run "TestQueryEngine.*Context" -count=1
```

Expected:

- FAIL because explicit context-provider seams do not exist yet

- [ ] **Step 3: Implement minimal context-provider layer**

Implement:

- a small context-provider abstraction inside `internal/queryengine`
- default system-context and user-context providers that preserve current behavior
- QueryEngine wiring that calls these providers instead of hardcoding context assembly inline

- [ ] **Step 4: Re-run targeted tests**

Run:

```powershell
go test ./internal/queryengine -run "TestQueryEngine.*Context" -count=1
```

Expected:

- PASS

---

## Task 3: Tighten QueryEngine prompt input assembly

**Files:**
- Modify: `internal/queryengine/queryengine.go`
- Modify: `internal/prompt/builder.go`
- Modify: `internal/prompt/builder_test.go`

- [ ] **Step 1: Write failing tests for prompt input structure**

Add tests proving:

- QueryEngine passes system context, user context, workspace context, tool surface, and memory inputs through distinct prompt inputs
- tool surface is filtered through session policy at prompt-build time
- memory inputs preserve typed grouping instead of flattening everything into a single list

- [ ] **Step 2: Run targeted tests to verify they fail**

Run:

```powershell
go test ./internal/prompt ./internal/queryengine -run "Test(Build|Compose|QueryEngine).*" -count=1
```

Expected:

- FAIL because current QueryEngine/prompt handoff is still too inline and under-specified

- [ ] **Step 3: Implement minimal prompt-input tightening**

Implement:

- explicit construction of prompt inputs in QueryEngine
- prompt builder updates only where needed to keep user/system/workspace/tool/memory lanes distinct
- no speculative prompt hierarchy redesign beyond this stage

- [ ] **Step 4: Re-run targeted tests**

Run:

```powershell
go test ./internal/prompt ./internal/queryengine -run "Test(Build|Compose|QueryEngine).*" -count=1
```

Expected:

- PASS

---

## Task 4: Harden QueryEngine tool-loop semantics

**Files:**
- Modify: `internal/queryengine/queryengine.go`
- Modify: `internal/queryengine/queryengine_test.go`
- Modify: `internal/runtime/runner_test.go`

- [ ] **Step 1: Write failing tests for tool-loop invariants**

Add tests proving:

- repeated identical tool calls do not create infinite loops
- deferred tools remain bounded
- approval-required tool calls exit through the approval path without emitting false runtime-failure semantics
- approved continuation produces a coherent assistant follow-up

- [ ] **Step 2: Run targeted tests to verify they fail**

Run:

```powershell
go test ./internal/queryengine ./internal/runtime -run "Test(QueryEngine|Runner).*(Tool|Approval|Continue)" -count=1
```

Expected:

- FAIL for at least one invariant that is not yet enforced precisely enough

- [ ] **Step 3: Implement minimal tool-loop hardening**

Implement:

- stricter pending-tool execution guards
- cleaner separation between approval-required exits and real runtime errors
- no new tool features; only runtime semantic tightening

- [ ] **Step 4: Re-run targeted tests**

Run:

```powershell
go test ./internal/queryengine ./internal/runtime -run "Test(QueryEngine|Runner).*(Tool|Approval|Continue)" -count=1
```

Expected:

- PASS

---

## Task 5: Tighten compaction writeback semantics around QueryEngine

**Files:**
- Modify: `internal/compaction/service.go`
- Modify: `internal/compaction/service_test.go`
- Modify: `internal/queryengine/queryengine.go`
- Modify: `internal/runtime/runner_test.go`
- Modify: `internal/memory/service.go`

- [ ] **Step 1: Write failing tests for compact-boundary behavior**

Add tests proving:

- compaction emits a boundary and updates QueryEngine state consistently
- summary writeback to memory happens only when a real summary exists
- compact replay and cleanup hooks preserve the QueryEngine message view
- token-threshold analysis and stored compact state stay in sync

- [ ] **Step 2: Run targeted tests to verify they fail**

Run:

```powershell
go test ./internal/compaction ./internal/queryengine ./internal/runtime -run "Test(Service|QueryEngine|Runner).*(Compact|MemorySaved)" -count=1
```

Expected:

- FAIL because current compact writeback semantics are still only partially aligned

- [ ] **Step 3: Implement minimal compaction semantic tightening**

Implement:

- compaction result handling improvements in QueryEngine
- tighter memory-save conditions
- state bookkeeping fixes needed for later persistence/recovery work

- [ ] **Step 4: Re-run targeted tests**

Run:

```powershell
go test ./internal/compaction ./internal/queryengine ./internal/runtime -run "Test(Service|QueryEngine|Runner).*(Compact|MemorySaved)" -count=1
```

Expected:

- PASS

---

## Task 6: Repository verification after Stage 1 code changes

**Files:**
- Modify if needed: `docs/plan/2026-04-12-p0-stage1-queryengine-parity-plan.md`

- [ ] **Step 1: Run focused post-change tests**

Run:

```powershell
go test ./internal/queryengine ./internal/runtime ./internal/prompt ./internal/compaction ./internal/memory -count=1
```

Expected:

- PASS

- [ ] **Step 2: Run repository-level verification**

Run:

```powershell
go test ./... -count=1
```

Expected:

- PASS

- [ ] **Step 3: Run mandatory functional testing**

Run at minimum:

```powershell
go run ./cmd/myclaw version
```

Expected:

- prints the current dev version string without regression

Then run:

```powershell
go run ./cmd/myclaw tui
```

Functional checks:

- TUI starts successfully
- a basic user prompt still produces an assistant response
- a tool-triggering prompt still shows the tool/approval flow
- no obvious regression in compaction-triggered session behavior

- [ ] **Step 4: Record results before claiming stage completion**

Record:

- exact test commands executed
- pass/fail status
- any known gaps deferred to the next stage

---

## What comes after Stage 1

If this stage passes, the next recommended stage is:

**P0 Stage 2: session persistence and recovery parity**

That stage should cover:

- transcript persistence format
- session metadata persistence
- restore-time message cleanup
- QueryEngine continuation-safe state restoration

## Progress notes

- 2026-04-12: added QueryEngine context-provider seams and prompt-override handoff.
- 2026-04-12: compaction now persists a `[compact_boundary]` message into the stored session transcript.
- 2026-04-12: file-backed session storage is wired into CLI and daemon entrypoints.
- 2026-04-12: added `session.ContinuationMessages` and switched QueryEngine bootstrap to load continuation-safe transcript views (latest summary + latest compact boundary + post-boundary tail).
- 2026-04-12: `session.Manager` now seeds session/message counters from persisted store state so reloads continue allocating monotonic `main-*`, `session-*`, and `msg-*` IDs instead of reusing `000001`.
- 2026-04-12: added persisted `Session.Metadata` recovery anchors and now update them from both normal message appends and QueryEngine compaction (`last activity`, `last user/assistant message`, `last compact boundary`, `last compaction summary`, `last compaction reason`, `last compacted at`).
- 2026-04-12: QueryEngine now reconstructs recovery-oriented runtime state from persisted session metadata on first session load, including compact anchors, latest user/assistant content, message count, and a `restored` compaction phase marker.
- 2026-04-12: recovery loading is now centralized behind `session.RecoverySnapshot`, which bundles persisted session metadata, full transcript history, and the continuation-safe transcript view for QueryEngine/bootstrap consumers.
- 2026-04-12: `RecoverySnapshot` now exposes derived resume anchors (`LastUserMessage`, `LastAssistantMessage`, `CompactBoundary`, `CompactionSummary`, `HasCompaction`) so higher-level resume logic can consume a stable recovery protocol instead of rescanning transcript slices.
- 2026-04-12: `RecoverySnapshot` now exposes `ContinuationState`, and `Runner.ResumeSubagent` consumes it to reject new prompts only when a prior subagent run is already terminal but the child session still ends in an unfinished user-side turn; running subagent resume semantics remain unchanged.
- 2026-04-12: runtime continuation gating now also checks pending approvals before allowing terminal-run subagent resume, so `ResumeSubagent` distinguishes approval-blocked recovery from ordinary unfinished-turn recovery without regressing running-session resume behavior.
- 2026-04-12: pending approval anchors are now written into `Session.Metadata`, surfaced through `RecoverySnapshot.ContinuationState` as `awaiting_approval`, and cleared through a new runtime-level `UpdateApprovalStatus` path used by gateway and TUI so approval-state recovery stops depending on ad hoc `ApprovalManager` calls.
- 2026-04-12: runner startup now rehydrates pending approvals from persisted session metadata back into `ApprovalManager`, with enough stored fields (`run_id`, `user_message_id`, `category`, `rule_source`, tool metadata) for restored approvals to continue through `ApproveAndContinue` after process restart.
- 2026-04-12: restored approval continuations now also clear the full persisted approval anchor set after `ApproveAndContinue`, keeping recovered sessions from carrying stale approval-blocked metadata once the approval has been consumed.
- 2026-04-12: `ResumeSubagent` now rejects resumes against runs that are still `running`, so one child session cannot accumulate overlapping active runs; gateway resume flow was tightened accordingly to resume only after the earlier run reaches a terminal state while still reusing the same child session.
- 2026-04-12: `agent.task resume` now matches runtime resume semantics by rejecting attempts to resume a run that is still `running`, removing a second entrypoint that previously allowed overlapping active runs on the same delegated child session.
- 2026-04-12: recovery continuation now treats a trailing `tool` message as an unfinished assistant-side turn instead of a ready-for-user boundary, so interrupted tool completions no longer permit a fresh prompt before the assistant follow-up is reconstructed or completed.
- 2026-04-12: recovery continuation now uses the persisted compact boundary as an explicit ready-for-user resume anchor when only the compacted view (`summary + [compact_boundary]`) remains, so compaction-only restores no longer collapse into an ambiguous `empty` state.
- 2026-04-12: recovery now synthesizes a compact-boundary anchor from persisted metadata when replay/cleanup paths have removed the original boundary message from the stored transcript, preventing compaction-aware restore state from disappearing just because the durable transcript was trimmed after compaction.
- 2026-04-12: recovery now also synthesizes a compaction-summary anchor from persisted metadata when replay/cleanup paths have removed the original summary message from the stored transcript, keeping compaction-aware restore state stable even when the durable transcript only retains the post-compact tail.
- 2026-04-12: QueryEngine restored state now keeps `CompactBoundaryCount` consistent with recovered compaction anchors, eliminating the previous mismatch where `LastCompactBoundaryID` was restored but the boundary count remained `0`.
- 2026-04-12: QueryEngine restored state now marks `LastCompactionPhase=restored` whenever any compaction anchor is recovered, eliminating the previous mismatch where compaction boundary/summary state was restored but the compaction phase remained empty if `LastCompactionReason` was absent.
- 2026-04-12: recovery bootstrap now synthesizes the continuation message view itself from compaction metadata when the durable transcript only retains the post-compact tail, so `QueryEngine.Messages()` stays aligned with the recovered summary/boundary anchors instead of exposing a tail-only view while state claims compaction anchors exist.
- 2026-04-12: `session.Manager.ContinuationMessages()` now uses the same synthesized recovery view as `RecoverySnapshot` and `QueryEngine`, eliminating the previous protocol split where manager-level continuation access still returned a plain tail-only view after compaction replay/cleanup trimming.
- 2026-04-12: default user context now includes `current_date`, aligned with `claude-code/src/context.ts` `currentDate` semantics instead of omitting a source-defined user-context field.
- 2026-04-12: `UserContextProvider` now receives `workspace.Context`, and default user-context loading now injects `CLAUDE.md` content into the user-context lane while workspace loading now reads `CLAUDE.md`, aligning the Go runtime with `claude-code/src/context.ts` where `claudeMd` is part of user context rather than an unrelated workspace artifact.
- 2026-04-12: default system context now includes a source-aligned `git_status` snapshot for git worktrees (`Current branch`, `Status`, `Recent commits`) and still omits it outside git repos, matching the `claude-code/src/context.ts` design where `gitStatus` belongs to system context instead of being inferred ad hoc inside prompts.
- 2026-04-12: the `git_status` snapshot now also includes source-aligned `Main branch (you will usually use this for PRs)` and optional `Git user` fields, with default-branch resolution falling back to the current branch when no remote default branch is available.
- 2026-04-12: `git_status` now also mirrors the source 2k truncation behavior for oversized `git status --short` output, including the same explicit truncation notice instead of dumping arbitrarily large status payloads into system context.
- 2026-04-12: default system context now supports a source-aligned optional cache-breaker injection seam (`[CACHE_BREAKER: ...]`) through `QueryEngine.Config.SystemPromptInjection`, so cache-breaking state lives in system context rather than being mixed into ordinary prompts.
- 2026-04-12: the runtime now exposes context-level disable seams for `claudeMd` and `gitStatus` (`QueryEngine.Config.DisableClaudeMd` / `DisableGitStatus`), so the core can represent the same “conditionally omit these context lanes” semantics that `claude-code/src/context.ts` applies via environment/mode checks before host wiring is added.
- 2026-04-12: prompt assembly now includes an explicit `CoordinatorSystemPrompt` layer that overrides agent/custom/default prompts while still allowing tail append, bringing the Go prompt precedence closer to the Claude Code order documented in `17-system-prompt-and-model-resolution.md`.
- 2026-04-12: prompt assembly now also supports the proactive/KAIROS-style agent branch where agent instructions append to the default prompt as `# Custom Agent Instructions` instead of replacing it; QueryEngine now exposes this as an explicit `ProactiveAgentPrompt` flag for source-aligned runtime wiring later.
- 2026-04-12: QueryEngine now carries an explicit `MainLoopModel` into `llm.GenerateRequest`, and the OpenAI-compatible client prefers request-level model override over its factory default; this is the first source-aligned seam toward Claude Code style main-loop model resolution instead of a purely static client model.
- 2026-04-12: `runtime.Options` and the CLI TUI entrypoint now forward the configured main-loop model into QueryEngine instead of leaving model selection trapped inside the LLM client factory, extending the source-aligned model-resolution seam from QueryEngine internals into runtime/bootstrap composition.
- 2026-04-12: `gateway.Options` and the daemon entrypoint now also forward the configured main-loop model into `runtime.Runner`, so both CLI/TUI and daemon/websocket entrypaths resolve the main-loop model through the same runtime seam instead of diverging by host.
- 2026-04-12: session metadata now persists both `initial_main_loop_model` and `main_loop_model_override`; QueryEngine resolves the per-session override ahead of the configured base model, and runtime now exposes a dedicated `SetSessionMainLoopModelOverride` path so source-defined session-scoped model overrides no longer require direct metadata mutation.
- 2026-04-12: runtime/queryengine now also expose an explicit `ClearSessionMainLoopModelOverride` path, so session-scoped overrides can be removed and queries fall back to the configured base model instead of relying on ad hoc metadata edits.
- 2026-04-12: QueryEngine now applies the first two source-defined `getRuntimeMainLoopModel()` plan-mode branches at request time: `opusplan` resolves to the default Opus model in plan mode, and `haiku` upgrades to the default Sonnet model in plan mode instead of being sent through unchanged.
- 2026-04-12: runtime now exposes the session model state triple (`main_loop_model`, `session_main_loop_model_override`, `resolved_main_loop_model`) through Runner getters and websocket `session_status`, aligning the host-facing session state more closely with Claude Code's `mainLoopModel` / `mainLoopModelForSession` split instead of keeping model state implicit.
- 2026-04-12: reading base session model state now also latches `initial_main_loop_model` into persisted session metadata even before the first query, so connect/status flows preserve the session's initial model choice instead of only writing it during the first model pass.
- 2026-04-12: websocket control protocol now supports `session_set_model`, allowing hosts to set a session-scoped model override or clear it with `model=default`; this reuses the runtime session-override path and keeps `session_status` model state consistent with the control action.
- 2026-04-12: QueryEngine now applies the `opusplan` 200k-token gate from `getRuntimeMainLoopModel()`, keeping `opusplan` unresolved in plan mode once estimated context exceeds 200k tokens; the fix also corrected a deeper bootstrap bug where `SubmitPrompt()` could previously skip loading stored transcript history before appending the first new user message of the process lifetime.
- 2026-04-12: `main loop model resolution` now also carries `LLM provider` through `app -> gateway/runtime -> QueryEngine`, allowing the Claude Code `getDefaultSonnetModel()` split to resolve `haiku + plan mode` to `claude-sonnet-4-5` on non-`firstParty` providers instead of always hardcoding `claude-sonnet-4-6`.
- 2026-04-12: tool execution now uses the same filtered availability boundary as prompt exposure: blanket-denied or disabled tools are rejected as unavailable at `Inspect/Invoke` time instead of still entering approval/execution through the raw registry, matching Claude Code's filtered tool-pool semantics more closely.
- 2026-04-13: MCP deny-rule matching now also recognizes the Claude Code wildcard form `mcp__server__*`, so server-wide MCP deny rules filter prompt exposure, runtime permission checks, and direct tool execution consistently alongside the already-supported `mcp__server` prefix form.
- 2026-04-13: prompt tool lines now follow Claude Code's cache-stable partition ordering more closely: built-in tools are kept as a contiguous, name-sorted prefix and non-builtin/MCP tools are name-sorted after them, instead of preserving raw registry insertion order or globally interleaving all tools.
- 2026-04-13: prompt tool-pool assembly now also deduplicates by tool name with built-in precedence, mirroring Claude Code's `assembleToolPool()` behavior where built-ins win over same-named MCP tools after partitioned sorting.
- 2026-04-13: `tool.search` invocation is now policy-aware, so deny-filtered tools no longer leak back into runtime search results after being removed from the prompt-visible tool pool.
- 2026-04-13: `tool.search` now searches only deferred tools, matching Claude Code's deferred-tool discovery role instead of re-listing ordinary prompt-visible tools such as `system.run`.
- 2026-04-13: `tool.search` now recognizes the Claude Code direct-selection form `select:<tool_name>` while still applying the existing enabled/deny/deferred filters, so deferred tools can be selected by exact name instead of only by keyword search.
- 2026-04-13: `tool.search` direct selection now also matches the Claude Code multi-select and loaded-tool fallback semantics: `select:A,B,C` preserves order, deduplicates repeated selections, prefers deferred matches first, and still returns already-loaded tools by exact name when they are not in the deferred set but remain enabled and not deny-filtered.
- 2026-04-13: bare exact-name `tool.search` queries now also match the Claude Code fast path: deferred tools still win first, but exact-name queries fall back to already-loaded tools when the name is not in the deferred set, avoiding unnecessary no-match churn after compaction/subagent prompts emit a raw loaded tool name.
- 2026-04-13: `tool.search` keyword/prefix discovery now enforces the Claude Code default `max_results=5` cap at the tool boundary, so unbounded deferred-tool matches no longer spill past the source-defined default result count while direct-selection paths remain unaffected.
- 2026-04-13: `tool.search` keyword grammar now honors Claude Code style required terms (`+term`) by pre-filtering deferred candidates before scoring, so queries like `+slack send` no longer degrade into plain substring search that can return tools missing the mandatory term.
- 2026-04-13: `tool.search` optimistic enablement is no longer hardcoded `true`; the Go runtime now mirrors the clearest source-defined mode gates, disabling the tool when `CLAUDE_CODE_DISABLE_EXPERIMENTAL_BETAS=true`, `ENABLE_TOOL_SEARCH=false`, or `ENABLE_TOOL_SEARCH=auto:100`, while keeping default/`auto`/`auto:0` behavior enabled.
- 2026-04-13: websocket `session_set_permission` now carries `plan_mode` and `auto_mode`, runs updated policies through `permissions.SetupPolicy`, and round-trips normalized permission state (`plan_mode`, `auto_mode`, `workspace_roots`) back through both `session_set_permission` and `session_status`, so host-driven permission changes now share the same validation and normalization boundary as bootstrap config.
- 2026-04-13: plan-mode runtime model resolution no longer hardcodes default Opus/Sonnet IDs: `opusplan` and `haiku` now flow through env-backed default model helpers, honoring `ANTHROPIC_DEFAULT_OPUS_MODEL` and `ANTHROPIC_DEFAULT_SONNET_MODEL` the same way Claude Code's `model.ts` routes default-model overrides.
- 2026-04-13: main-loop model resolution now parses Claude Code aliases before runtime selection across base config and session overrides: `sonnet` resolves to the default Sonnet model, `haiku` resolves to the default Haiku model outside plan mode, `opus` and `best` resolve to the default Opus model, and `opusplan` resolves to the default Sonnet model outside plan mode.
- 2026-04-13: runtime plan-mode alias handling now matches Claude Code's `parseUserSpecifiedModel()` + `getRuntimeMainLoopModel()` split more closely: `opusplan` upgrades to the default Opus model only while plan mode is active and estimated context stays under 200k, otherwise it remains on the already-resolved default Sonnet model instead of preserving the raw alias string.
- 2026-04-13: alias resolution now applies consistently across `QueryEngine`, `Runner`, and websocket `session_set_model`, while the host-visible session state keeps the raw override (`session_main_loop_model_override`) separate from the resolved runtime model (`resolved_main_loop_model`).
- 2026-04-13: permissions now recognize the Claude Code external modes `bypassPermissions` and `dontAsk` (including `bypass-permissions` / `dont-ask` normalization at setup/control boundaries), so websocket `session_set_permission` can round-trip those source-defined modes instead of rejecting them as unknown.
- 2026-04-13: `bypassPermissions` now behaves like a real runtime permission mode instead of a missing alias: it still respects earlier rule-based `ask`/`deny` decisions, but bypasses the later mode-based approval boundary; `dontAsk` now transforms approval-required decisions into denials rather than surfacing interactive approval prompts.
- 2026-04-13: CLI/TUI and daemon startup now share a single `bootstrapRuntime` assembly seam for workspace-root resolution, permission policy normalization, persistent session manager wiring, LLM client construction, and runtime runner creation, reducing the remaining gap with Claude Code's centralized application bootstrap layer.
- 2026-04-13: compaction now has a Claude Code style `microcompact` layer for historical high-cost tool results: older `system.run`/MCP tool outputs are cleared to `[Old tool result content cleared]` before full summary compaction, and QueryEngine emits `compact.micro` when this warning-threshold cleanup path runs.
- 2026-04-13: permissions now also recognize the remaining Claude Code user-addressable modes `default`, `acceptEdits`, `plan`, and `auto` at setup/control boundaries; `plan` / `auto` set their policy flags, `acceptEdits` gets minimal source-aligned Go semantics, and optional subagent mode normalization now preserves the empty state before deriving a safer child policy.
- 2026-04-13: QueryEngine now distinguishes final permission denials from approval-required decisions: only `RequiresApproval` creates an approval request, while `deny` rule results stop as final permission denials, matching Claude Code's `PermissionResult` behavior split more closely.
