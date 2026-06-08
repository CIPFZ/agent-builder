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

	"github.com/charmbracelet/crush/internal/runtimeapi"
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

func TestRuntimeHTTPServerRoutesRecoveryStatusToRuntimeService(t *testing.T) {
	t.Parallel()

	service := &recordingRuntimeService{
		recoveryStatus: RuntimeRecoveryStatus{
			RuntimeStartedAt:  "2026-05-23T00:00:00Z",
			LastEventSequence: 7,
			ActiveTurns:       []RuntimeTurn{{ID: "turn-1", SessionID: "session-1", Status: "running"}},
			InterruptedTurns:  []RuntimeTurn{{ID: "turn-2", SessionID: "session-1", Status: "interrupted"}},
			PendingPermissions: []RuntimePermissionRequest{{
				ID:         "perm-1",
				SessionID:  "session-1",
				TurnID:     "turn-1",
				ToolCallID: "tool-1",
				ToolName:   "bash",
				Action:     "execute",
				Risk:       "execute",
				Status:     "pending",
				CreatedAt:  1000,
			}},
		},
	}
	server := newRuntimeHTTPServer(service)
	req, err := http.NewRequest(http.MethodGet, "/v1/recovery/status", nil)
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Authorization", "Bearer "+server.Token())

	resp := httptestResponse(server, req)
	if resp.status != http.StatusOK {
		t.Fatalf("status = %d body = %s", resp.status, resp.body.String())
	}
	if service.recoveryStatusCalls != 1 {
		t.Fatalf("recoveryStatusCalls = %d, want 1", service.recoveryStatusCalls)
	}
	var recovery RuntimeRecoveryStatus
	if err := json.Unmarshal(resp.body.Bytes(), &recovery); err != nil {
		t.Fatal(err)
	}
	if recovery.LastEventSequence != 7 || len(recovery.ActiveTurns) != 1 || len(recovery.PendingPermissions) != 1 {
		t.Fatalf("recovery = %#v", recovery)
	}
}

func TestRuntimeHTTPServerRoutesMCPRequestsToRuntimeService(t *testing.T) {
	t.Parallel()

	service := &recordingRuntimeService{
		mcpRequests: RuntimeMCPRequestsResponse{Requests: []RuntimeMCPRequest{{
			ID:       "mcp-req-1",
			Kind:     "auth",
			Server:   "docs",
			Status:   "pending",
			Redacted: true,
		}}},
		mcpRequest: RuntimeMCPRequestResponse{Request: RuntimeMCPRequest{
			ID:       "mcp-req-1",
			Kind:     "auth",
			Server:   "docs",
			Status:   "completed",
			Redacted: true,
		}},
	}
	server := newRuntimeHTTPServer(service)
	req, err := http.NewRequest(http.MethodGet, "/v1/mcp/requests?kind=auth&status=pending&server=docs", nil)
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Authorization", "Bearer "+server.Token())
	resp := httptestResponse(server, req)
	if resp.status != http.StatusOK {
		t.Fatalf("list status = %d body = %s", resp.status, resp.body.String())
	}
	var list RuntimeMCPRequestsResponse
	if err := json.Unmarshal(resp.body.Bytes(), &list); err != nil {
		t.Fatal(err)
	}
	if len(list.Requests) != 1 || list.Requests[0].ID != "mcp-req-1" {
		t.Fatalf("list = %#v", list)
	}

	req, err = http.NewRequest(http.MethodPost, "/v1/mcp/requests/mcp-req-1/decision", strings.NewReader(`{"action":"approve","responseSummary":"approved"}`))
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Authorization", "Bearer "+server.Token())
	resp = httptestResponse(server, req)
	if resp.status != http.StatusOK {
		t.Fatalf("decision status = %d body = %s", resp.status, resp.body.String())
	}
	if service.mcpRequestDecision.RequestID != "mcp-req-1" || service.mcpRequestDecision.Action != "approve" {
		t.Fatalf("decision = %#v", service.mcpRequestDecision)
	}
}

