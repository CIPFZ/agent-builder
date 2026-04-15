package tools_test

import (
	"bufio"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

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

func TestDiscoverMCPClientToolsAppliesHeadersHelperAndOverridesStaticHeaders(t *testing.T) {
	var gotStatic, gotDynamic, gotServer, gotURL string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var request map[string]any
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			t.Fatalf("decode request: %v", err)
		}
		gotStatic = r.Header.Get("X-Static")
		gotDynamic = r.Header.Get("X-Dynamic")
		gotServer = r.Header.Get("X-Server-Name")
		gotURL = r.Header.Get("X-Server-Url")
		switch request["method"] {
		case "initialize":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"jsonrpc": "2.0",
				"id":      request["id"],
				"result": map[string]any{
					"protocolVersion": "2024-11-05",
					"capabilities": map[string]any{
						"tools": map[string]any{},
					},
				},
			})
		default:
			_ = json.NewEncoder(w).Encode(map[string]any{
				"jsonrpc": "2.0",
				"id":      request["id"],
				"result":  map[string]any{"tools": []map[string]any{}},
			})
		}
	}))
	defer server.Close()

	result, err := tools.DiscoverMCPClientTools(context.Background(), []tools.MCPConnection{{
		Name:          "filesystem",
		Type:          "streamable_http",
		BaseURL:       server.URL,
		Headers:       map[string]string{"X-Static": "static"},
		HeadersHelper: os.Args[0] + " -test.run=TestMCPHeadersHelperProcess",
	}})
	if err != nil {
		t.Fatalf("discover mcp with headers helper: %v", err)
	}
	if len(result.Tools["filesystem"].Tools) != 0 {
		t.Fatalf("tools = %#v, want no tools", result.Tools["filesystem"].Tools)
	}
	if gotStatic != "dynamic" {
		t.Fatalf("X-Static = %q, want dynamic override", gotStatic)
	}
	if gotDynamic != "from-helper" {
		t.Fatalf("X-Dynamic = %q, want helper header", gotDynamic)
	}
	if gotServer != "filesystem" {
		t.Fatalf("X-Server-Name = %q, want server name env", gotServer)
	}
	if gotURL != server.URL {
		t.Fatalf("X-Server-Url = %q, want server URL env", gotURL)
	}
}

func TestDiscoverMCPClientToolsReturnsNeedsAuthPseudoTool(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("WWW-Authenticate", `Bearer authorization_uri="https://auth.example/authorize"`)
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = w.Write([]byte(`{"jsonrpc":"2.0","error":{"code":401,"message":"Unauthorized"}}`))
	}))
	defer server.Close()

	result, err := tools.DiscoverMCPClientTools(context.Background(), []tools.MCPConnection{{
		Name:    "filesystem",
		Type:    "streamable_http",
		BaseURL: server.URL,
	}})
	if err != nil {
		t.Fatalf("discover auth-required mcp: %v", err)
	}
	auth, ok := result.NeedsAuth["filesystem"]
	if !ok {
		t.Fatalf("needs-auth map = %#v, want filesystem entry", result.NeedsAuth)
	}
	if auth.Name != "mcp__filesystem__authenticate" || auth.Status != "needs-auth" {
		t.Fatalf("auth = %#v, want Claude-like pseudo tool metadata", auth)
	}
	if auth.AuthURL != "https://auth.example/authorize" {
		t.Fatalf("auth url = %q, want parsed authorization URL", auth.AuthURL)
	}
	if !strings.Contains(auth.Message, "authenticate") || !strings.Contains(auth.Message, "filesystem") {
		t.Fatalf("auth message = %q, want human-readable auth guidance", auth.Message)
	}
}

func TestMCPAuthToolReturnsAuthUrlAndNeedsAuthStatus(t *testing.T) {
	tool := tools.NewMCPAuthTool("filesystem", "https://auth.example/authorize", "Authenticate filesystem")
	result, err := tool.Invoke(context.Background(), session.Session{}, `{"server":"filesystem"}`)
	if err != nil {
		t.Fatalf("invoke auth tool: %v", err)
	}
	var parsed map[string]any
	if err := json.Unmarshal([]byte(result), &parsed); err != nil {
		t.Fatalf("auth output JSON: %v", err)
	}
	if parsed["status"] != "needs-auth" {
		t.Fatalf("parsed = %#v, want needs-auth status", parsed)
	}
	if parsed["authUrl"] != "https://auth.example/authorize" {
		t.Fatalf("parsed = %#v, want auth URL", parsed)
	}
	if !strings.Contains(parsed["message"].(string), "Authenticate filesystem") {
		t.Fatalf("parsed = %#v, want auth message", parsed)
	}
}

