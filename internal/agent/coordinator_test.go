package agent

import (
	"context"
	"errors"
	"testing"

	"charm.land/catwalk/pkg/catwalk"
	"charm.land/fantasy"
	"charm.land/fantasy/providers/anthropic"
	"charm.land/fantasy/providers/bedrock"
	"github.com/CIPFZ/agent-builder/internal/agent/tools"
	"github.com/CIPFZ/agent-builder/internal/config"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// mockSessionAgent is a minimal mock for the SessionAgent interface.
type mockSessionAgent struct {
	model     Model
	runFunc   func(ctx context.Context, call SessionAgentCall) (*fantasy.AgentResult, error)
	cancelled []string
}

func (m *mockSessionAgent) Run(ctx context.Context, call SessionAgentCall) (*fantasy.AgentResult, error) {
	return m.runFunc(ctx, call)
}

func (m *mockSessionAgent) Model() Model                        { return m.model }
func (m *mockSessionAgent) SetModels(large, small Model)        {}
func (m *mockSessionAgent) SetTools(tools []fantasy.AgentTool)  {}
func (m *mockSessionAgent) SetSystemPrompt(systemPrompt string) {}
func (m *mockSessionAgent) Cancel(sessionID string) {
	m.cancelled = append(m.cancelled, sessionID)
}
func (m *mockSessionAgent) CancelAll()                                  {}
func (m *mockSessionAgent) IsSessionBusy(sessionID string) bool         { return false }
func (m *mockSessionAgent) IsBusy() bool                                { return false }
func (m *mockSessionAgent) QueuedPrompts(sessionID string) int          { return 0 }
func (m *mockSessionAgent) QueuedPromptsList(sessionID string) []string { return nil }
func (m *mockSessionAgent) ClearQueue(sessionID string)                 {}
func (m *mockSessionAgent) Summarize(context.Context, string, fantasy.ProviderOptions) error {
	return nil
}

// newTestCoordinator creates a minimal coordinator for unit testing runSubAgent.
func newTestCoordinator(t *testing.T, env fakeEnv, providerID string, providerCfg config.ProviderConfig) *coordinator {
	cfg, err := config.Init(env.workingDir, "", false)
	require.NoError(t, err)
	cfg.Config().Providers.Set(providerID, providerCfg)
	return &coordinator{
		cfg:      cfg,
		sessions: env.sessions,
	}
}

// newMockAgent creates a mockSessionAgent with the given provider and run function.
func newMockAgent(providerID string, maxTokens int64, runFunc func(context.Context, SessionAgentCall) (*fantasy.AgentResult, error)) *mockSessionAgent {
	return &mockSessionAgent{
		model: Model{
			CatwalkCfg: catwalk.Model{
				DefaultMaxTokens: maxTokens,
			},
			ModelCfg: config.SelectedModel{
				Provider: providerID,
			},
		},
		runFunc: runFunc,
	}
}

// agentResultWithText creates a minimal AgentResult with the given text response.
func agentResultWithText(text string) *fantasy.AgentResult {
	return &fantasy.AgentResult{
		Response: fantasy.Response{
			Content: fantasy.ResponseContent{
				fantasy.TextContent{Text: text},
			},
		},
	}
}

type recordingAgentTaskRecorder struct {
	started   []AgentTaskRecord
	progress  []AgentTaskRecord
	completed []AgentTaskRecord
	failed    []AgentTaskRecord
}

func (r *recordingAgentTaskRecorder) EvaluateToolCall(context.Context, SchedulerToolCall) (SchedulerToolPolicyDecision, error) {
	return SchedulerToolPolicyDecision{Decision: "allow", Risk: "execute", Mode: "ask"}, nil
}

func (r *recordingAgentTaskRecorder) ToolCallStarted(context.Context, SchedulerToolCall) error {
	return nil
}
func (r *recordingAgentTaskRecorder) ToolCallOutput(context.Context, SchedulerToolCallResult) error {
	return nil
}
func (r *recordingAgentTaskRecorder) ToolCallCompleted(context.Context, SchedulerToolCallResult) error {
	return nil
}
func (r *recordingAgentTaskRecorder) ToolCallFailed(context.Context, SchedulerToolCallResult) error {
	return nil
}
func (r *recordingAgentTaskRecorder) ToolCallCancelled(context.Context, SchedulerToolCallResult) error {
	return nil
}
func (r *recordingAgentTaskRecorder) AgentTaskStarted(_ context.Context, record AgentTaskRecord) error {
	r.started = append(r.started, record)
	return nil
}
func (r *recordingAgentTaskRecorder) AgentTaskProgress(_ context.Context, record AgentTaskRecord) error {
	r.progress = append(r.progress, record)
	return nil
}
func (r *recordingAgentTaskRecorder) AgentTaskCompleted(_ context.Context, record AgentTaskRecord) error {
	r.completed = append(r.completed, record)
	return nil
}
func (r *recordingAgentTaskRecorder) AgentTaskFailed(_ context.Context, record AgentTaskRecord) error {
	r.failed = append(r.failed, record)
	return nil
}

func TestRunSubAgent(t *testing.T) {
	const providerID = "test-provider"
	providerCfg := config.ProviderConfig{ID: providerID}

	t.Run("happy path", func(t *testing.T) {
		env := testEnv(t)
		coord := newTestCoordinator(t, env, providerID, providerCfg)

		parentSession, err := env.sessions.Create(t.Context(), "Parent")
		require.NoError(t, err)

		agent := newMockAgent(providerID, 4096, func(_ context.Context, call SessionAgentCall) (*fantasy.AgentResult, error) {
			assert.Equal(t, "do something", call.Prompt)
			assert.Equal(t, int64(4096), call.MaxOutputTokens)
			return agentResultWithText("done"), nil
		})

		resp, err := coord.runSubAgent(t.Context(), subAgentParams{
			Agent:          agent,
			SessionID:      parentSession.ID,
			AgentMessageID: "msg-1",
			ToolCallID:     "call-1",
			Prompt:         "do something",
			SessionTitle:   "Test Session",
		})
		require.NoError(t, err)
		assert.Equal(t, "done", resp.Content)
		assert.False(t, resp.IsError)
	})

	t.Run("ModelCfg.MaxTokens overrides default", func(t *testing.T) {
		env := testEnv(t)
		coord := newTestCoordinator(t, env, providerID, providerCfg)

		parentSession, err := env.sessions.Create(t.Context(), "Parent")
		require.NoError(t, err)

		agent := &mockSessionAgent{
			model: Model{
				CatwalkCfg: catwalk.Model{
					DefaultMaxTokens: 4096,
				},
				ModelCfg: config.SelectedModel{
					Provider:  providerID,
					MaxTokens: 8192,
				},
			},
			runFunc: func(_ context.Context, call SessionAgentCall) (*fantasy.AgentResult, error) {
				assert.Equal(t, int64(8192), call.MaxOutputTokens)
				return agentResultWithText("ok"), nil
			},
		}

		resp, err := coord.runSubAgent(t.Context(), subAgentParams{
			Agent:          agent,
			SessionID:      parentSession.ID,
			AgentMessageID: "msg-1",
			ToolCallID:     "call-1",
			Prompt:         "test",
			SessionTitle:   "Test",
		})
		require.NoError(t, err)
		assert.Equal(t, "ok", resp.Content)
	})

	t.Run("session creation failure with canceled context", func(t *testing.T) {
		env := testEnv(t)
		coord := newTestCoordinator(t, env, providerID, providerCfg)

		parentSession, err := env.sessions.Create(t.Context(), "Parent")
		require.NoError(t, err)

		agent := newMockAgent(providerID, 4096, nil)

		// Use a canceled context to trigger CreateTaskSession failure.
		ctx, cancel := context.WithCancel(t.Context())
		cancel()

		_, err = coord.runSubAgent(ctx, subAgentParams{
			Agent:          agent,
			SessionID:      parentSession.ID,
			AgentMessageID: "msg-1",
			ToolCallID:     "call-1",
			Prompt:         "test",
			SessionTitle:   "Test",
		})
		require.Error(t, err)
	})

	t.Run("provider not configured", func(t *testing.T) {
		env := testEnv(t)
		coord := newTestCoordinator(t, env, providerID, providerCfg)

		parentSession, err := env.sessions.Create(t.Context(), "Parent")
		require.NoError(t, err)

		// Agent references a provider that doesn't exist in config.
		agent := newMockAgent("unknown-provider", 4096, nil)

		_, err = coord.runSubAgent(t.Context(), subAgentParams{
			Agent:          agent,
			SessionID:      parentSession.ID,
			AgentMessageID: "msg-1",
			ToolCallID:     "call-1",
			Prompt:         "test",
			SessionTitle:   "Test",
		})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "model provider not configured")
	})

	t.Run("agent run error returns error response", func(t *testing.T) {
		env := testEnv(t)
		coord := newTestCoordinator(t, env, providerID, providerCfg)

		parentSession, err := env.sessions.Create(t.Context(), "Parent")
		require.NoError(t, err)

		agent := newMockAgent(providerID, 4096, func(_ context.Context, _ SessionAgentCall) (*fantasy.AgentResult, error) {
			return nil, errors.New("provider request failed")
		})

		resp, err := coord.runSubAgent(t.Context(), subAgentParams{
			Agent:          agent,
			SessionID:      parentSession.ID,
			AgentMessageID: "msg-1",
			ToolCallID:     "call-1",
			Prompt:         "test",
			SessionTitle:   "Test",
		})
		// runSubAgent returns (errorResponse, nil) when agent.Run fails — not a Go error.
		require.NoError(t, err)
		assert.True(t, resp.IsError)
		assert.Equal(t, "Failed to generate response: provider request failed", resp.Content)
	})

	t.Run("session setup callback is invoked", func(t *testing.T) {
		env := testEnv(t)
		coord := newTestCoordinator(t, env, providerID, providerCfg)

		parentSession, err := env.sessions.Create(t.Context(), "Parent")
		require.NoError(t, err)

		var setupCalledWith string
		agent := newMockAgent(providerID, 4096, func(_ context.Context, _ SessionAgentCall) (*fantasy.AgentResult, error) {
			return agentResultWithText("ok"), nil
		})

		_, err = coord.runSubAgent(t.Context(), subAgentParams{
			Agent:          agent,
			SessionID:      parentSession.ID,
			AgentMessageID: "msg-1",
			ToolCallID:     "call-1",
			Prompt:         "test",
			SessionTitle:   "Test",
			SessionSetup: func(sessionID string) {
				setupCalledWith = sessionID
			},
		})
		require.NoError(t, err)
		assert.NotEmpty(t, setupCalledWith, "SessionSetup should have been called")
	})

	t.Run("cost propagation to parent session", func(t *testing.T) {
		env := testEnv(t)
		coord := newTestCoordinator(t, env, providerID, providerCfg)

		parentSession, err := env.sessions.Create(t.Context(), "Parent")
		require.NoError(t, err)

		agent := newMockAgent(providerID, 4096, func(ctx context.Context, call SessionAgentCall) (*fantasy.AgentResult, error) {
			// Simulate the agent incurring cost by updating the child session.
			childSession, err := env.sessions.Get(ctx, call.SessionID)
			if err != nil {
				return nil, err
			}
			childSession.Cost = 0.05
			_, err = env.sessions.Save(ctx, childSession)
			if err != nil {
				return nil, err
			}
			return agentResultWithText("ok"), nil
		})

		_, err = coord.runSubAgent(t.Context(), subAgentParams{
			Agent:          agent,
			SessionID:      parentSession.ID,
			AgentMessageID: "msg-1",
			ToolCallID:     "call-1",
			Prompt:         "test",
			SessionTitle:   "Test",
		})
		require.NoError(t, err)

		updated, err := env.sessions.Get(t.Context(), parentSession.ID)
		require.NoError(t, err)
		assert.InDelta(t, 0.05, updated.Cost, 1e-9)
	})

	t.Run("records agent task lifecycle", func(t *testing.T) {
		env := testEnv(t)
		coord := newTestCoordinator(t, env, providerID, providerCfg)
		recorder := &recordingAgentTaskRecorder{}
		coord.schedulerRecorder = recorder
		coord.agentTaskRecorder = recorder

		parentSession, err := env.sessions.Create(t.Context(), "Parent")
		require.NoError(t, err)

		agent := newMockAgent(providerID, 4096, func(_ context.Context, call SessionAgentCall) (*fantasy.AgentResult, error) {
			assert.Equal(t, "turn-1", call.TurnID)
			return agentResultWithText("ok"), nil
		})
		ctx := context.WithValue(t.Context(), tools.TurnIDContextKey, "turn-1")

		_, err = coord.runSubAgent(ctx, subAgentParams{
			Agent:          agent,
			SessionID:      parentSession.ID,
			AgentMessageID: "msg-1",
			ToolCallID:     "call-1",
			Prompt:         "test prompt",
			SessionTitle:   "Task",
			Kind:           "subagent",
			Name:           "agent",
			AllowedTools:   []string{"view"},
		})
		require.NoError(t, err)
		require.Len(t, recorder.started, 1)
		require.Len(t, recorder.completed, 1)
		assert.Equal(t, "task_call-1", recorder.started[0].ID)
		assert.Equal(t, parentSession.ID, recorder.started[0].ParentSessionID)
		assert.Equal(t, "turn-1", recorder.started[0].ParentTurnID)
		assert.Equal(t, "call-1", recorder.started[0].ParentToolCallID)
		assert.NotEmpty(t, recorder.started[0].ChildSessionID)
		assert.Equal(t, "ok", recorder.completed[0].ResultSummary)
	})

	t.Run("policy denial blocks child session", func(t *testing.T) {
		env := testEnv(t)
		coord := newTestCoordinator(t, env, providerID, providerCfg)
		coord.schedulerRecorder = denyingSchedulerRecorder{}

		parentSession, err := env.sessions.Create(t.Context(), "Parent")
		require.NoError(t, err)
		agent := newMockAgent(providerID, 4096, func(context.Context, SessionAgentCall) (*fantasy.AgentResult, error) {
			t.Fatal("agent should not run")
			return nil, nil
		})

		resp, err := coord.runSubAgent(t.Context(), subAgentParams{
			Agent:          agent,
			SessionID:      parentSession.ID,
			AgentMessageID: "msg-1",
			ToolCallID:     "call-1",
			Prompt:         "test",
			SessionTitle:   "Task",
			Name:           "agent",
		})
		require.NoError(t, err)
		assert.True(t, resp.IsError)
		children, err := env.sessions.List(t.Context())
		require.NoError(t, err)
		count := 0
		for _, child := range children {
			if child.ParentSessionID == parentSession.ID {
				count++
			}
		}
		assert.Zero(t, count)
	})

	t.Run("send to active child session routes to child agent", func(t *testing.T) {
		env := testEnv(t)
		coord := newTestCoordinator(t, env, providerID, providerCfg)
		coord.childAgents = make(map[string]SessionAgent)

		parentSession, err := env.sessions.Create(t.Context(), "Parent")
		require.NoError(t, err)

		ready := make(chan string, 1)
		release := make(chan struct{})
		var followUpPrompt string
		var followUpTurn string
		childAgent := newMockAgent(providerID, 4096, func(ctx context.Context, call SessionAgentCall) (*fantasy.AgentResult, error) {
			if call.Prompt == "initial" {
				ready <- call.SessionID
				<-release
				return agentResultWithText("initial done"), nil
			}
			followUpPrompt = call.Prompt
			followUpTurn = call.TurnID
			return agentResultWithText("follow-up done"), nil
		})

		done := make(chan error, 1)
		go func() {
			_, runErr := coord.runSubAgent(t.Context(), subAgentParams{
				Agent:          childAgent,
				SessionID:      parentSession.ID,
				AgentMessageID: "msg-1",
				ToolCallID:     "call-1",
				Prompt:         "initial",
				SessionTitle:   "Task",
			})
			done <- runErr
		}()
		childSessionID := <-ready
		require.NoError(t, coord.SendToSession(t.Context(), childSessionID, "turn-follow", "follow up"))
		close(release)
		require.NoError(t, <-done)
		assert.Equal(t, "follow up", followUpPrompt)
		assert.Equal(t, "turn-follow", followUpTurn)
	})
}

