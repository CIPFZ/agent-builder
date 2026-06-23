# Project positioning

Gemini CLI is positioned as a terminal-first developer agent for direct access
to Gemini models, codebase understanding, file edits, shell execution, web
search/fetch, MCP extension, checkpointing, and automation. The README presents
three major modes: interactive terminal use, non-interactive/headless scripting
with `--prompt` and JSON output, and integration surfaces such as GitHub
workflows, IDE companion, MCP servers, and Agent2Agent remote agents
(`README.md`, `docs/cli/headless.md`, `docs/ide-integration/index.md`,
`docs/core/remote-agents.md`).

For the planned Agent Builder-based agentic operations client, the important
positioning is not just "coding assistant"; it is a locally anchored operations
client that can run in a human terminal, a headless automation flow, an IDE
bridge, or an agent-to-agent server. That maps closely to Agent Builder's existing TUI,
CLI, MCP, provider, hook, permission, and session primitives, but Gemini CLI has
a broader product shape around policy, sandbox expansion, extension packaging,
and structured event output.

# Technology stack and repository shape

Gemini CLI is a TypeScript/Node 20 monorepo using npm workspaces (`package.json`).
The main workspaces are:

- `packages/core`: model client, agent loop, tools, scheduler, policy engine,
  sandbox managers, MCP clients, subagents, memory/context, telemetry, storage,
  and shared event types (`packages/core/package.json`).
- `packages/cli`: yargs argument parsing, settings loading, Ink/React terminal
  UI, interactive and non-interactive runners, ACP mode, extension commands,
  skill commands, sandbox relaunch logic, and session commands
  (`packages/cli/package.json`, `packages/cli/src/gemini.tsx`).
- `packages/a2a-server`: HTTP/A2A server wrapper for remote agent execution
  (`packages/a2a-server/src/http/app.ts`, `packages/a2a-server/src/agent/task.ts`).
- `packages/vscode-ide-companion`: VS Code companion extension.
- `integration-tests`, `evals`, `memory-tests`, and `perf-tests`: broad behavior,
  regression, resource, and agent quality coverage.

Dependencies worth noting are `@google/genai` for Gemini APIs,
`@modelcontextprotocol/sdk` for MCP, `@a2a-js/sdk` for Agent2Agent,
OpenTelemetry libraries, `ink`/React for TUI, `execa` and `node-pty` for process
execution, `zod`/JSON schema utilities for validation, and `simple-git`.

# Startup and main loop

Startup is centralized in `packages/cli/src/gemini.tsx`:

- `main()` sets process cleanup, unhandled rejection handling, signal handlers,
  console patching, startup profiling, settings loading, worktree setup, stale
  artifact cleanup, argument parsing, trusted-folder loading, and session ID or
  resume resolution.
- It creates a partial config before sandbox entry so auth and remote admin
  settings can be resolved outside the sandbox.
- If sandboxing is enabled and the process is not already inside a sandbox, it
  relaunches through `start_sandbox()` and exits the parent process.
- After sandbox handling, it loads the final `Config`, initializes storage,
  registers telemetry cleanup, creates policy updater wiring, fires session-end
  hooks on exit, initializes terminal/theme, initializes app services, and then
  selects ACP, interactive UI, or non-interactive execution.

CLI config construction is in `packages/cli/src/config/config.ts`:
`parseArguments()` defines the yargs surface; `loadCliConfig()` merges CLI flags,
settings, trust state, policy, sandbox, model, MCP, extension, hooks, telemetry,
and UI options into a core `Config`.

The model/tool main loop lives in `packages/core/src/core/client.ts` and
`packages/core/src/agent/legacy-agent-session.ts`. `GeminiClient.sendMessageStream()`
fires BeforeAgent hooks, handles context injection, model routing, loop
detection, context overflow checks, compression, IDE context deltas, stream
events, next-speaker continuation, and AfterAgent hooks. `LegacyAgentProtocol`
wraps that into a higher-level `AgentProtocol`, collects tool requests from
model events, calls the `Scheduler`, records completed tool calls back into
chat history, and emits normalized `AgentEvent` values.

# Model/provider abstraction

