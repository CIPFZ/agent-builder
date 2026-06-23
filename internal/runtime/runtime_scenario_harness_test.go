package runtime

import (
	"context"
	"encoding/json"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/CIPFZ/agent-builder/internal/agent"
	"github.com/CIPFZ/agent-builder/internal/apitypes"
	"github.com/CIPFZ/agent-builder/internal/db"
	"github.com/CIPFZ/agent-builder/internal/permission"
	"github.com/CIPFZ/agent-builder/internal/runtimeapi"
	"github.com/CIPFZ/agent-builder/internal/tools/scheduler"
)

type runtimeScenarioHarness struct {
	t       *testing.T
	ctx     context.Context
	service *runtimeService
	connDir string
}

func newRuntimeScenarioHarness(t *testing.T) *runtimeScenarioHarness {
	t.Helper()
	dataDir := t.TempDir()
	conn, err := db.Connect(context.Background(), dataDir)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_ = db.Release(dataDir)
	})
	service := newRuntimeService()
	service.turns = newRuntimeTurnStore(conn)
	service.toolCalls = scheduler.New(NewRuntimeToolCallStoreForDB(conn))
	service.permissionStore = newRuntimePermissionStore(conn)
	service.compactBoundaries = newRuntimeCompactBoundaryStore(conn)
	service.worktrees = newRuntimeWorktreeStore(conn)
	service.agentTasks = newRuntimeAgentTaskStore(conn)
	service.hookExecutions = newRuntimeHookExecutionStore(conn)
	service.mcpRequestStore = newRuntimeMCPRequestStore(conn)
	service.refs = newRuntimeRefStore(conn, dataDir)
	service.eventStore = newRuntimeEventStore(conn)
	service.runs = newRuntimeRunStore(conn)
	service.transitions = newRuntimeRunTransitionStore(conn)
	service.policy = defaultRuntimePolicy()
	return &runtimeScenarioHarness{t: t, ctx: context.Background(), service: service, connDir: dataDir}
}

func (h *runtimeScenarioHarness) attachBackend() {
	h.t.Helper()
	runtimeWorkbench, workspace := workbenchForSkillTest(h.t)
	h.service.runtime = runtimeWorkbench
	h.service.workspace = &apitypes.Workspace{ID: workspace.ID, Path: workspace.Path}
}

func (h *runtimeScenarioHarness) restartedService() *runtimeService {
	h.t.Helper()
	restarted := newRuntimeService()
	restarted.turns = newRuntimeTurnStore(h.service.turns.db)
	restarted.toolCalls = scheduler.New(NewRuntimeToolCallStoreForDB(h.service.turns.db))
	restarted.permissionStore = newRuntimePermissionStore(h.service.turns.db)
	restarted.compactBoundaries = newRuntimeCompactBoundaryStore(h.service.turns.db)
	restarted.worktrees = newRuntimeWorktreeStore(h.service.turns.db)
	restarted.agentTasks = newRuntimeAgentTaskStore(h.service.turns.db)
	restarted.hookExecutions = newRuntimeHookExecutionStore(h.service.turns.db)
	restarted.mcpRequestStore = newRuntimeMCPRequestStore(h.service.turns.db)
	restarted.refs = newRuntimeRefStore(h.service.turns.db, h.connDir)
	restarted.eventStore = newRuntimeEventStore(h.service.turns.db)
	restarted.runs = newRuntimeRunStore(h.service.turns.db)
	restarted.transitions = newRuntimeRunTransitionStore(h.service.turns.db)
	restarted.policy = h.service.policy
	return restarted
}

func (h *runtimeScenarioHarness) seedTurn(sessionID, turnID string) {
	h.t.Helper()
	_, err := h.service.turns.Upsert(h.ctx, RuntimeTurn{
		ID:        turnID,
		SessionID: sessionID,
		Status:    turnStatusRunning,
		StartedAt: time.Now().UTC().UnixMilli(),
	})
	if err != nil {
		h.t.Fatal(err)
	}
}

func (h *runtimeScenarioHarness) evaluatePolicy(mode permission.PolicyMode, rules []RuntimePolicyRule, call agent.SchedulerToolCall) agent.SchedulerToolPolicyDecision {
	h.t.Helper()
	h.service.policy = runtimePolicyFromParts(mode, "scenario", rules, time.Now().UnixMilli())
	recorder := runtimeSchedulerRecorder{service: h.service}
	decision, err := recorder.EvaluateToolCall(h.ctx, call)
	if err != nil {
		h.t.Fatal(err)
	}
	return decision
}

func (h *runtimeScenarioHarness) replay(turnID string) RuntimeReplayExportResponse {
	h.t.Helper()
	resp, err := h.service.ReplayExport(h.ctx, RuntimeReplayExportRequest{TurnID: turnID})
	if err != nil {
		h.t.Fatal(err)
	}
	return resp
}

func (h *runtimeScenarioHarness) assertEventType(eventType string) {
	h.t.Helper()
	events, err := h.service.Events(h.ctx)
	if err != nil {
		h.t.Fatal(err)
	}
	if !slices.ContainsFunc(events.Events, func(event RuntimeEvent) bool { return event.Type == eventType }) {
		h.t.Fatalf("missing event %s in %#v", eventType, events.Events)
	}
}

