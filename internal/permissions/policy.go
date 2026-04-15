package permissions

import (
	"encoding/json"
	"strings"
)

type Mode string

const (
	ModeDefault           Mode = "default"
	ModeAsk               Mode = "ask"
	ModeAcceptEdits       Mode = "acceptEdits"
	ModePlan              Mode = "plan"
	ModeAuto              Mode = "auto"
	ModeWorkspaceWrite    Mode = "workspace-write"
	ModeDangerFullAccess  Mode = "danger-full-access"
	ModeBypassPermissions Mode = "bypassPermissions"
	ModeDontAsk           Mode = "dontAsk"
)

type Policy struct {
	Mode                     Mode
	SubagentMode             Mode
	PlanMode                 bool
	AutoMode                 bool
	WorkspaceRoots           []string
	RuleLayers               []RuleLayer
	Rules                    []Rule
	DangerousCommandPatterns []string
	AutoClassifier           AutoClassifier
}

type AutoClassifier func(Request) (Decision, bool)

type Request struct {
	ToolName            string
	Command             string
	WorkDir             string
	ReadOnly            bool
	Destructive         bool
	AutoClassifierInput any
}

type Action string

const (
	ActionAllow Action = "allow"
	ActionAsk   Action = "ask"
	ActionDeny  Action = "deny"
)

type Match struct {
	CommandContains []string `json:"command_contains,omitempty"`
	WorkDirPrefixes []string `json:"workdir_prefixes,omitempty"`
}

type Rule struct {
	ToolName string `json:"tool_name"`
	Source   string `json:"source,omitempty"`
	Action   Action `json:"action"`
	Match    Match  `json:"match"`
}

type RuleSource string

const (
	RuleSourceSystem  RuleSource = "system"
	RuleSourceConfig  RuleSource = "config"
	RuleSourceProject RuleSource = "project"
	RuleSourceLocal   RuleSource = "local"
	RuleSourceFlag    RuleSource = "flag"
	RuleSourcePolicy  RuleSource = "policy"
	RuleSourceCLIArg  RuleSource = "cli"
	RuleSourceCommand RuleSource = "command"
	RuleSourceSession RuleSource = "session"
)

type RuleLayer struct {
	Source RuleSource
	Rules  []Rule
}

type Decision struct {
	Allowed            bool
	RequiresApproval   bool
	Category           Category
	RuleSource         string
	Reason             string
	DecisionReason     DecisionReason
	UpdatedInput       string
	UpdatedInputObject map[string]any
	UpdatedPermissions []PermissionUpdate
	AcceptFeedback     string
	ContentBlocks      []map[string]any
}

type DecisionReasonType string

const (
	DecisionReasonRule                 DecisionReasonType = "rule"
	DecisionReasonMode                 DecisionReasonType = "mode"
	DecisionReasonSubcommandResults    DecisionReasonType = "subcommandResults"
	DecisionReasonPermissionPromptTool DecisionReasonType = "permissionPromptTool"
	DecisionReasonHook                 DecisionReasonType = "hook"
	DecisionReasonAsyncAgent           DecisionReasonType = "asyncAgent"
	DecisionReasonSandboxOverride      DecisionReasonType = "sandboxOverride"
	DecisionReasonClassifier           DecisionReasonType = "classifier"
	DecisionReasonWorkingDir           DecisionReasonType = "workingDir"
	DecisionReasonSafetyCheck          DecisionReasonType = "safetyCheck"
	DecisionReasonOther                DecisionReasonType = "other"
)

type DecisionReason struct {
	Type                     DecisionReasonType
	Rule                     *Rule
	Mode                     Mode
	HookName                 string
	HookSource               string
	Reason                   string
	Classifier               string
	PermissionPromptToolName string
	ToolResult               any
}

type PermissionUpdateType string

const (
	PermissionUpdateAddRules          PermissionUpdateType = "addRules"
	PermissionUpdateReplaceRules      PermissionUpdateType = "replaceRules"
	PermissionUpdateRemoveRules       PermissionUpdateType = "removeRules"
	PermissionUpdateSetMode           PermissionUpdateType = "setMode"
	PermissionUpdateAddDirectories    PermissionUpdateType = "addDirectories"
	PermissionUpdateRemoveDirectories PermissionUpdateType = "removeDirectories"
)

