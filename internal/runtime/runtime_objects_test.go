package runtime

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/CIPFZ/agent-builder/internal/agent"
	agenttools "github.com/CIPFZ/agent-builder/internal/agent/tools"
	"github.com/CIPFZ/agent-builder/internal/config"
	"github.com/CIPFZ/agent-builder/internal/db"
	"github.com/CIPFZ/agent-builder/internal/message"
	"github.com/CIPFZ/agent-builder/internal/runtimeapi"
	"github.com/CIPFZ/agent-builder/internal/tools/scheduler"
)

func TestRuntimeObjectStoreCreateGetListPersistence(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	dataDir := t.TempDir()
	conn, err := db.Connect(ctx, dataDir)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Release(dataDir) })
	store := newRuntimeObjectStore(conn, dataDir)

	output, err := store.Create(ctx, runtimeObjectCreateRequest{
		ProjectID: "project-1",
		SessionID: "session-1", TurnID: "turn-1", ToolCallID: "tool-1",
		Kind: runtimeObjectKindOutput, MediaType: "text/plain", ContentType: "stdout",
		Payload: []byte("hello"),
	})
	if err != nil {
		t.Fatal(err)
	}
	artifact, err := store.Create(ctx, runtimeObjectCreateRequest{
		ProjectID: "project-1",
		SessionID: "session-1", TurnID: "turn-1", TaskID: "task-1",
		Kind: runtimeObjectKindArtifact, MediaType: "application/json", ContentType: "structured_output",
		Payload: []byte(`{"ok":true}`),
	})
	if err != nil {
		t.Fatal(err)
	}
	restarted := newRuntimeObjectStore(conn, dataDir)
	got, err := restarted.Get(ctx, output.URI)
	if err != nil {
		t.Fatal(err)
	}
	if got.ID != output.ID || got.URI != output.URI || got.StorageKind != runtimeObjectStorageInline {
		t.Fatalf("output ref = %#v", got)
	}
	artifacts, err := restarted.List(ctx, RuntimeObjectListRequest{SessionID: "session-1", Kind: runtimeObjectKindArtifact})
	if err != nil {
		t.Fatal(err)
	}
	if len(artifacts) != 1 || artifacts[0].ID != artifact.ID {
		t.Fatalf("artifact refs = %#v", artifacts)
	}
}

func TestRuntimeObjectStoreRejectsTraversalAndRedactsUnsafeRead(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	dataDir := t.TempDir()
	conn, err := db.Connect(ctx, dataDir)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Release(dataDir) })
	store := newRuntimeObjectStore(conn, dataDir)
	if _, err := store.Create(ctx, runtimeObjectCreateRequest{
		ProjectID: "project-1",
		SessionID: "session-1", Kind: runtimeObjectKindOutput, StoragePath: "../escape.txt", Payload: []byte("bad"),
	}); err == nil {
		t.Fatal("expected traversal storage path to be rejected")
	}
	ref, err := store.Create(ctx, runtimeObjectCreateRequest{
		ProjectID: "project-1",
		SessionID: "session-1", Kind: runtimeObjectKindOutput, Payload: []byte("Authorization: Bearer secret-token"),
	})
	if err != nil {
		t.Fatal(err)
	}
	if ref.RedactionStatus != runtimeObjectRedactionUnsafe || strings.Contains(ref.Preview, "secret-token") {
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
	refs, err := service.objects.List(ctx, RuntimeObjectListRequest{ToolCallID: "tool-1", Kind: runtimeObjectKindShellJobOutput})
	if err != nil {
		t.Fatal(err)
	}
	if len(refs) == 0 || refs[0].SizeBytes <= int64(len(call.Stdout)) {
		t.Fatalf("refs = %#v call stdout len=%d", refs, len(call.Stdout))
	}
	objectPath := filepath.Join(service.objects.dataDir, "projects", "project-1", "objects", filepath.FromSlash(refs[0].StoragePath))
	if _, err := os.Stat(objectPath); err != nil {
		t.Fatalf("project object payload missing at %s: %v", objectPath, err)
	}
	content, err := service.objects.ReadContent(ctx, refs[0].ID)
	if err != nil {
		t.Fatal(err)
	}
	if content.Content != stdout {
		t.Fatalf("stored content len = %d, want %d", len(content.Content), len(stdout))
	}
}

