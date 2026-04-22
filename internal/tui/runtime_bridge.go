package tui

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"myclaw/internal/approval"
	"myclaw/internal/diagnostics"
	"myclaw/internal/orchestration"
	"myclaw/internal/queryengine"
	"myclaw/internal/runtime"
	"myclaw/internal/session"
	"myclaw/internal/tools"
)

type RuntimeBridge struct {
	ctx      context.Context
	sessions *session.Manager
	runner   *runtime.Runner
	session  session.Session
	logger   *diagnostics.Logger

	mu   sync.RWMutex
	send func(tea.Msg)
}

func NewRuntimeBridge(sessions *session.Manager, runner *runtime.Runner, agentID string) *RuntimeBridge {
	return NewRuntimeBridgeWithContext(context.Background(), sessions, runner, agentID, nil)
}

func NewRuntimeBridgeWithContext(ctx context.Context, sessions *session.Manager, runner *runtime.Runner, agentID string, logger *diagnostics.Logger) *RuntimeBridge {
	if sessions == nil {
		sessions = session.NewManager(nil)
	}
	bridge := &RuntimeBridge{
		ctx:      ctx,
		sessions: sessions,
		runner:   runner,
		session:  sessions.GetOrCreateMain(agentID),
		logger:   logger,
	}
	if runner != nil {
		runner.SetReportToolProgress(func(progress tools.ToolProgress) {
			bridge.dispatch(RuntimeEventMsg{Event: runtime.RuntimeEvent{
				Type:      "tool.progress",
				Session:   bridge.session,
				ToolUseID: progress.ToolUseID,
				Progress:  cloneToolProgress(progress),
			}})
		})
	}
	return bridge
}

func (b *RuntimeBridge) Attach(send func(tea.Msg)) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.send = send
}

func (b *RuntimeBridge) SendUserMessage(input string) error {
	if err := b.ctx.Err(); err != nil {
		return err
	}
	b.log("info", "tui.bridge", "send_message.begin", "dispatching user message", "", map[string]any{
		"session_id": b.session.ID,
		"input":      input,
	})
	msg, err := b.sessions.AppendMessage(b.session.ID, "user", input)
	if err != nil {
		return err
	}
	if b.runner == nil {
		return nil
	}
	go func() {
		err := b.runner.HandleUserMessage(b.ctx, b.session, msg, sinkFunc(func(event runtime.RuntimeEvent) error {
			b.logRuntimeEvent(event)
			b.dispatch(RuntimeEventMsg{Event: event})
			return nil
		}))
		if err != nil {
			var approvalErr *queryengine.ApprovalRequiredError
			if errors.As(err, &approvalErr) {
				b.log("info", "tui.bridge", "send_message.awaiting_approval", approvalErr.Error(), "", map[string]any{
					"tool_name":  approvalErr.ToolName,
					"tool_input": approvalErr.ToolInput,
				})
				return
			}
			b.log("error", "tui.bridge", "send_message.error", err.Error(), "", nil)
			b.dispatch(BridgeErrMsg{Err: err})
		}
	}()
	return nil
}

func (b *RuntimeBridge) SessionSnapshots() []sessionSnapshot {
	sessions := b.sessions.ListSessions()
	snapshots := make([]sessionSnapshot, 0, len(sessions))
	for _, sess := range sessions {
		messages, _ := b.sessions.Messages(sess.ID)
		snapshot := sessionSnapshot{
			Session:      sess,
			MessageCount: len(messages),
		}
		if len(messages) > 0 && snapshot.Session.Metadata.LastActivityAt.IsZero() {
			snapshot.Session.Metadata.LastActivityAt = messages[len(messages)-1].CreatedAt
		}
		for _, message := range messages {
			if snapshot.FirstUserMessage == "" && message.Role == "user" {
				snapshot.FirstUserMessage = message.Content
			}
			if strings.TrimSpace(message.Content) != "" {
				snapshot.LastMessage = message.Content
			}
		}
		snapshots = append(snapshots, snapshot)
	}
	return snapshots
}

