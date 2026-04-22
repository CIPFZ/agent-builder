# Current Capability Gap Review

Date: 2026-04-22

## Review Frame

This review is intentionally scoped to the new project target:

- a Claude Code-aligned Go runtime
- a `myclawd` control plane
- a React operator UI
- capability priority driven by real project control needs

This is not a "full Claude Code product UI parity" scorecard.

## Bottom Line

Under the new target, the project is materially ahead of the old all-module parity framing.

Current practical status:

- runtime core is already meaningful and usable
- local code modification workflows are close to usable
- MCP and skills are among the strongest implemented areas
- the largest missing area is the external execution surface and control plane maturation

## Current Capability Estimate

| Capability Area | Current Estimate | Notes |
|---|---:|---|
| Query and runtime loop | 70% | usable core path exists |
| File read/write/edit/search | 80% | one of the strongest completed surfaces |
| Tool invocation lifecycle | 65% | strong main path, still simpler than Claude |
| MCP runtime | 75% | dynamic discovery and invocation exist |
| Skills runtime | 70% | discovery and injection are strong |
| Subagent runtime | 60% | usable but still shallower than Claude |
| Permission and approval flow | 60% | working core, still simplified |
| Session and recovery | 60% | good local foundation |
| Shell execution | 55% | present, but not yet production-grade control surface |
| ssh execution | 15% | effectively missing as a first-class capability |
| Docker control | 40% | possible via shell or MCP, not productized |
| Database operations | 35% | possible via shell or MCP, not productized |
| myclawd control plane | 45% | websocket gateway exists, but not full product protocol |
| Go TUI | 30% | adequate as a light shell, not a parity target |
| React operator UI | 0% | not started |

## What Is Already Strong Enough To Build On

### Query and Tool Loop

The central runtime path is already real, not speculative.

Primary evidence:

- `internal/queryengine/queryengine.go`
- `internal/runtime/runner.go`
- `internal/tools/registry.go`

This means the project should now optimize for execution surface and control-plane expansion instead of reopening the core loop first.

### File Modification

Primary evidence:

- `internal/tools/filesystem_tools.go`

The project already has solid file control primitives:

- read
- write
- edit
- multiedit
- glob
- grep
- ls

This makes "control a Go web project source tree" a realistic near-term target.

### MCP And Skills

Primary evidence:

- `internal/tools/mcp_dynamic.go`
- `internal/tools/mcp_client.go`
- `internal/tools/mcp_oauth.go`
- `internal/tools/skill_discovery.go`
- `internal/tools/skill_frontmatter.go`
- `internal/tools/bundled_skills.go`

This is one of the clearest strengths in the current codebase and should be treated as foundation, not backlog.

### Basic Subagent Delegation

Primary evidence:

- `internal/tools/agent_tool.go`
- `internal/agent/manager.go`
- `internal/runtime/runner.go`

Spawn/wait/resume/stop exist. The gap is not "subagent missing". The gap is deeper lifecycle and control semantics.

## Most Important Gaps Against The New Goal

### 1. ssh Is Missing As A First-Class Runtime Capability

The new target requires controlling machines and services beyond the local repo. That cannot stay as an afterthought.

Without first-class ssh support:

- remote deployment flows remain awkward
- external Docker host control remains awkward
- remote DB operations remain awkward
- the runtime cannot fully control a real project environment

### 2. Docker And Database Control Exist Only As Side Paths

Today these are mostly reachable by:

- shell commands
- MCP servers

That is not enough for stable operator workflows.

The project needs either:

- first-class Docker and DB tools
- or first-class runtime wrappers on top of shell/MCP with stable contracts

### 3. myclawd Is Not Yet The Unified Control Plane

The websocket gateway is useful, but the project still lacks a fully normalized control-plane contract for:

- tool progress
- approvals
- task/subagent lifecycle
- runtime inventory
- terminal and web client parity

### 4. TUI Should Stop Being Treated As A Primary Parity Surface

The Go TUI is still useful, but it should be treated as:

- a lightweight operator shell
- a debug/control entry point

It should not remain the main product parity target.

### 5. Subagent Lifecycle Needs To Be Deepened

The project has delegation, but still needs:

- better task modeling
- stronger background behavior
- output/result contracts
- future isolation and remote execution semantics

## Immediate Conclusion

The correct next move is not another broad parity sweep.

The correct next move is:

1. keep runtime core stable
2. expand execution surface
3. normalize `myclawd` as the control plane
4. start a React operator UI only after the protocol is stable enough

## Current Strategic Priority

Highest priority:

- ssh
- shell hardening
- docker control
- database control
- subagent/task lifecycle strengthening
- `myclawd` protocol contracts

Medium priority:

- worktree and isolation strengthening
- runtime inventory and operator observability
- React operator UI bootstrap

Low priority for now:

- Go TUI feature growth
- Go TUI parity with Claude Code visual/product behavior

