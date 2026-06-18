package runtime

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/charmbracelet/crush/internal/agent"
	"github.com/charmbracelet/crush/internal/config"
	"github.com/charmbracelet/crush/internal/hooks"
	"github.com/charmbracelet/crush/internal/runtimeapi"
)

const (
	hookSourceConfig = "config"

	hookStatusDiscovered = "discovered"
	hookStatusConfigured = "configured"
	hookStatusEnabled    = "enabled"
	hookStatusDisabled   = "disabled"
	hookStatusRunning    = "running"
	hookStatusCompleted  = "completed"
	hookStatusBlocked    = "blocked"
	hookStatusDenied     = "denied"
	hookStatusFailed     = "failed"
	hookStatusSkipped    = "skipped"
)

var supportedRuntimeHookEvents = map[string]struct{}{
	hooks.EventPreToolUse:         {},
	hooks.EventPostToolUse:        {},
	hooks.EventPostToolUseFailure: {},
	hooks.EventUserPromptSubmit:   {},
	hooks.EventPreCompact:         {},
	hooks.EventPostCompact:        {},
	hooks.EventPostSampling:       {},
	hooks.EventStop:               {},
}

type runtimeHookExecutionStore struct {
	db *sql.DB
}

func newRuntimeHookExecutionStore(db *sql.DB) runtimeHookExecutionStore {
	return runtimeHookExecutionStore{db: db}
}

