package tui

import (
	"context"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"

	"myclaw/internal/agent"
	"myclaw/internal/approval"
	"myclaw/internal/compaction"
	"myclaw/internal/diagnostics"
	"myclaw/internal/llm"
	"myclaw/internal/memory"
	"myclaw/internal/orchestration"
	"myclaw/internal/permissions"
	"myclaw/internal/queryengine"
	"myclaw/internal/runtime"
	"myclaw/internal/session"
	"myclaw/internal/tools"
	"myclaw/internal/workspace"
)

// testRuntimeBridge is a test-only direct runtime helper used to validate legacy
// semantics without keeping a production alternate backend path inside internal/tui.
type testRuntimeBridge struct {
	ctx      context.Context
	sessions *session.Manager
	runner   *runtime.Runner
	session  session.Session
	logger   *diagnostics.Logger

	mu   sync.RWMutex
	send func(tea.Msg)
}

func newTestRuntimeBridge(sessions *session.Manager, runner *runtime.Runner, agentID string) *testRuntimeBridge {
	return newTestRuntimeBridgeWithContext(context.Background(), sessions, runner, agentID, nil)
}

func newTestRuntimeBridgeWithContext(ctx context.Context, sessions *session.Manager, runner *runtime.Runner, agentID string, logger *diagnostics.Logger) *testRuntimeBridge {
	if sessions == nil {
		sessions = session.NewManager(nil)
	}
	bridge := &testRuntimeBridge{
		ctx:      ctx,
		sessions: sessions,
		runner:   runner,
		session:  sessions.GetOrCreateMain(agentID),
		logger:   logger,
	}
	if runner != nil {
		runner.SetReportToolProgress(func(progress tools.ToolProgress) {
			progressCopy := cloneToolProgress(progress)
			bridge.dispatch(RuntimeEventMsg{Event: clientEventFromRuntimeEvent(runtime.RuntimeEvent{
				Type:      "tool.progress",
				Session:   bridge.session,
				ToolUseID: progress.ToolUseID,
				Progress:  &progressCopy,
			})})
		})
	}
	return bridge
}

func (b *testRuntimeBridge) Attach(send func(tea.Msg)) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.send = send
}

func (b *testRuntimeBridge) SendUserMessage(input string) error {
	if err := b.ctx.Err(); err != nil {
		return err
	}
	msg, err := b.sessions.AppendMessage(b.session.ID, "user", input)
	if err != nil {
		return err
	}
	if b.runner == nil {
		return nil
	}
	go func() {
		err := b.runner.HandleUserMessage(b.ctx, b.session, msg, testSinkFunc(func(event runtime.RuntimeEvent) error {
			b.dispatch(RuntimeEventMsg{Event: clientEventFromRuntimeEvent(event)})
			return nil
		}))
		if err != nil {
			var approvalErr *queryengine.ApprovalRequiredError
			if errors.As(err, &approvalErr) {
				return
			}
			b.dispatch(BridgeErrMsg{Err: err})
		}
	}()
	return nil
}

func (b *testRuntimeBridge) SessionSnapshots() []sessionSnapshot {
	sessions := b.sessions.ListSessions()
	snapshots := make([]sessionSnapshot, 0, len(sessions))
	for _, sess := range sessions {
		messages, _ := b.sessions.Messages(sess.ID)
		snapshot := sessionSnapshot{
			Session:      sess,
			MessageCount: len(messages),
		}
		if len(messages) > 0 && snapshot.Session.Metadata.LastActivityAt.IsZero() {
			snapshot.Session.Metadata.LastActivityAt = messages[len(messages)-1].CreatedAt
		}
		for _, message := range messages {
			if snapshot.FirstUserMessage == "" && message.Role == "user" {
				snapshot.FirstUserMessage = message.Content
			}
			if strings.TrimSpace(message.Content) != "" {
				snapshot.LastMessage = message.Content
			}
		}
		snapshots = append(snapshots, snapshot)
	}
	return snapshots
}

func (b *testRuntimeBridge) ResumeSession(id string) (session.RecoverySnapshot, bool) {
	snapshot, ok := b.sessions.RecoverySnapshot(id)
	if !ok {
		return session.RecoverySnapshot{}, false
	}
	b.session = snapshot.Session
	return snapshot, true
}

