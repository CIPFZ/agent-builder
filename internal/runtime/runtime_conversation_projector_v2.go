package runtime

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/CIPFZ/agent-builder/internal/runtimeapi"
)

// projectCanonicalConversationEventV2 freezes semantic entity payloads at the
// raw event boundary. Catch-up reads this outbox; it never reinterprets an old
// raw sequence using a later store state.
func (r *runtimeService) projectCanonicalConversationEventV2(ctx context.Context, raw RuntimeEvent) error {
	if raw.Sequence <= 0 || strings.TrimSpace(raw.SessionID) == "" || runtimeapi.IsEphemeralEventType(raw.Type) {
		return nil
	}
	r.conversationV2Mu.Lock()
	defer r.conversationV2Mu.Unlock()
	store := newRuntimeConversationEventStoreV2(r.eventStore.db)
	checkpoint, failureReason, exists, err := store.checkpoint(ctx, raw.SessionID)
	if err != nil {
		return err
	}
	if failureReason != "" {
		return fmt.Errorf("canonical projector requires snapshot recovery: %s", failureReason)
	}
	pending := r.pendingCanonicalRawEvents(raw.SessionID, checkpoint, raw)
	if len(pending) == 0 {
		return nil
	}
	if !exists {
		first := pending[0]
		snapshot, buildErr := r.buildSessionConversationSnapshotV2At(ctx, first.SessionID, RuntimeCanonicalConversationSnapshotRequest{}, first.Sequence, &first)
		if buildErr != nil {
			return buildErr
		}
		snapshot.Cursor = strconv.FormatInt(first.Sequence, 10)
		if err := store.bootstrapRaw(ctx, snapshot, first); err != nil {
			return err
		}
		r.removePendingCanonicalRaw(first)
		checkpoint = first.Sequence
		pending = pending[1:]
	}
	for index, event := range pending {
		if err := r.projectCanonicalRawLocked(ctx, store, checkpoint, event, index < len(pending)-1); err != nil {
			return err
		}
		checkpoint = event.Sequence
		r.removePendingCanonicalRaw(event)
	}
	return nil
}

func (r *runtimeService) projectCanonicalRawLocked(ctx context.Context, store runtimeConversationEventStoreV2, checkpoint int64, raw RuntimeEvent, hasLater bool) error {
	if hasLater {
		r.conversationV2Deferred[raw.SessionID] = true
		return store.commitProjectedRaw(ctx, raw, []RuntimeConversationEntityEventV2{})
	}
	previous, err := store.loadSnapshot(ctx, raw.SessionID, checkpoint)
	if err != nil {
		return err
	}
	snapshot := RuntimeCanonicalConversationSnapshot{}
	if raw.Type != runtimeapi.EventSessionDeleted {
		snapshot, err = r.buildSessionConversationSnapshotV2At(ctx, raw.SessionID, RuntimeCanonicalConversationSnapshotRequest{}, raw.Sequence, &raw)
		if err != nil {
			return err
		}
	}
	var events []RuntimeConversationEntityEventV2
	deferred := r.conversationV2Deferred[raw.SessionID]
	delete(r.conversationV2Deferred, raw.SessionID)
	if deferred || raw.Type == runtimeapi.EventMessageDeleted || raw.Type == runtimeapi.EventSessionDeleted || raw.Type == runtimeapi.EventConversationReconciled {
		events, err = canonicalDiffEntityEvents(raw, previous, snapshot)
	} else {
		events, err = canonicalEntityEventsForRaw(raw, snapshot)
	}
	if err != nil {
		return err
	}
	if err := store.commitProjectedRaw(ctx, raw, events); err != nil {
		return err
	}
	return nil
}

func (r *runtimeService) pendingCanonicalRawEvents(sessionID string, checkpoint int64, current RuntimeEvent) []RuntimeEvent {
	r.mu.Lock()
	pending := []RuntimeEvent{}
	seen := map[int64]bool{}
	for _, event := range r.conversationV2Pending[sessionID] {
		if event.Sequence > checkpoint && !runtimeapi.IsEphemeralEventType(event.Type) {
			pending = append(pending, event)
			seen[event.Sequence] = true
		}
	}
	r.mu.Unlock()
	if current.Sequence > checkpoint && !seen[current.Sequence] {
		pending = append(pending, current)
	}
	sort.Slice(pending, func(i, j int) bool { return pending[i].Sequence < pending[j].Sequence })
	return pending
}

