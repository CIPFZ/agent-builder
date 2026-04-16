package tools

import (
	"bufio"
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
)

const defaultMCPToolTimeout = 100000000 * time.Millisecond

type MCPDiscoveryResult struct {
	Tools            map[string]MCPToolsListResult
	Prompts          map[string]MCPPromptsListResult
	Resources        map[string][]MCPResource
	NeedsAuth        map[string]MCPAuthToolResult
	Caller           MCPToolCaller
	ContextualCaller MCPContextualToolCaller
	PromptCaller     MCPPromptCaller
	ResourceReader   MCPResourceReader
	ResourceLister   MCPResourceLister
	Reconnect        MCPReconnectFunc
	Close            func() error
}

type mcpRuntime struct {
	mu          sync.Mutex
	nextID      int
	client      *http.Client
	sessions    map[string]mcpTransportSession
	connections map[string]MCPConnection
}

type mcpTransportSession struct {
	connection MCPConnection
	transport  mcpTransport
}

type mcpTransport interface {
	rpc(context.Context, string, map[string]any, bool, ProgressFunc) (map[string]any, error)
	close() error
}

type mcpDiscoveryLists struct {
	Tools     MCPToolsListResult
	Prompts   MCPPromptsListResult
	Resources []MCPResource
}

func DiscoverMCPClientTools(ctx context.Context, connections []MCPConnection) (MCPDiscoveryResult, error) {
	runtime := &mcpRuntime{
		client:   &http.Client{Timeout: 30 * time.Second},
		sessions: make(map[string]mcpTransportSession),
	}
	runtime.connections = make(map[string]MCPConnection)
	discoveredTools := make(map[string]MCPToolsListResult)
	discoveredPrompts := make(map[string]MCPPromptsListResult)
	discoveredResources := make(map[string][]MCPResource)
	discoveredNeedsAuth := make(map[string]MCPAuthToolResult)
	for _, connection := range connections {
		name := strings.TrimSpace(connection.Name)
		if name == "" {
			return MCPDiscoveryResult{}, fmt.Errorf("MCP connection is missing name")
		}
		runtime.setConnection(name, connection)
		if !isHTTPMCPConnection(connection) && !isStdioMCPConnection(connection) {
			continue
		}
		transport, err := runtime.open(ctx, connection)
		if err != nil {
			return MCPDiscoveryResult{}, err
		}
		result, err := runtime.discover(ctx, connection, transport)
		if err != nil {
			var authErr *mcpAuthRequiredError
			if errors.As(err, &authErr) {
				discoveredNeedsAuth[name] = buildMCPAuthToolResult(name, connection, authErr)
				_ = transport.close()
				continue
			}
			_ = transport.close()
			return MCPDiscoveryResult{}, err
		}
		runtime.setSession(name, mcpTransportSession{
			connection: connection,
			transport:  transport,
		})
		discoveredTools[name] = result.Tools
		discoveredPrompts[name] = result.Prompts
		discoveredResources[name] = result.Resources
	}
	return MCPDiscoveryResult{
		Tools:            discoveredTools,
		Prompts:          discoveredPrompts,
		Resources:        discoveredResources,
		NeedsAuth:        discoveredNeedsAuth,
		Caller:           runtime.callTool,
		ContextualCaller: runtime.callToolWithRequest,
		PromptCaller:     runtime.getPrompt,
		ResourceReader:   runtime.readResource,
		ResourceLister:   runtime.listResources,
		Reconnect:        runtime.reconnect,
		Close:            runtime.close,
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

func isStdioMCPConnection(connection MCPConnection) bool {
	switch strings.ToLower(strings.TrimSpace(connection.Type)) {
	case "stdio":
		return true
	default:
		return strings.TrimSpace(connection.Command) != ""
	}
}

func (r *mcpRuntime) open(ctx context.Context, connection MCPConnection) (mcpTransport, error) {
	if isStdioMCPConnection(connection) {
		return r.openStdio(ctx, connection)
	}
	return &mcpHTTPTransport{runtime: r, connection: connection}, nil
}

func (r *mcpRuntime) openStdio(ctx context.Context, connection MCPConnection) (mcpTransport, error) {
	command := strings.TrimSpace(connection.Command)
	if command == "" {
		return nil, fmt.Errorf("MCP connection %q is missing command", connection.Name)
	}
	cmd := exec.CommandContext(ctx, command, connection.Args...)
	cmd.Env = mergedMCPEnvironment(connection.Env)
	stdin, err := cmd.StdinPipe()
	if err != nil {
		return nil, err
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		_ = stdin.Close()
		return nil, err
	}
	stderr := &bytes.Buffer{}
	cmd.Stderr = stderr
	if err := cmd.Start(); err != nil {
		_ = stdin.Close()
		return nil, err
	}
	return &mcpStdioTransport{
		runtime:    r,
		connection: connection,
		cmd:        cmd,
		stdin:      stdin,
		stdout:     bufio.NewReader(stdout),
		stderr:     stderr,
	}, nil
}

func (r *mcpRuntime) discover(ctx context.Context, connection MCPConnection, transport mcpTransport) (mcpDiscoveryLists, error) {
	initResult, err := transport.rpc(ctx, "initialize", map[string]any{
		"protocolVersion": "2024-11-05",
		"capabilities":    map[string]any{},
		"clientInfo": map[string]any{
			"name":    "myclaw",
			"version": "dev",
		},
	}, true, nil)
	if err != nil {
		return mcpDiscoveryLists{}, fmt.Errorf("initialize MCP server %q: %w", connection.Name, err)
	}
	_, _ = transport.rpc(ctx, "notifications/initialized", map[string]any{}, false, nil)
	lists := mcpDiscoveryLists{}
	capabilities, _ := initResult["capabilities"].(map[string]any)
	result, err := transport.rpc(ctx, "tools/list", map[string]any{}, true, nil)
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
		result, err := transport.rpc(ctx, "prompts/list", map[string]any{}, true, nil)
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
	if capabilities != nil && capabilities["resources"] != nil {
		result, err := transport.rpc(ctx, "resources/list", map[string]any{}, true, nil)
		if err != nil {
			return mcpDiscoveryLists{}, fmt.Errorf("list MCP resources for %q: %w", connection.Name, err)
		}
		encoded, err := json.Marshal(result)
		if err != nil {
			return mcpDiscoveryLists{}, err
		}
		var resources MCPResourcesListResult
		if err := json.Unmarshal(encoded, &resources); err != nil {
			return mcpDiscoveryLists{}, err
		}
		lists.Resources = make([]MCPResource, 0, len(resources.Resources))
		for _, item := range resources.Resources {
			lists.Resources = append(lists.Resources, MCPResource{
				URI:         item.URI,
				Name:        firstNonEmpty(strings.TrimSpace(item.Name), strings.TrimSpace(item.Title), strings.TrimSpace(item.URI)),
				Description: item.Description,
			})
		}
	}
	return lists, nil
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if trimmed := strings.TrimSpace(value); trimmed != "" {
			return trimmed
		}
	}
	return ""
}

func cloneStringMap(input map[string]string) map[string]string {
	if input == nil {
		return nil
	}
	cloned := make(map[string]string, len(input))
	for key, value := range input {
		cloned[key] = value
	}
	return cloned
}

func (r *mcpRuntime) session(server string) (mcpTransportSession, bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	session, ok := r.sessions[server]
	return session, ok
}

func (r *mcpRuntime) setConnection(server string, connection MCPConnection) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.connections == nil {
		r.connections = make(map[string]MCPConnection)
	}
	r.connections[server] = connection
}

