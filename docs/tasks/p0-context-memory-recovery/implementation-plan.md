# P0 Context Memory Recovery Implementation Plan

1. Add failing recovery tests for tool-use/tool-result identity and pending approval rehydration.
2. Add tests for deterministic workspace instruction and memory ordering.
3. Add tests for compaction boundary and invoked skill recovery.
4. Implement minimal production changes in session metadata, recovery snapshot, QueryEngine restore, and runtime rehydration.
5. Run focused validation.

Commit: `feat: add session recovery baseline`.