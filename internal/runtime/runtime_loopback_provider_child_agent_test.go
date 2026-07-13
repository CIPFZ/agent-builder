package runtime

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/CIPFZ/agent-builder/internal/agent"
	"github.com/CIPFZ/agent-builder/internal/config"
)

func TestPhase321LoopbackProviderChildAgentExecutionSmoke(t *testing.T) {
	t.Run("completes through real coordinator and loopback provider", func(t *testing.T) {
		providerCalls := 0
		service, run, task := phase321ProviderBackedTaskFixture(t, func(w http.ResponseWriter, r *http.Request) {
			providerCalls++
			writePhase321OpenAIChatStream(w, "phase 32.1 child provider completed")
		})

		resp, err := service.ExecuteRunTask(context.Background(), run.ID, task.ID)
		if err != nil {
			t.Fatal(err)
		}
		if !resp.Accepted || !resp.ExecutionStarted {
			t.Fatalf("execute response = %#v", resp)
		}
		if providerCalls == 0 {
			t.Fatal("loopback provider was not called")
		}
		refreshed, err := service.AgentTask(context.Background(), task.ID)
		if err != nil {
			t.Fatal(err)
		}
		if refreshed.Task.Status != agentTaskStatusCompleted || !strings.Contains(refreshed.Task.ResultSummary, "phase 32.1 child provider completed") {
			t.Fatalf("task = %#v", refreshed.Task)
		}
		if len(refreshed.Task.ArtifactRefs) != 0 {
			t.Fatalf("provider text should not create artifact refs without terminal recorder refs: %#v", refreshed.Task.ArtifactRefs)
		}
	})

	t.Run("provider failure terminalizes task without artifact refs", func(t *testing.T) {
		service, run, task := phase321ProviderBackedTaskFixture(t, func(w http.ResponseWriter, _ *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusInternalServerError)
			_, _ = w.Write([]byte(`{"error":{"message":"phase 32.1 provider disconnected"}}`))
		})

		resp, err := service.ExecuteRunTask(context.Background(), run.ID, task.ID)
		if err != nil {
			t.Fatal(err)
		}
		if !resp.Accepted || !resp.ExecutionStarted || resp.Task.Status != agentTaskStatusFailed {
			t.Fatalf("provider failure execute response = %#v", resp)
		}
		refreshed, getErr := service.AgentTask(context.Background(), task.ID)
		if getErr != nil {
			t.Fatal(getErr)
		}
		if refreshed.Task.Status != agentTaskStatusFailed || refreshed.Task.Progress != 100 {
			t.Fatalf("failed task = %#v", refreshed.Task)
		}
		if len(refreshed.Task.ArtifactRefs) != 0 {
			t.Fatalf("failed provider execution created artifact refs: %#v", refreshed.Task.ArtifactRefs)
		}
		refs, refsErr := service.Objects(context.Background(), RuntimeObjectListRequest{TaskID: task.ID})
		if refsErr != nil {
			t.Fatal(refsErr)
		}
		if len(refs.Objects) != 0 {
			t.Fatalf("failed provider execution created refs: %#v", refs.Objects)
		}
	})
}