func (s runtimeHookExecutionStore) Upsert(ctx context.Context, execution RuntimeHookExecution) (RuntimeHookExecution, error) {
	if s.db == nil {
		return execution, nil
	}
	execution = sanitizeRuntimeHookExecution(execution)
	_, err := s.db.ExecContext(ctx, `
INSERT INTO runtime_hook_executions (
    id, hook_id, hook_name, hook_source, event, status, session_id, turn_id,
    tool_call_id, task_id, capability_id, mcp_server, skill, context_ref,
    policy_mode, policy_profile, policy_rule, policy_decision, policy_reason,
    headless, headless_reason, sandbox_decision_id, sandbox_status,
    scope_kind, scope_value, reason, error, input_summary, output_summary,
    context_summary, input_rewritten, context_injected, redacted, started_at,
    completed_at, duration_ms
) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
ON CONFLICT(id) DO UPDATE SET
    hook_id = excluded.hook_id,
    hook_name = excluded.hook_name,
    hook_source = excluded.hook_source,
    event = excluded.event,
    status = excluded.status,
    session_id = COALESCE(NULLIF(excluded.session_id, ''), runtime_hook_executions.session_id),
    turn_id = COALESCE(NULLIF(excluded.turn_id, ''), runtime_hook_executions.turn_id),
    tool_call_id = COALESCE(NULLIF(excluded.tool_call_id, ''), runtime_hook_executions.tool_call_id),
    task_id = COALESCE(NULLIF(excluded.task_id, ''), runtime_hook_executions.task_id),
    capability_id = COALESCE(NULLIF(excluded.capability_id, ''), runtime_hook_executions.capability_id),
    mcp_server = COALESCE(NULLIF(excluded.mcp_server, ''), runtime_hook_executions.mcp_server),
    skill = COALESCE(NULLIF(excluded.skill, ''), runtime_hook_executions.skill),
    context_ref = COALESCE(NULLIF(excluded.context_ref, ''), runtime_hook_executions.context_ref),
    policy_mode = COALESCE(NULLIF(excluded.policy_mode, ''), runtime_hook_executions.policy_mode),
    policy_profile = COALESCE(NULLIF(excluded.policy_profile, ''), runtime_hook_executions.policy_profile),
    policy_rule = COALESCE(NULLIF(excluded.policy_rule, ''), runtime_hook_executions.policy_rule),
    policy_decision = COALESCE(NULLIF(excluded.policy_decision, ''), runtime_hook_executions.policy_decision),
    policy_reason = COALESCE(NULLIF(excluded.policy_reason, ''), runtime_hook_executions.policy_reason),
    headless = CASE WHEN excluded.headless != 0 THEN excluded.headless ELSE runtime_hook_executions.headless END,
    headless_reason = COALESCE(NULLIF(excluded.headless_reason, ''), runtime_hook_executions.headless_reason),
    sandbox_decision_id = COALESCE(NULLIF(excluded.sandbox_decision_id, ''), runtime_hook_executions.sandbox_decision_id),
    sandbox_status = COALESCE(NULLIF(excluded.sandbox_status, ''), runtime_hook_executions.sandbox_status),
    scope_kind = COALESCE(NULLIF(excluded.scope_kind, ''), runtime_hook_executions.scope_kind),
    scope_value = COALESCE(NULLIF(excluded.scope_value, ''), runtime_hook_executions.scope_value),
    reason = COALESCE(NULLIF(excluded.reason, ''), runtime_hook_executions.reason),
    error = COALESCE(NULLIF(excluded.error, ''), runtime_hook_executions.error),
    input_summary = COALESCE(NULLIF(excluded.input_summary, ''), runtime_hook_executions.input_summary),
    output_summary = COALESCE(NULLIF(excluded.output_summary, ''), runtime_hook_executions.output_summary),
    context_summary = COALESCE(NULLIF(excluded.context_summary, ''), runtime_hook_executions.context_summary),
    input_rewritten = CASE WHEN excluded.input_rewritten != 0 THEN excluded.input_rewritten ELSE runtime_hook_executions.input_rewritten END,
    context_injected = CASE WHEN excluded.context_injected != 0 THEN excluded.context_injected ELSE runtime_hook_executions.context_injected END,
    redacted = excluded.redacted,
    started_at = runtime_hook_executions.started_at,
    completed_at = COALESCE(excluded.completed_at, runtime_hook_executions.completed_at),
    duration_ms = CASE WHEN excluded.duration_ms != 0 THEN excluded.duration_ms ELSE runtime_hook_executions.duration_ms END`,
		execution.ID,
		execution.HookID,
		execution.HookName,
		execution.HookSource,
		execution.Event,
		execution.Status,
		nullableString(execution.SessionID),
		nullableString(execution.TurnID),
		nullableString(execution.ToolCallID),
		nullableString(execution.TaskID),
		nullableString(execution.CapabilityID),
		nullableString(execution.MCPServer),
		nullableString(execution.Skill),
		nullableString(execution.ContextRef),
		nullableString(execution.PolicyMode),
		nullableString(execution.PolicyProfile),
		nullableString(execution.PolicyRule),
		nullableString(execution.PolicyDecision),
		nullableString(execution.PolicyReason),
		boolToInt(execution.Headless),
		nullableString(execution.HeadlessReason),
		nullableString(execution.SandboxDecisionID),
		nullableString(execution.SandboxStatus),
		nullableString(execution.ScopeKind),
		nullableString(execution.ScopeValue),
		nullableString(execution.Reason),
		nullableString(execution.Error),
		nullableString(execution.InputSummary),
		nullableString(execution.OutputSummary),
		nullableString(execution.ContextSummary),
		boolToInt(execution.InputRewritten),
		boolToInt(execution.ContextInjected),
		boolToInt(execution.Redacted),
		execution.StartedAt,
		nullableInt64(execution.CompletedAt),
		execution.DurationMS,
	)
	if err != nil {
		return RuntimeHookExecution{}, fmt.Errorf("failed to upsert runtime hook execution: %w", err)
	}
	return execution, nil
}

func (s runtimeHookExecutionStore) Get(ctx context.Context, id string) (RuntimeHookExecution, error) {
	if s.db == nil {
		return RuntimeHookExecution{}, errors.New("runtime hook execution store is unavailable")
	}
	rows, err := s.db.QueryContext(ctx, `
SELECT id, hook_id, hook_name, hook_source, event, status, session_id, turn_id,
    tool_call_id, task_id, capability_id, mcp_server, skill, context_ref,
    policy_mode, policy_profile, policy_rule, policy_decision, policy_reason,
    headless, headless_reason, sandbox_decision_id, sandbox_status,
    scope_kind, scope_value, reason, error, input_summary, output_summary,
    context_summary, input_rewritten, context_injected, redacted, started_at,
    completed_at, duration_ms
FROM runtime_hook_executions
WHERE id = ?`, strings.TrimSpace(id))
	if err != nil {
		return RuntimeHookExecution{}, fmt.Errorf("failed to get runtime hook execution: %w", err)
	}
	defer rows.Close() //nolint:errcheck
	items, err := scanRuntimeHookExecutionRows(rows)
	if err != nil {
		return RuntimeHookExecution{}, err
	}
	if len(items) == 0 {
		return RuntimeHookExecution{}, sql.ErrNoRows
	}
	return items[0], nil
}

