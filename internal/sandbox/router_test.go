package sandbox

import (
	"context"
	"runtime"
	"testing"

	"myclaw/internal/session"
)

type captureExecutor struct {
	command string
}

func (e *captureExecutor) Run(_ context.Context, command string) (string, error) {
	e.command = command
	return "ok", nil
}

func TestRouterRoutesBySessionMainFlag(t *testing.T) {
	router := NewRouter(nil, nil)
	command := "printf hi"
	if runtime.GOOS == "windows" {
		command = "Write-Output hi"
	}

	mainOut, err := router.Run(context.Background(), session.Session{IsMain: true}, command)
	if err != nil {
		t.Fatalf("host run: %v", err)
	}
	if mainOut != "hi" {
		t.Fatalf("main output = %q, want %q", mainOut, "hi")
	}

	sandboxOut, err := router.Run(context.Background(), session.Session{IsMain: false}, command)
	if err != nil {
		t.Fatalf("sandbox run: %v", err)
	}
	if sandboxOut != "[sandbox] "+command {
		t.Fatalf("sandbox output = %q, want sandbox marker", sandboxOut)
	}
}

func TestRouterAppliesWorktreeWorkingDirectoryPrefix(t *testing.T) {
	host := &captureExecutor{}
	router := NewRouter(host, &captureExecutor{})
	sess := session.Session{
		IsMain: true,
		Metadata: session.SessionMetadata{
			AgentWorktreePath: `C:\repo\worktree`,
		},
	}

	if _, err := router.Run(context.Background(), sess, "git status"); err != nil {
		t.Fatalf("router run: %v", err)
	}

	want := `Set-Location -LiteralPath 'C:\repo\worktree'; git status`
	if runtime.GOOS != "windows" {
		want = "cd 'C:\\repo\\worktree' && git status"
	}
	if host.command != want {
		t.Fatalf("command = %q, want %q", host.command, want)
	}
}

func TestRouterRunWithOptionsUsesExplicitWorkingDirectory(t *testing.T) {
	host := &captureExecutor{}
	router := NewRouter(host, &captureExecutor{})

	if _, err := router.RunWithOptions(context.Background(), session.Session{IsMain: true}, "git status", RunOptions{
		WorkDir: `C:\repo\workspace`,
	}); err != nil {
		t.Fatalf("router run with options: %v", err)
	}

	want := `Set-Location -LiteralPath 'C:\repo\workspace'; git status`
	if runtime.GOOS != "windows" {
		want = "cd 'C:\\repo\\workspace' && git status"
	}
	if host.command != want {
		t.Fatalf("command = %q, want %q", host.command, want)
	}
}

func TestBuildHostCommandUsesRequestedShellFlavor(t *testing.T) {
	tests := []struct {
		name     string
		options  RunOptions
		wantExec string
		wantArg1 string
	}{
		{
			name:     "bash",
			options:  RunOptions{Shell: ShellFlavorBash, WorkDir: "C:/repo"},
			wantExec: "bash",
			wantArg1: "-lc",
		},
		{
			name:     "powershell",
			options:  RunOptions{Shell: ShellFlavorPowerShell, WorkDir: "C:/repo"},
			wantExec: "pwsh",
			wantArg1: "-NoProfile",
		},
	}
	if runtime.GOOS == "windows" {
		tests[1].wantExec = "powershell"
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			cmd := buildHostCommand(context.Background(), "echo hi", tc.options)
			if got := cmd.Args[0]; got != tc.wantExec {
				t.Fatalf("exec = %q, want %q", got, tc.wantExec)
			}
			if got := cmd.Args[1]; got != tc.wantArg1 {
				t.Fatalf("arg1 = %q, want %q", got, tc.wantArg1)
			}
			if cmd.Dir != tc.options.WorkDir {
				t.Fatalf("dir = %q, want %q", cmd.Dir, tc.options.WorkDir)
			}
		})
	}
}
