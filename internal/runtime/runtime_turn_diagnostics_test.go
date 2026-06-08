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

func TestRuntimeInterruptedSummaryPhase51PermissionAndShellSignals(t *testing.T) {
	t.Parallel()

	dir := phase51RuntimeDevDir(t)
	exit := 9
	turn := RuntimeTurn{
		ID:         "turn-phase51-signals",
		SessionID:  "session-phase51",
		Status:     turnStatusInterrupted,
		StartedAt:  1000,
		FinishedAt: 2200,
		Error:      "runtime restarted before permission or shell completion",
	}
	toolCalls := []RuntimeToolCall{
		{
			ID:         "tool-pending-permission",
			TurnID:     turn.ID,
			Name:       "write",
			Source:     "builtin",
			Status:     string(scheduler.ToolCallCancelled),
			StartedAt:  1100,
			FinishedAt: 2200,
			Display: RuntimeToolCallDisplay{
				Kind:          "file_write",
				Title:         "Write pending file",
				PrimaryTarget: filepath.Join(dir, "pending-permission.md"),
			},
		},
		{
			ID:         "tool-denied-permission",
			TurnID:     turn.ID,
			Name:       "bash",
			Source:     "shell",
			Status:     string(scheduler.ToolCallDenied),
			StartedAt:  1200,
			FinishedAt: 1300,
			Display: RuntimeToolCallDisplay{
				Kind:          "shell",
				Title:         "Run denied command",
				Command:       "Remove-Item blocked.txt",
				WorkingDir:    dir,
				FailureReason: "permission denied",
			},
		},
		{
			ID:         "tool-nonzero-shell",
			TurnID:     turn.ID,
			Name:       "bash",
			Source:     "shell",
			Status:     string(scheduler.ToolCallCompleted),
			StartedAt:  1400,
			FinishedAt: 1500,
			ExitCode:   9,
			Stderr:     "phase51 shell failed",
			Display: RuntimeToolCallDisplay{
				Kind:          "shell",
				Title:         "Run failing command",
				Command:       "go test ./phase51",
				WorkingDir:    dir,
				ExitCode:      &exit,
				StderrExcerpt: "phase51 shell failed",
				FailureReason: "phase51 shell failed",
			},
		},
	}
	permissions := []RuntimePermissionRequest{
		{ID: "perm-pending", TurnID: turn.ID, ToolCallID: "tool-pending-permission", Status: permissionStatusExpired},
		{ID: "perm-denied", TurnID: turn.ID, ToolCallID: "tool-denied-permission", Status: permissionStatusDenied},
	}

	diag := buildRuntimeTurnDiagnostics(turn, nil, toolCalls, permissions, nil)
	summary := buildRuntimeInterruptedSummary(turn, diag, toolCalls)
	if summary == nil {
		t.Fatal("interrupted summary missing")
	}
	if summary.PendingTool.ID != "tool-pending-permission" || summary.PendingTool.Status != string(scheduler.ToolCallCancelled) {
		t.Fatalf("pending permission recovery tool = %#v", summary.PendingTool)
	}
	if summary.PermissionCounts.Pending != 1 || summary.PermissionCounts.Expired != 1 || summary.PermissionCounts.Denied != 1 {
		t.Fatalf("permission signals = %#v", summary.PermissionCounts)
	}
	if summary.DeniedToolCount != 1 || summary.CancelledToolCount != 1 || summary.NonzeroExitShellCount != 1 {
		t.Fatalf("tool signals = %#v", summary)
	}
	if summary.LastFailedTool.ID != "tool-nonzero-shell" || summary.LastFailedTool.ExitCode == nil || *summary.LastFailedTool.ExitCode != 9 {
		t.Fatalf("nonzero shell recovery = %#v", summary.LastFailedTool)
	}
	if !strings.Contains(summary.SummaryText, "Permissions: pending 1, denied 1") || !strings.Contains(summary.SummaryText, "nonzero shell 1") {
		t.Fatalf("summary text does not explain signals: %q", summary.SummaryText)
	}
}

