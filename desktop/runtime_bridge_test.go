package main

import (
	"context"
	"testing"

	runtime "github.com/charmbracelet/crush/internal/runtime"
)

func TestRuntimeBridgeDelegatesToRuntimeService(t *testing.T) {
	t.Parallel()

	service := &recordingRuntimeService{}
	bridge := &RuntimeBridge{service: service}

	if _, err := bridge.Chat(context.Background(), RuntimeChatRequest{Prompt: "hello"}); err != nil {
		t.Fatal(err)
	}
	if service.chatCalls != 1 {
		t.Fatalf("chatCalls = %d, want 1", service.chatCalls)
	}
}

func TestRuntimeBridgeForwardsCapabilityRefresh(t *testing.T) {
	t.Parallel()

	service := &recordingRuntimeService{}
	bridge := &RuntimeBridge{service: service}

	resp, err := bridge.RefreshCapability(context.Background(), "skill:docs")
	if err != nil {
		t.Fatal(err)
	}
	if service.refreshedCapability != "skill:docs" {
		t.Fatalf("refreshed capability = %q", service.refreshedCapability)
	}
	if resp.Capability.ID != "skill:docs" || resp.Capability.State != "loaded" {
		t.Fatalf("response = %#v", resp)
	}
}

func TestRuntimeBridgeForwardsToolSearch(t *testing.T) {
	t.Parallel()

	service := &recordingRuntimeService{}
	bridge := &RuntimeBridge{service: service}

	resp, err := bridge.SearchTools(context.Background(), RuntimeToolSearchRequest{Query: "docs"})
	if err != nil {
		t.Fatal(err)
	}
	if service.toolSearchQuery != "docs" || resp.Query != "docs" {
		t.Fatalf("tool search forwarding failed: query=%q resp=%#v", service.toolSearchQuery, resp)
	}
}

func TestRuntimeBridgeForwardsReplayExport(t *testing.T) {
	t.Parallel()

	service := &recordingRuntimeService{}
	bridge := &RuntimeBridge{service: service}

	resp, err := bridge.ReplayExport(context.Background(), RuntimeReplayExportRequest{TurnID: "turn-1", After: 3})
	if err != nil {
		t.Fatal(err)
	}
	if service.replayExportRequest.TurnID != "turn-1" || service.replayExportRequest.After != 3 {
		t.Fatalf("replay export request = %#v", service.replayExportRequest)
	}
	if resp.TurnID != "turn-1" || !resp.Summary.Redacted {
		t.Fatalf("replay export response = %#v", resp)
	}
}

func TestRuntimeBridgeForwardsMCPRequestDecision(t *testing.T) {
	t.Parallel()

	service := &recordingRuntimeService{}
	bridge := &RuntimeBridge{service: service}

	resp, err := bridge.DecideMCPRequest(context.Background(), RuntimeMCPRequestDecision{RequestID: "mcp-req-1", Action: "approve"})
	if err != nil {
		t.Fatal(err)
	}
	if service.mcpRequestDecision.RequestID != "mcp-req-1" || service.mcpRequestDecision.Action != "approve" {
		t.Fatalf("mcp decision = %#v", service.mcpRequestDecision)
	}
	if resp.Request.ID != "mcp-req-1" || resp.Request.Status != "completed" {
		t.Fatalf("response = %#v", resp)
	}
}

type recordingRuntimeService struct {
	chatCalls           int
	refreshedCapability string
	toolSearchQuery     string
	replayExportRequest runtime.RuntimeReplayExportRequest
	mcpRequestDecision  runtime.RuntimeMCPRequestDecision
}

func (s *recordingRuntimeService) Status(context.Context) (RuntimeStatus, error) {
	return RuntimeStatus{}, nil
}

func (s *recordingRuntimeService) RecoveryStatus(context.Context) (RuntimeRecoveryStatus, error) {
	return RuntimeRecoveryStatus{}, nil
}

func (s *recordingRuntimeService) Models(context.Context) (RuntimeModelsResponse, error) {
	return RuntimeModelsResponse{}, nil
}

func (s *recordingRuntimeService) GetModelConfig(context.Context) (RuntimeConfigResponse, error) {
	return RuntimeConfigResponse{}, nil
}

func (s *recordingRuntimeService) SaveModelConfig(context.Context, RuntimeModelConfig) (RuntimeConfigResponse, error) {
	return RuntimeConfigResponse{}, nil
}

func (s *recordingRuntimeService) DiscoverModelConfig(context.Context, RuntimeModelConfig) (RuntimeModelDiscoveryResponse, error) {
	return RuntimeModelDiscoveryResponse{}, nil
}

