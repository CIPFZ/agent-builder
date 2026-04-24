# Subagent Runtime Capability

Date: 2026-04-24

## Purpose

This folder defines the subagent runtime capability module for `myclaw`.

The module goal is not to "add subagent support" from scratch.

`myclaw` already contains a meaningful delegated-task core:

- `agent.task` tool execution
- subagent spawn and resume paths
- background output file writing
- derived subagent permission policy
- worktree isolation hooks
- `myclawd` methods for spawn, list, status, stop, steer, and resume

What is still missing is turning that partial core into a stable, executable module that is:

- aligned to Claude Code task and worker semantics
- lifecycle-complete for real delegated work
- observable and controllable from the shared control plane
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

- the current partial subagent implementation is assessed explicitly
- the lifecycle contract is made explicit
- the Claude Code semantic references are explicit
- the Go ownership boundary is explicit
- the `myclawd` control-plane expectations are explicit
- the test and review gates are strong enough for downstream Claude Code implementation

## Module Outcome

After this module lands, `myclaw` should be able to treat delegated work as a first-class runtime surface instead of a partially wired helper path.

That means:

- delegated tasks have stable identity and lifecycle state
- running tasks can be observed and controlled through `myclawd`
- completed or stopped tasks can be resumed with the same child session context
- fork and worktree semantics are explicit instead of incidental
- downstream Docker, database, and project-control work can safely build on this lifecycle
