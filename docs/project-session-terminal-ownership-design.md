# Project / Session / Terminal Ownership Design

Status: design gate. Do not implement runtime or UI changes from this document
until the contract tests in this document are accepted.

## Problem

The product needs a stable enterprise-grade relationship between projects,
conversations, and integrated terminals.

Current behavior is incomplete:

- A runtime workspace already represents the current working directory, but the
  product does not yet expose a clear project ownership model around it.
- Sessions are persisted under the current workspace, but terminal ownership is
  not tied to sessions.
- Terminals are runtime-owned processes, but the visible terminal tabs are
  currently arranged by React local state.
- `CreateTerminal` accepts a `cwd` and size only; it does not require a
  `sessionId` or prove that the terminal belongs to the active conversation.
- Switching conversations cannot reliably restore the correct terminal list
  from runtime DTOs.

This makes the UI feel like project, conversation, and terminal are separate
features instead of one coherent work surface.

## Product Model

The first-version model should be strict:

```text
Project
  = one working directory
  = file/review/git/worktree scope
  = owns many sessions

Session
  = one conversation inside one project
  = owns turns, messages, permissions, diagnostics, run projections
  = owns many live terminals

Terminal
  = one PTY/ConPTY-backed shell process
  = belongs to exactly one session
  = starts in the session project's working directory
  = may change shell cwd independently after startup
```

Rules:

- Project is not a chat. It is the workspace boundary.
- Session is the conversation boundary.
- Terminal is a live process boundary.
- A terminal belongs to a session, not to a React tab and not only to a project.
- File and review tools are project-scoped. They may be visible only when the
  active surface is project-aware.
- Terminal is conversation-scoped. Every session can have multiple terminals.
- Shell `cd` changes affect only that terminal process. They must not mutate
  project path or session working directory.

## Authority Rules

Go runtime is authoritative for:

- current project/workspace identity and path;
- session lifecycle and active session selection;
- terminal lifecycle, process handles, PTY/ConPTY state, shell profile, status,
  dimensions, and bounded replay history;
- session-to-terminal ownership;
- cleanup on session deletion, runtime restart, and app shutdown.

React is authoritative only for:

- panel open/closed state;
- focused tab id for the currently hydrated session;
- transient layout measurements.

React must not:

- infer terminal ownership from visible tabs;
- keep a terminal alive after runtime deleted it;
- create terminal state from browser memory after reload;
- restore terminals from xterm text, event payloads, assistant prose, or action
  metadata.

Wails and HTTP remain adapters. Both transports must expose the same DTO
contract.

## Runtime DTO Contract

Extend terminal DTOs so ownership is explicit:

```go
type RuntimeTerminal struct {
    ID          string `json:"id"`
    ProjectID   string `json:"projectId"`
    SessionID   string `json:"sessionId"`
    Title       string `json:"title,omitempty"`
    CWD         string `json:"cwd"`
    InitialCWD  string `json:"initialCwd,omitempty"`
    Shell       string `json:"shell"`
    ShellPath   string `json:"shellPath,omitempty"`
    ShellArgs   []string `json:"shellArgs,omitempty"`
    Columns     int    `json:"columns,omitempty"`
    Rows        int    `json:"rows,omitempty"`
    Status      string `json:"status"`
    ExitCode    *int   `json:"exitCode,omitempty"`
    CreatedAt   int64  `json:"createdAt"`
    UpdatedAt   int64  `json:"updatedAt"`
}

type RuntimeTerminalCreateRequest struct {
    SessionID string `json:"sessionId"`
    ID        string `json:"id,omitempty"`
    CWD       string `json:"cwd,omitempty"`
    ProfileID string `json:"profileId,omitempty"`
    ShellPath string `json:"shellPath,omitempty"`
    ShellArgs []string `json:"shellArgs,omitempty"`
    Columns   int    `json:"columns,omitempty"`
    Rows      int    `json:"rows,omitempty"`
}

type RuntimeSessionTerminalsResponse struct {
    SessionID string            `json:"sessionId"`
    Terminals []RuntimeTerminal `json:"terminals"`
}
```

