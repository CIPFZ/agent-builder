# MCP Runtime Capability Implementation Plan

Date: 2026-04-23

## 1. Planning Goal

Provide a concrete implementation sequence for turning the current partial MCP core into a complete `myclaw` runtime module.

## 2. Current Baseline

Already present:

- dynamic MCP discovery
- MCP OAuth primitives
- MCP auth pseudo-tool support
- MCP resource and prompt access tools
- MCP prompt/skill projection
- runtime inventory snapshots

Still missing as a module:

- config/bootstrap reachability
- explicit control-plane management surface
- module-level implementation contract

## 3. Phase Breakdown

## Phase 1: Config And Bootstrap Reachability

### Objective

Make MCP server definitions loadable in ordinary `myclaw` startup.

### Target Files

- `internal/config/config.go`
- `internal/config/config_test.go`
- `internal/app/bootstrap.go`
- `internal/app/*_test.go` as needed

### Required Work

- extend `config.MCP` to include server definitions, not just enabled/skills flags
- support both stdio and HTTP-style MCP configuration
- validate and normalize configured server entries
- pass configured MCP connections into runtime bootstrap

### Acceptance

- `myclaw` can start with real MCP server definitions from config
- runtime gets `MCPClients` from normal bootstrap, not only tests/manual construction

## Phase 2: Runtime Discovery Normalization

### Objective

Ensure startup discovery and runtime snapshots are complete and explicit.

### Target Files

- `internal/runtime/runner.go`
- `internal/queryengine/queryengine.go`
- `internal/tools/mcp_client.go`
- `internal/tools/mcp_dynamic.go`

### Required Work

- verify one canonical discovery path is used
- ensure discovered tools/prompts/resources/skills all land in runtime-owned state
- ensure runtime inventory and snapshots reflect configured and connected servers consistently
- ensure prompt/resource/skill projection continues to use generic runtime surfaces

### Acceptance

- discovery is stable and centralized
- inventory shape is explicit and reviewable

## Phase 3: Auth And Reconnect Lifecycle Hardening

### Objective

Close the auth-required and reconnect lifecycle as a first-class runtime capability.

### Target Files

- `internal/tools/mcp_oauth.go`
- `internal/tools/mcp_client.go`
- `internal/tools/extended_tools.go`
- `internal/queryengine/queryengine.go`

### Required Work

- preserve explicit auth-required pseudo-tool behavior
- preserve challenge/scope/resource-metadata propagation
- ensure reconnect refreshes changed tools/prompts/resources/skills
- ensure runtime state updates replace stale pseudo-tools / stale inventory

### Acceptance

- auth-required servers remain visible and actionable
- reconnect is inventory-refreshing, not only transport-refreshing

## Phase 4: myclawd Control-Plane Exposure

### Objective

Expose MCP state and management actions in the shared control plane.

### Target Files

- `internal/gateway/server.go`
- `internal/protocol/ws/message.go`
- `internal/runtime/runner.go`
- `internal/gateway/server_test.go`

### Required Work

- add MCP-focused websocket methods and payloads for:
  - inventory/status read
  - reconnect
  - auth start / auth status where appropriate
- reuse runner/runtime ownership rather than duplicating MCP logic in gateway
- define stable payload fields for clients

### Acceptance

- future React UI can manage MCP from `myclawd` without backend redesign
- no frontend-owned MCP lifecycle logic is required

## Phase 5: Tests And Validation

### Objective

Close the module with focused tests and representative fixture validation.

### Target Files

- `internal/config/config_test.go`
- `internal/tools/mcp_dynamic_test.go`
- `internal/tools/mcp_oauth_test.go`
- `internal/queryengine/tool_lifecycle_test.go`
- `internal/runtime/runner_test.go`
- `internal/gateway/server_test.go`

### Acceptance

- relevant focused suites pass
- representative stdio/http MCP flows are validated

## 4. Risks

### Risk A: Over-scoping Into UI Work

Mitigation:

- keep this module runtime and control-plane only

### Risk B: Splitting Discovery Across Multiple Paths

Mitigation:

- preserve one canonical discovery/refresh path

### Risk C: Treating MCP As "Already Done"

Mitigation:

- explicitly fix config/bootstrap and control-plane gaps

## 5. Non-Goals

- full UI parity with Claude Code MCP screens
- plugin-marketplace work
- generalized skill re-architecture beyond MCP touchpoints
- Docker/DB tool implementation

## 6. Definition Of Done

This module is complete when:

- MCP servers are configurable and bootstrapped in normal runtime startup
- dynamic inventory is stable and refreshable
- auth-required and reconnect behavior are explicit and test-covered
- `myclawd` exposes MCP state and actions through shared protocol methods
- the implementation passes the review checklist without a blocking gap
