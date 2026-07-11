package runtime

import (
	"context"
	"errors"
	"math"
	"testing"
	"time"

	"charm.land/fantasy"
	"github.com/CIPFZ/agent-builder/internal/agent"
	"github.com/CIPFZ/agent-builder/internal/apitypes"
	"github.com/CIPFZ/agent-builder/internal/config"
	"github.com/CIPFZ/agent-builder/internal/db"
	"github.com/CIPFZ/agent-builder/internal/message"
)

// newContextGovernanceTestService wires a real runtimeService against a
// fresh workbench/workspace, mirroring the setup used by the context usage
// tests in runtime_context_usage_test.go.
func newContextGovernanceTestService(t *testing.T) (*runtimeService, string) {
	t.Helper()
	// SaveContextGovernanceSettings writes through config.ConfigStore to the
	// real global config JSON file and triggers a reload. Point that at an
	// isolated temp directory so the test never touches (or is polluted by)
	// the developer machine's actual ~/.local/share/agent-builder config.
	t.Setenv("AGENT_BUILDER_GLOBAL_DATA", t.TempDir())
	runtimeWorkbench, workspace := workbenchForSkillTest(t)
	service := newRuntimeService()
	service.runtime = runtimeWorkbench
	service.workspace = &apitypes.Workspace{ID: workspace.ID, Path: workspace.Path}
	conn, err := db.Connect(context.Background(), workspace.Config.Options.DataDirectory)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Release(workspace.Config.Options.DataDirectory) })
	service.turns = newRuntimeTurnStore(conn)
	session, err := runtimeWorkbench.CreateSession(context.Background(), workspace.ID, "context governance")
	if err != nil {
		t.Fatal(err)
	}
	service.sessionID = session.ID
	return service, session.ID
}