func TestRuntimeScenarioHarnessPolicyToolShellGolden(t *testing.T) {
	scenarios := []struct {
		name   string
		run    func(*testing.T, *runtimeScenarioHarness)
		verify func(*testing.T, RuntimeReplayExportResponse)
	}{
		{
			name: "plan mode blocks write and shell tools",
			run: func(t *testing.T, h *runtimeScenarioHarness) {
				h.seedTurn("session-plan", "turn-plan")
				write := h.evaluatePolicy(permission.PolicyModePlan, nil, agent.SchedulerToolCall{
					ID: "tool-write", SessionID: "session-plan", TurnID: "turn-plan", Name: "write", Source: string(scheduler.ToolSourceBuiltin), CapabilityID: "builtin:write", InputSummary: `{"file_path":"notes.txt","content":"x"}`,
				})
				if write.Decision != string(permission.PolicyDeny) || write.Mode != string(permission.PolicyModePlan) {
					t.Fatalf("write decision = %#v", write)
				}
				shell := h.evaluatePolicy(permission.PolicyModePlan, nil, agent.SchedulerToolCall{
					ID: "tool-shell", SessionID: "session-plan", TurnID: "turn-plan", Name: "bash", Source: string(scheduler.ToolSourceShell), CapabilityID: "builtin:bash", InputSummary: `{"command":"go test ./..."}`,
				})
				if shell.Decision != string(permission.PolicyDeny) || shell.Risk != string(permission.RiskExecute) {
					t.Fatalf("shell decision = %#v", shell)
				}
			},
			verify: func(t *testing.T, replay RuntimeReplayExportResponse) {
				if !hasReplayPolicy(replay, "tool-write", permission.PolicyDeny, permission.PolicyModePlan) || !hasReplayPolicy(replay, "tool-shell", permission.PolicyDeny, permission.PolicyModePlan) {
					t.Fatalf("missing plan policy replay: %#v", replay.Summary.PolicyDecisions)
				}
			},
		},
		{
			name: "auto read allows read and asks non-read by scoped rules",
			run: func(t *testing.T, h *runtimeScenarioHarness) {
				h.seedTurn("session-auto", "turn-auto")
				read := h.evaluatePolicy(permission.PolicyModeAutoRead, nil, agent.SchedulerToolCall{
					ID: "tool-read", SessionID: "session-auto", TurnID: "turn-auto", Name: "view", Source: string(scheduler.ToolSourceBuiltin), CapabilityID: "builtin:view", InputSummary: `{"file_path":"README.md"}`,
				})
				if read.Decision != string(permission.PolicyAllow) {
					t.Fatalf("read decision = %#v", read)
				}
				ask := h.evaluatePolicy(permission.PolicyModeAutoRead, []RuntimePolicyRule{{
					ID: "ask-shell-go-test", Decision: string(permission.PolicyAsk), Source: "scenario", ShellPrefix: "go test", Reason: "Shell tests require review.",
				}}, agent.SchedulerToolCall{
					ID: "tool-ask", SessionID: "session-auto", TurnID: "turn-auto", Name: "bash", Source: string(scheduler.ToolSourceShell), CapabilityID: "builtin:bash", InputSummary: `{"command":"go test ./..."}`,
				})
				if ask.Decision != string(permission.PolicyDeny) || !strings.Contains(ask.Reason, "requires approval") || !ask.Headless {
					t.Fatalf("headless ask decision should fail closed without permission service: %#v", ask)
				}
				deny := h.evaluatePolicy(permission.PolicyModeAutoRead, []RuntimePolicyRule{{
					ID: "deny-write", Decision: string(permission.PolicyDeny), Source: "scenario", BuiltinTool: "write", Reason: "Scenario denies writes.",
				}}, agent.SchedulerToolCall{
					ID: "tool-deny", SessionID: "session-auto", TurnID: "turn-auto", Name: "write", Source: string(scheduler.ToolSourceBuiltin), CapabilityID: "builtin:write", InputSummary: `{"file_path":"x","content":"y"}`,
				})
				if deny.Decision != string(permission.PolicyDeny) || deny.RuleID != "deny-write" {
					t.Fatalf("deny decision = %#v", deny)
				}
			},
			verify: func(t *testing.T, replay RuntimeReplayExportResponse) {
				if !hasReplayPolicy(replay, "tool-read", permission.PolicyAllow, permission.PolicyModeAutoRead) || !hasReplayPolicy(replay, "tool-deny", permission.PolicyDeny, permission.PolicyModeAutoRead) {
					t.Fatalf("missing auto-read replay: %#v", replay.Summary.PolicyDecisions)
				}
				if !slices.ContainsFunc(replay.Summary.PolicyDecisions, func(item RuntimeReplayPolicyDecision) bool {
					return item.ToolCallID == "tool-ask" && item.Headless && item.HeadlessReason != "" && item.ShellReason != ""
				}) {
					t.Fatalf("missing headless ask replay: %#v", replay.Summary.PolicyDecisions)
				}
			},
		},
		{
			name: "shell destructive command classification and denial path",
			run: func(t *testing.T, h *runtimeScenarioHarness) {
				h.seedTurn("session-shell", "turn-shell")
				decision := h.evaluatePolicy(permission.PolicyModePlan, nil, agent.SchedulerToolCall{
					ID: "tool-rm", SessionID: "session-shell", TurnID: "turn-shell", Name: "bash", Source: string(scheduler.ToolSourceShell), CapabilityID: "builtin:bash", InputSummary: `{"command":"rm -rf build"}`,
				})
				if decision.Decision != string(permission.PolicyDeny) || decision.Risk != string(permission.RiskDestructive) || decision.ShellRisk != string(permission.RiskDestructive) {
					t.Fatalf("destructive shell decision = %#v", decision)
				}
			},
			verify: func(t *testing.T, replay RuntimeReplayExportResponse) {
				if !slices.ContainsFunc(replay.Summary.PolicyDecisions, func(item RuntimeReplayPolicyDecision) bool {
					return item.ToolCallID == "tool-rm" && item.ShellRisk == string(permission.RiskDestructive)
				}) {
					t.Fatalf("missing shell classification replay: %#v", replay.Summary.PolicyDecisions)
				}
			},
		},
	}

	for _, scenario := range scenarios {
		t.Run(scenario.name, func(t *testing.T) {
			h := newRuntimeScenarioHarness(t)
			scenario.run(t, h)
			turnID := "turn-" + strings.Split(scenario.name, " ")[0]
			switch {
			case strings.Contains(scenario.name, "plan mode"):
				turnID = "turn-plan"
			case strings.Contains(scenario.name, "auto read"):
				turnID = "turn-auto"
			case strings.Contains(scenario.name, "shell destructive"):
				turnID = "turn-shell"
			}
			scenario.verify(t, h.replay(turnID))
		})
	}
}

