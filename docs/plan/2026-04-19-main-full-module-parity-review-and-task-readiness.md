# Main Full Module Parity Review And Task Readiness

Date: 2026-04-19

## Scope

This document records a full-module parity review of the Go runtime against the local Claude Code source at `C:\Users\ytq\work\ai\agent-builder\claude-code`.

This review is stricter than a runtime-only comparison. It treats the full Claude Code module tree as the source of truth, including:

- host runtime
- command system
- query and tool loop
- MCP and skills
- session persistence and resume
- subagents, worktree isolation, and swarm
- plugin and extension platform
- hooks, settings, feature control, telemetry, and LSP
- bridge, remote, server, and upstream proxy
- UI/runtime modes such as output styles, keybindings, vim, and voice

The baseline reviewed on the Go side is the remote `origin/main` snapshot at commit `35fcd57943aeaff3afe70800494b48c36bf33563`.

## Source Baseline

Reviewed Go areas:

- `cmd/*`
- `internal/*`

Reviewed Claude Code areas:

- `claude-code/src/*`
- `claude-code/src/services/*`
- `claude-code/src/tools/*`
- `claude-code/src/utils/*`
- `claude-code/src/commands/*`
- `claude-code/src/bridge/*`
- `claude-code/src/cli/*`
- `claude-code/src/remote/*`
- `claude-code/src/server/*`
- `claude-code/src/upstreamproxy/*`
- `claude-code/src/plugins/*`
- `claude-code/src/skills/*`
- `claude-code/src/state/*`
- `claude-code/src/tasks/*`
- `claude-code/src/keybindings/*`
- `claude-code/src/vim/*`
- `claude-code/src/voice/*`

## Status Legend

- `Strong`: shape and runtime semantics are already close; remaining work is mostly edge completion
- `Partial`: the Go implementation has meaningful structure, but important source semantics are still missing
- `Missing`: there is no first-class equivalent subsystem, or only a narrow approximation exists

## Top-Level Module Review

| Claude module | Go parity | Notes |
|---|---|---|
| `assistant` | Missing | no Claude assistant mode equivalent |
| `bootstrap` | Partial | basic runtime bootstrap exists, but no Claude-grade bootstrap state / feature gate orchestration |
| `bridge` | Partial | local gateway/TUI bridge exists; remote bridge lifecycle does not |
| `buddy` | Missing | absent |
| `cli` | Missing | no structured IO, transport stack, or host control protocol parity |
| `commands` | Missing | only a tiny CLI/TUI command surface compared with Claude command runtime |
| `components` | Missing | no equivalent React UI component layer |
| `constants` | Missing | only scattered inline constants on the Go side |
| `context` | Partial | user/system/workspace context exists, but not Claude's stable contract |
| `coordinator` | Partial | orchestration coordinator exists, but not Claude coordinator mode |
| `entrypoints` | Missing | no SDK-style entrypoint layer |
| `hooks` | Missing | only hook seams exist, not a hook runtime |
| `ink` | Missing | Go TUI is not Claude Ink UI |
| `keybindings` | Missing | absent |
| `memdir` | Missing | absent |
| `migrations` | Missing | absent |
| `moreright` | Missing | absent |
| `native-ts` | Missing | absent |
| `outputStyles` | Missing | absent |
| `plugins` | Missing | only skill/plugin traces, not a plugin platform |
| `query` | Partial | core loop exists, host input processing does not |
| `remote` | Missing | absent except small local approximations |
| `schemas` | Missing | absent |
| `screens` | Missing | absent |
| `server` | Partial | local websocket server exists, not Claude direct-connect server runtime |
| `services` | Partial | compact/MCP slices exist; most service platform modules do not |
| `skills` | Partial to Strong | one of the best-replicated areas |
| `state` | Missing | no Claude AppStateStore-class host state system |
| `tasks` | Partial | basic run/task registry exists, not full Claude task runtime |
| `tools` | Partial | strong core surface, but far from full Claude tool ecosystem |
| `types` | Missing | no equivalent centralized host contract layer |
| `upstreamproxy` | Missing | absent |
| `utils` | Partial | some core utilities replicated, most platform utilities missing |
| `vim` | Missing | absent |
| `voice` | Missing | absent |

## Subsystem Summary

### Stronger Areas

- QueryEngine core turn loop
- MCP runtime basics
- skill discovery, frontmatter parsing, bundled skills, and skill execution skeleton
- local TUI plus runtime bridge
- basic subagent spawn/wait/resume/stop flow

### Mid-Parity Areas

- prompt assembly
- permissions policy core
- session continuation and compact-boundary recovery
- worktree minimum viable isolation
- websocket control plane

### Weak Areas

- memdir and persistent memory substrate
- plugin and extension platform
- hooks runtime
- LSP integration
- structured CLI host protocol
- bridge/remote/server/upstream proxy
- coordinator/swarm/team runtime
- output styles
- keybindings, vim, voice, buddy

## Highest-Severity Gaps

### 1. Memory Substrate Is Not Source-Equivalent

The Go runtime still uses in-process memory buckets in `internal/memory/service.go`.

Claude Code relies on a file-backed memory substrate centered on `src/memdir/*`, plus session memory extraction and team memory sync. This affects:

- prompt quality
- resume quality
- agent memory persistence
- cross-session behavioral consistency

This is an architectural gap, not a small feature omission.

