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

func TestRuntimeInterruptedSummaryPreservesRecoverySignals(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	producedPath := filepath.Join(dir, "produced.md")
	missingPath := filepath.Join(dir, "missing.md")
	if err := os.WriteFile(producedPath, []byte("ok"), 0o600); err != nil {
		t.Fatal(err)
	}
	exit := 7
	turn := RuntimeTurn{
		ID:            "turn-interrupted",
		SessionID:     "session-1",
		Status:        turnStatusInterrupted,
		UserMessageID: "message-1",
		StartedAt:     1000,
		FinishedAt:    2600,
		DurationMS:    1600,
		Error:         "runtime restarted before turn completed",
	}
	messages := []RuntimeMessage{{
		ID:      "message-1",
		Role:    "user",
		Content: "write " + producedPath + " and " + missingPath,
	}}
	toolCalls := []RuntimeToolCall{
		{
			ID:           "tool-write",
			TurnID:       turn.ID,
			Name:         "write",
			Source:       "builtin",
			Status:       "completed",
			InputSummary: `{"file_path":"` + producedPath + `","content":"ok"}`,
			StartedAt:    1100,
			FinishedAt:   1200,
			Display:      RuntimeToolCallDisplay{Kind: "file_write", Title: "Write file", Target: producedPath, PrimaryTarget: producedPath, ArtifactCount: 1},
		},
		{
			ID:         "tool-shell",
			TurnID:     turn.ID,
			Name:       "bash",
			Source:     "shell",
			Status:     "completed",
			StartedAt:  1300,
			FinishedAt: 1400,
			ExitCode:   7,
			Display: RuntimeToolCallDisplay{
				Kind:          "shell",
				Title:         "Run command",
				Command:       "go test ./missing",
				WorkingDir:    dir,
				ExitCode:      &exit,
				StderrExcerpt: "failed",
				FailureReason: "failed",
			},
		},
		{
			ID:         "tool-running",
			TurnID:     turn.ID,
			Name:       "mcp_writer",
			Source:     "mcp",
			Status:     "cancelled",
			StartedAt:  1500,
			FinishedAt: 2600,
			Structured: `{"artifact_refs":[{"path":"` + strings.ReplaceAll(producedPath, `\`, `\\`) + `"}]}`,
			Display: RuntimeToolCallDisplay{
				Kind:            "generic",
				Title:           "MCP writer",
				PrimaryTarget:   producedPath,
				Targets:         []string{producedPath},
				ArtifactRefs:    []string{producedPath},
				ArtifactSummary: "1 artifact ref",
			},
		},
	}
	permissions := []RuntimePermissionRequest{
		{ID: "perm-pending", TurnID: turn.ID, Status: permissionStatusPending},
		{ID: "perm-denied", TurnID: turn.ID, Status: permissionStatusDenied},
	}
	events := []RuntimeEvent{
		{Sequence: 10, TurnID: turn.ID, CreatedAt: "2026-06-07T10:00:00Z"},
		{Sequence: 12, TurnID: turn.ID, CreatedAt: "2026-06-07T10:00:02Z"},
	}
	diag := buildRuntimeTurnDiagnostics(turn, messages, toolCalls, permissions, events)
	summary := buildRuntimeInterruptedSummary(turn, diag, toolCalls)
	if summary == nil {
		t.Fatal("interrupted summary missing")
	}

	if summary.TurnID != turn.ID || summary.SessionID != turn.SessionID || summary.Status != turnStatusInterrupted {
		t.Fatalf("identity = %#v", summary)
	}
	if summary.InterruptedAt != turn.FinishedAt || summary.DurationMS != 1600 || summary.Source != "runtime_recovery" {
		t.Fatalf("time/source = %#v", summary)
	}
	if summary.LastCompletedTool.ID != "tool-write" || summary.LastCompletedTool.Target != producedPath {
		t.Fatalf("last completed = %#v", summary.LastCompletedTool)
	}
	if summary.LastFailedTool.ID != "tool-shell" || summary.LastFailedTool.Command != "go test ./missing" || summary.LastFailedTool.WorkingDir != dir || summary.LastFailedTool.ExitCode == nil || *summary.LastFailedTool.ExitCode != 7 {
		t.Fatalf("last failed = %#v", summary.LastFailedTool)
	}
	if summary.PendingTool.ID != "tool-running" || !slices.Contains(summary.PendingTool.ArtifactRefs, producedPath) {
		t.Fatalf("pending tool = %#v", summary.PendingTool)
	}
	if !slices.Contains(summary.ProducedArtifacts, producedPath) || !slices.Contains(summary.VerifiedArtifacts, producedPath) || !slices.Contains(summary.MissingArtifacts, missingPath) {
		t.Fatalf("artifact summary = %#v", summary)
	}
	if summary.PermissionCounts.Pending != 1 || summary.PermissionCounts.Denied != 1 {
		t.Fatalf("permission summary = %#v", summary.PermissionCounts)
	}
	if summary.CancelledToolCount != 1 || summary.NonzeroExitShellCount != 1 || summary.LastRuntimeEventSequence != 12 {
		t.Fatalf("signals/event = %#v", summary)
	}
	if !strings.Contains(summary.SummaryText, "Pending tool at interruption") || !strings.Contains(summary.SummaryText, "Missing artifacts") {
		t.Fatalf("summary text = %q", summary.SummaryText)
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

func TestRuntimeSessionActivityRestoresInterruptedSummaryWithoutStaleRunningTool(t *testing.T) {
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

	sess, err := runtimeBackend.CreateSession(context.Background(), workspace.ID, "interrupted")
	if err != nil {
		t.Fatal(err)
	}
	ws, err := runtimeBackend.GetWorkspace(workspace.ID)
	if err != nil {
		t.Fatal(err)
	}
	expectedPath := filepath.Join(workspace.Path, "tmp", "runtime-dev", "interrupted-summary.md")
	userMessage, err := ws.Messages.Create(context.Background(), sess.ID, message.CreateMessageParams{
		Role:  message.User,
		Parts: []message.ContentPart{message.TextContent{Text: "write " + expectedPath}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := service.turns.Upsert(context.Background(), RuntimeTurn{
		ID:            "turn-interrupted-activity",
		SessionID:     sess.ID,
		Status:        turnStatusRunning,
		UserMessageID: userMessage.ID,
		PromptPreview: "write " + expectedPath,
		StartedAt:     time.Now().Add(-2 * time.Second).UnixMilli(),
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := service.toolCalls.CreateCall(context.Background(), scheduler.ToolCallRequest{
		ID:           "tool-completed",
		SessionID:    sess.ID,
		TurnID:       "turn-interrupted-activity",
		Name:         "write",
		Source:       scheduler.ToolSourceBuiltin,
		InputSummary: `{"file_path":"` + expectedPath + `","content":"ok"}`,
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := service.toolCalls.CompleteCall(context.Background(), scheduler.ToolCallResult{
		ToolCallID:   "tool-completed",
		Status:       scheduler.ToolCallCompleted,
		ArtifactRefs: []string{expectedPath},
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := service.toolCalls.CreateCall(context.Background(), scheduler.ToolCallRequest{
		ID:        "tool-running",
		SessionID: sess.ID,
		TurnID:    "turn-interrupted-activity",
		Name:      "bash",
		Source:    scheduler.ToolSourceShell,
		Command:   "Start-Sleep -Seconds 60",
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := service.permissionStore.Upsert(context.Background(), RuntimePermissionRequest{
		ID:         "perm-denied",
		SessionID:  sess.ID,
		TurnID:     "turn-interrupted-activity",
		ToolCallID: "tool-running",
		ToolName:   "bash",
		Action:     "execute",
		Status:     permissionStatusDenied,
		CreatedAt:  time.Now().UnixMilli(),
	}); err != nil {
		t.Fatal(err)
	}

	interrupted, err := service.turns.InterruptUnfinished(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	cancelled, err := cancelUnfinishedRuntimeToolCalls(context.Background(), service.toolCalls, conn)
	if err != nil {
		t.Fatal(err)
	}
	for _, turn := range interrupted {
		service.storeRuntimeEvent(RuntimeEvent{
			Type:      "turn.interrupted",
			SessionID: turn.SessionID,
			TurnID:    turn.ID,
			CreatedAt: time.Now().UTC().Format(time.RFC3339Nano),
		})
	}
	for _, call := range cancelled {
		service.storeRuntimeEvent(runtimeToolCallEvent("tool.call.cancelled", call, map[string]any{"summary": "runtime restarted"}))
	}

	activity, err := service.SessionActivity(context.Background(), sess.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(activity.Turns) != 1 || activity.Turns[0].Status != turnStatusInterrupted {
		t.Fatalf("turns = %#v", activity.Turns)
	}
	var runningTools []RuntimeToolCall
	for _, call := range activity.ToolCalls {
		if call.Status == "running" || call.Status == "pending" || call.Status == "waiting_permission" {
			runningTools = append(runningTools, call)
		}
	}
	if len(runningTools) != 0 {
		t.Fatalf("stale running tools restored: %#v", runningTools)
	}
	summary := activity.Turns[0].Interrupted
	if summary == nil {
		t.Fatal("interrupted summary missing")
	}
	if summary.TurnID != "turn-interrupted-activity" || summary.PendingTool.ID != "tool-running" || summary.PendingTool.Status != "cancelled" {
		t.Fatalf("interrupted summary = %#v", summary)
	}
	if summary.LastCompletedTool.ID != "tool-completed" || summary.PermissionCounts.Denied != 1 || summary.CancelledToolCount != 1 {
		t.Fatalf("summary details = %#v", summary)
	}
}
