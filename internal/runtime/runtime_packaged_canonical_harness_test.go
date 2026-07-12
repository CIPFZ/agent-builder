package runtime

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/CIPFZ/agent-builder/internal/message"
	"github.com/CIPFZ/agent-builder/internal/runtimeapi"
)

func TestPhase7PackagedCanonicalSeed(t *testing.T) {
	if os.Getenv(phase362SeedEnv) != "1" {
		t.Skip("set " + phase362SeedEnv + "=1 to seed packaged canonical conversation data")
	}
	root := phase362HarnessRoot(t)
	manifestPath := os.Getenv(phase362ManifestNameEnv)
	if manifestPath == "" {
		manifestPath = filepath.Join(root, "harness-manifest.json")
	}
	if err := os.MkdirAll(filepath.Join(root, "config"), 0o700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("AGENT_BUILDER_DESKTOP_ROOT", root)
	writeRuntimeDevModelConfig(t, root, os.Getenv(phase362ProviderURLEnv))
	if err := os.WriteFile(filepath.Join(root, "config", "policy.json"), []byte(`{"mode":"full_access"}`), 0o600); err != nil {
		t.Fatal(err)
	}

	ctx := context.Background()
	service := newRuntimeService()
	if _, err := service.Status(ctx); err != nil {
		t.Fatal(err)
	}
	workspaceID := service.workspace.ID
	ws, err := service.runtime.GetWorkspace(workspaceID)
	if err != nil {
		t.Fatal(err)
	}
	sess, err := service.runtime.CreateSession(ctx, workspaceID, "Phase 7 packaged canonical conversation")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := service.SelectSession(ctx, sess.ID); err != nil {
		t.Fatal(err)
	}
	const finalText = "Phase 7 packaged canonical final is visible"
	user, err := ws.Messages.Create(ctx, sess.ID, message.CreateMessageParams{Role: message.User, Parts: []message.ContentPart{message.TextContent{Text: "Verify packaged canonical rendering"}}, Metadata: map[string]string{"turn_id": "turn-phase7-final"}})
	if err != nil {
		t.Fatal(err)
	}
	final, err := ws.Messages.Create(ctx, sess.ID, message.CreateMessageParams{Role: message.Assistant, Parts: []message.ContentPart{message.TextContent{Text: finalText}, message.Finish{Reason: "stop"}}, Metadata: map[string]string{"turn_id": "turn-phase7-final", "conversation_phase": "final"}})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := service.turns.Upsert(ctx, RuntimeTurn{ID: "turn-phase7-final", SessionID: sess.ID, Status: turnStatusCompleted, UserMessageID: user.ID, LatestAssistantMessageID: final.ID, StartedAt: user.CreatedAt, FinishedAt: final.UpdatedAt}); err != nil {
		t.Fatal(err)
	}
	for _, event := range []RuntimeEvent{{Type: runtimeapi.EventTurnStarted, SessionID: sess.ID, TurnID: "turn-phase7-final"}, {Type: runtimeapi.EventMessageCreated, SessionID: sess.ID, TurnID: "turn-phase7-final", MessageID: user.ID}, {Type: runtimeapi.EventMessageCreated, SessionID: sess.ID, TurnID: "turn-phase7-final", MessageID: final.ID}, {Type: runtimeapi.EventMessageCompleted, SessionID: sess.ID, TurnID: "turn-phase7-final", MessageID: final.ID}, {Type: runtimeapi.EventTurnCompleted, SessionID: sess.ID, TurnID: "turn-phase7-final"}} {
		service.publishRuntimeEvent(event)
	}
	if _, err := service.SessionConversationSnapshotV2(ctx, sess.ID, RuntimeCanonicalConversationSnapshotRequest{}); err != nil {
		t.Fatal(err)
	}
	writePhase362HarnessManifest(t, manifestPath, map[string]string{"sessionID": sess.ID, "finalText": finalText})
}
