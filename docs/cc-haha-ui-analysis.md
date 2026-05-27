# cc-haha UI Analysis

Status: reference analysis for Agent Builder runtime feature planning.

This document summarizes the cc-haha screenshots provided during frontend
planning. cc-haha is used as a feature and workflow reference for a Claude
Code-style desktop client. Agent Builder must not copy cc-haha source code,
branding, assets, or exact visual styling.

Claude Desktop remains the main information-architecture reference for the
conversation experience. cc-haha is most useful as a runtime capability surface
reference.

## High-Level Pattern

cc-haha separates chat from runtime controls through two navigation layers:

```text
Primary sidebar
  New chat
  Scheduled tasks
  Project/session list
  Settings

Settings secondary navigation
  Providers
  Permissions
  General
  H5 access
  IM integration
  Terminal
  MCP
  Agents
  Skills
  Memory
  Plugins
  Computer Use
  Token usage
  Diagnostics
  About
```

The product lesson is that a coding agent desktop client needs a substantial
runtime control plane, but the main chat surface should stay focused.

## New Chat

Observed elements:

- centered empty state
- large composer
- `+` action
- permission mode selector
- environment/status selector
- model selector
- run button
- project picker

Agent Builder adaptation:

```text
Composer
  input
  add/context menu
  permission mode
  model selector
  run button
  project/workdir picker
```

Implementation candidates:

- Ant Design X `Sender`
- Ant Design `Dropdown`
- Ant Design `Button`
- Ant Design `Select`
- Ant Design `Tag`

Runtime-backed data:

- active session
- active project/workdir
- model config
- policy mode
- runtime status
- effective scope

## Permission Mode

Observed permission modes:

- ask before tool execution
- auto-accept file edits
- plan mode
- bypass all permission checks

Agent Builder mapping:

```text
Ask
Auto edits
Plan
Bypass
```

The UI may expose these modes in the composer and settings, but Go runtime
policy remains the source of truth. React must not implement local allow/deny
logic.

## Project And Session Sidebar

Observed elements:

- project group
- session rows under a project
- session title
- missing directory status
- date
- session search
- refresh action
- cleanup/delete action
- expand more

Agent Builder target:

- workspace/project grouping
- sessions by workspace
- current working directory
- worktree/repo status
- missing directory state
- search sessions
- refresh sessions
- archive/delete/clear session actions

First pass can use existing runtime session and status data while project APIs
are still evolving.

## Scheduled Tasks

Observed elements:

- scheduled task page
- creation entry
- empty state
- note that tasks run only while desktop app is open
- `/schedule` creation hint

Agent Builder target:

- scheduled task list
- next run time
- enabled/disabled
- last run status
- last result
- run now
- delete
- create from chat or slash command when runtime supports it

This should be implemented only when the runtime owns the schedule and task
lifecycle. Until then, show a blocked/unavailable state.

## Providers

Observed elements:

- provider list
- default provider marker
- connection/auth status
- login/OAuth instructions
- add provider

Agent Builder target:

```text
Providers
  provider list
  default provider
  auth status
  API key / OAuth / base URL
  model discovery
  verify connection
```

Implementation candidates:

- Ant Design `Card`
- Ant Design `Form`
- Ant Design `Button`
- Ant Design `Tag`
- Ant Design `Descriptions`

## General Settings

Observed settings:

- theme
- application language
- reply language
- reasoning strength
- thinking mode
- system notifications
- UI scale
- web search API keys
- data storage location

Agent Builder grouping:

```text
Appearance
Language
Model behavior
Notifications
Search providers
Data directory
```

Reasoning strength and thinking mode must map to runtime/model configuration,
not frontend prompt assembly.

## Terminal

Observed elements:

- terminal status
- current shell path
- startup shell selection
- Bash path configuration
- save/restore defaults
- embedded terminal
- clear and restart actions

Agent Builder target:

- shell/runtime environment diagnostics
- shell path configuration
- tool execution environment display
- optional embedded terminal after permission and security review

Interactive terminal access should be treated as a permission-sensitive feature.
The first pass may be read-only diagnostics plus configuration.

## MCP

Observed elements:

- MCP service page
- add server
- summary cards: total, connected, needs attention
- empty state
- Local / Project / User scope
- STDIO / HTTP / SSE transport

Agent Builder target:

- server list
- server state
- scope: local / project / user
- transport: stdio / HTTP / SSE
- tools/resources/prompts counts
- pending auth/elicitation
- enable/disable
- refresh/retry
- add/edit server
- tool allowlist

