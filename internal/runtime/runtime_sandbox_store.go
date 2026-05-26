package runtime

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"
)

const (
	sandboxModeNone     = "none"
	sandboxModeRequired = "required"

	sandboxStatusNotRequired = "not_required"
	sandboxStatusApplied     = "applied"
	sandboxStatusUnavailable = "unavailable"
	sandboxStatusDenied      = "denied"
	sandboxStatusFailed      = "failed"

	sandboxExecutorNone                = "none"
	sandboxExecutorUnavailableBoundary = "unavailable_boundary"
)

var errRuntimeSandboxDecisionNotFound = errors.New("runtime sandbox decision not found")

type runtimeSandboxDecisionStore struct {
	db  *sql.DB
	mu  sync.RWMutex
	mem map[string]RuntimeSandboxDecision
}

func newRuntimeSandboxDecisionStore(db *sql.DB) runtimeSandboxDecisionStore {
	return runtimeSandboxDecisionStore{db: db, mem: map[string]RuntimeSandboxDecision{}}
}

func (s *runtimeSandboxDecisionStore) Upsert(ctx context.Context, decision RuntimeSandboxDecision) (RuntimeSandboxDecision, error) {
	if s == nil {
		return RuntimeSandboxDecision{}, errors.New("runtime sandbox decision store is not available")
	}
	decision.ID = strings.TrimSpace(decision.ID)
	decision.SessionID = strings.TrimSpace(decision.SessionID)
	if decision.ID == "" {
		decision.ID = newRuntimeSandboxDecisionID()
	}
	if decision.SessionID == "" {
		return RuntimeSandboxDecision{}, errors.New("sandbox decision session id is required")
	}
	if decision.Mode == "" {
		decision.Mode = sandboxModeNone
	}
	if decision.Status == "" {
		decision.Status = sandboxStatusNotRequired
	}
	if decision.CreatedAt == 0 {
		decision.CreatedAt = time.Now().UTC().UnixMilli()
	}
	decision.CommandSummary = preview(redactRuntimeString("command", decision.CommandSummary), runtimePartPreviewLimit)
	decision.Reason = preview(redactRuntimeString("reason", decision.Reason), auditPreviewLimit)
	decision.Error = preview(redactRuntimeString("error", decision.Error), auditPreviewLimit)
	decision.CWD = pathSafeSummary(decision.CWD)
	decision.WorktreePath = pathSafeSummary(decision.WorktreePath)
	for i := range decision.AllowedPaths {
		decision.AllowedPaths[i] = pathSafeSummary(decision.AllowedPaths[i])
	}
	for i := range decision.DeniedPaths {
		decision.DeniedPaths[i] = pathSafeSummary(decision.DeniedPaths[i])
	}
	if s.db == nil {
		s.mu.Lock()
		defer s.mu.Unlock()
		if s.mem == nil {
			s.mem = map[string]RuntimeSandboxDecision{}
		}
		s.mem[decision.ID] = decision
		return decision, nil
	}
	_, err := s.db.ExecContext(ctx, `
INSERT INTO runtime_sandbox_decisions (
    id, session_id, turn_id, tool_call_id, task_id, mode, status, executor,
    cwd, worktree_id, worktree_path, command_summary, policy_mode, policy_profile,
    policy_rule, reason, error, allowed_paths_json, denied_paths_json,
    network_allowed, network_reason, created_at, completed_at
) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
ON CONFLICT(id) DO UPDATE SET
    session_id = COALESCE(NULLIF(excluded.session_id, ''), runtime_sandbox_decisions.session_id),
    turn_id = COALESCE(NULLIF(excluded.turn_id, ''), runtime_sandbox_decisions.turn_id),
    tool_call_id = COALESCE(NULLIF(excluded.tool_call_id, ''), runtime_sandbox_decisions.tool_call_id),
    task_id = COALESCE(NULLIF(excluded.task_id, ''), runtime_sandbox_decisions.task_id),
    mode = excluded.mode,
    status = excluded.status,
    executor = COALESCE(NULLIF(excluded.executor, ''), runtime_sandbox_decisions.executor),
    cwd = COALESCE(NULLIF(excluded.cwd, ''), runtime_sandbox_decisions.cwd),
    worktree_id = COALESCE(NULLIF(excluded.worktree_id, ''), runtime_sandbox_decisions.worktree_id),
    worktree_path = COALESCE(NULLIF(excluded.worktree_path, ''), runtime_sandbox_decisions.worktree_path),
    command_summary = COALESCE(NULLIF(excluded.command_summary, ''), runtime_sandbox_decisions.command_summary),
    policy_mode = COALESCE(NULLIF(excluded.policy_mode, ''), runtime_sandbox_decisions.policy_mode),
    policy_profile = COALESCE(NULLIF(excluded.policy_profile, ''), runtime_sandbox_decisions.policy_profile),
    policy_rule = COALESCE(NULLIF(excluded.policy_rule, ''), runtime_sandbox_decisions.policy_rule),
    reason = COALESCE(NULLIF(excluded.reason, ''), runtime_sandbox_decisions.reason),
    error = COALESCE(NULLIF(excluded.error, ''), runtime_sandbox_decisions.error),
    allowed_paths_json = COALESCE(NULLIF(excluded.allowed_paths_json, ''), runtime_sandbox_decisions.allowed_paths_json),
    denied_paths_json = COALESCE(NULLIF(excluded.denied_paths_json, ''), runtime_sandbox_decisions.denied_paths_json),
    network_allowed = excluded.network_allowed,
    network_reason = COALESCE(NULLIF(excluded.network_reason, ''), runtime_sandbox_decisions.network_reason),
    completed_at = COALESCE(excluded.completed_at, runtime_sandbox_decisions.completed_at)`,
		decision.ID, decision.SessionID, nullableString(decision.TurnID), nullableString(decision.ToolCallID), nullableString(decision.TaskID),
		decision.Mode, decision.Status, nullableString(decision.Executor), nullableString(decision.CWD), nullableString(decision.WorktreeID), nullableString(decision.WorktreePath),
		nullableString(decision.CommandSummary), nullableString(decision.PolicyMode), nullableString(decision.PolicyProfile), nullableString(decision.PolicyRule),
		nullableString(decision.Reason), nullableString(decision.Error), nullableString(encodeRuntimeStringRefs(decision.AllowedPaths)), nullableString(encodeRuntimeStringRefs(decision.DeniedPaths)),
		boolInt(decision.NetworkAllowed), nullableString(decision.NetworkReason), decision.CreatedAt, nullableSandboxInt64(decision.CompletedAt))
	if err != nil {
		return RuntimeSandboxDecision{}, fmt.Errorf("failed to upsert runtime sandbox decision: %w", err)
	}
	return s.Get(ctx, decision.ID)
}

