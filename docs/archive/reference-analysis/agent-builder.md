# Project positioning

Agent Builder is a terminal-first AI coding assistant, but its internals are already
closer to a local agent runtime than a simple chat CLI. The README frames it as
multi-model, session-based, LSP-enhanced, MCP-extensible, and terminal-native
(`README.md`). For the agentic operations client effort, the important point is
that Agent Builder already has a model loop, tool governance, session persistence,
events, MCP, hooks, skills, and an emerging client/server split.

The current product center is still software development: built-in tools read,
edit, search, run shell commands, consult LSPs, and delegate read-only research
tasks. Operations workflows such as cluster upgrade, runbook execution, staged
approval, rollback, and audit reporting are not first-class domain concepts yet.

# Technology stack and repository shape

The implementation is Go, module `github.com/CIPFZ/agent-builder`. Core
dependencies are Charm's UI/runtime stack and provider abstraction:

- `charm.land/fantasy` for provider-independent agent/tool streaming.
- `charm.land/catwalk` for provider/model metadata.
- `charm.land/bubbletea/v2`, `lipgloss/v2`, and `glamour/v2` for the TUI.
- SQLite through `sqlc`, `github.com/ncruces/go-sqlite3`, and
  `modernc.org/sqlite`.
- `github.com/modelcontextprotocol/go-sdk` for MCP.
- PostHog for opt-out telemetry.

The repo shape is clean for runtime extraction: CLI commands live in
`internal/cmd`, top-level service wiring in `internal/app`, model/tool
orchestration in `internal/agent`, tools in `internal/agent/tools`, persistence
in `internal/session`, `internal/message`, `internal/history`,
`internal/filetracker`, and SQL under `internal/db`. Client/server mode is split
through `internal/workspace`, `internal/server`, `internal/workbench`, and
`internal/client`.

Build/test conventions are in `Taskfile.yaml`: builds run with `CGO_ENABLED=0`
and `GOEXPERIMENT=greenteagc`; `task test` runs `go test -race -failfast ./...`;
`task fmt` runs `gofumpt -w .`.

# Startup and main loop

`main.go` is intentionally thin. It optionally starts pprof when
`AGENT_BUILDER_PROFILE` is set, then calls `cmd.Execute()` (`main.go:22`).

Interactive startup is rooted at `rootCmd` (`internal/cmd/root.go:76`). It
resolves a workspace, initializes telemetry, builds `common.DefaultCommon(ws)`,
constructs the Bubble Tea model, subscribes workspace events into the TUI, and
runs the Bubble Tea program.

Workspace startup has two modes:

- In-process mode creates config, data directory, SQLite connection, logs,
  `app.App`, and `workspace.AppWorkspace` (`internal/cmd/root.go:247`).
- Client/server mode is gated by `AGENT_BUILDER_CLIENT_SERVER` (`internal/cmd/root.go:212`),
  connects to or starts a detached local server, creates a remote workspace, and
  wraps it in `workspace.ClientWorkspace` (`internal/cmd/root.go:307`,
  `internal/cmd/root.go:326`).

Non-interactive execution is a parallel entry point. `agent-builder run` reads prompt
args/stdin, optionally overrides models, resolves or creates a session, sends a
message, then streams assistant message deltas from pub/sub or server events to
stdout (`internal/cmd/run.go:30`, `internal/cmd/run.go:158`,
`internal/app/app.go:207`).

# Model/provider abstraction

Agent Builder delegates model protocol differences to `fantasy`, but builds providers
itself from Agent Builder config. `Coordinator.Run` refreshes models every turn,
resolves provider config, merges model/provider/Catwalk options, refreshes OAuth
or env-derived credentials on 401s, then calls the current `SessionAgent`
(`internal/agent/coordinator.go:162`).

