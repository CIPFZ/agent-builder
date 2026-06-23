# Claude Code Reference Analysis

## Project positioning

Claude Code is best understood as an agent runtime packaged as a CLI/TUI,
not just a chat client. The reference tree at `refer/claude-code` shows a
product that combines a terminal REPL, model streaming, tool execution,
permission governance, session persistence, remote control, plugins, MCP, LSP,
skills, background tasks, and subagents.

The most important positioning signal is that user-facing commands and
model-facing tools are separate. `refer/claude-code/src/commands.ts` handles
slash commands such as `/mcp`, `/plugin`, `/tasks`, `/permissions`, and
`/resume`; `refer/claude-code/src/tools.ts` registers model-callable actions
such as Bash, file edits, MCP tools, task tools, plan mode, and agent spawning.
The core conversation runtime is pulled into `refer/claude-code/src/QueryEngine.ts`,
which explicitly says it owns lifecycle and session state for headless/SDK use.

For the planned Agent Builder-based operations client, the reference value is less
"copy Claude Code UI" and more "treat the agent as a governed execution
runtime with multiple clients."

## Technology stack and repository shape

Claude Code is TypeScript on Bun with React/Ink-style terminal UI. Its source
shape is broad and productized:

- `refer/claude-code/src/main.tsx`: top-level startup and feature-gated
  assembly.
- `refer/claude-code/src/screens/REPL.tsx` and `refer/claude-code/src/components/`:
  interactive terminal surface.
- `refer/claude-code/src/ink/`: vendored/custom terminal rendering stack.
- `refer/claude-code/src/commands/`: slash commands.
- `refer/claude-code/src/tools/`: model-callable tools.
- `refer/claude-code/src/services/api/`: Anthropic/provider API access,
  retry, logging, usage, prompt cache tracking.
- `refer/claude-code/src/services/mcp/` and `refer/claude-code/src/services/lsp/`:
  extension protocols.
- `refer/claude-code/src/skills/` and `refer/claude-code/src/plugins/`:
  prompt/workflow and distribution extension layers.
- `refer/claude-code/src/tasks/`, `refer/claude-code/src/remote/`,
  `refer/claude-code/src/bridge/`, and `refer/claude-code/src/cli/transports/`:
  background and remote execution paths.

Compared with Agent Builder, which is Go-based with Bubble Tea, SQLite/sqlc, and
`charm.land/fantasy`, Claude Code has more product-side TypeScript surface,
but the architectural boundaries map cleanly to Agent Builder subsystems:
`internal/agent`, `internal/agent/tools`, `internal/permission`,
`internal/session`, `internal/hooks`, `internal/skills`, `internal/lsp`,
and `internal/ui`.

## Startup and main loop

`refer/claude-code/src/main.tsx` is an application assembler. The reference
docs in `refer/claude-code/docs/03-startup-and-main-loop.md` identify it as
responsible for early prefetching, configuration, auth, feature gates,
plugin/skill/MCP/LSP initialization, command/tool assembly, and selecting
interactive, remote, assistant, bridge, or headless paths.

The REPL path is intentionally delayed through
`refer/claude-code/src/replLauncher.tsx`, which dynamically imports the UI
entrypoints and wraps `REPL` in `App`. `refer/claude-code/src/components/App.tsx`
provides shared app state and metrics contexts. This keeps the runtime usable
outside the TUI.

The execution loop is separated into `refer/claude-code/src/QueryEngine.ts`.
`QueryEngine` keeps mutable messages, read-file state, usage, permission
denials, abort controller state, and per-turn skill discovery, then delegates
the model/tool loop to `refer/claude-code/src/query.ts`. This is a useful
contrast with a purely UI-driven loop: the runtime can be embedded by SDK,
remote clients, and tests.

## Model/provider abstraction

Claude Code separates model selection from provider connection. Model defaults,
aliases, subscription-based choices, plan-mode changes, and model capability
checks live under `refer/claude-code/src/utils/model/`, especially
`utils/model/model.ts`, `modelStrings.ts`, `providers.ts`, `modelCapabilities.ts`,
and `modelAllowlist.ts`.

