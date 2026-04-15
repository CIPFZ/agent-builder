package tools_test

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"myclaw/internal/session"
	"myclaw/internal/tools"
)

func TestDiscoverMCPToolsBuildsClaudeNamedRegistryTools(t *testing.T) {
	schema := map[string]any{
		"type": "object",
		"properties": map[string]any{
			"path": map[string]any{"type": "string"},
		},
	}
	list := tools.MCPToolsListResult{
		Tools: []tools.MCPToolListItem{{
			Name:        "read_file",
			Description: "Read a file",
			InputSchema: schema,
		}},
	}

	discovered := tools.DiscoverMCPTools("filesystem", list, func(_ context.Context, server, name string, input map[string]any) (tools.MCPToolResult, error) {
		return tools.MCPToolResult{
			Content: []map[string]any{{"type": "text", "text": server + ":" + name + ":" + input["path"].(string)}},
		}, nil
	})

	if len(discovered) != 1 {
		t.Fatalf("discovered = %d, want 1", len(discovered))
	}

	def := discovered[0].Definition()
	if def.Name != "mcp__filesystem__read_file" {
		t.Fatalf("definition name = %q, want Claude MCP tool name", def.Name)
	}
	if def.Source != "mcp" {
		t.Fatalf("definition source = %q, want mcp", def.Source)
	}
	if def.InputSchema["properties"].(map[string]any)["path"] == nil {
		t.Fatalf("definition schema = %#v, want passthrough input schema", def.InputSchema)
	}

	schema["properties"].(map[string]any)["path"].(map[string]any)["type"] = "number"
	if def.InputSchema["properties"].(map[string]any)["path"].(map[string]any)["type"] != "string" {
		t.Fatalf("definition schema mutated with source schema, want clone")
	}
}

func TestRegisterDiscoveredMCPToolsPreservesStructuredResultPayload(t *testing.T) {
	registry := tools.NewRegistry()
	tools.RegisterDiscoveredMCPTools(registry, "filesystem", tools.MCPToolsListResult{
		Tools: []tools.MCPToolListItem{{
			Name:        "read_file",
			Description: "Read a file",
			InputSchema: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"path": map[string]any{"type": "string"},
				},
			},
		}},
	}, func(_ context.Context, server, name string, input map[string]any) (tools.MCPToolResult, error) {
		if server != "filesystem" || name != "read_file" || input["path"] != "README.md" {
			t.Fatalf("caller args = %q %q %#v", server, name, input)
		}
		return tools.MCPToolResult{
			Content: []map[string]any{{"type": "text", "text": "file contents"}},
			StructuredContent: map[string]any{
				"path": "README.md",
			},
			Meta: map[string]any{
				"source": "bridge",
			},
			IsError: true,
		}, nil
	})

	out, err := registry.Invoke(context.Background(), session.Session{}, "mcp__filesystem__read_file", `{"path":"README.md"}`)
	if err != nil {
		t.Fatalf("invoke: %v", err)
	}
	if !strings.Contains(out, "file contents") || !strings.Contains(out, `"structuredContent"`) || !strings.Contains(out, `"_meta"`) || !strings.Contains(out, `"isError":true`) {
		t.Fatalf("output = %q, want preserved MCP result payload", out)
	}

	var parsed map[string]any
	if err := json.Unmarshal([]byte(out), &parsed); err != nil {
		t.Fatalf("unmarshal output: %v", err)
	}
	if parsed["isError"] != true {
		t.Fatalf("parsed = %#v, want isError true preserved in payload", parsed)
	}
	if _, ok := parsed["structuredContent"].(map[string]any); !ok {
		t.Fatalf("parsed = %#v, want structuredContent preserved in payload", parsed)
	}
	if _, ok := parsed["_meta"].(map[string]any); !ok {
		t.Fatalf("parsed = %#v, want _meta preserved in payload", parsed)
	}
}