func TestDiscoverMCPClientToolsReconnectsAfterSessionExpiredToolCall(t *testing.T) {
	var initializeCount, toolListCount, callCount int
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var request map[string]any
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			t.Fatalf("decode request: %v", err)
		}
		method, _ := request["method"].(string)
		switch method {
		case "initialize":
			initializeCount++
			_ = json.NewEncoder(w).Encode(map[string]any{
				"jsonrpc": "2.0",
				"id":      request["id"],
				"result": map[string]any{
					"protocolVersion": "2024-11-05",
					"capabilities": map[string]any{
						"tools": map[string]any{},
					},
				},
			})
		case "notifications/initialized":
			w.WriteHeader(http.StatusNoContent)
		case "tools/list":
			toolListCount++
			_ = json.NewEncoder(w).Encode(map[string]any{
				"jsonrpc": "2.0",
				"id":      request["id"],
				"result": map[string]any{
					"tools": []map[string]any{{
						"name":        "read_file",
						"description": "Read a file",
						"inputSchema": map[string]any{
							"type": "object",
							"properties": map[string]any{
								"path": map[string]any{"type": "string"},
							},
						},
					}},
				},
			})
		case "tools/call":
			callCount++
			if callCount == 1 {
				w.WriteHeader(http.StatusNotFound)
				_, _ = w.Write([]byte(`{"jsonrpc":"2.0","error":{"code":-32001,"message":"Session not found"}}`))
				return
			}
			_ = json.NewEncoder(w).Encode(map[string]any{
				"jsonrpc": "2.0",
				"id":      request["id"],
				"result": map[string]any{
					"content": []map[string]any{{"type": "text", "text": "reconnected"}},
				},
			})
		default:
			t.Fatalf("unexpected method %q", method)
		}
	}))
	defer server.Close()

	result, err := tools.DiscoverMCPClientTools(context.Background(), []tools.MCPConnection{{
		Name:    "filesystem",
		Type:    "streamable_http",
		BaseURL: server.URL,
	}})
	if err != nil {
		t.Fatalf("discover mcp: %v", err)
	}

	out, err := result.Caller(context.Background(), "filesystem", "read_file", map[string]any{"path": "README.md"})
	if err != nil {
		t.Fatalf("tool call with reconnect: %v", err)
	}
	if got := firstTextContent(t, out.Content); got != "reconnected" {
		t.Fatalf("tool content = %q, want reconnected after retry", got)
	}
	if initializeCount < 2 || toolListCount < 2 || callCount < 2 {
		t.Fatalf("counts = init %d list %d call %d, want reconnect/retry", initializeCount, toolListCount, callCount)
	}
}

func TestDiscoverMCPClientToolsSendsClaudeToolUseIDMeta(t *testing.T) {
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
					"description": "Read a file",
					"inputSchema": map[string]any{"type": "object"},
				}}},
			})
		case "tools/call":
			params, _ := request["params"].(map[string]any)
			meta, _ := params["_meta"].(map[string]any)
			if meta["claudecode/toolUseId"] != "toolu-mcp-42" {
				t.Fatalf("tools/call params = %#v, want Claude toolUseId meta", params)
			}
			_ = json.NewEncoder(w).Encode(map[string]any{
				"jsonrpc": "2.0",
				"id":      request["id"],
				"result": map[string]any{
					"content": []map[string]any{{"type": "text", "text": "ok"}},
				},
			})
		default:
			t.Fatalf("unexpected method %q", method)
		}
	}))
	defer server.Close()

	result, err := tools.DiscoverMCPClientTools(context.Background(), []tools.MCPConnection{{
		Name:    "filesystem",
		Type:    "streamable_http",
		BaseURL: server.URL,
	}})
	if err != nil {
		t.Fatalf("discover mcp client: %v", err)
	}
	_, err = result.ContextualCaller(context.Background(), tools.MCPToolCallRequest{
		Server:    "filesystem",
		Name:      "read_file",
		Input:     map[string]any{"path": "README.md"},
		ToolUseID: "toolu-mcp-42",
	})
	if err != nil {
		t.Fatalf("call mcp tool: %v", err)
	}
}

