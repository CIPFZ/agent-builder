package runtime

import (
	"context"
	"testing"
	"time"

	"github.com/CIPFZ/agent-builder/internal/agent"
	"github.com/CIPFZ/agent-builder/internal/db"
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
		ParentTaskID:     "task-parent",
		ChildSessionID:   "session-child",
		TeamID:           "team-1",
		Dependencies:     []string{"task-dependency"},
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
	if task.ParentTaskID != "task-parent" || task.TeamID != "team-1" || len(task.Dependencies) != 1 || task.Dependencies[0] != "task-dependency" {
		t.Fatalf("task hierarchy = %#v", task)
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
	if _, err := store.Upsert(context.Background(), RuntimeAgentTask{
		ID:              "task-invalid",
		ParentSessionID: "session-parent",
		Status:          "not-a-status",
	}); err == nil {
		t.Fatal("expected invalid status error")
	}
}

func TestRuntimeAgentTaskRoleMessageResultStores(t *testing.T) {
	t.Parallel()

	dataDir := t.TempDir()
	conn, err := db.Connect(context.Background(), dataDir)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_ = db.Release(dataDir)
	})

	roleStore := newRuntimeAgentRoleStore(conn)
	role, err := roleStore.Upsert(context.Background(), RuntimeAgentRoleDefinition{
		ID:              "reviewer",
		Name:            "reviewer",
		Title:           "Reviewer",
		Description:     "Reviews scoped changes.",
		PromptSummary:   "Review only.",
		AllowedTools:    []string{"view", "grep", "view"},
		CapabilityScope: []string{"read"},
		Model:           "small",
		Risk:            "read",
		PolicyMetadata:  map[string]string{"source": "test"},
		Source:          "test",
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(role.AllowedTools) != 2 || role.PolicyMetadata["source"] != "test" {
		t.Fatalf("role = %#v", role)
	}
	roles, err := roleStore.List(context.Background())
	if err != nil || len(roles) != 1 {
		t.Fatalf("roles=%#v err=%v", roles, err)
	}

	messageStore := newRuntimeAgentTaskMessageStore(conn)
	msg, err := messageStore.Create(context.Background(), RuntimeAgentTaskMessage{
		TaskID:          "task-1",
		ParentTurnID:    "turn-1",
		ParentSessionID: "parent",
		ChildSessionID:  "child",
		Direction:       taskMessageDirectionChildToParent,
		Kind:            taskMessageKindResult,
		ContentSummary:  "done",
		Payload:         map[string]any{"ok": true},
		ArtifactRefs:    []string{"artifact:file:test.txt"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if msg.ID == "" || msg.Payload["ok"] != true {
		t.Fatalf("msg=%#v", msg)
	}
	if msg.Sequence != 1 || msg.Status != taskMessageStatusCreated {
		t.Fatalf("message sequence/status = %#v", msg)
	}
	delivered, err := messageStore.UpdateStatus(context.Background(), msg.ID, taskMessageStatusDelivered, "")
	if err != nil {
		t.Fatal(err)
	}
	if delivered.Status != taskMessageStatusDelivered || delivered.DeliveredAt == 0 {
		t.Fatalf("delivered=%#v", delivered)
	}
	processed, err := messageStore.UpdateStatus(context.Background(), msg.ID, taskMessageStatusProcessed, "")
	if err != nil {
		t.Fatal(err)
	}
	if processed.Status != taskMessageStatusProcessed || processed.ProcessedAt == 0 {
		t.Fatalf("processed=%#v", processed)
	}
	rejected, err := messageStore.Create(context.Background(), RuntimeAgentTaskMessage{
		TaskID:         "task-1",
		Direction:      taskMessageDirectionParentToChild,
		Kind:           taskMessageKindControl,
		Status:         taskMessageStatusRejected,
		ContentSummary: "stop rejected",
		Error:          "already final",
	})
	if err != nil {
		t.Fatal(err)
	}
	if rejected.Sequence != 2 || rejected.Status != taskMessageStatusRejected || rejected.Error == "" {
		t.Fatalf("rejected=%#v", rejected)
	}
	messages, err := messageStore.ListByTask(context.Background(), "task-1")
	if err != nil || len(messages) != 2 || messages[0].ArtifactRefs[0] != "artifact:file:test.txt" || messages[1].Sequence != 2 {
		t.Fatalf("messages=%#v err=%v", messages, err)
	}

	resultStore := newRuntimeAgentTaskResultStore(conn)
	result, err := resultStore.Upsert(context.Background(), RuntimeAgentTaskResult{
		TaskID:              "task-1",
		Status:              agentTaskStatusCompleted,
		Summary:             "done",
		ArtifactRefs:        []string{"artifact:file:test.txt"},
		RelatedMessageRefs:  []string{msg.ID},
		RelatedToolCallRefs: []string{"tool-1"},
		CompactBoundaryRefs: []string{"compact-1"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.Status != agentTaskStatusCompleted || result.RelatedMessageRefs[0] != msg.ID || result.CompactBoundaryRefs[0] != "compact-1" {
		t.Fatalf("result=%#v", result)
	}
}

func TestRuntimeAgentTaskScopeViolationFailsClosed(t *testing.T) {
	t.Parallel()

	service := &runtimeService{}
	task := RuntimeAgentTask{
		ID:              "task-1",
		Status:          agentTaskStatusRunning,
		AllowedTools:    []string{"view"},
		CapabilityScope: []string{"read"},
		CWD:             "C:/work/project",
	}
	if reason := service.agentTaskScopeViolation(task, agent.SchedulerToolCall{Name: "write", Source: "builtin", CapabilityID: "builtin:write"}); reason == "" {
		t.Fatal("write should be denied by allowed_tools")
	}
	if reason := service.agentTaskScopeViolation(task, agent.SchedulerToolCall{Name: "view", Source: "builtin", CapabilityID: "builtin:view", InputSummary: `{"cwd":"C:/work/other"}`}); reason == "" {
		t.Fatal("out-of-scope cwd should be denied")
	}
	if reason := service.agentTaskScopeViolation(task, agent.SchedulerToolCall{Name: "view", Source: "builtin", CapabilityID: "builtin:view", InputSummary: `{"cwd":"C:/work/project/sub"}`}); reason != "" {
		t.Fatalf("view under cwd should be allowed: %s", reason)
	}
	task.Status = agentTaskStatusCancelled
	if reason := service.agentTaskScopeViolation(task, agent.SchedulerToolCall{Name: "view", Source: "builtin", CapabilityID: "builtin:view"}); reason == "" {
		t.Fatal("final task should be denied")
	}
}