func TestCanonicalLargeToolInputUsesBoundedPreviewsAndReadableRefs(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	service := newTestRuntimeServiceWithRefs(t)
	recorder := &runtimeSchedulerRecorder{service: service}
	const inputSentinel = "FULL_WRITE_BODY_MUST_NOT_ENTER_CANONICAL"
	const commandSentinel = "FULL_COMMAND_TAIL_MUST_NOT_ENTER_CANONICAL"
	rawInputBytes, err := json.Marshal(map[string]string{
		"file_path": "large.txt",
		"content":   strings.Repeat("body-", 4000) + inputSentinel,
	})
	if err != nil {
		t.Fatal(err)
	}
	rawInput := string(rawInputBytes)
	rawCommand := "printf " + strings.Repeat("x", 4000) + commandSentinel
	if err := recorder.ToolCallStarted(ctx, agent.SchedulerToolCall{
		ID: "tool-large-input", SessionID: "session-1", TurnID: "turn-1", Name: "write", Source: string(scheduler.ToolSourceBuiltin), InputSummary: rawInput, Command: rawCommand,
	}); err != nil {
		t.Fatal(err)
	}
	stored, err := service.toolCalls.GetCall(ctx, "tool-large-input")
	if err != nil {
		t.Fatal(err)
	}
	if len(stored.InputSummary) > canonicalToolPreviewLimit || len(stored.Command) > canonicalToolPreviewLimit || stored.InputRef == "" || stored.CommandRef == "" {
		t.Fatalf("stored tool call is not bounded/ref-backed: %#v", stored)
	}
	if stored.InputByteLength != len(rawInput) || stored.CommandByteLength != len(rawCommand) {
		t.Fatalf("stored lengths input=%d command=%d", stored.InputByteLength, stored.CommandByteLength)
	}
	snapshot, err := service.buildSessionConversationSnapshotV2(ctx, "session-1", RuntimeCanonicalConversationSnapshotRequest{})
	if err != nil {
		t.Fatal(err)
	}
	if len(snapshot.ToolCalls) != 1 {
		t.Fatalf("tool calls = %#v", snapshot.ToolCalls)
	}
	canonical := snapshot.ToolCalls[0]
	if !canonical.InputTruncated || !canonical.CommandTruncated || canonical.InputRef != stored.InputRef || canonical.CommandRef != stored.CommandRef {
		t.Fatalf("canonical tool call = %#v", canonical)
	}
	encoded, err := json.Marshal(snapshot)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(encoded), inputSentinel) || strings.Contains(string(encoded), commandSentinel) || len(canonical.InputPreview) > canonicalToolPreviewLimit || len(canonical.CommandPreview) > canonicalToolPreviewLimit {
		t.Fatalf("full tool input leaked into canonical snapshot: %s", encoded)
	}
	baseline := RuntimeCanonicalConversationSnapshot{SchemaVersion: RuntimeConversationSchemaVersion, SessionID: "session-1", Cursor: "0", Scope: RuntimeConversationScopeFull, Turns: []RuntimeCanonicalTurn{}, Messages: []RuntimeCanonicalMessage{}, AssistantSteps: []RuntimeCanonicalAssistantStep{}, ToolCalls: []RuntimeCanonicalToolCall{}, ToolResults: []RuntimeCanonicalToolResult{}, Permissions: []RuntimeCanonicalPermission{}, TodoPlans: []RuntimeCanonicalTodoPlan{}, AgentTasks: []RuntimeCanonicalAgentTask{}, Notices: []RuntimeCanonicalNotice{}}
	events, err := canonicalDiffEntityEvents(RuntimeEvent{ID: "raw-1", Sequence: 1, SessionID: "session-1"}, baseline, snapshot)
	if err != nil {
		t.Fatal(err)
	}
	eventJSON, err := json.Marshal(events)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(eventJSON), inputSentinel) || strings.Contains(string(eventJSON), commandSentinel) {
		t.Fatalf("full tool input leaked into canonical entity event: %s", eventJSON)
	}
	inputContent, err := service.ReadObjectContent(ctx, canonical.InputRef)
	if err != nil || inputContent.Content != rawInput {
		t.Fatalf("input object content length=%d err=%v", len(inputContent.Content), err)
	}
	commandContent, err := service.ReadObjectContent(ctx, canonical.CommandRef)
	if err != nil || commandContent.Content != rawCommand {
		t.Fatalf("command object content length=%d err=%v", len(commandContent.Content), err)
	}
}

