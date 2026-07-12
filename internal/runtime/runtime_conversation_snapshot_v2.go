package runtime

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/CIPFZ/agent-builder/internal/runtimeapi"
	"github.com/CIPFZ/agent-builder/internal/tools/scheduler"
)

type canonicalEventRange struct{ first, last int64 }

// SessionConversationSnapshotV2 builds the canonical conversation solely from
// persisted stores. It deliberately does not call the legacy output/activity
// projection, which may include active in-memory state and UI grouping policy.
func (r *runtimeService) SessionConversationSnapshotV2(ctx context.Context, sessionID string, req RuntimeCanonicalConversationSnapshotRequest) (RuntimeCanonicalConversationSnapshot, error) {
	r.conversationV2Mu.Lock()
	semantic, err := r.buildSessionConversationSnapshotV2(ctx, sessionID, RuntimeCanonicalConversationSnapshotRequest{})
	if err != nil {
		r.conversationV2Mu.Unlock()
		return RuntimeCanonicalConversationSnapshot{}, err
	}
	store := newRuntimeConversationEventStoreV2(r.eventStore.db)
	checkpoint, reason, exists, err := store.checkpoint(ctx, semantic.SessionID)
	if err != nil {
		r.conversationV2Mu.Unlock()
		return RuntimeCanonicalConversationSnapshot{}, err
	}
	if !exists {
		if err := store.seedSnapshot(ctx, semantic); err != nil {
			r.conversationV2Mu.Unlock()
			return RuntimeCanonicalConversationSnapshot{}, err
		}
		checkpoint, _ = strconv.ParseInt(semantic.Cursor, 10, 64)
	}
	materialized, err := store.loadSnapshot(ctx, semantic.SessionID, checkpoint)
	if err != nil {
		r.conversationV2Mu.Unlock()
		return RuntimeCanonicalConversationSnapshot{}, err
	}
	var recovery *RuntimeEvent
	if reason != "" || !canonicalSemanticSnapshotsEqual(materialized, semantic) {
		maxSequence, maxErr := r.eventStore.MaxSequence(ctx)
		if maxErr != nil {
			r.conversationV2Mu.Unlock()
			return RuntimeCanonicalConversationSnapshot{}, maxErr
		}
		r.mu.Lock()
		if r.nextEventSequence > maxSequence {
			maxSequence = r.nextEventSequence
		}
		raw := RuntimeEvent{ID: newRuntimeEventID(), Sequence: maxSequence + 1, Type: runtimeapi.EventConversationReconciled, SessionID: semantic.SessionID, CreatedAt: time.Now().UTC().Format(time.RFC3339Nano), Payload: map[string]any{"reason": firstNonEmpty(reason, "semantic_state_reconciled")}}
		raw = r.appendRuntimeEventLocked(raw)
		r.mu.Unlock()
		events, diffErr := canonicalDiffEntityEvents(raw, materialized, semantic)
		if diffErr != nil {
			r.conversationV2Mu.Unlock()
			return RuntimeCanonicalConversationSnapshot{}, diffErr
		}
		if err := store.commitProjectedRaw(ctx, raw, events); err != nil {
			r.conversationV2Mu.Unlock()
			return RuntimeCanonicalConversationSnapshot{}, err
		}
		r.removePendingCanonicalThrough(raw.SessionID, raw.Sequence)
		checkpoint = raw.Sequence
		materialized, err = store.loadSnapshot(ctx, semantic.SessionID, checkpoint)
		if err != nil {
			r.conversationV2Mu.Unlock()
			return RuntimeCanonicalConversationSnapshot{}, err
		}
		recovery = &raw
	}
	applyCanonicalWindow(&materialized, req)
	if err := materialized.Validate(); err != nil {
		r.conversationV2Mu.Unlock()
		return RuntimeCanonicalConversationSnapshot{}, err
	}
	r.conversationV2Mu.Unlock()
	if recovery != nil {
		if r.eventStream != nil {
			r.eventStream.Publish(*recovery)
		}
	}
	return materialized, nil
}

func (r *runtimeService) buildSessionConversationSnapshotV2(ctx context.Context, sessionID string, req RuntimeCanonicalConversationSnapshotRequest) (RuntimeCanonicalConversationSnapshot, error) {
	return r.buildSessionConversationSnapshotV2At(ctx, sessionID, req, 0, nil)
}

