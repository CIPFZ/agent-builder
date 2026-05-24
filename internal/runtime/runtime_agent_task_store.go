package runtime

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"
)

const (
	agentTaskKindSubagent     = "subagent"
	agentTaskKindAgenticFetch = "agentic_fetch"
	agentTaskKindBackground   = "background"

	agentTaskStatusQueued      = "queued"
	agentTaskStatusRunning     = "running"
	agentTaskStatusCompleted   = "completed"
	agentTaskStatusFailed      = "failed"
	agentTaskStatusCancelled   = "cancelled"
	agentTaskStatusInterrupted = "interrupted"
)

var errRuntimeAgentTaskNotFound = errors.New("runtime agent task not found")

type runtimeAgentTaskStore struct {
	db *sql.DB
}

func newRuntimeAgentTaskStore(db *sql.DB) runtimeAgentTaskStore {
	return runtimeAgentTaskStore{db: db}
}

func (s runtimeAgentTaskStore) Upsert(ctx context.Context, task RuntimeAgentTask) (RuntimeAgentTask, error) {
	if s.db == nil {
		return RuntimeAgentTask{}, errors.New("runtime agent task database is not available")
	}
	task.ID = strings.TrimSpace(task.ID)
	task.ParentSessionID = strings.TrimSpace(task.ParentSessionID)
	if task.ID == "" {
		return RuntimeAgentTask{}, errors.New("agent task id is required")
	}
	if task.ParentSessionID == "" {
		return RuntimeAgentTask{}, errors.New("agent task parent session id is required")
	}
	if task.Title == "" {
		task.Title = "Agent task"
	}
	if task.Kind == "" {
		task.Kind = agentTaskKindSubagent
	}
	if task.Status == "" {
		task.Status = agentTaskStatusQueued
	}
	if task.Progress < 0 {
		task.Progress = 0
	}
	if task.Progress > 100 {
		task.Progress = 100
	}
	now := time.Now().UnixMilli()
	if task.StartedAt == 0 {
		task.StartedAt = now
	}
	task.UpdatedAt = now
	if task.FinishedAt > 0 && isFinalAgentTaskStatus(task.Status) {
		task.UpdatedAt = task.FinishedAt
	}
	allowedTools, err := encodeStringSlice(task.AllowedTools)
	if err != nil {
		return RuntimeAgentTask{}, err
	}
	capabilityScope, err := encodeStringSlice(task.CapabilityScope)
	if err != nil {
		return RuntimeAgentTask{}, err
	}
	artifactRefs, err := encodeStringSlice(task.ArtifactRefs)
	if err != nil {
		return RuntimeAgentTask{}, err
	}
	_, err = s.db.ExecContext(ctx, `
INSERT INTO runtime_agent_tasks (
    id, parent_turn_id, parent_session_id, parent_tool_call_id, child_session_id,
    title, kind, role, name, prompt_summary, model, provider,
    allowed_tools_json, capability_scope_json, cwd, worktree, status, progress,
    result_summary, artifact_refs_json, started_at, updated_at, finished_at, error
) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
ON CONFLICT(id) DO UPDATE SET
    parent_turn_id = COALESCE(NULLIF(excluded.parent_turn_id, ''), runtime_agent_tasks.parent_turn_id),
    parent_session_id = COALESCE(NULLIF(excluded.parent_session_id, ''), runtime_agent_tasks.parent_session_id),
    parent_tool_call_id = COALESCE(NULLIF(excluded.parent_tool_call_id, ''), runtime_agent_tasks.parent_tool_call_id),
    child_session_id = COALESCE(NULLIF(excluded.child_session_id, ''), runtime_agent_tasks.child_session_id),
    title = COALESCE(NULLIF(excluded.title, ''), runtime_agent_tasks.title),
    kind = COALESCE(NULLIF(excluded.kind, ''), runtime_agent_tasks.kind),
    role = COALESCE(NULLIF(excluded.role, ''), runtime_agent_tasks.role),
    name = COALESCE(NULLIF(excluded.name, ''), runtime_agent_tasks.name),
    prompt_summary = COALESCE(NULLIF(excluded.prompt_summary, ''), runtime_agent_tasks.prompt_summary),
    model = COALESCE(NULLIF(excluded.model, ''), runtime_agent_tasks.model),
    provider = COALESCE(NULLIF(excluded.provider, ''), runtime_agent_tasks.provider),
    allowed_tools_json = COALESCE(excluded.allowed_tools_json, runtime_agent_tasks.allowed_tools_json),
    capability_scope_json = COALESCE(excluded.capability_scope_json, runtime_agent_tasks.capability_scope_json),
    cwd = COALESCE(NULLIF(excluded.cwd, ''), runtime_agent_tasks.cwd),
    worktree = COALESCE(NULLIF(excluded.worktree, ''), runtime_agent_tasks.worktree),
    status = CASE
        WHEN runtime_agent_tasks.status IN ('completed', 'failed', 'cancelled', 'interrupted')
             AND excluded.status IN ('queued', 'running')
        THEN runtime_agent_tasks.status
        ELSE excluded.status
    END,
    progress = CASE
        WHEN excluded.progress != 0 THEN excluded.progress
        ELSE runtime_agent_tasks.progress
    END,
    result_summary = COALESCE(NULLIF(excluded.result_summary, ''), runtime_agent_tasks.result_summary),
    artifact_refs_json = COALESCE(excluded.artifact_refs_json, runtime_agent_tasks.artifact_refs_json),
    started_at = runtime_agent_tasks.started_at,
    updated_at = excluded.updated_at,
    finished_at = CASE
        WHEN runtime_agent_tasks.status IN ('completed', 'failed', 'cancelled', 'interrupted')
             AND excluded.status IN ('queued', 'running')
        THEN runtime_agent_tasks.finished_at
        WHEN excluded.status NOT IN ('completed', 'failed', 'cancelled', 'interrupted')
        THEN NULL
        ELSE COALESCE(excluded.finished_at, runtime_agent_tasks.finished_at)
    END,
    error = COALESCE(NULLIF(excluded.error, ''), runtime_agent_tasks.error)`,
		task.ID,
		nullableString(task.ParentTurnID),
		task.ParentSessionID,
		nullableString(task.ParentToolCallID),
		nullableString(task.ChildSessionID),
		task.Title,
		task.Kind,
		nullableString(task.Role),
		nullableString(task.Name),
		nullableString(task.PromptSummary),
		nullableString(task.Model),
		nullableString(task.Provider),
		allowedTools,
		capabilityScope,
		nullableString(task.CWD),
		nullableString(task.Worktree),
		task.Status,
		task.Progress,
		nullableString(task.ResultSummary),
		artifactRefs,
		task.StartedAt,
		task.UpdatedAt,
		nullableInt64(task.FinishedAt),
		nullableString(task.Error),
	)
	if err != nil {
		return RuntimeAgentTask{}, fmt.Errorf("failed to upsert runtime agent task: %w", err)
	}
	return s.Get(ctx, task.ID)
}