func (r *runtimeService) removePendingCanonicalRaw(raw RuntimeEvent) {
	r.mu.Lock()
	if pending := r.conversationV2Pending[raw.SessionID]; pending != nil {
		delete(pending, raw.Sequence)
		if len(pending) == 0 {
			delete(r.conversationV2Pending, raw.SessionID)
		}
	}
	r.mu.Unlock()
}
func (r *runtimeService) removePendingCanonicalThrough(sessionID string, sequence int64) {
	r.mu.Lock()
	if pending := r.conversationV2Pending[sessionID]; pending != nil {
		for seq := range pending {
			if seq <= sequence {
				delete(pending, seq)
			}
		}
		if len(pending) == 0 {
			delete(r.conversationV2Pending, sessionID)
		}
	}
	r.mu.Unlock()
}

func canonicalDiffEntityEvents(raw RuntimeEvent, previous, current RuntimeCanonicalConversationSnapshot) ([]RuntimeConversationEntityEventV2, error) {
	before := map[string]canonicalSnapshotStateRow{}
	after := map[string]canonicalSnapshotStateRow{}
	for _, row := range canonicalSnapshotStateRows(previous) {
		before[row.kind+":"+row.meta.ID] = row
	}
	for _, row := range canonicalSnapshotStateRows(current) {
		after[row.kind+":"+row.meta.ID] = row
	}
	out := []RuntimeConversationEntityEventV2{}
	for key, row := range after {
		old, exists := before[key]
		if exists && old.raw == row.raw {
			continue
		}
		row = canonicalStateRowAtSequence(row, raw.Sequence)
		event, err := canonicalUpsertEventFromStateRow(raw, row)
		if err != nil {
			return nil, err
		}
		out = append(out, event)
	}
	for key, row := range before {
		if _, exists := after[key]; exists {
			continue
		}
		event := RuntimeConversationEntityEventV2{SchemaVersion: RuntimeConversationSchemaVersion, ID: fmt.Sprintf("conversation-v2:%s:%s:%s", raw.ID, row.kind, row.meta.ID), SessionID: raw.SessionID, TurnID: row.meta.TurnID, Sequence: strconv.FormatInt(raw.Sequence, 10), CreatedAt: parseRuntimeEventMillis(raw.CreatedAt), EntityType: row.kind, EntityID: row.meta.ID, Operation: RuntimeConversationOperationDelete, Revision: strconv.FormatInt(raw.Sequence, 10), TombstoneReason: firstNonEmpty(ternary(raw.Type == runtimeapi.EventSessionDeleted, "session_deleted", ""), ternary(raw.Type == runtimeapi.EventMessageDeleted, "message_deleted", "entity_removed"))}
		out = append(out, event)
	}
	sort.Slice(out, func(i, j int) bool {
		ri, rj := canonicalEntityRank(out[i].EntityType), canonicalEntityRank(out[j].EntityType)
		if ri != rj {
			return ri < rj
		}
		return out[i].EntityID < out[j].EntityID
	})
	for _, event := range out {
		if err := event.Validate(); err != nil {
			return nil, err
		}
	}
	return out, nil
}

