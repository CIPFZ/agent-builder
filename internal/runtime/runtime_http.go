package runtime

import (
	"context"
	"crypto/subtle"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"mime"
	"net"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/CIPFZ/agent-builder/internal/config"
	"github.com/gorilla/websocket"
)

type RuntimeAPIEndpointResponse struct {
	URL   string `json:"url"`
	Token string `json:"token"`
}

type runtimeHTTPServer struct {
	mu       sync.Mutex
	service  RuntimeService
	server   *http.Server
	listener net.Listener
	url      string
	token    string
}

var runtimeTerminalWebSocketUpgrader = websocket.Upgrader{
	CheckOrigin: func(*http.Request) bool {
		return true
	},
}

const (
	runtimeTerminalStreamBatchBytes = 64 * 1024
	runtimeTerminalStreamBatchWait  = 8 * time.Millisecond
	runtimeTerminalStreamAckTimeout = 15 * time.Second
	runtimeTerminalStreamReadLimit  = 1024 * 1024
)

func newRuntimeHTTPServer(service RuntimeService) *runtimeHTTPServer {
	return &runtimeHTTPServer{
		service: service,
		token:   newStreamToken(),
	}
}

func (s *runtimeHTTPServer) Start() error {
	return s.start("127.0.0.1:0")
}

func (s *runtimeHTTPServer) StartAt(address, token string) error {
	if strings.TrimSpace(token) != "" {
		s.mu.Lock()
		s.token = strings.TrimSpace(token)
		s.mu.Unlock()
	}
	return s.start(firstNonEmpty(strings.TrimSpace(address), "127.0.0.1:0"))
}

func (s *runtimeHTTPServer) start(address string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.url != "" {
		return nil
	}

	listener, err := net.Listen("tcp", address)
	if err != nil {
		return fmt.Errorf("failed to listen for runtime HTTP API: %w", err)
	}

	server := &http.Server{
		Handler:           s,
		ReadHeaderTimeout: 5 * time.Second,
	}
	s.listener = listener
	s.server = server
	s.url = fmt.Sprintf("http://%s", listener.Addr().String())

	go func() {
		if err := server.Serve(listener); err != nil && err != http.ErrServerClosed {
			slog.Error("Runtime HTTP API server stopped", "error", err)
		}
	}()

	return nil
}

func (s *runtimeHTTPServer) Close(ctx context.Context) error {
	s.mu.Lock()
	server := s.server
	s.server = nil
	s.listener = nil
	s.url = ""
	s.mu.Unlock()

	if server == nil {
		return nil
	}
	return server.Shutdown(ctx)
}

func (s *runtimeHTTPServer) URL() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.url
}

func (s *runtimeHTTPServer) Token() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.token
}

