package runtime

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"charm.land/catwalk/pkg/catwalk"
	mcptools "github.com/CIPFZ/agent-builder/internal/agent/tools/mcp"
	"github.com/CIPFZ/agent-builder/internal/apitypes"
	"github.com/CIPFZ/agent-builder/internal/config"
	"github.com/CIPFZ/agent-builder/internal/db"
	"github.com/CIPFZ/agent-builder/internal/permission"
	"github.com/CIPFZ/agent-builder/internal/runtimeapi"
	"github.com/CIPFZ/agent-builder/internal/tools/scheduler"
	"github.com/CIPFZ/agent-builder/internal/workbench"
	sdkmcp "github.com/modelcontextprotocol/go-sdk/mcp"
)

func TestRuntimeExternalMCPInterruptedStructuredRefsFixture(t *testing.T) {
	_ = mcptools.Close(context.Background())
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	t.Cleanup(func() { _ = mcptools.Close(context.Background()) })

	dir := phase61RuntimeDevDir(t)
	t.Setenv("AGENT_BUILDER_DESKTOP_ROOT", dir)
	workingDir := filepath.Join(dir, "workspace")
	dataDir := filepath.Join(dir, "data")
	if err := os.MkdirAll(workingDir, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(dataDir, 0o700); err != nil {
		t.Fatal(err)
	}

	structuredPath := filepath.Join(dir, "phase61-structured-artifact.json")
	prosePath := filepath.Join(dir, "phase61-prose-should-not-count.json")
	serverScript := filepath.Join(dir, "phase61-mcp-server.cjs")
	if err := os.WriteFile(serverScript, []byte(phase61MCPServerScript()), 0o600); err != nil {
		t.Fatal(err)
	}
	nodePath, err := exec.LookPath("node")
	if err != nil {
		t.Skipf("node is required for external MCP stdio fixture: %v", err)
	}

	provider := newPhase61FakeProvider(t, structuredPath, prosePath)
	defer provider.shutdown()

	cfg := config.NewRuntimeConfig(workingDir, dataDir, false)
	cfg.Options.AutoLSP = ptr(false)
	cfg.Options.DisableAutoSummarize = true
	cfg.Providers.Set("phase61-provider", config.ProviderConfig{
		ID:      "phase61-provider",
		Name:    "Phase 6.1 fake provider",
		BaseURL: provider.URL + "/v1",
		Type:    catwalk.TypeOpenAICompat,
		APIKey:  "test-key",
		Models: []catwalk.Model{{
			ID:               "phase61-model",
			Name:             "phase61-model",
			ContextWindow:    8192,
			DefaultMaxTokens: 1024,
		}},
	})
	cfg.Models = map[config.SelectedModelType]config.SelectedModel{
		config.SelectedModelTypeLarge: {Provider: "phase61-provider", Model: "phase61-model"},
		config.SelectedModelTypeSmall: {Provider: "phase61-provider", Model: "phase61-model"},
	}
	cfg.MCP = config.MCPs{
		"phase61": {
			Type:         config.MCPStdio,
			Command:      nodePath,
			Args:         []string{serverScript},
			EnabledTools: []string{"structured_artifact"},
			Timeout:      5,
		},
	}
	cfg.Permissions = &config.Permissions{AllowedTools: []string{"mcp_phase61_structured_artifact"}}
	cfg.SetupAgents()

	conn, err := db.Connect(ctx, dataDir)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Release(dataDir) })

	service := newRuntimeService()
	service.turns = newRuntimeTurnStore(conn)
	service.toolCalls = scheduler.New(NewRuntimeToolCallStoreForDB(conn))
	service.permissionStore = newRuntimePermissionStore(conn)
	service.worktrees = newRuntimeWorktreeStore(conn)
	service.agentTasks = newRuntimeAgentTaskStore(conn)
	service.hookExecutions = newRuntimeHookExecutionStore(conn)
	service.mcpRequestStore = newRuntimeMCPRequestStore(conn)
	service.refs = newRuntimeRefStore(conn, dataDir)
	service.eventStore = newRuntimeEventStore(conn)
	service.policy = runtimePolicyFromParts(permission.PolicyModeFullAccess, "phase61", nil, time.Now().UnixMilli())

	store := config.NewRuntimeStore(workingDir, cfg)
	recorder := &runtimeSchedulerRecorder{service: service}
	runtimeWorkbench := workbench.NewWithSchedulerRecorder(ctx, store, nil, recorder)
	_, workspace, err := runtimeWorkbench.CreateWorkspace(apitypes.Workspace{
		Path:    workingDir,
		DataDir: dataDir,
		Config:  cfg,
		YOLO:    true,
		Env:     os.Environ(),
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { runtimeWorkbench.DeleteWorkspace(workspace.ID) })
	if err := mcptools.InitializeSingle(ctx, "phase61", store); err != nil {
		t.Fatalf("initialize phase61 mcp: %v", err)
	}
	mcptools.RefreshTools(ctx, store, "phase61")
	if err := mcptools.WaitForInit(ctx); err != nil {
		t.Fatalf("mcp init failed: %v", err)
	}
	if err := runtimeWorkbench.UpdateAgent(ctx, workspace.ID); err != nil {
		t.Fatalf("update agent with MCP tools: %v", err)
	}
	service.runtime = runtimeWorkbench
	service.workspace = &apitypes.Workspace{ID: workspace.ID, Path: workspace.Path}
	service.runtimeCtx = ctx
	service.cancel = cancel
	project := registerRuntimeMCPTestProject(t, service, workingDir)

	chat, err := service.Chat(ctx, RuntimeChatRequest{
		Prompt:    "Use the external MCP structured_artifact tool to create " + structuredPath + ". Do not use " + prosePath + " as an artifact.",
		ProjectID: project.ID,
		Scope:     "project",
	})
	if err != nil {
		t.Fatal(err)
	}
	sessionID := service.sessionID
	if sessionID == "" {
		t.Fatal("chat did not establish a runtime session")
	}
	if err := waitForPhase61ToolCompletion(ctx, service, chat.TurnID, structuredPath); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(structuredPath); err != nil {
		t.Fatalf("external MCP server did not create artifact: %v", err)
	}
	if _, err := os.Stat(prosePath); !os.IsNotExist(err) {
		t.Fatalf("prose-only path should not be created; stat err = %v", err)
	}
	if !waitForPhase61SecondRequest(ctx, provider) {
		t.Fatal("fake provider never received the post-tool request; turn was not interrupted before final completion")
	}

	cancel()
	restarted := newRuntimeService()
	restarted.runtime = runtimeWorkbench
	restarted.workspace = &apitypes.Workspace{ID: workspace.ID, Path: workspace.Path}
	restarted.turns = newRuntimeTurnStore(conn)
	restarted.toolCalls = scheduler.New(NewRuntimeToolCallStoreForDB(conn))
	restarted.permissionStore = newRuntimePermissionStore(conn)
	restarted.worktrees = newRuntimeWorktreeStore(conn)
	restarted.agentTasks = newRuntimeAgentTaskStore(conn)
	restarted.hookExecutions = newRuntimeHookExecutionStore(conn)
	restarted.mcpRequestStore = newRuntimeMCPRequestStore(conn)
	restarted.refs = newRuntimeRefStore(conn, dataDir)
	restarted.eventStore = newRuntimeEventStore(conn)
	restarted.policy = service.policy

	interrupted, err := restarted.turns.InterruptUnfinished(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	cancelledTools, err := cancelUnfinishedRuntimeToolCalls(context.Background(), restarted.toolCalls, conn)
	if err != nil {
		t.Fatal(err)
	}
	for _, turn := range interrupted {
		restarted.storeRuntimeEvent(runtimeapi.Event{
			ID:        newRuntimeEventID(),
			Type:      runtimeapi.EventTurnInterrupted,
			CreatedAt: time.Now().UTC().Format(time.RFC3339Nano),
			SessionID: turn.SessionID,
			TurnID:    turn.ID,
			Payload: map[string]any{
				"status": turnStatusInterrupted,
				"error":  turn.Error,
			},
		})
	}
	for _, call := range cancelledTools {
		restarted.storeRuntimeEvent(runtimeToolCallEvent(runtimeapi.EventToolCallCancelled, call, map[string]any{
			"name":    call.Name,
			"summary": "runtime restarted",
			"status":  string(call.Status),
		}))
	}

	activity, err := restarted.SessionActivity(context.Background(), sessionID)
	if err != nil {
		t.Fatal(err)
	}
	if len(activity.Turns) != 1 || activity.Turns[0].Status != turnStatusInterrupted {
		t.Fatalf("interrupted turn was not restored from SessionActivity: %#v", activity.Turns)
	}
	turn := activity.Turns[0]
	if !slices.Contains(turn.Diagnostics.ProducedArtifacts, structuredPath) {
		t.Fatalf("structured artifact ref missing from diagnostics: %#v", turn.Diagnostics)
	}
	if !slices.Contains(turn.Diagnostics.VerifiedArtifacts, structuredPath) {
		t.Fatalf("structured artifact was not verified on disk: %#v", turn.Diagnostics)
	}
	if slices.Contains(turn.Diagnostics.ProducedArtifacts, prosePath) {
		t.Fatalf("assistant prose-only path counted as produced artifact: %#v", turn.Diagnostics.ProducedArtifacts)
	}
	if turn.Diagnostics.ArtifactConfidenceSummary.StructuredMCPCustomRefs == 0 {
		t.Fatalf("structured MCP confidence missing: %#v", turn.Diagnostics.ArtifactConfidenceSummary)
	}
	if turn.Diagnostics.ArtifactCounts.StructuredRefs == 0 {
		t.Fatalf("diagnostics artifact counts missing structured refs: %#v", turn.Diagnostics.ArtifactCounts)
	}
	summary := turn.Interrupted
	if summary == nil {
		t.Fatal("interrupted recovery summary missing")
	}
	if !slices.Contains(summary.ProducedArtifacts, structuredPath) || slices.Contains(summary.ProducedArtifacts, prosePath) {
		t.Fatalf("interrupted artifact summary = %#v", summary.ProducedArtifacts)
	}
	if summary.LastCompletedTool.Source != string(scheduler.ToolSourceMCP) {
		t.Fatalf("last completed tool did not come from MCP path: %#v", summary.LastCompletedTool)
	}
	if summary.LastCompletedTool.Target != structuredPath || len(summary.LastCompletedTool.ArtifactRefs) == 0 {
		t.Fatalf("target/display metadata was not restored: %#v", summary.LastCompletedTool)
	}
	if summary.ArtifactCounts.StructuredRefs == 0 || !strings.Contains(summary.SummaryText, structuredPath) {
		t.Fatalf("interrupted recovery summary lost structured refs: %#v", summary)
	}
	for _, call := range activity.ToolCalls {
		if call.Status == string(scheduler.ToolCallRunning) || call.Status == string(scheduler.ToolCallPending) || call.Status == string(scheduler.ToolCallWaitingPermission) {
			t.Fatalf("stale live tool restored after interruption: %#v", call)
		}
	}
}

func TestRuntimeHTTPAndSSEMCPInterruptedStructuredRefsFixture(t *testing.T) {
	for _, tc := range []struct {
		name       string
		transport  string
		configType config.MCPType
	}{
		{name: "streamable_http", transport: "http", configType: config.MCPHttp},
		{name: "sse", transport: "sse", configType: config.MCPSSE},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_ = mcptools.Close(context.Background())
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			t.Cleanup(func() { _ = mcptools.Close(context.Background()) })

			dir := phase61RuntimeDevDir(t)
			t.Setenv("AGENT_BUILDER_DESKTOP_ROOT", dir)
			workingDir := filepath.Join(dir, "workspace")
			dataDir := filepath.Join(dir, "data")
			if err := os.MkdirAll(workingDir, 0o700); err != nil {
				t.Fatal(err)
			}
			if err := os.MkdirAll(dataDir, 0o700); err != nil {
				t.Fatal(err)
			}

			mcpName := "phase66_" + strings.ReplaceAll(tc.name, "-", "_")
			toolName := "mcp_" + mcpName + "_structured_artifact"
			structuredPath := filepath.Join(dir, tc.name+"-structured-artifact.json")
			prosePath := filepath.Join(dir, tc.name+"-prose-should-not-count.json")
			mcpServer := newPhase66MCPHTTPServer(t, structuredPath, tc.transport)
			t.Cleanup(func() {
				_ = mcptools.Close(context.Background())
				mcpServer.CloseClientConnections()
				mcpServer.Close()
			})

			provider := newPhase66FakeProvider(t, structuredPath, prosePath, toolName)
			defer provider.shutdown()

			cfg := config.NewRuntimeConfig(workingDir, dataDir, false)
			cfg.Options.AutoLSP = ptr(false)
			cfg.Options.DisableAutoSummarize = true
			cfg.Providers.Set("phase66-provider-"+tc.name, config.ProviderConfig{
				ID:      "phase66-provider-" + tc.name,
				Name:    "Phase 6.6 fake provider " + tc.name,
				BaseURL: provider.URL + "/v1",
				Type:    catwalk.TypeOpenAICompat,
				APIKey:  "test-key",
				Models: []catwalk.Model{{
					ID:               "phase66-model",
					Name:             "phase66-model",
					ContextWindow:    8192,
					DefaultMaxTokens: 1024,
				}},
			})
			cfg.Models = map[config.SelectedModelType]config.SelectedModel{
				config.SelectedModelTypeLarge: {Provider: "phase66-provider-" + tc.name, Model: "phase66-model"},
				config.SelectedModelTypeSmall: {Provider: "phase66-provider-" + tc.name, Model: "phase66-model"},
			}
			cfg.MCP = config.MCPs{
				mcpName: {
					Type:         tc.configType,
					URL:          mcpServer.URL,
					Headers:      map[string]string{"Authorization": "Bearer phase66-test"},
					EnabledTools: []string{"structured_artifact"},
					Timeout:      5,
				},
			}
			cfg.Permissions = &config.Permissions{AllowedTools: []string{toolName}}
			cfg.SetupAgents()

			conn, err := db.Connect(ctx, dataDir)
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { _ = db.Release(dataDir) })

			service := newPhase66RuntimeService(conn, dataDir, nil)
			service.policy = runtimePolicyFromParts(permission.PolicyModeFullAccess, "phase66", nil, time.Now().UnixMilli())

			store := config.NewRuntimeStore(workingDir, cfg)
			recorder := &runtimeSchedulerRecorder{service: service}
			runtimeWorkbench := workbench.NewWithSchedulerRecorder(ctx, store, nil, recorder)
			_, workspace, err := runtimeWorkbench.CreateWorkspace(apitypes.Workspace{
				Path:    workingDir,
				DataDir: dataDir,
				Config:  cfg,
				YOLO:    true,
				Env:     os.Environ(),
			})
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { runtimeWorkbench.DeleteWorkspace(workspace.ID) })
			if err := mcptools.InitializeSingle(ctx, mcpName, store); err != nil {
				t.Fatalf("initialize %s mcp: %v", mcpName, err)
			}
			mcptools.RefreshTools(ctx, store, mcpName)
			if err := mcptools.WaitForInit(ctx); err != nil {
				t.Fatalf("mcp init failed: %v", err)
			}
			if err := runtimeWorkbench.UpdateAgent(ctx, workspace.ID); err != nil {
				t.Fatalf("update agent with MCP tools: %v", err)
			}
			service.runtime = runtimeWorkbench
			service.workspace = &apitypes.Workspace{ID: workspace.ID, Path: workspace.Path}
			service.runtimeCtx = ctx
			service.cancel = cancel
			project := registerRuntimeMCPTestProject(t, service, workingDir)

			chat, err := service.Chat(ctx, RuntimeChatRequest{
				Prompt:    "Use the MCP structured_artifact tool to create " + structuredPath + ". Do not use " + prosePath + " as an artifact.",
				ProjectID: project.ID,
				Scope:     "project",
			})
			if err != nil {
				t.Fatal(err)
			}
			sessionID := service.sessionID
			if sessionID == "" {
				t.Fatal("chat did not establish a runtime session")
			}
			if err := waitForPhase61ToolCompletion(ctx, service, chat.TurnID, structuredPath); err != nil {
				t.Fatal(err)
			}
			if _, err := os.Stat(structuredPath); err != nil {
				t.Fatalf("MCP server did not create artifact: %v", err)
			}
			if _, err := os.Stat(prosePath); !os.IsNotExist(err) {
				t.Fatalf("prose-only path should not be created; stat err = %v", err)
			}
			if !waitForPhase61SecondRequest(ctx, provider) {
				t.Fatal("fake provider never received the post-tool request; turn was not interrupted before final completion")
			}
			if mcpServer.authRequests.Load() == 0 {
				t.Fatal("MCP HTTP/SSE fixture did not observe configured auth header")
			}
			// Phase 6.8 exercises the successful disconnect edge: after the
			// scheduler has recorded completed MCP output, a transport close must
			// not erase or replay partial state into artifact evidence.
			mcpServer.CloseClientConnections()

			cancel()
			restarted := newPhase66RuntimeService(conn, dataDir, runtimeWorkbench)
			restarted.workspace = &apitypes.Workspace{ID: workspace.ID, Path: workspace.Path}
			restarted.policy = service.policy
			interrupted, err := restarted.turns.InterruptUnfinished(context.Background())
			if err != nil {
				t.Fatal(err)
			}
			cancelledTools, err := cancelUnfinishedRuntimeToolCalls(context.Background(), restarted.toolCalls, conn)
			if err != nil {
				t.Fatal(err)
			}
			for _, turn := range interrupted {
				restarted.storeRuntimeEvent(runtimeapi.Event{
					ID:        newRuntimeEventID(),
					Type:      runtimeapi.EventTurnInterrupted,
					CreatedAt: time.Now().UTC().Format(time.RFC3339Nano),
					SessionID: turn.SessionID,
					TurnID:    turn.ID,
					Payload: map[string]any{
						"status": turnStatusInterrupted,
						"error":  turn.Error,
					},
				})
			}
			for _, call := range cancelledTools {
				restarted.storeRuntimeEvent(runtimeToolCallEvent(runtimeapi.EventToolCallCancelled, call, map[string]any{
					"name":    call.Name,
					"summary": "runtime restarted",
					"status":  string(call.Status),
				}))
			}

			activity, err := restarted.SessionActivity(context.Background(), sessionID)
			if err != nil {
				t.Fatal(err)
			}
			if len(activity.Turns) != 1 || activity.Turns[0].Status != turnStatusInterrupted {
				t.Fatalf("interrupted turn was not restored from SessionActivity: %#v", activity.Turns)
			}
			turn := activity.Turns[0]
			if !slices.Contains(turn.Diagnostics.ProducedArtifacts, structuredPath) {
				t.Fatalf("structured artifact ref missing from diagnostics: %#v", turn.Diagnostics)
			}
			if !slices.Contains(turn.Diagnostics.VerifiedArtifacts, structuredPath) {
				t.Fatalf("structured artifact was not verified on disk: %#v", turn.Diagnostics)
			}
			if slices.Contains(turn.Diagnostics.ProducedArtifacts, prosePath) {
				t.Fatalf("assistant prose-only path counted as produced artifact: %#v", turn.Diagnostics.ProducedArtifacts)
			}
			if turn.Diagnostics.ArtifactCounts.StructuredRefs == 0 {
				t.Fatalf("diagnostics artifact counts missing structured refs: %#v", turn.Diagnostics.ArtifactCounts)
			}
			summary := turn.Interrupted
			if summary == nil {
				t.Fatal("interrupted recovery summary missing")
			}
			if !slices.Contains(summary.ProducedArtifacts, structuredPath) || slices.Contains(summary.ProducedArtifacts, prosePath) {
				t.Fatalf("interrupted artifact summary = %#v", summary.ProducedArtifacts)
			}
			if summary.LastCompletedTool.Source != string(scheduler.ToolSourceMCP) {
				t.Fatalf("last completed tool did not come from MCP path: %#v", summary.LastCompletedTool)
			}
			if summary.LastCompletedTool.Target != structuredPath || len(summary.LastCompletedTool.ArtifactRefs) == 0 {
				t.Fatalf("target/display metadata was not restored: %#v", summary.LastCompletedTool)
			}
			for _, call := range activity.ToolCalls {
				if call.Status == string(scheduler.ToolCallRunning) || call.Status == string(scheduler.ToolCallPending) || call.Status == string(scheduler.ToolCallWaitingPermission) {
					t.Fatalf("stale live tool restored after interruption: %#v", call)
				}
			}
			replay, err := restarted.ReplayExport(context.Background(), RuntimeReplayExportRequest{SessionID: sessionID, TurnID: chat.TurnID})
			if err != nil {
				t.Fatal(err)
			}
			if !slices.ContainsFunc(replay.Summary.ToolCalls, func(call RuntimeToolCall) bool {
				return call.Source == string(scheduler.ToolSourceMCP) && call.Status == string(scheduler.ToolCallCompleted) && call.Display.Target == structuredPath
			}) {
				t.Fatalf("completed MCP scheduler output missing from replay: %#v", replay.Summary.ToolCalls)
			}
			structuredFile := filepath.Base(structuredPath)
			if !slices.ContainsFunc(replay.Summary.ArtifactRefs, func(ref RuntimeRef) bool {
				return ref.ContentType == "structured_output" && strings.Contains(ref.Preview, structuredFile)
			}) {
				t.Fatalf("structured MCP artifact ref missing from replay: %#v", replay.Summary.ArtifactRefs)
			}
		})
	}
}

