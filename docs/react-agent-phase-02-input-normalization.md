# Phase 2: Runtime Input Normalization

Status: planned.

## Goal

Replace prompt-only chat submission with a runtime-owned normalized input
contract. Text, images, voice transcripts, slash commands, shell mode, meta
prompts, and project/session scope should enter the runtime through one
structured path.

## User Problem

If input normalization stays in React or adapter code, runtime cannot reliably
explain what the user actually submitted. It also blocks future support for
images, voice, slash commands, command-mode input, prompt-submit hooks, and
hidden task/meta prompts.

## Claude Code Reference

Claude Code input processing is concentrated in:

- `src/utils/processUserInput/processUserInput.ts`
- `src/utils/processUserInput/processTextPrompt.ts`
- `src/utils/processUserInput/processSlashCommand.tsx`
- `src/utils/processUserInput/processBashCommand.tsx`

That flow returns:

- normalized messages;
- `shouldQuery`;
- optional command result text;
- optional allowed tools/model/effort overrides;
- hook-blocking behavior;
- image metadata/meta messages.

Agent Builder should implement the same class of boundary in Go, not in React.

## Current Agent Builder Evidence

- `internal/runtime/runtime_contract_types.go`
  - `RuntimeChatRequest` currently carries only `Prompt`, `SessionID`,
    `ProjectID`, and `Scope`.
- `internal/runtime/runtime_turns.go`
  - `Chat(...)` trims prompt text and creates/selects a session before
    starting the turn.
- `internal/agent/agent.go`
  - `createUserMessage(...)` persists text and attachment parts after runtime
    has already accepted the request.
  - `preparePrompt(...)` handles image capability fallback for model history.
- `client/src/app/shell/WorkbenchShell.tsx`
  - composer submission currently calls adapter `sendPrompt(...)` with a text
    prompt.
- `client/src/runtime/wailsWorkbenchAdapter.ts`
  - adapter maps prompt submission to the runtime `Chat` transport.

The current split means React/composer behavior can distinguish input modes
before Go runtime has a durable normalized record. This phase moves that
boundary into runtime.

## Runtime Contract

Introduce a new request alongside the old `RuntimeChatRequest` during the
transition:

```go
type RuntimeUserInputRequest struct {
    SessionID string                  `json:"sessionId,omitempty"`
    ProjectID string                  `json:"projectId,omitempty"`
    Scope     string                  `json:"scope,omitempty"`
    Mode      string                  `json:"mode"`
    Items     []RuntimeUserInputItem  `json:"items"`
    Options   RuntimeUserInputOptions `json:"options,omitempty"`
}

type RuntimeUserInputItem struct {
    Type        string            `json:"type"`
    Text        string            `json:"text,omitempty"`
    Data        string            `json:"data,omitempty"`
    MIMEType    string            `json:"mimeType,omitempty"`
    FileName    string            `json:"fileName,omitempty"`
    SourcePath  string            `json:"sourcePath,omitempty"`
    Metadata    map[string]string `json:"metadata,omitempty"`
}

type RuntimeUserInputOptions struct {
    IsMeta            bool   `json:"isMeta,omitempty"`
    SkipSlashCommands bool   `json:"skipSlashCommands,omitempty"`
    BridgeOrigin      bool   `json:"bridgeOrigin,omitempty"`
    VoiceSource       string `json:"voiceSource,omitempty"`
    ClientRequestID   string `json:"clientRequestId,omitempty"`
}
```

Recommended `Mode` values:

- `prompt`
- `slash`
- `shell`
- `voice`
- `meta`

Recommended `Item.Type` values:

- `text`
- `image`
- `audio_transcript`
- `file_ref`
- `ide_selection`
- `pasted_text`

Add normalized output:

```go
type RuntimeNormalizedInput struct {
    ID                  string                    `json:"id"`
    SessionID           string                    `json:"sessionId"`
    ProjectID           string                    `json:"projectId,omitempty"`
    Mode                string                    `json:"mode"`
    Prompt              string                    `json:"prompt,omitempty"`
    Messages            []RuntimeMessageDraft     `json:"messages"`
    Attachments          []RuntimeAttachmentDraft  `json:"attachments,omitempty"`
    ShouldQuery          bool                      `json:"shouldQuery"`
    Command              *RuntimeInputCommand      `json:"command,omitempty"`
    HookOutcome          *RuntimeInputHookOutcome  `json:"hookOutcome,omitempty"`
    ModelOverride        string                    `json:"modelOverride,omitempty"`
    AllowedToolsOverride []string                  `json:"allowedToolsOverride,omitempty"`
    CreatedAt            int64                     `json:"createdAt"`
}
```

