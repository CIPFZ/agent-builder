package permission

import (
	"encoding/json"
	"regexp"
	"strings"

	"github.com/charmbracelet/crush/internal/tools/scheduler"
)

type PolicyMode string

const (
	PolicyModeAsk      PolicyMode = "ask"
	PolicyModeAutoRead PolicyMode = "auto_read"
	PolicyModePlan     PolicyMode = "plan"
	PolicyModeDenyAll  PolicyMode = "deny_all"
)

type Risk string

const (
	RiskRead        Risk = "read"
	RiskWrite       Risk = "write"
	RiskExecute     Risk = "execute"
	RiskNetwork     Risk = "network"
	RiskSecret      Risk = "secret"
	RiskDestructive Risk = "destructive"
)

type PolicyDecision string

const (
	PolicyAllow PolicyDecision = "allow"
	PolicyAsk   PolicyDecision = "ask"
	PolicyDeny  PolicyDecision = "deny"
)

type PermissionPolicy interface {
	Evaluate(scheduler.ToolCall) PolicyResult
}

type PolicyResult struct {
	Decision PolicyDecision
	Risk     Risk
	Reason   string
	Mode     PolicyMode
}

type StaticPolicy struct {
	Mode PolicyMode
}

func NewPermissionPolicy(mode PolicyMode) StaticPolicy {
	return StaticPolicy{Mode: NormalizePolicyMode(mode)}
}

func NormalizePolicyMode(mode PolicyMode) PolicyMode {
	switch mode {
	case PolicyModeAsk, PolicyModeAutoRead, PolicyModePlan, PolicyModeDenyAll:
		return mode
	default:
		return PolicyModeAsk
	}
}

func (p StaticPolicy) Evaluate(call scheduler.ToolCall) PolicyResult {
	risk := ClassifyToolCallRisk(call)
	mode := NormalizePolicyMode(p.Mode)
	switch mode {
	case PolicyModeDenyAll:
		return PolicyResult{Decision: PolicyDeny, Risk: risk, Reason: "Policy mode denies all tool calls.", Mode: mode}
	case PolicyModePlan:
		if risk == RiskRead {
			return PolicyResult{Decision: PolicyAllow, Risk: risk, Reason: "Plan mode allows read-only tool calls.", Mode: mode}
		}
		return PolicyResult{Decision: PolicyDeny, Risk: risk, Reason: "Plan mode blocks mutating, execute, network, destructive, or secret tool calls.", Mode: mode}
	case PolicyModeAutoRead:
		if risk == RiskRead {
			return PolicyResult{Decision: PolicyAllow, Risk: risk, Reason: "Auto-read mode allows read-only tool calls.", Mode: mode}
		}
		return PolicyResult{Decision: PolicyAsk, Risk: risk, Reason: "Auto-read mode asks before non-read tool calls.", Mode: mode}
	default:
		return PolicyResult{Decision: PolicyAsk, Risk: risk, Reason: "Ask mode requires approval for tool calls.", Mode: mode}
	}
}

func ClassifyRisk(toolName, inputSummary string) Risk {
	name := strings.ToLower(strings.TrimSpace(toolName))
	input := strings.ToLower(inputSummary)
	switch {
	case strings.Contains(input, "token") || strings.Contains(input, "api_key") || strings.Contains(input, "secret"):
		return RiskSecret
	case strings.Contains(input, "read mcp") || strings.Contains(input, "list mcp"):
		return RiskRead
	case name == "todos":
		return RiskWrite
	case name == "job_kill" || strings.Contains(name, "kill"):
		return RiskDestructive
	case strings.Contains(input, "network") || strings.Contains(input, "external"):
		return RiskNetwork
	case strings.HasPrefix(name, "mcp_"):
		return RiskNetwork
	case name == "bash" || name == "shell" || name == "job" || name == "job_output" || strings.Contains(name, "lsp_restart"):
		if ClassifyShellCommandRisk(inputSummary) == RiskDestructive {
			return RiskDestructive
		}
		if name == "job_output" {
			return RiskRead
		}
		return RiskExecute
	case strings.Contains(name, "write") || strings.Contains(name, "edit"):
		return RiskWrite
	case strings.Contains(name, "fetch") || strings.Contains(name, "download") || strings.Contains(name, "web"):
		return RiskNetwork
	default:
		return RiskRead
	}
}

