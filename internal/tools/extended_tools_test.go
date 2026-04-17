package tools_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"myclaw/internal/permissions"
	"myclaw/internal/session"
	"myclaw/internal/tools"
)

func TestWebFetchToolFetchesHTTPContentWithDefaultFetcherAndUsesClaudeSchema(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		_, _ = w.Write([]byte("<html><body><h1>Release Notes</h1><p>Ship it.</p></body></html>"))
	}))
	defer server.Close()

	tool := tools.NewWebFetchTool(nil)
	def := tool.Definition()
	properties := def.InputSchema["properties"].(map[string]any)
	if properties["url"] == nil || properties["prompt"] == nil {
		t.Fatalf("schema = %#v, want url and prompt properties", def.InputSchema)
	}

	got, err := tool.Invoke(context.Background(), session.Session{}, `{"url":"`+server.URL+`","prompt":"summarize"}`)
	if err != nil {
		t.Fatalf("web fetch: %v", err)
	}
	if !strings.Contains(got, "Release Notes") || !strings.Contains(got, "Ship it.") {
		t.Fatalf("output = %q, want fetched page text", got)
	}
}

func TestWebFetchReturnsRedirectRetryInstruction(t *testing.T) {
	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte("target"))
	}))
	defer target.Close()
	redirector := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, target.URL+"/final", http.StatusFound)
	}))
	defer redirector.Close()

	got, err := tools.NewWebFetchTool(nil).Invoke(context.Background(), session.Session{}, `{"url":"`+redirector.URL+`","prompt":"summarize"}`)
	if err != nil {
		t.Fatalf("web fetch redirect: %v", err)
	}
	if !strings.Contains(got, "redirected") || !strings.Contains(got, target.URL+"/final") {
		t.Fatalf("output = %q, want retry instruction with redirected URL", got)
	}
	var parsed map[string]any
	if err := json.Unmarshal([]byte(got), &parsed); err != nil {
		t.Fatalf("redirect output JSON: %v", err)
	}
	redirect, _ := parsed["redirect"].(map[string]any)
	if redirect["originalUrl"] != redirector.URL || redirect["redirectUrl"] != target.URL+"/final" {
		t.Fatalf("redirect = %#v, want original and redirected URLs", redirect)
	}
}

func TestWebFetchFollowsSameOriginRedirect(t *testing.T) {
	server := httptest.NewServer(nil)
	mux := http.NewServeMux()
	mux.HandleFunc("/start", func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "/final", http.StatusTemporaryRedirect)
	})
	mux.HandleFunc("/final", func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte("<h1>Final page</h1>"))
	})
	server.Config.Handler = mux
	defer server.Close()

	got, err := tools.NewWebFetchTool(nil).Invoke(context.Background(), session.Session{}, `{"url":"`+server.URL+`/start","prompt":"summarize"}`)
	if err != nil {
		t.Fatalf("web fetch redirect: %v", err)
	}
	if !strings.Contains(got, "Final page") || strings.Contains(got, "redirected") {
		t.Fatalf("output = %q, want final same-origin page", got)
	}
}

func TestWebFetchDomainPermissionRules(t *testing.T) {
	tool := tools.NewWebFetchTool(nil)
	checker, ok := any(tool).(tools.ContextualPermissionCheckingTool)
	if !ok {
		t.Fatal("WebFetch must expose domain-level contextual permissions")
	}

	decision, err := checker.CheckPermissionsWithContext(context.Background(), tools.ToolUseContext{
		ToolName: "WebFetch",
		Input:    `{"url":"https://blocked.example/private","prompt":"summarize"}`,
		Policy: permissions.Policy{Rules: []permissions.Rule{{
			ToolName: "WebFetch",
			Action:   permissions.ActionDeny,
			Match:    permissions.Match{CommandContains: []string{"domain:blocked.example"}},
		}}},
	})
	if err != nil {
		t.Fatalf("check permissions: %v", err)
	}
	if decision.Allowed || decision.Category != permissions.CategoryRuleDenied {
		t.Fatalf("decision = %#v, want domain deny", decision)
	}
}

func TestMCPResourceToolsListAndReadContextResources(t *testing.T) {
	listTool := tools.NewListMcpResourcesTool()
	readTool := tools.NewReadMcpResourceTool()
	ctx := tools.ToolUseContext{
		Session:  session.Session{},
		ToolName: "ListMcpResources",
		MCPResources: map[string][]tools.MCPResource{
			"filesystem": {{URI: "file:///tmp/a", Name: "a", Description: "resource a"}},
		},
	}

	listed, err := listTool.InvokeWithContext(context.Background(), ctx)
	if err != nil {
		t.Fatalf("list mcp resources: %v", err)
	}
	var resources []any
	if err := json.Unmarshal([]byte(listed.Output), &resources); err != nil {
		t.Fatalf("list output JSON: %v", err)
	}
	if len(resources) != 1 {
		t.Fatalf("output = %q, want Claude-style resources array", listed.Output)
	}
	resource := resources[0].(map[string]any)
	if resource["server"] != "filesystem" || resource["uri"] != "file:///tmp/a" || resource["name"] != "a" || resource["description"] != "resource a" {
		t.Fatalf("resource = %#v, want structured MCP resource payload", resource)
	}

	readCtx := ctx
	readCtx.ToolName = "ReadMcpResource"
	readCtx.Input = `{"server":"filesystem","uri":"file:///tmp/a"}`
	read, err := readTool.InvokeWithContext(context.Background(), readCtx)
	if err != nil {
		t.Fatalf("read mcp resource: %v", err)
	}
	var parsed map[string]any
	if err := json.Unmarshal([]byte(read.Output), &parsed); err != nil {
		t.Fatalf("read output JSON: %v", err)
	}
	contents, _ := parsed["contents"].([]any)
	if len(contents) != 1 {
		t.Fatalf("output = %q, want Claude-style contents array", read.Output)
	}
	content := contents[0].(map[string]any)
	if content["uri"] != "file:///tmp/a" || content["text"] != "resource a" {
		t.Fatalf("content = %#v, want resource URI and text", content)
	}
}

func TestDynamicMCPToolUsesClaudeNameSchemaAndCaller(t *testing.T) {
	var gotServer, gotName string
	var gotInput map[string]any
	tool := tools.NewMCPTool(tools.MCPToolDefinition{
		Server:      "filesystem",
		Name:        "read_file",
		Description: "Read a file",
		InputSchema: map[string]any{"type": "object", "properties": map[string]any{"path": map[string]any{"type": "string"}}},
	}, func(_ context.Context, server, name string, input map[string]any) (tools.MCPToolResult, error) {
		gotServer, gotName, gotInput = server, name, input
		return tools.MCPToolResult{Content: []map[string]any{{"type": "text", "text": "file contents"}}}, nil
	})

	def := tool.Definition()
	if def.Name != "mcp__filesystem__read_file" || def.Source != "mcp" {
		t.Fatalf("definition = %#v, want Claude MCP tool name/source", def)
	}
	out, err := tool.Invoke(context.Background(), session.Session{}, `{"path":"README.md"}`)
	if err != nil {
		t.Fatalf("invoke mcp tool: %v", err)
	}
	if gotServer != "filesystem" || gotName != "read_file" || gotInput["path"] != "README.md" {
		t.Fatalf("call = server %q name %q input %#v", gotServer, gotName, gotInput)
	}
	if !strings.Contains(out, "file contents") {
		t.Fatalf("output = %q, want text content", out)
	}
}

func TestDynamicMCPToolContextualLifecyclePreservesMetaAndReportsProgress(t *testing.T) {
	var events []tools.ToolProgress
	tool := tools.NewMCPContextualTool(tools.MCPToolDefinition{
		Server:      "filesystem",
		Name:        "read_file",
		Description: "Read a file",
		InputSchema: map[string]any{"type": "object"},
	}, func(_ context.Context, req tools.MCPToolCallRequest) (tools.MCPToolResult, error) {
		if req.Server != "filesystem" || req.Name != "read_file" || req.Input["path"] != "README.md" {
			t.Fatalf("request = %#v, want server/name/input", req)
		}
		if req.ToolUseID != "toolu-1" || req.Timeout <= 0 {
			t.Fatalf("request = %#v, want tool use id and timeout", req)
		}
		if req.Meta["claudecode/toolUseId"] != "toolu-1" {
			t.Fatalf("request meta = %#v, want Claude toolUseId", req.Meta)
		}
		if req.ReportProgress != nil {
			req.ReportProgress(tools.ToolProgress{ToolUseID: req.ToolUseID, Type: "progress", Message: "server progress"})
		}
		return tools.MCPToolResult{
			Content:           []map[string]any{{"type": "text", "text": "file contents"}},
			StructuredContent: map[string]any{"ok": true},
			Meta:              map[string]any{"server": "filesystem"},
		}, nil
	})

	result, err := tool.(tools.ContextualTool).InvokeWithContext(context.Background(), tools.ToolUseContext{
		ToolName:  "mcp__filesystem__read_file",
		ToolUseID: "toolu-1",
		Input:     `{"path":"README.md"}`,
		ReportProgress: func(progress tools.ToolProgress) {
			events = append(events, progress)
		},
	})
	if err != nil {
		t.Fatalf("invoke contextual mcp tool: %v", err)
	}
	if result.StructuredContent == nil || result.Meta["server"] != "filesystem" {
		t.Fatalf("result = %#v, want structured content and metadata preserved", result)
	}
	if !strings.Contains(result.Output, "file contents") {
		t.Fatalf("output = %q, want text content envelope", result.Output)
	}
	var types []string
	for _, event := range events {
		types = append(types, event.Type)
	}
	if !reflect.DeepEqual(types, []string{"started", "progress", "completed"}) {
		t.Fatalf("progress types = %#v, want started/progress/completed", types)
	}
}

func TestDynamicMCPToolUsesClaudeMCPToolTimeoutEnv(t *testing.T) {
	t.Setenv("MCP_TOOL_TIMEOUT", "1234")
	tool := tools.NewMCPContextualTool(tools.MCPToolDefinition{
		Server: "filesystem",
		Name:   "read_file",
	}, func(_ context.Context, req tools.MCPToolCallRequest) (tools.MCPToolResult, error) {
		if req.Timeout != 1234*time.Millisecond {
			t.Fatalf("timeout = %s, want MCP_TOOL_TIMEOUT milliseconds", req.Timeout)
		}
		return tools.MCPToolResult{Content: []map[string]any{{"type": "text", "text": "ok"}}}, nil
	})

	if _, err := tool.(tools.ContextualTool).InvokeWithContext(context.Background(), tools.ToolUseContext{
		ToolName: "mcp__filesystem__read_file",
		Input:    `{}`,
	}); err != nil {
		t.Fatalf("invoke contextual mcp tool: %v", err)
	}
}

