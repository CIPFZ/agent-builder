# React Operator UI Console Implementation Plan

Date: 2026-04-25

## 1. Implementation Strategy

Implement the React UI in capability-visible phases.

The first goal is to see and operate everything already implemented.

Do not start by polishing a generic chat screen. Start by proving that the UI can expose the current runtime.

## 2. Work Package A: Frontend Bootstrap

Required outcome:

- add a React app workspace under an agreed directory, recommended `web/operator`
- use TypeScript
- use Ant Design and Ant Design X
- add websocket client for `myclawd`
- add typed protocol models for current `internal/protocol/ws/message.go`

Suggested stack:

- Vite
- React
- TypeScript
- Ant Design
- Ant Design X
- `@tanstack/react-query` only if useful for request state
- lightweight local state store such as Zustand only if protocol state becomes hard to manage with React state

## 3. Work Package B: myclawd Client SDK

Required outcome:

- single frontend client module owns websocket connect/send/request/event subscription
- request/response correlation by ID
- event reducer for session, transcript, tools, approvals, tasks, MCP, skills
- reconnect state visible in UI

Do not scatter raw websocket calls across components.

## 4. Work Package C: Conversation Surface

Required outcome:

- `Bubble.List` renders user, assistant, tool, system, and attachment messages
- `Sender` sends prompts through `send_message`
- streaming `assistant.delta` updates the active assistant bubble
- final `message.created` reconciles streaming content
- tool calls render inline cards

## 5. Work Package D: Operator Side Panels

Required outcome:

- approval center
- subagent/task panel
- MCP panel
- skills panel
- runtime/session status panel
- tool execution detail drawer

These panels should be visible from the first usable UI, even if some show protocol gaps.

## 6. Work Package E: Capability Views

Required outcome:

- file operations view from file-related tool events
- shell/SSH execution view from tool events
- MCP inventory page from `mcp_status`
- skills page from MCP skill inventory plus transcript skill attachments
- task dashboard from subagent APIs/events

## 7. Work Package F: Protocol Gap Ledger

Required outcome:

- document every frontend requirement that cannot be met by current `myclawd`
- do not fake missing backend truth
- create follow-up backend tasks when needed

Initial expected gaps:

- `skills_status`
- `tools_inventory`
- `session_messages` or `session_history`
- `file_tree` / `file_preview`
- normalized operation history

## 8. Delivery Order

Implement in this order:

1. frontend scaffold and dev command
2. typed `myclawd` client
3. conversation page with streaming transcript
4. tool lifecycle cards
5. approval center
6. subagent/task panel
7. MCP and skills panels
8. runtime/session status panel
9. protocol gap ledger and manual validation docs

## 9. Completion Standard

The module is complete when:

- the React UI can connect to local `myclawd`
- user can chat with the agent
- tool calls are visible
- approvals can be handled
- MCP inventory is visible
- skills visibility is present with documented gaps
- subagent/task state is visible
- current runtime/session status is visible
- missing backend contracts are explicitly listed
