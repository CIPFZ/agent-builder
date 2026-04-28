package queryengine_test

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"myclaw/internal/llm"
	"myclaw/internal/memory"
	"myclaw/internal/model"
	"myclaw/internal/queryengine"
	"myclaw/internal/session"
	storememory "myclaw/internal/store/memory"
	"myclaw/internal/tools"
	"myclaw/internal/workspace"
)

func TestReadToolStatePersistsAndDeterministicContextRebuildsAfterRestart(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, ".git"), 0o755); err != nil {
		t.Fatalf("mkdir .git: %v", err)
	}
	if err := os.WriteFile(filepath.Join(root, "CLAUDE.md"), []byte("workspace rules"), 0o644); err != nil {
		t.Fatalf("write CLAUDE.md: %v", err)
	}
	readPath := filepath.Join(root, "main.go")
	if err := os.WriteFile(readPath, []byte("package main\n"), 0o644); err != nil {
		t.Fatalf("write main.go: %v", err)
	}

	store := storememory.NewSessionStore()
	manager := session.NewManager(store)
	sess := manager.GetOrCreateMain("main")
	sess.Metadata.AgentWorktreePath = root
	if err := manager.UpdateMetadata(sess.ID, func(metadata *session.SessionMetadata) {
		metadata.AgentWorktreePath = root
	}); err != nil {
		t.Fatalf("set worktree: %v", err)
	}

	client := &scriptedClient{scripts: [][]llm.StreamEvent{
		{
			{Type: "tool.call", ToolName: "Read", ToolUseID: "toolu-read-1", ProviderMessageID: "provider-1", ToolInput: `{"file_path":"main.go"}`, ToolInputObject: map[string]any{"file_path": "main.go"}},
			{Type: "message.end"},
		},
		{{Type: "text.delta", Delta: "done"}, {Type: "message.end"}},
	}}
	engine := queryengine.New(queryengine.Config{
		Sessions:        manager,
		Client:          client,
		WorkspaceLoader: workspace.NewLoader(root),
		ToolRegistry:    tools.NewRegistry(tools.NewReadTool()),
		MemoryService:   memory.NewService(),
	})

	if err := engine.SubmitPrompt(context.Background(), sess, "read main", &captureSink{}); err != nil {
		t.Fatalf("submit prompt: %v", err)
	}

	stored, ok := manager.GetByID(sess.ID)
	if !ok {
		t.Fatal("session not stored")
	}
	if len(stored.Metadata.ReadFiles) != 1 {
		t.Fatalf("read-file metadata = %#v, want one entry", stored.Metadata.ReadFiles)
	}
	if stored.Metadata.ReadFiles[0].Path != readPath || stored.Metadata.ReadFiles[0].Hash == "" || stored.Metadata.ReadFiles[0].Size == 0 {
		t.Fatalf("read-file metadata not populated: %#v", stored.Metadata.ReadFiles[0])
	}

	restartedClient := &scriptedClient{scripts: [][]llm.StreamEvent{
		{{Type: "text.delta", Delta: "continued"}, {Type: "message.end"}},
	}}
	restartedMemory := memory.NewService()
	restartedMemory.RecoverSession(stored)
	restarted := queryengine.New(queryengine.Config{
		Sessions:        session.NewManager(store),
		Client:          restartedClient,
		WorkspaceLoader: workspace.NewLoader(root),
		ToolRegistry:    tools.NewRegistry(tools.NewReadTool()),
		MemoryService:   restartedMemory,
	})

	continued, ok := session.NewManager(store).GetByID(sess.ID)
	if !ok {
		t.Fatal("continued session missing")
	}
	if err := restarted.SubmitPrompt(context.Background(), continued, "continue", &captureSink{}); err != nil {
		t.Fatalf("submit after restart: %v", err)
	}
	requests := restartedClient.Requests()
	if len(requests) != 1 {
		t.Fatalf("restarted requests = %d, want 1", len(requests))
	}
	system := requests[0].Context.SystemPrompt + "\n" + strings.Join(requests[0].Context.SystemContextLines, "\n")
	if !strings.Contains(system, "read_file=") || !strings.Contains(system, "main.go") {
		t.Fatalf("rebuilt context missing read-file state:\n%s", system)
	}
	if len(requests[0].Context.WorkspaceLines) == 0 || !strings.Contains(requests[0].Context.WorkspaceLines[0], "workspace rules") {
		t.Fatalf("rebuilt context missing workspace rules: %#v", requests[0].Context.WorkspaceLines)
	}
}

func TestCorruptReadFileStateFallsBackWithoutPanic(t *testing.T) {
	root := t.TempDir()
	store := storememory.NewSessionStore()
	manager := session.NewManager(store)
	sess := manager.GetOrCreateMain("main")
	if err := manager.UpdateMetadata(sess.ID, func(metadata *session.SessionMetadata) {
		metadata.AgentWorktreePath = root
		metadata.ReadFiles = []model.ReadFileMetadata{{Path: filepath.Join(root, "missing.go"), Hash: "stale", Size: 10}}
	}); err != nil {
		t.Fatalf("set metadata: %v", err)
	}

	client := &scriptedClient{scripts: [][]llm.StreamEvent{
		{{Type: "text.delta", Delta: "ok"}, {Type: "message.end"}},
	}}
	engine := queryengine.New(queryengine.Config{
		Sessions:        manager,
		Client:          client,
		WorkspaceLoader: workspace.NewLoader(root),
		ToolRegistry:    tools.NewRegistry(tools.NewReadTool()),
	})
	current, _ := manager.GetByID(sess.ID)
	if err := engine.SubmitPrompt(context.Background(), current, "continue", &captureSink{}); err != nil {
		t.Fatalf("submit with corrupt read-file state should fall back: %v", err)
	}
	requests := client.Requests()
	if len(requests) != 1 {
		t.Fatalf("requests = %d, want 1", len(requests))
	}
	if !strings.Contains(strings.Join(requests[0].Context.SystemContextLines, "\n"), "read_file_stale=") {
		t.Fatalf("system context missing stale read-file fallback: %#v", requests[0].Context.SystemContextLines)
	}
}
