package queryengine_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"myclaw/internal/llm"
	"myclaw/internal/permissions"
	"myclaw/internal/queryengine"
	"myclaw/internal/session"
	"myclaw/internal/tools"
	"myclaw/internal/workspace"
)

type lifecycleScriptedClient struct {
	mu       sync.Mutex
	scripts  [][]llm.StreamEvent
	requests []llm.GenerateRequest
}

func (c *lifecycleScriptedClient) Stream(ctx context.Context, req llm.GenerateRequest, handler llm.StreamHandler) error {
	c.mu.Lock()
	c.requests = append(c.requests, req)
	events := c.scripts[0]
	c.scripts = c.scripts[1:]
	c.mu.Unlock()
	for _, event := range events {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}
		if err := handler.OnEvent(event); err != nil {
			return err
		}
	}
	return nil
}

func (c *lifecycleScriptedClient) Requests() []llm.GenerateRequest {
	c.mu.Lock()
	defer c.mu.Unlock()
	return append([]llm.GenerateRequest(nil), c.requests...)
}

func TestQueryEngineRegistersConfiguredMCPToolsAndInvokesCaller(t *testing.T) {
	sessions := session.NewManager(nil)
	sess := sessions.GetOrCreateMain("main")
	msg, err := sessions.AppendMessage(sess.ID, "user", "read via mcp")
	if err != nil {
		t.Fatalf("append user message: %v", err)
	}
	client := &lifecycleScriptedClient{scripts: [][]llm.StreamEvent{{
		{Type: "tool.call", ToolName: "mcp__filesystem__read_file", ToolInput: `{"path":"README.md"}`, ToolUseID: "toolu-mcp"},
		{Type: "message.end"},
	}, {
		{Type: "text.delta", Delta: "done"},
		{Type: "message.end"},
	}}}
	var gotServer, gotName string
	var gotInput map[string]any
	engine := queryengine.New(queryengine.Config{
		Sessions:         sessions,
		Client:           client,
		WorkspaceLoader:  workspace.NewLoader(""),
		PermissionPolicy: permissions.Policy{Mode: permissions.ModeDangerFullAccess},
		MCPTools: map[string]tools.MCPToolsListResult{
			"filesystem": {Tools: []tools.MCPToolListItem{{
				Name:        "read_file",
				Description: "Read a file",
				InputSchema: map[string]any{
					"type": "object",
					"properties": map[string]any{
						"path": map[string]any{"type": "string"},
					},
				},
			}}},
		},
		MCPToolCaller: func(_ context.Context, server, name string, input map[string]any) (tools.MCPToolResult, error) {
			gotServer, gotName, gotInput = server, name, input
			return tools.MCPToolResult{Content: []map[string]any{{"type": "text", "text": "mcp file contents"}}}, nil
		},
	})
	if err := engine.SubmitMessage(context.Background(), sess, msg, &captureSink{}); err != nil {
		t.Fatalf("submit message: %v", err)
	}
	if gotServer != "filesystem" || gotName != "read_file" || gotInput["path"] != "README.md" {
		t.Fatalf("mcp call = server %q name %q input %#v", gotServer, gotName, gotInput)
	}
	requests := client.Requests()
	if len(requests) == 0 {
		t.Fatal("expected model request")
	}
	foundTool := false
	for _, def := range requests[0].Tools {
		if def.Name == "mcp__filesystem__read_file" {
			foundTool = true
			properties, _ := def.InputSchema["properties"].(map[string]any)
			if properties["path"] == nil {
				t.Fatalf("mcp tool schema = %#v, want MCP input schema", def.InputSchema)
			}
		}
	}
	if !foundTool {
		t.Fatalf("tools = %#v, want discovered MCP tool", requests[0].Tools)
	}
	assertToolMessageContains(t, sessions, sess.ID, "mcp file contents")
}

