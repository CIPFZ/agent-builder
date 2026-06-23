# Phase 6: Tasks, Subagents, And Background Work

Status: planned.

## Goal

Make AgentTask, subagent, and background task behavior product-ready. The
runtime already owns much of the data; this phase turns it into an explainable,
controllable workflow.

## User Problem

Subagents and background tasks are powerful only if users can see:

- what task was created;
- which session owns it;
- what it is allowed to do;
- whether it is still running;
- how to send follow-up;
- what result/artifacts it produced;
- how cancellation works.

Without clear UI, runtime capability feels invisible or unreliable.

## Claude Code Reference

Relevant Claude Code areas:

- `src/tools/AgentTool/*`
- `src/tools/TaskCreateTool/*`
- `src/tools/TaskGetTool/*`
- `src/tools/TaskListTool/*`
- `src/tools/TaskStopTool/*`
- `src/tools/TaskOutputTool/*`
- `src/tasks/*`
- `src/utils/task/*`
- `src/hooks/useTasksV2.ts`
- `src/hooks/useBackgroundTaskNavigation.ts`
- `src/utils/processUserInput/processSlashCommand.tsx` background fork path.

## Current Agent Builder Evidence

- `internal/agent/task_tools.go`
- `internal/runtime/runtime_agent_tasks.go`
- `internal/runtime/runtime_agent_task_tools.go`
- `internal/runtime/runtime_agent_task_comm_store.go`
- `internal/runtime/runtime_agent_task_runner.go`
- `internal/runtime/runtime_agent_roles.go`
- `internal/runtime/runtime_agent_task_scope.go`
- `internal/agent/coordinator.go`

## Runtime Work

### Task Lifecycle Contract

Confirm and document lifecycle:

```text
created -> running -> completed
                  -> failed
                  -> cancelled
```

Each task must have:

- parent session id;
- parent turn id;
- parent tool call id;
- child session id;
- role;
- allowed tools;
- capability scope;
- cwd/worktree;
- provider/model;
- result refs;
- artifact refs;
- cancellation detail.

### Task Visibility APIs

Add or harden:

```text
GET /v1/sessions/{session_id}/agent-tasks
GET /v1/turns/{turn_id}/agent-tasks
GET /v1/agent-tasks/{task_id}
POST /v1/agent-tasks/{task_id}/messages
POST /v1/agent-tasks/{task_id}/cancel
GET /v1/agent-tasks/{task_id}/output
```

Wails bridge must expose the same reads/actions.

### Background Task Boundary

Do not add a general unattended scheduler until foreground/user-triggered
semantics are solid.

First supported background model:

- user-triggered task runs in child session;
- task can continue while user views another session;
- result is delivered into runtime task messages;
- UI can navigate to child session;
- cancellation is explicit.

Scheduled tasks can be a later subphase with a separate design gate.

### Task Result Re-entry

If a task result should inform the main agent, it must re-enter through a
runtime-owned meta input:

```text
AgentTask completed
  -> runtime creates task result message
  -> optional user-approved "send result to main agent"
  -> SubmitUserInput(isMeta=true, source=task_result)
```

Do not let React inject hidden prompts.

## Frontend Display

Add a task panel:

- active tasks list;
- completed/failed/cancelled tasks;
- task details drawer;
- child session link;
- allowed tools/scope;
- progress/messages;
- output/artifact refs;
- follow-up input;
- cancel button for active tasks.

Timeline rendering:

- when a tool creates a task, show a task row under that tool;
- task progress can be collapsed;
- task completion should show summary and artifact refs;
- background tasks should be visible even after session switch.

## Frontend Ownership Rules

- React does not own task lifecycle.
- React does not synthesize task messages.
- React can hold selected task id and expanded task rows.
- React actions call runtime and then reread task/session activity DTOs.

## Tests

Runtime tests:

- task creation records parent/child ownership;
- task scope denies disallowed tools;
- follow-up message delivery and processed status;
- cancellation terminalizes task and child session;
- output refs survive restart;
- completed task can be read by parent session activity.

Frontend tests:

- maps task DTO to task panel;
- cancel action rereads runtime;
- follow-up action creates runtime task message;
- session switch does not lose visible active task list if runtime says active.

Browser smoke:

- trigger a subagent/task tool;
- verify task appears under the turn and in task panel;
- send follow-up;
- cancel or complete;
- refresh and verify state survives from runtime.

## Acceptance Criteria

- Users can see and control runtime-owned tasks.
- Task/subagent state survives reload.
- Background work is explicit, bounded, and visible.

