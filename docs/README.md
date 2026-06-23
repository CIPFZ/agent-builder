# Agent Builder Docs

This directory contains current architecture docs plus historical reference
material. Use this index to avoid following superseded phase plans.

## Current Entry Points

- `claude-code-runtime-parity-audit.md`: current full runtime parity audit
  against the local Claude Code source snapshot.
- `claude-code-alignment-next-roadmap.md`: current execution roadmap. This is
  the next-planning source of truth.
- `claude-code-alignment-module-priority.md`: short pointer to the current
  roadmap and parity audit.
- `client-runtime-architecture-review.md`: current architecture boundary review.
- `frontend-runtime-integration-notes.md`: active frontend/runtime integration
  notes, including Wails vs Vite/browser transport constraints.
- `frontend-runtime-ui-technical-plan.md`: active frontend rewrite technology,
  UI/runtime integration, reference analysis, and client recovery plan.
- `workbench-runtime-feature-roadmap.md`: active handoff roadmap for main chat,
  sessions, model switching, timeline rendering, permissions, skills, MCP, and
  projects.
- `tool-thinking-permission-integration-plan.md`: completed frontend/runtime
  integration milestone for runtime tool calls, thinking, and permissions.
- `runtime-parity-closure-stabilization-plan.md`: completed 2026-06-03 runtime
  closure scenario coverage and contract hardening record.
- `workbench-skills-mcp-management-plan.md`: active Phase 6 implementation
  record for runtime-backed Skills and MCP management surfaces.
- `plugin-skills-runtime-integration-plan.md`: active follow-up phase for
  runtime-backed plugin capability-bundle DTOs and plugin-center integration.
- `react-agent-architecture-audit.md`: current runtime-first ReAct architecture
  audit after the project/session/terminal ownership work.
- `react-agent-implementation-roadmap.md`: staged implementation roadmap for
  runtime input normalization, callchain observability, tool/permission/result
  hardening, context/compact/memory, hooks/recovery, tasks/subagents, and
  React rendering.
  - `react-agent-phase-01-callchain-observability.md`
  - `react-agent-phase-02-input-normalization.md`
  - `react-agent-phase-03-tool-permission-result-loop.md`
  - `react-agent-phase-04-context-prompt-compact-memory.md`
  - `react-agent-phase-05-hooks-error-recovery.md`
  - `react-agent-phase-06-tasks-subagents-background.md`
  - `react-agent-phase-07-frontend-react-workbench.md`

## Active Design Baselines

These documents describe runtime/client contracts that still matter, but many
items are already partially implemented. Read their status blocks before using
their checklists.

- `client-architecture-and-core-flow.md`
- `desktop-runtime-boundary.md`
- `turn-task-run-model.md`
- `tool-scheduler-design.md`
- `permission-policy-model.md`
- `client-first-runtime-refactor.md`
- `client-information-architecture.md`
- `frontend-runtime-ui-technical-plan.md`
- `agentic-operations-client.md`
- `architecture-decisions.md`
- `legacy-agent-builder-inventory.md`

## Historical Material

Historical docs live under `archive/`. They are useful for background only and
must not override the current roadmap.

Notable archived baselines:

- `archive/phase-2-runtime-api-boundary.md`
- `archive/dev-baseline.md`
- `archive/agent-builder-claude-code-gap-analysis.md`
- `archive/reference-analysis/`
- `archive/tui-removal-plan.md`
