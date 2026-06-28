package runtime

import (
	"context"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/CIPFZ/agent-builder/internal/runtimeapi"
)

func (r *runtimeService) SessionOutput(ctx context.Context, sessionID string, req RuntimeOutputRequest) (RuntimeOutputSnapshot, error) {
	if err := r.ensureWorkspaceStarted(ctx, false); err != nil {
		return RuntimeOutputSnapshot{}, err
	}
	sessionID = strings.TrimSpace(sessionID)
	if sessionID == "" {
		return RuntimeOutputSnapshot{}, fmt.Errorf("session id is required")
	}
	activity, err := r.hydrateSessionActivity(ctx, sessionID, "", req.Limit)
	if err != nil {
		return RuntimeOutputSnapshot{}, err
	}
	projection := buildRuntimeOutputProjection(activity)
	cursor := r.runtimeOutputCursor(ctx, sessionID, activity.Events)
	return projection.snapshot(sessionID, cursor), nil
}

func (r *runtimeService) SessionOutputEvents(ctx context.Context, sessionID string, after string) (RuntimeOutputEventsResponse, error) {
	if err := r.ensureWorkspaceStarted(ctx, false); err != nil {
		return RuntimeOutputEventsResponse{}, err
	}
	sessionID = strings.TrimSpace(sessionID)
	if sessionID == "" {
		return RuntimeOutputEventsResponse{}, fmt.Errorf("session id is required")
	}
	afterSequence, _ := strconv.ParseInt(strings.TrimSpace(after), 10, 64)
	activity, err := r.hydrateSessionActivity(ctx, sessionID, "", 0)
	if err != nil {
		return RuntimeOutputEventsResponse{}, err
	}
	rawEvents := r.runtimeOutputRawEvents(ctx, sessionID, afterSequence)
	projection := buildRuntimeOutputProjection(activity)
	events := projection.eventsFromRuntimeEvents(rawEvents)
	cursor := r.runtimeOutputCursor(ctx, sessionID, rawEvents)
	if cursor == "0" {
		cursor = projection.snapshot(sessionID, r.runtimeOutputCursor(ctx, sessionID, activity.Events)).Cursor
	}
	return RuntimeOutputEventsResponse{SessionID: sessionID, Cursor: cursor, Events: events}, nil
}

