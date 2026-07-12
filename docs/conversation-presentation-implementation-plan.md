# Conversation Presentation Implementation Plan

This document is the recoverable implementation plan for the next conversation
presentation pass. It refines presentation only; the canonical Go conversation
store and atomic Wails entity stream remain the sole source of truth.

## Status legend

- `[ ]` pending
- `[~]` in progress
- `[x]` implemented, tested, reviewed, and committed
- `[!]` blocked with evidence recorded below

## Product outcome

The conversation keeps one readable sequence:

```text
User message
Process disclosure (flat, expanded while active, collapsed when safely complete)
Final response

Todo capsule (independent, above the composer)
Agent/Team monitor (persistent summary + right-side details)
```

Users should primarily read the final response, while retaining access to every
tool, result, permission, hook, compact boundary, Subagent, and Agent Team event.
No presentation decision may delete, regroup, or become a second writer for a
canonical entity.

## Non-negotiable design rules

1. **One canonical owner.** `Turn`, `Message`, `ToolCall`, `ToolResult`,
   `Permission`, `TodoPlan`, `AgentTask`, and semantic notices come only from
   the normalized canonical store.
2. **Flat reading edge.** Process summary, tool groups, tool rows, Agent rows,
   and expanded details share one left reading boundary. Hierarchy is expressed
   with typography, surfaces, and disclosure controls, never recursive
   `margin-left` or Collapse content padding.
3. **Todo remains an independent capsule.** It stays in `ConversationDock`, is
   not duplicated in the process timeline, and does not follow process folding.
4. **Process folding is presentation state.** Folding changes visibility only;
   entities stay mounted in the canonical store and late updates remain
   attachable.
5. **Agent dual view, single data.** The timeline shows lifecycle summaries;
   the right panel shows the persistent roster and full details. Both project
   the same canonical `AgentTask` objects.
6. **Stable scrolling.** Auto-collapse, jump controls, Todo, Agent status, and
   late updates must not move the composer or steal the user's reading anchor.
7. **Bounded detail surfaces.** Long tool/file output scrolls locally; Markdown
   tables wrap long cells before falling back to local horizontal scrolling.

## Process disclosure state machine

Per Turn, frontend presentation state is one of:

```text
auto -> manual_open
auto -> manual_closed
manual_open (latched for this Turn)
manual_closed (latched for this Turn)
```

Automatic behavior applies only while the mode is `auto`:

| Turn/process condition | Default presentation |
|---|---|
| running, queued, streaming | expanded |
| waiting for permission/user action | expanded and cannot auto-collapse |
| failed/interrupted/cancelled with unresolved attention | expanded |
| completed with final response | collapsed |
| completed but Agent/Tool still active | expanded |
| terminal with late non-attention updates | retain current disclosure state |

Additional rules:

- A terminal transition may auto-collapse only when the user is pinned near the
  bottom and has not manually opened/closed that Turn.
- If the user is reading earlier content, completion does not collapse content
  under their cursor.
- A late `ToolResult`, Agent message, artifact, or revision never resets a
  manual choice and never briefly reopens a completed process.
- Session switching restores deterministic defaults from canonical status; no
  disclosure state leaks between sessions or Turn ids.

Collapsed summary format:

```text
处理完成 · 8 个工具 · 4 个 Subagent · 42 秒
处理需关注 · 1 个 Agent 失败 · 1 个仍在运行
```

## Surface ownership

### Main process timeline

Shows only high-signal, turn-ordered activity:

- process narration with visible content;
- tool groups and independently expandable tool details;
- permission lifecycle and required action;
- Hook, Compact, and Recovery notices;
- Subagent/Agent Team lifecycle summaries.

It does not show:

- prompt/context assembly inventory;
- TodoPlan rows;
- every Agent progress message or every child tool invocation;
- diagnostic payloads already available in dedicated panels.

### Todo capsule

- Remains above the composer through `ConversationDock`.
- Shows current item and progress while active.
- Opens the full Todo list on click.
- May become a compact completed capsule, but never becomes a timeline row.
- Uses canonical `TodoPlan` ownership by Turn and Session.

### Agent activity in the timeline

Subagent lifecycle uses one stable capsule/row per task. Status updates mutate
that row instead of appending duplicates:

```text
🌸 Runtime Review · 正在检查
🌸 Runtime Review · 已完成
```

