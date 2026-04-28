# P1 Context Cache And Memory Depth Test Validation Plan

Date: 2026-04-29

## Required Tests

1. Workspace instruction loading:
   - multiple `CLAUDE.md` files load in deterministic order
   - repeated loads produce identical projections

2. Read-file state:
   - records path, size, mtime/hash or equivalent invalidation key
   - survives context rebuild where persisted

3. Context cache:
   - unchanged inputs hit cache
   - changed files invalidate cache and rebuild
   - corrupt cache is rejected or conservatively bypassed

4. Projected history:
   - excludes or compresses non-model history
   - preserves tool use/result identity

5. History snip/replay:
   - replay after compaction boundary keeps identity stable

6. Compaction memory recovery:
   - memory summary is saved after compaction
   - restart can recover summary for rebuild

7. Restart rebuild:
   - identical persisted state produces deterministic context output

8. Error paths:
   - missing files, corrupt cache, or invalid memory summary do not panic
   - failure is explicit or conservative

9. P0/P1.1 regressions:
   - slash command registry remains active
   - SubmitPrompt single-processing remains covered
   - continuation snapshot remains covered

## Required Commands

```powershell
go test ./internal/workspace ./internal/prompt ./internal/memory ./internal/model ./internal/session ./internal/runtime ./internal/queryengine
go test ./internal/session ./internal/store/... ./internal/runtime ./internal/queryengine ./internal/approval ./internal/agent ./internal/tui ./internal/gateway
go test ./...
```

## Expected Result

All packages pass or report `[no test files]`.