func (r *runtimeService) runtimeOutputRawEvents(ctx context.Context, sessionID string, after int64) []RuntimeEvent {
	if r.eventStore.db != nil {
		if resp, err := r.eventStore.ListSession(ctx, sessionID, after); err == nil {
			return resp.Events
		}
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	events := make([]RuntimeEvent, 0)
	for _, event := range r.events {
		if event.SessionID == sessionID && event.Sequence > after {
			events = append(events, event)
		}
	}
	return events
}

func (r *runtimeService) runtimeOutputCursor(ctx context.Context, sessionID string, events []RuntimeEvent) string {
	var sequence int64
	for _, event := range events {
		if event.Sequence > sequence {
			sequence = event.Sequence
		}
	}
	if sequence == 0 && r.eventStore.db != nil {
		if resp, err := r.eventStore.ListSession(ctx, sessionID, 0); err == nil {
			sequence = resp.LastSequence
		}
	}
	if sequence == 0 {
		r.mu.Lock()
		for _, event := range r.events {
			if event.SessionID == sessionID && event.Sequence > sequence {
				sequence = event.Sequence
			}
		}
		r.mu.Unlock()
	}
	return strconv.FormatInt(sequence, 10)
}

type runtimeOutputProjection struct {
	messages     map[string]RuntimeMessage
	turns        map[string]RuntimeTurn
	steps        map[string]RuntimeAssistantStep
	toolCalls    map[string]RuntimeToolCall
	toolResults  map[string]RuntimeToolResult
	permissions  map[string]RuntimePermissionRequest
	stepByCallID map[string]string
}

func buildRuntimeOutputProjection(activity RuntimeSessionActivityWindowResponse) runtimeOutputProjection {
	p := runtimeOutputProjection{
		messages:     map[string]RuntimeMessage{},
		turns:        map[string]RuntimeTurn{},
		steps:        map[string]RuntimeAssistantStep{},
		toolCalls:    map[string]RuntimeToolCall{},
		toolResults:  map[string]RuntimeToolResult{},
		permissions:  map[string]RuntimePermissionRequest{},
		stepByCallID: map[string]string{},
	}
	for _, msg := range activity.Messages {
		p.messages[msg.ID] = msg
	}
	for _, turn := range activity.Turns {
		p.turns[turn.ID] = turn
	}
	for _, call := range activity.ToolCalls {
		p.toolCalls[call.ID] = call
	}
	for _, perm := range activity.Permissions {
		p.permissions[perm.ID] = perm
	}

	messageTurnIDs := runtimeOutputMessageTurnIDs(activity.Messages, activity.Turns, activity.ToolCalls)
	stepIndexByTurn := map[string]int{}
	for _, msg := range sortedRuntimeOutputMessages(activity.Messages) {
		if msg.Role != "assistant" {
			continue
		}
		turnID := messageTurnIDs[msg.ID]
		if turnID == "" {
			turnID = runtimeOutputNearestTurnID(msg, activity.Turns)
		}
		stepIndexByTurn[turnID]++
		step := runtimeAssistantStepFromMessage(msg, turnID, stepIndexByTurn[turnID]-1)
		p.steps[step.ID] = step
		for _, callID := range step.ToolCallIDs {
			p.stepByCallID[callID] = step.ID
		}
	}

	for _, msg := range sortedRuntimeOutputMessages(activity.Messages) {
		if msg.Role != "tool" {
			continue
		}
		for _, part := range msg.Parts {
			if part.Type != "tool_result" || part.ToolCallID == "" {
				continue
			}
			turnID := messageTurnIDs[msg.ID]
			if call := p.toolCalls[part.ToolCallID]; call.TurnID != "" {
				turnID = call.TurnID
			}
			result := RuntimeToolResult{
				ID:               runtimeToolResultID(msg.ID, part.ToolCallID),
				SessionID:        msg.SessionID,
				TurnID:           turnID,
				MessageID:        msg.ID,
				ToolCallID:       part.ToolCallID,
				ToolName:         part.Name,
				Status:           runtimeToolResultStatus(part),
				ContentPreview:   part.Content,
				DataPreview:      part.Data,
				Metadata:         part.Metadata,
				DeliveredToModel: part.DeliveredToModel,
				CreatedAt:        msg.CreatedAt,
			}
			p.toolResults[result.ID] = result
		}
	}

	resultsByCall := map[string][]RuntimeToolResult{}
	for _, result := range p.toolResults {
		resultsByCall[result.ToolCallID] = append(resultsByCall[result.ToolCallID], result)
	}
	for callID, call := range p.toolCalls {
		if stepID := p.stepByCallID[callID]; stepID != "" {
			call.AssistantStepID = stepID
		}
		results := resultsByCall[callID]
		sort.SliceStable(results, func(i, j int) bool {
			if results[i].CreatedAt != results[j].CreatedAt {
				return results[i].CreatedAt < results[j].CreatedAt
			}
			return results[i].ID < results[j].ID
		})
		for _, result := range results {
			call.ResultIDs = append(call.ResultIDs, result.ID)
			call.LatestResultID = result.ID
		}
		p.toolCalls[callID] = call
	}
	return p
}

func (p runtimeOutputProjection) snapshot(sessionID, cursor string) RuntimeOutputSnapshot {
	return RuntimeOutputSnapshot{
		SessionID:      sessionID,
		Cursor:         cursor,
		Messages:       sortedRuntimeOutputMap(p.messages),
		Turns:          sortedRuntimeOutputTurnMap(p.turns),
		AssistantSteps: sortedRuntimeOutputStepMap(p.steps),
		ToolCalls:      sortedRuntimeOutputToolCallMap(p.toolCalls),
		ToolResults:    sortedRuntimeOutputToolResultMap(p.toolResults),
		Permissions:    sortedRuntimeOutputPermissionMap(p.permissions),
	}
}

func (p runtimeOutputProjection) eventsFromRuntimeEvents(events []RuntimeEvent) []RuntimeOutputEvent {
	out := make([]RuntimeOutputEvent, 0, len(events))
	for _, event := range events {
		sequenceBase := event.Sequence * 100
		offset := int64(0)
		appendEvent := func(output RuntimeOutputEvent) {
			offset++
			output.ID = event.ID + ":" + strconv.FormatInt(offset, 10)
			output.Sequence = sequenceBase + offset
			output.SessionID = event.SessionID
			output.TurnID = firstNonEmpty(output.TurnID, event.TurnID)
			output.CreatedAt = runtimeOutputEventTime(event)
			out = append(out, output)
		}
		switch event.Type {
		case runtimeapi.EventMessageCreated, runtimeapi.EventMessageUpdated, runtimeapi.EventMessageCompleted:
			if msg, ok := p.messages[event.MessageID]; ok {
				operation := "append"
				if event.Type != runtimeapi.EventMessageCreated {
					operation = "update"
				}
				msgCopy := msg
				appendEvent(RuntimeOutputEvent{Kind: "message." + operationName(operation), EntityID: msg.ID, Operation: operation, Message: &msgCopy})
			}
			for _, step := range p.steps {
				if step.MessageID == event.MessageID {
					stepCopy := step
					appendEvent(RuntimeOutputEvent{Kind: "assistant_step.updated", EntityID: step.ID, Operation: "update", AssistantStep: &stepCopy, TurnID: step.TurnID})
				}
			}
			for _, result := range p.toolResults {
				if result.MessageID == event.MessageID {
					resultCopy := result
					appendEvent(RuntimeOutputEvent{Kind: "tool_result.created", EntityID: result.ID, Operation: "append", ToolResult: &resultCopy, TurnID: result.TurnID})
				}
			}
		case runtimeapi.EventTurnStarted, runtimeapi.EventTurnCompleted, runtimeapi.EventTurnFailed, runtimeapi.EventTurnCancelled:
			if turn, ok := p.turns[event.TurnID]; ok {
				operation := "update"
				kind := "turn.updated"
				if event.Type == runtimeapi.EventTurnStarted {
					operation = "append"
					kind = "turn.created"
				}
				turnCopy := turn
				appendEvent(RuntimeOutputEvent{Kind: kind, EntityID: turn.ID, Operation: operation, Turn: &turnCopy})
			}
		case runtimeapi.EventToolCallStarted, runtimeapi.EventToolCallCompleted, runtimeapi.EventToolCallFailed, runtimeapi.EventToolCallOutput:
			if call, ok := p.toolCalls[event.ToolCallID]; ok {
				operation := "update"
				kind := "tool_call.updated"
				if event.Type == runtimeapi.EventToolCallStarted {
					operation = "append"
					kind = "tool_call.created"
				}
				callCopy := call
				appendEvent(RuntimeOutputEvent{Kind: kind, EntityID: call.ID, Operation: operation, ToolCall: &callCopy, TurnID: call.TurnID})
			}
			if event.Type == runtimeapi.EventToolCallOutput || event.Type == runtimeapi.EventToolCallFailed {
				for _, result := range p.toolResults {
					if result.ToolCallID == event.ToolCallID {
						resultCopy := result
						appendEvent(RuntimeOutputEvent{Kind: "tool_result.created", EntityID: result.ID, Operation: "append", ToolResult: &resultCopy, TurnID: result.TurnID})
					}
				}
			}
		case runtimeapi.EventPermissionRequested, runtimeapi.EventPermissionDecided:
			for _, perm := range p.permissions {
				if perm.ID == runtimeOutputPayloadString(event, "permission_id") || (event.ToolCallID != "" && perm.ToolCallID == event.ToolCallID) {
					operation := "update"
					kind := "permission.updated"
					if event.Type == runtimeapi.EventPermissionRequested {
						operation = "append"
						kind = "permission.created"
					}
					permCopy := perm
					appendEvent(RuntimeOutputEvent{Kind: kind, EntityID: perm.ID, Operation: operation, Permission: &permCopy, TurnID: perm.TurnID})
				}
			}
		}
	}
	sort.SliceStable(out, func(i, j int) bool {
		return out[i].Sequence < out[j].Sequence
	})
	return out
}

func runtimeAssistantStepFromMessage(msg RuntimeMessage, turnID string, index int) RuntimeAssistantStep {
	var textParts []string
	var thinkingParts []string
	var toolCallIDs []string
	for _, part := range msg.Parts {
		switch part.Type {
		case "text":
			if strings.TrimSpace(part.Text) != "" {
				textParts = append(textParts, part.Text)
			}
		case "reasoning":
			if strings.TrimSpace(part.Thinking) != "" {
				thinkingParts = append(thinkingParts, part.Thinking)
			}
		case "tool_call":
			if part.ToolCallID != "" {
				toolCallIDs = append(toolCallIDs, part.ToolCallID)
			}
		}
	}
	status := "streaming"
	if msg.Finished {
		status = "completed"
	} else if len(toolCallIDs) > 0 {
		status = "waiting_tool"
	}
	if msg.Error != "" {
		status = "failed"
	}
	finishedAt := int64(0)
	if msg.Finished {
		finishedAt = msg.UpdatedAt
	}
	return RuntimeAssistantStep{
		ID:              "assistant-step-" + msg.ID,
		SessionID:       msg.SessionID,
		TurnID:          turnID,
		MessageID:       msg.ID,
		Index:           index,
		Status:          status,
		Text:            strings.TrimSpace(strings.Join(textParts, "\n\n")),
		ThinkingSummary: strings.TrimSpace(strings.Join(thinkingParts, "\n\n")),
		ToolCallIDs:     toolCallIDs,
		StartedAt:       msg.CreatedAt,
		UpdatedAt:       firstPositiveInt64(msg.UpdatedAt, msg.CreatedAt),
		FinishedAt:      finishedAt,
	}
}

func runtimeOutputMessageTurnIDs(messages []RuntimeMessage, turns []RuntimeTurn, calls []RuntimeToolCall) map[string]string {
	out := map[string]string{}
	turnByUserMessage := map[string]string{}
	for _, turn := range turns {
		if turn.UserMessageID != "" {
			out[turn.UserMessageID] = turn.ID
			turnByUserMessage[turn.UserMessageID] = turn.ID
		}
		if turn.LatestAssistantMessageID != "" {
			out[turn.LatestAssistantMessageID] = turn.ID
		}
		if turn.LatestMessageID != "" {
			out[turn.LatestMessageID] = turn.ID
		}
	}
	for _, call := range calls {
		if call.MessageID != "" && call.TurnID != "" {
			out[call.MessageID] = call.TurnID
		}
	}
	currentTurnID := ""
	for _, msg := range sortedRuntimeOutputMessages(messages) {
		if turnID := turnByUserMessage[msg.ID]; turnID != "" {
			currentTurnID = turnID
		}
		if out[msg.ID] == "" && currentTurnID != "" {
			out[msg.ID] = currentTurnID
		}
	}
	return out
}

func runtimeOutputNearestTurnID(msg RuntimeMessage, turns []RuntimeTurn) string {
	bestID := ""
	var bestStarted int64
	for _, turn := range turns {
		if turn.SessionID != msg.SessionID {
			continue
		}
		started := firstPositiveInt64(turn.StartedAt, turn.FinishedAt)
		if started <= msg.CreatedAt && started >= bestStarted {
			bestID = turn.ID
			bestStarted = started
		}
	}
	return bestID
}

func runtimeToolResultID(messageID, toolCallID string) string {
	return "tool-result-" + messageID + "-" + toolCallID
}

func runtimeToolResultStatus(part RuntimeMessagePart) string {
	if part.IsError {
		return "error"
	}
	return "success"
}

func sortedRuntimeOutputMessages(messages []RuntimeMessage) []RuntimeMessage {
	out := append([]RuntimeMessage(nil), messages...)
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].CreatedAt != out[j].CreatedAt {
			return out[i].CreatedAt < out[j].CreatedAt
		}
		return out[i].ID < out[j].ID
	})
	return out
}

