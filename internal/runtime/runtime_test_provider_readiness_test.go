package runtime

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
)

func TestRuntimeLocalModelConfigReadinessAllowsSchedulerCandidateProjection(t *testing.T) {
	root := runtimeDevTestRoot(t, "phase271-test-provider-readiness")
	t.Setenv("AGENT_BUILDER_DESKTOP_ROOT", root)

	provider := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"id":"chatcmpl-test","choices":[{"message":{"role":"assistant","content":"ok"}}]}`))
	}))
	defer provider.Close()

	writeRuntimeDevModelConfig(t, root, provider.URL)

	service := newRuntimeService()
	if _, err := service.Status(context.Background()); err != nil {
		t.Fatal(err)
	}

	run, err := service.runs.EnsureForSession(context.Background(), "workspace-1", "session-1", "test provider readiness", runtimeRunSourceUserPrompt)
	if err != nil {
		t.Fatal(err)
	}
	turn, err := service.turns.Upsert(context.Background(), RuntimeTurn{
		ID:        "turn-readiness",
		SessionID: "session-1",
		Status:    turnStatusQueued,
		StartedAt: 1000,
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := service.runs.LinkTurn(context.Background(), run.ID, "session-1", turn.ID, turn.StartedAt); err != nil {
		t.Fatal(err)
	}
	task, err := service.agentTasks.Upsert(context.Background(), RuntimeAgentTask{
		ID:              "task-readiness-queued",
		ParentSessionID: "session-1",
		ParentTurnID:    turn.ID,
		ChildSessionID:  "session-child-readiness",
		PromptSummary:   "readiness task prompt",
		Status:          agentTaskStatusQueued,
		StartedAt:       1100,
	})
	if err != nil {
		t.Fatal(err)
	}

	projection, err := service.RunProjection(context.Background(), RuntimeRunProjectionRequest{SessionID: "session-1"})
	if err != nil {
		t.Fatal(err)
	}
	if projection.Run.ID == "" || !containsString(projection.Run.TaskIDs, task.ID) {
		t.Fatalf("projection = %#v", projection.Run)
	}
	plan, err := service.RunSchedulerPlan(context.Background(), RuntimeRunSchedulerPlanRequest{RunID: run.ID, TaskID: task.ID})
	if err != nil {
		t.Fatal(err)
	}
	if len(plan.Plan.Items) != 1 || !plan.Plan.Items[0].CanSchedule || !plan.Plan.Items[0].OwnershipVerified {
		t.Fatalf("scheduler plan = %#v", plan.Plan.Items)
	}
}

func runtimeDevTestRoot(t *testing.T, name string) string {
	t.Helper()
	repoRoot, err := filepath.Abs(filepath.Join("..", ".."))
	if err != nil {
		t.Fatal(err)
	}
	root := filepath.Join(repoRoot, "tmp", "runtime-dev", name)
	if !isPathInside(filepath.Join(repoRoot, "tmp", "runtime-dev"), root) {
		t.Fatalf("refusing runtime dev root outside tmp/runtime-dev: %s", root)
	}
	if err := os.RemoveAll(root); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(root, "config"), 0o700); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_ = os.RemoveAll(root)
	})
	return root
}

func writeRuntimeDevModelConfig(t *testing.T, root, providerURL string) {
	t.Helper()
	data, err := json.MarshalIndent(RuntimeModelConfig{
		Protocol: "openai",
		URL:      providerURL,
		APIKey:   "phase-27-local-test-token",
		Model:    "phase-27-local-model",
		Models:   []string{"phase-27-local-model"},
	}, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "config", "model.json"), data, 0o600); err != nil {
		t.Fatal(err)
	}
}

func isPathInside(root, target string) bool {
	rel, err := filepath.Rel(filepath.Clean(root), filepath.Clean(target))
	return err == nil && rel != "." && rel != ".." && !filepath.IsAbs(rel) && len(rel) >= 2 && rel[:2] != ".."
}

func containsString(values []string, want string) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}