func canonicalStateRowAtSequence(row canonicalSnapshotStateRow, sequence int64) canonicalSnapshotStateRow {
	revision := strconv.FormatInt(sequence, 10)
	if row.meta.ActivitySequence == "0" {
		row.meta.ActivitySequence = revision
	}
	row.meta.Revision = revision
	switch row.kind {
	case RuntimeConversationEntityTurn:
		var v RuntimeCanonicalTurn
		_ = json.Unmarshal([]byte(row.raw), &v)
		v.RuntimeConversationEntityMeta = row.meta
		raw, _ := json.Marshal(v)
		row.raw = string(raw)
	case RuntimeConversationEntityMessage:
		var v RuntimeCanonicalMessage
		_ = json.Unmarshal([]byte(row.raw), &v)
		v.RuntimeConversationEntityMeta = row.meta
		raw, _ := json.Marshal(v)
		row.raw = string(raw)
	case RuntimeConversationEntityAssistantStep:
		var v RuntimeCanonicalAssistantStep
		_ = json.Unmarshal([]byte(row.raw), &v)
		v.RuntimeConversationEntityMeta = row.meta
		raw, _ := json.Marshal(v)
		row.raw = string(raw)
	case RuntimeConversationEntityToolCall:
		var v RuntimeCanonicalToolCall
		_ = json.Unmarshal([]byte(row.raw), &v)
		v.RuntimeConversationEntityMeta = row.meta
		raw, _ := json.Marshal(v)
		row.raw = string(raw)
	case RuntimeConversationEntityToolResult:
		var v RuntimeCanonicalToolResult
		_ = json.Unmarshal([]byte(row.raw), &v)
		v.RuntimeConversationEntityMeta = row.meta
		raw, _ := json.Marshal(v)
		row.raw = string(raw)
	case RuntimeConversationEntityPermission:
		var v RuntimeCanonicalPermission
		_ = json.Unmarshal([]byte(row.raw), &v)
		v.RuntimeConversationEntityMeta = row.meta
		raw, _ := json.Marshal(v)
		row.raw = string(raw)
	case RuntimeConversationEntityTodoPlan:
		var v RuntimeCanonicalTodoPlan
		_ = json.Unmarshal([]byte(row.raw), &v)
		v.RuntimeConversationEntityMeta = row.meta
		raw, _ := json.Marshal(v)
		row.raw = string(raw)
	case RuntimeConversationEntityAgentTask:
		var v RuntimeCanonicalAgentTask
		_ = json.Unmarshal([]byte(row.raw), &v)
		v.RuntimeConversationEntityMeta = row.meta
		raw, _ := json.Marshal(v)
		row.raw = string(raw)
	case RuntimeConversationEntityNotice:
		var v RuntimeCanonicalNotice
		_ = json.Unmarshal([]byte(row.raw), &v)
		v.RuntimeConversationEntityMeta = row.meta
		raw, _ := json.Marshal(v)
		row.raw = string(raw)
	}
	return row
}

func canonicalUpsertEventFromStateRow(raw RuntimeEvent, row canonicalSnapshotStateRow) (RuntimeConversationEntityEventV2, error) {
	event := RuntimeConversationEntityEventV2{SchemaVersion: RuntimeConversationSchemaVersion, ID: fmt.Sprintf("conversation-v2:%s:%s:%s", raw.ID, row.kind, row.meta.ID), SessionID: raw.SessionID, TurnID: row.meta.TurnID, Sequence: strconv.FormatInt(raw.Sequence, 10), CreatedAt: parseRuntimeEventMillis(raw.CreatedAt), EntityType: row.kind, EntityID: row.meta.ID, Operation: RuntimeConversationOperationUpsert, Revision: row.meta.Revision}
	switch row.kind {
	case RuntimeConversationEntityTurn:
		var v RuntimeCanonicalTurn
		_ = json.Unmarshal([]byte(row.raw), &v)
		event.Turn = &v
	case RuntimeConversationEntityMessage:
		var v RuntimeCanonicalMessage
		_ = json.Unmarshal([]byte(row.raw), &v)
		event.Message = &v
	case RuntimeConversationEntityAssistantStep:
		var v RuntimeCanonicalAssistantStep
		_ = json.Unmarshal([]byte(row.raw), &v)
		event.AssistantStep = &v
	case RuntimeConversationEntityToolCall:
		var v RuntimeCanonicalToolCall
		_ = json.Unmarshal([]byte(row.raw), &v)
		event.ToolCall = &v
	case RuntimeConversationEntityToolResult:
		var v RuntimeCanonicalToolResult
		_ = json.Unmarshal([]byte(row.raw), &v)
		event.ToolResult = &v
	case RuntimeConversationEntityPermission:
		var v RuntimeCanonicalPermission
		_ = json.Unmarshal([]byte(row.raw), &v)
		event.Permission = &v
	case RuntimeConversationEntityTodoPlan:
		var v RuntimeCanonicalTodoPlan
		_ = json.Unmarshal([]byte(row.raw), &v)
		event.TodoPlan = &v
	case RuntimeConversationEntityAgentTask:
		var v RuntimeCanonicalAgentTask
		_ = json.Unmarshal([]byte(row.raw), &v)
		event.AgentTask = &v
	case RuntimeConversationEntityNotice:
		var v RuntimeCanonicalNotice
		_ = json.Unmarshal([]byte(row.raw), &v)
		event.Notice = &v
	}
	return event, event.Validate()
}

