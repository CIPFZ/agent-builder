package runtime

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"regexp"
	"strings"
	"time"

	"github.com/charmbracelet/crush/internal/backend"
	"github.com/charmbracelet/crush/internal/permission"
	"github.com/charmbracelet/crush/internal/runtimeapi"
)

var runtimePolicyModes = []string{
	string(permission.PolicyModeAsk),
	string(permission.PolicyModeAutoRead),
	string(permission.PolicyModePlan),
	string(permission.PolicyModeDenyAll),
}

type runtimePolicyFile struct {
	Mode      string              `json:"mode"`
	Profile   string              `json:"profile,omitempty"`
	Rules     []RuntimePolicyRule `json:"rules,omitempty"`
	UpdatedAt int64               `json:"updatedAt,omitempty"`
}

func defaultRuntimePolicy() RuntimePolicy {
	return RuntimePolicy{
		Mode:        string(permission.PolicyModeAsk),
		Modes:       append([]string(nil), runtimePolicyModes...),
		Profile:     string(permission.PolicyProfileDefault),
		Description: runtimePolicyDescription(permission.PolicyModeAsk),
	}
}

func normalizeRuntimePolicyMode(mode string) (permission.PolicyMode, error) {
	normalized := permission.NormalizePolicyMode(permission.PolicyMode(strings.TrimSpace(mode)))
	if strings.TrimSpace(mode) != "" && normalized != permission.PolicyMode(strings.TrimSpace(mode)) {
		return "", fmt.Errorf("invalid policy mode: %s", mode)
	}
	return normalized, nil
}

func runtimePolicyDescription(mode permission.PolicyMode) string {
	switch permission.NormalizePolicyMode(mode) {
	case permission.PolicyModeAutoRead:
		return "Read-only tool calls are allowed; other tool calls request approval."
	case permission.PolicyModePlan:
		return "Read-only tool calls are allowed; mutating, execute, network, destructive, and secret tool calls are blocked."
	case permission.PolicyModeDenyAll:
		return "All tool calls are blocked."
	default:
		return "Tool calls request approval unless explicitly pre-approved by runtime policy."
	}
}

func runtimePolicyFromMode(mode permission.PolicyMode, updatedAt int64) RuntimePolicy {
	return runtimePolicyFromParts(mode, "default", nil, updatedAt)
}

func runtimePolicyFromParts(mode permission.PolicyMode, profile string, rules []RuntimePolicyRule, updatedAt int64) RuntimePolicy {
	mode = permission.NormalizePolicyMode(mode)
	profile = permission.NormalizePolicyProfile(profile)
	normalizedRules, diagnostics := normalizeRuntimePolicyRules(rules)
	return RuntimePolicy{
		Mode:        string(mode),
		Modes:       append([]string(nil), runtimePolicyModes...),
		Profile:     profile,
		Rules:       normalizedRules,
		Diagnostics: diagnostics,
		Description: runtimePolicyDescription(mode),
		UpdatedAt:   updatedAt,
	}
}

func loadRuntimePolicy(layout desktopLayout) (RuntimePolicy, error) {
	data, err := os.ReadFile(layout.PolicyConfigPath)
	if errors.Is(err, os.ErrNotExist) {
		return defaultRuntimePolicy(), nil
	}
	if err != nil {
		return RuntimePolicy{}, fmt.Errorf("failed to read runtime policy config: %w", err)
	}
	var file runtimePolicyFile
	if err := json.Unmarshal(data, &file); err != nil {
		return RuntimePolicy{}, fmt.Errorf("failed to parse runtime policy config: %w", err)
	}
	mode, err := normalizeRuntimePolicyMode(file.Mode)
	if err != nil {
		return RuntimePolicy{}, err
	}
	return runtimePolicyFromParts(mode, file.Profile, file.Rules, file.UpdatedAt), nil
}

func saveRuntimePolicy(layout desktopLayout, policy RuntimePolicy) error {
	if err := ensureDesktopLayout(layout); err != nil {
		return err
	}
	data, err := json.MarshalIndent(runtimePolicyFile{
		Mode:      policy.Mode,
		Profile:   policy.Profile,
		Rules:     policy.Rules,
		UpdatedAt: policy.UpdatedAt,
	}, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to encode runtime policy config: %w", err)
	}
	data = append(data, '\n')
	if err := os.WriteFile(layout.PolicyConfigPath, data, 0o600); err != nil {
		return fmt.Errorf("failed to write runtime policy config: %w", err)
	}
	return nil
}

