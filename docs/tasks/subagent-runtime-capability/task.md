# Subagent Runtime Capability Task

Date: 2026-04-24

## 1. Task Purpose

This is the single execution document for Claude Code to implement the subagent runtime capability module.

Claude Code should use this file as the primary task entrypoint.

This file consolidates:

- scope
- non-goals
- Claude Code source alignment
- current Go ownership boundary
- implementation order
- validation requirements
- delivery format

Claude Code should not invent a new subagent architecture outside this document and the referenced implementation rules.

## 2. Required Reading

Before writing code, read these files in order:

1. `docs/execution/implementation-rules.md`
2. `docs/tasks/subagent-runtime-capability/task.md`
3. `docs/tasks/subagent-runtime-capability/design.md`
4. `docs/tasks/subagent-runtime-capability/source-alignment.md`
5. `docs/tasks/subagent-runtime-capability/implementation-plan.md`
6. `docs/tasks/subagent-runtime-capability/test-validation-plan.md`
7. `docs/tasks/subagent-runtime-capability/review-checklist.md`

After reading, output a short execution summary before coding.

That summary must include:

- module objective
- module non-goals
- Claude semantic alignment points
- target Go files
- planned implementation order
- planned test and validation steps

## 3. Objective

Turn the existing partial delegated-task implementation in `myclaw` into a first-class runtime capability that supports stable spawn, background execution, control, resume, and control-plane visibility aligned to Claude Code worker semantics.

## 4. Current Reality

`myclaw` already has meaningful subagent internals:

- `agent.task` tool execution through `internal/runtime/runner.go`
- `SpawnSubagentWithOptions` with agent type, model, effort, worktree isolation, and fork handling
- `ResumeSubagent` with child-session reuse and persisted worktree path reuse
- derived subagent permission policies
- background output files and local notification hooks
- `myclawd` methods for spawn, tasks list, subagent list, status, stop, steer, and resume

But the current implementation is incomplete as a module because:

- lifecycle state is still too thin for real operator control
- `subagent_steer` does not yet represent a reliable runtime control channel for actual delegated runs
- `spawn_subagent` on the gateway path exposes only a small subset of runtime options
- background result and notification behavior is not yet normalized as one stable task contract
- `agent.task` still returns ad hoc JSON strings instead of a clearly normalized delegated-task result contract
- the downstream implementation target is not yet documented as one coherent task

## 5. In Scope

- delegated task lifecycle model
- `agent.task` runtime contract
- subagent spawn semantics
- background delegated execution semantics
- fork/default-subagent semantics
- worktree isolation semantics for delegated runs
- control actions for running or completed delegated tasks
- resume semantics using existing child session context
- stable output, notification, and result contracts
- `myclawd` control-plane support for delegated task visibility and control
- focused unit, integration, and functional tests

## 6. Out Of Scope

Do not implement any of the following in this task:

- React operator UI implementation
- Go TUI parity work
- Docker control
- database control
- remote bridge or cloud worker infrastructure
- broad team mailbox / swarm features beyond what is required for local delegated-task parity
- speculative rewrite of the entire session system

## 7. Architecture Constraints

- Delegated-task behavior must remain a shared runtime capability, not a frontend-owned feature.
- Do not solve subagent lifecycle gaps with client-only polling or client-only state.
- Prefer evolving existing runtime and gateway ownership rather than creating a second task subsystem.
- Preserve child session reuse and worktree reuse where those semantics already exist.
- Do not destabilize MCP, SSH, or the main query loop just to force UI-style parity.
- Keep the backend contract transport-neutral so both light CLI usage and future React UI can consume it.

## 8. Claude Code Semantic Alignment

Use these Claude Code source areas as semantic references:

- `claude-code/src/Tool.ts`
- `claude-code/src/QueryEngine.ts`
- `claude-code/src/constants/prompts.ts`
- `claude-code/src/coordinator/coordinatorMode.ts`
- `claude-code/src/tools/AgentTool/AgentTool.tsx`
- `claude-code/src/tools/AgentTool/runAgent.ts`
- `claude-code/src/tools/AgentTool/resumeAgent.ts`
- `claude-code/src/tools/AgentTool/forkSubagent.ts`
- `claude-code/src/tools/SendMessageTool/SendMessageTool.ts`
- `claude-code/src/tools/TaskStopTool/TaskStopTool.ts`
- `claude-code/src/cli/print.ts`