func TestDiscoverMCPClientToolsRetriesUrlElicitationToolCall(t *testing.T) {
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
					"name":        "open_url",
					"description": "Open URL",
					"inputSchema": map[string]any{"type": "object"},
				}}},
			})
		case "tools/call":
			callCount++
			if callCount == 1 {
				_ = json.NewEncoder(w).Encode(map[string]any{
					"jsonrpc": "2.0",
					"id":      request["id"],
					"error": map[string]any{
						"code":    -32042,
						"message": "URL elicitation required",
						"data": map[string]any{
							"elicitations": []map[string]any{{"url": "https://example.com/auth", "message": "Open auth URL"}},
						},
					},
				})
				return
			}
			_ = json.NewEncoder(w).Encode(map[string]any{
				"jsonrpc": "2.0",
				"id":      request["id"],
				"result": map[string]any{
					"content": []map[string]any{{"type": "text", "text": "after elicitation"}},
				},
			})
		default:
			t.Fatalf("unexpected method %q", method)
		}
	}))
	defer server.Close()

	result, err := tools.DiscoverMCPClientTools(context.Background(), []tools.MCPConnection{{
		Name:    "browser",
		Type:    "streamable_http",
		BaseURL: server.URL,
	}})
	if err != nil {
		t.Fatalf("discover mcp client: %v", err)
	}
	var elicited bool
	toolResult, err := result.ContextualCaller(context.Background(), tools.MCPToolCallRequest{
		Server: "browser",
		Name:   "open_url",
		Input:  map[string]any{},
		HandleElicitation: func(_ context.Context, request tools.ElicitationRequest) (tools.ElicitationResult, error) {
			elicitations, _ := request.Params["elicitations"].([]any)
			if request.ServerName != "browser" || len(elicitations) != 1 {
				t.Fatalf("elicitation request = %#v, want server and one elicitation", request)
			}
			elicited = true
			return tools.ElicitationResult{Value: "accepted"}, nil
		},
	})
	if err != nil {
		t.Fatalf("call tool after elicitation: %v", err)
	}
	if !elicited || callCount != 2 {
		t.Fatalf("elicited=%v callCount=%d, want elicitation and retry", elicited, callCount)
	}
	if toolResult.Content[0]["text"] != "after elicitation" {
		t.Fatalf("tool result = %#v, want retry result", toolResult)
	}
}

func TestDiscoverMCPClientToolsRetriesSessionExpiredResourceRead(t *testing.T) {
	var readCount int
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
					"capabilities":    map[string]any{"resources": map[string]any{}},
				},
			})
		case "notifications/initialized":
			w.WriteHeader(http.StatusNoContent)
		case "tools/list":
			_ = json.NewEncoder(w).Encode(map[string]any{"jsonrpc": "2.0", "id": request["id"], "result": map[string]any{"tools": []map[string]any{}}})
		case "resources/list":
			_ = json.NewEncoder(w).Encode(map[string]any{"jsonrpc": "2.0", "id": request["id"], "result": map[string]any{"resources": []map[string]any{{"uri": "file:///a.txt"}}}})
		case "resources/read":
			readCount++
			if readCount == 1 {
				w.WriteHeader(http.StatusNotFound)
				_, _ = w.Write([]byte(`{"jsonrpc":"2.0","error":{"code":-32001,"message":"Session not found"}}`))
				return
			}
			_ = json.NewEncoder(w).Encode(map[string]any{
				"jsonrpc": "2.0",
				"id":      request["id"],
				"result":  map[string]any{"contents": []map[string]any{{"uri": "file:///a.txt", "text": "reconnected resource"}}},
			})
		default:
			t.Fatalf("unexpected method %q", method)
		}
	}))
	defer server.Close()

	result, err := tools.DiscoverMCPClientTools(context.Background(), []tools.MCPConnection{{
		Name:    "filesystem",
		Type:    "streamable_http",
		BaseURL: server.URL,
	}})
	if err != nil {
		t.Fatalf("discover mcp client: %v", err)
	}
	read, err := result.ResourceReader(context.Background(), "filesystem", "file:///a.txt")
	if err != nil {
		t.Fatalf("read resource after reconnect: %v", err)
	}
	if readCount != 2 || read.Contents[0].Text != "reconnected resource" {
		t.Fatalf("readCount=%d read=%#v, want retry result", readCount, read)
	}
}

