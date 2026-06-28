package runtime

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/CIPFZ/agent-builder/internal/agent"
	"github.com/CIPFZ/agent-builder/internal/db"
	"github.com/CIPFZ/agent-builder/internal/runtimeapi"
	"github.com/CIPFZ/agent-builder/internal/tools/scheduler"
	"github.com/gorilla/websocket"
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

func TestRuntimeHTTPServerServesClientShellWithoutToken(t *testing.T) {
	distDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(distDir, "index.html"), []byte(`<!doctype html><title>Agent Builder</title><div id="root"></div>`), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(filepath.Join(distDir, "assets"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(distDir, "assets", "app.js"), []byte(`console.log("ok")`), 0o644); err != nil {
		t.Fatal(err)
	}
	t.Setenv("AGENT_BUILDER_CLIENT_DIST", distDir)

	server := newRuntimeHTTPServer(&recordingRuntimeService{})
	req, err := http.NewRequest(http.MethodGet, "/", nil)
	if err != nil {
		t.Fatal(err)
	}
	resp := httptestResponse(server, req)
	if resp.status != http.StatusOK || !strings.Contains(resp.body.String(), "Agent Builder") {
		t.Fatalf("client shell status = %d body = %s", resp.status, resp.body.String())
	}

	req, err = http.NewRequest(http.MethodGet, "/assets/app.js", nil)
	if err != nil {
		t.Fatal(err)
	}
	resp = httptestResponse(server, req)
	if resp.status != http.StatusOK || !strings.Contains(resp.body.String(), `console.log("ok")`) {
		t.Fatalf("client asset status = %d body = %s", resp.status, resp.body.String())
	}

	req, err = http.NewRequest(http.MethodGet, "/v1/runtime/status", nil)
	if err != nil {
		t.Fatal(err)
	}
	resp = httptestResponse(server, req)
	if resp.status != http.StatusUnauthorized {
		t.Fatalf("api status = %d, want unauthorized", resp.status)
	}
}

func TestRuntimeHTTPServerRoutesOpenProjectToRuntimeService(t *testing.T) {
	t.Parallel()

	service := &recordingRuntimeService{
		openProject: RuntimeOpenProjectResponse{
			Project: RuntimeProject{ID: "workspace-1", Name: "repo", Path: "C:\\work\\repo", Current: true},
			Status:  RuntimeStatus{WorkspaceID: "workspace-1", WorkingDir: "C:\\work\\repo", Ready: true},
		},
	}
	server := newRuntimeHTTPServer(service)

	req, err := http.NewRequest(http.MethodPost, "/v1/projects/open", strings.NewReader(`{"path":"C:\\work\\repo","createMissing":true}`))
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Authorization", "Bearer "+server.Token())
	resp := httptestResponse(server, req)
	if resp.status != http.StatusOK {
		t.Fatalf("open project status = %d body = %s", resp.status, resp.body.String())
	}
	if service.openProjectReq.Path != "C:\\work\\repo" || !service.openProjectReq.CreateMissing {
		t.Fatalf("open project request = %#v", service.openProjectReq)
	}
	var opened RuntimeOpenProjectResponse
	if err := json.Unmarshal(resp.body.Bytes(), &opened); err != nil {
		t.Fatal(err)
	}
	if opened.Project.ID != "workspace-1" || opened.Status.WorkingDir != "C:\\work\\repo" {
		t.Fatalf("open project response = %#v", opened)
	}

	req, err = http.NewRequest(http.MethodGet, "/v1/dev/module?token="+server.Token()+"&method=POST&path=/v1/projects/open&body=%7B%22path%22%3A%22C%3A%5C%5Cwork%5C%5Crepo%22%7D", nil)
	if err != nil {
		t.Fatal(err)
	}
	resp = httptestResponse(server, req)
	if resp.status != http.StatusOK {
		t.Fatalf("dev-module open project status = %d body = %s", resp.status, resp.body.String())
	}
	if service.openProjectReq.Path != "C:\\work\\repo" {
		t.Fatalf("dev-module open project request = %#v", service.openProjectReq)
	}
}

func TestRuntimeHTTPServerRoutesCreateProjectToRuntimeService(t *testing.T) {
	t.Parallel()

	service := &recordingRuntimeService{
		openProject: RuntimeOpenProjectResponse{
			Project: RuntimeProject{ID: "workspace-1", Name: "Blank", Path: "C:\\app\\data\\projects\\Blank", Current: true},
			Status:  RuntimeStatus{WorkspaceID: "workspace-1", WorkingDir: "C:\\app\\data\\projects\\Blank", Ready: true},
		},
	}
	server := newRuntimeHTTPServer(service)

	req, err := http.NewRequest(http.MethodPost, "/v1/projects", strings.NewReader(`{"name":"Blank"}`))
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Authorization", "Bearer "+server.Token())
	resp := httptestResponse(server, req)
	if resp.status != http.StatusOK {
		t.Fatalf("create project status = %d body = %s", resp.status, resp.body.String())
	}
	if service.createProjectReq.Name != "Blank" {
		t.Fatalf("create project request = %#v", service.createProjectReq)
	}
	var created RuntimeOpenProjectResponse
	if err := json.Unmarshal(resp.body.Bytes(), &created); err != nil {
		t.Fatal(err)
	}
	if created.Project.Name != "Blank" || created.Status.WorkingDir == "" {
		t.Fatalf("create project response = %#v", created)
	}

	req, err = http.NewRequest(http.MethodGet, "/v1/dev/module?token="+server.Token()+"&method=POST&path=/v1/projects&body=%7B%22name%22%3A%22Blank%22%7D", nil)
	if err != nil {
		t.Fatal(err)
	}
	resp = httptestResponse(server, req)
	if resp.status != http.StatusOK {
		t.Fatalf("dev-module create project status = %d body = %s", resp.status, resp.body.String())
	}
	if service.createProjectReq.Name != "Blank" {
		t.Fatalf("dev-module create project request = %#v", service.createProjectReq)
	}
}

func TestRuntimeHTTPServerRoutesRenameProjectToRuntimeService(t *testing.T) {
	t.Parallel()

	service := &recordingRuntimeService{
		openProject: RuntimeOpenProjectResponse{
			Project: RuntimeProject{ID: "workspace-1", Name: "Renamed", Path: "C:\\app\\data\\projects\\Renamed", Current: true},
			Status:  RuntimeStatus{WorkspaceID: "workspace-1", WorkingDir: "C:\\app\\data\\projects\\Renamed", Ready: true},
		},
	}
	server := newRuntimeHTTPServer(service)

	req, err := http.NewRequest(http.MethodPost, "/v1/projects/workspace-1/rename", strings.NewReader(`{"name":"Renamed"}`))
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Authorization", "Bearer "+server.Token())
	resp := httptestResponse(server, req)
	if resp.status != http.StatusOK {
		t.Fatalf("rename project status = %d body = %s", resp.status, resp.body.String())
	}
	if service.renameProjectReq.ProjectID != "workspace-1" || service.renameProjectReq.Name != "Renamed" {
		t.Fatalf("rename project request = %#v", service.renameProjectReq)
	}
	var renamed RuntimeOpenProjectResponse
	if err := json.Unmarshal(resp.body.Bytes(), &renamed); err != nil {
		t.Fatal(err)
	}
	if renamed.Project.Name != "Renamed" || renamed.Status.WorkingDir == "" {
		t.Fatalf("rename project response = %#v", renamed)
	}

	req, err = http.NewRequest(http.MethodGet, "/v1/dev/module?token="+server.Token()+"&method=POST&path=/v1/projects/workspace-1/rename&body=%7B%22name%22%3A%22Again%22%7D", nil)
	if err != nil {
		t.Fatal(err)
	}
	resp = httptestResponse(server, req)
	if resp.status != http.StatusOK {
		t.Fatalf("dev-module rename project status = %d body = %s", resp.status, resp.body.String())
	}
	if service.renameProjectReq.ProjectID != "workspace-1" || service.renameProjectReq.Name != "Again" {
		t.Fatalf("dev-module rename project request = %#v", service.renameProjectReq)
	}
}

func TestRuntimeHTTPServerRoutesOpenProjectInExplorerToRuntimeService(t *testing.T) {
	t.Parallel()

	service := &recordingRuntimeService{
		openProject: RuntimeOpenProjectResponse{
			Project: RuntimeProject{ID: "workspace-1", Name: "repo", Path: "C:\\work\\repo", Current: true},
			Status:  RuntimeStatus{WorkspaceID: "workspace-1", WorkingDir: "C:\\work\\repo", Ready: true},
		},
	}
	server := newRuntimeHTTPServer(service)

	req, err := http.NewRequest(http.MethodPost, "/v1/projects/workspace-1/open-explorer", nil)
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Authorization", "Bearer "+server.Token())
	resp := httptestResponse(server, req)
	if resp.status != http.StatusOK {
		t.Fatalf("open explorer status = %d body = %s", resp.status, resp.body.String())
	}
	if service.openProjectInExplorerReq.ProjectID != "workspace-1" {
		t.Fatalf("open explorer request = %#v", service.openProjectInExplorerReq)
	}
	var opened RuntimeOpenProjectResponse
	if err := json.Unmarshal(resp.body.Bytes(), &opened); err != nil {
		t.Fatal(err)
	}
	if opened.Project.ID != "workspace-1" || opened.Status.WorkingDir != "C:\\work\\repo" {
		t.Fatalf("open explorer response = %#v", opened)
	}

	req, err = http.NewRequest(http.MethodGet, "/v1/dev/module?token="+server.Token()+"&method=POST&path=/v1/projects/workspace-1/open-explorer", nil)
	if err != nil {
		t.Fatal(err)
	}
	resp = httptestResponse(server, req)
	if resp.status != http.StatusOK {
		t.Fatalf("dev-module open explorer status = %d body = %s", resp.status, resp.body.String())
	}
	if service.openProjectInExplorerReq.ProjectID != "workspace-1" {
		t.Fatalf("dev-module open explorer request = %#v", service.openProjectInExplorerReq)
	}
}

func TestRuntimeHTTPServerRoutesRemoveProjectToRuntimeService(t *testing.T) {
	t.Parallel()

	service := &recordingRuntimeService{
		openProject: RuntimeOpenProjectResponse{
			Project: RuntimeProject{ID: "workspace-next", Name: "next", Path: "C:\\work\\next", Current: true},
			Status:  RuntimeStatus{WorkspaceID: "workspace-next", WorkingDir: "C:\\work\\next", Ready: true},
		},
	}
	server := newRuntimeHTTPServer(service)

	req, err := http.NewRequest(http.MethodDelete, "/v1/projects/workspace-1", nil)
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Authorization", "Bearer "+server.Token())
	resp := httptestResponse(server, req)
	if resp.status != http.StatusOK {
		t.Fatalf("remove project status = %d body = %s", resp.status, resp.body.String())
	}
	if service.removeProjectReq.ProjectID != "workspace-1" {
		t.Fatalf("remove project request = %#v", service.removeProjectReq)
	}
	var removed RuntimeOpenProjectResponse
	if err := json.Unmarshal(resp.body.Bytes(), &removed); err != nil {
		t.Fatal(err)
	}
	if removed.Project.ID != "workspace-next" || removed.Status.WorkingDir != "C:\\work\\next" {
		t.Fatalf("remove project response = %#v", removed)
	}

	req, err = http.NewRequest(http.MethodGet, "/v1/dev/module?token="+server.Token()+"&method=DELETE&path=/v1/projects/workspace-1", nil)
	if err != nil {
		t.Fatal(err)
	}
	resp = httptestResponse(server, req)
	if resp.status != http.StatusOK {
		t.Fatalf("dev-module remove project status = %d body = %s", resp.status, resp.body.String())
	}
	if service.removeProjectReq.ProjectID != "workspace-1" {
		t.Fatalf("dev-module remove project request = %#v", service.removeProjectReq)
	}
}

func TestRuntimeHTTPServerRoutesPermissionDecisionActionMetadata(t *testing.T) {
	t.Parallel()

	service := &recordingRuntimeService{
		status: RuntimeStatus{Ready: true, WorkspaceID: "workspace-1", SessionID: "session-1"},
	}
	server := newRuntimeHTTPServer(service)

	req, err := http.NewRequest(http.MethodPost, "/v1/permissions/perm-1/decision", strings.NewReader(`{"action":"deny"}`))
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Authorization", "Bearer "+server.Token())
	resp := httptestResponse(server, req)
	if resp.status != http.StatusOK {
		t.Fatalf("decision status = %d body = %s", resp.status, resp.body.String())
	}
	if service.permissionDecision.PermissionID != "perm-1" || service.permissionDecision.Action != "deny" {
		t.Fatalf("permission decision = %#v", service.permissionDecision)
	}
	var decision RuntimeStatus
	if err := json.Unmarshal(resp.body.Bytes(), &decision); err != nil {
		t.Fatal(err)
	}
	if decision.Action == nil || !decision.Action.Accepted || decision.Action.Source.Kind != runtimePermissionDecisionActionSourceKind || decision.Action.Source.Action != "deny" || decision.Action.Source.IdempotentBy != "permission_id" {
		t.Fatalf("decision action metadata = %#v", decision.Action)
	}

	req, err = http.NewRequest(http.MethodGet, "/v1/runtime/status", nil)
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Authorization", "Bearer "+server.Token())
	resp = httptestResponse(server, req)
	if resp.status != http.StatusOK {
		t.Fatalf("plain status = %d body = %s", resp.status, resp.body.String())
	}
	var status RuntimeStatus
	if err := json.Unmarshal(resp.body.Bytes(), &status); err != nil {
		t.Fatal(err)
	}
	if status.Action != nil {
		t.Fatalf("plain status should not carry decision action metadata: %#v", status.Action)
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
	var mcpDecision RuntimeMCPRequestResponse
	if err := json.Unmarshal(resp.body.Bytes(), &mcpDecision); err != nil {
		t.Fatal(err)
	}
	if mcpDecision.Action == nil || !mcpDecision.Action.Accepted || mcpDecision.Action.Source.Action != "approve" || mcpDecision.Action.Source.IdempotentBy != "mcp_request_id" {
		t.Fatalf("mcp decision action metadata = %#v", mcpDecision.Action)
	}
	if strings.Contains(strings.ToLower(resp.body.String()), "approved") {
		t.Fatalf("mcp decision response copied response summary into transport payload: %s", resp.body.String())
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

func TestRuntimeHTTPServerRoutesReactCallchainToRuntimeService(t *testing.T) {
	t.Parallel()

	service := &recordingRuntimeService{
		reactCallchain: RuntimeReactCallchainResponse{
			SessionID: "session-1",
			TurnID:    "turn-1",
			Nodes: []RuntimeReactCallNode{{
				ID:        "node-1",
				Kind:      reactNodeAssistantFinal,
				SessionID: "session-1",
				TurnID:    "turn-1",
				Sequence:  1,
			}},
			Summary: RuntimeReactCallSummary{
				StopReason:                 "model_stop",
				StopReasonMessage:          "Tool result delivered; final response is empty.",
				DeliveredToolResultCount:   1,
				UndeliveredToolResultCount: 0,
				ToolResultDeliveries: []RuntimeToolResultDelivery{{
					ToolCallID:          "tool-1",
					ToolResultMessageID: "msg-tool",
					DeliveredToModel:    true,
					DeliveredAtStep:     2,
					Reason:              "included_in_model_input",
				}},
			},
			ToolResultDeliveries: []RuntimeToolResultDelivery{{
				ToolCallID:          "tool-1",
				ToolResultMessageID: "msg-tool",
				DeliveredToModel:    true,
				DeliveredAtStep:     2,
				Reason:              "included_in_model_input",
			}},
			Source: RuntimeReactCallSource{SessionActivityParity: true, EventsAreRefreshOnly: true},
		},
		sessionReactCallchain: RuntimeReactCallchainResponse{
			SessionID: "session-1",
			Nodes: []RuntimeReactCallNode{{
				ID:        "node-session",
				Kind:      reactNodeUserInput,
				SessionID: "session-1",
				Sequence:  1,
			}},
			Source: RuntimeReactCallSource{SessionActivityParity: true, EventsAreRefreshOnly: true},
		},
	}
	server := newRuntimeHTTPServer(service)

	req, err := http.NewRequest(http.MethodGet, "/v1/turns/turn-1/react-callchain", nil)
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Authorization", "Bearer "+server.Token())
	resp := httptestResponse(server, req)
	if resp.status != http.StatusOK {
		t.Fatalf("turn status = %d body = %s", resp.status, resp.body.String())
	}
	if service.reactCallchainTurnID != "turn-1" {
		t.Fatalf("turn id = %q", service.reactCallchainTurnID)
	}
	var turnCallchain RuntimeReactCallchainResponse
	if err := json.Unmarshal(resp.body.Bytes(), &turnCallchain); err != nil {
		t.Fatal(err)
	}
	if turnCallchain.TurnID != "turn-1" || len(turnCallchain.Nodes) != 1 || !turnCallchain.Source.EventsAreRefreshOnly {
		t.Fatalf("turn callchain = %#v", turnCallchain)
	}
	if turnCallchain.Summary.StopReasonMessage != "Tool result delivered; final response is empty." || len(turnCallchain.ToolResultDeliveries) != 1 || !turnCallchain.ToolResultDeliveries[0].DeliveredToModel {
		t.Fatalf("turn delivery = %#v summary=%#v", turnCallchain.ToolResultDeliveries, turnCallchain.Summary)
	}

	req, err = http.NewRequest(http.MethodGet, "/v1/sessions/session-1/react-callchain?limit=3", nil)
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Authorization", "Bearer "+server.Token())
	resp = httptestResponse(server, req)
	if resp.status != http.StatusOK {
		t.Fatalf("session status = %d body = %s", resp.status, resp.body.String())
	}
	if service.sessionReactCallchainID != "session-1" || service.sessionReactCallchainLimit != 3 {
		t.Fatalf("session args = %q %d", service.sessionReactCallchainID, service.sessionReactCallchainLimit)
	}
	var sessionCallchain RuntimeReactCallchainResponse
	if err := json.Unmarshal(resp.body.Bytes(), &sessionCallchain); err != nil {
		t.Fatal(err)
	}
	if sessionCallchain.SessionID != "session-1" || len(sessionCallchain.Nodes) != 1 {
		t.Fatalf("session callchain = %#v", sessionCallchain)
	}
}

func TestRuntimeHTTPServerRoutesPromptAssembliesToRuntimeService(t *testing.T) {
	t.Parallel()

	service := &recordingRuntimeService{
		promptAssemblies: RuntimePromptAssembliesResponse{Assemblies: []RuntimePromptAssembly{{
			ID:        "assembly-1",
			SessionID: "session-1",
			TurnID:    "turn-1",
			Step:      2,
			Provider:  "openai",
			Model:     "test-model",
			System: RuntimePromptSystemSummary{
				Source:        "runtime",
				Hash:          "sha256:system",
				PromptPrefix:  true,
				SourceRefs:    []string{"system:runtime"},
				Redacted:      true,
				TokenEstimate: 24,
			},
			Messages: RuntimePromptMessageSummary{
				Count:           3,
				ByRole:          map[string]int{"user": 1, "assistant": 1, "tool": 1},
				ToolResultCount: 1,
				RawPromptStored: false,
			},
			Tools: RuntimePromptToolSummary{
				Selected:      []string{"bash"},
				Omitted:       []string{"webfetch"},
				SelectedCount: 1,
				OmittedCount:  1,
			},
			Skills: RuntimePromptSkillSummary{
				LoadedCount:      1,
				LoadedNames:      []string{"agent-builder-config"},
				XMLPresent:       true,
				XMLHash:          "sha256:skills",
				RawContentStored: false,
			},
			MCP: RuntimePromptMCPSummary{
				ServerCount:      1,
				InstructionCount: 1,
				Servers:          []string{"docs"},
				InstructionHash:  "sha256:mcp",
				RawContentStored: false,
			},
			ContextSources: []RuntimeContextSource{{
				ID:            "ctx-1",
				Kind:          "project_memory",
				Name:          "AGENTS.md",
				Enabled:       true,
				State:         "loaded",
				ContentHash:   "sha256:context",
				TokenEstimate: 32,
			}},
			Compact: []RuntimeCompactBoundary{{
				ID:         "compact-1",
				SessionID:  "session-1",
				TurnID:     "turn-1",
				Kind:       "microcompact",
				Trigger:    "budget",
				Status:     "completed",
				SummaryRef: "ref-compact",
			}},
			Budget: RuntimeBudgetReport{
				ContextSources:       RuntimeBudgetBucket{Count: 1, EstimatedTokens: 32},
				SelectedToolSchemas:  RuntimeBudgetBucket{Count: 1, EstimatedTokens: 8},
				TotalEstimatedTokens: 64,
			},
			CreatedAt: 1234,
		}}},
	}
	server := newRuntimeHTTPServer(service)

	req, err := http.NewRequest(http.MethodGet, "/v1/turns/turn-1/prompt-assemblies", nil)
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Authorization", "Bearer "+server.Token())
	resp := httptestResponse(server, req)
	if resp.status != http.StatusOK {
		t.Fatalf("turn assemblies status = %d body = %s", resp.status, resp.body.String())
	}
	if service.promptAssembliesTurnID != "turn-1" || service.turn.Turn.ID != "" {
		t.Fatalf("turn prompt route args turn=%q genericTurn=%q", service.promptAssembliesTurnID, service.turn.Turn.ID)
	}
	var turnAssemblies RuntimePromptAssembliesResponse
	if err := json.Unmarshal(resp.body.Bytes(), &turnAssemblies); err != nil {
		t.Fatal(err)
	}
	if len(turnAssemblies.Assemblies) != 1 || turnAssemblies.Assemblies[0].System.Hash == "" || turnAssemblies.Assemblies[0].Messages.RawPromptStored {
		t.Fatalf("turn assemblies = %#v", turnAssemblies)
	}
	if strings.Contains(resp.body.String(), "full prompt") || strings.Contains(resp.body.String(), "secret") {
		t.Fatalf("prompt assembly response leaked raw content: %s", resp.body.String())
	}

	req, err = http.NewRequest(http.MethodGet, "/v1/sessions/session-1/prompt-assemblies?limit=5", nil)
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Authorization", "Bearer "+server.Token())
	resp = httptestResponse(server, req)
	if resp.status != http.StatusOK {
		t.Fatalf("session assemblies status = %d body = %s", resp.status, resp.body.String())
	}
	if service.promptAssembliesSessionID != "session-1" || service.promptAssembliesLimit != 5 {
		t.Fatalf("session prompt route args session=%q limit=%d", service.promptAssembliesSessionID, service.promptAssembliesLimit)
	}

	req, err = http.NewRequest(http.MethodGet, "/v1/dev/module?token="+server.Token()+"&path=/v1/turns/turn-2/prompt-assemblies", nil)
	if err != nil {
		t.Fatal(err)
	}
	resp = httptestResponse(server, req)
	if resp.status != http.StatusOK || service.promptAssembliesTurnID != "turn-2" {
		t.Fatalf("dev turn assemblies status = %d body = %s turn=%q", resp.status, resp.body.String(), service.promptAssembliesTurnID)
	}

	req, err = http.NewRequest(http.MethodGet, "/v1/dev/module?token="+server.Token()+"&path=/v1/sessions/session-2/prompt-assemblies&limit=7", nil)
	if err != nil {
		t.Fatal(err)
	}
	resp = httptestResponse(server, req)
	if resp.status != http.StatusOK || service.promptAssembliesSessionID != "session-2" || service.promptAssembliesLimit != 7 {
		t.Fatalf("dev session assemblies status = %d body = %s session=%q limit=%d", resp.status, resp.body.String(), service.promptAssembliesSessionID, service.promptAssembliesLimit)
	}
}

func TestRuntimeHTTPServerRoutesUserInputToRuntimeService(t *testing.T) {
	t.Parallel()

	service := &recordingRuntimeService{
		userInputResponse: RuntimeChatResponse{
			RequestID: "turn-1",
			TurnID:    "turn-1",
			Status:    RuntimeStatus{SessionID: "session-1"},
			NormalizedInput: &RuntimeNormalizedInput{
				ID:          "input-1",
				SessionID:   "session-1",
				Mode:        runtimeInputModePrompt,
				Prompt:      "hello",
				ShouldQuery: true,
			},
		},
		userInput: RuntimeNormalizedInput{
			ID:        "input-1",
			SessionID: "session-1",
			Mode:      runtimeInputModePrompt,
			Prompt:    "hello",
		},
	}
	server := newRuntimeHTTPServer(service)

	req, err := http.NewRequest(http.MethodPost, "/v1/user-inputs", strings.NewReader(`{
  "sessionId":"session-1",
  "mode":"prompt",
  "items":[{"type":"text","text":"hello"}],
  "options":{"clientRequestId":"client-1"}
}`))
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Authorization", "Bearer "+server.Token())
	resp := httptestResponse(server, req)
	if resp.status != http.StatusOK {
		t.Fatalf("submit status = %d body = %s", resp.status, resp.body.String())
	}
	if service.userInputReq.SessionID != "session-1" || service.userInputReq.Mode != "prompt" || len(service.userInputReq.Items) != 1 || service.userInputReq.Options.ClientRequestID != "client-1" {
		t.Fatalf("submit request = %#v", service.userInputReq)
	}
	var submitted RuntimeChatResponse
	if err := json.Unmarshal(resp.body.Bytes(), &submitted); err != nil {
		t.Fatal(err)
	}
	if submitted.NormalizedInput == nil || submitted.NormalizedInput.ID != "input-1" {
		t.Fatalf("submitted = %#v", submitted)
	}

	req, err = http.NewRequest(http.MethodGet, "/v1/user-inputs/input-1", nil)
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Authorization", "Bearer "+server.Token())
	resp = httptestResponse(server, req)
	if resp.status != http.StatusOK {
		t.Fatalf("get status = %d body = %s", resp.status, resp.body.String())
	}
	if service.userInputID != "input-1" {
		t.Fatalf("user input id = %q", service.userInputID)
	}

	body := url.QueryEscape(`{"mode":"slash","items":[{"type":"text","text":"/status"}]}`)
	req, err = http.NewRequest(http.MethodGet, "/v1/dev/module?token="+server.Token()+"&method=POST&path=/v1/user-inputs&body="+body, nil)
	if err != nil {
		t.Fatal(err)
	}
	resp = httptestResponse(server, req)
	if resp.status != http.StatusOK {
		t.Fatalf("dev submit status = %d body = %s", resp.status, resp.body.String())
	}
	if service.userInputReq.Mode != "slash" {
		t.Fatalf("dev submit request = %#v", service.userInputReq)
	}
}

func TestRuntimeHTTPServerDevModuleRoutesReactCallchain(t *testing.T) {
	t.Parallel()

	service := &recordingRuntimeService{
		reactCallchain: RuntimeReactCallchainResponse{
			SessionID: "session-1",
			TurnID:    "turn-1",
			Summary: RuntimeReactCallSummary{
				StopReason:        "model_stop",
				StopReasonMessage: "Tool result delivered; final response is empty.",
			},
			ToolResultDeliveries: []RuntimeToolResultDelivery{{ToolCallID: "tool-1", DeliveredToModel: true}},
		},
		sessionReactCallchain: RuntimeReactCallchainResponse{SessionID: "session-1"},
	}
	server := newRuntimeHTTPServer(service)

	req, err := http.NewRequest(http.MethodGet, "/v1/dev/module?method=GET&path=%2Fv1%2Fturns%2Fturn-1%2Freact-callchain&token="+server.Token(), nil)
	if err != nil {
		t.Fatal(err)
	}
	resp := httptestResponse(server, req)
	if resp.status != http.StatusOK {
		t.Fatalf("turn module status = %d body = %s", resp.status, resp.body.String())
	}
	if service.reactCallchainTurnID != "turn-1" {
		t.Fatalf("turn id = %q", service.reactCallchainTurnID)
	}
	body := resp.body.String()
	if !strings.Contains(body, "Tool result delivered; final response is empty.") || !strings.Contains(body, "deliveredToModel") {
		t.Fatalf("turn module body = %s", body)
	}

	req, err = http.NewRequest(http.MethodGet, "/v1/dev/module?method=GET&path=%2Fv1%2Fsessions%2Fsession-1%2Freact-callchain%3Flimit%3D4&token="+server.Token(), nil)
	if err != nil {
		t.Fatal(err)
	}
	resp = httptestResponse(server, req)
	if resp.status != http.StatusOK {
		t.Fatalf("session module status = %d body = %s", resp.status, resp.body.String())
	}
	if service.sessionReactCallchainID != "session-1" || service.sessionReactCallchainLimit != 4 {
		t.Fatalf("session args = %q %d", service.sessionReactCallchainID, service.sessionReactCallchainLimit)
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

func TestRuntimeHTTPServerRoutesSessionOutputToRuntimeService(t *testing.T) {
	t.Parallel()

	service := &recordingRuntimeService{
		output: RuntimeOutputSnapshot{
			SessionID: "session-1",
			Cursor:    "3",
			Messages:  []RuntimeMessage{{ID: "msg-1", SessionID: "session-1", Role: "user", ClientRequestID: "client-1"}},
		},
		outputEvents: RuntimeOutputEventsResponse{
			SessionID: "session-1",
			Cursor:    "4",
			Events:    []RuntimeOutputEvent{{ID: "out-1", Sequence: 401, SessionID: "session-1", Kind: "message.created", EntityID: "msg-1", Operation: "append"}},
		},
	}
	server := newRuntimeHTTPServer(service)

	req, err := http.NewRequest(http.MethodGet, "/v1/sessions/session-1/output?limit=4&cursor=2", nil)
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Authorization", "Bearer "+server.Token())
	resp := httptestResponse(server, req)
	if resp.status != http.StatusOK {
		t.Fatalf("status = %d body = %s", resp.status, resp.body.String())
	}
	if service.outputSession != "session-1" || service.outputRequest.Limit != 4 || service.outputRequest.Cursor != "2" || !service.outputRequest.Snapshot {
		t.Fatalf("output request = session:%q req:%#v", service.outputSession, service.outputRequest)
	}
	var snapshot RuntimeOutputSnapshot
	if err := json.Unmarshal(resp.body.Bytes(), &snapshot); err != nil {
		t.Fatal(err)
	}
	if snapshot.Cursor != "3" || snapshot.Messages[0].ClientRequestID != "client-1" {
		t.Fatalf("snapshot = %#v", snapshot)
	}

	req, err = http.NewRequest(http.MethodGet, "/v1/sessions/session-1/output/events?cursor=3", nil)
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Authorization", "Bearer "+server.Token())
	resp = httptestResponse(server, req)
	if resp.status != http.StatusOK {
		t.Fatalf("events status = %d body = %s", resp.status, resp.body.String())
	}
	if service.outputEventsSession != "session-1" || service.outputEventsAfter != "3" {
		t.Fatalf("output events request = session:%q after:%q", service.outputEventsSession, service.outputEventsAfter)
	}
	var events RuntimeOutputEventsResponse
	if err := json.Unmarshal(resp.body.Bytes(), &events); err != nil {
		t.Fatal(err)
	}
	if events.Cursor != "4" || len(events.Events) != 1 {
		t.Fatalf("events = %#v", events)
	}
}

func TestRuntimeHTTPServerRoutesRunTransitionHistoryToRuntimeService(t *testing.T) {
	t.Parallel()

	service := &recordingRuntimeService{
		transitionHistory: RuntimeRunTransitionHistoryResponse{
			Transitions: []RuntimeRunTransition{{
				ID:        "transition-1",
				RunID:     "run-1",
				SessionID: "session-1",
				TurnID:    "turn-1",
				ToStatus:  runtimeRunStatusCompleted,
				Source:    runtimeRunTransitionSourceTurnFinished,
				CreatedAt: 2000,
			}},
			Window: RuntimeActivityWindow{Limit: 4, ToEnd: true},
			Source: RuntimeRunTransitionHistorySource{
				Kind:      runtimeRunTransitionHistorySourceKind,
				ReadOnly:  true,
				AuditOnly: true,
			},
		},
	}
	server := newRuntimeHTTPServer(service)

	req, err := http.NewRequest(http.MethodGet, "/v1/run-transitions?run_id=run-1&limit=4&cursor=v1%3Atransition", nil)
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Authorization", "Bearer "+server.Token())
	resp := httptestResponse(server, req)
	if resp.status != http.StatusOK {
		t.Fatalf("status = %d body = %s", resp.status, resp.body.String())
	}
	if service.transitionHistoryReq.RunID != "run-1" || service.transitionHistoryReq.Limit != 4 || service.transitionHistoryReq.Cursor != "v1:transition" {
		t.Fatalf("run transition args = %#v", service.transitionHistoryReq)
	}
	var history RuntimeRunTransitionHistoryResponse
	if err := json.Unmarshal(resp.body.Bytes(), &history); err != nil {
		t.Fatal(err)
	}
	if len(history.Transitions) != 1 || !history.Source.ReadOnly || !history.Source.AuditOnly {
		t.Fatalf("history = %#v", history)
	}

	req, err = http.NewRequest(http.MethodGet, "/v1/run-transitions?session_id=session-1", nil)
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Authorization", "Bearer "+server.Token())
	resp = httptestResponse(server, req)
	if resp.status != http.StatusOK || service.transitionHistoryReq.SessionID != "session-1" {
		t.Fatalf("session route status = %d body = %s args=%#v", resp.status, resp.body.String(), service.transitionHistoryReq)
	}

	req, err = http.NewRequest(http.MethodGet, "/v1/run-transitions?turn_id=turn-1", nil)
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Authorization", "Bearer "+server.Token())
	resp = httptestResponse(server, req)
	if resp.status != http.StatusOK || service.transitionHistoryReq.TurnID != "turn-1" {
		t.Fatalf("turn route status = %d body = %s args=%#v", resp.status, resp.body.String(), service.transitionHistoryReq)
	}
}

func TestRuntimeHTTPServerRoutesRunSchedulerPlanToRuntimeService(t *testing.T) {
	t.Parallel()

	service := &recordingRuntimeService{
		runSchedulerPlan: RuntimeRunSchedulerPlanResponse{
			Plan: RuntimeRunSchedulerPlan{
				RunID:            "run-1",
				PrimarySessionID: "session-1",
				Items: []RuntimeRunSchedulerPlanItem{{
					ID:                "task:task-1",
					Kind:              runtimeRunSchedulerPlanModeTaskTurn,
					SessionID:         "session-1",
					TurnID:            "turn-1",
					TaskID:            "task-1",
					CanSchedule:       true,
					OwnershipVerified: true,
					RequiredPreflight: true,
				}},
			},
			Source: RuntimeRunSchedulerPlanSource{
				Kind:                  runtimeRunSchedulerPlanSourceKind,
				ReadOnly:              true,
				StartsWorker:          false,
				SessionActivityParity: true,
			},
		},
	}
	server := newRuntimeHTTPServer(service)

	req, err := http.NewRequest(http.MethodGet, "/v1/run-scheduler-plan?run_id=run-1&session_id=session-1&mode=task_turn&turn_id=turn-1&checkpoint_id=checkpoint-1&task_id=task-1&limit=3&cursor=v1%3Aplan", nil)
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Authorization", "Bearer "+server.Token())
	resp := httptestResponse(server, req)
	if resp.status != http.StatusOK {
		t.Fatalf("status = %d body = %s", resp.status, resp.body.String())
	}
	if service.runSchedulerPlanReq.RunID != "run-1" || service.runSchedulerPlanReq.SessionID != "session-1" || service.runSchedulerPlanReq.Mode != "task_turn" || service.runSchedulerPlanReq.TurnID != "turn-1" || service.runSchedulerPlanReq.CheckpointID != "checkpoint-1" || service.runSchedulerPlanReq.TaskID != "task-1" || service.runSchedulerPlanReq.Limit != 3 || service.runSchedulerPlanReq.Cursor != "v1:plan" {
		t.Fatalf("scheduler plan args = %#v", service.runSchedulerPlanReq)
	}
	if service.chatCalls != 0 || service.cancelledTask != "" {
		t.Fatalf("scheduler plan route caused side effects: chatCalls=%d cancelledTask=%q", service.chatCalls, service.cancelledTask)
	}
	var plan RuntimeRunSchedulerPlanResponse
	if err := json.Unmarshal(resp.body.Bytes(), &plan); err != nil {
		t.Fatal(err)
	}
	if len(plan.Plan.Items) != 1 || !plan.Source.ReadOnly || plan.Source.StartsWorker || !plan.Source.SessionActivityParity {
		t.Fatalf("scheduler plan = %#v", plan)
	}
}

func TestRuntimeHTTPServerRoutesRunSchedulerExecuteTaskToRuntimeService(t *testing.T) {
	t.Parallel()

	service := &recordingRuntimeService{
		executeRunTask: RuntimeRunSchedulerExecuteTaskResponse{
			Accepted:         true,
			ExecutionStarted: true,
			Reason:           runtimeRunSchedulerExecuteTaskReasonForegroundExecutionStarted,
			Task:             RuntimeAgentTask{ID: "task-1", Status: agentTaskStatusRunning},
			RefreshTargets:   runtimeRunSchedulerRefreshTargets(),
			Source: RuntimeRunSchedulerExecuteTaskSource{
				Kind:                  runtimeRunSchedulerExecuteTaskSourceKind,
				Action:                runtimeRunSchedulerExecuteTaskAction,
				WorkbenchOnly:         true,
				StartsWorker:          false,
				IdempotentByTaskID:    true,
				SessionActivityParity: true,
			},
		},
	}
	server := newRuntimeHTTPServer(service)

	req, err := http.NewRequest(http.MethodPost, "/v1/runs/run-1/tasks/task-1/execute", nil)
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Authorization", "Bearer "+server.Token())
	resp := httptestResponse(server, req)
	if resp.status != http.StatusOK {
		t.Fatalf("status = %d body = %s", resp.status, resp.body.String())
	}
	if service.executeRunID != "run-1" || service.executeTaskID != "task-1" {
		t.Fatalf("execute args = %q/%q", service.executeRunID, service.executeTaskID)
	}
	if service.chatCalls != 0 || service.cancelledTask != "" {
		t.Fatalf("execute route caused side effects: chatCalls=%d cancelledTask=%q", service.chatCalls, service.cancelledTask)
	}
	var execute RuntimeRunSchedulerExecuteTaskResponse
	if err := json.Unmarshal(resp.body.Bytes(), &execute); err != nil {
		t.Fatal(err)
	}
	if !execute.Accepted || !execute.ExecutionStarted || execute.Source.StartsWorker || !execute.Source.WorkbenchOnly || !execute.Source.IdempotentByTaskID || !execute.Source.SessionActivityParity {
		t.Fatalf("execute response = %#v", execute)
	}
	if execute.Action == nil || !execute.Action.Accepted || execute.Action.Source.Action != runtimeRunSchedulerExecuteTaskAction || execute.Action.Source.IdempotentBy != "task_id" {
		t.Fatalf("execute action metadata = %#v", execute.Action)
	}

	req, err = http.NewRequest(http.MethodGet, "/v1/dev/module?token="+server.Token()+"&method=POST&path=/v1/runs/run-2/tasks/task-2/execute", nil)
	if err != nil {
		t.Fatal(err)
	}
	resp = httptestResponse(server, req)
	if resp.status != http.StatusOK {
		t.Fatalf("dev-module status = %d body = %s", resp.status, resp.body.String())
	}
	if service.executeRunID != "run-2" || service.executeTaskID != "task-2" {
		t.Fatalf("dev-module execute args = %q/%q", service.executeRunID, service.executeTaskID)
	}
}

func TestRuntimeHTTPServerRoutesTerminalLifecycleToRuntimeService(t *testing.T) {
	t.Parallel()

	service := &recordingRuntimeService{
		terminalResponse: RuntimeTerminalResponse{Terminal: RuntimeTerminal{
			ID:        "term-1",
			SessionID: "session-1",
			CWD:       "C:\\work",
			Shell:     "PowerShell",
			Columns:   100,
			Rows:      24,
			Status:    "running",
		}},
		sessionTerminals: RuntimeSessionTerminalsResponse{
			SessionID: "session-1",
			Terminals: []RuntimeTerminal{{ID: "term-1", SessionID: "session-1"}},
		},
	}
	server := newRuntimeHTTPServer(service)

	req, err := http.NewRequest(http.MethodPost, "/v1/terminals", strings.NewReader(`{"sessionId":"session-1","id":"term-1","cwd":"C:\\work"}`))
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Authorization", "Bearer "+server.Token())
	resp := httptestResponse(server, req)
	if resp.status != http.StatusOK {
		t.Fatalf("create status = %d body = %s", resp.status, resp.body.String())
	}
	if service.createdTerminal.SessionID != "session-1" || service.createdTerminal.ID != "term-1" || service.createdTerminal.CWD != "C:\\work" {
		t.Fatalf("created terminal = %#v", service.createdTerminal)
	}

	req, err = http.NewRequest(http.MethodGet, "/v1/sessions/session-1/terminals", nil)
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Authorization", "Bearer "+server.Token())
	resp = httptestResponse(server, req)
	if resp.status != http.StatusOK {
		t.Fatalf("session terminals status = %d body = %s", resp.status, resp.body.String())
	}
	if service.sessionTerminalsID != "session-1" {
		t.Fatalf("session terminals id = %q", service.sessionTerminalsID)
	}
	var list RuntimeSessionTerminalsResponse
	if err := json.Unmarshal(resp.body.Bytes(), &list); err != nil {
		t.Fatal(err)
	}
	if list.SessionID != "session-1" || len(list.Terminals) != 1 || list.Terminals[0].SessionID != "session-1" {
		t.Fatalf("session terminals response = %#v", list)
	}

	req, err = http.NewRequest(http.MethodGet, "/v1/dev/module?token="+server.Token()+"&path=/v1/sessions/session-1/terminals", nil)
	if err != nil {
		t.Fatal(err)
	}
	resp = httptestResponse(server, req)
	if resp.status != http.StatusOK {
		t.Fatalf("dev-module session terminals status = %d body = %s", resp.status, resp.body.String())
	}

	for _, legacyPath := range []string{
		"/v1/terminals/term-1/input",
		"/v1/terminals/term-1/resize",
		"/v1/terminals/term-1/execute",
		"/v1/terminals/term-1/events",
	} {
		req, err = http.NewRequest(http.MethodPost, legacyPath, strings.NewReader(`{}`))
		if err != nil {
			t.Fatal(err)
		}
		req.Header.Set("Authorization", "Bearer "+server.Token())
		resp = httptestResponse(server, req)
		if resp.status != http.StatusNotFound {
			t.Fatalf("legacy terminal path %s status = %d body = %s", legacyPath, resp.status, resp.body.String())
		}
	}

	req, err = http.NewRequest(http.MethodDelete, "/v1/terminals/term-1", nil)
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Authorization", "Bearer "+server.Token())
	resp = httptestResponse(server, req)
	if resp.status != http.StatusOK {
		t.Fatalf("delete status = %d body = %s", resp.status, resp.body.String())
	}
	if service.deletedTerminalID != "term-1" {
		t.Fatalf("deleted terminal = %q", service.deletedTerminalID)
	}
}

func TestRuntimeHTTPServerTerminalWebSocketRoutesInputAndResize(t *testing.T) {
	t.Parallel()

	inputSeen := make(chan RuntimeTerminalInputRequest, 1)
	resizeSeen := make(chan RuntimeTerminalResizeRequest, 1)
	service := &recordingRuntimeService{
		terminalInputSeen:  inputSeen,
		terminalResizeSeen: resizeSeen,
	}
	runtimeServer := newRuntimeHTTPServer(service)
	httpServer := httptest.NewServer(runtimeServer)
	defer httpServer.Close()

	wsURL := "ws" + strings.TrimPrefix(httpServer.URL, "http") + "/v1/terminals/term-1/stream?token=" + runtimeServer.Token()
	conn, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()

	if err := conn.WriteJSON(RuntimeTerminalStreamRequest{Type: "input", Data: "pwd\r"}); err != nil {
		t.Fatal(err)
	}
	select {
	case req := <-inputSeen:
		if req.Data != "pwd\r" {
			t.Fatalf("input request = %#v", req)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for terminal websocket input")
	}

	if err := conn.WriteJSON(RuntimeTerminalStreamRequest{Type: "resize", Columns: 120, Rows: 32}); err != nil {
		t.Fatal(err)
	}
	select {
	case req := <-resizeSeen:
		if req.Columns != 120 || req.Rows != 32 {
			t.Fatalf("resize request = %#v", req)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for terminal websocket resize")
	}
}

func TestRuntimeHTTPServerTerminalWebSocketWaitsForAckBeforeNextOutput(t *testing.T) {
	t.Parallel()

	events := make(chan RuntimeTerminalEvent, 4)
	service := &recordingRuntimeService{terminalEvents: events}
	runtimeServer := newRuntimeHTTPServer(service)
	httpServer := httptest.NewServer(runtimeServer)
	defer httpServer.Close()

	wsURL := "ws" + strings.TrimPrefix(httpServer.URL, "http") + "/v1/terminals/term-1/stream?token=" + runtimeServer.Token()
	conn, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()

	events <- RuntimeTerminalEvent{TerminalID: "term-1", Sequence: 1, Data: "first"}
	var first RuntimeTerminalStreamMessage
	if err := conn.ReadJSON(&first); err != nil {
		t.Fatal(err)
	}
	if first.Type != "output" || len(first.Events) != 1 || first.Events[0].Sequence != 1 || first.Events[0].Data != "first" {
		t.Fatalf("first websocket output = %#v", first)
	}

	events <- RuntimeTerminalEvent{TerminalID: "term-1", Sequence: 2, Data: "second"}
	secondRead := make(chan RuntimeTerminalStreamMessage, 1)
	secondErr := make(chan error, 1)
	go func() {
		var message RuntimeTerminalStreamMessage
		err := conn.ReadJSON(&message)
		if err != nil {
			secondErr <- err
			return
		}
		secondRead <- message
	}()
	select {
	case message := <-secondRead:
		t.Fatalf("received output before ack: %#v", message)
	case err := <-secondErr:
		t.Fatalf("read failed before ack: %v", err)
	case <-time.After(150 * time.Millisecond):
	}

	if err := conn.WriteJSON(RuntimeTerminalStreamRequest{Type: "ack", Sequence: 1}); err != nil {
		t.Fatal(err)
	}
	var second RuntimeTerminalStreamMessage
	select {
	case second = <-secondRead:
	case err := <-secondErr:
		t.Fatal(err)
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for second output after ack")
	}
	if second.Type != "output" || len(second.Events) != 1 || second.Events[0].Sequence != 2 || second.Events[0].Data != "second" {
		t.Fatalf("second websocket output = %#v", second)
	}
}

func TestRuntimeHTTPServerTerminalWebSocketMissingTerminalSendsFinalError(t *testing.T) {
	t.Parallel()

	service := newRuntimeService()
	runtimeServer := newRuntimeHTTPServer(service)
	httpServer := httptest.NewServer(runtimeServer)
	defer httpServer.Close()

	wsURL := "ws" + strings.TrimPrefix(httpServer.URL, "http") + "/v1/terminals/missing/stream?token=" + runtimeServer.Token()
	conn, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()

	var message RuntimeTerminalStreamMessage
	if err := conn.ReadJSON(&message); err != nil {
		t.Fatal(err)
	}
	if message.Type != "final" || len(message.Events) != 1 || !message.Events[0].Final || message.Events[0].Error == "" {
		t.Fatalf("missing terminal websocket output = %#v", message)
	}
}

func TestRuntimeTerminalStreamBatchRespectsByteLimit(t *testing.T) {
	t.Parallel()

	events := make(chan RuntimeTerminalEvent, 3)
	events <- RuntimeTerminalEvent{Sequence: 1, Data: strings.Repeat("a", runtimeTerminalStreamBatchBytes/2)}
	events <- RuntimeTerminalEvent{Sequence: 2, Data: strings.Repeat("b", runtimeTerminalStreamBatchBytes/2)}
	events <- RuntimeTerminalEvent{Sequence: 3, Data: "tail"}

	batch, ok := nextRuntimeTerminalStreamBatch(context.Background(), events)
	if !ok {
		t.Fatal("expected terminal stream batch")
	}
	if len(batch) != 2 || batch[0].Sequence != 1 || batch[1].Sequence != 2 {
		t.Fatalf("batch = %#v", batch)
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
				Checkpoints: []RuntimeRunCheckpoint{{
					ID:             "checkpoint-1",
					Status:         turnStatusInterrupted,
					AcknowledgedAt: 1300,
				}},
				CreatedAt: 1000,
				UpdatedAt: 1200,
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

	req, err = http.NewRequest(http.MethodPost, "/v1/runs/run-1/checkpoints/checkpoint-1/acknowledge", nil)
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Authorization", "Bearer "+server.Token())
	resp = httptestResponse(server, req)
	if resp.status != http.StatusOK {
		t.Fatalf("ack status = %d body = %s", resp.status, resp.body.String())
	}
	if service.ackRunID != "run-1" || service.ackCheckpointID != "checkpoint-1" {
		t.Fatalf("ack args = %q %q", service.ackRunID, service.ackCheckpointID)
	}
	var ack RuntimeRunResponse
	if err := json.Unmarshal(resp.body.Bytes(), &ack); err != nil {
		t.Fatal(err)
	}
	if ack.Action == nil || !ack.Action.Accepted || ack.Action.Source.Action != runtimeRunCheckpointActionAcknowledge || ack.Action.Source.IdempotentBy != "run_id+checkpoint_id" {
		t.Fatalf("ack action metadata = %#v", ack.Action)
	}

	req, err = http.NewRequest(http.MethodPost, "/v1/runs/run-1/checkpoints/checkpoint-1/discard", nil)
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Authorization", "Bearer "+server.Token())
	resp = httptestResponse(server, req)
	if resp.status != http.StatusOK {
		t.Fatalf("discard status = %d body = %s", resp.status, resp.body.String())
	}
	if service.discardRunID != "run-1" || service.discardCheckpointID != "checkpoint-1" {
		t.Fatalf("discard args = %q %q", service.discardRunID, service.discardCheckpointID)
	}
	var discard RuntimeRunResponse
	if err := json.Unmarshal(resp.body.Bytes(), &discard); err != nil {
		t.Fatal(err)
	}
	if discard.Action == nil || !discard.Action.Accepted || discard.Action.Source.Action != runtimeRunCheckpointActionDiscard || discard.Action.Source.IdempotentBy != "run_id+checkpoint_id" {
		t.Fatalf("discard action metadata = %#v", discard.Action)
	}

	req, err = http.NewRequest(http.MethodPost, "/v1/runs/run-1/checkpoints/checkpoint-1/resume", nil)
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Authorization", "Bearer "+server.Token())
	resp = httptestResponse(server, req)
	if resp.status != http.StatusOK {
		t.Fatalf("resume status = %d body = %s", resp.status, resp.body.String())
	}
	if service.resumeRunID != "run-1" || service.resumeCheckpointID != "checkpoint-1" {
		t.Fatalf("resume args = %q %q", service.resumeRunID, service.resumeCheckpointID)
	}
	var resume RuntimeRunResumeResponse
	if err := json.Unmarshal(resp.body.Bytes(), &resume); err != nil {
		t.Fatal(err)
	}
	if resume.RunID != "run-1" || resume.CheckpointID != "checkpoint-1" || resume.TurnID == "" {
		t.Fatalf("resume response = %#v", resume)
	}
	if resume.Action == nil || !resume.Action.Accepted || resume.Action.Source.Action != runtimeRunCheckpointActionResume || resume.Action.Source.IdempotentBy != "" || !resume.Action.Source.StartsWorker {
		t.Fatalf("resume action metadata = %#v", resume.Action)
	}
	if resume.Run.Action != nil {
		t.Fatalf("nested run response should not carry resume action metadata: %#v", resume.Run.Action)
	}
}

func TestRuntimeHTTPServerRoutesRunSummariesToRuntimeService(t *testing.T) {
	t.Parallel()

	service := &recordingRuntimeService{
		runSummaries: RuntimeRunSummariesResponse{
			Runs: []RuntimeRunSummary{{
				ID:               "run-1",
				WorkspaceID:      "workspace-1",
				PrimarySessionID: "session-1",
				SessionIDs:       []string{"session-1", "session-child"},
				Objective:        "ship summaries",
				Source:           runtimeRunSourceUserPrompt,
				CreatedAt:        1000,
				UpdatedAt:        1200,
			}},
			Source: runtimeRunSummarySource(),
		},
		runSummary: RuntimeRunSummaryResponse{
			Run: RuntimeRunSummary{
				ID:               "run-1",
				WorkspaceID:      "workspace-1",
				PrimarySessionID: "session-1",
				SessionIDs:       []string{"session-1"},
				Objective:        "ship summaries",
				Source:           runtimeRunSourceUserPrompt,
				CreatedAt:        1000,
				UpdatedAt:        1200,
			},
			Source: runtimeRunSummarySource(),
		},
	}
	server := newRuntimeHTTPServer(service)

	req, err := http.NewRequest(http.MethodGet, "/v1/run-summaries", nil)
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Authorization", "Bearer "+server.Token())
	resp := httptestResponse(server, req)
	if resp.status != http.StatusOK {
		t.Fatalf("summaries status = %d body = %s", resp.status, resp.body.String())
	}
	var list RuntimeRunSummariesResponse
	if err := json.Unmarshal(resp.body.Bytes(), &list); err != nil {
		t.Fatal(err)
	}
	if len(list.Runs) != 1 || list.Runs[0].ID != "run-1" || !list.Source.SummaryOnly || !list.Source.ProjectionRequiredForLifecycle {
		t.Fatalf("summaries = %#v", list)
	}
	var rawList map[string]any
	if err := json.Unmarshal(resp.body.Bytes(), &rawList); err != nil {
		t.Fatal(err)
	}
	rawRuns, ok := rawList["runs"].([]any)
	if !ok || len(rawRuns) != 1 {
		t.Fatalf("raw summary runs = %#v", rawList["runs"])
	}
	rawRun, ok := rawRuns[0].(map[string]any)
	if !ok {
		t.Fatalf("raw summary run = %#v", rawRuns[0])
	}
	for _, field := range []string{"status", "finishedAt", "checkpoints", "diagnostics", "artifacts", "permissions", "projection"} {
		if _, ok := rawRun[field]; ok {
			t.Fatalf("summary run leaked %q field: %s", field, resp.body.String())
		}
	}

	req, err = http.NewRequest(http.MethodGet, "/v1/run-summaries/run-1", nil)
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Authorization", "Bearer "+server.Token())
	resp = httptestResponse(server, req)
	if resp.status != http.StatusOK {
		t.Fatalf("summary status = %d body = %s", resp.status, resp.body.String())
	}
	if service.runSummaryID != "run-1" {
		t.Fatalf("run summary id = %q, want run-1", service.runSummaryID)
	}
	var detail RuntimeRunSummaryResponse
	if err := json.Unmarshal(resp.body.Bytes(), &detail); err != nil {
		t.Fatal(err)
	}
	if detail.Run.ID != "run-1" || !detail.Source.ReadOnly || !detail.Source.PersistedRunAuthority {
		t.Fatalf("summary detail = %#v", detail)
	}

	req, err = http.NewRequest(http.MethodGet, "/v1/dev/module?token="+server.Token()+"&method=GET&path=/v1/run-summaries/run-2", nil)
	if err != nil {
		t.Fatal(err)
	}
	resp = httptestResponse(server, req)
	if resp.status != http.StatusOK {
		t.Fatalf("dev-module summary status = %d body = %s", resp.status, resp.body.String())
	}
	if service.runSummaryID != "run-2" {
		t.Fatalf("dev-module run summary id = %q, want run-2", service.runSummaryID)
	}
}

func TestRuntimeHTTPServerRoutesRunCheckpointMarkersToRuntimeService(t *testing.T) {
	t.Parallel()

	service := &recordingRuntimeService{
		runCheckpointMarkers: RuntimeRunCheckpointMarkersResponse{
			Markers: []RuntimeRunCheckpointMarker{{
				RunID:          "run-1",
				CheckpointID:   "checkpoint-1",
				TurnID:         "turn-1",
				AcknowledgedAt: 1200,
				ResumedTurnIDs: []string{"turn-resume-1"},
			}},
			Source: runtimeRunCheckpointMarkerSource(),
		},
		runCheckpointMarker: RuntimeRunCheckpointMarkerResponse{
			Marker: RuntimeRunCheckpointMarker{
				RunID:        "run-1",
				CheckpointID: "checkpoint-1",
				TurnID:       "turn-1",
				DiscardedAt:  1300,
			},
			Source: runtimeRunCheckpointMarkerSource(),
		},
	}
	server := newRuntimeHTTPServer(service)

	req, err := http.NewRequest(http.MethodGet, "/v1/runs/run-1/checkpoint-markers", nil)
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Authorization", "Bearer "+server.Token())
	resp := httptestResponse(server, req)
	if resp.status != http.StatusOK {
		t.Fatalf("markers status = %d body = %s", resp.status, resp.body.String())
	}
	if service.runCheckpointMarkersID != "run-1" {
		t.Fatalf("marker list run id = %q", service.runCheckpointMarkersID)
	}
	var list RuntimeRunCheckpointMarkersResponse
	if err := json.Unmarshal(resp.body.Bytes(), &list); err != nil {
		t.Fatal(err)
	}
	if len(list.Markers) != 1 || !list.Source.MarkerOnly || !list.Source.ProjectionRequiredForEligibility {
		t.Fatalf("marker list = %#v", list)
	}
	var rawList map[string]any
	if err := json.Unmarshal(resp.body.Bytes(), &rawList); err != nil {
		t.Fatal(err)
	}
	rawMarkers, ok := rawList["markers"].([]any)
	if !ok || len(rawMarkers) != 1 {
		t.Fatalf("raw markers = %#v", rawList["markers"])
	}
	rawMarker, ok := rawMarkers[0].(map[string]any)
	if !ok {
		t.Fatalf("raw marker = %#v", rawMarkers[0])
	}
	for _, field := range []string{"status", "summary", "artifactRefs", "resumeEligible", "projection", "action"} {
		if _, ok := rawMarker[field]; ok {
			t.Fatalf("marker list leaked %q field: %s", field, resp.body.String())
		}
	}

	req, err = http.NewRequest(http.MethodGet, "/v1/runs/run-1/checkpoint-markers/checkpoint-1", nil)
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Authorization", "Bearer "+server.Token())
	resp = httptestResponse(server, req)
	if resp.status != http.StatusOK {
		t.Fatalf("marker status = %d body = %s", resp.status, resp.body.String())
	}
	if service.runCheckpointMarkerRunID != "run-1" || service.runCheckpointMarkerID != "checkpoint-1" {
		t.Fatalf("marker detail ids = %q/%q", service.runCheckpointMarkerRunID, service.runCheckpointMarkerID)
	}
	var detail RuntimeRunCheckpointMarkerResponse
	if err := json.Unmarshal(resp.body.Bytes(), &detail); err != nil {
		t.Fatal(err)
	}
	if detail.Marker.CheckpointID != "checkpoint-1" || !detail.Source.ReadOnly || !detail.Source.MarkerOnly {
		t.Fatalf("marker detail = %#v", detail)
	}
	if service.runProjectionRequest.SessionID != "" || service.runSchedulerPlanReq.RunID != "" || service.ackRunID != "" || service.resumeRunID != "" {
		t.Fatalf("marker routes called projection/scheduler/write paths: projection=%#v plan=%#v ack=%q resume=%q", service.runProjectionRequest, service.runSchedulerPlanReq, service.ackRunID, service.resumeRunID)
	}

	req, err = http.NewRequest(http.MethodGet, "/v1/dev/module?token="+server.Token()+"&method=GET&path=/v1/runs/run-2/checkpoint-markers/checkpoint-2", nil)
	if err != nil {
		t.Fatal(err)
	}
	resp = httptestResponse(server, req)
	if resp.status != http.StatusOK {
		t.Fatalf("dev-module marker status = %d body = %s", resp.status, resp.body.String())
	}
	if service.runCheckpointMarkerRunID != "run-2" || service.runCheckpointMarkerID != "checkpoint-2" {
		t.Fatalf("dev-module marker ids = %q/%q", service.runCheckpointMarkerRunID, service.runCheckpointMarkerID)
	}
}

func TestRuntimeHTTPServerRunStatusWriterRereadSmoke(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	runtimeWorkbench, workspace := workbenchForSkillTest(t)
	conn, err := db.Connect(ctx, workspace.DataDir)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_ = db.Release(workspace.DataDir)
	})
	service := newRuntimeService()
	service.runtime = runtimeWorkbench
	service.workspace = &workspace
	service.turns = newRuntimeTurnStore(conn)
	service.runs = newRuntimeRunStore(conn)
	service.toolCalls = scheduler.New(NewRuntimeToolCallStoreForDB(conn))
	service.permissionStore = newRuntimePermissionStore(conn)
	service.eventStore = newRuntimeEventStore(conn)
	service.agentTasks = newRuntimeAgentTaskStore(conn)

	sess, err := runtimeWorkbench.CreateSession(ctx, workspace.ID, "http status writer reread")
	if err != nil {
		t.Fatal(err)
	}
	run, err := service.runs.EnsureForSession(ctx, workspace.ID, sess.ID, "http status writer reread", runtimeRunSourceUserPrompt)
	if err != nil {
		t.Fatal(err)
	}
	recorder := runtimeSchedulerRecorder{service: service}
	if err := recorder.AgentTaskCompleted(ctx, agent.AgentTaskRecord{
		ID:              "task-http-status-writer",
		ParentSessionID: sess.ID,
		Title:           "http status writer",
		Status:          agentTaskStatusCompleted,
		Progress:        100,
		ResultSummary:   "http reread completed",
		StartedAt:       run.CreatedAt + 100,
		FinishedAt:      run.CreatedAt + 200,
	}); err != nil {
		t.Fatal(err)
	}
	service.storeRuntimeEvent(RuntimeEvent{
		ID:        newRuntimeEventID(),
		Type:      "run.status.payload_smoke",
		SessionID: sess.ID,
		Payload: map[string]any{
			"run_id": run.ID,
			"status": runtimeRunStatusFailed,
		},
	})
	server := newRuntimeHTTPServer(service)

	req, err := http.NewRequest(http.MethodGet, "/v1/runs/"+run.ID, nil)
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Authorization", "Bearer "+server.Token())
	resp := httptestResponse(server, req)
	if resp.status != http.StatusOK {
		t.Fatalf("run detail status = %d body = %s", resp.status, resp.body.String())
	}
	var detail RuntimeRunResponse
	if err := json.Unmarshal(resp.body.Bytes(), &detail); err != nil {
		t.Fatal(err)
	}
	if detail.Run.Status != runtimeRunStatusCompleted || detail.Projection.Status != runtimeRunStatusCompleted {
		t.Fatalf("HTTP run detail did not reread projection status: %#v", detail)
	}
	if detail.Action != nil {
		t.Fatalf("plain HTTP reread should not carry write action metadata: %#v", detail.Action)
	}

	req, err = http.NewRequest(http.MethodGet, "/v1/sessions/"+sess.ID+"/run-projection", nil)
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Authorization", "Bearer "+server.Token())
	resp = httptestResponse(server, req)
	if resp.status != http.StatusOK {
		t.Fatalf("run projection status = %d body = %s", resp.status, resp.body.String())
	}
	var projection RuntimeRunProjectionResponse
	if err := json.Unmarshal(resp.body.Bytes(), &projection); err != nil {
		t.Fatal(err)
	}
	if projection.Run.ID != run.ID || projection.Run.Status != runtimeRunStatusCompleted || !projection.Run.Source.SessionActivityParity {
		t.Fatalf("HTTP projection did not preserve API DTO parity: %#v", projection.Run)
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
			Checkpoints: []RuntimeRunCheckpoint{{
				ID:     "checkpoint-1",
				Status: turnStatusInterrupted,
			}},
			CreatedAt: 1000,
			UpdatedAt: 1200,
		}},
		transitionHistory: RuntimeRunTransitionHistoryResponse{
			Transitions: []RuntimeRunTransition{{
				ID:        "transition-1",
				RunID:     "run-1",
				SessionID: "session-1",
				TurnID:    "turn-1",
				ToStatus:  runtimeRunStatusCompleted,
				Source:    runtimeRunTransitionSourceTurnFinished,
				CreatedAt: 1200,
			}},
			Window: RuntimeActivityWindow{Limit: 7, ToEnd: true},
			Source: RuntimeRunTransitionHistorySource{
				Kind:      runtimeRunTransitionHistorySourceKind,
				ReadOnly:  true,
				AuditOnly: true,
			},
		},
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

	req, err = http.NewRequest(http.MethodGet, "/v1/dev/module?token="+server.Token()+"&path=/v1/run-transitions%3Frun_id%3Drun-1&limit=7&cursor=v1%3Atransition", nil)
	if err != nil {
		t.Fatal(err)
	}
	resp = httptestResponse(server, req)
	if resp.status != http.StatusOK || service.transitionHistoryReq.RunID != "run-1" || service.transitionHistoryReq.Limit != 7 || service.transitionHistoryReq.Cursor != "v1:transition" {
		t.Fatalf("transition history status = %d body = %s args=%#v", resp.status, resp.body.String(), service.transitionHistoryReq)
	}

	req, err = http.NewRequest(http.MethodGet, "/v1/dev/module?token="+server.Token()+"&path=/v1/run-scheduler-plan%3Frun_id%3Drun-1%26task_id%3Dtask-1&limit=8&cursor=v1%3Aplan", nil)
	if err != nil {
		t.Fatal(err)
	}
	resp = httptestResponse(server, req)
	if resp.status != http.StatusOK || service.runSchedulerPlanReq.RunID != "run-1" || service.runSchedulerPlanReq.TaskID != "task-1" || service.runSchedulerPlanReq.Limit != 8 || service.runSchedulerPlanReq.Cursor != "v1:plan" {
		t.Fatalf("scheduler plan status = %d body = %s args=%#v", resp.status, resp.body.String(), service.runSchedulerPlanReq)
	}
	if service.chatCalls != 0 || service.cancelledTask != "" {
		t.Fatalf("scheduler plan dev route caused side effects: chatCalls=%d cancelledTask=%q", service.chatCalls, service.cancelledTask)
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

	req, err = http.NewRequest(http.MethodGet, "/v1/dev/module?token="+server.Token()+"&method=POST&path=/v1/runs/run-1/checkpoints/checkpoint-1/acknowledge", nil)
	if err != nil {
		t.Fatal(err)
	}
	resp = httptestResponse(server, req)
	if resp.status != http.StatusOK || service.ackRunID != "run-1" || service.ackCheckpointID != "checkpoint-1" {
		t.Fatalf("ack status = %d body = %s args=%q/%q", resp.status, resp.body.String(), service.ackRunID, service.ackCheckpointID)
	}

	req, err = http.NewRequest(http.MethodGet, "/v1/dev/module?token="+server.Token()+"&method=POST&path=/v1/runs/run-1/checkpoints/checkpoint-1/discard", nil)
	if err != nil {
		t.Fatal(err)
	}
	resp = httptestResponse(server, req)
	if resp.status != http.StatusOK || service.discardRunID != "run-1" || service.discardCheckpointID != "checkpoint-1" {
		t.Fatalf("discard status = %d body = %s args=%q/%q", resp.status, resp.body.String(), service.discardRunID, service.discardCheckpointID)
	}

	req, err = http.NewRequest(http.MethodGet, "/v1/dev/module?token="+server.Token()+"&method=POST&path=/v1/runs/run-1/checkpoints/checkpoint-1/resume", nil)
	if err != nil {
		t.Fatal(err)
	}
	resp = httptestResponse(server, req)
	if resp.status != http.StatusOK || service.resumeRunID != "run-1" || service.resumeCheckpointID != "checkpoint-1" {
		t.Fatalf("resume status = %d body = %s args=%q/%q", resp.status, resp.body.String(), service.resumeRunID, service.resumeCheckpointID)
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
	if turn.Action != nil {
		t.Fatalf("ordinary turn read returned action metadata: %#v", turn.Action)
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
	var status RuntimeStatus
	if err := json.Unmarshal(resp.body.Bytes(), &status); err != nil {
		t.Fatal(err)
	}
	if status.Action == nil || status.Action.Source.Action != runtimeTurnActionCancel || status.Action.Reason != runtimeTurnActionReasonCancelled {
		t.Fatalf("cancel action metadata = %#v", status.Action)
	}

	req, err = http.NewRequest(http.MethodPost, "/v1/turns/turn-1/interrupted/done", nil)
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Authorization", "Bearer "+server.Token())
	resp = httptestResponse(server, req)
	if resp.status != http.StatusOK {
		t.Fatalf("interrupted done status = %d body = %s", resp.status, resp.body.String())
	}
	var acknowledged RuntimeTurnResponse
	if err := json.Unmarshal(resp.body.Bytes(), &acknowledged); err != nil {
		t.Fatal(err)
	}
	if acknowledged.Action == nil || acknowledged.Action.Source.Action != runtimeTurnActionMarkInterruptedDone || acknowledged.Action.Reason != runtimeTurnActionReasonInterruptedMarkedDone {
		t.Fatalf("mark interrupted done action metadata = %#v", acknowledged.Action)
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

	req, err = http.NewRequest(http.MethodPost, "/v1/sessions", strings.NewReader(`{"title":"Created"}`))
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Authorization", "Bearer "+server.Token())
	resp = httptestResponse(server, req)
	if resp.status != http.StatusOK || service.createSessionReq.Title != "Created" {
		t.Fatalf("create status = %d req = %#v body = %s", resp.status, service.createSessionReq, resp.body.String())
	}
	var created RuntimeSessionResponse
	if err := json.Unmarshal(resp.body.Bytes(), &created); err != nil {
		t.Fatal(err)
	}
	if created.Session.ID == "" || !created.Session.Active {
		t.Fatalf("created session = %#v", created.Session)
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
		check  func(t *testing.T, resp httpRecorder)
	}{
		{
			name:   "create session",
			method: http.MethodPost,
			path:   "/v1/sessions",
			body:   `{"title":"Draft"}`,
			check: func(t *testing.T, resp httpRecorder) {
				t.Helper()
				if service.createSessionReq.Title != "Draft" {
					t.Fatalf("create session req = %#v", service.createSessionReq)
				}
			},
		},
		{
			name:   "new chat draft",
			method: http.MethodPost,
			path:   "/v1/runtime/new-chat",
			body:   `{"title":"Draft"}`,
		},
		{
			name:   "select session",
			method: http.MethodPost,
			path:   "/v1/sessions/session-2/select",
			check: func(t *testing.T, resp httpRecorder) {
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
			check: func(t *testing.T, resp httpRecorder) {
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
			check: func(t *testing.T, resp httpRecorder) {
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
			check: func(t *testing.T, resp httpRecorder) {
				t.Helper()
				if strings.Contains(resp.body.String(), `"action"`) {
					t.Fatalf("ordinary turn read returned action metadata: %s", resp.body.String())
				}
			},
		},
		{
			name:   "cancel turn",
			method: http.MethodPost,
			path:   "/v1/turns/turn-1/cancel",
			check: func(t *testing.T, resp httpRecorder) {
				t.Helper()
				if service.cancelledTurn != "turn-1" {
					t.Fatalf("cancelled turn = %q, want turn-1", service.cancelledTurn)
				}
				body := resp.body.String()
				if !strings.Contains(body, `"action":"`+runtimeTurnActionCancel+`"`) || !strings.Contains(body, `"reason":"`+runtimeTurnActionReasonCancelled+`"`) {
					t.Fatalf("cancel action metadata missing: %s", body)
				}
			},
		},
		{
			name:   "mark interrupted done",
			method: http.MethodPost,
			path:   "/v1/turns/turn-1/interrupted/done",
			check: func(t *testing.T, resp httpRecorder) {
				t.Helper()
				if service.markInterruptedDoneTurn != "turn-1" {
					t.Fatalf("mark interrupted done turn = %q, want turn-1", service.markInterruptedDoneTurn)
				}
				body := resp.body.String()
				if !strings.Contains(body, `"action":"`+runtimeTurnActionMarkInterruptedDone+`"`) || !strings.Contains(body, `"reason":"`+runtimeTurnActionReasonInterruptedMarkedDone+`"`) {
					t.Fatalf("mark interrupted done action metadata missing: %s", body)
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
				tt.check(t, resp)
			}
		})
	}
}

func TestRuntimeHTTPServerSmokeCoversSessionTurnAndInventory(t *testing.T) {
	t.Parallel()

	service := &recordingRuntimeService{
		status: RuntimeStatus{Ready: true, SessionID: "session-1"},
		skills: RuntimeSkillsResponse{
			Skills: []RuntimeSkill{{Name: "agent-builder-config", Builtin: true, Enabled: true, State: "normal"}},
		},
		mcpServers: RuntimeMCPServersResponse{
			Servers: []RuntimeMCPServer{{Name: "docs", Type: "http", State: "connected"}},
		},
		capabilities: RuntimeCapabilitiesResponse{
			Capabilities: []RuntimeCapability{{ID: "skill:agent-builder-config", Kind: "skill", Name: "agent-builder-config", Enabled: true}},
		},
	}
	server := newRuntimeHTTPServer(service)
	client := runtimeSmokeClient{server: server, token: server.Token()}

	if session := client.postSession(t); session.Session.ID != "session-1" || !session.Session.Active {
		t.Fatalf("new session response = %#v", session)
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
	req, err := http.NewRequest(http.MethodPost, "/v1/capabilities/skill%3Aagent-builder-config/refresh", nil)
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Authorization", "Bearer "+server.Token())

	resp := httptestResponse(server, req)
	if resp.status != http.StatusOK {
		t.Fatalf("status = %d body = %s", resp.status, resp.body.String())
	}
	if service.refreshedCapability != "skill:agent-builder-config" {
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
		agentTasks: RuntimeAgentTasksResponse{Tasks: []RuntimeAgentTask{{
			ID: "task-1", ParentSessionID: "session-1", ParentTurnID: "turn-1", ParentToolCallID: "tool-1", ChildSessionID: "child-1", Status: agentTaskStatusRunning,
		}}},
		agentTaskMessages: RuntimeAgentTaskMessagesResponse{Messages: []RuntimeAgentTaskMessage{{
			ID: "msg-1", TaskID: "task-1", Direction: taskMessageDirectionChildToParent, Kind: taskMessageKindProgress, Status: taskMessageStatusCreated, CreatedAt: 1,
		}}},
		agentTaskMessage: RuntimeAgentTaskMessageResponse{Message: RuntimeAgentTaskMessage{
			ID: "msg-follow-up", TaskID: "task-1", Direction: taskMessageDirectionParentToChild, Kind: taskMessageKindInstruction, Status: taskMessageStatusRejected, ContentSummary: "check logs",
		}},
		agentTaskResult: RuntimeAgentTaskResultResponse{Result: RuntimeAgentTaskResult{TaskID: "task-1", Status: agentTaskStatusCompleted, Summary: "done"}},
		agentTaskOutput: RuntimeAgentTaskOutputResponse{TaskID: "task-1", Status: agentTaskStatusCompleted, Summary: "done", OutputRefs: []string{"runtime://refs/ref-1"}},
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

	req, err = http.NewRequest(http.MethodGet, "/v1/sessions/session-1/agent-tasks", nil)
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Authorization", "Bearer "+server.Token())
	resp = httptestResponse(server, req)
	if resp.status != http.StatusOK || !strings.Contains(resp.body.String(), "child-1") || service.agentTaskSession != "session-1" {
		t.Fatalf("session agent tasks status = %d body = %s session=%q", resp.status, resp.body.String(), service.agentTaskSession)
	}

	req, err = http.NewRequest(http.MethodGet, "/v1/turns/turn-1/agent-tasks", nil)
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Authorization", "Bearer "+server.Token())
	resp = httptestResponse(server, req)
	if resp.status != http.StatusOK || !strings.Contains(resp.body.String(), "tool-1") || service.agentTaskTurn != "turn-1" {
		t.Fatalf("turn agent tasks status = %d body = %s turn=%q", resp.status, resp.body.String(), service.agentTaskTurn)
	}

	req, err = http.NewRequest(http.MethodGet, "/v1/agent-tasks/task-1/messages", nil)
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Authorization", "Bearer "+server.Token())
	resp = httptestResponse(server, req)
	if resp.status != http.StatusOK || !strings.Contains(resp.body.String(), "msg-1") {
		t.Fatalf("messages status = %d body = %s", resp.status, resp.body.String())
	}

	req, err = http.NewRequest(http.MethodPost, "/v1/agent-tasks/task-1/messages", strings.NewReader(`{"direction":"parent_to_child","kind":"instruction","contentSummary":"check logs"}`))
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Authorization", "Bearer "+server.Token())
	resp = httptestResponse(server, req)
	if resp.status != http.StatusOK {
		t.Fatalf("follow-up status = %d body = %s", resp.status, resp.body.String())
	}
	if !strings.Contains(resp.body.String(), "msg-follow-up") || !strings.Contains(resp.body.String(), taskMessageStatusRejected) {
		t.Fatalf("follow-up body = %s", resp.body.String())
	}
	if service.agentTaskFollowUpID != "task-1" || service.agentTaskFollowUpReq.ContentSummary != "check logs" {
		t.Fatalf("follow-up id=%q req=%#v", service.agentTaskFollowUpID, service.agentTaskFollowUpReq)
	}
	if service.agentTaskMessageCreateID != "" {
		t.Fatalf("POST /messages used create path for task %q", service.agentTaskMessageCreateID)
	}

	req, err = http.NewRequest(http.MethodGet, "/v1/agent-tasks/task-1/result", nil)
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Authorization", "Bearer "+server.Token())
	resp = httptestResponse(server, req)
	if resp.status != http.StatusOK || !strings.Contains(resp.body.String(), "done") {
		t.Fatalf("result status = %d body = %s", resp.status, resp.body.String())
	}

	req, err = http.NewRequest(http.MethodGet, "/v1/agent-tasks/task-1/output", nil)
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Authorization", "Bearer "+server.Token())
	resp = httptestResponse(server, req)
	if resp.status != http.StatusOK || !strings.Contains(resp.body.String(), "runtime://refs/ref-1") {
		t.Fatalf("output status = %d body = %s", resp.status, resp.body.String())
	}

	req, err = http.NewRequest(http.MethodPost, "/v1/agent-tasks/task-1/cancel", nil)
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Authorization", "Bearer "+server.Token())
	resp = httptestResponse(server, req)
	if resp.status != http.StatusOK {
		t.Fatalf("cancel status = %d body = %s", resp.status, resp.body.String())
	}
	if service.cancelledTask != "task-1" {
		t.Fatalf("cancelled task = %q", service.cancelledTask)
	}
	var cancel RuntimeAgentTaskResponse
	if err := json.Unmarshal(resp.body.Bytes(), &cancel); err != nil {
		t.Fatal(err)
	}
	if cancel.Action == nil || !cancel.Action.Accepted || cancel.Action.Source.Action != runtimeAgentTaskCancelAction || cancel.Action.Source.IdempotentBy != "task_id" {
		t.Fatalf("cancel action metadata = %#v", cancel.Action)
	}

	req, err = http.NewRequest(http.MethodGet, "/v1/"+legacyTaskRouteSegment()+"/task-1/output", nil)
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Authorization", "Bearer "+server.Token())
	resp = httptestResponse(server, req)
	if resp.status != http.StatusNotFound {
		t.Fatalf("legacy task output status = %d body = %s, want 404", resp.status, resp.body.String())
	}

	req, err = http.NewRequest(http.MethodGet, "/v1/turns/turn-1/"+legacyTaskRouteSegment(), nil)
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Authorization", "Bearer "+server.Token())
	resp = httptestResponse(server, req)
	if resp.status != http.StatusNotFound {
		t.Fatalf("legacy turn tasks status = %d body = %s, want 404", resp.status, resp.body.String())
	}
}

func legacyTaskRouteSegment() string {
	return "tasks"
}

func TestRuntimeHTTPServerRoutesSkillsToRuntimeService(t *testing.T) {
	t.Parallel()

	service := &recordingRuntimeService{
		skills: RuntimeSkillsResponse{
			Skills: []RuntimeSkill{{Name: "agent-builder-config", Builtin: true, Enabled: true, State: "normal"}},
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
	if len(skills.Skills) != 1 || skills.Skills[0].Name != "agent-builder-config" {
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

func (c runtimeSmokeClient) postSession(t *testing.T) RuntimeSessionResponse {
	t.Helper()
	req, err := http.NewRequest(http.MethodPost, "/v1/sessions", strings.NewReader(`{"title":"Smoke"}`))
	if err != nil {
		t.Fatal(err)
	}
	var response RuntimeSessionResponse
	c.doJSON(t, req, &response)
	return response
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