func runtimePermissionPolicy(policy RuntimePolicy) permission.PermissionPolicy {
	return permission.NewScopedPermissionPolicy(permission.PolicyMode(policy.Mode), policy.Profile, permissionRulesFromRuntime(policy.Rules))
}

func permissionRulesFromRuntime(rules []RuntimePolicyRule) []permission.PolicyRule {
	out := make([]permission.PolicyRule, 0, len(rules))
	for _, rule := range rules {
		out = append(out, permission.PolicyRule{
			ID:            rule.ID,
			Decision:      permission.PolicyDecision(rule.Decision),
			Source:        rule.Source,
			Reason:        rule.Reason,
			Tool:          rule.Tool,
			CapabilityID:  rule.CapabilityID,
			BuiltinTool:   rule.BuiltinTool,
			MCPServer:     rule.MCPServer,
			MCPTool:       rule.MCPTool,
			MCPResource:   rule.MCPResource,
			MCPPrompt:     rule.MCPPrompt,
			Skill:         rule.Skill,
			Subagent:      rule.Subagent,
			TaskScope:     rule.TaskScope,
			CWDPrefix:     rule.CWDPrefix,
			PathPrefix:    rule.PathPrefix,
			ShellPrefix:   rule.ShellPrefix,
			ShellRegex:    rule.ShellRegex,
			PolicyMode:    permission.PolicyMode(rule.PolicyMode),
			PolicyProfile: rule.PolicyProfile,
			ScopeKind:     rule.ScopeKind,
			ScopeValue:    rule.ScopeValue,
			Precedence:    rule.Precedence,
		})
	}
	return out
}

func normalizeRuntimePolicyRules(rules []RuntimePolicyRule) ([]RuntimePolicyRule, []RuntimePolicyDiagnostic) {
	permissionRules := permission.NormalizePolicyRules(permissionRulesFromRuntime(rules))
	validByID := map[string]permission.PolicyRule{}
	for _, rule := range permissionRules {
		validByID[rule.ID] = rule
	}
	out := make([]RuntimePolicyRule, 0, len(permissionRules))
	var diagnostics []RuntimePolicyDiagnostic
	seen := map[string]struct{}{}
	for _, raw := range rules {
		id := strings.TrimSpace(raw.ID)
		if id == "" {
			diagnostics = append(diagnostics, RuntimePolicyDiagnostic{Level: "error", Reason: "policy rule id is required"})
			continue
		}
		if _, ok := seen[id]; ok {
			diagnostics = append(diagnostics, RuntimePolicyDiagnostic{RuleID: id, Level: "error", Reason: "duplicate policy rule id"})
			continue
		}
		seen[id] = struct{}{}
		rule, ok := validByID[id]
		if !ok {
			diagnostics = append(diagnostics, RuntimePolicyDiagnostic{RuleID: id, Level: "error", Reason: "policy rule decision or scope is invalid"})
			continue
		}
		if rule.ShellRegex != "" {
			if _, err := regexp.Compile(rule.ShellRegex); err != nil {
				diagnostics = append(diagnostics, RuntimePolicyDiagnostic{RuleID: id, Level: "error", Reason: "invalid shell regex: " + err.Error()})
				continue
			}
		}
		out = append(out, RuntimePolicyRule{
			ID:            rule.ID,
			Decision:      string(rule.Decision),
			Source:        rule.Source,
			Reason:        rule.Reason,
			Tool:          rule.Tool,
			CapabilityID:  rule.CapabilityID,
			BuiltinTool:   rule.BuiltinTool,
			MCPServer:     rule.MCPServer,
			MCPTool:       rule.MCPTool,
			MCPResource:   rule.MCPResource,
			MCPPrompt:     rule.MCPPrompt,
			Skill:         rule.Skill,
			Subagent:      rule.Subagent,
			TaskScope:     rule.TaskScope,
			CWDPrefix:     rule.CWDPrefix,
			PathPrefix:    rule.PathPrefix,
			ShellPrefix:   rule.ShellPrefix,
			ShellRegex:    rule.ShellRegex,
			PolicyMode:    string(rule.PolicyMode),
			PolicyProfile: rule.PolicyProfile,
			ScopeKind:     rule.ScopeKind,
			ScopeValue:    rule.ScopeValue,
			Precedence:    rule.Precedence,
		})
	}
	return out, diagnostics
}

