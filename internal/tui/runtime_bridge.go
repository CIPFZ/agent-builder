package tui

import (
	"context"
	"errors"
	"fmt"
	"sync"

	tea "github.com/charmbracelet/bubbletea"

	"myclaw/internal/approval"
	"myclaw/internal/diagnostics"
	"myclaw/internal/queryengine"
	"myclaw/internal/runtime"
	"myclaw/internal/session"
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
	return &RuntimeBridge{
		ctx:      ctx,
		sessions: sessions,
		runner:   runner,
		session:  sessions.GetOrCreateMain(agentID),
		logger:   logger,
	}
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
	updated, err := b.runner.ApprovalManager().UpdateStatus(id, approval.StatusRejected)
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
	if event.ToolInput != "" {
		fields["tool_input"] = event.ToolInput
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

type sinkFunc func(runtime.RuntimeEvent) error

func (s sinkFunc) Emit(event runtime.RuntimeEvent) error {
	return s(event)
}

type Options struct {
	LLMLabel string
	Logger   *diagnostics.Logger
}

func Run(ctx context.Context, sessions *session.Manager, runner *runtime.Runner, _ any, _ any, options Options) error {
	if runner == nil {
		return errors.New("nil runner")
	}
	bridge := NewRuntimeBridgeWithContext(ctx, sessions, runner, "main", options.Logger)
	model := NewModel(bridge, ModelConfig{
		SessionID: bridge.session.ID,
		LLMLabel:  options.LLMLabel,
		LogPath: func() string {
			if options.Logger == nil {
				return ""
			}
			return options.Logger.Path()
		}(),
	})
	bridge.log("info", "tui", "startup", fmt.Sprintf("starting session %s", bridge.session.ID), "", map[string]any{
		"llm": options.LLMLabel,
		"log_path": func() string {
			if options.Logger == nil {
				return ""
			}
			return options.Logger.Path()
		}(),
	})
	program := tea.NewProgram(model, tea.WithAltScreen(), tea.WithMouseCellMotion())
	bridge.Attach(program.Send)
	_, err := program.Run()
	if err != nil {
		bridge.log("error", "tui", "shutdown.error", err.Error(), "", nil)
	} else {
		bridge.log("info", "tui", "shutdown", "program exited cleanly", "", nil)
	}
	return err
}