func (r *runtimeService) buildSessionConversationSnapshotV2At(ctx context.Context, sessionID string, req RuntimeCanonicalConversationSnapshotRequest, maxRawSequence int64, pending *RuntimeEvent) (RuntimeCanonicalConversationSnapshot, error) {
	if err := r.ensureWorkspaceStarted(ctx, false); err != nil {
		return RuntimeCanonicalConversationSnapshot{}, err
	}
	sessionID = strings.TrimSpace(sessionID)
	if sessionID == "" {
		return RuntimeCanonicalConversationSnapshot{}, errors.New("session id is required")
	}
	if req.Scope != "" && req.Scope != RuntimeConversationScopeFull && req.Scope != RuntimeConversationScopeWindow {
		return RuntimeCanonicalConversationSnapshot{}, fmt.Errorf("invalid canonical conversation scope %q", req.Scope)
	}
	if req.Limit < 0 {
		return RuntimeCanonicalConversationSnapshot{}, errors.New("canonical conversation window limit cannot be negative")
	}
	if req.Before != "" && req.Scope != RuntimeConversationScopeWindow {
		return RuntimeCanonicalConversationSnapshot{}, errors.New("canonical conversation before turn requires window scope")
	}

	turns, err := r.turns.ListBySession(ctx, sessionID)
	if err != nil {
		return RuntimeCanonicalConversationSnapshot{}, err
	}
	messagesResp, err := r.SessionMessages(ctx, sessionID)
	if err != nil {
		return RuntimeCanonicalConversationSnapshot{}, err
	}
	calls := make([]scheduler.ToolCall, 0)
	if r.toolCalls != nil {
		calls, err = r.toolCalls.ListSessionCalls(ctx, sessionID)
		if err != nil {
			return RuntimeCanonicalConversationSnapshot{}, err
		}
	}
	permissions, err := r.permissionStore.ListBySession(ctx, sessionID)
	if err != nil {
		return RuntimeCanonicalConversationSnapshot{}, err
	}
	tasks, err := r.agentTasks.ListBySession(ctx, sessionID)
	if err != nil {
		return RuntimeCanonicalConversationSnapshot{}, err
	}
	eventsResp, err := r.eventStore.ListSession(ctx, sessionID, 0)
	if err != nil {
		return RuntimeCanonicalConversationSnapshot{}, err
	}
	if pending != nil {
		eventsResp.Events = append(eventsResp.Events, *pending)
		if eventsResp.FirstSequence == 0 {
			eventsResp.FirstSequence = pending.Sequence
		}
		eventsResp.LastSequence = pending.Sequence
	}
	if maxRawSequence > 0 {
		filtered := make([]RuntimeEvent, 0, len(eventsResp.Events))
		var first, last int64
		for _, event := range eventsResp.Events {
			if event.Sequence > maxRawSequence {
				break
			}
			if first == 0 {
				first = event.Sequence
			}
			last = event.Sequence
			filtered = append(filtered, event)
		}
		eventsResp.Events = filtered
		eventsResp.FirstSequence = first
		eventsResp.LastSequence = last
	}
	if maxRawSequence > 0 {
		terminalTurns := map[string]bool{}
		for _, event := range eventsResp.Events {
			switch event.Type {
			case "turn.completed", "turn.failed", "turn.cancelled", "turn.interrupted":
				terminalTurns[event.TurnID] = true
			}
		}
		for i := range turns {
			if canonicalTurnTerminal(turns[i].Status) && !terminalTurns[turns[i].ID] {
				turns[i].Status = "running"
				turns[i].FinishedAt = 0
				turns[i].Error = ""
			}
		}
	}

	ranges := canonicalEventRanges(eventsResp.Events)
	turnByID := make(map[string]RuntimeTurn, len(turns))
	messageTurn := map[string]string{}
	for _, turn := range turns {
		turnByID[turn.ID] = turn
		messageTurn[turn.UserMessageID] = turn.ID
		messageTurn[turn.LatestAssistantMessageID] = turn.ID
		messageTurn[turn.LatestMessageID] = turn.ID
	}
	for _, call := range calls {
		if call.MessageID != "" {
			messageTurn[call.MessageID] = call.TurnID
		}
	}
	for _, event := range eventsResp.Events {
		if event.MessageID != "" && event.TurnID != "" && messageTurn[event.MessageID] == "" {
			messageTurn[event.MessageID] = event.TurnID
		}
	}
	for _, msg := range messagesResp.Messages {
		if id := firstNonEmpty(msg.Metadata["turnId"], msg.Metadata["turn_id"]); id != "" {
			messageTurn[msg.ID] = id
		}
	}
	assignMessagesToTurns(messagesResp.Messages, turns, messageTurn)

	snapshot := RuntimeCanonicalConversationSnapshot{SchemaVersion: RuntimeConversationSchemaVersion, SessionID: sessionID, Cursor: strconv.FormatInt(eventsResp.LastSequence, 10), Scope: RuntimeConversationScopeFull, Turns: []RuntimeCanonicalTurn{}, Messages: []RuntimeCanonicalMessage{}, AssistantSteps: []RuntimeCanonicalAssistantStep{}, ToolCalls: []RuntimeCanonicalToolCall{}, ToolResults: []RuntimeCanonicalToolResult{}, Permissions: []RuntimeCanonicalPermission{}, TodoPlans: []RuntimeCanonicalTodoPlan{}, AgentTasks: []RuntimeCanonicalAgentTask{}, Notices: []RuntimeCanonicalNotice{}}

	stepByMessage := map[string]string{}
	for _, msg := range messagesResp.Messages {
		if msg.Role == "assistant" {
			stepByMessage[msg.ID] = "assistant-step:" + msg.ID
		}
	}
	seenStep := map[string]int{}
	for _, turn := range turns {
		finalID := canonicalFinalMessageID(turn, messagesResp.Messages)
		meta := canonicalMeta("turn", turn.ID, sessionID, turn.ID, turn.StartedAt, max64(turn.StartedAt, turn.FinishedAt), ranges)
		snapshot.Turns = append(snapshot.Turns, RuntimeCanonicalTurn{RuntimeConversationEntityMeta: meta, Status: turn.Status, UserMessageID: turn.UserMessageID, FinalMessageID: finalID, StartedAt: turn.StartedAt, FinishedAt: turn.FinishedAt, Error: turn.Error})
	}
	for _, msg := range messagesResp.Messages {
		tid := messageTurn[msg.ID]
		phase := canonicalMessagePhase(msg, turnByID[tid], messagesResp.Messages)
		meta := canonicalMeta("message", msg.ID, sessionID, tid, msg.CreatedAt, msg.UpdatedAt, ranges)
		if phase == RuntimeConversationPhaseFinal {
			meta.Revision = maxDecimal(meta.Revision, strconv.FormatInt(ranges["turn:"+tid].last, 10))
		}
		cm := RuntimeCanonicalMessage{RuntimeConversationEntityMeta: meta, Role: msg.Role, Phase: phase, AssistantStepID: stepByMessage[msg.ID], Status: ternary(msg.Finished, "completed", "streaming"), Content: msg.Content, ClientRequestID: msg.ClientRequestID, Error: msg.Error}
		snapshot.Messages = append(snapshot.Messages, cm)
		if msg.Role == "assistant" {
			seenStep[tid]++
			sm := canonicalDerivedMeta(meta, "assistantStep", stepByMessage[msg.ID])
			snapshot.AssistantSteps = append(snapshot.AssistantSteps, RuntimeCanonicalAssistantStep{RuntimeConversationEntityMeta: sm, MessageID: msg.ID, Index: seenStep[tid] - 1, Status: cm.Status, StartedAt: msg.CreatedAt, FinishedAt: ternaryInt64(msg.Finished, msg.UpdatedAt, 0)})
		}
		for ordinal, part := range msg.Parts {
			if part.Type != "tool_result" || part.ToolCallID == "" {
				continue
			}
			id := fmt.Sprintf("tool-result:%s:%d", msg.ID, ordinal)
			rm := canonicalDerivedMeta(meta, "toolResult", id)
			status := "completed"
			if part.IsError {
				status = "failed"
			}
			snapshot.ToolResults = append(snapshot.ToolResults, RuntimeCanonicalToolResult{RuntimeConversationEntityMeta: rm, ToolCallID: part.ToolCallID, Ordinal: ordinal, Status: status, ContentPreview: boundedPreview(part.Content, 4096), ErrorPreview: ternary(part.IsError, boundedPreview(part.Content, 4096), ""), OutputRefs: nonEmptyStrings(part.StoredPath), DeliveredToModel: part.DeliveredToModel})
		}
	}
	resultsByCall := map[string][]string{}
	resultRevisionByCall := map[string]string{}
	for _, result := range snapshot.ToolResults {
		resultsByCall[result.ToolCallID] = append(resultsByCall[result.ToolCallID], result.ID)
		resultRevisionByCall[result.ToolCallID] = maxDecimal(resultRevisionByCall[result.ToolCallID], result.Revision)
	}
	for _, call := range calls {
		created, updated := call.StartedAt.UnixMilli(), call.FinishedAt.UnixMilli()
		if call.FinishedAt.IsZero() {
			updated = created
		}
		meta := canonicalMeta("toolCall", call.ID, sessionID, call.TurnID, created, updated, ranges)
		meta.Revision = maxDecimal(meta.Revision, resultRevisionByCall[call.ID])
		var exit *int
		if !call.FinishedAt.IsZero() {
			value := call.ExitCode
			exit = &value
		}
		snapshot.ToolCalls = append(snapshot.ToolCalls, RuntimeCanonicalToolCall{RuntimeConversationEntityMeta: meta, MessageID: call.MessageID, AssistantStepID: stepByMessage[call.MessageID], Name: call.Name, Source: string(call.Source), Status: string(call.Status), InputJSON: validJSONObjectOrEmpty(call.InputSummary), Command: call.Command, Risk: call.Risk, ResultIDs: resultsByCall[call.ID], StartedAt: created, FinishedAt: ternaryInt64(!call.FinishedAt.IsZero(), updated, 0), ExitCode: exit, Error: call.Error})
	}
	for _, p := range permissions {
		meta := canonicalMeta("permission", p.ID, sessionID, p.TurnID, p.CreatedAt, max64(p.CreatedAt, p.DecidedAt), ranges)
		snapshot.Permissions = append(snapshot.Permissions, RuntimeCanonicalPermission{RuntimeConversationEntityMeta: meta, ToolCallID: p.ToolCallID, Status: p.Status, Description: p.Description, Action: p.Action, Path: p.Path, Target: p.Target, Risk: p.Risk, PolicyMode: p.PolicyMode, PolicyReason: p.PolicyReason, PolicyRuleID: p.PolicyRuleID, PolicyRuleSource: p.PolicyRuleSource, PolicyScopeKind: p.PolicyScopeKind, PolicyScopeValue: p.PolicyScopeValue, PolicyTargetSummary: p.PolicyTargetSummary, Reason: p.Reason, Decision: p.Decision, RequestedAt: p.CreatedAt, DecidedAt: p.DecidedAt})
	}
	for _, task := range tasks {
		if task.ParentSessionID != sessionID {
			continue
		}
		meta := canonicalMeta("agentTask", task.ID, sessionID, task.ParentTurnID, task.StartedAt, task.UpdatedAt, ranges)
		output, _ := r.AgentTaskOutput(ctx, task.ID)
		detail, _ := r.AgentTask(ctx, task.ID)
		messages, messageCount, messagesTruncated := canonicalAgentTaskMessages(detail.Messages)
		snapshot.AgentTasks = append(snapshot.AgentTasks, RuntimeCanonicalAgentTask{RuntimeConversationEntityMeta: meta, ParentToolCallID: task.ParentToolCallID, ParentTaskID: task.ParentTaskID, ChildSessionID: task.ChildSessionID, TeamID: task.TeamID, TeamRole: task.Role, Title: task.Title, Kind: task.Kind, Name: task.Name, PromptSummary: task.PromptSummary, Model: task.Model, Provider: task.Provider, AllowedTools: append([]string(nil), task.AllowedTools...), CapabilityScope: append([]string(nil), task.CapabilityScope...), CWD: task.CWD, Worktree: task.Worktree, Status: task.Status, Progress: task.Progress, ResultSummary: output.Summary, ArtifactRefs: append([]string(nil), output.ArtifactRefs...), Dependencies: append([]string(nil), task.Dependencies...), ResultRefs: append([]string(nil), output.RelatedMessageRefs...), OutputRefs: append([]string(nil), output.OutputRefs...), StartedAt: task.StartedAt, FinishedAt: task.FinishedAt, Error: task.Error, Messages: messages, MessageCount: messageCount, MessagesTruncated: messagesTruncated})
	}
	snapshot.TodoPlans = canonicalTodoPlans(eventsResp.Events, sessionID, ranges, turnByID)
	snapshot.Notices = canonicalNotices(eventsResp.Events, sessionID, ranges)

	canonicalSortSnapshot(&snapshot)
	if req.Before != "" {
		found := false
		for _, turn := range snapshot.Turns {
			if turn.ID == req.Before {
				found = true
				break
			}
		}
		if !found {
			return RuntimeCanonicalConversationSnapshot{}, fmt.Errorf("canonical conversation before turn %q was not found", req.Before)
		}
	}
	applyCanonicalWindow(&snapshot, req)
	if err := snapshot.Validate(); err != nil {
		return RuntimeCanonicalConversationSnapshot{}, err
	}
	return snapshot, nil
}

