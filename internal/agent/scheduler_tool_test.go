package agent

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"testing"
	"time"

	"charm.land/fantasy"
	"github.com/CIPFZ/agent-builder/internal/permission"
	"github.com/CIPFZ/agent-builder/internal/shell"
	"github.com/stretchr/testify/require"
)

type queuedSchedulerRecorder struct {
	decision  SchedulerToolPolicyDecision
	mu        sync.Mutex
	events    []string
	queued    chan SchedulerToolCall
	started   chan SchedulerToolCall
	cancelled chan SchedulerToolCallResult
}

func newQueuedSchedulerRecorder() *queuedSchedulerRecorder {
	return &queuedSchedulerRecorder{
		decision:  SchedulerToolPolicyDecision{Decision: string(permission.PolicyAllow), Risk: string(permission.RiskExecute), Mode: string(permission.PolicyModeAutoRead)},
		queued:    make(chan SchedulerToolCall, 1),
		started:   make(chan SchedulerToolCall, 1),
		cancelled: make(chan SchedulerToolCallResult, 1),
	}
}

func (r *queuedSchedulerRecorder) addEvent(event string) {
	r.mu.Lock()
	r.events = append(r.events, event)
	r.mu.Unlock()
}

func (r *queuedSchedulerRecorder) EvaluateToolCall(context.Context, SchedulerToolCall) (SchedulerToolPolicyDecision, error) {
	return r.decision, nil
}
func (r *queuedSchedulerRecorder) ToolCallQueued(_ context.Context, call SchedulerToolCall) error {
	r.addEvent("queued")
	r.queued <- call
	return nil
}
func (r *queuedSchedulerRecorder) ToolCallStarted(_ context.Context, call SchedulerToolCall) error {
	r.addEvent("started")
	r.started <- call
	return nil
}
func (r *queuedSchedulerRecorder) ToolCallOutput(context.Context, SchedulerToolCallResult) error {
	return nil
}
func (r *queuedSchedulerRecorder) ToolCallCompleted(context.Context, SchedulerToolCallResult) error {
	r.addEvent("completed")
	return nil
}
func (r *queuedSchedulerRecorder) ToolCallFailed(context.Context, SchedulerToolCallResult) error {
	return nil
}
func (r *queuedSchedulerRecorder) ToolCallCancelled(_ context.Context, result SchedulerToolCallResult) error {
	r.addEvent("cancelled")
	r.cancelled <- result
	return nil
}

type testToolResourceGovernor struct {
	sem map[string]chan struct{}
}

func newTestToolResourceGovernor(limits map[string]int) *testToolResourceGovernor {
	governor := &testToolResourceGovernor{sem: make(map[string]chan struct{}, len(limits))}
	for class, limit := range limits {
		governor.sem[class] = make(chan struct{}, limit)
	}
	return governor
}

func (g *testToolResourceGovernor) AcquireTool(ctx context.Context, class string, _ int64) (func(), error) {
	sem := g.sem[class]
	if sem == nil {
		return func() {}, nil
	}
	select {
	case sem <- struct{}{}:
	case <-ctx.Done():
		return nil, ctx.Err()
	}
	var once sync.Once
	return func() { once.Do(func() { <-sem }) }, nil
}

type recordingSchedulerRecorder struct {
	decision    agentPolicyDecision
	started     bool
	startErr    error
	failErr     error
	completeErr error
	failed      SchedulerToolCallResult
	completed   SchedulerToolCallResult
	gotCall     SchedulerToolCall
}

type agentPolicyDecision = SchedulerToolPolicyDecision

func (r *recordingSchedulerRecorder) EvaluateToolCall(_ context.Context, call SchedulerToolCall) (SchedulerToolPolicyDecision, error) {
	r.gotCall = call
	return SchedulerToolPolicyDecision(r.decision), nil
}

func (r *recordingSchedulerRecorder) ToolCallStarted(context.Context, SchedulerToolCall) error {
	r.started = true
	return r.startErr
}

func (r *recordingSchedulerRecorder) ToolCallOutput(context.Context, SchedulerToolCallResult) error {
	return nil
}

func (r *recordingSchedulerRecorder) ToolCallCompleted(_ context.Context, result SchedulerToolCallResult) error {
	r.completed = result
	return r.completeErr
}

func (r *recordingSchedulerRecorder) ToolCallFailed(_ context.Context, result SchedulerToolCallResult) error {
	r.failed = result
	return r.failErr
}

func (r *recordingSchedulerRecorder) ToolCallCancelled(context.Context, SchedulerToolCallResult) error {
	return nil
}

