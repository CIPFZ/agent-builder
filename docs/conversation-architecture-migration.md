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
| Plan | `[x]` | `6c58cadd` | Persist architecture, contracts, phases, and acceptance gates; independently reviewed. |
| 1. Contract | `[x]` | `83e62781` | Versioned Go/TypeScript contracts, validation, and shared fixture completed and independently reviewed. |
| 2. Runtime snapshot | `[x]` | `30476cf0` | Persisted-only canonical snapshot, stable semantic identity/order, Wails bridge, recovery tests, and independent review completed. |
| 3. Entity stream | `[x]` | `204608d2` | Persistent atomic entity outbox, materialized snapshot state, recovery cursors, Wails stream, and shadow comparison completed. |
| 4. Frontend store | `[x]` | `cdd76802` | Normalized reducer, revision-safe snapshot merge, pure Turn selector, stable presentation grouping, tests, and independent review completed. |
| 5. Convergence | `[x]` | `(this phase commit)` | Canonical-mode Session stream is the only live writer; refresh, switching, reconnect, and explicit recovery converge by cursor. |
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

### Phase 1: Freeze contract `[x]`

Deliverables:

- Versioned Go DTO draft and TypeScript mirror.
- State machines and identity/ownership/revision/delete rules.
- Full/window scope and cursor semantics.
- Compatibility mapping and cross-language fixtures.

Exit gate: grouping ownership is unambiguous; every structured activity and
late-event scenario is representable; independent contract review passes.

Implementation evidence:

- Go contract: `internal/runtime/runtime_conversation_contract_v2.go`
- TypeScript mirror: `client/src/runtime/canonicalConversationTypes.ts`
- Shared fixture: `internal/runtime/testdata/conversation_contract_v2.json`
- Cursor, activity sequence, and revision use decimal strings so values beyond
  JavaScript's safe integer range remain exact.
- Entity events use typed optional payloads; Go validation enforces exactly one
  matching payload for upserts and no payload for deletes.
- Contract contains no `tool_group`, `quiet`, `defaultExpanded`, `any`, or
  `map[string]any` presentation/transport escape hatch.

Verification:

- `go test ./internal/runtime -run TestCanonicalConversationV2 -count=1`
- `go build ./internal/runtime`
- `cd client && npm run build`
- `cd client && npm run lint`
- `cd client && npm run smoke:conversation-contract-v2`

Independent review notes:

- Corrected window cursors to decimal strings as well as the main cursor.
- Added event/payload identity validation for entity, Session, Turn, and
  revision ownership.
- Full snapshots reject window metadata; window snapshots require it.
- Snapshot validation rejects nil collections so canonical JSON uses arrays,
  never ambiguous `null` values.
- Added required event creation timestamp. Review approved after these changes.

### Phase 2: Runtime canonical snapshot `[x]`

Deliverables:

- Read-only canonical RuntimeService/Wails snapshot.
- Stable semantic order and Message phase.
- No UI tool groups or expansion policy.
- Running, terminal, historical, recovery, and large-Session tests.

Exit gate: process entity IDs are identical before/after final response and on
Session reopen.

Implementation decisions:

- Canonical activity sequence/revision come from the first/latest persisted
  Runtime event affecting an entity. Legacy rows without events use revision
  `0` plus deterministic `(createdAt, entity-rank, id)` ordering; timestamps
  never masquerade as event cursors.
- The canonical full snapshot reads persisted stores only. Active in-memory
  request state cannot make the same cursor return a different snapshot.
- Message phase prefers persisted semantic metadata. The compatibility fallback
  recognizes final only for a finished terminal Turn assistant message without
  tool-use completion; all other assistant messages are intermediate.
- AssistantStep and ToolResult use one shared stable derivation rule for full
  and window snapshots.
- Existing Todo rows cannot satisfy stable identity. Phase 2 therefore adds
  minimum plan/item identity and persists full structured `todo.updated`
  evidence. Legacy Todo data is not presented as canonical until rewritten by
  the identified format; no content hash or array index is treated as a stable
  canonical identity.

Implementation evidence:

- Persisted-only mapper and RuntimeService entry point:
  `internal/runtime/runtime_conversation_snapshot_v2.go`.
- Wails transport-only forwarding: `desktop/runtime_bridge.go`; the TypeScript
  bridge surface mirrors the v2 request/snapshot without becoming a state
  authority.
- Event-index metadata uses the first/latest relevant persisted event as
  `activitySequence`/`revision`, encoded as decimal strings. Unrelated events
  do not mutate entity revisions.
- Tool calls are read by Session rather than by known Turns, preserving
  orphaned/recovery-state persisted calls.
- Final-message resolution requires a terminal Turn, a finished assistant
  message, and no `tool_use`/tool-call part. Persisted snake_case and camelCase
  phase metadata cannot bypass this gate.
- Todo item UUIDs and a stable Session plan ID are persisted; the first rich
  Todo event owns plan creation/Turn identity and the latest event owns its
  revision/update state. Legacy Todo payloads without item IDs are omitted.