func TestRuntimeMCPPartialStructuredOutputCancelledOnRestartDoesNotProduceArtifact(t *testing.T) {
	dir := phase61RuntimeDevDir(t)
	workingDir := filepath.Join(dir, "workspace")
	dataDir := filepath.Join(dir, "data")
	if err := os.MkdirAll(workingDir, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(dataDir, 0o700); err != nil {
		t.Fatal(err)
	}
	cfg := config.NewRuntimeConfig(workingDir, dataDir, false)
	cfg.Options.AutoLSP = ptr(false)
	cfg.Options.DisableAutoSummarize = true
	store := config.NewRuntimeStore(workingDir, cfg)
	runtimeWorkbench := workbench.NewWithSchedulerRecorder(context.Background(), store, nil, nil)
	_, workspace, err := runtimeWorkbench.CreateWorkspace(apitypes.Workspace{
		Path:    workingDir,
		DataDir: dataDir,
		Config:  cfg,
		YOLO:    true,
		Env:     os.Environ(),
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { runtimeWorkbench.DeleteWorkspace(workspace.ID) })
	sess, err := runtimeWorkbench.CreateSession(context.Background(), workspace.ID, "phase66 partial")
	if err != nil {
		t.Fatal(err)
	}
	partialPath := filepath.Join(dir, "phase66-partial-should-not-count.json")

	conn, err := db.Connect(context.Background(), dataDir)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Release(dataDir) })

	service := newPhase66RuntimeService(conn, dataDir, nil)
	service.runtime = runtimeWorkbench
	service.workspace = &workspace
	sessionID := sess.ID
	turnID := "turn-phase66-partial"
	if _, err := service.turns.Upsert(context.Background(), RuntimeTurn{
		ID:            turnID,
		SessionID:     sessionID,
		Status:        turnStatusRunning,
		PromptPreview: "Create " + partialPath,
		StartedAt:     time.Now().UTC().Add(-time.Second).UnixMilli(),
	}); err != nil {
		t.Fatal(err)
	}
	call, err := service.toolCalls.CreateCall(context.Background(), scheduler.ToolCallRequest{
		ID:           "tool-phase66-partial",
		SessionID:    sessionID,
		TurnID:       turnID,
		Name:         "mcp_phase66_structured_artifact",
		Source:       scheduler.ToolSourceMCP,
		CapabilityID: "mcp:phase66:structured_artifact",
		InputSummary: `{"path":"` + strings.ReplaceAll(partialPath, `\`, `\\`) + `"}`,
	})
	if err != nil {
		t.Fatal(err)
	}
	partialStructured := `{"artifact_refs":[{"path":"` + strings.ReplaceAll(partialPath, `\`, `\\`) + `"}]}`
	if _, err := service.toolCalls.CompleteCall(context.Background(), scheduler.ToolCallResult{
		ToolCallID:    call.ID,
		Status:        scheduler.ToolCallRunning,
		OutputSummary: partialStructured,
		Structured:    partialStructured,
	}); err != nil {
		t.Fatal(err)
	}

	restarted := newPhase66RuntimeService(conn, dataDir, runtimeWorkbench)
	restarted.workspace = &workspace
	interrupted, err := restarted.turns.InterruptUnfinished(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(interrupted) != 1 {
		t.Fatalf("expected interrupted turn, got %#v", interrupted)
	}
	cancelled, err := cancelUnfinishedRuntimeToolCalls(context.Background(), restarted.toolCalls, conn)
	if err != nil {
		t.Fatal(err)
	}
	if len(cancelled) != 1 || cancelled[0].Status != scheduler.ToolCallCancelled {
		t.Fatalf("expected cancelled unfinished tool, got %#v", cancelled)
	}
	activity, err := restarted.SessionActivity(context.Background(), sessionID)
	if err != nil {
		t.Fatal(err)
	}
	if len(activity.Turns) != 1 || activity.Turns[0].Status != turnStatusInterrupted {
		t.Fatalf("interrupted turn missing from activity: %#v", activity.Turns)
	}
	diag := activity.Turns[0].Diagnostics
	if slices.Contains(diag.ProducedArtifacts, partialPath) || diag.ArtifactCounts.StructuredRefs != 0 {
		t.Fatalf("partial running MCP output must not count as produced: %#v", diag)
	}
	if activity.Turns[0].Interrupted == nil {
		t.Fatal("interrupted summary missing")
	}
	if slices.Contains(activity.Turns[0].Interrupted.ProducedArtifacts, partialPath) {
		t.Fatalf("partial running MCP output leaked into interrupted summary: %#v", activity.Turns[0].Interrupted)
	}
	for _, call := range activity.ToolCalls {
		if call.ID == "tool-phase66-partial" && call.Status != string(scheduler.ToolCallCancelled) {
			t.Fatalf("unfinished tool was not terminal after recovery: %#v", call)
		}
	}
}

func phase61RuntimeDevDir(t *testing.T) string {
	t.Helper()
	name := strings.NewReplacer("/", "-", "\\", "-", " ", "-").Replace(t.Name())
	dir, err := filepath.Abs(filepath.Join("..", "..", "tmp", "runtime-dev", "phase61-tests", name))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(dir, filepath.Join("tmp", "runtime-dev", "phase61-tests")) {
		t.Fatalf("refusing to clean unexpected phase61 test directory: %s", dir)
	}
	_ = os.RemoveAll(dir)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	return dir
}

func registerRuntimeMCPTestProject(t *testing.T, service *runtimeService, workingDir string) runtimeProjectRecord {
	t.Helper()
	store, err := service.projectStore(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	project, err := store.UpsertActiveByPath(context.Background(), workingDir)
	if err != nil {
		t.Fatal(err)
	}
	service.mu.Lock()
	service.activeProjectID = project.ID
	service.projectPath = workingDir
	service.mu.Unlock()
	return project
}

type phase66MCPHTTPServer struct {
	*httptest.Server
	authRequests atomic.Int32
}

type phase66StructuredArtifactInput struct {
	Path string `json:"path"`
}

func newPhase66MCPHTTPServer(t *testing.T, structuredPath, transport string) *phase66MCPHTTPServer {
	t.Helper()
	server := sdkmcp.NewServer(&sdkmcp.Implementation{Name: "phase66-" + transport, Version: "1.0.0"}, nil)
	state := &phase66MCPHTTPServer{}
	sdkmcp.AddTool(server, &sdkmcp.Tool{
		Name:        "structured_artifact",
		Description: "Create a local artifact and return native structured artifact refs.",
	}, func(ctx context.Context, req *sdkmcp.CallToolRequest, input phase66StructuredArtifactInput) (*sdkmcp.CallToolResult, any, error) {
		target := firstNonEmpty(input.Path, structuredPath)
		if err := os.WriteFile(target, []byte(`{"ok":true}`), 0o600); err != nil {
			return nil, nil, err
		}
		payload := map[string]any{
			"artifact_refs": []map[string]any{{
				"path":   target,
				"target": "phase66-report",
				"display": map[string]any{
					"label":  "Phase 6.6 structured report",
					"target": target,
				},
			}},
			"target":         target,
			"display_target": target,
			"metadata": map[string]any{
				"source":     "phase66_" + transport,
				"confidence": "native_structured_content",
			},
		}
		return &sdkmcp.CallToolResult{
			Content:           []sdkmcp.Content{&sdkmcp.TextContent{Text: "created " + target}},
			StructuredContent: payload,
		}, nil, nil
	})

	var handler http.Handler
	switch transport {
	case "sse":
		handler = sdkmcp.NewSSEHandler(func(*http.Request) *sdkmcp.Server { return server }, nil)
	default:
		handler = sdkmcp.NewStreamableHTTPHandler(func(*http.Request) *sdkmcp.Server { return server }, nil)
	}
	httpServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") == "Bearer phase66-test" {
			state.authRequests.Add(1)
		}
		handler.ServeHTTP(w, r)
	}))
	state.Server = httpServer
	return state
}

