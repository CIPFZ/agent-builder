package queryengine

import (
	"context"
	"fmt"
	"strings"

	"myclaw/internal/tools"
)

func registerConfiguredLSPTools(registry *tools.Registry, handler tools.LSPHandler, servers []tools.LSPServerConfig) {
	if registry == nil || len(servers) == 0 {
		return
	}
	for _, tool := range tools.NewLSPTools(handler, servers) {
		registry.Register(tool)
	}
}

func (q *QueryEngine) lspBoundaries() []ExtensionBoundary {
	if q == nil || len(q.lspServers) == 0 {
		return defaultLSPBoundaries()
	}
	out := make([]ExtensionBoundary, 0, len(q.lspServers))
	for _, server := range q.lspServers {
		server = tools.NormalizeLSPServerConfig(server)
		if server.Name == "" {
			continue
		}
		state := tools.LSPStateConfigured
		status := tools.LSPStateConfigured
		notes := "Configured LSP runtime boundary; external LSP process startup is deferred."
		recovery := tools.ExtensionRecoveryRebuildFromDiscovery
		if !server.Enabled {
			state = tools.ExtensionStateDisabled
			status = tools.ExtensionStateDisabled
			notes = "Configured LSP server is disabled by config."
			recovery = tools.ExtensionRecoveryRebuildFromDiscovery
		} else if q.lspHandler != nil {
			state = tools.ExtensionStateActive
			status = tools.ExtensionStateActive
			notes = "Configured LSP server has a runtime handler."
		}
		item := ExtensionBoundary{
			LifecycleType:            tools.ExtensionTypeLSPBoundary,
			Name:                     server.Name,
			Kind:                     "lsp",
			Source:                   server.Source,
			Status:                   status,
			Phase:                    "P2.2",
			Notes:                    notes,
			Capabilities:             append([]string(nil), server.Capabilities...),
			LifecycleState:           state,
			RecoveryBehavior:         recovery,
			Version:                  server.Version,
			LanguageIDs:              append([]string(nil), server.LanguageIDs...),
			FilePatterns:             append([]string(nil), server.FilePatterns...),
			Command:                  lspCommandSummary(server),
			CWD:                      server.CWD,
			WorkspaceRoot:            server.WorkspaceRoot,
			Enabled:                  server.Enabled,
			ReadOnlyCapabilities:     append([]string(nil), server.ReadOnlyCapabilities...),
			MutatingCapabilities:     append([]string(nil), server.MutatingCapabilities...),
			PermissionClassification: tools.LSPPermissionClassification(server),
		}
		out = append(out, q.applyLSPBoundaryLifecycle(item))
	}
	return out
}

func (q *QueryEngine) applyLSPBoundaryLifecycle(item ExtensionBoundary) ExtensionBoundary {
	item.Source = strings.ToLower(strings.TrimSpace(item.Source))
	if item.Source == "" {
		item.Source = "lsp"
	}
	record, ok := q.lifecycleRecord(tools.ExtensionLifecycleRecord{
		Type:         tools.ExtensionTypeLSPBoundary,
		Source:       item.Source,
		Name:         item.Name,
		Version:      item.Version,
		State:        item.LifecycleState,
		Capabilities: item.Capabilities,
	})
	if !ok {
		return item
	}
	item.LifecycleState = tools.NormalizeLSPState(record.State)
	item.Status = item.LifecycleState
	item.LastError = record.LastError
	item.LastUpdated = lifecycleTimeString(record.LastUpdated)
	item.RecoveryBehavior = record.RecoveryBehavior
	if len(record.Capabilities) > 0 {
		item.Capabilities = record.Capabilities
	}
	if record.Version != "" {
		item.Version = record.Version
	}
	if item.LastError != "" {
		item.Notes = item.LastError
	}
	return item
}

func lspCommandSummary(server tools.LSPServerConfig) string {
	parts := make([]string, 0, 1+len(server.Args))
	if strings.TrimSpace(server.Command) != "" {
		parts = append(parts, strings.TrimSpace(server.Command))
	}
	parts = append(parts, server.Args...)
	return strings.Join(parts, " ")
}

func (q *QueryEngine) HandleLSPRequest(ctx context.Context, request tools.LSPRequest) (tools.ToolResult, error) {
	serverName := strings.TrimSpace(request.Server)
	if serverName == "" && len(q.lspServers) == 1 {
		serverName = q.lspServers[0].Name
	}
	if serverName == "" {
		return tools.ToolResult{}, fmt.Errorf("LSP runtime unavailable: server is required")
	}
	for _, boundary := range q.lspBoundaries() {
		if !strings.EqualFold(boundary.Name, serverName) {
			continue
		}
		switch boundary.LifecycleState {
		case tools.ExtensionStateDisabled:
			return tools.ToolResult{}, fmt.Errorf("LSP runtime unavailable: server %q is disabled by extension lifecycle state", boundary.Name)
		case tools.ExtensionStateFailed:
			return tools.ToolResult{}, fmt.Errorf("LSP runtime unavailable: server %q failed: %s", boundary.Name, boundary.LastError)
		case tools.ExtensionStateDegraded:
			return tools.ToolResult{}, fmt.Errorf("LSP runtime degraded: server %q: %s", boundary.Name, boundary.LastError)
		}
		break
	}
	if q.lspHandler == nil {
		return tools.ToolResult{}, fmt.Errorf("LSP runtime unavailable: no handler configured for %s", request.ToolName)
	}
	return q.lspHandler.HandleLSPRequest(ctx, request)
}