func TestMCPToolResultNormalizesBlobImageAndResourcePayloads(t *testing.T) {
	blob := base64.StdEncoding.EncodeToString([]byte("blob-payload"))
	image := base64.StdEncoding.EncodeToString([]byte("image-payload"))
	resource := base64.StdEncoding.EncodeToString([]byte("resource-payload"))
	tool := tools.NewMCPTool(tools.MCPToolDefinition{
		Server: "filesystem",
		Name:   "read_binary",
	}, func(_ context.Context, server, name string, input map[string]any) (tools.MCPToolResult, error) {
		if server != "filesystem" || name != "read_binary" {
			t.Fatalf("caller args = %q %q", server, name)
		}
		return tools.MCPToolResult{Content: []map[string]any{
			{"type": "blob", "mimeType": "application/octet-stream", "blob": blob},
			{"type": "image", "mimeType": "image/png", "data": image},
			{"type": "resource", "resource": map[string]any{"uri": "file:///workspace/data.bin", "mimeType": "application/octet-stream", "blob": resource}},
		}}, nil
	})

	out, err := tool.Invoke(context.Background(), session.Session{}, `{"path":"data.bin"}`)
	if err != nil {
		t.Fatalf("invoke mcp tool: %v", err)
	}
	if strings.Contains(out, blob) || strings.Contains(out, image) || strings.Contains(out, resource) {
		t.Fatalf("output = %q, want binary payload saved to temp files", out)
	}
	if !strings.Contains(out, "blobSavedTo") {
		t.Fatalf("output = %q, want blobSavedTo markers", out)
	}
	if !strings.Contains(out, "myclaw-mcp-output") {
		t.Fatalf("output = %q, want temp output directory marker", out)
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

func TestMCPHeadersHelperProcess(t *testing.T) {
	if os.Getenv("GO_WANT_MCP_HEADERS_HELPER") != "1" {
		return
	}
	helperName := os.Getenv("CLAUDE_CODE_MCP_SERVER_NAME")
	helperURL := os.Getenv("CLAUDE_CODE_MCP_SERVER_URL")
	headers := map[string]string{
		"X-Dynamic":     "from-helper",
		"X-Static":      "dynamic",
		"X-Server-Name": helperName,
		"X-Server-Url":  helperURL,
	}
	encoded, err := json.Marshal(headers)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	_, _ = os.Stdout.Write(append(encoded, '\n'))
	os.Exit(0)
}

func TestDiscoverMCPClientToolsSupportsStdioToolsAndPrompts(t *testing.T) {
	connection := tools.MCPConnection{
		Name:    "stdio-server",
		Type:    "stdio",
		Command: os.Args[0],
		Args:    []string{"-test.run=TestMCPStdioHelperProcess"},
		Env: map[string]string{
			"GO_WANT_HELPER_PROCESS": "1",
			"MCP_ENV_TOKEN":          "env-from-parent",
		},
	}

	result, err := tools.DiscoverMCPClientTools(context.Background(), []tools.MCPConnection{connection})
	if err != nil {
		t.Fatalf("discover stdio MCP: %v", err)
	}

	toolList, ok := result.Tools["stdio-server"]
	if !ok {
		t.Fatalf("tool server missing from discovery result: %#v", result.Tools)
	}
	if len(toolList.Tools) != 1 || toolList.Tools[0].Name != "echo_env" {
		t.Fatalf("tool list = %#v, want echo_env", toolList.Tools)
	}

	promptList, ok := result.Prompts["stdio-server"]
	if !ok {
		t.Fatalf("prompt server missing from discovery result: %#v", result.Prompts)
	}
	if len(promptList.Prompts) != 1 || promptList.Prompts[0].Name != "build_prompt" {
		t.Fatalf("prompt list = %#v, want build_prompt", promptList.Prompts)
	}

	toolResult, err := result.Caller(context.Background(), "stdio-server", "echo_env", map[string]any{"path": "README.md"})
	if err != nil {
		t.Fatalf("tool call: %v", err)
	}
	if got := firstTextContent(t, toolResult.Content); !strings.Contains(got, "env-from-parent") || !strings.Contains(got, "README.md") {
		t.Fatalf("tool result content = %q, want env and input echoed", got)
	}

	promptResult, err := result.PromptCaller(context.Background(), "stdio-server", "build_prompt", map[string]any{"topic": "transport"})
	if err != nil {
		t.Fatalf("prompt call: %v", err)
	}
	if promptResult.Description != "stdio prompt" {
		t.Fatalf("prompt description = %q, want stdio prompt", promptResult.Description)
	}
	if got := firstTextMessage(t, promptResult.Messages); !strings.Contains(got, "transport") {
		t.Fatalf("prompt message = %q, want topic echoed", got)
	}
}

func TestDiscoverMCPClientToolsCloseShutsDownStdioTransport(t *testing.T) {
	shutdownFile := filepath.Join(t.TempDir(), "shutdown.txt")
	connection := tools.MCPConnection{
		Name:    "stdio-server",
		Type:    "stdio",
		Command: os.Args[0],
		Args:    []string{"-test.run=TestMCPStdioHelperProcess"},
		Env: map[string]string{
			"GO_WANT_HELPER_PROCESS": "1",
			"MCP_SHUTDOWN_FILE":      shutdownFile,
		},
	}

	result, err := tools.DiscoverMCPClientTools(context.Background(), []tools.MCPConnection{connection})
	if err != nil {
		t.Fatalf("discover stdio MCP: %v", err)
	}
	if result.Close == nil {
		t.Fatal("Close is nil, want lifecycle shutdown hook")
	}
	if err := result.Close(); err != nil {
		t.Fatalf("close MCP discovery result: %v", err)
	}
	for i := 0; i < 50; i++ {
		if data, err := os.ReadFile(shutdownFile); err == nil && strings.TrimSpace(string(data)) == "closed" {
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatalf("stdio helper did not write shutdown marker at %s", shutdownFile)
}

func TestDiscoverMCPClientToolsPassesEnvToStdioServer(t *testing.T) {
	connection := tools.MCPConnection{
		Name:    "stdio-server",
		Type:    "stdio",
		Command: os.Args[0],
		Args:    []string{"-test.run=TestMCPStdioHelperProcess"},
		Env: map[string]string{
			"GO_WANT_HELPER_PROCESS": "1",
			"MCP_ENV_TOKEN":          "env-from-config",
		},
	}

	result, err := tools.DiscoverMCPClientTools(context.Background(), []tools.MCPConnection{connection})
	if err != nil {
		t.Fatalf("discover stdio MCP: %v", err)
	}

	toolResult, err := result.Caller(context.Background(), "stdio-server", "echo_env", map[string]any{"path": "notes.txt"})
	if err != nil {
		t.Fatalf("tool call: %v", err)
	}
	if got := firstTextContent(t, toolResult.Content); !strings.Contains(got, "env-from-config") {
		t.Fatalf("tool result content = %q, want env from MCPConnection", got)
	}
}

func TestDiscoverMCPClientToolsSupportsStdioPromptsGet(t *testing.T) {
	connection := tools.MCPConnection{
		Name:    "stdio-server",
		Type:    "stdio",
		Command: os.Args[0],
		Args:    []string{"-test.run=TestMCPStdioHelperProcess"},
		Env: map[string]string{
			"GO_WANT_HELPER_PROCESS": "1",
		},
	}

	result, err := tools.DiscoverMCPClientTools(context.Background(), []tools.MCPConnection{connection})
	if err != nil {
		t.Fatalf("discover stdio MCP: %v", err)
	}

	promptResult, err := result.PromptCaller(context.Background(), "stdio-server", "build_prompt", map[string]any{"topic": "stdio"})
	if err != nil {
		t.Fatalf("prompt call: %v", err)
	}
	if got := firstTextMessage(t, promptResult.Messages); !strings.Contains(got, "stdio") {
		t.Fatalf("prompt message = %q, want topic echoed", got)
	}
}

func TestDiscoverMCPClientToolsDiscoversHTTPResourcesAndListToolUsesThem(t *testing.T) {
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
					"capabilities": map[string]any{
						"tools":     map[string]any{},
						"resources": map[string]any{},
					},
				},
			})
		case "notifications/initialized":
			w.WriteHeader(http.StatusNoContent)
		case "tools/list":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"jsonrpc": "2.0",
				"id":      request["id"],
				"result":  map[string]any{"tools": []map[string]any{}},
			})
		case "resources/list":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"jsonrpc": "2.0",
				"id":      request["id"],
				"result": map[string]any{
					"resources": []map[string]any{{
						"uri":         "file:///workspace/README.md",
						"name":        "README",
						"description": "project readme",
					}},
				},
			})
		default:
			t.Fatalf("unexpected method %q", method)
		}
	}))
	defer server.Close()

	result, err := tools.DiscoverMCPClientTools(context.Background(), []tools.MCPConnection{
		{Name: "filesystem", Type: "streamable_http", BaseURL: server.URL},
	})
	if err != nil {
		t.Fatalf("discover http MCP: %v", err)
	}
	if len(result.Resources["filesystem"]) != 1 {
		t.Fatalf("resources = %#v, want one live resource", result.Resources)
	}
	if result.Resources["filesystem"][0].URI != "file:///workspace/README.md" {
		t.Fatalf("resources = %#v, want discovered URI", result.Resources)
	}

	listed, err := tools.NewListMcpResourcesTool().InvokeWithContext(context.Background(), tools.ToolUseContext{
		MCPResources: result.Resources,
	})
	if err != nil {
		t.Fatalf("list resources: %v", err)
	}
	if !strings.Contains(listed.Output, "file:///workspace/README.md") || !strings.Contains(listed.Output, "project readme") {
		t.Fatalf("output = %q, want live MCP resource listing", listed.Output)
	}
}

