package runtime

import (
	"context"
	"encoding/json"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/charmbracelet/crush/internal/config"
	"github.com/charmbracelet/crush/internal/csync"
	"github.com/charmbracelet/crush/internal/db"
	"github.com/charmbracelet/crush/internal/message"
	"github.com/charmbracelet/crush/internal/permission"
	"github.com/charmbracelet/crush/internal/proto"
	"github.com/charmbracelet/crush/internal/runtimeapi"
)

func TestRuntimeMCPRequestStoreAuthCreateGetListUpdatePersistence(t *testing.T) {
	t.Parallel()

	dataDir := t.TempDir()
	conn, err := db.Connect(context.Background(), dataDir)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Release(dataDir) })

	store := newRuntimeMCPRequestStore(conn)
	auth, err := store.Upsert(context.Background(), RuntimeMCPRequest{
		ID:             "mcp-req-1",
		Kind:           mcpRequestKindAuth,
		Server:         "docs",
		CapabilityID:   "mcp_server:docs",
		SessionID:      "session-1",
		TurnID:         "turn-1",
		Status:         mcpRequestStatusPending,
		Description:    "Authorization: Bearer secret token",
		PolicyMode:     "ask",
		PolicyDecision: "ask",
		PolicyRisk:     "secret",
		CreatedAt:      1000,
		UpdatedAt:      1000,
	})
	if err != nil {
		t.Fatal(err)
	}
	if auth.ID != "mcp-req-1" || auth.Kind != mcpRequestKindAuth || auth.Status != mcpRequestStatusPending {
		t.Fatalf("auth = %#v", auth)
	}
	if strings.Contains(strings.ToLower(auth.Description), "secret") || !auth.Redacted {
		t.Fatalf("auth request was not redacted: %#v", auth)
	}

	got, err := store.Get(context.Background(), "mcp-req-1")
	if err != nil {
		t.Fatal(err)
	}
	if got.Server != "docs" || got.PolicyDecision != "ask" {
		t.Fatalf("got = %#v", got)
	}

	pending, err := store.List(context.Background(), RuntimeMCPRequestListRequest{Kind: mcpRequestKindAuth, Status: mcpRequestStatusPending})
	if err != nil {
		t.Fatal(err)
	}
	if len(pending) != 1 || pending[0].ID != "mcp-req-1" {
		t.Fatalf("pending = %#v", pending)
	}

	completed, err := store.Mark(context.Background(), "mcp-req-1", mcpRequestStatusCompleted, "token accepted", "")
	if err != nil {
		t.Fatal(err)
	}
	if completed.Status != mcpRequestStatusCompleted || completed.CompletedAt == 0 || completed.ResponseSummary == "" {
		t.Fatalf("completed = %#v", completed)
	}
}

func TestRuntimeMCPRequestStoreElicitationCreateGetListUpdatePersistence(t *testing.T) {
	t.Parallel()

	dataDir := t.TempDir()
	conn, err := db.Connect(context.Background(), dataDir)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Release(dataDir) })

	store := newRuntimeMCPRequestStore(conn)
	elicitation, err := store.Upsert(context.Background(), RuntimeMCPRequest{
		ID:          "mcp-ask-1",
		Kind:        mcpRequestKindElicitation,
		Server:      "docs",
		Status:      mcpRequestStatusPending,
		Prompt:      "Choose docs project",
		Description: "MCP server needs user input.",
		CreatedAt:   1000,
		UpdatedAt:   1000,
	})
	if err != nil {
		t.Fatal(err)
	}
	if elicitation.Kind != mcpRequestKindElicitation || elicitation.Prompt == "" {
		t.Fatalf("elicitation = %#v", elicitation)
	}
	answered, err := store.Mark(context.Background(), "mcp-ask-1", mcpRequestStatusCompleted, "selected public docs", "")
	if err != nil {
		t.Fatal(err)
	}
	if answered.Status != mcpRequestStatusCompleted || answered.ResponseSummary != "selected public docs" {
		t.Fatalf("answered = %#v", answered)
	}
}