Notes:

- `sessionId` is required for terminal creation.
- `projectId` is derived from the owning session/workspace, not accepted as
  user input in the create request.
- `cwd` is optional. If omitted, runtime uses the session project's working
  directory.
- If `cwd` is provided, runtime must normalize it and decide whether it must be
  inside the project. The recommended first-version rule is: allow only the
  project directory or descendants unless a later explicit terminal profile
  permission gate accepts broader paths.
- `InitialCWD` records startup cwd. `CWD` remains runtime-known startup cwd
  unless a future shell integration can report live cwd safely. Do not infer
  live cwd from prompt text.

## Runtime Storage

First version should keep live terminal process state in memory only.

Recommended runtime structures:

```go
terminalsByID map[string]*runtimeTerminalState
terminalIDsBySession map[string]map[string]struct{}
```

Each `runtimeTerminalState` should include:

- `ProjectID`
- `SessionID`
- `InitialCWD`
- current DTO fields
- PTY/ConPTY handle
- shell process
- bounded replay events
- bounded subscriber set

Do not persist live terminal process state in the database for this phase.
Persisting a terminal record without the shell process would create misleading
restore semantics. After app restart, no live terminal should appear unless a
future explicit terminal persistence design is accepted.

## API Surface

HTTP:

```text
GET    /v1/sessions/{session_id}/terminals
POST   /v1/terminals
GET    /v1/terminals/{terminal_id}/stream?after=N
DELETE /v1/terminals/{terminal_id}
```

Wails bridge:

```text
SessionTerminals(sessionID string) RuntimeSessionTerminalsResponse
CreateTerminal(req RuntimeTerminalCreateRequest) RuntimeTerminalResponse
DeleteTerminal(terminalID string) RuntimeTerminalResponse
```

Behavior:

- `GET /v1/sessions/{id}/terminals` returns only terminals owned by that
  session.
- `POST /v1/terminals` rejects an empty or unknown `sessionId`.
- `POST /v1/terminals` rejects `cwd` outside the session project boundary unless
  later policy explicitly allows it.
- `DELETE /v1/terminals/{id}` is idempotent only if the product chooses that
  behavior in contract tests. Recommended first-version behavior: deleting a
  missing terminal returns not found so stale frontend state is visible during
  development.
- Terminal WebSocket stream remains terminal-scoped. The stream must verify the
  terminal id exists; it does not select or change sessions.

## Frontend Contract

The frontend adapter should expose:

```ts
listSessionTerminals(sessionID: string): Promise<TerminalViewModel[]>
createTerminal(request: {
  sessionId: string
  cwd?: string
  columns?: number
  rows?: number
}): Promise<TerminalViewModel>
```

Workspace behavior:

- Selecting a session hydrates session activity and session terminal list from
  runtime DTOs.
- Switching sessions replaces visible terminal tabs with the selected session's
  runtime terminal DTOs.
- Switching away from a session does not close that session's terminals.
- Closing a terminal tab calls runtime delete. The UI removes the tab only after
  runtime confirms deletion or after a durable reread shows it is gone.
- Refresh/reload reads terminals from runtime. Browser memory is not a restore
  source.
- If the active session has no terminals and the right panel is open, show the
  empty launcher. Do not create a terminal until the user requests one.

## Project Tools In Session View

There are two coherent product choices:

Option A:

- Project overview page shows files and review.
- Session page shows terminals only.
- This keeps conversation focused but makes file/review feel detached from the
  active conversation.

Option B:

- Any session that belongs to a project may show terminal, files, and review.
- File/review remain project-scoped and runtime-read-only where applicable.
- Terminal tabs remain session-scoped.

Recommended direction: Option B.