func (b *testRuntimeBridge) TaskPanelSnapshot() taskPanelSnapshot {
	snapshot := taskPanelSnapshot{SessionID: b.session.ID}
	if b.runner == nil {
		return snapshot
	}

	var coordinator *orchestration.Coordinator
	if hook := b.runner.Orchestrator(); hook != nil {
		coordinator = coordinatorFromHook(hook)
	}

	for _, run := range b.runner.AgentManager().List() {
		if run.ParentSessionID != b.session.ID {
			continue
		}
		task := taskSnapshot{
			RunID:               run.ID,
			Label:               run.Label,
			Prompt:              run.Prompt,
			Status:              string(run.Status),
			ParentSessionID:     run.ParentSessionID,
			ChildSessionID:      run.ChildSessionID,
			ChildSessionKey:     run.ChildSessionKey,
			Output:              run.Output,
			ControlMessageCount: len(b.runner.AgentManager().ControlMessages(run.ID)),
		}
		if run.Err != nil {
			task.Error = run.Err.Error()
		}
		if messages, ok := b.sessions.Messages(run.ChildSessionID); ok {
			task.MessageCount = len(messages)
			for i := len(messages) - 1; i >= 0; i-- {
				if messages[i].Role == "assistant" && strings.TrimSpace(messages[i].Content) != "" {
					task.LastAssistant = messages[i].Content
					break
				}
			}
		}
		if coordinator != nil {
			if state, ok := coordinator.GetRun(run.ID); ok {
				task.LastEvent = state.LastEvent
				task.Message = state.Message
				task.NextAction = state.NextAction
				task.RecommendedRole = state.RecommendedRole
				task.RecommendedAction = state.RecommendedAction
				task.DecisionPriority = state.DecisionPriority
				task.DecisionReason = state.DecisionReason
				task.AutoExecutable = state.AutoExecutable
			}
		}
		switch run.Status {
		case "running":
			snapshot.RunningCount++
		case "completed":
			snapshot.CompletedCount++
		case "failed":
			snapshot.FailedCount++
		case "stopped":
			snapshot.StoppedCount++
		}
		snapshot.Tasks = append(snapshot.Tasks, task)
	}
	return snapshot
}

func (b *testRuntimeBridge) PlatformStatusSnapshot() platformStatusSnapshot {
	snapshot := platformStatusSnapshot{
		SessionID:  b.session.ID,
		SessionKey: b.session.Key,
		AgentID:    b.session.AgentID,
		IsMain:     b.session.IsMain,
	}
	if b.runner == nil {
		return snapshot
	}

	policy := b.runner.PermissionPolicyForSession(b.session.ID)
	snapshot.WorkspaceRoots = append([]string(nil), policy.WorkspaceRoots...)
	snapshot.BaseModel = b.runner.BaseMainLoopModelForSession(b.session.ID)
	snapshot.ModelOverride = b.runner.SessionMainLoopModelOverride(b.session.ID)
	snapshot.ResolvedModel = b.runner.ResolvedMainLoopModelForSession(b.session.ID)
	if models, err := b.runner.AvailableModels(b.ctx); err == nil {
		snapshot.AvailableModels = make([]platformModelOption, 0, len(models))
		for _, model := range models {
			snapshot.AvailableModels = append(snapshot.AvailableModels, platformModelOption{
				Value:               model.ID,
				Label:               valueOrUnset(model.DisplayName),
				Description:         strings.TrimSpace(model.Description),
				ContextWindowTokens: model.ContextWindowTokens,
				MaxOutputTokens:     model.MaxOutputTokens,
			})
		}
	}

	inventory := b.runner.MCPInventory()
	snapshot.MCPServerCount = inventory.ServerCount
	snapshot.MCPToolCount = inventory.ToolCount
	snapshot.MCPPromptCount = inventory.PromptCount
	snapshot.MCPResourceCount = inventory.ResourceCount
	return snapshot
}

func (b *testRuntimeBridge) SetSessionModel(model string) error {
	if err := b.ctx.Err(); err != nil {
		return err
	}
	if b.runner == nil {
		return nil
	}
	return b.runner.SetSessionMainLoopModelOverride(b.session.ID, model)
}

