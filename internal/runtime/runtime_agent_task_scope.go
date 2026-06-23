package runtime

import (
	"context"
	"path/filepath"
	"strings"

	"github.com/CIPFZ/agent-builder/internal/agent"
)

func (r *runtimeService) agentTaskForChildSession(ctx context.Context, sessionID string) (RuntimeAgentTask, bool) {
	sessionID = strings.TrimSpace(sessionID)
	if sessionID == "" || r.agentTasks.db == nil {
		return RuntimeAgentTask{}, false
	}
	tasks, err := r.agentTasks.ListByChildSession(ctx, sessionID)
	if err != nil || len(tasks) == 0 {
		return RuntimeAgentTask{}, false
	}
	return tasks[len(tasks)-1], true
}

func (r *runtimeService) agentTaskScopeViolation(task RuntimeAgentTask, call agent.SchedulerToolCall) string {
	if isFinalAgentTaskStatus(task.Status) {
		return "agent task is no longer active"
	}
	if len(task.AllowedTools) > 0 && !matchesRuntimeScopeValue(task.AllowedTools, call.Name, call.CapabilityID) {
		return "agent task scope denied tool " + firstNonEmpty(call.Name, call.CapabilityID)
	}
	if len(task.CapabilityScope) > 0 && !capabilityAllowedByTaskScope(task.CapabilityScope, call) {
		return "agent task scope denied capability " + firstNonEmpty(call.CapabilityID, call.Source+":"+call.Name)
	}
	if task.CWD != "" {
		target := extractCWDFromToolInput(call.InputSummary)
		if target != "" && !pathInsideScope(task.CWD, target) {
			return "agent task scope denied cwd " + target
		}
	}
	if task.Worktree != "" {
		target := extractCWDFromToolInput(call.InputSummary)
		if target != "" && !pathInsideScope(task.Worktree, target) {
			return "agent task scope denied worktree " + target
		}
	}
	return ""
}

func matchesRuntimeScopeValue(allowed []string, values ...string) bool {
	for _, allow := range allowed {
		allow = strings.ToLower(strings.TrimSpace(allow))
		if allow == "" {
			continue
		}
		for _, value := range values {
			value = strings.ToLower(strings.TrimSpace(value))
			if value == "" {
				continue
			}
			if allow == "*" || allow == value {
				return true
			}
			if strings.HasSuffix(allow, "*") && strings.HasPrefix(value, strings.TrimSuffix(allow, "*")) {
				return true
			}
			if strings.Contains(value, ":"+allow) {
				return true
			}
		}
	}
	return false
}

func capabilityAllowedByTaskScope(scope []string, call agent.SchedulerToolCall) bool {
	if matchesRuntimeScopeValue(scope, call.CapabilityID, call.Source, call.Name) {
		return true
	}
	risk := strings.ToLower(call.Risk)
	for _, item := range scope {
		if strings.EqualFold(item, risk) {
			return true
		}
	}
	source := strings.ToLower(strings.TrimSpace(call.Source))
	switch source {
	case "builtin":
		if matchesRuntimeScopeValue(scope, "read") && isReadOnlyTaskTool(call.Name) {
			return true
		}
		return matchesRuntimeScopeValue(scope, "builtin", "tool")
	case "shell":
		return matchesRuntimeScopeValue(scope, "shell", "execute")
	case "mcp":
		return matchesRuntimeScopeValue(scope, "mcp", "network")
	}
	return false
}

func isReadOnlyTaskTool(name string) bool {
	switch strings.ToLower(strings.TrimSpace(name)) {
	case "view", "grep", "glob", "ls", "diagnostics", "references", "agent_builder_info", "agent_builder_logs", "job_output", "list_mcp_resources", "read_mcp_resource", "task_list", "task_get", "task_output":
		return true
	default:
		return false
	}
}

func extractCWDFromToolInput(input string) string {
	for _, key := range []string{`"effective_cwd"`, `"cwd"`, `"working_dir"`, `"workdir"`} {
		idx := strings.Index(input, key)
		if idx < 0 {
			continue
		}
		rest := input[idx+len(key):]
		colon := strings.Index(rest, ":")
		if colon < 0 {
			continue
		}
		rest = strings.TrimSpace(rest[colon+1:])
		if !strings.HasPrefix(rest, `"`) {
			continue
		}
		rest = rest[1:]
		end := strings.Index(rest, `"`)
		if end > 0 {
			return rest[:end]
		}
	}
	return ""
}

func pathInsideScope(scope, target string) bool {
	scopeAbs, err := filepath.Abs(scope)
	if err != nil {
		scopeAbs = scope
	}
	targetAbs, err := filepath.Abs(target)
	if err != nil {
		targetAbs = target
	}
	scopeClean := strings.ToLower(filepath.Clean(scopeAbs))
	targetClean := strings.ToLower(filepath.Clean(targetAbs))
	return targetClean == scopeClean || strings.HasPrefix(targetClean, scopeClean+string(filepath.Separator))
}
