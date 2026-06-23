package tools

import (
	"context"
	_ "embed"
	"fmt"

	"charm.land/fantasy"
	"github.com/CIPFZ/agent-builder/internal/shell"
)

const (
	JobKillToolName = "job_kill"
)

//go:embed job_kill.md
var jobKillDescription string

type JobKillParams struct {
	ShellID string `json:"shell_id" description:"The ID of the background shell to terminate"`
}

type JobKillResponseMetadata struct {
	ShellID         string `json:"shell_id"`
	Command         string `json:"command"`
	Description     string `json:"description"`
	Status          string `json:"status"`
	Stdout          string `json:"stdout,omitempty"`
	Stderr          string `json:"stderr,omitempty"`
	ExitCode        int    `json:"exit_code,omitempty"`
	SandboxMode     string `json:"sandbox_mode,omitempty"`
	SandboxStatus   string `json:"sandbox_status,omitempty"`
	SandboxExecutor string `json:"sandbox_executor,omitempty"`
	SandboxReason   string `json:"sandbox_reason,omitempty"`
	SandboxError    string `json:"sandbox_error,omitempty"`
}

func NewJobKillTool() fantasy.AgentTool {
	return fantasy.NewAgentTool(
		JobKillToolName,
		jobKillDescription,
		func(ctx context.Context, params JobKillParams, call fantasy.ToolCall) (fantasy.ToolResponse, error) {
			if params.ShellID == "" {
				return fantasy.NewTextErrorResponse("missing shell_id"), nil
			}

			bgManager := shell.GetBackgroundShellManager()

			bgShell, ok := bgManager.Get(params.ShellID)
			if !ok {
				return fantasy.NewTextErrorResponse(fmt.Sprintf("background shell not found: %s", params.ShellID)), nil
			}

			metadata := JobKillResponseMetadata{
				ShellID:     params.ShellID,
				Command:     bgShell.Command,
				Description: bgShell.Description,
				Status:      "cancelled",
			}
			if meta, ok := SandboxMetadataFromContext(ctx); ok {
				metadata.SandboxMode = meta.Mode
				metadata.SandboxStatus = meta.Status
				metadata.SandboxExecutor = meta.Executor
				metadata.SandboxReason = meta.Reason
				metadata.SandboxError = meta.Error
			}

			err := bgManager.Kill(params.ShellID)
			if err != nil {
				return fantasy.NewTextErrorResponse(err.Error()), nil
			}
			stdout, stderr, _, execErr := bgShell.GetOutput()
			metadata.Stdout = TruncateOutput(stdout)
			metadata.Stderr = TruncateOutput(stderr)
			metadata.ExitCode = shell.ExitCode(execErr)

			result := fmt.Sprintf("Background shell %s terminated successfully", params.ShellID)
			return fantasy.WithResponseMetadata(fantasy.NewTextResponse(result), metadata), nil
		})
}
