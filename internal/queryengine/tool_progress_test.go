package queryengine_test

import (
	"context"
	"testing"

	"myclaw/internal/llm"
	"myclaw/internal/permissions"
	"myclaw/internal/queryengine"
	"myclaw/internal/session"
	"myclaw/internal/tools"
	"myclaw/internal/workspace"
)

// TestToolProgressEmitsSharedRuntimeEvent verifies tool progress
// is emitted as shared runtime event with RunID/Session/ToolName
func TestToolProgressEmitsSharedRuntimeEvent(t *testing.T) {
	sessions := session.NewManager(nil)
	sess := sessions.GetOrCreateMain("main")
	msg, err := sessions.AppendMessage(sess.ID, "user", "run progress tool")
	if err != nil {
		t.Fatalf("append user message: %v", err)
	}

	client := &scriptedClient{
		scripts: [][]llm.StreamEvent{
			{
				{Type: "tool.call", ToolName: "progress.tool", ToolInput: "execute", ToolUseID: "toolu-progress"},
				{Type: "message.end"},
			},
			{
				{Type: "text.delta", Delta: "done"},
				{Type: "message.end"},
			},
		},
	}

	var progressEvents []queryengine.Event
	registry := tools.NewRegistry(
		progressToolForQueryEngine{
			stubToolForQueryEngine: stubToolForQueryEngine{
				def:     tools.Definition{Name: "progress.tool", Description: "Tool that reports progress", Enabled: true},
				enabled: true,
			},
		},
	)

	engine := queryengine.New(queryengine.Config{
		Sessions:         sessions,
		Client:           client,
		WorkspaceLoader:  workspace.NewLoader(""),
		ToolRegistry:     registry,
		PermissionPolicy: permissions.Policy{Mode: permissions.ModeDangerFullAccess},
	})

	sink := &captureSink{}

	if err := engine.SubmitMessage(context.Background(), sess, msg, sink); err != nil {
		t.Fatalf("submit message: %v", err)
	}

	// Filter for tool.progress events
	for _, event := range sink.events {
		if event.Type == "tool.progress" {
			progressEvents = append(progressEvents, event)
		}
	}

	if len(progressEvents) == 0 {
		t.Fatalf("expected at least one tool.progress event, got none")
	}

	// Verify first progress event structure
	event := progressEvents[0]
	if event.Type != "tool.progress" {
		t.Errorf("event.Type = %q, want tool.progress", event.Type)
	}
	if event.RunID == "" {
		t.Errorf("event.RunID is empty, want non-empty RunID")
	}
	if event.Session.ID != sess.ID {
		t.Errorf("event.Session.ID = %q, want %q", event.Session.ID, sess.ID)
	}
	if event.ToolName != "progress.tool" {
		t.Errorf("event.ToolName = %q, want progress.tool", event.ToolName)
	}
	if event.ToolUseID != "toolu-progress" {
		t.Errorf("event.ToolUseID = %q, want toolu-progress", event.ToolUseID)
	}
	if event.Progress == nil {
		t.Fatalf("event.Progress is nil, want non-nil Progress")
	}
	if event.Progress.ToolUseID != "toolu-progress" {
		t.Errorf("event.Progress.ToolUseID = %q, want toolu-progress", event.Progress.ToolUseID)
	}
	if event.Progress.Type != "executing" {
		t.Errorf("event.Progress.Type = %q, want executing", event.Progress.Type)
	}
	if event.Progress.Message != "running progress.tool" {
		t.Errorf("event.Progress.Message = %q, want 'running progress.tool'", event.Progress.Message)
	}
	if event.Progress.Data == nil {
		t.Errorf("event.Progress.Data is nil, want non-nil Data")
	}
}

// progressToolForQueryEngine is a test tool that reports progress
type progressToolForQueryEngine struct {
	stubToolForQueryEngine
}

func (t progressToolForQueryEngine) InvokeWithContext(ctx context.Context, toolCtx tools.ToolUseContext) (tools.ToolResult, error) {
	// Report progress during execution
	toolCtx.ReportProgress(tools.ToolProgress{
		ToolUseID: toolCtx.ToolUseID,
		Type:      "executing",
		Message:   "running " + toolCtx.ToolName,
		Data: map[string]any{
			"step": "start",
		},
	})

	return tools.ToolResult{
		Output: "progress tool completed",
	}, nil
}