func (s *recordingRuntimeService) VerifyModelConfig(context.Context, RuntimeModelConfig) (RuntimeModelVerifyResponse, error) {
	return RuntimeModelVerifyResponse{}, nil
}

func (s *recordingRuntimeService) Chat(context.Context, RuntimeChatRequest) (RuntimeChatResponse, error) {
	s.chatCalls++
	return RuntimeChatResponse{RequestID: "request-1", TurnID: "request-1"}, nil
}

func (s *recordingRuntimeService) Turn(context.Context, string) (RuntimeTurnResponse, error) {
	return RuntimeTurnResponse{}, nil
}

func (s *recordingRuntimeService) Turns(context.Context, string) (RuntimeTurnsResponse, error) {
	return RuntimeTurnsResponse{}, nil
}

func (s *recordingRuntimeService) ToolCall(context.Context, string) (RuntimeToolCallResponse, error) {
	return RuntimeToolCallResponse{}, nil
}

func (s *recordingRuntimeService) TurnToolCalls(context.Context, string) (RuntimeToolCallsResponse, error) {
	return RuntimeToolCallsResponse{}, nil
}

func (s *recordingRuntimeService) SandboxDecisions(context.Context, runtime.RuntimeSandboxDecisionListRequest) (runtime.RuntimeSandboxDecisionsResponse, error) {
	return runtime.RuntimeSandboxDecisionsResponse{}, nil
}

func (s *recordingRuntimeService) SandboxDecision(context.Context, string) (runtime.RuntimeSandboxDecisionResponse, error) {
	return runtime.RuntimeSandboxDecisionResponse{}, nil
}

func (s *recordingRuntimeService) Refs(context.Context, RuntimeRefListRequest) (RuntimeRefsResponse, error) {
	return RuntimeRefsResponse{}, nil
}

func (s *recordingRuntimeService) Ref(context.Context, string) (RuntimeRefResponse, error) {
	return RuntimeRefResponse{}, nil
}

func (s *recordingRuntimeService) ReadRefContent(context.Context, string) (RuntimeRefContentResponse, error) {
	return RuntimeRefContentResponse{}, nil
}

func (s *recordingRuntimeService) TurnCompactBoundaries(context.Context, string) (RuntimeCompactBoundariesResponse, error) {
	return RuntimeCompactBoundariesResponse{}, nil
}

func (s *recordingRuntimeService) SessionCompactBoundaries(context.Context, string) (RuntimeCompactBoundariesResponse, error) {
	return RuntimeCompactBoundariesResponse{}, nil
}

func (s *recordingRuntimeService) Worktrees(context.Context) (RuntimeWorktreesResponse, error) {
	return RuntimeWorktreesResponse{}, nil
}

func (s *recordingRuntimeService) Worktree(context.Context, string) (RuntimeWorktreeResponse, error) {
	return RuntimeWorktreeResponse{}, nil
}

func (s *recordingRuntimeService) CreateWorktree(context.Context, RuntimeWorktreeCreateRequest) (RuntimeWorktreeResponse, error) {
	return RuntimeWorktreeResponse{}, nil
}

func (s *recordingRuntimeService) EnterWorktree(context.Context, string, RuntimeWorktreeActionRequest) (RuntimeWorktreeResponse, error) {
	return RuntimeWorktreeResponse{}, nil
}

func (s *recordingRuntimeService) ExitWorktree(context.Context, string, RuntimeWorktreeActionRequest) (RuntimeWorktreeResponse, error) {
	return RuntimeWorktreeResponse{}, nil
}

func (s *recordingRuntimeService) CleanupWorktree(context.Context, string, RuntimeWorktreeActionRequest) (RuntimeWorktreeResponse, error) {
	return RuntimeWorktreeResponse{}, nil
}

func (s *recordingRuntimeService) AgentTask(context.Context, string) (RuntimeAgentTaskResponse, error) {
	return RuntimeAgentTaskResponse{}, nil
}

func (s *recordingRuntimeService) TaskEffectiveScope(context.Context, string) (RuntimeEffectiveScopeResponse, error) {
	return RuntimeEffectiveScopeResponse{}, nil
}

func (s *recordingRuntimeService) TurnAgentTasks(context.Context, string) (RuntimeAgentTasksResponse, error) {
	return RuntimeAgentTasksResponse{}, nil
}

func (s *recordingRuntimeService) CancelAgentTask(context.Context, string) (RuntimeAgentTaskResponse, error) {
	return RuntimeAgentTaskResponse{}, nil
}

func (s *recordingRuntimeService) AgentRoles(context.Context) (RuntimeAgentRolesResponse, error) {
	return RuntimeAgentRolesResponse{}, nil
}