type PermissionUpdateDestination string

const (
	PermissionUpdateDestinationUserSettings    PermissionUpdateDestination = "userSettings"
	PermissionUpdateDestinationProjectSettings PermissionUpdateDestination = "projectSettings"
	PermissionUpdateDestinationLocalSettings   PermissionUpdateDestination = "localSettings"
	PermissionUpdateDestinationSession         PermissionUpdateDestination = "session"
	PermissionUpdateDestinationCLIArg          PermissionUpdateDestination = "cliArg"
)

type PermissionRuleValue struct {
	ToolName    string `json:"toolName"`
	RuleContent string `json:"ruleContent,omitempty"`
}

type PermissionUpdate struct {
	Type        PermissionUpdateType        `json:"type"`
	Destination PermissionUpdateDestination `json:"destination"`
	Rules       []PermissionRuleValue       `json:"rules,omitempty"`
	Behavior    Action                      `json:"behavior,omitempty"`
	Mode        Mode                        `json:"mode,omitempty"`
	Directories []string                    `json:"directories,omitempty"`
}

type Category string

const (
	CategoryApproval          Category = "approval"
	CategoryWorkspaceBoundary Category = "workspace-boundary"
	CategoryPlanMode          Category = "plan-mode"
	CategoryDangerousCommand  Category = "dangerous-command"
	CategoryDestructiveTool   Category = "destructive-tool"
	CategoryRuleDenied        Category = "rule-denied"
)

func (p Policy) Evaluate(req Request) Decision {
	for _, rule := range p.Rules {
		if !rule.matches(req) {
			continue
		}
		switch rule.Action {
		case ActionAllow:
			return Decision{Allowed: true, RuleSource: rule.Source, Reason: "allowed by rule", DecisionReason: DecisionReason{Type: DecisionReasonRule, Rule: &rule}}
		case ActionAsk:
			return p.finalizeDecision(Decision{
				RequiresApproval: true,
				Category:         CategoryApproval,
				RuleSource:       rule.Source,
				Reason:           "approval required by rule",
				DecisionReason:   DecisionReason{Type: DecisionReasonRule, Rule: &rule},
			})
		case ActionDeny:
			return Decision{Category: CategoryRuleDenied, RuleSource: rule.Source, Reason: "denied by rule", DecisionReason: DecisionReason{Type: DecisionReasonRule, Rule: &rule}}
		}
	}
	if p.Mode == ModeBypassPermissions {
		return Decision{Allowed: true, Reason: "bypassed by permission mode", DecisionReason: DecisionReason{Type: DecisionReasonMode, Mode: p.Mode}}
	}
	if !isShellTool(req.ToolName) {
		if req.Destructive {
			if p.PlanMode {
				return p.finalizeDecision(Decision{
					RequiresApproval: true,
					Category:         CategoryPlanMode,
					Reason:           "plan mode requires approval for destructive tool actions",
				})
			}
			switch p.Mode {
			case ModeDangerFullAccess, ModeWorkspaceWrite, ModeAcceptEdits:
				return Decision{Allowed: true, Reason: "destructive non-system tool allowed by mode", DecisionReason: DecisionReason{Type: DecisionReasonMode, Mode: p.Mode}}
			case ModeDefault, ModeAsk, ModePlan, ModeAuto, ModeDontAsk:
				fallthrough
			default:
				return p.finalizeDecision(Decision{
					RequiresApproval: true,
					Category:         CategoryDestructiveTool,
					Reason:           "destructive tool action requires explicit approval",
				})
			}
		}
		return Decision{Allowed: true, Reason: "non-system tool allowed by default", DecisionReason: DecisionReason{Type: DecisionReasonMode, Mode: p.Mode}}
	}
	if p.PlanMode {
		return p.finalizeDecision(Decision{
			RequiresApproval: true,
			Category:         CategoryPlanMode,
			Reason:           "plan mode requires approval for system actions",
		})
	}
	if matchesAny(req.Command, p.DangerousCommandPatterns) {
		return p.finalizeDecision(Decision{
			RequiresApproval: true,
			Category:         CategoryDangerousCommand,
			Reason:           "dangerous command requires explicit approval",
		})
	}

	switch p.Mode {
	case ModeDangerFullAccess:
		return Decision{Allowed: true, DecisionReason: DecisionReason{Type: DecisionReasonMode, Mode: p.Mode}}
	case ModePlan:
		return p.finalizeDecision(Decision{
			RequiresApproval: true,
			Category:         CategoryPlanMode,
			Reason:           "plan mode requires approval for system actions",
		})
	case ModeAuto:
		if p.AutoClassifier != nil {
			if decision, ok := p.AutoClassifier(req); ok {
				return p.finalizeDecision(decision)
			}
		}
		if insideAnyRoot(req.WorkDir, p.WorkspaceRoots) {
			return Decision{Allowed: true, DecisionReason: DecisionReason{Type: DecisionReasonMode, Mode: p.Mode}}
		}
		return p.finalizeDecision(Decision{
			RequiresApproval: true,
			Category:         CategoryWorkspaceBoundary,
			Reason:           "requested operation is outside configured workspace roots",
		})
	case ModeWorkspaceWrite:
		if insideAnyRoot(req.WorkDir, p.WorkspaceRoots) {
			return Decision{Allowed: true, DecisionReason: DecisionReason{Type: DecisionReasonMode, Mode: p.Mode}}
		}
		return p.finalizeDecision(Decision{
			RequiresApproval: true,
			Category:         CategoryWorkspaceBoundary,
			Reason:           "requested operation is outside configured workspace roots",
		})
	case ModeDefault, ModeAcceptEdits, ModeAsk, ModeDontAsk:
		fallthrough
	default:
		return p.finalizeDecision(Decision{
			RequiresApproval: true,
			Category:         CategoryApproval,
			Reason:           "policy requires explicit approval",
		})
	}
}