func TestExecuteStartedAgentTask(t *testing.T) {
	const providerID = "test-provider"
	providerCfg := config.ProviderConfig{ID: providerID}

	t.Run("completed execution skips duplicate start evidence", func(t *testing.T) {
		env := testEnv(t)
		coord := newTestCoordinator(t, env, providerID, providerCfg)
		recorder := &recordingAgentTaskRecorder{}
		coord.schedulerRecorder = recorder
		coord.agentTaskRecorder = recorder

		parent, err := env.sessions.Create(t.Context(), "Parent")
		require.NoError(t, err)
		child, err := env.sessions.CreateTaskSession(t.Context(), "child-task-1", parent.ID, "Task")
		require.NoError(t, err)

		agent := newMockAgent(providerID, 4096, func(ctx context.Context, call SessionAgentCall) (*fantasy.AgentResult, error) {
			assert.Equal(t, child.ID, call.SessionID)
			assert.Equal(t, "turn-1", call.TurnID)
			assert.Equal(t, "review the output", call.Prompt)
			assert.True(t, call.NonInteractive)
			assert.Equal(t, "C:/work/project/.agent-builder/worktrees/wt-1", ctx.Value(tools.EffectiveCWDContextKey))
			return agentResultWithText("completed summary"), nil
		})

		result, err := coord.ExecuteStartedAgentTask(t.Context(), StartedAgentTaskExecutionRequest{
			Agent:                   agent,
			TaskID:                  "task-started-1",
			ParentSessionID:         parent.ID,
			ParentTurnID:            "turn-1",
			ParentToolCallID:        "tool-1",
			ChildSessionID:          child.ID,
			Title:                   "Review output",
			Kind:                    "subagent",
			Role:                    "reviewer",
			Name:                    "agent",
			Prompt:                  "review the output",
			PromptSummary:           "review",
			Provider:                providerID,
			Model:                   "model-1",
			AllowedTools:            []string{"view"},
			CapabilityScope:         []string{"C:/work/project"},
			CWD:                     "C:/work/project",
			Worktree:                "C:/work/project/.agent-builder/worktrees/wt-1",
			StartedAt:               1100,
			StartAlreadyRecorded:    true,
			WorkbenchOnly:           true,
			EventPayloadRefreshOnly: true,
		})
		require.NoError(t, err)
		assert.Equal(t, "task-started-1", result.TaskID)
		assert.Equal(t, "completed", result.Status)
		assert.True(t, result.Terminal)
		assert.True(t, result.NoStaleResume)
		assert.True(t, result.CompletionOnlyRefs)
		require.Empty(t, recorder.started)
		require.Empty(t, recorder.progress)
		require.Len(t, recorder.completed, 1)
		assert.Equal(t, "task-started-1", recorder.completed[0].ID)
		assert.Equal(t, child.ID, recorder.completed[0].ChildSessionID)
		assert.Equal(t, "completed summary", recorder.completed[0].ResultSummary)
	})

	t.Run("active execution routes follow-up and unregisters after return", func(t *testing.T) {
		env := testEnv(t)
		coord := newTestCoordinator(t, env, providerID, providerCfg)
		coord.childAgents = make(map[string]SessionAgent)

		parent, err := env.sessions.Create(t.Context(), "Parent")
		require.NoError(t, err)
		child, err := env.sessions.CreateTaskSession(t.Context(), "child-task-2", parent.ID, "Task")
		require.NoError(t, err)

		ready := make(chan struct{}, 1)
		release := make(chan struct{})
		var followUpPrompt string
		agent := newMockAgent(providerID, 4096, func(ctx context.Context, call SessionAgentCall) (*fantasy.AgentResult, error) {
			if call.Prompt == "initial" {
				ready <- struct{}{}
				<-release
				return agentResultWithText("initial done"), nil
			}
			followUpPrompt = call.Prompt
			return agentResultWithText("follow-up done"), nil
		})

		done := make(chan error, 1)
		go func() {
			_, runErr := coord.ExecuteStartedAgentTask(t.Context(), StartedAgentTaskExecutionRequest{
				Agent:                agent,
				TaskID:               "task-started-2",
				ParentSessionID:      parent.ID,
				ParentTurnID:         "turn-2",
				ParentToolCallID:     "tool-2",
				ChildSessionID:       child.ID,
				Prompt:               "initial",
				StartAlreadyRecorded: true,
				WorkbenchOnly:        true,
			})
			done <- runErr
		}()
		<-ready
		require.NoError(t, coord.SendToSession(t.Context(), child.ID, "turn-follow", "follow up"))
		close(release)
		require.NoError(t, <-done)
		assert.Equal(t, "follow up", followUpPrompt)
		assert.Error(t, coord.SendToSession(t.Context(), child.ID, "turn-after", "after return"))
	})

	t.Run("active execution routes cancellation and records cancelled terminal evidence", func(t *testing.T) {
		env := testEnv(t)
		coord := newTestCoordinator(t, env, providerID, providerCfg)
		recorder := &recordingAgentTaskRecorder{}
		coord.agentTaskRecorder = recorder
		coord.childAgents = make(map[string]SessionAgent)

		parent, err := env.sessions.Create(t.Context(), "Parent")
		require.NoError(t, err)
		child, err := env.sessions.CreateTaskSession(t.Context(), "child-task-3", parent.ID, "Task")
		require.NoError(t, err)

		ready := make(chan struct{}, 1)
		release := make(chan struct{})
		agent := newMockAgent(providerID, 4096, func(ctx context.Context, call SessionAgentCall) (*fantasy.AgentResult, error) {
			ready <- struct{}{}
			<-release
			return nil, context.Canceled
		})
		ctx, cancel := context.WithCancel(t.Context())
		defer cancel()

		done := make(chan error, 1)
		go func() {
			_, runErr := coord.ExecuteStartedAgentTask(ctx, StartedAgentTaskExecutionRequest{
				Agent:                agent,
				TaskID:               "task-started-3",
				ParentSessionID:      parent.ID,
				ParentTurnID:         "turn-3",
				ChildSessionID:       child.ID,
				Prompt:               "initial",
				StartAlreadyRecorded: true,
				WorkbenchOnly:        true,
			})
			done <- runErr
		}()
		<-ready
		coord.Cancel(child.ID)
		cancel()
		close(release)
		require.NoError(t, <-done)
		assert.Equal(t, []string{child.ID}, agent.cancelled)
		require.Empty(t, recorder.started)
		require.Len(t, recorder.failed, 1)
		assert.Equal(t, "cancelled", recorder.failed[0].Status)
		assert.Empty(t, recorder.failed[0].ArtifactRefs)
	})

	t.Run("policy denial records failed terminal evidence without running agent", func(t *testing.T) {
		env := testEnv(t)
		coord := newTestCoordinator(t, env, providerID, providerCfg)
		recorder := &recordingAgentTaskRecorder{}
		coord.schedulerRecorder = denyingSchedulerRecorder{}
		coord.agentTaskRecorder = recorder

		parent, err := env.sessions.Create(t.Context(), "Parent")
		require.NoError(t, err)
		child, err := env.sessions.CreateTaskSession(t.Context(), "child-task-4", parent.ID, "Task")
		require.NoError(t, err)
		agent := newMockAgent(providerID, 4096, func(context.Context, SessionAgentCall) (*fantasy.AgentResult, error) {
			t.Fatal("agent should not run when policy denies started task execution")
			return nil, nil
		})

		result, err := coord.ExecuteStartedAgentTask(t.Context(), StartedAgentTaskExecutionRequest{
			Agent:                agent,
			TaskID:               "task-started-4",
			ParentSessionID:      parent.ID,
			ParentTurnID:         "turn-4",
			ParentToolCallID:     "tool-4",
			ChildSessionID:       child.ID,
			Prompt:               "initial",
			StartAlreadyRecorded: true,
			WorkbenchOnly:        true,
		})
		require.NoError(t, err)
		assert.Equal(t, "failed", result.Status)
		require.Empty(t, recorder.started)
		require.Len(t, recorder.failed, 1)
		assert.Equal(t, "task-started-4", recorder.failed[0].ID)
		assert.Equal(t, "failed", recorder.failed[0].Status)
		assert.Empty(t, recorder.failed[0].ArtifactRefs)
	})

	t.Run("rejects missing pre-recorded start evidence", func(t *testing.T) {
		env := testEnv(t)
		coord := newTestCoordinator(t, env, providerID, providerCfg)
		agent := newMockAgent(providerID, 4096, func(context.Context, SessionAgentCall) (*fantasy.AgentResult, error) {
			t.Fatal("agent should not run without start evidence")
			return nil, nil
		})

		_, err := coord.ExecuteStartedAgentTask(t.Context(), StartedAgentTaskExecutionRequest{
			Agent:           agent,
			TaskID:          "task-started-5",
			ParentSessionID: "session-parent",
			ParentTurnID:    "turn-5",
			ChildSessionID:  "session-child",
		})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "pre-recorded start evidence")
	})
}