func (s *runtimeHTTPServer) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodOptions {
		writeRuntimeJSON(w, http.StatusNoContent, nil)
		return
	}
	if r.Method == http.MethodGet && r.URL.Path == "/v1/dev/jsonp" {
		s.handleJSONP(w, r)
		return
	}
	if r.Method == http.MethodGet && r.URL.Path == "/v1/dev/module" {
		s.handleDevModule(w, r)
		return
	}
	if r.Method == http.MethodGet && !strings.HasPrefix(r.URL.Path, "/v1/") {
		if s.handleClientAsset(w, r) {
			return
		}
	}
	if !s.authorized(r) {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}

	switch {
	case r.Method == http.MethodGet && r.URL.Path == "/v1/runtime/status":
		value, err := s.service.Status(r.Context())
		writeRuntimeResult(w, value, err)
	case r.Method == http.MethodGet && r.URL.Path == "/v1/sidebar-projection":
		value, err := s.service.SidebarProjection(r.Context())
		writeRuntimeResult(w, value, err)
	case r.Method == http.MethodGet && r.URL.Path == "/v1/projects":
		value, err := s.service.Projects(r.Context())
		writeRuntimeResult(w, value, err)
	case r.Method == http.MethodPost && r.URL.Path == "/v1/projects/open":
		var req RuntimeOpenProjectRequest
		if !decodeRuntimeJSON(w, r, &req) {
			return
		}
		value, err := s.service.OpenProject(r.Context(), req)
		writeRuntimeResult(w, value, err)
	case r.Method == http.MethodPost && r.URL.Path == "/v1/projects":
		var req RuntimeCreateProjectRequest
		if !decodeRuntimeJSON(w, r, &req) {
			return
		}
		value, err := s.service.CreateProject(r.Context(), req)
		writeRuntimeResult(w, value, err)
	case r.Method == http.MethodPost && projectRenamePathID(r.URL.Path) != "":
		var req RuntimeRenameProjectRequest
		if !decodeRuntimeJSON(w, r, &req) {
			return
		}
		req.ProjectID = projectRenamePathID(r.URL.Path)
		value, err := s.service.RenameProject(r.Context(), req)
		writeRuntimeResult(w, value, err)
	case r.Method == http.MethodPost && projectOpenExplorerPathID(r.URL.Path) != "":
		value, err := s.service.OpenProjectInExplorer(r.Context(), RuntimeProjectActionRequest{
			ProjectID: projectOpenExplorerPathID(r.URL.Path),
		})
		writeRuntimeResult(w, value, err)
	case r.Method == http.MethodDelete && projectPathID(r.URL.Path) != "":
		value, err := s.service.RemoveProject(r.Context(), RuntimeProjectActionRequest{
			ProjectID: projectPathID(r.URL.Path),
		})
		writeRuntimeResult(w, value, err)
	case r.Method == http.MethodGet && projectMemoryPathID(r.URL.Path) != "":
		value, err := s.service.ProjectMemories(r.Context(), projectMemoryPathID(r.URL.Path))
		writeRuntimeResult(w, value, err)
	case r.Method == http.MethodPost && projectMemoryPathID(r.URL.Path) != "":
		var req RuntimeMemoryCreateRequest
		if !decodeRuntimeJSON(w, r, &req) {
			return
		}
		req.ProjectID = projectMemoryPathID(r.URL.Path)
		value, err := s.service.CreateProjectMemory(r.Context(), req)
		writeRuntimeResult(w, value, err)
	case r.Method == http.MethodPost && projectMemoryRefreshPathID(r.URL.Path) != "":
		value, err := s.service.RefreshProjectMemoryIndex(r.Context(), projectMemoryRefreshPathID(r.URL.Path))
		writeRuntimeResult(w, value, err)
	case r.Method == http.MethodGet && projectMemoryDiagnosticsPathID(r.URL.Path) != "":
		value, err := s.service.ProjectMemoryDiagnostics(r.Context(), projectMemoryDiagnosticsPathID(r.URL.Path))
		writeRuntimeResult(w, value, err)
	case r.Method == http.MethodGet && memoryPathID(r.URL.Path) != "":
		value, err := s.service.ProjectMemory(r.Context(), memoryPathID(r.URL.Path))
		writeRuntimeResult(w, value, err)
	case (r.Method == http.MethodPut || r.Method == http.MethodPatch) && memoryPathID(r.URL.Path) != "":
		var req RuntimeMemoryUpdateRequest
		if !decodeRuntimeJSON(w, r, &req) {
			return
		}
		value, err := s.service.UpdateProjectMemory(r.Context(), memoryPathID(r.URL.Path), req)
		writeRuntimeResult(w, value, err)
	case r.Method == http.MethodPost && memoryDisablePathID(r.URL.Path) != "":
		var req RuntimeMemoryDisableRequest
		if r.Body != nil && r.ContentLength != 0 && !decodeRuntimeJSON(w, r, &req) {
			return
		}
		value, err := s.service.DisableProjectMemory(r.Context(), memoryDisablePathID(r.URL.Path), req)
		writeRuntimeResult(w, value, err)
	case (r.Method == http.MethodDelete && memoryPathID(r.URL.Path) != "") || (r.Method == http.MethodPost && memoryDeletePathID(r.URL.Path) != ""):
		var req RuntimeMemoryDeleteRequest
		if r.Body != nil && r.ContentLength != 0 && !decodeRuntimeJSON(w, r, &req) {
			return
		}
		id := memoryPathID(r.URL.Path)
		if id == "" {
			id = memoryDeletePathID(r.URL.Path)
		}
		value, err := s.service.DeleteProjectMemory(r.Context(), id, req)
		writeRuntimeResult(w, value, err)
	case r.Method == http.MethodGet && r.URL.Path == "/v1/recovery/status":
		value, err := s.service.RecoveryStatus(r.Context())
		writeRuntimeResult(w, value, err)
	case r.Method == http.MethodPost && recoveryTurnResumePathID(r.URL.Path) != "":
		var req RuntimeResumeInterruptedTurnRequest
		if r.Body != nil && r.ContentLength != 0 && !decodeRuntimeJSON(w, r, &req) {
			return
		}
		value, err := s.service.ResumeInterruptedTurn(r.Context(), recoveryTurnResumePathID(r.URL.Path), req)
		writeRuntimeResult(w, value, err)
	case r.Method == http.MethodPost && recoveryTurnDiscardPathID(r.URL.Path) != "":
		value, err := s.service.DiscardInterruptedTurn(r.Context(), recoveryTurnDiscardPathID(r.URL.Path))
		writeRuntimeResult(w, value, err)
	case r.Method == http.MethodPost && recoveryErrorRetryPathID(r.URL.Path) != "":
		value, err := s.service.RetryRecoverableError(r.Context(), recoveryErrorRetryPathID(r.URL.Path))
		writeRuntimeResult(w, value, err)
	case r.Method == http.MethodGet && r.URL.Path == "/v1/config/model":
		value, err := s.service.GetModelConfig(r.Context())
		writeRuntimeResult(w, value, err)
	case r.Method == http.MethodGet && r.URL.Path == "/v1/config/providers":
		value, err := s.service.ProviderCatalog(r.Context())
		writeRuntimeResult(w, value, err)
	case r.Method == http.MethodGet && r.URL.Path == "/v1/config/configured-providers":
		value, err := s.service.ConfiguredProviders(r.Context())
		writeRuntimeResult(w, value, err)
	case r.Method == http.MethodPost && r.URL.Path == "/v1/config/configured-providers":
		var req RuntimeConfiguredProviderRequest
		if !decodeRuntimeJSON(w, r, &req) {
			return
		}
		value, err := s.service.SaveConfiguredProvider(r.Context(), req)
		writeRuntimeResult(w, value, err)
	case r.Method == http.MethodPut && configuredProviderPathID(r.URL.Path) != "":
		var req RuntimeConfiguredProviderRequest
		if !decodeRuntimeJSON(w, r, &req) {
			return
		}
		req.ID = configuredProviderPathID(r.URL.Path)
		value, err := s.service.SaveConfiguredProvider(r.Context(), req)
		writeRuntimeResult(w, value, err)
	case r.Method == http.MethodDelete && configuredProviderPathID(r.URL.Path) != "":
		value, err := s.service.DeleteConfiguredProvider(r.Context(), configuredProviderPathID(r.URL.Path))
		writeRuntimeResult(w, value, err)
	case r.Method == http.MethodPost && configuredProviderModelsPathID(r.URL.Path) != "":
		value, err := s.service.DiscoverConfiguredProviderModels(r.Context(), configuredProviderModelsPathID(r.URL.Path))
		writeRuntimeResult(w, value, err)
	case r.Method == http.MethodPost && configuredProviderTestPathID(r.URL.Path) != "":
		value, err := s.service.TestConfiguredProvider(r.Context(), configuredProviderTestPathID(r.URL.Path))
		writeRuntimeResult(w, value, err)
	case r.Method == http.MethodPost && configuredProviderLatencyPathID(r.URL.Path) != "":
		value, err := s.service.MeasureConfiguredProviderLatency(r.Context(), configuredProviderLatencyPathID(r.URL.Path))
		writeRuntimeResult(w, value, err)
	case r.Method == http.MethodPost && r.URL.Path == "/v1/config/model/verify":
		var req RuntimeModelConfig
		if !decodeRuntimeJSON(w, r, &req) {
			return
		}
		value, err := s.service.VerifyModelConfig(r.Context(), req)
		writeRuntimeResult(w, value, err)
	case r.Method == http.MethodPost && r.URL.Path == "/v1/config/model/discover":
		var req RuntimeModelConfig
		if !decodeRuntimeJSON(w, r, &req) {
			return
		}
		value, err := s.service.DiscoverModelConfig(r.Context(), req)
		writeRuntimeResult(w, value, err)
	case r.Method == http.MethodPut && r.URL.Path == "/v1/config/model":
		var req RuntimeModelConfig
		if !decodeRuntimeJSON(w, r, &req) {
			return
		}
		value, err := s.service.SaveModelConfig(r.Context(), req)
		writeRuntimeResult(w, value, err)
	case r.Method == http.MethodGet && r.URL.Path == "/v1/config/models":
		value, err := s.service.Models(r.Context())
		writeRuntimeResult(w, value, err)
	case r.Method == http.MethodGet && r.URL.Path == "/v1/config/selected-model":
		value, err := s.service.SelectedModel(r.Context())
		writeRuntimeResult(w, value, err)
	case r.Method == http.MethodPut && r.URL.Path == "/v1/config/selected-model":
		var req RuntimeSelectedModelRequest
		if !decodeRuntimeJSON(w, r, &req) {
			return
		}
		value, err := s.service.SaveSelectedModel(r.Context(), req)
		writeRuntimeResult(w, value, err)
	case r.Method == http.MethodGet && r.URL.Path == "/v1/permissions":
		value, err := s.service.Permissions(r.Context())
		writeRuntimeResult(w, value, err)
	case r.Method == http.MethodGet && r.URL.Path == "/v1/policy":
		value, err := s.service.GetPolicy(r.Context())
		writeRuntimeResult(w, value, err)
	case r.Method == http.MethodPut && r.URL.Path == "/v1/policy":
		var req RuntimePolicyUpdateRequest
		if !decodeRuntimeJSON(w, r, &req) {
			return
		}
		value, err := s.service.UpdatePolicy(r.Context(), req)
		writeRuntimeResult(w, value, err)
	case r.Method == http.MethodGet && r.URL.Path == "/v1/settings/context-governance":
		value, err := s.service.ContextGovernanceSettings(r.Context())
		writeRuntimeResult(w, value, err)
	case r.Method == http.MethodPut && r.URL.Path == "/v1/settings/context-governance":
		var req RuntimeContextGovernanceSettings
		if !decodeRuntimeJSON(w, r, &req) {
			return
		}
		value, err := s.service.SaveContextGovernanceSettings(r.Context(), req)
		writeRuntimeResult(w, value, err)
	case r.Method == http.MethodPost && permissionDecisionPath(r.URL.Path) != "":
		permissionID := permissionDecisionPath(r.URL.Path)
		var req RuntimePermissionDecision
		if !decodeRuntimeJSON(w, r, &req) {
			return
		}
		req.PermissionID = permissionID
		value, err := s.service.DecidePermission(r.Context(), req)
		writeRuntimeResult(w, value, err)
	case r.Method == http.MethodPost && r.URL.Path == "/v1/sessions":
		var req RuntimeSessionCreateRequest
		if r.Body != nil && r.ContentLength != 0 && !decodeRuntimeJSON(w, r, &req) {
			return
		}
		value, err := s.service.CreateSession(r.Context(), req)
		writeRuntimeResult(w, value, err)
	case r.Method == http.MethodPost && r.URL.Path == "/v1/runtime/new-chat":
		var req RuntimeSessionCreateRequest
		if r.Body != nil && r.ContentLength != 0 && !decodeRuntimeJSON(w, r, &req) {
			return
		}
		value, err := s.service.NewChat(r.Context(), req.Title)
		writeRuntimeResult(w, value, err)
	case r.Method == http.MethodGet && r.URL.Path == "/v1/sessions":
		value, err := s.service.Sessions(r.Context())
		writeRuntimeResult(w, value, err)
	case r.Method == http.MethodGet && sessionMessagesPathID(r.URL.Path) != "":
		value, err := s.service.SessionMessages(r.Context(), sessionMessagesPathID(r.URL.Path))
		writeRuntimeResult(w, value, err)
	case r.Method == http.MethodGet && sessionContextUsagePathID(r.URL.Path) != "":
		value, err := s.service.SessionContextUsage(r.Context(), sessionContextUsagePathID(r.URL.Path))
		writeRuntimeResult(w, value, err)
	case r.Method == http.MethodPost && sessionCompactPathID(r.URL.Path) != "":
		var req RuntimeContextActionRequest
		if !decodeRuntimeJSON(w, r, &req) {
			return
		}
		req.SessionID = sessionCompactPathID(r.URL.Path)
		value, err := s.service.ManualCompact(r.Context(), req)
		writeRuntimeResult(w, value, err)
	case r.Method == http.MethodGet && sessionOutputStreamPathID(r.URL.Path) != "":
		s.handleSessionOutputStream(w, r, sessionOutputStreamPathID(r.URL.Path))
	case r.Method == http.MethodGet && sessionOutputEventsPathID(r.URL.Path) != "":
		value, err := s.service.SessionOutputEvents(r.Context(), sessionOutputEventsPathID(r.URL.Path), runtimeQueryCursor(r))
		writeRuntimeResult(w, value, err)
	case r.Method == http.MethodGet && sessionOutputPathID(r.URL.Path) != "":
		value, err := s.service.SessionOutput(r.Context(), sessionOutputPathID(r.URL.Path), RuntimeOutputRequest{
			Snapshot: true,
			Cursor:   runtimeQueryCursor(r),
			Limit:    runtimeQueryLimit(r),
		})
		writeRuntimeResult(w, value, err)
	case r.Method == http.MethodGet && sessionActivityPathID(r.URL.Path) != "":
		value, err := s.service.SessionActivity(r.Context(), sessionActivityPathID(r.URL.Path))
		writeRuntimeResult(w, value, err)
	case r.Method == http.MethodGet && sessionActivityWindowPathID(r.URL.Path) != "":
		value, err := s.service.SessionActivityCursorWindow(r.Context(), sessionActivityWindowPathID(r.URL.Path), runtimeQueryCursor(r), runtimeQueryLimit(r))
		writeRuntimeResult(w, value, err)
	case r.Method == http.MethodGet && sessionReactCallchainPathID(r.URL.Path) != "":
		value, err := s.service.SessionReactCallchain(r.Context(), sessionReactCallchainPathID(r.URL.Path), runtimeQueryLimit(r))
		writeRuntimeResult(w, value, err)
	case r.Method == http.MethodGet && sessionPromptAssembliesPathID(r.URL.Path) != "":
		value, err := s.service.PromptAssembliesBySession(r.Context(), sessionPromptAssembliesPathID(r.URL.Path), runtimeQueryLimit(r))
		writeRuntimeResult(w, value, err)
	case r.Method == http.MethodGet && sessionTerminalsPathID(r.URL.Path) != "":
		value, err := s.service.SessionTerminals(r.Context(), sessionTerminalsPathID(r.URL.Path))
		writeRuntimeResult(w, value, err)
	case r.Method == http.MethodGet && sessionAgentTasksPathID(r.URL.Path) != "":
		value, err := s.service.SessionAgentTasks(r.Context(), sessionAgentTasksPathID(r.URL.Path))
		writeRuntimeResult(w, value, err)
	case r.Method == http.MethodGet && sessionRunProjectionPathID(r.URL.Path) != "":
		value, err := s.service.RunProjection(r.Context(), RuntimeRunProjectionRequest{
			SessionID: sessionRunProjectionPathID(r.URL.Path),
			Cursor:    runtimeQueryCursor(r),
			Limit:     runtimeQueryLimit(r),
		})
		writeRuntimeResult(w, value, err)
	case r.Method == http.MethodGet && r.URL.Path == "/v1/run-transitions":
		value, err := s.service.RunTransitionHistory(r.Context(), RuntimeRunTransitionHistoryRequest{
			RunID:     r.URL.Query().Get("run_id"),
			SessionID: r.URL.Query().Get("session_id"),
			TurnID:    r.URL.Query().Get("turn_id"),
			Cursor:    runtimeQueryCursor(r),
			Limit:     runtimeQueryLimit(r),
		})
		writeRuntimeResult(w, value, err)
	case r.Method == http.MethodGet && r.URL.Path == "/v1/run-scheduler-plan":
		value, err := s.service.RunSchedulerPlan(r.Context(), RuntimeRunSchedulerPlanRequest{
			RunID:        r.URL.Query().Get("run_id"),
			SessionID:    r.URL.Query().Get("session_id"),
			Mode:         r.URL.Query().Get("mode"),
			TurnID:       r.URL.Query().Get("turn_id"),
			CheckpointID: r.URL.Query().Get("checkpoint_id"),
			TaskID:       r.URL.Query().Get("task_id"),
			Cursor:       runtimeQueryCursor(r),
			Limit:        runtimeQueryLimit(r),
		})
		writeRuntimeResult(w, value, err)
	case r.Method == http.MethodGet && sessionTodosPathID(r.URL.Path) != "":
		value, err := s.service.SessionTodos(r.Context(), sessionTodosPathID(r.URL.Path))
		writeRuntimeResult(w, value, err)
	case r.Method == http.MethodPost && sessionTurnsPathID(r.URL.Path) != "":
		var req RuntimeChatRequest
		if !decodeRuntimeJSON(w, r, &req) {
			return
		}
		req.SessionID = sessionTurnsPathID(r.URL.Path)
		value, err := s.service.Chat(r.Context(), req)
		writeRuntimeResult(w, value, err)
	case r.Method == http.MethodPost && r.URL.Path == "/v1/turns":
		var req RuntimeChatRequest
		if !decodeRuntimeJSON(w, r, &req) {
			return
		}
		value, err := s.service.Chat(r.Context(), req)
		writeRuntimeResult(w, value, err)
	case r.Method == http.MethodPost && r.URL.Path == "/v1/user-inputs":
		var req RuntimeUserInputRequest
		if !decodeRuntimeJSON(w, r, &req) {
			return
		}
		value, err := s.service.SubmitUserInput(r.Context(), req)
		writeRuntimeResult(w, value, err)
	case r.Method == http.MethodPost && r.URL.Path == "/v1/context/manual-compact":
		var req RuntimeContextActionRequest
		if !decodeRuntimeJSON(w, r, &req) {
			return
		}
		value, err := s.service.ManualCompact(r.Context(), req)
		writeRuntimeResult(w, value, err)
	case r.Method == http.MethodPost && r.URL.Path == "/v1/context/manual-snip":
		var req RuntimeContextActionRequest
		if !decodeRuntimeJSON(w, r, &req) {
			return
		}
		value, err := s.service.ManualSnip(r.Context(), req)
		writeRuntimeResult(w, value, err)
	case r.Method == http.MethodPost && r.URL.Path == "/v1/terminals":
		var req RuntimeTerminalCreateRequest
		if r.Body != nil && r.ContentLength != 0 && !decodeRuntimeJSON(w, r, &req) {
			return
		}
		value, err := s.service.CreateTerminal(r.Context(), req)
		writeRuntimeResult(w, value, err)
	case r.Method == http.MethodGet && terminalStreamPathID(r.URL.Path) != "":
		s.handleTerminalStream(w, r, terminalStreamPathID(r.URL.Path))
	case r.Method == http.MethodDelete && terminalPathID(r.URL.Path) != "":
		value, err := s.service.DeleteTerminal(r.Context(), terminalPathID(r.URL.Path))
		writeRuntimeResult(w, value, err)
	case r.Method == http.MethodGet && r.URL.Path == "/v1/turns":
		value, err := s.service.Turns(r.Context(), r.URL.Query().Get("status"))
		writeRuntimeResult(w, value, err)
	case r.Method == http.MethodGet && r.URL.Path == "/v1/runs":
		value, err := s.service.Runs(r.Context())
		writeRuntimeResult(w, value, err)
	case r.Method == http.MethodGet && r.URL.Path == "/v1/run-summaries":
		value, err := s.service.RunSummaries(r.Context())
		writeRuntimeResult(w, value, err)
	case r.Method == http.MethodGet && runSummaryPathID(r.URL.Path) != "":
		value, err := s.service.RunSummary(r.Context(), runSummaryPathID(r.URL.Path))
		writeRuntimeResult(w, value, err)
	case r.Method == http.MethodGet && runCheckpointMarkersPathID(r.URL.Path) != "":
		value, err := s.service.RunCheckpointMarkers(r.Context(), runCheckpointMarkersPathID(r.URL.Path))
		writeRuntimeResult(w, value, err)
	case r.Method == http.MethodGet && runCheckpointMarkerPathIDs(r.URL.Path).runID != "":
		ids := runCheckpointMarkerPathIDs(r.URL.Path)
		value, err := s.service.RunCheckpointMarker(r.Context(), ids.runID, ids.checkpointID)
		writeRuntimeResult(w, value, err)
	case r.Method == http.MethodGet && runPathID(r.URL.Path) != "":
		value, err := s.service.Run(r.Context(), runPathID(r.URL.Path))
		writeRuntimeResult(w, value, err)
	case r.Method == http.MethodPost && runCheckpointAcknowledgePathIDs(r.URL.Path).runID != "":
		ids := runCheckpointAcknowledgePathIDs(r.URL.Path)
		value, err := s.service.AcknowledgeRunCheckpoint(r.Context(), ids.runID, ids.checkpointID)
		writeRuntimeResult(w, value, err)
	case r.Method == http.MethodPost && runCheckpointDiscardPathIDs(r.URL.Path).runID != "":
		ids := runCheckpointDiscardPathIDs(r.URL.Path)
		value, err := s.service.DiscardRunCheckpoint(r.Context(), ids.runID, ids.checkpointID)
		writeRuntimeResult(w, value, err)
	case r.Method == http.MethodPost && runCheckpointResumePathIDs(r.URL.Path).runID != "":
		ids := runCheckpointResumePathIDs(r.URL.Path)
		value, err := s.service.ResumeRunCheckpoint(r.Context(), ids.runID, ids.checkpointID)
		writeRuntimeResult(w, value, err)
	case r.Method == http.MethodPost && runTaskExecutePathIDs(r.URL.Path).runID != "":
		ids := runTaskExecutePathIDs(r.URL.Path)
		value, err := s.service.ExecuteRunTask(r.Context(), ids.runID, ids.taskID)
		writeRuntimeResult(w, value, err)
	case r.Method == http.MethodGet && r.URL.Path == "/v1/refs":
		value, err := s.service.Refs(r.Context(), RuntimeRefListRequest{
			SessionID:  r.URL.Query().Get("session_id"),
			TurnID:     r.URL.Query().Get("turn_id"),
			ToolCallID: r.URL.Query().Get("tool_call_id"),
			TaskID:     r.URL.Query().Get("task_id"),
			Kind:       r.URL.Query().Get("kind"),
		})
		writeRuntimeResult(w, value, err)
	case r.Method == http.MethodGet && r.URL.Path == "/v1/sandbox/decisions":
		value, err := s.service.SandboxDecisions(r.Context(), RuntimeSandboxDecisionListRequest{
			SessionID:  r.URL.Query().Get("session_id"),
			TurnID:     r.URL.Query().Get("turn_id"),
			ToolCallID: r.URL.Query().Get("tool_call_id"),
			TaskID:     r.URL.Query().Get("task_id"),
		})
		writeRuntimeResult(w, value, err)
	case r.Method == http.MethodGet && sandboxDecisionPathID(r.URL.Path) != "":
		value, err := s.service.SandboxDecision(r.Context(), sandboxDecisionPathID(r.URL.Path))
		writeRuntimeResult(w, value, err)
	case r.Method == http.MethodGet && refContentPathID(r.URL.Path) != "":
		value, err := s.service.ReadRefContent(r.Context(), refContentPathID(r.URL.Path))
		writeRuntimeResult(w, value, err)
	case r.Method == http.MethodGet && refPathID(r.URL.Path) != "":
		value, err := s.service.Ref(r.Context(), refPathID(r.URL.Path))
		writeRuntimeResult(w, value, err)
	case r.Method == http.MethodGet && turnToolCallsPathID(r.URL.Path) != "":
		value, err := s.service.TurnToolCalls(r.Context(), turnToolCallsPathID(r.URL.Path))
		writeRuntimeResult(w, value, err)
	case r.Method == http.MethodGet && turnReactCallchainPathID(r.URL.Path) != "":
		value, err := s.service.ReactCallchain(r.Context(), turnReactCallchainPathID(r.URL.Path))
		writeRuntimeResult(w, value, err)
	case r.Method == http.MethodGet && userInputPathID(r.URL.Path) != "":
		value, err := s.service.UserInput(r.Context(), userInputPathID(r.URL.Path))
		writeRuntimeResult(w, value, err)
	case r.Method == http.MethodGet && turnActivityPathID(r.URL.Path) != "":
		value, err := s.service.TurnActivity(r.Context(), turnActivityPathID(r.URL.Path))
		writeRuntimeResult(w, value, err)
	case r.Method == http.MethodGet && turnCompactPathID(r.URL.Path) != "":
		value, err := s.service.TurnCompactBoundaries(r.Context(), turnCompactPathID(r.URL.Path))
		writeRuntimeResult(w, value, err)
	case r.Method == http.MethodGet && turnPromptAssembliesPathID(r.URL.Path) != "":
		value, err := s.service.PromptAssembliesByTurn(r.Context(), turnPromptAssembliesPathID(r.URL.Path))
		writeRuntimeResult(w, value, err)
	case r.Method == http.MethodGet && r.URL.Path == "/v1/hooks":
		value, err := s.service.Hooks(r.Context())
		writeRuntimeResult(w, value, err)
	case r.Method == http.MethodGet && r.URL.Path == "/v1/hook-executions":
		value, err := s.service.HookExecutions(r.Context(), RuntimeHookExecutionsRequest{
			SessionID:  r.URL.Query().Get("session_id"),
			TurnID:     r.URL.Query().Get("turn_id"),
			ToolCallID: r.URL.Query().Get("tool_call_id"),
			TaskID:     r.URL.Query().Get("task_id"),
			Event:      r.URL.Query().Get("event"),
			Status:     r.URL.Query().Get("status"),
		})
		writeRuntimeResult(w, value, err)
	case r.Method == http.MethodGet && hookExecutionPathID(r.URL.Path) != "":
		value, err := s.service.HookExecution(r.Context(), hookExecutionPathID(r.URL.Path))
		writeRuntimeResult(w, value, err)
	case r.Method == http.MethodGet && r.URL.Path == "/v1/worktrees":
		value, err := s.service.Worktrees(r.Context())
		writeRuntimeResult(w, value, err)
	case r.Method == http.MethodPost && r.URL.Path == "/v1/worktrees":
		var req RuntimeWorktreeCreateRequest
		if !decodeRuntimeJSON(w, r, &req) {
			return
		}
		value, err := s.service.CreateWorktree(r.Context(), req)
		writeRuntimeResult(w, value, err)
	case r.Method == http.MethodPost && worktreeEnterPathID(r.URL.Path) != "":
		var req RuntimeWorktreeActionRequest
		if r.Body != nil && r.ContentLength != 0 && !decodeRuntimeJSON(w, r, &req) {
			return
		}
		value, err := s.service.EnterWorktree(r.Context(), worktreeEnterPathID(r.URL.Path), req)
		writeRuntimeResult(w, value, err)
	case r.Method == http.MethodPost && worktreeExitPathID(r.URL.Path) != "":
		var req RuntimeWorktreeActionRequest
		if r.Body != nil && r.ContentLength != 0 && !decodeRuntimeJSON(w, r, &req) {
			return
		}
		value, err := s.service.ExitWorktree(r.Context(), worktreeExitPathID(r.URL.Path), req)
		writeRuntimeResult(w, value, err)
	case r.Method == http.MethodPost && worktreeCleanupPathID(r.URL.Path) != "":
		var req RuntimeWorktreeActionRequest
		if r.Body != nil && r.ContentLength != 0 && !decodeRuntimeJSON(w, r, &req) {
			return
		}
		value, err := s.service.CleanupWorktree(r.Context(), worktreeCleanupPathID(r.URL.Path), req)
		writeRuntimeResult(w, value, err)
	case r.Method == http.MethodGet && worktreePathID(r.URL.Path) != "":
		value, err := s.service.Worktree(r.Context(), worktreePathID(r.URL.Path))
		writeRuntimeResult(w, value, err)
	case r.Method == http.MethodGet && turnAgentTasksPathID(r.URL.Path) != "":
		value, err := s.service.TurnAgentTasks(r.Context(), turnAgentTasksPathID(r.URL.Path))
		writeRuntimeResult(w, value, err)
	case r.Method == http.MethodGet && turnTodosPathID(r.URL.Path) != "":
		value, err := s.service.TurnTodos(r.Context(), turnTodosPathID(r.URL.Path))
		writeRuntimeResult(w, value, err)
	case r.Method == http.MethodGet && turnPathID(r.URL.Path) != "":
		value, err := s.service.Turn(r.Context(), turnPathID(r.URL.Path))
		writeRuntimeResult(w, value, err)
	case r.Method == http.MethodPost && turnInterruptedDonePathID(r.URL.Path) != "":
		value, err := s.service.MarkInterruptedDone(r.Context(), turnInterruptedDonePathID(r.URL.Path))
		writeRuntimeResult(w, value, err)
	case r.Method == http.MethodGet && toolCallPathID(r.URL.Path) != "":
		value, err := s.service.ToolCall(r.Context(), toolCallPathID(r.URL.Path))
		writeRuntimeResult(w, value, err)
	case r.Method == http.MethodPost && agentTaskCancelPathID(r.URL.Path) != "":
		value, err := s.service.CancelAgentTask(r.Context(), agentTaskCancelPathID(r.URL.Path))
		writeRuntimeResult(w, value, err)
	case r.Method == http.MethodGet && agentTaskMessagesPathID(r.URL.Path) != "":
		value, err := s.service.AgentTaskMessages(r.Context(), agentTaskMessagesPathID(r.URL.Path))
		writeRuntimeResult(w, value, err)
	case r.Method == http.MethodPost && agentTaskMessagesPathID(r.URL.Path) != "":
		var req RuntimeAgentTaskMessageCreateRequest
		if !decodeRuntimeJSON(w, r, &req) {
			return
		}
		value, err := s.service.SendAgentTaskFollowUp(r.Context(), agentTaskMessagesPathID(r.URL.Path), req)
		writeRuntimeResult(w, value, err)
	case r.Method == http.MethodPost && agentTaskFollowUpPathID(r.URL.Path) != "":
		var req RuntimeAgentTaskMessageCreateRequest
		if !decodeRuntimeJSON(w, r, &req) {
			return
		}
		value, err := s.service.SendAgentTaskFollowUp(r.Context(), agentTaskFollowUpPathID(r.URL.Path), req)
		writeRuntimeResult(w, value, err)
	case r.Method == http.MethodGet && agentTaskResultPathID(r.URL.Path) != "":
		value, err := s.service.AgentTaskResult(r.Context(), agentTaskResultPathID(r.URL.Path))
		writeRuntimeResult(w, value, err)
	case r.Method == http.MethodGet && agentTaskOutputPathID(r.URL.Path) != "":
		value, err := s.service.AgentTaskOutput(r.Context(), agentTaskOutputPathID(r.URL.Path))
		writeRuntimeResult(w, value, err)
	case r.Method == http.MethodGet && agentTaskEffectiveScopePathID(r.URL.Path) != "":
		value, err := s.service.TaskEffectiveScope(r.Context(), agentTaskEffectiveScopePathID(r.URL.Path))
		writeRuntimeResult(w, value, err)
	case r.Method == http.MethodGet && agentTaskPathID(r.URL.Path) != "":
		value, err := s.service.AgentTask(r.Context(), agentTaskPathID(r.URL.Path))
		writeRuntimeResult(w, value, err)
	case r.Method == http.MethodGet && r.URL.Path == "/v1/agent-roles":
		value, err := s.service.AgentRoles(r.Context())
		writeRuntimeResult(w, value, err)
	case r.Method == http.MethodGet && agentRolePathID(r.URL.Path) != "":
		value, err := s.service.AgentRole(r.Context(), agentRolePathID(r.URL.Path))
		writeRuntimeResult(w, value, err)
	case r.Method == http.MethodPut && sessionPathID(r.URL.Path) != "":
		var req RuntimeSessionUpdateRequest
		if !decodeRuntimeJSON(w, r, &req) {
			return
		}
		req.SessionID = sessionPathID(r.URL.Path)
		value, err := s.service.RenameSession(r.Context(), req)
		writeRuntimeResult(w, value, err)
	case r.Method == http.MethodDelete && sessionPathID(r.URL.Path) != "":
		value, err := s.service.DeleteSession(r.Context(), sessionPathID(r.URL.Path))
		writeRuntimeResult(w, value, err)
	case r.Method == http.MethodPost && strings.HasPrefix(r.URL.Path, "/v1/sessions/") && strings.HasSuffix(r.URL.Path, "/select"):
		value, err := s.service.SelectSession(r.Context(), trimPathID(r.URL.Path, "/v1/sessions/", "/select"))
		writeRuntimeResult(w, value, err)
	case r.Method == http.MethodGet && sessionPathID(r.URL.Path) != "":
		value, err := s.service.Session(r.Context(), sessionPathID(r.URL.Path))
		writeRuntimeResult(w, value, err)
	case r.Method == http.MethodPost && turnCancelPathID(r.URL.Path) != "":
		value, err := s.service.CancelTurn(r.Context(), turnCancelPathID(r.URL.Path))
		writeRuntimeResult(w, value, err)
	case r.Method == http.MethodGet && r.URL.Path == "/v1/events" && strings.Contains(r.Header.Get("Accept"), "text/event-stream"):
		s.handleEvents(w, r)
	case r.Method == http.MethodGet && r.URL.Path == "/v1/events":
		after, err := parseRuntimeSequence(r.URL.Query().Get("after"))
		if err != nil {
			writeRuntimeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
			return
		}
		value, err := s.service.Events(r.Context(), after)
		writeRuntimeResult(w, value, err)
	case r.Method == http.MethodGet && r.URL.Path == "/v1/skills":
		value, err := s.service.Skills(r.Context())
		writeRuntimeResult(w, value, err)
	case r.Method == http.MethodGet && r.URL.Path == "/v1/plugins":
		value, err := s.service.Plugins(r.Context())
		writeRuntimeResult(w, value, err)
	case r.Method == http.MethodPost && r.URL.Path == "/v1/skills/refresh":
		value, err := s.service.RefreshSkills(r.Context())
		writeRuntimeResult(w, value, err)
	case r.Method == http.MethodPost && r.URL.Path == "/v1/skills":
		var req RuntimeSkillCreateRequest
		if !decodeRuntimeJSON(w, r, &req) {
			return
		}
		value, err := s.service.CreateSkill(r.Context(), req)
		writeRuntimeResult(w, value, err)
	case r.Method == http.MethodPost && r.URL.Path == "/v1/skills/paths":
		var req RuntimeSkillPathRequest
		if !decodeRuntimeJSON(w, r, &req) {
			return
		}
		value, err := s.service.AddSkillPath(r.Context(), req)
		writeRuntimeResult(w, value, err)
	case r.Method == http.MethodPost && strings.HasPrefix(r.URL.Path, "/v1/skills/") && strings.HasSuffix(r.URL.Path, "/enabled"):
		name := trimPathID(r.URL.Path, "/v1/skills/", "/enabled")
		var req RuntimeSkillToggleRequest
		if !decodeRuntimeJSON(w, r, &req) {
			return
		}
		req.Name = name
		value, err := s.service.SetSkillEnabled(r.Context(), req)
		writeRuntimeResult(w, value, err)
	case r.Method == http.MethodGet && r.URL.Path == "/v1/mcp/servers":
		value, err := s.service.MCPServers(r.Context())
		writeRuntimeResult(w, value, err)
	case r.Method == http.MethodPut && strings.HasPrefix(r.URL.Path, "/v1/mcp/servers/"):
		name := strings.TrimPrefix(r.URL.Path, "/v1/mcp/servers/")
		var req RuntimeMCPServerConfigRequest
		if !decodeRuntimeJSON(w, r, &req) {
			return
		}
		req.Name = name
		value, err := s.service.SaveMCPServer(r.Context(), req)
		writeRuntimeResult(w, value, err)
	case r.Method == http.MethodPost && strings.HasPrefix(r.URL.Path, "/v1/mcp/servers/") && strings.HasSuffix(r.URL.Path, "/refresh"):
		name := trimPathID(r.URL.Path, "/v1/mcp/servers/", "/refresh")
		value, err := s.service.RefreshMCPServer(r.Context(), name)
		writeRuntimeResult(w, value, err)
	case r.Method == http.MethodPost && strings.HasPrefix(r.URL.Path, "/v1/mcp/servers/") && strings.Contains(r.URL.Path, "/tools/") && strings.HasSuffix(r.URL.Path, "/enabled"):
		server, tool := mcpToolEnabledPathIDs(r.URL.Path)
		var req RuntimeMCPToolToggleRequest
		if !decodeRuntimeJSON(w, r, &req) {
			return
		}
		req.Server = server
		req.Tool = tool
		value, err := s.service.SetMCPToolEnabled(r.Context(), req)
		writeRuntimeResult(w, value, err)
	case r.Method == http.MethodPost && strings.HasPrefix(r.URL.Path, "/v1/mcp/servers/") && strings.HasSuffix(r.URL.Path, "/enabled"):
		name := trimPathID(r.URL.Path, "/v1/mcp/servers/", "/enabled")
		var req RuntimeMCPServerToggleRequest
		if !decodeRuntimeJSON(w, r, &req) {
			return
		}
		req.Name = name
		value, err := s.service.SetMCPServerEnabled(r.Context(), req)
		writeRuntimeResult(w, value, err)
	case r.Method == http.MethodGet && strings.HasPrefix(r.URL.Path, "/v1/mcp/servers/") && strings.HasSuffix(r.URL.Path, "/tools"):
		value, err := s.service.MCPTools(r.Context(), trimPathID(r.URL.Path, "/v1/mcp/servers/", "/tools"))
		writeRuntimeResult(w, value, err)
	case r.Method == http.MethodGet && strings.HasPrefix(r.URL.Path, "/v1/mcp/servers/") && strings.HasSuffix(r.URL.Path, "/resources"):
		value, err := s.service.MCPResources(r.Context(), trimPathID(r.URL.Path, "/v1/mcp/servers/", "/resources"))
		writeRuntimeResult(w, value, err)
	case r.Method == http.MethodGet && strings.HasPrefix(r.URL.Path, "/v1/mcp/servers/") && strings.HasSuffix(r.URL.Path, "/prompts"):
		value, err := s.service.MCPPrompts(r.Context(), trimPathID(r.URL.Path, "/v1/mcp/servers/", "/prompts"))
		writeRuntimeResult(w, value, err)
	case r.Method == http.MethodGet && r.URL.Path == "/v1/mcp/requests":
		value, err := s.service.MCPRequests(r.Context(), RuntimeMCPRequestListRequest{
			Kind:   r.URL.Query().Get("kind"),
			Status: r.URL.Query().Get("status"),
			Server: r.URL.Query().Get("server"),
		})
		writeRuntimeResult(w, value, err)
	case r.Method == http.MethodGet && mcpRequestPathID(r.URL.Path) != "":
		value, err := s.service.MCPRequest(r.Context(), mcpRequestPathID(r.URL.Path))
		writeRuntimeResult(w, value, err)
	case r.Method == http.MethodPost && mcpRequestDecisionPathID(r.URL.Path) != "":
		var req RuntimeMCPRequestDecision
		if !decodeRuntimeJSON(w, r, &req) {
			return
		}
		req.RequestID = mcpRequestDecisionPathID(r.URL.Path)
		value, err := s.service.DecideMCPRequest(r.Context(), req)
		writeRuntimeResult(w, value, err)
	case r.Method == http.MethodPost && strings.HasPrefix(r.URL.Path, "/v1/mcp/servers/") && strings.HasSuffix(r.URL.Path, "/retry"):
		value, err := s.service.RetryMCPServer(r.Context(), trimPathID(r.URL.Path, "/v1/mcp/servers/", "/retry"))
		writeRuntimeResult(w, value, err)
	case r.Method == http.MethodGet && r.URL.Path == "/v1/capabilities":
		value, err := s.service.Capabilities(r.Context())
		writeRuntimeResult(w, value, err)
	case r.Method == http.MethodPost && capabilityRefreshPathID(r.URL.Path) != "":
		value, err := s.service.RefreshCapability(r.Context(), capabilityRefreshPathID(r.URL.Path))
		writeRuntimeResult(w, value, err)
	case r.Method == http.MethodPost && r.URL.Path == "/v1/tools/search":
		var req RuntimeToolSearchRequest
		if !decodeRuntimeJSON(w, r, &req) {
			return
		}
		value, err := s.service.SearchTools(r.Context(), req)
		writeRuntimeResult(w, value, err)
	case r.Method == http.MethodGet && r.URL.Path == "/v1/context/sources":
		value, err := s.service.ContextSources(r.Context())
		writeRuntimeResult(w, value, err)
	case r.Method == http.MethodGet && r.URL.Path == "/v1/read-files":
		value, err := s.service.ReadFiles(r.Context(), r.URL.Query().Get("session_id"))
		writeRuntimeResult(w, value, err)
	case r.Method == http.MethodGet && strings.HasPrefix(r.URL.Path, "/v1/audit/turns/"):
		value, err := s.service.AuditTurn(r.Context(), strings.TrimPrefix(r.URL.Path, "/v1/audit/turns/"))
		writeRuntimeResult(w, value, err)
	case r.Method == http.MethodGet && strings.HasPrefix(r.URL.Path, "/v1/audit/sessions/"):
		value, err := s.service.AuditSession(r.Context(), strings.TrimPrefix(r.URL.Path, "/v1/audit/sessions/"))
		writeRuntimeResult(w, value, err)
	case r.Method == http.MethodGet && r.URL.Path == "/v1/replay/export":
		after, err := parseRuntimeSequence(r.URL.Query().Get("after"))
		if err != nil {
			writeRuntimeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
			return
		}
		value, err := s.service.ReplayExport(r.Context(), RuntimeReplayExportRequest{
			SessionID: r.URL.Query().Get("session_id"),
			TurnID:    r.URL.Query().Get("turn_id"),
			After:     after,
		})
		writeRuntimeResult(w, value, err)
	case r.Method == http.MethodGet && sessionCompactPathID(r.URL.Path) != "":
		value, err := s.service.SessionCompactBoundaries(r.Context(), sessionCompactPathID(r.URL.Path))
		writeRuntimeResult(w, value, err)
	default:
		http.NotFound(w, r)
	}
}