func (s runtimeHookExecutionStore) List(ctx context.Context, req RuntimeHookExecutionsRequest) ([]RuntimeHookExecution, error) {
	if s.db == nil {
		return nil, nil
	}
	query := `
SELECT id, hook_id, hook_name, hook_source, event, status, session_id, turn_id,
    tool_call_id, task_id, capability_id, mcp_server, skill, context_ref,
    policy_mode, policy_profile, policy_rule, policy_decision, policy_reason,
    headless, headless_reason, sandbox_decision_id, sandbox_status,
    scope_kind, scope_value, reason, error, input_summary, output_summary,
    context_summary, input_rewritten, context_injected, redacted, started_at,
    completed_at, duration_ms
FROM runtime_hook_executions`
	var clauses []string
	var args []any
	add := func(column, value string) {
		value = strings.TrimSpace(value)
		if value == "" {
			return
		}
		clauses = append(clauses, column+" = ?")
		args = append(args, value)
	}
	add("session_id", req.SessionID)
	add("turn_id", req.TurnID)
	add("tool_call_id", req.ToolCallID)
	add("task_id", req.TaskID)
	add("event", req.Event)
	add("status", req.Status)
	if len(clauses) > 0 {
		query += " WHERE " + strings.Join(clauses, " AND ")
	}
	query += " ORDER BY started_at ASC"
	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("failed to list runtime hook executions: %w", err)
	}
	defer rows.Close() //nolint:errcheck
	return scanRuntimeHookExecutionRows(rows)
}

func (s runtimeHookExecutionStore) InterruptRunning(ctx context.Context, now int64) ([]RuntimeHookExecution, error) {
	if s.db == nil {
		return nil, nil
	}
	running, err := s.List(ctx, RuntimeHookExecutionsRequest{Status: hookStatusRunning})
	if err != nil {
		return nil, err
	}
	for i := range running {
		running[i].Status = hookStatusFailed
		running[i].Reason = "Runtime restarted while hook was running."
		running[i].Error = "hook execution interrupted by runtime restart"
		running[i].CompletedAt = now
		if running[i].StartedAt > 0 && now >= running[i].StartedAt {
			running[i].DurationMS = now - running[i].StartedAt
		}
		if _, err := s.Upsert(ctx, running[i]); err != nil {
			return nil, err
		}
	}
	return running, nil
}

