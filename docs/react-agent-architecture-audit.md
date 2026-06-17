# ReAct Agent Architecture Audit

Status: current design audit after `a6fa754f`.

Agent Builder baseline: `main` after project/session/terminal ownership and
permission/timeline ordering fixes.

Claude Code comparison source:

- Local source: `C:\Users\ytq\work\ai\myclaw\claude-code`
- Public learning reference: `shareAI-lab/learn-claude-code`

This document records the backend-first review of the single-agent ReAct path.
It intentionally starts from runtime correctness before UI rendering. The UI is
still covered because a correct kernel that cannot explain itself to users is
not a complete product.

## Fixed Product Boundary

- Go runtime is the source of truth.
- React is a product UI and DTO mapper.
- Wails and HTTP are adapters.
- CLI/TUI compatibility is legacy and must not shape the desktop product path.
- Runtime state must not be inferred from terminal text, assistant prose,
  runtime event payloads, action metadata, or browser memory.
- Terminal output must not enter runtime events, RunProjection,
  SessionActivity, or React WorkbenchViewModel.

## Current Single-Agent Call Chain

The current backend single-agent path is structurally valid:

```text
Runtime.Chat
  -> ensureStarted
  -> create/select session
  -> ensure/link Run
  -> create RuntimeTurn queued/running
  -> goroutine runChat
  -> runtime.SendMessage
  -> sessionAgent.Run
  -> create user message
  -> preparePrompt(history)
  -> fantasy.Agent.Stream
  -> assistant text/tool_call persisted in messages
  -> schedulerTool evaluates policy and records ToolCall
  -> hookedTool runs PreToolUse/PostToolUse
  -> real tool executes
  -> OnToolResult writes message.Tool
  -> fantasy continues the next model step
  -> runChat persists final RuntimeTurn, audit, events, Run reconciliation
```

Important Agent Builder evidence:

- `internal/runtime/runtime_turns.go`: `Chat(...)`, `runChat(...)`.
- `internal/agent/agent.go`: `sessionAgent.Run(...)`, `preparePrompt(...)`.
- `internal/agent/scheduler_tool.go`: ToolCall policy, permission, scheduler
  recording, result recording.
- `internal/agent/hooked_tool.go`: hook lifecycle around tool execution.
- `internal/runtime/runtime_permissions.go`: pending permission decisions.
- `internal/runtime/runtime_sessions.go`: SessionActivity hydration.
- `internal/runtime/runtime_compact.go`: compact boundary read and write path.

Important Claude Code comparison evidence:

- `src/utils/processUserInput/processUserInput.ts`
- `src/utils/processUserInput/processSlashCommand.tsx`
- `src/utils/processUserInput/processTextPrompt.ts`
- `src/query.ts`
- `src/utils/toolResultStorage.ts`
- `src/services/compact/*`
- `src/utils/permissions/*`
- `src/utils/hooks/*`
- `src/utils/task/*`, `src/tasks/*`
- `src/utils/systemPrompt.ts`

## ReAct Correctness Finding

The backend is not fundamentally using the wrong ReAct model. The current
model is still:

```text
model response -> tool calls -> tool execution -> tool results -> next model
response -> final assistant response
```

The path can still stop after a tool only when a structured stop condition
fires:

- scheduler policy returns deny and sets `StopTurn`;
- a hook halts the turn;
- provider/API error handling terminates the assistant message;
- context/loop stop conditions stop the loop;
- cancellation interrupts the request.

Therefore, user-visible cases such as "tool finished but no final assistant
content" must be debugged from persisted runtime evidence:

- RuntimeTurn status and error.
- assistant message sequence and finish reasons.
- ToolCall status and error.
- pending or terminal permission requests.
- hook execution status.
- provider error / retry / fallback evidence.

The UI should never decide this from visible timeline order alone.

## Major Gap: Input Normalization

Claude Code has a real input normalization front door. It accepts string input,
content blocks, images, pasted content, slash commands, bash mode, bridge
origin, meta prompts, IDE selection, and prompt-submit hooks. It returns a
normalized message list plus a `shouldQuery` decision.

