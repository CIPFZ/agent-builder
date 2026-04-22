# SSH Runtime Capability Review Fix Task

Date: 2026-04-23

## 1. Purpose

This document is the follow-up execution document for Claude Code after the first SSH implementation review.

Use this document only for closing review blocking issues.

Do not use it to expand scope or redesign the module.

## 2. Required Reading

Before coding, read these files in order:

1. `docs/tasks/ssh-runtime-capability/task.md`
2. `docs/tasks/ssh-runtime-capability/implementation-plan.md`
3. `docs/tasks/ssh-runtime-capability/review-checklist.md`
4. `docs/tasks/ssh-runtime-capability/review-fix-task.md`

After reading, output a short summary of:

- current incomplete items
- file targets for each item
- planned tests for each item

Then implement directly.

## 3. Scope

This task only covers fixing the review-blocking gaps in the first SSH implementation.

Do not add:

- SCP
- SFTP
- PTY
- tunnels
- jump hosts
- Docker follow-up work
- database follow-up work

## 4. Blocking Issues To Close

## Blocking Issue 1: SSH Permission Classification Is Incomplete

Current problem:

- SSH is not recognized by the current shell/system permission path.
- In some permission modes it is treated as a destructive non-system tool and can be allowed under the wrong semantics.

Primary target:

- `internal/permissions/policy.go`

Required outcome:

- SSH must follow the intended high-sensitivity approval path.
- SSH must not bypass the default approval semantics defined for this module.

## Blocking Issue 2: Structured Result And Progress Are Not Fully Propagated

Current problem:

- the tool produces `StructuredContent` and `Meta`
- but the query/runtime/gateway path does not fully preserve and expose them
- progress is not fully surfaced through the shared control-plane path

Primary targets:

- `internal/queryengine/queryengine.go`
- `internal/runtime/runner.go`
- `internal/gateway/server.go`

Required outcome:

- `tool.progress` must be observable through the shared runtime/control-plane path
- `tool.result` must not degrade to text-only behavior
- SSH structured result fields must be available to downstream control-plane consumers
- do not add an SSH-only side protocol

## Blocking Issue 3: QueryEngine Default Registry Does Not Include SSH

Current problem:

- SSH is registered in the runtime default registry
- SSH is not registered in the QueryEngine default registry

Primary target:

- `internal/queryengine/queryengine.go`

Required outcome:

- direct QueryEngine construction must also expose SSH

## Blocking Issue 4: Host Key Checking Default Has Hidden Local Side Effects

Current problem:

- the current executor default uses `StrictHostKeyChecking=accept-new`
- this can mutate local `known_hosts`
- that side effect is not aligned clearly with the current module semantics

Primary target:

- `internal/tools/system/ssh_executor.go`

Required outcome:

- either change to a more conservative default
- or keep the current behavior only if it is explicitly justified and aligned with the written module constraints

If it is kept:

- document the side effect in the final summary
- ensure the behavior is intentional, not accidental

## 5. Implementation Constraints

- only close the blocking review items
- preserve the first-class `SSH` tool design
- keep approval, permission, progress, and result on the shared runtime path
- avoid architecture drift
- do not ask the user to choose between multiple designs unless truly blocked

## 6. Test Requirements

At minimum, add or update tests for:

- SSH permission classification behavior
- QueryEngine default tool registry exposure
- tool progress propagation
- structured tool result propagation
- any changed SSH executor default behavior

Run the relevant tests after implementation.

## 7. Delivery Requirements

When finished, output:

- actual files changed
- which blocking issues are now closed
- tests added or updated
- commands run and their results
- any remaining risk

## 8. Starter Prompt

Use this prompt to continue the SSH module in Claude Code:

```text
Continue the SSH runtime capability module and only fix the review-blocking issues. Do not expand scope and do not redesign the architecture.

Before coding, read these files in order:
1. docs/tasks/ssh-runtime-capability/task.md
2. docs/tasks/ssh-runtime-capability/implementation-plan.md
3. docs/tasks/ssh-runtime-capability/review-checklist.md
4. docs/tasks/ssh-runtime-capability/review-fix-task.md

After reading, output:
- current incomplete items
- file targets for each item
- planned tests

Then implement directly.

Blocking issues to close:
1. SSH permission classification must follow the intended high-sensitivity path
2. SSH structured result and progress must propagate through queryengine -> runtime -> gateway -> client
3. QueryEngine default registry must include SSH
4. SSH executor host-key-checking behavior must be aligned and intentional

Constraints:
- do not add SCP/SFTP/PTY/tunnel/jump host
- do not add an SSH-only side protocol
- keep SSH as a first-class tool
- add focused tests and run relevant test commands

When finished, report:
- files changed
- which blocking issues are closed
- tests added or updated
- test commands and results
- remaining risks
```
