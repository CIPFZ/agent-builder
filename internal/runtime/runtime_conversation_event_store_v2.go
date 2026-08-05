package runtime

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"time"
)

type runtimeConversationEventStoreV2 struct{ db *sql.DB }

func newRuntimeConversationEventStoreV2(db *sql.DB) runtimeConversationEventStoreV2 {
	return runtimeConversationEventStoreV2{db: db}
}

func (s runtimeConversationEventStoreV2) ensure(ctx context.Context) error {
	if s.db == nil {
		return errors.New("canonical conversation event database is not available")
	}
	_, err := s.db.ExecContext(ctx, `CREATE TABLE IF NOT EXISTS conversation_entity_events_v2 (
session_id TEXT NOT NULL, raw_sequence INTEGER NOT NULL, ordinal INTEGER NOT NULL,
event_id TEXT NOT NULL UNIQUE, entity_type TEXT NOT NULL, entity_id TEXT NOT NULL,
operation TEXT NOT NULL, revision TEXT NOT NULL, event_json TEXT NOT NULL, created_at INTEGER NOT NULL,
PRIMARY KEY(session_id,raw_sequence,ordinal), UNIQUE(session_id,raw_sequence,entity_type,entity_id,operation));
CREATE TABLE IF NOT EXISTS conversation_projector_checkpoints_v2 (
session_id TEXT PRIMARY KEY,last_raw_sequence INTEGER NOT NULL,failure_reason TEXT,updated_at INTEGER NOT NULL);
CREATE TABLE IF NOT EXISTS conversation_projector_batches_v2 (
session_id TEXT NOT NULL,raw_sequence INTEGER NOT NULL,previous_raw_sequence INTEGER NOT NULL,entity_count INTEGER NOT NULL,created_at INTEGER NOT NULL,PRIMARY KEY(session_id,raw_sequence));
CREATE TABLE IF NOT EXISTS conversation_projector_retention_v2 (
session_id TEXT PRIMARY KEY,floor_raw_sequence INTEGER NOT NULL,updated_at INTEGER NOT NULL);
CREATE TABLE IF NOT EXISTS conversation_entities_v2 (
session_id TEXT NOT NULL,entity_type TEXT NOT NULL,entity_id TEXT NOT NULL,turn_id TEXT,activity_sequence TEXT NOT NULL,revision TEXT NOT NULL,entity_json TEXT NOT NULL,updated_at INTEGER NOT NULL,PRIMARY KEY(session_id,entity_type,entity_id));`)
	return err
}

func (s runtimeConversationEventStoreV2) checkpoint(ctx context.Context, sessionID string) (int64, string, bool, error) {
	if err := s.ensure(ctx); err != nil {
		return 0, "", false, err
	}
	var seq int64
	var reason sql.NullString
	err := s.db.QueryRowContext(ctx, `SELECT last_raw_sequence,failure_reason FROM conversation_projector_checkpoints_v2 WHERE session_id=?`, sessionID).Scan(&seq, &reason)
	if errors.Is(err, sql.ErrNoRows) {
		return 0, "", false, nil
	}
	if err != nil {
		return 0, "", false, err
	}
	return seq, reason.String, true, nil
}

