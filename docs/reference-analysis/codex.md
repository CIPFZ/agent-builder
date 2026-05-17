# Project positioning

Codex is positioned as a local-first coding agent with multiple front doors:
interactive terminal UI, non-interactive `exec`, an IDE/desktop-oriented
app-server, an MCP server mode, and SDKs. The repository README frames Codex CLI
as "a coding agent from OpenAI that runs locally on your computer"
(`README.md`), while `codex-rs/README.md` says the Rust implementation is the
maintained CLI and highlights `core/`, `exec/`, `tui/`, and `cli/` as the main
crates.

The architectural center is not the CLI parser. It is `codex-rs/core`, which
contains the session loop, tool orchestration, model client, context handling,
subagents, and sandbox policy. Client surfaces are comparatively thin:
`codex-rs/cli/src/main.rs` multiplexes subcommands, `codex-rs/tui` hosts the
terminal experience, `codex-rs/app-server` exposes JSON-RPC for app clients, and
`sdk/python` plus `sdk/typescript` wrap app-server semantics for external
programmatic use.

# Technology stack and repository shape

The reference repo is a mixed Rust, TypeScript, Python, Bazel, and pnpm
workspace. The maintained product is the Rust workspace under `codex-rs`:

- `codex-rs/core`: core agent runtime and business logic.
- `codex-rs/protocol`: internal submission/event protocol, model item types,
  approvals, permissions, and serialization.
- `codex-rs/tools`: shared tool schema primitives and tool metadata.
- `codex-rs/app-server` and `codex-rs/app-server-protocol`: JSON-RPC API,
  transport, typed protocol, and integration tests.
- `codex-rs/tui`: Ratatui-based terminal UI and snapshot tests.
- `codex-rs/exec`: headless/non-interactive CLI.
- `codex-rs/thread-store`, `codex-rs/message-history`, and `codex-rs/state`:
  JSONL/thread state, prompt history, SQLite state and logs.
- `codex-rs/codex-mcp`, `codex-rs/core-plugins`, `codex-rs/core-skills`,
  `codex-rs/plugin`: extension system.
- `codex-rs/sandboxing`, `codex-rs/linux-sandbox`, and
  `codex-rs/windows-sandbox-rs`: process isolation.
- `codex-rs/model-provider`, `codex-rs/model-provider-info`,
  `codex-rs/models-manager`, and `codex-rs/codex-api`: provider catalog,
  auth, model metadata, HTTP/SSE/WebSocket clients.

The CLI package `codex-cli` is mostly distribution glue, including
`codex-cli/bin/codex.js` and install scripts. The root docs in `docs/` mostly
redirect to hosted developer docs, so the concrete implementation details are
in source.

# Startup and main loop

`codex-rs/cli/src/main.rs` defines a single `MultitoolCli` and routes to
interactive TUI, `exec`, `review`, auth, MCP, plugin, app-server, sandbox, and
debug subcommands. The interactive default is treated as the no-subcommand path.

The core runtime starts through `Codex::spawn` in
`codex-rs/core/src/session/mod.rs`. `Codex` is explicitly documented as a
submission queue/event queue interface. It owns `tx_sub`, `rx_event`, an
`AgentStatus` watch receiver, a `Session`, and a shared termination future.
`Codex::spawn_internal` creates async channels, loads plugins and skills,
loads AGENTS.md instructions, prepares exec policy, resolves the default model,
creates session services, and starts the background submission loop.

The session loop itself is split across `codex-rs/core/src/session/handlers.rs`
and `codex-rs/core/src/session/turn.rs`. `run_turn` in `turn.rs` is the main
sampling loop: it builds prompt state, streams Responses API events, records
items, detects tool calls, runs tool calls, appends tool outputs, handles
compaction, and emits terminal turn events. `codex-rs/protocol/src/protocol.rs`
defines the `Submission`, `Op`, `Event`, and `EventMsg` types exchanged across
the queue.

# Model/provider abstraction

Provider support is split cleanly:

- `codex-rs/model-provider-info/src/lib.rs` defines `ModelProviderInfo`,
  built-in provider IDs, base URLs, auth options, retry settings, WebSocket
  support, and the `WireApi` enum. Current wire support is the Responses API.
- `codex-rs/model-provider/src/lib.rs` exports `ModelProvider`,
  `ProviderCapabilities`, and auth provider construction.
- `codex-rs/models-manager/src/manager.rs` owns model catalog refresh,
  `models_cache.json`, ETag handling, bundled model metadata, visibility
  filtering, default model selection, and collaboration mode presets.