func (s *runtimeSandboxDecisionStore) Get(ctx context.Context, id string) (RuntimeSandboxDecision, error) {
	id = strings.TrimSpace(id)
	if id == "" {
		return RuntimeSandboxDecision{}, errRuntimeSandboxDecisionNotFound
	}
	if s == nil {
		return RuntimeSandboxDecision{}, errRuntimeSandboxDecisionNotFound
	}
	if s.db == nil {
		s.mu.RLock()
		defer s.mu.RUnlock()
		decision, ok := s.mem[id]
		if !ok {
			return RuntimeSandboxDecision{}, errRuntimeSandboxDecisionNotFound
		}
		return decision, nil
	}
	row := s.db.QueryRowContext(ctx, `
SELECT id, session_id, turn_id, tool_call_id, task_id, mode, status, executor,
    cwd, worktree_id, worktree_path, command_summary, policy_mode, policy_profile,
    policy_rule, reason, error, allowed_paths_json, denied_paths_json,
    network_allowed, network_reason, created_at, completed_at
FROM runtime_sandbox_decisions WHERE id = ?`, id)
	decision, err := scanRuntimeSandboxDecision(row)
	if errors.Is(err, sql.ErrNoRows) {
		return RuntimeSandboxDecision{}, errRuntimeSandboxDecisionNotFound
	}
	return decision, err
}

func (s *runtimeSandboxDecisionStore) List(ctx context.Context, req RuntimeSandboxDecisionListRequest) ([]RuntimeSandboxDecision, error) {
	if s == nil {
		return nil, nil
	}
	if s.db == nil {
		s.mu.RLock()
		defer s.mu.RUnlock()
		items := make([]RuntimeSandboxDecision, 0, len(s.mem))
		for _, decision := range s.mem {
			if sandboxDecisionMatchesList(decision, req) {
				items = append(items, decision)
			}
		}
		sort.SliceStable(items, func(i, j int) bool { return items[i].CreatedAt < items[j].CreatedAt })
		return items, nil
	}
	query := `
SELECT id, session_id, turn_id, tool_call_id, task_id, mode, status, executor,
    cwd, worktree_id, worktree_path, command_summary, policy_mode, policy_profile,
    policy_rule, reason, error, allowed_paths_json, denied_paths_json,
    network_allowed, network_reason, created_at, completed_at
FROM runtime_sandbox_decisions`
	var clauses []string
	var args []any
	if req.SessionID != "" {
		clauses = append(clauses, "session_id = ?")
		args = append(args, req.SessionID)
	}
	if req.TurnID != "" {
		clauses = append(clauses, "turn_id = ?")
		args = append(args, req.TurnID)
	}
	if req.ToolCallID != "" {
		clauses = append(clauses, "tool_call_id = ?")
		args = append(args, req.ToolCallID)
	}
	if req.TaskID != "" {
		clauses = append(clauses, "task_id = ?")
		args = append(args, req.TaskID)
	}
	if len(clauses) > 0 {
		query += " WHERE " + strings.Join(clauses, " AND ")
	}
	query += " ORDER BY created_at ASC"
	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("failed to list runtime sandbox decisions: %w", err)
	}
	defer rows.Close() //nolint:errcheck
	var items []RuntimeSandboxDecision
	for rows.Next() {
		decision, err := scanRuntimeSandboxDecision(rows)
		if err != nil {
			return nil, err
		}
		items = append(items, decision)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("failed to iterate runtime sandbox decisions: %w", err)
	}
	return items, nil
}

