# P1.3 Implementation Plan

Date: 2026-04-29

1. Add tests for structured Agent task isolation request parsing.
2. Add tests for agent lifecycle metadata, foreground/background transition, and restore.
3. Add runtime tests for session metadata persistence, recovery, cwd override, worktree isolation, allowed-tools, and permission inheritance.
4. Add gateway/TUI projection tests for client-visible task isolation fields.
5. Extend agent run and session metadata structs.
6. Persist metadata through runtime continuation mapping.
7. Wire Agent tool and websocket spawn payloads into `SubagentOptions`.
8. Update review checklist after validation.