func (r *mcpRuntime) connection(server string) (MCPConnection, bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	connection, ok := r.connections[server]
	return connection, ok
}

func (r *mcpRuntime) setSession(server string, session mcpTransportSession) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.sessions[server] = session
	if r.connections == nil {
		r.connections = make(map[string]MCPConnection)
	}
	r.connections[server] = session.connection
}

func (r *mcpRuntime) close() error {
	r.mu.Lock()
	sessions := make([]mcpTransportSession, 0, len(r.sessions))
	for server, session := range r.sessions {
		sessions = append(sessions, session)
		delete(r.sessions, server)
	}
	r.mu.Unlock()

	var errs []error
	for _, session := range sessions {
		if err := session.transport.close(); err != nil {
			errs = append(errs, err)
		}
	}
	return errors.Join(errs...)
}

func (r *mcpRuntime) callTool(ctx context.Context, server, name string, input map[string]any) (MCPToolResult, error) {
	return r.callToolWithRequest(ctx, MCPToolCallRequest{Server: server, Name: name, Input: input})
}

func (r *mcpRuntime) callToolWithRequest(ctx context.Context, req MCPToolCallRequest) (MCPToolResult, error) {
	timeout := req.Timeout
	if timeout <= 0 {
		timeout = mcpToolTimeout()
	}
	if timeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, timeout)
		defer cancel()
	}
	meta := mergeMCPToolCallMeta(req.Meta, req.ToolUseID)
	var lastErr error
	for attempt := 0; attempt < 4; attempt++ {
		toolResult, err := r.callToolWithSessionRetry(ctx, req.Server, req.Name, req.Input, meta, req.ToolUseID, req.ReportProgress)
		if err == nil {
			return toolResult, nil
		}
		lastErr = err
		elicitations := mcpURLElicitations(err)
		if len(elicitations) == 0 || req.HandleElicitation == nil || attempt == 3 {
			break
		}
		for _, elicitation := range elicitations {
			result, elicitErr := req.HandleElicitation(ctx, ElicitationRequest{
				ServerName: req.Server,
				Params:     elicitation,
			})
			if elicitErr != nil {
				return MCPToolResult{}, elicitErr
			}
			action := mcpElicitationAction(result)
			if action != "accept" {
				return mcpURLDeclinedToolResult(req.Name, action), nil
			}
		}
	}
	return MCPToolResult{}, lastErr
}

func mcpToolTimeout() time.Duration {
	raw := strings.TrimSpace(os.Getenv("MCP_TOOL_TIMEOUT"))
	if raw == "" {
		return defaultMCPToolTimeout
	}
	millis, err := strconv.Atoi(raw)
	if err != nil || millis <= 0 {
		return defaultMCPToolTimeout
	}
	return time.Duration(millis) * time.Millisecond
}

