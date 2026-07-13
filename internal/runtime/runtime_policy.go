package runtime

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"strings"
	"time"

	"github.com/CIPFZ/agent-builder/internal/db"
	"github.com/CIPFZ/agent-builder/internal/permission"
	"github.com/CIPFZ/agent-builder/internal/runtimeapi"
	"github.com/CIPFZ/agent-builder/internal/workbench"
)

var runtimePolicyModes = []string{
	string(permission.PolicyModeAsk),
	string(permission.PolicyModeAutoRead),
	string(permission.PolicyModeFullAccess),
	string(permission.PolicyModePlan),
	string(permission.PolicyModeDenyAll),
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
		return "只读工具自动执行，其余操作请求审批。"
	case permission.PolicyModeFullAccess:
		return "工具调用自动执行，仍受 runtime 安全边界和显式拒绝规则约束。"
	case permission.PolicyModePlan:
		return "只读工具自动执行；写入、执行、网络、破坏性和敏感操作会被阻止。"
	case permission.PolicyModeDenyAll:
		return "阻止所有工具调用。"
	default:
		return "工具调用按 runtime 规则请求审批。"
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

func loadRuntimePolicy(ctx context.Context, conn *sql.DB) (RuntimePolicy, error) {
	var modeValue, profile, rulesJSON string
	var updatedAt int64
	err := conn.QueryRowContext(ctx, `SELECT mode, profile, rules_json, updated_at FROM policy_settings WHERE scope = 'global' AND project_id = ''`).Scan(&modeValue, &profile, &rulesJSON, &updatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return defaultRuntimePolicy(), nil
	}
	if err != nil {
		return RuntimePolicy{}, fmt.Errorf("load runtime policy: %w", err)
	}
	var rules []RuntimePolicyRule
	if err := json.Unmarshal([]byte(rulesJSON), &rules); err != nil {
		return RuntimePolicy{}, fmt.Errorf("decode runtime policy rules: %w", err)
	}
	mode, err := normalizeRuntimePolicyMode(modeValue)
	if err != nil {
		return RuntimePolicy{}, err
	}
	return runtimePolicyFromParts(mode, profile, rules, updatedAt), nil
}

func saveRuntimePolicy(ctx context.Context, conn *sql.DB, policy RuntimePolicy) error {
	rulesJSON, err := json.Marshal(policy.Rules)
	if err != nil {
		return fmt.Errorf("encode runtime policy rules: %w", err)
	}
	if _, err := conn.ExecContext(ctx, `INSERT INTO policy_settings (scope, project_id, mode, profile, rules_json, updated_at)
VALUES ('global', '', ?, ?, ?, ?) ON CONFLICT(scope, project_id) DO UPDATE SET mode = excluded.mode, profile = excluded.profile, rules_json = excluded.rules_json, updated_at = excluded.updated_at`, policy.Mode, policy.Profile, string(rulesJSON), policy.UpdatedAt); err != nil {
		return fmt.Errorf("save runtime policy: %w", err)
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
	conn, dataDir, err := openDesktopDB(ctx)
	if err != nil {
		return RuntimePolicyResponse{}, err
	}
	defer db.Release(dataDir) //nolint:errcheck
	policy, err := loadRuntimePolicy(ctx, conn)
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
	conn, dataDir, err := openDesktopDB(ctx)
	if err != nil {
		return RuntimePolicyResponse{}, err
	}
	defer db.Release(dataDir) //nolint:errcheck
	current, err := loadRuntimePolicy(ctx, conn)
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
	if err := saveRuntimePolicy(ctx, conn, updated); err != nil {
		return RuntimePolicyResponse{}, err
	}
	r.mu.Lock()
	started := r.runtime != nil && r.workspace != nil && r.runtimeConfigured
	r.mu.Unlock()
	if started {
		if err := r.applyPolicyToWorkspace(ctx, updated); err != nil {
			return RuntimePolicyResponse{}, err
		}
		if err := r.runtime.UpdateAgent(ctx, r.workspace.ID); err != nil && !errors.Is(err, workbench.ErrAgentNotInitialized) {
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
