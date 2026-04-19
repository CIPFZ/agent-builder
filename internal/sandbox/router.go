package sandbox

import (
	"context"
	"fmt"
	"os/exec"
	"runtime"
	"strings"

	"myclaw/internal/session"
)

type Executor interface {
	Run(context.Context, string) (string, error)
}

type Router struct {
	host    Executor
	sandbox Executor
}

func NewRouter(host, sandbox Executor) *Router {
	if host == nil {
		host = HostExecutor{}
	}
	if sandbox == nil {
		sandbox = MockSandboxExecutor{}
	}
	return &Router{
		host:    host,
		sandbox: sandbox,
	}
}

func (r *Router) Run(ctx context.Context, sess session.Session, command string) (string, error) {
	command = withSessionWorktreePrefix(sess, command)
	if sess.IsMain {
		return r.host.Run(ctx, command)
	}
	return r.sandbox.Run(ctx, command)
}

func withSessionWorktreePrefix(sess session.Session, command string) string {
	command = strings.TrimSpace(command)
	worktreePath := strings.TrimSpace(sess.Metadata.AgentWorktreePath)
	if worktreePath == "" || command == "" {
		return command
	}
	quoted := "'" + strings.ReplaceAll(worktreePath, "'", "''") + "'"
	if runtime.GOOS == "windows" {
		return "Set-Location -LiteralPath " + quoted + "; " + command
	}
	return "cd " + quoted + " && " + command
}

type HostExecutor struct{}

func (HostExecutor) Run(ctx context.Context, command string) (string, error) {
	cmd := buildHostCommand(ctx, command)
	out, err := cmd.CombinedOutput()
	output := strings.TrimSpace(string(out))
	if err != nil {
		if output == "" {
			return "", err
		}
		return output, err
	}
	return output, nil
}

func buildHostCommand(ctx context.Context, command string) *exec.Cmd {
	if runtime.GOOS == "windows" {
		return exec.CommandContext(ctx, "powershell", "-NoProfile", "-Command", command)
	}
	return exec.CommandContext(ctx, "bash", "-lc", command)
}

type MockSandboxExecutor struct{}

func (MockSandboxExecutor) Run(_ context.Context, command string) (string, error) {
	return fmt.Sprintf("[sandbox] %s", command), nil
}