func TestRuntimeHTTPServerRoutesPolicyToRuntimeService(t *testing.T) {
	t.Parallel()

	service := &recordingRuntimeService{
		policy: RuntimePolicyResponse{Policy: RuntimePolicy{Mode: "ask", Modes: []string{"ask", "auto_read", "plan", "deny_all"}}},
	}
	server := newRuntimeHTTPServer(service)
	req, err := http.NewRequest(http.MethodGet, "/v1/policy", nil)
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Authorization", "Bearer "+server.Token())

	resp := httptestResponse(server, req)
	if resp.status != http.StatusOK {
		t.Fatalf("get status = %d body = %s", resp.status, resp.body.String())
	}
	if service.policyCalls != 1 {
		t.Fatalf("policyCalls = %d, want 1", service.policyCalls)
	}

	req, err = http.NewRequest(http.MethodPut, "/v1/policy", strings.NewReader(`{"mode":"plan","profile":"default","rules":[{"id":"deny-bash","decision":"deny","builtinTool":"bash","source":"test"}]}`))
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Authorization", "Bearer "+server.Token())
	resp = httptestResponse(server, req)
	if resp.status != http.StatusOK {
		t.Fatalf("put status = %d body = %s", resp.status, resp.body.String())
	}
	if service.updatedPolicyMode != "plan" {
		t.Fatalf("updated policy mode = %q, want plan", service.updatedPolicyMode)
	}
	if service.updatedPolicyProfile != "default" || len(service.updatedPolicyRules) != 1 || service.updatedPolicyRules[0].ID != "deny-bash" {
		t.Fatalf("updated policy rules/profile = %#v %q", service.updatedPolicyRules, service.updatedPolicyProfile)
	}
}

func TestRuntimeHTTPServerRoutesSessionActivityToRuntimeService(t *testing.T) {
	t.Parallel()

	service := &recordingRuntimeService{
		activity: RuntimeSessionActivityResponse{
			SessionID: "session-1",
			Messages: []RuntimeMessage{{
				ID:        "msg-1",
				SessionID: "session-1",
				Role:      "assistant",
				Content:   "done",
				CreatedAt: 1000,
			}},
			Turns: []RuntimeTurn{{ID: "turn-1", SessionID: "session-1", Status: "completed"}},
			ToolCalls: []RuntimeToolCall{{
				ID:        "tool-1",
				SessionID: "session-1",
				TurnID:    "turn-1",
				Name:      "bash",
				Source:    "shell",
				Status:    "completed",
				StartedAt: 1000,
			}},
			Permissions: []RuntimePermissionRequest{{
				ID:         "perm-1",
				SessionID:  "session-1",
				TurnID:     "turn-1",
				ToolCallID: "tool-1",
				ToolName:   "bash",
				Action:     "execute",
				Status:     "allowed_once",
				CreatedAt:  1000,
				DecidedAt:  1100,
			}},
			Policy: RuntimePolicy{Mode: "ask"},
		},
	}
	server := newRuntimeHTTPServer(service)
	req, err := http.NewRequest(http.MethodGet, "/v1/sessions/session-1/activity", nil)
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Authorization", "Bearer "+server.Token())

	resp := httptestResponse(server, req)
	if resp.status != http.StatusOK {
		t.Fatalf("status = %d body = %s", resp.status, resp.body.String())
	}
	if service.activitySession != "session-1" {
		t.Fatalf("activitySession = %q", service.activitySession)
	}
	var activity RuntimeSessionActivityResponse
	if err := json.Unmarshal(resp.body.Bytes(), &activity); err != nil {
		t.Fatal(err)
	}
	if activity.SessionID != "session-1" || len(activity.ToolCalls) != 1 || len(activity.Permissions) != 1 || activity.Permissions[0].Status != "allowed_once" {
		t.Fatalf("activity = %#v", activity)
	}
}

