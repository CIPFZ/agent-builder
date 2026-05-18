# Phase 2 Runtime API Boundary

This document is the active Phase 2 plan. Phase 1 proved that the desktop
client can talk to the real Crush runtime through Wails commands and a local
SSE event stream. Phase 2 turns that working bridge into a stable runtime API
boundary and a usable assistant client baseline that can support the desktop
client, future Web clients, headless clients, skills, and MCP.

## Goal

Make Crush runtime-first:

```text
Client/UI -> Runtime API + Event Stream -> Crush runtime
```

Wails remains a desktop adapter. It must not become the long-term product
protocol. Runtime state, sessions, turns, messages, tools, permissions, skills,
MCP, usage, and audit data must come from Go/Crush.

Phase 2 must also leave the project with a usable desktop assistant experience.
At the end of this phase, the app should be comparable to common market
assistant clients for core conversation use: configure a model, start or resume
conversations, stream responses, observe tools, approve risky actions, inspect
errors, and use available skills and MCP capabilities.

## Scope

Phase 2 includes four connected tracks:

1. Runtime API boundary.
2. Runtime event and audit schema.
3. Skill and MCP capability integration.
4. Assistant client baseline.

The goal is not to build a plugin marketplace or a full operations workflow in
this phase. The goal is to make the primitives visible, configurable, testable,
observable, and usable through one runtime boundary.

## Runtime Concepts

| Concept | Meaning |
| --- | --- |
| `Session` | Conversation state backed by Crush session storage. |
| `Turn` | One user input and the runtime work produced by it. |
| `Message` | User, assistant, tool, thinking, or system-visible content. |
| `MessagePart` | Structured content inside a message, including text, thinking, tool calls, tool results, and media. |
| `ToolCall` | A built-in tool or MCP tool invocation and result. |
| `PermissionRequest` | A runtime request for user approval before a risky action. |
| `Skill` | A discovered `SKILL.md` capability that can be injected into agent context. |
| `MCPServer` | A configured Model Context Protocol server. |
| `MCPCapability` | Tool, prompt, or resource exposed by an MCP server. |
| `Capability` | A normalized runtime view over built-in tools, skills, MCP tools, scripts, and future plugin-provided tools. |
| `Usage` | Token, model, timing, and cost-related runtime usage data. |
| `AuditEvent` | Durable evidence of important runtime actions and decisions. |

## Transport

Phase 2 should use:

```text
HTTP JSON API + SSE event stream
```

This is enough for the desktop app and future local Web clients:

- HTTP handles commands and queries.
- SSE handles runtime events.
- Wails can call the same Go service directly or route through the same local
  API adapter.

WebSocket or JSON-RPC can be added later if multi-client collaboration, remote
runtime control, or bidirectional streaming requires it.

## Minimal API

The exact package structure can evolve, but the API contract should start with
these operations:

```text
GET  /v1/runtime/status

GET  /v1/sessions
POST /v1/sessions
GET  /v1/sessions/{session_id}
GET  /v1/sessions/{session_id}/messages

POST /v1/sessions/{session_id}/turns
GET  /v1/turns/{turn_id}
POST /v1/turns/{turn_id}/cancel

GET  /v1/permissions
POST /v1/permissions/{permission_id}/decision

GET  /v1/capabilities
GET  /v1/skills
POST /v1/skills/refresh

GET  /v1/mcp/servers
PUT  /v1/mcp/servers/{server_name}
POST /v1/mcp/servers/{server_name}/refresh
GET  /v1/mcp/servers/{server_name}/tools
GET  /v1/mcp/servers/{server_name}/resources
GET  /v1/mcp/servers/{server_name}/prompts

GET  /v1/events
```

Write APIs must be designed so secrets are not leaked in normal responses,
events, or logs.

## Event Schema

Runtime events should be machine-readable and stable enough for UI, tests, and
future headless clients.

Recommended event names:

```text
runtime.started
runtime.failed

session.created
session.updated
session.deleted

turn.started
turn.progress
turn.completed
turn.failed
turn.cancelled

message.created
message.updated
message.completed

tool.call.started
tool.call.output
tool.call.completed
tool.call.failed

permission.requested
permission.decided

skill.discovery.started
skill.discovery.completed
skill.discovery.failed
skill.enabled
skill.disabled

mcp.server.starting
mcp.server.connected
mcp.server.failed
mcp.server.disabled
mcp.tools.updated
mcp.resources.updated
mcp.prompts.updated

usage.updated
audit.recorded
```

