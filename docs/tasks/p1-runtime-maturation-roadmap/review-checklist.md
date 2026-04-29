# P1 Runtime Maturation Roadmap Review Checklist

Date: 2026-04-26

## 1. Scope Review

- [x] P1 builds on P0 and does not redefine P0 foundations.
- [x] P1 focuses on runtime durability, not UI visual parity.
- [x] P1 excludes telemetry, enterprise managed settings, full bridge/remote, and plugin marketplace.
- [x] P1 includes AppState/session continuity, context cache/memory depth, subagent task isolation, and extension platform foundation.

## 2. Source Alignment Review

- [x] AppState/session continuity references `AppStateStore`, history, session, screens/components, and `QueryEngine`.
- [x] Context/cache/memory references `context.ts`, `memdir`, session memory docs, history snip/replay docs, and read-file state docs.
- [x] Subagent/task isolation references `AgentTool`, `tasks`, task command, AppState task fields, and agent lifecycle docs.
- [x] Extension foundation references MCP, skills, plugins, LSP, plugin utils, and command surfaces.

## 3. Go Ownership Review

- [x] Session continuity ownership includes session, store, runtime, queryengine, approval, agent, TUI, and gateway.
- [x] Context depth ownership includes workspace, prompt, memory, model, session, runtime, and queryengine.
- [x] Subagent ownership includes agent, agent tool, agents loader, runtime worktree, permissions, TUI, and gateway.
- [x] Extension ownership includes MCP tools, skill tools, config, runtime, queryengine, gateway, and optional `internal/extensions`.

## 4. Sequencing Review

- [x] AppState/session continuity is first.
- [x] Context cache/memory depth follows state continuity.
- [x] Subagent task isolation follows continuity and context inheritance.
- [x] Extension platform follows stable inventory and recovery semantics.

## 5. Validation Review

- [x] Each workstream has focused test categories.
- [x] Each workstream has suggested Go test commands.
- [x] P1 has one end-to-end scenario.
- [x] `go test ./...` remains the final validation command.
- [x] Restart and reconnect are explicit validation requirements.

## 6. Implementation Readiness

- [x] Child task folder names are explicit.
- [x] The recommended first child task is explicit.
- [x] Non-goals are explicit enough to prevent scope creep.
- [x] Missing P0 dependencies must be recorded rather than bypassed.

## 7. P1 Completion Status

- [x] P1.1 AppState/session continuity is recorded complete in its review checklist.
- [x] P1.2 Context cache/memory depth is recorded complete in its review checklist.
- [x] P1.3 Subagent task isolation is recorded complete in its review checklist.
- [x] P1.4 Extension platform foundation is recorded complete in its review checklist.
- [x] P1.5 closure gate is documented under `docs/tasks/p1-runtime-maturation-closure/`.
- [x] P2 handoff is documented under `docs/tasks/p2-runtime-expansion-roadmap/`.
- [x] P2/P3 items are not claimed as P1-complete.

