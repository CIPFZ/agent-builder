# P1.3 Design

Date: 2026-04-29

## Model

The agent manager owns live lifecycle state. Session metadata owns durable recovery state. Runtime bridges the two by persisting every agent run update and rehydrating it on runner construction.

Subagent isolation metadata is part of the run contract:

- background flag
- isolation mode
- cwd override
- remote boundary name
- allowed tools
- effective permission mode
- whether permission was inherited
- output file path
- parent run and continuation mode for retry/resume/continue

## Boundaries

Gateway and TUI expose these fields but do not infer them. They receive projected state from runtime continuation snapshots or agent manager payloads.

Remote isolation is represented as explicit boundary metadata. P1.3 does not implement remote transport or execution.

## Safety

Subagent permissions are derived conservatively. Parent bypass or auto modes do not automatically grant equally broad subagent modes unless an explicit subagent mode is configured.

Worktree isolation rewrites workspace roots to the isolated worktree. CWD override is persisted and used as a child session workspace root when no worktree path is active.