func (b *testRuntimeBridge) ClearSessionModel() error {
	if err := b.ctx.Err(); err != nil {
		return err
	}
	if b.runner == nil {
		return nil
	}
	return b.runner.ClearSessionMainLoopModelOverride(b.session.ID)
}

func (b *testRuntimeBridge) ContextSnapshot() contextSnapshot {
	if b.runner == nil {
		return contextSnapshot{}
	}
	snapshot, err := b.runner.ContextSnapshot(b.session.ID)
	if err != nil {
		return contextSnapshot{}
	}
	return contextSnapshot{
		Model:               snapshot.Model,
		UsedTokens:          snapshot.UsedTokens,
		ContextWindowTokens: snapshot.ContextWindowTokens,
		UsagePercent:        snapshot.UsagePercent,
		CategoryLines:       append([]string(nil), snapshot.CategoryLines...),
	}
}

func (b *testRuntimeBridge) CompactionSnapshot() compactionSnapshot {
	if b.runner == nil {
		return compactionSnapshot{}
	}
	snapshot, err := b.runner.CompactionSnapshot(b.session.ID)
	if err != nil {
		return compactionSnapshot{}
	}
	return compactionSnapshot{
		EstimatedTokens:      snapshot.Analysis.EstimatedTokens,
		ContextWindowTokens:  snapshot.Analysis.ContextWindowTokens,
		WarningThreshold:     snapshot.Analysis.WarningThreshold,
		ErrorThreshold:       snapshot.Analysis.ErrorThreshold,
		AutoCompactThreshold: snapshot.Analysis.AutoCompactThreshold,
		BlockingThreshold:    snapshot.Analysis.BlockingThreshold,
		AboveWarning:         snapshot.Analysis.IsAboveWarningThreshold,
		AboveError:           snapshot.Analysis.IsAboveErrorThreshold,
		AboveAutoCompact:     snapshot.Analysis.IsAboveAutoCompactThreshold,
		AtBlockingLimit:      snapshot.Analysis.IsAtBlockingLimit,
		LastCompactionReason: snapshot.LastCompactionReason,
		LastCompactedAtLabel: formatTimestamp(snapshot.LastCompactedAt),
	}
}

func (b *testRuntimeBridge) CompactSession(customInstructions string) (compactionActionResult, error) {
	if err := b.ctx.Err(); err != nil {
		return compactionActionResult{}, err
	}
	if b.runner == nil {
		return compactionActionResult{}, nil
	}
	result, err := b.runner.CompactSession(b.session.ID, customInstructions)
	if err != nil {
		return compactionActionResult{}, err
	}
	return compactionActionResult{
		Changed:        result.Changed,
		Reason:         result.Reason,
		OriginalCount:  result.OriginalCount,
		CompactedCount: result.CompactedCount,
	}, nil
}

func (b *testRuntimeBridge) MicrocompactSession() (compactionActionResult, error) {
	if err := b.ctx.Err(); err != nil {
		return compactionActionResult{}, err
	}
	if b.runner == nil {
		return compactionActionResult{}, nil
	}
	result, err := b.runner.MicrocompactSession(b.session.ID)
	if err != nil {
		return compactionActionResult{}, err
	}
	return compactionActionResult{
		Changed:        result.Changed,
		Reason:         result.Reason,
		OriginalCount:  result.OriginalCount,
		CompactedCount: result.CompactedCount,
	}, nil
}

func (b *testRuntimeBridge) MCPSnapshot() mcpSnapshot {
	if b.runner == nil {
		return mcpSnapshot{}
	}
	runtimeServers := b.runner.MCPServers()
	servers := make([]mcpServerSnapshot, 0, len(runtimeServers))
	for _, server := range runtimeServers {
		servers = append(servers, mcpServerSnapshot{
			Name:          server.Name,
			TransportType: server.TransportType,
			Endpoint:      server.Endpoint,
			Enabled:       server.Enabled,
			Tools:         append([]string(nil), server.Tools...),
			Prompts:       append([]string(nil), server.Prompts...),
			Resources:     append([]string(nil), server.Resources...),
		})
	}
	return mcpSnapshot{Servers: servers}
}

