package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"myclaw/internal/permissions"
	"myclaw/internal/session"
)

const (
	LSPStateDiscovered = ExtensionStateDiscovered
	LSPStateConfigured = "configured"
	LSPStateStarting   = "starting"
	LSPStateActive     = ExtensionStateActive
	LSPStateDegraded   = ExtensionStateDegraded
	LSPStateFailed     = ExtensionStateFailed
	LSPStateDisabled   = ExtensionStateDisabled
	LSPStateStopped    = "stopped"

	LSPPermissionReadOnly = "read_only"
	LSPPermissionMutating = "mutating"
	LSPPermissionMixed    = "mixed"
)

type LSPServerConfig struct {
	Name                 string
	Source               string
	Version              string
	LanguageIDs          []string
	FilePatterns         []string
	Command              string
	Args                 []string
	Env                  map[string]string
	CWD                  string
	WorkspaceRoot        string
	Enabled              bool
	Capabilities         []string
	ReadOnlyCapabilities []string
	MutatingCapabilities []string
}

type LSPRequest struct {
	ToolName    string
	Server      string
	Path        string
	Query       string
	Symbol      string
	Line        int
	Column      int
	Input       map[string]any
	Session     session.Session
	Policy      permissions.Policy
	Servers     []LSPServerConfig
	Capability  string
	ReadOnly    bool
	Destructive bool
}

type LSPHandler interface {
	HandleLSPRequest(context.Context, LSPRequest) (ToolResult, error)
}

func NormalizeLSPServerConfig(config LSPServerConfig) LSPServerConfig {
	config.Name = strings.TrimSpace(config.Name)
	config.Source = strings.ToLower(strings.TrimSpace(config.Source))
	if config.Source == "" {
		config.Source = "lsp"
	}
	config.Version = strings.TrimSpace(config.Version)
	config.LanguageIDs = compactSortedLSPStrings(config.LanguageIDs)
	config.FilePatterns = compactSortedLSPStrings(config.FilePatterns)
	config.Command = strings.TrimSpace(config.Command)
	config.Args = compactTrimmedLSPStrings(config.Args)
	config.Env = normalizeLSPEnv(config.Env)
	config.CWD = strings.TrimSpace(config.CWD)
	config.WorkspaceRoot = strings.TrimSpace(config.WorkspaceRoot)
	config.Capabilities = compactSortedLSPStrings(config.Capabilities)
	config.ReadOnlyCapabilities = compactSortedLSPStrings(config.ReadOnlyCapabilities)
	config.MutatingCapabilities = compactSortedLSPStrings(config.MutatingCapabilities)
	if len(config.Capabilities) == 0 {
		config.Capabilities = compactSortedLSPStrings(append(append([]string(nil), config.ReadOnlyCapabilities...), config.MutatingCapabilities...))
	}
	return config
}

func NormalizeLSPServerConfigs(configs []LSPServerConfig) []LSPServerConfig {
	if len(configs) == 0 {
		return nil
	}
	byName := make(map[string]LSPServerConfig, len(configs))
	for _, config := range configs {
		config = NormalizeLSPServerConfig(config)
		if config.Name == "" {
			continue
		}
		byName[strings.ToLower(config.Name)] = config
	}
	out := make([]LSPServerConfig, 0, len(byName))
	for _, config := range byName {
		out = append(out, config)
	}
	sort.Slice(out, func(i, j int) bool {
		return strings.ToLower(out[i].Name) < strings.ToLower(out[j].Name)
	})
	return out
}

func NormalizeLSPState(state string) string {
	switch strings.ToLower(strings.TrimSpace(state)) {
	case LSPStateConfigured, ExtensionStateLoaded:
		return LSPStateConfigured
	case LSPStateStarting:
		return LSPStateStarting
	case LSPStateActive:
		return LSPStateActive
	case LSPStateDegraded:
		return LSPStateDegraded
	case LSPStateFailed:
		return LSPStateFailed
	case LSPStateDisabled:
		return LSPStateDisabled
	case LSPStateStopped, ExtensionStateUnloaded:
		return LSPStateStopped
	default:
		return LSPStateDiscovered
	}
}

func LSPPermissionClassification(config LSPServerConfig) string {
	config = NormalizeLSPServerConfig(config)
	hasReadOnly := len(config.ReadOnlyCapabilities) > 0
	hasMutating := len(config.MutatingCapabilities) > 0
	switch {
	case hasReadOnly && hasMutating:
		return LSPPermissionMixed
	case hasReadOnly:
		return LSPPermissionReadOnly
	case hasMutating:
		return LSPPermissionMutating
	default:
		return LSPPermissionReadOnly
	}
}

func NewLSPTools(handler LSPHandler, servers []LSPServerConfig) []Tool {
	normalized := enabledLSPServerConfigs(servers)
	return []Tool{
		newLSPTool("lsp_symbol_search", "Search symbols from configured LSP servers.", "symbol_search", handler, normalized),
		newLSPTool("lsp_definition", "Find a symbol definition from configured LSP servers.", "definition", handler, normalized),
		newLSPTool("lsp_references", "Find symbol references from configured LSP servers.", "references", handler, normalized),
		newLSPTool("lsp_diagnostics", "Read diagnostics from configured LSP servers.", "diagnostics", handler, normalized),
	}
}

