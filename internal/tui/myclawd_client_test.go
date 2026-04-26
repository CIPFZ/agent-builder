package tui

import (
	"context"
	"encoding/json"
	"log"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/gorilla/websocket"

	"myclaw/internal/gateway"
	protocolws "myclaw/internal/protocol/ws"
	"myclaw/internal/session"
)

func TestWebsocketURLDefaultsPath(t *testing.T) {
	got, err := websocketURL("http://127.0.0.1:8080")
	if err != nil {
		t.Fatalf("websocketURL error: %v", err)
	}
	if got != "ws://127.0.0.1:8080/ws" {
		t.Fatalf("url = %q", got)
	}
}

func TestMyclawdClientConnectAndSnapshots(t *testing.T) {
	sessions := session.NewManager(nil)
	server := gateway.NewServerWithOptions(log.Default(), sessions, nil, gateway.Options{})
	mux := http.NewServeMux()
	mux.HandleFunc("/ws", server.HandleWebSocket)
	httpServer := httptest.NewServer(mux)
	defer httpServer.Close()

	store := newClientStore()
	client := NewMyclawdClient(context.Background(), httpServer.URL+"/ws", "main", store, nil)
	if err := client.Start(); err != nil {
		t.Fatalf("Start error: %v", err)
	}
	defer client.Close()

	status := client.PlatformStatusSnapshot()
	if status.SessionID == "" || status.SessionKey == "" {
		t.Fatalf("unexpected session snapshot: %#v", status)
	}
}

func TestMyclawdClientSendUserMessageCreatesUserEvent(t *testing.T) {
	client, cleanup, msgCh, _ := startGatewayBackedClient(t)
	defer cleanup()

	if err := client.SendUserMessage("hello from tui"); err != nil {
		t.Fatalf("SendUserMessage error: %v", err)
	}

	msg := waitForClientEvent(t, msgCh, func(event clientEvent) bool {
		return event.Type == "message.created" && event.Message != nil && event.Message.Role == "user"
	})
	if msg.Event.Message.Content != "hello from tui" {
		t.Fatalf("user content = %q", msg.Event.Message.Content)
	}
}

