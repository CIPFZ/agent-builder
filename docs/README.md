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
- `frontend-backend-integration-notes.md`: active frontend/backend integration
  notes, including Wails vs Vite/browser transport constraints.
- `frontend-runtime-ui-technical-plan.md`: active frontend rewrite technology,
  UI/runtime integration, reference analysis, and client recovery plan.
- `workbench-runtime-feature-roadmap.md`: active handoff roadmap for main chat,
  sessions, model switching, timeline rendering, permissions, skills, MCP, and
  projects.
- `tool-thinking-permission-integration-plan.md`: completed frontend/backend
  integration milestone for runtime tool calls, thinking, and permissions.
- `runtime-parity-closure-stabilization-plan.md`: completed 2026-06-03 runtime
  closure scenario coverage and contract hardening record.

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
- `legacy-crush-inventory.md`

## Historical Material

Historical docs live under `archive/`. They are useful for background only and
must not override the current roadmap.

Notable archived baselines:

- `archive/phase-2-runtime-api-boundary.md`
- `archive/dev-baseline.md`
- `archive/crush-claude-code-gap-analysis.md`
- `archive/reference-analysis/`
- `archive/tui-removal-plan.md`