func TestExecuteConfiguredStartedAgentTask(t *testing.T) {
	const providerID = "test-provider"
	providerCfg := config.ProviderConfig{ID: providerID}

	t.Run("builds configured task agent and skips duplicate start evidence", func(t *testing.T) {
		env := testEnv(t)
		coord := newTestCoordinator(t, env, providerID, providerCfg)
		recorder := &recordingAgentTaskRecorder{}
		coord.agentTaskRecorder = recorder

		parent, err := env.sessions.Create(t.Context(), "Parent")
		require.NoError(t, err)
		child, err := env.sessions.CreateTaskSession(t.Context(), "child-configured-1", parent.ID, "Task")
		require.NoError(t, err)

		builderCalls := 0
		taskAgent := newMockAgent(providerID, 4096, func(_ context.Context, call SessionAgentCall) (*fantasy.AgentResult, error) {
			assert.Equal(t, child.ID, call.SessionID)
			assert.Equal(t, "turn-configured-1", call.TurnID)
			assert.Equal(t, "configured prompt", call.Prompt)
			assert.True(t, call.NonInteractive)
			return agentResultWithText("configured done"), nil
		})
		coord.startedTaskAgentBuilder = func(context.Context) (SessionAgent, []string, error) {
			builderCalls++
			return taskAgent, []string{"view", "grep"}, nil
		}

		result, err := coord.ExecuteConfiguredStartedAgentTask(t.Context(), StartedAgentTaskExecutionRequest{
			TaskID:               "task-configured-1",
			ParentSessionID:      parent.ID,
			ParentTurnID:         "turn-configured-1",
			ParentToolCallID:     "tool-configured-1",
			ChildSessionID:       child.ID,
			Role:                 config.AgentTask,
			Prompt:               "configured prompt",
			StartAlreadyRecorded: true,
			WorkbenchOnly:        true,
		})
		require.NoError(t, err)
		assert.Equal(t, 1, builderCalls)
		assert.Equal(t, "completed", result.Status)
		require.Empty(t, recorder.started)
		require.Empty(t, recorder.progress)
		require.Len(t, recorder.completed, 1)
		assert.Equal(t, []string{"view", "grep"}, recorder.completed[0].AllowedTools)
		assert.Equal(t, config.AgentTask, recorder.completed[0].Role)
		assert.Equal(t, "agent", recorder.completed[0].Name)
	})

	t.Run("unsupported role fails terminally without building agent", func(t *testing.T) {
		env := testEnv(t)
		coord := newTestCoordinator(t, env, providerID, providerCfg)
		recorder := &recordingAgentTaskRecorder{}
		coord.agentTaskRecorder = recorder
		coord.startedTaskAgentBuilder = func(context.Context) (SessionAgent, []string, error) {
			t.Fatal("builder should not run for unsupported role")
			return nil, nil, nil
		}

		result, err := coord.ExecuteConfiguredStartedAgentTask(t.Context(), StartedAgentTaskExecutionRequest{
			TaskID:               "task-configured-unsupported",
			ParentSessionID:      "session-parent",
			ParentTurnID:         "turn-configured-unsupported",
			ChildSessionID:       "session-child",
			Role:                 "reviewer",
			Prompt:               "prompt",
			StartAlreadyRecorded: true,
			WorkbenchOnly:        true,
		})
		require.NoError(t, err)
		assert.Equal(t, "failed", result.Status)
		require.Len(t, recorder.failed, 1)
		assert.Contains(t, recorder.failed[0].Error, "unsupported started task role")
		assert.Empty(t, recorder.failed[0].ArtifactRefs)
	})

	t.Run("missing task agent config fails terminally", func(t *testing.T) {
		env := testEnv(t)
		coord := newTestCoordinator(t, env, providerID, providerCfg)
		recorder := &recordingAgentTaskRecorder{}
		coord.agentTaskRecorder = recorder
		delete(coord.cfg.Config().Agents, config.AgentTask)

		result, err := coord.ExecuteConfiguredStartedAgentTask(t.Context(), StartedAgentTaskExecutionRequest{
			TaskID:               "task-configured-missing",
			ParentSessionID:      "session-parent",
			ParentTurnID:         "turn-configured-missing",
			ChildSessionID:       "session-child",
			Role:                 config.AgentTask,
			Prompt:               "prompt",
			StartAlreadyRecorded: true,
			WorkbenchOnly:        true,
		})
		require.Error(t, err)
		assert.Equal(t, "failed", result.Status)
		require.Len(t, recorder.failed, 1)
		assert.Contains(t, recorder.failed[0].Error, "task agent not configured")
		assert.Empty(t, recorder.failed[0].ArtifactRefs)
	})
}

