package runtime

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"unicode/utf8"

	"github.com/CIPFZ/agent-builder/internal/db"
)

const runtimeConversationSearchLimit = 50

func (r *runtimeService) SearchSessionConversationV2(ctx context.Context, sessionID string, req RuntimeConversationSearchRequestV2) (RuntimeConversationSearchResponseV2, error) {
	if err := r.ensureWorkspaceStarted(ctx, false); err != nil {
		return RuntimeConversationSearchResponseV2{}, err
	}
	sessionID, req.Query = strings.TrimSpace(sessionID), strings.TrimSpace(req.Query)
	if sessionID == "" || req.Query == "" {
		return RuntimeConversationSearchResponseV2{}, errors.New("session id and search query are required")
	}
	r.mu.Lock()
	workspaceID := r.workspace.ID
	r.mu.Unlock()
	workspace, err := r.runtime.GetWorkspace(workspaceID)
	if err != nil {
		return RuntimeConversationSearchResponseV2{}, err
	}
	conn, err := db.Connect(ctx, workspace.DataDir)
	if err != nil {
		return RuntimeConversationSearchResponseV2{}, err
	}
	defer db.Release(workspace.DataDir) //nolint:errcheck
	limit := req.Limit
	if limit <= 0 {
		limit = 20
	}
	if limit > runtimeConversationSearchLimit {
		limit = runtimeConversationSearchLimit
	}
	ftsQuery := `"` + strings.ReplaceAll(req.Query, `"`, `""`) + `"`
	candidates := make([]RuntimeConversationSearchResultV2, 0, limit)
	err = queryRuntimeRows(ctx, conn, `
SELECT f.message_id, COALESCE(e.turn_id, ''), f.role, CAST(f.created_at AS INTEGER)
FROM message_search_fts AS f
LEFT JOIN conversation_entities_v2 AS e
  ON e.session_id = f.session_id AND e.entity_type = 'message' AND e.entity_id = f.message_id
WHERE f.session_id = ? AND message_search_fts MATCH ?
ORDER BY bm25(message_search_fts), CAST(f.created_at AS INTEGER) DESC
LIMIT ?`, func(rows *sql.Rows) error {
		var result RuntimeConversationSearchResultV2
		if err := rows.Scan(&result.MessageID, &result.TurnID, &result.Role, &result.CreatedAt); err != nil {
			return err
		}
		candidates = append(candidates, result)
		return nil
	}, sessionID, ftsQuery, limit)
	if err != nil {
		return RuntimeConversationSearchResponseV2{}, fmt.Errorf("failed to search conversation: %w", err)
	}
	results := make([]RuntimeConversationSearchResultV2, 0, len(candidates))
	for _, result := range candidates {
		msg, readErr := r.runtime.GetSessionMessage(ctx, workspaceID, sessionID, result.MessageID)
		if readErr != nil || msg.IsSummaryMessage {
			continue
		}
		if result.TurnID == "" {
			result.TurnID = firstNonEmpty(msg.Metadata["turn_id"], msg.Metadata["turnId"])
		}
		content := toRuntimeMessage(toAPITypeMessage(msg)).Content
		result.Snippet = conversationSearchSnippet(content, req.Query, 240)
		results = append(results, result)
	}
	return RuntimeConversationSearchResponseV2{SchemaVersion: RuntimeConversationSchemaVersion, SessionID: sessionID, Results: results}, nil
}

func conversationSearchSnippet(content, query string, limit int) string {
	content = strings.TrimSpace(content)
	if len(content) <= limit {
		return content
	}
	index := strings.Index(strings.ToLower(content), strings.ToLower(query))
	if index < 0 {
		index = 0
	}
	start := index - limit/3
	if start < 0 {
		start = 0
	}
	for start < len(content) && !utf8.RuneStart(content[start]) {
		start++
	}
	end := start + limit
	if end > len(content) {
		end = len(content)
		start = end - limit
	}
	for start < len(content) && !utf8.RuneStart(content[start]) {
		start++
	}
	for end > start && !utf8.ValidString(content[start:end]) {
		end--
	}
	preview, _ := boundedUTF8Content(content[start:end], limit)
	if start > 0 {
		preview = "…" + preview
	}
	if end < len(content) {
		preview += "…"
	}
	return preview
}
