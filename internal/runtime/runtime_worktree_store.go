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
	worktreeStatusCreated        = "created"
	worktreeStatusEntered        = "entered"
	worktreeStatusExited         = "exited"
	worktreeStatusCleanupPending = "cleanup_pending"
	worktreeStatusCleaned        = "cleaned"
	worktreeStatusPreserved      = "preserved"
	worktreeStatusInterrupted    = "interrupted"
	worktreeStatusMissing        = "missing_path"
	worktreeStatusError          = "error"

	worktreePreserveNever     = "never"
	worktreePreserveOnExit    = "on_exit"
	worktreePreserveOnFailure = "on_failure"
	worktreePreserveAlways    = "always"

	worktreeCleanupManual = "manual"
	worktreeCleanupExit   = "on_exit"
)

var errRuntimeWorktreeNotFound = errors.New("runtime worktree not found")

type runtimeWorktreeStore struct {
	db *sql.DB
}

func newRuntimeWorktreeStore(db *sql.DB) runtimeWorktreeStore {
	return runtimeWorktreeStore{db: db}
}

func (s runtimeWorktreeStore) Upsert(ctx context.Context, wt RuntimeWorktree) (RuntimeWorktree, error) {
	if s.db == nil {
		return RuntimeWorktree{}, errors.New("runtime worktree database is not available")
	}
	wt.ID = strings.TrimSpace(wt.ID)
	wt.SessionID = strings.TrimSpace(wt.SessionID)
	wt.BaseRepoPath = strings.TrimSpace(wt.BaseRepoPath)
	wt.WorktreePath = strings.TrimSpace(wt.WorktreePath)
	wt.Branch = strings.TrimSpace(wt.Branch)
	if wt.ID == "" {
		return RuntimeWorktree{}, errors.New("worktree id is required")
	}
	if wt.SessionID == "" {
		return RuntimeWorktree{}, errors.New("worktree session id is required")
	}
	if wt.BaseRepoPath == "" {
		return RuntimeWorktree{}, errors.New("worktree base repo path is required")
	}
	if wt.WorktreePath == "" {
		return RuntimeWorktree{}, errors.New("worktree path is required")
	}
	if wt.Branch == "" {
		return RuntimeWorktree{}, errors.New("worktree branch is required")
	}
	if wt.Status == "" {
		wt.Status = worktreeStatusCreated
	}
	if wt.PreservePolicy == "" {
		wt.PreservePolicy = worktreePreserveOnFailure
	}
	if wt.CleanupPolicy == "" {
		wt.CleanupPolicy = worktreeCleanupManual
	}
	now := time.Now().UnixMilli()
	if wt.CreatedAt == 0 {
		wt.CreatedAt = now
	}
	wt.UpdatedAt = now
	meta, err := encodeStringMap(wt.Metadata)
	if err != nil {
		return RuntimeWorktree{}, err
	}
	_, err = s.db.ExecContext(ctx, `
INSERT INTO runtime_worktrees (
    id, session_id, turn_id, task_id, base_repo_path, worktree_path, branch, ref,
    status, preserve_policy, cleanup_policy, created_at, entered_at, exited_at,
    cleaned_at, updated_at, error, owner, metadata_json
) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
ON CONFLICT(id) DO UPDATE SET
    session_id = COALESCE(NULLIF(excluded.session_id, ''), runtime_worktrees.session_id),
    turn_id = COALESCE(NULLIF(excluded.turn_id, ''), runtime_worktrees.turn_id),
    task_id = COALESCE(NULLIF(excluded.task_id, ''), runtime_worktrees.task_id),
    base_repo_path = COALESCE(NULLIF(excluded.base_repo_path, ''), runtime_worktrees.base_repo_path),
    worktree_path = COALESCE(NULLIF(excluded.worktree_path, ''), runtime_worktrees.worktree_path),
    branch = COALESCE(NULLIF(excluded.branch, ''), runtime_worktrees.branch),
    ref = COALESCE(NULLIF(excluded.ref, ''), runtime_worktrees.ref),
    status = excluded.status,
    preserve_policy = COALESCE(NULLIF(excluded.preserve_policy, ''), runtime_worktrees.preserve_policy),
    cleanup_policy = COALESCE(NULLIF(excluded.cleanup_policy, ''), runtime_worktrees.cleanup_policy),
    entered_at = COALESCE(excluded.entered_at, runtime_worktrees.entered_at),
    exited_at = COALESCE(excluded.exited_at, runtime_worktrees.exited_at),
    cleaned_at = COALESCE(excluded.cleaned_at, runtime_worktrees.cleaned_at),
    updated_at = excluded.updated_at,
    error = excluded.error,
    owner = COALESCE(NULLIF(excluded.owner, ''), runtime_worktrees.owner),
    metadata_json = COALESCE(excluded.metadata_json, runtime_worktrees.metadata_json)`,
		wt.ID,
		wt.SessionID,
		nullableString(wt.TurnID),
		nullableString(wt.TaskID),
		wt.BaseRepoPath,
		wt.WorktreePath,
		wt.Branch,
		nullableString(wt.Ref),
		wt.Status,
		wt.PreservePolicy,
		wt.CleanupPolicy,
		wt.CreatedAt,
		nullableInt64(wt.EnteredAt),
		nullableInt64(wt.ExitedAt),
		nullableInt64(wt.CleanedAt),
		wt.UpdatedAt,
		nullableString(wt.Error),
		nullableString(wt.Owner),
		meta,
	)
	if err != nil {
		return RuntimeWorktree{}, fmt.Errorf("failed to upsert runtime worktree: %w", err)
	}
	return s.Get(ctx, wt.ID)
}

