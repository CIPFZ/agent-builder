package runtime

import (
	"context"
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"

	"github.com/CIPFZ/agent-builder/internal/agent"
	"github.com/CIPFZ/agent-builder/internal/db"
	"github.com/CIPFZ/agent-builder/internal/permission"
	"github.com/CIPFZ/agent-builder/internal/runtimeapi"
	"github.com/CIPFZ/agent-builder/internal/tools/scheduler"
)

func TestRuntimeSandboxDecisionStorePersistence(t *testing.T) {
	ctx := context.Background()
	dataDir := t.TempDir()
	conn, err := db.Connect(ctx, dataDir)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Release(dataDir) })

	store := newRuntimeSandboxDecisionStore(conn)
	decision, err := store.Upsert(ctx, RuntimeSandboxDecision{
		SessionID:      "session-1",
		TurnID:         "turn-1",
		ToolCallID:     "tool-1",
		TaskID:         "task-1",
		Mode:           sandboxModeRequired,
		Status:         sandboxStatusUnavailable,
		Executor:       sandboxExecutorUnavailableBoundary,
		CWD:            filepath.Join(dataDir, "repo"),
		WorktreeID:     "wt-1",
		WorktreePath:   filepath.Join(dataDir, "repo", ".agent-builder", "worktrees", "wt-1"),
		CommandSummary: "rm -rf build",
		PolicyMode:     string(permission.PolicyModeAsk),
		PolicyProfile:  string(permission.PolicyProfileDefault),
		PolicyRule:     "deny-destructive",
		Reason:         "sandbox unavailable",
		Error:          "sandbox unavailable; fail closed",
		AllowedPaths:   []string{filepath.Join(dataDir, "repo")},
		DeniedPaths:    []string{filepath.Join(dataDir, "secret")},
		NetworkAllowed: false,
		NetworkReason:  "network denied",
	})
	if err != nil {
		t.Fatal(err)
	}
	restarted := newRuntimeSandboxDecisionStore(conn)
	got, err := restarted.Get(ctx, decision.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.ID != decision.ID || got.Status != sandboxStatusUnavailable || got.CWD == "" || len(got.AllowedPaths) != 1 {
		t.Fatalf("decision = %#v", got)
	}
	items, err := restarted.List(ctx, RuntimeSandboxDecisionListRequest{TurnID: "turn-1"})
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 1 || items[0].ID != decision.ID {
		t.Fatalf("items = %#v", items)
	}
}

func TestRuntimeSandboxDecisionPayloadIsPathSafe(t *testing.T) {
	payload := runtimeSandboxDecisionPayload(RuntimeSandboxDecision{
		ID:             "sandbox_1",
		SessionID:      "session-1",
		TurnID:         "turn-1",
		ToolCallID:     "tool-1",
		TaskID:         "task-1",
		Mode:           sandboxModeRequired,
		Status:         sandboxStatusDenied,
		Executor:       sandboxExecutorUnavailableBoundary,
		CWD:            filepath.Join("C:", "Users", "ytq", "repo"),
		WorktreePath:   filepath.Join("C:", "Users", "ytq", "repo", ".agent-builder", "worktrees", "wt-1"),
		CommandSummary: "rm -rf build",
		Reason:         "path validation denied command scope",
		Error:          "sandbox unavailable; fail closed",
		AllowedPaths:   []string{filepath.Join("C:", "Users", "ytq", "repo")},
		DeniedPaths:    []string{filepath.Join("C:", "Users", "ytq", "secret")},
		NetworkReason:  "network denied",
	})
	data, err := json.Marshal(payload)
	if err != nil {
		t.Fatal(err)
	}
	text := string(data)
	if strings.Contains(text, "C:\\Users\\ytq") || !strings.Contains(text, "fail closed") {
		t.Fatalf("payload not path-safe: %s", text)
	}
}

func TestRuntimeSandboxDecisionAuditAndReplaySummary(t *testing.T) {
	decision := RuntimeSandboxDecision{
		ID:             "sandbox_1",
		SessionID:      "session-1",
		TurnID:         "turn-1",
		ToolCallID:     "tool-1",
		TaskID:         "task-1",
		Mode:           sandboxModeRequired,
		Status:         sandboxStatusApplied,
		Executor:       sandboxExecutorUnavailableBoundary,
		CWD:            filepath.Join(t.TempDir(), "repo"),
		WorktreeID:     "wt-1",
		WorktreePath:   filepath.Join(t.TempDir(), "repo", ".agent-builder", "worktrees", "wt-1"),
		CommandSummary: "pwd",
		PolicyMode:     string(permission.PolicyModeAutoRead),
		PolicyProfile:  string(permission.PolicyProfileDefault),
		Reason:         "sandbox executor selected for shell command",
	}
	payload := runtimeSandboxDecisionPayload(decision)
	replay := buildRuntimeReplaySummary(
		RuntimeAuditTurnSummary{TurnID: "turn-1"},
		[]RuntimeEvent{{Type: runtimeapi.EventSandboxApplied, Payload: payload}},
		[]RuntimeAuditEvent{{Type: "sandbox_decision_recorded", Payload: map[string]any{"extra": map[string]any{"sandbox": payload}}}},
	)
	if len(replay.SandboxDecisions) != 1 || replay.SandboxDecisions[0].Status != sandboxStatusApplied {
		t.Fatalf("replay summary = %#v", replay.SandboxDecisions)
	}
}

