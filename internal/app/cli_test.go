package app

import (
	"bytes"
	"context"
	"io"
	"os"
	"path/filepath"
	"testing"
	"time"

	"myclaw/internal/config"
	"myclaw/internal/model"
)

func TestRunCLIHelpIncludesTUI(t *testing.T) {
	var stdout bytes.Buffer
	if err := RunCLI(context.Background(), nil, &stdout, &bytes.Buffer{}); err != nil {
		t.Fatalf("RunCLI help: %v", err)
	}

	if got := stdout.String(); !bytes.Contains([]byte(got), []byte("tui")) {
		t.Fatalf("help output missing tui command: %q", got)
	}
}

func TestRunCLIDispatchesTUICommand(t *testing.T) {
	original := runTUI
	defer func() { runTUI = original }()

	called := false
	runTUI = func(_ context.Context, _ []string, _ io.Writer, _ io.Writer) error {
		called = true
		return nil
	}

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	if err := RunCLI(context.Background(), []string{"tui"}, &stdout, &stderr); err != nil {
		t.Fatalf("RunCLI tui: %v", err)
	}
	if !called {
		t.Fatal("expected tui command to dispatch to runTUI")
	}
}

func TestResolveTUIWorkspaceRootsFallsBackToCurrentDir(t *testing.T) {
	cwd := t.TempDir()
	roots, err := resolveTUIWorkspaceRoots(cwd, nil)
	if err != nil {
		t.Fatalf("resolve workspace roots: %v", err)
	}
	if len(roots) != 1 {
		t.Fatalf("workspace roots = %#v, want one root", roots)
	}
	if got, want := roots[0], filepath.Clean(cwd); got != want {
		t.Fatalf("workspace root = %q, want %q", got, want)
	}
}

func TestNewTUICompactorReturnsNilWhenVerificationModeDisabled(t *testing.T) {
	if got := newTUICompactor(config.Config{}); got != nil {
		t.Fatalf("newTUICompactor() = %#v, want nil when verification mode disabled", got)
	}
}

func TestNewTUICompactorUsesLowThresholdsForVerificationMode(t *testing.T) {
	compactor := newTUICompactor(config.Config{
		Compact: config.CompactConfig{VerificationMode: true},
	})
	if compactor == nil {
		t.Fatal("newTUICompactor() = nil, want service")
	}

	analysis := compactor.Analyze([]model.Message{
		{Role: "user", Content: "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"},
		{Role: "assistant", Content: "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"},
	})
	if !analysis.IsAboveWarningThreshold {
		t.Fatal("warning threshold not reached in verification mode")
	}
	if !analysis.IsAboveAutoCompactThreshold {
		t.Fatal("auto-compact threshold not reached in verification mode")
	}
}

func TestNewPersistentSessionManagerPersistsMessagesAcrossReload(t *testing.T) {
	root := filepath.Join(t.TempDir(), "logs", "sessions")

	manager, err := newPersistentSessionManager(root)
	if err != nil {
		t.Fatalf("newPersistentSessionManager: %v", err)
	}
	main := manager.GetOrCreateMain("main")
	if _, err := manager.AppendMessage(main.ID, "user", "hello"); err != nil {
		t.Fatalf("append message: %v", err)
	}

	reloaded, err := newPersistentSessionManager(root)
	if err != nil {
		t.Fatalf("reload persistent session manager: %v", err)
	}
	sessions := reloaded.ListSessions()
	if len(sessions) != 1 {
		t.Fatalf("sessions = %#v, want one reloaded session", sessions)
	}
	messages, ok := reloaded.Messages(main.ID)
	if !ok || len(messages) != 1 {
		t.Fatalf("messages = %#v, want one persisted message", messages)
	}
	if messages[0].Content != "hello" {
		t.Fatalf("message content = %q, want hello", messages[0].Content)
	}
	if _, err := os.Stat(filepath.Join(root, "sessions.json")); err != nil {
		t.Fatalf("sessions.json missing: %v", err)
	}
}

func TestNewPersistentSessionManagerKeepsIDsMonotonicAcrossReload(t *testing.T) {
	root := filepath.Join(t.TempDir(), "logs", "sessions")

	manager, err := newPersistentSessionManager(root)
	if err != nil {
		t.Fatalf("newPersistentSessionManager: %v", err)
	}
	main := manager.GetOrCreateMain("main")
	first, err := manager.AppendMessage(main.ID, "user", "hello")
	if err != nil {
		t.Fatalf("append first message: %v", err)
	}

	reloaded, err := newPersistentSessionManager(root)
	if err != nil {
		t.Fatalf("reload persistent session manager: %v", err)
	}
	second, err := reloaded.AppendMessage(main.ID, "assistant", "world")
	if err != nil {
		t.Fatalf("append second message: %v", err)
	}
	if second.ID == first.ID {
		t.Fatalf("second message id = %q, want unique id after reload", second.ID)
	}
	if second.ID != "msg-000002" {
		t.Fatalf("second message id = %q, want msg-000002", second.ID)
	}
}

func TestNewPersistentSessionManagerReloadsSessionMetadata(t *testing.T) {
	root := filepath.Join(t.TempDir(), "logs", "sessions")

	manager, err := newPersistentSessionManager(root)
	if err != nil {
		t.Fatalf("newPersistentSessionManager: %v", err)
	}
	main := manager.GetOrCreateMain("main")
	now := time.Unix(123, 0).UTC()
	if err := manager.UpdateMetadata(main.ID, func(metadata *model.SessionMetadata) {
		metadata.LastCompactBoundaryID = "compact-1"
		metadata.LastCompactionSummaryID = "summary-1"
		metadata.LastCompactionReason = "message-limit"
		metadata.LastCompactedAt = now
	}); err != nil {
		t.Fatalf("update metadata: %v", err)
	}

	reloaded, err := newPersistentSessionManager(root)
	if err != nil {
		t.Fatalf("reload persistent session manager: %v", err)
	}
	got, ok := reloaded.GetByID(main.ID)
	if !ok {
		t.Fatalf("session %q not found after reload", main.ID)
	}
	if got.Metadata.LastCompactBoundaryID != "compact-1" {
		t.Fatalf("metadata = %#v, want compact boundary", got.Metadata)
	}
	if got.Metadata.LastCompactedAt != now {
		t.Fatalf("metadata = %#v, want compaction time %v", got.Metadata, now)
	}
}
