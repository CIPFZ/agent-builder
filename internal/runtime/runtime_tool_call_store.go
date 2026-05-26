package runtime

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/charmbracelet/crush/internal/tools/scheduler"
)

type runtimeSQLiteToolCallStore struct {
	db *sql.DB
}

func newRuntimeSQLiteToolCallStore(db *sql.DB) scheduler.Store {
	if db == nil {
		return scheduler.NewMemoryStore()
	}
	return runtimeSQLiteToolCallStore{db: db}
}

func (s runtimeSQLiteToolCallStore) Upsert(ctx context.Context, call scheduler.ToolCall) (scheduler.ToolCall, error) {
	if s.db == nil {
		return scheduler.ToolCall{}, errors.New("runtime tool call database is not available")
	}
	call.ID = strings.TrimSpace(call.ID)
	if call.ID == "" {
		return scheduler.ToolCall{}, errors.New("tool call id is required")
	}
	if call.Source == "" {
		call.Source = scheduler.ToolSourceUnknown
	}
	call.CapabilityID = strings.TrimSpace(call.CapabilityID)
	if call.CapabilityID == "" {
		call.CapabilityID = capabilityIDForToolName(call.Name)
	}
	if call.Status == "" {
		call.Status = scheduler.ToolCallPending
	}
	if call.StartedAt.IsZero() {
		call.StartedAt = time.Now().UTC()
	}
	if call.JobID != "" && call.JobStatus == "" {
		call.JobStatus = string(call.Status)
	}
	_, err := s.db.ExecContext(ctx, `
INSERT INTO runtime_tool_calls (
    id, turn_id, session_id, message_id, name, source, capability_id, status,
    job_id, command, risk, policy_reason, policy_mode, policy_profile, policy_headless,
    policy_headless_reason, policy_rule_id, policy_rule_source, policy_scope_kind, policy_scope_value, policy_target_summary,
    shell_risk, shell_reason, sandbox_decision_id, sandbox_mode, sandbox_status,
    sandbox_executor, sandbox_reason, sandbox_error, exit_code, job_status, job_started_at, job_finished_at,
	input_summary, output_summary, model_content, structured_output, stdout, stderr, is_error,
	output_refs_json, artifact_refs_json, diff_refs_json,
	compacted, compact_ref, compact_boundary_id, compact_original_estimated_tokens, compacted_at,
    started_at, finished_at, error
) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
ON CONFLICT(id) DO UPDATE SET
    turn_id = COALESCE(NULLIF(excluded.turn_id, ''), runtime_tool_calls.turn_id),
    session_id = COALESCE(NULLIF(excluded.session_id, ''), runtime_tool_calls.session_id),
    message_id = COALESCE(NULLIF(excluded.message_id, ''), runtime_tool_calls.message_id),
    name = COALESCE(NULLIF(excluded.name, ''), runtime_tool_calls.name),
    source = COALESCE(NULLIF(excluded.source, ''), runtime_tool_calls.source),
    capability_id = COALESCE(NULLIF(excluded.capability_id, ''), runtime_tool_calls.capability_id),
    job_id = COALESCE(NULLIF(excluded.job_id, ''), runtime_tool_calls.job_id),
    command = COALESCE(NULLIF(excluded.command, ''), runtime_tool_calls.command),
    risk = COALESCE(NULLIF(excluded.risk, ''), runtime_tool_calls.risk),
    policy_reason = COALESCE(NULLIF(excluded.policy_reason, ''), runtime_tool_calls.policy_reason),
    policy_mode = COALESCE(NULLIF(excluded.policy_mode, ''), runtime_tool_calls.policy_mode),
    policy_profile = COALESCE(NULLIF(excluded.policy_profile, ''), runtime_tool_calls.policy_profile),
    policy_headless = CASE WHEN excluded.policy_headless != 0 THEN excluded.policy_headless ELSE runtime_tool_calls.policy_headless END,
    policy_headless_reason = COALESCE(NULLIF(excluded.policy_headless_reason, ''), runtime_tool_calls.policy_headless_reason),
    policy_rule_id = COALESCE(NULLIF(excluded.policy_rule_id, ''), runtime_tool_calls.policy_rule_id),
    policy_rule_source = COALESCE(NULLIF(excluded.policy_rule_source, ''), runtime_tool_calls.policy_rule_source),
    policy_scope_kind = COALESCE(NULLIF(excluded.policy_scope_kind, ''), runtime_tool_calls.policy_scope_kind),
    policy_scope_value = COALESCE(NULLIF(excluded.policy_scope_value, ''), runtime_tool_calls.policy_scope_value),
    policy_target_summary = COALESCE(NULLIF(excluded.policy_target_summary, ''), runtime_tool_calls.policy_target_summary),
    shell_risk = COALESCE(NULLIF(excluded.shell_risk, ''), runtime_tool_calls.shell_risk),
    shell_reason = COALESCE(NULLIF(excluded.shell_reason, ''), runtime_tool_calls.shell_reason),
    sandbox_decision_id = COALESCE(NULLIF(excluded.sandbox_decision_id, ''), runtime_tool_calls.sandbox_decision_id),
    sandbox_mode = COALESCE(NULLIF(excluded.sandbox_mode, ''), runtime_tool_calls.sandbox_mode),
    sandbox_status = COALESCE(NULLIF(excluded.sandbox_status, ''), runtime_tool_calls.sandbox_status),
    sandbox_executor = COALESCE(NULLIF(excluded.sandbox_executor, ''), runtime_tool_calls.sandbox_executor),
    sandbox_reason = COALESCE(NULLIF(excluded.sandbox_reason, ''), runtime_tool_calls.sandbox_reason),
    sandbox_error = COALESCE(NULLIF(excluded.sandbox_error, ''), runtime_tool_calls.sandbox_error),
    exit_code = CASE WHEN excluded.exit_code != 0 THEN excluded.exit_code ELSE runtime_tool_calls.exit_code END,
    job_status = COALESCE(NULLIF(excluded.job_status, ''), runtime_tool_calls.job_status),
    job_started_at = COALESCE(excluded.job_started_at, runtime_tool_calls.job_started_at),
    job_finished_at = COALESCE(excluded.job_finished_at, runtime_tool_calls.job_finished_at),
    status = CASE
        WHEN runtime_tool_calls.status IN ('completed', 'failed', 'cancelled', 'denied')
             AND excluded.status IN ('pending', 'running', 'waiting_permission')
        THEN runtime_tool_calls.status
        ELSE excluded.status
    END,
    input_summary = COALESCE(NULLIF(excluded.input_summary, ''), runtime_tool_calls.input_summary),
    output_summary = COALESCE(NULLIF(excluded.output_summary, ''), runtime_tool_calls.output_summary),
    model_content = COALESCE(NULLIF(excluded.model_content, ''), runtime_tool_calls.model_content),
    structured_output = COALESCE(NULLIF(excluded.structured_output, ''), runtime_tool_calls.structured_output),
    stdout = COALESCE(NULLIF(excluded.stdout, ''), runtime_tool_calls.stdout),
    stderr = COALESCE(NULLIF(excluded.stderr, ''), runtime_tool_calls.stderr),
    output_refs_json = COALESCE(NULLIF(excluded.output_refs_json, ''), runtime_tool_calls.output_refs_json),
    artifact_refs_json = COALESCE(NULLIF(excluded.artifact_refs_json, ''), runtime_tool_calls.artifact_refs_json),
    diff_refs_json = COALESCE(NULLIF(excluded.diff_refs_json, ''), runtime_tool_calls.diff_refs_json),
    is_error = CASE WHEN excluded.is_error != 0 THEN excluded.is_error ELSE runtime_tool_calls.is_error END,
    compacted = CASE WHEN excluded.compacted != 0 THEN excluded.compacted ELSE runtime_tool_calls.compacted END,
    compact_ref = COALESCE(NULLIF(excluded.compact_ref, ''), runtime_tool_calls.compact_ref),
    compact_boundary_id = COALESCE(NULLIF(excluded.compact_boundary_id, ''), runtime_tool_calls.compact_boundary_id),
    compact_original_estimated_tokens = CASE WHEN excluded.compact_original_estimated_tokens != 0 THEN excluded.compact_original_estimated_tokens ELSE runtime_tool_calls.compact_original_estimated_tokens END,
    compacted_at = COALESCE(excluded.compacted_at, runtime_tool_calls.compacted_at),
    started_at = runtime_tool_calls.started_at,
    finished_at = CASE
        WHEN runtime_tool_calls.status IN ('completed', 'failed', 'cancelled', 'denied')
             AND excluded.status IN ('pending', 'running', 'waiting_permission')
        THEN runtime_tool_calls.finished_at
        WHEN excluded.status NOT IN ('completed', 'failed', 'cancelled', 'denied')
        THEN NULL
        ELSE COALESCE(excluded.finished_at, runtime_tool_calls.finished_at)
    END,
    error = COALESCE(NULLIF(excluded.error, ''), runtime_tool_calls.error)`,
		call.ID,
		call.TurnID,
		call.SessionID,
		nullableString(call.MessageID),
		call.Name,
		string(call.Source),
		nullableString(call.CapabilityID),
		string(call.Status),
		nullableString(call.JobID),
		nullableString(call.Command),
		nullableString(call.Risk),
		nullableString(call.PolicyReason),
		nullableString(call.PolicyMode),
		nullableString(call.PolicyProfile),
		boolInt(call.PolicyHeadless),
		nullableString(call.PolicyHeadlessReason),
		nullableString(call.PolicyRuleID),
		nullableString(call.PolicyRuleSource),
		nullableString(call.PolicyScopeKind),
		nullableString(call.PolicyScopeValue),
		nullableString(call.PolicyTargetSummary),
		nullableString(call.ShellRisk),
		nullableString(call.ShellReason),
		nullableString(call.SandboxDecisionID),
		nullableString(call.SandboxMode),
		nullableString(call.SandboxStatus),
		nullableString(call.SandboxExecutor),
		nullableString(call.SandboxReason),
		nullableString(call.SandboxError),
		call.ExitCode,
		nullableString(call.JobStatus),
		nullableTimeMillis(call.JobStartedAt),
		nullableTimeMillis(call.JobFinishedAt),
		nullableString(call.InputSummary),
		nullableString(call.OutputSummary),
		nullableString(call.ModelContent),
		nullableString(call.Structured),
		nullableString(call.Stdout),
		nullableString(call.Stderr),
		boolInt(call.IsError),
		nullableString(encodeRuntimeStringRefs(call.OutputRefs)),
		nullableString(encodeRuntimeStringRefs(call.ArtifactRefs)),
		nullableString(encodeRuntimeStringRefs(call.DiffRefs)),
		boolInt(call.Compacted),
		nullableString(call.CompactRef),
		nullableString(call.CompactBoundaryID),
		call.CompactOriginalEstimatedTokens,
		nullableTimeMillis(call.CompactedAt),
		call.StartedAt.UnixMilli(),
		nullableTimeMillis(call.FinishedAt),
		nullableString(call.Error),
	)
	if err != nil {
		return scheduler.ToolCall{}, fmt.Errorf("failed to upsert runtime tool call: %w", err)
	}
	return s.Get(ctx, call.ID)
}

