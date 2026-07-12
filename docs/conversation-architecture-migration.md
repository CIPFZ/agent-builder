# Conversation Architecture Migration

This document is the recoverable source of truth for rebuilding the Agent
Builder conversation-output path. Update its status, evidence, review notes,
and commit reference after every phase.

## Status

- `[ ]` pending
- `[~]` in progress
- `[x]` completed, reviewed, tested, and committed
- `[!]` blocked, with the blocker recorded in this document

| Phase | Status | Commit | Result |
|---|---|---|---|
| Baseline | `[x]` | `50377c48` | Preserve current UI and convergence mitigations before migration. |
| Plan | `[x]` | recorded by this commit | Persist architecture, contracts, phases, and acceptance gates; independently reviewed. |
| 1. Contract | `[ ]` | — | Freeze canonical snapshot and entity-event contracts. |
| 2. Runtime snapshot | `[ ]` | — | Add a semantic snapshot without UI grouping. |
| 3. Entity stream | `[ ]` | — | Add revisioned canonical upsert/delete events. |
| 4. Frontend store | `[ ]` | — | Normalize entities and group for presentation exactly once. |
| 5. Convergence | `[ ]` | — | Make the Session stream the only live conversation writer. |
| 6. Structured activity | `[ ]` | — | Migrate Todo, Permission, Subagent, and Agent Team. |
| 7. Cutover | `[ ]` | — | Remove the old projection and verify end to end. |

## Why this migration exists

The current path projects the same facts repeatedly:

```text
Agent/domain entities
  -> persisted runtime activity
  -> RuntimeConversationItem projection
  -> projected snapshot and projected events
  -> frontend entity + item store
  -> Turn projection
  -> frontend tool grouping
  -> Timeline UI
```

This creates multiple authorities for identity, grouping, order, lifecycle,
and visibility. A live event can show a running tool, a lagging/windowed
snapshot can remove it, and a later snapshot can restore it. A single
`tool_call` item can also be replaced by a different `tool_group` item when a
second groupable call appears.

Known contract conflicts:

- Runtime groups tools and React groups/compacts them again.
- Runtime `tool_group` carries both `toolCallId` and `toolCallIds`; the client
  selects the singular ID first and can render only the first call.
- Snapshot absence may mean window truncation, persistence lag, projection
  replacement, or deletion; scope is not declared.
- Wails live events and periodic workbench snapshots both write the same
  conversation store.
- `RuntimeConversationItem` mixes semantic references with UI policy such as
  titles, quiet grouping, counts, and default expansion.
- Projected event sequence uses raw sequence arithmetic and the client derives
  a cursor by dividing it, coupling transport to presentation offsets.

The baseline `preserveUnconfirmedLiveTurnOutput` mitigation is temporary. It
reduces flicker but cannot establish deterministic convergence.

## Product target

Every user input creates one stable Turn:

```text
Turn
|- User message
|- Process activity
|  |- reasoning/intermediate assistant output
|  |- ToolCalls and ToolResults
|  |- Permissions
|  |- Todo updates
|  |- Subagent / Agent Team activity
|  `- context, hook, compact, and recovery notices
`- Final assistant message
```

Finalizing a Turn changes its terminal status and final-message ownership. It
must not rebuild or remove process activity.

## Ownership boundary

Runtime owns semantic truth:

- Session, Turn, Message, AssistantStep, ToolCall, ToolResult, Permission,
  TodoPlan, AgentTask, HookExecution, and semantic notices.
- Stable identity, ownership, phase, lifecycle, order, revision, timestamps,
  and durable references.
- The distinction between reasoning, intermediate output, and final response.

React owns presentation policy:

- Visual tool grouping and quiet-tool compaction.
- Labels, icons, disclosures, spacing, responsive layout, and bounded previews.

Wails owns transport only. It must not reproduce persistence, lifecycle, or
grouping rules.

## Required invariants

### Identity and ownership

- Canonical IDs never change from creation through terminal status.
- Visual grouping never creates or replaces canonical IDs.
- Every Turn-owned entity carries `sessionId` and `turnId`.
- ToolResult and Permission carry `toolCallId`.
- TodoPlan carries `ownerTurnId`.
- AgentTask carries `parentTurnId` and `parentToolCallId`, with
  `parentTaskId/teamId` available for hierarchy.

