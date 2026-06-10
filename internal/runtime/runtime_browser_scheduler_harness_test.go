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

	"github.com/charmbracelet/crush/internal/runtimeapi"
)

const phase281HarnessEnv = "AGENT_BUILDER_PHASE281_BROWSER_HARNESS"

func TestPhase281BrowserSchedulerClickHarnessServer(t *testing.T) {
	if os.Getenv(phase281HarnessEnv) != "1" {
		t.Skip("set " + phase281HarnessEnv + "=1 to run the local browser scheduler click harness server")
	}

	root := phase281HarnessRoot(t)
	if err := os.MkdirAll(filepath.Join(root, "config"), 0o700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("AGENT_BUILDER_DESKTOP_ROOT", root)

	provider := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"id":"chatcmpl-phase281","choices":[{"message":{"role":"assistant","content":"ok"}}]}`))
	}))
	defer provider.Close()
	writeRuntimeDevModelConfig(t, root, provider.URL)

	ctx := context.Background()
	service := newRuntimeService()
	if _, err := service.Status(ctx); err != nil {
		t.Fatal(err)
	}
	service.agentTaskRunner = nil
	service.mu.Lock()
	workspaceID := service.workspace.ID
	service.mu.Unlock()
	if workspaceID == "" {
		t.Fatal("workspace id is empty after runtime readiness")
	}

	sess, err := service.runtime.CreateSession(ctx, workspaceID, "Phase 28.1 scheduler click")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := service.SelectSession(ctx, sess.ID); err != nil {
		t.Fatal(err)
	}
	run, err := service.runs.EnsureForSession(ctx, workspaceID, sess.ID, "phase 28.1 scheduler click", runtimeRunSourceUserPrompt)
	if err != nil {
		t.Fatal(err)
	}
	turn, err := service.turns.Upsert(ctx, RuntimeTurn{
		ID:        "turn-phase281",
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
		ID:              "task-phase281-queued",
		ParentSessionID: sess.ID,
		ParentTurnID:    turn.ID,
		ChildSessionID:  "session-phase281-child-queued",
		PromptSummary:   "queued phase 28.1 task",
		Status:          agentTaskStatusQueued,
		StartedAt:       1100,
	})
	if err != nil {
		t.Fatal(err)
	}
	terminal, err := service.agentTasks.Upsert(ctx, RuntimeAgentTask{
		ID:              "task-phase281-terminal",
		ParentSessionID: sess.ID,
		ParentTurnID:    turn.ID,
		ChildSessionID:  "session-phase281-child-terminal",
		PromptSummary:   "terminal phase 28.1 task",
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
			CreatedAt: time.Now().UTC().Format(time.RFC3339Nano),
			Payload: map[string]any{
				"task_id": terminal.ID,
				"status":  agentTaskStatusCompleted,
				"source":  "phase281_duplicate_refresh",
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
	writePhase281HarnessManifest(t, manifestPath, map[string]string{
		"runtimeURL":     server.URL(),
		"runtimeToken":   server.Token(),
		"sessionID":      sess.ID,
		"runID":          run.ID,
		"queuedTaskID":   queued.ID,
		"terminalTaskID": terminal.ID,
		"stopPath":       stopPath,
	})

	deadline := time.Now().Add(phase281HarnessTimeout())
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

func phase281HarnessRoot(t *testing.T) string {
	t.Helper()
	repoRoot, err := filepath.Abs(filepath.Join("..", ".."))
	if err != nil {
		t.Fatal(err)
	}
	root := os.Getenv("AGENT_BUILDER_PHASE281_HARNESS_ROOT")
	if root == "" {
		root = filepath.Join(repoRoot, "tmp", "runtime-dev", "phase28-browser-scheduler-click")
	}
	root, err = filepath.Abs(root)
	if err != nil {
		t.Fatal(err)
	}
	if !isPathInside(filepath.Join(repoRoot, "tmp", "runtime-dev"), root) {
		t.Fatalf("refusing phase 28.1 harness root outside tmp/runtime-dev: %s", root)
	}
	return root
}

func phase281HarnessTimeout() time.Duration {
	seconds, err := strconv.Atoi(os.Getenv("AGENT_BUILDER_PHASE281_HARNESS_TIMEOUT_SECONDS"))
	if err != nil || seconds <= 0 {
		seconds = 180
	}
	return time.Duration(seconds) * time.Second
}

func writePhase281HarnessManifest(t *testing.T, path string, value map[string]string) {
	t.Helper()
	data, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatal(err)
	}
}
