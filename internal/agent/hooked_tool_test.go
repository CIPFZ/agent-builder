package agent

import (
	"context"
	"testing"

	"charm.land/fantasy"
	"github.com/charmbracelet/crush/internal/config"
	"github.com/charmbracelet/crush/internal/hooks"
	"github.com/charmbracelet/crush/internal/permission"
	"github.com/stretchr/testify/require"
)

// fakeTool records the context it was invoked with so tests can assert on
// values stamped onto it by the hookedTool decorator.
type fakeTool struct {
	name    string
	called  bool
	gotCtx  context.Context
	gotCall fantasy.ToolCall
	resp    fantasy.ToolResponse
	err     error
}

func (f *fakeTool) Info() fantasy.ToolInfo {
	return fantasy.ToolInfo{Name: f.name}
}

func (f *fakeTool) Run(ctx context.Context, call fantasy.ToolCall) (fantasy.ToolResponse, error) {
	f.called = true
	f.gotCtx = ctx
	f.gotCall = call
	return f.resp, f.err
}

func (f *fakeTool) ProviderOptions() fantasy.ProviderOptions     { return nil }
func (f *fakeTool) SetProviderOptions(_ fantasy.ProviderOptions) {}

type recordingHookRecorder struct {
	started   []RuntimeHookExecution
	completed []RuntimeHookExecution
	skipped   []RuntimeHookExecution
	blocked   []RuntimeHookExecution
	failed    []RuntimeHookExecution
	context   []RuntimeHookExecution
	rewritten []RuntimeHookExecution
}

func (r *recordingHookRecorder) HooksDiscovered(context.Context, []RuntimeHookConfig) error {
	return nil
}
func (r *recordingHookRecorder) HookExecutionStarted(_ context.Context, e RuntimeHookExecution) error {
	r.started = append(r.started, e)
	return nil
}
func (r *recordingHookRecorder) HookExecutionCompleted(_ context.Context, e RuntimeHookExecution) error {
	r.completed = append(r.completed, e)
	return nil
}
func (r *recordingHookRecorder) HookExecutionSkipped(_ context.Context, e RuntimeHookExecution) error {
	r.skipped = append(r.skipped, e)
	return nil
}
func (r *recordingHookRecorder) HookExecutionBlocked(_ context.Context, e RuntimeHookExecution) error {
	r.blocked = append(r.blocked, e)
	return nil
}
func (r *recordingHookRecorder) HookExecutionFailed(_ context.Context, e RuntimeHookExecution) error {
	r.failed = append(r.failed, e)
	return nil
}
func (r *recordingHookRecorder) HookContextInjected(_ context.Context, e RuntimeHookExecution) error {
	r.context = append(r.context, e)
	return nil
}
func (r *recordingHookRecorder) HookInputRewritten(_ context.Context, e RuntimeHookExecution) error {
	r.rewritten = append(r.rewritten, e)
	return nil
}

// newRunner builds a hooks.Runner from a single HookConfig, running the
// config-loader path that compiles the matcher regex.
func newRunner(t *testing.T, cmd string) *hooks.Runner {
	t.Helper()
	cfg := &config.Config{
		Hooks: map[string][]config.HookConfig{
			hooks.EventPreToolUse: {{Command: cmd}},
		},
	}
	require.NoError(t, cfg.ValidateHooks())
	return hooks.NewRunner(cfg.Hooks[hooks.EventPreToolUse], t.TempDir(), t.TempDir())
}

func newRunnerForEvents(t *testing.T, configs map[string][]config.HookConfig) *hooks.Runner {
	t.Helper()
	cfg := &config.Config{Hooks: configs}
	require.NoError(t, cfg.ValidateHooks())
	return hooks.NewRunnerForEvents(cfg.Hooks, t.TempDir(), t.TempDir())
}

