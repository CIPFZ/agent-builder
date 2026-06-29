package runtime

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"
)

type runtimeRecoveryLink struct {
	ID               string
	SourceTurnID     string
	ResumedTurnID    string
	Action           string
	Mode             string
	InterruptionKind string
	CreatedAt        string
}

type runtimeRecoveryLinkStore struct {
	db *sql.DB
}

func newRuntimeRecoveryLinkStore(db *sql.DB) runtimeRecoveryLinkStore {
	return runtimeRecoveryLinkStore{db: db}
}

func (s runtimeRecoveryLinkStore) Insert(ctx context.Context, link runtimeRecoveryLink) (runtimeRecoveryLink, error) {
	if s.db == nil {
		return runtimeRecoveryLink{}, errors.New("runtime recovery database is not available")
	}
	link.ID = strings.TrimSpace(link.ID)
	link.SourceTurnID = strings.TrimSpace(link.SourceTurnID)
	link.ResumedTurnID = strings.TrimSpace(link.ResumedTurnID)
	if link.ID == "" {
		link.ID = newRuntimeRecoveryLinkID()
	}
	if link.SourceTurnID == "" || link.ResumedTurnID == "" {
		return runtimeRecoveryLink{}, errors.New("source and resumed turn ids are required")
	}
	if link.CreatedAt == "" {
		link.CreatedAt = time.Now().UTC().Format(time.RFC3339Nano)
	}
	_, err := s.db.ExecContext(ctx, `
INSERT INTO runtime_recovery_links (
    id, source_turn_id, resumed_turn_id, action, mode, interruption_kind, created_at
) VALUES (?, ?, ?, ?, ?, ?, ?)
ON CONFLICT(id) DO NOTHING`,
		link.ID, link.SourceTurnID, link.ResumedTurnID, link.Action, link.Mode, link.InterruptionKind, link.CreatedAt)
	if err != nil {
		return runtimeRecoveryLink{}, fmt.Errorf("failed to insert runtime recovery link: %w", err)
	}
	return link, nil
}

func (s runtimeRecoveryLinkStore) ListBySource(ctx context.Context, sourceTurnID string) ([]runtimeRecoveryLink, error) {
	if s.db == nil {
		return nil, nil
	}
	rows, err := s.db.QueryContext(ctx, `
SELECT id, source_turn_id, resumed_turn_id, action, mode, interruption_kind, created_at
FROM runtime_recovery_links
WHERE source_turn_id = ?
ORDER BY created_at ASC`, strings.TrimSpace(sourceTurnID))
	if err != nil {
		return nil, err
	}
	defer rows.Close() //nolint:errcheck
	var links []runtimeRecoveryLink
	for rows.Next() {
		var link runtimeRecoveryLink
		if err := rows.Scan(&link.ID, &link.SourceTurnID, &link.ResumedTurnID, &link.Action, &link.Mode, &link.InterruptionKind, &link.CreatedAt); err != nil {
			return nil, err
		}
		links = append(links, link)
	}
	return links, rows.Err()
}

func newRuntimeRecoveryLinkID() string {
	return "recovery_link_" + newStreamToken()
}