func (s runtimeWorktreeStore) Get(ctx context.Context, id string) (RuntimeWorktree, error) {
	if s.db == nil {
		return RuntimeWorktree{}, errors.New("runtime worktree database is not available")
	}
	row := s.db.QueryRowContext(ctx, runtimeWorktreeSelectSQL()+` WHERE id = ?`, strings.TrimSpace(id))
	wt, err := scanRuntimeWorktree(row)
	if errors.Is(err, sql.ErrNoRows) {
		return RuntimeWorktree{}, errRuntimeWorktreeNotFound
	}
	return wt, err
}

func (s runtimeWorktreeStore) List(ctx context.Context) ([]RuntimeWorktree, error) {
	return s.list(ctx, `1 = 1`)
}

func (s runtimeWorktreeStore) ListBySession(ctx context.Context, sessionID string) ([]RuntimeWorktree, error) {
	return s.list(ctx, `session_id = ?`, strings.TrimSpace(sessionID))
}

func (s runtimeWorktreeStore) ListByTurn(ctx context.Context, turnID string) ([]RuntimeWorktree, error) {
	return s.list(ctx, `turn_id = ?`, strings.TrimSpace(turnID))
}

func (s runtimeWorktreeStore) ListByTask(ctx context.Context, taskID string) ([]RuntimeWorktree, error) {
	return s.list(ctx, `task_id = ?`, strings.TrimSpace(taskID))
}

func (s runtimeWorktreeStore) ListActive(ctx context.Context) ([]RuntimeWorktree, error) {
	return s.list(ctx, `status IN (?, ?, ?)`, worktreeStatusCreated, worktreeStatusEntered, worktreeStatusCleanupPending)
}

func (s runtimeWorktreeStore) list(ctx context.Context, where string, args ...any) ([]RuntimeWorktree, error) {
	if s.db == nil {
		return nil, errors.New("runtime worktree database is not available")
	}
	rows, err := s.db.QueryContext(ctx, runtimeWorktreeSelectSQL()+` WHERE `+where+` ORDER BY updated_at ASC`, args...)
	if err != nil {
		return nil, fmt.Errorf("failed to list runtime worktrees: %w", err)
	}
	defer rows.Close() //nolint:errcheck

	var out []RuntimeWorktree
	for rows.Next() {
		wt, err := scanRuntimeWorktree(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, wt)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("failed to iterate runtime worktrees: %w", err)
	}
	return out, nil
}

