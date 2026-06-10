package runtime

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/charmbracelet/crush/internal/agent"
	"github.com/charmbracelet/crush/internal/config"
	"github.com/charmbracelet/crush/internal/runtimeapi"
)

const phase331HarnessEnv = "AGENT_BUILDER_PHASE331_BROWSER_HARNESS"

func TestPhase331BrowserProviderCompletionHarnessServer(t *testing.T) {
	if os.Getenv(phase331HarnessEnv) != "1" {
		t.Skip("set " + phase331HarnessEnv + "=1 to run the browser provider completion harness server")
	}

	root := phase331HarnessRoot(t)
	if err := os.MkdirAll(filepath.Join(root, "config"), 0o700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("AGENT_BUILDER_DESKTOP_ROOT", root)

	provider := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "/models") {
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"object":"list","data":[{"id":"phase-27-local-model","object":"model"}]}`))
			return
		}
		writePhase321OpenAIChatStream(w, "phase 33.1 browser provider completed")
	}))
	defer provider.Close()
	writeRuntimeDevModelConfig(t, root, provider.URL)
	if err := os.WriteFile(filepath.Join(root, "config", "policy.json"), []byte(`{"mode":"full_access"}`), 0o600); err != nil {
		t.Fatal(err)
	}

	ctx := context.Background()
	service := newRuntimeService()
	if _, err := service.Status(ctx); err != nil {
		t.Fatal(err)
	}
	defer func() {
		if service.runtime != nil && service.workspace != nil {
			service.runtime.DeleteWorkspace(service.workspace.ID)
		}
	}()
	workspaceID := service.workspace.ID
	ws, err := service.runtime.GetWorkspace(workspaceID)
	if err != nil {
		t.Fatal(err)
	}
	sess, err := service.runtime.CreateSession(ctx, workspaceID, "Phase 33.1 browser provider completion")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := service.SelectSession(ctx, sess.ID); err != nil {
		t.Fatal(err)
	}
	child, err := ws.Sessions.CreateTaskSession(ctx, "tool-phase331-parent", sess.ID, "Phase 33.1 child task")
	if err != nil {
		t.Fatal(err)
	}
	run, err := service.runs.EnsureForSession(ctx, workspaceID, sess.ID, "phase 33.1 browser provider completion", runtimeRunSourceUserPrompt)
	if err != nil {
		t.Fatal(err)
	}
	turn, err := service.turns.Upsert(ctx, RuntimeTurn{
		ID:        "turn-phase331",
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
		ID:               "task-phase331-queued",
		ParentSessionID:  sess.ID,
		ParentTurnID:     turn.ID,
		ParentToolCallID: "tool-phase331-parent",
		ChildSessionID:   child.ID,
		Title:            "Browser provider completion",
		Kind:             agentTaskKindSubagent,
		Role:             config.AgentTask,
		Name:             agent.AgentToolName,
		PromptSummary:    "Return exactly: phase 33.1 browser provider completed",
		Provider:         localProviderID,
		Model:            "phase-27-local-model",
		Status:           agentTaskStatusQueued,
		StartedAt:        1100,
	})
	if err != nil {
		t.Fatal(err)
	}
	terminal, err := service.agentTasks.Upsert(ctx, RuntimeAgentTask{
		ID:              "task-phase331-terminal",
		ParentSessionID: sess.ID,
		ParentTurnID:    turn.ID,
		ChildSessionID:  "session-phase331-terminal",
		PromptSummary:   "terminal phase 33.1 task",
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
				"task_id": queued.ID,
				"source":  "phase331_duplicate_refresh",
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
	writePhase331HarnessManifest(t, manifestPath, map[string]string{
		"runtimeURL":     server.URL(),
		"runtimeToken":   server.Token(),
		"sessionID":      sess.ID,
		"runID":          run.ID,
		"queuedTaskID":   queued.ID,
		"terminalTaskID": terminal.ID,
		"stopPath":       stopPath,
	})

	deadline := time.Now().Add(phase331HarnessTimeout())
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

func phase331HarnessRoot(t *testing.T) string {
	t.Helper()
	repoRoot, err := filepath.Abs(filepath.Join("..", ".."))
	if err != nil {
		t.Fatal(err)
	}
	root := os.Getenv("AGENT_BUILDER_PHASE331_HARNESS_ROOT")
	if root == "" {
		root = filepath.Join(repoRoot, "tmp", "runtime-dev", "phase33-browser-provider-completion-smoke")
	}
	root, err = filepath.Abs(root)
	if err != nil {
		t.Fatal(err)
	}
	if !isPathInside(filepath.Join(repoRoot, "tmp", "runtime-dev"), root) {
		t.Fatalf("refusing phase 33.1 harness root outside tmp/runtime-dev: %s", root)
	}
	return root
}

func phase331HarnessTimeout() time.Duration {
	seconds, err := strconv.Atoi(os.Getenv("AGENT_BUILDER_PHASE331_HARNESS_TIMEOUT_SECONDS"))
	if err != nil || seconds <= 0 {
		seconds = 240
	}
	return time.Duration(seconds) * time.Second
}

func writePhase331HarnessManifest(t *testing.T, path string, value map[string]string) {
	t.Helper()
	data, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatal(err)
	}
}
