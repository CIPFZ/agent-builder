package main

func NewRuntimeService() RuntimeService {

	return newRuntimeService()

}

func newRuntimeService() *runtimeService {

	service := &runtimeService{

		requests: make(map[string]runtimeRequestState),

		sessionTurns: make(map[string]string),

		toolEvents: make(map[string]runtimeToolEventState),

		permissions: make(map[string]pendingRuntimePermission),

		eventStream: newRuntimeSSEServer(),
	}

	service.httpAPI = newRuntimeHTTPServer(service)

	return service

}
