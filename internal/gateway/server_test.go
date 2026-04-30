package gateway

import (
	"context"
	"io"
	"log"
	"net/http"
	"net/http/httptest"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/gorilla/websocket"

	"myclaw/internal/agent"
	"myclaw/internal/approval"
	"myclaw/internal/llm"
	"myclaw/internal/orchestration"
	"myclaw/internal/permissions"
	protocolws "myclaw/internal/protocol/ws"
	"myclaw/internal/queryengine"
	runtimepkg "myclaw/internal/runtime"
	"myclaw/internal/sandbox"
	"myclaw/internal/session"
	"myclaw/internal/tools"
	systemtools "myclaw/internal/tools/system"
	"myclaw/internal/workspace"
)

type orchestrationHook struct {
	mu     sync.Mutex
	events []orchestration.Event
}

func TestShouldSuppressContinuationRunErrorForApprovalRequired(t *testing.T) {
	err := &queryengine.ApprovalRequiredError{ToolName: "Bash", Reason: "needs approval"}

	if !shouldSuppressContinuationRunError(err) {
		t.Fatalf("shouldSuppressContinuationRunError(%v) = false, want true", err)
	}
}

type permissionHookFunc func(context.Context, queryengine.PermissionHookRequest) (permissions.Decision, bool, error)

func (f permissionHookFunc) CheckPermission(ctx context.Context, request queryengine.PermissionHookRequest) (permissions.Decision, bool, error) {
	return f(ctx, request)
}

type progressToolForGateway struct{}

type shellFailureExecutorForGateway struct{}

func (shellFailureExecutorForGateway) Run(_ context.Context, _ string) (string, error) {
	return "", context.DeadlineExceeded
}

func (shellFailureExecutorForGateway) RunDetailed(_ context.Context, _ string) (sandbox.ExecutionResult, error) {
	return sandbox.ExecutionResult{
		Stdout:        "permission denied",
		Stderr:        "exit status 1",
		ExitCode:      1,
		ExecutionMode: "host",
	}, context.DeadlineExceeded
}

func (progressToolForGateway) Definition() tools.Definition {
	return tools.Definition{
		Name:        "progress.echo",
		Description: "Emit one progress event and return a result.",
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"value": map[string]any{"type": "string"},
			},
			"required": []string{"value"},
		},
	}
}

func (progressToolForGateway) Invoke(_ context.Context, _ session.Session, input string) (string, error) {
	return input, nil
}

func (progressToolForGateway) InvokeWithContext(_ context.Context, toolCtx tools.ToolUseContext) (tools.ToolResult, error) {
	toolCtx.ReportProgress(tools.ToolProgress{
		ToolUseID: toolCtx.ToolUseID,
		Type:      "progress",
		Message:   "halfway there",
		Data: map[string]any{
			"phase": "mid",
		},
	})
	return tools.ToolResult{Output: "progress complete"}, nil
}

func (progressToolForGateway) IsEnabled() bool           { return true }
func (progressToolForGateway) IsReadOnly(string) bool    { return true }
func (progressToolForGateway) IsDestructive(string) bool { return false }
func (progressToolForGateway) PromptDescription() string { return "emit one progress event" }
func (progressToolForGateway) SearchHint() string        { return "progress echo" }
func (progressToolForGateway) AlwaysLoad() bool          { return false }
func (progressToolForGateway) ShouldDefer() bool         { return false }

func (h *orchestrationHook) Handle(_ context.Context, event orchestration.Event) error {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.events = append(h.events, event)
	return nil
}

func (h *orchestrationHook) Events() []orchestration.Event {
	h.mu.Lock()
	defer h.mu.Unlock()
	out := make([]orchestration.Event, len(h.events))
	copy(out, h.events)
	return out
}

func waitForPermissionRequired(t *testing.T, conn *websocket.Conn) string {
	t.Helper()
	if err := conn.SetReadDeadline(time.Now().Add(2 * time.Second)); err != nil {
		t.Fatalf("set permission.required read deadline: %v", err)
	}
	defer func() {
		if err := conn.SetReadDeadline(time.Time{}); err != nil {
			t.Fatalf("clear permission.required read deadline: %v", err)
		}
	}()

	for i := 0; i < 16; i++ {
		var event protocolws.Message
		if err := conn.ReadJSON(&event); err != nil {
			t.Fatalf("read permission-producing event %d: %v", i, err)
		}
		if event.Type == protocolws.TypeEvent && event.Event == "permission.required" {
			approvalID, _ := event.Payload["approval_id"].(string)
			return approvalID
		}
	}

	t.Fatal("expected permission.required event")
	return ""
}

func waitForEvent(t *testing.T, conn *websocket.Conn, eventName string) protocolws.Message {
	t.Helper()
	if err := conn.SetReadDeadline(time.Now().Add(2 * time.Second)); err != nil {
		t.Fatalf("set %s read deadline: %v", eventName, err)
	}
	defer func() {
		if err := conn.SetReadDeadline(time.Time{}); err != nil {
			t.Fatalf("clear %s read deadline: %v", eventName, err)
		}
	}()

	for i := 0; i < 24; i++ {
		var event protocolws.Message
		if err := conn.ReadJSON(&event); err != nil {
			t.Fatalf("read event while waiting for %s: %v", eventName, err)
		}
		if event.Type == protocolws.TypeEvent && event.Event == eventName {
			return event
		}
	}

	t.Fatalf("expected %s event", eventName)
	return protocolws.Message{}
}

func waitForResponseID(t *testing.T, conn *websocket.Conn, id string) protocolws.Message {
	t.Helper()
	if err := conn.SetReadDeadline(time.Now().Add(2 * time.Second)); err != nil {
		t.Fatalf("set response %s read deadline: %v", id, err)
	}
	defer func() {
		if err := conn.SetReadDeadline(time.Time{}); err != nil {
			t.Fatalf("clear response %s read deadline: %v", id, err)
		}
	}()

	for i := 0; i < 24; i++ {
		var msg protocolws.Message
		if err := conn.ReadJSON(&msg); err != nil {
			t.Fatalf("read message while waiting for response %s: %v", id, err)
		}
		if msg.Type == protocolws.TypeResponse && msg.ID == id {
			return msg
		}
	}

	t.Fatalf("expected response %s", id)
	return protocolws.Message{}
}

func readCanUseToolControlRequest(t *testing.T, conn *websocket.Conn) protocolws.Message {
	t.Helper()
	if err := conn.SetReadDeadline(time.Now().Add(2 * time.Second)); err != nil {
		t.Fatalf("set can_use_tool read deadline: %v", err)
	}
	defer func() {
		if err := conn.SetReadDeadline(time.Time{}); err != nil {
			t.Fatalf("clear can_use_tool read deadline: %v", err)
		}
	}()

	for i := 0; i < 16; i++ {
		var message protocolws.Message
		if err := conn.ReadJSON(&message); err != nil {
			t.Fatalf("read can_use_tool event %d: %v", i, err)
		}
		if message.Type == protocolws.TypeControlRequest {
			request, _ := message.Payload["request"].(map[string]any)
			if request["subtype"] == "can_use_tool" {
				return message
			}
		}
		if message.Type == protocolws.TypeEvent && message.Event == "permission.required" {
			t.Fatalf("unexpected permission.required before can_use_tool: %#v", message.Payload)
		}
		if message.Type == protocolws.TypeEvent && message.Event == "run.error" {
			t.Fatalf("unexpected run.error before can_use_tool: %#v", message.Payload)
		}
	}

	t.Fatal("expected can_use_tool control_request")
	return protocolws.Message{}
}

func connectGatewayTestClient(t *testing.T, server *Server) *websocket.Conn {
	t.Helper()
	httpServer := httptest.NewServer(http.HandlerFunc(server.HandleWebSocket))
	t.Cleanup(httpServer.Close)

	wsURL := "ws" + strings.TrimPrefix(httpServer.URL, "http")
	conn, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
	if err != nil {
		t.Fatalf("dial websocket: %v", err)
	}
	t.Cleanup(func() { _ = conn.Close() })

	if err := conn.WriteJSON(protocolws.Message{
		Type:   protocolws.TypeRequest,
		ID:     "1",
		Method: protocolws.MethodConnect,
		Payload: map[string]any{
			"role":            "sdk",
			"client_identity": "mcp-test",
			"agent_id":        "main",
		},
	}); err != nil {
		t.Fatalf("write connect: %v", err)
	}

	var response protocolws.Message
	if err := conn.ReadJSON(&response); err != nil {
		t.Fatalf("read connect response: %v", err)
	}
	if response.Type != protocolws.TypeResponse || !response.OK {
		t.Fatalf("connect response = %#v, want ok response", response)
	}
	_ = waitForEvent(t, conn, protocolws.EventHello)
	return conn
}

func readGatewayResponse(t *testing.T, conn *websocket.Conn) protocolws.Message {
	t.Helper()
	if err := conn.SetReadDeadline(time.Now().Add(2 * time.Second)); err != nil {
		t.Fatalf("set response read deadline: %v", err)
	}
	defer func() {
		if err := conn.SetReadDeadline(time.Time{}); err != nil {
			t.Fatalf("clear response read deadline: %v", err)
		}
	}()

	var response protocolws.Message
	if err := conn.ReadJSON(&response); err != nil {
		t.Fatalf("read response: %v", err)
	}
	return response
}

func TestGatewaySessionLifecycleMethods(t *testing.T) {
	manager := session.NewManager(nil)
	server := NewServer(log.New(io.Discard, "", 0), manager, llm.NewMockClient())
	conn := connectGatewayTestClient(t, server)

	if err := conn.WriteJSON(protocolws.Message{
		Type:    protocolws.TypeRequest,
		ID:      "session-list-1",
		Method:  protocolws.MethodSessionList,
		Payload: map[string]any{},
	}); err != nil {
		t.Fatalf("write session_list: %v", err)
	}
	listResponse := readGatewayResponse(t, conn)
	if !listResponse.OK {
		t.Fatalf("session_list response = %#v, want ok", listResponse)
	}
	sessions, _ := listResponse.Payload["sessions"].([]any)
	if len(sessions) != 1 {
		t.Fatalf("session_list returned %d sessions, want 1", len(sessions))
	}
	mainSession, _ := sessions[0].(map[string]any)
	if mainSession["title"] != "Main session" {
		t.Fatalf("main session title = %v, want Main session", mainSession["title"])
	}

	if err := conn.WriteJSON(protocolws.Message{
		Type:    protocolws.TypeRequest,
		ID:      "session-new-1",
		Method:  protocolws.MethodSessionNew,
		Payload: map[string]any{},
	}); err != nil {
		t.Fatalf("write session_new: %v", err)
	}
	newResponse := readGatewayResponse(t, conn)
	if !newResponse.OK {
		t.Fatalf("session_new response = %#v, want ok", newResponse)
	}
	newSessionID, _ := newResponse.Payload["session_id"].(string)
	newSessionKey, _ := newResponse.Payload["session_key"].(string)
	if newSessionID == "" || newSessionKey == "" {
		t.Fatalf("session_new payload missing session identity: %#v", newResponse.Payload)
	}
	hello := waitForEvent(t, conn, protocolws.EventHello)
	if hello.Payload["session_key"] != newSessionKey {
		t.Fatalf("session_new hello session_key = %v, want %s", hello.Payload["session_key"], newSessionKey)
	}

	if _, err := manager.AppendMessage(newSessionID, "user", "Investigate websocket sessions"); err != nil {
		t.Fatalf("append user message: %v", err)
	}
	if _, err := manager.AppendMessage(newSessionID, "assistant", "Session restored"); err != nil {
		t.Fatalf("append assistant message: %v", err)
	}

	if err := conn.WriteJSON(protocolws.Message{
		Type:   protocolws.TypeRequest,
		ID:     "session-messages-1",
		Method: protocolws.MethodSessionMessages,
		Payload: map[string]any{
			"session_key": newSessionKey,
		},
	}); err != nil {
		t.Fatalf("write session_messages: %v", err)
	}
	messagesResponse := readGatewayResponse(t, conn)
	if !messagesResponse.OK {
		t.Fatalf("session_messages response = %#v, want ok", messagesResponse)
	}
	messages, _ := messagesResponse.Payload["messages"].([]any)
	if len(messages) != 2 {
		t.Fatalf("session_messages returned %d messages, want 2", len(messages))
	}
	firstMessage, _ := messages[0].(map[string]any)
	if firstMessage["role"] != "user" || firstMessage["content"] != "Investigate websocket sessions" {
		t.Fatalf("first restored message = %#v", firstMessage)
	}

	if err := conn.WriteJSON(protocolws.Message{
		Type:    protocolws.TypeRequest,
		ID:      "session-list-2",
		Method:  protocolws.MethodSessionList,
		Payload: map[string]any{},
	}); err != nil {
		t.Fatalf("write second session_list: %v", err)
	}
	secondListResponse := readGatewayResponse(t, conn)
	if !secondListResponse.OK {
		t.Fatalf("second session_list response = %#v, want ok", secondListResponse)
	}
	secondSessions, _ := secondListResponse.Payload["sessions"].([]any)
	if len(secondSessions) != 2 {
		t.Fatalf("second session_list returned %d sessions, want 2", len(secondSessions))
	}
	var foundNew bool
	for _, raw := range secondSessions {
		item, _ := raw.(map[string]any)
		if item["session_key"] == newSessionKey {
			foundNew = true
			if item["title"] != "Investigate websocket sessions" {
				t.Fatalf("new session title = %v, want last user message", item["title"])
			}
			if item["message_count"] != float64(2) {
				t.Fatalf("new session message_count = %v, want 2", item["message_count"])
			}
		}
	}
	if !foundNew {
		t.Fatalf("new session %s not found in session_list: %#v", newSessionKey, secondSessions)
	}
}

func TestGatewayDeletesNonActiveSession(t *testing.T) {
	manager := session.NewManager(nil)
	server := NewServer(log.New(io.Discard, "", 0), manager, llm.NewMockClient())
	conn := connectGatewayTestClient(t, server)

	toDelete := manager.CreateSession("main")
	keep := manager.CreateSession("main")

	if err := conn.WriteJSON(protocolws.Message{
		Type:   protocolws.TypeRequest,
		ID:     "delete-session-1",
		Method: protocolws.MethodSessionDelete,
		Payload: map[string]any{
			"session_key": toDelete.Key,
		},
	}); err != nil {
		t.Fatalf("write session_delete: %v", err)
	}
	deleteResponse := readGatewayResponse(t, conn)
	if !deleteResponse.OK {
		t.Fatalf("session_delete response = %#v, want ok", deleteResponse)
	}
	if _, ok := manager.GetByID(toDelete.ID); ok {
		t.Fatalf("deleted session %s still exists", toDelete.ID)
	}
	if _, ok := manager.GetByID(keep.ID); !ok {
		t.Fatalf("non-deleted session %s missing", keep.ID)
	}
}

func TestGatewayAllowsDeletingActiveNonMainSession(t *testing.T) {
	manager := session.NewManager(nil)
	server := NewServer(log.New(io.Discard, "", 0), manager, llm.NewMockClient())
	conn := connectGatewayTestClient(t, server)

	if err := conn.WriteJSON(protocolws.Message{
		Type:    protocolws.TypeRequest,
		ID:      "create-active-session",
		Method:  protocolws.MethodSessionNew,
		Payload: map[string]any{},
	}); err != nil {
		t.Fatalf("write session_new: %v", err)
	}
	createResponse := readGatewayResponse(t, conn)
	if !createResponse.OK {
		t.Fatalf("session_new response = %#v, want ok", createResponse)
	}
	newSessionKey, _ := createResponse.Payload["session_key"].(string)
	_ = waitForEvent(t, conn, protocolws.EventHello)

	if err := conn.WriteJSON(protocolws.Message{
		Type:   protocolws.TypeRequest,
		ID:     "delete-active-session",
		Method: protocolws.MethodSessionDelete,
		Payload: map[string]any{
			"session_key": newSessionKey,
		},
	}); err != nil {
		t.Fatalf("write active non-main session_delete: %v", err)
	}
	deleteResponse := readGatewayResponse(t, conn)
	if !deleteResponse.OK {
		t.Fatalf("active non-main session_delete response = %#v, want ok", deleteResponse)
	}
	if _, ok := manager.GetByKey(newSessionKey); ok {
		t.Fatalf("deleted active session %s still exists", newSessionKey)
	}
	activeSessionKey, _ := deleteResponse.Payload["active_session_key"].(string)
	if activeSessionKey == "" || activeSessionKey == newSessionKey {
		t.Fatalf("active_session_key = %q, want fallback session", activeSessionKey)
	}
	hello := waitForEvent(t, conn, protocolws.EventHello)
	if hello.Payload["session_key"] != activeSessionKey {
		t.Fatalf("delete active hello session_key = %v, want %s", hello.Payload["session_key"], activeSessionKey)
	}
}

func TestGatewayRejectsDeletingMainSession(t *testing.T) {
	manager := session.NewManager(nil)
	server := NewServer(log.New(io.Discard, "", 0), manager, llm.NewMockClient())
	conn := connectGatewayTestClient(t, server)

	main := manager.GetOrCreateMain("main")
	if err := conn.WriteJSON(protocolws.Message{
		Type:   protocolws.TypeRequest,
		ID:     "delete-main-session",
		Method: protocolws.MethodSessionDelete,
		Payload: map[string]any{
			"session_key": main.Key,
		},
	}); err != nil {
		t.Fatalf("write main session_delete: %v", err)
	}
	deleteResponse := readGatewayResponse(t, conn)
	if deleteResponse.OK {
		t.Fatalf("main session_delete response = %#v, want error", deleteResponse)
	}
	if deleteResponse.Error == nil || deleteResponse.Error.Message != "main session cannot be deleted" {
		t.Fatalf("main session_delete error = %#v", deleteResponse.Error)
	}
}

func waitForGatewayMCPServerStatus(t *testing.T, conn *websocket.Conn, serverName, wantStatus string) map[string]any {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if err := conn.WriteJSON(protocolws.Message{
			Type:   protocolws.TypeRequest,
			ID:     "mcp-status",
			Method: protocolws.MethodMCPStatus,
			Payload: map[string]any{
				"server": serverName,
			},
		}); err != nil {
			t.Fatalf("write mcp_status while waiting for %s: %v", wantStatus, err)
		}
		response := readGatewayResponse(t, conn)
		if !response.OK {
			t.Fatalf("mcp_status response = %#v, want ok", response)
		}
		servers, _ := response.Payload["servers"].([]any)
		if len(servers) != 1 {
			t.Fatalf("servers = %#v, want single filtered server", response.Payload["servers"])
		}
		serverPayload, _ := servers[0].(map[string]any)
		if serverPayload["status"] == wantStatus {
			return serverPayload
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("timed out waiting for MCP server %q to reach status %q", serverName, wantStatus)
	return nil
}

func TestHandleWebSocketMCPStatusReturnsNeedsAuthInventory(t *testing.T) {
	sessionManager := session.NewManager(nil)
	server := NewServer(log.New(io.Discard, "", 0), sessionManager, llm.NewMockClient())
	server.runner = runtimepkg.NewRunnerWithOptions(sessionManager, llm.NewMockClient(), workspace.NewLoader(""), nil, runtimepkg.Options{
		PermissionPolicy: permissions.Policy{Mode: permissions.ModeDangerFullAccess},
		MCPClients:       []tools.MCPConnection{{Name: "filesystem", Type: "streamable_http", BaseURL: "https://mcp.example"}},
		MCPSkills: map[string][]tools.SkillCommand{
			"filesystem": {{Name: "docs-skill", DisplayName: "Docs Skill"}},
		},
		MCPNeedsAuth: map[string]tools.MCPAuthToolResult{
			"filesystem": {
				Status:  "needs-auth",
				AuthURL: "https://auth.example/authorize",
				Message: "Authenticate filesystem",
			},
		},
	})
	server.queue = runtimepkg.NewQueue(server.runner)

	conn := connectGatewayTestClient(t, server)
	if err := conn.WriteJSON(protocolws.Message{
		Type:   protocolws.TypeRequest,
		ID:     "2",
		Method: protocolws.MethodMCPStatus,
		Payload: map[string]any{
			"server": "filesystem",
		},
	}); err != nil {
		t.Fatalf("write mcp_status: %v", err)
	}

	response := readGatewayResponse(t, conn)
	if !response.OK {
		t.Fatalf("response = %#v, want ok mcp_status response", response)
	}
	inventory, _ := response.Payload["inventory"].(map[string]any)
	if inventory["server_count"] != float64(1) && inventory["server_count"] != 1 {
		t.Fatalf("inventory = %#v, want one MCP server", inventory)
	}
	if inventory["skill_count"] != float64(1) && inventory["skill_count"] != 1 {
		t.Fatalf("inventory = %#v, want one MCP-derived skill", inventory)
	}
	servers, _ := response.Payload["servers"].([]any)
	if len(servers) != 1 {
		t.Fatalf("servers = %#v, want single filtered MCP server", response.Payload["servers"])
	}
	serverPayload, _ := servers[0].(map[string]any)
	if serverPayload["name"] != "filesystem" || serverPayload["status"] != "needs-auth" || serverPayload["auth_url"] != "https://auth.example/authorize" {
		t.Fatalf("server payload = %#v, want explicit needs-auth MCP surface", serverPayload)
	}
	skillsPayload, _ := serverPayload["skills"].([]any)
	if len(skillsPayload) != 1 || skillsPayload[0] != "Docs Skill" {
		t.Fatalf("server payload = %#v, want MCP skill surface", serverPayload)
	}
}

func TestHandleWebSocketMCPStatusReturnsStartupDiscoveryFailure(t *testing.T) {
	sessionManager := session.NewManager(nil)
	server := NewServer(log.New(io.Discard, "", 0), sessionManager, llm.NewMockClient())
	server.runner = runtimepkg.NewRunnerWithOptions(sessionManager, llm.NewMockClient(), workspace.NewLoader(""), nil, runtimepkg.Options{
		PermissionPolicy: permissions.Policy{Mode: permissions.ModeDangerFullAccess},
		MCPClients:       []tools.MCPConnection{{Name: "broken", Type: "stdio"}},
	})
	server.queue = runtimepkg.NewQueue(server.runner)

	conn := connectGatewayTestClient(t, server)
	if err := conn.WriteJSON(protocolws.Message{
		Type:   protocolws.TypeRequest,
		ID:     "2",
		Method: protocolws.MethodMCPStatus,
		Payload: map[string]any{
			"server": "broken",
		},
	}); err != nil {
		t.Fatalf("write mcp_status: %v", err)
	}

	response := readGatewayResponse(t, conn)
	if !response.OK {
		t.Fatalf("response = %#v, want ok mcp_status response", response)
	}
	servers, _ := response.Payload["servers"].([]any)
	if len(servers) != 1 {
		t.Fatalf("servers = %#v, want single filtered MCP server", response.Payload["servers"])
	}
	serverPayload, _ := servers[0].(map[string]any)
	if serverPayload["status"] != "error" {
		t.Fatalf("server payload = %#v, want explicit error status", serverPayload)
	}
	if !strings.Contains(serverPayload["error"].(string), "missing command") {
		t.Fatalf("server payload = %#v, want startup discovery failure detail", serverPayload)
	}
}

func TestHandleWebSocketMCPReconnectRefreshesInventory(t *testing.T) {
	sessionManager := session.NewManager(nil)
	server := NewServer(log.New(io.Discard, "", 0), sessionManager, llm.NewMockClient())
	server.runner = runtimepkg.NewRunnerWithOptions(sessionManager, llm.NewMockClient(), workspace.NewLoader(""), nil, runtimepkg.Options{
		PermissionPolicy: permissions.Policy{Mode: permissions.ModeDangerFullAccess},
		MCPClients:       []tools.MCPConnection{{Name: "filesystem", Type: "configured"}},
		MCPTools: map[string]tools.MCPToolsListResult{
			"filesystem": {Tools: []tools.MCPToolListItem{{Name: "old_tool"}}},
		},
		MCPSkills: map[string][]tools.SkillCommand{
			"filesystem": {{Name: "old-skill", DisplayName: "Old Skill"}},
		},
		MCPReconnect: func(_ context.Context, server string) (tools.MCPReconnectResult, error) {
			if server != "filesystem" {
				t.Fatalf("reconnect server = %q, want filesystem", server)
			}
			return tools.MCPReconnectResult{
				Client: tools.MCPConnection{Name: "filesystem", Type: "streamable_http", BaseURL: "https://mcp.example"},
				Tools:  tools.MCPToolsListResult{Tools: []tools.MCPToolListItem{{Name: "new_tool"}}},
				Skills: []tools.SkillCommand{{Name: "new-skill", DisplayName: "New Skill"}},
			}, nil
		},
	})
	server.queue = runtimepkg.NewQueue(server.runner)

	conn := connectGatewayTestClient(t, server)
	if err := conn.WriteJSON(protocolws.Message{
		Type:   protocolws.TypeRequest,
		ID:     "2",
		Method: protocolws.MethodMCPReconnect,
		Payload: map[string]any{
			"server": "filesystem",
		},
	}); err != nil {
		t.Fatalf("write mcp_reconnect: %v", err)
	}

	response := readGatewayResponse(t, conn)
	if !response.OK || response.Payload["status"] != "reconnected" {
		t.Fatalf("response = %#v, want successful reconnect response", response)
	}
	serverPayload, _ := response.Payload["server"].(map[string]any)
	toolsPayload, _ := serverPayload["tools"].([]any)
	if len(toolsPayload) != 1 || toolsPayload[0] != "new_tool" {
		t.Fatalf("server payload = %#v, want refreshed MCP tool inventory", serverPayload)
	}
	skillsPayload, _ := serverPayload["skills"].([]any)
	if len(skillsPayload) != 1 || skillsPayload[0] != "New Skill" {
		t.Fatalf("server payload = %#v, want refreshed MCP skill inventory", serverPayload)
	}
}

func TestHandleWebSocketMCPAuthenticateUsesStoredAuthContextAndReconnectsOnCompletion(t *testing.T) {
	completion := make(chan tools.MCPAuthCompletionResult, 1)
	challenge := map[string]string{
		"authorization_uri": "https://auth.example/authorize",
		"resource_metadata": "https://auth.example/.well-known/oauth-protected-resource",
	}
	sessionManager := session.NewManager(nil)
	server := NewServer(log.New(io.Discard, "", 0), sessionManager, llm.NewMockClient())
	server.runner = runtimepkg.NewRunnerWithOptions(sessionManager, llm.NewMockClient(), workspace.NewLoader(""), nil, runtimepkg.Options{
		PermissionPolicy: permissions.Policy{Mode: permissions.ModeDangerFullAccess},
		MCPClients:       []tools.MCPConnection{{Name: "filesystem", Type: "streamable_http", BaseURL: "https://mcp.example"}},
		MCPNeedsAuth: map[string]tools.MCPAuthToolResult{
			"filesystem": {
				Status:              "needs-auth",
				AuthURL:             "https://auth.example/original",
				Message:             "Authenticate filesystem",
				Scope:               "files:read",
				ResourceMetadataURL: "https://auth.example/.well-known/oauth-protected-resource",
				Challenge:           challenge,
			},
		},
		MCPAuthenticator: func(_ context.Context, server string, connection tools.MCPConnection) (tools.MCPAuthStartResult, error) {
			if server != "filesystem" || connection.BaseURL != "https://mcp.example" {
				t.Fatalf("authenticate server=%q connection=%#v, want filesystem MCP connection", server, connection)
			}
			if connection.AuthURL != "https://auth.example/original" || connection.AuthScope != "files:read" {
				t.Fatalf("authenticate connection = %#v, want stored auth URL and scope", connection)
			}
			if connection.AuthResourceMetadataURL != "https://auth.example/.well-known/oauth-protected-resource" {
				t.Fatalf("authenticate connection = %#v, want stored auth resource metadata", connection)
			}
			if connection.AuthChallenge["authorization_uri"] != challenge["authorization_uri"] {
				t.Fatalf("authenticate connection = %#v, want stored auth challenge", connection)
			}
			return tools.MCPAuthStartResult{
				Status:              "auth_url",
				AuthURL:             "https://auth.example/authorize",
				Message:             "Open browser",
				Scope:               "files:read",
				ResourceMetadataURL: "https://auth.example/.well-known/oauth-protected-resource",
				Challenge:           challenge,
				Completion:          completion,
			}, nil
		},
		MCPReconnect: func(_ context.Context, server string) (tools.MCPReconnectResult, error) {
			if server != "filesystem" {
				t.Fatalf("reconnect server = %q, want filesystem", server)
			}
			return tools.MCPReconnectResult{
				Client:    tools.MCPConnection{Name: "filesystem", Type: "streamable_http", BaseURL: "https://mcp.example"},
				Tools:     tools.MCPToolsListResult{Tools: []tools.MCPToolListItem{{Name: "new_tool"}}},
				Prompts:   tools.MCPPromptsListResult{Prompts: []tools.MCPPromptListItem{{Name: "new_prompt"}}},
				Resources: []tools.MCPResource{{URI: "file:///new.txt", Name: "new"}},
				Skills:    []tools.SkillCommand{{Name: "new-skill", DisplayName: "New Skill"}},
			}, nil
		},
	})
	server.queue = runtimepkg.NewQueue(server.runner)

	conn := connectGatewayTestClient(t, server)
	if err := conn.WriteJSON(protocolws.Message{
		Type:   protocolws.TypeRequest,
		ID:     "2",
		Method: protocolws.MethodMCPAuthenticate,
		Payload: map[string]any{
			"server": "filesystem",
		},
	}); err != nil {
		t.Fatalf("write mcp_authenticate: %v", err)
	}

	response := readGatewayResponse(t, conn)
	if !response.OK || response.Payload["status"] != "auth_url" {
		t.Fatalf("response = %#v, want auth_url response", response)
	}
	authPayload, _ := response.Payload["auth"].(map[string]any)
	if authPayload["auth_url"] != "https://auth.example/authorize" || authPayload["scope"] != "files:read" {
		t.Fatalf("auth payload = %#v, want auth start details", authPayload)
	}
	if authPayload["resource_metadata_url"] != "https://auth.example/.well-known/oauth-protected-resource" {
		t.Fatalf("auth payload = %#v, want auth resource metadata", authPayload)
	}
	authChallenge, _ := authPayload["challenge"].(map[string]any)
	if authChallenge["authorization_uri"] != challenge["authorization_uri"] {
		t.Fatalf("auth payload = %#v, want preserved auth challenge", authPayload)
	}
	serverPayload, _ := response.Payload["server"].(map[string]any)
	if serverPayload["status"] != "needs-auth" {
		t.Fatalf("server payload = %#v, want explicit needs-auth status after auth start", serverPayload)
	}
	completion <- tools.MCPAuthCompletionResult{Status: "complete", Message: "Authenticated"}

	refreshed := waitForGatewayMCPServerStatus(t, conn, "filesystem", "connected")
	toolsPayload, _ := refreshed["tools"].([]any)
	if len(toolsPayload) != 1 || toolsPayload[0] != "new_tool" {
		t.Fatalf("server payload = %#v, want reconnected MCP tool inventory", refreshed)
	}
	promptsPayload, _ := refreshed["prompts"].([]any)
	if len(promptsPayload) != 1 || promptsPayload[0] != "new_prompt" {
		t.Fatalf("server payload = %#v, want reconnected MCP prompt inventory", refreshed)
	}
	skillsPayload, _ := refreshed["skills"].([]any)
	if len(skillsPayload) != 1 || skillsPayload[0] != "New Skill" {
		t.Fatalf("server payload = %#v, want reconnected MCP skill inventory", refreshed)
	}
}

func TestHandleWebSocketConnectAndSendMessage(t *testing.T) {
	sessionManager := session.NewManager(nil)
	server := NewServer(log.New(io.Discard, "", 0), sessionManager, llm.NewMockClient())
	httpServer := httptest.NewServer(http.HandlerFunc(server.HandleWebSocket))
	t.Cleanup(httpServer.Close)

	wsURL := "ws" + strings.TrimPrefix(httpServer.URL, "http")
	conn, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
	if err != nil {
		t.Fatalf("dial websocket: %v", err)
	}
	t.Cleanup(func() {
		_ = conn.Close()
	})

	connect := protocolws.Message{
		Type:   protocolws.TypeRequest,
		ID:     "1",
		Method: protocolws.MethodConnect,
		Payload: map[string]any{
			"role":            "client",
			"client_identity": "web-ui",
			"agent_id":        "main",
		},
	}
	if err := conn.WriteJSON(connect); err != nil {
		t.Fatalf("write connect: %v", err)
	}

	var connectRes protocolws.Message
	if err := conn.ReadJSON(&connectRes); err != nil {
		t.Fatalf("read connect response: %v", err)
	}
	if connectRes.Type != protocolws.TypeResponse || !connectRes.OK {
		t.Fatalf("connect response = %#v, want ok response", connectRes)
	}
	sessionID, _ := connectRes.Payload["session_id"].(string)
	sessionKey, _ := connectRes.Payload["session_key"].(string)
	if sessionID == "" || sessionKey == "" {
		t.Fatalf("missing session info in payload: %#v", connectRes.Payload)
	}

	var hello protocolws.Message
	if err := conn.ReadJSON(&hello); err != nil {
		t.Fatalf("read hello event: %v", err)
	}
	if hello.Type != protocolws.TypeEvent || hello.Event != protocolws.EventHello {
		t.Fatalf("hello event = %#v, want hello event", hello)
	}
	if got := hello.Payload["client_identity"]; got != "web-ui" {
		t.Fatalf("hello client identity = %#v, want %q", got, "web-ui")
	}

	inbound := protocolws.Message{
		Type:   protocolws.TypeRequest,
		ID:     "2",
		Method: protocolws.MethodSendMessage,
		Payload: map[string]any{
			"content": "hello",
		},
	}
	if err := conn.WriteJSON(inbound); err != nil {
		t.Fatalf("write inbound: %v", err)
	}

	var ack protocolws.Message
	if err := conn.ReadJSON(&ack); err != nil {
		t.Fatalf("read ack: %v", err)
	}
	if ack.Type != protocolws.TypeResponse || !ack.OK {
		t.Fatalf("ack = %#v, want ok response", ack)
	}

	var created protocolws.Message
	if err := conn.ReadJSON(&created); err != nil {
		t.Fatalf("read created event: %v", err)
	}

	if created.Type != protocolws.TypeEvent || created.Event != protocolws.EventMessageCreated {
		t.Fatalf("created event = %#v, want message.created event", created)
	}
	if got := created.Payload["session_id"]; got != sessionID {
		t.Fatalf("created session id = %#v, want %q", got, sessionID)
	}
	if got := created.Payload["session_key"]; got != sessionKey {
		t.Fatalf("created session key = %#v, want %q", got, sessionKey)
	}
	message, ok := created.Payload["message"].(map[string]any)
	if !ok {
		t.Fatalf("created message = %#v, want object", created.Payload["message"])
	}
	if got := message["content"]; got != "hello" {
		t.Fatalf("created message content = %#v, want %q", got, "hello")
	}

	var lifecycleStart protocolws.Message
	if err := conn.ReadJSON(&lifecycleStart); err != nil {
		t.Fatalf("read lifecycle start: %v", err)
	}
	if lifecycleStart.Type != protocolws.TypeEvent || lifecycleStart.Event != "agent.lifecycle.start" {
		t.Fatalf("lifecycle start = %#v, want agent.lifecycle.start", lifecycleStart)
	}

	var assistantCreated protocolws.Message
	deltaCount := 0
	for {
		var event protocolws.Message
		if err := conn.ReadJSON(&event); err != nil {
			t.Fatalf("read runtime event: %v", err)
		}

		if event.Type == protocolws.TypeEvent && event.Event == "assistant.delta" {
			deltaCount++
			continue
		}
		if event.Type == protocolws.TypeEvent && (event.Event == "model.request.start" || event.Event == "model.request.end") {
			continue
		}

		assistantCreated = event
		break
	}
	if deltaCount == 0 {
		t.Fatal("expected at least one assistant.delta event")
	}
	if assistantCreated.Type != protocolws.TypeEvent || assistantCreated.Event != protocolws.EventMessageCreated {
		t.Fatalf("assistant created = %#v, want message.created", assistantCreated)
	}
	assistantMessage, ok := assistantCreated.Payload["message"].(map[string]any)
	if !ok {
		t.Fatalf("assistant message = %#v, want object", assistantCreated.Payload["message"])
	}
	if got := assistantMessage["role"]; got != "assistant" {
		t.Fatalf("assistant role = %#v, want %q", got, "assistant")
	}
	if got := assistantMessage["content"]; got != "Received: hello [workspace:3]" {
		t.Fatalf("assistant content = %#v, want %q", got, "Received: hello [workspace:3]")
	}

	var lifecycleEnd protocolws.Message
	if err := conn.ReadJSON(&lifecycleEnd); err != nil {
		t.Fatalf("read lifecycle end: %v", err)
	}
	if lifecycleEnd.Type != protocolws.TypeEvent || lifecycleEnd.Event != "agent.lifecycle.end" {
		t.Fatalf("lifecycle end = %#v, want agent.lifecycle.end", lifecycleEnd)
	}

	messages, ok := sessionManager.Messages(sessionID)
	if !ok {
		t.Fatalf("messages for session %q not found", sessionID)
	}
	var conversation []session.Message
	for _, message := range messages {
		if message.Role == "attachment" && message.Subtype == "skill_listing" {
			continue
		}
		conversation = append(conversation, message)
	}
	if len(conversation) != 2 {
		t.Fatalf("conversation message count = %d in %#v, want user and assistant", len(conversation), messages)
	}
	if conversation[0].Content != "hello" {
		t.Fatalf("stored message content = %q, want %q", conversation[0].Content, "hello")
	}
	if conversation[1].Role != "assistant" || conversation[1].Content != "Received: hello [workspace:3]" {
		t.Fatalf("stored assistant message = %#v, want assistant reply", conversation[1])
	}
}

func TestHandleWebSocketSendMessageDeduplicatesIdempotentRetry(t *testing.T) {
	sessionManager := session.NewManager(nil)
	server := NewServer(log.New(io.Discard, "", 0), sessionManager, llm.NewMockClient())
	httpServer := httptest.NewServer(http.HandlerFunc(server.HandleWebSocket))
	t.Cleanup(httpServer.Close)

	wsURL := "ws" + strings.TrimPrefix(httpServer.URL, "http")
	conn, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
	if err != nil {
		t.Fatalf("dial websocket: %v", err)
	}
	t.Cleanup(func() { _ = conn.Close() })

	if err := conn.WriteJSON(protocolws.Message{
		Type:   protocolws.TypeRequest,
		ID:     "1",
		Method: protocolws.MethodConnect,
		Payload: map[string]any{
			"role":            "client",
			"client_identity": "myclaw-tui",
			"agent_id":        "main",
		},
	}); err != nil {
		t.Fatalf("write connect: %v", err)
	}
	var connectRes protocolws.Message
	if err := conn.ReadJSON(&connectRes); err != nil {
		t.Fatalf("read connect response: %v", err)
	}
	var hello protocolws.Message
	if err := conn.ReadJSON(&hello); err != nil {
		t.Fatalf("read hello event: %v", err)
	}
	sessionID, _ := connectRes.Payload["session_id"].(string)

	for _, id := range []string{"2", "3"} {
		if err := conn.WriteJSON(protocolws.Message{
			Type:   protocolws.TypeRequest,
			ID:     id,
			Method: protocolws.MethodSendMessage,
			Payload: map[string]any{
				"content":    "hello once",
				"request_id": "tui-2",
			},
		}); err != nil {
			t.Fatalf("write send %s: %v", id, err)
		}
		var ack protocolws.Message
		if err := conn.ReadJSON(&ack); err != nil {
			t.Fatalf("read ack %s: %v", id, err)
		}
		if ack.Type != protocolws.TypeResponse || !ack.OK || ack.ID != id {
			t.Fatalf("ack %s = %#v, want ok response with same id", id, ack)
		}
		if id == "2" {
			var created protocolws.Message
			if err := conn.ReadJSON(&created); err != nil {
				t.Fatalf("read created event: %v", err)
			}
			if created.Type != protocolws.TypeEvent || created.Event != protocolws.EventMessageCreated {
				t.Fatalf("created = %#v, want message.created", created)
			}
		}
	}

	messages, ok := sessionManager.Messages(sessionID)
	if !ok {
		t.Fatalf("messages for session %q not found", sessionID)
	}
	userMessages := 0
	for _, message := range messages {
		if message.Role == "user" && message.Content == "hello once" {
			userMessages++
		}
	}
	if userMessages != 1 {
		t.Fatalf("user message count = %d, want one idempotent send", userMessages)
	}
}

func TestHandleWebSocketToolLoop(t *testing.T) {
	sessionManager := session.NewManager(nil)
	server := NewServer(log.New(io.Discard, "", 0), sessionManager, llm.NewMockClient())
	httpServer := httptest.NewServer(http.HandlerFunc(server.HandleWebSocket))
	t.Cleanup(httpServer.Close)

	wsURL := "ws" + strings.TrimPrefix(httpServer.URL, "http")
	conn, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
	if err != nil {
		t.Fatalf("dial websocket: %v", err)
	}
	t.Cleanup(func() { _ = conn.Close() })

	if err := conn.WriteJSON(protocolws.Message{
		Type:   protocolws.TypeRequest,
		ID:     "1",
		Method: protocolws.MethodConnect,
		Payload: map[string]any{
			"role":            "client",
			"client_identity": "web-ui",
			"agent_id":        "main",
		},
	}); err != nil {
		t.Fatalf("write connect: %v", err)
	}

	var discard protocolws.Message
	_ = conn.ReadJSON(&discard)
	_ = conn.ReadJSON(&discard)

	if err := conn.WriteJSON(protocolws.Message{
		Type:   protocolws.TypeRequest,
		ID:     "2",
		Method: protocolws.MethodSendMessage,
		Payload: map[string]any{
			"content": "tool upper hello world",
		},
	}); err != nil {
		t.Fatalf("write inbound: %v", err)
	}

	events := make([]protocolws.Message, 0, 10)
	for len(events) < 20 {
		if err := conn.SetReadDeadline(time.Now().Add(2 * time.Second)); err != nil {
			t.Fatalf("set event read deadline: %v", err)
		}

		var msg protocolws.Message
		if err := conn.ReadJSON(&msg); err != nil {
			t.Fatalf("read event %d: %v", len(events), err)
		}
		events = append(events, msg)
		if msg.Type == protocolws.TypeEvent && msg.Event == "agent.lifecycle.end" {
			break
		}
	}

	foundToolCalled := false
	foundToolResult := false
	foundAssistantFinal := false
	for _, event := range events {
		if event.Type == protocolws.TypeEvent && event.Event == "tool.called" {
			foundToolCalled = true
		}
		if event.Type == protocolws.TypeEvent && event.Event == "tool.result" {
			foundToolResult = true
		}
		if event.Type == protocolws.TypeEvent && event.Event == protocolws.EventMessageCreated {
			if message, ok := event.Payload["message"].(map[string]any); ok {
				if message["role"] == "assistant" && message["content"] == "Using tool result: text.upper: HELLO WORLD" {
					foundAssistantFinal = true
				}
			}
		}
	}

	if !foundToolCalled {
		t.Fatal("expected tool.called event")
	}
	if !foundToolResult {
		t.Fatal("expected tool.result event")
	}
	if !foundAssistantFinal {
		t.Fatal("expected final assistant message from tool result")
	}
}

func TestHandleWebSocketEmitsToolProgressEvents(t *testing.T) {
	sessionManager := session.NewManager(nil)
	registry := tools.NewRegistry(progressToolForGateway{})
	server := NewServerWithOptions(log.New(io.Discard, "", 0), sessionManager, &progressToolCallClient{}, Options{
		PermissionPolicy: permissions.Policy{Mode: permissions.ModeDangerFullAccess},
	})
	server.runner = runtimepkg.NewRunnerWithOptions(sessionManager, &progressToolCallClient{}, workspace.NewLoader(""), registry, runtimepkg.Options{
		PermissionPolicy:   permissions.Policy{Mode: permissions.ModeDangerFullAccess},
		ReportToolProgress: server.reportToolProgress,
	})
	server.queue = runtimepkg.NewQueue(server.runner)

	httpServer := httptest.NewServer(http.HandlerFunc(server.HandleWebSocket))
	t.Cleanup(httpServer.Close)

	wsURL := "ws" + strings.TrimPrefix(httpServer.URL, "http")
	conn, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
	if err != nil {
		t.Fatalf("dial websocket: %v", err)
	}
	t.Cleanup(func() { _ = conn.Close() })

	if err := conn.WriteJSON(protocolws.Message{
		Type:   protocolws.TypeRequest,
		ID:     "1",
		Method: protocolws.MethodConnect,
		Payload: map[string]any{
			"role":            "client",
			"client_identity": "web-ui",
			"agent_id":        "main",
		},
	}); err != nil {
		t.Fatalf("write connect: %v", err)
	}
	_ = conn.ReadJSON(&protocolws.Message{})
	_ = conn.ReadJSON(&protocolws.Message{})

	if err := conn.WriteJSON(protocolws.Message{
		Type:   protocolws.TypeRequest,
		ID:     "2",
		Method: protocolws.MethodSendMessage,
		Payload: map[string]any{
			"content": "emit gateway progress",
		},
	}); err != nil {
		t.Fatalf("write send_message: %v", err)
	}

	foundProgress := false
	for i := 0; i < 24; i++ {

		var msg protocolws.Message
		if err := conn.ReadJSON(&msg); err != nil {
			t.Fatalf("read websocket event %d: %v", i, err)
		}
		if msg.Type == protocolws.TypeEvent && msg.Event == "tool.progress" {
			if got := msg.Payload["run_id"]; got == "" {
				t.Fatalf("tool.progress run_id = %#v, want populated run id", got)
			}
			if got := msg.Payload["session_id"]; got == "" {
				t.Fatalf("tool.progress session_id = %#v, want populated session id", got)
			}
			if got := msg.Payload["tool_name"]; got != "progress.echo" {
				continue
			}
			if got := msg.Payload["tool_use_id"]; got != "toolu-progress-gateway" {
				t.Fatalf("tool.progress tool_use_id = %#v, want toolu-progress-gateway", got)
			}
			if got := msg.Payload["type"]; got != "progress" {
				t.Fatalf("tool.progress type = %#v, want progress", got)
			}
			if got := msg.Payload["message"]; got != "halfway there" {
				t.Fatalf("tool.progress message = %#v, want halfway there", got)
			}
			data, _ := msg.Payload["data"].(map[string]any)
			if data["phase"] != "mid" {
				t.Fatalf("tool.progress data = %#v, want phase=mid", data)
			}
			foundProgress = true
			break
		}
	}
	if !foundProgress {
		t.Fatal("expected tool.progress websocket event")
	}
}

func TestHandleWebSocketDoesNotEmitRunErrorWhenApprovalIsRequired(t *testing.T) {
	sessionManager := session.NewManager(nil)
	server := NewServerWithOptions(log.New(io.Discard, "", 0), sessionManager, llm.NewMockClient(), Options{
		PermissionPolicy: permissions.Policy{Mode: permissions.ModeAsk},
	})
	httpServer := httptest.NewServer(http.HandlerFunc(server.HandleWebSocket))
	t.Cleanup(httpServer.Close)

	wsURL := "ws" + strings.TrimPrefix(httpServer.URL, "http")
	conn, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
	if err != nil {
		t.Fatalf("dial websocket: %v", err)
	}
	t.Cleanup(func() { _ = conn.Close() })

	if err := conn.WriteJSON(protocolws.Message{
		Type:   protocolws.TypeRequest,
		ID:     "1",
		Method: protocolws.MethodConnect,
		Payload: map[string]any{
			"role":            "client",
			"client_identity": "web-ui",
			"agent_id":        "main",
		},
	}); err != nil {
		t.Fatalf("write connect: %v", err)
	}
	var discard protocolws.Message
	_ = conn.ReadJSON(&discard)
	_ = conn.ReadJSON(&discard)

	if err := conn.WriteJSON(protocolws.Message{
		Type:   protocolws.TypeRequest,
		ID:     "2",
		Method: protocolws.MethodSendMessage,
		Payload: map[string]any{
			"content": "tool run pwd",
		},
	}); err != nil {
		t.Fatalf("write inbound: %v", err)
	}

	foundPermissionRequired := false
	if err := conn.SetReadDeadline(time.Now().Add(2 * time.Second)); err != nil {
		t.Fatalf("set read deadline: %v", err)
	}
	for i := 0; i < 6; i++ {
		var event protocolws.Message
		if err := conn.ReadJSON(&event); err != nil {
			if foundPermissionRequired {
				break
			}
			t.Fatalf("read event %d: %v", i, err)
		}
		if event.Type == protocolws.TypeEvent && event.Event == "permission.required" {
			if got := event.Payload["tool_name"]; got != "system.run" {
				t.Fatalf("permission.required tool_name = %#v, want system.run", got)
			}
			foundPermissionRequired = true
			continue
		}
		if event.Type == protocolws.TypeEvent && event.Event == "run.error" {
			t.Fatalf("unexpected run.error payload = %#v", event.Payload)
		}
	}

	if !foundPermissionRequired {
		t.Fatal("expected permission.required event")
	}
}

func TestHandleWebSocketPermissionHookAllowBypassesApprovalPrompt(t *testing.T) {
	sessionManager := session.NewManager(nil)
	var hookCalls int
	server := NewServerWithOptions(log.New(io.Discard, "", 0), sessionManager, llm.NewMockClient(), Options{
		PermissionPolicy: permissions.Policy{Mode: permissions.ModeAsk},
		PermissionHook: permissionHookFunc(func(_ context.Context, request queryengine.PermissionHookRequest) (permissions.Decision, bool, error) {
			hookCalls++
			if request.ToolName != "system.run" {
				t.Fatalf("permission hook tool = %q, want system.run", request.ToolName)
			}
			return permissions.Decision{
				Allowed: true,
				DecisionReason: permissions.DecisionReason{
					Type:     permissions.DecisionReasonHook,
					HookName: "PermissionRequest",
					Reason:   "allowed by gateway hook",
				},
			}, true, nil
		}),
	})
	httpServer := httptest.NewServer(http.HandlerFunc(server.HandleWebSocket))
	t.Cleanup(httpServer.Close)

	wsURL := "ws" + strings.TrimPrefix(httpServer.URL, "http")
	conn, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
	if err != nil {
		t.Fatalf("dial websocket: %v", err)
	}
	t.Cleanup(func() { _ = conn.Close() })

	if err := conn.WriteJSON(protocolws.Message{
		Type:   protocolws.TypeRequest,
		ID:     "1",
		Method: protocolws.MethodConnect,
		Payload: map[string]any{
			"role":            "client",
			"client_identity": "web-ui",
			"agent_id":        "main",
		},
	}); err != nil {
		t.Fatalf("write connect: %v", err)
	}
	var discard protocolws.Message
	_ = conn.ReadJSON(&discard)
	_ = conn.ReadJSON(&discard)

	if err := conn.WriteJSON(protocolws.Message{
		Type:   protocolws.TypeRequest,
		ID:     "2",
		Method: protocolws.MethodSendMessage,
		Payload: map[string]any{
			"content": "tool run pwd",
		},
	}); err != nil {
		t.Fatalf("write inbound: %v", err)
	}

	foundToolCalled := false
	if err := conn.SetReadDeadline(time.Now().Add(2 * time.Second)); err != nil {
		t.Fatalf("set read deadline: %v", err)
	}
	for i := 0; i < 12; i++ {
		var event protocolws.Message
		if err := conn.ReadJSON(&event); err != nil {
			if foundToolCalled {
				break
			}
			t.Fatalf("read event %d: %v", i, err)
		}
		if event.Type == protocolws.TypeEvent && event.Event == "permission.required" {
			t.Fatalf("unexpected permission.required payload = %#v", event.Payload)
		}
		if event.Type == protocolws.TypeEvent && event.Event == "tool.called" {
			foundToolCalled = true
			if got := event.Payload["tool_name"]; got != "system.run" {
				t.Fatalf("tool.called tool_name = %#v, want system.run", got)
			}
			break
		}
		if event.Type == protocolws.TypeEvent && event.Event == "run.error" {
			t.Fatalf("unexpected run.error payload = %#v", event.Payload)
		}
	}
	if err := conn.SetReadDeadline(time.Time{}); err != nil {
		t.Fatalf("clear read deadline: %v", err)
	}

	if hookCalls != 1 {
		t.Fatalf("hook calls = %d, want one PermissionRequest hook call", hookCalls)
	}
	if !foundToolCalled {
		t.Fatal("expected hook-allowed tool.called event")
	}
}

func TestHandleWebSocketPermissionHookDenyContinuesWithErrorToolResult(t *testing.T) {
	sessionManager := session.NewManager(nil)
	server := NewServerWithOptions(log.New(io.Discard, "", 0), sessionManager, llm.NewMockClient(), Options{
		PermissionPolicy: permissions.Policy{Mode: permissions.ModeAsk},
		PermissionHook: permissionHookFunc(func(_ context.Context, request queryengine.PermissionHookRequest) (permissions.Decision, bool, error) {
			return permissions.Decision{
				Reason: "denied by gateway hook",
				DecisionReason: permissions.DecisionReason{
					Type:     permissions.DecisionReasonHook,
					HookName: "PermissionRequest",
					Reason:   "denied by gateway hook",
				},
			}, true, nil
		}),
	})
	httpServer := httptest.NewServer(http.HandlerFunc(server.HandleWebSocket))
	t.Cleanup(httpServer.Close)

	wsURL := "ws" + strings.TrimPrefix(httpServer.URL, "http")
	conn, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
	if err != nil {
		t.Fatalf("dial websocket: %v", err)
	}
	t.Cleanup(func() { _ = conn.Close() })

	if err := conn.WriteJSON(protocolws.Message{
		Type:   protocolws.TypeRequest,
		ID:     "1",
		Method: protocolws.MethodConnect,
		Payload: map[string]any{
			"role":            "client",
			"client_identity": "web-ui",
			"agent_id":        "main",
		},
	}); err != nil {
		t.Fatalf("write connect: %v", err)
	}
	var discard protocolws.Message
	_ = conn.ReadJSON(&discard)
	_ = conn.ReadJSON(&discard)

	if err := conn.WriteJSON(protocolws.Message{
		Type:   protocolws.TypeRequest,
		ID:     "2",
		Method: protocolws.MethodSendMessage,
		Payload: map[string]any{
			"content": "tool run pwd",
		},
	}); err != nil {
		t.Fatalf("write inbound: %v", err)
	}

	if err := conn.SetReadDeadline(time.Now().Add(2 * time.Second)); err != nil {
		t.Fatalf("set read deadline: %v", err)
	}
	defer func() { _ = conn.SetReadDeadline(time.Time{}) }()
	foundToolResult := false
	foundAssistant := false
	for i := 0; i < 24; i++ {
		var event protocolws.Message
		if err := conn.ReadJSON(&event); err != nil {
			if foundToolResult && foundAssistant {
				break
			}
			t.Fatalf("read event %d: %v", i, err)
		}
		if event.Type == protocolws.TypeEvent && event.Event == "permission.required" {
			t.Fatalf("unexpected permission.required payload = %#v", event.Payload)
		}
		if event.Type == protocolws.TypeEvent && event.Event == "tool.called" {
			t.Fatalf("unexpected tool.called payload = %#v", event.Payload)
		}
		if event.Type == protocolws.TypeEvent && event.Event == "run.error" {
			t.Fatalf("unexpected run.error payload = %#v", event.Payload)
		}
		if event.Type == protocolws.TypeEvent && event.Event == "tool.result" {
			foundToolResult = true
			message, _ := event.Payload["message"].(map[string]any)
			content, _ := message["content"].(string)
			if !strings.Contains(content, "denied by gateway hook") {
				t.Fatalf("tool.result payload = %#v, want hook denial reason", event.Payload)
			}
		}
		if event.Type == protocolws.TypeEvent && event.Event == protocolws.EventMessageCreated {
			message, _ := event.Payload["message"].(map[string]any)
			if message["role"] == "assistant" {
				foundAssistant = true
			}
		}
		if foundToolResult && foundAssistant {
			break
		}
	}
	if !foundToolResult || !foundAssistant {
		t.Fatalf("found tool.result=%v assistant=%v, want both after hook deny", foundToolResult, foundAssistant)
	}
}

func TestHandleWebSocketPermissionRequiredIncludesPromptMetadata(t *testing.T) {
	sessionManager := session.NewManager(nil)
	server := NewServerWithOptions(log.New(io.Discard, "", 0), sessionManager, llm.NewMockClient(), Options{
		PermissionPolicy: permissions.Policy{Mode: permissions.ModeAsk},
		PermissionHook: permissionHookFunc(func(_ context.Context, request queryengine.PermissionHookRequest) (permissions.Decision, bool, error) {
			return permissions.Decision{
				RequiresApproval: true,
				Reason:           "review rich prompt metadata",
				AcceptFeedback:   "Provide changes before approval",
				ContentBlocks: []map[string]any{{
					"type": "text",
					"text": "metadata block",
				}},
				DecisionReason: permissions.DecisionReason{
					Type:     permissions.DecisionReasonHook,
					HookName: "PermissionRequest",
					Reason:   "review rich prompt metadata",
				},
			}, true, nil
		}),
	})
	httpServer := httptest.NewServer(http.HandlerFunc(server.HandleWebSocket))
	t.Cleanup(httpServer.Close)

	wsURL := "ws" + strings.TrimPrefix(httpServer.URL, "http")
	conn, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
	if err != nil {
		t.Fatalf("dial websocket: %v", err)
	}
	t.Cleanup(func() { _ = conn.Close() })

	if err := conn.WriteJSON(protocolws.Message{
		Type:   protocolws.TypeRequest,
		ID:     "1",
		Method: protocolws.MethodConnect,
		Payload: map[string]any{
			"role":            "client",
			"client_identity": "web-ui",
			"agent_id":        "main",
		},
	}); err != nil {
		t.Fatalf("write connect: %v", err)
	}
	var discard protocolws.Message
	_ = conn.ReadJSON(&discard)
	_ = conn.ReadJSON(&discard)

	if err := conn.WriteJSON(protocolws.Message{
		Type:   protocolws.TypeRequest,
		ID:     "2",
		Method: protocolws.MethodSendMessage,
		Payload: map[string]any{
			"content": "tool run pwd",
		},
	}); err != nil {
		t.Fatalf("write inbound: %v", err)
	}

	if err := conn.SetReadDeadline(time.Now().Add(2 * time.Second)); err != nil {
		t.Fatalf("set read deadline: %v", err)
	}
	defer func() { _ = conn.SetReadDeadline(time.Time{}) }()
	for i := 0; i < 12; i++ {
		var event protocolws.Message
		if err := conn.ReadJSON(&event); err != nil {
			t.Fatalf("read event %d: %v", i, err)
		}
		if event.Type != protocolws.TypeEvent || event.Event != "permission.required" {
			continue
		}
		if got := event.Payload["accept_feedback"]; got != "Provide changes before approval" {
			t.Fatalf("accept_feedback = %#v, want hook feedback prompt", got)
		}
		blocks, ok := event.Payload["content_blocks"].([]any)
		if !ok || len(blocks) != 1 {
			t.Fatalf("content_blocks = %#v, want one block", event.Payload["content_blocks"])
		}
		return
	}
	t.Fatal("expected permission.required")
}

func TestHandleWebSocketApprovalListIncludesPromptMetadata(t *testing.T) {
	sessionManager := session.NewManager(nil)
	server := NewServerWithOptions(log.New(io.Discard, "", 0), sessionManager, llm.NewMockClient(), Options{
		PermissionPolicy: permissions.Policy{Mode: permissions.ModeAsk},
		PermissionHook: permissionHookFunc(func(_ context.Context, request queryengine.PermissionHookRequest) (permissions.Decision, bool, error) {
			return permissions.Decision{
				RequiresApproval: true,
				Reason:           "review metadata",
				AcceptFeedback:   "Explain why this command is needed",
				ContentBlocks: []map[string]any{{
					"type": "text",
					"text": "approval list block",
				}},
				DecisionReason: permissions.DecisionReason{
					Type:     permissions.DecisionReasonHook,
					HookName: "PermissionRequest",
					Reason:   "review metadata",
				},
			}, true, nil
		}),
	})
	httpServer := httptest.NewServer(http.HandlerFunc(server.HandleWebSocket))
	t.Cleanup(httpServer.Close)

	wsURL := "ws" + strings.TrimPrefix(httpServer.URL, "http")
	conn, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
	if err != nil {
		t.Fatalf("dial websocket: %v", err)
	}
	t.Cleanup(func() { _ = conn.Close() })

	if err := conn.WriteJSON(protocolws.Message{
		Type:   protocolws.TypeRequest,
		ID:     "1",
		Method: protocolws.MethodConnect,
		Payload: map[string]any{
			"role":            "client",
			"client_identity": "web-ui",
			"agent_id":        "main",
		},
	}); err != nil {
		t.Fatalf("write connect: %v", err)
	}
	var discard protocolws.Message
	_ = conn.ReadJSON(&discard)
	_ = conn.ReadJSON(&discard)

	if err := conn.WriteJSON(protocolws.Message{
		Type:   protocolws.TypeRequest,
		ID:     "2",
		Method: protocolws.MethodSendMessage,
		Payload: map[string]any{
			"content": "tool run pwd",
		},
	}); err != nil {
		t.Fatalf("write inbound: %v", err)
	}
	_ = waitForPermissionRequired(t, conn)

	if err := conn.WriteJSON(protocolws.Message{
		Type:   protocolws.TypeRequest,
		ID:     "3",
		Method: protocolws.MethodApprovalList,
		Payload: map[string]any{
			"status": "pending",
		},
	}); err != nil {
		t.Fatalf("write approval_list: %v", err)
	}
	var res protocolws.Message
	if err := conn.ReadJSON(&res); err != nil {
		t.Fatalf("read approval_list: %v", err)
	}
	approvals, _ := res.Payload["approvals"].([]any)
	if len(approvals) != 1 {
		t.Fatalf("approvals = %#v, want one approval", res.Payload["approvals"])
	}
	item, _ := approvals[0].(map[string]any)
	if got := item["accept_feedback"]; got != "Explain why this command is needed" {
		t.Fatalf("approval accept_feedback = %#v, want metadata", got)
	}
	blocks, ok := item["content_blocks"].([]any)
	if !ok || len(blocks) != 1 {
		t.Fatalf("approval content_blocks = %#v, want one block", item["content_blocks"])
	}
}

func TestHandleWebSocketApprovalApproveCarriesFeedbackBlocksIntoToolResult(t *testing.T) {
	sessionManager := session.NewManager(nil)
	server := NewServerWithOptions(log.New(io.Discard, "", 0), sessionManager, llm.NewMockClient(), Options{
		PermissionPolicy: permissions.Policy{Mode: permissions.ModeAsk},
	})
	httpServer := httptest.NewServer(http.HandlerFunc(server.HandleWebSocket))
	t.Cleanup(httpServer.Close)

	wsURL := "ws" + strings.TrimPrefix(httpServer.URL, "http")
	conn, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
	if err != nil {
		t.Fatalf("dial websocket: %v", err)
	}
	t.Cleanup(func() { _ = conn.Close() })

	if err := conn.WriteJSON(protocolws.Message{
		Type:   protocolws.TypeRequest,
		ID:     "1",
		Method: protocolws.MethodConnect,
		Payload: map[string]any{
			"role":            "client",
			"client_identity": "web-ui",
			"agent_id":        "main",
		},
	}); err != nil {
		t.Fatalf("write connect: %v", err)
	}
	var connectResponse protocolws.Message
	if err := conn.ReadJSON(&connectResponse); err != nil {
		t.Fatalf("read connect response: %v", err)
	}
	var discard protocolws.Message
	_ = conn.ReadJSON(&discard)

	sessionID, _ := connectResponse.Payload["session_id"].(string)
	if sessionID == "" {
		t.Fatalf("connect response payload = %#v, want session_id", connectResponse.Payload)
	}

	if err := conn.WriteJSON(protocolws.Message{
		Type:   protocolws.TypeRequest,
		ID:     "2",
		Method: protocolws.MethodSendMessage,
		Payload: map[string]any{
			"content": "tool run pwd",
		},
	}); err != nil {
		t.Fatalf("write inbound: %v", err)
	}
	approvalID := waitForPermissionRequired(t, conn)
	if approvalID == "" {
		t.Fatal("permission.required event did not include approval_id")
	}

	if err := conn.WriteJSON(protocolws.Message{
		Type:   protocolws.TypeRequest,
		ID:     "3",
		Method: protocolws.MethodApprovalApprove,
		Payload: map[string]any{
			"approval_id":     approvalID,
			"accept_feedback": "approved with UI note",
			"content_blocks": []map[string]any{{
				"type": "text",
				"text": "extra UI block",
			}},
		},
	}); err != nil {
		t.Fatalf("write approval approve: %v", err)
	}
	waitForEvent(t, conn, "tool.result")

	messages, ok := sessionManager.Messages(sessionID)
	if !ok {
		t.Fatalf("messages for session %q not found", sessionID)
	}
	var toolMessage session.Message
	for _, message := range messages {
		if message.Role == "tool" {
			toolMessage = message
			break
		}
	}
	if toolMessage.ID == "" {
		t.Fatalf("messages = %#v, want tool message", messages)
	}
	if len(toolMessage.Blocks) != 3 {
		t.Fatalf("tool message blocks = %#v, want tool result plus approval feedback blocks", toolMessage.Blocks)
	}
	if toolMessage.Blocks[1].Text != "approved with UI note" || toolMessage.Blocks[2].Text != "extra UI block" {
		t.Fatalf("tool message blocks = %#v, want approval feedback blocks", toolMessage.Blocks)
	}
}

func TestHandleWebSocketApprovalDecisionDeduplicatesIdempotentRetry(t *testing.T) {
	sessionManager := session.NewManager(nil)
	server := NewServerWithOptions(log.New(io.Discard, "", 0), sessionManager, llm.NewMockClient(), Options{
		PermissionPolicy: permissions.Policy{Mode: permissions.ModeAsk},
	})
	httpServer := httptest.NewServer(http.HandlerFunc(server.HandleWebSocket))
	t.Cleanup(httpServer.Close)

	wsURL := "ws" + strings.TrimPrefix(httpServer.URL, "http")
	conn, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
	if err != nil {
		t.Fatalf("dial websocket: %v", err)
	}
	t.Cleanup(func() { _ = conn.Close() })

	if err := conn.WriteJSON(protocolws.Message{
		Type:   protocolws.TypeRequest,
		ID:     "1",
		Method: protocolws.MethodConnect,
		Payload: map[string]any{
			"role":            "client",
			"client_identity": "myclaw-tui",
			"agent_id":        "main",
		},
	}); err != nil {
		t.Fatalf("write connect: %v", err)
	}
	var connectResponse protocolws.Message
	if err := conn.ReadJSON(&connectResponse); err != nil {
		t.Fatalf("read connect response: %v", err)
	}
	var hello protocolws.Message
	if err := conn.ReadJSON(&hello); err != nil {
		t.Fatalf("read hello event: %v", err)
	}
	sessionID, _ := connectResponse.Payload["session_id"].(string)

	if err := conn.WriteJSON(protocolws.Message{
		Type:   protocolws.TypeRequest,
		ID:     "2",
		Method: protocolws.MethodSendMessage,
		Payload: map[string]any{
			"content": "tool run pwd",
		},
	}); err != nil {
		t.Fatalf("write inbound: %v", err)
	}
	approvalID := waitForPermissionRequired(t, conn)
	if approvalID == "" {
		t.Fatal("permission.required event did not include approval_id")
	}

	decisionPayload := map[string]any{
		"approval_id": approvalID,
		"request_id":  "tui-approve-1",
	}
	if err := conn.WriteJSON(protocolws.Message{
		Type:    protocolws.TypeRequest,
		ID:      "3",
		Method:  protocolws.MethodApprovalApprove,
		Payload: decisionPayload,
	}); err != nil {
		t.Fatalf("write approval approve: %v", err)
	}
	waitForResponseID(t, conn, "3")
	waitForEvent(t, conn, "tool.result")

	if err := conn.WriteJSON(protocolws.Message{
		Type:    protocolws.TypeRequest,
		ID:      "4",
		Method:  protocolws.MethodApprovalApprove,
		Payload: decisionPayload,
	}); err != nil {
		t.Fatalf("write duplicate approval approve: %v", err)
	}
	waitForResponseID(t, conn, "4")
	time.Sleep(100 * time.Millisecond)

	messages, ok := sessionManager.Messages(sessionID)
	if !ok {
		t.Fatalf("messages for session %q not found", sessionID)
	}
	toolMessages := 0
	for _, message := range messages {
		if message.Role == "tool" {
			toolMessages++
		}
	}
	if toolMessages != 1 {
		t.Fatalf("tool message count = %d, want one idempotent approval continuation", toolMessages)
	}
}

func TestHandleWebSocketCanUseToolControlRequestAllowRunsTool(t *testing.T) {
	sessionManager := session.NewManager(nil)
	server := NewServerWithOptions(log.New(io.Discard, "", 0), sessionManager, llm.NewMockClient(), Options{
		PermissionPolicy: permissions.Policy{Mode: permissions.ModeAsk},
	})
	httpServer := httptest.NewServer(http.HandlerFunc(server.HandleWebSocket))
	t.Cleanup(httpServer.Close)

	wsURL := "ws" + strings.TrimPrefix(httpServer.URL, "http")
	conn, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
	if err != nil {
		t.Fatalf("dial websocket: %v", err)
	}
	t.Cleanup(func() { _ = conn.Close() })

	if err := conn.WriteJSON(protocolws.Message{
		Type:   protocolws.TypeRequest,
		ID:     "1",
		Method: protocolws.MethodConnect,
		Payload: map[string]any{
			"role":                        "sdk",
			"client_identity":             "sdk-host",
			"agent_id":                    "main",
			"supports_permission_control": true,
		},
	}); err != nil {
		t.Fatalf("write connect: %v", err)
	}
	var discard protocolws.Message
	_ = conn.ReadJSON(&discard)
	_ = conn.ReadJSON(&discard)

	if err := conn.WriteJSON(protocolws.Message{
		Type:   protocolws.TypeRequest,
		ID:     "2",
		Method: protocolws.MethodSendMessage,
		Payload: map[string]any{
			"content": "tool run pwd",
		},
	}); err != nil {
		t.Fatalf("write inbound: %v", err)
	}

	var control protocolws.Message
	if err := conn.SetReadDeadline(time.Now().Add(2 * time.Second)); err != nil {
		t.Fatalf("set read deadline: %v", err)
	}
	for i := 0; i < 12; i++ {
		if err := conn.ReadJSON(&control); err != nil {
			t.Fatalf("read control request %d: %v", i, err)
		}
		if control.Type == protocolws.TypeControlRequest {
			break
		}
		if control.Type == protocolws.TypeEvent && control.Event == "permission.required" {
			t.Fatalf("unexpected permission.required before can_use_tool: %#v", control.Payload)
		}
		if control.Type == protocolws.TypeEvent && control.Event == "run.error" {
			t.Fatalf("unexpected run.error before can_use_tool: %#v", control.Payload)
		}
	}
	if control.Type != protocolws.TypeControlRequest {
		t.Fatalf("message = %#v, want control_request", control)
	}
	request, _ := control.Payload["request"].(map[string]any)
	if got := request["subtype"]; got != "can_use_tool" {
		t.Fatalf("control request subtype = %#v, want can_use_tool", got)
	}
	if got := request["tool_name"]; got != "system.run" {
		t.Fatalf("control request tool_name = %#v, want system.run", got)
	}
	if got := request["input"]; got != "pwd" {
		t.Fatalf("control request input = %#v, want pwd", got)
	}

	if err := conn.WriteJSON(protocolws.Message{
		Type: protocolws.TypeControlResponse,
		ID:   control.ID,
		Payload: map[string]any{
			"behavior": "allow",
		},
	}); err != nil {
		t.Fatalf("write control response: %v", err)
	}

	foundToolCalled := false
	for i := 0; i < 12; i++ {
		var event protocolws.Message
		if err := conn.ReadJSON(&event); err != nil {
			if foundToolCalled {
				break
			}
			t.Fatalf("read event %d: %v", i, err)
		}
		if event.Type == protocolws.TypeEvent && event.Event == "tool.called" {
			foundToolCalled = true
			if got := event.Payload["tool_name"]; got != "system.run" {
				t.Fatalf("tool.called tool_name = %#v, want system.run", got)
			}
			break
		}
		if event.Type == protocolws.TypeEvent && event.Event == "permission.required" {
			t.Fatalf("unexpected permission.required after can_use_tool allow: %#v", event.Payload)
		}
		if event.Type == protocolws.TypeEvent && event.Event == "run.error" {
			t.Fatalf("unexpected run.error after can_use_tool allow: %#v", event.Payload)
		}
	}
	if err := conn.SetReadDeadline(time.Time{}); err != nil {
		t.Fatalf("clear read deadline: %v", err)
	}
	if !foundToolCalled {
		t.Fatal("expected can_use_tool allow to continue to tool.called")
	}
}

func TestHandleWebSocketCanUseToolControlRequestIncludesStructuredDecisionReason(t *testing.T) {
	sessionManager := session.NewManager(nil)
	server := NewServerWithOptions(log.New(io.Discard, "", 0), sessionManager, llm.NewMockClient(), Options{
		PermissionPolicy: permissions.Policy{
			Mode: permissions.ModeAsk,
			Rules: []permissions.Rule{{
				ToolName: "system.run",
				Action:   permissions.ActionAsk,
				Source:   string(permissions.RuleSourceProject),
			}},
		},
	})
	httpServer := httptest.NewServer(http.HandlerFunc(server.HandleWebSocket))
	t.Cleanup(httpServer.Close)

	wsURL := "ws" + strings.TrimPrefix(httpServer.URL, "http")
	conn, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
	if err != nil {
		t.Fatalf("dial websocket: %v", err)
	}
	t.Cleanup(func() { _ = conn.Close() })

	if err := conn.WriteJSON(protocolws.Message{
		Type:   protocolws.TypeRequest,
		ID:     "1",
		Method: protocolws.MethodConnect,
		Payload: map[string]any{
			"role":                        "sdk",
			"client_identity":             "sdk-host",
			"agent_id":                    "main",
			"supports_permission_control": true,
		},
	}); err != nil {
		t.Fatalf("write connect: %v", err)
	}
	var discard protocolws.Message
	_ = conn.ReadJSON(&discard)
	_ = conn.ReadJSON(&discard)

	if err := conn.WriteJSON(protocolws.Message{
		Type:   protocolws.TypeRequest,
		ID:     "2",
		Method: protocolws.MethodSendMessage,
		Payload: map[string]any{
			"content": "tool run pwd",
		},
	}); err != nil {
		t.Fatalf("write inbound: %v", err)
	}

	control := readCanUseToolControlRequest(t, conn)
	request, _ := control.Payload["request"].(map[string]any)
	details, _ := request["decision_reason_details"].(map[string]any)
	if details["type"] != "rule" {
		t.Fatalf("decision_reason_details = %#v, want rule object", details)
	}
	if request["decision_reason"] != nil {
		t.Fatalf("decision_reason = %#v, want nil for rule reason per Claude structuredIO serialization", request["decision_reason"])
	}

	if err := conn.WriteJSON(protocolws.Message{
		Type: protocolws.TypeControlResponse,
		ID:   control.ID,
		Payload: map[string]any{
			"behavior": "deny",
			"message":  "stop after reason assertion",
		},
	}); err != nil {
		t.Fatalf("write control response: %v", err)
	}
}

func TestHandleWebSocketPermissionRequiredIncludesStructuredDecisionReason(t *testing.T) {
	sessionManager := session.NewManager(nil)
	server := NewServerWithOptions(log.New(io.Discard, "", 0), sessionManager, llm.NewMockClient(), Options{
		PermissionPolicy: permissions.Policy{
			Mode: permissions.ModeAsk,
			Rules: []permissions.Rule{{
				ToolName: "system.run",
				Action:   permissions.ActionAsk,
				Source:   string(permissions.RuleSourceProject),
			}},
		},
	})
	httpServer := httptest.NewServer(http.HandlerFunc(server.HandleWebSocket))
	t.Cleanup(httpServer.Close)

	wsURL := "ws" + strings.TrimPrefix(httpServer.URL, "http")
	conn, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
	if err != nil {
		t.Fatalf("dial websocket: %v", err)
	}
	t.Cleanup(func() { _ = conn.Close() })

	if err := conn.WriteJSON(protocolws.Message{
		Type:   protocolws.TypeRequest,
		ID:     "1",
		Method: protocolws.MethodConnect,
		Payload: map[string]any{
			"role":            "client",
			"client_identity": "web-ui",
			"agent_id":        "main",
		},
	}); err != nil {
		t.Fatalf("write connect: %v", err)
	}
	var discard protocolws.Message
	_ = conn.ReadJSON(&discard)
	_ = conn.ReadJSON(&discard)

	if err := conn.WriteJSON(protocolws.Message{
		Type:   protocolws.TypeRequest,
		ID:     "2",
		Method: protocolws.MethodSendMessage,
		Payload: map[string]any{
			"content": "tool run pwd",
		},
	}); err != nil {
		t.Fatalf("write inbound: %v", err)
	}

	if err := conn.SetReadDeadline(time.Now().Add(2 * time.Second)); err != nil {
		t.Fatalf("set read deadline: %v", err)
	}
	defer func() { _ = conn.SetReadDeadline(time.Time{}) }()
	for i := 0; i < 12; i++ {
		var event protocolws.Message
		if err := conn.ReadJSON(&event); err != nil {
			t.Fatalf("read event %d: %v", i, err)
		}
		if event.Type == protocolws.TypeEvent && event.Event == "permission.required" {
			details, _ := event.Payload["decision_reason_details"].(map[string]any)
			if details["type"] != "rule" {
				t.Fatalf("decision_reason_details = %#v, want rule object", details)
			}
			if got := event.Payload["decision_reason"]; got != nil {
				t.Fatalf("decision_reason = %#v, want omitted string for rule reason", got)
			}
			return
		}
	}
	t.Fatal("expected permission.required")
}

func TestHandleWebSocketCanUseToolControlResponseDenyContinuesWithErrorToolResult(t *testing.T) {
	sessionManager := session.NewManager(nil)
	server := NewServerWithOptions(log.New(io.Discard, "", 0), sessionManager, llm.NewMockClient(), Options{
		PermissionPolicy: permissions.Policy{Mode: permissions.ModeAsk},
	})
	httpServer := httptest.NewServer(http.HandlerFunc(server.HandleWebSocket))
	t.Cleanup(httpServer.Close)

	wsURL := "ws" + strings.TrimPrefix(httpServer.URL, "http")
	conn, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
	if err != nil {
		t.Fatalf("dial websocket: %v", err)
	}
	t.Cleanup(func() { _ = conn.Close() })

	if err := conn.WriteJSON(protocolws.Message{
		Type:   protocolws.TypeRequest,
		ID:     "1",
		Method: protocolws.MethodConnect,
		Payload: map[string]any{
			"role":                        "sdk",
			"client_identity":             "sdk-host",
			"agent_id":                    "main",
			"supports_permission_control": true,
		},
	}); err != nil {
		t.Fatalf("write connect: %v", err)
	}
	var discard protocolws.Message
	_ = conn.ReadJSON(&discard)
	_ = conn.ReadJSON(&discard)

	if err := conn.WriteJSON(protocolws.Message{
		Type:   protocolws.TypeRequest,
		ID:     "2",
		Method: protocolws.MethodSendMessage,
		Payload: map[string]any{
			"content": "tool run pwd",
		},
	}); err != nil {
		t.Fatalf("write inbound: %v", err)
	}

	control := readCanUseToolControlRequest(t, conn)
	if err := conn.WriteJSON(protocolws.Message{
		Type: protocolws.TypeControlResponse,
		ID:   control.ID,
		Payload: map[string]any{
			"behavior": "deny",
			"message":  "denied by sdk host",
		},
	}); err != nil {
		t.Fatalf("write control response: %v", err)
	}

	foundToolResult := false
	foundAssistant := false
	if err := conn.SetReadDeadline(time.Now().Add(2 * time.Second)); err != nil {
		t.Fatalf("set deny continuation read deadline: %v", err)
	}
	defer func() { _ = conn.SetReadDeadline(time.Time{}) }()
	for i := 0; i < 24; i++ {
		var event protocolws.Message
		if err := conn.ReadJSON(&event); err != nil {
			if foundToolResult && foundAssistant {
				break
			}
			t.Fatalf("read event %d: %v", i, err)
		}
		if event.Type == protocolws.TypeEvent && event.Event == "permission.required" {
			t.Fatalf("unexpected permission.required after can_use_tool deny: %#v", event.Payload)
		}
		if event.Type == protocolws.TypeEvent && event.Event == "tool.called" {
			t.Fatalf("unexpected tool.called after can_use_tool deny: %#v", event.Payload)
		}
		if event.Type == protocolws.TypeEvent && event.Event == "run.error" {
			t.Fatalf("unexpected run.error after can_use_tool deny: %#v", event.Payload)
		}
		if event.Type == protocolws.TypeEvent && event.Event == "tool.result" {
			foundToolResult = true
			message, _ := event.Payload["message"].(map[string]any)
			content, _ := message["content"].(string)
			if !strings.Contains(content, "denied by sdk host") {
				t.Fatalf("tool.result payload = %#v, want sdk denial reason", event.Payload)
			}
		}
		if event.Type == protocolws.TypeEvent && event.Event == protocolws.EventMessageCreated {
			message, _ := event.Payload["message"].(map[string]any)
			if message["role"] == "assistant" {
				foundAssistant = true
			}
		}
		if foundToolResult && foundAssistant {
			break
		}
	}
	if !foundToolResult || !foundAssistant {
		t.Fatalf("found tool.result=%v assistant=%v, want both after can_use_tool deny", foundToolResult, foundAssistant)
	}
}

func TestHandleWebSocketCanUseToolControlResponseUpdatedInputDrivesToolCall(t *testing.T) {
	sessionManager := session.NewManager(nil)
	server := NewServerWithOptions(log.New(io.Discard, "", 0), sessionManager, llm.NewMockClient(), Options{
		PermissionPolicy: permissions.Policy{Mode: permissions.ModeAsk},
	})
	httpServer := httptest.NewServer(http.HandlerFunc(server.HandleWebSocket))
	t.Cleanup(httpServer.Close)

	wsURL := "ws" + strings.TrimPrefix(httpServer.URL, "http")
	conn, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
	if err != nil {
		t.Fatalf("dial websocket: %v", err)
	}
	t.Cleanup(func() { _ = conn.Close() })

	if err := conn.WriteJSON(protocolws.Message{
		Type:   protocolws.TypeRequest,
		ID:     "1",
		Method: protocolws.MethodConnect,
		Payload: map[string]any{
			"role":                        "sdk",
			"client_identity":             "sdk-host",
			"agent_id":                    "main",
			"supports_permission_control": true,
		},
	}); err != nil {
		t.Fatalf("write connect: %v", err)
	}
	var discard protocolws.Message
	_ = conn.ReadJSON(&discard)
	_ = conn.ReadJSON(&discard)

	if err := conn.WriteJSON(protocolws.Message{
		Type:   protocolws.TypeRequest,
		ID:     "2",
		Method: protocolws.MethodSendMessage,
		Payload: map[string]any{
			"content": "tool run pwd",
		},
	}); err != nil {
		t.Fatalf("write inbound: %v", err)
	}

	control := readCanUseToolControlRequest(t, conn)
	if err := conn.WriteJSON(protocolws.Message{
		Type: protocolws.TypeControlResponse,
		ID:   control.ID,
		Payload: map[string]any{
			"behavior":      "allow",
			"updated_input": "echo sdk-host",
		},
	}); err != nil {
		t.Fatalf("write control response: %v", err)
	}

	foundToolCalled := false
	for i := 0; i < 12; i++ {
		var event protocolws.Message
		if err := conn.ReadJSON(&event); err != nil {
			if foundToolCalled {
				break
			}
			t.Fatalf("read event %d: %v", i, err)
		}
		if event.Type == protocolws.TypeEvent && event.Event == "tool.called" {
			foundToolCalled = true
			if got := event.Payload["tool_input"]; got != "echo sdk-host" {
				t.Fatalf("tool.called tool_input = %#v, want updated input", got)
			}
			break
		}
		if event.Type == protocolws.TypeEvent && event.Event == "permission.required" {
			t.Fatalf("unexpected permission.required after can_use_tool allow: %#v", event.Payload)
		}
		if event.Type == protocolws.TypeEvent && event.Event == "run.error" {
			t.Fatalf("unexpected run.error after can_use_tool allow: %#v", event.Payload)
		}
	}
	if !foundToolCalled {
		t.Fatal("expected updated can_use_tool input to continue to tool.called")
	}
}

func TestHandleWebSocketCanUseToolControlRequestCarriesObjectNativeInput(t *testing.T) {
	sessionManager := session.NewManager(nil)
	server := NewServerWithOptions(log.New(io.Discard, "", 0), sessionManager, &objectToolCallClient{}, Options{
		PermissionPolicy: permissions.Policy{Mode: permissions.ModeAsk},
	})
	httpServer := httptest.NewServer(http.HandlerFunc(server.HandleWebSocket))
	t.Cleanup(httpServer.Close)

	wsURL := "ws" + strings.TrimPrefix(httpServer.URL, "http")
	conn, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
	if err != nil {
		t.Fatalf("dial websocket: %v", err)
	}
	t.Cleanup(func() { _ = conn.Close() })

	if err := conn.WriteJSON(protocolws.Message{
		Type:   protocolws.TypeRequest,
		ID:     "1",
		Method: protocolws.MethodConnect,
		Payload: map[string]any{
			"role":                        "sdk",
			"client_identity":             "sdk-host",
			"agent_id":                    "main",
			"supports_permission_control": true,
		},
	}); err != nil {
		t.Fatalf("write connect: %v", err)
	}
	var discard protocolws.Message
	_ = conn.ReadJSON(&discard)
	_ = conn.ReadJSON(&discard)

	if err := conn.WriteJSON(protocolws.Message{
		Type:   protocolws.TypeRequest,
		ID:     "2",
		Method: protocolws.MethodSendMessage,
		Payload: map[string]any{
			"content": "emit object tool input",
		},
	}); err != nil {
		t.Fatalf("write inbound: %v", err)
	}

	control := readCanUseToolControlRequest(t, conn)
	request, _ := control.Payload["request"].(map[string]any)
	input, ok := request["input"].(map[string]any)
	if !ok {
		t.Fatalf("control request input = %#v, want object-native map", request["input"])
	}
	if got := input["command"]; got != "pwd" {
		t.Fatalf("control request input.command = %#v, want pwd", got)
	}

	if err := conn.WriteJSON(protocolws.Message{
		Type: protocolws.TypeControlResponse,
		ID:   control.ID,
		Payload: map[string]any{
			"behavior": "deny",
			"message":  "stop after input assertion",
		},
	}); err != nil {
		t.Fatalf("write control response: %v", err)
	}
}

func TestHandleWebSocketToolLifecycleCarriesStructuredShellPayload(t *testing.T) {
	sessionManager := session.NewManager(nil)
	server := NewServerWithOptions(log.New(io.Discard, "", 0), sessionManager, &objectToolCallClient{}, Options{
		PermissionPolicy: permissions.Policy{Mode: permissions.ModeDangerFullAccess},
	})
	httpServer := httptest.NewServer(http.HandlerFunc(server.HandleWebSocket))
	t.Cleanup(httpServer.Close)

	wsURL := "ws" + strings.TrimPrefix(httpServer.URL, "http")
	conn, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
	if err != nil {
		t.Fatalf("dial websocket: %v", err)
	}
	t.Cleanup(func() { _ = conn.Close() })

	if err := conn.WriteJSON(protocolws.Message{
		Type:   protocolws.TypeRequest,
		ID:     "1",
		Method: protocolws.MethodConnect,
		Payload: map[string]any{
			"role":            "client",
			"client_identity": "web-ui",
			"agent_id":        "main",
		},
	}); err != nil {
		t.Fatalf("write connect: %v", err)
	}
	var discard protocolws.Message
	_ = conn.ReadJSON(&discard)
	_ = conn.ReadJSON(&discard)

	if err := conn.WriteJSON(protocolws.Message{
		Type:   protocolws.TypeRequest,
		ID:     "2",
		Method: protocolws.MethodSendMessage,
		Payload: map[string]any{
			"content": "emit object tool input",
		},
	}); err != nil {
		t.Fatalf("write inbound: %v", err)
	}

	var called, result protocolws.Message
	for i := 0; i < 20; i++ {
		var event protocolws.Message
		if err := conn.ReadJSON(&event); err != nil {
			t.Fatalf("read event %d: %v", i, err)
		}
		if event.Type == protocolws.TypeEvent && event.Event == "tool.called" {
			called = event
		}
		if event.Type == protocolws.TypeEvent && event.Event == "tool.result" {
			result = event
			break
		}
	}
	if called.Event != "tool.called" || result.Event != "tool.result" {
		t.Fatalf("called=%#v result=%#v, want tool.called and tool.result", called, result)
	}
	if called.Payload["tool_name"] != "system.run" {
		t.Fatalf("tool.called payload = %#v, want system.run", called.Payload)
	}
	if called.Payload["tool_use_id"] == "" || called.Payload["provider_message_id"] == "" {
		t.Fatalf("tool.called payload = %#v, want tool and provider ids", called.Payload)
	}
	inputObject, _ := called.Payload["tool_input_object"].(map[string]any)
	if inputObject["command"] != "pwd" {
		t.Fatalf("tool.called payload = %#v, want structured input object", called.Payload)
	}
	if result.Payload["tool_use_id"] == "" || result.Payload["provider_message_id"] == "" {
		t.Fatalf("tool.result payload = %#v, want lifecycle ids", result.Payload)
	}
	meta, _ := result.Payload["meta"].(map[string]any)
	if meta["command"] != "pwd" {
		t.Fatalf("tool.result meta = %#v, want shell command", meta)
	}
	if meta["working_directory"] == "" {
		t.Fatalf("tool.result meta = %#v, want working directory", meta)
	}
	structured, _ := result.Payload["structured_content"].(map[string]any)
	if structured["command"] != "pwd" || structured["exit_code"] != float64(0) {
		t.Fatalf("tool.result structured = %#v, want shell structured payload", structured)
	}
}

func TestHandleWebSocketShellFailurePreservesStructuredResult(t *testing.T) {
	sessionManager := session.NewManager(nil)
	registry := tools.NewRegistry(systemtools.NewBashTool(sandbox.NewRouter(shellFailureExecutorForGateway{}, nil)))
	client := &failingShellToolCallClient{}
	server := NewServerWithOptions(log.New(io.Discard, "", 0), sessionManager, client, Options{
		PermissionPolicy: permissions.Policy{Mode: permissions.ModeDangerFullAccess},
	})
	server.runner = runtimepkg.NewRunnerWithOptions(sessionManager, client, workspace.NewLoader(""), registry, runtimepkg.Options{
		PermissionPolicy: permissions.Policy{Mode: permissions.ModeDangerFullAccess},
	})
	server.queue = runtimepkg.NewQueue(server.runner)

	httpServer := httptest.NewServer(http.HandlerFunc(server.HandleWebSocket))
	t.Cleanup(httpServer.Close)

	wsURL := "ws" + strings.TrimPrefix(httpServer.URL, "http")
	conn, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
	if err != nil {
		t.Fatalf("dial websocket: %v", err)
	}
	t.Cleanup(func() { _ = conn.Close() })

	if err := conn.WriteJSON(protocolws.Message{
		Type:   protocolws.TypeRequest,
		ID:     "1",
		Method: protocolws.MethodConnect,
		Payload: map[string]any{
			"role":            "client",
			"client_identity": "web-ui",
			"agent_id":        "main",
		},
	}); err != nil {
		t.Fatalf("write connect: %v", err)
	}
	_ = conn.ReadJSON(&protocolws.Message{})
	_ = conn.ReadJSON(&protocolws.Message{})

	if err := conn.WriteJSON(protocolws.Message{
		Type:   protocolws.TypeRequest,
		ID:     "2",
		Method: protocolws.MethodSendMessage,
		Payload: map[string]any{
			"content": "emit failing shell input",
		},
	}); err != nil {
		t.Fatalf("write inbound: %v", err)
	}

	for i := 0; i < 20; i++ {
		var event protocolws.Message
		if err := conn.ReadJSON(&event); err != nil {
			t.Fatalf("read event %d: %v", i, err)
		}
		if event.Type != protocolws.TypeEvent || event.Event != "tool.result" {
			continue
		}
		meta, _ := event.Payload["meta"].(map[string]any)
		structured, _ := event.Payload["structured_content"].(map[string]any)
		if meta["exit_code"] != float64(1) || meta["stderr"] != "exit status 1" {
			t.Fatalf("tool.result meta = %#v, want preserved failure shell meta", meta)
		}
		if structured["exit_code"] != float64(1) || structured["stdout"] != "permission denied" {
			t.Fatalf("tool.result structured = %#v, want preserved failure structured payload", structured)
		}
		message, _ := event.Payload["message"].(map[string]any)
		if !strings.Contains(message["content"].(string), "permission denied") {
			t.Fatalf("tool.result message = %#v, want shell output content", message)
		}
		return
	}
	t.Fatal("expected tool.result for failing shell")
}

func TestHandleWebSocketCanUseToolControlRequestRacesFallbackPermissionHook(t *testing.T) {
	sessionManager := session.NewManager(nil)
	server := NewServerWithOptions(log.New(io.Discard, "", 0), sessionManager, llm.NewMockClient(), Options{
		PermissionPolicy:         permissions.Policy{Mode: permissions.ModeAsk},
		PermissionControlTimeout: 2 * time.Second,
		PermissionHook: permissionHookFunc(func(_ context.Context, request queryengine.PermissionHookRequest) (permissions.Decision, bool, error) {
			if request.ToolName != "system.run" {
				t.Fatalf("permission hook tool = %q, want system.run", request.ToolName)
			}
			return permissions.Decision{
				Allowed: true,
				DecisionReason: permissions.DecisionReason{
					Type:     permissions.DecisionReasonHook,
					HookName: "PermissionRequest",
					Reason:   "allowed before sdk host responded",
				},
			}, true, nil
		}),
	})
	httpServer := httptest.NewServer(http.HandlerFunc(server.HandleWebSocket))
	t.Cleanup(httpServer.Close)

	wsURL := "ws" + strings.TrimPrefix(httpServer.URL, "http")
	conn, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
	if err != nil {
		t.Fatalf("dial websocket: %v", err)
	}
	t.Cleanup(func() { _ = conn.Close() })

	if err := conn.WriteJSON(protocolws.Message{
		Type:   protocolws.TypeRequest,
		ID:     "1",
		Method: protocolws.MethodConnect,
		Payload: map[string]any{
			"role":                        "sdk",
			"client_identity":             "sdk-host",
			"agent_id":                    "main",
			"supports_permission_control": true,
		},
	}); err != nil {
		t.Fatalf("write connect: %v", err)
	}
	var discard protocolws.Message
	_ = conn.ReadJSON(&discard)
	_ = conn.ReadJSON(&discard)

	if err := conn.WriteJSON(protocolws.Message{
		Type:   protocolws.TypeRequest,
		ID:     "2",
		Method: protocolws.MethodSendMessage,
		Payload: map[string]any{
			"content": "tool run pwd",
		},
	}); err != nil {
		t.Fatalf("write inbound: %v", err)
	}

	foundControlRequest := false
	foundToolCalled := false
	if err := conn.SetReadDeadline(time.Now().Add(3 * time.Second)); err != nil {
		t.Fatalf("set read deadline: %v", err)
	}
	for i := 0; i < 16; i++ {
		var message protocolws.Message
		if err := conn.ReadJSON(&message); err != nil {
			if foundToolCalled {
				break
			}
			t.Fatalf("read message %d: %v", i, err)
		}
		if message.Type == protocolws.TypeControlRequest {
			foundControlRequest = true
			continue
		}
		if message.Type == protocolws.TypeEvent && message.Event == "tool.called" {
			foundToolCalled = true
			break
		}
		if message.Type == protocolws.TypeEvent && message.Event == "permission.required" {
			t.Fatalf("unexpected permission.required while hook should win race: %#v", message.Payload)
		}
		if message.Type == protocolws.TypeEvent && message.Event == "run.error" {
			t.Fatalf("unexpected run.error while hook should win race: %#v", message.Payload)
		}
	}
	if err := conn.SetReadDeadline(time.Time{}); err != nil {
		t.Fatalf("clear read deadline: %v", err)
	}
	if !foundControlRequest {
		t.Fatal("expected sdk can_use_tool control_request to be emitted")
	}
	if !foundToolCalled {
		t.Fatal("expected PermissionRequest hook to win race before sdk timeout")
	}
}

func TestHandleWebSocketApprovalListReturnsPendingRequests(t *testing.T) {
	sessionManager := session.NewManager(nil)
	server := NewServerWithOptions(log.New(io.Discard, "", 0), sessionManager, llm.NewMockClient(), Options{
		PermissionPolicy: permissions.Policy{Mode: permissions.ModeAsk},
	})
	httpServer := httptest.NewServer(http.HandlerFunc(server.HandleWebSocket))
	t.Cleanup(httpServer.Close)

	wsURL := "ws" + strings.TrimPrefix(httpServer.URL, "http")
	conn, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
	if err != nil {
		t.Fatalf("dial websocket: %v", err)
	}
	t.Cleanup(func() { _ = conn.Close() })

	if err := conn.WriteJSON(protocolws.Message{
		Type:   protocolws.TypeRequest,
		ID:     "1",
		Method: protocolws.MethodConnect,
		Payload: map[string]any{
			"role":            "client",
			"client_identity": "web-ui",
			"agent_id":        "main",
		},
	}); err != nil {
		t.Fatalf("write connect: %v", err)
	}
	_ = conn.ReadJSON(&protocolws.Message{})
	_ = conn.ReadJSON(&protocolws.Message{})

	if err := conn.WriteJSON(protocolws.Message{
		Type:   protocolws.TypeRequest,
		ID:     "2",
		Method: protocolws.MethodSendMessage,
		Payload: map[string]any{
			"content": "tool run pwd",
		},
	}); err != nil {
		t.Fatalf("write send_message: %v", err)
	}

	foundPermissionRequired := false
	if err := conn.SetReadDeadline(time.Now().Add(2 * time.Second)); err != nil {
		t.Fatalf("set read deadline: %v", err)
	}
	for i := 0; i < 8; i++ {

		var msg protocolws.Message
		if err := conn.ReadJSON(&msg); err != nil {
			if foundPermissionRequired {
				break
			}
			t.Fatalf("read permission-producing event %d: %v", i, err)
		}
		if msg.Type == protocolws.TypeEvent && msg.Event == "permission.required" {
			foundPermissionRequired = true
			break
		}
	}
	if !foundPermissionRequired {
		t.Fatal("expected permission.required event before approval_list")
	}

	if err := conn.WriteJSON(protocolws.Message{
		Type:   protocolws.TypeRequest,
		ID:     "3",
		Method: protocolws.MethodApprovalList,
	}); err != nil {
		t.Fatalf("write approval_list: %v", err)
	}
	if err := conn.SetReadDeadline(time.Now().Add(2 * time.Second)); err != nil {
		t.Fatalf("set approval_list read deadline: %v", err)
	}
	var res protocolws.Message
	if err := conn.ReadJSON(&res); err != nil {
		t.Fatalf("read approval_list: %v", err)
	}
	if !res.OK {
		t.Fatalf("approval_list response = %#v, want ok", res)
	}
	items, ok := res.Payload["approvals"].([]any)
	if !ok || len(items) != 1 {
		t.Fatalf("approvals payload = %#v, want one item", res.Payload["approvals"])
	}
	first, ok := items[0].(map[string]any)
	if !ok {
		t.Fatalf("first approval = %#v, want object", items[0])
	}
	if got := first["tool_name"]; got != "system.run" {
		t.Fatalf("approval tool_name = %#v, want system.run", got)
	}
}

func TestHandleWebSocketApprovalApproveUpdatesRequestStatus(t *testing.T) {
	sessionManager := session.NewManager(nil)
	server := NewServerWithOptions(log.New(io.Discard, "", 0), sessionManager, llm.NewMockClient(), Options{
		PermissionPolicy: permissions.Policy{Mode: permissions.ModeAsk},
	})
	httpServer := httptest.NewServer(http.HandlerFunc(server.HandleWebSocket))
	t.Cleanup(httpServer.Close)

	wsURL := "ws" + strings.TrimPrefix(httpServer.URL, "http")
	conn, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
	if err != nil {
		t.Fatalf("dial websocket: %v", err)
	}
	t.Cleanup(func() { _ = conn.Close() })

	if err := conn.WriteJSON(protocolws.Message{
		Type:   protocolws.TypeRequest,
		ID:     "1",
		Method: protocolws.MethodConnect,
		Payload: map[string]any{
			"role":            "client",
			"client_identity": "web-ui",
			"agent_id":        "main",
		},
	}); err != nil {
		t.Fatalf("write connect: %v", err)
	}
	_ = conn.ReadJSON(&protocolws.Message{})
	_ = conn.ReadJSON(&protocolws.Message{})

	if err := conn.WriteJSON(protocolws.Message{
		Type:   protocolws.TypeRequest,
		ID:     "2",
		Method: protocolws.MethodSendMessage,
		Payload: map[string]any{
			"content": "tool run pwd",
		},
	}); err != nil {
		t.Fatalf("write send_message: %v", err)
	}

	var approvalID string
	if err := conn.SetReadDeadline(time.Now().Add(2 * time.Second)); err != nil {
		t.Fatalf("set read deadline: %v", err)
	}
	for i := 0; i < 8; i++ {

		var msg protocolws.Message
		if err := conn.ReadJSON(&msg); err != nil {
			if approvalID != "" {
				break
			}
			t.Fatalf("read event %d: %v", i, err)
		}
		if msg.Type == protocolws.TypeEvent && msg.Event == "permission.required" {
			approvalID, _ = msg.Payload["approval_id"].(string)
			break
		}
	}
	if approvalID == "" {
		t.Fatal("expected approval id from permission.required")
	}

	if err := conn.WriteJSON(protocolws.Message{
		Type:   protocolws.TypeRequest,
		ID:     "3",
		Method: protocolws.MethodApprovalApprove,
		Payload: map[string]any{
			"approval_id": approvalID,
		},
	}); err != nil {
		t.Fatalf("write approval_approve: %v", err)
	}
	if err := conn.SetReadDeadline(time.Now().Add(2 * time.Second)); err != nil {
		t.Fatalf("set approve read deadline: %v", err)
	}
	var approveRes protocolws.Message
	if err := conn.ReadJSON(&approveRes); err != nil {
		t.Fatalf("read approval_approve: %v", err)
	}
	if !approveRes.OK {
		t.Fatalf("approval_approve response = %#v, want ok", approveRes)
	}
	if got := approveRes.Payload["status"]; got != "approved" {
		t.Fatalf("approval status = %#v, want approved", got)
	}

	foundToolResult := false
	foundAssistant := false
	if err := conn.SetReadDeadline(time.Now().Add(2 * time.Second)); err != nil {
		t.Fatalf("set continuation read deadline: %v", err)
	}
	for i := 0; i < 256; i++ {
		var event protocolws.Message
		if err := conn.ReadJSON(&event); err != nil {
			if foundToolResult && foundAssistant {
				return
			}
			t.Fatalf("read continuation event %d: %v", i, err)
		}
		if event.Type == protocolws.TypeEvent && event.Event == "tool.result" {
			foundToolResult = true
		}
		if event.Type == protocolws.TypeEvent && event.Event == protocolws.EventMessageCreated {
			if message, ok := event.Payload["message"].(map[string]any); ok && message["role"] == "assistant" {
				foundAssistant = true
			}
		}
		if foundToolResult && foundAssistant {
			return
		}
	}
	t.Fatal("expected approval continuation to emit tool.result and assistant reply")
}

func TestHandleWebSocketApprovalRejectUpdatesRequestStatus(t *testing.T) {
	sessionManager := session.NewManager(nil)
	server := NewServerWithOptions(log.New(io.Discard, "", 0), sessionManager, llm.NewMockClient(), Options{
		PermissionPolicy: permissions.Policy{Mode: permissions.ModeAsk},
	})
	httpServer := httptest.NewServer(http.HandlerFunc(server.HandleWebSocket))
	t.Cleanup(httpServer.Close)

	wsURL := "ws" + strings.TrimPrefix(httpServer.URL, "http")
	conn, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
	if err != nil {
		t.Fatalf("dial websocket: %v", err)
	}
	t.Cleanup(func() { _ = conn.Close() })

	if err := conn.WriteJSON(protocolws.Message{
		Type:   protocolws.TypeRequest,
		ID:     "1",
		Method: protocolws.MethodConnect,
		Payload: map[string]any{
			"role":            "client",
			"client_identity": "web-ui",
			"agent_id":        "main",
		},
	}); err != nil {
		t.Fatalf("write connect: %v", err)
	}
	_ = conn.ReadJSON(&protocolws.Message{})
	_ = conn.ReadJSON(&protocolws.Message{})

	if err := conn.WriteJSON(protocolws.Message{
		Type:   protocolws.TypeRequest,
		ID:     "2",
		Method: protocolws.MethodSendMessage,
		Payload: map[string]any{
			"content": "tool run pwd",
		},
	}); err != nil {
		t.Fatalf("write send_message: %v", err)
	}

	var approvalID string
	if err := conn.SetReadDeadline(time.Now().Add(2 * time.Second)); err != nil {
		t.Fatalf("set read deadline: %v", err)
	}
	for i := 0; i < 8; i++ {

		var msg protocolws.Message
		if err := conn.ReadJSON(&msg); err != nil {
			if approvalID != "" {
				break
			}
			t.Fatalf("read event %d: %v", i, err)
		}
		if msg.Type == protocolws.TypeEvent && msg.Event == "permission.required" {
			approvalID, _ = msg.Payload["approval_id"].(string)
			break
		}
	}
	if approvalID == "" {
		t.Fatal("expected approval id from permission.required")
	}

	if err := conn.WriteJSON(protocolws.Message{
		Type:   protocolws.TypeRequest,
		ID:     "3",
		Method: protocolws.MethodApprovalReject,
		Payload: map[string]any{
			"approval_id": approvalID,
		},
	}); err != nil {
		t.Fatalf("write approval_reject: %v", err)
	}
	if err := conn.SetReadDeadline(time.Now().Add(2 * time.Second)); err != nil {
		t.Fatalf("set reject read deadline: %v", err)
	}
	var rejectRes protocolws.Message
	if err := conn.ReadJSON(&rejectRes); err != nil {
		t.Fatalf("read approval_reject: %v", err)
	}
	if !rejectRes.OK {
		t.Fatalf("approval_reject response = %#v, want ok", rejectRes)
	}
	if got := rejectRes.Payload["status"]; got != "rejected" {
		t.Fatalf("approval status = %#v, want rejected", got)
	}
}

func TestHandleWebSocketApprovalRejectWithFeedbackContinuesWithErrorToolResult(t *testing.T) {
	sessionManager := session.NewManager(nil)
	server := NewServerWithOptions(log.New(io.Discard, "", 0), sessionManager, llm.NewMockClient(), Options{
		PermissionPolicy: permissions.Policy{Mode: permissions.ModeAsk},
	})
	httpServer := httptest.NewServer(http.HandlerFunc(server.HandleWebSocket))
	t.Cleanup(httpServer.Close)

	wsURL := "ws" + strings.TrimPrefix(httpServer.URL, "http")
	conn, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
	if err != nil {
		t.Fatalf("dial websocket: %v", err)
	}
	t.Cleanup(func() { _ = conn.Close() })

	if err := conn.WriteJSON(protocolws.Message{
		Type:   protocolws.TypeRequest,
		ID:     "1",
		Method: protocolws.MethodConnect,
		Payload: map[string]any{
			"role":            "client",
			"client_identity": "web-ui",
			"agent_id":        "main",
		},
	}); err != nil {
		t.Fatalf("write connect: %v", err)
	}
	var connectResponse protocolws.Message
	if err := conn.ReadJSON(&connectResponse); err != nil {
		t.Fatalf("read connect response: %v", err)
	}
	var discard protocolws.Message
	_ = conn.ReadJSON(&discard)
	sessionID, _ := connectResponse.Payload["session_id"].(string)
	if sessionID == "" {
		t.Fatalf("connect response payload = %#v, want session_id", connectResponse.Payload)
	}

	if err := conn.WriteJSON(protocolws.Message{
		Type:   protocolws.TypeRequest,
		ID:     "2",
		Method: protocolws.MethodSendMessage,
		Payload: map[string]any{
			"content": "tool run pwd",
		},
	}); err != nil {
		t.Fatalf("write send_message: %v", err)
	}
	approvalID := waitForPermissionRequired(t, conn)

	if err := conn.WriteJSON(protocolws.Message{
		Type:   protocolws.TypeRequest,
		ID:     "3",
		Method: protocolws.MethodApprovalReject,
		Payload: map[string]any{
			"approval_id":     approvalID,
			"reject_feedback": "use a safer command",
			"content_blocks": []map[string]any{{
				"type": "text",
				"text": "extra rejection block",
			}},
		},
	}); err != nil {
		t.Fatalf("write approval_reject: %v", err)
	}
	waitForEvent(t, conn, "tool.result")

	messages, ok := sessionManager.Messages(sessionID)
	if !ok {
		t.Fatalf("messages for session %q not found", sessionID)
	}
	var toolMessage session.Message
	for _, message := range messages {
		if message.Role == "tool" {
			toolMessage = message
			break
		}
	}
	if toolMessage.ID == "" {
		t.Fatalf("messages = %#v, want rejected tool result message", messages)
	}
	if len(toolMessage.Blocks) != 2 {
		t.Fatalf("tool message blocks = %#v, want error tool result plus reject content block", toolMessage.Blocks)
	}
	if !toolMessage.Blocks[0].IsError || !strings.Contains(toolMessage.Blocks[0].Content, "use a safer command") {
		t.Fatalf("tool result block = %#v, want rejection feedback error", toolMessage.Blocks[0])
	}
	if toolMessage.Blocks[1].Text != "extra rejection block" {
		t.Fatalf("reject content block = %#v, want appended reject block", toolMessage.Blocks[1])
	}
}

func TestHandleWebSocketApprovalListCanFilterByStatus(t *testing.T) {
	sessionManager := session.NewManager(nil)
	server := NewServerWithOptions(log.New(io.Discard, "", 0), sessionManager, llm.NewMockClient(), Options{
		PermissionPolicy: permissions.Policy{Mode: permissions.ModeAsk},
	})
	httpServer := httptest.NewServer(http.HandlerFunc(server.HandleWebSocket))
	t.Cleanup(httpServer.Close)

	wsURL := "ws" + strings.TrimPrefix(httpServer.URL, "http")
	conn, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
	if err != nil {
		t.Fatalf("dial websocket: %v", err)
	}
	t.Cleanup(func() { _ = conn.Close() })

	if err := conn.WriteJSON(protocolws.Message{
		Type:   protocolws.TypeRequest,
		ID:     "1",
		Method: protocolws.MethodConnect,
		Payload: map[string]any{
			"role":            "client",
			"client_identity": "web-ui",
			"agent_id":        "main",
		},
	}); err != nil {
		t.Fatalf("write connect: %v", err)
	}
	_ = conn.ReadJSON(&protocolws.Message{})
	_ = conn.ReadJSON(&protocolws.Message{})

	if err := conn.WriteJSON(protocolws.Message{
		Type:   protocolws.TypeRequest,
		ID:     "2",
		Method: protocolws.MethodSendMessage,
		Payload: map[string]any{
			"content": "tool run pwd",
		},
	}); err != nil {
		t.Fatalf("write send_message: %v", err)
	}

	var approvalID string
	if err := conn.SetReadDeadline(time.Now().Add(2 * time.Second)); err != nil {
		t.Fatalf("set read deadline: %v", err)
	}
	for i := 0; i < 8; i++ {

		var msg protocolws.Message
		if err := conn.ReadJSON(&msg); err != nil {
			if approvalID != "" {
				break
			}
			t.Fatalf("read event %d: %v", i, err)
		}
		if msg.Type == protocolws.TypeEvent && msg.Event == "permission.required" {
			approvalID, _ = msg.Payload["approval_id"].(string)
			break
		}
	}
	if approvalID == "" {
		t.Fatal("expected approval id")
	}

	if err := conn.WriteJSON(protocolws.Message{
		Type:   protocolws.TypeRequest,
		ID:     "3",
		Method: protocolws.MethodApprovalReject,
		Payload: map[string]any{
			"approval_id": approvalID,
		},
	}); err != nil {
		t.Fatalf("write approval_reject: %v", err)
	}
	if err := conn.SetReadDeadline(time.Now().Add(2 * time.Second)); err != nil {
		t.Fatalf("set reject read deadline: %v", err)
	}
	var rejectRes protocolws.Message
	if err := conn.ReadJSON(&rejectRes); err != nil {
		t.Fatalf("read approval_reject: %v", err)
	}
	if !rejectRes.OK {
		t.Fatalf("approval_reject response = %#v, want ok", rejectRes)
	}

	for i := 0; i < 2; i++ {
		var maybeEvent protocolws.Message
		if err := conn.SetReadDeadline(time.Now().Add(2 * time.Second)); err != nil {
			t.Fatalf("set approval.updated read deadline: %v", err)
		}
		if err := conn.ReadJSON(&maybeEvent); err == nil {
			if maybeEvent.Type == protocolws.TypeEvent && maybeEvent.Event == "approval.updated" {
				break
			}
		}
	}

	if err := conn.WriteJSON(protocolws.Message{
		Type:   protocolws.TypeRequest,
		ID:     "4",
		Method: protocolws.MethodApprovalList,
		Payload: map[string]any{
			"status": "pending",
		},
	}); err != nil {
		t.Fatalf("write approval_list pending: %v", err)
	}
	if err := conn.SetReadDeadline(time.Now().Add(2 * time.Second)); err != nil {
		t.Fatalf("set approval_list pending read deadline: %v", err)
	}
	var pendingRes protocolws.Message
	if err := conn.ReadJSON(&pendingRes); err != nil {
		t.Fatalf("read approval_list pending: %v", err)
	}
	pendingItems, ok := pendingRes.Payload["approvals"].([]any)
	if !ok {
		t.Fatalf("pending approvals payload = %#v", pendingRes.Payload["approvals"])
	}
	if len(pendingItems) != 0 {
		t.Fatalf("pending approvals = %#v, want empty", pendingItems)
	}

	if err := conn.WriteJSON(protocolws.Message{
		Type:   protocolws.TypeRequest,
		ID:     "5",
		Method: protocolws.MethodApprovalList,
		Payload: map[string]any{
			"status": "rejected",
		},
	}); err != nil {
		t.Fatalf("write approval_list rejected: %v", err)
	}
	if err := conn.SetReadDeadline(time.Now().Add(2 * time.Second)); err != nil {
		t.Fatalf("set approval_list rejected read deadline: %v", err)
	}
	var rejectedRes protocolws.Message
	if err := conn.ReadJSON(&rejectedRes); err != nil {
		t.Fatalf("read approval_list rejected: %v", err)
	}
	rejectedItems, ok := rejectedRes.Payload["approvals"].([]any)
	if !ok || len(rejectedItems) != 1 {
		t.Fatalf("rejected approvals payload = %#v, want one item", rejectedRes.Payload["approvals"])
	}
}

func TestHandleWebSocketApprovalDecisionEmitsAuditEvent(t *testing.T) {
	sessionManager := session.NewManager(nil)
	server := NewServerWithOptions(log.New(io.Discard, "", 0), sessionManager, llm.NewMockClient(), Options{
		PermissionPolicy: permissions.Policy{Mode: permissions.ModeAsk},
	})
	httpServer := httptest.NewServer(http.HandlerFunc(server.HandleWebSocket))
	t.Cleanup(httpServer.Close)

	wsURL := "ws" + strings.TrimPrefix(httpServer.URL, "http")
	conn, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
	if err != nil {
		t.Fatalf("dial websocket: %v", err)
	}
	t.Cleanup(func() { _ = conn.Close() })

	if err := conn.WriteJSON(protocolws.Message{
		Type:   protocolws.TypeRequest,
		ID:     "1",
		Method: protocolws.MethodConnect,
		Payload: map[string]any{
			"role":            "client",
			"client_identity": "web-ui",
			"agent_id":        "main",
		},
	}); err != nil {
		t.Fatalf("write connect: %v", err)
	}
	_ = conn.ReadJSON(&protocolws.Message{})
	_ = conn.ReadJSON(&protocolws.Message{})

	if err := conn.WriteJSON(protocolws.Message{
		Type:   protocolws.TypeRequest,
		ID:     "2",
		Method: protocolws.MethodSendMessage,
		Payload: map[string]any{
			"content": "tool run pwd",
		},
	}); err != nil {
		t.Fatalf("write send_message: %v", err)
	}

	var approvalID string
	if err := conn.SetReadDeadline(time.Now().Add(2 * time.Second)); err != nil {
		t.Fatalf("set read deadline: %v", err)
	}
	for i := 0; i < 8; i++ {

		var msg protocolws.Message
		if err := conn.ReadJSON(&msg); err != nil {
			if approvalID != "" {
				break
			}
			t.Fatalf("read event %d: %v", i, err)
		}
		if msg.Type == protocolws.TypeEvent && msg.Event == "permission.required" {
			approvalID, _ = msg.Payload["approval_id"].(string)
			break
		}
	}
	if approvalID == "" {
		t.Fatal("expected approval id")
	}

	if err := conn.WriteJSON(protocolws.Message{
		Type:   protocolws.TypeRequest,
		ID:     "3",
		Method: protocolws.MethodApprovalReject,
		Payload: map[string]any{
			"approval_id": approvalID,
		},
	}); err != nil {
		t.Fatalf("write approval_reject: %v", err)
	}
	if err := conn.SetReadDeadline(time.Now().Add(2 * time.Second)); err != nil {
		t.Fatalf("set reject read deadline: %v", err)
	}
	var rejectRes protocolws.Message
	if err := conn.ReadJSON(&rejectRes); err != nil {
		t.Fatalf("read approval_reject: %v", err)
	}
	if !rejectRes.OK {
		t.Fatalf("approval_reject response = %#v", rejectRes)
	}

	for i := 0; i < 4; i++ {
		var event protocolws.Message
		if err := conn.SetReadDeadline(time.Now().Add(2 * time.Second)); err != nil {
			t.Fatalf("set audit read deadline: %v", err)
		}
		if err := conn.ReadJSON(&event); err != nil {
			t.Fatalf("read audit event %d: %v", i, err)
		}
		if event.Type == protocolws.TypeEvent && event.Event == "approval.updated" {
			if got := event.Payload["status"]; got != "rejected" {
				t.Fatalf("approval.updated status = %#v, want rejected", got)
			}
			return
		}
	}
	t.Fatal("expected approval.updated event")
}

func TestHandleWebSocketApprovalClearRemovesTerminalRecords(t *testing.T) {
	sessionManager := session.NewManager(nil)
	server := NewServerWithOptions(log.New(io.Discard, "", 0), sessionManager, llm.NewMockClient(), Options{
		PermissionPolicy: permissions.Policy{Mode: permissions.ModeAsk},
	})
	httpServer := httptest.NewServer(http.HandlerFunc(server.HandleWebSocket))
	t.Cleanup(httpServer.Close)

	wsURL := "ws" + strings.TrimPrefix(httpServer.URL, "http")
	conn, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
	if err != nil {
		t.Fatalf("dial websocket: %v", err)
	}
	t.Cleanup(func() { _ = conn.Close() })

	if err := conn.WriteJSON(protocolws.Message{
		Type:   protocolws.TypeRequest,
		ID:     "1",
		Method: protocolws.MethodConnect,
		Payload: map[string]any{
			"role":            "client",
			"client_identity": "web-ui",
			"agent_id":        "main",
		},
	}); err != nil {
		t.Fatalf("write connect: %v", err)
	}
	_ = conn.ReadJSON(&protocolws.Message{})
	_ = conn.ReadJSON(&protocolws.Message{})

	for i := 0; i < 2; i++ {
		if err := conn.WriteJSON(protocolws.Message{
			Type:   protocolws.TypeRequest,
			ID:     string(rune('2' + i)),
			Method: protocolws.MethodSendMessage,
			Payload: map[string]any{
				"content": "tool run pwd",
			},
		}); err != nil {
			t.Fatalf("write send_message %d: %v", i, err)
		}
		if err := conn.SetReadDeadline(time.Now().Add(2 * time.Second)); err != nil {
			t.Fatalf("set permission read deadline %d: %v", i, err)
		}
		var approvalID string
		for j := 0; j < 16; j++ {

			var msg protocolws.Message
			if err := conn.ReadJSON(&msg); err != nil {
				t.Fatalf("read event %d/%d: %v", i, j, err)
			}
			if msg.Type == protocolws.TypeEvent && msg.Event == "permission.required" {
				approvalID, _ = msg.Payload["approval_id"].(string)
				break
			}
		}
		if err := conn.SetReadDeadline(time.Time{}); err != nil {
			t.Fatalf("clear permission read deadline %d: %v", i, err)
		}
		if approvalID == "" {
			t.Fatalf("iteration %d missing approval id", i)
		}
		method := protocolws.MethodApprovalReject
		if i == 1 {
			method = protocolws.MethodApprovalApprove
		}
		if err := conn.WriteJSON(protocolws.Message{
			Type:   protocolws.TypeRequest,
			ID:     string(rune('4' + i)),
			Method: method,
			Payload: map[string]any{
				"approval_id": approvalID,
			},
		}); err != nil {
			t.Fatalf("write approval decision %d: %v", i, err)
		}
		var res protocolws.Message
		if err := conn.SetReadDeadline(time.Now().Add(2 * time.Second)); err != nil {
			t.Fatalf("set approval decision read deadline %d: %v", i, err)
		}
		if err := conn.ReadJSON(&res); err != nil {
			t.Fatalf("read approval decision %d: %v", i, err)
		}
		if err := conn.SetReadDeadline(time.Time{}); err != nil {
			t.Fatalf("clear approval decision read deadline %d: %v", i, err)
		}
		if !res.OK {
			t.Fatalf("approval decision response %d = %#v", i, res)
		}
	}

	if err := conn.WriteJSON(protocolws.Message{
		Type:   protocolws.TypeRequest,
		ID:     "6",
		Method: protocolws.MethodApprovalClear,
	}); err != nil {
		t.Fatalf("write approval_clear: %v", err)
	}
	var clearRes protocolws.Message
	if err := conn.SetReadDeadline(time.Now().Add(2 * time.Second)); err != nil {
		t.Fatalf("set approval_clear read deadline: %v", err)
	}
	for i := 0; i < 16; i++ {
		if err := conn.ReadJSON(&clearRes); err != nil {
			t.Fatalf("read approval_clear %d: %v", i, err)
		}
		if clearRes.Type == protocolws.TypeResponse && clearRes.ID == "6" {
			break
		}
	}
	if err := conn.SetReadDeadline(time.Time{}); err != nil {
		t.Fatalf("clear approval_clear read deadline: %v", err)
	}
	if !clearRes.OK {
		t.Fatalf("approval_clear response = %#v", clearRes)
	}
	if got := clearRes.Payload["cleared"]; got != float64(2) && got != 2 {
		t.Fatalf("cleared count = %#v, want 2", got)
	}
}

func TestHandleWebSocketSpawnSubagent(t *testing.T) {
	sessionManager := session.NewManager(nil)
	server := NewServerWithOptions(log.New(io.Discard, "", 0), sessionManager, llm.NewMockClient(), Options{
		PermissionPolicy: permissions.Policy{Mode: permissions.ModeDangerFullAccess},
	})
	httpServer := httptest.NewServer(http.HandlerFunc(server.HandleWebSocket))
	t.Cleanup(httpServer.Close)

	wsURL := "ws" + strings.TrimPrefix(httpServer.URL, "http")
	conn, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
	if err != nil {
		t.Fatalf("dial websocket: %v", err)
	}
	t.Cleanup(func() { _ = conn.Close() })

	if err := conn.WriteJSON(protocolws.Message{
		Type:   protocolws.TypeRequest,
		ID:     "1",
		Method: protocolws.MethodConnect,
		Payload: map[string]any{
			"role":            "client",
			"client_identity": "web-ui",
			"agent_id":        "main",
		},
	}); err != nil {
		t.Fatalf("write connect: %v", err)
	}
	var discard protocolws.Message
	_ = conn.ReadJSON(&discard)
	_ = conn.ReadJSON(&discard)

	if err := conn.WriteJSON(protocolws.Message{
		Type:   protocolws.TypeRequest,
		ID:     "2",
		Method: protocolws.MethodSpawnSubagent,
		Payload: map[string]any{
			"label":  "research",
			"prompt": "tool upper hello subagent",
		},
	}); err != nil {
		t.Fatalf("write spawn request: %v", err)
	}

	var ack protocolws.Message
	if err := conn.ReadJSON(&ack); err != nil {
		t.Fatalf("read spawn ack: %v", err)
	}
	if ack.Type != protocolws.TypeResponse || !ack.OK {
		t.Fatalf("spawn ack = %#v, want ok response", ack)
	}

	for i := 0; i < 16; i++ {
		var event protocolws.Message
		if err := conn.ReadJSON(&event); err != nil {
			t.Fatalf("read event %d: %v", i, err)
		}
		if event.Type == protocolws.TypeEvent && event.Event == protocolws.EventSubagentCompleted {
			if got := event.Payload["status"]; got != "completed" {
				t.Fatalf("subagent status = %#v, want completed", got)
			}
			if got := event.Payload["output"]; got != "Using tool result: text.upper: HELLO SUBAGENT" {
				t.Fatalf("subagent output = %#v, want final assistant output", got)
			}
			return
		}
	}

	t.Fatal("expected subagent.completed event")
}

func TestHandleWebSocketSessionStatus(t *testing.T) {
	sessionManager := session.NewManager(nil)
	server := NewServer(log.New(io.Discard, "", 0), sessionManager, llm.NewMockClient())
	httpServer := httptest.NewServer(http.HandlerFunc(server.HandleWebSocket))
	t.Cleanup(httpServer.Close)

	wsURL := "ws" + strings.TrimPrefix(httpServer.URL, "http")
	conn, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
	if err != nil {
		t.Fatalf("dial websocket: %v", err)
	}
	t.Cleanup(func() { _ = conn.Close() })

	if err := conn.WriteJSON(protocolws.Message{
		Type:   protocolws.TypeRequest,
		ID:     "1",
		Method: protocolws.MethodConnect,
		Payload: map[string]any{
			"role":            "client",
			"client_identity": "web-ui",
			"agent_id":        "main",
		},
	}); err != nil {
		t.Fatalf("write connect: %v", err)
	}
	var res protocolws.Message
	if err := conn.ReadJSON(&res); err != nil {
		t.Fatalf("read connect response: %v", err)
	}
	_ = conn.ReadJSON(&protocolws.Message{})
	sessionID, _ := res.Payload["session_id"].(string)

	if err := conn.WriteJSON(protocolws.Message{
		Type:   protocolws.TypeRequest,
		ID:     "2",
		Method: protocolws.MethodSessionStatus,
		Payload: map[string]any{
			"session_id": sessionID,
		},
	}); err != nil {
		t.Fatalf("write session status: %v", err)
	}
	var status protocolws.Message
	if err := conn.ReadJSON(&status); err != nil {
		t.Fatalf("read session status: %v", err)
	}
	if !status.OK {
		t.Fatalf("status response = %#v, want ok", status)
	}
	if got := status.Payload["session_id"]; got != sessionID {
		t.Fatalf("session status id = %#v, want %q", got, sessionID)
	}
}

func TestHandleWebSocketSessionStatusIncludesDerivedPermissionModeForSubagent(t *testing.T) {
	sessionManager := session.NewManager(nil)
	server := NewServerWithOptions(log.New(io.Discard, "", 0), sessionManager, llm.NewMockClient(), Options{
		PermissionPolicy: permissions.Policy{
			Mode:         permissions.ModeDangerFullAccess,
			SubagentMode: permissions.ModeAsk,
		},
	})
	httpServer := httptest.NewServer(http.HandlerFunc(server.HandleWebSocket))
	t.Cleanup(httpServer.Close)

	wsURL := "ws" + strings.TrimPrefix(httpServer.URL, "http")
	conn, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
	if err != nil {
		t.Fatalf("dial websocket: %v", err)
	}
	t.Cleanup(func() { _ = conn.Close() })

	if err := conn.WriteJSON(protocolws.Message{
		Type:   protocolws.TypeRequest,
		ID:     "1",
		Method: protocolws.MethodConnect,
		Payload: map[string]any{
			"role":            "client",
			"client_identity": "web-ui",
			"agent_id":        "main",
		},
	}); err != nil {
		t.Fatalf("write connect: %v", err)
	}
	var discard protocolws.Message
	_ = conn.ReadJSON(&discard)
	_ = conn.ReadJSON(&discard)

	if err := conn.WriteJSON(protocolws.Message{
		Type:   protocolws.TypeRequest,
		ID:     "2",
		Method: protocolws.MethodSpawnSubagent,
		Payload: map[string]any{
			"label":  "restricted",
			"prompt": "hello subagent",
		},
	}); err != nil {
		t.Fatalf("write spawn request: %v", err)
	}

	var spawnAck protocolws.Message
	if err := conn.ReadJSON(&spawnAck); err != nil {
		t.Fatalf("read spawn ack: %v", err)
	}
	childSessionID, _ := spawnAck.Payload["child_session_id"].(string)
	if childSessionID == "" {
		t.Fatalf("spawn ack = %#v, want child session id", spawnAck.Payload)
	}

	if err := conn.WriteJSON(protocolws.Message{
		Type:   protocolws.TypeRequest,
		ID:     "3",
		Method: protocolws.MethodSessionStatus,
		Payload: map[string]any{
			"session_id": childSessionID,
		},
	}); err != nil {
		t.Fatalf("write session status: %v", err)
	}

	var statusRes protocolws.Message
	for i := 0; i < 4; i++ {
		if err := conn.ReadJSON(&statusRes); err != nil {
			t.Fatalf("read session status %d: %v", i, err)
		}
		if statusRes.Type == protocolws.TypeResponse && statusRes.ID == "3" {
			break
		}
	}
	if got := statusRes.Payload["permission_mode"]; got != string(permissions.ModeAsk) {
		t.Fatalf("permission mode = %#v, want %q", got, permissions.ModeAsk)
	}
}

func TestHandleWebSocketSessionSetPermissionUpdatesStatusAndEnforcement(t *testing.T) {
	sessionManager := session.NewManager(nil)
	server := NewServerWithOptions(log.New(io.Discard, "", 0), sessionManager, llm.NewMockClient(), Options{
		PermissionPolicy: permissions.Policy{Mode: permissions.ModeDangerFullAccess},
	})
	httpServer := httptest.NewServer(http.HandlerFunc(server.HandleWebSocket))
	t.Cleanup(httpServer.Close)

	wsURL := "ws" + strings.TrimPrefix(httpServer.URL, "http")
	conn, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
	if err != nil {
		t.Fatalf("dial websocket: %v", err)
	}
	t.Cleanup(func() { _ = conn.Close() })

	if err := conn.WriteJSON(protocolws.Message{
		Type:   protocolws.TypeRequest,
		ID:     "1",
		Method: protocolws.MethodConnect,
		Payload: map[string]any{
			"role":            "client",
			"client_identity": "web-ui",
			"agent_id":        "main",
		},
	}); err != nil {
		t.Fatalf("write connect: %v", err)
	}
	var connectRes protocolws.Message
	if err := conn.ReadJSON(&connectRes); err != nil {
		t.Fatalf("read connect response: %v", err)
	}
	_ = conn.ReadJSON(&protocolws.Message{})
	sessionID, _ := connectRes.Payload["session_id"].(string)

	if err := conn.WriteJSON(protocolws.Message{
		Type:   protocolws.TypeRequest,
		ID:     "2",
		Method: protocolws.MethodSessionSetPermission,
		Payload: map[string]any{
			"session_id": sessionID,
			"mode":       "ask",
		},
	}); err != nil {
		t.Fatalf("write session_set_permission: %v", err)
	}
	var setRes protocolws.Message
	if err := conn.ReadJSON(&setRes); err != nil {
		t.Fatalf("read session_set_permission: %v", err)
	}
	if !setRes.OK {
		t.Fatalf("set permission response = %#v, want ok", setRes)
	}

	if err := conn.WriteJSON(protocolws.Message{
		Type:   protocolws.TypeRequest,
		ID:     "3",
		Method: protocolws.MethodSessionStatus,
		Payload: map[string]any{
			"session_id": sessionID,
		},
	}); err != nil {
		t.Fatalf("write session status: %v", err)
	}
	var statusRes protocolws.Message
	if err := conn.ReadJSON(&statusRes); err != nil {
		t.Fatalf("read session status: %v", err)
	}
	if got := statusRes.Payload["permission_mode"]; got != "ask" {
		t.Fatalf("permission mode = %#v, want ask", got)
	}

	if err := conn.WriteJSON(protocolws.Message{
		Type:   protocolws.TypeRequest,
		ID:     "4",
		Method: protocolws.MethodSendMessage,
		Payload: map[string]any{
			"content": "tool run pwd",
		},
	}); err != nil {
		t.Fatalf("write send_message: %v", err)
	}
	if err := conn.SetReadDeadline(time.Now().Add(2 * time.Second)); err != nil {
		t.Fatalf("set permission-required read deadline: %v", err)
	}
	for i := 0; i < 16; i++ {
		var event protocolws.Message
		if err := conn.ReadJSON(&event); err != nil {
			t.Fatalf("read event %d: %v", i, err)
		}
		if event.Type == protocolws.TypeEvent && event.Event == "permission.required" {
			if err := conn.SetReadDeadline(time.Time{}); err != nil {
				t.Fatalf("clear permission-required read deadline: %v", err)
			}
			return
		}
	}

	t.Fatal("expected permission.required after session permission was switched to ask")
}

func TestHandleWebSocketSessionSetPermissionUpdatesPlanAndAutoModeStatus(t *testing.T) {
	sessionManager := session.NewManager(nil)
	server := NewServerWithOptions(log.New(io.Discard, "", 0), sessionManager, llm.NewMockClient(), Options{
		PermissionPolicy: permissions.Policy{Mode: permissions.ModeDangerFullAccess},
	})
	httpServer := httptest.NewServer(http.HandlerFunc(server.HandleWebSocket))
	t.Cleanup(httpServer.Close)

	wsURL := "ws" + strings.TrimPrefix(httpServer.URL, "http")
	conn, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
	if err != nil {
		t.Fatalf("dial websocket: %v", err)
	}
	t.Cleanup(func() { _ = conn.Close() })

	if err := conn.WriteJSON(protocolws.Message{
		Type:   protocolws.TypeRequest,
		ID:     "1",
		Method: protocolws.MethodConnect,
		Payload: map[string]any{
			"role":            "client",
			"client_identity": "web-ui",
			"agent_id":        "main",
		},
	}); err != nil {
		t.Fatalf("write connect: %v", err)
	}
	var connectRes protocolws.Message
	if err := conn.ReadJSON(&connectRes); err != nil {
		t.Fatalf("read connect response: %v", err)
	}
	_ = conn.ReadJSON(&protocolws.Message{})
	sessionID, _ := connectRes.Payload["session_id"].(string)

	if err := conn.WriteJSON(protocolws.Message{
		Type:   protocolws.TypeRequest,
		ID:     "2",
		Method: protocolws.MethodSessionSetPermission,
		Payload: map[string]any{
			"session_id":      sessionID,
			"mode":            "workspace-write",
			"plan_mode":       true,
			"workspace_roots": []string{"C:/repo", "C:/repo/subdir"},
		},
	}); err != nil {
		t.Fatalf("write session_set_permission: %v", err)
	}
	var setRes protocolws.Message
	if err := conn.ReadJSON(&setRes); err != nil {
		t.Fatalf("read session_set_permission: %v", err)
	}
	if !setRes.OK {
		t.Fatalf("set permission response = %#v, want ok", setRes)
	}
	if got := setRes.Payload["plan_mode"]; got != true {
		t.Fatalf("set permission plan_mode = %#v, want true", got)
	}
	if got := setRes.Payload["auto_mode"]; got != false {
		t.Fatalf("set permission auto_mode = %#v, want false", got)
	}
	roots, ok := setRes.Payload["workspace_roots"].([]any)
	if !ok || len(roots) != 1 || roots[0] != "C:/repo" {
		t.Fatalf("set permission workspace_roots = %#v, want collapsed root", setRes.Payload["workspace_roots"])
	}

	if err := conn.WriteJSON(protocolws.Message{
		Type:   protocolws.TypeRequest,
		ID:     "3",
		Method: protocolws.MethodSessionStatus,
		Payload: map[string]any{
			"session_id": sessionID,
		},
	}); err != nil {
		t.Fatalf("write session status: %v", err)
	}
	var statusRes protocolws.Message
	if err := conn.ReadJSON(&statusRes); err != nil {
		t.Fatalf("read session status: %v", err)
	}
	if got := statusRes.Payload["plan_mode"]; got != true {
		t.Fatalf("status plan_mode = %#v, want true", got)
	}
	if got := statusRes.Payload["auto_mode"]; got != false {
		t.Fatalf("status auto_mode = %#v, want false", got)
	}
	roots, ok = statusRes.Payload["workspace_roots"].([]any)
	if !ok || len(roots) != 1 || roots[0] != "C:/repo" {
		t.Fatalf("status workspace_roots = %#v, want collapsed root", statusRes.Payload["workspace_roots"])
	}
}

func TestHandleWebSocketSessionSetPermissionRejectsInvalidPlanAndAutoCombination(t *testing.T) {
	sessionManager := session.NewManager(nil)
	server := NewServerWithOptions(log.New(io.Discard, "", 0), sessionManager, llm.NewMockClient(), Options{
		PermissionPolicy: permissions.Policy{Mode: permissions.ModeDangerFullAccess},
	})
	httpServer := httptest.NewServer(http.HandlerFunc(server.HandleWebSocket))
	t.Cleanup(httpServer.Close)

	wsURL := "ws" + strings.TrimPrefix(httpServer.URL, "http")
	conn, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
	if err != nil {
		t.Fatalf("dial websocket: %v", err)
	}
	t.Cleanup(func() { _ = conn.Close() })

	if err := conn.WriteJSON(protocolws.Message{
		Type:   protocolws.TypeRequest,
		ID:     "1",
		Method: protocolws.MethodConnect,
		Payload: map[string]any{
			"role":            "client",
			"client_identity": "web-ui",
			"agent_id":        "main",
		},
	}); err != nil {
		t.Fatalf("write connect: %v", err)
	}
	var connectRes protocolws.Message
	if err := conn.ReadJSON(&connectRes); err != nil {
		t.Fatalf("read connect response: %v", err)
	}
	_ = conn.ReadJSON(&protocolws.Message{})
	sessionID, _ := connectRes.Payload["session_id"].(string)

	if err := conn.WriteJSON(protocolws.Message{
		Type:   protocolws.TypeRequest,
		ID:     "2",
		Method: protocolws.MethodSessionSetPermission,
		Payload: map[string]any{
			"session_id": sessionID,
			"mode":       "danger-full-access",
			"plan_mode":  true,
			"auto_mode":  true,
		},
	}); err != nil {
		t.Fatalf("write session_set_permission: %v", err)
	}
	var setRes protocolws.Message
	if err := conn.ReadJSON(&setRes); err != nil {
		t.Fatalf("read session_set_permission: %v", err)
	}
	if setRes.OK {
		t.Fatalf("set permission response = %#v, want error", setRes)
	}
	if setRes.Error == nil || !strings.Contains(setRes.Error.Message, "plan mode and auto mode cannot be enabled together") {
		t.Fatalf("set permission error = %#v, want plan/auto validation message", setRes.Error)
	}
}

func TestHandleWebSocketSessionSetPermissionNormalizesClaudeCodeExternalModes(t *testing.T) {
	sessionManager := session.NewManager(nil)
	server := NewServerWithOptions(log.New(io.Discard, "", 0), sessionManager, llm.NewMockClient(), Options{
		PermissionPolicy: permissions.Policy{Mode: permissions.ModeDangerFullAccess},
	})
	httpServer := httptest.NewServer(http.HandlerFunc(server.HandleWebSocket))
	t.Cleanup(httpServer.Close)

	wsURL := "ws" + strings.TrimPrefix(httpServer.URL, "http")
	conn, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
	if err != nil {
		t.Fatalf("dial websocket: %v", err)
	}
	t.Cleanup(func() { _ = conn.Close() })

	if err := conn.WriteJSON(protocolws.Message{
		Type:   protocolws.TypeRequest,
		ID:     "1",
		Method: protocolws.MethodConnect,
		Payload: map[string]any{
			"role":            "client",
			"client_identity": "web-ui",
			"agent_id":        "main",
		},
	}); err != nil {
		t.Fatalf("write connect: %v", err)
	}
	var connectRes protocolws.Message
	if err := conn.ReadJSON(&connectRes); err != nil {
		t.Fatalf("read connect response: %v", err)
	}
	_ = conn.ReadJSON(&protocolws.Message{})
	sessionID, _ := connectRes.Payload["session_id"].(string)

	if err := conn.WriteJSON(protocolws.Message{
		Type:   protocolws.TypeRequest,
		ID:     "2",
		Method: protocolws.MethodSessionSetPermission,
		Payload: map[string]any{
			"session_id":    sessionID,
			"mode":          "bypass-permissions",
			"subagent_mode": "dont-ask",
		},
	}); err != nil {
		t.Fatalf("write session_set_permission: %v", err)
	}
	var setRes protocolws.Message
	if err := conn.ReadJSON(&setRes); err != nil {
		t.Fatalf("read session_set_permission: %v", err)
	}
	if !setRes.OK {
		t.Fatalf("set permission response = %#v, want ok", setRes)
	}
	if got := setRes.Payload["permission_mode"]; got != "bypassPermissions" {
		t.Fatalf("permission mode = %#v, want normalized bypassPermissions", got)
	}
	if got := setRes.Payload["subagent_mode"]; got != "dontAsk" {
		t.Fatalf("subagent mode = %#v, want normalized dontAsk", got)
	}
}

func TestHandleWebSocketSessionStatusIncludesMainLoopModelState(t *testing.T) {
	sessionManager := session.NewManager(nil)
	server := NewServerWithOptions(log.New(io.Discard, "", 0), sessionManager, llm.NewMockClient(), Options{
		PermissionPolicy: permissions.Policy{Mode: permissions.ModeDangerFullAccess},
		MainLoopModel:    "claude-sonnet-4-6",
	})
	httpServer := httptest.NewServer(http.HandlerFunc(server.HandleWebSocket))
	t.Cleanup(httpServer.Close)

	wsURL := "ws" + strings.TrimPrefix(httpServer.URL, "http")
	conn, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
	if err != nil {
		t.Fatalf("dial websocket: %v", err)
	}
	t.Cleanup(func() { _ = conn.Close() })

	if err := conn.WriteJSON(protocolws.Message{
		Type:   protocolws.TypeRequest,
		ID:     "1",
		Method: protocolws.MethodConnect,
		Payload: map[string]any{
			"role":            "client",
			"client_identity": "web-ui",
			"agent_id":        "main",
		},
	}); err != nil {
		t.Fatalf("write connect: %v", err)
	}
	var connectRes protocolws.Message
	if err := conn.ReadJSON(&connectRes); err != nil {
		t.Fatalf("read connect response: %v", err)
	}
	_ = conn.ReadJSON(&protocolws.Message{})
	sessionID, _ := connectRes.Payload["session_id"].(string)

	if err := server.runner.SetSessionMainLoopModelOverride(sessionID, "claude-opus-4-6"); err != nil {
		t.Fatalf("set session main loop model override: %v", err)
	}

	if err := conn.WriteJSON(protocolws.Message{
		Type:   protocolws.TypeRequest,
		ID:     "2",
		Method: protocolws.MethodSessionStatus,
		Payload: map[string]any{
			"session_id": sessionID,
		},
	}); err != nil {
		t.Fatalf("write session status: %v", err)
	}
	var statusRes protocolws.Message
	if err := conn.ReadJSON(&statusRes); err != nil {
		t.Fatalf("read session status: %v", err)
	}
	if got := statusRes.Payload["main_loop_model"]; got != "claude-sonnet-4-6" {
		t.Fatalf("main loop model = %#v, want base model", got)
	}
	if got := statusRes.Payload["session_main_loop_model_override"]; got != "claude-opus-4-6" {
		t.Fatalf("session model override = %#v, want override model", got)
	}
	if got := statusRes.Payload["resolved_main_loop_model"]; got != "claude-opus-4-6" {
		t.Fatalf("resolved main loop model = %#v, want resolved override model", got)
	}
	updated, ok := sessionManager.GetByID(sessionID)
	if !ok {
		t.Fatalf("session %q not found", sessionID)
	}
	if updated.Metadata.InitialMainLoopModel != "claude-sonnet-4-6" {
		t.Fatalf("metadata = %#v, want initial main loop model latched before first query", updated.Metadata)
	}
}

func TestHandleWebSocketSessionStatusLatchesInitialMainLoopModelWithoutOverride(t *testing.T) {
	sessionManager := session.NewManager(nil)
	server := NewServerWithOptions(log.New(io.Discard, "", 0), sessionManager, llm.NewMockClient(), Options{
		PermissionPolicy: permissions.Policy{Mode: permissions.ModeDangerFullAccess},
		MainLoopModel:    "claude-sonnet-4-6",
	})
	httpServer := httptest.NewServer(http.HandlerFunc(server.HandleWebSocket))
	t.Cleanup(httpServer.Close)

	wsURL := "ws" + strings.TrimPrefix(httpServer.URL, "http")
	conn, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
	if err != nil {
		t.Fatalf("dial websocket: %v", err)
	}
	t.Cleanup(func() { _ = conn.Close() })

	if err := conn.WriteJSON(protocolws.Message{
		Type:   protocolws.TypeRequest,
		ID:     "1",
		Method: protocolws.MethodConnect,
		Payload: map[string]any{
			"role":            "client",
			"client_identity": "web-ui",
			"agent_id":        "main",
		},
	}); err != nil {
		t.Fatalf("write connect: %v", err)
	}
	var connectRes protocolws.Message
	if err := conn.ReadJSON(&connectRes); err != nil {
		t.Fatalf("read connect response: %v", err)
	}
	_ = conn.ReadJSON(&protocolws.Message{})
	sessionID, _ := connectRes.Payload["session_id"].(string)

	if err := conn.WriteJSON(protocolws.Message{
		Type:   protocolws.TypeRequest,
		ID:     "2",
		Method: protocolws.MethodSessionStatus,
		Payload: map[string]any{
			"session_id": sessionID,
		},
	}); err != nil {
		t.Fatalf("write session status: %v", err)
	}
	var statusRes protocolws.Message
	if err := conn.ReadJSON(&statusRes); err != nil {
		t.Fatalf("read session status: %v", err)
	}
	if got := statusRes.Payload["main_loop_model"]; got != "claude-sonnet-4-6" {
		t.Fatalf("main loop model = %#v, want base model", got)
	}
	updated, ok := sessionManager.GetByID(sessionID)
	if !ok {
		t.Fatalf("session %q not found", sessionID)
	}
	if updated.Metadata.InitialMainLoopModel != "claude-sonnet-4-6" {
		t.Fatalf("metadata = %#v, want initial main loop model latched on status read", updated.Metadata)
	}
}

func TestHandleWebSocketSessionSetModelUpdatesStatusAndResolvedModelState(t *testing.T) {
	sessionManager := session.NewManager(nil)
	server := NewServerWithOptions(log.New(io.Discard, "", 0), sessionManager, llm.NewMockClient(), Options{
		PermissionPolicy: permissions.Policy{Mode: permissions.ModeDangerFullAccess},
		MainLoopModel:    "claude-sonnet-4-6",
	})
	httpServer := httptest.NewServer(http.HandlerFunc(server.HandleWebSocket))
	t.Cleanup(httpServer.Close)

	wsURL := "ws" + strings.TrimPrefix(httpServer.URL, "http")
	conn, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
	if err != nil {
		t.Fatalf("dial websocket: %v", err)
	}
	t.Cleanup(func() { _ = conn.Close() })

	if err := conn.WriteJSON(protocolws.Message{
		Type:   protocolws.TypeRequest,
		ID:     "1",
		Method: protocolws.MethodConnect,
		Payload: map[string]any{
			"role":            "client",
			"client_identity": "web-ui",
			"agent_id":        "main",
		},
	}); err != nil {
		t.Fatalf("write connect: %v", err)
	}
	var connectRes protocolws.Message
	if err := conn.ReadJSON(&connectRes); err != nil {
		t.Fatalf("read connect response: %v", err)
	}
	_ = conn.ReadJSON(&protocolws.Message{})
	sessionID, _ := connectRes.Payload["session_id"].(string)

	if err := conn.WriteJSON(protocolws.Message{
		Type:   protocolws.TypeRequest,
		ID:     "2",
		Method: protocolws.MethodSessionSetModel,
		Payload: map[string]any{
			"session_id": sessionID,
			"model":      "claude-opus-4-6",
		},
	}); err != nil {
		t.Fatalf("write session_set_model: %v", err)
	}
	var setRes protocolws.Message
	if err := conn.ReadJSON(&setRes); err != nil {
		t.Fatalf("read session_set_model: %v", err)
	}
	if !setRes.OK {
		t.Fatalf("set model response = %#v, want ok", setRes)
	}

	if err := conn.WriteJSON(protocolws.Message{
		Type:   protocolws.TypeRequest,
		ID:     "3",
		Method: protocolws.MethodSessionStatus,
		Payload: map[string]any{
			"session_id": sessionID,
		},
	}); err != nil {
		t.Fatalf("write session status: %v", err)
	}
	var statusRes protocolws.Message
	if err := conn.ReadJSON(&statusRes); err != nil {
		t.Fatalf("read session status: %v", err)
	}
	if got := statusRes.Payload["session_main_loop_model_override"]; got != "claude-opus-4-6" {
		t.Fatalf("session model override = %#v, want override model", got)
	}
	if got := statusRes.Payload["resolved_main_loop_model"]; got != "claude-opus-4-6" {
		t.Fatalf("resolved main loop model = %#v, want resolved override model", got)
	}
}

func TestHandleWebSocketSessionSetModelAliasUpdatesResolvedModelState(t *testing.T) {
	sessionManager := session.NewManager(nil)
	server := NewServerWithOptions(log.New(io.Discard, "", 0), sessionManager, llm.NewMockClient(), Options{
		PermissionPolicy: permissions.Policy{Mode: permissions.ModeDangerFullAccess},
		MainLoopModel:    "claude-sonnet-4-6",
	})
	httpServer := httptest.NewServer(http.HandlerFunc(server.HandleWebSocket))
	t.Cleanup(httpServer.Close)

	wsURL := "ws" + strings.TrimPrefix(httpServer.URL, "http")
	conn, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
	if err != nil {
		t.Fatalf("dial websocket: %v", err)
	}
	t.Cleanup(func() { _ = conn.Close() })

	if err := conn.WriteJSON(protocolws.Message{
		Type:   protocolws.TypeRequest,
		ID:     "1",
		Method: protocolws.MethodConnect,
		Payload: map[string]any{
			"role":            "client",
			"client_identity": "web-ui",
			"agent_id":        "main",
		},
	}); err != nil {
		t.Fatalf("write connect: %v", err)
	}
	var connectRes protocolws.Message
	if err := conn.ReadJSON(&connectRes); err != nil {
		t.Fatalf("read connect: %v", err)
	}
	_ = conn.ReadJSON(&protocolws.Message{})
	sessionID, _ := connectRes.Payload["session_id"].(string)

	if err := conn.WriteJSON(protocolws.Message{
		Type:   protocolws.TypeRequest,
		ID:     "2",
		Method: protocolws.MethodSessionSetModel,
		Payload: map[string]any{
			"session_id": sessionID,
			"model":      "best",
		},
	}); err != nil {
		t.Fatalf("write session_set_model: %v", err)
	}
	var setRes protocolws.Message
	if err := conn.ReadJSON(&setRes); err != nil {
		t.Fatalf("read session_set_model: %v", err)
	}
	if !setRes.OK {
		t.Fatalf("set model response = %#v, want ok", setRes)
	}
	if got := setRes.Payload["session_main_loop_model_override"]; got != "best" {
		t.Fatalf("session model override = %#v, want raw alias override", got)
	}
	if got := setRes.Payload["resolved_main_loop_model"]; got != "claude-opus-4-6" {
		t.Fatalf("resolved main loop model = %#v, want resolved alias model", got)
	}
}

func TestHandleWebSocketSessionSetModelDefaultClearsOverride(t *testing.T) {
	sessionManager := session.NewManager(nil)
	server := NewServerWithOptions(log.New(io.Discard, "", 0), sessionManager, llm.NewMockClient(), Options{
		PermissionPolicy: permissions.Policy{Mode: permissions.ModeDangerFullAccess},
		MainLoopModel:    "claude-sonnet-4-6",
	})
	httpServer := httptest.NewServer(http.HandlerFunc(server.HandleWebSocket))
	t.Cleanup(httpServer.Close)

	wsURL := "ws" + strings.TrimPrefix(httpServer.URL, "http")
	conn, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
	if err != nil {
		t.Fatalf("dial websocket: %v", err)
	}
	t.Cleanup(func() { _ = conn.Close() })

	if err := conn.WriteJSON(protocolws.Message{
		Type:   protocolws.TypeRequest,
		ID:     "1",
		Method: protocolws.MethodConnect,
		Payload: map[string]any{
			"role":            "client",
			"client_identity": "web-ui",
			"agent_id":        "main",
		},
	}); err != nil {
		t.Fatalf("write connect: %v", err)
	}
	var connectRes protocolws.Message
	if err := conn.ReadJSON(&connectRes); err != nil {
		t.Fatalf("read connect response: %v", err)
	}
	_ = conn.ReadJSON(&protocolws.Message{})
	sessionID, _ := connectRes.Payload["session_id"].(string)

	if err := conn.WriteJSON(protocolws.Message{
		Type:   protocolws.TypeRequest,
		ID:     "2",
		Method: protocolws.MethodSessionSetModel,
		Payload: map[string]any{
			"session_id": sessionID,
			"model":      "claude-opus-4-6",
		},
	}); err != nil {
		t.Fatalf("write session_set_model override: %v", err)
	}
	var setOverrideRes protocolws.Message
	if err := conn.ReadJSON(&setOverrideRes); err != nil {
		t.Fatalf("read session_set_model override: %v", err)
	}
	if !setOverrideRes.OK {
		t.Fatalf("set override response = %#v, want ok", setOverrideRes)
	}

	if err := conn.WriteJSON(protocolws.Message{
		Type:   protocolws.TypeRequest,
		ID:     "3",
		Method: protocolws.MethodSessionSetModel,
		Payload: map[string]any{
			"session_id": sessionID,
			"model":      "default",
		},
	}); err != nil {
		t.Fatalf("write session_set_model default: %v", err)
	}
	var clearRes protocolws.Message
	if err := conn.ReadJSON(&clearRes); err != nil {
		t.Fatalf("read session_set_model default: %v", err)
	}
	if !clearRes.OK {
		t.Fatalf("clear model response = %#v, want ok", clearRes)
	}

	if err := conn.WriteJSON(protocolws.Message{
		Type:   protocolws.TypeRequest,
		ID:     "4",
		Method: protocolws.MethodSessionStatus,
		Payload: map[string]any{
			"session_id": sessionID,
		},
	}); err != nil {
		t.Fatalf("write session status: %v", err)
	}
	var statusRes protocolws.Message
	if err := conn.ReadJSON(&statusRes); err != nil {
		t.Fatalf("read session status: %v", err)
	}
	if got := statusRes.Payload["session_main_loop_model_override"]; got != "" {
		t.Fatalf("session model override = %#v, want cleared override", got)
	}
	if got := statusRes.Payload["resolved_main_loop_model"]; got != "claude-sonnet-4-6" {
		t.Fatalf("resolved main loop model = %#v, want fallback base model", got)
	}
}

func TestHandleWebSocketSessionSetModelAffectsSubsequentQueryRequests(t *testing.T) {
	sessionManager := session.NewManager(nil)
	client := &captureModelClient{}
	server := NewServerWithOptions(log.New(io.Discard, "", 0), sessionManager, client, Options{
		PermissionPolicy: permissions.Policy{Mode: permissions.ModeDangerFullAccess},
		MainLoopModel:    "claude-sonnet-4-6",
	})
	httpServer := httptest.NewServer(http.HandlerFunc(server.HandleWebSocket))
	t.Cleanup(httpServer.Close)

	wsURL := "ws" + strings.TrimPrefix(httpServer.URL, "http")
	conn, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
	if err != nil {
		t.Fatalf("dial websocket: %v", err)
	}
	t.Cleanup(func() { _ = conn.Close() })

	if err := conn.WriteJSON(protocolws.Message{
		Type:   protocolws.TypeRequest,
		ID:     "1",
		Method: protocolws.MethodConnect,
		Payload: map[string]any{
			"role":            "client",
			"client_identity": "web-ui",
			"agent_id":        "main",
		},
	}); err != nil {
		t.Fatalf("write connect: %v", err)
	}
	var connectRes protocolws.Message
	if err := conn.ReadJSON(&connectRes); err != nil {
		t.Fatalf("read connect response: %v", err)
	}
	_ = conn.ReadJSON(&protocolws.Message{})
	sessionID, _ := connectRes.Payload["session_id"].(string)

	if err := conn.WriteJSON(protocolws.Message{
		Type:   protocolws.TypeRequest,
		ID:     "2",
		Method: protocolws.MethodSessionSetModel,
		Payload: map[string]any{
			"session_id": sessionID,
			"model":      "claude-opus-4-6",
		},
	}); err != nil {
		t.Fatalf("write session_set_model: %v", err)
	}
	var setRes protocolws.Message
	if err := conn.ReadJSON(&setRes); err != nil {
		t.Fatalf("read session_set_model: %v", err)
	}
	if !setRes.OK {
		t.Fatalf("set model response = %#v, want ok", setRes)
	}

	if err := conn.WriteJSON(protocolws.Message{
		Type:   protocolws.TypeRequest,
		ID:     "3",
		Method: protocolws.MethodSendMessage,
		Payload: map[string]any{
			"content": "hello",
		},
	}); err != nil {
		t.Fatalf("write send_message: %v", err)
	}

	for i := 0; i < 12; i++ {
		var event protocolws.Message
		if err := conn.ReadJSON(&event); err != nil {
			t.Fatalf("read event %d: %v", i, err)
		}
		if event.Type == protocolws.TypeEvent && event.Event == "agent.lifecycle.end" {
			break
		}
	}

	if client.lastRequest.Model != "claude-opus-4-6" {
		t.Fatalf("request model = %q, want session-set model override", client.lastRequest.Model)
	}
}

func TestHandleWebSocketSessionSetModelDefaultRestoresBaseModelForQueries(t *testing.T) {
	sessionManager := session.NewManager(nil)
	client := &captureModelClient{}
	server := NewServerWithOptions(log.New(io.Discard, "", 0), sessionManager, client, Options{
		PermissionPolicy: permissions.Policy{Mode: permissions.ModeDangerFullAccess},
		MainLoopModel:    "claude-sonnet-4-6",
	})
	httpServer := httptest.NewServer(http.HandlerFunc(server.HandleWebSocket))
	t.Cleanup(httpServer.Close)

	wsURL := "ws" + strings.TrimPrefix(httpServer.URL, "http")
	conn, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
	if err != nil {
		t.Fatalf("dial websocket: %v", err)
	}
	t.Cleanup(func() { _ = conn.Close() })

	if err := conn.WriteJSON(protocolws.Message{
		Type:   protocolws.TypeRequest,
		ID:     "1",
		Method: protocolws.MethodConnect,
		Payload: map[string]any{
			"role":            "client",
			"client_identity": "web-ui",
			"agent_id":        "main",
		},
	}); err != nil {
		t.Fatalf("write connect: %v", err)
	}
	var connectRes protocolws.Message
	if err := conn.ReadJSON(&connectRes); err != nil {
		t.Fatalf("read connect response: %v", err)
	}
	_ = conn.ReadJSON(&protocolws.Message{})
	sessionID, _ := connectRes.Payload["session_id"].(string)

	for idx, model := range []string{"claude-opus-4-6", "default"} {
		if err := conn.WriteJSON(protocolws.Message{
			Type:   protocolws.TypeRequest,
			ID:     string(rune('2' + idx)),
			Method: protocolws.MethodSessionSetModel,
			Payload: map[string]any{
				"session_id": sessionID,
				"model":      model,
			},
		}); err != nil {
			t.Fatalf("write session_set_model %d: %v", idx, err)
		}
		var setRes protocolws.Message
		if err := conn.ReadJSON(&setRes); err != nil {
			t.Fatalf("read session_set_model %d: %v", idx, err)
		}
		if !setRes.OK {
			t.Fatalf("set model response %d = %#v, want ok", idx, setRes)
		}
	}

	if err := conn.WriteJSON(protocolws.Message{
		Type:   protocolws.TypeRequest,
		ID:     "4",
		Method: protocolws.MethodSendMessage,
		Payload: map[string]any{
			"content": "hello",
		},
	}); err != nil {
		t.Fatalf("write send_message: %v", err)
	}

	for i := 0; i < 12; i++ {
		var event protocolws.Message
		if err := conn.ReadJSON(&event); err != nil {
			t.Fatalf("read event %d: %v", i, err)
		}
		if event.Type == protocolws.TypeEvent && event.Event == "agent.lifecycle.end" {
			break
		}
	}

	if client.lastRequest.Model != "claude-sonnet-4-6" {
		t.Fatalf("request model = %q, want base model after default reset", client.lastRequest.Model)
	}
}

func TestHandleWebSocketSessionSetPermissionCascadeUpdatesExistingSubagentStatus(t *testing.T) {
	sessionManager := session.NewManager(nil)
	server := NewServerWithOptions(log.New(io.Discard, "", 0), sessionManager, llm.NewMockClient(), Options{
		PermissionPolicy: permissions.Policy{
			Mode:         permissions.ModeDangerFullAccess,
			SubagentMode: permissions.ModeWorkspaceWrite,
		},
	})
	httpServer := httptest.NewServer(http.HandlerFunc(server.HandleWebSocket))
	t.Cleanup(httpServer.Close)

	wsURL := "ws" + strings.TrimPrefix(httpServer.URL, "http")
	conn, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
	if err != nil {
		t.Fatalf("dial websocket: %v", err)
	}
	t.Cleanup(func() { _ = conn.Close() })

	if err := conn.WriteJSON(protocolws.Message{
		Type:   protocolws.TypeRequest,
		ID:     "1",
		Method: protocolws.MethodConnect,
		Payload: map[string]any{
			"role":            "client",
			"client_identity": "web-ui",
			"agent_id":        "main",
		},
	}); err != nil {
		t.Fatalf("write connect: %v", err)
	}
	var connectRes protocolws.Message
	if err := conn.ReadJSON(&connectRes); err != nil {
		t.Fatalf("read connect response: %v", err)
	}
	_ = conn.ReadJSON(&protocolws.Message{})
	sessionID, _ := connectRes.Payload["session_id"].(string)

	if err := conn.WriteJSON(protocolws.Message{
		Type:   protocolws.TypeRequest,
		ID:     "2",
		Method: protocolws.MethodSpawnSubagent,
		Payload: map[string]any{
			"label":  "child",
			"prompt": "hello child",
		},
	}); err != nil {
		t.Fatalf("write spawn request: %v", err)
	}
	var spawnAck protocolws.Message
	if err := conn.ReadJSON(&spawnAck); err != nil {
		t.Fatalf("read spawn ack: %v", err)
	}
	childSessionID, _ := spawnAck.Payload["child_session_id"].(string)
	if childSessionID == "" {
		t.Fatalf("spawn ack payload = %#v, want child session id", spawnAck.Payload)
	}

	if err := conn.WriteJSON(protocolws.Message{
		Type:   protocolws.TypeRequest,
		ID:     "3",
		Method: protocolws.MethodSessionSetPermission,
		Payload: map[string]any{
			"session_id":        sessionID,
			"mode":              "ask",
			"cascade_subagents": true,
		},
	}); err != nil {
		t.Fatalf("write session_set_permission: %v", err)
	}
	var setRes protocolws.Message
	for i := 0; i < 8; i++ {
		if err := conn.ReadJSON(&setRes); err != nil {
			t.Fatalf("read session_set_permission response %d: %v", i, err)
		}
		if setRes.Type == protocolws.TypeResponse && setRes.ID == "3" {
			break
		}
	}
	if !setRes.OK {
		t.Fatalf("set permission response = %#v, want ok", setRes)
	}

	if err := conn.WriteJSON(protocolws.Message{
		Type:   protocolws.TypeRequest,
		ID:     "4",
		Method: protocolws.MethodSessionStatus,
		Payload: map[string]any{
			"session_id": childSessionID,
		},
	}); err != nil {
		t.Fatalf("write session status: %v", err)
	}
	var statusRes protocolws.Message
	for i := 0; i < 8; i++ {
		if err := conn.ReadJSON(&statusRes); err != nil {
			t.Fatalf("read status response %d: %v", i, err)
		}
		if statusRes.Type == protocolws.TypeResponse && statusRes.ID == "4" {
			break
		}
	}
	if got := statusRes.Payload["permission_mode"]; got != "ask" {
		t.Fatalf("child permission mode = %#v, want ask", got)
	}
}

func TestHandleWebSocketTasksAndSubagentsList(t *testing.T) {
	sessionManager := session.NewManager(nil)
	server := NewServer(log.New(io.Discard, "", 0), sessionManager, llm.NewMockClient())
	httpServer := httptest.NewServer(http.HandlerFunc(server.HandleWebSocket))
	t.Cleanup(httpServer.Close)

	wsURL := "ws" + strings.TrimPrefix(httpServer.URL, "http")
	conn, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
	if err != nil {
		t.Fatalf("dial websocket: %v", err)
	}
	t.Cleanup(func() { _ = conn.Close() })

	if err := conn.WriteJSON(protocolws.Message{
		Type:   protocolws.TypeRequest,
		ID:     "1",
		Method: protocolws.MethodConnect,
		Payload: map[string]any{
			"role":            "client",
			"client_identity": "web-ui",
			"agent_id":        "main",
		},
	}); err != nil {
		t.Fatalf("write connect: %v", err)
	}
	_ = conn.ReadJSON(&protocolws.Message{})
	_ = conn.ReadJSON(&protocolws.Message{})

	if err := conn.WriteJSON(protocolws.Message{
		Type:   protocolws.TypeRequest,
		ID:     "2",
		Method: protocolws.MethodSpawnSubagent,
		Payload: map[string]any{
			"label":  "research",
			"prompt": "tool upper hello subagent",
		},
	}); err != nil {
		t.Fatalf("write spawn request: %v", err)
	}
	_ = conn.ReadJSON(&protocolws.Message{})

	if err := conn.WriteJSON(protocolws.Message{
		Type:   protocolws.TypeRequest,
		ID:     "3",
		Method: protocolws.MethodTasksList,
	}); err != nil {
		t.Fatalf("write tasks list: %v", err)
	}
	var tasks protocolws.Message
	if err := conn.ReadJSON(&tasks); err != nil {
		t.Fatalf("read tasks list: %v", err)
	}
	if !tasks.OK {
		t.Fatalf("tasks response = %#v, want ok", tasks)
	}

	if err := conn.WriteJSON(protocolws.Message{
		Type:   protocolws.TypeRequest,
		ID:     "4",
		Method: protocolws.MethodSubagentList,
	}); err != nil {
		t.Fatalf("write subagent list: %v", err)
	}
	var subagents protocolws.Message
	for i := 0; i < 4; i++ {
		if err := conn.ReadJSON(&subagents); err != nil {
			t.Fatalf("read subagent list message %d: %v", i, err)
		}
		if subagents.Type == protocolws.TypeResponse && subagents.ID == "4" {
			break
		}
	}
	if !subagents.OK {
		t.Fatalf("subagents response = %#v, want ok", subagents)
	}
}

func TestHandleWebSocketSubagentStop(t *testing.T) {
	sessionManager := session.NewManager(nil)
	server := NewServer(log.New(io.Discard, "", 0), sessionManager, llm.NewMockClient())
	httpServer := httptest.NewServer(http.HandlerFunc(server.HandleWebSocket))
	t.Cleanup(httpServer.Close)

	wsURL := "ws" + strings.TrimPrefix(httpServer.URL, "http")
	conn, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
	if err != nil {
		t.Fatalf("dial websocket: %v", err)
	}
	t.Cleanup(func() { _ = conn.Close() })

	if err := conn.WriteJSON(protocolws.Message{
		Type:   protocolws.TypeRequest,
		ID:     "1",
		Method: protocolws.MethodConnect,
		Payload: map[string]any{
			"role":            "client",
			"client_identity": "web-ui",
			"agent_id":        "main",
		},
	}); err != nil {
		t.Fatalf("write connect: %v", err)
	}
	_ = conn.ReadJSON(&protocolws.Message{})
	_ = conn.ReadJSON(&protocolws.Message{})

	run, err := server.runner.AgentManager().Spawn(context.Background(), agent.SpawnRequest{
		ParentSessionID: "main-000001",
		ParentAgentID:   "main",
		Label:           "blocking",
		Prompt:          "stay alive",
		Run: func(ctx context.Context, _ agent.RunContext) (string, error) {
			<-ctx.Done()
			return "", ctx.Err()
		},
	})
	if err != nil {
		t.Fatalf("spawn direct run: %v", err)
	}

	if err := conn.WriteJSON(protocolws.Message{
		Type:   protocolws.TypeRequest,
		ID:     "2",
		Method: protocolws.MethodSubagentStop,
		Payload: map[string]any{
			"run_id": run.ID,
		},
	}); err != nil {
		t.Fatalf("write subagent stop: %v", err)
	}
	var stop protocolws.Message
	if err := conn.ReadJSON(&stop); err != nil {
		t.Fatalf("read stop response: %v", err)
	}
	if !stop.OK {
		t.Fatalf("stop response = %#v, want ok", stop)
	}
}

func TestHandleWebSocketMemoryList(t *testing.T) {
	sessionManager := session.NewManager(nil)
	server := NewServer(log.New(io.Discard, "", 0), sessionManager, llm.NewMockClient())
	sess := sessionManager.GetOrCreateMain("main")
	server.runner.MemoryService().SaveCompactionSummary(sess, session.Message{
		ID:        "summary-1",
		SessionID: sess.ID,
		Role:      "summary",
		Content:   "Summary: remembered fact",
	})

	httpServer := httptest.NewServer(http.HandlerFunc(server.HandleWebSocket))
	t.Cleanup(httpServer.Close)

	wsURL := "ws" + strings.TrimPrefix(httpServer.URL, "http")
	conn, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
	if err != nil {
		t.Fatalf("dial websocket: %v", err)
	}
	t.Cleanup(func() { _ = conn.Close() })

	if err := conn.WriteJSON(protocolws.Message{
		Type:   protocolws.TypeRequest,
		ID:     "1",
		Method: protocolws.MethodConnect,
		Payload: map[string]any{
			"role":            "client",
			"client_identity": "web-ui",
			"agent_id":        "main",
		},
	}); err != nil {
		t.Fatalf("write connect: %v", err)
	}
	_ = conn.ReadJSON(&protocolws.Message{})
	_ = conn.ReadJSON(&protocolws.Message{})

	if err := conn.WriteJSON(protocolws.Message{
		Type:   protocolws.TypeRequest,
		ID:     "2",
		Method: protocolws.MethodMemoryList,
	}); err != nil {
		t.Fatalf("write memory list: %v", err)
	}
	var res protocolws.Message
	if err := conn.ReadJSON(&res); err != nil {
		t.Fatalf("read memory list: %v", err)
	}
	if !res.OK {
		t.Fatalf("memory list response = %#v, want ok", res)
	}
	memories, ok := res.Payload["memories"].([]any)
	if !ok || len(memories) == 0 {
		t.Fatalf("memories payload = %#v, want non-empty list", res.Payload["memories"])
	}
	first, ok := memories[0].(map[string]any)
	if !ok {
		t.Fatalf("first memory = %#v, want object", memories[0])
	}
	if got := first["type"]; got != "summary" {
		t.Fatalf("memory type = %#v, want %q", got, "summary")
	}
}

func TestHandleWebSocketSubagentSteer(t *testing.T) {
	sessionManager := session.NewManager(nil)
	server := NewServer(log.New(io.Discard, "", 0), sessionManager, llm.NewMockClient())
	httpServer := httptest.NewServer(http.HandlerFunc(server.HandleWebSocket))
	t.Cleanup(httpServer.Close)

	wsURL := "ws" + strings.TrimPrefix(httpServer.URL, "http")
	conn, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
	if err != nil {
		t.Fatalf("dial websocket: %v", err)
	}
	t.Cleanup(func() { _ = conn.Close() })

	if err := conn.WriteJSON(protocolws.Message{
		Type:   protocolws.TypeRequest,
		ID:     "1",
		Method: protocolws.MethodConnect,
		Payload: map[string]any{
			"role":            "client",
			"client_identity": "web-ui",
			"agent_id":        "main",
		},
	}); err != nil {
		t.Fatalf("write connect: %v", err)
	}
	_ = conn.ReadJSON(&protocolws.Message{})
	_ = conn.ReadJSON(&protocolws.Message{})

	run, err := server.runner.AgentManager().Spawn(context.Background(), agent.SpawnRequest{
		ParentSessionID: "main-000001",
		ParentAgentID:   "main",
		Label:           "blocking",
		Prompt:          "stay alive",
		Run: func(ctx context.Context, runCtx agent.RunContext) (string, error) {
			for {
				select {
				case <-ctx.Done():
					return "", ctx.Err()
				default:
					controls := server.runner.AgentManager().ControlMessages(runCtx.RunID)
					if len(controls) > 0 {
						return controls[len(controls)-1], nil
					}
					time.Sleep(10 * time.Millisecond)
				}
			}
		},
	})
	if err != nil {
		t.Fatalf("spawn direct run: %v", err)
	}

	if err := conn.WriteJSON(protocolws.Message{
		Type:   protocolws.TypeRequest,
		ID:     "2",
		Method: protocolws.MethodSubagentSteer,
		Payload: map[string]any{
			"run_id":  run.ID,
			"message": "switch to safer plan",
		},
	}); err != nil {
		t.Fatalf("write subagent steer: %v", err)
	}
	var steer protocolws.Message
	if err := conn.ReadJSON(&steer); err != nil {
		t.Fatalf("read steer response: %v", err)
	}
	if !steer.OK {
		t.Fatalf("steer response = %#v, want ok", steer)
	}
}

func TestHandleWebSocketSubagentSteerEmitsUpdatedEvent(t *testing.T) {
	sessionManager := session.NewManager(nil)
	server := NewServer(log.New(io.Discard, "", 0), sessionManager, llm.NewMockClient())
	httpServer := httptest.NewServer(http.HandlerFunc(server.HandleWebSocket))
	t.Cleanup(httpServer.Close)

	wsURL := "ws" + strings.TrimPrefix(httpServer.URL, "http")
	conn, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
	if err != nil {
		t.Fatalf("dial websocket: %v", err)
	}
	t.Cleanup(func() { _ = conn.Close() })

	if err := conn.WriteJSON(protocolws.Message{
		Type:   protocolws.TypeRequest,
		ID:     "1",
		Method: protocolws.MethodConnect,
		Payload: map[string]any{
			"role":            "client",
			"client_identity": "web-ui",
			"agent_id":        "main",
		},
	}); err != nil {
		t.Fatalf("write connect: %v", err)
	}
	_ = conn.ReadJSON(&protocolws.Message{})
	_ = conn.ReadJSON(&protocolws.Message{})

	run, err := server.runner.AgentManager().Spawn(context.Background(), agent.SpawnRequest{
		ParentSessionID: "main-000001",
		ParentAgentID:   "main",
		Label:           "worker",
		Prompt:          "stay alive",
		Run: func(ctx context.Context, _ agent.RunContext) (string, error) {
			<-ctx.Done()
			return "", ctx.Err()
		},
	})
	if err != nil {
		t.Fatalf("spawn run: %v", err)
	}

	if err := conn.WriteJSON(protocolws.Message{
		Type:   protocolws.TypeRequest,
		ID:     "2",
		Method: protocolws.MethodSubagentSteer,
		Payload: map[string]any{
			"run_id":  run.ID,
			"message": "adjust plan",
		},
	}); err != nil {
		t.Fatalf("write subagent steer: %v", err)
	}
	var ack protocolws.Message
	if err := conn.ReadJSON(&ack); err != nil {
		t.Fatalf("read subagent steer ack: %v", err)
	}
	if !ack.OK {
		t.Fatalf("subagent steer ack = %#v, want ok", ack)
	}

	for i := 0; i < 4; i++ {
		var event protocolws.Message
		if err := conn.ReadJSON(&event); err != nil {
			t.Fatalf("read subagent updated event %d: %v", i, err)
		}
		if event.Type == protocolws.TypeEvent && event.Event == "subagent.updated" {
			if got := event.Payload["status"]; got != "running" {
				t.Fatalf("subagent.updated status = %#v, want running lifecycle state", got)
			}
			if got := event.Payload["last_action"]; got != "steered" {
				t.Fatalf("subagent.updated last_action = %#v, want steered", got)
			}
			return
		}
	}
	t.Fatal("expected subagent.updated event")
}

func TestHandleWebSocketSubagentSteerEmitsOrchestrationUpdatedEvent(t *testing.T) {
	sessionManager := session.NewManager(nil)
	server := NewServer(log.New(io.Discard, "", 0), sessionManager, llm.NewMockClient())
	httpServer := httptest.NewServer(http.HandlerFunc(server.HandleWebSocket))
	t.Cleanup(httpServer.Close)

	wsURL := "ws" + strings.TrimPrefix(httpServer.URL, "http")
	conn, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
	if err != nil {
		t.Fatalf("dial websocket: %v", err)
	}
	t.Cleanup(func() { _ = conn.Close() })

	if err := conn.WriteJSON(protocolws.Message{
		Type:   protocolws.TypeRequest,
		ID:     "1",
		Method: protocolws.MethodConnect,
		Payload: map[string]any{
			"role":            "client",
			"client_identity": "web-ui",
			"agent_id":        "main",
		},
	}); err != nil {
		t.Fatalf("write connect: %v", err)
	}
	_ = conn.ReadJSON(&protocolws.Message{})
	_ = conn.ReadJSON(&protocolws.Message{})

	run, err := server.runner.AgentManager().Spawn(context.Background(), agent.SpawnRequest{
		ParentSessionID: "main-000001",
		ParentAgentID:   "main",
		Label:           "worker",
		Prompt:          "stay alive",
		Run: func(ctx context.Context, _ agent.RunContext) (string, error) {
			<-ctx.Done()
			return "", ctx.Err()
		},
	})
	if err != nil {
		t.Fatalf("spawn run: %v", err)
	}

	if err := conn.WriteJSON(protocolws.Message{
		Type:   protocolws.TypeRequest,
		ID:     "2",
		Method: protocolws.MethodSubagentSteer,
		Payload: map[string]any{
			"run_id":  run.ID,
			"message": "adjust plan",
		},
	}); err != nil {
		t.Fatalf("write subagent steer: %v", err)
	}
	var ack protocolws.Message
	if err := conn.ReadJSON(&ack); err != nil {
		t.Fatalf("read subagent steer ack: %v", err)
	}
	if !ack.OK {
		t.Fatalf("subagent steer ack = %#v, want ok", ack)
	}

	foundUpdated := false
	for i := 0; i < 6; i++ {
		var event protocolws.Message
		if err := conn.ReadJSON(&event); err != nil {
			t.Fatalf("read orchestration event %d: %v", i, err)
		}
		if event.Type != protocolws.TypeEvent {
			continue
		}
		if event.Event == protocolws.EventSubagentUpdated {
			foundUpdated = true
			continue
		}
		if event.Event == protocolws.EventOrchestrationUpdated {
			if !foundUpdated {
				t.Fatal("expected subagent.updated before orchestration.updated")
			}
			if got := event.Payload["run_id"]; got != run.ID {
				t.Fatalf("orchestration.updated run_id = %#v, want %q", got, run.ID)
			}
			if got := event.Payload["status"]; got != "running" {
				t.Fatalf("orchestration.updated status = %#v, want running lifecycle state", got)
			}
			if got := event.Payload["last_action"]; got != "steered" {
				t.Fatalf("orchestration.updated last_action = %#v, want steered", got)
			}
			if got := event.Payload["recommended_action"]; got != "monitor_replanned_run" {
				t.Fatalf("orchestration.updated recommended_action = %#v, want monitor_replanned_run", got)
			}
			if got := event.Payload["decision_type"]; got != "monitor_replanned_run" {
				t.Fatalf("orchestration.updated decision_type = %#v, want monitor_replanned_run", got)
			}
			return
		}
	}
	t.Fatal("expected orchestration.updated event")
}

func TestHandleWebSocketSubagentStatusReturnsControlMessages(t *testing.T) {
	sessionManager := session.NewManager(nil)
	server := NewServer(log.New(io.Discard, "", 0), sessionManager, llm.NewMockClient())
	httpServer := httptest.NewServer(http.HandlerFunc(server.HandleWebSocket))
	t.Cleanup(httpServer.Close)

	wsURL := "ws" + strings.TrimPrefix(httpServer.URL, "http")
	conn, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
	if err != nil {
		t.Fatalf("dial websocket: %v", err)
	}
	t.Cleanup(func() { _ = conn.Close() })

	if err := conn.WriteJSON(protocolws.Message{
		Type:   protocolws.TypeRequest,
		ID:     "1",
		Method: protocolws.MethodConnect,
		Payload: map[string]any{
			"role":            "client",
			"client_identity": "web-ui",
			"agent_id":        "main",
		},
	}); err != nil {
		t.Fatalf("write connect: %v", err)
	}
	_ = conn.ReadJSON(&protocolws.Message{})
	_ = conn.ReadJSON(&protocolws.Message{})

	run, err := server.runner.AgentManager().Spawn(context.Background(), agent.SpawnRequest{
		ParentSessionID: "main-000001",
		ParentAgentID:   "main",
		Label:           "worker",
		Prompt:          "stay alive",
		Run: func(ctx context.Context, _ agent.RunContext) (string, error) {
			<-ctx.Done()
			return "", ctx.Err()
		},
	})
	if err != nil {
		t.Fatalf("spawn run: %v", err)
	}
	if err := server.runner.AgentManager().Steer(run.ID, "adjust plan"); err != nil {
		t.Fatalf("steer run: %v", err)
	}

	if err := conn.WriteJSON(protocolws.Message{
		Type:   protocolws.TypeRequest,
		ID:     "2",
		Method: protocolws.MethodSubagentStatus,
		Payload: map[string]any{
			"run_id": run.ID,
		},
	}); err != nil {
		t.Fatalf("write subagent_status: %v", err)
	}
	var res protocolws.Message
	if err := conn.ReadJSON(&res); err != nil {
		t.Fatalf("read subagent_status: %v", err)
	}
	if !res.OK {
		t.Fatalf("subagent status response = %#v, want ok", res)
	}
	if got := res.Payload["run_id"]; got != run.ID {
		t.Fatalf("run_id = %#v, want %q", got, run.ID)
	}
	messages, ok := res.Payload["control_messages"].([]any)
	if !ok || len(messages) != 1 || messages[0] != "adjust plan" {
		t.Fatalf("control_messages = %#v, want [adjust plan]", res.Payload["control_messages"])
	}
}

func TestHandleWebSocketSubagentUpdatedIsSentToOrchestrationHook(t *testing.T) {
	sessionManager := session.NewManager(nil)
	hook := &orchestrationHook{}
	server := NewServerWithOptions(log.New(io.Discard, "", 0), sessionManager, llm.NewMockClient(), Options{
		Orchestrator: hook,
	})
	httpServer := httptest.NewServer(http.HandlerFunc(server.HandleWebSocket))
	t.Cleanup(httpServer.Close)

	wsURL := "ws" + strings.TrimPrefix(httpServer.URL, "http")
	conn, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
	if err != nil {
		t.Fatalf("dial websocket: %v", err)
	}
	t.Cleanup(func() { _ = conn.Close() })

	if err := conn.WriteJSON(protocolws.Message{
		Type:   protocolws.TypeRequest,
		ID:     "1",
		Method: protocolws.MethodConnect,
		Payload: map[string]any{
			"role":            "client",
			"client_identity": "web-ui",
			"agent_id":        "main",
		},
	}); err != nil {
		t.Fatalf("write connect: %v", err)
	}
	_ = conn.ReadJSON(&protocolws.Message{})
	_ = conn.ReadJSON(&protocolws.Message{})

	run, err := server.runner.AgentManager().Spawn(context.Background(), agent.SpawnRequest{
		ParentSessionID: "main-000001",
		ParentAgentID:   "main",
		Label:           "worker",
		Prompt:          "stay alive",
		Run: func(ctx context.Context, _ agent.RunContext) (string, error) {
			<-ctx.Done()
			return "", ctx.Err()
		},
	})
	if err != nil {
		t.Fatalf("spawn run: %v", err)
	}

	if err := conn.WriteJSON(protocolws.Message{
		Type:   protocolws.TypeRequest,
		ID:     "2",
		Method: protocolws.MethodSubagentSteer,
		Payload: map[string]any{
			"run_id":  run.ID,
			"message": "adjust plan",
		},
	}); err != nil {
		t.Fatalf("write subagent steer: %v", err)
	}
	var ack protocolws.Message
	if err := conn.ReadJSON(&ack); err != nil {
		t.Fatalf("read subagent steer ack: %v", err)
	}
	if !ack.OK {
		t.Fatalf("subagent steer ack = %#v, want ok", ack)
	}
	for i := 0; i < 4; i++ {
		var event protocolws.Message
		if err := conn.ReadJSON(&event); err != nil {
			t.Fatalf("read subagent updated event %d: %v", i, err)
		}
		if event.Type == protocolws.TypeEvent && event.Event == "subagent.updated" {
			break
		}
	}

	found := false
	for _, event := range hook.Events() {
		if event.Type == "subagent.updated" && event.RunID == run.ID && event.Status == "running" && event.Action == "steered" {
			found = true
			break
		}
	}
	if !found {
		deadline := time.Now().Add(500 * time.Millisecond)
		for time.Now().Before(deadline) {
			for _, event := range hook.Events() {
				if event.Type == "subagent.updated" && event.RunID == run.ID && event.Status == "running" && event.Action == "steered" {
					found = true
					break
				}
			}
			if found {
				break
			}
			time.Sleep(10 * time.Millisecond)
		}
	}
	if !found {
		t.Fatalf("expected subagent.updated orchestration hook event, got %#v", hook.Events())
	}
}

func TestHandleWebSocketOrchestrationStatusReturnsTrackedRuns(t *testing.T) {
	sessionManager := session.NewManager(nil)
	server := NewServer(log.New(io.Discard, "", 0), sessionManager, llm.NewMockClient())
	httpServer := httptest.NewServer(http.HandlerFunc(server.HandleWebSocket))
	t.Cleanup(httpServer.Close)

	wsURL := "ws" + strings.TrimPrefix(httpServer.URL, "http")
	conn, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
	if err != nil {
		t.Fatalf("dial websocket: %v", err)
	}
	t.Cleanup(func() { _ = conn.Close() })

	if err := conn.WriteJSON(protocolws.Message{
		Type:   protocolws.TypeRequest,
		ID:     "1",
		Method: protocolws.MethodConnect,
		Payload: map[string]any{
			"role":            "client",
			"client_identity": "web-ui",
			"agent_id":        "main",
		},
	}); err != nil {
		t.Fatalf("write connect: %v", err)
	}
	_ = conn.ReadJSON(&protocolws.Message{})
	_ = conn.ReadJSON(&protocolws.Message{})

	if err := conn.WriteJSON(protocolws.Message{
		Type:   protocolws.TypeRequest,
		ID:     "2",
		Method: protocolws.MethodSendMessage,
		Payload: map[string]any{
			"content": "tool upper hello world",
		},
	}); err != nil {
		t.Fatalf("write send_message: %v", err)
	}
	for i := 0; i < 32; i++ {
		var event protocolws.Message
		if err := conn.ReadJSON(&event); err != nil {
			t.Fatalf("read runtime event %d: %v", i, err)
		}
		if event.Type == protocolws.TypeEvent && event.Event == "agent.lifecycle.end" {
			break
		}
	}

	if err := conn.WriteJSON(protocolws.Message{
		Type:   protocolws.TypeRequest,
		ID:     "3",
		Method: protocolws.MethodOrchestrationStatus,
	}); err != nil {
		t.Fatalf("write orchestration_status: %v", err)
	}
	var res protocolws.Message
	if err := conn.ReadJSON(&res); err != nil {
		t.Fatalf("read orchestration_status: %v", err)
	}
	if !res.OK {
		t.Fatalf("orchestration_status response = %#v, want ok", res)
	}
	runs, ok := res.Payload["runs"].([]any)
	if !ok || len(runs) == 0 {
		t.Fatalf("runs payload = %#v, want non-empty list", res.Payload["runs"])
	}
	first, ok := runs[0].(map[string]any)
	if !ok {
		t.Fatalf("first run = %#v, want object", runs[0])
	}
	if got := first["status"]; got == nil || got == "" {
		t.Fatalf("run status = %#v, want populated status", got)
	}
	if got := first["dispatcher_state"]; got == nil || got == "" {
		t.Fatalf("dispatcher_state = %#v, want populated value", got)
	}
	if got := first["next_action"]; got == nil || got == "" {
		t.Fatalf("next_action = %#v, want populated value", got)
	}
	if got := first["recommended_role"]; got == nil || got == "" {
		t.Fatalf("recommended_role = %#v, want populated value", got)
	}
	if got := first["recommended_action"]; got == nil || got == "" {
		t.Fatalf("recommended_action = %#v, want populated value", got)
	}
	if got := first["decision_type"]; got == nil || got == "" {
		t.Fatalf("decision_type = %#v, want populated value", got)
	}
	if got := first["decision_reason"]; got == nil || got == "" {
		t.Fatalf("decision_reason = %#v, want populated value", got)
	}
	if got := first["decision_priority"]; got == nil || got == "" {
		t.Fatalf("decision_priority = %#v, want populated value", got)
	}
}

func TestHandleWebSocketOrchestrationHistoryReturnsDecisionRecords(t *testing.T) {
	sessionManager := session.NewManager(nil)
	server := NewServer(log.New(io.Discard, "", 0), sessionManager, llm.NewMockClient())
	httpServer := httptest.NewServer(http.HandlerFunc(server.HandleWebSocket))
	t.Cleanup(httpServer.Close)

	wsURL := "ws" + strings.TrimPrefix(httpServer.URL, "http")
	conn, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
	if err != nil {
		t.Fatalf("dial websocket: %v", err)
	}
	t.Cleanup(func() { _ = conn.Close() })

	if err := conn.WriteJSON(protocolws.Message{
		Type:   protocolws.TypeRequest,
		ID:     "1",
		Method: protocolws.MethodConnect,
		Payload: map[string]any{
			"role":            "client",
			"client_identity": "web-ui",
			"agent_id":        "main",
		},
	}); err != nil {
		t.Fatalf("write connect: %v", err)
	}
	_ = conn.ReadJSON(&protocolws.Message{})
	_ = conn.ReadJSON(&protocolws.Message{})

	if err := conn.WriteJSON(protocolws.Message{
		Type:   protocolws.TypeRequest,
		ID:     "2",
		Method: protocolws.MethodSendMessage,
		Payload: map[string]any{
			"content": "tool upper hello world",
		},
	}); err != nil {
		t.Fatalf("write send_message: %v", err)
	}

	var runID string
	for i := 0; i < 32; i++ {
		var event protocolws.Message
		if err := conn.ReadJSON(&event); err != nil {
			t.Fatalf("read runtime event %d: %v", i, err)
		}
		if event.Type == protocolws.TypeEvent {
			if id, _ := event.Payload["run_id"].(string); id != "" {
				runID = id
			}
		}
		if event.Type == protocolws.TypeEvent && event.Event == "agent.lifecycle.end" {
			break
		}
	}
	if runID == "" {
		t.Fatal("expected run id from runtime events")
	}

	if err := conn.WriteJSON(protocolws.Message{
		Type:   protocolws.TypeRequest,
		ID:     "3",
		Method: protocolws.MethodOrchestrationHistory,
		Payload: map[string]any{
			"run_id": runID,
		},
	}); err != nil {
		t.Fatalf("write orchestration_history: %v", err)
	}
	var res protocolws.Message
	if err := conn.ReadJSON(&res); err != nil {
		t.Fatalf("read orchestration_history: %v", err)
	}
	if !res.OK {
		t.Fatalf("orchestration_history response = %#v, want ok", res)
	}
	records, ok := res.Payload["history"].([]any)
	if !ok || len(records) == 0 {
		t.Fatalf("history payload = %#v, want non-empty list", res.Payload["history"])
	}
	first, ok := records[0].(map[string]any)
	if !ok {
		t.Fatalf("first history record = %#v, want object", records[0])
	}
	if got := first["event_type"]; got == nil || got == "" {
		t.Fatalf("event_type = %#v, want populated value", got)
	}
	if got := first["decision_type"]; got == nil || got == "" {
		t.Fatalf("decision_type = %#v, want populated value", got)
	}
	if got := first["decision_priority"]; got == nil || got == "" {
		t.Fatalf("decision_priority = %#v, want populated value", got)
	}
}

func TestHandleWebSocketOrchestrationHistoryCanFilterRecords(t *testing.T) {
	sessionManager := session.NewManager(nil)
	server := NewServer(log.New(io.Discard, "", 0), sessionManager, llm.NewMockClient())
	httpServer := httptest.NewServer(http.HandlerFunc(server.HandleWebSocket))
	t.Cleanup(httpServer.Close)

	wsURL := "ws" + strings.TrimPrefix(httpServer.URL, "http")
	conn, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
	if err != nil {
		t.Fatalf("dial websocket: %v", err)
	}
	t.Cleanup(func() { _ = conn.Close() })

	if err := conn.WriteJSON(protocolws.Message{
		Type:   protocolws.TypeRequest,
		ID:     "1",
		Method: protocolws.MethodConnect,
		Payload: map[string]any{
			"role":            "client",
			"client_identity": "web-ui",
			"agent_id":        "main",
		},
	}); err != nil {
		t.Fatalf("write connect: %v", err)
	}
	_ = conn.ReadJSON(&protocolws.Message{})
	_ = conn.ReadJSON(&protocolws.Message{})

	if err := conn.WriteJSON(protocolws.Message{
		Type:   protocolws.TypeRequest,
		ID:     "2",
		Method: protocolws.MethodSendMessage,
		Payload: map[string]any{
			"content": "tool upper hello world",
		},
	}); err != nil {
		t.Fatalf("write send_message: %v", err)
	}

	var runID string
	for i := 0; i < 32; i++ {
		var event protocolws.Message
		if err := conn.ReadJSON(&event); err != nil {
			t.Fatalf("read runtime event %d: %v", i, err)
		}
		if event.Type == protocolws.TypeEvent {
			if id, _ := event.Payload["run_id"].(string); id != "" {
				runID = id
			}
		}
		if event.Type == protocolws.TypeEvent && event.Event == "agent.lifecycle.end" {
			break
		}
	}
	if runID == "" {
		t.Fatal("expected run id from runtime events")
	}

	if err := conn.WriteJSON(protocolws.Message{
		Type:   protocolws.TypeRequest,
		ID:     "3",
		Method: protocolws.MethodOrchestrationHistory,
		Payload: map[string]any{
			"run_id":            runID,
			"decision_priority": "medium",
			"status":            "running_tool",
		},
	}); err != nil {
		t.Fatalf("write filtered orchestration_history: %v", err)
	}
	var res protocolws.Message
	if err := conn.ReadJSON(&res); err != nil {
		t.Fatalf("read filtered orchestration_history: %v", err)
	}
	if !res.OK {
		t.Fatalf("filtered orchestration_history response = %#v, want ok", res)
	}
	records, ok := res.Payload["history"].([]any)
	if !ok || len(records) != 1 {
		t.Fatalf("filtered history payload = %#v, want one item", res.Payload["history"])
	}
	first, ok := records[0].(map[string]any)
	if !ok {
		t.Fatalf("first filtered history record = %#v, want object", records[0])
	}
	if got := first["status"]; got != "running_tool" {
		t.Fatalf("filtered history status = %#v, want running_tool", got)
	}
	if got := first["decision_priority"]; got != "medium" {
		t.Fatalf("filtered history priority = %#v, want medium", got)
	}
	summary, ok := res.Payload["summary"].(map[string]any)
	if !ok {
		t.Fatalf("summary payload = %#v, want object", res.Payload["summary"])
	}
	if got := summary["record_count"]; got != float64(1) {
		t.Fatalf("summary record_count = %#v, want 1", got)
	}
}

func TestHandleWebSocketOrchestrationSummaryAggregatesSessionState(t *testing.T) {
	sessionManager := session.NewManager(nil)
	server := NewServer(log.New(io.Discard, "", 0), sessionManager, llm.NewMockClient())
	httpServer := httptest.NewServer(http.HandlerFunc(server.HandleWebSocket))
	t.Cleanup(httpServer.Close)

	wsURL := "ws" + strings.TrimPrefix(httpServer.URL, "http")
	conn, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
	if err != nil {
		t.Fatalf("dial websocket: %v", err)
	}
	t.Cleanup(func() { _ = conn.Close() })

	if err := conn.WriteJSON(protocolws.Message{
		Type:   protocolws.TypeRequest,
		ID:     "1",
		Method: protocolws.MethodConnect,
		Payload: map[string]any{
			"role":            "client",
			"client_identity": "web-ui",
			"agent_id":        "main",
		},
	}); err != nil {
		t.Fatalf("write connect: %v", err)
	}
	_ = conn.ReadJSON(&protocolws.Message{})
	_ = conn.ReadJSON(&protocolws.Message{})

	if err := conn.WriteJSON(protocolws.Message{
		Type:   protocolws.TypeRequest,
		ID:     "2",
		Method: protocolws.MethodSendMessage,
		Payload: map[string]any{
			"content": "tool upper hello world",
		},
	}); err != nil {
		t.Fatalf("write tool run: %v", err)
	}
	for i := 0; i < 32; i++ {
		var event protocolws.Message
		if err := conn.ReadJSON(&event); err != nil {
			t.Fatalf("read tool runtime event %d: %v", i, err)
		}
		if event.Type == protocolws.TypeEvent && event.Event == "agent.lifecycle.end" {
			break
		}
	}

	if err := conn.WriteJSON(protocolws.Message{
		Type:   protocolws.TypeRequest,
		ID:     "3",
		Method: protocolws.MethodSendMessage,
		Payload: map[string]any{
			"content": "hello",
		},
	}); err != nil {
		t.Fatalf("write plain run: %v", err)
	}
	for i := 0; i < 32; i++ {
		var event protocolws.Message
		if err := conn.ReadJSON(&event); err != nil {
			t.Fatalf("read plain runtime event %d: %v", i, err)
		}
		if event.Type == protocolws.TypeEvent && event.Event == "agent.lifecycle.end" {
			break
		}
	}

	if err := conn.WriteJSON(protocolws.Message{
		Type:   protocolws.TypeRequest,
		ID:     "4",
		Method: protocolws.MethodOrchestrationSummary,
	}); err != nil {
		t.Fatalf("write orchestration_summary: %v", err)
	}
	var res protocolws.Message
	if err := conn.ReadJSON(&res); err != nil {
		t.Fatalf("read orchestration_summary: %v", err)
	}
	if !res.OK {
		t.Fatalf("orchestration_summary response = %#v, want ok", res)
	}
	if got := res.Payload["run_count"]; got != float64(2) {
		t.Fatalf("run_count = %#v, want 2", got)
	}
	statusCounts, ok := res.Payload["status_counts"].(map[string]any)
	if !ok {
		t.Fatalf("status_counts = %#v, want object", res.Payload["status_counts"])
	}
	if got := statusCounts["completed"]; got != float64(2) {
		t.Fatalf("completed count = %#v, want 2", got)
	}
	priorityCounts, ok := res.Payload["priority_counts"].(map[string]any)
	if !ok {
		t.Fatalf("priority_counts = %#v, want object", res.Payload["priority_counts"])
	}
	if got := priorityCounts["low"]; got != float64(2) {
		t.Fatalf("low priority count = %#v, want 2", got)
	}
	recommendedCounts, ok := res.Payload["recommended_action_counts"].(map[string]any)
	if !ok {
		t.Fatalf("recommended_action_counts = %#v, want object", res.Payload["recommended_action_counts"])
	}
	if got := recommendedCounts["close_or_follow_up"]; got != float64(2) {
		t.Fatalf("close_or_follow_up count = %#v, want 2", got)
	}
	if got := res.Payload["top_priority"]; got != "low" {
		t.Fatalf("top_priority = %#v, want low", got)
	}
}

func TestHandleWebSocketOrchestrationEvaluateReturnsSuggestions(t *testing.T) {
	sessionManager := session.NewManager(nil)
	server := NewServerWithOptions(log.New(io.Discard, "", 0), sessionManager, llm.NewMockClient(), Options{
		PermissionPolicy: permissions.Policy{Mode: permissions.ModeAsk},
	})
	httpServer := httptest.NewServer(http.HandlerFunc(server.HandleWebSocket))
	t.Cleanup(httpServer.Close)

	wsURL := "ws" + strings.TrimPrefix(httpServer.URL, "http")
	conn, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
	if err != nil {
		t.Fatalf("dial websocket: %v", err)
	}
	t.Cleanup(func() { _ = conn.Close() })

	if err := conn.WriteJSON(protocolws.Message{
		Type:   protocolws.TypeRequest,
		ID:     "1",
		Method: protocolws.MethodConnect,
		Payload: map[string]any{
			"role":            "client",
			"client_identity": "web-ui",
			"agent_id":        "main",
		},
	}); err != nil {
		t.Fatalf("write connect: %v", err)
	}
	_ = conn.ReadJSON(&protocolws.Message{})
	_ = conn.ReadJSON(&protocolws.Message{})

	if err := conn.WriteJSON(protocolws.Message{
		Type:   protocolws.TypeRequest,
		ID:     "2",
		Method: protocolws.MethodSendMessage,
		Payload: map[string]any{
			"content": "tool run pwd",
		},
	}); err != nil {
		t.Fatalf("write approval-producing message: %v", err)
	}
	_ = waitForPermissionRequired(t, conn)

	if err := conn.WriteJSON(protocolws.Message{
		Type:   protocolws.TypeRequest,
		ID:     "3",
		Method: protocolws.MethodSendMessage,
		Payload: map[string]any{
			"content": "hello",
		},
	}); err != nil {
		t.Fatalf("write plain message: %v", err)
	}
	for i := 0; i < 32; i++ {
		var event protocolws.Message
		if err := conn.ReadJSON(&event); err != nil {
			t.Fatalf("read plain runtime event %d: %v", i, err)
		}
		if event.Type == protocolws.TypeEvent && event.Event == "agent.lifecycle.end" {
			break
		}
	}

	if err := conn.WriteJSON(protocolws.Message{
		Type:   protocolws.TypeRequest,
		ID:     "4",
		Method: protocolws.MethodOrchestrationEvaluate,
	}); err != nil {
		t.Fatalf("write orchestration_evaluate: %v", err)
	}
	var res protocolws.Message
	if err := conn.ReadJSON(&res); err != nil {
		t.Fatalf("read orchestration_evaluate: %v", err)
	}
	if !res.OK {
		t.Fatalf("orchestration_evaluate response = %#v, want ok", res)
	}
	suggestions, ok := res.Payload["suggestions"].([]any)
	if !ok || len(suggestions) < 2 {
		t.Fatalf("suggestions payload = %#v, want at least two items", res.Payload["suggestions"])
	}
	first, ok := suggestions[0].(map[string]any)
	if !ok {
		t.Fatalf("first suggestion = %#v, want object", suggestions[0])
	}
	if got := first["category"]; got != "approval" {
		t.Fatalf("first suggestion category = %#v, want approval", got)
	}
	if got := first["suggested_action"]; got != "request_human_approval" {
		t.Fatalf("first suggestion action = %#v, want request_human_approval", got)
	}
	if got := first["blocking"]; got != true {
		t.Fatalf("first suggestion blocking = %#v, want true", got)
	}
	summary, ok := res.Payload["summary"].(map[string]any)
	if !ok {
		t.Fatalf("evaluate summary = %#v, want object", res.Payload["summary"])
	}
	if got := summary["suggestion_count"]; got == nil || got == float64(0) {
		t.Fatalf("suggestion_count = %#v, want non-zero", got)
	}
}

func TestHandleWebSocketOrchestrationEvaluateCanFilterSuggestions(t *testing.T) {
	sessionManager := session.NewManager(nil)
	server := NewServerWithOptions(log.New(io.Discard, "", 0), sessionManager, llm.NewMockClient(), Options{
		PermissionPolicy: permissions.Policy{Mode: permissions.ModeAsk},
	})
	httpServer := httptest.NewServer(http.HandlerFunc(server.HandleWebSocket))
	t.Cleanup(httpServer.Close)

	wsURL := "ws" + strings.TrimPrefix(httpServer.URL, "http")
	conn, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
	if err != nil {
		t.Fatalf("dial websocket: %v", err)
	}
	t.Cleanup(func() { _ = conn.Close() })

	if err := conn.WriteJSON(protocolws.Message{
		Type:   protocolws.TypeRequest,
		ID:     "1",
		Method: protocolws.MethodConnect,
		Payload: map[string]any{
			"role":            "client",
			"client_identity": "web-ui",
			"agent_id":        "main",
		},
	}); err != nil {
		t.Fatalf("write connect: %v", err)
	}
	_ = conn.ReadJSON(&protocolws.Message{})
	_ = conn.ReadJSON(&protocolws.Message{})

	if err := conn.WriteJSON(protocolws.Message{
		Type:   protocolws.TypeRequest,
		ID:     "2",
		Method: protocolws.MethodSendMessage,
		Payload: map[string]any{
			"content": "tool run pwd",
		},
	}); err != nil {
		t.Fatalf("write approval-producing message: %v", err)
	}
	_ = waitForPermissionRequired(t, conn)

	if err := conn.WriteJSON(protocolws.Message{
		Type:   protocolws.TypeRequest,
		ID:     "3",
		Method: protocolws.MethodSendMessage,
		Payload: map[string]any{
			"content": "hello",
		},
	}); err != nil {
		t.Fatalf("write plain message: %v", err)
	}
	for i := 0; i < 32; i++ {
		var event protocolws.Message
		if err := conn.ReadJSON(&event); err != nil {
			t.Fatalf("read plain runtime event %d: %v", i, err)
		}
		if event.Type == protocolws.TypeEvent && event.Event == "agent.lifecycle.end" {
			break
		}
	}

	if err := conn.WriteJSON(protocolws.Message{
		Type:   protocolws.TypeRequest,
		ID:     "4",
		Method: protocolws.MethodOrchestrationEvaluate,
		Payload: map[string]any{
			"category":      "approval",
			"priority":      "high",
			"blocking_only": true,
		},
	}); err != nil {
		t.Fatalf("write filtered orchestration_evaluate: %v", err)
	}
	var res protocolws.Message
	if err := conn.ReadJSON(&res); err != nil {
		t.Fatalf("read filtered orchestration_evaluate: %v", err)
	}
	if !res.OK {
		t.Fatalf("filtered orchestration_evaluate response = %#v, want ok", res)
	}
	suggestions, ok := res.Payload["suggestions"].([]any)
	if !ok || len(suggestions) != 1 {
		t.Fatalf("filtered suggestions payload = %#v, want one item", res.Payload["suggestions"])
	}
	first, ok := suggestions[0].(map[string]any)
	if !ok {
		t.Fatalf("first filtered suggestion = %#v, want object", suggestions[0])
	}
	if got := first["category"]; got != "approval" {
		t.Fatalf("filtered category = %#v, want approval", got)
	}
	if got := first["priority"]; got != "high" {
		t.Fatalf("filtered priority = %#v, want high", got)
	}
	summary, ok := res.Payload["summary"].(map[string]any)
	if !ok {
		t.Fatalf("filtered evaluate summary = %#v, want object", res.Payload["summary"])
	}
	if got := summary["suggestion_count"]; got != float64(1) {
		t.Fatalf("filtered suggestion_count = %#v, want 1", got)
	}
}

func TestHandleWebSocketOrchestrationPlanReturnsOrderedSteps(t *testing.T) {
	sessionManager := session.NewManager(nil)
	server := NewServerWithOptions(log.New(io.Discard, "", 0), sessionManager, llm.NewMockClient(), Options{
		PermissionPolicy: permissions.Policy{Mode: permissions.ModeAsk},
	})
	httpServer := httptest.NewServer(http.HandlerFunc(server.HandleWebSocket))
	t.Cleanup(httpServer.Close)

	wsURL := "ws" + strings.TrimPrefix(httpServer.URL, "http")
	conn, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
	if err != nil {
		t.Fatalf("dial websocket: %v", err)
	}
	t.Cleanup(func() { _ = conn.Close() })

	if err := conn.WriteJSON(protocolws.Message{
		Type:   protocolws.TypeRequest,
		ID:     "1",
		Method: protocolws.MethodConnect,
		Payload: map[string]any{
			"role":            "client",
			"client_identity": "web-ui",
			"agent_id":        "main",
		},
	}); err != nil {
		t.Fatalf("write connect: %v", err)
	}
	_ = conn.ReadJSON(&protocolws.Message{})
	_ = conn.ReadJSON(&protocolws.Message{})

	if err := conn.WriteJSON(protocolws.Message{
		Type:   protocolws.TypeRequest,
		ID:     "2",
		Method: protocolws.MethodSendMessage,
		Payload: map[string]any{
			"content": "tool run pwd",
		},
	}); err != nil {
		t.Fatalf("write approval-producing message: %v", err)
	}
	_ = waitForPermissionRequired(t, conn)

	if err := conn.WriteJSON(protocolws.Message{
		Type:   protocolws.TypeRequest,
		ID:     "3",
		Method: protocolws.MethodSendMessage,
		Payload: map[string]any{
			"content": "hello",
		},
	}); err != nil {
		t.Fatalf("write plain message: %v", err)
	}
	for i := 0; i < 32; i++ {
		var event protocolws.Message
		if err := conn.ReadJSON(&event); err != nil {
			t.Fatalf("read plain runtime event %d: %v", i, err)
		}
		if event.Type == protocolws.TypeEvent && event.Event == "agent.lifecycle.end" {
			break
		}
	}

	if err := conn.WriteJSON(protocolws.Message{
		Type:   protocolws.TypeRequest,
		ID:     "4",
		Method: protocolws.MethodOrchestrationPlan,
		Payload: map[string]any{
			"blocking_only": true,
		},
	}); err != nil {
		t.Fatalf("write orchestration_plan: %v", err)
	}
	var res protocolws.Message
	if err := conn.ReadJSON(&res); err != nil {
		t.Fatalf("read orchestration_plan: %v", err)
	}
	if !res.OK {
		t.Fatalf("orchestration_plan response = %#v, want ok", res)
	}
	steps, ok := res.Payload["steps"].([]any)
	if !ok || len(steps) != 1 {
		t.Fatalf("plan steps payload = %#v, want one item", res.Payload["steps"])
	}
	first, ok := steps[0].(map[string]any)
	if !ok {
		t.Fatalf("first step = %#v, want object", steps[0])
	}
	if got := first["title"]; got == nil || got == "" {
		t.Fatalf("step title = %#v, want populated value", got)
	}
	if got := first["action_kind"]; got != "approval" {
		t.Fatalf("step action_kind = %#v, want approval", got)
	}
	if got := first["action_id"]; got == nil || got == "" {
		t.Fatalf("step action_id = %#v, want populated value", got)
	}
	if got := first["phase"]; got != "stabilize" {
		t.Fatalf("step phase = %#v, want stabilize", got)
	}
	if got := first["state"]; got != "pending" {
		t.Fatalf("step state = %#v, want pending", got)
	}
	if got := first["result"]; got != "" {
		t.Fatalf("step result = %#v, want empty", got)
	}
	if got := first["updated_at"]; got == nil || got == "" {
		t.Fatalf("step updated_at = %#v, want populated value", got)
	}
	if got := first["depends_on"]; got != "" {
		t.Fatalf("first step depends_on = %#v, want empty", got)
	}
	if got := first["suggested_action"]; got != "request_human_approval" {
		t.Fatalf("step suggested_action = %#v, want request_human_approval", got)
	}
	if got := res.Payload["summary"]; got == nil || got == "" {
		t.Fatalf("plan summary = %#v, want populated value", got)
	}
	groups, ok := res.Payload["groups"].(map[string]any)
	if !ok {
		t.Fatalf("plan groups = %#v, want object", res.Payload["groups"])
	}
	if got := groups["approval"]; got != float64(1) {
		t.Fatalf("approval group count = %#v, want 1", got)
	}
	sections, ok := res.Payload["priority_sections"].(map[string]any)
	if !ok {
		t.Fatalf("priority_sections = %#v, want object", res.Payload["priority_sections"])
	}
	if got := sections["high"]; got != float64(1) {
		t.Fatalf("high priority section count = %#v, want 1", got)
	}
	phaseSections, ok := res.Payload["phase_sections"].(map[string]any)
	if !ok {
		t.Fatalf("phase_sections = %#v, want object", res.Payload["phase_sections"])
	}
	if got := phaseSections["stabilize"]; got != float64(1) {
		t.Fatalf("stabilize phase count = %#v, want 1", got)
	}
}

func TestHandleWebSocketOrchestrationPlanChainsDependencies(t *testing.T) {
	sessionManager := session.NewManager(nil)
	server := NewServerWithOptions(log.New(io.Discard, "", 0), sessionManager, llm.NewMockClient(), Options{
		PermissionPolicy: permissions.Policy{Mode: permissions.ModeAsk},
	})
	httpServer := httptest.NewServer(http.HandlerFunc(server.HandleWebSocket))
	t.Cleanup(httpServer.Close)

	wsURL := "ws" + strings.TrimPrefix(httpServer.URL, "http")
	conn, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
	if err != nil {
		t.Fatalf("dial websocket: %v", err)
	}
	t.Cleanup(func() { _ = conn.Close() })

	if err := conn.WriteJSON(protocolws.Message{
		Type:   protocolws.TypeRequest,
		ID:     "1",
		Method: protocolws.MethodConnect,
		Payload: map[string]any{
			"role":            "client",
			"client_identity": "web-ui",
			"agent_id":        "main",
		},
	}); err != nil {
		t.Fatalf("write connect: %v", err)
	}
	_ = conn.ReadJSON(&protocolws.Message{})
	_ = conn.ReadJSON(&protocolws.Message{})

	if err := conn.WriteJSON(protocolws.Message{
		Type:   protocolws.TypeRequest,
		ID:     "2",
		Method: protocolws.MethodSendMessage,
		Payload: map[string]any{
			"content": "tool run pwd",
		},
	}); err != nil {
		t.Fatalf("write approval-producing message: %v", err)
	}
	_ = waitForPermissionRequired(t, conn)

	if err := conn.WriteJSON(protocolws.Message{
		Type:   protocolws.TypeRequest,
		ID:     "3",
		Method: protocolws.MethodSendMessage,
		Payload: map[string]any{
			"content": "hello",
		},
	}); err != nil {
		t.Fatalf("write plain message: %v", err)
	}
	for i := 0; i < 32; i++ {
		var event protocolws.Message
		if err := conn.ReadJSON(&event); err != nil {
			t.Fatalf("read plain runtime event %d: %v", i, err)
		}
		if event.Type == protocolws.TypeEvent && event.Event == "agent.lifecycle.end" {
			break
		}
	}

	if err := conn.WriteJSON(protocolws.Message{
		Type:   protocolws.TypeRequest,
		ID:     "4",
		Method: protocolws.MethodOrchestrationPlan,
	}); err != nil {
		t.Fatalf("write orchestration_plan: %v", err)
	}
	var res protocolws.Message
	if err := conn.ReadJSON(&res); err != nil {
		t.Fatalf("read orchestration_plan: %v", err)
	}
	if !res.OK {
		t.Fatalf("orchestration_plan response = %#v, want ok", res)
	}
	steps, ok := res.Payload["steps"].([]any)
	if !ok || len(steps) < 2 {
		t.Fatalf("plan steps payload = %#v, want at least two items", res.Payload["steps"])
	}
	first, _ := steps[0].(map[string]any)
	second, _ := steps[1].(map[string]any)
	if second["depends_on"] != first["action_id"] {
		t.Fatalf("second depends_on = %#v, want %v", second["depends_on"], first["action_id"])
	}
	if second["state"] != "blocked" {
		t.Fatalf("second state = %#v, want blocked", second["state"])
	}
}

func TestHandleWebSocketOrchestrationPlanStepUpdatePersistsState(t *testing.T) {
	sessionManager := session.NewManager(nil)
	server := NewServerWithOptions(log.New(io.Discard, "", 0), sessionManager, llm.NewMockClient(), Options{
		PermissionPolicy: permissions.Policy{Mode: permissions.ModeAsk},
	})
	httpServer := httptest.NewServer(http.HandlerFunc(server.HandleWebSocket))
	t.Cleanup(httpServer.Close)

	wsURL := "ws" + strings.TrimPrefix(httpServer.URL, "http")
	conn, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
	if err != nil {
		t.Fatalf("dial websocket: %v", err)
	}
	t.Cleanup(func() { _ = conn.Close() })

	if err := conn.WriteJSON(protocolws.Message{
		Type:   protocolws.TypeRequest,
		ID:     "1",
		Method: protocolws.MethodConnect,
		Payload: map[string]any{
			"role":            "client",
			"client_identity": "web-ui",
			"agent_id":        "main",
		},
	}); err != nil {
		t.Fatalf("write connect: %v", err)
	}
	_ = conn.ReadJSON(&protocolws.Message{})
	_ = conn.ReadJSON(&protocolws.Message{})

	if err := conn.WriteJSON(protocolws.Message{
		Type:   protocolws.TypeRequest,
		ID:     "2",
		Method: protocolws.MethodSendMessage,
		Payload: map[string]any{
			"content": "tool run pwd",
		},
	}); err != nil {
		t.Fatalf("write approval-producing message: %v", err)
	}
	_ = waitForPermissionRequired(t, conn)

	if err := conn.WriteJSON(protocolws.Message{
		Type:   protocolws.TypeRequest,
		ID:     "3",
		Method: protocolws.MethodOrchestrationPlan,
	}); err != nil {
		t.Fatalf("write orchestration_plan: %v", err)
	}
	var planRes protocolws.Message
	if err := conn.ReadJSON(&planRes); err != nil {
		t.Fatalf("read orchestration_plan: %v", err)
	}
	steps, ok := planRes.Payload["steps"].([]any)
	if !ok || len(steps) == 0 {
		t.Fatalf("plan steps payload = %#v, want non-empty", planRes.Payload["steps"])
	}
	first, _ := steps[0].(map[string]any)
	actionID, _ := first["action_id"].(string)
	if actionID == "" {
		t.Fatal("expected action_id")
	}

	if err := conn.WriteJSON(protocolws.Message{
		Type:   protocolws.TypeRequest,
		ID:     "4",
		Method: protocolws.MethodOrchestrationPlanStepUpdate,
		Payload: map[string]any{
			"action_id": actionID,
			"state":     "completed",
			"result":    "approved manually",
		},
	}); err != nil {
		t.Fatalf("write plan step update: %v", err)
	}
	var updateRes protocolws.Message
	if err := conn.ReadJSON(&updateRes); err != nil {
		t.Fatalf("read plan step update: %v", err)
	}
	if !updateRes.OK {
		t.Fatalf("plan step update response = %#v, want ok", updateRes)
	}

	var updatedEvent protocolws.Message
	if err := conn.ReadJSON(&updatedEvent); err != nil {
		t.Fatalf("read plan step updated event: %v", err)
	}
	if updatedEvent.Type != protocolws.TypeEvent || updatedEvent.Event != protocolws.EventOrchestrationPlanStepUpdated {
		t.Fatalf("plan step updated event = %#v, want orchestration.plan_step.updated", updatedEvent)
	}
	stepPayload, ok := updatedEvent.Payload["step"].(map[string]any)
	if !ok {
		t.Fatalf("updated event step payload = %#v, want object", updatedEvent.Payload["step"])
	}
	if stepPayload["action_id"] != actionID {
		t.Fatalf("updated event action_id = %#v, want %q", stepPayload["action_id"], actionID)
	}
	if stepPayload["state"] != "completed" {
		t.Fatalf("updated event state = %#v, want completed", stepPayload["state"])
	}

	if err := conn.WriteJSON(protocolws.Message{
		Type:   protocolws.TypeRequest,
		ID:     "5",
		Method: protocolws.MethodOrchestrationPlan,
	}); err != nil {
		t.Fatalf("write orchestration_plan reload: %v", err)
	}
	var reloadRes protocolws.Message
	if err := conn.ReadJSON(&reloadRes); err != nil {
		t.Fatalf("read orchestration_plan reload: %v", err)
	}
	reloadedSteps, ok := reloadRes.Payload["steps"].([]any)
	if !ok || len(reloadedSteps) == 0 {
		t.Fatalf("reloaded steps payload = %#v, want non-empty", reloadRes.Payload["steps"])
	}
	reloadedFirst, _ := reloadedSteps[0].(map[string]any)
	if reloadedFirst["state"] != "completed" {
		t.Fatalf("reloaded state = %#v, want completed", reloadedFirst["state"])
	}
	if reloadedFirst["result"] != "approved manually" {
		t.Fatalf("reloaded result = %#v, want approved manually", reloadedFirst["result"])
	}
}

func TestHandleWebSocketOrchestrationPlanStepUpdateRejectsInvalidTransition(t *testing.T) {
	sessionManager := session.NewManager(nil)
	server := NewServerWithOptions(log.New(io.Discard, "", 0), sessionManager, llm.NewMockClient(), Options{
		PermissionPolicy: permissions.Policy{Mode: permissions.ModeAsk},
	})
	httpServer := httptest.NewServer(http.HandlerFunc(server.HandleWebSocket))
	t.Cleanup(httpServer.Close)

	wsURL := "ws" + strings.TrimPrefix(httpServer.URL, "http")
	conn, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
	if err != nil {
		t.Fatalf("dial websocket: %v", err)
	}
	t.Cleanup(func() { _ = conn.Close() })

	if err := conn.WriteJSON(protocolws.Message{
		Type:   protocolws.TypeRequest,
		ID:     "1",
		Method: protocolws.MethodConnect,
		Payload: map[string]any{
			"role":            "client",
			"client_identity": "web-ui",
			"agent_id":        "main",
		},
	}); err != nil {
		t.Fatalf("write connect: %v", err)
	}
	_ = conn.ReadJSON(&protocolws.Message{})
	_ = conn.ReadJSON(&protocolws.Message{})

	if err := conn.WriteJSON(protocolws.Message{
		Type:   protocolws.TypeRequest,
		ID:     "2",
		Method: protocolws.MethodSendMessage,
		Payload: map[string]any{
			"content": "tool run pwd",
		},
	}); err != nil {
		t.Fatalf("write approval-producing message: %v", err)
	}
	_ = waitForPermissionRequired(t, conn)

	if err := conn.WriteJSON(protocolws.Message{
		Type:   protocolws.TypeRequest,
		ID:     "3",
		Method: protocolws.MethodSendMessage,
		Payload: map[string]any{
			"content": "hello",
		},
	}); err != nil {
		t.Fatalf("write plain message: %v", err)
	}
	for i := 0; i < 32; i++ {
		var event protocolws.Message
		if err := conn.ReadJSON(&event); err != nil {
			t.Fatalf("read plain runtime event %d: %v", i, err)
		}
		if event.Type == protocolws.TypeEvent && event.Event == "agent.lifecycle.end" {
			break
		}
	}

	if err := conn.WriteJSON(protocolws.Message{
		Type:   protocolws.TypeRequest,
		ID:     "4",
		Method: protocolws.MethodOrchestrationPlan,
	}); err != nil {
		t.Fatalf("write orchestration_plan: %v", err)
	}
	var planRes protocolws.Message
	if err := conn.ReadJSON(&planRes); err != nil {
		t.Fatalf("read orchestration_plan: %v", err)
	}
	steps, ok := planRes.Payload["steps"].([]any)
	if !ok || len(steps) < 2 {
		t.Fatalf("plan steps payload = %#v, want at least two items", planRes.Payload["steps"])
	}
	second, _ := steps[1].(map[string]any)
	actionID, _ := second["action_id"].(string)
	if actionID == "" {
		t.Fatal("expected second action_id")
	}

	if err := conn.WriteJSON(protocolws.Message{
		Type:   protocolws.TypeRequest,
		ID:     "5",
		Method: protocolws.MethodOrchestrationPlanStepUpdate,
		Payload: map[string]any{
			"action_id": actionID,
			"state":     "completed",
			"result":    "skipped ahead",
		},
	}); err != nil {
		t.Fatalf("write invalid plan step update: %v", err)
	}
	var updateRes protocolws.Message
	if err := conn.ReadJSON(&updateRes); err != nil {
		t.Fatalf("read invalid plan step update: %v", err)
	}
	if updateRes.OK {
		t.Fatalf("invalid plan step update response = %#v, want error", updateRes)
	}
}

func TestHandleWebSocketOrchestrationPlanStepUpdateUnlocksDependentStep(t *testing.T) {
	sessionManager := session.NewManager(nil)
	server := NewServerWithOptions(log.New(io.Discard, "", 0), sessionManager, llm.NewMockClient(), Options{
		PermissionPolicy: permissions.Policy{Mode: permissions.ModeAsk},
	})
	httpServer := httptest.NewServer(http.HandlerFunc(server.HandleWebSocket))
	t.Cleanup(httpServer.Close)

	wsURL := "ws" + strings.TrimPrefix(httpServer.URL, "http")
	conn, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
	if err != nil {
		t.Fatalf("dial websocket: %v", err)
	}
	t.Cleanup(func() { _ = conn.Close() })

	if err := conn.WriteJSON(protocolws.Message{
		Type:   protocolws.TypeRequest,
		ID:     "1",
		Method: protocolws.MethodConnect,
		Payload: map[string]any{
			"role":            "client",
			"client_identity": "web-ui",
			"agent_id":        "main",
		},
	}); err != nil {
		t.Fatalf("write connect: %v", err)
	}
	_ = conn.ReadJSON(&protocolws.Message{})
	_ = conn.ReadJSON(&protocolws.Message{})

	if err := conn.WriteJSON(protocolws.Message{
		Type:   protocolws.TypeRequest,
		ID:     "2",
		Method: protocolws.MethodSendMessage,
		Payload: map[string]any{
			"content": "tool run pwd",
		},
	}); err != nil {
		t.Fatalf("write approval-producing message: %v", err)
	}
	_ = waitForPermissionRequired(t, conn)

	if err := conn.WriteJSON(protocolws.Message{
		Type:   protocolws.TypeRequest,
		ID:     "3",
		Method: protocolws.MethodSendMessage,
		Payload: map[string]any{
			"content": "hello",
		},
	}); err != nil {
		t.Fatalf("write plain message: %v", err)
	}
	for i := 0; i < 32; i++ {
		var event protocolws.Message
		if err := conn.ReadJSON(&event); err != nil {
			t.Fatalf("read plain runtime event %d: %v", i, err)
		}
		if event.Type == protocolws.TypeEvent && event.Event == "agent.lifecycle.end" {
			break
		}
	}

	if err := conn.WriteJSON(protocolws.Message{
		Type:   protocolws.TypeRequest,
		ID:     "4",
		Method: protocolws.MethodOrchestrationPlan,
	}); err != nil {
		t.Fatalf("write orchestration_plan: %v", err)
	}
	var planRes protocolws.Message
	if err := conn.ReadJSON(&planRes); err != nil {
		t.Fatalf("read orchestration_plan: %v", err)
	}
	steps, ok := planRes.Payload["steps"].([]any)
	if !ok || len(steps) < 2 {
		t.Fatalf("plan steps payload = %#v, want at least two items", planRes.Payload["steps"])
	}
	first, _ := steps[0].(map[string]any)
	second, _ := steps[1].(map[string]any)
	firstActionID, _ := first["action_id"].(string)
	if firstActionID == "" {
		t.Fatal("expected first action_id")
	}
	if second["state"] != "blocked" {
		t.Fatalf("initial second state = %#v, want blocked", second["state"])
	}

	if err := conn.WriteJSON(protocolws.Message{
		Type:   protocolws.TypeRequest,
		ID:     "5",
		Method: protocolws.MethodOrchestrationPlanStepUpdate,
		Payload: map[string]any{
			"action_id": firstActionID,
			"state":     "completed",
			"result":    "approved manually",
		},
	}); err != nil {
		t.Fatalf("write plan step update: %v", err)
	}
	var updateRes protocolws.Message
	if err := conn.ReadJSON(&updateRes); err != nil {
		t.Fatalf("read plan step update: %v", err)
	}
	if !updateRes.OK {
		t.Fatalf("plan step update response = %#v, want ok", updateRes)
	}
	_ = conn.ReadJSON(&protocolws.Message{})

	if err := conn.WriteJSON(protocolws.Message{
		Type:   protocolws.TypeRequest,
		ID:     "6",
		Method: protocolws.MethodOrchestrationPlan,
	}); err != nil {
		t.Fatalf("write orchestration_plan reload: %v", err)
	}
	var reloadRes protocolws.Message
	if err := conn.ReadJSON(&reloadRes); err != nil {
		t.Fatalf("read orchestration_plan reload: %v", err)
	}
	reloadedSteps, ok := reloadRes.Payload["steps"].([]any)
	if !ok || len(reloadedSteps) < 2 {
		t.Fatalf("reloaded plan steps payload = %#v, want at least two items", reloadRes.Payload["steps"])
	}
	reloadedSecond, _ := reloadedSteps[1].(map[string]any)
	if reloadedSecond["state"] != "ready" {
		t.Fatalf("reloaded second state = %#v, want ready", reloadedSecond["state"])
	}
}

func TestHandleWebSocketOrchestrationPlanStepHistoryReturnsExecutionAudit(t *testing.T) {
	sessionManager := session.NewManager(nil)
	server := NewServerWithOptions(log.New(io.Discard, "", 0), sessionManager, llm.NewMockClient(), Options{
		PermissionPolicy: permissions.Policy{Mode: permissions.ModeAsk},
	})
	httpServer := httptest.NewServer(http.HandlerFunc(server.HandleWebSocket))
	t.Cleanup(httpServer.Close)

	wsURL := "ws" + strings.TrimPrefix(httpServer.URL, "http")
	conn, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
	if err != nil {
		t.Fatalf("dial websocket: %v", err)
	}
	t.Cleanup(func() { _ = conn.Close() })

	if err := conn.WriteJSON(protocolws.Message{
		Type:   protocolws.TypeRequest,
		ID:     "1",
		Method: protocolws.MethodConnect,
		Payload: map[string]any{
			"role":            "client",
			"client_identity": "web-ui",
			"agent_id":        "main",
		},
	}); err != nil {
		t.Fatalf("write connect: %v", err)
	}
	_ = conn.ReadJSON(&protocolws.Message{})
	_ = conn.ReadJSON(&protocolws.Message{})

	if err := conn.WriteJSON(protocolws.Message{
		Type:   protocolws.TypeRequest,
		ID:     "2",
		Method: protocolws.MethodSendMessage,
		Payload: map[string]any{
			"content": "tool run pwd",
		},
	}); err != nil {
		t.Fatalf("write approval-producing message: %v", err)
	}
	_ = waitForPermissionRequired(t, conn)

	if err := conn.WriteJSON(protocolws.Message{
		Type:   protocolws.TypeRequest,
		ID:     "3",
		Method: protocolws.MethodOrchestrationPlan,
	}); err != nil {
		t.Fatalf("write orchestration_plan: %v", err)
	}
	var planRes protocolws.Message
	if err := conn.ReadJSON(&planRes); err != nil {
		t.Fatalf("read orchestration_plan: %v", err)
	}
	steps, ok := planRes.Payload["steps"].([]any)
	if !ok || len(steps) == 0 {
		t.Fatalf("plan steps payload = %#v, want non-empty", planRes.Payload["steps"])
	}
	first, _ := steps[0].(map[string]any)
	actionID, _ := first["action_id"].(string)
	if actionID == "" {
		t.Fatal("expected first action_id")
	}

	for idx, state := range []string{"in_progress", "completed"} {
		if err := conn.WriteJSON(protocolws.Message{
			Type:   protocolws.TypeRequest,
			ID:     string(rune('4' + idx)),
			Method: protocolws.MethodOrchestrationPlanStepUpdate,
			Payload: map[string]any{
				"action_id": actionID,
				"state":     state,
				"result":    state,
			},
		}); err != nil {
			t.Fatalf("write plan step update %d: %v", idx, err)
		}
		var updateRes protocolws.Message
		if err := conn.ReadJSON(&updateRes); err != nil {
			t.Fatalf("read plan step update %d: %v", idx, err)
		}
		if !updateRes.OK {
			t.Fatalf("plan step update %d response = %#v, want ok", idx, updateRes)
		}
		_ = conn.ReadJSON(&protocolws.Message{})
	}

	if err := conn.WriteJSON(protocolws.Message{
		Type:   protocolws.TypeRequest,
		ID:     "6",
		Method: protocolws.MethodOrchestrationPlanStepHistory,
		Payload: map[string]any{
			"action_id": actionID,
		},
	}); err != nil {
		t.Fatalf("write orchestration_plan_step_history: %v", err)
	}
	var historyRes protocolws.Message
	if err := conn.ReadJSON(&historyRes); err != nil {
		t.Fatalf("read orchestration_plan_step_history: %v", err)
	}
	if !historyRes.OK {
		t.Fatalf("plan step history response = %#v, want ok", historyRes)
	}
	history, ok := historyRes.Payload["history"].([]any)
	if !ok || len(history) != 2 {
		t.Fatalf("plan step history payload = %#v, want two items", historyRes.Payload["history"])
	}
	firstRecord, _ := history[0].(map[string]any)
	secondRecord, _ := history[1].(map[string]any)
	if firstRecord["state"] != "in_progress" {
		t.Fatalf("first history state = %#v, want in_progress", firstRecord["state"])
	}
	if secondRecord["state"] != "completed" {
		t.Fatalf("second history state = %#v, want completed", secondRecord["state"])
	}
}

func TestHandleWebSocketOrchestrationPlanStepHistoryCanFilterAndSummarize(t *testing.T) {
	sessionManager := session.NewManager(nil)
	server := NewServerWithOptions(log.New(io.Discard, "", 0), sessionManager, llm.NewMockClient(), Options{
		PermissionPolicy: permissions.Policy{Mode: permissions.ModeAsk},
	})
	httpServer := httptest.NewServer(http.HandlerFunc(server.HandleWebSocket))
	t.Cleanup(httpServer.Close)

	wsURL := "ws" + strings.TrimPrefix(httpServer.URL, "http")
	conn, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
	if err != nil {
		t.Fatalf("dial websocket: %v", err)
	}
	t.Cleanup(func() { _ = conn.Close() })

	if err := conn.WriteJSON(protocolws.Message{
		Type:   protocolws.TypeRequest,
		ID:     "1",
		Method: protocolws.MethodConnect,
		Payload: map[string]any{
			"role":            "client",
			"client_identity": "web-ui",
			"agent_id":        "main",
		},
	}); err != nil {
		t.Fatalf("write connect: %v", err)
	}
	_ = conn.ReadJSON(&protocolws.Message{})
	_ = conn.ReadJSON(&protocolws.Message{})

	if err := conn.WriteJSON(protocolws.Message{
		Type:   protocolws.TypeRequest,
		ID:     "2",
		Method: protocolws.MethodSendMessage,
		Payload: map[string]any{
			"content": "tool run pwd",
		},
	}); err != nil {
		t.Fatalf("write approval-producing message: %v", err)
	}
	_ = waitForPermissionRequired(t, conn)

	if err := conn.WriteJSON(protocolws.Message{
		Type:   protocolws.TypeRequest,
		ID:     "3",
		Method: protocolws.MethodOrchestrationPlan,
	}); err != nil {
		t.Fatalf("write orchestration_plan: %v", err)
	}
	var planRes protocolws.Message
	if err := conn.ReadJSON(&planRes); err != nil {
		t.Fatalf("read orchestration_plan: %v", err)
	}
	steps, ok := planRes.Payload["steps"].([]any)
	if !ok || len(steps) == 0 {
		t.Fatalf("plan steps payload = %#v, want non-empty", planRes.Payload["steps"])
	}
	first, _ := steps[0].(map[string]any)
	actionID, _ := first["action_id"].(string)
	if actionID == "" {
		t.Fatal("expected first action_id")
	}

	for i, state := range []string{"in_progress", "completed"} {
		if err := conn.WriteJSON(protocolws.Message{
			Type:   protocolws.TypeRequest,
			ID:     string(rune('4' + i)),
			Method: protocolws.MethodOrchestrationPlanStepUpdate,
			Payload: map[string]any{
				"action_id": actionID,
				"state":     state,
				"result":    state,
			},
		}); err != nil {
			t.Fatalf("write plan step update %d: %v", i, err)
		}
		var updateRes protocolws.Message
		if err := conn.ReadJSON(&updateRes); err != nil {
			t.Fatalf("read plan step update %d: %v", i, err)
		}
		if !updateRes.OK {
			t.Fatalf("plan step update %d response = %#v, want ok", i, updateRes)
		}
		_ = conn.ReadJSON(&protocolws.Message{})
	}

	if err := conn.WriteJSON(protocolws.Message{
		Type:   protocolws.TypeRequest,
		ID:     "6",
		Method: protocolws.MethodOrchestrationPlanStepHistory,
		Payload: map[string]any{
			"action_id": actionID,
			"state":     "completed",
		},
	}); err != nil {
		t.Fatalf("write filtered orchestration_plan_step_history: %v", err)
	}
	var historyRes protocolws.Message
	if err := conn.ReadJSON(&historyRes); err != nil {
		t.Fatalf("read filtered orchestration_plan_step_history: %v", err)
	}
	if !historyRes.OK {
		t.Fatalf("plan step history response = %#v, want ok", historyRes)
	}
	history, ok := historyRes.Payload["history"].([]any)
	if !ok || len(history) != 1 {
		t.Fatalf("filtered history payload = %#v, want one item", historyRes.Payload["history"])
	}
	record, _ := history[0].(map[string]any)
	if record["state"] != "completed" {
		t.Fatalf("filtered history state = %#v, want completed", record["state"])
	}
	summary, ok := historyRes.Payload["summary"].(map[string]any)
	if !ok {
		t.Fatalf("history summary = %#v, want object", historyRes.Payload["summary"])
	}
	if summary["record_count"] != float64(1) && summary["record_count"] != 1 {
		t.Fatalf("summary record_count = %#v, want 1", summary["record_count"])
	}
	if summary["latest_state"] != "completed" {
		t.Fatalf("summary latest_state = %#v, want completed", summary["latest_state"])
	}
	stateCounts, ok := summary["state_counts"].(map[string]any)
	if !ok {
		t.Fatalf("summary state_counts = %#v, want object", summary["state_counts"])
	}
	if stateCounts["completed"] != float64(1) && stateCounts["completed"] != 1 {
		t.Fatalf("summary completed count = %#v, want 1", stateCounts["completed"])
	}
}

func TestHandleWebSocketOrchestrationPlanOverviewSummarizesExecutionProgress(t *testing.T) {
	sessionManager := session.NewManager(nil)
	server := NewServerWithOptions(log.New(io.Discard, "", 0), sessionManager, llm.NewMockClient(), Options{
		PermissionPolicy: permissions.Policy{Mode: permissions.ModeAsk},
	})
	httpServer := httptest.NewServer(http.HandlerFunc(server.HandleWebSocket))
	t.Cleanup(httpServer.Close)

	wsURL := "ws" + strings.TrimPrefix(httpServer.URL, "http")
	conn, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
	if err != nil {
		t.Fatalf("dial websocket: %v", err)
	}
	t.Cleanup(func() { _ = conn.Close() })

	if err := conn.WriteJSON(protocolws.Message{
		Type:   protocolws.TypeRequest,
		ID:     "1",
		Method: protocolws.MethodConnect,
		Payload: map[string]any{
			"role":            "client",
			"client_identity": "web-ui",
			"agent_id":        "main",
		},
	}); err != nil {
		t.Fatalf("write connect: %v", err)
	}
	_ = conn.ReadJSON(&protocolws.Message{})
	_ = conn.ReadJSON(&protocolws.Message{})

	if err := conn.WriteJSON(protocolws.Message{
		Type:   protocolws.TypeRequest,
		ID:     "2",
		Method: protocolws.MethodSendMessage,
		Payload: map[string]any{
			"content": "tool run pwd",
		},
	}); err != nil {
		t.Fatalf("write approval-producing message: %v", err)
	}
	_ = waitForPermissionRequired(t, conn)

	if err := conn.WriteJSON(protocolws.Message{
		Type:   protocolws.TypeRequest,
		ID:     "3",
		Method: protocolws.MethodSendMessage,
		Payload: map[string]any{
			"content": "hello",
		},
	}); err != nil {
		t.Fatalf("write plain message: %v", err)
	}
	for i := 0; i < 32; i++ {
		var event protocolws.Message
		if err := conn.ReadJSON(&event); err != nil {
			t.Fatalf("read plain runtime event %d: %v", i, err)
		}
		if event.Type == protocolws.TypeEvent && event.Event == "agent.lifecycle.end" {
			break
		}
	}

	if err := conn.WriteJSON(protocolws.Message{
		Type:   protocolws.TypeRequest,
		ID:     "4",
		Method: protocolws.MethodOrchestrationPlan,
	}); err != nil {
		t.Fatalf("write orchestration_plan: %v", err)
	}
	var planRes protocolws.Message
	if err := conn.ReadJSON(&planRes); err != nil {
		t.Fatalf("read orchestration_plan: %v", err)
	}
	steps, ok := planRes.Payload["steps"].([]any)
	if !ok || len(steps) < 2 {
		t.Fatalf("plan steps payload = %#v, want at least two items", planRes.Payload["steps"])
	}
	first, _ := steps[0].(map[string]any)
	second, _ := steps[1].(map[string]any)
	actionID, _ := first["action_id"].(string)
	secondActionID, _ := second["action_id"].(string)
	if actionID == "" {
		t.Fatal("expected first action_id")
	}
	if secondActionID == "" {
		t.Fatal("expected second action_id")
	}

	if err := conn.WriteJSON(protocolws.Message{
		Type:   protocolws.TypeRequest,
		ID:     "5",
		Method: protocolws.MethodOrchestrationPlanStepUpdate,
		Payload: map[string]any{
			"action_id": actionID,
			"state":     "completed",
			"result":    "approved",
		},
	}); err != nil {
		t.Fatalf("write plan step update: %v", err)
	}
	var updateRes protocolws.Message
	if err := conn.ReadJSON(&updateRes); err != nil {
		t.Fatalf("read plan step update: %v", err)
	}
	if !updateRes.OK {
		t.Fatalf("plan step update response = %#v, want ok", updateRes)
	}
	_ = conn.ReadJSON(&protocolws.Message{})

	if err := conn.WriteJSON(protocolws.Message{
		Type:   protocolws.TypeRequest,
		ID:     "6",
		Method: protocolws.MethodOrchestrationPlanOverview,
	}); err != nil {
		t.Fatalf("write orchestration_plan_overview: %v", err)
	}
	var overviewRes protocolws.Message
	if err := conn.ReadJSON(&overviewRes); err != nil {
		t.Fatalf("read orchestration_plan_overview: %v", err)
	}
	if !overviewRes.OK {
		t.Fatalf("plan overview response = %#v, want ok", overviewRes)
	}
	if overviewRes.Payload["total_steps"] != float64(2) && overviewRes.Payload["total_steps"] != 2 {
		t.Fatalf("overview total_steps = %#v, want 2", overviewRes.Payload["total_steps"])
	}
	if overviewRes.Payload["completed_steps"] != float64(1) && overviewRes.Payload["completed_steps"] != 1 {
		t.Fatalf("overview completed_steps = %#v, want 1", overviewRes.Payload["completed_steps"])
	}
	if overviewRes.Payload["failed_steps"] != float64(0) && overviewRes.Payload["failed_steps"] != 0 {
		t.Fatalf("overview failed_steps = %#v, want 0", overviewRes.Payload["failed_steps"])
	}
	if overviewRes.Payload["ready_steps"] != float64(1) && overviewRes.Payload["ready_steps"] != 1 {
		t.Fatalf("overview ready_steps = %#v, want 1", overviewRes.Payload["ready_steps"])
	}
	if overviewRes.Payload["pending_steps"] != float64(0) && overviewRes.Payload["pending_steps"] != 0 {
		t.Fatalf("overview pending_steps = %#v, want 0", overviewRes.Payload["pending_steps"])
	}
	if overviewRes.Payload["in_progress_steps"] != float64(0) && overviewRes.Payload["in_progress_steps"] != 0 {
		t.Fatalf("overview in_progress_steps = %#v, want 0", overviewRes.Payload["in_progress_steps"])
	}
	if overviewRes.Payload["progress_percent"] != float64(50) && overviewRes.Payload["progress_percent"] != 50 {
		t.Fatalf("overview progress_percent = %#v, want 50", overviewRes.Payload["progress_percent"])
	}
	if overviewRes.Payload["has_blocked_steps"] != false {
		t.Fatalf("overview has_blocked_steps = %#v, want false", overviewRes.Payload["has_blocked_steps"])
	}
	stateCounts, ok := overviewRes.Payload["state_counts"].(map[string]any)
	if !ok {
		t.Fatalf("overview state_counts = %#v, want object", overviewRes.Payload["state_counts"])
	}
	if stateCounts["completed"] != float64(1) && stateCounts["completed"] != 1 {
		t.Fatalf("overview completed count = %#v, want 1", stateCounts["completed"])
	}
	if stateCounts["ready"] != float64(1) && stateCounts["ready"] != 1 {
		t.Fatalf("overview ready count = %#v, want 1", stateCounts["ready"])
	}
	if overviewRes.Payload["active_steps"] != float64(1) && overviewRes.Payload["active_steps"] != 1 {
		t.Fatalf("overview active_steps = %#v, want 1", overviewRes.Payload["active_steps"])
	}
	if overviewRes.Payload["terminal_steps"] != float64(1) && overviewRes.Payload["terminal_steps"] != 1 {
		t.Fatalf("overview terminal_steps = %#v, want 1", overviewRes.Payload["terminal_steps"])
	}
	if overviewRes.Payload["latest_active_action"] != secondActionID {
		t.Fatalf("overview latest_active_action = %#v, want %q", overviewRes.Payload["latest_active_action"], secondActionID)
	}
	if overviewRes.Payload["latest_ready_action"] != secondActionID {
		t.Fatalf("overview latest_ready_action = %#v, want %q", overviewRes.Payload["latest_ready_action"], secondActionID)
	}
	if overviewRes.Payload["latest_in_progress_action"] != "" {
		t.Fatalf("overview latest_in_progress_action = %#v, want empty", overviewRes.Payload["latest_in_progress_action"])
	}
	if overviewRes.Payload["latest_pending_action"] != "" {
		t.Fatalf("overview latest_pending_action = %#v, want empty", overviewRes.Payload["latest_pending_action"])
	}
	if overviewRes.Payload["latest_terminal_action"] != actionID {
		t.Fatalf("overview latest_terminal_action = %#v, want %q", overviewRes.Payload["latest_terminal_action"], actionID)
	}
	if overviewRes.Payload["latest_completed_action"] != actionID {
		t.Fatalf("overview latest_completed_action = %#v, want %q", overviewRes.Payload["latest_completed_action"], actionID)
	}
	if overviewRes.Payload["latest_failed_action"] != "" {
		t.Fatalf("overview latest_failed_action = %#v, want empty", overviewRes.Payload["latest_failed_action"])
	}
	if overviewRes.Payload["latest_blocked_action"] != "" {
		t.Fatalf("overview latest_blocked_action = %#v, want empty", overviewRes.Payload["latest_blocked_action"])
	}
	if _, ok := overviewRes.Payload["last_updated_at"].(string); !ok {
		t.Fatalf("overview last_updated_at = %#v, want string", overviewRes.Payload["last_updated_at"])
	}
}

func TestHandleWebSocketOrchestrationPlanOverviewExposesLatestInProgressAction(t *testing.T) {
	sessionManager := session.NewManager(nil)
	server := NewServerWithOptions(log.New(io.Discard, "", 0), sessionManager, llm.NewMockClient(), Options{
		PermissionPolicy: permissions.Policy{Mode: permissions.ModeAsk},
	})
	httpServer := httptest.NewServer(http.HandlerFunc(server.HandleWebSocket))
	t.Cleanup(httpServer.Close)

	wsURL := "ws" + strings.TrimPrefix(httpServer.URL, "http")
	conn, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
	if err != nil {
		t.Fatalf("dial websocket: %v", err)
	}
	t.Cleanup(func() { _ = conn.Close() })

	if err := conn.WriteJSON(protocolws.Message{
		Type:   protocolws.TypeRequest,
		ID:     "1",
		Method: protocolws.MethodConnect,
		Payload: map[string]any{
			"role":            "client",
			"client_identity": "web-ui",
			"agent_id":        "main",
		},
	}); err != nil {
		t.Fatalf("write connect: %v", err)
	}
	_ = conn.ReadJSON(&protocolws.Message{})
	_ = conn.ReadJSON(&protocolws.Message{})

	if err := conn.WriteJSON(protocolws.Message{
		Type:   protocolws.TypeRequest,
		ID:     "2",
		Method: protocolws.MethodSendMessage,
		Payload: map[string]any{
			"content": "tool run pwd",
		},
	}); err != nil {
		t.Fatalf("write approval-producing message: %v", err)
	}
	_ = waitForPermissionRequired(t, conn)

	if err := conn.WriteJSON(protocolws.Message{
		Type:   protocolws.TypeRequest,
		ID:     "3",
		Method: protocolws.MethodSendMessage,
		Payload: map[string]any{
			"content": "hello",
		},
	}); err != nil {
		t.Fatalf("write plain message: %v", err)
	}
	for i := 0; i < 32; i++ {
		var event protocolws.Message
		if err := conn.ReadJSON(&event); err != nil {
			t.Fatalf("read plain runtime event %d: %v", i, err)
		}
		if event.Type == protocolws.TypeEvent && event.Event == "agent.lifecycle.end" {
			break
		}
	}

	if err := conn.WriteJSON(protocolws.Message{
		Type:   protocolws.TypeRequest,
		ID:     "4",
		Method: protocolws.MethodOrchestrationPlan,
	}); err != nil {
		t.Fatalf("write orchestration_plan: %v", err)
	}
	var planRes protocolws.Message
	if err := conn.ReadJSON(&planRes); err != nil {
		t.Fatalf("read orchestration_plan: %v", err)
	}
	steps, ok := planRes.Payload["steps"].([]any)
	if !ok || len(steps) < 2 {
		t.Fatalf("plan steps payload = %#v, want at least two items", planRes.Payload["steps"])
	}
	first, _ := steps[0].(map[string]any)
	second, _ := steps[1].(map[string]any)
	firstActionID, _ := first["action_id"].(string)
	secondActionID, _ := second["action_id"].(string)

	if err := conn.WriteJSON(protocolws.Message{
		Type:   protocolws.TypeRequest,
		ID:     "5",
		Method: protocolws.MethodOrchestrationPlanStepUpdate,
		Payload: map[string]any{
			"action_id": firstActionID,
			"state":     "completed",
			"result":    "approved",
		},
	}); err != nil {
		t.Fatalf("write first plan step update: %v", err)
	}
	var firstUpdateRes protocolws.Message
	if err := conn.ReadJSON(&firstUpdateRes); err != nil {
		t.Fatalf("read first plan step update: %v", err)
	}
	if !firstUpdateRes.OK {
		t.Fatalf("first plan step update response = %#v, want ok", firstUpdateRes)
	}
	_ = conn.ReadJSON(&protocolws.Message{})

	if err := conn.WriteJSON(protocolws.Message{
		Type:   protocolws.TypeRequest,
		ID:     "6",
		Method: protocolws.MethodOrchestrationPlanStepUpdate,
		Payload: map[string]any{
			"action_id": secondActionID,
			"state":     "in_progress",
			"result":    "reviewing",
		},
	}); err != nil {
		t.Fatalf("write second plan step update: %v", err)
	}
	var secondUpdateRes protocolws.Message
	if err := conn.ReadJSON(&secondUpdateRes); err != nil {
		t.Fatalf("read second plan step update: %v", err)
	}
	if !secondUpdateRes.OK {
		t.Fatalf("second plan step update response = %#v, want ok", secondUpdateRes)
	}
	_ = conn.ReadJSON(&protocolws.Message{})

	if err := conn.WriteJSON(protocolws.Message{
		Type:   protocolws.TypeRequest,
		ID:     "7",
		Method: protocolws.MethodOrchestrationPlanOverview,
	}); err != nil {
		t.Fatalf("write orchestration_plan_overview: %v", err)
	}
	var overviewRes protocolws.Message
	if err := conn.ReadJSON(&overviewRes); err != nil {
		t.Fatalf("read orchestration_plan_overview: %v", err)
	}
	if !overviewRes.OK {
		t.Fatalf("plan overview response = %#v, want ok", overviewRes)
	}
	if overviewRes.Payload["in_progress_steps"] != float64(1) && overviewRes.Payload["in_progress_steps"] != 1 {
		t.Fatalf("overview in_progress_steps = %#v, want 1", overviewRes.Payload["in_progress_steps"])
	}
	if overviewRes.Payload["ready_steps"] != float64(0) && overviewRes.Payload["ready_steps"] != 0 {
		t.Fatalf("overview ready_steps = %#v, want 0", overviewRes.Payload["ready_steps"])
	}
	if overviewRes.Payload["latest_in_progress_action"] != secondActionID {
		t.Fatalf("overview latest_in_progress_action = %#v, want %q", overviewRes.Payload["latest_in_progress_action"], secondActionID)
	}
	if overviewRes.Payload["latest_ready_action"] != "" {
		t.Fatalf("overview latest_ready_action = %#v, want empty", overviewRes.Payload["latest_ready_action"])
	}
	if overviewRes.Payload["latest_active_action"] != secondActionID {
		t.Fatalf("overview latest_active_action = %#v, want %q", overviewRes.Payload["latest_active_action"], secondActionID)
	}
}

func TestHandleWebSocketOrchestrationPlanOverviewExposesLatestBlockedAction(t *testing.T) {
	sessionManager := session.NewManager(nil)
	server := NewServerWithOptions(log.New(io.Discard, "", 0), sessionManager, llm.NewMockClient(), Options{
		PermissionPolicy: permissions.Policy{Mode: permissions.ModeAsk},
	})
	httpServer := httptest.NewServer(http.HandlerFunc(server.HandleWebSocket))
	t.Cleanup(httpServer.Close)

	wsURL := "ws" + strings.TrimPrefix(httpServer.URL, "http")
	conn, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
	if err != nil {
		t.Fatalf("dial websocket: %v", err)
	}
	t.Cleanup(func() { _ = conn.Close() })

	if err := conn.WriteJSON(protocolws.Message{
		Type:   protocolws.TypeRequest,
		ID:     "1",
		Method: protocolws.MethodConnect,
		Payload: map[string]any{
			"role":            "client",
			"client_identity": "web-ui",
			"agent_id":        "main",
		},
	}); err != nil {
		t.Fatalf("write connect: %v", err)
	}
	_ = conn.ReadJSON(&protocolws.Message{})
	_ = conn.ReadJSON(&protocolws.Message{})

	if err := conn.WriteJSON(protocolws.Message{
		Type:   protocolws.TypeRequest,
		ID:     "2",
		Method: protocolws.MethodSendMessage,
		Payload: map[string]any{
			"content": "tool run pwd",
		},
	}); err != nil {
		t.Fatalf("write blocked message: %v", err)
	}
	_ = waitForPermissionRequired(t, conn)

	if err := conn.WriteJSON(protocolws.Message{
		Type:   protocolws.TypeRequest,
		ID:     "3",
		Method: protocolws.MethodSendMessage,
		Payload: map[string]any{
			"content": "tool upper hi",
		},
	}); err != nil {
		t.Fatalf("write second message: %v", err)
	}
	for i := 0; i < 32; i++ {
		var event protocolws.Message
		if err := conn.ReadJSON(&event); err != nil {
			t.Fatalf("read second runtime event %d: %v", i, err)
		}
		if event.Type == protocolws.TypeEvent && event.Event == "agent.lifecycle.end" {
			break
		}
	}

	if err := conn.WriteJSON(protocolws.Message{
		Type:   protocolws.TypeRequest,
		ID:     "4",
		Method: protocolws.MethodOrchestrationPlanOverview,
	}); err != nil {
		t.Fatalf("write orchestration_plan_overview: %v", err)
	}
	var overviewRes protocolws.Message
	if err := conn.ReadJSON(&overviewRes); err != nil {
		t.Fatalf("read orchestration_plan_overview: %v", err)
	}
	if !overviewRes.OK {
		t.Fatalf("plan overview response = %#v, want ok", overviewRes)
	}
	if overviewRes.Payload["failed_steps"] != float64(0) && overviewRes.Payload["failed_steps"] != 0 {
		t.Fatalf("overview failed_steps = %#v, want 0", overviewRes.Payload["failed_steps"])
	}
	if overviewRes.Payload["ready_steps"] != float64(0) && overviewRes.Payload["ready_steps"] != 0 {
		t.Fatalf("overview ready_steps = %#v, want 0", overviewRes.Payload["ready_steps"])
	}
	if overviewRes.Payload["pending_steps"] != float64(1) && overviewRes.Payload["pending_steps"] != 1 {
		t.Fatalf("overview pending_steps = %#v, want 1", overviewRes.Payload["pending_steps"])
	}
	if overviewRes.Payload["in_progress_steps"] != float64(0) && overviewRes.Payload["in_progress_steps"] != 0 {
		t.Fatalf("overview in_progress_steps = %#v, want 0", overviewRes.Payload["in_progress_steps"])
	}
	if overviewRes.Payload["latest_blocked_action"] != "run-000001:request_human_approval" {
		t.Fatalf("overview latest_blocked_action = %#v, want %q", overviewRes.Payload["latest_blocked_action"], "run-000001:request_human_approval")
	}
	if overviewRes.Payload["latest_ready_action"] != "" {
		t.Fatalf("overview latest_ready_action = %#v, want empty", overviewRes.Payload["latest_ready_action"])
	}
	if overviewRes.Payload["latest_in_progress_action"] != "" {
		t.Fatalf("overview latest_in_progress_action = %#v, want empty", overviewRes.Payload["latest_in_progress_action"])
	}
	if overviewRes.Payload["latest_pending_action"] != "run-000001:request_human_approval" {
		t.Fatalf("overview latest_pending_action = %#v, want %q", overviewRes.Payload["latest_pending_action"], "run-000001:request_human_approval")
	}
	if overviewRes.Payload["latest_failed_action"] != "" {
		t.Fatalf("overview latest_failed_action = %#v, want empty", overviewRes.Payload["latest_failed_action"])
	}
	if overviewRes.Payload["latest_completed_action"] != "" {
		t.Fatalf("overview latest_completed_action = %#v, want empty", overviewRes.Payload["latest_completed_action"])
	}
}

func TestHandleWebSocketOrchestrationPlanExecutionHistorySummarizesSessionAudit(t *testing.T) {
	sessionManager := session.NewManager(nil)
	server := NewServerWithOptions(log.New(io.Discard, "", 0), sessionManager, llm.NewMockClient(), Options{
		PermissionPolicy: permissions.Policy{Mode: permissions.ModeAsk},
	})
	httpServer := httptest.NewServer(http.HandlerFunc(server.HandleWebSocket))
	t.Cleanup(httpServer.Close)

	wsURL := "ws" + strings.TrimPrefix(httpServer.URL, "http")
	conn, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
	if err != nil {
		t.Fatalf("dial websocket: %v", err)
	}
	t.Cleanup(func() { _ = conn.Close() })

	if err := conn.WriteJSON(protocolws.Message{
		Type:   protocolws.TypeRequest,
		ID:     "1",
		Method: protocolws.MethodConnect,
		Payload: map[string]any{
			"role":            "client",
			"client_identity": "web-ui",
			"agent_id":        "main",
		},
	}); err != nil {
		t.Fatalf("write connect: %v", err)
	}
	_ = conn.ReadJSON(&protocolws.Message{})
	_ = conn.ReadJSON(&protocolws.Message{})

	if err := conn.WriteJSON(protocolws.Message{
		Type:   protocolws.TypeRequest,
		ID:     "2",
		Method: protocolws.MethodSendMessage,
		Payload: map[string]any{
			"content": "tool run pwd",
		},
	}); err != nil {
		t.Fatalf("write approval-producing message: %v", err)
	}
	_ = waitForPermissionRequired(t, conn)

	if err := conn.WriteJSON(protocolws.Message{
		Type:   protocolws.TypeRequest,
		ID:     "3",
		Method: protocolws.MethodSendMessage,
		Payload: map[string]any{
			"content": "hello",
		},
	}); err != nil {
		t.Fatalf("write plain message: %v", err)
	}
	for i := 0; i < 32; i++ {
		var event protocolws.Message
		if err := conn.ReadJSON(&event); err != nil {
			t.Fatalf("read plain runtime event %d: %v", i, err)
		}
		if event.Type == protocolws.TypeEvent && event.Event == "agent.lifecycle.end" {
			break
		}
	}

	if err := conn.WriteJSON(protocolws.Message{
		Type:   protocolws.TypeRequest,
		ID:     "4",
		Method: protocolws.MethodOrchestrationPlan,
	}); err != nil {
		t.Fatalf("write orchestration_plan: %v", err)
	}
	var planRes protocolws.Message
	if err := conn.ReadJSON(&planRes); err != nil {
		t.Fatalf("read orchestration_plan: %v", err)
	}
	steps, ok := planRes.Payload["steps"].([]any)
	if !ok || len(steps) < 2 {
		t.Fatalf("plan steps payload = %#v, want at least two items", planRes.Payload["steps"])
	}
	first, _ := steps[0].(map[string]any)
	second, _ := steps[1].(map[string]any)
	firstActionID, _ := first["action_id"].(string)
	secondActionID, _ := second["action_id"].(string)
	if firstActionID == "" || secondActionID == "" {
		t.Fatal("expected action ids")
	}

	for idx, update := range []map[string]any{
		{"action_id": firstActionID, "state": "completed", "result": "approved"},
		{"action_id": secondActionID, "state": "in_progress", "result": "reviewing"},
	} {
		if err := conn.WriteJSON(protocolws.Message{
			Type:    protocolws.TypeRequest,
			ID:      string(rune('5' + idx)),
			Method:  protocolws.MethodOrchestrationPlanStepUpdate,
			Payload: update,
		}); err != nil {
			t.Fatalf("write plan step update %d: %v", idx, err)
		}
		var updateRes protocolws.Message
		if err := conn.ReadJSON(&updateRes); err != nil {
			t.Fatalf("read plan step update %d: %v", idx, err)
		}
		if !updateRes.OK {
			t.Fatalf("plan step update %d response = %#v, want ok", idx, updateRes)
		}
		_ = conn.ReadJSON(&protocolws.Message{})
	}

	if err := conn.WriteJSON(protocolws.Message{
		Type:   protocolws.TypeRequest,
		ID:     "7",
		Method: protocolws.MethodOrchestrationPlanExecutionHistory,
	}); err != nil {
		t.Fatalf("write orchestration_plan_execution_history: %v", err)
	}
	var historyRes protocolws.Message
	if err := conn.ReadJSON(&historyRes); err != nil {
		t.Fatalf("read orchestration_plan_execution_history: %v", err)
	}
	if !historyRes.OK {
		t.Fatalf("plan execution history response = %#v, want ok", historyRes)
	}
	history, ok := historyRes.Payload["history"].([]any)
	if !ok || len(history) != 2 {
		t.Fatalf("session execution history payload = %#v, want two items", historyRes.Payload["history"])
	}
	summary, ok := historyRes.Payload["summary"].(map[string]any)
	if !ok {
		t.Fatalf("session execution summary = %#v, want object", historyRes.Payload["summary"])
	}
	if summary["record_count"] != float64(2) && summary["record_count"] != 2 {
		t.Fatalf("summary record_count = %#v, want 2", summary["record_count"])
	}
	stateCounts, ok := summary["state_counts"].(map[string]any)
	if !ok {
		t.Fatalf("summary state_counts = %#v, want object", summary["state_counts"])
	}
	if stateCounts["completed"] != float64(1) && stateCounts["completed"] != 1 {
		t.Fatalf("summary completed count = %#v, want 1", stateCounts["completed"])
	}
	if stateCounts["in_progress"] != float64(1) && stateCounts["in_progress"] != 1 {
		t.Fatalf("summary in_progress count = %#v, want 1", stateCounts["in_progress"])
	}
	actionCounts, ok := summary["action_counts"].(map[string]any)
	if !ok {
		t.Fatalf("summary action_counts = %#v, want object", summary["action_counts"])
	}
	if actionCounts[firstActionID] != float64(1) && actionCounts[firstActionID] != 1 {
		t.Fatalf("summary first action count = %#v, want 1", actionCounts[firstActionID])
	}
	if summary["latest_recorded_action"] != secondActionID {
		t.Fatalf("summary latest_recorded_action = %#v, want %q", summary["latest_recorded_action"], secondActionID)
	}
	if summary["latest_recorded_state"] != "in_progress" {
		t.Fatalf("summary latest_recorded_state = %#v, want in_progress", summary["latest_recorded_state"])
	}
	if summary["latest_recorded_result"] != "reviewing" {
		t.Fatalf("summary latest_recorded_result = %#v, want reviewing", summary["latest_recorded_result"])
	}
	if summary["latest_ready_action"] != "" {
		t.Fatalf("summary latest_ready_action = %#v, want empty", summary["latest_ready_action"])
	}
	if summary["latest_ready_at"] != "" {
		t.Fatalf("summary latest_ready_at = %#v, want empty", summary["latest_ready_at"])
	}
	if summary["latest_ready_result"] != "" {
		t.Fatalf("summary latest_ready_result = %#v, want empty", summary["latest_ready_result"])
	}
	if summary["latest_ready_state"] != "" {
		t.Fatalf("summary latest_ready_state = %#v, want empty", summary["latest_ready_state"])
	}
	if summary["latest_in_progress_action"] != secondActionID {
		t.Fatalf("summary latest_in_progress_action = %#v, want %q", summary["latest_in_progress_action"], secondActionID)
	}
	if _, ok := summary["latest_in_progress_at"].(string); !ok {
		t.Fatalf("summary latest_in_progress_at = %#v, want string", summary["latest_in_progress_at"])
	}
	if summary["latest_in_progress_result"] != "reviewing" {
		t.Fatalf("summary latest_in_progress_result = %#v, want reviewing", summary["latest_in_progress_result"])
	}
	if summary["latest_in_progress_state"] != "in_progress" {
		t.Fatalf("summary latest_in_progress_state = %#v, want in_progress", summary["latest_in_progress_state"])
	}
	if summary["latest_pending_action"] != "" {
		t.Fatalf("summary latest_pending_action = %#v, want empty", summary["latest_pending_action"])
	}
	if summary["latest_pending_at"] != "" {
		t.Fatalf("summary latest_pending_at = %#v, want empty", summary["latest_pending_at"])
	}
	if summary["latest_pending_result"] != "" {
		t.Fatalf("summary latest_pending_result = %#v, want empty", summary["latest_pending_result"])
	}
	if summary["latest_pending_state"] != "" {
		t.Fatalf("summary latest_pending_state = %#v, want empty", summary["latest_pending_state"])
	}
	if summary["latest_terminal_action"] != firstActionID {
		t.Fatalf("summary latest_terminal_action = %#v, want %q", summary["latest_terminal_action"], firstActionID)
	}
	if summary["latest_terminal_state"] != "completed" {
		t.Fatalf("summary latest_terminal_state = %#v, want completed", summary["latest_terminal_state"])
	}
	if summary["latest_terminal_result"] != "approved" {
		t.Fatalf("summary latest_terminal_result = %#v, want approved", summary["latest_terminal_result"])
	}
	if _, ok := summary["latest_terminal_at"].(string); !ok {
		t.Fatalf("summary latest_terminal_at = %#v, want string", summary["latest_terminal_at"])
	}
	if summary["latest_completed_result"] != "approved" {
		t.Fatalf("summary latest_completed_result = %#v, want approved", summary["latest_completed_result"])
	}
	if summary["latest_completed_state"] != "completed" {
		t.Fatalf("summary latest_completed_state = %#v, want completed", summary["latest_completed_state"])
	}
	if _, ok := summary["latest_completed_at"].(string); !ok {
		t.Fatalf("summary latest_completed_at = %#v, want string", summary["latest_completed_at"])
	}
	if _, ok := summary["last_recorded_at"].(string); !ok {
		t.Fatalf("summary last_recorded_at = %#v, want string", summary["last_recorded_at"])
	}
}

func TestHandleWebSocketOrchestrationPlanExecutionHistoryCanFilterAndExposeLatestActiveAction(t *testing.T) {
	sessionManager := session.NewManager(nil)
	server := NewServerWithOptions(log.New(io.Discard, "", 0), sessionManager, llm.NewMockClient(), Options{
		PermissionPolicy: permissions.Policy{Mode: permissions.ModeAsk},
	})
	httpServer := httptest.NewServer(http.HandlerFunc(server.HandleWebSocket))
	t.Cleanup(httpServer.Close)

	wsURL := "ws" + strings.TrimPrefix(httpServer.URL, "http")
	conn, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
	if err != nil {
		t.Fatalf("dial websocket: %v", err)
	}
	t.Cleanup(func() { _ = conn.Close() })

	if err := conn.WriteJSON(protocolws.Message{
		Type:   protocolws.TypeRequest,
		ID:     "1",
		Method: protocolws.MethodConnect,
		Payload: map[string]any{
			"role":            "client",
			"client_identity": "web-ui",
			"agent_id":        "main",
		},
	}); err != nil {
		t.Fatalf("write connect: %v", err)
	}
	_ = conn.ReadJSON(&protocolws.Message{})
	_ = conn.ReadJSON(&protocolws.Message{})

	if err := conn.WriteJSON(protocolws.Message{
		Type:   protocolws.TypeRequest,
		ID:     "2",
		Method: protocolws.MethodSendMessage,
		Payload: map[string]any{
			"content": "tool run pwd",
		},
	}); err != nil {
		t.Fatalf("write approval-producing message: %v", err)
	}
	_ = waitForPermissionRequired(t, conn)

	if err := conn.WriteJSON(protocolws.Message{
		Type:   protocolws.TypeRequest,
		ID:     "3",
		Method: protocolws.MethodSendMessage,
		Payload: map[string]any{
			"content": "hello",
		},
	}); err != nil {
		t.Fatalf("write plain message: %v", err)
	}
	for i := 0; i < 32; i++ {
		var event protocolws.Message
		if err := conn.ReadJSON(&event); err != nil {
			t.Fatalf("read plain runtime event %d: %v", i, err)
		}
		if event.Type == protocolws.TypeEvent && event.Event == "agent.lifecycle.end" {
			break
		}
	}

	if err := conn.WriteJSON(protocolws.Message{
		Type:   protocolws.TypeRequest,
		ID:     "4",
		Method: protocolws.MethodOrchestrationPlan,
	}); err != nil {
		t.Fatalf("write orchestration_plan: %v", err)
	}
	var planRes protocolws.Message
	if err := conn.ReadJSON(&planRes); err != nil {
		t.Fatalf("read orchestration_plan: %v", err)
	}
	steps, ok := planRes.Payload["steps"].([]any)
	if !ok || len(steps) < 2 {
		t.Fatalf("plan steps payload = %#v, want at least two items", planRes.Payload["steps"])
	}
	first, _ := steps[0].(map[string]any)
	second, _ := steps[1].(map[string]any)
	firstActionID, _ := first["action_id"].(string)
	secondActionID, _ := second["action_id"].(string)
	if firstActionID == "" || secondActionID == "" {
		t.Fatal("expected action ids")
	}

	for idx, update := range []map[string]any{
		{"action_id": firstActionID, "state": "completed", "result": "approved"},
		{"action_id": secondActionID, "state": "in_progress", "result": "reviewing"},
	} {
		if err := conn.WriteJSON(protocolws.Message{
			Type:    protocolws.TypeRequest,
			ID:      string(rune('5' + idx)),
			Method:  protocolws.MethodOrchestrationPlanStepUpdate,
			Payload: update,
		}); err != nil {
			t.Fatalf("write plan step update %d: %v", idx, err)
		}
		var updateRes protocolws.Message
		if err := conn.ReadJSON(&updateRes); err != nil {
			t.Fatalf("read plan step update %d: %v", idx, err)
		}
		if !updateRes.OK {
			t.Fatalf("plan step update %d response = %#v, want ok", idx, updateRes)
		}
		_ = conn.ReadJSON(&protocolws.Message{})
	}

	if err := conn.WriteJSON(protocolws.Message{
		Type:   protocolws.TypeRequest,
		ID:     "7",
		Method: protocolws.MethodOrchestrationPlanExecutionHistory,
		Payload: map[string]any{
			"state": "in_progress",
		},
	}); err != nil {
		t.Fatalf("write filtered orchestration_plan_execution_history: %v", err)
	}
	var historyRes protocolws.Message
	if err := conn.ReadJSON(&historyRes); err != nil {
		t.Fatalf("read filtered orchestration_plan_execution_history: %v", err)
	}
	if !historyRes.OK {
		t.Fatalf("filtered plan execution history response = %#v, want ok", historyRes)
	}
	history, ok := historyRes.Payload["history"].([]any)
	if !ok || len(history) != 1 {
		t.Fatalf("filtered session execution history payload = %#v, want one item", historyRes.Payload["history"])
	}
	record, _ := history[0].(map[string]any)
	if record["action_id"] != secondActionID {
		t.Fatalf("filtered history action_id = %#v, want %q", record["action_id"], secondActionID)
	}
	summary, ok := historyRes.Payload["summary"].(map[string]any)
	if !ok {
		t.Fatalf("filtered session execution summary = %#v, want object", historyRes.Payload["summary"])
	}
	if summary["latest_active_action"] != secondActionID {
		t.Fatalf("summary latest_active_action = %#v, want %q", summary["latest_active_action"], secondActionID)
	}
	if _, ok := summary["latest_active_at"].(string); !ok {
		t.Fatalf("summary latest_active_at = %#v, want string", summary["latest_active_at"])
	}
	if summary["latest_active_result"] != "reviewing" {
		t.Fatalf("summary latest_active_result = %#v, want reviewing", summary["latest_active_result"])
	}
	if summary["latest_active_state"] != "in_progress" {
		t.Fatalf("summary latest_active_state = %#v, want in_progress", summary["latest_active_state"])
	}
	if summary["latest_recorded_action"] != secondActionID {
		t.Fatalf("summary latest_recorded_action = %#v, want %q", summary["latest_recorded_action"], secondActionID)
	}
	if summary["latest_recorded_state"] != "in_progress" {
		t.Fatalf("summary latest_recorded_state = %#v, want in_progress", summary["latest_recorded_state"])
	}
	if summary["latest_recorded_result"] != "reviewing" {
		t.Fatalf("summary latest_recorded_result = %#v, want reviewing", summary["latest_recorded_result"])
	}
	if summary["latest_ready_action"] != "" {
		t.Fatalf("summary latest_ready_action = %#v, want empty", summary["latest_ready_action"])
	}
	if summary["latest_ready_result"] != "" {
		t.Fatalf("summary latest_ready_result = %#v, want empty", summary["latest_ready_result"])
	}
	if summary["latest_in_progress_action"] != secondActionID {
		t.Fatalf("summary latest_in_progress_action = %#v, want %q", summary["latest_in_progress_action"], secondActionID)
	}
	if summary["latest_pending_action"] != "" {
		t.Fatalf("summary latest_pending_action = %#v, want empty", summary["latest_pending_action"])
	}
	if summary["latest_completed_action"] != "" {
		t.Fatalf("summary latest_completed_action = %#v, want empty", summary["latest_completed_action"])
	}
	if summary["latest_failed_action"] != "" {
		t.Fatalf("summary latest_failed_action = %#v, want empty", summary["latest_failed_action"])
	}
	if summary["latest_terminal_action"] != "" {
		t.Fatalf("summary latest_terminal_action = %#v, want empty", summary["latest_terminal_action"])
	}
	if summary["latest_terminal_state"] != "" {
		t.Fatalf("summary latest_terminal_state = %#v, want empty", summary["latest_terminal_state"])
	}
	if summary["latest_terminal_result"] != "" {
		t.Fatalf("summary latest_terminal_result = %#v, want empty", summary["latest_terminal_result"])
	}
}

func TestHandleWebSocketOrchestrationPlanExecutionHistoryExposesLatestTerminalActions(t *testing.T) {
	sessionManager := session.NewManager(nil)
	server := NewServerWithOptions(log.New(io.Discard, "", 0), sessionManager, llm.NewMockClient(), Options{
		PermissionPolicy: permissions.Policy{Mode: permissions.ModeAsk},
	})
	httpServer := httptest.NewServer(http.HandlerFunc(server.HandleWebSocket))
	t.Cleanup(httpServer.Close)

	wsURL := "ws" + strings.TrimPrefix(httpServer.URL, "http")
	conn, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
	if err != nil {
		t.Fatalf("dial websocket: %v", err)
	}
	t.Cleanup(func() { _ = conn.Close() })

	if err := conn.WriteJSON(protocolws.Message{
		Type:   protocolws.TypeRequest,
		ID:     "1",
		Method: protocolws.MethodConnect,
		Payload: map[string]any{
			"role":            "client",
			"client_identity": "web-ui",
			"agent_id":        "main",
		},
	}); err != nil {
		t.Fatalf("write connect: %v", err)
	}
	_ = conn.ReadJSON(&protocolws.Message{})
	_ = conn.ReadJSON(&protocolws.Message{})

	if err := conn.WriteJSON(protocolws.Message{
		Type:   protocolws.TypeRequest,
		ID:     "2",
		Method: protocolws.MethodSendMessage,
		Payload: map[string]any{
			"content": "tool run pwd",
		},
	}); err != nil {
		t.Fatalf("write approval-producing message: %v", err)
	}
	_ = waitForPermissionRequired(t, conn)

	if err := conn.WriteJSON(protocolws.Message{
		Type:   protocolws.TypeRequest,
		ID:     "3",
		Method: protocolws.MethodSendMessage,
		Payload: map[string]any{
			"content": "hello",
		},
	}); err != nil {
		t.Fatalf("write plain message: %v", err)
	}
	for i := 0; i < 32; i++ {
		var event protocolws.Message
		if err := conn.ReadJSON(&event); err != nil {
			t.Fatalf("read plain runtime event %d: %v", i, err)
		}
		if event.Type == protocolws.TypeEvent && event.Event == "agent.lifecycle.end" {
			break
		}
	}

	if err := conn.WriteJSON(protocolws.Message{
		Type:   protocolws.TypeRequest,
		ID:     "4",
		Method: protocolws.MethodOrchestrationPlan,
	}); err != nil {
		t.Fatalf("write orchestration_plan: %v", err)
	}
	var planRes protocolws.Message
	if err := conn.ReadJSON(&planRes); err != nil {
		t.Fatalf("read orchestration_plan: %v", err)
	}
	steps, ok := planRes.Payload["steps"].([]any)
	if !ok || len(steps) < 2 {
		t.Fatalf("plan steps payload = %#v, want at least two items", planRes.Payload["steps"])
	}
	first, _ := steps[0].(map[string]any)
	second, _ := steps[1].(map[string]any)
	firstActionID, _ := first["action_id"].(string)
	secondActionID, _ := second["action_id"].(string)

	if err := conn.WriteJSON(protocolws.Message{
		Type:   protocolws.TypeRequest,
		ID:     "5",
		Method: protocolws.MethodOrchestrationPlanStepUpdate,
		Payload: map[string]any{
			"action_id": firstActionID,
			"state":     "completed",
			"result":    "approved",
		},
	}); err != nil {
		t.Fatalf("write first plan step update: %v", err)
	}
	var firstUpdateRes protocolws.Message
	if err := conn.ReadJSON(&firstUpdateRes); err != nil {
		t.Fatalf("read first plan step update: %v", err)
	}
	if !firstUpdateRes.OK {
		t.Fatalf("first plan step update response = %#v, want ok", firstUpdateRes)
	}
	_ = conn.ReadJSON(&protocolws.Message{})

	if err := conn.WriteJSON(protocolws.Message{
		Type:   protocolws.TypeRequest,
		ID:     "6",
		Method: protocolws.MethodOrchestrationPlanStepUpdate,
		Payload: map[string]any{
			"action_id": secondActionID,
			"state":     "failed",
			"result":    "review failed",
		},
	}); err != nil {
		t.Fatalf("write second plan step update: %v", err)
	}
	var secondUpdateRes protocolws.Message
	if err := conn.ReadJSON(&secondUpdateRes); err != nil {
		t.Fatalf("read second plan step update: %v", err)
	}
	if !secondUpdateRes.OK {
		t.Fatalf("second plan step update response = %#v, want ok", secondUpdateRes)
	}
	_ = conn.ReadJSON(&protocolws.Message{})

	if err := conn.WriteJSON(protocolws.Message{
		Type:   protocolws.TypeRequest,
		ID:     "7",
		Method: protocolws.MethodOrchestrationPlanExecutionHistory,
	}); err != nil {
		t.Fatalf("write orchestration_plan_execution_history: %v", err)
	}
	var historyRes protocolws.Message
	if err := conn.ReadJSON(&historyRes); err != nil {
		t.Fatalf("read orchestration_plan_execution_history: %v", err)
	}
	if !historyRes.OK {
		t.Fatalf("plan execution history response = %#v, want ok", historyRes)
	}
	summary, ok := historyRes.Payload["summary"].(map[string]any)
	if !ok {
		t.Fatalf("summary payload = %#v, want object", historyRes.Payload["summary"])
	}
	if summary["latest_completed_action"] != firstActionID {
		t.Fatalf("summary latest_completed_action = %#v, want %q", summary["latest_completed_action"], firstActionID)
	}
	if summary["latest_failed_action"] != secondActionID {
		t.Fatalf("summary latest_failed_action = %#v, want %q", summary["latest_failed_action"], secondActionID)
	}
	if summary["latest_recorded_action"] != secondActionID {
		t.Fatalf("summary latest_recorded_action = %#v, want %q", summary["latest_recorded_action"], secondActionID)
	}
	if summary["latest_recorded_state"] != "failed" {
		t.Fatalf("summary latest_recorded_state = %#v, want failed", summary["latest_recorded_state"])
	}
	if summary["latest_recorded_result"] != "review failed" {
		t.Fatalf("summary latest_recorded_result = %#v, want review failed", summary["latest_recorded_result"])
	}
	if summary["latest_terminal_action"] != secondActionID {
		t.Fatalf("summary latest_terminal_action = %#v, want %q", summary["latest_terminal_action"], secondActionID)
	}
	if summary["latest_terminal_state"] != "failed" {
		t.Fatalf("summary latest_terminal_state = %#v, want failed", summary["latest_terminal_state"])
	}
	if summary["latest_terminal_result"] != "review failed" {
		t.Fatalf("summary latest_terminal_result = %#v, want review failed", summary["latest_terminal_result"])
	}
	if _, ok := summary["latest_terminal_at"].(string); !ok {
		t.Fatalf("summary latest_terminal_at = %#v, want string", summary["latest_terminal_at"])
	}
	if summary["latest_completed_result"] != "approved" {
		t.Fatalf("summary latest_completed_result = %#v, want approved", summary["latest_completed_result"])
	}
	if summary["latest_failed_result"] != "review failed" {
		t.Fatalf("summary latest_failed_result = %#v, want review failed", summary["latest_failed_result"])
	}
	if summary["latest_failed_state"] != "failed" {
		t.Fatalf("summary latest_failed_state = %#v, want failed", summary["latest_failed_state"])
	}
	if _, ok := summary["latest_completed_at"].(string); !ok {
		t.Fatalf("summary latest_completed_at = %#v, want string", summary["latest_completed_at"])
	}
	if _, ok := summary["latest_failed_at"].(string); !ok {
		t.Fatalf("summary latest_failed_at = %#v, want string", summary["latest_failed_at"])
	}
	if summary["latest_ready_action"] != "" {
		t.Fatalf("summary latest_ready_action = %#v, want empty", summary["latest_ready_action"])
	}
	if summary["latest_ready_at"] != "" {
		t.Fatalf("summary latest_ready_at = %#v, want empty", summary["latest_ready_at"])
	}
	if summary["latest_ready_state"] != "" {
		t.Fatalf("summary latest_ready_state = %#v, want empty", summary["latest_ready_state"])
	}
	if summary["latest_in_progress_action"] != "" {
		t.Fatalf("summary latest_in_progress_action = %#v, want empty", summary["latest_in_progress_action"])
	}
	if summary["latest_in_progress_at"] != "" {
		t.Fatalf("summary latest_in_progress_at = %#v, want empty", summary["latest_in_progress_at"])
	}
	if summary["latest_in_progress_result"] != "" {
		t.Fatalf("summary latest_in_progress_result = %#v, want empty", summary["latest_in_progress_result"])
	}
	if summary["latest_in_progress_state"] != "" {
		t.Fatalf("summary latest_in_progress_state = %#v, want empty", summary["latest_in_progress_state"])
	}
	if summary["latest_pending_action"] != "" {
		t.Fatalf("summary latest_pending_action = %#v, want empty", summary["latest_pending_action"])
	}
	if summary["latest_pending_at"] != "" {
		t.Fatalf("summary latest_pending_at = %#v, want empty", summary["latest_pending_at"])
	}
	if summary["latest_pending_result"] != "" {
		t.Fatalf("summary latest_pending_result = %#v, want empty", summary["latest_pending_result"])
	}
	if summary["latest_pending_state"] != "" {
		t.Fatalf("summary latest_pending_state = %#v, want empty", summary["latest_pending_state"])
	}
	if summary["latest_active_action"] != "" {
		t.Fatalf("summary latest_active_action = %#v, want empty", summary["latest_active_action"])
	}
	if summary["latest_active_at"] != "" {
		t.Fatalf("summary latest_active_at = %#v, want empty", summary["latest_active_at"])
	}
	if summary["latest_active_result"] != "" {
		t.Fatalf("summary latest_active_result = %#v, want empty", summary["latest_active_result"])
	}
	if summary["latest_active_state"] != "" {
		t.Fatalf("summary latest_active_state = %#v, want empty", summary["latest_active_state"])
	}
}

func TestHandleWebSocketOrchestrationPlanExecutionHistoryCanFilterByActionID(t *testing.T) {
	sessionManager := session.NewManager(nil)
	server := NewServerWithOptions(log.New(io.Discard, "", 0), sessionManager, llm.NewMockClient(), Options{
		PermissionPolicy: permissions.Policy{Mode: permissions.ModeAsk},
	})
	httpServer := httptest.NewServer(http.HandlerFunc(server.HandleWebSocket))
	t.Cleanup(httpServer.Close)

	wsURL := "ws" + strings.TrimPrefix(httpServer.URL, "http")
	conn, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
	if err != nil {
		t.Fatalf("dial websocket: %v", err)
	}
	t.Cleanup(func() { _ = conn.Close() })

	if err := conn.WriteJSON(protocolws.Message{
		Type:   protocolws.TypeRequest,
		ID:     "1",
		Method: protocolws.MethodConnect,
		Payload: map[string]any{
			"role":            "client",
			"client_identity": "web-ui",
			"agent_id":        "main",
		},
	}); err != nil {
		t.Fatalf("write connect: %v", err)
	}
	_ = conn.ReadJSON(&protocolws.Message{})
	_ = conn.ReadJSON(&protocolws.Message{})

	if err := conn.WriteJSON(protocolws.Message{
		Type:   protocolws.TypeRequest,
		ID:     "2",
		Method: protocolws.MethodSendMessage,
		Payload: map[string]any{
			"content": "tool run pwd",
		},
	}); err != nil {
		t.Fatalf("write approval-producing message: %v", err)
	}
	_ = waitForPermissionRequired(t, conn)

	if err := conn.WriteJSON(protocolws.Message{
		Type:   protocolws.TypeRequest,
		ID:     "3",
		Method: protocolws.MethodSendMessage,
		Payload: map[string]any{
			"content": "hello",
		},
	}); err != nil {
		t.Fatalf("write plain message: %v", err)
	}
	for i := 0; i < 32; i++ {
		var event protocolws.Message
		if err := conn.ReadJSON(&event); err != nil {
			t.Fatalf("read plain runtime event %d: %v", i, err)
		}
		if event.Type == protocolws.TypeEvent && event.Event == "agent.lifecycle.end" {
			break
		}
	}

	if err := conn.WriteJSON(protocolws.Message{
		Type:   protocolws.TypeRequest,
		ID:     "4",
		Method: protocolws.MethodOrchestrationPlan,
	}); err != nil {
		t.Fatalf("write orchestration_plan: %v", err)
	}
	var planRes protocolws.Message
	if err := conn.ReadJSON(&planRes); err != nil {
		t.Fatalf("read orchestration_plan: %v", err)
	}
	steps, ok := planRes.Payload["steps"].([]any)
	if !ok || len(steps) < 2 {
		t.Fatalf("plan steps payload = %#v, want at least two items", planRes.Payload["steps"])
	}
	first, _ := steps[0].(map[string]any)
	second, _ := steps[1].(map[string]any)
	firstActionID, _ := first["action_id"].(string)
	secondActionID, _ := second["action_id"].(string)
	if firstActionID == "" || secondActionID == "" {
		t.Fatal("expected action ids")
	}

	for idx, update := range []map[string]any{
		{"action_id": firstActionID, "state": "completed", "result": "approved"},
		{"action_id": secondActionID, "state": "in_progress", "result": "reviewing"},
	} {
		if err := conn.WriteJSON(protocolws.Message{
			Type:    protocolws.TypeRequest,
			ID:      string(rune('5' + idx)),
			Method:  protocolws.MethodOrchestrationPlanStepUpdate,
			Payload: update,
		}); err != nil {
			t.Fatalf("write plan step update %d: %v", idx, err)
		}
		var updateRes protocolws.Message
		if err := conn.ReadJSON(&updateRes); err != nil {
			t.Fatalf("read plan step update %d: %v", idx, err)
		}
		if !updateRes.OK {
			t.Fatalf("plan step update %d response = %#v, want ok", idx, updateRes)
		}
		_ = conn.ReadJSON(&protocolws.Message{})
	}

	if err := conn.WriteJSON(protocolws.Message{
		Type:   protocolws.TypeRequest,
		ID:     "7",
		Method: protocolws.MethodOrchestrationPlanExecutionHistory,
		Payload: map[string]any{
			"action_id": secondActionID,
		},
	}); err != nil {
		t.Fatalf("write action filtered orchestration_plan_execution_history: %v", err)
	}
	var historyRes protocolws.Message
	if err := conn.ReadJSON(&historyRes); err != nil {
		t.Fatalf("read action filtered orchestration_plan_execution_history: %v", err)
	}
	if !historyRes.OK {
		t.Fatalf("action filtered plan execution history response = %#v, want ok", historyRes)
	}
	history, ok := historyRes.Payload["history"].([]any)
	if !ok || len(history) != 1 {
		t.Fatalf("action filtered session execution history payload = %#v, want one item", historyRes.Payload["history"])
	}
	record, _ := history[0].(map[string]any)
	if record["action_id"] != secondActionID {
		t.Fatalf("action filtered history action_id = %#v, want %q", record["action_id"], secondActionID)
	}
	summary, ok := historyRes.Payload["summary"].(map[string]any)
	if !ok {
		t.Fatalf("action filtered summary = %#v, want object", historyRes.Payload["summary"])
	}
	if summary["latest_active_action"] != secondActionID {
		t.Fatalf("action filtered summary latest_active_action = %#v, want %q", summary["latest_active_action"], secondActionID)
	}
}

func TestHandleWebSocketOrchestrationPlanExecutionHistoryCanFilterBySince(t *testing.T) {
	sessionManager := session.NewManager(nil)
	server := NewServerWithOptions(log.New(io.Discard, "", 0), sessionManager, llm.NewMockClient(), Options{
		PermissionPolicy: permissions.Policy{Mode: permissions.ModeAsk},
	})
	httpServer := httptest.NewServer(http.HandlerFunc(server.HandleWebSocket))
	t.Cleanup(httpServer.Close)

	wsURL := "ws" + strings.TrimPrefix(httpServer.URL, "http")
	conn, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
	if err != nil {
		t.Fatalf("dial websocket: %v", err)
	}
	t.Cleanup(func() { _ = conn.Close() })

	if err := conn.WriteJSON(protocolws.Message{
		Type:   protocolws.TypeRequest,
		ID:     "1",
		Method: protocolws.MethodConnect,
		Payload: map[string]any{
			"role":            "client",
			"client_identity": "web-ui",
			"agent_id":        "main",
		},
	}); err != nil {
		t.Fatalf("write connect: %v", err)
	}
	_ = conn.ReadJSON(&protocolws.Message{})
	_ = conn.ReadJSON(&protocolws.Message{})

	if err := conn.WriteJSON(protocolws.Message{
		Type:   protocolws.TypeRequest,
		ID:     "2",
		Method: protocolws.MethodSendMessage,
		Payload: map[string]any{
			"content": "tool run pwd",
		},
	}); err != nil {
		t.Fatalf("write approval-producing message: %v", err)
	}
	_ = waitForPermissionRequired(t, conn)

	if err := conn.WriteJSON(protocolws.Message{
		Type:   protocolws.TypeRequest,
		ID:     "3",
		Method: protocolws.MethodSendMessage,
		Payload: map[string]any{
			"content": "hello",
		},
	}); err != nil {
		t.Fatalf("write plain message: %v", err)
	}
	for i := 0; i < 32; i++ {
		var event protocolws.Message
		if err := conn.ReadJSON(&event); err != nil {
			t.Fatalf("read plain runtime event %d: %v", i, err)
		}
		if event.Type == protocolws.TypeEvent && event.Event == "agent.lifecycle.end" {
			break
		}
	}

	if err := conn.WriteJSON(protocolws.Message{
		Type:   protocolws.TypeRequest,
		ID:     "4",
		Method: protocolws.MethodOrchestrationPlan,
	}); err != nil {
		t.Fatalf("write orchestration_plan: %v", err)
	}
	var planRes protocolws.Message
	if err := conn.ReadJSON(&planRes); err != nil {
		t.Fatalf("read orchestration_plan: %v", err)
	}
	steps, ok := planRes.Payload["steps"].([]any)
	if !ok || len(steps) < 2 {
		t.Fatalf("plan steps payload = %#v, want at least two items", planRes.Payload["steps"])
	}
	first, _ := steps[0].(map[string]any)
	second, _ := steps[1].(map[string]any)
	firstActionID, _ := first["action_id"].(string)
	secondActionID, _ := second["action_id"].(string)
	if firstActionID == "" || secondActionID == "" {
		t.Fatal("expected action ids")
	}

	if err := conn.WriteJSON(protocolws.Message{
		Type:   protocolws.TypeRequest,
		ID:     "5",
		Method: protocolws.MethodOrchestrationPlanStepUpdate,
		Payload: map[string]any{
			"action_id": firstActionID,
			"state":     "completed",
			"result":    "approved",
		},
	}); err != nil {
		t.Fatalf("write first plan step update: %v", err)
	}
	var firstUpdateRes protocolws.Message
	if err := conn.ReadJSON(&firstUpdateRes); err != nil {
		t.Fatalf("read first plan step update: %v", err)
	}
	if !firstUpdateRes.OK {
		t.Fatalf("first plan step update response = %#v, want ok", firstUpdateRes)
	}
	var firstUpdateEvent protocolws.Message
	if err := conn.ReadJSON(&firstUpdateEvent); err != nil {
		t.Fatalf("read first plan step update event: %v", err)
	}
	stepPayload, ok := firstUpdateEvent.Payload["step"].(map[string]any)
	if !ok {
		t.Fatalf("first update event step payload = %#v, want object", firstUpdateEvent.Payload["step"])
	}
	since, _ := stepPayload["updated_at"].(string)
	if since == "" {
		t.Fatal("expected updated_at from first step update event")
	}

	time.Sleep(10 * time.Millisecond)
	if err := conn.WriteJSON(protocolws.Message{
		Type:   protocolws.TypeRequest,
		ID:     "6",
		Method: protocolws.MethodOrchestrationPlanStepUpdate,
		Payload: map[string]any{
			"action_id": secondActionID,
			"state":     "in_progress",
			"result":    "reviewing",
		},
	}); err != nil {
		t.Fatalf("write second plan step update: %v", err)
	}
	var secondUpdateRes protocolws.Message
	if err := conn.ReadJSON(&secondUpdateRes); err != nil {
		t.Fatalf("read second plan step update: %v", err)
	}
	if !secondUpdateRes.OK {
		t.Fatalf("second plan step update response = %#v, want ok", secondUpdateRes)
	}
	_ = conn.ReadJSON(&protocolws.Message{})

	if err := conn.WriteJSON(protocolws.Message{
		Type:   protocolws.TypeRequest,
		ID:     "7",
		Method: protocolws.MethodOrchestrationPlanExecutionHistory,
		Payload: map[string]any{
			"since": since,
		},
	}); err != nil {
		t.Fatalf("write since-filtered orchestration_plan_execution_history: %v", err)
	}
	var historyRes protocolws.Message
	if err := conn.ReadJSON(&historyRes); err != nil {
		t.Fatalf("read since-filtered orchestration_plan_execution_history: %v", err)
	}
	if !historyRes.OK {
		t.Fatalf("since-filtered plan execution history response = %#v, want ok", historyRes)
	}
	history, ok := historyRes.Payload["history"].([]any)
	if !ok || len(history) != 1 {
		t.Fatalf("since-filtered session execution history payload = %#v, want one item", historyRes.Payload["history"])
	}
	record, _ := history[0].(map[string]any)
	if record["action_id"] != secondActionID {
		t.Fatalf("since-filtered history action_id = %#v, want %q", record["action_id"], secondActionID)
	}
}

func TestHandleWebSocketOrchestrationPlanExecutionHistoryRejectsInvalidSince(t *testing.T) {
	sessionManager := session.NewManager(nil)
	server := NewServer(log.New(io.Discard, "", 0), sessionManager, llm.NewMockClient())
	httpServer := httptest.NewServer(http.HandlerFunc(server.HandleWebSocket))
	t.Cleanup(httpServer.Close)

	wsURL := "ws" + strings.TrimPrefix(httpServer.URL, "http")
	conn, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
	if err != nil {
		t.Fatalf("dial websocket: %v", err)
	}
	t.Cleanup(func() { _ = conn.Close() })

	if err := conn.WriteJSON(protocolws.Message{
		Type:   protocolws.TypeRequest,
		ID:     "1",
		Method: protocolws.MethodConnect,
		Payload: map[string]any{
			"role":            "client",
			"client_identity": "web-ui",
			"agent_id":        "main",
		},
	}); err != nil {
		t.Fatalf("write connect: %v", err)
	}
	_ = conn.ReadJSON(&protocolws.Message{})
	_ = conn.ReadJSON(&protocolws.Message{})

	if err := conn.WriteJSON(protocolws.Message{
		Type:   protocolws.TypeRequest,
		ID:     "2",
		Method: protocolws.MethodOrchestrationPlanExecutionHistory,
		Payload: map[string]any{
			"since": "not-a-timestamp",
		},
	}); err != nil {
		t.Fatalf("write invalid since request: %v", err)
	}

	var res protocolws.Message
	if err := conn.ReadJSON(&res); err != nil {
		t.Fatalf("read invalid since response: %v", err)
	}
	if res.OK {
		t.Fatalf("invalid since response = %#v, want error", res)
	}
	if res.Error == nil || res.Error.Message != "invalid since timestamp" {
		t.Fatalf("invalid since error = %#v, want invalid since timestamp", res.Error)
	}
}

func TestHandleWebSocketOrchestrationPlanExecutionHistoryCanFilterByUntil(t *testing.T) {
	sessionManager := session.NewManager(nil)
	server := NewServerWithOptions(log.New(io.Discard, "", 0), sessionManager, llm.NewMockClient(), Options{
		PermissionPolicy: permissions.Policy{Mode: permissions.ModeAsk},
	})
	httpServer := httptest.NewServer(http.HandlerFunc(server.HandleWebSocket))
	t.Cleanup(httpServer.Close)

	wsURL := "ws" + strings.TrimPrefix(httpServer.URL, "http")
	conn, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
	if err != nil {
		t.Fatalf("dial websocket: %v", err)
	}
	t.Cleanup(func() { _ = conn.Close() })

	if err := conn.WriteJSON(protocolws.Message{
		Type:   protocolws.TypeRequest,
		ID:     "1",
		Method: protocolws.MethodConnect,
		Payload: map[string]any{
			"role":            "client",
			"client_identity": "web-ui",
			"agent_id":        "main",
		},
	}); err != nil {
		t.Fatalf("write connect: %v", err)
	}
	_ = conn.ReadJSON(&protocolws.Message{})
	_ = conn.ReadJSON(&protocolws.Message{})

	if err := conn.WriteJSON(protocolws.Message{
		Type:   protocolws.TypeRequest,
		ID:     "2",
		Method: protocolws.MethodSendMessage,
		Payload: map[string]any{
			"content": "tool run pwd",
		},
	}); err != nil {
		t.Fatalf("write approval-producing message: %v", err)
	}
	_ = waitForPermissionRequired(t, conn)

	if err := conn.WriteJSON(protocolws.Message{
		Type:   protocolws.TypeRequest,
		ID:     "3",
		Method: protocolws.MethodSendMessage,
		Payload: map[string]any{
			"content": "hello",
		},
	}); err != nil {
		t.Fatalf("write plain message: %v", err)
	}
	for i := 0; i < 32; i++ {
		var event protocolws.Message
		if err := conn.ReadJSON(&event); err != nil {
			t.Fatalf("read plain runtime event %d: %v", i, err)
		}
		if event.Type == protocolws.TypeEvent && event.Event == "agent.lifecycle.end" {
			break
		}
	}

	if err := conn.WriteJSON(protocolws.Message{
		Type:   protocolws.TypeRequest,
		ID:     "4",
		Method: protocolws.MethodOrchestrationPlan,
	}); err != nil {
		t.Fatalf("write orchestration_plan: %v", err)
	}
	var planRes protocolws.Message
	if err := conn.ReadJSON(&planRes); err != nil {
		t.Fatalf("read orchestration_plan: %v", err)
	}
	steps, ok := planRes.Payload["steps"].([]any)
	if !ok || len(steps) < 2 {
		t.Fatalf("plan steps payload = %#v, want at least two items", planRes.Payload["steps"])
	}
	first, _ := steps[0].(map[string]any)
	second, _ := steps[1].(map[string]any)
	firstActionID, _ := first["action_id"].(string)
	secondActionID, _ := second["action_id"].(string)
	if firstActionID == "" || secondActionID == "" {
		t.Fatal("expected action ids")
	}

	if err := conn.WriteJSON(protocolws.Message{
		Type:   protocolws.TypeRequest,
		ID:     "5",
		Method: protocolws.MethodOrchestrationPlanStepUpdate,
		Payload: map[string]any{
			"action_id": firstActionID,
			"state":     "completed",
			"result":    "approved",
		},
	}); err != nil {
		t.Fatalf("write first plan step update: %v", err)
	}
	var firstUpdateRes protocolws.Message
	if err := conn.ReadJSON(&firstUpdateRes); err != nil {
		t.Fatalf("read first plan step update: %v", err)
	}
	if !firstUpdateRes.OK {
		t.Fatalf("first plan step update response = %#v, want ok", firstUpdateRes)
	}
	var firstUpdateEvent protocolws.Message
	if err := conn.ReadJSON(&firstUpdateEvent); err != nil {
		t.Fatalf("read first plan step update event: %v", err)
	}
	stepPayload, ok := firstUpdateEvent.Payload["step"].(map[string]any)
	if !ok {
		t.Fatalf("first update event step payload = %#v, want object", firstUpdateEvent.Payload["step"])
	}
	until, _ := stepPayload["updated_at"].(string)
	if until == "" {
		t.Fatal("expected updated_at from first step update event")
	}

	time.Sleep(10 * time.Millisecond)
	if err := conn.WriteJSON(protocolws.Message{
		Type:   protocolws.TypeRequest,
		ID:     "6",
		Method: protocolws.MethodOrchestrationPlanStepUpdate,
		Payload: map[string]any{
			"action_id": secondActionID,
			"state":     "in_progress",
			"result":    "reviewing",
		},
	}); err != nil {
		t.Fatalf("write second plan step update: %v", err)
	}
	var secondUpdateRes protocolws.Message
	if err := conn.ReadJSON(&secondUpdateRes); err != nil {
		t.Fatalf("read second plan step update: %v", err)
	}
	if !secondUpdateRes.OK {
		t.Fatalf("second plan step update response = %#v, want ok", secondUpdateRes)
	}
	_ = conn.ReadJSON(&protocolws.Message{})

	if err := conn.WriteJSON(protocolws.Message{
		Type:   protocolws.TypeRequest,
		ID:     "7",
		Method: protocolws.MethodOrchestrationPlanExecutionHistory,
		Payload: map[string]any{
			"until": until,
		},
	}); err != nil {
		t.Fatalf("write until-filtered orchestration_plan_execution_history: %v", err)
	}
	var historyRes protocolws.Message
	if err := conn.ReadJSON(&historyRes); err != nil {
		t.Fatalf("read until-filtered orchestration_plan_execution_history: %v", err)
	}
	if !historyRes.OK {
		t.Fatalf("until-filtered plan execution history response = %#v, want ok", historyRes)
	}
	history, ok := historyRes.Payload["history"].([]any)
	if !ok || len(history) != 1 {
		t.Fatalf("until-filtered session execution history payload = %#v, want one item", historyRes.Payload["history"])
	}
	record, _ := history[0].(map[string]any)
	if record["action_id"] != firstActionID {
		t.Fatalf("until-filtered history action_id = %#v, want %q", record["action_id"], firstActionID)
	}
}

func TestHandleWebSocketOrchestrationPlanExecutionHistoryRejectsInvalidUntil(t *testing.T) {
	sessionManager := session.NewManager(nil)
	server := NewServer(log.New(io.Discard, "", 0), sessionManager, llm.NewMockClient())
	httpServer := httptest.NewServer(http.HandlerFunc(server.HandleWebSocket))
	t.Cleanup(httpServer.Close)

	wsURL := "ws" + strings.TrimPrefix(httpServer.URL, "http")
	conn, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
	if err != nil {
		t.Fatalf("dial websocket: %v", err)
	}
	t.Cleanup(func() { _ = conn.Close() })

	if err := conn.WriteJSON(protocolws.Message{
		Type:   protocolws.TypeRequest,
		ID:     "1",
		Method: protocolws.MethodConnect,
		Payload: map[string]any{
			"role":            "client",
			"client_identity": "web-ui",
			"agent_id":        "main",
		},
	}); err != nil {
		t.Fatalf("write connect: %v", err)
	}
	_ = conn.ReadJSON(&protocolws.Message{})
	_ = conn.ReadJSON(&protocolws.Message{})

	if err := conn.WriteJSON(protocolws.Message{
		Type:   protocolws.TypeRequest,
		ID:     "2",
		Method: protocolws.MethodOrchestrationPlanExecutionHistory,
		Payload: map[string]any{
			"until": "not-a-timestamp",
		},
	}); err != nil {
		t.Fatalf("write invalid until request: %v", err)
	}

	var res protocolws.Message
	if err := conn.ReadJSON(&res); err != nil {
		t.Fatalf("read invalid until response: %v", err)
	}
	if res.OK {
		t.Fatalf("invalid until response = %#v, want error", res)
	}
	if res.Error == nil || res.Error.Message != "invalid until timestamp" {
		t.Fatalf("invalid until error = %#v, want invalid until timestamp", res.Error)
	}
}

func TestHandleWebSocketOrchestrationPlanExecutionHistoryCanFilterBySinceAndUntil(t *testing.T) {
	sessionManager := session.NewManager(nil)
	server := NewServerWithOptions(log.New(io.Discard, "", 0), sessionManager, llm.NewMockClient(), Options{
		PermissionPolicy: permissions.Policy{Mode: permissions.ModeAsk},
	})
	httpServer := httptest.NewServer(http.HandlerFunc(server.HandleWebSocket))
	t.Cleanup(httpServer.Close)

	wsURL := "ws" + strings.TrimPrefix(httpServer.URL, "http")
	conn, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
	if err != nil {
		t.Fatalf("dial websocket: %v", err)
	}
	t.Cleanup(func() { _ = conn.Close() })

	if err := conn.WriteJSON(protocolws.Message{
		Type:   protocolws.TypeRequest,
		ID:     "1",
		Method: protocolws.MethodConnect,
		Payload: map[string]any{
			"role":            "client",
			"client_identity": "web-ui",
			"agent_id":        "main",
		},
	}); err != nil {
		t.Fatalf("write connect: %v", err)
	}
	_ = conn.ReadJSON(&protocolws.Message{})
	_ = conn.ReadJSON(&protocolws.Message{})

	for idx, content := range []string{"tool run pwd", "hello", "tool upper hi"} {
		if err := conn.WriteJSON(protocolws.Message{
			Type:   protocolws.TypeRequest,
			ID:     string(rune('2' + idx)),
			Method: protocolws.MethodSendMessage,
			Payload: map[string]any{
				"content": content,
			},
		}); err != nil {
			t.Fatalf("write seed message %d: %v", idx, err)
		}
		if idx == 0 {
			_ = waitForPermissionRequired(t, conn)
			continue
		}
		limit := 32
		for i := 0; i < limit; i++ {
			var event protocolws.Message
			if err := conn.ReadJSON(&event); err != nil {
				t.Fatalf("read seed runtime event %d/%d: %v", idx, i, err)
			}
			if event.Type == protocolws.TypeEvent && event.Event == "agent.lifecycle.end" {
				break
			}
		}
	}

	if err := conn.WriteJSON(protocolws.Message{
		Type:   protocolws.TypeRequest,
		ID:     "5",
		Method: protocolws.MethodOrchestrationPlan,
	}); err != nil {
		t.Fatalf("write orchestration_plan: %v", err)
	}
	var planRes protocolws.Message
	if err := conn.ReadJSON(&planRes); err != nil {
		t.Fatalf("read orchestration_plan: %v", err)
	}
	steps, ok := planRes.Payload["steps"].([]any)
	if !ok || len(steps) < 3 {
		t.Fatalf("plan steps payload = %#v, want at least three items", planRes.Payload["steps"])
	}
	first, _ := steps[0].(map[string]any)
	second, _ := steps[1].(map[string]any)
	third, _ := steps[2].(map[string]any)
	firstActionID, _ := first["action_id"].(string)
	secondActionID, _ := second["action_id"].(string)
	thirdActionID, _ := third["action_id"].(string)
	if firstActionID == "" || secondActionID == "" || thirdActionID == "" {
		t.Fatal("expected action ids")
	}

	if err := conn.WriteJSON(protocolws.Message{
		Type:   protocolws.TypeRequest,
		ID:     "6",
		Method: protocolws.MethodOrchestrationPlanStepUpdate,
		Payload: map[string]any{
			"action_id": firstActionID,
			"state":     "completed",
			"result":    "approved",
		},
	}); err != nil {
		t.Fatalf("write first plan step update: %v", err)
	}
	var firstUpdateRes protocolws.Message
	if err := conn.ReadJSON(&firstUpdateRes); err != nil {
		t.Fatalf("read first plan step update: %v", err)
	}
	if !firstUpdateRes.OK {
		t.Fatalf("first plan step update response = %#v, want ok", firstUpdateRes)
	}
	var firstUpdateEvent protocolws.Message
	if err := conn.ReadJSON(&firstUpdateEvent); err != nil {
		t.Fatalf("read first plan step update event: %v", err)
	}
	firstStepPayload, ok := firstUpdateEvent.Payload["step"].(map[string]any)
	if !ok {
		t.Fatalf("first update event step payload = %#v, want object", firstUpdateEvent.Payload["step"])
	}
	since, _ := firstStepPayload["updated_at"].(string)
	if since == "" {
		t.Fatal("expected since timestamp")
	}
	time.Sleep(10 * time.Millisecond)

	if err := conn.WriteJSON(protocolws.Message{
		Type:   protocolws.TypeRequest,
		ID:     "7",
		Method: protocolws.MethodOrchestrationPlanStepUpdate,
		Payload: map[string]any{
			"action_id": secondActionID,
			"state":     "completed",
			"result":    "reviewed",
		},
	}); err != nil {
		t.Fatalf("write second plan step update: %v", err)
	}
	var secondUpdateRes protocolws.Message
	if err := conn.ReadJSON(&secondUpdateRes); err != nil {
		t.Fatalf("read second plan step update: %v", err)
	}
	if !secondUpdateRes.OK {
		t.Fatalf("second plan step update response = %#v, want ok", secondUpdateRes)
	}
	var secondUpdateEvent protocolws.Message
	if err := conn.ReadJSON(&secondUpdateEvent); err != nil {
		t.Fatalf("read second plan step update event: %v", err)
	}
	secondStepPayload, ok := secondUpdateEvent.Payload["step"].(map[string]any)
	if !ok {
		t.Fatalf("second update event step payload = %#v, want object", secondUpdateEvent.Payload["step"])
	}
	until, _ := secondStepPayload["updated_at"].(string)
	if until == "" {
		t.Fatal("expected until timestamp")
	}
	time.Sleep(10 * time.Millisecond)

	if err := conn.WriteJSON(protocolws.Message{
		Type:   protocolws.TypeRequest,
		ID:     "8",
		Method: protocolws.MethodOrchestrationPlanStepUpdate,
		Payload: map[string]any{
			"action_id": thirdActionID,
			"state":     "in_progress",
			"result":    "finalizing",
		},
	}); err != nil {
		t.Fatalf("write third plan step update: %v", err)
	}
	var thirdUpdateRes protocolws.Message
	if err := conn.ReadJSON(&thirdUpdateRes); err != nil {
		t.Fatalf("read third plan step update: %v", err)
	}
	if !thirdUpdateRes.OK {
		t.Fatalf("third plan step update response = %#v, want ok", thirdUpdateRes)
	}
	_ = conn.ReadJSON(&protocolws.Message{})

	if err := conn.WriteJSON(protocolws.Message{
		Type:   protocolws.TypeRequest,
		ID:     "9",
		Method: protocolws.MethodOrchestrationPlanExecutionHistory,
		Payload: map[string]any{
			"since": since,
			"until": until,
		},
	}); err != nil {
		t.Fatalf("write bounded orchestration_plan_execution_history: %v", err)
	}
	var historyRes protocolws.Message
	if err := conn.ReadJSON(&historyRes); err != nil {
		t.Fatalf("read bounded orchestration_plan_execution_history: %v", err)
	}
	if !historyRes.OK {
		t.Fatalf("bounded plan execution history response = %#v, want ok", historyRes)
	}
	history, ok := historyRes.Payload["history"].([]any)
	if !ok || len(history) != 1 {
		t.Fatalf("bounded session execution history payload = %#v, want one item", historyRes.Payload["history"])
	}
	record, _ := history[0].(map[string]any)
	if record["action_id"] != secondActionID {
		t.Fatalf("bounded history action_id = %#v, want %q", record["action_id"], secondActionID)
	}
}

func TestHandleWebSocketSubagentResumeReusesChildSession(t *testing.T) {
	sessionManager := session.NewManager(nil)
	server := NewServer(log.New(io.Discard, "", 0), sessionManager, llm.NewMockClient())
	httpServer := httptest.NewServer(http.HandlerFunc(server.HandleWebSocket))
	t.Cleanup(httpServer.Close)

	wsURL := "ws" + strings.TrimPrefix(httpServer.URL, "http")
	conn, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
	if err != nil {
		t.Fatalf("dial websocket: %v", err)
	}
	t.Cleanup(func() { _ = conn.Close() })

	if err := conn.WriteJSON(protocolws.Message{
		Type:   protocolws.TypeRequest,
		ID:     "1",
		Method: protocolws.MethodConnect,
		Payload: map[string]any{
			"role":            "client",
			"client_identity": "web-ui",
			"agent_id":        "main",
		},
	}); err != nil {
		t.Fatalf("write connect: %v", err)
	}
	_ = conn.ReadJSON(&protocolws.Message{})
	_ = conn.ReadJSON(&protocolws.Message{})

	if err := conn.WriteJSON(protocolws.Message{
		Type:   protocolws.TypeRequest,
		ID:     "2",
		Method: protocolws.MethodSpawnSubagent,
		Payload: map[string]any{
			"label":  "research",
			"prompt": "tool upper first run",
		},
	}); err != nil {
		t.Fatalf("write spawn request: %v", err)
	}
	var spawn protocolws.Message
	if err := conn.ReadJSON(&spawn); err != nil {
		t.Fatalf("read spawn response: %v", err)
	}
	firstRunID, _ := spawn.Payload["run_id"].(string)
	firstChildSessionKey, _ := spawn.Payload["child_session_key"].(string)
	completed := false
	for i := 0; i < 6; i++ {
		var event protocolws.Message
		if err := conn.ReadJSON(&event); err != nil {
			t.Fatalf("read spawn follow-up %d: %v", i, err)
		}
		runID, _ := event.Payload["run_id"].(string)
		status, _ := event.Payload["status"].(string)
		if runID == firstRunID && status == "completed" {
			completed = true
			break
		}
	}
	if !completed {
		t.Fatalf("did not observe completion event for first run %q", firstRunID)
	}

	if err := conn.WriteJSON(protocolws.Message{
		Type:   protocolws.TypeRequest,
		ID:     "3",
		Method: protocolws.MethodSubagentResume,
		Payload: map[string]any{
			"run_id": firstRunID,
			"prompt": "tool upper second run",
			"label":  "research-resume",
		},
	}); err != nil {
		t.Fatalf("write resume request: %v", err)
	}
	var resume protocolws.Message
	// skip any async completed event that may arrive first
	for i := 0; i < 4; i++ {
		if err := conn.ReadJSON(&resume); err != nil {
			t.Fatalf("read resume response %d: %v", i, err)
		}
		if resume.Type == protocolws.TypeResponse && resume.ID == "3" {
			break
		}
	}
	if !resume.OK {
		t.Fatalf("resume response = %#v, want ok", resume)
	}
	secondChildSessionKey, _ := resume.Payload["child_session_key"].(string)
	if secondChildSessionKey != firstChildSessionKey {
		t.Fatalf("resume child session key = %q, want reuse %q", secondChildSessionKey, firstChildSessionKey)
	}
}

func TestHandleWebSocketSubagentResumeEmitsCompletedEventForResumedAttempt(t *testing.T) {
	sessionManager := session.NewManager(nil)
	server := NewServer(log.New(io.Discard, "", 0), sessionManager, llm.NewMockClient())
	httpServer := httptest.NewServer(http.HandlerFunc(server.HandleWebSocket))
	t.Cleanup(httpServer.Close)

	wsURL := "ws" + strings.TrimPrefix(httpServer.URL, "http")
	conn, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
	if err != nil {
		t.Fatalf("dial websocket: %v", err)
	}
	t.Cleanup(func() { _ = conn.Close() })

	if err := conn.WriteJSON(protocolws.Message{
		Type:   protocolws.TypeRequest,
		ID:     "1",
		Method: protocolws.MethodConnect,
		Payload: map[string]any{
			"role":            "client",
			"client_identity": "web-ui",
			"agent_id":        "main",
		},
	}); err != nil {
		t.Fatalf("write connect: %v", err)
	}
	_ = conn.ReadJSON(&protocolws.Message{})
	_ = conn.ReadJSON(&protocolws.Message{})

	if err := conn.WriteJSON(protocolws.Message{
		Type:   protocolws.TypeRequest,
		ID:     "2",
		Method: protocolws.MethodSpawnSubagent,
		Payload: map[string]any{
			"label":  "research",
			"prompt": "tool upper first run",
		},
	}); err != nil {
		t.Fatalf("write spawn request: %v", err)
	}
	var spawn protocolws.Message
	if err := conn.ReadJSON(&spawn); err != nil {
		t.Fatalf("read spawn response: %v", err)
	}
	runID, _ := spawn.Payload["run_id"].(string)
	firstCompleted := false
	for i := 0; i < 6; i++ {

		var msg protocolws.Message
		if err := conn.ReadJSON(&msg); err != nil {
			t.Fatalf("read first attempt follow-up %d: %v", i, err)
		}
		if msg.Type == protocolws.TypeEvent && msg.Event == protocolws.EventSubagentCompleted {
			if gotRunID, _ := msg.Payload["run_id"].(string); gotRunID == runID {
				firstCompleted = true
				break
			}
		}
	}
	if !firstCompleted {
		t.Fatalf("did not observe completion event for first run %q", runID)
	}

	if err := conn.WriteJSON(protocolws.Message{
		Type:   protocolws.TypeRequest,
		ID:     "3",
		Method: protocolws.MethodSubagentResume,
		Payload: map[string]any{
			"run_id": runID,
			"prompt": "tool upper second run",
			"label":  "research-resume",
		},
	}); err != nil {
		t.Fatalf("write resume request: %v", err)
	}

	var resume protocolws.Message
	for i := 0; i < 4; i++ {
		if err := conn.ReadJSON(&resume); err != nil {
			t.Fatalf("read resume response %d: %v", i, err)
		}
		if resume.Type == protocolws.TypeResponse && resume.ID == "3" {
			break
		}
	}
	if !resume.OK {
		t.Fatalf("resume response = %#v, want ok", resume)
	}

	for i := 0; i < 6; i++ {

		var msg protocolws.Message
		if err := conn.ReadJSON(&msg); err != nil {
			t.Fatalf("read resumed attempt follow-up %d: %v", i, err)
		}
		if msg.Type == protocolws.TypeEvent && msg.Event == protocolws.EventSubagentCompleted {
			if gotRunID, _ := msg.Payload["run_id"].(string); gotRunID == runID {
				if got := msg.Payload["status"]; got != "completed" {
					t.Fatalf("resumed completion status = %#v, want completed", got)
				}
				if got := msg.Payload["last_action"]; got != "resumed" {
					t.Fatalf("resumed completion last_action = %#v, want resumed", got)
				}
				return
			}
		}
	}
	t.Fatalf("did not observe completion event for resumed attempt %q", runID)
}

func TestHandleWebSocketSystemRunToolLoop(t *testing.T) {
	sessionManager := session.NewManager(nil)
	server := NewServer(log.New(io.Discard, "", 0), sessionManager, llm.NewMockClient())
	httpServer := httptest.NewServer(http.HandlerFunc(server.HandleWebSocket))
	t.Cleanup(httpServer.Close)

	wsURL := "ws" + strings.TrimPrefix(httpServer.URL, "http")
	conn, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
	if err != nil {
		t.Fatalf("dial websocket: %v", err)
	}
	t.Cleanup(func() { _ = conn.Close() })

	if err := conn.WriteJSON(protocolws.Message{
		Type:   protocolws.TypeRequest,
		ID:     "1",
		Method: protocolws.MethodConnect,
		Payload: map[string]any{
			"role":            "client",
			"client_identity": "web-ui",
			"agent_id":        "main",
		},
	}); err != nil {
		t.Fatalf("write connect: %v", err)
	}

	var discard protocolws.Message
	_ = conn.ReadJSON(&discard)
	_ = conn.ReadJSON(&discard)

	if err := conn.WriteJSON(protocolws.Message{
		Type:   protocolws.TypeRequest,
		ID:     "2",
		Method: protocolws.MethodSendMessage,
		Payload: map[string]any{
			"content": "tool run " + helloCommand(),
		},
	}); err != nil {
		t.Fatalf("write inbound: %v", err)
	}

	events := make([]protocolws.Message, 0, 20)
	for len(events) < 30 {

		var msg protocolws.Message
		if err := conn.ReadJSON(&msg); err != nil {
			t.Fatalf("read event %d: %v", len(events), err)
		}
		events = append(events, msg)
		if msg.Type == protocolws.TypeEvent && msg.Event == "agent.lifecycle.end" {
			break
		}
	}

	foundSystemRun := false
	foundToolResult := false
	foundAssistantFinal := false
	for _, event := range events {
		if event.Type == protocolws.TypeEvent && event.Event == "tool.called" {
			if event.Payload["tool_name"] == "system.run" {
				foundSystemRun = true
			}
		}
		if event.Type == protocolws.TypeEvent && event.Event == "tool.result" {
			if message, ok := event.Payload["message"].(map[string]any); ok {
				if strings.Contains(message["content"].(string), "system.run: hello") {
					foundToolResult = true
				}
			}
		}
		if event.Type == protocolws.TypeEvent && event.Event == protocolws.EventMessageCreated {
			if message, ok := event.Payload["message"].(map[string]any); ok {
				if message["role"] == "assistant" && message["content"] == "Using tool result: system.run: hello" {
					foundAssistantFinal = true
				}
			}
		}
	}

	if !foundSystemRun {
		t.Fatal("expected tool.called for system.run")
	}
	if !foundToolResult {
		t.Fatal("expected tool.result containing command output")
	}
	if !foundAssistantFinal {
		t.Fatal("expected final assistant message from system.run result")
	}
}

func TestHandleWebSocketSystemRunUsesSandboxForNonMainSession(t *testing.T) {
	sessionManager := session.NewManager(nil)
	child := sessionManager.CreateChild("main", "agent:main:child:test")
	server := NewServer(log.New(io.Discard, "", 0), sessionManager, llm.NewMockClient())
	httpServer := httptest.NewServer(http.HandlerFunc(server.HandleWebSocket))
	t.Cleanup(httpServer.Close)

	wsURL := "ws" + strings.TrimPrefix(httpServer.URL, "http")
	conn, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
	if err != nil {
		t.Fatalf("dial websocket: %v", err)
	}
	t.Cleanup(func() { _ = conn.Close() })

	if err := conn.WriteJSON(protocolws.Message{
		Type:   protocolws.TypeRequest,
		ID:     "1",
		Method: protocolws.MethodConnect,
		Payload: map[string]any{
			"role":            "client",
			"client_identity": "web-ui",
			"agent_id":        "main",
			"session_key":     child.Key,
		},
	}); err != nil {
		t.Fatalf("write connect: %v", err)
	}

	var discard protocolws.Message
	_ = conn.ReadJSON(&discard)
	_ = conn.ReadJSON(&discard)

	if err := conn.WriteJSON(protocolws.Message{
		Type:   protocolws.TypeRequest,
		ID:     "2",
		Method: protocolws.MethodSendMessage,
		Payload: map[string]any{
			"content": "tool run " + helloCommand(),
		},
	}); err != nil {
		t.Fatalf("write inbound: %v", err)
	}

	foundSandboxToolResult := false
	for i := 0; i < 30; i++ {

		var msg protocolws.Message
		if err := conn.ReadJSON(&msg); err != nil {
			t.Fatalf("read event %d: %v", i, err)
		}
		if msg.Type == protocolws.TypeEvent && msg.Event == "tool.result" {
			meta, _ := msg.Payload["meta"].(map[string]any)
			structured, _ := msg.Payload["structured_content"].(map[string]any)
			message, _ := msg.Payload["message"].(map[string]any)
			if msg.Payload["tool_name"] == "system.run" &&
				meta["execution_mode"] == "sandbox" &&
				structured["execution_mode"] == "sandbox" &&
				structured["command"] == helloCommand() &&
				meta["working_directory"] != "" &&
				strings.Contains(message["content"].(string), "[sandbox]") {
				foundSandboxToolResult = true
			}
		}
		if msg.Type == protocolws.TypeEvent && msg.Event == "agent.lifecycle.end" {
			break
		}
	}

	if !foundSandboxToolResult {
		t.Fatal("expected sandboxed tool result for non-main session")
	}
}

func TestHandleWebSocketReusesMainSessionByAgent(t *testing.T) {
	sessionManager := session.NewManager(nil)
	server := NewServer(log.New(io.Discard, "", 0), sessionManager, llm.NewMockClient())
	httpServer := httptest.NewServer(http.HandlerFunc(server.HandleWebSocket))
	t.Cleanup(httpServer.Close)

	wsURL := "ws" + strings.TrimPrefix(httpServer.URL, "http")

	firstConn, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
	if err != nil {
		t.Fatalf("dial first websocket: %v", err)
	}
	t.Cleanup(func() {
		_ = firstConn.Close()
	})

	if err := firstConn.WriteJSON(protocolws.Message{
		Type:   protocolws.TypeRequest,
		ID:     "1",
		Method: protocolws.MethodConnect,
		Payload: map[string]any{
			"role":            "client",
			"client_identity": "web-ui-1",
			"agent_id":        "main",
		},
	}); err != nil {
		t.Fatalf("write first connect: %v", err)
	}

	var firstRes protocolws.Message
	if err := firstConn.ReadJSON(&firstRes); err != nil {
		t.Fatalf("read first connect response: %v", err)
	}
	var firstHello protocolws.Message
	if err := firstConn.ReadJSON(&firstHello); err != nil {
		t.Fatalf("read first hello: %v", err)
	}
	firstSessionKey, _ := firstRes.Payload["session_key"].(string)

	secondConn, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
	if err != nil {
		t.Fatalf("dial second websocket: %v", err)
	}
	t.Cleanup(func() {
		_ = secondConn.Close()
	})

	if err := secondConn.WriteJSON(protocolws.Message{
		Type:   protocolws.TypeRequest,
		ID:     "2",
		Method: protocolws.MethodConnect,
		Payload: map[string]any{
			"role":            "client",
			"client_identity": "web-ui-2",
			"agent_id":        "main",
		},
	}); err != nil {
		t.Fatalf("write second connect: %v", err)
	}

	var secondRes protocolws.Message
	if err := secondConn.ReadJSON(&secondRes); err != nil {
		t.Fatalf("read second connect response: %v", err)
	}
	secondSessionKey, _ := secondRes.Payload["session_key"].(string)
	if secondSessionKey != firstSessionKey {
		t.Fatalf("second session key = %q, want reuse %q", secondSessionKey, firstSessionKey)
	}
}

func TestHandleWebSocketRejectsMismatchedAgentAndSession(t *testing.T) {
	sessionManager := session.NewManager(nil)
	existing := sessionManager.GetOrCreateMain("main")
	server := NewServer(log.New(io.Discard, "", 0), sessionManager, llm.NewMockClient())
	httpServer := httptest.NewServer(http.HandlerFunc(server.HandleWebSocket))
	t.Cleanup(httpServer.Close)

	wsURL := "ws" + strings.TrimPrefix(httpServer.URL, "http")
	conn, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
	if err != nil {
		t.Fatalf("dial websocket: %v", err)
	}
	t.Cleanup(func() {
		_ = conn.Close()
	})

	if err := conn.WriteJSON(protocolws.Message{
		Type:   protocolws.TypeRequest,
		ID:     "1",
		Method: protocolws.MethodConnect,
		Payload: map[string]any{
			"role":            "client",
			"client_identity": "web-ui",
			"agent_id":        "other",
			"session_key":     existing.Key,
		},
	}); err != nil {
		t.Fatalf("write connect: %v", err)
	}

	var res protocolws.Message
	if err := conn.ReadJSON(&res); err != nil {
		t.Fatalf("read response: %v", err)
	}
	if res.Type != protocolws.TypeResponse || res.OK {
		t.Fatalf("response = %#v, want error response", res)
	}
	if res.Error == nil {
		t.Fatalf("response error = nil, want error payload")
	}
}

func TestHandleWebSocketQueuesMessagesPerSession(t *testing.T) {
	sessionManager := session.NewManager(nil)
	server := NewServer(log.New(io.Discard, "", 0), sessionManager, &slowMockClient{delay: 50 * time.Millisecond})
	httpServer := httptest.NewServer(http.HandlerFunc(server.HandleWebSocket))
	t.Cleanup(httpServer.Close)

	wsURL := "ws" + strings.TrimPrefix(httpServer.URL, "http")
	conn, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
	if err != nil {
		t.Fatalf("dial websocket: %v", err)
	}
	t.Cleanup(func() { _ = conn.Close() })

	if err := conn.WriteJSON(protocolws.Message{
		Type:   protocolws.TypeRequest,
		ID:     "1",
		Method: protocolws.MethodConnect,
		Payload: map[string]any{
			"role":            "client",
			"client_identity": "web-ui",
			"agent_id":        "main",
		},
	}); err != nil {
		t.Fatalf("write connect: %v", err)
	}
	var discard protocolws.Message
	_ = conn.ReadJSON(&discard)
	_ = conn.ReadJSON(&discard)

	if err := conn.WriteJSON(protocolws.Message{
		Type:   protocolws.TypeRequest,
		ID:     "2",
		Method: protocolws.MethodSendMessage,
		Payload: map[string]any{
			"content": "first",
		},
	}); err != nil {
		t.Fatalf("write first message: %v", err)
	}
	if err := conn.WriteJSON(protocolws.Message{
		Type:   protocolws.TypeRequest,
		ID:     "3",
		Method: protocolws.MethodSendMessage,
		Payload: map[string]any{
			"content": "second",
		},
	}); err != nil {
		t.Fatalf("write second message: %v", err)
	}

	foundQueued := false
	foundSecondAssistant := false
	for i := 0; i < 20; i++ {
		var event protocolws.Message
		if err := conn.ReadJSON(&event); err != nil {
			t.Fatalf("read queued event %d: %v", i, err)
		}

		if event.Type == protocolws.TypeEvent && event.Event == "queue.enqueued" {
			foundQueued = true
		}
		if event.Type == protocolws.TypeEvent && event.Event == protocolws.EventMessageCreated {
			if message, ok := event.Payload["message"].(map[string]any); ok {
				if message["role"] == "assistant" && message["content"] == "Slow: second [workspace:3]" {
					foundSecondAssistant = true
					break
				}
			}
		}
	}

	if !foundQueued {
		t.Fatal("expected queue.enqueued event")
	}
	if !foundSecondAssistant {
		t.Fatal("expected assistant reply for second queued message")
	}
}

type slowMockClient struct {
	delay time.Duration
}

func (c *slowMockClient) Stream(ctx context.Context, req llm.GenerateRequest, handler llm.StreamHandler) error {
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-time.After(c.delay):
	}

	if err := handler.OnEvent(llm.StreamEvent{Type: "text.delta", Delta: "Slow:"}); err != nil {
		return err
	}
	if err := handler.OnEvent(llm.StreamEvent{Type: "text.delta", Delta: " " + req.Context.UserInput}); err != nil {
		return err
	}
	if len(req.Context.WorkspaceLines) > 0 {
		if err := handler.OnEvent(llm.StreamEvent{Type: "text.delta", Delta: " [workspace:3]"}); err != nil {
			return err
		}
	}
	return handler.OnEvent(llm.StreamEvent{Type: "message.end"})
}

type objectToolCallClient struct{}

func (c *objectToolCallClient) Stream(_ context.Context, _ llm.GenerateRequest, handler llm.StreamHandler) error {
	if err := handler.OnEvent(llm.StreamEvent{
		Type:            "tool.call",
		ToolName:        "system.run",
		ToolInput:       `{"command":"pwd"}`,
		ToolInputObject: map[string]any{"command": "pwd"},
	}); err != nil {
		return err
	}
	return handler.OnEvent(llm.StreamEvent{Type: "message.end"})
}

type progressToolCallClient struct{}

func (c *progressToolCallClient) Stream(_ context.Context, _ llm.GenerateRequest, handler llm.StreamHandler) error {
	if err := handler.OnEvent(llm.StreamEvent{
		Type:            "tool.call",
		ToolName:        "progress.echo",
		ToolInput:       `{"value":"hello"}`,
		ToolInputObject: map[string]any{"value": "hello"},
		ToolUseID:       "toolu-progress-gateway",
	}); err != nil {
		return err
	}
	return handler.OnEvent(llm.StreamEvent{Type: "message.end"})
}

type failingShellToolCallClient struct {
	call int
}

func (c *failingShellToolCallClient) Stream(_ context.Context, _ llm.GenerateRequest, handler llm.StreamHandler) error {
	if c.call == 0 {
		c.call++
		if err := handler.OnEvent(llm.StreamEvent{
			Type:            "tool.call",
			ToolName:        "Bash",
			ToolInput:       `{"command":"cat missing.txt"}`,
			ToolInputObject: map[string]any{"command": "cat missing.txt"},
			ToolUseID:       "toolu-shell-gateway-fail",
		}); err != nil {
			return err
		}
		return handler.OnEvent(llm.StreamEvent{Type: "message.end"})
	}
	if err := handler.OnEvent(llm.StreamEvent{Type: "text.delta", Delta: "done"}); err != nil {
		return err
	}
	return handler.OnEvent(llm.StreamEvent{Type: "message.end"})
}

type captureModelClient struct {
	lastRequest llm.GenerateRequest
	callCount   int
}

func (c *captureModelClient) Stream(_ context.Context, req llm.GenerateRequest, handler llm.StreamHandler) error {
	c.lastRequest = req
	c.callCount++
	if err := handler.OnEvent(llm.StreamEvent{Type: "text.delta", Delta: "Captured"}); err != nil {
		return err
	}
	return handler.OnEvent(llm.StreamEvent{Type: "message.end"})
}

func TestHandleWebSocketSendMessageRoutesSlashCommandThroughRuntimeRegistry(t *testing.T) {
	sessionManager := session.NewManager(nil)
	client := &captureModelClient{}
	server := NewServerWithOptions(log.New(io.Discard, "", 0), sessionManager, client, Options{})
	conn := connectGatewayTestClient(t, server)

	if err := conn.WriteJSON(protocolws.Message{
		Type:   protocolws.TypeRequest,
		ID:     "2",
		Method: protocolws.MethodSendMessage,
		Payload: map[string]any{
			"content": "/status include runtime state",
		},
	}); err != nil {
		t.Fatalf("write send_message: %v", err)
	}

	response := readGatewayResponse(t, conn)
	if response.Type != protocolws.TypeResponse || !response.OK {
		t.Fatalf("send_message response = %#v, want ok", response)
	}
	_ = waitForEvent(t, conn, "agent.lifecycle.end")

	if client.callCount != 1 {
		t.Fatalf("llm call count = %d, want 1", client.callCount)
	}
	if client.lastRequest.Context.UserInput != "include runtime state" {
		t.Fatalf("gateway user input = %q, want normalized slash command args", client.lastRequest.Context.UserInput)
	}
	if client.lastRequest.UserMessage.Content != "include runtime state" {
		t.Fatalf("gateway user message = %q, want normalized slash command args", client.lastRequest.UserMessage.Content)
	}
}

func TestHandleWebSocketUnknownSlashCommandEmitsRuntimeError(t *testing.T) {
	sessionManager := session.NewManager(nil)
	client := &captureModelClient{}
	server := NewServerWithOptions(log.New(io.Discard, "", 0), sessionManager, client, Options{})
	conn := connectGatewayTestClient(t, server)

	if err := conn.WriteJSON(protocolws.Message{
		Type:   protocolws.TypeRequest,
		ID:     "2",
		Method: protocolws.MethodSendMessage,
		Payload: map[string]any{
			"content": "/not-a-command",
		},
	}); err != nil {
		t.Fatalf("write send_message: %v", err)
	}

	response := readGatewayResponse(t, conn)
	if response.Type != protocolws.TypeResponse || !response.OK {
		t.Fatalf("send_message response = %#v, want accepted async request", response)
	}
	event := waitForEvent(t, conn, "run.error")
	if got, _ := event.Payload["message"].(string); !strings.Contains(got, "not registered") {
		t.Fatalf("run.error payload = %#v, want explicit unknown command error", event.Payload)
	}
	if client.callCount != 0 {
		t.Fatalf("llm call count = %d, want 0 for unknown slash command", client.callCount)
	}
}

func TestHandleWebSocketPassesConfiguredMainLoopModelIntoGenerateRequest(t *testing.T) {
	sessionManager := session.NewManager(nil)
	client := &captureModelClient{}
	server := NewServerWithOptions(log.New(io.Discard, "", 0), sessionManager, client, Options{
		MainLoopModel: "claude-opus-4-6",
	})
	httpServer := httptest.NewServer(http.HandlerFunc(server.HandleWebSocket))
	t.Cleanup(httpServer.Close)

	wsURL := "ws" + strings.TrimPrefix(httpServer.URL, "http")
	conn, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
	if err != nil {
		t.Fatalf("dial websocket: %v", err)
	}
	t.Cleanup(func() { _ = conn.Close() })

	if err := conn.WriteJSON(protocolws.Message{
		Type:   protocolws.TypeRequest,
		ID:     "1",
		Method: protocolws.MethodConnect,
		Payload: map[string]any{
			"role":            "client",
			"client_identity": "web-ui",
			"agent_id":        "main",
		},
	}); err != nil {
		t.Fatalf("write connect: %v", err)
	}
	_ = conn.ReadJSON(&protocolws.Message{})
	_ = conn.ReadJSON(&protocolws.Message{})

	if err := conn.WriteJSON(protocolws.Message{
		Type:   protocolws.TypeRequest,
		ID:     "2",
		Method: protocolws.MethodSendMessage,
		Payload: map[string]any{
			"content": "hello",
		},
	}); err != nil {
		t.Fatalf("write inbound: %v", err)
	}

	for i := 0; i < 12; i++ {
		var event protocolws.Message
		if err := conn.ReadJSON(&event); err != nil {
			t.Fatalf("read event %d: %v", i, err)
		}
		if event.Type == protocolws.TypeEvent && event.Event == "agent.lifecycle.end" {
			break
		}
	}

	if client.lastRequest.Model != "claude-opus-4-6" {
		t.Fatalf("request model = %q, want %q", client.lastRequest.Model, "claude-opus-4-6")
	}
}

func helloCommand() string {
	if runtime.GOOS == "windows" {
		return "Write-Output hello"
	}
	return "printf hello"
}

func TestHandleWebSocketSessionStatusIncludesToolContracts(t *testing.T) {
	sessionManager := session.NewManager(nil)
	server := NewServerWithOptions(log.New(io.Discard, "", 0), sessionManager, llm.NewMockClient(), Options{})
	httpServer := httptest.NewServer(http.HandlerFunc(server.HandleWebSocket))
	t.Cleanup(httpServer.Close)

	wsURL := "ws" + strings.TrimPrefix(httpServer.URL, "http")
	conn, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
	if err != nil {
		t.Fatalf("dial websocket: %v", err)
	}
	t.Cleanup(func() { _ = conn.Close() })

	if err := conn.WriteJSON(protocolws.Message{Type: protocolws.TypeRequest, ID: "1", Method: protocolws.MethodConnect, Payload: map[string]any{"role": "client", "client_identity": "test", "agent_id": "main"}}); err != nil {
		t.Fatalf("write connect: %v", err)
	}
	_ = conn.ReadJSON(&protocolws.Message{})
	_ = conn.ReadJSON(&protocolws.Message{})

	if err := conn.WriteJSON(protocolws.Message{Type: protocolws.TypeRequest, ID: "2", Method: protocolws.MethodSessionStatus, Payload: map[string]any{}}); err != nil {
		t.Fatalf("write session_status: %v", err)
	}
	var res protocolws.Message
	if err := conn.SetReadDeadline(time.Now().Add(2 * time.Second)); err != nil {
		t.Fatalf("set read deadline: %v", err)
	}
	if err := conn.ReadJSON(&res); err != nil {
		t.Fatalf("read session_status: %v", err)
	}
	items, ok := res.Payload["tool_contracts"].([]any)
	if !ok || len(items) == 0 {
		t.Fatalf("tool_contracts = %#v, want non-empty contract list", res.Payload["tool_contracts"])
	}
	for _, item := range items {
		contract, ok := item.(map[string]any)
		if !ok {
			continue
		}
		if contract["name"] == "Bash" {
			if contract["input_schema"] == nil || contract["read_only"] != false {
				t.Fatalf("Bash contract = %#v, want input schema and non-readonly classification", contract)
			}
			return
		}
	}
	t.Fatalf("tool_contracts = %#v, want Bash contract", items)
}

func TestHandleWebSocketExtensionInventoryReturnsRuntimeProjection(t *testing.T) {
	sessionManager := session.NewManager(nil)
	server := NewServerWithOptions(log.New(io.Discard, "", 0), sessionManager, llm.NewMockClient(), Options{})
	httpServer := httptest.NewServer(http.HandlerFunc(server.HandleWebSocket))
	t.Cleanup(httpServer.Close)

	wsURL := "ws" + strings.TrimPrefix(httpServer.URL, "http")
	conn, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
	if err != nil {
		t.Fatalf("dial websocket: %v", err)
	}
	t.Cleanup(func() { _ = conn.Close() })

	if err := conn.WriteJSON(protocolws.Message{Type: protocolws.TypeRequest, ID: "1", Method: protocolws.MethodConnect, Payload: map[string]any{"role": "client", "client_identity": "test", "agent_id": "main"}}); err != nil {
		t.Fatalf("write connect: %v", err)
	}
	_ = conn.ReadJSON(&protocolws.Message{})
	_ = conn.ReadJSON(&protocolws.Message{})

	if err := conn.WriteJSON(protocolws.Message{Type: protocolws.TypeRequest, ID: "2", Method: protocolws.MethodExtensionInventory, Payload: map[string]any{}}); err != nil {
		t.Fatalf("write extension_inventory: %v", err)
	}
	var res protocolws.Message
	if err := conn.SetReadDeadline(time.Now().Add(2 * time.Second)); err != nil {
		t.Fatalf("set read deadline: %v", err)
	}
	if err := conn.ReadJSON(&res); err != nil {
		t.Fatalf("read extension_inventory: %v", err)
	}
	if res.Type != protocolws.TypeResponse || !res.OK {
		t.Fatalf("extension_inventory response = %#v, want ok", res)
	}
	inventory, ok := res.Payload["inventory"].(map[string]any)
	if !ok {
		t.Fatalf("inventory payload = %#v, want object", res.Payload["inventory"])
	}
	toolsPayload, ok := inventory["tools"].([]any)
	if !ok || len(toolsPayload) == 0 {
		t.Fatalf("tools payload = %#v, want non-empty runtime tool projection", inventory["tools"])
	}
	firstTool, _ := toolsPayload[0].(map[string]any)
	if firstTool["lifecycle_type"] != "tool" || firstTool["lifecycle_state"] == "" {
		t.Fatalf("tool lifecycle payload = %#v, want lifecycle projection fields", firstTool)
	}
	commandsPayload, ok := inventory["commands"].([]any)
	if !ok || len(commandsPayload) == 0 {
		t.Fatalf("commands payload = %#v, want runtime command projection", inventory["commands"])
	}
	var foundPermissions bool
	var foundStatusMetadata bool
	for _, raw := range commandsPayload {
		command, _ := raw.(map[string]any)
		if command["name"] == "permissions" && command["source"] == "runtime" {
			foundPermissions = true
		}
		if command["name"] == "status" && command["source"] == "runtime" {
			if command["type"] != "slash" ||
				command["lifecycle_type"] != "command" ||
				command["lifecycle_state"] != "active" ||
				command["category"] != "runtime" ||
				command["visibility"] != "always" ||
				command["behavior"] != "query" ||
				command["user_invocable"] != true {
				t.Fatalf("runtime /status command metadata = %#v, want full runtime metadata", command)
			}
			foundStatusMetadata = true
		}
	}
	if !foundPermissions {
		t.Fatalf("commands payload = %#v, want runtime /permissions command", commandsPayload)
	}
	if !foundStatusMetadata {
		t.Fatalf("commands payload = %#v, want runtime /status metadata", commandsPayload)
	}
	lsp, ok := inventory["lsp_boundaries"].([]any)
	if !ok || len(lsp) != 1 {
		t.Fatalf("lsp_boundaries = %#v, want deferred LSP boundary", inventory["lsp_boundaries"])
	}
	lspBoundary, _ := lsp[0].(map[string]any)
	if lspBoundary["lifecycle_type"] != "lsp_boundary" || lspBoundary["lifecycle_state"] != "discovered" {
		t.Fatalf("lsp lifecycle payload = %#v, want lifecycle projection", lspBoundary)
	}
	deferred, ok := inventory["deferred_capabilities"].([]any)
	if !ok || len(deferred) == 0 {
		t.Fatalf("deferred_capabilities = %#v, want explicit deferred capabilities", inventory["deferred_capabilities"])
	}
}

func TestHandleWebSocketExtensionInventorySerializesLSPRuntimeProjection(t *testing.T) {
	sessionManager := session.NewManager(nil)
	runner := runtimepkg.NewRunnerWithOptions(sessionManager, llm.NewMockClient(), workspace.NewLoader(""), tools.NewRegistry(), runtimepkg.Options{
		LSPServers: []tools.LSPServerConfig{{
			Name:                 "gopls",
			LanguageIDs:          []string{"go"},
			FilePatterns:         []string{"**/*.go"},
			Command:              "gopls",
			Args:                 []string{"serve"},
			WorkspaceRoot:        "C:/repo",
			Enabled:              true,
			ReadOnlyCapabilities: []string{"definition", "diagnostics"},
			MutatingCapabilities: []string{"rename"},
		}},
	})
	server := NewServerWithOptions(log.New(io.Discard, "", 0), sessionManager, llm.NewMockClient(), Options{Runner: runner})
	httpServer := httptest.NewServer(http.HandlerFunc(server.HandleWebSocket))
	t.Cleanup(httpServer.Close)

	wsURL := "ws" + strings.TrimPrefix(httpServer.URL, "http")
	conn, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
	if err != nil {
		t.Fatalf("dial websocket: %v", err)
	}
	t.Cleanup(func() { _ = conn.Close() })

	if err := conn.WriteJSON(protocolws.Message{Type: protocolws.TypeRequest, ID: "1", Method: protocolws.MethodConnect, Payload: map[string]any{"role": "client", "client_identity": "test", "agent_id": "main"}}); err != nil {
		t.Fatalf("write connect: %v", err)
	}
	_ = conn.ReadJSON(&protocolws.Message{})
	_ = conn.ReadJSON(&protocolws.Message{})

	if err := conn.WriteJSON(protocolws.Message{Type: protocolws.TypeRequest, ID: "2", Method: protocolws.MethodExtensionInventory, Payload: map[string]any{}}); err != nil {
		t.Fatalf("write extension_inventory: %v", err)
	}
	var res protocolws.Message
	if err := conn.SetReadDeadline(time.Now().Add(2 * time.Second)); err != nil {
		t.Fatalf("set read deadline: %v", err)
	}
	if err := conn.ReadJSON(&res); err != nil {
		t.Fatalf("read extension_inventory: %v", err)
	}
	inventory, ok := res.Payload["inventory"].(map[string]any)
	if !ok {
		t.Fatalf("inventory payload = %#v", res.Payload)
	}
	lsp, ok := inventory["lsp_boundaries"].([]any)
	if !ok || len(lsp) != 1 {
		t.Fatalf("lsp_boundaries = %#v, want configured server", inventory["lsp_boundaries"])
	}
	boundary, _ := lsp[0].(map[string]any)
	if boundary["name"] != "gopls" ||
		boundary["lifecycle_state"] != tools.LSPStateConfigured ||
		boundary["status"] != tools.LSPStateConfigured ||
		boundary["workspace_root"] != "C:/repo" ||
		boundary["permission_classification"] != tools.LSPPermissionMixed {
		t.Fatalf("lsp boundary payload = %#v", boundary)
	}
	if got := boundary["language_ids"].([]any); len(got) != 1 || got[0] != "go" {
		t.Fatalf("language ids payload = %#v", boundary["language_ids"])
	}
	if got := boundary["read_only_capabilities"].([]any); len(got) != 2 {
		t.Fatalf("read_only_capabilities payload = %#v", boundary["read_only_capabilities"])
	}
	if got := boundary["mutating_capabilities"].([]any); len(got) != 1 || got[0] != "rename" {
		t.Fatalf("mutating_capabilities payload = %#v", boundary["mutating_capabilities"])
	}
}

func TestHandleWebSocketRemoteBridgeStateProjection(t *testing.T) {
	sessionManager := session.NewManager(nil)
	approvalManager := approval.NewManager()
	mainSession := sessionManager.GetOrCreateMain("main")
	pendingApproval := approvalManager.Create(mainSession.ID, "run-1", "msg-1", "system.run", "pwd", "approval required", "approval", "")
	runner := runtimepkg.NewRunnerWithOptions(sessionManager, llm.NewMockClient(), workspace.NewLoader(""), tools.NewRegistry(), runtimepkg.Options{
		ApprovalManager: approvalManager,
	})
	server := NewServerWithOptions(log.New(io.Discard, "", 0), sessionManager, llm.NewMockClient(), Options{Runner: runner})
	httpServer := httptest.NewServer(http.HandlerFunc(server.HandleWebSocket))
	t.Cleanup(httpServer.Close)

	wsURL := "ws" + strings.TrimPrefix(httpServer.URL, "http")
	conn, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
	if err != nil {
		t.Fatalf("dial websocket: %v", err)
	}
	t.Cleanup(func() { _ = conn.Close() })

	if err := conn.WriteJSON(protocolws.Message{Type: protocolws.TypeRequest, ID: "1", Method: protocolws.MethodConnect, Payload: map[string]any{"role": "sdk", "client_identity": "sdk-client", "agent_id": "main"}}); err != nil {
		t.Fatalf("write connect: %v", err)
	}
	_ = conn.ReadJSON(&protocolws.Message{})
	_ = conn.ReadJSON(&protocolws.Message{})

	if err := conn.WriteJSON(protocolws.Message{Type: protocolws.TypeRequest, ID: "2", Method: protocolws.MethodRemoteHeartbeat, Payload: map[string]any{
		"connection_id":   "conn-remote",
		"client_identity": "sdk-client",
		"device_id":       "device-1",
		"user_id":         "user-1",
		"agent_id":        "main",
		"transport_kind":  "websocket",
		"capabilities":    []any{"heartbeat", "approval_forwarding"},
	}}); err != nil {
		t.Fatalf("write remote heartbeat: %v", err)
	}
	var heartbeat protocolws.Message
	if err := conn.SetReadDeadline(time.Now().Add(2 * time.Second)); err != nil {
		t.Fatalf("set read deadline: %v", err)
	}
	if err := conn.ReadJSON(&heartbeat); err != nil {
		t.Fatalf("read remote heartbeat: %v", err)
	}
	if heartbeat.Type != protocolws.TypeResponse || !heartbeat.OK {
		t.Fatalf("remote heartbeat response = %#v", heartbeat)
	}
	remote, ok := heartbeat.Payload["remote"].(map[string]any)
	if !ok {
		t.Fatalf("remote payload = %#v", heartbeat.Payload)
	}
	identities, ok := remote["identities"].([]any)
	if !ok || len(identities) != 1 {
		t.Fatalf("remote identities = %#v", remote["identities"])
	}
	identity, _ := identities[0].(map[string]any)
	if identity["connection_id"] != "conn-remote" ||
		identity["trust_state"] != string(runtimepkg.RemoteTrustUnknown) ||
		identity["liveness_state"] != string(runtimepkg.RemoteLivenessConnected) {
		t.Fatalf("remote identity payload = %#v", identity)
	}

	if err := conn.WriteJSON(protocolws.Message{Type: protocolws.TypeRequest, ID: "3", Method: protocolws.MethodRemoteTrustUpdate, Payload: map[string]any{
		"connection_id": "conn-remote",
		"trust_state":   string(runtimepkg.RemoteTrustTrusted),
	}}); err != nil {
		t.Fatalf("write trust update: %v", err)
	}
	var trusted protocolws.Message
	if err := conn.ReadJSON(&trusted); err != nil {
		t.Fatalf("read trust update: %v", err)
	}
	remote = trusted.Payload["remote"].(map[string]any)
	identity = remote["identities"].([]any)[0].(map[string]any)
	if identity["trust_state"] != string(runtimepkg.RemoteTrustTrusted) {
		t.Fatalf("trusted identity payload = %#v", identity)
	}

	if err := conn.WriteJSON(protocolws.Message{Type: protocolws.TypeRequest, ID: "4", Method: protocolws.MethodRemoteApprovalCorrelate, Payload: map[string]any{
		"local_approval_id":     pendingApproval.ID,
		"remote_correlation_id": "remote-approval-1",
		"connection_id":         "conn-remote",
		"client_identity":       "sdk-client",
		"device_id":             "device-1",
		"status":                string(runtimepkg.RemoteApprovalPending),
		"decision_payload":      map[string]any{"kind": "prompt"},
	}}); err != nil {
		t.Fatalf("write approval correlation: %v", err)
	}
	var correlation protocolws.Message
	if err := conn.ReadJSON(&correlation); err != nil {
		t.Fatalf("read approval correlation: %v", err)
	}
	remote = correlation.Payload["remote"].(map[string]any)
	correlations, ok := remote["approval_correlations"].([]any)
	if !ok || len(correlations) != 1 {
		t.Fatalf("approval correlations = %#v", remote["approval_correlations"])
	}
	item := correlations[0].(map[string]any)
	if item["local_approval_id"] != pendingApproval.ID || item["remote_correlation_id"] != "remote-approval-1" {
		t.Fatalf("approval correlation payload = %#v", item)
	}
}

func TestHandleWebSocketRemoteHeartbeatPreservesTrustState(t *testing.T) {
	sessionManager := session.NewManager(nil)
	server := NewServerWithOptions(log.New(io.Discard, "", 0), sessionManager, llm.NewMockClient(), Options{})
	conn := connectGatewayTestClient(t, server)

	heartbeatPayload := map[string]any{
		"connection_id":   "conn-remote",
		"client_identity": "sdk-client",
		"device_id":       "device-1",
		"user_id":         "user-1",
		"agent_id":        "main",
		"transport_kind":  "websocket",
		"capabilities":    []any{"heartbeat", "approval_forwarding"},
	}
	if err := conn.WriteJSON(protocolws.Message{Type: protocolws.TypeRequest, ID: "heartbeat-1", Method: protocolws.MethodRemoteHeartbeat, Payload: heartbeatPayload}); err != nil {
		t.Fatalf("write first heartbeat: %v", err)
	}
	firstHeartbeat := readGatewayResponse(t, conn)
	if !firstHeartbeat.OK {
		t.Fatalf("first heartbeat response = %#v, want ok", firstHeartbeat)
	}

	if err := conn.WriteJSON(protocolws.Message{Type: protocolws.TypeRequest, ID: "trust-1", Method: protocolws.MethodRemoteTrustUpdate, Payload: map[string]any{
		"connection_id": "conn-remote",
		"trust_state":   string(runtimepkg.RemoteTrustTrusted),
	}}); err != nil {
		t.Fatalf("write trust update: %v", err)
	}
	trusted := readGatewayResponse(t, conn)
	if !trusted.OK {
		t.Fatalf("trust update response = %#v, want ok", trusted)
	}

	if err := conn.WriteJSON(protocolws.Message{Type: protocolws.TypeRequest, ID: "heartbeat-2", Method: protocolws.MethodRemoteHeartbeat, Payload: heartbeatPayload}); err != nil {
		t.Fatalf("write second heartbeat: %v", err)
	}
	secondHeartbeat := readGatewayResponse(t, conn)
	identity := firstRemoteIdentityPayload(t, secondHeartbeat)
	if identity["trust_state"] != string(runtimepkg.RemoteTrustTrusted) {
		t.Fatalf("trust state after second heartbeat = %#v, want trusted", identity)
	}

	if err := conn.WriteJSON(protocolws.Message{Type: protocolws.TypeRequest, ID: "trust-2", Method: protocolws.MethodRemoteTrustUpdate, Payload: map[string]any{
		"connection_id": "conn-remote",
		"trust_state":   string(runtimepkg.RemoteTrustRevoked),
	}}); err != nil {
		t.Fatalf("write revoked trust update: %v", err)
	}
	revoked := readGatewayResponse(t, conn)
	if !revoked.OK {
		t.Fatalf("revoked trust update response = %#v, want ok", revoked)
	}

	if err := conn.WriteJSON(protocolws.Message{Type: protocolws.TypeRequest, ID: "heartbeat-3", Method: protocolws.MethodRemoteHeartbeat, Payload: heartbeatPayload}); err != nil {
		t.Fatalf("write third heartbeat: %v", err)
	}
	thirdHeartbeat := readGatewayResponse(t, conn)
	identity = firstRemoteIdentityPayload(t, thirdHeartbeat)
	if identity["trust_state"] != string(runtimepkg.RemoteTrustRevoked) {
		t.Fatalf("trust state after revoked heartbeat = %#v, want revoked", identity)
	}
}

func TestHandleWebSocketRemoteApprovalCorrelateValidatesLocalApprovalAuthority(t *testing.T) {
	sessionManager := session.NewManager(nil)
	mainSession := sessionManager.GetOrCreateMain("main")
	otherSession := sessionManager.CreateSession("main")
	approvalManager := approval.NewManager()
	otherApproval := approvalManager.Create(otherSession.ID, "run-1", "msg-1", "system.run", "pwd", "approval required", "approval", "")
	validApproval := approvalManager.Create(mainSession.ID, "run-2", "msg-2", "system.run", "pwd", "approval required", "approval", "")
	runner := runtimepkg.NewRunnerWithOptions(sessionManager, llm.NewMockClient(), workspace.NewLoader(""), tools.NewRegistry(), runtimepkg.Options{
		ApprovalManager: approvalManager,
	})
	server := NewServerWithOptions(log.New(io.Discard, "", 0), sessionManager, llm.NewMockClient(), Options{Runner: runner})
	conn := connectGatewayTestClient(t, server)

	writeRemoteApprovalCorrelation := func(id, localApprovalID, remoteCorrelationID string) protocolws.Message {
		t.Helper()
		if err := conn.WriteJSON(protocolws.Message{Type: protocolws.TypeRequest, ID: id, Method: protocolws.MethodRemoteApprovalCorrelate, Payload: map[string]any{
			"local_approval_id":     localApprovalID,
			"remote_correlation_id": remoteCorrelationID,
			"connection_id":         "conn-remote",
			"client_identity":       "sdk-client",
			"device_id":             "device-1",
			"status":                string(runtimepkg.RemoteApprovalPending),
			"decision_payload":      map[string]any{"kind": "prompt"},
		}}); err != nil {
			t.Fatalf("write approval correlation %s: %v", id, err)
		}
		return readGatewayResponse(t, conn)
	}

	missing := writeRemoteApprovalCorrelation("missing", "approval-missing", "remote-missing")
	if missing.OK || missing.Error == nil || !strings.Contains(missing.Error.Message, "not found") {
		t.Fatalf("missing approval correlation response = %#v, want not found error", missing)
	}
	if snapshot := runner.RemoteSnapshot(mainSession.ID); len(snapshot.ApprovalCorrelations) != 0 {
		t.Fatalf("snapshot after missing approval correlation = %#v, want none", snapshot.ApprovalCorrelations)
	}

	crossSession := writeRemoteApprovalCorrelation("cross-session", otherApproval.ID, "remote-other")
	if crossSession.OK || crossSession.Error == nil || !strings.Contains(crossSession.Error.Message, "belongs to session") {
		t.Fatalf("cross-session approval correlation response = %#v, want session ownership error", crossSession)
	}
	if snapshot := runner.RemoteSnapshot(mainSession.ID); len(snapshot.ApprovalCorrelations) != 0 {
		t.Fatalf("snapshot after cross-session approval correlation = %#v, want none", snapshot.ApprovalCorrelations)
	}

	valid := writeRemoteApprovalCorrelation("valid", validApproval.ID, "remote-valid")
	if !valid.OK {
		t.Fatalf("valid approval correlation response = %#v, want ok", valid)
	}
	snapshot := runner.RemoteSnapshot(mainSession.ID)
	if len(snapshot.ApprovalCorrelations) != 1 || snapshot.ApprovalCorrelations[0].LocalApprovalID != validApproval.ID {
		t.Fatalf("snapshot after valid approval correlation = %#v, want one valid record", snapshot.ApprovalCorrelations)
	}
}

func firstRemoteIdentityPayload(t *testing.T, message protocolws.Message) map[string]any {
	t.Helper()
	if message.Type != protocolws.TypeResponse || !message.OK {
		t.Fatalf("remote response = %#v, want ok response", message)
	}
	remote, ok := message.Payload["remote"].(map[string]any)
	if !ok {
		t.Fatalf("remote payload = %#v, want object", message.Payload["remote"])
	}
	identities, ok := remote["identities"].([]any)
	if !ok || len(identities) != 1 {
		t.Fatalf("remote identities = %#v, want one identity", remote["identities"])
	}
	identity, ok := identities[0].(map[string]any)
	if !ok {
		t.Fatalf("remote identity = %#v, want object", identities[0])
	}
	return identity
}

func TestRuntimeSinkSerializesUnknownRuntimeEventsWithSharedPayload(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		upgrader := websocket.Upgrader{CheckOrigin: func(*http.Request) bool { return true }}
		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			t.Errorf("upgrade: %v", err)
			return
		}

		client := NewClient("client-1", conn)
		client.BindSession("session-1", "agent:main:main")
		if err := (runtimeSink{client: client}).Emit(runtimepkg.RuntimeEvent{Type: runtimepkg.EventCommandCompleted, RunID: "run-1", Error: "command failed"}); err != nil {
			t.Errorf("emit: %v", err)
		}
	}))
	t.Cleanup(server.Close)

	wsURL := "ws" + strings.TrimPrefix(server.URL, "http")
	conn, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
	if err != nil {
		t.Fatalf("dial websocket: %v", err)
	}
	t.Cleanup(func() { _ = conn.Close() })

	var msg protocolws.Message
	if err := conn.SetReadDeadline(time.Now().Add(2 * time.Second)); err != nil {
		t.Fatalf("set read deadline: %v", err)
	}
	if err := conn.ReadJSON(&msg); err != nil {
		t.Fatalf("read event: %v", err)
	}
	if msg.Event != runtimepkg.EventCommandCompleted || msg.Payload["error"] != "command failed" || msg.Payload["message"] != "command failed" {
		t.Fatalf("event = %#v, want shared runtime payload", msg)
	}
}
