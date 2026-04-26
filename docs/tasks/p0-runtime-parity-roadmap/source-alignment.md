# P0 Runtime Parity Source Alignment

Date: 2026-04-26

## 1. Alignment Basis

This roadmap is based on:

- `docs/claude-code-go-parity-semantic-review.md`
- `claude-code/docs/03-startup-and-main-loop.md`
- `claude-code/docs/04-command-system.md`
- `claude-code/docs/05-tool-system.md`
- `claude-code/docs/07-query-engine-and-context.md`
- `claude-code/docs/08-permissions-and-safety.md`
- `claude-code/docs/11-agents-and-tasks.md`
- `claude-code/docs/16-session-persistence-and-recovery.md`
- `claude-code/docs/17-system-prompt-and-model-resolution.md`
- `claude-code/docs/19-context-compression-and-history-management.md`
- `claude-code/docs/21-memory-and-claude-md.md`
- `claude-code/docs/22-cli-structured-io-and-transports.md`

## 2. P0.1 Tool Parity Core

Claude source areas:

- `claude-code/src/Tool.ts`
- `claude-code/src/tools.ts`
- `claude-code/src/tools/`
- `claude-code/src/tools/AgentTool/AgentTool.tsx`
- `claude-code/src/utils/permissions/`
- `claude-code/src/QueryEngine.ts`

Go source areas:

- `internal/tools/registry.go`
- `internal/tools/filesystem_tools.go`
- `internal/tools/extended_tools.go`
- `internal/tools/agent_tool.go`
- `internal/tools/skill_discovery.go`
- `internal/tools/skill_frontmatter.go`
- `internal/tools/system/run.go`
- `internal/queryengine/queryengine.go`
- `internal/permissions/policy.go`
- `internal/approval/manager.go`

Alignment requirements:

- tool calls are controlled execution protocol, not helper functions
- permission evaluation happens before execution
- tool identity is stable across tool call and tool result
- structured input and observable input are distinct where needed
- failures produce model-consumable tool result semantics

## 3. P0.2 Command Registry

Claude source areas:

- `claude-code/src/commands.ts`
- `claude-code/src/commands/`
- `claude-code/src/components/PromptInput/`
- `claude-code/src/screens/`

Go source areas:

- `internal/app/cli.go`
- `internal/tui/commands.go`
- `internal/tui/model.go`
- `internal/queryengine/queryengine.go`
- possible new package: `internal/commands`

Alignment requirements:

- slash commands are registered capabilities
- command visibility is runtime-dependent
- command execution is not hard-coded in one UI client
- commands can produce immediate messages or model queries

## 4. P0.3 Context, Memory, And Recovery

Claude source areas:

- `claude-code/src/context.ts`
- `claude-code/src/history.ts`
- `claude-code/src/assistant/sessionHistory.ts`
- `claude-code/src/memdir/`
- `claude-code/src/state/AppStateStore.ts`
- `claude-code/src/QueryEngine.ts`
- `claude-code/docs/33-session-memory-scheduling-and-concurrency.md`
- `claude-code/docs/34-history-snip-and-replay-projection.md`
- `claude-code/docs/35-claude-md-loading-and-instruction-assembly.md`
- `claude-code/docs/38-read-file-state-and-context-cache-mechanics.md`

Go source areas:

- `internal/workspace/loader.go`
- `internal/prompt/builder.go`
- `internal/memory/service.go`
- `internal/session/manager.go`
- `internal/session/recovery.go`
- `internal/store/file/session_store.go`
- `internal/model/claude_transcript.go`
- `internal/runtime/session_compaction.go`
- `internal/queryengine/context_provider.go`

Alignment requirements:

- workspace instructions are explicit context, not incidental file reads
- transcript blocks preserve tool-use identity
- compaction boundaries are first-class recovery objects
- memory and context cache are separate concepts
- recovery restores enough runtime state for continuation

## 5. P0.4 Runtime Structured Events

Claude source areas:

- `claude-code/src/cli/structuredIO.ts`
- `claude-code/src/cli/transports/`
- `claude-code/src/entrypoints/sdk/`
- `claude-code/src/QueryEngine.ts`
- `claude-code/src/Tool.ts`

Go source areas:

- `internal/runtime/runner.go`
- `internal/queryengine/queryengine.go`
- `internal/gateway/server.go`
- `internal/protocol/ws/message.go`
- `internal/tui/runtime_bridge.go`

Alignment requirements:

- runtime events are client-neutral
- gateway is transport, not business logic owner
- permission prompts, tool lifecycle, command lifecycle, and compaction lifecycle are machine-readable
- event payloads are stable enough for future SDK and React clients

