package runtime

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"testing"
	"time"

	"github.com/CIPFZ/agent-builder/internal/message"
)

const (
	memoryAcceptanceSeedEnv      = "AGENT_BUILDER_MEMORY_ACCEPTANCE_SEED"
	memoryAcceptanceRootEnv      = "AGENT_BUILDER_MEMORY_ACCEPTANCE_ROOT"
	memoryAcceptanceManifestEnv  = "AGENT_BUILDER_MEMORY_ACCEPTANCE_MANIFEST"
	memoryAcceptanceTurnsEnv     = "AGENT_BUILDER_MEMORY_ACCEPTANCE_TURNS"
	memoryConcurrencySeedEnv     = "AGENT_BUILDER_MEMORY_CONCURRENCY_SEED"
	memoryConcurrencyProviderEnv = "AGENT_BUILDER_MEMORY_CONCURRENCY_PROVIDER_URL"
	memoryConcurrencySessionsEnv = "AGENT_BUILDER_MEMORY_CONCURRENCY_SESSIONS"
	memoryLongStreamSeedEnv      = "AGENT_BUILDER_MEMORY_LONG_STREAM_SEED"
	memoryLongStreamProviderEnv  = "AGENT_BUILDER_MEMORY_LONG_STREAM_PROVIDER_URL"
)