func (s *runtimeHTTPServer) handleClientAsset(w http.ResponseWriter, r *http.Request) bool {
	distDir, ok := findClientDistDir()
	if !ok {
		return false
	}
	requestPath := strings.TrimPrefix(r.URL.Path, "/")
	if requestPath == "" {
		requestPath = "index.html"
	}
	cleanPath := filepath.Clean(filepath.FromSlash(requestPath))
	if cleanPath == "." || strings.HasPrefix(cleanPath, ".."+string(filepath.Separator)) || cleanPath == ".." || filepath.IsAbs(cleanPath) {
		http.NotFound(w, r)
		return true
	}
	filePath := filepath.Join(distDir, cleanPath)
	if info, err := os.Stat(filePath); err != nil || info.IsDir() {
		filePath = filepath.Join(distDir, "index.html")
	}
	if contentType := mime.TypeByExtension(filepath.Ext(filePath)); contentType != "" {
		w.Header().Set("Content-Type", contentType)
	}
	http.ServeFile(w, r, filePath)
	return true
}

func findClientDistDir() (string, bool) {
	if value := strings.TrimSpace(os.Getenv("AGENT_BUILDER_CLIENT_DIST")); value != "" {
		if hasClientIndex(value) {
			return value, true
		}
		return "", false
	}
	wd, err := os.Getwd()
	if err != nil {
		return "", false
	}
	dir := wd
	for {
		candidate := filepath.Join(dir, "client", "dist")
		if hasClientIndex(candidate) {
			return candidate, true
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return "", false
		}
		dir = parent
	}
}