func mergeMCPToolCallMeta(meta map[string]any, toolUseID string) map[string]any {
	merged := cloneAnyMap(meta)
	if merged == nil {
		merged = make(map[string]any)
	}
	if toolMeta := mcpToolUseMeta(toolUseID); len(toolMeta) > 0 {
		for key, value := range toolMeta {
			merged[key] = value
		}
		if _, ok := merged["progressToken"]; !ok {
			merged["progressToken"] = toolUseID
		}
	}
	if len(merged) == 0 {
		return nil
	}
	return merged
}

func mcpProgressReporter(server, tool, toolUseID string, reportProgress ProgressFunc) ProgressFunc {
	if reportProgress == nil {
		return nil
	}
	return func(progress ToolProgress) {
		data := cloneAnyMap(progress.Data)
		if data == nil {
			data = make(map[string]any)
		}
		data["type"] = "mcp_progress"
		data["status"] = "progress"
		data["serverName"] = server
		data["toolName"] = tool
		message := strings.TrimSpace(progress.Message)
		if message == "" {
			message = stringField(data, "message")
		}
		if message != "" {
			data["progressMessage"] = message
		}
		reportProgress(ToolProgress{
			ToolUseID: toolUseID,
			Type:      "progress",
			Message:   message,
			Data:      data,
		})
	}
}

func toolProgressFromMCPNotification(params map[string]any) ToolProgress {
	data := cloneAnyMap(params)
	message := strings.TrimSpace(stringField(params, "message"))
	if message == "" {
		message = strings.TrimSpace(stringField(params, "progressMessage"))
	}
	return ToolProgress{
		Type:    "progress",
		Message: message,
		Data:    data,
	}
}

func (r *mcpRuntime) callToolWithSessionRetry(ctx context.Context, server, name string, input map[string]any, meta map[string]any, toolUseID string, reportProgress ProgressFunc) (MCPToolResult, error) {
	toolResult, err := r.callToolOnce(ctx, server, name, input, meta, toolUseID, reportProgress)
	if err == nil {
		return normalizeMCPToolResult(toolResult, server), nil
	}
	if !isMCPSessionExpiredError(err) {
		return MCPToolResult{}, err
	}
	if _, reconnectErr := r.reconnect(ctx, server); reconnectErr != nil {
		return MCPToolResult{}, reconnectErr
	}
	toolResult, err = r.callToolOnce(ctx, server, name, input, meta, toolUseID, reportProgress)
	if err != nil {
		return MCPToolResult{}, err
	}
	return normalizeMCPToolResult(toolResult, server), nil
}

