package queryengine

import (
	"context"
	"fmt"
	"myclaw/internal/tools"
	"sort"
	"strings"
)

type MCPInventory struct {
	ServerCount   int
	ToolCount     int
	PromptCount   int
	ResourceCount int
	SkillCount    int
}

type MCPServerSnapshot struct {
	LifecycleType           string
	Name                    string
	Source                  string
	TransportType           string
	Endpoint                string
	Enabled                 bool
	Status                  string
	Version                 string
	Capabilities            []string
	LifecycleState          string
	LastError               string
	LastUpdated             string
	RecoveryBehavior        string
	Tools                   []string
	Prompts                 []string
	Resources               []string
	Skills                  []string
	AuthURL                 string
	AuthMessage             string
	AuthScope               string
	AuthResourceMetadataURL string
	AuthChallenge           map[string]string
	Error                   string
}

func registerConfiguredMCPTools(registry *tools.Registry, configured map[string]tools.MCPToolsListResult, caller tools.MCPToolCaller, contextualCaller tools.MCPContextualToolCaller) {
	if registry == nil || len(configured) == 0 {
		return
	}
	servers := make([]string, 0, len(configured))
	for server := range configured {
		servers = append(servers, server)
	}
	sort.Strings(servers)
	for _, server := range servers {
		if contextualCaller != nil {
			tools.RegisterDiscoveredMCPToolsWithContextualCaller(registry, server, configured[server], contextualCaller)
			continue
		}
		tools.RegisterDiscoveredMCPTools(registry, server, configured[server], caller)
	}
}

func prepareMCPConfig(cfg *Config, toolRegistry *tools.Registry) {
	if cfg.MCPOAuthStore == nil {
		cfg.MCPOAuthStore = tools.NewDefaultMCPOAuthStore()
	}
	if len(cfg.MCPNeedsAuth) > 0 {
		cfg.MCPNeedsAuth = cloneMCPAuthResults(cfg.MCPNeedsAuth)
	}
	if len(cfg.MCPFailures) > 0 {
		cfg.MCPFailures = cloneStringMap(cfg.MCPFailures)
	}
	if len(cfg.MCPClients) > 0 {
		discovered, err := tools.DiscoverMCPClientToolsWithOAuth(context.Background(), cfg.MCPClients, cfg.MCPOAuthStore)
		if err == nil {
			mergeDiscoveredMCPConfig(cfg, discovered)
		}
	}
	registerMCPAuthTools(toolRegistry, cfg.MCPNeedsAuth)
}

func mergeDiscoveredMCPConfig(cfg *Config, discovered tools.MCPDiscoveryResult) {
	if cfg.MCPTools == nil {
		cfg.MCPTools = make(map[string]tools.MCPToolsListResult)
	}
	for server, result := range discovered.Tools {
		cfg.MCPTools[server] = result
		if cfg.MCPFailures != nil {
			delete(cfg.MCPFailures, server)
		}
	}
	if cfg.MCPPrompts == nil {
		cfg.MCPPrompts = make(map[string]tools.MCPPromptsListResult)
	}
	for server, result := range discovered.Prompts {
		cfg.MCPPrompts[server] = result
	}
	if cfg.MCPSkills == nil {
		cfg.MCPSkills = make(map[string][]tools.SkillCommand)
	}
	for server, skills := range discovered.Skills {
		cfg.MCPSkills[server] = append([]tools.SkillCommand(nil), skills...)
	}
	if cfg.MCPResources == nil {
		cfg.MCPResources = make(map[string][]tools.MCPResource)
	}
	for server, resources := range discovered.Resources {
		cfg.MCPResources[server] = append([]tools.MCPResource(nil), resources...)
	}
	if cfg.MCPNeedsAuth == nil {
		cfg.MCPNeedsAuth = make(map[string]tools.MCPAuthToolResult)
	}
	for server := range discovered.Tools {
		delete(cfg.MCPNeedsAuth, server)
	}
	for server, auth := range discovered.NeedsAuth {
		delete(cfg.MCPTools, server)
		delete(cfg.MCPPrompts, server)
		delete(cfg.MCPSkills, server)
		delete(cfg.MCPResources, server)
		if cfg.MCPFailures != nil {
			delete(cfg.MCPFailures, server)
		}
		cfg.MCPNeedsAuth[server] = cloneMCPAuthResult(auth)
	}
	if len(discovered.Failures) > 0 {
		if cfg.MCPFailures == nil {
			cfg.MCPFailures = make(map[string]string)
		}
		for server, message := range discovered.Failures {
			if _, ok := cfg.MCPNeedsAuth[server]; ok {
				continue
			}
			cfg.MCPFailures[server] = strings.TrimSpace(message)
		}
	}
	if cfg.MCPToolCaller == nil {
		cfg.MCPToolCaller = discovered.Caller
	}
	if cfg.MCPContextualToolCaller == nil {
		cfg.MCPContextualToolCaller = discovered.ContextualCaller
	}
	if cfg.MCPPromptCaller == nil {
		cfg.MCPPromptCaller = discovered.PromptCaller
	}
	if cfg.MCPResourceReader == nil {
		cfg.MCPResourceReader = discovered.ResourceReader
	}
	if cfg.MCPResourceLister == nil {
		cfg.MCPResourceLister = discovered.ResourceLister
	}
	if cfg.MCPReconnect == nil {
		cfg.MCPReconnect = discovered.Reconnect
	}
}

