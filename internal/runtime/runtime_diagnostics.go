package runtime

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	goruntime "runtime"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/CIPFZ/agent-builder/internal/db"
	"github.com/CIPFZ/agent-builder/internal/version"
)

const (
	diagnosticKindProviderFailure    = "provider_failure"
	diagnosticKindTurnInterrupted    = "turn_interrupted"
	diagnosticKindToolFailure        = "tool_failure"
	diagnosticKindPersistenceFailure = "persistence_failure"

	diagnosticCheckProvider = "provider_connection"
	diagnosticCheckPath     = "path_access"
	diagnosticCheckSQLite   = "sqlite_quick_check"

	maxPersistenceDiagnostics = 32
)

type runtimePersistenceDiagnostic struct {
	ID        string
	Operation string
	ErrorCode string
	Summary   string
	TurnID    string
	CreatedAt time.Time
}

type diagnosticToolFact struct {
	ID, TurnID, SessionID, Name, Status, OutputSummary, Error string
	ParentStatus                                              string
	PolicyReason, SandboxStatus, SandboxReason                string
	ExitCode                                                  int
	IsError                                                   bool
	StartedAt, FinishedAt                                     int64
}

func (r *runtimeService) recordPersistenceDiagnostic(operation, errorCode string, err error, turnID string) {
	if err == nil {
		return
	}
	now := time.Now().UTC()
	record := runtimePersistenceDiagnostic{
		ID:        "persistence:" + operation + ":" + strconv.FormatInt(now.UnixNano(), 36),
		Operation: strings.TrimSpace(operation),
		ErrorCode: firstNonEmpty(strings.TrimSpace(errorCode), classifyPersistenceErrorCode(err)),
		Summary:   preview(err.Error(), 480),
		TurnID:    strings.TrimSpace(turnID),
		CreatedAt: now,
	}
	r.diagnosticMu.Lock()
	r.persistenceDiagnostics = append(r.persistenceDiagnostics, record)
	if len(r.persistenceDiagnostics) > maxPersistenceDiagnostics {
		r.persistenceDiagnostics = append([]runtimePersistenceDiagnostic(nil), r.persistenceDiagnostics[len(r.persistenceDiagnostics)-maxPersistenceDiagnostics:]...)
	}
	r.diagnosticMu.Unlock()
}

func classifyPersistenceErrorCode(err error) string {
	lower := strings.ToLower(fmt.Sprint(err))
	switch {
	case strings.Contains(lower, "readonly") || strings.Contains(lower, "read-only"):
		return "db_readonly"
	case strings.Contains(lower, "locked") || strings.Contains(lower, "busy"):
		return "db_locked"
	case strings.Contains(lower, "disk") && (strings.Contains(lower, "full") || strings.Contains(lower, "space")):
		return "disk_full"
	case strings.Contains(lower, "schema") || strings.Contains(lower, "migration") || strings.Contains(lower, "no such table"):
		return "db_schema"
	case strings.Contains(lower, "open") || strings.Contains(lower, "connect") || strings.Contains(lower, "database is not available"):
		return "db_open"
	default:
		return "db_write"
	}
}

func isPersistenceDiagnosticError(err error) bool {
	lower := strings.ToLower(fmt.Sprint(err))
	for _, marker := range []string{"database", "sqlite", "schema", "migration", "readonly", "read-only", "locked", "disk full", "runtime state store"} {
		if strings.Contains(lower, marker) {
			return true
		}
	}
	return false
}

func (r *runtimeService) DiagnosticIncidents(ctx context.Context, req RuntimeDiagnosticIncidentsRequest) (RuntimeDiagnosticIncidentsResponse, error) {
	limit := req.Limit
	if limit <= 0 {
		limit = 100
	}
	if limit > 100 {
		limit = 100
	}
	conn, err := r.configDB(ctx)
	if err != nil {
		r.recordPersistenceDiagnostic("diagnostic_database_open", classifyPersistenceErrorCode(err), err, "")
		return r.projectPersistenceDiagnostics(req, limit), nil
	}
	layout, _ := resolveDesktopLayout()
	defer db.Release(layout.DataDir) //nolint:errcheck

	incidents, err := r.queryDiagnosticIncidents(ctx, conn, req, limit)
	if err != nil {
		r.recordPersistenceDiagnostic("diagnostic_query", classifyPersistenceErrorCode(err), err, "")
		fallback := r.projectPersistenceDiagnostics(req, limit)
		if len(fallback.Incidents) > 0 {
			return fallback, nil
		}
		return RuntimeDiagnosticIncidentsResponse{}, err
	}
	incidents = append(incidents, r.projectPersistenceDiagnosticItems(req)...)
	sort.SliceStable(incidents, func(i, j int) bool { return incidents[i].LastObservedAt > incidents[j].LastObservedAt })
	return finalizeDiagnosticResponse(incidents, limit), nil
}

func (r *runtimeService) DiagnosticIncident(ctx context.Context, incidentID string) (RuntimeDiagnosticIncident, error) {
	incidentID = strings.TrimSpace(incidentID)
	if incidentID == "" {
		return RuntimeDiagnosticIncident{}, errors.New("incident id is required")
	}
	resp, err := r.DiagnosticIncidents(ctx, RuntimeDiagnosticIncidentsRequest{Limit: 100})
	if err != nil {
		return RuntimeDiagnosticIncident{}, err
	}
	for _, incident := range resp.Incidents {
		if incident.ID == incidentID {
			return incident, nil
		}
	}
	return RuntimeDiagnosticIncident{}, fmt.Errorf("diagnostic incident %s was not found", incidentID)
}