func TestDynamicMCPToolContextualLifecycleReturnsIsErrorWithMeta(t *testing.T) {
	tool := tools.NewMCPContextualTool(tools.MCPToolDefinition{
		Server: "filesystem",
		Name:   "read_file",
	}, func(context.Context, tools.MCPToolCallRequest) (tools.MCPToolResult, error) {
		return tools.MCPToolResult{
			Content: []map[string]any{{"type": "text", "text": "denied"}},
			Meta:    map[string]any{"reason": "policy"},
			IsError: true,
		}, nil
	})

	result, err := tool.(tools.ContextualTool).InvokeWithContext(context.Background(), tools.ToolUseContext{
		ToolName: "mcp__filesystem__read_file",
		Input:    `{}`,
	})
	if err == nil {
		t.Fatal("invoke contextual mcp tool: expected error for MCP isError result")
	}
	if !result.IsError || result.Meta["reason"] != "policy" || !strings.Contains(result.Output, "denied") {
		t.Fatalf("result = %#v, want isError result with preserved meta and content", result)
	}
}

func TestDynamicMCPToolMarksServerNeedsAuthWhenToolCallReturnsUnauthorized(t *testing.T) {
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
			w.Header().Set("WWW-Authenticate", `Bearer authorization_uri="https://auth.example/authorize"`)
			w.WriteHeader(http.StatusUnauthorized)
			_, _ = w.Write([]byte(`{"jsonrpc":"2.0","error":{"code":401,"message":"Unauthorized"}}`))
		default:
			t.Fatalf("unexpected method %q", method)
		}
	}))
	defer server.Close()

	discovered, err := tools.DiscoverMCPClientTools(context.Background(), []tools.MCPConnection{{
		Name:    "filesystem",
		Type:    "streamable_http",
		BaseURL: server.URL,
	}})
	if err != nil {
		t.Fatalf("discover mcp client: %v", err)
	}
	var appState map[string]any
	tool := tools.NewMCPContextualTool(tools.MCPToolDefinition{
		Server: "filesystem",
		Name:   "read_file",
	}, discovered.ContextualCaller)
	result, err := tool.(tools.ContextualTool).InvokeWithContext(context.Background(), tools.ToolUseContext{
		ToolName: "mcp__filesystem__read_file",
		Input:    `{"path":"README.md"}`,
		AppState: map[string]any{
			"mcp": map[string]any{
				"clients": []tools.MCPConnection{{
					Name:    "filesystem",
					Type:    "connected",
					BaseURL: server.URL,
				}},
				"tools": []tools.Definition{{Name: "mcp__filesystem__read_file"}},
			},
		},
		SetAppState: func(update func(map[string]any) map[string]any) {
			appState = update(appState)
		},
	})
	if err == nil {
		t.Fatal("invoke contextual mcp tool: expected auth-required error")
	}
	if result.Output != "" {
		t.Fatalf("result = %#v, want no successful tool result", result)
	}
	mcpState, ok := appState["mcp"].(map[string]any)
	if !ok {
		t.Fatalf("appState = %#v, want mcp state", appState)
	}
	clients, ok := mcpState["clients"].([]tools.MCPConnection)
	if !ok || len(clients) != 1 {
		t.Fatalf("clients = %#v, want one MCP client", mcpState["clients"])
	}
	if clients[0].Type != "needs-auth" || clients[0].Name != "filesystem" || clients[0].BaseURL != server.URL {
		t.Fatalf("clients = %#v, want filesystem marked needs-auth with config preserved", clients)
	}
	stateTools, ok := mcpState["tools"].([]tools.Definition)
	if !ok {
		t.Fatalf("tools = %#v, want tool definitions", mcpState["tools"])
	}
	if len(stateTools) != 1 || stateTools[0].Name != "mcp__filesystem__authenticate" {
		t.Fatalf("tools = %#v, want real MCP tools replaced by authenticate pseudo tool", stateTools)
	}
}

func TestDynamicMCPToolPreservesInsufficientScopeWhenToolCallReturnsForbidden(t *testing.T) {
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
			w.Header().Set("WWW-Authenticate", `Bearer error="insufficient_scope", scope="files:admin", authorization_uri="https://auth.example/authorize"`)
			w.WriteHeader(http.StatusForbidden)
			_, _ = w.Write([]byte(`{"error":"insufficient_scope"}`))
		default:
			t.Fatalf("unexpected method %q", method)
		}
	}))
	defer server.Close()

	discovered, err := tools.DiscoverMCPClientTools(context.Background(), []tools.MCPConnection{{
		Name:    "filesystem",
		Type:    "streamable_http",
		BaseURL: server.URL,
	}})
	if err != nil {
		t.Fatalf("discover mcp client: %v", err)
	}
	var appState map[string]any
	tool := tools.NewMCPContextualTool(tools.MCPToolDefinition{
		Server: "filesystem",
		Name:   "read_file",
	}, discovered.ContextualCaller)
	_, err = tool.(tools.ContextualTool).InvokeWithContext(context.Background(), tools.ToolUseContext{
		ToolName: "mcp__filesystem__read_file",
		Input:    `{"path":"README.md"}`,
		AppState: map[string]any{
			"mcp": map[string]any{
				"clients": []tools.MCPConnection{{Name: "filesystem", Type: "connected", BaseURL: server.URL}},
				"tools":   []tools.Definition{{Name: "mcp__filesystem__read_file"}},
			},
		},
		SetAppState: func(update func(map[string]any) map[string]any) {
			appState = update(appState)
		},
	})
	if err == nil {
		t.Fatal("invoke contextual mcp tool: expected insufficient-scope auth error")
	}
	mcpAuth, ok := appState["mcpAuth"].(map[string]any)
	if !ok {
		t.Fatalf("appState = %#v, want mcpAuth state", appState)
	}
	authState, ok := mcpAuth["filesystem"].(map[string]any)
	if !ok {
		t.Fatalf("mcpAuth = %#v, want filesystem auth state", mcpAuth)
	}
	if authState["scope"] != "files:admin" {
		t.Fatalf("authState = %#v, want scope preserved", authState)
	}
	challenge, ok := authState["challenge"].(map[string]string)
	if !ok || challenge["error"] != "insufficient_scope" || challenge["authorization_uri"] != "https://auth.example/authorize" {
		t.Fatalf("authState = %#v, want parsed challenge preserved", authState)
	}
}

func TestMCPAuthToolReturnsAuthURLBeforeReconnectCompletes(t *testing.T) {
	var authenticated bool
	var reconnected bool
	tool := tools.NewMCPAuthTool("filesystem", "https://auth.example/start", "Authenticate filesystem")
	result, err := tool.(tools.ContextualTool).InvokeWithContext(context.Background(), tools.ToolUseContext{
		ToolName: "mcp__filesystem__authenticate",
		MCPClients: []tools.MCPConnection{{
			Name:    "filesystem",
			Type:    "streamable_http",
			BaseURL: "https://mcp.example",
		}},
		MCPAuthenticator: func(_ context.Context, server string, connection tools.MCPConnection) (tools.MCPAuthStartResult, error) {
			authenticated = true
			if server != "filesystem" || connection.BaseURL != "https://mcp.example" {
				t.Fatalf("auth server=%q connection=%#v, want filesystem connection", server, connection)
			}
			return tools.MCPAuthStartResult{Status: "auth_url", AuthURL: "https://auth.example/authorize", Message: "Open browser"}, nil
		},
		MCPReconnect: func(_ context.Context, server string) (tools.MCPReconnectResult, error) {
			reconnected = true
			if server != "filesystem" {
				t.Fatalf("reconnect server=%q, want filesystem", server)
			}
			return tools.MCPReconnectResult{
				Tools:     tools.MCPToolsListResult{Tools: []tools.MCPToolListItem{{Name: "read_file"}}},
				Resources: []tools.MCPResource{{URI: "file:///README.md", Name: "README"}},
			}, nil
		},
	})
	if err != nil {
		t.Fatalf("invoke auth tool: %v", err)
	}
	if !authenticated {
		t.Fatal("expected authenticator to be called")
	}
	if reconnected {
		t.Fatal("auth_url result must return before reconnecting; reconnect waits for OAuth completion")
	}
	var parsed map[string]any
	if err := json.Unmarshal([]byte(result.Output), &parsed); err != nil {
		t.Fatalf("auth output JSON: %v", err)
	}
	if parsed["status"] != "auth_url" || parsed["authUrl"] != "https://auth.example/authorize" {
		t.Fatalf("auth output = %#v, want auth_url", parsed)
	}
	if _, ok := parsed["reconnected"]; ok {
		t.Fatalf("auth output = %#v, did not want synchronous reconnect state", parsed)
	}
}

func TestMCPAuthToolPassesChallengeContextToAuthenticator(t *testing.T) {
	tool := tools.NewMCPAuthToolFromResult("filesystem", tools.MCPAuthToolResult{
		AuthURL: "https://auth.example/start",
		Message: "Authenticate filesystem",
		Scope:   "files:admin",
		Challenge: map[string]string{
			"error":             "insufficient_scope",
			"resource_metadata": "https://auth.example/.well-known/oauth-protected-resource",
		},
	})
	var seen tools.MCPConnection
	result, err := tool.(tools.ContextualTool).InvokeWithContext(context.Background(), tools.ToolUseContext{
		MCPClients: []tools.MCPConnection{{
			Name:    "filesystem",
			Type:    "needs-auth",
			BaseURL: "https://mcp.example",
		}},
		MCPAuthenticator: func(_ context.Context, _ string, connection tools.MCPConnection) (tools.MCPAuthStartResult, error) {
			seen = connection
			return tools.MCPAuthStartResult{
				Status:              "auth_url",
				AuthURL:             "https://auth.example/authorize",
				Message:             "Open browser",
				Scope:               connection.AuthScope,
				ResourceMetadataURL: connection.AuthResourceMetadataURL,
			}, nil
		},
	})
	if err != nil {
		t.Fatalf("invoke auth tool: %v", err)
	}
	if seen.AuthURL != "https://auth.example/start" ||
		seen.AuthScope != "files:admin" ||
		seen.AuthResourceMetadataURL != "https://auth.example/.well-known/oauth-protected-resource" ||
		seen.AuthChallenge["error"] != "insufficient_scope" {
		t.Fatalf("authenticator connection = %#v, want typed auth challenge context", seen)
	}
	var payload map[string]any
	if err := json.Unmarshal([]byte(result.Output), &payload); err != nil {
		t.Fatalf("decode auth output: %v", err)
	}
	if payload["scope"] != "files:admin" ||
		payload["resourceMetadataUrl"] != "https://auth.example/.well-known/oauth-protected-resource" {
		t.Fatalf("payload = %#v, want scope and resource metadata preserved", payload)
	}
}