func registerMCPAuthTools(registry *tools.Registry, auth map[string]tools.MCPAuthToolResult) {
	if registry == nil || len(auth) == 0 {
		return
	}
	servers := make([]string, 0, len(auth))
	for server := range auth {
		servers = append(servers, server)
	}
	sort.Strings(servers)
	for _, server := range servers {
		result := auth[server]
		registry.Register(tools.NewMCPAuthToolFromResult(server, result))
	}
}

func (q *QueryEngine) MCPInventory() MCPInventory {
	q.toolContextMu.Lock()
	defer q.toolContextMu.Unlock()

	servers := make(map[string]struct{})
	for _, client := range q.mcpClients {
		if name := strings.TrimSpace(client.Name); name != "" {
			servers[name] = struct{}{}
		}
	}
	for server := range q.mcpTools {
		if strings.TrimSpace(server) != "" {
			servers[server] = struct{}{}
		}
	}
	for server := range q.mcpPrompts {
		if strings.TrimSpace(server) != "" {
			servers[server] = struct{}{}
		}
	}
	for server := range q.mcpResources {
		if strings.TrimSpace(server) != "" {
			servers[server] = struct{}{}
		}
	}
	for server := range q.mcpSkills {
		if strings.TrimSpace(server) != "" {
			servers[server] = struct{}{}
		}
	}
	for server := range q.mcpNeedsAuth {
		if strings.TrimSpace(server) != "" {
			servers[server] = struct{}{}
		}
	}
	for server := range q.mcpFailures {
		if strings.TrimSpace(server) != "" {
			servers[server] = struct{}{}
		}
	}

	inventory := MCPInventory{ServerCount: len(servers)}
	for _, result := range q.mcpTools {
		inventory.ToolCount += len(result.Tools)
	}
	for _, result := range q.mcpPrompts {
		inventory.PromptCount += len(result.Prompts)
	}
	for _, resources := range q.mcpResources {
		inventory.ResourceCount += len(resources)
	}
	for _, skills := range q.mcpSkills {
		inventory.SkillCount += len(skills)
	}
	return inventory
}

