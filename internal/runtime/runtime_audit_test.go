package runtime

import (
	"context"
	"encoding/json"
	"strings"
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
		ID:           "audit-1",
		SessionID:    "session-1",
		TurnID:       "turn-1",
		ToolCallID:   "tool-1",
		PermissionID: "perm-1",
		Type:         "started",
		CreatedAt:    "2026-05-18T00:00:00Z",
		Payload:      map[string]any{"model": "test-model"},
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
	if resp.Events[0].ToolCallID != "tool-1" || resp.Events[0].PermissionID != "perm-1" {
		t.Fatalf("linkage = %#v", resp.Events[0])
	}

	resp, err = store.ListSession(context.Background(), "session-1")
	if err != nil {
		t.Fatal(err)
	}
	if len(resp.Events) != 1 {
		t.Fatalf("session events = %#v", resp.Events)
	}
}

func TestAuditPayloadRedactsSecrets(t *testing.T) {
	t.Parallel()

	payload, err := auditPayload(auditEntry{
		RequestID:     "turn-1",
		Event:         "started",
		PromptPreview: `use api_key=sk-secret and Authorization: Bearer token`,
		MCPServers: []RuntimeMCPServer{
			{
				Name:    "docs",
				Type:    "http",
				URL:     "https://user:password@example.com/mcp?token=secret",
				Headers: map[string]string{"Authorization": "Bearer secret", "X-Team": "docs"},
				Env:     map[string]string{"API_TOKEN": "secret", "MODE": "test"},
			},
		},
		ToolCalls: []auditToolCall{
			{ID: "tool-1", Name: "bash", Input: `{"api_key":"sk-secret"}`, Output: "ok"},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	data, err := json.Marshal(payload)
	if err != nil {
		t.Fatal(err)
	}
	text := string(data)
	for _, leaked := range []string{"sk-secret", "Bearer secret", "API_TOKEN\":\"secret", "user:password", "token=secret"} {
		if strings.Contains(text, leaked) {
			t.Fatalf("audit payload leaked %q: %s", leaked, text)
		}
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

func TestAuditPayloadIncludesSkillActivationSummary(t *testing.T) {
	t.Parallel()

	summary := runtimeTurnSkillSummary([]RuntimeSkill{
		{Name: "docs", Builtin: true, Enabled: true, State: capabilityStateUnloaded, Path: "crush://skills/docs", SkillFilePath: "crush://skills/docs/SKILL.md", CapabilityID: "skill:docs"},
		{Name: "disabled", Enabled: false, State: capabilityStateDisabled, Reason: "disabled_skill"},
		{Name: "broken", Enabled: false, State: capabilityStateFailed, Error: "parse failed"},
	}, "ask")
	payload, err := auditPayload(auditEntry{
		RequestID:    "turn-1",
		Event:        "started",
		SkillSummary: &summary,
	})
	if err != nil {
		t.Fatal(err)
	}
	parsed := runtimeTurnSkillSummaryFromPayload(payload["skill_summary"].(map[string]any))
	if parsed.AvailableCount != 3 || len(parsed.Activated) != 1 || parsed.Activated[0].Name != "docs" {
		t.Fatalf("activated skill summary missing: %#v", parsed)
	}
	if len(parsed.Excluded) != 1 || parsed.Excluded[0].Name != "disabled" {
		t.Fatalf("excluded skill summary missing: %#v", parsed)
	}
	if len(parsed.Failed) != 1 || parsed.Failed[0].Name != "broken" {
		t.Fatalf("failed skill summary missing: %#v", parsed)
	}
}

func TestAuditPayloadRedactsCapabilityLoadError(t *testing.T) {
	t.Parallel()

	payload, err := auditPayload(auditEntry{
		Event:            "capability_failed",
		CapabilityID:     "mcp:docs:search",
		CapabilityKind:   "mcp_tool",
		CapabilitySource: "docs",
		CapabilityState:  "failed",
		CapabilityReason: "refresh_failed",
		CapabilityError:  "Authorization: Bearer secret-token",
	})
	if err != nil {
		t.Fatal(err)
	}
	data, err := json.Marshal(payload)
	if err != nil {
		t.Fatal(err)
	}
	text := string(data)
	if strings.Contains(text, "secret-token") || strings.Contains(text, "Bearer secret") {
		t.Fatalf("capability audit leaked secret: %s", text)
	}
	if !strings.Contains(text, "capability_failed") || !strings.Contains(text, "mcp:docs:search") {
		t.Fatalf("capability audit fields missing: %s", text)
	}
}

func TestAuditPayloadIncludesMCPDecisionWithoutSecrets(t *testing.T) {
	t.Parallel()

	payload, err := auditPayload(auditEntry{
		Event:        "mcp_server_refresh_failed",
		CapabilityID: "mcp_server:docs",
		MCPServer:    "docs",
		MCPKind:      "server",
		MCPStatus:    "failed",
		MCPDecision:  "deny",
		MCPRisk:      "network",
		MCPReason:    "Authorization: Bearer secret-token",
	})
	if err != nil {
		t.Fatal(err)
	}
	data, err := json.Marshal(payload)
	if err != nil {
		t.Fatal(err)
	}
	text := string(data)
	if strings.Contains(text, "secret-token") || strings.Contains(text, "Bearer secret") {
		t.Fatalf("mcp audit leaked secret: %s", text)
	}
	for _, want := range []string{"mcp_server_refresh_failed", "mcp_server:docs", "network", "deny"} {
		if !strings.Contains(text, want) {
			t.Fatalf("mcp audit missing %q: %s", want, text)
		}
	}
}
