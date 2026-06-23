# Frontend Runtime UI Technical Plan

Status: active baseline for the Agent Builder frontend rewrite.

This document is the single frontend rewrite reference. It consolidates the
previous Claude Desktop, Codex, cc-haha, and client state recovery notes into
one plan.

## Product Goal

Agent Builder is a desktop coding-agent client for a Go runtime aligned with
Claude Code-style capabilities: sessions, turns, messages, thinking, tools,
permissions, tasks, subagents, skills, MCP, worktrees, audit, recovery, and
budget/context state.

The UI should be conversation-first, project-aware, and runtime-aware. It
should make agent execution state visible and recoverable without becoming a
terminal transcript or a runtime admin console.

## Reference Boundaries

Reference products may inform information architecture and workflow, but not
branding or proprietary visual identity.

- Codex is the primary reference for the coding-agent workbench shape:
  project/session navigation, central conversation workspace, composer runtime
  controls, settings taxonomy, and a right environment inspector.
- Claude Desktop is a reference for restrained conversation-first structure and
  progressive disclosure into secondary workspaces.
- `NanmiCoder/cc-haha` is a runtime feature coverage reference for providers,
  permissions, MCP, agents, skills, memory, usage, diagnostics, and computer
  use setup.
- Ant Design and Ant Design X are open-source UI implementation foundations and
  may define the default visual language.

Agent Builder must not copy proprietary branding, assets, exact layouts, exact
styling, copy, colors, typography, spacing, or animations from reference
products.

## UI Stack Decision

The baseline principle for all frontend work is:

```text
Ant Design and Ant Design X provide common interaction controls and AI
interaction primitives. Agent Builder owns the Codex-style workbench layout and
runtime-specific product components.
```

This is the foundation for the rewrite. Do not replace it with a fully custom
UI system, and do not let default Ant Design composition define the product
shape.

Use this stack for the rewrite:

```text
React
TypeScript
Vite
Ant Design
Ant Design X
TanStack Query
Zustand, only for UI state
CSS Modules plus Ant Design theme tokens
```

The direction is not "Ant Design default admin UI" and not fully custom UI.
Use Ant Design and Ant Design X as the component foundation, then build the
custom Codex-style Agent Builder workbench and runtime-domain components on
top.

Use Ant Design for general application controls:

- `Layout`, `Splitter`, `Flex`
- `Menu`, `Dropdown`, `Tabs`, `Segmented`
- `Form`, `Input`, `Select`, `Switch`, `Radio`
- `Modal`, `Drawer`, `Popover`, `Tooltip`
- `Badge`, `Tag`, `Alert`, `Progress`, `Timeline`
- `Table`, `List`, `Descriptions`, `Collapse`

Use Ant Design X for AI interaction primitives:

- `Conversations`
- `Bubble`, `Bubble.List`
- `Sender`
- `Prompts`, `Suggestion`, `Welcome`
- `ThoughtChain`, `Think`
- `Attachments`, `FileCard`, `Folder`, `Sources`
- `XMarkdown`, `CodeHighlighter`, `Mermaid`

Custom Agent Builder components should map runtime domain objects into these
foundations:

```text
RuntimeTurn              -> TurnTimelineItem
RuntimeToolCall          -> ToolCallCard / ToolCallDetail
RuntimePermissionRequest -> PermissionGate / PermissionReview
RuntimeAgentTask         -> AgentTaskCard / AgentTaskPanel
RuntimeWorktree          -> WorktreePanel
RuntimeAuditEvent        -> AuditDrawer
RuntimeRef               -> RefPreview / ArtifactPanel
RuntimeBudgetReport      -> BudgetStatus
```

Do not introduce Next.js, Tailwind, or a second design system unless a later
explicit architecture decision replaces this plan. Prefer Ant Design icons
while the product uses Ant Design as its main visual system.

## Visual Direction

The target is a Codex-like coding-agent desktop workbench, not a marketing page
or generic SaaS dashboard.

