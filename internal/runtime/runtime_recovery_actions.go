package runtime

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/CIPFZ/agent-builder/internal/runtimeapi"
)

const runtimeInterruptedResumePrompt = `You are resuming an interrupted Agent Builder turn.

The previous turn was interrupted before it completed.
Continue from the last reliable state. Do not assume unfinished tool calls succeeded.
If a missing tool result is needed, rerun the relevant read-only or safe operation.
If an operation may have side effects, ask for confirmation or inspect state first.`

const runtimeRecoveryCompactRetryCountKey = "recovery_compact_retry_count"

func (r *runtimeService) ResumeInterruptedTurn(ctx context.Context, turnID string, req RuntimeResumeInterruptedTurnRequest) (RuntimeTurnResponse, error) {
	if err := r.ensureStarted(ctx); err != nil {
		return RuntimeTurnResponse{}, err
	}
	turnID = strings.TrimSpace(turnID)
	if turnID == "" {
		return RuntimeTurnResponse{}, errors.New("turn id is required")
	}
	source, err := r.turns.Get(ctx, turnID)
	if err != nil {
		return RuntimeTurnResponse{}, fmt.Errorf("turn %s was not found: %w", turnID, err)
	}
	if source.Status != turnStatusInterrupted {
		return RuntimeTurnResponse{}, fmt.Errorf("turn %s is not interrupted", turnID)
	}
	recovered := r.classifyInterruptedTurn(ctx, source)
	mode := strings.TrimSpace(req.Mode)
	if mode == "" {
		mode = "continue"
	}
	prompt := runtimeInterruptedResumePrompt
	if userPrompt := strings.TrimSpace(req.Prompt); userPrompt != "" {
		prompt += "\n\nUser recovery instruction:\n" + userPrompt
	}
	metadata := cloneStringMap(req.Metadata)
	if metadata == nil {
		metadata = map[string]string{}
	}
	metadata["recovery_source_turn_id"] = source.ID
	metadata["recovery_action"] = runtimeRecoveryActionResumeInterruptedTurn
	metadata["recovery_mode"] = mode
	metadata["recovery_interruption_kind"] = recovered.InterruptionKind
	chat, err := r.SubmitUserInput(ctx, RuntimeUserInputRequest{
		SessionID: source.SessionID,
		Mode:      runtimeInputModePrompt,
		Items: []RuntimeUserInputItem{{
			Type:     runtimeInputItemText,
			Text:     prompt,
			Metadata: metadata,
		}},
		Options: RuntimeUserInputOptions{SkipSlashCommands: true},
	})
	if err != nil {
		return RuntimeTurnResponse{}, err
	}
	if _, err := r.recoveryLinks.Insert(ctx, runtimeRecoveryLink{
		SourceTurnID:     source.ID,
		ResumedTurnID:    chat.TurnID,
		Action:           runtimeRecoveryActionResumeInterruptedTurn,
		Mode:             mode,
		InterruptionKind: recovered.InterruptionKind,
	}); err != nil {
		return RuntimeTurnResponse{}, err
	}
	now := time.Now().UTC()
	r.storeRuntimeEvent(runtimeapi.Event{
		ID:        newRuntimeEventID(),
		Type:      runtimeapi.EventRecoveryTurnResumed,
		CreatedAt: now.Format(time.RFC3339Nano),
		SessionID: source.SessionID,
		TurnID:    chat.TurnID,
		Payload: map[string]any{
			"source_turn_id":    source.ID,
			"resumed_turn_id":   chat.TurnID,
			"mode":              mode,
			"interruption_kind": recovered.InterruptionKind,
			"refresh_only":      true,
		},
	})
	r.writeAudit(auditEntry{
		RequestID:     chat.TurnID,
		Event:         runtimeRecoveryActionResumeInterruptedTurn,
		Timestamp:     now.Format(time.RFC3339Nano),
		SessionID:     source.SessionID,
		Provider:      source.Provider,
		Model:         source.Model,
		PromptPreview: preview(prompt, auditPreviewLimit),
		Extra: map[string]any{
			"source_turn_id":    source.ID,
			"resumed_turn_id":   chat.TurnID,
			"mode":              mode,
			"interruption_kind": recovered.InterruptionKind,
		},
	})
	if projection, err := r.reconcileRuntimeRunForSession(ctx, source.SessionID); err == nil {
		r.recordRunTurnTransition(ctx, runtimeRunTransitionSourceRecoveryResume, RuntimeTurn{ID: chat.TurnID, SessionID: source.SessionID, Status: turnStatusRunning}, runtimeRunStatusInterrupted, firstNonEmpty(projection.Status, runtimeRunStatusActive), "interrupted turn resumed")
	}
	resp, err := r.Turn(ctx, chat.TurnID)
	if err != nil {
		return RuntimeTurnResponse{}, err
	}
	resp.Action = runtimeRecoveryActionMetadata(runtimeRecoveryActionResumeInterruptedTurn, runtimeRecoveryActionReasonResumed, "source_turn_id", true)
	return resp, nil
}

