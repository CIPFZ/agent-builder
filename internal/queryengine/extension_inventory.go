package queryengine

import (
	runtimecommands "myclaw/internal/commands"
	"myclaw/internal/session"
	"myclaw/internal/tools"
	"sort"
	"strings"
)

type ExtensionInventory struct {
	Summary              ExtensionInventorySummary
	Tools                []ExtensionTool
	Commands             []ExtensionCommand
	Skills               []ExtensionSkill
	MCPServers           []MCPServerSnapshot
	LSPBoundaries        []ExtensionBoundary
	DeferredCapabilities []string
}

type ExtensionInventorySummary struct {
	ToolCount        int
	CommandCount     int
	SkillCount       int
	MCPServerCount   int
	LSPBoundaryCount int
}

type ExtensionTool struct {
	Type             string
	Name             string
	Aliases          []string
	Description      string
	InputSchema      map[string]any
	Source           string
	Version          string
	Capabilities     []string
	SearchHint       string
	Enabled          bool
	ReadOnly         bool
	Destructive      bool
	ShouldDefer      bool
	AlwaysLoad       bool
	LifecycleState   string
	LastError        string
	LastUpdated      string
	RecoveryBehavior string
}

type ExtensionCommand struct {
	LifecycleType               string
	Type                        string
	Name                        string
	Aliases                     []string
	Description                 string
	ArgumentHint                string
	Category                    string
	Visibility                  string
	Behavior                    string
	Source                      string
	LoadedFrom                  string
	Version                     string
	Capabilities                []string
	LifecycleState              string
	LastError                   string
	LastUpdated                 string
	RecoveryBehavior            string
	HasUserSpecifiedDescription bool
	WhenToUse                   string
	DisableModelInvocation      bool
	UserInvocable               bool
	IsHidden                    bool
}

type ExtensionSkill = tools.SkillExtensionInventory

type ExtensionBoundary struct {
	LifecycleType            string
	Name                     string
	Kind                     string
	Source                   string
	Status                   string
	Phase                    string
	Notes                    string
	Capabilities             []string
	LifecycleState           string
	LastError                string
	LastUpdated              string
	RecoveryBehavior         string
	Version                  string
	LanguageIDs              []string
	FilePatterns             []string
	Command                  string
	CWD                      string
	WorkspaceRoot            string
	Enabled                  bool
	ReadOnlyCapabilities     []string
	MutatingCapabilities     []string
	PermissionClassification string
}

func (q *QueryEngine) ExtensionInventory(sessionID string) ExtensionInventory {
	if q == nil {
		return ExtensionInventory{
			LSPBoundaries:        defaultLSPBoundaries(),
			DeferredCapabilities: defaultDeferredExtensionCapabilities(),
		}
	}
	toolsProjection := q.extensionTools(sessionID)
	commands := q.extensionCommands(sessionID)
	skills := q.extensionSkills()
	servers := q.MCPServers()
	boundaries := q.lspBoundaries()
	deferred := defaultDeferredExtensionCapabilities()
	return ExtensionInventory{
		Summary: ExtensionInventorySummary{
			ToolCount:        len(toolsProjection),
			CommandCount:     len(commands),
			SkillCount:       len(skills),
			MCPServerCount:   len(servers),
			LSPBoundaryCount: len(boundaries),
		},
		Tools:                toolsProjection,
		Commands:             commands,
		Skills:               skills,
		MCPServers:           servers,
		LSPBoundaries:        boundaries,
		DeferredCapabilities: deferred,
	}
}

func (q *QueryEngine) RebuildExtensionInventory(sessionID string) ExtensionInventory {
	return q.ExtensionInventory(sessionID)
}

func (q *QueryEngine) extensionTools(sessionID string) []ExtensionTool {
	if q == nil || q.tools == nil {
		return nil
	}
	contracts := q.tools.Contracts(tools.ContractOptions{
		Policy:          q.PermissionPolicyForSession(sessionID),
		IncludeDeferred: true,
	})
	out := make([]ExtensionTool, 0, len(contracts))
	for _, contract := range contracts {
		item := ExtensionTool{
			Type:             tools.ExtensionTypeTool,
			Name:             strings.TrimSpace(contract.Name),
			Aliases:          compactAndSortStrings(contract.Aliases),
			Description:      strings.TrimSpace(contract.Description),
			InputSchema:      cloneAnyMap(contract.InputSchema),
			Source:           strings.ToLower(strings.TrimSpace(contract.Source)),
			Capabilities:     []string{"invoke"},
			SearchHint:       strings.TrimSpace(contract.SearchHint),
			Enabled:          contract.Enabled,
			ReadOnly:         contract.ReadOnly,
			Destructive:      contract.Destructive,
			ShouldDefer:      contract.ShouldDefer,
			AlwaysLoad:       contract.AlwaysLoad,
			LifecycleState:   tools.ExtensionStateActive,
			RecoveryBehavior: tools.ExtensionRecoveryRebuildFromDiscovery,
		}
		out = append(out, q.applyToolLifecycle(item))
	}
	sort.Slice(out, func(i, j int) bool {
		return strings.ToLower(out[i].Name) < strings.ToLower(out[j].Name)
	})
	return out
}

