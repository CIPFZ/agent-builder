package runtime

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"
)

const (
	mcpRequestKindAuth        = "auth"
	mcpRequestKindElicitation = "elicitation"

	mcpRequestStatusNone        = "none"
	mcpRequestStatusNotRequired = "not_required"
	mcpRequestStatusRequired    = "required"
	mcpRequestStatusPending     = "pending"
	mcpRequestStatusApproved    = "approved"
	mcpRequestStatusCompleted   = "completed"
	mcpRequestStatusDenied      = "denied"
	mcpRequestStatusFailed      = "failed"
	mcpRequestStatusExpired     = "expired"
	mcpRequestStatusCancelled   = "cancelled"
)

var errRuntimeMCPRequestNotFound = errors.New("runtime mcp request not found")

type runtimeMCPRequestStore struct {
	db *sql.DB
}

func newRuntimeMCPRequestStore(db *sql.DB) runtimeMCPRequestStore {
	return runtimeMCPRequestStore{db: db}
}

func (s runtimeMCPRequestStore) Upsert(ctx context.Context, req RuntimeMCPRequest) (RuntimeMCPRequest, error) {
	if s.db == nil {
		return RuntimeMCPRequest{}, errors.New("runtime mcp request database is not available")
	}
	req = normalizeRuntimeMCPRequest(req)
	if req.ID == "" {
		return RuntimeMCPRequest{}, errors.New("mcp request id is required")
	}
	if req.Kind == "" {
		return RuntimeMCPRequest{}, errors.New("mcp request kind is required")
	}
	if req.Server == "" {
		return RuntimeMCPRequest{}, errors.New("mcp request server is required")
	}
	if req.Status == "" {
		req.Status = mcpRequestStatusPending
	}
	now := time.Now().UnixMilli()
	if req.CreatedAt == 0 {
		req.CreatedAt = now
	}
	if req.UpdatedAt == 0 {
		req.UpdatedAt = now
	}
	_, err := s.db.ExecContext(ctx, `
INSERT INTO runtime_mcp_requests (
    id, kind, server, capability_id, session_id, turn_id, status, prompt, description,
    response_summary, policy_mode, policy_profile, policy_decision, policy_reason,
    policy_risk, policy_rule_id, policy_rule_source, policy_scope_kind, policy_scope_value,
    policy_target_summary, policy_headless, policy_headless_reason, created_at, updated_at,
    expires_at, completed_at, error, redacted
) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
ON CONFLICT(id) DO UPDATE SET
    kind = COALESCE(NULLIF(excluded.kind, ''), runtime_mcp_requests.kind),
    server = COALESCE(NULLIF(excluded.server, ''), runtime_mcp_requests.server),
    capability_id = COALESCE(NULLIF(excluded.capability_id, ''), runtime_mcp_requests.capability_id),
    session_id = COALESCE(NULLIF(excluded.session_id, ''), runtime_mcp_requests.session_id),
    turn_id = COALESCE(NULLIF(excluded.turn_id, ''), runtime_mcp_requests.turn_id),
    status = excluded.status,
    prompt = COALESCE(NULLIF(excluded.prompt, ''), runtime_mcp_requests.prompt),
    description = COALESCE(NULLIF(excluded.description, ''), runtime_mcp_requests.description),
    response_summary = COALESCE(NULLIF(excluded.response_summary, ''), runtime_mcp_requests.response_summary),
    policy_mode = COALESCE(NULLIF(excluded.policy_mode, ''), runtime_mcp_requests.policy_mode),
    policy_profile = COALESCE(NULLIF(excluded.policy_profile, ''), runtime_mcp_requests.policy_profile),
    policy_decision = COALESCE(NULLIF(excluded.policy_decision, ''), runtime_mcp_requests.policy_decision),
    policy_reason = COALESCE(NULLIF(excluded.policy_reason, ''), runtime_mcp_requests.policy_reason),
    policy_risk = COALESCE(NULLIF(excluded.policy_risk, ''), runtime_mcp_requests.policy_risk),
    policy_rule_id = COALESCE(NULLIF(excluded.policy_rule_id, ''), runtime_mcp_requests.policy_rule_id),
    policy_rule_source = COALESCE(NULLIF(excluded.policy_rule_source, ''), runtime_mcp_requests.policy_rule_source),
    policy_scope_kind = COALESCE(NULLIF(excluded.policy_scope_kind, ''), runtime_mcp_requests.policy_scope_kind),
    policy_scope_value = COALESCE(NULLIF(excluded.policy_scope_value, ''), runtime_mcp_requests.policy_scope_value),
    policy_target_summary = COALESCE(NULLIF(excluded.policy_target_summary, ''), runtime_mcp_requests.policy_target_summary),
    policy_headless = CASE WHEN excluded.policy_headless != 0 THEN excluded.policy_headless ELSE runtime_mcp_requests.policy_headless END,
    policy_headless_reason = COALESCE(NULLIF(excluded.policy_headless_reason, ''), runtime_mcp_requests.policy_headless_reason),
    updated_at = excluded.updated_at,
    expires_at = COALESCE(excluded.expires_at, runtime_mcp_requests.expires_at),
    completed_at = COALESCE(excluded.completed_at, runtime_mcp_requests.completed_at),
    error = COALESCE(NULLIF(excluded.error, ''), runtime_mcp_requests.error),
    redacted = 1`,
		req.ID,
		req.Kind,
		req.Server,
		nullableString(req.CapabilityID),
		nullableString(req.SessionID),
		nullableString(req.TurnID),
		req.Status,
		nullableString(req.Prompt),
		nullableString(req.Description),
		nullableString(req.ResponseSummary),
		nullableString(req.PolicyMode),
		nullableString(req.PolicyProfile),
		nullableString(req.PolicyDecision),
		nullableString(req.PolicyReason),
		nullableString(req.PolicyRisk),
		nullableString(req.PolicyRuleID),
		nullableString(req.PolicyRuleSource),
		nullableString(req.PolicyScopeKind),
		nullableString(req.PolicyScopeValue),
		nullableString(req.PolicyTargetSummary),
		boolInt(req.PolicyHeadless),
		nullableString(req.PolicyHeadlessReason),
		req.CreatedAt,
		req.UpdatedAt,
		nullableInt64(req.ExpiresAt),
		nullableInt64(req.CompletedAt),
		nullableString(req.Error),
		boolInt(req.Redacted),
	)
	if err != nil {
		return RuntimeMCPRequest{}, fmt.Errorf("failed to upsert runtime mcp request: %w", err)
	}
	return s.Get(ctx, req.ID)
}

