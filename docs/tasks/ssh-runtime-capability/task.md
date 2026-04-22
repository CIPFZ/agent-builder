# SSH Runtime Capability Task

Date: 2026-04-23

## 1. Task Purpose

This is the single execution document for Claude Code to implement the SSH runtime capability.

Claude Code should use this file as the primary task entrypoint.

This file consolidates:

- scope
- non-goals
- Claude Code source alignment
- Go ownership boundary
- implementation order
- validation requirements
- delivery format

Claude Code should not invent a new architecture outside this document and the referenced implementation rules.

## 2. Required Reading

Before writing code, read these files in order:

1. `docs/execution/implementation-rules.md`
2. `docs/tasks/ssh-runtime-capability/task.md`
3. `docs/tasks/ssh-runtime-capability/design.md`
4. `docs/tasks/ssh-runtime-capability/source-alignment.md`
5. `docs/tasks/ssh-runtime-capability/implementation-plan.md`
6. `docs/tasks/ssh-runtime-capability/test-validation-plan.md`
7. `docs/tasks/ssh-runtime-capability/review-checklist.md`

After reading, output a short execution summary before coding.

That summary must include:

- module objective
- module non-goals
- Claude semantic alignment points
- target Go files
- planned implementation order
- planned test and validation steps

## 3. Objective

Implement the first remote execution capability for `myclaw` by introducing a first-class `SSH` tool that executes approved remote commands and propagates progress and results through the runtime and `myclawd`.

## 4. In Scope

- first-class `SSH` tool
- structured SSH input schema
- SSH executor abstraction
- system `ssh` backend in v1
- permission and approval integration
- queryengine integration
- runtime registration
- gateway event forwarding
- focused unit and integration tests
- controlled functional validation

## 5. Out Of Scope

Do not implement any of the following in this task:

- SCP
- SFTP
- remote file sync
- remote patch upload
- PTY / interactive terminal
- tunnels
- jump host chains
- SSH config UI
- React UI work
- Docker module work
- database module work

## 6. Architecture Constraints

- SSH must be a first-class runtime tool.
- SSH must not be implemented as a wrapper around `system.run`.
- Approval, permission, progress, and result flow must stay on the shared runtime path.
- Do not add a separate SSH-only gateway protocol.
- Do not move approval or event logic into the frontend.
- Do not destabilize the local shell path just to fit SSH.

## 7. Claude Code Semantic Alignment

Use these Claude Code source areas as semantic references:

- `claude-code/src/Tool.ts`
- `claude-code/src/QueryEngine.ts`
- `claude-code/src/tools/BashTool/BashTool.tsx`
- `claude-code/src/tools/PowerShellTool/PowerShellTool.tsx`
- `claude-code/src/remote/*`
- `claude-code/src/bridge/*`
- `claude-code/src/cli/transports/*`

Carry over these semantics:

- structured tool contract
- centralized permission and approval flow
- tool progress lifecycle independent from final result
- transcript-safe tool invocation and tool result
- transport-neutral runtime events

## 8. Go Ownership Boundary

Primary new files:

- `internal/tools/system/ssh.go`
- `internal/tools/system/ssh_test.go`

Possible supporting files:

- `internal/tools/system/ssh_executor.go`
- `internal/tools/system/ssh_executor_test.go`

Existing files expected to change:

- `internal/tools/registry.go`
- `internal/runtime/runner.go`
- `internal/queryengine/queryengine.go`
- `internal/gateway/server.go`

Avoid changing `internal/sandbox/router.go` unless there is a small and clearly justified abstraction improvement.

## 9. Required Tool Contract

Implement `SSH` with this first-cut input model:

- required:
  - `host`
  - `command`
- optional:
  - `user`
  - `port`
  - `timeout`
  - `workdir`
  - `identity_file`

First-cut behavior:

- `Enabled = true`
- `ReadOnly = false`
- `Destructive = true`

Execution backend:

- use local system `ssh` binary in v1

## 10. Required Result And Progress Behavior

Internal result should preserve:

- host
- user
- port
- command
- stdout
- stderr
- exit code
- timeout state
- duration

Tool progress must emit at least:

- `ssh.started`
- `ssh.connecting`
- `ssh.running`
- `ssh.finished`
- `ssh.failed`

Progress payload should preserve:

- host
- user
- port
- command
- timeout state
- exit code when available

## 11. Required Implementation Order

Implement in this order:

1. SSH tool contract and input parsing
2. SSH executor abstraction
3. system `ssh` backend
4. progress emission
5. registry and runtime registration
6. queryengine approval / transcript compatibility
7. gateway event forwarding
8. tests
9. controlled validation

Do not jump ahead to Docker or DB follow-up work.

## 12. Validation Requirements

At minimum, cover:

- input parsing
- schema behavior
- command construction
- timeout behavior
- result shaping
- progress emission
- registry/runtime exposure
- approval flow
- queryengine tool lifecycle
- gateway event forwarding

Functional validation must include at least:

1. simple remote command success case
2. non-zero exit case
3. timeout case
4. approval-required case

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
You are implementing the myclaw SSH runtime capability.

Before coding, read these files in order:
1. docs/execution/implementation-rules.md
2. docs/tasks/ssh-runtime-capability/task.md
3. docs/tasks/ssh-runtime-capability/design.md
4. docs/tasks/ssh-runtime-capability/source-alignment.md
5. docs/tasks/ssh-runtime-capability/implementation-plan.md
6. docs/tasks/ssh-runtime-capability/test-validation-plan.md
7. docs/tasks/ssh-runtime-capability/review-checklist.md

After reading, output a short execution summary covering:
- objective
- non-goals
- Claude semantic alignment points
- target Go files
- implementation order
- test and validation plan

Then implement directly without asking me to make architecture decisions.

Constraints:
- SSH must be a first-class tool, not a wrapper around system.run
- keep approval / permission / progress / tool result on the shared runtime path
- do not add an SSH-only side protocol
- scope is remote command execution only
- add focused tests and run relevant validation

When finished, report:
- files changed
- completion mapped to implementation-plan.md
- tests run
- validation run
- remaining risks
```

## 15. Review Rule

After Claude Code finishes implementation, return to this planning/review workflow and review the code against:

- `docs/tasks/ssh-runtime-capability/review-checklist.md`

This task is not complete until that review closes.
