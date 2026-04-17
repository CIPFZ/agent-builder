# Main Branch Full Parity Review And Next Plan

Date: 2026-04-18

## Scope

This review compares the current Go runtime on latest `main` with the local Claude Code source at `C:\Users\ytq\work\ai\agent-builder\claude-code` using a strict 1:1 semantic standard.

The goal of this document is:

1. record the current parity status by runtime module
2. separate modules that are already materially replicated from modules that are only approximated
3. produce the next implementation order for the remaining replication work

This is a review and planning document. It does not claim implementation completeness unless the current Go source already demonstrates it.

## Source Baseline

Reviewed Go areas:

- `internal/app`
- `internal/runtime`
- `internal/queryengine`
- `internal/prompt`
- `internal/permissions`
- `internal/session`
- `internal/compaction`
- `internal/memory`
- `internal/workspace`
- `internal/agent`
- `internal/tools`
- `internal/orchestration`
- `internal/gateway`
- `internal/protocol`
- `internal/tui`

Reviewed Claude Code areas:

- `claude-code/src/setup.ts`
- `claude-code/src/bootstrap/state.ts`
- `claude-code/src/QueryEngine.ts`
- `claude-code/src/query.ts`
- `claude-code/src/context.ts`
- `claude-code/src/Tool.ts`
- `claude-code/src/services/compact/*`
- `claude-code/src/utils/model/*`
- `claude-code/src/utils/permissions/*`
- `claude-code/src/utils/processUserInput/*`
- `claude-code/src/utils/session*`
- `claude-code/src/memdir/*`
- `claude-code/src/utils/forkedAgent.ts`
- `claude-code/src/utils/worktree.ts`
- `claude-code/src/Task.ts`
- `claude-code/src/utils/swarm/*`
- `claude-code/src/bridge/*`
- `claude-code/src/cli/structuredIO.ts`
- `claude-code/src/cli/transports/*`
- `claude-code/src/utils/hooks/*`
- `claude-code/src/utils/plugins/*`

## Status Legend

- `Strong`: the Go implementation already matches the Claude subsystem shape closely enough that remaining work is incremental, not foundational
- `Partial`: the Go code has real structure and some source-aligned seams, but important source semantics are still absent
- `Missing`: no first-class subsystem exists yet, or the current code only covers a small approximation

## Overall Result

The Go runtime is no longer in an early skeleton state. MCP runtime, skill runtime, the central QueryEngine path, session recovery foundation, the WebSocket control plane, and the local TUI/runtime bridge all show substantial progress on `main`.

However, strict full-runtime parity is still far from complete. The largest remaining gaps are concentrated in:

- instruction loading and memory layering
- agent-definition and AgentTool semantics
- fork/isolation/background continuation
- team/swarm runtime
- hooks/plugin/LSP runtime

This means the next work should not return to MCP or skills first. The next correct focus is the long-running agent runtime contract that Claude Code builds on top of those already-merged surfaces.

## Module-By-Module Parity Status