func newPhase66RuntimeService(conn *sql.DB, dataDir string, runtimeWorkbench *workbench.Service) *runtimeService {
	service := newRuntimeService()
	service.runtime = runtimeWorkbench
	service.turns = newRuntimeTurnStore(conn)
	service.toolCalls = scheduler.New(NewRuntimeToolCallStoreForDB(conn))
	service.permissionStore = newRuntimePermissionStore(conn)
	service.worktrees = newRuntimeWorktreeStore(conn)
	service.agentTasks = newRuntimeAgentTaskStore(conn)
	service.hookExecutions = newRuntimeHookExecutionStore(conn)
	service.mcpRequestStore = newRuntimeMCPRequestStore(conn)
	service.refs = newRuntimeRefStore(conn, dataDir)
	service.eventStore = newRuntimeEventStore(conn)
	return service
}

func waitForPhase61ToolCompletion(ctx context.Context, service *runtimeService, turnID, structuredPath string) error {
	deadline := time.Now().Add(10 * time.Second)
	var last []scheduler.ToolCall
	for time.Now().Before(deadline) {
		calls, err := service.toolCalls.ListCalls(ctx, turnID)
		if err != nil {
			return err
		}
		last = calls
		for _, call := range calls {
			if call.Source != scheduler.ToolSourceMCP {
				continue
			}
			if call.Status == scheduler.ToolCallCompleted {
				return nil
			}
			if call.Status == scheduler.ToolCallFailed || call.Status == scheduler.ToolCallDenied {
				return fmt.Errorf("mcp tool failed before interruption: %#v", call)
			}
		}
		time.Sleep(50 * time.Millisecond)
	}
	return fmt.Errorf("timed out waiting for MCP structured artifact tool completion; calls=%#v", last)
}

