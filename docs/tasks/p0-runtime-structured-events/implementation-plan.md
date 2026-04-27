# P0 Runtime Structured Events Implementation Plan

1. Add failing tests for event constants and payload stability.
2. Define shared runtime event names and payload helpers.
3. Map QueryEngine events to runtime events without changing business semantics.
4. Expose gateway websocket payloads from the same event contract.
5. Cover TUI runtime bridge consumption.
6. Run focused validation.

Commit: `feat: stabilize runtime event contracts`.