| # | Runtime module | Status | Current Go evidence | Claude source pressure | Main gap |
|---|---|---|---|---|---|
| 1 | Startup and composition runtime | Partial | `internal/app/bootstrap.go`, `internal/runtime/runner.go` centralize session, permissions, MCP, skills, approvals, compaction | `src/setup.ts`, `src/bootstrap/state.ts` compose feature gates, bridge, hooks, plugins, worktree, auth, mode routing | no Claude-grade bootstrap state, feature gates, or unified mode router |
| 2 | QueryEngine session execution kernel | Strong | `internal/queryengine/queryengine.go` is a long-lived engine with tool loop, approvals, compaction, session state, ToolUseContext | `src/QueryEngine.ts`, `src/query.ts` | residual gaps in process-user-input state, hook lifecycle depth, and full app-state mutation surface |
| 3 | Context assembly system | Partial | `internal/queryengine/context_provider.go`, `internal/prompt/builder.go` provide user/system lanes, git status, current date, CLAUDE.md injection | `src/context.ts` | no full source ordering, include semantics, or cached CLAUDE.md and add-dir handling |
| 4 | Prompt assembly and model resolution | Partial | `internal/prompt/builder.go`, `internal/queryengine/model_resolution.go`, `internal/llm/openai_compatible.go` | `src/utils/model/model.ts`, `src/utils/systemPrompt.ts` | prompt layering and model capability routing are still narrower than Claude Code |
| 5 | Tool protocol and execution contract | Partial | `internal/tools/registry.go`, `internal/queryengine/queryengine.go`, runtime tool events and progress | `src/Tool.ts`, `src/services/tools/toolExecution.ts` | still missing full hook-mediated permission/result pipeline and richer decision/content semantics |
| 6 | Tool surface assembly and availability filtering | Partial | tool registry now assembles builtin, MCP, and skill tools; search and defer hints exist | Claude computes visible tools from gates, mode, capabilities, policy, plugin state | Go still lacks full source-equivalent filtering by runtime capability and policy metadata |
| 7 | Permissions, approvals, and safety boundary | Partial | `internal/permissions/policy.go`, `internal/approval/manager.go`, gateway permission control, plan/subagent derivation | `src/utils/permissions/*`, `src/types/permissions.ts` | missing full ToolPermissionContext, classifier-backed auto, dangerous-rule stripping, and broader prompt-avoidance semantics |
| 8 | Session persistence and recovery | Partial | `internal/session/manager.go`, `internal/session/recovery.go`, file store and pending-approval restore | `src/utils/sessionStorage.ts`, `src/utils/sessionRestore.ts`, related session state modules | transcript format and runtime recovery are still Go-specific; missing worktree/fork/task continuation state |
| 9 | Context compaction and history governance | Partial | `internal/compaction/service.go` has session-memory compact, block metadata support, microcompact hooks, recovery anchors | `src/services/compact/*` | no full reinjection pipeline, attachment economics, or all source compact variants |
| 10 | CLAUDE.md and workspace instruction memory | Partial | `internal/workspace/loader.go` loads root markdown files | `src/context.ts`, workspace and markdown loaders | only root-level file loading; no upward walk, includes, precedence, or managed/user/project/local layering |
| 11 | Session memory system | Missing | `internal/memory/service.go` stores summaries and simple entries per session | Claude maintains asynchronous session memory outside the main loop | no background extraction, no session-memory lifecycle, no resume-grade integration |
| 12 | Agent memory system | Missing | same `internal/memory/service.go` bucket store | Claude has scoped user/project/local agent memory | no per-agent scoped persistence or injection semantics |
| 13 | Memory snapshot and memory directory mechanics | Missing | no explicit memdir subsystem in Go | `src/memdir/*` | no inventory scan, snapshot sync, age metadata, or memory manifest |
| 14 | Agent definition loading and runtime assembly | Missing | `runtime.Options.AgentDefinitions` is only a seam; no source-grade loader | Claude loads built-in, plugin, user, project, policy agent definitions | missing full definition sources, merge precedence, validation, and active-agent filtering |
| 15 | AgentTool execution path | Partial | `internal/tools/agent_tool.go`, `internal/runtime/runner.go`, `internal/agent/manager.go` support spawn, list, wait, resume, steer, stop | Claude AgentTool differentiates fresh spawn, fork, background, isolation, identity, model, permission semantics | Go path is a usable delegator, not a full source-equivalent AgentTool |
| 16 | Task system | Partial | `internal/agent/manager.go`, TUI task panels, gateway task/subagent status methods | `src/Task.ts`, `src/tasks/*` | missing Claude task types, disk output protocol, task objects, and full status/output-file semantics |
| 17 | Agent lifecycle management | Partial | spawn, wait, stop, resume and runtime events exist; TUI surfaces task state | Claude background lifecycle includes richer transitions, idle/output-file behavior, and continuation semantics | no dedicated lifecycle state machine or background driver parity |
| 18 | Forked subagent path | Missing | `defaultSkillForkExecutor` currently spawns a child session; there is no cache-safe fork runtime | `src/utils/forkedAgent.ts` | no inherited prefix/fork context path, no fork-specific transcript or cache invariants |
| 19 | Agent isolation semantics | Missing | basic child session derivation only; no worktree/remote/cwd isolation model | `src/utils/worktree.ts`, remote isolation helpers | no worktree lifecycle, no remote isolation, no persisted isolation metadata |
| 20 | Agent background/resume continuation protocol | Partial | runner can resume prior child session if continuation is prompt-safe | Claude resumes sidechains with replacement state, worktree state, fork invariants, output files | resume exists, but not continuation-grade parity |
| 21 | Coordinator mode | Partial | `internal/orchestration/coordinator.go` tracks run state, suggestions, plan steps | `src/coordinator/*` and coordinator-mode prompt/runtime semantics | current coordinator is orchestration telemetry, not a true Claude coordinator runtime mode |
| 22 | Swarm, teammate, and team runtime | Missing | no teammate runtime, mailbox, or team file in Go | `src/utils/swarm/*`, `src/utils/teammate*`, `src/utils/team*` | whole subsystem absent |
| 23 | Leader permission bridge and team permission sync | Missing | no leader-worker permission relay in Go | `src/utils/swarm/leaderPermissionBridge.ts`, `permissionSync.ts` | absent |
| 24 | Structured IO and control protocol | Strong | `internal/protocol/ws/message.go`, `internal/gateway/server.go`, runtime events, approval controls, orchestration APIs | `src/cli/structuredIO.ts`, `src/cli/remoteIO.ts`, bridge messaging | Go now has a real machine-readable control plane; remaining gaps are about breadth, not existence |
| 25 | Transport layer | Partial | WebSocket gateway exists with control requests and liveness-like flow | `src/cli/transports/*` include SSE, WebSocket, hybrid uploaders, reconnect policies | no SSE/hybrid transport parity or full reconnect contract |
| 26 | Bridge and remote runtime entry | Partial | `internal/gateway/server.go`, `internal/app/daemon.go`, TUI/runtime bridge | `src/bridge/*`, `src/remote/*` | no source-grade remote control runtime, bridge API loop, session spawning, or environment registration |
| 27 | Hooks runtime | Missing | there are hook seams in QueryEngine options and orchestration, but no first-class user/runtime hooks subsystem | `src/utils/hooks/*`, `src/hooks/*` | no hook registry, config, file watchers, prompt/session/frontmatter/skill hooks |
| 28 | MCP integration runtime | Strong | `internal/tools/mcp_client.go`, `mcp_dynamic.go`, `mcp_oauth.go`, runner MCP discovery and prompt/resource integration | Claude MCP runtime in services/tools integration | runtime MCP surface is materially present; residual gaps are mostly downstream agent/plugin coupling |
| 29 | LSP integration runtime | Missing | no explicit LSP capability layer in Go | Claude plugin/LSP integration modules | absent |
| 30 | Skills and plugin loading runtime | Partial | skill discovery, frontmatter parsing, bundled skills, MCP prompt skill bridging are implemented in `internal/tools/skill_*` | Claude skills plus plugin loading affect commands, agents, hooks, output styles, MCP, LSP | skills are materially strong, but plugin runtime is still missing, so the combined module cannot be marked complete |

