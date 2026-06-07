package runtime

import (
	"context"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/charmbracelet/crush/internal/db"
	"github.com/charmbracelet/crush/internal/message"
	"github.com/charmbracelet/crush/internal/tools/scheduler"
)

func TestRuntimeTurnDiagnosticsMissingExpectedArtifact(t *testing.T) {
	t.Parallel()

	turn := RuntimeTurn{
		ID:            "turn-1",
		SessionID:     "session-1",
		Status:        turnStatusCompleted,
		UserMessageID: "message-1",
		PromptPreview: `write C:\work\agent-builder\tmp\runtime-dev\missing-report.md`,
	}
	messages := []RuntimeMessage{{
		ID:      "message-1",
		Role:    "user",
		Content: `请生成 C:\work\agent-builder\tmp\runtime-dev\missing-report.md`,
	}}

	diag := buildRuntimeTurnDiagnostics(turn, messages, nil, nil, nil)
	if !slices.Contains(diag.ExpectedArtifacts, `C:\work\agent-builder\tmp\runtime-dev\missing-report.md`) {
		t.Fatalf("expected artifacts = %#v", diag.ExpectedArtifacts)
	}
	if !slices.Contains(diag.MissingArtifacts, `C:\work\agent-builder\tmp\runtime-dev\missing-report.md`) {
		t.Fatalf("missing artifacts = %#v", diag.MissingArtifacts)
	}
	if diag.Warning == "" {
		t.Fatalf("warning was not set: %#v", diag)
	}
	if diag.WarningReason != "expected_artifact_not_produced" || diag.WarningSource != "tool_metadata" {
		t.Fatalf("warning reason/source = %q/%q", diag.WarningReason, diag.WarningSource)
	}
}

func TestRuntimeTurnDiagnosticsWriteProducedArtifactSuppressesWarning(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	path := filepath.Join(dir, "report.md")
	if err := os.WriteFile(path, []byte("ok"), 0o600); err != nil {
		t.Fatal(err)
	}
	turn := RuntimeTurn{
		ID:            "turn-1",
		SessionID:     "session-1",
		Status:        turnStatusCompleted,
		UserMessageID: "message-1",
	}
	messages := []RuntimeMessage{{ID: "message-1", Role: "user", Content: "输出到 " + path}}
	toolCalls := []RuntimeToolCall{{
		ID:           "tool-1",
		SessionID:    "session-1",
		TurnID:       "turn-1",
		Name:         "write",
		Status:       "completed",
		InputSummary: `{"file_path":"` + path + `","content":"ok"}`,
		StartedAt:    1000,
		FinishedAt:   1100,
		Display:      RuntimeToolCallDisplay{Kind: "file_write", Target: path},
	}}

	diag := buildRuntimeTurnDiagnostics(turn, messages, toolCalls, nil, nil)
	if !slices.Contains(diag.ExpectedArtifacts, path) {
		t.Fatalf("expected artifacts = %#v", diag.ExpectedArtifacts)
	}
	if !slices.Contains(diag.ProducedArtifacts, path) {
		t.Fatalf("produced artifacts = %#v", diag.ProducedArtifacts)
	}
	if !slices.Contains(diag.VerifiedArtifacts, path) {
		t.Fatalf("verified artifacts = %#v", diag.VerifiedArtifacts)
	}
	if len(diag.MissingArtifacts) != 0 || diag.Warning != "" {
		t.Fatalf("unexpected missing warning: %#v", diag)
	}
}

func TestRuntimeTurnDiagnosticsProducedMetadataMissingOnDiskWarnsFromFilesystem(t *testing.T) {
	t.Parallel()

	path := filepath.Join(t.TempDir(), "missing-produced.md")
	turn := RuntimeTurn{ID: "turn-1", SessionID: "session-1", Status: turnStatusCompleted, UserMessageID: "message-1"}
	messages := []RuntimeMessage{{ID: "message-1", Role: "user", Content: "write " + path}}
	toolCalls := []RuntimeToolCall{{
		ID:           "tool-1",
		SessionID:    "session-1",
		TurnID:       "turn-1",
		Name:         "write",
		Status:       "completed",
		InputSummary: `{"file_path":"` + path + `","content":"ok"}`,
		Display:      RuntimeToolCallDisplay{Kind: "file_write", Target: path},
	}}

	diag := buildRuntimeTurnDiagnostics(turn, messages, toolCalls, nil, nil)
	if !slices.Contains(diag.ProducedArtifacts, path) || !slices.Contains(diag.MissingArtifacts, path) {
		t.Fatalf("artifact diagnostics = %#v", diag)
	}
	if diag.WarningReason != "produced_artifact_missing_on_disk" || diag.WarningSource != "filesystem" {
		t.Fatalf("warning reason/source = %q/%q", diag.WarningReason, diag.WarningSource)
	}
	if diag.ArtifactVerificationAt == 0 {
		t.Fatalf("artifact verification time missing: %#v", diag)
	}
}