func TestToolResultGuardUsesProjectObjectWithoutWorkingDirectoryCopy(t *testing.T) {
	t.Parallel()
	ctx := context.WithValue(context.Background(), agenttools.TurnIDContextKey, "turn-1")
	service := newTestRuntimeServiceWithRefs(t)
	recorder := &runtimeSchedulerRecorder{service: service}
	content := strings.Repeat("large tool output\n", 1000)
	if err := recorder.ToolCallStarted(ctx, agent.SchedulerToolCall{
		ID: "tool-guard", SessionID: "session-1", TurnID: "turn-1", Name: "bash", Source: string(scheduler.ToolSourceShell),
	}); err != nil {
		t.Fatal(err)
	}
	result := agent.SchedulerToolCallResult{
		ToolCallID: "tool-guard", SessionID: "session-1", TurnID: "turn-1", Name: "bash", Source: string(scheduler.ToolSourceShell), ModelVisibleContent: content,
	}
	if err := recorder.ToolCallOutput(ctx, result); err != nil {
		t.Fatal(err)
	}
	if err := recorder.ToolCallCompleted(ctx, result); err != nil {
		t.Fatal(err)
	}
	cfg := config.DefaultToolResultGuardConfig()
	cfg.MaxResultChars = 100
	guard := agent.NewToolResultGuard(cfg, "session-1", recorder)
	processed := guard.Process(ctx, message.ToolResult{ToolCallID: "tool-guard", Name: "bash", Content: content})
	if !strings.HasPrefix(processed.StoredPath, "runtime://objects/") || !strings.Contains(processed.Content, processed.StoredPath) {
		t.Fatalf("guard did not reference Runtime object: %#v", processed)
	}
	refs, err := service.objects.List(ctx, RuntimeObjectListRequest{SessionID: "session-1", ToolCallID: "tool-guard"})
	if err != nil || len(refs) == 0 {
		t.Fatalf("object refs = %#v err=%v", refs, err)
	}
	if len(refs) != 1 {
		t.Fatalf("output and completion created duplicate Objects: %#v", refs)
	}
	ref := refs[len(refs)-1]
	objectPath, err := service.objects.resolveStoragePath("project-1", ref.StoragePath)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(objectPath); err != nil {
		t.Fatalf("project object payload missing: %v", err)
	}
	if _, err := os.Stat(filepath.Join(service.objects.dataDir, ".agent-builder", "results")); !os.IsNotExist(err) {
		t.Fatalf("legacy working-directory result store was created: %v", err)
	}
}

func TestRuntimeObjectsCompactTaskReplayAndEvents(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	service := newTestRuntimeServiceWithRefs(t)
	if _, err := service.toolCalls.CreateCall(ctx, scheduler.ToolCallRequest{ID: "tool-1", SessionID: "session-1", TurnID: "turn-1", Name: "bash", Source: scheduler.ToolSourceShell}); err != nil {
		t.Fatal(err)
	}
	if _, err := service.toolCalls.CompleteCall(ctx, scheduler.ToolCallResult{ToolCallID: "tool-1", Status: scheduler.ToolCallCompleted, ModelContent: strings.Repeat("large output ", 80)}); err != nil {
		t.Fatal(err)
	}
	call, err := service.toolCalls.GetCall(ctx, "tool-1")
	if err != nil {
		t.Fatal(err)
	}
	ref, err := service.createRuntimeObject(ctx, runtimeObjectCreateRequest{
		SessionID:   "session-1",
		TurnID:      "turn-1",
		ToolCallID:  "tool-1",
		Kind:        runtimeObjectKindCompactOriginalOutput,
		MediaType:   "text/plain",
		ContentType: "compact_original_output",
		Payload:     []byte("large output"),
		Summary:     "original tool output preserved before projection replacement",
	})
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
	if len(msg.ArtifactRefs) != 1 || !strings.HasPrefix(msg.ArtifactRefs[0], "runtime://objects/") {
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
	service.activeProjectID = "project-1"
	service.turns = newRuntimeTurnStore(conn)
	service.toolCalls = scheduler.New(NewRuntimeToolCallStoreForDB(conn))
	service.objects = newRuntimeObjectStore(conn, dataDir)
	service.agentTasks = newRuntimeAgentTaskStore(conn)
	service.eventStore = newRuntimeEventStore(conn)
	return service
}