Agent Team uses one stable aggregate row per `teamId`:

```text
Agent Team · 3 运行中 · 2 已完成
```

Expanding the aggregate lists members on the same left edge. Only high-signal
events return to the timeline: started, waiting, important finding, failed,
completed. Clicking a task/member opens the right panel and selects that task.

### Persistent Agent/Team monitor

A compact summary is visible near the right-side workspace control whenever
the active session owns Agent tasks:

```text
🌸 ✳ ✺  3 运行中 · 7 完成
```

Clicking it opens the Tasks surface. The right panel provides:

- teams grouped by explicit `teamId` and independent tasks separately;
- role, objective, parent Turn/tool/task, status, and progress;
- bounded Agent message history and child-tool activity;
- artifacts/output references, errors, cancel, and follow-up actions;
- selection synchronized with a clicked timeline Agent capsule.

The right panel remains useful when the main process is collapsed. It must not
copy Agent state into browser-owned models; selection/tab state alone is local.

## Existing implementation inventory

Reusable foundations:

- `canonicalStructuredActivity.ts`: canonical Todo/Permission/AgentTask
  projection and explicit `agentTeams` grouping.
- `ProcessDisclosure.tsx` and `processDisclosurePolicy.ts`: process boundary
  and existing automatic-state vocabulary.
- `TraceRow.tsx`: flat row primitive and independently bounded detail body.
- `TodoTaskBar.tsx`: independent Todo capsule in `ConversationDock`.
- `AgentTaskTimelineRow`: current per-task timeline entry and right-panel link.
- `AgentTaskPanel`, `AgentTaskList`, `AgentTaskDetail`: existing task detail
  surface.
- `Workspace.tsx`: right-panel tabs, selected Agent task, and Timeline wiring.

Gaps to close:

- `ProcessDisclosure` currently renders its body continuously and needs the
  explicit state machine restored without Ant Collapse indentation.
- Agent timeline rows expose too much inline detail and do not aggregate teams.
- The right-side header has no persistent Agent/Team status summary.
- Team/member selection and timeline-to-panel focus need one presentation
  contract.
- Scroll anchoring must be coordinated with auto-collapse.

## Implementation phases

### Phase 0: Baseline fixtures and contracts `[x]`

- Capture current no-tool, multi-tool, failed-tool, Todo, Subagent, and Team
  fixtures.
- Define pure `ProcessDisclosureModel`, `AgentActivitySummary`, and
  `AgentTeamPresentation` types derived from existing view models.
- Add pure tests for counts, attention state, stable ids, and flat ordering.

Exit gate: no visual changes; fixtures demonstrate the current canonical input
for every later phase.

Implementation evidence:

- `conversationPresentationFixtures.ts` captures no-tool, multi-tool,
  failed-tool, Todo, Subagent, and explicit Team canonical projections.
- `conversationPresentationModel.ts` defines pure `ProcessDisclosureModel`,
  `AgentActivitySummary`, and `AgentTeamPresentation` contracts over existing
  canonical Turn and Agent task view models.
- Stable presentation ids derive only from canonical Turn, Session, Team, and
  entity ids. Process ordering delegates to the existing single canonical
  grouping pass; Team member order preserves the canonical task projection.
- Todo remains only on the Turn's independent `todoPlan` field and is absent
  from process presentation items.

Verification:

- `cd client && npm.cmd run smoke:conversation-presentation-model`
- `cd client && npm.cmd run lint`
- `cd client && npm.cmd run build`
- `cd client && npm.cmd run smoke:canonical-structured-activity`
- `cd client && npm.cmd run smoke:canonical-conversation-store`

Independent review notes:

- Review removed optional-field inference for Agent identity and made Agent
  counts consume the existing `AgentTaskViewModel` projection explicitly.
- Review removed timestamp re-sorting from Team presentation so the pure
  contract cannot become a second canonical order authority.
- Final diff review confirmed no JSX, CSS, disclosure behavior, Todo dock, or
  Agent panel changes; Phase 0 is presentation-contract-only.

### Phase 1: Flat outer process disclosure `[x]`

- Implement the per-Turn `auto/manual_open/manual_closed` state machine.
- Add one flat summary header around `processStream`; do not use nested Ant
  Collapse padding.
- Keep running/permission/attention states open and safely collapse completed
  processes.
