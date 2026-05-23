package agent

import (
	"context"
	"testing"

	"charm.land/fantasy"
	"github.com/charmbracelet/crush/internal/permission"
	"github.com/stretchr/testify/require"
)

type recordingSchedulerRecorder struct {
	decision agentPolicyDecision
	started  bool
	failed   SchedulerToolCallResult
	gotCall  SchedulerToolCall
}

type agentPolicyDecision = SchedulerToolPolicyDecision

func (r *recordingSchedulerRecorder) EvaluateToolCall(_ context.Context, call SchedulerToolCall) (SchedulerToolPolicyDecision, error) {
	r.gotCall = call
	return SchedulerToolPolicyDecision(r.decision), nil
}

func (r *recordingSchedulerRecorder) ToolCallStarted(context.Context, SchedulerToolCall) error {
	r.started = true
	return nil
}

func (r *recordingSchedulerRecorder) ToolCallOutput(context.Context, SchedulerToolCallResult) error {
	return nil
}

func (r *recordingSchedulerRecorder) ToolCallCompleted(context.Context, SchedulerToolCallResult) error {
	return nil
}

func (r *recordingSchedulerRecorder) ToolCallFailed(_ context.Context, result SchedulerToolCallResult) error {
	r.failed = result
	return nil
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
	require.True(t, resp.StopTurn)
	require.False(t, inner.called)
	require.False(t, recorder.started)
	require.Equal(t, "denied", recorder.failed.Status)
	require.Equal(t, "tool-1", recorder.failed.ToolCallID)
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
	require.Equal(t, "builtin:bash", recorder.gotCall.CapabilityID)
}
