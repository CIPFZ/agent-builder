package main

import (
	"context"
	"crypto/subtle"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/http"
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
	case r.Method == http.MethodGet && r.URL.Path == "/v1/config/model":
		value, err := s.service.GetModelConfig(r.Context())
		writeRuntimeResult(w, value, err)
	case r.Method == http.MethodPut && r.URL.Path == "/v1/config/model":
		var req RuntimeModelConfig
		if !decodeRuntimeJSON(w, r, &req) {
			return
		}
		value, err := s.service.SaveModelConfig(r.Context(), req)
		writeRuntimeResult(w, value, err)
	case r.Method == http.MethodGet && r.URL.Path == "/v1/permissions":
		value, err := s.service.Permissions(r.Context())
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
		status, err := s.service.Status(r.Context())
		if err != nil {
			writeRuntimeError(w, err)
			return
		}
		writeRuntimeJSON(w, http.StatusOK, map[string]any{
			"sessions": []map[string]any{{
				"id":          status.SessionID,
				"working_dir": status.WorkingDir,
				"model":       status.Model,
				"provider":    status.Provider,
			}},
		})
	case r.Method == http.MethodGet && sessionPathID(r.URL.Path) != "":
		status, err := s.service.Status(r.Context())
		if err != nil {
			writeRuntimeError(w, err)
			return
		}
		if status.SessionID != sessionPathID(r.URL.Path) {
			http.NotFound(w, r)
			return
		}
		writeRuntimeJSON(w, http.StatusOK, map[string]any{"session": status})
	case r.Method == http.MethodGet && sessionMessagesPathID(r.URL.Path) != "":
		value, err := s.service.Messages(r.Context())
		writeRuntimeResult(w, value, err)
	case r.Method == http.MethodPost && sessionTurnsPathID(r.URL.Path) != "":
		var req RuntimeChatRequest
		if !decodeRuntimeJSON(w, r, &req) {
			return
		}
		value, err := s.service.Chat(r.Context(), req)
		writeRuntimeResult(w, value, err)
	case r.Method == http.MethodPost && turnCancelPathID(r.URL.Path) != "":
		value, err := s.service.Cancel(r.Context())
		writeRuntimeResult(w, value, err)
	case r.Method == http.MethodGet && r.URL.Path == "/v1/events":
		s.handleEvents(w, r)
	default:
		http.NotFound(w, r)
	}
}

func (s *runtimeHTTPServer) authorized(r *http.Request) bool {
	got := strings.TrimSpace(r.Header.Get("Authorization"))
	want := "Bearer " + s.Token()
	return subtle.ConstantTimeCompare([]byte(got), []byte(want)) == 1
}

func (s *runtimeHTTPServer) handleEvents(w http.ResponseWriter, r *http.Request) {
	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "streaming is not supported", http.StatusInternalServerError)
		return
	}

	events, unsubscribe := s.service.SubscribeEvents(r.Context())
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
	w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, OPTIONS")
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
