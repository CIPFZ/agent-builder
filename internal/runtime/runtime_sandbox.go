package runtime

import (
	"context"
	"encoding/json"
	"fmt"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/charmbracelet/crush/internal/agent"
	"github.com/charmbracelet/crush/internal/agent/tools"
	"github.com/charmbracelet/crush/internal/permission"
	"github.com/charmbracelet/crush/internal/runtimeapi"
	"github.com/charmbracelet/crush/internal/tools/scheduler"
)

func (r *runtimeSchedulerRecorder) evaluateSandboxDecision(ctx context.Context, call agent.SchedulerToolCall, policyResult permission.PolicyResult) (RuntimeSandboxDecision, bool, error) {
	if r == nil || r.service == nil {
		return RuntimeSandboxDecision{}, false, nil
	}
	if !sandboxDecisionApplies(call) {
		return RuntimeSandboxDecision{}, false, nil
	}
	store, err := r.service.ensureSandboxDecisionStore(ctx)
	if err != nil {
		return RuntimeSandboxDecision{}, false, err
	}
	decision := r.service.buildSandboxDecision(ctx, call, policyResult)
	decision, err = store.Upsert(ctx, decision)
	if err != nil {
		return RuntimeSandboxDecision{}, false, err
	}
	if r.service != nil && r.service.toolCalls != nil && call.ID != "" {
		_, _ = r.service.toolCalls.CompleteCall(ctx, scheduler.ToolCallResult{
			ToolCallID:           call.ID,
			Status:               scheduler.ToolCallRunning,
			JobID:                call.JobID,
			Command:              call.Command,
			Risk:                 call.Risk,
			PolicyReason:         call.PolicyReason,
			PolicyMode:           call.PolicyMode,
			PolicyProfile:        call.PolicyProfile,
			PolicyRuleID:         call.PolicyRuleID,
			PolicyRuleSource:     call.PolicyRuleSource,
			PolicyScopeKind:      call.PolicyScopeKind,
			PolicyScopeValue:     call.PolicyScopeValue,
			PolicyTargetSummary:  call.PolicyTargetSummary,
			ShellRisk:            call.ShellRisk,
			ShellReason:          call.ShellReason,
			SandboxDecisionID:    decision.ID,
			SandboxMode:          decision.Mode,
			SandboxStatus:        decision.Status,
			SandboxExecutor:      decision.Executor,
			SandboxReason:        decision.Reason,
			SandboxError:         decision.Error,
			PolicyHeadless:       call.PolicyHeadless,
			PolicyHeadlessReason: call.PolicyHeadlessReason,
			JobStatus:            call.JobStatus,
			JobStartedAt:         timeFromMillis(call.JobStartedAt),
		})
	}
	r.service.recordSandboxDecision(ctx, decision)
	if decision.Status == sandboxStatusDenied || decision.Status == sandboxStatusFailed {
		return decision, true, nil
	}
	return decision, false, nil
}

func sandboxDecisionApplies(call agent.SchedulerToolCall) bool {
	source := strings.ToLower(strings.TrimSpace(call.Source))
	name := strings.ToLower(strings.TrimSpace(call.Name))
	if source == "shell" {
		return true
	}
	switch name {
	case "bash", "shell", "execute", "job_output", "job_kill":
		return true
	default:
		return false
	}
}

