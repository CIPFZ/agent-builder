# Claude Code Agent Runtime Full Replication Plan

**Goal:** record every Claude Code agent runtime module that must be replicated in Go so the project can target internal architecture parity without dropping hidden runtime seams.

**Replication standard:** the target is not API similarity or behavioral approximation. The target is Claude Code runtime architecture parity at the module and responsibility level, with Go used only as the implementation language.

**Scope rule:** this document tracks runtime modules only. Product UI polish, non-runtime product surfaces, and unrelated builder ideas are out of scope unless they are required by the runtime architecture itself.

---

## Core principle

Claude Code is not just:

- a query loop
- a tool registry
- a permissions switch

It is a long-running, recoverable, governable, delegating, host-integratable agent runtime.

For this project, any module that materially participates in that runtime must be treated as a required replication target.

## Execution rule

Before any formal replication work starts, the workflow rule is:

1. existing tests first
2. then replication
3. after replication, mandatory functional testing

This rule applies to every runtime module, not just user-facing features.

### Required interpretation

- "existing tests first" means the current relevant test suite must be run before changing a target module, so the baseline is known.
- "then replication" means implementation happens only after the current baseline has been checked.
- "mandatory functional testing" means replication is not considered complete after unit tests alone; the resulting runtime behavior must also be exercised through realistic functional flows.

### Minimum verification contract for every replication task

Each future implementation plan must explicitly include:

- baseline test commands to run before edits
- implementation scope
- post-change unit/integration test commands
- post-change functional test flows

If a future plan does not include all four, it is incomplete.

---

## Layer 1: Runtime core

### 1. Startup and composition runtime

**Claude Code responsibility:**
- bootstraps config, gates, policies, bootstrap state
- initializes plugins, skills, MCP, LSP, auth-related runtime dependencies
- assembles commands, tools, agents, app state, runtime mode
- routes into REPL, bridge, remote, assistant, SDK-hosted execution

**Go replication target:**
- a single runtime composition entry that wires all runtime subsystems
- explicit mode routing instead of ad hoc initialization spread across commands

**Current Go mapping:**
- `internal/app`
- `internal/runtime`
- `internal/config`

### 2. QueryEngine session execution kernel

**Claude Code responsibility:**
- owns the long-lived conversation container
- manages turn submission, tool loop, usage tracking, abort, permission denials
- owns message history, read-file state, discovered skills, loaded memory paths
- acts as the runtime execution center, not a thin LLM wrapper

**Go replication target:**
- full long-lived QueryEngine with persistent per-session state and turn lifecycle control

**Current Go mapping:**
- `internal/queryengine`
- `internal/engine`

### 3. Context assembly system

**Claude Code responsibility:**
- builds `system context` and `user context`
- injects workspace state, git state, current date, memory, CLAUDE.md, extra directories
- separates runtime context sources instead of flattening them into one prompt blob

**Go replication target:**
- layered context provider pipeline with explicit source ordering and reusable context state

**Current Go mapping:**
- `internal/prompt`
- `internal/workspace`

### 4. Prompt assembly and model resolution system

**Claude Code responsibility:**
- resolves effective system prompt via override/coordinator/agent/custom/default/append layering
- resolves effective main-loop model from session overrides, CLI flags, env, settings, provider aliases, and mode
- treats prompt and model selection as runtime policy, not static config

**Go replication target:**
- prompt composer and model resolver with the same priority rules and runtime branching

**Current Go mapping:**
- `internal/prompt`
- `internal/llm`
- `internal/model`

### 5. Tool protocol and execution contract

**Claude Code responsibility:**
- defines tool schema, tool context, permission context, progress events, structured results, transcript integration
- makes tool invocation a controlled runtime protocol rather than naked function calls

**Go replication target:**
- unified tool interface carrying execution context, permission state, progress, result schema, and transcript semantics

**Current Go mapping:**
- `internal/tools`
- `internal/permissions`
- `internal/sandbox`

### 6. Tool surface assembly and availability filtering

**Claude Code responsibility:**
- computes session-visible tools from the full base tool set
- filters by feature gate, platform, user type, mode, external capability state

