package runtime

import (
	"context"
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
	mcptools "github.com/charmbracelet/crush/internal/agent/tools/mcp"
	"github.com/charmbracelet/crush/internal/backend"
	"github.com/charmbracelet/crush/internal/config"
	"github.com/charmbracelet/crush/internal/db"
	"github.com/charmbracelet/crush/internal/permission"
	"github.com/charmbracelet/crush/internal/proto"
	"github.com/charmbracelet/crush/internal/runtimeapi"
	"github.com/charmbracelet/crush/internal/tools/scheduler"
)

func TestRuntimeExternalMCPInterruptedStructuredRefsFixture(t *testing.T) {
	_ = mcptools.Close(context.Background())
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	t.Cleanup(func() { _ = mcptools.Close(context.Background()) })

	dir := phase61RuntimeDevDir(t)
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
	service.compactBoundaries = newRuntimeCompactBoundaryStore(conn)
	service.worktrees = newRuntimeWorktreeStore(conn)
	service.agentTasks = newRuntimeAgentTaskStore(conn)
	service.hookExecutions = newRuntimeHookExecutionStore(conn)
	service.mcpRequestStore = newRuntimeMCPRequestStore(conn)
	service.refs = newRuntimeRefStore(conn, dataDir)
	service.eventStore = newRuntimeEventStore(conn)
	service.policy = runtimePolicyFromParts(permission.PolicyModeFullAccess, "phase61", nil, time.Now().UnixMilli())

	store := config.NewRuntimeStore(workingDir, cfg)
	recorder := &runtimeSchedulerRecorder{service: service}
	runtimeBackend := backend.NewWithSchedulerRecorder(ctx, store, nil, recorder)
	_, workspace, err := runtimeBackend.CreateWorkspace(proto.Workspace{
		Path:    workingDir,
		DataDir: dataDir,
		Config:  cfg,
		YOLO:    true,
		Env:     os.Environ(),
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { runtimeBackend.DeleteWorkspace(workspace.ID) })
	if err := mcptools.InitializeSingle(ctx, "phase61", store); err != nil {
		t.Fatalf("initialize phase61 mcp: %v", err)
	}
	mcptools.RefreshTools(ctx, store, "phase61")
	if err := mcptools.WaitForInit(ctx); err != nil {
		t.Fatalf("mcp init failed: %v", err)
	}
	if err := runtimeBackend.UpdateAgent(ctx, workspace.ID); err != nil {
		t.Fatalf("update agent with MCP tools: %v", err)
	}
	service.runtime = runtimeBackend
	service.workspace = &proto.Workspace{ID: workspace.ID, Path: workspace.Path}
	service.runtimeCtx = ctx
	service.cancel = cancel

	chat, err := service.Chat(ctx, RuntimeChatRequest{
		Prompt: "Use the external MCP structured_artifact tool to create " + structuredPath + ". Do not use " + prosePath + " as an artifact.",
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
	if !provider.secondRequestSeen() {
		t.Fatal("fake provider never received the post-tool request; turn was not interrupted before final completion")
	}

	cancel()
	restarted := newRuntimeService()
	restarted.runtime = runtimeBackend
	restarted.workspace = &proto.Workspace{ID: workspace.ID, Path: workspace.Path}
	restarted.turns = newRuntimeTurnStore(conn)
	restarted.toolCalls = scheduler.New(NewRuntimeToolCallStoreForDB(conn))
	restarted.permissionStore = newRuntimePermissionStore(conn)
	restarted.compactBoundaries = newRuntimeCompactBoundaryStore(conn)
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

type phase61FakeProvider struct {
	*httptest.Server
	structuredPath string
	prosePath      string
	calls          atomic.Int32
	toolCalls      atomic.Int32
	second         chan struct{}
	release        chan struct{}
}

func newPhase61FakeProvider(t *testing.T, structuredPath, prosePath string) *phase61FakeProvider {
	t.Helper()
	provider := &phase61FakeProvider{
		structuredPath: structuredPath,
		prosePath:      prosePath,
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
				"name":      "mcp_phase61_structured_artifact",
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