## What Is Already Materially Strong On Main

These areas are no longer the highest-priority parity blockers:

- QueryEngine as a long-lived session kernel
- MCP runtime surface including OAuth, resources, prompts, and dynamic tools
- structured WebSocket control plane and approval/control messages
- local TUI/runtime bridge for approvals, task panels, session resume, transcript/tool progress
- skill discovery and bundled skill execution surface

The review implication is important: the next phase should build on these surfaces, not revisit them first.

## What Is Still Fundamentally Missing

The remaining work is dominated by runtime semantics that Claude Code uses to make long-running agents safe and resumable:

- layered instruction memory (`CLAUDE.md`, managed, project, local, includes, upward walk)
- async session memory and scoped agent memory
- memdir snapshot and memory inventory
- agent definition loading and active-agent filtering
- forked subagents
- worktree and remote isolation
- continuation-grade background resume
- teammate/swarm/team runtime
- hooks runtime
- plugin and LSP runtime

## Parity Progress Snapshot

Strict module count against the 30-module inventory:

- `Strong`: 4
- `Partial`: 15
- `Missing`: 11

This is materially better than the earlier P0-only picture, but still well below full runtime parity. The current branch history closed MCP and skill parity work; the next major milestone has to close the long-running agent/runtime contract.

## Next Replication Order

### Phase 1: Instruction And Memory Foundation

Why first:

- Claude Code threads instruction loading, session memory, and agent memory through prompt assembly, compaction, resume, and agent execution
- without this layer, later agent parity work will sit on the wrong runtime model