func (r *runtimeService) buildSandboxDecision(ctx context.Context, call agent.SchedulerToolCall, policyResult permission.PolicyResult) RuntimeSandboxDecision {
	now := time.Now().UTC().UnixMilli()
	command := firstNonEmpty(call.Command, shellCommandFromInputSummary(call.InputSummary))
	task, _ := r.agentTaskForChildSession(ctx, call.SessionID)
	cwd, worktreeID, worktreePath, pathErr := r.sandboxEffectiveScope(ctx, call, task)
	mode := sandboxModeNone
	status := sandboxStatusNotRequired
	executor := sandboxExecutorNone
	reason := "Sandbox is not required for read-only shell metadata access."
	errText := ""
	networkAllowed := false
	networkReason := "network denied unless a future sandbox executor can enforce it"
	allowedPaths := []string{}
	if cwd != "" {
		allowedPaths = append(allowedPaths, cwd)
	}
	if worktreePath != "" && worktreePath != cwd {
		allowedPaths = append(allowedPaths, worktreePath)
	}
	if pathErr != "" {
		mode = sandboxModeRequired
		status = sandboxStatusDenied
		executor = sandboxExecutorUnavailableBoundary
		reason = "Sandbox path validation denied command scope."
		errText = pathErr
	} else if sandboxRequiresIsolation(call, policyResult) {
		mode = sandboxModeRequired
		executor = sandboxExecutorUnavailableBoundary
		if sandboxOSIsolationAvailable() {
			status = sandboxStatusApplied
			reason = "Sandbox executor selected for shell command."
		} else if sandboxCanUseNoSandboxFallback(policyResult) {
			status = sandboxStatusUnavailable
			reason = "No OS sandbox executor is available; deterministic policy explicitly allowed no-sandbox execution."
			errText = "os sandbox unavailable"
		} else {
			status = sandboxStatusDenied
			reason = "No OS sandbox executor is available for a high-risk shell command."
			errText = "sandbox unavailable; fail closed"
		}
	} else if strings.EqualFold(call.Name, "job_kill") {
		mode = sandboxModeRequired
		status = sandboxStatusDenied
		executor = sandboxExecutorUnavailableBoundary
		reason = "Background job termination is destructive and requires a sandbox-capable executor or explicit allow."
		errText = "sandbox unavailable; fail closed"
	}
	return RuntimeSandboxDecision{
		ID:             newRuntimeSandboxDecisionID(),
		SessionID:      call.SessionID,
		TurnID:         call.TurnID,
		ToolCallID:     call.ID,
		TaskID:         task.ID,
		Mode:           mode,
		Status:         status,
		Executor:       executor,
		CWD:            cwd,
		WorktreeID:     worktreeID,
		WorktreePath:   worktreePath,
		CommandSummary: command,
		PolicyMode:     string(policyResult.Mode),
		PolicyProfile:  policyResult.Profile,
		PolicyRule:     policyResult.RuleID,
		Reason:         reason,
		Error:          errText,
		AllowedPaths:   allowedPaths,
		NetworkAllowed: networkAllowed,
		NetworkReason:  networkReason,
		CreatedAt:      now,
	}
}

func sandboxRequiresIsolation(call agent.SchedulerToolCall, result permission.PolicyResult) bool {
	if result.Risk == permission.RiskDestructive || result.Risk == permission.RiskNetwork || result.Risk == permission.RiskSecret {
		return true
	}
	if strings.EqualFold(call.Name, "job_kill") {
		return true
	}
	if !strings.EqualFold(call.Name, "job_output") && strings.EqualFold(call.Source, "shell") {
		return result.Risk == permission.RiskExecute || result.Risk == permission.RiskWrite
	}
	return false
}

func sandboxCanUseNoSandboxFallback(result permission.PolicyResult) bool {
	if result.Decision != permission.PolicyAllow {
		return false
	}
	if result.Headless {
		return false
	}
	switch result.Risk {
	case permission.RiskDestructive, permission.RiskNetwork, permission.RiskSecret:
		return false
	default:
		return true
	}
}

func sandboxOSIsolationAvailable() bool {
	return false
}

func (r *runtimeService) sandboxEffectiveScope(ctx context.Context, call agent.SchedulerToolCall, task RuntimeAgentTask) (string, string, string, string) {
	requested := extractCWDFromToolInput(call.InputSummary)
	base := ""
	r.mu.Lock()
	if r.workspace != nil {
		base = r.workspace.Path
	}
	r.mu.Unlock()
	cwd := firstNonEmpty(requested, tools.GetEffectiveCWDFromContext(ctx), base)
	worktreePath := tools.GetWorktreePathFromContext(ctx)
	worktreeID := ""
	if task.ID != "" {
		scope := r.effectiveScopeForTask(ctx, task)
		cwd = firstNonEmpty(scope.EffectiveCWD, scope.BaseCWD, cwd)
		worktreeID = scope.WorktreeID
		worktreePath = firstNonEmpty(scope.WorktreePath, worktreePath)
		if requested != "" && !pathInsideScope(cwd, requested) {
			return cwd, worktreeID, worktreePath, "requested cwd escapes AgentTask effective scope"
		}
		if task.Worktree != "" && requested != "" && !pathInsideScope(task.Worktree, requested) {
			return cwd, worktreeID, worktreePath, "requested cwd escapes AgentTask worktree scope"
		}
	}
	if strings.TrimSpace(cwd) != "" {
		abs, err := filepath.Abs(cwd)
		if err != nil {
			return cwd, worktreeID, worktreePath, err.Error()
		}
		cwd = filepath.Clean(abs)
	}
	if strings.TrimSpace(worktreePath) != "" {
		abs, err := filepath.Abs(worktreePath)
		if err != nil {
			return cwd, worktreeID, worktreePath, err.Error()
		}
		worktreePath = filepath.Clean(abs)
	}
	return cwd, worktreeID, worktreePath, ""
}