func (s runtimeAgentTaskStore) Get(ctx context.Context, id string) (RuntimeAgentTask, error) {
	if s.db == nil {
		return RuntimeAgentTask{}, errors.New("runtime agent task database is not available")
	}
	row := s.db.QueryRowContext(ctx, runtimeAgentTaskSelectSQL()+` WHERE id = ?`, strings.TrimSpace(id))
	task, err := scanRuntimeAgentTask(row)
	if errors.Is(err, sql.ErrNoRows) {
		return RuntimeAgentTask{}, errRuntimeAgentTaskNotFound
	}
	return task, err
}

func (s runtimeAgentTaskStore) ListByTurn(ctx context.Context, turnID string) ([]RuntimeAgentTask, error) {
	return s.list(ctx, `parent_turn_id = ?`, strings.TrimSpace(turnID))
}

func (s runtimeAgentTaskStore) ListByChildSession(ctx context.Context, childSessionID string) ([]RuntimeAgentTask, error) {
	return s.list(ctx, `child_session_id = ?`, strings.TrimSpace(childSessionID))
}

func (s runtimeAgentTaskStore) ListByStatus(ctx context.Context, status string) ([]RuntimeAgentTask, error) {
	status = strings.TrimSpace(status)
	if status == "active" {
		return s.list(ctx, `status IN (?, ?)`, agentTaskStatusQueued, agentTaskStatusRunning)
	}
	if status == "" {
		return s.list(ctx, `1 = 1`)
	}
	return s.list(ctx, `status = ?`, status)
}

func (s runtimeAgentTaskStore) InterruptUnfinished(ctx context.Context) ([]RuntimeAgentTask, error) {
	active, err := s.ListByStatus(ctx, "active")
	if err != nil {
		return nil, err
	}
	now := time.Now().UnixMilli()
	for i := range active {
		active[i].Status = agentTaskStatusInterrupted
		active[i].Progress = 100
		active[i].FinishedAt = now
		active[i].Error = firstNonEmpty(active[i].Error, "runtime restarted before agent task completed")
		if _, err := s.Upsert(ctx, active[i]); err != nil {
			return nil, err
		}
	}
	return active, nil
}

