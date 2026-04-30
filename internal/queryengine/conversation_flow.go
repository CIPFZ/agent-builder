package queryengine

import (
	"context"
	"fmt"
	"myclaw/internal/approval"
	"myclaw/internal/memory"
	"myclaw/internal/model"
	"myclaw/internal/session"
	"myclaw/internal/tools"
	"strings"
)

type SnipReplayResult struct {
	Messages []session.Message
	Executed bool
}

type PostCompactCleanupResult struct {
	Messages []session.Message
	Executed bool
}

type SessionStartCompactHook interface {
	ProcessSessionStartCompact(context.Context, session.Session) ([]session.Message, error)
}

type TranscriptPathProvider func(session.Session) string

func (q *QueryEngine) SubmitPrompt(ctx context.Context, sess session.Session, promptText string, sink EventSink) error {
	result, err := q.inputs.Process(ctx, sess, promptText)
	if err != nil {
		return err
	}
	q.recordInputProcessing(result)
	if len(result.Messages) > 0 {
		if err := q.emitImmediateMessages(sess, result.Messages, sink); err != nil {
			return err
		}
	}
	if !result.ShouldQuery {
		if len(result.Messages) > 0 {
			return nil
		}
		if strings.TrimSpace(result.ResultText) == "" {
			return nil
		}
		reply, err := q.sessions.AppendMessage(sess.ID, "assistant", result.ResultText)
		if err != nil {
			return err
		}
		q.appendMutableMessage(sess.ID, reply)
		q.setLastAssistantReply(reply.Content)
		return q.emit(sink, Event{
			Type:    "message.created",
			Session: sess,
			Message: &reply,
		})
	}
	normalized := result.NormalizedInput
	if strings.TrimSpace(normalized) == "" {
		return nil
	}
	q.ensureMutableMessages(sess.ID)
	msg, err := q.sessions.AppendMessage(sess.ID, "user", normalized)
	if err != nil {
		return err
	}
	q.appendMutableMessage(sess.ID, msg)
	return q.submitMessage(ctx, sess, msg, sink, false)
}

func (q *QueryEngine) SubmitMessage(ctx context.Context, sess session.Session, userMessage session.Message, sink EventSink) error {
	return q.submitMessage(ctx, sess, userMessage, sink, true)
}

func (q *QueryEngine) submitMessage(ctx context.Context, sess session.Session, userMessage session.Message, sink EventSink, processInput bool) error {
	runID := fmt.Sprintf("run-%06d", q.nextRunID.Add(1))
	ctx, release := q.beginRun(ctx, runID, sess.ID)
	defer release()
	q.ensureMutableMessages(sess.ID)
	q.ensureUserMessageTracked(sess.ID, userMessage)
	if processInput {
		result, err := q.inputs.Process(ctx, sess, userMessage.Content)
		if err != nil {
			q.emitRunError(sink, Event{Type: "run.error", Session: sess, RunID: runID, Message: &userMessage, Error: err.Error()})
			return err
		}
		q.recordInputProcessing(result)
		if len(result.Messages) > 0 {
			if err := q.emitImmediateMessages(sess, result.Messages, sink); err != nil {
				return err
			}
		}
		if !result.ShouldQuery {
			if len(result.Messages) > 0 || strings.TrimSpace(result.ResultText) == "" {
				return nil
			}
			reply, err := q.sessions.AppendMessage(sess.ID, "assistant", result.ResultText)
			if err != nil {
				return err
			}
			q.appendMutableMessage(sess.ID, reply)
			q.setLastAssistantReply(reply.Content)
			return q.emit(sink, Event{
				Type:    "message.created",
				Session: sess,
				Message: &reply,
			})
		}
		if strings.TrimSpace(result.NormalizedInput) == "" {
			return nil
		}
		userMessage.Content = result.NormalizedInput
	}
	if err := q.maybeInjectDynamicSkillAttachments(sess, userMessage); err != nil {
		return err
	}
	if err := q.maybeInjectSkillListingAttachment(sess); err != nil {
		return err
	}
	if err := q.emit(sink, Event{
		Type:    "agent.lifecycle.start",
		Session: sess,
		RunID:   runID,
		Message: &userMessage,
	}); err != nil {
		return err
	}

	select {
	case <-ctx.Done():
		return ctx.Err()
	default:
	}

	stream, err := q.runModelPass(ctx, sess, userMessage, runID, sink)
	if err != nil {
		q.emitRunError(sink, Event{Type: "run.error", Session: sess, RunID: runID, Error: err.Error()})
		return err
	}
	q.recordModelPass()
	reply, err := q.executeTurnLoop(ctx, sess, userMessage, runID, sink, &toolCall{
		name:              stream.ToolName,
		input:             stream.ToolInput,
		inputObject:       normalizedToolInputObject(stream.ToolInput, stream.ToolInputObject),
		toolUseID:         stream.ToolUseID,
		providerMessageID: stream.ProviderMessageID,
	}, stream)
	if err != nil {
		if !isApprovalRequiredError(err) {
			q.emitRunError(sink, Event{Type: "run.error", Session: sess, RunID: runID, Error: err.Error()})
		}
		return err
	}
	return q.emit(sink, Event{
		Type:    "agent.lifecycle.end",
		Session: sess,
		RunID:   runID,
		Message: &reply,
	})
}