func (r *runtimeService) queryDiagnosticIncidents(ctx context.Context, conn *sql.DB, req RuntimeDiagnosticIncidentsRequest, limit int) ([]RuntimeDiagnosticIncident, error) {
	before := diagnosticBeforeMillis(req.Before)
	args := []any{turnStatusFailed, turnStatusInterrupted, before}
	where := `status IN (?, ?) AND updated_at < ?`
	if sessionID := strings.TrimSpace(req.SessionID); sessionID != "" {
		where += ` AND session_id = ?`
		args = append(args, sessionID)
	}
	args = append(args, limit*3+10)
	var turns []RuntimeTurn
	if err := queryRuntimeRows(ctx, conn, `SELECT id, session_id, status, COALESCE(provider,''), COALESCE(model,''), COALESCE(error,''), started_at, updated_at, COALESCE(finished_at,0) FROM runtime_turns WHERE `+where+` ORDER BY updated_at DESC LIMIT ?`, func(rows *sql.Rows) error {
		var turn RuntimeTurn
		var updated int64
		if err := rows.Scan(&turn.ID, &turn.SessionID, &turn.Status, &turn.Provider, &turn.Model, &turn.Error, &turn.StartedAt, &updated, &turn.FinishedAt); err != nil {
			return err
		}
		turns = append(turns, turn)
		return nil
	}, args...); err != nil {
		return nil, fmt.Errorf("query diagnostic turns: %w", err)
	}

	turnIDs := make([]string, 0, len(turns))
	turnByID := make(map[string]RuntimeTurn, len(turns))
	for _, turn := range turns {
		turnIDs = append(turnIDs, turn.ID)
		turnByID[turn.ID] = turn
	}
	tools, err := queryDiagnosticTools(ctx, conn, turnIDs, req.SessionID, before, limit*3+10)
	if err != nil {
		return nil, err
	}
	toolIDs := make([]string, 0, len(tools))
	for _, tool := range tools {
		toolIDs = append(toolIDs, tool.ID)
	}
	evidenceByTurn, evidenceByTool, recoveredTurns, lastObserved, err := queryDiagnosticEvidence(ctx, conn, turnIDs, toolIDs, turnByID)
	if err != nil {
		return nil, err
	}

	incidents := make([]RuntimeDiagnosticIncident, 0, len(turns)+len(tools))
	incidentTurnIDs := map[string]struct{}{}
	for _, turn := range turns {
		incident, ok := buildTurnDiagnosticIncident(turn, evidenceByTurn[turn.ID], recoveredTurns[turn.ID], lastObserved[turn.ID])
		if !ok || !diagnosticIncidentMatches(incident, req) {
			continue
		}
		incidentTurnIDs[turn.ID] = struct{}{}
		incidents = append(incidents, incident)
	}
	for _, tool := range tools {
		if _, merged := incidentTurnIDs[tool.TurnID]; merged {
			continue
		}
		if tool.ParentStatus == turnStatusCompleted {
			continue
		}
		incident, ok := buildToolDiagnosticIncident(tool, evidenceByTool[tool.ID])
		if ok && diagnosticIncidentMatches(incident, req) {
			incidents = append(incidents, incident)
		}
	}
	return incidents, nil
}

func queryDiagnosticTools(ctx context.Context, conn *sql.DB, turnIDs []string, sessionID string, before int64, limit int) ([]diagnosticToolFact, error) {
	where := `c.status = 'failed' AND c.started_at < ?`
	args := []any{before}
	if strings.TrimSpace(sessionID) != "" {
		where += ` AND c.session_id = ?`
		args = append(args, strings.TrimSpace(sessionID))
	}
	args = append(args, limit)
	rows, err := conn.QueryContext(ctx, `SELECT c.id, c.turn_id, c.session_id, c.name, c.status, COALESCE(c.output_summary,''), COALESCE(c.error,''), COALESCE(c.policy_reason,''), COALESCE(c.sandbox_status,''), COALESCE(c.sandbox_reason,''), c.exit_code, c.is_error, c.started_at, COALESCE(c.finished_at,0), COALESCE(t.status,'') FROM runtime_tool_calls c LEFT JOIN runtime_turns t ON t.id=c.turn_id WHERE `+where+` ORDER BY c.started_at DESC LIMIT ?`, args...)
	if err != nil {
		return nil, fmt.Errorf("query diagnostic tool calls: %w", err)
	}
	defer rows.Close() //nolint:errcheck
	var tools []diagnosticToolFact
	for rows.Next() {
		var item diagnosticToolFact
		if err := rows.Scan(&item.ID, &item.TurnID, &item.SessionID, &item.Name, &item.Status, &item.OutputSummary, &item.Error, &item.PolicyReason, &item.SandboxStatus, &item.SandboxReason, &item.ExitCode, &item.IsError, &item.StartedAt, &item.FinishedAt, &item.ParentStatus); err != nil {
			return nil, err
		}
		if strings.Contains(strings.ToLower(item.PolicyReason), "denied") || strings.EqualFold(item.SandboxStatus, "denied") {
			continue
		}
		tools = append(tools, item)
	}
	return tools, rows.Err()
}

