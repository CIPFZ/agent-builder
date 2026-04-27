# P0 Context Memory Recovery Test Validation Plan

Required coverage: workspace instruction loading, prompt context ordering, memory injection, compaction boundary persistence, pending approval recovery, tool-use/tool-result recovery, invoked skill recovery, and recovered session continuation.

```powershell
go test ./internal/workspace ./internal/prompt ./internal/memory ./internal/session ./internal/store/... ./internal/model ./internal/runtime ./internal/queryengine
```