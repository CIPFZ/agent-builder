package runtime

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/CIPFZ/agent-builder/internal/agent"
	"github.com/CIPFZ/agent-builder/internal/db"
	"github.com/CIPFZ/agent-builder/internal/runtimeapi"
	"github.com/CIPFZ/agent-builder/internal/tools/scheduler"
)

func TestRuntimeRefStoreCreateGetListPersistence(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	dataDir := t.TempDir()
	conn, err := db.Connect(ctx, dataDir)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Release(dataDir) })
	store := newRuntimeRefStore(conn, dataDir)

	output, err := store.Create(ctx, runtimeRefCreateRequest{
		SessionID: "session-1", TurnID: "turn-1", ToolCallID: "tool-1",
		Kind: runtimeRefKindOutput, MediaType: "text/plain", ContentType: "stdout",
		Payload: []byte("hello"),
	})
	if err != nil {
		t.Fatal(err)
	}
	artifact, err := store.Create(ctx, runtimeRefCreateRequest{
		SessionID: "session-1", TurnID: "turn-1", TaskID: "task-1",
		Kind: runtimeRefKindArtifact, MediaType: "application/json", ContentType: "structured_output",
		Payload: []byte(`{"ok":true}`),
	})
	if err != nil {
		t.Fatal(err)
	}
	restarted := newRuntimeRefStore(conn, dataDir)
	got, err := restarted.Get(ctx, output.URI)
	if err != nil {
		t.Fatal(err)
	}
	if got.ID != output.ID || got.URI != output.URI || got.StorageKind != runtimeRefStorageInline {
		t.Fatalf("output ref = %#v", got)
	}
	artifacts, err := restarted.List(ctx, RuntimeRefListRequest{SessionID: "session-1", Kind: runtimeRefKindArtifact})
	if err != nil {
		t.Fatal(err)
	}
	if len(artifacts) != 1 || artifacts[0].ID != artifact.ID {
		t.Fatalf("artifact refs = %#v", artifacts)
	}
}

func TestRuntimeRefStoreRejectsTraversalAndRedactsUnsafeRead(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	dataDir := t.TempDir()
	conn, err := db.Connect(ctx, dataDir)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Release(dataDir) })
	store := newRuntimeRefStore(conn, dataDir)
	if _, err := store.Create(ctx, runtimeRefCreateRequest{
		SessionID: "session-1", Kind: runtimeRefKindOutput, StoragePath: "../escape.txt", Payload: []byte("bad"),
	}); err == nil {
		t.Fatal("expected traversal storage path to be rejected")
	}
	ref, err := store.Create(ctx, runtimeRefCreateRequest{
		SessionID: "session-1", Kind: runtimeRefKindOutput, Payload: []byte("Authorization: Bearer secret-token"),
	})
	if err != nil {
		t.Fatal(err)
	}
	if ref.RedactionStatus != runtimeRefRedactionUnsafe || strings.Contains(ref.Preview, "secret-token") {
		t.Fatalf("unsafe ref not redacted: %#v", ref)
	}
	content, err := store.ReadContent(ctx, ref.ID)
	if err != nil {
		t.Fatal(err)
	}
	if !content.Redacted || strings.Contains(content.Content, "secret-token") {
		t.Fatalf("unsafe content leaked: %#v", content)
	}
}

func TestRuntimeRecorderLargeStdoutCreatesRefAndPreservesModelResult(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	service := newTestRuntimeServiceWithRefs(t)
	rec := &runtimeSchedulerRecorder{service: service}
	if err := rec.ToolCallStarted(ctx, agent.SchedulerToolCall{
		ID: "tool-1", SessionID: "session-1", TurnID: "turn-1", Name: "bash", Source: string(scheduler.ToolSourceShell),
	}); err != nil {
		t.Fatal(err)
	}
	stdout := strings.Repeat("line\n", 2000)
	if err := rec.ToolCallCompleted(ctx, agent.SchedulerToolCallResult{
		ToolCallID: "tool-1", SessionID: "session-1", TurnID: "turn-1", Name: "bash", Source: string(scheduler.ToolSourceShell), Stdout: stdout, ModelVisibleContent: stdout,
	}); err != nil {
		t.Fatal(err)
	}
	call, err := service.toolCalls.GetCall(ctx, "tool-1")
	if err != nil {
		t.Fatal(err)
	}
	if len(call.OutputRefs) == 0 || call.Stdout == "" || len(call.Stdout) >= len(stdout) {
		t.Fatalf("call refs/preview = %#v", call)
	}
	refs, err := service.refs.List(ctx, RuntimeRefListRequest{ToolCallID: "tool-1", Kind: runtimeRefKindShellJobOutput})
	if err != nil {
		t.Fatal(err)
	}
	if len(refs) == 0 || refs[0].SizeBytes <= int64(len(call.Stdout)) {
		t.Fatalf("refs = %#v call stdout len=%d", refs, len(call.Stdout))
	}
	content, err := service.refs.ReadContent(ctx, refs[0].ID)
	if err != nil {
		t.Fatal(err)
	}
	if content.Content != stdout {
		t.Fatalf("stored content len = %d, want %d", len(content.Content), len(stdout))
	}
}

