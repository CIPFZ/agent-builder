package runtime

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/CIPFZ/agent-builder/internal/apitypes"
	"github.com/CIPFZ/agent-builder/internal/db"
	"github.com/CIPFZ/agent-builder/internal/message"
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