**Go replication target:**
- runtime tool surface builder instead of static registry-only exposure

**Current Go mapping:**
- `internal/tools`

### 7. Permissions, approvals, and safety boundary

**Claude Code responsibility:**
- manages permission modes
- evaluates allow/deny/ask rules from multiple sources
- detects dangerous rules and unsafe auto-approval patterns
- integrates plan mode and subagent permission semantics into runtime execution

**Go replication target:**
- first-class permission subsystem embedded into tool execution and agent lifecycle

**Current Go mapping:**
- `internal/permissions`
- `internal/approval`
- `internal/sandbox`

### 8. Session persistence and recovery

**Claude Code responsibility:**
- persists transcript JSONL and session metadata
- restores session mode, agent identity, model override, worktree state, todos, context-collapse state
- cleans transcript history into a replayable continuation state

**Go replication target:**
- log-based persistence and continuation-grade restore, not just message reload

**Current Go mapping:**
- `internal/session`
- `internal/store`

### 9. Context compaction and history governance

**Claude Code responsibility:**
- auto-compact thresholding
- microcompact for expensive tool results
- session-memory compact
- traditional compact
- post-compact cleanup and attachment reinjection

**Go replication target:**
- multi-layer compaction pipeline for long-running sessions

**Current Go mapping:**
- `internal/compaction`

---

## Layer 2: Long-running memory and state continuity

### 10. CLAUDE.md and workspace instruction memory

**Claude Code responsibility:**
- loads managed, user, project, and local instruction memory
- walks upward by cwd
- supports include semantics and precedence rules

**Go replication target:**
- workspace instruction loader with deterministic precedence and recursive assembly

**Current Go mapping:**
- `internal/memory`
- `internal/workspace`

### 11. Session memory system

**Claude Code responsibility:**
- maintains rolling session memory outside the main loop
- updates memory asynchronously using delegated extraction
- feeds compact and resume flows

**Go replication target:**
- background-maintained session memory lifecycle, not just in-turn summarization

**Current Go mapping:**
- `internal/memory`

### 12. Agent memory system

**Claude Code responsibility:**
- persists memory by agent identity and scope
- supports `user`, `project`, and `local` scopes
- injects persistent agent memory into agent runtime

**Go replication target:**
- scoped persistent memory keyed by agent type and runtime environment

**Current Go mapping:**
- `internal/memory`
- `internal/agent`

### 13. Memory snapshot and memory directory mechanics

**Claude Code responsibility:**
- scans structured memory directories
- handles agent memory snapshot sync and update hints
- treats memory as an inventory, not a single file

**Go replication target:**
- memory manifest, snapshot, and sync protocol layer

**Current Go mapping:**
- currently missing as an explicit subsystem

---

## Layer 3: Agent runtime and delegation

### 14. Agent definition loading and runtime assembly

**Claude Code responsibility:**
- loads built-in, plugin, user, project, flag, and policy agent definitions
- merges definitions by source priority
- validates runtime-relevant fields such as tools, disallowed tools, permission mode, background, isolation, memory, hooks, MCP requirements, omitClaudeMd
- computes session-visible agent set

**Go replication target:**
- runtime agent-definition loader and filtered active-agent surface

**Current Go mapping:**
- partial `internal/agent`
- major missing subsystem

### 15. AgentTool execution path

**Claude Code responsibility:**
- spawns subagents from the tool layer
- selects fresh-agent path vs fork path
- manages model, mode, isolation, cwd, background, prompt, identity

**Go replication target:**
- dedicated AgentTool-equivalent module, not generic orchestration glue

**Current Go mapping:**
- `internal/agent`
- `internal/orchestration`

### 16. Task system

**Claude Code responsibility:**
- treats agents as task objects
- lists and manages background tasks
- supports status, output files, foreground/background transitions, resume points

**Go replication target:**
- explicit task registry and task lifecycle model

**Current Go mapping:**
- partial `internal/agent`
- partial `internal/runtime`

### 17. Agent lifecycle management

