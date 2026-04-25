# Operator Session Control Plane Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add real `myclawd` session lifecycle support and wire the React Operator UI left sidebar to real runtime sessions.

**Architecture:** Add websocket session methods in the Go protocol and gateway, backed by `session.Manager`. Then update the React SDK/store/UI so `New chat`, `Recents`, transcript restore, and session switching all flow through `myclawd`.

**Tech Stack:** Go websocket gateway, `session.Manager`, React 19, TypeScript, Ant Design X `Conversations`, Vitest.

---

### Task 1: Add websocket session protocol and gateway handlers

**Files:**
- Modify: `internal/protocol/ws/message.go`
- Modify: `internal/session/manager.go`
- Modify: `internal/gateway/server.go`
- Test: `internal/gateway/server_test.go`

- [ ] Add protocol methods `session_list`, `session_new`, and `session_messages`.
- [ ] Add request payload structs for session creation and message lookup.
- [ ] Add a `CreateSession(agentID string)` helper in `session.Manager` that generates a runtime-owned session key.
- [ ] Add gateway handlers that return session summaries and transcript messages.
- [ ] Add gateway tests that connect, create a session, send messages, list sessions, and restore messages.
- [ ] Run `go test ./internal/protocol/ws ./internal/session ./internal/gateway`.

### Task 2: Extend React protocol and reducer state

**Files:**
- Modify: `web/operator/src/lib/protocol.ts`
- Modify: `web/operator/src/lib/store.ts`
- Test: `web/operator/src/lib/store.test.ts`

- [ ] Add `SessionSummary` and session-scoped state fields.
- [ ] Add reducer actions for `sessions/list`, `session/created`, `session/activate`, `session/messages`.
- [ ] Ensure runtime events update only the active session when a session key/id is available.
- [ ] Add Vitest coverage for session list, activation, transcript restore, and active session updates.
- [ ] Run `npm test`.

### Task 3: Wire session methods into the React app

**Files:**
- Modify: `web/operator/src/App.tsx`
- Modify: `web/operator/src/styles.css`

- [ ] Bootstrap `session_list` after connect.
- [ ] Replace static `sessionItems` with real session summaries.
- [ ] Make `New chat` call `session_new`, then connect to the returned `session_key`.
- [ ] Make selecting a recent session call `session_messages` and refresh session-scoped panels.
- [ ] Keep the visible connect control in the header.
- [ ] Keep the left sidebar collapse behavior.
- [ ] Run `npm run typecheck` and `npm run build`.

### Task 4: Manual validation and commit

**Files:**
- Modify if needed: `docs/tasks/react-operator-ui-console/test-validation-plan.md`

- [ ] Start `myclawd.exe`.
- [ ] Start the React dev server manually or with the user-approved command.
- [ ] Connect to `ws://127.0.0.1:18080/ws`.
- [ ] Create a new chat and verify it appears in Recents.
- [ ] Send a message in two different sessions and verify switching restores each transcript.
- [ ] Refresh the browser and verify sessions list from the daemon.
- [ ] Commit the implementation with a concise message.
