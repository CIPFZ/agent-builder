# P0 Command Registry Implementation Plan

1. Add failing tests for command registration, aliases, visibility, and execution result shape.
2. Create `internal/commands` with registry, metadata, context, and result types.
3. Register the initial P0 command set.
4. Route TUI command parsing through the shared registry.
5. Add QueryEngine input processor tests for command results.
6. Run focused validation.

Commit: `feat: add runtime command registry`.