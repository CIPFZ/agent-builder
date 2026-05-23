package runtime

import (
	"context"
	"database/sql"
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
	_, err := s.db.ExecContext(ctx, `
INSERT INTO runtime_tool_calls (
    id, turn_id, session_id, message_id, name, source, capability_id, status,
    job_id, command, risk, policy_reason, exit_code,
    input_summary, output_summary, model_content, structured_output, stdout, stderr, is_error,
    started_at, finished_at, error
) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
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
    exit_code = CASE WHEN excluded.exit_code != 0 THEN excluded.exit_code ELSE runtime_tool_calls.exit_code END,
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
    is_error = CASE WHEN excluded.is_error != 0 THEN excluded.is_error ELSE runtime_tool_calls.is_error END,
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
		call.ExitCode,
		nullableString(call.InputSummary),
		nullableString(call.OutputSummary),
		nullableString(call.ModelContent),
		nullableString(call.Structured),
		nullableString(call.Stdout),
		nullableString(call.Stderr),
		boolInt(call.IsError),
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
    job_id, command, risk, policy_reason, exit_code,
    input_summary, output_summary, model_content, structured_output, stdout, stderr, is_error,
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
    job_id, command, risk, policy_reason, exit_code,
    input_summary, output_summary, model_content, structured_output, stdout, stderr, is_error,
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
	var messageID, capabilityID, jobID, command, risk, policyReason, inputSummary, outputSummary, modelContent, structured, stdout, stderr, errText sql.NullString
	var source, status string
	var isError, exitCode int
	var startedAt int64
	var finishedAt sql.NullInt64
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
		&exitCode,
		&inputSummary,
		&outputSummary,
		&modelContent,
		&structured,
		&stdout,
		&stderr,
		&isError,
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
	call.ExitCode = exitCode
	call.InputSummary = inputSummary.String
	call.OutputSummary = outputSummary.String
	call.ModelContent = modelContent.String
	call.Structured = structured.String
	call.Stdout = stdout.String
	call.Stderr = stderr.String
	call.IsError = isError != 0
	call.StartedAt = time.UnixMilli(startedAt).UTC()
	if finishedAt.Valid {
		call.FinishedAt = time.UnixMilli(finishedAt.Int64).UTC()
	}
	call.Error = errText.String
	return call, nil
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
