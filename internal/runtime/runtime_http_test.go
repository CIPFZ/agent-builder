package runtime

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"strconv"
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

func TestRuntimeHTTPServerAllowsQueryTokenForEventSource(t *testing.T) {
	t.Parallel()

	server := newRuntimeHTTPServer(&recordingRuntimeService{})
	req, err := http.NewRequest(http.MethodGet, "/v1/events?token="+server.Token(), nil)
	if err != nil {
		t.Fatal(err)
	}
	resp := httptestResponse(server, req)
	if resp.status != http.StatusOK {
		t.Fatalf("status = %d, want %d body = %s", resp.status, http.StatusOK, resp.body.String())
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

func TestRuntimeHTTPServerRoutesModelVerifyToRuntimeService(t *testing.T) {
	t.Parallel()

	service := &recordingRuntimeService{}
	server := newRuntimeHTTPServer(service)
	req, err := http.NewRequest(http.MethodPost, "/v1/config/model/verify", strings.NewReader(`{
  "protocol": "openai",
  "url": "https://api.example.com",
  "apiKey": "secret",
  "model": "test-model"
}`))
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Authorization", "Bearer "+server.Token())

	resp := httptestResponse(server, req)
	if resp.status != http.StatusOK {
		t.Fatalf("status = %d body = %s", resp.status, resp.body.String())
	}
	if strings.Contains(resp.body.String(), "secret") {
		t.Fatalf("verify response leaked api key: %s", resp.body.String())
	}
}

func TestRuntimeHTTPServerRoutesModelDiscoveryToRuntimeService(t *testing.T) {
	t.Parallel()

	service := &recordingRuntimeService{}
	server := newRuntimeHTTPServer(service)
	req, err := http.NewRequest(http.MethodPost, "/v1/config/model/discover", strings.NewReader(`{
  "protocol": "openai",
  "url": "https://api.example.com",
  "apiKey": "secret"
}`))
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Authorization", "Bearer "+server.Token())

	resp := httptestResponse(server, req)
	if resp.status != http.StatusOK {
		t.Fatalf("status = %d body = %s", resp.status, resp.body.String())
	}
	if strings.Contains(resp.body.String(), "secret") {
		t.Fatalf("discovery response leaked api key: %s", resp.body.String())
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

func TestRuntimeHTTPServerRoutesDraftTurnToRuntimeService(t *testing.T) {
	t.Parallel()

	service := &recordingRuntimeService{}
	server := newRuntimeHTTPServer(service)
	req, err := http.NewRequest(http.MethodPost, "/v1/turns", strings.NewReader(`{"prompt":"hello"}`))
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

func TestRuntimeHTTPServerRoutesTurnGetAndCancelToRuntimeService(t *testing.T) {
	t.Parallel()

	service := &recordingRuntimeService{
		turn: RuntimeTurnResponse{Turn: RuntimeTurn{ID: "turn-1", SessionID: "session-1", Status: "running"}},
	}
	server := newRuntimeHTTPServer(service)

	req, err := http.NewRequest(http.MethodGet, "/v1/turns/turn-1", nil)
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Authorization", "Bearer "+server.Token())
	resp := httptestResponse(server, req)
	if resp.status != http.StatusOK {
		t.Fatalf("get status = %d body = %s", resp.status, resp.body.String())
	}
	var turn RuntimeTurnResponse
	if err := json.Unmarshal(resp.body.Bytes(), &turn); err != nil {
		t.Fatal(err)
	}
	if turn.Turn.ID != "turn-1" || turn.Turn.Status != "running" {
		t.Fatalf("turn = %#v", turn.Turn)
	}

	req, err = http.NewRequest(http.MethodPost, "/v1/turns/turn-1/cancel", nil)
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Authorization", "Bearer "+server.Token())
	resp = httptestResponse(server, req)
	if resp.status != http.StatusOK {
		t.Fatalf("cancel status = %d body = %s", resp.status, resp.body.String())
	}
	if service.cancelledTurn != "turn-1" {
		t.Fatalf("cancelledTurn = %q, want turn-1", service.cancelledTurn)
	}
}

func TestRuntimeHTTPServerRoutesTurnsQueryToRuntimeService(t *testing.T) {
	t.Parallel()

	service := &recordingRuntimeService{
		turns: RuntimeTurnsResponse{Turns: []RuntimeTurn{{ID: "turn-1", SessionID: "session-1", Status: "running"}}},
	}
	server := newRuntimeHTTPServer(service)
	req, err := http.NewRequest(http.MethodGet, "/v1/turns?status=active", nil)
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Authorization", "Bearer "+server.Token())
	resp := httptestResponse(server, req)
	if resp.status != http.StatusOK {
		t.Fatalf("status = %d body = %s", resp.status, resp.body.String())
	}
	var turns RuntimeTurnsResponse
	if err := json.Unmarshal(resp.body.Bytes(), &turns); err != nil {
		t.Fatal(err)
	}
	if len(turns.Turns) != 1 || turns.Turns[0].ID != "turn-1" {
		t.Fatalf("turns = %#v", turns.Turns)
	}
	if service.turnsStatus != "active" {
		t.Fatalf("turns status = %q, want active", service.turnsStatus)
	}
}

func TestRuntimeHTTPServerRoutesToolCallQueriesToRuntimeService(t *testing.T) {
	t.Parallel()

	service := &recordingRuntimeService{
		toolCall: RuntimeToolCallResponse{ToolCall: RuntimeToolCall{ID: "tool-1", TurnID: "turn-1", Name: "bash"}},
		toolCalls: RuntimeToolCallsResponse{ToolCalls: []RuntimeToolCall{
			{ID: "tool-1", TurnID: "turn-1", Name: "bash"},
		}},
	}
	server := newRuntimeHTTPServer(service)

	req, err := http.NewRequest(http.MethodGet, "/v1/turns/turn-1/tool-calls", nil)
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Authorization", "Bearer "+server.Token())
	resp := httptestResponse(server, req)
	if resp.status != http.StatusOK {
		t.Fatalf("list status = %d body = %s", resp.status, resp.body.String())
	}
	var list RuntimeToolCallsResponse
	if err := json.Unmarshal(resp.body.Bytes(), &list); err != nil {
		t.Fatal(err)
	}
	if len(list.ToolCalls) != 1 || list.ToolCalls[0].ID != "tool-1" {
		t.Fatalf("tool calls = %#v", list.ToolCalls)
	}

	req, err = http.NewRequest(http.MethodGet, "/v1/tool-calls/tool-1", nil)
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Authorization", "Bearer "+server.Token())
	resp = httptestResponse(server, req)
	if resp.status != http.StatusOK {
		t.Fatalf("get status = %d body = %s", resp.status, resp.body.String())
	}
	var detail RuntimeToolCallResponse
	if err := json.Unmarshal(resp.body.Bytes(), &detail); err != nil {
		t.Fatal(err)
	}
	if detail.ToolCall.ID != "tool-1" {
		t.Fatalf("tool call = %#v", detail.ToolCall)
	}
}

func TestRuntimeHTTPServerRoutesSessionManagementToRuntimeService(t *testing.T) {
	t.Parallel()

	service := &recordingRuntimeService{status: RuntimeStatus{Ready: true, SessionID: "session-1"}}
	server := newRuntimeHTTPServer(service)

	req, err := http.NewRequest(http.MethodGet, "/v1/sessions", nil)
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Authorization", "Bearer "+server.Token())
	resp := httptestResponse(server, req)
	if resp.status != http.StatusOK {
		t.Fatalf("list status = %d body = %s", resp.status, resp.body.String())
	}
	var sessions RuntimeSessionsResponse
	if err := json.Unmarshal(resp.body.Bytes(), &sessions); err != nil {
		t.Fatal(err)
	}
	if len(sessions.Sessions) != 2 || sessions.Sessions[0].ID != "session-1" {
		t.Fatalf("sessions = %#v", sessions.Sessions)
	}

	req, err = http.NewRequest(http.MethodPost, "/v1/sessions/session-2/select", nil)
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Authorization", "Bearer "+server.Token())
	resp = httptestResponse(server, req)
	if resp.status != http.StatusOK || service.selectedSession != "session-2" {
		t.Fatalf("select status = %d session = %q body = %s", resp.status, service.selectedSession, resp.body.String())
	}

	req, err = http.NewRequest(http.MethodGet, "/v1/sessions/session-2/messages", nil)
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Authorization", "Bearer "+server.Token())
	resp = httptestResponse(server, req)
	if resp.status != http.StatusOK || service.messageSession != "session-2" {
		t.Fatalf("messages status = %d session = %q body = %s", resp.status, service.messageSession, resp.body.String())
	}

	req, err = http.NewRequest(http.MethodPut, "/v1/sessions/session-2", strings.NewReader(`{"title":"Renamed"}`))
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Authorization", "Bearer "+server.Token())
	resp = httptestResponse(server, req)
	if resp.status != http.StatusOK || service.renamedSession.SessionID != "session-2" || service.renamedSession.Title != "Renamed" {
		t.Fatalf("rename status = %d req = %#v body = %s", resp.status, service.renamedSession, resp.body.String())
	}

	req, err = http.NewRequest(http.MethodDelete, "/v1/sessions/session-2", nil)
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Authorization", "Bearer "+server.Token())
	resp = httptestResponse(server, req)
	if resp.status != http.StatusOK || service.deletedSession != "session-2" {
		t.Fatalf("delete status = %d session = %q body = %s", resp.status, service.deletedSession, resp.body.String())
	}
}

func TestRuntimeHTTPServerSmokeCoversSessionTurnAndInventory(t *testing.T) {
	t.Parallel()

	service := &recordingRuntimeService{
		status: RuntimeStatus{Ready: true, SessionID: "session-1"},
		skills: RuntimeSkillsResponse{
			Skills: []RuntimeSkill{{Name: "crush-config", Builtin: true, Enabled: true, State: "normal"}},
		},
		mcpServers: RuntimeMCPServersResponse{
			Servers: []RuntimeMCPServer{{Name: "docs", Type: "http", State: "connected"}},
		},
		capabilities: RuntimeCapabilitiesResponse{
			Capabilities: []RuntimeCapability{{ID: "skill:crush-config", Kind: "skill", Name: "crush-config", Enabled: true}},
		},
	}
	server := newRuntimeHTTPServer(service)
	client := runtimeSmokeClient{server: server, token: server.Token()}

	if status := client.postSession(t); status.SessionID != "session-1" {
		t.Fatalf("new session status = %#v", status)
	}
	if response := client.postTurn(t, "session-1", "hello"); response.RequestID == "" {
		t.Fatalf("turn response = %#v", response)
	}
	if skills := client.getSkills(t); len(skills.Skills) != 1 {
		t.Fatalf("skills = %#v", skills)
	}
	if servers := client.getMCPServers(t); len(servers.Servers) != 1 {
		t.Fatalf("mcp servers = %#v", servers)
	}
	if capabilities := client.getCapabilities(t); len(capabilities.Capabilities) != 1 {
		t.Fatalf("capabilities = %#v", capabilities)
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

func TestRuntimeHTTPServerRoutesSkillManagementToRuntimeService(t *testing.T) {
	t.Parallel()

	service := &recordingRuntimeService{}
	server := newRuntimeHTTPServer(service)
	req, err := http.NewRequest(http.MethodPost, "/v1/skills", strings.NewReader(`{
  "name": "my-skill",
  "description": "Use when testing.",
  "instructions": "# My Skill"
}`))
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Authorization", "Bearer "+server.Token())
	resp := httptestResponse(server, req)
	if resp.status != http.StatusOK {
		t.Fatalf("create status = %d body = %s", resp.status, resp.body.String())
	}
	if service.createdSkill.Name != "my-skill" || service.createdSkill.Description == "" {
		t.Fatalf("created skill = %#v", service.createdSkill)
	}

	req, err = http.NewRequest(http.MethodPost, "/v1/skills/paths", strings.NewReader(`{"path":".agents/skills"}`))
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Authorization", "Bearer "+server.Token())
	resp = httptestResponse(server, req)
	if resp.status != http.StatusOK {
		t.Fatalf("path status = %d body = %s", resp.status, resp.body.String())
	}
	if service.addedSkillPath != ".agents/skills" {
		t.Fatalf("added skill path = %q", service.addedSkillPath)
	}
}

func TestRuntimeHTTPServerRoutesMCPServersToRuntimeService(t *testing.T) {
	t.Parallel()

	service := &recordingRuntimeService{
		mcpServers: RuntimeMCPServersResponse{
			Servers: []RuntimeMCPServer{{Name: "docs", Type: "http", State: "connected"}},
		},
	}
	server := newRuntimeHTTPServer(service)
	req, err := http.NewRequest(http.MethodGet, "/v1/mcp/servers", nil)
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Authorization", "Bearer "+server.Token())

	resp := httptestResponse(server, req)
	if resp.status != http.StatusOK {
		t.Fatalf("status = %d body = %s", resp.status, resp.body.String())
	}
	if service.mcpServerCalls != 1 {
		t.Fatalf("mcpServerCalls = %d, want 1", service.mcpServerCalls)
	}
}

func TestRuntimeHTTPServerRoutesMCPEditingToRuntimeService(t *testing.T) {
	t.Parallel()

	service := &recordingRuntimeService{}
	server := newRuntimeHTTPServer(service)
	req, err := http.NewRequest(http.MethodPut, "/v1/mcp/servers/docs", strings.NewReader(`{
  "type": "http",
  "url": "https://example.com/mcp",
  "headers": {"Authorization": "Bearer secret"}
}`))
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Authorization", "Bearer "+server.Token())

	resp := httptestResponse(server, req)
	if resp.status != http.StatusOK {
		t.Fatalf("status = %d body = %s", resp.status, resp.body.String())
	}
	if service.savedMCPServer.Name != "docs" || service.savedMCPServer.URL != "https://example.com/mcp" {
		t.Fatalf("saved mcp server = %#v", service.savedMCPServer)
	}
	if strings.Contains(resp.body.String(), "secret") {
		t.Fatalf("mcp response leaked secret: %s", resp.body.String())
	}
}

func TestRuntimeHTTPServerRoutesMCPToolToggleBeforeServerToggle(t *testing.T) {
	t.Parallel()

	service := &recordingRuntimeService{}
	server := newRuntimeHTTPServer(service)
	req, err := http.NewRequest(http.MethodPost, "/v1/mcp/servers/docs/tools/search/enabled", strings.NewReader(`{"enabled":false}`))
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Authorization", "Bearer "+server.Token())

	resp := httptestResponse(server, req)
	if resp.status != http.StatusOK {
		t.Fatalf("status = %d body = %s", resp.status, resp.body.String())
	}
	if service.toggledMCPTool.Server != "docs" || service.toggledMCPTool.Tool != "search" || service.toggledMCPTool.Enabled {
		t.Fatalf("tool toggle = %#v", service.toggledMCPTool)
	}
	if service.toggledMCPServer.Name != "" {
		t.Fatalf("server toggle was called for tool route: %#v", service.toggledMCPServer)
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
	req.Header.Set("Accept", "text/event-stream")
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

func TestRuntimeHTTPServerRoutesEventsHistoryToRuntimeService(t *testing.T) {
	t.Parallel()

	service := newRuntimeService()
	service.storeRuntimeEvent(RuntimeEvent{
		ID:        "event-1",
		Type:      "message.created",
		CreatedAt: "2026-05-18T00:00:00Z",
		MessageID: "message-1",
	})
	server := newRuntimeHTTPServer(service)
	req, err := http.NewRequest(http.MethodGet, "/v1/events", nil)
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Authorization", "Bearer "+server.Token())

	resp := httptestResponse(server, req)
	if resp.status != http.StatusOK {
		t.Fatalf("status = %d body = %s", resp.status, resp.body.String())
	}
	var history RuntimeEventsResponse
	if err := json.Unmarshal(resp.body.Bytes(), &history); err != nil {
		t.Fatal(err)
	}
	if len(history.Events) != 1 || history.Events[0].ID != "event-1" {
		t.Fatalf("history = %#v", history.Events)
	}
}

func TestRuntimeHTTPServerRoutesEventsAfterCursor(t *testing.T) {
	t.Parallel()

	service := newRuntimeService()
	first := service.storeRuntimeEvent(RuntimeEvent{
		Type:      "message.created",
		CreatedAt: "2026-05-18T00:00:00Z",
		MessageID: "message-1",
	})
	second := service.storeRuntimeEvent(RuntimeEvent{
		Type:      "message.created",
		CreatedAt: "2026-05-18T00:00:01Z",
		MessageID: "message-2",
	})
	server := newRuntimeHTTPServer(service)
	req, err := http.NewRequest(http.MethodGet, "/v1/events?after="+strconv.FormatInt(first.Sequence, 10), nil)
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Authorization", "Bearer "+server.Token())

	resp := httptestResponse(server, req)
	if resp.status != http.StatusOK {
		t.Fatalf("status = %d body = %s", resp.status, resp.body.String())
	}
	var history RuntimeEventsResponse
	if err := json.Unmarshal(resp.body.Bytes(), &history); err != nil {
		t.Fatal(err)
	}
	if len(history.Events) != 1 || history.Events[0].Sequence != second.Sequence {
		t.Fatalf("history = %#v", history.Events)
	}
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

type runtimeSmokeClient struct {
	server *runtimeHTTPServer
	token  string
}

func (c runtimeSmokeClient) postSession(t *testing.T) RuntimeStatus {
	t.Helper()
	req, err := http.NewRequest(http.MethodPost, "/v1/sessions", strings.NewReader(`{"title":"Smoke"}`))
	if err != nil {
		t.Fatal(err)
	}
	var status RuntimeStatus
	c.doJSON(t, req, &status)
	return status
}

func (c runtimeSmokeClient) postTurn(t *testing.T, sessionID, prompt string) RuntimeChatResponse {
	t.Helper()
	req, err := http.NewRequest(http.MethodPost, "/v1/sessions/"+sessionID+"/turns", strings.NewReader(`{"prompt":`+strconv.Quote(prompt)+`}`))
	if err != nil {
		t.Fatal(err)
	}
	var response RuntimeChatResponse
	c.doJSON(t, req, &response)
	return response
}

func (c runtimeSmokeClient) getSkills(t *testing.T) RuntimeSkillsResponse {
	t.Helper()
	req, err := http.NewRequest(http.MethodGet, "/v1/skills", nil)
	if err != nil {
		t.Fatal(err)
	}
	var response RuntimeSkillsResponse
	c.doJSON(t, req, &response)
	return response
}

func (c runtimeSmokeClient) getMCPServers(t *testing.T) RuntimeMCPServersResponse {
	t.Helper()
	req, err := http.NewRequest(http.MethodGet, "/v1/mcp/servers", nil)
	if err != nil {
		t.Fatal(err)
	}
	var response RuntimeMCPServersResponse
	c.doJSON(t, req, &response)
	return response
}

func (c runtimeSmokeClient) getCapabilities(t *testing.T) RuntimeCapabilitiesResponse {
	t.Helper()
	req, err := http.NewRequest(http.MethodGet, "/v1/capabilities", nil)
	if err != nil {
		t.Fatal(err)
	}
	var response RuntimeCapabilitiesResponse
	c.doJSON(t, req, &response)
	return response
}

func (c runtimeSmokeClient) doJSON(t *testing.T, req *http.Request, target any) {
	t.Helper()
	req.Header.Set("Authorization", "Bearer "+c.token)
	resp := httptestResponse(c.server, req)
	if resp.status != http.StatusOK {
		t.Fatalf("%s %s status = %d body = %s", req.Method, req.URL.Path, resp.status, resp.body.String())
	}
	if err := json.Unmarshal(resp.body.Bytes(), target); err != nil {
		t.Fatal(err)
	}
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
