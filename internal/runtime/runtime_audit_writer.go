package runtime

import (
	"context"
	"encoding/json"
	"log/slog"
	"os"
	"path/filepath"
	"time"

	"github.com/charmbracelet/crush/internal/runtimeapi"
)

type auditEntry struct {
	RequestID             string                     `json:"request_id"`
	Event                 string                     `json:"event"`
	Timestamp             string                     `json:"timestamp"`
	WorkspaceID           string                     `json:"workspace_id,omitempty"`
	SessionID             string                     `json:"session_id,omitempty"`
	Provider              string                     `json:"provider,omitempty"`
	Model                 string                     `json:"model,omitempty"`
	PromptLength          int                        `json:"prompt_length,omitempty"`
	PromptPreview         string                     `json:"prompt_preview,omitempty"`
	ResponseLength        int                        `json:"response_length,omitempty"`
	ResponsePreview       string                     `json:"response_preview,omitempty"`
	DurationMS            int64                      `json:"duration_ms,omitempty"`
	FinishReason          string                     `json:"finish_reason,omitempty"`
	UsageBefore           *RuntimeUsage              `json:"usage_before,omitempty"`
	UsageAfter            *RuntimeUsage              `json:"usage_after,omitempty"`
	UsageDelta            *RuntimeUsage              `json:"usage_delta,omitempty"`
	Skills                []RuntimeSkill             `json:"skills,omitempty"`
	SkillSummary          *RuntimeTurnSkillSummary   `json:"skill_summary,omitempty"`
	ContextSummary        *RuntimeTurnContextSummary `json:"context_summary,omitempty"`
	Budget                *RuntimeBudgetReport       `json:"budget,omitempty"`
	CompactBoundary       *RuntimeCompactBoundary    `json:"compact_boundary,omitempty"`
	MCPServers            []RuntimeMCPServer         `json:"mcp_servers,omitempty"`
	MCPTools              []RuntimeMCPTool           `json:"mcp_tools,omitempty"`
	ToolCalls             []auditToolCall            `json:"tool_calls,omitempty"`
	Error                 string                     `json:"error,omitempty"`
	LatestAssistantID     string                     `json:"latest_assistant_id,omitempty"`
	LatestAssistantFinish bool                       `json:"latest_assistant_finished,omitempty"`
	PermissionTool        string                     `json:"permission_tool,omitempty"`
	PermissionAction      string                     `json:"permission_action,omitempty"`
	PermissionID          string                     `json:"permission_id,omitempty"`
	PermissionPath        string                     `json:"permission_path,omitempty"`
	PermissionPolicy      string                     `json:"permission_policy,omitempty"`
	PermissionRisk        string                     `json:"permission_risk,omitempty"`
	PermissionReason      string                     `json:"permission_reason,omitempty"`
	PolicyMode            string                     `json:"policy_mode,omitempty"`
	PolicyProfile         string                     `json:"policy_profile,omitempty"`
	PolicyHeadless        bool                       `json:"policy_headless,omitempty"`
	PolicyHeadlessReason  string                     `json:"policy_headless_reason,omitempty"`
	PolicyRuleID          string                     `json:"policy_rule_id,omitempty"`
	PolicyRuleSource      string                     `json:"policy_rule_source,omitempty"`
	PolicyScopeKind       string                     `json:"policy_scope_kind,omitempty"`
	PolicyScopeValue      string                     `json:"policy_scope_value,omitempty"`
	PolicyTargetSummary   string                     `json:"policy_target_summary,omitempty"`
	ShellRisk             string                     `json:"shell_risk,omitempty"`
	ShellReason           string                     `json:"shell_reason,omitempty"`
	ToolCallID            string                     `json:"tool_call_id,omitempty"`
	CapabilityID          string                     `json:"capability_id,omitempty"`
	CapabilityKind        string                     `json:"capability_kind,omitempty"`
	CapabilitySource      string                     `json:"capability_source,omitempty"`
	CapabilityState       string                     `json:"capability_state,omitempty"`
	CapabilityReason      string                     `json:"capability_reason,omitempty"`
	CapabilityError       string                     `json:"capability_error,omitempty"`
	MCPServer             string                     `json:"mcp_server,omitempty"`
	MCPName               string                     `json:"mcp_name,omitempty"`
	MCPKind               string                     `json:"mcp_kind,omitempty"`
	MCPStatus             string                     `json:"mcp_status,omitempty"`
	MCPDecision           string                     `json:"mcp_decision,omitempty"`
	MCPRisk               string                     `json:"mcp_risk,omitempty"`
	MCPReason             string                     `json:"mcp_reason,omitempty"`
	AgentTask             *RuntimeAgentTask          `json:"agent_task,omitempty"`
	Extra                 map[string]any             `json:"extra,omitempty"`
}

