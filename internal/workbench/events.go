package workbench

import (
	"context"

	mcptools "github.com/CIPFZ/agent-builder/internal/agent/tools/mcp"
	"github.com/CIPFZ/agent-builder/internal/app"
	"github.com/CIPFZ/agent-builder/internal/config"
	"github.com/CIPFZ/agent-builder/internal/pubsub"
)

// WarningEvent carries non-fatal runtime warnings for adapters that expose
// workspace events.
type WarningEvent struct {
	Message string
}

// SubscribeEvents returns a per-caller event channel for a workspace.
// Each caller receives all events; multiple callers do not compete.
func (b *Service) SubscribeEvents(ctx context.Context, workspaceID string) (<-chan pubsub.Event[any], error) {
	ws, err := b.GetWorkspace(workspaceID)
	if err != nil {
		return nil, err
	}

	return ws.Events(ctx), nil
}

// SubscribeRawEvents is retained for runtime callers while the runtime API
// settles on SubscribeEvents as the raw application event stream.
func (b *Service) SubscribeRawEvents(ctx context.Context, workspaceID string) (<-chan pubsub.Event[any], error) {
	return b.SubscribeEvents(ctx, workspaceID)
}

// GetLSPStates returns the state of all LSP clients.
func (b *Service) GetLSPStates(workspaceID string) (map[string]app.LSPClientInfo, error) {
	_, err := b.GetWorkspace(workspaceID)
	if err != nil {
		return nil, err
	}

	return app.GetLSPStates(), nil
}

// GetLSPDiagnostics returns diagnostics for a specific LSP client in
// the workspace.
func (b *Service) GetLSPDiagnostics(workspaceID, lspName string) (any, error) {
	ws, err := b.GetWorkspace(workspaceID)
	if err != nil {
		return nil, err
	}

	for name, client := range ws.LSPManager.Clients().Seq2() {
		if name == lspName {
			return client.GetDiagnostics(), nil
		}
	}

	return nil, ErrLSPClientNotFound
}

// GetWorkspaceConfig returns the workspace-level configuration.
func (b *Service) GetWorkspaceConfig(workspaceID string) (*config.Config, error) {
	ws, err := b.GetWorkspace(workspaceID)
	if err != nil {
		return nil, err
	}

	return ws.Cfg.Config(), nil
}

// GetWorkspaceProviders returns the configured providers for a
// workspace.
func (b *Service) GetWorkspaceProviders(workspaceID string) (any, error) {
	ws, err := b.GetWorkspace(workspaceID)
	if err != nil {
		return nil, err
	}

	providers, _ := config.Providers(ws.Cfg.Config())
	return providers, nil
}

// LSPStart starts an LSP server for the given path.
func (b *Service) LSPStart(ctx context.Context, workspaceID, path string) error {
	ws, err := b.GetWorkspace(workspaceID)
	if err != nil {
		return err
	}

	ws.LSPManager.Start(ctx, path)
	return nil
}

// LSPStopAll stops all LSP servers for a workspace.
func (b *Service) LSPStopAll(ctx context.Context, workspaceID string) error {
	ws, err := b.GetWorkspace(workspaceID)
	if err != nil {
		return err
	}

	ws.LSPManager.StopAll(ctx)
	return nil
}

// MCPGetStates returns the current state of all MCP clients.
func (b *Service) MCPGetStates(_ string) map[string]mcptools.ClientInfo {
	return mcptools.GetStates()
}

// MCPRefreshPrompts refreshes prompts for a named MCP client.
func (b *Service) MCPRefreshPrompts(ctx context.Context, _ string, name string) {
	mcptools.RefreshPrompts(ctx, name)
}

// MCPRefreshResources refreshes resources for a named MCP client.
func (b *Service) MCPRefreshResources(ctx context.Context, _ string, name string) {
	mcptools.RefreshResources(ctx, name)
}
