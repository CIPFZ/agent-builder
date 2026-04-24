# Shell Runtime Capability Source Alignment

Date: 2026-04-25

## 1. Alignment Purpose

This file anchors the shell runtime capability module to Claude Code source semantics.

The goal is not line-by-line UI parity.

The goal is to make sure `myclaw` follows the same execution direction and does not drift into a project-specific shell model that later blocks parity.

## 2. Primary Claude Code Areas

Use these source areas as semantic references:

- `claude-code/src/tools/BashTool/BashTool.ts`
- `claude-code/src/tools/PowerShellTool/PowerShellTool.ts`
- `claude-code/src/tools/RunTool/RunTool.ts`
- `claude-code/src/Tool.ts`
- `claude-code/src/QueryEngine.ts`
- `claude-code/src/permissions.ts`
- surrounding runtime event and message-log handling for tool lifecycle behavior

## 3. Semantics To Preserve

### A. Shell Is A First-Class Tool Surface

Claude Code does not treat shell execution as an invisible implementation detail.

For `myclaw`, preserve:

- shell commands are explicit tools
- lifecycle flows through the shared runtime
- approval is policy-driven

### B. Policy Owns Approval

Claude Code keeps shell approval behavior in shared permission logic.

For `myclaw`, preserve:

- approval must come from runtime policy
- gateway/client must not become the approval authority
- shell sensitivity must remain explicit

### C. Shared Tool Lifecycle

Claude Code routes execution through a shared lifecycle that supports observability and control.

For `myclaw`, preserve:

- tool.called
- tool.progress
- tool.result
- run.error / approval events where appropriate

### D. Session Context Matters

Claude Code execution is aware of runtime context.

For `myclaw`, preserve:

- child session/worktree context must influence shell working directory
- execution should not silently ignore runtime isolation

### E. Reusable Execution Semantics

Claude Code uses consistent execution semantics across local and richer execution tools.

For `myclaw`, preserve:

- shell base semantics should be reusable by ssh and later execution modules
- do not create a local-only shell model that later conflicts with richer tools

## 4. Things Not To Copy Literally

Do not treat the following as required:

- exact TS implementation structure
- exact UI rendering behavior
- exact transport/event names where `myclaw` already has a stable equivalent

The requirement is semantic parity, not file-by-file syntax parity.

## 5. Go Mapping

Map Claude semantics into these Go areas:

- shell tool execution:
  - `internal/tools/system/run.go`
- runtime lifecycle:
  - `internal/queryengine/queryengine.go`
  - `internal/runtime/runner.go`
- permission behavior:
  - `internal/permissions`
  - queryengine permission hook path
- control plane:
  - `internal/gateway/server.go`

## 6. Review Standard

During implementation and review, reject changes that:

- move approval logic into clients
- bypass shared runtime lifecycle for shell
- ignore worktree/session context
- expose shell results only through ad hoc formatted text when structured runtime fields already exist
- create Docker/DB/ssh-specific execution semantics that should belong to the shell base layer
