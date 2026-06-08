package main

import (
	"context"
	"testing"

	runtime "github.com/charmbracelet/crush/internal/runtime"
)

func TestRuntimeBridgeDelegatesToRuntimeService(t *testing.T) {
	t.Parallel()

	service := &recordingRuntimeService{}
	bridge := &RuntimeBridge{service: service}

	if _, err := bridge.Chat(context.Background(), RuntimeChatRequest{Prompt: "hello"}); err != nil {
		t.Fatal(err)
	}
	if service.chatCalls != 1 {
		t.Fatalf("chatCalls = %d, want 1", service.chatCalls)
	}
}

func TestRuntimeBridgeForwardsCapabilityRefresh(t *testing.T) {
	t.Parallel()

	service := &recordingRuntimeService{}
	bridge := &RuntimeBridge{service: service}

	resp, err := bridge.RefreshCapability(context.Background(), "skill:docs")
	if err != nil {
		t.Fatal(err)
	}
	if service.refreshedCapability != "skill:docs" {
		t.Fatalf("refreshed capability = %q", service.refreshedCapability)
	}
	if resp.Capability.ID != "skill:docs" || resp.Capability.State != "loaded" {
		t.Fatalf("response = %#v", resp)
	}
}

func TestRuntimeBridgeForwardsToolSearch(t *testing.T) {
	t.Parallel()

	service := &recordingRuntimeService{}
	bridge := &RuntimeBridge{service: service}

	resp, err := bridge.SearchTools(context.Background(), RuntimeToolSearchRequest{Query: "docs"})
	if err != nil {
		t.Fatal(err)
	}
	if service.toolSearchQuery != "docs" || resp.Query != "docs" {
		t.Fatalf("tool search forwarding failed: query=%q resp=%#v", service.toolSearchQuery, resp)
	}
}

func TestRuntimeBridgeForwardsReplayExport(t *testing.T) {
	t.Parallel()

	service := &recordingRuntimeService{}
	bridge := &RuntimeBridge{service: service}

	resp, err := bridge.ReplayExport(context.Background(), RuntimeReplayExportRequest{TurnID: "turn-1", After: 3})
	if err != nil {
		t.Fatal(err)
	}
	if service.replayExportRequest.TurnID != "turn-1" || service.replayExportRequest.After != 3 {
		t.Fatalf("replay export request = %#v", service.replayExportRequest)
	}
	if resp.TurnID != "turn-1" || !resp.Summary.Redacted {
		t.Fatalf("replay export response = %#v", resp)
	}
}

func TestRuntimeBridgeForwardsEventsCursor(t *testing.T) {
	t.Parallel()

	service := &recordingRuntimeService{}
	bridge := &RuntimeBridge{service: service}

	if _, err := bridge.Events(context.Background(), 42); err != nil {
		t.Fatal(err)
	}
	if service.eventsAfter != 42 {
		t.Fatalf("events cursor = %d, want 42", service.eventsAfter)
	}
}