func (s runtimeSQLiteToolCallStore) Get(ctx context.Context, id string) (scheduler.ToolCall, error) {
	row := s.db.QueryRowContext(ctx, `
SELECT id, turn_id, session_id, message_id, name, source, capability_id, status,
    job_id, command, risk, policy_reason, policy_mode, policy_profile, policy_headless,
    policy_headless_reason, policy_rule_id, policy_rule_source, policy_scope_kind, policy_scope_value, policy_target_summary,
    shell_risk, shell_reason, sandbox_decision_id, sandbox_mode, sandbox_status,
    sandbox_executor, sandbox_reason, sandbox_error, exit_code, job_status, job_started_at, job_finished_at,
    input_summary, output_summary, model_content, structured_output, stdout, stderr, is_error,
    output_refs_json, artifact_refs_json, diff_refs_json,
    compacted, compact_ref, compact_boundary_id, compact_original_estimated_tokens, compacted_at,
    started_at, finished_at, error
FROM runtime_tool_calls
WHERE id = ?`, strings.TrimSpace(id))
	call, err := scanRuntimeToolCall(row)
	if errors.Is(err, sql.ErrNoRows) {
		return scheduler.ToolCall{}, scheduler.ErrToolCallNotFound
	}
	return call, err
}

func (s runtimeSQLiteToolCallStore) ListByTurn(ctx context.Context, turnID string) ([]scheduler.ToolCall, error) {
	rows, err := s.db.QueryContext(ctx, `
SELECT id, turn_id, session_id, message_id, name, source, capability_id, status,
    job_id, command, risk, policy_reason, policy_mode, policy_profile, policy_headless,
    policy_headless_reason, policy_rule_id, policy_rule_source, policy_scope_kind, policy_scope_value, policy_target_summary,
    shell_risk, shell_reason, sandbox_decision_id, sandbox_mode, sandbox_status,
    sandbox_executor, sandbox_reason, sandbox_error, exit_code, job_status, job_started_at, job_finished_at,
    input_summary, output_summary, model_content, structured_output, stdout, stderr, is_error,
    output_refs_json, artifact_refs_json, diff_refs_json,
    compacted, compact_ref, compact_boundary_id, compact_original_estimated_tokens, compacted_at,
    started_at, finished_at, error
FROM runtime_tool_calls
WHERE turn_id = ?
ORDER BY started_at ASC`, strings.TrimSpace(turnID))
	if err != nil {
		return nil, fmt.Errorf("failed to list runtime tool calls: %w", err)
	}
	defer rows.Close() //nolint:errcheck

	var calls []scheduler.ToolCall
	for rows.Next() {
		call, err := scanRuntimeToolCall(rows)
		if err != nil {
			return nil, err
		}
		calls = append(calls, call)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("failed to iterate runtime tool calls: %w", err)
	}
	sort.SliceStable(calls, func(i, j int) bool {
		return calls[i].StartedAt.Before(calls[j].StartedAt)
	})
	return calls, nil
}