type auditToolCall struct {
	ID                   string `json:"id,omitempty"`
	Name                 string `json:"name"`
	Input                string `json:"input,omitempty"`
	Output               string `json:"output,omitempty"`
	JobID                string `json:"job_id,omitempty"`
	Command              string `json:"command,omitempty"`
	Risk                 string `json:"risk,omitempty"`
	PolicyMode           string `json:"policy_mode,omitempty"`
	PolicyProfile        string `json:"policy_profile,omitempty"`
	PolicyHeadless       bool   `json:"policy_headless,omitempty"`
	PolicyHeadlessReason string `json:"policy_headless_reason,omitempty"`
	Headless             bool   `json:"headless,omitempty"`
	HeadlessReason       string `json:"headless_reason,omitempty"`
	PolicyRuleID         string `json:"policy_rule_id,omitempty"`
	PolicyScopeKind      string `json:"policy_scope_kind,omitempty"`
	PolicyScopeValue     string `json:"policy_scope_value,omitempty"`
	ShellRisk            string `json:"shell_risk,omitempty"`
	ShellReason          string `json:"shell_reason,omitempty"`
	ExitCode             int    `json:"exit_code,omitempty"`
	IsError              bool   `json:"is_error,omitempty"`
	Status               string `json:"status,omitempty"`
	StartedAt            int64  `json:"started_at,omitempty"`
	FinishedAt           int64  `json:"finished_at,omitempty"`
}

func (r *runtimeService) writeAudit(entry auditEntry) {
	r.writeRuntimeAuditEvent(context.Background(), entry)

	layout, err := resolveDesktopLayout()
	if err != nil {
		slog.Error("Failed to resolve desktop audit path", "error", err)
		return
	}
	if err := ensureDesktopLayout(layout); err != nil {
		slog.Error("Failed to create desktop audit directory", "error", err)
		return
	}
	if entry.Timestamp == "" {
		entry.Timestamp = time.Now().Format(time.RFC3339Nano)
	}
	path := filepath.Join(layout.LogsDir, "agent-builder-audit.jsonl")
	file, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o600)
	if err != nil {
		slog.Error("Failed to open desktop audit log", "path", path, "error", err)
		return
	}
	defer file.Close() //nolint:errcheck

	payload, err := auditPayload(entry)
	if err != nil {
		slog.Error("Failed to prepare desktop audit entry", "error", err)
		return
	}
	data, err := json.Marshal(payload)
	if err != nil {
		slog.Error("Failed to encode desktop audit entry", "error", err)
		return
	}
	if _, err := file.Write(append(data, '\n')); err != nil {
		slog.Error("Failed to write desktop audit entry", "path", path, "error", err)
	}
}

func (r *runtimeService) writeRuntimeAuditEvent(ctx context.Context, entry auditEntry) {
	db, err := r.workspaceDB(ctx)
	if err != nil {
		slog.Debug("Runtime audit database unavailable", "error", err)
		return
	}
	payload, err := auditPayload(entry)
	if err != nil {
		slog.Error("Failed to prepare runtime audit payload", "error", err)
		return
	}
	event := RuntimeAuditEvent{
		ID:           newRuntimeEventID(),
		SessionID:    entry.SessionID,
		TurnID:       entry.RequestID,
		ToolCallID:   entry.ToolCallID,
		PermissionID: entry.PermissionID,
		Type:         entry.Event,
		CreatedAt:    firstNonEmpty(entry.Timestamp, time.Now().UTC().Format(time.RFC3339Nano)),
		Payload:      payload,
	}
	if err := newRuntimeAuditStore(db).Append(ctx, event); err != nil {
		slog.Error("Failed to write runtime audit event", "error", err)
		return
	}
	r.storeRuntimeEvent(runtimeapi.Event{
		ID:         newRuntimeEventID(),
		Type:       runtimeapi.EventAuditRecorded,
		CreatedAt:  event.CreatedAt,
		SessionID:  event.SessionID,
		TurnID:     event.TurnID,
		ToolCallID: event.ToolCallID,
		Payload: map[string]any{
			"audit_id":      event.ID,
			"type":          event.Type,
			"permission_id": event.PermissionID,
		},
	})
}

func auditPayload(entry auditEntry) (map[string]any, error) {
	data, err := json.Marshal(entry)
	if err != nil {
		return nil, err
	}
	var payload map[string]any
	if err := json.Unmarshal(data, &payload); err != nil {
		return nil, err
	}
	return redactRuntimePayload(payload), nil
}