- `codex-rs/core/src/client.rs` owns actual request execution through
  `ModelClient` and per-turn `ModelClientSession`.

`ModelClient` is session-scoped and carries auth, provider, thread/session IDs,
installation ID, WebSocket fallback state, and telemetry configuration.
`ModelClientSession` is turn-scoped and caches the Responses WebSocket
connection plus the `x-codex-turn-state` sticky-routing token. The source is
explicit that reusing a `ModelClientSession` across turns is invalid.

For a Crush implementation, this argues for separating provider catalog/model
metadata from the streaming turn client. Crush already uses `charm.land/fantasy`
as the provider abstraction; Codex suggests adding a layer above provider calls
for model catalogs, capabilities, transport selection, sticky per-turn state,
and request telemetry.

# Session, message, and event state

Codex distinguishes runtime session state, replayable thread state, global
message history, and operational logs:

- Runtime session state lives in `Session` under
  `codex-rs/core/src/session/session.rs` and session services/state under
  `codex-rs/core/src/state`.
- Thread-facing state is wrapped by `CodexThread` in
  `codex-rs/core/src/codex_thread.rs`, which exposes `submit`, `steer_input`,
  shutdown, memory mode, app-server client info, and turn-context override
  validation.
- Thread persistence contracts are in `codex-rs/thread-store/src/types.rs`.
  The model includes `CreateThreadParams`, `ResumeThreadParams`,
  `StoredThread`, `StoredTurn`, item pagination, archive/unarchive, metadata
  patches, fork provenance, token usage, git info, approval mode, and sandbox
  policy.
- Prompt history is a separate append-only JSONL file in
  `codex-rs/message-history/src/lib.rs`, stored at
  `~/.codex/history.jsonl` with file locking and size trimming.
- SQLite state and logs are in `codex-rs/state/src/runtime.rs` and
  `codex-rs/state/src/log_db.rs`. The state runtime opens WAL-mode SQLite
  databases under Codex home, runs migrations, and keeps logs in a separate DB
  to reduce lock contention.

The internal protocol is item-oriented. `codex-rs/protocol/src/protocol.rs`
defines `EventMsg` variants such as `TurnStarted`, `TurnComplete`,
`ExecApprovalRequest`, `ApplyPatchApprovalRequest`, `RequestPermissions`, MCP
startup, raw response items, warnings, token usage, and tool lifecycle events.
This event stream is the main integration seam for UIs and app-server.

# Tool registry and execution protocol

Codex treats tools as first-class typed runtimes. Shared schema primitives live
in `codex-rs/tools/src/lib.rs`: `ToolSpec`, `ResponsesApiTool`,
`FreeformTool`, `ResponsesApiNamespace`, `ToolDefinition`, `ToolPayload`,
`ToolOutput`, `ToolExecutor`, and `ToolsConfig`.

Core tool execution is layered:

- `codex-rs/core/src/tools/router.rs` converts model `ResponseItem`s into
  `ToolCall`s and dispatches them.
- `codex-rs/core/src/tools/registry.rs` stores `CoreToolRuntime`s by
  `ToolName`, exposes model-visible specs, hook payloads, telemetry tags,
  argument-diff consumers, and post-tool payloads.
- `codex-rs/core/src/tools/parallel.rs` runs tool calls with cancellation and
  a read/write lock: tools that support parallel calls share a read lock, while
  non-parallel tools take the write lock.
- Tool implementations live under `codex-rs/core/src/tools/handlers`, including
  shell/unified exec, apply patch, view image, plan, request permissions,
  request user input, tool search, MCP, plugin install, and multi-agent tools.

`ToolOutput` converts handler results back into model input items, with
different shapes for function-call output, custom tool output, tool-search
output, MCP output, apply-patch output, and aborted tools
(`codex-rs/core/src/tools/context.rs`). This is a useful separation for Crush:
tool implementations should return a structured result that the conversation
layer formats into model-visible content and UI-visible events.

# Permission, approval, and safety model

Codex centralizes approval and sandbox execution in
`codex-rs/core/src/tools/orchestrator.rs` and
`codex-rs/core/src/tools/sandboxing.rs`. The orchestrator sequence is:
determine approval requirement, request approval if needed, select sandbox,
run the tool, handle sandbox/network denials, and optionally retry after
approval. Approval decisions can be cached through `ApprovalStore` so
"approved for session" avoids repeated prompts.

