package runtime

import (
	"context"
	"log/slog"
	"strings"
	"sync"
	"time"

	"github.com/CIPFZ/agent-builder/internal/apitypes"
	"github.com/CIPFZ/agent-builder/internal/message"
	"github.com/CIPFZ/agent-builder/internal/permission"
	"github.com/CIPFZ/agent-builder/internal/pubsub"
	"github.com/CIPFZ/agent-builder/internal/runtimeapi"
	"github.com/CIPFZ/agent-builder/internal/session"
)

const runtimeEventLimit = 200

func (r *runtimeService) Events(_ context.Context, afterValues ...int64) (RuntimeEventsResponse, error) {
	var after int64
	if len(afterValues) > 0 {
		after = afterValues[0]
	}
	r.mu.Lock()
	defer r.mu.Unlock()

	return r.eventsAfterLocked(after), nil
}

func (r *runtimeService) EventsEndpoint(_ context.Context) (RuntimeEventsEndpointResponse, error) {
	if err := r.httpAPI.Start(); err != nil {
		return RuntimeEventsEndpointResponse{}, err
	}
	return RuntimeEventsEndpointResponse{
		URL:   r.httpAPI.URL() + "/v1/events",
		Token: r.httpAPI.Token(),
	}, nil
}

func (r *runtimeService) SubscribeEvents(_ context.Context, afterValues ...int64) (<-chan RuntimeEvent, func()) {
	var after int64
	if len(afterValues) > 0 {
		after = afterValues[0]
	}
	events := make(chan RuntimeEvent, runtimeEventLimit+16)
	if r.eventStream == nil {
		r.eventStream = newRuntimeSSEServer()
	}
	r.mu.Lock()
	history := r.eventsAfterLocked(after)
	for _, event := range history.Events {
		events <- event
	}
	if history.SnapshotRequired {
		events <- newSnapshotRequiredEvent(after, history.FirstSequence, history.LastSequence)
	}
	r.eventStream.addSubscriber(events)
	r.mu.Unlock()
	return events, func() {
		r.eventStream.removeSubscriber(events)
	}
}

func (r *runtimeService) consumeRuntimeEvents(ctx context.Context, workspaceID string) {
	events, err := r.runtime.SubscribeRawEvents(ctx, workspaceID)
	if err != nil {
		slog.Error("Failed to subscribe to Agent Builder runtime events", "workspace_id", workspaceID, "error", err)
		return
	}
	for {
		select {
		case event, ok := <-events:
			if !ok {
				return
			}
			r.recordRuntimeEvent(event)
		case <-ctx.Done():
			return
		}
	}
}