func TestRuntimeSandboxEvaluationBoundaryAndHeadlessFailClosed(t *testing.T) {
	ctx := context.Background()
	dataDir := t.TempDir()
	conn, err := db.Connect(ctx, dataDir)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Release(dataDir) })

	service := newRuntimeService()
	service.turns = newRuntimeTurnStore(conn)
	service.agentTasks = newRuntimeAgentTaskStore(conn)
	service.eventStore = newRuntimeEventStore(conn)
	service.toolCalls = scheduler.New(NewRuntimeToolCallStoreForDB(conn))
	service.policy = runtimePolicyFromParts(permission.PolicyModeAsk, "headless", nil, 0)

	if _, err := service.toolCalls.CreateCall(ctx, scheduler.ToolCallRequest{
		ID:           "tool-1",
		SessionID:    "session-1",
		TurnID:       "turn-1",
		Name:         "bash",
		Source:       scheduler.ToolSourceShell,
		Command:      "pwd",
		InputSummary: `{"command":"pwd","cwd":"` + filepath.Join(dataDir, "repo") + `"}`,
	}); err != nil {
		t.Fatal(err)
	}
	call, err := service.toolCalls.GetCall(ctx, "tool-1")
	if err != nil {
		t.Fatal(err)
	}
	rec := &runtimeSchedulerRecorder{service: service}
	decision, denied, err := rec.evaluateSandboxDecision(ctx, agent.SchedulerToolCall{
		ID:           call.ID,
		SessionID:    call.SessionID,
		TurnID:       call.TurnID,
		Name:         call.Name,
		Source:       string(call.Source),
		Command:      call.Command,
		InputSummary: call.InputSummary,
	}, permission.PolicyResult{Decision: permission.PolicyAllow, Risk: permission.RiskExecute, Mode: permission.PolicyModeAsk, Profile: string(permission.PolicyProfileDefault)})
	if err != nil {
		t.Fatal(err)
	}
	if denied || decision.Status != sandboxStatusUnavailable || decision.ID == "" || decision.Error == "" {
		t.Fatalf("decision = %#v denied=%v", decision, denied)
	}
	tool, err := service.toolCalls.GetCall(ctx, "tool-1")
	if err != nil {
		t.Fatal(err)
	}
	if tool.SandboxDecisionID != decision.ID || tool.SandboxStatus != sandboxStatusUnavailable {
		t.Fatalf("tool call sandbox metadata missing: %#v", tool)
	}
	events, err := service.Events(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if !containsEventType(events.Events, runtimeapi.EventSandboxDecisionRecorded) || !containsEventType(events.Events, runtimeapi.EventSandboxUnavailable) {
		t.Fatalf("sandbox events missing: %#v", events.Events)
	}

	if _, err := service.toolCalls.CreateCall(ctx, scheduler.ToolCallRequest{ID: "tool-2", SessionID: "session-1", TurnID: "turn-1", Name: "bash", Source: scheduler.ToolSourceShell, Command: "touch file"}); err != nil {
		t.Fatal(err)
	}
	_, err = rec.EvaluateToolCall(ctx, agent.SchedulerToolCall{ID: "tool-2", SessionID: "session-1", TurnID: "turn-1", Name: "bash", Source: string(scheduler.ToolSourceShell), Command: "touch file", InputSummary: `{"command":"touch file"}`})
	if err != nil {
		t.Fatal(err)
	}
	if events, err := service.Events(ctx); err != nil {
		t.Fatal(err)
	} else if !containsEventType(events.Events, runtimeapi.EventPermissionPolicyApplied) {
		t.Fatalf("headless policy event missing: %#v", events.Events)
	}
}