func TestQueryEngineDiscoversMCPClientToolsAndInvokesServer(t *testing.T) {
	var calledTool bool
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var request map[string]any
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			t.Fatalf("decode request: %v", err)
		}
		method, _ := request["method"].(string)
		response := map[string]any{"jsonrpc": "2.0", "id": request["id"]}
		switch method {
		case "initialize":
			response["result"] = map[string]any{"protocolVersion": "2024-11-05", "capabilities": map[string]any{}}
		case "notifications/initialized":
			w.WriteHeader(http.StatusAccepted)
			return
		case "tools/list":
			response["result"] = map[string]any{"tools": []any{map[string]any{
				"name":        "read_file",
				"description": "Read a file",
				"inputSchema": map[string]any{"type": "object", "properties": map[string]any{"path": map[string]any{"type": "string"}}},
			}}}
		case "tools/call":
			calledTool = true
			response["result"] = map[string]any{"content": []any{map[string]any{"type": "text", "text": "queryengine remote contents"}}}
		default:
			t.Fatalf("unexpected MCP method %q", method)
		}
		_ = json.NewEncoder(w).Encode(response)
	}))
	defer server.Close()

	sessions := session.NewManager(nil)
	sess := sessions.GetOrCreateMain("main")
	msg, err := sessions.AppendMessage(sess.ID, "user", "read via discovered mcp")
	if err != nil {
		t.Fatalf("append user message: %v", err)
	}
	client := &lifecycleScriptedClient{scripts: [][]llm.StreamEvent{{
		{Type: "tool.call", ToolName: "mcp__filesystem__read_file", ToolInput: `{"path":"README.md"}`, ToolUseID: "toolu-mcp"},
		{Type: "message.end"},
	}, {
		{Type: "text.delta", Delta: "done"},
		{Type: "message.end"},
	}}}
	engine := queryengine.New(queryengine.Config{
		Sessions:         sessions,
		Client:           client,
		WorkspaceLoader:  workspace.NewLoader(""),
		PermissionPolicy: permissions.Policy{Mode: permissions.ModeDangerFullAccess},
		MCPClients: []tools.MCPConnection{{
			Name:    "filesystem",
			Type:    "streamable_http",
			BaseURL: server.URL,
		}},
	})
	if err := engine.SubmitMessage(context.Background(), sess, msg, &captureSink{}); err != nil {
		t.Fatalf("submit message: %v", err)
	}
	if !calledTool {
		t.Fatal("expected MCP tools/call to be sent to discovered server")
	}
	assertToolMessageContains(t, sessions, sess.ID, "queryengine remote contents")
}

func TestQueryEngineDiscoversMCPClientPromptsAsSkillsAndInvokesServer(t *testing.T) {
	var gotPromptArgs map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var request map[string]any
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			t.Fatalf("decode request: %v", err)
		}
		method, _ := request["method"].(string)
		response := map[string]any{"jsonrpc": "2.0", "id": request["id"]}
		switch method {
		case "initialize":
			response["result"] = map[string]any{
				"protocolVersion": "2024-11-05",
				"capabilities": map[string]any{
					"prompts": map[string]any{},
					"tools":   map[string]any{},
				},
			}
		case "notifications/initialized":
			w.WriteHeader(http.StatusAccepted)
			return
		case "tools/list":
			response["result"] = map[string]any{"tools": []any{}}
		case "prompts/list":
			response["result"] = map[string]any{"prompts": []any{map[string]any{
				"name":        "review",
				"description": "Review a target",
				"arguments": []any{map[string]any{
					"name":        "target",
					"description": "Target to review",
					"required":    true,
				}},
			}}}
		case "prompts/get":
			params, _ := request["params"].(map[string]any)
			gotPromptArgs, _ = params["arguments"].(map[string]any)
			response["result"] = map[string]any{"messages": []any{map[string]any{
				"role": "user",
				"content": map[string]any{
					"type": "text",
					"text": "MCP prompt body for " + gotPromptArgs["target"].(string),
				},
			}}}
		default:
			t.Fatalf("unexpected MCP method %q", method)
		}
		_ = json.NewEncoder(w).Encode(response)
	}))
	defer server.Close()

	sessions := session.NewManager(nil)
	sess := sessions.GetOrCreateMain("main")
	msg, err := sessions.AppendMessage(sess.ID, "user", "invoke mcp skill")
	if err != nil {
		t.Fatalf("append user message: %v", err)
	}
	client := &lifecycleScriptedClient{scripts: [][]llm.StreamEvent{{
		{Type: "tool.call", ToolName: "Skill", ToolInput: `{"skill":"mcp__filesystem__review","args":"README.md"}`, ToolUseID: "toolu-skill"},
		{Type: "message.end"},
	}, {
		{Type: "text.delta", Delta: "done"},
		{Type: "message.end"},
	}}}
	engine := queryengine.New(queryengine.Config{
		Sessions:         sessions,
		Client:           client,
		WorkspaceLoader:  workspace.NewLoader(""),
		PermissionPolicy: permissions.Policy{Mode: permissions.ModeDangerFullAccess},
		MCPClients: []tools.MCPConnection{{
			Name:    "filesystem",
			Type:    "http",
			BaseURL: server.URL,
		}},
	})
	if err := engine.SubmitMessage(context.Background(), sess, msg, &captureSink{}); err != nil {
		t.Fatalf("submit message: %v", err)
	}
	if gotPromptArgs["target"] != "README.md" {
		t.Fatalf("prompt args = %#v, want target from skill args", gotPromptArgs)
	}
	assertToolMessageContains(t, sessions, sess.ID, "MCP prompt body for README.md")
}