- Preserve manual choices across late entity revisions.
- Coordinate terminal auto-collapse with sticky-bottom state and reading anchor.

Exit gate: completion does not jump the viewport; expanding tool groups/details
does not add left indentation; Todo remains unchanged.

Implementation evidence:

- `processDisclosurePolicy.ts` now owns the pure per-Turn
  `auto/manual_open/manual_closed` reducer and its one-shot safe-completion
  transition.
- `ProcessDisclosure.tsx` uses an accessible native disclosure button and a
  retained process body controlled by `hidden`; the outer boundary does not
  use Ant Collapse or its content padding.
- `Workspace` forwards the existing sticky-bottom `pinned` intent through
  `Timeline`. Safe completion auto-collapses only on its first transition
  while pinned; an unpinned reader consumes that transition without layout
  movement, so later repinning cannot collapse older content unexpectedly.
- Running, streaming, queued, permission-waiting, terminal-without-final, and
  failed/interrupted attention remain open in automatic mode. Manual choices
  latch for the Turn and ignore late entity revisions.
- The disclosure chevron is positioned outside the content edge, leaving the
  summary label, process stream, groups, and rows on one flat reading boundary.

Verification:

- `cd client && npm.cmd run smoke:process-disclosure-policy`
- `cd client && npm.cmd run smoke:conversation-presentation-model`
- `cd client && npm.cmd run lint`
- `cd client && npm.cmd run build`
- `cd client && npm.cmd run smoke:canonical-conversation-store`
- `cd client && npm.cmd run smoke:canonical-conversation-convergence`
- `cd client && npm.cmd run smoke:canonical-structured-activity`
- `cd client && npm.cmd run smoke:canonical-conversation-cutover`
- `cd client && npm.cmd run smoke:conversation-contract-v2`

Independent review notes:

- Review added the missing terminal-without-final gate so a Turn cannot fold
  before the final response exists.
- Review verified canonical tool-group status propagates active/failure member
  state and that late non-attention revisions do not replay auto-collapse.
- Final scope review found no Todo capsule, Agent aggregation, Agent panel, or
  canonical store/Wails writer changes.

### Phase 2: Agent timeline projection `[x]`

- Reduce individual Agent rows to compact lifecycle capsules.
- Aggregate explicit Team members into one stable Team row.
- Keep failed/waiting tasks visible even when other process content collapses.
- Open/select the corresponding right-panel task from every Agent capsule.
- Do not render routine Agent messages or child tools as duplicate timeline
  rows; expose them in details.

Exit gate: one task has one stable timeline identity; one Team has one stable
aggregate identity; updates never append duplicates.

Implementation evidence:

- `agentTimelineProjection.ts` performs one pure projection over the existing
  canonical process rows. Independent tasks retain `agentTask:<taskId>` and
  explicit Teams reuse the Phase 0 `agent-team:<teamId>` contract.
- A Team is emitted at its first canonical member position; later members are
  collected into that row in canonical activity order. Status revisions
  update the same task/Team ids and never append duplicate rows.
- Individual Subagents now render as compact lifecycle capsules containing
  only title and status. Prompt, progress, model, messages, outputs, and
  artifacts remain in the existing right-side task detail.
- Team capsules expose running/completed/waiting/failed counts and expand to
  compact member capsules on the same flat reading edge. Waiting and failed
  states keep the Phase 1 process boundary open.
- Every independent/member capsule forwards its canonical task id through the
  existing `openAgentTask` path, which selects the task and opens the existing
  Tasks panel.

Verification:

- `cd client && npm.cmd run smoke:agent-timeline-projection`
- `cd client && npm.cmd run smoke:conversation-presentation-model`
- `cd client && npm.cmd run smoke:process-disclosure-policy`
- `cd client && npm.cmd run smoke:canonical-conversation-store`
- `cd client && npm.cmd run smoke:canonical-conversation-convergence`
- `cd client && npm.cmd run smoke:canonical-structured-activity`
- `cd client && npm.cmd run smoke:canonical-conversation-cutover`
- `cd client && npm.cmd run smoke:conversation-contract-v2`
- `cd client && npm.cmd run lint`
- `cd client && npm.cmd run build`

Independent review notes:

- Review replaced a locally derived Team identity with the frozen Phase 0
  `AgentTeamPresentation` identity and ordering contract.
- Review added explicit waiting/failed Team counts so attention remains visible
  without copying detail-panel content into the timeline.