Design rules:

- Keep the main chat surface minimal.
- Put advanced runtime controls in structured management and inspector
  surfaces.
- Favor dense but legible desktop-tool layout.
- Avoid oversized hero sections, decorative card-heavy layouts, and
  explanatory in-app copy.
- Avoid nested cards and page sections styled as floating cards.
- Use restrained borders, compact spacing, and stable panel dimensions.
- Use CSS Modules and Ant Design theme tokens for feature-level layout.
- Avoid expanding global CSS as the main styling mechanism.

## Runtime Integration Rule

The Go runtime remains the only source of truth for:

- sessions
- turns
- messages
- tool calls
- permission requests and decisions
- agent tasks and subagent communication
- worktrees and effective scope
- MCP servers, tools, resources, and prompts
- skills and capabilities
- refs, artifacts, diffs, and compact summaries
- audit records
- recovery state
- model and policy config
- budget and context state

React may own only UI state:

- selected panel, route, tab, drawer, or modal
- sidebar collapsed state
- local filters, sort, and search text
- unsaved form drafts
- composer draft before submit
- transient hover, focus, and expanded state

Frontend code must not infer runtime facts from rendered message text. If a
runtime-backed capability is missing, show an explicit unavailable or blocked
state instead of fabricating frontend-only state.

## Data Fetching And Events

Use TanStack Query for runtime API reads and mutations:

- sessions, messages, turns, tools, tasks, permissions
- model, provider, and policy settings
- MCP, skills, capabilities
- audit, refs, worktrees, recovery, usage, budget

Use the runtime event stream to invalidate or update query data. The event
stream is an incremental notification channel, not the only reconstruction
mechanism. Reload must recover visible state from runtime APIs.

Use Zustand only for product UI state that is not runtime truth.

## App Shell

The first screen must be a usable agent conversation client.

Target shell:

```text
Agent Builder Shell
  Left Sidebar
    New chat
    Search
    Projects / Workspaces
      Sessions
    Skills
    MCP / Connectors
    Automations
    Runtime / Settings

  Center Workspace
    Session header
    Conversation timeline
      User messages
      Assistant messages
      Thinking / reasoning
      Tool calls
      Permission gates
      Todos / plans
      Agent tasks
      Recovery notices
      Refs / artifacts
    Composer

  Right Inspector
    Environment
    Tool details
    Permission details
    Agent task / subagent details
    Worktree and diff details
    Refs / artifacts
    Context and budget
    Audit and replay
    MCP / skills / capability diagnostics
```

The sidebar should be project-first where runtime data supports it. First pass
may use existing session and status APIs while project/workspace APIs evolve.

## Composer

The composer represents a scoped runtime operation, not just a text input.

Target controls:

```text
Prompt input
Attach / add context
Permission policy mode
Model selector
Workspace / project selector
Local / remote mode, when supported
Branch / worktree selector, when supported
Send / cancel
```

Policy modes may include:

- Ask
- Auto edits
- Plan
- Bypass

The UI may expose policy modes, but the Go runtime policy remains the source of
truth. React must not implement local allow/deny logic.

Unsupported runtime-backed controls should be disabled or marked unavailable.

## Timeline

The timeline should show runtime primitives as structured objects rather than
hiding them in assistant message text.

Timeline item types:

- user message
- assistant message
- thinking / reasoning
- tool call
- permission request
- todo / plan
- agent task
- edited files / changed file summary
- diff / review entry
- refs / artifacts
- recovery notice

Ant Design X `Bubble.List` can render message-like items, but runtime-specific
items need custom components.

## Right Runtime Inspector

The right inspector is a core Agent Builder differentiator. It makes the
agent's effective scope and side effects visible while the conversation stays
focused.

Inspector sections:

- cwd / workspace
- branch / worktree
- dirty diff and changed files
- permission mode
- active tools
- active agent task
- sources and context
- auth status
- audit and replay links
- budget / context status
- selected tool, permission, task, ref, or diff detail