Provider construction is centralized in `buildProvider`, covering OpenAI,
Anthropic, OpenRouter, Vercel, Azure, Bedrock, Google Gemini, Vertex, Hyper, and
OpenAI-compatible APIs (`internal/agent/coordinator.go:852`). Provider options
include special handling for reasoning effort, Anthropic thinking, OpenAI
Responses API, Copilot Responses model IDs, and provider-specific extra body
fields.

Model/provider metadata comes from configured providers plus Catwalk cache and
auto-update. `config.Providers` loads cached and remote provider lists unless
auto-update or default providers are disabled (`internal/config/provider.go:139`).
Custom providers are represented by `ProviderConfig` and selected models by
`SelectedModel` (`internal/config/config.go:70`, `internal/config/config.go:91`).

# Session, message, and event state

Sessions are persisted in SQLite and exposed through `session.Service`
(`internal/session/session.go:49`, `internal/session/session.go:63`). A session
stores parent session ID, title, message count, token usage, summary message ID,
cost, todos, and timestamps. Child task sessions are first-class rows via
`CreateTaskSession` (`internal/session/session.go:102`), and agent-tool session
IDs use `messageID$$toolCallID` (`internal/session/session.go:292`).

Messages are persisted as typed JSON parts in SQLite (`internal/message/message.go:23`).
Parts cover text, reasoning, image URL, binary, tool calls, tool results, and
finish markers (`internal/message/message.go:222`). The agent creates and
updates assistant messages incrementally as stream callbacks arrive, then writes
tool-result messages for tool outputs (`internal/agent/agent.go:161`).

The initial migration creates `sessions`, `messages`, and file history tables
(`internal/db/migrations/20250424200609_initial.sql`). Later migrations add
summary messages, session todos, provider fields, and read-file tracking
(`internal/db/migrations/20250810000000_add_is_summary_message.sql`,
`internal/db/migrations/20250812000000_add_todos_to_sessions.sql`,
`internal/db/migrations/20260127000000_add_read_files_table.sql`).

Runtime events use typed pub/sub brokers. `app.setupEvents` bridges session,
message, permission, history, agent notification, MCP, LSP, and skill events
into one TUI-facing broker (`internal/app/app.go:473`). This is a useful
foundation for a desktop/web operations client because state changes already
flow through observable channels.

# Tool registry and execution protocol

Tool registration is in `coordinator.buildTools` (`internal/agent/coordinator.go:460`).
It assembles built-ins, optional LSP tools, optional MCP resource tools, MCP
server tools, filters them through the active agent's `AllowedTools` and
`AllowedMCP`, sorts them by name, and wraps top-level tools with hooks.

Built-ins include `bash`, file read/write/edit/multiedit, grep/glob/ls, fetch,
download, Sourcegraph, todos, LSP diagnostics/references/restart, job output and
kill, Agent Builder info/logs, MCP resource tools, and the `agent` delegation tool
(`internal/config/config.go:661`).

The protocol is `fantasy.AgentTool`: each tool provides schema information and a
`Run` function. The session and assistant message IDs are passed through context
so tools can make permission requests and write consistent tool-result messages
(`internal/agent/agent.go:161`).

Tool outputs are preserved as message tool results and often include metadata.
For example, `bash` returns timings, captured output, working directory,
background status, and shell ID (`internal/agent/tools/bash.go:48`), while edit
tools return additions/removals and old/new content metadata
(`internal/agent/tools/edit.go`).

# Permission, approval, and safety model

The permission service is session-local and interactive. Tools call
`permission.Service.Request`; the service checks global skip mode, allowed tool
patterns, hook pre-approval, per-session auto-approval, and previously granted
persistent permissions before publishing a permission request and waiting for a
grant/deny response (`internal/permission/permission.go:64`,
`internal/permission/permission.go:161`).

There are several approval shortcuts:

- `--yolo` sets `SkipPermissionRequests` during setup
  (`internal/cmd/root.go:247`).
- Non-interactive local runs auto-approve the session
  (`internal/app/app.go:207`).
