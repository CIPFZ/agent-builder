package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"charm.land/fantasy"
	"github.com/charmbracelet/crush/internal/agent/tools"
	"github.com/charmbracelet/crush/internal/hooks"
	"github.com/tidwall/sjson"
)

// hookedTool wraps a fantasy.AgentTool to run configured hook lifecycle
// events around the inner tool.
type hookedTool struct {
	inner    fantasy.AgentTool
	runner   *hooks.Runner
	recorder RuntimeHookRecorder
}

func newHookedTool(inner fantasy.AgentTool, runner *hooks.Runner, recorder SchedulerRecorder) *hookedTool {
	var hookRecorder RuntimeHookRecorder
	if recorder != nil {
		hookRecorder, _ = recorder.(RuntimeHookRecorder)
	}
	return newHookedToolWithRecorder(inner, runner, hookRecorder)
}

func newHookedToolWithRecorder(inner fantasy.AgentTool, runner *hooks.Runner, recorder RuntimeHookRecorder) *hookedTool {
	return &hookedTool{inner: inner, runner: runner, recorder: recorder}
}

// wrapToolsWithHooks returns a tool slice with each entry wrapped in a
// hookedTool. Returns the original slice unchanged when runner is nil or
// when isSubAgent is true — sub-agents never fire hooks, the top-level
// invocation of the sub-agent tool itself is wrapped on the caller's side.
func wrapToolsWithHooks(tools []fantasy.AgentTool, runner *hooks.Runner, recorder SchedulerRecorder, isSubAgent bool) []fantasy.AgentTool {
	if runner == nil || isSubAgent {
		return tools
	}
	out := make([]fantasy.AgentTool, len(tools))
	for i, tool := range tools {
		out[i] = newHookedTool(tool, runner, recorder)
	}
	return out
}

func (h *hookedTool) Info() fantasy.ToolInfo {
	return h.inner.Info()
}

func (h *hookedTool) ProviderOptions() fantasy.ProviderOptions {
	return h.inner.ProviderOptions()
}

func (h *hookedTool) SetProviderOptions(opts fantasy.ProviderOptions) {
	h.inner.SetProviderOptions(opts)
}

func (h *hookedTool) Run(ctx context.Context, call fantasy.ToolCall) (fantasy.ToolResponse, error) {
	sessionID := tools.GetSessionFromContext(ctx)
	turnID := tools.GetTurnFromContext(ctx)
	toolName := nonEmptyString(call.Name, h.inner.Info().Name)
	base := h.hookExecutionBase(ctx, hooks.EventPreToolUse, sessionID, turnID, call.ID, toolName, call.Input)
	result, err := h.runHookEvent(ctx, base, call.Input, "")
	if err != nil {
		slog.Warn("Hook execution error, proceeding with tool call when policy allows",
			"tool", call.Name, "error", err)
	}

	if result.Decision == hooks.DecisionDeny || result.Halt {
		reason := fmt.Sprintf("Tool call blocked by hook. Reason: %s", result.Reason)
		if result.Halt {
			reason = fmt.Sprintf("Turn halted by hook. Reason: %s", result.Reason)
		}
		resp := fantasy.NewTextErrorResponse(reason)
		// Halt ends the whole turn; a plain deny only blocks this tool
		// call so the model can see the error and try something else.
		resp.StopTurn = result.Halt
		resp.Metadata = hookMetadataJSON(result)
		h.recordHookAggregate(ctx, base, result, "blocked", reason, "")
		return resp, nil
	}

	if result.UpdatedInput != "" {
		call.Input = result.UpdatedInput
		if h.recorder != nil {
			rewriteExec := base
			rewriteExec.Status = "completed"
			rewriteExec.InputRewritten = true
			rewriteExec.InputSummary = call.Input
			rewriteExec.Reason = "hook input rewrite accepted"
			rewriteExec.CompletedAt = time.Now().UnixMilli()
			rewriteExec.DurationMS = maxInt64(0, rewriteExec.CompletedAt-rewriteExec.StartedAt)
			_ = h.recorder.HookInputRewritten(ctx, rewriteExec)
		}
	}

	resp, err := h.inner.Run(ctx, call)
	if result.Context != "" {
		if resp.Content != "" {
			resp.Content += "\n"
		}
		resp.Content += result.Context
		if h.recorder != nil {
			contextExec := base
			contextExec.Status = "completed"
			contextExec.ContextInjected = true
			contextExec.ContextSummary = result.Context
			contextExec.CompletedAt = time.Now().UnixMilli()
			contextExec.DurationMS = maxInt64(0, contextExec.CompletedAt-contextExec.StartedAt)
			_ = h.recorder.HookContextInjected(ctx, contextExec)
		}
	}

	postEvent := hooks.EventPostToolUse
	output := resp.Content
	if err != nil || resp.IsError {
		postEvent = hooks.EventPostToolUseFailure
		output = responseError(resp, err)
	}
	postBase := h.hookExecutionBase(ctx, postEvent, sessionID, turnID, call.ID, toolName, call.Input)
	postResult, postErr := h.runHookEvent(ctx, postBase, call.Input, output)
	if postErr != nil && err == nil {
		slog.Warn("Post hook execution error", "tool", call.Name, "error", postErr)
	}
	if postResult.Context != "" {
		if resp.Content != "" {
			resp.Content += "\n"
		}
		resp.Content += postResult.Context
		if h.recorder != nil {
			contextExec := postBase
			contextExec.Status = "completed"
			contextExec.ContextInjected = true
			contextExec.ContextSummary = postResult.Context
			contextExec.CompletedAt = time.Now().UnixMilli()
			contextExec.DurationMS = maxInt64(0, contextExec.CompletedAt-contextExec.StartedAt)
			_ = h.recorder.HookContextInjected(ctx, contextExec)
		}
	}
	if postResult.Decision == hooks.DecisionDeny || postResult.Halt {
		reason := fmt.Sprintf("Tool result blocked by hook. Reason: %s", postResult.Reason)
		h.recordHookAggregate(ctx, postBase, postResult, "blocked", reason, output)
		resp.IsError = true
		resp.StopTurn = true
		if resp.Content != "" {
			resp.Content += "\n"
		}
		resp.Content += reason
	}
	resp.Metadata = mergeHookMetadata(mergeHookMetadata(resp.Metadata, result), postResult)
	return resp, nil
}

