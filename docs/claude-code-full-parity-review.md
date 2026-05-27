# Claude Code Full Parity Review

Date: 2026-05-27

Agent Builder baseline: `d51590b76a680e683b9d5335797c7076c16a5b05`

Claude Code comparison source: `C:\Users\ytq\work\ai\myclaw\claude-code`

The current authoritative full audit is:

- [`docs/claude-code-runtime-parity-closure-review.md`](./claude-code-runtime-parity-closure-review.md)

This file remains as the full parity entry point and supersedes older phase
roadmaps that listed compact, persisted replay, tool discovery guardrails,
policy profiles, output/artifact refs, MCP auth/elicitation, worktree recovery,
sandbox boundary, context/read-file hardening, hooks, or AgentTask/coordinator
communication as missing top-level runtime blockers.

## Current Conclusion

| Scope | Judgment |
| --- | --- |
| Single main-agent core runtime | 92 percent complete. Durable turns, scheduler/tool calls, permissions, policy profiles, compact/reinjection, replay, context/read-file state, hooks, MCP, worktree/sandbox decisions, refs, recovery, and adapter contracts exist. |
| Local multi-agent / AgentTask / coordinator runtime | 84 percent complete. Local task roles, model-facing task tools, parent-child messages, follow-up delivery, stop/cancel, output/artifact refs, replay, recovery, worktree/cwd scope, and rejection semantics exist. Remote teammate/fleet/cloud remains P3. |
| Core Claude Code runtime parity excluding TUI/CLI/remote/product surfaces | 87 percent complete. Remaining work is stabilization and hardening, not a missing core primitive. |

No current core runtime blocker remains for the local desktop runtime parity
scope. Agent Builder should enter runtime closure/stabilization before React
diagnostics deepen.

## Current Capability Matrix

The complete evidence matrix is in
[`docs/claude-code-runtime-parity-closure-review.md`](./claude-code-runtime-parity-closure-review.md#current-capability-matrix).
It covers:

- Query engine / turn lifecycle
- Session lifecycle / cancellation / interruption / recovery
- Tool protocol / scheduler / structured output normalization
- Tool discovery / search / recursive guardrails / deadlock avoidance
- Compact / context budget / micro compact / full compact / reinjection
- Context / memory / CLAUDE.md / AGENTS / read-file state
- Permission policy / profiles / plan/headless mode / scoped rules / shell safety
- Model-assisted permission advisor
- Hooks lifecycle / hook policy precedence / hook replay/recovery
- MCP lifecycle / resources / prompts / tools / auth / elicitation
- Skills / allowed tools / activation / plugin metadata boundary
- AgentTask / subagent roles / task tools
- Parent-child messaging / coordinator-worker communication
- Worktree / cwd isolation / sandbox
- Remote runtime
- Audit / durable events / replay / eval harness / local observability
- Provider/model configuration above fantasy
- React runtime boundary / diagnostics / recovery consumption
- Data model / migrations / persistence / recovery
- Background shell jobs / artifacts / output refs
- Adapter boundary: HTTP / Wails
- Excluded Claude Code product/UI surfaces

## Summary Classification

| Classification | Current items |
| --- | --- |
| Completed | Runtime spine; event/audit/recovery; scheduler; policy/headless/scoped rules; capability registry; tool discovery; compact/reinjection; context/read-file hardening; MCP auth/elicitation; output/artifact refs; worktree recovery; sandbox decision boundary; hooks lifecycle foundation; local AgentTask communication; persisted replay. |
| Partial | Advanced memory taxonomy; full Claude hook event breadth; skills/plugin governance; OS-level sandbox executor maturity; provider/model health policy above fantasy; React deep diagnostics; shell parser fixture parity. |
| Missing / runtime blocker | None in the local parity scope. |
| Runtime hardening | Closure scenarios; shell parser fixtures; hook/policy/scope/MCP/sandbox/AgentTask bypass fixtures; compact/replay/ref restart fixtures; migration/replay contract checks. |
| React/page diagnostics later | Compact, replay, policy, task mailbox, artifact/output, hook, and worktree diagnostics over stable runtime APIs. |
| P3/product optimization later | Remote runtime/remote agents/SSH/cloud teammate; advisory permission explanations; plugin package governance; advanced session memory compact; OS sandbox executor hardening. |
| Not needed | Terminal UI, Ink, keybindings, Vim input, slash command UI, CLI argument UX, Anthropic product growth/login, first-party telemetry sinks, GrowthBook, Datadog, marketplace-first install, provider protocol rewrite, `charm.land/fantasy` changes. |

## Hooks And AgentTask Closure

Hooks are complete as a local runtime foundation: pre-tool, post-tool, and
post-tool-failure lifecycle records are runtime-owned, persisted, audited,
replayable, and recoverable. Hook allow is advisory and cannot override
deterministic deny, headless fail-closed, task scope, sandbox, or MCP gates.
Claude Code hook events beyond the local core are later unless a runtime
workflow needs them.

AgentTask/coordinator communication is complete as a local runtime primitive.
The runtime owns task messages, delivery/processed/rejected status, backend
follow-up delivery to child sessions, stop/cancel control records, output and
artifact refs, replay, recovery, HTTP/Wails DTOs, and model-facing task tools.
Remote teammate/fleet/cloud semantics are P3.

## Next Session

Recommended unique next task:

```text
runtime: run parity closure stabilization and scenario coverage
```

Runtime remains first because React must render runtime-owned facts. The client
must not infer compact, task, hook, artifact, policy, replay, or worktree state
from local reducers or message parsing.
