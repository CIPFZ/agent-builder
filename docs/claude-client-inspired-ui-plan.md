# Claude-Client-Inspired UI Plan

This document plans the next React client phase for Agent Builder. It uses the
Claude web/desktop chat client as a product-experience reference for
information architecture, layout, interaction flow, status presentation, and
detail surfaces.

It does not use Claude Code CLI/TUI as a UI reference. Claude Code remains a
runtime primitive reference only.

## Reference Method

Current reference state:

- No Claude client screenshot or live desktop observation was available in this
  planning session.
- This plan is therefore based on user-provided direction, existing product
  knowledge of modern Claude-style conversation clients, current Agent Builder
  runtime/client docs, and a light code inventory of the current React surfaces.
- Assumption to validate later: the Claude client reference should emphasize a
  conversation-first shell, compact left navigation, central chat timeline,
  fixed composer, contextual right-side detail surface, clear empty states,
  message actions, artifact/task detail panels, model/status controls, and
  settings behind explicit entry points.

When screenshots or live observation are available, update this document with
the concrete UI patterns extracted from them. Record patterns only. Do not copy
Claude branding, logo, trademarked names, proprietary visual assets, or exact
visual styling.

## Fixed Principles

- `charm.land/fantasy` must not be modified.
- Runtime / agent execution primitives reference Claude Code.
- React UI / interaction experience references the Claude web/desktop client.
- Do not restore TUI/CLI, terminal UI, Ink layout, keybindings, or slash command
  UI as the product path.
- Claude-client-inspired UI is a product experience reference, not a brand or
  visual clone.
- Do not copy Claude logos, marks, trademarks, brand colors, copy, or
  proprietary assets.
- Go runtime is the source of truth for sessions, turns, messages, tools,
  permissions, tasks, capabilities, settings, events, audit, recovery, context,
  and compact state.
- React only owns UI state: route, selected item, drawer open/closed, local
  form drafts, composer draft before submit, and panel filters.
- UI must not invent business facts before runtime contracts exist.
- Missing runtime-backed UI must be labeled `Blocked by runtime API`.
- Model-assisted permission can only be advisory; it cannot approve actions.
- Compact is runtime lifecycle, not a frontend action.
- Tool search/deadlock avoidance is scheduler/runtime responsibility.
- Agent communication/coordinator is runtime primitive, not UI event stitching.
- Provider/model config is a policy/config layer above fantasy, not a second
  provider abstraction.

## Current Runtime Support

The current codebase already supports enough runtime state for a meaningful
Claude-client-inspired shell:

| UI need | Runtime support |
| --- | --- |
| Session list and active session | `GET /v1/sessions`, session/message APIs, `client/src/features/chat/ChatSidebar.tsx` |
| Conversation timeline | runtime messages, turns, tool calls, permissions, audit/task summaries |
| Send/cancel turn | `POST /v1/sessions/{session_id}/turns`, `POST /v1/turns/{turn_id}/cancel` |
| Permission review | `GET /v1/permissions`, decision API, `PermissionReviewModal` |
| Model/settings entry | `runtime_model*.go`, `ModelSettingsDrawer.tsx` |
| Skills/MCP/capabilities | runtime skills/MCP/capability APIs and panels |
| Audit/detail view | turn/session audit APIs and `RuntimeAuditDrawer` |
| Recovery/resume | `/v1/recovery/status`, active turns, pending permissions, event cursor |
| Task summaries | `RuntimeAgentTask` APIs and timeline item |

## Blocked By Runtime API

These UI areas should be designed as placeholders or blocked states until Go
runtime contracts exist:

| UI area | Blocker |
| --- | --- |
| Compact/context budget warning | Compact boundary, budget accounting, `budget.updated`, compact APIs |
| Compact history/detail | `runtime_compact_boundaries` APIs and audit events |
| Tool search/discovery detail | Tool search API, selected tool audit, scheduler deadlock events |
| Task/subagent communication panel | Task message/mailbox/result/artifact APIs |
| Artifact/detail drawer beyond summaries | Durable artifact refs and content APIs |
| Scoped policy rule editor | Policy scope model, precedence diagnostics, rule APIs |
| Worktree/sandbox/remote controls | Isolation lifecycle APIs and cleanup states |
| Replay/export diagnostics | Persisted event replay/export APIs |
| Advanced context/memory/source view | Context source provenance, read-file reinjection, compact source replay |
| Advisory permission explanation | Advisory-only classifier result fields and audit |

## UI Patterns To Borrow

Because no screenshot was available, these are assumptions to validate against
the user-provided Claude client running interface:

- Navigation: left rail/sidebar with session/project list, new chat, search,
  and compact access to settings/capability areas.
- Main layout: central conversation timeline with a stable composer at the
  bottom and status/model affordances near the header or composer.
