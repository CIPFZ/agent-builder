# P0 Tool Parity Core Implementation Plan

1. Add failing tests for tool identity and tool result contracts.
2. Harden `tools.ToolResult` and QueryEngine tool-result emission as needed.
3. Add tests for shell success, failure, and approval-required paths.
4. Add tests for file tool classifications, workspace boundaries, and failures.
5. Add tests for TodoWrite, Agent, Skill, and MCP dynamic result semantics.
6. Implement the minimum production changes in the owning packages.
7. Run focused tests and then the roadmap validation command for this workstream.

Commit: `feat: align tool identity semantics`.