package runtime

import "github.com/charmbracelet/crush/internal/tools/scheduler"

func NewRuntimeService() RuntimeService {

	return newRuntimeService()

}

func newRuntimeService() *runtimeService {

	service := &runtimeService{

		requests: make(map[string]runtimeRequestState),

		sessionTurns: make(map[string]string),

		toolEvents: make(map[string]runtimeToolEventState),

		toolCalls:         scheduler.New(NewRuntimeToolCallStore()),
		refs:              runtimeRefStore{},
		compactBoundaries: runtimeCompactBoundaryStore{},
		worktrees:         runtimeWorktreeStore{},
		sandboxDecisions:  runtimeSandboxDecisionStore{},
		hookExecutions:    runtimeHookExecutionStore{},
		runs:              runtimeRunStore{},
		transitions:       runtimeRunTransitionStore{},

		agentTasks: runtimeAgentTaskStore{},

		permissions:     make(map[string]pendingRuntimePermission),
		policy:          defaultRuntimePolicy(),
		capabilityLoads: make(map[string]runtimeCapabilityLoadRecord),
		toolDiscovery:   newRuntimeToolDiscoveryState(),
		terminals:       make(map[string]*runtimeTerminalState),

		eventStream: newRuntimeSSEServer(),
	}

	service.httpAPI = newRuntimeHTTPServer(service)

	return service
}
