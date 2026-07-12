package runtime

import (
	"context"
	"encoding/json"
	"errors"
	"reflect"
	"strconv"
	"time"
)

func (s runtimeConversationEventStoreV2) seedSnapshot(ctx context.Context, snapshot RuntimeCanonicalConversationSnapshot) error {
	if err := s.ensure(ctx); err != nil {
		return err
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback() //nolint:errcheck
	if _, err = tx.ExecContext(ctx, `DELETE FROM conversation_entities_v2 WHERE session_id=?`, snapshot.SessionID); err != nil {
		return err
	}
	for _, entity := range canonicalSnapshotStateRows(snapshot) {
		if _, err = tx.ExecContext(ctx, `INSERT INTO conversation_entities_v2(session_id,entity_type,entity_id,turn_id,activity_sequence,revision,entity_json,updated_at) VALUES(?,?,?,?,?,?,?,?)`, snapshot.SessionID, entity.kind, entity.meta.ID, nullableString(entity.meta.TurnID), entity.meta.ActivitySequence, entity.meta.Revision, entity.raw, entity.meta.UpdatedAt); err != nil {
			return err
		}
	}
	cursor, err := strconv.ParseInt(snapshot.Cursor, 10, 64)
	if err != nil {
		return err
	}
	if _, err = tx.ExecContext(ctx, `INSERT INTO conversation_projector_checkpoints_v2(session_id,last_raw_sequence,failure_reason,updated_at) VALUES(?,?,NULL,?) ON CONFLICT(session_id) DO UPDATE SET last_raw_sequence=excluded.last_raw_sequence,failure_reason=NULL,updated_at=excluded.updated_at`, snapshot.SessionID, cursor, time.Now().UnixMilli()); err != nil {
		return err
	}
	return tx.Commit()
}

func (s runtimeConversationEventStoreV2) loadSnapshot(ctx context.Context, sessionID string, checkpoint int64) (RuntimeCanonicalConversationSnapshot, error) {
	if err := s.ensure(ctx); err != nil {
		return RuntimeCanonicalConversationSnapshot{}, err
	}
	snapshot := RuntimeCanonicalConversationSnapshot{SchemaVersion: RuntimeConversationSchemaVersion, SessionID: sessionID, Cursor: strconv.FormatInt(checkpoint, 10), Scope: RuntimeConversationScopeFull, Turns: []RuntimeCanonicalTurn{}, Messages: []RuntimeCanonicalMessage{}, AssistantSteps: []RuntimeCanonicalAssistantStep{}, ToolCalls: []RuntimeCanonicalToolCall{}, ToolResults: []RuntimeCanonicalToolResult{}, Permissions: []RuntimeCanonicalPermission{}, TodoPlans: []RuntimeCanonicalTodoPlan{}, AgentTasks: []RuntimeCanonicalAgentTask{}, Notices: []RuntimeCanonicalNotice{}}
	rows, err := s.db.QueryContext(ctx, `SELECT entity_type,entity_json FROM conversation_entities_v2 WHERE session_id=? ORDER BY entity_type,entity_id`, sessionID)
	if err != nil {
		return RuntimeCanonicalConversationSnapshot{}, err
	}
	defer rows.Close() //nolint:errcheck
	for rows.Next() {
		var kind, raw string
		if err := rows.Scan(&kind, &raw); err != nil {
			return RuntimeCanonicalConversationSnapshot{}, err
		}
		switch kind {
		case RuntimeConversationEntityTurn:
			var v RuntimeCanonicalTurn
			err = json.Unmarshal([]byte(raw), &v)
			snapshot.Turns = append(snapshot.Turns, v)
		case RuntimeConversationEntityMessage:
			var v RuntimeCanonicalMessage
			err = json.Unmarshal([]byte(raw), &v)
			snapshot.Messages = append(snapshot.Messages, v)
		case RuntimeConversationEntityAssistantStep:
			var v RuntimeCanonicalAssistantStep
			err = json.Unmarshal([]byte(raw), &v)
			snapshot.AssistantSteps = append(snapshot.AssistantSteps, v)
		case RuntimeConversationEntityToolCall:
			var v RuntimeCanonicalToolCall
			err = json.Unmarshal([]byte(raw), &v)
			snapshot.ToolCalls = append(snapshot.ToolCalls, v)
		case RuntimeConversationEntityToolResult:
			var v RuntimeCanonicalToolResult
			err = json.Unmarshal([]byte(raw), &v)
			snapshot.ToolResults = append(snapshot.ToolResults, v)
		case RuntimeConversationEntityPermission:
			var v RuntimeCanonicalPermission
			err = json.Unmarshal([]byte(raw), &v)
			snapshot.Permissions = append(snapshot.Permissions, v)
		case RuntimeConversationEntityTodoPlan:
			var v RuntimeCanonicalTodoPlan
			err = json.Unmarshal([]byte(raw), &v)
			snapshot.TodoPlans = append(snapshot.TodoPlans, v)
		case RuntimeConversationEntityAgentTask:
			var v RuntimeCanonicalAgentTask
			err = json.Unmarshal([]byte(raw), &v)
			snapshot.AgentTasks = append(snapshot.AgentTasks, v)
		case RuntimeConversationEntityNotice:
			var v RuntimeCanonicalNotice
			err = json.Unmarshal([]byte(raw), &v)
			snapshot.Notices = append(snapshot.Notices, v)
		}
		if err != nil {
			return RuntimeCanonicalConversationSnapshot{}, err
		}
	}
	if err := rows.Err(); err != nil {
		return RuntimeCanonicalConversationSnapshot{}, err
	}
	canonicalSortSnapshot(&snapshot)
	return snapshot, nil
}

func (s runtimeConversationEventStoreV2) commitProjectedRaw(ctx context.Context, raw RuntimeEvent, events []RuntimeConversationEntityEventV2) error {
	if err := s.ensure(ctx); err != nil {
		return err
	}
	payload, err := json.Marshal(redactRuntimePayload(raw.Payload))
	if err != nil {
		return err
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback() //nolint:errcheck
	if _, err = tx.ExecContext(ctx, `INSERT INTO runtime_events(sequence,id,type,session_id,turn_id,message_id,tool_call_id,payload_json,created_at) VALUES(?,?,?,?,?,?,?,?,?)`, raw.Sequence, raw.ID, raw.Type, nullableString(raw.SessionID), nullableString(raw.TurnID), nullableString(raw.MessageID), nullableString(raw.ToolCallID), string(payload), raw.CreatedAt); err != nil {
		return err
	}
	var previous int64
	if err = tx.QueryRowContext(ctx, `SELECT last_raw_sequence FROM conversation_projector_checkpoints_v2 WHERE session_id=?`, raw.SessionID).Scan(&previous); err != nil {
		return err
	}
	for ordinal, event := range events {
		encoded, marshalErr := json.Marshal(event)
		if marshalErr != nil {
			return marshalErr
		}
		if _, err = tx.ExecContext(ctx, `INSERT INTO conversation_entity_events_v2(session_id,raw_sequence,ordinal,event_id,entity_type,entity_id,operation,revision,event_json,created_at) VALUES(?,?,?,?,?,?,?,?,?,?)`, raw.SessionID, raw.Sequence, ordinal, event.ID, event.EntityType, event.EntityID, event.Operation, event.Revision, string(encoded), event.CreatedAt); err != nil {
			return err
		}
		if event.Operation == RuntimeConversationOperationDelete {
			if _, err = tx.ExecContext(ctx, `DELETE FROM conversation_entities_v2 WHERE session_id=? AND entity_type=? AND entity_id=?`, raw.SessionID, event.EntityType, event.EntityID); err != nil {
				return err
			}
		} else {
			meta := event.payloadMeta()
			entityRaw, marshalErr := canonicalEventEntityJSON(event)
			if marshalErr != nil {
				return marshalErr
			}
			if _, err = tx.ExecContext(ctx, `INSERT INTO conversation_entities_v2(session_id,entity_type,entity_id,turn_id,activity_sequence,revision,entity_json,updated_at) VALUES(?,?,?,?,?,?,?,?) ON CONFLICT(session_id,entity_type,entity_id) DO UPDATE SET turn_id=excluded.turn_id,activity_sequence=conversation_entities_v2.activity_sequence,revision=excluded.revision,entity_json=excluded.entity_json,updated_at=excluded.updated_at`, raw.SessionID, event.EntityType, event.EntityID, nullableString(meta.TurnID), meta.ActivitySequence, meta.Revision, string(entityRaw), meta.UpdatedAt); err != nil {
				return err
			}
		}
	}
	if _, err = tx.ExecContext(ctx, `INSERT INTO conversation_projector_batches_v2(session_id,raw_sequence,previous_raw_sequence,entity_count,created_at) VALUES(?,?,?,?,?)`, raw.SessionID, raw.Sequence, previous, len(events), time.Now().UnixMilli()); err != nil {
		return err
	}
	if _, err = tx.ExecContext(ctx, `UPDATE conversation_projector_checkpoints_v2 SET last_raw_sequence=?,failure_reason=NULL,updated_at=? WHERE session_id=?`, raw.Sequence, time.Now().UnixMilli(), raw.SessionID); err != nil {
		return err
	}
	return tx.Commit()
}

type canonicalSnapshotStateRow struct {
	kind string
	meta RuntimeConversationEntityMeta
	raw  string
}

func canonicalSnapshotStateRows(s RuntimeCanonicalConversationSnapshot) []canonicalSnapshotStateRow {
	out := []canonicalSnapshotStateRow{}
	add := func(kind string, meta RuntimeConversationEntityMeta, value any) {
		raw, _ := json.Marshal(value)
		out = append(out, canonicalSnapshotStateRow{kind: kind, meta: meta, raw: string(raw)})
	}
	for _, v := range s.Turns {
		add(RuntimeConversationEntityTurn, v.RuntimeConversationEntityMeta, v)
	}
	for _, v := range s.Messages {
		add(RuntimeConversationEntityMessage, v.RuntimeConversationEntityMeta, v)
	}
	for _, v := range s.AssistantSteps {
		add(RuntimeConversationEntityAssistantStep, v.RuntimeConversationEntityMeta, v)
	}
	for _, v := range s.ToolCalls {
		add(RuntimeConversationEntityToolCall, v.RuntimeConversationEntityMeta, v)
	}
	for _, v := range s.ToolResults {
		add(RuntimeConversationEntityToolResult, v.RuntimeConversationEntityMeta, v)
	}
	for _, v := range s.Permissions {
		add(RuntimeConversationEntityPermission, v.RuntimeConversationEntityMeta, v)
	}
	for _, v := range s.TodoPlans {
		add(RuntimeConversationEntityTodoPlan, v.RuntimeConversationEntityMeta, v)
	}
	for _, v := range s.AgentTasks {
		add(RuntimeConversationEntityAgentTask, v.RuntimeConversationEntityMeta, v)
	}
	for _, v := range s.Notices {
		add(RuntimeConversationEntityNotice, v.RuntimeConversationEntityMeta, v)
	}
	return out
}
func canonicalEventEntityJSON(event RuntimeConversationEntityEventV2) ([]byte, error) {
	switch event.EntityType {
	case RuntimeConversationEntityTurn:
		return json.Marshal(event.Turn)
	case RuntimeConversationEntityMessage:
		return json.Marshal(event.Message)
	case RuntimeConversationEntityAssistantStep:
		return json.Marshal(event.AssistantStep)
	case RuntimeConversationEntityToolCall:
		return json.Marshal(event.ToolCall)
	case RuntimeConversationEntityToolResult:
		return json.Marshal(event.ToolResult)
	case RuntimeConversationEntityPermission:
		return json.Marshal(event.Permission)
	case RuntimeConversationEntityTodoPlan:
		return json.Marshal(event.TodoPlan)
	case RuntimeConversationEntityAgentTask:
		return json.Marshal(event.AgentTask)
	case RuntimeConversationEntityNotice:
		return json.Marshal(event.Notice)
	}
	return nil, errors.New("unknown canonical entity type")
}

func (s runtimeConversationEventStoreV2) bootstrapRaw(ctx context.Context, snapshot RuntimeCanonicalConversationSnapshot, raw RuntimeEvent) error {
	if err := s.ensure(ctx); err != nil {
		return err
	}
	payload, err := json.Marshal(redactRuntimePayload(raw.Payload))
	if err != nil {
		return err
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback() //nolint:errcheck
	if _, err = tx.ExecContext(ctx, `INSERT INTO runtime_events(sequence,id,type,session_id,turn_id,message_id,tool_call_id,payload_json,created_at) VALUES(?,?,?,?,?,?,?,?,?)`, raw.Sequence, raw.ID, raw.Type, nullableString(raw.SessionID), nullableString(raw.TurnID), nullableString(raw.MessageID), nullableString(raw.ToolCallID), string(payload), raw.CreatedAt); err != nil {
		return err
	}
	if _, err = tx.ExecContext(ctx, `DELETE FROM conversation_entities_v2 WHERE session_id=?`, snapshot.SessionID); err != nil {
		return err
	}
	for _, entity := range canonicalSnapshotStateRows(snapshot) {
		if _, err = tx.ExecContext(ctx, `INSERT INTO conversation_entities_v2(session_id,entity_type,entity_id,turn_id,activity_sequence,revision,entity_json,updated_at) VALUES(?,?,?,?,?,?,?,?)`, snapshot.SessionID, entity.kind, entity.meta.ID, nullableString(entity.meta.TurnID), entity.meta.ActivitySequence, entity.meta.Revision, entity.raw, entity.meta.UpdatedAt); err != nil {
			return err
		}
	}
	if _, err = tx.ExecContext(ctx, `INSERT INTO conversation_projector_checkpoints_v2(session_id,last_raw_sequence,failure_reason,updated_at) VALUES(?,?,NULL,?)`, snapshot.SessionID, raw.Sequence, time.Now().UnixMilli()); err != nil {
		return err
	}
	return tx.Commit()
}

func (s runtimeConversationEventStoreV2) markFailure(ctx context.Context, sessionID, reason string) error {
	if err := s.ensure(ctx); err != nil {
		return err
	}
	_, err := s.db.ExecContext(ctx, `UPDATE conversation_projector_checkpoints_v2 SET failure_reason=?,updated_at=? WHERE session_id=?`, reason, time.Now().UnixMilli(), sessionID)
	return err
}

func canonicalSemanticSnapshotsEqual(a, b RuntimeCanonicalConversationSnapshot) bool {
	return reflect.DeepEqual(canonicalComparableState(a), canonicalComparableState(b))
}
func canonicalComparableState(s RuntimeCanonicalConversationSnapshot) map[string]string {
	out := map[string]string{}
	for _, row := range canonicalSnapshotStateRows(s) {
		var value map[string]any
		if json.Unmarshal([]byte(row.raw), &value) != nil {
			continue
		}
		delete(value, "activitySequence")
		delete(value, "revision")
		delete(value, "updatedAt")
		encoded, _ := json.Marshal(value)
		out[row.kind+":"+row.meta.ID] = string(encoded)
	}
	return out
}