func TestRuntimeRefsCompactTaskReplayAndEvents(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	service := newTestRuntimeServiceWithRefs(t)
	if _, err := service.toolCalls.CreateCall(ctx, scheduler.ToolCallRequest{ID: "tool-1", SessionID: "session-1", TurnID: "turn-1", Name: "bash", Source: scheduler.ToolSourceShell}); err != nil {
		t.Fatal(err)
	}
	if _, err := service.toolCalls.CompleteCall(ctx, scheduler.ToolCallResult{ToolCallID: "tool-1", Status: scheduler.ToolCallCompleted, ModelContent: strings.Repeat("large output ", 80)}); err != nil {
		t.Fatal(err)
	}
	ref, err := service.createRuntimeRef(ctx, runtimeRefCreateRequest{
		SessionID:   "session-1",
		TurnID:      "turn-1",
		ToolCallID:  "tool-1",
		Kind:        runtimeRefKindCompactOriginalOutput,
		MediaType:   "text/plain",
		ContentType: "compact_original_output",
		Payload:     []byte("large output"),
		Summary:     "original tool output preserved before projection replacement",
	})
	if err != nil {
		t.Fatal(err)
	}
	service.storeRuntimeEvent(runtimeapi.Event{
		ID:         newRuntimeEventID(),
		Type:       runtimeapi.EventCompactOutputPreserved,
		CreatedAt:  time.Now().UTC().Format(time.RFC3339Nano),
		SessionID:  "session-1",
		TurnID:     "turn-1",
		ToolCallID: "tool-1",
		Payload:    runtimeRefEventPayload(ref),
	})
	call, err := service.toolCalls.GetCall(ctx, "tool-1")
	if err != nil {
		t.Fatal(err)
	}
	call.OutputRefs = append(call.OutputRefs, ref.URI)
	if _, err := service.toolCalls.UpdateCall(ctx, call); err != nil {
		t.Fatal(err)
	}
	task := RuntimeAgentTask{ID: "task-1", ParentSessionID: "session-1", ParentTurnID: "turn-1", ParentToolCallID: "tool-task", Title: "Task", Name: "agent", Status: agentTaskStatusCompleted}
	task.ArtifactRefs = service.ensureTaskArtifactRefs(ctx, task, []string{"artifact:file:result.txt"})
	if _, err := service.agentTasks.Upsert(ctx, task); err != nil {
		t.Fatal(err)
	}
	msg, err := service.createAgentTaskMessage(ctx, task, RuntimeAgentTaskMessage{Kind: taskMessageKindArtifact, Direction: taskMessageDirectionChildToParent, ContentSummary: "artifact", ArtifactRefs: task.ArtifactRefs})
	if err != nil {
		t.Fatal(err)
	}
	if len(msg.ArtifactRefs) != 1 || !strings.HasPrefix(msg.ArtifactRefs[0], "runtime://refs/") {
		t.Fatalf("task message refs = %#v", msg.ArtifactRefs)
	}
	replay, err := service.ReplayExport(ctx, RuntimeReplayExportRequest{SessionID: "session-1", TurnID: "turn-1"})
	if err != nil {
		t.Fatal(err)
	}
	if len(replay.Summary.CompactOutputRefs) == 0 || len(replay.Summary.TaskArtifactRefs) == 0 {
		t.Fatalf("replay refs missing: %#v", replay.Summary)
	}
	replayJSON, err := json.Marshal(replay)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(replayJSON), "inlinePayload") {
		t.Fatalf("replay leaked inline payload field: %#v", replay.Summary)
	}
	var artifactEvent RuntimeEvent
	for _, event := range replay.Events {
		if event.Type == runtimeapi.EventTaskArtifactCreated {
			artifactEvent = event
		}
	}
	if artifactEvent.ID == "" || len(stringSliceFromMap(artifactEvent.Payload, "artifact_refs")) == 0 || stringFromMap(artifactEvent.Payload, "artifact:file") != "" {
		t.Fatalf("artifact event = %#v", artifactEvent)
	}
}

func newTestRuntimeServiceWithRefs(t *testing.T) *runtimeService {
	t.Helper()
	dataDir := t.TempDir()
	conn, err := db.Connect(context.Background(), dataDir)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Release(dataDir) })
	service := newRuntimeService()
	service.turns = newRuntimeTurnStore(conn)
	service.toolCalls = scheduler.New(NewRuntimeToolCallStoreForDB(conn))
	service.refs = newRuntimeRefStore(conn, dataDir)
	service.compactBoundaries = newRuntimeCompactBoundaryStore(conn)
	service.agentTasks = newRuntimeAgentTaskStore(conn)
	service.eventStore = newRuntimeEventStore(conn)
	return service
}
