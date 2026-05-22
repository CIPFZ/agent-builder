# Reference Project Analysis

This directory records architecture analysis for Crush and selected reference
agent projects. The goal is to support future Crush-based runtime design, not
to copy implementations across projects.

## Projects

- [Crush](./crush.md)
- [Claude Code](./claude-code.md)
- [Codex](./codex.md)
- [Gemini CLI](./gemini-cli.md)
- [Comparison](./comparison.md)

## Analysis Scope

Each project analysis follows the same structure:

1. Project positioning.
2. Technology stack and repository shape.
3. Startup and main loop.
4. Model/provider abstraction.
5. Session, message, and event state.
6. Tool registry and execution protocol.
7. Permission, approval, and safety model.
8. MCP, plugin, skill, and extension model.
9. Subagent, task, and scheduler model.
10. Sandbox, process, and workspace isolation.
11. Context loading, memory, and compression.
12. Client/UI/API boundaries.
13. Observability, telemetry, tests, and evals.
14. Designs worth borrowing.
15. Gaps or risks for our target product.
16. Implications for Crush-based implementation.

## Target Product Lens

The analysis is evaluated against the planned product direction:

- Crush remains the main implementation base.
- The target is an agentic operations client for enterprise product workflows.
- Runtime capabilities matter more than chat UI.
- Key runtime capabilities include tool governance, permissions, sandboxing,
  task state, recovery, plugins, and agent teams.
