# Phase 5: Hooks And Error Recovery

Status: planned.

## Goal

Expand hook coverage and harden recovery so failures remain structured,
recoverable, and explainable after reload.

## User Problem

Hooks and errors are currently visible mainly around tool execution. Users also
need clear behavior around prompt submission, compact, post-model sampling, and
recoverable model errors. A failure should not leave stale permissions,
orphaned tool calls, or unexplained empty assistant rows.

## Claude Code Reference

Claude Code includes hook and recovery paths across:

- UserPromptSubmit hooks;
- PreToolUse/PostToolUse/PostToolUseFailure hooks;
- PreCompact/PostCompact hooks;
- PostSampling hooks;
- stop hooks;
- fallback model retry;
- tombstoning orphaned streaming attempts;
- synthetic missing tool result blocks;
- prompt-too-long, media-size, and max-output-token recovery.

Relevant files:

- `src/query.ts`
- `src/utils/hooks/*`
- `src/query/stopHooks.ts`
- `src/services/compact/*`

## Current Agent Builder Evidence

- `internal/hooks/runner.go`
- `internal/hooks/input.go`
- `internal/agent/hooked_tool.go`
- `internal/runtime/runtime_hooks.go`
- `internal/agent/agent.go` error handling and synthetic tool result path.
- `internal/runtime/runtime_recovery.go` startup recovery.

## Backend Work

### Hook Event Expansion

Add hook events only when there is a concrete runtime use:

- `UserPromptSubmit`
- `PreCompact`
- `PostCompact`
- `PostSampling`
- `Stop`

Each event must have:

- typed payload;
- redaction rules;
- timeout behavior;
- fail-open/fail-closed policy;
- runtime hook execution record;
- replay/read DTO.

### Prompt Submit Hooks

Prompt hooks run after input normalization and before turn creation or model
execution.

Outcomes:

- allow;
- block with user-visible reason;
- rewrite normalized input;
- inject context;
- prevent model query.

These outcomes must become normalized input evidence.

### Error Recovery

Add structured recovery records:

```go
type RuntimeRecoveryRecord struct {
    ID          string `json:"id"`
    SessionID   string `json:"sessionId"`
    TurnID      string `json:"turnId,omitempty"`
    Kind        string `json:"kind"`
    Status      string `json:"status"`
    Reason      string `json:"reason"`
    EvidenceRef string `json:"evidenceRef,omitempty"`
    CreatedAt   int64  `json:"createdAt"`
}
```

Recommended kinds:

- `missing_tool_result`
- `provider_error`
- `model_fallback`
- `prompt_too_long`
- `media_too_large`
- `max_output_tokens`
- `stale_permission_cancelled`
- `stale_tool_cancelled`
- `hook_blocked`

### Fallback Model

If fallback model support exists in config/fantasy, record:

- original model;
- fallback model;
- retry attempt;
- orphaned assistant tombstone/synthetic result handling.

If fallback is not implemented, record this as a later subphase and still make
provider errors visible through recovery DTOs.

## Frontend Display

Add recovery indicators:

- failed turn row shows runtime error and recovery status;
- callchain inspector shows synthetic tool results;
- hook panel shows prompt/tool/compact/sampling hooks;
- stale recovered permissions are read-only, not actionable.

The UI should distinguish:

- blocked by hook;
- denied by permission;
- provider failed;
- model retried/fallback;
- compact failed;
- user cancelled.

## Frontend Ownership Rules

- React does not create recovery records.
- React does not resurrect permission or MCP actionability.
- React does not keep old pending permission cards after runtime says terminal.
- Event payloads only trigger rereads.

## Tests

Backend tests:

- prompt submit hook blocks before turn execution;
- prompt submit hook rewrites input and evidence is persisted;
- post tool hook halt records terminal reason;
- compact hook failure records recovery event;
- provider error with tool_use synthesizes missing result;
- restart cancels stale running/waiting evidence;
- recovered permission is read-only after reload.

Frontend tests:

- recovery row renders runtime reason;
- terminal permission is not clickable;
- hook execution list renders all hook event kinds from DTOs.

Browser smoke:

- configure a blocking hook fixture;
- submit a prompt;
- verify UI shows blocked-by-hook without creating a model turn;
- refresh and verify the state remains read-only/runtime-owned.

## Acceptance Criteria

- Hook outcomes and recovery decisions are durable backend facts.
- Users can understand why a turn failed or stopped.
- No stale actionability survives reload.

