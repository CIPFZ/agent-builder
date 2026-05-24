package main

import (
	"context"

	runtime "github.com/charmbracelet/crush/internal/runtime"
)

type RuntimeStatus = runtime.RuntimeStatus
type RuntimeModel = runtime.RuntimeModel
type RuntimeModelsResponse = runtime.RuntimeModelsResponse
type RuntimeConfigResponse = runtime.RuntimeConfigResponse
type RuntimeChatRequest = runtime.RuntimeChatRequest
type RuntimeChatResponse = runtime.RuntimeChatResponse
type RuntimeTurn = runtime.RuntimeTurn
type RuntimeTurnResponse = runtime.RuntimeTurnResponse
type RuntimeTurnsResponse = runtime.RuntimeTurnsResponse
type RuntimeTodosResponse = runtime.RuntimeTodosResponse
type RuntimeToolCall = runtime.RuntimeToolCall
type RuntimeToolCallResponse = runtime.RuntimeToolCallResponse
type RuntimeToolCallsResponse = runtime.RuntimeToolCallsResponse
type RuntimeCompactBoundary = runtime.RuntimeCompactBoundary
type RuntimeCompactBoundariesResponse = runtime.RuntimeCompactBoundariesResponse
type RuntimeCompactToolCallRef = runtime.RuntimeCompactToolCallRef
type RuntimeBudgetReport = runtime.RuntimeBudgetReport
type RuntimeBudgetBucket = runtime.RuntimeBudgetBucket
type RuntimeAgentTask = runtime.RuntimeAgentTask
type RuntimeAgentTaskResponse = runtime.RuntimeAgentTaskResponse
type RuntimeAgentTasksResponse = runtime.RuntimeAgentTasksResponse
type RuntimeMessage = runtime.RuntimeMessage
type RuntimeMessagePart = runtime.RuntimeMessagePart
type RuntimeSession = runtime.RuntimeSession
type RuntimeSessionsResponse = runtime.RuntimeSessionsResponse
type RuntimeSessionResponse = runtime.RuntimeSessionResponse
type RuntimeSessionUpdateRequest = runtime.RuntimeSessionUpdateRequest
type RuntimeMessagesResponse = runtime.RuntimeMessagesResponse
type RuntimePermissionRequest = runtime.RuntimePermissionRequest
type RuntimePermissionsResponse = runtime.RuntimePermissionsResponse
type RuntimePermissionDecision = runtime.RuntimePermissionDecision
type RuntimePolicy = runtime.RuntimePolicy
type RuntimePolicyResponse = runtime.RuntimePolicyResponse
type RuntimePolicyUpdateRequest = runtime.RuntimePolicyUpdateRequest
type RuntimeRequests = runtime.RuntimeRequests
type RuntimeUsage = runtime.RuntimeUsage
type RuntimeEventStats = runtime.RuntimeEventStats
type RuntimeEventsResponse = runtime.RuntimeEventsResponse
type RuntimeEventsEndpointResponse = runtime.RuntimeEventsEndpointResponse
type RuntimeRecoveryStatus = runtime.RuntimeRecoveryStatus
type RuntimeSkill = runtime.RuntimeSkill
type RuntimeSkillsResponse = runtime.RuntimeSkillsResponse
type RuntimeSkillCreateRequest = runtime.RuntimeSkillCreateRequest
type RuntimeSkillPathRequest = runtime.RuntimeSkillPathRequest
type RuntimeSkillToggleRequest = runtime.RuntimeSkillToggleRequest
type RuntimeMCPCounts = runtime.RuntimeMCPCounts
type RuntimeMCPServer = runtime.RuntimeMCPServer
type RuntimeMCPServersResponse = runtime.RuntimeMCPServersResponse
type RuntimeMCPServerConfigRequest = runtime.RuntimeMCPServerConfigRequest
type RuntimeMCPServerToggleRequest = runtime.RuntimeMCPServerToggleRequest
type RuntimeMCPTool = runtime.RuntimeMCPTool
type RuntimeMCPToolsResponse = runtime.RuntimeMCPToolsResponse
type RuntimeMCPToolToggleRequest = runtime.RuntimeMCPToolToggleRequest
type RuntimeMCPResource = runtime.RuntimeMCPResource
type RuntimeMCPResourcesResponse = runtime.RuntimeMCPResourcesResponse
type RuntimeMCPPrompt = runtime.RuntimeMCPPrompt
type RuntimeMCPPromptsResponse = runtime.RuntimeMCPPromptsResponse
type RuntimeCapability = runtime.RuntimeCapability
type RuntimeCapabilitiesResponse = runtime.RuntimeCapabilitiesResponse
type RuntimeCapabilityResponse = runtime.RuntimeCapabilityResponse
type RuntimeContextSource = runtime.RuntimeContextSource
type RuntimeContextSourcesResponse = runtime.RuntimeContextSourcesResponse
type RuntimeModelConfig = runtime.RuntimeModelConfig
type RuntimeModelVerifyResponse = runtime.RuntimeModelVerifyResponse
type RuntimeModelDiscoveryResponse = runtime.RuntimeModelDiscoveryResponse
type RuntimeAPIEndpointResponse = runtime.RuntimeAPIEndpointResponse
type RuntimeAuditEvent = runtime.RuntimeAuditEvent
type RuntimeAuditResponse = runtime.RuntimeAuditResponse
type RuntimeEvent = runtime.RuntimeEvent