func (q *QueryEngine) ApproveAndContinue(ctx context.Context, approvalID string, sink EventSink) error {
	request, ok := q.approvals.Get(approvalID)
	if !ok {
		return fmt.Errorf("approval %q not found", approvalID)
	}
	ctx, release := q.beginRun(ctx, request.RunID, request.SessionID)
	defer release()
	if request.Status == approval.StatusRejected {
		return fmt.Errorf("approval %q was already rejected", approvalID)
	}
	if request.Status == approval.StatusPending {
		updated, err := q.approvals.UpdateStatus(approvalID, approval.StatusApproved)
		if err != nil {
			return err
		}
		request = updated
	}
	_ = q.sessions.UpdateMetadata(request.SessionID, func(metadata *session.SessionMetadata) {
		if metadata.PendingApprovalID == request.ID {
			metadata.PendingApprovalID = ""
			metadata.PendingApprovalStatus = ""
			metadata.PendingApprovalToolName = ""
			metadata.PendingApprovalToolInput = ""
			metadata.PendingApprovalToolInputObject = nil
			metadata.PendingApprovalToolUseID = ""
			metadata.PendingApprovalProviderMsgID = ""
			metadata.PendingApprovalReason = ""
			metadata.PendingApprovalDecisionReason = ""
			metadata.PendingApprovalAcceptFeedback = ""
			metadata.PendingApprovalContentBlocks = nil
			metadata.PendingApprovalRunID = ""
			metadata.PendingApprovalUserMessageID = ""
			metadata.PendingApprovalCategory = ""
			metadata.PendingApprovalRuleSource = ""
		}
	})

	sess, ok := q.sessions.GetByID(request.SessionID)
	if !ok {
		return fmt.Errorf("session %q not found", request.SessionID)
	}
	userMessage, ok := q.findMessageByID(request.SessionID, request.UserMessageID)
	if !ok {
		return fmt.Errorf("user message %q not found", request.UserMessageID)
	}

	reply, err := q.executeTurnLoop(ctx, sess, userMessage, request.RunID, sink, &toolCall{
		name:              request.ToolName,
		input:             request.ToolInput,
		inputObject:       cloneAnyMap(request.ToolInputObject),
		toolUseID:         request.ToolUseID,
		providerMessageID: request.ProviderMessageID,
		acceptFeedback:    request.AcceptFeedback,
		contentBlocks:     cloneAnyMaps(request.ContentBlocks),
		skipPermission:    true,
	}, nil)
	if err != nil {
		if !isApprovalRequiredError(err) {
			q.emitRunError(sink, Event{Type: "run.error", Session: sess, RunID: request.RunID, Error: err.Error()})
		}
		return err
	}
	return q.emit(sink, Event{
		Type:    "agent.lifecycle.end",
		Session: sess,
		RunID:   request.RunID,
		Message: &reply,
	})
}

