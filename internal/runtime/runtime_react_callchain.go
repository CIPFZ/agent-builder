package runtime

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"

	"github.com/charmbracelet/crush/internal/message"
)

const (
	reactNodeUserInput          = "user_input"
	reactNodeAssistantStep      = "assistant_step"
	reactNodeToolCall           = "tool_call"
	reactNodePermissionRequest  = "permission_request"
	reactNodePermissionDecision = "permission_decision"
	reactNodeHookExecution      = "hook_execution"
	reactNodeToolResult         = "tool_result"
	reactNodeAssistantFinal     = "assistant_final"
	reactNodeTurnTerminal       = "turn_terminal"
	reactNodeSyntheticRecovery  = "synthetic_recovery"
)

type runtimeReactCallchainInput struct {
	sessionID   string
	turnID      string
	turns       []RuntimeTurn
	messages    []RuntimeMessage
	toolCalls   []RuntimeToolCall
	permissions []RuntimePermissionRequest
	hooks       []RuntimeHookExecution
}

func (r *runtimeService) ReactCallchain(ctx context.Context, turnID string) (RuntimeReactCallchainResponse, error) {
	if err := r.ensureWorkspaceStarted(ctx, false); err != nil {
		return RuntimeReactCallchainResponse{}, err
	}
	turnID = strings.TrimSpace(turnID)
	if turnID == "" {
		return RuntimeReactCallchainResponse{}, errors.New("turn id is required")
	}
	turn, err := r.runtimeTurnForActivity(ctx, turnID)
	if err != nil {
		return RuntimeReactCallchainResponse{}, err
	}
	policy, err := r.activityPolicy(ctx)
	if err != nil {
		return RuntimeReactCallchainResponse{}, err
	}
	activity, err := r.hydrateActivityForSelection(ctx, turn.SessionID, []RuntimeTurn{turn}, policy, runtimeActivitySelection{
		turnIDs: map[string]struct{}{turn.ID: {}},
	})
	if err != nil {
		return RuntimeReactCallchainResponse{}, err
	}
	hooks := r.runtimeReactHooks(ctx, RuntimeHookExecutionsRequest{TurnID: turn.ID})
	return buildRuntimeReactCallchain(runtimeReactCallchainInput{
		sessionID:   turn.SessionID,
		turnID:      turn.ID,
		turns:       []RuntimeTurn{turn},
		messages:    activity.Messages,
		toolCalls:   activity.ToolCalls,
		permissions: activity.Permissions,
		hooks:       hooks,
	}), nil
}

func (r *runtimeService) SessionReactCallchain(ctx context.Context, sessionID string, limit int) (RuntimeReactCallchainResponse, error) {
	if err := r.ensureWorkspaceStarted(ctx, false); err != nil {
		return RuntimeReactCallchainResponse{}, err
	}
	sessionID = strings.TrimSpace(sessionID)
	if sessionID == "" {
		return RuntimeReactCallchainResponse{}, errors.New("session id is required")
	}
	activity, err := r.hydrateSessionActivity(ctx, sessionID, "", 0)
	if err != nil {
		return RuntimeReactCallchainResponse{}, err
	}
	turns := runtimeReactLimitTurns(activity.Turns, limit)
	hooks := r.runtimeReactHooks(ctx, RuntimeHookExecutionsRequest{SessionID: sessionID})
	return buildRuntimeReactCallchain(runtimeReactCallchainInput{
		sessionID:   sessionID,
		turns:       turns,
		messages:    activity.Messages,
		toolCalls:   activity.ToolCalls,
		permissions: activity.Permissions,
		hooks:       hooks,
	}), nil
}

func (r *runtimeService) runtimeReactHooks(ctx context.Context, req RuntimeHookExecutionsRequest) []RuntimeHookExecution {
	if r.hookExecutions.db == nil {
		return nil
	}
	hooks, err := r.hookExecutions.List(ctx, req)
	if err != nil {
		return nil
	}
	return hooks
}