`ExecApprovalRequirement` has `Skip`, `NeedsApproval`, and `Forbidden` states.
The default requirement is derived from `AskForApproval` and
`FileSystemSandboxPolicy`. Permission profiles are richer than the legacy
`SandboxPolicy`: `TurnContext` exposes `permission_profile`,
`file_system_sandbox_policy`, `network_sandbox_policy`, and a compatibility
`sandbox_policy` (`codex-rs/core/src/session/turn_context.rs`).

Safety is broader than prompt approval. The code includes:

- Guardian review routing in `codex-rs/core/src/guardian` and orchestration
  hooks in `tools/orchestrator.rs`.
- Command policy support in `codex-rs/core/src/exec_policy.rs`.
- Network policy and managed proxy handling in
  `codex-rs/core/src/network_policy_decision.rs` and
  `codex-rs/core/src/tools/network_approval`.
- MCP approval templates and app connector policy in
  `codex-rs/core/src/mcp_tool_call.rs`.
- Hook integration before permission requests and tool use in
  `codex-rs/core/src/hook_runtime.rs`.

Crush already has permissions and hooks. The borrowable idea is a single
tool-orchestration layer that owns approval, sandbox choice, retry semantics,
telemetry, and hook evaluation instead of spreading those decisions across
individual tools.

# MCP, plugin, skill, and extension model

Codex has four extension lanes:

1. MCP client/server integration. `codex-rs/codex-mcp/src/connection_manager.rs`
   owns running MCP clients, startup status events, tool/resource/template
   aggregation, elicitation management, auth status, and shutdown.
   `codex-rs/core/src/mcp_tool_call.rs` handles MCP tool approvals, app
   connector policy, lifecycle events, argument rewriting, result formatting,
   and metrics.
2. Plugins. `codex-rs/core-plugins/src/loader.rs` loads configured plugins
   from config layers and marketplaces, reads `.mcp.json`, hook config, app
   config, skill roots, and handles duplicate MCP server names. Marketplace
   add/remove/upgrade/install flows live in the same crate.
3. Skills. `codex-rs/core-skills/src/loader.rs` discovers `SKILL.md` files
   from repo, user, system, admin, and plugin roots, parses YAML frontmatter
   and `agents/openai.yaml`, validates metadata, policies, and dependencies,
   and enforces scan depth. `codex-rs/core-skills/src/manager.rs` caches
   outcomes by cwd and effective config.
4. Native extension contributors. `codex-rs/core/src/tools/router.rs` asks
   `ExtensionRegistry` contributors for tool executors, while session lifecycle
   hooks are called from `CodexThread` in `codex-rs/core/src/codex_thread.rs`.

Skill invocation has explicit and implicit paths. `core-skills/src/injection.rs`
collects `$skill` mentions, structured skill inputs, plugin paths, and MCP path
mentions, then injects selected `SKILL.md` bodies. This is close to Crush's
current skill system, but Codex adds product policy, plugin provenance, tool
dependencies, and UI toggles.

# Subagent, task, and scheduler model

Subagents are implemented as additional Codex threads scoped under a shared
`AgentControl`. `codex-rs/core/src/agent/control.rs` defines `AgentControl`,
`LiveAgent`, `SpawnAgentOptions`, fork modes, spawn-slot reservations,
metadata, inherited shell snapshots, inherited exec policy, and inter-agent
communication. It uses a weak pointer to `ThreadManagerState` to avoid
reference cycles.

The model-visible multi-agent surface is in
`codex-rs/core/src/tools/handlers/multi_agents_v2`. It exposes `spawn_agent`,
`send_message`, `followup_task`, `wait_agent`, `list_agents`, and
`close_agent`; tool specs are generated in
`codex-rs/core/src/tools/handlers/multi_agents_spec.rs`. A spawned agent can
start from a fresh task or from forked parent history, with `FullHistory` or
`LastNTurns` modes.

There is also a job-like batch lane in
`codex-rs/core/src/tools/handlers/agent_jobs.rs` and related files, with
state-backed agent jobs in `codex-rs/state/src/runtime.rs`. This is relevant
for agentic operations clients: Codex separates live conversational subagents
from persisted jobs/results, which avoids forcing every background operation
to be represented only as a chat thread.

# Sandbox, process, and workspace isolation

Codex supports multiple sandbox backends and abstracts them through
`codex-rs/sandboxing/src/lib.rs` and `SandboxManager`. Platform-specific
backends include Linux Landlock/bubblewrap, macOS seatbelt, and Windows
sandboxing. Windows has both legacy restricted-token and elevated backends for
unified exec in `codex-rs/windows-sandbox-rs/src/unified_exec/mod.rs`.