func TestMCPAuthToolUsesCachedOAuthContextFromStore(t *testing.T) {
	store := tools.NewMCPOAuthMemoryStore()
	provider := tools.NewMCPOAuthProvider(store, "filesystem", tools.MCPConnection{
		Name:    "filesystem",
		BaseURL: "https://mcp.example",
	})
	if err := provider.SaveStepUpScope("files:cached"); err != nil {
		t.Fatalf("save step-up scope: %v", err)
	}
	if err := provider.SaveDiscoveryState(tools.MCPOAuthDiscoveryState{
		AuthorizationServerURL: "https://auth.example",
		ResourceMetadataURL:    "https://auth.example/.well-known/oauth-protected-resource",
	}); err != nil {
		t.Fatalf("save discovery: %v", err)
	}
	tool := tools.NewMCPAuthTool("filesystem", "https://auth.example/start", "Authenticate filesystem")

	var seen tools.MCPConnection
	_, err := tool.(tools.ContextualTool).InvokeWithContext(context.Background(), tools.ToolUseContext{
		MCPClients: []tools.MCPConnection{{
			Name:    "filesystem",
			Type:    "needs-auth",
			BaseURL: "https://mcp.example",
		}},
		MCPOAuthStore: store,
		MCPAuthenticator: func(_ context.Context, _ string, connection tools.MCPConnection) (tools.MCPAuthStartResult, error) {
			seen = connection
			return tools.MCPAuthStartResult{Status: "auth_url", AuthURL: "https://auth.example/authorize"}, nil
		},
	})
	if err != nil {
		t.Fatalf("invoke auth tool: %v", err)
	}
	if seen.AuthScope != "files:cached" ||
		seen.AuthResourceMetadataURL != "https://auth.example/.well-known/oauth-protected-resource" {
		t.Fatalf("authenticator connection = %#v, want cached OAuth context from store", seen)
	}
}

func TestMCPAuthToolReconnectsAfterOAuthCompletionAndRefreshesAppState(t *testing.T) {
	completion := make(chan tools.MCPAuthCompletionResult)
	reconnected := make(chan struct{})
	var appState map[string]any
	tool := tools.NewMCPAuthTool("filesystem", "https://auth.example/start", "Authenticate filesystem")
	result, err := tool.(tools.ContextualTool).InvokeWithContext(context.Background(), tools.ToolUseContext{
		ToolName: "mcp__filesystem__authenticate",
		MCPClients: []tools.MCPConnection{{
			Name:    "filesystem",
			Type:    "http",
			BaseURL: "https://mcp.example",
		}},
		AppState: map[string]any{
			"mcp": map[string]any{
				"clients": []tools.MCPConnection{{Name: "filesystem", Type: "http", BaseURL: "https://mcp.example"}},
				"tools": []tools.Definition{
					{Name: "mcp__filesystem__authenticate"},
					{Name: "mcp__other__read_file"},
				},
				"commands": []tools.Command{{Name: "filesystem:old"}, {Name: "other:cmd"}},
				"resources": map[string][]tools.MCPResource{
					"filesystem": {{URI: "file:///old.txt", Name: "old"}},
				},
			},
		},
		SetAppState: func(update func(map[string]any) map[string]any) {
			appState = update(appState)
			select {
			case <-reconnected:
			default:
				close(reconnected)
			}
		},
		MCPAuthenticator: func(_ context.Context, _ string, _ tools.MCPConnection) (tools.MCPAuthStartResult, error) {
			return tools.MCPAuthStartResult{
				Status:     "auth_url",
				AuthURL:    "https://auth.example/authorize",
				Message:    "Open browser",
				Completion: completion,
			}, nil
		},
		MCPReconnect: func(_ context.Context, server string) (tools.MCPReconnectResult, error) {
			if server != "filesystem" {
				t.Fatalf("reconnect server=%q, want filesystem", server)
			}
			return tools.MCPReconnectResult{
				Client: tools.MCPConnection{Name: "filesystem", Type: "connected", BaseURL: "https://mcp.example"},
				Tools: tools.MCPToolsListResult{Tools: []tools.MCPToolListItem{{
					Name:        "read_file",
					Description: "Read file.",
				}}},
				Prompts:   tools.MCPPromptsListResult{Prompts: []tools.MCPPromptListItem{{Name: "review"}}},
				Resources: []tools.MCPResource{{URI: "file:///README.md", Name: "README"}},
			}, nil
		},
	})
	if err != nil {
		t.Fatalf("invoke auth tool: %v", err)
	}
	var parsed map[string]any
	if err := json.Unmarshal([]byte(result.Output), &parsed); err != nil {
		t.Fatalf("auth output JSON: %v", err)
	}
	if parsed["status"] != "auth_url" || parsed["authUrl"] != "https://auth.example/authorize" {
		t.Fatalf("auth output = %#v, want auth_url before completion", parsed)
	}

	completion <- tools.MCPAuthCompletionResult{Status: "complete"}
	select {
	case <-reconnected:
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for OAuth completion reconnect")
	}
	mcpState, ok := appState["mcp"].(map[string]any)
	if !ok {
		t.Fatalf("appState = %#v, want mcp state", appState)
	}
	stateTools, ok := mcpState["tools"].([]tools.Definition)
	if !ok {
		t.Fatalf("mcp state = %#v, want tools definitions", mcpState)
	}
	var sawReal, sawAuth, sawOther bool
	for _, def := range stateTools {
		switch def.Name {
		case "mcp__filesystem__read_file":
			sawReal = true
		case "mcp__filesystem__authenticate":
			sawAuth = true
		case "mcp__other__read_file":
			sawOther = true
		}
	}
	if !sawReal || sawAuth || !sawOther {
		t.Fatalf("tools = %#v, want filesystem auth replaced with real tool and other tools preserved", stateTools)
	}
	resources := mcpState["resources"].(map[string][]tools.MCPResource)
	if got := resources["filesystem"]; len(got) != 1 || got[0].URI != "file:///README.md" {
		t.Fatalf("resources = %#v, want refreshed filesystem resources", resources)
	}
	clients := mcpState["clients"].([]tools.MCPConnection)
	if len(clients) != 1 || clients[0].Name != "filesystem" || clients[0].Type != "connected" {
		t.Fatalf("clients = %#v, want reconnected client swapped into app state", clients)
	}
}

func TestMCPAuthToolImmediateCompletionRefreshesAppState(t *testing.T) {
	var appState map[string]any
	tool := tools.NewMCPAuthTool("filesystem", "https://auth.example/start", "Authenticate filesystem")
	result, err := tool.(tools.ContextualTool).InvokeWithContext(context.Background(), tools.ToolUseContext{
		ToolName: "mcp__filesystem__authenticate",
		AppState: map[string]any{
			"mcp": map[string]any{
				"tools": []tools.Definition{
					{Name: "mcp__filesystem__authenticate"},
				},
			},
		},
		SetAppState: func(update func(map[string]any) map[string]any) {
			appState = update(appState)
		},
		MCPAuthenticator: func(_ context.Context, _ string, _ tools.MCPConnection) (tools.MCPAuthStartResult, error) {
			return tools.MCPAuthStartResult{Status: "complete", Message: "Authenticated"}, nil
		},
		MCPReconnect: func(_ context.Context, server string) (tools.MCPReconnectResult, error) {
			if server != "filesystem" {
				t.Fatalf("reconnect server=%q, want filesystem", server)
			}
			return tools.MCPReconnectResult{
				Tools: tools.MCPToolsListResult{Tools: []tools.MCPToolListItem{{
					Name:        "read_file",
					Description: "Read file.",
				}}},
			}, nil
		},
	})
	if err != nil {
		t.Fatalf("invoke auth tool: %v", err)
	}
	var parsed map[string]any
	if err := json.Unmarshal([]byte(result.Output), &parsed); err != nil {
		t.Fatalf("auth output JSON: %v", err)
	}
	if parsed["reconnected"] != true {
		t.Fatalf("auth output = %#v, want synchronous reconnect status", parsed)
	}
	mcpState, ok := appState["mcp"].(map[string]any)
	if !ok {
		t.Fatalf("appState = %#v, want mcp state", appState)
	}
	stateTools, ok := mcpState["tools"].([]tools.Definition)
	if !ok {
		t.Fatalf("mcp state = %#v, want tools definitions", mcpState)
	}
	if len(stateTools) != 1 || stateTools[0].Name != "mcp__filesystem__read_file" {
		t.Fatalf("tools = %#v, want authenticate replaced with real tool", stateTools)
	}
}

func TestListMcpResourcesUsesLiveListerAndReturnsClaudeArray(t *testing.T) {
	tool := tools.NewListMcpResourcesTool()
	result, err := tool.InvokeWithContext(context.Background(), tools.ToolUseContext{
		ToolName: "ListMcpResources",
		MCPResources: map[string][]tools.MCPResource{
			"filesystem": {{URI: "file:///stale.txt", Name: "stale"}},
		},
		MCPResourceLister: func(_ context.Context, server string) ([]tools.MCPResource, error) {
			if server != "" {
				t.Fatalf("server filter = %q, want all servers", server)
			}
			return []tools.MCPResource{{URI: "file:///fresh.txt", Name: "fresh", Description: "live"}}, nil
		},
	})
	if err != nil {
		t.Fatalf("list resources: %v", err)
	}
	var parsed []map[string]any
	if err := json.Unmarshal([]byte(result.Output), &parsed); err != nil {
		t.Fatalf("list output should be Claude array JSON, got %q: %v", result.Output, err)
	}
	if len(parsed) != 1 || parsed[0]["uri"] != "file:///fresh.txt" || parsed[0]["name"] != "fresh" {
		t.Fatalf("resources = %#v, want live resource array", parsed)
	}
}