func TestSchedulerToolDeniesBeforeInnerRun(t *testing.T) {
	t.Parallel()

	inner := &fakeTool{name: "write"}
	recorder := &recordingSchedulerRecorder{
		decision: SchedulerToolPolicyDecision{
			Decision: string(permission.PolicyDeny),
			Risk:     string(permission.RiskWrite),
			Reason:   "Plan mode blocks write tools.",
			Mode:     string(permission.PolicyModePlan),
		},
	}
	tool := newSchedulerTool(inner, recorder)

	resp, err := tool.Run(t.Context(), fantasy.ToolCall{ID: "tool-1", Name: "write", Input: "{}"})
	require.NoError(t, err)
	require.True(t, resp.IsError)
	require.False(t, resp.StopTurn)
	require.False(t, inner.called)
	require.False(t, recorder.started)
	require.Equal(t, "denied", recorder.failed.Status)
	require.Equal(t, "tool-1", recorder.failed.ToolCallID)
}

func TestSchedulerToolDeniesTodosInPlanMode(t *testing.T) {
	t.Parallel()

	inner := &fakeTool{name: "todos"}
	recorder := &recordingSchedulerRecorder{
		decision: SchedulerToolPolicyDecision{
			Decision: string(permission.PolicyDeny),
			Risk:     string(permission.RiskWrite),
			Reason:   "Plan mode blocks mutating, execute, network, destructive, or secret tool calls.",
			Mode:     string(permission.PolicyModePlan),
		},
	}
	tool := newSchedulerTool(inner, recorder)

	resp, err := tool.Run(t.Context(), fantasy.ToolCall{ID: "tool-1", Name: "todos", Input: `{"todos":[]}`})
	require.NoError(t, err)
	require.True(t, resp.IsError)
	require.False(t, resp.StopTurn)
	require.False(t, inner.called)
	require.Equal(t, "denied", recorder.failed.Status)
	require.Equal(t, "write", recorder.failed.Risk)
	require.Equal(t, "Plan mode blocks mutating, execute, network, destructive, or secret tool calls.", recorder.failed.PolicyReason)
}

func TestSchedulerToolAllowsReadTools(t *testing.T) {
	t.Parallel()

	inner := &fakeTool{name: "view", resp: fantasy.NewTextResponse("ok")}
	recorder := &recordingSchedulerRecorder{
		decision: SchedulerToolPolicyDecision{
			Decision: string(permission.PolicyAllow),
			Risk:     string(permission.RiskRead),
			Mode:     string(permission.PolicyModePlan),
		},
	}
	tool := newSchedulerTool(inner, recorder)

	resp, err := tool.Run(t.Context(), fantasy.ToolCall{ID: "tool-1", Name: "view", Input: "{}"})
	require.NoError(t, err)
	require.Equal(t, "ok", resp.Content)
	require.True(t, inner.called)
	require.True(t, recorder.started)
}

func TestSchedulerToolPassesSourceToPolicyRecorder(t *testing.T) {
	t.Parallel()

	inner := &fakeTool{name: "bash", resp: fantasy.NewTextResponse("ok")}
	recorder := &recordingSchedulerRecorder{
		decision: SchedulerToolPolicyDecision{
			Decision: string(permission.PolicyAllow),
			Risk:     string(permission.RiskExecute),
			Mode:     string(permission.PolicyModeAutoRead),
		},
	}
	tool := newSchedulerTool(inner, recorder)

	_, err := tool.Run(t.Context(), fantasy.ToolCall{ID: "tool-1", Name: "bash", Input: `{"command":"go test ./..."}`})
	require.NoError(t, err)
	require.Equal(t, "shell", recorder.gotCall.Source)
	require.Equal(t, "shell:bash", recorder.gotCall.CapabilityID)
}

func TestSchedulerToolPassesShellMetadata(t *testing.T) {
	t.Parallel()

	inner := &fakeTool{name: "bash", resp: fantasy.WithResponseMetadata(fantasy.NewTextResponse("ok"), map[string]any{
		"shell_id":  "ABC",
		"command":   "go test ./...",
		"stdout":    "ok",
		"stderr":    "",
		"exit_code": 0,
		"status":    "completed",
	})}
	recorder := &recordingSchedulerRecorder{
		decision: SchedulerToolPolicyDecision{
			Decision: string(permission.PolicyAllow),
			Risk:     string(permission.RiskExecute),
			Reason:   "allowed",
			Mode:     string(permission.PolicyModeAsk),
		},
	}
	tool := newSchedulerTool(inner, recorder)

	_, err := tool.Run(t.Context(), fantasy.ToolCall{ID: "tool-1", Name: "bash", Input: `{"command":"go test ./..."}`})
	require.NoError(t, err)
	require.Equal(t, "ABC", recorder.completed.JobID)
	require.Equal(t, "go test ./...", recorder.completed.Command)
	require.Equal(t, "execute", recorder.completed.Risk)
	require.Equal(t, "allowed", recorder.completed.PolicyReason)
	require.Equal(t, "ok", recorder.completed.Stdout)
	require.Equal(t, "completed", recorder.completed.JobStatus)
}

