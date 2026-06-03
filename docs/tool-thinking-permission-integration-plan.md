# Tool, Thinking, And Permission Integration Plan

Status: completed on 2026-06-03.

This frontend/backend integration milestone is complete. The main conversation
surface now consumes runtime-owned tool calls, safe thinking summaries, policy
mode, permission requests, and active-turn state instead of using React-owned
mock state.

Completion notes:

- Real runtime tool calls render as timeline cards with lifecycle status and
  output.
- Runtime permission requests render inline and support allow once, allow for
  session, and deny.
- Permission mode is loaded from and saved to the runtime policy API.
- Thinking content is grouped by turn, collapsed by default, and shown only as
  runtime-provided safe content.
- Multi-session execution no longer cancels the previous session when a new
  session starts.
- Sidebar sessions show active execution state independently.
- Timeline ordering is user message, thinking, tool, permission/progress, then
  assistant response.
- Browser verification covered allow, deny, pending permission UI behavior,
  tool output display, and multi-session busy indicators.

Superseding next step:

- [`runtime-parity-closure-stabilization-plan.md`](./runtime-parity-closure-stabilization-plan.md)

This plan starts after the main composer can use the configured runtime
provider/model for real chat. The next milestone is to make tool use, safe
thinking/progress display, and permission decisions visible and usable from the
main conversation surface.

## Goal

When the user enters a task such as:

```text
ping 一下 百度
```

Agent Builder should route the request through the real runtime, expose the
runtime-selected tool calls, enforce the configured permission mode, and render
the resulting timeline without React inventing tool or permission state.

The UI must make these facts clear:

- which tool the runtime wants to use;
- whether the tool is waiting for permission, running, completed, failed, or
  denied;
- what permission mode produced the decision;
- what safe thinking/progress summary is available;
- what final assistant response was produced.

## Current Baseline

Already implemented or available:

- Main chat sends real turns through the configured provider/model.
- Runtime/SQLite remains the state source for sessions, turns, messages,
  provider settings, and selected model.
- `RuntimeMessage.parts` can expose:
  - `text`
  - `reasoning`
  - `tool_call`
  - `tool_result`
  - `finish`
- Runtime has `RuntimeToolCall` DTOs and persisted scheduler/store support.
- HTTP routes exist for:
  - `GET /v1/turns/{turn_id}`
  - `GET /v1/turns/{turn_id}/tool-calls`
  - `GET /v1/tool-calls/{tool_call_id}`
  - `GET /v1/permissions`
  - `POST /v1/permissions/{permission_id}/decision`
  - `GET /v1/policy`
  - `PUT /v1/policy`
- Wails bridge already exposes permissions and policy methods.
- Frontend has a runtime adapter boundary and HTTP/dev fallback.
- Current conversation rendering is still message-only with `Bubble.List`.

Important constraints:

- Do not expose hidden chain-of-thought.
- Only display safe runtime-provided reasoning summaries, progress labels,
  tool plans, and tool metadata.
- Do not add mock/default business data in React.
- Vite/browser dev must continue to work through HTTP/dev fallback.

## Product Permission Modes

The user-facing permission modes should be the three modes below. They map to
runtime policy, but the runtime remains the source of truth.

| UI label | Product meaning | Runtime mapping | Required behavior |
| --- | --- | --- | --- |
| 默认模式 | Ask before non-preapproved tools | `ask` | Tool calls request approval unless a scoped runtime rule allows/denies them. |
| 自动审查 | Allow read-only, ask for risky actions | `auto_read` | Read-only tools can run; execute/network/write/destructive/secret tools ask. |
| 完全访问权限 | User explicitly permits broad tool execution | New product alias, likely `bypass_permissions` or `full_access` | Needs backend support before UI can enable it. Must still keep audit records and should not silently bypass destructive safeguards unless explicitly designed. |

Existing runtime modes are:

- `ask`
- `auto_read`
- `plan`
- `deny_all`

Implementation decision needed:

- Add a runtime-supported `full_access`/`bypass_permissions` mode, or
- represent "完全访问权限" as high-precedence allow rules with an explicit profile.