func buildRuntimeReactCallchain(input runtimeReactCallchainInput) RuntimeReactCallchainResponse {
	turns := append([]RuntimeTurn(nil), input.turns...)
	sort.SliceStable(turns, func(i, j int) bool {
		left := firstPositiveInt64(turns[i].StartedAt, turns[i].FinishedAt)
		right := firstPositiveInt64(turns[j].StartedAt, turns[j].FinishedAt)
		if left != right {
			return left < right
		}
		return turns[i].ID < turns[j].ID
	})
	turnIDs := map[string]struct{}{}
	for _, turn := range turns {
		if turn.ID != "" {
			turnIDs[turn.ID] = struct{}{}
		}
	}
	messages := runtimeReactMessagesForTurns(input.messages, turns, input.turnID)
	toolCalls := runtimeReactToolCallsForTurns(input.toolCalls, turnIDs)
	permissions := runtimeReactPermissionsForTurns(input.permissions, turnIDs)
	hooks := runtimeReactHooksForTurns(input.hooks, turnIDs)

	messageByID := map[string]RuntimeMessage{}
	for _, msg := range messages {
		messageByID[msg.ID] = msg
	}
	toolCallByID := map[string]RuntimeToolCall{}
	for _, call := range toolCalls {
		toolCallByID[call.ID] = call
	}
	toolResultByCallID := map[string]RuntimeMessagePart{}
	for _, msg := range messages {
		for _, part := range msg.Parts {
			if part.Type == "tool_result" && part.ToolCallID != "" {
				toolResultByCallID[part.ToolCallID] = part
			}
		}
	}

	builder := runtimeReactBuilder{
		sessionID:     input.sessionID,
		turnID:        input.turnID,
		toolCallByID:  toolCallByID,
		toolResultIDs: map[string]struct{}{},
	}
	var currentUserID string
	var currentAssistantID string
	assistantToolCalls := map[string]string{}
	var lastAssistant RuntimeMessage
	for _, msg := range messages {
		msgTurnID := runtimeReactMessageTurnID(msg, turns)
		switch msg.Role {
		case string(message.User):
			node := builder.add(RuntimeReactCallNode{
				ID:         "message:" + msg.ID,
				Kind:       reactNodeUserInput,
				SessionID:  msg.SessionID,
				TurnID:     msgTurnID,
				MessageID:  msg.ID,
				Status:     "created",
				Title:      "User input",
				Summary:    preview(msg.Content, runtimePartPreviewLimit),
				StartedAt:  msg.CreatedAt,
				FinishedAt: msg.CreatedAt,
				Evidence:   map[string]string{"messageRole": msg.Role},
			})
			currentUserID = node.ID
			currentAssistantID = ""
		case string(message.Assistant):
			lastAssistant = msg
			toolParts := runtimeReactToolCallParts(msg)
			kind := reactNodeAssistantFinal
			title := "Assistant final"
			if len(toolParts) > 0 {
				kind = reactNodeAssistantStep
				title = "Assistant step"
			}
			node := builder.add(RuntimeReactCallNode{
				ID:           "message:" + msg.ID,
				ParentID:     currentUserID,
				Kind:         kind,
				SessionID:    msg.SessionID,
				TurnID:       msgTurnID,
				MessageID:    msg.ID,
				Status:       runtimeReactAssistantStatus(msg),
				FinishReason: msg.FinishReason,
				Title:        title,
				Summary:      preview(msg.Content, runtimePartPreviewLimit),
				Error:        msg.Error,
				StartedAt:    msg.CreatedAt,
				FinishedAt:   msg.UpdatedAt,
				Evidence:     map[string]string{"messageRole": msg.Role},
			})
			currentAssistantID = node.ID
			if kind == reactNodeAssistantFinal {
				builder.summary.HasFinalAssistant = true
				builder.summary.FinalAssistantMessageID = msg.ID
			}
			if msg.FinishReason != "" {
				builder.summary.LastAssistantFinishReason = msg.FinishReason
			}
			for _, part := range toolParts {
				assistantToolCalls[part.ToolCallID] = msg.ID
				call := toolCallByID[part.ToolCallID]
				status := firstNonEmpty(call.Status, "created")
				startedAt := firstPositiveInt64(call.StartedAt, msg.CreatedAt)
				finishedAt := call.FinishedAt
				if finishedAt == 0 && isFinalToolCallStatus(status) {
					finishedAt = msg.UpdatedAt
				}
				builder.add(RuntimeReactCallNode{
					ID:         "tool_call:" + part.ToolCallID,
					ParentID:   currentAssistantID,
					Kind:       reactNodeToolCall,
					SessionID:  firstNonEmpty(call.SessionID, msg.SessionID),
					TurnID:     firstNonEmpty(call.TurnID, msgTurnID),
					MessageID:  msg.ID,
					ToolCallID: part.ToolCallID,
					Status:     status,
					Title:      firstNonEmpty(call.Display.Title, part.Name, call.Name),
					Summary:    firstNonEmpty(call.InputSummary, part.Input),
					Error:      call.Error,
					StartedAt:  startedAt,
					FinishedAt: finishedAt,
					Evidence: map[string]string{
						"messagePart": "tool_call",
						"storeStatus": call.Status,
					},
				})
			}
		case string(message.Tool):
			for _, part := range msg.Parts {
				if part.Type != "tool_result" || part.ToolCallID == "" {
					continue
				}
				parentID := "tool_call:" + part.ToolCallID
				if _, ok := assistantToolCalls[part.ToolCallID]; !ok {
					builder.missing("tool_result_without_assistant_tool_call:" + part.ToolCallID)
				}
				call := toolCallByID[part.ToolCallID]
				status := "completed"
				if part.IsError {
					status = "failed"
				}
				if call.Status != "" {
					status = call.Status
				}
				builder.add(RuntimeReactCallNode{
					ID:         "tool_result:" + msg.ID + ":" + part.ToolCallID,
					ParentID:   parentID,
					Kind:       reactNodeToolResult,
					SessionID:  msg.SessionID,
					TurnID:     firstNonEmpty(call.TurnID, msgTurnID),
					MessageID:  msg.ID,
					ToolCallID: part.ToolCallID,
					Status:     status,
					Title:      firstNonEmpty(part.Name, call.Name, "Tool result"),
					Summary:    preview(firstNonEmpty(part.Content, part.Data, call.OutputSummary), runtimePartPreviewLimit),
					Error:      firstNonEmpty(call.Error, runtimeReactPartError(part)),
					StartedAt:  msg.CreatedAt,
					FinishedAt: msg.CreatedAt,
					Evidence:   map[string]string{"messagePart": "tool_result"},
				})
				builder.toolResultIDs[part.ToolCallID] = struct{}{}
			}
		}
	}

	for _, call := range toolCalls {
		parentID := "tool_call:" + call.ID
		if _, ok := assistantToolCalls[call.ID]; !ok {
			builder.add(RuntimeReactCallNode{
				ID:         "tool_call_store:" + call.ID,
				Kind:       reactNodeToolCall,
				SessionID:  call.SessionID,
				TurnID:     call.TurnID,
				MessageID:  call.MessageID,
				ToolCallID: call.ID,
				Status:     call.Status,
				Title:      firstNonEmpty(call.Display.Title, call.Name),
				Summary:    call.InputSummary,
				Error:      call.Error,
				StartedAt:  call.StartedAt,
				FinishedAt: call.FinishedAt,
				Evidence:   map[string]string{"storeOnly": "true"},
			})
			parentID = "tool_call_store:" + call.ID
		}
		if _, ok := toolResultByCallID[call.ID]; !ok && isFinalToolCallStatus(call.Status) && call.Status != "denied" && call.Status != "cancelled" {
			builder.missing("assistant_tool_call_without_tool_result:" + call.ID)
		}
		if _, ok := toolResultByCallID[call.ID]; ok && call.Status != "" && !runtimeReactToolResultStatusCompatible(call.Status) {
			builder.missing("tool_call_status_conflicts_with_message_result:" + call.ID)
		}
		_ = parentID
	}

	for _, perm := range permissions {
		kind := reactNodePermissionRequest
		if isFinalPermissionStatus(perm.Status) {
			kind = reactNodePermissionDecision
		}
		tool := toolCallByID[perm.ToolCallID]
		builder.add(RuntimeReactCallNode{
			ID:           "permission:" + perm.ID,
			ParentID:     runtimeReactToolParentID(perm.ToolCallID),
			Kind:         kind,
			SessionID:    perm.SessionID,
			TurnID:       perm.TurnID,
			ToolCallID:   perm.ToolCallID,
			PermissionID: perm.ID,
			Status:       firstNonEmpty(perm.Status, perm.Decision),
			Title:        firstNonEmpty(perm.ToolName, tool.Name, "Permission"),
			Summary:      firstNonEmpty(perm.Reason, perm.PolicyReason, perm.Description),
			StartedAt:    perm.CreatedAt,
			FinishedAt:   perm.DecidedAt,
			Evidence: map[string]string{
				"decision": perm.Decision,
				"action":   perm.Action,
			},
		})
		if perm.Status == permissionStatusPending && runtimeReactTurnTerminal(turns, perm.TurnID) {
			builder.missing("permission_pending_after_terminal_turn:" + perm.ID)
		}
	}
	for _, hook := range hooks {
		builder.add(RuntimeReactCallNode{
			ID:              "hook:" + hook.ID,
			ParentID:        runtimeReactToolParentID(hook.ToolCallID),
			Kind:            reactNodeHookExecution,
			SessionID:       hook.SessionID,
			TurnID:          hook.TurnID,
			ToolCallID:      hook.ToolCallID,
			HookExecutionID: hook.ID,
			Status:          hook.Status,
			Title:           firstNonEmpty(hook.HookName, hook.Event, "Hook"),
			Summary:         firstNonEmpty(hook.Reason, hook.OutputSummary, hook.InputSummary),
			Error:           hook.Error,
			StartedAt:       hook.StartedAt,
			FinishedAt:      hook.CompletedAt,
			Evidence: map[string]string{
				"event": hook.Event,
			},
		})
	}
	for _, turn := range turns {
		builder.add(RuntimeReactCallNode{
			ID:         "turn:" + turn.ID + ":terminal",
			ParentID:   currentUserID,
			Kind:       reactNodeTurnTerminal,
			SessionID:  turn.SessionID,
			TurnID:     turn.ID,
			Status:     turn.Status,
			Title:      "Turn terminal",
			Summary:    runtimeReactTurnSummary(turn),
			Error:      turn.Error,
			StartedAt:  turn.StartedAt,
			FinishedAt: turn.FinishedAt,
			Evidence: map[string]string{
				"provider": turn.Provider,
				"model":    turn.Model,
			},
		})
	}
	if len(turns) > 0 && !builder.summary.HasFinalAssistant && runtimeReactTurnsTerminal(turns) {
		builder.missing("turn_completed_without_final_assistant")
	}
	builder.summary.ToolCallCount = len(toolCalls)
	builder.summary.PermissionCount = len(permissions)
	builder.summary.HookCount = len(hooks)
	if lastAssistant.FinishReason != "" {
		builder.summary.LastAssistantFinishReason = lastAssistant.FinishReason
	}
	builder.summary.StopReason = runtimeReactStopReason(turns, permissions, hooks, builder.summary)
	builder.finish()
	return RuntimeReactCallchainResponse{
		SessionID: input.sessionID,
		TurnID:    input.turnID,
		Nodes:     builder.nodes,
		Summary:   builder.summary,
		Source: RuntimeReactCallSource{
			SessionActivityParity: true,
			UsesMessages:          true,
			UsesToolCalls:         true,
			UsesPermissions:       true,
			UsesHooks:             true,
			EventsAreRefreshOnly:  true,
		},
	}
}