func (b *testRuntimeBridge) Approve(id string) error {
	if err := b.ctx.Err(); err != nil {
		return err
	}
	go func() {
		err := b.runner.ApproveAndContinue(b.ctx, id, testSinkFunc(func(event runtime.RuntimeEvent) error {
			b.dispatch(RuntimeEventMsg{Event: clientEventFromRuntimeEvent(event)})
			return nil
		}))
		if err != nil {
			var approvalErr *queryengine.ApprovalRequiredError
			if errors.As(err, &approvalErr) {
				return
			}
			b.dispatch(BridgeErrMsg{Err: err})
		}
	}()
	return nil
}

func (b *testRuntimeBridge) Reject(id string) error {
	if err := b.ctx.Err(); err != nil {
		return err
	}
	updated, err := b.runner.UpdateApprovalStatus(id, approval.StatusRejected)
	if err != nil {
		return err
	}
	b.dispatch(RuntimeEventMsg{Event: clientEventFromRuntimeEvent(runtime.RuntimeEvent{
		Type:     "approval.updated",
		Session:  b.session,
		Approval: &updated,
	})})
	return nil
}

func (b *testRuntimeBridge) dispatch(msg tea.Msg) {
	b.mu.RLock()
	defer b.mu.RUnlock()
	if b.send != nil {
		b.send(msg)
	}
}

type testSinkFunc func(runtime.RuntimeEvent) error

func (s testSinkFunc) Emit(event runtime.RuntimeEvent) error {
	return s(event)
}

func coordinatorFromHook(hook orchestration.Hook) *orchestration.Coordinator {
	switch typed := hook.(type) {
	case *orchestration.Coordinator:
		return typed
	case orchestration.Chain:
		for _, child := range typed {
			if coordinator := coordinatorFromHook(child); coordinator != nil {
				return coordinator
			}
		}
	}
	return nil
}

func formatTimestamp(value time.Time) string {
	if value.IsZero() {
		return ""
	}
	return value.UTC().Format("2006-01-02 15:04 MST")
}

func TestRuntimeBridgeStreamsAssistantEvents(t *testing.T) {
	sessions := session.NewManager(nil)
	runner := runtime.NewRunnerWithOptions(sessions, llm.NewMockClient(), workspace.NewLoader(""), nil, runtime.Options{
		PermissionPolicy: permissions.Policy{Mode: permissions.ModeDangerFullAccess},
	})
	bridge := newTestRuntimeBridge(sessions, runner, "main")

	ch := make(chan tea.Msg, 16)
	bridge.Attach(func(msg tea.Msg) { ch <- msg })

	if err := bridge.SendUserMessage("hello"); err != nil {
		t.Fatalf("SendUserMessage: %v", err)
	}

	events := waitForEventTypes(t, ch, 2*time.Second, "assistant.delta", "message.created")
	assertHasEventType(t, events, "assistant.delta")
	assertHasEventType(t, events, "message.created")
}

func TestRuntimeBridgeSurfacesPermissionRequired(t *testing.T) {
	sessions := session.NewManager(nil)
	runner := runtime.NewRunnerWithOptions(sessions, llm.NewMockClient(), workspace.NewLoader(""), nil, runtime.Options{
		PermissionPolicy: permissions.Policy{Mode: permissions.ModeAsk},
	})
	bridge := newTestRuntimeBridge(sessions, runner, "main")

	ch := make(chan tea.Msg, 16)
	bridge.Attach(func(msg tea.Msg) { ch <- msg })

	if err := bridge.SendUserMessage("tool run pwd"); err != nil {
		t.Fatalf("SendUserMessage: %v", err)
	}

	events := waitForEventTypes(t, ch, 2*time.Second, "permission.required")
	assertHasEventType(t, events, "permission.required")
}

func TestRuntimeBridgeDispatchesToolProgress(t *testing.T) {
	sessions := session.NewManager(nil)
	runner := runtime.NewRunnerWithOptions(sessions, llm.NewMockClient(), workspace.NewLoader(""), nil, runtime.Options{
		PermissionPolicy: permissions.Policy{Mode: permissions.ModeDangerFullAccess},
	})
	bridge := newTestRuntimeBridge(sessions, runner, "main")

	ch := make(chan tea.Msg, 32)
	bridge.Attach(func(msg tea.Msg) { ch <- msg })

	if err := bridge.SendUserMessage("tool run pwd"); err != nil {
		t.Fatalf("SendUserMessage: %v", err)
	}

	events := waitForEventTypes(t, ch, 2*time.Second, "tool.progress", "tool.result")
	var progress runtime.RuntimeEvent
	for _, event := range events {
		if event.Type == "tool.progress" {
			progress = event
			break
		}
	}
	if progress.Progress == nil || progress.Progress.ToolUseID == "" {
		t.Fatalf("progress event = %#v, want tool progress with tool use id", progress)
	}
}

