package runtime

import (
	"context"
	"log/slog"
	"time"

	"github.com/charmbracelet/crush/internal/message"
	"github.com/charmbracelet/crush/internal/permission"
	"github.com/charmbracelet/crush/internal/proto"
	"github.com/charmbracelet/crush/internal/pubsub"
	"github.com/charmbracelet/crush/internal/runtimeapi"
	"github.com/charmbracelet/crush/internal/session"
)

const runtimeEventLimit = 200

func (r *runtimeService) Events(context.Context) (RuntimeEventsResponse, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	events := make([]RuntimeEvent, len(r.events))
	copy(events, r.events)
	return RuntimeEventsResponse{Events: events}, nil
}

func (r *runtimeService) EventsEndpoint(_ context.Context) (RuntimeEventsEndpointResponse, error) {
	if err := r.ensureEventStream(); err != nil {
		return RuntimeEventsEndpointResponse{}, err
	}
	return RuntimeEventsEndpointResponse{URL: r.eventStream.URL()}, nil
}

func (r *runtimeService) SubscribeEvents(_ context.Context) (<-chan RuntimeEvent, func()) {
	events := make(chan RuntimeEvent, 64)
	if r.eventStream == nil {
		r.eventStream = newRuntimeSSEServer()
	}
	r.eventStream.addSubscriber(events)
	return events, func() {
		r.eventStream.removeSubscriber(events)
	}
}

func (r *runtimeService) consumeRuntimeEvents(ctx context.Context, workspaceID string) {
	events, err := r.runtime.SubscribeRawEvents(ctx, workspaceID)
	if err != nil {
		slog.Error("Failed to subscribe to Crush runtime events", "workspace_id", workspaceID, "error", err)
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
			runtimePerm := toRuntimePermissionRequest(perm)
			var runtimeEvent RuntimeEvent
			now := time.Now()
			r.mu.Lock()
			r.permissions[perm.ID] = pendingRuntimePermission{
				Permission: runtimePerm,
				Raw:        perm,
			}
			r.eventStats.permissionEvents++
			r.eventStats.lastEventAt = now.UnixMilli()
			runtimeEvent = newPermissionRuntimeEvent(now, runtimePerm)
			runtimeEvent.TurnID = r.sessionTurns[perm.SessionID]
			runtimeEvent = r.appendRuntimeEventLocked(runtimeEvent)
			r.mu.Unlock()
			r.publishRuntimeEvent(runtimeEvent)

			slog.Info("Desktop permission requested", "workspace_id", workspaceID, "session_id", perm.SessionID, "tool", perm.ToolName, "action", perm.Action, "path", perm.Path)
			r.writeAudit(auditEntry{
				Event:            "permission_requested",
				Timestamp:        time.Now().Format(time.RFC3339Nano),
				WorkspaceID:      workspaceID,
				SessionID:        perm.SessionID,
				PermissionTool:   perm.ToolName,
				PermissionAction: perm.Action,
				PermissionPath:   perm.Path,
				PermissionPolicy: "ask",
			})
		case <-ctx.Done():
			return
		}
	}
}

func (r *runtimeService) recordRuntimeEvent(event pubsub.Event[any]) {
	r.mu.Lock()

	var runtimeEvents []RuntimeEvent
	now := time.Now()
	r.eventStats.lastEventAt = now.UnixMilli()
	switch payload := event.Payload.(type) {
	case pubsub.Event[message.Message]:
		r.eventStats.messageEvents++
		msg := toProtoMessage(payload.Payload)
		turnID := r.sessionTurns[msg.SessionID]
		runtimeEvent := newMessageRuntimeEvent(now, msg)
		runtimeEvent.TurnID = turnID
		runtimeEvents = append(runtimeEvents, r.appendRuntimeEventLocked(runtimeEvent))
		for _, event := range newToolRuntimeEvents(now, msg, turnID, r.toolEvents) {
			runtimeEvents = append(runtimeEvents, r.appendRuntimeEventLocked(event))
		}
		if payload.Payload.Role == message.Assistant {
			r.eventStats.assistantEvents++
		}
	case pubsub.Event[proto.Message]:
		r.eventStats.messageEvents++
		turnID := r.sessionTurns[payload.Payload.SessionID]
		runtimeEvent := newMessageRuntimeEvent(now, payload.Payload)
		runtimeEvent.TurnID = turnID
		runtimeEvents = append(runtimeEvents, r.appendRuntimeEventLocked(runtimeEvent))
		for _, event := range newToolRuntimeEvents(now, payload.Payload, turnID, r.toolEvents) {
			runtimeEvents = append(runtimeEvents, r.appendRuntimeEventLocked(event))
		}
		if payload.Payload.Role == proto.Assistant {
			r.eventStats.assistantEvents++
		}
	case pubsub.Event[proto.Session]:
		r.eventStats.sessionEvents++
		runtimeEvents = append(runtimeEvents, r.appendRuntimeEventLocked(newSessionRuntimeEvent(now, payload.Payload.ID, payload.Payload.Title)))
	case pubsub.Event[session.Session]:
		r.eventStats.sessionEvents++
		runtimeEvents = append(runtimeEvents, r.appendRuntimeEventLocked(newSessionRuntimeEvent(now, payload.Payload.ID, payload.Payload.Title)))
	case pubsub.Event[permission.PermissionRequest]:
		r.eventStats.permissionEvents++
	case pubsub.Event[proto.PermissionRequest]:
		r.eventStats.permissionEvents++
	default:
		r.eventStats.otherEvents++
	}
	r.mu.Unlock()
	for _, runtimeEvent := range runtimeEvents {
		r.publishRuntimeEvent(runtimeEvent)
	}
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
	r.events = append(r.events, event)
	if len(r.events) > runtimeEventLimit {
		r.events = r.events[len(r.events)-runtimeEventLimit:]
	}
	return event
}

func (r *runtimeService) ensureEventStream() error {
	if r.eventStream == nil {
		r.eventStream = newRuntimeSSEServer()
	}
	return r.eventStream.Start()
}

func (r *runtimeService) publishRuntimeEvent(event RuntimeEvent) {
	if event.Type == "" || r.eventStream == nil {
		return
	}
	if err := r.ensureEventStream(); err != nil {
		slog.Error("Failed to start desktop runtime SSE stream", "error", err)
		return
	}
	r.eventStream.Publish(event)
}

func newMessageRuntimeEvent(createdAt time.Time, msg proto.Message) RuntimeEvent {
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

func newToolRuntimeEvents(createdAt time.Time, msg proto.Message, turnID string, states map[string]runtimeToolEventState) []RuntimeEvent {
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
	event.Payload = map[string]any{
		"permission_id": perm.ID,
		"tool_name":     perm.ToolName,
		"action":        perm.Action,
		"description":   perm.Description,
		"path":          perm.Path,
		"summary":       perm.ToolName + ":" + perm.Action,
	}
	return event
}