func (s runtimeMCPRequestStore) Get(ctx context.Context, id string) (RuntimeMCPRequest, error) {
	if s.db == nil {
		return RuntimeMCPRequest{}, errors.New("runtime mcp request database is not available")
	}
	row := s.db.QueryRowContext(ctx, `
SELECT id, kind, server, capability_id, session_id, turn_id, status, prompt, description,
    response_summary, policy_mode, policy_profile, policy_decision, policy_reason, policy_risk,
    policy_rule_id, policy_rule_source, policy_scope_kind, policy_scope_value, policy_target_summary,
    policy_headless, policy_headless_reason, created_at, updated_at, expires_at, completed_at, error, redacted
FROM runtime_mcp_requests
WHERE id = ?`, strings.TrimSpace(id))
	req, err := scanRuntimeMCPRequest(row)
	if errors.Is(err, sql.ErrNoRows) {
		return RuntimeMCPRequest{}, errRuntimeMCPRequestNotFound
	}
	return req, err
}

func (s runtimeMCPRequestStore) List(ctx context.Context, filter RuntimeMCPRequestListRequest) ([]RuntimeMCPRequest, error) {
	if s.db == nil {
		return nil, errors.New("runtime mcp request database is not available")
	}
	query := `
SELECT id, kind, server, capability_id, session_id, turn_id, status, prompt, description,
    response_summary, policy_mode, policy_profile, policy_decision, policy_reason, policy_risk,
    policy_rule_id, policy_rule_source, policy_scope_kind, policy_scope_value, policy_target_summary,
    policy_headless, policy_headless_reason, created_at, updated_at, expires_at, completed_at, error, redacted
FROM runtime_mcp_requests`
	var where []string
	var args []any
	if strings.TrimSpace(filter.Kind) != "" {
		where = append(where, "kind = ?")
		args = append(args, strings.TrimSpace(filter.Kind))
	}
	if strings.TrimSpace(filter.Status) != "" {
		where = append(where, "status = ?")
		args = append(args, strings.TrimSpace(filter.Status))
	}
	if strings.TrimSpace(filter.Server) != "" {
		where = append(where, "server = ?")
		args = append(args, strings.TrimSpace(filter.Server))
	}
	if len(where) > 0 {
		query += " WHERE " + strings.Join(where, " AND ")
	}
	query += " ORDER BY created_at ASC"
	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("failed to list runtime mcp requests: %w", err)
	}
	defer rows.Close() //nolint:errcheck

	var requests []RuntimeMCPRequest
	for rows.Next() {
		req, err := scanRuntimeMCPRequest(rows)
		if err != nil {
			return nil, err
		}
		requests = append(requests, req)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("failed to iterate runtime mcp requests: %w", err)
	}
	return requests, nil
}

func (s runtimeMCPRequestStore) Mark(ctx context.Context, id, status, responseSummary, errText string) (RuntimeMCPRequest, error) {
	req, err := s.Get(ctx, id)
	if err != nil {
		return RuntimeMCPRequest{}, err
	}
	now := time.Now().UnixMilli()
	req.Status = strings.TrimSpace(status)
	req.ResponseSummary = redactRuntimeString("response_summary", responseSummary)
	req.Error = redactRuntimeString("error", errText)
	req.UpdatedAt = now
	if req.Status != mcpRequestStatusPending && req.Status != mcpRequestStatusRequired {
		req.CompletedAt = now
	}
	req.Redacted = true
	return s.Upsert(ctx, req)
}