func (r *runtimeService) consumeDesktopPermissions(ctx context.Context, workspaceID string, permissions permission.Service) {
	events := permissions.Subscribe(ctx)
	for {
		select {
		case event, ok := <-events:
			if !ok {
				return
			}
			perm := event.Payload
			if perm.TurnID == "" {
				r.mu.Lock()
				perm.TurnID = r.sessionTurns[perm.SessionID]
				r.mu.Unlock()
			}
			if perm.Risk == "" {
				perm.Risk = permission.ClassifyRisk(perm.ToolName, perm.Description)
			}
			if perm.PolicyMode == "" && r.workspace != nil {
				if ws, err := r.runtime.GetWorkspace(r.workspace.ID); err == nil {
					perm.PolicyMode = string(ws.Permissions.PolicyMode())
				}
			}
			if perm.PolicyReason == "" {
				perm.PolicyReason = "Runtime policy requires approval for this tool call."
			}
			if perm.PolicyProfile == "" {
				perm.PolicyProfile = permission.NormalizePolicyProfile("")
			}
			if perm.Decision == "" {
				perm.Decision = "ask"
			}
			if perm.Status == "" {
				perm.Status = "pending"
			}
			runtimePerm := toRuntimePermissionRequest(perm)
			var runtimeEvent RuntimeEvent
			now := time.Now()
			r.mu.Lock()
			if r.toolCalls != nil && perm.ToolCallID != "" {
				if _, err := r.toolCalls.GetCall(context.Background(), perm.ToolCallID); err == nil {
					_, _ = r.toolCalls.MarkWaitingPermission(context.Background(), perm.ToolCallID)
				}
			}
			if perm.TurnID != "" {
				turn, err := r.turns.Get(context.Background(), perm.TurnID)
				if err == nil {
					turn.Status = turnStatusWaitingPermission
					_, _ = r.turns.Upsert(context.Background(), turn)
				}
			}
			r.permissions[perm.ID] = pendingRuntimePermission{
				Permission: runtimePerm,
				Raw:        perm,
			}
			if r.permissionStore.db != nil {
				_, _ = r.permissionStore.Upsert(context.Background(), runtimePerm)
			}
			r.eventStats.permissionEvents++
			r.eventStats.lastEventAt = now.UnixMilli()
			runtimeEvent = newPermissionRuntimeEvent(now, runtimePerm)
			runtimeEvent.TurnID = firstNonEmpty(runtimeEvent.TurnID, r.sessionTurns[perm.SessionID])
			runtimeEvent = r.appendRuntimeEventLocked(runtimeEvent)
			r.mu.Unlock()
			r.publishRuntimeEvent(runtimeEvent)

			slog.Info("Desktop permission requested", "workspace_id", workspaceID, "session_id", perm.SessionID, "tool", perm.ToolName, "action", perm.Action, "path", perm.Path)
			r.writeAudit(auditEntry{
				RequestID:        perm.TurnID,
				Event:            "permission_requested",
				Timestamp:        time.Now().Format(time.RFC3339Nano),
				WorkspaceID:      workspaceID,
				SessionID:        perm.SessionID,
				PermissionTool:   perm.ToolName,
				PermissionAction: perm.Action,
				PermissionPath:   perm.Path,
				PermissionPolicy: firstNonEmpty(perm.Decision, "ask"),
				PermissionRisk:   string(perm.Risk),
				PermissionReason: perm.PolicyReason,
				PolicyMode:       perm.PolicyMode,
				PolicyProfile:    perm.PolicyProfile,
				Extra: map[string]any{
					"headless":        perm.Headless,
					"headless_reason": perm.HeadlessReason,
				},
				PolicyRuleID:        perm.RuleID,
				PolicyRuleSource:    perm.RuleSource,
				PolicyScopeKind:     perm.ScopeKind,
				PolicyScopeValue:    perm.ScopeValue,
				PolicyTargetSummary: perm.TargetSummary,
				PermissionID:        perm.ID,
				ToolCallID:          perm.ToolCallID,
			})
		case <-ctx.Done():
			return
		}
	}
}

