# Phase 7: React Workbench Runtime Rendering

Status: planned.

## Goal

Render the complete runtime-owned ReAct story in the product UI. React should
make the agent understandable without becoming the runtime source of truth.

## User Problem

Even when backend state is correct, users judge the product through the UI.
The current UI can show confusing order, stale optimistic messages, or missing
final assistant context. This phase aligns the workbench with runtime DTOs.

## Claude Code Reference

Claude Code's UI is not the visual target, but its data discipline is useful:

- `src/query.ts` yields assistant, tool result, recovery, and tombstone events
  from the agent loop.
- `src/utils/processUserInput/*` separates input processing from model query
  execution.
- `src/components/permissions/*` renders permission decisions from structured
  permission state.
- `src/components/tasks/*` renders task state from task records rather than
  assistant prose.
- `src/services/compact/*` produces compact boundary and summary messages from
  explicit compact decisions.

Agent Builder should render equivalent runtime-owned facts through a desktop
product UI, not copy Claude Code's terminal/Ink surfaces.

## Current Agent Builder Evidence

- `client/src/app/shell/WorkbenchShell.tsx`
  - owns workbench composition, optimistic prompt rows, and high-level actions.
- `client/src/features/timeline/Timeline.tsx`
  - renders assistant/user/tool/process timeline rows.
- `client/src/features/permissions/PermissionGate.tsx`
  - renders permission decisions and decision failures.
- `client/src/runtime/wailsWorkbenchAdapter.ts`
  - maps runtime DTOs into `WorkbenchViewModel`.
- `client/src/runtime/workbenchTypes.ts`
  - defines frontend view models and runtime adapter interface.
- `internal/runtime/runtime_sessions.go`
  - hydrates `SessionActivity`, activity windows, and turn activity.
- `internal/runtime/runtime_contract_types.go`
  - defines runtime DTOs consumed by the frontend.

## Backend Prerequisites

This phase should start only after the relevant DTOs exist:

- SessionActivity / TurnActivity;
- ReactCallchain;
- normalized input evidence;
- ToolCall / permission / hook DTOs;
- compact/prompt assembly DTOs;
- AgentTask DTOs;
- RunProjection where needed.

## Frontend Architecture

The adapter maps runtime DTOs into display view models:

```text
runtime DTOs
  -> adapter mapping
  -> WorkbenchViewModel display fields
  -> React components
```

React local state is allowed for:

- composer draft;
- selected project/session;
- selected tab/panel;
- expanded/collapsed rows;
- scroll position;
- transient optimistic spinner with strict replacement after runtime reread.

React local state is not allowed for:

- turn status;
- final assistant status;
- tool status;
- permission actionability;
- task status;
- compact state;
- artifact refs;
- terminal ownership;
- Run lifecycle.

## Timeline Rendering Model

Render timeline from runtime structural rows:

```text
User input
Assistant step
  Tool call
    Permission/hook
    Tool result
Assistant final
Turn terminal marker
```

Rules:

- assistant messages containing tool calls are not displayed as final answers;
- pre-tool assistant prose can be shown as assistant step text if the runtime
  marks it as step content;
- final assistant is displayed only when runtime identifies it;
- missing final assistant is displayed as a runtime stop reason;
- permission cards are nested under their tool call;
- tool result output is summarized, never full unbounded content;
- compact markers are backend rows.

## UI Surfaces

### Main Chat

- readable conversation;
- grouped tool/process rows;
- final assistant text;
- terminal turn status;
- retry/follow-up affordances when runtime allows them.

### Callchain Inspector

- full ordered runtime callchain;
- evidence ids for debugging;
- anomalies and stop reasons;
- useful during development and later as advanced diagnostics.

### Context Panel

- model/provider;
- prompt assembly summary;
- budget;
- tools selected;
- skills/MCP/context sources;
- compact boundaries.

### Task Panel

- active/completed tasks;
- child session navigation;
- follow-up/cancel actions;
- output/artifact refs.

### Permission UI

- active permission request;
- allow once / allow session / deny;
- terminal permission read-only state after decision/reload;
- decision failure surfaced in-card.

## Adapter Work

1. Add low-level methods for new DTOs.
2. Keep full hydrate fallback.
3. Use action refresh targets only as reread selectors.
4. Never merge action payloads into business state.
5. Preserve browser dev fallback:
   - Wails desktop binding when available;
   - HTTP/dev transport otherwise;
   - dev module fallback where fetch/XHR is unavailable.

## Visual Design

Use Ant Design and Ant Design X patterns already present in the app.

Important behavior:

- dense operational UI, not marketing layout;
- no nested cards for timeline rows;
- stable row dimensions where possible;
- tool/process rows remain readable with long command names;
- permission buttons are clear and show errors;
- no terminal output in WorkbenchViewModel.

## Tests

Frontend tests:

- mapper uses DTO status for final assistant;
- mapper drops stale optimistic rows after runtime hydration;
- permission actionability comes from DTO;
- callchain rows preserve backend sequence;
- task and compact panels do not parse prose.

Build:

```text
cd client && npm run build
git diff --check
```

Browser smoke:

- text-only prompt;
- tool prompt with permission;
- deny permission;
- allow permission and observe final assistant;
- compact/tool-result indicator if fixture available;
- task/subagent prompt if fixture available;
- reload and verify no stale local actionability.

## Acceptance Criteria

- The UI explains the ReAct loop clearly.
- Runtime DTOs are the only source of lifecycle/actionability truth.
- Users can see why a turn stopped or continued.
- Runtime capabilities are visible enough to be useful.
