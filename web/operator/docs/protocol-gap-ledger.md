# React Operator UI Protocol Gap Ledger

Date: 2026-04-25

## Current Gaps

1. `skills_status`
   The UI can show MCP-derived skills and runtime-observed invocations, but there is no dedicated myclawd method for a complete skills catalog across bundled, plugin, dynamic, and MCP sources.

2. `tools_inventory`
   The UI can render live tool lifecycle events, but there is no first-class inventory API for all registered tools, categories, permission semantics, or capability metadata.

3. `session_history`
   `send_message` and runtime events support live transcript rendering, but there is no explicit history API for cold reload transcript restoration or pagination.

4. `file_tree` and `file_preview`
   File capability visibility is event-derived from `Read`, `Write`, `Edit`, `MultiEdit`, `Glob`, `Grep`, and `LS`, but there is no browse-first contract for workspace tree, file preview, or diff retrieval.

5. normalized operation history
   Shell, SSH, and file views currently derive from tool events. A dedicated operation history API would make filtering, replay, and persisted operator auditing more robust.

## Explicit Non-Goals

- Docker as a first-class UI module without a backend runtime contract
- Database as a first-class UI module without a backend runtime contract
- Frontend-side approval inference
- Frontend-side tool safety inference
