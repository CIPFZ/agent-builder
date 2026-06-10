package runtime

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"testing"
	"time"

	"github.com/charmbracelet/crush/internal/agent"
	"github.com/charmbracelet/crush/internal/runtimeapi"
)

const phase291HarnessEnv = "AGENT_BUILDER_PHASE291_BROWSER_HARNESS"

func TestPhase291BrowserSchedulerWorkerCompletionHarnessServer(t *testing.T) {
	if os.Getenv(phase291HarnessEnv) != "1" {
		t.Skip("set " + phase291HarnessEnv + "=1 to run the local browser scheduler worker completion harness server")
	}

	root := phase291HarnessRoot(t)
	if err := os.MkdirAll(filepath.Join(root, "config"), 0o700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("AGENT_BUILDER_DESKTOP_ROOT", root)

	provider := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"id":"chatcmpl-phase291","choices":[{"message":{"role":"assistant","content":"ok"}}]}`))
	}))
	defer provider.Close()
	writeRuntimeDevModelConfig(t, root, provider.URL)

	ctx := context.Background()
	service := newRuntimeService()
	if _, err := service.Status(ctx); err != nil {
		t.Fatal(err)
	}
	service.mu.Lock()
	workspaceID := service.workspace.ID
	service.mu.Unlock()
	if workspaceID == "" {
		t.Fatal("workspace id is empty after runtime readiness")
	}

	sess, err := service.runtime.CreateSession(ctx, workspaceID, "Phase 29.1 worker completion")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := service.SelectSession(ctx, sess.ID); err != nil {
		t.Fatal(err)
	}
	run, err := service.runs.EnsureForSession(ctx, workspaceID, sess.ID, "phase 29.1 worker completion", runtimeRunSourceUserPrompt)
	if err != nil {
		t.Fatal(err)
	}
	turn, err := service.turns.Upsert(ctx, RuntimeTurn{
		ID:        "turn-phase291",
		SessionID: sess.ID,
		Status:    turnStatusQueued,
		StartedAt: 1000,
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := service.runs.LinkTurn(ctx, run.ID, sess.ID, turn.ID, turn.StartedAt); err != nil {
		t.Fatal(err)
	}
	queued, err := service.agentTasks.Upsert(ctx, RuntimeAgentTask{
		ID:               "task-phase291-queued",
		ParentSessionID:  sess.ID,
		ParentTurnID:     turn.ID,
		ParentToolCallID: "tool-phase291-parent",
		ChildSessionID:   "session-phase291-child-queued",
		Title:            "Complete worker output",
		Kind:             agentTaskKindSubagent,
		Role:             "phase291-test-runner",
		Name:             "agent",
		PromptSummary:    "complete worker output",
		Status:           agentTaskStatusQueued,
		StartedAt:        1100,
	})
	if err != nil {
		t.Fatal(err)
	}
	terminal, err := service.agentTasks.Upsert(ctx, RuntimeAgentTask{
		ID:              "task-phase291-terminal",
		ParentSessionID: sess.ID,
		ParentTurnID:    turn.ID,
		ChildSessionID:  "session-phase291-child-terminal",
		PromptSummary:   "terminal phase 29.1 task",
		Status:          agentTaskStatusCompleted,
		Progress:        100,
		StartedAt:       1200,
		FinishedAt:      1300,
	})
	if err != nil {
		t.Fatal(err)
	}
	service.agentTaskRunner = &recordingRuntimeAgentTaskRunner{
		run: func(ctx context.Context, req RuntimeAgentTaskExecutionRequest) (RuntimeAgentTaskExecutionResult, error) {
			recorder := runtimeSchedulerRecorder{service: service}
			err := recorder.AgentTaskCompleted(ctx, agent.AgentTaskRecord{
				ID:               req.TaskID,
				ParentTurnID:     req.ParentTurnID,
				ParentSessionID:  req.ParentSessionID,
				ParentToolCallID: req.ParentToolCallID,
				ChildSessionID:   req.ChildSessionID,
				Title:            req.Title,
				Kind:             req.Kind,
				Role:             req.Role,
				Name:             req.Name,
				PromptSummary:    req.PromptSummary,
				Status:           agentTaskStatusCompleted,
				Progress:         100,
				ResultSummary:    "phase 29.1 worker completed",
				ArtifactRefs:     []string{"phase29-worker-completion-artifact"},
				StartedAt:        req.StartedAt,
				FinishedAt:       time.Now().UnixMilli(),
			})
			return RuntimeAgentTaskExecutionResult{
				TaskID:             req.TaskID,
				Status:             agentTaskStatusCompleted,
				Terminal:           true,
				ResultSummary:      "phase 29.1 worker completed",
				ArtifactRefs:       []string{"phase29-worker-completion-artifact"},
				NoStaleResume:      true,
				CompletionOnlyRefs: true,
			}, err
		},
	}
	for i := 0; i < 2; i++ {
		service.storeRuntimeEvent(RuntimeEvent{
			ID:        newRuntimeEventID(),
			Type:      runtimeapi.EventTaskArtifactCreated,
			SessionID: sess.ID,
			TurnID:    turn.ID,
			CreatedAt: time.Now().UTC().Format(time.RFC3339Nano),
			Payload: map[string]any{
				"task_id": queued.ID,
				"source":  "phase291_duplicate_refresh",
			},
		})
	}

	server := newRuntimeHTTPServer(service)
	if err := server.StartAt("127.0.0.1:0", "agent-builder-dev"); err != nil {
		t.Fatal(err)
	}
	defer func() {
		closeCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = server.Close(closeCtx)
	}()

	manifestPath := filepath.Join(root, "harness-manifest.json")
	stopPath := filepath.Join(root, "harness-stop")
	writePhase291HarnessManifest(t, manifestPath, map[string]string{
		"runtimeURL":     server.URL(),
		"runtimeToken":   server.Token(),
		"sessionID":      sess.ID,
		"runID":          run.ID,
		"queuedTaskID":   queued.ID,
		"terminalTaskID": terminal.ID,
		"stopPath":       stopPath,
	})

	deadline := time.Now().Add(phase291HarnessTimeout())
	for {
		if _, err := os.Stat(stopPath); err == nil {
			return
		}
		if time.Now().After(deadline) {
			t.Fatalf("timed out waiting for harness stop file: %s", stopPath)
		}
		time.Sleep(200 * time.Millisecond)
	}
}

func phase291HarnessRoot(t *testing.T) string {
	t.Helper()
	repoRoot, err := filepath.Abs(filepath.Join("..", ".."))
	if err != nil {
		t.Fatal(err)
	}
	root := os.Getenv("AGENT_BUILDER_PHASE291_HARNESS_ROOT")
	if root == "" {
		root = filepath.Join(repoRoot, "tmp", "runtime-dev", "phase29-worker-completion-browser-smoke")
	}
	root, err = filepath.Abs(root)
	if err != nil {
		t.Fatal(err)
	}
	if !isPathInside(filepath.Join(repoRoot, "tmp", "runtime-dev"), root) {
		t.Fatalf("refusing phase 29.1 harness root outside tmp/runtime-dev: %s", root)
	}
	return root
}

func phase291HarnessTimeout() time.Duration {
	seconds, err := strconv.Atoi(os.Getenv("AGENT_BUILDER_PHASE291_HARNESS_TIMEOUT_SECONDS"))
	if err != nil || seconds <= 0 {
		seconds = 180
	}
	return time.Duration(seconds) * time.Second
}

func writePhase291HarnessManifest(t *testing.T, path string, value map[string]string) {
	t.Helper()
	data, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatal(err)
	}
}