func hasRuntimePolicyErrors(diagnostics []RuntimePolicyDiagnostic) bool {
	for _, diagnostic := range diagnostics {
		if diagnostic.Level == "error" {
			return true
		}
	}
	return false
}

func (r *runtimeService) applyPolicyToWorkspace(ctx context.Context, policy RuntimePolicy) error {
	if r.runtime == nil || r.workspace == nil {
		return nil
	}
	ws, err := r.runtime.GetWorkspace(r.workspace.ID)
	if err != nil {
		return err
	}
	mode := permission.PolicyMode(policy.Mode)
	ws.Permissions.SetPolicy(runtimePermissionPolicy(policy), mode)
	return nil
}

func (r *runtimeService) GetPolicy(ctx context.Context) (RuntimePolicyResponse, error) {
	layout, err := resolveDesktopLayout()
	if err != nil {
		return RuntimePolicyResponse{}, err
	}
	policy, err := loadRuntimePolicy(layout)
	if err != nil {
		return RuntimePolicyResponse{}, err
	}
	r.mu.Lock()
	r.policy = policy
	r.mu.Unlock()
	return RuntimePolicyResponse{Policy: policy}, nil
}

func (r *runtimeService) UpdatePolicy(ctx context.Context, req RuntimePolicyUpdateRequest) (RuntimePolicyResponse, error) {
	mode, err := normalizeRuntimePolicyMode(req.Mode)
	if err != nil {
		return RuntimePolicyResponse{}, err
	}
	layout, err := resolveDesktopLayout()
	if err != nil {
		return RuntimePolicyResponse{}, err
	}
	current, err := loadRuntimePolicy(layout)
	if err != nil {
		return RuntimePolicyResponse{}, err
	}
	rules := req.Rules
	if rules == nil {
		rules = current.Rules
	}
	profile := req.Profile
	if strings.TrimSpace(profile) == "" {
		profile = current.Profile
	}
	updated := runtimePolicyFromParts(mode, profile, rules, time.Now().UnixMilli())
	if hasRuntimePolicyErrors(updated.Diagnostics) {
		return RuntimePolicyResponse{}, fmt.Errorf("invalid policy rules")
	}
	if err := saveRuntimePolicy(layout, updated); err != nil {
		return RuntimePolicyResponse{}, err
	}
	r.mu.Lock()
	started := r.runtime != nil && r.workspace != nil
	r.mu.Unlock()
	if started {
		if err := r.applyPolicyToWorkspace(ctx, updated); err != nil {
			return RuntimePolicyResponse{}, err
		}
		if err := r.runtime.UpdateAgent(ctx, r.workspace.ID); err != nil && !errors.Is(err, backend.ErrAgentNotInitialized) {
			return RuntimePolicyResponse{}, err
		}
	}
	r.mu.Lock()
	r.policy = updated
	sessionID := r.sessionID
	r.mu.Unlock()
	r.storeRuntimeEvent(runtimeapi.Event{
		ID:        newRuntimeEventID(),
		Type:      runtimeapi.EventPermissionPolicyApplied,
		CreatedAt: time.Now().UTC().Format(time.RFC3339Nano),
		SessionID: sessionID,
		Payload: map[string]any{
			"mode":        updated.Mode,
			"description": updated.Description,
			"decision":    "configure",
			"reason":      "Runtime policy mode updated.",
		},
	})
	if started {
		r.writeAudit(auditEntry{
			Event:            "permission_policy_applied",
			Timestamp:        time.Now().Format(time.RFC3339Nano),
			SessionID:        sessionID,
			PermissionPolicy: "configure",
			PolicyMode:       updated.Mode,
			PermissionReason: "Runtime policy mode updated.",
		})
	}
	return RuntimePolicyResponse{Policy: updated}, nil
}