func canonicalEventRanges(events []RuntimeEvent) map[string]canonicalEventRange {
	out := map[string]canonicalEventRange{}
	add := func(kind, id string, seq int64) {
		id = strings.TrimSpace(id)
		if id == "" {
			return
		}
		key := kind + ":" + id
		v := out[key]
		if v.first == 0 || seq < v.first {
			v.first = seq
		}
		if seq > v.last {
			v.last = seq
		}
		out[key] = v
	}
	for _, e := range events {
		switch e.Type {
		case "turn.started", "turn.completed", "turn.failed", "turn.cancelled", "turn.interrupted":
			add("turn", e.TurnID, e.Sequence)
		case "message.created", "message.updated", "message.completed":
			add("message", e.MessageID, e.Sequence)
		case "tool.call.started", "tool.call.output", "tool.call.completed", "tool.call.failed", "tool.call.cancelled":
			add("toolCall", e.ToolCallID, e.Sequence)
		case "permission.requested", "permission.decided":
			add("permission", stringFromMap(e.Payload, "permission_id"), e.Sequence)
		case runtimeapi.EventTaskStarted, runtimeapi.EventTaskProgress, runtimeapi.EventTaskCompleted, runtimeapi.EventTaskFailed, runtimeapi.EventTaskCancelled, runtimeapi.EventTaskInterrupted, runtimeapi.EventTaskRoleLoaded, runtimeapi.EventTaskScopeApplied, runtimeapi.EventTaskScopeDenied, runtimeapi.EventTaskMessageCreated, runtimeapi.EventTaskMessageDelivered, runtimeapi.EventTaskMessageProcessed, runtimeapi.EventTaskMessageRejected, runtimeapi.EventTaskResultUpdated, runtimeapi.EventTaskArtifactCreated:
			add("agentTask", firstNonEmpty(stringFromMap(e.Payload, "task_id"), stringFromMap(e.Payload, "agent_task_id")), e.Sequence)
		case "todo.updated":
			add("todoPlan", stringFromMap(e.Payload, "plan_id"), e.Sequence)
		}
		if id, _, ok := canonicalNoticeIdentity(e); ok {
			add("notice", id, e.Sequence)
		}
	}
	return out
}