func TestRuntimeMCPRequestsAppearInRecoveryAndPersistAcrossRestart(t *testing.T) {
	t.Cleanup(db.ResetPool)
	dataDir := t.TempDir()
	conn, err := db.Connect(context.Background(), dataDir)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Release(dataDir) })

	service := newRuntimeService()
	runtimeBackend, workspace := backendForSkillTest(t)
	service.runtime = runtimeBackend
	service.workspace = &proto.Workspace{ID: workspace.ID, Path: workspace.Path}
	service.turns = newRuntimeTurnStore(conn)
	service.permissionStore = newRuntimePermissionStore(conn)
	service.mcpRequestStore = newRuntimeMCPRequestStore(conn)
	if _, err := service.mcpRequestStore.Upsert(context.Background(), RuntimeMCPRequest{
		ID:        "mcp-req-1",
		Kind:      mcpRequestKindAuth,
		Server:    "docs",
		Status:    mcpRequestStatusPending,
		CreatedAt: 1000,
		UpdatedAt: 1000,
	}); err != nil {
		t.Fatal(err)
	}

	recovery, err := service.RecoveryStatus(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(recovery.PendingMCPRequests) != 1 || recovery.PendingMCPRequests[0].ID != "mcp-req-1" {
		t.Fatalf("pending mcp requests = %#v", recovery.PendingMCPRequests)
	}

	restarted := newRuntimeMCPRequestStore(conn)
	got, err := restarted.Get(context.Background(), "mcp-req-1")
	if err != nil {
		t.Fatal(err)
	}
	if got.Status != mcpRequestStatusPending {
		t.Fatalf("restarted got = %#v", got)
	}
}