func TestRuntimeTurnDiagnosticsNoProducedMetadataMissingWarnsFromToolMetadata(t *testing.T) {
	t.Parallel()

	path := filepath.Join(t.TempDir(), "missing-no-produced.md")
	turn := RuntimeTurn{ID: "turn-1", SessionID: "session-1", Status: turnStatusCompleted, UserMessageID: "message-1"}
	messages := []RuntimeMessage{{ID: "message-1", Role: "user", Content: "write " + path}}

	diag := buildRuntimeTurnDiagnostics(turn, messages, nil, nil, nil)
	if !slices.Contains(diag.MissingArtifacts, path) {
		t.Fatalf("missing artifacts = %#v", diag.MissingArtifacts)
	}
	if diag.WarningReason != "expected_artifact_not_produced" || diag.WarningSource != "tool_metadata" {
		t.Fatalf("warning reason/source = %q/%q", diag.WarningReason, diag.WarningSource)
	}
}

func TestRuntimeTurnDiagnosticsShellCreatedExplicitFilePathCountsAsProduced(t *testing.T) {
	t.Parallel()

	path := filepath.Join(t.TempDir(), "shell-created.md")
	if err := os.WriteFile(path, []byte("ok"), 0o600); err != nil {
		t.Fatal(err)
	}
	turn := RuntimeTurn{ID: "turn-1", SessionID: "session-1", Status: turnStatusCompleted, UserMessageID: "message-1"}
	messages := []RuntimeMessage{{ID: "message-1", Role: "user", Content: "write " + path}}
	toolCalls := []RuntimeToolCall{{
		ID:      "tool-1",
		TurnID:  "turn-1",
		Name:    "powershell",
		Source:  "shell",
		Status:  "completed",
		Command: `Set-Content -LiteralPath "` + path + `" -Value ok`,
		Display: RuntimeToolCallDisplay{Kind: "shell", Command: `Set-Content -LiteralPath "` + path + `" -Value ok`},
	}}

	diag := buildRuntimeTurnDiagnostics(turn, messages, toolCalls, nil, nil)
	if !slices.Contains(diag.ProducedArtifacts, path) || !slices.Contains(diag.VerifiedArtifacts, path) {
		t.Fatalf("artifact diagnostics = %#v", diag)
	}
	if diag.Warning != "" {
		t.Fatalf("unexpected warning: %#v", diag)
	}
}