func (r *runtimeService) recordSandboxDecision(ctx context.Context, decision RuntimeSandboxDecision) {
	payload := runtimeSandboxDecisionPayload(decision)
	r.storeRuntimeEvent(runtimeapi.Event{
		ID:         newRuntimeEventID(),
		Type:       runtimeapi.EventSandboxDecisionRecorded,
		CreatedAt:  time.Now().UTC().Format(time.RFC3339Nano),
		SessionID:  decision.SessionID,
		TurnID:     decision.TurnID,
		ToolCallID: decision.ToolCallID,
		Payload:    payload,
	})
	switch decision.Status {
	case sandboxStatusApplied:
		r.storeRuntimeEvent(runtimeapi.Event{ID: newRuntimeEventID(), Type: runtimeapi.EventSandboxApplied, CreatedAt: time.Now().UTC().Format(time.RFC3339Nano), SessionID: decision.SessionID, TurnID: decision.TurnID, ToolCallID: decision.ToolCallID, Payload: payload})
	case sandboxStatusUnavailable:
		r.storeRuntimeEvent(runtimeapi.Event{ID: newRuntimeEventID(), Type: runtimeapi.EventSandboxUnavailable, CreatedAt: time.Now().UTC().Format(time.RFC3339Nano), SessionID: decision.SessionID, TurnID: decision.TurnID, ToolCallID: decision.ToolCallID, Payload: payload})
	case sandboxStatusDenied:
		r.storeRuntimeEvent(runtimeapi.Event{ID: newRuntimeEventID(), Type: runtimeapi.EventSandboxDenied, CreatedAt: time.Now().UTC().Format(time.RFC3339Nano), SessionID: decision.SessionID, TurnID: decision.TurnID, ToolCallID: decision.ToolCallID, Payload: payload})
	case sandboxStatusFailed:
		r.storeRuntimeEvent(runtimeapi.Event{ID: newRuntimeEventID(), Type: runtimeapi.EventSandboxFailed, CreatedAt: time.Now().UTC().Format(time.RFC3339Nano), SessionID: decision.SessionID, TurnID: decision.TurnID, ToolCallID: decision.ToolCallID, Payload: payload})
	}
	if r.canWriteWorktreeAudit() {
		r.writeAudit(auditEntry{
			RequestID:        decision.TurnID,
			Event:            "sandbox_decision_recorded",
			Timestamp:        time.Now().UTC().Format(time.RFC3339Nano),
			SessionID:        decision.SessionID,
			ToolCallID:       decision.ToolCallID,
			PermissionPath:   decision.CWD,
			PermissionPolicy: decision.Status,
			PermissionReason: decision.Reason,
			PolicyMode:       decision.PolicyMode,
			PolicyProfile:    decision.PolicyProfile,
			PolicyRuleID:     decision.PolicyRule,
			AgentTask:        sandboxAuditTask(decision),
			Error:            decision.Error,
			Extra:            map[string]any{"sandbox": payload},
		})
	}
	_ = ctx
}

func runtimeSandboxDecisionPayload(decision RuntimeSandboxDecision) map[string]any {
	return map[string]any{
		"id":              decision.ID,
		"session_id":      decision.SessionID,
		"turn_id":         decision.TurnID,
		"tool_call_id":    decision.ToolCallID,
		"task_id":         decision.TaskID,
		"mode":            decision.Mode,
		"status":          decision.Status,
		"executor":        decision.Executor,
		"cwd":             pathSafeSummary(decision.CWD),
		"worktree_id":     decision.WorktreeID,
		"worktree_path":   pathSafeSummary(decision.WorktreePath),
		"command_summary": preview(redactRuntimeString("command", decision.CommandSummary), 160),
		"policy_mode":     decision.PolicyMode,
		"policy_profile":  decision.PolicyProfile,
		"policy_rule":     decision.PolicyRule,
		"reason":          decision.Reason,
		"error":           decision.Error,
		"allowed_paths":   sandboxPathSummaries(decision.AllowedPaths),
		"denied_paths":    sandboxPathSummaries(decision.DeniedPaths),
		"network_allowed": decision.NetworkAllowed,
		"network_reason":  decision.NetworkReason,
		"created_at":      decision.CreatedAt,
		"completed_at":    decision.CompletedAt,
		"summary":         fmt.Sprintf("sandbox %s %s", decision.Mode, decision.Status),
	}
}