Gemini CLI is Gemini-centered. The primary provider abstraction is not a
multi-provider interface like Agent Builder's `fantasy`; it is a Gemini content
generator abstraction. Core config imports `createContentGenerator`,
`createContentGeneratorConfig`, `ContentGenerator`, and auth types from
`packages/core/src/core/contentGenerator.ts`, then `GeminiClient.generateContent()`
calls `this.getContentGeneratorOrFail().generateContent(...)`
(`packages/core/src/core/client.ts`).

Model selection is richer than a static model string:

- `loadCliConfig()` resolves the requested model from CLI, env, or settings and
  defaults to an `auto` alias (`packages/cli/src/config/config.ts`).
- `GeminiClient.processTurn()` asks `ModelRouterService` to route each sequence
  unless a current sequence model is already sticky
  (`packages/core/src/core/client.ts`).
- `applyModelSelection()` and availability services handle fallback and launch
  gates (`packages/core/src/core/client.ts`, `packages/core/src/config/models.ts`).
- `ModelConfigService` supports runtime model configs and per-agent override
  scopes (`packages/core/src/services/modelConfigService.ts`,
  `packages/core/src/agents/registry.ts`).

For Agent Builder, the reusable idea is the runtime model configuration and routing
layer, not the provider shape. Agent Builder already has provider abstraction through
`charm.land/fantasy`; a Agent Builder implementation should keep providers generic and
add Gemini-style model routing, per-agent overrides, and fallback policy above
that layer.

# Session, message, and event state

Gemini CLI has two state models:

- The new `AgentProtocol`/`AgentSession` event trajectory in
  `packages/core/src/agent/types.ts` and `packages/core/src/agent/agent-session.ts`.
  It defines durable event types such as `initialize`, `session_update`,
  `message`, `agent_start`, `agent_end`, `tool_request`, `tool_update`,
  `tool_response`, `elicitation_request`, `usage`, `error`, and `custom`.
- The legacy Gemini chat/session model in `GeminiChat`, `Turn`, and
  `ChatRecordingService`, adapted into the event protocol by
  `LegacyAgentProtocol` and `event-translator.ts`
  (`packages/core/src/core/turn.ts`,
  `packages/core/src/agent/legacy-agent-session.ts`,
  `packages/core/src/agent/event-translator.ts`).

Persistence is append-only JSONL rather than a relational database.
`ChatRecordingService` writes initial metadata, message records, metadata
updates via `$set`, and rewind records via `$rewindTo`
(`packages/core/src/services/chatRecordingService.ts`). It stores main chats in
project temp `chats/` and subagent sessions under a parent session directory.
It records messages, thoughts, token summaries, tool calls, summaries,
directories, and can delete session artifacts or rewind to a message ID.

Agent Builder already has SQLite/sqlc session persistence. The event model is still
worth borrowing: a normalized stream/event protocol would give the TUI,
headless JSON output, future API servers, and subagents the same observable
contract instead of each consuming provider-specific message structures.

# Tool registry and execution protocol

Tool definitions follow a declarative pattern:

- `DeclarativeTool`, `BaseDeclarativeTool`, `ToolInvocation`, `ToolResult`, and
  confirmation detail types are in `packages/core/src/tools/tools.ts`.
- Each tool validates parameters, builds a per-call invocation, and separates
  `shouldConfirmExecute()` from `execute()`.
- `ToolRegistry` registers built-ins, discovered command-backed tools, MCP
  tools, aliases, filtering, active/inactive logic, model-specific function
  declarations, plan-mode schema adjustments, and fully qualified MCP names
  (`packages/core/src/tools/tool-registry.ts`).
- `Config.createToolRegistry()` is the core built-in registration site
  (`packages/core/src/config/config.ts`), registering file, search, edit, shell,
  background shell, web, ask-user, plan, tracker, skill activation, MCP resource,
  and subagent tools.

Execution is centralized in `Scheduler` (`packages/core/src/scheduler/scheduler.ts`):

- It accepts one or more `ToolCallRequestInfo` values.
- It queues batches if another batch is active.
- It validates and creates invocations.
- It batches contiguous parallelizable calls; each tool schema gets a
  `wait_for_previous` parameter injected by `DeclarativeTool.addWaitForPreviousParameter()`
  (`packages/core/src/tools/tools.ts`).