func TestMyclawdClientSendUserMessageDoesNotRefreshSnapshots(t *testing.T) {
	upgrader := websocket.Upgrader{CheckOrigin: func(*http.Request) bool { return true }}
	methods := make(chan string, 16)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			t.Errorf("upgrade: %v", err)
			return
		}
		defer conn.Close()
		for {
			var req protocolws.Message
			if err := conn.ReadJSON(&req); err != nil {
				return
			}
			methods <- req.Method
			writeHarnessResponse(t, conn, req.ID, defaultHarnessPayloadFor(req.Method))
		}
	}))
	defer server.Close()

	client := NewMyclawdClient(context.Background(), server.URL+"/ws", "main", newClientStore(), nil)
	if err := client.Start(); err != nil {
		t.Fatalf("Start error: %v", err)
	}
	defer client.Close()
	drainMethods(methods)
	errCh := make(chan error, 1)
	go func() {
		errCh <- client.SendUserMessage("hello")
	}()

	if method := waitForMethod(t, methods); method != protocolws.MethodSendMessage {
		t.Fatalf("method = %q, want send message", method)
	}
	select {
	case err := <-errCh:
		if err != nil {
			t.Fatalf("SendUserMessage error: %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("SendUserMessage did not return after send ack")
	}

	select {
	case method := <-methods:
		t.Fatalf("unexpected follow-up request after send: %s", method)
	case <-time.After(150 * time.Millisecond):
	}
}

func TestMyclawdClientUpdatesStoreForProgressApprovalAndTasks(t *testing.T) {
	store := newClientStore()
	client, cleanup, msgCh, conn := startClientWithHarness(t, store)
	defer cleanup()

	respondToOutstandingRequests(t, conn)
	writeHarnessEvent(t, conn, protocolws.EventMessage("tool.called", map[string]any{
		"run_id":      "run-1",
		"tool_name":   "Read",
		"tool_use_id": "toolu-1",
		"tool_input":  `{"file_path":"README.md"}`,
		"session_id":  "sess-1",
		"session_key": "agent:main:main",
		"agent_id":    "main",
		"is_main":     true,
	}))
	writeHarnessEvent(t, conn, protocolws.EventMessage("tool.progress", map[string]any{
		"run_id":      "run-1",
		"tool_name":   "Read",
		"tool_use_id": "toolu-1",
		"type":        "read.progress",
		"message":     "Reading README.md",
		"data": map[string]any{
			"output": "line-1\nline-2",
		},
		"session_id":  "sess-1",
		"session_key": "agent:main:main",
	}))
	writeHarnessEvent(t, conn, protocolws.EventMessage("tool.result", map[string]any{
		"run_id":      "run-1",
		"tool_name":   "Read",
		"tool_use_id": "toolu-1",
		"message": map[string]any{
			"id":      "tool-msg-1",
			"role":    "tool",
			"content": "Read: hello",
			"blocks": []any{
				map[string]any{
					"type":        "tool_result",
					"tool_use_id": "toolu-1",
					"content":     "hello",
				},
			},
		},
		"session_id":  "sess-1",
		"session_key": "agent:main:main",
	}))
	writeHarnessEvent(t, conn, protocolws.EventMessage("permission.required", map[string]any{
		"approval_id": "approval-1",
		"run_id":      "run-2",
		"tool_name":   "system.run",
		"tool_input":  "pwd",
		"reason":      "needs approval",
		"status":      "pending",
		"category":    "shell",
		"rule_source": "policy",
		"session_id":  "sess-1",
		"session_key": "agent:main:main",
	}))
	writeHarnessEvent(t, conn, protocolws.EventMessage(protocolws.EventSubagentUpdated, map[string]any{
		"run_id":            "agent-1",
		"label":             "research",
		"status":            "running",
		"parent_session_id": "sess-1",
		"child_session_id":  "child-1",
		"message":           "Working",
	}))

	waitForClientEvent(t, msgCh, func(event clientEvent) bool { return event.Type == "tool.result" })
	waitForClientEvent(t, msgCh, func(event clientEvent) bool { return event.Type == "permission.required" })
	waitForClientEvent(t, msgCh, func(event clientEvent) bool { return event.Type == protocolws.EventSubagentUpdated })

	snap := store.snapshot()
	if len(snap.Transcript) == 0 {
		t.Fatal("expected transcript entries in store")
	}
	last := snap.Transcript[len(snap.Transcript)-1]
	if last.Role != "tool" || last.ToolStatus != toolStatusSucceeded || last.Content != "hello" {
		t.Fatalf("last transcript entry = %#v", last)
	}
	if snap.Approval == nil || snap.Approval.ID != "approval-1" {
		t.Fatalf("approval snapshot = %#v", snap.Approval)
	}
	taskSnap := client.TaskPanelSnapshot()
	if len(taskSnap.Tasks) == 0 || taskSnap.Tasks[0].RunID != "agent-1" {
		t.Fatalf("task snapshot = %#v", taskSnap)
	}
}

func TestMyclawdClientDisconnectReturnsRequestError(t *testing.T) {
	store := newClientStore()
	client, cleanup, _, conn := startClientWithHarness(t, store)
	defer cleanup()

	if err := client.Approve(""); err == nil {
		t.Fatal("expected missing approval id error")
	}

	if err := conn.Close(); err != nil {
		t.Fatalf("harness close: %v", err)
	}
	time.Sleep(50 * time.Millisecond)
	if err := client.Reject("approval-1"); err == nil {
		t.Fatal("expected reject failure after disconnect")
	}
}

func TestModelUpdateWithStoreBackedClientEventShowsApproval(t *testing.T) {
	store := newClientStore()
	model := NewModel(&fakeBridge{}, ModelConfig{LLMLabel: "test-model"})
	model.bindStore(store)

	updated, _ := model.Update(RuntimeEventMsg{Event: clientEvent{
		Type: "permission.required",
		Tool: &clientToolEvent{
			Approval: &clientApproval{
				ID:        "approval-1",
				ToolName:  "system.run",
				ToolInput: "pwd",
				Reason:    "needs approval",
				Status:    "pending",
			},
		},
	}})
	model = updated.(Model)

	if model.pendingApproval == nil || model.pendingApproval.ID != "approval-1" {
		t.Fatalf("pending approval = %#v", model.pendingApproval)
	}
	if !model.approvalDialog.active() {
		t.Fatal("approval dialog should be active")
	}
}

func TestClientStoreTracksLatestApproval(t *testing.T) {
	store := newClientStore()
	store.applyApproval(approvalView{ID: "approval-1", ToolName: "system.run", Status: "pending"})
	store.applyApproval(approvalView{ID: "approval-2", ToolName: "read_file", Status: "pending"})

	latest, ok := store.latestApproval()
	if !ok {
		t.Fatal("expected latest approval")
	}
	if latest.ID != "approval-2" {
		t.Fatalf("latest approval = %#v", latest)
	}
}

func startGatewayBackedClient(t *testing.T) (*MyclawdClient, func(), chan RuntimeEventMsg, *clientStore) {
	t.Helper()
	sessions := session.NewManager(nil)
	server := gateway.NewServerWithOptions(log.Default(), sessions, nil, gateway.Options{})
	mux := http.NewServeMux()
	mux.HandleFunc("/ws", server.HandleWebSocket)
	httpServer := httptest.NewServer(mux)

	store := newClientStore()
	client := NewMyclawdClient(context.Background(), httpServer.URL+"/ws", "main", store, nil)
	msgCh := make(chan RuntimeEventMsg, 16)
	client.Attach(func(msg tea.Msg) {
		if event, ok := msg.(RuntimeEventMsg); ok {
			msgCh <- event
		}
	})
	if err := client.Start(); err != nil {
		httpServer.Close()
		t.Fatalf("Start error: %v", err)
	}
	return client, func() {
		_ = client.Close()
		httpServer.Close()
	}, msgCh, store
}

func startClientWithHarness(t *testing.T, store *clientStore) (*MyclawdClient, func(), chan RuntimeEventMsg, *websocket.Conn) {
	t.Helper()
	upgrader := websocket.Upgrader{CheckOrigin: func(*http.Request) bool { return true }}
	connCh := make(chan *websocket.Conn, 1)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			t.Errorf("upgrade: %v", err)
			return
		}
		connCh <- conn
		req := readHarnessRequest(t, conn)
		if req.Method != protocolws.MethodConnect {
			t.Fatalf("first method = %q", req.Method)
		}
		writeHarnessResponse(t, conn, req.ID, map[string]any{
			"session_id":  "sess-1",
			"session_key": "agent:main:main",
		})
		writeHarnessEvent(t, conn, protocolws.EventMessage(protocolws.EventHello, map[string]any{
			"session_id":  "sess-1",
			"session_key": "agent:main:main",
			"agent_id":    "main",
		}))
	}))

	client := NewMyclawdClient(context.Background(), server.URL+"/ws", "main", store, nil)
	msgCh := make(chan RuntimeEventMsg, 32)
	client.Attach(func(msg tea.Msg) {
		if event, ok := msg.(RuntimeEventMsg); ok {
			msgCh <- event
		}
	})
	if err := client.Start(); err != nil {
		server.Close()
		t.Fatalf("Start error: %v", err)
	}

	var conn *websocket.Conn
	select {
	case conn = <-connCh:
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for harness connection")
	}

	return client, func() {
		_ = client.Close()
		_ = conn.Close()
		server.Close()
	}, msgCh, conn
}

