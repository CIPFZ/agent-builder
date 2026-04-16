package queryengine_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
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

func TestQueryEngineMarksMCPServerNeedsAuthAfterToolCallUnauthorized(t *testing.T) {
	var callCount int
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var request map[string]any
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			t.Fatalf("decode request: %v", err)
		}
		method, _ := request["method"].(string)
		switch method {
		case "initialize":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"jsonrpc": "2.0",
				"id":      request["id"],
				"result": map[string]any{
					"protocolVersion": "2024-11-05",
					"capabilities":    map[string]any{"tools": map[string]any{}},
				},
			})
		case "notifications/initialized":
			w.WriteHeader(http.StatusNoContent)
		case "tools/list":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"jsonrpc": "2.0",
				"id":      request["id"],
				"result": map[string]any{"tools": []map[string]any{{
					"name":        "read_file",
					"description": "Read file",
					"inputSchema": map[string]any{"type": "object"},
				}}},
			})
		case "tools/call":
			callCount++
			w.Header().Set("WWW-Authenticate", `Bearer authorization_uri="https://auth.example/authorize"`)
			w.WriteHeader(http.StatusUnauthorized)
			_, _ = w.Write([]byte(`{"jsonrpc":"2.0","error":{"code":401,"message":"Unauthorized"}}`))
		default:
			t.Fatalf("unexpected method %q", method)
		}
	}))
	defer server.Close()

	sessions := session.NewManager(nil)
	sess := sessions.GetOrCreateMain("main")
	msg, err := sessions.AppendMessage(sess.ID, "user", "read via expired mcp")
	if err != nil {
		t.Fatalf("append user message: %v", err)
	}
	client := &lifecycleScriptedClient{scripts: [][]llm.StreamEvent{{
		{Type: "tool.call", ToolName: "mcp__filesystem__read_file", ToolInput: `{"path":"README.md"}`, ToolUseID: "toolu-mcp"},
		{Type: "message.end"},
	}, {
		{Type: "text.delta", Delta: "needs auth"},
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
	if callCount != 1 {
		t.Fatalf("callCount = %d, want one failed MCP tool call", callCount)
	}
	requests := client.Requests()
	if len(requests) < 2 {
		t.Fatalf("requests = %d, want second model request after auth failure", len(requests))
	}
	if !hasToolDefinition(requests[0].Tools, "mcp__filesystem__read_file") {
		t.Fatalf("first request tools = %#v, want real MCP tool before auth failure", requests[0].Tools)
	}
	if !hasToolDefinition(requests[1].Tools, "mcp__filesystem__authenticate") {
		t.Fatalf("second request tools = %#v, want authenticate pseudo tool after 401", requests[1].Tools)
	}
	if hasToolDefinition(requests[1].Tools, "mcp__filesystem__read_file") {
		t.Fatalf("second request tools = %#v, real MCP tools should be hidden until auth reconnect", requests[1].Tools)
	}
}

func TestQueryEngineMCPAuthToolReturnsAuthURLBeforeRefreshingToolSurface(t *testing.T) {
	sessions := session.NewManager(nil)
	sess := sessions.GetOrCreateMain("main")
	msg, err := sessions.AppendMessage(sess.ID, "user", "authenticate mcp")
	if err != nil {
		t.Fatalf("append user message: %v", err)
	}
	var authenticated bool
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !authenticated {
			w.Header().Set("WWW-Authenticate", `Bearer resource_metadata="https://auth.example/.well-known/oauth-protected-resource"`)
			w.WriteHeader(http.StatusUnauthorized)
			_, _ = w.Write([]byte("authenticate"))
			return
		}
		var request map[string]any
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			t.Fatalf("decode request: %v", err)
		}
		method, _ := request["method"].(string)
		switch method {
		case "initialize":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"jsonrpc": "2.0",
				"id":      request["id"],
				"result": map[string]any{
					"protocolVersion": "2024-11-05",
					"capabilities":    map[string]any{"tools": map[string]any{}, "resources": map[string]any{}},
				},
			})
		case "notifications/initialized":
			w.WriteHeader(http.StatusNoContent)
		case "tools/list":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"jsonrpc": "2.0",
				"id":      request["id"],
				"result": map[string]any{"tools": []map[string]any{{
					"name":        "read_file",
					"description": "Read file",
					"inputSchema": map[string]any{"type": "object"},
				}}},
			})
		case "resources/list":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"jsonrpc": "2.0",
				"id":      request["id"],
				"result":  map[string]any{"resources": []map[string]any{{"uri": "file:///README.md", "name": "README"}}},
			})
		default:
			t.Fatalf("unexpected method %q", method)
		}
	}))
	defer server.Close()

	client := &lifecycleScriptedClient{scripts: [][]llm.StreamEvent{{
		{Type: "tool.call", ToolName: "mcp__filesystem__authenticate", ToolInput: `{}`, ToolUseID: "toolu-auth"},
		{Type: "message.end"},
	}, {
		{Type: "text.delta", Delta: "authenticated"},
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
		MCPAuthenticator: func(context.Context, string, tools.MCPConnection) (tools.MCPAuthStartResult, error) {
			authenticated = true
			return tools.MCPAuthStartResult{Status: "auth_url", AuthURL: "https://auth.example/authorize"}, nil
		},
	})
	if err := engine.SubmitMessage(context.Background(), sess, msg, &captureSink{}); err != nil {
		t.Fatalf("submit message: %v", err)
	}
	requests := client.Requests()
	if len(requests) < 2 {
		t.Fatalf("requests = %d, want second model request after auth tool", len(requests))
	}
	if !hasToolDefinition(requests[0].Tools, "mcp__filesystem__authenticate") {
		t.Fatalf("first request tools = %#v, want authenticate pseudo tool", requests[0].Tools)
	}
	if !hasToolDefinition(requests[1].Tools, "mcp__filesystem__authenticate") {
		t.Fatalf("second request tools = %#v, want authenticate pseudo tool until OAuth completion reconnect", requests[1].Tools)
	}
	if hasToolDefinition(requests[1].Tools, "mcp__filesystem__read_file") {
		t.Fatalf("second request tools = %#v, real MCP tools must wait for OAuth completion reconnect", requests[1].Tools)
	}
}