Every event should include:

| Field | Requirement |
| --- | --- |
| `id` | Unique event id. |
| `type` | Stable event name. |
| `created_at` | RFC3339 timestamp. |
| `session_id` | Present when related to a session. |
| `turn_id` | Present when related to a turn. |
| `message_id` | Present when related to a message. |
| `tool_call_id` | Present when related to a tool call. |
| `payload` | Event-specific structured data. |

Events must not contain raw API keys, authorization headers, or full secret
values.

## Skill Integration

Crush already has a skill subsystem:

- Skill discovery from built-in and configured paths.
- `SKILL.md` parsing and validation.
- Disabled skill filtering.
- Prompt XML generation.
- Discovery diagnostics and pub/sub events.

Phase 2 should expose this through the runtime boundary.

### Required Behavior

- The client can list discovered skills.
- The client can see builtin, local, enabled, disabled, and error states.
- The client can refresh skill discovery.
- The runtime emits skill discovery events.
- Skill metadata is available to audit and diagnostics views.
- Disabled skills are persisted in Crush config, not in frontend state.
- Skill instructions stay runtime-owned. The frontend may display metadata, but
  it should not inject skill content into model context.

### Minimal Skill API Shape

```json
{
  "name": "crush-config",
  "description": "Use when the user needs help configuring Crush.",
  "builtin": true,
  "enabled": true,
  "path": "crush://skills/crush-config",
  "skill_file_path": "crush://skills/crush-config/SKILL.md",
  "state": "normal",
  "error": ""
}
```

### Acceptance Criteria

- Built-in skills are visible through `GET /v1/skills`.
- Local skills from configured paths are visible through the same API.
- Invalid skills appear as diagnostics instead of silently disappearing.
- Disabling a skill updates config and the next agent context excludes it.
- Skill refresh publishes `skill.discovery.*` events.

## MCP Integration

Crush already supports MCP server configuration and runtime clients:

- `stdio`, `http`, and `sse` MCP transports.
- Server state tracking.
- Tool, prompt, and resource discovery.
- Enabled and disabled MCP tools.
- MCP tool execution through the tool system.

Phase 2 should make MCP first-class in the runtime boundary.

### Required Behavior

- The client can list configured MCP servers.
- The client can see server state: disabled, starting, connected, or error.
- The client can add, update, disable, and refresh an MCP server.
- The client can list MCP tools, prompts, and resources.
- MCP state changes publish runtime events.
- MCP tools appear in normalized capabilities.
- MCP tool calls flow through existing permission, tool, message, usage, and
  audit events.
- MCP configuration with secrets must be redacted in API responses and logs.

### Minimal MCP Server API Shape

```json
{
  "name": "docs",
  "type": "http",
  "url": "https://example.com/mcp",
  "disabled": false,
  "state": "connected",
  "counts": {
    "tools": 3,
    "prompts": 1,
    "resources": 2
  },
  "error": ""
}
```

### Minimal MCP Tool API Shape

```json
{
  "server": "docs",
  "name": "search_docs",
  "description": "Search product documentation.",
  "enabled": true,
  "input_schema": {}
}
```

### Acceptance Criteria

- Configured MCP servers are visible through `GET /v1/mcp/servers`.
- MCP server startup and failures are visible in events.
- MCP tools are visible through both MCP-specific APIs and
  `GET /v1/capabilities`.
- A model-triggered MCP tool call is visible as `tool.call.*` events.
- MCP configuration responses redact secrets in `env`, `headers`, and auth-like
  fields.

## Capability View

Phase 2 should introduce a normalized read-only capability view. This is not a
plugin package yet. It is an inventory that lets the client and runtime reason
about what the agent can use.

Example:

```json
{
  "id": "mcp:docs:search_docs",
  "kind": "mcp_tool",
  "name": "search_docs",
  "source": "docs",
  "enabled": true,
  "risk": "read",
  "description": "Search product documentation."
}
```

Initial capability kinds:

- `builtin_tool`
- `mcp_tool`
- `mcp_prompt`
- `mcp_resource`
- `skill`