- It runs BeforeTool hooks, checks policy, resolves confirmation, updates
  policy for persistent approvals, executes via `ToolExecutor`, handles tail
  tool calls, tracks live output/progress, handles sandbox expansion retries,
  logs telemetry, and returns completed calls.

The protocol distinction between tool request, live update, confirmation, final
response, and model-facing function response is the most useful part for Agent Builder.
Agent Builder has built-in tools and permission checks, but an explicit scheduler state
machine would make parallelism, policy, live progress, and retry behavior easier
to reason about.

# Permission, approval, and safety model

Gemini CLI uses a policy engine as the central approval primitive. The public
reference describes rules with `toolName`, `mcpName`, `subagent`,
`toolAnnotations`, `argsPattern`, `commandPrefix`, `commandRegex`, `decision`,
`priority`, `modes`, and `interactive` fields (`docs/reference/policy-engine.md`).
The implementation is in `packages/core/src/policy/policy-engine.ts`, with
config loading in `packages/core/src/policy/config.ts` and TOML parsing in
`packages/core/src/policy/toml-loader.ts`.

Approval modes are `default`, `autoEdit`, `plan`, and `yolo`
(`packages/core/src/policy/types.ts`). `loadCliConfig()` disables or downgrades
risky modes when YOLO is admin-disabled or the workspace is untrusted
(`packages/cli/src/config/config.ts`). `Scheduler._processToolCall()` is the
enforcement point: hooks can ask or modify input, policy can allow/deny/ask,
confirmation can cancel, and successful "always allow" choices update policy
(`packages/core/src/scheduler/scheduler.ts`).

Default policies live as TOML in `packages/core/src/policy/policies/`, including
`read-only.toml`, `write.toml`, `plan.toml`, `yolo.toml`,
`non-interactive.toml`, `sandbox-default.toml`, `agents.toml`, and
`discovered.toml`. In non-interactive mode, `ask_user` is excluded and
ask-user decisions become denials (`packages/cli/src/config/config.ts`,
`docs/reference/policy-engine.md`).

There is also a safety-checker path for policy contributed by extensions
(`docs/extensions/reference.md`, `packages/core/src/safety/*`) and a ConSeca
checker integration (`packages/core/src/safety/conseca/conseca.ts`).

# MCP, plugin, skill, and extension model

MCP lifecycle is handled by `McpClientManager`
(`packages/core/src/tools/mcp-client-manager.ts`). It tracks configured and
running servers, blocked servers, disabled servers, diagnostics, client keys,
extension-owned servers, resource registries, and coalesced context refreshes.
It starts configured MCP servers only in trusted folders, applies admin
allowlists and required servers, merges extension/user config restrictively
(`includeTools` intersection, `excludeTools` union), and refreshes model context
when tools/resources/instructions change.

Extensions are a first-class package model. `docs/extensions/reference.md`
defines `gemini-extension.json` with `mcpServers`, context files, excluded
tools, settings/env vars, commands, hooks, skills, subagents, policies, safety
checkers, themes, and plan settings. `ExtensionManager` loads, validates,
hydrates, installs, updates, enables, disables, and exposes extensions
(`packages/cli/src/config/extension-manager.ts`).

Skills are discovered by `SkillManager` from built-ins, extensions, user dirs,
and workspace dirs (`packages/core/src/skills/skillManager.ts`,
`docs/cli/using-agent-skills.md`). The `ActivateSkillTool` exposes activation
to the model (`packages/core/src/tools/activate-skill.ts`), and skills are
included in prompt/context providers (`packages/core/src/prompts/promptProvider.ts`,
`packages/core/src/services/memoryService.ts`).

Agent Builder already has MCP and skills. Gemini's extension packaging is broader than
Agent Builder's current local context-file model and would be valuable for agentic
operations: teams can ship operational runbooks, MCP servers, policy, hooks,
subagents, and UI themes as one installable artifact.

# Subagent, task, and scheduler model

Subagents are exposed as a single `invoke_agent` tool implemented by
`AgentTool` (`packages/core/src/agents/agent-tool.ts`). `AgentRegistry` loads
built-in agents, project agents, user agents, and extension agents; applies
settings overrides; registers per-agent model configs; adds dynamic policy for
remote agents; and watches model changes (`packages/core/src/agents/registry.ts`).

