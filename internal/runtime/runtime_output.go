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
	projection := buildRuntimeOutputProjectionFromInput(r.runtimeConversationProjectionInput(ctx, sessionID, activity))
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
	projection := buildRuntimeOutputProjectionFromInput(r.runtimeConversationProjectionInput(ctx, sessionID, activity))
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

type runtimeConversationProjectionInput struct {
	Activity RuntimeSessionActivityWindowResponse
	Hooks    []RuntimeHookExecution
	Tasks    []RuntimeAgentTask
	Todos    *RuntimeTodoSummary
	Compact  []RuntimeCompactBoundary
}

func (r *runtimeService) runtimeConversationProjectionInput(ctx context.Context, sessionID string, activity RuntimeSessionActivityWindowResponse) runtimeConversationProjectionInput {
	input := runtimeConversationProjectionInput{Activity: activity}
	if r.hookExecutions.db != nil {
		if hooks, err := r.hookExecutions.List(ctx, RuntimeHookExecutionsRequest{SessionID: sessionID}); err == nil {
			input.Hooks = hooks
		}
	}
	if r.agentTasks.db != nil {
		if tasks, err := r.agentTasks.ListBySession(ctx, sessionID); err == nil {
			input.Tasks = tasks
		}
	}
	if todos, err := r.SessionTodos(ctx, sessionID); err == nil && todos.Summary.Total > 0 {
		summary := todos.Summary
		input.Todos = &summary
	}
	if compact, err := r.SessionCompactBoundaries(ctx, sessionID); err == nil {
		input.Compact = compact.Boundaries
	}
	return input
}

type runtimeOutputProjection struct {
	messages     map[string]RuntimeMessage
	turns        map[string]RuntimeTurn
	steps        map[string]RuntimeAssistantStep
	toolCalls    map[string]RuntimeToolCall
	toolResults  map[string]RuntimeToolResult
	permissions  map[string]RuntimePermissionRequest
	items        map[string]RuntimeConversationItem
	hooks        map[string]RuntimeHookExecution
	tasks        map[string]RuntimeAgentTask
	todos        *RuntimeTodoSummary
	compact      map[string]RuntimeCompactBoundary
	events       []RuntimeEvent
	stepByCallID map[string]string
}

func buildRuntimeOutputProjection(activity RuntimeSessionActivityWindowResponse) runtimeOutputProjection {
	return buildRuntimeOutputProjectionFromInput(runtimeConversationProjectionInput{Activity: activity})
}

func buildRuntimeOutputProjectionFromInput(input runtimeConversationProjectionInput) runtimeOutputProjection {
	activity := input.Activity
	p := runtimeOutputProjection{
		messages:     map[string]RuntimeMessage{},
		turns:        map[string]RuntimeTurn{},
		steps:        map[string]RuntimeAssistantStep{},
		toolCalls:    map[string]RuntimeToolCall{},
		toolResults:  map[string]RuntimeToolResult{},
		permissions:  map[string]RuntimePermissionRequest{},
		items:        map[string]RuntimeConversationItem{},
		hooks:        map[string]RuntimeHookExecution{},
		tasks:        map[string]RuntimeAgentTask{},
		compact:      map[string]RuntimeCompactBoundary{},
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
	for _, hook := range input.Hooks {
		p.hooks[hook.ID] = hook
	}
	for _, task := range input.Tasks {
		p.tasks[task.ID] = task
	}
	for _, boundary := range input.Compact {
		p.compact[boundary.ID] = boundary
	}
	p.todos = input.Todos
	p.events = append([]RuntimeEvent(nil), activity.Events...)

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
			call.Result = runtimeToolResultView(result)
		}
		call = p.applyConversationToolPolicy(call)
		p.toolCalls[callID] = call
	}
	p.items = p.buildConversationItems(messageTurnIDs)
	return p
}

func (p runtimeOutputProjection) snapshot(sessionID, cursor string) RuntimeOutputSnapshot {
	return RuntimeOutputSnapshot{
		SessionID:      sessionID,
		Cursor:         cursor,
		Version:        1,
		Items:          sortedRuntimeConversationItemMap(p.items),
		Messages:       sortedRuntimeOutputMap(p.messages),
		Turns:          sortedRuntimeOutputTurnMap(p.turns),
		AssistantSteps: sortedRuntimeOutputStepMap(p.steps),
		ToolCalls:      sortedRuntimeOutputToolCallMap(p.toolCalls),
		ToolResults:    sortedRuntimeOutputToolResultMap(p.toolResults),
		Permissions:    sortedRuntimeOutputPermissionMap(p.permissions),
		Hooks:          sortedRuntimeOutputHookMap(p.hooks),
		AgentTasks:     sortedRuntimeOutputTaskMap(p.tasks),
		Todos:          p.todos,
		Compact:        sortedRuntimeOutputCompactMap(p.compact),
	}
}