func (h *hookedTool) runHookEvent(ctx context.Context, base RuntimeHookExecution, input, output string) (hooks.AggregateResult, error) {
	if h.runner == nil {
		return hooks.AggregateResult{Decision: hooks.DecisionNone}, nil
	}
	base.OutputSummary = output
	if !h.runner.HasMatchingHooks(base.Event, base.HookName) {
		base.Status = "skipped"
		base.Reason = "no matching hooks"
		base.CompletedAt = time.Now().UnixMilli()
		base.DurationMS = maxInt64(0, base.CompletedAt-base.StartedAt)
		if h.recorder != nil {
			_ = h.recorder.HookExecutionSkipped(ctx, base)
		}
		return hooks.AggregateResult{Decision: hooks.DecisionNone}, nil
	}
	if h.recorder != nil {
		started := base
		started.Status = "running"
		_ = h.recorder.HookExecutionStarted(ctx, started)
	}
	result, err := h.runner.Run(ctx, base.Event, base.SessionID, base.HookName, input, output)
	completed := base
	completed.CompletedAt = time.Now().UnixMilli()
	completed.DurationMS = maxInt64(0, completed.CompletedAt-completed.StartedAt)
	completed.InputRewritten = result.UpdatedInput != ""
	completed.ContextInjected = result.Context != ""
	completed.ContextSummary = result.Context
	completed.OutputSummary = nonEmptyString(output, result.Reason)
	completed.Reason = result.Reason
	completed.PolicyDecision = result.Decision.String()
	if err != nil {
		completed.Status = "failed"
		completed.Error = err.Error()
		if h.recorder != nil {
			_ = h.recorder.HookExecutionFailed(ctx, completed)
		}
		return result, err
	}
	if ctx.Err() != nil {
		completed.Status = "failed"
		completed.Error = ctx.Err().Error()
		if h.recorder != nil {
			_ = h.recorder.HookExecutionFailed(ctx, completed)
		}
		return result, nil
	}
	if result.Decision == hooks.DecisionDeny || result.Halt {
		completed.Status = "blocked"
		if h.recorder != nil {
			_ = h.recorder.HookExecutionBlocked(ctx, completed)
		}
		return result, nil
	}
	completed.Status = "completed"
	if h.recorder != nil {
		_ = h.recorder.HookExecutionCompleted(ctx, completed)
	}
	return result, nil
}

