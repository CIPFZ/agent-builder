# P0 Runtime Structured Events Design

Contracts:

- Event names are stable constants.
- Event payloads have documented fields.
- QueryEngine emits lifecycle semantics once; runtime and gateway transport them.
- Gateway does not invent separate business semantics.
- TUI consumes shared event families where practical.

Event families: session lifecycle, message lifecycle, tool lifecycle, permission and approval lifecycle, compaction lifecycle, command lifecycle, agent/task lifecycle, and MCP/skill inventory lifecycle.