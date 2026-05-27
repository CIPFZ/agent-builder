# Claude Desktop UI Analysis

Status: reference analysis for the Agent Builder frontend rewrite.

This document summarizes the Claude Desktop screenshots provided during the UI
planning discussion. Claude Desktop is used only as an information architecture
and workflow reference. Agent Builder must not copy Claude branding, proprietary
assets, exact visual styling, copy, colors, typography, spacing, or animations.

## High-Level Pattern

The screenshots show a stable desktop workbench:

```text
Global desktop shell
  Left primary navigation
  Center workspace
  Bottom composer for chat views
  Secondary management workspaces for projects, artifacts, and customization
  Account/settings menu at the bottom of the sidebar
```

The important lesson is that the main chat view stays simple while advanced
capabilities are moved into structured secondary workspaces such as Projects,
Artifacts, and Customize.

## Global Shell

Observed controls:

- menu button
- sidebar toggle
- global search
- back / forward navigation
- window controls
- small account/status affordance

Agent Builder mapping:

- sidebar toggle
- global search across sessions, refs, tools, skills, and audit
- back / forward navigation inside the desktop shell
- runtime status and diagnostics affordance
- settings/account menu

Implementation candidates:

- Ant Design `Layout`
- Ant Design `Button`, `Tooltip`, `Dropdown`
- Wails window controls where needed

## Left Navigation

Observed primary entries:

- New chat
- Projects
- Artifacts
- Customize
- Recents
- account/plan area at the bottom

Agent Builder mapping:

```text
New chat
Workspaces / Projects
Artifacts / Refs
Customize / Runtime
Recents / Sessions
Account / Settings
```

Runtime mapping:

| Claude area | Agent Builder area | Runtime-backed data |
| --- | --- | --- |
| New chat | New session / turn entry | sessions, turns |
| Projects | Workspaces / project context | workspace/project APIs when available |
| Artifacts | Refs, diffs, outputs, reports | refs, artifact refs, diff refs, output refs |
| Customize | Runtime capabilities | skills, MCP, hooks, policy, capabilities |
| Recents | Sessions | session list and active session |
| Account | Settings/status | model config, policy, runtime diagnostics |

First pass:

- New chat
- Sessions
- Runtime/Customize
- Skills
- MCP/Connectors
- Artifacts/Refs placeholder

## Empty Chat Home

Observed layout:

- plan/status badge near the top
- large greeting
- central large composer
- prompt chips below composer
- composer toolbar with add button, model picker, voice controls

Agent Builder adaptation:

- avoid Claude greeting, symbol, and copy
- use Agent Builder-specific task framing
- keep the central first-run task input
- show runtime/model configuration state clearly

Possible Agent Builder copy:

```text
Start an agent task
Describe a task, investigation, code change, or workflow.
```

Runtime-backed states:

- model not configured
- runtime unavailable
- active/interrupted turn recovered
- pending permission exists
- current model/provider/policy

Implementation candidates:

- Ant Design X `Welcome`
- Ant Design X `Prompts`
- Ant Design X `Sender`
- Ant Design `Alert`, `Tag`, `Dropdown`

## Active Chat View

Observed layout:

- sidebar remains visible
- session title appears at top with a dropdown
- main content becomes scrollable conversation
- composer is fixed near the bottom
- return-to-bottom affordance appears while scrolled
- disclaimer below composer
- share/export action appears near the upper-right workspace area

Agent Builder target:

```text
Session Header
  title
  model/provider
  runtime status
  active turn status
  audit/export/share actions

Timeline
  user messages
  assistant messages
  thinking/reasoning
  tool calls
  permissions
  todos/plans
  agent tasks
  refs/artifacts
  recovery notices

Composer Dock
  text input
  add/context menu
  model selector
  policy/mode selector
  send/cancel
```

Implementation candidates:

- Ant Design X `Bubble.List`
- Ant Design X `Sender`
- Ant Design X `ThoughtChain`
- custom `ToolCallCard`
- custom `PermissionGate`
- custom `AgentTaskCard`

## Composer Add Menu

Observed items:

- Add files or photos
- Add to project
- Skills
- Add connectors
- Web search
- Use style

Agent Builder mapping:

```text
Attach files / folders
Add context source
Add to workspace
Enable skills
Enable MCP tools
Enable search or tool capability
Use agent role / profile
```