func (r *runtimeService) consumePermissionPolicyApplications(ctx context.Context, workspaceID string, permissions permission.Service) {
	events := permissions.SubscribePolicyApplications(ctx)
	for {
		select {
		case event, ok := <-events:
			if !ok {
				return
			}
			applied := event.Payload
			if applied.TurnID == "" {
				r.mu.Lock()
				applied.TurnID = r.sessionTurns[applied.SessionID]
				r.mu.Unlock()
			}
			r.storeRuntimeEvent(runtimeapi.Event{
				ID:         newRuntimeEventID(),
				Type:       runtimeapi.EventPermissionPolicyApplied,
				CreatedAt:  time.Now().UTC().Format(time.RFC3339Nano),
				SessionID:  applied.SessionID,
				TurnID:     applied.TurnID,
				ToolCallID: applied.ToolCallID,
				Payload: map[string]any{
					"tool_name":           applied.ToolName,
					"action":              applied.Action,
					"path":                applied.Path,
					"target":              applied.Target,
					"decision":            applied.Decision,
					"risk":                applied.Risk,
					"reason":              applied.Reason,
					"mode":                applied.Mode,
					"profile":             applied.Profile,
					"headless":            applied.Headless,
					"headless_reason":     applied.HeadlessReason,
					"matched_rule_id":     applied.RuleID,
					"matched_rule_source": applied.RuleSource,
					"scope_kind":          applied.RuleScopeKind,
					"scope_value":         applied.RuleScopeValue,
					"target_summary":      applied.TargetSummary,
					"shell_risk":          applied.ShellRisk,
					"shell_reason":        applied.ShellReason,
				},
			})
			if applied.RuleID != "" {
				eventType := runtimeapi.EventPolicyRuleMatched
				if applied.Decision == permission.PolicyDeny {
					eventType = runtimeapi.EventPolicyRuleDenied
				} else if applied.Decision == permission.PolicyAsk {
					eventType = runtimeapi.EventPolicyRuleAsk
				}
				r.storeRuntimeEvent(runtimeapi.Event{
					ID:         newRuntimeEventID(),
					Type:       eventType,
					CreatedAt:  time.Now().UTC().Format(time.RFC3339Nano),
					SessionID:  applied.SessionID,
					TurnID:     applied.TurnID,
					ToolCallID: applied.ToolCallID,
					Payload: map[string]any{
						"rule_id":         applied.RuleID,
						"source":          applied.RuleSource,
						"scope_kind":      applied.RuleScopeKind,
						"scope_value":     applied.RuleScopeValue,
						"decision":        applied.Decision,
						"risk":            applied.Risk,
						"reason":          applied.Reason,
						"mode":            applied.Mode,
						"profile":         applied.Profile,
						"headless":        applied.Headless,
						"headless_reason": applied.HeadlessReason,
					},
				})
			}
			if applied.ShellReason != "" {
				r.storeRuntimeEvent(runtimeapi.Event{
					ID:         newRuntimeEventID(),
					Type:       runtimeapi.EventShellPolicyClassified,
					CreatedAt:  time.Now().UTC().Format(time.RFC3339Nano),
					SessionID:  applied.SessionID,
					TurnID:     applied.TurnID,
					ToolCallID: applied.ToolCallID,
					Payload: map[string]any{
						"risk":            applied.ShellRisk,
						"reason":          applied.ShellReason,
						"target_summary":  applied.TargetSummary,
						"decision":        applied.Decision,
						"mode":            applied.Mode,
						"profile":         applied.Profile,
						"headless":        applied.Headless,
						"headless_reason": applied.HeadlessReason,
					},
				})
			}
			r.writeAudit(auditEntry{
				RequestID:        applied.TurnID,
				Event:            "permission_policy_applied",
				Timestamp:        time.Now().Format(time.RFC3339Nano),
				WorkspaceID:      workspaceID,
				SessionID:        applied.SessionID,
				PermissionTool:   applied.ToolName,
				PermissionAction: applied.Action,
				PermissionPath:   applied.Path,
				PermissionPolicy: string(applied.Decision),
				PermissionRisk:   string(applied.Risk),
				PermissionReason: applied.Reason,
				PolicyMode:       string(applied.Mode),
				PolicyProfile:    applied.Profile,
				Extra: map[string]any{
					"headless":        applied.Headless,
					"headless_reason": applied.HeadlessReason,
				},
				PolicyRuleID:        applied.RuleID,
				PolicyRuleSource:    applied.RuleSource,
				PolicyScopeKind:     applied.RuleScopeKind,
				PolicyScopeValue:    applied.RuleScopeValue,
				PolicyTargetSummary: applied.TargetSummary,
				ShellRisk:           string(applied.ShellRisk),
				ShellReason:         applied.ShellReason,
				ToolCallID:          applied.ToolCallID,
			})
		case <-ctx.Done():
			return
		}
	}
}