func TestSkillToolLoadsSkillPathAndRecordsInvocation(t *testing.T) {
	dir := t.TempDir()
	path := dir + "/SKILL.md"
	if err := os.WriteFile(path, []byte("---\nname: demo\n---\nUse demo skill."), 0o644); err != nil {
		t.Fatalf("write skill: %v", err)
	}
	appState := map[string]any{}
	tool := tools.NewSkillTool()
	properties := tool.Definition().InputSchema["properties"].(map[string]any)
	if properties["skill"] == nil || properties["args"] == nil {
		t.Fatalf("schema = %#v, want Claude skill/args inputs", tool.Definition().InputSchema)
	}
	if properties["name"] != nil || properties["path"] != nil {
		t.Fatalf("schema = %#v, should not expose Go-only name/path fields", tool.Definition().InputSchema)
	}

	result, err := tool.InvokeWithContext(context.Background(), tools.ToolUseContext{
		Session:  session.Session{},
		AgentID:  "agent-1",
		ToolName: "Skill",
		Input:    `{"skill":"demo","path":"` + strings.ReplaceAll(path, `\`, `\\`) + `"}`,
		AppState: appState,
		SetAppState: func(update func(map[string]any) map[string]any) {
			appState = update(appState)
		},
	})
	if err != nil {
		t.Fatalf("invoke skill: %v", err)
	}
	if strings.Contains(result.Output, "Use demo skill.") {
		t.Fatalf("output = %q, should not include expanded skill content", result.Output)
	}
	if len(result.NewMessages) != 1 || !strings.Contains(result.NewMessages[0].Content, "Use demo skill.") {
		t.Fatalf("new messages = %#v, want skill file contents injected as meta message", result.NewMessages)
	}
	invoked, _ := appState["invokedSkills"].([]any)
	if len(invoked) != 1 {
		t.Fatalf("appState = %#v, want one invoked skill recorded", appState)
	}
	record := invoked[0].(map[string]any)
	if record["skillName"] != "demo" || record["skillPath"] != path || record["content"] == "" || record["agentId"] != "agent-1" || record["invokedAt"] == "" {
		t.Fatalf("invoked skill = %#v, want Claude-style state metadata", record)
	}
}

func TestSkillToolLoadsCatalogFrontmatterAndReturnsInlineJSON(t *testing.T) {
	dir := t.TempDir()
	skillDir := filepath.Join(dir, "skills", "verify")
	if err := os.MkdirAll(skillDir, 0o755); err != nil {
		t.Fatalf("mkdir skill: %v", err)
	}
	path := filepath.Join(skillDir, "SKILL.md")
	if err := os.WriteFile(path, []byte("---\nallowed-tools: Read,Grep\nmodel: claude-sonnet-4-5\n---\nRun verification."), 0o644); err != nil {
		t.Fatalf("write skill: %v", err)
	}
	tool := tools.NewSkillTool()
	appState := map[string]any{
		"skillRoots": []any{filepath.Join(dir, "skills")},
	}

	result, err := tool.InvokeWithContext(context.Background(), tools.ToolUseContext{
		Session:  session.Session{AgentID: "main"},
		ToolName: "Skill",
		Input:    `{"skill":"verify","args":"unit tests"}`,
		AppState: appState,
		SetAppState: func(update func(map[string]any) map[string]any) {
			appState = update(appState)
		},
	})
	if err != nil {
		t.Fatalf("invoke skill: %v", err)
	}
	var parsed map[string]any
	if err := json.Unmarshal([]byte(result.Output), &parsed); err != nil {
		t.Fatalf("skill output JSON: %v\n%s", err, result.Output)
	}
	if parsed["status"] != "inline" || parsed["commandName"] != "verify" || parsed["model"] != "claude-sonnet-4-5" {
		t.Fatalf("output = %#v, want inline skill metadata", parsed)
	}
	if _, hasContent := parsed["content"]; hasContent {
		t.Fatalf("output = %#v, should not include expanded skill content", parsed)
	}
	if len(result.NewMessages) != 1 || !strings.Contains(result.NewMessages[0].Content, "Run verification.") || !strings.Contains(result.NewMessages[0].Content, "unit tests") {
		t.Fatalf("new messages = %#v, want skill content and args", result.NewMessages)
	}
}

func TestLoadClaudeSkillDirectoriesLoadsSourcesLegacyCommandsAndDedupesRealpath(t *testing.T) {
	tools.ClearDynamicSkills()
	t.Cleanup(tools.ClearDynamicSkills)

	dir := t.TempDir()
	configHome := filepath.Join(dir, "home", ".claude")
	managedRoot := filepath.Join(dir, "managed")
	project := filepath.Join(dir, "workspace", "pkg")
	additional := filepath.Join(dir, "extra")

	managedSkill := filepath.Join(managedRoot, ".claude", "skills", "managed", "SKILL.md")
	userSkill := filepath.Join(configHome, "skills", "user", "SKILL.md")
	projectSkill := filepath.Join(project, ".claude", "skills", "project", "SKILL.md")
	additionalSkill := filepath.Join(additional, ".claude", "skills", "extra", "SKILL.md")
	legacyCommand := filepath.Join(project, ".claude", "commands", "legacy.md")
	for path, body := range map[string]string{
		managedSkill:    "---\ndescription: managed skill\n---\nManaged body.",
		userSkill:       "---\ndescription: user skill\n---\nUser body.",
		projectSkill:    "---\ndescription: project skill\n---\nProject body.",
		additionalSkill: "---\ndescription: extra skill\n---\nExtra body.",
		legacyCommand:   "---\ndescription: legacy command\n---\nLegacy body.",
	} {
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatalf("mkdir %q: %v", path, err)
		}
		if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
			t.Fatalf("write %q: %v", path, err)
		}
	}
	duplicateRoot := filepath.Join(dir, "dupe")
	if err := os.MkdirAll(filepath.Join(duplicateRoot, ".claude", "skills"), 0o755); err != nil {
		t.Fatalf("mkdir duplicate root: %v", err)
	}
	duplicateSkillDir := filepath.Join(duplicateRoot, ".claude", "skills", "project")
	if err := os.Symlink(filepath.Dir(projectSkill), duplicateSkillDir); err == nil {
		// Include a realpath duplicate only on filesystems that support symlinks.
		additional = duplicateRoot
	}

	loaded := tools.LoadClaudeSkillDirectories(tools.SkillDiscoveryOptions{
		CWD:             project,
		ConfigHome:      configHome,
		ManagedRoot:     managedRoot,
		AdditionalDirs:  []string{additional},
		IncludeUser:     true,
		IncludeProject:  true,
		IncludeManaged:  true,
		IncludeLegacy:   true,
		IncludeExplicit: true,
	})

	names := make([]string, 0, len(loaded))
	for _, skill := range loaded {
		names = append(names, skill.Name)
	}
	want := []string{"managed", "user", "project", "legacy"}
	if additional != duplicateRoot {
		want = []string{"managed", "user", "project", "extra", "legacy"}
	}
	if !reflect.DeepEqual(names, want) {
		t.Fatalf("loaded skill names = %#v, want %#v", names, want)
	}
	if got := tools.GetDynamicSkills(); len(got) != len(want) {
		t.Fatalf("dynamic skills = %#v, want loaded skills registered", got)
	}
	gotNames := make([]string, 0, len(want))
	for _, skill := range tools.GetDynamicSkills() {
		gotNames = append(gotNames, skill.Name)
	}
	if !reflect.DeepEqual(gotNames, want) {
		t.Fatalf("dynamic skill order = %#v, want Claude load order %#v", gotNames, want)
	}
}

func TestLoadClaudeSkillDirectoriesBareModeLoadsOnlyAdditionalDirs(t *testing.T) {
	tools.ClearDynamicSkills()
	t.Cleanup(tools.ClearDynamicSkills)

	dir := t.TempDir()
	configHome := filepath.Join(dir, "home", ".claude")
	project := filepath.Join(dir, "workspace")
	additional := filepath.Join(dir, "extra")
	for path, body := range map[string]string{
		filepath.Join(configHome, "skills", "user", "SKILL.md"):             "---\ndescription: user skill\n---\nUser body.",
		filepath.Join(project, ".claude", "skills", "project", "SKILL.md"):  "---\ndescription: project skill\n---\nProject body.",
		filepath.Join(additional, ".claude", "skills", "extra", "SKILL.md"): "---\ndescription: extra skill\n---\nExtra body.",
		filepath.Join(project, ".claude", "commands", "legacy", "SKILL.md"): "---\ndescription: legacy command\n---\nLegacy body.",
		filepath.Join(project, ".claude", "commands", "legacy-file.md"):     "---\ndescription: legacy file command\n---\nLegacy file body.",
	} {
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatalf("mkdir %q: %v", path, err)
		}
		if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
			t.Fatalf("write %q: %v", path, err)
		}
	}

	loaded := tools.LoadClaudeSkillDirectories(tools.SkillDiscoveryOptions{
		CWD:             project,
		ConfigHome:      configHome,
		AdditionalDirs:  []string{additional},
		IncludeUser:     true,
		IncludeProject:  true,
		IncludeLegacy:   true,
		IncludeExplicit: true,
		BareMode:        true,
	})

	if len(loaded) != 1 || loaded[0].Name != "extra" {
		t.Fatalf("bare loaded = %#v, want only additional skill", loaded)
	}
}

func TestLoadClaudeSkillDirectoriesSkillsLockedSuppressesAllSources(t *testing.T) {
	tools.ClearDynamicSkills()
	t.Cleanup(tools.ClearDynamicSkills)

	dir := t.TempDir()
	configHome := filepath.Join(dir, "home", ".claude")
	project := filepath.Join(dir, "workspace")
	additional := filepath.Join(dir, "extra")
	for path, body := range map[string]string{
		filepath.Join(configHome, "skills", "user", "SKILL.md"):             "---\ndescription: user skill\n---\nUser body.",
		filepath.Join(project, ".claude", "skills", "project", "SKILL.md"):  "---\ndescription: project skill\n---\nProject body.",
		filepath.Join(additional, ".claude", "skills", "extra", "SKILL.md"): "---\ndescription: extra skill\n---\nExtra body.",
		filepath.Join(project, ".claude", "commands", "legacy.md"):          "---\ndescription: legacy command\n---\nLegacy body.",
	} {
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatalf("mkdir %q: %v", path, err)
		}
		if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
			t.Fatalf("write %q: %v", path, err)
		}
	}

	loaded := tools.LoadClaudeSkillDirectories(tools.SkillDiscoveryOptions{
		CWD:             project,
		ConfigHome:      configHome,
		AdditionalDirs:  []string{additional},
		IncludeUser:     true,
		IncludeProject:  true,
		IncludeLegacy:   true,
		IncludeExplicit: true,
		SkillsLocked:    true,
	})

	if len(loaded) != 0 {
		t.Fatalf("locked loaded = %#v, want no skills", loaded)
	}
	if got := tools.GetDynamicSkills(); len(got) != 0 {
		t.Fatalf("dynamic skills = %#v, want locked discovery to avoid registration", got)
	}
}

func TestSkillToolInjectsInlineSkillAsMetaMessageAndCompactToolResult(t *testing.T) {
	dir := t.TempDir()
	root := filepath.Join(dir, "skills")
	skillDir := filepath.Join(root, "review")
	if err := os.MkdirAll(skillDir, 0o755); err != nil {
		t.Fatalf("mkdir skill: %v", err)
	}
	path := filepath.Join(skillDir, "SKILL.md")
	if err := os.WriteFile(path, []byte("---\nallowed-tools: Read,Grep\nmodel: claude-sonnet-4-5\neffort: high\n---\nReview content."), 0o644); err != nil {
		t.Fatalf("write skill: %v", err)
	}
	appState := map[string]any{"skillRoots": []any{root}}
	result, err := tools.NewSkillTool().InvokeWithContext(context.Background(), tools.ToolUseContext{
		Session:   session.Session{ID: "sess-1", AgentID: "agent-1"},
		AgentID:   "agent-1",
		ToolName:  "Skill",
		ToolUseID: "toolu-skill",
		Input:     `{"skill":"review","args":"README.md"}`,
		AppState:  appState,
		SetAppState: func(update func(map[string]any) map[string]any) {
			appState = update(appState)
		},
	})
	if err != nil {
		t.Fatalf("invoke skill: %v", err)
	}
	var parsed map[string]any
	if err := json.Unmarshal([]byte(result.Output), &parsed); err != nil {
		t.Fatalf("skill output JSON: %v", err)
	}
	if parsed["success"] != true || parsed["commandName"] != "review" || parsed["status"] != "inline" {
		t.Fatalf("output = %#v, want compact success metadata", parsed)
	}
	if _, hasContent := parsed["content"]; hasContent {
		t.Fatalf("output = %#v, should not include expanded skill content in tool_result", parsed)
	}
	if len(result.NewMessages) != 1 {
		t.Fatalf("new messages = %#v, want one injected meta message", result.NewMessages)
	}
	msg := result.NewMessages[0]
	if msg.Role != "user" || !msg.IsMeta || msg.Subtype != "skill" || !strings.Contains(msg.Content, "Review content.") {
		t.Fatalf("new message = %#v, want Claude-style meta user skill content", msg)
	}
	if msg.LogicalParentID != "toolu-skill" {
		t.Fatalf("new message logical parent = %q, want tool use id", msg.LogicalParentID)
	}
	modified := result.ContextModifier(tools.ToolUseContext{AppState: map[string]any{}})
	if got := testStringList(modified.AppState["skillAllowedTools"]); !reflect.DeepEqual(got, []string{"Read", "Grep"}) {
		t.Fatalf("skillAllowedTools = %#v, want allowed tools", got)
	}
	if modified.AppState["skillModel"] != "claude-sonnet-4-5" || modified.AppState["skillEffort"] != "high" {
		t.Fatalf("appState = %#v, want model/effort context propagation", modified.AppState)
	}
}

