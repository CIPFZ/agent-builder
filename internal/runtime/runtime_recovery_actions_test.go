package runtime

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"slices"
	"testing"
	"time"

	"github.com/CIPFZ/agent-builder/internal/runtimeapi"
	"github.com/CIPFZ/agent-builder/internal/tools/scheduler"
)

func TestRuntimeRecoveryStatusClassifiesTurnsErrorsAndActions(t *testing.T) {
	service, cleanup := runtimeRecoveryActionTestService(t)
	defer cleanup()
	ctx := context.Background()
	sessionID := runtimeRecoveryActionTestSession(t, service, "recovery status")

	_, err := service.turns.Upsert(ctx, RuntimeTurn{
		ID:         "turn-tool",
		SessionID:  sessionID,
		Status:     turnStatusInterrupted,
		StartedAt:  1000,
		FinishedAt: 2000,
		Error:      "runtime restarted before turn completed",
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := service.toolCalls.CreateCall(ctx, scheduler.ToolCallRequest{ID: "tool-open", SessionID: sessionID, TurnID: "turn-tool", Name: "bash"}); err != nil {
		t.Fatal(err)
	}
	_, err = service.turns.Upsert(ctx, RuntimeTurn{
		ID:            "turn-rate-limit",
		SessionID:     sessionID,
		Status:        turnStatusFailed,
		Provider:      "openai",
		Model:         "test-model",
		PromptPreview: "hello",
		StartedAt:     3000,
		FinishedAt:    4000,
		Error:         "provider returned 429 rate limit",
	})
	if err != nil {
		t.Fatal(err)
	}

	status, err := service.RecoveryStatus(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(status.InterruptedTurns) != 1 || status.InterruptedTurns[0].InterruptionKind != interruptionKindToolCall || !status.InterruptedTurns[0].ResumeEligible {
		t.Fatalf("interrupted turns = %#v", status.InterruptedTurns)
	}
	if len(status.RecoverableErrors) != 1 || status.RecoverableErrors[0].Kind != recoverableErrorRateLimited || !status.RecoverableErrors[0].RetryEligible {
		t.Fatalf("recoverable errors = %#v", status.RecoverableErrors)
	}
	if !slices.ContainsFunc(status.Actions, func(action RuntimeRecoveryAction) bool {
		return action.Kind == runtimeRecoveryActionResumeInterruptedTurn && action.TurnID == "turn-tool" && action.StartsWorker
	}) {
		t.Fatalf("resume action missing from %#v", status.Actions)
	}
	if !slices.ContainsFunc(status.Actions, func(action RuntimeRecoveryAction) bool {
		return action.Kind == runtimeRecoveryActionRetryRecoverableError && action.TurnID == "turn-rate-limit" && action.StartsWorker
	}) {
		t.Fatalf("retry action missing from %#v", status.Actions)
	}
}

func TestRuntimeServiceResumeInterruptedTurnCreatesNewTurnLinkAndMetadata(t *testing.T) {
	service, cleanup := runtimeRecoveryActionTestService(t)
	defer cleanup()
	ctx := context.Background()
	sessionID := runtimeRecoveryActionTestSession(t, service, "resume interrupted")

	run, err := service.runs.EnsureForSession(ctx, service.workspace.ID, sessionID, "resume source", runtimeRunSourceUserPrompt)
	if err != nil {
		t.Fatal(err)
	}
	source, err := service.turns.Upsert(ctx, RuntimeTurn{
		ID:                       "turn-source",
		SessionID:                sessionID,
		Status:                   turnStatusInterrupted,
		Provider:                 "openai",
		Model:                    "test-model",
		PromptPreview:            "resume work",
		LatestAssistantMessageID: "assistant-partial",
		StartedAt:                1000,
		FinishedAt:               2000,
		Error:                    "runtime restarted during generation",
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := service.runs.LinkTurn(ctx, run.ID, sessionID, source.ID, source.StartedAt); err != nil {
		t.Fatal(err)
	}

	resp, err := service.ResumeInterruptedTurn(ctx, source.ID, RuntimeResumeInterruptedTurnRequest{Mode: "continue", Prompt: "prefer a short answer"})
	if err != nil {
		t.Fatal(err)
	}
	if resp.Turn.ID == "" || resp.Turn.ID == source.ID || resp.Turn.SessionID == "" {
		t.Fatalf("resumed turn = %#v sourceSession=%q", resp.Turn, source.SessionID)
	}
	if resp.Action == nil || !resp.Action.Accepted || resp.Action.Source.Action != runtimeRecoveryActionResumeInterruptedTurn || resp.Action.Source.IdempotentBy != "source_turn_id" || !resp.Action.Source.StartsWorker {
		t.Fatalf("action metadata = %#v", resp.Action)
	}
	links, err := service.recoveryLinks.ListBySource(ctx, source.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(links) != 1 || links[0].ResumedTurnID != resp.Turn.ID || links[0].InterruptionKind != interruptionKindGeneration {
		t.Fatalf("links = %#v", links)
	}
	storedSource, err := service.turns.Get(ctx, source.ID)
	if err != nil {
		t.Fatal(err)
	}
	if storedSource.Status != turnStatusInterrupted {
		t.Fatalf("source turn should remain interrupted, got %#v", storedSource)
	}
	input, err := service.userInputs.GetByTurn(ctx, resp.Turn.ID)
	if err != nil {
		t.Fatal(err)
	}
	meta := input.Messages[0].Metadata
	if meta["recovery_source_turn_id"] != source.ID || meta["recovery_action"] != runtimeRecoveryActionResumeInterruptedTurn || meta["recovery_interruption_kind"] != interruptionKindGeneration {
		t.Fatalf("recovery metadata = %#v", meta)
	}
	events, err := service.Events(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if !slices.ContainsFunc(events.Events, func(event RuntimeEvent) bool {
		return event.Type == runtimeapi.EventRecoveryTurnResumed && event.TurnID == resp.Turn.ID
	}) {
		t.Fatalf("resume event missing from %#v", events.Events)
	}
}

func TestRuntimeServiceDiscardInterruptedTurnWritesStructuredRecoveryAction(t *testing.T) {
	service, cleanup := runtimeRecoveryActionTestService(t)
	defer cleanup()
	ctx := context.Background()
	sessionID := runtimeRecoveryActionTestSession(t, service, "discard interrupted")
	_, err := service.turns.Upsert(ctx, RuntimeTurn{
		ID:         "turn-discard",
		SessionID:  sessionID,
		Status:     turnStatusInterrupted,
		Provider:   "openai",
		Model:      "test-model",
		StartedAt:  1000,
		FinishedAt: 2000,
	})
	if err != nil {
		t.Fatal(err)
	}

	resp, err := service.DiscardInterruptedTurn(ctx, "turn-discard")
	if err != nil {
		t.Fatal(err)
	}
	if resp.Turn.Status != turnStatusCancelled || resp.Turn.Error != "interrupted_turn_discarded" {
		t.Fatalf("discarded turn = %#v", resp.Turn)
	}
	if resp.Action == nil || !resp.Action.Accepted || resp.Action.Source.Action != runtimeRecoveryActionDiscardInterruptedTurn || resp.Action.Source.IdempotentBy != "turn_id" || resp.Action.Source.StartsWorker {
		t.Fatalf("discard action metadata = %#v", resp.Action)
	}
	events, err := service.Events(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if !slices.ContainsFunc(events.Events, func(event RuntimeEvent) bool {
		return event.Type == runtimeapi.EventRecoveryTurnDiscarded && event.TurnID == "turn-discard"
	}) {
		t.Fatalf("discard event missing from %#v", events.Events)
	}
}

func TestRuntimeServiceRetryRecoverableErrorStartsNewTurnFromStoredInput(t *testing.T) {
	service, cleanup := runtimeRecoveryActionTestService(t)
	defer cleanup()
	ctx := context.Background()
	sessionID := runtimeRecoveryActionTestSession(t, service, "retry recoverable")
	turn, err := service.turns.Upsert(ctx, RuntimeTurn{
		ID:            "turn-failed",
		SessionID:     sessionID,
		Status:        turnStatusFailed,
		Provider:      "openai",
		Model:         "test-model",
		PromptPreview: "preview prompt",
		StartedAt:     1000,
		FinishedAt:    2000,
		Error:         "rate limit exceeded",
	})
	if err != nil {
		t.Fatal(err)
	}
	_, err = service.userInputs.Upsert(ctx, RuntimeNormalizedInput{
		ID:          "input-original",
		SessionID:   sessionID,
		Mode:        runtimeInputModePrompt,
		Prompt:      "full original prompt",
		ShouldQuery: true,
		CreatedAt:   time.Now().UnixMilli(),
		Messages: []RuntimeMessageDraft{{
			Role:     "user",
			Content:  "full original prompt",
			Metadata: map[string]string{"inputMode": runtimeInputModePrompt},
		}},
	}, []RuntimeUserInputItem{{Type: runtimeInputItemText, Text: "full original prompt"}}, turn.ID)
	if err != nil {
		t.Fatal(err)
	}

	resp, err := service.RetryRecoverableError(ctx, "turn:"+turn.ID)
	if err != nil {
		t.Fatal(err)
	}
	if resp.Chat.TurnID == "" || resp.Chat.TurnID == turn.ID {
		t.Fatalf("retry response = %#v", resp)
	}
	if resp.Action == nil || !resp.Action.Accepted || resp.Action.Source.Action != runtimeRecoveryActionRetryRecoverableError || resp.Action.Source.IdempotentBy != "error_id" || !resp.Action.Source.StartsWorker {
		t.Fatalf("retry action metadata = %#v", resp.Action)
	}
	input, err := service.userInputs.GetByTurn(ctx, resp.Chat.TurnID)
	if err != nil {
		t.Fatal(err)
	}
	if input.Prompt != "full original prompt" || input.Messages[0].Metadata["recovery_error_kind"] != recoverableErrorRateLimited {
		t.Fatalf("retry input = %#v", input)
	}
}

func runtimeRecoveryActionTestService(t *testing.T) (*runtimeService, func()) {
	t.Helper()
	root := runtimeDevTestRoot(t, "recovery-actions-"+t.Name())
	t.Setenv("AGENT_BUILDER_DESKTOP_ROOT", root)
	provider := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"id": "chatcmpl-recovery-test",
			"choices": []map[string]any{{
				"message": map[string]any{"role": "assistant", "content": "ok"},
			}},
			"usage": map[string]any{"prompt_tokens": 1, "completion_tokens": 1, "total_tokens": 2},
		})
	}))
	writeRuntimeDevModelConfig(t, root, provider.URL)
	service := newRuntimeService()
	if err := service.ensureWorkspaceStarted(context.Background(), false); err != nil {
		provider.Close()
		t.Fatal(err)
	}
	return service, provider.Close
}

func runtimeRecoveryActionTestSession(t *testing.T, service *runtimeService, title string) string {
	t.Helper()
	session, err := service.runtime.CreateSession(context.Background(), service.workspace.ID, title)
	if err != nil {
		t.Fatal(err)
	}
	return session.ID
}