func (r *runtimeService) DiscardInterruptedTurn(ctx context.Context, turnID string) (RuntimeTurnResponse, error) {
	if err := r.ensureStarted(ctx); err != nil {
		return RuntimeTurnResponse{}, err
	}
	turnID = strings.TrimSpace(turnID)
	if turnID == "" {
		return RuntimeTurnResponse{}, errors.New("turn id is required")
	}
	turn, err := r.turns.Get(ctx, turnID)
	if err != nil {
		return RuntimeTurnResponse{}, fmt.Errorf("turn %s was not found: %w", turnID, err)
	}
	if turn.Status != turnStatusInterrupted {
		return RuntimeTurnResponse{}, fmt.Errorf("turn %s is not interrupted", turnID)
	}
	now := time.Now().UTC()
	runStatusBefore := r.runtimeRunStatusForSession(ctx, turn.SessionID)
	turn.Status = turnStatusCancelled
	turn.Error = "interrupted_turn_discarded"
	if turn.FinishedAt == 0 {
		turn.FinishedAt = now.UnixMilli()
	}
	stored, err := r.turns.Upsert(ctx, turn)
	if err != nil {
		return RuntimeTurnResponse{}, err
	}
	if projection, err := r.reconcileRuntimeRunForSession(ctx, stored.SessionID); err == nil {
		r.recordRunTurnTransition(ctx, runtimeRunTransitionSourceRecoveryDiscard, stored, runStatusBefore, firstNonEmpty(projection.Status, runtimeRunStatusCancelled), "interrupted turn discarded")
	}
	r.storeRuntimeEvent(runtimeapi.Event{
		ID:        newRuntimeEventID(),
		Type:      runtimeapi.EventRecoveryTurnDiscarded,
		CreatedAt: now.Format(time.RFC3339Nano),
		SessionID: stored.SessionID,
		TurnID:    stored.ID,
		Payload: map[string]any{
			"status": "cancelled",
			"reason": "interrupted_turn_discarded",
		},
	})
	r.writeAudit(auditEntry{
		RequestID: stored.ID,
		Event:     runtimeRecoveryActionDiscardInterruptedTurn,
		Timestamp: now.Format(time.RFC3339Nano),
		SessionID: stored.SessionID,
		Provider:  stored.Provider,
		Model:     stored.Model,
		Error:     "interrupted_turn_discarded",
		Extra:     map[string]any{"source_turn_id": stored.ID},
	})
	resp, err := r.Turn(ctx, stored.ID)
	if err != nil {
		return RuntimeTurnResponse{}, err
	}
	resp.Action = runtimeRecoveryActionMetadata(runtimeRecoveryActionDiscardInterruptedTurn, runtimeRecoveryActionReasonDiscarded, "turn_id", false)
	return resp, nil
}