func hasClientIndex(dir string) bool {
	info, err := os.Stat(filepath.Join(dir, "index.html"))
	return err == nil && !info.IsDir()
}

func (s *runtimeHTTPServer) authorized(r *http.Request) bool {
	got := strings.TrimSpace(r.Header.Get("Authorization"))
	want := "Bearer " + s.Token()
	if subtle.ConstantTimeCompare([]byte(got), []byte(want)) == 1 {
		return true
	}
	queryToken := strings.TrimSpace(r.URL.Query().Get("token"))
	return queryToken != "" && subtle.ConstantTimeCompare([]byte(queryToken), []byte(s.Token())) == 1
}

func (s *runtimeHTTPServer) handleJSONP(w http.ResponseWriter, r *http.Request) {
	if !s.authorized(r) {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}
	callback := strings.TrimSpace(r.URL.Query().Get("callback"))
	if !validJSONPCallback(callback) {
		http.Error(w, "invalid callback", http.StatusBadRequest)
		return
	}

	var value any
	var err error
	switch strings.TrimSpace(r.URL.Query().Get("path")) {
	case "/v1/runtime/status":
		value, err = s.service.Status(r.Context())
	case "/v1/sessions":
		value, err = s.service.Sessions(r.Context())
	case "/v1/config/models":
		value, err = s.service.Models(r.Context())
	case "/v1/config/selected-model":
		value, err = s.service.SelectedModel(r.Context())
	case "/v1/config/providers":
		value, err = s.service.ProviderCatalog(r.Context())
	case "/v1/config/configured-providers":
		value, err = s.service.ConfiguredProviders(r.Context())
	default:
		http.NotFound(w, r)
		return
	}
	if err != nil {
		writeRuntimeJSONP(w, callback, map[string]string{"error": err.Error()})
		return
	}
	writeRuntimeJSONP(w, callback, value)
}

