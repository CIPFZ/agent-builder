# SSH Runtime Capability Design

Date: 2026-04-22

## 1. Objective

Introduce a first-class ssh runtime capability that allows `myclaw` to execute approved commands on remote hosts through the same runtime semantics used for local tools.

This is the first detailed module because it is the highest-leverage missing capability in the new architecture.

## 2. Scope

This task covers:

- remote command execution over ssh
- first-class tool contract
- policy and approval integration
- progress and result lifecycle
- `myclawd` event support

This task does **not** cover:

- full file transfer/sync workflows
- port forwarding
- interactive TTY sessions
- SSH config management UI
- React operator UI implementation

These are intentionally deferred.

## 3. Design Decision

The first ssh capability should cover **remote command execution only**.

This is the correct first cut because:

- it is the minimum capability needed for real project control
- it composes naturally with Docker and database administration
- it fits the current runtime and approval model
- it avoids prematurely taking on file-sync and interactive session complexity

So the initial contract is:

- connect to a remote host
- run a command
- stream progress
- return stdout/stderr and execution metadata

## 4. Current Go Ownership Points

The current best ownership points are:

- `internal/tools/system/run.go`
- `internal/sandbox/router.go`
- `internal/tools/registry.go`
- `internal/runtime/runner.go`
- `internal/queryengine/queryengine.go`
- `internal/gateway/server.go`

This indicates that ssh should be implemented as a sibling execution capability to shell tools, not as a one-off side feature elsewhere.

## 5. Claude Code Reference

Claude Code does not provide a direct `SshTool` module to clone, so alignment must follow runtime/tool/control-plane semantics.

Primary reference areas:

- `claude-code/src/Tool.ts`
- `claude-code/src/QueryEngine.ts`
- `claude-code/src/tools/BashTool/BashTool.tsx`
- `claude-code/src/tools/PowerShellTool/PowerShellTool.tsx`
- `claude-code/src/cli/structuredIO.ts`
- `claude-code/src/cli/transports/*`
- `claude-code/src/bridge/*`
- `claude-code/src/remote/*`

Alignment target:

- ssh behaves like a first-class tool invocation with the same policy, approval, progress, and transcript expectations
- transport and UI do not invent ssh-specific semantics outside the runtime

## 6. Target Design

## 6.1 Tool Surface

Introduce a new first-class runtime tool:

- `SSH`

Initial input contract:

```json
{
  "host": "prod-1.example.com",
  "user": "deploy",
  "port": 22,
  "command": "docker ps",
  "timeout": 30000,
  "workdir": "/srv/app",
  "identity_file": "~/.ssh/id_ed25519"
}
```

Required fields for the first cut:

- `host`
- `command`

Optional fields:

- `user`
- `port`
- `timeout`
- `workdir`
- `identity_file`

Not in first cut:

- jump host chains
- agent forwarding configuration
- env var maps
- inline secrets transport

## 6.2 Runtime Execution Model

The ssh tool should be implemented as an execution tool, not as a generic MCP wrapper and not as a frontend-side helper.

Suggested structure:

- a new execution abstraction under `internal/tools/system` or a closely related execution package
- a dedicated ssh runner that invokes the local `ssh` client initially
- structured result capture

The first implementation should use the system `ssh` client rather than a native Go SSH stack.

Reason:

- lower implementation risk
- easier parity with real operator workflows
- keeps behavior closer to what users expect on dev machines and servers

The runtime can later swap the backend if needed.

## 6.3 Approval And Policy Model

ssh must always be treated as a high-sensitivity execution capability.

Default first-cut policy behavior:

- remote execution requires approval unless explicit permission rules allow it
- all ssh executions are treated as non-read-only by default, even if the command looks read-only

Why:

- remote side effects are too hard to classify safely in v1
- this keeps the system correct before it becomes convenient

Later optimization:

- add optional remote read-only classification rules after the core capability is stable

## 6.4 Progress Lifecycle

The ssh tool should emit the same class of lifecycle signals as other execution tools.

Minimum progress states:

- `ssh.started`
- `ssh.connecting`
- `ssh.running`
- `ssh.finished`
- `ssh.failed`

Payload should include:

- `tool_use_id`
- `host`
- `user`
- `command`
- `timed_out`
- optional `exit_code`

## 6.5 Result Contract

The result must be structured enough for:

- transcript persistence
- TUI rendering
- future React rendering
- subagent reuse

Minimum result envelope:

```json
{
  "host": "prod-1.example.com",
  "user": "deploy",
  "command": "docker ps",
  "stdout": "...",
  "stderr": "",
  "exitCode": 0,
  "timedOut": false,
  "durationMs": 1234
}
```

The output can still be rendered as text for current compatibility, but backend contracts should normalize around the structured model.

## 6.6 myclawd Protocol Impact

`myclawd` must not treat ssh as a special frontend-only feature.

Needed protocol support:

- execution progress events carry ssh metadata
- approval prompts preserve structured ssh input
- final tool result events preserve structured ssh result fields

This should reuse the generic execution-tool event framework where possible instead of inventing a separate ssh protocol.

## 7. File And Module Impact

Expected primary impact:

- `internal/tools/system/run.go`
- new ssh execution file under `internal/tools/system/`
- `internal/tools/registry.go`
- `internal/queryengine/queryengine.go`
- `internal/runtime/runner.go`
- `internal/gateway/server.go`
- tests in corresponding packages

Possible new files:

- `internal/tools/system/ssh.go`
- `internal/tools/system/ssh_test.go`

## 8. Sequencing

### Step 1

Define the ssh tool contract and runtime result model.

### Step 2

Implement the ssh runner on top of the local `ssh` executable.

### Step 3

Integrate approval and policy behavior.

### Step 4

Add runtime progress emission.

### Step 5

Extend `myclawd` event payload compatibility.

### Step 6

Add focused tests and functional validation flow.

## 9. Risks

### Risk 1: Over-designing SSH Up Front

Avoid:

- file sync
- TTY mode
- tunnels
- jump hosts
- multiplexing

The first target is command execution only.

### Risk 2: Treating SSH As Just Another Shell String

That would make approval, protocol, and UI integration weak.

SSH must be a first-class runtime capability.

### Risk 3: Trying To Solve Remote File Editing In The Same Task

Remote file editing can later be layered on:

- ssh command execution
- remote shell tools
- or a future dedicated remote file capability

It should not block the first ssh module.

## 10. Non-Goals

- remote file browser
- remote patch upload
- remote interactive terminal emulator
- remote session persistence model
- SSH frontend UX

## 11. Acceptance Criteria

The module is done when:

- the model can invoke `SSH` as a first-class tool
- remote command execution works through runtime contracts
- approval and permission flow works correctly
- progress and final result are visible through runtime and `myclawd`
- downstream Docker and DB work can build on top of ssh instead of bypassing it

## 12. Validation Plan

### Unit Tests

- input parsing and defaults
- command construction
- timeout behavior
- result shaping
- approval gating behavior

### Runtime Tests

- QueryEngine can invoke ssh tool
- structured input survives tool call lifecycle
- progress events are emitted
- result is persisted and surfaced correctly

### Functional Validation

Minimum functional flow:

1. invoke ssh tool against a controlled test host
2. run a read-only command like `pwd`
3. verify approval behavior
4. verify stdout/stderr/exit code capture
5. verify progress visibility through runtime and gateway

