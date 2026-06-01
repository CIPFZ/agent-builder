package permission

import (
	"encoding/json"
	"fmt"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"github.com/charmbracelet/crush/internal/tools/scheduler"
)

type PolicyMode string

const (
	PolicyModeAsk        PolicyMode = "ask"
	PolicyModeAutoRead   PolicyMode = "auto_read"
	PolicyModeFullAccess PolicyMode = "full_access"
	PolicyModePlan       PolicyMode = "plan"
	PolicyModeDenyAll    PolicyMode = "deny_all"
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

type PolicyProfile string

const (
	PolicyProfileDefault  PolicyProfile = "default"
	PolicyProfileHeadless PolicyProfile = "headless"
	PolicyProfileTask     PolicyProfile = "task"
	PolicyProfileRecovery PolicyProfile = "recovery"
)

type PermissionPolicy interface {
	Evaluate(scheduler.ToolCall) PolicyResult
}

type PolicyRuleDecision = PolicyDecision

type PolicyRuleScope struct {
	Kind  string `json:"kind"`
	Value string `json:"value"`
}

type PolicyRule struct {
	ID            string             `json:"id"`
	Decision      PolicyRuleDecision `json:"decision"`
	Source        string             `json:"source,omitempty"`
	Reason        string             `json:"reason,omitempty"`
	Tool          string             `json:"tool,omitempty"`
	CapabilityID  string             `json:"capability_id,omitempty"`
	BuiltinTool   string             `json:"builtin_tool,omitempty"`
	MCPServer     string             `json:"mcp_server,omitempty"`
	MCPTool       string             `json:"mcp_tool,omitempty"`
	MCPResource   string             `json:"mcp_resource,omitempty"`
	MCPPrompt     string             `json:"mcp_prompt,omitempty"`
	Skill         string             `json:"skill,omitempty"`
	Subagent      string             `json:"subagent,omitempty"`
	TaskScope     string             `json:"task_scope,omitempty"`
	CWDPrefix     string             `json:"cwd_prefix,omitempty"`
	PathPrefix    string             `json:"path_prefix,omitempty"`
	ShellPrefix   string             `json:"shell_prefix,omitempty"`
	ShellRegex    string             `json:"shell_regex,omitempty"`
	PolicyMode    PolicyMode         `json:"policy_mode,omitempty"`
	PolicyProfile string             `json:"policy_profile,omitempty"`
	ScopeKind     string             `json:"scope_kind,omitempty"`
	ScopeValue    string             `json:"scope_value,omitempty"`
	Precedence    int                `json:"precedence,omitempty"`
}

type PolicyResult struct {
	Decision       PolicyDecision
	Risk           Risk
	Reason         string
	Mode           PolicyMode
	Profile        string
	Headless       bool
	HeadlessReason string
	RuleID         string
	RuleSource     string
	RuleDecision   PolicyDecision
	RuleScopeKind  string
	RuleScopeValue string
	TargetSummary  string
	Shell          ShellClassification
}

type StaticPolicy struct {
	Mode    PolicyMode
	Profile string
	Rules   []PolicyRule
}

func NewPermissionPolicy(mode PolicyMode) StaticPolicy {
	return StaticPolicy{Mode: NormalizePolicyMode(mode)}
}

func NewScopedPermissionPolicy(mode PolicyMode, profile string, rules []PolicyRule) StaticPolicy {
	return StaticPolicy{
		Mode:    NormalizePolicyMode(mode),
		Profile: NormalizePolicyProfile(profile),
		Rules:   NormalizePolicyRules(rules),
	}
}

func NormalizePolicyMode(mode PolicyMode) PolicyMode {
	switch mode {
	case PolicyModeAsk, PolicyModeAutoRead, PolicyModeFullAccess, PolicyModePlan, PolicyModeDenyAll:
		return mode
	default:
		return PolicyModeAsk
	}
}

func NormalizePolicyProfile(profile string) string {
	normalized := strings.ToLower(strings.TrimSpace(profile))
	switch normalized {
	case "", "default", "interactive":
		return string(PolicyProfileDefault)
	case "headless", "non_interactive", "non-interactive", "background":
		return string(PolicyProfileHeadless)
	case "task", "subagent", "task/subagent", "agent_task", "agent-task":
		return string(PolicyProfileTask)
	case "recovery", "replay", "replay-safe", "replay_safe":
		return string(PolicyProfileRecovery)
	default:
		return strings.TrimSpace(profile)
	}
}

func IsHeadlessPolicyProfile(profile string) bool {
	switch NormalizePolicyProfile(profile) {
	case string(PolicyProfileHeadless), string(PolicyProfileTask), string(PolicyProfileRecovery):
		return true
	default:
		return false
	}
}

func (p StaticPolicy) Evaluate(call scheduler.ToolCall) PolicyResult {
	risk := ClassifyToolCallRisk(call)
	mode := NormalizePolicyMode(p.Mode)
	profile := NormalizePolicyProfile(p.Profile)
	target := TargetSummaryForToolCall(call)
	shell := ShellClassification{}
	if isShellToolCall(call) {
		shell = ClassifyShellCommand(call.InputSummary)
		if shell.Risk == RiskDestructive {
			risk = RiskDestructive
		}
	}
	base := p.evaluateBaseline(mode, risk)
	base.TargetSummary = target
	base.Shell = shell
	base.Profile = profile
	if rule, matched := p.matchRule(call, mode); matched {
		base.Decision = normalizePolicyDecision(rule.Decision, base.Decision)
		base.RuleID = rule.ID
		base.RuleSource = rule.Source
		base.RuleDecision = base.Decision
		base.RuleScopeKind, base.RuleScopeValue = ruleMatchedScope(rule, call)
		if rule.Reason != "" {
			base.Reason = rule.Reason
		} else {
			base.Reason = fmt.Sprintf("Scoped policy rule %s matched %s %s.", rule.ID, base.RuleScopeKind, base.RuleScopeValue)
		}
		return applyProfileSemantics(base)
	}
	return applyProfileSemantics(base)
}

func applyProfileSemantics(result PolicyResult) PolicyResult {
	result.Profile = NormalizePolicyProfile(result.Profile)
	if !IsHeadlessPolicyProfile(result.Profile) {
		return result
	}
	result.Headless = true
	if result.HeadlessReason == "" {
		result.HeadlessReason = fmt.Sprintf("Policy profile %s is non-interactive; ask decisions fail closed.", result.Profile)
	}
	if result.Decision == PolicyAsk {
		result.Decision = PolicyDeny
		if result.Reason == "" {
			result.Reason = result.HeadlessReason
		} else {
			result.Reason = result.Reason + " " + result.HeadlessReason
		}
	}
	return result
}

func (p StaticPolicy) evaluateBaseline(mode PolicyMode, risk Risk) PolicyResult {
	switch mode {
	case PolicyModeFullAccess:
		return PolicyResult{Decision: PolicyAllow, Risk: risk, Reason: "Full access mode allows tool calls without interactive approval.", Mode: mode}
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

func (p StaticPolicy) matchRule(call scheduler.ToolCall, mode PolicyMode) (PolicyRule, bool) {
	rules := NormalizePolicyRules(p.Rules)
	sort.SliceStable(rules, func(i, j int) bool {
		pi := rulePrecedence(rules[i])
		pj := rulePrecedence(rules[j])
		if pi != pj {
			return pi > pj
		}
		return strings.Compare(rules[i].ID, rules[j].ID) < 0
	})
	for _, rule := range rules {
		if rule.PolicyMode != "" && NormalizePolicyMode(rule.PolicyMode) != mode {
			continue
		}
		if rule.PolicyProfile != "" && !strings.EqualFold(NormalizePolicyProfile(rule.PolicyProfile), NormalizePolicyProfile(p.Profile)) {
			continue
		}
		if policyRuleMatches(rule, call) {
			return rule, true
		}
	}
	return PolicyRule{}, false
}

func NormalizePolicyRules(rules []PolicyRule) []PolicyRule {
	out := make([]PolicyRule, 0, len(rules))
	for _, rule := range rules {
		rule.ID = strings.TrimSpace(rule.ID)
		if rule.ID == "" {
			continue
		}
		rule.Decision = normalizePolicyDecision(rule.Decision, "")
		if rule.Decision == "" {
			continue
		}
		rule.Source = strings.TrimSpace(rule.Source)
		rule.Reason = strings.TrimSpace(rule.Reason)
		rule.Tool = strings.TrimSpace(rule.Tool)
		rule.CapabilityID = strings.TrimSpace(rule.CapabilityID)
		rule.BuiltinTool = strings.TrimSpace(rule.BuiltinTool)
		rule.MCPServer = strings.TrimSpace(rule.MCPServer)
		rule.MCPTool = strings.TrimSpace(rule.MCPTool)
		rule.MCPResource = strings.TrimSpace(rule.MCPResource)
		rule.MCPPrompt = strings.TrimSpace(rule.MCPPrompt)
		rule.Skill = strings.TrimSpace(rule.Skill)
		rule.Subagent = strings.TrimSpace(rule.Subagent)
		rule.TaskScope = strings.TrimSpace(rule.TaskScope)
		rule.CWDPrefix = cleanPathPrefix(rule.CWDPrefix)
		rule.PathPrefix = cleanPathPrefix(rule.PathPrefix)
		rule.ShellPrefix = strings.TrimSpace(rule.ShellPrefix)
		rule.ShellRegex = strings.TrimSpace(rule.ShellRegex)
		if strings.TrimSpace(string(rule.PolicyMode)) != "" {
			rule.PolicyMode = NormalizePolicyMode(rule.PolicyMode)
		}
		if strings.TrimSpace(rule.PolicyProfile) != "" {
			rule.PolicyProfile = NormalizePolicyProfile(rule.PolicyProfile)
		}
		rule.ScopeKind = strings.TrimSpace(rule.ScopeKind)
		rule.ScopeValue = strings.TrimSpace(rule.ScopeValue)
		if rule.Precedence == 0 {
			rule.Precedence = rulePrecedence(rule)
		}
		out = append(out, rule)
	}
	return out
}

func normalizePolicyDecision(decision PolicyDecision, fallback PolicyDecision) PolicyDecision {
	switch decision {
	case PolicyAllow, PolicyAsk, PolicyDeny:
		return decision
	default:
		return fallback
	}
}

func ClassifyRisk(toolName, inputSummary string) Risk {
	name := strings.ToLower(strings.TrimSpace(toolName))
	input := strings.ToLower(inputSummary)
	switch {
	case strings.Contains(input, "token") || strings.Contains(input, "api_key") || strings.Contains(input, "secret"):
		return RiskSecret
	case strings.Contains(input, "read mcp") || strings.Contains(input, "list mcp") || strings.Contains(input, "read skill") || strings.Contains(input, "skill activation"):
		return RiskRead
	case isReadOnlyBuiltinTool(name):
		return RiskRead
	case name == "todos":
		return RiskWrite
	case name == "agent" || name == "agentic_fetch":
		return RiskExecute
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
		return RiskExecute
	}
}

func isReadOnlyBuiltinTool(name string) bool {
	switch name {
	case "view", "ls", "grep", "glob", "rg", "diagnostics", "references", "crush_info", "crush_logs", "job_output", "list_mcp_resources", "read_mcp_resource", "context_activation":
		return true
	default:
		return false
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

func TargetSummaryForToolCall(call scheduler.ToolCall) string {
	if strings.TrimSpace(call.Command) != "" {
		return "shell:" + previewPolicyText(call.Command, 160)
	}
	if call.CapabilityID != "" {
		return call.CapabilityID
	}
	if call.Source != "" {
		return string(call.Source) + ":" + call.Name
	}
	return call.Name
}

func isDestructiveInput(inputSummary string) bool {
	return ClassifyShellCommandRisk(inputSummary) == RiskDestructive
}

type ShellClassification struct {
	Risk          Risk   `json:"risk,omitempty"`
	Reason        string `json:"reason,omitempty"`
	TargetSummary string `json:"target_summary,omitempty"`
	Command       string `json:"command,omitempty"`
	Shell         string `json:"shell,omitempty"`
}

func ClassifyShellCommandRisk(inputSummary string) Risk {
	return ClassifyShellCommand(inputSummary).Risk
}

func ClassifyShellCommand(inputSummary string) ShellClassification {
	command := strings.TrimSpace(extractShellCommand(inputSummary))
	result := ShellClassification{
		Risk:          RiskExecute,
		Command:       command,
		Shell:         detectShell(command),
		TargetSummary: previewPolicyText(command, 160),
		Reason:        "Shell command execution requires approval.",
	}
	if command == "" {
		result.Reason = "Empty shell command cannot be proven safe."
		return result
	}
	if reason, target := classifyDestructiveShellCommand(command); reason != "" {
		result.Risk = RiskDestructive
		result.Reason = reason
		result.TargetSummary = target
	}
	return result
}

func extractShellCommand(inputSummary string) string {
	var payload map[string]any
	if err := json.Unmarshal([]byte(inputSummary), &payload); err == nil {
		for _, key := range []string{"command", "Command", "cmd", "script"} {
			if value, ok := payload[key].(string); ok {
				return value
			}
		}
	}
	return inputSummary
}

func policyRuleMatches(rule PolicyRule, call scheduler.ToolCall) bool {
	seen := false
	if rule.Tool != "" {
		seen = true
		if !strings.EqualFold(rule.Tool, call.Name) {
			return false
		}
	}
	if rule.CapabilityID != "" {
		seen = true
		if !matchPolicyValue(rule.CapabilityID, call.CapabilityID) {
			return false
		}
	}
	if rule.BuiltinTool != "" {
		seen = true
		if call.Source != scheduler.ToolSourceBuiltin || !strings.EqualFold(rule.BuiltinTool, call.Name) {
			return false
		}
	}
	if rule.MCPServer != "" || rule.MCPTool != "" || rule.MCPResource != "" || rule.MCPPrompt != "" {
		seen = true
		if call.Source != scheduler.ToolSourceMCP && !strings.HasPrefix(strings.ToLower(call.CapabilityID), "mcp") {
			return false
		}
		parts := strings.Split(call.CapabilityID, ":")
		if rule.MCPServer != "" && !mcpCapabilityPartMatches(parts, 1, rule.MCPServer) {
			return false
		}
		if rule.MCPTool != "" && !(strings.HasPrefix(call.CapabilityID, "mcp:") && mcpCapabilityPartMatches(parts, 2, rule.MCPTool)) {
			return false
		}
		if rule.MCPResource != "" && !(strings.HasPrefix(call.CapabilityID, "mcp_resource:") && mcpCapabilityPartMatches(parts, 2, rule.MCPResource)) {
			return false
		}
		if rule.MCPPrompt != "" && !(strings.HasPrefix(call.CapabilityID, "mcp_prompt:") && mcpCapabilityPartMatches(parts, 2, rule.MCPPrompt)) {
			return false
		}
	}
	if rule.Skill != "" {
		seen = true
		if !matchPolicyValue("skill:"+rule.Skill, call.CapabilityID) && !strings.EqualFold(rule.Skill, call.Name) {
			return false
		}
	}
	if rule.Subagent != "" || rule.TaskScope != "" {
		seen = true
		if rule.Subagent != "" && !matchesAnyText(rule.Subagent, call.CapabilityID, call.Name, call.InputSummary) {
			return false
		}
		if rule.TaskScope != "" && !matchesAnyText(rule.TaskScope, call.CapabilityID, call.Name, call.InputSummary) {
			return false
		}
	}
	if rule.CWDPrefix != "" {
		seen = true
		if !pathPrefixMatches(rule.CWDPrefix, extractStringFromInput(call.InputSummary, "working_dir", "cwd")) {
			return false
		}
	}
	if rule.PathPrefix != "" {
		seen = true
		if !pathPrefixMatches(rule.PathPrefix, extractStringFromInput(call.InputSummary, "path", "file_path", "target", "uri", "working_dir")) {
			return false
		}
	}
	command := firstNonEmpty(call.Command, extractShellCommand(call.InputSummary))
	if rule.ShellPrefix != "" {
		seen = true
		if !strings.HasPrefix(strings.TrimSpace(command), rule.ShellPrefix) {
			return false
		}
	}
	if rule.ShellRegex != "" {
		seen = true
		re, err := regexp.Compile(rule.ShellRegex)
		if err != nil || !re.MatchString(command) {
			return false
		}
	}
	if rule.ScopeKind != "" || rule.ScopeValue != "" {
		seen = true
		if !genericScopeMatches(rule, call) {
			return false
		}
	}
	return seen
}

func ruleMatchedScope(rule PolicyRule, call scheduler.ToolCall) (string, string) {
	switch {
	case rule.CapabilityID != "":
		return "capability", rule.CapabilityID
	case rule.BuiltinTool != "":
		return "builtin_tool", rule.BuiltinTool
	case rule.MCPTool != "":
		return "mcp_tool", firstNonEmpty(rule.MCPServer, "*") + ":" + rule.MCPTool
	case rule.MCPResource != "":
		return "mcp_resource", firstNonEmpty(rule.MCPServer, "*") + ":" + rule.MCPResource
	case rule.MCPPrompt != "":
		return "mcp_prompt", firstNonEmpty(rule.MCPServer, "*") + ":" + rule.MCPPrompt
	case rule.MCPServer != "":
		return "mcp_server", rule.MCPServer
	case rule.Skill != "":
		return "skill", rule.Skill
	case rule.Subagent != "":
		return "subagent", rule.Subagent
	case rule.TaskScope != "":
		return "task_scope", rule.TaskScope
	case rule.CWDPrefix != "":
		return "cwd_prefix", rule.CWDPrefix
	case rule.PathPrefix != "":
		return "path_prefix", rule.PathPrefix
	case rule.ShellRegex != "":
		return "shell_regex", rule.ShellRegex
	case rule.ShellPrefix != "":
		return "shell_prefix", rule.ShellPrefix
	case rule.ScopeKind != "":
		return rule.ScopeKind, rule.ScopeValue
	case rule.Tool != "":
		return "tool", rule.Tool
	default:
		return "tool", call.Name
	}
}

func rulePrecedence(rule PolicyRule) int {
	if rule.Precedence > 0 {
		return rule.Precedence
	}
	score := 0
	if rule.Decision == PolicyDeny {
		score += 3000
	} else if rule.Decision == PolicyAsk {
		score += 2000
	} else {
		score += 1000
	}
	for _, value := range []string{rule.CapabilityID, rule.MCPTool, rule.MCPResource, rule.MCPPrompt, rule.Skill, rule.Subagent, rule.TaskScope, rule.ShellRegex} {
		if value != "" {
			score += 100
		}
	}
	for _, value := range []string{rule.BuiltinTool, rule.MCPServer, rule.Tool, rule.CWDPrefix, rule.PathPrefix, rule.ShellPrefix, rule.ScopeKind} {
		if value != "" {
			score += 50
		}
	}
	if rule.PolicyMode != "" {
		score += 10
	}
	if rule.PolicyProfile != "" {
		score += 10
	}
	return score
}

func matchPolicyValue(pattern, value string) bool {
	pattern = strings.TrimSpace(pattern)
	value = strings.TrimSpace(value)
	if pattern == "" {
		return true
	}
	if pattern == "*" {
		return value != ""
	}
	if strings.HasSuffix(pattern, "*") {
		return strings.HasPrefix(strings.ToLower(value), strings.ToLower(strings.TrimSuffix(pattern, "*")))
	}
	return strings.EqualFold(pattern, value)
}

func mcpCapabilityPartMatches(parts []string, index int, pattern string) bool {
	if index >= len(parts) {
		return false
	}
	if index == 1 {
		return matchPolicyValue(pattern, parts[index])
	}
	return matchPolicyValue(pattern, strings.Join(parts[index:], ":"))
}

func matchesAnyText(pattern string, values ...string) bool {
	if strings.TrimSpace(pattern) == "" {
		return true
	}
	for _, value := range values {
		if strings.Contains(strings.ToLower(value), strings.ToLower(pattern)) {
			return true
		}
	}
	return false
}

func genericScopeMatches(rule PolicyRule, call scheduler.ToolCall) bool {
	kind := strings.ToLower(strings.TrimSpace(rule.ScopeKind))
	value := strings.TrimSpace(rule.ScopeValue)
	switch kind {
	case "", "tool":
		return matchPolicyValue(value, call.Name)
	case "capability", "capability_id":
		return matchPolicyValue(value, call.CapabilityID)
	case "source":
		return matchPolicyValue(value, string(call.Source))
	case "shell_prefix":
		return strings.HasPrefix(strings.TrimSpace(extractShellCommand(call.InputSummary)), value)
	case "path_prefix":
		return pathPrefixMatches(value, extractStringFromInput(call.InputSummary, "path", "file_path", "target", "working_dir"))
	default:
		return matchesAnyText(value, call.CapabilityID, call.Name, call.InputSummary)
	}
}

func extractStringFromInput(input string, keys ...string) string {
	var payload map[string]any
	if err := json.Unmarshal([]byte(input), &payload); err != nil {
		return ""
	}
	for _, key := range keys {
		if value, ok := payload[key].(string); ok && strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}

func cleanPathPrefix(path string) string {
	path = strings.TrimSpace(path)
	if path == "" {
		return ""
	}
	path = filepath.FromSlash(path)
	if looksLikeWindowsAbs(path) {
		return cleanWindowsPath(path)
	}
	if cleaned, err := filepath.Abs(path); err == nil {
		path = cleaned
	}
	return filepath.Clean(path)
}

func pathPrefixMatches(prefix, path string) bool {
	prefix = cleanPathPrefix(prefix)
	path = cleanPathPrefix(path)
	if prefix == "" || path == "" {
		return false
	}
	if looksLikeWindowsAbs(prefix) || looksLikeWindowsAbs(path) {
		prefix = strings.ToLower(cleanWindowsPath(prefix))
		path = strings.ToLower(cleanWindowsPath(path))
		return path == prefix || strings.HasPrefix(path, prefix+`\`)
	}
	prefix = strings.ToLower(filepath.Clean(prefix))
	path = strings.ToLower(filepath.Clean(path))
	return path == prefix || strings.HasPrefix(path, prefix+string(filepath.Separator))
}

func looksLikeWindowsAbs(path string) bool {
	return len(path) >= 3 && path[1] == ':' && (path[2] == '\\' || path[2] == '/')
}

func cleanWindowsPath(path string) string {
	path = strings.ReplaceAll(path, "/", `\`)
	volume := path[:2]
	rest := strings.TrimPrefix(path[2:], `\`)
	parts := strings.Split(rest, `\`)
	stack := make([]string, 0, len(parts))
	for _, part := range parts {
		switch part {
		case "", ".":
			continue
		case "..":
			if len(stack) > 0 {
				stack = stack[:len(stack)-1]
			}
		default:
			stack = append(stack, part)
		}
	}
	if len(stack) == 0 {
		return volume + `\`
	}
	return volume + `\` + strings.Join(stack, `\`)
}

func isShellToolCall(call scheduler.ToolCall) bool {
	return call.Source == scheduler.ToolSourceShell || strings.EqualFold(call.Name, "bash") || strings.EqualFold(call.Name, "shell")
}

func previewPolicyText(value string, limit int) string {
	value = strings.Join(strings.Fields(value), " ")
	if len(value) <= limit {
		return value
	}
	if limit <= 3 {
		return value[:limit]
	}
	return value[:limit-3] + "..."
}

func classifyDestructiveShellCommand(command string) (string, string) {
	statements := splitShellStatements(command)
	for _, statement := range statements {
		tokens := shellFields(statement)
		if len(tokens) == 0 {
			continue
		}
		if reason, target := classifyTokenizedCommand(tokens, statement); reason != "" {
			return reason, target
		}
		if reason, target := classifyPatternFallback(statement); reason != "" {
			return reason, target
		}
	}
	if reason, target := classifyPatternFallback(command); reason != "" {
		return reason, target
	}
	return "", ""
}

func classifyTokenizedCommand(tokens []string, statement string) (string, string) {
	cmd := normalizeCommandToken(tokens[0])
	lowerTokens := make([]string, 0, len(tokens))
	for _, token := range tokens {
		lowerTokens = append(lowerTokens, strings.ToLower(token))
	}
	switch cmd {
	case "rm":
		if hasPowerShellFlag(lowerTokens, "recurse") && hasPowerShellFlag(lowerTokens, "force") {
			return "Shell policy detected PowerShell recursive forced delete.", commandTargetSummary(cmd, tokens)
		}
		if hasPowerShellFlag(lowerTokens, "recurse") {
			return "Shell policy detected PowerShell recursive delete.", commandTargetSummary(cmd, tokens)
		}
		if hasPowerShellFlag(lowerTokens, "force") {
			return "Shell policy detected PowerShell forced delete.", commandTargetSummary(cmd, tokens)
		}
		if hasShortFlag(lowerTokens, "r") && hasShortFlag(lowerTokens, "f") {
			return "Shell policy detected recursive forced delete.", commandTargetSummary("rm", tokens)
		}
		if hasShortFlag(lowerTokens, "r") || hasLongFlag(lowerTokens, "--recursive") {
			return "Shell policy detected recursive delete.", commandTargetSummary("rm", tokens)
		}
		if hasShortFlag(lowerTokens, "f") || hasLongFlag(lowerTokens, "--force") {
			return "Shell policy detected forced delete.", commandTargetSummary("rm", tokens)
		}
	case "del", "erase":
		if hasPowerShellFlag(lowerTokens, "recurse") && hasPowerShellFlag(lowerTokens, "force") {
			return "Shell policy detected PowerShell recursive forced delete.", commandTargetSummary(cmd, tokens)
		}
		if hasPowerShellFlag(lowerTokens, "recurse") {
			return "Shell policy detected PowerShell recursive delete.", commandTargetSummary(cmd, tokens)
		}
		if hasPowerShellFlag(lowerTokens, "force") {
			return "Shell policy detected PowerShell forced delete.", commandTargetSummary(cmd, tokens)
		}
		if hasSlashFlag(lowerTokens, "/s") || hasSlashFlag(lowerTokens, "/q") || hasSlashFlag(lowerTokens, "/f") {
			return "Shell policy detected cmd delete with dangerous flags.", commandTargetSummary(cmd, tokens)
		}
	case "rmdir", "rd":
		if hasSlashFlag(lowerTokens, "/s") || hasShortFlag(lowerTokens, "r") {
			return "Shell policy detected recursive directory delete.", commandTargetSummary(cmd, tokens)
		}
	case "remove-item", "ri":
		if hasPowerShellFlag(lowerTokens, "recurse") && hasPowerShellFlag(lowerTokens, "force") {
			return "Shell policy detected PowerShell recursive forced delete.", commandTargetSummary(cmd, tokens)
		}
		if hasPowerShellFlag(lowerTokens, "recurse") {
			return "Shell policy detected PowerShell recursive delete.", commandTargetSummary(cmd, tokens)
		}
		if hasPowerShellFlag(lowerTokens, "force") {
			return "Shell policy detected PowerShell forced delete.", commandTargetSummary(cmd, tokens)
		}
	case "git":
		if len(lowerTokens) >= 3 && lowerTokens[1] == "reset" && containsToken(lowerTokens[2:], "--hard") {
			return "Shell policy detected git reset --hard.", commandTargetSummary("git reset", tokens)
		}
		if len(lowerTokens) >= 3 && lowerTokens[1] == "checkout" && destructiveGitCheckout(lowerTokens[2:]) {
			return "Shell policy detected destructive git checkout.", commandTargetSummary("git checkout", tokens)
		}
		if len(lowerTokens) >= 3 && lowerTokens[1] == "restore" && destructiveGitCheckout(lowerTokens[2:]) {
			return "Shell policy detected destructive git restore.", commandTargetSummary("git restore", tokens)
		}
		if len(lowerTokens) >= 3 && lowerTokens[1] == "clean" && hasShortFlag(lowerTokens[2:], "f") && !containsToken(lowerTokens[2:], "--dry-run") && !hasShortFlag(lowerTokens[2:], "n") {
			return "Shell policy detected git clean forced delete.", commandTargetSummary("git clean", tokens)
		}
	case "kill", "killall", "pkill", "taskkill", "stop-process":
		return "Shell policy detected process termination.", commandTargetSummary(cmd, tokens)
	case "chmod", "chown":
		return "Shell policy detected ownership or permission change.", commandTargetSummary(cmd, tokens)
	case "clear-content", "set-content", "out-file", "sc":
		return "Shell policy detected content overwrite command.", commandTargetSummary(cmd, tokens)
	}
	if redirectsOverwrite(statement) {
		return "Shell policy detected redirection overwrite.", previewPolicyText(statement, 160)
	}
	return "", ""
}

var fallbackDestructiveShellPatterns = []struct {
	pattern *regexp.Regexp
	reason  string
}{
	{regexp.MustCompile(`(?i)\bgit\s+reset\s+--hard\b`), "Shell policy detected git reset --hard."},
	{regexp.MustCompile(`(?i)\bgit\s+(checkout|restore)\s+(--\s+)?\.`), "Shell policy detected destructive git checkout or restore."},
	{regexp.MustCompile(`(?i)(^|[;&|()\s])(kill|killall|pkill|stop-process|taskkill)(\s|$)`), "Shell policy detected process termination."},
	{regexp.MustCompile(`(?i)(^|[;&|()\s])(chmod|chown)(\s|$)`), "Shell policy detected ownership or permission change."},
	{regexp.MustCompile(`(?i)(^|[;&|()\s])rm(\.exe)?\s+[^;&|]*-[a-z]*r[a-z]*f|(^|[;&|()\s])rm(\.exe)?\s+[^;&|]*-[a-z]*f[a-z]*r`), "Shell policy detected recursive forced delete."},
	{regexp.MustCompile(`(?i)(^|[;&|()\s])(remove-item|rm|ri)\s+[^;&|]*(?:-recurse|-r)\b[^;&|]*(?:-force|-f)\b`), "Shell policy detected PowerShell recursive forced delete."},
	{regexp.MustCompile(`(?i)(^|[;&|()\s])(remove-item|rm|ri)\s+[^;&|]*(?:-recurse|-force)\b`), "Shell policy detected PowerShell destructive delete."},
	{regexp.MustCompile(`(?i)(^|[;&|()\s])(del|erase|rmdir|rd)(\.exe)?\s+[^;&|]*(/s|/q|/f)\b`), "Shell policy detected cmd delete with dangerous flags."},
	{regexp.MustCompile(`(?i)\b(clear-content|set-content|out-file)\b`), "Shell policy detected content overwrite command."},
	{regexp.MustCompile(`(?i)(^|[;&|()\s])remove-item\s+[^;&|]*(?:-literalpath|-path)\s+["']?[^;&|]+["']?[^;&|]*(?:-recurse|-force)\b`), "Shell policy detected PowerShell destructive delete."},
	{regexp.MustCompile(`(?i)(^|[;&|()\s])git\s+clean\s+[^;&|]*-[a-z]*f[a-z]*`), "Shell policy detected git clean forced delete."},
}

func classifyPatternFallback(command string) (string, string) {
	for _, item := range fallbackDestructiveShellPatterns {
		if item.pattern.MatchString(command) {
			return item.reason, previewPolicyText(command, 160)
		}
	}
	if redirectsOverwrite(command) {
		return "Shell policy detected redirection overwrite.", previewPolicyText(command, 160)
	}
	return "", ""
}

func splitShellStatements(command string) []string {
	var out []string
	var current strings.Builder
	var quote rune
	escaped := false
	for _, r := range command {
		if escaped {
			current.WriteRune(r)
			escaped = false
			continue
		}
		if r == '\\' || r == '`' {
			current.WriteRune(r)
			escaped = true
			continue
		}
		if quote != 0 {
			current.WriteRune(r)
			if r == quote {
				quote = 0
			}
			continue
		}
		if r == '\'' || r == '"' {
			quote = r
			current.WriteRune(r)
			continue
		}
		if strings.ContainsRune(";&|\n", r) {
			if text := strings.TrimSpace(current.String()); text != "" {
				out = append(out, text)
			}
			current.Reset()
			continue
		}
		current.WriteRune(r)
	}
	if text := strings.TrimSpace(current.String()); text != "" {
		out = append(out, text)
	}
	if len(out) == 0 {
		return []string{command}
	}
	return out
}

func shellFields(statement string) []string {
	var out []string
	var current strings.Builder
	var quote rune
	escaped := false
	for _, r := range strings.TrimSpace(statement) {
		if escaped {
			current.WriteRune(r)
			escaped = false
			continue
		}
		if r == '\\' || r == '`' {
			escaped = true
			continue
		}
		if quote != 0 {
			if r == quote {
				quote = 0
				continue
			}
			current.WriteRune(r)
			continue
		}
		if r == '\'' || r == '"' {
			quote = r
			continue
		}
		if r == ' ' || r == '\t' || r == '\r' || r == '\n' {
			if current.Len() > 0 {
				out = append(out, current.String())
				current.Reset()
			}
			continue
		}
		current.WriteRune(r)
	}
	if current.Len() > 0 {
		out = append(out, current.String())
	}
	return out
}

func normalizeCommandToken(token string) string {
	token = strings.Trim(strings.ToLower(token), `"'`)
	token = strings.TrimSuffix(token, ".exe")
	if idx := strings.LastIndexAny(token, `/\`); idx >= 0 {
		token = token[idx+1:]
	}
	return token
}

func hasShortFlag(tokens []string, flag string) bool {
	for _, token := range tokens {
		if strings.HasPrefix(token, "-") && !strings.HasPrefix(token, "--") && strings.Contains(token[1:], flag) {
			return true
		}
	}
	return false
}

func hasLongFlag(tokens []string, flag string) bool {
	return containsToken(tokens, strings.ToLower(flag))
}

func hasSlashFlag(tokens []string, flag string) bool {
	flag = strings.ToLower(flag)
	for _, token := range tokens {
		if strings.EqualFold(token, flag) || strings.HasPrefix(strings.ToLower(token), flag+":") {
			return true
		}
	}
	return false
}

func hasPowerShellFlag(tokens []string, flag string) bool {
	flag = strings.TrimLeft(strings.ToLower(flag), "-")
	for _, token := range tokens {
		token = strings.TrimLeft(strings.ToLower(token), "-/")
		if token == flag || strings.HasPrefix(token, flag+":") {
			return true
		}
	}
	return false
}

func containsToken(tokens []string, want string) bool {
	for _, token := range tokens {
		if strings.EqualFold(token, want) {
			return true
		}
	}
	return false
}

func destructiveGitCheckout(args []string) bool {
	for i, token := range args {
		if token == "." {
			return true
		}
		if token == "--" && i+1 < len(args) && args[i+1] == "." {
			return true
		}
	}
	return false
}

func redirectsOverwrite(statement string) bool {
	inQuote := rune(0)
	escaped := false
	for i, r := range statement {
		if escaped {
			escaped = false
			continue
		}
		if r == '\\' || r == '`' {
			escaped = true
			continue
		}
		if inQuote != 0 {
			if r == inQuote {
				inQuote = 0
			}
			continue
		}
		if r == '\'' || r == '"' {
			inQuote = r
			continue
		}
		if r != '>' {
			continue
		}
		prev := rune(0)
		if i > 0 {
			prev = rune(statement[i-1])
		}
		next := rune(0)
		if i+1 < len(statement) {
			next = rune(statement[i+1])
		}
		if prev == '>' || next == '>' || next == '&' {
			continue
		}
		return true
	}
	return false
}

func commandTargetSummary(prefix string, tokens []string) string {
	var targets []string
	for _, token := range tokens[1:] {
		if strings.HasPrefix(token, "-") || strings.HasPrefix(token, "/") {
			continue
		}
		targets = append(targets, token)
	}
	if len(targets) == 0 {
		return prefix
	}
	return previewPolicyText(prefix+" "+strings.Join(targets, " "), 160)
}

func detectShell(command string) string {
	lower := strings.ToLower(command)
	switch {
	case strings.Contains(lower, "remove-item") || strings.Contains(lower, "stop-process") || strings.Contains(lower, "out-file"):
		return "powershell"
	case strings.Contains(lower, "del /") || strings.Contains(lower, "rmdir /") || strings.Contains(lower, "taskkill"):
		return "cmd"
	default:
		return "shell"
	}
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