// RuntimeBridge is the Wails adapter. It intentionally delegates to
// runtime.RuntimeService so desktop bindings do not become the business
// boundary.
type RuntimeBridge struct {
	service runtime.RuntimeService
}

func NewRuntimeBridge() *RuntimeBridge {

	return &RuntimeBridge{

		service: runtime.NewRuntimeService(),
	}

}

func (r *RuntimeBridge) Status(ctx context.Context) (RuntimeStatus, error) {

	return r.service.Status(ctx)

}

func (r *RuntimeBridge) RecoveryStatus(ctx context.Context) (RuntimeRecoveryStatus, error) {

	return r.service.RecoveryStatus(ctx)

}

func (r *RuntimeBridge) Models(ctx context.Context) (RuntimeModelsResponse, error) {

	return r.service.Models(ctx)

}

func (r *RuntimeBridge) GetModelConfig(ctx context.Context) (RuntimeConfigResponse, error) {

	return r.service.GetModelConfig(ctx)

}

func (r *RuntimeBridge) SaveModelConfig(ctx context.Context, req RuntimeModelConfig) (RuntimeConfigResponse, error) {

	return r.service.SaveModelConfig(ctx, req)

}

func (r *RuntimeBridge) DiscoverModelConfig(ctx context.Context, req RuntimeModelConfig) (RuntimeModelDiscoveryResponse, error) {

	return r.service.DiscoverModelConfig(ctx, req)

}

func (r *RuntimeBridge) VerifyModelConfig(ctx context.Context, req RuntimeModelConfig) (RuntimeModelVerifyResponse, error) {

	return r.service.VerifyModelConfig(ctx, req)

}

func (r *RuntimeBridge) Chat(ctx context.Context, req RuntimeChatRequest) (RuntimeChatResponse, error) {

	return r.service.Chat(ctx, req)

}

func (r *RuntimeBridge) Turn(ctx context.Context, turnID string) (RuntimeTurnResponse, error) {

	return r.service.Turn(ctx, turnID)

}

func (r *RuntimeBridge) Turns(ctx context.Context, status string) (RuntimeTurnsResponse, error) {

	return r.service.Turns(ctx, status)

}

func (r *RuntimeBridge) ToolCall(ctx context.Context, toolCallID string) (RuntimeToolCallResponse, error) {

	return r.service.ToolCall(ctx, toolCallID)

}