func TestRuntimeSandboxScopeAndRefMetadataPropagation(t *testing.T) {
	ctx := context.Background()
	dataDir := t.TempDir()
	conn, err := db.Connect(ctx, dataDir)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Release(dataDir) })

	repo := initTestGitRepo(t, filepath.Join(dataDir, "repo"))
	service := newRuntimeService()
	service.turns = newRuntimeTurnStore(conn)
	service.agentTasks = newRuntimeAgentTaskStore(conn)
	service.refs = newRuntimeRefStore(conn, dataDir)
	service.eventStore = newRuntimeEventStore(conn)
	service.worktrees = newRuntimeWorktreeStore(conn)
	service.policy = runtimePolicyFromMode(permission.PolicyModeAutoRead, 0)
	if _, err := service.agentTasks.Upsert(ctx, RuntimeAgentTask{ID: "task-1", ParentSessionID: "session-1", ParentTurnID: "turn-1", ChildSessionID: "child-1", CWD: repo, Worktree: filepath.Join(repo, ".agent-builder", "worktrees", "wt-1"), Status: agentTaskStatusRunning}); err != nil {
		t.Fatal(err)
	}
	call := agent.SchedulerToolCall{ID: "tool-1", SessionID: "child-1", TurnID: "turn-1", Name: "bash", Source: string(scheduler.ToolSourceShell), Command: "pwd", InputSummary: `{"cwd":"` + filepath.Join(repo, "escape") + `"}`}
	rec := &runtimeSchedulerRecorder{service: service}
	decision, denied, err := rec.evaluateSandboxDecision(ctx, call, permission.PolicyResult{Decision: permission.PolicyAllow, Risk: permission.RiskExecute, Mode: permission.PolicyModeAutoRead, Profile: string(permission.PolicyProfileTask)})
	if err != nil {
		t.Fatal(err)
	}
	if !denied || !strings.Contains(decision.Error, "escapes AgentTask") {
		t.Fatalf("scope escape should be denied: %#v denied=%v", decision, denied)
	}
	ref, err := service.createRuntimeRef(ctx, runtimeRefCreateRequest{
		SessionID:         "session-1",
		TurnID:            "turn-1",
		ToolCallID:        "tool-1",
		TaskID:            "task-1",
		SandboxDecisionID: decision.ID,
		SandboxMode:       decision.Mode,
		SandboxStatus:     decision.Status,
		Kind:              runtimeRefKindShellJobOutput,
		MediaType:         "text/plain",
		ContentType:       "stdout",
		Payload:           []byte("hello"),
		Summary:           "hello",
	})
	if err != nil {
		t.Fatal(err)
	}
	if ref.SandboxDecisionID != decision.ID || ref.SandboxMode != decision.Mode || ref.SandboxStatus != decision.Status {
		t.Fatalf("ref sandbox metadata = %#v", ref)
	}
	toolRefs, err := service.Refs(ctx, RuntimeRefListRequest{ToolCallID: "tool-1"})
	if err != nil {
		t.Fatal(err)
	}
	if len(toolRefs.Refs) != 1 || toolRefs.Refs[0].SandboxDecisionID != decision.ID {
		t.Fatalf("tool refs = %#v", toolRefs.Refs)
	}
	decisionPayload := runtimeSandboxDecisionPayload(decision)
	replay := buildRuntimeReplaySummary(
		RuntimeAuditTurnSummary{TurnID: "turn-1"},
		[]RuntimeEvent{{Type: runtimeapi.EventSandboxDenied, Payload: decisionPayload}},
		[]RuntimeAuditEvent{{Type: "sandbox_decision_recorded", Payload: map[string]any{"extra": map[string]any{"sandbox": decisionPayload}}}},
	)
	if len(replay.SandboxDecisions) == 0 || replay.SandboxDecisions[0].ID != decision.ID {
		t.Fatalf("replay summary = %#v", replay.SandboxDecisions)
	}
}

func TestRuntimeSandboxWindowsDestructiveCoverage(t *testing.T) {
	result := permission.PolicyResult{Decision: permission.PolicyAllow, Risk: permission.RiskDestructive, Mode: permission.PolicyModeAsk, Profile: string(permission.PolicyProfileDefault)}
	decision := (&runtimeService{}).buildSandboxDecision(context.Background(), agent.SchedulerToolCall{SessionID: "session-1", TurnID: "turn-1", Name: "powershell", Source: string(scheduler.ToolSourceShell), Command: "Remove-Item -Recurse -Force C:\\temp", InputSummary: `{"command":"Remove-Item -Recurse -Force C:\\temp"}`}, result)
	if decision.Status != sandboxStatusDenied || !strings.Contains(strings.ToLower(decision.Error), "sandbox unavailable") {
		t.Fatalf("windows destructive sandbox decision = %#v", decision)
	}
}

func containsEventType(events []RuntimeEvent, eventType string) bool {
	for _, event := range events {
		if event.Type == eventType {
			return true
		}
	}
	return false
}