func (q *QueryEngine) RejectAndContinue(ctx context.Context, approvalID, feedback string, contentBlocks []map[string]any, sink EventSink) error {
	request, ok := q.approvals.Get(approvalID)
	if !ok {
		return fmt.Errorf("approval %q not found", approvalID)
	}
	ctx, release := q.beginRun(ctx, request.RunID, request.SessionID)
	defer release()
	if request.Status == approval.StatusApproved {
		return fmt.Errorf("approval %q was already approved", approvalID)
	}
	if request.Status == approval.StatusPending {
		updated, err := q.approvals.UpdateStatus(approvalID, approval.StatusRejected)
		if err != nil {
			return err
		}
		request = updated
	}
	q.clearPendingApprovalMetadata(request)

	sess, ok := q.sessions.GetByID(request.SessionID)
	if !ok {
		return fmt.Errorf("session %q not found", request.SessionID)
	}

	toolUseID := strings.TrimSpace(request.ToolUseID)
	if toolUseID == "" {
		toolUseID = fmt.Sprintf("toolu-%s-%s", request.RunID, strings.ReplaceAll(request.ToolName, ".", "-"))
	}
	rejectionMessage := rejectMessageWithFeedback(strings.TrimSpace(feedback))
	blocks := []model.MessageBlock{
		{
			Type:      model.MessageBlockToolResult,
			ToolUseID: toolUseID,
			Content:   rejectionMessage,
			IsError:   true,
		},
	}
	blocks = append(blocks, messageBlocksFromContentMaps(contentBlocks)...)
	toolMsg, err := q.sessions.AppendMessageWithBlocks(sess.ID, "tool", fmt.Sprintf("%s: Error: %s", request.ToolName, rejectionMessage), "", blocks)
	if err != nil {
		return err
	}
	q.appendMutableMessage(sess.ID, toolMsg)
	if err := q.emit(sink, Event{
		Type:              "tool.result",
		Session:           sess,
		RunID:             request.RunID,
		Message:           &toolMsg,
		ToolUseID:         toolUseID,
		ProviderMessageID: request.ProviderMessageID,
		ToolName:          request.ToolName,
		ToolInput:         request.ToolInput,
		ToolError:         true,
	}); err != nil {
		return err
	}
	reply, err := q.completeWithToolResult(ctx, sess, request.RunID, sink, toolMsg)
	if err != nil {
		if !isApprovalRequiredError(err) {
			q.emitRunError(sink, Event{Type: "run.error", Session: sess, RunID: request.RunID, Error: err.Error()})
		}
		return err
	}
	return q.emit(sink, Event{
		Type:    "agent.lifecycle.end",
		Session: sess,
		RunID:   request.RunID,
		Message: &reply,
	})
}

func (q *QueryEngine) completeWithPermissionRejection(ctx context.Context, sess session.Session, runID string, sink EventSink, pending *toolCall, reason string, contentBlocks []map[string]any) (session.Message, error) {
	toolUseID := strings.TrimSpace(pending.toolUseID)
	if toolUseID == "" {
		toolUseID = fmt.Sprintf("toolu-%s-%s", runID, strings.ReplaceAll(pending.name, ".", "-"))
	}
	rejectionMessage := strings.TrimSpace(reason)
	if rejectionMessage == "" {
		rejectionMessage = rejectMessage
	}
	blocks := []model.MessageBlock{
		{
			Type:      model.MessageBlockToolResult,
			ToolUseID: toolUseID,
			Content:   rejectionMessage,
			IsError:   true,
		},
	}
	blocks = append(blocks, messageBlocksFromContentMaps(contentBlocks)...)
	toolMsg, err := q.sessions.AppendMessageWithBlocks(sess.ID, "tool", fmt.Sprintf("%s: Error: %s", pending.name, rejectionMessage), "", blocks)
	if err != nil {
		return session.Message{}, err
	}
	q.appendMutableMessage(sess.ID, toolMsg)
	if err := q.emit(sink, Event{
		Type:              "tool.result",
		Session:           sess,
		RunID:             runID,
		Message:           &toolMsg,
		ToolUseID:         toolUseID,
		ProviderMessageID: pending.providerMessageID,
		ToolName:          pending.name,
		ToolInput:         pending.input,
		ToolInputObject:   cloneAnyMap(pending.inputObject),
		ToolError:         true,
	}); err != nil {
		return session.Message{}, err
	}
	return q.completeWithToolResult(ctx, sess, runID, sink, toolMsg)
}