func TestRuntimeScenarioHarnessToolDiscoveryMCPAndSkillGolden(t *testing.T) {
	t.Parallel()

	h := newRuntimeScenarioHarness(t)
	h.seedTurn("session-discovery", "turn-discovery")
	h.service.policy = runtimePolicyFromParts(permission.PolicyModeAutoRead, "scenario", []RuntimePolicyRule{{
		ID: "deny-mcp-write", Decision: string(permission.PolicyDeny), Source: "scenario", MCPTool: "write", Reason: "MCP write is disabled in scenario.",
	}}, time.Now().UnixMilli())
	caps := []RuntimeCapability{
		{ID: "builtin:view", Kind: "builtin_tool", Name: "view", Enabled: true, Risk: "read", Description: "Read files.", State: capabilityStateLoaded},
		{ID: "builtin:write", Kind: "builtin_tool", Name: "write", Enabled: true, Risk: "write", Description: "Write files.", State: capabilityStateLoaded},
		{ID: "mcp:docs:search", Kind: "mcp_tool", Name: "search", Source: "docs", Enabled: true, Risk: "network", Description: "Search docs.", State: capabilityStateUnloaded},
		{ID: "mcp:docs:write", Kind: "mcp_tool", Name: "write", Source: "docs", Enabled: true, Risk: "network", Description: "Write docs.", State: capabilityStateUnloaded},
		{ID: "mcp:disabled:search", Kind: "mcp_tool", Name: "search", Source: "disabled", Enabled: false, Risk: "network", Description: "Disabled MCP search.", State: capabilityStateDisabled},
		{ID: "skill:writer", Kind: "skill", Name: "writer", Enabled: true, Risk: "context", Description: "Writer skill.", State: capabilityStateUnloaded, Diagnostics: "Skill declares allowed_tools metadata; runtime preserves it as policy hints only.", SchemaSummary: "skill instructions; allowed_tools=write"},
	}
	results, omitted := h.service.filterAndScoreToolSearch("select:view,write,search,writer", caps, 10)
	if !slices.ContainsFunc(results, func(result RuntimeToolSearchResult) bool { return result.Name == "view" }) {
		t.Fatalf("selected results = %#v", results)
	}
	if !slices.ContainsFunc(omitted, func(item RuntimeToolSearchOmission) bool {
		return item.ID == "mcp:docs:write" && item.Reason == "policy_denied"
	}) {
		t.Fatalf("policy denied MCP omission missing: %#v", omitted)
	}
	if !slices.ContainsFunc(omitted, func(item RuntimeToolSearchOmission) bool { return item.ID == "mcp:disabled:search" }) {
		t.Fatalf("disabled MCP omission missing: %#v", omitted)
	}
	h.service.recordToolSearch(RuntimeToolSearchRequest{Query: "select:view,write,search,writer", SessionID: "session-discovery", TurnID: "turn-discovery"}, RuntimeToolSearchResponse{
		Query:        "select:view,write,search,writer",
		Results:      results,
		Omitted:      omitted,
		Total:        len(results),
		BudgetImpact: toolSearchBudgetImpact(results, omitted),
	})
	h.service.recordToolDisclosure(h.ctx, "session-discovery", "turn-discovery", []string{"view"}, []RuntimeToolSearchOmission{
		{Name: "write", Reason: toolDiscoveryReasonDeferred},
		{Name: "search", Reason: toolDiscoveryReasonDeferred},
	}, RuntimeBudgetBucket{Count: 1, EstimatedTokens: 8}, RuntimeBudgetBucket{Count: 2, EstimatedTokens: 20})

	replay := h.replay("turn-discovery")
	if !slices.Contains(replay.Summary.ToolDiscovery.Selected, "view") || !slices.Contains(replay.Summary.ToolDiscovery.Omitted, "write") {
		t.Fatalf("tool discovery replay = %#v", replay.Summary.ToolDiscovery)
	}
	if !slices.Contains(replay.Summary.ToolDiscovery.Denied, "write") {
		t.Fatalf("denied tool discovery replay = %#v", replay.Summary.ToolDiscovery)
	}
	if replay.Summary.ToolDiscovery.BudgetImpact.Omitted.Count == 0 {
		t.Fatalf("tool discovery budget impact missing: %#v", replay.Summary.ToolDiscovery)
	}
	if !slices.ContainsFunc(results, func(result RuntimeToolSearchResult) bool { return result.ID == "skill:writer" }) {
		t.Fatalf("skill allowed_tools metadata should not deny discovery by itself: %#v", results)
	}
}

