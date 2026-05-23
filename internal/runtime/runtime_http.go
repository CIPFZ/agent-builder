package runtime

import (
	"context"
	"crypto/subtle"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"
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

func newRuntimeHTTPServer(service RuntimeService) *runtimeHTTPServer {
	return &runtimeHTTPServer{
		service: service,
		token:   newStreamToken(),
	}
}

func (s *runtimeHTTPServer) Start() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.url != "" {
		return nil
	}

	listener, err := net.Listen("tcp", "127.0.0.1:0")
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
	if !s.authorized(r) {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}

	switch {
	case r.Method == http.MethodGet && r.URL.Path == "/v1/runtime/status":
		value, err := s.service.Status(r.Context())
		writeRuntimeResult(w, value, err)
	case r.Method == http.MethodGet && r.URL.Path == "/v1/recovery/status":
		value, err := s.service.RecoveryStatus(r.Context())
		writeRuntimeResult(w, value, err)
	case r.Method == http.MethodGet && r.URL.Path == "/v1/config/model":
		value, err := s.service.GetModelConfig(r.Context())
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
		var req struct {
			Title string `json:"title"`
		}
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
	case r.Method == http.MethodGet && r.URL.Path == "/v1/turns":
		value, err := s.service.Turns(r.Context(), r.URL.Query().Get("status"))
		writeRuntimeResult(w, value, err)
	case r.Method == http.MethodGet && turnToolCallsPathID(r.URL.Path) != "":
		value, err := s.service.TurnToolCalls(r.Context(), turnToolCallsPathID(r.URL.Path))
		writeRuntimeResult(w, value, err)
	case r.Method == http.MethodGet && turnTasksPathID(r.URL.Path) != "":
		value, err := s.service.TurnAgentTasks(r.Context(), turnTasksPathID(r.URL.Path))
		writeRuntimeResult(w, value, err)
	case r.Method == http.MethodGet && turnTodosPathID(r.URL.Path) != "":
		value, err := s.service.TurnTodos(r.Context(), turnTodosPathID(r.URL.Path))
		writeRuntimeResult(w, value, err)
	case r.Method == http.MethodGet && turnPathID(r.URL.Path) != "":
		value, err := s.service.Turn(r.Context(), turnPathID(r.URL.Path))
		writeRuntimeResult(w, value, err)
	case r.Method == http.MethodGet && toolCallPathID(r.URL.Path) != "":
		value, err := s.service.ToolCall(r.Context(), toolCallPathID(r.URL.Path))
		writeRuntimeResult(w, value, err)
	case r.Method == http.MethodPost && taskCancelPathID(r.URL.Path) != "":
		value, err := s.service.CancelAgentTask(r.Context(), taskCancelPathID(r.URL.Path))
		writeRuntimeResult(w, value, err)
	case r.Method == http.MethodGet && taskPathID(r.URL.Path) != "":
		value, err := s.service.AgentTask(r.Context(), taskPathID(r.URL.Path))
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
	case r.Method == http.MethodGet && r.URL.Path == "/v1/capabilities":
		value, err := s.service.Capabilities(r.Context())
		writeRuntimeResult(w, value, err)
	case r.Method == http.MethodPost && capabilityRefreshPathID(r.URL.Path) != "":
		value, err := s.service.RefreshCapability(r.Context(), capabilityRefreshPathID(r.URL.Path))
		writeRuntimeResult(w, value, err)
	case r.Method == http.MethodGet && strings.HasPrefix(r.URL.Path, "/v1/audit/turns/"):
		value, err := s.service.AuditTurn(r.Context(), strings.TrimPrefix(r.URL.Path, "/v1/audit/turns/"))
		writeRuntimeResult(w, value, err)
	case r.Method == http.MethodGet && strings.HasPrefix(r.URL.Path, "/v1/audit/sessions/"):
		value, err := s.service.AuditSession(r.Context(), strings.TrimPrefix(r.URL.Path, "/v1/audit/sessions/"))
		writeRuntimeResult(w, value, err)
	default:
		http.NotFound(w, r)
	}
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
	w.Header().Set("Access-Control-Allow-Origin", "http://127.0.0.1")

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
	writeRuntimeJSON(w, status, map[string]string{"error": err.Error()})
}

func writeRuntimeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("Access-Control-Allow-Origin", "http://127.0.0.1")
	w.Header().Set("Access-Control-Allow-Headers", "Authorization, Content-Type")
	w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
	w.WriteHeader(status)
	if value == nil || status == http.StatusNoContent {
		return
	}
	if err := json.NewEncoder(w).Encode(value); err != nil {
		slog.Error("Failed to encode runtime HTTP response", "error", err)
	}
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

func sessionMessagesPathID(path string) string {
	return trimPathID(path, "/v1/sessions/", "/messages")
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

func turnCancelPathID(path string) string {
	return trimPathID(path, "/v1/turns/", "/cancel")
}

func turnPathID(path string) string {
	if strings.HasSuffix(path, "/tool-calls") || strings.HasSuffix(path, "/todos") {
		return ""
	}
	id := strings.TrimPrefix(path, "/v1/turns/")
	if id == path || id == "" || strings.Contains(id, "/") {
		return ""
	}
	return id
}

func turnToolCallsPathID(path string) string {
	return trimPathID(path, "/v1/turns/", "/tool-calls")
}

func turnTasksPathID(path string) string {
	return trimPathID(path, "/v1/turns/", "/tasks")
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

func taskCancelPathID(path string) string {
	return trimPathID(path, "/v1/tasks/", "/cancel")
}

func taskPathID(path string) string {
	id := strings.TrimPrefix(path, "/v1/tasks/")
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