Future phases can add:

- `script`
- `native_command`
- `plugin_tool`
- `agent`

## Assistant Client Baseline

Phase 2 must produce a client that can be used as a real auxiliary assistant,
not only as an API demo. The UI remains thin, but it must expose enough runtime
capability for practical daily use.

### Required User Flows

- Configure a model provider from the desktop app.
- Verify model connectivity without using mock providers.
- Start a new conversation.
- Resume a previous conversation.
- Send a user message and see streamed assistant output.
- See thinking/tool activity as structured runtime parts.
- See MCP tool activity in the same timeline as built-in tools.
- See which skills and MCP servers are available to the runtime.
- Approve, allow for session, deny, or cancel permission requests.
- Cancel an active turn.
- See clear errors when model, config, MCP, skill, or runtime setup fails.
- Open an audit/debug view for the current turn with timing, model, usage,
  tool calls, permission decisions, skill availability, and MCP availability.

### Client UX Requirements

- The first screen must remain conversation-first.
- Model configuration is easy to find, but not the center of the daily chat
  experience after setup.
- Advanced settings, proxy, MCP details, and diagnostics stay behind explicit
  panels or drawers.
- Empty states should tell the user what is missing: model config, unavailable
  MCP server, invalid skill, or runtime error.
- Tool, skill, MCP, permission, and audit details are visible when needed but do
  not crowd the default chat view.
- The frontend must not synthesize runtime messages, tool calls, usage, or
  permission decisions.

### Desktop Artifact Requirements

The Phase 2 desktop build should keep the Phase 1 artifact shape:

```text
AgentBuilder.exe
config/
data/
logs/
```

The app should be testable on the local machine as a packaged desktop client.
Configuration written from the UI must persist under the runtime-owned config
directory, and logs/audit data must stay outside source-controlled paths.

## Audit Requirements

Phase 2 audit data should answer these questions:

- Which model was used?
- Which user input started the turn?
- Which skills were available to the agent?
- Which MCP servers and tools were available?
- Which tools were called?
- Which permission decisions were made?
- How long did the turn take?
- What usage/tokens were reported by the provider?
- Did the turn complete, fail, or get cancelled?

Audit records should be structured and redact secrets by default.

## Non-goals

Phase 2 should not include:

- Full operation/run workflow.
- SSH troubleshooting MVP.
- Plugin marketplace.
- Plugin package signing.
- Agent teams.
- Full policy engine.
- Full sandbox or worktree isolation.
- Multi-tenant enterprise RBAC.
- Remote multi-user runtime.

These are later phases. Phase 2 only prepares the runtime boundary they need.

## Implementation Order

1. Document and freeze the Phase 2 API and event schema.
2. Extract current Wails runtime operations behind a transport-neutral Go
   service interface.
3. Add a local HTTP API adapter.
4. Promote the SSE stream from desktop bridge detail to runtime API primitive.
5. Add skill list, refresh, diagnostics, enable, and disable operations.
6. Add MCP server list, refresh, status, and capability APIs.
7. Normalize built-in tools, skills, and MCP tools into capabilities.
8. Update the desktop client to use the stable runtime shapes for chat, model
   config, sessions, permissions, skill/MCP status, and audit/debug views.
9. Add API smoke tests for session, turn, events, skill discovery, and MCP
   discovery.
10. Add packaged desktop smoke testing for model configuration, one real
    conversation, event streaming, and cancellation.

## Phase 2 Acceptance

Phase 2 is complete when:

- A client can create and read a session through the runtime API.
- A client can send one user turn and cancel it.
- A client receives assistant, tool, permission, usage, skill, and MCP events.
- Skills are discoverable and refreshable through the runtime API.
- MCP servers and MCP tools are visible through the runtime API.
- MCP tool calls appear in the same tool event stream as built-in tools.
- Runtime API responses and logs redact secrets.
- The desktop client works as a thin UI over the runtime API.
- The desktop client can be used as a basic assistant client: configure model,
  chat, stream output, resume history, inspect tools, approve permissions,
  inspect skill/MCP status, view useful errors, cancel an active turn, and see
  turn audit details.
- A packaged desktop build can be manually tested on the local machine.
- The existing TUI is not broken.