func TestSkillToolPermissionAllowsSafeSkillProperties(t *testing.T) {
	dir := t.TempDir()
	root := filepath.Join(dir, "skills")
	skillDir := filepath.Join(root, "safe")
	if err := os.MkdirAll(skillDir, 0o755); err != nil {
		t.Fatalf("mkdir skill: %v", err)
	}
	if err := os.WriteFile(filepath.Join(skillDir, "SKILL.md"), []byte("---\ndescription: Safe skill\nwhen_to_use: when safe\n---\nSafe body."), 0o644); err != nil {
		t.Fatalf("write skill: %v", err)
	}
	registry := tools.NewRegistry(tools.NewSkillTool())
	decision, checked, err := registry.CheckPermissionsWithContext(context.Background(), tools.ToolUseContext{
		ToolName:    "Skill",
		Input:       `{"skill":"/safe","args":"now"}`,
		InputObject: map[string]any{"skill": "/safe", "args": "now"},
		AppState:    map[string]any{"skillRoots": []any{root}},
		Policy:      permissions.Policy{Mode: permissions.ModeAsk},
	})
	if err != nil {
		t.Fatalf("check permission: %v", err)
	}
	if !checked {
		t.Fatal("checked = false, want skill-specific permission checker")
	}
	if !decision.Allowed || decision.RequiresApproval {
		t.Fatalf("decision = %#v, want safe skill auto-allowed", decision)
	}
	if decision.UpdatedInputObject["skill"] != "safe" || decision.UpdatedInputObject["args"] != "now" {
		t.Fatalf("updated input = %#v, want normalized skill and original args", decision.UpdatedInputObject)
	}
}

func TestSkillToolPermissionRequiresApprovalForUnsafeProperties(t *testing.T) {
	dir := t.TempDir()
	root := filepath.Join(dir, "skills")
	skillDir := filepath.Join(root, "unsafe")
	if err := os.MkdirAll(skillDir, 0o755); err != nil {
		t.Fatalf("mkdir skill: %v", err)
	}
	if err := os.WriteFile(filepath.Join(skillDir, "SKILL.md"), []byte("---\nallowed-tools: Bash\n---\nUnsafe body."), 0o644); err != nil {
		t.Fatalf("write skill: %v", err)
	}
	registry := tools.NewRegistry(tools.NewSkillTool())
	decision, checked, err := registry.CheckPermissionsWithContext(context.Background(), tools.ToolUseContext{
		ToolName:    "Skill",
		Input:       `{"skill":"unsafe","args":"--all"}`,
		InputObject: map[string]any{"skill": "unsafe", "args": "--all"},
		AppState:    map[string]any{"skillRoots": []any{root}},
		Policy:      permissions.Policy{Mode: permissions.ModeDangerFullAccess},
	})
	if err != nil {
		t.Fatalf("check permission: %v", err)
	}
	if !checked {
		t.Fatal("checked = false, want skill-specific permission checker")
	}
	if decision.Allowed || !decision.RequiresApproval || decision.Category != permissions.CategoryApproval {
		t.Fatalf("decision = %#v, want unsafe skill approval", decision)
	}
	if !strings.Contains(decision.Reason, "Execute skill: unsafe") {
		t.Fatalf("reason = %q, want Claude-style execute skill prompt", decision.Reason)
	}
	if len(decision.UpdatedPermissions) != 2 {
		t.Fatalf("updated permissions = %#v, want exact and prefix suggestions", decision.UpdatedPermissions)
	}
}

func TestSkillToolPermissionRulesOverrideSafeProperties(t *testing.T) {
	dir := t.TempDir()
	root := filepath.Join(dir, "skills")
	skillDir := filepath.Join(root, "review")
	if err := os.MkdirAll(skillDir, 0o755); err != nil {
		t.Fatalf("mkdir skill: %v", err)
	}
	if err := os.WriteFile(filepath.Join(skillDir, "SKILL.md"), []byte("---\ndescription: Review\n---\nReview body."), 0o644); err != nil {
		t.Fatalf("write skill: %v", err)
	}
	registry := tools.NewRegistry(tools.NewSkillTool())
	deny, checked, err := registry.CheckPermissionsWithContext(context.Background(), tools.ToolUseContext{
		ToolName:    "Skill",
		InputObject: map[string]any{"skill": "review"},
		AppState:    map[string]any{"skillRoots": []any{root}},
		Policy: permissions.Policy{Rules: []permissions.Rule{{
			ToolName: "Skill",
			Action:   permissions.ActionDeny,
			Match:    permissions.Match{CommandContains: []string{"review"}},
		}}},
	})
	if err != nil {
		t.Fatalf("deny permission: %v", err)
	}
	if !checked || deny.Allowed || deny.Category != permissions.CategoryRuleDenied {
		t.Fatalf("deny decision = %#v checked=%v, want deny rule to win", deny, checked)
	}

	allow, checked, err := registry.CheckPermissionsWithContext(context.Background(), tools.ToolUseContext{
		ToolName:    "Skill",
		InputObject: map[string]any{"skill": "review"},
		AppState:    map[string]any{"skillRoots": []any{root}},
		Policy: permissions.Policy{Rules: []permissions.Rule{{
			ToolName: "Skill",
			Action:   permissions.ActionAllow,
			Match:    permissions.Match{CommandContains: []string{"review"}},
		}}},
	})
	if err != nil {
		t.Fatalf("allow permission: %v", err)
	}
	if !checked || !allow.Allowed || allow.RequiresApproval {
		t.Fatalf("allow decision = %#v checked=%v, want allow rule", allow, checked)
	}
}

func TestSkillToolInvokesRemoteCanonicalSkillFromResolver(t *testing.T) {
	appState := map[string]any{
		"remoteCanonicalSkillResolver": tools.RemoteCanonicalSkillResolverFunc(func(slug string) (tools.SkillCommand, bool, error) {
			if slug != "review" {
				return tools.SkillCommand{}, false, nil
			}
			return tools.SkillCommand{
				Name:        "_canonical_review",
				Description: "Remote canonical review",
				Content:     "Remote canonical body for $ARGUMENTS.",
				Source:      "remote",
				LoadedFrom:  "remote",
			}, true, nil
		}),
	}

	result, err := tools.NewSkillTool().InvokeWithContext(context.Background(), tools.ToolUseContext{
		Session:  session.Session{ID: "sess-remote", AgentID: "agent-remote"},
		ToolName: "Skill",
		Input:    `{"skill":"_canonical_review","args":"diff"}`,
		AppState: appState,
	})
	if err != nil {
		t.Fatalf("invoke remote canonical skill: %v", err)
	}
	if len(result.NewMessages) != 1 {
		t.Fatalf("new messages = %#v, want injected canonical skill message", result.NewMessages)
	}
	if !strings.Contains(result.NewMessages[0].Content, "Remote canonical body for diff.") {
		t.Fatalf("content = %q, want remote canonical body with args", result.NewMessages[0].Content)
	}
	var parsed map[string]any
	if err := json.Unmarshal([]byte(result.Output), &parsed); err != nil {
		t.Fatalf("skill output JSON: %v\n%s", err, result.Output)
	}
	if parsed["commandName"] != "_canonical_review" || parsed["status"] != "inline" {
		t.Fatalf("output = %#v, want inline remote canonical metadata", parsed)
	}
}

func TestBuildSkillListingAttachmentFormatsNewSkillsWithinBudget(t *testing.T) {
	skills := []tools.SkillCommand{
		{Name: "commit", Description: "Create a git commit", WhenToUse: "when changes should be committed"},
		{Name: "verbose", Description: strings.Repeat("x", 300)},
	}
	msg := tools.BuildSkillListingAttachmentMessage("skill-listing-1", "sess-1", skills, 16000, true)
	if msg.Role != "attachment" || msg.Subtype != "skill_listing" || !msg.IsMeta || !msg.IsVisibleInTranscriptOnly {
		t.Fatalf("message = %#v, want Claude-style skill_listing attachment metadata", msg)
	}
	var payload map[string]any
	if err := json.Unmarshal([]byte(msg.Content), &payload); err != nil {
		t.Fatalf("skill listing payload JSON: %v", err)
	}
	if payload["type"] != "skill_listing" || payload["skillCount"] != float64(2) || payload["isInitial"] != true {
		t.Fatalf("payload = %#v, want skill_listing count metadata", payload)
	}
	content, _ := payload["content"].(string)
	if !strings.Contains(content, "- commit: Create a git commit - when changes should be committed") {
		t.Fatalf("content = %q, want description plus when-to-use", content)
	}
	if strings.Contains(content, strings.Repeat("x", 260)) {
		t.Fatalf("content = %q, want long descriptions capped like Claude", content)
	}
}

func TestFormatSkillListingFallsBackToNamesOnlyWhenBudgetIsTiny(t *testing.T) {
	skills := []tools.SkillCommand{
		{Name: "alpha", Description: strings.Repeat("a", 200)},
		{Name: "beta", Description: strings.Repeat("b", 200)},
	}
	got := tools.FormatSkillListingWithinBudget(skills, 1)
	if got != "- alpha\n- beta" {
		t.Fatalf("listing = %q, want names-only fallback", got)
	}
}

func testStringList(value any) []string {
	switch typed := value.(type) {
	case []string:
		return append([]string(nil), typed...)
	case []any:
		out := make([]string, 0, len(typed))
		for _, item := range typed {
			if text, ok := item.(string); ok {
				out = append(out, text)
			}
		}
		return out
	default:
		return nil
	}
}

func TestDiscoverSkillDirsForPathsFindsNestedDirsAndSkipsNodeModules(t *testing.T) {
	cwd := filepath.Join(t.TempDir(), "workspace")
	nested := filepath.Join(cwd, "pkg", "feature")
	nestedSkillDir := filepath.Join(nested, ".claude", "skills")
	cwdSkillDir := filepath.Join(cwd, ".claude", "skills")
	nodeModules := filepath.Join(cwd, "node_modules", "pkg")
	nodeModulesSkillDir := filepath.Join(nodeModules, ".claude", "skills")

	for _, dir := range []string{nestedSkillDir, cwdSkillDir, nodeModulesSkillDir} {
		if err := os.MkdirAll(filepath.Join(dir, "demo"), 0o755); err != nil {
			t.Fatalf("mkdir skill dir %q: %v", dir, err)
		}
		if err := os.WriteFile(filepath.Join(dir, "demo", "SKILL.md"), []byte("---\n---\nbody"), 0o644); err != nil {
			t.Fatalf("write skill dir %q: %v", dir, err)
		}
	}

	got := tools.DiscoverSkillDirsForPaths([]string{
		filepath.Join(nested, "file.go"),
		filepath.Join(nodeModules, "file.go"),
	}, cwd)

	want := []string{nestedSkillDir}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("discover skill dirs = %#v, want %#v", got, want)
	}
}

