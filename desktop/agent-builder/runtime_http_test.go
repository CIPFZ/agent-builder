package main

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"testing"
)

func TestRuntimeHTTPServerRequiresBearerToken(t *testing.T) {
	t.Parallel()

	server := newRuntimeHTTPServer(&recordingRuntimeService{})
	req, err := http.NewRequest(http.MethodGet, "/v1/runtime/status", nil)
	if err != nil {
		t.Fatal(err)
	}
	resp := httptestResponse(server, req)
	if resp.status != http.StatusUnauthorized {
		t.Fatalf("status = %d, want %d", resp.status, http.StatusUnauthorized)
	}
}

func TestRuntimeHTTPServerRoutesStatusToRuntimeService(t *testing.T) {
	t.Parallel()

	service := &recordingRuntimeService{
		status: RuntimeStatus{
			Ready:     true,
			SessionID: "session-1",
		},
	}
	server := newRuntimeHTTPServer(service)
	req, err := http.NewRequest(http.MethodGet, "/v1/runtime/status", nil)
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Authorization", "Bearer "+server.Token())

	resp := httptestResponse(server, req)
	if resp.status != http.StatusOK {
		t.Fatalf("status = %d body = %s", resp.status, resp.body.String())
	}
	if service.statusCalls != 1 {
		t.Fatalf("statusCalls = %d, want 1", service.statusCalls)
	}
	var status RuntimeStatus
	if err := json.Unmarshal(resp.body.Bytes(), &status); err != nil {
		t.Fatal(err)
	}
	if status.SessionID != "session-1" {
		t.Fatalf("SessionID = %q", status.SessionID)
	}
}

func TestRuntimeHTTPServerRoutesSessionTurnToRuntimeService(t *testing.T) {
	t.Parallel()

	service := &recordingRuntimeService{}
	server := newRuntimeHTTPServer(service)
	req, err := http.NewRequest(http.MethodPost, "/v1/sessions/session-1/turns", strings.NewReader(`{"prompt":"hello"}`))
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Authorization", "Bearer "+server.Token())

	resp := httptestResponse(server, req)
	if resp.status != http.StatusOK {
		t.Fatalf("status = %d body = %s", resp.status, resp.body.String())
	}
	if service.chatCalls != 1 {
		t.Fatalf("chatCalls = %d, want 1", service.chatCalls)
	}
}

func TestRuntimeHTTPServerRoutesSkillsToRuntimeService(t *testing.T) {
	t.Parallel()

	service := &recordingRuntimeService{
		skills: RuntimeSkillsResponse{
			Skills: []RuntimeSkill{{Name: "crush-config", Builtin: true, Enabled: true, State: "normal"}},
		},
	}
	server := newRuntimeHTTPServer(service)
	req, err := http.NewRequest(http.MethodGet, "/v1/skills", nil)
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Authorization", "Bearer "+server.Token())

	resp := httptestResponse(server, req)
	if resp.status != http.StatusOK {
		t.Fatalf("status = %d body = %s", resp.status, resp.body.String())
	}
	if service.skillsCalls != 1 {
		t.Fatalf("skillsCalls = %d, want 1", service.skillsCalls)
	}
	var skills RuntimeSkillsResponse
	if err := json.Unmarshal(resp.body.Bytes(), &skills); err != nil {
		t.Fatal(err)
	}
	if len(skills.Skills) != 1 || skills.Skills[0].Name != "crush-config" {
		t.Fatalf("skills = %#v", skills.Skills)
	}
}

func TestRuntimeServiceAPIEndpointBindsLoopbackWithToken(t *testing.T) {
	t.Parallel()

	service := newRuntimeService()
	endpoint, err := service.APIEndpoint(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_ = service.httpAPI.Close(context.Background())
	})

	if !strings.HasPrefix(endpoint.URL, "http://127.0.0.1:") {
		t.Fatalf("URL = %q, want loopback random port", endpoint.URL)
	}
	if endpoint.Token == "" {
		t.Fatal("token is empty")
	}
}

func TestRuntimeHTTPServerStreamsRuntimeEvents(t *testing.T) {
	t.Parallel()

	service := newRuntimeService()
	server := newRuntimeHTTPServer(service)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, "/v1/events", nil)
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Authorization", "Bearer "+server.Token())
	recorder := newStreamingRecorder()
	done := make(chan struct{})
	go func() {
		server.ServeHTTP(recorder, req)
		close(done)
	}()

	recorder.waitForLine(t, ": connected")
	service.publishRuntimeEvent(RuntimeEvent{
		ID:        "event-1",
		Type:      "message.created",
		CreatedAt: "2026-05-18T00:00:00Z",
		MessageID: "message-1",
	})

	line := recorder.waitForPrefix(t, "data: ")
	var event RuntimeEvent
	if err := json.Unmarshal([]byte(strings.TrimPrefix(line, "data: ")), &event); err != nil {
		t.Fatal(err)
	}
	if event.ID != "event-1" || event.MessageID != "message-1" {
		t.Fatalf("event = %#v", event)
	}
	cancel()
	<-done
}

type httpRecorder struct {
	header http.Header
	status int
	body   bytes.Buffer
}

type streamingRecorder struct {
	*httpRecorder
	lines chan string
}

func newStreamingRecorder() *streamingRecorder {
	return &streamingRecorder{
		httpRecorder: &httpRecorder{header: make(http.Header)},
		lines:        make(chan string, 16),
	}
}

func (r *streamingRecorder) Write(data []byte) (int, error) {
	written, err := r.httpRecorder.Write(data)
	scanner := bufio.NewScanner(bytes.NewReader(data))
	for scanner.Scan() {
		if scanner.Text() != "" {
			r.lines <- scanner.Text()
		}
	}
	return written, err
}

func (r *streamingRecorder) Flush() {}

func (r *streamingRecorder) waitForLine(t *testing.T, want string) {
	t.Helper()
	for i := 0; i < 16; i++ {
		line := <-r.lines
		if line == want {
			return
		}
	}
	t.Fatalf("line %q was not received", want)
}

func (r *streamingRecorder) waitForPrefix(t *testing.T, prefix string) string {
	t.Helper()
	for i := 0; i < 16; i++ {
		line := <-r.lines
		if strings.HasPrefix(line, prefix) {
			return line
		}
	}
	t.Fatalf("line with prefix %q was not received", prefix)
	return ""
}

func httptestResponse(handler http.Handler, req *http.Request) httpRecorder {
	recorder := httpRecorder{header: make(http.Header)}
	handler.ServeHTTP(&recorder, req)
	return recorder
}

func (r *httpRecorder) Header() http.Header {
	return r.header
}

func (r *httpRecorder) Write(data []byte) (int, error) {
	if r.status == 0 {
		r.status = http.StatusOK
	}
	return r.body.Write(data)
}

func (r *httpRecorder) WriteHeader(statusCode int) {
	r.status = statusCode
}
