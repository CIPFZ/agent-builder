package runtime

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
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
	Type                  string
	Session               session.Session
	RunID                 string
	Message               *session.Message
	Delta                 string
	ToolName              string
	ToolInput             string
	ToolInputObject       map[string]any
	DecisionReason        string
	DecisionReasonDetails map[string]any
	AcceptFeedback        string
	ContentBlocks         []map[string]any
	Error                 string
	Approval              *approval.Request
}

type Options struct {
	PermissionPolicy          permissions.Policy
	Compactor                 *compaction.Service
	AgentManager              *agent.Manager
	MemoryService             *memory.Service
	ApprovalManager           *approval.Manager
	Orchestrator              orchestration.Hook
	PermissionHook            queryengine.PermissionHook
	PreToolUseHook            queryengine.PreToolUseHook
	PostToolUseHook           queryengine.PostToolUseHook
	PostToolUseFailureHook    queryengine.PostToolUseFailureHook
	PermissionUpdatePersister queryengine.PermissionUpdatePersister
	MainLoopModel             string
	LLMProvider               string
	Commands                  []tools.Command
	QuerySource               string
	CustomSystemPrompt        string
	AppendSystemPrompt        string
	ToolCustomSystemPrompt    string
	ToolAppendSystemPrompt    string
	Debug                     bool
	Verbose                   bool
	ThinkingConfig            map[string]any
	AgentDefinitions          tools.AgentDefinitions
	MaxBudgetUSD              float64
	IsNonInteractiveSession   bool
	RequireCanUseTool         bool
	QueryTracking             tools.QueryTracking
	ReadFileState             map[string]any
	ContentReplacementState   map[string]any
	CriticalSystemReminder    string
	PreserveToolUseResults    bool
	RenderedSystemPrompt      string
	FileReadingLimits         tools.ResourceLimits
	GlobLimits                tools.ResourceLimits
	MCPClients                []tools.MCPConnection
	MCPResources              map[string][]tools.MCPResource
	MCPResourceReader         tools.MCPResourceReader
	MCPResourceLister         tools.MCPResourceLister
	MCPTools                  map[string]tools.MCPToolsListResult
	MCPToolCaller             tools.MCPToolCaller
	MCPContextualToolCaller   tools.MCPContextualToolCaller
	MCPOAuthStore             tools.MCPOAuthStore
	MCPPrompts                map[string]tools.MCPPromptsListResult
	MCPPromptCaller           tools.MCPPromptCaller
	MCPAuthenticator          tools.MCPAuthenticator
	MCPReconnect              tools.MCPReconnectFunc
	SkillRoots                []string
	SkillDiscovery            tools.SkillDiscoveryOptions
	SkillForkExecutor         tools.SkillForkExecutor
	RequestPrompt             tools.RequestPromptFunc
	ReportToolProgress        tools.ProgressFunc
	AddNotification           tools.AddNotificationFunc
	HandleElicitation         tools.ElicitationFunc
	SetConversationID         tools.SetConversationIDFunc
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
			systemtools.NewBashTool(router),
			systemtools.NewPowerShellTool(router),
			systemtools.NewRunTool(router),
			tools.NewReadTool(),
			tools.NewWriteTool(),
			tools.NewEditTool(),
			tools.NewMultiEditTool(),
			tools.NewGlobTool(),
			tools.NewGrepTool(),
			tools.NewLSTool(),
			tools.NewTodoWriteTool(),
			tools.NewWebFetchTool(nil),
			tools.NewWebSearchTool(),
			tools.NewListMcpResourcesTool(),
			tools.NewReadMcpResourceTool(),
			tools.NewSkillTool(),
			tools.NewNotebookEditTool(),
			tools.NewEnterPlanModeTool(),
			tools.NewExitPlanModeTool(),
		)
		toolRegistry.Register(tools.NewClaudeAgentTool(options.AgentManager, nil))
		toolRegistry.Register(tools.NewAgentTaskTool(options.AgentManager, nil))
		toolRegistry.Register(tools.NewClaudeToolSearchTool(toolRegistry))
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
	if len(options.MCPClients) > 0 {
		discovered, err := tools.DiscoverMCPClientTools(context.Background(), options.MCPClients)
		if err == nil {
			if len(discovered.Tools) > 0 {
				if options.MCPTools == nil {
					options.MCPTools = make(map[string]tools.MCPToolsListResult)
				}
				for server, result := range discovered.Tools {
					if _, exists := options.MCPTools[server]; !exists {
						options.MCPTools[server] = result
					}
				}
			}
			if len(discovered.Prompts) > 0 {
				if options.MCPPrompts == nil {
					options.MCPPrompts = make(map[string]tools.MCPPromptsListResult)
				}
				for server, result := range discovered.Prompts {
					if _, exists := options.MCPPrompts[server]; !exists {
						options.MCPPrompts[server] = result
					}
				}
			}
			if options.MCPToolCaller == nil {
				options.MCPToolCaller = discovered.Caller
			}
			if options.MCPContextualToolCaller == nil {
				options.MCPContextualToolCaller = discovered.ContextualCaller
			}
			if options.MCPPromptCaller == nil {
				options.MCPPromptCaller = discovered.PromptCaller
			}
			if len(discovered.Resources) > 0 {
				if options.MCPResources == nil {
					options.MCPResources = make(map[string][]tools.MCPResource)
				}
				for server, resources := range discovered.Resources {
					if _, exists := options.MCPResources[server]; !exists {
						options.MCPResources[server] = append([]tools.MCPResource(nil), resources...)
					}
				}
			}
			if options.MCPResourceReader == nil {
				options.MCPResourceReader = discovered.ResourceReader
			}
			if options.MCPResourceLister == nil {
				options.MCPResourceLister = discovered.ResourceLister
			}
			if options.MCPReconnect == nil {
				options.MCPReconnect = discovered.Reconnect
			}
		}
	}
	rehydratePendingApprovals(sessions, options.ApprovalManager)

	runner := &Runner{
		sessions: sessions,
		options:  options,
	}
	if runner.options.SkillForkExecutor == nil {
		runner.options.SkillForkExecutor = runner.defaultSkillForkExecutor
	}
	if len(runner.options.SkillRoots) > 0 {
		tools.AddSkillDirectories(runner.options.SkillRoots)
	}
	if runner.options.SkillDiscovery.CWD != "" ||
		runner.options.SkillDiscovery.ConfigHome != "" ||
		runner.options.SkillDiscovery.ManagedRoot != "" ||
		len(runner.options.SkillDiscovery.AdditionalDirs) > 0 {
		tools.LoadClaudeSkillDirectories(runner.options.SkillDiscovery)
	}
	runner.engine = queryengine.New(queryengine.Config{
		Sessions:                  sessions,
		Client:                    client,
		WorkspaceLoader:           workspaceLoader,
		ToolRegistry:              toolRegistry,
		AgentManager:              options.AgentManager,
		PermissionPolicy:          options.PermissionPolicy,
		Compactor:                 options.Compactor,
		MemoryService:             options.MemoryService,
		ApprovalManager:           options.ApprovalManager,
		PermissionHook:            options.PermissionHook,
		PreToolUseHook:            options.PreToolUseHook,
		PostToolUseHook:           options.PostToolUseHook,
		PostToolUseFailureHook:    options.PostToolUseFailureHook,
		PermissionUpdatePersister: options.PermissionUpdatePersister,
		MainLoopModel:             options.MainLoopModel,
		LLMProvider:               options.LLMProvider,
		Commands:                  options.Commands,
		QuerySource:               options.QuerySource,
		CustomSystemPrompt:        options.CustomSystemPrompt,
		AppendSystemPrompt:        options.AppendSystemPrompt,
		ToolCustomSystemPrompt:    options.ToolCustomSystemPrompt,
		ToolAppendSystemPrompt:    options.ToolAppendSystemPrompt,
		Debug:                     options.Debug,
		Verbose:                   options.Verbose,
		ThinkingConfig:            options.ThinkingConfig,
		AgentDefinitions:          options.AgentDefinitions,
		MaxBudgetUSD:              options.MaxBudgetUSD,
		IsNonInteractiveSession:   options.IsNonInteractiveSession,
		RequireCanUseTool:         options.RequireCanUseTool,
		QueryTracking:             options.QueryTracking,
		ReadFileState:             options.ReadFileState,
		ContentReplacementState:   options.ContentReplacementState,
		CriticalSystemReminder:    options.CriticalSystemReminder,
		PreserveToolUseResults:    options.PreserveToolUseResults,
		RenderedSystemPrompt:      options.RenderedSystemPrompt,
		FileReadingLimits:         options.FileReadingLimits,
		GlobLimits:                options.GlobLimits,
		MCPClients:                options.MCPClients,
		MCPResources:              options.MCPResources,
		MCPResourceReader:         options.MCPResourceReader,
		MCPResourceLister:         options.MCPResourceLister,
		MCPTools:                  runner.options.MCPTools,
		MCPToolCaller:             runner.options.MCPToolCaller,
		MCPContextualToolCaller:   runner.options.MCPContextualToolCaller,
		MCPOAuthStore:             runner.options.MCPOAuthStore,
		MCPPrompts:                runner.options.MCPPrompts,
		MCPPromptCaller:           runner.options.MCPPromptCaller,
		MCPAuthenticator:          runner.options.MCPAuthenticator,
		MCPReconnect:              runner.options.MCPReconnect,
		SkillRoots:                runner.options.SkillRoots,
		SkillForkExecutor:         runner.options.SkillForkExecutor,
		RequestPrompt:             options.RequestPrompt,
		ReportToolProgress:        options.ReportToolProgress,
		AddNotification:           options.AddNotification,
		HandleElicitation:         options.HandleElicitation,
		SetConversationID:         options.SetConversationID,
	})
	return runner
}

