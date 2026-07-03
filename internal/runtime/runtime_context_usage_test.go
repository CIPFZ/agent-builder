package runtime

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/CIPFZ/agent-builder/internal/apitypes"
	"github.com/CIPFZ/agent-builder/internal/contextmgr"
	"github.com/CIPFZ/agent-builder/internal/db"
	"github.com/CIPFZ/agent-builder/internal/message"
	"github.com/CIPFZ/agent-builder/internal/runtimeapi"
)

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
			got := contextThresholds(tt.window, tt.maxOutput)
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
	boundaryAt := int64(2000)
	if _, err := conn.ExecContext(context.Background(), `UPDATE messages SET created_at = ?, updated_at = ? WHERE id = ?`, int64(1000), int64(1000), oldMessage.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := conn.ExecContext(context.Background(), `
INSERT INTO runtime_context_boundaries (
	id, session_id, turn_id, projection_id, kind, trigger, status, created_at, completed_at
) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)
`, "boundary-1", session.ID, "turn-1", "projection-1", "full", "manual", "completed", boundaryAt, boundaryAt); err != nil {
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
	if _, err := conn.ExecContext(context.Background(), `UPDATE messages SET created_at = ?, updated_at = ? WHERE id = ?`, int64(3000), int64(3000), anchorMessage.ID); err != nil {
		t.Fatal(err)
	}
	nextMessage, err := ws.Messages.Create(context.Background(), session.ID, message.CreateMessageParams{
		Role:  message.User,
		Parts: []message.ContentPart{message.TextContent{Text: strings.Repeat("next", 40)}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := conn.ExecContext(context.Background(), `UPDATE messages SET created_at = ?, updated_at = ? WHERE id = ?`, int64(4000), int64(4000), nextMessage.ID); err != nil {
		t.Fatal(err)
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

func TestManualCompactAppendsSummaryAndEvents(t *testing.T) {
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
	output := buildRuntimeOutputProjectionFromInput(runtimeConversationProjectionInput{
		Activity: RuntimeSessionActivityWindowResponse{
			SessionID: session.ID,
			Messages: []RuntimeMessage{
				{ID: "user-visible", SessionID: session.ID, Role: "user", Content: "visible", CreatedAt: 1},
			},
			Turns: []RuntimeTurn{{ID: "turn-compact", SessionID: session.ID, Status: "completed", UserMessageID: "user-visible", StartedAt: 1, FinishedAt: 2}},
		},
		Compact: []RuntimeCompactBoundary{{
			ID:               resp.Boundary.ID,
			SessionID:        session.ID,
			TurnID:           "turn-compact",
			Kind:             "full",
			Status:           contextmgr.ProjectionStatusCompleted,
			Trigger:          "manual",
			SummaryMessageID: resp.Boundary.SummaryMessageID,
			CreatedAt:        resp.Boundary.CreatedAt,
			CompletedAt:      resp.Boundary.CompletedAt,
		}},
	}).snapshot(session.ID, "1")
	assertHasConversationKind(t, output.Items, "compact_boundary")
}
