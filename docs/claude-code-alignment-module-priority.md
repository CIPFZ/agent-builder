# Claude Code Alignment Module Priority

Status: refreshed on 2026-05-27 from current `main`.

Agent Builder baseline: `d51590b76a680e683b9d5335797c7076c16a5b05`

Authoritative audit:

- [`docs/claude-code-runtime-parity-closure-review.md`](./claude-code-runtime-parity-closure-review.md)

Older priority lists that made compact, persisted replay, tool discovery
guardrails, policy profiles/headless semantics, output/artifact refs, MCP
auth/elicitation, worktree recovery, sandbox boundary, context/read-file
hardening, hooks lifecycle, or AgentTask/coordinator communication the next
missing module are superseded by current code evidence.

## Priority Decision

Recommended unique next task:

```text
runtime: run parity closure stabilization and scenario coverage
```

No core runtime blocker remains for the local desktop parity scope. The highest
priority is proving the runtime closure with scenarios before React diagnostics
or P3 product work.

## Module Priority Table

| Rank | Module | Bucket | Evidence / reason |
| --- | --- | --- | --- |
| 1 | Runtime parity closure stabilization and scenario coverage | Next | Completed primitives now need cross-boundary proof across hooks, policy/headless, MCP, AgentTask, sandbox/worktree, compact/replay, refs, and recovery. |
| 2 | Shell parser fixture expansion | Parallel hardening | Deterministic policy exists in `internal/permission/policy.go`, but Bash/PowerShell/cmd parity needs broader destructive/read-only fixtures. |
| 3 | Diagnostics DTO consistency checks | Parallel hardening | React must mirror runtime contracts from `internal/runtimeapi/contract.go` and `client/src/runtime/types.ts`. |
| 4 | React compact/task/policy/replay/hook/worktree diagnostics | Later | Allowed only after closure confidence and stable runtime APIs. React remains a runtime consumer. |
| 5 | Advisory permission explanations | P3/product later | Must be advisory-only and cannot approve high-risk actions. |
| 6 | Plugin package governance | P3/product later | Skills/MCP metadata exist; package trust/signing is product hardening, not current parity blocker. |
| 7 | Advanced memory/session memory compact | P3/runtime later | AGENTS/CLAUDE/read-file state exists; Claude Code's fuller memory taxonomy can wait. |
| 8 | Remote runtime / remote agent / SSH / cloud teammate | P3/product later | No current local runtime dependency blocks parity. |
| 9 | Terminal UI, CLI/TUI UX, slash UI, keybindings, product telemetry/growth, provider rewrite | Not needed | Explicitly excluded from desktop runtime parity. |

## Completed Runtime Foundations

- Runtime spine, turn/session lifecycle, cancellation/interruption records,
  event store, audit, replay export, and recovery.
- Tool scheduler integration, ToolCall normalization, output refs, artifact
  refs, and shell job metadata.
- Tool discovery/search with disclosure, omissions, guardrails, and replay.
- Compact budget, micro compact, full compact, reinjection refs, and persisted
  compact boundaries.
- Context source loading for AGENTS/CLAUDE, context diagnostics, and read-file
  state hardening.
- Permission policy profiles, headless fail-closed semantics, scoped rules, and
  shell risk classification.
- MCP lifecycle with tools/resources/prompts, policy filtering,
  auth/elicitation records, recovery, replay, and redaction.
- Hooks lifecycle foundation with persistence, audit, replay, and recovery.
- Local AgentTask/coordinator communication with model-facing task tools,
  parent-child messages, backend follow-up delivery, stop/cancel, output and
  artifact refs, replay, recovery, scope, and worktree integration.
- Worktree lifecycle, cleanup/recovery, task cwd scope, and sandbox decision
  boundary records.

## Partial / Hardening

- Full Claude Code memory taxonomy and session memory compact.
- Full Claude hook event breadth beyond local core pre/post/failure hooks.
- Shell parser parity fixture breadth.
- OS-level sandbox executor maturity.
- Signed/local plugin package governance.
- Provider/model health policy above fantasy.
- React deep diagnostics.

## Runtime Blockers

None for the local desktop runtime parity scope.

## React/Page Diagnostics Later

React/page work should wait unless it is pure read-only DTO mirroring over
stable runtime APIs. The first React diagnostics targets after closure are:

- compact budget/reinjection panels;
- replay export viewer;
- policy profile/rule diagnostics;
- task mailbox/coordinator view;
- artifact/output detail drawer;
- hook/worktree diagnostics.

## P3 / Product Optimization Later

- remote runtime, remote agent, SSH/cloud teammate;
- advisory permission advisor;
- signed/local plugin package governance;
- advanced memory/session memory compact;
- provider/model health dashboards over fantasy;
- OS sandbox executor maturity.

## Not Needed

- Terminal UI / Ink / terminal layout.
- Keybindings / Vim input state.
- Slash command UI.
- CLI argument UX.
- Anthropic subscription/pass/product growth surfaces.
- Claude.ai OAuth/product login surfaces.
- First-party telemetry sinks / GrowthBook / Datadog.
- Marketplace-first plugin browsing/install.
- Provider/model protocol rewrite.
- Changes to `charm.land/fantasy`.
- TUI/CLI main-path restoration.
