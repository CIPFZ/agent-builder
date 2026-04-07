package permissions

import "strings"

type Mode string

const (
	ModeAsk              Mode = "ask"
	ModeWorkspaceWrite   Mode = "workspace-write"
	ModeDangerFullAccess Mode = "danger-full-access"
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
}

type Request struct {
	ToolName    string
	Command     string
	WorkDir     string
	ReadOnly    bool
	Destructive bool
}

type Action string

const (
	ActionAllow Action = "allow"
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
	RuleSourceSession RuleSource = "session"
)

type RuleLayer struct {
	Source RuleSource
	Rules  []Rule
}

type Decision struct {
	Allowed          bool
	RequiresApproval bool
	Category         Category
	RuleSource       string
	Reason           string
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
			return Decision{Allowed: true, RuleSource: rule.Source, Reason: "allowed by rule"}
		case ActionDeny:
			return Decision{Category: CategoryRuleDenied, RuleSource: rule.Source, Reason: "denied by rule"}
		}
	}
	if req.ToolName != "system.run" {
		if req.Destructive {
			if p.PlanMode {
				return Decision{
					RequiresApproval: true,
					Category:         CategoryPlanMode,
					Reason:           "plan mode requires approval for destructive tool actions",
				}
			}
			switch p.Mode {
			case ModeDangerFullAccess, ModeWorkspaceWrite:
				return Decision{Allowed: true, Reason: "destructive non-system tool allowed by mode"}
			case ModeAsk:
				fallthrough
			default:
				return Decision{
					RequiresApproval: true,
					Category:         CategoryDestructiveTool,
					Reason:           "destructive tool action requires explicit approval",
				}
			}
		}
		return Decision{Allowed: true, Reason: "non-system tool allowed by default"}
	}
	if p.PlanMode {
		return Decision{
			RequiresApproval: true,
			Category:         CategoryPlanMode,
			Reason:           "plan mode requires approval for system actions",
		}
	}
	if matchesAny(req.Command, p.DangerousCommandPatterns) {
		return Decision{
			RequiresApproval: true,
			Category:         CategoryDangerousCommand,
			Reason:           "dangerous command requires explicit approval",
		}
	}

	switch p.Mode {
	case ModeDangerFullAccess:
		return Decision{Allowed: true}
	case ModeWorkspaceWrite:
		if insideAnyRoot(req.WorkDir, p.WorkspaceRoots) {
			return Decision{Allowed: true}
		}
		return Decision{
			RequiresApproval: true,
			Category:         CategoryWorkspaceBoundary,
			Reason:           "requested operation is outside configured workspace roots",
		}
	case ModeAsk:
		fallthrough
	default:
		return Decision{
			RequiresApproval: true,
			Category:         CategoryApproval,
			Reason:           "policy requires explicit approval",
		}
	}
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
	if r.ToolName != "" && r.ToolName != req.ToolName {
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
	case ModeDangerFullAccess:
		return ModeWorkspaceWrite
	case ModeWorkspaceWrite:
		return ModeAsk
	case ModeAsk:
		fallthrough
	default:
		return ModeAsk
	}
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