**Claude Code responsibility:**
- foreground run
- background transition
- async lifecycle driver
- task notifications
- resume continuation
- final result and output file management

**Go replication target:**
- full agent lifecycle state machine

**Current Go mapping:**
- partial `internal/agent`
- partial `internal/runtime`

### 18. Forked subagent path

**Claude Code responsibility:**
- supports context-inheriting fork path separate from fresh subagents
- preserves cache-identical prompt prefixes
- isolates intermediate noise from main context
- blocks recursive fork behavior

**Go replication target:**
- explicit fork runtime path with inherited rendered prompt and inherited message prefix semantics

**Current Go mapping:**
- missing

### 19. Agent isolation semantics

**Claude Code responsibility:**
- supports `worktree` isolation
- supports `remote` isolation
- supports `cwd` override
- persists and restores isolation metadata
- cleans up retained or disposable worktrees

**Go replication target:**
- three distinct isolation mechanisms with different runtime semantics

**Current Go mapping:**
- `internal/workspace`
- `internal/sandbox`
- major gaps remain

### 20. Agent background/resume continuation protocol

**Claude Code responsibility:**
- resumes agents from transcript plus metadata
- restores replacement state, worktree state, and fork-specific prompt invariants
- continues existing agent identity instead of re-spawning a lookalike worker

**Go replication target:**
- continuation-grade resume protocol for agents and tasks

**Current Go mapping:**
- partial `internal/agent`
- partial `internal/session`

---

## Layer 4: Multi-agent organization runtime

### 21. Coordinator mode

**Claude Code responsibility:**
- defines a dedicated runtime mode for synthesis and delegation
- changes prompt shape and work partitioning strategy
- makes coordinator an orchestrator role, not just a stronger agent

**Go replication target:**
- explicit coordinator mode with dedicated prompt and runtime semantics

**Current Go mapping:**
- early hints in `internal/orchestration`
- mostly missing

### 22. Swarm, teammate, and team runtime

**Claude Code responsibility:**
- models leader, teammate, team identity, team mailbox, idle state, permission escalation, and persistent team file
- supports in-process teammate execution and team-level permission sync

**Go replication target:**
- team runtime, not just multiple goroutines or child agents

**Current Go mapping:**
- mostly missing

### 23. Leader permission bridge and team permission sync

**Claude Code responsibility:**
- routes worker permission requests to leader approval
- synchronizes permission state across teammates

**Go replication target:**
- delegation-aware approval bridge

**Current Go mapping:**
- missing

---

## Layer 5: Host integration and runtime extensibility

### 24. Structured IO and control protocol

**Claude Code responsibility:**
- exposes machine-readable runtime events
- transports permission requests, hook outputs, session state changes, tool lifecycle messages, and control requests
- supports external hosts and SDK-driven sessions

**Go replication target:**
- runtime control-plane protocol independent of TUI rendering

**Current Go mapping:**
- `internal/protocol`
- `internal/gateway`

### 25. Transport layer

**Claude Code responsibility:**
- supports SSE, WebSocket, hybrid control transports
- handles reconnect, liveness, retry policy, and remote-host runtime connectivity

**Go replication target:**
- transport abstraction for hosted runtime control

**Current Go mapping:**
- `internal/gateway`
- partial `internal/protocol`

### 26. Bridge and remote runtime entry

**Claude Code responsibility:**
- lets the same runtime operate under remote or IDE-hosted control surfaces
- separates runtime execution from local REPL assumptions

**Go replication target:**
- bridge/remote runtime ingress layer

**Current Go mapping:**
- `internal/gateway`
- `internal/app/daemon.go`

### 27. Hooks runtime

**Claude Code responsibility:**
- supports session hooks, prompt hooks, HTTP hooks, skill/frontmatter hooks
- allows runtime-time rule injection and side-effect interception around the agent loop

**Go replication target:**
- official hook points in runtime lifecycle

**Current Go mapping:**
- missing as a first-class subsystem

### 28. MCP integration runtime

**Claude Code responsibility:**
- integrates MCP clients, resource listing, resource read, MCP-driven tool availability, and agent requirements

