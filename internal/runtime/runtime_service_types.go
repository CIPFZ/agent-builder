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
	Sessions(context.Context) (RuntimeSessionsResponse, error)
	Session(context.Context, string) (RuntimeSessionResponse, error)
	SelectSession(context.Context, string) (RuntimeStatus, error)
	RenameSession(context.Context, RuntimeSessionUpdateRequest) (RuntimeSessionsResponse, error)
	DeleteSession(context.Context, string) (RuntimeSessionsResponse, error)
	SessionMessages(context.Context, string) (RuntimeMessagesResponse, error)
	Messages(context.Context) (RuntimeMessagesResponse, error)
	Permissions(context.Context) (RuntimePermissionsResponse, error)
	Events(context.Context, ...int64) (RuntimeEventsResponse, error)
	EventsEndpoint(context.Context) (RuntimeEventsEndpointResponse, error)
	SubscribeEvents(context.Context, ...int64) (<-chan RuntimeEvent, func())
	AuditTurn(context.Context, string) (RuntimeAuditResponse, error)
	AuditSession(context.Context, string) (RuntimeAuditResponse, error)
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
	Capabilities(context.Context) (RuntimeCapabilitiesResponse, error)
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
	turns             runtimeTurnStore
	permissionStore   runtimePermissionStore
	permissions       map[string]pendingRuntimePermission
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