func (r *Runner) defaultSkillForkExecutor(ctx context.Context, request tools.SkillForkRequest) (tools.ToolResult, error) {
	label := request.Command.Agent
	if label == "" {
		label = request.Command.Name
	}
	promptText := strings.TrimSpace(request.Command.Content)
	var frontmatter []string
	if len(request.Command.AllowedTools) > 0 {
		frontmatter = append(frontmatter, "Allowed tools: "+strings.Join(request.Command.AllowedTools, ", "))
	}
	if request.Command.Model != "" {
		frontmatter = append(frontmatter, "Model: "+request.Command.Model)
	}
	if request.Command.Effort != "" {
		frontmatter = append(frontmatter, "Effort: "+request.Command.Effort)
	}
	if len(frontmatter) > 0 {
		promptText = strings.Join(frontmatter, "\n") + "\n\n" + promptText
	}
	if request.Args != "" {
		promptText += "\n\nArguments: " + request.Args
	}
	if promptText == "" {
		promptText = request.Command.Name
	}
	run, err := r.SpawnSubagentWithOptions(ctx, request.ToolContext.Session, label, promptText, SubagentOptions{
		AllowedTools: request.Command.AllowedTools,
		Model:        request.Command.Model,
		Effort:       request.Command.Effort,
	})
	if err != nil {
		return tools.ToolResult{}, err
	}
	completed, err := r.options.AgentManager.Wait(ctx, run.ID, 0)
	if err != nil {
		return tools.ToolResult{}, err
	}
	output := map[string]any{
		"success":   true,
		"status":    "forked",
		"agent":     label,
		"runId":     completed.ID,
		"result":    completed.Output,
		"sessionId": completed.ChildSessionID,
	}
	encoded, err := json.Marshal(output)
	if err != nil {
		return tools.ToolResult{}, err
	}
	return tools.ToolResult{Output: string(encoded)}, nil
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
	return r.SpawnSubagentWithOptions(ctx, parent, label, promptText, SubagentOptions{})
}