func (r *mcpRuntime) callToolOnce(ctx context.Context, server, name string, input map[string]any, meta map[string]any, toolUseID string, reportProgress ProgressFunc) (MCPToolResult, error) {
	session, ok := r.session(server)
	if !ok {
		return MCPToolResult{}, fmt.Errorf("MCP server %q is not connected", server)
	}
	params := map[string]any{
		"name":      name,
		"arguments": input,
	}
	if len(meta) > 0 {
		params["_meta"] = meta
	}
	result, err := session.transport.rpc(ctx, "tools/call", params, true, mcpProgressReporter(server, name, toolUseID, reportProgress))
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

func (r *mcpRuntime) getPrompt(ctx context.Context, server, name string, arguments map[string]any) (MCPPromptResult, error) {
	result, err := r.getPromptOnce(ctx, server, name, arguments)
	if err == nil {
		return result, nil
	}
	if !isMCPSessionExpiredError(err) {
		return MCPPromptResult{}, err
	}
	if _, reconnectErr := r.reconnect(ctx, server); reconnectErr != nil {
		return MCPPromptResult{}, reconnectErr
	}
	return r.getPromptOnce(ctx, server, name, arguments)
}

func (r *mcpRuntime) getPromptOnce(ctx context.Context, server, name string, arguments map[string]any) (MCPPromptResult, error) {
	session, ok := r.session(server)
	if !ok {
		return MCPPromptResult{}, fmt.Errorf("MCP server %q is not connected", server)
	}
	result, err := session.transport.rpc(ctx, "prompts/get", map[string]any{
		"name":      name,
		"arguments": arguments,
	}, true, nil)
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

func (r *mcpRuntime) reconnect(ctx context.Context, server string) (MCPReconnectResult, error) {
	session, ok := r.session(server)
	var connection MCPConnection
	if !ok {
		var found bool
		connection, found = r.connection(server)
		if !found {
			return MCPReconnectResult{}, fmt.Errorf("MCP server %q is not connected", server)
		}
	} else {
		connection = session.connection
		_ = session.transport.close()
	}
	transport, err := r.open(ctx, connection)
	if err != nil {
		return MCPReconnectResult{}, err
	}
	result, err := r.discover(ctx, connection, transport)
	if err != nil {
		_ = transport.close()
		return MCPReconnectResult{}, err
	}
	r.setSession(server, mcpTransportSession{
		connection: connection,
		transport:  transport,
	})
	return MCPReconnectResult{
		Client:    connection,
		Tools:     result.Tools,
		Prompts:   result.Prompts,
		Resources: result.Resources,
	}, nil
}

func (r *mcpRuntime) readResource(ctx context.Context, server, uri string) (MCPResourceReadResult, error) {
	result, err := r.readResourceOnce(ctx, server, uri)
	if err == nil {
		return result, nil
	}
	if !isMCPSessionExpiredError(err) {
		return MCPResourceReadResult{}, err
	}
	if _, reconnectErr := r.reconnect(ctx, server); reconnectErr != nil {
		return MCPResourceReadResult{}, reconnectErr
	}
	return r.readResourceOnce(ctx, server, uri)
}

func (r *mcpRuntime) readResourceOnce(ctx context.Context, server, uri string) (MCPResourceReadResult, error) {
	session, ok := r.session(server)
	if !ok {
		return MCPResourceReadResult{}, fmt.Errorf("MCP server %q is not connected", server)
	}
	result, err := session.transport.rpc(ctx, "resources/read", map[string]any{
		"uri": uri,
	}, true, nil)
	if err != nil {
		return MCPResourceReadResult{}, err
	}
	encoded, err := json.Marshal(result)
	if err != nil {
		return MCPResourceReadResult{}, err
	}
	var raw struct {
		Contents []map[string]any `json:"contents"`
	}
	if err := json.Unmarshal(encoded, &raw); err != nil {
		return MCPResourceReadResult{}, err
	}
	out := MCPResourceReadResult{Contents: make([]MCPResourceReadContent, 0, len(raw.Contents))}
	for _, item := range raw.Contents {
		content, err := normalizeMCPResourceContent(item)
		if err != nil {
			return MCPResourceReadResult{}, err
		}
		out.Contents = append(out.Contents, content)
	}
	return out, nil
}

func (r *mcpRuntime) listResources(ctx context.Context, serverFilter string) ([]MCPResource, error) {
	r.mu.Lock()
	servers := make([]string, 0, len(r.sessions))
	for server := range r.sessions {
		if serverFilter == "" || server == serverFilter {
			servers = append(servers, server)
		}
	}
	r.mu.Unlock()
	sort.Strings(servers)
	var out []MCPResource
	var errs []error
	for _, server := range servers {
		resources, err := r.listResourcesForServer(ctx, server)
		if err != nil {
			errs = append(errs, err)
			continue
		}
		out = append(out, resources...)
	}
	if len(out) == 0 && len(errs) > 0 {
		return nil, errors.Join(errs...)
	}
	return out, nil
}

func (r *mcpRuntime) listResourcesForServer(ctx context.Context, server string) ([]MCPResource, error) {
	result, err := r.listResourcesForServerOnce(ctx, server)
	if err == nil {
		return result, nil
	}
	if !isMCPSessionExpiredError(err) {
		return nil, err
	}
	if _, reconnectErr := r.reconnect(ctx, server); reconnectErr != nil {
		return nil, reconnectErr
	}
	return r.listResourcesForServerOnce(ctx, server)
}

func (r *mcpRuntime) listResourcesForServerOnce(ctx context.Context, server string) ([]MCPResource, error) {
	session, ok := r.session(server)
	if !ok {
		return nil, fmt.Errorf("MCP server %q is not connected", server)
	}
	result, err := session.transport.rpc(ctx, "resources/list", nil, true, nil)
	if err != nil {
		return nil, err
	}
	encoded, err := json.Marshal(result)
	if err != nil {
		return nil, err
	}
	var resources MCPResourcesListResult
	if err := json.Unmarshal(encoded, &resources); err != nil {
		return nil, err
	}
	out := make([]MCPResource, 0, len(resources.Resources))
	for _, resource := range resources.Resources {
		out = append(out, MCPResource{
			URI:         resource.URI,
			Name:        firstNonEmpty(resource.Title, resource.Name),
			Description: resource.Description,
		})
	}
	return out, nil
}

func normalizeMCPResourceContent(item map[string]any) (MCPResourceReadContent, error) {
	content := MCPResourceReadContent{
		URI:      strings.TrimSpace(stringField(item, "uri")),
		MimeType: strings.TrimSpace(stringField(item, "mimeType")),
		Text:     stringField(item, "text"),
	}
	if content.Text != "" {
		return content, nil
	}
	blob := stringField(item, "blob")
	if blob == "" {
		return content, nil
	}
	path, err := saveMCPResourceBlob(content.URI, content.MimeType, blob)
	if err != nil {
		return MCPResourceReadContent{}, err
	}
	content.BlobSavedTo = path
	content.Text = fmt.Sprintf("Binary MCP resource saved to %s", path)
	return content, nil
}

func saveMCPResourceBlob(uri, mimeType, encoded string) (string, error) {
	data, err := base64.StdEncoding.DecodeString(encoded)
	if err != nil {
		return "", err
	}
	outputDir := filepath.Join(os.TempDir(), "myclaw-mcp-output")
	if err := os.MkdirAll(outputDir, 0o755); err != nil {
		return "", err
	}
	prefix := sanitizeMCPFilename(firstNonEmpty(uri, mimeType, "resource"))
	if prefix == "" {
		prefix = "resource"
	}
	file, err := os.CreateTemp(outputDir, prefix+"-*")
	if err != nil {
		return "", err
	}
	defer file.Close()
	if _, err := file.Write(data); err != nil {
		_ = os.Remove(file.Name())
		return "", err
	}
	return file.Name(), nil
}

func sanitizeMCPFilename(value string) string {
	var builder strings.Builder
	previousUnderscore := false
	for _, r := range value {
		valid := (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9')
		if valid {
			builder.WriteRune(r)
			previousUnderscore = false
			continue
		}
		if !previousUnderscore {
			builder.WriteByte('_')
			previousUnderscore = true
		}
	}
	return strings.Trim(builder.String(), "_")
}

type mcpHTTPTransport struct {
	runtime    *mcpRuntime
	connection MCPConnection
}

func (t *mcpHTTPTransport) close() error { return nil }

func (t *mcpHTTPTransport) rpc(ctx context.Context, method string, params map[string]any, expectResponse bool, _ ProgressFunc) (map[string]any, error) {
	request := map[string]any{
		"jsonrpc": "2.0",
		"method":  method,
	}
	if params != nil {
		request["params"] = params
	}
	if expectResponse {
		request["id"] = t.runtime.nextRequestID()
	}
	encoded, err := json.Marshal(request)
	if err != nil {
		return nil, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, connectionURL(t.connection), bytes.NewReader(encoded))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", "myclaw-mcp/1.0")
	headers, _ := resolveMCPRequestHeaders(ctx, t.connection)
	for key, value := range headers {
		if strings.TrimSpace(key) != "" {
			req.Header.Set(key, value)
		}
	}
	resp, err := t.runtime.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		body, _ := io.ReadAll(resp.Body)
		if resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden {
			challenge := extractMCPAuthChallenge(resp, body)
			return nil, &mcpAuthRequiredError{
				serverName: t.connection.Name,
				method:     method,
				statusCode: resp.StatusCode,
				authURL:    challengeAuthURL(challenge),
				scope:      challenge["scope"],
				challenge:  challenge,
				message:    strings.TrimSpace(string(body)),
			}
		}
		if resp.StatusCode == http.StatusNotFound && containsSessionExpiredCode(body) {
			return nil, &mcpSessionExpiredError{
				serverName: t.connection.Name,
				method:     method,
				httpStatus: resp.StatusCode,
				body:       string(body),
			}
		}
		return nil, &mcpHTTPStatusError{
			serverName: t.connection.Name,
			method:     method,
			httpStatus: resp.StatusCode,
			body:       string(body),
		}
	}
	if !expectResponse || resp.StatusCode == http.StatusAccepted || resp.ContentLength == 0 {
		return nil, nil
	}
	var envelope struct {
		Result map[string]any `json:"result"`
		Error  *struct {
			Code    int            `json:"code"`
			Message string         `json:"message"`
			Data    map[string]any `json:"data,omitempty"`
		} `json:"error"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&envelope); err != nil {
		return nil, err
	}
	if envelope.Error != nil {
		if envelope.Error.Code == -32001 {
			return nil, &mcpSessionExpiredError{
				serverName: t.connection.Name,
				method:     method,
				rpcCode:    envelope.Error.Code,
				message:    envelope.Error.Message,
			}
		}
		return nil, &mcpRPCError{
			serverName: t.connection.Name,
			method:     method,
			rpcCode:    envelope.Error.Code,
			message:    envelope.Error.Message,
			data:       envelope.Error.Data,
		}
	}
	if envelope.Result == nil {
		return map[string]any{}, nil
	}
	return envelope.Result, nil
}

type mcpStdioTransport struct {
	runtime    *mcpRuntime
	connection MCPConnection
	cmd        *exec.Cmd
	stdin      io.WriteCloser
	stdout     *bufio.Reader
	stderr     *bytes.Buffer
	mu         sync.Mutex
}

func (t *mcpStdioTransport) close() error {
	if t.stdin != nil {
		_ = t.stdin.Close()
	}
	if t.cmd == nil {
		return nil
	}
	done := make(chan error, 1)
	go func() {
		done <- t.cmd.Wait()
	}()
	select {
	case err := <-done:
		return err
	case <-time.After(2 * time.Second):
	}
	if t.cmd.Process != nil {
		_ = t.cmd.Process.Kill()
	}
	return <-done
}

func (t *mcpStdioTransport) rpc(ctx context.Context, method string, params map[string]any, expectResponse bool, reportProgress ProgressFunc) (map[string]any, error) {
	request := map[string]any{
		"jsonrpc": "2.0",
		"method":  method,
	}
	if params != nil {
		request["params"] = params
	}
	if expectResponse {
		request["id"] = t.runtime.nextRequestID()
	}
	encoded, err := json.Marshal(request)
	if err != nil {
		return nil, err
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	if ctx.Err() != nil {
		return nil, ctx.Err()
	}
	if _, err := t.stdin.Write(append(encoded, '\n')); err != nil {
		return nil, err
	}
	if !expectResponse {
		return nil, nil
	}
	for {
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		line, err := t.stdout.ReadBytes('\n')
		if err != nil {
			return nil, err
		}
		line = bytes.TrimSpace(line)
		if len(line) == 0 {
			continue
		}
		var envelope struct {
			Method string          `json:"method"`
			Params map[string]any  `json:"params"`
			Result json.RawMessage `json:"result"`
			Error  *struct {
				Code    int            `json:"code"`
				Message string         `json:"message"`
				Data    map[string]any `json:"data,omitempty"`
			} `json:"error"`
		}
		if err := json.Unmarshal(line, &envelope); err != nil {
			return nil, err
		}
		if envelope.Method != "" {
			if envelope.Method == "notifications/progress" && reportProgress != nil {
				reportProgress(toolProgressFromMCPNotification(envelope.Params))
			}
			continue
		}
		if envelope.Error != nil {
			return nil, &mcpRPCError{
				serverName: t.connection.Name,
				method:     method,
				rpcCode:    envelope.Error.Code,
				message:    envelope.Error.Message,
				data:       envelope.Error.Data,
			}
		}
		if len(envelope.Result) == 0 {
			continue
		}
		var result map[string]any
		if err := json.Unmarshal(envelope.Result, &result); err != nil {
			return nil, err
		}
		if result == nil {
			return map[string]any{}, nil
		}
		return result, nil
	}
}

func mergedMCPEnvironment(overrides map[string]string) []string {
	env := os.Environ()
	if len(overrides) == 0 {
		return env
	}
	index := make(map[string]int, len(env))
	for i, pair := range env {
		if eq := strings.Index(pair, "="); eq > 0 {
			index[pair[:eq]] = i
		}
	}
	for key, value := range overrides {
		pair := key + "=" + value
		if idx, ok := index[key]; ok {
			env[idx] = pair
			continue
		}
		env = append(env, pair)
	}
	return env
}

func connectionURL(connection MCPConnection) string {
	if url := strings.TrimSpace(connection.BaseURL); url != "" {
		return url
	}
	return strings.TrimSpace(connection.URL)
}

type mcpRPCError struct {
	serverName string
	method     string
	rpcCode    int
	message    string
	data       map[string]any
}

func (e *mcpRPCError) Error() string {
	return fmt.Sprintf("MCP server %q %s failed (%d): %s", e.serverName, e.method, e.rpcCode, e.message)
}

func mcpURLElicitationErrorData(err error) map[string]any {
	var rpcErr *mcpRPCError
	if !errors.As(err, &rpcErr) || rpcErr.rpcCode != -32042 {
		return nil
	}
	if rpcErr.data == nil {
		return map[string]any{"message": rpcErr.message}
	}
	return cloneAnyMap(rpcErr.data)
}

func mcpURLElicitations(err error) []map[string]any {
	data := mcpURLElicitationErrorData(err)
	if data == nil {
		return nil
	}
	raw, ok := data["elicitations"]
	if !ok {
		return nil
	}
	items := make([]any, 0)
	switch typed := raw.(type) {
	case []any:
		items = append(items, typed...)
	case []map[string]any:
		for _, item := range typed {
			items = append(items, item)
		}
	default:
		return nil
	}
	elicitations := make([]map[string]any, 0, len(items))
	for _, item := range items {
		object, ok := item.(map[string]any)
		if !ok {
			continue
		}
		if stringField(object, "mode") != "url" ||
			stringField(object, "url") == "" ||
			stringField(object, "elicitationId") == "" ||
			stringField(object, "message") == "" {
			continue
		}
		elicitations = append(elicitations, cloneAnyMap(object))
	}
	return elicitations
}

func mcpElicitationAction(result ElicitationResult) string {
	if result.Cancelled {
		return "cancel"
	}
	if result.Data != nil {
		if action := strings.ToLower(strings.TrimSpace(stringField(result.Data, "action"))); action != "" {
			return normalizeMCPURLAction(action)
		}
	}
	return normalizeMCPURLAction(result.Value)
}

func normalizeMCPURLAction(action string) string {
	switch strings.ToLower(strings.TrimSpace(action)) {
	case "", "accept", "accepted", "retry", "retry now":
		return "accept"
	case "decline", "declined":
		return "decline"
	case "cancel", "cancelled", "canceled":
		return "cancel"
	default:
		return strings.ToLower(strings.TrimSpace(action))
	}
}

func mcpURLDeclinedToolResult(tool, action string) MCPToolResult {
	word := "cancelled"
	if action == "decline" {
		word = "declined"
	} else if action != "cancel" && strings.TrimSpace(action) != "" {
		word = action + "ed"
	}
	text := fmt.Sprintf("URL elicitation was %s by the user. The tool %q could not complete because it requires the user to open a URL.", word, tool)
	return MCPToolResult{Content: []map[string]any{{"type": "text", "text": text}}}
}

type mcpHTTPStatusError struct {
	serverName string
	method     string
	httpStatus int
	body       string
}

func (e *mcpHTTPStatusError) Error() string {
	if e.body == "" {
		return fmt.Sprintf("MCP server %q returned HTTP %d for %s", e.serverName, e.httpStatus, e.method)
	}
	return fmt.Sprintf("MCP server %q returned HTTP %d for %s: %s", e.serverName, e.httpStatus, e.method, e.body)
}

type mcpAuthRequiredError struct {
	serverName string
	method     string
	statusCode int
	authURL    string
	scope      string
	challenge  map[string]string
	message    string
}

func (e *mcpAuthRequiredError) Error() string {
	if e.authURL != "" {
		return fmt.Sprintf("MCP server %q requires authentication (%d) for %s: %s (%s)", e.serverName, e.statusCode, e.method, e.message, e.authURL)
	}
	return fmt.Sprintf("MCP server %q requires authentication (%d) for %s: %s", e.serverName, e.statusCode, e.method, e.message)
}

type mcpSessionExpiredError struct {
	serverName string
	method     string
	httpStatus int
	rpcCode    int
	message    string
	body       string
}

func (e *mcpSessionExpiredError) Error() string {
	if e.body != "" {
		return fmt.Sprintf("MCP server %q %s session expired: %s", e.serverName, e.method, e.body)
	}
	if e.message != "" {
		return fmt.Sprintf("MCP server %q %s session expired (%d): %s", e.serverName, e.method, e.rpcCode, e.message)
	}
	return fmt.Sprintf("MCP server %q %s session expired", e.serverName, e.method)
}

func isMCPSessionExpiredError(err error) bool {
	var sessionErr *mcpSessionExpiredError
	if errors.As(err, &sessionErr) {
		return true
	}
	var rpcErr *mcpRPCError
	if errors.As(err, &rpcErr) {
		return rpcErr.rpcCode == -32001
	}
	var statusErr *mcpHTTPStatusError
	if errors.As(err, &statusErr) {
		return statusErr.httpStatus == http.StatusNotFound && containsSessionExpiredCode([]byte(statusErr.body))
	}
	return false
}

func containsSessionExpiredCode(body []byte) bool {
	if len(body) == 0 {
		return false
	}
	return bytes.Contains(body, []byte(`"code":-32001`)) || bytes.Contains(body, []byte(`"code": -32001`))
}

func buildMCPAuthToolResult(serverName string, connection MCPConnection, err *mcpAuthRequiredError) MCPAuthToolResult {
	authURL := strings.TrimSpace(err.authURL)
	toolName := BuildMCPToolName(serverName, "authenticate")
	message := fmt.Sprintf("The %s MCP server requires authentication. Use %s to authenticate.", serverName, toolName)
	if details := strings.TrimSpace(err.message); details != "" {
		message = message + " " + details
	}
	if scope := strings.TrimSpace(err.scope); scope != "" {
		message = message + " Required scope: " + scope + "."
	}
	if authURL != "" {
		message = fmt.Sprintf("%s Open this authorization URL to continue: %s", message, authURL)
	}
	return MCPAuthToolResult{
		Name:                toolName,
		Status:              "needs-auth",
		AuthURL:             authURL,
		Message:             message,
		Scope:               strings.TrimSpace(err.scope),
		ResourceMetadataURL: strings.TrimSpace(err.challenge["resource_metadata"]),
		Challenge:           cloneStringMap(err.challenge),
	}
}

func MCPAuthToolResultFromError(serverName string, err error) (MCPAuthToolResult, bool) {
	var authErr *mcpAuthRequiredError
	if !errors.As(err, &authErr) {
		return MCPAuthToolResult{}, false
	}
	serverName = strings.TrimSpace(serverName)
	if serverName == "" {
		serverName = authErr.serverName
	}
	if serverName == "" {
		return MCPAuthToolResult{}, false
	}
	return buildMCPAuthToolResult(serverName, MCPConnection{Name: serverName}, authErr), true
}

func extractMCPAuthURL(resp *http.Response, body []byte) string {
	return challengeAuthURL(extractMCPAuthChallenge(resp, body))
}

func extractMCPAuthChallenge(resp *http.Response, body []byte) map[string]string {
	challenge := make(map[string]string)
	if resp == nil {
		return challenge
	}
	for _, value := range resp.Header.Values("WWW-Authenticate") {
		mergeStringMap(challenge, parseWWWAuthenticate(value))
	}
	if len(body) > 0 {
		mergeStringMap(challenge, parseWWWAuthenticate(string(body)))
	}
	return challenge
}

func challengeAuthURL(challenge map[string]string) string {
	for _, key := range []string{"authorization_uri", "authorization_url", "auth_url"} {
		value := strings.TrimSpace(challenge[key])
		if strings.HasPrefix(value, "http://") || strings.HasPrefix(value, "https://") {
			return value
		}
	}
	return ""
}

func parseAuthorizationURL(value string) string {
	return challengeAuthURL(parseWWWAuthenticate(value))
}

func parseWWWAuthenticate(value string) map[string]string {
	value = strings.TrimSpace(value)
	out := make(map[string]string)
	if value == "" {
		return out
	}
	if strings.HasPrefix(strings.ToLower(value), "bearer ") {
		value = strings.TrimSpace(value[len("bearer "):])
	}
	for _, token := range splitAuthChallengeTokens(value) {
		token = strings.TrimSpace(token)
		key, raw, ok := strings.Cut(token, "=")
		if !ok {
			continue
		}
		key = strings.ToLower(strings.TrimSpace(key))
		raw = strings.Trim(strings.TrimSpace(raw), `"' `)
		if key != "" && raw != "" {
			out[key] = raw
		}
	}
	return out
}

func splitAuthChallengeTokens(value string) []string {
	var tokens []string
	var builder strings.Builder
	inQuote := false
	for _, r := range value {
		switch r {
		case '"':
			inQuote = !inQuote
			builder.WriteRune(r)
		case ',':
			if inQuote {
				builder.WriteRune(r)
				continue
			}
			tokens = append(tokens, builder.String())
			builder.Reset()
		default:
			builder.WriteRune(r)
		}
	}
	if builder.Len() > 0 {
		tokens = append(tokens, builder.String())
	}
	return tokens
}

func mergeStringMap(target, source map[string]string) {
	for key, value := range source {
		if strings.TrimSpace(key) != "" && strings.TrimSpace(value) != "" {
			target[key] = value
		}
	}
}

func resolveMCPRequestHeaders(ctx context.Context, connection MCPConnection) (map[string]string, error) {
	headers := cloneStringMap(connection.Headers)
	helper := strings.TrimSpace(connection.HeadersHelper)
	if helper == "" {
		return headers, nil
	}
	dynamic, err := runMCPHeadersHelper(ctx, connection, helper)
	if err != nil {
		return headers, err
	}
	for key, value := range dynamic {
		headers[key] = value
	}
	return headers, nil
}

func runMCPHeadersHelper(ctx context.Context, connection MCPConnection, helper string) (map[string]string, error) {
	var cmd *exec.Cmd
	if runtime.GOOS == "windows" {
		cmd = exec.CommandContext(ctx, "cmd", "/C", helper)
	} else {
		cmd = exec.CommandContext(ctx, "sh", "-c", helper)
	}
	cmd.Env = mergedMCPEnvironment(map[string]string{
		"CLAUDE_CODE_MCP_SERVER_NAME": connection.Name,
		"CLAUDE_CODE_MCP_SERVER_URL":  connectionURL(connection),
		"GO_WANT_MCP_HEADERS_HELPER":  "1",
	})
	output, err := cmd.Output()
	if err != nil {
		return nil, err
	}
	var headers map[string]string
	if err := json.Unmarshal(bytes.TrimSpace(output), &headers); err != nil {
		return nil, err
	}
	if headers == nil {
		return nil, fmt.Errorf("headers helper returned null")
	}
	for key, value := range headers {
		if strings.TrimSpace(key) == "" {
			return nil, fmt.Errorf("headers helper returned blank header name")
		}
		if strings.TrimSpace(value) == "" {
			return nil, fmt.Errorf("headers helper returned blank value for %q", key)
		}
	}
	return headers, nil
}

func normalizeMCPToolResult(result MCPToolResult, serverName string) MCPToolResult {
	if len(result.Content) == 0 {
		return result
	}
	normalized := make([]map[string]any, 0, len(result.Content))
	for _, item := range result.Content {
		normalized = append(normalized, normalizeMCPResultContentItem(item, serverName))
	}
	result.Content = normalized
	return result
}

func normalizeMCPResultContentItem(item map[string]any, serverName string) map[string]any {
	if item == nil {
		return nil
	}
	typ := strings.ToLower(strings.TrimSpace(stringField(item, "type")))
	switch typ {
	case "blob", "image":
		encoded := stringField(item, "blob")
		if encoded == "" {
			encoded = stringField(item, "data")
		}
		if encoded == "" {
			return item
		}
		mimeType := stringField(item, "mimeType")
		path, err := saveMCPBinaryPayload(serverName, typ, mimeType, encoded)
		if err != nil {
			return map[string]any{
				"type":     "text",
				"text":     fmt.Sprintf("%s binary content could not be saved to disk: %v", strings.Title(typ), err),
				"mimeType": mimeType,
			}
		}
		return map[string]any{
			"type":        "text",
			"text":        fmt.Sprintf("%s binary content saved to %s", strings.Title(typ), path),
			"mimeType":    mimeType,
			"blobSavedTo": path,
		}
	case "resource":
		resource, _ := item["resource"].(map[string]any)
		if resource == nil {
			return item
		}
		if text := strings.TrimSpace(stringField(resource, "text")); text != "" {
			out := map[string]any{
				"type": "text",
				"text": text,
			}
			if uri := strings.TrimSpace(stringField(resource, "uri")); uri != "" {
				out["uri"] = uri
			}
			if mimeType := strings.TrimSpace(stringField(resource, "mimeType")); mimeType != "" {
				out["mimeType"] = mimeType
			}
			return out
		}
		encoded := stringField(resource, "blob")
		if encoded == "" {
			encoded = stringField(resource, "data")
		}
		if encoded == "" {
			return item
		}
		mimeType := strings.TrimSpace(stringField(resource, "mimeType"))
		path, err := saveMCPBinaryPayload(serverName, "resource", mimeType, encoded)
		if err != nil {
			return map[string]any{
				"type":     "text",
				"text":     fmt.Sprintf("Resource binary content could not be saved to disk: %v", err),
				"mimeType": mimeType,
			}
		}
		out := map[string]any{
			"type":        "text",
			"text":        fmt.Sprintf("Resource binary content saved to %s", path),
			"mimeType":    mimeType,
			"blobSavedTo": path,
		}
		if uri := strings.TrimSpace(stringField(resource, "uri")); uri != "" {
			out["uri"] = uri
		}
		return out
	default:
		return item
	}
}

func saveMCPBinaryPayload(serverName, kind, mimeType, encoded string) (string, error) {
	data, err := base64.StdEncoding.DecodeString(encoded)
	if err != nil {
		return "", err
	}
	outputDir := filepath.Join(os.TempDir(), "myclaw-mcp-output")
	if err := os.MkdirAll(outputDir, 0o755); err != nil {
		return "", err
	}
	prefix := sanitizeMCPFilename(firstNonEmpty(serverName, kind, mimeType, "resource"))
	if prefix == "" {
		prefix = "resource"
	}
	file, err := os.CreateTemp(outputDir, prefix+"-*")
	if err != nil {
		return "", err
	}
	defer file.Close()
	if _, err := file.Write(data); err != nil {
		_ = os.Remove(file.Name())
		return "", err
	}
	return file.Name(), nil
}

func (r *mcpRuntime) nextRequestID() int {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.nextID++
	return r.nextID
}