The API side lives in `refer/claude-code/src/services/api/`. The reference docs
point to `services/api/client.ts` for Anthropic API, Bedrock, Vertex, and Azure
Foundry client setup. `services/api/claude.ts` converts internal messages and
tools into Anthropic beta message requests, applies betas and prompt-cache
headers, streams events, updates usage, and logs API success/failure. It also
tracks request IDs through bootstrap state.

For Agent Builder, `charm.land/fantasy` already gives a provider abstraction. The
lesson is to keep a second layer above provider calls for product policy:
model aliases, capability gates, default model choice, per-mode model override,
usage accounting, retry/fallback behavior, and provider-specific feature
availability.

## Session, message, and event state

Claude Code has both in-memory process state and append-only transcript state.
`refer/claude-code/src/bootstrap/state.ts` is a large singleton containing
session ID, parent session ID, project root, current cwd, model usage, cost,
telemetry providers, prompt IDs, permission mode flags, scheduled tasks,
session-created teams, invoked skills, prompt-cache latches, last API request,
last API messages, and many product feature latches.

Persistent transcripts are JSONL under a sanitized project directory. The
main implementation is `refer/claude-code/src/utils/sessionStorage.ts`.
It writes serialized messages with `cwd`, `userType`, `sessionId`, timestamp,
version, git branch, and optional metadata. It distinguishes transcript
messages from ephemeral progress messages, bridges legacy progress entries,
caps large transcript reads at 50 MB, and writes subagent transcripts under
`<sessionId>/subagents/agent-<agentId>.jsonl`.

Session metadata is rich. `refer/claude-code/src/types/logs.ts` includes
summary, custom title, AI title, tags, agent name/color, PR links, worktree
state, attribution snapshots, file history snapshots, context collapse records,
and content replacement records. This makes resume, session listing, and
cross-client display practical.

## Tool registry and execution protocol

The central registry is `refer/claude-code/src/tools.ts`. `getAllBaseTools()`
returns built-ins such as `AgentTool`, `BashTool`, file read/edit/write,
notebook edit, web fetch/search, todos, plan mode, task tools, MCP resource
tools, LSP, worktree tools, team tools, cron/monitor tools, and test-only
permission tools. Availability is controlled by feature gates, env vars,
user type, platform support, and permission blanket-deny filters.

The tool abstraction in `refer/claude-code/src/Tool.ts` is the deeper lesson.
`ToolUseContext` carries commands, tools, model settings, thinking config,
MCP clients/resources, agent definitions, app state getters/setters,
notifications, OS notifications, memory attachment tracking, skill discovery
tracking, file read state, attribution state, messages, and optional SDK
status callbacks. `ToolPermissionContext` carries mode, working directories,
allow/deny/ask rules, bypass availability, auto-mode availability, dangerous
rules, prompt-avoidance flags, and pre-plan-mode state.

This is an execution protocol, not a bare function registry. Tool calls produce
schema-visible inputs, progress events, UI rendering, transcript messages,
permission decisions, persisted large outputs, and resumable state. Bash is
the most concrete example: `refer/claude-code/src/tools/BashTool/BashTool.tsx`
handles command schema, background execution, sandbox override flags, security
parsing, read-only classification, progress display, output truncation, shell
task registration, and file-history tracking.

## Permission, approval, and safety model

Permission modes are formal runtime state, not UI hints. The reference docs
and `refer/claude-code/src/types/permissions.ts` identify modes including
`default`, `acceptEdits`, `bypassPermissions`, `dontAsk`, and `plan`, with
internal modes such as `auto` and `bubble`. Permission sources include user,
project, local, flag, policy, CLI, command, and session scopes.