func (q *QueryEngine) clearPendingApprovalMetadata(request approval.Request) {
	_ = q.sessions.UpdateMetadata(request.SessionID, func(metadata *session.SessionMetadata) {
		if metadata.PendingApprovalID == request.ID {
			metadata.PendingApprovalID = ""
			metadata.PendingApprovalStatus = ""
			metadata.PendingApprovalToolName = ""
			metadata.PendingApprovalToolInput = ""
			metadata.PendingApprovalToolInputObject = nil
			metadata.PendingApprovalToolUseID = ""
			metadata.PendingApprovalProviderMsgID = ""
			metadata.PendingApprovalReason = ""
			metadata.PendingApprovalDecisionReason = ""
			metadata.PendingApprovalAcceptFeedback = ""
			metadata.PendingApprovalContentBlocks = nil
			metadata.PendingApprovalRunID = ""
			metadata.PendingApprovalUserMessageID = ""
			metadata.PendingApprovalCategory = ""
			metadata.PendingApprovalRuleSource = ""
		}
	})
}

func (q *QueryEngine) Sessions() *session.Manager {
	return q.sessions
}

func (q *QueryEngine) MemoryService() *memory.Service {
	return q.memory
}

func (q *QueryEngine) ApprovalManager() *approval.Manager {
	return q.approvals
}

func (q *QueryEngine) SetReportToolProgress(report tools.ProgressFunc) {
	q.reportToolProgress = report
}

func (q *QueryEngine) Messages(sessionID string) []session.Message {
	q.ensureMutableMessages(sessionID)
	q.msgMu.RLock()
	defer q.msgMu.RUnlock()
	items := q.messages[sessionID]
	return append([]session.Message(nil), items...)
}

