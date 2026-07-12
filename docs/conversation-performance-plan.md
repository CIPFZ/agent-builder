# Conversation Performance and Incremental Loading Plan

## Status

- `[x]` Investigation and root-cause confirmation
- `[x]` Phase 1: bounded conversation loading and rendering
- `[x]` Phase 2: indexed projections and event allocation reduction
- `[x]` Phase 3: durable large-content references and conversation search
- `[ ]` Phase 4: profiling gates and long-session regression coverage

## Problem statement

The current desktop product requests a full canonical conversation snapshot,
keeps every visited Session in an unbounded browser cache, and mounts every
Turn and collapsed process detail in the WebView DOM. A live measurement on
2026-07-13 showed roughly 136 MB private memory in the Go process and 3.9-4.7
GB in the Agent Builder WebView renderer. The primary capacity problem is
therefore the browser-owned full-history projection and render path.

The canonical Go store remains the full recoverable source of truth. React
must only retain a bounded working set that can be reconstructed from Runtime.

## Target interaction model

1. Project/session navigation loads Session summaries only.
2. Opening a Session shows a loading state and requests its newest 30 Turns.
3. Scrolling near the top requests the preceding 30 Turns using the earliest
   loaded Turn as the `before` boundary.
4. Historical pages merge by canonical identity and revision without changing
   the live stream cursor.
5. Switching Session closes the old stream and retains at most a small bounded
   cache of recent Session windows.
6. Collapsed process details are not mounted. A later virtualization phase
   limits mounted Turn DOM even after many historical pages have been loaded.
7. Search and indexed navigation query Go/SQLite and load a window around the
   target Turn; they never require a full browser-side conversation scan.

## Invariants

- Go Runtime remains authoritative for entity identity, revision, ordering,
  persistence, and recovery.
- Wails remains the only frontend/runtime transport.
- The newest-window snapshot cursor establishes the live event cursor.
- Loading older history must not move or replace the live event cursor.
- A window omission is never interpreted as deletion.
- Duplicate pages and out-of-order entity revisions are idempotent.
- Prepending history preserves the visible scroll anchor.
- Session cache and mounted DOM have explicit capacity bounds.

## Phase 1: bounded loading and rendering

### Runtime and transport

- Reuse `SessionConversationSnapshotV2` with `scope: window`.
- Initial request: `{ scope: "window", limit: 30 }`.
- Historical request: `{ scope: "window", limit: 30, before: earliestTurnId }`.
- Keep the existing canonical live stream after the newest snapshot cursor.

### Frontend coordinator and store

- Extend the snapshot adapter to accept window request options.
- Add coordinator state/actions for initial loading and loading earlier pages.
- Merge historical pages into the normalized store without regressing its live
  cursor or replacing the newest-window pagination boundary.
- Track the earliest loaded boundary and `hasMoreBefore` separately from the
  live cursor.
- Replace the unbounded Session Map with an LRU cache capped at two Sessions.
- Add explicit eviction for deleted Sessions and project/draft transitions.

### Rendering

- Trigger earlier-page loading near the scroll container top.
- Preserve `scrollHeight - scrollTop` across a prepend.
- Render a lightweight loading row above the Timeline.
- Do not mount completed collapsed process details.
- Memoize canonical projections by store identity.

### Phase 1 acceptance

- Opening a historical Session does not request `scope: full`.
- Initial normalized store contains at most 30 Turns plus live additions.
- Reaching the top loads the next page once and preserves scroll position.
- Switching among more than two Sessions evicts older canonical stores.
- Collapsed completed Turns contain no Markdown/tool detail DOM.
- Canonical convergence and structured-activity tests continue to pass.

## Phase 2: projection and event efficiency

- Maintain entity IDs indexed by Turn so selectors are O(entities in Turn),
  not O(Turns x all entities).
- Introduce revision-aware memoized selectors for Turn and structured views.
- Copy only entity dictionaries touched by a canonical event batch.
- Coalesce high-frequency streaming message updates to a frame or bounded
  50-100 ms interval.
- Add Turn-level render memoization and Timeline virtualization.

Implementation progress:

- Canonical store maintains revision-safe `Turn -> entity key` indexes during
  snapshot hydration, historical merge, event upsert, and tombstone deletion.
- Turn selection now visits only indexed entities owned by that Turn instead
  of concatenating every entity dictionary once per Turn.
- Live event batches clone only the entity dictionaries whose kinds occur in
  that batch; untouched dictionaries preserve referential identity.
- Workspace canonical projections are memoized by normalized store identity.
- Turn projections expose semantic revision keys. Timeline memoizes Turn
  wrappers by those keys so an update in one Turn does not rerender unrelated
  Turn subtrees.
- Completed Turns outside a 1,000 px viewport margin release their rendered
  Markdown/tool subtree and retain a measured-height placeholder. Active
  Turns remain mounted regardless of viewport position.
- Runtime already bounds persisted `message.updated` amplification with a
  250 ms coalescing window while keeping advisory text deltas outside the
  canonical persisted cursor. Remaining Phase 2 work is profiler-backed
  tuning of virtualization margins and placeholder behavior.

## Phase 3: content and navigation

- Remove duplicated full `Message.Content` / `PartsJSON` payloads.
- Persist large message/tool content behind durable references and transport
  bounded previews by default.
- Parse Markdown only for mounted visible/expanded messages.
- Add SQLite-backed conversation search returning Turn/Message IDs, snippets,
  and timestamps.
- Add a Runtime query that loads a bounded window around a target Turn.

Implementation progress:

- Removed canonical `Message.partsJson`. The field had no React consumer and
  duplicated persisted structured parts, including potentially large tool and
  binary payloads, alongside canonical Message text and ToolResult entities.
  Persisted message parts and model-context assembly remain unchanged.
- Existing ToolResult transport remains bounded to a 4 KiB preview and durable
  output references. General Runtime refs already spill payloads larger than
  8 KiB to file storage and expose an explicit content-read operation.
- Canonical Message text is capped at 64 KiB per entity with byte length and
  truncation metadata. Truncated Messages expose an explicit Wails-only read
  of the full persisted content, scoped by both Session ID and Message ID;
  Runtime rejects cross-Session reads. The Timeline loads this content only
  when requested and then uses it for rendering and copy actions.
- Session search now queries the existing SQLite FTS5 message index and
  returns at most 50 bounded result summaries with canonical Message/Turn IDs.
  Selecting a result loads a 30-Turn window centered around the target,
  preserves the live cursor/store, scrolls the Turn into view, and highlights
  it temporarily. Search and navigation are Wails-only.

## Phase 4: measurement and regression gates

Create deterministic fixtures for 100, 1,000, and 10,000 Turns and record:

- Go private bytes and heap profile;
- WebView renderer private bytes and JS heap snapshot;
- mounted DOM node count;
- initial Session-open latency;
- prepend latency and scroll-anchor error;
- allocations and render count per live event.

Initial targets:

- idle desktop total private memory below 700 MB;
- 1,000-Turn Session open below 900 MB renderer private memory;
- leaving a large Session returns to baseline plus 150 MB after GC;
- live output reaches a stable heap plateau;
- one entity update does not scan or reproject unrelated Turns.

## Rollout and rollback

- Keep full snapshots available as a diagnostic/recovery API, not the normal
  product read path.
- Land Phase 1 behind the existing canonical V2 contract without adding a
  parallel conversation authority.
- Each phase must preserve snapshot-plus-events convergence tests and can be
  reverted independently at the frontend window/coordinator boundary.
