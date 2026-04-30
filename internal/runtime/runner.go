package runtime

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"myclaw/internal/agent"
	"myclaw/internal/agents"
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
	ToolUseID             string
	ProviderMessageID     string
	ToolName              string
	ToolInput             string
	ToolInputObject       map[string]any
	ToolError             bool
	Progress              *tools.ToolProgress
	StructuredContent     any
	Meta                  map[string]any
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
	MaxTurns                  int
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
	ModelCatalog              llm.ModelCatalog
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
	MCPSkills                 map[string][]tools.SkillCommand
	MCPPromptCaller           tools.MCPPromptCaller
	MCPNeedsAuth              map[string]tools.MCPAuthToolResult
	MCPAuthenticator          tools.MCPAuthenticator
	MCPReconnect              tools.MCPReconnectFunc
	DisableMCPPromptSkills    bool
	ExtensionLifecycle        []tools.ExtensionLifecycleRecord
	LSPServers                []tools.LSPServerConfig
	LSPHandler                tools.LSPHandler
	BundledSkills             tools.BundledSkillOptions
	SkillRoots                []string
	SkillDiscovery            tools.SkillDiscoveryOptions
	SkillForkExecutor         tools.SkillForkExecutor
	AgentTaskExecutor         tools.AgentTaskExecutor
	AgentDiscovery            AgentDiscoveryOptions
	RequestPrompt             tools.RequestPromptFunc
	ReportToolProgress        tools.ProgressFunc
	AddNotification           tools.AddNotificationFunc
	HandleElicitation         tools.ElicitationFunc
	SetConversationID         tools.SetConversationIDFunc
	WorktreeManager           WorktreeManager
}

type AgentDiscoveryOptions = agents.DiscoveryOptions

type Runner struct {
	sessions    *session.Manager
	options     Options
	engine      *queryengine.QueryEngine
	policyMu    sync.RWMutex
	stateMu     sync.RWMutex
	stateErrors []string
	remote      *RemoteManager
}

type MCPInventory = queryengine.MCPInventory
type MCPServerSnapshot = queryengine.MCPServerSnapshot
type ExtensionInventory = queryengine.ExtensionInventory
type ExtensionInventorySummary = queryengine.ExtensionInventorySummary
type ExtensionTool = queryengine.ExtensionTool
type ExtensionCommand = queryengine.ExtensionCommand
type ExtensionSkill = queryengine.ExtensionSkill
type ExtensionBoundary = queryengine.ExtensionBoundary
type ExtensionLifecycleOperationResult = tools.ExtensionLifecycleOperationResult

func NewRunner(sessions *session.Manager, client llm.Client, workspaceLoader *workspace.Loader, toolRegistry *tools.Registry) *Runner {
	return NewRunnerWithOptions(sessions, client, workspaceLoader, toolRegistry, Options{
		PermissionPolicy: permissions.Policy{Mode: permissions.ModeDangerFullAccess},
	})
}