func (b *RuntimeBridge) ResumeSession(id string) (session.RecoverySnapshot, bool) {
	snapshot, ok := b.sessions.RecoverySnapshot(id)
	if !ok {
		return session.RecoverySnapshot{}, false
	}
	b.session = snapshot.Session
	b.log("info", "tui.bridge", "session.resume", "resumed session", "", map[string]any{"session_id": id})
	return snapshot, true
}

func (b *RuntimeBridge) TaskPanelSnapshot() taskPanelSnapshot {
	snapshot := taskPanelSnapshot{SessionID: b.session.ID}
	if b.runner == nil {
		return snapshot
	}

	var coordinator *orchestration.Coordinator
	if hook := b.runner.Orchestrator(); hook != nil {
		coordinator = coordinatorFromHook(hook)
	}

	for _, run := range b.runner.AgentManager().List() {
		if run.ParentSessionID != b.session.ID {
			continue
		}
		task := taskSnapshot{
			RunID:               run.ID,
			Label:               run.Label,
			Prompt:              run.Prompt,
			Status:              string(run.Status),
			ParentSessionID:     run.ParentSessionID,
			ChildSessionID:      run.ChildSessionID,
			ChildSessionKey:     run.ChildSessionKey,
			Output:              run.Output,
			ControlMessageCount: len(b.runner.AgentManager().ControlMessages(run.ID)),
		}
		if run.Err != nil {
			task.Error = run.Err.Error()
		}
		if messages, ok := b.sessions.Messages(run.ChildSessionID); ok {
			task.MessageCount = len(messages)
			for i := len(messages) - 1; i >= 0; i-- {
				if messages[i].Role == "assistant" && strings.TrimSpace(messages[i].Content) != "" {
					task.LastAssistant = messages[i].Content
					break
				}
			}
		}
		if coordinator != nil {
			if state, ok := coordinator.GetRun(run.ID); ok {
				task.LastEvent = state.LastEvent
				task.Message = state.Message
				task.NextAction = state.NextAction
				task.RecommendedRole = state.RecommendedRole
				task.RecommendedAction = state.RecommendedAction
				task.DecisionPriority = state.DecisionPriority
				task.DecisionReason = state.DecisionReason
				task.AutoExecutable = state.AutoExecutable
			}
		}
		switch run.Status {
		case "running":
			snapshot.RunningCount++
		case "completed":
			snapshot.CompletedCount++
		case "failed":
			snapshot.FailedCount++
		case "stopped":
			snapshot.StoppedCount++
		}
		snapshot.Tasks = append(snapshot.Tasks, task)
	}
	return snapshot
}

func (b *RuntimeBridge) PlatformStatusSnapshot() platformStatusSnapshot {
	snapshot := platformStatusSnapshot{
		SessionID:  b.session.ID,
		SessionKey: b.session.Key,
		AgentID:    b.session.AgentID,
		IsMain:     b.session.IsMain,
	}
	if b.runner == nil {
		return snapshot
	}

	policy := b.runner.PermissionPolicyForSession(b.session.ID)
	snapshot.WorkspaceRoots = append([]string(nil), policy.WorkspaceRoots...)
	snapshot.BaseModel = b.runner.BaseMainLoopModelForSession(b.session.ID)
	snapshot.ModelOverride = b.runner.SessionMainLoopModelOverride(b.session.ID)
	snapshot.ResolvedModel = b.runner.ResolvedMainLoopModelForSession(b.session.ID)
	if models, err := b.runner.AvailableModels(b.ctx); err == nil {
		snapshot.AvailableModels = make([]platformModelOption, 0, len(models))
		for _, model := range models {
			snapshot.AvailableModels = append(snapshot.AvailableModels, platformModelOption{
				Value:               model.ID,
				Label:               valueOrUnset(model.DisplayName),
				Description:         strings.TrimSpace(model.Description),
				ContextWindowTokens: model.ContextWindowTokens,
				MaxOutputTokens:     model.MaxOutputTokens,
			})
		}
	}

	inventory := b.runner.MCPInventory()
	snapshot.MCPServerCount = inventory.ServerCount
	snapshot.MCPToolCount = inventory.ToolCount
	snapshot.MCPPromptCount = inventory.PromptCount
	snapshot.MCPResourceCount = inventory.ResourceCount
	return snapshot
}