Runtime requirements:

- file/ref/context-source APIs for attachments
- workspace/project context APIs
- skill listing and activation metadata
- MCP server/tool listing and enablement
- capability inventory and policy state
- agent role/profile APIs

Unsupported items should be disabled or marked unavailable rather than
implemented as local-only state.

## Model Picker

Observed items:

- current model selector inside composer
- model list with selected check
- plan/upgrade notices
- adaptive thinking toggle
- more models entry

Agent Builder mapping:

```text
Model picker
  provider/model list
  selected model
  model capabilities
  reasoning/thinking mode
  context window / budget status
  configure models
```

Runtime-backed data:

- configured models
- provider/model config
- runtime status
- budget/context report when available
- policy or reasoning mode when available

Implementation candidates:

- Ant Design `Dropdown` or `Popover`
- Ant Design `Switch`, `Tag`, `Descriptions`

## Account Menu

Observed items:

- Settings
- Language
- Get help
- Upgrade plan
- Get apps and extensions
- Learn more
- Log out

Agent Builder mapping:

```text
Settings
Language
Runtime diagnostics
Extensions / plugins
Documentation
About
Exit / log out if auth exists
```

The sidebar footer can also expose:

- current workspace
- provider/model
- runtime connection state
- policy mode

## Projects Workspace

Observed layout:

- sidebar remains
- page title
- sort button
- search button
- New project button
- centered empty state

Agent Builder adaptation:

- use this for Workspaces / Projects once runtime has a project model
- group sessions, instructions, context sources, files/refs, skills, and MCP
  scope under a workspace
- first pass may show a blocked or read-only state if runtime project APIs are
  not ready

## Artifacts Workspace

Observed layout:

- page title
- New artifact button
- prominent search field
- centered empty state

Agent Builder adaptation:

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
- support preview when `readRefContent` can return content
- otherwise show summary and provenance

## Customize Workspace

Observed layout:

```text
Top back header
Left secondary navigation
  Skills
  Connectors
  Plugins
Main content
```

This maps strongly to Agent Builder runtime management.

Agent Builder sections:

- Skills
- Connectors / MCP
- Agent roles
- Hooks
- Policy
- Capabilities
- Plugins, when package governance exists

Possible product title:

```text
Runtime Capabilities
```

or:

```text
Customize Agent Builder
```

## Skills Workspace

Observed layout:

```text
Customize subnav
Skill list and tree
Skill detail panel
```

Observed detail content:

- skill title
- enabled toggle
- overflow menu
- added by
- trigger
- description
- preview/code toggle
- rendered `SKILL.md`

Agent Builder mapping:

- builtin skills
- local skills
- enabled/disabled
- path and `SKILL.md`
- allowed tools
- activation metadata
- diagnostics/errors
- create skill
- add skill path
- refresh

Implementation candidates:

- Ant Design `Splitter`
- Ant Design `Tree`
- Ant Design `List`
- Ant Design `Descriptions`
- Ant Design `Switch`
- Ant Design `Tabs`
- Ant Design X `XMarkdown` or markdown renderer

## Connectors Workspace

Observed layout:

```text
Customize subnav
Connector list
Connector detail / connect state
```

Agent Builder mapping:

- MCP servers as connectors
- server connection state
- tools/resources/prompts counts
- connect/edit/refresh
- enable/disable server
- enable/disable tool
- auth/elicitation request state

Implementation candidates:

- Ant Design `List`
- Ant Design `Descriptions`
- Ant Design `Button`
- Ant Design `Switch`
- Ant Design `Tabs`
- Ant Design `Alert`

## First-Version Feature List

Must implement in the first frontend rewrite:

- global shell
- left navigation
- new chat
- session list
- chat timeline
- composer
- model picker
- add/context menu
- thinking/tool/permission timeline items
- runtime customize workspace
- skills management
- MCP/connectors management
- settings
- audit/diagnostics entry

Second version:

- workspaces/projects
- artifacts/refs center
- worktree/diff inspector
- agent/subagent task panel
- budget/context panel
- replay/recovery detail
- hooks/policy editor

## Product Principle

Keep the main chat view minimal. Put advanced runtime capabilities into
structured management and inspector surfaces. Agent Builder should show tools,
thinking, permissions, tasks, and audit as structured runtime objects, not as
unstructured text embedded in the assistant response.