- `permissions.allowed_tools` can allow tools or tool/action pairs
  (`internal/config/config.go:235`).
- Hooks can return `allow`, which stamps the context with a one-call approval
  (`internal/permission/permission.go:23`).

The safety model is pragmatic rather than hardened. The bash tool bans selected
commands and package-manager/global-install patterns, requires approval for
non-read-only commands, truncates output, and supports backgrounding
(`internal/agent/tools/bash.go:48`). File edit tools require the file to have
been read first and reject edits if the file changed since last read
(`internal/agent/tools/edit.go`). There is no OS sandbox, network isolation,
secret boundary, RBAC, dry-run level, risk tier, or enterprise approval workflow.

# MCP, plugin, skill, and extension model

MCP is the main tool/plugin extension mechanism. Config supports `stdio`, HTTP,
and SSE transports with env/arg/header/url expansion (`internal/config/config.go:183`).
MCP initialization creates client sessions concurrently, records per-server
state, lists tools/prompts/resources, and publishes list/state-change events
(`internal/agent/tools/mcp/init.go:64`, `internal/agent/tools/mcp/init.go:166`).
MCP tools are adapted into Agent Builder tools and permission-gated like built-ins
(`internal/agent/tools/mcp-tools.go:24`, `internal/agent/tools/mcp-tools.go:99`).

Skills implement the Agent Skills `SKILL.md` convention. Agent Builder parses YAML
frontmatter, validates names/descriptions, discovers skill files under builtin,
global, and configured paths, deduplicates with user skills overriding builtins,
filters disabled skills, and injects active skills into the system prompt as XML
(`internal/skills/skills.go:38`, `internal/skills/skills.go:219`,
`internal/skills/skills.go:296`, `internal/agent/coordinator.go:1139`).
`skills.Tracker` records which active skills were loaded during a turn
(`internal/skills/tracker.go:15`).

Hooks are another extension point. Only `PreToolUse` exists today
(`internal/hooks/hooks.go:15`). A hook runner matches tool names by regex,
deduplicates identical commands, executes matching shell commands in parallel,
and aggregates allow/deny/halt/input-rewrite/context outputs
(`internal/hooks/runner.go:36`, `internal/hooks/runner.go:89`,
`internal/hooks/hooks.go:94`).

There is no packaged marketplace/plugin distribution format beyond MCP servers,
skills directories, and config-defined hooks.

# Subagent, task, and scheduler model

Agent Builder has a real subagent primitive, but it is narrow. Default config defines a
`coder` agent with all non-disabled tools and a `task` agent with read-only
tools and no MCP by default (`internal/config/config.go:715`). The `agent` tool
is a `fantasy.NewParallelAgentTool` that creates a task subagent, starts a child
session, runs the subagent non-interactively, and rolls child session cost into
the parent (`internal/agent/agent_tool.go:26`,
`internal/agent/coordinator.go:1064`).

The coordinator interface hints at future multiple-agent support, but there is
currently one current agent plus an agents map; main-agent switching is not
implemented (`internal/agent/coordinator.go:74`). Prompt queuing is per session
inside `sessionAgent`, with cancellation and queue clearing
(`internal/agent/agent.go:86`, `internal/agent/agent.go:1169`).

There is no general scheduler, DAG/state machine, long-running task object,
retry policy, dependency graph, checkpoint model, or cross-agent work planner.
Background shell jobs exist, but they are process/job primitives rather than
agentic workflow tasks (`internal/shell/background.go:15`).

# Sandbox, process, and workspace isolation

Workspace isolation is mostly convention and process boundary, not sandboxing.
The working directory is resolved at startup and passed to config, tools, MCP,
LSP, and shell runners (`internal/cmd/root.go:247`). File tools generally smart
join against the working directory and permission prompts include path context,
but the filesystem is not jailed.