func TestDiscoverSkillDirsForPathsStopsAtGitignoredSkillDir(t *testing.T) {
	cwd := filepath.Join(t.TempDir(), "workspace")
	nested := filepath.Join(cwd, "pkg", "feature")
	nestedSkillDir := filepath.Join(nested, ".claude", "skills")
	if err := os.MkdirAll(filepath.Join(nestedSkillDir, "demo"), 0o755); err != nil {
		t.Fatalf("mkdir nested skill dir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(nestedSkillDir, "demo", "SKILL.md"), []byte("---\n---\nbody"), 0o644); err != nil {
		t.Fatalf("write nested skill: %v", err)
	}
	if err := os.WriteFile(filepath.Join(nested, ".gitignore"), []byte(".claude/skills\n"), 0o644); err != nil {
		t.Fatalf("write gitignore: %v", err)
	}

	got := tools.DiscoverSkillDirsForPaths([]string{filepath.Join(nested, "file.go")}, cwd)
	if len(got) != 0 {
		t.Fatalf("discover skill dirs = %#v, want gitignored nested skill dir pruned", got)
	}
}

func TestAddSkillDirectoriesLoadsDynamicSkillsAndActivatesConditionalSkills(t *testing.T) {
	tools.ClearDynamicSkills()
	t.Cleanup(tools.ClearDynamicSkills)

	cwd := filepath.Join(t.TempDir(), "workspace")
	skillRoot := filepath.Join(cwd, "pkg", ".claude", "skills")
	unconditionalDir := filepath.Join(skillRoot, "alpha")
	conditionalDir := filepath.Join(skillRoot, "beta")
	for _, dir := range []string{unconditionalDir, conditionalDir} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatalf("mkdir %q: %v", dir, err)
		}
	}
	if err := os.WriteFile(filepath.Join(unconditionalDir, "SKILL.md"), []byte("---\nname: alpha-display\n---\nAlpha body."), 0o644); err != nil {
		t.Fatalf("write alpha skill: %v", err)
	}
	if err := os.WriteFile(filepath.Join(conditionalDir, "SKILL.md"), []byte("---\npaths:\n  - src/**/*.go\n---\nBeta body."), 0o644); err != nil {
		t.Fatalf("write beta skill: %v", err)
	}

	tools.AddSkillDirectories([]string{skillRoot})

	got := tools.GetDynamicSkills()
	if len(got) != 1 || got[0].Name != "alpha" {
		t.Fatalf("dynamic skills = %#v, want unconditional skill only before activation", got)
	}

	activated := tools.ActivateConditionalSkillsForPaths([]string{
		filepath.Join(cwd, "src", "main.go"),
	}, cwd)
	if !reflect.DeepEqual(activated, []string{"beta"}) {
		t.Fatalf("activated = %#v, want beta", activated)
	}

	got = tools.GetDynamicSkills()
	if len(got) != 2 {
		t.Fatalf("dynamic skills after activation = %#v, want 2 skills", got)
	}
	names := []string{got[0].Name, got[1].Name}
	if !containsAllStrings(names, []string{"alpha", "beta"}) {
		t.Fatalf("dynamic skill names = %#v, want alpha and beta", names)
	}
}

func TestSkillToolResolvesSkillFromAppStateDynamicSkills(t *testing.T) {
	tool := tools.NewSkillTool()
	appState := map[string]any{
		"dynamicSkills": map[string]any{
			"app-dynamic": map[string]any{
				"name":    "app-dynamic",
				"path":    filepath.Join(t.TempDir(), "app-dynamic", "SKILL.md"),
				"content": "App-state dynamic skill body.",
			},
		},
	}

	result, err := tool.InvokeWithContext(context.Background(), tools.ToolUseContext{
		Input:    `{"skill":"app-dynamic","args":"demo args"}`,
		AppState: appState,
	})
	if err != nil {
		t.Fatalf("invoke dynamic skill: %v", err)
	}
	var parsed map[string]any
	if err := json.Unmarshal([]byte(result.Output), &parsed); err != nil {
		t.Fatalf("dynamic skill output JSON: %v\n%s", err, result.Output)
	}
	content := ""
	if len(result.NewMessages) > 0 {
		content = result.NewMessages[0].Content
	}
	if !strings.Contains(content, "App-state dynamic skill body.") || !strings.Contains(content, "demo args") {
		t.Fatalf("content = %q, want appState dynamic skill content and args", content)
	}
}

func TestSkillToolResolvesResourceBackedMCPSkillFromAppState(t *testing.T) {
	tool := tools.NewSkillTool()
	appState := map[string]any{
		"mcpSkills": map[string][]tools.SkillCommand{
			"filesystem": {{
				Name:          "filesystem:review",
				Description:   "Review filesystem edits.",
				Content:       "Review MCP skill for $target.",
				Source:        "mcp",
				LoadedFrom:    "mcp",
				ArgumentNames: []string{"target"},
				AllowedTools:  []string{"Read"},
			}},
		},
	}

	result, err := tool.InvokeWithContext(context.Background(), tools.ToolUseContext{
		Session:  session.Session{ID: "sess-mcp-skill", AgentID: "agent-mcp-skill"},
		ToolName: "Skill",
		Input:    `{"skill":"filesystem:review","args":"README.md"}`,
		AppState: appState,
	})
	if err != nil {
		t.Fatalf("invoke mcp skill: %v", err)
	}
	if len(result.NewMessages) != 1 || !strings.Contains(result.NewMessages[0].Content, "Review MCP skill for README.md.") {
		t.Fatalf("new messages = %#v, want inline MCP skill message with rendered args", result.NewMessages)
	}
	var parsed map[string]any
	if err := json.Unmarshal([]byte(result.Output), &parsed); err != nil {
		t.Fatalf("skill output JSON: %v\n%s", err, result.Output)
	}
	if parsed["commandName"] != "filesystem:review" || parsed["status"] != "inline" {
		t.Fatalf("output = %#v, want inline MCP skill metadata", parsed)
	}
}

func TestSkillToolExecutesInlineAndFencedShellBlocksAndExposesHooks(t *testing.T) {
	dir := t.TempDir()
	skillDir := filepath.Join(dir, "skills", "shelly")
	if err := os.MkdirAll(skillDir, 0o755); err != nil {
		t.Fatalf("mkdir skill dir: %v", err)
	}
	path := filepath.Join(skillDir, "SKILL.md")
	tick := string('`')
	contents := "---\n" +
		"shell: powershell\n" +
		"hooks:\n" +
		"  PreToolUse:\n" +
		"    - matcher: \"*\"\n" +
		"      command: echo hook\n" +
		"---\n" +
		"Shell inline: !" + tick + "echo inline" + tick + "\n\n" +
		"Shell block:\n" +
		tick + tick + tick + "!\n" +
		"echo block\n" +
		tick + tick + tick + "\n"
	if err := os.WriteFile(path, []byte(contents), 0o644); err != nil {
		t.Fatalf("write skill: %v", err)
	}

	var calls []tools.SkillShellRequest
	appState := map[string]any{
		"skillRoots": []any{filepath.Join(dir, "skills")},
		"skillShellExecutor": tools.SkillShellExecutor(func(_ context.Context, request tools.SkillShellRequest) (string, error) {
			calls = append(calls, request)
			return "[" + string(request.Shell) + "] " + strings.TrimSpace(request.Command), nil
		}),
	}

	result, err := tools.NewSkillTool().InvokeWithContext(context.Background(), tools.ToolUseContext{
		Session:  session.Session{ID: "session-shell", AgentID: "agent-shell"},
		ToolName: "Skill",
		Input:    `{"skill":"shelly","args":"alpha beta"}`,
		AppState: appState,
		SetAppState: func(update func(map[string]any) map[string]any) {
			appState = update(appState)
		},
	})
	if err != nil {
		t.Fatalf("invoke shell skill: %v", err)
	}
	if len(calls) != 2 {
		t.Fatalf("shell calls = %#v, want inline and fenced execution", calls)
	}
	for _, call := range calls {
		if call.Shell != tools.FrontmatterShellPowershell {
			t.Fatalf("shell call = %#v, want powershell frontmatter selection", call)
		}
	}
	var parsed map[string]any
	if err := json.Unmarshal([]byte(result.Output), &parsed); err != nil {
		t.Fatalf("shell skill output JSON: %v\n%s", err, result.Output)
	}
	content := ""
	if len(result.NewMessages) > 0 {
		content = result.NewMessages[0].Content
	}
	if !strings.Contains(content, "[powershell] echo inline") || !strings.Contains(content, "[powershell] echo block") {
		t.Fatalf("content = %q, want shell execution output replacements", content)
	}
	if hooks, ok := parsed["hooks"].(map[string]any); !ok || hooks["raw"] == "" {
		t.Fatalf("parsed hooks = %#v, want hooks metadata surfaced in output", parsed["hooks"])
	}
	invoked, _ := appState["invokedSkills"].([]any)
	if len(invoked) != 1 {
		t.Fatalf("invoked skills = %#v, want one recorded skill", invoked)
	}
	record := invoked[0].(map[string]any)
	if record["hooks"] == nil {
		t.Fatalf("invoked skill record = %#v, want hooks metadata captured in context", record)
	}
}

func TestSkillToolKeepsDirectoryNameWhenFrontmatterHasDisplayName(t *testing.T) {
	dir := t.TempDir()
	root := filepath.Join(dir, "skills")
	skillDir := filepath.Join(root, "verify")
	if err := os.MkdirAll(skillDir, 0o755); err != nil {
		t.Fatalf("mkdir skill: %v", err)
	}
	if err := os.WriteFile(filepath.Join(skillDir, "SKILL.md"), []byte("---\nname: Verification Display\ndescription: Run verification.\n---\nUse this skill."), 0o644); err != nil {
		t.Fatalf("write skill: %v", err)
	}
	result, err := tools.NewSkillTool().InvokeWithContext(context.Background(), tools.ToolUseContext{
		Input:    `{"skill":"verify"}`,
		AppState: map[string]any{"skillRoots": []any{root}},
	})
	if err != nil {
		t.Fatalf("invoke skill: %v", err)
	}
	var parsed map[string]any
	if err := json.Unmarshal([]byte(result.Output), &parsed); err != nil {
		t.Fatalf("skill output JSON: %v\n%s", err, result.Output)
	}
	if parsed["commandName"] != "verify" {
		t.Fatalf("commandName = %q, want directory skill name", parsed["commandName"])
	}
}