func TestHookedTool_RecordsBeforeAndAfterLifecycle(t *testing.T) {
	t.Parallel()

	inner := &fakeTool{name: "view", resp: fantasy.NewTextResponse("ok")}
	recorder := &recordingHookRecorder{}
	runner := newRunnerForEvents(t, map[string][]config.HookConfig{
		hooks.EventPreToolUse:  {{Command: `echo '{"decision":"allow"}'`}},
		hooks.EventPostToolUse: {{Command: `echo '{"context":"post context"}'`}},
	})
	tool := newHookedToolWithRecorder(inner, runner, recorder)

	resp, err := tool.Run(t.Context(), fantasy.ToolCall{ID: "call-life", Name: "view", Input: "{}"})
	require.NoError(t, err)
	require.Equal(t, "ok\npost context", resp.Content)
	require.True(t, inner.called)
	require.Len(t, recorder.started, 2)
	require.Len(t, recorder.completed, 2)
	require.Len(t, recorder.context, 1)
	require.Equal(t, hooks.EventPreToolUse, recorder.completed[0].Event)
	require.Equal(t, hooks.EventPostToolUse, recorder.completed[1].Event)
	require.Equal(t, "completed", recorder.context[0].Status)
	require.NotZero(t, recorder.context[0].CompletedAt)
}

func TestHookedTool_RecordsPostFailureLifecycle(t *testing.T) {
	t.Parallel()

	inner := &fakeTool{name: "bash", resp: fantasy.NewTextErrorResponse("boom")}
	recorder := &recordingHookRecorder{}
	runner := newRunnerForEvents(t, map[string][]config.HookConfig{
		hooks.EventPostToolUseFailure: {{Command: `echo '{"context":"failure context"}'`}},
	})
	tool := newHookedToolWithRecorder(inner, runner, recorder)

	resp, err := tool.Run(t.Context(), fantasy.ToolCall{ID: "call-fail", Name: "bash", Input: `{"command":"bad"}`})
	require.NoError(t, err)
	require.True(t, resp.IsError)
	require.Contains(t, resp.Content, "failure context")
	require.Len(t, recorder.skipped, 1, "pre hook has no matching hooks")
	require.Len(t, recorder.started, 1)
	require.Len(t, recorder.completed, 1)
	require.Equal(t, hooks.EventPostToolUseFailure, recorder.completed[0].Event)
}

func TestHookedTool_RecordsRewriteAndContextWithSameExecution(t *testing.T) {
	t.Parallel()

	inner := &fakeTool{name: "bash", resp: fantasy.NewTextResponse("ok")}
	recorder := &recordingHookRecorder{}
	runner := newRunnerForEvents(t, map[string][]config.HookConfig{
		hooks.EventPreToolUse: {{Command: `echo '{"updated_input":{"command":"echo rewritten"},"context":"pre context"}'`}},
	})
	tool := newHookedToolWithRecorder(inner, runner, recorder)

	resp, err := tool.Run(t.Context(), fantasy.ToolCall{ID: "call-rewrite", Name: "bash", Input: `{"command":"echo original"}`})
	require.NoError(t, err)
	require.Contains(t, resp.Content, "pre context")
	require.JSONEq(t, `{"command":"echo rewritten"}`, inner.gotCall.Input)
	require.Len(t, recorder.rewritten, 1)
	require.Equal(t, recorder.completed[0].ID, recorder.rewritten[0].ID)
	require.Equal(t, "completed", recorder.rewritten[0].Status)
	require.NotZero(t, recorder.rewritten[0].CompletedAt)
	require.Len(t, recorder.context, 1)
	require.Equal(t, recorder.completed[0].ID, recorder.context[0].ID)
}

func TestHookedTool_RecordsHookFailureAndStillRunsTool(t *testing.T) {
	t.Parallel()

	inner := &fakeTool{name: "view", resp: fantasy.NewTextResponse("ok")}
	recorder := &recordingHookRecorder{}
	runner := newRunnerForEvents(t, map[string][]config.HookConfig{
		hooks.EventPreToolUse: {{Command: `this-command-does-not-exist`}},
	})
	tool := newHookedToolWithRecorder(inner, runner, recorder)

	resp, err := tool.Run(t.Context(), fantasy.ToolCall{ID: "call-hook-fail", Name: "view", Input: "{}"})
	require.NoError(t, err)
	require.Equal(t, "ok", resp.Content)
	require.True(t, inner.called)
	require.Len(t, recorder.started, 1)
	require.Len(t, recorder.completed, 1)
	require.Empty(t, recorder.failed)
	require.Equal(t, "none", recorder.completed[0].PolicyDecision)
}