Process execution is in `codex-rs/core/src/exec.rs`. It defines `ExecParams`,
timeouts, cancellation, output caps, live output deltas, process-group killing,
environment handling, network proxy enforcement, Windows sandbox filesystem
overrides, and sandbox selection. Output is capped to protect the agent process
from unbounded stdout/stderr.

Workspaces are represented explicitly. `TurnContext` carries resolved turn
environments and a primary cwd. App-server APIs expose thread and turn
environment selections in
`codex-rs/app-server-protocol/src/protocol/v2/thread.rs` and
`codex-rs/app-server-protocol/src/protocol/v2/turn.rs`. Permission profiles
can materialize symbolic workspace roots, and app-server turn start can
override cwd, runtime workspace roots, sandbox, permissions, and model.

# Context loading, memory, and compression

AGENTS.md loading is implemented in `codex-rs/core/src/agents_md.rs`. It walks
from a project root to cwd, reads `AGENTS.override.md` before `AGENTS.md`,
supports configured fallback filenames, respects a byte budget, and returns
instruction source paths for clients. Global Codex-home instructions are also
supported.

Conversation context is managed by `codex-rs/core/src/context_manager`. It
tracks model-visible history and truncates function outputs for context
budgeting. Compaction is handled in `codex-rs/core/src/compact.rs`; it can run
manual or auto compaction, emits compaction lifecycle events, uses a
summarization prompt, replaces history with compacted history, records
analytics, and resets the WebSocket session after compaction.

Memory is split into read/write/MCP crates under `codex-rs/memories`. Memory
usage is detected from shell-like commands in
`codex-rs/core/src/memory_usage.rs`, and memory mode is persisted on threads
through `CodexThread::set_thread_memory_mode`. The app-server includes
`memory_reset` and thread memory mode APIs in tests and request processors.

# Client/UI/API boundaries

The cleanest public boundary is app-server. `codex-rs/app-server/src/main.rs`
starts a JSON-RPC server over stdio, Unix sockets, WebSocket, or off. It has
session-source selection, strict config mode, WebSocket auth, plugin startup
tasks, and optional remote control.

`codex-rs/app-server/src/lib.rs` runs two loops: a processor loop for incoming
JSON-RPC and request dispatch, and an outbound loop for per-connection writes.
`codex-rs/app-server/src/request_processors.rs` maps JSON-RPC methods to core
operations for threads, turns, models, MCP resources/tools, plugin management,
skills, hooks, command exec, config, account auth, feedback, realtime, review,
and permissions.

The typed v2 API is in `codex-rs/app-server-protocol/src/protocol/v2`.
`thread.rs` defines `thread/start`, `thread/resume`, fork, read, list, archive,
rollback, shell command, dynamic tools, environments, and permissions.
`turn.rs` defines `turn/start`, `turn/steer`, `turn/interrupt`, multimodal
`UserInput`, output schemas, model/effort overrides, and collaboration mode.

The TUI consumes the same underlying core concepts and has extensive snapshot
coverage under `codex-rs/tui/src/**/snapshots`. For Crush, an operations
client should probably not bind directly to Bubble Tea internals; a stable API
layer around sessions, turns, tools, approvals, and events is the more durable
boundary.

# Observability, telemetry, tests, and evals

Codex uses multiple observability channels:

- `tracing` spans and events throughout core, app-server, tools, MCP, and
  client code.
- `codex-rs/otel` for session telemetry and metrics such as thread started,
  tool calls, compaction, MCP calls, and unified exec usage.
- `codex-rs/analytics` for product analytics, accepted-line fingerprints, and
  structured event payloads.
- Local SQLite log capture in `codex-rs/state/src/log_db.rs`, with a bounded
  background queue and batched inserts.
- App-server analytics and JSON-RPC error tracking in
  `codex-rs/app-server/src/request_processors.rs`.

Tests are broad and behavioral. Core has unit/integration tests beside source
files, for example `codex-rs/core/src/session/tests.rs`,
`codex-rs/core/src/tools/handlers/multi_agents_tests.rs`, and
`codex-rs/core/src/exec_policy_tests.rs`. App-server has high-level JSON-RPC
tests under `codex-rs/app-server/tests/suite/v2`, covering thread lifecycle,
turn lifecycle, permissions, MCP, plugins, skills, command exec, config,
realtime, and safety downgrade flows. TUI uses snapshot tests under
`codex-rs/tui/src/**/snapshots`.