Agent Builder currently exposes a thin runtime chat request:

```go
type RuntimeChatRequest struct {
    Prompt    string `json:"prompt"`
    SessionID string `json:"sessionId,omitempty"`
    ProjectID string `json:"projectId,omitempty"`
    Scope     string `json:"scope,omitempty"`
}
```

This is too small for the product target. It forces the frontend or adapter to
own distinctions that should be runtime-owned:

- plain text prompt;
- image attachment;
- voice transcript;
- slash command;
- direct shell command mode;
- meta prompt / hidden system input;
- project/session scope;
- prompt-submit hook outcome.

The first architecture repair must introduce runtime-owned normalized input
DTOs before adding more UI behavior.

## Twelve-Area Review

| Area | Current state | Judgment |
| --- | --- | --- |
| User input normalization | Chat request is prompt-only; attachments are supported lower in agent message creation but not as runtime input contract. | Incomplete. Must become a runtime front door. |
| Tool execution / permission / result | Scheduler, policy, pending decisions, ToolCall store, ToolResultGuard, hooks exist. | Core exists; needs sharper callchain observability and stop semantics. |
| Message/context assembly | `preparePrompt` handles history, microcompact, orphan result filtering, synthetic tool results, image support fallback. | Good foundation; needs normalized input and pre-model budget governance. |
| Hooks | PreToolUse, PostToolUse, PostToolUseFailure exist with persistence/replay. | Core exists; prompt/compact/sampling hooks are missing. |
| Todo/towrite | `todos` is available as a tool. | Need runtime-owned todo state review before product UI. |
| Compact | ToolResultGuard, microcompact, compact boundaries, refs exist. | Good foundation; needs Claude Code-like pre-model snip/auto-compact/recovery plan. |
| Memory | Context files, skills, prompt injection, context source summaries exist. | Partial; no unified memory prefetch/selection contract. |
| Subagent | AgentTask/task tools and child sessions exist. | Strong foundation; needs product-visible lifecycle and result return loop. |
| Prompt dynamic assembly | system prompt, MCP instructions, skills XML, prompt prefix exist. | Good foundation; needs auditable prompt assembly snapshot. |
| Error recovery | Orphan tool repair, provider error finish, startup stale cancellation exist. | Good foundation; fallback/tombstone/withheld recoverable errors are incomplete. |
| Task system | Runtime AgentTask stores/messages/results/cancel exist. | Backend foundation exists; UI and scheduler ownership need phase gates. |
| Background task | Shell background and AgentTask runner pieces exist. | Partial; no full scheduled/background task re-entry loop yet. |

## Current Risk

The risk is not that the single-agent ReAct loop is wrong. The risk is that
important harness decisions still leak across layers:

- input type is not normalized at the runtime boundary;
- frontend may still create optimistic timeline illusions;
- compact and context governance are not always pre-model decisions;
- final assistant absence is not yet easy to explain from one runtime DTO;
- tasks/subagents exist but are hard to understand from the product UI.

## Architecture Direction

The implementation should proceed in dependency order:

1. Make the actual ReAct chain observable from runtime DTOs.
2. Add runtime-owned input normalization.
3. Harden tool, permission, and tool-result continuation semantics.
4. Move context/prompt/compact/memory decisions into auditable runtime
   snapshots before model calls.
5. Add missing hook and error recovery boundaries.
6. Finish task/subagent/background task runtime-to-UI surfaces.
7. Render the complete ReAct story in React from runtime DTOs only.

Detailed phase documents:

- `react-agent-phase-01-callchain-observability.md`
- `react-agent-phase-02-input-normalization.md`
- `react-agent-phase-03-tool-permission-result-loop.md`
- `react-agent-phase-04-context-prompt-compact-memory.md`
- `react-agent-phase-05-hooks-error-recovery.md`
- `react-agent-phase-06-tasks-subagents-background.md`
- `react-agent-phase-07-frontend-react-workbench.md`