func scanRuntimeHookExecutionRows(rows *sql.Rows) ([]RuntimeHookExecution, error) {
	var items []RuntimeHookExecution
	for rows.Next() {
		var item RuntimeHookExecution
		var sessionID, turnID, toolCallID, taskID, capabilityID, mcpServer, skill, contextRef sql.NullString
		var policyMode, policyProfile, policyRule, policyDecision, policyReason sql.NullString
		var headlessReason, sandboxDecisionID, sandboxStatus, scopeKind, scopeValue sql.NullString
		var reason, errorText, inputSummary, outputSummary, contextSummary sql.NullString
		var completedAt sql.NullInt64
		var headless, inputRewritten, contextInjected, redacted int
		if err := rows.Scan(
			&item.ID,
			&item.HookID,
			&item.HookName,
			&item.HookSource,
			&item.Event,
			&item.Status,
			&sessionID,
			&turnID,
			&toolCallID,
			&taskID,
			&capabilityID,
			&mcpServer,
			&skill,
			&contextRef,
			&policyMode,
			&policyProfile,
			&policyRule,
			&policyDecision,
			&policyReason,
			&headless,
			&headlessReason,
			&sandboxDecisionID,
			&sandboxStatus,
			&scopeKind,
			&scopeValue,
			&reason,
			&errorText,
			&inputSummary,
			&outputSummary,
			&contextSummary,
			&inputRewritten,
			&contextInjected,
			&redacted,
			&item.StartedAt,
			&completedAt,
			&item.DurationMS,
		); err != nil {
			return nil, fmt.Errorf("failed to scan runtime hook execution: %w", err)
		}
		item.SessionID = sessionID.String
		item.TurnID = turnID.String
		item.ToolCallID = toolCallID.String
		item.TaskID = taskID.String
		item.CapabilityID = capabilityID.String
		item.MCPServer = mcpServer.String
		item.Skill = skill.String
		item.ContextRef = contextRef.String
		item.PolicyMode = policyMode.String
		item.PolicyProfile = policyProfile.String
		item.PolicyRule = policyRule.String
		item.PolicyDecision = policyDecision.String
		item.PolicyReason = policyReason.String
		item.Headless = headless != 0
		item.HeadlessReason = headlessReason.String
		item.SandboxDecisionID = sandboxDecisionID.String
		item.SandboxStatus = sandboxStatus.String
		item.ScopeKind = scopeKind.String
		item.ScopeValue = scopeValue.String
		item.Reason = reason.String
		item.Error = errorText.String
		item.InputSummary = inputSummary.String
		item.OutputSummary = outputSummary.String
		item.ContextSummary = contextSummary.String
		item.InputRewritten = inputRewritten != 0
		item.ContextInjected = contextInjected != 0
		item.Redacted = redacted != 0
		item.CompletedAt = completedAt.Int64
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("failed to iterate runtime hook executions: %w", err)
	}
	return items, nil
}

func (r *runtimeService) Hooks(ctx context.Context) (RuntimeHooksResponse, error) {
	cfg, _, err := r.workspaceConfig(ctx)
	if err != nil {
		return RuntimeHooksResponse{}, err
	}
	return runtimeHooksFromConfig(cfg.Config().Hooks), nil
}

func (r *runtimeService) HookExecutions(ctx context.Context, req RuntimeHookExecutionsRequest) (RuntimeHookExecutionsResponse, error) {
	if err := r.ensureWorkspaceStarted(ctx, false); err != nil {
		return RuntimeHookExecutionsResponse{}, err
	}
	items, err := r.hookExecutions.List(ctx, req)
	if err != nil {
		return RuntimeHookExecutionsResponse{}, err
	}
	return RuntimeHookExecutionsResponse{Executions: items}, nil
}

func (r *runtimeService) HookExecution(ctx context.Context, id string) (RuntimeHookExecutionResponse, error) {
	if err := r.ensureWorkspaceStarted(ctx, false); err != nil {
		return RuntimeHookExecutionResponse{}, err
	}
	item, err := r.hookExecutions.Get(ctx, id)
	if err != nil {
		return RuntimeHookExecutionResponse{}, err
	}
	return RuntimeHookExecutionResponse{Execution: item}, nil
}

func runtimeHooksFromConfig(configured map[string][]config.HookConfig) RuntimeHooksResponse {
	var resp RuntimeHooksResponse
	for event, items := range configured {
		if _, ok := supportedRuntimeHookEvents[event]; !ok {
			resp.Diagnostics = append(resp.Diagnostics, "unknown hook event: "+event)
		}
		for i, item := range items {
			hook := RuntimeHook{
				ID:      runtimeHookID(event, item, i),
				Name:    preview(item.Command, auditPreviewLimit),
				Source:  hookSourceConfig,
				Event:   event,
				Matcher: item.Matcher,
				Enabled: strings.TrimSpace(item.Command) != "",
				Timeout: item.Timeout,
			}
			hook.Status = hookStatusEnabled
			if !hook.Enabled {
				hook.Status = hookStatusDisabled
				hook.Diagnostics = "command is required"
			} else if _, ok := supportedRuntimeHookEvents[event]; !ok {
				hook.Status = hookStatusDisabled
				hook.Diagnostics = "unsupported hook event"
			}
			resp.Hooks = append(resp.Hooks, hook)
		}
	}
	return resp
}

