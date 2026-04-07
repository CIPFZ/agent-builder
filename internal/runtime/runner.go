package runtime

import (
	"context"
	"fmt"
	"sync"

	"myclaw/internal/agent"
	"myclaw/internal/approval"
	"myclaw/internal/compaction"
	"myclaw/internal/llm"
	"myclaw/internal/memory"
	"myclaw/internal/orchestration"
	"myclaw/internal/permissions"
	"myclaw/internal/queryengine"
	"myclaw/internal/sandbox"
	"myclaw/internal/session"
	"myclaw/internal/tools"
	systemtools "myclaw/internal/tools/system"
	"myclaw/internal/workspace"
)

type EventSink interface {
	Emit(RuntimeEvent) error
}

type RuntimeEvent struct {
	Type      string
	Session   session.Session
	RunID     string
	Message   *session.Message
	Delta     string
	ToolName  string
	ToolInput string
	Error     string
	Approval  *approval.Request
}

type Options struct {
	PermissionPolicy permissions.Policy
	Compactor        *compaction.Service
	AgentManager     *agent.Manager
	MemoryService    *memory.Service
	ApprovalManager  *approval.Manager
	Orchestrator     orchestration.Hook
}

type Runner struct {
	sessions *session.Manager
	options  Options
	engine   *queryengine.QueryEngine
	policyMu sync.RWMutex
}

func NewRunner(sessions *session.Manager, client llm.Client, workspaceLoader *workspace.Loader, toolRegistry *tools.Registry) *Runner {
	return NewRunnerWithOptions(sessions, client, workspaceLoader, toolRegistry, Options{
		PermissionPolicy: permissions.Policy{Mode: permissions.ModeDangerFullAccess},
	})
}

func NewRunnerWithOptions(sessions *session.Manager, client llm.Client, workspaceLoader *workspace.Loader, toolRegistry *tools.Registry, options Options) *Runner {
	if client == nil {
		client = llm.NewMockClient()
	}
	if workspaceLoader == nil {
		workspaceLoader = workspace.NewLoader("")
	}
	if options.AgentManager == nil {
		options.AgentManager = agent.NewManager()
	}
	if toolRegistry == nil {
		router := sandbox.NewRouter(nil, nil)
		toolRegistry = tools.NewRegistry(
			tools.NewTextUpperTool(),
			systemtools.NewRunTool(router),
		)
		toolRegistry.Register(tools.NewAgentTaskTool(options.AgentManager, nil))
		toolRegistry.Register(tools.NewToolSearchTool(toolRegistry))
	}
	if options.MemoryService == nil {
		options.MemoryService = memory.NewService()
	}
	if options.ApprovalManager == nil {
		options.ApprovalManager = approval.NewManager()
	}
	if options.Compactor == nil {
		options.Compactor = compaction.NewService(compaction.Config{
			MaxMessages:             6,
			MaxEstimatedTokens:      180,
			ContextWindowTokens:     256,
			WarningBufferTokens:     64,
			ErrorBufferTokens:       32,
			AutoCompactBufferTokens: 20,
			BlockingBufferTokens:    8,
			PreserveRecentTurns:     3,
			SummaryPrefix:           "Summary:",
		})
	}

	return &Runner{
		sessions:  sessions,
		options:   options,
		engine: queryengine.New(queryengine.Config{
			Sessions:         sessions,
			Client:           client,
			WorkspaceLoader:  workspaceLoader,
			ToolRegistry:     toolRegistry,
			AgentManager:     options.AgentManager,
			PermissionPolicy: options.PermissionPolicy,
			Compactor:        options.Compactor,
			MemoryService:    options.MemoryService,
			ApprovalManager:  options.ApprovalManager,
		}),
	}
}

func (r *Runner) HandleUserMessage(ctx context.Context, sess session.Session, userMessage session.Message, sink EventSink) error {
	return r.engine.SubmitMessage(ctx, sess, userMessage, querySinkFunc(func(event queryengine.Event) error {
		runtimeEvent := fromQueryEvent(event)
		if runtimeEvent.Type == "run.error" {
			emitRunError(sink, runtimeEvent)
			return nil
		}
		return r.emitEvent(ctx, sink, runtimeEvent)
	}))
}

func (r *Runner) SpawnSubagent(ctx context.Context, parent session.Session, label, promptText string) (*agent.Run, error) {
	key := fmt.Sprintf("agent:%s:child:%d", parent.AgentID, len(r.options.AgentManager.List())+1)
	child := r.sessions.CreateChild(parent.AgentID, key)
	r.engine.SetSessionPermissionPolicy(child.ID, r.PermissionPolicyForSession(parent.ID).DeriveForSubagent())

	run, err := r.options.AgentManager.Spawn(ctx, agent.SpawnRequest{
		ParentSessionID: parent.ID,
		ParentAgentID:   parent.AgentID,
		ChildSessionID:  child.ID,
		ChildSessionKey: child.Key,
		Label:           label,
		Prompt:          promptText,
		Run: func(ctx context.Context, runCtx agent.RunContext) (string, error) {
			msg, err := r.sessions.AppendMessage(runCtx.ChildSessionID, "user", promptText)
			if err != nil {
				return "", err
			}
			if err := r.HandleUserMessage(ctx, child, msg, nil); err != nil {
				return "", err
			}
			messages, ok := r.sessions.Messages(runCtx.ChildSessionID)
			if !ok || len(messages) == 0 {
				return "", nil
			}
			for i := len(messages) - 1; i >= 0; i-- {
				if messages[i].Role == "assistant" {
					return messages[i].Content, nil
				}
			}
			return "", nil
		},
	})
	if err != nil {
		return nil, err
	}
	return run, nil
}