func (q *QueryEngine) toolUseContext(ctx context.Context, sess session.Session, pending *toolCall, runID string, sink EventSink) tools.ToolUseContext {
	policy := q.PermissionPolicyForSession(sess.ID)
	q.toolContextMu.Lock()
	appState := q.toolAppStates[sess.ID]
	if appState == nil {
		appState = make(map[string]any)
		q.toolAppStates[sess.ID] = appState
	}
	if len(q.skillRoots) > 0 {
		appState["skillRoots"] = append([]string(nil), q.skillRoots...)
	}
	if q.skillForkExecutor != nil {
		appState["skillForkExecutor"] = q.skillForkExecutor
	}
	if q.agentTaskExecutor != nil {
		appState["agentTaskExecutor"] = q.agentTaskExecutor
	}
	if len(q.mcpPrompts) > 0 {
		appState["mcpPrompts"] = cloneMCPPrompts(q.mcpPrompts)
	} else {
		delete(appState, "mcpPrompts")
	}
	if len(q.mcpSkills) > 0 {
		appState["mcpSkills"] = cloneMCPSkills(q.mcpSkills)
	} else {
		delete(appState, "mcpSkills")
	}
	if q.mcpPromptCaller != nil {
		appState["mcpPromptCaller"] = q.mcpPromptCaller
	} else {
		delete(appState, "mcpPromptCaller")
	}
	if len(q.mcpNeedsAuth) > 0 {
		appState["mcpAuth"] = mcpAuthAppState(q.mcpNeedsAuth)
	} else {
		delete(appState, "mcpAuth")
	}
	decisions := q.toolDecisions[sess.ID]
	if decisions == nil {
		decisions = make(map[string]tools.ToolDecision)
		q.toolDecisions[sess.ID] = decisions
	}
	q.toolContextMu.Unlock()

	reportProgress := func(progress tools.ToolProgress) {
		// Emit progress as shared runtime event first
		_ = q.emit(sink, Event{
			Type:      "tool.progress",
			Session:   sess,
			RunID:     runID,
			ToolName:  pending.name,
			ToolUseID: pending.toolUseID,
			Progress:  &progress,
		})
		// Then call legacy callback for compatibility
		if q.reportToolProgress != nil {
			q.reportToolProgress(progress)
		}
	}
	requestPrompt := q.requestPrompt
	if requestPrompt == nil {
		requestPrompt = func(string, string, tools.PromptRequest) (tools.PromptResponse, error) {
			return tools.PromptResponse{}, nil
		}
	}
	addNotification := q.addNotification
	if addNotification == nil {
		addNotification = func(tools.Notification) {}
	}
	handleElicitation := q.handleElicitation
	if handleElicitation == nil {
		handleElicitation = func(context.Context, tools.ElicitationRequest) (tools.ElicitationResult, error) {
			return tools.ElicitationResult{}, nil
		}
	}
	setConversationID := q.setConversationID
	if setConversationID == nil {
		setConversationID = func(string) {}
	}
	refreshTools := func() []tools.Definition {
		return q.exposedTools(sess.ID, true)
	}

	return tools.ToolUseContext{
		AbortContext:       ctx,
		Session:            sess,
		WorkDir:            resolveWorkDir(sess, q.workspace),
		ToolName:           pending.name,
		ToolUseID:          pending.toolUseID,
		Input:              pending.input,
		InputObject:        cloneAnyMap(pending.inputObject),
		Policy:             policy,
		AvailableTools:     q.exposedTools(sess.ID, true),
		AgentID:            sess.AgentID,
		MainLoopModel:      q.mainLoopModelForSession(sess.ID),
		LLMProvider:        q.llmProvider,
		Commands:           append([]tools.Command(nil), q.commands...),
		QuerySource:        q.querySource,
		CustomSystemPrompt: q.toolCustomSystemPrompt,
		AppendSystemPrompt: q.toolAppendSystemPrompt,
		Debug:              q.debug,
		Verbose:            q.verbose,
		ThinkingConfig:     cloneAnyMap(q.thinkingConfig),
		AgentDefinitions: tools.AgentDefinitions{
			ActiveAgents:      append([]string(nil), q.agentDefinitions.ActiveAgents...),
			AllowedAgentTypes: append([]string(nil), q.agentDefinitions.AllowedAgentTypes...),
			Definitions:       cloneAgentDefinitions(q.agentDefinitions.Definitions),
		},
		MaxBudgetUSD:            q.maxBudgetUSD,
		IsNonInteractive:        q.isNonInteractiveSession,
		RequireCanUseTool:       q.requireCanUseTool,
		QueryTracking:           q.queryTracking,
		ReadFileState:           cloneAnyMap(q.readFileState),
		ContentReplacementState: cloneAnyMap(q.contentReplacementState),
		CriticalSystemReminder:  q.criticalSystemReminder,
		PreserveToolUseResults:  q.preserveToolUseResults,
		RenderedSystemPrompt:    q.renderedSystemPrompt,
		Messages:                q.Messages(sess.ID),
		AppState:                appState,
		SetAppState:             q.setToolAppStateFunc(sess.ID),
		ToolDecisions:           decisions,
		FileReadingLimits:       q.fileReadingLimits,
		GlobLimits:              q.globLimits,
		MCPClients:              append([]tools.MCPConnection(nil), q.mcpClients...),
		MCPResources:            cloneMCPResources(q.mcpResources),
		MCPResourceReader:       q.mcpResourceReader,
		MCPResourceLister:       q.mcpResourceLister,
		MCPContextualToolCaller: q.mcpContextualToolCaller,
		MCPOAuthStore:           q.mcpOAuthStore,
		MCPAuthenticator:        q.mcpAuthenticator,
		MCPReconnect:            q.mcpReconnectFunc(),
		RequestPrompt:           requestPrompt,
		ReportProgress:          reportProgress,
		AddNotification:         addNotification,
		RefreshTools:            refreshTools,
		HandleElicitation:       handleElicitation,
		SetConversationID:       setConversationID,
		CanUseTool:              q.canUseToolFunc(ctx, sess),
	}
}

func (q *QueryEngine) mcpReconnectFunc() tools.MCPReconnectFunc {
	if q.mcpReconnect == nil {
		return nil
	}
	return func(ctx context.Context, server string) (tools.MCPReconnectResult, error) {
		return q.reconnectMCPServer(ctx, server)
	}
}
