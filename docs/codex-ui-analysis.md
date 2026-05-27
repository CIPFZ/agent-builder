# Codex UI Analysis

Status: reference analysis for Agent Builder coding-agent workspace design.

This document summarizes the Codex desktop screenshots provided during
frontend planning. Codex is used as a reference for coding-agent workspace
structure, runtime controls, environment context, settings organization, and
review flows. Agent Builder must not copy Codex branding, proprietary assets,
or exact visual styling.

## High-Level Pattern

Codex presents a coding-agent workbench:

```text
Desktop menu bar
Left navigation and project/session tree
Center workspace
Right environment inspector
Bottom composer
Settings workspace with secondary navigation
```

Compared with Claude Desktop and cc-haha:

- Claude Desktop is the reference for a restrained conversation-first
  experience.
- cc-haha is the reference for Claude Code runtime feature coverage.
- Codex is the closest reference for a coding-agent desktop workspace.

## Main Shell

Observed left navigation:

- New chat
- Search
- Skills
- Plugins
- Automations
- Projects
- Project-scoped sessions
- Settings

Agent Builder target:

```text
Left Sidebar
  New chat
  Search
  Skills
  Plugins
  Automations
  Projects / Workspaces
    Sessions
  Settings
```

The project-first structure is a better fit for Agent Builder than a flat
recent-session list because runtime state is naturally scoped by workspace,
cwd, repo, branch, worktree, and session.

## Empty Chat Home

Observed elements:

- centered prompt framing the current project
- compact task composer
- add/context button
- permission mode
- model selector
- send button
- workspace/project selector
- local mode selector
- branch selector

Agent Builder target:

```text
Composer
  prompt
  add context
  permission policy
  model
  workspace/project
  local/remote mode
  branch/worktree
  send/cancel
```

This is more suitable than a generic chat composer because an Agent Builder
turn is a runtime operation with scope, policy, model, and execution context.

Implementation candidates:

- Ant Design X `Sender`
- Ant Design `Dropdown`
- Ant Design `Segmented`
- Ant Design `Select`
- Ant Design `Tag`
- Ant Design `Button`

## Project And Session Tree

Observed pattern:

```text
Project
  conversation title
  last updated time
```

Agent Builder target:

- project/workspace grouping
- current repo/workdir
- session list
- active turn indicators
- missing directory indicators
- branch/worktree state
- dirty-change indicators
- remote/auth state when available

First pass can use existing session and status APIs while project/workspace APIs
evolve.

## Active Chat Workspace

Observed elements:

- top session title
- central timeline
- assistant response text
- processing duration
- editing/thinking state
- file change summary
- review entry
- attachment thumbnails
- right environment inspector
- composer stays available at the bottom

Agent Builder target timeline objects:

- user messages
- assistant messages
- thinking/reasoning
- tool calls
- permission gates
- todos/plans
- agent tasks
- edited files
- changed-file summary
- diff/review entry
- refs/artifacts
- recovery notices

The UI should show runtime primitives as structured objects rather than hiding
them inside assistant message text.

## Right Environment Inspector

Observed environment panel:

- changed lines summary
- local mode
- current branch
- commit entry
- GitHub CLI auth state
- source list such as web search

Agent Builder target:

```text
Runtime Inspector
  cwd / workspace
  branch / worktree
  dirty diff
  changed files
  permission mode
  active tools
  active agent task
  sources
  auth status
  audit/replay links
```

This should become a core differentiator for Agent Builder. It makes the agent
runtime's effective scope and side effects visible while the conversation stays
focused.

## Skills

Observed Skills page:

- title and description
- refresh action
- search
- create/new skill action
- installed skill list
- two-column list layout
- item icon, name, short description, installed check

Agent Builder target:

```text
Skills Overview
  search
  refresh
  create skill
  installed/enabled state
  source
  short description

Skill Detail
  SKILL.md preview
  files
  activation metadata
  allowed tools
  diagnostics
```

Codex is a good reference for the overview list; Claude Desktop is a better
reference for deep skill details.

## Automations

Observed Automations page:

- title and help link
- "view templates"
- "create through chat"
- empty state
- quick templates:
  - daily brief
  - weekly review
  - project monitor

Agent Builder target:

- scheduled tasks
- project monitors
- recurring agent runs
- manual run templates
- create from chat
- run now
- enable/disable
- last result and next run

This should connect to runtime-owned automation/task APIs. Until then, it can
be a placeholder or template-only surface.

## Settings Structure

Observed settings navigation:

- General
- Appearance
- Config
- Personalization
- Keyboard shortcuts
- MCP servers
- Hooks
- Connections
- Git
- Environment
- Worktrees
- Browser
- Computer control
- Archived conversations

Agent Builder should separate product settings from runtime settings:

```text
Product Settings
  Appearance
  Language
  Keyboard shortcuts
  Personalization

Runtime Settings
  Config
  Policy
  MCP
  Hooks
  Git
  Environment
  Worktrees
  Browser
  Computer Use
  Archived sessions
```

## General Settings

Observed controls:

- work mode:
  - coding
  - daily work
- default permission
- auto review
- full access permission
- default open target
- agent environment
- integrated terminal shell
- language

Agent Builder mapping:

```text
Work mode
  coding
  general task

Permission policy
  ask
  auto review
  full access

Environment
  native
  WSL/container/future

Open target
  VS Code
  system default

Shell
  PowerShell
  CMD
  Git Bash
  WSL
```

These settings should map to runtime config and policy. React must not enforce
permission or sandbox decisions locally.

## Appearance Settings

Observed controls:

- theme: light / dark / system
- theme preview rendered as code diff
- import theme
- copy theme
- accent color
- background
- foreground
- UI font
- code font
- translucent sidebar
- contrast

Agent Builder target:

- light / dark / system
- Ant Design token preview
- accent color
- compact/dense mode
- UI font
- code font
- UI scale

The code-diff preview is especially appropriate for a coding agent client.
First pass can keep the theme surface simpler.

## Config Settings

Observed controls:

- `config.toml` custom settings
- deprecated config warning
- config scope selector
- open `config.toml`
- approval policy
- sandbox setting
- workspace dependencies
- current version
- allow installation of Node/Python tools
- diagnose workspace
- reset/reinstall workspace

Agent Builder target:

```text
Config
  config file path
  policy mode
  sandbox mode
  workspace dependencies
  diagnostics
  reset/reinstall runtime dependencies
```

This page should show the relationship between UI settings and the underlying
runtime config source.

## MCP Servers

Observed MCP page:

- server list
- add server
- per-server settings
- enable/disable switch
- auth button for servers requiring authentication

Agent Builder target:

- combine Codex's simple list interaction with cc-haha's summary cards
- add/edit server
- enable/disable server
- server auth status
- server settings
- tool/resource/prompt counts
- pending auth/elicitation status

## Computer Control

Observed page:

- Google Chrome control integration
- browser extension connection status
- install button
- always-allowed apps list

Agent Builder target:

```text
Browser / Computer Control
  browser extension
  allowed apps
  app control permissions

Computer Use Setup
  Python
  dependencies
  virtualenv
  environment checks
```

Codex focuses on controlling external applications. cc-haha focuses on Python
environment readiness. Agent Builder should treat these as separate capability
surfaces.

## Plugins

Observed left-nav entry exists, but the screenshots do not show plugin details.

Agent Builder target:

- keep Plugins as a later feature unless capability package governance becomes
  first-version scope
- avoid making plugin UI the primary way to manage runtime tools before the
  runtime contract is stable

## Menu Bar

Observed desktop menu:

- File
- Edit
- View
- Window
- Help

Agent Builder should preserve a desktop-appropriate menu bar if Wails supports
the required menu integration:

- File: new chat, open workspace, settings
- Edit: copy, paste, find
- View: toggle sidebar, zoom, theme
- Window: window controls
- Help: docs, diagnostics, about

## Combined Reference Roles

The three UI references now have distinct roles:

```text
Claude Desktop
  restrained primary chat experience
  simple main workspace
  high-level Projects / Artifacts / Customize split

cc-haha
  Claude Code runtime feature checklist
  providers, policy, MCP, agents, skills, memory, diagnostics, usage

Codex
  coding-agent workspace mechanics
  project tree
  composer runtime controls
  right environment inspector
  Git/worktree/diff/review/config/settings
```

## Proposed Agent Builder Shell

```text
App Shell
  Left Sidebar
    New chat
    Search
    Skills
    Plugins
    Automations
    Projects
      Sessions
    Settings

  Center Workspace
    Chat
    Runtime pages
    Settings pages

  Right Inspector
    Environment
    Tool details
    File changes
    Diff/review
    Task status
    Sources
    Audit

  Composer
    Prompt
    Attach/context
    Policy mode
    Model
    Workspace
    Local/remote mode
    Branch/worktree
    Send/cancel
```

## Priority Recommendation

First implementation phase:

1. Project/session sidebar.
2. Chat timeline plus Ant Design X composer.
3. Composer runtime controls: model, policy, workspace, branch/worktree.
4. Right runtime inspector.
5. Tools, thinking, permissions, and tasks in the timeline.
6. Skills list.
7. MCP management.
8. Settings: General, Config, Policy, Environment, Git, Worktrees.
9. Diagnostics.
10. Usage, Automations, and Computer Control.

## Product Principle

Codex is the closest reference for Agent Builder as a coding-agent desktop
client. Agent Builder should borrow the project-first workspace, composer
runtime controls, right environment inspector, and settings taxonomy while
keeping the implementation grounded in Ant Design, Ant Design X, and Go runtime
state.

