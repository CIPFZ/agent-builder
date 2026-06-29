package runtime

import (
	"context"
	"strings"
	"time"
)

const (
	interruptionKindPrompt          = "interrupted_prompt"
	interruptionKindGeneration      = "interrupted_generation"
	interruptionKindToolCall        = "interrupted_tool_call"
	interruptionKindExternalRequest = "interrupted_external_request"
	interruptionKindPermission      = "interrupted_permission"
	interruptionKindMCP             = "interrupted_mcp"
	interruptionKindHook            = "interrupted_hook"
	interruptionKindUnknown         = "unknown"

	recoverableErrorContextLengthExceeded      = "context_length_exceeded"
	recoverableErrorRateLimited                = "rate_limited"
	recoverableErrorOverloaded                 = "overloaded"
	recoverableErrorNetworkTransient           = "network_transient"
	recoverableErrorAuthExpired                = "auth_expired"
	recoverableErrorModelNotFound              = "model_not_found"
	recoverableErrorModelCapabilityUnsupported = "model_capability_unsupported"
	recoverableErrorPermissionRequired         = "permission_required"
	recoverableErrorPolicyDenied               = "policy_denied"
	recoverableErrorProviderFallbackAvailable  = "provider_fallback_available"
	recoverableErrorUnknown                    = "unknown"
)

func (r *runtimeService) classifyInterruptedTurn(ctx context.Context, turn RuntimeTurn) RuntimeRecoveredTurn {
	recovered := RuntimeRecoveredTurn{
		RuntimeTurn:      turn,
		InterruptionKind: interruptionKindUnknown,
		ResumeEligible:   turn.Status == turnStatusInterrupted,
		DiscardEligible:  turn.Status == turnStatusInterrupted,
		MarkDoneEligible: turn.Status == turnStatusInterrupted,
		Reason:           firstNonEmpty(turn.Error, "runtime interrupted before turn completed"),
		ResumeHint:       "Continue in a new turn from the last reliable runtime state. Unfinished tool calls will not be replayed.",
	}
	if turn.Status != turnStatusInterrupted {
		return recovered
	}
	if r.toolCalls != nil {
		if calls, err := r.toolCalls.ListCalls(ctx, turn.ID); err == nil {
			for _, call := range calls {
				rtCall := toRuntimeToolCall(call)
				if rtCall.Status == "running" || rtCall.Status == "queued" || rtCall.Status == "cancelled" {
					recovered.OpenToolCalls = append(recovered.OpenToolCalls, rtCall)
				}
			}
		}
	}
	if r.permissionStore.db != nil {
		if perms, err := r.permissionStore.ListBySession(ctx, turn.SessionID); err == nil {
			for _, perm := range perms {
				if perm.TurnID == turn.ID && perm.Status == permissionStatusPending {
					recovered.InterruptionKind = interruptionKindPermission
					return recovered
				}
			}
		}
	}
	if r.mcpRequestStore.db != nil {
		if reqs, err := r.mcpRequestStore.List(ctx, RuntimeMCPRequestListRequest{Status: mcpRequestStatusPending}); err == nil {
			for _, req := range reqs {
				if req.TurnID == turn.ID {
					recovered.InterruptionKind = interruptionKindMCP
					return recovered
				}
			}
		}
	}
	if r.hookExecutions.db != nil {
		if hooks, err := r.hookExecutions.List(ctx, RuntimeHookExecutionsRequest{TurnID: turn.ID}); err == nil {
			for _, hook := range hooks {
				if hook.Status == hookStatusRunning || strings.Contains(strings.ToLower(hook.Error), "interrupted") {
					recovered.InterruptionKind = interruptionKindHook
					return recovered
				}
			}
		}
	}
	if len(recovered.OpenToolCalls) > 0 {
		recovered.InterruptionKind = interruptionKindToolCall
		return recovered
	}
	if strings.Contains(strings.ToLower(turn.Error), "permission") {
		recovered.InterruptionKind = interruptionKindPermission
	} else if strings.Contains(strings.ToLower(turn.Error), "mcp") {
		recovered.InterruptionKind = interruptionKindMCP
	} else if turn.LatestAssistantMessageID != "" || turn.LatestMessageID != "" {
		recovered.InterruptionKind = interruptionKindGeneration
	} else if turn.UserMessageID != "" || turn.PromptPreview != "" {
		recovered.InterruptionKind = interruptionKindPrompt
	}
	return recovered
}

func classifyRuntimeRecoverableError(turn RuntimeTurn) (RuntimeRecoverableError, bool) {
	if turn.Status != turnStatusFailed || strings.TrimSpace(turn.Error) == "" {
		return RuntimeRecoverableError{}, false
	}
	kind, retryable, compactEligible, userAction := classifyRuntimeProviderErrorText(turn.Error)
	err := RuntimeRecoverableError{
		ID:              "turn:" + turn.ID,
		Kind:            kind,
		Severity:        "error",
		SessionID:       turn.SessionID,
		TurnID:          turn.ID,
		Provider:        turn.Provider,
		Model:           turn.Model,
		Message:         turn.Error,
		RetryEligible:   retryable,
		CompactEligible: compactEligible,
		UserAction:      userAction,
		CreatedAt:       time.UnixMilli(firstNonZeroInt64(turn.FinishedAt, turn.StartedAt)).UTC().Format(time.RFC3339Nano),
		Details: map[string]any{
			"prompt_preview": turn.PromptPreview,
		},
	}
	return err, kind != recoverableErrorUnknown || retryable || compactEligible || userAction != ""
}

func classifyRuntimeProviderErrorText(text string) (kind string, retryable bool, compactEligible bool, userAction string) {
	lower := strings.ToLower(text)
	switch {
	case strings.Contains(lower, "context length") || strings.Contains(lower, "maximum context") || strings.Contains(lower, "prompt is too long") || strings.Contains(lower, "too many tokens"):
		return recoverableErrorContextLengthExceeded, false, true, "compact_context"
	case strings.Contains(lower, "rate limit") || strings.Contains(lower, "too many requests") || strings.Contains(lower, "429"):
		return recoverableErrorRateLimited, true, false, ""
	case strings.Contains(lower, "overloaded") || strings.Contains(lower, "temporarily unavailable") || strings.Contains(lower, "503"):
		return recoverableErrorOverloaded, true, false, ""
	case strings.Contains(lower, "timeout") || strings.Contains(lower, "connection reset") || strings.Contains(lower, "connection refused"):
		return recoverableErrorNetworkTransient, true, false, ""
	case strings.Contains(lower, "unauthorized") || strings.Contains(lower, "api key") || strings.Contains(lower, "authentication"):
		return recoverableErrorAuthExpired, false, false, "refresh_auth"
	case strings.Contains(lower, "model not found") || strings.Contains(lower, "unknown model"):
		return recoverableErrorModelNotFound, false, false, "select_model"
	case strings.Contains(lower, "unsupported") || strings.Contains(lower, "does not support") || strings.Contains(lower, "image"):
		return recoverableErrorModelCapabilityUnsupported, false, false, "adjust_input_or_model"
	case strings.Contains(lower, "permission required"):
		return recoverableErrorPermissionRequired, false, false, "grant_permission"
	case strings.Contains(lower, "policy denied"):
		return recoverableErrorPolicyDenied, false, false, "change_policy"
	default:
		return recoverableErrorUnknown, false, false, ""
	}
}