func TestRuntimeBridgeDoesNotSurfaceBridgeErrorForPermissionRequired(t *testing.T) {
	sessions := session.NewManager(nil)
	runner := runtime.NewRunnerWithOptions(sessions, llm.NewMockClient(), workspace.NewLoader(""), nil, runtime.Options{
		PermissionPolicy: permissions.Policy{Mode: permissions.ModeAsk},
	})
	bridge := newTestRuntimeBridge(sessions, runner, "main")

	ch := make(chan tea.Msg, 16)
	bridge.Attach(func(msg tea.Msg) { ch <- msg })

	if err := bridge.SendUserMessage("tool run pwd"); err != nil {
		t.Fatalf("SendUserMessage: %v", err)
	}

	deadline := time.After(1500 * time.Millisecond)
	for {
		select {
		case raw := <-ch:
			switch msg := raw.(type) {
			case RuntimeEventMsg:
				if msg.Event.Type == "permission.required" {
					grace := time.After(250 * time.Millisecond)
					for {
						select {
						case followup := <-ch:
							if errMsg, ok := followup.(BridgeErrMsg); ok {
								t.Fatalf("unexpected BridgeErrMsg for approval-required flow: %v", errMsg.Err)
							}
						case <-grace:
							return
						}
					}
				}
			case BridgeErrMsg:
				t.Fatalf("unexpected BridgeErrMsg for approval-required flow: %v", msg.Err)
			}
		case <-deadline:
			t.Fatal("timed out waiting for permission.required")
		}
	}
}

func TestRuntimeBridgeApproveContinuesBlockedExecution(t *testing.T) {
	sessions := session.NewManager(nil)
	runner := runtime.NewRunnerWithOptions(sessions, llm.NewMockClient(), workspace.NewLoader(""), nil, runtime.Options{
		PermissionPolicy: permissions.Policy{
			Mode: permissions.ModeAsk,
			Rules: []permissions.Rule{{
				ToolName: "text.upper",
				Action:   permissions.ActionAsk,
				Match: permissions.Match{
					CommandContains: []string{"hello world"},
				},
			}},
		},
		ApprovalManager: approval.NewManager(),
	})
	bridge := newTestRuntimeBridge(sessions, runner, "main")

	ch := make(chan tea.Msg, 32)
	bridge.Attach(func(msg tea.Msg) { ch <- msg })

	if err := bridge.SendUserMessage("tool upper hello world"); err != nil {
		t.Fatalf("SendUserMessage: %v", err)
	}

	events := waitForEventTypes(t, ch, 2*time.Second, "permission.required")
	var approvalID string
	for _, event := range events {
		if event.Type == "permission.required" && event.Approval != nil {
			approvalID = event.Approval.ID
			break
		}
	}
	if approvalID == "" {
		t.Fatal("expected approval id")
	}

	if err := bridge.Approve(approvalID); err != nil {
		t.Fatalf("Approve: %v", err)
	}

	events = waitForEventTypes(t, ch, 2*time.Second, "tool.result", "message.created")
	assertHasEventType(t, events, "tool.result")
	assertHasEventType(t, events, "message.created")
}

func waitForRuntimeEvents(t *testing.T, ch <-chan tea.Msg, min int, timeout time.Duration) []runtime.RuntimeEvent {
	t.Helper()
	deadline := time.After(timeout)
	events := make([]runtime.RuntimeEvent, 0, min)
	for len(events) < min {
		select {
		case raw := <-ch:
			if msg, ok := raw.(RuntimeEventMsg); ok {
				events = append(events, runtimeEventFromClientEvent(msg.Event))
			}
		case <-deadline:
			t.Fatalf("timed out waiting for %d runtime events, got %#v", min, events)
		}
	}
	return events
}

