package runtime

import (
	"context"
	"strings"
	"testing"
	"time"

	"charm.land/fantasy"
	"github.com/CIPFZ/agent-builder/internal/agent"
	"github.com/CIPFZ/agent-builder/internal/apitypes"
	"github.com/CIPFZ/agent-builder/internal/contextmgr"
	"github.com/CIPFZ/agent-builder/internal/db"
	"github.com/CIPFZ/agent-builder/internal/message"
	"github.com/CIPFZ/agent-builder/internal/runtimeapi"
)

func isolateRuntimeDesktopDB(t *testing.T, service *runtimeService) {
	t.Helper()
	service.desktopDataDir = t.TempDir()
	t.Cleanup(service.releaseConfigDB)
}

func TestContextThresholdSnapshots(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name          string
		window        int
		maxOutput     int
		autoCompactAt int
		warningAt     int
		blockingAt    int
	}{
		{name: "200k", window: 200000, maxOutput: 20000, autoCompactAt: 167000, warningAt: 147000, blockingAt: 177000},
		{name: "1m", window: 1000000, maxOutput: 20000, autoCompactAt: 967000, warningAt: 947000, blockingAt: 977000},
		{name: "64k", window: 64000, maxOutput: 4096, autoCompactAt: 53504, warningAt: 47104, blockingAt: 58624},
	}
	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := contextThresholds(tt.window, tt.maxOutput, nil)
			if got.AutoCompactAt != tt.autoCompactAt || got.WarningAt != tt.warningAt || got.BlockingAt != tt.blockingAt {
				t.Fatalf("thresholds = %#v", got)
			}
		})
	}
}