func TestReadMcpResourceUsesLiveReaderAndReturnsText(t *testing.T) {
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
					"capabilities": map[string]any{
						"tools":     map[string]any{},
						"resources": map[string]any{},
					},
				},
			})
		case "notifications/initialized":
			w.WriteHeader(http.StatusNoContent)
		case "tools/list":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"jsonrpc": "2.0",
				"id":      request["id"],
				"result":  map[string]any{"tools": []map[string]any{}},
			})
		case "resources/list":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"jsonrpc": "2.0",
				"id":      request["id"],
				"result": map[string]any{
					"resources": []map[string]any{{
						"uri":         "file:///workspace/README.md",
						"name":        "README",
						"description": "project readme",
					}},
				},
			})
		case "resources/read":
			params, _ := request["params"].(map[string]any)
			if params["uri"] != "file:///workspace/README.md" {
				t.Fatalf("resources/read params = %#v, want uri", params)
			}
			_ = json.NewEncoder(w).Encode(map[string]any{
				"jsonrpc": "2.0",
				"id":      request["id"],
				"result": map[string]any{
					"contents": []map[string]any{{
						"uri":      "file:///workspace/README.md",
						"mimeType": "text/plain",
						"text":     "hello from live resource",
					}},
				},
			})
		default:
			t.Fatalf("unexpected method %q", method)
		}
	}))
	defer server.Close()

	result, err := tools.DiscoverMCPClientTools(context.Background(), []tools.MCPConnection{
		{Name: "filesystem", Type: "streamable_http", BaseURL: server.URL},
	})
	if err != nil {
		t.Fatalf("discover http MCP: %v", err)
	}

	read, err := tools.NewReadMcpResourceTool().InvokeWithContext(context.Background(), tools.ToolUseContext{
		MCPResources:      result.Resources,
		MCPResourceReader: result.ResourceReader,
		Input:             `{"server":"filesystem","uri":"file:///workspace/README.md"}`,
	})
	if err != nil {
		t.Fatalf("read resource: %v", err)
	}

	var parsed map[string]any
	if err := json.Unmarshal([]byte(read.Output), &parsed); err != nil {
		t.Fatalf("read output JSON: %v", err)
	}
	contents, _ := parsed["contents"].([]any)
	if len(contents) != 1 {
		t.Fatalf("read output = %#v, want one contents entry", parsed)
	}
	content := contents[0].(map[string]any)
	if content["uri"] != "file:///workspace/README.md" || content["mimeType"] != "text/plain" || content["text"] != "hello from live resource" {
		t.Fatalf("content = %#v, want live resource text response", content)
	}
}