### Order and revision

- Each entity receives an immutable `activitySequence` when created.
- Each update increments `revision`; timestamps are not used as the primary
  ordering key.
- An update cannot move an entity within a Turn.

### Monotonic state

- Terminal states cannot regress.
- Observed content and refs cannot disappear through an ordinary upsert.
- Removal requires an explicit delete/tombstone event.
- A window snapshot missing an entity is not a deletion.

### Completion and recovery

- Final response updates do not change process IDs, tool membership/results,
  TodoPlan, Permissions, or AgentTasks.
- Reloading a historical Session reproduces the final live semantic structure.

### Snapshot/event convergence

For cursor `N`:

```text
snapshot(N) + ordered events(sequence > N) = current canonical state
```

- Snapshots declare `scope: full | window` and `schemaVersion`.
- Snapshot entities carry revision; older snapshots cannot overwrite newer
  event state.
- Cursor is a raw Runtime event cursor, not presentation-sequence arithmetic.

## Target contracts

Exact Go and TypeScript names are frozen in Phase 1. The semantic shape is:

```text
SessionConversationSnapshot
  schemaVersion
  sessionId
  cursor
  scope (full | window)
  window metadata (when windowed)
  turns[]
  messages[]
  assistantSteps[]
  toolCalls[]
  toolResults[]
  permissions[]
  todoPlans[]
  agentTasks[]
  notices[]
```

Every canonical entity includes:

```text
id, sessionId, turnId?, activitySequence, revision, createdAt, updatedAt
```

Entity events use one envelope:

```text
ConversationEntityEvent
  id, sessionId, sequence
  entityType, entityId
  operation (upsert | delete)
  revision
  entity? / tombstoneReason?
```

Important entity fields:

- Turn: `userMessageId`, explicit `finalMessageId`, status and terminal detail.
- Message: `phase = reasoning | intermediate | final`, optional step ID, parts,
  content, and status.
- ToolCall: step/message/parent refs, kind/source/name, status, semantic input,
  command, targets/cwd/risk, result IDs, timestamps. No `quiet`,
  `defaultExpanded`, preformatted group title, or UI group entity.
- ToolResult: toolCall ID, ordinal, status, bounded previews, durable output /
  artifact / diff refs, and delivery-to-model state.
- Permission: Turn and ToolCall ownership, request/decision state, policy data,
  and decision timestamp.
- TodoPlan: stable plan ID, owner Turn, revision, plan lifecycle, and Todo items
  with stable IDs, order, status, content, and active form.
- AgentTask: parent Turn/ToolCall/Task, team ID and role, status, progress,
  dependencies, messages, and result/output refs.

## Transport lifecycle

1. Register the stable Wails listener.
2. Request snapshot and receive cursor `N`.
3. Start the stream after `N`.
4. Apply only events with sequence greater than `N`.
5. Snapshot again only for initialization, Session switching, reconnect,
   cursor gap, or `snapshot_required`.

Periodic workbench refresh may update settings, diagnostics, context usage,
hooks, and task detail. It must not replace the active conversation store.

## Frontend projection

```text
canonical normalized store
  -> Turn semantic selector
  -> one presentation-grouping pass
  -> Timeline
```

A visual tool-group key derives from stable semantic ownership, such as
`turnId + assistantStepId/roundId + kind`. Adding a member does not replace an
existing ToolCall.

## Structured activity

### Todo

Todo is a structured plan, not a synthetic timeline summary. The Todo tool call
remains a ToolCall; TodoPlan drives the composer capsule. Plan replacement,
clear, item reorder, duplicate updates, unfinished terminal Turns, and restart
must have explicit semantics. A terminal Turn with unfinished items becomes
stopped/abandoned rather than continuing to spin.

### Subagent and Agent Team

AgentTask canonical parent/team relationships drive both the compact timeline
activity and the right task surface. The contract must support nested and
concurrent tasks, dynamic membership, partial success, cancellation
propagation, follow-ups, late messages/results, and deleted child Sessions.

### Permission

