# P1.3 Source Alignment

Date: 2026-04-29

## Claude Code Semantics

Claude Code delegated tasks are expected to be observable, resumable, and constrained by inherited safety boundaries. Background work must remain inspectable after reconnect or restart, and task output must be available without rerunning the model.

## Go Alignment

- `internal/agent.Manager` tracks lifecycle and control messages.
- `internal/runtime.Runner` creates child sessions, applies derived permission policy, applies worktree/cwd isolation, and persists metadata.
- `internal/session.SessionMetadata.AgentRuns` stores the recovery source.
- `internal/gateway` and `internal/tui` expose runtime projections.

## Known Gap

Remote isolation is recorded as metadata only. A later workstream must define transport, remote filesystem boundaries, and remote permission enforcement before remote execution is enabled.
