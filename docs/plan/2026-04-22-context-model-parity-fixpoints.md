# Context Model Parity Fixpoints

Date: 2026-04-22
Branch: `codex/context-model-parity`

## Goal

Capture the agreed fixpoints for the current `/model`, `/context`, and compaction-threshold work before additional parity rounds continue.

## Fixpoints

### 1. Dynamic compact thresholds must be model-aware

- The compact threshold cannot stay on a static window assumption.
- The effective context window must be derived from the resolved model metadata.
- The reserved output budget must use the model's real `max_output_tokens` when available.
- When model metadata is missing, the runtime may fall back to a conservative default reserve.
- This is required to stop premature compact for large-window models and to avoid overfilling smaller windows.

### 2. `/model` intentionally diverges from Claude Code

- Claude Code's `/model` command is not provider-discovery driven.
- `myclaw` will intentionally use provider/API-backed model discovery where the provider exposes model inventory.
- The source of truth for selectable models becomes:
  - remote discovered inventory first
  - configured profiles/models as fallback
- The selected model shown in transcript/system feedback must preserve the actual model id, not only a friendly label.

### 3. `/context` must move toward Claude-style context visibility

- Claude Code exposes context state as a first-class command and visualization.
- The current Go runtime had no `/context` command and no model-aware context view.
- This phase adds the minimum closed loop:
  - `/context` command
  - runtime context snapshot
  - model name
  - used tokens
  - effective context window
  - usage percentage
  - category lines for message usage, autocompact buffer, and free space
- Remaining parity gaps still exist versus Claude Code:
  - richer category analysis
  - full visualization fidelity
  - non-interactive context command parity
  - deeper compact-state explanation

## Review Baseline

Claude Code review baseline for this fixpoint set:

- `claude-code/src/commands/model/index.ts`
- `claude-code/src/commands/model/model.tsx`
- `claude-code/src/components/ModelPicker.tsx`
- `claude-code/src/utils/model/modelOptions.ts`
- `claude-code/src/utils/model/validateModel.ts`
- `claude-code/src/commands/context/index.ts`
- `claude-code/src/commands/context/context.tsx`
- `claude-code/src/commands/context/context-noninteractive.ts`
- `claude-code/src/components/ContextVisualization.tsx`
- `claude-code/src/utils/analyzeContext.ts`
- `claude-code/src/services/compact/autoCompact.ts`

## Completion For This Slice

This slice is considered closed when:

- provider-backed model discovery is wired into runtime and TUI
- configured models remain selectable when discovery fails
- session model overrides validate against discovered or configured inventory
- compact snapshot uses resolved model metadata
- `/context` is available in TUI and shows the runtime context snapshot
- targeted tests and full Go test suite pass
