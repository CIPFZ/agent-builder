package tui

import (
	"context"
	"log"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"myclaw/internal/gateway"
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

	client := NewMyclawdClient(context.Background(), httpServer.URL+"/ws", "main", newClientStore(), nil)
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
	sessions := session.NewManager(nil)
	server := gateway.NewServerWithOptions(log.Default(), sessions, nil, gateway.Options{})
	mux := http.NewServeMux()
	mux.HandleFunc("/ws", server.HandleWebSocket)
	httpServer := httptest.NewServer(mux)
	defer httpServer.Close()

	client := NewMyclawdClient(context.Background(), httpServer.URL+"/ws", "main", newClientStore(), nil)
	msgCh := make(chan RuntimeEventMsg, 8)
	client.Attach(func(msg tea.Msg) {
		if event, ok := msg.(RuntimeEventMsg); ok {
			msgCh <- event
		}
	})
	if err := client.Start(); err != nil {
		t.Fatalf("Start error: %v", err)
	}
	defer client.Close()

	if err := client.SendUserMessage("hello from tui"); err != nil {
		t.Fatalf("SendUserMessage error: %v", err)
	}

	deadline := time.After(3 * time.Second)
	for {
		select {
		case msg := <-msgCh:
			if msg.Event.Type == "message.created" && msg.Event.Message != nil && msg.Event.Message.Role == "user" {
				if msg.Event.Message.Content != "hello from tui" {
					t.Fatalf("user content = %q", msg.Event.Message.Content)
				}
				return
			}
		case <-deadline:
			t.Fatal("timed out waiting for user message event")
		}
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