func sandboxPathSummaries(paths []string) []string {
	if len(paths) == 0 {
		return nil
	}
	out := make([]string, 0, len(paths))
	for _, path := range paths {
		if summary := pathSafeSummary(path); summary != "" {
			out = append(out, summary)
		}
	}
	return out
}

func runtimeSandboxDecisionFromPayload(payload map[string]any) RuntimeSandboxDecision {
	return RuntimeSandboxDecision{
		ID:             stringFromMap(payload, "id"),
		SessionID:      stringFromMap(payload, "session_id"),
		TurnID:         stringFromMap(payload, "turn_id"),
		ToolCallID:     stringFromMap(payload, "tool_call_id"),
		TaskID:         stringFromMap(payload, "task_id"),
		Mode:           stringFromMap(payload, "mode"),
		Status:         stringFromMap(payload, "status"),
		Executor:       stringFromMap(payload, "executor"),
		CWD:            stringFromMap(payload, "cwd"),
		WorktreeID:     stringFromMap(payload, "worktree_id"),
		WorktreePath:   stringFromMap(payload, "worktree_path"),
		CommandSummary: stringFromMap(payload, "command_summary"),
		PolicyMode:     stringFromMap(payload, "policy_mode"),
		PolicyProfile:  stringFromMap(payload, "policy_profile"),
		PolicyRule:     stringFromMap(payload, "policy_rule"),
		Reason:         stringFromMap(payload, "reason"),
		Error:          stringFromMap(payload, "error"),
		AllowedPaths:   stringSliceFromMap(payload, "allowed_paths"),
		DeniedPaths:    stringSliceFromMap(payload, "denied_paths"),
		NetworkAllowed: boolFromMap(payload, "network_allowed"),
		NetworkReason:  stringFromMap(payload, "network_reason"),
		CreatedAt:      int64(intFromMap(payload, "created_at")),
		CompletedAt:    int64(intFromMap(payload, "completed_at")),
	}
}

func runtimeSandboxDecisionFromAudit(event RuntimeAuditEvent) RuntimeSandboxDecision {
	return runtimeSandboxDecisionFromPayload(asMap(asMap(event.Payload["extra"])["sandbox"]))
}

func sandboxAuditTask(decision RuntimeSandboxDecision) *RuntimeAgentTask {
	if decision.TaskID == "" {
		return nil
	}
	return &RuntimeAgentTask{
		ID:              decision.TaskID,
		ParentTurnID:    decision.TurnID,
		ParentSessionID: decision.SessionID,
		CWD:             decision.CWD,
		Worktree:        decision.WorktreePath,
	}
}

func shellCommandFromInputSummary(input string) string {
	var payload map[string]any
	if err := json.Unmarshal([]byte(input), &payload); err == nil {
		for _, key := range []string{"command", "Command", "cmd", "script"} {
			if value, ok := payload[key].(string); ok {
				return value
			}
		}
	}
	return input
}

func sandboxMetadata(decision RuntimeSandboxDecision) tools.SandboxContextMetadata {
	return tools.SandboxContextMetadata{
		DecisionID: decision.ID,
		Mode:       decision.Mode,
		Status:     decision.Status,
		Executor:   decision.Executor,
		Reason:     decision.Reason,
		Error:      decision.Error,
	}
}

func sandboxDeniedPolicyDecision(base agent.SchedulerToolPolicyDecision, decision RuntimeSandboxDecision) agent.SchedulerToolPolicyDecision {
	base.Decision = string(permission.PolicyDeny)
	base.Reason = firstNonEmpty(decision.Error, decision.Reason, "Runtime sandbox denied shell command.")
	base.SandboxDecisionID = decision.ID
	base.SandboxMode = decision.Mode
	base.SandboxStatus = decision.Status
	base.SandboxExecutor = decision.Executor
	base.SandboxReason = decision.Reason
	base.SandboxError = decision.Error
	return base
}

func applySandboxToDecision(base agent.SchedulerToolPolicyDecision, decision RuntimeSandboxDecision) agent.SchedulerToolPolicyDecision {
	base.SandboxDecisionID = decision.ID
	base.SandboxMode = decision.Mode
	base.SandboxStatus = decision.Status
	base.SandboxExecutor = decision.Executor
	base.SandboxReason = decision.Reason
	base.SandboxError = decision.Error
	return base
}

func sandboxRuntimeGOOS() string {
	return runtime.GOOS
}
