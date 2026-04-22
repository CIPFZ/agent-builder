# Frontend And Backend Boundary

## Boundary Rule

The backend owns execution semantics. The frontend owns operator interaction.

Do not let UI code become the place where runtime rules live.

## Responsibility Split

| Area | Go Runtime Core | myclawd | React UI |
|---|---|---|---|
| Query turn loop | Owns | Exposes | Consumes |
| Tool invocation | Owns | Exposes | Triggers and renders |
| Tool progress | Emits | Streams | Displays |
| Permissions and approvals | Owns decisions and state | Exposes request/continue/reject | Displays and submits |
| Session persistence | Owns | Exposes | Consumes |
| Subagent lifecycle | Owns | Exposes | Operates and visualizes |
| MCP and skills runtime | Owns | Exposes inventory and invoke flow | Displays and triggers |
| ssh / docker / db tools | Owns | Exposes | Displays and triggers |
| Rich layout and interaction | No | No | Owns |
| Diff viewers / file trees / panels | No | Optional API support | Owns |

## Go Runtime Core Responsibilities

The runtime must remain authoritative for:

- what a session is
- what a tool is
- what a task/subagent is
- what permission mode means
- when approvals are required
- how tool results become transcript state
- how recovery and resume behave

## myclawd Responsibilities

`myclawd` is the product-facing contract layer.

It should standardize:

- request and response schemas
- streaming event schemas
- approval request payloads
- task/subagent status payloads
- runtime inventory payloads
- compatibility between terminal and web clients

`myclawd` should not become a second runtime. It is an adapter and control plane.

## React UI Responsibilities

The UI should focus on:

- interaction flow
- usability
- visibility
- visual grouping
- action affordances

It should not decide:

- whether a tool is safe
- whether a session can resume
- whether a permission update is valid
- whether a task lifecycle transition is legal

## Example Boundary: Approval

Correct split:

- runtime decides approval is required
- `myclawd` streams an approval event
- React UI shows the approval card
- user approves or rejects
- `myclawd` sends continue/reject to runtime
- runtime updates transcript and task state

Incorrect split:

- frontend infers approval logic from tool names or command text

## Example Boundary: Tool Progress

Correct split:

- runtime emits progress events with stable tool identifiers
- `myclawd` forwards them
- frontend renders timeline, badges, expandable details

Incorrect split:

- frontend fabricates progress states from partial message text

## Boundary Enforcement Rules

1. New runtime capability must land in Go first.
2. New operator workflow must consume `myclawd` contracts, not runtime internals.
3. Any state transition used by the UI must already exist in backend contracts.
4. The terminal client and React client should reuse the same backend semantics.

