package main

import (
	"context"
)

func NewRuntimeBridge() *RuntimeBridge {

	return &RuntimeBridge{

		service: NewRuntimeService(),
	}

}

func (r *RuntimeBridge) Status(ctx context.Context) (RuntimeStatus, error) {

	return r.service.Status(ctx)

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

func (r *RuntimeBridge) VerifyModelConfig(ctx context.Context, req RuntimeModelConfig) (RuntimeModelVerifyResponse, error) {

	return r.service.VerifyModelConfig(ctx, req)

}

func (r *RuntimeBridge) Chat(ctx context.Context, req RuntimeChatRequest) (RuntimeChatResponse, error) {

	return r.service.Chat(ctx, req)

}

func (r *RuntimeBridge) Turn(ctx context.Context, turnID string) (RuntimeTurnResponse, error) {

	return r.service.Turn(ctx, turnID)

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
