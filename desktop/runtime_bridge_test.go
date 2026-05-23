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

type recordingRuntimeService struct {
	chatCalls int
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

func (s *recordingRuntimeService) Capabilities(context.Context) (RuntimeCapabilitiesResponse, error) {
	return RuntimeCapabilitiesResponse{}, nil
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
