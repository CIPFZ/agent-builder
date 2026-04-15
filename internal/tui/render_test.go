package tui

import (
	"testing"

	"myclaw/internal/approval"
	"myclaw/internal/runtime"
	"myclaw/internal/session"
)

func TestRenderSnapshotCopiesDisplayState(t *testing.T) {
	model := NewModel(&fakeBridge{}, ModelConfig{
		SessionID: "main-1",
		LLMLabel:  "test-model",
		LogPath:   "logs/tui.jsonl",
	})
	model.input = "hello"
	model.cursorPos = 2
	model.busy = true
	model.activity.Label = "Running turn"
	model.transcript = append(model.transcript, transcriptEntry{Role: "user", Content: "hello"})

	snapshot := newRenderSnapshot(model, 96)

	if snapshot.Width != 96 {
		t.Fatalf("Width = %d, want 96", snapshot.Width)
	}
	if snapshot.Input.Text != "hello" || snapshot.Input.Cursor != 2 {
		t.Fatalf("input snapshot = %#v, want text hello cursor 2", snapshot.Input)
	}
	if !snapshot.Busy || snapshot.Activity != "Running turn" {
		t.Fatalf("busy/activity = %v/%q, want true/Running turn", snapshot.Busy, snapshot.Activity)
	}
	if len(snapshot.Transcript) != 1 || snapshot.Transcript[0].Role != "user" {
		t.Fatalf("transcript snapshot = %#v, want copied user entry", snapshot.Transcript)
	}
	model.transcript[0].Content = "mutated"
	if snapshot.Transcript[0].Content != "hello" {
		t.Fatalf("snapshot transcript mutated to %q, want immutable copy", snapshot.Transcript[0].Content)
	}
}

func TestRendererSectionsExposeTranscriptApprovalAndPrompt(t *testing.T) {
	model := NewModel(&fakeBridge{})
	model.applyRuntimeEvent(runtime.RuntimeEvent{Type: "assistant.delta", Delta: "working"})
	model.applyRuntimeEvent(runtime.RuntimeEvent{Type: "tool.called", ToolName: "system.run", ToolInput: "pwd"})
	model.applyRuntimeEvent(runtime.RuntimeEvent{
		Type:     "tool.result",
		ToolName: "system.run",
		Message:  &session.Message{Role: "tool", Content: "/repo"},
	})
	model.applyRuntimeEvent(runtime.RuntimeEvent{
		Type:     "permission.required",
		Approval: &approval.Request{ID: "approval-1", ToolName: "system.run", ToolInput: "pwd"},
	})

	view := newRenderer().renderScreen(newRenderSnapshot(model, 88))

	for _, want := range []string{
		"myclaw",
		"Welcome back",
		"assistant",
		"working",
		"tool",
		"system.run",
		"/repo",
		"Permission Required",
		"Ctrl+Y approve",
	} {
		if !contains(view, want) {
			t.Fatalf("view missing %q: %q", want, view)
		}
	}
}

func TestRendererFooterSwitchesHelpForApproval(t *testing.T) {
	model := NewModel(&fakeBridge{})
	plain := newRenderer().renderPrompt(newRenderSnapshot(model, 80))
	if !contains(plain, "Enter to send") {
		t.Fatalf("plain prompt missing Enter help: %q", plain)
	}

	model.pendingApproval = &approval.Request{ID: "approval-1"}
	approvalPrompt := newRenderer().renderPrompt(newRenderSnapshot(model, 80))
	if !contains(approvalPrompt, "Ctrl+Y approve") || contains(approvalPrompt, "Enter to send") {
		t.Fatalf("approval prompt help = %q, want approval shortcuts only", approvalPrompt)
	}
}

func TestRenderInputWithCursorPreservesUnicodeRunes(t *testing.T) {
	rendered := renderInputWithCursor("你好", 1, 40)

	if !contains(rendered, "你") || !contains(rendered, "好") {
		t.Fatalf("rendered input = %q, want both CJK runes preserved", rendered)
	}
}
