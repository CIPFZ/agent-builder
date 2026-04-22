# SSH Runtime Capability Source Alignment

Date: 2026-04-22

## 1. Alignment Goal

Claude Code does not have a direct `SshTool` file to replicate, so SSH alignment must follow Claude Code's existing execution-tool semantics rather than filename parity.

The alignment target is:

- tool invocation remains first-class
- permission and approval stay centralized
- progress and result events stay transport-neutral
- frontend clients consume runtime events instead of inventing SSH logic locally

## 2. User Requirement Alignment

The user's practical target is not "generic remote access".

The target is:

- let `myclaw` control a real Go web project
- allow later remote Docker operations
- allow later remote database operations
- let subagents and runtime chaining use the same capability

That means SSH v1 must be designed as shared runtime infrastructure, not a temporary shell shortcut.

## 3. Claude Code Reference Areas

## 3.1 Tool Contract

Primary references:

- `claude-code/src/Tool.ts`
- `claude-code/src/tools/BashTool/BashTool.tsx`
- `claude-code/src/tools/PowerShellTool/PowerShellTool.tsx`

Semantics to carry over:

- structured tool input schema
- read-only vs destructive classification exists at tool layer
- tool-specific validation and permission checks happen before execution
- progress is emitted as typed tool progress, not raw log text
- result shaping is part of the tool contract

## 3.2 Query Execution And Approval Loop

Primary references:

- `claude-code/src/QueryEngine.ts`
- `claude-code/src/query.ts`

Semantics to carry over:

- permission checks happen before the tool call
- approval requests preserve the concrete tool input
- tool call and tool result are part of the transcript lifecycle
- tool progress is independent from final tool result

## 3.3 Remote / Bridge / Transport

Primary references:

- `claude-code/src/remote/remotePermissionBridge.ts`
- `claude-code/src/remote/SessionsWebSocket.ts`
- `claude-code/src/bridge/*`
- `claude-code/src/cli/transports/*`

Semantics to carry over:

- remote/control-plane transports carry runtime events
- approval callbacks preserve structured tool information
- clients do not own execution semantics
- transport payloads should be stable across multiple frontends

## 4. Go Ownership Mapping

## 4.1 Tool Layer

Primary Go files:

- `internal/tools/system/run.go`
- `internal/tools/registry.go`

Target mapping:

- `run.go` is the current execution-tool reference implementation
- SSH should be added as a sibling execution tool, not baked into `RunTool`
- the registry must expose SSH like any other first-class tool

Planned ownership:

- `internal/tools/system/ssh.go`
- `internal/tools/system/ssh_test.go`

## 4.2 Execution Routing Layer

Primary Go file:

- `internal/sandbox/router.go`

Target mapping:

- current router only distinguishes host vs sandbox local execution
- SSH must not be forced through the existing local shell execution path
- SSH needs a dedicated execution path with its own request model

Planned ownership:

- either extend the router with explicit remote execution entrypoints
- or add a sibling execution adapter used by `SSH` while keeping local shell routing unchanged

Recommended choice:

- keep local shell router behavior stable
- introduce a dedicated SSH executor abstraction instead of overloading `Router.Run`

Reason:

- local sandbox routing and remote SSH policy are different concerns
- collapsing both into one generic `Run(command string)` shape will make later Docker and DB tooling harder to reason about

## 4.3 Query / Permission / Transcript Layer

Primary Go file:

- `internal/queryengine/queryengine.go`

Target mapping:

- this already owns permission evaluation, approval creation, tool.called, tool.result, and tool progress forwarding
- SSH should fit into the existing lifecycle without special-case transcript behavior

Implementation consequence:

- SSH tool progress types must remain compatible with `tools.ToolProgress`
- SSH input must survive approval and transcript serialization without losing structure

## 4.4 Runtime Assembly Layer

Primary Go file:

- `internal/runtime/runner.go`

Target mapping:

- runtime assembly currently registers shell tools and other core tools
- SSH must be registered here by default so it becomes part of the main runtime surface

Implementation consequence:

- the runner must construct the SSH dependencies once and share them with the tool registry
- SSH cannot rely on frontend-only injection

## 4.5 Gateway / Control Plane Layer

Primary Go file:

- `internal/gateway/server.go`

Target mapping:

- gateway already forwards `tool.called`, `tool.result`, and `permission.required`
- SSH should reuse the same event families and only extend payload richness where necessary

Implementation consequence:

- do not add a separate "ssh-only websocket protocol"
- extend generic tool payloads so current TUI and later React UI can both consume them

## 5. Semantic Differences To Keep Explicit

SSH is not equivalent to local Bash or PowerShell.

Important differences:

- execution target is remote
- local worktree path semantics do not automatically apply
- command safety classification is less trustworthy
- credentials and host identity become part of the execution contract

Therefore SSH must align with Claude's execution semantics, but it must not pretend to be just another local command string.

## 6. Alignment Rules For This Module

Implementation is aligned only if all of the following remain true:

1. SSH is registered as a first-class tool.
2. SSH input remains structured through permission, approval, transcript, and gateway layers.
3. Approval logic does not rely on frontend heuristics.
4. Progress uses generic tool progress transport, not ad hoc console text.
5. Gateway and future React UI consume the same runtime event model.
6. Local shell execution behavior is not destabilized just to make SSH fit.

## 7. Drift Indicators

The SSH implementation is drifting away from the target architecture if any of the following happen:

- SSH is exposed only as `system.run` command templating
- approval is based only on `command` text and ignores host data
- gateway adds SSH-only event types where generic tool progress would be enough
- remote path handling is mixed into local worktree routing
- frontend code starts constructing SSH commands instead of the runtime owning the contract