func runtimeReactLimitTurns(turns []RuntimeTurn, limit int) []RuntimeTurn {
	if limit <= 0 || len(turns) <= limit {
		return append([]RuntimeTurn(nil), turns...)
	}
	return append([]RuntimeTurn(nil), turns[len(turns)-limit:]...)
}

func runtimeReactToolParentID(toolCallID string) string {
	toolCallID = strings.TrimSpace(toolCallID)
	if toolCallID == "" {
		return ""
	}
	return "tool_call:" + toolCallID
}

type runtimeReactBuilder struct {
	sessionID     string
	turnID        string
	nodes         []RuntimeReactCallNode
	summary       RuntimeReactCallSummary
	toolCallByID  map[string]RuntimeToolCall
	toolResultIDs map[string]struct{}
	missingSet    map[string]struct{}
}

func (b *runtimeReactBuilder) add(node RuntimeReactCallNode) RuntimeReactCallNode {
	if node.SessionID == "" {
		node.SessionID = b.sessionID
	}
	if node.TurnID == "" && b.turnID != "" {
		node.TurnID = b.turnID
	}
	if node.ID == "" {
		node.ID = fmt.Sprintf("node:%d", len(b.nodes)+1)
	}
	node.Sequence = len(b.nodes) + 1
	b.nodes = append(b.nodes, node)
	return node
}