func sandboxDecisionMatchesList(decision RuntimeSandboxDecision, req RuntimeSandboxDecisionListRequest) bool {
	if req.SessionID != "" && decision.SessionID != req.SessionID {
		return false
	}
	if req.TurnID != "" && decision.TurnID != req.TurnID {
		return false
	}
	if req.ToolCallID != "" && decision.ToolCallID != req.ToolCallID {
		return false
	}
	if req.TaskID != "" && decision.TaskID != req.TaskID {
		return false
	}
	return true
}

type runtimeSandboxDecisionScanner interface {
	Scan(dest ...any) error
}

func scanRuntimeSandboxDecision(scanner runtimeSandboxDecisionScanner) (RuntimeSandboxDecision, error) {
	var decision RuntimeSandboxDecision
	var turnID, toolCallID, taskID, executor, cwd, worktreeID, worktreePath, commandSummary, policyMode, policyProfile, policyRule, reason, errText, allowedJSON, deniedJSON, networkReason sql.NullString
	var networkAllowed int
	var completedAt sql.NullInt64
	if err := scanner.Scan(
		&decision.ID, &decision.SessionID, &turnID, &toolCallID, &taskID, &decision.Mode, &decision.Status, &executor,
		&cwd, &worktreeID, &worktreePath, &commandSummary, &policyMode, &policyProfile, &policyRule,
		&reason, &errText, &allowedJSON, &deniedJSON, &networkAllowed, &networkReason, &decision.CreatedAt, &completedAt,
	); err != nil {
		return RuntimeSandboxDecision{}, err
	}
	decision.TurnID = turnID.String
	decision.ToolCallID = toolCallID.String
	decision.TaskID = taskID.String
	decision.Executor = executor.String
	decision.CWD = cwd.String
	decision.WorktreeID = worktreeID.String
	decision.WorktreePath = worktreePath.String
	decision.CommandSummary = commandSummary.String
	decision.PolicyMode = policyMode.String
	decision.PolicyProfile = policyProfile.String
	decision.PolicyRule = policyRule.String
	decision.Reason = reason.String
	decision.Error = errText.String
	decision.AllowedPaths = decodeRuntimeStringRefs(allowedJSON.String)
	decision.DeniedPaths = decodeRuntimeStringRefs(deniedJSON.String)
	decision.NetworkAllowed = networkAllowed != 0
	decision.NetworkReason = networkReason.String
	if completedAt.Valid {
		decision.CompletedAt = completedAt.Int64
	}
	return decision, nil
}

func newRuntimeSandboxDecisionID() string {
	return "sandbox_" + newRequestID()
}

func nullableSandboxInt64(value int64) any {
	if value == 0 {
		return nil
	}
	return value
}

func (r *runtimeService) SandboxDecisions(ctx context.Context, req RuntimeSandboxDecisionListRequest) (RuntimeSandboxDecisionsResponse, error) {
	store, err := r.ensureSandboxDecisionStore(ctx)
	if err != nil {
		return RuntimeSandboxDecisionsResponse{}, err
	}
	items, err := store.List(ctx, req)
	if err != nil {
		return RuntimeSandboxDecisionsResponse{}, err
	}
	return RuntimeSandboxDecisionsResponse{Decisions: items}, nil
}

func (r *runtimeService) SandboxDecision(ctx context.Context, id string) (RuntimeSandboxDecisionResponse, error) {
	store, err := r.ensureSandboxDecisionStore(ctx)
	if err != nil {
		return RuntimeSandboxDecisionResponse{}, err
	}
	decision, err := store.Get(ctx, id)
	if err != nil {
		return RuntimeSandboxDecisionResponse{}, err
	}
	return RuntimeSandboxDecisionResponse{Decision: decision}, nil
}

func (r *runtimeService) ensureSandboxDecisionStore(ctx context.Context) (*runtimeSandboxDecisionStore, error) {
	if r.sandboxDecisions.db != nil || r.sandboxDecisions.mem != nil {
		return &r.sandboxDecisions, nil
	}
	if r.turns.db != nil {
		r.sandboxDecisions = newRuntimeSandboxDecisionStore(r.turns.db)
		return &r.sandboxDecisions, nil
	}
	db, err := r.workspaceDB(ctx)
	if err != nil {
		return nil, err
	}
	r.sandboxDecisions = newRuntimeSandboxDecisionStore(db)
	return &r.sandboxDecisions, nil
}
