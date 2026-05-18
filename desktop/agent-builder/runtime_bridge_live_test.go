//go:build desktop_live

package main

import (
	"context"
	"os"
	"path/filepath"
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

func TestDesktopRuntimeBridgeLiveToolPermission(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()

	root := desktopRootForTest(t)
	generated := filepath.Join(root, "mergesort.c")
	t.Cleanup(func() {
		_ = os.Remove(generated)
	})

	bridge := NewRuntimeBridge()
	resp, err := bridge.Chat(ctx, RuntimeChatRequest{
		Prompt: "使用C语言写一个归并排序",
	})
	if err != nil {
		t.Fatal(err)
	}
	if strings.TrimSpace(resp.RequestID) == "" {
		t.Fatal("empty runtime request id")
	}

	var latest RuntimeMessage
	var status RuntimeStatus
	for deadline := time.Now().Add(2 * time.Minute); time.Now().Before(deadline); {
		messages, err := bridge.Messages(ctx)
		if err != nil {
			t.Fatal(err)
		}
		for _, msg := range messages.Messages {
			if msg.Role == "assistant" {
				latest = msg
			}
		}
		status, err = bridge.Status(ctx)
		if err != nil {
			t.Fatal(err)
		}
		if strings.TrimSpace(latest.Content) != "" && latest.Finished {
			break
		}
		time.Sleep(500 * time.Millisecond)
	}
	if strings.TrimSpace(latest.Content) == "" || !latest.Finished {
		t.Fatalf("assistant did not finish after tool permission flow: status=%#v latest=%#v", status, latest)
	}
	if status.Events.PermissionEvents == 0 {
		t.Fatalf("expected permission event in tool flow: %#v", status.Events)
	}
	t.Logf("request_id=%s finish_reason=%s content_len=%d permission_events=%d", resp.RequestID, latest.FinishReason, len(latest.Content), status.Events.PermissionEvents)
}

func desktopRootForTest(t *testing.T) string {
	t.Helper()
	root := strings.TrimSpace(os.Getenv("AGENT_BUILDER_DESKTOP_ROOT"))
	if root != "" {
		return root
	}
	workingDir, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	return filepath.Join(workingDir, "bin")
}