func (q *QueryEngine) MCPServers() []MCPServerSnapshot {
	q.toolContextMu.Lock()

	index := make(map[string]*MCPServerSnapshot)
	for _, client := range q.mcpClients {
		name := strings.TrimSpace(client.Name)
		if name == "" {
			continue
		}
		server := ensureMCPServerSnapshot(index, name)
		server.TransportType = strings.TrimSpace(client.Type)
		server.Endpoint = describeMCPEndpoint(client)
		server.Enabled = true
		server.Status = "configured"
	}
	for serverName, resources := range q.mcpResources {
		server := ensureMCPServerSnapshot(index, serverName)
		if server.Status != "needs-auth" {
			server.Status = "connected"
		}
		for _, resource := range resources {
			label := strings.TrimSpace(resource.URI)
			if label == "" {
				label = strings.TrimSpace(resource.Name)
			}
			server.Resources = append(server.Resources, label)
		}
	}
	for serverName, result := range q.mcpTools {
		server := ensureMCPServerSnapshot(index, serverName)
		if server.Status != "needs-auth" {
			server.Status = "connected"
		}
		for _, tool := range result.Tools {
			server.Tools = append(server.Tools, strings.TrimSpace(tool.Name))
		}
	}
	for serverName, result := range q.mcpPrompts {
		server := ensureMCPServerSnapshot(index, serverName)
		if server.Status != "needs-auth" {
			server.Status = "connected"
		}
		for _, prompt := range result.Prompts {
			server.Prompts = append(server.Prompts, strings.TrimSpace(prompt.Name))
		}
	}
	for serverName, skills := range q.mcpSkills {
		server := ensureMCPServerSnapshot(index, serverName)
		if server.Status != "needs-auth" && server.Status != "error" {
			server.Status = "connected"
		}
		for _, skill := range skills {
			server.Skills = append(server.Skills, mcpSkillSnapshotLabel(skill))
		}
	}
	for serverName, message := range q.mcpFailures {
		server := ensureMCPServerSnapshot(index, serverName)
		server.Status = "error"
		server.Error = strings.TrimSpace(message)
	}
	for serverName, auth := range q.mcpNeedsAuth {
		server := ensureMCPServerSnapshot(index, serverName)
		server.Status = strings.TrimSpace(auth.Status)
		if server.Status == "" {
			server.Status = "needs-auth"
		}
		server.AuthURL = strings.TrimSpace(auth.AuthURL)
		server.AuthMessage = strings.TrimSpace(auth.Message)
		server.AuthScope = strings.TrimSpace(auth.Scope)
		server.AuthResourceMetadataURL = strings.TrimSpace(auth.ResourceMetadataURL)
		server.AuthChallenge = cloneStringMap(auth.Challenge)
	}
	for _, record := range q.extensionLifecycle {
		record = tools.NormalizeExtensionLifecycleRecord(record)
		if record.Type != tools.ExtensionTypeMCPServer || record.Source != "mcp" || record.Name == "" {
			continue
		}
		server := ensureMCPServerSnapshot(index, record.Name)
		if strings.TrimSpace(server.Status) == "" {
			switch record.State {
			case tools.ExtensionStateFailed:
				server.Status = "error"
			case tools.ExtensionStateDegraded:
				server.Status = "degraded"
			default:
				server.Status = "configured"
			}
		}
		if strings.TrimSpace(server.Error) == "" {
			server.Error = record.LastError
		}
	}

	servers := make([]MCPServerSnapshot, 0, len(index))
	for _, server := range index {
		server.Tools = compactAndSortStrings(server.Tools)
		server.Prompts = compactAndSortStrings(server.Prompts)
		server.Resources = compactAndSortStrings(server.Resources)
		server.Skills = compactAndSortStrings(server.Skills)
		if strings.TrimSpace(server.Status) == "" {
			server.Status = "configured"
		}
		if !server.Enabled {
			server.Enabled = true
		}
		server.LifecycleType = tools.ExtensionTypeMCPServer
		server.Source = "mcp"
		server.Capabilities = mcpServerCapabilities(*server)
		servers = append(servers, *server)
	}
	q.toolContextMu.Unlock()
	for i := range servers {
		servers[i] = q.applyMCPServerLifecycle(servers[i])
	}
	sort.Slice(servers, func(i, j int) bool {
		return servers[i].Name < servers[j].Name
	})
	return servers
}

func (q *QueryEngine) ReconnectMCP(ctx context.Context, server string) (MCPServerSnapshot, error) {
	server = strings.TrimSpace(server)
	if server == "" {
		return MCPServerSnapshot{}, fmt.Errorf("MCP reconnect requires server name")
	}
	if _, err := q.reconnectMCPServer(ctx, server); err != nil {
		return MCPServerSnapshot{}, err
	}
	return q.mcpServerSnapshot(server)
}