func TestHookedTool_RecordsSkippedWhenNoHookMatches(t *testing.T) {
	t.Parallel()

	inner := &fakeTool{name: "view", resp: fantasy.NewTextResponse("ok")}
	recorder := &recordingHookRecorder{}
	runner := newRunnerForEvents(t, map[string][]config.HookConfig{
		hooks.EventPreToolUse: {{Command: `exit 0`, Matcher: "^bash$"}},
	})
	tool := newHookedToolWithRecorder(inner, runner, recorder)

	_, err := tool.Run(t.Context(), fantasy.ToolCall{ID: "call-skip", Name: "view", Input: "{}"})
	require.NoError(t, err)
	require.True(t, inner.called)
	require.Len(t, recorder.skipped, 2, "pre and post lifecycle events are both runtime-visible")
	require.Equal(t, hooks.EventPreToolUse, recorder.skipped[0].Event)
	require.Equal(t, hooks.EventPostToolUse, recorder.skipped[1].Event)
	for _, skipped := range recorder.skipped {
		require.Equal(t, "skipped", skipped.Status)
		require.NotZero(t, skipped.CompletedAt)
	}
}

func TestHookedTool_RecordsCancellationAsFailed(t *testing.T) {
	t.Parallel()

	inner := &fakeTool{name: "view", resp: fantasy.NewTextResponse("ok")}
	recorder := &recordingHookRecorder{}
	runner := newRunnerForEvents(t, map[string][]config.HookConfig{
		hooks.EventPreToolUse: {{Command: `sleep 1`, Timeout: 1}},
	})
	tool := newHookedToolWithRecorder(inner, runner, recorder)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	resp, err := tool.Run(ctx, fantasy.ToolCall{ID: "call-cancel", Name: "view", Input: "{}"})
	require.NoError(t, err)
	require.Equal(t, "ok", resp.Content)
	require.True(t, inner.called)
	require.Len(t, recorder.failed, 1)
	require.Equal(t, "failed", recorder.failed[0].Status)
	require.Contains(t, recorder.failed[0].Error, "context canceled")
}

func TestHookedTool_HookAllowCannotOverrideSchedulerPolicyDeny(t *testing.T) {
	t.Parallel()

	inner := &fakeTool{name: "bash", resp: fantasy.NewTextResponse("should not run")}
	schedulerRecorder := &recordingSchedulerRecorder{
		decision: SchedulerToolPolicyDecision{
			Decision:          string(permission.PolicyDeny),
			Risk:              string(permission.RiskExecute),
			Reason:            "headless ask fail-closed",
			Mode:              string(permission.PolicyModeAsk),
			Profile:           "headless",
			Headless:          true,
			HeadlessReason:    "ask requires interaction",
			SandboxDecisionID: "sandbox-deny-1",
			SandboxStatus:     "denied",
			RuleScopeKind:     "cwd",
			RuleScopeValue:    "C:\\restricted",
		},
	}
	scheduled := newSchedulerTool(inner, schedulerRecorder)
	recorder := &recordingHookRecorder{}
	runner := newRunnerForEvents(t, map[string][]config.HookConfig{
		hooks.EventPreToolUse: {{Command: `echo '{"decision":"allow","updated_input":{"command":"echo bypass"}}'`}},
	})
	tool := newHookedToolWithRecorder(scheduled, runner, recorder)

	resp, err := tool.Run(t.Context(), fantasy.ToolCall{ID: "call-policy-deny", Name: "bash", Input: `{"command":"rm -rf nope"}`})
	require.NoError(t, err)
	require.True(t, resp.IsError)
	require.True(t, resp.StopTurn)
	require.False(t, inner.called)
	require.Equal(t, "denied", schedulerRecorder.failed.Status)
	require.Contains(t, schedulerRecorder.failed.PolicyReason, "headless ask fail-closed")
	require.Len(t, recorder.completed, 1)
	require.Len(t, recorder.rewritten, 1)
	require.Len(t, recorder.skipped, 1, "failed scheduler result still records the post-failure hook path")
}