func (q *QueryEngine) extensionCommands(sessionID string) []ExtensionCommand {
	if q == nil {
		return nil
	}
	byName := make(map[string]ExtensionCommand)
	for _, command := range runtimecommands.NewDefaultRegistry().List(q.extensionCommandContext(sessionID)) {
		item := extensionCommandFromRuntimeMetadata(command)
		if item.Name == "" {
			continue
		}
		byName[item.Name] = item
	}
	for _, command := range q.commands {
		if strings.TrimSpace(command.Name) == "" {
			continue
		}
		item := extensionCommandFromConfigured(command)
		byName[item.Name] = item
	}
	out := make([]ExtensionCommand, 0, len(byName))
	for _, command := range byName {
		out = append(out, q.applyCommandLifecycle(command))
	}
	sort.Slice(out, func(i, j int) bool {
		return strings.ToLower(out[i].Name) < strings.ToLower(out[j].Name)
	})
	return out
}

func (q *QueryEngine) extensionCommandContext(sessionID string) runtimecommands.Context {
	if q == nil || q.sessions == nil {
		return defaultCommandContext(session.Session{})
	}
	if sess, ok := q.sessions.GetByID(sessionID); ok {
		return defaultCommandContext(sess)
	}
	return defaultCommandContext(session.Session{})
}

func extensionCommandFromRuntimeMetadata(command runtimecommands.Metadata) ExtensionCommand {
	return ExtensionCommand{
		LifecycleType:    tools.ExtensionTypeCommand,
		Type:             "slash",
		Name:             strings.TrimSpace(command.Name),
		Aliases:          compactAndSortStrings(command.Aliases),
		Description:      strings.TrimSpace(command.Description),
		ArgumentHint:     strings.TrimSpace(command.ArgumentHint),
		Category:         strings.TrimSpace(command.Category),
		Visibility:       string(command.Visibility),
		Behavior:         string(command.Behavior),
		Source:           "runtime",
		Capabilities:     []string{"invoke"},
		LifecycleState:   tools.ExtensionStateActive,
		RecoveryBehavior: tools.ExtensionRecoveryRebuildFromDiscovery,
		UserInvocable:    true,
	}
}

func extensionCommandFromConfigured(command tools.Command) ExtensionCommand {
	name := strings.TrimPrefix(strings.ToLower(strings.TrimSpace(command.Name)), "/")
	source := strings.TrimSpace(command.Source)
	if source == "" {
		source = "dynamic"
	}
	commandType := strings.TrimSpace(command.Type)
	if commandType == "" {
		commandType = "slash"
	}
	return ExtensionCommand{
		LifecycleType:               tools.ExtensionTypeCommand,
		Type:                        commandType,
		Name:                        name,
		Description:                 strings.TrimSpace(command.Description),
		Source:                      strings.ToLower(source),
		LoadedFrom:                  strings.TrimSpace(command.LoadedFrom),
		Version:                     strings.TrimSpace(command.Version),
		Capabilities:                []string{"invoke"},
		LifecycleState:              tools.ExtensionStateActive,
		RecoveryBehavior:            tools.ExtensionRecoveryRebuildFromDiscovery,
		HasUserSpecifiedDescription: command.HasUserSpecifiedDescription,
		WhenToUse:                   strings.TrimSpace(command.WhenToUse),
		DisableModelInvocation:      command.DisableModelInvocation,
		UserInvocable:               command.UserInvocable,
		IsHidden:                    command.IsHidden,
	}
}

func (q *QueryEngine) extensionSkills() []ExtensionSkill {
	if q == nil {
		return nil
	}
	q.toolContextMu.Lock()
	mcpSkills := cloneMCPSkills(q.mcpSkills)
	q.toolContextMu.Unlock()

	out := make([]ExtensionSkill, 0)
	seenSkills := make(map[string]struct{})
	add := func(skills []tools.SkillCommand, source string) {
		for _, skill := range skills {
			item := tools.SkillExtensionInventoryItem(skill, source)
			if item.Name == "" {
				continue
			}
			if _, exists := seenSkills[item.Name]; exists {
				continue
			}
			seenSkills[item.Name] = struct{}{}
			out = append(out, q.applySkillLifecycle(item))
		}
	}
	add(tools.GetBundledSkills(), "bundled")
	add(tools.GetBuiltinPluginSkillCommands(), "plugin")
	add(tools.GetDynamicSkills(), "dynamic")
	servers := make([]string, 0, len(mcpSkills))
	for server := range mcpSkills {
		servers = append(servers, server)
	}
	sort.Strings(servers)
	for _, server := range servers {
		skills := append([]tools.SkillCommand(nil), mcpSkills[server]...)
		for i := range skills {
			if strings.TrimSpace(skills[i].MCPServer) == "" {
				skills[i].MCPServer = server
			}
			if strings.TrimSpace(skills[i].Source) == "" {
				skills[i].Source = "mcp"
			}
		}
		add(skills, "mcp")
	}
	sort.Slice(out, func(i, j int) bool {
		left := strings.ToLower(out[i].Name)
		right := strings.ToLower(out[j].Name)
		if left != right {
			return left < right
		}
		return strings.ToLower(out[i].Source) < strings.ToLower(out[j].Source)
	})
	return out
}

func defaultLSPBoundaries() []ExtensionBoundary {
	return []ExtensionBoundary{{
		LifecycleType:    tools.ExtensionTypeLSPBoundary,
		Name:             "language-server-protocol",
		Kind:             "lsp",
		Source:           "lsp",
		Status:           "deferred",
		Phase:            "P2/P3",
		Notes:            "Schema boundary only; full LSP lifecycle is deferred.",
		Capabilities:     []string{"placeholder"},
		LifecycleState:   tools.ExtensionStateDiscovered,
		RecoveryBehavior: tools.ExtensionRecoveryUnsupported,
	}}
}

func defaultDeferredExtensionCapabilities() []string {
	return []string{"plugin_marketplace", "remote_extension_lifecycle"}
}
