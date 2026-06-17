package agent

import (
	"context"
	"errors"
	"testing"

	"charm.land/fantasy"
	"github.com/charmbracelet/crush/internal/permission"
	"github.com/stretchr/testify/require"
)

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
