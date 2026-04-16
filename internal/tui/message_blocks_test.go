package tui

import (
	"strings"
	"testing"

	"myclaw/internal/model"
	"myclaw/internal/runtime"
	"myclaw/internal/session"
)

func TestRendererRendersStructuredAssistantMessageBlocks(t *testing.T) {
	tuiModel := NewModel(&fakeBridge{})
	tuiModel.applyRuntimeEvent(runtime.RuntimeEvent{
		Type: "message.created",
		Message: &session.Message{
			Role:    "assistant",
			Content: "I will inspect the repo.",
			Blocks: []model.MessageBlock{
				{Type: model.MessageBlockThinking, Text: "Need to inspect files"},
				{Type: model.MessageBlockText, Text: "I will inspect the repo."},
				{Type: model.MessageBlockToolUse, ID: "toolu-1", Name: "Read", InputObject: map[string]any{"file_path": "README.md"}},
			},
		},
	})

	view := tuiModel.View()

	for _, want := range []string{
		"assistant",
		"thinking",
		"Need to inspect files",
		"I will inspect the repo.",
		"tool use",
		"Read",
		"file_path=README.md",
	} {
		if !strings.Contains(view, want) {
			t.Fatalf("view missing %q: %q", want, view)
		}
	}
}

func TestTUIStateCreatesSpecialMessageBlocksForRuntimeEvents(t *testing.T) {
	state := newTUIState()

	state.applyRuntimeEvent(runtime.RuntimeEvent{Type: "run.error", Error: "model failed"})
	state.applyRuntimeEvent(runtime.RuntimeEvent{Type: "compact.boundary"})
	state.applyRuntimeEvent(runtime.RuntimeEvent{Type: "compact.cleaned"})
	state.applyRuntimeEvent(runtime.RuntimeEvent{
		Type: "message.created",
		Message: &session.Message{
			Role:    "system",
			Subtype: "compact_boundary",
			Content: "Conversation compacted",
		},
	})
	state.applyRuntimeEvent(runtime.RuntimeEvent{
		Type: "message.created",
		Message: &session.Message{
			Role:    "user",
			Content: "<local-command-stdout>\nok\n</local-command-stdout>\n<local-command-stderr>\nwarn\n</local-command-stderr>",
		},
	})
	state.applyRuntimeEvent(runtime.RuntimeEvent{
		Type: "message.created",
		Message: &session.Message{
			Role:             "user",
			Content:          "Compacted earlier context into a summary",
			IsCompactSummary: true,
		},
	})
	state.applyRuntimeEvent(runtime.RuntimeEvent{
		Type: "message.created",
		Message: &session.Message{
			Role:    "system",
			Content: "Session restored",
		},
	})
	state.applyRuntimeEvent(runtime.RuntimeEvent{
		Type: "message.created",
		Message: &session.Message{
			Role:    "user",
			Content: "<bash-stdout>\npkg ok\n</bash-stdout><bash-stderr>\nwarn\n</bash-stderr>",
		},
	})

	if len(state.transcript) != 8 {
		t.Fatalf("transcript len = %d, want 8: %#v", len(state.transcript), state.transcript)
	}
	wants := []string{messageKindError, messageKindCompact, messageKindCompact, messageKindCompact, messageKindLocalCommand, messageKindCompact, messageKindSystem, messageKindLocalCommand}
	for i, want := range wants {
		if state.transcript[i].Kind != want {
			t.Fatalf("entry %d kind = %q, want %q: %#v", i, state.transcript[i].Kind, want, state.transcript[i])
		}
	}
	if state.transcript[4].LocalStdout != "ok" || state.transcript[4].LocalStderr != "warn" {
		t.Fatalf("local command entry = %#v, want stdout/stderr extracted", state.transcript[4])
	}
	if state.transcript[7].LocalStdout != "pkg ok" || state.transcript[7].LocalStderr != "warn" {
		t.Fatalf("bash output entry = %#v, want stdout/stderr extracted", state.transcript[7])
	}
}

func TestRendererRendersSpecialMessageBlocks(t *testing.T) {
	tuiModel := NewModel(&fakeBridge{})
	tuiModel.transcript = []transcriptEntry{
		{Kind: messageKindError, Role: "system", Content: "model failed"},
		{Kind: messageKindCompact, Role: "system", Content: "Conversation compacted"},
		{Kind: messageKindSystem, Role: "system", Content: "Session restored"},
		{Kind: messageKindLocalCommand, Role: "user", LocalStdout: "ok", LocalStderr: "warn"},
		{Role: "tool", ToolName: "Bash", ToolInputObject: map[string]any{"command": "go test ./..."}, ToolStatus: toolStatusFailed, ToolError: true, Content: "exit status 1"},
	}

	view := tuiModel.View()

	for _, want := range []string{
		"error",
		"model failed",
		"compact",
		"Conversation compacted",
		"system",
		"Session restored",
		"local command",
		"stdout",
		"ok",
		"stderr",
		"warn",
		"tool[failed]",
		"Bash",
		"command=go test ./...",
		"Error: exit status 1",
	} {
		if !strings.Contains(view, want) {
			t.Fatalf("view missing %q: %q", want, view)
		}
	}
}