// Not t.Parallel(): newContextGovernanceTestService uses t.Setenv to
// sandbox the global config path, which the testing package forbids
// combining with parallel subtests.
func TestContextGovernanceSettingsRoundTrip(t *testing.T) {
	ctx := context.Background()
	service, _ := newContextGovernanceTestService(t)

	initial, err := service.ContextGovernanceSettings(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if initial.Settings.AutoCompactEnabled != nil || initial.Settings.AutoCompactPercent != nil {
		t.Fatalf("expected empty settings before any save, got %#v", initial.Settings)
	}

	pct := 0.6
	disabled := false
	keepRecent := 8
	saved, err := service.SaveContextGovernanceSettings(ctx, RuntimeContextGovernanceSettings{
		AutoCompactEnabled:     &disabled,
		AutoCompactPercent:     &pct,
		MicrocompactKeepRecent: keepRecent,
		SummaryModel:           "small",
	})
	if err != nil {
		t.Fatal(err)
	}
	if saved.Settings.AutoCompactEnabled == nil || *saved.Settings.AutoCompactEnabled != false {
		t.Fatalf("save response autoCompactEnabled = %#v", saved.Settings.AutoCompactEnabled)
	}

	// GET after PUT must reflect exactly what was saved (end-to-end
	// persistence, not just an echo of the write request).
	reloaded, err := service.ContextGovernanceSettings(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if reloaded.Settings.AutoCompactEnabled == nil || *reloaded.Settings.AutoCompactEnabled != false {
		t.Fatalf("reloaded autoCompactEnabled = %#v", reloaded.Settings.AutoCompactEnabled)
	}
	if reloaded.Settings.AutoCompactPercent == nil || *reloaded.Settings.AutoCompactPercent != 0.6 {
		t.Fatalf("reloaded autoCompactPercent = %#v", reloaded.Settings.AutoCompactPercent)
	}
	if reloaded.Settings.MicrocompactKeepRecent != 8 {
		t.Fatalf("reloaded microcompactKeepRecent = %d, want 8", reloaded.Settings.MicrocompactKeepRecent)
	}
	if reloaded.Settings.SummaryModel != "small" {
		t.Fatalf("reloaded summaryModel = %q, want small", reloaded.Settings.SummaryModel)
	}
}

func TestSaveContextGovernanceSettingsRejectsInvalidPercent(t *testing.T) {
	ctx := context.Background()
	service, _ := newContextGovernanceTestService(t)

	badPct := 3.0
	_, err := service.SaveContextGovernanceSettings(ctx, RuntimeContextGovernanceSettings{AutoCompactPercent: &badPct})
	if err == nil {
		t.Fatal("expected validation error for out-of-range percent")
	}
	if !errors.Is(err, config.ErrContextGovernanceInvalid) {
		t.Fatalf("error = %v, want it to wrap config.ErrContextGovernanceInvalid", err)
	}
}

func TestContextGovernanceForAppliesSavedGlobalDefaults(t *testing.T) {
	ctx := context.Background()
	service, sessionID := newContextGovernanceTestService(t)

	// Before any save, the accessor returns the documented defaults.
	gov := service.contextGovernanceFor(ctx, sessionID, "gpt-4o")
	if !gov.AutoCompactEnabled || gov.AutoCompactPercent != nil || !gov.MicrocompactEnabled || gov.MicrocompactKeepRecent != config.DefaultContextGovernanceMicrocompactKeepRecent {
		t.Fatalf("default resolved governance = %#v", gov)
	}

	pct := 0.4
	if _, err := service.SaveContextGovernanceSettings(ctx, RuntimeContextGovernanceSettings{AutoCompactPercent: &pct}); err != nil {
		t.Fatal(err)
	}
	gov = service.contextGovernanceFor(ctx, sessionID, "gpt-4o")
	if gov.AutoCompactPercent == nil || *gov.AutoCompactPercent != 0.4 {
		t.Fatalf("resolved governance after save = %#v", gov)
	}
}

// TestComputeContextUsagePercentOverrideLowersAutoCompactAt is the WP3
// acceptance case: an autoCompactPercent override moves the trigger point,
// and it can only pull it earlier than the formula-derived value.
func TestComputeContextUsagePercentOverrideLowersAutoCompactAt(t *testing.T) {
	ctx := context.Background()
	service, sessionID := newContextGovernanceTestService(t)

	before, err := service.computeContextUsage(ctx, sessionID, "", "gpt-4o", 0, nil)
	if err != nil {
		t.Fatal(err)
	}

	pct := 0.05
	if _, err := service.SaveContextGovernanceSettings(ctx, RuntimeContextGovernanceSettings{AutoCompactPercent: &pct}); err != nil {
		t.Fatal(err)
	}

	after, err := service.computeContextUsage(ctx, sessionID, "", "gpt-4o", 0, nil)
	if err != nil {
		t.Fatal(err)
	}
	if after.AutoCompactAt >= before.AutoCompactAt {
		t.Fatalf("expected percent override to lower autoCompactAt: before=%d after=%d", before.AutoCompactAt, after.AutoCompactAt)
	}
	wantAt := int(math.Round(float64(after.ContextWindow) * 0.05))
	if after.AutoCompactAt != wantAt {
		t.Fatalf("autoCompactAt = %d, want %d (window=%d)", after.AutoCompactAt, wantAt, after.ContextWindow)
	}
}

// TestBuildModelInputProjectionSkipsAutoCompactWhenDisabled is the WP3
// acceptance case: turning off autoCompactEnabled must stop auto-compaction
// even once usage crosses the threshold, while the usage/warning level
// still reflects the real (uncompacted) state so the UI can show the
// "please compact manually" copy.
func TestBuildModelInputProjectionSkipsAutoCompactWhenDisabled(t *testing.T) {
	ctx := context.Background()
	service, sessionID := newContextGovernanceTestService(t)

	disabled := false
	if _, err := service.SaveContextGovernanceSettings(ctx, RuntimeContextGovernanceSettings{AutoCompactEnabled: &disabled}); err != nil {
		t.Fatal(err)
	}

	runtimeWorkbench := service.runtime
	ws, err := runtimeWorkbench.GetWorkspace(service.workspace.ID)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := ws.Messages.Create(ctx, sessionID, message.CreateMessageParams{
		Role:  message.User,
		Parts: []message.ContentPart{message.TextContent{Text: "start a long task"}},
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := ws.Messages.Create(ctx, sessionID, message.CreateMessageParams{
		Role:  message.Assistant,
		Parts: []message.ContentPart{message.TextContent{Text: "large context anchor"}, message.Finish{Reason: message.FinishReasonEndTurn, Time: time.Now().Unix()}},
		Usage: message.Usage{InputTokens: 120000, OutputTokens: 12000},
	}); err != nil {
		t.Fatal(err)
	}

	result, err := service.buildModelInputProjection(ctx, agent.ModelInputSnapshot{
		SessionID: sessionID,
		TurnID:    "turn-disabled",
		Step:      1,
		Model:     "gpt-4o",
		Messages: []fantasy.Message{
			fantasy.NewUserMessage("start a long task"),
			{Role: fantasy.MessageRoleAssistant, Content: []fantasy.MessagePart{fantasy.TextPart{Text: "large context anchor"}}},
			fantasy.NewUserMessage("continue"),
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Messages) == 0 {
		t.Fatalf("projection = %#v", result)
	}

	updated, err := runtimeWorkbench.GetSession(ctx, service.workspace.ID, sessionID)
	if err != nil {
		t.Fatal(err)
	}
	if updated.SummaryMessageID != "" {
		t.Fatalf("auto compact must not run while disabled, got summary message id %q", updated.SummaryMessageID)
	}

	conn, err := service.workspaceDB(ctx)
	if err != nil {
		t.Fatal(err)
	}
	var boundaries int
	if err := conn.QueryRowContext(ctx, `SELECT COUNT(*) FROM runtime_context_boundaries WHERE session_id = ?`, sessionID).Scan(&boundaries); err != nil {
		t.Fatal(err)
	}
	if boundaries != 0 {
		t.Fatalf("expected no compact boundaries while auto compact is disabled, got %d", boundaries)
	}

	usage, err := service.computeContextUsage(ctx, sessionID, "turn-disabled", "gpt-4o", 0, nil)
	if err != nil {
		t.Fatal(err)
	}
	if usage.AutoCompactEnabled {
		t.Fatal("usage.AutoCompactEnabled must be false once the setting is disabled")
	}
	if usage.Level == contextUsageLevelOK {
		t.Fatalf("expected a non-ok warning level once usage exceeds the threshold, got %#v", usage)
	}
}