- Final scope review confirmed no persistent Agent monitor, Tasks panel
  restructuring, Todo change, or canonical store/Wails writer change.

### Phase 3: Persistent Agent/Team monitor `[x]`

- Add a compact Agent status summary beside the right workspace control.
- Open the Tasks tab from the summary and preserve selected task/team.
- Group the Tasks panel into Agent Teams and independent tasks.
- Keep full messages, tools, outputs, and actions in the detail surface with
  bounded scrolling.

Exit gate: users can monitor active Agents while the process is collapsed and
can navigate timeline -> task detail -> team without state divergence.

Implementation evidence:

- `AgentActivityMonitor` renders a compact active/completed/attention summary
  beside the existing right-workspace control whenever the active Session has
  canonical Agent tasks. It remains visible independently of process folding.
- Clicking the monitor opens the existing Tasks tab without changing
  `selectedAgentTaskID`; timeline task/member clicks still select through the
  same `openAgentTask` path before opening that tab.
- `agentTaskPanelProjection.ts` derives the monitor summary, explicit
  `teamId` groups, and independent tasks from the same canonical
  `AgentTaskViewModel[]`. It reuses the Phase 0 Team presentation contract and
  preserves canonical member order and object references.
- The Tasks list now renders Agent Team sections and an independent-task
  section. A selected timeline member stays highlighted inside its Team and
  drives the existing detail surface.
- Task details retain the complete canonical bounded message window, related
  child-tool ids, output/artifact references, and existing cancel/follow-up
  actions. The detail column has a bounded local vertical scroll surface.

Verification:

- `cd client && npm.cmd run smoke:agent-monitor-panel`
- `cd client && npm.cmd run smoke:agent-timeline-projection`
- `cd client && npm.cmd run smoke:conversation-presentation-model`
- `cd client && npm.cmd run smoke:process-disclosure-policy`
- `cd client && npm.cmd run smoke:canonical-conversation-store`
- `cd client && npm.cmd run smoke:canonical-conversation-convergence`
- `cd client && npm.cmd run smoke:canonical-structured-activity`
- `cd client && npm.cmd run smoke:canonical-conversation-cutover`
- `cd client && npm.cmd run smoke:conversation-contract-v2`
- `cd client && npm.cmd run lint`
- `cd client && npm.cmd run build`

Independent review notes:

- Review verified that the new projection retains canonical task references
  and owns no lifecycle state, preventing monitor/timeline/panel divergence.
- Review verified monitor open does not reset task selection and that Team
  membership is derived only from explicit `teamId`.
- Final scope review confirmed no Todo dock change, Phase 4 cross-surface
  polish, canonical store mutation, or Wails writer change.

### Phase 4: Todo and cross-surface polish `[ ]`

- Preserve the Todo capsule as an independent dock action.
- Verify Todo/Permission/Agent floating controls do not resize the composer.
- Normalize flat alignment, status language, icons, responsive behavior, and
  attention colors.
- Ensure completed Todo presentation is compact without duplicating history.

Exit gate: no nested indentation and no composer flash across all combinations.

### Phase 5: End-to-end verification `[ ]`

- Automated matrix: active -> completed, manual open/closed, failed,
  permission, Todo, Subagent, Team, late ToolResult, late Agent message,
  session A -> B -> A, reload, restart, historical, and compact/recovery.
- Real packaged Wails checks for running process, terminal auto-collapse,
  timeline-to-Agent-panel navigation, Todo independence, and scroll anchoring.
- Independent implementation review and architecture-document update.

Exit gate: build/lint/smokes/Go tests and packaged scenarios pass; each phase is
committed independently before the next begins.

## Recommended starting point

Begin with **Phase 0**, then **Phase 1 only**. The first implementation change
should be a pure disclosure reducer/model, not JSX state scattered across
`Timeline` and `ProcessDisclosure`. Wire it to one Turn after its transition and
anchor tests pass. Agent/Team work begins only after the outer process boundary
is stable; otherwise two interacting disclosure systems will obscure scroll and
late-update regressions.

## Required review discipline

- Mark exactly one phase `[~]` while implementing.
- Implement, test, independently review, update this document, and commit that
  phase before starting another.
- Do not change canonical Runtime ownership or add a second conversation writer
  as part of presentation work.
- Do not preserve a hidden legacy presentation path as a fallback.
