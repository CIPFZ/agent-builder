# MCP Runtime Capability Test And Validation Plan

Date: 2026-04-23

## 1. Purpose

Define the minimum test and validation bar for the MCP runtime capability module.

## 2. Test Areas

## A. Config And Bootstrap

Cover:

- config decoding of MCP server definitions
- env/file override behavior
- validation of incomplete or invalid MCP server entries
- bootstrap wiring from config into runtime options

Suggested targets:

- `internal/config/config_test.go`
- bootstrap tests near `internal/app/bootstrap.go`

## B. Discovery And Dynamic Registration

Cover:

- stdio MCP discovery
- HTTP MCP discovery
- tool/prompt/resource/skill extraction
- registration of discovered MCP tools
- prompt/resource inventory snapshotting

Suggested targets:

- `internal/tools/mcp_dynamic_test.go`
- `internal/queryengine/tool_lifecycle_test.go`
- `internal/runtime/runner_test.go`

## C. Auth And OAuth

Cover:

- auth-required discovery path
- generated authenticate pseudo-tool behavior
- OAuth store persistence behavior
- challenge/scope/resource-metadata propagation
- post-auth reconnect path

Suggested targets:

- `internal/tools/mcp_oauth_test.go`
- `internal/tools/extended_tools_test.go`
- `internal/queryengine/tool_lifecycle_test.go`

## D. Reconnect And Invalidation

Cover:

- reconnect after tool list change
- reconnect after prompt/resource list change
- stale inventory replacement
- runtime state updates after reconnect

Suggested targets:

- `internal/tools/mcp_dynamic_test.go`
- `internal/queryengine/tool_lifecycle_test.go`
- `internal/runtime/runner_test.go`

## E. Control Plane

Cover:

- websocket status/inventory request
- reconnect request
- auth-related MCP control request if introduced
- payload field stability

Suggested targets:

- `internal/gateway/server_test.go`

## 3. Functional Validation Scenarios

At minimum, validate these flows:

1. configured stdio server starts and exposes tools/resources
2. HTTP server requiring auth surfaces authenticate behavior instead of disappearing
3. reconnect refreshes changed tool/prompt/resource inventory
4. prompt/resource access works through shared runtime surfaces
5. MCP-derived skills remain visible only where intended

## 4. Suggested Test Commands

At minimum, run:

```bash
go test ./internal/tools ./internal/queryengine ./internal/runtime ./internal/gateway ./internal/config ./internal/app
```

If the test surface becomes too slow, also run the focused subsets used during implementation:

```bash
go test ./internal/tools -run MCP
go test ./internal/queryengine -run MCP
go test ./internal/runtime -run MCP
go test ./internal/gateway -run MCP
go test ./internal/config ./internal/app
```

## 5. Exit Criteria

The module should not be considered ready for review unless:

- config/bootstrap tests pass
- MCP discovery tests pass
- OAuth/auth-required tests pass
- reconnect tests pass
- control-plane tests pass
- at least one representative stdio flow and one auth-required HTTP flow were functionally validated