Built-ins include codebase investigator, CLI help, generalist, and optional
browser agent (`packages/core/src/agents/codebase-investigator.ts`,
`cli-help-agent.ts`, `generalist-agent.ts`,
`agents/browser/browserAgentDefinition.ts`). User-defined local agents are
Markdown files with YAML frontmatter, documented in `docs/core/subagents.md`.
They support tool allowlists/wildcards, inline MCP servers, model config,
temperature, max turns, and timeout. Remote agents use Agent2Agent and are
loaded from agent cards (`packages/core/src/agents/a2a-client-manager.ts`,
`packages/core/src/agents/remote-invocation.ts`).

Local subagent execution is implemented in `packages/core/src/agents/local-executor.ts`
and `local-invocation.ts`. It creates isolated loop context, tool registry,
MCP manager, scheduler, chat recording, and compression behavior. Recursion is
blocked by not exposing subagent tools to subagents. The scheduler itself is
tool-call oriented rather than cron/job oriented; long-running operational
tasks are represented as agent runs and shell/background process tools rather
than a durable workflow scheduler.

# Sandbox, process, and workspace isolation

Gemini CLI supports both full-process sandbox relaunch and tool-level sandbox
preparation. The docs cover macOS `sandbox-exec`, Docker/Podman, Windows native
sandboxing, gVisor/runsc, and LXC (`docs/cli/sandbox.md`).

Startup relaunch is in `packages/cli/src/gemini.tsx`: before expensive init, it
checks `loadSandboxConfig()` and calls `start_sandbox()`
(`packages/cli/src/utils/sandbox.ts`). The sandbox process receives stdin
injected into `--prompt` when needed.

Tool-level abstraction is `SandboxManager`
(`packages/core/src/services/sandboxManager.ts`), with `prepareCommand()`,
known-safe/dangerous command checks, denial parsing, environment sanitization,
workspace/include/forbidden path resolution, secret-file discovery, governance
file protection, and dynamic `SandboxPermissions`. `Scheduler._execute()` can
detect `sandbox_expansion_required`, prompt for a sandbox expansion, add
additional permissions to the tool args, and retry the command
(`packages/core/src/scheduler/scheduler.ts`). `ShellTool` integrates with these
policies for command execution (`packages/core/src/tools/shell.ts`).

Agent Builder currently has shell execution and permissions, but Gemini's dynamic
sandbox expansion is a strong pattern for operations: start restricted, detect
denied network/path access, ask for a scoped one-run grant, then retry without
making the whole session unsafe.

# Context loading, memory, and compression

Context enters Gemini CLI through several layers:

- Project/user context files such as `GEMINI.md`, configured by context
  settings and loaded before config construction (`packages/cli/src/config/config.ts`,
  `docs/cli/gemini-md.md`).
- Directory and workspace context via `FileDiscoveryService`,
  `WorkspaceContext`, include directories, IDE workspace folders, and
  `.geminiignore`/gitignore-aware filtering (`packages/core/src/config/config.ts`,
  `packages/core/src/services/fileDiscoveryService.ts`).
- Memory services and hierarchical memory import (`packages/core/src/config/memory.ts`,
  `packages/core/src/services/memoryService.ts`).
- Skills and MCP instructions inserted into prompts
  (`packages/core/src/prompts/promptProvider.ts`,
  `packages/core/src/tools/mcp-client-manager.ts`).
- IDE context deltas in `GeminiClient.getIdeContextParts()`
  (`packages/core/src/core/client.ts`).
- Hook-provided additional context wrapped in `<hook_context>` at session,
  agent, and tool stages (`packages/cli/src/gemini.tsx`,
  `packages/core/src/core/client.ts`,
  `packages/core/src/scheduler/hook-utils.ts`).

Compression is explicit and automatic. `GeminiClient.tryCompressChat()` calls
`ChatCompressionService.compress()` and handles compressed history, failed
inflation, token-count failures, empty summaries, and truncation
(`packages/core/src/core/client.ts`,
`packages/core/src/context/chatCompressionService.ts`). Tool-output masking and
distillation are separate mechanisms to reduce bulky tool outputs
(`packages/core/src/context/toolOutputMaskingService.ts`).