func waitForEventTypes(t *testing.T, ch <-chan tea.Msg, timeout time.Duration, wants ...string) []runtime.RuntimeEvent {
	t.Helper()
	deadline := time.After(timeout)
	events := make([]runtime.RuntimeEvent, 0, len(wants)+2)
	seen := make(map[string]bool, len(wants))
	for {
		done := true
		for _, want := range wants {
			if !seen[want] {
				done = false
				break
			}
		}
		if done {
			return events
		}

		select {
		case raw := <-ch:
			if msg, ok := raw.(RuntimeEventMsg); ok {
				event := runtimeEventFromClientEvent(msg.Event)
				events = append(events, event)
				seen[event.Type] = true
			}
		case <-deadline:
			t.Fatalf("timed out waiting for event types %v, got %#v", wants, events)
		}
	}
}

func assertHasEventType(t *testing.T, events []runtime.RuntimeEvent, want string) {
	t.Helper()
	for _, event := range events {
		if event.Type == want {
			return
		}
	}
	t.Fatalf("events missing %q: %#v", want, events)
}

func TestRuntimeBridgeRejectMarksApprovalRejected(t *testing.T) {
	sessions := session.NewManager(nil)
	runner := runtime.NewRunnerWithOptions(sessions, llm.NewMockClient(), workspace.NewLoader(""), nil, runtime.Options{
		PermissionPolicy: permissions.Policy{Mode: permissions.ModeAsk},
		ApprovalManager:  approval.NewManager(),
	})
	bridge := newTestRuntimeBridge(sessions, runner, "main")

	ch := make(chan tea.Msg, 16)
	bridge.Attach(func(msg tea.Msg) { ch <- msg })

	if err := bridge.SendUserMessage("tool run pwd"); err != nil {
		t.Fatalf("SendUserMessage: %v", err)
	}
	events := waitForEventTypes(t, ch, 2*time.Second, "permission.required")
	var approvalID string
	for _, event := range events {
		if event.Type == "permission.required" && event.Approval != nil {
			approvalID = event.Approval.ID
			break
		}
	}
	if approvalID == "" {
		t.Fatal("expected approval id")
	}
	if err := bridge.Reject(approvalID); err != nil {
		t.Fatalf("Reject: %v", err)
	}
	request, ok := runner.ApprovalManager().Get(approvalID)
	if !ok || request.Status != approval.StatusRejected {
		t.Fatalf("approval after reject = %#v, ok=%v", request, ok)
	}
}

func TestRuntimeBridgeContextCancellationStopsSend(t *testing.T) {
	sessions := session.NewManager(nil)
	runner := runtime.NewRunnerWithOptions(sessions, llm.NewMockClient(), workspace.NewLoader(""), nil, runtime.Options{
		PermissionPolicy: permissions.Policy{Mode: permissions.ModeDangerFullAccess},
	})
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	bridge := newTestRuntimeBridgeWithContext(ctx, sessions, runner, "main", nil)
	if err := bridge.SendUserMessage("hello"); err == nil {
		t.Fatal("expected canceled bridge to reject send")
	}
}

func TestRuntimeBridgeTaskPanelSnapshotIncludesDelegatedRuns(t *testing.T) {
	sessions := session.NewManager(nil)
	coordinator := orchestration.NewCoordinator()
	runner := runtime.NewRunnerWithOptions(sessions, llm.NewMockClient(), workspace.NewLoader(""), nil, runtime.Options{
		PermissionPolicy: permissions.Policy{Mode: permissions.ModeDangerFullAccess},
		AgentManager:     agent.NewManager(),
		Orchestrator:     coordinator,
	})
	bridge := newTestRuntimeBridge(sessions, runner, "main")
	parent := sessions.GetOrCreateMain("main")

	run, err := runner.SpawnSubagent(context.Background(), parent, "research", "hello subagent")
	if err != nil {
		t.Fatalf("SpawnSubagent: %v", err)
	}
	if _, err := runner.AgentManager().Wait(context.Background(), run.ID, 2*time.Second); err != nil {
		t.Fatalf("Wait: %v", err)
	}

	snapshot := bridge.TaskPanelSnapshot()
	if snapshot.SessionID != parent.ID {
		t.Fatalf("session id = %q, want %q", snapshot.SessionID, parent.ID)
	}
	if snapshot.CompletedCount != 1 || len(snapshot.Tasks) != 1 {
		t.Fatalf("snapshot = %#v, want one completed task", snapshot)
	}
	task := snapshot.Tasks[0]
	if task.RunID != run.ID || task.Label != "research" || task.ChildSessionID == "" {
		t.Fatalf("task = %#v, want populated delegated run", task)
	}
	if task.MessageCount == 0 || task.LastAssistant == "" {
		t.Fatalf("task = %#v, want child session transcript details", task)
	}
	if task.Status != "completed" {
		t.Fatalf("task = %#v, want completed task status", task)
	}
}