func (s runtimeAgentTaskStore) list(ctx context.Context, where string, args ...any) ([]RuntimeAgentTask, error) {
	if s.db == nil {
		return nil, errors.New("runtime agent task database is not available")
	}
	rows, err := s.db.QueryContext(ctx, runtimeAgentTaskSelectSQL()+` WHERE `+where+` ORDER BY updated_at ASC`, args...)
	if err != nil {
		return nil, fmt.Errorf("failed to list runtime agent tasks: %w", err)
	}
	defer rows.Close() //nolint:errcheck

	var tasks []RuntimeAgentTask
	for rows.Next() {
		task, err := scanRuntimeAgentTask(rows)
		if err != nil {
			return nil, err
		}
		tasks = append(tasks, task)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("failed to iterate runtime agent tasks: %w", err)
	}
	return tasks, nil
}

func runtimeAgentTaskSelectSQL() string {
	return `
SELECT id, parent_turn_id, parent_session_id, parent_tool_call_id, child_session_id,
    title, kind, role, name, prompt_summary, model, provider,
    allowed_tools_json, capability_scope_json, cwd, worktree, status, progress,
    result_summary, artifact_refs_json, started_at, updated_at, finished_at, error
FROM runtime_agent_tasks`
}

type runtimeAgentTaskScanner interface {
	Scan(dest ...any) error
}

func scanRuntimeAgentTask(scanner runtimeAgentTaskScanner) (RuntimeAgentTask, error) {
	var task RuntimeAgentTask
	var parentTurnID, parentToolCallID, childSessionID, role, name, promptSummary, model, provider, allowedTools, capabilityScope, cwd, worktree, resultSummary, artifactRefs, errText sql.NullString
	var finishedAt sql.NullInt64
	if err := scanner.Scan(
		&task.ID,
		&parentTurnID,
		&task.ParentSessionID,
		&parentToolCallID,
		&childSessionID,
		&task.Title,
		&task.Kind,
		&role,
		&name,
		&promptSummary,
		&model,
		&provider,
		&allowedTools,
		&capabilityScope,
		&cwd,
		&worktree,
		&task.Status,
		&task.Progress,
		&resultSummary,
		&artifactRefs,
		&task.StartedAt,
		&task.UpdatedAt,
		&finishedAt,
		&errText,
	); err != nil {
		return RuntimeAgentTask{}, err
	}
	task.ParentTurnID = parentTurnID.String
	task.ParentToolCallID = parentToolCallID.String
	task.ChildSessionID = childSessionID.String
	task.Role = role.String
	task.Name = name.String
	task.PromptSummary = promptSummary.String
	task.Model = model.String
	task.Provider = provider.String
	task.AllowedTools = decodeStringSlice(allowedTools.String)
	task.CapabilityScope = decodeStringSlice(capabilityScope.String)
	task.CWD = cwd.String
	task.Worktree = worktree.String
	task.ResultSummary = resultSummary.String
	task.ArtifactRefs = decodeStringSlice(artifactRefs.String)
	if finishedAt.Valid {
		task.FinishedAt = finishedAt.Int64
	}
	task.Error = errText.String
	return task, nil
}

func encodeStringSlice(values []string) (any, error) {
	if len(values) == 0 {
		return nil, nil
	}
	data, err := json.Marshal(values)
	if err != nil {
		return nil, fmt.Errorf("failed to encode string slice: %w", err)
	}
	return string(data), nil
}

func decodeStringSlice(data string) []string {
	if strings.TrimSpace(data) == "" {
		return nil
	}
	var values []string
	_ = json.Unmarshal([]byte(data), &values)
	return values
}

func encodeStringMap(values map[string]string) (any, error) {
	if len(values) == 0 {
		return nil, nil
	}
	data, err := json.Marshal(values)
	if err != nil {
		return nil, fmt.Errorf("failed to encode string map: %w", err)
	}
	return string(data), nil
}

func decodeStringMap(data string) map[string]string {
	if strings.TrimSpace(data) == "" {
		return nil
	}
	values := map[string]string{}
	_ = json.Unmarshal([]byte(data), &values)
	return values
}

func isFinalAgentTaskStatus(status string) bool {
	switch status {
	case agentTaskStatusCompleted, agentTaskStatusFailed, agentTaskStatusCancelled, agentTaskStatusInterrupted:
		return true
	default:
		return false
	}
}
