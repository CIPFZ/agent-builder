package main

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/charmbracelet/crush/internal/db"
)

func TestRuntimeAuditStoreAppendAndListTurn(t *testing.T) {
	t.Parallel()

	dataDir := t.TempDir()
	conn, err := db.Connect(context.Background(), dataDir)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_ = db.Release(dataDir)
	})

	store := newRuntimeAuditStore(conn)
	err = store.Append(context.Background(), RuntimeAuditEvent{
		ID:        "audit-1",
		SessionID: "session-1",
		TurnID:    "turn-1",
		Type:      "started",
		CreatedAt: "2026-05-18T00:00:00Z",
		Payload:   map[string]any{"model": "test-model"},
	})
	if err != nil {
		t.Fatal(err)
	}

	resp, err := store.ListTurn(context.Background(), "turn-1")
	if err != nil {
		t.Fatal(err)
	}
	if len(resp.Events) != 1 {
		t.Fatalf("events = %#v", resp.Events)
	}
	if resp.Events[0].Payload["model"] != "test-model" {
		t.Fatalf("payload = %#v", resp.Events[0].Payload)
	}

	resp, err = store.ListSession(context.Background(), "session-1")
	if err != nil {
		t.Fatal(err)
	}
	if len(resp.Events) != 1 {
		t.Fatalf("session events = %#v", resp.Events)
	}
}

func TestAuditPayloadIncludesRuntimeCapabilitySnapshot(t *testing.T) {
	t.Parallel()

	payload, err := auditPayload(auditEntry{
		RequestID: "turn-1",
		Event:     "started",
		Skills: []RuntimeSkill{
			{Name: "skill-creator", Builtin: true, Enabled: true, State: "normal"},
		},
		MCPServers: []RuntimeMCPServer{
			{Name: "docs", Type: "http", URL: "[REDACTED_URL]", State: "connected"},
		},
		MCPTools: []RuntimeMCPTool{
			{Server: "docs", Name: "search", Enabled: true},
		},
		ToolCalls: []auditToolCall{
			{ID: "tool-1", Name: "bash", Input: "pwd", Output: "C:/work"},
		},
	})
	if err != nil {
		t.Fatal(err)
	}

	data, err := json.Marshal(payload)
	if err != nil {
		t.Fatal(err)
	}
	var decoded struct {
		Skills     []RuntimeSkill     `json:"skills"`
		MCPServers []RuntimeMCPServer `json:"mcp_servers"`
		MCPTools   []RuntimeMCPTool   `json:"mcp_tools"`
		ToolCalls  []auditToolCall    `json:"tool_calls"`
	}
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatal(err)
	}
	if len(decoded.Skills) != 1 || decoded.Skills[0].Name != "skill-creator" {
		t.Fatalf("skills missing from payload: %#v", decoded.Skills)
	}
	if len(decoded.MCPServers) != 1 || decoded.MCPServers[0].URL != "[REDACTED_URL]" {
		t.Fatalf("mcp servers missing/redaction changed: %#v", decoded.MCPServers)
	}
	if len(decoded.MCPTools) != 1 || decoded.MCPTools[0].Name != "search" {
		t.Fatalf("mcp tools missing from payload: %#v", decoded.MCPTools)
	}
	if len(decoded.ToolCalls) != 1 || decoded.ToolCalls[0].Output != "C:/work" {
		t.Fatalf("tool calls missing from payload: %#v", decoded.ToolCalls)
	}
}