func queryDiagnosticEvidence(ctx context.Context, conn *sql.DB, turnIDs, toolIDs []string, turns map[string]RuntimeTurn) (map[string][]RuntimeDiagnosticEvidence, map[string][]RuntimeDiagnosticEvidence, map[string]bool, map[string]string, error) {
	byTurn := map[string][]RuntimeDiagnosticEvidence{}
	byTool := map[string][]RuntimeDiagnosticEvidence{}
	recovered := map[string]bool{}
	last := map[string]string{}
	for _, turn := range turns {
		ts := millisRFC3339(firstNonZeroInt64(turn.FinishedAt, turn.StartedAt))
		byTurn[turn.ID] = append(byTurn[turn.ID], RuntimeDiagnosticEvidence{ID: "turn:" + turn.ID, Source: "runtime_turns", Kind: turn.Status, Label: "Turn 状态", Summary: firstNonEmpty(preview(turn.Error, 240), turn.Status), Timestamp: ts, SessionID: turn.SessionID, TurnID: turn.ID, Metadata: nonEmptyMetadata(map[string]string{"provider": turn.Provider, "model": turn.Model, "status": turn.Status})})
		last[turn.ID] = ts
	}
	if len(turnIDs) == 0 && len(toolIDs) == 0 {
		return byTurn, byTool, recovered, last, nil
	}
	turnClause, turnArgs := sqlInClause(turnIDs)
	toolClause, toolArgs := sqlInClause(toolIDs)
	if len(turnIDs) > 0 {
		rows, err := conn.QueryContext(ctx, `SELECT id,type,COALESCE(session_id,''),COALESCE(turn_id,''),COALESCE(tool_call_id,''),created_at FROM runtime_events WHERE turn_id IN (`+turnClause+`) ORDER BY sequence ASC`, turnArgs...)
		if err != nil {
			return nil, nil, nil, nil, err
		}
		if err := consumeRuntimeRows(rows, func(rows *sql.Rows) error {
			var id, kind, sessionID, turnID, toolID, created string
			if err := rows.Scan(&id, &kind, &sessionID, &turnID, &toolID, &created); err != nil {
				return err
			}
			if !isRelevantDiagnosticEvent(kind) {
				return nil
			}
			byTurn[turnID] = append(byTurn[turnID], RuntimeDiagnosticEvidence{ID: id, Source: "runtime_events", Kind: kind, Label: "Runtime 事件", Summary: kind, Timestamp: created, SessionID: sessionID, TurnID: turnID, ToolCallID: toolID})
			last[turnID] = laterTimestamp(last[turnID], created)
			return nil
		}); err != nil {
			return nil, nil, nil, nil, err
		}

		rows, err = conn.QueryContext(ctx, `SELECT id,type,COALESCE(session_id,''),COALESCE(turn_id,''),COALESCE(tool_call_id,''),created_at FROM runtime_audit_events WHERE turn_id IN (`+turnClause+`) ORDER BY created_at ASC`, turnArgs...)
		if err != nil {
			return nil, nil, nil, nil, err
		}
		if err := consumeRuntimeRows(rows, func(rows *sql.Rows) error {
			var id, kind, sessionID, turnID, toolID, created string
			if err := rows.Scan(&id, &kind, &sessionID, &turnID, &toolID, &created); err != nil {
				return err
			}
			if !isRelevantDiagnosticAudit(kind) {
				return nil
			}
			byTurn[turnID] = append(byTurn[turnID], RuntimeDiagnosticEvidence{ID: id, Source: "runtime_audit_events", Kind: kind, Label: "审计记录", Summary: kind, Timestamp: created, SessionID: sessionID, TurnID: turnID, ToolCallID: toolID})
			last[turnID] = laterTimestamp(last[turnID], created)
			return nil
		}); err != nil {
			return nil, nil, nil, nil, err
		}

		rows, err = conn.QueryContext(ctx, `SELECT l.id,l.source_turn_id,l.resumed_turn_id,l.action,l.created_at,COALESCE(t.status,'') FROM runtime_recovery_links l LEFT JOIN runtime_turns t ON t.id=l.resumed_turn_id WHERE l.source_turn_id IN (`+turnClause+`) ORDER BY l.created_at ASC`, turnArgs...)
		if err != nil {
			return nil, nil, nil, nil, err
		}
		if err := consumeRuntimeRows(rows, func(rows *sql.Rows) error {
			var id, sourceID, resumedID, action, created, status string
			if err := rows.Scan(&id, &sourceID, &resumedID, &action, &created, &status); err != nil {
				return err
			}
			byTurn[sourceID] = append(byTurn[sourceID], RuntimeDiagnosticEvidence{ID: id, Source: "runtime_recovery_links", Kind: "recovery", Label: "恢复记录", Summary: action, Timestamp: created, TurnID: sourceID, Metadata: map[string]string{"resumedTurnId": resumedID, "resumedStatus": status}})
			if status == turnStatusCompleted {
				recovered[sourceID] = true
			}
			last[sourceID] = laterTimestamp(last[sourceID], created)
			return nil
		}); err != nil {
			return nil, nil, nil, nil, err
		}

		for _, spec := range []struct{ query, source, label string }{
			{`SELECT id,status,COALESCE(session_id,''),COALESCE(turn_id,''),COALESCE(tool_call_id,''),COALESCE(error,reason,''),started_at FROM runtime_hook_executions WHERE turn_id IN (` + turnClause + `)`, "runtime_hook_executions", "Hook 证据"},
			{`SELECT id,status,COALESCE(session_id,''),COALESCE(turn_id,''),'',COALESCE(error,response_summary,''),created_at FROM runtime_mcp_requests WHERE turn_id IN (` + turnClause + `)`, "runtime_mcp_requests", "MCP 证据"},
			{`SELECT id,status,session_id,COALESCE(turn_id,''),COALESCE(tool_call_id,''),COALESCE(policy_reason,decision,''),created_at FROM runtime_permission_requests WHERE turn_id IN (` + turnClause + `)`, "runtime_permission_requests", "权限证据"},
			{`SELECT id,status,session_id,COALESCE(turn_id,''),COALESCE(tool_call_id,''),COALESCE(error,reason,''),created_at FROM runtime_sandbox_decisions WHERE turn_id IN (` + turnClause + `)`, "runtime_sandbox_decisions", "Sandbox 证据"},
		} {
			rows, err = conn.QueryContext(ctx, spec.query, turnArgs...)
			if err != nil {
				return nil, nil, nil, nil, err
			}
			if err := consumeRuntimeRows(rows, func(rows *sql.Rows) error {
				var id, kind, sessionID, turnID, toolID, summary string
				var created int64
				if err := rows.Scan(&id, &kind, &sessionID, &turnID, &toolID, &summary, &created); err != nil {
					return err
				}
				if !isRelevantAuxiliaryDiagnosticEvidence(kind, summary) {
					return nil
				}
				ev := RuntimeDiagnosticEvidence{ID: id, Source: spec.source, Kind: kind, Label: spec.label, Summary: preview(summary, 180), Timestamp: millisRFC3339(created), SessionID: sessionID, TurnID: turnID, ToolCallID: toolID}
				byTurn[turnID] = append(byTurn[turnID], ev)
				if toolID != "" {
					byTool[toolID] = append(byTool[toolID], ev)
				}
				return nil
			}); err != nil {
				return nil, nil, nil, nil, err
			}
		}
	}
	if len(toolIDs) > 0 {
		rows, err := conn.QueryContext(ctx, `SELECT id,turn_id,session_id,name,status,COALESCE(output_summary,''),COALESCE(error,''),exit_code,started_at,COALESCE(finished_at,0) FROM runtime_tool_calls WHERE id IN (`+toolClause+`)`, toolArgs...)
		if err != nil {
			return nil, nil, nil, nil, err
		}
		if err := consumeRuntimeRows(rows, func(rows *sql.Rows) error {
			var id, turnID, sessionID, name, status, output, errText string
			var exitCode int
			var started, finished int64
			if err := rows.Scan(&id, &turnID, &sessionID, &name, &status, &output, &errText, &exitCode, &started, &finished); err != nil {
				return err
			}
			ev := RuntimeDiagnosticEvidence{ID: "tool:" + id, Source: "runtime_tool_calls", Kind: status, Label: "工具执行", Summary: firstNonEmpty(preview(errText, 200), preview(output, 200), status), Timestamp: millisRFC3339(firstNonZeroInt64(finished, started)), SessionID: sessionID, TurnID: turnID, ToolCallID: id, Metadata: nonEmptyMetadata(map[string]string{"tool": name, "exitCode": strconv.Itoa(exitCode)})}
			byTool[id] = append(byTool[id], ev)
			if turnID != "" {
				byTurn[turnID] = append(byTurn[turnID], ev)
			}
			return nil
		}); err != nil {
			return nil, nil, nil, nil, err
		}

		rows, err = conn.QueryContext(ctx, `SELECT id,COALESCE(turn_id,''),COALESCE(tool_call_id,''),path,status,created_at FROM runtime_permission_requests WHERE tool_call_id IN (`+toolClause+`) AND path IS NOT NULL AND path <> ''`, toolArgs...)
		if err != nil {
			return nil, nil, nil, nil, err
		}
		if err := consumeRuntimeRows(rows, func(rows *sql.Rows) error {
			var id, turnID, toolID, path, status string
			var created int64
			if err := rows.Scan(&id, &turnID, &toolID, &path, &status, &created); err != nil {
				return err
			}
			ev := RuntimeDiagnosticEvidence{ID: id + ":path", Source: "runtime_permission_requests", Kind: "path", Label: "引用路径", Summary: "故障证据包含一个明确路径", Timestamp: millisRFC3339(created), TurnID: turnID, ToolCallID: toolID, Metadata: map[string]string{"path": path, "status": status}}
			byTool[toolID] = append(byTool[toolID], ev)
			if turnID != "" {
				byTurn[turnID] = append(byTurn[turnID], ev)
			}
			return nil
		}); err != nil {
			return nil, nil, nil, nil, err
		}
	}
	return byTurn, byTool, recovered, last, nil
}

