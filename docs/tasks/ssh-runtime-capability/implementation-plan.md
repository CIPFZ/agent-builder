# SSH Runtime Capability Implementation Plan

Date: 2026-04-22

## 1. Objective

Implement the first remote execution capability for `myclaw` by introducing a first-class `SSH` tool that can execute approved commands on remote hosts and propagate structured progress and results through the runtime and `myclawd`.

## 2. Module Boundary

This module includes:

- SSH tool contract
- SSH executor abstraction
- approval and permission integration
- query/runtime/gateway event propagation
- focused tests and functional validation

This module excludes:

- SCP/SFTP
- remote patch application
- interactive PTY
- tunnels
- multi-hop/jump-host support
- secret-management productization

## 3. Design Decisions

## 3.1 First-Cut Capability

Implement only remote command execution.

Minimum input fields:

- `host`
- `command`

Optional first-cut fields:

- `user`
- `port`
- `timeout`
- `workdir`
- `identity_file`

Optional but deferred:

- `known_hosts_file`
- `strict_host_key_checking`
- `proxy_jump`
- `env`
- `agent_forwarding`

These are deferred because they expand the surface faster than the runtime and policy model can absorb safely in v1.

## 3.2 Execution Backend

Use the local system `ssh` binary in v1.

Reasons:

- fastest path to a real operator-grade capability
- easiest to validate in real environments
- lower implementation risk than immediately adopting a Go-native SSH stack
- keeps failure modes closer to operator expectations

The backend must still be wrapped behind a small internal interface so a future Go-native implementation can replace it without changing the tool contract.

## 3.3 Permission Classification

Treat all SSH commands as approval-sensitive and destructive by default in v1.

Required first-cut rule:

- `ReadOnly = false`
- `Destructive = true`

Reason:

- remote-side effects are not locally observable enough to classify safely
- correctness matters more than convenience for the first release

## 4. Target File-Level Changes

## 4.1 New Files

- `internal/tools/system/ssh.go`
- `internal/tools/system/ssh_test.go`

Recommended supporting files if needed:

- `internal/tools/system/ssh_executor.go`
- `internal/tools/system/ssh_executor_test.go`

## 4.2 Existing Files To Update

- `internal/tools/registry.go`
- `internal/runtime/runner.go`
- `internal/queryengine/queryengine.go`
- `internal/gateway/server.go`

Optional update only if really justified:

- `internal/sandbox/router.go`

The preferred approach is to avoid changing `router.go` for SSH v1 unless a clear shared execution abstraction is extracted without polluting the local execution path.

## 5. Planned Tool Contract

## 5.1 Input Schema

Recommended model-facing schema:

```json
{
  "type": "object",
  "properties": {
    "host": { "type": "string" },
    "user": { "type": "string" },
    "port": { "type": "number" },
    "command": { "type": "string" },
    "timeout": { "type": "number" },
    "workdir": { "type": "string" },
    "identity_file": { "type": "string" }
  },
  "required": ["host", "command"]
}
```

Recommended internal parsed struct:

```go
type SSHInput struct {
    Host         string
    User         string
    Port         int
    Command      string
    Timeout      time.Duration
    Workdir      string
    IdentityFile string
}
```

## 5.2 Result Contract

Recommended internal result shape:

```go
type SSHResult struct {
    Host       string
    User       string
    Port       int
    Command    string
    Stdout     string
    Stderr     string
    ExitCode   int
    TimedOut   bool
    DurationMs int64
}
```

Recommended `tools.ToolResult` usage:

- `Output` contains a compact text rendering for current compatibility
- `StructuredContent` contains the structured SSH result object
- `Meta` contains event-friendly fields that gateway can pass through without reparsing the output string

## 5.3 Progress Contract

Use generic `tools.ToolProgress` with SSH-specific `Type` values.

Required first-cut sequence:

- `ssh.started`
- `ssh.connecting`
- `ssh.running`
- `ssh.finished`
- `ssh.failed`

Recommended `Data` payload fields:

- `tool`
- `host`
- `user`
- `port`
- `command`
- `timed_out`
- `exit_code`

## 6. Execution Flow

## 6.1 Invocation Path

Target runtime flow:

1. model calls `SSH`
2. registry resolves `SSH`
3. permission layer evaluates with host-aware input
4. approval request captures structured SSH input if approval is required
5. query engine emits `tool.called`
6. SSH tool emits progress during execution
7. tool returns structured result
8. query engine appends tool result to transcript
9. gateway forwards generic tool events to clients

## 6.2 Command Construction

