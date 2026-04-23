package system

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"myclaw/internal/sandbox"
	"myclaw/internal/session"
	"myclaw/internal/tools"
)

var _ tools.Tool = (*SSHTool)(nil)
var _ tools.ContextualTool = (*SSHTool)(nil)

// SSHTool implements remote command execution via SSH
type SSHTool struct {
	router   *sandbox.Router
	executor SSHExecutor
}

// SSHInput represents the structured input for SSH tool
type SSHInput struct {
	Host         string        `json:"host"`
	Command      string        `json:"command"`
	User         string        `json:"user,omitempty"`
	Port         int           `json:"port,omitempty"`
	Timeout      time.Duration `json:"timeout,omitempty"`
	Workdir      string        `json:"workdir,omitempty"`
	IdentityFile string        `json:"identity_file,omitempty"`
}

// SSHResult represents the structured result from SSH execution
type SSHResult struct {
	Host       string `json:"host"`
	User       string `json:"user"`
	Port       int    `json:"port"`
	Command    string `json:"command"`
	Stdout     string `json:"stdout"`
	Stderr     string `json:"stderr"`
	ExitCode   int    `json:"exit_code"`
	TimedOut   bool   `json:"timed_out"`
	DurationMs int64  `json:"duration_ms"`
}

// SSHExecutor abstracts SSH execution backend
type SSHExecutor interface {
	Execute(ctx context.Context, input SSHInput, progressFn func(tools.ToolProgress)) (SSHResult, error)
}

// NewSSHTool creates a new SSH tool with system ssh backend
func NewSSHTool(router *sandbox.Router) *SSHTool {
	if router == nil {
		router = sandbox.NewRouter(nil, nil)
	}
	return &SSHTool{
		router:   router,
		executor: NewSystemSSHExecutor(),
	}
}

func (t *SSHTool) Definition() tools.Definition {
	return tools.Definition{
		Name:        "SSH",
		Description: "Execute a command on a remote host over SSH.",
		InputSchema: sshInputSchema(),
		Enabled:     true,
		ReadOnly:    false,
		Destructive: true,
	}
}

func (t *SSHTool) Invoke(ctx context.Context, sess session.Session, input string) (string, error) {
	result, err := t.InvokeWithContext(ctx, tools.ToolUseContext{
		Session: sess,
		Input:   input,
	})
	if err != nil {
		return "", err
	}
	return result.Output, nil
}

func (t *SSHTool) InvokeWithContext(ctx context.Context, toolCtx tools.ToolUseContext) (tools.ToolResult, error) {
	toolCtx = toolCtx.Normalized()
	input, err := parseSSHInput(toolCtx.Input)
	if err != nil {
		return tools.ToolResult{}, err
	}

	result, err := t.executor.Execute(ctx, input, func(progress tools.ToolProgress) {
		if progress.ToolUseID == "" {
			progress.ToolUseID = toolCtx.ToolUseID
		}
		if progress.Data == nil {
			progress.Data = map[string]any{}
		}
		if _, ok := progress.Data["tool"]; !ok {
			progress.Data["tool"] = "SSH"
		}
		if _, ok := progress.Data["host"]; !ok {
			progress.Data["host"] = input.Host
		}
		if _, ok := progress.Data["command"]; !ok {
			progress.Data["command"] = input.Command
		}
		toolCtx.ReportProgress(progress)
	})

	text := renderSSHOutput(result)
	toolResult := tools.ToolResult{
		Output:            text,
		StructuredContent: result,
		Meta: map[string]any{
			"tool":        "SSH",
			"host":        result.Host,
			"user":        result.User,
			"port":        result.Port,
			"command":     result.Command,
			"exit_code":   result.ExitCode,
			"timed_out":   result.TimedOut,
			"duration_ms": result.DurationMs,
		},
	}
	if err != nil {
		return toolResult, err
	}
	return toolResult, nil
}

func (t *SSHTool) IsEnabled() bool {
	return true
}

func (t *SSHTool) IsReadOnly(input string) bool {
	return false
}

func (t *SSHTool) IsDestructive(input string) bool {
	return true
}

func (t *SSHTool) ShouldDefer() bool {
	return false
}

func (t *SSHTool) AlwaysLoad() bool {
	return false
}

func (t *SSHTool) PromptDescription() string {
	return "Execute a command on a remote host over SSH."
}

func (t *SSHTool) SearchHint() string {
	return "ssh remote command"
}

func sshInputSchema() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"host":          map[string]any{"type": "string", "description": "Remote host"},
			"command":       map[string]any{"type": "string", "description": "Command to execute remotely"},
			"user":          map[string]any{"type": "string", "description": "SSH username"},
			"port":          map[string]any{"type": "number", "description": "SSH port"},
			"timeout":       map[string]any{"type": "number", "description": "Optional timeout in milliseconds"},
			"workdir":       map[string]any{"type": "string", "description": "Remote working directory"},
			"identity_file": map[string]any{"type": "string", "description": "Path to SSH private key"},
		},
		"required": []string{"host", "command"},
	}
}

func parseSSHInput(raw string) (SSHInput, error) {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return SSHInput{}, fmt.Errorf("SSH requires structured JSON input")
	}
	if !strings.HasPrefix(trimmed, "{") {
		return SSHInput{}, fmt.Errorf("SSH requires structured JSON input")
	}

	var payload map[string]any
	if err := json.Unmarshal([]byte(trimmed), &payload); err != nil {
		return SSHInput{}, fmt.Errorf("invalid SSH input: %w", err)
	}

	input := SSHInput{}
	if host, ok := payload["host"].(string); ok {
		input.Host = strings.TrimSpace(host)
	}
	if command, ok := payload["command"].(string); ok {
		input.Command = strings.TrimSpace(command)
	}
	if user, ok := payload["user"].(string); ok {
		input.User = strings.TrimSpace(user)
	}
	if workdir, ok := payload["workdir"].(string); ok {
		input.Workdir = strings.TrimSpace(workdir)
	}
	if identityFile, ok := payload["identity_file"].(string); ok {
		input.IdentityFile = strings.TrimSpace(identityFile)
	}

	if port, ok := intFromAny(payload["port"]); ok {
		input.Port = port
	}
	if timeout := durationMillis(payload["timeout"]); timeout > 0 {
		input.Timeout = timeout
	}

	if input.Host == "" {
		return SSHInput{}, fmt.Errorf("SSH requires host")
	}
	if input.Command == "" {
		return SSHInput{}, fmt.Errorf("SSH requires command")
	}

	return input, nil
}

func intFromAny(value any) (int, bool) {
	switch v := value.(type) {
	case float64:
		return int(v), true
	case int:
		return v, true
	case int64:
		return int(v), true
	case json.Number:
		parsed, err := v.Int64()
		if err != nil {
			return 0, false
		}
		return int(parsed), true
	default:
		return 0, false
	}
}

func renderSSHOutput(result SSHResult) string {
	output := fmt.Sprintf("host: %s\ncommand: %s\nexit_code: %d\nduration_ms: %d", result.Host, result.Command, result.ExitCode, result.DurationMs)
	if result.TimedOut {
		output += "\ntimed_out: true"
	}
	if result.Stdout != "" {
		output += "\n\nstdout:\n" + result.Stdout
	}
	if result.Stderr != "" {
		output += "\n\nstderr:\n" + result.Stderr
	}
	return output
}
