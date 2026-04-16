package queryengine

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"myclaw/internal/agent"
	"myclaw/internal/approval"
	"myclaw/internal/compaction"
	"myclaw/internal/llm"
	"myclaw/internal/memory"
	"myclaw/internal/model"
	"myclaw/internal/permissions"
	"myclaw/internal/prompt"
	"myclaw/internal/sandbox"
	"myclaw/internal/session"
	"myclaw/internal/tools"
	systemtools "myclaw/internal/tools/system"
	"myclaw/internal/workspace"
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

type PermissionHookRequest struct {
	Session           session.Session
	RunID             string
	ToolName          string
	ToolInput         string
	ToolInputObject   map[string]any
	ToolUseID         string
	ProviderMessageID string
	Decision          permissions.Decision
	Policy            permissions.Policy
}

type PermissionHook interface {
	CheckPermission(context.Context, PermissionHookRequest) (permissions.Decision, bool, error)
}

type PreToolUseHookRequest struct {
	Session           session.Session
	RunID             string
	ToolName          string
	ToolInput         string
	ToolInputObject   map[string]any
	ToolUseID         string
	ProviderMessageID string
	Policy            permissions.Policy
}

type PreToolUseHookResult struct {
	UpdatedInput          string
	UpdatedInputObject    map[string]any
	HasPermissionDecision bool
	PermissionDecision    permissions.Decision
	BlockingError         string
	PreventContinuation   bool
	StopReason            string
	AdditionalContexts    []string
	HookMessages          []map[string]any
	Cancelled             bool
	ExecutionError        string
}

func (r PreToolUseHookResult) UpdatedInputValue() (string, bool, error) {
	return permissions.Decision{
		UpdatedInput:       r.UpdatedInput,
		UpdatedInputObject: r.UpdatedInputObject,
	}.UpdatedInputValue()
}

type PreToolUseHook interface {
	BeforeToolUse(context.Context, PreToolUseHookRequest) (PreToolUseHookResult, bool, error)
}

type PostToolUseHookRequest struct {
	Session           session.Session
	RunID             string
	ToolName          string
	ToolInput         string
	ToolInputObject   map[string]any
	ToolUseID         string
	ProviderMessageID string
	ToolOutput        string
	Policy            permissions.Policy
}

type PostToolUseHookResult struct {
	BlockingError        string
	PreventContinuation  bool
	StopReason           string
	AdditionalContexts   []string
	UpdatedMCPToolOutput string
	HookMessages         []map[string]any
	Cancelled            bool
	ExecutionError       string
}

type PostToolUseHook interface {
	AfterToolUse(context.Context, PostToolUseHookRequest) (PostToolUseHookResult, bool, error)
}

type PostToolUseFailureHookRequest struct {
	Session           session.Session
	RunID             string
	ToolName          string
	ToolInput         string
	ToolInputObject   map[string]any
	ToolUseID         string
	ProviderMessageID string
	Error             string
	IsInterrupt       bool
	Policy            permissions.Policy
}

type PostToolUseFailureHookResult struct {
	BlockingError      string
	AdditionalContexts []string
	HookMessages       []map[string]any
	Cancelled          bool
	ExecutionError     string
}

type PostToolUseFailureHook interface {
	AfterToolUseFailure(context.Context, PostToolUseFailureHookRequest) (PostToolUseFailureHookResult, bool, error)
}

type PermissionUpdatePersister interface {
	PersistPermissionUpdates(context.Context, session.Session, []permissions.PermissionUpdate) error
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
	MCPPromptCaller            tools.MCPPromptCaller
	MCPOAuthStore              tools.MCPOAuthStore
	MCPAuthenticator           tools.MCPAuthenticator
	MCPReconnect               tools.MCPReconnectFunc
	SkillRoots                 []string
	SkillForkExecutor          tools.SkillForkExecutor
	RequestPrompt              tools.RequestPromptFunc
	ReportToolProgress         tools.ProgressFunc
	AddNotification            tools.AddNotificationFunc
	HandleElicitation          tools.ElicitationFunc
	SetConversationID          tools.SetConversationIDFunc
}

type ProcessResult struct {
	NormalizedInput string
	ShouldQuery     bool
	ResultText      string
	InputMode       string
	CommandName     string
	Messages        []ImmediateMessage
}

type ImmediateMessage struct {
	Role    string
	Content string
}

type InputProcessor interface {
	Process(context.Context, session.Session, string) (ProcessResult, error)
}

type noopInputProcessor struct{}

type InputProcessorFunc func(context.Context, session.Session, string) (ProcessResult, error)

func (f InputProcessorFunc) Process(ctx context.Context, sess session.Session, input string) (ProcessResult, error) {
	return f(ctx, sess, input)
}

func (noopInputProcessor) Process(_ context.Context, _ session.Session, input string) (ProcessResult, error) {
	normalized := strings.TrimSpace(input)
	if normalized == "" {
		return ProcessResult{}, nil
	}
	return ProcessResult{
		NormalizedInput: normalized,
		ShouldQuery:     true,
		InputMode:       "prompt",
	}, nil
}

type PermissionDenial struct {
	RunID     string
	SessionID string
	ToolName  string
	ToolInput string
	Reason    string
}

type ApprovalRequiredError struct {
	ToolName  string
	ToolInput string
	Reason    string
}

func (e *ApprovalRequiredError) Error() string {
	return fmt.Sprintf("tool %s requires approval: %s", e.ToolName, e.Reason)
}

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

type State struct {
	ActiveRunID                   string
	LastRunID                     string
	LastEvent                     string
	LastError                     string
	LastSessionID                 string
	PermissionDenials             []PermissionDenial
	LastAssistantReply            string
	LastUserInput                 string
	MessageCount                  int
	LastTurnStartedAt             time.Time
	LastTurnCompletedAt           time.Time
	LastTurnDuration              time.Duration
	StreamDeltaCount              int
	ActiveAssistantText           string
	StreamEventCount              int
	LastStreamEvent               string
	RecentStreamEvents            []string
	LastPromptTokens              int
	LastCompletionTokens          int
	LastTotalTokens               int
	TotalEstimatedTokens          int
	TokenBudget                   int
	BudgetExceeded                bool
	TurnCount                     int
	LastInputMode                 string
	LastCommandName               string
	LastImmediateMessageCount     int
	CompactBoundaryCount          int
	LastCompactBoundaryID         string
	LastModelPassCount            int
	MaxTurns                      int
	MaxTurnsExceeded              bool
	LastEstimatedContextTokens    int
	ContextWindowTokens           int
	WarningThresholdTokens        int
	ErrorThresholdTokens          int
	AutoCompactThresholdTokens    int
	BlockingThresholdTokens       int
	IsAboveWarningThreshold       bool
	IsAboveErrorThreshold         bool
	IsAboveAutoCompactThreshold   bool
	IsAtBlockingContextLimit      bool
	LastCompactionReason          string
	LastCompactionOriginalCount   int
	LastCompactionResultCount     int
	LastCompactionPhase           string
	LastCompactionReplayExecuted  bool
	LastCompactionReplayCount     int
	LastCompactionMemorySaved     bool
	LastCompactionSummaryID       string
	LastCompactionCleanupExecuted bool
	LastCompactionCleanupCount    int
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
	mcpToolCaller              tools.MCPToolCaller
	mcpContextualToolCaller    tools.MCPContextualToolCaller
	mcpOAuthStore              tools.MCPOAuthStore
	mcpAuthenticator           tools.MCPAuthenticator
	mcpReconnect               tools.MCPReconnectFunc
	mcpPrompts                 map[string]tools.MCPPromptsListResult
	mcpPromptCaller            tools.MCPPromptCaller
	skillRoots                 []string
	skillForkExecutor          tools.SkillForkExecutor
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
		client = llm.NewMockClient()
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
		inputs = noopInputProcessor{}
	}
	userContextProvider := cfg.UserContextProvider
	if userContextProvider == nil {
		userContextProvider = defaultUserContextProvider(cfg.DisableClaudeMd)
	}
	systemContextProvider := cfg.SystemContextProvider
	if systemContextProvider == nil {
		systemContextProvider = defaultSystemContextProvider(cfg.SystemPromptInjection, cfg.DisableGitStatus)
	}
	if len(cfg.MCPClients) > 0 {
		discovered, err := tools.DiscoverMCPClientTools(context.Background(), cfg.MCPClients)
		if err == nil {
			if len(discovered.Tools) > 0 {
				if cfg.MCPTools == nil {
					cfg.MCPTools = make(map[string]tools.MCPToolsListResult)
				}
				for server, result := range discovered.Tools {
					if _, exists := cfg.MCPTools[server]; !exists {
						cfg.MCPTools[server] = result
					}
				}
			}
			if len(discovered.Prompts) > 0 {
				if cfg.MCPPrompts == nil {
					cfg.MCPPrompts = make(map[string]tools.MCPPromptsListResult)
				}
				for server, result := range discovered.Prompts {
					if _, exists := cfg.MCPPrompts[server]; !exists {
						cfg.MCPPrompts[server] = result
					}
				}
			}
			if cfg.MCPToolCaller == nil {
				cfg.MCPToolCaller = discovered.Caller
			}
			if cfg.MCPContextualToolCaller == nil {
				cfg.MCPContextualToolCaller = discovered.ContextualCaller
			}
			if cfg.MCPPromptCaller == nil {
				cfg.MCPPromptCaller = discovered.PromptCaller
			}
			if len(discovered.Resources) > 0 {
				if cfg.MCPResources == nil {
					cfg.MCPResources = make(map[string][]tools.MCPResource)
				}
				for server, resources := range discovered.Resources {
					if _, exists := cfg.MCPResources[server]; !exists {
						cfg.MCPResources[server] = append([]tools.MCPResource(nil), resources...)
					}
				}
			}
			if cfg.MCPResourceReader == nil {
				cfg.MCPResourceReader = discovered.ResourceReader
			}
			if cfg.MCPResourceLister == nil {
				cfg.MCPResourceLister = discovered.ResourceLister
			}
			if cfg.MCPReconnect == nil {
				cfg.MCPReconnect = discovered.Reconnect
			}
			registerMCPAuthTools(toolRegistry, discovered.NeedsAuth)
		}
	}
	registerConfiguredMCPTools(toolRegistry, cfg.MCPTools, cfg.MCPToolCaller, cfg.MCPContextualToolCaller)

	engine := &QueryEngine{
		sessions:                  sessionsMgr,
		client:                    client,
		workspace:                 workspaceLoader,
		tools:                     toolRegistry,
		compactor:                 cfg.Compactor,
		memory:                    memSvc,
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
		mcpToolCaller:             cfg.MCPToolCaller,
		mcpContextualToolCaller:   cfg.MCPContextualToolCaller,
		mcpOAuthStore:             cfg.MCPOAuthStore,
		mcpAuthenticator:          cfg.MCPAuthenticator,
		mcpReconnect:              cfg.MCPReconnect,
		mcpPrompts:                cloneMCPPrompts(cfg.MCPPrompts),
		mcpPromptCaller:           cfg.MCPPromptCaller,
		skillRoots:                append([]string(nil), cfg.SkillRoots...),
		skillForkExecutor:         cfg.SkillForkExecutor,
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
		includePartialStreamEvents: cfg.IncludePartialStreamEvents,
		snipReplay:                 cfg.SnipReplay,
		postCompactCleanup:         cfg.PostCompactCleanup,
		sessionStartCompactHook:    cfg.SessionStartCompactHook,
		transcriptPathProvider:     cfg.TranscriptPathProvider,
	}
	engine.seedCompactBoundaryCounter()
	return engine
}

func registerConfiguredMCPTools(registry *tools.Registry, configured map[string]tools.MCPToolsListResult, caller tools.MCPToolCaller, contextualCaller tools.MCPContextualToolCaller) {
	if registry == nil || len(configured) == 0 {
		return
	}
	servers := make([]string, 0, len(configured))
	for server := range configured {
		servers = append(servers, server)
	}
	sort.Strings(servers)
	for _, server := range servers {
		if contextualCaller != nil {
			tools.RegisterDiscoveredMCPToolsWithContextualCaller(registry, server, configured[server], contextualCaller)
			continue
		}
		tools.RegisterDiscoveredMCPTools(registry, server, configured[server], caller)
	}
}

func registerMCPAuthTools(registry *tools.Registry, auth map[string]tools.MCPAuthToolResult) {
	if registry == nil || len(auth) == 0 {
		return
	}
	servers := make([]string, 0, len(auth))
	for server := range auth {
		servers = append(servers, server)
	}
	sort.Strings(servers)
	for _, server := range servers {
		result := auth[server]
		registry.Register(tools.NewMCPAuthToolFromResult(server, result))
	}
}

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
	return q.SubmitMessage(ctx, sess, msg, sink)
}