type runtimeToolCallScanner interface {
	Scan(dest ...any) error
}

func scanRuntimeToolCall(scanner runtimeToolCallScanner) (scheduler.ToolCall, error) {
	var call scheduler.ToolCall
	var messageID, capabilityID, jobID, command, risk, policyReason, policyMode, policyProfile, policyHeadlessReason, policyRuleID, policyRuleSource, policyScopeKind, policyScopeValue, policyTargetSummary, shellRisk, shellReason, sandboxDecisionID, sandboxMode, sandboxStatus, sandboxExecutor, sandboxReason, sandboxError, jobStatus, inputSummary, outputSummary, modelContent, structured, stdout, stderr, outputRefsJSON, artifactRefsJSON, diffRefsJSON, compactRef, compactBoundaryID, errText sql.NullString
	var source, status string
	var isError, compacted, policyHeadless, exitCode, compactOriginalEstimatedTokens int
	var startedAt int64
	var jobStartedAt, jobFinishedAt, compactedAt, finishedAt sql.NullInt64
	if err := scanner.Scan(
		&call.ID,
		&call.TurnID,
		&call.SessionID,
		&messageID,
		&call.Name,
		&source,
		&capabilityID,
		&status,
		&jobID,
		&command,
		&risk,
		&policyReason,
		&policyMode,
		&policyProfile,
		&policyHeadless,
		&policyHeadlessReason,
		&policyRuleID,
		&policyRuleSource,
		&policyScopeKind,
		&policyScopeValue,
		&policyTargetSummary,
		&shellRisk,
		&shellReason,
		&sandboxDecisionID,
		&sandboxMode,
		&sandboxStatus,
		&sandboxExecutor,
		&sandboxReason,
		&sandboxError,
		&exitCode,
		&jobStatus,
		&jobStartedAt,
		&jobFinishedAt,
		&inputSummary,
		&outputSummary,
		&modelContent,
		&structured,
		&stdout,
		&stderr,
		&isError,
		&outputRefsJSON,
		&artifactRefsJSON,
		&diffRefsJSON,
		&compacted,
		&compactRef,
		&compactBoundaryID,
		&compactOriginalEstimatedTokens,
		&compactedAt,
		&startedAt,
		&finishedAt,
		&errText,
	); err != nil {
		return scheduler.ToolCall{}, err
	}
	call.MessageID = messageID.String
	call.Source = scheduler.ToolSource(source)
	call.CapabilityID = capabilityID.String
	call.Status = scheduler.ToolCallStatus(status)
	call.JobID = jobID.String
	call.Command = command.String
	call.Risk = risk.String
	call.PolicyReason = policyReason.String
	call.PolicyMode = policyMode.String
	call.PolicyProfile = policyProfile.String
	call.PolicyHeadless = policyHeadless != 0
	call.PolicyHeadlessReason = policyHeadlessReason.String
	call.PolicyRuleID = policyRuleID.String
	call.PolicyRuleSource = policyRuleSource.String
	call.PolicyScopeKind = policyScopeKind.String
	call.PolicyScopeValue = policyScopeValue.String
	call.PolicyTargetSummary = policyTargetSummary.String
	call.ShellRisk = shellRisk.String
	call.ShellReason = shellReason.String
	call.SandboxDecisionID = sandboxDecisionID.String
	call.SandboxMode = sandboxMode.String
	call.SandboxStatus = sandboxStatus.String
	call.SandboxExecutor = sandboxExecutor.String
	call.SandboxReason = sandboxReason.String
	call.SandboxError = sandboxError.String
	call.ExitCode = exitCode
	call.JobStatus = jobStatus.String
	if jobStartedAt.Valid {
		call.JobStartedAt = time.UnixMilli(jobStartedAt.Int64).UTC()
	}
	if jobFinishedAt.Valid {
		call.JobFinishedAt = time.UnixMilli(jobFinishedAt.Int64).UTC()
	}
	call.InputSummary = inputSummary.String
	call.OutputSummary = outputSummary.String
	call.ModelContent = modelContent.String
	call.Structured = structured.String
	call.Stdout = stdout.String
	call.Stderr = stderr.String
	call.OutputRefs = decodeRuntimeStringRefs(outputRefsJSON.String)
	call.ArtifactRefs = decodeRuntimeStringRefs(artifactRefsJSON.String)
	call.DiffRefs = decodeRuntimeStringRefs(diffRefsJSON.String)
	call.IsError = isError != 0
	call.Compacted = compacted != 0
	call.CompactRef = compactRef.String
	call.CompactBoundaryID = compactBoundaryID.String
	call.CompactOriginalEstimatedTokens = compactOriginalEstimatedTokens
	if compactedAt.Valid {
		call.CompactedAt = time.UnixMilli(compactedAt.Int64).UTC()
	}
	call.StartedAt = time.UnixMilli(startedAt).UTC()
	if finishedAt.Valid {
		call.FinishedAt = time.UnixMilli(finishedAt.Int64).UTC()
	}
	call.Error = errText.String
	return call, nil
}

func encodeRuntimeStringRefs(values []string) string {
	if len(values) == 0 {
		return ""
	}
	data, err := json.Marshal(values)
	if err != nil {
		return ""
	}
	return string(data)
}

func decodeRuntimeStringRefs(data string) []string {
	if strings.TrimSpace(data) == "" {
		return nil
	}
	var values []string
	if err := json.Unmarshal([]byte(data), &values); err != nil {
		return nil
	}
	return values
}

func boolInt(value bool) int {
	if value {
		return 1
	}
	return 0
}

func nullableTimeMillis(value time.Time) any {
	if value.IsZero() {
		return nil
	}
	return value.UnixMilli()
}
