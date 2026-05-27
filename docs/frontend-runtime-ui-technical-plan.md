# Frontend Runtime UI Technical Plan

Status: active baseline for the next client rewrite.

This document defines the frontend technology choices, product rules, and
runtime integration goals for the next Agent Builder UI. The new UI starts from
a clean React implementation while preserving the existing Go runtime as the
source of truth.

## Product Goal

Agent Builder is a desktop agent client for a Go runtime aligned with Claude
Code style capabilities: sessions, turns, messages, thinking, tools,
permissions, tasks, subagents, skills, MCP, worktrees, audit, recovery, and
budget/context state.

The UI should be conversation-first and runtime-aware. It should make agent
execution state visible and recoverable without turning the client into a
terminal transcript or a backend admin console.

## Reference Boundaries

Reference products may inform information architecture and workflow, but not
branding or proprietary visual identity.

- Claude Desktop is a reference for high-level product structure: session
  navigation, central conversation workspace, composer, contextual details, and
  progressive disclosure.
- `NanmiCoder/cc-haha` may be used as a feature and workflow reference only.
  Do not copy its source code, proprietary assets, exact layouts, or visual
  styling.
- Ant Design and Ant Design X are open-source UI implementation foundations and
  may define the default visual language for this product.

The product should not clone Claude branding, logos, exact colors, typography,
copy, spacing, or animations.

The current Claude Desktop screenshot analysis is archived in
`docs/claude-desktop-ui-analysis.md`.

The current cc-haha screenshot analysis is archived in
`docs/cc-haha-ui-analysis.md`.

The current Codex screenshot analysis is archived in
`docs/codex-ui-analysis.md`.

## Technology Decision

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

Notes:

- Keep Wails as the desktop shell adapter.
- Keep the runtime API abstraction under `client/src/runtime`.
- Do not introduce Next.js or a server-rendered frontend framework.
- Do not introduce Tailwind as a second design system unless a later explicit
  architecture decision replaces this plan.
- Prefer Ant Design icons while the product uses Ant Design as its main visual
  system. Revisit icon choice only if the design language changes.

## Component Responsibilities

Ant Design X should own AI interaction primitives:

| UI need | Preferred foundation |
| --- | --- |
| Conversation list | `Conversations` |
| Chat messages | `Bubble`, `Bubble.List` |
| Composer | `Sender` |
| Suggested prompts | `Prompts`, `Suggestion`, `Welcome` |
| Thinking and reasoning | `ThoughtChain`, `Think` |
| Attachments and files | `Attachments`, `FileCard`, `Folder`, `Sources` |
| Markdown/code/diagrams | `XMarkdown`, `CodeHighlighter`, `Mermaid` |

Ant Design should own general application controls:

| UI need | Preferred foundation |
| --- | --- |
| App shell and panels | `Layout`, `Splitter`, `Flex` |
| Navigation and menus | `Menu`, `Dropdown`, `Tabs`, `Segmented` |
| Forms and settings | `Form`, `Input`, `Select`, `Switch`, `Radio` |
| Dialogs and overlays | `Modal`, `Drawer`, `Popover`, `Tooltip` |
| Status and metadata | `Badge`, `Tag`, `Alert`, `Progress`, `Timeline` |
| Dense data views | `Table`, `Descriptions`, `Collapse` |

Custom Agent Builder components should map runtime domain objects into the UI
components above. Examples:

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

- selected panel or drawer
- sidebar collapsed state
- local filters
- unsaved form drafts
- composer draft before submit
- transient hover/focus/expanded state

Frontend code must not infer runtime facts from rendered message text. If a
runtime-backed capability is missing, show an explicit blocked or unavailable
state instead of fabricating UI state.

## Data Fetching And Events

Use TanStack Query for runtime API reads and mutations:

- sessions, messages, turns, tools, tasks, permissions
- model/policy settings
- MCP, skills, capabilities
- audit, refs, worktrees, recovery

Use the runtime event stream to invalidate or update query data. The event
stream should not become the only reconstruction mechanism; reload must recover
from runtime APIs.

Use Zustand only for product UI state that is not runtime truth.

## Main Information Architecture

```text
Agent Builder Shell
  Left Navigation
    Workspaces/projects when runtime supports them
    Sessions
    Search
    New chat
    Runtime feature entries

  Center Workspace
    Session header
    Runtime status
    Conversation timeline
      Messages
      Thinking
      Tool calls
      Permissions
      Todos/plans
      Agent tasks
      Recovery notices
    Composer

  Right Inspector
    Tool details
    Permission details
    Agent task/subagent details
    Worktree and diff details
    Refs/artifacts
    Context and budget
    Audit and replay
    MCP/skills/capability diagnostics

  Settings Surfaces
    Model/provider config
    Policy mode
    MCP server config
    Skill paths and creation
```

## Runtime Feature Coverage

Ant Design X is sufficient for the primary AI interaction surface, but Agent
Builder still needs runtime-specific domain components.

| Runtime feature | UI strategy |
| --- | --- |
| Single-agent chat | Ant Design X `Bubble.List` plus runtime message mapping |
| Thinking/reasoning | Ant Design X thinking components plus runtime part mapping |
| Tool calls | Ant Design X thought chain plus custom `ToolCallCard` |
| Permission review | Custom Ant Design modal/drawer and timeline item |
| Multi-agent/subagents | Custom task cards and right-side task panel |
| MCP and skills | Ant Design management panels |
| Worktrees and diffs | Custom inspector panels using Ant Design layout |
| Audit/replay | Custom drawer with runtime records and summaries |
| Recovery | Runtime-backed notices and pending-state surfaces |
| Budget/context | Runtime-backed status and inspector panels |

## Frontend Project Shape

Target module layout for the rewrite:

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
- Avoid a global CSS file as the main styling mechanism. Use Ant Design theme
  tokens and CSS Modules for feature-level layout.
- Do not let Ant Design X hooks become the runtime state source. They may be
  used only as presentation helpers around runtime-owned state.

## First Rewrite Milestones

1. Create the new app shell, providers, theme, and runtime query layer.
2. Implement sessions plus active session restore.
3. Implement chat timeline with Ant Design X bubbles and runtime messages.
4. Implement composer with Ant Design X `Sender`, send/cancel, disabled states,
   and model/status affordances.
5. Implement tool/thinking/permission timeline items.
6. Add the right inspector for tool detail, audit, refs, tasks, worktrees, and
   settings entry points.
7. Add MCP, skills, capability, and policy management surfaces.
8. Add recovery, budget/context, and replay/audit polish.

## Acceptance Criteria

- The first screen is a usable agent conversation client.
- Reload reconstructs visible state from Go runtime APIs.
- Active turns, pending permissions, and interrupted tasks are visible.
- Tools, thinking, permissions, agent tasks, and audit are first-class timeline
  objects, not hidden debug output.
- The UI can display multi-agent/task state without pretending subagents are
  independent frontend chats.
- Ant Design and Ant Design X provide the default visual language.
- Claude and other clients are used only for product structure reference.
- Unsupported runtime-backed capabilities are marked unavailable or blocked.
- No runtime state is stored in browser memory as the source of truth.