func (s runtimeMCPRequestStore) CancelActionableOnStartup(ctx context.Context, reason string) ([]RuntimeMCPRequest, error) {
	if s.db == nil {
		return nil, errors.New("runtime mcp request database is not available")
	}
	var cancelled []RuntimeMCPRequest
	for _, status := range []string{mcpRequestStatusPending, mcpRequestStatusRequired} {
		requests, err := s.List(ctx, RuntimeMCPRequestListRequest{Status: status})
		if err != nil {
			return nil, err
		}
		for _, req := range requests {
			updated, err := s.Mark(ctx, req.ID, mcpRequestStatusCancelled, "", reason)
			if err != nil {
				return nil, err
			}
			cancelled = append(cancelled, updated)
		}
	}
	return cancelled, nil
}

type runtimeMCPRequestScanner interface {
	Scan(dest ...any) error
}

func scanRuntimeMCPRequest(scanner runtimeMCPRequestScanner) (RuntimeMCPRequest, error) {
	var req RuntimeMCPRequest
	var capabilityID, sessionID, turnID, prompt, description, responseSummary sql.NullString
	var policyMode, policyProfile, policyDecision, policyReason, policyRisk sql.NullString
	var policyRuleID, policyRuleSource, policyScopeKind, policyScopeValue, policyTargetSummary sql.NullString
	var policyHeadlessReason, errText sql.NullString
	var expiresAt, completedAt sql.NullInt64
	var policyHeadless, redacted int
	if err := scanner.Scan(
		&req.ID,
		&req.Kind,
		&req.Server,
		&capabilityID,
		&sessionID,
		&turnID,
		&req.Status,
		&prompt,
		&description,
		&responseSummary,
		&policyMode,
		&policyProfile,
		&policyDecision,
		&policyReason,
		&policyRisk,
		&policyRuleID,
		&policyRuleSource,
		&policyScopeKind,
		&policyScopeValue,
		&policyTargetSummary,
		&policyHeadless,
		&policyHeadlessReason,
		&req.CreatedAt,
		&req.UpdatedAt,
		&expiresAt,
		&completedAt,
		&errText,
		&redacted,
	); err != nil {
		return RuntimeMCPRequest{}, err
	}
	req.CapabilityID = capabilityID.String
	req.SessionID = sessionID.String
	req.TurnID = turnID.String
	req.Prompt = prompt.String
	req.Description = description.String
	req.ResponseSummary = responseSummary.String
	req.PolicyMode = policyMode.String
	req.PolicyProfile = policyProfile.String
	req.PolicyDecision = policyDecision.String
	req.PolicyReason = policyReason.String
	req.PolicyRisk = policyRisk.String
	req.PolicyRuleID = policyRuleID.String
	req.PolicyRuleSource = policyRuleSource.String
	req.PolicyScopeKind = policyScopeKind.String
	req.PolicyScopeValue = policyScopeValue.String
	req.PolicyTargetSummary = policyTargetSummary.String
	req.PolicyHeadless = policyHeadless != 0
	req.PolicyHeadlessReason = policyHeadlessReason.String
	if expiresAt.Valid {
		req.ExpiresAt = expiresAt.Int64
	}
	if completedAt.Valid {
		req.CompletedAt = completedAt.Int64
	}
	req.Error = errText.String
	req.Redacted = redacted != 0
	return normalizeRuntimeMCPRequest(req), nil
}

func normalizeRuntimeMCPRequest(req RuntimeMCPRequest) RuntimeMCPRequest {
	req.ID = strings.TrimSpace(req.ID)
	req.Kind = strings.TrimSpace(req.Kind)
	req.Server = strings.TrimSpace(req.Server)
	req.CapabilityID = strings.TrimSpace(req.CapabilityID)
	req.SessionID = strings.TrimSpace(req.SessionID)
	req.TurnID = strings.TrimSpace(req.TurnID)
	req.Status = strings.TrimSpace(req.Status)
	req.Prompt = redactRuntimeString("prompt", preview(req.Prompt, 500))
	req.Description = redactRuntimeString("description", preview(req.Description, 500))
	req.ResponseSummary = redactRuntimeString("response_summary", preview(req.ResponseSummary, 500))
	req.PolicyReason = redactRuntimeString("policy_reason", preview(req.PolicyReason, 500))
	req.PolicyHeadlessReason = redactRuntimeString("policy_headless_reason", preview(req.PolicyHeadlessReason, 500))
	req.Error = redactRuntimeString("error", preview(req.Error, 500))
	req.Redacted = true
	return req
}

func mcpRequestStatusTerminal(status string) bool {
	switch status {
	case mcpRequestStatusCompleted, mcpRequestStatusApproved, mcpRequestStatusDenied, mcpRequestStatusFailed, mcpRequestStatusExpired, mcpRequestStatusCancelled:
		return true
	default:
		return false
	}
}