func waitForPhase61SecondRequest(ctx context.Context, provider *phase61FakeProvider) bool {
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if provider.secondRequestSeen() {
			return true
		}
		select {
		case <-ctx.Done():
			return provider.secondRequestSeen()
		case <-time.After(20 * time.Millisecond):
		}
	}
	return provider.secondRequestSeen()
}

type phase61FakeProvider struct {
	*httptest.Server
	structuredPath string
	prosePath      string
	toolName       string
	calls          atomic.Int32
	toolCalls      atomic.Int32
	second         chan struct{}
	release        chan struct{}
}

func newPhase61FakeProvider(t *testing.T, structuredPath, prosePath string) *phase61FakeProvider {
	return newPhase66FakeProvider(t, structuredPath, prosePath, "mcp_phase61_structured_artifact")
}

func newPhase66FakeProvider(t *testing.T, structuredPath, prosePath, toolName string) *phase61FakeProvider {
	t.Helper()
	provider := &phase61FakeProvider{
		structuredPath: structuredPath,
		prosePath:      prosePath,
		toolName:       toolName,
		second:         make(chan struct{}),
		release:        make(chan struct{}),
	}
	mux := http.NewServeMux()
	mux.HandleFunc("/v1/models", func(w http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{
			"object": "list",
			"data": []map[string]any{{
				"id":       "phase61-model",
				"object":   "model",
				"created":  1,
				"owned_by": "phase61",
			}},
		})
	})
	mux.HandleFunc("/v1/chat/completions", provider.handleChat)
	provider.Server = httptest.NewServer(mux)
	return provider
}

