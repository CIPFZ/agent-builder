package system

import (
	"context"
	"encoding/json"
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

type captureShellOptionsExecutor struct {
	options sandbox.RunOptions
	command string
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

func (e shellTestExecutor) RunDetailed(ctx context.Context, command string) (sandbox.ExecutionResult, error) {
	if e.wait {
		<-ctx.Done()
		return sandbox.ExecutionResult{TimedOut: true, ExecutionMode: "host"}, ctx.Err()
	}
	if command == "long-output" {
		return sandbox.ExecutionResult{
			Stdout:        strings.Repeat("x", maxShellOutputBytes+32),
			ExitCode:      0,
			ExecutionMode: "host",
		}, nil
	}
	exitCode := 0
	if e.err != nil {
		exitCode = 1
	}
	return sandbox.ExecutionResult{
		Stdout:        e.output,
		ExitCode:      exitCode,
		ExecutionMode: "host",
	}, e.err
}

func (e *captureShellOptionsExecutor) Run(_ context.Context, command string) (string, error) {
	e.command = command
	return "ok", nil
}

func (e *captureShellOptionsExecutor) RunDetailedWithOptions(_ context.Context, command string, options sandbox.RunOptions) (sandbox.ExecutionResult, error) {
	e.command = command
	e.options = options
	return sandbox.ExecutionResult{
		Stdout:        "ok",
		ExitCode:      0,
		ExecutionMode: "host",
	}, nil
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

func TestShellToolReturnsStructuredResultAndMeta(t *testing.T) {
	router := sandbox.NewRouter(shellTestExecutor{output: "ok"}, nil)
	tool := NewBashTool(router)

	result, err := tool.InvokeWithContext(context.Background(), tools.ToolUseContext{
		Session: session.Session{
			ID:     "main-000001",
			Key:    "C:/repo",
			IsMain: true,
			Metadata: session.SessionMetadata{
				AgentWorktreePath: "C:/repo/.worktree/child",
			},
		},
		WorkDir: `C:\repo\.worktree\child`,
		Input:   `{"command":"git status"}`,
	})
	if err != nil {
		t.Fatalf("invoke shell: %v", err)
	}
	if result.Meta["command"] != "git status" || result.Meta["working_directory"] != `C:\repo\.worktree\child` {
		t.Fatalf("meta = %#v, want command and working_directory", result.Meta)
	}
	if result.Meta["execution_mode"] != "host" || result.Meta["exit_code"] != 0 {
		t.Fatalf("meta = %#v, want execution mode and exit code", result.Meta)
	}
	encoded, _ := json.Marshal(result.StructuredContent)
	text := string(encoded)
	for _, want := range []string{`"command":"git status"`, `"working_directory":"C:\\repo\\.worktree\\child"`, `"execution_mode":"host"`, `"stdout":"ok"`, `"exit_code":0`} {
		if !strings.Contains(text, want) {
			t.Fatalf("structured content = %s, missing %s", text, want)
		}
	}
}

func TestShellToolUsesToolContextWorkDirWhenProvided(t *testing.T) {
	router := sandbox.NewRouter(shellTestExecutor{output: "ok"}, nil)
	tool := NewBashTool(router)

	result, err := tool.InvokeWithContext(context.Background(), tools.ToolUseContext{
		Session: session.Session{
			ID:     "child-000001",
			Key:    "session:key",
			IsMain: false,
		},
		WorkDir: "C:/repo/.worktree/child",
		Input:   `{"command":"pwd"}`,
	})
	if err != nil {
		t.Fatalf("invoke shell: %v", err)
	}
	if got := result.Meta["working_directory"]; got != "C:/repo/.worktree/child" {
		t.Fatalf("meta = %#v, want WorkDir from tool context", result.Meta)
	}
}

func TestShellToolReturnsStructuredFailureResult(t *testing.T) {
	router := sandbox.NewRouter(shellTestExecutor{output: "permission denied", err: context.DeadlineExceeded}, nil)
	tool := NewBashTool(router)

	result, err := tool.InvokeWithContext(context.Background(), tools.ToolUseContext{
		Session: session.Session{ID: "main-000001", Key: "C:/repo", IsMain: true},
		Input:   `{"command":"cat missing.txt"}`,
	})
	if err == nil {
		t.Fatal("expected shell failure")
	}
	if result.Meta["exit_code"] != 1 {
		t.Fatalf("meta = %#v, want non-zero exit code", result.Meta)
	}
	if result.Meta["stdout"] != "permission denied" {
		t.Fatalf("meta = %#v, want captured output", result.Meta)
	}
	encoded, _ := json.Marshal(result.StructuredContent)
	if !strings.Contains(string(encoded), `"exit_code":1`) {
		t.Fatalf("structured content = %s, want exit_code 1", string(encoded))
	}
}

func TestShellToolDispatchesRequestedShellFlavor(t *testing.T) {
	executor := &captureShellOptionsExecutor{}
	router := sandbox.NewRouter(executor, nil)
	bashTool := NewBashTool(router)
	powerShellTool := NewPowerShellTool(router)

	if _, err := bashTool.InvokeWithContext(context.Background(), tools.ToolUseContext{
		Session: session.Session{ID: "main-000001", Key: "C:/repo", IsMain: true},
		WorkDir: "C:/repo/worktree",
		Input:   `{"command":"echo bash"}`,
	}); err != nil {
		t.Fatalf("invoke bash: %v", err)
	}
	if executor.options.Shell != sandbox.ShellFlavorBash || executor.options.WorkDir != "C:/repo/worktree" {
		t.Fatalf("bash options = %#v, want bash flavor and explicit workdir", executor.options)
	}

	if _, err := powerShellTool.InvokeWithContext(context.Background(), tools.ToolUseContext{
		Session: session.Session{ID: "main-000001", Key: "C:/repo", IsMain: true},
		WorkDir: "C:/repo/worktree",
		Input:   `{"command":"Write-Output hi"}`,
	}); err != nil {
		t.Fatalf("invoke powershell: %v", err)
	}
	if executor.options.Shell != sandbox.ShellFlavorPowerShell {
		t.Fatalf("powershell options = %#v, want powershell flavor", executor.options)
	}
}

func TestShellInputSchemaDoesNotAdvertiseSandboxOverrideWithoutImplementation(t *testing.T) {
	properties, _ := shellInputSchema()["properties"].(map[string]any)
	if _, ok := properties["dangerouslyDisableSandbox"]; ok {
		t.Fatalf("schema properties = %#v, did not want unimplemented dangerouslyDisableSandbox field", properties)
	}
}
