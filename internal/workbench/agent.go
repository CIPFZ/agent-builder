package workbench

import (
	"context"

	"github.com/CIPFZ/agent-builder/internal/agent"
	"github.com/CIPFZ/agent-builder/internal/apitypes"
	"github.com/CIPFZ/agent-builder/internal/config"
	"github.com/CIPFZ/agent-builder/internal/skills"
)

// SendMessage sends a prompt to the agent coordinator for the given
// workspace and session.
func (b *Service) SendMessage(ctx context.Context, workspaceID string, msg apitypes.AgentMessage) error {
	ws, err := b.GetWorkspace(workspaceID)
	if err != nil {
		return err
	}

	if ws.AgentCoordinator == nil {
		return ErrAgentNotInitialized
	}

	_, err = ws.AgentCoordinator.RunWithMetadata(ctx, msg.SessionID, msg.TurnID, msg.Prompt, msg.Metadata, apitypes.AttachmentsToMessage(msg.Attachments)...)
	return err
}

// SendSessionMessage sends a follow-up prompt to an existing coordinator-owned
// session. Child AgentTask sessions are routed to their child agent when active.
func (b *Service) SendSessionMessage(ctx context.Context, workspaceID string, msg apitypes.AgentMessage) error {
	ws, err := b.GetWorkspace(workspaceID)
	if err != nil {
		return err
	}
	if ws.AgentCoordinator == nil {
		return ErrAgentNotInitialized
	}
	return ws.AgentCoordinator.SendToSession(ctx, msg.SessionID, msg.TurnID, msg.Prompt)
}

func (b *Service) ExecuteStartedAgentTask(ctx context.Context, workspaceID string, req agent.StartedAgentTaskExecutionRequest) (agent.StartedAgentTaskExecutionResult, error) {
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
func (b *Service) GetAgentInfo(workspaceID string) (apitypes.AgentInfo, error) {
	ws, err := b.GetWorkspace(workspaceID)
	if err != nil {
		return apitypes.AgentInfo{}, err
	}

	var agentInfo apitypes.AgentInfo
	if ws.AgentCoordinator != nil {
		m := ws.AgentCoordinator.Model()
		agentInfo = apitypes.AgentInfo{
			Model:    m.CatwalkCfg,
			ModelCfg: m.ModelCfg,
			IsBusy:   ws.AgentCoordinator.IsBusy(),
			IsReady:  true,
		}
	}
	return agentInfo, nil
}

// InitAgent initializes the coder agent for the workspace.
func (b *Service) InitAgent(ctx context.Context, workspaceID string) error {
	ws, err := b.GetWorkspace(workspaceID)
	if err != nil {
		return err
	}

	return ws.InitCoderAgent(ctx)
}

// UpdateAgent reloads the agent model configuration.
func (b *Service) UpdateAgent(ctx context.Context, workspaceID string) error {
	ws, err := b.GetWorkspace(workspaceID)
	if err != nil {
		return err
	}

	return ws.UpdateAgentModel(ctx)
}

func (b *Service) RefreshWorkspaceSkills(ctx context.Context, workspaceID string) error {
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
func (b *Service) CancelSession(workspaceID, sessionID string) error {
	ws, err := b.GetWorkspace(workspaceID)
	if err != nil {
		return err
	}

	if ws.AgentCoordinator != nil {
		ws.AgentCoordinator.Cancel(sessionID)
	}
	return nil
}

// GenerateCompactSummary drives the model summarizer for a compact
// operation. Returns the summary text on success; the caller falls back to
// the heuristic summarizer on error.
func (b *Service) GenerateCompactSummary(ctx context.Context, workspaceID string, req agent.CompactSummaryRequest) (agent.CompactSummaryResult, error) {
	ws, err := b.GetWorkspace(workspaceID)
	if err != nil {
		return agent.CompactSummaryResult{}, err
	}
	if ws.AgentCoordinator == nil {
		return agent.CompactSummaryResult{}, ErrAgentNotInitialized
	}
	return ws.AgentCoordinator.GenerateCompactSummary(ctx, req)
}

// RunCompactHooks executes matching PreCompact / PostCompact hooks. Errors
// from the runner are returned so the caller can slog.Warn without failing
// the compact.
func (b *Service) RunCompactHooks(ctx context.Context, workspaceID, event, sessionID, turnID, payloadJSON string) (agent.CompactHookResult, error) {
	ws, err := b.GetWorkspace(workspaceID)
	if err != nil {
		return agent.CompactHookResult{}, err
	}
	if ws.AgentCoordinator == nil {
		return agent.CompactHookResult{}, ErrAgentNotInitialized
	}
	return ws.AgentCoordinator.RunCompactHooks(ctx, event, sessionID, turnID, payloadJSON)
}

// MarkSessionReadFilesStale marks every recorded read for the session as
// stale, forcing the model to re-read files it wants to work with. This is
// called after a successful compact so we do not carry cached file state
// past the boundary.
func (b *Service) MarkSessionReadFilesStale(ctx context.Context, workspaceID, sessionID, reason string) error {
	ws, err := b.GetWorkspace(workspaceID)
	if err != nil {
		return err
	}
	if ws.FileTracker == nil {
		return nil
	}
	return ws.FileTracker.MarkSessionStale(ctx, sessionID, reason)
}

// SummarizeSession triggers a session summarization.
func (b *Service) SummarizeSession(ctx context.Context, workspaceID, sessionID string) error {
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
func (b *Service) QueuedPrompts(workspaceID, sessionID string) (int, error) {
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
func (b *Service) ClearQueue(workspaceID, sessionID string) error {
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
func (b *Service) QueuedPromptsList(workspaceID, sessionID string) ([]string, error) {
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
func (b *Service) GetDefaultSmallModel(workspaceID, providerID string) (config.SelectedModel, error) {
	ws, err := b.GetWorkspace(workspaceID)
	if err != nil {
		return config.SelectedModel{}, err
	}

	return ws.GetDefaultSmallModel(providerID), nil
}
