# MCP Runtime Capability Design

Date: 2026-04-23

## 1. Objective

Convert the current partial MCP implementation into a stable module that supports:

- configured MCP servers
- runtime discovery
- dynamic MCP tool/resource/prompt/skill registration
- auth-required and OAuth flows
- reconnect and invalidation refresh
- `myclawd` control-plane visibility

This module is aligned to three inputs together:

1. user requirement:
   - MCP is one of the core required abilities
   - later Docker / DB / external-system control should be able to use MCP instead of only shell
2. current architecture:
   - runtime first
   - `myclawd` as shared control plane
   - React UI later
3. Claude Code semantic reference:
   - centralized MCP discovery and runtime injection
   - auth-required pseudo-tools
   - dynamic inventory refresh
   - resources/prompts/skills exposed through generic runtime surfaces

## 2. Current Assessment

`myclaw` already has more MCP runtime than the rest of the docs imply.

Existing strengths:

- `internal/tools/mcp_client.go` already discovers tools, prompts, resources, skills, and reconnect behavior
- `internal/tools/mcp_oauth.go` already provides OAuth store/provider/auth flow primitives
- `internal/queryengine/queryengine.go` already knows how to register discovered MCP tools and project prompts/skills into tool context
- `internal/tools/extended_tools.go` already exposes `ListMcpResources`, `ReadMcpResource`, MCP auth tools, and MCP prompt skill projection
- `internal/runtime/runner.go` already tracks MCP inventory snapshots

Current module-level gaps:

1. app bootstrap does not yet expose a real MCP connection configuration surface
2. MCP runtime ownership is not documented as one coherent implementation target
3. `myclawd` does not yet expose explicit MCP management semantics
4. there is no module-level implementation/review contract for downstream Claude Code execution

The design therefore should not "rebuild MCP from scratch". It should complete and normalize the existing partial implementation.

## 3. Design Decision

The MCP module should be implemented as **one runtime/control-plane module**, not split into separate "MCP tools", "MCP auth", "MCP resources", and "MCP prompts" projects.

This is the correct cut because:

- all four behaviors share the same connection lifecycle
- all four behaviors share the same discovery and invalidation semantics
- all four behaviors must remain synchronized in the runtime inventory
- Claude Code models them as one connected MCP surface rather than isolated features

## 4. Target Module Shape

The completed module should have four internal layers.

### Layer A: Config And Bootstrap

Purpose:

- make MCP servers declarative and reachable from normal startup

Responsibilities:

- config schema for MCP server definitions
- file/env resolution
- validation and normalization
- bootstrap wiring into runtime options

This is the biggest immediate product gap because MCP cannot be treated as first-class if runtime startup cannot load real server definitions.

### Layer B: Discovery And Dynamic Inventory

Purpose:

- centralize the initial MCP connection and inventory discovery path

Responsibilities:

- discover tools/prompts/resources/skills
- register dynamic tool definitions
- preserve runtime snapshots
- refresh inventory on reconnect and invalidation

This layer must continue to own tool/prompt/resource coupling so inventory changes do not drift across separate code paths.

### Layer C: Auth And Reconnect Lifecycle

Purpose:

- make HTTP auth-required servers actionable and recoverable

Responsibilities:

- OAuth token store
- auth-required pseudo-tool generation
- challenge/scope propagation
- reconnect after auth completion
- reconnect after list invalidation

This layer is important because auth-required servers should degrade into an explicit "authenticate" action, not vanish silently or require frontend-owned logic.

### Layer D: Control Plane Exposure

Purpose:

- expose MCP state through `myclawd` so clients consume a shared protocol

Responsibilities:

- MCP inventory/state read APIs
- reconnect/auth control actions
- consistent payload shape for server status and counts
- no frontend-only MCP management semantics

This is required by the runtime-first architecture. If MCP state exists only in runtime internals, the future React operator UI cannot consume it cleanly.

## 5. Key Behaviors

### 5.1 Server Configuration

The design should add explicit MCP server configuration ownership under `config.MCP`, not leave MCP clients only as hand-constructed runtime options in tests or internal bootstraps.

Each configured server should define:

- stable name
- transport type
- endpoint or command details
- env/headers/helper configuration
- auth metadata when applicable

The module does not need a rich settings UI. It does need a real backend configuration contract.

### 5.2 Discovery

Startup discovery should remain centralized and should produce:

- server connection snapshots
- tool inventory
- prompt inventory
- resource inventory
- derived MCP skills
- auth-required pseudo-tools where necessary

This should continue to flow into runtime/queryengine ownership rather than spawning disconnected registries.

### 5.3 Auth-Required Semantics

When a server needs auth:

- the server should remain visible in MCP inventory
- an authenticate action should be explicit
- challenge and scope metadata should survive
- post-auth reconnect should replace pseudo-tools with real tools

This is the same semantic direction Claude Code uses with `McpAuthTool`.

### 5.4 Reconnect And Invalidation

Reconnect should not just reopen the transport. It must refresh dynamic inventory:

- changed tools
- changed prompts
- changed resources
- changed derived MCP skills

That refreshed inventory must flow into runtime-owned data structures and any exposed control-plane state.

### 5.5 Prompt / Resource / Skill Exposure

MCP prompts and resources should remain exposed through generic runtime surfaces:

- `ListMcpResources`
- `ReadMcpResource`
- prompt projection through MCP prompt skills
- derived MCP skill listing

Do not invent a separate prompt/resource interaction protocol for the client.

### 5.6 Control Plane

`myclawd` should expose enough MCP state to make a future UI possible without runtime refactoring.

That means at minimum:

- list configured/discovered MCP servers
- current auth/connection status
- counts for tools/prompts/resources
- explicit reconnect and auth operations

This should be additive to existing runtime behavior, not a second MCP subsystem.

## 6. Non-Goals

This module should not:

- implement React MCP settings
- fully replicate Claude Code MCP UI flows
- redesign plugin/skill architecture beyond MCP touchpoints
- introduce remote bridge or cloud-session semantics
- absorb Docker or DB domain logic into MCP work

## 7. Acceptance Criteria

This design is correct if:

- MCP becomes reachable from normal `myclaw` runtime startup
- the dynamic inventory lifecycle is explicit and shared
- auth-required and reconnect semantics are explicit
- `myclawd` gains real MCP management semantics
- the module can be implemented by Claude Code without inventing architecture