type denyingSchedulerRecorder struct{}

func (denyingSchedulerRecorder) EvaluateToolCall(context.Context, SchedulerToolCall) (SchedulerToolPolicyDecision, error) {
	return SchedulerToolPolicyDecision{Decision: "deny", Risk: "execute", Reason: "blocked", Mode: "plan"}, nil
}
func (denyingSchedulerRecorder) ToolCallStarted(context.Context, SchedulerToolCall) error { return nil }
func (denyingSchedulerRecorder) ToolCallOutput(context.Context, SchedulerToolCallResult) error {
	return nil
}
func (denyingSchedulerRecorder) ToolCallCompleted(context.Context, SchedulerToolCallResult) error {
	return nil
}
func (denyingSchedulerRecorder) ToolCallFailed(context.Context, SchedulerToolCallResult) error {
	return nil
}
func (denyingSchedulerRecorder) ToolCallCancelled(context.Context, SchedulerToolCallResult) error {
	return nil
}

func TestUpdateParentSessionCost(t *testing.T) {
	t.Run("accumulates cost correctly", func(t *testing.T) {
		env := testEnv(t)
		cfg, err := config.Init(env.workingDir, "", false)
		require.NoError(t, err)
		coord := &coordinator{cfg: cfg, sessions: env.sessions}

		parent, err := env.sessions.Create(t.Context(), "Parent")
		require.NoError(t, err)

		child, err := env.sessions.CreateTaskSession(t.Context(), "tool-1", parent.ID, "Child")
		require.NoError(t, err)

		// Set child cost.
		child.Cost = 0.10
		_, err = env.sessions.Save(t.Context(), child)
		require.NoError(t, err)

		err = coord.updateParentSessionCost(t.Context(), child.ID, parent.ID)
		require.NoError(t, err)

		updated, err := env.sessions.Get(t.Context(), parent.ID)
		require.NoError(t, err)
		assert.InDelta(t, 0.10, updated.Cost, 1e-9)
	})

	t.Run("accumulates multiple child costs", func(t *testing.T) {
		env := testEnv(t)
		cfg, err := config.Init(env.workingDir, "", false)
		require.NoError(t, err)
		coord := &coordinator{cfg: cfg, sessions: env.sessions}

		parent, err := env.sessions.Create(t.Context(), "Parent")
		require.NoError(t, err)

		child1, err := env.sessions.CreateTaskSession(t.Context(), "tool-1", parent.ID, "Child1")
		require.NoError(t, err)
		child1.Cost = 0.05
		_, err = env.sessions.Save(t.Context(), child1)
		require.NoError(t, err)

		child2, err := env.sessions.CreateTaskSession(t.Context(), "tool-2", parent.ID, "Child2")
		require.NoError(t, err)
		child2.Cost = 0.03
		_, err = env.sessions.Save(t.Context(), child2)
		require.NoError(t, err)

		err = coord.updateParentSessionCost(t.Context(), child1.ID, parent.ID)
		require.NoError(t, err)
		err = coord.updateParentSessionCost(t.Context(), child2.ID, parent.ID)
		require.NoError(t, err)

		updated, err := env.sessions.Get(t.Context(), parent.ID)
		require.NoError(t, err)
		assert.InDelta(t, 0.08, updated.Cost, 1e-9)
	})

	t.Run("child session not found", func(t *testing.T) {
		env := testEnv(t)
		cfg, err := config.Init(env.workingDir, "", false)
		require.NoError(t, err)
		coord := &coordinator{cfg: cfg, sessions: env.sessions}

		parent, err := env.sessions.Create(t.Context(), "Parent")
		require.NoError(t, err)

		err = coord.updateParentSessionCost(t.Context(), "non-existent", parent.ID)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "get child session")
	})

	t.Run("parent session not found", func(t *testing.T) {
		env := testEnv(t)
		cfg, err := config.Init(env.workingDir, "", false)
		require.NoError(t, err)
		coord := &coordinator{cfg: cfg, sessions: env.sessions}

		parent, err := env.sessions.Create(t.Context(), "Parent")
		require.NoError(t, err)
		child, err := env.sessions.CreateTaskSession(t.Context(), "tool-1", parent.ID, "Child")
		require.NoError(t, err)

		err = coord.updateParentSessionCost(t.Context(), child.ID, "non-existent")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "get parent session")
	})

	t.Run("zero cost handled correctly", func(t *testing.T) {
		env := testEnv(t)
		cfg, err := config.Init(env.workingDir, "", false)
		require.NoError(t, err)
		coord := &coordinator{cfg: cfg, sessions: env.sessions}

		parent, err := env.sessions.Create(t.Context(), "Parent")
		require.NoError(t, err)
		child, err := env.sessions.CreateTaskSession(t.Context(), "tool-1", parent.ID, "Child")
		require.NoError(t, err)

		err = coord.updateParentSessionCost(t.Context(), child.ID, parent.ID)
		require.NoError(t, err)

		updated, err := env.sessions.Get(t.Context(), parent.ID)
		require.NoError(t, err)
		assert.InDelta(t, 0.0, updated.Cost, 1e-9)
	})
}