func TestRuntimeBridgePhase62PackagedHandoffRecoveryContract(t *testing.T) {
	t.Parallel()

	service := &recordingRuntimeService{
		status: RuntimeStatus{SessionID: "session-new"},
		activity: RuntimeSessionActivityResponse{
			SessionID: "session-new",
			Turns: []RuntimeTurn{{
				ID:        "turn-interrupted",
				SessionID: "session-new",
				Status:    "interrupted",
				Diagnostics: runtime.RuntimeTurnDiagnostics{
					ProducedArtifacts: []string{"tmp/runtime-dev/phase62-structured.json"},
				},
				Interrupted: &runtime.RuntimeInterruptedSummary{
					TurnID:    "turn-interrupted",
					SessionID: "session-new",
					PendingTool: runtime.RuntimeInterruptedToolSummary{
						ID:     "tool-stale",
						Name:   "bash",
						Status: "cancelled",
					},
					ProducedArtifacts: []string{"tmp/runtime-dev/phase62-structured.json"},
				},
			}},
			ToolCalls: []RuntimeToolCall{{
				ID:        "tool-stale",
				SessionID: "session-new",
				TurnID:    "turn-interrupted",
				Name:      "bash",
				Status:    "cancelled",
			}},
		},
		eventsResponse: RuntimeEventsResponse{
			Events: []RuntimeEvent{{
				Sequence:  7,
				Type:      "turn.interrupted",
				SessionID: "session-new",
				TurnID:    "turn-interrupted",
			}},
		},
		markInterruptedDoneResponse: RuntimeTurnResponse{
			Turn: RuntimeTurn{ID: "turn-interrupted", SessionID: "session-new", Status: "cancelled"},
		},
	}
	bridge := &RuntimeBridge{service: service}

	if _, err := bridge.NewChat(context.Background(), ""); err != nil {
		t.Fatal(err)
	}
	if service.newChatTitle != "" {
		t.Fatalf("new chat title = %q, want empty draft title", service.newChatTitle)
	}

	chat, err := bridge.Chat(context.Background(), RuntimeChatRequest{Prompt: "phase62 packaged handoff"})
	if err != nil {
		t.Fatal(err)
	}
	if len(service.chatRequests) != 1 || service.chatRequests[0].SessionID != "" {
		t.Fatalf("new-chat handoff should submit a draft chat without stale session id: %#v", service.chatRequests)
	}
	if chat.Status.SessionID != "session-new" || chat.TurnID != "turn-new" {
		t.Fatalf("chat response = %#v", chat)
	}

	events, err := bridge.Events(context.Background(), 6)
	if err != nil {
		t.Fatal(err)
	}
	if service.eventsAfter != 6 {
		t.Fatalf("events cursor = %d, want 6", service.eventsAfter)
	}
	if len(events.Events) != 1 || events.Events[0].Type != "turn.interrupted" {
		t.Fatalf("events should be lifecycle refresh triggers: %#v", events.Events)
	}

	activity, err := bridge.SessionActivity(context.Background(), "session-new")
	if err != nil {
		t.Fatal(err)
	}
	if service.sessionActivityID != "session-new" {
		t.Fatalf("session activity id = %q", service.sessionActivityID)
	}
	if len(activity.Turns) != 1 || activity.Turns[0].Interrupted == nil {
		t.Fatalf("interrupted recovery should hydrate from SessionActivity: %#v", activity.Turns)
	}
	if len(activity.ToolCalls) != 1 || activity.ToolCalls[0].Status != "cancelled" {
		t.Fatalf("stale running/waiting tool was restored: %#v", activity.ToolCalls)
	}
	for _, produced := range activity.Turns[0].Diagnostics.ProducedArtifacts {
		if produced == "tmp/runtime-dev/phase62-prose-only.json" {
			t.Fatalf("assistant prose-only artifact was treated as runtime evidence: %#v", activity.Turns[0].Diagnostics)
		}
	}

	done, err := bridge.MarkInterruptedDone(context.Background(), "turn-interrupted")
	if err != nil {
		t.Fatal(err)
	}
	if service.markInterruptedDoneID != "turn-interrupted" {
		t.Fatalf("mark interrupted done id = %q", service.markInterruptedDoneID)
	}
	if done.Turn.Status != "cancelled" {
		t.Fatalf("MarkInterruptedDone should preserve cancelled terminal acknowledgement semantics: %#v", done.Turn)
	}
}