func phase321ProviderBackedTaskFixture(t *testing.T, handler http.HandlerFunc) (*runtimeService, RuntimeRun, RuntimeAgentTask) {
	t.Helper()

	root := runtimeDevTestRoot(t, "phase321-loopback-provider-child-agent-"+strings.ReplaceAll(t.Name(), "/", "-")+"-"+time.Now().Format("20060102150405.000000000"))
	t.Setenv("AGENT_BUILDER_DESKTOP_ROOT", root)

	provider := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.HasSuffix(r.URL.Path, "/models"):
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"object":"list","data":[{"id":"phase-27-local-model","object":"model"}]}`))
		case strings.Contains(r.URL.Path, "chat/completions") || strings.Contains(r.URL.Path, "responses"):
			_, _ = io.Copy(io.Discard, r.Body)
			handler(w, r)
		default:
			_, _ = io.Copy(io.Discard, r.Body)
			handler(w, r)
		}
	}))
	t.Cleanup(provider.Close)
	writeRuntimeDevModelConfig(t, root, provider.URL)
	if err := os.WriteFile(filepath.Join(root, "config", "policy.json"), []byte(`{"mode":"full_access"}`), 0o600); err != nil {
		t.Fatal(err)
	}

	service := newRuntimeService()
	if _, err := service.Status(context.Background()); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if service.runtime != nil && service.workspace != nil {
			service.runtime.DeleteWorkspace(service.workspace.ID)
		}
	})

	workspaceID := service.workspace.ID
	ws, err := service.runtime.GetWorkspace(workspaceID)
	if err != nil {
		t.Fatal(err)
	}
	parent, err := service.runtime.CreateSession(context.Background(), workspaceID, "Phase 32.1 provider-backed child agent")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := service.SelectSession(context.Background(), parent.ID); err != nil {
		t.Fatal(err)
	}
	child, err := ws.Sessions.CreateTaskSession(context.Background(), "tool-phase321-parent", parent.ID, "Phase 32.1 child task")
	if err != nil {
		t.Fatal(err)
	}
	run, err := service.runs.EnsureForSession(context.Background(), workspaceID, parent.ID, "phase 32.1 provider-backed child agent", runtimeRunSourceUserPrompt)
	if err != nil {
		t.Fatal(err)
	}
	turn, err := service.turns.Upsert(context.Background(), RuntimeTurn{
		ID:        "turn-phase321-" + strings.ReplaceAll(t.Name(), "/", "-"),
		SessionID: parent.ID,
		Status:    turnStatusQueued,
		StartedAt: time.Now().Add(-time.Second).UnixMilli(),
		Provider:  localProviderID,
		Model:     "phase-27-local-model",
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := service.runs.LinkTurn(context.Background(), run.ID, parent.ID, turn.ID, turn.StartedAt); err != nil {
		t.Fatal(err)
	}
	task, err := service.agentTasks.Upsert(context.Background(), RuntimeAgentTask{
		ID:               "task-phase321-" + strings.ReplaceAll(t.Name(), "/", "-"),
		ParentSessionID:  parent.ID,
		ParentTurnID:     turn.ID,
		ParentToolCallID: "tool-phase321-parent",
		ChildSessionID:   child.ID,
		Title:            "Loopback provider child agent",
		Kind:             agentTaskKindSubagent,
		Role:             config.AgentTask,
		Name:             agent.AgentToolName,
		PromptSummary:    "Return exactly: phase 32.1 child provider completed",
		Provider:         localProviderID,
		Model:            "phase-27-local-model",
		Status:           agentTaskStatusQueued,
		StartedAt:        time.Now().Add(-500 * time.Millisecond).UnixMilli(),
	})
	if err != nil {
		t.Fatal(err)
	}
	return service, run, task
}

func writePhase321OpenAIChatStream(w http.ResponseWriter, text string) {
	w.Header().Set("Content-Type", "text/event-stream")
	_, _ = w.Write([]byte(`data: {"id":"chatcmpl-phase321","object":"chat.completion.chunk","choices":[{"index":0,"delta":{"role":"assistant"},"finish_reason":null}]}` + "\n\n"))
	_, _ = w.Write([]byte(`data: {"id":"chatcmpl-phase321","object":"chat.completion.chunk","choices":[{"index":0,"delta":{"content":` + quotePhase321JSON(text) + `},"finish_reason":null}]}` + "\n\n"))
	_, _ = w.Write([]byte(`data: {"id":"chatcmpl-phase321","object":"chat.completion.chunk","choices":[{"index":0,"delta":{},"finish_reason":"stop"}],"usage":{"prompt_tokens":8,"completion_tokens":6,"total_tokens":14}}` + "\n\n"))
	_, _ = w.Write([]byte("data: [DONE]\n\n"))
}

func quotePhase321JSON(value string) string {
	escaped := strings.ReplaceAll(value, `\`, `\\`)
	escaped = strings.ReplaceAll(escaped, `"`, `\"`)
	return `"` + escaped + `"`
}