func TestReadMcpResourceSavesBlobResourcesToTempFile(t *testing.T) {
	blobBytes := []byte("binary resource payload")
	blobBase64 := base64.StdEncoding.EncodeToString(blobBytes)
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
					"capabilities": map[string]any{
						"tools":     map[string]any{},
						"resources": map[string]any{},
					},
				},
			})
		case "notifications/initialized":
			w.WriteHeader(http.StatusNoContent)
		case "tools/list":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"jsonrpc": "2.0",
				"id":      request["id"],
				"result":  map[string]any{"tools": []map[string]any{}},
			})
		case "resources/list":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"jsonrpc": "2.0",
				"id":      request["id"],
				"result": map[string]any{
					"resources": []map[string]any{{
						"uri":         "file:///workspace/blob.bin",
						"name":        "blob",
						"description": "binary resource",
					}},
				},
			})
		case "resources/read":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"jsonrpc": "2.0",
				"id":      request["id"],
				"result": map[string]any{
					"contents": []map[string]any{{
						"uri":      "file:///workspace/blob.bin",
						"mimeType": "application/octet-stream",
						"blob":     blobBase64,
					}},
				},
			})
		default:
			t.Fatalf("unexpected method %q", method)
		}
	}))
	defer server.Close()

	result, err := tools.DiscoverMCPClientTools(context.Background(), []tools.MCPConnection{
		{Name: "filesystem", Type: "streamable_http", BaseURL: server.URL},
	})
	if err != nil {
		t.Fatalf("discover http MCP: %v", err)
	}

	read, err := tools.NewReadMcpResourceTool().InvokeWithContext(context.Background(), tools.ToolUseContext{
		MCPResources:      result.Resources,
		MCPResourceReader: result.ResourceReader,
		Input:             `{"server":"filesystem","uri":"file:///workspace/blob.bin"}`,
	})
	if err != nil {
		t.Fatalf("read blob resource: %v", err)
	}
	if strings.Contains(read.Output, blobBase64) {
		t.Fatalf("output = %q, want base64 payload to be saved to disk rather than returned inline", read.Output)
	}
	if !strings.Contains(read.Output, "blobSavedTo") {
		t.Fatalf("output = %q, want blobSavedTo path in Claude-like JSON", read.Output)
	}
	if !strings.Contains(read.Output, "myclaw-mcp-output") {
		t.Fatalf("output = %q, want saved blob path under temp output directory", read.Output)
	}
	var parsed map[string]any
	if err := json.Unmarshal([]byte(read.Output), &parsed); err != nil {
		t.Fatalf("blob output JSON: %v", err)
	}
	contents, _ := parsed["contents"].([]any)
	if len(contents) != 1 {
		t.Fatalf("blob output = %#v, want one contents entry", parsed)
	}
	content := contents[0].(map[string]any)
	savedTo, _ := content["blobSavedTo"].(string)
	if savedTo == "" {
		t.Fatalf("content = %#v, want blobSavedTo path", content)
	}
	data, err := os.ReadFile(savedTo)
	if err != nil {
		t.Fatalf("read saved blob: %v", err)
	}
	if string(data) != string(blobBytes) {
		t.Fatalf("saved blob = %q, want decoded bytes", string(data))
	}
	if text, _ := content["text"].(string); !strings.Contains(text, savedTo) {
		t.Fatalf("content text = %q, want save path explanation", text)
	}
}