func TestParseSkillFileSupportsYAMLishFrontmatterAndBraceExpansion(t *testing.T) {
	path := filepath.Join(t.TempDir(), "skills", "verify", "SKILL.md")
	command := tools.ParseSkillFile("verify", path, `---
name: Verification Display
description: Run verification.
when_to_use: Use this when you want verification.
version: 1.2.3
user-invocable: true
allowed-tools:
  - Read
  - Grep
argument-hint: <target> <pattern>
arguments:
  - target
  - pattern
model: inherit
disable-model-invocation: false
context: fork
agent: verifier
effort: high
paths:
  - src/**/*.{ts,tsx}
  - docs/**
shell: powershell
---
Body text.
`)

	if command.Name != "verify" {
		t.Fatalf("name = %q, want directory-derived command name", command.Name)
	}
	if command.DisplayName != "Verification Display" {
		t.Fatalf("displayName = %q, want frontmatter display name", command.DisplayName)
	}
	if command.Description != "Run verification." {
		t.Fatalf("description = %q, want parsed frontmatter description", command.Description)
	}
	if command.WhenToUse != "Use this when you want verification." {
		t.Fatalf("whenToUse = %q, want parsed frontmatter when_to_use", command.WhenToUse)
	}
	if command.Version != "1.2.3" {
		t.Fatalf("version = %q, want parsed frontmatter version", command.Version)
	}
	if !command.UserInvocable {
		t.Fatal("userInvocable = false, want true")
	}
	if !reflect.DeepEqual(command.AllowedTools, []string{"Read", "Grep"}) {
		t.Fatalf("allowedTools = %#v, want parsed list", command.AllowedTools)
	}
	if command.ArgumentHint != "<target> <pattern>" {
		t.Fatalf("argumentHint = %q, want parsed argument hint", command.ArgumentHint)
	}
	if !reflect.DeepEqual(command.ArgumentNames, []string{"target", "pattern"}) {
		t.Fatalf("argumentNames = %#v, want parsed argument names", command.ArgumentNames)
	}
	if command.Model != "" {
		t.Fatalf("model = %q, want inherit to clear model override", command.Model)
	}
	if command.DisableModelInvocation {
		t.Fatal("disableModelInvocation = true, want false")
	}
	if command.Context != "fork" {
		t.Fatalf("context = %q, want fork", command.Context)
	}
	if command.Agent != "verifier" {
		t.Fatalf("agent = %q, want verifier", command.Agent)
	}
	if command.Effort != "high" {
		t.Fatalf("effort = %q, want parsed effort", command.Effort)
	}
	if !reflect.DeepEqual(command.Paths, []string{"src/**/*.ts", "src/**/*.tsx", "docs/**"}) {
		t.Fatalf("paths = %#v, want brace-expanded glob list", command.Paths)
	}
	if command.Shell != "powershell" {
		t.Fatalf("shell = %q, want parsed shell", command.Shell)
	}
	if command.Content != "Body text.\n" {
		t.Fatalf("content = %q, want body without frontmatter", command.Content)
	}
}

func TestSubstituteSkillArgumentsUsesShellAwareParsingAndBoundaryMatching(t *testing.T) {
	got := tools.SubstituteSkillArguments(
		`first=$0 second=$1 named=$target keep=$targetX args=$ARGUMENTS indexed=$ARGUMENTS[0] nochange=$ARGUMENTS[99]`,
		`alpha "beta gamma"`,
		true,
		[]string{"target"},
	)

	if !strings.Contains(got, "first=alpha") {
		t.Fatalf("output = %q, want $0 substitution", got)
	}
	if !strings.Contains(got, "second=beta gamma") {
		t.Fatalf("output = %q, want quoted args to stay intact as one token", got)
	}
	if !strings.Contains(got, "named=alpha") {
		t.Fatalf("output = %q, want named substitution", got)
	}
	if !strings.Contains(got, "keep=$targetX") {
		t.Fatalf("output = %q, want boundary-safe named substitution", got)
	}
	if !strings.Contains(got, `args=alpha "beta gamma"`) {
		t.Fatalf("output = %q, want $ARGUMENTS to keep raw shell text", got)
	}
	if !strings.Contains(got, "indexed=alpha") {
		t.Fatalf("output = %q, want indexed substitution", got)
	}
	if !strings.Contains(got, "nochange=") {
		t.Fatalf("output = %q, want out-of-range indexes to become empty", got)
	}
	if strings.Contains(got, "ARGUMENTS: alpha \"beta gamma\"") {
		t.Fatalf("output = %q, did not want append when placeholders were present", got)
	}

	appended := tools.SubstituteSkillArguments("No placeholders here.", "alpha beta", true, nil)
	if !strings.Contains(appended, "ARGUMENTS: alpha beta") {
		t.Fatalf("output = %q, want append when no placeholders are present", appended)
	}

	empty := tools.SubstituteSkillArguments("No placeholders here.", "", true, nil)
	if empty != "No placeholders here." {
		t.Fatalf("output = %q, want empty args to leave content unchanged", empty)
	}

	exactNamed := tools.SubstituteSkillArguments("$target", "alpha", true, []string{"target"})
	if exactNamed != "alpha" {
		t.Fatalf("output = %q, want exact named argument at end to be replaced", exactNamed)
	}
}

func TestParseSkillFileDefaultsUserInvocableToTrue(t *testing.T) {
	command := tools.ParseSkillFile("verify", filepath.Join(t.TempDir(), "SKILL.md"), "Use this skill.")
	if !command.UserInvocable {
		t.Fatal("userInvocable = false, want Claude default true")
	}
}

func TestSkillToolRejectsDisableModelInvocationAndReportsForkedSkills(t *testing.T) {
	dir := t.TempDir()
	root := filepath.Join(dir, "skills")
	blockedDir := filepath.Join(root, "blocked")
	if err := os.MkdirAll(blockedDir, 0o755); err != nil {
		t.Fatalf("mkdir blocked skill: %v", err)
	}
	if err := os.WriteFile(filepath.Join(blockedDir, "SKILL.md"), []byte("---\ndisable-model-invocation: true\n---\nSecret."), 0o644); err != nil {
		t.Fatalf("write blocked skill: %v", err)
	}
	tool := tools.NewSkillTool()
	_, err := tool.InvokeWithContext(context.Background(), tools.ToolUseContext{
		Input:    `{"skill":"blocked"}`,
		AppState: map[string]any{"skillRoots": []any{root}},
	})
	if err == nil || !strings.Contains(err.Error(), "disable-model-invocation") {
		t.Fatalf("error = %v, want disable-model-invocation rejection", err)
	}

	forkDir := filepath.Join(root, "forked")
	if err := os.MkdirAll(forkDir, 0o755); err != nil {
		t.Fatalf("mkdir forked skill: %v", err)
	}
	if err := os.WriteFile(filepath.Join(forkDir, "SKILL.md"), []byte("---\ncontext: fork\nagent: verifier\n---\nFork this."), 0o644); err != nil {
		t.Fatalf("write forked skill: %v", err)
	}
	result, err := tool.InvokeWithContext(context.Background(), tools.ToolUseContext{
		AgentID:  "agent-main",
		Input:    `{"skill":"forked"}`,
		AppState: map[string]any{"skillRoots": []any{root}},
	})
	if err != nil {
		t.Fatalf("forked skill: %v", err)
	}
	var parsed map[string]any
	if err := json.Unmarshal([]byte(result.Output), &parsed); err != nil {
		t.Fatalf("forked output JSON: %v", err)
	}
	if parsed["status"] != "forked" || parsed["agent"] != "verifier" {
		t.Fatalf("output = %#v, want forked skill metadata", parsed)
	}
}

func TestSkillToolExpandsArgumentsSessionAndSkillDirPlaceholders(t *testing.T) {
	dir := t.TempDir()
	skillDir := filepath.Join(dir, "skills", "expand")
	if err := os.MkdirAll(skillDir, 0o755); err != nil {
		t.Fatalf("mkdir skill: %v", err)
	}
	path := filepath.Join(skillDir, "SKILL.md")
	if err := os.WriteFile(path, []byte("---\narguments: first second\n---\nskill=${CLAUDE_SKILL_DIR}\nsession=${CLAUDE_SESSION_ID}\nargs=$ARGUMENTS\nfirst=$0\nsecond=$1"), 0o644); err != nil {
		t.Fatalf("write skill: %v", err)
	}
	tool := tools.NewSkillTool()
	result, err := tool.InvokeWithContext(context.Background(), tools.ToolUseContext{
		Session:  session.Session{ID: "session-123", AgentID: "agent-1"},
		ToolName: "Skill",
		Input:    `{"skill":"expand","args":"alpha beta"}`,
		AppState: map[string]any{"skillRoots": []any{filepath.Join(dir, "skills")}},
	})
	if err != nil {
		t.Fatalf("invoke skill: %v", err)
	}
	var parsed map[string]any
	if err := json.Unmarshal([]byte(result.Output), &parsed); err != nil {
		t.Fatalf("skill output JSON: %v\n%s", err, result.Output)
	}
	content := ""
	if len(result.NewMessages) > 0 {
		content = result.NewMessages[0].Content
	}
	if !strings.Contains(content, "Base directory for this skill: "+skillDir) {
		t.Fatalf("content = %q, want skill base directory prefix", content)
	}
	if !strings.Contains(content, "skill="+filepath.ToSlash(skillDir)) {
		t.Fatalf("content = %q, want skill dir expansion", content)
	}
	if !strings.Contains(content, "session=session-123") {
		t.Fatalf("content = %q, want session ID expansion", content)
	}
	if !strings.Contains(content, "args=alpha beta") || !strings.Contains(content, "first=alpha") || !strings.Contains(content, "second=beta") {
		t.Fatalf("content = %q, want argument expansion", content)
	}
}

func TestSkillInvokedSkillsAreAgentScoped(t *testing.T) {
	appState := map[string]any{}
	tools.AddInvokedSkill(appState, tools.InvokedSkillInfo{
		SkillName: "alpha",
		SkillPath: "/tmp/alpha/SKILL.md",
		Content:   "alpha content",
		AgentID:   "agent-a",
	})
	tools.AddInvokedSkill(appState, tools.InvokedSkillInfo{
		SkillName: "beta",
		SkillPath: "/tmp/beta/SKILL.md",
		Content:   "beta content",
		AgentID:   "agent-b",
	})

	agentA := tools.GetInvokedSkillsForAgent(appState, "agent-a")
	if len(agentA) != 1 || agentA[0].SkillName != "alpha" {
		t.Fatalf("agent-a skills = %#v, want alpha only", agentA)
	}

	tools.ClearInvokedSkillsForAgent(appState, "agent-a")
	agentA = tools.GetInvokedSkillsForAgent(appState, "agent-a")
	if len(agentA) != 0 {
		t.Fatalf("agent-a skills after clear = %#v, want none", agentA)
	}
	agentB := tools.GetInvokedSkillsForAgent(appState, "agent-b")
	if len(agentB) != 1 || agentB[0].SkillName != "beta" {
		t.Fatalf("agent-b skills = %#v, want beta preserved", agentB)
	}
}

func containsAllStrings(values []string, expected []string) bool {
	seen := make(map[string]struct{}, len(values))
	for _, value := range values {
		seen[value] = struct{}{}
	}
	for _, value := range expected {
		if _, ok := seen[value]; !ok {
			return false
		}
	}
	return true
}