The shell implementation uses `mvdan.cc/sh/v3` POSIX shell emulation across
platforms and injects `AGENT_BUILDER=1`, `AGENT=agent-builder`, and `AI_AGENT=agent-builder` into shell
environments (`internal/shell/shell.go:31`). Background shells are managed by a
singleton manager with a 50-job limit and cleanup/kill APIs
(`internal/shell/background.go:15`).

MCP stdio servers are spawned as local processes with resolved env and args;
HTTP/SSE MCP servers use configured endpoints and headers
(`internal/agent/tools/mcp/init.go:440`). LSP servers are likewise local
process integrations through the LSP manager.

Client/server mode creates a separate long-lived Agent Builder server over Unix socket
or Windows named pipe by default (`internal/server/server.go:47`). That helps
separate UI clients from runtime state, but it is not an execution sandbox.

# Context loading, memory, and compression

Context files are configured through `Options.ContextPaths`, with defaults such
as `.github/copilot-instructions.md`, `CLAUDE.md`, `GEMINI.md`, `AGENT_BUILDER.md`, and
`AGENTS.md` (`internal/config/config.go:28`, `internal/config/config.go:262`).
System prompts are Go templates under `internal/agent/templates`, built via
`agent/prompts.go` and `agent/prompt` (`internal/agent/prompts.go:10`).

The coder prompt includes runtime environment, context files, available skills,
and strict tool/process rules (`internal/agent/templates/coder.md.tpl`). The
task prompt is intentionally terse and read-only oriented
(`internal/agent/templates/task.md.tpl`).

Automatic compression is summarization, not vector memory. The agent watches
token usage against the model context window and triggers summarization when
remaining context falls below either a fixed buffer for large contexts or 20% of
smaller contexts (`internal/agent/agent.go:161`). Summary messages are marked
and the resumed prompt history starts from the summary message
(`internal/agent/agent.go:628`, `internal/agent/agent.go:955`). Todos are
included in summaries via `buildSummaryPrompt` (`internal/agent/agent.go`).

File memory includes file history for edits and a read-file tracker. The edit
safety rule requiring prior read depends on `read_files`
(`internal/db/migrations/20260127000000_add_read_files_table.sql`).

# Client/UI/API boundaries

The most important boundary is `workspace.Workspace`: it defines everything a
frontend needs for sessions, messages, agent control, permissions, file tracker,
history, LSP, config, project init, MCP, events, and shutdown
(`internal/workspace/workspace.go:61`). `AppWorkspace` delegates to in-process
services (`internal/workspace/app_workspace.go:26`), while `ClientWorkspace`
delegates to HTTP client calls (`internal/workspace/client_workspace.go:32`).

The server exposes a broad `/v1` API for health/version, workspaces, events,
providers, sessions, messages, file tracker, LSP, permissions, agent run/cancel,
config mutation, project initialization, and MCP operations
(`internal/server/server.go:87`, `internal/server/server.go:109`). The default
transport is a local Unix socket or Windows named pipe, with TCP also supported.

This boundary is directly relevant to a desktop/web operations client: there is
already a headless-ish local runtime API, but it is shaped around coding
sessions and TUI needs. It lacks operation/task resources, durable audit APIs,
domain object APIs, multi-user auth, and streaming event schemas designed for
external clients.

# Observability, telemetry, tests, and evals

Logs go to `.agent-builder/logs/agent-builder.log`, with debug HTTP clients available for
provider calls. `AGENT_BUILDER_PROFILE` starts pprof on localhost:6060 (`main.go:22`).

Telemetry is opt-out PostHog. `event.Init` configures a Charm endpoint and
pseudonymous distinct ID; event capture merges base properties such as OS,
architecture, terminal, shell, Agent Builder version, Go version, non-interactive mode,
and continuation flags (`internal/event/event.go:50`, `internal/event/event.go:80`).
Metrics are disabled by `AGENT_BUILDER_DISABLE_METRICS`, `DO_NOT_TRACK`, or config
(`internal/cmd/root.go:697`).