I did not find a single central eval harness comparable to product eval suites
in the inspected paths. The repository emphasizes deterministic protocol,
runtime, and UI tests over benchmark-style agent evals.

# Designs worth borrowing

- Queue-pair core API: `Codex` as submission queue plus event queue is a clean
  boundary for TUI, app-server, and non-interactive clients.
- App-server protocol: typed JSON-RPC with `thread/*`, `turn/*`, `mcp/*`,
  `plugin/*`, `skills/*`, and `commandExec/*` groups gives clients a stable
  surface that avoids embedding UI concerns in the agent core.
- Central tool orchestrator: approvals, sandboxing, retry, network denial, and
  guardian review sit in one path instead of being duplicated per tool.
- Permission profiles: richer than a small enum, but still mappable to legacy
  sandbox modes for compatibility.
- Tool output abstraction: handler output is separate from model-visible
  response items, UI lifecycle events, telemetry preview, and post-tool hooks.
- Thread store abstraction: in-memory and local stores share the same
  contracts, enabling tests, app-server pagination, resume, fork, archive, and
  metadata repair.
- Subagents as threads: this reuses the same turn loop, permissions, storage,
  and event model instead of inventing a separate worker runtime.
- Skills/plugins/MCP provenance: extensions carry source and policy metadata,
  which matters for enterprise controls and UI trust indicators.
- Explicit environments: thread and turn environments make remote/local
  workspace operations a first-class concept.

# Gaps or risks for our target product

- Complexity is high. Codex has many crates and feature gates; copying the
  structure directly into Crush would likely slow delivery.
- Provider abstraction is OpenAI Responses centric. Crush's provider matrix is
  broader through fantasy, so Codex's `ModelClient` design must be translated,
  not imported as-is.
- The app-server is large and product-specific. Its account, ChatGPT, apps,
  marketplace, remote-control, and realtime APIs are more than a first
  operations client likely needs.
- Safety has several overlapping systems: permission profiles, approval policy,
  guardian review, execpolicy, network policy, hooks, MCP approval metadata,
  and app connector policy. This is powerful but can be hard for users to
  predict.
- The plugin marketplace model implies distribution, trust, update, and
  provenance work. For Crush, local plugins/skills/MCP may be enough initially.
- Windows sandboxing is substantial engineering. If Crush targets parity, this
  becomes a dedicated workstream, not a small feature.
- Subagent concurrency needs operational limits. Codex has max threads, depth,
  and timeout settings; Crush should avoid unbounded background agent spawning.
- JSONL plus SQLite plus history files create migration and repair concerns.
  The operational client should decide which state is authoritative before
  adding multiple persistence channels.

# Implications for Crush-based implementation

Crush already has useful primitives: Go implementation, Bubble Tea UI,
SQLite/sqlc sessions, config service, built-in tools, hooks, permission
checking, MCP integration, LSP support, and skills. The Codex reference suggests
the next layer for an agentic operations client should be a stable core API
around sessions, turns, events, tool calls, approvals, and persisted thread
state.

Recommended implementation direction:

- Add a client-facing protocol boundary before adding another UI. A JSON-RPC or
  local socket API modeled after Codex `thread/*` and `turn/*` would let a web,
  desktop, or operations client attach without depending on TUI internals.
- Keep Crush's provider abstraction, but add a capability/model metadata layer
  that exposes model features, context window, tool support, reasoning options,
  and transport behavior to the turn builder.
- Refactor tool execution toward a central orchestrator. Crush's hooks and
  permissions should converge with sandbox selection, approval caching, output
  truncation, and telemetry in one path.
- Treat subagents as sessions or child sessions, not a separate execution
  concept. Store parent/child edges, status, last task message, depth, role,
  and limits in SQLite.
- Start with local plugins/skills/MCP provenance, not marketplace sync. The
  important operational need is knowing which extension provided which tool,
  which permissions it needs, and whether it is enabled.
- Preserve a compact event stream. Codex's event vocabulary is extensive; Crush
  can start with session configured, turn started/completed, assistant/user
  message items, tool started/completed, approval requested/resolved,
  permission changes, MCP startup, warnings, and token usage.
- Make environments/workspaces explicit. Even if Crush initially runs local
  only, the protocol should carry cwd, workspace roots, environment id, shell,
  and sandbox profile so remote workers or isolated workspaces fit later.
- Use SQLite as the main operational state store. Crush already has sqlc
  migrations; prefer extending that over adding separate authoritative JSONL
  thread stores unless JSONL is needed for interoperability or append-only
  audit export.