`refer/claude-code/src/utils/permissions/permissionSetup.ts` loads rules,
applies permission updates, validates extra workspace directories, and strips
or flags dangerous classifier-bypassing rules. It explicitly treats broad
Bash rules, broad PowerShell rules, and Agent auto-approval as dangerous for
auto mode. Tool-level safety is also specialized: Bash has large dedicated
files for permissions, read-only validation, path validation, sed validation,
destructive warnings, and sandbox decisions under
`refer/claude-code/src/tools/BashTool/`.

Plan mode is part of permission design. `refer/claude-code/src/tools/EnterPlanModeTool/`
switches the agent into a read/design phase, while
`refer/claude-code/src/tools/ExitPlanModeTool/ExitPlanModeV2Tool.ts` restores
or requests approval to leave plan mode. For operations workflows, this is
worth borrowing: planning, approval, and execution should be state-machine
transitions with audit trails.

## MCP, plugin, skill, and extension model

Claude Code uses several extension layers in parallel.

Skills are markdown/frontmatter-driven prompt and workflow fragments. The
loader is `refer/claude-code/src/skills/loadSkillsDir.ts`; bundled skills live
under `refer/claude-code/src/skills/bundled/`. Skills can include metadata
such as description, allowed tools, hooks, paths, model, effort, and inline
or fork execution behavior.

Plugins are broader distribution units. `refer/claude-code/src/types/plugin.ts`
and `refer/claude-code/src/utils/plugins/schemas.ts` model commands, agents,
skills, hooks, output styles, MCP servers, LSP servers, and settings. Built-in
plugin registration is in `refer/claude-code/src/plugins/builtinPlugins.ts`;
plugin command loading is in `refer/claude-code/src/utils/plugins/loadPluginCommands.ts`.

MCP is a first-class external tool/resource layer. `refer/claude-code/src/services/mcp/client.ts`
uses stdio, SSE, streamable HTTP, WebSocket, and SDK-control transports;
connects clients; lists tools/resources/prompts; handles OAuth/session expiry;
truncates large descriptions and outputs; persists binary or large results;
and maps MCP tools through `MCPTool`, `ListMcpResourcesTool`, and
`ReadMcpResourceTool`.

For Agent Builder, the implication is to avoid making "extensions" a single plugin
interface. Skills, MCP servers, hooks, commands, LSP, and packaged plugins
have different trust and lifecycle requirements.

## Subagent, task, and scheduler model

Subagents are launched through `refer/claude-code/src/tools/AgentTool/AgentTool.tsx`.
Inputs include description, prompt, subagent type, model, background flag,
name, team name, mode, isolation, and cwd. Supporting files include
`runAgent.ts`, `forkSubagent.ts`, `resumeAgent.ts`, `agentMemory.ts`,
`agentMemorySnapshot.ts`, and `loadAgentsDir.ts`.

Built-in roles are explicit under `refer/claude-code/src/tools/AgentTool/built-in/`:
general-purpose, explore, plan, verification, and Claude Code guide agents.
This is role-constrained delegation rather than generic worker spawning.

Tasks are visible and manageable. `refer/claude-code/src/commands/tasks/index.ts`
defines `/tasks` as the command to list and manage background tasks. App state
in `refer/claude-code/src/state/AppStateStore.ts` has `tasks`,
`agentNameRegistry`, `foregroundedTaskId`, `viewingAgentTaskId`, and per-agent
todos. Bash can register local shell tasks through
`refer/claude-code/src/tasks/LocalShellTask/LocalShellTask.js`; remote agents
use remote task paths. Scheduling appears through cron and monitor tools in
`tools.ts`, guarded by feature flags such as `AGENT_TRIGGERS`.

## Sandbox, process, and workspace isolation

Claude Code distinguishes several isolation types:

- Command sandboxing: `refer/claude-code/src/tools/BashTool/shouldUseSandbox.ts`
  and `refer/claude-code/src/utils/sandbox/` decide whether shell execution
  should run in a sandbox and allow an explicit dangerous override in Bash
  input.
- Workspace isolation: `refer/claude-code/src/utils/worktree.ts` creates
  agent worktrees and decides whether to clean them up or preserve them based
  on changes.
