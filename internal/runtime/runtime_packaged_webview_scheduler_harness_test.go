package runtime

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/CIPFZ/agent-builder/internal/agent"
	"github.com/CIPFZ/agent-builder/internal/config"
	"github.com/CIPFZ/agent-builder/internal/runtimeapi"
)

const (
	phase362SeedEnv         = "AGENT_BUILDER_PHASE362_PACKAGED_WEBVIEW_SEED"
	phase362SeedRootEnv     = "AGENT_BUILDER_PHASE362_HARNESS_ROOT"
	phase362ProviderURLEnv  = "AGENT_BUILDER_PHASE362_PROVIDER_URL"
	phase362ManifestNameEnv = "AGENT_BUILDER_PHASE362_MANIFEST"
)

func TestPhase362PackagedWebViewSchedulerSeed(t *testing.T) {
	if os.Getenv(phase362SeedEnv) != "1" {
		t.Skip("set " + phase362SeedEnv + "=1 to seed packaged WebView scheduler smoke data")
	}

	root := phase362HarnessRoot(t)
	providerURL := os.Getenv(phase362ProviderURLEnv)
	if providerURL == "" {
		t.Fatal(phase362ProviderURLEnv + " is required")
	}
	manifestPath := os.Getenv(phase362ManifestNameEnv)
	if manifestPath == "" {
		manifestPath = filepath.Join(root, "harness-manifest.json")
	}
	if err := os.MkdirAll(filepath.Join(root, "config"), 0o700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("AGENT_BUILDER_DESKTOP_ROOT", root)
	writeRuntimeDevModelConfig(t, root, providerURL)
	writeRuntimeDevPolicy(t, root, "full_access")

	ctx := context.Background()
	service := newRuntimeService()
	if _, err := service.Status(ctx); err != nil {
		t.Fatal(err)
	}
	workspaceID := service.workspace.ID
	ws, err := service.runtime.GetWorkspace(workspaceID)
	if err != nil {
		t.Fatal(err)
	}
	sess, err := service.runtime.CreateSession(ctx, workspaceID, "Phase 36.2 packaged WebView scheduler click")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := service.SelectSession(ctx, sess.ID); err != nil {
		t.Fatal(err)
	}
	child, err := ws.Sessions.CreateTaskSession(ctx, "tool-phase362-parent", sess.ID, "Phase 36.2 child task")
	if err != nil {
		t.Fatal(err)
	}
	run, err := service.runs.EnsureForSession(ctx, workspaceID, sess.ID, "phase 36.2 packaged WebView scheduler click", runtimeRunSourceUserPrompt)
	if err != nil {
		t.Fatal(err)
	}
	turn, err := service.turns.Upsert(ctx, RuntimeTurn{
		ID:        "turn-phase362",
		SessionID: sess.ID,
		Status:    turnStatusQueued,
		StartedAt: 1000,
		Provider:  localProviderID,
		Model:     "phase-27-local-model",
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := service.runs.LinkTurn(ctx, run.ID, sess.ID, turn.ID, turn.StartedAt); err != nil {
		t.Fatal(err)
	}
	queued, err := service.agentTasks.Upsert(ctx, RuntimeAgentTask{
		ID:               "task-phase362-queued",
		ParentSessionID:  sess.ID,
		ParentTurnID:     turn.ID,
		ParentToolCallID: "tool-phase362-parent",
		ChildSessionID:   child.ID,
		Title:            "Packaged WebView provider completion",
		Kind:             agentTaskKindSubagent,
		Role:             config.AgentTask,
		Name:             agent.AgentToolName,
		PromptSummary:    "Return exactly: phase 36.2 packaged WebView provider completed",
		Provider:         localProviderID,
		Model:            "phase-27-local-model",
		Status:           agentTaskStatusQueued,
		StartedAt:        1100,
	})
	if err != nil {
		t.Fatal(err)
	}
	terminal, err := service.agentTasks.Upsert(ctx, RuntimeAgentTask{
		ID:              "task-phase362-terminal",
		ParentSessionID: sess.ID,
		ParentTurnID:    turn.ID,
		ChildSessionID:  "session-phase362-terminal",
		PromptSummary:   "terminal phase 36.2 task",
		Status:          agentTaskStatusCompleted,
		Progress:        100,
		StartedAt:       1200,
		FinishedAt:      1300,
	})
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 2; i++ {
		service.storeRuntimeEvent(RuntimeEvent{
			ID:        newRuntimeEventID(),
			Type:      runtimeapi.EventTaskCompleted,
			SessionID: sess.ID,
			TurnID:    turn.ID,
			CreatedAt: "2026-06-11T00:00:00Z",
			Payload: map[string]any{
				"task_id": terminal.ID,
				"status":  agentTaskStatusCompleted,
				"source":  "phase362_duplicate_refresh",
			},
		})
	}

	queuedPlan, err := service.runtimeRunSchedulerPlan(ctx, RuntimeRunSchedulerPlanRequest{
		RunID:  run.ID,
		TaskID: queued.ID,
		Mode:   runtimeRunSchedulerPlanModeTaskTurn,
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(queuedPlan.Plan.Items) != 1 || !queuedPlan.Plan.Items[0].CanSchedule {
		t.Fatalf("queued task is not schedulable: %#v", queuedPlan.Plan.Items)
	}
	terminalPlan, err := service.runtimeRunSchedulerPlan(ctx, RuntimeRunSchedulerPlanRequest{
		RunID:  run.ID,
		TaskID: terminal.ID,
		Mode:   runtimeRunSchedulerPlanModeTaskTurn,
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(terminalPlan.Plan.Items) != 1 || terminalPlan.Plan.Items[0].CanSchedule {
		t.Fatalf("terminal task should not be schedulable: %#v", terminalPlan.Plan.Items)
	}

	writePhase362HarnessManifest(t, manifestPath, map[string]string{
		"sessionID":      sess.ID,
		"runID":          run.ID,
		"queuedTaskID":   queued.ID,
		"terminalTaskID": terminal.ID,
	})
}

func phase362HarnessRoot(t *testing.T) string {
	t.Helper()
	repoRoot, err := filepath.Abs(filepath.Join("..", ".."))
	if err != nil {
		t.Fatal(err)
	}
	root := os.Getenv(phase362SeedRootEnv)
	if root == "" {
		root = filepath.Join(repoRoot, "tmp", "runtime-dev", "phase362-packaged-webview-scheduler-click")
	}
	root, err = filepath.Abs(root)
	if err != nil {
		t.Fatal(err)
	}
	if !isPathInside(filepath.Join(repoRoot, "tmp", "runtime-dev"), root) {
		t.Fatalf("refusing phase 36.2 harness root outside tmp/runtime-dev: %s", root)
	}
	return root
}

func writePhase362HarnessManifest(t *testing.T, path string, value map[string]string) {
	t.Helper()
	data, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatal(err)
	}
}
