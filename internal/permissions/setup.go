package permissions

import (
	"fmt"
	"sort"
	"strings"
)

func SetupPolicy(policy Policy) (Policy, error) {
	policy.Mode = normalizeMode(policy.Mode)
	policy.SubagentMode = normalizeOptionalMode(policy.SubagentMode)
	if !isKnownMode(policy.Mode) {
		return Policy{}, fmt.Errorf("unknown permission mode %q", policy.Mode)
	}
	if policy.SubagentMode != "" && !isKnownMode(policy.SubagentMode) {
		return Policy{}, fmt.Errorf("unknown subagent permission mode %q", policy.SubagentMode)
	}

	policy.WorkspaceRoots = dedupeNormalized(policy.WorkspaceRoots)
	policy.WorkspaceRoots = collapseNestedRoots(policy.WorkspaceRoots)
	policy.DangerousCommandPatterns = dedupeNormalized(policy.DangerousCommandPatterns)

	for idx := range policy.Rules {
		normalized, err := normalizeRule(policy.Rules[idx], policy.WorkspaceRoots)
		if err != nil {
			return Policy{}, err
		}
		policy.Rules[idx] = normalized
	}
	layers, err := normalizeRuleLayers(policy.RuleLayers, policy.WorkspaceRoots)
	if err != nil {
		return Policy{}, err
	}
	policy.RuleLayers = layers
	policy.Rules = mergeRuleLayers(policy.Rules, policy.RuleLayers)

	if policy.Mode == ModePlan {
		policy.PlanMode = true
	}
	if policy.Mode == ModeAuto {
		policy.AutoMode = true
	}
	if (policy.Mode == ModeWorkspaceWrite || policy.Mode == ModeAuto) && len(policy.WorkspaceRoots) == 0 {
		return Policy{}, fmt.Errorf("workspace-write mode requires at least one workspace root")
	}
	if policy.PlanMode && policy.AutoMode {
		return Policy{}, fmt.Errorf("plan mode and auto mode cannot be enabled together")
	}
	if policy.AutoMode && policy.Mode == ModeAsk {
		return Policy{}, fmt.Errorf("auto mode cannot run with ask permission mode")
	}
	if policy.SubagentMode != "" && modeRank(policy.SubagentMode) > modeRank(policy.Mode) {
		return Policy{}, fmt.Errorf("subagent mode %q cannot be more permissive than parent mode %q", policy.SubagentMode, policy.Mode)
	}
	if policy.AutoMode {
		for _, rule := range policy.Rules {
			if rule.Action != ActionAllow {
				continue
			}
			if isDangerousAutoModeRule(rule) {
				return Policy{}, fmt.Errorf("auto mode cannot use dangerous allow rule for %q", rule.ToolName)
			}
		}
	}

	return policy, nil
}

func normalizeMode(mode Mode) Mode {
	switch strings.TrimSpace(string(mode)) {
	case "":
		return ModeDefault
	case "bypass-permissions":
		return ModeBypassPermissions
	case "dont-ask":
		return ModeDontAsk
	default:
		return Mode(strings.TrimSpace(string(mode)))
	}
}

func normalizeOptionalMode(mode Mode) Mode {
	if strings.TrimSpace(string(mode)) == "" {
		return ""
	}
	return normalizeMode(mode)
}

func isKnownMode(mode Mode) bool {
	switch mode {
	case ModeDefault, ModeAsk, ModeAcceptEdits, ModePlan, ModeAuto, ModeWorkspaceWrite, ModeDangerFullAccess, ModeBypassPermissions, ModeDontAsk:
		return true
	default:
		return false
	}
}

func modeRank(mode Mode) int {
	switch mode {
	case ModeDefault, ModeAsk, ModePlan, ModeDontAsk:
		return 1
	case ModeAuto, ModeAcceptEdits, ModeWorkspaceWrite:
		return 2
	case ModeDangerFullAccess:
		return 3
	case ModeBypassPermissions:
		return 4
	default:
		return 0
	}
}