func sanitizeRuntimeHookExecution(execution RuntimeHookExecution) RuntimeHookExecution {
	execution.HookName = preview(redactRuntimeString("hook_name", execution.HookName), auditPreviewLimit)
	execution.HookSource = firstNonEmpty(execution.HookSource, hookSourceConfig)
	execution.Reason = preview(redactRuntimeString("reason", execution.Reason), auditPreviewLimit)
	execution.Error = preview(redactRuntimeString("error", execution.Error), auditPreviewLimit)
	execution.InputSummary = preview(redactRuntimeString("input", execution.InputSummary), runtimePartPreviewLimit)
	execution.OutputSummary = preview(redactRuntimeString("output", execution.OutputSummary), runtimePartPreviewLimit)
	execution.ContextSummary = preview(redactRuntimeString("context", execution.ContextSummary), runtimePartPreviewLimit)
	execution.PolicyReason = preview(redactRuntimeString("policy_reason", execution.PolicyReason), auditPreviewLimit)
	execution.HeadlessReason = preview(redactRuntimeString("headless_reason", execution.HeadlessReason), auditPreviewLimit)
	execution.Redacted = true
	if execution.StartedAt == 0 {
		execution.StartedAt = time.Now().UnixMilli()
	}
	if execution.ID == "" {
		execution.ID = "hook_" + newRequestID()
	}
	return execution
}

func runtimeHookID(event string, hook config.HookConfig, index int) string {
	sum := sha256.Sum256([]byte(fmt.Sprintf("%s\x00%s\x00%s\x00%d", event, hook.Matcher, hook.Command, index)))
	return "hook:" + event + ":" + hex.EncodeToString(sum[:8])
}

func runtimeHookIDFromInfo(event string, info hooks.HookInfo, index int) string {
	return runtimeHookID(event, config.HookConfig{Command: info.Name, Matcher: info.Matcher}, index)
}

func runtimeHookExecutionFromPayload(payload map[string]any) RuntimeHookExecution {
	return RuntimeHookExecution{
		ID:                stringFromMap(payload, "execution_id"),
		HookID:            stringFromMap(payload, "hook_id"),
		HookName:          stringFromMap(payload, "hook_name"),
		HookSource:        stringFromMap(payload, "hook_source"),
		Event:             stringFromMap(payload, "event"),
		Status:            stringFromMap(payload, "status"),
		SessionID:         stringFromMap(payload, "session_id"),
		TurnID:            stringFromMap(payload, "turn_id"),
		ToolCallID:        stringFromMap(payload, "tool_call_id"),
		TaskID:            stringFromMap(payload, "task_id"),
		CapabilityID:      stringFromMap(payload, "capability_id"),
		MCPServer:         stringFromMap(payload, "mcp_server"),
		Skill:             stringFromMap(payload, "skill"),
		ContextRef:        stringFromMap(payload, "context_ref"),
		PolicyMode:        stringFromMap(payload, "policy_mode"),
		PolicyProfile:     stringFromMap(payload, "policy_profile"),
		PolicyRule:        stringFromMap(payload, "policy_rule"),
		PolicyDecision:    stringFromMap(payload, "policy_decision"),
		PolicyReason:      stringFromMap(payload, "policy_reason"),
		Headless:          boolFromMap(payload, "headless"),
		HeadlessReason:    stringFromMap(payload, "headless_reason"),
		SandboxDecisionID: stringFromMap(payload, "sandbox_decision_id"),
		SandboxStatus:     stringFromMap(payload, "sandbox_status"),
		ScopeKind:         stringFromMap(payload, "scope_kind"),
		ScopeValue:        stringFromMap(payload, "scope_value"),
		Reason:            stringFromMap(payload, "reason"),
		Error:             stringFromMap(payload, "error"),
		InputSummary:      stringFromMap(payload, "input_summary"),
		OutputSummary:     stringFromMap(payload, "output_summary"),
		ContextSummary:    stringFromMap(payload, "context_summary"),
		InputRewritten:    boolFromMap(payload, "input_rewritten"),
		ContextInjected:   boolFromMap(payload, "context_injected"),
		Redacted:          boolFromMap(payload, "redacted"),
		StartedAt:         int64(intFromMap(payload, "started_at")),
		CompletedAt:       int64(intFromMap(payload, "completed_at")),
		DurationMS:        int64(intFromMap(payload, "duration_ms")),
	}
}

