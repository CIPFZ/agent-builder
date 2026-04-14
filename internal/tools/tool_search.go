package tools

import (
	"context"
	"strings"

	"myclaw/internal/permissions"
	"myclaw/internal/session"
)

type ToolSearchTool struct {
	registry *Registry
}

func NewToolSearchTool(registry *Registry) *ToolSearchTool {
	return &ToolSearchTool{registry: registry}
}

func (t *ToolSearchTool) Definition() Definition {
	return Definition{
		Name:        "tool.search",
		Description: "Search available and deferred tools by capability keywords.",
		AlwaysLoad:  true,
	}
}

func (t *ToolSearchTool) Invoke(_ context.Context, _ session.Session, input string) (string, error) {
	return t.InvokeWithPolicy(context.Background(), session.Session{}, input, permissions.Policy{})
}

func (t *ToolSearchTool) InvokeWithPolicy(_ context.Context, _ session.Session, input string, policy permissions.Policy) (string, error) {
	if t.registry == nil {
		return "", nil
	}
	results := t.registry.Search(input, SearchOptions{
		Policy:       policy,
		DeferredOnly: true,
		MaxResults:   5,
	})
	if len(results) == 0 {
		return "No matching tools found.", nil
	}
	lines := make([]string, 0, len(results))
	for _, def := range results {
		parts := []string{def.Name + ": " + def.Description}
		if def.SearchHint != "" {
			parts = append(parts, "search hint: "+def.SearchHint)
		}
		if def.ShouldDefer {
			parts = append(parts, "deferred")
		}
		if def.AlwaysLoad {
			parts = append(parts, "always-loaded")
		}
		lines = append(lines, strings.Join(parts, " [")+strings.Repeat("]", len(parts)-1))
	}
	return strings.Join(lines, "\n"), nil
}

func (t *ToolSearchTool) IsEnabled() bool {
	return toolSearchEnabledOptimistic()
}

func (t *ToolSearchTool) IsReadOnly(_ string) bool {
	return true
}

func (t *ToolSearchTool) IsDestructive(_ string) bool {
	return false
}

func (t *ToolSearchTool) ShouldDefer() bool {
	return false
}

func (t *ToolSearchTool) AlwaysLoad() bool {
	return true
}

func (t *ToolSearchTool) PromptDescription() string {
	return "Search available and deferred tools by capability keywords."
}

func (t *ToolSearchTool) SearchHint() string {
	return "find tool by capability"
}