func canonicalEntityEventsForRaw(raw RuntimeEvent, s RuntimeCanonicalConversationSnapshot) ([]RuntimeConversationEntityEventV2, error) {
	turns := map[string]RuntimeCanonicalTurn{}
	messages := map[string]RuntimeCanonicalMessage{}
	steps := map[string]RuntimeCanonicalAssistantStep{}
	calls := map[string]RuntimeCanonicalToolCall{}
	results := map[string]RuntimeCanonicalToolResult{}
	perms := map[string]RuntimeCanonicalPermission{}
	todos := map[string]RuntimeCanonicalTodoPlan{}
	tasks := map[string]RuntimeCanonicalAgentTask{}
	notices := map[string]RuntimeCanonicalNotice{}
	for _, v := range s.Turns {
		turns[v.ID] = v
	}
	for _, v := range s.Messages {
		messages[v.ID] = v
	}
	for _, v := range s.AssistantSteps {
		steps[v.ID] = v
	}
	for _, v := range s.ToolCalls {
		calls[v.ID] = v
	}
	for _, v := range s.ToolResults {
		results[v.ID] = v
	}
	for _, v := range s.Permissions {
		perms[v.ID] = v
	}
	for _, v := range s.TodoPlans {
		todos[v.ID] = v
	}
	for _, v := range s.AgentTasks {
		tasks[v.ID] = v
	}
	for _, v := range s.Notices {
		notices[v.ID] = v
	}
	out := []RuntimeConversationEntityEventV2{}
	created := parseRuntimeEventMillis(raw.CreatedAt)
	if created == 0 {
		created = time.Now().UnixMilli()
	}
	base := func(kind, id, turnID, revision string) RuntimeConversationEntityEventV2 {
		return RuntimeConversationEntityEventV2{SchemaVersion: RuntimeConversationSchemaVersion, ID: fmt.Sprintf("conversation-v2:%s:%s:%s", raw.ID, kind, id), SessionID: raw.SessionID, TurnID: turnID, Sequence: strconv.FormatInt(raw.Sequence, 10), CreatedAt: created, EntityType: kind, EntityID: id, Operation: RuntimeConversationOperationUpsert, Revision: revision}
	}
	addTurn := func(id string) {
		if v, ok := turns[id]; ok {
			e := base(RuntimeConversationEntityTurn, id, v.TurnID, v.Revision)
			e.Turn = &v
			out = append(out, e)
		}
	}
	addMessage := func(id string) {
		if v, ok := messages[id]; ok {
			e := base(RuntimeConversationEntityMessage, id, v.TurnID, v.Revision)
			e.Message = &v
			out = append(out, e)
			if v.AssistantStepID != "" {
				if step, ok := steps[v.AssistantStepID]; ok {
					se := base(RuntimeConversationEntityAssistantStep, step.ID, step.TurnID, step.Revision)
					se.AssistantStep = &step
					out = append(out, se)
				}
			}
			for _, result := range results {
				if strings.HasPrefix(result.ID, "tool-result:"+id+":") {
					re := base(RuntimeConversationEntityToolResult, result.ID, result.TurnID, result.Revision)
					re.ToolResult = &result
					out = append(out, re)
					if call, ok := calls[result.ToolCallID]; ok {
						ce := base(RuntimeConversationEntityToolCall, call.ID, call.TurnID, call.Revision)
						ce.ToolCall = &call
						out = append(out, ce)
					}
				}
			}
		}
	}
	addCall := func(id string) {
		if v, ok := calls[id]; ok {
			e := base(RuntimeConversationEntityToolCall, id, v.TurnID, v.Revision)
			e.ToolCall = &v
			out = append(out, e)
		}
	}
	addPermission := func(id string) {
		if v, ok := perms[id]; ok {
			e := base(RuntimeConversationEntityPermission, id, v.TurnID, v.Revision)
			e.Permission = &v
			out = append(out, e)
		}
	}
	addTodo := func(id string) {
		if v, ok := todos[id]; ok {
			e := base(RuntimeConversationEntityTodoPlan, id, v.TurnID, v.Revision)
			e.TodoPlan = &v
			out = append(out, e)
		}
	}
	addTask := func(id string) {
		if v, ok := tasks[id]; ok {
			e := base(RuntimeConversationEntityAgentTask, id, v.TurnID, v.Revision)
			e.AgentTask = &v
			out = append(out, e)
		}
	}
	addNotice := func(id string) {
		if v, ok := notices[id]; ok {
			e := base(RuntimeConversationEntityNotice, id, v.TurnID, v.Revision)
			e.Notice = &v
			out = append(out, e)
		}
	}
	switch raw.Type {
	case runtimeapi.EventTurnStarted, runtimeapi.EventTurnCompleted, runtimeapi.EventTurnFailed, runtimeapi.EventTurnCancelled, runtimeapi.EventTurnInterrupted:
		addTurn(raw.TurnID)
		for _, plan := range todos {
			if plan.OwnerTurnID == raw.TurnID {
				addTodo(plan.ID)
			}
		}
		if turn, ok := turns[raw.TurnID]; ok && turn.FinalMessageID != "" {
			addMessage(turn.FinalMessageID)
		}
	case runtimeapi.EventMessageCreated, runtimeapi.EventMessageUpdated, runtimeapi.EventMessageCompleted:
		addMessage(raw.MessageID)
	case runtimeapi.EventMessageDeleted:
		ids := append([]string{raw.MessageID}, stringSliceFromMap(raw.Payload, "derived_entity_ids")...)
		for _, id := range ids {
			kind := RuntimeConversationEntityMessage
			if strings.HasPrefix(id, "assistant-step:") {
				kind = RuntimeConversationEntityAssistantStep
			} else if strings.HasPrefix(id, "tool-result:") {
				kind = RuntimeConversationEntityToolResult
			}
			e := base(kind, id, raw.TurnID, strconv.FormatInt(raw.Sequence, 10))
			e.Operation = RuntimeConversationOperationDelete
			e.TombstoneReason = "message_deleted"
			out = append(out, e)
		}
	case runtimeapi.EventToolCallStarted, runtimeapi.EventToolCallOutput, runtimeapi.EventToolCallCompleted, runtimeapi.EventToolCallFailed, runtimeapi.EventToolCallCancelled:
		addCall(raw.ToolCallID)
	case runtimeapi.EventPermissionRequested, runtimeapi.EventPermissionDecided:
		addPermission(stringFromMap(raw.Payload, "permission_id"))
	case runtimeapi.EventTodoUpdated:
		addTodo(stringFromMap(raw.Payload, "plan_id"))
	case runtimeapi.EventTaskStarted, runtimeapi.EventTaskProgress, runtimeapi.EventTaskCompleted, runtimeapi.EventTaskFailed, runtimeapi.EventTaskCancelled, runtimeapi.EventTaskInterrupted, runtimeapi.EventTaskRoleLoaded, runtimeapi.EventTaskScopeApplied, runtimeapi.EventTaskScopeDenied, runtimeapi.EventTaskMessageCreated, runtimeapi.EventTaskMessageDelivered, runtimeapi.EventTaskMessageProcessed, runtimeapi.EventTaskMessageRejected, runtimeapi.EventTaskResultUpdated, runtimeapi.EventTaskArtifactCreated:
		addTask(firstNonEmpty(stringFromMap(raw.Payload, "task_id"), stringFromMap(raw.Payload, "agent_task_id")))
	case runtimeapi.EventSessionDeleted:
		for _, ref := range canonicalEntityRefsFromPayload(raw.Payload["entity_refs"]) {
			kind, id, turnID := stringFromMap(ref, "entity_type"), stringFromMap(ref, "entity_id"), stringFromMap(ref, "turn_id")
			if kind == "" || id == "" {
				continue
			}
			e := base(kind, id, turnID, strconv.FormatInt(raw.Sequence, 10))
			e.Operation = RuntimeConversationOperationDelete
			e.TombstoneReason = "session_deleted"
			out = append(out, e)
		}
	}
	if id, _, ok := canonicalNoticeIdentity(raw); ok {
		addNotice(id)
	}
	// One raw sequence is an atomic group. Remove dependency duplicates then
	// sort by semantic rank and stable ID.
	unique := map[string]RuntimeConversationEntityEventV2{}
	for _, e := range out {
		unique[e.EntityType+":"+e.EntityID+":"+e.Operation] = e
	}
	out = out[:0]
	for _, e := range unique {
		if err := e.Validate(); err != nil {
			return nil, err
		}
		out = append(out, e)
	}
	sort.Slice(out, func(i, j int) bool {
		ri := canonicalEntityRank(out[i].EntityType)
		rj := canonicalEntityRank(out[j].EntityType)
		if ri != rj {
			return ri < rj
		}
		if out[i].EntityID != out[j].EntityID {
			return out[i].EntityID < out[j].EntityID
		}
		return out[i].Operation < out[j].Operation
	})
	return out, nil
}

