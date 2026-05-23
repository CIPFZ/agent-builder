package runtime

import (
	"context"
	"testing"
	"time"

	"github.com/charmbracelet/crush/internal/db"
)

func TestRuntimeAgentTaskStoreUpsertListGetAndInterrupt(t *testing.T) {
	t.Parallel()

	dataDir := t.TempDir()
	conn, err := db.Connect(context.Background(), dataDir)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_ = db.Release(dataDir)
	})

	store := newRuntimeAgentTaskStore(conn)
	task, err := store.Upsert(context.Background(), RuntimeAgentTask{
		ID:               "task-1",
		ParentTurnID:     "turn-1",
		ParentSessionID:  "session-parent",
		ParentToolCallID: "tool-1",
		ChildSessionID:   "session-child",
		Title:            "Fetch Analysis",
		Kind:             agentTaskKindAgenticFetch,
		Role:             "fetch",
		Name:             "agentic_fetch",
		PromptSummary:    "summarize page",
		Provider:         "openai",
		Model:            "gpt-test",
		AllowedTools:     []string{"web_fetch", "web_search"},
		CapabilityScope:  []string{"network", "read"},
		CWD:              "C:/work",
		Status:           agentTaskStatusRunning,
		Progress:         25,
		StartedAt:        time.Now().UnixMilli(),
	})
	if err != nil {
		t.Fatal(err)
	}
	if task.ID != "task-1" || task.ParentTurnID != "turn-1" || task.ParentToolCallID != "tool-1" || task.ChildSessionID != "session-child" {
		t.Fatalf("task linkage = %#v", task)
	}
	if len(task.AllowedTools) != 2 || task.CapabilityScope[0] != "network" {
		t.Fatalf("scope = %#v %#v", task.AllowedTools, task.CapabilityScope)
	}

	tasks, err := store.ListByTurn(context.Background(), "turn-1")
	if err != nil {
		t.Fatal(err)
	}
	if len(tasks) != 1 || tasks[0].ID != "task-1" {
		t.Fatalf("tasks = %#v", tasks)
	}

	interrupted, err := store.InterruptUnfinished(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(interrupted) != 1 || interrupted[0].Status != agentTaskStatusInterrupted {
		t.Fatalf("interrupted = %#v", interrupted)
	}
	stored, err := store.Get(context.Background(), "task-1")
	if err != nil {
		t.Fatal(err)
	}
	if stored.Status != agentTaskStatusInterrupted || stored.FinishedAt == 0 || stored.Progress != 100 {
		t.Fatalf("stored = %#v", stored)
	}
}