func (r *RuntimeBridge) TurnToolCalls(ctx context.Context, turnID string) (RuntimeToolCallsResponse, error) {

	return r.service.TurnToolCalls(ctx, turnID)

}

func (r *RuntimeBridge) TurnCompactBoundaries(ctx context.Context, turnID string) (RuntimeCompactBoundariesResponse, error) {

	return r.service.TurnCompactBoundaries(ctx, turnID)

}

func (r *RuntimeBridge) SessionCompactBoundaries(ctx context.Context, sessionID string) (RuntimeCompactBoundariesResponse, error) {

	return r.service.SessionCompactBoundaries(ctx, sessionID)

}

func (r *RuntimeBridge) AgentTask(ctx context.Context, taskID string) (RuntimeAgentTaskResponse, error) {

	return r.service.AgentTask(ctx, taskID)

}

func (r *RuntimeBridge) TurnAgentTasks(ctx context.Context, turnID string) (RuntimeAgentTasksResponse, error) {

	return r.service.TurnAgentTasks(ctx, turnID)

}

func (r *RuntimeBridge) CancelAgentTask(ctx context.Context, taskID string) (RuntimeAgentTaskResponse, error) {

	return r.service.CancelAgentTask(ctx, taskID)

}

func (r *RuntimeBridge) Sessions(ctx context.Context) (RuntimeSessionsResponse, error) {

	return r.service.Sessions(ctx)

}

func (r *RuntimeBridge) Session(ctx context.Context, sessionID string) (RuntimeSessionResponse, error) {

	return r.service.Session(ctx, sessionID)

}

func (r *RuntimeBridge) SelectSession(ctx context.Context, sessionID string) (RuntimeStatus, error) {

	return r.service.SelectSession(ctx, sessionID)

}

func (r *RuntimeBridge) RenameSession(ctx context.Context, req RuntimeSessionUpdateRequest) (RuntimeSessionsResponse, error) {

	return r.service.RenameSession(ctx, req)

}

func (r *RuntimeBridge) DeleteSession(ctx context.Context, sessionID string) (RuntimeSessionsResponse, error) {

	return r.service.DeleteSession(ctx, sessionID)

}

func (r *RuntimeBridge) SessionMessages(ctx context.Context, sessionID string) (RuntimeMessagesResponse, error) {

	return r.service.SessionMessages(ctx, sessionID)

}

func (r *RuntimeBridge) Messages(ctx context.Context) (RuntimeMessagesResponse, error) {

	return r.service.Messages(ctx)

}

func (r *RuntimeBridge) Permissions(ctx context.Context) (RuntimePermissionsResponse, error) {

	return r.service.Permissions(ctx)

}

func (r *RuntimeBridge) GetPolicy(ctx context.Context) (RuntimePolicyResponse, error) {

	return r.service.GetPolicy(ctx)

}

func (r *RuntimeBridge) UpdatePolicy(ctx context.Context, req RuntimePolicyUpdateRequest) (RuntimePolicyResponse, error) {

	return r.service.UpdatePolicy(ctx, req)

}

func (r *RuntimeBridge) Events(ctx context.Context) (RuntimeEventsResponse, error) {

	return r.service.Events(ctx)

}

func (r *RuntimeBridge) EventsEndpoint(ctx context.Context) (RuntimeEventsEndpointResponse, error) {

	return r.service.EventsEndpoint(ctx)

}

func (r *RuntimeBridge) AuditTurn(ctx context.Context, turnID string) (RuntimeAuditResponse, error) {

	return r.service.AuditTurn(ctx, turnID)

}

func (r *RuntimeBridge) AuditSession(ctx context.Context, sessionID string) (RuntimeAuditResponse, error) {

	return r.service.AuditSession(ctx, sessionID)

}

func (r *RuntimeBridge) Skills(ctx context.Context) (RuntimeSkillsResponse, error) {

	return r.service.Skills(ctx)

}

