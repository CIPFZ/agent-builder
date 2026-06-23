package workbench

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/CIPFZ/agent-builder/internal/agent"
	mcptools "github.com/CIPFZ/agent-builder/internal/agent/tools/mcp"
	"github.com/CIPFZ/agent-builder/internal/apitypes"
	"github.com/CIPFZ/agent-builder/internal/config"
	"github.com/CIPFZ/agent-builder/internal/oauth"
	"github.com/CIPFZ/agent-builder/internal/skills"
)

// MCPResourceContents holds the contents of an MCP resource returned
// by the workbench.
type MCPResourceContents struct {
	URI      string `json:"uri"`
	MIMEType string `json:"mime_type,omitempty"`
	Text     string `json:"text,omitempty"`
	Blob     []byte `json:"blob,omitempty"`
}

// SetConfigField sets a key/value pair in the config file for the
// given scope.
func (b *Service) SetConfigField(workspaceID string, scope config.Scope, key string, value any) error {
	ws, err := b.GetWorkspace(workspaceID)
	if err != nil {
		return err
	}
	return ws.Cfg.SetConfigField(scope, key, value)
}

// RemoveConfigField removes a key from the config file for the given
// scope.
func (b *Service) RemoveConfigField(workspaceID string, scope config.Scope, key string) error {
	ws, err := b.GetWorkspace(workspaceID)
	if err != nil {
		return err
	}
	return ws.Cfg.RemoveConfigField(scope, key)
}

// UpdatePreferredModel updates the preferred model for the given type
// and persists it to the config file at the given scope.
func (b *Service) UpdatePreferredModel(workspaceID string, scope config.Scope, modelType config.SelectedModelType, model config.SelectedModel) error {
	ws, err := b.GetWorkspace(workspaceID)
	if err != nil {
		return err
	}
	return ws.Cfg.UpdatePreferredModel(scope, modelType, model)
}

// SetProviderAPIKey sets the API key for a provider and persists it.
func (b *Service) SetProviderAPIKey(workspaceID string, scope config.Scope, providerID string, apiKey any) error {
	ws, err := b.GetWorkspace(workspaceID)
	if err != nil {
		return err
	}
	return ws.Cfg.SetProviderAPIKey(scope, providerID, apiKey)
}

// ImportCopilot attempts to import a GitHub Copilot token from disk.
func (b *Service) ImportCopilot(workspaceID string) (*oauth.Token, bool, error) {
	ws, err := b.GetWorkspace(workspaceID)
	if err != nil {
		return nil, false, err
	}
	token, ok := ws.Cfg.ImportCopilot()
	return token, ok, nil
}

// RefreshOAuthToken refreshes the OAuth token for a provider.
func (b *Service) RefreshOAuthToken(ctx context.Context, workspaceID string, scope config.Scope, providerID string) error {
	ws, err := b.GetWorkspace(workspaceID)
	if err != nil {
		return err
	}
	return ws.Cfg.RefreshOAuthToken(ctx, scope, providerID)
}

// ProjectNeedsInitialization checks whether the project in this
// workspace needs initialization.
func (b *Service) ProjectNeedsInitialization(workspaceID string) (bool, error) {
	ws, err := b.GetWorkspace(workspaceID)
	if err != nil {
		return false, err
	}
	return config.ProjectNeedsInitialization(ws.Cfg)
}

// MarkProjectInitialized marks the project as initialized.
func (b *Service) MarkProjectInitialized(workspaceID string) error {
	ws, err := b.GetWorkspace(workspaceID)
	if err != nil {
		return err
	}
	return config.MarkProjectInitialized(ws.Cfg)
}

// InitializePrompt builds the initialization prompt for the workspace.
func (b *Service) InitializePrompt(workspaceID string) (string, error) {
	ws, err := b.GetWorkspace(workspaceID)
	if err != nil {
		return "", err
	}
	return agent.InitializePrompt(ws.Cfg)
}

// ReadSkill reads a skill's content by ID.
func (b *Service) ReadSkill(ctx context.Context, workspaceID, skillID string) ([]byte, apitypes.SkillReadResult, error) {
	ws, err := b.GetWorkspace(workspaceID)
	if err != nil {
		return nil, apitypes.SkillReadResult{}, err
	}

	mgr := ws.Skills
	content, result, err := skills.ReadContent(
		mgr.ActiveSkills(), mgr.ResolvedPaths(), mgr.WorkingDir(), skillID,
	)
	if err != nil {
		return nil, apitypes.SkillReadResult{}, err
	}
	return content, apitypes.SkillReadResult{
		Name:        result.Name,
		Description: result.Description,
		Source:      string(result.Source),
		Builtin:     result.Builtin,
	}, nil
}

