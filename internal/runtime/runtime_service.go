package runtime

import "github.com/CIPFZ/agent-builder/internal/tools/scheduler"

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
		promptAssemblies:  runtimePromptAssemblyStore{},
		worktrees:         runtimeWorktreeStore{},
		sandboxDecisions:  runtimeSandboxDecisionStore{},
		hookExecutions:    runtimeHookExecutionStore{},
		runs:              runtimeRunStore{},
		transitions:       runtimeRunTransitionStore{},
		recoveryLinks:     runtimeRecoveryLinkStore{},

		agentTasks: runtimeAgentTaskStore{},

		permissions:          make(map[string]pendingRuntimePermission),
		policy:               defaultRuntimePolicy(),
		capabilityLoads:      make(map[string]runtimeCapabilityLoadRecord),
		toolDiscovery:        newRuntimeToolDiscoveryState(),
		terminalsByID:        make(map[string]*runtimeTerminalState),
		terminalIDsBySession: make(map[string]map[string]struct{}),

		eventStream: newRuntimeSSEServer(),
	}

	service.httpAPI = newRuntimeHTTPServer(service)

	return service
}
