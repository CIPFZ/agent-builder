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
	tuiModel.applyRuntimeEvent(clientEventFromRuntimeEvent(runtime.RuntimeEvent{
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
	}))

	view := tuiModel.View()

	for _, want := range []string{
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
	if strings.Contains(view, "assistant:") {
		t.Fatalf("view still contains assistant label: %q", view)
	}
}

func TestRendererRendersRichUserMessageBlocksIncludingImages(t *testing.T) {
	tuiModel := NewModel(&fakeBridge{})
	tuiModel.transcript = []transcriptEntry{
		{
			Role: "user",
			Blocks: []model.MessageBlock{
				{Type: model.MessageBlockText, Text: "Please inspect this screenshot"},
				{
					Type: "image",
					Raw: map[string]any{
						"type": "image",
						"source": map[string]any{
							"type":       "base64",
							"media_type": "image/png",
							"data":       "abc123",
						},
					},
				},
			},
		},
	}

	view := tuiModel.View()

	for _, want := range []string{
		"> Please inspect this screenshot",
		"Please inspect this screenshot",
		"image",
		"image/png",
	} {
		if !strings.Contains(view, want) {
			t.Fatalf("view missing %q: %q", want, view)
		}
	}
	if strings.Contains(view, "user:") {
		t.Fatalf("view still contains user label: %q", view)
	}
}

func TestRendererRendersTranscriptToolResultBlocksSeparateFromLiveToolProgress(t *testing.T) {
	tuiModel := NewModel(&fakeBridge{})
	tuiModel.transcript = []transcriptEntry{
		{
			Role: "tool",
			Blocks: []model.MessageBlock{
				{
					Type:      model.MessageBlockToolResult,
					ToolUseID: "toolu-1",
					Content:   "README contents",
				},
			},
		},
	}

	view := tuiModel.View()

	for _, want := range []string{
		"tool result",
		"README contents",
	} {
		if !strings.Contains(view, want) {
			t.Fatalf("view missing %q: %q", want, view)
		}
	}
	if strings.Contains(view, "tool[running]") {
		t.Fatalf("view = %q, did not want runtime tool-progress chrome for transcript tool_result", view)
	}
}

func TestTUIStateCreatesSpecialMessageBlocksForRuntimeEvents(t *testing.T) {
	state := newTUIState()

	state.applyRuntimeEvent(clientEventFromRuntimeEvent(runtime.RuntimeEvent{Type: "run.error", Error: "model failed"}))
	state.applyRuntimeEvent(clientEventFromRuntimeEvent(runtime.RuntimeEvent{Type: "compact.boundary"}))
	state.applyRuntimeEvent(clientEventFromRuntimeEvent(runtime.RuntimeEvent{Type: "compact.cleaned"}))
	state.applyRuntimeEvent(clientEventFromRuntimeEvent(runtime.RuntimeEvent{
		Type: "message.created",
		Message: &session.Message{
			Role:    "system",
			Subtype: "compact_boundary",
			Content: "Conversation compacted",
		},
	}))
	state.applyRuntimeEvent(clientEventFromRuntimeEvent(runtime.RuntimeEvent{
		Type: "message.created",
		Message: &session.Message{
			Role:    "user",
			Content: "<local-command-stdout>\nok\n</local-command-stdout>\n<local-command-stderr>\nwarn\n</local-command-stderr>",
		},
	}))
	state.applyRuntimeEvent(clientEventFromRuntimeEvent(runtime.RuntimeEvent{
		Type: "message.created",
		Message: &session.Message{
			Role:             "user",
			Content:          "Compacted earlier context into a summary",
			IsCompactSummary: true,
		},
	}))
	state.applyRuntimeEvent(clientEventFromRuntimeEvent(runtime.RuntimeEvent{
		Type: "message.created",
		Message: &session.Message{
			Role:    "system",
			Content: "Session restored",
		},
	}))
	state.applyRuntimeEvent(clientEventFromRuntimeEvent(runtime.RuntimeEvent{
		Type: "message.created",
		Message: &session.Message{
			Role:    "user",
			Content: "<bash-stdout>\npkg ok\n</bash-stdout><bash-stderr>\nwarn\n</bash-stderr>",
		},
	}))

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

func TestTUIStatePreservesToolResultBlocksFromMessageCreated(t *testing.T) {
	state := newTUIState()

	state.applyRuntimeEvent(clientEventFromRuntimeEvent(runtime.RuntimeEvent{
		Type: "message.created",
		Message: &session.Message{
			Role: "tool",
			Blocks: []model.MessageBlock{
				{
					Type:      model.MessageBlockToolResult,
					ToolUseID: "toolu-1",
					Content:   "README contents",
				},
			},
		},
	}))

	if len(state.transcript) != 1 {
		t.Fatalf("transcript len = %d, want 1", len(state.transcript))
	}
	if len(state.transcript[0].Blocks) != 1 || state.transcript[0].Blocks[0].Type != model.MessageBlockToolResult {
		t.Fatalf("tool transcript = %#v, want preserved tool_result block", state.transcript[0])
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

func TestTranscriptSearchIndexesRichBlockRendering(t *testing.T) {
	tuiModel := NewModel(&fakeBridge{})
	tuiModel.transcript = []transcriptEntry{
		{
			Role: "user",
			Blocks: []model.MessageBlock{
				{Type: model.MessageBlockText, Text: "Please inspect this screenshot"},
				{
					Type: "image",
					Raw: map[string]any{
						"type": "image",
						"source": map[string]any{
							"type":       "base64",
							"media_type": "image/png",
						},
					},
				},
			},
		},
	}

	tuiModel.startTranscriptSearch()
	tuiModel.appendTranscriptSearchQuery("image/png")

	if tuiModel.viewport.Search.MatchCount != 1 {
		t.Fatalf("match count = %d, want 1", tuiModel.viewport.Search.MatchCount)
	}
}