func enabledLSPServerConfigs(servers []LSPServerConfig) []LSPServerConfig {
	normalized := NormalizeLSPServerConfigs(servers)
	out := make([]LSPServerConfig, 0, len(normalized))
	for _, server := range normalized {
		if !server.Enabled {
			continue
		}
		out = append(out, server)
	}
	return out
}

type lspTool struct {
	name        string
	description string
	capability  string
	handler     LSPHandler
	servers     []LSPServerConfig
}

func newLSPTool(name, description, capability string, handler LSPHandler, servers []LSPServerConfig) *lspTool {
	return &lspTool{
		name:        name,
		description: description,
		capability:  capability,
		handler:     handler,
		servers:     append([]LSPServerConfig(nil), servers...),
	}
}

func (t *lspTool) Definition() Definition {
	return Definition{
		Name:        t.name,
		Description: t.description,
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"server": map[string]any{"type": "string"},
				"path":   map[string]any{"type": "string"},
				"query":  map[string]any{"type": "string"},
				"symbol": map[string]any{"type": "string"},
				"line":   map[string]any{"type": "integer"},
				"column": map[string]any{"type": "integer"},
			},
		},
		Source:      "lsp",
		SearchHint:  "language server " + t.capability,
		Enabled:     len(t.servers) > 0,
		ReadOnly:    true,
		Destructive: false,
		ShouldDefer: true,
		AlwaysLoad:  true,
	}
}

func (t *lspTool) Invoke(ctx context.Context, sess session.Session, input string) (string, error) {
	var parsed map[string]any
	if strings.TrimSpace(input) != "" {
		if err := json.Unmarshal([]byte(input), &parsed); err != nil {
			return "", err
		}
	}
	result, err := t.InvokeWithContext(ctx, ToolUseContext{
		Session:     sess,
		ToolName:    t.name,
		Input:       input,
		InputObject: parsed,
	})
	if err != nil {
		return "", err
	}
	return result.Output, nil
}

func (t *lspTool) InvokeWithContext(ctx context.Context, toolCtx ToolUseContext) (ToolResult, error) {
	toolCtx = toolCtx.Normalized()
	if len(t.servers) == 0 {
		return ToolResult{}, fmt.Errorf("LSP runtime unavailable: no configured LSP servers")
	}
	if t.handler == nil {
		return ToolResult{}, fmt.Errorf("LSP runtime unavailable: no handler configured for %s", t.name)
	}
	request := LSPRequest{
		ToolName:    t.name,
		Server:      stringFromMap(toolCtx.InputObject, "server"),
		Path:        stringFromMap(toolCtx.InputObject, "path"),
		Query:       stringFromMap(toolCtx.InputObject, "query"),
		Symbol:      stringFromMap(toolCtx.InputObject, "symbol"),
		Line:        intFromMap(toolCtx.InputObject, "line"),
		Column:      intFromMap(toolCtx.InputObject, "column"),
		Input:       cloneAnyMap(toolCtx.InputObject),
		Session:     toolCtx.Session,
		Policy:      toolCtx.Policy,
		Servers:     append([]LSPServerConfig(nil), t.servers...),
		Capability:  t.capability,
		ReadOnly:    true,
		Destructive: false,
	}
	return t.handler.HandleLSPRequest(ctx, request)
}

func (t *lspTool) CheckPermissionsWithContext(_ context.Context, toolCtx ToolUseContext) (permissions.Decision, error) {
	return toolCtx.Policy.Evaluate(permissions.Request{
		ToolName:    t.name,
		Command:     stringFromMap(toolCtx.InputObject, "path"),
		WorkDir:     toolCtx.WorkDir,
		ReadOnly:    true,
		Destructive: false,
	}), nil
}

func (t *lspTool) IsEnabled() bool           { return len(t.servers) > 0 }
func (t *lspTool) IsReadOnly(string) bool    { return true }
func (t *lspTool) IsDestructive(string) bool { return false }
func (t *lspTool) ShouldDefer() bool         { return true }
func (t *lspTool) AlwaysLoad() bool          { return true }
func (t *lspTool) PromptDescription() string { return t.description }
func (t *lspTool) SearchHint() string        { return "language server " + t.capability }

func normalizeLSPEnv(input map[string]string) map[string]string {
	if len(input) == 0 {
		return nil
	}
	out := make(map[string]string, len(input))
	for key, value := range input {
		key = strings.TrimSpace(key)
		if key == "" {
			continue
		}
		out[key] = strings.TrimSpace(value)
	}
	return out
}

func compactSortedLSPStrings(values []string) []string {
	out := compactTrimmedLSPStrings(values)
	sort.Strings(out)
	return out
}

func compactTrimmedLSPStrings(values []string) []string {
	if len(values) == 0 {
		return nil
	}
	seen := make(map[string]struct{}, len(values))
	out := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		out = append(out, value)
	}
	return out
}

func stringFromMap(input map[string]any, key string) string {
	if input == nil {
		return ""
	}
	value, _ := input[key].(string)
	return strings.TrimSpace(value)
}

func intFromMap(input map[string]any, key string) int {
	if input == nil {
		return 0
	}
	switch value := input[key].(type) {
	case int:
		return value
	case float64:
		return int(value)
	default:
		return 0
	}
}