- Timeline: messages remain the primary object; tool/task/permission/audit
  items appear as structured cards that can expand or open details.
- Empty states: first-run state guides model setup; no-session state guides
  session creation; runtime-error state points to diagnostics.
- Message actions: per-message affordances should be compact and contextual
  such as copy, inspect, retry later, open audit, or open artifacts when
  runtime APIs exist.
- Composer: supports text draft, submit, disabled state, active-turn cancel,
  attachment entry point, model/status display, and clear blocked states for
  unavailable APIs.
- Detail area: right drawer/panel handles tool details, artifacts, task
  progress, audit/debug, context/source visibility, and settings without
  crowding the main timeline.
- Permission review: modal or drawer is interruptive enough for risky actions,
  shows runtime risk/reason/target summary, and posts decisions back to runtime.
- Recovery: resumed/interrupted/pending state is visible in the timeline and
  header, not hidden in console logs.

## Information Architecture

```text
App Shell
  Left Sidebar
    New chat
    Search
    Sessions / projects
    Runtime feature entries
      Capabilities
      Skills
      MCP
      Diagnostics
      Settings

  Center Workspace
    Header
      Session title
      Model/status
      Active turn / recovery state
    Timeline
      User/assistant messages
      Turn status
      Tool cards
      Permission cards
      Plan/todo status
      Task/subagent status
      Recovery/compact/status notices
    Composer
      Text draft
      Attachment affordance
      Model/status affordance
      Send/cancel

  Right Drawer / Modal
    Permission review
    Tool detail
    Artifact/detail
    Task/subagent progress
    Audit/debug
    Context/source visibility
    Settings/model/provider
    MCP/skills/capability diagnostics
```

## Module Boundaries

### Claude-Client-Inspired App Shell

| Field | Plan |
| --- | --- |
| Goal | Make the first screen a polished conversation-first desktop client shell with stable left navigation, central chat, and right detail surfaces. |
| Non-goals | No terminal UI, no Claude branding clone, no React-owned runtime state, no business feature implementation. |
| Go packages | None required for first pass; consumes existing `internal/runtime` APIs. |
| React packages | `client/src/app/AssistantClient.tsx`, `client/src/app/App.css`, `client/src/features/chat/*`, `client/src/features/capabilities/*`, settings/audit/permission drawers. |
| Runtime API / event schema | Existing session/message/turn/status/recovery/events APIs. |
| Data model changes | None. |
| Tests | Client type/build, smoke check for loading sessions, opening drawers, switching feature views. |
| Acceptance | Reload recovers from runtime APIs; shell does not synthesize messages/tasks/permissions; first screen is chat, not settings. |
| Risks | Treating frontend reducers as truth, overloading navigation with diagnostics. |
| Blocked by / Unlocks | Not blocked; unlocks clearer UI targets for runtime APIs. |

### Session / Sidebar / Project Navigation

| Field | Plan |
| --- | --- |
| Goal | Align left navigation with Claude-client-style session/project browsing, new chat, search, resume, and runtime feature entry points. |
| Non-goals | No local-only session cache as truth, no slash-command navigation. |
| Go packages | Existing session APIs. Future project/workspace APIs if needed. |
| React packages | `ChatSidebar.tsx`, `useAssistantClient.tsx`. |
| Runtime API / event schema | Existing sessions; future project/workspace summaries if runtime defines them. |
| Data model changes | None now. Project metadata later only if runtime owns it. |
| Tests | Session selection/resume smoke; empty/no-model/no-session states. |
| Acceptance | Sidebar state survives refresh by reloading runtime sessions; feature entries do not imply unsupported runtime features. |
| Risks | Confusing project with session before runtime project model exists. |
| Blocked by / Unlocks | Project grouping blocked by runtime project/workspace API. |

### Chat Timeline

| Field | Plan |
| --- | --- |
| Goal | Present messages, turns, tools, permissions, tasks, audit summaries, plan/todo state, and recovery notices as a readable central timeline. |
| Non-goals | No message-text parsing to infer runtime state; no CLI transcript mimicry. |
| Go packages | Existing message/turn/tool/permission/task/audit APIs. |
| React packages | `ChatWorkspace.tsx`, `TimelineItems.tsx`, `chatTimeline.ts`, `MessageItem.tsx`. |
| Runtime API / event schema | Existing `message.*`, `turn.*`, `tool.call.*`, `permission.*`, `task.*`, `audit.recorded`. Compact/budget later. |
| Data model changes | None in UI. |
| Tests | Timeline ordering and status rendering; refresh from API after events. |
| Acceptance | Timeline can be reconstructed after reload; cards link to runtime detail APIs; unsupported details show blocked state. |
| Risks | Timeline noise from audit events; duplicated task/tool cards. |
| Blocked by / Unlocks | Compact warning, task messaging, and artifacts blocked by runtime APIs. |

