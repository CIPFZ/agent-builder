# Claude Code Go Parity Review and Roadmap

Date: 2026-05-01

## Scope

This review compares the Go implementation in `cmd/` and `internal/` with the local Claude Code source snapshot in `claude-code/src` and the research notes in `claude-code/docs`.

The target is semantic parity, not line-count parity. The assessment separates:

- runtime kernel parity
- tool and permission semantics
- session, recovery, memory, and compaction
- subagent and task execution
- extension systems
- command and product-surface coverage

## Current Parity Estimate

Overall full Claude Code parity: about 35%.

Runtime-kernel parity is meaningfully higher, about 55%-65%, because the Go version already has a working Claude Code style execution loop:

- user prompt submission
- model streaming
- tool call detection
- tool permission checks
- approval request and continuation
- tool result transcript writes
- multi-pass tool loop
- session persistence and recovery
- compaction and memory hooks
- subagent spawning and continuation
- TUI, daemon, and WebSocket gateway surfaces

The complete product-level parity is lower because Claude Code includes a much larger command system, plugin marketplace, provider routing, auth, managed settings, remote/IDE flows, telemetry, feature gates, and mature security policy behavior.

## Layered Progress

| Area | Estimated parity |
| --- | ---: |
| QueryEngine and core runtime loop | 60%-65% |
| Tool loop, tool result, approval lifecycle | 55%-60% |
| Session, transcript, compaction, recovery | 50%-60% |
| Subagent, agent.task, background/resume | 45%-55% |
| TUI, daemon, gateway control plane | 35%-45% |
| MCP, Skill, LSP extension entry points | 35%-45% |
| Permission and safety semantics | 30%-40% |
| Slash commands | 15%-25% |
| Plugin, marketplace, managed config | 10%-20% |
| Auth, provider routing, telemetry, feature gates | 10%-20% |
| WebSearch, WebFetch, and external information tools | 15%-25% |

## Review Findings

### P1: Git context collection can hang model turns

File: `internal/queryengine/context_provider.go`

Default system context gathers git status using synchronous `exec.Command(...).Output()` calls without a context or timeout. During review, `go test ./...` timed out, and narrowing the test showed `TestQueryEngineToolSearchDoesNotRevealDeniedTools` stuck in `gitStatusSnapshot` while running git commands.

Impact:

- first model request can block before any LLM call
- context rebuild can hang on slow, locked, or credential-blocked git repositories
- Windows environments are especially exposed to child-process waits

Required direction:

- add bounded git command execution
- fail closed by omitting git context rather than blocking the turn
- cover with a test that simulates a hanging git command

### P1: File tools bypass workspace path policy

File: `internal/tools/filesystem_tools.go`

Filesystem tools execute `file_path` directly. The current permission policy mostly evaluates non-shell tools through read-only/destructive classification and the current workdir, not the target path.

Impact:

- `Read` can read absolute paths outside workspace roots
- `Write` and `Edit` can be allowed in `workspace-write` based on workdir even when `file_path` is outside the workspace
- this is below Claude Code's expected safety boundary for file actions

Required direction:

- add path-aware permission checks for `Read`, `Write`, `Edit`, `MultiEdit`, `Glob`, `Grep`, `LS`, and `NotebookEdit`
- resolve relative paths against session worktree or cwd consistently
- reject or require approval for paths outside configured workspace roots

### P1: Plan mode tool state is not wired into enforcement policy

File: `internal/tools/extended_tools.go`

`EnterPlanMode` and `ExitPlanMode` update `AppState.toolPermissionContext`, but the QueryEngine permission path evaluates `q.PermissionPolicyForSession`. Unless another layer mirrors AppState into the session policy, later tool calls are not actually constrained by plan mode.

Impact:

- plan mode is represented in tool state but not enforced as a permission boundary
- destructive and system actions may not be constrained the way Claude Code expects

Required direction:

- make plan mode a session permission policy transition
- preserve the pre-plan mode
- require approval or explicit transition when exiting plan mode
- add regression tests that a shell action after `EnterPlanMode` requires approval

### P2: Slash command surface is a small placeholder subset

File: `internal/commands/registry.go`

The default registry currently exposes a small set of local commands:

- `/help`
- `/permissions`
- `/model`
- `/memory`
- `/resume`
- `/compact`
- `/tasks`
- `/mcp`
- `/status`

Claude Code has a much larger command system with command-specific handlers, plugin commands, auth/config flows, review/security workflows, output styles, and installation flows.

Required direction:

- define the target command inventory
- mark commands as implemented, stubbed, delegated, or intentionally out of scope
- prioritize high-frequency runtime commands before product-growth commands

### P2: WebSearch is advertised but non-functional

File: `internal/tools/extended_tools.go`

`WebSearch` is registered and exposed as an enabled read-only tool, but invocation always returns `WebSearch backend is not configured`.

Impact:

- the model sees a capability that cannot succeed
- tool exposure does not match actual runtime capability

Required direction:

- gate `WebSearch` exposure behind configured backend availability, or
- implement a real backend adapter, or
- expose it as disabled/degraded in tool inventory and prompt context

## What Is Already Strong

The Go implementation is no longer a skeleton. The following pieces are already close enough to be useful as runtime foundations:

- QueryEngine request and tool loop structure
- native tool schema forwarding
- approval required, approve, reject, and continuation flow
- provider tool-use identity preservation
- tool result transcript semantics
- session metadata and recovery snapshot
- compaction boundary and summary memory
- agent task foreground and background execution
- worktree/cwd/isolation metadata for subagents
- MCP discovery and dynamic tool registration entry points
- skill loading, dynamic skill listing, and invoked-skill tracking
- gateway and TUI event projection

## Main Gaps Against Claude Code

### Security policy depth

Claude Code treats permissions as a full execution institution: mode, rules, path boundaries, dangerous shell patterns, plan mode, approval UX, and rule persistence all interact. The Go version has the shape of this system but still needs deeper enforcement at file path, shell parsing, and mode-transition boundaries.

### Command system coverage

The Go command registry is a minimal runtime interface. Claude Code's command surface is much broader and includes plugin commands, settings commands, auth, review workflows, install flows, output style controls, and specialized operational commands.

### Extension ecosystem

The Go version has MCP, Skill, LSP, and lifecycle inventory primitives, but Claude Code also has plugin marketplace behavior, builtin plugin policy, install/update/blocklist flows, schema validation, and managed configuration.

### Provider and enterprise behavior

Claude Code includes auth, provider routing, model capability restrictions, feature gates, telemetry, growth experiments, and managed policy behavior. The Go implementation currently focuses on basic LLM client compatibility.

### UI and product surface

The TUI and daemon are useful validation surfaces, but they are not yet a complete Ink UI parity implementation. Vim/keybindings, statusline, dialogs, permission management, command UX, and remote/IDE flows need further work.

## Recommended Implementation Path

### Phase 1: Stabilize safety and blocking behavior

Goal: make the existing runtime safe and reliable enough to serve as an SDK base.

Tasks:

1. Add timeout-aware git context collection.
2. Add path-aware permission checks for filesystem tools.
3. Wire plan mode into session permission policy.
4. Gate or implement WebSearch capability exposure.
5. Re-run `go test ./...` and keep the full suite under a bounded runtime.

Exit criteria:

- `go test ./...` passes without hangs.
- file tools cannot access outside workspace roots without approval or denial.
- plan mode affects actual permission decisions.
- no advertised builtin tool is guaranteed to fail because of missing backend.

### Phase 2: Formalize parity inventory

Goal: make progress measurable instead of impression-based.

Tasks:

1. Create a command parity matrix under `docs/review/`.
2. Create a tool parity matrix under `docs/review/`.
3. Create an extension parity matrix for MCP, Skill, LSP, and Plugin.
4. Add status labels: implemented, partial, stub, missing, intentionally out of scope.
5. Link each implemented item to Go files and tests.

Exit criteria:

- every major Claude Code command and tool has an explicit status
- missing work can be planned by module instead of rediscovered repeatedly

### Phase 3: Fill high-frequency Claude Code behavior

Goal: improve user-visible parity where it affects daily coding use.

Tasks:

1. Expand `/permissions`, `/model`, `/compact`, `/resume`, `/mcp`, and `/tasks` into real runtime workflows.
2. Add command lifecycle and plugin command loading semantics.
3. Improve shell safety classification, including PowerShell-specific parsing.
4. Improve file tool outputs to match Claude Code's model-facing format more closely.
5. Add regression tests for common coding-agent flows.

Exit criteria:

- the TUI can support common Claude Code sessions without relying on placeholder command output
- approval prompts and command outputs carry enough structured data for SDK consumers

### Phase 4: Extension and SDK boundary

Goal: turn the runtime into a reusable Go SDK foundation.

Tasks:

1. Define public SDK-facing interfaces outside `internal/`.
2. Stabilize event contracts for model, tool, approval, compaction, and subagent events.
3. Move provider-specific details behind narrow adapters.
4. Add plugin install/load/update policy.
5. Add examples for embedding the runtime in another Go program.

Exit criteria:

- a downstream Go app can create sessions, submit prompts, inspect tool approvals, continue runs, and receive events without depending on TUI or daemon internals

### Phase 5: Product parity expansion

Goal: selectively replicate Claude Code product behaviors that matter for this project.

Tasks:

1. Add managed settings and policy layers.
2. Add provider auth and model capability restrictions.
3. Add remote/IDE bridge contracts where needed.
4. Add telemetry only if product requirements need it.
5. Decide which Claude Code growth, marketplace, and enterprise features are intentionally out of scope.

Exit criteria:

- remaining gaps are explicit product choices, not unknown missing behavior

## Suggested Immediate Backlog

1. `fix(queryengine): bound git context snapshot commands`
2. `fix(tools): enforce workspace roots for filesystem tools`
3. `fix(queryengine): enforce plan mode through session policy`
4. `fix(tools): gate unavailable WebSearch from prompt exposure`
5. `docs(review): add command and tool parity matrices`

## Verification Notes

Commands run during review:

```powershell
go test ./...
```

Result:

- timed out after about 124 seconds
- root cause narrowed to `internal/queryengine` git context collection

Additional grouped test runs showed many packages passing, including:

- `internal/app`
- `internal/runtime`
- `internal/tui`
- `internal/permissions`
- `internal/tools` until the combined run reached the queryengine hang path
- most foundational packages such as `session`, `store`, `memory`, `model`, `config`, and `workspace`

The current verification state is therefore: broad coverage exists, but full-suite verification is blocked by the git context hang.