func canonicalMeta(kind, id, sid, tid string, created, updated int64, ranges map[string]canonicalEventRange) RuntimeConversationEntityMeta {
	v := ranges[kind+":"+id]
	return RuntimeConversationEntityMeta{ID: id, SessionID: sid, TurnID: tid, ActivitySequence: strconv.FormatInt(v.first, 10), Revision: strconv.FormatInt(v.last, 10), CreatedAt: created, UpdatedAt: max64(created, updated)}
}
func canonicalDerivedMeta(parent RuntimeConversationEntityMeta, kind, id string) RuntimeConversationEntityMeta {
	parent.ID = id
	return parent
}
func canonicalFinalMessageID(turn RuntimeTurn, messages []RuntimeMessage) string {
	if !canonicalTurnTerminal(turn.Status) || turn.LatestAssistantMessageID == "" {
		return ""
	}
	for _, m := range messages {
		if m.ID != turn.LatestAssistantMessageID || m.Role != "assistant" || !m.Finished {
			continue
		}
		if phase := firstNonEmpty(m.Metadata["conversation_phase"], m.Metadata["conversationPhase"]); phase != "" && phase != RuntimeConversationPhaseFinal {
			return ""
		}
		if m.FinishReason == "tool_use" {
			return ""
		}
		for _, p := range m.Parts {
			if p.Type == "tool_call" {
				return ""
			}
		}
		return m.ID
	}
	return ""
}