func runtimeHookExecutionFromAudit(event RuntimeAuditEvent) RuntimeHookExecution {
	extra := asMap(event.Payload["extra"])
	hook := runtimeHookExecutionFromPayload(asMap(extra["hook_execution"]))
	if hook.ID == "" {
		hook = runtimeHookExecutionFromPayload(event.Payload)
	}
	if hook.SessionID == "" {
		hook.SessionID = event.SessionID
	}
	if hook.TurnID == "" {
		hook.TurnID = event.TurnID
	}
	if hook.ToolCallID == "" {
		hook.ToolCallID = event.ToolCallID
	}
	return hook
}

func (r *runtimeService) recordHookExecutionEvent(eventType string, execution RuntimeHookExecution) {
	execution = sanitizeRuntimeHookExecution(execution)
	r.storeRuntimeEvent(runtimeapi.Event{
		ID:         newRuntimeEventID(),
		Type:       eventType,
		CreatedAt:  time.Now().UTC().Format(time.RFC3339Nano),
		SessionID:  execution.SessionID,
		TurnID:     execution.TurnID,
		ToolCallID: execution.ToolCallID,
		Payload:    runtimeHookExecutionPayload(execution),
	})
}

func runtimeHookExecutionPayload(execution RuntimeHookExecution) map[string]any {
	return map[string]any{
		"execution_id":        execution.ID,
		"hook_id":             execution.HookID,
		"hook_name":           execution.HookName,
		"hook_source":         execution.HookSource,
		"event":               execution.Event,
		"status":              execution.Status,
		"session_id":          execution.SessionID,
		"turn_id":             execution.TurnID,
		"tool_call_id":        execution.ToolCallID,
		"task_id":             execution.TaskID,
		"capability_id":       execution.CapabilityID,
		"mcp_server":          execution.MCPServer,
		"skill":               execution.Skill,
		"context_ref":         execution.ContextRef,
		"policy_mode":         execution.PolicyMode,
		"policy_profile":      execution.PolicyProfile,
		"policy_rule":         execution.PolicyRule,
		"policy_decision":     execution.PolicyDecision,
		"policy_reason":       execution.PolicyReason,
		"headless":            execution.Headless,
		"headless_reason":     execution.HeadlessReason,
		"sandbox_decision_id": execution.SandboxDecisionID,
		"sandbox_status":      execution.SandboxStatus,
		"scope_kind":          execution.ScopeKind,
		"scope_value":         execution.ScopeValue,
		"reason":              execution.Reason,
		"error":               execution.Error,
		"input_summary":       execution.InputSummary,
		"output_summary":      execution.OutputSummary,
		"context_summary":     execution.ContextSummary,
		"input_rewritten":     execution.InputRewritten,
		"context_injected":    execution.ContextInjected,
		"redacted":            execution.Redacted,
		"started_at":          execution.StartedAt,
		"completed_at":        execution.CompletedAt,
		"duration_ms":         execution.DurationMS,
		"summary":             execution.HookName + ":" + execution.Event,
	}
}

func (r *runtimeService) auditHookExecution(ctx context.Context, execution RuntimeHookExecution, eventType string) {
	if r == nil {
		return
	}
	execution = sanitizeRuntimeHookExecution(execution)
	store := newRuntimeAuditStore(r.hookExecutions.db)
	_ = store.Append(ctx, RuntimeAuditEvent{
		ID:         newRuntimeEventID(),
		SessionID:  execution.SessionID,
		TurnID:     execution.TurnID,
		ToolCallID: execution.ToolCallID,
		Type:       eventType,
		CreatedAt:  time.Now().UTC().Format(time.RFC3339Nano),
		Payload: map[string]any{
			"event":               eventType,
			"hook_id":             execution.HookID,
			"hook_event":          execution.Event,
			"hook_status":         execution.Status,
			"hook_source":         execution.HookSource,
			"capability_id":       execution.CapabilityID,
			"policy_mode":         execution.PolicyMode,
			"policy_profile":      execution.PolicyProfile,
			"policy_rule":         execution.PolicyRule,
			"policy_decision":     execution.PolicyDecision,
			"policy_reason":       execution.PolicyReason,
			"headless":            execution.Headless,
			"sandbox_decision_id": execution.SandboxDecisionID,
			"sandbox_status":      execution.SandboxStatus,
			"scope_kind":          execution.ScopeKind,
			"scope_value":         execution.ScopeValue,
			"reason":              execution.Reason,
			"error":               execution.Error,
			"redacted":            true,
			"extra": map[string]any{
				"hook_execution": runtimeHookExecutionPayload(execution),
			},
		},
	})
}

