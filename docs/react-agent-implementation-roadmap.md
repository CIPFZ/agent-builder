# ReAct Agent Implementation Roadmap

Status: implementation planning document.

This roadmap turns the ReAct architecture audit into staged work. Each phase is
small enough to implement and validate independently, but detailed enough to
guide coding without re-opening the full architecture debate.

The sequence is intentional:

1. First prove and expose the backend call chain.
2. Then introduce a runtime input contract.
3. Then harden tool continuation and permission semantics.
4. Then improve context, prompt, compact, and memory governance.
5. Then add hooks and error recovery breadth.
6. Then make task/subagent/background work coherent.
7. Finally render the full runtime story in React.

## Phase Map

| Phase | Document | Goal | Primary source of truth |
| --- | --- | --- | --- |
| 1 | `react-agent-phase-01-callchain-observability.md` | Runtime-owned ReAct callchain DTO and diagnostics. | Go runtime messages, turns, ToolCalls, permissions, hooks, events. |
| 2 | `react-agent-phase-02-input-normalization.md` | Normalize text/images/voice/slash/shell/meta input in runtime. | New normalized input DTO and persisted user input evidence. |
| 3 | `react-agent-phase-03-tool-permission-result-loop.md` | Make tool execution, permission decisions, and post-tool continuation explainable. | ToolCall store, permission store, message tool results. |
| 4 | `react-agent-phase-04-context-prompt-compact-memory.md` | Auditable prompt assembly, compact, memory/context selection. | Runtime prompt assembly snapshot and compact/context stores. |
| 5 | `react-agent-phase-05-hooks-error-recovery.md` | Add prompt/compact/sampling hooks and stronger recovery semantics. | Hook execution records and recovery DTOs. |
| 6 | `react-agent-phase-06-tasks-subagents-background.md` | Product-ready task/subagent/background task lifecycle. | Runtime AgentTask stores, child sessions, task messages/results. |
| 7 | `react-agent-phase-07-frontend-react-workbench.md` | React renders runtime truth clearly without owning runtime state. | SessionActivity, callchain DTO, RunProjection, task/compact/hook DTOs. |

## Cross-Phase Rules

- Add backend contract tests before UI.
- Keep HTTP/dev transport and Wails bridge contract-compatible.
- Runtime events are refresh triggers only.
- React may keep local UI state for layout, active panel, expanded rows, and
  pending input composition only.
- React must not derive lifecycle, actionability, artifacts, task state,
  compact state, or final assistant state from prose or local reducers.
- Every phase with visible UI needs a browser smoke on the Vite/dev transport
  and, where relevant, a Wails bridge contract test.
- If a phase changes frontend code, run `cd client && npm run build`.
- Every phase runs `git diff --check`.

## Why Not Start With UI

The current user-visible timeline problems are symptoms. Fixing the UI first
can hide backend ambiguity and make future recovery harder. Phase 1 gives the
frontend an explicit DTO that says:

- which user input started the turn;
- which assistant step produced tool calls;
- which tools ran and why;
- which permissions blocked or allowed execution;
- which tool results were fed back to the model;
- whether the model produced a final assistant message;
- why the loop stopped.

After that, the UI can render the truth instead of guessing from message order.

