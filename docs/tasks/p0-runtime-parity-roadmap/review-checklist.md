# P0 Runtime Parity Roadmap Review Checklist

Date: 2026-04-26

## 1. Scope Review

- [ ] P0 focuses on runtime behavior, not UI visual parity.
- [ ] P0 excludes telemetry, enterprise managed settings, bridge/remote, and plugin marketplace.
- [ ] P0 includes tool parity, command registry, context/memory/recovery, and structured runtime events.
- [ ] Every P0 item is needed before P1/P2 work can be reliable.

## 2. Source Alignment Review

- [ ] Tool work references `Tool.ts`, `tools.ts`, `tools/`, `AgentTool`, permissions, and `QueryEngine`.
- [ ] Command work references `commands.ts` and `commands/`.
- [ ] Context/recovery work references `context.ts`, history/session docs, memory docs, and `QueryEngine`.
- [ ] Event work references `structuredIO.ts`, SDK entrypoints, `QueryEngine`, and `Tool.ts`.

## 3. Go Ownership Review

- [ ] Tool ownership is in `internal/tools`, `internal/tools/system`, `internal/queryengine`, `internal/runtime`, `internal/permissions`, and `internal/approval`.
- [ ] Command ownership is not left only in `internal/tui`.
- [ ] Context/recovery ownership includes `workspace`, `prompt`, `memory`, `session`, `store`, `model`, `runtime`, and `queryengine`.
- [ ] Event ownership includes `runtime`, `queryengine`, `gateway`, `protocol/ws`, and `tui/runtime_bridge`.

## 4. Sequencing Review

- [ ] Tool parity comes before event schema finalization.
- [ ] Command registry comes before full session continuation scenarios.
- [ ] Recovery tests wait for stable tool and command result shapes.
- [ ] Runtime structured events are final integration work.

## 5. Validation Review

- [ ] Each workstream has focused test categories.
- [ ] Each workstream has suggested Go test commands.
- [ ] P0 has one end-to-end scenario.
- [ ] `go test ./...` remains the final validation command.

## 6. Implementation Readiness

- [ ] This roadmap does not ask implementers to infer child task scope from memory.
- [ ] Child task folder names are explicit.
- [ ] The recommended first child task is explicit.
- [ ] Non-goals are explicit enough to prevent scope creep.


## Validation Result

2026-04-28: Focused workstream validation and `go test ./...` completed with exit code 0 in worktree `C:\Users\ytq\work\ai\agent-builder\.worktrees\claude-code-semantic-review`.

Known limits remain assigned to P1/P2: full Claude Code concrete tool parity, full SDK/structured IO transport compatibility, React Ink UI parity, telemetry/GrowthBook, enterprise managed settings, bridge/remote, plugin marketplace, broad LSP, and full read-file/context-cache semantics.