func canonicalMessagePhase(msg RuntimeMessage, turn RuntimeTurn, messages []RuntimeMessage) string {
	if msg.Role != "assistant" {
		return ""
	}
	phase := RuntimeConversationPhaseIntermediate
	if canonicalFinalMessageID(turn, messages) == msg.ID {
		phase = RuntimeConversationPhaseFinal
	}
	if persisted := firstNonEmpty(msg.Metadata["conversation_phase"], msg.Metadata["conversationPhase"]); persisted == RuntimeConversationPhaseReasoning || persisted == RuntimeConversationPhaseIntermediate {
		phase = persisted
	}
	return phase
}

func canonicalTurnTerminal(status string) bool {
	switch status {
	case "completed", "failed", "cancelled", "interrupted":
		return true
	}
	return false
}
func assignMessagesToTurns(messages []RuntimeMessage, turns []RuntimeTurn, owners map[string]string) {
	for _, m := range messages {
		if owners[m.ID] != "" {
			continue
		}
		for i := len(turns) - 1; i >= 0; i-- {
			t := turns[i]
			if m.CreatedAt >= t.StartedAt && (t.FinishedAt == 0 || m.CreatedAt <= t.FinishedAt) {
				owners[m.ID] = t.ID
				break
			}
		}
	}
}
func canonicalTodoPlans(events []RuntimeEvent, sid string, ranges map[string]canonicalEventRange, turns map[string]RuntimeTurn) []RuntimeCanonicalTodoPlan {
	latest := map[string]RuntimeEvent{}
	first := map[string]RuntimeEvent{}
	for _, e := range events {
		if e.Type != "todo.updated" {
			continue
		}
		id := stringFromMap(e.Payload, "plan_id")
		if id != "" {
			if _, ok := first[id]; !ok {
				first[id] = e
			}
			latest[id] = e
		}
	}
	out := []RuntimeCanonicalTodoPlan{}
	for id, e := range latest {
		raw, ok := e.Payload["todos"].([]any)
		if !ok {
			continue
		}
		items := []RuntimeCanonicalTodoItem{}
		valid := true
		for i, v := range raw {
			m, ok := v.(map[string]any)
			if !ok {
				valid = false
				break
			}
			iid := stringFromMap(m, "id")
			if iid == "" {
				valid = false
				break
			}
			items = append(items, RuntimeCanonicalTodoItem{ID: iid, Order: i, Status: stringFromMap(m, "status"), Content: stringFromMap(m, "content"), ActiveForm: firstNonEmpty(stringFromMap(m, "active_form"), stringFromMap(m, "activeForm"))})
		}
		if !valid {
			continue
		}
		created := parseRuntimeEventMillis(first[id].CreatedAt)
		updated := parseRuntimeEventMillis(e.CreatedAt)
		ownerTurnID := first[id].TurnID
		meta := canonicalMeta("todoPlan", id, sid, ownerTurnID, created, updated, ranges)
		meta.Revision = maxDecimal(meta.Revision, strconv.FormatInt(ranges["turn:"+ownerTurnID].last, 10))
		status := "active"
		if len(items) == 0 {
			status = "cleared"
		}
		complete := len(items) > 0
		for _, item := range items {
			if item.Status != "completed" {
				complete = false
				break
			}
		}
		if complete {
			status = "completed"
		} else if status == "active" && canonicalTurnTerminal(turns[ownerTurnID].Status) {
			status = "abandoned"
		}
		out = append(out, RuntimeCanonicalTodoPlan{RuntimeConversationEntityMeta: meta, OwnerTurnID: ownerTurnID, Status: status, Items: items})
	}
	return out
}