## Runtime Management Surfaces

First-version runtime management should cover:

- Providers and models
- Policy
- MCP / connectors
- Skills
- Agent roles and task visibility
- Capabilities
- Diagnostics
- Usage
- Settings

Later surfaces:

- Workspaces / projects
- Artifacts / refs center
- Memory / context editor
- Scheduled tasks
- Terminal setup
- Computer use setup
- Plugins

Plugins should not become the primary runtime tool-management surface before
capability package governance is stable.

## Settings Taxonomy

Separate product settings from runtime settings.

Product settings:

- Appearance
- Language
- Keyboard shortcuts
- Personalization

Runtime settings:

- Config
- Providers and models
- Policy
- MCP
- Hooks
- Git
- Environment
- Worktrees
- Browser / computer control
- Archived sessions

Settings must show the relationship between UI controls and underlying runtime
config. React must not become the policy or config authority.

## Skills

Skills management should combine an overview list with a detailed inspector.

Target data:

- builtin and local skills
- installed and enabled state
- source and path
- trigger / activation metadata
- description
- allowed tools
- diagnostics and errors
- rendered `SKILL.md`
- create skill
- add skill path
- refresh

Implementation candidates:

- `Splitter`
- `Tree`
- `List`
- `Descriptions`
- `Switch`
- `Tabs`
- markdown rendering with Ant Design X

## MCP / Connectors

MCP is a first-version priority because Agent Builder already has runtime MCP
APIs.

Target data:

- server list
- server connection state
- scope: local / project / user
- transport: stdio / HTTP / SSE
- tools, resources, and prompts counts
- pending auth / elicitation
- enable / disable
- refresh / retry
- add / edit server
- tool allowlist

## Agents And Tasks

Agent roles and agent tasks should be modeled separately.

Agent roles:

- role list
- model / provider
- source
- status
- allowed tools
- description
- detail panel

Agent tasks:

- running tasks
- completed tasks
- parent / child session relationship
- cancellation
- result summary
- artifacts

Subagents must not be pretended into independent frontend chats unless the
runtime models them that way.

## Artifacts And Refs

Artifacts should become a runtime evidence/ref center:

- generated files
- diffs
- tool outputs
- screenshots
- reports
- compact summaries
- task results
- refs

First pass:

- list runtime refs when available
- show `artifactRefs`, `diffRefs`, and `outputRefs`
- preview content when `readRefContent` can return content
- otherwise show summary and provenance

## Usage And Diagnostics

Usage target:

- today's tokens
- active session tokens
- 30-day tokens
- cost
- per model/provider usage
- per session usage
- context budget and compaction status

Diagnostics target:

- runtime health
- event stats
- audit stats
- active turns
- pending permissions
- failed tools
- recovery status
- export replay
- export diagnostics bundle
- copy redacted summary
- recent event list

Diagnostics are first-version priority because Agent Builder's runtime is built
around events, audit, replay, and recovery.

## State Recovery

Client recovery is part of the frontend contract. React memory state cannot be
the source of truth for core business state.

Current implemented foundations:

- event sequence / cursor
- `/v1/recovery/status`
- turn / tool / permission / audit persistence foundations
- interrupted turn / task marking
- pending permission recovery
- snapshot-required recovery behavior

Remaining gaps:

- long-term persisted event replay / export
- compact-aware recovery and post-compact reinjection
- richer task / tool / audit diagnostics after refresh
- reducing remaining polling assumptions to short fallback only

Recovery principles:

1. API is the source of truth.
2. Event stream is incremental notification.
3. Client startup must recover complete current visible state without events.
4. Unfinished turns must have explicit status.
5. Pending permissions must be displayable after refresh.
6. Reconnect must not lose critical events.

Startup recovery flow:

```text
load model config
  -> status
  -> sessions
  -> active session messages
  -> active/running turns
  -> pending permissions
  -> capabilities
  -> skills/MCP
  -> recent audit/events
  -> subscribe events with cursor
```

The UI must distinguish:

- loading
- model not configured
- runtime unavailable
- no sessions
- active session loaded
- recovery in progress

## Event Cursor

Runtime events should include:

```text
event_id
sequence
created_at
type
```

Client subscription:

```text
GET /v1/events?after={sequence}
Accept: text/event-stream
```

If `after` is too old, runtime should return snapshot-required semantics and
the client must perform a full state refresh.

SSE reconnect strategy:

1. Reconnect after a delay.
2. Resume with last sequence.
3. If that fails, fetch `GET /v1/events?after=...`.
4. If runtime requires a snapshot, perform full refresh.

Do not rely on long-term 700ms message polling. Polling may exist only as a
short fallback.

## Active Turn And Permission Recovery

Runtime should provide:

```text
GET /v1/turns?status=active
GET /v1/sessions/{session_id}/turns?status=active
GET /v1/permissions
GET /v1/recovery/status
```

After recovery:

- running turn: show active status and cancel
- waiting permission turn: show permission modal or drawer
- interrupted turn: show interrupted status and audit
- no active turn: show normal history

`GET /v1/permissions` must return all pending permissions and not depend on
SSE delivery.

Permission data should include:

- session id
- turn id
- tool call id
- created at
- risk
- status

If the corresponding turn is cancelled or interrupted, runtime should mark the
permission cancelled or expired rather than leaving the UI stuck.

Runtime restart recovery should scan persisted turn state:

```text
runtime_turns where status in queued/running/waiting_permission/cancelling
```

Rules:

- recoverable background tasks continue
- unrecoverable model/tool calls become interrupted
- pending permission with missing tool call becomes expired
- write `turn.interrupted` and `audit.recorded`

## Frontend Project Shape

Target module layout:

```text
client/src/
  app/
    App.tsx
    providers.tsx
    theme.ts
    shell/

  runtime/
    api.ts
    types.ts
    queries.ts
    mutations.ts
    events.ts
    adapters/

  ui/
    antd-wrappers/
    empty-state/
    status/

  features/
    sessions/
    chat/
    composer/
    timeline/
    tools/
    permissions/
    agents/
    worktrees/
    refs/
    audit/
    mcp/
    skills/
    settings/
```

Rules:

- Keep runtime DTOs and API calls out of visual components where practical.
- Build view-model mappers for Ant Design X components.
- Keep feature components grouped by runtime domain.
- Do not let Ant Design X hooks become the runtime state source.

## First Rewrite Milestones

1. Create the new app shell, providers, theme, and runtime query layer.
2. Implement project/session sidebar and active session restore.
3. Implement chat timeline with Ant Design X bubbles and runtime messages.
4. Implement composer with Ant Design X `Sender`, send/cancel, disabled
   states, and model/status/policy affordances.
5. Implement tool, thinking, permission, and task timeline items.
6. Add the right runtime inspector for environment, tool detail, audit, refs,
   tasks, worktrees, and settings entry points.
7. Add MCP, skills, capability, provider, and policy management surfaces.
8. Add recovery, budget/context, diagnostics, and replay/audit polish.

## Acceptance Criteria

- The first screen is a usable agent conversation client.
- The UI follows a Codex-like workbench information architecture without
  copying proprietary visual identity.
- Reload reconstructs visible state from Go runtime APIs.
- Active turns, pending permissions, and interrupted tasks are visible.
- Tools, thinking, permissions, agent tasks, and audit are first-class timeline
  objects.
- The UI can display multi-agent/task state without pretending subagents are
  independent frontend chats.
- Ant Design and Ant Design X provide the default component foundation.
- Custom runtime-domain components provide Agent Builder's product identity.
- Unsupported runtime-backed capabilities are marked unavailable or blocked.
- No runtime state is stored in browser memory as the source of truth.