- CWD override: AgentTool can redirect local execution to another directory
  without cloning or worktree isolation.
- Remote isolation: AgentTool can route work to remote execution and register
  it as an asynchronous task.

The reference docs in `refer/claude-code/docs/41-agent-isolation-worktree-remote-and-cwd-overrides.md`
make a key point: worktree isolation protects file modifications, remote
isolation protects execution environment, and cwd override only changes
default local paths. These should not be collapsed into one "sandbox" flag.

## Context loading, memory, and compression

Claude Code treats context as an actively managed resource. Query setup in
`refer/claude-code/src/QueryEngine.ts` loads memory prompts, system prompt
parts, plugin cache state, and file read state, then passes the full context
to the query loop.

Memory and project instructions are handled by files such as
`refer/claude-code/src/utils/claudemd.ts`, `refer/claude-code/src/memdir/memdir.ts`,
`refer/claude-code/src/memdir/memoryScan.ts`, and
`refer/claude-code/src/memdir/findRelevantMemories.ts`. The docs under
`refer/claude-code/docs/21-memory-and-claude-md.md`,
`35-claude-md-loading-and-instruction-assembly.md`, and
`36-memory-taxonomy-and-drift-prevention.md` describe the distinction between
project instructions, session memory, agent memory, and shared/team memory.

Compression is multi-layered. `refer/claude-code/src/services/compact/autoCompact.ts`
computes thresholds; `compact.ts` handles full compaction; `microCompact.ts`
removes or replaces high-cost tool results; `sessionMemoryCompact.ts` uses
session memory to replace older context while preserving tool-use/tool-result
pairs; and `postCompactCleanup.ts` re-injects needed attachments. This is a
strong design to borrow for long-running operations sessions.

## Client/UI/API boundaries

Claude Code has several clients over the same runtime:

- Interactive TUI: `refer/claude-code/src/screens/REPL.tsx`,
  `src/components/`, and custom `src/ink/`.
- Headless/SDK query engine: `refer/claude-code/src/QueryEngine.ts` and
  `refer/claude-code/src/entrypoints/agentSdkTypes.ts`.
- Structured IO: `refer/claude-code/src/cli/structuredIO.ts` serializes SDK
  messages, hook input, permission updates, session metadata, and action
  requirements.
- Transports: `refer/claude-code/src/cli/transports/SSETransport.ts`,
  `WebSocketTransport.ts`, and `HybridTransport.ts`.
- Remote/bridge: `refer/claude-code/src/remote/` and `refer/claude-code/src/bridge/`
  connect local runtime state to external hosts and remote control.

The key boundary is that UI rendering is not the protocol. Permissions,
tool lifecycle, hooks, session state, and control requests are machine-readable
events. This matters directly for an operations client, where web UI, TUI,
automation, and integrations may all need to drive the same agent runtime.

## Observability, telemetry, tests, and evals

Observability is pervasive. `refer/claude-code/src/bootstrap/state.ts` tracks
cost, API duration, tool duration, hook duration, classifier duration, line
adds/removes, model usage, prompt IDs, request IDs, and in-memory errors.
`refer/claude-code/src/services/api/logging.ts` records API query/error/success
metadata. `refer/claude-code/src/services/analytics/` provides the telemetry
pipeline.

The reference telemetry audit in
`refer/claude-code/docs/45-telemetry-and-reporting-rules-audit.md` points to
important guardrails: `services/analytics/index.ts` limits normal metadata to
booleans and numbers unless string fields are explicitly marked safe;
`datadog.ts` whitelists events and skips non-first-party providers;
`metadata.ts` truncates and normalizes tool input metadata; GrowthBook handles
feature targeting in `growthbook.ts`.

Evaluation and harness support are visible in
`refer/claude-code/docs/32-harness-and-eval-runtime.md`, structured SDK types,
VCR support in `refer/claude-code/src/services/vcr.ts`, and test-only tools
such as `TestingPermissionTool` registered in test mode by `tools.ts`.