func TestRuntimeBridgeNarrowActivityUsesRuntimeService(t *testing.T) {
	t.Parallel()

	service := &recordingRuntimeService{
		activityWindow: RuntimeSessionActivityWindowResponse{
			SessionID: "session-window",
			Turns:     []RuntimeTurn{{ID: "turn-window", SessionID: "session-window", Status: "running"}},
			Window:    RuntimeActivityWindow{Limit: 2, ToEnd: true},
		},
		turnActivity: RuntimeTurnActivityResponse{
			SessionID: "session-window",
			TurnID:    "turn-window",
			Turns:     []RuntimeTurn{{ID: "turn-window", SessionID: "session-window", Status: "running"}},
		},
		runProjection: RuntimeRunProjectionResponse{Run: RuntimeRunProjection{
			ID:               "run:session:session-window",
			PrimarySessionID: "session-window",
			Source: RuntimeRunProjectionSource{
				Kind:                  "session_activity_projection",
				ReadOnly:              true,
				SessionActivityParity: true,
			},
		}},
	}
	bridge := &RuntimeBridge{service: service}

	window, err := bridge.SessionActivityWindow(context.Background(), "session-window", 2)
	if err != nil {
		t.Fatal(err)
	}
	if service.sessionActivityWindowID != "session-window" || service.sessionActivityWindowLimit != 2 {
		t.Fatalf("window args = %q %d", service.sessionActivityWindowID, service.sessionActivityWindowLimit)
	}
	if window.Window.Limit != 2 || len(window.Turns) != 1 {
		t.Fatalf("window = %#v", window)
	}
	cursorWindow, err := bridge.SessionActivityCursorWindow(context.Background(), "session-window", "v1:cursor", 3)
	if err != nil {
		t.Fatal(err)
	}
	if service.sessionActivityWindowID != "session-window" || service.sessionActivityWindowCursor != "v1:cursor" || service.sessionActivityWindowLimit != 3 {
		t.Fatalf("cursor window args = %q %q %d", service.sessionActivityWindowID, service.sessionActivityWindowCursor, service.sessionActivityWindowLimit)
	}
	if cursorWindow.SessionID != "session-window" {
		t.Fatalf("cursor window = %#v", cursorWindow)
	}

	turnActivity, err := bridge.TurnActivity(context.Background(), "turn-window")
	if err != nil {
		t.Fatal(err)
	}
	if service.turnActivityID != "turn-window" || turnActivity.TurnID != "turn-window" {
		t.Fatalf("turn activity = %#v service id %q", turnActivity, service.turnActivityID)
	}

	runProjection, err := bridge.RunProjection(context.Background(), RuntimeRunProjectionRequest{SessionID: "session-window", Cursor: "v1:run", Limit: 4})
	if err != nil {
		t.Fatal(err)
	}
	if service.runProjectionRequest.SessionID != "session-window" || service.runProjectionRequest.Cursor != "v1:run" || service.runProjectionRequest.Limit != 4 {
		t.Fatalf("run projection request = %#v", service.runProjectionRequest)
	}
	if runProjection.Run.PrimarySessionID != "session-window" || !runProjection.Run.Source.ReadOnly || !runProjection.Run.Source.SessionActivityParity {
		t.Fatalf("run projection = %#v", runProjection)
	}
}

