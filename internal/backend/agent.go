package backend

import (
	"context"

	"github.com/charmbracelet/crush/internal/agent"
	"github.com/charmbracelet/crush/internal/config"
	"github.com/charmbracelet/crush/internal/proto"
	"github.com/charmbracelet/crush/internal/skills"
)

// SendMessage sends a prompt to the agent coordinator for the given
// workspace and session.
func (b *Backend) SendMessage(ctx context.Context, workspaceID string, msg proto.AgentMessage) error {
	ws, err := b.GetWorkspace(workspaceID)
	if err != nil {
		return err
	}

	if ws.AgentCoordinator == nil {
		return ErrAgentNotInitialized
	}

	_, err = ws.AgentCoordinator.Run(ctx, msg.SessionID, msg.TurnID, msg.Prompt, proto.AttachmentsToMessage(msg.Attachments)...)
	return err
}

// SendSessionMessage sends a follow-up prompt to an existing coordinator-owned
// session. Child AgentTask sessions are routed to their child agent when active.
func (b *Backend) SendSessionMessage(ctx context.Context, workspaceID string, msg proto.AgentMessage) error {
	ws, err := b.GetWorkspace(workspaceID)
	if err != nil {
		return err
	}
	if ws.AgentCoordinator == nil {
		return ErrAgentNotInitialized
	}
	return ws.AgentCoordinator.SendToSession(ctx, msg.SessionID, msg.TurnID, msg.Prompt)
}

func (b *Backend) ExecuteStartedAgentTask(ctx context.Context, workspaceID string, req agent.StartedAgentTaskExecutionRequest) (agent.StartedAgentTaskExecutionResult, error) {
	ws, err := b.GetWorkspace(workspaceID)
	if err != nil {
		return agent.StartedAgentTaskExecutionResult{}, err
	}
	if ws.AgentCoordinator == nil {
		return agent.StartedAgentTaskExecutionResult{}, ErrAgentNotInitialized
	}
	return ws.AgentCoordinator.ExecuteConfiguredStartedAgentTask(ctx, req)
}

// GetAgentInfo returns the agent's model and busy status.
func (b *Backend) GetAgentInfo(workspaceID string) (proto.AgentInfo, error) {
	ws, err := b.GetWorkspace(workspaceID)
	if err != nil {
		return proto.AgentInfo{}, err
	}

	var agentInfo proto.AgentInfo
	if ws.AgentCoordinator != nil {
		m := ws.AgentCoordinator.Model()
		agentInfo = proto.AgentInfo{
			Model:    m.CatwalkCfg,
			ModelCfg: m.ModelCfg,
			IsBusy:   ws.AgentCoordinator.IsBusy(),
			IsReady:  true,
		}
	}
	return agentInfo, nil
}

// InitAgent initializes the coder agent for the workspace.
func (b *Backend) InitAgent(ctx context.Context, workspaceID string) error {
	ws, err := b.GetWorkspace(workspaceID)
	if err != nil {
		return err
	}

	return ws.InitCoderAgent(ctx)
}

// UpdateAgent reloads the agent model configuration.
func (b *Backend) UpdateAgent(ctx context.Context, workspaceID string) error {
	ws, err := b.GetWorkspace(workspaceID)
	if err != nil {
		return err
	}

	return ws.UpdateAgentModel(ctx)
}

func (b *Backend) RefreshWorkspaceSkills(ctx context.Context, workspaceID string) error {
	ws, err := b.GetWorkspace(workspaceID)
	if err != nil {
		return err
	}
	discoveryCfg := skillsDiscoveryConfig(ws.Cfg)
	allSkills, activeSkills, states := skills.DiscoverFromConfig(discoveryCfg)
	if ws.Skills != nil {
		ws.Skills.SetDiscoverySnapshot(allSkills, activeSkills, states,
			skills.WithResolvedPaths(discoveryCfg.ResolvePaths()),
			skills.WithWorkingDir(discoveryCfg.WorkingDir),
		)
		ws.Skills.PublishStates(states)
	}
	return ws.RefreshSkills(ctx)
}

// CancelSession cancels an ongoing agent operation for the given
// session.
func (b *Backend) CancelSession(workspaceID, sessionID string) error {
	ws, err := b.GetWorkspace(workspaceID)
	if err != nil {
		return err
	}

	if ws.AgentCoordinator != nil {
		ws.AgentCoordinator.Cancel(sessionID)
	}
	return nil
}

// SummarizeSession triggers a session summarization.
func (b *Backend) SummarizeSession(ctx context.Context, workspaceID, sessionID string) error {
	ws, err := b.GetWorkspace(workspaceID)
	if err != nil {
		return err
	}

	if ws.AgentCoordinator == nil {
		return ErrAgentNotInitialized
	}

	return ws.AgentCoordinator.Summarize(ctx, sessionID)
}

// QueuedPrompts returns the number of queued prompts for the session.
func (b *Backend) QueuedPrompts(workspaceID, sessionID string) (int, error) {
	ws, err := b.GetWorkspace(workspaceID)
	if err != nil {
		return 0, err
	}

	if ws.AgentCoordinator == nil {
		return 0, nil
	}

	return ws.AgentCoordinator.QueuedPrompts(sessionID), nil
}

// ClearQueue clears the prompt queue for the session.
func (b *Backend) ClearQueue(workspaceID, sessionID string) error {
	ws, err := b.GetWorkspace(workspaceID)
	if err != nil {
		return err
	}

	if ws.AgentCoordinator != nil {
		ws.AgentCoordinator.ClearQueue(sessionID)
	}
	return nil
}

// QueuedPromptsList returns the list of queued prompt strings for a
// session.
func (b *Backend) QueuedPromptsList(workspaceID, sessionID string) ([]string, error) {
	ws, err := b.GetWorkspace(workspaceID)
	if err != nil {
		return nil, err
	}

	if ws.AgentCoordinator == nil {
		return nil, nil
	}

	return ws.AgentCoordinator.QueuedPromptsList(sessionID), nil
}

// GetDefaultSmallModel returns the default small model for a provider.
func (b *Backend) GetDefaultSmallModel(workspaceID, providerID string) (config.SelectedModel, error) {
	ws, err := b.GetWorkspace(workspaceID)
	if err != nil {
		return config.SelectedModel{}, err
	}

	return ws.GetDefaultSmallModel(providerID), nil
}