func (r *runtimeService) recordRuntimeEvent(event pubsub.Event[any]) {
	r.mu.Lock()

	var runtimeEvents []RuntimeEvent
	var deltaEvents []RuntimeEvent
	now := time.Now()
	r.eventStats.lastEventAt = now.UnixMilli()
	switch payload := event.Payload.(type) {
	case pubsub.Event[message.Message]:
		if payload.Type == pubsub.DeletedEvent {
			// A deleted message (e.g. the empty assistant placeholder removed
			// before a reactive-compact retry) must never be replayed as
			// message.created — newMessageRuntimeEvent cannot tell a deleted
			// row from a fresh one. Persist nothing; just nudge the session
			// output stream so subscribers drop the ghost item on the next
			// flush-loop diff against the DB.
			sessionID := payload.Payload.SessionID
			r.mu.Unlock()
			if sessionID != "" {
				r.publishSessionOutputEventFromRuntime(RuntimeEvent{SessionID: sessionID})
			}
			return
		}
		r.eventStats.messageEvents++
		msg := toAPITypeMessage(payload.Payload)
		turnID := r.sessionTurns[msg.SessionID]
		r.recordUserMessageForTurnLocked(context.Background(), msg, turnID)
		r.recordToolCallsFromMessage(context.Background(), msg, turnID, now)
		deltaEvents = append(deltaEvents, r.deriveOutputTextDeltasLocked(msg, turnID, now)...)
		runtimeEvent := newMessageRuntimeEvent(now, msg)
		runtimeEvent.TurnID = turnID
		if kept := r.recordMessageEventWithCoalesceLocked(runtimeEvent, now); kept != nil {
			runtimeEvents = append(runtimeEvents, *kept)
		}
		for _, event := range newToolRuntimeEvents(now, msg, turnID, r.toolEvents) {
			runtimeEvents = append(runtimeEvents, r.appendRuntimeEventLocked(event))
		}
		if payload.Payload.Role == message.Assistant {
			r.eventStats.assistantEvents++
		}
		// WP5 "事件时机": a completed assistant message with real (non-
		// estimated) usage is a new anchor for computeContextUsage, so
		// schedule a debounced context.usage.updated recompute. message.
		// Message (unlike the apitypes.Message case below) still carries
		// Usage, which is the only reliable "was this estimated" signal —
		// agent.go's OnStepFinish only ever persists Usage for real provider
		// responses (see runtime_context_usage.go's anchor selection).
		if runtimeEvent.Type == runtimeapi.EventMessageCompleted && payload.Payload.Role == message.Assistant && !payload.Payload.Usage.IsZero() {
			r.scheduleContextUsageUpdate(msg.SessionID, turnID)
		}
	case pubsub.Event[apitypes.Message]:
		r.eventStats.messageEvents++
		turnID := r.sessionTurns[payload.Payload.SessionID]
		r.recordUserMessageForTurnLocked(context.Background(), payload.Payload, turnID)
		r.recordToolCallsFromMessage(context.Background(), payload.Payload, turnID, now)
		deltaEvents = append(deltaEvents, r.deriveOutputTextDeltasLocked(payload.Payload, turnID, now)...)
		runtimeEvent := newMessageRuntimeEvent(now, payload.Payload)
		runtimeEvent.TurnID = turnID
		if kept := r.recordMessageEventWithCoalesceLocked(runtimeEvent, now); kept != nil {
			runtimeEvents = append(runtimeEvents, *kept)
		}
		for _, event := range newToolRuntimeEvents(now, payload.Payload, turnID, r.toolEvents) {
			runtimeEvents = append(runtimeEvents, r.appendRuntimeEventLocked(event))
		}
		if payload.Payload.Role == apitypes.Assistant {
			r.eventStats.assistantEvents++
		}
	case pubsub.Event[apitypes.Session]:
		r.eventStats.sessionEvents++
		runtimeEvents = append(runtimeEvents, r.appendRuntimeEventLocked(newSessionRuntimeEvent(now, payload.Payload.ID, payload.Payload.Title)))
	case pubsub.Event[session.Session]:
		r.eventStats.sessionEvents++
		runtimeEvents = append(runtimeEvents, r.appendRuntimeEventLocked(newSessionRuntimeEvent(now, payload.Payload.ID, payload.Payload.Title)))
	case pubsub.Event[permission.PermissionRequest]:
		r.eventStats.permissionEvents++
	case pubsub.Event[apitypes.PermissionRequest]:
		r.eventStats.permissionEvents++
	default:
		r.eventStats.otherEvents++
	}
	r.mu.Unlock()
	// Ephemeral text deltas are pushed immediately with no persistence,
	// no ring-buffer eviction, and no coalescing debounce.
	for _, runtimeEvent := range deltaEvents {
		r.publishRuntimeEvent(runtimeEvent)
	}
	for _, runtimeEvent := range runtimeEvents {
		r.publishRuntimeEvent(runtimeEvent)
	}
}

