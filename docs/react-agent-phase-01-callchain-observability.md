# Phase 1: Runtime ReAct Callchain Observability

Status: planned.

## Goal

Expose a runtime-owned ReAct callchain read model for a session and for a turn.
This phase does not change model behavior. It makes the current behavior
auditable and gives React a stable display contract.

## User Problem

Users currently see confusing timeline behavior:

- assistant text can appear above the tool that it was actually preparing;
- a tool may finish but the UI does not clearly show whether the model
  continued;
- permission cards can look detached from the exact tool step;
- final assistant absence is hard to distinguish from frontend rendering bugs.

The product needs a single runtime read that explains the sequence.

## Claude Code Reference

Claude Code keeps a structured message loop in `src/query.ts`:

- assistant messages are collected per API iteration;
- tool_use blocks are detected during streaming;
- tool results are appended as user/tool result messages;
- the loop continues when tool_use occurred;
- missing tool results are synthesized on error or abort.

The equivalent Agent Builder read model should not copy Claude Code UI. It
should expose the same underlying facts.

## Current Agent Builder Evidence

- `internal/runtime/runtime_turns.go`
  - `Chat(...)` creates queued/running turns.
  - `runChat(...)` persists terminal turn status and latest assistant message.
- `internal/agent/agent.go`
  - `OnToolCall` and `OnToolResult` persist assistant and tool messages.
  - `OnStepFinish` writes finish reasons.
  - `preparePrompt(...)` filters orphan results and synthesizes missing ones.
- `internal/runtime/runtime_tool_calls.go`
  - ToolCall store is the structured lifecycle source.
- `internal/runtime/runtime_permissions.go`
  - Permission store owns active and terminal permission decisions.
- `internal/runtime/runtime_hooks.go`
  - Hook records explain pre/post tool decisions.

## Backend Contract

Add read-only DTOs:

```go
type RuntimeReactCallchainResponse struct {
    SessionID string                   `json:"sessionId"`
    TurnID    string                   `json:"turnId,omitempty"`
    Nodes     []RuntimeReactCallNode   `json:"nodes"`
    Summary   RuntimeReactCallSummary  `json:"summary"`
    Source    RuntimeReactCallSource   `json:"source"`
}

type RuntimeReactCallNode struct {
    ID                 string                 `json:"id"`
    ParentID           string                 `json:"parentId,omitempty"`
    Kind               string                 `json:"kind"`
    SessionID          string                 `json:"sessionId"`
    TurnID             string                 `json:"turnId,omitempty"`
    MessageID          string                 `json:"messageId,omitempty"`
    ToolCallID         string                 `json:"toolCallId,omitempty"`
    PermissionID       string                 `json:"permissionId,omitempty"`
    HookExecutionID    string                 `json:"hookExecutionId,omitempty"`
    Sequence           int                    `json:"sequence"`
    Status             string                 `json:"status,omitempty"`
    FinishReason       string                 `json:"finishReason,omitempty"`
    Title              string                 `json:"title,omitempty"`
    Summary            string                 `json:"summary,omitempty"`
    Error              string                 `json:"error,omitempty"`
    StartedAt          int64                  `json:"startedAt,omitempty"`
    FinishedAt         int64                  `json:"finishedAt,omitempty"`
    Evidence           map[string]string      `json:"evidence,omitempty"`
}
```

Recommended `Kind` values:

- `user_input`
- `assistant_step`
- `tool_call`
- `permission_request`
- `permission_decision`
- `hook_execution`
- `tool_result`
- `assistant_final`
- `turn_terminal`
- `synthetic_recovery`

Recommended `Summary` fields:

- `hasFinalAssistant`
- `finalAssistantMessageId`
- `lastAssistantFinishReason`
- `toolCallCount`
- `permissionCount`
- `hookCount`
- `stopReason`
- `missingEvidence`

Recommended `Source` fields:

- `sessionActivityParity: true`
- `usesMessages: true`
- `usesToolCalls: true`
- `usesPermissions: true`
- `usesHooks: true`
- `eventsAreRefreshOnly: true`

## Backend Implementation

1. Add `runtime_react_callchain.go`.
2. Build callchain from durable reads:
   - turns from `runtime_turns`;
   - messages from session message store;
   - ToolCalls from scheduler store;
   - permissions from permission store and in-memory pending set;
   - hook executions from hook store;
   - events only as optional timestamp/order evidence, not state truth.
3. Use structural order first:
   - user input message;
   - assistant message containing tool_call;
   - tool call lifecycle;
   - permission/hook nodes under the tool call;
   - tool result;
   - next assistant step or final assistant;
   - terminal turn node.
4. Detect and report anomalies:
   - assistant tool call without tool result;
   - tool result without assistant tool call;
   - ToolCall store status conflicts with message evidence;
   - turn completed without final assistant message;
   - permission pending after terminal tool/turn.
5. Do not mutate state in the read path.

## HTTP And Wails

Add transport-neutral service methods:

```go
ReactCallchain(ctx context.Context, turnID string) (RuntimeReactCallchainResponse, error)
SessionReactCallchain(ctx context.Context, sessionID string, limit int) (RuntimeReactCallchainResponse, error)
```

HTTP routes:

```text
GET /v1/turns/{turn_id}/react-callchain
GET /v1/sessions/{session_id}/react-callchain?limit=N
```

Wails bridge:

```go
ReactCallchain(turnID string)
SessionReactCallchain(sessionID string, limit int)
```

## Frontend Display

Do not replace the whole chat UI in this phase. Add a read-only diagnostic
surface behind the existing timeline:

- a compact "Turn chain" inspector in the right/diagnostics area;
- each node displayed as an ordered row;
- tool calls grouped under assistant steps;
- permission and hook rows grouped under the tool call they affected;
- final assistant status visible even when final assistant text is empty;
- anomaly banner if runtime says evidence is missing or conflicting.

The primary chat timeline may continue using current `SessionActivity`, but
it should be allowed to call the callchain read for debugging.

## Frontend Ownership Rules

- React does not build the callchain from conversation messages.
- React does not infer final assistant absence.
- React does not re-order nodes except for stable rendering of the backend
  `sequence`.
- Expansion/collapse state is local UI state and can stay in React.

## Tests

Backend tests:

- single text-only assistant final;
- assistant -> one tool -> final assistant;
- assistant -> two tools -> final assistant;
- permission ask -> allow once -> tool result -> final assistant;
- permission deny -> StopTurn -> no final assistant, stop reason explained;
- hook halt -> StopTurn, hook evidence linked to tool;
- provider error after tool call creates synthetic result evidence;
- orphan tool result is reported but not treated as source of truth.

HTTP/Wails tests:

- routes call runtime service and preserve `turnID/sessionID`;
- missing turn returns not found;
- limit applies only to number of turns/nodes returned, not node correctness.

Frontend tests:

- maps DTO nodes into view models without parsing assistant prose;
- shows final assistant absent/present from summary;
- groups permission/hook under tool call.

Browser smoke:

- run a prompt that triggers one tool;
- open diagnostic callchain;
- verify user, assistant step, tool call, tool result, final assistant appear
  in order.

## Acceptance Criteria

- A backend DTO can explain the exact ReAct sequence for a turn.
- The DTO can distinguish backend stop from frontend rendering failure.
- The UI can display the chain without message parsing.
- No runtime state is derived from runtime event payloads or React state.