func TestSchedulerToolPassesMediaStructuredMetadata(t *testing.T) {
	t.Parallel()

	resp := fantasy.NewImageResponse([]byte("image"), "image/png")
	resp.Metadata = `{"artifact_path":"C:\\Users\\ytq\\work\\ai\\agent-builder\\tmp\\runtime-dev\\phase65-image.json"}`
	inner := &fakeTool{name: "mcp_server_image_artifact", resp: resp}
	recorder := &recordingSchedulerRecorder{
		decision: SchedulerToolPolicyDecision{
			Decision: string(permission.PolicyAllow),
			Risk:     string(permission.RiskNetwork),
			Mode:     string(permission.PolicyModeAsk),
		},
	}
	tool := newSchedulerTool(inner, recorder)

	_, err := tool.Run(t.Context(), fantasy.ToolCall{ID: "tool-1", Name: "mcp_server_image_artifact", Input: `{}`})
	require.NoError(t, err)
	require.JSONEq(t, resp.Metadata, recorder.completed.StructuredOutputSummary)
}

func TestSchedulerToolStampsPolicyApprovalForInnerPermission(t *testing.T) {
	t.Parallel()

	inner := &fakeTool{name: "bash", resp: fantasy.NewTextResponse("ok")}
	recorder := &recordingSchedulerRecorder{
		decision: SchedulerToolPolicyDecision{
			Decision: string(permission.PolicyAllow),
			Risk:     string(permission.RiskExecute),
			Mode:     string(permission.PolicyModeAutoRead),
		},
	}
	tool := newSchedulerTool(inner, recorder)

	_, err := tool.Run(t.Context(), fantasy.ToolCall{ID: "tool-1", Name: "bash", Input: `{"command":"go test ./..."}`})
	require.NoError(t, err)

	svc := permission.NewPermissionService(t.TempDir(), false, nil)
	svc.SetPolicyMode(permission.PolicyModeAutoRead)
	granted, err := svc.Request(inner.gotCtx, permission.CreatePermissionRequest{
		SessionID:   "session-1",
		ToolCallID:  "tool-1",
		ToolName:    "bash",
		Source:      "shell",
		Action:      "execute",
		Description: `{"command":"go test ./..."}`,
		Path:        t.TempDir(),
	})
	require.NoError(t, err)
	require.True(t, granted)
}

func TestSchedulerToolCleansUpWhenStartRecordingFails(t *testing.T) {
	t.Parallel()

	inner := &fakeTool{name: "view", resp: fantasy.NewTextResponse("ok")}
	recorder := &recordingSchedulerRecorder{
		decision: SchedulerToolPolicyDecision{
			Decision: string(permission.PolicyAllow),
			Risk:     string(permission.RiskRead),
			Mode:     string(permission.PolicyModeAutoRead),
		},
		startErr: errors.New("store unavailable"),
	}
	tool := newSchedulerTool(inner, recorder)

	resp, err := tool.Run(t.Context(), fantasy.ToolCall{ID: "tool-1", Name: "view", Input: "{}"})
	require.NoError(t, err)
	require.True(t, resp.IsError)
	require.False(t, inner.called)
	require.Equal(t, "store unavailable", recorder.failed.Error)
}

func TestSchedulerToolReturnsFinalRecorderError(t *testing.T) {
	t.Parallel()

	inner := &fakeTool{name: "view", resp: fantasy.NewTextResponse("ok")}
	recorder := &recordingSchedulerRecorder{
		decision: SchedulerToolPolicyDecision{
			Decision: string(permission.PolicyAllow),
			Risk:     string(permission.RiskRead),
			Mode:     string(permission.PolicyModeAutoRead),
		},
		completeErr: errors.New("complete failed"),
	}
	tool := newSchedulerTool(inner, recorder)

	_, err := tool.Run(t.Context(), fantasy.ToolCall{ID: "tool-1", Name: "view", Input: "{}"})
	require.EqualError(t, err, "complete failed")
	require.Equal(t, "ok", recorder.completed.ModelVisibleContent)
}