### Composer

| Field | Plan |
| --- | --- |
| Goal | Provide Claude-client-style bottom composer with text draft, send/cancel, model/status affordance, attachment entry, and disabled states. |
| Non-goals | No slash command UI, no frontend prompt assembly, no frontend attachment ingestion without runtime attachment contract. |
| Go packages | Existing turn creation/cancel APIs; future attachment APIs. |
| React packages | `Composer.tsx`, `ChatWorkspace.tsx`. |
| Runtime API / event schema | Existing send/cancel; future attachment upload/source APIs. |
| Data model changes | None now. |
| Tests | Send disabled/enabled states, active turn cancel, model-not-configured state. |
| Acceptance | Composer only submits runtime turn requests; attachments are blocked until runtime supports them. |
| Risks | Hidden draft loss, implying unavailable attachment support. |
| Blocked by / Unlocks | Attachment/source chips blocked by runtime attachment/source API. |

### Permission Review

| Field | Plan |
| --- | --- |
| Goal | Make permission review feel like a native client safety flow with clear risk, target, policy reason, allow-once/session, deny, and cancel turn. |
| Non-goals | No UI risk classification, no model self-approval, no secret exposure. |
| Go packages | `internal/permission`, `internal/runtime/runtime_permissions.go`, `runtime_policy.go`. |
| React packages | `PermissionReviewModal.tsx`, timeline permission cards. |
| Runtime API / event schema | Existing permission list/decision events. Future advisor fields are advisory only. |
| Data model changes | None now; future scoped policy/advisor fields from runtime. |
| Tests | Pending permission recovery, decision submit, secret redaction display. |
| Acceptance | UI displays runtime-provided risk/reason/target summary and submits a decision; no local allow logic. |
| Risks | Overexposing params, making advisory text look authoritative. |
| Blocked by / Unlocks | Scoped rule editor/advisor blocked by runtime policy APIs. |

### Tool Cards / Progress / Detail

| Field | Plan |
| --- | --- |
| Goal | Show tools as structured cards with status, source, capability, short output, errors, progress, and open-detail action. |
| Non-goals | No direct tool execution from React, no model-facing output edits in UI. |
| Go packages | Existing ToolCall store/scheduler; future output/artifact refs. |
| React packages | `TimelineItems.tsx`, future detail drawer. |
| Runtime API / event schema | Existing tool call APIs/events; future output refs and `scheduler.deadlock.prevented`. |
| Data model changes | None in UI. |
| Tests | Tool status rendering, failed/cancelled states, open audit/detail. |
| Acceptance | Tool cards read runtime summaries and details; large output/artifact unavailable states are explicit. |
| Risks | Overcrowded timeline, leaking raw output. |
| Blocked by / Unlocks | Durable output refs/artifact refs blocked by runtime. |

### Task / Subagent Panel

| Field | Plan |
| --- | --- |
| Goal | Provide a Claude-client-style side panel for task/subagent progress, parent/child status, cancellation, and later messages/artifacts. |
| Non-goals | No UI-built agent communication, no swarm UI before coordinator runtime. |
| Go packages | Existing `runtime_agent_tasks.go`; future task messages/artifacts. |
| React packages | Timeline task item and future task drawer/panel. |
| Runtime API / event schema | Existing task summary/cancel; future `task.message.created`, `task.result.updated`, `task.artifact.created`. |
| Data model changes | None in UI. |
| Tests | Task list/detail/cancel and recovery states. |
| Acceptance | Panel reflects runtime task status only; messaging/artifacts show blocked until APIs exist. |
| Risks | Presenting child agent state as independent chat without runtime contract. |
| Blocked by / Unlocks | Full panel blocked by AgentTask communication APIs. |

### Artifact / Detail Drawer

| Field | Plan |
| --- | --- |
| Goal | Use a right drawer for artifacts, tool outputs, diffs, generated files, context/source details, and audit-linked evidence. |
| Non-goals | No local filesystem scanning as artifact truth, no proprietary Claude artifact clone. |
| Go packages | Future artifact/output ref APIs. |
| React packages | New drawer routing layered with `RuntimeAuditDrawer` patterns. |
| Runtime API / event schema | Future artifact refs, output refs, compact summary refs. |
| Data model changes | None in UI. |
| Tests | Drawer routing and blocked states first; content tests once APIs exist. |
| Acceptance | Drawer opens from timeline cards and shows runtime-backed refs or a blocked-by-runtime message. |
| Risks | Creating fake artifact state from message text. |
| Blocked by / Unlocks | Blocked by artifact/output ref runtime APIs. |

### Settings / Model / Provider Config

