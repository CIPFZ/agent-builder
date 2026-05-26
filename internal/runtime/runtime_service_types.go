package runtime

import (
	"context"
	"sync"

	"github.com/charmbracelet/crush/internal/backend"
	"github.com/charmbracelet/crush/internal/permission"
	"github.com/charmbracelet/crush/internal/proto"
	"github.com/charmbracelet/crush/internal/runtimeapi"
)

// RuntimeService is the transport-neutral runtime boundary used by Wails and
// the local HTTP adapter.
type RuntimeService interface {
	Status(context.Context) (RuntimeStatus, error)
	RecoveryStatus(context.Context) (RuntimeRecoveryStatus, error)
	Models(context.Context) (RuntimeModelsResponse, error)
	GetModelConfig(context.Context) (RuntimeConfigResponse, error)
	SaveModelConfig(context.Context, RuntimeModelConfig) (RuntimeConfigResponse, error)
	DiscoverModelConfig(context.Context, RuntimeModelConfig) (RuntimeModelDiscoveryResponse, error)
	VerifyModelConfig(context.Context, RuntimeModelConfig) (RuntimeModelVerifyResponse, error)
	Chat(context.Context, RuntimeChatRequest) (RuntimeChatResponse, error)
	Turn(context.Context, string) (RuntimeTurnResponse, error)
	Turns(context.Context, string) (RuntimeTurnsResponse, error)
	ToolCall(context.Context, string) (RuntimeToolCallResponse, error)
	TurnToolCalls(context.Context, string) (RuntimeToolCallsResponse, error)
	SandboxDecisions(context.Context, RuntimeSandboxDecisionListRequest) (RuntimeSandboxDecisionsResponse, error)
	SandboxDecision(context.Context, string) (RuntimeSandboxDecisionResponse, error)
	Refs(context.Context, RuntimeRefListRequest) (RuntimeRefsResponse, error)
	Ref(context.Context, string) (RuntimeRefResponse, error)
	ReadRefContent(context.Context, string) (RuntimeRefContentResponse, error)
	TurnCompactBoundaries(context.Context, string) (RuntimeCompactBoundariesResponse, error)
	SessionCompactBoundaries(context.Context, string) (RuntimeCompactBoundariesResponse, error)
	Worktrees(context.Context) (RuntimeWorktreesResponse, error)
	Worktree(context.Context, string) (RuntimeWorktreeResponse, error)
	CreateWorktree(context.Context, RuntimeWorktreeCreateRequest) (RuntimeWorktreeResponse, error)
	EnterWorktree(context.Context, string, RuntimeWorktreeActionRequest) (RuntimeWorktreeResponse, error)
	ExitWorktree(context.Context, string, RuntimeWorktreeActionRequest) (RuntimeWorktreeResponse, error)
	CleanupWorktree(context.Context, string, RuntimeWorktreeActionRequest) (RuntimeWorktreeResponse, error)
	AgentTask(context.Context, string) (RuntimeAgentTaskResponse, error)
	TaskEffectiveScope(context.Context, string) (RuntimeEffectiveScopeResponse, error)
	TurnAgentTasks(context.Context, string) (RuntimeAgentTasksResponse, error)
	CancelAgentTask(context.Context, string) (RuntimeAgentTaskResponse, error)
	AgentRoles(context.Context) (RuntimeAgentRolesResponse, error)
	AgentRole(context.Context, string) (RuntimeAgentRoleResponse, error)
	AgentTaskMessages(context.Context, string) (RuntimeAgentTaskMessagesResponse, error)
	CreateAgentTaskMessage(context.Context, string, RuntimeAgentTaskMessageCreateRequest) (RuntimeAgentTaskMessageResponse, error)
	AgentTaskResult(context.Context, string) (RuntimeAgentTaskResultResponse, error)
	SessionTodos(context.Context, string) (RuntimeTodosResponse, error)
	TurnTodos(context.Context, string) (RuntimeTodosResponse, error)
	Sessions(context.Context) (RuntimeSessionsResponse, error)
	Session(context.Context, string) (RuntimeSessionResponse, error)
	SelectSession(context.Context, string) (RuntimeStatus, error)
	RenameSession(context.Context, RuntimeSessionUpdateRequest) (RuntimeSessionsResponse, error)
	DeleteSession(context.Context, string) (RuntimeSessionsResponse, error)
	SessionMessages(context.Context, string) (RuntimeMessagesResponse, error)
	Messages(context.Context) (RuntimeMessagesResponse, error)
	Permissions(context.Context) (RuntimePermissionsResponse, error)
	GetPolicy(context.Context) (RuntimePolicyResponse, error)
	UpdatePolicy(context.Context, RuntimePolicyUpdateRequest) (RuntimePolicyResponse, error)
	Events(context.Context, ...int64) (RuntimeEventsResponse, error)
	EventsEndpoint(context.Context) (RuntimeEventsEndpointResponse, error)
	SubscribeEvents(context.Context, ...int64) (<-chan RuntimeEvent, func())
	AuditTurn(context.Context, string) (RuntimeAuditResponse, error)
	AuditSession(context.Context, string) (RuntimeAuditResponse, error)
	ReplayExport(context.Context, RuntimeReplayExportRequest) (RuntimeReplayExportResponse, error)
	Skills(context.Context) (RuntimeSkillsResponse, error)
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
	APIEndpoint(context.Context) (RuntimeAPIEndpointResponse, error)
	DecidePermission(context.Context, RuntimePermissionDecision) (RuntimeStatus, error)
	Cancel(context.Context) (RuntimeStatus, error)
	CancelTurn(context.Context, string) (RuntimeStatus, error)
	NewChat(context.Context, string) (RuntimeStatus, error)
}

// runtimeService owns workspace, session, and agent lifecycle.
type runtimeService struct {
	mu                sync.Mutex
	runtime           *backend.Backend
	workspace         *proto.Workspace
	sessionID         string
	runtimeCtx        context.Context
	cancel            context.CancelFunc
	eventStats        runtimeEventStats
	requests          map[string]runtimeRequestState
	sessionTurns      map[string]string
	toolEvents        map[string]runtimeToolEventState
	toolCalls         runtimeToolCallStore
	refs              runtimeRefStore
	compactBoundaries runtimeCompactBoundaryStore
	worktrees         runtimeWorktreeStore
	sandboxDecisions  runtimeSandboxDecisionStore
	agentTasks        runtimeAgentTaskStore
	turns             runtimeTurnStore
	eventStore        runtimeEventStore
	permissionStore   runtimePermissionStore
	mcpRequestStore   runtimeMCPRequestStore
	permissions       map[string]pendingRuntimePermission
	policy            RuntimePolicy
	capabilityLoads   map[string]runtimeCapabilityLoadRecord
	toolDiscovery     runtimeToolDiscoveryState
	recovery          runtimeRecoveryRecord
	events            []RuntimeEvent
	nextEventSequence int64
	eventStream       *runtimeSSEServer
	httpAPI           *runtimeHTTPServer
}

type RuntimeEvent = runtimeapi.Event

type pendingRuntimePermission struct {
	Permission RuntimePermissionRequest
	Raw        permission.PermissionRequest
}
