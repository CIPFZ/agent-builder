package tui

import "testing"

func TestParseClientEventAssistantDelta(t *testing.T) {
	event, ok := parseClientEventMessage(wsMessageLike{
		Type:  "event",
		Event: "assistant.delta",
		Payload: map[string]any{
			"delta":      "hello",
			"session_id": "session-1",
		},
	})

	if !ok {
		t.Fatal("parse = false, want true")
	}
	if event.Message == nil || event.Message.Content != "hello" {
		t.Fatalf("message = %#v, want assistant delta hello", event.Message)
	}
	if event.Tool == nil || event.Tool.ProgressMessage != "hello" {
		t.Fatalf("tool = %#v, want progress message hello", event.Tool)
	}
}