func (b *RuntimeBridge) SetSessionModel(model string) error {
	if err := b.ctx.Err(); err != nil {
		return err
	}
	if b.runner == nil {
		return nil
	}
	return b.runner.SetSessionMainLoopModelOverride(b.session.ID, model)
}

func (b *RuntimeBridge) ClearSessionModel() error {
	if err := b.ctx.Err(); err != nil {
		return err
	}
	if b.runner == nil {
		return nil
	}
	return b.runner.ClearSessionMainLoopModelOverride(b.session.ID)
}

func (b *RuntimeBridge) ContextSnapshot() contextSnapshot {
	if b.runner == nil {
		return contextSnapshot{}
	}
	snapshot, err := b.runner.ContextSnapshot(b.session.ID)
	if err != nil {
		return contextSnapshot{}
	}
	return contextSnapshot{
		Model:               snapshot.Model,
		UsedTokens:          snapshot.UsedTokens,
		ContextWindowTokens: snapshot.ContextWindowTokens,
		UsagePercent:        snapshot.UsagePercent,
		CategoryLines:       append([]string(nil), snapshot.CategoryLines...),
	}
}

func (b *RuntimeBridge) CompactionSnapshot() compactionSnapshot {
	if b.runner == nil {
		return compactionSnapshot{}
	}
	snapshot, err := b.runner.CompactionSnapshot(b.session.ID)
	if err != nil {
		return compactionSnapshot{}
	}
	return compactionSnapshot{
		EstimatedTokens:      snapshot.Analysis.EstimatedTokens,
		ContextWindowTokens:  snapshot.Analysis.ContextWindowTokens,
		WarningThreshold:     snapshot.Analysis.WarningThreshold,
		ErrorThreshold:       snapshot.Analysis.ErrorThreshold,
		AutoCompactThreshold: snapshot.Analysis.AutoCompactThreshold,
		BlockingThreshold:    snapshot.Analysis.BlockingThreshold,
		AboveWarning:         snapshot.Analysis.IsAboveWarningThreshold,
		AboveError:           snapshot.Analysis.IsAboveErrorThreshold,
		AboveAutoCompact:     snapshot.Analysis.IsAboveAutoCompactThreshold,
		AtBlockingLimit:      snapshot.Analysis.IsAtBlockingLimit,
		LastCompactionReason: snapshot.LastCompactionReason,
		LastCompactedAtLabel: formatTimestamp(snapshot.LastCompactedAt),
	}
}

func (b *RuntimeBridge) CompactSession(customInstructions string) (compactionActionResult, error) {
	if err := b.ctx.Err(); err != nil {
		return compactionActionResult{}, err
	}
	if b.runner == nil {
		return compactionActionResult{}, nil
	}
	result, err := b.runner.CompactSession(b.session.ID, customInstructions)
	if err != nil {
		return compactionActionResult{}, err
	}
	return compactionActionResult{
		Changed:        result.Changed,
		Reason:         result.Reason,
		OriginalCount:  result.OriginalCount,
		CompactedCount: result.CompactedCount,
	}, nil
}

func (b *RuntimeBridge) MicrocompactSession() (compactionActionResult, error) {
	if err := b.ctx.Err(); err != nil {
		return compactionActionResult{}, err
	}
	if b.runner == nil {
		return compactionActionResult{}, nil
	}
	result, err := b.runner.MicrocompactSession(b.session.ID)
	if err != nil {
		return compactionActionResult{}, err
	}
	return compactionActionResult{
		Changed:        result.Changed,
		Reason:         result.Reason,
		OriginalCount:  result.OriginalCount,
		CompactedCount: result.CompactedCount,
	}, nil
}

func (b *RuntimeBridge) MCPSnapshot() mcpSnapshot {
	if b.runner == nil {
		return mcpSnapshot{}
	}
	runtimeServers := b.runner.MCPServers()
	servers := make([]mcpServerSnapshot, 0, len(runtimeServers))
	for _, server := range runtimeServers {
		servers = append(servers, mcpServerSnapshot{
			Name:          server.Name,
			TransportType: server.TransportType,
			Endpoint:      server.Endpoint,
			Enabled:       server.Enabled,
			Tools:         append([]string(nil), server.Tools...),
			Prompts:       append([]string(nil), server.Prompts...),
			Resources:     append([]string(nil), server.Resources...),
		})
	}
	return mcpSnapshot{Servers: servers}
}

