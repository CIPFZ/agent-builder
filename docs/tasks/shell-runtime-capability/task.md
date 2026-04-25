# Shell Runtime Capability Task

Date: 2026-04-25

## 1. Task Purpose

This is the single execution document for Claude Code to implement the shell runtime capability module.

Claude Code should use this file as the primary entrypoint.

This file consolidates:

- scope
- non-goals
- Claude Code source alignment
- current Go ownership boundary
- implementation order
- validation requirements
- delivery format

Claude Code should not invent a new shell execution architecture outside this document and the referenced implementation rules.

## 2. Required Reading

Before writing code, read these files in order:

1. `docs/execution/implementation-rules.md`
2. `docs/tasks/shell-runtime-capability/task.md`
3. `docs/tasks/shell-runtime-capability/design.md`
4. `docs/tasks/shell-runtime-capability/source-alignment.md`
5. `docs/tasks/shell-runtime-capability/implementation-plan.md`
6. `docs/tasks/shell-runtime-capability/test-validation-plan.md`
7. `docs/tasks/shell-runtime-capability/review-checklist.md`

After reading, output a short execution summary before coding.

That summary must include:

- module objective
- module non-goals
- Claude semantic alignment points
- target Go files
- planned implementation order
- planned test and validation steps

## 3. Objective

Turn the current shell tooling in `myclaw` into a first-class Claude Code-aligned runtime capability that supports stable local command execution, approval semantics, lifecycle observability, and shared control-plane integration.

## 4. Current Reality

`myclaw` already contains shell-facing tools and a working runtime path:

- shell tools exist
- query/runtime can invoke them
- approvals exist in simplified form
- basic tool progress and tool result event paths already exist

But shell execution is still incomplete as a module because:

- the current shell contract is still too thin and tool-centric
- approval behavior is not yet documented and hardened as one coherent shell policy surface
- shell result and progress semantics are still less explicit than Claude Code
- session/worktree-aware execution rules are not fully documented as a module contract
- downstream execution modules still lack a stable shell base layer

## 5. In Scope

- shell tool contract normalization for local execution
- PowerShell / Bash / generic run-tool semantic alignment where relevant
- approval classification and permission behavior for shell execution
- progress, partial output, and final result lifecycle behavior
- session/worktree-aware execution semantics
- runtime event behavior for shell calls
- `myclawd` control-plane support for shell progress and results
- focused unit, integration, and functional tests

## 6. Out Of Scope

Do not implement any of the following in this task:

- ssh capability expansion beyond what is required for shell alignment boundaries
- Docker feature implementation
- database-specific feature implementation
- React execution UI
- Go TUI parity work
- broad sandbox redesign unrelated to shell execution semantics
- speculative remote execution redesign

## 7. Architecture Constraints

- Shell must remain a shared runtime capability, not a client-owned feature.
- Do not implement shell behavior as ad hoc special cases in gateway or UI layers.
- Approval, progress, and result semantics must stay on shared runtime paths.
- Do not add execution logic that bypasses runtime/session/worktree context.
- Do not treat shell as a hidden implementation detail for future modules.
- Keep this task scoped to the shell base layer, not downstream Docker/DB features.

## 8. Claude Code Semantic Alignment

Use these Claude Code source areas as semantic references:

- `claude-code/src/tools/BashTool/BashTool.ts`
- `claude-code/src/tools/PowerShellTool/PowerShellTool.ts`
- `claude-code/src/tools/RunTool/RunTool.ts`
- `claude-code/src/QueryEngine.ts`
- `claude-code/src/Tool.ts`
- `claude-code/src/permissions.ts`
- `claude-code/src/services/MessageLog.ts`

Carry over these semantics:

- shell execution is a first-class tool lifecycle, not a hidden subprocess helper
- permission and approval behavior is centrally decided from runtime policy
- execution progress is visible on the shared event path
- result payloads preserve enough structure for downstream clients
- worktree and session context influence execution boundaries
- shell is the reusable base for richer execution surfaces

## 9. Go Ownership Boundary

Current primary files:

- `internal/tools/system/run.go`
- `internal/tools/system/ssh.go`
- `internal/queryengine/queryengine.go`
- `internal/runtime/runner.go`
- `internal/gateway/server.go`
- `internal/app/bootstrap.go`
- `internal/permissions`
- `internal/sandbox`

Likely test ownership:

- `internal/tools/system/run_test.go`
- shell-related new tests under `internal/queryengine`
- shell-related new tests under `internal/runtime`
- `internal/gateway/server_test.go`
- approval / permission tests where needed

## 10. Required Module Behaviors

The completed module must provide:

1. stable shell tool contracts for local execution
2. explicit approval and permission behavior for shell execution
3. session-aware and worktree-aware execution semantics
4. progress and result lifecycle visibility through shared runtime paths
5. client-neutral `myclawd` protocol support for shell execution state
6. a reusable base contract for ssh, Docker, and database modules

## 11. Required Implementation Order

Implement in this order:

1. assess and normalize shell tool contract boundaries
2. harden permission and approval semantics
3. normalize progress and result lifecycle behavior
4. expose shell lifecycle state through `myclawd`
5. add focused tests
6. run functional validation with representative local command scenarios

Do not jump ahead to Docker or database-specific workflows.

## 12. Validation Requirements

At minimum, cover:

- local shell command success path
- non-zero exit / failure path
- approval-required path
- progress propagation path
- structured tool result path where relevant
- session/worktree path resolution behavior
- gateway control-plane visibility

Functional validation must include at least:

1. local project command execution success case
2. approval-gated shell command case
3. tool progress visibility through websocket client
4. worktree/session-scoped command execution case

## 13. Delivery Requirements

When implementation is complete, output:

- actual files created or updated
- implementation status mapped to `implementation-plan.md`
- tests executed and their results
- functional validation executed and its results
- remaining risks
- any code/document mismatch found during implementation

## 14. Start Prompt For Claude Code

Use the following prompt to start work:

```text
You are implementing the myclaw shell runtime capability module.

Before coding, read these files in order:
1. docs/execution/implementation-rules.md
2. docs/tasks/shell-runtime-capability/task.md
3. docs/tasks/shell-runtime-capability/design.md
4. docs/tasks/shell-runtime-capability/source-alignment.md
5. docs/tasks/shell-runtime-capability/implementation-plan.md
6. docs/tasks/shell-runtime-capability/test-validation-plan.md
7. docs/tasks/shell-runtime-capability/review-checklist.md

After reading, output a short execution summary covering:
- module objective
- non-goals
- Claude semantic alignment points
- target Go files
- implementation order
- test/validation plan

Then implement the module exactly within the documented scope.

Do not expand the task into ssh, Docker, database, React UI, or TUI work.
Do not invent a separate shell architecture outside the docs.
Keep approval, progress, result, and control-plane behavior on shared runtime paths.
```