func (r *runtimeService) RetryRecoverableError(ctx context.Context, errorID string) (RuntimeRecoveryRetryResponse, error) {
	if err := r.ensureStarted(ctx); err != nil {
		return RuntimeRecoveryRetryResponse{}, err
	}
	errorID = strings.TrimSpace(errorID)
	if errorID == "" {
		return RuntimeRecoveryRetryResponse{}, errors.New("error id is required")
	}
	turnID := strings.TrimPrefix(errorID, "turn:")
	if turnID == errorID {
		return RuntimeRecoveryRetryResponse{}, fmt.Errorf("unsupported recoverable error id %q", errorID)
	}
	turn, err := r.turns.Get(ctx, turnID)
	if err != nil {
		return RuntimeRecoveryRetryResponse{}, err
	}
	recoverable, ok := classifyRuntimeRecoverableError(turn)
	if !ok {
		return RuntimeRecoveryRetryResponse{}, fmt.Errorf("error %s is not recoverable", errorID)
	}
	if !recoverable.RetryEligible && !recoverable.CompactEligible {
		return RuntimeRecoveryRetryResponse{ErrorID: errorID, Error: recoverable, Action: runtimeRecoveryRejectedAction(errorID, recoverable.UserAction)}, nil
	}
	r.storeRuntimeEvent(runtimeapi.Event{
		ID:        newRuntimeEventID(),
		Type:      runtimeapi.EventRecoveryRetryStarted,
		CreatedAt: time.Now().UTC().Format(time.RFC3339Nano),
		SessionID: turn.SessionID,
		TurnID:    turn.ID,
		Payload: map[string]any{
			"error_id": errorID,
			"kind":     recoverable.Kind,
		},
	})
	if recoverable.CompactEligible {
		r.storeRuntimeEvent(runtimeapi.Event{ID: newRuntimeEventID(), Type: runtimeapi.EventRecoveryCompactRetryStarted, CreatedAt: time.Now().UTC().Format(time.RFC3339Nano), SessionID: turn.SessionID, TurnID: turn.ID, Payload: map[string]any{"error_id": errorID}})
		if _, compactErr := r.ManualCompact(ctx, RuntimeContextActionRequest{SessionID: turn.SessionID, TurnID: turn.ID, Reason: "recoverable_error_compact_retry"}); compactErr != nil {
			r.storeRuntimeEvent(runtimeapi.Event{ID: newRuntimeEventID(), Type: runtimeapi.EventRecoveryCompactRetryFailed, CreatedAt: time.Now().UTC().Format(time.RFC3339Nano), SessionID: turn.SessionID, TurnID: turn.ID, Payload: map[string]any{"error_id": errorID, "error": compactErr.Error()}})
			return RuntimeRecoveryRetryResponse{}, compactErr
		}
		r.storeRuntimeEvent(runtimeapi.Event{ID: newRuntimeEventID(), Type: runtimeapi.EventRecoveryCompactRetryCompleted, CreatedAt: time.Now().UTC().Format(time.RFC3339Nano), SessionID: turn.SessionID, TurnID: turn.ID, Payload: map[string]any{"error_id": errorID}})
	}
	input, inputErr := r.userInputs.GetByTurn(ctx, turn.ID)
	prompt := turn.PromptPreview
	if inputErr == nil && strings.TrimSpace(input.Prompt) != "" {
		prompt = input.Prompt
	}
	metadata := map[string]string{
		"recovery_source_turn_id": turn.ID,
		"recovery_action":         runtimeRecoveryActionRetryRecoverableError,
		"recovery_error_id":       errorID,
		"recovery_error_kind":     recoverable.Kind,
	}
	chat, err := r.SubmitUserInput(ctx, RuntimeUserInputRequest{
		SessionID: turn.SessionID,
		Mode:      runtimeInputModePrompt,
		Items:     []RuntimeUserInputItem{{Type: runtimeInputItemText, Text: prompt, Metadata: metadata}},
		Options:   RuntimeUserInputOptions{SkipSlashCommands: true},
	})
	if err != nil {
		r.storeRuntimeEvent(runtimeapi.Event{ID: newRuntimeEventID(), Type: runtimeapi.EventRecoveryRetryFailed, CreatedAt: time.Now().UTC().Format(time.RFC3339Nano), SessionID: turn.SessionID, TurnID: turn.ID, Payload: map[string]any{"error_id": errorID, "error": err.Error()}})
		return RuntimeRecoveryRetryResponse{}, err
	}
	r.storeRuntimeEvent(runtimeapi.Event{ID: newRuntimeEventID(), Type: runtimeapi.EventRecoveryRetryCompleted, CreatedAt: time.Now().UTC().Format(time.RFC3339Nano), SessionID: turn.SessionID, TurnID: chat.TurnID, Payload: map[string]any{"error_id": errorID, "retry_turn_id": chat.TurnID}})
	return RuntimeRecoveryRetryResponse{
		ErrorID: errorID,
		Error:   recoverable,
		Chat:    chat,
		Action:  runtimeRecoveryActionMetadata(runtimeRecoveryActionRetryRecoverableError, runtimeRecoveryActionReasonRetryStarted, "error_id", true),
	}, nil
}