func NewRunnerWithOptions(sessions *session.Manager, client llm.Client, workspaceLoader *workspace.Loader, toolRegistry *tools.Registry, options Options) *Runner {
	if client == nil {
		client = llm.NewUnavailableClient("llm client is not configured")
	}
	if workspaceLoader == nil {
		workspaceLoader = workspace.NewLoader("")
	}
	if options.AgentManager == nil {
		options.AgentManager = agent.NewManager()
	}
	if options.WorktreeManager == nil {
		options.WorktreeManager = newGitWorktreeManager()
	}
	if toolRegistry == nil {
		router := sandbox.NewRouter(nil, nil)
		toolRegistry = tools.NewRegistry(
			tools.NewTextUpperTool(),
			systemtools.NewBashTool(router),
			systemtools.NewPowerShellTool(router),
			systemtools.NewRunTool(router),
			systemtools.NewSSHTool(router),
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
	if options.MCPOAuthStore == nil {
		options.MCPOAuthStore = tools.NewDefaultMCPOAuthStore()
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
	if needsAgentDiscovery(options.AgentDiscovery) {
		loaded := agents.LoadClaudeAgentDefinitions(agents.DiscoveryOptions(options.AgentDiscovery))
		if options.AgentDefinitions.Definitions == nil {
			options.AgentDefinitions.Definitions = make(map[string]tools.AgentDefinition)
		}
		activeNames := make([]string, 0, len(loaded.Active))
		for _, definition := range loaded.Active {
			options.AgentDefinitions.Definitions[definition.AgentType] = tools.AgentDefinition{
				AgentType:       definition.AgentType,
				SystemPrompt:    definition.SystemPrompt,
				MemoryScope:     definition.MemoryScope,
				MaxTurns:        definition.MaxTurns,
				Background:      definition.Background,
				Isolation:       definition.Isolation,
				InitialPrompt:   definition.InitialPrompt,
				PermissionMode:  definition.PermissionMode,
				DisallowedTools: append([]string(nil), definition.DisallowedTools...),
			}
			activeNames = append(activeNames, definition.AgentType)
		}
		if len(activeNames) > 0 {
			options.AgentDefinitions.ActiveAgents = append([]string(nil), activeNames...)
			options.AgentDefinitions.AllowedAgentTypes = append([]string(nil), activeNames...)
		}
	}
	rehydratePendingApprovals(sessions, options.ApprovalManager)
	stateErrors := rehydrateAgentRuns(sessions, options.AgentManager)

	runner := &Runner{
		sessions:    sessions,
		options:     options,
		stateErrors: stateErrors,
	}
	runner.remote = NewRemoteManager(sessions)
	runner.options.AgentManager.SetUpdateHook(runner.persistAgentRun)
	if runner.options.SkillForkExecutor == nil {
		runner.options.SkillForkExecutor = runner.defaultSkillForkExecutor
	}
	if runner.options.AgentTaskExecutor == nil {
		runner.options.AgentTaskExecutor = runner.defaultAgentTaskExecutor
	}
	if len(tools.GetBundledSkills()) == 0 {
		bundled := runner.options.BundledSkills
		if bundled.CWD == "" {
			bundled.CWD = runner.options.SkillDiscovery.CWD
		}
		tools.InitClaudeBundledSkills(bundled)
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
		MaxTurns:                  options.MaxTurns,
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
		ModelCatalog:              options.ModelCatalog,
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
		MCPSkills:                 runner.options.MCPSkills,
		MCPPromptCaller:           runner.options.MCPPromptCaller,
		MCPNeedsAuth:              runner.options.MCPNeedsAuth,
		MCPAuthenticator:          runner.options.MCPAuthenticator,
		MCPReconnect:              runner.options.MCPReconnect,
		DisableMCPPromptSkills:    runner.options.DisableMCPPromptSkills,
		ExtensionLifecycle:        runner.options.ExtensionLifecycle,
		LSPServers:                runner.options.LSPServers,
		LSPHandler:                runner.options.LSPHandler,
		SkillRoots:                runner.options.SkillRoots,
		SkillForkExecutor:         runner.options.SkillForkExecutor,
		AgentTaskExecutor:         runner.options.AgentTaskExecutor,
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
		AgentType:    strings.TrimSpace(request.Command.Agent),
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

func (r *Runner) defaultAgentTaskExecutor(ctx context.Context, request tools.AgentTaskRequest) (tools.ToolResult, error) {
	label := strings.TrimSpace(request.Label)
	if label == "" {
		label = "task"
	}
	promptText := strings.TrimSpace(request.Prompt)
	if promptText == "" {
		promptText = label
	}
	options := SubagentOptions{
		AgentType:               request.AgentType,
		Isolation:               request.Isolation,
		CWD:                     request.CWD,
		RemoteIsolationBoundary: request.RemoteIsolationBoundary,
		AllowedTools:            append([]string(nil), request.AllowedTools...),
		PermissionMode:          request.PermissionMode,
		OutputFile:              request.OutputFile,
		UseFork:                 shouldUseForkSubagent(request.ToolContext.Input, request.AgentType),
	}
	runInBackground := r.shouldRunSubagentInBackground(request)
	options.RunInBackground = runInBackground
	run, err := r.SpawnSubagentWithOptions(ctx, request.ToolContext.Session, label, promptText, options)
	if err != nil {
		return tools.ToolResult{}, err
	}
	if runInBackground {
		return encodeAsyncLaunchedSubagentResult(*run, label, promptText, request.ToolContext)
	}
	waitCtx, cancel := context.WithTimeout(ctx, 50*time.Millisecond)
	defer cancel()
	completed, err := r.options.AgentManager.Wait(waitCtx, run.ID, 0)
	if err != nil {
		if ctx.Err() != nil {
			return tools.ToolResult{}, ctx.Err()
		}
		return encodeDelegatedTaskToolResult(*run, label, "", true, request.ToolContext)
	}
	return encodeDelegatedTaskToolResult(completed, label, completed.Output, false, request.ToolContext)
}

func (r *Runner) shouldRunSubagentInBackground(request tools.AgentTaskRequest) bool {
	if request.RunInBackground {
		return true
	}
	agentType := strings.TrimSpace(request.AgentType)
	if agentType == "" {
		return false
	}
	def, ok := r.options.AgentDefinitions.Definitions[agentType]
	return ok && def.Background
}

type delegatedTaskToolResult struct {
	Success           bool   `json:"success"`
	Agent             string `json:"agent"`
	RunID             string `json:"runId"`
	AgentID           string `json:"agentId,omitempty"`
	SessionID         string `json:"sessionId"`
	SessionKey        string `json:"sessionKey,omitempty"`
	Status            string `json:"status"`
	LastAction        string `json:"lastAction,omitempty"`
	Prompt            string `json:"prompt,omitempty"`
	Result            string `json:"result,omitempty"`
	OutputFile        string `json:"outputFile,omitempty"`
	CanReadOutputFile *bool  `json:"canReadOutputFile,omitempty"`
	Background        bool   `json:"background,omitempty"`
	Error             string `json:"error,omitempty"`
}

func encodeDelegatedTaskToolResult(run agent.Run, label, result string, background bool, toolCtx tools.ToolUseContext) (tools.ToolResult, error) {
	output := delegatedTaskToolResult{
		Success:    true,
		Agent:      label,
		RunID:      run.ID,
		SessionID:  run.ChildSessionID,
		SessionKey: run.ChildSessionKey,
		Status:     string(run.Status),
		LastAction: string(run.LastAction),
		Background: background,
	}
	if background {
		output.AgentID = run.ID
		output.Prompt = run.Prompt
		output.OutputFile = subagentOutputPath(run.ID)
		canReadOutputFile := canReadSubagentOutput(toolCtx)
		output.CanReadOutputFile = &canReadOutputFile
	}
	if result != "" {
		output.Result = result
	}
	if run.ErrorSummary != "" {
		output.Error = run.ErrorSummary
	}
	encoded, err := json.Marshal(output)
	if err != nil {
		return tools.ToolResult{}, err
	}
	return tools.ToolResult{Output: string(encoded)}, nil
}

func encodeAsyncLaunchedSubagentResult(run agent.Run, label, _ string, toolCtx tools.ToolUseContext) (tools.ToolResult, error) {
	return encodeDelegatedTaskToolResult(run, label, "", true, toolCtx)
}

func canReadSubagentOutput(toolCtx tools.ToolUseContext) bool {
	for _, def := range toolCtx.AvailableTools {
		switch strings.TrimSpace(def.Name) {
		case "Read", "Bash":
			return true
		}
	}
	return false
}

func subagentOutputPath(runID string) string {
	runID = strings.TrimSpace(runID)
	if runID == "" {
		runID = "agent"
	}
	return filepath.Join(os.TempDir(), "myclaw-subagents", runID+".log")
}

func writeSubagentOutputFile(runID, content string) error {
	path := subagentOutputPath(runID)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	return os.WriteFile(path, []byte(content), 0o644)
}

func (r *Runner) HandleUserMessage(ctx context.Context, sess session.Session, userMessage session.Message, sink EventSink) error {
	if latest, ok := r.sessions.GetByID(sess.ID); ok {
		sess = latest
	}
	return r.engine.SubmitMessage(ctx, sess, userMessage, querySinkFunc(func(event queryengine.Event) error {
		runtimeEvent := fromQueryEvent(event)
		if runtimeEvent.Type == "run.error" {
			emitRunError(sink, runtimeEvent)
			return nil
		}
		return r.emitEvent(ctx, sink, runtimeEvent)
	}))
}

func (r *Runner) State() queryengine.State {
	return r.engine.State()
}

func (r *Runner) SpawnSubagent(ctx context.Context, parent session.Session, label, promptText string) (*agent.Run, error) {
	return r.SpawnSubagentWithOptions(ctx, parent, label, promptText, SubagentOptions{})
}

type SubagentOptions struct {
	AllowedTools            []string
	Model                   string
	Effort                  string
	AgentType               string
	Isolation               string
	CWD                     string
	RemoteIsolationBoundary string
	PermissionMode          string
	RunInBackground         bool
	ParentRunID             string
	ContinuationMode        string
	OutputFile              string
	UseFork                 bool
}

const forkSubagentType = "fork"

func needsAgentDiscovery(opts AgentDiscoveryOptions) bool {
	return strings.TrimSpace(opts.CWD) != "" ||
		strings.TrimSpace(opts.ConfigHome) != "" ||
		strings.TrimSpace(opts.ManagedRoot) != "" ||
		len(opts.AdditionalDirs) > 0
}

func (r *Runner) SpawnSubagentWithOptions(ctx context.Context, parent session.Session, label, promptText string, options SubagentOptions) (*agent.Run, error) {
	if latest, ok := r.sessions.GetByID(parent.ID); ok {
		parent = latest
	}
	resolvedAgentType := strings.TrimSpace(options.AgentType)
	isForkPath := options.UseFork
	if isForkPath && isForkChildSession(parent) {
		return nil, fmt.Errorf("Fork is not available inside a forked worker. Complete your task directly using your tools.")
	}
	key := fmt.Sprintf("agent:%s:child:%d", parent.AgentID, len(r.options.AgentManager.List())+1)
	childAgentID := parent.AgentID
	if isForkPath {
		childAgentID = forkSubagentType
	} else {
		childAgentID = resolvedAgentType
	}
	child := r.sessions.CreateChild(childAgentID, key)
	parentPolicy := r.PermissionPolicyForSession(parent.ID)
	var resolvedDefinition tools.AgentDefinition
	hasResolvedDefinition := false
	if !isForkPath {
		resolvedDefinition, hasResolvedDefinition = r.options.AgentDefinitions.Definitions[resolvedAgentType]
	}
	childPolicy := parentPolicy.DeriveForSubagent()
	if hasResolvedDefinition {
		childPolicy = applyAgentPermissionModeOverride(parentPolicy, childPolicy, resolvedDefinition.PermissionMode)
		childPolicy.Rules = append(agentDisallowedToolRules(resolvedDefinition.DisallowedTools), childPolicy.Rules...)
	}
	childPolicy = applyAgentPermissionModeOverride(parentPolicy, childPolicy, options.PermissionMode)
	effectiveIsolation := strings.TrimSpace(options.Isolation)
	if effectiveIsolation == "" && hasResolvedDefinition {
		effectiveIsolation = strings.TrimSpace(resolvedDefinition.Isolation)
	}
	var worktree AgentWorktree
	if effectiveIsolation == "worktree" {
		baseDir := r.workspaceRootForSession(parent)
		createdWorktree, createErr := r.options.WorktreeManager.Create(ctx, baseDir, child.ID)
		if createErr != nil {
			return nil, createErr
		}
		worktree = createdWorktree
		childPolicy = rewritePolicyForWorktree(childPolicy, worktree.Path, baseDir)
	}
	cwdOverride := filepath.Clean(strings.TrimSpace(options.CWD))
	if cwdOverride == "." {
		cwdOverride = ""
	}
	if cwdOverride != "" && effectiveIsolation != "worktree" && !cwdWithinWorkspaceRoots(cwdOverride, parentPolicy.WorkspaceRoots) {
		return nil, fmt.Errorf("subagent cwd %q is outside inherited workspace roots", cwdOverride)
	}
	if cwdOverride != "" && effectiveIsolation != "worktree" {
		childPolicy = restrictPolicyToWorkspaceRoot(childPolicy, cwdOverride)
	}
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
	_ = r.sessions.UpdateMetadata(child.ID, func(metadata *session.SessionMetadata) {
		if isForkPath {
			metadata.AgentType = forkSubagentType
			metadata.AgentSystemPrompt = forkParentSystemPrompt(parent, r.options.RenderedSystemPrompt)
			metadata.AgentMemoryScope = ""
			metadata.AgentMaxTurns = 0
		} else if resolvedAgentType != "" {
			metadata.AgentType = resolvedAgentType
			if hasResolvedDefinition {
				metadata.AgentSystemPrompt = strings.TrimSpace(resolvedDefinition.SystemPrompt)
				metadata.AgentMemoryScope = strings.TrimSpace(resolvedDefinition.MemoryScope)
				metadata.AgentMaxTurns = resolvedDefinition.MaxTurns
				metadata.AgentIsolation = strings.TrimSpace(resolvedDefinition.Isolation)
			}
		}
		if effectiveIsolation == "worktree" {
			metadata.AgentIsolation = "worktree"
			metadata.AgentWorktreePath = worktree.Path
			metadata.AgentWorktreeBranch = worktree.Branch
			metadata.AgentWorktreeHeadCommit = worktree.HeadCommit
			metadata.AgentWorktreeGitRoot = worktree.GitRoot
		} else if cwdOverride != "" {
			metadata.AgentCWD = cwdOverride
			if strings.TrimSpace(metadata.AgentIsolation) == "" {
				metadata.AgentIsolation = "cwd"
			}
		}
	})
	effectivePrompt := promptText
	if isForkPath {
		effectivePrompt = buildForkSubagentPrompt(promptText)
		if effectiveIsolation == "worktree" {
			effectivePrompt += "\n\n" + buildForkWorktreeNotice(r.workspaceRootForSession(parent), worktree.Path)
		}
	} else if hasResolvedDefinition {
		if initial := strings.TrimSpace(resolvedDefinition.InitialPrompt); initial != "" {
			if strings.TrimSpace(effectivePrompt) != "" {
				effectivePrompt = initial + "\n\n" + effectivePrompt
			} else {
				effectivePrompt = initial
			}
		}
	}

	run, err := r.options.AgentManager.Spawn(ctx, agent.SpawnRequest{
		ParentSessionID:         parent.ID,
		ParentAgentID:           parent.AgentID,
		ChildSessionID:          child.ID,
		ChildSessionKey:         child.Key,
		Label:                   label,
		Prompt:                  effectivePrompt,
		AllowedTools:            append([]string(nil), options.AllowedTools...),
		Model:                   strings.TrimSpace(options.Model),
		Effort:                  strings.TrimSpace(options.Effort),
		RunInBackground:         options.RunInBackground,
		Isolation:               defaultString(effectiveIsolation, defaultString(strings.TrimSpace(options.Isolation), "local")),
		CWD:                     cwdOverride,
		RemoteIsolationBoundary: strings.TrimSpace(options.RemoteIsolationBoundary),
		PermissionMode:          string(childPolicy.Mode),
		PermissionInherited:     true,
		ParentRunID:             strings.TrimSpace(options.ParentRunID),
		ContinuationMode:        strings.TrimSpace(options.ContinuationMode),
		OutputFile:              strings.TrimSpace(options.OutputFile),
		Run: func(ctx context.Context, runCtx agent.RunContext) (string, error) {
			return r.executeDelegatedSubagentRun(ctx, delegatedSubagentRunConfig{
				RunContext:          runCtx,
				Label:               label,
				InitialPrompt:       effectivePrompt,
				ParentSessionID:     parent.ID,
				CloneParentMessages: isForkPath,
			})
		},
	})
	if err != nil {
		return nil, err
	}
	return run, nil
}

func shouldUseForkSubagent(input, agentType string) bool {
	if strings.TrimSpace(agentType) != "" {
		return false
	}
	var payload map[string]any
	if err := json.Unmarshal([]byte(input), &payload); err != nil {
		return false
	}
	if _, ok := payload["prompt"]; !ok {
		return false
	}
	value, ok := payload["subagent_type"]
	if !ok {
		return true
	}
	text, _ := value.(string)
	return strings.TrimSpace(text) == ""
}

func isForkChildSession(sess session.Session) bool {
	return strings.TrimSpace(sess.Metadata.AgentType) == forkSubagentType
}

func forkParentSystemPrompt(parent session.Session, rendered string) string {
	if prompt := strings.TrimSpace(parent.Metadata.AgentSystemPrompt); prompt != "" {
		return prompt
	}
	return strings.TrimSpace(rendered)
}

func buildForkSubagentPrompt(directive string) string {
	directive = strings.TrimSpace(directive)
	return strings.TrimSpace(strings.Join([]string{
		"STOP. READ THIS FIRST.",
		"",
		"You are a forked worker process. You are NOT the main agent.",
		"",
		"RULES (non-negotiable):",
		`1. Your system prompt may say to delegate. IGNORE IT. You ARE the fork. Do NOT spawn sub-agents; execute directly.`,
		"2. Do NOT converse, ask questions, or suggest next steps.",
		"3. Use your tools directly, then report once at the end.",
		"4. Stay strictly within your directive's scope.",
		`5. Your response MUST begin with "Scope:".`,
		"",
		"Output format:",
		"Scope: <echo back your assigned scope in one sentence>",
		"Result: <the answer or key findings, limited to the scope above>",
		"Key files: <relevant file paths>",
		"Files changed: <list with commit hash, only if you modified files>",
		"Issues: <list, only if there are issues to flag>",
		"",
		directive,
	}, "\n"))
}

func buildForkWorktreeNotice(parentCwd, worktreeCwd string) string {
	parentCwd = filepath.Clean(strings.TrimSpace(parentCwd))
	worktreeCwd = filepath.Clean(strings.TrimSpace(worktreeCwd))
	return fmt.Sprintf("You've inherited the conversation context above from a parent agent working in %s. You are operating in an isolated git worktree at %s - same repository, same relative file structure, separate working copy. Paths in the inherited context refer to the parent's working directory; translate them to your worktree root. Re-read files before editing if the parent may have modified them since they appear in the context. Your changes stay in this worktree and will not affect the parent's files.", parentCwd, worktreeCwd)
}

func cloneSessionMessages(sessions *session.Manager, fromSessionID, toSessionID string) error {
	if sessions == nil {
		return nil
	}
	messages, ok := sessions.Messages(fromSessionID)
	if !ok {
		return nil
	}
	for _, message := range messages {
		cloned := message
		cloned.ID = ""
		cloned.SessionID = toSessionID
		if _, err := sessions.AppendModelMessage(toSessionID, cloned); err != nil {
			return err
		}
	}
	return nil
}

func applyAgentPermissionModeOverride(parent, child permissions.Policy, mode string) permissions.Policy {
	mode = strings.TrimSpace(mode)
	if mode == "" {
		return child
	}
	switch parent.Mode {
	case permissions.ModeBypassPermissions, permissions.ModeAcceptEdits, permissions.ModeAuto:
		return child
	}
	override := child
	override.Mode = permissions.Mode(mode)
	normalized, err := permissions.SetupPolicy(override)
	if err != nil {
		return child
	}
	return normalized
}

func agentDisallowedToolRules(specs []string) []permissions.Rule {
	rules := make([]permissions.Rule, 0, len(specs))
	for _, spec := range specs {
		toolName := disallowedToolName(spec)
		if toolName == "" {
			continue
		}
		rules = append(rules, permissions.Rule{
			ToolName: toolName,
			Action:   permissions.ActionDeny,
			Source:   string(permissions.RuleSourceCommand),
		})
	}
	return rules
}

func disallowedToolName(spec string) string {
	spec = strings.TrimSpace(spec)
	if spec == "" {
		return ""
	}
	if idx := strings.Index(spec, "("); idx >= 0 {
		spec = strings.TrimSpace(spec[:idx])
	}
	return spec
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
		parentSession, ok := r.sessions.GetByID(previous.ParentSessionID)
		if !ok {
			return nil, fmt.Errorf("parent session %q not found", previous.ParentSessionID)
		}
		childPolicy, err := rebuildRecoveredSubagentPolicy(parentPolicy, previous, child, r.workspaceRootForSession(parentSession))
		if err != nil {
			return nil, err
		}
		r.engine.SetSessionPermissionPolicy(previous.ChildSessionID, childPolicy)
	}
	return r.options.AgentManager.Resume(ctx, previous.ID, agent.SpawnRequest{
		ParentSessionID:         previous.ParentSessionID,
		ParentAgentID:           previous.ParentAgentID,
		ChildSessionID:          previous.ChildSessionID,
		ChildSessionKey:         previous.ChildSessionKey,
		Label:                   label,
		Prompt:                  promptText,
		AllowedTools:            append([]string(nil), previous.AllowedTools...),
		Model:                   previous.Model,
		Effort:                  previous.Effort,
		RunInBackground:         previous.RunInBackground,
		Isolation:               previous.Isolation,
		CWD:                     previous.CWD,
		RemoteIsolationBoundary: previous.RemoteIsolationBoundary,
		PermissionMode:          string(r.PermissionPolicyForSession(previous.ChildSessionID).Mode),
		PermissionInherited:     true,
		ParentRunID:             previous.ID,
		ContinuationMode:        "resume",
		OutputFile:              previous.OutputFile,
		Run: func(ctx context.Context, runCtx agent.RunContext) (string, error) {
			return r.executeDelegatedSubagentRun(ctx, delegatedSubagentRunConfig{
				RunContext:    runCtx,
				Label:         label,
				InitialPrompt: promptText,
			})
		},
	})
}

type delegatedSubagentRunConfig struct {
	RunContext          agent.RunContext
	Label               string
	InitialPrompt       string
	ParentSessionID     string
	CloneParentMessages bool
}

func (r *Runner) executeDelegatedSubagentRun(ctx context.Context, cfg delegatedSubagentRunConfig) (string, error) {
	notify := func(message string, run agent.Run, extra map[string]any) {
		if r.options.AddNotification == nil {
			return
		}
		data := delegatedRunNotificationData(run)
		for key, value := range extra {
			data[key] = value
		}
		r.options.AddNotification(tools.Notification{
			Key:      cfg.RunContext.RunID,
			Priority: "info",
			Message:  message,
			Data:     data,
		})
	}
	fail := func(err error) (string, error) {
		run, _ := r.options.AgentManager.Get(cfg.RunContext.RunID)
		if err != nil && ctx.Err() != nil && (run.Status == agent.StatusStopped || run.Status == agent.StatusClosed) {
			return "", err
		}
		notify(fmt.Sprintf("Subagent %q failed: %v", cfg.Label, err), run, map[string]any{
			"status": string(agent.StatusFailed),
			"error":  err.Error(),
		})
		return "", err
	}

	if cfg.CloneParentMessages {
		if err := cloneSessionMessages(r.sessions, cfg.ParentSessionID, cfg.RunContext.ChildSessionID); err != nil {
			return fail(err)
		}
	}

	pendingPrompts := []string{cfg.InitialPrompt}
	lastOutput := ""
	for len(pendingPrompts) > 0 {
		prompt := strings.TrimSpace(pendingPrompts[0])
		pendingPrompts = pendingPrompts[1:]
		if prompt == "" {
			continue
		}

		msg, err := r.sessions.AppendMessage(cfg.RunContext.ChildSessionID, "user", prompt)
		if err != nil {
			return fail(err)
		}
		currentChild, ok := r.sessions.GetByID(cfg.RunContext.ChildSessionID)
		if !ok {
			return fail(fmt.Errorf("child session %q not found", cfg.RunContext.ChildSessionID))
		}
		if err := r.HandleUserMessage(ctx, currentChild, msg, nil); err != nil {
			return fail(err)
		}
		if ctx.Err() != nil {
			return "", ctx.Err()
		}
		output, err := r.latestAssistantOutput(cfg.RunContext.ChildSessionID)
		if err != nil {
			return fail(err)
		}
		lastOutput = output
		pendingPrompts = append(pendingPrompts, r.options.AgentManager.DrainControlMessages(cfg.RunContext.RunID)...)
	}

	if err := writeSubagentOutputFile(cfg.RunContext.RunID, lastOutput); err != nil {
		return fail(err)
	}
	outputFile := subagentOutputPath(cfg.RunContext.RunID)
	if err := r.options.AgentManager.SetOutputFile(cfg.RunContext.RunID, outputFile); err != nil {
		return fail(err)
	}
	if cleanupErr := r.finalizeWorktreeForSession(ctx, cfg.RunContext.ChildSessionID); cleanupErr != nil {
		return fail(cleanupErr)
	}
	run, _ := r.options.AgentManager.Get(cfg.RunContext.RunID)
	notify(fmt.Sprintf("Subagent %q completed", cfg.Label), run, map[string]any{
		"status":      string(agent.StatusCompleted),
		"output_file": outputFile,
	})
	return lastOutput, nil
}

func (r *Runner) latestAssistantOutput(sessionID string) (string, error) {
	messages, ok := r.sessions.Messages(sessionID)
	if !ok || len(messages) == 0 {
		return "", nil
	}
	for i := len(messages) - 1; i >= 0; i-- {
		if messages[i].Role == "assistant" {
			return messages[i].Content, nil
		}
	}
	return "", nil
}

func delegatedRunNotificationData(run agent.Run) map[string]any {
	data := map[string]any{
		"run_id":            run.ID,
		"child_session_id":  run.ChildSessionID,
		"child_session_key": run.ChildSessionKey,
		"status":            string(run.Status),
	}
	if run.LastAction != "" {
		data["last_action"] = string(run.LastAction)
	}
	if run.OutputFile != "" {
		data["output_file"] = run.OutputFile
	}
	if run.ErrorSummary != "" {
		data["error"] = run.ErrorSummary
	}
	return data
}

func (r *Runner) workspaceRootForSession(sess session.Session) string {
	if root := strings.TrimSpace(sess.Metadata.AgentWorktreePath); root != "" {
		return filepath.Clean(root)
	}
	if root := strings.TrimSpace(sess.Metadata.AgentCWD); root != "" {
		return filepath.Clean(root)
	}
	if r.engine == nil {
		return resolveWorkDir(sess, nil)
	}
	return resolveWorkDir(sess, r.engine.WorkspaceLoader())
}

func rewritePolicyForWorktree(policy permissions.Policy, worktreePath, parentRoot string) permissions.Policy {
	worktreePath = filepath.Clean(strings.TrimSpace(worktreePath))
	parentRoot = filepath.Clean(strings.TrimSpace(parentRoot))
	if worktreePath == "" {
		return policy
	}
	updated := policy
	roots := make([]string, 0, len(policy.WorkspaceRoots)+1)
	replaced := false
	for _, root := range policy.WorkspaceRoots {
		normalized := filepath.Clean(strings.TrimSpace(root))
		if normalized == "" {
			continue
		}
		if parentRoot != "" && normalized == parentRoot {
			roots = append(roots, worktreePath)
			replaced = true
			continue
		}
		roots = append(roots, normalized)
	}
	if !replaced {
		roots = append([]string{worktreePath}, roots...)
	}
	updated.WorkspaceRoots = roots
	return updated
}

func rebuildRecoveredSubagentPolicy(parentPolicy permissions.Policy, previous agent.Run, child session.Session, parentRoot string) (permissions.Policy, error) {
	childPolicy := parentPolicy.DeriveForSubagent()
	if worktreePath := strings.TrimSpace(child.Metadata.AgentWorktreePath); worktreePath != "" {
		childPolicy = rewritePolicyForWorktree(childPolicy, worktreePath, parentRoot)
	} else if cwd := recoveredSubagentCWD(previous, child); cwd != "" {
		if !cwdWithinWorkspaceRoots(cwd, parentPolicy.WorkspaceRoots) {
			return permissions.Policy{}, fmt.Errorf("recovered subagent cwd %q is outside inherited workspace roots", cwd)
		}
		childPolicy = restrictPolicyToWorkspaceRoot(childPolicy, cwd)
	}
	childPolicy = applyAgentPermissionModeOverride(parentPolicy, childPolicy, previous.PermissionMode)
	childPolicy.Rules = append(skillAllowedToolRules(previous.AllowedTools), childPolicy.Rules...)
	return childPolicy, nil
}

func recoveredSubagentCWD(previous agent.Run, child session.Session) string {
	if cwd := filepath.Clean(strings.TrimSpace(child.Metadata.AgentCWD)); cwd != "." {
		return cwd
	}
	if cwd := filepath.Clean(strings.TrimSpace(previous.CWD)); cwd != "." {
		return cwd
	}
	return ""
}

func restrictPolicyToWorkspaceRoot(policy permissions.Policy, root string) permissions.Policy {
	root = filepath.Clean(strings.TrimSpace(root))
	if root == "" || root == "." {
		return policy
	}
	updated := policy
	updated.WorkspaceRoots = []string{root}
	return updated
}

func (r *Runner) finalizeWorktreeForSession(ctx context.Context, sessionID string) error {
	sess, ok := r.sessions.GetByID(sessionID)
	if !ok {
		return nil
	}
	path := strings.TrimSpace(sess.Metadata.AgentWorktreePath)
	if path == "" {
		return nil
	}
	head := strings.TrimSpace(sess.Metadata.AgentWorktreeHeadCommit)
	changed := true
	if head != "" {
		var err error
		changed, err = r.options.WorktreeManager.HasChanges(ctx, path, head)
		if err != nil {
			return err
		}
	}
	if changed {
		return nil
	}
	if err := r.options.WorktreeManager.Remove(ctx, AgentWorktree{
		Path:       path,
		Branch:     strings.TrimSpace(sess.Metadata.AgentWorktreeBranch),
		HeadCommit: head,
		GitRoot:    strings.TrimSpace(sess.Metadata.AgentWorktreeGitRoot),
	}); err != nil {
		return err
	}
	return r.sessions.UpdateMetadata(sessionID, func(metadata *session.SessionMetadata) {
		metadata.AgentWorktreePath = ""
		metadata.AgentWorktreeBranch = ""
		metadata.AgentWorktreeHeadCommit = ""
		metadata.AgentWorktreeGitRoot = ""
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
	if root := strings.TrimSpace(sess.Metadata.AgentWorktreePath); root != "" {
		return root
	}
	if root := strings.TrimSpace(sess.Metadata.AgentCWD); root != "" {
		return root
	}
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
	model = strings.TrimSpace(model)
	if model != "" && r.options.ModelCatalog != nil {
		if err := r.options.ModelCatalog.ValidateModel(context.Background(), model); err != nil {
			return err
		}
	}
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

func (r *Runner) MCPInventory() MCPInventory {
	return r.engine.MCPInventory()
}

func (r *Runner) ExtensionInventory(sessionID string) ExtensionInventory {
	if r == nil || r.engine == nil {
		return ExtensionInventory{}
	}
	return r.engine.ExtensionInventory(sessionID)
}

func (r *Runner) RebuildExtensionInventory(sessionID string) ExtensionInventory {
	if r == nil || r.engine == nil {
		return ExtensionInventory{}
	}
	return r.engine.RebuildExtensionInventory(sessionID)
}

func (r *Runner) ExtensionLifecycleRecords() []tools.ExtensionLifecycleRecord {
	if r == nil || r.engine == nil {
		return nil
	}
	return r.engine.ExtensionLifecycleRecords()
}

func (r *Runner) DisableExtension(target tools.ExtensionLifecycleRecord) (ExtensionLifecycleOperationResult, error) {
	return r.engine.DisableExtension(target)
}

func (r *Runner) EnableExtension(target tools.ExtensionLifecycleRecord) (ExtensionLifecycleOperationResult, error) {
	return r.engine.EnableExtension(target)
}

func (r *Runner) ReloadExtension(ctx context.Context, target tools.ExtensionLifecycleRecord) (ExtensionLifecycleOperationResult, error) {
	return r.engine.ReloadExtension(ctx, target)
}

func (r *Runner) MarkExtensionDegraded(target tools.ExtensionLifecycleRecord, message string) (ExtensionLifecycleOperationResult, error) {
	return r.engine.MarkExtensionDegraded(target, message)
}

func (r *Runner) MarkExtensionFailed(target tools.ExtensionLifecycleRecord, message string) (ExtensionLifecycleOperationResult, error) {
	return r.engine.MarkExtensionFailed(target, message)
}

func (r *Runner) MCPServers() []MCPServerSnapshot {
	return r.engine.MCPServers()
}

func (r *Runner) ReconnectMCP(ctx context.Context, server string) (MCPServerSnapshot, error) {
	return r.engine.ReconnectMCP(ctx, server)
}

func (r *Runner) AuthenticateMCP(ctx context.Context, server string) (tools.MCPAuthStartResult, MCPServerSnapshot, error) {
	return r.engine.AuthenticateMCP(ctx, server)
}

func compactAndSortStrings(values []string) []string {
	if len(values) == 0 {
		return nil
	}
	seen := make(map[string]struct{}, len(values))
	out := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		out = append(out, value)
	}
	sort.Strings(out)
	return out
}

func defaultString(value, fallback string) string {
	if strings.TrimSpace(value) == "" {
		return fallback
	}
	return value
}

func cwdWithinWorkspaceRoots(cwd string, roots []string) bool {
	cwd = filepath.Clean(strings.TrimSpace(cwd))
	if cwd == "" || len(roots) == 0 {
		return true
	}
	for _, root := range roots {
		root = filepath.Clean(strings.TrimSpace(root))
		if root == "" {
			continue
		}
		if strings.EqualFold(cwd, root) {
			return true
		}
		rel, err := filepath.Rel(root, cwd)
		if err != nil {
			continue
		}
		if rel != "." && !strings.HasPrefix(rel, "..") && !filepath.IsAbs(rel) {
			return true
		}
	}
	return false
}

func (r *Runner) MemoryService() *memory.Service {
	return r.engine.MemoryService()
}

func (r *Runner) ApprovalManager() *approval.Manager {
	return r.engine.ApprovalManager()
}

func (r *Runner) SetReportToolProgress(report tools.ProgressFunc) {
	r.options.ReportToolProgress = report
	if r.engine != nil {
		r.engine.SetReportToolProgress(report)
	}
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
	if !r.engine.HasSessionPermissionPolicy(sessionID) {
		return permissions.Policy{}, false
	}
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
		ToolUseID:             event.ToolUseID,
		ProviderMessageID:     event.ProviderMessageID,
		ToolName:              event.ToolName,
		ToolInput:             event.ToolInput,
		ToolInputObject:       cloneAnyMap(event.ToolInputObject),
		ToolError:             event.ToolError,
		Progress:              cloneToolProgress(event.Progress),
		StructuredContent:     event.StructuredContent,
		Meta:                  cloneAnyMap(event.Meta),
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

func cloneToolProgress(input *tools.ToolProgress) *tools.ToolProgress {
	if input == nil {
		return nil
	}
	cloned := *input
	cloned.Data = cloneAnyMap(input.Data)
	return &cloned
}

func (r *Runner) ToolContractsForSession(sessionID string) []tools.Contract {
	if r == nil || r.engine == nil {
		return nil
	}
	return r.engine.ToolContractsForSession(sessionID)
}

func (r *Runner) UpsertRemoteIdentity(sessionID string, identity RemoteIdentity) (RemoteIdentity, error) {
	return r.remote.UpsertIdentity(sessionID, identity)
}

func (r *Runner) RecordRemoteHeartbeat(sessionID, connectionID string, at, deadline time.Time) (RemoteIdentity, error) {
	return r.remote.RecordHeartbeat(sessionID, connectionID, at, deadline)
}

func (r *Runner) MarkRemoteReconnecting(sessionID, connectionID string, at, deadline time.Time) (RemoteIdentity, error) {
	return r.remote.MarkReconnecting(sessionID, connectionID, at, deadline)
}

func (r *Runner) DisconnectRemote(sessionID, connectionID string, at time.Time) (RemoteIdentity, error) {
	return r.remote.Disconnect(sessionID, connectionID, at)
}

func (r *Runner) UpdateRemoteTrust(sessionID, connectionID string, state RemoteTrustState) (RemoteIdentity, error) {
	return r.remote.UpdateTrust(sessionID, connectionID, state)
}

func (r *Runner) RecordRemoteApprovalCorrelation(sessionID string, record RemoteApprovalCorrelation) (RemoteApprovalCorrelation, error) {
	return r.remote.RecordApprovalCorrelation(sessionID, record)
}

func (r *Runner) RemoteSnapshot(sessionID string) RemoteSnapshot {
	return r.RemoteSnapshotAt(sessionID, time.Now().UTC())
}

func (r *Runner) RemoteSnapshotAt(sessionID string, at time.Time) RemoteSnapshot {
	if r == nil || r.remote == nil {
		return RemoteSnapshot{SessionID: sessionID}
	}
	return r.remote.SnapshotAt(sessionID, at)
}