func (r *runtimeService) recordUserMessageForTurnLocked(ctx context.Context, msg apitypes.Message, turnID string) {
	if turnID == "" || msg.ID == "" || msg.Role != apitypes.User {
		return
	}
	turn, err := r.turns.Get(ctx, turnID)
	if err != nil || turn.UserMessageID != "" {
		return
	}
	turn.UserMessageID = msg.ID
	_, _ = r.turns.Upsert(ctx, turn)
}

func (s runtimeEventStats) snapshot() RuntimeEventStats {
	return RuntimeEventStats{
		LastEventAt:      s.lastEventAt,
		MessageEvents:    s.messageEvents,
		SessionEvents:    s.sessionEvents,
		OtherEvents:      s.otherEvents,
		AssistantEvents:  s.assistantEvents,
		PermissionEvents: s.permissionEvents,
	}
}

func (r *runtimeService) storeRuntimeEvent(event RuntimeEvent) RuntimeEvent {
	r.mu.Lock()
	event = r.appendRuntimeEventLocked(event)
	r.mu.Unlock()
	r.publishRuntimeEvent(event)
	return event
}

func (r *runtimeService) appendRuntimeEventLocked(event RuntimeEvent) RuntimeEvent {
	if event.ID == "" {
		event.ID = newRuntimeEventID()
	}
	if event.CreatedAt == "" {
		event.CreatedAt = time.Now().UTC().Format(time.RFC3339Nano)
	}
	if event.Payload == nil {
		event.Payload = map[string]any{}
	}
	event.Payload = redactRuntimePayload(event.Payload)
	// Ephemeral runtime events (streaming text deltas etc.) never touch the
	// ring buffer or the persisted event store — they are pushed straight
	// to subscribers as advisory increments. They also do not consume
	// sequence numbers so the persisted cursor semantics stay clean.
	if runtimeapi.IsEphemeralEventType(event.Type) {
		return event
	}
	if event.Sequence == 0 {
		r.nextEventSequence++
		event.Sequence = r.nextEventSequence
	} else if event.Sequence > r.nextEventSequence {
		r.nextEventSequence = event.Sequence
	}
	r.events = append(r.events, event)
	if len(r.events) > runtimeEventLimit {
		r.events = r.events[len(r.events)-runtimeEventLimit:]
	}
	if r.eventStore.db != nil {
		if err := r.eventStore.Append(context.Background(), event); err != nil {
			slog.Error("Failed to persist runtime event", "event_id", event.ID, "sequence", event.Sequence, "type", event.Type, "error", err)
		}
	}
	return event
}

func (r *runtimeService) eventsAfterLocked(after int64) RuntimeEventsResponse {
	events := make([]RuntimeEvent, 0, len(r.events))
	var firstSequence, lastSequence int64
	if len(r.events) > 0 {
		firstSequence = r.events[0].Sequence
		lastSequence = r.events[len(r.events)-1].Sequence
	}
	snapshotRequired := after > 0 && firstSequence > 0 && after < firstSequence-1
	for _, event := range r.events {
		if after == 0 || event.Sequence > after {
			events = append(events, event)
		}
	}
	return RuntimeEventsResponse{
		Events:           events,
		SnapshotRequired: snapshotRequired,
		FirstSequence:    firstSequence,
		LastSequence:     lastSequence,
	}
}

func (r *runtimeService) ensureEventStream() error {
	if r.eventStream == nil {
		r.eventStream = newRuntimeSSEServer()
	}
	return r.eventStream.Start()
}

func (r *runtimeService) publishRuntimeEvent(event RuntimeEvent) {
	if event.Type == "" {
		return
	}
	// Ephemeral events don't have a persisted sequence number; they are
	// still valid to broadcast to per-session subscribers.
	if event.Sequence == 0 && !runtimeapi.IsEphemeralEventType(event.Type) {
		event = r.storeRuntimeEvent(event)
		return
	}
	r.publishSessionOutputEventFromRuntime(event)
	if runtimeapi.IsEphemeralEventType(event.Type) {
		return
	}
	if r.eventStream == nil {
		return
	}
	if err := r.ensureEventStream(); err != nil {
		slog.Error("Failed to start desktop runtime SSE stream", "error", err)
		return
	}
	r.eventStream.Publish(event)
}