func TestRuntimeBridgePlatformStatusSnapshotIncludesSessionWorkspaceAndMCPDetails(t *testing.T) {
	sessions := session.NewManager(nil)
	runner := runtime.NewRunnerWithOptions(sessions, llm.NewMockClient(), workspace.NewLoader(""), nil, runtime.Options{
		PermissionPolicy: permissions.Policy{
			Mode:           permissions.ModeWorkspaceWrite,
			WorkspaceRoots: []string{"C:/repo", "C:/repo/subdir"},
		},
		MCPClients: []tools.MCPConnection{
			{Name: "filesystem"},
			{Name: "figma"},
		},
		MCPResources: map[string][]tools.MCPResource{
			"filesystem": {
				{URI: "file://README.md", Name: "README"},
				{URI: "file://docs/plan.md", Name: "Plan"},
			},
			"figma": {
				{URI: "figma://node/1", Name: "Node"},
			},
		},
		MCPTools: map[string]tools.MCPToolsListResult{
			"filesystem": {Tools: []tools.MCPToolListItem{{Name: "read_file"}, {Name: "write_file"}}},
			"figma":      {Tools: []tools.MCPToolListItem{{Name: "get_design"}}},
		},
		MCPPrompts: map[string]tools.MCPPromptsListResult{
			"filesystem": {Prompts: []tools.MCPPromptListItem{{Name: "summarize"}}},
		},
	})
	parent := sessions.GetOrCreateMain("main")
	if err := runner.SetSessionMainLoopModelOverride(parent.ID, "claude-opus-4-6"); err != nil {
		t.Fatalf("SetSessionMainLoopModelOverride: %v", err)
	}

	bridge := newTestRuntimeBridge(sessions, runner, "main")
	snapshot := bridge.PlatformStatusSnapshot()

	if snapshot.SessionID != parent.ID {
		t.Fatalf("session id = %q, want %q", snapshot.SessionID, parent.ID)
	}
	if snapshot.SessionKey != parent.Key || snapshot.AgentID != parent.AgentID || !snapshot.IsMain {
		t.Fatalf("snapshot = %#v, want main session identity details", snapshot)
	}
	if len(snapshot.WorkspaceRoots) != 2 || snapshot.WorkspaceRoots[0] != "C:/repo" || snapshot.WorkspaceRoots[1] != "C:/repo/subdir" {
		t.Fatalf("workspace roots = %#v, want configured policy roots", snapshot.WorkspaceRoots)
	}
	if snapshot.ModelOverride != "claude-opus-4-6" {
		t.Fatalf("model override = %q, want claude-opus-4-6", snapshot.ModelOverride)
	}
	if snapshot.MCPServerCount != 2 || snapshot.MCPToolCount != 3 || snapshot.MCPPromptCount != 1 || snapshot.MCPResourceCount != 3 {
		t.Fatalf("snapshot = %#v, want aggregated MCP counts", snapshot)
	}
}