func runtimeWorktreeSelectSQL() string {
	return `
SELECT id, session_id, turn_id, task_id, base_repo_path, worktree_path, branch, ref,
    status, preserve_policy, cleanup_policy, created_at, entered_at, exited_at,
    cleaned_at, updated_at, error, owner, metadata_json
FROM runtime_worktrees`
}

type runtimeWorktreeScanner interface {
	Scan(dest ...any) error
}

func scanRuntimeWorktree(scanner runtimeWorktreeScanner) (RuntimeWorktree, error) {
	var wt RuntimeWorktree
	var turnID, taskID, ref, errText, owner, meta sql.NullString
	var enteredAt, exitedAt, cleanedAt sql.NullInt64
	if err := scanner.Scan(
		&wt.ID,
		&wt.SessionID,
		&turnID,
		&taskID,
		&wt.BaseRepoPath,
		&wt.WorktreePath,
		&wt.Branch,
		&ref,
		&wt.Status,
		&wt.PreservePolicy,
		&wt.CleanupPolicy,
		&wt.CreatedAt,
		&enteredAt,
		&exitedAt,
		&cleanedAt,
		&wt.UpdatedAt,
		&errText,
		&owner,
		&meta,
	); err != nil {
		return RuntimeWorktree{}, err
	}
	wt.TurnID = turnID.String
	wt.TaskID = taskID.String
	wt.Ref = ref.String
	if enteredAt.Valid {
		wt.EnteredAt = enteredAt.Int64
	}
	if exitedAt.Valid {
		wt.ExitedAt = exitedAt.Int64
	}
	if cleanedAt.Valid {
		wt.CleanedAt = cleanedAt.Int64
	}
	wt.Error = errText.String
	wt.Owner = owner.String
	wt.Metadata = decodeStringMap(meta.String)
	return wt, nil
}

func runtimeWorktreeFromPayload(payload map[string]any) RuntimeWorktree {
	wt := RuntimeWorktree{
		ID:             stringFromMap(payload, "id"),
		SessionID:      firstNonEmpty(stringFromMap(payload, "sessionId"), stringFromMap(payload, "session_id")),
		TurnID:         firstNonEmpty(stringFromMap(payload, "turnId"), stringFromMap(payload, "turn_id")),
		TaskID:         firstNonEmpty(stringFromMap(payload, "taskId"), stringFromMap(payload, "task_id")),
		BaseRepoPath:   firstNonEmpty(stringFromMap(payload, "baseRepoPath"), stringFromMap(payload, "base_repo_path")),
		WorktreePath:   firstNonEmpty(stringFromMap(payload, "worktreePath"), stringFromMap(payload, "worktree_path")),
		Branch:         stringFromMap(payload, "branch"),
		Ref:            stringFromMap(payload, "ref"),
		Status:         stringFromMap(payload, "status"),
		PreservePolicy: firstNonEmpty(stringFromMap(payload, "preservePolicy"), stringFromMap(payload, "preserve_policy")),
		CleanupPolicy:  firstNonEmpty(stringFromMap(payload, "cleanupPolicy"), stringFromMap(payload, "cleanup_policy")),
		Error:          stringFromMap(payload, "error"),
		Owner:          stringFromMap(payload, "owner"),
	}
	wt.CreatedAt = int64(intFromMap(payload, "createdAt"))
	wt.EnteredAt = int64(intFromMap(payload, "enteredAt"))
	wt.ExitedAt = int64(intFromMap(payload, "exitedAt"))
	wt.CleanedAt = int64(intFromMap(payload, "cleanedAt"))
	wt.UpdatedAt = int64(intFromMap(payload, "updatedAt"))
	if raw := asMap(payload["metadata"]); len(raw) > 0 {
		wt.Metadata = map[string]string{}
		for key, value := range raw {
			wt.Metadata[key] = fmt.Sprint(value)
		}
	} else if raw := stringFromMap(payload, "metadata_json"); strings.TrimSpace(raw) != "" {
		_ = json.Unmarshal([]byte(raw), &wt.Metadata)
	}
	return wt
}