func TestMemoryAcceptanceSeedLongStream(t *testing.T) {
	if os.Getenv(memoryLongStreamSeedEnv) != "1" {
		t.Skip("set " + memoryLongStreamSeedEnv + "=1 to seed long-stream memory acceptance data")
	}
	root := memoryAcceptanceRoot(t, "memory-long-stream")
	if err := os.MkdirAll(filepath.Join(root, "config"), 0o700); err != nil {
		t.Fatal(err)
	}
	providerURL := os.Getenv(memoryLongStreamProviderEnv)
	if providerURL == "" {
		t.Fatal(memoryLongStreamProviderEnv + " is required")
	}
	t.Setenv("AGENT_BUILDER_DESKTOP_ROOT", root)
	writeRuntimeDevModelConfig(t, root, providerURL)
	writeRuntimeDevPolicy(t, root, "full_access")

	ctx := context.Background()
	service := newRuntimeService()
	if _, err := service.Status(ctx); err != nil {
		t.Fatal(err)
	}
	sess, err := service.runtime.CreateSession(ctx, service.workspace.ID, "Long-stream memory acceptance")
	if err != nil {
		t.Fatal(err)
	}
	manifestPath := os.Getenv(memoryAcceptanceManifestEnv)
	if manifestPath == "" {
		manifestPath = filepath.Join(root, "memory-long-stream-manifest.json")
	}
	data, err := json.MarshalIndent(map[string]any{"sessionID": sess.ID}, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(manifestPath, data, 0o600); err != nil {
		t.Fatal(err)
	}
}

func TestMemoryAcceptanceSeedConcurrentSessions(t *testing.T) {
	if os.Getenv(memoryConcurrencySeedEnv) != "1" {
		t.Skip("set " + memoryConcurrencySeedEnv + "=1 to seed concurrent Session memory acceptance data")
	}
	root := memoryAcceptanceRoot(t, "memory-concurrency")
	if err := os.MkdirAll(filepath.Join(root, "config"), 0o700); err != nil {
		t.Fatal(err)
	}
	providerURL := os.Getenv(memoryConcurrencyProviderEnv)
	if providerURL == "" {
		t.Fatal(memoryConcurrencyProviderEnv + " is required")
	}
	t.Setenv("AGENT_BUILDER_DESKTOP_ROOT", root)
	writeRuntimeDevModelConfig(t, root, providerURL)
	writeRuntimeDevPolicy(t, root, "full_access")

	sessionCount := 100
	if value, parseErr := strconv.Atoi(os.Getenv(memoryConcurrencySessionsEnv)); parseErr == nil && value > 0 {
		sessionCount = value
	}
	ctx := context.Background()
	service := newRuntimeService()
	if _, err := service.Status(ctx); err != nil {
		t.Fatal(err)
	}
	sessionIDs := make([]string, 0, sessionCount)
	for index := 0; index < sessionCount; index++ {
		sess, err := service.runtime.CreateSession(ctx, service.workspace.ID, fmt.Sprintf("Concurrency memory Session %03d", index+1))
		if err != nil {
			t.Fatalf("create Session %d: %v", index+1, err)
		}
		sessionIDs = append(sessionIDs, sess.ID)
	}
	manifestPath := os.Getenv(memoryAcceptanceManifestEnv)
	if manifestPath == "" {
		manifestPath = filepath.Join(root, "memory-concurrency-manifest.json")
	}
	data, err := json.MarshalIndent(map[string]any{"sessionIDs": sessionIDs, "sessionCount": sessionCount}, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(manifestPath, data, 0o600); err != nil {
		t.Fatal(err)
	}
}

func TestMemoryAcceptanceSeedLongSession(t *testing.T) {
	if os.Getenv(memoryAcceptanceSeedEnv) != "1" {
		t.Skip("set " + memoryAcceptanceSeedEnv + "=1 to seed the long-Session memory acceptance fixture")
	}
	root := memoryAcceptanceRoot(t, "memory-long-session")
	if err := os.MkdirAll(filepath.Join(root, "config"), 0o700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("AGENT_BUILDER_DESKTOP_ROOT", root)
	writeRuntimeDevModelConfig(t, root, "http://127.0.0.1:1/v1")
	writeRuntimeDevPolicy(t, root, "full_access")

	turnCount := 1000
	if value, parseErr := strconv.Atoi(os.Getenv(memoryAcceptanceTurnsEnv)); parseErr == nil && value > 0 {
		turnCount = value
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
	sess, err := service.runtime.CreateSession(ctx, workspaceID, fmt.Sprintf("Memory acceptance: %d Turns", turnCount))
	if err != nil {
		t.Fatal(err)
	}
	baseTime := time.Now().Add(-time.Duration(turnCount) * time.Minute).UnixMilli()
	for index := 0; index < turnCount; index++ {
		turnID := fmt.Sprintf("turn-memory-%06d", index+1)
		user, createErr := ws.Messages.Create(ctx, sess.ID, message.CreateMessageParams{
			Role:     message.User,
			Parts:    []message.ContentPart{message.TextContent{Text: fmt.Sprintf("Persistent memory acceptance user message %d", index+1)}},
			Metadata: map[string]string{"turn_id": turnID},
		})
		if createErr != nil {
			t.Fatalf("create user message %d: %v", index+1, createErr)
		}
		assistant, createErr := ws.Messages.Create(ctx, sess.ID, message.CreateMessageParams{
			Role: message.Assistant,
			Parts: []message.ContentPart{
				message.TextContent{Text: fmt.Sprintf("Persistent memory acceptance assistant response %d", index+1)},
				message.Finish{Reason: message.FinishReasonEndTurn},
			},
			Metadata: map[string]string{"turn_id": turnID, "conversation_phase": "final"},
		})
		if createErr != nil {
			t.Fatalf("create assistant message %d: %v", index+1, createErr)
		}
		startedAt := baseTime + int64(index)*60_000
		if _, err := service.turns.Upsert(ctx, RuntimeTurn{
			ID: turnID, SessionID: sess.ID, Status: turnStatusCompleted,
			UserMessageID: user.ID, LatestAssistantMessageID: assistant.ID,
			StartedAt: startedAt, FinishedAt: startedAt + 1_000,
		}); err != nil {
			t.Fatalf("create Turn %d: %v", index+1, err)
		}
	}
	if _, err := service.SessionConversationSnapshotV2(ctx, sess.ID, RuntimeCanonicalConversationSnapshotRequest{}); err != nil {
		t.Fatal(err)
	}

	manifestPath := os.Getenv(memoryAcceptanceManifestEnv)
	if manifestPath == "" {
		manifestPath = filepath.Join(root, "memory-acceptance-manifest.json")
	}
	data, err := json.MarshalIndent(map[string]any{"sessionID": sess.ID, "turnCount": turnCount}, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(manifestPath, data, 0o600); err != nil {
		t.Fatal(err)
	}
}

func memoryAcceptanceRoot(t *testing.T, fallbackName string) string {
	t.Helper()
	repoRoot, err := filepath.Abs(filepath.Join("..", ".."))
	if err != nil {
		t.Fatal(err)
	}
	root := os.Getenv(memoryAcceptanceRootEnv)
	if root == "" {
		root = filepath.Join(repoRoot, "tmp", "runtime-dev", fallbackName)
	}
	root, err = filepath.Abs(root)
	if err != nil {
		t.Fatal(err)
	}
	if !isPathInside(filepath.Join(repoRoot, "tmp", "runtime-dev"), root) {
		t.Fatalf("refusing memory acceptance root outside tmp/runtime-dev: %s", root)
	}
	return root
}
