//go:build desktop_live

package main

import (
	"context"
	"strings"
	"testing"
	"time"
)

func TestDesktopRuntimeBridgeLiveChat(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()

	bridge := NewRuntimeBridge()
	resp, err := bridge.Chat(ctx, RuntimeChatRequest{
		Prompt: "Reply only OK for a desktop runtime connectivity test.",
	})
	if err != nil {
		t.Fatal(err)
	}
	if strings.TrimSpace(resp.RequestID) == "" {
		t.Fatal("empty runtime request id")
	}
	var latest RuntimeMessage
	for deadline := time.Now().Add(3 * time.Minute); time.Now().Before(deadline); {
		messages, err := bridge.Messages(ctx)
		if err != nil {
			t.Fatal(err)
		}
		for _, msg := range messages.Messages {
			if msg.Role == "assistant" {
				latest = msg
			}
		}
		if strings.TrimSpace(latest.Content) != "" && latest.Finished {
			break
		}
		time.Sleep(250 * time.Millisecond)
	}
	if strings.TrimSpace(latest.Content) == "" {
		t.Fatalf("empty response from provider %q model %q", latest.Provider, latest.Model)
	}
	status, err := bridge.Status(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if status.Usage.TotalTokens == 0 {
		t.Fatal("runtime status did not expose session token usage")
	}
	if status.Events.MessageEvents == 0 || status.Events.AssistantEvents == 0 {
		t.Fatalf("runtime status did not expose Crush message events: %#v", status.Events)
	}
	t.Logf("request_id=%s provider=%s model=%s content=%q", resp.RequestID, latest.Provider, latest.Model, latest.Content)
}
