package main

import (
	"context"
	"testing"

	runtime "github.com/CIPFZ/agent-builder/internal/runtime"
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

func TestRuntimeBridgeForwardsUserInput(t *testing.T) {
	t.Parallel()

	service := &recordingRuntimeService{
		userInputResponse: RuntimeChatResponse{
			RequestID: "turn-1",
			TurnID:    "turn-1",
			Status:    RuntimeStatus{SessionID: "session-1"},
			NormalizedInput: &RuntimeNormalizedInput{
				ID:        "input-1",
				SessionID: "session-1",
				Mode:      "prompt",
			},
		},
		userInput: RuntimeNormalizedInput{ID: "input-1", SessionID: "session-1", Mode: "prompt"},
	}
	bridge := &RuntimeBridge{service: service}

	submitted, err := bridge.SubmitUserInput(context.Background(), RuntimeUserInputRequest{
		SessionID: "session-1",
		Mode:      "prompt",
		Items:     []RuntimeUserInputItem{{Type: "text", Text: "hello"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if service.userInputReq.SessionID != "session-1" || service.userInputReq.Mode != "prompt" {
		t.Fatalf("submit request = %#v", service.userInputReq)
	}
	if submitted.NormalizedInput == nil || submitted.NormalizedInput.ID != "input-1" {
		t.Fatalf("submitted = %#v", submitted)
	}

	read, err := bridge.UserInput(context.Background(), "input-1")
	if err != nil {
		t.Fatal(err)
	}
	if service.userInputID != "input-1" || read.ID != "input-1" {
		t.Fatalf("read = %#v service id=%q", read, service.userInputID)
	}
}

func TestRuntimeBridgeForwardsSessionContextUsage(t *testing.T) {
	t.Parallel()

	service := &recordingRuntimeService{
		contextUsage: RuntimeContextUsage{SessionID: "session-1", UsedTokens: 123, PercentUsed: 12},
	}
	bridge := &RuntimeBridge{service: service}

	usage, err := bridge.SessionContextUsage(context.Background(), "session-1")
	if err != nil {
		t.Fatal(err)
	}
	if service.contextUsageSessionID != "session-1" || usage.UsedTokens != 123 {
		t.Fatalf("usage=%#v session=%q", usage, service.contextUsageSessionID)
	}
}

func TestRuntimeBridgeForwardsContextCompactionStatus(t *testing.T) {
	t.Parallel()
	service := &recordingRuntimeService{contextCompactionStatus: RuntimeContextCompactionStatus{SessionID: "session-1", CircuitOpen: true}}
	bridge := &RuntimeBridge{service: service}
	status, err := bridge.ContextCompactionStatus(context.Background(), "session-1")
	if err != nil {
		t.Fatal(err)
	}
	if status.SessionID != "session-1" || !status.CircuitOpen {
		t.Fatalf("status = %#v", status)
	}
}

func TestRuntimeBridgeForwardsContextGovernanceSettings(t *testing.T) {
	t.Parallel()

	pct := 0.7
	service := &recordingRuntimeService{
		contextGovernanceSettings: RuntimeContextGovernanceSettings{AutoCompactPercent: &pct},
	}
	bridge := &RuntimeBridge{service: service}

	got, err := bridge.ContextGovernanceSettings(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if got.Settings.AutoCompactPercent == nil || *got.Settings.AutoCompactPercent != 0.7 {
		t.Fatalf("settings = %#v", got.Settings)
	}

	disabled := false
	saved, err := bridge.SaveContextGovernanceSettings(context.Background(), RuntimeContextGovernanceSettings{
		AutoCompactEnabled: &disabled,
	})
	if err != nil {
		t.Fatal(err)
	}
	if service.contextGovernanceSaveCalls != 1 {
		t.Fatalf("save calls = %d, want 1", service.contextGovernanceSaveCalls)
	}
	if service.contextGovernanceSaveReq.AutoCompactEnabled == nil || *service.contextGovernanceSaveReq.AutoCompactEnabled != false {
		t.Fatalf("save request = %#v", service.contextGovernanceSaveReq)
	}
	if saved.Settings.AutoCompactEnabled == nil || *saved.Settings.AutoCompactEnabled != false {
		t.Fatalf("save response = %#v", saved.Settings)
	}
}

func TestRuntimeBridgeForwardsReactCallchain(t *testing.T) {
	t.Parallel()

	service := &recordingRuntimeService{
		reactCallchain: RuntimeReactCallchainResponse{
			SessionID: "session-1",
			TurnID:    "turn-1",
			Summary: RuntimeReactCallSummary{
				StopReason:        "model_stop",
				StopReasonMessage: "Tool result delivered; final response is empty.",
			},
			ToolResultDeliveries: []RuntimeToolResultDelivery{{
				ToolCallID:          "tool-1",
				ToolResultMessageID: "msg-tool",
				DeliveredToModel:    true,
				DeliveredAtStep:     2,
			}},
		},
		sessionReactCallchain: RuntimeReactCallchainResponse{SessionID: "session-1"},
	}
	bridge := &RuntimeBridge{service: service}

	turnResp, err := bridge.ReactCallchain(context.Background(), "turn-1")
	if err != nil {
		t.Fatal(err)
	}
	if service.reactCallchainTurnID != "turn-1" || turnResp.TurnID != "turn-1" {
		t.Fatalf("turn callchain = %#v service turn=%q", turnResp, service.reactCallchainTurnID)
	}
	if turnResp.Summary.StopReasonMessage == "" || len(turnResp.ToolResultDeliveries) != 1 || !turnResp.ToolResultDeliveries[0].DeliveredToModel {
		t.Fatalf("turn delivery = %#v summary=%#v", turnResp.ToolResultDeliveries, turnResp.Summary)
	}

	sessionResp, err := bridge.SessionReactCallchain(context.Background(), "session-1", 5)
	if err != nil {
		t.Fatal(err)
	}
	if service.sessionReactCallchainID != "session-1" || service.sessionReactCallchainLimit != 5 || sessionResp.SessionID != "session-1" {
		t.Fatalf("session callchain = %#v service session=%q limit=%d", sessionResp, service.sessionReactCallchainID, service.sessionReactCallchainLimit)
	}
}

func TestRuntimeBridgeForwardsPromptAssemblies(t *testing.T) {
	t.Parallel()

	service := &recordingRuntimeService{
		promptAssemblies: RuntimePromptAssembliesResponse{Assemblies: []RuntimePromptAssembly{{
			ID:        "assembly-1",
			SessionID: "session-1",
			TurnID:    "turn-1",
			Step:      1,
			Provider:  "openai",
			Model:     "test-model",
			System: RuntimePromptSystemSummary{
				Hash:     "sha256:system",
				Redacted: true,
			},
			Messages: RuntimePromptMessageSummary{
				Count:           2,
				RawPromptStored: false,
			},
			Tools: RuntimePromptToolSummary{
				Selected:      []string{"bash"},
				SelectedCount: 1,
			},
			Skills: RuntimePromptSkillSummary{
				LoadedNames:      []string{"agent-builder-config"},
				LoadedCount:      1,
				RawContentStored: false,
			},
			MCP: RuntimePromptMCPSummary{
				Servers:          []string{"docs"},
				ServerCount:      1,
				ServerListHash:   "sha256:mcp-servers",
				RawContentStored: false,
			},
			ContextSources: []RuntimeContextSource{{
				ID:      "ctx-1",
				Kind:    "project_memory",
				Name:    "AGENTS.md",
				Enabled: true,
				State:   "loaded",
			}},
			Budget: RuntimeBudgetReport{
				TotalEstimatedTokens: 42,
			},
		}}},
	}
	bridge := &RuntimeBridge{service: service}

	turnResp, err := bridge.TurnPromptAssemblies(context.Background(), "turn-1")
	if err != nil {
		t.Fatal(err)
	}
	if service.promptAssembliesTurnID != "turn-1" || len(turnResp.Assemblies) != 1 || turnResp.Assemblies[0].Messages.RawPromptStored {
		t.Fatalf("turn assemblies = %#v service turn=%q", turnResp, service.promptAssembliesTurnID)
	}

	sessionResp, err := bridge.SessionPromptAssemblies(context.Background(), "session-1", 4)
	if err != nil {
		t.Fatal(err)
	}
	if service.promptAssembliesSessionID != "session-1" || service.promptAssembliesLimit != 4 || len(sessionResp.Assemblies) != 1 {
		t.Fatalf("session assemblies = %#v service session=%q limit=%d", sessionResp, service.promptAssembliesSessionID, service.promptAssembliesLimit)
	}
}

func TestRuntimeBridgeForwardsTodoQueries(t *testing.T) {
	t.Parallel()

	service := &recordingRuntimeService{
		todos: RuntimeTodosResponse{Summary: runtime.RuntimeTodoSummary{
			SessionID: "session-1",
			Todos: []runtime.RuntimeTodo{{
				Content:    "Inspect plan",
				Status:     "in_progress",
				ActiveForm: "Inspecting plan",
			}},
			InProgress: 1,
			Total:      1,
		}},
	}
	bridge := &RuntimeBridge{service: service}

	sessionResp, err := bridge.SessionTodos(context.Background(), "session-1")
	if err != nil {
		t.Fatal(err)
	}
	if service.todoSessionID != "session-1" || sessionResp.Summary.Total != 1 {
		t.Fatalf("session todos = %#v service session=%q", sessionResp, service.todoSessionID)
	}

	turnResp, err := bridge.TurnTodos(context.Background(), "turn-1")
	if err != nil {
		t.Fatal(err)
	}
	if service.todoTurnID != "turn-1" || turnResp.Summary.Total != 1 {
		t.Fatalf("turn todos = %#v service turn=%q", turnResp, service.todoTurnID)
	}
}

func TestSessionConversationSnapshotV2ForwardsToRuntime(t *testing.T) {
	service := &recordingRuntimeService{}
	bridge := &RuntimeBridge{service: service}
	req := RuntimeCanonicalConversationSnapshotRequest{Scope: runtime.RuntimeConversationScopeWindow, Limit: 12, Before: "turn-20"}
	snapshot, err := bridge.SessionConversationSnapshotV2(context.Background(), "session-v2", req)
	if err != nil {
		t.Fatal(err)
	}
	if service.conversationSnapshotSessionID != "session-v2" || service.conversationSnapshotRequest != req {
		t.Fatalf("request not forwarded: session=%q req=%#v", service.conversationSnapshotSessionID, service.conversationSnapshotRequest)
	}
	if snapshot.SessionID != "session-v2" {
		t.Fatalf("snapshot session = %q", snapshot.SessionID)
	}
}

func TestSessionConversationEventsV2ForwardsToRuntime(t *testing.T) {
	service := &recordingRuntimeService{}
	bridge := &RuntimeBridge{service: service}
	resp, err := bridge.SessionConversationEventsV2(context.Background(), "session-v2", RuntimeCanonicalConversationEventsRequestV2{After: "9007199254740993", LimitRawEvents: 7})
	if err != nil {
		t.Fatal(err)
	}
	if resp.SessionID != "session-v2" || resp.Cursor != "9007199254740993" {
		t.Fatalf("response=%#v", resp)
	}
}

func TestSessionConversationStreamV2ReplacesSessionAndPreservesAtomicBatch(t *testing.T) {
	bridge := &RuntimeBridge{}
	cancelled := 0
	bridge.installSessionConversationStreamV2("old", &runtimeBridgeOutputStream{sessionID: "session-1", cancel: func() { cancelled++ }})
	bridge.installSessionConversationStreamV2("new", &runtimeBridgeOutputStream{sessionID: "session-1", cancel: func() {}})
	if cancelled != 1 || bridge.conversationV2StreamBySession["session-1"] != "new" || bridge.conversationV2Streams["old"] != nil {
		t.Fatalf("replacement maps=%#v cancelled=%d", bridge.conversationV2StreamBySession, cancelled)
	}
	events := make([]runtime.RuntimeConversationEntityEventV2, 100)
	for i := range events {
		events[i].Sequence = "42"
	}
	batch := runtime.RuntimeCanonicalConversationEventBatchV2{SchemaVersion: 2, SessionID: "session-1", AfterCursor: "41", Cursor: "42", Events: events, Deltas: []runtime.RuntimeConversationTextDeltaV2{{MessageID: "message-live", PartType: "text", Delta: "x", ContentLength: 1}}, SnapshotRequired: true, Reason: "overflow"}
	message := RuntimeCanonicalConversationStreamMessageV2{StreamID: "new", RuntimeCanonicalConversationEventBatchV2: batch}
	if len(message.Events) != 100 || len(message.Deltas) != 1 || message.Deltas[0].Delta != "x" || message.Cursor != "42" || !message.SnapshotRequired || message.Reason != "overflow" {
		t.Fatalf("batch changed at bridge: %#v", message)
	}
	if ok := bridge.stopSessionConversationStreamV2("new"); !ok {
		t.Fatal("stop did not find stream")
	}
	if bridge.conversationV2StreamBySession["session-1"] != "" {
		t.Fatal("stop left session mapping")
	}
}

func TestStopRuntimeEventStreamUnknownStreamIsNoop(t *testing.T) {
	bridge := NewRuntimeBridge()
	ok, err := bridge.StopRuntimeEventStream(context.Background(), runtime.RuntimeEventStreamStopRequest{StreamID: "ghost"})
	if err != nil {
		t.Fatal(err)
	}
	if ok {
		t.Fatal("unknown runtime event stream should not be stopped")
	}
}

func TestRuntimeBridgeForwardsTerminalLifecycle(t *testing.T) {
	t.Parallel()

	service := &recordingRuntimeService{
		terminalResponse: RuntimeTerminalResponse{Terminal: RuntimeTerminal{
			ID:        "term-1",
			SessionID: "session-1",
			CWD:       "C:\\work",
			Shell:     "PowerShell",
			Status:    "running",
		}},
		sessionTerminals: RuntimeSessionTerminalsResponse{
			SessionID: "session-1",
			Terminals: []RuntimeTerminal{{ID: "term-1", SessionID: "session-1"}},
		},
	}
	bridge := &RuntimeBridge{service: service}

	if _, err := bridge.CreateTerminal(context.Background(), RuntimeTerminalCreateRequest{SessionID: "session-1", ID: "term-1", CWD: "C:\\work"}); err != nil {
		t.Fatal(err)
	}
	if service.createdTerminal.SessionID != "session-1" || service.createdTerminal.ID != "term-1" || service.createdTerminal.CWD != "C:\\work" {
		t.Fatalf("created terminal = %#v", service.createdTerminal)
	}

	list, err := bridge.SessionTerminals(context.Background(), "session-1")
	if err != nil {
		t.Fatal(err)
	}
	if service.sessionTerminalsID != "session-1" || len(list.Terminals) != 1 || list.Terminals[0].SessionID != "session-1" {
		t.Fatalf("session terminals = %#v serviceID=%q", list, service.sessionTerminalsID)
	}

	if _, err := bridge.DeleteTerminal(context.Background(), "term-1"); err != nil {
		t.Fatal(err)
	}
	if service.deletedTerminalID != "term-1" {
		t.Fatalf("deleted terminal = %q", service.deletedTerminalID)
	}
}

func TestRuntimeBridgeForwardsCreateSession(t *testing.T) {
	t.Parallel()

	service := &recordingRuntimeService{}
	bridge := &RuntimeBridge{service: service}

	resp, err := bridge.CreateSession(context.Background(), RuntimeSessionCreateRequest{Title: "New runtime chat"})
	if err != nil {
		t.Fatal(err)
	}
	if service.createSessionReq.Title != "New runtime chat" {
		t.Fatalf("create session req = %#v", service.createSessionReq)
	}
	if resp.Session.ID == "" || !resp.Session.Active {
		t.Fatalf("create session response = %#v", resp)
	}
}

func TestRuntimeBridgeForwardsOpenProject(t *testing.T) {
	t.Parallel()

	service := &recordingRuntimeService{
		openProject: RuntimeOpenProjectResponse{
			Project: RuntimeProject{ID: "workspace-1", Name: "repo", Path: "C:\\work\\repo", Current: true},
			Status:  RuntimeStatus{WorkspaceID: "workspace-1", WorkingDir: "C:\\work\\repo", Ready: true},
		},
	}
	bridge := &RuntimeBridge{service: service}

	resp, err := bridge.OpenProject(context.Background(), RuntimeOpenProjectRequest{Path: "C:\\work\\repo", CreateMissing: true})
	if err != nil {
		t.Fatal(err)
	}
	if service.openProjectReq.Path != "C:\\work\\repo" || !service.openProjectReq.CreateMissing {
		t.Fatalf("open project request = %#v", service.openProjectReq)
	}
	if resp.Project.ID != "workspace-1" || resp.Status.WorkingDir != "C:\\work\\repo" {
		t.Fatalf("open project response = %#v", resp)
	}
}

func TestRuntimeBridgeForwardsCreateProject(t *testing.T) {
	t.Parallel()

	service := &recordingRuntimeService{
		openProject: RuntimeOpenProjectResponse{
			Project: RuntimeProject{ID: "workspace-1", Name: "Blank", Path: "C:\\app\\data\\projects\\Blank", Current: true},
			Status:  RuntimeStatus{WorkspaceID: "workspace-1", WorkingDir: "C:\\app\\data\\projects\\Blank", Ready: true},
		},
	}
	bridge := &RuntimeBridge{service: service}

	resp, err := bridge.CreateProject(context.Background(), RuntimeCreateProjectRequest{Name: "Blank"})
	if err != nil {
		t.Fatal(err)
	}
	if service.createProjectReq.Name != "Blank" {
		t.Fatalf("create project request = %#v", service.createProjectReq)
	}
	if resp.Project.ID != "workspace-1" || resp.Project.Name != "Blank" {
		t.Fatalf("create project response = %#v", resp)
	}
}

func TestRuntimeBridgeForwardsRenameProject(t *testing.T) {
	service := &recordingRuntimeService{
		openProject: RuntimeOpenProjectResponse{
			Project: RuntimeProject{ID: "workspace-1", Name: "Renamed", Path: "C:\\app\\data\\projects\\Renamed", Current: true},
			Status:  RuntimeStatus{WorkspaceID: "workspace-1", WorkingDir: "C:\\app\\data\\projects\\Renamed"},
		},
	}
	bridge := RuntimeBridge{service: service}

	resp, err := bridge.RenameProject(context.Background(), RuntimeRenameProjectRequest{ProjectID: "workspace-1", Name: "Renamed"})
	if err != nil {
		t.Fatal(err)
	}
	if service.renameProjectReq.ProjectID != "workspace-1" || service.renameProjectReq.Name != "Renamed" {
		t.Fatalf("rename project request = %#v", service.renameProjectReq)
	}
	if resp.Project.ID != "workspace-1" || resp.Project.Name != "Renamed" {
		t.Fatalf("rename project response = %#v", resp)
	}
}

func TestRuntimeBridgeForwardsOpenProjectInExplorer(t *testing.T) {
	service := &recordingRuntimeService{
		openProject: RuntimeOpenProjectResponse{
			Project: RuntimeProject{ID: "workspace-1", Name: "repo", Path: "C:\\work\\repo", Current: true},
			Status:  RuntimeStatus{WorkspaceID: "workspace-1", WorkingDir: "C:\\work\\repo"},
		},
	}
	bridge := RuntimeBridge{service: service}

	resp, err := bridge.OpenProjectInExplorer(context.Background(), RuntimeProjectActionRequest{ProjectID: "workspace-1"})
	if err != nil {
		t.Fatal(err)
	}
	if service.openProjectInExplorerReq.ProjectID != "workspace-1" {
		t.Fatalf("open explorer request = %#v", service.openProjectInExplorerReq)
	}
	if resp.Project.ID != "workspace-1" || resp.Project.Name != "repo" {
		t.Fatalf("open explorer response = %#v", resp)
	}
}

func TestRuntimeBridgeForwardsRemoveProject(t *testing.T) {
	service := &recordingRuntimeService{
		openProject: RuntimeOpenProjectResponse{
			Project: RuntimeProject{ID: "workspace-next", Name: "next", Path: "C:\\work\\next", Current: true},
			Status:  RuntimeStatus{WorkspaceID: "workspace-next", WorkingDir: "C:\\work\\next"},
		},
	}
	bridge := RuntimeBridge{service: service}

	resp, err := bridge.RemoveProject(context.Background(), RuntimeProjectActionRequest{ProjectID: "workspace-1"})
	if err != nil {
		t.Fatal(err)
	}
	if service.removeProjectReq.ProjectID != "workspace-1" {
		t.Fatalf("remove project request = %#v", service.removeProjectReq)
	}
	if resp.Project.ID != "workspace-next" || resp.Project.Name != "next" {
		t.Fatalf("remove project response = %#v", resp)
	}
}

func TestRuntimeBridgeSelectProjectDirectoryRequiresDesktopApp(t *testing.T) {
	t.Parallel()

	bridge := &RuntimeBridge{service: &recordingRuntimeService{}}
	if path, err := bridge.SelectProjectDirectory(context.Background()); err == nil || path != "" {
		t.Fatalf("SelectProjectDirectory path=%q err=%v, want empty path and error outside desktop app", path, err)
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

func TestRuntimeBridgeForwardsRecoveryActions(t *testing.T) {
	t.Parallel()

	service := &recordingRuntimeService{}
	bridge := &RuntimeBridge{service: service}

	resumed, err := bridge.ResumeInterruptedTurn(context.Background(), "turn-resume", RuntimeResumeInterruptedTurnRequest{Mode: "continue", Prompt: "keep going"})
	if err != nil {
		t.Fatal(err)
	}
	if service.resumeInterruptedTurnID != "turn-resume" || service.resumeInterruptedTurnReq.Prompt != "keep going" || resumed.Turn.ID != "turn-resume" {
		t.Fatalf("resume = %#v id=%q req=%#v", resumed, service.resumeInterruptedTurnID, service.resumeInterruptedTurnReq)
	}

	discarded, err := bridge.DiscardInterruptedTurn(context.Background(), "turn-discard")
	if err != nil {
		t.Fatal(err)
	}
	if service.discardInterruptedTurnID != "turn-discard" || discarded.Turn.Status != "cancelled" {
		t.Fatalf("discard = %#v id=%q", discarded, service.discardInterruptedTurnID)
	}

	retried, err := bridge.RetryRecoverableError(context.Background(), "turn:turn-failed")
	if err != nil {
		t.Fatal(err)
	}
	if service.retryRecoverableErrorID != "turn:turn-failed" || retried.ErrorID != "turn:turn-failed" {
		t.Fatalf("retry = %#v id=%q", retried, service.retryRecoverableErrorID)
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
		transitionHistory: RuntimeRunTransitionHistoryResponse{
			Transitions: []RuntimeRunTransition{{
				ID:        "transition-1",
				RunID:     "run-1",
				SessionID: "session-window",
				TurnID:    "turn-window",
				ToStatus:  "completed",
				Source:    "turn_finished",
				CreatedAt: 1200,
			}},
			Window: RuntimeActivityWindow{Limit: 5, ToEnd: true},
			Source: RuntimeRunTransitionHistorySource{
				Kind:      "run_transition_history",
				ReadOnly:  true,
				AuditOnly: true,
			},
		},
		runSchedulerPlan: RuntimeRunSchedulerPlanResponse{
			Plan: RuntimeRunSchedulerPlan{
				RunID:            "run-1",
				PrimarySessionID: "session-window",
				Items: []RuntimeRunSchedulerPlanItem{{
					ID:                "task:task-1",
					Kind:              "task_turn",
					SessionID:         "session-window",
					TurnID:            "turn-window",
					TaskID:            "task-1",
					CanSchedule:       true,
					OwnershipVerified: true,
					RequiredPreflight: true,
				}},
			},
			Source: RuntimeRunSchedulerPlanSource{
				Kind:                  "run_scheduler_plan",
				ReadOnly:              true,
				StartsWorker:          false,
				SessionActivityParity: true,
			},
		},
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

	transitionHistory, err := bridge.RunTransitionHistory(context.Background(), RuntimeRunTransitionHistoryRequest{RunID: "run-1", Cursor: "v1:transition", Limit: 5})
	if err != nil {
		t.Fatal(err)
	}
	if service.transitionHistoryReq.RunID != "run-1" || service.transitionHistoryReq.Cursor != "v1:transition" || service.transitionHistoryReq.Limit != 5 {
		t.Fatalf("transition history request = %#v", service.transitionHistoryReq)
	}
	if len(transitionHistory.Transitions) != 1 || !transitionHistory.Source.ReadOnly || !transitionHistory.Source.AuditOnly {
		t.Fatalf("transition history = %#v", transitionHistory)
	}

	schedulerPlan, err := bridge.RunSchedulerPlan(context.Background(), RuntimeRunSchedulerPlanRequest{RunID: "run-1", TaskID: "task-1", Cursor: "v1:plan", Limit: 6})
	if err != nil {
		t.Fatal(err)
	}
	if service.runSchedulerPlanReq.RunID != "run-1" || service.runSchedulerPlanReq.TaskID != "task-1" || service.runSchedulerPlanReq.Cursor != "v1:plan" || service.runSchedulerPlanReq.Limit != 6 {
		t.Fatalf("scheduler plan request = %#v", service.runSchedulerPlanReq)
	}
	if len(schedulerPlan.Plan.Items) != 1 || !schedulerPlan.Source.ReadOnly || schedulerPlan.Source.StartsWorker || !schedulerPlan.Source.SessionActivityParity {
		t.Fatalf("scheduler plan = %#v", schedulerPlan)
	}

	execute, err := bridge.ExecuteRunTask(context.Background(), "run-1", "task-1")
	if err != nil {
		t.Fatal(err)
	}
	if service.executeRunID != "run-1" || service.executeTaskID != "task-1" {
		t.Fatalf("execute args = %q/%q", service.executeRunID, service.executeTaskID)
	}
	if execute.Source.StartsWorker || !execute.Source.WorkbenchOnly || !execute.Source.IdempotentByTaskID || !execute.Source.SessionActivityParity {
		t.Fatalf("execute response = %#v", execute)
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

	resume, err := bridge.ResumeRunCheckpoint(context.Background(), "run-1", "checkpoint-1")
	if err != nil {
		t.Fatal(err)
	}
	if service.resumeRunID != "run-1" || service.resumeCheckpointID != "checkpoint-1" || resume.TurnID != "turn-resume" {
		t.Fatalf("resume = %#v args=%q/%q", resume, service.resumeRunID, service.resumeCheckpointID)
	}
}

func TestRuntimeBridgePhase311PackagedSchedulerClickContract(t *testing.T) {
	t.Parallel()

	service := &recordingRuntimeService{
		runProjection: RuntimeRunProjectionResponse{
			Run: RuntimeRunProjection{
				ID:               "run-phase311",
				WorkspaceID:      "workspace-phase311",
				PrimarySessionID: "session-phase311",
				SessionIDs:       []string{"session-phase311"},
				TurnIDs:          []string{"turn-phase311"},
				TaskIDs:          []string{"task-phase311-queued", "task-phase311-terminal"},
				Status:           "running",
				EvidenceCursor:   "v1:phase311",
				Source: RuntimeRunProjectionSource{
					Kind:                  "run_projection",
					ReadOnly:              true,
					SessionActivityParity: true,
					Evidence:              []string{"session_activity_subset"},
				},
			},
		},
		runSchedulerPlan: RuntimeRunSchedulerPlanResponse{
			Plan: RuntimeRunSchedulerPlan{
				RunID:            "run-phase311",
				PrimarySessionID: "session-phase311",
				SessionIDs:       []string{"session-phase311"},
				Items: []RuntimeRunSchedulerPlanItem{{
					ID:                "task-phase311-queued",
					Kind:              "agent_task",
					SessionID:         "session-phase311",
					TurnID:            "turn-phase311",
					TaskID:            "task-phase311-queued",
					CanSchedule:       true,
					OwnershipVerified: true,
					RequiredPreflight: true,
					RefreshTargets:    []string{"run_projection", "session_activity_window"},
				}, {
					ID:                "task-phase311-terminal",
					Kind:              "agent_task",
					SessionID:         "session-phase311",
					TurnID:            "turn-phase311",
					TaskID:            "task-phase311-terminal",
					CanSchedule:       false,
					PreflightReason:   "task_status_terminal",
					RequiredPreflight: true,
					RefreshTargets:    []string{"run_projection"},
					CancellationScope: "task",
					DiagnosticsRoute:  "run_projection",
					OwnershipVerified: true,
				}},
			},
			Source: RuntimeRunSchedulerPlanSource{
				Kind:                  "run_scheduler_plan",
				ReadOnly:              true,
				StartsWorker:          false,
				SessionActivityParity: true,
				Evidence:              []string{"runtime_owned_seed"},
			},
		},
		executeRunTask: RuntimeRunSchedulerExecuteTaskResponse{
			Accepted:         true,
			ExecutionStarted: true,
			Task: RuntimeAgentTask{
				ID:              "task-phase311-queued",
				ParentSessionID: "session-phase311",
				ParentTurnID:    "turn-phase311",
				Status:          "running",
			},
			RefreshTargets: []string{"run_projection", "session_activity_window"},
			Source: RuntimeRunSchedulerExecuteTaskSource{
				Kind:                  "run_scheduler_execute_task",
				Action:                "execute_task",
				WorkbenchOnly:         true,
				StartsWorker:          false,
				IdempotentByTaskID:    true,
				SessionActivityParity: true,
				Evidence:              []string{"runtime_owned_execute"},
			},
		},
	}
	bridge := &RuntimeBridge{service: service}

	projection, err := bridge.RunProjection(context.Background(), RuntimeRunProjectionRequest{
		SessionID: "session-phase311",
		Limit:     24,
	})
	if err != nil {
		t.Fatal(err)
	}
	if !projection.Run.Source.ReadOnly || !projection.Run.Source.SessionActivityParity {
		t.Fatalf("projection source = %#v", projection.Run.Source)
	}
	if len(projection.Run.TaskIDs) != 2 {
		t.Fatalf("projection task ids = %#v", projection.Run.TaskIDs)
	}

	plan, err := bridge.RunSchedulerPlan(context.Background(), RuntimeRunSchedulerPlanRequest{
		RunID:  "run-phase311",
		TaskID: "task-phase311-queued",
		Mode:   "task_turn",
		Limit:  24,
	})
	if err != nil {
		t.Fatal(err)
	}
	if service.runSchedulerPlanReq.RunID != "run-phase311" || service.runSchedulerPlanReq.TaskID != "task-phase311-queued" {
		t.Fatalf("scheduler plan request = %#v", service.runSchedulerPlanReq)
	}
	if !plan.Source.ReadOnly || plan.Source.StartsWorker || !plan.Source.SessionActivityParity {
		t.Fatalf("scheduler plan source = %#v", plan.Source)
	}
	if len(plan.Plan.Items) != 2 || !plan.Plan.Items[0].CanSchedule || plan.Plan.Items[1].CanSchedule {
		t.Fatalf("scheduler items = %#v", plan.Plan.Items)
	}

	execute, err := bridge.ExecuteRunTask(context.Background(), "run-phase311", "task-phase311-queued")
	if err != nil {
		t.Fatal(err)
	}
	if service.executeRunID != "run-phase311" || service.executeTaskID != "task-phase311-queued" {
		t.Fatalf("execute args = %q/%q", service.executeRunID, service.executeTaskID)
	}
	if !execute.Accepted || !execute.ExecutionStarted || execute.Task.Status != "running" {
		t.Fatalf("execute = %#v", execute)
	}
	if !execute.Source.WorkbenchOnly || execute.Source.StartsWorker || !execute.Source.IdempotentByTaskID || !execute.Source.SessionActivityParity {
		t.Fatalf("execute source = %#v", execute.Source)
	}

	permissions, err := bridge.Permissions(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	for _, permission := range permissions.Permissions {
		if permission.Status == "pending" {
			t.Fatalf("stale pending permission resurrected: %#v", permission)
		}
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

func TestRuntimeBridgeForwardsConfiguredProviderWrites(t *testing.T) {
	t.Parallel()

	service := &recordingRuntimeService{
		saveConfiguredProviderResp: RuntimeConfiguredProviderResponse{Provider: RuntimeConfiguredProvider{
			ID:         "provider-1",
			ProviderID: "openai",
			Name:       "OpenAI",
			Enabled:    true,
		}},
		deleteConfiguredProviders: RuntimeConfiguredProvidersResponse{Providers: []RuntimeConfiguredProvider{{
			ID:         "provider-2",
			ProviderID: "anthropic",
			Name:       "Anthropic",
			Enabled:    false,
		}}},
	}
	bridge := &RuntimeBridge{service: service}

	saved, err := bridge.SaveConfiguredProvider(context.Background(), RuntimeConfiguredProviderRequest{
		ID:         "provider-1",
		ProviderID: "openai",
		Name:       "OpenAI",
		Enabled:    true,
	})
	if err != nil {
		t.Fatal(err)
	}
	if service.saveConfiguredProviderCalls != 1 {
		t.Fatalf("save calls = %d, want 1", service.saveConfiguredProviderCalls)
	}
	if service.saveConfiguredProviderReq.ProviderID != "openai" || service.saveConfiguredProviderReq.Name != "OpenAI" || !service.saveConfiguredProviderReq.Enabled {
		t.Fatalf("save request = %#v", service.saveConfiguredProviderReq)
	}
	if saved.Provider.ID != "provider-1" || saved.Provider.ProviderID != "openai" || !saved.Provider.Enabled {
		t.Fatalf("save response = %#v", saved.Provider)
	}

	deleted, err := bridge.DeleteConfiguredProvider(context.Background(), "provider-2")
	if err != nil {
		t.Fatal(err)
	}
	if service.deleteConfiguredProviderID != "provider-2" {
		t.Fatalf("delete id = %q", service.deleteConfiguredProviderID)
	}
	if len(deleted.Providers) != 1 || deleted.Providers[0].ID != "provider-2" {
		t.Fatalf("delete response = %#v", deleted)
	}
}

func TestRuntimeBridgeForwardsAgentTaskReadsAndOutput(t *testing.T) {
	t.Parallel()

	service := &recordingRuntimeService{
		agentTasks: RuntimeAgentTasksResponse{Tasks: []RuntimeAgentTask{{ID: "task-1", ParentSessionID: "session-1", ParentTurnID: "turn-1", Status: "running"}}},
		agentTaskOutput: RuntimeAgentTaskOutputResponse{
			TaskID:     "task-1",
			Status:     "completed",
			Summary:    "done",
			OutputRefs: []string{"runtime://objects/ref-1"},
		},
	}
	bridge := &RuntimeBridge{service: service}

	sessionTasks, err := bridge.SessionAgentTasks(context.Background(), "session-1")
	if err != nil {
		t.Fatal(err)
	}
	if service.agentTaskSessionID != "session-1" || len(sessionTasks.Tasks) != 1 || sessionTasks.Tasks[0].ID != "task-1" {
		t.Fatalf("session tasks=%#v session=%q", sessionTasks, service.agentTaskSessionID)
	}

	output, err := bridge.AgentTaskOutput(context.Background(), "task-1")
	if err != nil {
		t.Fatal(err)
	}
	if service.agentTaskOutputID != "task-1" || output.OutputRefs[0] != "runtime://objects/ref-1" {
		t.Fatalf("output=%#v task=%q", output, service.agentTaskOutputID)
	}
}

type recordingRuntimeService struct {
	chatCalls                     int
	chatRequests                  []RuntimeChatRequest
	userInputReq                  RuntimeUserInputRequest
	userInputResponse             RuntimeChatResponse
	userInputID                   string
	userInput                     RuntimeNormalizedInput
	refreshedCapability           string
	toolSearchQuery               string
	replayExportRequest           runtime.RuntimeReplayExportRequest
	mcpRequestDecision            runtime.RuntimeMCPRequestDecision
	eventsAfter                   int64
	status                        RuntimeStatus
	openProjectReq                RuntimeOpenProjectRequest
	createProjectReq              RuntimeCreateProjectRequest
	renameProjectReq              RuntimeRenameProjectRequest
	openProjectInExplorerReq      RuntimeProjectActionRequest
	removeProjectReq              RuntimeProjectActionRequest
	openProject                   RuntimeOpenProjectResponse
	activity                      RuntimeSessionActivityResponse
	conversationSnapshotSessionID string
	conversationSnapshotRequest   RuntimeCanonicalConversationSnapshotRequest
	contextUsage                  RuntimeContextUsage
	contextCompactionStatus       RuntimeContextCompactionStatus
	contextUsageSessionID         string
	contextGovernanceSettings     RuntimeContextGovernanceSettings
	contextGovernanceSaveReq      RuntimeContextGovernanceSettings
	contextGovernanceSaveCalls    int
	activityWindow                RuntimeSessionActivityWindowResponse
	turnActivity                  RuntimeTurnActivityResponse
	reactCallchain                RuntimeReactCallchainResponse
	reactCallchainTurnID          string
	sessionReactCallchain         RuntimeReactCallchainResponse
	sessionReactCallchainID       string
	sessionReactCallchainLimit    int
	promptAssemblies              RuntimePromptAssembliesResponse
	promptAssembliesTurnID        string
	promptAssembliesSessionID     string
	promptAssembliesLimit         int
	eventsResponse                RuntimeEventsResponse
	newChatTitle                  string
	createSessionReq              RuntimeSessionCreateRequest
	sessionActivityID             string
	sessionActivityWindowID       string
	sessionActivityWindowCursor   string
	sessionActivityWindowLimit    int
	turnActivityID                string
	runProjectionRequest          RuntimeRunProjectionRequest
	runProjection                 RuntimeRunProjectionResponse
	transitionHistoryReq          RuntimeRunTransitionHistoryRequest
	transitionHistory             RuntimeRunTransitionHistoryResponse
	runSchedulerPlanReq           RuntimeRunSchedulerPlanRequest
	runSchedulerPlan              RuntimeRunSchedulerPlanResponse
	executeRunID                  string
	executeTaskID                 string
	executeRunTask                RuntimeRunSchedulerExecuteTaskResponse
	agentTaskSessionID            string
	agentTasks                    RuntimeAgentTasksResponse
	agentTaskOutputID             string
	agentTaskOutput               RuntimeAgentTaskOutputResponse
	todos                         RuntimeTodosResponse
	todoSessionID                 string
	todoTurnID                    string
	runs                          RuntimeRunsResponse
	runCheckpointMarkersID        string
	runCheckpointMarkers          RuntimeRunCheckpointMarkersResponse
	runCheckpointMarkerRunID      string
	runCheckpointMarkerID         string
	runCheckpointMarker           RuntimeRunCheckpointMarkerResponse
	run                           RuntimeRunResponse
	runID                         string
	ackRunID                      string
	ackCheckpointID               string
	discardRunID                  string
	discardCheckpointID           string
	resumeRunID                   string
	resumeCheckpointID            string
	resume                        RuntimeRunResumeResponse
	markInterruptedDoneID         string
	markInterruptedDoneResponse   RuntimeTurnResponse
	resumeInterruptedTurnID       string
	resumeInterruptedTurnReq      RuntimeResumeInterruptedTurnRequest
	discardInterruptedTurnID      string
	retryRecoverableErrorID       string
	terminalResponse              RuntimeTerminalResponse
	sessionTerminals              RuntimeSessionTerminalsResponse
	sessionTerminalsID            string
	createdTerminal               RuntimeTerminalCreateRequest
	terminalInputID               string
	terminalInput                 runtime.RuntimeTerminalInputRequest
	terminalResizeID              string
	terminalResize                runtime.RuntimeTerminalResizeRequest
	deletedTerminalID             string
	saveConfiguredProviderReq     RuntimeConfiguredProviderRequest
	saveConfiguredProviderCalls   int
	saveConfiguredProviderResp    RuntimeConfiguredProviderResponse
	deleteConfiguredProviderID    string
	deleteConfiguredProviders     RuntimeConfiguredProvidersResponse
}

func (s *recordingRuntimeService) Status(context.Context) (RuntimeStatus, error) {
	return s.status, nil
}

func (s *recordingRuntimeService) RecoveryStatus(context.Context) (RuntimeRecoveryStatus, error) {
	return RuntimeRecoveryStatus{}, nil
}

func (s *recordingRuntimeService) ResumeInterruptedTurn(_ context.Context, turnID string, req RuntimeResumeInterruptedTurnRequest) (RuntimeTurnResponse, error) {
	s.resumeInterruptedTurnID = turnID
	s.resumeInterruptedTurnReq = req
	return RuntimeTurnResponse{Turn: RuntimeTurn{ID: turnID, Status: "running", PromptPreview: req.Prompt}}, nil
}

func (s *recordingRuntimeService) DiscardInterruptedTurn(_ context.Context, turnID string) (RuntimeTurnResponse, error) {
	s.discardInterruptedTurnID = turnID
	return RuntimeTurnResponse{Turn: RuntimeTurn{ID: turnID, Status: "cancelled"}}, nil
}

func (s *recordingRuntimeService) RetryRecoverableError(_ context.Context, errorID string) (RuntimeRecoveryRetryResponse, error) {
	s.retryRecoverableErrorID = errorID
	return RuntimeRecoveryRetryResponse{ErrorID: errorID}, nil
}

func (s *recordingRuntimeService) Projects(context.Context) (RuntimeProjectsResponse, error) {
	return RuntimeProjectsResponse{}, nil
}

func (s *recordingRuntimeService) SidebarProjection(context.Context) (RuntimeSidebarProjectionResponse, error) {
	return RuntimeSidebarProjectionResponse{}, nil
}

func (s *recordingRuntimeService) OpenProject(_ context.Context, req RuntimeOpenProjectRequest) (RuntimeOpenProjectResponse, error) {
	s.openProjectReq = req
	return s.openProject, nil
}

func (s *recordingRuntimeService) CreateProject(_ context.Context, req RuntimeCreateProjectRequest) (RuntimeOpenProjectResponse, error) {
	s.createProjectReq = req
	return s.openProject, nil
}

func (s *recordingRuntimeService) RenameProject(_ context.Context, req RuntimeRenameProjectRequest) (RuntimeOpenProjectResponse, error) {
	s.renameProjectReq = req
	return s.openProject, nil
}

func (s *recordingRuntimeService) OpenProjectInExplorer(_ context.Context, req RuntimeProjectActionRequest) (RuntimeOpenProjectResponse, error) {
	s.openProjectInExplorerReq = req
	return s.openProject, nil
}

func (s *recordingRuntimeService) RemoveProject(_ context.Context, req RuntimeProjectActionRequest) (RuntimeOpenProjectResponse, error) {
	s.removeProjectReq = req
	return s.openProject, nil
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

func (s *recordingRuntimeService) SaveConfiguredProvider(_ context.Context, req RuntimeConfiguredProviderRequest) (RuntimeConfiguredProviderResponse, error) {
	s.saveConfiguredProviderReq = req
	s.saveConfiguredProviderCalls++
	if s.saveConfiguredProviderResp.Provider.ID == "" {
		s.saveConfiguredProviderResp = RuntimeConfiguredProviderResponse{Provider: RuntimeConfiguredProvider{
			ID:         req.ID,
			ProviderID: req.ProviderID,
			Name:       req.Name,
			Enabled:    req.Enabled,
		}}
	}
	return s.saveConfiguredProviderResp, nil
}

func (s *recordingRuntimeService) DeleteConfiguredProvider(_ context.Context, providerID string) (RuntimeConfiguredProvidersResponse, error) {
	s.deleteConfiguredProviderID = providerID
	return s.deleteConfiguredProviders, nil
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

func (s *recordingRuntimeService) CreateTerminal(_ context.Context, req RuntimeTerminalCreateRequest) (RuntimeTerminalResponse, error) {
	s.createdTerminal = req
	return s.terminalResponse, nil
}

func (s *recordingRuntimeService) TerminalSettings(context.Context) (RuntimeTerminalSettingsResponse, error) {
	return RuntimeTerminalSettingsResponse{}, nil
}

func (s *recordingRuntimeService) AppearanceSettings(context.Context) (RuntimeAppearanceSettingsResponse, error) {
	return RuntimeAppearanceSettingsResponse{}, nil
}

func (s *recordingRuntimeService) SaveAppearanceSettings(context.Context, RuntimeAppearanceSettings) (RuntimeAppearanceSettingsResponse, error) {
	return RuntimeAppearanceSettingsResponse{}, nil
}

func (s *recordingRuntimeService) OpenTargetSettings(context.Context) (RuntimeOpenTargetSettingsResponse, error) {
	return RuntimeOpenTargetSettingsResponse{}, nil
}

func (s *recordingRuntimeService) SaveOpenTargetSettings(context.Context, RuntimeOpenTargetSettings) (RuntimeOpenTargetSettingsResponse, error) {
	return RuntimeOpenTargetSettingsResponse{}, nil
}

func (s *recordingRuntimeService) SaveTerminalSettings(context.Context, RuntimeTerminalSettings) (RuntimeTerminalSettingsResponse, error) {
	return RuntimeTerminalSettingsResponse{}, nil
}

func (s *recordingRuntimeService) SessionTerminals(_ context.Context, sessionID string) (RuntimeSessionTerminalsResponse, error) {
	s.sessionTerminalsID = sessionID
	if s.sessionTerminals.SessionID == "" {
		s.sessionTerminals.SessionID = sessionID
	}
	return s.sessionTerminals, nil
}

func (s *recordingRuntimeService) WriteTerminalInput(_ context.Context, terminalID string, req runtime.RuntimeTerminalInputRequest) (RuntimeTerminalResponse, error) {
	s.terminalInputID = terminalID
	s.terminalInput = req
	return s.terminalResponse, nil
}

func (s *recordingRuntimeService) ResizeTerminal(_ context.Context, terminalID string, req runtime.RuntimeTerminalResizeRequest) (RuntimeTerminalResponse, error) {
	s.terminalResizeID = terminalID
	s.terminalResize = req
	return s.terminalResponse, nil
}

func (s *recordingRuntimeService) SubscribeTerminalEvents(context.Context, string, ...int64) (<-chan RuntimeTerminalEvent, func()) {
	ch := make(chan RuntimeTerminalEvent)
	close(ch)
	return ch, func() {}
}

func (s *recordingRuntimeService) DeleteTerminal(_ context.Context, terminalID string) (RuntimeTerminalResponse, error) {
	s.deletedTerminalID = terminalID
	return s.terminalResponse, nil
}

func (s *recordingRuntimeService) Chat(_ context.Context, req RuntimeChatRequest) (RuntimeChatResponse, error) {
	s.chatCalls++
	s.chatRequests = append(s.chatRequests, req)
	return RuntimeChatResponse{RequestID: "request-1", TurnID: "turn-new", Status: RuntimeStatus{SessionID: "session-new"}}, nil
}

func (s *recordingRuntimeService) SubmitUserInput(_ context.Context, req RuntimeUserInputRequest) (RuntimeChatResponse, error) {
	s.userInputReq = req
	if s.userInputResponse.RequestID == "" {
		s.userInputResponse = RuntimeChatResponse{RequestID: "request-1", TurnID: "turn-new", Status: RuntimeStatus{SessionID: "session-new"}}
	}
	return s.userInputResponse, nil
}

func (s *recordingRuntimeService) UserInput(_ context.Context, inputID string) (RuntimeNormalizedInput, error) {
	s.userInputID = inputID
	if s.userInput.ID == "" {
		s.userInput.ID = inputID
	}
	return s.userInput, nil
}

func (s *recordingRuntimeService) Turn(context.Context, string) (RuntimeTurnResponse, error) {
	return RuntimeTurnResponse{}, nil
}

func (s *recordingRuntimeService) Turns(context.Context, string) (RuntimeTurnsResponse, error) {
	return RuntimeTurnsResponse{}, nil
}

func (s *recordingRuntimeService) ReactCallchain(_ context.Context, turnID string) (RuntimeReactCallchainResponse, error) {
	s.reactCallchainTurnID = turnID
	if s.reactCallchain.TurnID == "" {
		s.reactCallchain.TurnID = turnID
	}
	return s.reactCallchain, nil
}

func (s *recordingRuntimeService) SessionReactCallchain(_ context.Context, sessionID string, limit int) (RuntimeReactCallchainResponse, error) {
	s.sessionReactCallchainID = sessionID
	s.sessionReactCallchainLimit = limit
	if s.sessionReactCallchain.SessionID == "" {
		s.sessionReactCallchain.SessionID = sessionID
	}
	return s.sessionReactCallchain, nil
}

func (s *recordingRuntimeService) PromptAssembliesByTurn(_ context.Context, turnID string) (RuntimePromptAssembliesResponse, error) {
	s.promptAssembliesTurnID = turnID
	return s.promptAssemblies, nil
}

func (s *recordingRuntimeService) PromptAssembliesBySession(_ context.Context, sessionID string, limit int) (RuntimePromptAssembliesResponse, error) {
	s.promptAssembliesSessionID = sessionID
	s.promptAssembliesLimit = limit
	return s.promptAssemblies, nil
}

func (s *recordingRuntimeService) ManualCompact(context.Context, runtime.RuntimeContextActionRequest) (runtime.RuntimeManualCompactResponse, error) {
	return runtime.RuntimeManualCompactResponse{}, nil
}

func (s *recordingRuntimeService) Runs(context.Context) (RuntimeRunsResponse, error) {
	return s.runs, nil
}

func (s *recordingRuntimeService) RunSummaries(context.Context) (runtime.RuntimeRunSummariesResponse, error) {
	return runtime.RuntimeRunSummariesResponse{}, nil
}

func (s *recordingRuntimeService) RunSummary(context.Context, string) (runtime.RuntimeRunSummaryResponse, error) {
	return runtime.RuntimeRunSummaryResponse{}, nil
}

func (s *recordingRuntimeService) RunCheckpointMarkers(_ context.Context, runID string) (RuntimeRunCheckpointMarkersResponse, error) {
	s.runCheckpointMarkersID = runID
	return s.runCheckpointMarkers, nil
}

func (s *recordingRuntimeService) RunCheckpointMarker(_ context.Context, runID, checkpointID string) (RuntimeRunCheckpointMarkerResponse, error) {
	s.runCheckpointMarkerRunID = runID
	s.runCheckpointMarkerID = checkpointID
	return s.runCheckpointMarker, nil
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

func (s *recordingRuntimeService) ResumeRunCheckpoint(_ context.Context, runID, checkpointID string) (RuntimeRunResumeResponse, error) {
	s.resumeRunID = runID
	s.resumeCheckpointID = checkpointID
	if s.resume.RunID == "" {
		s.resume.RunID = runID
		s.resume.CheckpointID = checkpointID
		s.resume.TurnID = "turn-resume"
	}
	return s.resume, nil
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

func (s *recordingRuntimeService) Objects(context.Context, RuntimeObjectListRequest) (RuntimeObjectsResponse, error) {
	return RuntimeObjectsResponse{}, nil
}

func (s *recordingRuntimeService) Object(context.Context, string) (RuntimeObjectResponse, error) {
	return RuntimeObjectResponse{}, nil
}

func (s *recordingRuntimeService) ReadObjectContent(context.Context, string) (RuntimeObjectContentResponse, error) {
	return RuntimeObjectContentResponse{}, nil
}

func (s *recordingRuntimeService) SessionConversationMessageContentV2(context.Context, string, string) (RuntimeCanonicalMessageContentResponseV2, error) {
	return RuntimeCanonicalMessageContentResponseV2{}, nil
}

func (s *recordingRuntimeService) SearchSessionConversationV2(context.Context, string, RuntimeConversationSearchRequestV2) (RuntimeConversationSearchResponseV2, error) {
	return RuntimeConversationSearchResponseV2{}, nil
}

func (s *recordingRuntimeService) TurnCompactBoundaries(context.Context, string) (RuntimeCompactBoundariesResponse, error) {
	return RuntimeCompactBoundariesResponse{}, nil
}

func (s *recordingRuntimeService) SessionCompactBoundaries(context.Context, string) (RuntimeCompactBoundariesResponse, error) {
	return RuntimeCompactBoundariesResponse{}, nil
}

func (s *recordingRuntimeService) ContextCompactionStatus(context.Context, string) (RuntimeContextCompactionStatus, error) {
	return s.contextCompactionStatus, nil
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

func (s *recordingRuntimeService) SessionAgentTasks(_ context.Context, sessionID string) (RuntimeAgentTasksResponse, error) {
	s.agentTaskSessionID = sessionID
	return s.agentTasks, nil
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

func (s *recordingRuntimeService) AgentTaskOutput(_ context.Context, taskID string) (RuntimeAgentTaskOutputResponse, error) {
	s.agentTaskOutputID = taskID
	return s.agentTaskOutput, nil
}

func (s *recordingRuntimeService) SessionTodos(_ context.Context, sessionID string) (RuntimeTodosResponse, error) {
	s.todoSessionID = sessionID
	return s.todos, nil
}

func (s *recordingRuntimeService) TurnTodos(_ context.Context, turnID string) (RuntimeTodosResponse, error) {
	s.todoTurnID = turnID
	return s.todos, nil
}

func (s *recordingRuntimeService) Sessions(context.Context) (RuntimeSessionsResponse, error) {
	return RuntimeSessionsResponse{}, nil
}

func (s *recordingRuntimeService) Session(context.Context, string) (RuntimeSessionResponse, error) {
	return RuntimeSessionResponse{}, nil
}

func (s *recordingRuntimeService) CreateSession(_ context.Context, req RuntimeSessionCreateRequest) (RuntimeSessionResponse, error) {
	s.createSessionReq = req
	return RuntimeSessionResponse{Session: RuntimeSession{ID: "session-created", Title: req.Title, Active: true}}, nil
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

func (s *recordingRuntimeService) SessionContextUsage(_ context.Context, sessionID string) (RuntimeContextUsage, error) {
	s.contextUsageSessionID = sessionID
	if s.contextUsage.SessionID == "" {
		s.contextUsage.SessionID = sessionID
	}
	return s.contextUsage, nil
}

func (s *recordingRuntimeService) SessionConversationSnapshotV2(_ context.Context, sessionID string, req RuntimeCanonicalConversationSnapshotRequest) (RuntimeCanonicalConversationSnapshot, error) {
	s.conversationSnapshotSessionID = sessionID
	s.conversationSnapshotRequest = req
	return RuntimeCanonicalConversationSnapshot{SchemaVersion: runtime.RuntimeConversationSchemaVersion, SessionID: sessionID, Cursor: "0", Scope: runtime.RuntimeConversationScopeFull, Turns: []runtime.RuntimeCanonicalTurn{}, Messages: []runtime.RuntimeCanonicalMessage{}, AssistantSteps: []runtime.RuntimeCanonicalAssistantStep{}, ToolCalls: []runtime.RuntimeCanonicalToolCall{}, ToolResults: []runtime.RuntimeCanonicalToolResult{}, Permissions: []runtime.RuntimeCanonicalPermission{}, TodoPlans: []runtime.RuntimeCanonicalTodoPlan{}, AgentTasks: []runtime.RuntimeCanonicalAgentTask{}, Notices: []runtime.RuntimeCanonicalNotice{}}, nil
}

func (s *recordingRuntimeService) SessionConversationEventsV2(_ context.Context, sessionID string, req runtime.RuntimeCanonicalConversationEventsRequestV2) (runtime.RuntimeCanonicalConversationEventsResponseV2, error) {
	return runtime.RuntimeCanonicalConversationEventsResponseV2{SchemaVersion: 2, SessionID: sessionID, AfterCursor: req.After, Cursor: req.After, Events: []runtime.RuntimeConversationEntityEventV2{}}, nil
}

func (s *recordingRuntimeService) SubscribeSessionConversationEventsV2(context.Context, string, string) (<-chan runtime.RuntimeCanonicalConversationEventBatchV2, func()) {
	ch := make(chan runtime.RuntimeCanonicalConversationEventBatchV2)
	return ch, func() { close(ch) }
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

func (s *recordingRuntimeService) RunTransitionHistory(_ context.Context, req RuntimeRunTransitionHistoryRequest) (RuntimeRunTransitionHistoryResponse, error) {
	s.transitionHistoryReq = req
	return s.transitionHistory, nil
}

func (s *recordingRuntimeService) RunSchedulerPlan(_ context.Context, req RuntimeRunSchedulerPlanRequest) (RuntimeRunSchedulerPlanResponse, error) {
	s.runSchedulerPlanReq = req
	return s.runSchedulerPlan, nil
}

func (s *recordingRuntimeService) ExecuteRunTask(_ context.Context, runID, taskID string) (RuntimeRunSchedulerExecuteTaskResponse, error) {
	s.executeRunID = runID
	s.executeTaskID = taskID
	if s.executeRunTask.Source.Kind == "" {
		s.executeRunTask.Source = RuntimeRunSchedulerExecuteTaskSource{
			Kind:                  "run_scheduler_execute_task",
			Action:                "execute_task",
			WorkbenchOnly:         true,
			StartsWorker:          false,
			IdempotentByTaskID:    true,
			SessionActivityParity: true,
		}
	}
	return s.executeRunTask, nil
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

func (s *recordingRuntimeService) ContextGovernanceSettings(context.Context) (RuntimeContextGovernanceSettingsResponse, error) {
	return RuntimeContextGovernanceSettingsResponse{Settings: s.contextGovernanceSettings}, nil
}

func (s *recordingRuntimeService) ContextStatistics(context.Context, RuntimeContextStatisticsRequest) (RuntimeContextStatistics, error) {
	return RuntimeContextStatistics{}, nil
}

func (s *recordingRuntimeService) DiagnosticIncidents(context.Context, RuntimeDiagnosticIncidentsRequest) (RuntimeDiagnosticIncidentsResponse, error) {
	return RuntimeDiagnosticIncidentsResponse{}, nil
}

func (s *recordingRuntimeService) DiagnosticIncident(context.Context, string) (RuntimeDiagnosticIncident, error) {
	return RuntimeDiagnosticIncident{}, nil
}

func (s *recordingRuntimeService) RunTargetedDiagnostic(context.Context, string, string) (RuntimeTargetedDiagnostic, error) {
	return RuntimeTargetedDiagnostic{}, nil
}

func (s *recordingRuntimeService) DiagnosticSupportInformation(context.Context, string) (RuntimeDiagnosticSupportInformation, error) {
	return RuntimeDiagnosticSupportInformation{}, nil
}

func (s *recordingRuntimeService) OpenDiagnosticDataDirectory(context.Context) (bool, error) {
	return true, nil
}

func (s *recordingRuntimeService) SaveContextGovernanceSettings(_ context.Context, req RuntimeContextGovernanceSettings) (RuntimeContextGovernanceSettingsResponse, error) {
	s.contextGovernanceSaveCalls++
	s.contextGovernanceSaveReq = req
	s.contextGovernanceSettings = req
	return RuntimeContextGovernanceSettingsResponse{Settings: req}, nil
}

func (s *recordingRuntimeService) Events(_ context.Context, afterValues ...int64) (RuntimeEventsResponse, error) {
	if len(afterValues) > 0 {
		s.eventsAfter = afterValues[0]
	}
	return s.eventsResponse, nil
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

func (s *recordingRuntimeService) ProjectMemories(context.Context, string) (RuntimeMemoryListResponse, error) {
	return RuntimeMemoryListResponse{}, nil
}

func (s *recordingRuntimeService) ProjectMemory(context.Context, string) (RuntimeMemoryDetailResponse, error) {
	return RuntimeMemoryDetailResponse{}, nil
}

func (s *recordingRuntimeService) CreateProjectMemory(context.Context, RuntimeMemoryCreateRequest) (RuntimeMemoryRecord, error) {
	return RuntimeMemoryRecord{}, nil
}

func (s *recordingRuntimeService) UpdateProjectMemory(context.Context, string, RuntimeMemoryUpdateRequest) (RuntimeMemoryRecord, error) {
	return RuntimeMemoryRecord{}, nil
}

func (s *recordingRuntimeService) DisableProjectMemory(context.Context, string, RuntimeMemoryDisableRequest) (RuntimeMemoryRecord, error) {
	return RuntimeMemoryRecord{}, nil
}

func (s *recordingRuntimeService) DeleteProjectMemory(context.Context, string, RuntimeMemoryDeleteRequest) (RuntimeMemoryRecord, error) {
	return RuntimeMemoryRecord{}, nil
}

func (s *recordingRuntimeService) RefreshProjectMemoryIndex(context.Context, string) (RuntimeMemoryIndexResponse, error) {
	return RuntimeMemoryIndexResponse{}, nil
}

func (s *recordingRuntimeService) ProjectMemoryDiagnostics(context.Context, string) (RuntimeMemoryDiagnostics, error) {
	return RuntimeMemoryDiagnostics{}, nil
}

func (s *recordingRuntimeService) ProjectStorageDiagnostics(context.Context, string) (RuntimeProjectStorageDiagnostics, error) {
	return RuntimeProjectStorageDiagnostics{}, nil
}

func (s *recordingRuntimeService) ContextSources(context.Context) (RuntimeContextSourcesResponse, error) {
	return RuntimeContextSourcesResponse{}, nil
}

func (s *recordingRuntimeService) ReadFiles(context.Context, string) (RuntimeReadFilesResponse, error) {
	return RuntimeReadFilesResponse{}, nil
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
