# P1 Runtime Maturation Roadmap Review Checklist

Date: 2026-04-26

## 1. Scope Review

- [ ] P1 builds on P0 and does not redefine P0 foundations.
- [ ] P1 focuses on runtime durability, not UI visual parity.
- [ ] P1 excludes telemetry, enterprise managed settings, full bridge/remote, and plugin marketplace.
- [ ] P1 includes AppState/session continuity, context cache/memory depth, subagent task isolation, and extension platform foundation.

## 2. Source Alignment Review

- [ ] AppState/session continuity references `AppStateStore`, history, session, screens/components, and `QueryEngine`.
- [ ] Context/cache/memory references `context.ts`, `memdir`, session memory docs, history snip/replay docs, and read-file state docs.
- [ ] Subagent/task isolation references `AgentTool`, `tasks`, task command, AppState task fields, and agent lifecycle docs.
- [ ] Extension foundation references MCP, skills, plugins, LSP, plugin utils, and command surfaces.

## 3. Go Ownership Review

- [ ] Session continuity ownership includes session, store, runtime, queryengine, approval, agent, TUI, and gateway.
- [ ] Context depth ownership includes workspace, prompt, memory, model, session, runtime, and queryengine.
- [ ] Subagent ownership includes agent, agent tool, agents loader, runtime worktree, permissions, TUI, and gateway.
- [ ] Extension ownership includes MCP tools, skill tools, config, runtime, queryengine, gateway, and optional `internal/extensions`.

## 4. Sequencing Review

- [ ] AppState/session continuity is first.
- [ ] Context cache/memory depth follows state continuity.
- [ ] Subagent task isolation follows continuity and context inheritance.
- [ ] Extension platform follows stable inventory and recovery semantics.

## 5. Validation Review

- [ ] Each workstream has focused test categories.
- [ ] Each workstream has suggested Go test commands.
- [ ] P1 has one end-to-end scenario.
- [ ] `go test ./...` remains the final validation command.
- [ ] Restart and reconnect are explicit validation requirements.

## 6. Implementation Readiness

- [ ] Child task folder names are explicit.
- [ ] The recommended first child task is explicit.
- [ ] Non-goals are explicit enough to prevent scope creep.
- [ ] Missing P0 dependencies must be recorded rather than bypassed.