func (q *QueryEngine) AuthenticateMCP(ctx context.Context, server string) (tools.MCPAuthStartResult, MCPServerSnapshot, error) {
	server = strings.TrimSpace(server)
	if server == "" {
		return tools.MCPAuthStartResult{}, MCPServerSnapshot{}, fmt.Errorf("MCP authenticate requires server name")
	}
	connection, ok := q.mcpClient(server)
	if !ok {
		return tools.MCPAuthStartResult{}, MCPServerSnapshot{}, fmt.Errorf("MCP server %q not configured", server)
	}
	authenticator := q.mcpAuthenticator
	if authenticator == nil && q.mcpOAuthStore != nil {
		authenticator = tools.NewDefaultMCPOAuthAuthenticator(q.mcpOAuthStore)
	}
	if authenticator == nil {
		return tools.MCPAuthStartResult{}, MCPServerSnapshot{}, fmt.Errorf("MCP authentication is not configured for %q", server)
	}
	connection = q.enrichMCPConnectionWithAuthState(server, connection)
	result, err := authenticator(ctx, server, connection)
	if err != nil {
		if auth, ok := tools.MCPAuthToolResultFromError(server, err); ok {
			q.applyMCPAuthState(server, auth)
		}
		return tools.MCPAuthStartResult{}, MCPServerSnapshot{}, err
	}
	if strings.TrimSpace(result.Status) != "" && toolsMCPAuthCompleteStatus(result.Status) {
		if _, err := q.reconnectMCPServer(ctx, server); err != nil {
			return result, MCPServerSnapshot{}, err
		}
	} else {
		q.applyMCPAuthState(server, tools.MCPAuthToolResult{
			Name:                tools.BuildMCPToolName(server, "authenticate"),
			AuthURL:             strings.TrimSpace(result.AuthURL),
			Message:             strings.TrimSpace(result.Message),
			Scope:               strings.TrimSpace(result.Scope),
			ResourceMetadataURL: strings.TrimSpace(result.ResourceMetadataURL),
			Challenge:           cloneStringMap(result.Challenge),
		})
		if result.Completion != nil {
			q.startMCPAuthCompletionContinuation(server, result.Completion)
		}
	}
	snapshot, snapshotErr := q.mcpServerSnapshot(server)
	return result, snapshot, snapshotErr
}

func (q *QueryEngine) mcpServerSnapshot(server string) (MCPServerSnapshot, error) {
	server = strings.TrimSpace(server)
	for _, snapshot := range q.MCPServers() {
		if snapshot.Name == server {
			return snapshot, nil
		}
	}
	return MCPServerSnapshot{}, fmt.Errorf("MCP server %q not found", server)
}

func (q *QueryEngine) markMCPServerNeedsAuth(toolName string, err error) {
	server, ok := q.mcpServerFromToolName(toolName)
	if !ok {
		return
	}
	auth, ok := tools.MCPAuthToolResultFromError(server, err)
	if !ok {
		return
	}
	q.applyMCPAuthState(server, auth)
}

func (q *QueryEngine) mcpServerFromToolName(toolName string) (string, bool) {
	server, _, ok := q.mcpToolTarget(toolName)
	return server, ok
}

func (q *QueryEngine) mcpToolTarget(toolName string) (string, string, bool) {
	toolName = strings.TrimSpace(toolName)
	if !strings.HasPrefix(toolName, "mcp__") {
		return "", "", false
	}
	for _, server := range q.mcpServerNames() {
		prefix := tools.BuildMCPToolName(server, "")
		if !strings.HasPrefix(toolName, prefix) {
			continue
		}
		return server, strings.TrimPrefix(toolName, prefix), true
	}
	rest := strings.TrimPrefix(toolName, "mcp__")
	server, name, ok := strings.Cut(rest, "__")
	if !ok || strings.TrimSpace(server) == "" || strings.TrimSpace(name) == "" {
		return "", "", false
	}
	return server, name, true
}

func (q *QueryEngine) mcpServerNames() []string {
	q.toolContextMu.Lock()
	defer q.toolContextMu.Unlock()
	seen := make(map[string]struct{})
	names := make([]string, 0, len(q.mcpClients)+len(q.mcpTools)+len(q.mcpPrompts)+len(q.mcpResources)+len(q.mcpNeedsAuth))
	add := func(name string) {
		name = strings.TrimSpace(name)
		if name == "" {
			return
		}
		if _, ok := seen[name]; ok {
			return
		}
		seen[name] = struct{}{}
		names = append(names, name)
	}
	for _, client := range q.mcpClients {
		add(client.Name)
	}
	for name := range q.mcpTools {
		add(name)
	}
	for name := range q.mcpPrompts {
		add(name)
	}
	for name := range q.mcpResources {
		add(name)
	}
	for name := range q.mcpSkills {
		add(name)
	}
	for name := range q.mcpNeedsAuth {
		add(name)
	}
	sort.Strings(names)
	return names
}

