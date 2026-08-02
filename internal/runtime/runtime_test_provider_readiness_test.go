package runtime

import (
	"context"
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
	base := filepath.Join(repoRoot, "tmp", "runtime-dev")
	if err := os.MkdirAll(base, 0o700); err != nil {
		t.Fatal(err)
	}
	root, err := os.MkdirTemp(base, name+"-")
	if err != nil {
		t.Fatal(err)
	}
	if !isPathInside(base, root) {
		t.Fatalf("refusing runtime dev root outside tmp/runtime-dev: %s", root)
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
	t.Setenv("AGENT_BUILDER_DESKTOP_ROOT", root)
	service := newRuntimeService()
	_, err := service.SaveConfiguredProvider(context.Background(), RuntimeConfiguredProviderRequest{
		ProviderID:   "openai",
		Name:         "Test Provider",
		Protocol:     "openai-compat",
		APIEndpoint:  providerURL,
		APIKey:       "phase-27-local-test-token",
		DefaultModel: "phase-27-local-model",
		Models:       []RuntimeProviderModel{{ID: "phase-27-local-model"}},
		Enabled:      true,
	})
	if err != nil {
		t.Fatal(err)
	}
}

func writeRuntimeDevPolicy(t *testing.T, root, mode string) {
	t.Helper()
	t.Setenv("AGENT_BUILDER_DESKTOP_ROOT", root)
	if _, err := newRuntimeService().UpdatePolicy(context.Background(), RuntimePolicyUpdateRequest{Mode: mode}); err != nil {
		t.Fatal(err)
	}
}

func containsString(values []string, want string) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}