func TestRuntimeTurnDiagnosticsMCPStructuredArtifactRefsCountAsProduced(t *testing.T) {
	t.Parallel()

	path := filepath.Join(t.TempDir(), "mcp-created.md")
	if err := os.WriteFile(path, []byte("ok"), 0o600); err != nil {
		t.Fatal(err)
	}
	turn := RuntimeTurn{ID: "turn-1", SessionID: "session-1", Status: turnStatusCompleted, UserMessageID: "message-1"}
	messages := []RuntimeMessage{{ID: "message-1", Role: "user", Content: "write " + path}}
	toolCalls := []RuntimeToolCall{{
		ID:         "tool-1",
		TurnID:     "turn-1",
		Name:       "custom_writer",
		Source:     "mcp",
		Status:     "completed",
		Structured: `{"artifact_refs":[{"path":"` + strings.ReplaceAll(path, `\`, `\\`) + `"}]}`,
		Display:    RuntimeToolCallDisplay{Kind: "generic"},
	}}

	diag := buildRuntimeTurnDiagnostics(turn, messages, toolCalls, nil, nil)
	if !slices.Contains(diag.ProducedArtifacts, path) || !slices.Contains(diag.VerifiedArtifacts, path) {
		t.Fatalf("artifact diagnostics = %#v", diag)
	}
	if diag.Warning != "" {
		t.Fatalf("unexpected warning: %#v", diag)
	}
}

func TestRuntimeTurnDiagnosticsReadStructuredPathDoesNotCountAsProduced(t *testing.T) {
	t.Parallel()

	path := filepath.Join(t.TempDir(), "read-only.md")
	if err := os.WriteFile(path, []byte("existing"), 0o600); err != nil {
		t.Fatal(err)
	}
	turn := RuntimeTurn{ID: "turn-1", SessionID: "session-1", Status: turnStatusCompleted, UserMessageID: "message-1"}
	messages := []RuntimeMessage{{ID: "message-1", Role: "user", Content: "write " + path}}
	toolCalls := []RuntimeToolCall{{
		ID:         "tool-1",
		TurnID:     "turn-1",
		Name:       "view",
		Source:     "builtin",
		Status:     "completed",
		Structured: `{"path":"` + strings.ReplaceAll(path, `\`, `\\`) + `"}`,
		Display:    RuntimeToolCallDisplay{Kind: "file_read", Target: path},
	}}

	diag := buildRuntimeTurnDiagnostics(turn, messages, toolCalls, nil, nil)
	if slices.Contains(diag.ProducedArtifacts, path) {
		t.Fatalf("read-only structured path counted as produced: %#v", diag)
	}
	if !slices.Contains(diag.VerifiedArtifacts, path) {
		t.Fatalf("existing expected artifact should still verify on disk: %#v", diag)
	}
}

func TestRuntimeTurnDiagnosticsCountsFailedAndDeniedTools(t *testing.T) {
	t.Parallel()

	turn := RuntimeTurn{ID: "turn-1", SessionID: "session-1", Status: turnStatusCompleted}
	toolCalls := []RuntimeToolCall{
		{ID: "tool-1", TurnID: "turn-1", Status: "failed", StartedAt: 1000},
		{ID: "tool-2", TurnID: "turn-1", Status: "denied", StartedAt: 1100},
		{ID: "tool-3", TurnID: "turn-1", Status: "completed", StartedAt: 1200},
	}

	diag := buildRuntimeTurnDiagnostics(turn, nil, toolCalls, nil, nil)
	if diag.FailedToolCount != 1 || diag.DeniedToolCount != 1 {
		t.Fatalf("tool counts = failed %d denied %d", diag.FailedToolCount, diag.DeniedToolCount)
	}
	if diag.LastToolStatus != "completed" {
		t.Fatalf("last tool status = %q", diag.LastToolStatus)
	}
}

func TestRuntimeTurnDiagnosticsSummaryIncludesStatusKindPermissionsAndEvents(t *testing.T) {
	t.Parallel()

	turn := RuntimeTurn{ID: "turn-1", SessionID: "session-1", Status: turnStatusRunning, StartedAt: 1000}
	toolCalls := []RuntimeToolCall{
		{ID: "tool-1", TurnID: "turn-1", Status: "completed", StartedAt: 1000, Display: RuntimeToolCallDisplay{Kind: "file_read", Title: "Read file"}},
		{ID: "tool-2", TurnID: "turn-1", Status: "running", StartedAt: 1200, Display: RuntimeToolCallDisplay{Kind: "shell", Title: "Run command"}},
	}
	permissions := []RuntimePermissionRequest{
		{ID: "perm-1", TurnID: "turn-1", Status: permissionStatusPending},
		{ID: "perm-2", TurnID: "turn-1", Status: permissionStatusAllowedOnce},
		{ID: "perm-3", TurnID: "turn-1", Status: permissionStatusDenied},
		{ID: "perm-other", TurnID: "turn-other", Status: permissionStatusPending},
	}
	events := []RuntimeEvent{
		{Sequence: 7, TurnID: "turn-1", CreatedAt: "2026-06-07T10:00:00Z"},
		{Sequence: 9, TurnID: "turn-1", CreatedAt: "2026-06-07T10:00:02Z"},
	}

	diag := buildRuntimeTurnDiagnostics(turn, nil, toolCalls, permissions, events)
	if diag.TurnID != "turn-1" || diag.SessionID != "session-1" || diag.Status != turnStatusRunning {
		t.Fatalf("identity/status diagnostics = %#v", diag)
	}
	if diag.RunningDurationMS <= 0 || diag.ComputedAt == 0 {
		t.Fatalf("duration diagnostics = %#v", diag)
	}
	if diag.ToolCountsByStatus["completed"] != 1 || diag.ToolCountsByStatus["running"] != 1 {
		t.Fatalf("tool status counts = %#v", diag.ToolCountsByStatus)
	}
	if diag.ToolCountsByKind["file_read"] != 1 || diag.ToolCountsByKind["shell"] != 1 {
		t.Fatalf("tool kind counts = %#v", diag.ToolCountsByKind)
	}
	if diag.PermissionCounts.Pending != 1 || diag.PermissionCounts.Allowed != 1 || diag.PermissionCounts.Denied != 1 {
		t.Fatalf("permission counts = %#v", diag.PermissionCounts)
	}
	if diag.LastToolID != "tool-2" || diag.LastToolTitle != "Run command" {
		t.Fatalf("last tool = %#v", diag)
	}
	if diag.LastRuntimeEventSequence != 9 || diag.LastRuntimeEventAt == 0 {
		t.Fatalf("last event = %#v", diag)
	}
}

func TestRuntimeTurnDiagnosticsNonzeroShellCompletedSignal(t *testing.T) {
	t.Parallel()

	exit := 7
	turn := RuntimeTurn{ID: "turn-1", SessionID: "session-1", Status: turnStatusCompleted}
	toolCalls := []RuntimeToolCall{{
		ID:     "tool-1",
		TurnID: "turn-1",
		Name:   "bash",
		Source: "shell",
		Status: "completed",
		Display: RuntimeToolCallDisplay{
			Kind:     "shell",
			ExitCode: &exit,
			Title:    "Run failing command",
		},
	}}

	diag := buildRuntimeTurnDiagnostics(turn, nil, toolCalls, nil, nil)
	if diag.FailedToolCount != 0 {
		t.Fatalf("persisted completed shell should not increment failed tool count: %#v", diag)
	}
	if diag.NonzeroExitShellCount != 1 {
		t.Fatalf("nonzero shell signal missing: %#v", diag)
	}
}

func TestRuntimeSessionActivityExposesTurnDiagnosticsWarning(t *testing.T) {
	t.Parallel()

	runtimeBackend, workspace := backendForSkillTest(t)
	conn, err := db.Connect(context.Background(), workspace.DataDir)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_ = db.Release(workspace.DataDir)
	})
	service := newRuntimeService()
	service.runtime = runtimeBackend
	service.workspace = &workspace
	service.turns = newRuntimeTurnStore(conn)
	service.toolCalls = scheduler.New(NewRuntimeToolCallStoreForDB(conn))
	service.permissionStore = newRuntimePermissionStore(conn)
	service.eventStore = newRuntimeEventStore(conn)

	sess, err := runtimeBackend.CreateSession(context.Background(), workspace.ID, "diagnostics")
	if err != nil {
		t.Fatal(err)
	}
	ws, err := runtimeBackend.GetWorkspace(workspace.ID)
	if err != nil {
		t.Fatal(err)
	}
	expectedPath := filepath.Join(workspace.Path, "tmp", "runtime-dev", "activity-missing.md")
	userMessage, err := ws.Messages.Create(context.Background(), sess.ID, message.CreateMessageParams{
		Role:  message.User,
		Parts: []message.ContentPart{message.TextContent{Text: "write " + expectedPath}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := service.turns.Upsert(context.Background(), RuntimeTurn{
		ID:            "turn-diagnostics",
		SessionID:     sess.ID,
		Status:        turnStatusCompleted,
		UserMessageID: userMessage.ID,
		PromptPreview: "write " + expectedPath,
		StartedAt:     time.Now().Add(-time.Second).UnixMilli(),
		FinishedAt:    time.Now().UnixMilli(),
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := service.permissionStore.Upsert(context.Background(), RuntimePermissionRequest{
		ID:        "perm-diagnostics",
		SessionID: sess.ID,
		TurnID:    "turn-diagnostics",
		ToolName:  "bash",
		Action:    "execute",
		Status:    permissionStatusDenied,
		CreatedAt: time.Now().UnixMilli(),
	}); err != nil {
		t.Fatal(err)
	}
	service.storeRuntimeEvent(RuntimeEvent{
		Type:      "tool.call.failed",
		SessionID: sess.ID,
		TurnID:    "turn-diagnostics",
		CreatedAt: time.Now().UTC().Format(time.RFC3339Nano),
		Payload:   map[string]any{"summary": "failed"},
	})

	activity, err := service.SessionActivity(context.Background(), sess.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(activity.Turns) != 1 {
		t.Fatalf("turns = %#v", activity.Turns)
	}
	diag := activity.Turns[0].Diagnostics
	if diag.Warning == "" {
		t.Fatalf("diagnostics warning missing: %#v", diag)
	}
	if !slices.Contains(diag.MissingArtifacts, expectedPath) {
		t.Fatalf("missing artifacts = %#v, want %s", diag.MissingArtifacts, expectedPath)
	}
	if diag.PermissionCounts.Denied != 1 {
		t.Fatalf("permission counts were not restored from activity: %#v", diag.PermissionCounts)
	}
	if diag.LastRuntimeEventSequence == 0 || diag.LastRuntimeEventAt == 0 {
		t.Fatalf("event diagnostics were not restored from activity: %#v", diag)
	}
	if _, err := os.Stat(expectedPath); !os.IsNotExist(err) {
		t.Fatalf("test expected artifact should not exist; stat err = %v", err)
	}
}
