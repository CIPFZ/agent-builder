# Implementation Rules

## Rule 1: Claude Source First

For every major capability:

1. identify the Claude Code source module that owns the semantics
2. review that source before changing Go code
3. define the Go ownership boundary to match the source semantics as closely as practical

## Rule 2: Runtime First, UI Later

Do not solve backend capability gaps in frontend code.

If a capability is real, it must exist first in:

- Go runtime
- `myclawd` protocol

Only then can the React UI consume it.

## Rule 3: TUI Is A Lightweight Shell

The Go TUI is not the primary parity target.

Allowed TUI investments:

- prompt input
- basic message flow
- approval handling
- basic task views
- basic runtime debugging

Avoid heavy investment in:

- advanced UI parity
- visual fidelity work
- deep interaction systems that duplicate planned React UI effort

## Rule 4: Prefer Stable Contracts Over Ad Hoc Shortcuts

Do not add one-off client-specific paths when a shared protocol is needed.

This applies especially to:

- approvals
- task control
- tool progress
- runtime inventory

## Rule 5: Test Before Expansion

For each new backend capability:

1. add or update focused tests
2. verify the failing condition if possible
3. implement the minimal aligned change
4. run focused tests
5. run the relevant functional flow

## Rule 6: Build The Execution Surface As First-Class Runtime Capability

Capabilities like ssh, docker, and db must not remain "just shell snippets".

Each must have:

- explicit runtime contract
- permission and approval integration
- progress and result semantics
- control-plane support

## Rule 7: Delete Stale Planning Aggressively

Keep only documentation that still has direct execution value for the active architecture and roadmap.

Do not preserve old planning just because it exists.