func (p runtimeOutputProjection) eventsFromRuntimeEvents(events []RuntimeEvent) []RuntimeOutputEvent {
	out := make([]RuntimeOutputEvent, 0, len(events))
	for _, event := range events {
		sequenceBase := event.Sequence * 100
		offset := int64(0)
		emittedItems := map[string]struct{}{}
		appendEvent := func(output RuntimeOutputEvent) {
			offset++
			output.ID = event.ID + ":" + strconv.FormatInt(offset, 10)
			output.Sequence = sequenceBase + offset
			output.SessionID = event.SessionID
			output.TurnID = firstNonEmpty(output.TurnID, event.TurnID)
			output.CreatedAt = runtimeOutputEventTime(event)
			out = append(out, output)
		}
		appendItemEvents := func(operation string) {
			for _, item := range p.itemsForRuntimeEvent(event) {
				if _, ok := emittedItems[item.ID]; ok {
					continue
				}
				emittedItems[item.ID] = struct{}{}
				itemCopy := item
				appendEvent(RuntimeOutputEvent{Kind: "conversation_item." + operationName(operation), EntityID: item.ID, Operation: operation, Item: &itemCopy, TurnID: item.TurnID})
			}
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
			if event.Type == runtimeapi.EventMessageCreated {
				appendItemEvents("append")
			} else {
				appendItemEvents("update")
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
			if event.Type == runtimeapi.EventTurnStarted {
				appendItemEvents("append")
			} else {
				appendItemEvents("update")
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
			if event.Type == runtimeapi.EventToolCallStarted {
				appendItemEvents("append")
			} else {
				appendItemEvents("update")
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
			if event.Type == runtimeapi.EventPermissionRequested {
				appendItemEvents("append")
			} else {
				appendItemEvents("update")
			}
		case runtimeapi.EventHookExecutionStarted, runtimeapi.EventHookExecutionCompleted, runtimeapi.EventHookExecutionSkipped, runtimeapi.EventHookExecutionBlocked, runtimeapi.EventHookExecutionFailed, runtimeapi.EventHookContextInjected, runtimeapi.EventHookInputRewritten:
			for _, hook := range p.hooks {
				if hook.ID == runtimeOutputPayloadString(event, "execution_id") || hook.ToolCallID == event.ToolCallID || hook.TurnID == event.TurnID {
					hookCopy := hook
					appendEvent(RuntimeOutputEvent{Kind: "hook.updated", EntityID: hook.ID, Operation: "update", Hook: &hookCopy, TurnID: hook.TurnID})
				}
			}
			appendItemEvents("update")
		case runtimeapi.EventTaskStarted, runtimeapi.EventTaskProgress, runtimeapi.EventTaskCompleted, runtimeapi.EventTaskFailed, runtimeapi.EventTaskCancelled, runtimeapi.EventTaskInterrupted:
			for _, task := range p.tasks {
				if task.ID == runtimeOutputPayloadString(event, "task_id") || task.ParentToolCallID == event.ToolCallID || task.ParentTurnID == event.TurnID {
					taskCopy := task
					appendEvent(RuntimeOutputEvent{Kind: "agent_task.updated", EntityID: task.ID, Operation: "update", AgentTask: &taskCopy, TurnID: task.ParentTurnID})
				}
			}
			appendItemEvents("update")
		case runtimeapi.EventTodoUpdated:
			if p.todos != nil {
				todosCopy := *p.todos
				appendEvent(RuntimeOutputEvent{Kind: "todo.updated", EntityID: "todo-summary-" + event.SessionID, Operation: "update", Todos: &todosCopy, TurnID: todosCopy.TurnID})
			}
			appendItemEvents("update")
		case runtimeapi.EventCompactBoundaryRecorded, runtimeapi.EventCompactMicroCompleted, runtimeapi.EventCompactFullCompleted, runtimeapi.EventCompactFailed, runtimeapi.EventCompactOutputPreserved:
			for _, boundary := range p.compact {
				if boundary.ID == runtimeOutputPayloadString(event, "boundary_id") || boundary.ID == runtimeOutputPayloadString(event, "compact_boundary_id") || boundary.TurnID == event.TurnID {
					boundaryCopy := boundary
					appendEvent(RuntimeOutputEvent{Kind: "compact.updated", EntityID: boundary.ID, Operation: "update", Compact: &boundaryCopy, TurnID: boundary.TurnID})
				}
			}
			appendItemEvents("update")
		case runtimeapi.EventContextSourceInjected, runtimeapi.EventContextReinjected, runtimeapi.EventContextSourceSkipped, runtimeapi.EventContextSourceFailed, runtimeapi.EventRecoveryStatusChanged, runtimeapi.EventRecoveryErrorClassified, runtimeapi.EventRecoveryRetryStarted, runtimeapi.EventRecoveryRetryCompleted, runtimeapi.EventRecoveryRetryFailed, runtimeapi.EventRecoveryCompactRetryStarted, runtimeapi.EventRecoveryCompactRetryCompleted, runtimeapi.EventRecoveryCompactRetryFailed:
			appendItemEvents("update")
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

func (p runtimeOutputProjection) applyConversationToolPolicy(call RuntimeToolCall) RuntimeToolCall {
	kind := firstNonEmpty(call.Kind, call.Display.Kind, runtimeConversationToolKind(call))
	call.Kind = kind
	call.Display.Kind = kind
	if call.Status == "" {
		call.Status = "queued"
	}
	if call.Status == "completed" && (call.IsError || call.Error != "" || (kind == "shell" && call.ExitCode != 0)) {
		call.Status = "failed"
	}
	for _, perm := range p.permissions {
		if perm.ToolCallID != call.ID {
			continue
		}
		switch perm.Status {
		case "pending":
			call.Status = "waiting_permission"
		case "denied", "cancelled", "expired":
			call.Status = "denied"
		}
	}
	if turn, ok := p.turns[call.TurnID]; ok && isRuntimeTurnTerminal(turn.Status) && !isRuntimeToolTerminal(call.Status) {
		call.Status = "interrupted"
		call.Error = firstNonEmpty(call.Error, turn.Error, "tool call did not finish before the turn ended")
	}
	call.Quiet = runtimeToolQuiet(kind, call.Status)
	call.Groupable = runtimeToolGroupable(kind, call.Status)
	call.DefaultExpanded = runtimeToolDefaultExpanded(call.Status)
	call.GroupKey = firstNonEmpty(call.GroupKey, runtimeToolGroupKey(call.TurnID, call.AssistantStepID, kind))
	return call
}

type runtimeConversationItemEntry struct {
	item RuntimeConversationItem
	rank int
}

func (p runtimeOutputProjection) buildConversationItems(messageTurnIDs map[string]string) map[string]RuntimeConversationItem {
	var entries []runtimeConversationItemEntry
	turnStart := map[string]int64{}
	for _, turn := range p.turns {
		turnStart[turn.ID] = firstPositiveInt64(turn.StartedAt, turn.FinishedAt)
	}
	stepRank := map[string]int{}
	for _, step := range p.steps {
		stepRank[step.ID] = 1000 + step.Index*100
	}
	appendEntry := func(item RuntimeConversationItem, rank int) {
		if item.ID == "" {
			return
		}
		item.SessionID = firstNonEmpty(item.SessionID, p.sessionID())
		entries = append(entries, runtimeConversationItemEntry{item: item, rank: rank})
	}
	for _, msg := range sortedRuntimeOutputMessagesFromMap(p.messages) {
		if msg.Hidden {
			continue
		}
		turnID := messageTurnIDs[msg.ID]
		switch msg.Role {
		case "user":
			appendEntry(RuntimeConversationItem{
				ID:        "user-message-" + msg.ID,
				Kind:      "user_message",
				SessionID: msg.SessionID,
				TurnID:    turnID,
				Role:      "user",
				Status:    runtimeMessageItemStatus(msg),
				Content:   msg.Content,
				MessageID: msg.ID,
				CreatedAt: msg.CreatedAt,
				UpdatedAt: firstPositiveInt64(msg.UpdatedAt, msg.CreatedAt),
			}, 0)
		case "assistant":
			stepID := "assistant-step-" + msg.ID
			baseRank := stepRank[stepID]
			if baseRank == 0 {
				baseRank = 1000
			}
			text, thinking, toolCallIDs := runtimeConversationAssistantParts(msg)
			if thinking != "" && runtimeShouldShowAssistantThinking(msg, p.turns[turnID]) {
				appendEntry(RuntimeConversationItem{
					ID:        "assistant-thinking-" + msg.ID,
					Kind:      "assistant_thinking",
					SessionID: msg.SessionID,
					TurnID:    turnID,
					Role:      "assistant",
					Phase:     "intermediate",
					Status:    runtimeThinkingItemStatus(msg, p.turns[turnID]),
					Summary:   thinking,
					Content:   thinking,
					MessageID: msg.ID,
					CreatedAt: msg.CreatedAt,
					UpdatedAt: firstPositiveInt64(msg.UpdatedAt, msg.CreatedAt),
				}, baseRank)
			}
			if text != "" || msg.Error != "" {
				phase := runtimeAssistantMessagePhase(msg, p.turns[turnID], len(toolCallIDs))
				rank := baseRank + 10
				if phase == "final" {
					rank = 900000
				}
				itemID := "assistant-message-" + msg.ID
				appendEntry(RuntimeConversationItem{
					ID:        itemID,
					Kind:      "assistant_message",
					SessionID: msg.SessionID,
					TurnID:    turnID,
					Role:      "assistant",
					Phase:     phase,
					Status:    runtimeMessageItemStatus(msg),
					Content:   text,
					Error:     msg.Error,
					MessageID: msg.ID,
					CreatedAt: msg.CreatedAt,
					UpdatedAt: firstPositiveInt64(msg.UpdatedAt, msg.CreatedAt),
				}, rank)
				for _, callID := range toolCallIDs {
					call := p.toolCalls[callID]
					call.AssistantItemID = itemID
					p.toolCalls[callID] = call
				}
			}
		}
	}
	p.appendConversationToolItems(&entries, stepRank)
	for _, perm := range sortedRuntimeOutputPermissionMap(p.permissions) {
		call := p.toolCalls[perm.ToolCallID]
		rank := stepRank[call.AssistantStepID] + 30
		if rank == 30 {
			rank = 3000
		}
		appendEntry(RuntimeConversationItem{
			ID:           "permission-request-" + perm.ID,
			Kind:         "permission_request",
			SessionID:    perm.SessionID,
			TurnID:       firstNonEmpty(perm.TurnID, call.TurnID),
			ParentID:     call.AssistantItemID,
			Status:       firstNonEmpty(perm.Status, call.Status),
			Title:        firstNonEmpty(perm.ToolName, call.Name),
			Summary:      firstNonEmpty(perm.Description, perm.Reason, perm.PolicyReason, perm.PolicyTargetSummary),
			ToolCallID:   perm.ToolCallID,
			PermissionID: perm.ID,
			CreatedAt:    perm.CreatedAt,
			UpdatedAt:    firstPositiveInt64(perm.DecidedAt, perm.CreatedAt),
		}, rank)
	}
	for _, hook := range sortedRuntimeOutputHookMap(p.hooks) {
		if !runtimeHookHighSignal(hook) {
			continue
		}
		appendEntry(RuntimeConversationItem{
			ID:         "hook-run-" + hook.ID,
			Kind:       "hook_run",
			SessionID:  hook.SessionID,
			TurnID:     hook.TurnID,
			Status:     hook.Status,
			Title:      firstNonEmpty(hook.HookName, hook.Event),
			Summary:    firstNonEmpty(hook.Reason, hook.Error, hook.OutputSummary, hook.ContextSummary),
			Error:      hook.Error,
			ToolCallID: hook.ToolCallID,
			HookRunID:  hook.ID,
			CreatedAt:  hook.StartedAt,
			UpdatedAt:  firstPositiveInt64(hook.CompletedAt, hook.StartedAt),
		}, 500)
	}
	for _, task := range sortedRuntimeOutputTaskMap(p.tasks) {
		call := p.toolCalls[task.ParentToolCallID]
		appendEntry(RuntimeConversationItem{
			ID:          "agent-task-" + task.ID,
			Kind:        "agent_task",
			SessionID:   task.ParentSessionID,
			TurnID:      firstNonEmpty(task.ParentTurnID, call.TurnID),
			ParentID:    call.AssistantItemID,
			Status:      task.Status,
			Title:       task.Title,
			Summary:     firstNonEmpty(task.ResultSummary, task.PromptSummary, task.Error),
			Error:       task.Error,
			ToolCallID:  task.ParentToolCallID,
			AgentTaskID: task.ID,
			CreatedAt:   task.StartedAt,
			UpdatedAt:   firstPositiveInt64(task.UpdatedAt, task.FinishedAt, task.StartedAt),
		}, 5000)
	}
	if p.todos != nil && p.todos.Total > 0 {
		appendEntry(RuntimeConversationItem{
			ID:        "todo-summary-" + p.todos.SessionID,
			Kind:      "todo_summary",
			SessionID: p.todos.SessionID,
			TurnID:    p.todos.TurnID,
			Status:    "updated",
			Title:     "Todo summary",
			Summary:   fmt.Sprintf("%d pending, %d in progress, %d completed", p.todos.Pending, p.todos.InProgress, p.todos.Completed),
			CreatedAt: p.todos.UpdatedAt,
			UpdatedAt: p.todos.UpdatedAt,
		}, 5100)
	}
	for _, event := range sortedRuntimeConversationEvents(p.events) {
		if item := runtimeConversationContextSourceItem(event); item.ID != "" {
			appendEntry(item, 5150)
		}
	}
	for _, boundary := range sortedRuntimeOutputCompactMap(p.compact) {
		kind := "compact_boundary"
		if boundary.Kind == compactKindMicro {
			kind = "microcompact_marker"
		}
		appendEntry(RuntimeConversationItem{
			ID:        kind + "-" + boundary.ID,
			Kind:      kind,
			SessionID: boundary.SessionID,
			TurnID:    boundary.TurnID,
			Status:    boundary.Status,
			Title:     firstNonEmpty(boundary.Trigger, boundary.Kind),
			Summary:   runtimeCompactBoundarySummary(boundary),
			Error:     boundary.Error,
			ContextID: boundary.ID,
			CreatedAt: boundary.CreatedAt,
			UpdatedAt: firstPositiveInt64(boundary.CompletedAt, boundary.CreatedAt),
		}, 5200)
		for _, ref := range boundary.ToolCallRefs {
			if ref.Replacement == "" && !ref.Preserved {
				continue
			}
			appendEntry(RuntimeConversationItem{
				ID:         "tool-result-replacement-" + boundary.ID + "-" + ref.ToolCallID,
				Kind:       "tool_result_replacement",
				SessionID:  boundary.SessionID,
				TurnID:     boundary.TurnID,
				Status:     boundary.Status,
				Title:      firstNonEmpty(ref.Name, "tool result replaced"),
				Summary:    firstNonEmpty(ref.Reason, ref.Replacement),
				ToolCallID: ref.ToolCallID,
				ContextID:  boundary.ID,
				CreatedAt:  boundary.CreatedAt,
				UpdatedAt:  firstPositiveInt64(boundary.CompletedAt, boundary.CreatedAt),
			}, 5210)
		}
	}
	for _, turn := range sortedRuntimeOutputTurnMap(p.turns) {
		if turn.Diagnostics.Warning != "" {
			appendEntry(RuntimeConversationItem{
				ID:        "diagnostic-warning-" + turn.ID,
				Kind:      "diagnostic_warning",
				SessionID: turn.SessionID,
				TurnID:    turn.ID,
				Status:    turn.Status,
				Title:     firstNonEmpty(turn.Diagnostics.WarningReason, "diagnostic warning"),
				Summary:   turn.Diagnostics.Warning,
				CreatedAt: firstPositiveInt64(turn.FinishedAt, turn.StartedAt),
				UpdatedAt: firstPositiveInt64(turn.FinishedAt, turn.StartedAt),
			}, 7000)
		}
		if !isRuntimeTurnTerminal(turn.Status) {
			appendEntry(RuntimeConversationItem{
				ID:        "turn-progress-" + turn.ID,
				Kind:      "turn_progress",
				SessionID: turn.SessionID,
				TurnID:    turn.ID,
				Status:    turn.Status,
				Title:     turn.Status,
				Summary:   turn.PromptPreview,
				CreatedAt: turn.StartedAt,
				UpdatedAt: firstPositiveInt64(turn.FinishedAt, turn.StartedAt),
			}, 8000)
			continue
		}
		if turn.Status != "completed" || runtimeTurnNeedsRecoveryNotice(turn, entries) {
			appendEntry(RuntimeConversationItem{
				ID:        "turn-terminal-" + turn.ID,
				Kind:      "turn_terminal",
				SessionID: turn.SessionID,
				TurnID:    turn.ID,
				Status:    turn.Status,
				Title:     turn.Status,
				Summary:   firstNonEmpty(turn.Error, turn.Diagnostics.StopReasonMessage, turn.InterruptedReason()),
				Error:     turn.Error,
				CreatedAt: firstPositiveInt64(turn.FinishedAt, turn.StartedAt),
				UpdatedAt: firstPositiveInt64(turn.FinishedAt, turn.StartedAt),
			}, 8000)
			if turn.Status == "interrupted" || turn.Interrupted != nil {
				appendEntry(RuntimeConversationItem{
					ID:        "recovery-notice-" + turn.ID,
					Kind:      "recovery_notice",
					SessionID: turn.SessionID,
					TurnID:    turn.ID,
					Status:    turn.Status,
					Title:     "Recovery notice",
					Summary:   firstNonEmpty(turn.InterruptedReason(), turn.Error, "turn ended before all output was complete"),
					Error:     turn.Error,
					CreatedAt: firstPositiveInt64(turn.FinishedAt, turn.StartedAt),
					UpdatedAt: firstPositiveInt64(turn.FinishedAt, turn.StartedAt),
				}, 8010)
			}
		}
	}
	sort.SliceStable(entries, func(i, j int) bool {
		leftTurn := turnStart[entries[i].item.TurnID]
		rightTurn := turnStart[entries[j].item.TurnID]
		if leftTurn != rightTurn {
			return leftTurn < rightTurn
		}
		if entries[i].item.TurnID != entries[j].item.TurnID {
			return entries[i].item.TurnID < entries[j].item.TurnID
		}
		if entries[i].rank != entries[j].rank {
			return entries[i].rank < entries[j].rank
		}
		if entries[i].item.CreatedAt != entries[j].item.CreatedAt {
			return entries[i].item.CreatedAt < entries[j].item.CreatedAt
		}
		return entries[i].item.ID < entries[j].item.ID
	})
	out := map[string]RuntimeConversationItem{}
	for index, entry := range entries {
		item := entry.item
		item.Sequence = int64(index + 1)
		out[item.ID] = item
	}
	return out
}

func (p runtimeOutputProjection) appendConversationToolItems(entries *[]runtimeConversationItemEntry, stepRank map[string]int) {
	calls := sortedRuntimeOutputToolCallMap(p.toolCalls)
	for i := 0; i < len(calls); {
		call := calls[i]
		group := []RuntimeToolCall{call}
		for j := i + 1; j < len(calls); j++ {
			next := calls[j]
			if !call.Groupable || !next.Groupable || call.GroupKey == "" || call.GroupKey != next.GroupKey || call.TurnID != next.TurnID || call.AssistantStepID != next.AssistantStepID {
				break
			}
			group = append(group, next)
		}
		if len(group) > 1 {
			var ids []string
			for _, grouped := range group {
				ids = append(ids, grouped.ID)
			}
			first := group[0]
			last := group[len(group)-1]
			*entries = append(*entries, runtimeConversationItemEntry{rank: stepRank[first.AssistantStepID] + 20, item: RuntimeConversationItem{
				ID:          "tool-group-" + first.GroupKey,
				Kind:        "tool_group",
				SessionID:   first.SessionID,
				TurnID:      first.TurnID,
				ParentID:    first.AssistantItemID,
				Status:      runtimeConversationGroupStatus(group),
				Title:       runtimeConversationToolGroupTitle(group),
				Summary:     runtimeConversationToolGroupTitle(group),
				ToolCallID:  first.ID,
				ToolCallIDs: ids,
				Display: RuntimeConversationDisplay{
					Kind:            first.Kind,
					GroupKey:        first.GroupKey,
					Groupable:       true,
					Quiet:           true,
					DefaultExpanded: false,
					ToolCallIDs:     ids,
				},
				CreatedAt: first.StartedAt,
				UpdatedAt: firstPositiveInt64(last.FinishedAt, last.StartedAt),
			}})
			i += len(group)
			continue
		}
		*entries = append(*entries, runtimeConversationItemEntry{rank: stepRank[call.AssistantStepID] + 20, item: RuntimeConversationItem{
			ID:         "tool-call-" + call.ID,
			Kind:       "tool_call",
			SessionID:  call.SessionID,
			TurnID:     call.TurnID,
			ParentID:   call.AssistantItemID,
			Status:     call.Status,
			Title:      firstNonEmpty(call.Display.Title, call.Name),
			Summary:    firstNonEmpty(call.OutputSummary, call.InputSummary, call.Display.Detail, call.Result.ContentPreview, call.Result.DataPreview),
			Error:      firstNonEmpty(call.Error, call.Display.FailureReason),
			ToolCallID: call.ID,
			Display: RuntimeConversationDisplay{
				Kind:            call.Kind,
				Title:           call.Display.Title,
				Detail:          call.Display.Detail,
				Target:          call.Display.Target,
				PrimaryTarget:   call.Display.PrimaryTarget,
				Targets:         call.Display.Targets,
				GroupKey:        call.GroupKey,
				Groupable:       call.Groupable,
				Quiet:           call.Quiet,
				DefaultExpanded: call.DefaultExpanded,
			},
			CreatedAt: call.StartedAt,
			UpdatedAt: firstPositiveInt64(call.FinishedAt, call.StartedAt),
		}})
		i++
	}
}

func runtimeConversationAssistantParts(msg RuntimeMessage) (string, string, []string) {
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
	text := strings.TrimSpace(strings.Join(textParts, "\n\n"))
	if text == "" {
		text = strings.TrimSpace(msg.Content)
	}
	return text, strings.TrimSpace(strings.Join(thinkingParts, "\n\n")), toolCallIDs
}

func runtimeAssistantMessagePhase(msg RuntimeMessage, turn RuntimeTurn, toolCallCount int) string {
	if msg.FinishReason == "tool_use" || toolCallCount > 0 {
		return "intermediate"
	}
	if turn.LatestAssistantMessageID != "" && turn.LatestAssistantMessageID != msg.ID {
		return "intermediate"
	}
	return "final"
}

func runtimeShouldShowAssistantThinking(msg RuntimeMessage, turn RuntimeTurn) bool {
	if msg.Role != "assistant" {
		return false
	}
	if !msg.Finished {
		return true
	}
	if turn.ID == "" {
		return false
	}
	return !isRuntimeTurnTerminal(turn.Status)
}

func runtimeThinkingItemStatus(msg RuntimeMessage, turn RuntimeTurn) string {
	if msg.Error != "" {
		return "failed"
	}
	if turn.ID != "" && !isRuntimeTurnTerminal(turn.Status) {
		return "running"
	}
	return runtimeMessageItemStatus(msg)
}

func runtimeMessageItemStatus(msg RuntimeMessage) string {
	if msg.Error != "" {
		return "failed"
	}
	if msg.Role == "assistant" && !msg.Finished {
		return "streaming"
	}
	return "completed"
}

func runtimeToolResultView(result RuntimeToolResult) RuntimeToolResultView {
	return RuntimeToolResultView{
		ID:               result.ID,
		MessageID:        result.MessageID,
		Status:           result.Status,
		ContentPreview:   result.ContentPreview,
		DataPreview:      result.DataPreview,
		DeliveredToModel: result.DeliveredToModel,
		ArtifactRefs:     append([]string(nil), result.ArtifactRefs...),
		DiffRefs:         append([]string(nil), result.DiffRefs...),
		CreatedAt:        result.CreatedAt,
	}
}

func runtimeConversationToolKind(call RuntimeToolCall) string {
	name := strings.ToLower(strings.TrimSpace(call.Name))
	switch {
	case call.Source == "shell" || call.Risk == "execute" || call.Command != "" || name == "bash" || name == "shell" || name == "cmd" || name == "powershell" || name == "pwsh" || name == "go" || name == "npm" || name == "node" || name == "python":
		return "shell"
	case name == "glob" || name == "grep" || name == "list" || name == "ls" || name == "dir" || strings.Contains(name, "search") || strings.Contains(name, "find"):
		return "file_search"
	case strings.Contains(name, "edit") || strings.Contains(name, "patch") || strings.Contains(name, "multiedit") || name == "apply_patch":
		return "file_edit"
	case strings.Contains(name, "read") || strings.Contains(name, "view") || strings.Contains(name, "open"):
		return "file_read"
	case strings.Contains(name, "write") || strings.Contains(name, "create"):
		return "file_write"
	default:
		return "generic"
	}
}

func runtimeHookHighSignal(hook RuntimeHookExecution) bool {
	return hook.Status == hookStatusBlocked || hook.Status == hookStatusFailed || hook.InputRewritten || hook.ContextInjected
}

func runtimeConversationGroupStatus(calls []RuntimeToolCall) string {
	for _, call := range calls {
		if call.Status != "completed" {
			return call.Status
		}
	}
	return "completed"
}

func runtimeConversationToolGroupTitle(calls []RuntimeToolCall) string {
	if len(calls) == 0 {
		return "Completed tools"
	}
	kind := calls[0].Kind
	switch kind {
	case "file_read":
		return fmt.Sprintf("Read %d files", len(calls))
	case "file_search":
		return fmt.Sprintf("Searched %d times", len(calls))
	case "shell":
		return fmt.Sprintf("Ran %d commands", len(calls))
	default:
		return fmt.Sprintf("Completed %d tools", len(calls))
	}
}

func runtimeCompactBoundarySummary(boundary RuntimeCompactBoundary) string {
	if boundary.Error != "" {
		return boundary.Error
	}
	if boundary.SummaryRef != "" {
		return boundary.SummaryRef
	}
	if len(boundary.MessageRefs) > 0 || len(boundary.ToolCallRefs) > 0 {
		return fmt.Sprintf("%d messages, %d tool outputs", len(boundary.MessageRefs), len(boundary.ToolCallRefs))
	}
	return boundary.Status
}

func runtimeTurnNeedsRecoveryNotice(turn RuntimeTurn, entries []runtimeConversationItemEntry) bool {
	if turn.Diagnostics.MissingFinalAssistant {
		return true
	}
	if turn.Interrupted != nil {
		return true
	}
	return false
}

func (turn RuntimeTurn) InterruptedReason() string {
	if turn.Interrupted == nil {
		return ""
	}
	return firstNonEmpty(turn.Interrupted.Reason, turn.Interrupted.SummaryText)
}

func isRuntimeTurnTerminal(status string) bool {
	return status == "completed" || status == "failed" || status == "cancelled" || status == "interrupted"
}

func isRuntimeToolTerminal(status string) bool {
	return status == "completed" || status == "failed" || status == "denied" || status == "cancelled" || status == "interrupted"
}

func (p runtimeOutputProjection) sessionID() string {
	for _, msg := range p.messages {
		return msg.SessionID
	}
	for _, turn := range p.turns {
		return turn.SessionID
	}
	return ""
}

func (p runtimeOutputProjection) itemsForRuntimeEvent(event RuntimeEvent) []RuntimeConversationItem {
	var out []RuntimeConversationItem
	for _, item := range p.items {
		if event.MessageID != "" && item.MessageID == event.MessageID {
			out = append(out, item)
			continue
		}
		if event.ToolCallID != "" && (item.ToolCallID == event.ToolCallID || containsRuntimeOutputString(item.ToolCallIDs, event.ToolCallID)) {
			out = append(out, item)
			continue
		}
		permissionID := runtimeOutputPayloadString(event, "permission_id")
		if permissionID != "" && item.PermissionID == permissionID {
			out = append(out, item)
			continue
		}
		hookID := runtimeOutputPayloadString(event, "execution_id")
		if hookID != "" && item.HookRunID == hookID {
			out = append(out, item)
			continue
		}
		taskID := runtimeOutputPayloadString(event, "task_id")
		if taskID != "" && item.AgentTaskID == taskID {
			out = append(out, item)
			continue
		}
		if event.Type == runtimeapi.EventTodoUpdated && item.Kind == "todo_summary" && item.SessionID == event.SessionID {
			out = append(out, item)
			continue
		}
		sourceID := runtimeOutputPayloadString(event, "source_id")
		if sourceID != "" && item.Kind == "context_source" && item.ContextID == sourceID && item.TurnID == event.TurnID {
			out = append(out, item)
			continue
		}
		if event.TurnID != "" && item.TurnID == event.TurnID && (strings.HasPrefix(event.Type, "recovery.") || strings.HasPrefix(event.Type, "context.") || strings.HasPrefix(event.Type, "compact.")) {
			out = append(out, item)
		}
	}
	sort.SliceStable(out, func(i, j int) bool {
		return out[i].Sequence < out[j].Sequence
	})
	return out
}

func runtimeConversationContextSourceItem(event RuntimeEvent) RuntimeConversationItem {
	status := ""
	switch event.Type {
	case runtimeapi.EventContextSourceInjected:
		return RuntimeConversationItem{}
	case runtimeapi.EventContextReinjected:
		status = "reinjected"
	case runtimeapi.EventContextSourceSkipped:
		status = "skipped"
	case runtimeapi.EventContextSourceFailed, runtimeapi.EventContextFailed:
		status = "failed"
	default:
		return RuntimeConversationItem{}
	}
	sourceID := runtimeOutputPayloadString(event, "source_id")
	if sourceID == "" {
		sourceID = runtimeOutputPayloadString(event, "context_id")
	}
	if sourceID == "" {
		return RuntimeConversationItem{}
	}
	title := firstNonEmpty(runtimeOutputPayloadString(event, "name"), runtimeOutputPayloadString(event, "kind"), sourceID)
	summary := firstNonEmpty(
		runtimeOutputPayloadString(event, "content_summary"),
		runtimeOutputPayloadString(event, "reason"),
		runtimeOutputPayloadString(event, "path"),
		runtimeOutputPayloadString(event, "uri"),
	)
	if status == "failed" {
		summary = firstNonEmpty(runtimeOutputPayloadString(event, "error"), summary)
	}
	createdAt := runtimeOutputEventTime(event)
	return RuntimeConversationItem{
		ID:        "context-source-" + event.TurnID + "-" + sourceID + "-" + status,
		Kind:      "context_source",
		SessionID: event.SessionID,
		TurnID:    event.TurnID,
		Status:    status,
		Title:     title,
		Summary:   summary,
		Error:     runtimeOutputPayloadString(event, "error"),
		ContextID: sourceID,
		Display: RuntimeConversationDisplay{
			Kind:          runtimeOutputPayloadString(event, "kind"),
			Target:        firstNonEmpty(runtimeOutputPayloadString(event, "path"), runtimeOutputPayloadString(event, "uri")),
			PrimaryTarget: firstNonEmpty(runtimeOutputPayloadString(event, "path"), runtimeOutputPayloadString(event, "uri")),
		},
		CreatedAt: createdAt,
		UpdatedAt: createdAt,
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

func sortedRuntimeOutputMessagesFromMap(values map[string]RuntimeMessage) []RuntimeMessage {
	out := make([]RuntimeMessage, 0, len(values))
	for _, value := range values {
		out = append(out, value)
	}
	return sortedRuntimeOutputMessages(out)
}

func sortedRuntimeOutputMap(values map[string]RuntimeMessage) []RuntimeMessage {
	out := make([]RuntimeMessage, 0, len(values))
	for _, value := range values {
		out = append(out, value)
	}
	return sortedRuntimeOutputMessages(out)
}

func sortedRuntimeConversationItemMap(values map[string]RuntimeConversationItem) []RuntimeConversationItem {
	out := make([]RuntimeConversationItem, 0, len(values))
	for _, value := range values {
		out = append(out, value)
	}
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].Sequence != out[j].Sequence {
			return out[i].Sequence < out[j].Sequence
		}
		return out[i].ID < out[j].ID
	})
	return out
}

func sortedRuntimeConversationEvents(values []RuntimeEvent) []RuntimeEvent {
	out := append([]RuntimeEvent(nil), values...)
	sort.SliceStable(out, func(i, j int) bool {
		left := runtimeOutputEventTime(out[i])
		right := runtimeOutputEventTime(out[j])
		if left != right {
			return left < right
		}
		if out[i].Sequence != out[j].Sequence {
			return out[i].Sequence < out[j].Sequence
		}
		return out[i].ID < out[j].ID
	})
	return out
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

func sortedRuntimeOutputHookMap(values map[string]RuntimeHookExecution) []RuntimeHookExecution {
	out := make([]RuntimeHookExecution, 0, len(values))
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

func sortedRuntimeOutputTaskMap(values map[string]RuntimeAgentTask) []RuntimeAgentTask {
	out := make([]RuntimeAgentTask, 0, len(values))
	for _, value := range values {
		out = append(out, value)
	}
	sort.SliceStable(out, func(i, j int) bool {
		left := firstPositiveInt64(out[i].StartedAt, out[i].UpdatedAt, out[i].FinishedAt)
		right := firstPositiveInt64(out[j].StartedAt, out[j].UpdatedAt, out[j].FinishedAt)
		if left != right {
			return left < right
		}
		return out[i].ID < out[j].ID
	})
	return out
}

func sortedRuntimeOutputCompactMap(values map[string]RuntimeCompactBoundary) []RuntimeCompactBoundary {
	out := make([]RuntimeCompactBoundary, 0, len(values))
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

func containsRuntimeOutputString(values []string, want string) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}