Carry over these semantics:

- delegated workers are first-class tasks with stable IDs
- background delegated work is observable independently from the main turn
- continuing work reuses the existing delegated session context
- stop and resume act on task identity rather than spawning unrelated replacement state
- fork behavior preserves parent context intentionally, not accidentally
- task results and notifications stay transport-neutral

## 9. Go Ownership Boundary

Current primary files:

- `internal/runtime/runner.go`
- `internal/runtime/runner_test.go`
- `internal/agent/manager.go`
- `internal/agent/manager_test.go`
- `internal/queryengine/queryengine.go`
- `internal/queryengine/queryengine_test.go`
- `internal/gateway/server.go`
- `internal/gateway/server_test.go`
- `internal/protocol/ws/message.go`
- `internal/orchestration/coordinator.go`

Likely supporting files:

- `internal/permissions/policy.go`
- `internal/permissions/setup.go`
- `internal/model/session.go`
- `internal/session/*`

## 10. Required Module Behaviors

The completed module must provide:

1. stable delegated-task identity and lifecycle state
2. explicit separation between lifecycle status and control actions
3. background delegated execution that remains observable after tool return
4. reliable control input for real delegated runs
5. resume behavior that reuses child session context when valid
6. explicit fork and worktree semantics
7. `myclawd`-visible delegated task inventory and control semantics
8. transcript-safe delegated-task result and notification behavior

## 11. Required Implementation Order

Implement in this order:

1. normalize delegated-task lifecycle and stored metadata
2. harden runtime spawn, control, and resume paths
3. normalize `agent.task` result and background contracts
4. expose the lifecycle through `myclawd`
5. add focused tests
6. run functional validation with real delegated-task scenarios

Do not jump ahead to Docker or database follow-up work.

## 12. Validation Requirements

At minimum, cover:

- lifecycle status transitions
- background delegated-task launch behavior
- output file and notification behavior
- real steer or continue behavior for runtime-owned delegated tasks
- stop semantics
- resume semantics with reused child session state
- worktree reuse and cleanup behavior
- permission derivation for delegated children
- gateway/control-plane task payload stability

Functional validation must include at least:

1. background delegated run completes and emits stable completion data
2. running delegated task receives and reflects control input
3. stopped or completed delegated task resumes with the same child session
4. worktree-isolated delegated task resumes without losing workspace context

## 13. Delivery Requirements

When implementation is complete, output:

- actual files created or updated
- implementation status mapped to `implementation-plan.md`
- tests executed and their results
- functional validation executed and its results
- remaining risks
- any code or document mismatch found during implementation

## 14. Start Prompt For Claude Code

Use the following prompt to start work:

```text
You are implementing the myclaw subagent runtime capability module.

Before coding, read these files in order:
1. docs/execution/implementation-rules.md
2. docs/tasks/subagent-runtime-capability/task.md
3. docs/tasks/subagent-runtime-capability/design.md
4. docs/tasks/subagent-runtime-capability/source-alignment.md
5. docs/tasks/subagent-runtime-capability/implementation-plan.md
6. docs/tasks/subagent-runtime-capability/test-validation-plan.md
7. docs/tasks/subagent-runtime-capability/review-checklist.md

After reading, output a short execution summary covering:
- objective
- non-goals
- Claude semantic alignment points
- target Go files
- implementation order
- test and validation plan

Then implement directly without asking me to make architecture decisions.

Constraints:
- keep delegated-task behavior on the shared runtime and control-plane path
- do not solve lifecycle gaps in frontend-only code
- preserve valid child-session reuse semantics
- make control actions work for real runtime-owned delegated runs, not only tests
- normalize lifecycle payloads so status and control actions are not conflated

When finished, report:
- files changed
- completion mapped to implementation-plan.md
- tests run
- validation run
- remaining risks
```

## 15. Review Rule

After Claude Code finishes implementation, return to this planning and review workflow and review the code against:

- `docs/tasks/subagent-runtime-capability/review-checklist.md`

This task is not complete until that review closes.
