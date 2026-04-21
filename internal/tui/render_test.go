package tui

import (
	"regexp"
	"strings"
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
		"MYCLAW",
		"Commands: /help  /clear  /model",
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

	model.applyRuntimeEvent(runtime.RuntimeEvent{
		Type:     "permission.required",
		Approval: &approval.Request{ID: "approval-1"},
	})
	approvalPrompt := newRenderer().renderPrompt(newRenderSnapshot(model, 80))
	if !contains(approvalPrompt, "Esc reject") || contains(approvalPrompt, "Enter to send") {
		t.Fatalf("approval prompt help = %q, want approval dialog shortcuts only", approvalPrompt)
	}
}

func TestRenderApprovalDialogUsesTypedSemanticCopy(t *testing.T) {
	tests := []struct {
		name    string
		request approval.Request
		wants   []string
	}{
		{
			name: "shell command",
			request: approval.Request{
				ID:        "approval-1",
				ToolName:  "system.run",
				ToolInput: "git status",
				Reason:    "dangerous command requires explicit approval",
			},
			wants: []string{"Command Approval", "Command: git status", "Tool: system.run"},
		},
		{
			name: "file edit",
			request: approval.Request{
				ID:              "approval-1",
				ToolName:        "Edit",
				Reason:          "destructive tool action requires explicit approval",
				ToolInputObject: map[string]any{"file_path": "internal/tui/render.go"},
			},
			wants: []string{"File Edit Approval", "Path: internal/tui/render.go", "Tool: Edit"},
		},
		{
			name: "filesystem read",
			request: approval.Request{
				ID:              "approval-1",
				ToolName:        "Read",
				ToolInputObject: map[string]any{"file_path": "README.md"},
			},
			wants: []string{"Read Approval", "Path: README.md", "Tool: Read"},
		},
		{
			name: "web fetch",
			request: approval.Request{
				ID:              "approval-1",
				ToolName:        "WebFetch",
				ToolInputObject: map[string]any{"url": "https://example.com"},
			},
			wants: []string{"Web Fetch Approval", "URL: https://example.com", "Tool: WebFetch"},
		},
		{
			name: "skill",
			request: approval.Request{
				ID:              "approval-1",
				ToolName:        "Skill",
				ToolInputObject: map[string]any{"skill": "brainstorming"},
			},
			wants: []string{"Skill Approval", "Skill: brainstorming", "Tool: Skill"},
		},
		{
			name: "generic fallback",
			request: approval.Request{
				ID:              "approval-1",
				ToolName:        "UnknownTool",
				ToolInputObject: map[string]any{"foo": "bar"},
			},
			wants: []string{"Permission Required", "Tool: UnknownTool"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			model := NewModel(&fakeBridge{})
			model.applyRuntimeEvent(runtime.RuntimeEvent{
				Type:     "permission.required",
				Approval: &tt.request,
			})

			view := newRenderer().renderApprovalDialog(newRenderSnapshot(model, 88))
			for _, want := range tt.wants {
				if !contains(view, want) {
					t.Fatalf("view missing %q: %q", want, view)
				}
			}
		})
	}
}

func TestRenderInputWithCursorPreservesUnicodeRunes(t *testing.T) {
	rendered := renderInputWithCursor("你好", 1, 40)

	if !contains(rendered, "你") || !contains(rendered, "好") {
		t.Fatalf("rendered input = %q, want both CJK runes preserved", rendered)
	}
}

func TestRenderInputWithCursorWrapsToVisualWidth(t *testing.T) {
	rendered := renderInputWithCursor("abcdefghij", 2, 5)
	lines := strings.Split(rendered, "\n")

	if len(lines) != 2 {
		t.Fatalf("line count = %d, want 2: %q", len(lines), rendered)
	}
	for _, line := range lines {
		if plain := stripANSITest(line); len([]rune(plain)) > 5 {
			t.Fatalf("wrapped line %q exceeds width 5", plain)
		}
	}
}

func TestRenderSnapshotTranscriptVisibleLinesShrinksForMultilinePrompt(t *testing.T) {
	model := NewModel(&fakeBridge{})
	model.setSize(80, 18)
	base := newRenderSnapshot(model, 80).transcriptVisibleLines()

	model.input = strings.Repeat("1", 80)
	model.cursorPos = len([]rune(model.input))
	model.setSize(24, 18)
	multiline := newRenderSnapshot(model, 24).transcriptVisibleLines()

	if multiline >= base {
		t.Fatalf("multiline visible lines = %d, want less than base %d", multiline, base)
	}
}

func TestRendererHeaderUsesReadableBranding(t *testing.T) {
	model := NewModel(&fakeBridge{}, ModelConfig{LLMLabel: "minimax / MiniMax-M2.7"})

	header := newRenderer().renderHeader(newRenderSnapshot(model, 88))
	if !contains(header, "MYCLAW") {
		t.Fatalf("header missing MYCLAW branding: %q", header)
	}
	if contains(header, "__  __") {
		t.Fatalf("header still contains large banner art: %q", header)
	}
	if !contains(header, "minimax / MiniMax-M2.7") {
		t.Fatalf("header missing model line: %q", header)
	}
}

func stripANSITest(s string) string {
	re := regexp.MustCompile(`\x1b\[[0-9;]*m`)
	return re.ReplaceAllString(s, "")
}
