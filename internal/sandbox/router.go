package sandbox

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os/exec"
	"runtime"
	"strings"
	"unicode/utf8"

	"golang.org/x/text/encoding/simplifiedchinese"
	"myclaw/internal/session"
)

type Executor interface {
	Run(context.Context, string) (string, error)
}

type ShellFlavor string

const (
	ShellFlavorDefault    ShellFlavor = "default"
	ShellFlavorBash       ShellFlavor = "bash"
	ShellFlavorPowerShell ShellFlavor = "powershell"
)

type RunOptions struct {
	WorkDir string
	Shell   ShellFlavor
}

type DetailedExecutor interface {
	RunDetailed(context.Context, string) (ExecutionResult, error)
}

type DetailedExecutorWithOptions interface {
	RunDetailedWithOptions(context.Context, string, RunOptions) (ExecutionResult, error)
}

type ExecutionResult struct {
	Stdout        string
	Stderr        string
	ExitCode      int
	TimedOut      bool
	ExecutionMode string
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
	result, err := r.RunDetailed(ctx, sess, command)
	return result.Output(), err
}

func (r *Router) RunDetailed(ctx context.Context, sess session.Session, command string) (ExecutionResult, error) {
	return r.RunDetailedWithOptions(ctx, sess, command, RunOptions{})
}

func (r *Router) RunWithOptions(ctx context.Context, sess session.Session, command string, options RunOptions) (string, error) {
	result, err := r.RunDetailedWithOptions(ctx, sess, command, options)
	return result.Output(), err
}

func (r *Router) RunDetailedWithOptions(ctx context.Context, sess session.Session, command string, options RunOptions) (ExecutionResult, error) {
	options = normalizedRunOptions(sess, options)
	mode := "sandbox"
	executor := r.sandbox
	if sess.IsMain {
		mode = "host"
		executor = r.host
	}
	if detailed, ok := executor.(DetailedExecutorWithOptions); ok {
		result, err := detailed.RunDetailedWithOptions(ctx, command, options)
		if strings.TrimSpace(result.ExecutionMode) == "" {
			result.ExecutionMode = mode
		}
		return result, err
	}
	command = withWorkingDirectoryPrefix(options.WorkDir, command)
	if detailed, ok := executor.(DetailedExecutor); ok {
		result, err := detailed.RunDetailed(ctx, command)
		if strings.TrimSpace(result.ExecutionMode) == "" {
			result.ExecutionMode = mode
		}
		return result, err
	}
	output, err := executor.Run(ctx, command)
	result := ExecutionResult{
		Stdout:        strings.TrimSpace(output),
		ExitCode:      exitCodeFromError(err),
		TimedOut:      ctx.Err() == context.DeadlineExceeded,
		ExecutionMode: mode,
	}
	return result, err
}

func normalizedRunOptions(sess session.Session, options RunOptions) RunOptions {
	options.WorkDir = strings.TrimSpace(options.WorkDir)
	if options.WorkDir == "" {
		options.WorkDir = strings.TrimSpace(sess.Metadata.AgentWorktreePath)
	}
	if options.Shell == "" {
		options.Shell = ShellFlavorDefault
	}
	return options
}

func withWorkingDirectoryPrefix(workDir, command string) string {
	command = strings.TrimSpace(command)
	workDir = strings.TrimSpace(workDir)
	if workDir == "" || command == "" {
		return command
	}
	quoted := "'" + strings.ReplaceAll(workDir, "'", "''") + "'"
	if runtime.GOOS == "windows" {
		return "Set-Location -LiteralPath " + quoted + "; " + command
	}
	return "cd " + quoted + " && " + command
}

type HostExecutor struct{}

func (HostExecutor) Run(ctx context.Context, command string) (string, error) {
	result, err := HostExecutor{}.RunDetailed(ctx, command)
	return result.Output(), err
}

func (HostExecutor) RunDetailed(ctx context.Context, command string) (ExecutionResult, error) {
	return HostExecutor{}.RunDetailedWithOptions(ctx, command, RunOptions{})
}

func (HostExecutor) RunDetailedWithOptions(ctx context.Context, command string, options RunOptions) (ExecutionResult, error) {
	cmd := buildHostCommand(ctx, command, options)
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	err := cmd.Run()
	result := ExecutionResult{
		Stdout:        strings.TrimSpace(decodeCommandOutputBytes(stdout.Bytes())),
		Stderr:        strings.TrimSpace(decodeCommandOutputBytes(stderr.Bytes())),
		ExitCode:      exitCodeFromError(err),
		TimedOut:      ctx.Err() == context.DeadlineExceeded,
		ExecutionMode: "host",
	}
	return result, err
}

func buildHostCommand(ctx context.Context, command string, options RunOptions) *exec.Cmd {
	executable := "bash"
	args := []string{"-lc", command}
	switch options.Shell {
	case ShellFlavorPowerShell:
		executable = "pwsh"
		if runtime.GOOS == "windows" {
			executable = "powershell"
		}
		args = []string{"-NoProfile", "-Command", command}
	case ShellFlavorBash:
		executable = "bash"
		args = []string{"-lc", command}
	default:
		if runtime.GOOS == "windows" {
			executable = "powershell"
			args = []string{"-NoProfile", "-Command", command}
		}
	}
	cmd := exec.CommandContext(ctx, executable, args...)
	if strings.TrimSpace(options.WorkDir) != "" {
		cmd.Dir = options.WorkDir
	}
	return cmd
}

func decodeCommandOutputBytes(data []byte) string {
	if utf8.Valid(data) {
		return string(data)
	}
	decoded, err := simplifiedchinese.GB18030.NewDecoder().Bytes(data)
	if err != nil {
		return string(data)
	}
	return string(decoded)
}

type MockSandboxExecutor struct{}

func (MockSandboxExecutor) Run(_ context.Context, command string) (string, error) {
	return fmt.Sprintf("[sandbox] %s", command), nil
}

func (MockSandboxExecutor) RunDetailed(_ context.Context, command string) (ExecutionResult, error) {
	return ExecutionResult{
		Stdout:        fmt.Sprintf("[sandbox] %s", command),
		ExecutionMode: "sandbox",
	}, nil
}

func (r ExecutionResult) Output() string {
	stdout := strings.TrimSpace(r.Stdout)
	stderr := strings.TrimSpace(r.Stderr)
	switch {
	case stdout == "":
		return stderr
	case stderr == "":
		return stdout
	default:
		return stdout + "\n" + stderr
	}
}

func exitCodeFromError(err error) int {
	if err == nil {
		return 0
	}
	var exitErr *exec.ExitError
	if errors.As(err, &exitErr) {
		return exitErr.ExitCode()
	}
	return 1
}
