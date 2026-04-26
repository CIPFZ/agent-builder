# Task Designs

This directory contains task-scoped design artifacts.

Planning rule:

- start from the active architecture
- decompose from large program areas into implementation modules
- then create detailed executable designs for one module at a time
- each module must provide a single Claude Code entry document: `docs/tasks/<module-name>/task.md`

Current order:

1. `p0-runtime-parity-roadmap`
2. `p1-runtime-maturation-roadmap`
3. `execution-surface-program`
4. `ssh-runtime-capability`
5. `mcp-runtime-capability`
6. `subagent-runtime-capability`
7. `shell-runtime-capability`
8. `tui-client-architecture`
9. `tui-charmbracelet-v2-migration`
10. `react-operator-ui-console`

Standard:

- `task-doc-standard.md`