func TestRuntimeMCPStartupCancelsStaleActionableAuthAndElicitationRequests(t *testing.T) {
	t.Cleanup(db.ResetPool)
	dataDir := t.TempDir()
	conn, err := db.Connect(context.Background(), dataDir)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Release(dataDir) })

	service := newRuntimeService()
	runtimeBackend, workspace := backendForSkillTest(t)
	service.runtime = runtimeBackend
	service.workspace = &proto.Workspace{ID: workspace.ID, Path: workspace.Path}
	service.turns = newRuntimeTurnStore(conn)
	service.permissionStore = newRuntimePermissionStore(conn)
	service.mcpRequestStore = newRuntimeMCPRequestStore(conn)
	service.eventStore = newRuntimeEventStore(conn)
	service.policy = defaultRuntimePolicy()
	service.nextEventSequence = 0

	sess, err := runtimeBackend.CreateSession(context.Background(), workspace.ID, "mcp startup recovery")
	if err != nil {
		t.Fatal(err)
	}
	ws, err := runtimeBackend.GetWorkspace(workspace.ID)
	if err != nil {
		t.Fatal(err)
	}
	userMessage, err := ws.Messages.Create(context.Background(), sess.ID, message.CreateMessageParams{
		Role:  message.User,
		Parts: []message.ContentPart{message.TextContent{Text: "trigger hosted MCP auth and elicitation"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	turnID := "turn-mcp-startup"
	if _, err := service.turns.Upsert(context.Background(), RuntimeTurn{
		ID:            turnID,
		SessionID:     sess.ID,
		Status:        turnStatusInterrupted,
		UserMessageID: userMessage.ID,
		PromptPreview: "trigger hosted MCP auth and elicitation",
		StartedAt:     time.Now().Add(-time.Second).UnixMilli(),
		FinishedAt:    time.Now().UnixMilli(),
	}); err != nil {
		t.Fatal(err)
	}
	for _, req := range []RuntimeMCPRequest{
		{
			ID:             "mcp-auth-stale",
			Kind:           mcpRequestKindAuth,
			Server:         "hosted-docs",
			CapabilityID:   "mcp_server:hosted-docs",
			SessionID:      sess.ID,
			TurnID:         turnID,
			Status:         mcpRequestStatusPending,
			Description:    "Authorization: Bearer sk-secret",
			PolicyDecision: "ask",
			PolicyMode:     string(permission.PolicyModeAutoRead),
			PolicyRisk:     string(permission.RiskSecret),
			CreatedAt:      1000,
			UpdatedAt:      1000,
		},
		{
			ID:             "mcp-elicitation-stale",
			Kind:           mcpRequestKindElicitation,
			Server:         "hosted-docs",
			CapabilityID:   "mcp:hosted-docs:select_project",
			SessionID:      sess.ID,
			TurnID:         turnID,
			Status:         mcpRequestStatusRequired,
			Prompt:         "Choose project",
			Description:    "Authorization: Bearer sk-secret",
			PolicyDecision: "ask",
			PolicyMode:     string(permission.PolicyModeAutoRead),
			PolicyRisk:     string(permission.RiskSecret),
			CreatedAt:      1001,
			UpdatedAt:      1001,
		},
	} {
		if _, err := service.mcpRequestStore.Upsert(context.Background(), req); err != nil {
			t.Fatal(err)
		}
	}

	cancelled, err := service.mcpRequestStore.CancelActionableOnStartup(context.Background(), "runtime restarted; stale MCP request is no longer actionable")
	if err != nil {
		t.Fatal(err)
	}
	if len(cancelled) != 2 {
		t.Fatalf("cancelled requests = %#v", cancelled)
	}
	for _, req := range cancelled {
		service.publishMCPRequestLifecycle(req)
		service.writeMCPRequestAudit(req)
		if req.Status != mcpRequestStatusCancelled || req.CompletedAt == 0 {
			t.Fatalf("request was not terminal after startup recovery: %#v", req)
		}
		if strings.Contains(strings.ToLower(req.Description+req.Error), "sk-secret") {
			t.Fatalf("cancelled request leaked secret: %#v", req)
		}
	}

	recovery, err := service.RecoveryStatus(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(recovery.PendingMCPRequests) != 0 {
		t.Fatalf("stale MCP requests remained actionable after startup recovery: %#v", recovery.PendingMCPRequests)
	}
	resp, err := service.DecideMCPRequest(context.Background(), RuntimeMCPRequestDecision{RequestID: "mcp-auth-stale", Action: "approve"})
	if err != nil {
		t.Fatal(err)
	}
	if resp.Request.Status != mcpRequestStatusCancelled {
		t.Fatalf("terminal cancelled request became actionable again: %#v", resp.Request)
	}

	replay, err := service.ReplayExport(context.Background(), RuntimeReplayExportRequest{SessionID: sess.ID, TurnID: turnID})
	if err != nil {
		t.Fatal(err)
	}
	if replay.Summary.Recovery.PendingMCPRequests != 0 {
		t.Fatalf("replay still reports pending MCP actionability: %#v", replay.Summary.Recovery)
	}
	for _, id := range []string{"mcp-auth-stale", "mcp-elicitation-stale"} {
		if !slices.ContainsFunc(replay.Summary.MCPRequests, func(item RuntimeReplayMCPRequest) bool {
			return item.RequestID == id && item.Status == mcpRequestStatusCancelled && item.Redacted
		}) {
			t.Fatalf("cancelled MCP request missing from replay: id=%s replay=%#v", id, replay.Summary.MCPRequests)
		}
	}

	turnActivity, err := service.TurnActivity(context.Background(), turnID)
	if err != nil {
		t.Fatal(err)
	}
	if len(turnActivity.Turns) != 1 || turnActivity.Turns[0].ID != turnID {
		t.Fatalf("turn activity = %#v", turnActivity.Turns)
	}
	if !slices.ContainsFunc(turnActivity.Events, func(event RuntimeEvent) bool {
		return event.Type == runtimeapi.EventMCPAuthDenied && stringFromMap(event.Payload, "status") == mcpRequestStatusCancelled
	}) || !slices.ContainsFunc(turnActivity.Events, func(event RuntimeEvent) bool {
		return event.Type == runtimeapi.EventMCPElicitationDenied && stringFromMap(event.Payload, "status") == mcpRequestStatusCancelled
	}) {
		t.Fatalf("terminal MCP events missing from narrow activity: %#v", turnActivity.Events)
	}
}

func TestRuntimeMCPRequestDecisionsEmitAuditEventsAndReplay(t *testing.T) {
	t.Cleanup(db.ResetPool)
	dataDir := t.TempDir()
	conn, err := db.Connect(context.Background(), dataDir)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Release(dataDir) })

	service := newRuntimeService()
	runtimeBackend, workspace := backendForSkillTest(t)
	service.runtime = runtimeBackend
	service.workspace = &proto.Workspace{ID: workspace.ID, Path: workspace.Path}
	service.sessionID = "session-1"
	service.turns = newRuntimeTurnStore(conn)
	service.eventStore = newRuntimeEventStore(conn)
	service.permissionStore = newRuntimePermissionStore(conn)
	service.mcpRequestStore = newRuntimeMCPRequestStore(conn)
	service.nextEventSequence = 0
	if _, err := service.mcpRequestStore.Upsert(context.Background(), RuntimeMCPRequest{
		ID:             "mcp-req-1",
		Kind:           mcpRequestKindAuth,
		Server:         "docs",
		CapabilityID:   "mcp_server:docs",
		SessionID:      "session-1",
		TurnID:         "turn-1",
		Status:         mcpRequestStatusPending,
		Description:    "auth needed",
		PolicyDecision: "ask",
		PolicyMode:     "ask",
		CreatedAt:      1000,
		UpdatedAt:      1000,
	}); err != nil {
		t.Fatal(err)
	}

	resp, err := service.DecideMCPRequest(context.Background(), RuntimeMCPRequestDecision{RequestID: "mcp-req-1", Action: "approve", ResponseSummary: "approved"})
	if err != nil {
		t.Fatal(err)
	}
	if resp.Request.Status != mcpRequestStatusCompleted {
		t.Fatalf("decision response = %#v", resp)
	}
	events, err := service.Events(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if !containsRuntimeEvent(events.Events, runtimeapi.EventMCPAuthCompleted) {
		t.Fatalf("events = %#v", events.Events)
	}
	replay, err := service.ReplayExport(context.Background(), RuntimeReplayExportRequest{SessionID: "session-1"})
	if err != nil {
		t.Fatal(err)
	}
	if len(replay.Summary.MCPRequests) != 1 || replay.Summary.MCPRequests[0].Status != mcpRequestStatusCompleted {
		t.Fatalf("replay mcp requests = %#v", replay.Summary.MCPRequests)
	}
}

func TestRuntimeMCPRequestEventAuditReplayRedactsSecrets(t *testing.T) {
	t.Cleanup(db.ResetPool)
	dataDir := t.TempDir()
	conn, err := db.Connect(context.Background(), dataDir)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Release(dataDir) })

	service := newRuntimeService()
	runtimeBackend, workspace := backendForSkillTest(t)
	service.runtime = runtimeBackend
	service.workspace = &proto.Workspace{ID: workspace.ID, Path: workspace.Path}
	service.sessionID = "session-secret"
	service.turns = newRuntimeTurnStore(conn)
	service.eventStore = newRuntimeEventStore(conn)
	service.permissionStore = newRuntimePermissionStore(conn)
	service.mcpRequestStore = newRuntimeMCPRequestStore(conn)
	req, err := service.createMCPAuthRequest(context.Background(), "docs", "mcp_server:docs", "Authorization: Bearer sk-secret password=abc", permission.PolicyResult{
		Decision: permission.PolicyAsk,
		Risk:     permission.RiskSecret,
		Mode:     permission.PolicyModeAsk,
		Reason:   "token=sk-secret",
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := service.DecideMCPRequest(context.Background(), RuntimeMCPRequestDecision{RequestID: req.ID, Action: "fail", Error: "Authorization: Bearer sk-secret"}); err != nil {
		t.Fatal(err)
	}
	replay, err := service.ReplayExport(context.Background(), RuntimeReplayExportRequest{SessionID: "session-secret"})
	if err != nil {
		t.Fatal(err)
	}
	data, err := json.Marshal(replay)
	if err != nil {
		t.Fatal(err)
	}
	text := strings.ToLower(string(data))
	for _, leaked := range []string{"sk-secret", "bearer sk", "password=abc", "token=sk"} {
		if strings.Contains(text, leaked) {
			t.Fatalf("mcp request replay leaked %q: %s", leaked, data)
		}
	}
}

func TestRuntimeMCPRequestDeniedCancelledFailedPaths(t *testing.T) {
	t.Cleanup(db.ResetPool)
	dataDir := t.TempDir()
	conn, err := db.Connect(context.Background(), dataDir)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Release(dataDir) })

	service := newRuntimeService()
	runtimeBackend, workspace := backendForSkillTest(t)
	service.runtime = runtimeBackend
	service.workspace = &proto.Workspace{ID: workspace.ID, Path: workspace.Path}
	service.mcpRequestStore = newRuntimeMCPRequestStore(conn)
	service.eventStore = newRuntimeEventStore(conn)
	for _, tc := range []struct {
		id     string
		kind   string
		action string
		status string
	}{
		{"auth-deny", mcpRequestKindAuth, "deny", mcpRequestStatusDenied},
		{"ask-cancel", mcpRequestKindElicitation, "cancel", mcpRequestStatusCancelled},
		{"auth-fail", mcpRequestKindAuth, "fail", mcpRequestStatusFailed},
	} {
		if _, err := service.mcpRequestStore.Upsert(context.Background(), RuntimeMCPRequest{ID: tc.id, Kind: tc.kind, Server: "docs", Status: mcpRequestStatusPending, CreatedAt: 1000, UpdatedAt: 1000}); err != nil {
			t.Fatal(err)
		}
		resp, err := service.DecideMCPRequest(context.Background(), RuntimeMCPRequestDecision{RequestID: tc.id, Action: tc.action, Error: "Authorization: Bearer token-secret"})
		if err != nil {
			t.Fatal(err)
		}
		if resp.Request.Status != tc.status {
			t.Fatalf("%s status = %#v", tc.id, resp.Request)
		}
		if strings.Contains(strings.ToLower(resp.Request.Error), "secret") {
			t.Fatalf("error leaked secret: %#v", resp.Request)
		}
	}
}

func TestRuntimeMCPPolicyAskCreatesPendingAuthAndHeadlessFailsClosed(t *testing.T) {
	t.Cleanup(db.ResetPool)
	dataDir := t.TempDir()
	conn, err := db.Connect(context.Background(), dataDir)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Release(dataDir) })

	service := newRuntimeService()
	runtimeBackend, workspace := backendForSkillTest(t)
	service.runtime = runtimeBackend
	service.workspace = &proto.Workspace{ID: workspace.ID, Path: workspace.Path}
	service.mcpRequestStore = newRuntimeMCPRequestStore(conn)
	service.eventStore = newRuntimeEventStore(conn)
	decision := permission.PolicyResult{Decision: permission.PolicyAsk, Risk: permission.RiskSecret, Mode: permission.PolicyModeAsk, Reason: "needs auth"}
	req, err := service.createMCPAuthRequest(context.Background(), "docs", "mcp_server:docs", "auth needed", decision)
	if err != nil {
		t.Fatal(err)
	}
	if req.Status != mcpRequestStatusPending {
		t.Fatalf("request = %#v", req)
	}

	headless := decision
	headless.Headless = true
	headless.HeadlessReason = "headless runtime cannot ask"
	blocked, err := service.createMCPElicitationRequest(context.Background(), "docs", "mcp_server:docs", "choose project", "input needed", headless)
	if err != nil {
		t.Fatal(err)
	}
	if blocked.Status != mcpRequestStatusFailed || !blocked.PolicyHeadless {
		t.Fatalf("headless request = %#v", blocked)
	}
}