## Designs worth borrowing

Use a runtime-first architecture. `QueryEngine` is the shape to emulate:
one stateful conversation engine that can be driven by TUI, SDK, remote,
and tests.

Make tools an execution protocol. Agent Builder tools should carry permission context,
progress, transcript semantics, UI rendering hints, persisted output handling,
and resumability rather than being simple function calls.

Treat permissions as product state. The combination of modes, scoped rules,
dangerous-rule validation, plan mode, and approval persistence is stronger
than a per-tool yes/no prompt.

Separate isolation semantics. Sandbox, worktree, cwd override, remote runtime,
and background process management solve different problems and should stay
separate in API and UI.

Persist enough metadata for operations. JSONL transcripts with parent chains,
session metadata, task summaries, worktree state, file history, attribution,
and content replacement records enable resume, audit, and cross-client status.

Invest in context lifecycle. Microcompact plus full compact plus session
memory compact is more operationally robust than waiting for context overflow.

## Gaps or risks for our target product

The reference code has a large amount of Anthropic-specific product coupling:
feature gates, GrowthBook, subscription tiers, Claude.ai OAuth, first-party
telemetry, model launch strings, ant-only branches, and provider-specific
headers. Agent Builder should borrow the architecture, not those assumptions.

The global bootstrap singleton is powerful but risky. `bootstrap/state.ts`
centralizes too many concerns for an enterprise operations client that may
need multiple concurrent sessions, stronger tenant isolation, and server-side
runtime operation. Agent Builder's service-oriented Go architecture and SQLite-backed
session model are a better base for explicit ownership.

The extension surface is broad and therefore high-risk. Plugins can carry
commands, agents, skills, hooks, MCP, LSP, settings, and output styles. For
enterprise operations, marketplace and plugin trust need a staged rollout:
local skills and MCP first, signed/managed plugins later.

The reference has many hidden feature gates. That makes product capability
hard to reason about from code alone. Agent Builder should expose runtime capabilities
through explicit config and admin policy, with diagnostics showing why a
capability is unavailable.

## Implications for Agent Builder-based implementation

Agent Builder already has the right core pieces: Go runtime, provider abstraction via
`charm.land/fantasy`, SQLite/sqlc sessions, tool interfaces under
`internal/agent/tools`, hooks under `internal/hooks`, permissions under
`internal/permission`, skills under `internal/skills`, LSP under `internal/lsp`,
and Bubble Tea UI under `internal/ui`.

The implementation direction should be:

1. Promote a runtime/session engine boundary around `internal/agent/agent.go`
   that can serve TUI, headless CLI, and future API clients.
2. Expand tool execution records so every tool call has permission decision,
   progress, result summary, persisted large-output reference, and audit
   metadata.
3. Add formal permission modes beyond allow lists: plan/read-only, default,
   accept-edits, bypass, and noninteractive deny/ask behavior.
4. Model background tasks and subagents as persisted entities, not UI-only
   goroutines. Store task ID, parent session, agent role, cwd/worktree/remote
   isolation, status, output location, and resume metadata.
5. Keep skills lightweight but add metadata that affects runtime: allowed
   tools, model/effort hint, hooks, paths, and whether the skill runs inline
   or as a fork/subagent.
6. Implement context compression as a lifecycle, not a command: start with
   tool-result compaction and transcript summaries, then add session-memory
   replacement and post-compact reinjection.
7. Define a machine-readable client protocol before building more UI: session
   events, tool events, permission requests, task events, hooks, and final
   results should all be serializable independent of Bubble Tea.
8. Keep telemetry local/auditable by default for enterprise use. Borrow the
   metadata discipline from Claude Code, but make sinks configurable and
   policy-controlled.

The practical takeaway is that Agent Builder should not become a Claude Code clone.
It should use Claude Code as evidence that the hard parts of an agentic
operations client are governance, state, recovery, and execution boundaries;
Agent Builder's Go architecture is a good base if those concerns become first-class
runtime APIs rather than TUI side effects.