func TestMCPStdioHelperProcess(t *testing.T) {
	if os.Getenv("GO_WANT_HELPER_PROCESS") != "1" {
		return
	}
	if shutdownFile := os.Getenv("MCP_SHUTDOWN_FILE"); shutdownFile != "" {
		defer os.WriteFile(shutdownFile, []byte("closed"), 0o644)
	}

	reader := bufio.NewReader(os.Stdin)
	writer := bufio.NewWriter(os.Stdout)
	envToken := os.Getenv("MCP_ENV_TOKEN")

	for {
		line, err := reader.ReadBytes('\n')
		if err != nil {
			if err != io.EOF {
				fmt.Fprintln(os.Stderr, err)
			}
			return
		}
		line = []byte(strings.TrimSpace(string(line)))
		if len(line) == 0 {
			continue
		}
		var request map[string]any
		if err := json.Unmarshal(line, &request); err != nil {
			fmt.Fprintln(os.Stderr, err)
			return
		}
		method, _ := request["method"].(string)
		if method == "notifications/initialized" {
			continue
		}
		if _, ok := request["id"]; !ok {
			continue
		}

		var response map[string]any
		switch method {
		case "initialize":
			response = map[string]any{
				"jsonrpc": "2.0",
				"id":      request["id"],
				"result": map[string]any{
					"protocolVersion": "2024-11-05",
					"capabilities": map[string]any{
						"tools":   map[string]any{},
						"prompts": map[string]any{},
					},
					"serverInfo": map[string]any{
						"name":    "stdio-helper",
						"version": "1.0.0",
					},
				},
			}
		case "tools/list":
			response = map[string]any{
				"jsonrpc": "2.0",
				"id":      request["id"],
				"result": map[string]any{
					"tools": []map[string]any{{
						"name":        "echo_env",
						"description": "Echo environment and input",
						"inputSchema": map[string]any{
							"type": "object",
							"properties": map[string]any{
								"path": map[string]any{"type": "string"},
							},
						},
					}},
				},
			}
		case "prompts/list":
			response = map[string]any{
				"jsonrpc": "2.0",
				"id":      request["id"],
				"result": map[string]any{
					"prompts": []map[string]any{{
						"name":        "build_prompt",
						"description": "stdio prompt",
						"arguments": []map[string]any{{
							"name":        "topic",
							"description": "Prompt topic",
							"required":    true,
						}},
					}},
				},
			}
		case "tools/call":
			params, _ := request["params"].(map[string]any)
			args, _ := params["arguments"].(map[string]any)
			path, _ := args["path"].(string)
			response = map[string]any{
				"jsonrpc": "2.0",
				"id":      request["id"],
				"result": map[string]any{
					"content": []map[string]any{{
						"type": "text",
						"text": "tool env=" + envToken + " path=" + path,
					}},
					"structuredContent": map[string]any{
						"path": path,
					},
				},
			}
		case "prompts/get":
			params, _ := request["params"].(map[string]any)
			args, _ := params["arguments"].(map[string]any)
			topic, _ := args["topic"].(string)
			response = map[string]any{
				"jsonrpc": "2.0",
				"id":      request["id"],
				"result": map[string]any{
					"description": "stdio prompt",
					"messages": []map[string]any{{
						"role": "user",
						"content": []map[string]any{{
							"type": "text",
							"text": "prompt topic " + topic,
						}},
					}},
				},
			}
		default:
			response = map[string]any{
				"jsonrpc": "2.0",
				"id":      request["id"],
				"error": map[string]any{
					"code":    -32601,
					"message": "unknown method",
				},
			}
		}

		encoded, _ := json.Marshal(response)
		if _, err := writer.Write(append(encoded, '\n')); err != nil {
			fmt.Fprintln(os.Stderr, err)
			return
		}
		if err := writer.Flush(); err != nil {
			fmt.Fprintln(os.Stderr, err)
			return
		}
	}
}

func firstTextContent(t *testing.T, content []map[string]any) string {
	t.Helper()
	for _, item := range content {
		if text, ok := item["text"].(string); ok {
			return text
		}
	}
	t.Fatalf("content = %#v, want text entry", content)
	return ""
}

func firstTextMessage(t *testing.T, messages []tools.MCPPromptMessage) string {
	t.Helper()
	for _, message := range messages {
		items, ok := message.Content.([]any)
		if !ok {
			continue
		}
		for _, item := range items {
			entry, ok := item.(map[string]any)
			if !ok {
				continue
			}
			if value, ok := entry["text"].(string); ok {
				return value
			}
		}
	}
	t.Fatalf("messages = %#v, want text content", messages)
	return ""
}

func requestIDFromBody(t *testing.T, r *http.Request) any {
	t.Helper()
	var request map[string]any
	if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
		t.Fatalf("decode request: %v", err)
	}
	return request["id"]
}
