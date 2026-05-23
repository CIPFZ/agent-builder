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
	permissionStatusPending        = "pending"
	permissionStatusAllowedOnce    = "allowed_once"
	permissionStatusAllowedSession = "allowed_session"
	permissionStatusDenied         = "denied"
	permissionStatusExpired        = "expired"
	permissionStatusCancelled      = "cancelled"
)

var errRuntimePermissionNotFound = errors.New("runtime permission request not found")

type runtimePermissionStore struct {
	db *sql.DB
}

func newRuntimePermissionStore(db *sql.DB) runtimePermissionStore {
	return runtimePermissionStore{db: db}
}

func (s runtimePermissionStore) Upsert(ctx context.Context, perm RuntimePermissionRequest) (RuntimePermissionRequest, error) {
	if s.db == nil {
		return RuntimePermissionRequest{}, errors.New("runtime permission database is not available")
	}
	perm.ID = strings.TrimSpace(perm.ID)
	perm.SessionID = strings.TrimSpace(perm.SessionID)
	if perm.ID == "" {
		return RuntimePermissionRequest{}, errors.New("permission id is required")
	}
	if perm.SessionID == "" {
		return RuntimePermissionRequest{}, errors.New("permission session id is required")
	}
	if perm.Status == "" {
		perm.Status = permissionStatusPending
	}
	if perm.CreatedAt == 0 {
		perm.CreatedAt = time.Now().UnixMilli()
	}
	params, err := encodeRuntimePermissionParams(perm.Params)
	if err != nil {
		return RuntimePermissionRequest{}, err
	}
	_, err = s.db.ExecContext(ctx, `
INSERT INTO runtime_permission_requests (
    id, session_id, turn_id, tool_call_id, tool_name, description, action,
    params_json, path, target, risk, status, created_at, decided_at
) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
ON CONFLICT(id) DO UPDATE SET
    session_id = COALESCE(NULLIF(excluded.session_id, ''), runtime_permission_requests.session_id),
    turn_id = COALESCE(NULLIF(excluded.turn_id, ''), runtime_permission_requests.turn_id),
    tool_call_id = COALESCE(NULLIF(excluded.tool_call_id, ''), runtime_permission_requests.tool_call_id),
    tool_name = COALESCE(NULLIF(excluded.tool_name, ''), runtime_permission_requests.tool_name),
    description = COALESCE(NULLIF(excluded.description, ''), runtime_permission_requests.description),
    action = COALESCE(NULLIF(excluded.action, ''), runtime_permission_requests.action),
    params_json = COALESCE(excluded.params_json, runtime_permission_requests.params_json),
    path = COALESCE(NULLIF(excluded.path, ''), runtime_permission_requests.path),
    target = COALESCE(NULLIF(excluded.target, ''), runtime_permission_requests.target),
    risk = COALESCE(NULLIF(excluded.risk, ''), runtime_permission_requests.risk),
    status = excluded.status,
    created_at = runtime_permission_requests.created_at,
    decided_at = COALESCE(excluded.decided_at, runtime_permission_requests.decided_at)`,
		perm.ID,
		perm.SessionID,
		nullableString(perm.TurnID),
		nullableString(perm.ToolCallID),
		perm.ToolName,
		nullableString(perm.Description),
		perm.Action,
		params,
		nullableString(perm.Path),
		nullableString(firstNonEmpty(perm.Target, perm.Path)),
		nullableString(perm.Risk),
		perm.Status,
		perm.CreatedAt,
		nullableInt64(perm.DecidedAt),
	)
	if err != nil {
		return RuntimePermissionRequest{}, fmt.Errorf("failed to upsert runtime permission request: %w", err)
	}
	return s.Get(ctx, perm.ID)
}

func (s runtimePermissionStore) Get(ctx context.Context, id string) (RuntimePermissionRequest, error) {
	if s.db == nil {
		return RuntimePermissionRequest{}, errors.New("runtime permission database is not available")
	}
	row := s.db.QueryRowContext(ctx, `
SELECT id, session_id, turn_id, tool_call_id, tool_name, description, action,
    params_json, path, target, risk, status, created_at, decided_at
FROM runtime_permission_requests
WHERE id = ?`, strings.TrimSpace(id))
	perm, err := scanRuntimePermission(row)
	if errors.Is(err, sql.ErrNoRows) {
		return RuntimePermissionRequest{}, errRuntimePermissionNotFound
	}
	return perm, err
}

func (s runtimePermissionStore) List(ctx context.Context, status string) ([]RuntimePermissionRequest, error) {
	if s.db == nil {
		return nil, errors.New("runtime permission database is not available")
	}
	status = strings.TrimSpace(status)
	query := `
SELECT id, session_id, turn_id, tool_call_id, tool_name, description, action,
    params_json, path, target, risk, status, created_at, decided_at
FROM runtime_permission_requests`
	var args []any
	if status != "" {
		query += ` WHERE status = ?`
		args = append(args, status)
	}
	query += ` ORDER BY created_at ASC`
	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("failed to list runtime permission requests: %w", err)
	}
	defer rows.Close() //nolint:errcheck

	var permissions []RuntimePermissionRequest
	for rows.Next() {
		perm, err := scanRuntimePermission(rows)
		if err != nil {
			return nil, err
		}
		permissions = append(permissions, perm)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("failed to iterate runtime permission requests: %w", err)
	}
	return permissions, nil
}

func (s runtimePermissionStore) Mark(ctx context.Context, id, status string, decidedAt int64) (RuntimePermissionRequest, error) {
	perm, err := s.Get(ctx, id)
	if err != nil {
		return RuntimePermissionRequest{}, err
	}
	perm.Status = status
	if decidedAt == 0 && status != permissionStatusPending {
		decidedAt = time.Now().UnixMilli()
	}
	perm.DecidedAt = decidedAt
	return s.Upsert(ctx, perm)
}

type runtimePermissionScanner interface {
	Scan(dest ...any) error
}

func scanRuntimePermission(scanner runtimePermissionScanner) (RuntimePermissionRequest, error) {
	var perm RuntimePermissionRequest
	var turnID, toolCallID, description, paramsJSON, path, target, risk sql.NullString
	var decidedAt sql.NullInt64
	if err := scanner.Scan(
		&perm.ID,
		&perm.SessionID,
		&turnID,
		&toolCallID,
		&perm.ToolName,
		&description,
		&perm.Action,
		&paramsJSON,
		&path,
		&target,
		&risk,
		&perm.Status,
		&perm.CreatedAt,
		&decidedAt,
	); err != nil {
		return RuntimePermissionRequest{}, err
	}
	perm.TurnID = turnID.String
	perm.ToolCallID = toolCallID.String
	perm.Description = description.String
	perm.Params = decodeRuntimePermissionParams(paramsJSON.String)
	perm.Path = path.String
	perm.Target = firstNonEmpty(target.String, path.String)
	perm.Risk = risk.String
	if decidedAt.Valid {
		perm.DecidedAt = decidedAt.Int64
	}
	return perm, nil
}

func encodeRuntimePermissionParams(params any) (any, error) {
	if params == nil {
		return nil, nil
	}
	data, err := json.Marshal(params)
	if err != nil {
		return nil, fmt.Errorf("failed to encode runtime permission params: %w", err)
	}
	return string(data), nil
}

func decodeRuntimePermissionParams(data string) any {
	if strings.TrimSpace(data) == "" {
		return nil
	}
	var params any
	if err := json.Unmarshal([]byte(data), &params); err != nil {
		return nil
	}
	return params
}