func TestRuntimeHTTPServerRoutesNarrowActivityToRuntimeService(t *testing.T) {
	t.Parallel()

	service := &recordingRuntimeService{
		activityWindow: RuntimeSessionActivityWindowResponse{
			SessionID: "session-1",
			Turns:     []RuntimeTurn{{ID: "turn-1", SessionID: "session-1", Status: "running"}},
			Window:    RuntimeActivityWindow{Limit: 2, ToEnd: true},
		},
		turnActivity: RuntimeTurnActivityResponse{
			SessionID: "session-1",
			TurnID:    "turn-1",
			Turns:     []RuntimeTurn{{ID: "turn-1", SessionID: "session-1", Status: "running"}},
		},
		runProjection: RuntimeRunProjectionResponse{Run: RuntimeRunProjection{
			ID:               "run:session:session-1",
			PrimarySessionID: "session-1",
			Source: RuntimeRunProjectionSource{
				Kind:                  "session_activity_projection",
				ReadOnly:              true,
				SessionActivityParity: true,
			},
		}},
	}
	server := newRuntimeHTTPServer(service)

	req, err := http.NewRequest(http.MethodGet, "/v1/sessions/session-1/activity-window?limit=2&cursor=v1%3A00000000000000000100%3A010%3Amessage%3Am1", nil)
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Authorization", "Bearer "+server.Token())
	resp := httptestResponse(server, req)
	if resp.status != http.StatusOK {
		t.Fatalf("window status = %d body = %s", resp.status, resp.body.String())
	}
	if service.activityWindowSession != "session-1" || service.activityWindowLimit != 2 || service.activityWindowCursor == "" {
		t.Fatalf("window args = %q %q %d", service.activityWindowSession, service.activityWindowCursor, service.activityWindowLimit)
	}
	var window RuntimeSessionActivityWindowResponse
	if err := json.Unmarshal(resp.body.Bytes(), &window); err != nil {
		t.Fatal(err)
	}
	if window.Window.Limit != 2 || len(window.Turns) != 1 {
		t.Fatalf("window = %#v", window)
	}

	req, err = http.NewRequest(http.MethodGet, "/v1/turns/turn-1/activity", nil)
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Authorization", "Bearer "+server.Token())
	resp = httptestResponse(server, req)
	if resp.status != http.StatusOK {
		t.Fatalf("turn activity status = %d body = %s", resp.status, resp.body.String())
	}
	if service.turnActivityID != "turn-1" {
		t.Fatalf("turn activity id = %q", service.turnActivityID)
	}
	var turnActivity RuntimeTurnActivityResponse
	if err := json.Unmarshal(resp.body.Bytes(), &turnActivity); err != nil {
		t.Fatal(err)
	}
	if turnActivity.TurnID != "turn-1" || len(turnActivity.Turns) != 1 {
		t.Fatalf("turn activity = %#v", turnActivity)
	}

	req, err = http.NewRequest(http.MethodGet, "/v1/sessions/session-1/run-projection?limit=4&cursor=v1%3Arun", nil)
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Authorization", "Bearer "+server.Token())
	resp = httptestResponse(server, req)
	if resp.status != http.StatusOK {
		t.Fatalf("run projection status = %d body = %s", resp.status, resp.body.String())
	}
	if service.runProjectionRequest.SessionID != "session-1" || service.runProjectionRequest.Limit != 4 || service.runProjectionRequest.Cursor != "v1:run" {
		t.Fatalf("run projection args = %#v", service.runProjectionRequest)
	}
	var projection RuntimeRunProjectionResponse
	if err := json.Unmarshal(resp.body.Bytes(), &projection); err != nil {
		t.Fatal(err)
	}
	if !projection.Run.Source.ReadOnly || !projection.Run.Source.SessionActivityParity {
		t.Fatalf("run projection source = %#v", projection.Run.Source)
	}
}

func TestRuntimeHTTPServerRoutesRunsToRuntimeService(t *testing.T) {
	t.Parallel()

	service := &recordingRuntimeService{
		runs: RuntimeRunsResponse{Runs: []RuntimeRun{{
			ID:               "run-1",
			WorkspaceID:      "workspace-1",
			PrimarySessionID: "session-1",
			SessionIDs:       []string{"session-1"},
			Objective:        "ship durable runs",
			Status:           "completed",
			Source:           runtimeRunSourceBackfill,
			CreatedAt:        1000,
			UpdatedAt:        1200,
		}}},
		run: RuntimeRunResponse{
			Run: RuntimeRun{
				ID:               "run-1",
				WorkspaceID:      "workspace-1",
				PrimarySessionID: "session-1",
				SessionIDs:       []string{"session-1"},
				Status:           "completed",
				Source:           runtimeRunSourceBackfill,
				CreatedAt:        1000,
				UpdatedAt:        1200,
			},
			Projection: RuntimeRunProjection{
				ID:               "run-1",
				PrimarySessionID: "session-1",
				Source: RuntimeRunProjectionSource{
					Kind:                  runtimeRunProjectionSourceKind,
					ReadOnly:              true,
					SessionActivityParity: true,
				},
			},
		},
	}
	server := newRuntimeHTTPServer(service)

	req, err := http.NewRequest(http.MethodGet, "/v1/runs", nil)
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Authorization", "Bearer "+server.Token())
	resp := httptestResponse(server, req)
	if resp.status != http.StatusOK {
		t.Fatalf("runs status = %d body = %s", resp.status, resp.body.String())
	}
	var runs RuntimeRunsResponse
	if err := json.Unmarshal(resp.body.Bytes(), &runs); err != nil {
		t.Fatal(err)
	}
	if len(runs.Runs) != 1 || runs.Runs[0].ID != "run-1" {
		t.Fatalf("runs = %#v", runs)
	}

	req, err = http.NewRequest(http.MethodGet, "/v1/runs/run-1", nil)
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Authorization", "Bearer "+server.Token())
	resp = httptestResponse(server, req)
	if resp.status != http.StatusOK {
		t.Fatalf("run status = %d body = %s", resp.status, resp.body.String())
	}
	if service.runID != "run-1" {
		t.Fatalf("run id = %q, want run-1", service.runID)
	}
	var run RuntimeRunResponse
	if err := json.Unmarshal(resp.body.Bytes(), &run); err != nil {
		t.Fatal(err)
	}
	if run.Run.ID != "run-1" || run.Projection.ID != "run-1" || !run.Projection.Source.SessionActivityParity {
		t.Fatalf("run response = %#v", run)
	}
}

