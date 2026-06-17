# Phase 4: Context, Prompt, Compact, And Memory

Status: planned.

## Goal

Make prompt assembly and context reduction auditable runtime decisions. The
runtime should know exactly which system prompt, memory/context sources, tools,
skills, MCP instructions, compact summaries, and attachments entered each model
step.

## User Problem

When a long conversation behaves poorly, users need to know what the model
actually saw. If compact, memory, and prompt assembly are invisible, users
cannot tell whether the model forgot context, saw stale tool output, or lost a
file/skill/memory source.

## Claude Code Reference

Claude Code `src/query.ts` does significant work before every model request:

- applies aggregate tool result budget;
- applies history snip;
- applies microcompact;
- applies context collapse;
- attempts auto compact;
- starts memory and skill prefetch;
- builds full system prompt with user/system context;
- chooses tools and model;
- handles prompt-too-long and recoverable context errors.

Claude Code compact implementation lives under `src/services/compact/*`.

## Current Agent Builder Evidence

- `internal/agent/agent.go`
  - system prompt and prompt prefix;
  - MCP instructions injection;
  - `preparePrompt(...)`;
  - cache controls;
  - auto summarization stop condition.
- `internal/agent/coordinator.go`
  - prompt build and skills XML injection.
- `internal/runtime/runtime_budget.go`
  - budget report.
- `internal/runtime/runtime_compact.go`
  - compact boundaries, refs, reinjection.
- `internal/skills/`
  - skill discovery and prompt XML.
- `internal/runtime/runtime_context.go`
  - context source reads.

## Backend Contract

Add a runtime prompt assembly snapshot:

```go
type RuntimePromptAssembly struct {
    ID             string                         `json:"id"`
    SessionID      string                         `json:"sessionId"`
    TurnID         string                         `json:"turnId"`
    Step           int                            `json:"step"`
    Model          string                         `json:"model"`
    Provider       string                         `json:"provider"`
    System         RuntimePromptSystemSummary     `json:"system"`
    Messages       RuntimePromptMessageSummary    `json:"messages"`
    Tools          RuntimePromptToolSummary       `json:"tools"`
    Skills         RuntimePromptSkillSummary      `json:"skills"`
    MCP            RuntimePromptMCPSummary        `json:"mcp"`
    ContextSources []RuntimeContextSource         `json:"contextSources,omitempty"`
    Compact        []RuntimeCompactBoundary       `json:"compact,omitempty"`
    Budget         RuntimeBudgetReport            `json:"budget"`
    CreatedAt      int64                          `json:"createdAt"`
}
```

Do not store raw full prompt text by default. Store redacted summaries, refs,
hashes, counts, and token estimates. Add a developer-only prompt dump later if
needed.

## Backend Implementation

1. Introduce a `PromptAssemblyRecorder` used inside `PrepareStep`.
2. Record one assembly snapshot per model step.
3. Include:
   - system prompt hash and source list;
   - prompt prefix presence;
   - MCP instruction server list;
   - selected tools and omitted tools;
   - skill list and loaded skill names;
   - context source list;
   - compact boundary refs;
   - tool result budget effects;
   - attachment/image counts;
   - estimated tokens.
4. Move any post-turn-only budget/compact insight that affects model input into
   pre-model assembly where possible.
5. Keep full compact summary/read-file reinjection as runtime refs.
6. Add memory review:
   - identify current AGENTS/CLAUDE/context path behavior;
   - define memory source DTOs;
   - distinguish project memory, user memory, skill instructions, and recent
     conversation summary.

## Compact Work

Current compact pieces are useful but should be made step-aware:

- single result persistence: ToolResultGuard;
- aggregate result budget: guard turn budget;
- microcompact: old tool result clearing;
- full compact: boundary and refs;
- reinjection: context sources and read file state.

Add missing concepts in phases:

- snip: remove low-value middle history while preserving protected tail;
- auto compact: proactive before model call, not only after turn;
- compact failure recovery: clear reason and retry path;
- compact visible DTO: summary, refs, token deltas, and user-facing status.

## Frontend Display

Add a "Context" diagnostic panel:

- current model/provider;
- prompt budget buckets;
- selected tools count;
- skills loaded count;
- MCP instruction sources;
- compact boundaries;
- memory/context source states;
- warnings for skipped/failed sources.

In the main timeline:

- compact boundary markers should appear as compact runtime markers;
- do not show raw prompt text;
- show "large tool output persisted" with a link/action to inspect runtime ref
  if the runtime exposes a safe read.

## Frontend Ownership Rules

- React does not estimate prompt truth from displayed messages.
- React may render token estimates from runtime DTOs.
- React must not store full prompt, full terminal output, or raw tool output in
  WorkbenchViewModel.
- Compact markers are DTO rows, not locally generated dividers.

## Tests

Backend tests:

- assembly snapshot exists for text-only turn;
- tool turn records selected tools and tool result delivery;
- MCP instructions list connected servers only;
- skills XML presence is recorded without storing full content;
- compact boundary refs appear in assembly after compact;
- failed context source appears as failed, not silently dropped;
- prompt snapshot redacts secrets.

Frontend tests:

- context panel renders DTO fields;
- no raw prompt/tool output enters view model;
- compact markers come from runtime DTOs.

Browser smoke:

- run a tool-heavy prompt;
- open context panel;
- verify tools, budget, compact/tool result indicators render from runtime.

## Acceptance Criteria

- Each model step has an auditable prompt assembly summary.
- Users can understand context loss/compact decisions.
- The UI can display context state without becoming a context state owner.

