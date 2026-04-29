# P1 Extension Platform Foundation Review Checklist

Date: 2026-04-29

## Entry Gate

- [x] P0 shared command registry remains stable.
- [x] SubmitPrompt/SubmitMessage single-processing remains stable.
- [x] P1.1 continuation snapshot remains stable.
- [x] P1.2 context cache and memory rebuild remain stable.
- [x] P1.3 subagent task isolation remains stable.

## Scope

- [x] P1.4 only; P2/P3 marketplace and full LSP are not implemented.
- [x] No placeholder-only implementation.
- [x] Gateway/TUI do not own extension truth.

## Inventory

- [x] Extension inventory schema exists.
- [x] Dynamic tools are projected from runtime tool contracts.
- [x] Dynamic commands are projected from runtime command configuration.
- [x] MCP servers/tools/resources/prompts/skills are projected.
- [x] Skill frontmatter allowed tools, hooks, context, and agent metadata are projected.
- [x] Inventory output is stable sorted and testable.

## Lifecycle And Recovery

- [x] MCP connected/error/needs-auth state remains visible through server snapshots.
- [x] Reconnect/auth state continues to update MCP snapshots.
- [x] Inventory rebuilds deterministically after runner rebuild from runtime-owned inputs.

## Permissions

- [x] Extension inventory does not grant broader permissions.
- [x] Denied tools are filtered by the current session permission policy.
- [x] Skill allowed-tools remain metadata and do not bypass runtime policy.

## Client Projection

- [x] Runtime exposes read-only extension inventory.
- [x] Gateway exposes `extension_inventory` by serializing runtime projection.
- [x] UI/gateway do not persist or infer inventory truth.

## Deferred Boundaries

- [x] Future LSP boundary is represented as schema-only deferred inventory.
- [x] Plugin marketplace is explicitly deferred to P2/P3.
- [x] Remote extension lifecycle is explicitly deferred.

## Tests

- [x] Focused runtime/tools/gateway extension inventory tests pass.
- [x] Required P1.4 package command passes.
- [x] P0/P1.1 regression command passes.
- [x] P1.2 regression command passes.
- [x] P1.3 regression command passes.
- [x] `go test ./...` passes.
