package system

import (
	"context"
	"strings"
	"testing"
	"time"

	"myclaw/internal/sandbox"
	"myclaw/internal/session"
	"myclaw/internal/tools"
)

type shellTestExecutor struct {
	output string
	err    error
	wait   bool
}

func (e shellTestExecutor) Run(ctx context.Context, command string) (string, error) {
	if e.wait {
		<-ctx.Done()
		return "", ctx.Err()
	}
	if command == "long-output" {
		return strings.Repeat("x", maxShellOutputBytes+32), nil
	}
	return e.output, e.err
}

func TestRunToolAutoClassifierInputReturnsCommand(t *testing.T) {
	tool := NewRunTool(nil)

	classifier, ok := any(tool).(tools.AutoClassifyingTool)
	if !ok {
		t.Fatal("RunTool must expose a Claude-style auto classifier input projection")
	}

	got := classifier.ToAutoClassifierInput("  cat README.md  ")
	if got != "cat README.md" {
		t.Fatalf("expected trimmed command classifier input, got %#v", got)
	}
}

func TestShellReadOnlyClassificationDoesNotAllowCompoundDestructiveCommands(t *testing.T) {
	tool := NewBashTool(nil)

	if tool.IsReadOnly(`{"command":"cat README.md && rm README.md"}`) {
		t.Fatal("compound command with destructive segment must not be classified read-only")
	}
	if !tool.IsReadOnly(`{"command":"git status --short"}`) {
		t.Fatal("simple read-only command should remain read-only")
	}
}

func TestShellToolHonorsTimeoutAndReportsProgress(t *testing.T) {
	router := sandbox.NewRouter(shellTestExecutor{wait: true}, nil)
	tool := NewBashTool(router)
	var progress []tools.ToolProgress

	_, err := tool.InvokeWithContext(context.Background(), tools.ToolUseContext{
		Session:        session.Session{ID: "main-000001", IsMain: true},
		ToolName:       "Bash",
		ToolUseID:      "toolu-bash",
		Input:          `{"command":"sleep 10","timeout":1}`,
		ReportProgress: func(item tools.ToolProgress) { progress = append(progress, item) },
	})
	if err == nil {
		t.Fatal("expected timeout error")
	}
	if len(progress) < 2 || progress[0].Type != "shell.started" || progress[len(progress)-1].Type != "shell.finished" {
		t.Fatalf("progress = %#v, want shell start and finish events", progress)
	}
	if progress[len(progress)-1].Data["timed_out"] != true {
		t.Fatalf("finish progress = %#v, want timed_out marker", progress[len(progress)-1])
	}
}

func TestShellToolBackgroundReturnsImmediatelyAndReportsBackgroundProgress(t *testing.T) {
	router := sandbox.NewRouter(shellTestExecutor{wait: true}, nil)
	tool := NewBashTool(router)
	var progress []tools.ToolProgress

	start := time.Now()
	result, err := tool.InvokeWithContext(context.Background(), tools.ToolUseContext{
		Session:        session.Session{ID: "main-000001", IsMain: true},
		ToolName:       "Bash",
		ToolUseID:      "toolu-bg",
		Input:          `{"command":"sleep 100","run_in_background":true}`,
		ReportProgress: func(item tools.ToolProgress) { progress = append(progress, item) },
	})
	if err != nil {
		t.Fatalf("background invoke: %v", err)
	}
	if time.Since(start) > 100*time.Millisecond {
		t.Fatal("background command did not return immediately")
	}
	if !strings.Contains(result.Output, "running in the background") {
		t.Fatalf("output = %q, want background status", result.Output)
	}
	if len(progress) == 0 || progress[0].Type != "shell.background_started" {
		t.Fatalf("progress = %#v, want background started event", progress)
	}
}

func TestShellToolTruncatesLargeOutput(t *testing.T) {
	router := sandbox.NewRouter(shellTestExecutor{}, nil)
	tool := NewBashTool(router)

	result, err := tool.InvokeWithContext(context.Background(), tools.ToolUseContext{
		Session: session.Session{ID: "main-000001", IsMain: true},
		Input:   `{"command":"long-output"}`,
	})
	if err != nil {
		t.Fatalf("invoke shell: %v", err)
	}
	if len(result.Output) > maxShellOutputBytes+200 {
		t.Fatalf("output len = %d, want truncated", len(result.Output))
	}
	if !strings.Contains(result.Output, "truncated") {
		t.Fatalf("output = %q, want truncation marker", result.Output)
	}
}