Tests are conventional Go tests across packages. There are focused tests for
tools, hooks, MCP, skills, config, file tracking, update, event, shell/file
helpers, and UI components. The project uses `testify` and Catwalk/golden-style
testing where appropriate. There is no evident operations-task eval harness or
agent benchmark suite in this repo.

# Designs worth borrowing

- The `workspace.Workspace` interface is a strong client/runtime seam. It lets
  the same frontend code run against an in-process app or server-backed runtime.
- The `app.App` service graph keeps sessions, messages, permissions, file
  tracking, LSP, MCP, and agent coordinator explicit and testable
  (`internal/app/app.go:79`).
- The tool pipeline is simple and extensible: build all tools, filter per
  agent, add MCP tools, wrap with hooks, expose through `fantasy`.
- Permission requests are typed, evented, and synchronous from the tool's point
  of view, which maps well to client approval UX.
- Hooks are small but useful: allow, deny, halt, context append, and input
  rewrite give a policy/interceptor shape without entangling the agent loop.
- Child sessions for subagents preserve provenance and cost accounting.
- Summary messages and orphaned tool-call repair show practical resilience for
  long conversations and interrupted streams.
- MCP state/events and skill diagnostics give useful extension health signals.

# Gaps or risks for our target product

- No durable operation/task state machine. Sessions and messages are not enough
  for multi-hour, resumable, partially failed operations with rollback.
- No enterprise permission model. Current approvals are local, per-session, and
  tool/path/action scoped, without identity, roles, resources, risk levels,
  approval chains, or policy-as-data.
- No sandbox or isolation boundary suitable for production operations. Shell,
  MCP, LSP, and file access run with local user privileges.
- No workflow/runbook abstraction. The model freely plans and invokes tools;
  deterministic workflow skeletons and required gates would need to be added.
- Subagents are limited to a read-only task agent. There is no scheduler,
  dependency graph, lifecycle policy, or bounded context/resource allocation per
  child agent.
- Client/server API is promising but still local-tool oriented. It needs stable
  external API contracts, auth, operation resources, audit/event schemas, and
  artifact management.
- Audit is incomplete. Tool calls and messages are persisted, but hook decisions,
  permission decisions, risk assessments, artifacts, and external system effects
  are not modeled as an immutable audit log.
- Config is powerful but trusted. Shell expansion in config and hooks is useful
  for developers but risky for enterprise-controlled workspaces.
- Provider/model logic is coupled to coordinator construction. It works, but an
  enterprise runtime may need clearer separation between model policy,
  credentials, tenant config, and execution.

# Implications for Agent Builder-based implementation

Agent Builder is a good base for an agentic operations client if treated as a runtime
foundation, not as the final product shape. The shortest path is to keep the Go
runtime, provider abstraction, session/message persistence, tool protocol, MCP,
hooks, skills, and workspace/server split, then add operations-specific layers.

Recommended implementation direction:

- Promote client/server mode from experimental/local to the default runtime
  boundary for desktop and web clients.
- Add a durable `operations` or `runs` domain next to sessions: state,
  steps, artifacts, approvals, risk level, rollback plan, checkpoints, and final
  report.
- Keep messages as transcript evidence, but make operation events the primary
  audit log.
- Extend permission requests into policy decisions with subject, tenant,
  resource, action, risk, dry-run/write mode, approval requirement, and expiry.
- Add a tool capability manifest: read/write/destructive/network/secret scopes,
  idempotency, timeout, retryability, and artifact output types.
- Wrap MCP and built-in tools behind the same capability/policy layer.
- Evolve `AgentTask` and `agent` into a scheduler-backed subagent model with
  bounded tools, bounded context, parent/child state, cancellation, retries, and
  summarized handoff.
- Add sandbox strategies per tool class: local worktree, container, restricted
  env, network policy, and secret injection policy.
- Preserve the TUI as one client, but design new operation-client APIs around
  runs, events, approvals, artifacts, and reports rather than around chat alone.