func buildTurnDiagnosticIncident(turn RuntimeTurn, evidence []RuntimeDiagnosticEvidence, recovered bool, lastObserved string) (RuntimeDiagnosticIncident, bool) {
	evidence = compactDiagnosticEvidence(evidence, 5)
	if turn.Status == turnStatusCancelled || strings.Contains(strings.ToLower(turn.Error), "user cancel") || strings.Contains(strings.ToLower(turn.Error), "permission denied") || strings.Contains(strings.ToLower(turn.Error), "policy denied") {
		return RuntimeDiagnosticIncident{}, false
	}
	incident := RuntimeDiagnosticIncident{ID: "turn:" + turn.ID, Severity: "error", SessionID: turn.SessionID, TurnID: turn.ID, Provider: turn.Provider, Model: turn.Model, CreatedAt: millisRFC3339(turn.StartedAt), LastObservedAt: firstNonEmpty(lastObserved, millisRFC3339(firstNonZeroInt64(turn.FinishedAt, turn.StartedAt))), Evidence: evidence, Resolved: recovered}
	if recovered {
		incident.Status = "recovered"
	} else {
		incident.Status = "unresolved"
	}
	if turn.Status == turnStatusInterrupted {
		incident.Kind = diagnosticKindTurnInterrupted
		incident.Title = "对话在完成前中断"
		incident.Summary = firstNonEmpty(preview(turn.Error, 220), "Runtime 未记录到最终回复")
		incident.Cause = "Runtime 检测到该 Turn 未正常生成最终回复，且状态停留在中断。"
		incident.Resolution = "打开关联会话查看最后活动；确认不再继续后，可结束异常状态。"
		incident.ErrorCode = "turn_interrupted"
		incident.Recoverable = !recovered
		incident.RecommendedCheckID = ""
		incident.Actions = diagnosticSessionActions(turn, true)
		return incident, len(evidence) > 0
	}
	recoverable, classified := classifyRuntimeRecoverableError(turn)
	incident.Kind = diagnosticKindProviderFailure
	incident.Title = "模型请求失败"
	incident.Summary = preview(turn.Error, 220)
	incident.ErrorCode = "provider_error"
	if classified {
		incident.ErrorCode = recoverable.Kind
	}
	incident.Cause, incident.Resolution = providerDiagnosticGuidance(incident.ErrorCode)
	incident.Recoverable = false
	if incident.ErrorCode == recoverableErrorNetworkTransient {
		incident.RecommendedCheckID = diagnosticCheckProvider
	}
	incident.Actions = providerDiagnosticActions(turn, incident.ErrorCode)
	return incident, strings.TrimSpace(turn.Error) != "" && len(evidence) > 0
}