func TestRuntimeMCPPolicyDeniedAndDisabledDoNotCreateAuthRequest(t *testing.T) {
	t.Parallel()

	dataDir := t.TempDir()
	conn, err := db.Connect(context.Background(), dataDir)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Release(dataDir) })

	service := newRuntimeService()
	service.mcpRequestStore = newRuntimeMCPRequestStore(conn)
	service.eventStore = newRuntimeEventStore(conn)
	service.policy = runtimePolicyFromMode(permission.PolicyModeDenyAll, 0)
	store := config.NewTestStore(&config.Config{
		Providers: csync.NewMap[string, config.ProviderConfig](),
		Models:    map[config.SelectedModelType]config.SelectedModel{},
		Options:   &config.Options{},
		MCP:       config.MCPs{"docs": {Type: config.MCPHttp, URL: "https://example.com/mcp"}},
	})
	if err := service.refreshMCPServerLifecycle(context.Background(), store, "workspace", "docs", "test"); err != nil {
		t.Fatal(err)
	}
	requests, err := service.mcpRequestStore.List(context.Background(), RuntimeMCPRequestListRequest{})
	if err != nil {
		t.Fatal(err)
	}
	if len(requests) != 0 {
		t.Fatalf("policy denied created requests: %#v", requests)
	}

	service.policy = defaultRuntimePolicy()
	store.Config().MCP["disabled"] = config.MCPConfig{Type: config.MCPHttp, URL: "https://example.com/mcp", Disabled: true}
	if err := service.refreshMCPServerLifecycle(context.Background(), store, "workspace", "disabled", "test"); err != nil {
		t.Fatal(err)
	}
	requests, err = service.mcpRequestStore.List(context.Background(), RuntimeMCPRequestListRequest{})
	if err != nil {
		t.Fatal(err)
	}
	if len(requests) != 0 {
		t.Fatalf("disabled created requests: %#v", requests)
	}
}

