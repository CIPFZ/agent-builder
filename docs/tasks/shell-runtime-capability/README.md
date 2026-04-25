# Shell Runtime Capability

Date: 2026-04-25

## Purpose

This folder defines the shell runtime capability module for `myclaw`.

The module goal is not to "add a shell command tool".

`myclaw` already has shell-facing tools and a usable runtime loop, but the current shell surface is still below the level required for stable Claude Code-aligned project control.

This module turns shell execution into a first-class runtime capability that is:

- aligned to Claude Code execution semantics
- permission-aware and reviewable
- observable from the shared `myclawd` control plane
- strong enough to serve as the base layer for ssh, Docker, and database workflows

## Read Order

1. `task.md`
2. `design.md`
3. `source-alignment.md`
4. `implementation-plan.md`
5. `test-validation-plan.md`
6. `review-checklist.md`

## Delivery Standard

This module is only ready for implementation when all of the following are true:

- the current shell execution reality is assessed explicitly
- the target shell contract is constrained and executable
- Claude Code semantic references are explicit
- approval, progress, and result behavior are defined
- `myclawd` control-plane expectations are explicit
- the test and review gates are strong enough for downstream Claude Code implementation

## Module Outcome

After this module lands, `myclaw` should be able to treat shell execution as a stable runtime capability instead of a thin tool wrapper.

That means:

- shell tools have clear execution and approval contracts
- progress and result behavior are observable from shared runtime paths
- session/worktree-aware execution semantics are explicit
- `myclawd` can expose shell lifecycle state without client-specific hacks
- ssh, Docker, and database execution modules can build on top of this layer instead of inventing their own semantics