Reason: the user mental model is "a project has many sessions; each session has
its own terminals." If a session belongs to a project, hiding project tools from
that session makes the work surface less coherent. The important authority rule
is not hiding tools; it is keeping ownership clear:

- files/review read project/worktree state;
- terminal reads session terminal state;
- conversation reads `SessionActivity` / `RunProjection`.

If Option B is accepted, the earlier rule "conversation only terminal" should
be replaced with: "non-project draft chat only has terminal; project-bound
sessions may show project tools."

## Lifecycle Semantics

### Create Session

- New project session starts with no terminals.
- First terminal is created only on user action.
- Runtime records the session id on the terminal.

### Select Session

- Runtime active session changes through `SelectSession`.
- Frontend rereads:
  - session list;
  - `SessionActivity` or accepted narrow session activity;
  - `RunProjection` when needed;
  - `SessionTerminals(sessionID)`.
- Terminals for other sessions keep running but are not shown.

### Close Terminal Tab

- UI calls `DeleteTerminal(terminalID)`.
- Runtime kills the PTY process and removes it from both terminal maps.
- Runtime publishes final terminal event to subscribers.
- Frontend rereads the session terminal list or removes the tab after confirmed
  success.

### Delete Session

- Runtime closes all terminals owned by that session before or during session
  deletion.
- Terminal close events should be final and bounded.
- A deleted session must never leave orphan PTY processes.

### Runtime Restart / App Shutdown

- All live terminals are closed.
- No terminal DTOs are restored after restart.
- Terminal replay history is not durable across restart.

### WebSocket Disconnect

- Disconnect detaches the renderer only.
- The shell process keeps running while the terminal exists.
- Reconnect uses `after=N` and bounded replay.
- If the reconnect cursor is older than retained replay, the frontend may miss
  old display bytes; the shell process remains authoritative.

## Performance And Resource Requirements

The target is stable enterprise use with low memory and CPU overhead.

Runtime:

- Keep terminal reads scoped by session, not global.
- Keep replay history byte-bounded per terminal.
- Keep subscriber queues bounded.
- Batch output over WebSocket.
- Apply backpressure by waiting for xterm acknowledgements.
- Remove stale subscribers instead of growing queues.
- Limit max terminals per session and globally. Proposed defaults:
  - max 8 running terminals per session;
  - max 32 running terminals globally;
  - limits should be constants/configurable later, not React-only checks.
- Normalize terminal dimensions and reject pathological values.
- Close all child processes on session delete, terminal delete, runtime stop,
  and app shutdown.

Frontend:

- Use xterm.js for rendering; do not render terminal output in React.
- Serialize `terminal.write` calls.
- Acknowledge output only after xterm write callback.
- Bound xterm scrollback.
- Debounce resize.
- Do not keep full terminal output in component state.
- Do not store terminal output in the workbench view model.

Large output:

- `>10 MB` output must stream without unbounded memory growth.
- Runtime replay is a recent-history buffer, not a durable log.
- Durable capture of large command output should be implemented through file
  artifacts or explicit logging later, not terminal scrollback.

Long-running commands:

- Long-running shell commands must survive panel close/open and WebSocket
  reconnect.
- They do not survive terminal deletion, session deletion, or runtime shutdown.

Interactive commands:

- Full-screen and interactive commands are supported through PTY/xterm byte
  flow.
- React must not intercept command input except to send bytes to the stream.
- Keyboard shortcuts reserved by the app must be reviewed carefully so they do
  not break terminal-native interactions.

## Security And Isolation

- Terminal creation must verify the session exists under the current project.
- Terminal reads by session must not leak terminals from other sessions.
- Terminal stream attach must require a valid runtime API token like the rest of
  the HTTP runtime API.
- Shell path/profile override should remain runtime-validated. Do not trust a
  frontend-provided shell path blindly in enterprise mode.
- Do not log terminal input/output by default. Terminal text may contain
  secrets.
