# MCP Runtime Capability Source Alignment

Date: 2026-04-23

## 1. Purpose

This document maps the intended MCP module behavior to the relevant Claude Code semantic owners and the current `myclaw` Go-side ownership points.

The goal is semantic alignment, not filename-copying.

## 2. Primary Claude Code References

### Core Runtime Ownership

- `claude-code/src/Tool.ts`
- `claude-code/src/QueryEngine.ts`

Why they matter:

- they define how tools are injected into the query lifecycle
- they define the runtime context available to tools
- they define the shared contract for tool execution and result handling

### MCP Connection And Discovery Ownership

- `claude-code/src/services/mcp/client.ts`
- `claude-code/src/services/mcp/auth.ts`
- `claude-code/src/services/mcp/MCPConnectionManager.tsx`

Why they matter:

- they define discovery, connection, auth, reconnect, and inventory refresh semantics
- they show that MCP is handled centrally rather than as ad hoc helper tools

### MCP Tool Surface Ownership

- `claude-code/src/tools/MCPTool/MCPTool.ts`
- `claude-code/src/tools/McpAuthTool/McpAuthTool.ts`
- `claude-code/src/tools/ListMcpResourcesTool/ListMcpResourcesTool.ts`
- `claude-code/src/tools/ReadMcpResourceTool/ReadMcpResourceTool.ts`
- `claude-code/src/tools/SkillTool/SkillTool.ts`

Why they matter:

- they define the user/model-facing contract for MCP tools
- they show auth-required servers surfacing authenticate actions
- they show prompts/resources staying on generic runtime paths
- they show MCP prompt/skill projection behavior

## 3. Claude Semantics We Intend To Carry Over

The `myclaw` MCP module should preserve these semantics:

1. MCP discovery is centralized
2. MCP tools are dynamic but first-class
3. auth-required servers are represented explicitly rather than hidden
4. reconnect refreshes dynamic inventory
5. resources and prompts reuse shared runtime surfaces
6. MCP prompt/skill exposure is runtime-managed rather than UI-managed

## 4. Current Go Ownership Points

### Tool / Transport / Discovery Layer

- `internal/tools/mcp_client.go`
- `internal/tools/mcp_dynamic.go`
- `internal/tools/mcp_oauth.go`
- `internal/tools/extended_tools.go`
- `internal/tools/registry.go`

Current alignment:

- good alignment on discovery, OAuth primitives, dynamic tool projection, prompt/resource access

Current gaps:

- config/bootstrap reachability is not yet part of a completed module

### Query / Runtime Layer

- `internal/queryengine/queryengine.go`
- `internal/runtime/runner.go`

Current alignment:

- good alignment on injecting discovered MCP inventory into runtime context
- reconnect and inventory snapshots already have ownership points

Current gaps:

- module contract is not documented and not yet completed through normal startup/config paths

### App / Bootstrap Layer

- `internal/app/bootstrap.go`
- `internal/config/config.go`

Current alignment:

- minimal only

Current gaps:

- no first-class MCP connection configuration model
- bootstrap currently only toggles MCP skills behavior, not actual MCP server definitions

### Control Plane Layer

- `internal/gateway/server.go`
- `internal/protocol/ws/message.go`

Current alignment:

- almost none for MCP-specific management

Current gaps:

- no explicit MCP inventory management endpoints
- no explicit reconnect/auth control-plane methods

## 5. Explicit Non-Alignment Areas

The Go implementation should not attempt 1:1 parity on:

- React MCP settings screens
- console/browser OAuth UI visuals
- remote cloud bridge behaviors unrelated to local/runtime MCP capability

Those are product/UI layers, not the backend parity target for this module.

## 6. Alignment Decision

The correct replication target is:

- Claude Code MCP runtime semantics
- not Claude Code MCP UI fidelity

If there is any tradeoff, preserve in this order:

1. centralized MCP discovery and inventory lifecycle
2. first-class auth-required / reconnect semantics
3. shared runtime exposure of prompts/resources/skills
4. shared control-plane reachability
5. client/UI fidelity last