func buildToolDiagnosticIncident(tool diagnosticToolFact, evidence []RuntimeDiagnosticEvidence) (RuntimeDiagnosticIncident, bool) {
	evidence = compactDiagnosticEvidence(evidence, 5)
	if tool.Status != "failed" || (!tool.IsError && tool.Error == "") {
		return RuntimeDiagnosticIncident{}, false
	}
	if strings.Contains(strings.ToLower(tool.Error+" "+tool.PolicyReason), "permission denied") || strings.Contains(strings.ToLower(tool.Error+" "+tool.PolicyReason), "policy denied") {
		return RuntimeDiagnosticIncident{}, false
	}
	incident := RuntimeDiagnosticIncident{ID: "tool:" + tool.ID, Kind: diagnosticKindToolFailure, Severity: "error", Status: "unresolved", SessionID: tool.SessionID, TurnID: tool.TurnID, ToolCallID: tool.ID, Title: "工具执行失败", Summary: firstNonEmpty(preview(tool.Error, 220), preview(tool.OutputSummary, 220), "工具未成功完成"), Cause: "工具调用已明确以失败状态结束，且该失败未被关联 Turn 正常处理。", Resolution: "查看故障链中的工具输出摘要；需要再次执行时，请返回关联会话重新发起。", ErrorCode: "tool_execution_failed", Recoverable: false, CreatedAt: millisRFC3339(tool.StartedAt), LastObservedAt: millisRFC3339(firstNonZeroInt64(tool.FinishedAt, tool.StartedAt)), Evidence: evidence}
	if tool.SessionID != "" {
		incident.Actions = []RuntimeRecoveryAction{{ID: "session:" + tool.ID, Label: "打开关联会话", Kind: "open_session", SessionID: tool.SessionID, TurnID: tool.TurnID}}
	}
	for _, ev := range evidence {
		if ev.Metadata["path"] != "" && toolNeedsPathCheck(tool) {
			incident.RecommendedCheckID = diagnosticCheckPath
			break
		}
	}
	return incident, len(evidence) > 0
}

func diagnosticSessionActions(turn RuntimeTurn, allowStateCleanup bool) []RuntimeRecoveryAction {
	actions := make([]RuntimeRecoveryAction, 0, 2)
	if turn.SessionID != "" {
		actions = append(actions, RuntimeRecoveryAction{ID: "session:" + turn.ID, Label: "打开关联会话", Kind: "open_session", SessionID: turn.SessionID, TurnID: turn.ID})
	}
	if allowStateCleanup {
		actions = append(actions, RuntimeRecoveryAction{ID: "mark_done:" + turn.ID, Label: "结束异常状态", Kind: "mark_done", SessionID: turn.SessionID, TurnID: turn.ID})
	}
	return actions
}

func providerDiagnosticActions(turn RuntimeTurn, errorCode string) []RuntimeRecoveryAction {
	actions := make([]RuntimeRecoveryAction, 0, 2)
	switch errorCode {
	case recoverableErrorAuthExpired, recoverableErrorModelNotFound, recoverableErrorModelCapabilityUnsupported:
		actions = append(actions, RuntimeRecoveryAction{ID: "provider_settings:" + turn.ID, Label: "打开 Provider 设置", Kind: "open_provider_settings", SessionID: turn.SessionID, TurnID: turn.ID})
	}
	return append(actions, diagnosticSessionActions(turn, false)...)
}

func providerDiagnosticGuidance(errorCode string) (string, string) {
	switch errorCode {
	case recoverableErrorContextLengthExceeded:
		return "请求上下文超过了当前模型允许的长度。", "返回关联会话压缩上下文、减少输入内容或开启新会话后再发送。"
	case recoverableErrorRateLimited:
		return "Provider 明确返回了请求频率限制。", "等待限流窗口恢复后，返回关联会话重新发送。"
	case recoverableErrorOverloaded:
		return "Provider 当前过载或暂时不可用。", "稍后返回关联会话重新发送；持续发生时请查看 Provider 服务状态。"
	case recoverableErrorNetworkTransient:
		return "应用与 Provider 之间的连接超时或意外中断。", "检查当前网络；如需确认配置是否可连接，可运行 Provider 连接测试。"
	case recoverableErrorAuthExpired:
		return "Provider 拒绝了当前认证信息。", "打开 Provider 设置更新认证信息，测试配置后再返回会话。"
	case recoverableErrorModelNotFound:
		return "当前 Provider 找不到已配置的模型。", "打开 Provider 设置选择该 Provider 支持的模型。"
	case recoverableErrorModelCapabilityUnsupported:
		return "当前模型不支持本次请求所需的能力或输入类型。", "调整输入内容，或在 Provider 设置中选择支持该能力的模型。"
	default:
		return "模型请求已明确失败，但现有结构化证据不足以确认更具体的根因。", "查看技术信息中的错误码和摘要；检查 Provider 配置后返回关联会话。"
	}
}