func (b *runtimeReactBuilder) missing(value string) {
	value = strings.TrimSpace(value)
	if value == "" {
		return
	}
	if b.missingSet == nil {
		b.missingSet = map[string]struct{}{}
	}
	if _, ok := b.missingSet[value]; ok {
		return
	}
	b.missingSet[value] = struct{}{}
	b.summary.MissingEvidence = append(b.summary.MissingEvidence, value)
}

func (b *runtimeReactBuilder) finish() {
	sort.Strings(b.summary.MissingEvidence)
}

func runtimeReactMessagesForTurns(messages []RuntimeMessage, turns []RuntimeTurn, turnID string) []RuntimeMessage {
	if len(turns) == 0 {
		return nil
	}
	userIDs := map[string]struct{}{}
	for _, turn := range turns {
		if turn.UserMessageID != "" {
			userIDs[turn.UserMessageID] = struct{}{}
		}
	}
	if len(userIDs) == 0 {
		return append([]RuntimeMessage(nil), messages...)
	}
	out := make([]RuntimeMessage, 0, len(messages))
	inSelectedTurn := false
	for _, msg := range messages {
		if msg.Role == string(message.User) {
			_, inSelectedTurn = userIDs[msg.ID]
		}
		if inSelectedTurn {
			out = append(out, msg)
		}
	}
	if len(out) == 0 && turnID != "" {
		return append([]RuntimeMessage(nil), messages...)
	}
	return out
}