| Field | Plan |
| --- | --- |
| Goal | Keep settings discoverable but secondary: model provider, base URL, API key, verify, model discovery, policy mode, MCP/skills links. |
| Non-goals | No provider client rewrite, no fantasy changes, no default settings homepage. |
| Go packages | `runtime_model*.go`, config, policy. |
| React packages | `ModelSettingsDrawer.tsx`. |
| Runtime API / event schema | Existing model config/policy APIs; future model health/capability events. |
| Data model changes | Future model capability cache only if runtime owns it. |
| Tests | Save/verify/redaction/policy mode display. |
| Acceptance | Secrets remain redacted; config persists in runtime; settings does not own provider behavior. |
| Risks | Duplicating fantasy logic, leaking credentials. |
| Blocked by / Unlocks | Provider health/capability display partly blocked by runtime diagnostics. |

### MCP / Skills / Capability Diagnostics

| Field | Plan |
| --- | --- |
| Goal | Keep capability management accessible from the sidebar and drawers while chat remains primary. |
| Non-goals | No marketplace-first plugin UI, no frontend skill prompt injection. |
| Go packages | Existing skills/MCP/capabilities; future scoped activation/package governance. |
| React packages | `RuntimeFeatureWorkspace.tsx`, `RuntimeSkillPanel.tsx`, `RuntimeMcpPanel.tsx`, `RuntimeCapabilityPanel.tsx`. |
| Runtime API / event schema | Existing capability/skill/MCP events; future package/scoped policy events. |
| Data model changes | None now. |
| Tests | Refresh/toggle/add/edit flows use runtime; redacted secrets in display. |
| Acceptance | Panels show inventory and diagnostics without changing chat state directly. |
| Risks | Treating capability enablement as permission grant. |
| Blocked by / Unlocks | Package governance and scoped activation blocked by runtime APIs. |

### Audit / Debug Drawer

| Field | Plan |
| --- | --- |
| Goal | Make audit/debug a contextual drawer reachable from turns, tools, permissions, tasks, and diagnostics. |
| Non-goals | No frontend console log as audit, no replay fabrication. |
| Go packages | Existing runtime audit; future replay/export. |
| React packages | `RuntimeAuditDrawer.tsx`. |
| Runtime API / event schema | Existing audit by turn/session; future replay/export APIs. |
| Data model changes | None in UI. |
| Tests | Audit refresh by turn/session; redacted display. |
| Acceptance | Audit drawer reads runtime records; replay/export controls are blocked until APIs exist. |
| Risks | Too much raw JSON without meaningful grouping. |
| Blocked by / Unlocks | Replay/export blocked by runtime persisted event APIs. |

### Session Resume / Recovery UI

| Field | Plan |
| --- | --- |
| Goal | Surface active, waiting, interrupted, and recovered state clearly after reload/restart. |
| Non-goals | No frontend continuation logic, no event-only reconstruction. |
| Go packages | Existing `runtime_recovery.go`, turns, permissions, tasks. Future compact-aware recovery. |
| React packages | `useAssistantClient.tsx`, `ChatWorkspace.tsx`, timeline notices. |
| Runtime API / event schema | `/v1/recovery/status`, active turns, pending permissions, events. Future compact reinjection events. |
| Data model changes | None in UI. |
| Tests | Startup recovery states, pending permission modal after reload, interrupted task display. |
| Acceptance | Refresh/restart loads same facts from runtime; UI distinguishes loading/no-session/runtime-unavailable/interrupted. |
| Risks | Hiding interrupted state or clearing pending permissions locally. |
| Blocked by / Unlocks | Compact-aware recovery blocked by compact/reinjection APIs. |

## Recommended UI Commit Scope

First recommended product commit:

```text
client: align shell with Claude-client-style chat IA
```

Scope:

- Rework the app shell and navigation hierarchy around the conversation.
- Make the left sidebar the primary session/project/runtime feature navigator.
- Keep chat timeline central and composer anchored.
- Normalize right drawer routing for settings, audit, permissions, tool/task,
  and future artifact/detail surfaces.
- Add blocked-by-runtime states for compact budget, artifacts, task messaging,
  worktree/isolation, replay/export, and scoped policy rule editing.
- Do not change Go or React business logic beyond DTO consumption and
  presentation wiring.

Main tests:

- `cd client && npm run build`
- Manual/browser smoke for load sessions, send/cancel, permission review,
  model settings, capabilities/skills/MCP, audit drawer, and reload recovery.

## Acceptance Checklist

- First screen is conversation-first.
- Left sidebar can navigate sessions and runtime feature surfaces.
- Center timeline remains the primary working area.
- Composer clearly shows send/cancel/model/status and blocked attachments.
- Permission review uses runtime risk/reason/target only.
- Tool/task/audit/detail surfaces open from runtime-backed timeline facts.
- Missing runtime APIs are explicitly marked blocked.
- No Claude branding or proprietary assets are copied.
- No TUI/CLI UI patterns are reintroduced as the product path.
- React remains a thin consumer of Go runtime state.
