package runtime

import (
	"context"
	"sync"

	"charm.land/fantasy"
	"github.com/CIPFZ/agent-builder/internal/apitypes"
	"github.com/CIPFZ/agent-builder/internal/contextmgr"
	"github.com/CIPFZ/agent-builder/internal/permission"
	"github.com/CIPFZ/agent-builder/internal/runtimeapi"
	"github.com/CIPFZ/agent-builder/internal/workbench"
)

// RuntimeService is the transport-neutral runtime boundary used by Wails and
// the local HTTP adapter.
type RuntimeService interface {
	Status(context.Context) (RuntimeStatus, error)
	RecoveryStatus(context.Context) (RuntimeRecoveryStatus, error)
	ResumeInterruptedTurn(context.Context, string, RuntimeResumeInterruptedTurnRequest) (RuntimeTurnResponse, error)
	DiscardInterruptedTurn(context.Context, string) (RuntimeTurnResponse, error)
	RetryRecoverableError(context.Context, string) (RuntimeRecoveryRetryResponse, error)
	Projects(context.Context) (RuntimeProjectsResponse, error)
	SidebarProjection(context.Context) (RuntimeSidebarProjectionResponse, error)
	OpenProject(context.Context, RuntimeOpenProjectRequest) (RuntimeOpenProjectResponse, error)
	CreateProject(context.Context, RuntimeCreateProjectRequest) (RuntimeOpenProjectResponse, error)
	RenameProject(context.Context, RuntimeRenameProjectRequest) (RuntimeOpenProjectResponse, error)
	OpenProjectInExplorer(context.Context, RuntimeProjectActionRequest) (RuntimeOpenProjectResponse, error)
	RemoveProject(context.Context, RuntimeProjectActionRequest) (RuntimeOpenProjectResponse, error)
	ProjectMemories(context.Context, string) (RuntimeMemoryListResponse, error)
	ProjectMemory(context.Context, string) (RuntimeMemoryDetailResponse, error)
	CreateProjectMemory(context.Context, RuntimeMemoryCreateRequest) (RuntimeMemoryRecord, error)
	UpdateProjectMemory(context.Context, string, RuntimeMemoryUpdateRequest) (RuntimeMemoryRecord, error)
	DisableProjectMemory(context.Context, string, RuntimeMemoryDisableRequest) (RuntimeMemoryRecord, error)
	DeleteProjectMemory(context.Context, string, RuntimeMemoryDeleteRequest) (RuntimeMemoryRecord, error)
	RefreshProjectMemoryIndex(context.Context, string) (RuntimeMemoryIndexResponse, error)
	ProjectMemoryDiagnostics(context.Context, string) (RuntimeMemoryDiagnostics, error)
	Models(context.Context) (RuntimeModelsResponse, error)
	SelectedModel(context.Context) (RuntimeSelectedModelResponse, error)
	SaveSelectedModel(context.Context, RuntimeSelectedModelRequest) (RuntimeSelectedModelResponse, error)
	ProviderCatalog(context.Context) (RuntimeProviderCatalogResponse, error)
	ConfiguredProviders(context.Context) (RuntimeConfiguredProvidersResponse, error)
	SaveConfiguredProvider(context.Context, RuntimeConfiguredProviderRequest) (RuntimeConfiguredProviderResponse, error)
	DeleteConfiguredProvider(context.Context, string) (RuntimeConfiguredProvidersResponse, error)
	DiscoverConfiguredProviderModels(context.Context, string) (RuntimeProviderModelDiscoveryResponse, error)
	TestConfiguredProvider(context.Context, string) (RuntimeProviderTestResponse, error)
	MeasureConfiguredProviderLatency(context.Context, string) (RuntimeProviderTestResponse, error)
	GetModelConfig(context.Context) (RuntimeConfigResponse, error)
	SaveModelConfig(context.Context, RuntimeModelConfig) (RuntimeConfigResponse, error)
	DiscoverModelConfig(context.Context, RuntimeModelConfig) (RuntimeModelDiscoveryResponse, error)
	VerifyModelConfig(context.Context, RuntimeModelConfig) (RuntimeModelVerifyResponse, error)
	SessionTerminals(context.Context, string) (RuntimeSessionTerminalsResponse, error)
	CreateTerminal(context.Context, RuntimeTerminalCreateRequest) (RuntimeTerminalResponse, error)
	TerminalSettings(context.Context) (RuntimeTerminalSettingsResponse, error)
	SaveTerminalSettings(context.Context, RuntimeTerminalSettings) (RuntimeTerminalSettingsResponse, error)
	WriteTerminalInput(context.Context, string, RuntimeTerminalInputRequest) (RuntimeTerminalResponse, error)
	ResizeTerminal(context.Context, string, RuntimeTerminalResizeRequest) (RuntimeTerminalResponse, error)
	SubscribeTerminalEvents(context.Context, string, ...int64) (<-chan RuntimeTerminalEvent, func())
	DeleteTerminal(context.Context, string) (RuntimeTerminalResponse, error)
	Chat(context.Context, RuntimeChatRequest) (RuntimeChatResponse, error)
	SubmitUserInput(context.Context, RuntimeUserInputRequest) (RuntimeChatResponse, error)
	UserInput(context.Context, string) (RuntimeNormalizedInput, error)
	Turn(context.Context, string) (RuntimeTurnResponse, error)
	Turns(context.Context, string) (RuntimeTurnsResponse, error)
	ReactCallchain(context.Context, string) (RuntimeReactCallchainResponse, error)
	SessionReactCallchain(context.Context, string, int) (RuntimeReactCallchainResponse, error)
	Runs(context.Context) (RuntimeRunsResponse, error)
	RunSummaries(context.Context) (RuntimeRunSummariesResponse, error)
	RunSummary(context.Context, string) (RuntimeRunSummaryResponse, error)
	RunCheckpointMarkers(context.Context, string) (RuntimeRunCheckpointMarkersResponse, error)
	RunCheckpointMarker(context.Context, string, string) (RuntimeRunCheckpointMarkerResponse, error)
	Run(context.Context, string) (RuntimeRunResponse, error)
	AcknowledgeRunCheckpoint(context.Context, string, string) (RuntimeRunResponse, error)
	DiscardRunCheckpoint(context.Context, string, string) (RuntimeRunResponse, error)
	ResumeRunCheckpoint(context.Context, string, string) (RuntimeRunResumeResponse, error)
	ToolCall(context.Context, string) (RuntimeToolCallResponse, error)
	TurnToolCalls(context.Context, string) (RuntimeToolCallsResponse, error)
	SandboxDecisions(context.Context, RuntimeSandboxDecisionListRequest) (RuntimeSandboxDecisionsResponse, error)
	SandboxDecision(context.Context, string) (RuntimeSandboxDecisionResponse, error)
	Refs(context.Context, RuntimeRefListRequest) (RuntimeRefsResponse, error)
	Ref(context.Context, string) (RuntimeRefResponse, error)
	ReadRefContent(context.Context, string) (RuntimeRefContentResponse, error)
	TurnCompactBoundaries(context.Context, string) (RuntimeCompactBoundariesResponse, error)
	SessionCompactBoundaries(context.Context, string) (RuntimeCompactBoundariesResponse, error)
	PromptAssembliesByTurn(context.Context, string) (RuntimePromptAssembliesResponse, error)
	PromptAssembliesBySession(context.Context, string, int) (RuntimePromptAssembliesResponse, error)
	ManualCompact(context.Context, RuntimeContextActionRequest) (RuntimeManualCompactResponse, error)
	ManualSnip(context.Context, RuntimeContextActionRequest) (RuntimeManualSnipResponse, error)
	Hooks(context.Context) (RuntimeHooksResponse, error)
	HookExecutions(context.Context, RuntimeHookExecutionsRequest) (RuntimeHookExecutionsResponse, error)
	HookExecution(context.Context, string) (RuntimeHookExecutionResponse, error)
	Worktrees(context.Context) (RuntimeWorktreesResponse, error)
	Worktree(context.Context, string) (RuntimeWorktreeResponse, error)
	CreateWorktree(context.Context, RuntimeWorktreeCreateRequest) (RuntimeWorktreeResponse, error)
	EnterWorktree(context.Context, string, RuntimeWorktreeActionRequest) (RuntimeWorktreeResponse, error)
	ExitWorktree(context.Context, string, RuntimeWorktreeActionRequest) (RuntimeWorktreeResponse, error)
	CleanupWorktree(context.Context, string, RuntimeWorktreeActionRequest) (RuntimeWorktreeResponse, error)
	AgentTask(context.Context, string) (RuntimeAgentTaskResponse, error)
	SessionAgentTasks(context.Context, string) (RuntimeAgentTasksResponse, error)
	TaskEffectiveScope(context.Context, string) (RuntimeEffectiveScopeResponse, error)
	TurnAgentTasks(context.Context, string) (RuntimeAgentTasksResponse, error)
	CancelAgentTask(context.Context, string) (RuntimeAgentTaskResponse, error)
	AgentRoles(context.Context) (RuntimeAgentRolesResponse, error)
	AgentRole(context.Context, string) (RuntimeAgentRoleResponse, error)
	AgentTaskMessages(context.Context, string) (RuntimeAgentTaskMessagesResponse, error)
	CreateAgentTaskMessage(context.Context, string, RuntimeAgentTaskMessageCreateRequest) (RuntimeAgentTaskMessageResponse, error)
	SendAgentTaskFollowUp(context.Context, string, RuntimeAgentTaskMessageCreateRequest) (RuntimeAgentTaskMessageResponse, error)
	AgentTaskResult(context.Context, string) (RuntimeAgentTaskResultResponse, error)
	AgentTaskOutput(context.Context, string) (RuntimeAgentTaskOutputResponse, error)
	SessionTodos(context.Context, string) (RuntimeTodosResponse, error)
	TurnTodos(context.Context, string) (RuntimeTodosResponse, error)
	Sessions(context.Context) (RuntimeSessionsResponse, error)
	Session(context.Context, string) (RuntimeSessionResponse, error)
	CreateSession(context.Context, RuntimeSessionCreateRequest) (RuntimeSessionResponse, error)
	SelectSession(context.Context, string) (RuntimeStatus, error)
	RenameSession(context.Context, RuntimeSessionUpdateRequest) (RuntimeSessionsResponse, error)
	DeleteSession(context.Context, string) (RuntimeSessionsResponse, error)
	SessionMessages(context.Context, string) (RuntimeMessagesResponse, error)
	SessionContextUsage(context.Context, string) (RuntimeContextUsage, error)
	SessionOutput(context.Context, string, RuntimeOutputRequest) (RuntimeOutputSnapshot, error)
	SessionOutputEvents(context.Context, string, string) (RuntimeOutputEventsResponse, error)
	SubscribeSessionOutputEvents(context.Context, string, string) (<-chan RuntimeOutputEvent, func())
	SessionActivity(context.Context, string) (RuntimeSessionActivityResponse, error)
	SessionActivityWindow(context.Context, string, int) (RuntimeSessionActivityWindowResponse, error)
	SessionActivityCursorWindow(context.Context, string, string, int) (RuntimeSessionActivityWindowResponse, error)
	TurnActivity(context.Context, string) (RuntimeTurnActivityResponse, error)
	RunProjection(context.Context, RuntimeRunProjectionRequest) (RuntimeRunProjectionResponse, error)
	RunTransitionHistory(context.Context, RuntimeRunTransitionHistoryRequest) (RuntimeRunTransitionHistoryResponse, error)
	RunSchedulerPlan(context.Context, RuntimeRunSchedulerPlanRequest) (RuntimeRunSchedulerPlanResponse, error)
	ExecuteRunTask(context.Context, string, string) (RuntimeRunSchedulerExecuteTaskResponse, error)
	Messages(context.Context) (RuntimeMessagesResponse, error)
	Permissions(context.Context) (RuntimePermissionsResponse, error)
	GetPolicy(context.Context) (RuntimePolicyResponse, error)
	UpdatePolicy(context.Context, RuntimePolicyUpdateRequest) (RuntimePolicyResponse, error)
	ContextGovernanceSettings(context.Context) (RuntimeContextGovernanceSettingsResponse, error)
	SaveContextGovernanceSettings(context.Context, RuntimeContextGovernanceSettings) (RuntimeContextGovernanceSettingsResponse, error)
	Events(context.Context, ...int64) (RuntimeEventsResponse, error)
	EventsEndpoint(context.Context) (RuntimeEventsEndpointResponse, error)
	SubscribeEvents(context.Context, ...int64) (<-chan RuntimeEvent, func())
	AuditTurn(context.Context, string) (RuntimeAuditResponse, error)
	AuditSession(context.Context, string) (RuntimeAuditResponse, error)
	ReplayExport(context.Context, RuntimeReplayExportRequest) (RuntimeReplayExportResponse, error)
	Skills(context.Context) (RuntimeSkillsResponse, error)
	Plugins(context.Context) (RuntimePluginsResponse, error)
	RefreshSkills(context.Context) (RuntimeSkillsResponse, error)
	CreateSkill(context.Context, RuntimeSkillCreateRequest) (RuntimeSkillsResponse, error)
	AddSkillPath(context.Context, RuntimeSkillPathRequest) (RuntimeSkillsResponse, error)
	SetSkillEnabled(context.Context, RuntimeSkillToggleRequest) (RuntimeSkillsResponse, error)
	MCPServers(context.Context) (RuntimeMCPServersResponse, error)
	SaveMCPServer(context.Context, RuntimeMCPServerConfigRequest) (RuntimeMCPServersResponse, error)
	SetMCPServerEnabled(context.Context, RuntimeMCPServerToggleRequest) (RuntimeMCPServersResponse, error)
	RefreshMCPServer(context.Context, string) (RuntimeMCPServersResponse, error)
	SetMCPToolEnabled(context.Context, RuntimeMCPToolToggleRequest) (RuntimeMCPToolsResponse, error)
	MCPTools(context.Context, string) (RuntimeMCPToolsResponse, error)
	MCPResources(context.Context, string) (RuntimeMCPResourcesResponse, error)
	MCPPrompts(context.Context, string) (RuntimeMCPPromptsResponse, error)
	MCPRequests(context.Context, RuntimeMCPRequestListRequest) (RuntimeMCPRequestsResponse, error)
	MCPRequest(context.Context, string) (RuntimeMCPRequestResponse, error)
	DecideMCPRequest(context.Context, RuntimeMCPRequestDecision) (RuntimeMCPRequestResponse, error)
	RetryMCPServer(context.Context, string) (RuntimeMCPServersResponse, error)
	Capabilities(context.Context) (RuntimeCapabilitiesResponse, error)
	RefreshCapability(context.Context, string) (RuntimeCapabilityResponse, error)
	SearchTools(context.Context, RuntimeToolSearchRequest) (RuntimeToolSearchResponse, error)
	ContextSources(context.Context) (RuntimeContextSourcesResponse, error)
	ReadFiles(context.Context, string) (RuntimeReadFilesResponse, error)
	APIEndpoint(context.Context) (RuntimeAPIEndpointResponse, error)
	ServeHTTP(context.Context, string, string) (RuntimeAPIEndpointResponse, error)
	DecidePermission(context.Context, RuntimePermissionDecision) (RuntimeStatus, error)
	Cancel(context.Context) (RuntimeStatus, error)
	CancelTurn(context.Context, string) (RuntimeStatus, error)
	MarkInterruptedDone(context.Context, string) (RuntimeTurnResponse, error)
	NewChat(context.Context, string) (RuntimeStatus, error)
}