Permission state is independently recoverable from ToolCall status. Cover
restart while waiting, allow-session, deny/cancel/expire, decision-before-tool
update, and multiple permission requests for one tool.

### Long-running commands

A running command keeps one ID and remains visible with a bounded single-line
summary. Canonical timestamps are authoritative; any live elapsed timer is
local UI state. Cover long periods without output, high-frequency stdout,
interactive PTY, cancellation, exit code, late output, and large durable refs.

## Phases

### Phase 1: Freeze contract `[ ]`

Deliverables:

- Versioned Go DTO draft and TypeScript mirror.
- State machines and identity/ownership/revision/delete rules.
- Full/window scope and cursor semantics.
- Compatibility mapping and cross-language fixtures.

Exit gate: grouping ownership is unambiguous; every structured activity and
late-event scenario is representable; independent contract review passes.

### Phase 2: Runtime canonical snapshot `[ ]`

Deliverables:

- Read-only canonical RuntimeService/Wails snapshot.
- Stable semantic order and Message phase.
- No UI tool groups or expansion policy.
- Running, terminal, historical, recovery, and large-Session tests.

Exit gate: process entity IDs are identical before/after final response and on
Session reopen.

### Phase 3: Canonical entity stream `[ ]`

Deliverables:

- Revisioned upsert/delete stream.
- Snapshot cursor alignment, gap detection, and snapshot-required behavior.
- Dual-run comparison with the old path behind a version/feature flag.

Exit gate: snapshot `N` plus events after `N` deterministically reconstructs
state; duplicates and out-of-order events are harmless.

### Phase 4: Frontend normalized store `[ ]`

Deliverables:

- Canonical reducer and pure Turn selector.
- One tool-presentation grouping pass.
- Stable React keys/disclosure state.
- Full/window snapshot merge by revision.

Exit gate: tools never disappear when grouped or finalized; long-running tools
remain visible through all updates.

### Phase 5: Conversation convergence `[ ]`

Deliverables:

- Session stream is the only active conversation writer.
- Workbench refresh cannot replace conversation state.
- Snapshot usage restricted to defined recovery boundaries.
- Remove temporary lagging-snapshot preservation heuristics.

Exit gate: no periodic snapshot/live-event race; switch/reconnect preserves
cursor and entities.

### Phase 6: Structured activity `[ ]`

Deliverables:

- TodoPlan capsule.
- Permission migration.
- Subagent and Agent Team projections over canonical AgentTask.
- Hook/context/compact/recovery notice mapping.
- Remove duplicate synthetic summaries.

Exit gate: ownership is never inferred from a windowed event; timeline and
detail surfaces consume the same canonical entities.

### Phase 7: Cutover `[ ]`

Deliverables:

- Remove old RuntimeConversationItem grouping path and duplicate adapters.
- Update general architecture docs and delete obsolete tests.
- Full automated and packaged-Wails hands-on verification.

Exit gate: no-tool, multi-tool, failed-tool, permission, Todo, Subagent,
Agent Team, long-command, late result, reconnect, restart, historical,
compact, and recovery scenarios pass.

## Cross-phase acceptance scenarios

- Tool lifecycle: `queued -> running -> output -> completed -> final`; stable
  ID/order and continued visibility.
- Grouping: A never disappears when B joins; group key remains stable and both
  calls remain inspectable.
- Race: snapshot cursor 10, tool event 11, lagging window snapshot 10, complete
  event 12; cursor-10 data cannot delete or regress event-11 state.
- Late data: final response may precede final ToolResult/artifact persistence;
  late upsert attaches without rebuilding process history.
- Restart: waiting permission, active TodoPlan, running/cancelled AgentTask, and
  completed process history recover consistently.

## Phase completion procedure

1. Mark exactly one phase `[~]`.
2. Implement only its declared scope.
3. Run phase-specific and regression tests.
4. Perform independent review; use a Subagent when useful.
5. Record review notes and exact evidence here.
6. Mark `[x]` only after the exit gate passes.
7. Commit implementation and documentation together.
8. Add the commit hash to the table before beginning the next phase. A small
   hash-recording follow-up may be included at the start of the next phase.

Do not begin a later phase while an earlier phase is incomplete or blocked.