func (p *phase61FakeProvider) shutdown() {
	closeOnce(p.release)
	p.Close()
}

func (p *phase61FakeProvider) secondRequestSeen() bool {
	select {
	case <-p.second:
		return true
	default:
		return false
	}
}

func (p *phase61FakeProvider) handleChat(w http.ResponseWriter, r *http.Request) {
	p.calls.Add(1)
	body, _ := io.ReadAll(r.Body)
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	flusher, _ := w.(http.Flusher)
	if !phase61RequestHasTools(body) {
		p.writeSSE(w, map[string]any{
			"id":      "chatcmpl-phase61-title",
			"object":  "chat.completion.chunk",
			"created": 1,
			"model":   "phase61-model",
			"choices": []map[string]any{{
				"index":         0,
				"delta":         map[string]any{"role": "assistant", "content": "Phase 6.1"},
				"finish_reason": nil,
			}},
		})
		p.writeSSE(w, map[string]any{
			"id":      "chatcmpl-phase61-title",
			"object":  "chat.completion.chunk",
			"created": 1,
			"model":   "phase61-model",
			"choices": []map[string]any{{
				"index":         0,
				"delta":         map[string]any{},
				"finish_reason": "stop",
			}},
		})
		_, _ = fmt.Fprint(w, "data: [DONE]\n\n")
		if flusher != nil {
			flusher.Flush()
		}
		return
	}
	n := p.toolCalls.Add(1)
	if n == 1 {
		args, _ := json.Marshal(map[string]string{"path": p.structuredPath})
		toolCall := map[string]any{
			"index": 0,
			"id":    "call_phase61_structured",
			"type":  "function",
			"function": map[string]any{
				"name":      p.toolName,
				"arguments": string(args),
			},
		}
		p.writeSSE(w, map[string]any{
			"id":      "chatcmpl-phase61-1",
			"object":  "chat.completion.chunk",
			"created": 1,
			"model":   "phase61-model",
			"choices": []map[string]any{{
				"index": 0,
				"delta": map[string]any{
					"role":    "assistant",
					"content": "Mentioning " + p.prosePath + " in assistant prose only.",
				},
				"finish_reason": nil,
			}},
		})
		p.writeSSE(w, map[string]any{
			"id":      "chatcmpl-phase61-1",
			"object":  "chat.completion.chunk",
			"created": 1,
			"model":   "phase61-model",
			"choices": []map[string]any{{
				"index": 0,
				"delta": map[string]any{
					"tool_calls": []map[string]any{toolCall},
				},
				"finish_reason": nil,
			}},
		})
		_, _ = fmt.Fprint(w, "data: [DONE]\n\n")
		if flusher != nil {
			flusher.Flush()
		}
		return
	}
	closeOnce(p.second)
	select {
	case <-p.release:
	case <-r.Context().Done():
	}
}