func toolNeedsPathCheck(tool diagnosticToolFact) bool {
	lower := strings.ToLower(tool.Error + " " + tool.OutputSummary)
	for _, marker := range []string{"no such file", "not found", "does not exist", "cannot access", "access denied", "path", "directory"} {
		if strings.Contains(lower, marker) {
			return true
		}
	}
	return false
}

func diagnosticIncidentMatches(incident RuntimeDiagnosticIncident, req RuntimeDiagnosticIncidentsRequest) bool {
	if req.Kind != "" && incident.Kind != req.Kind {
		return false
	}
	if req.Status != "" && incident.Status != req.Status {
		return false
	}
	return req.SessionID == "" || incident.SessionID == req.SessionID
}

func (r *runtimeService) projectPersistenceDiagnosticItems(req RuntimeDiagnosticIncidentsRequest) []RuntimeDiagnosticIncident {
	r.diagnosticMu.Lock()
	records := append([]runtimePersistenceDiagnostic(nil), r.persistenceDiagnostics...)
	r.diagnosticMu.Unlock()
	items := make([]RuntimeDiagnosticIncident, 0, len(records))
	for _, record := range records {
		incident := RuntimeDiagnosticIncident{ID: record.ID, Kind: diagnosticKindPersistenceFailure, Severity: "error", Status: "unresolved", TurnID: record.TurnID, Title: "数据持久化失败", Summary: record.Summary, Cause: "Runtime 在写入关键状态时收到明确的数据库错误。", Resolution: "检查数据目录是否可写、磁盘空间和数据库占用情况；必要时运行数据库状态检查。", ErrorCode: record.ErrorCode, Recoverable: false, CreatedAt: record.CreatedAt.Format(time.RFC3339Nano), LastObservedAt: record.CreatedAt.Format(time.RFC3339Nano), RecommendedCheckID: diagnosticCheckSQLite, Evidence: []RuntimeDiagnosticEvidence{{ID: record.ID + ":operation", Source: "runtime_persistence", Kind: record.ErrorCode, Label: "持久化操作", Summary: record.Operation, Timestamp: record.CreatedAt.Format(time.RFC3339Nano), TurnID: record.TurnID}}, Actions: []RuntimeRecoveryAction{{ID: "data_directory:" + record.ID, Label: "打开数据目录", Kind: "open_data_directory", TurnID: record.TurnID}}}
		if diagnosticIncidentMatches(incident, req) {
			items = append(items, incident)
		}
	}
	return items
}

func (r *runtimeService) projectPersistenceDiagnostics(req RuntimeDiagnosticIncidentsRequest, limit int) RuntimeDiagnosticIncidentsResponse {
	return finalizeDiagnosticResponse(r.projectPersistenceDiagnosticItems(req), limit)
}

func finalizeDiagnosticResponse(items []RuntimeDiagnosticIncident, limit int) RuntimeDiagnosticIncidentsResponse {
	sort.SliceStable(items, func(i, j int) bool { return items[i].LastObservedAt > items[j].LastObservedAt })
	resp := RuntimeDiagnosticIncidentsResponse{}
	cutoff := time.Now().AddDate(0, 0, -7)
	for _, item := range items {
		if ts, err := time.Parse(time.RFC3339Nano, item.LastObservedAt); err == nil && ts.After(cutoff) {
			resp.RecentCount++
		}
		if item.Recoverable && !item.Resolved {
			resp.RecoverableCount++
		}
	}
	if len(items) > 0 {
		resp.LatestAt = items[0].LastObservedAt
	}
	if len(items) > limit {
		last := items[limit-1]
		resp.NextCursor = diagnosticCursor(last)
		items = items[:limit]
	}
	resp.Incidents = items
	if resp.Incidents == nil {
		resp.Incidents = []RuntimeDiagnosticIncident{}
	}
	return resp
}

