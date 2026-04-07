package app

import (
	"bytes"
	"context"
	"io"
	"path/filepath"
	"testing"

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