type SubagentOptions struct {
	AllowedTools []string
	Model        string
	Effort       string
}

func (r *Runner) SpawnSubagentWithOptions(ctx context.Context, parent session.Session, label, promptText string, options SubagentOptions) (*agent.Run, error) {
	key := fmt.Sprintf("agent:%s:child:%d", parent.AgentID, len(r.options.AgentManager.List())+1)
	child := r.sessions.CreateChild(parent.AgentID, key)
	childPolicy := r.PermissionPolicyForSession(parent.ID).DeriveForSubagent()
	childPolicy.Rules = append(skillAllowedToolRules(options.AllowedTools), childPolicy.Rules...)
	r.engine.SetSessionPermissionPolicy(child.ID, childPolicy)
	if options.Model != "" || options.Effort != "" {
		_ = r.sessions.UpdateMetadata(child.ID, func(metadata *session.SessionMetadata) {
			if strings.TrimSpace(options.Model) != "" {
				if strings.TrimSpace(metadata.InitialMainLoopModel) == "" {
					metadata.InitialMainLoopModel = r.BaseMainLoopModelForSession(parent.ID)
				}
				metadata.MainLoopModelOverride = strings.TrimSpace(options.Model)
			}
			metadata.MainLoopEffortOverride = strings.TrimSpace(options.Effort)
		})
	}

	run, err := r.options.AgentManager.Spawn(ctx, agent.SpawnRequest{
		ParentSessionID: parent.ID,
		ParentAgentID:   parent.AgentID,
		ChildSessionID:  child.ID,
		ChildSessionKey: child.Key,
		Label:           label,
		Prompt:          promptText,
		AllowedTools:    append([]string(nil), options.AllowedTools...),
		Model:           strings.TrimSpace(options.Model),
		Effort:          strings.TrimSpace(options.Effort),
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

func skillAllowedToolRules(toolNames []string) []permissions.Rule {
	rules := make([]permissions.Rule, 0, len(toolNames))
	seen := make(map[string]struct{}, len(toolNames))
	for _, toolName := range toolNames {
		toolName = strings.TrimSpace(toolName)
		if toolName == "" {
			continue
		}
		key := strings.ToLower(toolName)
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		rules = append(rules, permissions.Rule{
			ToolName: toolName,
			Source:   string(permissions.RuleSourceCommand),
			Action:   permissions.ActionAllow,
		})
	}
	return rules
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
	continuation, ok := r.sessions.ContinuationState(previous.ChildSessionID)
	if !ok {
		return nil, fmt.Errorf("child session %q recovery state not found", previous.ChildSessionID)
	}
	if previous.Status == agent.StatusRunning {
		return nil, fmt.Errorf("run %q is still running and cannot be resumed", previous.ID)
	}
	if pending, ok := r.pendingApprovalForSession(previous.ChildSessionID); ok {
		return nil, fmt.Errorf(
			"child session %q has pending approval %q and is not ready for a new prompt",
			previous.ChildSessionID,
			pending.ID,
		)
	}
	if !continuation.ReadyForPrompt {
		return nil, fmt.Errorf(
			"child session %q is not ready for a new prompt (status=%s anchor=%s)",
			previous.ChildSessionID,
			continuation.Status,
			continuation.ResumeFromMessageID,
		)
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

func (r *Runner) pendingApprovalForSession(sessionID string) (approval.Request, bool) {
	items := r.options.ApprovalManager.ListBySessionAndStatus(sessionID, approval.StatusPending)
	if len(items) == 0 {
		return approval.Request{}, false
	}
	return items[0], true
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

func (r *Runner) SetSessionMainLoopModelOverride(sessionID, model string) error {
	return r.engine.SetSessionMainLoopModelOverride(sessionID, model)
}

func (r *Runner) ClearSessionMainLoopModelOverride(sessionID string) error {
	return r.engine.ClearSessionMainLoopModelOverride(sessionID)
}

func (r *Runner) BaseMainLoopModelForSession(sessionID string) string {
	return r.engine.BaseMainLoopModelForSession(sessionID)
}

func (r *Runner) SessionMainLoopModelOverride(sessionID string) string {
	return r.engine.SessionMainLoopModelOverride(sessionID)
}

func (r *Runner) ResolvedMainLoopModelForSession(sessionID string) string {
	return r.engine.ResolvedMainLoopModelForSession(sessionID)
}

func (r *Runner) MemoryService() *memory.Service {
	return r.engine.MemoryService()
}

func (r *Runner) ApprovalManager() *approval.Manager {
	return r.engine.ApprovalManager()
}

func (r *Runner) UpdateApprovalStatus(approvalID string, status approval.Status) (approval.Request, error) {
	updated, err := r.options.ApprovalManager.UpdateStatus(approvalID, status)
	if err != nil {
		return approval.Request{}, err
	}
	_ = r.sessions.UpdateMetadata(updated.SessionID, func(metadata *session.SessionMetadata) {
		if metadata.PendingApprovalID == updated.ID {
			if status == approval.StatusPending {
				metadata.PendingApprovalStatus = string(status)
				return
			}
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
	return updated, nil
}

func (r *Runner) UpdateApprovalPromptMetadata(approvalID, acceptFeedback string, contentBlocks []map[string]any) (approval.Request, error) {
	updated, err := r.options.ApprovalManager.UpdatePromptMetadata(approvalID, acceptFeedback, contentBlocks)
	if err != nil {
		return approval.Request{}, err
	}
	_ = r.sessions.UpdateMetadata(updated.SessionID, func(metadata *session.SessionMetadata) {
		if metadata.PendingApprovalID == updated.ID {
			metadata.PendingApprovalAcceptFeedback = updated.AcceptFeedback
			metadata.PendingApprovalContentBlocks = cloneAnyMaps(updated.ContentBlocks)
		}
	})
	return updated, nil
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

func (r *Runner) RejectAndContinue(ctx context.Context, approvalID, feedback string, contentBlocks []map[string]any, sink EventSink) error {
	return r.engine.RejectAndContinue(ctx, approvalID, feedback, contentBlocks, querySinkFunc(func(event queryengine.Event) error {
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
		Type:                  event.Type,
		Session:               event.Session,
		RunID:                 event.RunID,
		Message:               event.Message,
		Delta:                 event.Delta,
		ToolName:              event.ToolName,
		ToolInput:             event.ToolInput,
		ToolInputObject:       cloneAnyMap(event.ToolInputObject),
		DecisionReason:        event.DecisionReason,
		DecisionReasonDetails: cloneAnyMap(event.DecisionReasonDetails),
		AcceptFeedback:        event.AcceptFeedback,
		ContentBlocks:         cloneAnyMaps(event.ContentBlocks),
		Error:                 event.Error,
		Approval:              event.Approval,
	}
}

func rehydratePendingApprovals(sessions *session.Manager, approvals *approval.Manager) {
	if sessions == nil || approvals == nil {
		return
	}
	for _, sess := range sessions.ListSessions() {
		meta := sess.Metadata
		if meta.PendingApprovalID == "" || meta.PendingApprovalStatus != string(approval.StatusPending) {
			continue
		}
		approvals.Restore(approval.Request{
			ID:                meta.PendingApprovalID,
			SessionID:         sess.ID,
			RunID:             meta.PendingApprovalRunID,
			UserMessageID:     meta.PendingApprovalUserMessageID,
			ToolName:          meta.PendingApprovalToolName,
			ToolInput:         meta.PendingApprovalToolInput,
			ToolInputObject:   cloneAnyMap(meta.PendingApprovalToolInputObject),
			ToolUseID:         meta.PendingApprovalToolUseID,
			ProviderMessageID: meta.PendingApprovalProviderMsgID,
			Category:          meta.PendingApprovalCategory,
			RuleSource:        meta.PendingApprovalRuleSource,
			Reason:            meta.PendingApprovalReason,
			DecisionReason:    meta.PendingApprovalDecisionReason,
			AcceptFeedback:    meta.PendingApprovalAcceptFeedback,
			ContentBlocks:     cloneAnyMaps(meta.PendingApprovalContentBlocks),
			Status:            approval.StatusPending,
			CreatedAt:         meta.LastActivityAt,
		})
	}
}

func cloneAnyMap(input map[string]any) map[string]any {
	if input == nil {
		return nil
	}
	cloned := make(map[string]any, len(input))
	for key, value := range input {
		cloned[key] = value
	}
	return cloned
}

func cloneAnyMaps(input []map[string]any) []map[string]any {
	if input == nil {
		return nil
	}
	cloned := make([]map[string]any, 0, len(input))
	for _, item := range input {
		cloned = append(cloned, cloneAnyMap(item))
	}
	return cloned
}
