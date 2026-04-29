# P1 Extension Platform Foundation Implementation Plan

Date: 2026-04-29

## Order

1. Add failing focused tests for runtime extension inventory.
2. Add failing focused tests for skill frontmatter inventory projection.
3. Add failing gateway test for `extension_inventory`.
4. Add tool-level skill inventory projection helper.
5. Add queryengine extension inventory schema and deterministic assembly.
6. Add runtime read-only API aliases.
7. Add websocket method constant and gateway payload serialization.
8. Create and update P1.4 docs and checklist.
9. Run focused and full validation commands.

## Implementation Notes

- Keep inventory read-only.
- Keep sorting stable and deterministic.
- Filter tools through effective session permission policy.
- Do not move MCP, skill, command, or tool truth into gateway.
- Keep LSP and marketplace as explicit deferred records.