func (q *QueryEngine) SubmitMessage(ctx context.Context, sess session.Session, userMessage session.Message, sink EventSink) error {
	runID := fmt.Sprintf("run-%06d", q.nextRunID.Add(1))
	ctx, release := q.beginRun(ctx, runID, sess.ID)
	defer release()
	q.ensureMutableMessages(sess.ID)
	q.ensureUserMessageTracked(sess.ID, userMessage)
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
		Type:      "tool.result",
		Session:   sess,
		RunID:     request.RunID,
		Message:   &toolMsg,
		ToolName:  request.ToolName,
		ToolInput: request.ToolInput,
	}); err != nil {
		return err
	}
	reply, err := q.completeWithToolResult(ctx, sess, request.RunID, sink, toolMsg)
	if err != nil {
		q.emitRunError(sink, Event{Type: "run.error", Session: sess, RunID: request.RunID, Error: err.Error()})
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
		Type:            "tool.result",
		Session:         sess,
		RunID:           runID,
		Message:         &toolMsg,
		ToolName:        pending.name,
		ToolInput:       pending.input,
		ToolInputObject: cloneAnyMap(pending.inputObject),
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

func (q *QueryEngine) Messages(sessionID string) []session.Message {
	q.ensureMutableMessages(sessionID)
	q.msgMu.RLock()
	defer q.msgMu.RUnlock()
	items := q.messages[sessionID]
	return append([]session.Message(nil), items...)
}

func (q *QueryEngine) toolUseContext(ctx context.Context, sess session.Session, pending *toolCall) tools.ToolUseContext {
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
	if len(q.mcpPrompts) > 0 {
		appState["mcpPrompts"] = cloneMCPPrompts(q.mcpPrompts)
	}
	if q.mcpPromptCaller != nil {
		appState["mcpPromptCaller"] = q.mcpPromptCaller
	}
	decisions := q.toolDecisions[sess.ID]
	if decisions == nil {
		decisions = make(map[string]tools.ToolDecision)
		q.toolDecisions[sess.ID] = decisions
	}
	q.toolContextMu.Unlock()

	reportProgress := q.reportToolProgress
	if reportProgress == nil {
		reportProgress = func(tools.ToolProgress) {}
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
		return q.tools.Expose(tools.ExposeOptions{IncludeDeferred: true, Policy: q.PermissionPolicyForSession(sess.ID)})
	}

	return tools.ToolUseContext{
		AbortContext:       ctx,
		Session:            sess,
		ToolName:           pending.name,
		ToolUseID:          pending.toolUseID,
		Input:              pending.input,
		InputObject:        cloneAnyMap(pending.inputObject),
		Policy:             policy,
		AvailableTools:     q.tools.Expose(tools.ExposeOptions{IncludeDeferred: true, Policy: policy}),
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
		result, err := q.mcpReconnect(ctx, server)
		if err != nil {
			return tools.MCPReconnectResult{}, err
		}
		q.toolContextMu.Lock()
		if len(result.Resources) > 0 {
			if q.mcpResources == nil {
				q.mcpResources = make(map[string][]tools.MCPResource)
			}
			q.mcpResources[server] = append([]tools.MCPResource(nil), result.Resources...)
		}
		if len(result.Prompts.Prompts) > 0 {
			if q.mcpPrompts == nil {
				q.mcpPrompts = make(map[string]tools.MCPPromptsListResult)
			}
			q.mcpPrompts[server] = result.Prompts
		}
		q.toolContextMu.Unlock()
		q.tools.Unregister(tools.BuildMCPToolName(server, "authenticate"))
		if len(result.Tools.Tools) > 0 {
			if q.mcpContextualToolCaller != nil {
				tools.RegisterDiscoveredMCPToolsWithContextualCaller(q.tools, server, result.Tools, q.mcpContextualToolCaller)
			} else {
				tools.RegisterDiscoveredMCPTools(q.tools, server, result.Tools, q.mcpToolCaller)
			}
		}
		return result, nil
	}
}

func (q *QueryEngine) canUseToolFunc(parentCtx context.Context, sess session.Session) tools.CanUseToolFunc {
	return func(ctx context.Context, req tools.CanUseToolRequest) (permissions.Decision, error) {
		if req.ForceDecision != nil {
			return *req.ForceDecision, nil
		}
		if ctx == nil {
			ctx = parentCtx
		}
		input := strings.TrimSpace(req.Input)
		inputObject := cloneAnyMap(req.InputObject)
		if input == "" && len(inputObject) > 0 {
			encoded, err := json.Marshal(inputObject)
			if err != nil {
				return permissions.Decision{}, err
			}
			input = string(encoded)
		}
		policy := q.PermissionPolicyForSession(sess.ID)
		toolDef, ok := q.tools.InspectWithPolicy(req.ToolName, input, policy)
		if !ok {
			return permissions.Decision{
				Category: permissions.CategoryRuleDenied,
				Reason:   fmt.Sprintf("tool %q is not available under the current tool policy", strings.TrimSpace(req.ToolName)),
			}, nil
		}
		toolDecision, checked, err := q.tools.CheckPermissionsWithContext(ctx, tools.ToolUseContext{
			AbortContext:       ctx,
			Session:            sess,
			ToolName:           req.ToolName,
			Input:              input,
			InputObject:        inputObject,
			Policy:             policy,
			AvailableTools:     q.tools.Expose(tools.ExposeOptions{IncludeDeferred: true, Policy: policy}),
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
			CanUseTool:              q.canUseToolFunc(ctx, sess),
		})
		if err != nil {
			return permissions.Decision{}, err
		}
		if checked {
			if updated, ok, err := toolDecision.UpdatedInputValue(); err != nil {
				return permissions.Decision{}, err
			} else if ok {
				input = updated
				inputObject = cloneAnyMap(toolDecision.UpdatedInputObject)
				toolDef, ok = q.tools.InspectWithPolicy(req.ToolName, input, policy)
				if !ok {
					return permissions.Decision{
						Category: permissions.CategoryRuleDenied,
						Reason:   fmt.Sprintf("tool %q is not available under the current tool policy", strings.TrimSpace(req.ToolName)),
					}, nil
				}
			}
			if !toolDecision.Allowed && (toolDecision.RequiresApproval || strings.TrimSpace(toolDecision.Reason) != "") {
				if toolDecision.RequiresApproval && q.permissionHook != nil {
					observableInput, observableInputObject := q.observableToolInput(req.ToolName, input, inputObject)
					hookDecision, decided, err := q.permissionHook.CheckPermission(ctx, PermissionHookRequest{
						Session:           sess,
						ToolName:          req.ToolName,
						ToolInput:         observableInput,
						ToolInputObject:   observableInputObject,
						ToolUseID:         req.ToolUseID,
						ProviderMessageID: req.ProviderMessageID,
						Decision:          toolDecision,
						Policy:            policy,
					})
					if err != nil {
						return permissions.Decision{}, err
					}
					if decided {
						if updated, ok, err := hookDecision.UpdatedInputValue(); err != nil {
							return permissions.Decision{}, err
						} else if ok {
							input = updated
							inputObject = cloneAnyMap(hookDecision.UpdatedInputObject)
							toolDef, ok = q.tools.InspectWithPolicy(req.ToolName, input, policy)
							if !ok {
								return permissions.Decision{
									Category: permissions.CategoryRuleDenied,
									Reason:   fmt.Sprintf("tool %q is not available under the current tool policy", strings.TrimSpace(req.ToolName)),
								}, nil
							}
						}
						if hookDecision.Allowed {
							toolDecision = hookDecision
						} else {
							return hookDecision, nil
						}
					} else {
						return toolDecision, nil
					}
				} else {
					return toolDecision, nil
				}
			}
		}
		autoClassifierInput, _ := q.tools.AutoClassifierInput(req.ToolName, input)
		decision := policy.Evaluate(permissions.Request{
			ToolName:            req.ToolName,
			Command:             input,
			WorkDir:             resolveWorkDir(sess, q.workspace),
			ReadOnly:            toolDef.ReadOnly,
			Destructive:         toolDef.Destructive,
			AutoClassifierInput: autoClassifierInput,
		})
		if !decision.Allowed && decision.RequiresApproval && q.permissionHook != nil {
			observableInput, observableInputObject := q.observableToolInput(req.ToolName, input, inputObject)
			hookDecision, decided, err := q.permissionHook.CheckPermission(ctx, PermissionHookRequest{
				Session:           sess,
				ToolName:          req.ToolName,
				ToolInput:         observableInput,
				ToolInputObject:   observableInputObject,
				ToolUseID:         req.ToolUseID,
				ProviderMessageID: req.ProviderMessageID,
				Decision:          decision,
				Policy:            policy,
			})
			if err != nil {
				return permissions.Decision{}, err
			}
			if decided {
				return hookDecision, nil
			}
		}
		if decision.Allowed {
			if updated, ok, err := toolDecision.UpdatedInputValue(); err != nil {
				return permissions.Decision{}, err
			} else if ok {
				decision.UpdatedInput = updated
				decision.UpdatedInputObject = cloneAnyMap(toolDecision.UpdatedInputObject)
			}
		}
		return decision, nil
	}
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

func (q *QueryEngine) markMCPServerNeedsAuth(toolName string, err error) {
	server, ok := mcpServerFromToolName(toolName)
	if !ok {
		return
	}
	auth, ok := tools.MCPAuthToolResultFromError(server, err)
	if !ok {
		return
	}
	prefix := strings.TrimSuffix(auth.Name, "authenticate")
	for _, def := range q.tools.Definitions() {
		if strings.HasPrefix(def.Name, prefix) {
			q.tools.Unregister(def.Name)
		}
	}
	q.tools.Register(tools.NewMCPAuthToolFromResult(server, auth))
}

func mcpServerFromToolName(toolName string) (string, bool) {
	toolName = strings.TrimSpace(toolName)
	if !strings.HasPrefix(toolName, "mcp__") {
		return "", false
	}
	rest := strings.TrimPrefix(toolName, "mcp__")
	index := strings.Index(rest, "__")
	if index <= 0 {
		return "", false
	}
	return rest[:index], true
}

func (q *QueryEngine) applyToolContextModifier(sessionID string, current tools.ToolUseContext, modifier func(tools.ToolUseContext) tools.ToolUseContext) {
	if modifier == nil {
		return
	}
	next := modifier(current)
	q.toolContextMu.Lock()
	defer q.toolContextMu.Unlock()
	if next.AppState != nil {
		q.toolAppStates[sessionID] = cloneAnyMap(next.AppState)
	}
	if next.ToolDecisions != nil {
		q.toolDecisions[sessionID] = cloneToolDecisions(next.ToolDecisions)
	}
}

func defaultFileReadingLimits(limits tools.ResourceLimits) tools.ResourceLimits {
	if limits.MaxTokens == 0 {
		limits.MaxTokens = 120000
	}
	if limits.MaxSizeBytes == 0 {
		limits.MaxSizeBytes = 10 * 1024 * 1024
	}
	return limits
}

func defaultGlobLimits(limits tools.ResourceLimits) tools.ResourceLimits {
	if limits.MaxResults == 0 {
		limits.MaxResults = 10000
	}
	return limits
}

func cloneMCPResources(resources map[string][]tools.MCPResource) map[string][]tools.MCPResource {
	if resources == nil {
		return nil
	}
	out := make(map[string][]tools.MCPResource, len(resources))
	for name, items := range resources {
		out[name] = append([]tools.MCPResource(nil), items...)
	}
	return out
}

func cloneMCPPrompts(prompts map[string]tools.MCPPromptsListResult) map[string]tools.MCPPromptsListResult {
	if prompts == nil {
		return nil
	}
	out := make(map[string]tools.MCPPromptsListResult, len(prompts))
	for server, result := range prompts {
		cloned := tools.MCPPromptsListResult{
			Prompts: append([]tools.MCPPromptListItem(nil), result.Prompts...),
		}
		for i := range cloned.Prompts {
			cloned.Prompts[i].Arguments = append([]tools.MCPPromptArgument(nil), cloned.Prompts[i].Arguments...)
		}
		out[server] = cloned
	}
	return out
}

func cloneToolDecisions(decisions map[string]tools.ToolDecision) map[string]tools.ToolDecision {
	if decisions == nil {
		return nil
	}
	out := make(map[string]tools.ToolDecision, len(decisions))
	for name, decision := range decisions {
		out[name] = decision
	}
	return out
}

func (q *QueryEngine) State() State {
	q.stateMu.RLock()
	defer q.stateMu.RUnlock()

	cloned := q.state
	cloned.PermissionDenials = append([]PermissionDenial(nil), q.state.PermissionDenials...)
	return cloned
}

func (q *QueryEngine) Interrupt() {
	q.cancelMu.Lock()
	cancel := q.cancel
	q.cancelMu.Unlock()
	if cancel != nil {
		cancel()
	}
}

func (q *QueryEngine) PermissionPolicyForSession(sessionID string) permissions.Policy {
	q.policyMu.RLock()
	policy, ok := q.policies[sessionID]
	q.policyMu.RUnlock()
	if ok {
		return policy
	}
	return q.policy
}

func (q *QueryEngine) SetSessionPermissionPolicy(sessionID string, policy permissions.Policy) {
	q.policyMu.Lock()
	defer q.policyMu.Unlock()
	q.policies[sessionID] = policy
}

func (q *QueryEngine) applyUpdatedPermissions(ctx context.Context, sess session.Session, updates []permissions.PermissionUpdate) error {
	if len(updates) == 0 {
		return nil
	}
	if q.permissionUpdatePersister != nil {
		if err := q.permissionUpdatePersister.PersistPermissionUpdates(ctx, sess, updates); err != nil {
			return err
		}
	}
	q.policyMu.Lock()
	defer q.policyMu.Unlock()
	policy, ok := q.policies[sess.ID]
	if !ok {
		policy = q.policy
	}
	q.policies[sess.ID] = policy.ApplyPermissionUpdates(updates)
	return nil
}

func (q *QueryEngine) SetSessionMainLoopModelOverride(sessionID, model string) error {
	return q.sessions.UpdateMetadata(sessionID, func(metadata *session.SessionMetadata) {
		if strings.TrimSpace(metadata.InitialMainLoopModel) == "" {
			metadata.InitialMainLoopModel = parseUserSpecifiedMainLoopModel(q.mainLoopModel, q.llmProvider)
		}
		metadata.MainLoopModelOverride = strings.TrimSpace(model)
	})
}

func (q *QueryEngine) ClearSessionMainLoopModelOverride(sessionID string) error {
	return q.sessions.UpdateMetadata(sessionID, func(metadata *session.SessionMetadata) {
		metadata.MainLoopModelOverride = ""
	})
}

func (q *QueryEngine) BaseMainLoopModelForSession(sessionID string) string {
	q.ensureSessionModelMetadata(sessionID)
	sess, ok := q.sessions.GetByID(sessionID)
	if ok {
		if initial := strings.TrimSpace(sess.Metadata.InitialMainLoopModel); initial != "" {
			return initial
		}
	}
	return parseUserSpecifiedMainLoopModel(q.mainLoopModel, q.llmProvider)
}

func (q *QueryEngine) SessionMainLoopModelOverride(sessionID string) string {
	sess, ok := q.sessions.GetByID(sessionID)
	if !ok {
		return ""
	}
	return strings.TrimSpace(sess.Metadata.MainLoopModelOverride)
}

func (q *QueryEngine) ResolvedMainLoopModelForSession(sessionID string) string {
	return q.mainLoopModelForSession(sessionID)
}

func (q *QueryEngine) runModelPass(ctx context.Context, sess session.Session, userMessage session.Message, runID string, sink EventSink) (*textStreamCollector, error) {
	q.ensureMutableMessages(sess.ID)
	q.ensureSessionModelMetadata(sess.ID)
	history := q.Messages(sess.ID)
	if q.compactor != nil {
		analysis := q.compactor.Analyze(history)
		q.recordCompactionAnalysis(analysis)
		if analysis.IsAboveWarningThreshold {
			q.recordCompactionPhase("warning")
			if err := q.emit(sink, Event{
				Type:    "compact.warning",
				Session: sess,
				RunID:   runID,
			}); err != nil {
				return nil, err
			}
			micro := q.compactor.Microcompact(history)
			if micro.Changed {
				if err := q.sessions.ReplaceMessages(sess.ID, micro.Messages); err != nil {
					return nil, err
				}
				q.replaceMutableMessages(sess.ID, micro.Messages)
				history = micro.Messages
				q.recordCompactionResult(micro)
				if err := q.emit(sink, Event{
					Type:    "compact.micro",
					Session: sess,
					RunID:   runID,
				}); err != nil {
					return nil, err
				}
			}
		}
		if analysis.IsAboveErrorThreshold {
			q.recordCompactionPhase("error")
			if err := q.emit(sink, Event{
				Type:    "compact.error",
				Session: sess,
				RunID:   runID,
			}); err != nil {
				return nil, err
			}
		}
		compactOptions, err := q.sessionMemoryCompactOptions(ctx, sess, analysis)
		if err != nil {
			return nil, err
		}
		result := q.compactor.CompactWithSessionMemoryOptions(history, q.latestSummaryMemory(sess.ID), q.lastSummarizedMessageID(sess.ID), compactOptions)
		q.recordCompactionResult(result)
		if analysis.IsAboveAutoCompactThreshold && result.Changed {
			q.recordCompactionPhase("auto")
			if err := q.emit(sink, Event{
				Type:    "compact.auto",
				Session: sess,
				RunID:   runID,
			}); err != nil {
				return nil, err
			}
		}
		if analysis.IsAtBlockingLimit && !result.Changed {
			q.recordCompactionPhase("blocked")
			if err := q.emit(sink, Event{
				Type:    "compact.blocked",
				Session: sess,
				RunID:   runID,
			}); err != nil {
				return nil, err
			}
			return nil, fmt.Errorf("context window blocking limit reached")
		}
		if result.Changed {
			compactedWithBoundary := cloneSessionMessages(result.Messages)
			var boundary session.Message
			if result.BoundaryMessage != nil {
				boundary = *result.BoundaryMessage
			} else {
				boundary = q.newCompactBoundary(sess.ID)
				compactedWithBoundary = append(compactedWithBoundary, boundary)
			}
			if err := q.sessions.ReplaceMessages(sess.ID, compactedWithBoundary); err != nil {
				return nil, err
			}
			q.replaceMutableMessages(sess.ID, compactedWithBoundary)
			history = compactedWithBoundary
			q.recordCompactBoundary(boundary)
			_ = q.sessions.UpdateMetadata(sess.ID, func(metadata *session.SessionMetadata) {
				metadata.LastCompactBoundaryID = boundary.ID
				metadata.LastCompactionReason = string(result.Reason)
				metadata.LastCompactedAt = boundary.CreatedAt
				if result.SummaryMessage != nil {
					metadata.LastCompactionSummaryID = result.SummaryMessage.ID
				}
				if result.SummarizedThroughID != "" {
					metadata.LastSummarizedMessageID = result.SummarizedThroughID
				}
			})
			if err := q.emit(sink, Event{
				Type:    "compact.boundary",
				Session: sess,
				RunID:   runID,
				Message: &boundary,
			}); err != nil {
				return nil, err
			}
			if q.snipReplay != nil {
				if replay := q.snipReplay(boundary, q.Messages(sess.ID)); replay != nil && replay.Executed {
					if err := q.sessions.ReplaceMessages(sess.ID, replay.Messages); err != nil {
						return nil, err
					}
					q.replaceMutableMessages(sess.ID, replay.Messages)
					q.recordCompactionReplay(len(replay.Messages))
					if err := q.emit(sink, Event{
						Type:    "compact.replayed",
						Session: sess,
						RunID:   runID,
					}); err != nil {
						return nil, err
					}
					history = q.Messages(sess.ID)
				}
			}
			if result.SummaryMessage != nil && q.memory != nil {
				summary := *result.SummaryMessage
				if _, saved := q.memory.SaveCompactionSummary(sess, summary); saved {
					q.recordCompactionMemorySaved(summary.ID)
					_ = q.emit(sink, Event{Type: "compact.memory_saved", Session: sess, RunID: runID, Message: &summary})
					_ = q.emit(sink, Event{Type: "memory.saved", Session: sess, RunID: runID, Message: &summary})
				}
			}
			if q.postCompactCleanup != nil {
				if cleanup := q.postCompactCleanup(boundary, q.Messages(sess.ID)); cleanup != nil && cleanup.Executed {
					if err := q.sessions.ReplaceMessages(sess.ID, cleanup.Messages); err != nil {
						return nil, err
					}
					q.replaceMutableMessages(sess.ID, cleanup.Messages)
					q.recordCompactionCleanup(len(cleanup.Messages))
					if err := q.emit(sink, Event{
						Type:    "compact.cleaned",
						Session: sess,
						RunID:   runID,
					}); err != nil {
						return nil, err
					}
					history = q.Messages(sess.ID)
				}
			}
		}
	}

	workspaceContext, err := q.workspace.Load()
	if err != nil {
		return nil, err
	}
	contextInput := prompt.Build(prompt.BuildInput{
		Session:                 sess,
		History:                 history,
		UserMessage:             userMessage,
		DefaultSystemPrompt:     q.defaultSystemPrompt,
		CustomSystemPrompt:      q.customSystemPrompt,
		AgentSystemPrompt:       q.agentSystemPrompt,
		CoordinatorSystemPrompt: q.coordinatorSystemPrompt,
		ProactiveAgentPrompt:    q.proactiveAgentPrompt,
		AppendSystemPrompt:      q.appendSystemPrompt,
		OverrideSystemPrompt:    q.overrideSystemPrompt,
		UserContextLines:        q.userContextLines(sess, workspaceContext),
		SystemContextLines:      q.systemContextLines(sess, workspaceContext),
		WorkspaceContext:        workspaceContext,
		Tools: q.tools.Expose(tools.ExposeOptions{
			Policy: q.PermissionPolicyForSession(sess.ID),
		}),
		SessionMemories:    q.memoryLines(sess.ID),
		SessionMemoryItems: q.memoryItems(sess.ID),
	})
	exposedTools := q.tools.Expose(tools.ExposeOptions{
		Policy: q.PermissionPolicyForSession(sess.ID),
	})
	stream := &textStreamCollector{
		sink:           sink,
		session:        sess,
		runID:          runID,
		onDelta:        q.recordAssistantDelta,
		onMessageEnd:   q.clearActiveAssistantText,
		onStreamEvent:  q.recordStreamEvent,
		includePartial: q.includePartialStreamEvents,
	}
	if err := q.client.Stream(ctx, llm.GenerateRequest{
		Session:     sess,
		UserMessage: userMessage,
		History:     history,
		Context:     contextInput,
		Model:       q.mainLoopModelForSessionWithHistory(sess.ID, history),
		Tools:       llmToolDefinitions(exposedTools),
	}, stream); err != nil {
		return nil, err
	}
	return stream, nil
}

func llmToolDefinitions(defs []tools.Definition) []llm.ToolDefinition {
	if len(defs) == 0 {
		return nil
	}
	out := make([]llm.ToolDefinition, 0, len(defs))
	for _, def := range defs {
		if strings.TrimSpace(def.Name) == "" {
			continue
		}
		out = append(out, llm.ToolDefinition{
			Name:        def.Name,
			Description: def.Description,
			InputSchema: cloneAnyMap(def.InputSchema),
		})
	}
	return out
}

func (q *QueryEngine) memoryLines(sessionID string) []string {
	if q.memory == nil {
		return nil
	}
	items := q.memory.List(sessionID)
	lines := make([]string, 0, len(items))
	for _, item := range items {
		lines = append(lines, item.Content)
	}
	return lines
}

func (q *QueryEngine) memoryItems(sessionID string) []memory.Item {
	if q.memory == nil {
		return nil
	}
	return q.memory.List(sessionID)
}

func (q *QueryEngine) sessionMemoryCompactOptions(ctx context.Context, sess session.Session, analysis compaction.Analysis) (compaction.SessionMemoryOptions, error) {
	var hookMessages []session.Message
	if q.sessionStartCompactHook != nil {
		messages, err := q.sessionStartCompactHook.ProcessSessionStartCompact(ctx, sess)
		if err != nil {
			return compaction.SessionMemoryOptions{}, err
		}
		hookMessages = append([]session.Message(nil), messages...)
	}
	transcriptPath := ""
	if q.transcriptPathProvider != nil {
		transcriptPath = q.transcriptPathProvider(sess)
	}
	threshold := 0
	if analysis.IsAboveAutoCompactThreshold {
		threshold = analysis.AutoCompactThreshold
	}
	return compaction.SessionMemoryOptions{
		HookMessages:         hookMessages,
		TranscriptPath:       transcriptPath,
		AutoCompactThreshold: threshold,
		InvokedSkills:        q.invokedSkillsForSession(sess),
	}, nil
}

func (q *QueryEngine) invokedSkillsForSession(sess session.Session) []tools.InvokedSkillInfo {
	q.toolContextMu.Lock()
	defer q.toolContextMu.Unlock()
	return tools.GetInvokedSkillsForAgent(q.toolAppStates[sess.ID], sess.AgentID)
}

func (q *QueryEngine) latestSummaryMemory(sessionID string) string {
	if q.memory == nil {
		return ""
	}
	items := q.memory.List(sessionID)
	for i := len(items) - 1; i >= 0; i-- {
		if items[i].Type == memory.TypeSummary {
			return items[i].Content
		}
	}
	if messages, ok := q.sessions.Messages(sessionID); ok {
		for i := len(messages) - 1; i >= 0; i-- {
			message := messages[i]
			if message.Role == "summary" || (message.Role == "user" && message.IsCompactSummary) {
				if strings.TrimSpace(message.Content) != "" {
					return message.Content
				}
			}
		}
	}
	return ""
}

func (q *QueryEngine) lastSummarizedMessageID(sessionID string) string {
	sess, ok := q.sessions.GetByID(sessionID)
	if !ok {
		return ""
	}
	return strings.TrimSpace(sess.Metadata.LastSummarizedMessageID)
}

func (q *QueryEngine) ensureSessionModelMetadata(sessionID string) {
	baseModel := parseUserSpecifiedMainLoopModel(q.mainLoopModel, q.llmProvider)
	if baseModel == "" {
		return
	}
	_ = q.sessions.UpdateMetadata(sessionID, func(metadata *session.SessionMetadata) {
		if strings.TrimSpace(metadata.InitialMainLoopModel) == "" {
			metadata.InitialMainLoopModel = baseModel
		}
	})
}

func (q *QueryEngine) mainLoopModelForSession(sessionID string) string {
	history, _ := q.sessions.Messages(sessionID)
	return q.mainLoopModelForSessionWithHistory(sessionID, history)
}

func (q *QueryEngine) mainLoopModelForSessionWithHistory(sessionID string, history []session.Message) string {
	policy := q.PermissionPolicyForSession(sessionID)
	exceeds200kTokens := estimateMessagesTokens(history) > 200000
	sess, ok := q.sessions.GetByID(sessionID)
	if ok {
		if override := strings.TrimSpace(sess.Metadata.MainLoopModelOverride); override != "" {
			return resolveRuntimeMainLoopModelWithProviderAndContext(override, policy, q.llmProvider, exceeds200kTokens)
		}
	}
	return resolveRuntimeMainLoopModelWithProviderAndContext(strings.TrimSpace(q.mainLoopModel), policy, q.llmProvider, exceeds200kTokens)
}

func (q *QueryEngine) userContextLines(sess session.Session, workspaceContext workspace.Context) []string {
	if q.userContextProvider == nil {
		return nil
	}
	return q.userContextProvider.Lines(sess, workspaceContext)
}

func (q *QueryEngine) systemContextLines(sess session.Session, workspaceContext workspace.Context) []string {
	if q.systemContextProvider == nil {
		return nil
	}
	return q.systemContextProvider.Lines(sess, workspaceContext, q.PermissionPolicyForSession(sess.ID))
}

func (q *QueryEngine) findMessageByID(sessionID, messageID string) (session.Message, bool) {
	messages, ok := q.sessions.Messages(sessionID)
	if !ok {
		return session.Message{}, false
	}
	for _, message := range messages {
		if message.ID == messageID {
			return message, true
		}
	}
	return session.Message{}, false
}

func (q *QueryEngine) emit(sink EventSink, event Event) error {
	q.recordEvent(event)
	if sink == nil {
		return nil
	}
	return sink.Emit(event)
}

func (q *QueryEngine) emitRunError(sink EventSink, event Event) {
	q.recordEvent(event)
	if sink == nil {
		return
	}
	_ = sink.Emit(event)
}

func isApprovalRequiredError(err error) bool {
	var approvalErr *ApprovalRequiredError
	return errors.As(err, &approvalErr)
}

func (q *QueryEngine) beginRun(parent context.Context, runID, sessionID string) (context.Context, func()) {
	startedAt := time.Now().UTC()
	ctx, cancel := context.WithCancel(parent)
	q.cancelMu.Lock()
	q.cancel = cancel
	q.cancelRun = runID
	q.cancelMu.Unlock()

	q.ensureMutableMessages(sessionID)
	q.msgMu.RLock()
	messageCount := len(q.messages[sessionID])
	q.msgMu.RUnlock()

	q.stateMu.Lock()
	q.state.ActiveRunID = runID
	q.state.LastRunID = runID
	q.state.LastSessionID = sessionID
	q.state.MessageCount = messageCount
	q.state.LastTurnStartedAt = startedAt
	q.state.LastTurnCompletedAt = time.Time{}
	q.state.LastTurnDuration = 0
	q.state.StreamDeltaCount = 0
	q.state.ActiveAssistantText = ""
	q.state.StreamEventCount = 0
	q.state.LastStreamEvent = ""
	q.state.RecentStreamEvents = nil
	q.state.TokenBudget = q.tokenBudget
	q.state.MaxTurns = q.effectiveMaxTurns()
	q.state.MaxTurnsExceeded = false
	q.state.LastModelPassCount = 0
	q.stateMu.Unlock()

	return ctx, func() {
		q.cancelMu.Lock()
		if q.cancelRun == runID {
			q.cancel = nil
			q.cancelRun = ""
		}
		q.cancelMu.Unlock()

		q.stateMu.Lock()
		if q.state.ActiveRunID == runID {
			q.state.ActiveRunID = ""
		}
		q.state.LastTurnCompletedAt = time.Now().UTC()
		if !q.state.LastTurnStartedAt.IsZero() {
			q.state.LastTurnDuration = q.state.LastTurnCompletedAt.Sub(q.state.LastTurnStartedAt)
			if q.state.LastTurnDuration <= 0 {
				q.state.LastTurnDuration = time.Nanosecond
			}
		}
		q.stateMu.Unlock()
	}
}

func (q *QueryEngine) recordEvent(event Event) {
	q.stateMu.Lock()
	defer q.stateMu.Unlock()

	q.state.LastRunID = event.RunID
	q.state.LastEvent = event.Type
	if event.Session.ID != "" {
		q.state.LastSessionID = event.Session.ID
	}
	if event.Error != "" {
		q.state.LastError = event.Error
	}
}

func (q *QueryEngine) recordPermissionDenial(denial PermissionDenial) {
	q.stateMu.Lock()
	defer q.stateMu.Unlock()
	q.state.PermissionDenials = append(q.state.PermissionDenials, denial)
}

func (q *QueryEngine) setLastAssistantReply(content string) {
	q.stateMu.Lock()
	defer q.stateMu.Unlock()
	q.state.LastAssistantReply = content
}

func (q *QueryEngine) recordAssistantDelta(delta string) {
	q.stateMu.Lock()
	defer q.stateMu.Unlock()
	q.state.StreamDeltaCount++
	q.state.ActiveAssistantText += delta
}

func (q *QueryEngine) clearActiveAssistantText() {
	q.stateMu.Lock()
	defer q.stateMu.Unlock()
	q.state.ActiveAssistantText = ""
}

func (q *QueryEngine) recordStreamEvent(event llm.StreamEvent) {
	q.stateMu.Lock()
	defer q.stateMu.Unlock()
	q.state.StreamEventCount++
	q.state.LastStreamEvent = event.Type
	entry := event.Type
	if event.ToolName != "" {
		entry += ":" + event.ToolName
	}
	q.state.RecentStreamEvents = append(q.state.RecentStreamEvents, entry)
	if len(q.state.RecentStreamEvents) > 12 {
		q.state.RecentStreamEvents = append([]string(nil), q.state.RecentStreamEvents[len(q.state.RecentStreamEvents)-12:]...)
	}
}

func (q *QueryEngine) recordUsageEstimate(sessionID, completion string) {
	promptTokens := estimateMessagesTokens(q.Messages(sessionID))
	completionTokens := estimateTextTokens(completion)
	total := promptTokens + completionTokens

	q.stateMu.Lock()
	defer q.stateMu.Unlock()
	q.state.LastPromptTokens = promptTokens
	q.state.LastCompletionTokens = completionTokens
	q.state.LastTotalTokens = total
	q.state.TotalEstimatedTokens += total
	q.state.TurnCount++
	q.state.TokenBudget = q.tokenBudget
	q.state.BudgetExceeded = q.tokenBudget > 0 && q.state.TotalEstimatedTokens > q.tokenBudget
}

func (q *QueryEngine) recordModelPass() {
	q.stateMu.Lock()
	defer q.stateMu.Unlock()
	q.state.LastModelPassCount++
}

func (q *QueryEngine) recordMaxTurnsExceeded() {
	q.stateMu.Lock()
	defer q.stateMu.Unlock()
	q.state.MaxTurnsExceeded = true
}

func (q *QueryEngine) recordInputProcessing(result ProcessResult) {
	q.stateMu.Lock()
	defer q.stateMu.Unlock()

	q.state.LastInputMode = strings.TrimSpace(result.InputMode)
	q.state.LastCommandName = strings.TrimSpace(result.CommandName)
	q.state.LastImmediateMessageCount = len(result.Messages)
}

func (q *QueryEngine) recordCompactBoundary(boundary session.Message) {
	q.stateMu.Lock()
	defer q.stateMu.Unlock()
	q.state.CompactBoundaryCount++
	q.state.LastCompactBoundaryID = boundary.ID
}

func (q *QueryEngine) recordCompactionAnalysis(analysis compaction.Analysis) {
	q.stateMu.Lock()
	defer q.stateMu.Unlock()
	q.state.LastEstimatedContextTokens = analysis.EstimatedTokens
	q.state.ContextWindowTokens = analysis.ContextWindowTokens
	q.state.WarningThresholdTokens = analysis.WarningThreshold
	q.state.ErrorThresholdTokens = analysis.ErrorThreshold
	q.state.AutoCompactThresholdTokens = analysis.AutoCompactThreshold
	q.state.BlockingThresholdTokens = analysis.BlockingThreshold
	q.state.IsAboveWarningThreshold = analysis.IsAboveWarningThreshold
	q.state.IsAboveErrorThreshold = analysis.IsAboveErrorThreshold
	q.state.IsAboveAutoCompactThreshold = analysis.IsAboveAutoCompactThreshold
	q.state.IsAtBlockingContextLimit = analysis.IsAtBlockingLimit
}

func (q *QueryEngine) recordCompactionResult(result compaction.Result) {
	q.stateMu.Lock()
	defer q.stateMu.Unlock()
	q.state.LastCompactionReason = string(result.Reason)
	q.state.LastCompactionOriginalCount = result.OriginalCount
	q.state.LastCompactionResultCount = result.CompactedCount
}

func (q *QueryEngine) recordCompactionPhase(phase string) {
	q.stateMu.Lock()
	defer q.stateMu.Unlock()
	q.state.LastCompactionPhase = phase
}

func (q *QueryEngine) recordCompactionReplay(count int) {
	q.stateMu.Lock()
	defer q.stateMu.Unlock()
	q.state.LastCompactionReplayExecuted = true
	q.state.LastCompactionReplayCount = count
}

func (q *QueryEngine) recordCompactionMemorySaved(summaryID string) {
	q.stateMu.Lock()
	defer q.stateMu.Unlock()
	q.state.LastCompactionMemorySaved = true
	q.state.LastCompactionSummaryID = summaryID
}

func (q *QueryEngine) recordCompactionCleanup(count int) {
	q.stateMu.Lock()
	defer q.stateMu.Unlock()
	q.state.LastCompactionCleanupExecuted = true
	q.state.LastCompactionCleanupCount = count
}

func (q *QueryEngine) emitImmediateMessages(sess session.Session, items []ImmediateMessage, sink EventSink) error {
	for _, item := range items {
		reply, err := q.sessions.AppendMessage(sess.ID, fallbackRole(item.Role), item.Content)
		if err != nil {
			return err
		}
		q.appendMutableMessage(sess.ID, reply)
		if reply.Role == "assistant" {
			q.setLastAssistantReply(reply.Content)
		}
		if err := q.emit(sink, Event{
			Type:    "message.created",
			Session: sess,
			Message: &reply,
		}); err != nil {
			return err
		}
	}
	return nil
}

func (q *QueryEngine) newCompactBoundary(sessionID string) session.Message {
	id := q.nextBoundaryID.Add(1)
	return session.Message{
		ID:        fmt.Sprintf("compact-%06d", id),
		SessionID: sessionID,
		Role:      "system",
		Content:   "[compact_boundary]",
		CreatedAt: time.Now().UTC(),
	}
}

func (q *QueryEngine) seedCompactBoundaryCounter() {
	var maxID uint64
	for _, sess := range q.sessions.ListSessions() {
		messages, ok := q.sessions.Messages(sess.ID)
		if !ok {
			continue
		}
		for _, message := range messages {
			if n, ok := compactBoundaryCounter(message.ID); ok && n > maxID {
				maxID = n
			}
		}
	}
	if maxID > 0 {
		q.nextBoundaryID.Store(maxID)
	}
}

func compactBoundaryCounter(id string) (uint64, bool) {
	if !strings.HasPrefix(id, "compact-") {
		return 0, false
	}
	suffix := strings.TrimPrefix(id, "compact-")
	if suffix == "" {
		return 0, false
	}
	var n uint64
	for _, r := range suffix {
		if r < '0' || r > '9' {
			return 0, false
		}
		n = n*10 + uint64(r-'0')
	}
	return n, true
}

func (q *QueryEngine) ensureMutableMessages(sessionID string) {
	q.msgMu.RLock()
	_, ok := q.messages[sessionID]
	q.msgMu.RUnlock()
	if ok {
		return
	}
	snapshot, ok := q.sessions.RecoverySnapshot(sessionID)
	if !ok {
		q.replaceMutableMessages(sessionID, nil)
		return
	}
	q.replaceMutableMessages(sessionID, snapshot.Continuation)
	q.restoreStateFromSession(snapshot)
}

func (q *QueryEngine) ensureUserMessageTracked(sessionID string, msg session.Message) {
	q.msgMu.RLock()
	items := q.messages[sessionID]
	for _, existing := range items {
		if existing.ID == msg.ID {
			q.msgMu.RUnlock()
			return
		}
	}
	q.msgMu.RUnlock()
	q.appendMutableMessage(sessionID, msg)
}

func (q *QueryEngine) maybeInjectSkillListingAttachment(sess session.Session) error {
	if _, ok := q.tools.InspectWithPolicy("Skill", "", q.PermissionPolicyForSession(sess.ID)); !ok {
		return nil
	}
	skills := q.skillListingCommands()
	if len(skills) == 0 {
		return nil
	}
	hasExistingListing := q.hasSkillListingAttachment(sess.ID)

	q.toolContextMu.Lock()
	appState := q.toolAppStates[sess.ID]
	if appState == nil {
		appState = make(map[string]any)
	}
	sent := stringSetFromAny(appState["sentSkillListingNames"])
	if len(sent) == 0 && hasExistingListing {
		for _, skill := range skills {
			if skill.Name != "" {
				sent[skill.Name] = struct{}{}
			}
		}
		appState["sentSkillListingNames"] = stringSetKeys(sent)
		q.toolAppStates[sess.ID] = appState
		q.toolContextMu.Unlock()
		return nil
	}
	isInitial := len(sent) == 0
	newSkills := make([]tools.SkillCommand, 0, len(skills))
	for _, skill := range skills {
		if skill.Name == "" || skill.DisableModelInvocation {
			continue
		}
		if _, ok := sent[skill.Name]; ok {
			continue
		}
		newSkills = append(newSkills, skill)
		sent[skill.Name] = struct{}{}
	}
	if len(newSkills) == 0 {
		q.toolAppStates[sess.ID] = appState
		q.toolContextMu.Unlock()
		return nil
	}
	appState["sentSkillListingNames"] = stringSetKeys(sent)
	q.toolAppStates[sess.ID] = appState
	q.toolContextMu.Unlock()

	message := tools.BuildSkillListingAttachmentMessage("", sess.ID, newSkills, 0, isInitial)
	appended, err := q.sessions.AppendModelMessage(sess.ID, message)
	if err != nil {
		return err
	}
	q.appendMutableMessage(sess.ID, appended)
	return nil
}

func (q *QueryEngine) skillListingCommands() []tools.SkillCommand {
	local := tools.GetDynamicSkills()
	mcp := tools.MCPPromptSkillCommands(q.mcpPrompts)
	if len(local) == 0 {
		return mcp
	}
	if len(mcp) == 0 {
		return local
	}
	out := make([]tools.SkillCommand, 0, len(local)+len(mcp))
	seen := make(map[string]struct{}, len(local)+len(mcp))
	for _, skill := range local {
		if skill.Name == "" {
			continue
		}
		if _, ok := seen[skill.Name]; ok {
			continue
		}
		seen[skill.Name] = struct{}{}
		out = append(out, skill)
	}
	for _, skill := range mcp {
		if skill.Name == "" {
			continue
		}
		if _, ok := seen[skill.Name]; ok {
			continue
		}
		seen[skill.Name] = struct{}{}
		out = append(out, skill)
	}
	return out
}

func (q *QueryEngine) maybeInjectDynamicSkillAttachments(sess session.Session, userMessage session.Message) error {
	if _, ok := q.tools.InspectWithPolicy("Skill", "", q.PermissionPolicyForSession(sess.ID)); !ok {
		return nil
	}
	cwd := ""
	if q.workspace != nil {
		if workspaceContext, err := q.workspace.Load(); err == nil {
			cwd = workspaceContext.Root
		}
	}
	if strings.TrimSpace(cwd) == "" {
		return nil
	}
	mentioned := extractSkillMentionedFiles(userMessage.Content)
	if len(mentioned) == 0 {
		return nil
	}
	dirs := tools.DiscoverSkillDirsForPaths(mentioned, cwd)
	if len(dirs) == 0 {
		return nil
	}
	added := tools.AddSkillDirectories(dirs)
	for _, dir := range added {
		msg := tools.BuildDynamicSkillAttachmentMessage("", sess.ID, dir, cwd)
		appended, err := q.sessions.AppendModelMessage(sess.ID, msg)
		if err != nil {
			return err
		}
		q.appendMutableMessage(sess.ID, appended)
	}
	return nil
}

func extractSkillMentionedFiles(content string) []string {
	fields := strings.Fields(content)
	out := make([]string, 0)
	for _, field := range fields {
		field = strings.Trim(field, " \t\r\n.,;:!?()[]{}<>\"'")
		if strings.HasPrefix(field, "@") && len(field) > 1 {
			out = append(out, strings.TrimPrefix(field, "@"))
		}
	}
	return out
}

func (q *QueryEngine) hasSkillListingAttachment(sessionID string) bool {
	q.msgMu.RLock()
	defer q.msgMu.RUnlock()
	for _, message := range q.messages[sessionID] {
		if message.Role == "attachment" && message.Subtype == "skill_listing" {
			return true
		}
	}
	return false
}

func (q *QueryEngine) appendMutableMessage(sessionID string, msg session.Message) {
	q.msgMu.Lock()
	q.messages[sessionID] = append(q.messages[sessionID], msg)
	count := len(q.messages[sessionID])
	q.msgMu.Unlock()

	q.stateMu.Lock()
	if q.state.LastSessionID == sessionID {
		q.state.MessageCount = count
	}
	if msg.Role == "user" {
		q.state.LastUserInput = msg.Content
	}
	q.stateMu.Unlock()
}

func (q *QueryEngine) replaceMutableMessages(sessionID string, items []session.Message) {
	cloned := append([]session.Message(nil), items...)
	q.msgMu.Lock()
	q.messages[sessionID] = cloned
	count := len(cloned)
	q.msgMu.Unlock()

	q.stateMu.Lock()
	if q.state.LastSessionID == sessionID {
		q.state.MessageCount = count
	}
	q.stateMu.Unlock()
}

type textStreamCollector struct {
	sink              EventSink
	session           session.Session
	runID             string
	builder           strings.Builder
	ToolName          string
	ToolInput         string
	ToolInputObject   map[string]any
	ToolUseID         string
	ProviderMessageID string
	onDelta           func(string)
	onMessageEnd      func()
	onStreamEvent     func(llm.StreamEvent)
	includePartial    bool
}

func (c *textStreamCollector) OnEvent(event llm.StreamEvent) error {
	if c.onStreamEvent != nil {
		c.onStreamEvent(event)
	}
	if c.includePartial && c.sink != nil {
		if err := c.sink.Emit(Event{
			Type:            "stream.event",
			Session:         c.session,
			RunID:           c.runID,
			Delta:           event.Delta,
			ToolName:        event.ToolName,
			ToolInput:       normalizedToolInput(event.ToolInput, event.ToolInputObject),
			ToolInputObject: cloneAnyMap(event.ToolInputObject),
		}); err != nil {
			return err
		}
	}
	switch event.Type {
	case "text.delta":
		c.builder.WriteString(event.Delta)
		if c.onDelta != nil {
			c.onDelta(event.Delta)
		}
		if c.sink != nil {
			return c.sink.Emit(Event{
				Type:    "assistant.delta",
				Session: c.session,
				RunID:   c.runID,
				Delta:   event.Delta,
			})
		}
	case "message.end":
		if c.onMessageEnd != nil {
			c.onMessageEnd()
		}
		if c.ToolName == "" {
			if name, input, ok := parseToolCallBlock(c.builder.String()); ok {
				c.ToolName = name
				c.ToolInput = input
				c.builder.Reset()
			}
		}
		return nil
	case "tool.call":
		c.ToolName = event.ToolName
		c.ToolInput = normalizedToolInput(event.ToolInput, event.ToolInputObject)
		c.ToolInputObject = cloneAnyMap(event.ToolInputObject)
		c.ToolUseID = event.ToolUseID
		c.ProviderMessageID = event.ProviderMessageID
		return nil
	}
	return nil
}

func (c *textStreamCollector) Content() string {
	return c.builder.String()
}

func parseToolCallBlock(content string) (string, string, bool) {
	raw := strings.TrimSpace(content)
	start := strings.Index(raw, "<tool_call>")
	end := strings.Index(raw, "</tool_call>")
	if start < 0 || end < 0 || end < start {
		return "", "", false
	}
	body := strings.TrimSpace(raw[start+len("<tool_call>") : end])
	var name string
	var input string
	for _, line := range strings.Split(body, "\n") {
		line = strings.TrimSpace(line)
		switch {
		case strings.HasPrefix(line, "name:"):
			name = strings.TrimSpace(strings.TrimPrefix(line, "name:"))
		case strings.HasPrefix(line, "input:"):
			input = strings.TrimSpace(strings.TrimPrefix(line, "input:"))
		}
	}
	if name == "" || input == "" {
		return "", "", false
	}
	return name, input, true
}

func normalizedToolInput(input string, inputObject map[string]any) string {
	if input != "" || inputObject == nil {
		return input
	}
	encoded, err := json.Marshal(inputObject)
	if err != nil {
		return input
	}
	return string(encoded)
}

func normalizedToolInputObject(input string, inputObject map[string]any) map[string]any {
	if inputObject != nil {
		return cloneAnyMap(inputObject)
	}
	input = strings.TrimSpace(input)
	if input == "" {
		return nil
	}
	var parsed map[string]any
	if err := json.Unmarshal([]byte(input), &parsed); err != nil {
		return nil
	}
	if parsed == nil {
		return nil
	}
	return parsed
}

func (q *QueryEngine) observableToolInput(name, input string, inputObject map[string]any) (string, map[string]any) {
	normalizedObject := normalizedToolInputObject(input, inputObject)
	if normalizedObject == nil {
		return input, nil
	}
	observableObject := q.tools.BackfillObservableInput(name, normalizedObject)
	encoded, err := json.Marshal(observableObject)
	if err != nil {
		return input, observableObject
	}
	return string(encoded), observableObject
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

func stringSetFromAny(value any) map[string]struct{} {
	out := make(map[string]struct{})
	switch typed := value.(type) {
	case []string:
		for _, item := range typed {
			if item = strings.TrimSpace(item); item != "" {
				out[item] = struct{}{}
			}
		}
	case []any:
		for _, item := range typed {
			if text, ok := item.(string); ok {
				if text = strings.TrimSpace(text); text != "" {
					out[text] = struct{}{}
				}
			}
		}
	case map[string]bool:
		for item, enabled := range typed {
			if enabled {
				if item = strings.TrimSpace(item); item != "" {
					out[item] = struct{}{}
				}
			}
		}
	case map[string]struct{}:
		for item := range typed {
			if item = strings.TrimSpace(item); item != "" {
				out[item] = struct{}{}
			}
		}
	}
	return out
}

func stringSetKeys(set map[string]struct{}) []string {
	out := make([]string, 0, len(set))
	for key := range set {
		out = append(out, key)
	}
	sort.Strings(out)
	return out
}

func messageBlocksFromContentMaps(input []map[string]any) []model.MessageBlock {
	if len(input) == 0 {
		return nil
	}
	blocks := make([]model.MessageBlock, 0, len(input))
	for _, item := range input {
		if item == nil {
			continue
		}
		blockType, _ := item["type"].(string)
		switch model.MessageBlockType(blockType) {
		case model.MessageBlockText:
			text, _ := item["text"].(string)
			blocks = append(blocks, model.MessageBlock{
				Type: model.MessageBlockText,
				Text: text,
			})
		default:
			blocks = append(blocks, model.MessageBlock{
				Type: model.MessageBlockType(blockType),
				Raw:  cloneAnyMap(item),
			})
		}
	}
	return blocks
}

func postToolUseHookBlocks(toolName, toolUseID string, result PostToolUseHookResult) []model.MessageBlock {
	blocks := make([]model.MessageBlock, 0, len(result.HookMessages)+5)
	hookName := "PostToolUse:" + toolName
	for _, message := range result.HookMessages {
		if message == nil {
			continue
		}
		if message["type"] == "hook_blocking_error" {
			continue
		}
		blocks = append(blocks, hookMessageBlock(message, hookName, toolUseID, "PostToolUse"))
	}
	if blockingError := strings.TrimSpace(result.BlockingError); blockingError != "" {
		blocks = append(blocks, model.MessageBlock{
			Type: model.MessageBlockType("hook_blocking_error"),
			Raw: map[string]any{
				"type":          "hook_blocking_error",
				"hookName":      hookName,
				"toolUseID":     toolUseID,
				"hookEvent":     "PostToolUse",
				"blockingError": blockingError,
			},
		})
	}
	if result.Cancelled {
		blocks = append(blocks, model.MessageBlock{
			Type: model.MessageBlockType("hook_cancelled"),
			Raw: map[string]any{
				"type":      "hook_cancelled",
				"hookName":  hookName,
				"toolUseID": toolUseID,
				"hookEvent": "PostToolUse",
			},
		})
	}
	if len(result.AdditionalContexts) > 0 {
		blocks = append(blocks, model.MessageBlock{
			Type: model.MessageBlockType("hook_additional_context"),
			Raw: map[string]any{
				"type":      "hook_additional_context",
				"content":   append([]string(nil), result.AdditionalContexts...),
				"hookName":  hookName,
				"toolUseID": toolUseID,
				"hookEvent": "PostToolUse",
			},
		})
	}
	if executionError := strings.TrimSpace(result.ExecutionError); executionError != "" {
		blocks = append(blocks, model.MessageBlock{
			Type: model.MessageBlockType("hook_error_during_execution"),
			Raw: map[string]any{
				"type":      "hook_error_during_execution",
				"content":   executionError,
				"hookName":  hookName,
				"toolUseID": toolUseID,
				"hookEvent": "PostToolUse",
			},
		})
	}
	if result.PreventContinuation {
		stopReason := strings.TrimSpace(result.StopReason)
		if stopReason == "" {
			stopReason = "Execution stopped by PostToolUse hook"
		}
		blocks = append(blocks, model.MessageBlock{
			Type: model.MessageBlockType("hook_stopped_continuation"),
			Raw: map[string]any{
				"type":      "hook_stopped_continuation",
				"message":   stopReason,
				"hookName":  hookName,
				"toolUseID": toolUseID,
				"hookEvent": "PostToolUse",
			},
		})
	}
	return blocks
}

func preToolUseHookBlocks(toolName, toolUseID string, result PreToolUseHookResult) []model.MessageBlock {
	blocks := make([]model.MessageBlock, 0, len(result.HookMessages)+4)
	hookName := "PreToolUse:" + toolName
	for _, message := range result.HookMessages {
		if message == nil {
			continue
		}
		blocks = append(blocks, hookMessageBlock(message, hookName, toolUseID, "PreToolUse"))
	}
	if len(result.AdditionalContexts) > 0 {
		blocks = append(blocks, model.MessageBlock{
			Type: model.MessageBlockType("hook_additional_context"),
			Raw: map[string]any{
				"type":      "hook_additional_context",
				"content":   append([]string(nil), result.AdditionalContexts...),
				"hookName":  hookName,
				"toolUseID": toolUseID,
				"hookEvent": "PreToolUse",
			},
		})
	}
	if result.Cancelled {
		blocks = append(blocks, model.MessageBlock{
			Type: model.MessageBlockType("hook_cancelled"),
			Raw: map[string]any{
				"type":      "hook_cancelled",
				"hookName":  hookName,
				"toolUseID": toolUseID,
				"hookEvent": "PreToolUse",
			},
		})
	}
	if executionError := strings.TrimSpace(result.ExecutionError); executionError != "" {
		blocks = append(blocks, model.MessageBlock{
			Type: model.MessageBlockType("hook_error_during_execution"),
			Raw: map[string]any{
				"type":      "hook_error_during_execution",
				"content":   executionError,
				"hookName":  hookName,
				"toolUseID": toolUseID,
				"hookEvent": "PreToolUse",
			},
		})
	}
	if result.PreventContinuation {
		stopReason := strings.TrimSpace(result.StopReason)
		if stopReason == "" {
			stopReason = "Execution stopped by hook"
		}
		blocks = append(blocks, model.MessageBlock{
			Type: model.MessageBlockType("hook_stopped_continuation"),
			Raw: map[string]any{
				"type":      "hook_stopped_continuation",
				"message":   stopReason,
				"hookName":  hookName,
				"toolUseID": toolUseID,
				"hookEvent": "PreToolUse",
			},
		})
	}
	return blocks
}

func postToolUseFailureHookBlocks(toolName, toolUseID string, result PostToolUseFailureHookResult) []model.MessageBlock {
	blocks := make([]model.MessageBlock, 0, len(result.HookMessages)+4)
	hookName := "PostToolUseFailure:" + toolName
	for _, message := range result.HookMessages {
		if message == nil {
			continue
		}
		if message["type"] == "hook_blocking_error" {
			continue
		}
		blocks = append(blocks, hookMessageBlock(message, hookName, toolUseID, "PostToolUseFailure"))
	}
	if blockingError := strings.TrimSpace(result.BlockingError); blockingError != "" {
		blocks = append(blocks, model.MessageBlock{
			Type: model.MessageBlockType("hook_blocking_error"),
			Raw: map[string]any{
				"type":          "hook_blocking_error",
				"hookName":      hookName,
				"toolUseID":     toolUseID,
				"hookEvent":     "PostToolUseFailure",
				"blockingError": blockingError,
			},
		})
	}
	if len(result.AdditionalContexts) > 0 {
		blocks = append(blocks, model.MessageBlock{
			Type: model.MessageBlockType("hook_additional_context"),
			Raw: map[string]any{
				"type":      "hook_additional_context",
				"content":   append([]string(nil), result.AdditionalContexts...),
				"hookName":  hookName,
				"toolUseID": toolUseID,
				"hookEvent": "PostToolUseFailure",
			},
		})
	}
	if result.Cancelled {
		blocks = append(blocks, model.MessageBlock{
			Type: model.MessageBlockType("hook_cancelled"),
			Raw: map[string]any{
				"type":      "hook_cancelled",
				"hookName":  hookName,
				"toolUseID": toolUseID,
				"hookEvent": "PostToolUseFailure",
			},
		})
	}
	if executionError := strings.TrimSpace(result.ExecutionError); executionError != "" {
		blocks = append(blocks, model.MessageBlock{
			Type: model.MessageBlockType("hook_error_during_execution"),
			Raw: map[string]any{
				"type":      "hook_error_during_execution",
				"content":   executionError,
				"hookName":  hookName,
				"toolUseID": toolUseID,
				"hookEvent": "PostToolUseFailure",
			},
		})
	}
	return blocks
}

func hookMessageBlock(message map[string]any, hookName, toolUseID, hookEvent string) model.MessageBlock {
	raw := cloneAnyMap(message)
	if _, ok := raw["type"]; !ok {
		raw["type"] = "hook_message"
	}
	if _, ok := raw["hookName"]; !ok {
		raw["hookName"] = hookName
	}
	if _, ok := raw["toolUseID"]; !ok {
		raw["toolUseID"] = toolUseID
	}
	if _, ok := raw["hookEvent"]; !ok {
		raw["hookEvent"] = hookEvent
	}
	blockType, _ := raw["type"].(string)
	return model.MessageBlock{
		Type: model.MessageBlockType(blockType),
		Raw:  raw,
	}
}

func (q *QueryEngine) latestUserMessage(sessionID string) session.Message {
	messages := q.Messages(sessionID)
	for i := len(messages) - 1; i >= 0; i-- {
		if messages[i].Role == "user" {
			return messages[i]
		}
	}
	return session.Message{}
}

func isMCPToolDefinition(def tools.Definition) bool {
	return strings.EqualFold(strings.TrimSpace(def.Source), "mcp") || strings.HasPrefix(strings.TrimSpace(def.Name), "mcp__")
}

const rejectMessage = "The user doesn't want to proceed with this tool use. The tool use was rejected (eg. if it was a file edit, the new_string was NOT written to the file). STOP what you are doing and wait for the user to tell you how to proceed."

const rejectMessageWithReasonPrefix = "The user doesn't want to proceed with this tool use. The tool use was rejected (eg. if it was a file edit, the new_string was NOT written to the file). To tell you how to proceed, the user said:\n"

func rejectMessageWithFeedback(feedback string) string {
	if strings.TrimSpace(feedback) == "" {
		return rejectMessage
	}
	return rejectMessageWithReasonPrefix + strings.TrimSpace(feedback)
}

func estimateMessagesTokens(messages []session.Message) int {
	total := 0
	for _, msg := range messages {
		total += estimateTextTokens(msg.Content)
	}
	return total
}

func estimateTextTokens(content string) int {
	content = strings.TrimSpace(content)
	if content == "" {
		return 0
	}
	return (len(content) + 3) / 4
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

type toolCall struct {
	name              string
	input             string
	inputObject       map[string]any
	toolUseID         string
	providerMessageID string
	acceptFeedback    string
	contentBlocks     []map[string]any
	skipPermission    bool
}

func (q *QueryEngine) executeTurnLoop(ctx context.Context, sess session.Session, userMessage session.Message, runID string, sink EventSink, pending *toolCall, current *textStreamCollector) (session.Message, error) {
	var lastExecutedToolName string
	var lastExecutedToolInput string
	var lastToolMessage *session.Message
	var deferredToolExecuted bool
	var approvedToolExecuted bool
	for {
		if pending != nil && pending.name != "" {
			if pending.name == lastExecutedToolName && pending.input == lastExecutedToolInput {
				if lastToolMessage != nil {
					return q.completeWithToolResult(ctx, sess, runID, sink, *lastToolMessage)
				}
				return session.Message{}, fmt.Errorf("repeated identical tool call detected: %s %s", pending.name, pending.input)
			}
			toolDef, ok := q.tools.InspectWithPolicy(pending.name, pending.input, q.PermissionPolicyForSession(sess.ID))
			if !ok {
				return session.Message{}, fmt.Errorf("tool %q is not available under the current tool policy", strings.TrimSpace(pending.name))
			}
			if deferredToolExecuted && toolDef.ShouldDefer {
				if lastToolMessage != nil {
					return q.completeWithToolResult(ctx, sess, runID, sink, *lastToolMessage)
				}
				return session.Message{}, fmt.Errorf("repeated deferred tool call detected: %s", pending.name)
			}
			skipPolicyEvaluation := false
			toolPermissionResolved := false
			var preHookResult PreToolUseHookResult
			var preHookHandled bool
			if !pending.skipPermission && q.preToolUseHook != nil {
				observableInput, observableInputObject := q.observableToolInput(pending.name, pending.input, pending.inputObject)
				var err error
				preHookResult, preHookHandled, err = q.preToolUseHook.BeforeToolUse(ctx, PreToolUseHookRequest{
					Session:           sess,
					RunID:             runID,
					ToolName:          pending.name,
					ToolInput:         observableInput,
					ToolInputObject:   observableInputObject,
					ToolUseID:         pending.toolUseID,
					ProviderMessageID: pending.providerMessageID,
					Policy:            q.PermissionPolicyForSession(sess.ID),
				})
				if err != nil {
					return session.Message{}, err
				}
				if preHookHandled {
					if updated, ok, err := preHookResult.UpdatedInputValue(); err != nil {
						return session.Message{}, err
					} else if ok {
						pending.input = updated
						pending.inputObject = cloneAnyMap(preHookResult.UpdatedInputObject)
						toolDef, ok = q.tools.InspectWithPolicy(pending.name, pending.input, q.PermissionPolicyForSession(sess.ID))
						if !ok {
							return session.Message{}, fmt.Errorf("tool %q is not available under the current tool policy", strings.TrimSpace(pending.name))
						}
					}
					if blockingError := strings.TrimSpace(preHookResult.BlockingError); blockingError != "" {
						decision := permissions.Decision{
							Reason: blockingError,
							DecisionReason: permissions.DecisionReason{
								Type:     permissions.DecisionReasonHook,
								HookName: "PreToolUse:" + pending.name,
								Reason:   blockingError,
							},
						}
						q.recordPermissionDenial(PermissionDenial{
							RunID:     runID,
							SessionID: sess.ID,
							ToolName:  pending.name,
							ToolInput: pending.input,
							Reason:    decision.Reason,
						})
						return q.completeWithPermissionRejection(ctx, sess, runID, sink, pending, decision.Reason, decision.ContentBlocks)
					}
					if preHookResult.HasPermissionDecision {
						decision := preHookResult.PermissionDecision
						if decision.Allowed {
							if err := q.applyUpdatedPermissions(ctx, sess, decision.UpdatedPermissions); err != nil {
								return session.Message{}, err
							}
							toolPermissionResolved = true
						} else if !decision.RequiresApproval {
							q.recordPermissionDenial(PermissionDenial{
								RunID:     runID,
								SessionID: sess.ID,
								ToolName:  pending.name,
								ToolInput: pending.input,
								Reason:    decision.Reason,
							})
							return q.completeWithPermissionRejection(ctx, sess, runID, sink, pending, decision.Reason, decision.ContentBlocks)
						} else {
							q.recordPermissionDenial(PermissionDenial{
								RunID:     runID,
								SessionID: sess.ID,
								ToolName:  pending.name,
								ToolInput: pending.input,
								Reason:    decision.Reason,
							})
							req := q.approvals.CreateWithPromptMetadata(sess.ID, runID, userMessage.ID, pending.name, pending.input, pending.inputObject, pending.toolUseID, pending.providerMessageID, decision.Reason, decision.SerializedDecisionReason(), decision.AcceptFeedback, decision.ContentBlocks, string(decision.Category), decision.RuleSource)
							_ = q.sessions.UpdateMetadata(sess.ID, func(metadata *session.SessionMetadata) {
								metadata.PendingApprovalID = req.ID
								metadata.PendingApprovalStatus = string(req.Status)
								metadata.PendingApprovalToolName = req.ToolName
								metadata.PendingApprovalToolInput = req.ToolInput
								metadata.PendingApprovalToolInputObject = cloneAnyMap(req.ToolInputObject)
								metadata.PendingApprovalToolUseID = req.ToolUseID
								metadata.PendingApprovalProviderMsgID = req.ProviderMessageID
								metadata.PendingApprovalReason = req.Reason
								metadata.PendingApprovalDecisionReason = req.DecisionReason
								metadata.PendingApprovalAcceptFeedback = req.AcceptFeedback
								metadata.PendingApprovalContentBlocks = cloneAnyMaps(req.ContentBlocks)
								metadata.PendingApprovalRunID = req.RunID
								metadata.PendingApprovalUserMessageID = req.UserMessageID
								metadata.PendingApprovalCategory = req.Category
								metadata.PendingApprovalRuleSource = req.RuleSource
							})
							if err := q.emit(sink, Event{
								Type:                  "permission.required",
								Session:               sess,
								RunID:                 runID,
								ToolName:              pending.name,
								ToolInput:             pending.input,
								DecisionReason:        decision.SerializedDecisionReason(),
								DecisionReasonDetails: decision.DecisionReason.Structured(),
								AcceptFeedback:        decision.AcceptFeedback,
								ContentBlocks:         cloneAnyMaps(decision.ContentBlocks),
								Approval:              &req,
							}); err != nil {
								return session.Message{}, err
							}
							return session.Message{}, &ApprovalRequiredError{
								ToolName:  pending.name,
								ToolInput: pending.input,
								Reason:    decision.Reason,
							}
						}
					}
				}
			}
			if !pending.skipPermission {
				if !toolPermissionResolved {
					toolDecision, checked, err := q.tools.CheckPermissionsWithContext(ctx, q.toolUseContext(ctx, sess, pending))
					if err != nil {
						return session.Message{}, err
					}
					if checked {
						if updated, ok, err := toolDecision.UpdatedInputValue(); err != nil {
							return session.Message{}, err
						} else if ok {
							pending.input = updated
							pending.inputObject = cloneAnyMap(toolDecision.UpdatedInputObject)
							toolDef, ok = q.tools.InspectWithPolicy(pending.name, pending.input, q.PermissionPolicyForSession(sess.ID))
							if !ok {
								return session.Message{}, fmt.Errorf("tool %q is not available under the current tool policy", strings.TrimSpace(pending.name))
							}
						}
						if !toolDecision.Allowed && (toolDecision.RequiresApproval || strings.TrimSpace(toolDecision.Reason) != "") {
							if toolDecision.RequiresApproval && q.permissionHook != nil {
								observableInput, observableInputObject := q.observableToolInput(pending.name, pending.input, pending.inputObject)
								hookDecision, decided, err := q.permissionHook.CheckPermission(ctx, PermissionHookRequest{
									Session:           sess,
									RunID:             runID,
									ToolName:          pending.name,
									ToolInput:         observableInput,
									ToolInputObject:   observableInputObject,
									ToolUseID:         pending.toolUseID,
									ProviderMessageID: pending.providerMessageID,
									Decision:          toolDecision,
									Policy:            q.PermissionPolicyForSession(sess.ID),
								})
								if err != nil {
									return session.Message{}, err
								}
								if decided {
									if updated, ok, err := hookDecision.UpdatedInputValue(); err != nil {
										return session.Message{}, err
									} else if ok {
										pending.input = updated
										pending.inputObject = cloneAnyMap(hookDecision.UpdatedInputObject)
										toolDef, ok = q.tools.InspectWithPolicy(pending.name, pending.input, q.PermissionPolicyForSession(sess.ID))
										if !ok {
											return session.Message{}, fmt.Errorf("tool %q is not available under the current tool policy", strings.TrimSpace(pending.name))
										}
									}
									if hookDecision.Allowed {
										if err := q.applyUpdatedPermissions(ctx, sess, hookDecision.UpdatedPermissions); err != nil {
											return session.Message{}, err
										}
										toolPermissionResolved = true
									} else if !hookDecision.RequiresApproval {
										return q.completeWithPermissionRejection(ctx, sess, runID, sink, pending, hookDecision.Reason, hookDecision.ContentBlocks)
									} else {
										toolDecision = hookDecision
									}
								}
							}
							if !toolPermissionResolved {
								q.recordPermissionDenial(PermissionDenial{
									RunID:     runID,
									SessionID: sess.ID,
									ToolName:  pending.name,
									ToolInput: pending.input,
									Reason:    toolDecision.Reason,
								})
								if !toolDecision.RequiresApproval {
									return q.completeWithPermissionRejection(ctx, sess, runID, sink, pending, toolDecision.Reason, toolDecision.ContentBlocks)
								}
								req := q.approvals.CreateWithPromptMetadata(sess.ID, runID, userMessage.ID, pending.name, pending.input, pending.inputObject, pending.toolUseID, pending.providerMessageID, toolDecision.Reason, toolDecision.SerializedDecisionReason(), toolDecision.AcceptFeedback, toolDecision.ContentBlocks, string(toolDecision.Category), toolDecision.RuleSource)
								_ = q.sessions.UpdateMetadata(sess.ID, func(metadata *session.SessionMetadata) {
									metadata.PendingApprovalID = req.ID
									metadata.PendingApprovalStatus = string(req.Status)
									metadata.PendingApprovalToolName = req.ToolName
									metadata.PendingApprovalToolInput = req.ToolInput
									metadata.PendingApprovalToolInputObject = cloneAnyMap(req.ToolInputObject)
									metadata.PendingApprovalToolUseID = req.ToolUseID
									metadata.PendingApprovalProviderMsgID = req.ProviderMessageID
									metadata.PendingApprovalReason = req.Reason
									metadata.PendingApprovalDecisionReason = req.DecisionReason
									metadata.PendingApprovalAcceptFeedback = req.AcceptFeedback
									metadata.PendingApprovalContentBlocks = cloneAnyMaps(req.ContentBlocks)
									metadata.PendingApprovalRunID = req.RunID
									metadata.PendingApprovalUserMessageID = req.UserMessageID
									metadata.PendingApprovalCategory = req.Category
									metadata.PendingApprovalRuleSource = req.RuleSource
								})
								if err := q.emit(sink, Event{
									Type:                  "permission.required",
									Session:               sess,
									RunID:                 runID,
									ToolName:              pending.name,
									ToolInput:             pending.input,
									DecisionReason:        toolDecision.SerializedDecisionReason(),
									DecisionReasonDetails: toolDecision.DecisionReason.Structured(),
									AcceptFeedback:        toolDecision.AcceptFeedback,
									ContentBlocks:         cloneAnyMaps(toolDecision.ContentBlocks),
									Approval:              &req,
								}); err != nil {
									return session.Message{}, err
								}
								return session.Message{}, &ApprovalRequiredError{
									ToolName:  pending.name,
									ToolInput: pending.input,
									Reason:    toolDecision.Reason,
								}
							}
						}
					}
				}
				if !skipPolicyEvaluation {
					autoClassifierInput, _ := q.tools.AutoClassifierInput(pending.name, pending.input)
					decision := q.PermissionPolicyForSession(sess.ID).Evaluate(permissions.Request{
						ToolName:            pending.name,
						Command:             pending.input,
						WorkDir:             resolveWorkDir(sess, q.workspace),
						ReadOnly:            toolDef.ReadOnly,
						Destructive:         toolDef.Destructive,
						AutoClassifierInput: autoClassifierInput,
					})
					if !decision.Allowed {
						if decision.RequiresApproval && q.permissionHook != nil {
							observableInput, observableInputObject := q.observableToolInput(pending.name, pending.input, pending.inputObject)
							hookDecision, decided, err := q.permissionHook.CheckPermission(ctx, PermissionHookRequest{
								Session:           sess,
								RunID:             runID,
								ToolName:          pending.name,
								ToolInput:         observableInput,
								ToolInputObject:   observableInputObject,
								ToolUseID:         pending.toolUseID,
								ProviderMessageID: pending.providerMessageID,
								Decision:          decision,
								Policy:            q.PermissionPolicyForSession(sess.ID),
							})
							if err != nil {
								return session.Message{}, err
							}
							if decided {
								if updated, ok, err := hookDecision.UpdatedInputValue(); err != nil {
									return session.Message{}, err
								} else if ok {
									pending.input = updated
									pending.inputObject = cloneAnyMap(hookDecision.UpdatedInputObject)
									toolDef, ok = q.tools.InspectWithPolicy(pending.name, pending.input, q.PermissionPolicyForSession(sess.ID))
									if !ok {
										return session.Message{}, fmt.Errorf("tool %q is not available under the current tool policy", strings.TrimSpace(pending.name))
									}
								}
								if hookDecision.Allowed {
									if err := q.applyUpdatedPermissions(ctx, sess, hookDecision.UpdatedPermissions); err != nil {
										return session.Message{}, err
									}
									skipPolicyEvaluation = true
								} else if !hookDecision.RequiresApproval {
									return q.completeWithPermissionRejection(ctx, sess, runID, sink, pending, hookDecision.Reason, hookDecision.ContentBlocks)
								} else {
									decision = hookDecision
								}
							}
						}
					}
					if !decision.Allowed && !skipPolicyEvaluation {
						q.recordPermissionDenial(PermissionDenial{
							RunID:     runID,
							SessionID: sess.ID,
							ToolName:  pending.name,
							ToolInput: pending.input,
							Reason:    decision.Reason,
						})
						if !decision.RequiresApproval {
							return q.completeWithPermissionRejection(ctx, sess, runID, sink, pending, decision.Reason, decision.ContentBlocks)
						}
						req := q.approvals.CreateWithPromptMetadata(sess.ID, runID, userMessage.ID, pending.name, pending.input, pending.inputObject, pending.toolUseID, pending.providerMessageID, decision.Reason, decision.SerializedDecisionReason(), decision.AcceptFeedback, decision.ContentBlocks, string(decision.Category), decision.RuleSource)
						_ = q.sessions.UpdateMetadata(sess.ID, func(metadata *session.SessionMetadata) {
							metadata.PendingApprovalID = req.ID
							metadata.PendingApprovalStatus = string(req.Status)
							metadata.PendingApprovalToolName = req.ToolName
							metadata.PendingApprovalToolInput = req.ToolInput
							metadata.PendingApprovalToolInputObject = cloneAnyMap(req.ToolInputObject)
							metadata.PendingApprovalToolUseID = req.ToolUseID
							metadata.PendingApprovalProviderMsgID = req.ProviderMessageID
							metadata.PendingApprovalReason = req.Reason
							metadata.PendingApprovalDecisionReason = req.DecisionReason
							metadata.PendingApprovalAcceptFeedback = req.AcceptFeedback
							metadata.PendingApprovalContentBlocks = cloneAnyMaps(req.ContentBlocks)
							metadata.PendingApprovalRunID = req.RunID
							metadata.PendingApprovalUserMessageID = req.UserMessageID
							metadata.PendingApprovalCategory = req.Category
							metadata.PendingApprovalRuleSource = req.RuleSource
						})
						if err := q.emit(sink, Event{
							Type:                  "permission.required",
							Session:               sess,
							RunID:                 runID,
							ToolName:              pending.name,
							ToolInput:             pending.input,
							DecisionReason:        decision.SerializedDecisionReason(),
							DecisionReasonDetails: decision.DecisionReason.Structured(),
							AcceptFeedback:        decision.AcceptFeedback,
							ContentBlocks:         cloneAnyMaps(decision.ContentBlocks),
							Approval:              &req,
						}); err != nil {
							return session.Message{}, err
						}
						return session.Message{}, &ApprovalRequiredError{
							ToolName:  pending.name,
							ToolInput: pending.input,
							Reason:    decision.Reason,
						}
					}
				}
			}
			toolUseID := strings.TrimSpace(pending.toolUseID)
			if toolUseID == "" {
				toolUseID = fmt.Sprintf("toolu-%s-%s", runID, strings.ReplaceAll(pending.name, ".", "-"))
			}
			providerMessageID := strings.TrimSpace(pending.providerMessageID)
			if providerMessageID == "" {
				providerMessageID = "msg-" + toolUseID
			}
			observableInput, observableInputObject := q.observableToolInput(pending.name, pending.input, pending.inputObject)
			toolUseMsg, err := q.sessions.AppendMessageWithBlocks(sess.ID, "assistant", fmt.Sprintf("%s: %s", pending.name, observableInput), providerMessageID, []model.MessageBlock{
				{
					Type:        model.MessageBlockToolUse,
					ID:          toolUseID,
					Name:        pending.name,
					Input:       observableInput,
					InputObject: observableInputObject,
				},
			})
			if err != nil {
				return session.Message{}, err
			}
			q.appendMutableMessage(sess.ID, toolUseMsg)

			if err := q.emit(sink, Event{
				Type:            "tool.called",
				Session:         sess,
				RunID:           runID,
				ToolName:        pending.name,
				ToolInput:       observableInput,
				ToolInputObject: observableInputObject,
			}); err != nil {
				return session.Message{}, err
			}
			executionContext := q.toolUseContext(ctx, sess, pending)
			toolResult, err := q.tools.InvokeWithContext(ctx, executionContext)
			if err != nil {
				q.markMCPServerNeedsAuth(pending.name, err)
				errorText := strings.TrimSpace(err.Error())
				if errorText == "" {
					errorText = "tool execution failed"
				}
				failureBlocks := []model.MessageBlock{
					{
						Type:      model.MessageBlockToolResult,
						ToolUseID: toolUseID,
						Content:   errorText,
						IsError:   true,
					},
				}
				if q.postToolUseFailureHook != nil {
					failureResult, handled, hookErr := q.postToolUseFailureHook.AfterToolUseFailure(ctx, PostToolUseFailureHookRequest{
						Session:           sess,
						RunID:             runID,
						ToolName:          pending.name,
						ToolInput:         observableInput,
						ToolInputObject:   observableInputObject,
						ToolUseID:         toolUseID,
						ProviderMessageID: providerMessageID,
						Error:             errorText,
						Policy:            q.PermissionPolicyForSession(sess.ID),
					})
					if hookErr != nil {
						return session.Message{}, hookErr
					}
					if handled {
						failureBlocks = append(failureBlocks, postToolUseFailureHookBlocks(pending.name, toolUseID, failureResult)...)
					}
				}
				toolMsg, err := q.sessions.AppendMessageWithBlocks(sess.ID, "tool", fmt.Sprintf("%s: %s", pending.name, errorText), "", failureBlocks)
				if err != nil {
					return session.Message{}, err
				}
				q.appendMutableMessage(sess.ID, toolMsg)
				if err := q.emit(sink, Event{
					Type:            "tool.result",
					Session:         sess,
					RunID:           runID,
					Message:         &toolMsg,
					ToolName:        pending.name,
					ToolInput:       observableInput,
					ToolInputObject: observableInputObject,
				}); err != nil {
					return session.Message{}, err
				}
				lastToolMessage = &toolMsg
				lastExecutedToolName = pending.name
				lastExecutedToolInput = pending.input
				if toolDef.ShouldDefer {
					deferredToolExecuted = true
				}
				pending = nil
				current = nil
				continue
			}
			toolOutput := toolResult.Output
			q.applyToolContextModifier(sess.ID, executionContext, toolResult.ContextModifier)
			var postHookResult PostToolUseHookResult
			var postHookHandled bool
			if q.postToolUseHook != nil {
				postHookResult, postHookHandled, err = q.postToolUseHook.AfterToolUse(ctx, PostToolUseHookRequest{
					Session:           sess,
					RunID:             runID,
					ToolName:          pending.name,
					ToolInput:         observableInput,
					ToolInputObject:   observableInputObject,
					ToolUseID:         toolUseID,
					ProviderMessageID: providerMessageID,
					ToolOutput:        toolOutput,
					Policy:            q.PermissionPolicyForSession(sess.ID),
				})
				if err != nil {
					return session.Message{}, err
				}
				if isMCPToolDefinition(toolDef) {
					if updatedOutput := strings.TrimSpace(postHookResult.UpdatedMCPToolOutput); updatedOutput != "" {
						toolOutput = updatedOutput
					}
				}
			}
			toolResultBlocks := []model.MessageBlock{
				{
					Type:      model.MessageBlockToolResult,
					ToolUseID: toolUseID,
					Content:   toolOutput,
				},
			}
			if feedback := strings.TrimSpace(pending.acceptFeedback); feedback != "" {
				toolResultBlocks = append(toolResultBlocks, model.MessageBlock{
					Type: model.MessageBlockText,
					Text: feedback,
				})
			}
			toolResultBlocks = append(toolResultBlocks, messageBlocksFromContentMaps(pending.contentBlocks)...)
			if preHookHandled {
				toolResultBlocks = append(toolResultBlocks, preToolUseHookBlocks(pending.name, toolUseID, preHookResult)...)
			}
			if postHookHandled {
				toolResultBlocks = append(toolResultBlocks, postToolUseHookBlocks(pending.name, toolUseID, postHookResult)...)
			}
			toolMsg, err := q.sessions.AppendMessageWithBlocks(sess.ID, "tool", fmt.Sprintf("%s: %s", pending.name, toolOutput), "", toolResultBlocks)
			if err != nil {
				return session.Message{}, err
			}
			q.appendMutableMessage(sess.ID, toolMsg)
			for _, newMessage := range toolResult.NewMessages {
				if strings.TrimSpace(newMessage.SessionID) == "" {
					newMessage.SessionID = sess.ID
				}
				if newMessage.CreatedAt.IsZero() {
					newMessage.CreatedAt = time.Now().UTC()
				}
				appended, err := q.sessions.AppendModelMessage(sess.ID, newMessage)
				if err != nil {
					return session.Message{}, err
				}
				q.appendMutableMessage(sess.ID, appended)
			}
			if err := q.emit(sink, Event{
				Type:            "tool.result",
				Session:         sess,
				RunID:           runID,
				Message:         &toolMsg,
				ToolName:        pending.name,
				ToolInput:       observableInput,
				ToolInputObject: observableInputObject,
			}); err != nil {
				return session.Message{}, err
			}
			lastToolMessage = &toolMsg
			lastExecutedToolName = pending.name
			lastExecutedToolInput = pending.input
			if pending.skipPermission {
				approvedToolExecuted = true
			}
			if toolDef.ShouldDefer {
				deferredToolExecuted = true
			}
			if preHookHandled && preHookResult.PreventContinuation {
				return toolMsg, nil
			}
			if postHookHandled && postHookResult.PreventContinuation {
				return toolMsg, nil
			}
			pending = nil
			current = nil
		}

		if current == nil {
			if limit := q.effectiveMaxTurns(); limit > 0 && q.State().LastModelPassCount >= limit {
				q.recordMaxTurnsExceeded()
				return session.Message{}, fmt.Errorf("reached maximum number of turns (%d)", limit)
			}
			stream, err := q.runModelPass(ctx, sess, userMessage, runID, sink)
			if err != nil {
				return session.Message{}, err
			}
			q.recordModelPass()
			current = stream
		}

		if current.ToolName != "" {
			if approvedToolExecuted && lastToolMessage != nil {
				return q.completeWithToolResult(ctx, sess, runID, sink, *lastToolMessage)
			}
			pending = &toolCall{name: current.ToolName, input: current.ToolInput, inputObject: normalizedToolInputObject(current.ToolInput, current.ToolInputObject), toolUseID: current.ToolUseID, providerMessageID: current.ProviderMessageID}
			current = nil
			continue
		}

		reply, err := q.sessions.AppendMessage(sess.ID, "assistant", current.Content())
		if err != nil {
			return session.Message{}, err
		}
		q.appendMutableMessage(sess.ID, reply)
		q.recordUsageEstimate(sess.ID, reply.Content)
		q.setLastAssistantReply(reply.Content)
		if err := q.emit(sink, Event{
			Type:    "message.created",
			Session: sess,
			RunID:   runID,
			Message: &reply,
		}); err != nil {
			return session.Message{}, err
		}
		return reply, nil
	}
}

func (q *QueryEngine) completeWithToolResult(ctx context.Context, sess session.Session, runID string, sink EventSink, toolMsg session.Message) (session.Message, error) {
	userMessage := q.latestUserMessage(sess.ID)
	if userMessage.ID == "" {
		userMessage = toolMsg
	}
	stream, err := q.runModelPass(ctx, sess, userMessage, runID, sink)
	if err != nil {
		return session.Message{}, err
	}
	q.recordModelPass()
	return q.executeTurnLoop(ctx, sess, userMessage, runID, sink, nil, stream)
}

func (q *QueryEngine) effectiveMaxTurns() int {
	if q.maxTurns <= 0 {
		return 8
	}
	return q.maxTurns
}

func fallbackRole(role string) string {
	role = strings.TrimSpace(role)
	switch role {
	case "user", "assistant", "tool", "system", "summary":
		return role
	default:
		return "assistant"
	}
}

func cloneSessionMessages(messages []session.Message) []session.Message {
	out := make([]session.Message, len(messages))
	copy(out, messages)
	return out
}

func (q *QueryEngine) restoreStateFromSession(snapshot session.RecoverySnapshot) {
	if snapshot.Session.ID == "" {
		return
	}
	if skills := snapshot.RecoveredInvokedSkills(); len(skills) > 0 {
		q.toolContextMu.Lock()
		appState := q.toolAppStates[snapshot.Session.ID]
		if appState == nil {
			appState = make(map[string]any)
		}
		for _, skill := range skills {
			tools.AddInvokedSkill(appState, tools.InvokedSkillInfo{
				SkillName: skill.SkillName,
				SkillPath: skill.SkillPath,
				Content:   skill.Content,
				InvokedAt: skill.InvokedAt,
				AgentID:   skill.AgentID,
			})
		}
		q.toolAppStates[snapshot.Session.ID] = appState
		q.toolContextMu.Unlock()
	}

	lastUserInput := ""
	if message, ok := snapshot.LastUserMessage(); ok {
		lastUserInput = message.Content
	}
	lastAssistantReply := ""
	if message, ok := snapshot.LastAssistantMessage(); ok {
		lastAssistantReply = message.Content
	}

	q.stateMu.Lock()
	defer q.stateMu.Unlock()
	q.state.LastSessionID = snapshot.Session.ID
	q.state.MessageCount = len(snapshot.Continuation)
	if lastUserInput != "" {
		q.state.LastUserInput = lastUserInput
	}
	if lastAssistantReply != "" {
		q.state.LastAssistantReply = lastAssistantReply
	}
	if boundary, ok := snapshot.CompactBoundary(); ok {
		q.state.LastCompactBoundaryID = boundary.ID
		q.state.CompactBoundaryCount = 1
		q.state.LastCompactionPhase = "restored"
	}
	if summary, ok := snapshot.CompactionSummary(); ok {
		q.state.LastCompactionSummaryID = summary.ID
		q.state.LastCompactionPhase = "restored"
	}
	if snapshot.Metadata.LastCompactionReason != "" {
		q.state.LastCompactionReason = snapshot.Metadata.LastCompactionReason
		q.state.LastCompactionPhase = "restored"
	}
}
