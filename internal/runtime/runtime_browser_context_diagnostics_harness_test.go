package runtime

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"testing"
	"time"
)

const phase4ContextHarnessEnv = "AGENT_BUILDER_PHASE4_CONTEXT_BROWSER_HARNESS"

func TestPhase4BrowserContextDiagnosticsHarnessServer(t *testing.T) {
	if os.Getenv(phase4ContextHarnessEnv) != "1" {
		t.Skip("set " + phase4ContextHarnessEnv + "=1 to run the browser context diagnostics harness server")
	}

	root := phase4ContextHarnessRoot(t)
	if err := os.MkdirAll(filepath.Join(root, "config"), 0o700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("AGENT_BUILDER_DESKTOP_ROOT", root)

	provider := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"id":"chatcmpl-phase4","choices":[{"message":{"role":"assistant","content":"ok"}}]}`))
	}))
	defer provider.Close()
	writeRuntimeDevModelConfig(t, root, provider.URL)

	ctx := context.Background()
	service := newRuntimeService()
	if _, err := service.OpenProject(ctx, RuntimeOpenProjectRequest{Path: root, CreateMissing: true}); err != nil {
		t.Fatal(err)
	}
	service.mu.Lock()
	workspaceID := service.workspace.ID
	service.mu.Unlock()
	if workspaceID == "" {
		t.Fatal("workspace id is empty after runtime readiness")
	}

	sess, err := service.runtime.CreateSession(ctx, workspaceID, "Phase 4 context diagnostics")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := service.SelectSession(ctx, sess.ID); err != nil {
		t.Fatal(err)
	}
	turn, err := service.turns.Upsert(ctx, RuntimeTurn{
		ID:        "turn-phase4-context",
		SessionID: sess.ID,
		Status:    turnStatusRunning,
		StartedAt: 1000,
		Provider:  "openai",
		Model:     "phase-4-context-model",
	})
	if err != nil {
		t.Fatal(err)
	}
	assembly, err := service.promptAssemblies.Upsert(ctx, RuntimePromptAssembly{
		ID:        runtimePromptAssemblyID(turn.ID, 1),
		SessionID: sess.ID,
		TurnID:    turn.ID,
		Step:      1,
		Provider:  "openai",
		Model:     "phase-4-context-model",
		System: RuntimePromptSystemSummary{
			Source:             "runtime",
			Hash:               "sha256:phase4-system",
			TokenEstimate:      24,
			PromptPrefix:       true,
			PromptPrefixHash:   "sha256:phase4-prefix",
			PromptPrefixTokens: 8,
			SourceRefs:         []string{"system:runtime", "prompt-prefix:runtime"},
			Redacted:           true,
		},
		Messages: RuntimePromptMessageSummary{
			Count:                3,
			ByRole:               map[string]int{"user": 1, "assistant": 1, "tool": 1},
			ToolResultCount:      1,
			DeliveredToolResults: 1,
			AttachmentCount:      1,
			ImageCount:           0,
			TokenEstimate:        96,
			RawPromptStored:      false,
		},
		Tools: RuntimePromptToolSummary{
			Selected:         []string{"bash", "read_file"},
			Omitted:          []string{"webfetch"},
			SelectedCount:    2,
			OmittedCount:     1,
			SelectedBudget:   RuntimeBudgetBucket{Count: 2, EstimatedTokens: 34},
			OmittedBudget:    RuntimeBudgetBucket{Count: 1, EstimatedTokens: 13},
			ResultCount:      1,
			PersistedResults: 1,
			CompactedResults: 1,
		},
		Skills: RuntimePromptSkillSummary{
			AvailableCount:   2,
			LoadedCount:      1,
			Names:            []string{"agent-builder-config", "openai-docs"},
			LoadedNames:      []string{"agent-builder-config"},
			XMLPresent:       true,
			XMLHash:          "sha256:phase4-skills",
			TokenEstimate:    12,
			RawContentStored: false,
		},
		MCP: RuntimePromptMCPSummary{
			ServerCount:      1,
			InstructionCount: 1,
			Servers:          []string{"docs"},
			ServerListHash:   "sha256:phase4-mcp-servers",
			InstructionHash:  "sha256:phase4-mcp",
			TokenEstimate:    10,
			RawContentStored: false,
		},
		ContextSources: []RuntimeContextSource{
			{
				ID:             "context:agents",
				Kind:           "project_memory",
				Name:           "AGENTS.md",
				Path:           filepath.Join(root, "AGENTS.md"),
				Scope:          "project",
				Enabled:        true,
				State:          capabilityStateLoaded,
				ContentSummary: "Project instructions loaded by runtime.",
				TokenEstimate:  32,
				ContentHash:    "sha256:phase4-agents",
			},
			{
				ID:         "context:missing-user-memory",
				Kind:       "user_memory",
				Name:       "User memory",
				Scope:      "user",
				Enabled:    true,
				State:      capabilityStateFailed,
				Reason:     "memory_unavailable",
				Error:      "user memory unavailable in harness",
				Provenance: "runtime",
			},
		},
		Compact: []RuntimeCompactBoundary{{
			ID:             "compact-phase4",
			SessionID:      sess.ID,
			TurnID:         turn.ID,
			Kind:           "microcompact",
			Trigger:        "tool_result_budget",
			Status:         "completed",
			SummaryRef:     "runtime://refs/compact-phase4",
			MessageRefs:    []string{"msg-1", "msg-2"},
			ToolCallRefs:   []RuntimeCompactToolCallRef{{ToolCallID: "tool-1", Name: "bash", Ref: "runtime://refs/tool-1", Preserved: true}},
			ReinjectedRefs: []RuntimeReinjectedRef{{ID: "read-1", Kind: "read_file", Name: "README.md", Status: "included", Ref: "runtime://refs/read-1"}},
			CreatedAt:      1200,
			CompletedAt:    1300,
		}},
		Budget: RuntimeBudgetReport{
			SessionID:            sess.ID,
			TurnID:               turn.ID,
			Model:                "phase-4-context-model",
			ContextWindow:        128000,
			InputBudget:          RuntimeBudgetBucket{Count: 1, EstimatedTokens: 4096},
			Messages:             RuntimeBudgetBucket{Count: 3, EstimatedTokens: 96},
			ContextSources:       RuntimeBudgetBucket{Count: 2, EstimatedTokens: 32},
			SelectedToolSchemas:  RuntimeBudgetBucket{Count: 2, EstimatedTokens: 34},
			OmittedToolSchemas:   RuntimeBudgetBucket{Count: 1, EstimatedTokens: 13},
			ToolOutputs:          RuntimeBudgetBucket{Count: 1, EstimatedTokens: 21},
			Skills:               RuntimeBudgetBucket{Count: 1, EstimatedTokens: 12},
			MCP:                  RuntimeBudgetBucket{Count: 1, EstimatedTokens: 10},
			TotalEstimatedTokens: 205,
			UpdatedAt:            1400,
		},
		CreatedAt: 1500,
	})
	if err != nil {
		t.Fatal(err)
	}

	server := newRuntimeHTTPServer(service)
	if err := server.StartAt("127.0.0.1:0", "agent-builder-dev"); err != nil {
		t.Fatal(err)
	}
	defer func() {
		closeCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = server.Close(closeCtx)
	}()

	manifestPath := filepath.Join(root, "harness-manifest.json")
	stopPath := filepath.Join(root, "harness-stop")
	writePhase4ContextHarnessManifest(t, manifestPath, map[string]string{
		"runtimeURL":   server.URL(),
		"runtimeToken": server.Token(),
		"sessionID":    sess.ID,
		"turnID":       turn.ID,
		"assemblyID":   assembly.ID,
		"stopPath":     stopPath,
	})

	deadline := time.Now().Add(phase4ContextHarnessTimeout())
	for {
		if _, err := os.Stat(stopPath); err == nil {
			return
		}
		if time.Now().After(deadline) {
			t.Fatalf("timed out waiting for harness stop file: %s", stopPath)
		}
		time.Sleep(200 * time.Millisecond)
	}
}

func phase4ContextHarnessRoot(t *testing.T) string {
	t.Helper()
	repoRoot, err := filepath.Abs(filepath.Join("..", ".."))
	if err != nil {
		t.Fatal(err)
	}
	root := os.Getenv("AGENT_BUILDER_PHASE4_CONTEXT_HARNESS_ROOT")
	if root == "" {
		root = filepath.Join(repoRoot, "tmp", "runtime-dev", "phase4-context-diagnostics-browser-smoke")
	}
	root, err = filepath.Abs(root)
	if err != nil {
		t.Fatal(err)
	}
	if !isPathInside(filepath.Join(repoRoot, "tmp", "runtime-dev"), root) {
		t.Fatalf("refusing phase 4 context harness root outside tmp/runtime-dev: %s", root)
	}
	return root
}

func phase4ContextHarnessTimeout() time.Duration {
	seconds, err := strconv.Atoi(os.Getenv("AGENT_BUILDER_PHASE4_CONTEXT_HARNESS_TIMEOUT_SECONDS"))
	if err != nil || seconds <= 0 {
		seconds = 180
	}
	return time.Duration(seconds) * time.Second
}

func writePhase4ContextHarnessManifest(t *testing.T, path string, value map[string]string) {
	t.Helper()
	data, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatal(err)
	}
}
