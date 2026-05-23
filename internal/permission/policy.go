package permission

import (
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
	risk := ClassifyRisk(call.Name, call.InputSummary)
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
	case strings.Contains(input, "network") || strings.Contains(input, "external"):
		return RiskNetwork
	case name == "bash" || name == "shell" || name == "job" || strings.Contains(name, "lsp_restart"):
		if strings.Contains(input, " rm ") || strings.Contains(input, " del ") || strings.Contains(input, "remove-item") || strings.Contains(input, "reset --hard") {
			return RiskDestructive
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

func policyToolCall(opts CreatePermissionRequest, risk Risk) scheduler.ToolCall {
	return scheduler.ToolCall{
		ID:           opts.ToolCallID,
		SessionID:    opts.SessionID,
		TurnID:       opts.TurnID,
		Name:         opts.ToolName,
		Source:       scheduler.ToolSourceUnknown,
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