func cloneMCPResources(resources map[string][]tools.MCPResource) map[string][]tools.MCPResource {
	if resources == nil {
		return nil
	}
	out := make(map[string][]tools.MCPResource, len(resources))
	for name, items := range resources {
		out[name] = append([]tools.MCPResource(nil), items...)
	}
	return out
}

func cloneMCPTools(configured map[string]tools.MCPToolsListResult) map[string]tools.MCPToolsListResult {
	if configured == nil {
		return nil
	}
	out := make(map[string]tools.MCPToolsListResult, len(configured))
	for server, result := range configured {
		out[server] = tools.MCPToolsListResult{
			Tools: append([]tools.MCPToolListItem(nil), result.Tools...),
		}
	}
	return out
}

func cloneMCPPrompts(prompts map[string]tools.MCPPromptsListResult) map[string]tools.MCPPromptsListResult {
	if prompts == nil {
		return nil
	}
	out := make(map[string]tools.MCPPromptsListResult, len(prompts))
	for server, result := range prompts {
		cloned := tools.MCPPromptsListResult{
			Prompts: append([]tools.MCPPromptListItem(nil), result.Prompts...),
		}
		for i := range cloned.Prompts {
			cloned.Prompts[i].Arguments = append([]tools.MCPPromptArgument(nil), cloned.Prompts[i].Arguments...)
		}
		out[server] = cloned
	}
	return out
}

func cloneMCPSkills(skills map[string][]tools.SkillCommand) map[string][]tools.SkillCommand {
	if skills == nil {
		return nil
	}
	out := make(map[string][]tools.SkillCommand, len(skills))
	for server, items := range skills {
		out[server] = append([]tools.SkillCommand(nil), items...)
	}
	return out
}

func cloneMCPAuthResults(values map[string]tools.MCPAuthToolResult) map[string]tools.MCPAuthToolResult {
	if values == nil {
		return nil
	}
	out := make(map[string]tools.MCPAuthToolResult, len(values))
	for server, auth := range values {
		out[server] = cloneMCPAuthResult(auth)
	}
	return out
}

func cloneMCPAuthResult(value tools.MCPAuthToolResult) tools.MCPAuthToolResult {
	value.Challenge = cloneStringMap(value.Challenge)
	return value
}

func mcpAuthAppState(values map[string]tools.MCPAuthToolResult) map[string]any {
	if len(values) == 0 {
		return nil
	}
	out := make(map[string]any, len(values))
	for server, auth := range values {
		out[server] = map[string]any{
			"status":              auth.Status,
			"authUrl":             auth.AuthURL,
			"message":             auth.Message,
			"scope":               auth.Scope,
			"resourceMetadataUrl": auth.ResourceMetadataURL,
			"challenge":           cloneStringMap(auth.Challenge),
		}
	}
	return out
}

func (q *QueryEngine) reconnectMCPServer(ctx context.Context, server string) (tools.MCPReconnectResult, error) {
	if q.mcpReconnect == nil {
		return tools.MCPReconnectResult{}, fmt.Errorf("MCP reconnect is not configured")
	}
	result, err := q.mcpReconnect(ctx, server)
	if err != nil {
		if auth, ok := tools.MCPAuthToolResultFromError(server, err); ok {
			q.applyMCPAuthState(server, auth)
		} else {
			q.applyMCPFailureState(server, err.Error())
		}
		return tools.MCPReconnectResult{}, err
	}
	q.applyMCPReconnectResult(server, result)
	return result, nil
}