func (r *RuntimeBridge) RefreshSkills(ctx context.Context) (RuntimeSkillsResponse, error) {

	return r.service.RefreshSkills(ctx)

}

func (r *RuntimeBridge) CreateSkill(ctx context.Context, req RuntimeSkillCreateRequest) (RuntimeSkillsResponse, error) {

	return r.service.CreateSkill(ctx, req)

}

func (r *RuntimeBridge) AddSkillPath(ctx context.Context, req RuntimeSkillPathRequest) (RuntimeSkillsResponse, error) {

	return r.service.AddSkillPath(ctx, req)

}

func (r *RuntimeBridge) SetSkillEnabled(ctx context.Context, req RuntimeSkillToggleRequest) (RuntimeSkillsResponse, error) {

	return r.service.SetSkillEnabled(ctx, req)

}

func (r *RuntimeBridge) MCPServers(ctx context.Context) (RuntimeMCPServersResponse, error) {

	return r.service.MCPServers(ctx)

}

func (r *RuntimeBridge) SaveMCPServer(ctx context.Context, req RuntimeMCPServerConfigRequest) (RuntimeMCPServersResponse, error) {

	return r.service.SaveMCPServer(ctx, req)

}

func (r *RuntimeBridge) SetMCPServerEnabled(ctx context.Context, req RuntimeMCPServerToggleRequest) (RuntimeMCPServersResponse, error) {

	return r.service.SetMCPServerEnabled(ctx, req)

}

func (r *RuntimeBridge) RefreshMCPServer(ctx context.Context, name string) (RuntimeMCPServersResponse, error) {

	return r.service.RefreshMCPServer(ctx, name)

}

func (r *RuntimeBridge) SetMCPToolEnabled(ctx context.Context, req RuntimeMCPToolToggleRequest) (RuntimeMCPToolsResponse, error) {

	return r.service.SetMCPToolEnabled(ctx, req)

}

func (r *RuntimeBridge) MCPTools(ctx context.Context, name string) (RuntimeMCPToolsResponse, error) {

	return r.service.MCPTools(ctx, name)

}

func (r *RuntimeBridge) MCPResources(ctx context.Context, name string) (RuntimeMCPResourcesResponse, error) {

	return r.service.MCPResources(ctx, name)

}

func (r *RuntimeBridge) MCPPrompts(ctx context.Context, name string) (RuntimeMCPPromptsResponse, error) {

	return r.service.MCPPrompts(ctx, name)

}

func (r *RuntimeBridge) Capabilities(ctx context.Context) (RuntimeCapabilitiesResponse, error) {

	return r.service.Capabilities(ctx)

}

func (r *RuntimeBridge) RefreshCapability(ctx context.Context, capabilityID string) (RuntimeCapabilityResponse, error) {

	return r.service.RefreshCapability(ctx, capabilityID)

}

func (r *RuntimeBridge) ContextSources(ctx context.Context) (RuntimeContextSourcesResponse, error) {

	return r.service.ContextSources(ctx)

}

func (r *RuntimeBridge) APIEndpoint(ctx context.Context) (RuntimeAPIEndpointResponse, error) {

	return r.service.APIEndpoint(ctx)

}

func (r *RuntimeBridge) DecidePermission(ctx context.Context, req RuntimePermissionDecision) (RuntimeStatus, error) {

	return r.service.DecidePermission(ctx, req)

}

func (r *RuntimeBridge) Cancel(ctx context.Context) (RuntimeStatus, error) {

	return r.service.Cancel(ctx)

}

func (r *RuntimeBridge) CancelTurn(ctx context.Context, turnID string) (RuntimeStatus, error) {

	return r.service.CancelTurn(ctx, turnID)

}

func (r *RuntimeBridge) NewChat(ctx context.Context, title string) (RuntimeStatus, error) {
	return r.service.NewChat(ctx, title)
}