func TestRuntimeScenarioHarnessCompactBudgetRecoveryGolden(t *testing.T) {
	t.Parallel()

	h := newRuntimeScenarioHarness(t)
	h.seedTurn("session-compact", "turn-compact")
	for i := 0; i < 4; i++ {
		id := "tool-compact-" + string(rune('1'+i))
		if _, err := h.service.toolCalls.CreateCall(h.ctx, scheduler.ToolCallRequest{
			ID: id, SessionID: "session-compact", TurnID: "turn-compact", Name: "bash", Source: scheduler.ToolSourceShell, InputSummary: `{"command":"printf output"}`,
		}); err != nil {
			t.Fatal(err)
		}
		if _, err := h.service.toolCalls.CompleteCall(h.ctx, scheduler.ToolCallResult{
			ToolCallID: id, Status: scheduler.ToolCallCompleted, OutputSummary: strings.Repeat("tool output ", 700),
		}); err != nil {
			t.Fatal(err)
		}
	}
	before := RuntimeBudgetReport{SessionID: "session-compact", TurnID: "turn-compact", ToolOutputs: RuntimeBudgetBucket{Count: 4, EstimatedTokens: 4000}, TotalEstimatedTokens: 4000, UpdatedAt: time.Now().UnixMilli()}
	h.service.recordTurnBudgetBoundary(h.ctx, "session-compact", "turn-compact", before)
	after, boundary := h.service.maybeMicroCompactToolOutputs(h.ctx, "session-compact", "turn-compact", before)
	if boundary == nil || after.TotalEstimatedTokens >= before.TotalEstimatedTokens {
		t.Fatalf("micro compact failed: before=%#v after=%#v boundary=%#v", before, after, boundary)
	}
	call, err := h.service.toolCalls.GetCall(h.ctx, "tool-compact-1")
	if err != nil {
		t.Fatal(err)
	}
	if !call.Compacted || call.OutputSummary != microCompactReplacementText || call.CompactBoundaryID != boundary.ID {
		t.Fatalf("micro compact invariant failed: %#v boundary=%#v", call, boundary)
	}

	for i := 0; i < runtimeEventLimit+2; i++ {
		h.service.storeRuntimeEvent(runtimeapi.Event{Type: "scenario.noop", CreatedAt: time.Now().UTC().Format(time.RFC3339Nano), SessionID: "session-compact", TurnID: "turn-compact", Payload: map[string]any{"index": i}})
	}
	events, err := h.service.Events(h.ctx, 1)
	if err != nil {
		t.Fatal(err)
	}
	if !events.SnapshotRequired {
		t.Fatalf("expected snapshot-required after cursor gap: %#v", events)
	}

	perm := RuntimePermissionRequest{ID: "perm-compact", SessionID: "session-compact", TurnID: "turn-compact", ToolCallID: "tool-compact-1", ToolName: "bash", Action: "execute", Risk: string(permission.RiskExecute), PolicyMode: string(permission.PolicyModeAutoRead), PolicyReason: "Auto-read asks before non-read tool calls.", PolicyRuleID: "ask-shell", Status: permissionStatusPending, CreatedAt: time.Now().UnixMilli()}
	if _, err := h.service.permissionStore.Upsert(h.ctx, perm); err != nil {
		t.Fatal(err)
	}
	h.service.permissions[perm.ID] = pendingRuntimePermission{Permission: perm}
	h.service.recovery.startedAt = time.Now().UTC()
	activeTurns, err := h.service.turns.List(h.ctx, "active")
	if err != nil {
		t.Fatal(err)
	}
	pendingPermissions, err := h.service.permissionStore.List(h.ctx, permissionStatusPending)
	if err != nil {
		t.Fatal(err)
	}
	status := RuntimeRecoveryStatus{
		RuntimeStartedAt:   h.service.recovery.startedAt.Format(time.RFC3339Nano),
		LastEventSequence:  events.LastSequence,
		ActiveTurns:        activeTurns,
		PendingPermissions: pendingPermissions,
		SnapshotRequired:   events.SnapshotRequired,
	}
	if len(status.PendingPermissions) != 1 || status.PendingPermissions[0].PolicyMode != string(permission.PolicyModeAutoRead) || status.PendingPermissions[0].PolicyRuleID != "ask-shell" {
		t.Fatalf("pending permission recovery lost policy context: %#v", status.PendingPermissions)
	}

	replay := h.replay("turn-compact")
	if !replay.Summary.Recovery.SnapshotRequired && !events.SnapshotRequired {
		t.Fatalf("recovery snapshot-required not observable: %#v", replay.Summary.Recovery)
	}
	if len(replay.Summary.CompactBoundaries) < 2 || replay.Summary.Budget == nil {
		t.Fatalf("compact replay missing budget/boundaries: %#v", replay.Summary)
	}
	if replay.Source != "runtime_audit_events+runtime_events" {
		t.Fatalf("replay source = %q, want persisted runtime_events", replay.Source)
	}
	if len(replay.Events) <= runtimeEventLimit {
		t.Fatalf("persisted replay did not survive event buffer rollover: got %d events", len(replay.Events))
	}
}

func TestRuntimeScenarioHarnessPersistedReplaySurvivesRestart(t *testing.T) {
	t.Parallel()

	h := newRuntimeScenarioHarness(t)
	h.seedTurn("session-restart", "turn-restart")
	h.service.storeRuntimeEvent(runtimeapi.Event{
		Type:      runtimeapi.EventToolSearchPerformed,
		CreatedAt: time.Now().UTC().Format(time.RFC3339Nano),
		SessionID: "session-restart",
		TurnID:    "turn-restart",
		Payload:   map[string]any{"query": "view", "selected": []string{"view"}, "omitted_count": 0, "summary": "1 matches"},
	})
	firstMax := h.service.nextEventSequence
	if firstMax == 0 {
		t.Fatal("expected runtime sequence to advance")
	}

	restarted := newRuntimeService()
	restarted.turns = newRuntimeTurnStore(h.service.turns.db)
	restarted.toolCalls = scheduler.New(NewRuntimeToolCallStoreForDB(h.service.turns.db))
	restarted.permissionStore = newRuntimePermissionStore(h.service.turns.db)
	restarted.compactBoundaries = newRuntimeCompactBoundaryStore(h.service.turns.db)
	restarted.worktrees = newRuntimeWorktreeStore(h.service.turns.db)
	restarted.agentTasks = newRuntimeAgentTaskStore(h.service.turns.db)
	restarted.eventStore = newRuntimeEventStore(h.service.turns.db)
	maxSequence, err := restarted.eventStore.MaxSequence(h.ctx)
	if err != nil {
		t.Fatal(err)
	}
	restarted.nextEventSequence = maxSequence

	replay, err := restarted.ReplayExport(h.ctx, RuntimeReplayExportRequest{TurnID: "turn-restart"})
	if err != nil {
		t.Fatal(err)
	}
	if replay.Source != "runtime_audit_events+runtime_events" {
		t.Fatalf("replay source = %q", replay.Source)
	}
	if replay.LastSequence != firstMax || len(replay.Events) != 1 || replay.Events[0].Type != runtimeapi.EventToolSearchPerformed {
		t.Fatalf("restart replay = %#v", replay)
	}
	next := restarted.storeRuntimeEvent(runtimeapi.Event{Type: "scenario.after_restart", SessionID: "session-restart", TurnID: "turn-restart"})
	if next.Sequence != firstMax+1 {
		t.Fatalf("next sequence after restart = %d, want %d", next.Sequence, firstMax+1)
	}
}