// runtimeContextUsageUpdateDebounce bounds how often a real-usage assistant
// step retriggers context.usage.updated. A multi-step turn can complete
// several assistant steps in quick succession; debouncing collapses that
// burst into a single recompute (18号 doc §4.1 coalesced update cadence).
const runtimeContextUsageUpdateDebounce = 500 * time.Millisecond

// contextUsageDebounceState holds the per-session pending timers for a given
// runtimeService instance. It lives outside the runtimeService struct
// (keyed by the service pointer in a package-level registry) because this
// work package's file ownership for the fix-round covers runtime_events.go
// only, not runtime_service_types.go where the struct is declared.
type contextUsageDebounceState struct {
	mu     sync.Mutex
	timers map[string]*time.Timer
}

var contextUsageDebounceRegistry sync.Map // map[*runtimeService]*contextUsageDebounceState

// scheduleContextUsageUpdate debounces a context.usage.updated recompute for
// sessionID: a call within the debounce window of a previous one resets the
// timer, so only the last call in a burst actually publishes.
func (r *runtimeService) scheduleContextUsageUpdate(sessionID, turnID string) {
	sessionID = strings.TrimSpace(sessionID)
	if sessionID == "" {
		return
	}
	stateAny, _ := contextUsageDebounceRegistry.LoadOrStore(r, &contextUsageDebounceState{timers: map[string]*time.Timer{}})
	state := stateAny.(*contextUsageDebounceState)
	state.mu.Lock()
	defer state.mu.Unlock()
	if existing, ok := state.timers[sessionID]; ok {
		existing.Stop()
	}
	state.timers[sessionID] = time.AfterFunc(runtimeContextUsageUpdateDebounce, func() {
		r.publishContextUsageUpdated(context.Background(), sessionID, turnID, "", 0, nil)
	})
}

func newSnapshotRequiredEvent(after, firstSequence, lastSequence int64) RuntimeEvent {
	event := runtimeapi.NewEvent(newRuntimeEventID(), runtimeapi.EventSnapshotRequired, time.Now())
	event.Sequence = lastSequence
	event.Payload = map[string]any{
		"after":             after,
		"first_sequence":    firstSequence,
		"last_sequence":     lastSequence,
		"snapshot_required": true,
	}
	return event
}

func newMessageRuntimeEvent(createdAt time.Time, msg apitypes.Message) RuntimeEvent {
	eventType := runtimeapi.EventMessageCreated
	switch {
	case msg.FinishPart() != nil:
		eventType = runtimeapi.EventMessageCompleted
	case msg.UpdatedAt > msg.CreatedAt:
		eventType = runtimeapi.EventMessageUpdated
	}
	event := runtimeapi.NewEvent(newRuntimeEventID(), eventType, createdAt)
	event.SessionID = msg.SessionID
	event.MessageID = msg.ID
	event.Payload = map[string]any{
		"role":    string(msg.Role),
		"summary": preview(msg.Content().Text, 160),
	}
	return event
}

func newUsageRuntimeEvent(createdAt time.Time, turnID, sessionID string, usage, delta RuntimeUsage) RuntimeEvent {
	event := runtimeapi.NewEvent(newRuntimeEventID(), runtimeapi.EventUsageUpdated, createdAt)
	event.SessionID = sessionID
	event.TurnID = turnID
	event.Payload = map[string]any{
		"usage": usage,
		"delta": delta,
	}
	return event
}

func newTurnFinishedRuntimeEvent(createdAt time.Time, turnID, sessionID, status string, duration time.Duration, provider, model string, usageDelta RuntimeUsage, errText string) RuntimeEvent {
	eventType := runtimeapi.EventTurnCompleted
	switch status {
	case "failed":
		eventType = runtimeapi.EventTurnFailed
	case "cancelled":
		eventType = runtimeapi.EventTurnCancelled
	}
	event := runtimeapi.NewEvent(newRuntimeEventID(), eventType, createdAt)
	event.SessionID = sessionID
	event.TurnID = turnID
	event.Payload = map[string]any{
		"status":      status,
		"duration_ms": duration.Milliseconds(),
		"provider":    provider,
		"model":       model,
		"usage_delta": usageDelta,
	}
	if errText != "" {
		event.Payload["error"] = errText
	}
	return event
}