- All canonical collections use deterministic ordering before any window is
  selected; window filtering preserves full-snapshot IDs, indexes, and
  revisions.

Verification:

- `go test ./...`
- `go build ./...`
- `cd client && npm.cmd run build`
- `cd client && npm.cmd run lint`
- `cd client && npm.cmd run smoke:conversation-contract-v2`

Focused coverage includes finalization identity stability, persisted-only
same-cursor behavior, full/window equivalence, sequence values above
JavaScript's safe integer range, Todo recovery, orphan ToolCalls, and
byte-identical reconstruction after Runtime service restart.

Independent review notes:

- Initial review blocked submission on final-phase gate bypass, unstable Todo
  creation metadata, incomplete deterministic sorting, and Turn-scoped tool
  loading that could omit recovered calls.
- All four findings were corrected and regression tests were added.
- Follow-up review ran the Go suite and frontend build/lint and approved Phase
  2 with no remaining blocking findings.

### Phase 3: Canonical entity stream `[x]`

Deliverables:

- Revisioned upsert/delete stream.
- Snapshot cursor alignment, gap detection, and snapshot-required behavior.
- Dual-run comparison with the old path behind a version/feature flag.

Exit gate: snapshot `N` plus events after `N` deterministically reconstructs
state; duplicates and out-of-order events are harmless.

Implementation decisions:

- The canonical stream is an independent persisted projector; it does not wrap
  legacy `SessionOutputEvents`, UI grouping, or presentation sequence math.
- Canonical mode commits the raw Runtime event, typed entity outbox, atomic raw
  batch watermark, materialized canonical entity state, and per-Session
  projector checkpoint in one SQLite transaction.
- Snapshots read materialized canonical state at the projector checkpoint.
  They never expose newer semantic rows under an older cursor.
- A semantic write lost before its raw event is recovered by a synthetic
  `conversation.reconciled` event. Recovery advances the cursor and writes the
  same typed outbox/state transaction instead of silently rebasing a cursor.
- Sequence assignment also enters a Runtime-owned pending journal. The
  projector drains all pending events for a Session in numeric order while
  holding its projector lock, so a later publisher cannot commit ahead of and
  discard an earlier sequence.
- A burst containing several already-assigned raw events advances each raw
  watermark atomically and coalesces its semantic state diff onto the final
  drained raw event. This preserves cursor history and final convergence; it
  is not a claim that mutable source tables provide historical row versions.
- Empty semantic events still create a batch watermark. Per-Session global
  sequence gaps are legal and are verified through explicit
  `previous_raw_sequence` links rather than `sequence + 1` arithmetic.
- Message and Session deletion produce explicit tombstones. State diffing also
  upserts affected ToolCalls when ToolResult membership changes; ordinary
  snapshot/window omission is never deletion.
- Stream overflow reliably replaces one buffered item with a single
  `snapshotRequired: true, reason: overflow` control batch and terminates the
  stream.
- Runtime mode is `legacy`, `canonical_v2_shadow`, or `canonical_v2`, selected
  by `AGENT_BUILDER_CONVERSATION_MODE` and defaulting to legacy. Shadow mode
  compares both directions across core semantic entities and exposes
  structured diagnostics without changing the active UI writer.

Implementation evidence:

- Projector and dependency mapping:
  `internal/runtime/runtime_conversation_projector_v2.go`.
- Atomic outbox/checkpoint store and materialized state:
  `internal/runtime/runtime_conversation_event_store_v2.go` and
  `internal/runtime/runtime_conversation_state_store_v2.go`.
- Catch-up and live batch stream:
  `internal/runtime/runtime_conversation_stream_v2.go`.
- Shadow comparison/diagnostics:
  `internal/runtime/runtime_conversation_shadow_v2.go`.
- Schema and Goose migration:
  `internal/db/schema.sql` and
  `internal/db/migrations/20260712000000_add_conversation_entity_stream_v2.sql`.
- Wails V2 snapshot/events/start/stop bridge:
  `desktop/runtime_bridge.go`; TypeScript transport mirrors remain unused by
  the active conversation writer until Phase 4/5.

Verification:

- `go test ./...`
- `go build ./...`
- `cd client && npm.cmd run build`
- `cd client && npm.cmd run lint`
- `cd client && npm.cmd run smoke:conversation-contract-v2`

Focused tests prove snapshot-plus-events convergence, frozen outbox payloads,
raw/outbox/state/checkpoint transaction rollback, atomic multi-entity batches,
empty watermarks, sparse cursors, explicit gaps, large decimal cursors,
observable overflow recovery, Message/Session tombstones, ToolCall dependency
updates, reverse publisher ordering, duplicate/out-of-order revision handling,
same-revision conflict detection, real service restart reconciliation, second
restart byte stability, shadow mismatch diagnostics, schema creation, and
Wails batch preservation/replacement/stop behavior.

Independent review notes:

- Initial review rejected a read-time reinterpretation design and required a
  persistent outbox/checkpoint plus derived revision propagation.
- The first implementation review found non-atomic raw/outbox writes,
  materialized snapshot alignment, overflow, delete dependency, shadow, Wails,
  and migration gaps. These were corrected with transactional materialized
  state and recovery reconciliation.
- The second review found reverse publisher ordering and missing exit-gate
  evidence. The projector now drains a durable in-memory pending journal by
  sequence; controlled reverse-order, revision idempotency, and true restart
  tests were added.
- Third review ran focused Runtime/Desktop tests and approved Phase 3 with no
  remaining blocking findings.

### Phase 4: Frontend normalized store `[x]`

Deliverables:

- Canonical reducer and pure Turn selector.
- One tool-presentation grouping pass.
- Stable React keys/disclosure state.
- Full/window snapshot merge by revision.

Exit gate: tools never disappear when grouped or finalized; long-running tools
remain visible through all updates.

Implementation evidence:

- `canonicalConversationStore.ts` owns nine normalized entity maps, decimal
  cursor/revision handling, tombstones, atomic batch application, and explicit
  recovery states.
- `canonicalConversationSelectors.ts` projects Turns using strict
  `userMessageId`/`finalMessageId` ownership and keeps TodoPlan separate by
  `ownerTurnId`.
- `canonicalConversationPresentation.ts` performs the only presentation
  grouping pass. Tool-group keys do not depend on members or status; all other
  keys use canonical entity type plus ID.
- `canonical-conversation-store-smoke.mjs` covers full/window merge, stale
  snapshots, tombstones, cursor gaps, session mismatch, atomic rollback,
  same-revision conflicts, values above `2^53`, final-arrival stability, Todo
  ownership, and stable grouping keys.
- Legacy session deletion no longer requires a V2 snapshot in legacy mode;
  canonical/shadow modes retain tombstone capture.

Verification:

- `go test ./...`
- `go build ./...`
- `cd client && npm.cmd run build`
- `cd client && npm.cmd run lint`
- `cd client && npm.cmd run smoke:canonical-conversation-store`
- `cd client && npm.cmd run smoke:conversation-contract-v2`

Independent review notes:

- Initial review rejected non-tool keys based only on entity ID and requested
  stronger atomicity, precision, snapshot, and final-arrival evidence.
- Keys now include canonical entity type, and the missing scenarios were added
  to the smoke suite. The second review approved Phase 4 with no remaining
  blocking findings.

### Phase 5: Conversation convergence `[x]`

Deliverables:

- Session stream is the only active conversation writer.
- Workbench refresh cannot replace conversation state.
- Snapshot usage restricted to defined recovery boundaries.
- Remove temporary lagging-snapshot preservation heuristics.

Exit gate: no periodic snapshot/live-event race; switch/reconnect preserves
cursor and entities.

Implementation evidence:

- `canonicalConversationStream.ts` registers the fixed Wails listener before
  starting the V2 stream and forwards each Runtime batch atomically.
- `canonicalConversationCoordinator.ts` owns per-Session cache/cursor,
  generation guards, switch-back catch-up, and snapshot recovery with bounded
  exponential retry. Snapshots are limited to first hydrate and explicit
  gap/conflict/snapshot-required/transport recovery.
- `canonicalConversationMode.ts` gates the writer on explicit Runtime
  diagnostics. Missing diagnostics cannot downgrade a confirmed canonical
  Session; legacy/shadow modes retain the legacy path until cutover.
- `WorkbenchShell.tsx` activates exactly one writer for the selected mode and
  preserves canonical state across unrelated workbench refreshes. The old
  cursor arithmetic and lagging-snapshot reconciliation heuristic are gone.
- `wailsWorkbenchAdapter.ts` no longer reads `SessionOutput` during normal
  hydrate or draft submit. Workspace conversation rendering consumes the
  canonical Turn projection in canonical mode.
- `canonicalConversationView.ts` uses `groupCanonicalProcess` once and emits
  stable tool wrappers; the Timeline presentation pass forwards those wrappers
  without regrouping them.
- Desktop emits an explicit `stream_closed` lifecycle envelope so silent
  transport termination enters the same recovery boundary.

Verification:

- `go test ./...`
- `go build ./...`
- `cd client && npm.cmd run build`
- `cd client && npm.cmd run lint`
- `cd client && npm.cmd run smoke:conversation-output`
- `cd client && npm.cmd run smoke:canonical-conversation-store`
- `cd client && npm.cmd run smoke:canonical-conversation-convergence`
- `cd client && npm.cmd run smoke:conversation-contract-v2`

Independent review notes:

- Initial review found an unconditional canonical-mode switch, a second
  adjacency grouping pass, and permanent disconnect after transient recovery
  failure. Runtime diagnostics gating, the canonical grouping wrapper, and
  generation-aware retry closed these issues.
- Final review found and then verified the fix for a diagnostics-timeout mode
  downgrade race. Phase 5 was approved with no remaining blocking findings.

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
