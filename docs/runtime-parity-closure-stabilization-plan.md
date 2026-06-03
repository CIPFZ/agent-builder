# Runtime Parity Closure Stabilization Plan

Status: completed for the 2026-06-03 runtime closure gate.

This plan starts after the frontend/backend tool, thinking, and permission
integration milestone. The goal is to prove that the runtime-owned primitives
now visible in the desktop workbench are durable, recoverable, replayable, and
safe across their cross-boundary interactions.

## 2026-06-03 Completion Record

The closure gate was run as scenario coverage rather than a new user-facing
surface. The runtime scenario harness now initializes the same durable stores
needed by replay/recovery closure, including MCP requests, hook executions, and
refs, and adds focused scenarios for:

- pending permission recovery after runtime reload, followed by denial through
  the runtime decision path, with permission, tool call, turn, event, audit, and
  replay facts updated;
- multi-session active turn recovery, proving running and waiting-permission
  turns remain independent while completed turns do not recover as active;
- MCP elicitation pending recovery, denial, replay provenance, and secret
  redaction;
- AgentTask message ordering, undeliverable follow-up rejection, durable
  mailbox state, and replay explanation.

Existing closure tests continue to cover policy/headless behavior, hook
precedence, shell risk classification, MCP request store paths, sandbox and
worktree decisions, compact/ref replay, output/artifact refs, HTTP route
contracts, and redaction.

Validation completed:

```powershell
go test ./internal/runtime -run "Scenario|Replay|Recovery|Permission|Policy|Hook|MCP|AgentTask|Sandbox|Worktree|Ref|Turn|Tool" -count=1
go test ./desktop -count=1
cd client; npm run lint
cd client; npm run build
```

No frontend, adapter implementation, or browser-visible behavior changed in
this closure pass, so manual in-app browser verification was not required.

Remaining risks:

- shell parser parity can still be broadened with more PowerShell, cmd, and
  POSIX fixture cases;
- React diagnostics remain intentionally blocked until the next phase consumes
  these runtime facts through DTOs rather than local reducers;
- OS-level sandbox executor maturity and remote teammate/runtime surfaces remain
  later product hardening, not part of this local closure gate.

## Goal

Add parity closure scenarios and contract checks for:

- tool calls and normalized output;
- permission policy, decisions, and recovery;
- hooks and policy precedence;
- MCP auth, elicitation, request state, and denial paths;
- AgentTask and coordinator communication;
- sandbox and worktree scope decisions;
- compact, reinjection, output refs, artifact refs, and replay;
- runtime event, audit, HTTP, Wails, and TypeScript DTO consistency.

The closure target is not a new user-facing page. It is a runtime confidence
gate before deeper React diagnostics, Skills/MCP management, Projects, or
future product surfaces.

## Why This Is Next

The current runtime already has the main local desktop primitives: turns,
ToolCalls, events, audit, replay, recovery, policy profiles, compact,
capability discovery, MCP lifecycle, hooks, worktrees, sandbox boundary records,
and local AgentTask communication.

The remaining risk is cross-boundary correctness. For example:

- a hook allow decision must not bypass deterministic deny, headless, scope,
  sandbox, or MCP gates;
- a pending permission must recover after refresh or restart;
- a denied tool must not leave the turn or UI permanently busy;
- AgentTask follow-up messages must remain ordered and recoverable;
- replay must preserve decision order while redacting sensitive payloads;
- React must be able to reconstruct visible state from runtime APIs, not from
  local reducers or message text parsing.

## Non-Goals

- Do not add remote runtime, SSH/cloud teammate, or remote agent product
  surfaces.
- Do not add advisory permission approval. Model-assisted permission
  explanations can come later and must remain advisory-only.
- Do not build React deep diagnostics before the runtime facts and contracts
  are stable.
- Do not introduce React-owned business state for task, tool, permission,
  compact, hook, replay, artifact, policy, or worktree truth.
- Do not rewrite provider/model protocol abstractions or change
  `charm.land/fantasy` for this stage.
- Avoid migrations unless a scenario proves a required runtime fact cannot be
  persisted or recovered.

## Work Packages

### 1. Scenario Harness Coverage

Primary files:

- `internal/runtime/runtime_scenario_harness_test.go`
- `internal/runtime/runtime_replay_export.go`
- `internal/runtime/runtime_recovery.go`
- `internal/runtime/runtime_turn_store.go`
- `internal/runtime/runtime_tool_call_store.go`

Scenarios:

- active turn recovers as running, waiting permission, completed, failed,
  cancelled, or interrupted as appropriate;
- pending permission recovers and can be allowed or denied after reload;
- deny updates permission, tool call, turn, event, audit, and replay facts;
- cancellation while waiting for permission expires or cancels the permission;
- multi-session active turns remain independent.

### 2. Policy And Hook Precedence

Primary files:

- `internal/permission/policy.go`
- `internal/runtime/runtime_policy.go`
- `internal/runtime/runtime_permissions.go`
- `internal/runtime/runtime_hooks.go`
- `internal/agent/hooked_tool.go`

Scenarios:

- hook allow cannot bypass deterministic deny;
- hook allow cannot bypass headless fail-closed behavior;
- hook allow cannot bypass session/project/worktree scope checks;
- hook allow cannot bypass sandbox or MCP request gates;
- policy reason, risk, mode, decision, and audit event remain explainable.

### 3. Shell And Tool Safety Fixtures

Primary files:

- `internal/permission/policy.go`
- `internal/tools/scheduler/`
- `internal/agent/scheduler_tool.go`
- `internal/runtime/runtime_scheduler_recorder.go`

Scenarios:

- read-only commands are classified consistently;
- write, execute, network, secret, and destructive command shapes trigger the
  expected policy outcome;
- PowerShell, cmd, and POSIX-like shell examples have fixture coverage where
  supported by the runtime classifier;
- tool output is separated into model-visible content and UI/audit diagnostics.

### 4. MCP And Capability Request Closure

Primary files:

- `internal/runtime/runtime_mcp.go`
- `internal/runtime/runtime_mcp_requests.go`
- `internal/runtime/runtime_capabilities.go`
- `internal/agent/tools/mcp/`

Scenarios:

- MCP pending auth or elicitation is persisted and recoverable;
- denied MCP request records event and audit facts;
- MCP tool execution uses the same scheduler, permission, output, and replay
  semantics as builtin tools;
- capability inventory does not grant permission by itself.

### 5. AgentTask And Coordinator Closure

Primary files:

- `internal/runtime/runtime_agent_tasks.go`
- `internal/runtime/runtime_agent_task_tools.go`
- `internal/runtime/runtime_agent_task_comm_store.go`
- `internal/agent/task_tools.go`
- `internal/agent/coordinator.go`

Scenarios:

- task follow-up, stop, list, get, message, and output paths remain ordered;
- rejected or undeliverable task messages are persisted and explainable;
- child task cwd/worktree scope denial is recorded;
- task output and artifact refs survive replay and recovery.

### 6. Compact, Refs, Replay, And Redaction

Primary files:

- `internal/runtime/runtime_compact.go`
- `internal/runtime/runtime_compact_store.go`
- `internal/runtime/runtime_refs.go`
- `internal/runtime/runtime_replay_export.go`
- `internal/runtime/runtime_audit.go`

Scenarios:

- compact boundaries and reinjection refs survive restart;
- output refs and artifact refs can be replayed without expanding large or
  sensitive payloads;
- replay preserves event ordering and decision provenance;
- secrets, tokens, headers, env values, and credential-like payloads are
  redacted from exported diagnostics.

### 7. Adapter And DTO Contract Checks

Primary files:

- `internal/runtimeapi/contract.go`
- `internal/runtime/runtime_http.go`
- `desktop/runtime_bridge.go`
- `client/src/runtime/workbenchTypes.ts`
- `client/src/runtime/wailsWorkbenchAdapter.ts`

Checks:

- HTTP and Wails expose the same runtime facts where the desktop client needs
  them;
- TypeScript DTO mirrors Go contracts without inventing fallback business
  state;
- event payloads are notifications and APIs remain the source of truth;
- dev/browser fallback continues to work without assuming Wails bindings.

## Acceptance Criteria

- Scenario tests fail on cross-boundary regressions in policy, hooks, MCP,
  AgentTask, sandbox/worktree, compact/replay, refs, recovery, or adapters.
- Replay and recovery can explain permission, tool, task, compact, and audit
  state from persisted runtime facts.
- No React state is required to reconstruct business facts after reload.
- Sensitive payloads are redacted in replay/export and UI-facing summaries.
- Runtime contracts are stable enough to unblock React diagnostics and
  Skills/MCP management surfaces.

## Validation

Minimum validation after changes:

```powershell
go test ./internal/runtime -run "Scenario|Replay|Recovery|Permission|Policy|Hook|MCP|AgentTask|Sandbox|Worktree|Ref|Turn|Tool" -count=1
go test ./desktop -count=1
cd client; npm run lint
cd client; npm run build
```

When UI or adapter behavior changes, also verify manually in the Codex in-app
browser against the local Vite/runtime target.

## Unlocks

After this closure gate:

1. React compact/task/policy/replay/hook/worktree diagnostics.
2. Skills settings and skill activation management.
3. MCP settings, server management, and enabled tool visibility.
4. Project-scoped sessions, model, policy, skills, MCP, and context.
5. Later product hardening such as advisory permission explanations, plugin
   package governance, advanced memory/session compact, and OS sandbox executor
   maturity.

## Recommended Commit Message

```text
docs: plan runtime parity closure stabilization
```