func hasToolDefinition(defs []llm.ToolDefinition, name string) bool {
	for _, def := range defs {
		if def.Name == name {
			return true
		}
	}
	return false
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
	assertSkillMessageContains(t, sessions, sess.ID, "MCP prompt body for README.md")
}

func TestQueryEngineAppendsSkillNewMessagesAfterToolResultAndContinuesWithContext(t *testing.T) {
	sessions := session.NewManager(nil)
	sess := sessions.GetOrCreateMain("main")
	dir := t.TempDir()
	root := filepath.Join(dir, "skills")
	skillDir := filepath.Join(root, "review")
	if err := os.MkdirAll(skillDir, 0o755); err != nil {
		t.Fatalf("mkdir skill: %v", err)
	}
	if err := os.WriteFile(filepath.Join(skillDir, "SKILL.md"), []byte("---\nallowed-tools: Read\nmodel: claude-sonnet-4-5\n---\nReview skill body."), 0o644); err != nil {
		t.Fatalf("write skill: %v", err)
	}
	msg, err := sessions.AppendMessage(sess.ID, "user", "invoke skill")
	if err != nil {
		t.Fatalf("append user message: %v", err)
	}
	client := &lifecycleScriptedClient{scripts: [][]llm.StreamEvent{{
		{Type: "tool.call", ToolName: "Skill", ToolInput: `{"skill":"review","args":"README.md"}`, ToolUseID: "toolu-skill"},
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
		ToolRegistry:     tools.NewRegistry(tools.NewSkillTool()),
		SkillRoots:       []string{root},
	})
	if err := engine.SubmitMessage(context.Background(), sess, msg, &captureSink{}); err != nil {
		t.Fatalf("submit message: %v", err)
	}
	messages, ok := sessions.Messages(sess.ID)
	if !ok {
		t.Fatalf("messages for session %q not found", sess.ID)
	}
	var toolIndex, skillIndex = -1, -1
	for i, message := range messages {
		if message.Role == "tool" && strings.Contains(message.Content, `"commandName":"review"`) {
			toolIndex = i
		}
		if message.Role == "user" && message.Subtype == "skill" && message.IsMeta {
			skillIndex = i
			if !strings.Contains(message.Content, "Review skill body.") || message.LogicalParentID != "toolu-skill" {
				t.Fatalf("skill message = %#v, want tagged injected skill body", message)
			}
		}
	}
	if toolIndex < 0 || skillIndex < 0 || skillIndex <= toolIndex {
		t.Fatalf("messages = %#v, want skill meta message after tool result", messages)
	}
	requests := client.Requests()
	if len(requests) != 2 {
		t.Fatalf("request count = %d, want model continuation after skill", len(requests))
	}
	foundSkillContext := false
	for _, history := range requests[1].History {
		if history.Role == "user" && history.Subtype == "skill" && strings.Contains(history.Content, "Review skill body.") {
			foundSkillContext = true
			break
		}
	}
	if !foundSkillContext {
		t.Fatalf("second request history = %#v, want injected skill context", requests[1].History)
	}
}

func TestQueryEngineAppliesInlineSkillContextModifierToPolicyAndModel(t *testing.T) {
	sessions := session.NewManager(nil)
	sess := sessions.GetOrCreateMain("main")
	dir := t.TempDir()
	root := filepath.Join(dir, "skills")
	skillDir := filepath.Join(root, "review")
	if err := os.MkdirAll(skillDir, 0o755); err != nil {
		t.Fatalf("mkdir skill: %v", err)
	}
	if err := os.WriteFile(filepath.Join(skillDir, "SKILL.md"), []byte("---\nallowed-tools: system.run\nmodel: haiku\neffort: high\n---\nReview skill body."), 0o644); err != nil {
		t.Fatalf("write skill: %v", err)
	}
	msg, err := sessions.AppendMessage(sess.ID, "user", "invoke skill")
	if err != nil {
		t.Fatalf("append user message: %v", err)
	}
	client := &lifecycleScriptedClient{scripts: [][]llm.StreamEvent{{
		{Type: "tool.call", ToolName: "Skill", ToolInput: `{"skill":"review","args":"README.md"}`, ToolUseID: "toolu-skill"},
		{Type: "message.end"},
	}, {
		{Type: "text.delta", Delta: "done"},
		{Type: "message.end"},
	}}}
	engine := queryengine.New(queryengine.Config{
		Sessions:         sessions,
		Client:           client,
		WorkspaceLoader:  workspace.NewLoader(""),
		PermissionPolicy: permissions.Policy{Mode: permissions.ModeAsk},
		ToolRegistry:     tools.NewRegistry(tools.NewSkillTool()),
		SkillRoots:       []string{root},
		MainLoopModel:    "sonnet",
	})
	if err := engine.SubmitMessage(context.Background(), sess, msg, &captureSink{}); err != nil {
		t.Fatalf("submit message: %v", err)
	}
	requests := client.Requests()
	if len(requests) < 2 {
		t.Fatalf("request count = %d, want continuation request after skill", len(requests))
	}
	if requests[1].Model != "claude-haiku-4-5" {
		t.Fatalf("continuation model = %q, want skill model override", requests[1].Model)
	}
	policy := engine.PermissionPolicyForSession(sess.ID)
	decision := policy.Evaluate(permissions.Request{ToolName: "system.run", Command: "echo ok", Destructive: true})
	if !decision.Allowed || decision.RuleSource != string(permissions.RuleSourceCommand) {
		t.Fatalf("decision = %#v, want skill allowed-tools command rule", decision)
	}
	refreshed, ok := sessions.GetByID(sess.ID)
	if !ok {
		t.Fatal("session missing")
	}
	if refreshed.Metadata.MainLoopModelOverride != "haiku" || refreshed.Metadata.MainLoopEffortOverride != "high" {
		t.Fatalf("metadata = %#v, want skill model and effort overrides", refreshed.Metadata)
	}
}

func TestQueryEngineInjectsSkillListingOnceAndThenOnlyNewSkills(t *testing.T) {
	tools.ClearDynamicSkills()
	t.Cleanup(tools.ClearDynamicSkills)

	dir := t.TempDir()
	root := filepath.Join(dir, "skills")
	writeSkill := func(name, description string) {
		t.Helper()
		skillDir := filepath.Join(root, name)
		if err := os.MkdirAll(skillDir, 0o755); err != nil {
			t.Fatalf("mkdir skill %q: %v", name, err)
		}
		if err := os.WriteFile(filepath.Join(skillDir, "SKILL.md"), []byte("---\ndescription: "+description+"\n---\nBody."), 0o644); err != nil {
			t.Fatalf("write skill %q: %v", name, err)
		}
	}
	writeSkill("alpha", "alpha skill")
	tools.AddSkillDirectories([]string{root})

	sessions := session.NewManager(nil)
	sess := sessions.GetOrCreateMain("main")
	client := &lifecycleScriptedClient{scripts: [][]llm.StreamEvent{
		{{Type: "text.delta", Delta: "one"}, {Type: "message.end"}},
		{{Type: "text.delta", Delta: "two"}, {Type: "message.end"}},
		{{Type: "text.delta", Delta: "three"}, {Type: "message.end"}},
	}}
	engine := queryengine.New(queryengine.Config{
		Sessions:         sessions,
		Client:           client,
		WorkspaceLoader:  workspace.NewLoader(""),
		PermissionPolicy: permissions.Policy{Mode: permissions.ModeDangerFullAccess},
		ToolRegistry:     tools.NewRegistry(tools.NewSkillTool()),
		SkillRoots:       []string{root},
	})

	msg1, err := sessions.AppendMessage(sess.ID, "user", "turn one")
	if err != nil {
		t.Fatalf("append turn one: %v", err)
	}
	if err := engine.SubmitMessage(context.Background(), sess, msg1, &captureSink{}); err != nil {
		t.Fatalf("submit turn one: %v", err)
	}
	assertSkillListingPayloads(t, engine.Messages(sess.ID), []string{"alpha"})

	msg2, err := sessions.AppendMessage(sess.ID, "user", "turn two")
	if err != nil {
		t.Fatalf("append turn two: %v", err)
	}
	if err := engine.SubmitMessage(context.Background(), sess, msg2, &captureSink{}); err != nil {
		t.Fatalf("submit turn two: %v", err)
	}
	assertSkillListingPayloads(t, engine.Messages(sess.ID), []string{"alpha"})

	writeSkill("beta", "beta skill")
	tools.AddSkillDirectories([]string{root})
	msg3, err := sessions.AppendMessage(sess.ID, "user", "turn three")
	if err != nil {
		t.Fatalf("append turn three: %v", err)
	}
	if err := engine.SubmitMessage(context.Background(), sess, msg3, &captureSink{}); err != nil {
		t.Fatalf("submit turn three: %v", err)
	}
	assertSkillListingPayloads(t, engine.Messages(sess.ID), []string{"alpha", "beta"})
}

func TestQueryEngineSkillListingIncludesMCPPromptSkills(t *testing.T) {
	tools.ClearDynamicSkills()
	t.Cleanup(tools.ClearDynamicSkills)

	sessions := session.NewManager(nil)
	sess := sessions.GetOrCreateMain("main")
	client := &lifecycleScriptedClient{scripts: [][]llm.StreamEvent{{
		{Type: "text.delta", Delta: "listed"},
		{Type: "message.end"},
	}}}
	engine := queryengine.New(queryengine.Config{
		Sessions:         sessions,
		Client:           client,
		WorkspaceLoader:  workspace.NewLoader(""),
		PermissionPolicy: permissions.Policy{Mode: permissions.ModeDangerFullAccess},
		ToolRegistry:     tools.NewRegistry(tools.NewSkillTool()),
		MCPPrompts: map[string]tools.MCPPromptsListResult{
			"docs": {Prompts: []tools.MCPPromptListItem{{
				Name:        "summarize",
				Description: "Summarize a document through MCP",
				Arguments:   []tools.MCPPromptArgument{{Name: "path"}},
			}}},
		},
	})
	msg, err := sessions.AppendMessage(sess.ID, "user", "what skills are available")
	if err != nil {
		t.Fatalf("append user: %v", err)
	}
	if err := engine.SubmitMessage(context.Background(), sess, msg, &captureSink{}); err != nil {
		t.Fatalf("submit: %v", err)
	}
	assertSkillListingPayloads(t, engine.Messages(sess.ID), []string{"mcp__docs__summarize"})
}

func TestQueryEngineInjectsDynamicSkillAttachmentForMentionedFileSkillDir(t *testing.T) {
	tools.ClearDynamicSkills()
	t.Cleanup(tools.ClearDynamicSkills)

	root := t.TempDir()
	nested := filepath.Join(root, "pkg", "feature")
	skillDir := filepath.Join(nested, ".claude", "skills", "reviewer")
	if err := os.MkdirAll(skillDir, 0o755); err != nil {
		t.Fatalf("mkdir skill: %v", err)
	}
	if err := os.WriteFile(filepath.Join(skillDir, "SKILL.md"), []byte("---\ndescription: Review feature files\n---\nReview feature."), 0o644); err != nil {
		t.Fatalf("write skill: %v", err)
	}
	if err := os.WriteFile(filepath.Join(nested, "file.go"), []byte("package feature"), 0o644); err != nil {
		t.Fatalf("write file: %v", err)
	}

	sessions := session.NewManager(nil)
	sess := sessions.GetOrCreateMain("main")
	client := &lifecycleScriptedClient{scripts: [][]llm.StreamEvent{{
		{Type: "text.delta", Delta: "dynamic"},
		{Type: "message.end"},
	}}}
	engine := queryengine.New(queryengine.Config{
		Sessions:         sessions,
		Client:           client,
		WorkspaceLoader:  workspace.NewLoader(root),
		PermissionPolicy: permissions.Policy{Mode: permissions.ModeDangerFullAccess},
		ToolRegistry:     tools.NewRegistry(tools.NewSkillTool()),
	})
	msg, err := sessions.AppendMessage(sess.ID, "user", "inspect @pkg/feature/file.go")
	if err != nil {
		t.Fatalf("append user: %v", err)
	}
	if err := engine.SubmitMessage(context.Background(), sess, msg, &captureSink{}); err != nil {
		t.Fatalf("submit: %v", err)
	}
	assertDynamicSkillAttachment(t, engine.Messages(sess.ID), filepath.Join(nested, ".claude", "skills"), []string{"reviewer"})
	assertSkillListingPayloads(t, engine.Messages(sess.ID), []string{"reviewer"})
}

func TestQueryEngineSuppressesSkillListingWhenTranscriptAlreadyHasListing(t *testing.T) {
	tools.ClearDynamicSkills()
	t.Cleanup(tools.ClearDynamicSkills)

	dir := t.TempDir()
	root := filepath.Join(dir, "skills")
	skillDir := filepath.Join(root, "alpha")
	if err := os.MkdirAll(skillDir, 0o755); err != nil {
		t.Fatalf("mkdir skill: %v", err)
	}
	if err := os.WriteFile(filepath.Join(skillDir, "SKILL.md"), []byte("---\ndescription: alpha skill\n---\nBody."), 0o644); err != nil {
		t.Fatalf("write skill: %v", err)
	}
	tools.AddSkillDirectories([]string{root})

	sessions := session.NewManager(nil)
	sess := sessions.GetOrCreateMain("main")
	existingListing := tools.BuildSkillListingAttachmentMessage("listing-existing", sess.ID, tools.GetDynamicSkills(), 0, true)
	if _, err := sessions.AppendModelMessage(sess.ID, existingListing); err != nil {
		t.Fatalf("append existing listing: %v", err)
	}
	msg, err := sessions.AppendMessage(sess.ID, "user", "resume")
	if err != nil {
		t.Fatalf("append user: %v", err)
	}
	client := &lifecycleScriptedClient{scripts: [][]llm.StreamEvent{{
		{Type: "text.delta", Delta: "resumed"},
		{Type: "message.end"},
	}}}
	engine := queryengine.New(queryengine.Config{
		Sessions:         sessions,
		Client:           client,
		WorkspaceLoader:  workspace.NewLoader(""),
		PermissionPolicy: permissions.Policy{Mode: permissions.ModeDangerFullAccess},
		ToolRegistry:     tools.NewRegistry(tools.NewSkillTool()),
		SkillRoots:       []string{root},
	})
	if err := engine.SubmitMessage(context.Background(), sess, msg, &captureSink{}); err != nil {
		t.Fatalf("submit resumed message: %v", err)
	}
	count := 0
	for _, message := range engine.Messages(sess.ID) {
		if message.Role == "attachment" && message.Subtype == "skill_listing" {
			count++
		}
	}
	if count != 1 {
		t.Fatalf("skill_listing count = %d, want existing listing only", count)
	}
}

func assertSkillListingPayloads(t *testing.T, messages []session.Message, wantNames []string) {
	t.Helper()
	gotNames := make([]string, 0)
	for _, message := range messages {
		if message.Role != "attachment" || message.Subtype != "skill_listing" {
			continue
		}
		var payload struct {
			Type       string `json:"type"`
			Content    string `json:"content"`
			SkillCount int    `json:"skillCount"`
			IsInitial  bool   `json:"isInitial"`
		}
		if err := json.Unmarshal([]byte(message.Content), &payload); err != nil {
			t.Fatalf("skill listing payload: %v", err)
		}
		if payload.Type != "skill_listing" || payload.SkillCount != 1 {
			t.Fatalf("payload = %#v, want one skill_listing entry per injection", payload)
		}
		for _, name := range wantNames {
			if strings.Contains(payload.Content, "- "+name+":") {
				gotNames = append(gotNames, name)
			}
		}
	}
	if !reflect.DeepEqual(gotNames, wantNames) {
		t.Fatalf("skill listing names = %#v, want %#v in messages %#v", gotNames, wantNames, messages)
	}
}

func assertDynamicSkillAttachment(t *testing.T, messages []session.Message, wantDir string, wantNames []string) {
	t.Helper()
	for _, message := range messages {
		if message.Role != "attachment" || message.Subtype != "dynamic_skill" {
			continue
		}
		var payload struct {
			Type        string   `json:"type"`
			SkillDir    string   `json:"skillDir"`
			SkillNames  []string `json:"skillNames"`
			DisplayPath string   `json:"displayPath"`
		}
		if err := json.Unmarshal([]byte(message.Content), &payload); err != nil {
			t.Fatalf("dynamic skill payload: %v", err)
		}
		if payload.Type == "dynamic_skill" && filepath.Clean(payload.SkillDir) == filepath.Clean(wantDir) && reflect.DeepEqual(payload.SkillNames, wantNames) && payload.DisplayPath != "" {
			return
		}
	}
	t.Fatalf("messages = %#v, want dynamic_skill attachment for %q names %#v", messages, wantDir, wantNames)
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
	if err := os.WriteFile(filepath.Join(skillDir, "SKILL.md"), []byte("---\ncontext: fork\nagent: verifier\nallowed-tools: Read,Grep\nmodel: claude-sonnet-4-5\neffort: high\n---\nVerify it."), 0o644); err != nil {
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
			if !reflect.DeepEqual(request.Command.AllowedTools, []string{"Read", "Grep"}) ||
				request.Command.Model != "claude-sonnet-4-5" ||
				request.Command.Effort != "high" ||
				request.Command.Context != "fork" {
				t.Fatalf("request command = %#v, want skill frontmatter propagated", request.Command)
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

func assertSkillMessageContains(t *testing.T, sessions *session.Manager, sessionID, want string) {
	t.Helper()
	messages, ok := sessions.Messages(sessionID)
	if !ok {
		t.Fatalf("messages for session %q not found", sessionID)
	}
	for _, message := range messages {
		if message.Role == "user" && message.Subtype == "skill" && strings.Contains(message.Content, want) {
			return
		}
	}
	t.Fatalf("messages = %#v, want skill message containing %q", messages, want)
}
