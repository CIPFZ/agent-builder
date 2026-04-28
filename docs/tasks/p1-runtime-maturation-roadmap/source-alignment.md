# P1 Runtime Maturation Source Alignment

Date: 2026-04-26

## 1. Alignment Basis

This roadmap is based on:

- `docs/claude-code-go-parity-semantic-review.md`
- `docs/tasks/p0-runtime-parity-roadmap/`
- `claude-code/docs/06-ui-state-and-repl.md`
- `claude-code/docs/09-extensions-plugins-skills-mcp-lsp.md`
- `claude-code/docs/10-bridge-remote-and-ide.md`
- `claude-code/docs/11-agents-and-tasks.md`
- `claude-code/docs/16-session-persistence-and-recovery.md`
- `claude-code/docs/21-memory-and-claude-md.md`
- `claude-code/docs/25-hooks-and-runtime-extensibility.md`
- `claude-code/docs/29-team-memory-sync-and-shared-repo-memory.md`
- `claude-code/docs/33-session-memory-scheduling-and-concurrency.md`
- `claude-code/docs/34-history-snip-and-replay-projection.md`
- `claude-code/docs/35-claude-md-loading-and-instruction-assembly.md`
- `claude-code/docs/37-agent-memory-snapshot-and-sync-protocol.md`
- `claude-code/docs/38-read-file-state-and-context-cache-mechanics.md`
- `claude-code/docs/40-agent-runtime-lifecycle-background-and-resume.md`
- `claude-code/docs/41-agent-isolation-worktree-remote-and-cwd-overrides.md`
- `claude-code/docs/42-agent-definition-loading-and-availability.md`

## 2. P1.1 AppState And Session Continuity

Claude source areas:

- `claude-code/src/state/AppStateStore.ts`
- `claude-code/src/history.ts`
- `claude-code/src/assistant/sessionHistory.ts`
- `claude-code/src/screens/`
- `claude-code/src/components/`
- `claude-code/src/QueryEngine.ts`

Go source areas:

- `internal/session/manager.go`
- `internal/session/recovery.go`
- `internal/store/file/session_store.go`
- `internal/runtime/runner.go`
- `internal/queryengine/queryengine.go`
- `internal/approval/manager.go`
- `internal/agent/manager.go`
- `internal/tui/state.go`
- `internal/gateway/server.go`

Alignment requirements:

- session state and UI/client state must not drift
- recovery must restore user-visible pending work
- approvals and tasks must be recoverable state, not transient UI state
- gateway and TUI must observe one shared continuation model

## 3. P1.2 Context Cache And Memory Depth

Claude source areas:

- `claude-code/src/context.ts`
- `claude-code/src/memdir/`
- `claude-code/src/QueryEngine.ts`
- `claude-code/src/history.ts`
- `claude-code/src/utils/`

Go source areas:

- `internal/workspace/loader.go`
- `internal/prompt/builder.go`
- `internal/memory/service.go`
- `internal/model/claude_transcript.go`
- `internal/session/recovery.go`
- `internal/runtime/session_compaction.go`
- `internal/queryengine/context_provider.go`
- `internal/queryengine/queryengine.go`

Alignment requirements:

- `CLAUDE.md` loading is deterministic
- read-file state is model context hygiene, not plain memory
- context cache invalidation is explicit
- projected history and replay are separate from persisted raw transcript
- recovered context must be reproducible

## 4. P1.3 Subagent Task Isolation

Claude source areas:

- `claude-code/src/tools/AgentTool/AgentTool.tsx`
- `claude-code/src/tasks/`
- `claude-code/src/commands/tasks/index.ts`
- `claude-code/src/state/AppStateStore.ts`
- `claude-code/docs/39-forking-subagents-and-context-economics.md`
- `claude-code/docs/40-agent-runtime-lifecycle-background-and-resume.md`
- `claude-code/docs/41-agent-isolation-worktree-remote-and-cwd-overrides.md`
- `claude-code/docs/42-agent-definition-loading-and-availability.md`

Go source areas:

- `internal/agent/manager.go`
- `internal/tools/agent_tool.go`
- `internal/agents/loader.go`
- `internal/runtime/worktree.go`
- `internal/runtime/runner.go`
- `internal/session/manager.go`
- `internal/permissions/policy.go`
- `internal/tui/tasks.go`
- `internal/gateway/server.go`

Alignment requirements:

- subagents are durable delegated work units
- background execution is inspectable and controllable
- child sessions inherit context and permissions safely
- worktree/cwd isolation is explicit
- agent definitions determine available roles and behavior

## 5. P1.4 Extension Platform Foundation

Claude source areas:

- `claude-code/src/services/mcp/`
- `claude-code/src/skills/`
- `claude-code/src/plugins/`
- `claude-code/src/services/lsp/`
- `claude-code/src/utils/plugins/`
- `claude-code/src/commands/plugin/`
- `claude-code/src/commands/mcp/`
- `claude-code/src/commands/skills/`

Go source areas:

- `internal/tools/mcp_client.go`
- `internal/tools/mcp_dynamic.go`
- `internal/tools/mcp_oauth.go`
- `internal/tools/skill_discovery.go`
- `internal/tools/skill_frontmatter.go`
- `internal/tools/bundled_skills.go`
- `internal/tools/registry.go`
- `internal/config/config.go`
- `internal/runtime/runner.go`
- `internal/queryengine/queryengine.go`
- `internal/gateway/server.go`

Alignment requirements:

- extension discovery and inventory are runtime-owned
- MCP, skills, plugin-like commands, and future LSP should not become separate ad hoc systems
- permission and allowed-tools rules apply across extension types
- extension state should be visible through control-plane APIs