# Client/UI/API boundaries

Gemini CLI has a clearer boundary than many CLIs:

- Core owns agent loop, tools, policy, telemetry, storage, MCP, agents, memory,
  and typed event surfaces.
- CLI owns argument parsing, settings files, terminal UI, interactive commands,
  extension management commands, sandbox relaunch, ACP transport, and
  non-interactive formatting.
- `AgentProtocol`/`AgentSession` is an emerging API boundary with event
  replay/resume (`packages/core/src/agent/types.ts`,
  `packages/core/src/agent/agent-session.ts`).
- Non-interactive output supports text, JSON, and streaming JSON
  (`packages/cli/src/nonInteractiveCli.ts`,
  `packages/core/src/output/json-formatter.ts`,
  `packages/core/src/output/stream-json-formatter.ts`).
- ACP mode is launched from `main()` via `runAcpClient()`
  (`packages/cli/src/gemini.tsx`, `packages/cli/src/acp/*`).
- A2A server mode wraps the agent in an HTTP/task server
  (`packages/a2a-server/src/http/app.ts`, `packages/a2a-server/src/agent/task.ts`).

For Agent Builder, this suggests making the agent/session/event layer explicitly
headless and UI-independent. The Bubble Tea TUI should consume the same event
stream as an API client or JSON runner.

# Observability, telemetry, tests, and evals

Telemetry is OpenTelemetry-based and supports local or GCP targets
(`docs/cli/telemetry.md`, `packages/core/src/telemetry/index.ts`,
`packages/core/src/telemetry/sdk.ts`). It logs configuration, prompts, API
requests/responses/errors, tool calls, tool output truncation, file operations,
model routing, chat compression, retries, extension lifecycle, agent runs,
approval mode changes/durations, IDE connections, hooks, safety verdicts, and
startup stats. Metrics cover sessions, tool count/latency, token usage, API
count/latency, file operations, model routing, agent duration/turns, UI flicker,
memory, CPU, event loop delay, and queue depth. Trace spans are used around
scheduling and agent calls (`packages/core/src/scheduler/scheduler.ts`,
`packages/core/src/agents/agent-tool.ts`).

Testing is broad:

- Unit tests beside implementation throughout `packages/core/src` and
  `packages/cli/src`.
- Integration tests for browser agent, hooks, MCP resources, policies,
  concurrency limits, shell background jobs, JSON output, sessions, plan mode,
  checkpointing, and file tools (`integration-tests/*`).
- Evals for behavior such as subagents, planning, memory, shell safety,
  concurrency, grep/search, tool output masking, and automated tool use
  (`evals/*`).
- Memory and performance regression suites with baselines (`memory-tests/*`,
  `perf-tests/*`).

Agent Builder should borrow the test taxonomy: unit tests for core invariants,
integration response snapshots for CLI behavior, resource/perf tests for
long-running clients, and evals for agent behavior that unit tests cannot
capture.

# Designs worth borrowing

- A normalized `AgentProtocol` event stream with replay/resume semantics
  (`packages/core/src/agent/types.ts`, `agent-session.ts`).
- A central scheduler state machine for tool validation, policy, confirmation,
  execution, live output, parallel batches, tail calls, and sandbox expansion
  (`packages/core/src/scheduler/scheduler.ts`).
- Declarative tools that separate schema, validation, invocation, confirmation,
  execution, display, and model-facing result (`packages/core/src/tools/tools.ts`).
- Policy engine as the source of truth for allow/deny/ask decisions, including
  mode-aware and subagent/MCP-aware rules (`packages/core/src/policy/*`).
- Dynamic sandbox expansion for scoped retry after denied permissions
  (`packages/core/src/scheduler/scheduler.ts`,
  `packages/core/src/services/sandboxManager.ts`).
- Unified subagent invocation through one tool, with isolated loop context,
  tool allowlists, inline MCP, and per-agent model config
  (`packages/core/src/agents/*`).
- Extension packages that bundle MCP, commands, skills, agents, policies,
  settings, and hooks (`docs/extensions/reference.md`).