func ClassifyToolCallRisk(call scheduler.ToolCall) Risk {
	source := strings.ToLower(strings.TrimSpace(string(call.Source)))
	switch scheduler.ToolSource(source) {
	case scheduler.ToolSourceShell:
		if call.Name == "job_output" {
			return RiskRead
		}
		if call.Name == "job_kill" {
			return RiskDestructive
		}
		if ClassifyShellCommandRisk(call.InputSummary) == RiskDestructive {
			return RiskDestructive
		}
		return RiskExecute
	case scheduler.ToolSourceMCP:
		return RiskNetwork
	}
	return ClassifyRisk(call.Name, call.InputSummary)
}

func isDestructiveInput(inputSummary string) bool {
	return ClassifyShellCommandRisk(inputSummary) == RiskDestructive
}

var destructiveShellPatterns = []*regexp.Regexp{
	regexp.MustCompile(`(?i)(^|[;&|()\s])rm(\.exe)?(\s|$)`),
	regexp.MustCompile(`(?i)(^|[;&|()\s])del(\.exe)?(\s|$)`),
	regexp.MustCompile(`(?i)(^|[;&|()\s])remove-item(\s|$)`),
	regexp.MustCompile(`(?i)(^|[;&|()\s])erase(\.exe)?(\s|$)`),
	regexp.MustCompile(`(?i)\bgit\s+reset\s+--hard\b`),
	regexp.MustCompile(`(?i)(^|[;&|()\s])(kill|killall|pkill|stop-process|taskkill)(\s|$)`),
	regexp.MustCompile(`(?i)(^|[;&|()\s])(chmod|chown)(\s|$)`),
	regexp.MustCompile(`(?i)(^|[;&|()\s])(rmdir|rd)(\s|$).*(\s|/|-)s\b`),
	regexp.MustCompile(`(?i)\b(remove|delete)\b.*\b(recurse|recursive|-r|-rf|/s)\b`),
	regexp.MustCompile(`(?i)(^|[^>])>[|]?\s*[^>\s]`),
}

// ClassifyShellCommandRisk is a conservative baseline classifier. It does not
// parse Bash, cmd, or PowerShell; it only identifies common destructive shapes
// before defaulting shell execution to execute risk.
func ClassifyShellCommandRisk(inputSummary string) Risk {
	command := extractShellCommand(inputSummary)
	lower := strings.ToLower(command)
	if strings.TrimSpace(lower) == "" {
		return RiskExecute
	}
	for _, pattern := range destructiveShellPatterns {
		if pattern.MatchString(lower) {
			return RiskDestructive
		}
	}
	return RiskExecute
}

func extractShellCommand(inputSummary string) string {
	var payload map[string]any
	if err := json.Unmarshal([]byte(inputSummary), &payload); err == nil {
		for _, key := range []string{"command", "Command"} {
			if value, ok := payload[key].(string); ok {
				return value
			}
		}
	}
	return inputSummary
}

func policyToolCall(opts CreatePermissionRequest, risk Risk) scheduler.ToolCall {
	return scheduler.ToolCall{
		ID:           opts.ToolCallID,
		SessionID:    opts.SessionID,
		TurnID:       opts.TurnID,
		Name:         opts.ToolName,
		Source:       scheduler.ToolSource(opts.Source),
		Status:       scheduler.ToolCallPending,
		InputSummary: firstNonEmpty(opts.Description, opts.Action, string(risk)),
	}
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if value != "" {
			return value
		}
	}
	return ""
}
