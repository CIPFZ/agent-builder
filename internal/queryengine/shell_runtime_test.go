package queryengine_test

import (
	"context"
	"strings"
	"testing"

	"myclaw/internal/llm"
	"myclaw/internal/permissions"
	"myclaw/internal/queryengine"
	"myclaw/internal/sandbox"
	"myclaw/internal/session"
	"myclaw/internal/tools"
	systemtools "myclaw/internal/tools/system"
	"myclaw/internal/workspace"
)

type shellFailureExecutor struct{}

func (shellFailureExecutor) Run(_ context.Context, _ string) (string, error) {
	return "", context.DeadlineExceeded
}

func (shellFailureExecutor) RunDetailed(_ context.Context, _ string) (sandbox.ExecutionResult, error) {
	return sandbox.ExecutionResult{
		Stdout:        "permission denied",
		Stderr:        "exit status 1",
		ExitCode:      1,
		ExecutionMode: "host",
	}, context.DeadlineExceeded
}

func TestQueryEngineEmitsStructuredShellResultWithWorktreeContext(t *testing.T) {
	sessions := session.NewManager(nil)
	sess := sessions.GetOrCreateMain("main")
	if err := sessions.UpdateMetadata(sess.ID, func(metadata *session.SessionMetadata) {
		metadata.AgentWorktreePath = "C:/repo/.worktrees/child"
	}); err != nil {
		t.Fatalf("update metadata: %v", err)
	}
	refreshed, ok := sessions.GetByID(sess.ID)
	if !ok {
		t.Fatal("session missing")
	}
	sess = refreshed
	msg, err := sessions.AppendMessage(sess.ID, "user", "run shell")
	if err != nil {
		t.Fatalf("append user message: %v", err)
	}

	client := &scriptedClient{
		scripts: [][]llm.StreamEvent{
			{
				{Type: "tool.call", ToolName: "Bash", ToolInput: `{"command":"pwd"}`, ToolInputObject: map[string]any{"command": "pwd"}, ToolUseID: "toolu-shell"},
				{Type: "message.end"},
			},
			{
				{Type: "text.delta", Delta: "done"},
				{Type: "message.end"},
			},
		},
	}
	engine := queryengine.New(queryengine.Config{
		Sessions:         sessions,
		Client:           client,
		WorkspaceLoader:  workspace.NewLoader("C:/repo"),
		PermissionPolicy: permissions.Policy{Mode: permissions.ModeDangerFullAccess},
	})
	sink := &captureSink{}

	if err := engine.SubmitMessage(context.Background(), sess, msg, sink); err != nil {
		t.Fatalf("submit message: %v", err)
	}

	var called, result *queryengine.Event
	for i := range sink.events {
		switch sink.events[i].Type {
		case "tool.called":
			called = &sink.events[i]
		case "tool.result":
			result = &sink.events[i]
		}
	}
	if called == nil || result == nil {
		t.Fatalf("events = %#v, want tool.called and tool.result", sink.events)
	}
	if called.ToolUseID != "toolu-shell" || called.ToolInputObject["command"] != "pwd" {
		t.Fatalf("called = %#v, want tool identity and structured input", called)
	}
	if result.StructuredContent == nil {
		t.Fatalf("result = %#v, want structured shell result", result)
	}
	if result.Meta["working_directory"] != "C:/repo/.worktrees/child" {
		t.Fatalf("meta = %#v, want worktree-aware working_directory", result.Meta)
	}
	if result.Meta["command"] != "pwd" {
		t.Fatalf("meta = %#v, want shell command", result.Meta)
	}
	if result.Message == nil || !strings.Contains(result.Message.Content, "Bash:") {
		t.Fatalf("message = %#v, want tool message content", result.Message)
	}
}

func TestQueryEnginePreservesStructuredShellFailureResult(t *testing.T) {
	sessions := session.NewManager(nil)
	sess := sessions.GetOrCreateMain("main")
	msg, err := sessions.AppendMessage(sess.ID, "user", "run failing shell")
	if err != nil {
		t.Fatalf("append user message: %v", err)
	}

	client := &scriptedClient{
		scripts: [][]llm.StreamEvent{
			{
				{Type: "tool.call", ToolName: "Bash", ToolInput: `{"command":"cat missing.txt"}`, ToolInputObject: map[string]any{"command": "cat missing.txt"}, ToolUseID: "toolu-shell-fail"},
				{Type: "message.end"},
			},
			{
				{Type: "text.delta", Delta: "handled"},
				{Type: "message.end"},
			},
		},
	}
	registry := tools.NewRegistry(systemtools.NewBashTool(sandbox.NewRouter(shellFailureExecutor{}, nil)))
	engine := queryengine.New(queryengine.Config{
		Sessions:         sessions,
		Client:           client,
		WorkspaceLoader:  workspace.NewLoader("C:/repo"),
		ToolRegistry:     registry,
		PermissionPolicy: permissions.Policy{Mode: permissions.ModeDangerFullAccess},
	})
	sink := &captureSink{}

	if err := engine.SubmitMessage(context.Background(), sess, msg, sink); err != nil {
		t.Fatalf("submit message: %v", err)
	}

	var result *queryengine.Event
	for i := range sink.events {
		if sink.events[i].Type == "tool.result" && sink.events[i].ToolName == "Bash" {
			result = &sink.events[i]
			break
		}
	}
	if result == nil {
		t.Fatalf("events = %#v, want shell tool.result", sink.events)
	}
	if !result.ToolError {
		t.Fatalf("result = %#v, want ToolError true", result)
	}
	if result.StructuredContent == nil {
		t.Fatalf("result = %#v, want structured failure content", result)
	}
	if result.Meta["exit_code"] != 1 || result.Meta["stderr"] != "exit status 1" {
		t.Fatalf("meta = %#v, want preserved failure meta", result.Meta)
	}
	if result.Message == nil || !strings.Contains(result.Message.Content, "permission denied") {
		t.Fatalf("message = %#v, want tool output preserved in failure message", result.Message)
	}
}