func runtimeReactToolCallsForTurns(calls []RuntimeToolCall, turnIDs map[string]struct{}) []RuntimeToolCall {
	if len(turnIDs) == 0 {
		return nil
	}
	var out []RuntimeToolCall
	for _, call := range calls {
		if _, ok := turnIDs[call.TurnID]; ok {
			out = append(out, call)
		}
	}
	return out
}

func runtimeReactPermissionsForTurns(perms []RuntimePermissionRequest, turnIDs map[string]struct{}) []RuntimePermissionRequest {
	if len(turnIDs) == 0 {
		return nil
	}
	var out []RuntimePermissionRequest
	for _, perm := range perms {
		if _, ok := turnIDs[perm.TurnID]; ok {
			out = append(out, perm)
		}
	}
	return out
}

func runtimeReactHooksForTurns(hooks []RuntimeHookExecution, turnIDs map[string]struct{}) []RuntimeHookExecution {
	if len(turnIDs) == 0 {
		return nil
	}
	var out []RuntimeHookExecution
	for _, hook := range hooks {
		if _, ok := turnIDs[hook.TurnID]; ok {
			out = append(out, hook)
		}
	}
	return out
}

func runtimeReactMessageTurnID(msg RuntimeMessage, turns []RuntimeTurn) string {
	for _, turn := range turns {
		if turn.UserMessageID == msg.ID || turn.LatestAssistantMessageID == msg.ID || turn.LatestMessageID == msg.ID {
			return turn.ID
		}
	}
	if len(turns) == 1 {
		return turns[0].ID
	}
	return ""
}

