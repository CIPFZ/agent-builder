package tools

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"time"
)

type MCPDiscoveryResult struct {
	Tools        map[string]MCPToolsListResult
	Prompts      map[string]MCPPromptsListResult
	Caller       MCPToolCaller
	PromptCaller MCPPromptCaller
}

type mcpHTTPRuntime struct {
	mu      sync.Mutex
	nextID  int
	client  *http.Client
	servers map[string]MCPConnection
}

func DiscoverMCPClientTools(ctx context.Context, connections []MCPConnection) (MCPDiscoveryResult, error) {
	runtime := &mcpHTTPRuntime{
		client:  &http.Client{Timeout: 30 * time.Second},
		servers: make(map[string]MCPConnection),
	}
	discoveredTools := make(map[string]MCPToolsListResult)
	discoveredPrompts := make(map[string]MCPPromptsListResult)
	for _, connection := range connections {
		if !isHTTPMCPConnection(connection) {
			continue
		}
		name := strings.TrimSpace(connection.Name)
		if name == "" {
			return MCPDiscoveryResult{}, fmt.Errorf("MCP connection is missing name")
		}
		result, err := runtime.discover(ctx, connection)
		if err != nil {
			return MCPDiscoveryResult{}, err
		}
		runtime.servers[name] = connection
		discoveredTools[name] = result.Tools
		discoveredPrompts[name] = result.Prompts
	}
	return MCPDiscoveryResult{
		Tools:        discoveredTools,
		Prompts:      discoveredPrompts,
		Caller:       runtime.callTool,
		PromptCaller: runtime.getPrompt,
	}, nil
}

func isHTTPMCPConnection(connection MCPConnection) bool {
	baseURL := connectionURL(connection)
	if strings.HasPrefix(baseURL, "http://") || strings.HasPrefix(baseURL, "https://") {
		return true
	}
	switch strings.ToLower(strings.TrimSpace(connection.Type)) {
	case "http", "streamable_http", "sse":
		return baseURL != ""
	default:
		return false
	}
}

type mcpDiscoveryLists struct {
	Tools   MCPToolsListResult
	Prompts MCPPromptsListResult
}

func (r *mcpHTTPRuntime) discover(ctx context.Context, connection MCPConnection) (mcpDiscoveryLists, error) {
	initResult, err := r.rpc(ctx, connection, "initialize", map[string]any{
		"protocolVersion": "2024-11-05",
		"capabilities":    map[string]any{},
		"clientInfo": map[string]any{
			"name":    "myclaw",
			"version": "dev",
		},
	}, true)
	if err != nil {
		return mcpDiscoveryLists{}, fmt.Errorf("initialize MCP server %q: %w", connection.Name, err)
	}
	_, _ = r.rpc(ctx, connection, "notifications/initialized", map[string]any{}, false)
	lists := mcpDiscoveryLists{}
	capabilities, _ := initResult["capabilities"].(map[string]any)
	result, err := r.rpc(ctx, connection, "tools/list", map[string]any{}, true)
	if err != nil {
		if capabilities == nil || capabilities["tools"] != nil {
			return mcpDiscoveryLists{}, fmt.Errorf("list MCP tools for %q: %w", connection.Name, err)
		}
	} else {
		encoded, err := json.Marshal(result)
		if err != nil {
			return mcpDiscoveryLists{}, err
		}
		if err := json.Unmarshal(encoded, &lists.Tools); err != nil {
			return mcpDiscoveryLists{}, err
		}
	}
	if capabilities != nil && capabilities["prompts"] != nil {
		result, err := r.rpc(ctx, connection, "prompts/list", map[string]any{}, true)
		if err != nil {
			return mcpDiscoveryLists{}, fmt.Errorf("list MCP prompts for %q: %w", connection.Name, err)
		}
		encoded, err := json.Marshal(result)
		if err != nil {
			return mcpDiscoveryLists{}, err
		}
		if err := json.Unmarshal(encoded, &lists.Prompts); err != nil {
			return mcpDiscoveryLists{}, err
		}
	}
	return lists, nil
}

func (r *mcpHTTPRuntime) callTool(ctx context.Context, server, name string, input map[string]any) (MCPToolResult, error) {
	connection, ok := r.servers[server]
	if !ok {
		return MCPToolResult{}, fmt.Errorf("MCP server %q is not connected", server)
	}
	result, err := r.rpc(ctx, connection, "tools/call", map[string]any{
		"name":      name,
		"arguments": input,
	}, true)
	if err != nil {
		return MCPToolResult{}, err
	}
	encoded, err := json.Marshal(result)
	if err != nil {
		return MCPToolResult{}, err
	}
	var toolResult MCPToolResult
	if err := json.Unmarshal(encoded, &toolResult); err != nil {
		return MCPToolResult{}, err
	}
	return toolResult, nil
}

func (r *mcpHTTPRuntime) getPrompt(ctx context.Context, server, name string, arguments map[string]any) (MCPPromptResult, error) {
	connection, ok := r.servers[server]
	if !ok {
		return MCPPromptResult{}, fmt.Errorf("MCP server %q is not connected", server)
	}
	result, err := r.rpc(ctx, connection, "prompts/get", map[string]any{
		"name":      name,
		"arguments": arguments,
	}, true)
	if err != nil {
		return MCPPromptResult{}, err
	}
	encoded, err := json.Marshal(result)
	if err != nil {
		return MCPPromptResult{}, err
	}
	var promptResult MCPPromptResult
	if err := json.Unmarshal(encoded, &promptResult); err != nil {
		return MCPPromptResult{}, err
	}
	return promptResult, nil
}

func (r *mcpHTTPRuntime) rpc(ctx context.Context, connection MCPConnection, method string, params map[string]any, expectResponse bool) (map[string]any, error) {
	request := map[string]any{
		"jsonrpc": "2.0",
		"method":  method,
	}
	if params != nil {
		request["params"] = params
	}
	if expectResponse {
		request["id"] = r.nextRequestID()
	}
	encoded, err := json.Marshal(request)
	if err != nil {
		return nil, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, connectionURL(connection), bytes.NewReader(encoded))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", "myclaw-mcp/1.0")
	for key, value := range connection.Headers {
		if strings.TrimSpace(key) != "" {
			req.Header.Set(key, value)
		}
	}
	resp, err := r.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("MCP server %q returned HTTP %d", connection.Name, resp.StatusCode)
	}
	if !expectResponse || resp.StatusCode == http.StatusAccepted || resp.ContentLength == 0 {
		return nil, nil
	}
	var envelope struct {
		Result map[string]any `json:"result"`
		Error  *struct {
			Code    int    `json:"code"`
			Message string `json:"message"`
		} `json:"error"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&envelope); err != nil {
		return nil, err
	}
	if envelope.Error != nil {
		return nil, fmt.Errorf("MCP server %q %s failed (%d): %s", connection.Name, method, envelope.Error.Code, envelope.Error.Message)
	}
	if envelope.Result == nil {
		return map[string]any{}, nil
	}
	return envelope.Result, nil
}

func connectionURL(connection MCPConnection) string {
	if url := strings.TrimSpace(connection.BaseURL); url != "" {
		return url
	}
	return strings.TrimSpace(connection.URL)
}

func (r *mcpHTTPRuntime) nextRequestID() int {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.nextID++
	return r.nextID
}
