# Phase 3: Tool, Permission, And Tool Result Loop

Status: planned.

## Goal

Make every tool execution path explainable and ensure post-tool continuation is
correct. The runtime must be able to answer: "Did the model see this tool
result, and why did the turn continue or stop?"

## User Problem

Users currently see permission cards and tool rows, but the UI can still feel
out of order. When a command completes with no final assistant text, users
cannot tell whether:

- the runtime stopped intentionally;
- the model failed after receiving the result;
- permission was denied;
- a hook halted the turn;
- the frontend failed to render the final assistant.

## Claude Code Reference

Claude Code `src/query.ts` treats tool_use detection as the loop continuation
signal, not the provider stop reason alone. It also synthesizes missing tool
results on abort/error and avoids leaking orphan tool results into retries.

Claude Code `src/utils/toolResultStorage.ts` persists large results and applies
aggregate tool result budgets before subsequent model calls.

## Current Agent Builder Evidence

- `internal/agent/scheduler_tool.go`
  - policy evaluation;
  - ToolCall start/output/complete/fail;
  - `StopTurn` on deterministic deny.
- `internal/agent/hooked_tool.go`
  - pre/post hook execution;
  - hook halt and result blocking.
- `internal/agent/agent.go`
  - `OnToolResult` writes tool message;
  - `OnStepFinish` converts `StopTurn` to end-turn finish reason;
  - error path synthesizes missing tool results.
- `internal/runtime/runtime_permissions.go`
  - pending permission decision path.
- `internal/agent/tool_result_guard.go`
  - large result persistence and turn budget.

## Runtime Work

### 1. Stop Reason Normalization

Add explicit stop reason fields to terminal turn/callchain diagnostics:

- `model_stop`
- `tool_use_followup`
- `permission_denied`
- `hook_halted`
- `provider_error`
- `context_limit`
- `loop_detected`
- `cancelled`
- `interrupted`

The stop reason must be written by runtime code, not inferred by React.

### 2. Tool Result Delivery Evidence

For each tool call, record whether its result entered the next model input:

```go
type RuntimeToolResultDelivery struct {
    ToolCallID          string `json:"toolCallId"`
    ToolResultMessageID string `json:"toolResultMessageId,omitempty"`
    DeliveredToModel    bool   `json:"deliveredToModel"`
    DeliveredAtStep     int    `json:"deliveredAtStep,omitempty"`
    Synthetic           bool   `json:"synthetic,omitempty"`
    Reason              string `json:"reason,omitempty"`
}
```

This can initially be computed read-only from message order and step sequence.
If the read cannot be reliable, add a narrow persisted delivery marker in the
agent loop.

### 3. Permission Semantics

Harden permission flow:

- pending permission must point to session, turn, and ToolCall;
- allow once resumes only the blocked tool;
- allow session records durable grant in permission service;
- deny completes ToolCall as denied and explains StopTurn if applicable;
- terminal permissions never become actionable again after reload.

### 4. Tool Result Guard

Review ToolResultGuard against Claude Code:

- per-tool threshold;
- exempt tools;
- persisted-output marker;
- aggregate turn budget;
- disk cleanup;
- original-size metadata;
- model-visible instruction for reading persisted output.

Do not put full tool output into runtime events, SessionActivity, RunProjection,
or WorkbenchViewModel.

### 5. Lazy Tool Loading

The existing `selectToolsForPreparedStep(...)` is the correct location for
tool disclosure. Strengthen it so diagnostics can explain:

- all available tools;
- selected tools for this step;
- base tools always included;
- why a tool was omitted.

## Frontend Display

Update tool rows so each tool shows:

- status: pending, waiting permission, running, completed, failed, denied,
  cancelled;
- permission decision, if any;
- hook status, if any;
- compact/persisted output indicator;
- "fed back to model" state;
- final turn stop reason.

If there is no final assistant text, show a concise runtime-provided reason:

- "Stopped after permission denial."
- "Stopped by hook."
- "Provider failed after tool result."
- "Tool result delivered; final response is empty."

Do not invent explanatory prose in React. Use runtime DTO fields.

## Frontend Ownership Rules

- React may own expanded/collapsed state for tool rows.
- React may show a transient spinner while waiting for runtime hydration.
- React must not infer tool completion from timeline position.
- React must not infer permission actionability from a button it rendered
  earlier.
- React must not decide whether a tool result was fed back to the model.
- React must not parse terminal output or assistant prose to decide tool
  success.

## Tests

Runtime tests:

- allow once resumes tool and delivers result;
- allow session grants repeated call without second prompt;
- deny records denied ToolCall and StopTurn reason;
- hook deny blocks only tool unless halt is set;
- hook halt stops turn;
- large result is persisted and delivery still records true;
- missing tool result synthetic path records synthetic recovery;
- final assistant empty still has terminal reason.

Frontend tests:

- permission decision buttons use runtime enum values;
- tool row displays delivery state from DTO;
- no final assistant uses runtime reason, not local inference.

Browser smoke:

- ask for environment info in ask mode;
- approve once;
- verify tool runs, result is displayed, and final assistant/stop reason is
  visible from runtime data.

## Acceptance Criteria

- A user can see why a turn stopped after a tool.
- Permission decisions are reflected by runtime rereads.
- Tool result visibility and delivery are runtime-owned.
- React does not parse assistant prose or tool output to infer lifecycle.