func (r *runtimeSchedulerRecorder) HooksDiscovered(ctx context.Context, hooks []agent.RuntimeHookConfig) error {
	if r == nil || r.service == nil {
		return nil
	}
	for i, hook := range hooks {
		status := hookStatusConfigured
		eventType := runtimeapi.EventHookConfigured
		reason := "hook configured"
		if !hook.Enabled {
			status = hookStatusDisabled
			eventType = runtimeapi.EventHookDiscovered
			reason = "hook disabled by validation"
		}
		id := runtimeHookID(hook.Event, config.HookConfig{Command: hook.Name, Matcher: hook.Matcher, Timeout: hook.Timeout}, i)
		r.service.storeRuntimeEvent(runtimeapi.Event{
			ID:        newRuntimeEventID(),
			Type:      eventType,
			CreatedAt: time.Now().UTC().Format(time.RFC3339Nano),
			Payload: map[string]any{
				"hook_id":     id,
				"hook_name":   preview(redactRuntimeString("hook_name", hook.Name), auditPreviewLimit),
				"hook_source": hookSourceConfig,
				"event":       hook.Event,
				"matcher":     hook.Matcher,
				"status":      status,
				"reason":      reason,
				"redacted":    true,
			},
		})
	}
	return nil
}

func (r *runtimeSchedulerRecorder) HookExecutionStarted(ctx context.Context, execution agent.RuntimeHookExecution) error {
	return r.recordHookExecution(ctx, execution, runtimeapi.EventHookExecutionStarted, "hook_execution_started")
}

func (r *runtimeSchedulerRecorder) HookExecutionCompleted(ctx context.Context, execution agent.RuntimeHookExecution) error {
	return r.recordHookExecution(ctx, execution, runtimeapi.EventHookExecutionCompleted, "hook_execution_completed")
}

func (r *runtimeSchedulerRecorder) HookExecutionSkipped(ctx context.Context, execution agent.RuntimeHookExecution) error {
	return r.recordHookExecution(ctx, execution, runtimeapi.EventHookExecutionSkipped, "hook_execution_skipped")
}

func (r *runtimeSchedulerRecorder) HookExecutionBlocked(ctx context.Context, execution agent.RuntimeHookExecution) error {
	return r.recordHookExecution(ctx, execution, runtimeapi.EventHookExecutionBlocked, "hook_execution_blocked")
}

func (r *runtimeSchedulerRecorder) HookExecutionFailed(ctx context.Context, execution agent.RuntimeHookExecution) error {
	return r.recordHookExecution(ctx, execution, runtimeapi.EventHookExecutionFailed, "hook_execution_failed")
}

func (r *runtimeSchedulerRecorder) HookContextInjected(ctx context.Context, execution agent.RuntimeHookExecution) error {
	return r.recordHookExecution(ctx, execution, runtimeapi.EventHookContextInjected, "hook_context_injected")
}

func (r *runtimeSchedulerRecorder) HookInputRewritten(ctx context.Context, execution agent.RuntimeHookExecution) error {
	return r.recordHookExecution(ctx, execution, runtimeapi.EventHookInputRewritten, "hook_input_rewritten")
}

