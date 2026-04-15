package tui

import (
	"testing"

	"myclaw/internal/approval"
	"myclaw/internal/runtime"
	"myclaw/internal/session"
)

func TestTUIStateInitializesFromConfig(t *testing.T) {
	state := newTUIState(ModelConfig{
		SessionID: "main-1",
		LLMLabel:  "test-model",
		LogPath:   "logs/tui.jsonl",
	})

	if state.diagnostics.SessionID != "main-1" {
		t.Fatalf("SessionID = %q, want main-1", state.diagnostics.SessionID)
	}
	if state.diagnostics.LLMLabel != "test-model" {
		t.Fatalf("LLMLabel = %q, want test-model", state.diagnostics.LLMLabel)
	}
	if state.diagnostics.LogPath != "logs/tui.jsonl" {
		t.Fatalf("LogPath = %q, want logs/tui.jsonl", state.diagnostics.LogPath)
	}
	if state.historyIndex != -1 {
		t.Fatalf("historyIndex = %d, want -1", state.historyIndex)
	}
	if len(state.transcript) != 0 {
		t.Fatalf("transcript len = %d, want 0", len(state.transcript))
	}
}

func TestTUIStateSubmitUserInputUpdatesLocalTurnState(t *testing.T) {
	state := newTUIState()
	state.input = "  hello  "
	state.cursorPos = len([]rune(state.input))
	state.suggestions = []string{"/help"}
	state.selectedIndex = 0

	text, ok := state.submitUserInput()

	if !ok || text != "hello" {
		t.Fatalf("submit = (%q, %v), want (hello, true)", text, ok)
	}
	if state.input != "" {
		t.Fatalf("input = %q, want cleared", state.input)
	}
	if state.cursorPos != 0 {
		t.Fatalf("cursorPos = %d, want 0", state.cursorPos)
	}
	if len(state.suggestions) != 0 || state.selectedIndex != -1 {
		t.Fatalf("suggestions = %#v selected = %d, want cleared", state.suggestions, state.selectedIndex)
	}
	if len(state.history) != 1 || state.history[0] != "hello" {
		t.Fatalf("history = %#v, want [hello]", state.history)
	}
	if len(state.transcript) != 1 || state.transcript[0].Role != "user" || state.transcript[0].Content != "hello" {
		t.Fatalf("transcript = %#v, want user hello", state.transcript)
	}
	if !state.busy {
		t.Fatal("busy = false, want true")
	}
}

func TestTUIStateSubmitUserInputIgnoresEmptyText(t *testing.T) {
	state := newTUIState()
	state.input = "   "
	state.suggestions = []string{"/help"}
	state.selectedIndex = 0

	text, ok := state.submitUserInput()

	if ok || text != "" {
		t.Fatalf("submit = (%q, %v), want empty false", text, ok)
	}
	if len(state.transcript) != 0 {
		t.Fatalf("transcript = %#v, want empty", state.transcript)
	}
	if state.busy {
		t.Fatal("busy = true, want false")
	}
	if len(state.suggestions) != 0 || state.selectedIndex != -1 {
		t.Fatalf("suggestions = %#v selected = %d, want cleared", state.suggestions, state.selectedIndex)
	}
}

func TestTUIStateApprovalDecisionsReturnPendingID(t *testing.T) {
	state := newTUIState()
	request := &approval.Request{ID: "approval-1", ToolName: "system.run", ToolInput: "pwd"}
	state.applyRuntimeEvent(runtime.RuntimeEvent{Type: "permission.required", Approval: request})

	approveID, ok := state.approvePending()
	if !ok || approveID != "approval-1" {
		t.Fatalf("approve = (%q, %v), want approval-1 true", approveID, ok)
	}
	if state.pendingApproval != nil {
		t.Fatalf("pending approval = %#v, want nil", state.pendingApproval)
	}
	if !state.busy {
		t.Fatal("busy = false after approve, want true")
	}

	state.applyRuntimeEvent(runtime.RuntimeEvent{Type: "permission.required", Approval: request})
	rejectID, ok := state.rejectPending()
	if !ok || rejectID != "approval-1" {
		t.Fatalf("reject = (%q, %v), want approval-1 true", rejectID, ok)
	}
	if state.pendingApproval != nil {
		t.Fatalf("pending approval = %#v, want nil", state.pendingApproval)
	}
	if state.busy {
		t.Fatal("busy = true after reject, want false")
	}
}

func TestTUIStateBridgeErrorTracksDiagnosticsAndClearsBusy(t *testing.T) {
	state := newTUIState()
	state.busy = true

	state.applyBridgeError(assertErr("boom"))

	if state.busy {
		t.Fatal("busy = true, want false")
	}
	if state.diagnostics.LastError != "boom" {
		t.Fatalf("LastError = %q, want boom", state.diagnostics.LastError)
	}
	if len(state.events) == 0 || state.events[len(state.events)-1] != "error: boom" {
		t.Fatalf("events = %#v, want trailing error", state.events)
	}
}

func TestTUIStateRuntimeReducerHandlesAssistantAndToolLifecycle(t *testing.T) {
	state := newTUIState()

	state.applyRuntimeEvent(runtime.RuntimeEvent{Type: "agent.lifecycle.start"})
	state.applyRuntimeEvent(runtime.RuntimeEvent{Type: "assistant.delta", Delta: "Hello"})
	state.applyRuntimeEvent(runtime.RuntimeEvent{Type: "assistant.delta", Delta: " world"})
	state.applyRuntimeEvent(runtime.RuntimeEvent{
		Type:    "message.created",
		Message: &session.Message{Role: "assistant", Content: "Hello world"},
	})
	state.applyRuntimeEvent(runtime.RuntimeEvent{Type: "tool.called", ToolName: "system.run", ToolInput: "pwd"})
	state.applyRuntimeEvent(runtime.RuntimeEvent{
		Type:     "tool.result",
		ToolName: "system.run",
		Message:  &session.Message{Role: "tool", Content: "/repo"},
	})

	if state.busy {
		t.Fatal("busy = true after assistant message, want false")
	}
	if state.diagnostics.EventCount != 6 {
		t.Fatalf("EventCount = %d, want 6", state.diagnostics.EventCount)
	}
	if len(state.transcript) != 2 {
		t.Fatalf("transcript len = %d, want 2: %#v", len(state.transcript), state.transcript)
	}
	if state.transcript[0].Role != "assistant" || state.transcript[0].Content != "Hello world" || state.transcript[0].Streaming {
		t.Fatalf("assistant entry = %#v, want finalized Hello world", state.transcript[0])
	}
	if state.transcript[1].Role != "tool" || state.transcript[1].ToolStatus != "result" || state.transcript[1].Content != "/repo" {
		t.Fatalf("tool entry = %#v, want result /repo", state.transcript[1])
	}
}