func (q *QueryEngine) applyMCPReconnectResult(server string, result tools.MCPReconnectResult) {
	server = strings.TrimSpace(server)
	if server == "" {
		server = strings.TrimSpace(result.Client.Name)
	}
	if server == "" {
		return
	}
	client := result.Client
	if strings.TrimSpace(client.Name) == "" {
		if existing, ok := q.mcpClient(server); ok {
			client = existing
		}
		client.Name = server
	}
	q.toolContextMu.Lock()
	q.setMCPClientLocked(server, client)
	if q.mcpResources == nil {
		q.mcpResources = make(map[string][]tools.MCPResource)
	}
	q.mcpResources[server] = append([]tools.MCPResource(nil), result.Resources...)
	if q.mcpPrompts == nil {
		q.mcpPrompts = make(map[string]tools.MCPPromptsListResult)
	}
	q.mcpPrompts[server] = cloneSingleMCPPromptResult(result.Prompts)
	if q.mcpSkills == nil {
		q.mcpSkills = make(map[string][]tools.SkillCommand)
	}
	q.mcpSkills[server] = append([]tools.SkillCommand(nil), result.Skills...)
	if q.mcpTools == nil {
		q.mcpTools = make(map[string]tools.MCPToolsListResult)
	}
	q.mcpTools[server] = cloneSingleMCPToolsResult(result.Tools)
	if q.mcpNeedsAuth != nil {
		delete(q.mcpNeedsAuth, server)
	}
	if q.mcpFailures != nil {
		delete(q.mcpFailures, server)
	}
	q.toolContextMu.Unlock()

	q.unregisterMCPServerTools(server)
	if q.mcpContextualToolCaller != nil {
		tools.RegisterDiscoveredMCPToolsWithContextualCaller(q.tools, server, result.Tools, q.mcpContextualToolCaller)
	} else {
		tools.RegisterDiscoveredMCPTools(q.tools, server, result.Tools, q.mcpToolCaller)
	}
}

func (q *QueryEngine) applyMCPAuthState(server string, auth tools.MCPAuthToolResult) {
	server = strings.TrimSpace(server)
	if server == "" {
		return
	}
	q.toolContextMu.Lock()
	existing := q.mcpNeedsAuth[server]
	if strings.TrimSpace(auth.Name) == "" {
		if strings.TrimSpace(existing.Name) != "" {
			auth.Name = existing.Name
		} else {
			auth.Name = tools.BuildMCPToolName(server, "authenticate")
		}
	}
	if strings.TrimSpace(auth.Status) == "" {
		auth.Status = strings.TrimSpace(existing.Status)
	}
	if strings.TrimSpace(auth.Status) == "" {
		auth.Status = "needs-auth"
	}
	if strings.TrimSpace(auth.AuthURL) == "" {
		auth.AuthURL = strings.TrimSpace(existing.AuthURL)
	}
	if strings.TrimSpace(auth.Message) == "" {
		auth.Message = strings.TrimSpace(existing.Message)
	}
	if strings.TrimSpace(auth.Scope) == "" {
		auth.Scope = strings.TrimSpace(existing.Scope)
	}
	if strings.TrimSpace(auth.ResourceMetadataURL) == "" {
		auth.ResourceMetadataURL = strings.TrimSpace(existing.ResourceMetadataURL)
	}
	if len(auth.Challenge) == 0 {
		auth.Challenge = cloneStringMap(existing.Challenge)
	}
	if q.mcpNeedsAuth == nil {
		q.mcpNeedsAuth = make(map[string]tools.MCPAuthToolResult)
	}
	q.mcpNeedsAuth[server] = cloneMCPAuthResult(auth)
	if q.mcpFailures != nil {
		delete(q.mcpFailures, server)
	}
	delete(q.mcpTools, server)
	delete(q.mcpPrompts, server)
	delete(q.mcpResources, server)
	delete(q.mcpSkills, server)
	q.toolContextMu.Unlock()

	q.unregisterMCPServerTools(server)
	q.tools.Register(tools.NewMCPAuthToolFromResult(server, auth))
}

func (q *QueryEngine) applyMCPFailureState(server, message string) {
	server = strings.TrimSpace(server)
	message = strings.TrimSpace(message)
	if server == "" || message == "" {
		return
	}
	q.toolContextMu.Lock()
	if q.mcpFailures == nil {
		q.mcpFailures = make(map[string]string)
	}
	q.mcpFailures[server] = message
	delete(q.mcpNeedsAuth, server)
	delete(q.mcpTools, server)
	delete(q.mcpPrompts, server)
	delete(q.mcpResources, server)
	delete(q.mcpSkills, server)
	q.toolContextMu.Unlock()

	q.unregisterMCPServerTools(server)
}

func (q *QueryEngine) unregisterMCPServerTools(server string) {
	q.tools.Unregister(tools.BuildMCPToolName(server, "authenticate"))
	prefix := tools.BuildMCPToolName(server, "")
	for _, def := range q.tools.Definitions() {
		if strings.HasPrefix(def.Name, prefix) {
			q.tools.Unregister(def.Name)
		}
	}
}

func (q *QueryEngine) mcpClient(server string) (tools.MCPConnection, bool) {
	server = strings.TrimSpace(server)
	q.toolContextMu.Lock()
	defer q.toolContextMu.Unlock()
	for _, client := range q.mcpClients {
		if strings.TrimSpace(client.Name) == server {
			return client, true
		}
	}
	return tools.MCPConnection{}, false
}