func (s *recordingRuntimeService) AgentRole(context.Context, string) (RuntimeAgentRoleResponse, error) {
	return RuntimeAgentRoleResponse{}, nil
}

func (s *recordingRuntimeService) AgentTaskMessages(context.Context, string) (RuntimeAgentTaskMessagesResponse, error) {
	return RuntimeAgentTaskMessagesResponse{}, nil
}

func (s *recordingRuntimeService) CreateAgentTaskMessage(context.Context, string, RuntimeAgentTaskMessageCreateRequest) (RuntimeAgentTaskMessageResponse, error) {
	return RuntimeAgentTaskMessageResponse{}, nil
}

func (s *recordingRuntimeService) AgentTaskResult(context.Context, string) (RuntimeAgentTaskResultResponse, error) {
	return RuntimeAgentTaskResultResponse{}, nil
}

func (s *recordingRuntimeService) SessionTodos(context.Context, string) (RuntimeTodosResponse, error) {
	return RuntimeTodosResponse{}, nil
}

func (s *recordingRuntimeService) TurnTodos(context.Context, string) (RuntimeTodosResponse, error) {
	return RuntimeTodosResponse{}, nil
}

func (s *recordingRuntimeService) Sessions(context.Context) (RuntimeSessionsResponse, error) {
	return RuntimeSessionsResponse{}, nil
}

func (s *recordingRuntimeService) Session(context.Context, string) (RuntimeSessionResponse, error) {
	return RuntimeSessionResponse{}, nil
}

func (s *recordingRuntimeService) SelectSession(context.Context, string) (RuntimeStatus, error) {
	return RuntimeStatus{}, nil
}

func (s *recordingRuntimeService) RenameSession(context.Context, RuntimeSessionUpdateRequest) (RuntimeSessionsResponse, error) {
	return RuntimeSessionsResponse{}, nil
}

func (s *recordingRuntimeService) DeleteSession(context.Context, string) (RuntimeSessionsResponse, error) {
	return RuntimeSessionsResponse{}, nil
}

func (s *recordingRuntimeService) SessionMessages(context.Context, string) (RuntimeMessagesResponse, error) {
	return RuntimeMessagesResponse{}, nil
}

func (s *recordingRuntimeService) Messages(context.Context) (RuntimeMessagesResponse, error) {
	return RuntimeMessagesResponse{}, nil
}

func (s *recordingRuntimeService) Permissions(context.Context) (RuntimePermissionsResponse, error) {
	return RuntimePermissionsResponse{}, nil
}

func (s *recordingRuntimeService) GetPolicy(context.Context) (RuntimePolicyResponse, error) {
	return RuntimePolicyResponse{}, nil
}

func (s *recordingRuntimeService) UpdatePolicy(context.Context, RuntimePolicyUpdateRequest) (RuntimePolicyResponse, error) {
	return RuntimePolicyResponse{}, nil
}

func (s *recordingRuntimeService) Events(context.Context, ...int64) (RuntimeEventsResponse, error) {
	return RuntimeEventsResponse{}, nil
}

func (s *recordingRuntimeService) EventsEndpoint(context.Context) (RuntimeEventsEndpointResponse, error) {
	return RuntimeEventsEndpointResponse{}, nil
}

func (s *recordingRuntimeService) SubscribeEvents(context.Context, ...int64) (<-chan RuntimeEvent, func()) {
	events := make(chan RuntimeEvent)
	return events, func() {
		close(events)
	}
}

func (s *recordingRuntimeService) AuditTurn(context.Context, string) (RuntimeAuditResponse, error) {
	return RuntimeAuditResponse{}, nil
}

func (s *recordingRuntimeService) AuditSession(context.Context, string) (RuntimeAuditResponse, error) {
	return RuntimeAuditResponse{}, nil
}

func (s *recordingRuntimeService) ReplayExport(_ context.Context, req runtime.RuntimeReplayExportRequest) (runtime.RuntimeReplayExportResponse, error) {
	s.replayExportRequest = req
	return runtime.RuntimeReplayExportResponse{TurnID: req.TurnID, Summary: runtime.RuntimeReplayExportSummary{Redacted: true}}, nil
}

func (s *recordingRuntimeService) Skills(context.Context) (RuntimeSkillsResponse, error) {
	return RuntimeSkillsResponse{}, nil
}

func (s *recordingRuntimeService) RefreshSkills(context.Context) (RuntimeSkillsResponse, error) {
	return RuntimeSkillsResponse{}, nil
}

func (s *recordingRuntimeService) CreateSkill(context.Context, RuntimeSkillCreateRequest) (RuntimeSkillsResponse, error) {
	return RuntimeSkillsResponse{}, nil
}