func (h *hookedTool) hookExecutionBase(ctx context.Context, event, sessionID, turnID, toolCallID, toolName, input string) RuntimeHookExecution {
	started := time.Now().UnixMilli()
	return RuntimeHookExecution{
		ID:                "hook_" + fmt.Sprintf("%d", started) + "_" + newRequestFragment() + "_" + strings.ToLower(event),
		HookID:            "aggregate:" + event + ":" + toolName,
		HookName:          toolName,
		HookSource:        "config",
		Event:             event,
		Status:            "running",
		SessionID:         sessionID,
		TurnID:            turnID,
		ToolCallID:        toolCallID,
		CapabilityID:      schedulerCapabilityIDForAgentTool(h.inner, toolName),
		PolicyMode:        "",
		PolicyProfile:     "",
		SandboxDecisionID: sandboxDecisionIDFromContext(ctx),
		SandboxStatus:     sandboxStatusFromContext(ctx),
		InputSummary:      input,
		StartedAt:         started,
	}
}

func (h *hookedTool) recordHookAggregate(ctx context.Context, base RuntimeHookExecution, result hooks.AggregateResult, status, reason, output string) {
	if h.recorder == nil {
		return
	}
	base.Status = status
	base.Reason = nonEmptyString(reason, result.Reason)
	base.OutputSummary = output
	base.ContextSummary = result.Context
	base.InputRewritten = result.UpdatedInput != ""
	base.ContextInjected = result.Context != ""
	base.PolicyDecision = result.Decision.String()
	base.CompletedAt = time.Now().UnixMilli()
	base.DurationMS = maxInt64(0, base.CompletedAt-base.StartedAt)
	if status == "blocked" {
		_ = h.recorder.HookExecutionBlocked(ctx, base)
		return
	}
	_ = h.recorder.HookExecutionCompleted(ctx, base)
}

// buildHookMetadata creates a HookMetadata from an AggregateResult.
func buildHookMetadata(result hooks.AggregateResult) hooks.HookMetadata {
	return hooks.HookMetadata{
		HookCount:    result.HookCount,
		Decision:     result.Decision.String(),
		Halt:         result.Halt,
		Reason:       result.Reason,
		InputRewrite: result.UpdatedInput != "",
		Hooks:        result.Hooks,
	}
}

// hookMetadataJSON builds a JSON string containing only the hook metadata.
func hookMetadataJSON(result hooks.AggregateResult) string {
	meta := buildHookMetadata(result)
	data, err := json.Marshal(meta)
	if err != nil {
		return ""
	}
	return `{"hook":` + string(data) + `}`
}

// mergeHookMetadata injects hook metadata into existing tool metadata.
func mergeHookMetadata(existing string, result hooks.AggregateResult) string {
	if result.HookCount == 0 {
		return existing
	}
	meta := buildHookMetadata(result)
	data, err := json.Marshal(meta)
	if err != nil {
		return existing
	}
	if existing == "" {
		existing = "{}"
	}
	merged, err := sjson.SetRaw(existing, "hook", string(data))
	if err != nil {
		return existing
	}
	return merged
}

func hookConfigsForRuntime(runner *hooks.Runner) []RuntimeHookConfig {
	if runner == nil {
		return nil
	}
	configs := runner.HooksByEvent()
	var out []RuntimeHookConfig
	for event, items := range configs {
		for _, item := range items {
			out = append(out, RuntimeHookConfig{
				Name:    item.Command,
				Source:  "config",
				Event:   event,
				Matcher: item.Matcher,
				Enabled: item.Command != "",
				Timeout: item.Timeout,
			})
		}
	}
	return out
}

func sandboxDecisionIDFromContext(ctx context.Context) string {
	if meta, ok := tools.SandboxMetadataFromContext(ctx); ok {
		return meta.DecisionID
	}
	return ""
}

func sandboxStatusFromContext(ctx context.Context) string {
	if meta, ok := tools.SandboxMetadataFromContext(ctx); ok {
		return meta.Status
	}
	return ""
}

func maxInt64(a, b int64) int64 {
	if a > b {
		return a
	}
	return b
}

func newRequestFragment() string {
	return strings.ReplaceAll(fmt.Sprintf("%p", &struct{}{}), "0x", "")
}