func (q *QueryEngine) enrichMCPConnectionWithAuthState(server string, connection tools.MCPConnection) tools.MCPConnection {
	if q.mcpOAuthStore != nil {
		connection = tools.EnrichMCPConnectionWithOAuthStore(q.mcpOAuthStore, server, connection)
	}
	q.toolContextMu.Lock()
	auth, ok := q.mcpNeedsAuth[server]
	q.toolContextMu.Unlock()
	if !ok {
		return connection
	}
	if strings.TrimSpace(connection.AuthURL) == "" {
		connection.AuthURL = strings.TrimSpace(auth.AuthURL)
	}
	if strings.TrimSpace(connection.AuthScope) == "" {
		connection.AuthScope = strings.TrimSpace(auth.Scope)
	}
	if strings.TrimSpace(connection.AuthResourceMetadataURL) == "" {
		connection.AuthResourceMetadataURL = strings.TrimSpace(auth.ResourceMetadataURL)
	}
	if len(connection.AuthChallenge) == 0 {
		connection.AuthChallenge = cloneStringMap(auth.Challenge)
	}
	return connection
}

func (q *QueryEngine) startMCPAuthCompletionContinuation(server string, completion <-chan tools.MCPAuthCompletionResult) {
	if completion == nil {
		return
	}
	go func() {
		completed, ok := <-completion
		if !ok {
			return
		}
		if completed.Error != nil {
			q.applyMCPAuthState(server, tools.MCPAuthToolResult{
				Name:    tools.BuildMCPToolName(server, "authenticate"),
				Status:  "error",
				Message: completed.Error.Error(),
			})
			return
		}
		status := strings.TrimSpace(completed.Status)
		if status != "" && !toolsMCPAuthCompleteStatus(status) {
			q.applyMCPAuthState(server, tools.MCPAuthToolResult{
				Name:    tools.BuildMCPToolName(server, "authenticate"),
				Status:  status,
				Message: strings.TrimSpace(completed.Message),
			})
			return
		}
		_, _ = q.reconnectMCPServer(context.Background(), server)
	}()
}

func (q *QueryEngine) setMCPClientLocked(server string, client tools.MCPConnection) {
	server = strings.TrimSpace(server)
	client.Name = server
	for index, existing := range q.mcpClients {
		if strings.TrimSpace(existing.Name) != server {
			continue
		}
		q.mcpClients[index] = client
		return
	}
	q.mcpClients = append(q.mcpClients, client)
}

func cloneSingleMCPToolsResult(result tools.MCPToolsListResult) tools.MCPToolsListResult {
	return cloneMCPTools(map[string]tools.MCPToolsListResult{"server": result})["server"]
}

func cloneSingleMCPPromptResult(result tools.MCPPromptsListResult) tools.MCPPromptsListResult {
	return cloneMCPPrompts(map[string]tools.MCPPromptsListResult{"server": result})["server"]
}

func mcpSkillSnapshotLabel(skill tools.SkillCommand) string {
	for _, value := range []string{
		strings.TrimSpace(skill.DisplayName),
		strings.TrimSpace(skill.Name),
	} {
		if value != "" {
			return value
		}
	}
	return ""
}

func ensureMCPServerSnapshot(index map[string]*MCPServerSnapshot, name string) *MCPServerSnapshot {
	name = strings.TrimSpace(name)
	server, ok := index[name]
	if ok {
		return server
	}
	server = &MCPServerSnapshot{Name: name}
	index[name] = server
	return server
}

func describeMCPEndpoint(client tools.MCPConnection) string {
	for _, value := range []string{
		strings.TrimSpace(client.URL),
		strings.TrimSpace(client.BaseURL),
		strings.TrimSpace(client.Command),
	} {
		if value == "" {
			continue
		}
		if value == client.Command && len(client.Args) > 0 {
			return strings.TrimSpace(value + " " + strings.Join(client.Args, " "))
		}
		return value
	}
	return ""
}

func toolsMCPAuthCompleteStatus(status string) bool {
	switch strings.ToLower(strings.TrimSpace(status)) {
	case "authenticated", "complete", "completed", "success", "reconnected":
		return true
	default:
		return false
	}
}