func (r *runtimeService) RunTargetedDiagnostic(ctx context.Context, incidentID, checkID string) (RuntimeTargetedDiagnostic, error) {
	incident, err := r.DiagnosticIncident(ctx, incidentID)
	if err != nil {
		return RuntimeTargetedDiagnostic{}, err
	}
	if checkID == "" {
		checkID = incident.RecommendedCheckID
	}
	if checkID == "" || checkID != incident.RecommendedCheckID {
		return RuntimeTargetedDiagnostic{}, fmt.Errorf("check %q is not available for incident %s", checkID, incidentID)
	}
	started := time.Now()
	checkCtx, cancel := context.WithTimeout(ctx, 8*time.Second)
	defer cancel()
	result := RuntimeTargetedDiagnostic{IncidentID: incident.ID, CheckID: checkID, CompletedAt: time.Now().UTC().Format(time.RFC3339Nano)}
	switch checkID {
	case diagnosticCheckProvider:
		result.Title = "Provider 连接测试"
		providerID, findErr := r.configuredProviderIDForIncident(checkCtx, incident)
		if findErr != nil {
			result.Status = "error"
			result.Summary = "无法定位该故障使用的 Provider 配置"
			result.Detail = findErr.Error()
			break
		}
		test, testErr := r.TestConfiguredProvider(checkCtx, providerID)
		if testErr != nil {
			result.Status = "error"
			result.Summary = "当前 Provider 配置无法完成连接测试"
			result.Detail = preview(testErr.Error(), 400)
		} else if test.OK {
			result.Status = "pass"
			result.Summary = "当前配置可以连接该 Provider"
			result.Detail = "测试未发送会话正文。"
		} else {
			result.Status = "error"
			result.Summary = "当前配置仍无法连接该 Provider"
			result.Detail = preview(test.Error, 400)
		}
	case diagnosticCheckPath:
		result.Title = "引用路径验证"
		path := incidentEvidencePath(incident)
		if path == "" {
			return RuntimeTargetedDiagnostic{}, errors.New("incident does not contain an eligible evidence path")
		}
		info, statErr := os.Stat(path)
		if statErr != nil {
			result.Status = "error"
			result.Summary = "该故障引用的路径当前不可访问"
			result.Detail = preview(statErr.Error(), 400)
		} else {
			result.Status = "pass"
			result.Summary = "该故障引用的路径当前存在并可访问"
			if info.IsDir() {
				result.Detail = "类型：目录"
			} else {
				result.Detail = "类型：文件"
			}
		}
	case diagnosticCheckSQLite:
		if incident.Kind != diagnosticKindPersistenceFailure {
			return RuntimeTargetedDiagnostic{}, errors.New("sqlite quick check requires a direct persistence incident")
		}
		result.Title = "SQLite quick check"
		conn, openErr := r.configDB(checkCtx)
		if openErr != nil {
			result.Status = "error"
			result.Summary = "数据库当前无法打开"
			result.Detail = preview(openErr.Error(), 400)
			break
		}
		layout, _ := resolveDesktopLayout()
		defer db.Release(layout.DataDir) //nolint:errcheck
		var quick string
		queryErr := conn.QueryRowContext(checkCtx, "PRAGMA quick_check").Scan(&quick)
		if queryErr != nil {
			result.Status = "error"
			result.Summary = "SQLite quick check 未完成"
			result.Detail = preview(queryErr.Error(), 400)
		} else if strings.EqualFold(strings.TrimSpace(quick), "ok") {
			result.Status = "pass"
			result.Summary = "SQLite quick check 未发现结构损坏"
			result.Detail = "该检查不会修复或修改用户数据。"
		} else {
			result.Status = "error"
			result.Summary = "SQLite quick check 报告异常"
			result.Detail = preview(quick, 400)
		}
	default:
		return RuntimeTargetedDiagnostic{}, fmt.Errorf("unsupported diagnostic check %q", checkID)
	}
	result.DurationMillis = time.Since(started).Milliseconds()
	result.CompletedAt = time.Now().UTC().Format(time.RFC3339Nano)
	r.diagnosticMu.Lock()
	r.diagnosticChecks[incident.ID+":"+checkID] = result
	r.diagnosticMu.Unlock()
	return result, nil
}

func (r *runtimeService) configuredProviderIDForIncident(ctx context.Context, incident RuntimeDiagnosticIncident) (string, error) {
	conn, err := r.configDB(ctx)
	if err != nil {
		return "", err
	}
	layout, _ := resolveDesktopLayout()
	defer db.Release(layout.DataDir) //nolint:errcheck
	var id string
	err = conn.QueryRowContext(ctx, `SELECT id FROM configured_providers WHERE id = ? OR provider_id = ? OR name = ? ORDER BY CASE WHEN id = ? THEN 0 ELSE 1 END LIMIT 1`, incident.Provider, incident.Provider, incident.Provider, incident.Provider).Scan(&id)
	if err != nil {
		return "", err
	}
	return id, nil
}

func (r *runtimeService) DiagnosticSupportInformation(ctx context.Context, incidentID string) (RuntimeDiagnosticSupportInformation, error) {
	incident, err := r.DiagnosticIncident(ctx, incidentID)
	if err != nil {
		return RuntimeDiagnosticSupportInformation{}, err
	}
	layout, _ := resolveDesktopLayout()
	sources := make([]string, 0, len(incident.Evidence))
	seen := map[string]struct{}{}
	for _, ev := range incident.Evidence {
		if _, ok := seen[ev.Source]; !ok {
			seen[ev.Source] = struct{}{}
			sources = append(sources, ev.Source)
		}
	}
	sort.Strings(sources)
	r.diagnosticMu.Lock()
	checks := []map[string]any{}
	for key, value := range r.diagnosticChecks {
		if strings.HasPrefix(key, incident.ID+":") {
			checks = append(checks, map[string]any{
				"checkId":        value.CheckID,
				"status":         value.Status,
				"summary":        value.Summary,
				"durationMillis": value.DurationMillis,
				"completedAt":    value.CompletedAt,
			})
		}
	}
	r.diagnosticMu.Unlock()
	previewData := map[string]any{"application": map[string]string{"version": version.Version, "buildId": version.BuildID, "os": goruntime.GOOS, "architecture": goruntime.GOARCH}, "incident": map[string]any{"id": incident.ID, "kind": incident.Kind, "errorCode": incident.ErrorCode, "status": incident.Status, "sessionId": incident.SessionID, "turnId": incident.TurnID, "toolCallId": incident.ToolCallID, "provider": incident.Provider, "model": incident.Model, "createdAt": incident.CreatedAt, "lastObservedAt": incident.LastObservedAt}, "evidenceSources": sources, "targetedChecks": checks, "storage": map[string]string{"dataDirectory": redactSupportPath(layout.DataDir), "logLocation": redactSupportPath(layout.LogsDir)}}
	encoded, err := json.MarshalIndent(previewData, "", "  ")
	if err != nil {
		return RuntimeDiagnosticSupportInformation{}, err
	}
	return RuntimeDiagnosticSupportInformation{IncidentID: incident.ID, Preview: previewData, Text: string(encoded)}, nil
}

func (r *runtimeService) OpenDiagnosticDataDirectory(context.Context) (bool, error) {
	layout, err := resolveDesktopLayout()
	if err != nil {
		return false, err
	}
	if err := ensureDesktopLayout(layout); err != nil {
		return false, err
	}
	if err := runtimeOpenPathInFileManager(layout.DataDir); err != nil {
		return false, err
	}
	return true, nil
}