- Coalesced MCP context refresh and restrictive merging of extension/user MCP
  configs (`packages/core/src/tools/mcp-client-manager.ts`).
- Append-only session/event recording with rewind markers, even if Agent Builder keeps
  SQLite as the storage runtime (`packages/core/src/services/chatRecordingService.ts`).
- OpenTelemetry events and metrics for every major agent/tool/model boundary
  (`docs/cli/telemetry.md`).

# Gaps or risks for our target product

- Provider abstraction is Gemini-specific. Agent Builder should not copy this directly
  because it already supports multiple providers through `fantasy`.
- The repository has a legacy/new split: `LegacyAgentProtocol` adapts the older
  Gemini loop into the new `AgentProtocol` (`packages/core/src/agent/legacy-agent-session.ts`).
  That indicates the event API is still settling.
- Policy is powerful but complex. Admin/user/workspace/default tiers, dynamic
  rules, extensions, allow/deny/ask, modes, command regex, MCP FQNs, and
  subagent aliases will need careful UX if ported.
- Workspace policy is documented as currently disabled in
  `docs/reference/policy-engine.md`, so this area has product and trust-model
  churn.
- Full-process sandbox relaunch is tightly coupled to Node and local CLI
  startup. Agent Builder in Go should design sandboxing independently, likely around
  command/tool execution first.
- Extension installation and execution carries supply-chain risk. Gemini
  mitigates with consent, trust folders, env sanitization, extension policy
  restrictions, and admin controls, but the attack surface remains large
  (`docs/extensions/reference.md`).
- Browser agent is powerful but high risk: persistent profiles, file upload,
  script evaluation, domain restrictions, and user-input blocking require
  dedicated product treatment (`docs/core/subagents.md`,
  `packages/core/src/agents/browser/*`).
- JSONL session storage is simple but may not fit multi-client, concurrent, or
  query-heavy operational dashboards. Agent Builder's SQLite base is better for that.
- The codebase contains many feature flags and experimental paths, increasing
  integration ambiguity for anything copied wholesale.

# Implications for Agent Builder-based implementation

Agent Builder should keep its Go architecture and `fantasy` provider abstraction, but
lift several architectural patterns:

- Define a core agent event stream in Go that can drive Bubble Tea, headless
  JSON/stream-json, future API servers, and subagents. Map existing Agent Builder
  `message`, `session`, `pubsub`, and UI events into this contract rather than
  letting UI-specific structures become the API.
- Introduce a scheduler layer between model tool calls and tool execution. It
  should own validation, hook execution, permission/policy decisions,
  confirmation, live updates, parallel batching, cancellation, telemetry, and
  final model responses.
- Evolve Agent Builder permissions into a mode-aware policy engine. Start smaller than
  Gemini: tool name, agent name, MCP server, command prefix/regex, args pattern,
  mode, and interactive/headless are enough for the first version.
- Preserve Agent Builder hooks, but align their placement with Gemini's BeforeAgent,
  AfterAgent, SessionStart/End, and BeforeTool separation so operational teams
  can inject policy/context without modifying providers.
- Add subagents as isolated Agent Builder agents exposed through a single delegation
  tool. Use existing `internal/agent/coordinator.go` concepts, but add
  per-subagent tool/MCP/model limits and prevent recursive delegation by
  construction.
- Prefer tool-level sandboxing and scoped sandbox expansion before attempting a
  full-process sandbox relaunch. This fits Go/Agent Builder and operations workflows
  better.
- Treat extensions as a later packaging layer over already-stable primitives:
  MCP servers, skills, hooks, policies, commands, and agents. Do not make
  extension loading the first dependency for core execution.
- Keep SQLite for sessions, but add an append-only event/audit table or JSON
  event payloads to support replay, rewind, telemetry correlation, and external
  clients.
- Add telemetry hooks at the same seams Gemini instruments: startup phases,
  model request/response/error, tool lifecycle, permission decision, policy
  update, hook call, session start/end, compression, and subagent run.
- Build tests around behavior, not just packages: policy enforcement,
  non-interactive denial, MCP lifecycle, subagent isolation, sandbox expansion,
  cancellation, output streaming, and context compression should each have
  integration coverage.