func TestRuntimeHTTPServerDevModuleRoutesToolPermissionAndPolicy(t *testing.T) {
	t.Parallel()

	service := &recordingRuntimeService{
		activity: RuntimeSessionActivityResponse{SessionID: "session-1", Policy: RuntimePolicy{Mode: "ask"}},
		toolCalls: RuntimeToolCallsResponse{ToolCalls: []RuntimeToolCall{{
			ID:        "tool-1",
			SessionID: "session-1",
			TurnID:    "turn-1",
			Name:      "bash",
			Source:    "shell",
			Status:    "waiting_permission",
			StartedAt: 1000,
		}}},
		policy: RuntimePolicyResponse{Policy: RuntimePolicy{Mode: "ask"}},
		runs: RuntimeRunsResponse{Runs: []RuntimeRun{{
			ID:               "run-1",
			WorkspaceID:      "workspace-1",
			PrimarySessionID: "session-1",
			Status:           "completed",
			Source:           runtimeRunSourceBackfill,
			CreatedAt:        1000,
			UpdatedAt:        1200,
		}}},
		run: RuntimeRunResponse{Run: RuntimeRun{
			ID:               "run-1",
			WorkspaceID:      "workspace-1",
			PrimarySessionID: "session-1",
			Status:           "completed",
			Source:           runtimeRunSourceBackfill,
			CreatedAt:        1000,
			UpdatedAt:        1200,
		}},
	}
	server := newRuntimeHTTPServer(service)

	req, err := http.NewRequest(http.MethodGet, "/v1/dev/module?token="+server.Token()+"&path=/v1/sessions/session-1/activity", nil)
	if err != nil {
		t.Fatal(err)
	}
	resp := httptestResponse(server, req)
	if resp.status != http.StatusOK || !strings.Contains(resp.body.String(), "session-1") {
		t.Fatalf("activity status = %d body = %s", resp.status, resp.body.String())
	}

	req, err = http.NewRequest(http.MethodGet, "/v1/dev/module?token="+server.Token()+"&path=/v1/turns/turn-1/tool-calls", nil)
	if err != nil {
		t.Fatal(err)
	}
	resp = httptestResponse(server, req)
	if resp.status != http.StatusOK || !strings.Contains(resp.body.String(), "tool-1") {
		t.Fatalf("tool calls status = %d body = %s", resp.status, resp.body.String())
	}

	req, err = http.NewRequest(http.MethodGet, "/v1/dev/module?token="+server.Token()+"&path=/v1/sessions/session-1/activity-window&limit=3", nil)
	if err != nil {
		t.Fatal(err)
	}
	resp = httptestResponse(server, req)
	if resp.status != http.StatusOK || service.activityWindowSession != "session-1" || service.activityWindowLimit != 3 {
		t.Fatalf("activity window status = %d body = %s args=%q/%d", resp.status, resp.body.String(), service.activityWindowSession, service.activityWindowLimit)
	}

	req, err = http.NewRequest(http.MethodGet, "/v1/dev/module?token="+server.Token()+"&path=/v1/sessions/session-1/run-projection&limit=5&cursor=v1%3Arun", nil)
	if err != nil {
		t.Fatal(err)
	}
	resp = httptestResponse(server, req)
	if resp.status != http.StatusOK || service.runProjectionRequest.SessionID != "session-1" || service.runProjectionRequest.Limit != 5 || service.runProjectionRequest.Cursor != "v1:run" {
		t.Fatalf("run projection status = %d body = %s args=%#v", resp.status, resp.body.String(), service.runProjectionRequest)
	}

	req, err = http.NewRequest(http.MethodGet, "/v1/dev/module?token="+server.Token()+"&path=/v1/turns/turn-1/activity", nil)
	if err != nil {
		t.Fatal(err)
	}
	resp = httptestResponse(server, req)
	if resp.status != http.StatusOK || service.turnActivityID != "turn-1" {
		t.Fatalf("turn activity status = %d body = %s id=%q", resp.status, resp.body.String(), service.turnActivityID)
	}

	req, err = http.NewRequest(http.MethodGet, "/v1/dev/module?token="+server.Token()+"&path=/v1/runs", nil)
	if err != nil {
		t.Fatal(err)
	}
	resp = httptestResponse(server, req)
	if resp.status != http.StatusOK || !strings.Contains(resp.body.String(), "run-1") {
		t.Fatalf("runs status = %d body = %s", resp.status, resp.body.String())
	}

	req, err = http.NewRequest(http.MethodGet, "/v1/dev/module?token="+server.Token()+"&path=/v1/runs/run-1", nil)
	if err != nil {
		t.Fatal(err)
	}
	resp = httptestResponse(server, req)
	if resp.status != http.StatusOK || service.runID != "run-1" {
		t.Fatalf("run status = %d body = %s id=%q", resp.status, resp.body.String(), service.runID)
	}

	body := `%7B%22mode%22%3A%22auto_read%22%7D`
	req, err = http.NewRequest(http.MethodGet, "/v1/dev/module?token="+server.Token()+"&method=PUT&path=/v1/policy&body="+body, nil)
	if err != nil {
		t.Fatal(err)
	}
	resp = httptestResponse(server, req)
	if resp.status != http.StatusOK {
		t.Fatalf("policy status = %d body = %s", resp.status, resp.body.String())
	}
	if service.updatedPolicyMode != "auto_read" {
		t.Fatalf("updatedPolicyMode = %q", service.updatedPolicyMode)
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
		toolCall: RuntimeToolCallResponse{ToolCall: RuntimeToolCall{ID: "tool-1", TurnID: "turn-1", Name: "bash", Display: RuntimeToolCallDisplay{Kind: "shell", Title: "已运行 1 条命令"}}},
		toolCalls: RuntimeToolCallsResponse{ToolCalls: []RuntimeToolCall{
			{ID: "tool-1", TurnID: "turn-1", Name: "bash", Display: RuntimeToolCallDisplay{Kind: "shell", Title: "已运行 1 条命令"}},
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
	if len(list.ToolCalls) != 1 || list.ToolCalls[0].ID != "tool-1" || list.ToolCalls[0].Display.Kind != "shell" {
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
	if detail.ToolCall.ID != "tool-1" || detail.ToolCall.Display.Title != "已运行 1 条命令" {
		t.Fatalf("tool call = %#v", detail.ToolCall)
	}
}

func TestRuntimeHTTPServerRoutesHookQueriesToRuntimeService(t *testing.T) {
	t.Parallel()

	service := &recordingRuntimeService{
		hooks: RuntimeHooksResponse{Hooks: []RuntimeHook{{
			ID:      "hook-1",
			Name:    "bash",
			Source:  "config",
			Event:   "PreToolUse",
			Enabled: true,
			Status:  "enabled",
		}}},
		hookExecution: RuntimeHookExecutionResponse{Execution: RuntimeHookExecution{
			ID:         "hook-exec-1",
			HookID:     "hook-1",
			Event:      "PreToolUse",
			Status:     "completed",
			SessionID:  "session-1",
			TurnID:     "turn-1",
			ToolCallID: "tool-1",
			Redacted:   true,
		}},
		hookExecutions: RuntimeHookExecutionsResponse{Executions: []RuntimeHookExecution{{
			ID:         "hook-exec-1",
			HookID:     "hook-1",
			Event:      "PreToolUse",
			Status:     "completed",
			SessionID:  "session-1",
			TurnID:     "turn-1",
			ToolCallID: "tool-1",
			Redacted:   true,
		}}},
	}
	server := newRuntimeHTTPServer(service)

	req, err := http.NewRequest(http.MethodGet, "/v1/hooks", nil)
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Authorization", "Bearer "+server.Token())
	resp := httptestResponse(server, req)
	if resp.status != http.StatusOK {
		t.Fatalf("hooks status = %d body = %s", resp.status, resp.body.String())
	}
	var hooksResp RuntimeHooksResponse
	if err := json.Unmarshal(resp.body.Bytes(), &hooksResp); err != nil {
		t.Fatal(err)
	}
	if len(hooksResp.Hooks) != 1 || hooksResp.Hooks[0].ID != "hook-1" {
		t.Fatalf("hooks = %#v", hooksResp.Hooks)
	}

	req, err = http.NewRequest(http.MethodGet, "/v1/hook-executions?session_id=session-1&turn_id=turn-1&tool_call_id=tool-1&event=PreToolUse&status=completed", nil)
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Authorization", "Bearer "+server.Token())
	resp = httptestResponse(server, req)
	if resp.status != http.StatusOK {
		t.Fatalf("hook executions status = %d body = %s", resp.status, resp.body.String())
	}
	if service.hookExecutionsReq.SessionID != "session-1" ||
		service.hookExecutionsReq.TurnID != "turn-1" ||
		service.hookExecutionsReq.ToolCallID != "tool-1" ||
		service.hookExecutionsReq.Event != "PreToolUse" ||
		service.hookExecutionsReq.Status != "completed" {
		t.Fatalf("hook execution request = %#v", service.hookExecutionsReq)
	}

	req, err = http.NewRequest(http.MethodGet, "/v1/hook-executions/hook-exec-1", nil)
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Authorization", "Bearer "+server.Token())
	resp = httptestResponse(server, req)
	if resp.status != http.StatusOK {
		t.Fatalf("hook execution status = %d body = %s", resp.status, resp.body.String())
	}
	var detail RuntimeHookExecutionResponse
	if err := json.Unmarshal(resp.body.Bytes(), &detail); err != nil {
		t.Fatal(err)
	}
	if detail.Execution.ID != "hook-exec-1" || !detail.Execution.Redacted {
		t.Fatalf("hook execution = %#v", detail.Execution)
	}
}

func TestRuntimeHTTPServerRoutesTodoQueriesToRuntimeService(t *testing.T) {
	t.Parallel()

	service := &recordingRuntimeService{
		todos: RuntimeTodosResponse{Summary: RuntimeTodoSummary{
			SessionID: "session-1",
			Todos: []RuntimeTodo{{
				Content: "Inspect plan",
				Status:  "in_progress",
			}},
			InProgress: 1,
			Total:      1,
		}},
	}
	server := newRuntimeHTTPServer(service)

	req, err := http.NewRequest(http.MethodGet, "/v1/sessions/session-1/todos", nil)
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Authorization", "Bearer "+server.Token())
	resp := httptestResponse(server, req)
	if resp.status != http.StatusOK {
		t.Fatalf("session todos status = %d body = %s", resp.status, resp.body.String())
	}
	if service.todoSession != "session-1" {
		t.Fatalf("todo session = %q, want session-1", service.todoSession)
	}

	req, err = http.NewRequest(http.MethodGet, "/v1/turns/turn-1/todos", nil)
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Authorization", "Bearer "+server.Token())
	resp = httptestResponse(server, req)
	if resp.status != http.StatusOK {
		t.Fatalf("turn todos status = %d body = %s", resp.status, resp.body.String())
	}
	if service.todoTurn != "turn-1" {
		t.Fatalf("todo turn = %q, want turn-1", service.todoTurn)
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

func TestRuntimeHTTPServerDevModuleRoutesChatLoop(t *testing.T) {
	t.Parallel()

	service := &recordingRuntimeService{
		status: RuntimeStatus{Ready: true, SessionID: "session-1"},
		turn:   RuntimeTurnResponse{Turn: RuntimeTurn{ID: "turn-1", SessionID: "session-1", Status: "completed"}},
	}
	server := newRuntimeHTTPServer(service)

	tests := []struct {
		name   string
		method string
		path   string
		body   string
		check  func(t *testing.T)
	}{
		{
			name:   "new chat",
			method: http.MethodPost,
			path:   "/v1/sessions",
			body:   `{"title":"Draft"}`,
		},
		{
			name:   "select session",
			method: http.MethodPost,
			path:   "/v1/sessions/session-2/select",
			check: func(t *testing.T) {
				t.Helper()
				if service.selectedSession != "session-2" {
					t.Fatalf("selected session = %q, want session-2", service.selectedSession)
				}
			},
		},
		{
			name:   "session messages",
			method: http.MethodGet,
			path:   "/v1/sessions/session-2/messages",
			check: func(t *testing.T) {
				t.Helper()
				if service.messageSession != "session-2" {
					t.Fatalf("message session = %q, want session-2", service.messageSession)
				}
			},
		},
		{
			name:   "session turn",
			method: http.MethodPost,
			path:   "/v1/sessions/session-2/turns",
			body:   `{"prompt":"hello"}`,
			check: func(t *testing.T) {
				t.Helper()
				if service.chatCalls == 0 {
					t.Fatal("chat was not called")
				}
			},
		},
		{
			name:   "turn",
			method: http.MethodGet,
			path:   "/v1/turns/turn-1",
		},
		{
			name:   "cancel turn",
			method: http.MethodPost,
			path:   "/v1/turns/turn-1/cancel",
			check: func(t *testing.T) {
				t.Helper()
				if service.cancelledTurn != "turn-1" {
					t.Fatalf("cancelled turn = %q, want turn-1", service.cancelledTurn)
				}
			},
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			req, err := http.NewRequest(http.MethodGet, "/v1/dev/module", nil)
			if err != nil {
				t.Fatal(err)
			}
			q := req.URL.Query()
			q.Set("token", server.Token())
			q.Set("method", tt.method)
			q.Set("path", tt.path)
			if tt.body != "" {
				q.Set("body", tt.body)
			}
			req.URL.RawQuery = q.Encode()

			resp := httptestResponse(server, req)
			if resp.status != http.StatusOK {
				t.Fatalf("status = %d body = %s", resp.status, resp.body.String())
			}
			if tt.check != nil {
				tt.check(t)
			}
		})
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

func TestRuntimeHTTPServerRoutesCapabilityRefreshToRuntimeService(t *testing.T) {
	t.Parallel()

	service := &recordingRuntimeService{}
	server := newRuntimeHTTPServer(service)
	req, err := http.NewRequest(http.MethodPost, "/v1/capabilities/skill%3Acrush-config/refresh", nil)
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Authorization", "Bearer "+server.Token())

	resp := httptestResponse(server, req)
	if resp.status != http.StatusOK {
		t.Fatalf("status = %d body = %s", resp.status, resp.body.String())
	}
	if service.refreshedCapability != "skill:crush-config" {
		t.Fatalf("refreshed capability = %q", service.refreshedCapability)
	}
	var response RuntimeCapabilityResponse
	if err := json.Unmarshal(resp.body.Bytes(), &response); err != nil {
		t.Fatal(err)
	}
	if response.Capability.State != "loaded" {
		t.Fatalf("capability = %#v", response.Capability)
	}
}

func TestRuntimeHTTPServerRoutesCapabilityRefreshWithEncodedSlashID(t *testing.T) {
	t.Parallel()

	service := &recordingRuntimeService{}
	server := newRuntimeHTTPServer(service)
	req, err := http.NewRequest(http.MethodPost, "/v1/capabilities/mcp_resource%3Adocs%3Adocs%3A%2F%2Fintro/refresh", nil)
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Authorization", "Bearer "+server.Token())

	resp := httptestResponse(server, req)
	if resp.status != http.StatusOK {
		t.Fatalf("status = %d body = %s", resp.status, resp.body.String())
	}
	if service.refreshedCapability != "mcp_resource:docs:docs://intro" {
		t.Fatalf("refreshed capability = %q", service.refreshedCapability)
	}
}

func TestRuntimeHTTPServerRoutesToolSearchToRuntimeService(t *testing.T) {
	t.Parallel()

	service := &recordingRuntimeService{}
	server := newRuntimeHTTPServer(service)
	req, err := http.NewRequest(http.MethodPost, "/v1/tools/search", strings.NewReader(`{"query":"docs","maxResults":2}`))
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Authorization", "Bearer "+server.Token())

	resp := httptestResponse(server, req)
	if resp.status != http.StatusOK {
		t.Fatalf("status = %d body = %s", resp.status, resp.body.String())
	}
	if service.toolSearchQuery != "docs" {
		t.Fatalf("tool search query = %q", service.toolSearchQuery)
	}
}

func TestRuntimeHTTPServerRoutesAgentTaskMessagingToRuntimeService(t *testing.T) {
	t.Parallel()

	service := &recordingRuntimeService{
		agentRoles: RuntimeAgentRolesResponse{Roles: []RuntimeAgentRoleDefinition{{ID: "task", Name: "task"}}},
		agentTaskMessages: RuntimeAgentTaskMessagesResponse{Messages: []RuntimeAgentTaskMessage{{
			ID: "msg-1", TaskID: "task-1", Direction: taskMessageDirectionChildToParent, Kind: taskMessageKindProgress, Status: taskMessageStatusCreated, CreatedAt: 1,
		}}},
		agentTaskResult: RuntimeAgentTaskResultResponse{Result: RuntimeAgentTaskResult{TaskID: "task-1", Status: agentTaskStatusCompleted, Summary: "done"}},
	}
	server := newRuntimeHTTPServer(service)

	req, err := http.NewRequest(http.MethodGet, "/v1/agent-roles", nil)
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Authorization", "Bearer "+server.Token())
	resp := httptestResponse(server, req)
	if resp.status != http.StatusOK {
		t.Fatalf("roles status = %d body = %s", resp.status, resp.body.String())
	}

	req, err = http.NewRequest(http.MethodGet, "/v1/tasks/task-1/messages", nil)
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Authorization", "Bearer "+server.Token())
	resp = httptestResponse(server, req)
	if resp.status != http.StatusOK || !strings.Contains(resp.body.String(), "msg-1") {
		t.Fatalf("messages status = %d body = %s", resp.status, resp.body.String())
	}

	req, err = http.NewRequest(http.MethodGet, "/v1/tasks/task-1/result", nil)
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Authorization", "Bearer "+server.Token())
	resp = httptestResponse(server, req)
	if resp.status != http.StatusOK || !strings.Contains(resp.body.String(), "done") {
		t.Fatalf("result status = %d body = %s", resp.status, resp.body.String())
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

func TestRuntimeHTTPServerRoutesPluginsToRuntimeService(t *testing.T) {
	t.Parallel()

	service := &recordingRuntimeService{
		plugins: RuntimePluginsResponse{
			Plugins: []RuntimePlugin{{ID: "runtime:skills", Name: "Runtime Skills", Kind: "skills", Category: "Skills", Source: "runtime", Enabled: true, State: "loaded"}},
		},
	}
	server := newRuntimeHTTPServer(service)
	req, err := http.NewRequest(http.MethodGet, "/v1/plugins", nil)
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Authorization", "Bearer "+server.Token())

	resp := httptestResponse(server, req)
	if resp.status != http.StatusOK {
		t.Fatalf("status = %d body = %s", resp.status, resp.body.String())
	}
	var plugins RuntimePluginsResponse
	if err := json.Unmarshal(resp.body.Bytes(), &plugins); err != nil {
		t.Fatal(err)
	}
	if len(plugins.Plugins) != 1 || plugins.Plugins[0].ID != "runtime:skills" {
		t.Fatalf("plugins = %#v", plugins.Plugins)
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
	first := service.storeRuntimeEvent(RuntimeEvent{
		ID:        "event-1",
		Type:      runtimeapi.EventTurnStarted,
		CreatedAt: "2026-05-18T00:00:00Z",
		SessionID: "session-1",
		TurnID:    "turn-1",
	})
	server := newRuntimeHTTPServer(service)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, "/v1/events?after="+strconv.FormatInt(first.Sequence, 10), nil)
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
	second := service.storeRuntimeEvent(RuntimeEvent{
		ID:         "event-2",
		Type:       runtimeapi.EventToolCallStarted,
		CreatedAt:  "2026-05-18T00:00:01Z",
		SessionID:  "session-1",
		TurnID:     "turn-1",
		ToolCallID: "tool-1",
	})

	recorder.waitForLine(t, "event: runtime-event")
	line := recorder.waitForPrefix(t, "data: ")
	var event RuntimeEvent
	if err := json.Unmarshal([]byte(strings.TrimPrefix(line, "data: ")), &event); err != nil {
		t.Fatal(err)
	}
	if event.ID != "event-2" || event.Sequence != second.Sequence || event.Sequence <= first.Sequence {
		t.Fatalf("event sequence = %#v, first = %#v, second = %#v", event, first, second)
	}
	if event.Type != runtimeapi.EventToolCallStarted || event.SessionID != "session-1" || event.TurnID != "turn-1" || event.ToolCallID != "tool-1" {
		t.Fatalf("event = %#v", event)
	}
	cancel()
	<-done
}

func TestRuntimeHTTPServerSSEReplaysHistoryAfterCursorWithMonotonicSequence(t *testing.T) {
	t.Parallel()

	service := newRuntimeService()
	first := service.storeRuntimeEvent(RuntimeEvent{
		Type:      runtimeapi.EventTurnStarted,
		CreatedAt: "2026-05-18T00:00:00Z",
		SessionID: "session-1",
		TurnID:    "turn-1",
	})
	second := service.storeRuntimeEvent(RuntimeEvent{
		Type:       runtimeapi.EventToolCallStarted,
		CreatedAt:  "2026-05-18T00:00:01Z",
		SessionID:  "session-1",
		TurnID:     "turn-1",
		ToolCallID: "tool-1",
	})
	third := service.storeRuntimeEvent(RuntimeEvent{
		Type:       runtimeapi.EventToolCallCompleted,
		CreatedAt:  "2026-05-18T00:00:02Z",
		SessionID:  "session-1",
		TurnID:     "turn-1",
		ToolCallID: "tool-1",
	})
	server := newRuntimeHTTPServer(service)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, "/v1/events?after="+strconv.FormatInt(first.Sequence, 10), nil)
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
	var events []RuntimeEvent
	for len(events) < 2 {
		recorder.waitForLine(t, "event: runtime-event")
		line := recorder.waitForPrefix(t, "data: ")
		var event RuntimeEvent
		if err := json.Unmarshal([]byte(strings.TrimPrefix(line, "data: ")), &event); err != nil {
			t.Fatal(err)
		}
		events = append(events, event)
	}
	cancel()
	<-done

	if events[0].Sequence != second.Sequence || events[1].Sequence != third.Sequence || events[0].Sequence >= events[1].Sequence {
		t.Fatalf("events are not monotonic after cursor: %#v", events)
	}
	for _, event := range events {
		if event.SessionID != "session-1" || event.TurnID != "turn-1" || event.ToolCallID != "tool-1" {
			t.Fatalf("lifecycle event linkage missing: %#v", event)
		}
	}
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

func TestRuntimeHTTPServerRoutesReplayExportToRuntimeService(t *testing.T) {
	t.Parallel()

	service := &recordingRuntimeService{
		replayExport: RuntimeReplayExportResponse{
			TurnID:      "turn-1",
			GeneratedAt: "2026-05-24T00:00:00Z",
			Source:      "test",
			Summary:     RuntimeReplayExportSummary{Redacted: true},
		},
	}
	server := newRuntimeHTTPServer(service)
	req, err := http.NewRequest(http.MethodGet, "/v1/replay/export?turn_id=turn-1&session_id=session-1&after=7", nil)
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Authorization", "Bearer "+server.Token())

	resp := httptestResponse(server, req)
	if resp.status != http.StatusOK {
		t.Fatalf("status = %d body = %s", resp.status, resp.body.String())
	}
	if service.replayExportRequest.TurnID != "turn-1" || service.replayExportRequest.SessionID != "session-1" || service.replayExportRequest.After != 7 {
		t.Fatalf("replay export request = %#v", service.replayExportRequest)
	}
	var replay RuntimeReplayExportResponse
	if err := json.Unmarshal(resp.body.Bytes(), &replay); err != nil {
		t.Fatal(err)
	}
	if replay.TurnID != "turn-1" || !replay.Summary.Redacted {
		t.Fatalf("replay export = %#v", replay)
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