func newToolRuntimeEvents(createdAt time.Time, msg apitypes.Message, turnID string, states map[string]runtimeToolEventState) []RuntimeEvent {
	events := make([]RuntimeEvent, 0)
	for _, call := range msg.ToolCalls() {
		if call.ID == "" {
			continue
		}
		state := states[call.ID]
		if !state.Started {
			event := runtimeapi.NewEvent(newRuntimeEventID(), runtimeapi.EventToolCallStarted, createdAt)
			event.SessionID = msg.SessionID
			event.TurnID = turnID
			event.MessageID = msg.ID
			event.ToolCallID = call.ID
			event.Payload = map[string]any{
				"name":     call.Name,
				"input":    preview(call.Input, runtimePartPreviewLimit),
				"finished": call.Finished,
			}
			events = append(events, event)
			state.Started = true
		}
		if call.Finished && !state.Completed {
			event := runtimeapi.NewEvent(newRuntimeEventID(), runtimeapi.EventToolCallCompleted, createdAt)
			event.SessionID = msg.SessionID
			event.TurnID = turnID
			event.MessageID = msg.ID
			event.ToolCallID = call.ID
			event.Payload = map[string]any{
				"name":  call.Name,
				"input": preview(call.Input, runtimePartPreviewLimit),
			}
			events = append(events, event)
			state.Completed = true
		}
		states[call.ID] = state
	}
	for _, result := range msg.ToolResults() {
		if result.ToolCallID == "" {
			continue
		}
		state := states[result.ToolCallID]
		if !state.Output {
			eventType := runtimeapi.EventToolCallOutput
			if result.IsError {
				eventType = runtimeapi.EventToolCallFailed
			}
			event := runtimeapi.NewEvent(newRuntimeEventID(), eventType, createdAt)
			event.SessionID = msg.SessionID
			event.TurnID = turnID
			event.MessageID = msg.ID
			event.ToolCallID = result.ToolCallID
			event.Payload = map[string]any{
				"name":     result.Name,
				"content":  preview(result.Content, runtimePartPreviewLimit),
				"is_error": result.IsError,
			}
			events = append(events, event)
			state.Output = true
			if result.IsError {
				state.Completed = true
			}
			states[result.ToolCallID] = state
		}
	}
	return events
}

func newSessionRuntimeEvent(createdAt time.Time, sessionID, title string) RuntimeEvent {
	event := runtimeapi.NewEvent(newRuntimeEventID(), runtimeapi.EventSessionUpdated, createdAt)
	event.SessionID = sessionID
	event.Payload = map[string]any{
		"title": title,
	}
	return event
}

func newPermissionRuntimeEvent(createdAt time.Time, perm RuntimePermissionRequest) RuntimeEvent {
	event := runtimeapi.NewEvent(newRuntimeEventID(), runtimeapi.EventPermissionRequested, createdAt)
	event.SessionID = perm.SessionID
	event.ToolCallID = perm.ToolCallID
	event.TurnID = perm.TurnID
	event.Payload = map[string]any{
		"permission_id":       perm.ID,
		"tool_name":           perm.ToolName,
		"action":              perm.Action,
		"description":         preview(perm.Description, 200),
		"path":                perm.Path,
		"risk":                perm.Risk,
		"reason":              firstNonEmpty(perm.Reason, perm.PolicyReason),
		"mode":                perm.PolicyMode,
		"profile":             perm.PolicyProfile,
		"headless":            perm.PolicyHeadless,
		"headless_reason":     perm.PolicyHeadlessReason,
		"matched_rule_id":     perm.PolicyRuleID,
		"matched_rule_source": perm.PolicyRuleSource,
		"scope_kind":          perm.PolicyScopeKind,
		"scope_value":         perm.PolicyScopeValue,
		"target_summary":      perm.PolicyTargetSummary,
		"decision":            firstNonEmpty(perm.Decision, "ask"),
		"status":              firstNonEmpty(perm.Status, "pending"),
		"summary":             perm.ToolName + ":" + perm.Action,
	}
	return event
}