func isShellTool(toolName string) bool {
	switch strings.TrimSpace(toolName) {
	case "system.run", "Bash", "PowerShell":
		return true
	default:
		return false
	}
}

func (d Decision) UpdatedInputValue() (string, bool, error) {
	if d.UpdatedInputObject != nil {
		encoded, err := json.Marshal(d.UpdatedInputObject)
		if err != nil {
			return "", false, err
		}
		return string(encoded), true, nil
	}
	updated := strings.TrimSpace(d.UpdatedInput)
	if updated == "" {
		return "", false, nil
	}
	return updated, true, nil
}

func (d Decision) SerializedDecisionReason() string {
	return d.DecisionReason.Serialize()
}

func (r DecisionReason) Serialize() string {
	switch r.Type {
	case DecisionReasonClassifier, DecisionReasonHook, DecisionReasonAsyncAgent, DecisionReasonSandboxOverride, DecisionReasonWorkingDir, DecisionReasonSafetyCheck, DecisionReasonOther:
		return strings.TrimSpace(r.Reason)
	case DecisionReasonRule, DecisionReasonMode, DecisionReasonSubcommandResults, DecisionReasonPermissionPromptTool:
		return ""
	default:
		return ""
	}
}

func (r DecisionReason) Structured() map[string]any {
	if r.Type == "" {
		return nil
	}
	out := map[string]any{
		"type": string(r.Type),
	}
	switch r.Type {
	case DecisionReasonRule:
		if r.Rule != nil {
			out["rule"] = *r.Rule
		}
	case DecisionReasonMode:
		if r.Mode != "" {
			out["mode"] = string(r.Mode)
		}
	case DecisionReasonSubcommandResults:
		if r.ToolResult != nil {
			out["reasons"] = r.ToolResult
		}
	case DecisionReasonPermissionPromptTool:
		if r.PermissionPromptToolName != "" {
			out["permissionPromptToolName"] = r.PermissionPromptToolName
		}
		if r.ToolResult != nil {
			out["toolResult"] = r.ToolResult
		}
	case DecisionReasonHook:
		if r.HookName != "" {
			out["hookName"] = r.HookName
		}
		if r.HookSource != "" {
			out["hookSource"] = r.HookSource
		}
		if r.Reason != "" {
			out["reason"] = r.Reason
		}
	case DecisionReasonClassifier:
		if r.Classifier != "" {
			out["classifier"] = r.Classifier
		}
		if r.Reason != "" {
			out["reason"] = r.Reason
		}
	case DecisionReasonAsyncAgent, DecisionReasonSandboxOverride, DecisionReasonWorkingDir, DecisionReasonSafetyCheck, DecisionReasonOther:
		if r.Reason != "" {
			out["reason"] = r.Reason
		}
	}
	return out
}

