package runtime

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/CIPFZ/agent-builder/internal/config"
	"github.com/CIPFZ/agent-builder/internal/db"
	"github.com/CIPFZ/agent-builder/internal/runtimeapi"
	"github.com/CIPFZ/agent-builder/internal/tools/scheduler"
)

func TestRuntimeHookExecutionStorePersistenceRedactionAndRecovery(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	dataDir := t.TempDir()
	conn, err := db.Connect(ctx, dataDir)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Release(dataDir) })

	store := newRuntimeHookExecutionStore(conn)
	started := time.Now().UnixMilli()
	created, err := store.Upsert(ctx, RuntimeHookExecution{
		ID:             "hook-exec-1",
		HookID:         "hook:PreToolUse:1",
		HookName:       "echo $API_KEY",
		HookSource:     "config",
		Event:          "PreToolUse",
		Status:         "running",
		SessionID:      "session-1",
		TurnID:         "turn-1",
		ToolCallID:     "tool-1",
		CapabilityID:   "shell:bash",
		PolicyMode:     "ask",
		PolicyProfile:  "headless",
		PolicyDecision: "allow",
		Headless:       true,
		InputSummary:   `{"authorization":"Bearer sk-secret","command":"echo ok"}`,
		OutputSummary:  "token=sk-secret",
		StartedAt:      started,
		Redacted:       true,
	})
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(created.InputSummary, "sk-secret") || strings.Contains(created.OutputSummary, "sk-secret") {
		t.Fatalf("hook execution leaked secret: %#v", created)
	}

	restarted := newRuntimeHookExecutionStore(conn)
	interrupted, err := restarted.InterruptRunning(ctx, started+10)
	if err != nil {
		t.Fatal(err)
	}
	if len(interrupted) != 1 || interrupted[0].Status != hookStatusFailed || interrupted[0].CompletedAt == 0 {
		t.Fatalf("interrupted = %#v", interrupted)
	}
	list, err := restarted.List(ctx, RuntimeHookExecutionsRequest{TurnID: "turn-1"})
	if err != nil {
		t.Fatal(err)
	}
	if len(list) != 1 || list[0].Status != hookStatusFailed {
		t.Fatalf("list = %#v", list)
	}
	data, err := json.Marshal(list)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(strings.ToLower(string(data)), "sk-secret") || strings.Contains(strings.ToLower(string(data)), "bearer sk") {
		t.Fatalf("persisted hook execution leaked secret: %s", data)
	}
}

func TestRuntimeReplayExportIncludesHookLifecycleSummary(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	dataDir := t.TempDir()
	conn, err := db.Connect(ctx, dataDir)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Release(dataDir) })

	service := newRuntimeService()
	service.eventStore = newRuntimeEventStore(conn)
	service.hookExecutions = newRuntimeHookExecutionStore(conn)
	service.turns = newRuntimeTurnStore(conn)
	service.refs = newRuntimeRefStore(conn, dataDir)
	service.toolCalls = scheduler.New(NewRuntimeToolCallStore())
	if _, err := service.turns.Upsert(ctx, RuntimeTurn{ID: "turn-1", SessionID: "session-1", Status: turnStatusCompleted}); err != nil {
		t.Fatal(err)
	}
	service.storeRuntimeEvent(runtimeapi.Event{
		ID:         "evt-hook-1",
		Type:       runtimeapi.EventHookExecutionCompleted,
		CreatedAt:  time.Now().UTC().Format(time.RFC3339Nano),
		SessionID:  "session-1",
		TurnID:     "turn-1",
		ToolCallID: "tool-1",
		Payload: map[string]any{
			"execution_id":    "hook-exec-1",
			"hook_id":         "hook:PreToolUse:1",
			"hook_name":       "bash",
			"hook_source":     "config",
			"event":           "PreToolUse",
			"status":          "completed",
			"capability_id":   "shell:bash",
			"policy_mode":     "ask",
			"policy_profile":  "default",
			"policy_decision": "allow",
			"redacted":        true,
			"started_at":      10,
			"completed_at":    20,
			"duration_ms":     10,
		},
	})

	replay, err := service.ReplayExport(ctx, RuntimeReplayExportRequest{SessionID: "session-1", TurnID: "turn-1"})
	if err != nil {
		t.Fatal(err)
	}
	if len(replay.Summary.Hooks) != 1 || replay.Summary.Hooks[0].Status != hookStatusCompleted {
		t.Fatalf("hooks summary = %#v", replay.Summary.Hooks)
	}
	if replay.Summary.Hooks[0].CapabilityID != "shell:bash" || replay.Summary.Hooks[0].PolicyMode != "ask" {
		t.Fatalf("hook refs/policy missing: %#v", replay.Summary.Hooks[0])
	}
}

func TestRuntimeHooksFromConfigDiagnostics(t *testing.T) {
	t.Parallel()

	resp := runtimeHooksFromConfig(map[string][]config.HookConfig{
		"PreToolUse": {{Command: "echo ok", Matcher: "^bash$"}},
		"Unknown":    {{Command: "echo no"}},
	})
	if len(resp.Hooks) != 2 {
		t.Fatalf("hooks = %#v", resp.Hooks)
	}
	if len(resp.Diagnostics) != 1 || !strings.Contains(resp.Diagnostics[0], "unknown hook event") {
		t.Fatalf("diagnostics = %#v", resp.Diagnostics)
	}
	var unknown RuntimeHook
	for _, hook := range resp.Hooks {
		if hook.Event == "Unknown" {
			unknown = hook
		}
	}
	if unknown.Status != hookStatusDisabled {
		t.Fatalf("unknown hook = %#v", unknown)
	}
}