The SSH tool should build the final process arguments rather than interpolate one giant shell string when possible.

Recommended command pattern:

- executable: `ssh`
- args:
  - `-p <port>` when set
  - `-i <identity_file>` when set
  - `<user>@<host>` or `<host>`
  - remote command string

If `workdir` is set, wrap the remote command as:

- POSIX target assumption for v1:
  - `cd '<workdir>' && <command>`

Important limitation to document:

- v1 assumes the remote side is shell-compatible and does not attempt Windows remote PowerShell semantics

That is acceptable because the module target is the user's remote Linux-style project control workflow.

## 6.3 Timeout Behavior

Timeout must cover the whole SSH process, not just the remote command body.

Required behavior:

- timeout cancels the local `ssh` process context
- result marks `TimedOut = true`
- progress emits a terminal state that makes timeout explicit

## 7. Permission And Approval Plan

## 7.1 Tool Inspection Behavior

`SSH` should report:

- `Enabled = true`
- `ReadOnly = false`
- `Destructive = true`

This keeps policy evaluation conservative.

## 7.2 Approval Input Shape

Approval payloads must preserve:

- `host`
- `user`
- `port`
- `command`
- `workdir`
- `identity_file`

The approval UI can redact or suppress sensitive path details later, but the runtime must preserve the full structured input.

## 7.3 Future-Proofing

Even though v1 treats everything as destructive, the implementation should keep host-aware structured input so future rules can distinguish:

- allowed hosts
- allowed users
- allowed command prefixes
- read-only remote commands

## 8. Runtime Assembly Plan

## 8.1 Registry

Register `SSH` alongside `Bash`, `PowerShell`, and `system.run`.

Recommended visible name:

- `SSH`

Recommended search hint:

- `remote ssh command`

## 8.2 Query Engine

No SSH-specific query-engine branch should be introduced unless strictly necessary.

Instead:

- reuse current `ToolUseContext`
- reuse existing permission/approval event flow
- ensure `StructuredContent` and `Meta` survive tool result handling where needed

If query engine currently drops structured tool result data needed by downstream clients, extend the generic path instead of adding an SSH-specific fast path.

## 8.3 Runtime Runner

Update runtime default assembly so SSH is present in all standard runtimes.

This is required because the architecture says SSH is core execution surface, not optional frontend add-on.

## 9. Gateway / myclawd Plan

## 9.1 Existing Events To Reuse

Reuse:

- `tool.called`
- `tool.result`
- `permission.required`

Reuse with richer payload:

- tool progress events already transported through `Progress`

## 9.2 Payload Extensions

For `tool.called`, include:

- `tool_input_object`

For `tool.result`, add when available:

- structured result fields or a generic `tool_meta` object

For progress events, ensure the SSH `Data` object is forwarded intact.

Recommended rule:

- gateway payload extensions must remain generic enough that future Docker and DB tools can reuse the same path

## 10. Sequenced Implementation Order

## Phase 1: Tool Contract

- add SSH input parsing
- add schema
- add definition metadata
- add read-only/destructive classification

## Phase 2: Executor

- add SSH executor abstraction
- implement system-ssh backend
- capture stdout, stderr, exit code, duration, timeout

## Phase 3: Progress

- emit SSH progress through `tools.ToolProgress`
- verify terminal-state emission on success and failure

## Phase 4: Runtime Integration

- register `SSH`
- ensure query engine transcript and approval flow preserve structured input

## Phase 5: Gateway Integration

- forward SSH progress metadata
- expose structured input and result payloads to clients

## Phase 6: Validation

- run focused tests
- run controlled functional SSH flow

## 11. Risks And Countermeasures

## Risk 1

SSH is implemented as a thin wrapper over `system.run`.

Countermeasure:

- require separate tool name, schema, result type, and progress types

## Risk 2

Local sandbox routing gets polluted with remote execution logic.

Countermeasure:

- keep SSH executor separate from local shell router unless a clean shared abstraction emerges

## Risk 3

Gateway grows SSH-only transport shapes that later Docker/DB tools cannot reuse.

Countermeasure:

- only extend generic tool payloads

## Risk 4

Approval UX becomes misleading because only the command text is shown.

Countermeasure:

- preserve host/user/port in approval-facing structured input

## 12. Completion Criteria

This plan is complete when:

- `SSH` is invokable as a first-class tool
- runtime permission flow handles it correctly
- `myclawd` clients can observe SSH tool call, progress, approval, and result
- the next module can build Docker and DB remote operations on top of `SSH` instead of inventing another remote-execution path