func (p Policy) ApplyPermissionUpdates(updates []PermissionUpdate) Policy {
	updated := p
	for _, update := range updates {
		switch update.Type {
		case PermissionUpdateAddRules:
			rules := permissionUpdateRules(update)
			if len(rules) > 0 {
				updated.Rules = append(append([]Rule{}, rules...), updated.Rules...)
			}
		case PermissionUpdateReplaceRules:
			updated.Rules = append(permissionUpdateRules(update), rulesWithoutBehavior(updated.Rules, update.Behavior)...)
		case PermissionUpdateRemoveRules:
			updated.Rules = removePermissionUpdateRules(updated.Rules, permissionUpdateRules(update))
		case PermissionUpdateSetMode:
			if update.Mode != "" {
				updated.Mode = update.Mode
			}
		case PermissionUpdateAddDirectories:
			updated.WorkspaceRoots = appendUniqueStrings(updated.WorkspaceRoots, update.Directories...)
		case PermissionUpdateRemoveDirectories:
			updated.WorkspaceRoots = removeStrings(updated.WorkspaceRoots, update.Directories...)
		}
	}
	return updated
}

func (p Policy) DeriveForSubagent() Policy {
	derived := p
	derived.SubagentMode = ""
	derived.PlanMode = false
	derived.AutoMode = false
	if p.SubagentMode != "" {
		derived.Mode = p.SubagentMode
		return derived
	}
	derived.Mode = saferMode(p.Mode)
	return derived
}

func (r Rule) matches(req Request) bool {
	if r.ToolName != "" && !ToolNameMatchesRule(r.ToolName, req.ToolName) {
		return false
	}
	if len(r.Match.CommandContains) > 0 {
		matched := false
		for _, part := range r.Match.CommandContains {
			if strings.Contains(strings.ToLower(req.Command), strings.ToLower(part)) {
				matched = true
				break
			}
		}
		if !matched {
			return false
		}
	}
	if len(r.Match.WorkDirPrefixes) > 0 && !insideAnyRoot(req.WorkDir, r.Match.WorkDirPrefixes) {
		return false
	}
	return true
}

func permissionUpdateRules(update PermissionUpdate) []Rule {
	if update.Behavior == "" {
		return nil
	}
	rules := make([]Rule, 0, len(update.Rules))
	for _, value := range update.Rules {
		toolName := strings.TrimSpace(value.ToolName)
		if toolName == "" {
			continue
		}
		rule := Rule{
			ToolName: toolName,
			Action:   update.Behavior,
			Source:   string(permissionUpdateRuleSource(update.Destination)),
		}
		if content := strings.TrimSpace(value.RuleContent); content != "" {
			rule.Match.CommandContains = []string{content}
		}
		rules = append(rules, rule)
	}
	return rules
}

func permissionUpdateRuleSource(destination PermissionUpdateDestination) RuleSource {
	switch destination {
	case PermissionUpdateDestinationUserSettings:
		return RuleSourceConfig
	case PermissionUpdateDestinationProjectSettings:
		return RuleSourceProject
	case PermissionUpdateDestinationLocalSettings:
		return RuleSourceLocal
	case PermissionUpdateDestinationSession:
		return RuleSourceSession
	case PermissionUpdateDestinationCLIArg:
		return RuleSourceCLIArg
	default:
		return RuleSourceSession
	}
}

func rulesWithoutBehavior(rules []Rule, behavior Action) []Rule {
	if behavior == "" {
		return append([]Rule(nil), rules...)
	}
	out := make([]Rule, 0, len(rules))
	for _, rule := range rules {
		if rule.Action != behavior {
			out = append(out, rule)
		}
	}
	return out
}

