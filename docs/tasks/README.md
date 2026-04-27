# Task Designs

This directory contains task-scoped design artifacts.

Planning rule:

- start from the active architecture
- decompose from large program areas into implementation modules
- then create detailed executable designs for one module at a time
- each module must provide a single Claude Code entry document: `docs/tasks/<module-name>/task.md`

Current order:

1. `execution-surface-program`
2. `ssh-runtime-capability`
3. `mcp-runtime-capability`
4. `subagent-runtime-capability`
5. `shell-runtime-capability`
6. `tui-client-architecture`
7. `tui-charmbracelet-v2-migration`
8. `react-operator-ui-console`

Standard:

- `task-doc-standard.md`