func runtimeRecoveryRejectedAction(errorID, reason string) *RuntimeWriteActionMetadata {
	return &RuntimeWriteActionMetadata{
		Accepted:       false,
		Reason:         firstNonEmpty(reason, runtimeRecoveryActionReasonRetryNotAccepted),
		RefreshTargets: runtimeRecoveryRefreshTargets(),
		Source: RuntimeWriteActionSource{
			Kind:                  runtimeRecoveryActionSourceKind,
			Action:                runtimeRecoveryActionRetryRecoverableError,
			WorkbenchOnly:         true,
			StartsWorker:          false,
			IdempotentBy:          "error_id",
			SessionActivityParity: true,
			Evidence:              []string{"runtime_turns", "runtime_events", "runtime_audit", errorID},
		},
	}
}

func (r *runtimeService) publishRecoverableErrorClassified(turn RuntimeTurn) (RuntimeRecoverableError, bool) {
	recoverable, ok := classifyRuntimeRecoverableError(turn)
	if !ok {
		return RuntimeRecoverableError{}, false
	}
	r.storeRuntimeEvent(runtimeapi.Event{
		ID:        newRuntimeEventID(),
		Type:      runtimeapi.EventRecoveryErrorClassified,
		CreatedAt: time.Now().UTC().Format(time.RFC3339Nano),
		SessionID: turn.SessionID,
		TurnID:    turn.ID,
		Payload: map[string]any{
			"error_id":         recoverable.ID,
			"kind":             recoverable.Kind,
			"retry_eligible":   recoverable.RetryEligible,
			"compact_eligible": recoverable.CompactEligible,
			"user_action":      recoverable.UserAction,
			"provider":         recoverable.Provider,
			"model":            recoverable.Model,
		},
	})
	return recoverable, true
}

