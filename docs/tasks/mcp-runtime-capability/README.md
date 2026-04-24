# MCP Runtime Capability

Date: 2026-04-23

## Purpose

This folder defines the MCP runtime capability module for `myclaw`.

The module goal is not to "add basic MCP support" from scratch.

`myclaw` already contains a meaningful MCP runtime core:

- MCP tool discovery
- MCP prompt discovery
- MCP resource discovery
- MCP OAuth storage and auth flow primitives
- dynamic MCP tool registration
- MCP prompt-to-skill projection

What is still missing is turning that partial core into a stable, executable module that is:

- reachable from normal app bootstrap
- aligned to Claude Code semantics
- manageable from the shared control plane
- reviewable as one coherent implementation target

## Read Order

1. `task.md`
2. `design.md`
3. `source-alignment.md`
4. `implementation-plan.md`
5. `test-validation-plan.md`
6. `review-checklist.md`

## Delivery Standard

This module is only ready for implementation when all of the following are true:

- the current partial MCP implementation is assessed explicitly
- the target module scope is constrained and executable
- the Claude Code semantic references are explicit
- the Go ownership boundary is explicit
- the `myclawd` control-plane expectations are explicit
- the test and review gates are strong enough for downstream Claude Code implementation

## Module Outcome

After this module lands, `myclaw` should be able to treat MCP as a first-class runtime capability instead of a partially wired internal feature.

That means:

- MCP servers can be configured and bootstrapped in normal runtime startup
- MCP tool/resource/prompt/skill inventory is stable and refreshable
- OAuth/auth-required and reconnect semantics are explicit
- `myclawd` can expose MCP state and control surfaces without client-specific hacks