func TestSchedulerToolRecordsQueuedBeforeResourceAdmission(t *testing.T) {
	governor := newTestToolResourceGovernor(map[string]int{ToolResourceShell: 1})
	occupied, err := governor.AcquireTool(context.Background(), ToolResourceShell, 0)
	require.NoError(t, err)
	recorder := newQueuedSchedulerRecorder()
	inner := &fakeTool{name: "bash", resp: fantasy.NewTextResponse("ok")}
	tool := newSchedulerTool(inner, recorder)
	done := make(chan error, 1)
	go func() {
		_, runErr := tool.Run(WithToolResourceGovernor(context.Background(), governor), fantasy.ToolCall{ID: "tool-queued", Name: "bash", Input: `{"command":"go test ./..."}`})
		done <- runErr
	}()

	select {
	case call := <-recorder.queued:
		require.Equal(t, "tool-queued", call.ID)
	case <-time.After(5 * time.Second):
		t.Fatal("tool call was not recorded as queued")
	}
	select {
	case <-recorder.started:
		t.Fatal("tool call started before resource admission")
	case <-time.After(50 * time.Millisecond):
	}
	occupied()
	select {
	case <-recorder.started:
	case <-time.After(5 * time.Second):
		t.Fatal("tool call did not start after resource release")
	}
	require.NoError(t, <-done)
	require.True(t, inner.called)
	recorder.mu.Lock()
	require.Equal(t, []string{"queued", "started", "completed"}, recorder.events)
	recorder.mu.Unlock()
}

func TestSchedulerToolCancelledDuringAdmissionNeverRunsInnerTool(t *testing.T) {
	governor := newTestToolResourceGovernor(map[string]int{ToolResourceShell: 1})
	occupied, err := governor.AcquireTool(context.Background(), ToolResourceShell, 0)
	require.NoError(t, err)
	defer occupied()
	recorder := newQueuedSchedulerRecorder()
	inner := &fakeTool{name: "bash", resp: fantasy.NewTextResponse("unexpected")}
	tool := newSchedulerTool(inner, recorder)
	ctx, cancel := context.WithCancel(WithToolResourceGovernor(context.Background(), governor))
	done := make(chan error, 1)
	go func() {
		_, runErr := tool.Run(ctx, fantasy.ToolCall{ID: "tool-cancelled", Name: "bash", Input: `{"command":"sleep 60"}`})
		done <- runErr
	}()

	select {
	case <-recorder.queued:
	case <-time.After(5 * time.Second):
		t.Fatal("tool call was not recorded as queued")
	}
	cancel()
	require.ErrorIs(t, <-done, context.Canceled)
	select {
	case result := <-recorder.cancelled:
		require.True(t, result.Cancelled)
	case <-time.After(5 * time.Second):
		t.Fatal("cancelled admission was not recorded")
	}
	require.False(t, inner.called)
	select {
	case <-recorder.started:
		t.Fatal("cancelled tool call was recorded as started")
	default:
	}
}

func TestSchedulerToolLightweightReadDoesNotEnterResourceQueue(t *testing.T) {
	governor := newTestToolResourceGovernor(map[string]int{ToolResourceHeavy: 1})
	occupied, err := governor.AcquireTool(context.Background(), ToolResourceHeavy, 0)
	require.NoError(t, err)
	defer occupied()
	recorder := newQueuedSchedulerRecorder()
	recorder.decision.Risk = string(permission.RiskRead)
	inner := &fakeTool{name: "view", resp: fantasy.NewTextResponse("ok")}
	tool := newSchedulerTool(inner, recorder)

	resp, err := tool.Run(WithToolResourceGovernor(context.Background(), governor), fantasy.ToolCall{ID: "tool-read", Name: "view", Input: `{"file_path":"README.md"}`})
	require.NoError(t, err)
	require.Equal(t, "ok", resp.Content)
	require.True(t, inner.called)
	select {
	case <-recorder.queued:
		t.Fatal("lightweight read tool entered the heavy resource queue")
	default:
	}
}

func TestSchedulerToolRetainsShellLeaseForBackgroundProcess(t *testing.T) {
	manager := shell.GetBackgroundShellManager()
	backgroundShell, err := manager.Start(context.Background(), t.TempDir(), nil, "sleep 10", "resource lease test")
	require.NoError(t, err)
	t.Cleanup(func() { _ = manager.Kill(backgroundShell.ID) })

	governor := newTestToolResourceGovernor(map[string]int{ToolResourceShell: 1})
	recorder := newQueuedSchedulerRecorder()
	response := fantasy.NewTextResponse("background shell started")
	response.Metadata = fmt.Sprintf(`{"shell_id":%q,"status":"running","background":true}`, backgroundShell.ID)
	inner := &fakeTool{name: "bash", resp: response}
	tool := newSchedulerTool(inner, recorder)

	_, err = tool.Run(WithToolResourceGovernor(context.Background(), governor), fantasy.ToolCall{ID: "tool-background", Name: "bash", Input: `{"command":"sleep 10","run_in_background":true}`})
	require.NoError(t, err)
	require.Len(t, governor.sem[ToolResourceShell], 1, "background process must retain its shell lease")
	require.NoError(t, manager.Kill(backgroundShell.ID))
	require.Eventually(t, func() bool { return len(governor.sem[ToolResourceShell]) == 0 }, 5*time.Second, time.Millisecond)
}
