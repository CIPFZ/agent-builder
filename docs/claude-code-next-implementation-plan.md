# Claude Code Next Implementation Plan

Status: runtime closure implementation completed on 2026-06-03.

Agent Builder baseline: `d51590b76a680e683b9d5335797c7076c16a5b05`

Authoritative audit:

- [`docs/claude-code-runtime-parity-closure-review.md`](./claude-code-runtime-parity-closure-review.md)

This plan is for the next session. It does not authorize implementation during
the audit/doc refresh session.

2026-06-03 update:

- The frontend/backend tool, thinking, and permission integration milestone is
  completed.
- Runtime parity closure stabilization and scenario coverage is completed for
  the local desktop closure gate.
- Detailed completion scope is tracked in
  [`runtime-parity-closure-stabilization-plan.md`](./runtime-parity-closure-stabilization-plan.md).
- The next implementation task is now Skills/MCP management surfaces, with
  React consuming runtime DTOs and no frontend-owned business truth.

## Current Decision

There is no core runtime blocker remaining for the local desktop Claude Code
runtime parity scope. Runtime closure stabilization and scenario coverage has
now been run for the 2026-06-03 gate.

Recommended unique next task:

```text
workbench: add Skills and MCP management surfaces
```

## Why Runtime Remains First

- Current code already includes compact/reinjection, persisted replay, tool
  discovery guardrails, policy profiles/headless semantics, output/artifact
  refs, MCP auth/elicitation, worktree recovery, sandbox boundary records,
  context/read-file hardening, hooks lifecycle, and local
  AgentTask/coordinator communication.
- The remaining risk is cross-boundary correctness: hooks, policy, headless,
  AgentTask scope, MCP request gates, sandbox/worktree, compact/replay, refs,
  and recovery all interact.
- React must stay a consumer of runtime truth. It should not derive task,
  compact, hook, replay, artifact, policy, or worktree state from local reducers
  or message parsing.

## Implementation Brief

| Field | Plan |
| --- | --- |
| Goal | Completed: add parity closure scenarios and contract checks that prove runtime-owned behavior across hooks, policy/headless, MCP auth/elicitation, AgentTask communication, sandbox/worktree scope, compact/reinjection, output/artifact refs, replay, and recovery. |
| Why this beats other modules | No missing local runtime primitive remains. Closure scenarios are the gate before React diagnostics and later advisory/plugin/remote work. |
| Non-goals | No remote runtime, SSH/cloud teammate, product telemetry, marketplace-first distribution, provider protocol rewrite, `charm.land/fantasy` changes, React-owned business state, or model self-approved permissions. |
| Go packages/files | `internal/runtime/runtime_scenario_harness_test.go`; `internal/runtime/runtime_replay_export.go`; `internal/runtime/runtime_audit.go`; `internal/runtime/runtime_hooks.go`; `internal/runtime/runtime_agent_tasks.go`; `internal/runtime/runtime_agent_task_comm_store.go`; `internal/runtime/runtime_sandbox.go`; `internal/runtime/runtime_worktrees.go`; `internal/runtime/runtime_mcp_requests.go`; `internal/permission/policy.go`; `internal/agent/task_tools.go`; `internal/agent/hooked_tool.go`. |
| React allowed scope | None by default. Type-only DTO mirrors in `client/src/runtime/types.ts` are allowed only if a stable runtime contract already exists and tests require the mirror. |
| Runtime API / event schema | Prefer existing events and replay summaries. Add fields only when a scenario cannot explain a runtime decision from persisted facts. |
| Data model / migrations | Avoid migrations unless a scenario proves a runtime-owned fact is not persistable or recoverable. |
| Tests | Hook allow cannot bypass deterministic deny/headless/scope/sandbox/MCP; headless ask fails closed; MCP pending/denied/recovered; AgentTask follow-up/stop/message order/reject path; task cwd/worktree scope denial; sandbox fail-closed path; compact reinjection and refs survive restart; replay redacts sensitive payloads and preserves event order. |
| Acceptance criteria | Scenario harness fails on cross-boundary regressions; replay/recovery explain decisions from persisted state; no React state is needed to reconstruct business facts; no secrets leak in replay/export; fixtures cover local AgentTask communication and hooks policy precedence. |
| Risks | Duplicating audit semantics, brittle golden fixtures, storing sensitive payloads, overfitting to one harness. |
| Blocked by | Nothing in the current local runtime scope. |
| Unlocks | Stable React diagnostics, advisory permission explanations, plugin governance, and remote/product work later. |
| Recommended commit message | Completed with `runtime: run parity closure stabilization` |

## Phase Map

| Phase | Status | Modules |
| --- | --- | --- |
| Completed | Runtime foundation | Runtime spine, scheduler, ToolCall store, event/audit/replay, recovery, compact, policy, MCP, hooks, worktree/sandbox boundary, refs, local AgentTask communication. |
| Completed | Runtime closure | Scenario and contract coverage across the completed primitives. |
| Parallel | Runtime hardening | Shell parser fixture expansion, diagnostics DTO consistency, migration/replay contract checks. |
| Next | Skills/MCP management | Runtime-backed management surfaces that consume DTOs without frontend-owned business truth. |
| Unblocked | React deep diagnostics | Allowed after closure confidence, provided diagnostics consume runtime APIs and DTOs. |
| Later | Product/runtime optimization | Advisory permission advisor, plugin governance, advanced memory/session memory compact, OS sandbox executor maturity, remote runtime/SSH/cloud teammate. |
| Not needed | Excluded Claude Code surfaces | TUI/CLI UI, slash UI, keybindings, provider rewrite, first-party telemetry, marketplace-first distribution, product growth/login surfaces. |

## React Diagnostics Entry Conditions

React diagnostics can proceed only when:

- the runtime API already owns the fact being displayed;
- replay/recovery can reconstruct the fact after reload;
- DTOs mirror Go contracts instead of inventing state;
- event streams are used as notifications, not as the sole source of business
  truth;
- closure scenarios cover the corresponding runtime primitive.

## Current Closure Notes

Hooks are complete as a local runtime foundation: lifecycle records, persistence,
audit, replay, and recovery exist for pre-tool, post-tool, and post-failure
paths. Claude Code's wider hook event catalog is later unless needed for a
desktop runtime workflow.

AgentTask/coordinator communication is complete as a local runtime primitive:
model-facing task tools, runtime-owned messages, follow-up delivery to child
sessions, delivered/processed/rejected status, stop/cancel, output/artifact
refs, replay, recovery, worktree/cwd scope, and adapter DTOs exist. Remote
teammate/fleet/cloud is P3.