func (r *runtimeService) maybeStartReactiveProviderRecovery(ctx context.Context, turn RuntimeTurn, recoverable RuntimeRecoverableError, sourceMetadata map[string]string) {
	if recoverable.Kind != recoverableErrorContextLengthExceeded || !recoverable.CompactEligible {
		return
	}
	if compactRetryCount(sourceMetadata) >= 1 {
		return
	}
	input, err := r.userInputs.GetByTurn(ctx, turn.ID)
	if err != nil {
		r.storeRuntimeEvent(runtimeapi.Event{
			ID:        newRuntimeEventID(),
			Type:      runtimeapi.EventRecoveryRetryFailed,
			CreatedAt: time.Now().UTC().Format(time.RFC3339Nano),
			SessionID: turn.SessionID,
			TurnID:    turn.ID,
			Payload: map[string]any{
				"error_id": recoverable.ID,
				"kind":     recoverable.Kind,
				"error":    "original runtime input is unavailable: " + err.Error(),
			},
		})
		return
	}
	if len(input.Attachments) > 0 {
		r.storeRuntimeEvent(runtimeapi.Event{
			ID:        newRuntimeEventID(),
			Type:      runtimeapi.EventRecoveryRetryFailed,
			CreatedAt: time.Now().UTC().Format(time.RFC3339Nano),
			SessionID: turn.SessionID,
			TurnID:    turn.ID,
			Payload: map[string]any{
				"error_id": recoverable.ID,
				"kind":     recoverable.Kind,
				"error":    "current input has attachments; automatic compact retry would drop user-provided media",
			},
		})
		return
	}
	prompt := strings.TrimSpace(input.Prompt)
	if prompt == "" {
		r.storeRuntimeEvent(runtimeapi.Event{
			ID:        newRuntimeEventID(),
			Type:      runtimeapi.EventRecoveryRetryFailed,
			CreatedAt: time.Now().UTC().Format(time.RFC3339Nano),
			SessionID: turn.SessionID,
			TurnID:    turn.ID,
			Payload: map[string]any{
				"error_id": recoverable.ID,
				"kind":     recoverable.Kind,
				"error":    "original runtime input has no retryable prompt text",
			},
		})
		return
	}
	r.storeRuntimeEvent(runtimeapi.Event{
		ID:        newRuntimeEventID(),
		Type:      runtimeapi.EventRecoveryRetryStarted,
		CreatedAt: time.Now().UTC().Format(time.RFC3339Nano),
		SessionID: turn.SessionID,
		TurnID:    turn.ID,
		Payload: map[string]any{
			"error_id": recoverable.ID,
			"kind":     recoverable.Kind,
			"strategy": "compact_retry",
		},
	})
	r.storeRuntimeEvent(runtimeapi.Event{
		ID:        newRuntimeEventID(),
		Type:      runtimeapi.EventRecoveryCompactRetryStarted,
		CreatedAt: time.Now().UTC().Format(time.RFC3339Nano),
		SessionID: turn.SessionID,
		TurnID:    turn.ID,
		Payload:   map[string]any{"error_id": recoverable.ID, "strategy": "compact_retry"},
	})
	if _, compactErr := r.ManualCompact(ctx, RuntimeContextActionRequest{SessionID: turn.SessionID, TurnID: turn.ID, Reason: "provider_context_length_reactive_retry"}); compactErr != nil {
		r.storeRuntimeEvent(runtimeapi.Event{
			ID:        newRuntimeEventID(),
			Type:      runtimeapi.EventRecoveryCompactRetryFailed,
			CreatedAt: time.Now().UTC().Format(time.RFC3339Nano),
			SessionID: turn.SessionID,
			TurnID:    turn.ID,
			Payload:   map[string]any{"error_id": recoverable.ID, "error": compactErr.Error()},
		})
		r.storeRuntimeEvent(runtimeapi.Event{
			ID:        newRuntimeEventID(),
			Type:      runtimeapi.EventRecoveryRetryFailed,
			CreatedAt: time.Now().UTC().Format(time.RFC3339Nano),
			SessionID: turn.SessionID,
			TurnID:    turn.ID,
			Payload:   map[string]any{"error_id": recoverable.ID, "kind": recoverable.Kind, "error": compactErr.Error()},
		})
		return
	}
	r.storeRuntimeEvent(runtimeapi.Event{
		ID:        newRuntimeEventID(),
		Type:      runtimeapi.EventRecoveryCompactRetryCompleted,
		CreatedAt: time.Now().UTC().Format(time.RFC3339Nano),
		SessionID: turn.SessionID,
		TurnID:    turn.ID,
		Payload:   map[string]any{"error_id": recoverable.ID},
	})
	metadata := cloneStringMap(sourceMetadata)
	if metadata == nil {
		metadata = map[string]string{}
	}
	metadata["recovery_source_turn_id"] = turn.ID
	metadata["recovery_action"] = runtimeRecoveryActionProviderCompactRetry
	metadata["recovery_error_id"] = recoverable.ID
	metadata["recovery_error_kind"] = recoverable.Kind
	metadata[runtimeRecoveryCompactRetryCountKey] = "1"
	chat, retryErr := r.SubmitUserInput(ctx, RuntimeUserInputRequest{
		SessionID: turn.SessionID,
		Mode:      runtimeInputModePrompt,
		Items: []RuntimeUserInputItem{{
			Type:     runtimeInputItemText,
			Text:     prompt,
			Metadata: metadata,
		}},
		Options: RuntimeUserInputOptions{SkipSlashCommands: true},
	})
	if retryErr != nil {
		r.storeRuntimeEvent(runtimeapi.Event{
			ID:        newRuntimeEventID(),
			Type:      runtimeapi.EventRecoveryRetryFailed,
			CreatedAt: time.Now().UTC().Format(time.RFC3339Nano),
			SessionID: turn.SessionID,
			TurnID:    turn.ID,
			Payload:   map[string]any{"error_id": recoverable.ID, "kind": recoverable.Kind, "error": retryErr.Error()},
		})
		return
	}
	r.storeRuntimeEvent(runtimeapi.Event{
		ID:        newRuntimeEventID(),
		Type:      runtimeapi.EventRecoveryRetryCompleted,
		CreatedAt: time.Now().UTC().Format(time.RFC3339Nano),
		SessionID: turn.SessionID,
		TurnID:    chat.TurnID,
		Payload:   map[string]any{"error_id": recoverable.ID, "retry_turn_id": chat.TurnID, "strategy": "compact_retry"},
	})
}

func compactRetryCount(metadata map[string]string) int {
	raw := strings.TrimSpace(metadata[runtimeRecoveryCompactRetryCountKey])
	if raw == "" {
		return 0
	}
	count, err := strconv.Atoi(raw)
	if err != nil || count < 0 {
		return 0
	}
	return count
}