func (s *runtimeHTTPServer) handleDevModule(w http.ResponseWriter, r *http.Request) {
	if !s.authorized(r) {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}

	value, err, ok := s.readDevRuntimeValue(r)
	if !ok {
		http.NotFound(w, r)
		return
	}
	if err != nil {
		writeRuntimeModule(w, map[string]string{"error": err.Error()})
		return
	}
	writeRuntimeModule(w, value)
}

func (s *runtimeHTTPServer) readDevRuntimeValue(r *http.Request) (any, error, bool) {
	method := firstNonEmpty(strings.ToUpper(strings.TrimSpace(r.URL.Query().Get("method"))), http.MethodGet)
	path := strings.TrimSpace(r.URL.Query().Get("path"))
	pathQuery := url.Values{}
	if idx := strings.Index(path, "?"); idx >= 0 {
		if parsed, err := url.ParseQuery(path[idx+1:]); err == nil {
			pathQuery = parsed
		}
		path = path[:idx]
	}
	body := strings.TrimSpace(r.URL.Query().Get("body"))

	switch {
	case method == http.MethodGet && path == "/v1/runtime/status":
		value, err := s.service.Status(r.Context())
		return value, err, true
	case method == http.MethodPost && path == "/v1/projects/open":
		var req RuntimeOpenProjectRequest
		if strings.TrimSpace(body) != "" {
			if err := json.Unmarshal([]byte(body), &req); err != nil {
				return nil, err, true
			}
		}
		value, err := s.service.OpenProject(r.Context(), req)
		return value, err, true
	case method == http.MethodPost && path == "/v1/projects":
		var req RuntimeCreateProjectRequest
		if strings.TrimSpace(body) != "" {
			if err := json.Unmarshal([]byte(body), &req); err != nil {
				return nil, err, true
			}
		}
		value, err := s.service.CreateProject(r.Context(), req)
		return value, err, true
	case method == http.MethodPost && projectRenamePathID(path) != "":
		var req RuntimeRenameProjectRequest
		if strings.TrimSpace(body) != "" {
			if err := json.Unmarshal([]byte(body), &req); err != nil {
				return nil, err, true
			}
		}
		req.ProjectID = projectRenamePathID(path)
		value, err := s.service.RenameProject(r.Context(), req)
		return value, err, true
	case method == http.MethodPost && projectOpenExplorerPathID(path) != "":
		value, err := s.service.OpenProjectInExplorer(r.Context(), RuntimeProjectActionRequest{
			ProjectID: projectOpenExplorerPathID(path),
		})
		return value, err, true
	case method == http.MethodDelete && projectPathID(path) != "":
		value, err := s.service.RemoveProject(r.Context(), RuntimeProjectActionRequest{
			ProjectID: projectPathID(path),
		})
		return value, err, true
	case method == http.MethodGet && path == "/v1/sessions":
		value, err := s.service.Sessions(r.Context())
		return value, err, true
	case method == http.MethodPost && path == "/v1/sessions":
		var req RuntimeSessionCreateRequest
		if strings.TrimSpace(body) != "" {
			if err := json.Unmarshal([]byte(body), &req); err != nil {
				return nil, err, true
			}
		}
		value, err := s.service.CreateSession(r.Context(), req)
		return value, err, true
	case method == http.MethodPost && path == "/v1/runtime/new-chat":
		var req RuntimeSessionCreateRequest
		if strings.TrimSpace(body) != "" {
			if err := json.Unmarshal([]byte(body), &req); err != nil {
				return nil, err, true
			}
		}
		value, err := s.service.NewChat(r.Context(), req.Title)
		return value, err, true
	case method == http.MethodPost && strings.HasPrefix(path, "/v1/sessions/") && strings.HasSuffix(path, "/select"):
		value, err := s.service.SelectSession(r.Context(), trimPathID(path, "/v1/sessions/", "/select"))
		return value, err, true
	case method == http.MethodGet && sessionMessagesPathID(path) != "":
		value, err := s.service.SessionMessages(r.Context(), sessionMessagesPathID(path))
		return value, err, true
	case method == http.MethodGet && sessionContextUsagePathID(path) != "":
		value, err := s.service.SessionContextUsage(r.Context(), sessionContextUsagePathID(path))
		return value, err, true
	case method == http.MethodPost && sessionCompactPathID(path) != "":
		var req RuntimeContextActionRequest
		if err := json.Unmarshal([]byte(body), &req); err != nil {
			return nil, err, true
		}
		req.SessionID = sessionCompactPathID(path)
		value, err := s.service.ManualCompact(r.Context(), req)
		return value, err, true
	case method == http.MethodGet && sessionOutputEventsPathID(path) != "":
		value, err := s.service.SessionOutputEvents(r.Context(), sessionOutputEventsPathID(path), runtimeDevModuleCursor(r, pathQuery))
		return value, err, true
	case method == http.MethodGet && sessionOutputPathID(path) != "":
		value, err := s.service.SessionOutput(r.Context(), sessionOutputPathID(path), RuntimeOutputRequest{
			Snapshot: true,
			Cursor:   runtimeDevModuleCursor(r, pathQuery),
			Limit:    runtimeDevModuleLimit(r, pathQuery),
		})
		return value, err, true
	case method == http.MethodGet && sessionActivityPathID(path) != "":
		value, err := s.service.SessionActivity(r.Context(), sessionActivityPathID(path))
		return value, err, true
	case method == http.MethodGet && sessionActivityWindowPathID(path) != "":
		value, err := s.service.SessionActivityCursorWindow(r.Context(), sessionActivityWindowPathID(path), runtimeDevModuleCursor(r, pathQuery), runtimeDevModuleLimit(r, pathQuery))
		return value, err, true
	case method == http.MethodGet && sessionReactCallchainPathID(path) != "":
		value, err := s.service.SessionReactCallchain(r.Context(), sessionReactCallchainPathID(path), runtimeDevModuleLimit(r, pathQuery))
		return value, err, true
	case method == http.MethodGet && sessionPromptAssembliesPathID(path) != "":
		value, err := s.service.PromptAssembliesBySession(r.Context(), sessionPromptAssembliesPathID(path), runtimeDevModuleLimit(r, pathQuery))
		return value, err, true
	case method == http.MethodGet && sessionTerminalsPathID(path) != "":
		value, err := s.service.SessionTerminals(r.Context(), sessionTerminalsPathID(path))
		return value, err, true
	case method == http.MethodGet && sessionRunProjectionPathID(path) != "":
		value, err := s.service.RunProjection(r.Context(), RuntimeRunProjectionRequest{
			SessionID: sessionRunProjectionPathID(path),
			Cursor:    runtimeDevModuleCursor(r, pathQuery),
			Limit:     runtimeDevModuleLimit(r, pathQuery),
		})
		return value, err, true
	case method == http.MethodGet && path == "/v1/run-transitions":
		value, err := s.service.RunTransitionHistory(r.Context(), RuntimeRunTransitionHistoryRequest{
			RunID:     pathQuery.Get("run_id"),
			SessionID: pathQuery.Get("session_id"),
			TurnID:    pathQuery.Get("turn_id"),
			Cursor:    runtimeDevModuleCursor(r, pathQuery),
			Limit:     runtimeDevModuleLimit(r, pathQuery),
		})
		return value, err, true
	case method == http.MethodGet && path == "/v1/run-scheduler-plan":
		value, err := s.service.RunSchedulerPlan(r.Context(), RuntimeRunSchedulerPlanRequest{
			RunID:        pathQuery.Get("run_id"),
			SessionID:    pathQuery.Get("session_id"),
			Mode:         pathQuery.Get("mode"),
			TurnID:       pathQuery.Get("turn_id"),
			CheckpointID: pathQuery.Get("checkpoint_id"),
			TaskID:       pathQuery.Get("task_id"),
			Cursor:       runtimeDevModuleCursor(r, pathQuery),
			Limit:        runtimeDevModuleLimit(r, pathQuery),
		})
		return value, err, true
	case method == http.MethodPost && sessionTurnsPathID(path) != "":
		var req RuntimeChatRequest
		if err := json.Unmarshal([]byte(body), &req); err != nil {
			return nil, err, true
		}
		req.SessionID = sessionTurnsPathID(path)
		value, err := s.service.Chat(r.Context(), req)
		return value, err, true
	case method == http.MethodPost && path == "/v1/turns":
		var req RuntimeChatRequest
		if err := json.Unmarshal([]byte(body), &req); err != nil {
			return nil, err, true
		}
		value, err := s.service.Chat(r.Context(), req)
		return value, err, true
	case method == http.MethodPost && path == "/v1/user-inputs":
		var req RuntimeUserInputRequest
		if err := json.Unmarshal([]byte(body), &req); err != nil {
			return nil, err, true
		}
		value, err := s.service.SubmitUserInput(r.Context(), req)
		return value, err, true
	case method == http.MethodPost && path == "/v1/context/manual-compact":
		var req RuntimeContextActionRequest
		if err := json.Unmarshal([]byte(body), &req); err != nil {
			return nil, err, true
		}
		value, err := s.service.ManualCompact(r.Context(), req)
		return value, err, true
	case method == http.MethodPost && path == "/v1/context/manual-snip":
		var req RuntimeContextActionRequest
		if err := json.Unmarshal([]byte(body), &req); err != nil {
			return nil, err, true
		}
		value, err := s.service.ManualSnip(r.Context(), req)
		return value, err, true
	case method == http.MethodGet && userInputPathID(path) != "":
		value, err := s.service.UserInput(r.Context(), userInputPathID(path))
		return value, err, true
	case method == http.MethodPost && path == "/v1/terminals":
		var req RuntimeTerminalCreateRequest
		if strings.TrimSpace(body) != "" {
			if err := json.Unmarshal([]byte(body), &req); err != nil {
				return nil, err, true
			}
		}
		value, err := s.service.CreateTerminal(r.Context(), req)
		return value, err, true
	case method == http.MethodDelete && terminalPathID(path) != "":
		value, err := s.service.DeleteTerminal(r.Context(), terminalPathID(path))
		return value, err, true
	case method == http.MethodPost && turnCancelPathID(path) != "":
		value, err := s.service.CancelTurn(r.Context(), turnCancelPathID(path))
		return value, err, true
	case method == http.MethodPost && turnInterruptedDonePathID(path) != "":
		value, err := s.service.MarkInterruptedDone(r.Context(), turnInterruptedDonePathID(path))
		return value, err, true
	case method == http.MethodGet && path == "/v1/runs":
		value, err := s.service.Runs(r.Context())
		return value, err, true
	case method == http.MethodGet && path == "/v1/run-summaries":
		value, err := s.service.RunSummaries(r.Context())
		return value, err, true
	case method == http.MethodGet && runSummaryPathID(path) != "":
		value, err := s.service.RunSummary(r.Context(), runSummaryPathID(path))
		return value, err, true
	case method == http.MethodGet && runCheckpointMarkersPathID(path) != "":
		value, err := s.service.RunCheckpointMarkers(r.Context(), runCheckpointMarkersPathID(path))
		return value, err, true
	case method == http.MethodGet && runCheckpointMarkerPathIDs(path).runID != "":
		ids := runCheckpointMarkerPathIDs(path)
		value, err := s.service.RunCheckpointMarker(r.Context(), ids.runID, ids.checkpointID)
		return value, err, true
	case method == http.MethodGet && runPathID(path) != "":
		value, err := s.service.Run(r.Context(), runPathID(path))
		return value, err, true
	case method == http.MethodPost && runCheckpointAcknowledgePathIDs(path).runID != "":
		ids := runCheckpointAcknowledgePathIDs(path)
		value, err := s.service.AcknowledgeRunCheckpoint(r.Context(), ids.runID, ids.checkpointID)
		return value, err, true
	case method == http.MethodPost && runCheckpointDiscardPathIDs(path).runID != "":
		ids := runCheckpointDiscardPathIDs(path)
		value, err := s.service.DiscardRunCheckpoint(r.Context(), ids.runID, ids.checkpointID)
		return value, err, true
	case method == http.MethodPost && runCheckpointResumePathIDs(path).runID != "":
		ids := runCheckpointResumePathIDs(path)
		value, err := s.service.ResumeRunCheckpoint(r.Context(), ids.runID, ids.checkpointID)
		return value, err, true
	case method == http.MethodPost && runTaskExecutePathIDs(path).runID != "":
		ids := runTaskExecutePathIDs(path)
		value, err := s.service.ExecuteRunTask(r.Context(), ids.runID, ids.taskID)
		return value, err, true
	case method == http.MethodGet && turnActivityPathID(path) != "":
		value, err := s.service.TurnActivity(r.Context(), turnActivityPathID(path))
		return value, err, true
	case method == http.MethodGet && turnToolCallsPathID(path) != "":
		value, err := s.service.TurnToolCalls(r.Context(), turnToolCallsPathID(path))
		return value, err, true
	case method == http.MethodGet && turnReactCallchainPathID(path) != "":
		value, err := s.service.ReactCallchain(r.Context(), turnReactCallchainPathID(path))
		return value, err, true
	case method == http.MethodGet && turnCompactPathID(path) != "":
		value, err := s.service.TurnCompactBoundaries(r.Context(), turnCompactPathID(path))
		return value, err, true
	case method == http.MethodGet && turnPromptAssembliesPathID(path) != "":
		value, err := s.service.PromptAssembliesByTurn(r.Context(), turnPromptAssembliesPathID(path))
		return value, err, true
	case method == http.MethodGet && turnPathID(path) != "":
		value, err := s.service.Turn(r.Context(), turnPathID(path))
		return value, err, true
	case method == http.MethodGet && toolCallPathID(path) != "":
		value, err := s.service.ToolCall(r.Context(), toolCallPathID(path))
		return value, err, true
	case method == http.MethodGet && path == "/v1/permissions":
		value, err := s.service.Permissions(r.Context())
		return value, err, true
	case method == http.MethodPost && permissionDecisionPath(path) != "":
		var req RuntimePermissionDecision
		if err := json.Unmarshal([]byte(body), &req); err != nil {
			return nil, err, true
		}
		req.PermissionID = permissionDecisionPath(path)
		value, err := s.service.DecidePermission(r.Context(), req)
		return value, err, true
	case method == http.MethodGet && path == "/v1/policy":
		value, err := s.service.GetPolicy(r.Context())
		return value, err, true
	case method == http.MethodPut && path == "/v1/policy":
		var req RuntimePolicyUpdateRequest
		if err := json.Unmarshal([]byte(body), &req); err != nil {
			return nil, err, true
		}
		value, err := s.service.UpdatePolicy(r.Context(), req)
		return value, err, true
	case method == http.MethodGet && path == "/v1/config/models":
		value, err := s.service.Models(r.Context())
		return value, err, true
	case method == http.MethodGet && path == "/v1/config/selected-model":
		value, err := s.service.SelectedModel(r.Context())
		return value, err, true
	case method == http.MethodPut && path == "/v1/config/selected-model":
		var req RuntimeSelectedModelRequest
		if err := json.Unmarshal([]byte(body), &req); err != nil {
			return nil, err, true
		}
		value, err := s.service.SaveSelectedModel(r.Context(), req)
		return value, err, true
	case method == http.MethodGet && path == "/v1/config/providers":
		value, err := s.service.ProviderCatalog(r.Context())
		return value, err, true
	case method == http.MethodGet && path == "/v1/config/configured-providers":
		value, err := s.service.ConfiguredProviders(r.Context())
		return value, err, true
	case method == http.MethodPost && path == "/v1/config/configured-providers":
		var req RuntimeConfiguredProviderRequest
		if err := json.Unmarshal([]byte(body), &req); err != nil {
			return nil, err, true
		}
		value, err := s.service.SaveConfiguredProvider(r.Context(), req)
		return value, err, true
	case method == http.MethodPut && configuredProviderPathID(path) != "":
		var req RuntimeConfiguredProviderRequest
		if err := json.Unmarshal([]byte(body), &req); err != nil {
			return nil, err, true
		}
		req.ID = configuredProviderPathID(path)
		value, err := s.service.SaveConfiguredProvider(r.Context(), req)
		return value, err, true
	case method == http.MethodDelete && configuredProviderPathID(path) != "":
		value, err := s.service.DeleteConfiguredProvider(r.Context(), configuredProviderPathID(path))
		return value, err, true
	case method == http.MethodPost && configuredProviderModelsPathID(path) != "":
		value, err := s.service.DiscoverConfiguredProviderModels(r.Context(), configuredProviderModelsPathID(path))
		return value, err, true
	case method == http.MethodPost && configuredProviderTestPathID(path) != "":
		value, err := s.service.TestConfiguredProvider(r.Context(), configuredProviderTestPathID(path))
		return value, err, true
	case method == http.MethodPost && configuredProviderLatencyPathID(path) != "":
		value, err := s.service.MeasureConfiguredProviderLatency(r.Context(), configuredProviderLatencyPathID(path))
		return value, err, true
	default:
		return nil, nil, false
	}
}