func (b *RuntimeBridge) Approve(id string) error {
	if err := b.ctx.Err(); err != nil {
		return err
	}
	b.log("info", "tui.bridge", "approval.approve.begin", "approving pending request", "", map[string]any{"approval_id": id})
	go func() {
		err := b.runner.ApproveAndContinue(b.ctx, id, sinkFunc(func(event runtime.RuntimeEvent) error {
			b.logRuntimeEvent(event)
			b.dispatch(RuntimeEventMsg{Event: event})
			return nil
		}))
		if err != nil {
			var approvalErr *queryengine.ApprovalRequiredError
			if errors.As(err, &approvalErr) {
				b.log("info", "tui.bridge", "approval.awaiting_approval", approvalErr.Error(), "", map[string]any{
					"approval_id": id,
					"tool_name":   approvalErr.ToolName,
					"tool_input":  approvalErr.ToolInput,
				})
				return
			}
			b.log("error", "tui.bridge", "approval.approve.error", err.Error(), "", map[string]any{"approval_id": id})
			b.dispatch(BridgeErrMsg{Err: err})
		}
	}()
	return nil
}

func (b *RuntimeBridge) Reject(id string) error {
	if err := b.ctx.Err(); err != nil {
		return err
	}
	updated, err := b.runner.UpdateApprovalStatus(id, approval.StatusRejected)
	if err != nil {
		return err
	}
	b.log("warn", "tui.bridge", "approval.reject", "approval rejected", updated.RunID, map[string]any{"approval_id": id})
	b.dispatch(RuntimeEventMsg{Event: runtime.RuntimeEvent{
		Type:     "approval.updated",
		Session:  b.session,
		Approval: &updated,
	}})
	return nil
}

func (b *RuntimeBridge) dispatch(msg tea.Msg) {
	b.mu.RLock()
	defer b.mu.RUnlock()
	if b.send != nil {
		b.log("debug", "tui.bridge", "dispatch", fmt.Sprintf("%T", msg), "", nil)
		b.send(msg)
	}
}

func (b *RuntimeBridge) logRuntimeEvent(event runtime.RuntimeEvent) {
	fields := map[string]any{}
	if event.ToolName != "" {
		fields["tool_name"] = event.ToolName
	}
	if event.ToolUseID != "" {
		fields["tool_use_id"] = event.ToolUseID
	}
	if event.ToolInput != "" {
		fields["tool_input"] = event.ToolInput
	}
	if event.ToolInputObject != nil {
		fields["tool_input_object"] = event.ToolInputObject
	}
	if event.Approval != nil {
		fields["approval_id"] = event.Approval.ID
	}
	if event.Message != nil {
		fields["message_role"] = event.Message.Role
		fields["message_content"] = event.Message.Content
	}
	message := event.Error
	if message == "" && event.Delta != "" {
		message = event.Delta
	}
	b.log("debug", "runtime", event.Type, message, event.RunID, fields)
}

func (b *RuntimeBridge) log(level, component, event, message, runID string, fields map[string]any) {
	if b.logger == nil {
		return
	}
	_ = b.logger.Log(diagnostics.Entry{
		Level:     level,
		Component: component,
		Event:     event,
		Message:   message,
		SessionID: b.session.ID,
		RunID:     runID,
		Fields:    fields,
	})
}

func coordinatorFromHook(hook orchestration.Hook) *orchestration.Coordinator {
	switch typed := hook.(type) {
	case *orchestration.Coordinator:
		return typed
	case orchestration.Chain:
		for _, child := range typed {
			if coordinator := coordinatorFromHook(child); coordinator != nil {
				return coordinator
			}
		}
	}
	return nil
}

func formatTimestamp(value time.Time) string {
	if value.IsZero() {
		return ""
	}
	return value.UTC().Format("2006-01-02 15:04 MST")
}

type sinkFunc func(runtime.RuntimeEvent) error

func (s sinkFunc) Emit(event runtime.RuntimeEvent) error {
	return s(event)
}
