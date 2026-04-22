# SSH Runtime Capability

Date: 2026-04-22

## Purpose

This folder defines the first remote execution module for `myclaw`.

The scope is intentionally narrow:

- first-class SSH remote command execution
- runtime approval and permission integration
- progress and result propagation through `myclawd`

This folder does not cover:

- file sync
- interactive terminal sessions
- port forwarding
- SSH config UI
- React operator UI implementation

## Read Order

1. `task.md`
2. `design.md`
3. `source-alignment.md`
4. `implementation-plan.md`
5. `test-validation-plan.md`
6. `review-checklist.md`
7. `review-fix-task.md`

## Delivery Standard

This module is only considered ready for implementation when all of the following are true:

- the Go ownership boundary is explicit
- the Claude Code semantic references are explicit
- the approval and protocol behavior are explicit
- the test and validation flow is explicit
- the review checklist is strong enough for downstream Claude Code implementation review

## Module Outcome

After this module lands, `myclaw` should be able to invoke SSH as a first-class tool and use it as the base remote execution surface for later Docker and database control work.