### 2. Plugin And Extension Platform Is Mostly Missing

Claude Code has a real platform layer covering:

- plugin discovery and validation
- plugin install/update/uninstall
- plugin-contributed commands
- plugin-contributed agents
- plugin-contributed skills
- plugin hooks
- plugin MCP and LSP servers
- plugin output styles
- plugin settings and policy

The Go runtime currently has only limited skill/plugin traces, not this platform.

### 3. Host Runtime And Command Surface Are Far Shallower

Go currently exposes a small host surface through:

- `cmd/myclaw/main.go`
- `internal/app/cli.go`
- `internal/tui/*`

Claude Code host behavior is much broader through:

- `src/main.tsx`
- `src/commands.ts`
- `src/cli/*`
- `src/entrypoints/*`

This means the Go runtime cannot yet be judged only as an "agent loop". A large amount of Claude product behavior still lives above the current Go host layer.

### 4. Subagent Runtime Is Only A Skeleton Relative To Claude

Go has usable single-subagent delegation, but Claude also has:

- forked agents
- background agents with persistent task lifecycle
- teammate/swarm runtime
- mailbox/send-message coordination
- remote-launched agents
- richer resume semantics

Current Go subagent behavior is operationally useful, but still materially shallower.

### 5. Bridge, Remote, Server, And Upstream Proxy Layers Are Not Replicated

Go has a local websocket gateway.

Claude Code additionally has:

- structured IO
- SSE / websocket / hybrid transports
- remote bridge session lifecycle
- direct-connect server session management
- upstream proxy for controlled remote environments

This is a major product/runtime gap and limits realistic parity testing in remote-hosted workflows.

## Full-Module Progress Estimate

Strict full-module parity estimate:

- overall parity: `25% - 30%`

By major area:

- query and tool loop: `45% - 50%`
- MCP: `55% - 65%`
- skills: `65% - 75%`
- permissions: `30% - 35%`
- prompt and context: `30% - 35%`
- session recovery: `35% - 40%`
- subagent/task lifecycle: `40% - 45%`
- worktree isolation: `30% - 35%`
- plugin/extension/LSP/hooks: `5% - 15%`
- bridge/remote/server/upstream proxy: `15% - 20%`
- UI/runtime modes and host shell: `10% - 20%`

## Task Readiness Assessment

This section is for practical validation rather than strict source review.

### What The Current Go Runtime Should Already Handle Reasonably

- short repository inspection tasks
- simple code reading and summarization
- basic local file edits
- basic shell-driven implementation tasks in one workspace
- MCP-backed information lookup when the configured servers are already available
- skill-driven prompt injection for focused local workflows
- simple single-subagent delegation

### What It Will Likely Struggle With

- long multi-stage tasks that rely on durable memory across turns
- tasks that require plugin-provided platform behavior
- tasks that depend on rich permission semantics and adaptive policy updates
- tasks that require Claude-grade background agents and later resume
- remote-hosted or bridge-controlled workflows
- collaborative team/swarm style decomposition
- workflows that depend on LSP or plugin host integration
- command-driven product behaviors that exist in Claude host layers but not in Go

### Practical Complexity Estimate

If Claude Code is treated as roughly "production-capable" on a broad range of engineering tasks, the current Go runtime is best viewed as:

- good for `small` tasks
- usable for some `medium` tasks
- not yet reliable for `large` Claude-style autonomous workflows

A practical sizing guide:

- `small`: usually supported
  - one repository
  - one clear goal
  - under roughly 5 to 15 tool actions
  - little or no need for resume, plugin integration, or advanced policy behavior
- `medium`: partially supported
  - several related edits
  - some search, test, and iteration
  - may benefit from one subagent
  - still best when the task stays local and mostly linear
- `large`: weak support today
  - many branches of work
  - background execution and later continuation
  - remote/bridge control
  - plugin/LSP/swarm cooperation
  - memory-intensive, long-horizon task management

### Expected Side-By-Side Behavior Against Claude Code

For the same task:

- on small local engineering tasks, the Go runtime should often look directionally similar
- on medium tasks, it may still complete the task, but will diverge sooner in runtime behavior and recovery semantics
- on large tasks, Claude Code should be substantially more stable because it has the host/runtime/platform layers that the Go runtime still lacks

In practical terms:

- for a focused "inspect, edit, test, summarize" task in one repo, the Go runtime may achieve `50% - 70%` of the observable usefulness
- for a multi-step task needing richer policy, resume, plugins, or subagent orchestration, the effective usefulness drops quickly
- for Claude-native platform tasks, the Go runtime may fail not because the model loop is wrong, but because the supporting host substrate is not there yet

## Suggested Validation Focus

Before further replication, practical validation should focus on these task classes:

1. small single-repo bugfix
2. small feature implementation with shell plus file edits
3. MCP-assisted lookup plus local edit
4. skill-triggered workflow
5. single subagent decomposition
6. interrupted task and resume attempt

These tests will show current real-world capability more honestly than only comparing code structure.

## Conclusion

The Go runtime is no longer a toy skeleton. It has real execution value already, especially on local, focused, linear engineering tasks.

But under a full Claude Code module-by-module standard, the current replication is still in the early-to-middle stage. The strongest parts are the local query/tool/MCP/skill path. The weakest parts are the host platform layers that make Claude Code robust across long-running, extensible, resumable, and remotely controlled workflows.