func sortedRuntimeOutputMap(values map[string]RuntimeMessage) []RuntimeMessage {
	out := make([]RuntimeMessage, 0, len(values))
	for _, value := range values {
		out = append(out, value)
	}
	return sortedRuntimeOutputMessages(out)
}

func sortedRuntimeOutputTurnMap(values map[string]RuntimeTurn) []RuntimeTurn {
	out := make([]RuntimeTurn, 0, len(values))
	for _, value := range values {
		out = append(out, value)
	}
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].StartedAt != out[j].StartedAt {
			return out[i].StartedAt < out[j].StartedAt
		}
		return out[i].ID < out[j].ID
	})
	return out
}

func sortedRuntimeOutputStepMap(values map[string]RuntimeAssistantStep) []RuntimeAssistantStep {
	out := make([]RuntimeAssistantStep, 0, len(values))
	for _, value := range values {
		out = append(out, value)
	}
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].TurnID != out[j].TurnID {
			return out[i].TurnID < out[j].TurnID
		}
		if out[i].Index != out[j].Index {
			return out[i].Index < out[j].Index
		}
		return out[i].ID < out[j].ID
	})
	return out
}

func sortedRuntimeOutputToolCallMap(values map[string]RuntimeToolCall) []RuntimeToolCall {
	out := make([]RuntimeToolCall, 0, len(values))
	for _, value := range values {
		out = append(out, value)
	}
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].TurnID != out[j].TurnID {
			return out[i].TurnID < out[j].TurnID
		}
		if out[i].StartedAt != out[j].StartedAt {
			return out[i].StartedAt < out[j].StartedAt
		}
		return out[i].ID < out[j].ID
	})
	return out
}

