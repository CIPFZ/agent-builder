# P1 Context Cache And Memory Depth Source Alignment

Date: 2026-04-29

## Claude Code Source Areas

Relevant semantic sources:

- `claude-code/src/context.ts`
- `claude-code/src/memdir/`
- `claude-code/src/QueryEngine.ts`
- `claude-code/src/history.ts`
- `claude-code/src/utils/`
- `claude-code/docs/21-memory-and-claude-md.md`
- `claude-code/docs/33-session-memory-scheduling-and-concurrency.md`
- `claude-code/docs/34-history-snip-and-replay-projection.md`
- `claude-code/docs/35-claude-md-loading-and-instruction-assembly.md`
- `claude-code/docs/38-read-file-state-and-context-cache-mechanics.md`

## Go Source Areas

Primary Go source areas:

- `internal/workspace/loader.go`
- `internal/prompt/builder.go`
- `internal/memory/service.go`
- `internal/model/claude_transcript.go`
- `internal/session/recovery.go`
- `internal/runtime/session_compaction.go`
- `internal/queryengine/context_provider.go`
- `internal/queryengine/queryengine.go`
- `internal/tools/file_tools.go`
- `internal/store/*`

## Alignment Requirements

- `CLAUDE.md` and workspace instructions load deterministically.
- Read-file state is tracked as context hygiene, separate from memory.
- Context cache invalidation is explicit and conservative.
- Projected history and replay are derived views, not mutations of raw transcript.
- Compaction memory save can be recovered after restart.
- Rebuilt context is reproducible from persisted runtime/session data.

## Known Gap After P1.2

P1.2 will not fully reproduce Claude Code's complete context cache economics, team memory sync, or agent memory snapshot protocol. It establishes deterministic Go-side rebuild primitives for later depth.