func redactSupportPath(path string) string {
	home, _ := os.UserHomeDir()
	clean := strings.TrimSpace(path)
	if home != "" && strings.HasPrefix(strings.ToLower(clean), strings.ToLower(home)) {
		return "~" + strings.TrimPrefix(clean, home)
	}
	return clean
}

func incidentEvidencePath(incident RuntimeDiagnosticIncident) string {
	for _, ev := range incident.Evidence {
		if path := strings.TrimSpace(ev.Metadata["path"]); path != "" {
			return path
		}
	}
	return ""
}

func sqlInClause(values []string) (string, []any) {
	holders := make([]string, len(values))
	args := make([]any, len(values))
	for i, value := range values {
		holders[i] = "?"
		args[i] = value
	}
	return strings.Join(holders, ","), args
}

func millisRFC3339(value int64) string {
	if value <= 0 {
		return ""
	}
	return time.UnixMilli(value).UTC().Format(time.RFC3339Nano)
}

func diagnosticBeforeMillis(value string) int64 {
	value = strings.TrimSpace(value)
	if value == "" {
		return time.Now().Add(time.Millisecond).UnixMilli()
	}
	if parts := strings.SplitN(value, ":", 2); len(parts) > 0 {
		if parsed, err := strconv.ParseInt(parts[0], 10, 64); err == nil {
			return parsed
		}
	}
	if parsed, err := time.Parse(time.RFC3339Nano, value); err == nil {
		return parsed.UnixMilli()
	}
	return time.Now().Add(time.Millisecond).UnixMilli()
}

func diagnosticCursor(item RuntimeDiagnosticIncident) string {
	parsed, _ := time.Parse(time.RFC3339Nano, item.LastObservedAt)
	return strconv.FormatInt(parsed.UnixMilli(), 10) + ":" + item.ID
}

func laterTimestamp(left, right string) string {
	if left == "" {
		return right
	}
	if right == "" {
		return left
	}
	if right > left {
		return right
	}
	return left
}

func nonEmptyMetadata(values map[string]string) map[string]string {
	result := map[string]string{}
	for key, value := range values {
		if strings.TrimSpace(value) != "" {
			result[key] = value
		}
	}
	if len(result) == 0 {
		return nil
	}
	return result
}

func isRelevantDiagnosticEvent(kind string) bool {
	lower := strings.ToLower(strings.TrimSpace(kind))
	return strings.Contains(lower, "failed") || strings.Contains(lower, "interrupted") || strings.HasPrefix(lower, "recovery.") || strings.HasPrefix(lower, "tool_call.")
}

func isRelevantDiagnosticAudit(kind string) bool {
	lower := strings.ToLower(strings.TrimSpace(kind))
	return strings.Contains(lower, "failed") || strings.Contains(lower, "interrupted") || strings.Contains(lower, "recovery") || strings.HasPrefix(lower, "tool_call_")
}

func isRelevantAuxiliaryDiagnosticEvidence(kind, summary string) bool {
	lower := strings.ToLower(strings.TrimSpace(kind + " " + summary))
	for _, marker := range []string{"fail", "error", "interrupt", "timeout", "denied", "blocked", "cancelled"} {
		if strings.Contains(lower, marker) {
			return true
		}
	}
	return false
}

func compactDiagnosticEvidence(evidence []RuntimeDiagnosticEvidence, limit int) []RuntimeDiagnosticEvidence {
	if len(evidence) == 0 {
		return []RuntimeDiagnosticEvidence{}
	}
	sort.SliceStable(evidence, func(i, j int) bool { return evidence[i].Timestamp < evidence[j].Timestamp })
	groups := make(map[string]int, len(evidence))
	compacted := make([]RuntimeDiagnosticEvidence, 0, len(evidence))
	for _, item := range evidence {
		group := diagnosticEvidenceGroup(item)
		if index, ok := groups[group]; ok {
			existing := &compacted[index]
			if existing.Metadata == nil {
				existing.Metadata = map[string]string{}
			}
			existing.Metadata["corroboratedBy"] = appendEvidenceSource(existing.Metadata["corroboratedBy"], item.Source)
			if item.Timestamp > existing.Timestamp {
				existing.Timestamp = item.Timestamp
			}
			continue
		}
		item.Metadata = nonEmptyMetadata(item.Metadata)
		groups[group] = len(compacted)
		compacted = append(compacted, item)
	}
	sort.SliceStable(compacted, func(i, j int) bool { return compacted[i].Timestamp < compacted[j].Timestamp })
	if limit > 0 && len(compacted) > limit {
		compacted = append([]RuntimeDiagnosticEvidence{compacted[0]}, compacted[len(compacted)-limit+1:]...)
	}
	return compacted
}

func diagnosticEvidenceGroup(item RuntimeDiagnosticEvidence) string {
	switch item.Source {
	case "runtime_turns":
		return "turn"
	case "runtime_events", "runtime_audit_events":
		kind := strings.ToLower(item.Kind)
		if strings.Contains(kind, "fail") {
			return "runtime_outcome:failed"
		}
		if strings.Contains(kind, "interrupt") {
			return "runtime_outcome:interrupted"
		}
		return "runtime_outcome:" + kind
	case "runtime_recovery_links":
		return "recovery"
	case "runtime_tool_calls":
		return "tool:" + item.ToolCallID
	default:
		return item.Source + ":" + firstNonEmpty(item.ToolCallID, item.Kind)
	}
}

func appendEvidenceSource(current, source string) string {
	if source == "" || strings.Contains(","+current+",", ","+source+",") {
		return current
	}
	if current == "" {
		return source
	}
	return current + "," + source
}
