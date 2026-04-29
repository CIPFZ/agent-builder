# P1.5 Runtime Maturation Closure Review Checklist

Date: 2026-04-29

## Scope

- [x] P1.5 is documented as P1 closure and P2 readiness gate.
- [x] P1.5 does not redesign P0/P1 modules.
- [x] React UI, plugin marketplace, full LSP, remote trusted-device, and enterprise policy work are excluded.

## Integration Gates

- [x] Runtime command registry is checked across QueryEngine, TUI, runtime inventory, and gateway payload paths.
- [x] Session/appstate recovery is checked against approval, agent task, extension inventory, and context cache expectations.
- [x] Context cache invalidation inputs are documented and covered by existing focused tests.
- [x] Subagent/background task recovery isolation metadata is documented and covered by existing focused tests.
- [x] Extension inventory includes runtime commands, configured commands, dynamic tools, skills, MCP servers, and LSP boundary placeholder.
- [x] Extension inventory sorting and command dedupe rules are explicit.
- [x] Gateway payload serialization is checked against runtime projection fields.
- [x] TUI command behavior remains covered by focused tests.

## Tests

- [x] Added closure-focused gateway runtime command metadata assertion.
- [x] Required P1.5 package validation command 1 passes.
- [x] Required P1.5 package validation command 2 passes.
- [x] Required P1.5 package validation command 3 passes.
- [x] Required P1.5 package validation command 4 passes.
- [x] `go test ./...` passes.

## Documentation

- [x] P1.5 task docs follow `task-doc-standard.md`.
- [x] P2 roadmap docs follow `task-doc-standard.md`.
- [x] P1 roadmap status records P1.1-P1.4 completion, P1.5 closure, and P2 handoff.

## Handoff

- [x] P2 deferred items are explicit.
- [x] No unimplemented P2/P3 capability is claimed as complete in P1.