func (s *runtimeHTTPServer) readDevRuntimeGetValue(r *http.Request) (any, error, bool) {
	switch strings.TrimSpace(r.URL.Query().Get("path")) {
	case "/v1/runtime/status":
		value, err := s.service.Status(r.Context())
		return value, err, true
	case "/v1/sessions":
		value, err := s.service.Sessions(r.Context())
		return value, err, true
	case "/v1/config/models":
		value, err := s.service.Models(r.Context())
		return value, err, true
	case "/v1/config/selected-model":
		value, err := s.service.SelectedModel(r.Context())
		return value, err, true
	case "/v1/config/providers":
		value, err := s.service.ProviderCatalog(r.Context())
		return value, err, true
	case "/v1/config/configured-providers":
		value, err := s.service.ConfiguredProviders(r.Context())
		return value, err, true
	default:
		return nil, nil, false
	}
}

func (s *runtimeHTTPServer) handleEvents(w http.ResponseWriter, r *http.Request) {
	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "streaming is not supported", http.StatusInternalServerError)
		return
	}

	after, err := parseRuntimeSequence(r.URL.Query().Get("after"))
	if err != nil {
		writeRuntimeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	events, unsubscribe := s.service.SubscribeEvents(r.Context(), after)
	defer unsubscribe()

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("Access-Control-Allow-Origin", "*")

	fmt.Fprint(w, ": connected\n\n")
	flusher.Flush()

	for {
		select {
		case event, ok := <-events:
			if !ok {
				return
			}
			data, err := json.Marshal(event)
			if err != nil {
				slog.Error("Failed to encode runtime HTTP SSE event", "error", err)
				continue
			}
			fmt.Fprintf(w, "event: runtime-event\ndata: %s\n\n", data)
			flusher.Flush()
		case <-r.Context().Done():
			return
		}
	}
}

