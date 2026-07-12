package runtime

import (
	"os"
	"strings"

	"github.com/CIPFZ/agent-builder/internal/tools/scheduler"
)

func NewRuntimeService() RuntimeService {

	return newRuntimeService()

}

func newRuntimeService() *runtimeService {

	service := &runtimeService{

		requests: make(map[string]runtimeRequestState),

		sessionTurns: make(map[string]string),

		toolEvents: make(map[string]runtimeToolEventState),

		toolCalls:        scheduler.New(NewRuntimeToolCallStore()),
		refs:             runtimeRefStore{},
		promptAssemblies: runtimePromptAssemblyStore{},
		worktrees:        runtimeWorktreeStore{},
		sandboxDecisions: runtimeSandboxDecisionStore{},
		hookExecutions:   runtimeHookExecutionStore{},
		runs:             runtimeRunStore{},
		transitions:      runtimeRunTransitionStore{},
		recoveryLinks:    runtimeRecoveryLinkStore{},

		agentTasks: runtimeAgentTaskStore{},

		permissions:          make(map[string]pendingRuntimePermission),
		policy:               defaultRuntimePolicy(),
		capabilityLoads:      make(map[string]runtimeCapabilityLoadRecord),
		toolDiscovery:        newRuntimeToolDiscoveryState(),
		terminalsByID:        make(map[string]*runtimeTerminalState),
		terminalIDsBySession: make(map[string]map[string]struct{}),

		eventStream: newRuntimeEventBroker(),

		messageStream: make(map[string]*messageStreamCursor),

		compactTurnStates: make(map[string]runtimeTurnCompactState),

		compactFailures:        make(map[string]int),
		conversationMode:       runtimeConversationModeFromEnvironment(),
		conversationV2Deferred: make(map[string]bool),
		conversationV2Pending:  make(map[string]map[int64]RuntimeEvent),
	}

	return service
}

func runtimeConversationModeFromEnvironment() string {
	switch strings.TrimSpace(os.Getenv("AGENT_BUILDER_CONVERSATION_MODE")) {
	case runtimeConversationModeShadow:
		return runtimeConversationModeShadow
	case runtimeConversationModeCanonical:
		return runtimeConversationModeCanonical
	default:
		return runtimeConversationModeLegacy
	}
}