Modules:

- 10. CLAUDE.md and workspace instruction memory
- 11. Session memory system
- 12. Agent memory system
- 13. Memory snapshot and memory directory mechanics

Deliverable:

- a source-aligned instruction and memory substrate that QueryEngine, compaction, resume, and future agents all consume

### Phase 2: Agent Definitions And AgentTool Parity

Why second:

- Claude AgentTool behavior depends on a real definition loader and runtime assembly layer
- current Go subagents are functionally useful but semantically much shallower than the source

Modules:

- 14. Agent definition loading and runtime assembly
- 15. AgentTool execution path
- 16. Task system
- 17. Agent lifecycle management

Deliverable:

- full agent-definition loading pipeline plus source-like AgentTool/task/lifecycle behavior

### Phase 3: Fork, Isolation, And Continuation

Why third:

- these are the runtime contracts that make Claude subagents resumable, cache-safe, and operationally isolated
- they depend on Phases 1 and 2 being correct first

Modules:

- 18. Forked subagent path
- 19. Agent isolation semantics
- 20. Agent background/resume continuation protocol

Deliverable:

- source-grade fork/background/resume behavior with worktree and continuation metadata

### Phase 4: Team Runtime

Why fourth:

- Claude swarm/teammate behavior is built on top of stable lifecycle, permissions, and continuation

Modules:

- 21. Coordinator mode
- 22. Swarm, teammate, and team runtime
- 23. Leader permission bridge and team permission sync

Deliverable:

- a real team runtime rather than generic orchestration telemetry

### Phase 5: Host Extensibility Completion

Why fifth:

- MCP and local structured IO are already strong enough that the remaining host work is about breadth and extension semantics
- plugin, hook, and LSP runtime should be built after the internal runtime contract is stable

Modules:

- 25. Transport layer
- 26. Bridge and remote runtime entry
- 27. Hooks runtime
- 29. LSP integration runtime
- 30. plugin runtime completion inside the combined skills/plugins module

Deliverable:

- remote-hosted and extension-capable runtime parity

## Immediate Task List

The next implementation stream should be created around these task packages, in this order:

1. `instruction-loader-parity`
   - implement upward workspace walk
   - add include semantics
   - add managed/user/project/local precedence
   - thread loader results through prompt/context assembly

2. `session-and-agent-memory-parity`
   - split session memory from agent memory
   - add scoped persistence
   - add async update/extraction workflow
   - feed compaction and resume from the new memory model

3. `memdir-parity`
   - add memory inventory scanning and snapshot types
   - add sync/update hints and age metadata
   - integrate with memory load paths

4. `agent-definition-and-agenttool-parity`
   - add definition sources and merge precedence
   - validate runtime fields
   - compute active agent surface
   - replace current simple subagent spawn path with source-like AgentTool logic

5. `task-lifecycle-parity`
   - introduce task objects, output-file handling, richer statuses, and lifecycle transitions

6. `fork-isolation-resume-parity`
   - implement fork runtime
   - implement worktree/cwd/remote isolation
   - persist and restore continuation metadata for background agents

## Implementation Rule For All Next Work

Every implementation task after this review must follow the same rule:

1. review the exact Claude source module before changing Go code
2. write the failing test first
3. run the failing test and capture the failure
4. implement the minimal source-aligned change
5. run focused tests
6. run the relevant functional flow

That rule is especially important for the next three epics because they affect persistence and continuation semantics and are easy to approximate incorrectly.

## Recommended Next Module To Execute

The next concrete execution target should be:

- `CLAUDE.md/instruction memory parity` plus `session/agent memory substrate`

Reason:

- it is the lowest missing layer that still blocks multiple later epics
- it will immediately improve context assembly, compaction, resume, and future agent-definition loading
- it does not require re-opening already-strong MCP or skill runtime work

## Review Conclusion

Latest `main` has materially progressed beyond the older P0 review. MCP parity and skill parity were real advances, and the runtime now has a serious execution/control backbone.

The project is not blocked on MCP. It is blocked on the long-running agent contract that Claude Code builds around instruction memory, memory persistence, agent definitions, fork/isolation, and team runtime. That is where the next full-cycle replication effort should stay until closed.