func (s *recordingRuntimeService) AddSkillPath(context.Context, RuntimeSkillPathRequest) (RuntimeSkillsResponse, error) {
	return RuntimeSkillsResponse{}, nil
}

func (s *recordingRuntimeService) SetSkillEnabled(context.Context, RuntimeSkillToggleRequest) (RuntimeSkillsResponse, error) {
	return RuntimeSkillsResponse{}, nil
}

func (s *recordingRuntimeService) MCPServers(context.Context) (RuntimeMCPServersResponse, error) {
	return RuntimeMCPServersResponse{}, nil
}

func (s *recordingRuntimeService) SaveMCPServer(context.Context, RuntimeMCPServerConfigRequest) (RuntimeMCPServersResponse, error) {
	return RuntimeMCPServersResponse{}, nil
}

func (s *recordingRuntimeService) SetMCPServerEnabled(context.Context, RuntimeMCPServerToggleRequest) (RuntimeMCPServersResponse, error) {
	return RuntimeMCPServersResponse{}, nil
}

func (s *recordingRuntimeService) RefreshMCPServer(context.Context, string) (RuntimeMCPServersResponse, error) {
	return RuntimeMCPServersResponse{}, nil
}

func (s *recordingRuntimeService) SetMCPToolEnabled(context.Context, RuntimeMCPToolToggleRequest) (RuntimeMCPToolsResponse, error) {
	return RuntimeMCPToolsResponse{}, nil
}

func (s *recordingRuntimeService) MCPTools(context.Context, string) (RuntimeMCPToolsResponse, error) {
	return RuntimeMCPToolsResponse{}, nil
}

func (s *recordingRuntimeService) MCPResources(context.Context, string) (RuntimeMCPResourcesResponse, error) {
	return RuntimeMCPResourcesResponse{}, nil
}

func (s *recordingRuntimeService) MCPPrompts(context.Context, string) (RuntimeMCPPromptsResponse, error) {
	return RuntimeMCPPromptsResponse{}, nil
}

func (s *recordingRuntimeService) MCPRequests(context.Context, RuntimeMCPRequestListRequest) (RuntimeMCPRequestsResponse, error) {
	return RuntimeMCPRequestsResponse{}, nil
}

func (s *recordingRuntimeService) MCPRequest(context.Context, string) (RuntimeMCPRequestResponse, error) {
	return RuntimeMCPRequestResponse{}, nil
}

func (s *recordingRuntimeService) DecideMCPRequest(_ context.Context, req RuntimeMCPRequestDecision) (RuntimeMCPRequestResponse, error) {
	s.mcpRequestDecision = req
	return RuntimeMCPRequestResponse{Request: RuntimeMCPRequest{ID: req.RequestID, Status: "completed", Redacted: true}}, nil
}

func (s *recordingRuntimeService) RetryMCPServer(context.Context, string) (RuntimeMCPServersResponse, error) {
	return RuntimeMCPServersResponse{}, nil
}

func (s *recordingRuntimeService) Capabilities(context.Context) (RuntimeCapabilitiesResponse, error) {
	return RuntimeCapabilitiesResponse{}, nil
}

func (s *recordingRuntimeService) RefreshCapability(_ context.Context, id string) (RuntimeCapabilityResponse, error) {
	s.refreshedCapability = id
	return RuntimeCapabilityResponse{Capability: RuntimeCapability{ID: id, Kind: "skill", Name: "docs", Enabled: true, State: "loaded"}}, nil
}

func (s *recordingRuntimeService) SearchTools(_ context.Context, req RuntimeToolSearchRequest) (RuntimeToolSearchResponse, error) {
	s.toolSearchQuery = req.Query
	return RuntimeToolSearchResponse{Query: req.Query}, nil
}

func (s *recordingRuntimeService) ContextSources(context.Context) (RuntimeContextSourcesResponse, error) {
	return RuntimeContextSourcesResponse{}, nil
}

func (s *recordingRuntimeService) APIEndpoint(context.Context) (RuntimeAPIEndpointResponse, error) {
	return RuntimeAPIEndpointResponse{}, nil
}

func (s *recordingRuntimeService) DecidePermission(context.Context, RuntimePermissionDecision) (RuntimeStatus, error) {
	return RuntimeStatus{}, nil
}

func (s *recordingRuntimeService) Cancel(context.Context) (RuntimeStatus, error) {
	return RuntimeStatus{}, nil
}

func (s *recordingRuntimeService) CancelTurn(context.Context, string) (RuntimeStatus, error) {
	return RuntimeStatus{}, nil
}

func (s *recordingRuntimeService) NewChat(context.Context, string) (RuntimeStatus, error) {
	return RuntimeStatus{}, nil
}

var _ runtime.RuntimeService = (*recordingRuntimeService)(nil)