func canonicalNotices(events []RuntimeEvent, sid string, ranges map[string]canonicalEventRange) []RuntimeCanonicalNotice {
	latest := map[string]RuntimeEvent{}
	first := map[string]RuntimeEvent{}
	kinds := map[string]string{}
	data := map[string]map[string]any{}
	for _, event := range events {
		id, kind, ok := canonicalNoticeIdentity(event)
		if !ok {
			continue
		}
		if _, exists := first[id]; !exists {
			first[id] = event
		}
		if data[id] == nil {
			data[id] = map[string]any{}
		}
		for key, value := range event.Payload {
			data[id][key] = value
		}
		latest[id], kinds[id] = event, kind
	}
	out := make([]RuntimeCanonicalNotice, 0, len(latest))
	for id, event := range latest {
		payloadJSON, _ := json.Marshal(data[id])
		created, updated := parseRuntimeEventMillis(first[id].CreatedAt), parseRuntimeEventMillis(event.CreatedAt)
		meta := canonicalMeta("notice", id, sid, firstNonEmpty(first[id].TurnID, event.TurnID), created, updated, ranges)
		out = append(out, RuntimeCanonicalNotice{RuntimeConversationEntityMeta: meta, Kind: kinds[id], Status: canonicalNoticeStatus(event.Type), Summary: canonicalNoticeSummary(event), Refs: canonicalNoticeRefs(data[id]), DataJSON: string(payloadJSON)})
	}
	return out
}

const canonicalAgentTaskMessageLimit = 64

func canonicalAgentTaskMessages(source []RuntimeAgentTaskMessage) ([]RuntimeCanonicalAgentTaskMessage, int, bool) {
	ordered := append([]RuntimeAgentTaskMessage(nil), source...)
	sort.SliceStable(ordered, func(i, j int) bool {
		if ordered[i].Sequence != ordered[j].Sequence {
			return ordered[i].Sequence < ordered[j].Sequence
		}
		return ordered[i].ID < ordered[j].ID
	})
	count := len(ordered)
	if count > canonicalAgentTaskMessageLimit {
		ordered = ordered[count-canonicalAgentTaskMessageLimit:]
	}
	out := make([]RuntimeCanonicalAgentTaskMessage, 0, len(ordered))
	for _, message := range ordered {
		out = append(out, RuntimeCanonicalAgentTaskMessage{ID: message.ID, Direction: message.Direction, Kind: message.Kind, Status: message.Status, Sequence: message.Sequence, ContentSummary: message.ContentSummary, RelatedToolCallID: message.RelatedToolCallID, RelatedMessageID: message.RelatedMessageID, ArtifactRefs: append([]string(nil), message.ArtifactRefs...), CreatedAt: message.CreatedAt, DeliveredAt: message.DeliveredAt, ProcessedAt: message.ProcessedAt, Error: message.Error})
	}
	return out, count, count > len(out)
}

