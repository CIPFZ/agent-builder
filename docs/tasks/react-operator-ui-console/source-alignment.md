# React Operator UI Console Source Alignment

Date: 2026-04-25

## 1. Alignment Purpose

This module aligns the React UI with Claude Code semantics and the current Go implementation.

The objective is semantic alignment, not component-by-component UI parity.

## 2. Claude Code Reference Areas

Use these Claude Code source areas as semantic references:

- `claude-code/src/QueryEngine.ts`
- `claude-code/src/Tool.ts`
- tool lifecycle and tool result handling
- permission/approval flow
- task/subagent lifecycle
- MCP and skills exposure patterns
- session and transcript behavior

## 3. myclaw Reference Areas

Use these Go source areas as current implementation references:

- `internal/protocol/ws/message.go`
- `internal/gateway/server.go`
- `internal/runtime/runner.go`
- `internal/queryengine/queryengine.go`
- `internal/tools/filesystem_tools.go`
- `internal/tools/system`
- `internal/tools/mcp_client.go`
- `internal/tools/mcp_dynamic.go`
- `internal/tools/skill_discovery.go`
- `internal/tools/skill_parity.go`
- `internal/tools/agent_tool.go`
- `internal/agent/manager.go`
- `internal/approval`
- `internal/permissions`
- `internal/session`

## 4. Semantic Rules

### A. Runtime Owns Meaning

The UI must not decide:

- whether a tool is safe
- whether approval is needed
- whether a subagent state transition is valid
- how MCP tools are discovered
- how skills are selected or injected

### B. Control Plane Owns Transport

The UI must consume `myclawd` websocket requests and events.

If a UI feature needs data that is not exposed, add or document a `myclawd` API gap.

### C. UI Owns Visibility

The React UI should make runtime state visible through:

- message bubbles
- tool cards
- approval panels
- task dashboards
- inventory pages
- detail drawers

Visibility does not mean duplicating runtime logic.

## 5. Ant Design X Usage Rules

Use Ant Design X where it directly matches AI interaction:

- `Bubble` / `Bubble.List` for transcript
- `Sender` for composer
- `Conversations` for sessions
- `Prompts` for suggested flows
- attachments and markdown rendering where needed

Use Ant Design for operational console surfaces:

- tables
- drawers
- forms
- tabs
- modals
- timelines
- descriptions

## 6. Review Standard

Reject implementation if:

- React calls runtime internals directly
- frontend fabricates backend state transitions
- UI hides protocol gaps by hardcoding fake inventory
- skills, MCP, subagent, approval, or tool semantics diverge from Go runtime behavior