// ListSkills returns the effective visible skills for a workspace.
func (b *Service) ListSkills(workspaceID string) ([]apitypes.SkillInfo, error) {
	ws, err := b.GetWorkspace(workspaceID)
	if err != nil {
		return nil, err
	}
	mgr := ws.Skills
	entries := skills.Catalog(mgr.ActiveSkills(), mgr.ResolvedPaths(), mgr.WorkingDir())
	result := make([]apitypes.SkillInfo, len(entries))
	for i, entry := range entries {
		result[i] = apitypes.SkillInfo{
			ID:          entry.ID,
			Name:        entry.Name,
			Description: entry.Description,
			Label:       entry.Label,
			Source:      string(entry.Source),
		}
	}
	return result, nil
}

// EnableDockerMCP validates Docker MCP availability, stages the
// configuration, starts the MCP client, and persists the config.
func (b *Service) EnableDockerMCP(ctx context.Context, workspaceID string) error {
	ws, err := b.GetWorkspace(workspaceID)
	if err != nil {
		return err
	}

	mcpConfig, err := ws.Cfg.PrepareDockerMCPConfig()
	if err != nil {
		return err
	}

	if err := mcptools.InitializeSingle(ctx, config.DockerMCPName, ws.Cfg); err != nil {
		disableErr := mcptools.DisableSingle(ws.Cfg, config.DockerMCPName)
		delete(ws.Cfg.Config().MCP, config.DockerMCPName)
		return fmt.Errorf("failed to start docker MCP: %w", errors.Join(err, disableErr))
	}

	if err := ws.Cfg.PersistDockerMCPConfig(mcpConfig); err != nil {
		disableErr := mcptools.DisableSingle(ws.Cfg, config.DockerMCPName)
		delete(ws.Cfg.Config().MCP, config.DockerMCPName)
		return fmt.Errorf("docker MCP started but failed to persist configuration: %w", errors.Join(err, disableErr))
	}

	return nil
}

// DisableDockerMCP closes the Docker MCP client, removes the
// configuration, and persists the change.
func (b *Service) DisableDockerMCP(workspaceID string) error {
	ws, err := b.GetWorkspace(workspaceID)
	if err != nil {
		return err
	}

	if err := mcptools.DisableSingle(ws.Cfg, config.DockerMCPName); err != nil {
		return fmt.Errorf("failed to disable docker MCP: %w", err)
	}

	if err := ws.Cfg.DisableDockerMCP(); err != nil {
		return err
	}

	return nil
}

// RefreshMCPTools refreshes the tools for a named MCP server.
func (b *Service) RefreshMCPTools(ctx context.Context, workspaceID, name string) error {
	ws, err := b.GetWorkspace(workspaceID)
	if err != nil {
		return err
	}
	mcptools.RefreshTools(ctx, ws.Cfg, name)
	return nil
}

// ReadMCPResource reads a resource from a named MCP server.
func (b *Service) ReadMCPResource(ctx context.Context, workspaceID, name, uri string) ([]MCPResourceContents, error) {
	ws, err := b.GetWorkspace(workspaceID)
	if err != nil {
		return nil, err
	}
	contents, err := mcptools.ReadResource(ctx, ws.Cfg, name, uri)
	if err != nil {
		return nil, err
	}
	result := make([]MCPResourceContents, len(contents))
	for i, c := range contents {
		result[i] = MCPResourceContents{
			URI:      c.URI,
			MIMEType: c.MIMEType,
			Text:     c.Text,
			Blob:     c.Blob,
		}
	}
	return result, nil
}

// GetMCPPrompt retrieves a prompt from a named MCP server.
func (b *Service) GetMCPPrompt(workspaceID, clientID, promptID string, args map[string]string) (string, error) {
	ws, err := b.GetWorkspace(workspaceID)
	if err != nil {
		return "", err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	result, err := mcptools.GetPromptMessages(ctx, ws.Cfg, clientID, promptID, args)
	if err != nil {
		return "", err
	}
	return strings.Join(result, " "), nil
}

// GetWorkingDir returns the working directory for a workspace.
func (b *Service) GetWorkingDir(workspaceID string) (string, error) {
	ws, err := b.GetWorkspace(workspaceID)
	if err != nil {
		return "", err
	}
	return ws.Cfg.WorkingDir(), nil
}