func (s *runtimeHTTPServer) handleSessionOutputStream(w http.ResponseWriter, r *http.Request, sessionID string) {
	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "streaming is not supported", http.StatusInternalServerError)
		return
	}
	after := strings.TrimSpace(r.URL.Query().Get("after"))
	events, unsubscribe := s.service.SubscribeSessionOutputEvents(r.Context(), sessionID, after)
	defer unsubscribe()

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("Access-Control-Allow-Origin", "*")
	fmt.Fprint(w, ": connected\n\n")
	flusher.Flush()
	for {
		select {
		case event, ok := <-events:
			if !ok {
				return
			}
			data, err := json.Marshal(event)
			if err != nil {
				slog.Error("Failed to encode session output SSE event", "error", err)
				continue
			}
			fmt.Fprintf(w, "event: output-event\ndata: %s\n\n", data)
			flusher.Flush()
		case <-r.Context().Done():
			return
		}
	}
}

func (s *runtimeHTTPServer) handleTerminalStream(w http.ResponseWriter, r *http.Request, terminalID string) {
	after, err := parseRuntimeSequence(r.URL.Query().Get("after"))
	if err != nil {
		writeRuntimeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	conn, err := runtimeTerminalWebSocketUpgrader.Upgrade(w, r, nil)
	if err != nil {
		return
	}
	defer conn.Close()
	conn.SetReadLimit(runtimeTerminalStreamReadLimit)

	ctx, cancel := context.WithCancel(r.Context())
	defer cancel()

	events, unsubscribe := s.service.SubscribeTerminalEvents(ctx, terminalID, after)
	defer unsubscribe()

	ackCh := make(chan int64, 16)
	writeDone := make(chan struct{})
	go func() {
		defer close(writeDone)
		for {
			batch, ok := nextRuntimeTerminalStreamBatch(ctx, events)
			if !ok {
				return
			}
			messageType := "output"
			if batch[len(batch)-1].Final {
				messageType = "final"
			}
			if err := conn.WriteJSON(RuntimeTerminalStreamMessage{Type: messageType, Events: batch}); err != nil {
				cancel()
				return
			}
			if !waitRuntimeTerminalStreamAck(ctx, ackCh, batch[len(batch)-1].Sequence) {
				cancel()
				return
			}
		}
	}()

	for {
		var req RuntimeTerminalStreamRequest
		if err := conn.ReadJSON(&req); err != nil {
			cancel()
			<-writeDone
			return
		}
		switch strings.TrimSpace(req.Type) {
		case "input":
			_, _ = s.service.WriteTerminalInput(ctx, terminalID, RuntimeTerminalInputRequest{
				Data:      req.Data,
				BinaryB64: req.BinaryB64,
			})
		case "resize":
			_, _ = s.service.ResizeTerminal(ctx, terminalID, RuntimeTerminalResizeRequest{
				Columns: req.Columns,
				Rows:    req.Rows,
			})
		case "ack":
			select {
			case ackCh <- req.Sequence:
			default:
			}
		case "close":
			_, _ = s.service.DeleteTerminal(ctx, terminalID)
		case "ping", "":
		default:
		}
	}
}

func nextRuntimeTerminalStreamBatch(ctx context.Context, events <-chan RuntimeTerminalEvent) ([]RuntimeTerminalEvent, bool) {
	var batch []RuntimeTerminalEvent
	batchBytes := 0
	timer := time.NewTimer(runtimeTerminalStreamBatchWait)
	defer timer.Stop()

	for {
		select {
		case event, ok := <-events:
			if !ok {
				return batch, len(batch) > 0
			}
			batch = append(batch, event)
			batchBytes += runtimeTerminalEventSize(event)
			if event.Final || batchBytes >= runtimeTerminalStreamBatchBytes {
				return batch, true
			}
		case <-timer.C:
			if len(batch) > 0 {
				return batch, true
			}
			timer.Reset(runtimeTerminalStreamBatchWait)
		case <-ctx.Done():
			return nil, false
		}
	}
}

func waitRuntimeTerminalStreamAck(ctx context.Context, ackCh <-chan int64, sequence int64) bool {
	if sequence <= 0 {
		return true
	}
	timer := time.NewTimer(runtimeTerminalStreamAckTimeout)
	defer timer.Stop()
	for {
		select {
		case ack := <-ackCh:
			if ack >= sequence {
				return true
			}
		case <-timer.C:
			return false
		case <-ctx.Done():
			return false
		}
	}
}

func parseRuntimeSequence(value string) (int64, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return 0, nil
	}
	sequence, err := strconv.ParseInt(value, 10, 64)
	if err != nil || sequence < 0 {
		return 0, fmt.Errorf("after must be a non-negative event sequence")
	}
	return sequence, nil
}

func writeRuntimeResult[T any](w http.ResponseWriter, value T, err error) {
	if err != nil {
		writeRuntimeError(w, err)
		return
	}
	writeRuntimeJSON(w, http.StatusOK, value)
}

func writeRuntimeError(w http.ResponseWriter, err error) {
	status := http.StatusInternalServerError
	if errors.Is(err, errModelConfigMissing) {
		status = http.StatusPreconditionRequired
	}
	if errors.Is(err, errSelectedModelMissing) {
		status = http.StatusPreconditionRequired
	}
	if errors.Is(err, errRuntimeTerminalMissing) {
		status = http.StatusNotFound
	}
	if errors.Is(err, config.ErrContextGovernanceInvalid) {
		status = http.StatusBadRequest
	}
	writeRuntimeJSON(w, status, map[string]string{"error": err.Error()})
}

func writeRuntimeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.Header().Set("Access-Control-Allow-Headers", "Authorization, Content-Type")
	w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, PATCH, DELETE, OPTIONS")
	w.WriteHeader(status)
	if value == nil || status == http.StatusNoContent {
		return
	}
	if err := json.NewEncoder(w).Encode(value); err != nil {
		slog.Error("Failed to encode runtime HTTP response", "error", err)
	}
}

func validJSONPCallback(callback string) bool {
	if callback == "" {
		return false
	}
	for index, char := range callback {
		valid := char == '_' || char == '$' || char == '.' ||
			char >= '0' && char <= '9' ||
			char >= 'A' && char <= 'Z' ||
			char >= 'a' && char <= 'z'
		if !valid || index == 0 && char >= '0' && char <= '9' {
			return false
		}
	}
	return true
}

func writeRuntimeJSONP(w http.ResponseWriter, callback string, value any) {
	data, err := json.Marshal(value)
	if err != nil {
		writeRuntimeError(w, fmt.Errorf("failed to encode runtime JSONP response: %w", err))
		return
	}
	w.Header().Set("Content-Type", "application/javascript")
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte(callback))
	_, _ = w.Write([]byte("("))
	_, _ = w.Write(data)
	_, _ = w.Write([]byte(");"))
}

func writeRuntimeModule(w http.ResponseWriter, value any) {
	data, err := json.Marshal(value)
	if err != nil {
		writeRuntimeError(w, fmt.Errorf("failed to encode runtime module response: %w", err))
		return
	}
	w.Header().Set("Content-Type", "application/javascript")
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte("export default "))
	_, _ = w.Write(data)
	_, _ = w.Write([]byte(";"))
}

func decodeRuntimeJSON(w http.ResponseWriter, r *http.Request, target any) bool {
	defer r.Body.Close() //nolint:errcheck
	if err := json.NewDecoder(r.Body).Decode(target); err != nil {
		writeRuntimeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid JSON request body"})
		return false
	}
	return true
}

func permissionDecisionPath(path string) string {
	return trimPathID(path, "/v1/permissions/", "/decision")
}

func recoveryTurnResumePathID(path string) string {
	return trimPathID(path, "/v1/recovery/turns/", "/resume")
}

func recoveryTurnDiscardPathID(path string) string {
	return trimPathID(path, "/v1/recovery/turns/", "/discard")
}

func recoveryErrorRetryPathID(path string) string {
	return trimPathID(path, "/v1/recovery/errors/", "/retry")
}

func mcpRequestDecisionPathID(path string) string {
	return trimPathID(path, "/v1/mcp/requests/", "/decision")
}

func configuredProviderPathID(path string) string {
	id := strings.TrimPrefix(path, "/v1/config/configured-providers/")
	if id == path || id == "" || strings.Contains(id, "/") {
		return ""
	}
	return id
}

func configuredProviderModelsPathID(path string) string {
	return trimPathID(path, "/v1/config/configured-providers/", "/models")
}

func configuredProviderTestPathID(path string) string {
	return trimPathID(path, "/v1/config/configured-providers/", "/test")
}

func configuredProviderLatencyPathID(path string) string {
	return trimPathID(path, "/v1/config/configured-providers/", "/latency")
}

func terminalPathID(path string) string {
	if strings.HasSuffix(path, "/stream") {
		return ""
	}
	id := strings.TrimPrefix(path, "/v1/terminals/")
	if id == path || id == "" || strings.Contains(id, "/") {
		return ""
	}
	return id
}

func projectRenamePathID(path string) string {
	return trimPathID(path, "/v1/projects/", "/rename")
}

func projectOpenExplorerPathID(path string) string {
	return trimPathID(path, "/v1/projects/", "/open-explorer")
}

func projectPathID(path string) string {
	if strings.HasSuffix(path, "/rename") || strings.HasSuffix(path, "/open-explorer") || strings.Contains(path, "/memory") {
		return ""
	}
	id := strings.TrimPrefix(path, "/v1/projects/")
	if id == path || id == "" || strings.Contains(id, "/") {
		return ""
	}
	return id
}

func projectMemoryPathID(path string) string {
	return trimPathID(path, "/v1/projects/", "/memory")
}

func projectMemoryRefreshPathID(path string) string {
	return trimPathID(path, "/v1/projects/", "/memory/refresh")
}

func projectMemoryDiagnosticsPathID(path string) string {
	return trimPathID(path, "/v1/projects/", "/memory/diagnostics")
}

func memoryDisablePathID(path string) string {
	return trimPathID(path, "/v1/memory/", "/disable")
}

func memoryDeletePathID(path string) string {
	return trimPathID(path, "/v1/memory/", "/delete")
}

func memoryPathID(path string) string {
	if strings.HasSuffix(path, "/disable") || strings.HasSuffix(path, "/delete") {
		return ""
	}
	id := strings.TrimPrefix(path, "/v1/memory/")
	if id == path || id == "" || strings.Contains(id, "/") {
		return ""
	}
	return id
}

func terminalStreamPathID(path string) string {
	return trimPathID(path, "/v1/terminals/", "/stream")
}

func mcpRequestPathID(path string) string {
	if strings.HasSuffix(path, "/decision") {
		return ""
	}
	id := strings.TrimPrefix(path, "/v1/mcp/requests/")
	if id == path || id == "" || strings.Contains(id, "/") {
		return ""
	}
	return id
}

func sessionMessagesPathID(path string) string {
	return trimPathID(path, "/v1/sessions/", "/messages")
}

func sessionContextUsagePathID(path string) string {
	return trimPathID(path, "/v1/sessions/", "/context-usage")
}

func sessionCompactPathID(path string) string {
	return trimPathID(path, "/v1/sessions/", "/compact")
}

func sessionOutputPathID(path string) string {
	return trimPathID(path, "/v1/sessions/", "/output")
}

func sessionOutputEventsPathID(path string) string {
	return trimPathID(path, "/v1/sessions/", "/output/events")
}

func sessionOutputStreamPathID(path string) string {
	return trimPathID(path, "/v1/sessions/", "/output/stream")
}

