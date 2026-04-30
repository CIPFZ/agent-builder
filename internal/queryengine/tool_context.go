package queryengine

import (
	"myclaw/internal/permissions"
	"myclaw/internal/session"
	"myclaw/internal/tools"
	"myclaw/internal/workspace"
	"strings"
)

func (q *QueryEngine) applyToolContextModifier(sessionID string, current tools.ToolUseContext, modifier func(tools.ToolUseContext) tools.ToolUseContext) {
	if modifier == nil {
		return
	}
	next := modifier(current)
	q.toolContextMu.Lock()
	defer q.toolContextMu.Unlock()
	if next.AppState != nil {
		q.toolAppStates[sessionID] = cloneAnyMap(next.AppState)
	}
	if next.ToolDecisions != nil {
		q.toolDecisions[sessionID] = cloneToolDecisions(next.ToolDecisions)
	}
	q.applySkillContextOverrides(sessionID, current, next)
}

func (q *QueryEngine) applySkillContextOverrides(sessionID string, current, next tools.ToolUseContext) {
	appState := next.AppState
	if appState == nil {
		return
	}
	if allowed := stringListFromAny(appState["skillAllowedTools"]); len(allowed) > 0 {
		policy := q.PermissionPolicyForSession(sessionID)
		policy.Rules = append(skillAllowedToolRules(allowed), policy.Rules...)
		q.SetSessionPermissionPolicy(sessionID, policy)
	}
	model := strings.TrimSpace(next.MainLoopModel)
	if model == "" {
		model = stringMapField(appState, "skillModel")
	}
	effort := stringMapField(appState, "skillEffort")
	if model == "" && effort == "" {
		return
	}
	_ = q.sessions.UpdateMetadata(sessionID, func(metadata *session.SessionMetadata) {
		if model != "" {
			if strings.TrimSpace(metadata.InitialMainLoopModel) == "" {
				metadata.InitialMainLoopModel = parseUserSpecifiedMainLoopModel(current.MainLoopModel, q.llmProvider)
			}
			metadata.MainLoopModelOverride = model
		}
		if effort != "" {
			metadata.MainLoopEffortOverride = effort
		}
	})
}

func skillAllowedToolRules(toolNames []string) []permissions.Rule {
	rules := make([]permissions.Rule, 0, len(toolNames))
	seen := make(map[string]struct{}, len(toolNames))
	for _, toolName := range toolNames {
		toolName = strings.TrimSpace(toolName)
		if toolName == "" {
			continue
		}
		key := strings.ToLower(toolName)
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		rules = append(rules, permissions.Rule{
			ToolName: toolName,
			Source:   string(permissions.RuleSourceCommand),
			Action:   permissions.ActionAllow,
		})
	}
	return rules
}

func (q *QueryEngine) WorkspaceLoader() *workspace.Loader {
	return q.workspace
}

func (q *QueryEngine) exposedTools(sessionID string, includeDeferred bool) []tools.Definition {
	exposed := q.tools.Expose(tools.ExposeOptions{
		IncludeDeferred: includeDeferred,
		Policy:          q.PermissionPolicyForSession(sessionID),
	})
	if len(q.mcpTools) == 0 && len(q.mcpNeedsAuth) == 0 {
		return exposed
	}
	out := make([]tools.Definition, 0, len(exposed))
	for _, def := range exposed {
		if !q.isCurrentMCPToolDefinition(def.Name) {
			continue
		}
		out = append(out, def)
	}
	return out
}

func (q *QueryEngine) isCurrentMCPToolDefinition(name string) bool {
	name = strings.TrimSpace(name)
	if !strings.HasPrefix(name, "mcp__") {
		return true
	}
	for server := range q.mcpNeedsAuth {
		authName := tools.BuildMCPToolName(server, "authenticate")
		if authName == name {
			return true
		}
		if strings.HasPrefix(name, tools.BuildMCPToolName(server, "")) {
			return false
		}
	}
	for configuredServer, result := range q.mcpTools {
		for _, item := range result.Tools {
			if tools.BuildMCPToolName(configuredServer, item.Name) == name {
				return true
			}
		}
		if strings.HasPrefix(name, tools.BuildMCPToolName(configuredServer, "")) {
			return false
		}
	}
	return false
}

func (q *QueryEngine) SetSessionMainLoopModelOverride(sessionID, model string) error {
	return q.sessions.UpdateMetadata(sessionID, func(metadata *session.SessionMetadata) {
		if strings.TrimSpace(metadata.InitialMainLoopModel) == "" {
			metadata.InitialMainLoopModel = parseUserSpecifiedMainLoopModel(q.mainLoopModel, q.llmProvider)
		}
		metadata.MainLoopModelOverride = strings.TrimSpace(model)
	})
}

func (q *QueryEngine) ClearSessionMainLoopModelOverride(sessionID string) error {
	return q.sessions.UpdateMetadata(sessionID, func(metadata *session.SessionMetadata) {
		metadata.MainLoopModelOverride = ""
	})
}

func (q *QueryEngine) BaseMainLoopModelForSession(sessionID string) string {
	q.ensureSessionModelMetadata(sessionID)
	sess, ok := q.sessions.GetByID(sessionID)
	if ok {
		if initial := strings.TrimSpace(sess.Metadata.InitialMainLoopModel); initial != "" {
			return initial
		}
	}
	return parseUserSpecifiedMainLoopModel(q.mainLoopModel, q.llmProvider)
}

func (q *QueryEngine) SessionMainLoopModelOverride(sessionID string) string {
	sess, ok := q.sessions.GetByID(sessionID)
	if !ok {
		return ""
	}
	return strings.TrimSpace(sess.Metadata.MainLoopModelOverride)
}

func (q *QueryEngine) ResolvedMainLoopModelForSession(sessionID string) string {
	return q.mainLoopModelForSession(sessionID)
}

func (q *QueryEngine) ToolContractsForSession(sessionID string) []tools.Contract {
	if q == nil || q.tools == nil {
		return nil
	}
	return q.tools.Contracts(tools.ContractOptions{Policy: q.PermissionPolicyForSession(sessionID), IncludeDeferred: true})
}
