# Task Designs

This directory contains task-scoped design artifacts.

Planning rule:

- start from the active architecture
- decompose from large program areas into implementation modules
- then create detailed executable designs for one module at a time
- each module must provide a single Claude Code entry document: `docs/tasks/<module-name>/task.md`

Current order:

1. `p0-runtime-parity-roadmap`
2. `execution-surface-program`
3. `ssh-runtime-capability`
4. `mcp-runtime-capability`
5. `subagent-runtime-capability`
6. `shell-runtime-capability`
7. `tui-client-architecture`
8. `tui-charmbracelet-v2-migration`
9. `react-operator-ui-console`

Standard:

- `task-doc-standard.md`
