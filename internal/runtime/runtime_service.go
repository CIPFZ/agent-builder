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

		toolCalls: scheduler.New(NewRuntimeToolCallStore()),

		permissions: make(map[string]pendingRuntimePermission),

		eventStream: newRuntimeSSEServer(),
	}

	service.httpAPI = newRuntimeHTTPServer(service)

	return service
}