func removePermissionUpdateRules(existing, remove []Rule) []Rule {
	out := make([]Rule, 0, len(existing))
	for _, rule := range existing {
		if containsEquivalentRule(remove, rule) {
			continue
		}
		out = append(out, rule)
	}
	return out
}

func containsEquivalentRule(rules []Rule, candidate Rule) bool {
	for _, rule := range rules {
		if rule.Action != candidate.Action || rule.ToolName != candidate.ToolName {
			continue
		}
		if !sameStrings(rule.Match.CommandContains, candidate.Match.CommandContains) {
			continue
		}
		if !sameStrings(rule.Match.WorkDirPrefixes, candidate.Match.WorkDirPrefixes) {
			continue
		}
		return true
	}
	return false
}

func appendUniqueStrings(values []string, additions ...string) []string {
	out := append([]string(nil), values...)
	seen := make(map[string]struct{}, len(out))
	for _, value := range out {
		seen[value] = struct{}{}
	}
	for _, value := range additions {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		out = append(out, value)
	}
	return out
}

func removeStrings(values []string, removals ...string) []string {
	remove := make(map[string]struct{}, len(removals))
	for _, value := range removals {
		value = strings.TrimSpace(value)
		if value != "" {
			remove[value] = struct{}{}
		}
	}
	out := make([]string, 0, len(values))
	for _, value := range values {
		if _, ok := remove[value]; ok {
			continue
		}
		out = append(out, value)
	}
	return out
}

func sameStrings(left, right []string) bool {
	if len(left) != len(right) {
		return false
	}
	for idx := range left {
		if left[idx] != right[idx] {
			return false
		}
	}
	return true
}

func ToolNameMatchesRule(ruleToolName, actualToolName string) bool {
	ruleToolName = strings.TrimSpace(ruleToolName)
	actualToolName = strings.TrimSpace(actualToolName)
	if ruleToolName == "" {
		return true
	}
	if ruleToolName == actualToolName {
		return true
	}
	if strings.HasSuffix(ruleToolName, "__*") {
		serverPrefix := strings.TrimSuffix(ruleToolName, "__*")
		if serverPrefix != "" && strings.HasPrefix(actualToolName, serverPrefix+"__") {
			return true
		}
	}
	return strings.HasPrefix(actualToolName, ruleToolName+"__")
}

func insideAnyRoot(workDir string, roots []string) bool {
	normalizedDir := normalizePath(workDir)
	if normalizedDir == "" {
		return false
	}
	for _, root := range roots {
		normalizedRoot := normalizePath(root)
		if normalizedRoot == "" {
			continue
		}
		if normalizedDir == normalizedRoot || strings.HasPrefix(normalizedDir, normalizedRoot+"/") {
			return true
		}
	}
	return false
}

func normalizePath(value string) string {
	value = strings.TrimSpace(strings.ReplaceAll(value, "\\", "/"))
	value = strings.TrimSuffix(value, "/")
	return value
}

func saferMode(mode Mode) Mode {
	switch mode {
	case ModeBypassPermissions, ModeDangerFullAccess:
		return ModeWorkspaceWrite
	case ModeAuto, ModeAcceptEdits, ModeWorkspaceWrite:
		return ModeAsk
	case ModeDefault, ModePlan, ModeDontAsk, ModeAsk:
		fallthrough
	default:
		return ModeAsk
	}
}

func (p Policy) finalizeDecision(decision Decision) Decision {
	if decision.DecisionReason.Type == "" {
		decision.DecisionReason = DecisionReason{Type: DecisionReasonMode, Mode: p.Mode}
	}
	if p.Mode == ModeDontAsk && decision.RequiresApproval {
		decision.RequiresApproval = false
		decision.Allowed = false
		if strings.TrimSpace(decision.Reason) == "" {
			decision.Reason = "don't ask mode denied approval-required action"
		}
	}
	return decision
}

func matchesAny(input string, patterns []string) bool {
	lower := strings.ToLower(input)
	for _, pattern := range patterns {
		if strings.Contains(lower, strings.ToLower(pattern)) {
			return true
		}
	}
	return false
}