func (p *phase61FakeProvider) writeSSE(w http.ResponseWriter, payload map[string]any) {
	data, _ := json.Marshal(payload)
	_, _ = fmt.Fprintf(w, "data: %s\n\n", data)
	if flusher, ok := w.(http.Flusher); ok {
		flusher.Flush()
	}
}

func phase61RequestHasTools(body []byte) bool {
	var payload struct {
		Tools []any `json:"tools"`
	}
	_ = json.Unmarshal(body, &payload)
	return len(payload.Tools) > 0
}

func closeOnce(ch chan struct{}) {
	defer func() { _ = recover() }()
	close(ch)
}

func phase61MCPServerScript() string {
	return `
const fs = require("fs");
const readline = require("readline");

const rl = readline.createInterface({ input: process.stdin });

function send(value) {
  process.stdout.write(JSON.stringify(value) + "\n");
}

function toolList(id) {
  send({
    jsonrpc: "2.0",
    id,
    result: {
      tools: [{
        name: "structured_artifact",
        description: "Create a local artifact and return a machine-readable artifact ref.",
        inputSchema: {
          type: "object",
          properties: { path: { type: "string" } },
          required: ["path"]
        }
      }]
    }
  });
}

rl.on("line", (line) => {
  if (!line.trim()) return;
  const msg = JSON.parse(line);
  if (msg.method === "initialize") {
    send({
      jsonrpc: "2.0",
      id: msg.id,
      result: {
        protocolVersion: "2025-06-18",
        capabilities: { tools: {} },
        serverInfo: { name: "phase61-external-mcp", version: "1.0.0" }
      }
    });
    return;
  }
  if (msg.method === "tools/list") {
    toolList(msg.id);
    return;
  }
  if (msg.method === "tools/call") {
    const args = msg.params && msg.params.arguments ? msg.params.arguments : {};
    const target = args.path || "";
    fs.writeFileSync(target, JSON.stringify({ ok: true, target }, null, 2));
    const payload = {
      artifact_refs: [{
        path: target,
        target: "phase61-report",
        display: { label: "Phase 6.1 structured report", target }
      }],
      target,
      display_target: target,
      metadata: { source: "external_mcp", confidence: "structured_ref" }
    };
    send({
      jsonrpc: "2.0",
      id: msg.id,
      result: {
        content: [{ type: "text", text: JSON.stringify(payload) }],
        structuredContent: payload,
        isError: false
      }
    });
    return;
  }
  if (msg.id !== undefined) {
    send({ jsonrpc: "2.0", id: msg.id, result: {} });
  }
});
`
}
