# Operator Session Control Plane Design

Date: 2026-04-25

## Purpose

The next React Operator UI phase should turn the current single-chat surface into a real operator console backed by `myclawd` session state.

The work should proceed in this order:

1. Session lifecycle and left sidebar behavior.
2. Approvals and tool lifecycle visibility.
3. MCP and skills depth.

The UI must continue to communicate only through `myclawd`. It must not call Go runtime internals or invent fake backend state.

## Recommended Approach

Implement backend websocket contracts and frontend UI together for each capability area.

Frontend-only session handling is explicitly rejected because it would make `New chat`, `Recents`, transcript restore, and capability ownership local illusions. The runtime already owns sessions through `session.Manager`, so the websocket control plane should expose that state directly.

## Phase 1: Session Lifecycle

### Goals

- `New chat` creates a real runtime session.
- `Recents` displays real sessions.
- Selecting a session restores its transcript and status.
- Reconnect preserves or restores the selected session when possible.
- The top chat title follows the selected session display name.

### myclawd Methods

Add these websocket methods:

- `session_list`
- `session_new`
- `session_messages`

`session_list` returns sessions with:

- `session_id`
- `session_key`
- `agent_id`
- `is_main`
- `message_count`
- `last_activity_at`
- `last_user_message`
- `title`

`session_new` creates a new non-main operator session for an agent and returns:

- `session_id`
- `session_key`
- `agent_id`
- `is_main`
- `status`

`session_messages` accepts `session_id` or `session_key` and returns transcript messages for that session.

### Backend Notes

`session.Manager` already exposes `ListSessions()` and `Messages()`. It can create sessions today through `CreateChild(agentID, key)`, but a helper such as `CreateSession(agentID)` should be added if needed to avoid UI-shaped key construction in the gateway.

The existing HTTP status handler exposes session summaries, but the React Operator UI should not use that endpoint as its control plane. The websocket protocol remains the single UI contract.

### Frontend Behavior

- Left sidebar uses Ant Design X `Conversations`.
- `New chat` calls `session_new`, then binds the client to the returned `session_key`.
- Session selection calls `session_messages`, then refreshes `session_status`, `approval_list`, `subagent_list`, `orchestration_status`, and `mcp_status`.
- Transcript, tools, approvals, subagents, and orchestration are scoped to the active session in state.
- If a method is unavailable, the UI records a protocol gap instead of faking a local session.

## Phase 2: Approvals And Tool Lifecycle

### Goals

- Pending approvals are visible from the composer.
- Approvals can be handled without leaving the conversation.
- Tool lifecycle is visible inline in the transcript.
- Tool details remain available in a drawer.

### myclawd Methods And Events

Use existing methods:

- `approval_list`
- `approval_approve`
- `approval_reject`
- `approval_clear`

Use existing events:

- `permission.required`
- `approval.updated`
- `approval.cleared`
- `tool.called`
- `tool.progress`
- `tool.result`
- `run.error`

### Frontend Behavior

- Composer shows a pending approval badge when approvals exist.
- A compact approval strip appears above the sender when approval is blocking active work.
- Approval drawer shows tool name, tool input, reason, content blocks, accept feedback, approve, and reject.
- Conversation renders inline tool cards for file, shell, SSH, MCP, skill, subagent, and unknown tool categories.
- Tool drawer shows raw input object, progress timeline, structured content, metadata, and error.

### Tool Classification

The frontend may classify known tool names for display only:

- File: `Read`, `Write`, `Edit`, `MultiEdit`, `Glob`, `Grep`, `LS`
- Shell/system: shell and bash execution tools
- SSH: SSH execution tools
- MCP: MCP dynamic tools
- Skills: `Skill` and skill-related tool events
- Subagents: task or agent tools

The frontend must not infer safety, approval requirements, or execution semantics.

## Phase 3: MCP And Skills

### Goals

- MCP inventory is integrated into the composer and status surfaces.
- Skills are visible without pretending to be a complete runtime catalog.
- MCP actions use real gateway methods.

### myclawd Methods

Use existing methods:

- `mcp_status`
- `mcp_reconnect`
- `mcp_authenticate`

### Frontend Behavior

- Composer `+` menu lists MCP servers, tools, prompts, resources, and MCP-derived skills.
- MCP server details show health, endpoint, auth state, tools, prompts, resources, skills, and errors.
- Reconnect and authenticate actions call `myclawd`.
- Skills are shown from `mcp_status` and observed skill tool events.
- Skill entries can insert intent text into the composer.

### Protocol Gaps

- `skills_status` is missing for a complete runtime skills catalog.
- There is no explicit `skills_invoke` contract.
- MCP prompt/resource content loading is not exposed as a clear first-class websocket method.

## Frontend Module Boundaries

The current `App.tsx` should be split as functionality grows:

- `src/lib/client.ts`: single websocket SDK.
- `src/lib/protocol.ts`: protocol types and method names.
- `src/lib/store.ts`: reducer entrypoint and shared helpers.
- `src/components/Sidebar.tsx`: collapse, new chat, recents, runtime connect.
- `src/components/Conversation.tsx`: transcript and inline tool cards.
- `src/components/Composer.tsx`: sender, model, permissions, MCP, skills, approval badge.
- `src/components/ApprovalDrawer.tsx`: approval handling.
- `src/components/ToolDrawer.tsx`: tool details.

This split should happen during Phase 1 if `App.tsx` changes substantially.

## State Model

The operator state should track:

- `activeSessionKey`
- session list
- session-scoped transcript
- session-scoped tools
- session-scoped approvals
- session-scoped subagents
- session-scoped orchestration
- global MCP inventory
- global or session-visible skills
- protocol gaps

Events that include `session_id` or `session_key` should update only the matching session state.

## Error Handling

- Unsupported websocket methods become visible protocol gaps.
- Failed requests show concise UI errors with method names.
- Reconnect should not discard existing UI state until a new session is successfully bound.
- Session switching errors should keep the previous active session.
- Approval and MCP action failures should leave the current data visible and show the failed action.

## Validation Plan

Backend:

- `go test ./internal/protocol/ws ./internal/gateway ./internal/session`

Frontend:

- `npm run typecheck`
- `npm run build`

Manual:

- Start `myclawd.exe`.
- Connect React UI to `ws://127.0.0.1:18080/ws`.
- Create a new chat and verify a new runtime session appears in Recents.
- Switch sessions and verify transcript restoration.
- Refresh the browser and verify session list recovery.
- Trigger approval-required work and verify composer badge, drawer actions, and tool state updates.
- Trigger file, shell, SSH, MCP, and skill-related work and verify inline tool cards and details.

## Non-Goals

- Do not implement Docker or database as first-class modules.
- Do not call runtime internals from React.
- Do not infer approval or safety rules in the frontend.
- Do not fake sessions, skills, MCP inventory, file trees, or tool history.