This is a first-version priority because Agent Builder already has runtime MCP
APIs.

## Agents

Observed elements:

- installed agents summary
- total count
- active/effective count
- source type count
- built-in agent list
- agent name
- model tag
- source tag
- enabled tag
- description
- allowed tools count
- detail affordance

Agent Builder target:

```text
Agent Roles
  role list
  model/provider
  source
  status
  allowed tools
  description
  detail

Agent Tasks
  running tasks
  completed tasks
  parent/child session
  cancellation
  result summary
```

The first pass should implement Agent Roles from runtime definitions and show
AgentTask status in chat timeline and inspectors.

## Skills

Observed elements:

- installed skills page
- empty state
- note that skills are managed in a local directory

Agent Builder target should combine cc-haha's list/status view with the richer
Claude Desktop skill detail pattern:

- builtin/local skills
- enabled/disabled
- path
- diagnostics
- allowed tools
- activation metadata
- create skill
- add skill path
- refresh
- preview `SKILL.md`

## Memory

Observed elements:

- project memory
- resource manager
- search project or memory file
- memory file list
- editor/detail pane
- refresh / restore / save actions

Agent Builder target:

```text
Memory / Context
  project instructions
  user memory
  context sources
  read-file state
  compact summaries
  managed instructions
```

First pass should show runtime context sources and memory state read-only unless
runtime editing APIs are explicitly available.

## Plugins

Observed state:

- installed plugins page
- empty state

Agent Builder target:

- defer until capability package governance is stable
- do not make plugins a first-version priority unless required by runtime

## Computer Use

Observed elements:

- enable switch
- Python 3 detection
- virtual environment status
- dependency status
- Python interpreter path
- install environment
- re-detect

Agent Builder target:

```text
Capability Setup
  Computer Use
  Browser
  Git
  Shell
  Python
  Node
```

This should be a capability dependency diagnostic and setup surface. First pass
can be read-only diagnostics if runtime setup actions are not ready.

## Token Usage

Observed elements:

- today token count
- yesterday token count
- 30-day token count
- yearly contribution-style heatmap
- intensity legend

Agent Builder target:

- today's tokens
- active session tokens
- 30-day tokens
- cost
- per model/provider usage
- per session usage
- heatmap
- context budget and compaction usage when available

Existing `RuntimeUsage` can support the first pass; `RuntimeBudgetReport` can
support context/budget views later.

## Diagnostics

Observed elements:

- log size
- event count
- 24h warnings
- retention policy
- Run Doctor
- log directory
- export diagnostics package
- copy error summary
- clear logs
- recent event list
- note that exported diagnostics are redacted

Agent Builder target:

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
- local log cleanup when supported

Diagnostics should be a first-version priority because Agent Builder's runtime
is designed around events, audit, replay, and recovery.

## About

Observed elements:

- app version
- update check
- GitHub link
- author/social links
- feedback issue link

Agent Builder target:

- app version
- build/runtime version
- update status when supported
- project links
- diagnostics/report issue link

This is lower priority than chat, runtime management, usage, and diagnostics.

## Proposed Agent Builder Information Architecture

Combine Claude Desktop's conversation structure with cc-haha's runtime control
surface:

```text
App Shell
  Chat
    Sessions
    Composer
    Model
    Policy mode
    Project/workdir
    Tool/thinking/permission/task timeline

  Tasks
    Active tasks
    Scheduled tasks
    Background agent tasks

  Workspaces
    Projects
    Sessions by workspace
    Context/memory

  Artifacts
    Refs
    Diffs
    Tool outputs
    Reports

  Runtime
    Providers
    Policy
    MCP
    Agents
    Skills
    Memory/Context
    Terminal
    Computer Use
    Plugins

  Usage
    Tokens
    Cost
    Budget/context

  Diagnostics
    Events
    Audit
    Recovery
    Logs
    Doctor
    Export
```

## Priority Recommendation

First implementation phase:

1. Chat shell, composer, and session list.
2. Policy mode, model selector, and project/workdir picker.
3. Thinking, tool, permission, and task timeline items.
4. Runtime settings: Providers, Policy, MCP, Skills, Agents.
5. Diagnostics.
6. Usage.

Later phases:

1. Memory/context editor.
2. Scheduled tasks.
3. Terminal and Computer Use setup.
4. Plugins.

## Product Principle

Claude Desktop should guide the minimal conversation-first workbench. cc-haha
should guide the runtime feature checklist. Agent Builder should combine both
with Ant Design and Ant Design X as the implementation foundation, while
keeping Go runtime state as the source of truth.

