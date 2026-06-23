package tools

import (
	"context"
	_ "embed"
	"fmt"
	"strings"

	"charm.land/fantasy"
	"github.com/CIPFZ/agent-builder/internal/shell"
)

const (
	JobOutputToolName = "job_output"
)

//go:embed job_output.md
var jobOutputDescription string

type JobOutputParams struct {
	ShellID string `json:"shell_id" description:"The ID of the background shell to retrieve output from"`
	Wait    bool   `json:"wait" description:"If true, block until the background shell completes before returning output"`
}

type JobOutputResponseMetadata struct {
	ShellID          string `json:"shell_id"`
	Command          string `json:"command"`
	Description      string `json:"description"`
	Done             bool   `json:"done"`
	Status           string `json:"status"`
	Stdout           string `json:"stdout,omitempty"`
	Stderr           string `json:"stderr,omitempty"`
	ExitCode         int    `json:"exit_code,omitempty"`
	WorkingDirectory string `json:"working_directory"`
	SandboxMode      string `json:"sandbox_mode,omitempty"`
	SandboxStatus    string `json:"sandbox_status,omitempty"`
	SandboxExecutor  string `json:"sandbox_executor,omitempty"`
	SandboxReason    string `json:"sandbox_reason,omitempty"`
	SandboxError     string `json:"sandbox_error,omitempty"`
}

func NewJobOutputTool() fantasy.AgentTool {
	return fantasy.NewAgentTool(
		JobOutputToolName,
		jobOutputDescription,
		func(ctx context.Context, params JobOutputParams, call fantasy.ToolCall) (fantasy.ToolResponse, error) {
			if params.ShellID == "" {
				return fantasy.NewTextErrorResponse("missing shell_id"), nil
			}

			bgManager := shell.GetBackgroundShellManager()
			bgShell, ok := bgManager.Get(params.ShellID)
			if !ok {
				return fantasy.NewTextErrorResponse(fmt.Sprintf("background shell not found: %s", params.ShellID)), nil
			}

			if params.Wait {
				bgShell.WaitContext(ctx)
			}

			stdout, stderr, done, err := bgShell.GetOutput()

			var outputParts []string
			if stdout != "" {
				outputParts = append(outputParts, stdout)
			}
			if stderr != "" {
				outputParts = append(outputParts, stderr)
			}

			status := "running"
			exitCode := 0
			if done {
				status = "completed"
				if err != nil {
					exitCode = shell.ExitCode(err)
					if exitCode != 0 {
						outputParts = append(outputParts, fmt.Sprintf("Exit code %d", exitCode))
					}
				}
			}

			output := strings.Join(outputParts, "\n")
			output = TruncateOutput(output)

			metadata := JobOutputResponseMetadata{
				ShellID:          params.ShellID,
				Command:          bgShell.Command,
				Description:      bgShell.Description,
				Done:             done,
				Status:           status,
				Stdout:           TruncateOutput(stdout),
				Stderr:           TruncateOutput(stderr),
				ExitCode:         exitCode,
				WorkingDirectory: bgShell.WorkingDir,
			}
			if meta, ok := SandboxMetadataFromContext(ctx); ok {
				metadata.SandboxMode = meta.Mode
				metadata.SandboxStatus = meta.Status
				metadata.SandboxExecutor = meta.Executor
				metadata.SandboxReason = meta.Reason
				metadata.SandboxError = meta.Error
			}

			if output == "" {
				output = BashNoOutput
			}

			result := fmt.Sprintf("Status: %s\n\n%s", status, output)
			return fantasy.WithResponseMetadata(fantasy.NewTextResponse(result), metadata), nil
		})
}