- Do not put terminal output in screenshots, diagnostics, or runtime events
  unless a later redaction design accepts it.
- File/review tools must not infer state from terminal output.

## Contract Tests Before Implementation

Runtime tests:

- Creating a terminal without `sessionId` fails.
- Creating a terminal for an unknown session fails.
- Creating a terminal for session A returns `SessionID=sessionA` and the current
  project id.
- Listing session A terminals excludes session B terminals.
- Selecting session B does not close session A terminals.
- Deleting session A closes all session A terminals and leaves session B
  terminals running.
- Runtime shutdown closes all terminals.
- Replacing a terminal id updates both `terminalsByID` and
  `terminalIDsBySession` without leaving stale ownership entries.
- `cwd` defaults to the project path.
- `cwd` outside the project path is rejected if the first-version project
  boundary rule is accepted.

HTTP tests:

- `GET /v1/sessions/{id}/terminals` returns the scoped list.
- `POST /v1/terminals` requires `sessionId`.
- `DELETE /v1/terminals/{id}` removes the terminal from the session list.
- WebSocket stream for a deleted terminal returns a terminal final/error path
  rather than hanging.
- Dev-module fallback exposes the same terminal list/create/delete contract if
  the frontend needs it in browser development.

Wails bridge tests:

- `SessionTerminals` forwards to runtime service.
- `CreateTerminal` preserves `sessionId`.
- `DeleteTerminal` preserves runtime deletion semantics.

Frontend adapter tests:

- `createTerminal` sends active `sessionId`.
- Session switch rereads `SessionTerminals`.
- Adapter does not merge terminal state from runtime events or action metadata.
- Deleting a terminal triggers runtime delete and reread/confirmed removal.

Browser smoke:

- Create two project sessions.
- Create two terminals in session A and one in session B.
- Switch between sessions and verify each session shows only its own terminals.
- Run a long command in session A, switch to B, return to A, and verify output
  continued.
- Close one tab and verify it stays removed after refresh.
- Delete a session and verify its terminals disappear and processes are gone.

## Implementation Phases

### Phase 1: Runtime Contract Gate

- Add DTO fields and `RuntimeSessionTerminalsResponse`.
- Add runtime tests for ownership and lifecycle.
- Do not wire UI yet.

### Phase 2: Runtime Ownership Implementation

- Add `SessionID`, `ProjectID`, `InitialCWD`, timestamps to terminal state.
- Add `terminalIDsBySession`.
- Validate sessions before terminal creation.
- Close terminals on session deletion.
- Add session-scoped terminal list helper.

### Phase 3: Transport

- Add HTTP `GET /v1/sessions/{id}/terminals`.
- Add Wails bridge `SessionTerminals`.
- Keep terminal WebSocket protocol unchanged except for ownership validation.
- Remove old unused terminal compatibility code found during implementation.

### Phase 4: Frontend Adapter

- Add `listSessionTerminals`.
- Require `sessionId` in `createTerminal`.
- Map `projectId` and `sessionId` into terminal view models.
- Keep terminal output out of `WorkbenchViewModel`.

### Phase 5: Workspace UI

- On session selection, hydrate terminal tabs from runtime.
- On create terminal, pass active session id.
- On close terminal, delete in runtime and reread.
- Apply accepted project-tool visibility rule.

### Phase 6: Stress And Lifecycle Validation

- Run runtime tests.
- Run client build.
- Browser-smoke multi-session/multi-terminal behavior.
- Stress test large output, reconnect, session switch, and deletion cleanup.

## Open Product Decision

One decision should be confirmed before implementation:

Should project-bound sessions show file/review tools in the right panel?

Recommended answer: yes.

Then the final rule becomes:

- project-bound session: terminal + file + review;
- project overview: file + review, and terminal only if there is an active
  session to own it;
- non-project draft chat: terminal only after a runtime session exists.

If the product instead wants file/review only on a separate project overview
page, terminal ownership still remains session-scoped.