func TestRuntimeBridgeForwardsDurableRunReads(t *testing.T) {
	t.Parallel()

	service := &recordingRuntimeService{
		runs: RuntimeRunsResponse{Runs: []RuntimeRun{{
			ID:               "run-1",
			WorkspaceID:      "workspace-1",
			PrimarySessionID: "session-1",
			Status:           "completed",
			Source:           "backfill",
		}}},
		run: RuntimeRunResponse{Run: RuntimeRun{
			ID:               "run-1",
			WorkspaceID:      "workspace-1",
			PrimarySessionID: "session-1",
			Status:           "completed",
			Source:           "backfill",
		}},
	}
	bridge := &RuntimeBridge{service: service}

	runs, err := bridge.Runs(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(runs.Runs) != 1 || runs.Runs[0].ID != "run-1" {
		t.Fatalf("runs = %#v", runs)
	}

	run, err := bridge.Run(context.Background(), "run-1")
	if err != nil {
		t.Fatal(err)
	}
	if service.runID != "run-1" || run.Run.ID != "run-1" {
		t.Fatalf("run = %#v service id %q", run, service.runID)
	}

	ack, err := bridge.AcknowledgeRunCheckpoint(context.Background(), "run-1", "checkpoint-1")
	if err != nil {
		t.Fatal(err)
	}
	if service.ackRunID != "run-1" || service.ackCheckpointID != "checkpoint-1" || ack.Run.ID != "run-1" {
		t.Fatalf("ack = %#v args=%q/%q", ack, service.ackRunID, service.ackCheckpointID)
	}

	discard, err := bridge.DiscardRunCheckpoint(context.Background(), "run-1", "checkpoint-1")
	if err != nil {
		t.Fatal(err)
	}
	if service.discardRunID != "run-1" || service.discardCheckpointID != "checkpoint-1" || discard.Run.ID != "run-1" {
		t.Fatalf("discard = %#v args=%q/%q", discard, service.discardRunID, service.discardCheckpointID)
	}
}

func TestRuntimeBridgeForwardsMCPRequestDecision(t *testing.T) {
	t.Parallel()

	service := &recordingRuntimeService{}
	bridge := &RuntimeBridge{service: service}

	resp, err := bridge.DecideMCPRequest(context.Background(), RuntimeMCPRequestDecision{RequestID: "mcp-req-1", Action: "approve"})
	if err != nil {
		t.Fatal(err)
	}
	if service.mcpRequestDecision.RequestID != "mcp-req-1" || service.mcpRequestDecision.Action != "approve" {
		t.Fatalf("mcp decision = %#v", service.mcpRequestDecision)
	}
	if resp.Request.ID != "mcp-req-1" || resp.Request.Status != "completed" {
		t.Fatalf("response = %#v", resp)
	}
}

type recordingRuntimeService struct {
	chatCalls                   int
	chatRequests                []RuntimeChatRequest
	refreshedCapability         string
	toolSearchQuery             string
	replayExportRequest         runtime.RuntimeReplayExportRequest
	mcpRequestDecision          runtime.RuntimeMCPRequestDecision
	eventsAfter                 int64
	status                      RuntimeStatus
	activity                    RuntimeSessionActivityResponse
	activityWindow              RuntimeSessionActivityWindowResponse
	turnActivity                RuntimeTurnActivityResponse
	eventsResponse              RuntimeEventsResponse
	newChatTitle                string
	sessionActivityID           string
	sessionActivityWindowID     string
	sessionActivityWindowCursor string
	sessionActivityWindowLimit  int
	turnActivityID              string
	runProjectionRequest        RuntimeRunProjectionRequest
	runProjection               RuntimeRunProjectionResponse
	runs                        RuntimeRunsResponse
	run                         RuntimeRunResponse
	runID                       string
	ackRunID                    string
	ackCheckpointID             string
	discardRunID                string
	discardCheckpointID         string
	markInterruptedDoneID       string
	markInterruptedDoneResponse RuntimeTurnResponse
}

func (s *recordingRuntimeService) Status(context.Context) (RuntimeStatus, error) {
	return s.status, nil
}

func (s *recordingRuntimeService) RecoveryStatus(context.Context) (RuntimeRecoveryStatus, error) {
	return RuntimeRecoveryStatus{}, nil
}

func (s *recordingRuntimeService) Models(context.Context) (RuntimeModelsResponse, error) {
	return RuntimeModelsResponse{}, nil
}

func (s *recordingRuntimeService) SelectedModel(context.Context) (RuntimeSelectedModelResponse, error) {
	return RuntimeSelectedModelResponse{}, nil
}

func (s *recordingRuntimeService) SaveSelectedModel(context.Context, RuntimeSelectedModelRequest) (RuntimeSelectedModelResponse, error) {
	return RuntimeSelectedModelResponse{}, nil
}

func (s *recordingRuntimeService) ProviderCatalog(context.Context) (RuntimeProviderCatalogResponse, error) {
	return RuntimeProviderCatalogResponse{}, nil
}

func (s *recordingRuntimeService) ConfiguredProviders(context.Context) (RuntimeConfiguredProvidersResponse, error) {
	return RuntimeConfiguredProvidersResponse{}, nil
}

func (s *recordingRuntimeService) SaveConfiguredProvider(context.Context, RuntimeConfiguredProviderRequest) (RuntimeConfiguredProviderResponse, error) {
	return RuntimeConfiguredProviderResponse{}, nil
}

func (s *recordingRuntimeService) DeleteConfiguredProvider(context.Context, string) (RuntimeConfiguredProvidersResponse, error) {
	return RuntimeConfiguredProvidersResponse{}, nil
}

func (s *recordingRuntimeService) DiscoverConfiguredProviderModels(context.Context, string) (RuntimeProviderModelDiscoveryResponse, error) {
	return RuntimeProviderModelDiscoveryResponse{}, nil
}

func (s *recordingRuntimeService) TestConfiguredProvider(context.Context, string) (RuntimeProviderTestResponse, error) {
	return RuntimeProviderTestResponse{}, nil
}

func (s *recordingRuntimeService) MeasureConfiguredProviderLatency(context.Context, string) (RuntimeProviderTestResponse, error) {
	return RuntimeProviderTestResponse{}, nil
}

func (s *recordingRuntimeService) GetModelConfig(context.Context) (RuntimeConfigResponse, error) {
	return RuntimeConfigResponse{}, nil
}

func (s *recordingRuntimeService) SaveModelConfig(context.Context, RuntimeModelConfig) (RuntimeConfigResponse, error) {
	return RuntimeConfigResponse{}, nil
}

func (s *recordingRuntimeService) DiscoverModelConfig(context.Context, RuntimeModelConfig) (RuntimeModelDiscoveryResponse, error) {
	return RuntimeModelDiscoveryResponse{}, nil
}

func (s *recordingRuntimeService) VerifyModelConfig(context.Context, RuntimeModelConfig) (RuntimeModelVerifyResponse, error) {
	return RuntimeModelVerifyResponse{}, nil
}

func (s *recordingRuntimeService) Chat(_ context.Context, req RuntimeChatRequest) (RuntimeChatResponse, error) {
	s.chatCalls++
	s.chatRequests = append(s.chatRequests, req)
	return RuntimeChatResponse{RequestID: "request-1", TurnID: "turn-new", Status: RuntimeStatus{SessionID: "session-new"}}, nil
}

func (s *recordingRuntimeService) Turn(context.Context, string) (RuntimeTurnResponse, error) {
	return RuntimeTurnResponse{}, nil
}

func (s *recordingRuntimeService) Turns(context.Context, string) (RuntimeTurnsResponse, error) {
	return RuntimeTurnsResponse{}, nil
}

func (s *recordingRuntimeService) Runs(context.Context) (RuntimeRunsResponse, error) {
	return s.runs, nil
}

func (s *recordingRuntimeService) Run(_ context.Context, runID string) (RuntimeRunResponse, error) {
	s.runID = runID
	return s.run, nil
}

func (s *recordingRuntimeService) AcknowledgeRunCheckpoint(_ context.Context, runID, checkpointID string) (RuntimeRunResponse, error) {
	s.ackRunID = runID
	s.ackCheckpointID = checkpointID
	return s.run, nil
}

func (s *recordingRuntimeService) DiscardRunCheckpoint(_ context.Context, runID, checkpointID string) (RuntimeRunResponse, error) {
	s.discardRunID = runID
	s.discardCheckpointID = checkpointID
	return s.run, nil
}

func (s *recordingRuntimeService) ToolCall(context.Context, string) (RuntimeToolCallResponse, error) {
	return RuntimeToolCallResponse{}, nil
}

func (s *recordingRuntimeService) TurnToolCalls(context.Context, string) (RuntimeToolCallsResponse, error) {
	return RuntimeToolCallsResponse{}, nil
}

func (s *recordingRuntimeService) Hooks(context.Context) (RuntimeHooksResponse, error) {
	return RuntimeHooksResponse{}, nil
}

func (s *recordingRuntimeService) HookExecutions(context.Context, RuntimeHookExecutionsRequest) (RuntimeHookExecutionsResponse, error) {
	return RuntimeHookExecutionsResponse{}, nil
}

func (s *recordingRuntimeService) HookExecution(context.Context, string) (RuntimeHookExecutionResponse, error) {
	return RuntimeHookExecutionResponse{}, nil
}

func (s *recordingRuntimeService) SandboxDecisions(context.Context, runtime.RuntimeSandboxDecisionListRequest) (runtime.RuntimeSandboxDecisionsResponse, error) {
	return runtime.RuntimeSandboxDecisionsResponse{}, nil
}

func (s *recordingRuntimeService) SandboxDecision(context.Context, string) (runtime.RuntimeSandboxDecisionResponse, error) {
	return runtime.RuntimeSandboxDecisionResponse{}, nil
}

func (s *recordingRuntimeService) Refs(context.Context, RuntimeRefListRequest) (RuntimeRefsResponse, error) {
	return RuntimeRefsResponse{}, nil
}

func (s *recordingRuntimeService) Ref(context.Context, string) (RuntimeRefResponse, error) {
	return RuntimeRefResponse{}, nil
}

func (s *recordingRuntimeService) ReadRefContent(context.Context, string) (RuntimeRefContentResponse, error) {
	return RuntimeRefContentResponse{}, nil
}

func (s *recordingRuntimeService) TurnCompactBoundaries(context.Context, string) (RuntimeCompactBoundariesResponse, error) {
	return RuntimeCompactBoundariesResponse{}, nil
}

func (s *recordingRuntimeService) SessionCompactBoundaries(context.Context, string) (RuntimeCompactBoundariesResponse, error) {
	return RuntimeCompactBoundariesResponse{}, nil
}

func (s *recordingRuntimeService) Worktrees(context.Context) (RuntimeWorktreesResponse, error) {
	return RuntimeWorktreesResponse{}, nil
}

func (s *recordingRuntimeService) Worktree(context.Context, string) (RuntimeWorktreeResponse, error) {
	return RuntimeWorktreeResponse{}, nil
}

func (s *recordingRuntimeService) CreateWorktree(context.Context, RuntimeWorktreeCreateRequest) (RuntimeWorktreeResponse, error) {
	return RuntimeWorktreeResponse{}, nil
}

func (s *recordingRuntimeService) EnterWorktree(context.Context, string, RuntimeWorktreeActionRequest) (RuntimeWorktreeResponse, error) {
	return RuntimeWorktreeResponse{}, nil
}

func (s *recordingRuntimeService) ExitWorktree(context.Context, string, RuntimeWorktreeActionRequest) (RuntimeWorktreeResponse, error) {
	return RuntimeWorktreeResponse{}, nil
}

func (s *recordingRuntimeService) CleanupWorktree(context.Context, string, RuntimeWorktreeActionRequest) (RuntimeWorktreeResponse, error) {
	return RuntimeWorktreeResponse{}, nil
}

func (s *recordingRuntimeService) AgentTask(context.Context, string) (RuntimeAgentTaskResponse, error) {
	return RuntimeAgentTaskResponse{}, nil
}

func (s *recordingRuntimeService) TaskEffectiveScope(context.Context, string) (RuntimeEffectiveScopeResponse, error) {
	return RuntimeEffectiveScopeResponse{}, nil
}

func (s *recordingRuntimeService) TurnAgentTasks(context.Context, string) (RuntimeAgentTasksResponse, error) {
	return RuntimeAgentTasksResponse{}, nil
}

func (s *recordingRuntimeService) CancelAgentTask(context.Context, string) (RuntimeAgentTaskResponse, error) {
	return RuntimeAgentTaskResponse{}, nil
}

func (s *recordingRuntimeService) AgentRoles(context.Context) (RuntimeAgentRolesResponse, error) {
	return RuntimeAgentRolesResponse{}, nil
}

func (s *recordingRuntimeService) AgentRole(context.Context, string) (RuntimeAgentRoleResponse, error) {
	return RuntimeAgentRoleResponse{}, nil
}

func (s *recordingRuntimeService) AgentTaskMessages(context.Context, string) (RuntimeAgentTaskMessagesResponse, error) {
	return RuntimeAgentTaskMessagesResponse{}, nil
}

func (s *recordingRuntimeService) CreateAgentTaskMessage(context.Context, string, RuntimeAgentTaskMessageCreateRequest) (RuntimeAgentTaskMessageResponse, error) {
	return RuntimeAgentTaskMessageResponse{}, nil
}

func (s *recordingRuntimeService) SendAgentTaskFollowUp(context.Context, string, RuntimeAgentTaskMessageCreateRequest) (RuntimeAgentTaskMessageResponse, error) {
	return RuntimeAgentTaskMessageResponse{}, nil
}

func (s *recordingRuntimeService) AgentTaskResult(context.Context, string) (RuntimeAgentTaskResultResponse, error) {
	return RuntimeAgentTaskResultResponse{}, nil
}

func (s *recordingRuntimeService) SessionTodos(context.Context, string) (RuntimeTodosResponse, error) {
	return RuntimeTodosResponse{}, nil
}

func (s *recordingRuntimeService) TurnTodos(context.Context, string) (RuntimeTodosResponse, error) {
	return RuntimeTodosResponse{}, nil
}

func (s *recordingRuntimeService) Sessions(context.Context) (RuntimeSessionsResponse, error) {
	return RuntimeSessionsResponse{}, nil
}

func (s *recordingRuntimeService) Session(context.Context, string) (RuntimeSessionResponse, error) {
	return RuntimeSessionResponse{}, nil
}

func (s *recordingRuntimeService) SelectSession(context.Context, string) (RuntimeStatus, error) {
	return RuntimeStatus{}, nil
}

func (s *recordingRuntimeService) RenameSession(context.Context, RuntimeSessionUpdateRequest) (RuntimeSessionsResponse, error) {
	return RuntimeSessionsResponse{}, nil
}

func (s *recordingRuntimeService) DeleteSession(context.Context, string) (RuntimeSessionsResponse, error) {
	return RuntimeSessionsResponse{}, nil
}

func (s *recordingRuntimeService) SessionMessages(context.Context, string) (RuntimeMessagesResponse, error) {
	return RuntimeMessagesResponse{}, nil
}

func (s *recordingRuntimeService) SessionActivity(_ context.Context, sessionID string) (RuntimeSessionActivityResponse, error) {
	s.sessionActivityID = sessionID
	return s.activity, nil
}

func (s *recordingRuntimeService) SessionActivityWindow(_ context.Context, sessionID string, limit int) (RuntimeSessionActivityWindowResponse, error) {
	return s.SessionActivityCursorWindow(context.Background(), sessionID, "", limit)
}

func (s *recordingRuntimeService) SessionActivityCursorWindow(_ context.Context, sessionID string, cursor string, limit int) (RuntimeSessionActivityWindowResponse, error) {
	s.sessionActivityWindowID = sessionID
	s.sessionActivityWindowCursor = cursor
	s.sessionActivityWindowLimit = limit
	if s.activityWindow.SessionID == "" {
		s.activityWindow.SessionID = sessionID
	}
	return s.activityWindow, nil
}

func (s *recordingRuntimeService) TurnActivity(_ context.Context, turnID string) (RuntimeTurnActivityResponse, error) {
	s.turnActivityID = turnID
	if s.turnActivity.TurnID == "" {
		s.turnActivity.TurnID = turnID
	}
	return s.turnActivity, nil
}

func (s *recordingRuntimeService) RunProjection(_ context.Context, req RuntimeRunProjectionRequest) (RuntimeRunProjectionResponse, error) {
	s.runProjectionRequest = req
	if s.runProjection.Run.PrimarySessionID == "" {
		s.runProjection.Run.PrimarySessionID = req.SessionID
	}
	return s.runProjection, nil
}

func (s *recordingRuntimeService) Messages(context.Context) (RuntimeMessagesResponse, error) {
	return RuntimeMessagesResponse{}, nil
}

func (s *recordingRuntimeService) Permissions(context.Context) (RuntimePermissionsResponse, error) {
	return RuntimePermissionsResponse{}, nil
}

func (s *recordingRuntimeService) GetPolicy(context.Context) (RuntimePolicyResponse, error) {
	return RuntimePolicyResponse{}, nil
}

func (s *recordingRuntimeService) UpdatePolicy(context.Context, RuntimePolicyUpdateRequest) (RuntimePolicyResponse, error) {
	return RuntimePolicyResponse{}, nil
}

func (s *recordingRuntimeService) Events(_ context.Context, afterValues ...int64) (RuntimeEventsResponse, error) {
	if len(afterValues) > 0 {
		s.eventsAfter = afterValues[0]
	}
	return s.eventsResponse, nil
}

func (s *recordingRuntimeService) EventsEndpoint(context.Context) (RuntimeEventsEndpointResponse, error) {
	return RuntimeEventsEndpointResponse{}, nil
}

func (s *recordingRuntimeService) SubscribeEvents(context.Context, ...int64) (<-chan RuntimeEvent, func()) {
	events := make(chan RuntimeEvent)
	return events, func() {
		close(events)
	}
}

func (s *recordingRuntimeService) AuditTurn(context.Context, string) (RuntimeAuditResponse, error) {
	return RuntimeAuditResponse{}, nil
}

func (s *recordingRuntimeService) AuditSession(context.Context, string) (RuntimeAuditResponse, error) {
	return RuntimeAuditResponse{}, nil
}

func (s *recordingRuntimeService) ReplayExport(_ context.Context, req runtime.RuntimeReplayExportRequest) (runtime.RuntimeReplayExportResponse, error) {
	s.replayExportRequest = req
	return runtime.RuntimeReplayExportResponse{TurnID: req.TurnID, Summary: runtime.RuntimeReplayExportSummary{Redacted: true}}, nil
}

func (s *recordingRuntimeService) Skills(context.Context) (RuntimeSkillsResponse, error) {
	return RuntimeSkillsResponse{}, nil
}

func (s *recordingRuntimeService) Plugins(context.Context) (RuntimePluginsResponse, error) {
	return RuntimePluginsResponse{}, nil
}

func (s *recordingRuntimeService) RefreshSkills(context.Context) (RuntimeSkillsResponse, error) {
	return RuntimeSkillsResponse{}, nil
}

func (s *recordingRuntimeService) CreateSkill(context.Context, RuntimeSkillCreateRequest) (RuntimeSkillsResponse, error) {
	return RuntimeSkillsResponse{}, nil
}

func (s *recordingRuntimeService) AddSkillPath(context.Context, RuntimeSkillPathRequest) (RuntimeSkillsResponse, error) {
	return RuntimeSkillsResponse{}, nil
}

func (s *recordingRuntimeService) SetSkillEnabled(context.Context, RuntimeSkillToggleRequest) (RuntimeSkillsResponse, error) {
	return RuntimeSkillsResponse{}, nil
}

func (s *recordingRuntimeService) MCPServers(context.Context) (RuntimeMCPServersResponse, error) {
	return RuntimeMCPServersResponse{}, nil
}

func (s *recordingRuntimeService) SaveMCPServer(context.Context, RuntimeMCPServerConfigRequest) (RuntimeMCPServersResponse, error) {
	return RuntimeMCPServersResponse{}, nil
}

func (s *recordingRuntimeService) SetMCPServerEnabled(context.Context, RuntimeMCPServerToggleRequest) (RuntimeMCPServersResponse, error) {
	return RuntimeMCPServersResponse{}, nil
}

func (s *recordingRuntimeService) RefreshMCPServer(context.Context, string) (RuntimeMCPServersResponse, error) {
	return RuntimeMCPServersResponse{}, nil
}

func (s *recordingRuntimeService) SetMCPToolEnabled(context.Context, RuntimeMCPToolToggleRequest) (RuntimeMCPToolsResponse, error) {
	return RuntimeMCPToolsResponse{}, nil
}

func (s *recordingRuntimeService) MCPTools(context.Context, string) (RuntimeMCPToolsResponse, error) {
	return RuntimeMCPToolsResponse{}, nil
}

func (s *recordingRuntimeService) MCPResources(context.Context, string) (RuntimeMCPResourcesResponse, error) {
	return RuntimeMCPResourcesResponse{}, nil
}

func (s *recordingRuntimeService) MCPPrompts(context.Context, string) (RuntimeMCPPromptsResponse, error) {
	return RuntimeMCPPromptsResponse{}, nil
}

func (s *recordingRuntimeService) MCPRequests(context.Context, RuntimeMCPRequestListRequest) (RuntimeMCPRequestsResponse, error) {
	return RuntimeMCPRequestsResponse{}, nil
}

func (s *recordingRuntimeService) MCPRequest(context.Context, string) (RuntimeMCPRequestResponse, error) {
	return RuntimeMCPRequestResponse{}, nil
}

func (s *recordingRuntimeService) DecideMCPRequest(_ context.Context, req RuntimeMCPRequestDecision) (RuntimeMCPRequestResponse, error) {
	s.mcpRequestDecision = req
	return RuntimeMCPRequestResponse{Request: RuntimeMCPRequest{ID: req.RequestID, Status: "completed", Redacted: true}}, nil
}

func (s *recordingRuntimeService) RetryMCPServer(context.Context, string) (RuntimeMCPServersResponse, error) {
	return RuntimeMCPServersResponse{}, nil
}

func (s *recordingRuntimeService) Capabilities(context.Context) (RuntimeCapabilitiesResponse, error) {
	return RuntimeCapabilitiesResponse{}, nil
}

func (s *recordingRuntimeService) RefreshCapability(_ context.Context, id string) (RuntimeCapabilityResponse, error) {
	s.refreshedCapability = id
	return RuntimeCapabilityResponse{Capability: RuntimeCapability{ID: id, Kind: "skill", Name: "docs", Enabled: true, State: "loaded"}}, nil
}

func (s *recordingRuntimeService) SearchTools(_ context.Context, req RuntimeToolSearchRequest) (RuntimeToolSearchResponse, error) {
	s.toolSearchQuery = req.Query
	return RuntimeToolSearchResponse{Query: req.Query}, nil
}

func (s *recordingRuntimeService) ContextSources(context.Context) (RuntimeContextSourcesResponse, error) {
	return RuntimeContextSourcesResponse{}, nil
}

func (s *recordingRuntimeService) ReadFiles(context.Context, string) (RuntimeReadFilesResponse, error) {
	return RuntimeReadFilesResponse{}, nil
}

func (s *recordingRuntimeService) APIEndpoint(context.Context) (RuntimeAPIEndpointResponse, error) {
	return RuntimeAPIEndpointResponse{}, nil
}

func (s *recordingRuntimeService) ServeHTTP(context.Context, string, string) (RuntimeAPIEndpointResponse, error) {
	return RuntimeAPIEndpointResponse{}, nil
}

func (s *recordingRuntimeService) DecidePermission(context.Context, RuntimePermissionDecision) (RuntimeStatus, error) {
	return RuntimeStatus{}, nil
}

func (s *recordingRuntimeService) Cancel(context.Context) (RuntimeStatus, error) {
	return RuntimeStatus{}, nil
}

func (s *recordingRuntimeService) CancelTurn(context.Context, string) (RuntimeStatus, error) {
	return RuntimeStatus{}, nil
}

func (s *recordingRuntimeService) MarkInterruptedDone(_ context.Context, turnID string) (RuntimeTurnResponse, error) {
	s.markInterruptedDoneID = turnID
	return s.markInterruptedDoneResponse, nil
}

func (s *recordingRuntimeService) NewChat(_ context.Context, title string) (RuntimeStatus, error) {
	s.newChatTitle = title
	return s.status, nil
}

var _ runtime.RuntimeService = (*recordingRuntimeService)(nil)
