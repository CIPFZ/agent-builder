package tui

import (
	"strings"
	"testing"

	"myclaw/internal/model"
	"myclaw/internal/runtime"
	"myclaw/internal/session"
	"myclaw/internal/tools"
)

func TestTUIStateTracksToolProgressLifecycleByToolUseID(t *testing.T) {
	state := newTUIState()

	state.applyRuntimeEvent(clientEventFromRuntimeEvent(runtime.RuntimeEvent{
		Type:      "tool.called",
		RunID:     "run-1",
		ToolUseID: "toolu-read",
		ToolName:  "Read",
		ToolInput: `{"file_path":"README.md"}`,
	}))
	state.applyRuntimeEvent(clientEventFromRuntimeEvent(runtime.RuntimeEvent{
		Type:      "tool.called",
		RunID:     "run-1",
		ToolUseID: "toolu-bash",
		ToolName:  "Bash",
		ToolInput: `{"command":"go test ./..."}`,
	}))
	state.applyRuntimeEvent(clientEventFromRuntimeEvent(runtime.RuntimeEvent{
		Type: "tool.progress",
		Progress: &tools.ToolProgress{
			ToolUseID: "toolu-read",
			Type:      "read.progress",
			Message:   "Reading README.md",
		},
	}))
	state.applyRuntimeEvent(clientEventFromRuntimeEvent(runtime.RuntimeEvent{
		Type:      "tool.result",
		RunID:     "run-1",
		ToolUseID: "toolu-read",
		ToolName:  "Read",
		Message: &session.Message{
			Role:    "tool",
			Content: "Read: hello",
			Blocks: []model.MessageBlock{{
				Type:      model.MessageBlockToolResult,
				ToolUseID: "toolu-read",
				Content:   "hello",
			}},
		},
	}))

	if len(state.transcript) != 2 {
		t.Fatalf("transcript len = %d, want 2: %#v", len(state.transcript), state.transcript)
	}
	read := state.transcript[0]
	if read.ToolUseID != "toolu-read" || read.ToolStatus != toolStatusSucceeded {
		t.Fatalf("read entry = %#v, want toolu-read succeeded", read)
	}
	if read.ToolProgressMessage != "Reading README.md" {
		t.Fatalf("read progress = %q, want Reading README.md", read.ToolProgressMessage)
	}
	if read.Content != "hello" {
		t.Fatalf("read content = %q, want hello", read.Content)
	}
	bash := state.transcript[1]
	if bash.ToolUseID != "toolu-bash" || bash.ToolStatus != toolStatusRunning {
		t.Fatalf("bash entry = %#v, want toolu-bash still running", bash)
	}
}

func TestTUIStateMarksToolResultErrors(t *testing.T) {
	state := newTUIState()
	state.applyRuntimeEvent(clientEventFromRuntimeEvent(runtime.RuntimeEvent{
		Type:      "tool.called",
		ToolUseID: "toolu-bash",
		ToolName:  "Bash",
		ToolInput: "bad",
	}))

	state.applyRuntimeEvent(clientEventFromRuntimeEvent(runtime.RuntimeEvent{
		Type:      "tool.result",
		ToolUseID: "toolu-bash",
		ToolName:  "Bash",
		ToolError: true,
		Message: &session.Message{
			Role:    "tool",
			Content: "Bash: exit status 1",
			Blocks: []model.MessageBlock{{
				Type:      model.MessageBlockToolResult,
				ToolUseID: "toolu-bash",
				Content:   "exit status 1",
				IsError:   true,
			}},
		},
	}))

	if len(state.transcript) != 1 {
		t.Fatalf("transcript len = %d, want 1", len(state.transcript))
	}
	entry := state.transcript[0]
	if entry.ToolStatus != toolStatusFailed || !entry.ToolError {
		t.Fatalf("entry = %#v, want failed error", entry)
	}
}

func TestRendererShowsToolProgressStates(t *testing.T) {
	tuiModel := NewModel(&fakeBridge{})
	tuiModel.applyRuntimeEvent(clientEventFromRuntimeEvent(runtime.RuntimeEvent{
		Type:      "tool.called",
		ToolUseID: "toolu-bash",
		ToolName:  "Bash",
		ToolInput: `{"command":"go test ./..."}`,
	}))
	tuiModel.applyRuntimeEvent(clientEventFromRuntimeEvent(runtime.RuntimeEvent{
		Type: "tool.progress",
		Progress: &tools.ToolProgress{
			ToolUseID: "toolu-bash",
			Type:      "shell.progress",
			Message:   "running tests",
			Data: map[string]any{
				"output": "pkg/a ok\npkg/b ok\npkg/c ok\npkg/d ok\npkg/e ok\npkg/f ok",
			},
		},
	}))
	view := tuiModel.View()
	for _, want := range []string{"tool[running]", "Bash", "running tests", "pkg/b ok", "pkg/f ok"} {
		if !strings.Contains(view, want) {
			t.Fatalf("running view missing %q: %q", want, view)
		}
	}

	tuiModel.applyRuntimeEvent(clientEventFromRuntimeEvent(runtime.RuntimeEvent{
		Type:      "tool.result",
		ToolUseID: "toolu-bash",
		ToolName:  "Bash",
		Message: &session.Message{
			Role:    "tool",
			Content: "Bash: ok",
			Blocks: []model.MessageBlock{{
				Type:      model.MessageBlockToolResult,
				ToolUseID: "toolu-bash",
				Content:   "ok",
			}},
		},
	}))

	view = tuiModel.View()
	for _, want := range []string{"tool[succeeded]", "Bash", "ok"} {
		if !strings.Contains(view, want) {
			t.Fatalf("succeeded view missing %q: %q", want, view)
		}
	}
}