func TestQueryEngineInjectsSkillForkExecutorIntoToolContext(t *testing.T) {
	sessions := session.NewManager(nil)
	sess := sessions.GetOrCreateMain("main")
	dir := t.TempDir()
	root := filepath.Join(dir, "skills")
	skillDir := filepath.Join(root, "verify")
	if err := os.MkdirAll(skillDir, 0o755); err != nil {
		t.Fatalf("mkdir skill: %v", err)
	}
	if err := os.WriteFile(filepath.Join(skillDir, "SKILL.md"), []byte("---\ncontext: fork\nagent: verifier\n---\nVerify it."), 0o644); err != nil {
		t.Fatalf("write skill: %v", err)
	}
	msg, err := sessions.AppendMessage(sess.ID, "user", "invoke skill")
	if err != nil {
		t.Fatalf("append user message: %v", err)
	}
	client := &lifecycleScriptedClient{scripts: [][]llm.StreamEvent{{
		{Type: "tool.call", ToolName: "Skill", ToolInput: `{"skill":"verify"}`, ToolUseID: "toolu-skill"},
		{Type: "message.end"},
	}, {
		{Type: "text.delta", Delta: "done"},
		{Type: "message.end"},
	}}}
	called := false
	engine := queryengine.New(queryengine.Config{
		Sessions:         sessions,
		Client:           client,
		WorkspaceLoader:  workspace.NewLoader(""),
		PermissionPolicy: permissions.Policy{Mode: permissions.ModeDangerFullAccess},
		SkillRoots:       []string{root},
		SkillForkExecutor: func(_ context.Context, request tools.SkillForkRequest) (tools.ToolResult, error) {
			called = true
			if request.Command.Name != "verify" || request.Command.Agent != "verifier" {
				t.Fatalf("request = %#v, want verify verifier skill", request)
			}
			return tools.ToolResult{Output: `{"status":"forked","agent":"verifier","result":"runtime executed"}`}, nil
		},
	})
	if err := engine.SubmitMessage(context.Background(), sess, msg, &captureSink{}); err != nil {
		t.Fatalf("submit message: %v", err)
	}
	if !called {
		t.Fatal("expected configured SkillForkExecutor to be called")
	}
	assertToolMessageContains(t, sessions, sess.ID, "runtime executed")
}

func assertToolMessageContains(t *testing.T, sessions *session.Manager, sessionID, want string) {
	t.Helper()
	messages, ok := sessions.Messages(sessionID)
	if !ok {
		t.Fatalf("messages for session %q not found", sessionID)
	}
	for _, message := range messages {
		if message.Role == "tool" && strings.Contains(message.Content, want) {
			return
		}
	}
	t.Fatalf("messages = %#v, want tool message containing %q", messages, want)
}