func sessionActivityPathID(path string) string {
	return trimPathID(path, "/v1/sessions/", "/activity")
}

func sessionActivityWindowPathID(path string) string {
	return trimPathID(path, "/v1/sessions/", "/activity-window")
}

func sessionReactCallchainPathID(path string) string {
	return trimPathID(path, "/v1/sessions/", "/react-callchain")
}

func sessionPromptAssembliesPathID(path string) string {
	return trimPathID(path, "/v1/sessions/", "/prompt-assemblies")
}

func sessionTerminalsPathID(path string) string {
	return trimPathID(path, "/v1/sessions/", "/terminals")
}

func sessionAgentTasksPathID(path string) string {
	return trimPathID(path, "/v1/sessions/", "/agent-tasks")
}

func sessionRunProjectionPathID(path string) string {
	return trimPathID(path, "/v1/sessions/", "/run-projection")
}

func sessionTodosPathID(path string) string {
	return trimPathID(path, "/v1/sessions/", "/todos")
}

func sessionTurnsPathID(path string) string {
	return trimPathID(path, "/v1/sessions/", "/turns")
}

func sessionPathID(path string) string {
	id := strings.TrimPrefix(path, "/v1/sessions/")
	if id == path || id == "" || strings.Contains(id, "/") {
		return ""
	}
	return id
}

func runPathID(path string) string {
	id := strings.TrimPrefix(path, "/v1/runs/")
	if id == path || id == "" || strings.Contains(id, "/") {
		return ""
	}
	return id
}

func runSummaryPathID(path string) string {
	id := strings.TrimPrefix(path, "/v1/run-summaries/")
	if id == path || id == "" || strings.Contains(id, "/") {
		return ""
	}
	return id
}

func runCheckpointMarkersPathID(path string) string {
	prefix := "/v1/runs/"
	if !strings.HasPrefix(path, prefix) {
		return ""
	}
	parts := strings.Split(strings.TrimPrefix(path, prefix), "/")
	if len(parts) != 2 || parts[0] == "" || parts[1] != "checkpoint-markers" {
		return ""
	}
	return parts[0]
}

func runCheckpointMarkerPathIDs(path string) runCheckpointPathIDs {
	prefix := "/v1/runs/"
	if !strings.HasPrefix(path, prefix) {
		return runCheckpointPathIDs{}
	}
	parts := strings.Split(strings.TrimPrefix(path, prefix), "/")
	if len(parts) != 3 || parts[0] == "" || parts[1] != "checkpoint-markers" || parts[2] == "" {
		return runCheckpointPathIDs{}
	}
	return runCheckpointPathIDs{runID: parts[0], checkpointID: parts[2]}
}

type runCheckpointPathIDs struct {
	runID        string
	checkpointID string
}

type runTaskPathIDs struct {
	runID  string
	taskID string
}

func runCheckpointAcknowledgePathIDs(path string) runCheckpointPathIDs {
	return runCheckpointActionPathIDs(path, "acknowledge")
}

func runCheckpointDiscardPathIDs(path string) runCheckpointPathIDs {
	return runCheckpointActionPathIDs(path, "discard")
}

func runCheckpointResumePathIDs(path string) runCheckpointPathIDs {
	return runCheckpointActionPathIDs(path, "resume")
}

func runCheckpointActionPathIDs(path, action string) runCheckpointPathIDs {
	prefix := "/v1/runs/"
	if !strings.HasPrefix(path, prefix) {
		return runCheckpointPathIDs{}
	}
	parts := strings.Split(strings.TrimPrefix(path, prefix), "/")
	if len(parts) != 4 || parts[1] != "checkpoints" || parts[3] != action || parts[0] == "" || parts[2] == "" {
		return runCheckpointPathIDs{}
	}
	return runCheckpointPathIDs{runID: parts[0], checkpointID: parts[2]}
}

func runTaskExecutePathIDs(path string) runTaskPathIDs {
	prefix := "/v1/runs/"
	if !strings.HasPrefix(path, prefix) {
		return runTaskPathIDs{}
	}
	parts := strings.Split(strings.TrimPrefix(path, prefix), "/")
	if len(parts) != 4 || parts[1] != "tasks" || parts[3] != "execute" || parts[0] == "" || parts[2] == "" {
		return runTaskPathIDs{}
	}
	return runTaskPathIDs{runID: parts[0], taskID: parts[2]}
}

func turnCancelPathID(path string) string {
	return trimPathID(path, "/v1/turns/", "/cancel")
}

func turnInterruptedDonePathID(path string) string {
	return trimPathID(path, "/v1/turns/", "/interrupted/done")
}

func turnPathID(path string) string {
	if strings.HasSuffix(path, "/activity") || strings.HasSuffix(path, "/tool-calls") || strings.HasSuffix(path, "/react-callchain") || strings.HasSuffix(path, "/todos") || strings.HasSuffix(path, "/compact") || strings.HasSuffix(path, "/prompt-assemblies") || strings.HasSuffix(path, "/interrupted/done") {
		return ""
	}
	id := strings.TrimPrefix(path, "/v1/turns/")
	if id == path || id == "" || strings.Contains(id, "/") {
		return ""
	}
	return id
}

func userInputPathID(path string) string {
	id := strings.TrimPrefix(path, "/v1/user-inputs/")
	if id == path || id == "" || strings.Contains(id, "/") {
		return ""
	}
	return id
}

func turnToolCallsPathID(path string) string {
	return trimPathID(path, "/v1/turns/", "/tool-calls")
}

func turnActivityPathID(path string) string {
	return trimPathID(path, "/v1/turns/", "/activity")
}

func turnReactCallchainPathID(path string) string {
	return trimPathID(path, "/v1/turns/", "/react-callchain")
}

func turnCompactPathID(path string) string {
	return trimPathID(path, "/v1/turns/", "/compact")
}

func turnPromptAssembliesPathID(path string) string {
	return trimPathID(path, "/v1/turns/", "/prompt-assemblies")
}

func runtimeQueryLimit(r *http.Request) int {
	limit, _ := strconv.Atoi(strings.TrimSpace(r.URL.Query().Get("limit")))
	if limit < 0 {
		return 0
	}
	return limit
}

func runtimeQueryCursor(r *http.Request) string {
	return strings.TrimSpace(r.URL.Query().Get("cursor"))
}

func runtimeDevModuleLimit(r *http.Request, pathQuery url.Values) int {
	limitText := firstNonEmpty(pathQuery.Get("limit"), r.URL.Query().Get("limit"))
	limit, _ := strconv.Atoi(strings.TrimSpace(limitText))
	if limit < 0 {
		return 0
	}
	return limit
}

func runtimeDevModuleCursor(r *http.Request, pathQuery url.Values) string {
	return strings.TrimSpace(firstNonEmpty(pathQuery.Get("cursor"), r.URL.Query().Get("cursor")))
}

func runtimeDevModuleAfter(r *http.Request, pathQuery url.Values) int64 {
	value := strings.TrimSpace(firstNonEmpty(pathQuery.Get("after"), r.URL.Query().Get("after")))
	after, _ := strconv.ParseInt(value, 10, 64)
	if after < 0 {
		return 0
	}
	return after
}

func turnAgentTasksPathID(path string) string {
	return trimPathID(path, "/v1/turns/", "/agent-tasks")
}

func turnTodosPathID(path string) string {
	return trimPathID(path, "/v1/turns/", "/todos")
}

func toolCallPathID(path string) string {
	id := strings.TrimPrefix(path, "/v1/tool-calls/")
	if id == path || id == "" || strings.Contains(id, "/") {
		return ""
	}
	return id
}

func hookExecutionPathID(path string) string {
	id := strings.TrimPrefix(path, "/v1/hook-executions/")
	if id == path || id == "" || strings.Contains(id, "/") {
		return ""
	}
	return id
}

func refContentPathID(path string) string {
	return trimPathID(path, "/v1/refs/", "/content")
}

func sandboxDecisionPathID(path string) string {
	id := strings.TrimPrefix(path, "/v1/sandbox/decisions/")
	if id == path || id == "" || strings.Contains(id, "/") {
		return ""
	}
	return id
}

func refPathID(path string) string {
	if strings.HasSuffix(path, "/content") {
		return ""
	}
	id := strings.TrimPrefix(path, "/v1/refs/")
	if id == path || id == "" || strings.Contains(id, "/") {
		return ""
	}
	return id
}

func agentTaskCancelPathID(path string) string {
	return trimPathID(path, "/v1/agent-tasks/", "/cancel")
}

func agentTaskMessagesPathID(path string) string {
	return trimPathID(path, "/v1/agent-tasks/", "/messages")
}

func agentTaskResultPathID(path string) string {
	return trimPathID(path, "/v1/agent-tasks/", "/result")
}

func agentTaskOutputPathID(path string) string {
	return trimPathID(path, "/v1/agent-tasks/", "/output")
}

func agentTaskFollowUpPathID(path string) string {
	return trimPathID(path, "/v1/agent-tasks/", "/follow-up")
}

func agentTaskEffectiveScopePathID(path string) string {
	return trimPathID(path, "/v1/agent-tasks/", "/effective-scope")
}

func agentTaskPathID(path string) string {
	if strings.HasSuffix(path, "/messages") || strings.HasSuffix(path, "/result") || strings.HasSuffix(path, "/output") || strings.HasSuffix(path, "/cancel") || strings.HasSuffix(path, "/follow-up") || strings.HasSuffix(path, "/effective-scope") {
		return ""
	}
	id := strings.TrimPrefix(path, "/v1/agent-tasks/")
	if id != path && id != "" && !strings.Contains(id, "/") {
		return id
	}
	return ""
}

func worktreeEnterPathID(path string) string {
	return trimPathID(path, "/v1/worktrees/", "/enter")
}

func worktreeExitPathID(path string) string {
	return trimPathID(path, "/v1/worktrees/", "/exit")
}

func worktreeCleanupPathID(path string) string {
	return trimPathID(path, "/v1/worktrees/", "/cleanup")
}

func worktreePathID(path string) string {
	if strings.HasSuffix(path, "/enter") || strings.HasSuffix(path, "/exit") || strings.HasSuffix(path, "/cleanup") {
		return ""
	}
	id := strings.TrimPrefix(path, "/v1/worktrees/")
	if id == path || id == "" || strings.Contains(id, "/") {
		return ""
	}
	return id
}

func agentRolePathID(path string) string {
	id := strings.TrimPrefix(path, "/v1/agent-roles/")
	if id == path || id == "" || strings.Contains(id, "/") {
		return ""
	}
	return id
}

func trimPathID(path, prefix, suffix string) string {
	if !strings.HasPrefix(path, prefix) || !strings.HasSuffix(path, suffix) {
		return ""
	}
	id := strings.TrimSuffix(strings.TrimPrefix(path, prefix), suffix)
	if id == "" || strings.Contains(id, "/") {
		return ""
	}
	return id
}

func mcpToolEnabledPathIDs(path string) (string, string) {
	const prefix = "/v1/mcp/servers/"
	const suffix = "/enabled"
	if !strings.HasPrefix(path, prefix) || !strings.HasSuffix(path, suffix) {
		return "", ""
	}
	rest := strings.TrimSuffix(strings.TrimPrefix(path, prefix), suffix)
	parts := strings.Split(rest, "/tools/")
	if len(parts) != 2 || parts[0] == "" || parts[1] == "" || strings.Contains(parts[0], "/") || strings.Contains(parts[1], "/") {
		return "", ""
	}
	return parts[0], parts[1]
}
