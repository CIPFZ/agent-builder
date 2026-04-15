package tools

import "strings"

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

type MCPPromptMessage struct {
	Role    string `json:"role,omitempty"`
	Content any    `json:"content,omitempty"`
}

type MCPPromptResult struct {
	Description string             `json:"description,omitempty"`
	Messages    []MCPPromptMessage `json:"messages,omitempty"`
	Meta        map[string]any     `json:"_meta,omitempty"`
}

func DiscoverMCPTools(server string, result MCPToolsListResult, caller MCPToolCaller) []Tool {
	discovered := make([]Tool, 0, len(result.Tools))
	for _, item := range result.Tools {
		discovered = append(discovered, NewMCPTool(MCPToolDefinition{
			Server:      server,
			Name:        item.Name,
			Description: item.Description,
			InputSchema: deepCloneAnyMap(item.InputSchema),
			ReadOnly:    mcpBoolAnnotation(item.Annotations, "readOnlyHint"),
			Destructive: mcpBoolAnnotation(item.Annotations, "destructiveHint"),
		}, caller))
	}
	return discovered
}

func RegisterDiscoveredMCPTools(registry *Registry, server string, result MCPToolsListResult, caller MCPToolCaller) {
	if registry == nil {
		return
	}
	for _, tool := range DiscoverMCPTools(server, result, caller) {
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