func TestRuntimeInterruptedPermissionLifecycleDiagnosticsAreComputed(t *testing.T) {
	t.Parallel()

	permissions := []RuntimePermissionRequest{
		{ID: "perm-expired", TurnID: "turn-interrupted", Status: permissionStatusExpired},
		{ID: "perm-cancelled", TurnID: "turn-interrupted", Status: permissionStatusCancelled},
		{ID: "perm-denied", TurnID: "turn-interrupted", Status: permissionStatusDenied},
	}
	interrupted := RuntimeTurn{ID: "turn-interrupted", SessionID: "session-1", Status: turnStatusInterrupted}

	diag := buildRuntimeTurnDiagnostics(interrupted, nil, nil, permissions, nil)
	if diag.PermissionCounts.Pending != 2 || diag.PermissionCounts.Expired != 1 || diag.PermissionCounts.Cancelled != 1 || diag.PermissionCounts.Denied != 1 {
		t.Fatalf("interrupted permission diagnostics = %#v", diag.PermissionCounts)
	}

	completed := RuntimeTurn{ID: "turn-interrupted", SessionID: "session-1", Status: turnStatusCompleted}
	diag = buildRuntimeTurnDiagnostics(completed, nil, nil, permissions, nil)
	if diag.PermissionCounts.Pending != 0 || diag.PermissionCounts.Expired != 1 || diag.PermissionCounts.Cancelled != 1 || diag.PermissionCounts.Denied != 1 {
		t.Fatalf("terminal permission diagnostics should not restore pending gates: %#v", diag.PermissionCounts)
	}
}