func TestSkillToolUsesForkExecutorHookFromAppState(t *testing.T) {
	dir := t.TempDir()
	root := filepath.Join(dir, "skills")
	forkDir := filepath.Join(root, "forked")
	if err := os.MkdirAll(forkDir, 0o755); err != nil {
		t.Fatalf("mkdir forked skill: %v", err)
	}
	if err := os.WriteFile(filepath.Join(forkDir, "SKILL.md"), []byte("---\ncontext: fork\nagent: verifier\nallowed-tools: Read,Write\n---\nFork this."), 0o644); err != nil {
		t.Fatalf("write forked skill: %v", err)
	}
	called := false
	appState := map[string]any{
		"skillRoots": []any{root},
		"skillForkExecutor": func(_ context.Context, request tools.SkillForkRequest) (tools.ToolResult, error) {
			called = true
			if request.Command.Name != "forked" || request.Command.Agent != "verifier" {
				t.Fatalf("request = %#v, want forked verifier skill", request)
			}
			if request.Args != "" {
				t.Fatalf("request args = %q, want empty", request.Args)
			}
			return tools.ToolResult{Output: `{"status":"forked","agent":"verifier","result":"executed"}`}, nil
		},
	}
	result, err := tools.NewSkillTool().InvokeWithContext(context.Background(), tools.ToolUseContext{
		Session:  session.Session{ID: "session-1", AgentID: "agent-main"},
		ToolName: "Skill",
		Input:    `{"skill":"forked"}`,
		AppState: appState,
	})
	if err != nil {
		t.Fatalf("invoke forked skill: %v", err)
	}
	if !called {
		t.Fatal("expected fork executor hook to be called")
	}
	if !strings.Contains(result.Output, `"status":"forked"`) || !strings.Contains(result.Output, `"agent":"verifier"`) {
		t.Fatalf("output = %q, want fork executor output", result.Output)
	}
}

func TestWebSearchRejectsConflictingDomainFilters(t *testing.T) {
	tool := tools.NewWebSearchTool()
	_, err := tool.Invoke(context.Background(), session.Session{}, `{"query":"golang","allowed_domains":["go.dev"],"blocked_domains":["example.com"]}`)
	if err == nil || !strings.Contains(err.Error(), "allowed_domains and blocked_domains cannot both be set") {
		t.Fatalf("error = %v, want conflicting domain filters rejection", err)
	}
}

func TestReadToolFormatsNotebookCells(t *testing.T) {
	path := filepath.Join(t.TempDir(), "demo.ipynb")
	notebook := `{"cells":[{"cell_type":"markdown","id":"intro","source":["# Title\n"],"metadata":{}},{"cell_type":"code","id":"run","source":["print(1)\n"],"metadata":{},"outputs":[],"execution_count":7}],"metadata":{},"nbformat":4,"nbformat_minor":5}`
	if err := os.WriteFile(path, []byte(notebook), 0o644); err != nil {
		t.Fatalf("write notebook: %v", err)
	}
	tool := tools.NewReadTool()
	got, err := tool.Invoke(context.Background(), session.Session{}, `{"file_path":"`+strings.ReplaceAll(path, `\`, `\\`)+`"}`)
	if err != nil {
		t.Fatalf("read notebook: %v", err)
	}
	if !strings.Contains(got, `<cell id="intro"`) || !strings.Contains(got, `cellType="markdown"`) || !strings.Contains(got, "print(1)") {
		t.Fatalf("notebook read output = %q, want cell-formatted notebook", got)
	}
	if strings.Contains(got, `"cells"`) {
		t.Fatalf("notebook read output = %q, did not want raw JSON", got)
	}
}

func TestNotebookEditToolRejectsUntilNotebookBackendExists(t *testing.T) {
	path := filepath.Join(t.TempDir(), "demo.ipynb")
	notebook := `{"cells":[{"cell_type":"code","id":"cell-1","source":["print(0)\n"],"metadata":{},"outputs":[{"output_type":"stream","text":["old\n"]}],"execution_count":7}],"metadata":{},"nbformat":4,"nbformat_minor":5}`
	if err := os.WriteFile(path, []byte(notebook), 0o644); err != nil {
		t.Fatalf("write notebook: %v", err)
	}
	tool := tools.NewNotebookEditTool()
	got, err := tool.Invoke(context.Background(), session.Session{}, `{"notebook_path":"`+strings.ReplaceAll(path, `\`, `\\`)+`","cell_id":"cell-1","new_source":"print(1)","edit_mode":"replace"}`)
	if err != nil {
		t.Fatalf("notebook edit: %v", err)
	}
	if !strings.Contains(got, "Notebook edited") || !strings.Contains(got, "cell-1") {
		t.Fatalf("output = %q, want edit summary", got)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read notebook: %v", err)
	}
	if !strings.Contains(string(data), "print(1)") || strings.Contains(string(data), "print(0)") {
		t.Fatalf("notebook = %s, want replaced source", string(data))
	}
	var parsed map[string]any
	if err := json.Unmarshal(data, &parsed); err != nil {
		t.Fatalf("parse notebook: %v", err)
	}
	cell := parsed["cells"].([]any)[0].(map[string]any)
	if cell["execution_count"] != nil {
		t.Fatalf("cell = %#v, want execution_count cleared after code edit", cell)
	}
	outputs, _ := cell["outputs"].([]any)
	if len(outputs) != 0 {
		t.Fatalf("cell = %#v, want outputs cleared after code edit", cell)
	}
}

func TestNotebookEditRequiresIPynbAndInsertCellType(t *testing.T) {
	tool := tools.NewNotebookEditTool()
	textPath := filepath.Join(t.TempDir(), "notes.txt")
	if err := os.WriteFile(textPath, []byte("notebook?"), 0o644); err != nil {
		t.Fatalf("write text: %v", err)
	}
	_, err := tool.Invoke(context.Background(), session.Session{}, `{"notebook_path":"`+strings.ReplaceAll(textPath, `\`, `\\`)+`","new_source":"print(1)"}`)
	if err == nil || !strings.Contains(err.Error(), ".ipynb") {
		t.Fatalf("error = %v, want non-ipynb rejection", err)
	}

	notebookPath := filepath.Join(t.TempDir(), "demo.ipynb")
	if err := os.WriteFile(notebookPath, []byte(`{"cells":[],"metadata":{},"nbformat":4,"nbformat_minor":5}`), 0o644); err != nil {
		t.Fatalf("write notebook: %v", err)
	}
	_, err = tool.Invoke(context.Background(), session.Session{}, `{"notebook_path":"`+strings.ReplaceAll(notebookPath, `\`, `\\`)+`","new_source":"# new","edit_mode":"insert"}`)
	if err == nil || !strings.Contains(err.Error(), "cell_type") {
		t.Fatalf("error = %v, want insert cell_type rejection", err)
	}
}

func TestPlanModeToolsReturnModeTransitionMarkers(t *testing.T) {
	enter := tools.NewEnterPlanModeTool()
	exit := tools.NewExitPlanModeTool()
	if exit.Definition().ReadOnly {
		t.Fatal("ExitPlanMode must not be read-only because Claude writes/approves a plan transition")
	}
	properties := exit.Definition().InputSchema["properties"].(map[string]any)
	if properties["allowedPrompts"] == nil {
		t.Fatalf("exit schema = %#v, want allowedPrompts", exit.Definition().InputSchema)
	}

	entered, err := enter.Invoke(context.Background(), session.Session{}, `{"plan":"inspect first"}`)
	if err != nil {
		t.Fatalf("enter plan mode: %v", err)
	}
	if !strings.Contains(entered, "Plan mode entered") {
		t.Fatalf("enter output = %q, want plan mode marker", entered)
	}
	exited, err := exit.Invoke(context.Background(), session.Session{}, `{"plan":"approved"}`)
	if err != nil {
		t.Fatalf("exit plan mode: %v", err)
	}
	if !strings.Contains(exited, "Plan mode exited") {
		t.Fatalf("exit output = %q, want plan mode exit marker", exited)
	}
}

func TestExitPlanModeContextWritesPlanAndRestoresAppState(t *testing.T) {
	path := filepath.Join(t.TempDir(), "plan.md")
	appState := map[string]any{
		"toolPermissionContext": map[string]any{
			"mode":        "plan",
			"prePlanMode": "default",
		},
	}
	tool := tools.NewExitPlanModeTool()
	result, err := tool.(tools.ContextualTool).InvokeWithContext(context.Background(), tools.ToolUseContext{
		Input:    `{"plan":"1. Run tests","planFilePath":"` + strings.ReplaceAll(path, `\`, `\\`) + `","allowedPrompts":[{"tool":"Bash","prompt":"run tests"}]}`,
		AppState: appState,
		SetAppState: func(update func(map[string]any) map[string]any) {
			appState = update(appState)
		},
	})
	if err != nil {
		t.Fatalf("exit plan mode: %v", err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read plan file: %v", err)
	}
	if string(data) != "1. Run tests" {
		t.Fatalf("plan file = %q, want written plan", string(data))
	}
	if !strings.Contains(result.Output, `"plan":"1. Run tests"`) || !strings.Contains(result.Output, `"allowedPrompts"`) {
		t.Fatalf("output = %q, want Claude-style plan output JSON", result.Output)
	}
	permissionContext := appState["toolPermissionContext"].(map[string]any)
	if permissionContext["mode"] != "default" || appState["hasExitedPlanMode"] != true || appState["needsPlanModeExitAttachment"] != true {
		t.Fatalf("appState = %#v, want restored mode and plan exit markers", appState)
	}
}

func TestExitPlanModeContextReadsPlanFromFileWhenInputPlanMissing(t *testing.T) {
	path := filepath.Join(t.TempDir(), "plan.md")
	if err := os.WriteFile(path, []byte("1. Existing disk plan"), 0o644); err != nil {
		t.Fatalf("write plan file: %v", err)
	}
	appState := map[string]any{
		"toolPermissionContext": map[string]any{
			"mode":        "plan",
			"prePlanMode": "acceptEdits",
		},
	}
	tool := tools.NewExitPlanModeTool()
	result, err := tool.(tools.ContextualTool).InvokeWithContext(context.Background(), tools.ToolUseContext{
		Input:    `{"planFilePath":"` + strings.ReplaceAll(path, `\`, `\\`) + `"}`,
		AppState: appState,
		SetAppState: func(update func(map[string]any) map[string]any) {
			appState = update(appState)
		},
	})
	if err != nil {
		t.Fatalf("exit plan mode: %v", err)
	}
	if !strings.Contains(result.Output, `"plan":"1. Existing disk plan"`) {
		t.Fatalf("output = %q, want plan recovered from disk", result.Output)
	}
	permissionContext := appState["toolPermissionContext"].(map[string]any)
	if permissionContext["mode"] != "acceptEdits" {
		t.Fatalf("appState = %#v, want restored pre-plan mode", appState)
	}
}

func TestExitPlanModeContextRejectsOutsidePlanModeWithoutMutatingState(t *testing.T) {
	appState := map[string]any{
		"toolPermissionContext": map[string]any{
			"mode": "default",
		},
	}
	tool := tools.NewExitPlanModeTool()
	_, err := tool.(tools.ContextualTool).InvokeWithContext(context.Background(), tools.ToolUseContext{
		Input:    `{"plan":"approved"}`,
		AppState: appState,
		SetAppState: func(update func(map[string]any) map[string]any) {
			appState = update(appState)
		},
	})
	if err == nil || !strings.Contains(err.Error(), "only for exiting plan mode") {
		t.Fatalf("error = %v, want outside-plan-mode rejection", err)
	}
	if appState["hasExitedPlanMode"] != nil || appState["needsPlanModeExitAttachment"] != nil {
		t.Fatalf("appState = %#v, should not mutate outside plan mode", appState)
	}
}
