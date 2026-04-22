package system

import (
	"bytes"
	"context"
	"fmt"
	"os/exec"
	"strings"
	"time"

	"myclaw/internal/tools"
)

type SystemSSHExecutor struct{}

func NewSystemSSHExecutor() *SystemSSHExecutor {
	return &SystemSSHExecutor{}
}

func (e *SystemSSHExecutor) Execute(ctx context.Context, input SSHInput, progressFn func(tools.ToolProgress)) (SSHResult, error) {
	result := SSHResult{
		Host:     input.Host,
		User:     input.User,
		Port:     input.Port,
		Command:  input.Command,
		ExitCode: -1,
	}

	if input.Port == 0 {
		input.Port = 22
	}
	result.Port = input.Port

	progressFn(tools.ToolProgress{
		Type:    "ssh.started",
		Message: fmt.Sprintf("SSH to %s", input.Host),
	})

	args := buildSSHArgs(input)
	command := buildRemoteCommand(input)

	progressFn(tools.ToolProgress{
		Type:    "ssh.connecting",
		Message: fmt.Sprintf("Connecting to %s", input.Host),
	})

	execCtx := ctx
	if input.Timeout > 0 {
		var cancel context.CancelFunc
		execCtx, cancel = context.WithTimeout(ctx, input.Timeout)
		defer cancel()
	}

	cmd := exec.CommandContext(execCtx, "ssh", append(args, command)...)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	progressFn(tools.ToolProgress{
		Type:    "ssh.running",
		Message: fmt.Sprintf("Running command on %s", input.Host),
	})

	start := time.Now()
	err := cmd.Run()
	duration := time.Since(start)

	result.Stdout = stdout.String()
	result.Stderr = stderr.String()
	result.DurationMs = duration.Milliseconds()
	result.TimedOut = execCtx.Err() == context.DeadlineExceeded

	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			result.ExitCode = exitErr.ExitCode()
		} else if result.TimedOut {
			result.ExitCode = -1
			progressFn(tools.ToolProgress{
				Type:    "ssh.failed",
				Message: fmt.Sprintf("SSH command timed out after %dms", result.DurationMs),
			})
			return result, fmt.Errorf("ssh command timed out")
		} else {
			progressFn(tools.ToolProgress{
				Type:    "ssh.failed",
				Message: fmt.Sprintf("SSH failed: %v", err),
			})
			return result, fmt.Errorf("ssh execution failed: %w", err)
		}
	} else {
		result.ExitCode = 0
	}

	progressFn(tools.ToolProgress{
		Type:    "ssh.finished",
		Message: fmt.Sprintf("SSH completed with exit code %d", result.ExitCode),
	})

	if result.ExitCode != 0 {
		return result, fmt.Errorf("ssh command exited with code %d", result.ExitCode)
	}

	return result, nil
}

func buildSSHArgs(input SSHInput) []string {
	args := []string{
		"-o", "BatchMode=yes",
		"-o", "StrictHostKeyChecking=accept-new",
	}

	if input.Port != 0 && input.Port != 22 {
		args = append(args, "-p", fmt.Sprintf("%d", input.Port))
	}

	if input.IdentityFile != "" {
		args = append(args, "-i", input.IdentityFile)
	}

	target := input.Host
	if input.User != "" {
		target = input.User + "@" + input.Host
	}
	args = append(args, target)

	return args
}

func buildRemoteCommand(input SSHInput) string {
	command := input.Command
	if input.Workdir != "" {
		escapedWorkdir := strings.ReplaceAll(input.Workdir, "'", "'\\''")
		command = fmt.Sprintf("cd '%s' && %s", escapedWorkdir, command)
	}
	return command
}