func TestRuntimeInterruptedSummaryPhase51StructuredRefsOnly(t *testing.T) {
	t.Parallel()

	dir := phase51RuntimeDevDir(t)
	structuredPath := filepath.Join(dir, "structured-mcp-artifact.json")
	prosePath := filepath.Join(dir, "prose-should-not-count.json")
	turn := RuntimeTurn{
		ID:         "turn-phase51-structured",
		SessionID:  "session-phase51",
		Status:     turnStatusInterrupted,
		StartedAt:  1000,
		FinishedAt: 2000,
		Error:      "runtime restarted before final assistant response",
	}
	toolCalls := []RuntimeToolCall{{
		ID:           "tool-custom-structured",
		TurnID:       turn.ID,
		Name:         "custom_artifact_writer",
		Source:       "mcp",
		Status:       string(scheduler.ToolCallCompleted),
		StartedAt:    1100,
		FinishedAt:   1200,
		Structured:   `{"artifact_refs":[{"path":"` + strings.ReplaceAll(structuredPath, `\`, `\\`) + `","target":"phase51-report","display":{"label":"Phase 5.1 report"}}]}`,
		ModelContent: "Assistant prose claims an artifact exists at " + prosePath,
		Display: RuntimeToolCallDisplay{
			Kind:          "generic",
			Title:         "Custom structured artifact",
			Detail:        "structured refs only",
			PrimaryTarget: structuredPath,
			Target:        structuredPath,
			Targets:       []string{structuredPath},
			ArtifactRefs:  []string{structuredPath},
		},
	}}

	diag := buildRuntimeTurnDiagnostics(turn, nil, toolCalls, nil, nil)
	summary := buildRuntimeInterruptedSummary(turn, diag, toolCalls)
	if summary == nil {
		t.Fatal("interrupted summary missing")
	}
	if !slices.Contains(summary.ProducedArtifacts, structuredPath) {
		t.Fatalf("structured artifact ref not preserved: %#v", summary.ProducedArtifacts)
	}
	if slices.Contains(summary.ProducedArtifacts, prosePath) {
		t.Fatalf("assistant prose path was incorrectly trusted: %#v", summary.ProducedArtifacts)
	}
	if summary.ArtifactCounts.StructuredRefs != 1 || diag.ArtifactConfidenceSummary.StructuredMCPCustomRefs != 1 {
		t.Fatalf("structured confidence = diag %#v summary %#v", diag.ArtifactConfidenceSummary, summary.ArtifactCounts)
	}
	if summary.LastCompletedTool.Target != structuredPath || !slices.Contains(summary.LastCompletedTool.ArtifactRefs, structuredPath) {
		t.Fatalf("interrupted tool metadata = %#v", summary.LastCompletedTool)
	}
	if summary.LastCompletedTool.Display.Title != "Custom structured artifact" || summary.LastCompletedTool.Display.Detail != "structured refs only" {
		t.Fatalf("display metadata not retained: %#v", summary.LastCompletedTool.Display)
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

	turnActivity, err := service.TurnActivity(context.Background(), "turn-diagnostics")
	if err != nil {
		t.Fatal(err)
	}
	assertNarrowActivityMatchesFullTurn(t, activity, RuntimeSessionActivityWindowResponse{
		SessionID:   turnActivity.SessionID,
		Messages:    turnActivity.Messages,
		Turns:       turnActivity.Turns,
		ToolCalls:   turnActivity.ToolCalls,
		Permissions: turnActivity.Permissions,
		Events:      turnActivity.Events,
		Policy:      turnActivity.Policy,
	}, "turn-diagnostics")

	window, err := service.SessionActivityWindow(context.Background(), sess.ID, 1)
	if err != nil {
		t.Fatal(err)
	}
	assertNarrowActivityMatchesFullTurn(t, activity, window, "turn-diagnostics")
}

func assertNarrowActivityMatchesFullTurn(t *testing.T, full RuntimeSessionActivityResponse, narrow RuntimeSessionActivityWindowResponse, turnID string) {
	t.Helper()
	fullTurn := findRuntimeTurn(full.Turns, turnID)
	narrowTurn := findRuntimeTurn(narrow.Turns, turnID)
	if fullTurn.ID == "" || narrowTurn.ID == "" {
		t.Fatalf("turn %q full=%#v narrow=%#v", turnID, full.Turns, narrow.Turns)
	}
	if narrowTurn.Diagnostics.Warning != fullTurn.Diagnostics.Warning ||
		!slices.Equal(narrowTurn.Diagnostics.MissingArtifacts, fullTurn.Diagnostics.MissingArtifacts) ||
		narrowTurn.Diagnostics.PermissionCounts != fullTurn.Diagnostics.PermissionCounts ||
		narrowTurn.Diagnostics.LastRuntimeEventSequence != fullTurn.Diagnostics.LastRuntimeEventSequence {
		t.Fatalf("diagnostics mismatch full=%#v narrow=%#v", fullTurn.Diagnostics, narrowTurn.Diagnostics)
	}
	if len(narrow.Messages) != 1 || narrow.Messages[0].ID != fullTurn.UserMessageID {
		t.Fatalf("narrow messages = %#v, want user message %q", narrow.Messages, fullTurn.UserMessageID)
	}
	if len(narrow.Permissions) != 1 || narrow.Permissions[0].Status != permissionStatusDenied || narrow.Permissions[0].TurnID != turnID {
		t.Fatalf("terminal permission evidence mismatch: %#v", narrow.Permissions)
	}
	if len(narrow.Events) != 1 || narrow.Events[0].TurnID != turnID || narrow.Events[0].Sequence != fullTurn.Diagnostics.LastRuntimeEventSequence {
		t.Fatalf("events mismatch: %#v", narrow.Events)
	}
	if len(narrow.ToolCalls) != 0 {
		t.Fatalf("tool calls mismatch: %#v", narrow.ToolCalls)
	}
}

func TestRuntimeSessionActivityCursorWindowPreservesMixedEvidenceParity(t *testing.T) {
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

	sess, err := runtimeBackend.CreateSession(context.Background(), workspace.ID, "cursor-window")
	if err != nil {
		t.Fatal(err)
	}
	ws, err := runtimeBackend.GetWorkspace(workspace.ID)
	if err != nil {
		t.Fatal(err)
	}
	oldMessage, err := ws.Messages.Create(context.Background(), sess.ID, message.CreateMessageParams{Role: message.User, Parts: []message.ContentPart{message.TextContent{Text: "old turn"}}})
	if err != nil {
		t.Fatal(err)
	}
	newArtifact := filepath.Join(workspace.Path, "tmp", "runtime-dev", "cursor-window.md")
	newMessage, err := ws.Messages.Create(context.Background(), sess.ID, message.CreateMessageParams{Role: message.User, Parts: []message.ContentPart{message.TextContent{Text: "write " + newArtifact}}})
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now()
	if _, err := service.turns.Upsert(context.Background(), RuntimeTurn{ID: "turn-old", SessionID: sess.ID, Status: turnStatusCompleted, UserMessageID: oldMessage.ID, PromptPreview: "old turn", StartedAt: now.Add(-2 * time.Minute).UnixMilli(), FinishedAt: now.Add(-90 * time.Second).UnixMilli()}); err != nil {
		t.Fatal(err)
	}
	if _, err := service.turns.Upsert(context.Background(), RuntimeTurn{ID: "turn-new", SessionID: sess.ID, Status: turnStatusInterrupted, UserMessageID: newMessage.ID, PromptPreview: "write " + newArtifact, StartedAt: now.Add(-30 * time.Second).UnixMilli(), FinishedAt: now.Add(-5 * time.Second).UnixMilli()}); err != nil {
		t.Fatal(err)
	}
	if _, err := service.toolCalls.CreateCall(context.Background(), scheduler.ToolCallRequest{ID: "tool-old", SessionID: sess.ID, TurnID: "turn-old", Name: "read", Source: scheduler.ToolSourceBuiltin}); err != nil {
		t.Fatal(err)
	}
	if _, err := service.toolCalls.CompleteCall(context.Background(), scheduler.ToolCallResult{ToolCallID: "tool-old", Status: scheduler.ToolCallCompleted, OutputSummary: "old"}); err != nil {
		t.Fatal(err)
	}
	if _, err := service.toolCalls.CreateCall(context.Background(), scheduler.ToolCallRequest{ID: "tool-new", SessionID: sess.ID, TurnID: "turn-new", Name: "write", Source: scheduler.ToolSourceBuiltin, InputSummary: `{"file_path":"` + newArtifact + `"}`}); err != nil {
		t.Fatal(err)
	}
	if _, err := service.toolCalls.CompleteCall(context.Background(), scheduler.ToolCallResult{ToolCallID: "tool-new", Status: scheduler.ToolCallCompleted, OutputSummary: "wrote", ArtifactRefs: []string{newArtifact}}); err != nil {
		t.Fatal(err)
	}
	if _, err := service.permissionStore.Upsert(context.Background(), RuntimePermissionRequest{ID: "perm-old", SessionID: sess.ID, TurnID: "turn-old", ToolCallID: "tool-old", ToolName: "read", Action: "read", Status: permissionStatusAllowedOnce, CreatedAt: now.Add(-110 * time.Second).UnixMilli(), DecidedAt: now.Add(-100 * time.Second).UnixMilli()}); err != nil {
		t.Fatal(err)
	}
	if _, err := service.permissionStore.Upsert(context.Background(), RuntimePermissionRequest{ID: "perm-new", SessionID: sess.ID, TurnID: "turn-new", ToolCallID: "tool-new", ToolName: "write", Action: "write", Status: permissionStatusCancelled, CreatedAt: now.Add(-20 * time.Second).UnixMilli(), DecidedAt: now.Add(-4 * time.Second).UnixMilli()}); err != nil {
		t.Fatal(err)
	}
	service.storeRuntimeEvent(RuntimeEvent{Type: "turn.started", SessionID: sess.ID, TurnID: "turn-old", CreatedAt: now.Add(-2 * time.Minute).UTC().Format(time.RFC3339Nano)})
	service.storeRuntimeEvent(RuntimeEvent{Type: "tool.call.completed", SessionID: sess.ID, TurnID: "turn-new", ToolCallID: "tool-new", CreatedAt: now.Add(-3 * time.Second).UTC().Format(time.RFC3339Nano)})

	full, err := service.SessionActivity(context.Background(), sess.ID)
	if err != nil {
		t.Fatal(err)
	}
	window, err := service.SessionActivityCursorWindow(context.Background(), sess.ID, "", 4)
	if err != nil {
		t.Fatal(err)
	}
	if window.Window.LastCursor == "" || !window.Window.HasMoreBefore || window.Window.EvidenceCount < 6 {
		t.Fatalf("window metadata = %#v", window.Window)
	}
	assertActivitySubsetMatchesFullTurn(t, full, window, "turn-new")

	previous, err := service.SessionActivityCursorWindow(context.Background(), sess.ID, window.Window.FirstCursor, 20)
	if err != nil {
		t.Fatal(err)
	}
	if previous.Window.LastCursor == "" || !previous.Window.HasMoreAfter {
		t.Fatalf("previous window metadata = %#v", previous.Window)
	}
	if findRuntimeTurn(previous.Turns, "turn-old").ID == "" {
		t.Fatalf("cursor window did not hydrate earlier mixed evidence: %#v", previous.Turns)
	}
}

func assertActivitySubsetMatchesFullTurn(t *testing.T, full RuntimeSessionActivityResponse, narrow RuntimeSessionActivityWindowResponse, turnID string) {
	t.Helper()
	fullTurn := findRuntimeTurn(full.Turns, turnID)
	narrowTurn := findRuntimeTurn(narrow.Turns, turnID)
	if fullTurn.ID == "" || narrowTurn.ID == "" {
		t.Fatalf("turn %q full=%#v narrow=%#v", turnID, full.Turns, narrow.Turns)
	}
	if narrowTurn.Diagnostics.Warning != fullTurn.Diagnostics.Warning ||
		!slices.Equal(narrowTurn.Diagnostics.ProducedArtifacts, fullTurn.Diagnostics.ProducedArtifacts) ||
		narrowTurn.Diagnostics.ArtifactCounts != fullTurn.Diagnostics.ArtifactCounts ||
		narrowTurn.Diagnostics.PermissionCounts != fullTurn.Diagnostics.PermissionCounts ||
		narrowTurn.Diagnostics.LastRuntimeEventSequence != fullTurn.Diagnostics.LastRuntimeEventSequence {
		t.Fatalf("diagnostics mismatch full=%#v narrow=%#v", fullTurn.Diagnostics, narrowTurn.Diagnostics)
	}
	if len(narrowTurn.Diagnostics.ProducedArtifacts) == 0 {
		t.Fatalf("produced artifact diagnostics missing: %#v", narrowTurn.Diagnostics)
	}
	toolCall := RuntimeToolCall{}
	for _, call := range narrow.ToolCalls {
		if call.ID == "tool-new" {
			toolCall = call
			break
		}
	}
	if toolCall.ID == "" || !slices.Contains(toolCall.ArtifactRefs, narrowTurn.Diagnostics.ProducedArtifacts[0]) {
		t.Fatalf("tool artifact evidence mismatch: %#v diag=%#v", narrow.ToolCalls, narrowTurn.Diagnostics)
	}
	permission := RuntimePermissionRequest{}
	for _, perm := range narrow.Permissions {
		if perm.ID == "perm-new" {
			permission = perm
			break
		}
	}
	if permission.ID == "" || permission.Status != permissionStatusCancelled {
		t.Fatalf("terminal permission evidence mismatch: %#v", narrow.Permissions)
	}
	eventFound := false
	for _, event := range narrow.Events {
		if event.TurnID == turnID && event.ToolCallID == "tool-new" {
			eventFound = true
			break
		}
	}
	if !eventFound {
		t.Fatalf("event evidence mismatch: %#v", narrow.Events)
	}
}

func findRuntimeTurn(turns []RuntimeTurn, turnID string) RuntimeTurn {
	for _, turn := range turns {
		if turn.ID == turnID {
			return turn
		}
	}
	return RuntimeTurn{}
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

func phase51RuntimeDevDir(t *testing.T) string {
	t.Helper()
	name := strings.NewReplacer("/", "-", "\\", "-", " ", "-").Replace(t.Name())
	dir, err := filepath.Abs(filepath.Join("..", "..", "tmp", "runtime-dev", "phase51-tests", name))
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	return dir
}