func TestRuntimeBridgeMCPSnapshotIncludesServerDetails(t *testing.T) {
	sessions := session.NewManager(nil)
	runner := runtime.NewRunnerWithOptions(sessions, llm.NewMockClient(), workspace.NewLoader(""), nil, runtime.Options{
		MCPClients: []tools.MCPConnection{
			{Name: "filesystem", Type: "stdio", Command: "npx", Args: []string{"@modelcontextprotocol/server-filesystem", "C:/repo"}},
			{Name: "figma", Type: "sse", URL: "https://figma.example/mcp"},
		},
		MCPResources: map[string][]tools.MCPResource{
			"filesystem": {{URI: "file://README.md"}, {URI: "file://docs/plan.md"}},
		},
		MCPTools: map[string]tools.MCPToolsListResult{
			"filesystem": {Tools: []tools.MCPToolListItem{{Name: "read_file"}, {Name: "write_file"}}},
			"figma":      {Tools: []tools.MCPToolListItem{{Name: "get_design"}}},
		},
		MCPPrompts: map[string]tools.MCPPromptsListResult{
			"filesystem": {Prompts: []tools.MCPPromptListItem{{Name: "summarize"}}},
		},
	})

	bridge := newTestRuntimeBridge(sessions, runner, "main")
	snapshot := bridge.MCPSnapshot()

	if len(snapshot.Servers) != 2 {
		t.Fatalf("servers = %#v, want 2", snapshot.Servers)
	}
	if snapshot.Servers[0].Name != "figma" || snapshot.Servers[1].Name != "filesystem" {
		t.Fatalf("servers = %#v, want sorted by name", snapshot.Servers)
	}
	filesystem := snapshot.Servers[1]
	if filesystem.TransportType != "stdio" || filesystem.Endpoint == "" {
		t.Fatalf("filesystem = %#v, want transport metadata", filesystem)
	}
	if len(filesystem.Tools) != 2 || filesystem.Tools[0] != "read_file" || filesystem.Prompts[0] != "summarize" || len(filesystem.Resources) != 2 {
		t.Fatalf("filesystem = %#v, want MCP tool/prompt/resource details", filesystem)
	}
}

func TestRuntimeBridgeCompactionSnapshotAndActions(t *testing.T) {
	sessions := session.NewManager(nil)
	sess := sessions.GetOrCreateMain("main")
	for _, entry := range []struct {
		role    string
		content string
	}{
		{"user", "first request"},
		{"assistant", "first answer"},
		{"user", "second request"},
		{"assistant", "second answer"},
		{"user", "third request"},
		{"assistant", "third answer"},
	} {
		if _, err := sessions.AppendMessage(sess.ID, entry.role, entry.content); err != nil {
			t.Fatalf("AppendMessage(%s): %v", entry.role, err)
		}
	}
	if err := sessions.UpdateMetadata(sess.ID, func(metadata *session.SessionMetadata) {
		metadata.LastCompactionReason = "message-limit"
		metadata.LastCompactedAt = time.Date(2026, time.April, 19, 10, 30, 0, 0, time.UTC)
	}); err != nil {
		t.Fatalf("UpdateMetadata: %v", err)
	}

	memSvc := memory.NewService()
	memSvc.SaveCompactionSummary(sess, session.Message{
		ID:               "summary-old",
		SessionID:        sess.ID,
		Role:             "summary",
		Content:          "Summary: old context",
		IsCompactSummary: true,
		CreatedAt:        time.Now().UTC(),
	})

	runner := runtime.NewRunnerWithOptions(sessions, llm.NewMockClient(), workspace.NewLoader(""), nil, runtime.Options{
		PermissionPolicy: permissions.Policy{Mode: permissions.ModeDangerFullAccess},
		MemoryService:    memSvc,
		Compactor: compaction.NewService(compaction.Config{
			MaxMessages:         3,
			PreserveRecentTurns: 1,
			SummaryPrefix:       "Summary:",
		}),
	})
	bridge := newTestRuntimeBridge(sessions, runner, "main")

	snapshot := bridge.CompactionSnapshot()
	if snapshot.LastCompactionReason != "message-limit" {
		t.Fatalf("snapshot = %#v, want recovered reason", snapshot)
	}
	if snapshot.LastCompactedAtLabel == "" {
		t.Fatalf("snapshot = %#v, want formatted compaction time", snapshot)
	}

	result, err := bridge.CompactSession("")
	if err != nil {
		t.Fatalf("CompactSession: %v", err)
	}
	if !result.Changed || result.Reason == "" {
		t.Fatalf("result = %#v, want changed compaction result", result)
	}

	micro, err := bridge.MicrocompactSession()
	if err != nil {
		t.Fatalf("MicrocompactSession: %v", err)
	}
	if micro.Reason != "microcompact" && micro.Changed {
		t.Fatalf("micro = %#v, want microcompact reason when changed", micro)
	}
}