func canonicalNoticeIdentity(event RuntimeEvent) (string, string, bool) {
	kind := ""
	sourceID := ""
	switch {
	case strings.HasPrefix(event.Type, "hook.execution."), event.Type == runtimeapi.EventHookContextInjected, event.Type == runtimeapi.EventHookInputRewritten:
		kind, sourceID = "hook", stringFromMap(event.Payload, "execution_id")
	case event.Type != runtimeapi.EventContextUsageUpdated && strings.HasPrefix(event.Type, "context."), strings.HasPrefix(event.Type, "skill.context."):
		// Prompt/context assembly is diagnostic state, not conversation
		// activity. Keep the raw events and prompt assembly records for audit
		// and diagnostics, but do not materialize one timeline notice per
		// source (skills, AGENTS.md probes, missing optional files, etc.).
		return "", "", false
	case strings.HasPrefix(event.Type, "compact.") && event.Type != runtimeapi.EventCompactProgress:
		kind, sourceID = "compact", firstNonEmpty(stringFromMap(event.Payload, "boundary_id"), stringFromMap(event.Payload, "compact_boundary_id"))
	case strings.HasPrefix(event.Type, "recovery."):
		kind, sourceID = "recovery", firstNonEmpty(stringFromMap(event.Payload, "error_id"), stringFromMap(event.Payload, "recovery_id"))
	default:
		return "", "", false
	}
	if sourceID == "" {
		sourceID = event.ID
	}
	return "notice:" + kind + ":" + sourceID, kind, true
}

func canonicalNoticeStatus(eventType string) string {
	switch {
	case strings.HasSuffix(eventType, ".started"), strings.HasSuffix(eventType, ".loading"):
		return "running"
	case strings.HasSuffix(eventType, ".failed"), strings.HasSuffix(eventType, ".blocked"):
		return "failed"
	case strings.HasSuffix(eventType, ".skipped"), strings.HasSuffix(eventType, ".omitted"):
		return "skipped"
	default:
		return "completed"
	}
}

func canonicalNoticeSummary(event RuntimeEvent) string {
	return firstNonEmpty(stringFromMap(event.Payload, "summary"), stringFromMap(event.Payload, "error"), stringFromMap(event.Payload, "reason"), stringFromMap(event.Payload, "source_id"), event.Type)
}