func canonicalEntityRank(kind string) int {
	switch kind {
	case RuntimeConversationEntityTurn:
		return 0
	case RuntimeConversationEntityMessage:
		return 1
	case RuntimeConversationEntityAssistantStep:
		return 2
	case RuntimeConversationEntityToolCall:
		return 3
	case RuntimeConversationEntityToolResult:
		return 4
	case RuntimeConversationEntityPermission:
		return 5
	case RuntimeConversationEntityTodoPlan:
		return 6
	case RuntimeConversationEntityAgentTask:
		return 7
	case RuntimeConversationEntityNotice:
		return 8
	}
	return 99
}

func canonicalConversationTombstoneRefs(s RuntimeCanonicalConversationSnapshot) []map[string]any {
	out := []map[string]any{}
	add := func(kind string, meta RuntimeConversationEntityMeta) {
		out = append(out, map[string]any{"entity_type": kind, "entity_id": meta.ID, "turn_id": meta.TurnID})
	}
	for _, v := range s.Turns {
		add(RuntimeConversationEntityTurn, v.RuntimeConversationEntityMeta)
	}
	for _, v := range s.Messages {
		add(RuntimeConversationEntityMessage, v.RuntimeConversationEntityMeta)
	}
	for _, v := range s.AssistantSteps {
		add(RuntimeConversationEntityAssistantStep, v.RuntimeConversationEntityMeta)
	}
	for _, v := range s.ToolCalls {
		add(RuntimeConversationEntityToolCall, v.RuntimeConversationEntityMeta)
	}
	for _, v := range s.ToolResults {
		add(RuntimeConversationEntityToolResult, v.RuntimeConversationEntityMeta)
	}
	for _, v := range s.Permissions {
		add(RuntimeConversationEntityPermission, v.RuntimeConversationEntityMeta)
	}
	for _, v := range s.TodoPlans {
		add(RuntimeConversationEntityTodoPlan, v.RuntimeConversationEntityMeta)
	}
	for _, v := range s.AgentTasks {
		add(RuntimeConversationEntityAgentTask, v.RuntimeConversationEntityMeta)
	}
	for _, v := range s.Notices {
		add(RuntimeConversationEntityNotice, v.RuntimeConversationEntityMeta)
	}
	return out
}

func canonicalEntityRefsFromPayload(value any) []map[string]any {
	switch refs := value.(type) {
	case []map[string]any:
		return refs
	case []any:
		out := []map[string]any{}
		for _, item := range refs {
			if ref, ok := item.(map[string]any); ok {
				out = append(out, ref)
			}
		}
		return out
	}
	return nil
}