func respondToOutstandingRequests(t *testing.T, conn *websocket.Conn) {
	t.Helper()
	deadline := time.Now().Add(250 * time.Millisecond)
	_ = conn.SetReadDeadline(deadline)
	defer conn.SetReadDeadline(time.Time{})
	for {
		var msg protocolws.Message
		if err := conn.ReadJSON(&msg); err != nil {
			if websocket.IsCloseError(err) {
				return
			}
			if ne, ok := err.(interface{ Timeout() bool }); ok && ne.Timeout() {
				return
			}
			return
		}
		writeHarnessResponse(t, conn, msg.ID, defaultHarnessPayloadFor(msg.Method))
	}
}

func defaultHarnessPayloadFor(method string) map[string]any {
	switch method {
	case protocolws.MethodSessionStatus:
		return map[string]any{
			"session_id":                       "sess-1",
			"session_key":                      "agent:main:main",
			"agent_id":                         "main",
			"is_main":                          true,
			"workspace_roots":                  []any{"C:/repo"},
			"main_loop_model":                  "base-model",
			"session_main_loop_model_override": "",
			"resolved_main_loop_model":         "base-model",
		}
	case protocolws.MethodSubagentList:
		return map[string]any{"runs": []any{}}
	case protocolws.MethodMCPStatus:
		return map[string]any{"servers": []any{}}
	case protocolws.MethodApprovalList:
		return map[string]any{"approvals": []any{}}
	default:
		return map[string]any{}
	}
}

func readHarnessRequest(t *testing.T, conn *websocket.Conn) protocolws.Message {
	t.Helper()
	_ = conn.SetReadDeadline(time.Now().Add(2 * time.Second))
	defer conn.SetReadDeadline(time.Time{})
	var msg protocolws.Message
	if err := conn.ReadJSON(&msg); err != nil {
		t.Fatalf("ReadJSON: %v", err)
	}
	return msg
}

func writeHarnessResponse(t *testing.T, conn *websocket.Conn, id string, payload map[string]any) {
	t.Helper()
	if err := conn.WriteJSON(protocolws.Message{
		Type:    protocolws.TypeResponse,
		ID:      id,
		OK:      true,
		Payload: payload,
	}); err != nil {
		t.Fatalf("WriteJSON response: %v", err)
	}
}

func writeHarnessEvent(t *testing.T, conn *websocket.Conn, msg protocolws.Message) {
	t.Helper()
	if msg.Payload == nil {
		msg.Payload = map[string]any{}
	}
	normalized := protocolws.Message{Type: msg.Type, Event: msg.Event}
	raw, err := json.Marshal(msg.Payload)
	if err != nil {
		t.Fatalf("marshal payload: %v", err)
	}
	if err := json.Unmarshal(raw, &normalized.Payload); err != nil {
		t.Fatalf("unmarshal payload: %v", err)
	}
	if err := conn.WriteJSON(normalized); err != nil {
		t.Fatalf("WriteJSON event: %v", err)
	}
}

func drainMethods(methods <-chan string) {
	for {
		select {
		case <-methods:
		default:
			return
		}
	}
}

func waitForMethod(t *testing.T, methods <-chan string) string {
	t.Helper()
	select {
	case method := <-methods:
		return method
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for websocket request")
		return ""
	}
}

func waitForClientEvent(t *testing.T, ch <-chan RuntimeEventMsg, match func(clientEvent) bool) RuntimeEventMsg {
	t.Helper()
	deadline := time.After(2 * time.Second)
	for {
		select {
		case msg := <-ch:
			if match(msg.Event) {
				return msg
			}
		case <-deadline:
			t.Fatal("timed out waiting for client event")
		}
	}
}