func sortedRuntimeOutputToolResultMap(values map[string]RuntimeToolResult) []RuntimeToolResult {
	out := make([]RuntimeToolResult, 0, len(values))
	for _, value := range values {
		out = append(out, value)
	}
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].TurnID != out[j].TurnID {
			return out[i].TurnID < out[j].TurnID
		}
		if out[i].CreatedAt != out[j].CreatedAt {
			return out[i].CreatedAt < out[j].CreatedAt
		}
		return out[i].ID < out[j].ID
	})
	return out
}

func sortedRuntimeOutputPermissionMap(values map[string]RuntimePermissionRequest) []RuntimePermissionRequest {
	out := make([]RuntimePermissionRequest, 0, len(values))
	for _, value := range values {
		out = append(out, value)
	}
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].CreatedAt != out[j].CreatedAt {
			return out[i].CreatedAt < out[j].CreatedAt
		}
		return out[i].ID < out[j].ID
	})
	return out
}

func runtimeOutputEventTime(event RuntimeEvent) int64 {
	if parsed, err := time.Parse(time.RFC3339Nano, event.CreatedAt); err == nil {
		return parsed.UnixMilli()
	}
	return time.Now().UnixMilli()
}

func runtimeOutputPayloadString(event RuntimeEvent, key string) string {
	if event.Payload == nil {
		return ""
	}
	if value, ok := event.Payload[key].(string); ok {
		return value
	}
	return ""
}

func operationName(operation string) string {
	if operation == "append" {
		return "created"
	}
	return "updated"
}
