package runtime

import "github.com/CIPFZ/agent-builder/internal/runtimeapi"

const (
	runtimeRecoveryActionSourceKind             = "recovery_action"
	runtimeRecoveryActionResumeInterruptedTurn  = "resume_interrupted_turn"
	runtimeRecoveryActionDiscardInterruptedTurn = "discard_interrupted_turn"
	runtimeRecoveryActionRetryRecoverableError  = "retry_recoverable_error"
	runtimeRecoveryActionProviderCompactRetry   = "provider_compact_retry"
	runtimeRecoveryActionReasonResumed          = "interrupted_turn_resumed"
	runtimeRecoveryActionReasonDiscarded        = "interrupted_turn_discarded"
	runtimeRecoveryActionReasonRetryStarted     = "recoverable_error_retry_started"
	runtimeRecoveryActionReasonRetryNotAccepted = "recoverable_error_retry_not_accepted"
)

func runtimeRecoveryRefreshTargets() []string {
	return []string{
		"status",
		"recovery",
		"turn_activity",
		"session_activity_window",
		"session_activity",
		"tool_calls",
		"run",
		"run_projection",
		"run_transition_history",
		"diagnostics",
		"permissions",
		"mcp_requests",
		"run_scheduler_plan",
	}
}

func runtimeRecoveryActionMetadata(action, reason, idempotentBy string, startsWorker bool, evidence ...string) *RuntimeWriteActionMetadata {
	if len(evidence) == 0 {
		evidence = []string{
			"runtime_turns",
			"runtime_recovery_links",
			"runtime_events",
			"runtime_audit",
			"runtime_run_transitions",
			"runtime_run_projection",
			"session_activity",
		}
	}
	return &RuntimeWriteActionMetadata{
		Accepted:       true,
		Reason:         reason,
		RefreshTargets: runtimeRecoveryRefreshTargets(),
		Source: RuntimeWriteActionSource{
			Kind:                  runtimeRecoveryActionSourceKind,
			Action:                action,
			WorkbenchOnly:         true,
			StartsWorker:          startsWorker,
			IdempotentBy:          idempotentBy,
			SessionActivityParity: true,
			Evidence:              evidence,
		},
	}
}

func recoveryEventTypeForAction(action string) string {
	switch action {
	case runtimeRecoveryActionResumeInterruptedTurn:
		return runtimeapi.EventRecoveryTurnResumed
	case runtimeRecoveryActionDiscardInterruptedTurn:
		return runtimeapi.EventRecoveryTurnDiscarded
	case runtimeRecoveryActionRetryRecoverableError:
		return runtimeapi.EventRecoveryRetryStarted
	default:
		return runtimeapi.EventRecoveryStatusChanged
	}
}
