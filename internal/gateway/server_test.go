package gateway

import (
	"context"
	"io"
	"log"
	"net/http"
	"net/http/httptest"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/gorilla/websocket"

	"myclaw/internal/agent"
	"myclaw/internal/llm"
	"myclaw/internal/orchestration"
	"myclaw/internal/permissions"
	protocolws "myclaw/internal/protocol/ws"
	"myclaw/internal/session"
)

type orchestrationHook struct {
	events []orchestration.Event
}

func (h *orchestrationHook) Handle(_ context.Context, event orchestration.Event) error {
	h.events = append(h.events, event)
	return nil
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

func TestHandleWebSocketConnectAndSendMessage(t *testing.T) {
	sessionManager := session.NewManager(nil)
	server := NewServer(log.New(io.Discard, "", 0), sessionManager, nil)
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
	if len(messages) != 2 {
		t.Fatalf("message count = %d, want 2", len(messages))
	}
	if messages[0].Content != "hello" {
		t.Fatalf("stored message content = %q, want %q", messages[0].Content, "hello")
	}
	if messages[1].Role != "assistant" || messages[1].Content != "Received: hello [workspace:3]" {
		t.Fatalf("stored assistant message = %#v, want assistant reply", messages[1])
	}
}

func TestHandleWebSocketToolLoop(t *testing.T) {
	sessionManager := session.NewManager(nil)
	server := NewServer(log.New(io.Discard, "", 0), sessionManager, nil)
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

func TestHandleWebSocketDoesNotEmitRunErrorWhenApprovalIsRequired(t *testing.T) {
	sessionManager := session.NewManager(nil)
	server := NewServerWithOptions(log.New(io.Discard, "", 0), sessionManager, nil, Options{
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

func TestHandleWebSocketApprovalListReturnsPendingRequests(t *testing.T) {
	sessionManager := session.NewManager(nil)
	server := NewServerWithOptions(log.New(io.Discard, "", 0), sessionManager, nil, Options{
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
	server := NewServerWithOptions(log.New(io.Discard, "", 0), sessionManager, nil, Options{
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
	server := NewServerWithOptions(log.New(io.Discard, "", 0), sessionManager, nil, Options{
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

func TestHandleWebSocketApprovalListCanFilterByStatus(t *testing.T) {
	sessionManager := session.NewManager(nil)
	server := NewServerWithOptions(log.New(io.Discard, "", 0), sessionManager, nil, Options{
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
	server := NewServerWithOptions(log.New(io.Discard, "", 0), sessionManager, nil, Options{
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
	server := NewServerWithOptions(log.New(io.Discard, "", 0), sessionManager, nil, Options{
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
	server := NewServerWithOptions(log.New(io.Discard, "", 0), sessionManager, nil, Options{
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
	server := NewServer(log.New(io.Discard, "", 0), sessionManager, nil)
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
	server := NewServerWithOptions(log.New(io.Discard, "", 0), sessionManager, nil, Options{
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
	server := NewServerWithOptions(log.New(io.Discard, "", 0), sessionManager, nil, Options{
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

func TestHandleWebSocketSessionSetPermissionCascadeUpdatesExistingSubagentStatus(t *testing.T) {
	sessionManager := session.NewManager(nil)
	server := NewServerWithOptions(log.New(io.Discard, "", 0), sessionManager, nil, Options{
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
	server := NewServer(log.New(io.Discard, "", 0), sessionManager, nil)
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
	server := NewServer(log.New(io.Discard, "", 0), sessionManager, nil)
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
	server := NewServer(log.New(io.Discard, "", 0), sessionManager, nil)
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
	server := NewServer(log.New(io.Discard, "", 0), sessionManager, nil)
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
	server := NewServer(log.New(io.Discard, "", 0), sessionManager, nil)
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
			if got := event.Payload["status"]; got != "steered" {
				t.Fatalf("subagent.updated status = %#v, want steered", got)
			}
			return
		}
	}
	t.Fatal("expected subagent.updated event")
}

func TestHandleWebSocketSubagentSteerEmitsOrchestrationUpdatedEvent(t *testing.T) {
	sessionManager := session.NewManager(nil)
	server := NewServer(log.New(io.Discard, "", 0), sessionManager, nil)
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
			if got := event.Payload["status"]; got != "steered" {
				t.Fatalf("orchestration.updated status = %#v, want steered", got)
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
	server := NewServer(log.New(io.Discard, "", 0), sessionManager, nil)
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
	server := NewServerWithOptions(log.New(io.Discard, "", 0), sessionManager, nil, Options{
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
	for _, event := range hook.events {
		if event.Type == "subagent.updated" && event.RunID == run.ID && event.Status == "steered" {
			found = true
			break
		}
	}
	if !found {
		deadline := time.Now().Add(500 * time.Millisecond)
		for time.Now().Before(deadline) {
			for _, event := range hook.events {
				if event.Type == "subagent.updated" && event.RunID == run.ID && event.Status == "steered" {
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
		t.Fatalf("expected subagent.updated orchestration hook event, got %#v", hook.events)
	}
}

func TestHandleWebSocketOrchestrationStatusReturnsTrackedRuns(t *testing.T) {
	sessionManager := session.NewManager(nil)
	server := NewServer(log.New(io.Discard, "", 0), sessionManager, nil)
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
	server := NewServer(log.New(io.Discard, "", 0), sessionManager, nil)
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
	server := NewServer(log.New(io.Discard, "", 0), sessionManager, nil)
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
	server := NewServer(log.New(io.Discard, "", 0), sessionManager, nil)
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
	server := NewServerWithOptions(log.New(io.Discard, "", 0), sessionManager, nil, Options{
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
	server := NewServerWithOptions(log.New(io.Discard, "", 0), sessionManager, nil, Options{
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
	server := NewServerWithOptions(log.New(io.Discard, "", 0), sessionManager, nil, Options{
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
	server := NewServerWithOptions(log.New(io.Discard, "", 0), sessionManager, nil, Options{
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
	server := NewServerWithOptions(log.New(io.Discard, "", 0), sessionManager, nil, Options{
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
	server := NewServerWithOptions(log.New(io.Discard, "", 0), sessionManager, nil, Options{
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
	server := NewServerWithOptions(log.New(io.Discard, "", 0), sessionManager, nil, Options{
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
	server := NewServerWithOptions(log.New(io.Discard, "", 0), sessionManager, nil, Options{
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
	server := NewServerWithOptions(log.New(io.Discard, "", 0), sessionManager, nil, Options{
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
	server := NewServerWithOptions(log.New(io.Discard, "", 0), sessionManager, nil, Options{
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
	server := NewServerWithOptions(log.New(io.Discard, "", 0), sessionManager, nil, Options{
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
	server := NewServerWithOptions(log.New(io.Discard, "", 0), sessionManager, nil, Options{
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
	server := NewServerWithOptions(log.New(io.Discard, "", 0), sessionManager, nil, Options{
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
	server := NewServerWithOptions(log.New(io.Discard, "", 0), sessionManager, nil, Options{
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
	server := NewServerWithOptions(log.New(io.Discard, "", 0), sessionManager, nil, Options{
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
	server := NewServerWithOptions(log.New(io.Discard, "", 0), sessionManager, nil, Options{
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
	server := NewServerWithOptions(log.New(io.Discard, "", 0), sessionManager, nil, Options{
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
	server := NewServer(log.New(io.Discard, "", 0), sessionManager, nil)
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
	server := NewServerWithOptions(log.New(io.Discard, "", 0), sessionManager, nil, Options{
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
	server := NewServer(log.New(io.Discard, "", 0), sessionManager, nil)
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
	server := NewServerWithOptions(log.New(io.Discard, "", 0), sessionManager, nil, Options{
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
	server := NewServer(log.New(io.Discard, "", 0), sessionManager, nil)
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

func TestHandleWebSocketSystemRunToolLoop(t *testing.T) {
	sessionManager := session.NewManager(nil)
	server := NewServer(log.New(io.Discard, "", 0), sessionManager, nil)
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
	server := NewServer(log.New(io.Discard, "", 0), sessionManager, nil)
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
			if message, ok := msg.Payload["message"].(map[string]any); ok {
				if strings.Contains(message["content"].(string), "system.run: [sandbox] "+helloCommand()) {
					foundSandboxToolResult = true
				}
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
	server := NewServer(log.New(io.Discard, "", 0), sessionManager, nil)
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
	server := NewServer(log.New(io.Discard, "", 0), sessionManager, nil)
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

func helloCommand() string {
	if runtime.GOOS == "windows" {
		return "Write-Output hello"
	}
	return "printf hello"
}
