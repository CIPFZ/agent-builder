# P0 Tool Parity Core Design

## Goal

Treat tools as a controlled runtime execution protocol, not helper functions.

## Contracts

- Tool identity is stable from model tool-use block to tool-result block.
- Tool results preserve model-consumable output, structured content, metadata, and error state.
- Tool classifications are explicit for read-only and destructive behavior.
- Permission and approval checks run before execution and keep conservative defaults.
- Observable input can differ from raw input only through an explicit backfill path.
- Tool execution errors are returned as observable tool-result errors, not swallowed.

## Non-Goals

- Full Claude Code concrete tool parity.
- React Ink UI parity.
- New broad execution surfaces outside the named P0 tools.