Recommendation:

1. Keep `ask` and `auto_read` as direct mappings.
2. Add a dedicated runtime mode for full access only if backend policy, audit,
   recovery, and UI warnings can all represent it clearly.
3. Until then, show "完全访问权限" disabled with an explanation that runtime
   support is pending.

## Runtime Contract Additions

Frontend should not assemble the timeline by guessing from message text. The
adapter should map these runtime-owned facts into view models:

```text
ConversationTimelineItem
  id
  kind: message | thinking | tool_call | tool_result | permission | turn_status
  sessionId
  turnId
  messageId optional
  toolCallId optional
  role optional
  status
  title
  content
  summary
  createdAt
  updatedAt
```

Minimum backend/frontend contract for Phase 1:

- Extend frontend DTO mirrors for:
  - `RuntimeMessage.parts`
  - `RuntimeToolCall`
  - `RuntimePermissionRequest`
  - `RuntimePolicy`
- Hydrate active session with:
  - messages from `GET /v1/sessions/{id}/messages`
  - active turn from status or `GET /v1/turns?status=active`
  - tool calls for active/recent turns from `GET /v1/turns/{id}/tool-calls`
  - pending permissions from `GET /v1/permissions`
  - current policy from `GET /v1/policy`
- Keep polling as a short fallback while busy; event stream can replace it
  later.

Backend gap to verify early:

- `RuntimeBridge` should expose `TurnToolCalls` and `ToolCall` if generated
  Wails bindings are expected to support the same surface as HTTP.
- HTTP/dev module fallback must support permission decision and policy update
  routes.
- Tool call recording must happen for actual shell/builtin/MCP execution, not
  only when a final message is read.

## Frontend UI Plan

Keep the existing shell. Add runtime-specific timeline components inside the
workspace rather than redesigning the whole app.

New frontend modules:

```text
client/src/features/timeline/
  Timeline.tsx
  Timeline.module.css
  TimelineItem.tsx
  ThinkingItem.tsx

client/src/features/tools/
  ToolCallCard.tsx
  ToolCallCard.module.css

client/src/features/permissions/
  PermissionGate.tsx
  PermissionModeControl.tsx
  PermissionReviewModal.tsx
```

View model additions:

```text
WorkbenchViewModel
  timeline: ConversationTimelineItem[]
  permissions:
    mode
    label
    options[]
    pending[]
```

Renderer behavior:

- User and assistant messages still use Ant Design X `Bubble`.
- Safe thinking/progress uses `ThoughtChain` or compact custom rows.
- Tool calls use `ToolCallCard` with:
  - icon by source: shell, builtin, MCP, unknown
  - status badge
  - command/input summary
  - output summary/stdout/stderr preview
  - policy/risk details
- Pending permission uses `PermissionGate` and/or modal with:
  - tool name
  - action/target/path
  - risk
  - policy reason
  - buttons: allow once, allow for session, deny
- The composer permission control shows only the three product modes:
  - 默认模式
  - 自动审查
  - 完全访问权限

Copy and retry behavior remains separate from tool/permission rendering.

## Thinking Display Rules

Do not display raw hidden chain-of-thought.

Allowed:

- Runtime-provided `reasoning` part only if it is already considered safe
  summary content by the runtime.
- Tool planning metadata such as "准备执行 shell: ping baidu.com".
- Progress states such as "等待权限", "执行中", "已完成".
- Summaries from audit/tool call output.

Not allowed:

- Asking the model to reveal private reasoning.
- Rendering provider hidden chain-of-thought fields.
- Treating `thinking` as always safe without an explicit runtime decision.

Recommended first pass:

- Render reasoning parts collapsed by default and label them as "思考摘要".
- Add a runtime field later if stronger separation is needed:

```text
RuntimeMessagePart
  type: reasoning
  thinking: string
  displaySafe: bool
  label: string
```

## Ping Baidu Acceptance Scenario

Primary manual scenario:

1. Set model to configured DeepSeek model.
2. Set permission mode to 默认模式.
3. Send:

```text
ping 一下 百度
```

Expected:

- User message appears immediately.
- Timeline shows a tool call intent, likely shell/network.
- Permission prompt appears before execution if policy classifies it as
  execute/network.
- Approving allows the turn to continue.
- Tool card changes from waiting permission to running to completed/failed.
- Assistant summarizes the result.
- Refresh recovers the message, tool card, permission decision, and final
  assistant response from runtime APIs.

Secondary scenarios:

- 自动审查:
  - read-only tools run without prompt;
  - shell/network ping still asks unless runtime explicitly classifies it safe.
- 完全访问权限:
  - disabled until backend support exists, or runs without prompt only after
    explicit runtime mode support and audit coverage are implemented.
- Deny:
  - permission denial marks the tool call denied/failed;
  - turn does not leave UI stuck in busy state;
  - assistant or timeline shows denial result.
- Cancel while waiting:
  - pending permission expires/cancels;
  - turn becomes cancelled;
  - UI clears stop state.

## Implementation Order

1. Runtime contract audit
   - Confirm tool call lifecycle is persisted during real turns.
   - Confirm permission requests include `turnId` and `toolCallId`.
   - Add missing HTTP/Wails bridge methods for tool calls and policy if needed.

2. Adapter DTOs and view model
   - Add TypeScript DTOs for message parts, tool calls, permissions, and policy.
   - Add adapter methods:
     - `loadPolicy`
     - `updatePolicy`
     - `loadPermissions`
     - `decidePermission`
     - `loadTurnToolCalls`
   - Keep HTTP/dev fallback parity.

3. Timeline composition
   - Convert runtime messages and parts into `ConversationTimelineItem[]`.
   - Merge tool calls by `turnId/toolCallId`.
   - Merge pending permissions by `toolCallId`.
   - Avoid duplicate tool cards when both message parts and tool-call APIs
     reference the same tool.

4. Permission UI
   - Replace current static composer permission dropdown with runtime policy.
   - Add `PermissionReviewModal` or inline `PermissionGate`.
   - Wire allow once, allow for session, deny to runtime.

5. Tool card UI
   - Add compact cards for shell/builtin/MCP tool calls.
   - Show risk/policy metadata without overwhelming the conversation.
   - Add detail affordance later for stdout/stderr/artifact refs.

6. Browser verification
   - Use the in-app browser to test mode switch, send, approve/deny, cancel,
     refresh recovery.

## Tests

Frontend:

- `cd client && npm run build`
- `cd client && npm run lint`

Backend when touching runtime/bridge:

- Focused runtime tests:
  - `go test ./internal/runtime -run "Permission|Policy|ToolCall|Turn" -count=1`
- Bridge tests:
  - `go test ./desktop -count=1`

Add or extend tests for:

- HTTP routes for `GET /v1/turns/{id}/tool-calls`.
- HTTP routes for `GET /v1/permissions` and decision.
- Wails bridge parity for tool call and permission methods.
- Policy mode update and persistence.
- Denied permission updates tool call and turn state.
- Recovery of pending permissions after refresh/restart.

## Risks

- Existing `reasoning.thinking` may contain content that should not be shown.
  Keep it collapsed and consider adding `displaySafe`.
- "完全访问权限" is not currently a direct runtime mode. Do not fake it in
  React.
- A model may not choose a shell tool for "ping 一下 百度" unless tool
  disclosure, prompts, and policy make the tool available.
- Tool call data may be present in message parts but not in scheduler store for
  all execution paths. Normalize before building UI around it.
- Event stream is not yet the primary frontend update path; short polling while
  busy is acceptable but should not become the long-term design.

## Done Criteria

- The main chat can show at least one real tool call generated by the runtime.
- Pending permission can be approved or denied from the UI.
- Permission mode shown in composer is loaded from runtime policy.
- Tool and permission state survives browser refresh.
- No mock tool, permission, or thinking data exists in React.
- Build/lint pass, and relevant Go tests pass if backend or bridge code changed.