func TestRuntimeScenarioHarnessCancellationExitsWaitingAndLoopStates(t *testing.T) {
	t.Parallel()

	h := newRuntimeScenarioHarness(t)
	h.seedTurn("session-cancel", "turn-cancel")
	if _, err := h.service.toolCalls.CreateCall(h.ctx, scheduler.ToolCallRequest{
		ID: "tool-wait", SessionID: "session-cancel", TurnID: "turn-cancel", Name: "bash", Source: scheduler.ToolSourceShell, InputSummary: `{"command":"sleep 10"}`,
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := h.service.toolCalls.MarkWaitingPermission(h.ctx, "tool-wait"); err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC().UnixMilli()
	turn, err := h.service.turns.Get(h.ctx, "turn-cancel")
	if err != nil {
		t.Fatal(err)
	}
	turn.Status = turnStatusCancelled
	turn.FinishedAt = now
	if _, err := h.service.turns.Upsert(h.ctx, turn); err != nil {
		t.Fatal(err)
	}
	if err := h.service.toolCalls.CancelCall(h.ctx, "tool-wait"); err != nil {
		t.Fatal(err)
	}
	cancelled, err := h.service.toolCalls.GetCall(h.ctx, "tool-wait")
	if err != nil {
		t.Fatal(err)
	}
	h.service.storeRuntimeEvent(runtimeToolCallEvent(runtimeapi.EventToolCallCancelled, cancelled, map[string]any{"name": cancelled.Name, "summary": "turn cancelled", "status": string(cancelled.Status)}))
	h.service.storeRuntimeEvent(runtimeapi.Event{Type: runtimeapi.EventTurnCancelled, CreatedAt: time.Now().UTC().Format(time.RFC3339Nano), SessionID: "session-cancel", TurnID: "turn-cancel", Payload: map[string]any{"status": "cancelled"}})
	turn, err = h.service.turns.Get(h.ctx, "turn-cancel")
	if err != nil {
		t.Fatal(err)
	}
	if turn.Status != turnStatusCancelled {
		t.Fatalf("turn not cancelled: %#v", turn)
	}
	call, err := h.service.toolCalls.GetCall(h.ctx, "tool-wait")
	if err != nil {
		t.Fatal(err)
	}
	if call.Status != scheduler.ToolCallCancelled {
		t.Fatalf("waiting tool not cancelled: %#v", call)
	}
	h.assertEventType(runtimeapi.EventTurnCancelled)
}

func TestRuntimeScenarioHarnessCancelTurnPreservesRunOwnership(t *testing.T) {
	t.Parallel()

	h := newRuntimeScenarioHarness(t)
	h.attachBackend()
	session, err := h.service.runtime.CreateSession(h.ctx, h.service.workspace.ID, "cancel ownership")
	if err != nil {
		t.Fatal(err)
	}
	h.service.sessionID = session.ID
	run, err := h.service.runs.EnsureForSession(h.ctx, h.service.workspace.ID, session.ID, "cancel ownership", runtimeRunSourceUserPrompt)
	if err != nil {
		t.Fatal(err)
	}
	startedAt := time.Now().Add(-time.Second).UTC().UnixMilli()
	turn, err := h.service.turns.Upsert(h.ctx, RuntimeTurn{
		ID:            "turn-cancel-owned",
		SessionID:     session.ID,
		Status:        turnStatusRunning,
		PromptPreview: "cancel ownership",
		StartedAt:     startedAt,
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := h.service.runs.LinkTurn(h.ctx, run.ID, session.ID, turn.ID, startedAt); err != nil {
		t.Fatal(err)
	}
	h.service.requests[turn.ID] = runtimeRequestState{SessionID: session.ID, Status: "running", StartedAt: startedAt}
	h.service.sessionTurns[session.ID] = turn.ID
	if _, err := h.service.toolCalls.CreateCall(h.ctx, scheduler.ToolCallRequest{
		ID: "tool-cancel-owned", SessionID: session.ID, TurnID: turn.ID, Name: "bash", Source: scheduler.ToolSourceShell, InputSummary: `{"command":"sleep 10"}`,
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := h.service.toolCalls.MarkWaitingPermission(h.ctx, "tool-cancel-owned"); err != nil {
		t.Fatal(err)
	}

	status, err := h.service.CancelTurn(h.ctx, turn.ID)
	if err != nil {
		t.Fatal(err)
	}
	if status.Action == nil {
		t.Fatal("cancel turn action metadata missing")
	}
	if !status.Action.Accepted || status.Action.Reason != runtimeTurnActionReasonCancelled {
		t.Fatalf("cancel action metadata = %#v", status.Action)
	}
	if status.Action.Source.Kind != runtimeTurnActionSourceKind || status.Action.Source.Action != runtimeTurnActionCancel {
		t.Fatalf("cancel action source = %#v", status.Action.Source)
	}
	if !status.Action.Source.WorkbenchOnly || status.Action.Source.StartsWorker || status.Action.Source.IdempotentBy != "turn_id" || !status.Action.Source.SessionActivityParity {
		t.Fatalf("cancel action source semantics = %#v", status.Action.Source)
	}
	if len(status.Action.RefreshTargets) == 0 {
		t.Fatal("cancel action refresh targets missing")
	}
	if !runtimeRunSessionLinkedToTurn(h.ctx, h.service.runs, run.ID, session.ID, turn.ID) {
		t.Fatalf("cancel broke run turn link: run=%s turn=%s", run.ID, turn.ID)
	}
	cancelledTurn, err := h.service.turns.Get(h.ctx, turn.ID)
	if err != nil {
		t.Fatal(err)
	}
	if cancelledTurn.Status != turnStatusCancelled || cancelledTurn.FinishedAt == 0 {
		t.Fatalf("turn was not terminalized before transition: %#v", cancelledTurn)
	}
	cancelledCall, err := h.service.toolCalls.GetCall(h.ctx, "tool-cancel-owned")
	if err != nil {
		t.Fatal(err)
	}
	if cancelledCall.Status != scheduler.ToolCallCancelled {
		t.Fatalf("tool call was not cancelled: %#v", cancelledCall)
	}
	transitions, err := h.service.transitions.ListByRun(h.ctx, run.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(transitions) != 1 || transitions[0].Source != runtimeRunTransitionSourceTurnCancelled || transitions[0].TurnID != turn.ID {
		t.Fatalf("cancel transition missing: %#v", transitions)
	}
	if transitions[0].CreatedAt != cancelledTurn.FinishedAt {
		t.Fatalf("cancel transition was not recorded from terminal turn evidence: transition=%#v turn=%#v", transitions[0], cancelledTurn)
	}
	detail, err := h.service.Run(h.ctx, run.ID)
	if err != nil {
		t.Fatal(err)
	}
	if detail.Run.Status != runtimeRunStatusCancelled || detail.Projection.Status != runtimeRunStatusCancelled {
		t.Fatalf("run detail was not reconciled as cancelled: run=%#v projection=%#v", detail.Run, detail.Projection)
	}
	h.assertEventType(runtimeapi.EventTurnCancelled)
}

func TestRuntimeScenarioHarnessPendingPermissionDecisionSurvivesReload(t *testing.T) {
	h := newRuntimeScenarioHarness(t)
	h.attachBackend()
	h.seedTurn("session-perm-reload", "turn-perm-reload")
	turn, err := h.service.turns.Get(h.ctx, "turn-perm-reload")
	if err != nil {
		t.Fatal(err)
	}
	turn.Status = turnStatusWaitingPermission
	if _, err := h.service.turns.Upsert(h.ctx, turn); err != nil {
		t.Fatal(err)
	}
	if _, err := h.service.toolCalls.CreateCall(h.ctx, scheduler.ToolCallRequest{
		ID: "tool-perm-reload", SessionID: "session-perm-reload", TurnID: "turn-perm-reload", Name: "bash", Source: scheduler.ToolSourceShell, InputSummary: `{"command":"go test ./..."}`,
		Risk: string(permission.RiskExecute), PolicyMode: string(permission.PolicyModeAutoRead), PolicyReason: "Auto-read asks before non-read tool calls.",
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := h.service.toolCalls.MarkWaitingPermission(h.ctx, "tool-perm-reload"); err != nil {
		t.Fatal(err)
	}
	perm := RuntimePermissionRequest{
		ID: "perm-reload", SessionID: "session-perm-reload", TurnID: "turn-perm-reload", ToolCallID: "tool-perm-reload", ToolName: "bash",
		Action: "execute", Risk: string(permission.RiskExecute), PolicyMode: string(permission.PolicyModeAutoRead), PolicyReason: "Auto-read asks before non-read tool calls.",
		PolicyRuleID: "ask-shell", PolicyRuleSource: "scenario", PolicyScopeKind: "shell_prefix", PolicyScopeValue: "go test", Status: permissionStatusPending, CreatedAt: time.Now().UnixMilli(),
	}
	if _, err := h.service.permissionStore.Upsert(h.ctx, perm); err != nil {
		t.Fatal(err)
	}

	restarted := h.restartedService()
	restarted.runtime = h.service.runtime
	restarted.workspace = h.service.workspace
	recovery, err := restarted.RecoveryStatus(h.ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(recovery.PendingPermissions) != 1 || recovery.PendingPermissions[0].ID != perm.ID {
		t.Fatalf("pending permission recovery = %#v", recovery.PendingPermissions)
	}
	if _, err := restarted.DecidePermission(h.ctx, RuntimePermissionDecision{PermissionID: perm.ID, Action: string(apitypes.PermissionDeny)}); err != nil {
		t.Fatal(err)
	}
	decided, err := restarted.permissionStore.Get(h.ctx, perm.ID)
	if err != nil {
		t.Fatal(err)
	}
	if decided.Status != permissionStatusDenied || decided.DecidedAt == 0 {
		t.Fatalf("decided permission = %#v", decided)
	}
	call, err := restarted.toolCalls.GetCall(h.ctx, "tool-perm-reload")
	if err != nil {
		t.Fatal(err)
	}
	if call.Status != scheduler.ToolCallDenied || !call.IsError {
		t.Fatalf("denied tool call = %#v", call)
	}
	deniedTurn, err := restarted.turns.Get(h.ctx, "turn-perm-reload")
	if err != nil {
		t.Fatal(err)
	}
	if deniedTurn.Status != turnStatusFailed || deniedTurn.Error == "" {
		t.Fatalf("turn after denied permission = %#v", deniedTurn)
	}
	replay, err := restarted.ReplayExport(h.ctx, RuntimeReplayExportRequest{SessionID: "session-perm-reload", TurnID: "turn-perm-reload"})
	if err != nil {
		t.Fatal(err)
	}
	if !slices.ContainsFunc(replay.Summary.PermissionEvents, func(item RuntimeReplayPermission) bool {
		return item.PermissionID == perm.ID && item.Status == permissionStatusDenied
	}) {
		t.Fatalf("permission denial missing from replay: %#v", replay.Summary.PermissionEvents)
	}
	if !slices.ContainsFunc(replay.Summary.ToolCalls, func(item RuntimeToolCall) bool {
		return item.ID == "tool-perm-reload" && item.Status == string(scheduler.ToolCallDenied)
	}) {
		t.Fatalf("tool denial missing from replay summary: %#v", replay.Summary.ToolCalls)
	}
}

func TestRuntimeScenarioHarnessMultiSessionActiveRecoveryIsIndependent(t *testing.T) {
	h := newRuntimeScenarioHarness(t)
	h.attachBackend()
	for _, item := range []struct {
		sessionID string
		turnID    string
		status    string
	}{
		{"session-a", "turn-a", turnStatusRunning},
		{"session-b", "turn-b", turnStatusWaitingPermission},
		{"session-c", "turn-c", turnStatusCompleted},
	} {
		if _, err := h.service.turns.Upsert(h.ctx, RuntimeTurn{ID: item.turnID, SessionID: item.sessionID, Status: item.status, StartedAt: time.Now().UnixMilli()}); err != nil {
			t.Fatal(err)
		}
	}
	recovery, err := h.service.RecoveryStatus(h.ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(recovery.ActiveTurns) != 2 {
		t.Fatalf("active turns = %#v", recovery.ActiveTurns)
	}
	if !slices.ContainsFunc(recovery.ActiveTurns, func(turn RuntimeTurn) bool { return turn.ID == "turn-a" && turn.SessionID == "session-a" }) ||
		!slices.ContainsFunc(recovery.ActiveTurns, func(turn RuntimeTurn) bool { return turn.ID == "turn-b" && turn.SessionID == "session-b" }) {
		t.Fatalf("active turns lost session separation: %#v", recovery.ActiveTurns)
	}
	if slices.ContainsFunc(recovery.ActiveTurns, func(turn RuntimeTurn) bool { return turn.ID == "turn-c" }) {
		t.Fatalf("completed turn should not recover as active: %#v", recovery.ActiveTurns)
	}
}

func TestRuntimeScenarioHarnessMCPPendingDenyRecoveryReplay(t *testing.T) {
	h := newRuntimeScenarioHarness(t)
	h.attachBackend()
	h.service.sessionID = "session-mcp"
	h.seedTurn("session-mcp", "turn-mcp")
	pending, err := h.service.createMCPElicitationRequest(h.ctx, "docs", "mcp:docs:select_project", "Choose project", "Authorization: Bearer sk-secret", permission.PolicyResult{
		Decision: permission.PolicyAsk,
		Risk:     permission.RiskSecret,
		Mode:     permission.PolicyModeAutoRead,
		Profile:  "default",
		Reason:   "MCP runtime input is required.",
		RuleID:   "ask-mcp",
	})
	if err != nil {
		t.Fatal(err)
	}
	if pending.Status != mcpRequestStatusPending || !pending.Redacted {
		t.Fatalf("pending mcp request = %#v", pending)
	}
	restarted := h.restartedService()
	restarted.runtime = h.service.runtime
	restarted.workspace = h.service.workspace
	recovery, err := restarted.RecoveryStatus(h.ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(recovery.PendingMCPRequests) != 1 || recovery.PendingMCPRequests[0].ID != pending.ID {
		t.Fatalf("pending mcp recovery = %#v", recovery.PendingMCPRequests)
	}
	resp, err := restarted.DecideMCPRequest(h.ctx, RuntimeMCPRequestDecision{RequestID: pending.ID, Action: "deny", Error: "Authorization: Bearer sk-secret"})
	if err != nil {
		t.Fatal(err)
	}
	if resp.Request.Status != mcpRequestStatusDenied || strings.Contains(strings.ToLower(resp.Request.Error), "sk-secret") {
		t.Fatalf("denied mcp request = %#v", resp.Request)
	}
	replay, err := restarted.ReplayExport(h.ctx, RuntimeReplayExportRequest{SessionID: "session-mcp"})
	if err != nil {
		t.Fatal(err)
	}
	if !slices.ContainsFunc(replay.Summary.MCPRequests, func(item RuntimeReplayMCPRequest) bool {
		return item.RequestID == pending.ID && item.Status == mcpRequestStatusDenied && item.PolicyMode == string(permission.PolicyModeAutoRead)
	}) {
		t.Fatalf("mcp denial missing from replay: %#v", replay.Summary.MCPRequests)
	}
	data, err := json.Marshal(replay)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(strings.ToLower(string(data)), "sk-secret") {
		t.Fatalf("mcp replay leaked secret: %s", data)
	}
}

func TestRuntimeScenarioHarnessAgentTaskMessageOrderingRejectAndReplay(t *testing.T) {
	h := newRuntimeScenarioHarness(t)
	h.seedTurn("session-task", "turn-task")
	task := RuntimeAgentTask{
		ID: "task-order", ParentSessionID: "session-task", ParentTurnID: "turn-task", ParentToolCallID: "tool-task", ChildSessionID: "child-task",
		Title: "Task order", Kind: agentTaskKindSubagent, Name: "agent", Status: agentTaskStatusRunning, StartedAt: time.Now().UnixMilli(),
	}
	if _, err := h.service.agentTasks.Upsert(h.ctx, task); err != nil {
		t.Fatal(err)
	}
	first, err := h.service.CreateAgentTaskMessage(h.ctx, task.ID, RuntimeAgentTaskMessageCreateRequest{Direction: taskMessageDirectionChildToParent, Kind: taskMessageKindProgress, ContentSummary: "progress one"})
	if err != nil {
		t.Fatal(err)
	}
	second, err := h.service.SendAgentTaskFollowUp(h.ctx, task.ID, RuntimeAgentTaskMessageCreateRequest{ContentSummary: "please continue"})
	if err != nil {
		t.Fatal(err)
	}
	if first.Message.Sequence != 1 || second.Message.Sequence != 2 || second.Message.Status != taskMessageStatusRejected || second.Message.Error == "" {
		t.Fatalf("message ordering/rejection failed: first=%#v second=%#v err=%v", first.Message, second.Message, err)
	}
	messages, err := newRuntimeAgentTaskMessageStore(h.service.turns.db).ListByTask(h.ctx, task.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(messages) != 2 || messages[0].Sequence != 1 || messages[1].Sequence != 2 {
		t.Fatalf("stored messages = %#v", messages)
	}
	replay := h.replay("turn-task")
	if !slices.ContainsFunc(replay.Summary.AgentTaskMessages, func(item RuntimeAgentTaskMessage) bool {
		return item.ID == second.Message.ID && item.Status == taskMessageStatusRejected && item.Error != ""
	}) {
		t.Fatalf("rejected task message missing from replay: %#v", replay.Summary.AgentTaskMessages)
	}
}

func TestRuntimeScenarioHarnessWorktreeReplayAndScopeGolden(t *testing.T) {
	t.Parallel()

	h := newRuntimeScenarioHarness(t)
	h.seedTurn("session-worktree", "turn-worktree")
	task := RuntimeAgentTask{
		ID:              "task-worktree",
		ParentTurnID:    "turn-worktree",
		ParentSessionID: "session-worktree",
		ChildSessionID:  "child-worktree",
		Title:           "Isolated work",
		Kind:            agentTaskKindSubagent,
		CWD:             filepath.Join(h.connDir, "repo"),
		Status:          agentTaskStatusRunning,
		StartedAt:       time.Now().UnixMilli(),
	}
	if _, err := h.service.agentTasks.Upsert(h.ctx, task); err != nil {
		t.Fatal(err)
	}
	wt := RuntimeWorktree{
		ID:             "wt-worktree",
		SessionID:      "session-worktree",
		TurnID:         "turn-worktree",
		TaskID:         "task-worktree",
		BaseRepoPath:   task.CWD,
		WorktreePath:   filepath.Join(task.CWD, ".agent-builder", "worktrees", "wt-worktree"),
		Branch:         "agent-builder-wt-worktree",
		Ref:            "HEAD",
		Status:         worktreeStatusEntered,
		PreservePolicy: worktreePreserveOnFailure,
		CleanupPolicy:  worktreeCleanupManual,
		Owner:          "runtime",
	}
	if _, err := h.service.worktrees.Upsert(h.ctx, wt); err != nil {
		t.Fatal(err)
	}
	if err := h.service.applyWorktreeToTask(h.ctx, wt); err != nil {
		t.Fatal(err)
	}
	h.service.recordWorktreeEvent(h.ctx, runtimeapi.EventWorktreeCreated, "worktree_created", wt, "")
	h.service.recordWorktreeEvent(h.ctx, runtimeapi.EventWorktreeEntered, "worktree_entered", wt, "")

	updated, err := h.service.agentTasks.Get(h.ctx, "task-worktree")
	if err != nil {
		t.Fatal(err)
	}
	inside := agent.SchedulerToolCall{Name: "view", Source: "builtin", CapabilityID: "builtin:view", InputSummary: `{"effective_cwd":"` + filepath.ToSlash(filepath.Join(wt.WorktreePath, "pkg")) + `"}`}
	if reason := h.service.agentTaskScopeViolation(updated, inside); reason != "" {
		t.Fatalf("inside worktree denied: %s", reason)
	}
	outside := agent.SchedulerToolCall{Name: "view", Source: "builtin", CapabilityID: "builtin:view", InputSummary: `{"effective_cwd":"` + filepath.ToSlash(filepath.Join(task.CWD, "pkg")) + `"}`}
	if reason := h.service.agentTaskScopeViolation(updated, outside); reason == "" {
		t.Fatal("outside worktree should be denied")
	}
	replay := h.replay("turn-worktree")
	if !slices.ContainsFunc(replay.Summary.Worktrees, func(item RuntimeWorktree) bool {
		return item.ID == wt.ID && item.Status == worktreeStatusEntered
	}) {
		t.Fatalf("worktree replay missing: %#v", replay.Summary.Worktrees)
	}
}

func TestRuntimeReplayExportRedactsSecretLikePayloads(t *testing.T) {
	t.Parallel()

	h := newRuntimeScenarioHarness(t)
	h.seedTurn("session-secret", "turn-secret")
	h.service.storeRuntimeEvent(runtimeapi.Event{
		Type:       runtimeapi.EventToolCallOutput,
		CreatedAt:  time.Now().UTC().Format(time.RFC3339Nano),
		SessionID:  "session-secret",
		TurnID:     "turn-secret",
		ToolCallID: "tool-secret",
		Payload: map[string]any{
			"authorization": "Bearer sk-secret",
			"summary":       "token=sk-secret",
			"nested": map[string]any{
				"api_key": "sk-secret",
			},
		},
	})
	h.service.writeAudit(auditEntry{
		RequestID:  "turn-secret",
		Event:      "tool_call_completed",
		SessionID:  "session-secret",
		ToolCallID: "tool-secret",
		ToolCalls: []auditToolCall{{
			ID: "tool-secret", Name: "bash", Output: "Authorization: Bearer sk-secret",
		}},
		Extra: map[string]any{"env": map[string]any{"API_TOKEN": "sk-secret"}},
	})
	replay := h.replay("turn-secret")
	data, err := json.Marshal(replay)
	if err != nil {
		t.Fatal(err)
	}
	text := strings.ToLower(string(data))
	if strings.Contains(text, "sk-secret") || strings.Contains(text, "bearer sk") {
		t.Fatalf("replay export leaked secret: %s", data)
	}
	if !replay.Summary.Redacted {
		t.Fatalf("replay summary did not mark redaction: %#v", replay.Summary)
	}
}

func hasReplayPolicy(replay RuntimeReplayExportResponse, toolCallID string, decision permission.PolicyDecision, mode permission.PolicyMode) bool {
	return slices.ContainsFunc(replay.Summary.PolicyDecisions, func(item RuntimeReplayPolicyDecision) bool {
		return item.ToolCallID == toolCallID && item.Decision == string(decision) && item.Mode == string(mode)
	})
}