func runtimeReactToolCallParts(msg RuntimeMessage) []RuntimeMessagePart {
	var parts []RuntimeMessagePart
	for _, part := range msg.Parts {
		if part.Type == "tool_call" && part.ToolCallID != "" {
			parts = append(parts, part)
		}
	}
	return parts
}

func runtimeReactAssistantStatus(msg RuntimeMessage) string {
	if msg.Error != "" {
		return "failed"
	}
	if msg.Finished {
		return "completed"
	}
	return "running"
}

func runtimeReactPartError(part RuntimeMessagePart) string {
	if part.IsError {
		return firstNonEmpty(part.Content, part.Data)
	}
	return ""
}

func runtimeReactToolResultStatusCompatible(status string) bool {
	switch status {
	case "", "completed", "failed":
		return true
	default:
		return false
	}
}

func runtimeReactTurnTerminal(turns []RuntimeTurn, turnID string) bool {
	for _, turn := range turns {
		if turn.ID == turnID {
			return isFinalRuntimeTurnStatus(turn.Status)
		}
	}
	return false
}

func runtimeReactTurnsTerminal(turns []RuntimeTurn) bool {
	if len(turns) == 0 {
		return false
	}
	for _, turn := range turns {
		if !isFinalRuntimeTurnStatus(turn.Status) {
			return false
		}
	}
	return true
}

func isFinalRuntimeTurnStatus(status string) bool {
	switch status {
	case turnStatusCompleted, turnStatusFailed, turnStatusCancelled, turnStatusInterrupted:
		return true
	default:
		return false
	}
}

func isFinalPermissionStatus(status string) bool {
	switch status {
	case permissionStatusAllowedOnce, permissionStatusAllowedSession, permissionStatusDenied, permissionStatusExpired, permissionStatusCancelled:
		return true
	default:
		return false
	}
}

func runtimeReactTurnSummary(turn RuntimeTurn) string {
	switch {
	case turn.Error != "":
		return turn.Error
	case turn.Status != "":
		return "Turn " + turn.Status
	default:
		return "Turn status unavailable"
	}
}

func runtimeReactStopReason(turns []RuntimeTurn, permissions []RuntimePermissionRequest, hooks []RuntimeHookExecution, summary RuntimeReactCallSummary) string {
	for _, perm := range permissions {
		if perm.Status == permissionStatusDenied || perm.Decision == "deny" {
			return "permission_denied"
		}
	}
	for _, hook := range hooks {
		if hook.Status == hookStatusBlocked || hook.Status == hookStatusDenied {
			return "hook_halted"
		}
	}
	for _, turn := range turns {
		switch turn.Status {
		case turnStatusCancelled:
			return "cancelled"
		case turnStatusInterrupted:
			return "interrupted"
		case turnStatusFailed:
			return "provider_error"
		}
	}
	if summary.HasFinalAssistant {
		switch summary.LastAssistantFinishReason {
		case "max_tokens":
			return "context_limit"
		case "error":
			return "provider_error"
		case "tool_use":
			return "tool_use_followup"
		default:
			return "model_stop"
		}
	}
	if runtimeReactTurnsTerminal(turns) {
		return "completed_without_final_assistant"
	}
	return ""
}