func TestContextUsageIgnoresPreBoundaryUsageAnchors(t *testing.T) {
	t.Parallel()

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

	session, err := runtimeWorkbench.CreateSession(context.Background(), workspace.ID, "context usage")
	if err != nil {
		t.Fatal(err)
	}
	ws, err := runtimeWorkbench.GetWorkspace(workspace.ID)
	if err != nil {
		t.Fatal(err)
	}
	oldText := strings.Repeat("old ", 20000)
	oldMessage, err := ws.Messages.Create(context.Background(), session.ID, message.CreateMessageParams{
		Role:  message.Assistant,
		Parts: []message.ContentPart{message.TextContent{Text: oldText}, message.Finish{Reason: message.FinishReasonEndTurn, Time: time.Now().Unix()}},
		Usage: message.Usage{InputTokens: 70000, OutputTokens: 10000},
	})
	if err != nil {
		t.Fatal(err)
	}
	summaryMessage, err := ws.Messages.Create(context.Background(), session.ID, message.CreateMessageParams{
		Role:             message.User,
		Parts:            []message.ContentPart{message.TextContent{Text: "compact summary"}},
		IsSummaryMessage: true,
		Metadata:         map[string]string{"synthetic": "true"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := conn.ExecContext(context.Background(), `
INSERT INTO runtime_context_boundaries (
	id, session_id, turn_id, projection_id, kind, trigger, status, summary_message_id, created_at, completed_at
) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
`, "boundary-1", session.ID, "turn-1", "projection-1", "full", "manual", "completed", summaryMessage.ID, int64(2000), int64(2000)); err != nil {
		t.Fatal(err)
	}
	anchorMessage, err := ws.Messages.Create(context.Background(), session.ID, message.CreateMessageParams{
		Role:  message.Assistant,
		Parts: []message.ContentPart{message.TextContent{Text: "anchor"}, message.Finish{Reason: message.FinishReasonEndTurn, Time: time.Now().Unix()}},
		Usage: message.Usage{InputTokens: 80, OutputTokens: 20},
	})
	if err != nil {
		t.Fatal(err)
	}
	nextMessage, err := ws.Messages.Create(context.Background(), session.ID, message.CreateMessageParams{
		Role:  message.User,
		Parts: []message.ContentPart{message.TextContent{Text: strings.Repeat("next", 40)}},
	})
	if err != nil {
		t.Fatal(err)
	}
	// Boundary selection is ordinal (summary message index), so identical
	// second-level timestamps on every message must not confuse the anchor:
	// force all messages to the exact same created_at.
	for _, id := range []string{oldMessage.ID, summaryMessage.ID, anchorMessage.ID, nextMessage.ID} {
		if _, err := conn.ExecContext(context.Background(), `UPDATE messages SET created_at = ?, updated_at = ? WHERE id = ?`, int64(1000), int64(1000), id); err != nil {
			t.Fatal(err)
		}
	}

	usage, err := service.computeContextUsage(context.Background(), session.ID, "turn-2", "test-model", 0, nil)
	if err != nil {
		t.Fatal(err)
	}
	if usage.Estimated {
		t.Fatalf("expected real usage anchor: %#v", usage)
	}
	if usage.CompactCount != 1 {
		t.Fatalf("compact count = %d", usage.CompactCount)
	}
	if usage.UsedTokens >= 70000 {
		t.Fatalf("pre-boundary usage leaked into context usage: %#v", usage)
	}
	if usage.UsedTokens < 100 {
		t.Fatalf("post-boundary anchor missing from context usage: %#v", usage)
	}
}

func TestContextUsageEstimatedWithoutRealUsageAnchor(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	runtimeWorkbench, workspace := workbenchForSkillTest(t)
	service := newRuntimeService()
	service.runtime = runtimeWorkbench
	service.workspace = &apitypes.Workspace{ID: workspace.ID, Path: workspace.Path}
	conn, err := db.Connect(ctx, workspace.Config.Options.DataDirectory)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Release(workspace.Config.Options.DataDirectory) })
	service.turns = newRuntimeTurnStore(conn)

	session, err := runtimeWorkbench.CreateSession(ctx, workspace.ID, "estimated usage")
	if err != nil {
		t.Fatal(err)
	}
	ws, err := runtimeWorkbench.GetWorkspace(workspace.ID)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := ws.Messages.Create(ctx, session.ID, message.CreateMessageParams{
		Role:  message.User,
		Parts: []message.ContentPart{message.TextContent{Text: "hello"}},
	}); err != nil {
		t.Fatal(err)
	}
	// Assistant message without provider usage: the estimation fallback never
	// persists message usage, so this message must not act as an anchor.
	if _, err := ws.Messages.Create(ctx, session.ID, message.CreateMessageParams{
		Role:  message.Assistant,
		Parts: []message.ContentPart{message.TextContent{Text: "estimated reply"}, message.Finish{Reason: message.FinishReasonEndTurn, Time: time.Now().Unix()}},
	}); err != nil {
		t.Fatal(err)
	}

	usage, err := service.computeContextUsage(ctx, session.ID, "turn-1", "test-model", 0, nil)
	if err != nil {
		t.Fatal(err)
	}
	if !usage.Estimated {
		t.Fatalf("assistant without real usage must not be an anchor: %#v", usage)
	}
}

func TestManualCompactAppendsSummaryAndEvents(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	runtimeWorkbench, workspace := workbenchForSkillTest(t)
	service := newRuntimeService()
	installTestCompactSummaryGenerator(service)
	service.runtime = runtimeWorkbench
	service.workspace = &apitypes.Workspace{ID: workspace.ID, Path: workspace.Path}
	conn, err := db.Connect(ctx, workspace.Config.Options.DataDirectory)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Release(workspace.Config.Options.DataDirectory) })
	service.turns = newRuntimeTurnStore(conn)

	session, err := runtimeWorkbench.CreateSession(ctx, workspace.ID, "manual compact")
	if err != nil {
		t.Fatal(err)
	}
	service.sessionID = session.ID
	ws, err := runtimeWorkbench.GetWorkspace(workspace.ID)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := ws.Messages.Create(ctx, session.ID, message.CreateMessageParams{
		Role:  message.User,
		Parts: []message.ContentPart{message.TextContent{Text: "please keep the API route details"}},
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := ws.Messages.Create(ctx, session.ID, message.CreateMessageParams{
		Role:  message.Assistant,
		Parts: []message.ContentPart{message.TextContent{Text: "implemented the route and tests"}, message.Finish{Reason: message.FinishReasonEndTurn, Time: time.Now().Unix()}},
	}); err != nil {
		t.Fatal(err)
	}
	resp, err := service.ManualCompact(ctx, RuntimeContextActionRequest{
		SessionID:    session.ID,
		TurnID:       "turn-compact",
		Instructions: "保留 API route",
		ProjectionID: "projection-compact",
	})
	if err != nil {
		t.Fatal(err)
	}
	if resp.Boundary.Kind != "full" || resp.Boundary.Status != contextmgr.ProjectionStatusCompleted || resp.Boundary.SummaryMessageID == "" {
		t.Fatalf("boundary = %#v", resp.Boundary)
	}
	updated, err := runtimeWorkbench.GetSession(ctx, workspace.ID, session.ID)
	if err != nil {
		t.Fatal(err)
	}
	if updated.SummaryMessageID != resp.Boundary.SummaryMessageID {
		t.Fatalf("summary id = %q, boundary = %q", updated.SummaryMessageID, resp.Boundary.SummaryMessageID)
	}
	messages, err := ws.Messages.List(ctx, session.ID)
	if err != nil {
		t.Fatal(err)
	}
	var summaryCount int
	for _, msg := range messages {
		if msg.IsSummaryMessage {
			summaryCount++
			if msg.Role != message.User || !strings.Contains(msg.Content().Text, "保留 API route") {
				t.Fatalf("summary message = %#v", msg)
			}
		}
	}
	if summaryCount != 1 {
		t.Fatalf("summary messages = %d, all = %#v", summaryCount, messages)
	}
	events, err := service.Events(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if !hasRuntimeEventType(events.Events, runtimeapi.EventCompactStarted) || !hasRuntimeEventType(events.Events, runtimeapi.EventCompactCompleted) {
		t.Fatalf("compact events missing: %#v", events.Events)
	}
}

func TestBuildModelInputProjectionAutoCompactsAboveThreshold(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	runtimeWorkbench, workspace := workbenchForSkillTest(t)
	service := newRuntimeService()
	installTestCompactSummaryGenerator(service)
	service.runtime = runtimeWorkbench
	service.workspace = &apitypes.Workspace{ID: workspace.ID, Path: workspace.Path}
	conn, err := db.Connect(ctx, workspace.Config.Options.DataDirectory)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Release(workspace.Config.Options.DataDirectory) })

	session, err := runtimeWorkbench.CreateSession(ctx, workspace.ID, "auto compact")
	if err != nil {
		t.Fatal(err)
	}
	service.sessionID = session.ID
	ws, err := runtimeWorkbench.GetWorkspace(workspace.ID)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := ws.Messages.Create(ctx, session.ID, message.CreateMessageParams{
		Role:  message.User,
		Parts: []message.ContentPart{message.TextContent{Text: "start a long task"}},
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := ws.Messages.Create(ctx, session.ID, message.CreateMessageParams{
		Role:  message.Assistant,
		Parts: []message.ContentPart{message.TextContent{Text: "large context anchor"}, message.Finish{Reason: message.FinishReasonEndTurn, Time: time.Now().Unix()}},
		Usage: message.Usage{InputTokens: 170000, OutputTokens: 15000},
	}); err != nil {
		t.Fatal(err)
	}
	result, err := service.buildModelInputProjection(ctx, agent.ModelInputSnapshot{
		SessionID: session.ID,
		TurnID:    "turn-auto",
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
	if len(result.Messages) == 0 || result.ProjectedMessageCount == 0 {
		t.Fatalf("projection = %#v", result)
	}
	updated, err := runtimeWorkbench.GetSession(ctx, workspace.ID, session.ID)
	if err != nil {
		t.Fatal(err)
	}
	if updated.SummaryMessageID == "" {
		t.Fatal("expected auto compact to set session summary message")
	}
	events, err := service.Events(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if !hasRuntimeEventType(events.Events, runtimeapi.EventCompactStarted) || !hasRuntimeEventType(events.Events, runtimeapi.EventCompactCompleted) {
		t.Fatalf("compact events missing: %#v", events.Events)
	}
	var completed int
	if err := conn.QueryRowContext(ctx, `SELECT COUNT(*) FROM runtime_context_boundaries WHERE session_id = ? AND kind = 'full' AND trigger = 'auto' AND status = 'completed'`, session.ID).Scan(&completed); err != nil {
		t.Fatal(err)
	}
	if completed != 1 {
		t.Fatalf("completed auto boundaries = %d", completed)
	}
	var boundaryID string
	if err := conn.QueryRowContext(ctx, `SELECT id FROM runtime_context_boundaries WHERE session_id = ? AND kind = 'full' AND trigger = 'auto' AND status = 'completed'`, session.ID).Scan(&boundaryID); err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(boundaryID, "ctxbound_full_auto_") {
		t.Fatalf("auto boundary id = %q, want ctxbound_full_auto_ prefix", boundaryID)
	}
}

func TestBuildModelInputProjectionKeepsCompactAcrossSteps(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	runtimeWorkbench, workspace := workbenchForSkillTest(t)
	service := newRuntimeService()
	installTestCompactSummaryGenerator(service)
	service.runtime = runtimeWorkbench
	service.workspace = &apitypes.Workspace{ID: workspace.ID, Path: workspace.Path}
	conn, err := db.Connect(ctx, workspace.Config.Options.DataDirectory)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Release(workspace.Config.Options.DataDirectory) })
	service.turns = newRuntimeTurnStore(conn)

	session, err := runtimeWorkbench.CreateSession(ctx, workspace.ID, "multi step compact")
	if err != nil {
		t.Fatal(err)
	}
	service.sessionID = session.ID
	ws, err := runtimeWorkbench.GetWorkspace(workspace.ID)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := ws.Messages.Create(ctx, session.ID, message.CreateMessageParams{
		Role:  message.User,
		Parts: []message.ContentPart{message.TextContent{Text: "start a long task"}},
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := ws.Messages.Create(ctx, session.ID, message.CreateMessageParams{
		Role:  message.Assistant,
		Parts: []message.ContentPart{message.TextContent{Text: "large context anchor"}, message.Finish{Reason: message.FinishReasonEndTurn, Time: time.Now().Unix()}},
		Usage: message.Usage{InputTokens: 170000, OutputTokens: 15000},
	}); err != nil {
		t.Fatal(err)
	}

	history := []fantasy.Message{
		fantasy.NewUserMessage("history 1"),
		{Role: fantasy.MessageRoleAssistant, Content: []fantasy.MessagePart{fantasy.TextPart{Text: "history reply 1"}}},
		fantasy.NewUserMessage("history 2"),
		{Role: fantasy.MessageRoleAssistant, Content: []fantasy.MessagePart{fantasy.TextPart{Text: "history reply 2"}}},
		fantasy.NewUserMessage("history 3"),
		{Role: fantasy.MessageRoleAssistant, Content: []fantasy.MessagePart{fantasy.TextPart{Text: "history reply 3"}}},
		fantasy.NewUserMessage("history 4"),
		{Role: fantasy.MessageRoleAssistant, Content: []fantasy.MessagePart{fantasy.TextPart{Text: "history reply 4"}}},
		fantasy.NewUserMessage("start a long task"),
	}
	step1, err := service.buildModelInputProjection(ctx, agent.ModelInputSnapshot{
		SessionID: session.ID,
		TurnID:    "turn-multi",
		Step:      1,
		Model:     "gpt-4o",
		Messages:  append([]fantasy.Message(nil), history...),
	})
	if err != nil {
		t.Fatal(err)
	}
	if !fantasyMessagesContainText(step1.Messages, "压缩延续") {
		t.Fatalf("step1 projection missing compact summary: %d messages", len(step1.Messages))
	}

	// Fantasy appends step results to the same prefix; the projection must
	// stay compacted on step 2 and step 3 instead of resending full history.
	step2Input := append(append([]fantasy.Message(nil), history...),
		fantasy.Message{Role: fantasy.MessageRoleAssistant, Content: []fantasy.MessagePart{fantasy.TextPart{Text: "step1 reply"}}},
	)
	step2, err := service.buildModelInputProjection(ctx, agent.ModelInputSnapshot{
		SessionID: session.ID,
		TurnID:    "turn-multi",
		Step:      2,
		Model:     "gpt-4o",
		Messages:  step2Input,
	})
	if err != nil {
		t.Fatal(err)
	}
	if !fantasyMessagesContainText(step2.Messages, "压缩延续") {
		t.Fatal("step2 lost the compact projection")
	}
	if fantasyMessagesContainText(step2.Messages, "history 1") {
		t.Fatal("step2 resent pre-compact history")
	}
	if !fantasyMessagesContainText(step2.Messages, "step1 reply") {
		t.Fatal("step2 dropped post-compact tail message")
	}

	step3Input := append(append([]fantasy.Message(nil), step2Input...),
		fantasy.Message{Role: fantasy.MessageRoleAssistant, Content: []fantasy.MessagePart{fantasy.TextPart{Text: "step2 reply"}}},
	)
	step3, err := service.buildModelInputProjection(ctx, agent.ModelInputSnapshot{
		SessionID: session.ID,
		TurnID:    "turn-multi",
		Step:      3,
		Model:     "gpt-4o",
		Messages:  step3Input,
	})
	if err != nil {
		t.Fatal(err)
	}
	if !fantasyMessagesContainText(step3.Messages, "压缩延续") {
		t.Fatal("step3 lost the compact projection")
	}
	if fantasyMessagesContainText(step3.Messages, "history 2") {
		t.Fatal("step3 resent pre-compact history")
	}

	var completed int
	if err := conn.QueryRowContext(ctx, `SELECT COUNT(*) FROM runtime_context_boundaries WHERE session_id = ? AND kind = 'full' AND trigger = 'auto' AND status = 'completed'`, session.ID).Scan(&completed); err != nil {
		t.Fatal(err)
	}
	if completed != 1 {
		t.Fatalf("expected exactly one auto compact for the turn, got %d", completed)
	}

	// Turn terminal state clears the in-memory projection state.
	service.clearCompactTurnState(session.ID, "turn-multi")
	if _, ok := service.compactStateForTurn(session.ID, "turn-multi"); ok {
		t.Fatal("compact turn state should be cleared")
	}
}

func TestBuildModelInputProjectionAutoCompactOncePerTurn(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	runtimeWorkbench, workspace := workbenchForSkillTest(t)
	service := newRuntimeService()
	installTestCompactSummaryGenerator(service)
	service.runtime = runtimeWorkbench
	service.workspace = &apitypes.Workspace{ID: workspace.ID, Path: workspace.Path}
	conn, err := db.Connect(ctx, workspace.Config.Options.DataDirectory)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Release(workspace.Config.Options.DataDirectory) })
	service.turns = newRuntimeTurnStore(conn)

	session, err := runtimeWorkbench.CreateSession(ctx, workspace.ID, "auto once per turn")
	if err != nil {
		t.Fatal(err)
	}
	service.sessionID = session.ID
	ws, err := runtimeWorkbench.GetWorkspace(workspace.ID)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := ws.Messages.Create(ctx, session.ID, message.CreateMessageParams{
		Role:  message.User,
		Parts: []message.ContentPart{message.TextContent{Text: "start"}},
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := ws.Messages.Create(ctx, session.ID, message.CreateMessageParams{
		Role:  message.Assistant,
		Parts: []message.ContentPart{message.TextContent{Text: "anchor"}, message.Finish{Reason: message.FinishReasonEndTurn, Time: time.Now().Unix()}},
		Usage: message.Usage{InputTokens: 170000, OutputTokens: 15000},
	}); err != nil {
		t.Fatal(err)
	}

	// The turn already consumed its single auto compact attempt.
	service.markAutoCompactAttempted(session.ID, "turn-once")
	result, err := service.buildModelInputProjection(ctx, agent.ModelInputSnapshot{
		SessionID: session.ID,
		TurnID:    "turn-once",
		Step:      2,
		Model:     "gpt-4o",
		Messages: []fantasy.Message{
			fantasy.NewUserMessage("start"),
			{Role: fantasy.MessageRoleAssistant, Content: []fantasy.MessagePart{fantasy.TextPart{Text: "anchor"}}},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Messages) == 0 {
		t.Fatalf("projection = %#v", result)
	}
	var boundaries int
	if err := conn.QueryRowContext(ctx, `SELECT COUNT(*) FROM runtime_context_boundaries WHERE session_id = ?`, session.ID).Scan(&boundaries); err != nil {
		t.Fatal(err)
	}
	if boundaries != 0 {
		t.Fatalf("auto compact ran despite per-turn flag: %d boundaries", boundaries)
	}
}

func installTestCompactSummaryGenerator(service *runtimeService) {
	service.compactSummaryGenerator = func(_ context.Context, req agent.CompactSummaryRequest) (agent.CompactSummaryResult, error) {
		return agent.CompactSummaryResult{Summary: req.Instructions + "\n\nCurrent objective and state preserved for continuation."}, nil
	}
}

func TestManualCompactRejectedWhileTurnRunning(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	runtimeWorkbench, workspace := workbenchForSkillTest(t)
	service := newRuntimeService()
	installTestCompactSummaryGenerator(service)
	service.runtime = runtimeWorkbench
	service.workspace = &apitypes.Workspace{ID: workspace.ID, Path: workspace.Path}
	conn, err := db.Connect(ctx, workspace.Config.Options.DataDirectory)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Release(workspace.Config.Options.DataDirectory) })
	service.turns = newRuntimeTurnStore(conn)

	session, err := runtimeWorkbench.CreateSession(ctx, workspace.ID, "busy compact")
	if err != nil {
		t.Fatal(err)
	}
	service.sessionID = session.ID

	service.mu.Lock()
	service.sessionTurns[session.ID] = "turn-running"
	service.requests["turn-running"] = runtimeRequestState{SessionID: session.ID, Status: "running"}
	service.mu.Unlock()

	_, err = service.ManualCompact(ctx, RuntimeContextActionRequest{
		SessionID: session.ID,
		TurnID:    "turn-running",
	})
	if err == nil || !strings.Contains(err.Error(), "会话正在运行") {
		t.Fatalf("expected busy rejection, got %v", err)
	}

	// Once the turn finishes, manual compact is allowed again.
	service.mu.Lock()
	state := service.requests["turn-running"]
	state.Status = "completed"
	state.Finished = true
	service.requests["turn-running"] = state
	service.mu.Unlock()
	ws, err := runtimeWorkbench.GetWorkspace(workspace.ID)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := ws.Messages.Create(ctx, session.ID, message.CreateMessageParams{
		Role:  message.User,
		Parts: []message.ContentPart{message.TextContent{Text: "please compact"}},
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := service.ManualCompact(ctx, RuntimeContextActionRequest{
		SessionID: session.ID,
		TurnID:    "turn-running",
	}); err != nil {
		t.Fatalf("manual compact after turn finished: %v", err)
	}
}

func TestManualCompactSecondBoundaryExcludesPreBoundaryRegion(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	runtimeWorkbench, workspace := workbenchForSkillTest(t)
	service := newRuntimeService()
	installTestCompactSummaryGenerator(service)
	service.runtime = runtimeWorkbench
	service.workspace = &apitypes.Workspace{ID: workspace.ID, Path: workspace.Path}
	conn, err := db.Connect(ctx, workspace.Config.Options.DataDirectory)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Release(workspace.Config.Options.DataDirectory) })
	service.turns = newRuntimeTurnStore(conn)

	session, err := runtimeWorkbench.CreateSession(ctx, workspace.ID, "pre tokens region")
	if err != nil {
		t.Fatal(err)
	}
	service.sessionID = session.ID
	ws, err := runtimeWorkbench.GetWorkspace(workspace.ID)
	if err != nil {
		t.Fatal(err)
	}
	first, err := ws.Messages.Create(ctx, session.ID, message.CreateMessageParams{
		Role:  message.User,
		Parts: []message.ContentPart{message.TextContent{Text: strings.Repeat("first task ", 100)}},
	})
	if err != nil {
		t.Fatal(err)
	}
	firstReply, err := ws.Messages.Create(ctx, session.ID, message.CreateMessageParams{
		Role:  message.Assistant,
		Parts: []message.ContentPart{message.TextContent{Text: strings.Repeat("first reply ", 100)}, message.Finish{Reason: message.FinishReasonEndTurn, Time: time.Now().Unix()}},
	})
	if err != nil {
		t.Fatal(err)
	}
	firstCompact, err := service.ManualCompact(ctx, RuntimeContextActionRequest{
		SessionID: session.ID,
		TurnID:    "turn-compact-1",
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(firstCompact.Boundary.MessageRefs) != 2 {
		t.Fatalf("first boundary refs = %#v", firstCompact.Boundary.MessageRefs)
	}

	second, err := ws.Messages.Create(ctx, session.ID, message.CreateMessageParams{
		Role:  message.User,
		Parts: []message.ContentPart{message.TextContent{Text: "second task"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	secondReply, err := ws.Messages.Create(ctx, session.ID, message.CreateMessageParams{
		Role:  message.Assistant,
		Parts: []message.ContentPart{message.TextContent{Text: "second reply"}, message.Finish{Reason: message.FinishReasonEndTurn, Time: time.Now().Unix()}},
	})
	if err != nil {
		t.Fatal(err)
	}
	secondCompact, err := service.ManualCompact(ctx, RuntimeContextActionRequest{
		SessionID: session.ID,
		TurnID:    "turn-compact-2",
	})
	if err != nil {
		t.Fatal(err)
	}
	refs := secondCompact.Boundary.MessageRefs
	if len(refs) != 2 || refs[0] != second.ID || refs[1] != secondReply.ID {
		t.Fatalf("second boundary refs = %#v, want [%s %s]", refs, second.ID, secondReply.ID)
	}
	for _, ref := range refs {
		if ref == first.ID || ref == firstReply.ID {
			t.Fatalf("second boundary folded pre-boundary message %s", ref)
		}
	}
	if secondCompact.Boundary.BudgetBefore == nil || firstCompact.Boundary.BudgetBefore == nil {
		t.Fatal("boundary budgets missing")
	}
	if secondCompact.Boundary.BudgetBefore.TotalEstimatedTokens >= firstCompact.Boundary.BudgetBefore.TotalEstimatedTokens {
		t.Fatalf("second preTokens %d should be below first %d",
			secondCompact.Boundary.BudgetBefore.TotalEstimatedTokens,
			firstCompact.Boundary.BudgetBefore.TotalEstimatedTokens)
	}
}

func fantasyMessagesContainText(messages []fantasy.Message, needle string) bool {
	for _, msg := range messages {
		for _, part := range msg.Content {
			if text, ok := fantasy.AsMessagePart[fantasy.TextPart](part); ok && strings.Contains(text.Text, needle) {
				return true
			}
		}
	}
	return false
}