## Runtime Implementation

1. Add `runtime_input.go` with pure normalization helpers.
2. Preserve `Chat(...)` as a compatibility wrapper:
   - convert `Prompt` into `RuntimeUserInputRequest{Mode:"prompt"}`;
   - call the normalized path internally.
3. Add `SubmitUserInput(...)` as the preferred runtime method.
4. Normalize images:
   - accept base64 and metadata;
   - validate MIME type;
   - optionally downsample later, but first preserve structured metadata;
   - never put raw image bytes into runtime events.
5. Normalize voice:
   - runtime receives transcript text and source metadata;
   - speech-to-text can be a later adapter feature;
   - runtime persists "voice transcript" as input evidence.
6. Normalize slash commands:
   - parse slash at runtime;
   - route known runtime commands to runtime actions;
   - return `ShouldQuery=false` for local commands;
   - return `ShouldQuery=true` only when a command expands into a model prompt.
7. Normalize shell mode:
   - decide whether shell mode means "run in terminal" or "ask agent to use
     bash";
   - do not execute shell from React;
   - record explicit input mode.
8. Normalize meta prompts:
   - hidden task/scheduler prompts must be marked `IsMeta`;
   - UI can hide them, but runtime persists and explains them.
9. Persist normalized input evidence with the turn.

## Data Model

Prefer no broad migration in the first implementation. Options:

- store normalized input JSON in existing turn metadata if available;
- if no safe field exists, add a narrow `runtime_user_inputs` table:

```text
id
session_id
turn_id
project_id
mode
prompt_preview
items_json
normalized_json
created_at
```

Use the table only if the input evidence cannot be recovered from existing
turn/message rows.

## HTTP And Wails

HTTP:

```text
POST /v1/user-inputs
GET  /v1/user-inputs/{input_id}
```

Wails:

```go
SubmitUserInput(ctx context.Context, req RuntimeUserInputRequest)
UserInput(ctx context.Context, inputID string)
```

Keep existing `Chat` until the frontend fully migrates.

## Frontend Display

The composer should produce structured input:

- text prompt as `text`;
- pasted image as `image`;
- voice transcript as `audio_transcript` plus text;
- slash command as `mode=slash`;
- shell input as `mode=shell`;
- hidden/system task input as `isMeta=true`.

UI expectations:

- show attached images as user-visible attachments;
- show voice input as transcript with a small voice marker;
- show slash commands as command rows if they do not query the model;
- show meta prompts only in diagnostics/callchain, not in the normal chat.

## Frontend Ownership Rules

- React may own temporary composer draft state.
- React must not decide whether slash command output is durable runtime state.
- React must not synthesize hidden model-visible messages outside the runtime
  normalized input response.
- The adapter maps composer state into DTOs and renders runtime responses.

## Tests

Runtime tests:

- prompt string compatibility maps to normalized input;
- image input produces attachment draft and input evidence;
- voice transcript produces prompt text plus voice metadata;
- unknown slash command is treated according to runtime policy;
- known local slash command returns `ShouldQuery=false`;
- prompt-submit hook block returns no model query;
- meta prompt persists but is marked hidden.

Frontend tests:

- composer maps text/images/voice/slash/shell into DTOs;
- adapter still falls back to old `Chat` if new method is unavailable during
  transition;
- UI renders normalized input evidence from runtime.

Browser smoke:

- send text prompt;
- attach/paste image in dev path if supported;
- run a slash command that does not query the model;
- verify timeline and callchain use runtime normalized input.

## Acceptance Criteria

- Runtime can explain every user input that starts or does not start a turn.
- React no longer owns business distinctions between prompt, slash, shell,
  image, voice, and meta input.
- Old `Chat` remains only as compatibility wrapper, not primary architecture.