func TestRuntimeMCPDeniedRequestBlocksServerRefreshAndCapabilityLoad(t *testing.T) {
	t.Parallel()

	dataDir := t.TempDir()
	conn, err := db.Connect(context.Background(), dataDir)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Release(dataDir) })

	service := newRuntimeService()
	service.mcpRequestStore = newRuntimeMCPRequestStore(conn)
	service.eventStore = newRuntimeEventStore(conn)
	service.policy = defaultRuntimePolicy()
	store := config.NewTestStore(&config.Config{
		Providers: csync.NewMap[string, config.ProviderConfig](),
		Models:    map[config.SelectedModelType]config.SelectedModel{},
		Options:   &config.Options{},
		MCP:       config.MCPs{"docs": {Type: config.MCPHttp, URL: "https://example.com/mcp"}},
	})
	if _, err := service.mcpRequestStore.Upsert(context.Background(), RuntimeMCPRequest{
		ID:        "mcp-denied-1",
		Kind:      mcpRequestKindAuth,
		Server:    "docs",
		Status:    mcpRequestStatusDenied,
		CreatedAt: 1000,
		UpdatedAt: 1000,
	}); err != nil {
		t.Fatal(err)
	}
	if err := service.refreshMCPServerLifecycle(context.Background(), store, "workspace", "docs", "test"); err != nil {
		t.Fatal(err)
	}
	events, err := service.Events(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if !containsRuntimeEvent(events.Events, runtimeapi.EventMCPServerBlocked) {
		t.Fatalf("events = %#v", events.Events)
	}
	records := cloneCapabilityLoadRecords(service.capabilityLoads)
	if records["mcp_server:docs"].State != capabilityStateFailed || !strings.Contains(records["mcp_server:docs"].Diagnostics, "mcp-denied-1") {
		t.Fatalf("capability records = %#v", records)
	}
}

func containsRuntimeEvent(events []RuntimeEvent, eventType string) bool {
	for _, event := range events {
		if event.Type == eventType {
			return true
		}
	}
	return false
}