func canonicalNoticeRefs(payload map[string]any) []string {
	return nonEmptyStrings(stringFromMap(payload, "source_id"), stringFromMap(payload, "error_id"), stringFromMap(payload, "boundary_id"), stringFromMap(payload, "execution_id"))
}
func parseRuntimeEventMillis(v string) int64 {
	t, err := time.Parse(time.RFC3339Nano, v)
	if err != nil {
		return 0
	}
	return t.UnixMilli()
}
func canonicalSortSnapshot(s *RuntimeCanonicalConversationSnapshot) {
	sort.SliceStable(s.Turns, func(i, j int) bool {
		return canonicalLess(s.Turns[i].RuntimeConversationEntityMeta, s.Turns[j].RuntimeConversationEntityMeta)
	})
	sort.SliceStable(s.Messages, func(i, j int) bool {
		return canonicalLess(s.Messages[i].RuntimeConversationEntityMeta, s.Messages[j].RuntimeConversationEntityMeta)
	})
	sort.SliceStable(s.ToolCalls, func(i, j int) bool {
		return canonicalLess(s.ToolCalls[i].RuntimeConversationEntityMeta, s.ToolCalls[j].RuntimeConversationEntityMeta)
	})
	sort.SliceStable(s.AssistantSteps, func(i, j int) bool {
		return canonicalLess(s.AssistantSteps[i].RuntimeConversationEntityMeta, s.AssistantSteps[j].RuntimeConversationEntityMeta)
	})
	sort.SliceStable(s.ToolResults, func(i, j int) bool {
		return canonicalLess(s.ToolResults[i].RuntimeConversationEntityMeta, s.ToolResults[j].RuntimeConversationEntityMeta)
	})
	sort.SliceStable(s.Permissions, func(i, j int) bool {
		return canonicalLess(s.Permissions[i].RuntimeConversationEntityMeta, s.Permissions[j].RuntimeConversationEntityMeta)
	})
	sort.SliceStable(s.TodoPlans, func(i, j int) bool {
		return canonicalLess(s.TodoPlans[i].RuntimeConversationEntityMeta, s.TodoPlans[j].RuntimeConversationEntityMeta)
	})
	sort.SliceStable(s.AgentTasks, func(i, j int) bool {
		return canonicalLess(s.AgentTasks[i].RuntimeConversationEntityMeta, s.AgentTasks[j].RuntimeConversationEntityMeta)
	})
	sort.SliceStable(s.Notices, func(i, j int) bool {
		return canonicalLess(s.Notices[i].RuntimeConversationEntityMeta, s.Notices[j].RuntimeConversationEntityMeta)
	})
}
func canonicalLess(a, b RuntimeConversationEntityMeta) bool {
	ai, _ := strconv.ParseInt(a.ActivitySequence, 10, 64)
	bi, _ := strconv.ParseInt(b.ActivitySequence, 10, 64)
	if ai > 0 && bi > 0 && ai != bi {
		return ai < bi
	}
	if a.CreatedAt != b.CreatedAt {
		return a.CreatedAt < b.CreatedAt
	}
	return a.ID < b.ID
}
func applyCanonicalWindow(s *RuntimeCanonicalConversationSnapshot, req RuntimeCanonicalConversationSnapshotRequest) {
	if req.Scope != "window" && req.Limit <= 0 {
		return
	}
	limit := req.Limit
	if limit <= 0 {
		limit = 50
	}
	end := len(s.Turns)
	if req.Before != "" {
		for i, t := range s.Turns {
			if t.ID == req.Before {
				end = i
				break
			}
		}
	}
	start := end - limit
	if start < 0 {
		start = 0
	}
	ids := map[string]bool{}
	selected := append([]RuntimeCanonicalTurn(nil), s.Turns[start:end]...)
	turnIDs := []string{}
	for _, t := range selected {
		ids[t.ID] = true
		turnIDs = append(turnIDs, t.ID)
	}
	s.Scope = RuntimeConversationScopeWindow
	window := &RuntimeConversationWindow{TurnIDs: turnIDs, HasMoreBefore: start > 0}
	if len(selected) > 0 {
		window.BeforeCursor = selected[0].ActivitySequence
		window.AfterCursor = selected[len(selected)-1].Revision
	}
	s.Window = window
	s.Turns = selected
	s.Messages = filterMessages(s.Messages, ids)
	s.AssistantSteps = filterSteps(s.AssistantSteps, ids)
	s.ToolCalls = filterCalls(s.ToolCalls, ids)
	s.ToolResults = filterResults(s.ToolResults, s.ToolCalls)
	s.Permissions = filterPermissions(s.Permissions, ids)
	s.TodoPlans = filterTodos(s.TodoPlans, ids)
	s.AgentTasks = filterTasks(s.AgentTasks, ids)
}
func filterMessages(v []RuntimeCanonicalMessage, ids map[string]bool) []RuntimeCanonicalMessage {
	o := []RuntimeCanonicalMessage{}
	for _, x := range v {
		if ids[x.TurnID] {
			o = append(o, x)
		}
	}
	return o
}
func filterSteps(v []RuntimeCanonicalAssistantStep, ids map[string]bool) []RuntimeCanonicalAssistantStep {
	o := []RuntimeCanonicalAssistantStep{}
	for _, x := range v {
		if ids[x.TurnID] {
			o = append(o, x)
		}
	}
	return o
}
func filterCalls(v []RuntimeCanonicalToolCall, ids map[string]bool) []RuntimeCanonicalToolCall {
	o := []RuntimeCanonicalToolCall{}
	for _, x := range v {
		if ids[x.TurnID] {
			o = append(o, x)
		}
	}
	return o
}
func filterResults(v []RuntimeCanonicalToolResult, c []RuntimeCanonicalToolCall) []RuntimeCanonicalToolResult {
	ids := map[string]bool{}
	for _, x := range c {
		ids[x.ID] = true
	}
	o := []RuntimeCanonicalToolResult{}
	for _, x := range v {
		if ids[x.ToolCallID] {
			o = append(o, x)
		}
	}
	return o
}
func filterPermissions(v []RuntimeCanonicalPermission, ids map[string]bool) []RuntimeCanonicalPermission {
	o := []RuntimeCanonicalPermission{}
	for _, x := range v {
		if ids[x.TurnID] {
			o = append(o, x)
		}
	}
	return o
}
func filterTodos(v []RuntimeCanonicalTodoPlan, ids map[string]bool) []RuntimeCanonicalTodoPlan {
	o := []RuntimeCanonicalTodoPlan{}
	for _, x := range v {
		if ids[x.OwnerTurnID] {
			o = append(o, x)
		}
	}
	return o
}
func filterTasks(v []RuntimeCanonicalAgentTask, ids map[string]bool) []RuntimeCanonicalAgentTask {
	o := []RuntimeCanonicalAgentTask{}
	for _, x := range v {
		if ids[x.TurnID] {
			o = append(o, x)
		}
	}
	return o
}
func boundedPreview(v string, n int) string {
	if len(v) <= n {
		return v
	}
	return v[:n]
}
func validJSONObjectOrEmpty(v string) string {
	var decoded any
	if strings.TrimSpace(v) == "" || json.Unmarshal([]byte(v), &decoded) != nil {
		return ""
	}
	return v
}
func nonEmptyStrings(v ...string) []string {
	o := []string{}
	for _, x := range v {
		if strings.TrimSpace(x) != "" {
			o = append(o, x)
		}
	}
	return o
}
func max64(a, b int64) int64 {
	if b > a {
		return b
	}
	return a
}
func maxDecimal(a, b string) string {
	av, _ := strconv.ParseUint(a, 10, 64)
	bv, _ := strconv.ParseUint(b, 10, 64)
	if bv > av {
		return strconv.FormatUint(bv, 10)
	}
	if a == "" {
		return "0"
	}
	return a
}
func ternary[T any](c bool, a, b T) T {
	if c {
		return a
	}
	return b
}
func ternaryInt64(c bool, a, b int64) int64 { return ternary(c, a, b) }