// runtimeService owns workspace, session, and agent lifecycle.
type runtimeService struct {
	mu                   sync.Mutex
	startMu              sync.Mutex
	starting             bool
	runtime              *workbench.Service
	workspace            *apitypes.Workspace
	runtimeConfigured    bool
	runtimeConfigKnown   bool
	projectPath          string
	activeProjectID      string
	sessionID            string
	runtimeCtx           context.Context
	cancel               context.CancelFunc
	eventStats           runtimeEventStats
	requests             map[string]runtimeRequestState
	sessionTurns         map[string]string
	toolEvents           map[string]runtimeToolEventState
	toolCalls            runtimeToolCallStore
	refs                 runtimeRefStore
	contextManager       contextmgr.Manager
	contextStore         contextmgr.SQLStore
	compactBoundaries    runtimeCompactBoundaryStore
	promptAssemblies     runtimePromptAssemblyStore
	worktrees            runtimeWorktreeStore
	sandboxDecisions     runtimeSandboxDecisionStore
	hookExecutions       runtimeHookExecutionStore
	agentTasks           runtimeAgentTaskStore
	turns                runtimeTurnStore
	userInputs           runtimeUserInputStore
	eventStore           runtimeEventStore
	permissionStore      runtimePermissionStore
	mcpRequestStore      runtimeMCPRequestStore
	runs                 runtimeRunStore
	transitions          runtimeRunTransitionStore
	recoveryLinks        runtimeRecoveryLinkStore
	agentTaskRunner      runtimeAgentTaskRunner
	permissions          map[string]pendingRuntimePermission
	policy               RuntimePolicy
	capabilityLoads      map[string]runtimeCapabilityLoadRecord
	toolDiscovery        runtimeToolDiscoveryState
	terminalsByID        map[string]*runtimeTerminalState
	terminalIDsBySession map[string]map[string]struct{}
	recovery             runtimeRecoveryRecord
	events               []RuntimeEvent
	nextEventSequence    int64
	eventStream          *runtimeSSEServer
	httpAPI              *runtimeHTTPServer
	messageStream        map[string]*messageStreamCursor
	sessionOutputStream  *runtimeSessionOutputBroker
	compactTurnMu        sync.Mutex
	compactTurnStates    map[string]runtimeTurnCompactState
}

// runtimeTurnCompactState is the per-(session,turn) in-memory state produced
// by a full compact executed mid-turn. It anchors the boundary projection so
// every subsequent step of the same turn keeps sending the compacted
// sequence (summary + pairing-safe tail) instead of falling back to the full
// history. It also carries the "at most one auto compact per turn" flag. The
// state is cleared when the turn reaches a terminal status and when a new
// turn starts for the session; the next turn is served by the
// session.SummaryMessageID slice in getSessionMessages.
type runtimeTurnCompactState struct {
	SummaryMessages      []fantasy.Message
	TailStartIndex       int
	SnapshotLen          int
	HasProjection        bool
	AutoCompactAttempted bool
}

// messageStreamCursor tracks how much of an assistant message's text /
// reasoning we have already forwarded as an ephemeral delta, and whether we
// have recently emitted a persisted message.updated so the next one can be
// coalesced.
type messageStreamCursor struct {
	sessionID         string
	lastTextLen       int
	lastReasoningLen  int
	lastUpdateEmitted int64
	completed         bool
}

type RuntimeEvent = runtimeapi.Event

type pendingRuntimePermission struct {
	Permission RuntimePermissionRequest
	Raw        permission.PermissionRequest
}
