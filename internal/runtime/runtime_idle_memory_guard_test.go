package runtime

import "testing"

func TestRuntimeIdleMemoryGuardRequiresTrulyIdleClientAndRuntime(t *testing.T) {
	idle := RuntimeIdleMemoryGuardRequest{ClientIdleMS: runtimeIdleMemoryGuardMinimumIdleMS}
	if got := evaluateRuntimeIdleMemoryGuard(idle, RuntimeStatus{}, 0); !got.Eligible || got.Reason != "" {
		t.Fatalf("idle response = %#v", got)
	}

	tests := []struct {
		name   string
		req    RuntimeIdleMemoryGuardRequest
		status RuntimeStatus
		perms  int
		reason string
	}{
		{name: "minimum idle", req: RuntimeIdleMemoryGuardRequest{ClientIdleMS: runtimeIdleMemoryGuardMinimumIdleMS - 1}, reason: "client_not_idle"},
		{name: "draft", req: RuntimeIdleMemoryGuardRequest{ClientIdleMS: runtimeIdleMemoryGuardMinimumIdleMS, HasUnsavedDraft: true}, reason: "unsaved_draft"},
		{name: "overlay", req: RuntimeIdleMemoryGuardRequest{ClientIdleMS: runtimeIdleMemoryGuardMinimumIdleMS, HasActiveOverlay: true}, reason: "active_overlay"},
		{name: "terminal interaction", req: RuntimeIdleMemoryGuardRequest{ClientIdleMS: runtimeIdleMemoryGuardMinimumIdleMS, HasTerminalInteraction: true}, reason: "terminal_interaction"},
		{name: "permission", req: idle, perms: 1, reason: "pending_permission"},
		{name: "turn", req: idle, status: RuntimeStatus{Requests: RuntimeRequests{Running: 1}}, reason: "active_turn"},
		{name: "background session", req: idle, status: RuntimeStatus{ActiveSessions: []RuntimeActiveSessionStatus{{Status: turnStatusQueued}}}, reason: "active_session"},
		{name: "resource", req: idle, status: RuntimeStatus{ResourceGovernor: RuntimeResourceGovernorStatus{Resources: []RuntimeResourceStatus{{Kind: string(runtimeResourceBrowserWorker), InUseCount: 1}}}}, reason: "resident_resource"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got := evaluateRuntimeIdleMemoryGuard(test.req, test.status, test.perms)
			if got.Eligible || got.Reason != test.reason || got.MinimumIdleMS != runtimeIdleMemoryGuardMinimumIdleMS {
				t.Fatalf("response = %#v, want reason %q", got, test.reason)
			}
		})
	}
}

func TestRuntimeIdleMemoryGuardAllowsBoundedIdleTerminal(t *testing.T) {
	status := RuntimeStatus{ResourceGovernor: RuntimeResourceGovernorStatus{Resources: []RuntimeResourceStatus{{
		Kind: string(runtimeResourceTerminal), InUseCount: 1, InUseBytes: runtimeTerminalMaxEventBytes,
	}}}}
	got := evaluateRuntimeIdleMemoryGuard(RuntimeIdleMemoryGuardRequest{ClientIdleMS: runtimeIdleMemoryGuardMinimumIdleMS}, status, 0)
	if !got.Eligible {
		t.Fatalf("idle terminal response = %#v", got)
	}
}
