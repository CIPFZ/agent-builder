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