func TestHookedTool_AllowDoesNotStampHookApproval(t *testing.T) {
	t.Parallel()

	inner := &fakeTool{name: "view", resp: fantasy.NewTextResponse("ok")}
	runner := newRunner(t, `echo '{"decision":"allow"}'`)
	tool := newHookedToolWithRecorder(inner, runner, nil)

	_, err := tool.Run(t.Context(), fantasy.ToolCall{ID: "call-1", Name: "view"})
	require.NoError(t, err)
	require.True(t, inner.called, "inner tool should have run")

	// Hook allow is an advisory/interaction result only. Runtime policy remains
	// the authority, so the inner tool must not see a permission bypass stamp.
	svc := permission.NewPermissionService(t.TempDir(), false, nil)
	ctx, cancel := context.WithCancel(inner.gotCtx)
	cancel()
	granted, err := svc.Request(ctx, permission.CreatePermissionRequest{
		SessionID:  "s1",
		ToolCallID: "call-1",
		ToolName:   "view",
		Action:     "read",
		Path:       t.TempDir(),
	})
	require.Error(t, err)
	require.False(t, granted, "hook allow must not bypass runtime permission policy")
}

func TestHookedTool_SilentDoesNotStampApproval(t *testing.T) {
	t.Parallel()

	inner := &fakeTool{name: "view", resp: fantasy.NewTextResponse("ok")}
	runner := newRunner(t, `exit 0`) // no stdout, no decision
	tool := newHookedToolWithRecorder(inner, runner, nil)

	_, err := tool.Run(t.Context(), fantasy.ToolCall{ID: "call-2", Name: "view"})
	require.NoError(t, err)
	require.True(t, inner.called)

	// With no hook opinion, a fresh permission request has nothing stamped
	// and must fall through to the normal flow. We verify by checking that
	// the context does not look pre-approved for this call ID: sending a
	// request that no subscriber resolves will block until cancelled.
	svc := permission.NewPermissionService(t.TempDir(), false, nil)
	ctx, cancel := context.WithCancel(inner.gotCtx)
	cancel()
	granted, err := svc.Request(ctx, permission.CreatePermissionRequest{
		SessionID:  "s1",
		ToolCallID: "call-2",
		ToolName:   "view",
		Action:     "read",
		Path:       t.TempDir(),
	})
	require.Error(t, err, "no approval stamped => request should reach the prompt path")
	require.False(t, granted)
}

func TestHookedTool_DenySkipsInnerTool(t *testing.T) {
	t.Parallel()

	inner := &fakeTool{name: "bash"}
	runner := newRunner(t, `echo "blocked" >&2; exit 2`)
	tool := newHookedToolWithRecorder(inner, runner, nil)

	resp, err := tool.Run(t.Context(), fantasy.ToolCall{ID: "call-3", Name: "bash"})
	require.NoError(t, err)
	require.False(t, inner.called, "denied call must not reach the inner tool")
	require.True(t, resp.IsError)
	require.Contains(t, resp.Content, "blocked")
}

func TestWrapToolsWithHooks(t *testing.T) {
	t.Parallel()

	runner := newRunner(t, `exit 0`)
	inputs := []fantasy.AgentTool{&fakeTool{name: "a"}, &fakeTool{name: "b"}}

	t.Run("top-level agent wraps every tool", func(t *testing.T) {
		t.Parallel()
		out := wrapToolsWithHooks(inputs, runner, nil, false)
		require.Len(t, out, len(inputs))
		for i, tool := range out {
			_, ok := tool.(*hookedTool)
			require.Truef(t, ok, "tool %d should be a *hookedTool", i)
		}
	})

	t.Run("sub-agent skips the wrap", func(t *testing.T) {
		t.Parallel()
		out := wrapToolsWithHooks(inputs, runner, nil, true)
		require.Equal(t, inputs, out, "sub-agent tools should be returned unwrapped")
		for _, tool := range out {
			_, isHooked := tool.(*hookedTool)
			require.False(t, isHooked, "sub-agent tool should not be wrapped")
		}
	})

	t.Run("nil runner skips the wrap for both agent kinds", func(t *testing.T) {
		t.Parallel()
		require.Equal(t, inputs, wrapToolsWithHooks(inputs, nil, nil, false))
		require.Equal(t, inputs, wrapToolsWithHooks(inputs, nil, nil, true))
	})
}