**Go replication target:**
- MCP runtime as part of the tool and agent surface computation

**Current Go mapping:**
- partial support under `internal/tools`
- no fully separated MCP subsystem yet

### 29. LSP integration runtime

**Claude Code responsibility:**
- treats language-service integration as runtime capability, not editor-only sugar

**Go replication target:**
- pluggable LSP-backed capability layer

**Current Go mapping:**
- missing as an explicit subsystem

### 30. Skills and plugin loading runtime

**Claude Code responsibility:**
- loads skills and plugins as runtime extensions
- allows them to affect prompt, hooks, commands, agents, and capabilities

**Go replication target:**
- extension-loading runtime, not just static tool additions

**Current Go mapping:**
- missing as a complete subsystem

---

## Required module inventory checklist

Use this checklist as the anti-omission baseline for future planning:

- [ ] Startup and composition runtime
- [ ] QueryEngine session execution kernel
- [ ] Context assembly system
- [ ] Prompt assembly and model resolution system
- [ ] Tool protocol and execution contract
- [ ] Tool surface assembly and availability filtering
- [ ] Permissions, approvals, and safety boundary
- [ ] Session persistence and recovery
- [ ] Context compaction and history governance
- [ ] CLAUDE.md and workspace instruction memory
- [ ] Session memory system
- [ ] Agent memory system
- [ ] Memory snapshot and memory directory mechanics
- [ ] Agent definition loading and runtime assembly
- [ ] AgentTool execution path
- [ ] Task system
- [ ] Agent lifecycle management
- [ ] Forked subagent path
- [ ] Agent isolation semantics
- [ ] Agent background/resume continuation protocol
- [ ] Coordinator mode
- [ ] Swarm, teammate, and team runtime
- [ ] Leader permission bridge and team permission sync
- [ ] Structured IO and control protocol
- [ ] Transport layer
- [ ] Bridge and remote runtime entry
- [ ] Hooks runtime
- [ ] MCP integration runtime
- [ ] LSP integration runtime
- [ ] Skills and plugin loading runtime

---

## Non-negotiable replication notes

- `fork` is a separate runtime path, not an optimization detail inside subagents.
- `plan mode` belongs to permission semantics, not only UX.
- `resume` means continuation of a persisted runtime identity, not replaying chat history.
- `compact` is a runtime survival system, not a summarization helper.
- `CLAUDE.md`, session memory, and agent memory are separate systems and must not be collapsed into one generic memory feature.
- `structured IO` is part of runtime architecture, not an optional integration extra.
- `coordinator/swarm` is part of Claude Code's agent organization model and should not be reduced to generic concurrency primitives.

---

## Suggested planning order

### Phase A: runtime core parity

- [ ] Startup and composition runtime
- [ ] QueryEngine session execution kernel
- [ ] Context assembly system
- [ ] Prompt assembly and model resolution system
- [ ] Tool protocol and execution contract
- [ ] Permissions, approvals, and safety boundary
- [ ] Session persistence and recovery
- [ ] Context compaction and history governance

### Phase B: long-running state and agent runtime parity

- [ ] CLAUDE.md and workspace instruction memory
- [ ] Session memory system
- [ ] Agent memory system
- [ ] Agent definition loading and runtime assembly
- [ ] AgentTool execution path
- [ ] Task system
- [ ] Agent lifecycle management
- [ ] Forked subagent path
- [ ] Agent isolation semantics
- [ ] Agent background/resume continuation protocol

### Phase C: multi-agent organization parity

- [ ] Coordinator mode
- [ ] Swarm, teammate, and team runtime
- [ ] Leader permission bridge and team permission sync

### Phase D: host/runtime platform parity

- [ ] Structured IO and control protocol
- [ ] Transport layer
- [ ] Bridge and remote runtime entry
- [ ] Hooks runtime
- [ ] MCP integration runtime
- [ ] LSP integration runtime
- [ ] Skills and plugin loading runtime

---

## Usage of this document

Every future implementation plan under `docs/plan/` should reference this file and explicitly state:

- which runtime modules it covers
- which required modules remain untouched
- whether any Claude Code module is intentionally deferred

If a future plan cannot map itself back to this module inventory, it is incomplete.

---

## Replication priority

Replication order must follow runtime dependency order, not feature visibility.

If the project implements higher-level agent features before the lower-level runtime semantics are aligned, those higher layers will be built on the wrong model and will need to be rewritten.

### P0: foundational runtime parity

These modules define the main runtime contract and should be completed first.

- [ ] Startup and composition runtime
- [ ] QueryEngine session execution kernel
- [ ] Context assembly system
- [ ] Prompt assembly and model resolution system
- [ ] Tool protocol and execution contract
- [ ] Tool surface assembly and availability filtering
- [ ] Permissions, approvals, and safety boundary
- [ ] Session persistence and recovery
- [ ] Context compaction and history governance

**Why P0 comes first:**
- every later agent capability executes through this chain
- Claude Code's runtime identity is defined here
- incorrect semantics here will poison subagents, tasks, fork, and remote runtime behavior

### P1: long-running agent runtime parity

These modules turn the runtime from a single-session loop into a persistent agent operating environment.

- [ ] CLAUDE.md and workspace instruction memory
- [ ] Session memory system
- [ ] Agent memory system
- [ ] Memory snapshot and memory directory mechanics
- [ ] Agent definition loading and runtime assembly
- [ ] AgentTool execution path
- [ ] Task system
- [ ] Agent lifecycle management
- [ ] Forked subagent path
- [ ] Agent isolation semantics
- [ ] Agent background/resume continuation protocol

**Why P1 follows P0:**
- agent/task behavior depends on session persistence, prompt layering, permission semantics, and compaction semantics being correct first
- Claude Code's long-running agent model is not separable from its session model

### P2: multi-agent organization parity

These modules define Claude Code's team-style multi-agent model.

- [ ] Coordinator mode
- [ ] Swarm, teammate, and team runtime
- [ ] Leader permission bridge and team permission sync

**Why P2 is later:**
- this is an organization layer built on top of stable agent lifecycle and permission propagation
- implementing it early would produce generic concurrency, not Claude Code's coordination model

### P3: host integration and extension parity

These modules expose the runtime to external hosts and extension surfaces.

- [ ] Structured IO and control protocol
- [ ] Transport layer
- [ ] Bridge and remote runtime entry
- [ ] Hooks runtime
- [ ] MCP integration runtime
- [ ] LSP integration runtime
- [ ] Skills and plugin loading runtime

**Why P3 is last:**
- these are critical for full parity, but they depend on the inner runtime model being stable enough to expose
- otherwise the project will freeze unstable internals into external protocol surfaces too early

---

## Current Go codebase assessment

This assessment is not a line-by-line audit. It is a runtime-architecture status reading used for prioritization.

### Overall judgment

The current Go codebase already has the skeleton of a Claude Code style runtime:

- a stateful QueryEngine
- a runtime runner
- session and agent managers
- permission evaluation
- basic compaction
- prompt building
- a gateway/protocol direction

However, it is still in an early-kernel stage.

It currently resembles:

- a workable single-process agent runtime kernel
- with basic subagent spawning and safety controls

It does not yet resemble:

- a full Claude Code runtime with long-lived session recovery
- layered memory systems
- fork semantics
- agent definition loading
- coordinator/swarm organization
- host-controlled runtime protocol parity

### Current strengths

#### QueryEngine direction is correct

`internal/queryengine/queryengine.go` already models the session loop as a stateful runtime object instead of a single request helper.

Good signs:

- owns session-scoped state
- integrates tools, permissions, compaction, memory, approvals
- tracks run state, token state, turn state, and compaction state

Assessment:

- **Status:** partial but structurally promising

#### Runner composition exists

`internal/runtime/runner.go` already composes the runtime from sessions, query engine, agent manager, approvals, memory, and compaction.

Good signs:

- there is already a runtime composition layer
- session-specific permission propagation exists
- subagent spawn/resume entrypoints already exist

Assessment:

- **Status:** partial composition layer, not yet full Claude Code-grade bootstrap/runtime composition

#### Permission model has a real foundation

`internal/permissions/policy.go` already includes:

- permission modes
- allow/deny rules
- workspace-root checks
- dangerous-command checks
- subagent mode derivation
- plan-mode gating

Assessment:

- **Status:** meaningful base, still much simpler than Claude Code's multi-source rule and dangerous-rule governance model

#### Compaction has an early semantic model

`internal/compaction/service.go` already has:

- token estimation
- thresholds
- message-limit and token-budget triggers
- summary creation
- preservation of recent turns

Assessment:

- **Status:** early compaction layer, not yet the multi-path compaction governance Claude Code uses

#### Prompt builder is already layered

`internal/prompt/builder.go` already separates:

- system prompt
- user context
- system context
- workspace context
- tools
- memory
- history

Assessment:

- **Status:** good foundation, but still much shallower than Claude Code's prompt hierarchy and runtime model-selection system

### Major gaps

#### Session persistence and recovery are not yet Claude Code-grade

`internal/session/manager.go` is currently an in-memory session manager.

What is still missing relative to Claude Code:

- transcript log persistence
- session metadata persistence
- restore-time cleanup and replay shaping
- context-collapse recovery state
- worktree and agent runtime state restoration

Assessment:

- **Status:** major gap

#### Memory system is still a single bucket service

`internal/memory/service.go` currently behaves like a simple session-scoped memory store.

What is still missing:

- separate CLAUDE.md loading pipeline
- async session memory maintenance
- scoped persistent agent memory
- memory snapshot sync
- memory directory manifest/inventory

Assessment:

- **Status:** major gap

#### Agent runtime is still closer to task spawning than AgentTool parity

`internal/agent/manager.go` provides spawn/wait/list/stop/steer.

What is still missing:

- agent definition loading
- explicit AgentTool semantics
- background lifecycle protocol
- continuation-grade resume
- output-file protocol
- worktree/remote/cwd isolation semantics
- fork child path

Assessment:

- **Status:** major gap

#### Tool runtime is still simpler than Claude Code's execution contract

The current tool system is usable, but still appears to be closer to a registry plus execution path than a full runtime protocol.

What is still missing:

- deeper tool progress protocol
- transcript-level tool result semantics
- dynamic availability filtering
- MCP/LSP-backed capability shaping
- runtime-facing tool lifecycle contract

Assessment:

- **Status:** partial, but still below parity target

#### Multi-agent organization layer is mostly absent

What is still missing:

- coordinator mode
- teammate identities
- team file model
- mailbox
- leader approval forwarding
- team permission sync

Assessment:

- **Status:** mostly missing

#### Host/runtime protocol layer is only partially present

The repo has `internal/protocol` and `internal/gateway`, which is the right direction.

What is still missing for parity:

- structured IO semantics equivalent to Claude Code
- durable control-plane event model
- transport parity
- remote-hosted runtime semantics

Assessment:

- **Status:** partial, but early

### Priority implication from current code

Given the current Go codebase state, the correct next move is not to jump into:

- coordinator
- builder-layer abstractions
- plugin ecosystems
- remote orchestration polish

The correct next move is to deepen P0 until the runtime core semantics are stable.

In practical terms, that means the near-term focus should stay on:

- QueryEngine
- prompt/context assembly
- permissions
- compaction
- persistence/recovery

Then move into:

- memory layering
- agent definition loading
- AgentTool semantics
- background/resume/isolation/fork

Only after that should the project commit to:

- coordinator/swarm
- structured host protocol parity
- extension runtime parity

---

## Working priority summary

If the project needs a compact rule for deciding what to build next, use this:

1. Stabilize the single-agent long-running runtime.
2. Add persistence, recovery, and context governance until sessions are continuation-safe.
3. Add layered memory and real agent runtime semantics.
4. Add fork, isolation, and background/resume.
5. Add team organization runtime.
6. Expose the stable runtime through host protocols and extension surfaces.

Anything that violates this order should require an explicit reason.