func TestGetProviderOptionsReasoningEffort(t *testing.T) {
	// Bedrock is Fantasy's Anthropic under a different provider name; options
	// must land under anthropic.Name so the Anthropic language model picks them up.
	tests := []struct {
		name         string
		providerType catwalk.Type
	}{
		{"anthropic honors reasoning_effort", catwalk.Type(anthropic.Name)},
		{"bedrock honors reasoning_effort", catwalk.Type(bedrock.Name)},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			model := Model{
				CatwalkCfg: catwalk.Model{
					ID:              "claude-opus-4-7",
					CanReason:       true,
					ReasoningLevels: []string{"max"},
				},
				ModelCfg: config.SelectedModel{
					Provider:        "test",
					ReasoningEffort: "max",
				},
			}
			providerCfg := config.ProviderConfig{ID: "test", Type: tc.providerType}

			opts := getProviderOptions(model, providerCfg)

			raw, ok := opts[anthropic.Name]
			require.True(t, ok, "options should be keyed under anthropic.Name for type %q", tc.providerType)
			parsed, ok := raw.(*anthropic.ProviderOptions)
			require.True(t, ok)
			require.NotNil(t, parsed.Effort)
			assert.Equal(t, anthropic.Effort("max"), *parsed.Effort)
		})
	}
}