func dedupeNormalized(values []string) []string {
	seen := make(map[string]struct{}, len(values))
	out := make([]string, 0, len(values))
	for _, value := range values {
		value = normalizePath(value)
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

func collapseNestedRoots(roots []string) []string {
	out := make([]string, 0, len(roots))
	for _, root := range roots {
		nested := false
		for _, existing := range out {
			if root == existing || strings.HasPrefix(root, existing+"/") {
				nested = true
				break
			}
		}
		if nested {
			continue
		}
		filtered := out[:0]
		for _, existing := range out {
			if existing != root && !strings.HasPrefix(existing, root+"/") {
				filtered = append(filtered, existing)
			}
		}
		out = append(filtered, root)
	}
	return out
}

func normalizeRule(rule Rule, workspaceRoots []string) (Rule, error) {
	rule.ToolName = strings.TrimSpace(rule.ToolName)
	rule.Source = strings.TrimSpace(rule.Source)
	rule.Match.CommandContains = dedupeNormalized(rule.Match.CommandContains)
	rule.Match.WorkDirPrefixes = dedupeNormalized(rule.Match.WorkDirPrefixes)
	if len(workspaceRoots) > 0 && len(rule.Match.WorkDirPrefixes) > 0 {
		for _, prefix := range rule.Match.WorkDirPrefixes {
			if !insideAnyRoot(prefix, workspaceRoots) {
				return Rule{}, fmt.Errorf("rule workdir prefix %q is outside configured workspace roots", prefix)
			}
		}
	}
	return rule, nil
}

func normalizeRuleLayers(layers []RuleLayer, workspaceRoots []string) ([]RuleLayer, error) {
	if len(layers) == 0 {
		return nil, nil
	}
	out := make([]RuleLayer, 0, len(layers))
	for _, layer := range layers {
		normalized := RuleLayer{Source: layer.Source}
		for _, rule := range layer.Rules {
			rule.Source = string(layer.Source)
			cleanRule, err := normalizeRule(rule, workspaceRoots)
			if err != nil {
				return nil, err
			}
			normalized.Rules = append(normalized.Rules, cleanRule)
		}
		out = append(out, normalized)
	}
	sort.SliceStable(out, func(i, j int) bool {
		return ruleSourceRank(out[i].Source) > ruleSourceRank(out[j].Source)
	})
	return out, nil
}

func mergeRuleLayers(inline []Rule, layers []RuleLayer) []Rule {
	if len(layers) == 0 {
		return inline
	}
	out := make([]Rule, 0, len(inline)+len(layers))
	out = append(out, inline...)
	for _, layer := range layers {
		out = append(out, layer.Rules...)
	}
	return out
}

func ruleSourceRank(source RuleSource) int {
	switch source {
	case RuleSourceSession:
		return 8
	case RuleSourceCommand:
		return 7
	case RuleSourceCLIArg:
		return 6
	case RuleSourcePolicy:
		return 5
	case RuleSourceFlag:
		return 4
	case RuleSourceLocal:
		return 3
	case RuleSourceProject:
		return 2
	case RuleSourceConfig:
		return 1
	case RuleSourceSystem:
		return 0
	default:
		return -1
	}
}

func isDangerousAutoModeRule(rule Rule) bool {
	switch rule.ToolName {
	case "agent.task", "Agent", "Task":
		return true
	case "system.run", "Bash", "PowerShell":
		if len(rule.Match.CommandContains) == 0 && len(rule.Match.WorkDirPrefixes) == 0 {
			return true
		}
		for _, pattern := range rule.Match.CommandContains {
			if isDangerousCommandPattern(pattern) {
				return true
			}
		}
		return false
	default:
		return false
	}
}

func isDangerousCommandPattern(pattern string) bool {
	pattern = strings.ToLower(strings.TrimSpace(pattern))
	if pattern == "" {
		return false
	}
	dangerous := []string{
		"python -c",
		"python3 -c",
		"node -e",
		"powershell -enc",
		"pwsh -enc",
		"cmd /c",
		"bash -c",
		"sh -c",
		"invoke-expression",
		"iex",
	}
	for _, marker := range dangerous {
		if strings.Contains(pattern, marker) {
			return true
		}
	}
	return false
}
