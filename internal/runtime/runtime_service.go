package runtime

import (
	"time"

	"github.com/CIPFZ/agent-builder/internal/config"
	"github.com/CIPFZ/agent-builder/internal/tools/scheduler"
)

func NewRuntimeService() RuntimeService {
	return newRuntimeService()
}

func newRuntimeService() *runtimeService {
	service := &runtimeService{
		requests: make(map[string]runtimeRequestState),

		sessionTurns:          make(map[string]string),
		activeSessionStatuses: make(map[string]RuntimeActiveSessionStatus),

		toolEvents: make(map[string]runtimeToolEventState),

		toolCalls:        scheduler.New(NewRuntimeToolCallStore()),
		objects:          runtimeObjectStore{},
		promptAssemblies: runtimePromptAssemblyStore{},
		worktrees:        runtimeWorktreeStore{},
		sandboxDecisions: runtimeSandboxDecisionStore{},
		hookExecutions:   runtimeHookExecutionStore{},
		runs:             runtimeRunStore{},
		transitions:      runtimeRunTransitionStore{},
		recoveryLinks:    runtimeRecoveryLinkStore{},

		agentTasks: runtimeAgentTaskStore{},

		permissions:           make(map[string]pendingRuntimePermission),
		policy:                defaultRuntimePolicy(),
		capabilityLoads:       make(map[string]runtimeCapabilityLoadRecord),
		toolDiscovery:         newRuntimeToolDiscoveryState(),
		resourceGovernor:      defaultRuntimeResourceGovernor(),
		capabilityResources:   make(map[string]func()),
		mcpIdleTimers:         make(map[string]*time.Timer),
		mcpIdleTTL:            defaultRuntimeMCPIdleTTL,
		mcpServerProjects:     make(map[string]string),
		projectMCPServers:     make(map[string]map[string]struct{}),
		mcpServerConfigs:      make(map[string]*config.ConfigStore),
		projectCapabilityUsed: make(map[string]int64),
		terminalsByID:         make(map[string]*runtimeTerminalState),
		terminalIDsBySession:  make(map[string]map[string]struct{}),

		eventStream: newRuntimeEventBroker(),

		messageStream: make(map[string]*messageStreamCursor),

		compactTurnStates: make(map[string]runtimeTurnCompactState),
		compactOperations: make(map[string]bool),

		compactFailures:        make(map[string]int),
		diagnosticChecks:       make(map[string]RuntimeTargetedDiagnostic),
		conversationV2Deferred: make(map[string]bool),
		conversationV2Pending:  make(map[string]map[int64]RuntimeEvent),
	}
	service.turnDispatcher = newRuntimeResourceDispatcher(service.resourceGovernor, runtimeResourceTurnWorkingSet, service.runQueuedModelTurn)

	return service
}