func (r *Runner) ResumeSubagent(ctx context.Context, runID, label, promptText string) (*agent.Run, error) {
	previous, ok := r.options.AgentManager.Get(runID)
	if !ok {
		return nil, fmt.Errorf("run %q not found", runID)
	}
	child, ok := r.sessions.GetByID(previous.ChildSessionID)
	if !ok {
		return nil, fmt.Errorf("child session %q not found", previous.ChildSessionID)
	}
	if label == "" {
		label = previous.Label
	}
	if _, ok := r.sessionPolicy(previous.ChildSessionID); !ok {
		parentPolicy := r.PermissionPolicyForSession(previous.ParentSessionID)
		r.engine.SetSessionPermissionPolicy(previous.ChildSessionID, parentPolicy.DeriveForSubagent())
	}
	return r.options.AgentManager.Spawn(ctx, agent.SpawnRequest{
		ParentSessionID: previous.ParentSessionID,
		ParentAgentID:   previous.ParentAgentID,
		ChildSessionID:  previous.ChildSessionID,
		ChildSessionKey: previous.ChildSessionKey,
		Label:           label,
		Prompt:          promptText,
		Run: func(ctx context.Context, runCtx agent.RunContext) (string, error) {
			msg, err := r.sessions.AppendMessage(runCtx.ChildSessionID, "user", promptText)
			if err != nil {
				return "", err
			}
			if err := r.HandleUserMessage(ctx, child, msg, nil); err != nil {
				return "", err
			}
			messages, ok := r.sessions.Messages(runCtx.ChildSessionID)
			if !ok || len(messages) == 0 {
				return "", nil
			}
			for i := len(messages) - 1; i >= 0; i-- {
				if messages[i].Role == "assistant" {
					return messages[i].Content, nil
				}
			}
			return "", nil
		},
	})
}

func emitRunError(sink EventSink, event RuntimeEvent) {
	if sink == nil {
		return
	}
	_ = sink.Emit(event)
}

func resolveWorkDir(sess session.Session, loader *workspace.Loader) string {
	if loader == nil {
		return sess.Key
	}
	ctx, err := loader.Load()
	if err != nil || ctx.Root == "" {
		return sess.Key
	}
	return ctx.Root
}

func (r *Runner) AgentManager() *agent.Manager {
	return r.options.AgentManager
}

func (r *Runner) PermissionPolicyForSession(sessionID string) permissions.Policy {
	return r.engine.PermissionPolicyForSession(sessionID)
}

func (r *Runner) SetSessionPermissionPolicy(sessionID string, policy permissions.Policy, cascade bool) {
	r.engine.SetSessionPermissionPolicy(sessionID, policy)
	if !cascade {
		return
	}
	r.cascadeSessionPolicy(sessionID, policy)
}

func (r *Runner) MemoryService() *memory.Service {
	return r.engine.MemoryService()
}

func (r *Runner) ApprovalManager() *approval.Manager {
	return r.engine.ApprovalManager()
}

func (r *Runner) Orchestrator() orchestration.Hook {
	return r.options.Orchestrator
}

func (r *Runner) ApproveAndContinue(ctx context.Context, approvalID string, sink EventSink) error {
	return r.engine.ApproveAndContinue(ctx, approvalID, querySinkFunc(func(event queryengine.Event) error {
		runtimeEvent := fromQueryEvent(event)
		if runtimeEvent.Type == "run.error" {
			emitRunError(sink, runtimeEvent)
			return nil
		}
		return r.emitEvent(ctx, sink, runtimeEvent)
	}))
}

func (r *Runner) emitEvent(ctx context.Context, sink EventSink, event RuntimeEvent) error {
	if sink != nil {
		if err := sink.Emit(event); err != nil {
			return err
		}
	}
	if r.options.Orchestrator != nil {
		if err := r.options.Orchestrator.Handle(ctx, orchestration.Event{
			Type:       event.Type,
			SessionID:  event.Session.ID,
			SessionKey: event.Session.Key,
			AgentID:    event.Session.AgentID,
			RunID:      event.RunID,
			ToolName:   event.ToolName,
			ToolInput:  event.ToolInput,
			Message:    event.Error,
		}); err != nil {
			return err
		}
	}
	return nil
}

func (r *Runner) sessionPolicy(sessionID string) (permissions.Policy, bool) {
	policy := r.engine.PermissionPolicyForSession(sessionID)
	if policy.Mode == "" {
		return permissions.Policy{}, false
	}
	return policy, true
}

func (r *Runner) cascadeSessionPolicy(parentSessionID string, parentPolicy permissions.Policy) {
	runs := r.options.AgentManager.List()
	queue := []string{parentSessionID}
	visited := map[string]struct{}{parentSessionID: {}}

	for len(queue) > 0 {
		current := queue[0]
		queue = queue[1:]

		for _, run := range runs {
			if run.ParentSessionID != current {
				continue
			}
			if _, seen := visited[run.ChildSessionID]; seen {
				continue
			}
			childPolicy := parentPolicy.DeriveForSubagent()
			r.engine.SetSessionPermissionPolicy(run.ChildSessionID, childPolicy)
			visited[run.ChildSessionID] = struct{}{}
			queue = append(queue, run.ChildSessionID)
		}
	}
}

type querySinkFunc func(queryengine.Event) error

func (s querySinkFunc) Emit(event queryengine.Event) error {
	return s(event)
}

func fromQueryEvent(event queryengine.Event) RuntimeEvent {
	return RuntimeEvent{
		Type:      event.Type,
		Session:   event.Session,
		RunID:     event.RunID,
		Message:   event.Message,
		Delta:     event.Delta,
		ToolName:  event.ToolName,
		ToolInput: event.ToolInput,
		Error:     event.Error,
		Approval:  event.Approval,
	}
}
