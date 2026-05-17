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
	if strings.TrimSpace(resp.Content) == "" {
		t.Fatalf("empty response from provider %q model %q", resp.Provider, resp.Model)
	}
	t.Logf("provider=%s model=%s content=%q", resp.Provider, resp.Model, resp.Content)
}