func (s runtimeConversationEventStoreV2) initializeCheckpoint(ctx context.Context, sessionID string, sequence int64, reason string) error {
	if err := s.ensure(ctx); err != nil {
		return err
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback() //nolint:errcheck
	now := time.Now().UnixMilli()
	if _, err = tx.ExecContext(ctx, `INSERT INTO conversation_projector_checkpoints_v2(session_id,last_raw_sequence,failure_reason,updated_at)
VALUES(?,?,NULLIF(?,''),?) ON CONFLICT(session_id) DO UPDATE SET last_raw_sequence=excluded.last_raw_sequence,failure_reason=excluded.failure_reason,updated_at=excluded.updated_at`, sessionID, sequence, reason, now); err != nil {
		return err
	}
	if _, err = tx.ExecContext(ctx, `INSERT INTO conversation_projector_retention_v2(session_id,floor_raw_sequence,updated_at) VALUES(?,?,?)
ON CONFLICT(session_id) DO UPDATE SET floor_raw_sequence=excluded.floor_raw_sequence,updated_at=excluded.updated_at`, sessionID, sequence, now); err != nil {
		return err
	}
	return tx.Commit()
}

func (s runtimeConversationEventStoreV2) retentionFloor(ctx context.Context, sessionID string) (int64, bool, error) {
	if err := s.ensure(ctx); err != nil {
		return 0, false, err
	}
	var floor int64
	err := s.db.QueryRowContext(ctx, `SELECT floor_raw_sequence FROM conversation_projector_retention_v2 WHERE session_id=?`, sessionID).Scan(&floor)
	if errors.Is(err, sql.ErrNoRows) {
		return 0, false, nil
	}
	return floor, err == nil, err
}

func (s runtimeConversationEventStoreV2) advanceRetentionFloor(ctx context.Context, sessionID string, floor int64) error {
	if err := s.ensure(ctx); err != nil {
		return err
	}
	_, err := s.db.ExecContext(ctx, `INSERT INTO conversation_projector_retention_v2(session_id,floor_raw_sequence,updated_at) VALUES(?,?,?)
ON CONFLICT(session_id) DO UPDATE SET floor_raw_sequence=MAX(conversation_projector_retention_v2.floor_raw_sequence,excluded.floor_raw_sequence),updated_at=excluded.updated_at`, sessionID, floor, time.Now().UnixMilli())
	return err
}

func (s runtimeConversationEventStoreV2) commitBatch(ctx context.Context, sessionID string, rawSequence int64, events []RuntimeConversationEntityEventV2) error {
	if err := s.ensure(ctx); err != nil {
		return err
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback() //nolint:errcheck
	for ordinal, event := range events {
		encoded, marshalErr := json.Marshal(event)
		if marshalErr != nil {
			return marshalErr
		}
		result, execErr := tx.ExecContext(ctx, `INSERT INTO conversation_entity_events_v2(session_id,raw_sequence,ordinal,event_id,entity_type,entity_id,operation,revision,event_json,created_at)
VALUES(?,?,?,?,?,?,?,?,?,?) ON CONFLICT(session_id,raw_sequence,entity_type,entity_id,operation) DO NOTHING`, sessionID, rawSequence, ordinal, event.ID, event.EntityType, event.EntityID, event.Operation, event.Revision, string(encoded), event.CreatedAt)
		if execErr != nil {
			return execErr
		}
		affected, _ := result.RowsAffected()
		if affected == 0 {
			var existing string
			if scanErr := tx.QueryRowContext(ctx, `SELECT event_json FROM conversation_entity_events_v2 WHERE session_id=? AND raw_sequence=? AND entity_type=? AND entity_id=? AND operation=?`, sessionID, rawSequence, event.EntityType, event.EntityID, event.Operation).Scan(&existing); scanErr != nil {
				return scanErr
			}
			if existing != string(encoded) {
				return fmt.Errorf("canonical projector conflict at sequence %d entity %s:%s", rawSequence, event.EntityType, event.EntityID)
			}
		}
	}
	var previous int64
	if err = tx.QueryRowContext(ctx, `SELECT last_raw_sequence FROM conversation_projector_checkpoints_v2 WHERE session_id=?`, sessionID).Scan(&previous); err != nil {
		return err
	}
	if _, err = tx.ExecContext(ctx, `INSERT OR IGNORE INTO conversation_projector_retention_v2(session_id,floor_raw_sequence,updated_at) VALUES(?,?,?)`, sessionID, previous, time.Now().UnixMilli()); err != nil {
		return err
	}
	if _, err = tx.ExecContext(ctx, `INSERT INTO conversation_projector_batches_v2(session_id,raw_sequence,previous_raw_sequence,entity_count,created_at) VALUES(?,?,?,?,?) ON CONFLICT(session_id,raw_sequence) DO UPDATE SET entity_count=excluded.entity_count`, sessionID, rawSequence, previous, len(events), time.Now().UnixMilli()); err != nil {
		return err
	}
	if _, err = tx.ExecContext(ctx, `INSERT INTO conversation_projector_checkpoints_v2(session_id,last_raw_sequence,failure_reason,updated_at) VALUES(?,?,NULL,?) ON CONFLICT(session_id) DO UPDATE SET last_raw_sequence=excluded.last_raw_sequence,failure_reason=NULL,updated_at=excluded.updated_at`, sessionID, rawSequence, time.Now().UnixMilli()); err != nil {
		return err
	}
	return tx.Commit()
}

type runtimeConversationBatchCursorV2 struct {
	previous, sequence int64
	entityCount        int
	encodedBytes       int64
}

func (s runtimeConversationEventStoreV2) batchCursorsAfter(ctx context.Context, sessionID string, after int64, limit int) ([]runtimeConversationBatchCursorV2, error) {
	if err := s.ensure(ctx); err != nil {
		return nil, err
	}
	query := `SELECT b.previous_raw_sequence,b.raw_sequence,b.entity_count,
COALESCE((SELECT SUM(length(CAST(e.event_json AS BLOB))) FROM conversation_entity_events_v2 e WHERE e.session_id=b.session_id AND e.raw_sequence=b.raw_sequence),0)
FROM conversation_projector_batches_v2 b WHERE b.session_id=? AND b.raw_sequence>? ORDER BY b.raw_sequence`
	args := []any{sessionID, after}
	if limit > 0 {
		query += ` LIMIT ?`
		args = append(args, limit)
	}
	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close() //nolint:errcheck
	out := []runtimeConversationBatchCursorV2{}
	for rows.Next() {
		var cursor runtimeConversationBatchCursorV2
		if err := rows.Scan(&cursor.previous, &cursor.sequence, &cursor.entityCount, &cursor.encodedBytes); err != nil {
			return nil, err
		}
		out = append(out, cursor)
	}
	return out, rows.Err()
}

func (s runtimeConversationEventStoreV2) listAfter(ctx context.Context, sessionID string, after int64) ([]RuntimeConversationEntityEventV2, error) {
	if err := s.ensure(ctx); err != nil {
		return nil, err
	}
	rows, err := s.db.QueryContext(ctx, `SELECT event_json FROM conversation_entity_events_v2 WHERE session_id=? AND raw_sequence>? ORDER BY raw_sequence,ordinal`, sessionID, after)
	if err != nil {
		return nil, err
	}
	defer rows.Close() //nolint:errcheck
	out := []RuntimeConversationEntityEventV2{}
	for rows.Next() {
		var raw string
		if err := rows.Scan(&raw); err != nil {
			return nil, err
		}
		var event RuntimeConversationEntityEventV2
		if err := json.Unmarshal([]byte(raw), &event); err != nil {
			return nil, err
		}
		out = append(out, event)
	}
	return out, rows.Err()
}

func (s runtimeConversationEventStoreV2) listRange(ctx context.Context, sessionID string, after, through int64) ([]RuntimeConversationEntityEventV2, error) {
	if err := s.ensure(ctx); err != nil {
		return nil, err
	}
	rows, err := s.db.QueryContext(ctx, `SELECT event_json FROM conversation_entity_events_v2 WHERE session_id=? AND raw_sequence>? AND raw_sequence<=? ORDER BY raw_sequence,ordinal`, sessionID, after, through)
	if err != nil {
		return nil, err
	}
	defer rows.Close() //nolint:errcheck
	out := []RuntimeConversationEntityEventV2{}
	for rows.Next() {
		var raw string
		if err := rows.Scan(&raw); err != nil {
			return nil, err
		}
		var event RuntimeConversationEntityEventV2
		if err := json.Unmarshal([]byte(raw), &event); err != nil {
			return nil, err
		}
		out = append(out, event)
	}
	return out, rows.Err()
}
