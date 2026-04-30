package queryengine

import (
	"context"
	"myclaw/internal/agent"
	"myclaw/internal/approval"
	"myclaw/internal/compaction"
	"myclaw/internal/llm"
	"myclaw/internal/memory"
	"myclaw/internal/permissions"
	"myclaw/internal/prompt"
	"myclaw/internal/sandbox"
	"myclaw/internal/session"
	"myclaw/internal/tools"
	systemtools "myclaw/internal/tools/system"
	"myclaw/internal/workspace"
	"sync"
	"sync/atomic"
)

type EventSink interface {
	Emit(Event) error
}

type Event struct {
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

type Config struct {
	Sessions                   *session.Manager
	Client                     llm.Client
	WorkspaceLoader            *workspace.Loader
	ToolRegistry               *tools.Registry
	AgentManager               *agent.Manager
	UserContextProvider        UserContextProvider
	SystemContextProvider      SystemContextProvider
	DefaultSystemPrompt        []string
	CustomSystemPrompt         string
	AgentSystemPrompt          string
	CoordinatorSystemPrompt    string
	ProactiveAgentPrompt       bool
	AppendSystemPrompt         string
	OverrideSystemPrompt       string
	MainLoopModel              string
	LLMProvider                string
	Commands                   []tools.Command
	QuerySource                string
	ToolCustomSystemPrompt     string
	ToolAppendSystemPrompt     string
	Debug                      bool
	Verbose                    bool
	ThinkingConfig             map[string]any
	AgentDefinitions           tools.AgentDefinitions
	MaxBudgetUSD               float64
	IsNonInteractiveSession    bool
	RequireCanUseTool          bool
	QueryTracking              tools.QueryTracking
	ReadFileState              map[string]any
	ContentReplacementState    map[string]any
	CriticalSystemReminder     string
	PreserveToolUseResults     bool
	RenderedSystemPrompt       string
	SystemPromptInjection      string
	DisableClaudeMd            bool
	DisableGitStatus           bool
	ModelCatalog               llm.ModelCatalog
	InputProcessor             InputProcessor
	IncludePartialStreamEvents bool
	EstimatedTokenBudget       int
	MaxTurns                   int
	SnipReplay                 func(session.Message, []session.Message) *SnipReplayResult
	PostCompactCleanup         func(session.Message, []session.Message) *PostCompactCleanupResult
	SessionStartCompactHook    SessionStartCompactHook
	TranscriptPathProvider     TranscriptPathProvider
	PermissionPolicy           permissions.Policy
	Compactor                  *compaction.Service
	MemoryService              *memory.Service
	ApprovalManager            *approval.Manager
	PermissionHook             PermissionHook
	PreToolUseHook             PreToolUseHook
	PostToolUseHook            PostToolUseHook
	PostToolUseFailureHook     PostToolUseFailureHook
	PermissionUpdatePersister  PermissionUpdatePersister
	FileReadingLimits          tools.ResourceLimits
	GlobLimits                 tools.ResourceLimits
	MCPClients                 []tools.MCPConnection
	MCPResources               map[string][]tools.MCPResource
	MCPResourceReader          tools.MCPResourceReader
	MCPResourceLister          tools.MCPResourceLister
	MCPTools                   map[string]tools.MCPToolsListResult
	MCPToolCaller              tools.MCPToolCaller
	MCPContextualToolCaller    tools.MCPContextualToolCaller
	MCPPrompts                 map[string]tools.MCPPromptsListResult
	MCPSkills                  map[string][]tools.SkillCommand
	MCPPromptCaller            tools.MCPPromptCaller
	MCPNeedsAuth               map[string]tools.MCPAuthToolResult
	MCPFailures                map[string]string
	MCPOAuthStore              tools.MCPOAuthStore
	MCPAuthenticator           tools.MCPAuthenticator
	MCPReconnect               tools.MCPReconnectFunc
	DisableMCPPromptSkills     bool
	ExtensionLifecycle         []tools.ExtensionLifecycleRecord
	LSPServers                 []tools.LSPServerConfig
	LSPHandler                 tools.LSPHandler
	SkillRoots                 []string
	SkillForkExecutor          tools.SkillForkExecutor
	AgentTaskExecutor          tools.AgentTaskExecutor
	RequestPrompt              tools.RequestPromptFunc
	ReportToolProgress         tools.ProgressFunc
	AddNotification            tools.AddNotificationFunc
	HandleElicitation          tools.ElicitationFunc
	SetConversationID          tools.SetConversationIDFunc
}

type QueryEngine struct {
	nextRunID                  atomic.Uint64
	nextBoundaryID             atomic.Uint64
	sessions                   *session.Manager
	client                     llm.Client
	workspace                  *workspace.Loader
	tools                      *tools.Registry
	compactor                  *compaction.Service
	memory                     *memory.Service
	contextCache               *prompt.ContextCache
	approvals                  *approval.Manager
	permissionHook             PermissionHook
	preToolUseHook             PreToolUseHook
	postToolUseHook            PostToolUseHook
	postToolUseFailureHook     PostToolUseFailureHook
	permissionUpdatePersister  PermissionUpdatePersister
	tokenBudget                int
	maxTurns                   int
	policy                     permissions.Policy
	policyMu                   sync.RWMutex
	policies                   map[string]permissions.Policy
	toolContextMu              sync.Mutex
	toolAppStates              map[string]map[string]any
	toolDecisions              map[string]map[string]tools.ToolDecision
	fileReadingLimits          tools.ResourceLimits
	globLimits                 tools.ResourceLimits
	mcpClients                 []tools.MCPConnection
	mcpResources               map[string][]tools.MCPResource
	mcpResourceReader          tools.MCPResourceReader
	mcpResourceLister          tools.MCPResourceLister
	mcpTools                   map[string]tools.MCPToolsListResult
	mcpToolCaller              tools.MCPToolCaller
	mcpContextualToolCaller    tools.MCPContextualToolCaller
	mcpNeedsAuth               map[string]tools.MCPAuthToolResult
	mcpFailures                map[string]string
	mcpOAuthStore              tools.MCPOAuthStore
	mcpAuthenticator           tools.MCPAuthenticator
	mcpReconnect               tools.MCPReconnectFunc
	mcpPrompts                 map[string]tools.MCPPromptsListResult
	mcpSkills                  map[string][]tools.SkillCommand
	mcpPromptCaller            tools.MCPPromptCaller
	disableMCPPromptSkills     bool
	extensionLifecycle         map[string]tools.ExtensionLifecycleRecord
	lspServers                 []tools.LSPServerConfig
	lspHandler                 tools.LSPHandler
	skillRoots                 []string
	skillForkExecutor          tools.SkillForkExecutor
	agentTaskExecutor          tools.AgentTaskExecutor
	requestPrompt              tools.RequestPromptFunc
	reportToolProgress         tools.ProgressFunc
	addNotification            tools.AddNotificationFunc
	handleElicitation          tools.ElicitationFunc
	setConversationID          tools.SetConversationIDFunc
	stateMu                    sync.RWMutex
	state                      State
	cancelMu                   sync.Mutex
	cancel                     context.CancelFunc
	cancelRun                  string
	msgMu                      sync.RWMutex
	messages                   map[string][]session.Message
	inputs                     InputProcessor
	userContextProvider        UserContextProvider
	systemContextProvider      SystemContextProvider
	defaultSystemPrompt        []string
	customSystemPrompt         string
	agentSystemPrompt          string
	coordinatorSystemPrompt    string
	proactiveAgentPrompt       bool
	appendSystemPrompt         string
	overrideSystemPrompt       string
	mainLoopModel              string
	llmProvider                string
	commands                   []tools.Command
	querySource                string
	toolCustomSystemPrompt     string
	toolAppendSystemPrompt     string
	debug                      bool
	verbose                    bool
	thinkingConfig             map[string]any
	agentDefinitions           tools.AgentDefinitions
	maxBudgetUSD               float64
	isNonInteractiveSession    bool
	requireCanUseTool          bool
	queryTracking              tools.QueryTracking
	readFileState              map[string]any
	contentReplacementState    map[string]any
	criticalSystemReminder     string
	preserveToolUseResults     bool
	renderedSystemPrompt       string
	systemPromptInjection      string
	disableClaudeMd            bool
	disableGitStatus           bool
	modelCatalog               llm.ModelCatalog
	includePartialStreamEvents bool
	snipReplay                 func(session.Message, []session.Message) *SnipReplayResult
	postCompactCleanup         func(session.Message, []session.Message) *PostCompactCleanupResult
	sessionStartCompactHook    SessionStartCompactHook
	transcriptPathProvider     TranscriptPathProvider
}

func New(cfg Config) *QueryEngine {
	sessionsMgr := cfg.Sessions
	if sessionsMgr == nil {
		sessionsMgr = session.NewManager(nil)
	}
	client := cfg.Client
	if client == nil {
		client = llm.NewUnavailableClient("llm client is not configured")
	}
	workspaceLoader := cfg.WorkspaceLoader
	if workspaceLoader == nil {
		workspaceLoader = workspace.NewLoader("")
	}
	toolRegistry := cfg.ToolRegistry
	if toolRegistry == nil {
		router := sandbox.NewRouter(nil, nil)
		agentManager := cfg.AgentManager
		if agentManager == nil {
			agentManager = agent.NewManager()
		}
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
		toolRegistry.Register(tools.NewClaudeAgentTool(agentManager, nil))
		toolRegistry.Register(tools.NewAgentTaskTool(agentManager, nil))
		toolRegistry.Register(tools.NewClaudeToolSearchTool(toolRegistry))
		toolRegistry.Register(tools.NewToolSearchTool(toolRegistry))
	}
	memSvc := cfg.MemoryService
	if memSvc == nil {
		memSvc = memory.NewService()
	}
	approvalMgr := cfg.ApprovalManager
	if approvalMgr == nil {
		approvalMgr = approval.NewManager()
	}
	policy := cfg.PermissionPolicy
	if policy.Mode == "" {
		policy.Mode = permissions.ModeDangerFullAccess
	}
	inputs := cfg.InputProcessor
	if inputs == nil {
		inputs = lifecycleInputProcessor{}
	}
	userContextProvider := cfg.UserContextProvider
	if userContextProvider == nil {
		userContextProvider = defaultUserContextProvider(cfg.DisableClaudeMd)
	}
	systemContextProvider := cfg.SystemContextProvider
	if systemContextProvider == nil {
		systemContextProvider = defaultSystemContextProvider(cfg.SystemPromptInjection, cfg.DisableGitStatus)
	}
	prepareMCPConfig(&cfg, toolRegistry)
	registerConfiguredMCPTools(toolRegistry, cfg.MCPTools, cfg.MCPToolCaller, cfg.MCPContextualToolCaller)

	engine := &QueryEngine{
		sessions:                  sessionsMgr,
		client:                    client,
		workspace:                 workspaceLoader,
		tools:                     toolRegistry,
		compactor:                 cfg.Compactor,
		memory:                    memSvc,
		contextCache:              prompt.NewContextCache(),
		approvals:                 approvalMgr,
		permissionHook:            cfg.PermissionHook,
		preToolUseHook:            cfg.PreToolUseHook,
		postToolUseHook:           cfg.PostToolUseHook,
		postToolUseFailureHook:    cfg.PostToolUseFailureHook,
		permissionUpdatePersister: cfg.PermissionUpdatePersister,
		tokenBudget:               cfg.EstimatedTokenBudget,
		maxTurns:                  cfg.MaxTurns,
		policy:                    policy,
		policies:                  make(map[string]permissions.Policy),
		toolAppStates:             make(map[string]map[string]any),
		toolDecisions:             make(map[string]map[string]tools.ToolDecision),
		fileReadingLimits:         defaultFileReadingLimits(cfg.FileReadingLimits),
		globLimits:                defaultGlobLimits(cfg.GlobLimits),
		mcpClients:                append([]tools.MCPConnection(nil), cfg.MCPClients...),
		mcpResources:              cloneMCPResources(cfg.MCPResources),
		mcpResourceReader:         cfg.MCPResourceReader,
		mcpResourceLister:         cfg.MCPResourceLister,
		mcpTools:                  cloneMCPTools(cfg.MCPTools),
		mcpToolCaller:             cfg.MCPToolCaller,
		mcpContextualToolCaller:   cfg.MCPContextualToolCaller,
		mcpNeedsAuth:              cloneMCPAuthResults(cfg.MCPNeedsAuth),
		mcpFailures:               cloneStringMap(cfg.MCPFailures),
		mcpOAuthStore:             cfg.MCPOAuthStore,
		mcpAuthenticator:          cfg.MCPAuthenticator,
		mcpReconnect:              cfg.MCPReconnect,
		mcpPrompts:                cloneMCPPrompts(cfg.MCPPrompts),
		mcpSkills:                 cloneMCPSkills(cfg.MCPSkills),
		mcpPromptCaller:           cfg.MCPPromptCaller,
		disableMCPPromptSkills:    cfg.DisableMCPPromptSkills,
		extensionLifecycle:        lifecycleRecordsMap(mergeLifecycleRecords(cfg.ExtensionLifecycle, recoveredExtensionLifecycleRecords(sessionsMgr))),
		lspServers:                tools.NormalizeLSPServerConfigs(cfg.LSPServers),
		lspHandler:                cfg.LSPHandler,
		skillRoots:                append([]string(nil), cfg.SkillRoots...),
		skillForkExecutor:         cfg.SkillForkExecutor,
		agentTaskExecutor:         cfg.AgentTaskExecutor,
		requestPrompt:             cfg.RequestPrompt,
		reportToolProgress:        cfg.ReportToolProgress,
		addNotification:           cfg.AddNotification,
		handleElicitation:         cfg.HandleElicitation,
		setConversationID:         cfg.SetConversationID,
		messages:                  make(map[string][]session.Message),
		inputs:                    inputs,
		userContextProvider:       userContextProvider,
		systemContextProvider:     systemContextProvider,
		defaultSystemPrompt:       cfg.DefaultSystemPrompt,
		customSystemPrompt:        cfg.CustomSystemPrompt,
		agentSystemPrompt:         cfg.AgentSystemPrompt,
		coordinatorSystemPrompt:   cfg.CoordinatorSystemPrompt,
		proactiveAgentPrompt:      cfg.ProactiveAgentPrompt,
		appendSystemPrompt:        cfg.AppendSystemPrompt,
		overrideSystemPrompt:      cfg.OverrideSystemPrompt,
		mainLoopModel:             cfg.MainLoopModel,
		llmProvider:               cfg.LLMProvider,
		commands:                  append([]tools.Command(nil), cfg.Commands...),
		querySource:               cfg.QuerySource,
		toolCustomSystemPrompt:    cfg.ToolCustomSystemPrompt,
		toolAppendSystemPrompt:    cfg.ToolAppendSystemPrompt,
		debug:                     cfg.Debug,
		verbose:                   cfg.Verbose,
		thinkingConfig:            cloneAnyMap(cfg.ThinkingConfig),
		agentDefinitions: tools.AgentDefinitions{
			ActiveAgents:      append([]string(nil), cfg.AgentDefinitions.ActiveAgents...),
			AllowedAgentTypes: append([]string(nil), cfg.AgentDefinitions.AllowedAgentTypes...),
			Definitions:       cloneAgentDefinitions(cfg.AgentDefinitions.Definitions),
		},
		maxBudgetUSD:               cfg.MaxBudgetUSD,
		isNonInteractiveSession:    cfg.IsNonInteractiveSession,
		requireCanUseTool:          cfg.RequireCanUseTool,
		queryTracking:              cfg.QueryTracking,
		readFileState:              cloneAnyMap(cfg.ReadFileState),
		contentReplacementState:    cloneAnyMap(cfg.ContentReplacementState),
		criticalSystemReminder:     cfg.CriticalSystemReminder,
		preserveToolUseResults:     cfg.PreserveToolUseResults,
		renderedSystemPrompt:       cfg.RenderedSystemPrompt,
		systemPromptInjection:      cfg.SystemPromptInjection,
		disableClaudeMd:            cfg.DisableClaudeMd,
		disableGitStatus:           cfg.DisableGitStatus,
		modelCatalog:               cfg.ModelCatalog,
		includePartialStreamEvents: cfg.IncludePartialStreamEvents,
		snipReplay:                 cfg.SnipReplay,
		postCompactCleanup:         cfg.PostCompactCleanup,
		sessionStartCompactHook:    cfg.SessionStartCompactHook,
		transcriptPathProvider:     cfg.TranscriptPathProvider,
	}
	registerConfiguredLSPTools(engine.tools, engine, engine.lspServers)
	if processor, ok := engine.inputs.(lifecycleInputProcessor); ok && processor.resolver == nil {
		processor.resolver = engine
		engine.inputs = processor
	}
	engine.state.MaxTurns = engine.effectiveMaxTurns(session.Session{})
	for _, sess := range sessionsMgr.ListSessions() {
		memSvc.RecoverSession(sess)
	}
	engine.seedCompactBoundaryCounter()
	return engine
}

func (q *QueryEngine) setToolAppStateFunc(sessionID string) func(func(map[string]any) map[string]any) {
	return func(update func(map[string]any) map[string]any) {
		if update == nil {
			return
		}
		q.toolContextMu.Lock()
		defer q.toolContextMu.Unlock()
		previous := q.toolAppStates[sessionID]
		if previous == nil {
			previous = make(map[string]any)
		}
		next := update(previous)
		if next == nil {
			next = make(map[string]any)
		}
		q.toolAppStates[sessionID] = next
	}
}
