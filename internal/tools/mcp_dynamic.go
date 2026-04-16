package tools

import (
	"context"
	"strings"
)

type MCPToolListItem struct {
	Name        string         `json:"name"`
	Description string         `json:"description,omitempty"`
	InputSchema map[string]any `json:"inputSchema,omitempty"`
	Annotations map[string]any `json:"annotations,omitempty"`
	Meta        map[string]any `json:"_meta,omitempty"`
}

type MCPToolsListResult struct {
	Tools []MCPToolListItem `json:"tools"`
}

type MCPPromptArgument struct {
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
	Required    bool   `json:"required,omitempty"`
}

type MCPPromptListItem struct {
	Name        string              `json:"name"`
	Description string              `json:"description,omitempty"`
	Arguments   []MCPPromptArgument `json:"arguments,omitempty"`
}

type MCPPromptsListResult struct {
	Prompts []MCPPromptListItem `json:"prompts"`
}

type MCPResourceListItem struct {
	URI         string `json:"uri"`
	Name        string `json:"name,omitempty"`
	Title       string `json:"title,omitempty"`
	Description string `json:"description,omitempty"`
	MimeType    string `json:"mimeType,omitempty"`
}

type MCPResourcesListResult struct {
	Resources []MCPResourceListItem `json:"resources"`
}

type MCPPromptMessage struct {
	Role    string `json:"role,omitempty"`
	Content any    `json:"content,omitempty"`
}

type MCPPromptResult struct {
	Description string             `json:"description,omitempty"`
	Messages    []MCPPromptMessage `json:"messages,omitempty"`
	Meta        map[string]any     `json:"_meta,omitempty"`
}

type MCPResourceReadContent struct {
	URI         string `json:"uri,omitempty"`
	MimeType    string `json:"mimeType,omitempty"`
	Text        string `json:"text,omitempty"`
	BlobSavedTo string `json:"blobSavedTo,omitempty"`
}

type MCPResourceReadResult struct {
	Contents []MCPResourceReadContent `json:"contents,omitempty"`
}

type MCPAuthToolResult struct {
	Name                string            `json:"name"`
	Status              string            `json:"status"`
	AuthURL             string            `json:"authUrl,omitempty"`
	Message             string            `json:"message"`
	Scope               string            `json:"scope,omitempty"`
	ResourceMetadataURL string            `json:"resourceMetadataUrl,omitempty"`
	Challenge           map[string]string `json:"challenge,omitempty"`
}

type MCPResourceReader func(ctx context.Context, server, uri string) (MCPResourceReadResult, error)

func DiscoverMCPTools(server string, result MCPToolsListResult, caller MCPToolCaller) []Tool {
	discovered := make([]Tool, 0, len(result.Tools))
	for _, item := range result.Tools {
		discovered = append(discovered, newMCPToolFromListItem(server, item, caller, nil))
	}
	return discovered
}

func DiscoverMCPToolsWithContextualCaller(server string, result MCPToolsListResult, caller MCPContextualToolCaller) []Tool {
	discovered := make([]Tool, 0, len(result.Tools))
	for _, item := range result.Tools {
		discovered = append(discovered, newMCPToolFromListItem(server, item, nil, caller))
	}
	return discovered
}

func newMCPToolFromListItem(server string, item MCPToolListItem, caller MCPToolCaller, contextualCaller MCPContextualToolCaller) Tool {
	def := MCPToolDefinition{
		Server:      server,
		Name:        item.Name,
		Description: item.Description,
		InputSchema: deepCloneAnyMap(item.InputSchema),
		ReadOnly:    mcpBoolAnnotation(item.Annotations, "readOnlyHint"),
		Destructive: mcpBoolAnnotation(item.Annotations, "destructiveHint"),
	}
	if contextualCaller != nil {
		return NewMCPContextualTool(def, contextualCaller)
	}
	return NewMCPTool(def, caller)
}

func RegisterDiscoveredMCPTools(registry *Registry, server string, result MCPToolsListResult, caller MCPToolCaller) {
	if registry == nil {
		return
	}
	for _, tool := range DiscoverMCPTools(server, result, caller) {
		registry.Register(tool)
	}
}

func RegisterDiscoveredMCPToolsWithContextualCaller(registry *Registry, server string, result MCPToolsListResult, caller MCPContextualToolCaller) {
	if registry == nil {
		return
	}
	for _, tool := range DiscoverMCPToolsWithContextualCaller(server, result, caller) {
		registry.Register(tool)
	}
}

func mcpBoolAnnotation(values map[string]any, key string) bool {
	if values == nil {
		return false
	}
	value, ok := values[key]
	if !ok {
		return false
	}
	typed, ok := value.(bool)
	return ok && typed
}

func mcpStringAnnotation(values map[string]any, key string) string {
	if values == nil {
		return ""
	}
	value, ok := values[key]
	if !ok {
		return ""
	}
	text, ok := value.(string)
	if !ok {
		return ""
	}
	return strings.TrimSpace(text)
}