func (r *runtimeSchedulerRecorder) recordHookExecution(ctx context.Context, execution agent.RuntimeHookExecution, runtimeEventType, auditType string) error {
	if r == nil || r.service == nil {
		return nil
	}
	item := runtimeHookExecutionFromAgent(execution)
	if item.SessionID == "" && item.ToolCallID != "" && r.service.toolCalls != nil {
		if call, err := r.service.toolCalls.GetCall(ctx, item.ToolCallID); err == nil {
			item.SessionID = firstNonEmpty(item.SessionID, call.SessionID)
			item.TurnID = firstNonEmpty(item.TurnID, call.TurnID)
			item.CapabilityID = firstNonEmpty(item.CapabilityID, call.CapabilityID)
		}
	}
	stored, err := r.service.hookExecutions.Upsert(ctx, item)
	if err != nil {
		return err
	}
	r.service.recordHookExecutionEvent(runtimeEventType, stored)
	r.service.auditHookExecution(ctx, stored, auditType)
	return nil
}

func runtimeHookExecutionFromAgent(execution agent.RuntimeHookExecution) RuntimeHookExecution {
	return sanitizeRuntimeHookExecution(RuntimeHookExecution{
		ID:                execution.ID,
		HookID:            execution.HookID,
		HookName:          execution.HookName,
		HookSource:        execution.HookSource,
		Event:             execution.Event,
		Status:            execution.Status,
		SessionID:         execution.SessionID,
		TurnID:            execution.TurnID,
		ToolCallID:        execution.ToolCallID,
		TaskID:            execution.TaskID,
		CapabilityID:      execution.CapabilityID,
		MCPServer:         execution.MCPServer,
		Skill:             execution.Skill,
		ContextRef:        execution.ContextRef,
		PolicyMode:        execution.PolicyMode,
		PolicyProfile:     execution.PolicyProfile,
		PolicyRule:        execution.PolicyRule,
		PolicyDecision:    execution.PolicyDecision,
		PolicyReason:      execution.PolicyReason,
		Headless:          execution.Headless,
		HeadlessReason:    execution.HeadlessReason,
		SandboxDecisionID: execution.SandboxDecisionID,
		SandboxStatus:     execution.SandboxStatus,
		ScopeKind:         execution.ScopeKind,
		ScopeValue:        execution.ScopeValue,
		Reason:            execution.Reason,
		Error:             execution.Error,
		InputSummary:      execution.InputSummary,
		OutputSummary:     execution.OutputSummary,
		ContextSummary:    execution.ContextSummary,
		InputRewritten:    execution.InputRewritten,
		ContextInjected:   execution.ContextInjected,
		Redacted:          true,
		StartedAt:         execution.StartedAt,
		CompletedAt:       execution.CompletedAt,
		DurationMS:        execution.DurationMS,
	})
}

func runtimeHookExecutionToAgent(execution RuntimeHookExecution) agent.RuntimeHookExecution {
	return agent.RuntimeHookExecution{
		ID:                execution.ID,
		HookID:            execution.HookID,
		HookName:          execution.HookName,
		HookSource:        execution.HookSource,
		Event:             execution.Event,
		Status:            execution.Status,
		SessionID:         execution.SessionID,
		TurnID:            execution.TurnID,
		ToolCallID:        execution.ToolCallID,
		TaskID:            execution.TaskID,
		CapabilityID:      execution.CapabilityID,
		MCPServer:         execution.MCPServer,
		Skill:             execution.Skill,
		ContextRef:        execution.ContextRef,
		PolicyMode:        execution.PolicyMode,
		PolicyProfile:     execution.PolicyProfile,
		PolicyRule:        execution.PolicyRule,
		PolicyDecision:    execution.PolicyDecision,
		PolicyReason:      execution.PolicyReason,
		Headless:          execution.Headless,
		HeadlessReason:    execution.HeadlessReason,
		SandboxDecisionID: execution.SandboxDecisionID,
		SandboxStatus:     execution.SandboxStatus,
		ScopeKind:         execution.ScopeKind,
		ScopeValue:        execution.ScopeValue,
		Reason:            execution.Reason,
		Error:             execution.Error,
		InputSummary:      execution.InputSummary,
		OutputSummary:     execution.OutputSummary,
		ContextSummary:    execution.ContextSummary,
		InputRewritten:    execution.InputRewritten,
		ContextInjected:   execution.ContextInjected,
		StartedAt:         execution.StartedAt,
		CompletedAt:       execution.CompletedAt,
		DurationMS:        execution.DurationMS,
	}
}

func boolToInt(value bool) int {
	if value {
		return 1
	}
	return 0
}
