package main

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	runtime "github.com/CIPFZ/agent-builder/internal/runtime"
	"github.com/wailsapp/wails/v3/pkg/application"
)

type RuntimeStatus = runtime.RuntimeStatus
type RuntimeProject = runtime.RuntimeProject
type RuntimeProjectsResponse = runtime.RuntimeProjectsResponse
type RuntimeSidebarProjectionResponse = runtime.RuntimeSidebarProjectionResponse
type RuntimeOpenProjectRequest = runtime.RuntimeOpenProjectRequest
type RuntimeCreateProjectRequest = runtime.RuntimeCreateProjectRequest
type RuntimeRenameProjectRequest = runtime.RuntimeRenameProjectRequest
type RuntimeProjectActionRequest = runtime.RuntimeProjectActionRequest
type RuntimeOpenProjectResponse = runtime.RuntimeOpenProjectResponse
type RuntimeMemoryRecord = runtime.RuntimeMemoryRecord
type RuntimeMemoryListResponse = runtime.RuntimeMemoryListResponse
type RuntimeMemoryDetailResponse = runtime.RuntimeMemoryDetailResponse
type RuntimeMemoryCreateRequest = runtime.RuntimeMemoryCreateRequest
type RuntimeMemoryUpdateRequest = runtime.RuntimeMemoryUpdateRequest
type RuntimeMemoryDisableRequest = runtime.RuntimeMemoryDisableRequest
type RuntimeMemoryDeleteRequest = runtime.RuntimeMemoryDeleteRequest
type RuntimeMemoryIndexResponse = runtime.RuntimeMemoryIndexResponse
type RuntimeMemoryIssue = runtime.RuntimeMemoryIssue
type RuntimeMemoryDiagnostics = runtime.RuntimeMemoryDiagnostics
type RuntimeModel = runtime.RuntimeModel
type RuntimeModelsResponse = runtime.RuntimeModelsResponse
type RuntimeSelectedModel = runtime.RuntimeSelectedModel
type RuntimeSelectedModelRequest = runtime.RuntimeSelectedModelRequest
type RuntimeSelectedModelResponse = runtime.RuntimeSelectedModelResponse
type RuntimeProviderType = runtime.RuntimeProviderType
type RuntimeProviderCatalogItem = runtime.RuntimeProviderCatalogItem
type RuntimeProviderCatalogResponse = runtime.RuntimeProviderCatalogResponse
type RuntimeConfiguredProvider = runtime.RuntimeConfiguredProvider
type RuntimeConfiguredProvidersResponse = runtime.RuntimeConfiguredProvidersResponse
type RuntimeConfiguredProviderRequest = runtime.RuntimeConfiguredProviderRequest
type RuntimeConfiguredProviderResponse = runtime.RuntimeConfiguredProviderResponse
type RuntimeProviderModelDiscoveryResponse = runtime.RuntimeProviderModelDiscoveryResponse
type RuntimeProviderTestResponse = runtime.RuntimeProviderTestResponse
type RuntimeConfigResponse = runtime.RuntimeConfigResponse
type RuntimeChatRequest = runtime.RuntimeChatRequest
type RuntimeChatResponse = runtime.RuntimeChatResponse
type RuntimeUserInputRequest = runtime.RuntimeUserInputRequest
type RuntimeUserInputItem = runtime.RuntimeUserInputItem
type RuntimeUserInputOptions = runtime.RuntimeUserInputOptions
type RuntimeNormalizedInput = runtime.RuntimeNormalizedInput
type RuntimeMessageDraft = runtime.RuntimeMessageDraft
type RuntimeAttachmentDraft = runtime.RuntimeAttachmentDraft
type RuntimeInputCommand = runtime.RuntimeInputCommand
type RuntimeInputHookOutcome = runtime.RuntimeInputHookOutcome
type RuntimeTerminal = runtime.RuntimeTerminal
type RuntimeTerminalCreateRequest = runtime.RuntimeTerminalCreateRequest
type RuntimeTerminalInputRequest = runtime.RuntimeTerminalInputRequest
type RuntimeTerminalResizeRequest = runtime.RuntimeTerminalResizeRequest
type RuntimeTerminalResponse = runtime.RuntimeTerminalResponse
type RuntimeSessionTerminalsResponse = runtime.RuntimeSessionTerminalsResponse
type RuntimeTerminalEvent = runtime.RuntimeTerminalEvent
type RuntimeTerminalStreamStartRequest = runtime.RuntimeTerminalStreamStartRequest
type RuntimeTerminalStreamAckRequest = runtime.RuntimeTerminalStreamAckRequest
type RuntimeTerminalStreamStopRequest = runtime.RuntimeTerminalStreamStopRequest
type RuntimeTerminalStreamMessage = runtime.RuntimeTerminalStreamMessage
type RuntimeTerminalStreamResponse = runtime.RuntimeTerminalStreamResponse
type RuntimeEventStreamStartRequest = runtime.RuntimeEventStreamStartRequest
type RuntimeEventStreamStopRequest = runtime.RuntimeEventStreamStopRequest
type RuntimeEventStreamResponse = runtime.RuntimeEventStreamResponse
type RuntimeEventStreamMessage = runtime.RuntimeEventStreamMessage
type RuntimeTerminalProfile = runtime.RuntimeTerminalProfile
type RuntimeTerminalSettings = runtime.RuntimeTerminalSettings
type RuntimeTerminalSettingsResponse = runtime.RuntimeTerminalSettingsResponse
type RuntimeTurn = runtime.RuntimeTurn
type RuntimeTurnResponse = runtime.RuntimeTurnResponse
type RuntimeTurnsResponse = runtime.RuntimeTurnsResponse
type RuntimeReactCallchainResponse = runtime.RuntimeReactCallchainResponse
type RuntimeReactCallNode = runtime.RuntimeReactCallNode
type RuntimeReactCallSummary = runtime.RuntimeReactCallSummary
type RuntimeReactCallSource = runtime.RuntimeReactCallSource
type RuntimeToolResultDelivery = runtime.RuntimeToolResultDelivery
type RuntimeRun = runtime.RuntimeRun
type RuntimeRunsResponse = runtime.RuntimeRunsResponse
type RuntimeRunResponse = runtime.RuntimeRunResponse
type RuntimeRunResumeResponse = runtime.RuntimeRunResumeResponse
type RuntimeTodosResponse = runtime.RuntimeTodosResponse
type RuntimeToolCall = runtime.RuntimeToolCall
type RuntimeToolCallResponse = runtime.RuntimeToolCallResponse
type RuntimeToolCallsResponse = runtime.RuntimeToolCallsResponse
type RuntimeHook = runtime.RuntimeHook
type RuntimeHooksResponse = runtime.RuntimeHooksResponse
type RuntimeHookExecution = runtime.RuntimeHookExecution
type RuntimeHookExecutionsRequest = runtime.RuntimeHookExecutionsRequest
type RuntimeHookExecutionsResponse = runtime.RuntimeHookExecutionsResponse
type RuntimeHookExecutionResponse = runtime.RuntimeHookExecutionResponse
type RuntimeObject = runtime.RuntimeObject
type RuntimeObjectListRequest = runtime.RuntimeObjectListRequest
type RuntimeObjectResponse = runtime.RuntimeObjectResponse
type RuntimeObjectsResponse = runtime.RuntimeObjectsResponse
type RuntimeObjectContentResponse = runtime.RuntimeObjectContentResponse
type RuntimeCanonicalMessageContentResponseV2 = runtime.RuntimeCanonicalMessageContentResponseV2
type RuntimeConversationSearchRequestV2 = runtime.RuntimeConversationSearchRequestV2
type RuntimeConversationSearchResponseV2 = runtime.RuntimeConversationSearchResponseV2
type RuntimeCompactBoundary = runtime.RuntimeCompactBoundary
type RuntimeCompactBoundariesResponse = runtime.RuntimeCompactBoundariesResponse
type RuntimeCompactToolCallRef = runtime.RuntimeCompactToolCallRef
type RuntimePromptAssembly = runtime.RuntimePromptAssembly
type RuntimePromptAssembliesResponse = runtime.RuntimePromptAssembliesResponse
type RuntimePromptSectionSummary = runtime.RuntimePromptSectionSummary
type RuntimePromptSystemSummary = runtime.RuntimePromptSystemSummary
type RuntimePromptMessageSummary = runtime.RuntimePromptMessageSummary
type RuntimePromptToolSummary = runtime.RuntimePromptToolSummary
type RuntimePromptSkillSummary = runtime.RuntimePromptSkillSummary
type RuntimePromptMCPSummary = runtime.RuntimePromptMCPSummary
type RuntimeBudgetReport = runtime.RuntimeBudgetReport
type RuntimeBudgetBucket = runtime.RuntimeBudgetBucket
type RuntimeWorktree = runtime.RuntimeWorktree
type RuntimeWorktreesResponse = runtime.RuntimeWorktreesResponse
type RuntimeWorktreeResponse = runtime.RuntimeWorktreeResponse
type RuntimeWorktreeCreateRequest = runtime.RuntimeWorktreeCreateRequest
type RuntimeWorktreeActionRequest = runtime.RuntimeWorktreeActionRequest
type RuntimeEffectiveScope = runtime.RuntimeEffectiveScope
type RuntimeEffectiveScopeResponse = runtime.RuntimeEffectiveScopeResponse
type RuntimeAgentTask = runtime.RuntimeAgentTask
type RuntimeAgentTaskResponse = runtime.RuntimeAgentTaskResponse
type RuntimeAgentTasksResponse = runtime.RuntimeAgentTasksResponse
type RuntimeAgentRoleDefinition = runtime.RuntimeAgentRoleDefinition
type RuntimeAgentRolesResponse = runtime.RuntimeAgentRolesResponse
type RuntimeAgentRoleResponse = runtime.RuntimeAgentRoleResponse
type RuntimeAgentTaskMessage = runtime.RuntimeAgentTaskMessage
type RuntimeAgentTaskMessagesResponse = runtime.RuntimeAgentTaskMessagesResponse
type RuntimeAgentTaskMessageCreateRequest = runtime.RuntimeAgentTaskMessageCreateRequest
type RuntimeAgentTaskMessageResponse = runtime.RuntimeAgentTaskMessageResponse
type RuntimeAgentTaskResult = runtime.RuntimeAgentTaskResult
type RuntimeAgentTaskResultResponse = runtime.RuntimeAgentTaskResultResponse
type RuntimeAgentTaskOutputResponse = runtime.RuntimeAgentTaskOutputResponse
type RuntimeMessage = runtime.RuntimeMessage
type RuntimeMessagePart = runtime.RuntimeMessagePart
type RuntimeSession = runtime.RuntimeSession
type RuntimeSessionsResponse = runtime.RuntimeSessionsResponse
type RuntimeSessionResponse = runtime.RuntimeSessionResponse
type RuntimeSessionCreateRequest = runtime.RuntimeSessionCreateRequest
type RuntimeSessionUpdateRequest = runtime.RuntimeSessionUpdateRequest
type RuntimeMessagesResponse = runtime.RuntimeMessagesResponse
type RuntimeAssistantStep = runtime.RuntimeAssistantStep
type RuntimeToolResult = runtime.RuntimeToolResult
type RuntimeSessionActivityResponse = runtime.RuntimeSessionActivityResponse
type RuntimeActivityWindow = runtime.RuntimeActivityWindow
type RuntimeSessionActivityWindowResponse = runtime.RuntimeSessionActivityWindowResponse
type RuntimeTurnActivityResponse = runtime.RuntimeTurnActivityResponse
type RuntimeRunProjectionRequest = runtime.RuntimeRunProjectionRequest
type RuntimeRunProjectionResponse = runtime.RuntimeRunProjectionResponse
type RuntimeRunTransitionHistoryRequest = runtime.RuntimeRunTransitionHistoryRequest
type RuntimeRunTransitionHistoryResponse = runtime.RuntimeRunTransitionHistoryResponse
type RuntimeRunTransitionHistorySource = runtime.RuntimeRunTransitionHistorySource
type RuntimeRunSchedulerPlanRequest = runtime.RuntimeRunSchedulerPlanRequest
type RuntimeRunSchedulerPlanResponse = runtime.RuntimeRunSchedulerPlanResponse
type RuntimeRunSchedulerExecuteTaskRequest = runtime.RuntimeRunSchedulerExecuteTaskRequest
type RuntimeRunSchedulerExecuteTaskResponse = runtime.RuntimeRunSchedulerExecuteTaskResponse
type RuntimeRunTransition = runtime.RuntimeRunTransition
type RuntimeRunProjection = runtime.RuntimeRunProjection
type RuntimeRunSchedulerPlan = runtime.RuntimeRunSchedulerPlan
type RuntimeRunSchedulerPlanItem = runtime.RuntimeRunSchedulerPlanItem
type RuntimeRunSchedulerTaskScope = runtime.RuntimeRunSchedulerTaskScope
type RuntimeRunSchedulerPlanSource = runtime.RuntimeRunSchedulerPlanSource
type RuntimeRunSchedulerExecuteTaskSource = runtime.RuntimeRunSchedulerExecuteTaskSource
type RuntimeRunCheckpoint = runtime.RuntimeRunCheckpoint
type RuntimeRunCheckpointMarker = runtime.RuntimeRunCheckpointMarker
type RuntimeRunCheckpointMarkersResponse = runtime.RuntimeRunCheckpointMarkersResponse
type RuntimeRunCheckpointMarkerResponse = runtime.RuntimeRunCheckpointMarkerResponse
type RuntimeRunDiagnostics = runtime.RuntimeRunDiagnostics
type RuntimeRunUserActions = runtime.RuntimeRunUserActions
type RuntimeRunUserAction = runtime.RuntimeRunUserAction
type RuntimeRunProjectionSource = runtime.RuntimeRunProjectionSource
type RuntimePermissionRequest = runtime.RuntimePermissionRequest
type RuntimePermissionsResponse = runtime.RuntimePermissionsResponse
type RuntimePermissionDecision = runtime.RuntimePermissionDecision
type RuntimePolicy = runtime.RuntimePolicy
type RuntimePolicyResponse = runtime.RuntimePolicyResponse
type RuntimePolicyUpdateRequest = runtime.RuntimePolicyUpdateRequest
type RuntimeContextGovernanceSettings = runtime.RuntimeContextGovernanceSettings
type RuntimeContextGovernanceSettingsResponse = runtime.RuntimeContextGovernanceSettingsResponse
type RuntimeContextGovernanceModelOverride = runtime.RuntimeContextGovernanceModelOverride
type RuntimeContextGovernanceProviderOverride = runtime.RuntimeContextGovernanceProviderOverride
type RuntimeRequests = runtime.RuntimeRequests
type RuntimeUsage = runtime.RuntimeUsage
type RuntimeContextUsage = runtime.RuntimeContextUsage
type RuntimeContextCategory = runtime.RuntimeContextCategory
type RuntimeEventStats = runtime.RuntimeEventStats
type RuntimeEventsResponse = runtime.RuntimeEventsResponse
type RuntimeReplayExportRequest = runtime.RuntimeReplayExportRequest
type RuntimeReplayExportResponse = runtime.RuntimeReplayExportResponse
type RuntimeReplayExportSummary = runtime.RuntimeReplayExportSummary
type RuntimeRecoveryStatus = runtime.RuntimeRecoveryStatus
type RuntimeRecoveredTurn = runtime.RuntimeRecoveredTurn
type RuntimeRecoverableError = runtime.RuntimeRecoverableError
type RuntimeRecoveryAction = runtime.RuntimeRecoveryAction
type RuntimeResumeInterruptedTurnRequest = runtime.RuntimeResumeInterruptedTurnRequest
type RuntimeRecoveryRetryResponse = runtime.RuntimeRecoveryRetryResponse
type RuntimeSkill = runtime.RuntimeSkill
type RuntimeSkillsResponse = runtime.RuntimeSkillsResponse
type RuntimePlugin = runtime.RuntimePlugin
type RuntimePluginsResponse = runtime.RuntimePluginsResponse
type RuntimeSkillCreateRequest = runtime.RuntimeSkillCreateRequest
type RuntimeSkillPathRequest = runtime.RuntimeSkillPathRequest
type RuntimeSkillToggleRequest = runtime.RuntimeSkillToggleRequest
type RuntimeMCPCounts = runtime.RuntimeMCPCounts
type RuntimeMCPServer = runtime.RuntimeMCPServer
type RuntimeMCPServersResponse = runtime.RuntimeMCPServersResponse
type RuntimeMCPServerConfigRequest = runtime.RuntimeMCPServerConfigRequest
type RuntimeMCPServerToggleRequest = runtime.RuntimeMCPServerToggleRequest
type RuntimeMCPTool = runtime.RuntimeMCPTool
type RuntimeMCPToolsResponse = runtime.RuntimeMCPToolsResponse
type RuntimeMCPToolToggleRequest = runtime.RuntimeMCPToolToggleRequest
type RuntimeMCPResource = runtime.RuntimeMCPResource
type RuntimeMCPResourcesResponse = runtime.RuntimeMCPResourcesResponse
type RuntimeMCPPrompt = runtime.RuntimeMCPPrompt
type RuntimeMCPPromptsResponse = runtime.RuntimeMCPPromptsResponse
type RuntimeMCPRequest = runtime.RuntimeMCPRequest
type RuntimeMCPRequestsResponse = runtime.RuntimeMCPRequestsResponse
type RuntimeMCPRequestResponse = runtime.RuntimeMCPRequestResponse
type RuntimeMCPRequestListRequest = runtime.RuntimeMCPRequestListRequest
type RuntimeMCPRequestDecision = runtime.RuntimeMCPRequestDecision
type RuntimeCapability = runtime.RuntimeCapability
type RuntimeCapabilitiesResponse = runtime.RuntimeCapabilitiesResponse
type RuntimeCapabilityResponse = runtime.RuntimeCapabilityResponse
type RuntimeToolSearchRequest = runtime.RuntimeToolSearchRequest
type RuntimeToolSearchResponse = runtime.RuntimeToolSearchResponse
type RuntimeToolSearchResult = runtime.RuntimeToolSearchResult
type RuntimeToolSearchOmission = runtime.RuntimeToolSearchOmission
type RuntimeToolSchemaBudgetImpact = runtime.RuntimeToolSchemaBudgetImpact
type RuntimeContextSource = runtime.RuntimeContextSource
type RuntimeContextSourcesResponse = runtime.RuntimeContextSourcesResponse
type RuntimeReadFileState = runtime.RuntimeReadFileState
type RuntimeReadFilesResponse = runtime.RuntimeReadFilesResponse
type RuntimeModelConfig = runtime.RuntimeModelConfig
type RuntimeModelVerifyResponse = runtime.RuntimeModelVerifyResponse
type RuntimeModelDiscoveryResponse = runtime.RuntimeModelDiscoveryResponse
type RuntimeAuditEvent = runtime.RuntimeAuditEvent
type RuntimeAuditResponse = runtime.RuntimeAuditResponse
type RuntimeEvent = runtime.RuntimeEvent
type RuntimeCanonicalConversationSnapshotRequest = runtime.RuntimeCanonicalConversationSnapshotRequest
type RuntimeCanonicalConversationSnapshot = runtime.RuntimeCanonicalConversationSnapshot
type RuntimeCanonicalConversationEventsRequestV2 = runtime.RuntimeCanonicalConversationEventsRequestV2
type RuntimeCanonicalConversationEventsResponseV2 = runtime.RuntimeCanonicalConversationEventsResponseV2
type RuntimeCanonicalConversationStreamStartRequestV2 = runtime.RuntimeCanonicalConversationStreamStartRequestV2
type RuntimeCanonicalConversationStreamStopRequestV2 = runtime.RuntimeCanonicalConversationStreamStopRequestV2
type RuntimeCanonicalConversationStreamResponseV2 = runtime.RuntimeCanonicalConversationStreamResponseV2
type RuntimeCanonicalConversationStreamMessageV2 = runtime.RuntimeCanonicalConversationStreamMessageV2

// RuntimeBridge is the Wails adapter. It intentionally delegates to
// runtime.RuntimeService so desktop bindings do not become the business
// boundary.
type RuntimeBridge struct {
	service                       runtime.RuntimeService
	terminalMu                    sync.Mutex
	terminalStreams               map[string]*runtimeBridgeTerminalStream
	conversationV2Mu              sync.Mutex
	conversationV2Streams         map[string]*runtimeBridgeOutputStream
	conversationV2StreamBySession map[string]string
	runtimeEventMu                sync.Mutex
	runtimeEventStreams           map[string]context.CancelFunc
}

type runtimeBridgeTerminalStream struct {
	cancel context.CancelFunc
	ackCh  chan int64
}

type runtimeBridgeOutputStream struct {
	cancel    context.CancelFunc
	sessionID string
}

func NewRuntimeBridge() *RuntimeBridge {

	return &RuntimeBridge{

		service:                       runtime.NewRuntimeService(),
		terminalStreams:               make(map[string]*runtimeBridgeTerminalStream),
		conversationV2Streams:         make(map[string]*runtimeBridgeOutputStream),
		conversationV2StreamBySession: make(map[string]string),
		runtimeEventStreams:           make(map[string]context.CancelFunc),
	}

}

func (r *RuntimeBridge) Status(ctx context.Context) (RuntimeStatus, error) {

	return r.service.Status(ctx)

}

func (r *RuntimeBridge) RecoveryStatus(ctx context.Context) (RuntimeRecoveryStatus, error) {

	return r.service.RecoveryStatus(ctx)

}

func (r *RuntimeBridge) ResumeInterruptedTurn(ctx context.Context, turnID string, req RuntimeResumeInterruptedTurnRequest) (RuntimeTurnResponse, error) {
	return r.service.ResumeInterruptedTurn(ctx, turnID, req)
}

func (r *RuntimeBridge) DiscardInterruptedTurn(ctx context.Context, turnID string) (RuntimeTurnResponse, error) {
	return r.service.DiscardInterruptedTurn(ctx, turnID)
}

func (r *RuntimeBridge) RetryRecoverableError(ctx context.Context, errorID string) (RuntimeRecoveryRetryResponse, error) {
	return r.service.RetryRecoverableError(ctx, errorID)
}

func (r *RuntimeBridge) Projects(ctx context.Context) (RuntimeProjectsResponse, error) {
	return r.service.Projects(ctx)
}

func (r *RuntimeBridge) SidebarProjection(ctx context.Context) (RuntimeSidebarProjectionResponse, error) {
	return r.service.SidebarProjection(ctx)
}

func (r *RuntimeBridge) OpenProject(ctx context.Context, req RuntimeOpenProjectRequest) (RuntimeOpenProjectResponse, error) {

	return r.service.OpenProject(ctx, req)

}

func (r *RuntimeBridge) CreateProject(ctx context.Context, req RuntimeCreateProjectRequest) (RuntimeOpenProjectResponse, error) {

	return r.service.CreateProject(ctx, req)

}

func (r *RuntimeBridge) RenameProject(ctx context.Context, req RuntimeRenameProjectRequest) (RuntimeOpenProjectResponse, error) {

	return r.service.RenameProject(ctx, req)

}

func (r *RuntimeBridge) OpenProjectInExplorer(ctx context.Context, req RuntimeProjectActionRequest) (RuntimeOpenProjectResponse, error) {

	return r.service.OpenProjectInExplorer(ctx, req)

}

func (r *RuntimeBridge) RemoveProject(ctx context.Context, req RuntimeProjectActionRequest) (RuntimeOpenProjectResponse, error) {

	return r.service.RemoveProject(ctx, req)

}

func (r *RuntimeBridge) ProjectMemories(ctx context.Context, projectID string) (RuntimeMemoryListResponse, error) {

	return r.service.ProjectMemories(ctx, projectID)

}

func (r *RuntimeBridge) ProjectMemory(ctx context.Context, memoryID string) (RuntimeMemoryDetailResponse, error) {

	return r.service.ProjectMemory(ctx, memoryID)

}

func (r *RuntimeBridge) CreateProjectMemory(ctx context.Context, req RuntimeMemoryCreateRequest) (RuntimeMemoryRecord, error) {

	return r.service.CreateProjectMemory(ctx, req)

}

func (r *RuntimeBridge) UpdateProjectMemory(ctx context.Context, memoryID string, req RuntimeMemoryUpdateRequest) (RuntimeMemoryRecord, error) {

	return r.service.UpdateProjectMemory(ctx, memoryID, req)

}

func (r *RuntimeBridge) DisableProjectMemory(ctx context.Context, memoryID string, req RuntimeMemoryDisableRequest) (RuntimeMemoryRecord, error) {

	return r.service.DisableProjectMemory(ctx, memoryID, req)

}

func (r *RuntimeBridge) DeleteProjectMemory(ctx context.Context, memoryID string, req RuntimeMemoryDeleteRequest) (RuntimeMemoryRecord, error) {

	return r.service.DeleteProjectMemory(ctx, memoryID, req)

}

func (r *RuntimeBridge) RefreshProjectMemoryIndex(ctx context.Context, projectID string) (RuntimeMemoryIndexResponse, error) {

	return r.service.RefreshProjectMemoryIndex(ctx, projectID)

}

func (r *RuntimeBridge) ProjectMemoryDiagnostics(ctx context.Context, projectID string) (RuntimeMemoryDiagnostics, error) {

	return r.service.ProjectMemoryDiagnostics(ctx, projectID)

}

func (r *RuntimeBridge) SelectProjectDirectory(ctx context.Context) (string, error) {

	app := application.Get()
	if app == nil {
		return "", errors.New("desktop application is not initialized")
	}
	return app.Dialog.OpenFile().
		CanChooseDirectories(true).
		CanChooseFiles(false).
		SetTitle("选择项目文件夹").
		PromptForSingleSelection()

}

func (r *RuntimeBridge) Models(ctx context.Context) (RuntimeModelsResponse, error) {

	return r.service.Models(ctx)

}

func (r *RuntimeBridge) SelectedModel(ctx context.Context) (RuntimeSelectedModelResponse, error) {

	return r.service.SelectedModel(ctx)

}

func (r *RuntimeBridge) SaveSelectedModel(ctx context.Context, req RuntimeSelectedModelRequest) (RuntimeSelectedModelResponse, error) {

	return r.service.SaveSelectedModel(ctx, req)

}

func (r *RuntimeBridge) ProviderCatalog(ctx context.Context) (RuntimeProviderCatalogResponse, error) {

	return r.service.ProviderCatalog(ctx)

}

func (r *RuntimeBridge) ConfiguredProviders(ctx context.Context) (RuntimeConfiguredProvidersResponse, error) {

	return r.service.ConfiguredProviders(ctx)

}

func (r *RuntimeBridge) SaveConfiguredProvider(ctx context.Context, req RuntimeConfiguredProviderRequest) (RuntimeConfiguredProviderResponse, error) {

	return r.service.SaveConfiguredProvider(ctx, req)

}

func (r *RuntimeBridge) DeleteConfiguredProvider(ctx context.Context, providerID string) (RuntimeConfiguredProvidersResponse, error) {

	return r.service.DeleteConfiguredProvider(ctx, providerID)

}

func (r *RuntimeBridge) DiscoverConfiguredProviderModels(ctx context.Context, providerID string) (RuntimeProviderModelDiscoveryResponse, error) {

	return r.service.DiscoverConfiguredProviderModels(ctx, providerID)

}

func (r *RuntimeBridge) TestConfiguredProvider(ctx context.Context, providerID string) (RuntimeProviderTestResponse, error) {

	return r.service.TestConfiguredProvider(ctx, providerID)

}

func (r *RuntimeBridge) MeasureConfiguredProviderLatency(ctx context.Context, providerID string) (RuntimeProviderTestResponse, error) {

	return r.service.MeasureConfiguredProviderLatency(ctx, providerID)

}

func (r *RuntimeBridge) GetModelConfig(ctx context.Context) (RuntimeConfigResponse, error) {

	return r.service.GetModelConfig(ctx)

}

func (r *RuntimeBridge) SaveModelConfig(ctx context.Context, req RuntimeModelConfig) (RuntimeConfigResponse, error) {

	return r.service.SaveModelConfig(ctx, req)

}

func (r *RuntimeBridge) DiscoverModelConfig(ctx context.Context, req RuntimeModelConfig) (RuntimeModelDiscoveryResponse, error) {

	return r.service.DiscoverModelConfig(ctx, req)

}

func (r *RuntimeBridge) VerifyModelConfig(ctx context.Context, req RuntimeModelConfig) (RuntimeModelVerifyResponse, error) {

	return r.service.VerifyModelConfig(ctx, req)

}

func (r *RuntimeBridge) Chat(ctx context.Context, req RuntimeChatRequest) (RuntimeChatResponse, error) {

	return r.service.Chat(ctx, req)

}

func (r *RuntimeBridge) SubmitUserInput(ctx context.Context, req RuntimeUserInputRequest) (RuntimeChatResponse, error) {

	return r.service.SubmitUserInput(ctx, req)

}

func (r *RuntimeBridge) UserInput(ctx context.Context, inputID string) (RuntimeNormalizedInput, error) {

	return r.service.UserInput(ctx, inputID)

}

func (r *RuntimeBridge) CreateTerminal(ctx context.Context, req RuntimeTerminalCreateRequest) (RuntimeTerminalResponse, error) {

	return r.service.CreateTerminal(ctx, req)

}

func (r *RuntimeBridge) TerminalSettings(ctx context.Context) (RuntimeTerminalSettingsResponse, error) {

	return r.service.TerminalSettings(ctx)

}

func (r *RuntimeBridge) SaveTerminalSettings(ctx context.Context, req RuntimeTerminalSettings) (RuntimeTerminalSettingsResponse, error) {

	return r.service.SaveTerminalSettings(ctx, req)

}

func (r *RuntimeBridge) WriteTerminalInput(ctx context.Context, terminalID string, req RuntimeTerminalInputRequest) (RuntimeTerminalResponse, error) {

	return r.service.WriteTerminalInput(ctx, terminalID, req)

}

func (r *RuntimeBridge) ResizeTerminal(ctx context.Context, terminalID string, req RuntimeTerminalResizeRequest) (RuntimeTerminalResponse, error) {

	return r.service.ResizeTerminal(ctx, terminalID, req)

}

func (r *RuntimeBridge) StartTerminalEventStream(ctx context.Context, req RuntimeTerminalStreamStartRequest) (RuntimeTerminalStreamResponse, error) {

	app := application.Get()
	if app == nil {
		return RuntimeTerminalStreamResponse{}, errors.New("desktop application is not initialized")
	}
	terminalID := strings.TrimSpace(req.TerminalID)
	if terminalID == "" {
		return RuntimeTerminalStreamResponse{}, errors.New("terminal id is required")
	}
	streamID := strings.TrimSpace(req.StreamID)
	if streamID == "" {
		streamID = fmt.Sprintf("terminal-stream-%d", time.Now().UnixNano())
	}
	eventName := "agent-builder:terminal-stream"
	streamCtx, cancel := context.WithCancel(context.Background())
	events, unsubscribe := r.service.SubscribeTerminalEvents(streamCtx, terminalID, req.After)
	stream := &runtimeBridgeTerminalStream{
		cancel: func() {
			cancel()
			unsubscribe()
		},
		ackCh: make(chan int64, 16),
	}

	r.terminalMu.Lock()
	if r.terminalStreams == nil {
		r.terminalStreams = make(map[string]*runtimeBridgeTerminalStream)
	}
	r.terminalStreams[streamID] = stream
	r.terminalMu.Unlock()

	go r.runTerminalEventStream(streamCtx, app, eventName, streamID, terminalID, events, stream)

	return RuntimeTerminalStreamResponse{StreamID: streamID, EventName: eventName}, nil

}

func (r *RuntimeBridge) AckTerminalEventStream(ctx context.Context, req RuntimeTerminalStreamAckRequest) (bool, error) {

	r.terminalMu.Lock()
	stream := r.terminalStreams[strings.TrimSpace(req.StreamID)]
	r.terminalMu.Unlock()
	if stream == nil {
		return false, nil
	}
	select {
	case stream.ackCh <- req.Sequence:
	default:
	}
	return true, nil

}

func (r *RuntimeBridge) StopTerminalEventStream(ctx context.Context, req RuntimeTerminalStreamStopRequest) (bool, error) {

	return r.stopTerminalEventStream(strings.TrimSpace(req.StreamID)), nil

}

func (r *RuntimeBridge) SessionTerminals(ctx context.Context, sessionID string) (RuntimeSessionTerminalsResponse, error) {

	return r.service.SessionTerminals(ctx, sessionID)

}

func (r *RuntimeBridge) DeleteTerminal(ctx context.Context, terminalID string) (RuntimeTerminalResponse, error) {

	return r.service.DeleteTerminal(ctx, terminalID)

}

const (
	runtimeBridgeTerminalStreamBatchBytes = 64 * 1024
	runtimeBridgeTerminalStreamBatchWait  = 8 * time.Millisecond
	runtimeBridgeTerminalStreamAckTimeout = 5 * time.Second
)

func (r *RuntimeBridge) runTerminalEventStream(
	ctx context.Context,
	app *application.App,
	eventName string,
	streamID string,
	terminalID string,
	events <-chan RuntimeTerminalEvent,
	stream *runtimeBridgeTerminalStream,
) {
	defer r.stopTerminalEventStream(streamID)
	for {
		batch, ok := nextRuntimeBridgeTerminalStreamBatch(ctx, events)
		if !ok {
			return
		}
		messageType := "output"
		if batch[len(batch)-1].Final {
			messageType = "final"
		}
		app.Event.Emit(eventName, RuntimeTerminalStreamMessage{
			Type:       messageType,
			StreamID:   streamID,
			TerminalID: terminalID,
			Events:     batch,
		})
		if !waitRuntimeBridgeTerminalStreamAck(ctx, stream.ackCh, batch[len(batch)-1].Sequence) {
			return
		}
	}
}

func (r *RuntimeBridge) stopTerminalEventStream(streamID string) bool {
	r.terminalMu.Lock()
	stream := r.terminalStreams[streamID]
	if stream != nil {
		delete(r.terminalStreams, streamID)
	}
	r.terminalMu.Unlock()
	if stream == nil {
		return false
	}
	stream.cancel()
	return true
}

func nextRuntimeBridgeTerminalStreamBatch(ctx context.Context, events <-chan RuntimeTerminalEvent) ([]RuntimeTerminalEvent, bool) {
	var batch []RuntimeTerminalEvent
	batchBytes := 0
	timer := time.NewTimer(runtimeBridgeTerminalStreamBatchWait)
	defer timer.Stop()

	for {
		select {
		case event, ok := <-events:
			if !ok {
				return batch, len(batch) > 0
			}
			batch = append(batch, event)
			batchBytes += runtimeBridgeTerminalEventSize(event)
			if event.Final || batchBytes >= runtimeBridgeTerminalStreamBatchBytes {
				return batch, true
			}
		case <-timer.C:
			if len(batch) > 0 {
				return batch, true
			}
			timer.Reset(runtimeBridgeTerminalStreamBatchWait)
		case <-ctx.Done():
			return nil, false
		}
	}
}

func waitRuntimeBridgeTerminalStreamAck(ctx context.Context, ackCh <-chan int64, sequence int64) bool {
	if sequence <= 0 {
		return true
	}
	timer := time.NewTimer(runtimeBridgeTerminalStreamAckTimeout)
	defer timer.Stop()
	for {
		select {
		case ack := <-ackCh:
			if ack >= sequence {
				return true
			}
		case <-timer.C:
			return false
		case <-ctx.Done():
			return false
		}
	}
}

func runtimeBridgeTerminalEventSize(event RuntimeTerminalEvent) int {
	return len(event.Data) + len(event.BinaryB64) + len(event.Error)
}

func (r *RuntimeBridge) Turn(ctx context.Context, turnID string) (RuntimeTurnResponse, error) {

	return r.service.Turn(ctx, turnID)

}

func (r *RuntimeBridge) Turns(ctx context.Context, status string) (RuntimeTurnsResponse, error) {

	return r.service.Turns(ctx, status)

}

func (r *RuntimeBridge) ReactCallchain(ctx context.Context, turnID string) (RuntimeReactCallchainResponse, error) {

	return r.service.ReactCallchain(ctx, turnID)

}

func (r *RuntimeBridge) SessionReactCallchain(ctx context.Context, sessionID string, limit int) (RuntimeReactCallchainResponse, error) {

	return r.service.SessionReactCallchain(ctx, sessionID, limit)

}

func (r *RuntimeBridge) Runs(ctx context.Context) (RuntimeRunsResponse, error) {

	return r.service.Runs(ctx)

}

func (r *RuntimeBridge) Run(ctx context.Context, runID string) (RuntimeRunResponse, error) {

	return r.service.Run(ctx, runID)

}

func (r *RuntimeBridge) RunCheckpointMarkers(ctx context.Context, runID string) (RuntimeRunCheckpointMarkersResponse, error) {

	return r.service.RunCheckpointMarkers(ctx, runID)

}

func (r *RuntimeBridge) RunCheckpointMarker(ctx context.Context, runID string, checkpointID string) (RuntimeRunCheckpointMarkerResponse, error) {

	return r.service.RunCheckpointMarker(ctx, runID, checkpointID)

}

func (r *RuntimeBridge) AcknowledgeRunCheckpoint(ctx context.Context, runID string, checkpointID string) (RuntimeRunResponse, error) {

	return r.service.AcknowledgeRunCheckpoint(ctx, runID, checkpointID)

}

func (r *RuntimeBridge) DiscardRunCheckpoint(ctx context.Context, runID string, checkpointID string) (RuntimeRunResponse, error) {

	return r.service.DiscardRunCheckpoint(ctx, runID, checkpointID)

}

func (r *RuntimeBridge) ResumeRunCheckpoint(ctx context.Context, runID string, checkpointID string) (RuntimeRunResumeResponse, error) {

	return r.service.ResumeRunCheckpoint(ctx, runID, checkpointID)

}

func (r *RuntimeBridge) ToolCall(ctx context.Context, toolCallID string) (RuntimeToolCallResponse, error) {

	return r.service.ToolCall(ctx, toolCallID)

}

func (r *RuntimeBridge) TurnToolCalls(ctx context.Context, turnID string) (RuntimeToolCallsResponse, error) {

	return r.service.TurnToolCalls(ctx, turnID)

}

func (r *RuntimeBridge) Hooks(ctx context.Context) (RuntimeHooksResponse, error) {

	return r.service.Hooks(ctx)

}

func (r *RuntimeBridge) HookExecutions(ctx context.Context, req RuntimeHookExecutionsRequest) (RuntimeHookExecutionsResponse, error) {

	return r.service.HookExecutions(ctx, req)

}

func (r *RuntimeBridge) HookExecution(ctx context.Context, executionID string) (RuntimeHookExecutionResponse, error) {

	return r.service.HookExecution(ctx, executionID)

}

func (r *RuntimeBridge) Objects(ctx context.Context, req RuntimeObjectListRequest) (RuntimeObjectsResponse, error) {

	return r.service.Objects(ctx, req)

}

func (r *RuntimeBridge) Object(ctx context.Context, refID string) (RuntimeObjectResponse, error) {

	return r.service.Object(ctx, refID)

}

func (r *RuntimeBridge) ReadObjectContent(ctx context.Context, refID string) (RuntimeObjectContentResponse, error) {

	return r.service.ReadObjectContent(ctx, refID)

}

func (r *RuntimeBridge) TurnCompactBoundaries(ctx context.Context, turnID string) (RuntimeCompactBoundariesResponse, error) {

	return r.service.TurnCompactBoundaries(ctx, turnID)

}

func (r *RuntimeBridge) SessionCompactBoundaries(ctx context.Context, sessionID string) (RuntimeCompactBoundariesResponse, error) {

	return r.service.SessionCompactBoundaries(ctx, sessionID)

}

func (r *RuntimeBridge) TurnPromptAssemblies(ctx context.Context, turnID string) (RuntimePromptAssembliesResponse, error) {

	return r.service.PromptAssembliesByTurn(ctx, turnID)

}

func (r *RuntimeBridge) SessionPromptAssemblies(ctx context.Context, sessionID string, limit int) (RuntimePromptAssembliesResponse, error) {

	return r.service.PromptAssembliesBySession(ctx, sessionID, limit)

}

func (r *RuntimeBridge) Worktrees(ctx context.Context) (RuntimeWorktreesResponse, error) {

	return r.service.Worktrees(ctx)

}

func (r *RuntimeBridge) Worktree(ctx context.Context, worktreeID string) (RuntimeWorktreeResponse, error) {

	return r.service.Worktree(ctx, worktreeID)

}

func (r *RuntimeBridge) CreateWorktree(ctx context.Context, req RuntimeWorktreeCreateRequest) (RuntimeWorktreeResponse, error) {

	return r.service.CreateWorktree(ctx, req)

}

func (r *RuntimeBridge) EnterWorktree(ctx context.Context, worktreeID string, req RuntimeWorktreeActionRequest) (RuntimeWorktreeResponse, error) {

	return r.service.EnterWorktree(ctx, worktreeID, req)

}

func (r *RuntimeBridge) ExitWorktree(ctx context.Context, worktreeID string, req RuntimeWorktreeActionRequest) (RuntimeWorktreeResponse, error) {

	return r.service.ExitWorktree(ctx, worktreeID, req)

}

func (r *RuntimeBridge) CleanupWorktree(ctx context.Context, worktreeID string, req RuntimeWorktreeActionRequest) (RuntimeWorktreeResponse, error) {

	return r.service.CleanupWorktree(ctx, worktreeID, req)

}

func (r *RuntimeBridge) AgentTask(ctx context.Context, taskID string) (RuntimeAgentTaskResponse, error) {

	return r.service.AgentTask(ctx, taskID)

}

func (r *RuntimeBridge) SessionAgentTasks(ctx context.Context, sessionID string) (RuntimeAgentTasksResponse, error) {

	return r.service.SessionAgentTasks(ctx, sessionID)

}

func (r *RuntimeBridge) TaskEffectiveScope(ctx context.Context, taskID string) (RuntimeEffectiveScopeResponse, error) {

	return r.service.TaskEffectiveScope(ctx, taskID)

}

func (r *RuntimeBridge) TurnAgentTasks(ctx context.Context, turnID string) (RuntimeAgentTasksResponse, error) {

	return r.service.TurnAgentTasks(ctx, turnID)

}

func (r *RuntimeBridge) SessionTodos(ctx context.Context, sessionID string) (RuntimeTodosResponse, error) {
	return r.service.SessionTodos(ctx, sessionID)
}

func (r *RuntimeBridge) TurnTodos(ctx context.Context, turnID string) (RuntimeTodosResponse, error) {
	return r.service.TurnTodos(ctx, turnID)
}

func (r *RuntimeBridge) CancelAgentTask(ctx context.Context, taskID string) (RuntimeAgentTaskResponse, error) {

	return r.service.CancelAgentTask(ctx, taskID)

}

func (r *RuntimeBridge) AgentRoles(ctx context.Context) (RuntimeAgentRolesResponse, error) {
	return r.service.AgentRoles(ctx)
}

func (r *RuntimeBridge) AgentRole(ctx context.Context, roleID string) (RuntimeAgentRoleResponse, error) {
	return r.service.AgentRole(ctx, roleID)
}

func (r *RuntimeBridge) AgentTaskMessages(ctx context.Context, taskID string) (RuntimeAgentTaskMessagesResponse, error) {
	return r.service.AgentTaskMessages(ctx, taskID)
}

func (r *RuntimeBridge) CreateAgentTaskMessage(ctx context.Context, taskID string, req RuntimeAgentTaskMessageCreateRequest) (RuntimeAgentTaskMessageResponse, error) {
	return r.service.CreateAgentTaskMessage(ctx, taskID, req)
}

func (r *RuntimeBridge) AgentTaskFollowUp(ctx context.Context, taskID string, req RuntimeAgentTaskMessageCreateRequest) (RuntimeAgentTaskMessageResponse, error) {
	return r.service.SendAgentTaskFollowUp(ctx, taskID, req)
}

func (r *RuntimeBridge) AgentTaskResult(ctx context.Context, taskID string) (RuntimeAgentTaskResultResponse, error) {
	return r.service.AgentTaskResult(ctx, taskID)
}

func (r *RuntimeBridge) AgentTaskOutput(ctx context.Context, taskID string) (RuntimeAgentTaskOutputResponse, error) {
	return r.service.AgentTaskOutput(ctx, taskID)
}

func (r *RuntimeBridge) Sessions(ctx context.Context) (RuntimeSessionsResponse, error) {

	return r.service.Sessions(ctx)

}

func (r *RuntimeBridge) Session(ctx context.Context, sessionID string) (RuntimeSessionResponse, error) {

	return r.service.Session(ctx, sessionID)

}

func (r *RuntimeBridge) CreateSession(ctx context.Context, req RuntimeSessionCreateRequest) (RuntimeSessionResponse, error) {

	return r.service.CreateSession(ctx, req)

}

func (r *RuntimeBridge) SelectSession(ctx context.Context, sessionID string) (RuntimeStatus, error) {

	return r.service.SelectSession(ctx, sessionID)

}

func (r *RuntimeBridge) RenameSession(ctx context.Context, req RuntimeSessionUpdateRequest) (RuntimeSessionsResponse, error) {

	return r.service.RenameSession(ctx, req)

}

func (r *RuntimeBridge) DeleteSession(ctx context.Context, sessionID string) (RuntimeSessionsResponse, error) {

	return r.service.DeleteSession(ctx, sessionID)

}

func (r *RuntimeBridge) SessionMessages(ctx context.Context, sessionID string) (RuntimeMessagesResponse, error) {

	return r.service.SessionMessages(ctx, sessionID)

}

func (r *RuntimeBridge) SessionContextUsage(ctx context.Context, sessionID string) (RuntimeContextUsage, error) {

	return r.service.SessionContextUsage(ctx, sessionID)

}

func (r *RuntimeBridge) SessionConversationSnapshotV2(ctx context.Context, sessionID string, req RuntimeCanonicalConversationSnapshotRequest) (RuntimeCanonicalConversationSnapshot, error) {
	return r.service.SessionConversationSnapshotV2(ctx, sessionID, req)
}

func (r *RuntimeBridge) SessionConversationMessageContentV2(ctx context.Context, sessionID, messageID string) (RuntimeCanonicalMessageContentResponseV2, error) {
	return r.service.SessionConversationMessageContentV2(ctx, sessionID, messageID)
}

func (r *RuntimeBridge) SearchSessionConversationV2(ctx context.Context, sessionID string, req RuntimeConversationSearchRequestV2) (RuntimeConversationSearchResponseV2, error) {
	return r.service.SearchSessionConversationV2(ctx, sessionID, req)
}

func (r *RuntimeBridge) SessionConversationEventsV2(ctx context.Context, sessionID string, req RuntimeCanonicalConversationEventsRequestV2) (RuntimeCanonicalConversationEventsResponseV2, error) {
	return r.service.SessionConversationEventsV2(ctx, sessionID, req)
}
func (r *RuntimeBridge) StartSessionConversationStreamV2(ctx context.Context, req RuntimeCanonicalConversationStreamStartRequestV2) (RuntimeCanonicalConversationStreamResponseV2, error) {
	app := application.Get()
	if app == nil {
		return RuntimeCanonicalConversationStreamResponseV2{}, errors.New("desktop application is not initialized")
	}
	sessionID := strings.TrimSpace(req.SessionID)
	if sessionID == "" {
		return RuntimeCanonicalConversationStreamResponseV2{}, errors.New("session id is required")
	}
	streamID := strings.TrimSpace(req.StreamID)
	if streamID == "" {
		streamID = fmt.Sprintf("conversation-v2-stream-%d", time.Now().UnixNano())
	}
	streamCtx, cancel := context.WithCancel(context.Background())
	batches, unsubscribe := r.service.SubscribeSessionConversationEventsV2(streamCtx, sessionID, req.After)
	r.installSessionConversationStreamV2(streamID, &runtimeBridgeOutputStream{sessionID: sessionID, cancel: func() { cancel(); unsubscribe() }})
	eventName := "agent-builder:conversation-v2-stream"
	go func() {
		defer r.stopSessionConversationStreamV2(streamID)
		for {
			select {
			case batch, ok := <-batches:
				if !ok {
					app.Event.Emit(eventName, RuntimeCanonicalConversationStreamMessageV2{StreamID: streamID, Lifecycle: "stream_closed"})
					return
				}
				app.Event.Emit(eventName, RuntimeCanonicalConversationStreamMessageV2{StreamID: streamID, RuntimeCanonicalConversationEventBatchV2: batch})
			case <-streamCtx.Done():
				return
			}
		}
	}()
	return RuntimeCanonicalConversationStreamResponseV2{StreamID: streamID, EventName: eventName}, nil
}

func (r *RuntimeBridge) installSessionConversationStreamV2(streamID string, stream *runtimeBridgeOutputStream) {
	r.conversationV2Mu.Lock()
	if r.conversationV2Streams == nil {
		r.conversationV2Streams = map[string]*runtimeBridgeOutputStream{}
	}
	if r.conversationV2StreamBySession == nil {
		r.conversationV2StreamBySession = map[string]string{}
	}
	existingID := r.conversationV2StreamBySession[stream.sessionID]
	existing := r.conversationV2Streams[existingID]
	if existingID != "" {
		delete(r.conversationV2Streams, existingID)
	}
	r.conversationV2Streams[streamID] = stream
	r.conversationV2StreamBySession[stream.sessionID] = streamID
	r.conversationV2Mu.Unlock()
	if existing != nil {
		existing.cancel()
	}
}

func (r *RuntimeBridge) StopSessionConversationStreamV2(ctx context.Context, req RuntimeCanonicalConversationStreamStopRequestV2) (bool, error) {
	return r.stopSessionConversationStreamV2(strings.TrimSpace(req.StreamID)), nil
}
func (r *RuntimeBridge) stopSessionConversationStreamV2(streamID string) bool {
	if streamID == "" {
		return false
	}
	r.conversationV2Mu.Lock()
	stream := r.conversationV2Streams[streamID]
	if stream != nil {
		delete(r.conversationV2Streams, streamID)
		if r.conversationV2StreamBySession[stream.sessionID] == streamID {
			delete(r.conversationV2StreamBySession, stream.sessionID)
		}
	}
	r.conversationV2Mu.Unlock()
	if stream == nil {
		return false
	}
	stream.cancel()
	return true
}

func (r *RuntimeBridge) SessionActivity(ctx context.Context, sessionID string) (RuntimeSessionActivityResponse, error) {

	return r.service.SessionActivity(ctx, sessionID)

}

func (r *RuntimeBridge) SessionActivityWindow(ctx context.Context, sessionID string, limit int) (RuntimeSessionActivityWindowResponse, error) {

	return r.service.SessionActivityWindow(ctx, sessionID, limit)

}

func (r *RuntimeBridge) SessionActivityCursorWindow(ctx context.Context, sessionID string, cursor string, limit int) (RuntimeSessionActivityWindowResponse, error) {

	return r.service.SessionActivityCursorWindow(ctx, sessionID, cursor, limit)

}

func (r *RuntimeBridge) TurnActivity(ctx context.Context, turnID string) (RuntimeTurnActivityResponse, error) {

	return r.service.TurnActivity(ctx, turnID)

}

func (r *RuntimeBridge) RunProjection(ctx context.Context, req RuntimeRunProjectionRequest) (RuntimeRunProjectionResponse, error) {

	return r.service.RunProjection(ctx, req)

}

func (r *RuntimeBridge) RunTransitionHistory(ctx context.Context, req RuntimeRunTransitionHistoryRequest) (RuntimeRunTransitionHistoryResponse, error) {

	return r.service.RunTransitionHistory(ctx, req)

}

func (r *RuntimeBridge) RunSchedulerPlan(ctx context.Context, req RuntimeRunSchedulerPlanRequest) (RuntimeRunSchedulerPlanResponse, error) {

	return r.service.RunSchedulerPlan(ctx, req)

}

func (r *RuntimeBridge) ExecuteRunTask(ctx context.Context, runID string, taskID string) (RuntimeRunSchedulerExecuteTaskResponse, error) {

	return r.service.ExecuteRunTask(ctx, runID, taskID)

}

func (r *RuntimeBridge) Messages(ctx context.Context) (RuntimeMessagesResponse, error) {

	return r.service.Messages(ctx)

}

func (r *RuntimeBridge) Permissions(ctx context.Context) (RuntimePermissionsResponse, error) {

	return r.service.Permissions(ctx)

}

func (r *RuntimeBridge) GetPolicy(ctx context.Context) (RuntimePolicyResponse, error) {

	return r.service.GetPolicy(ctx)

}

func (r *RuntimeBridge) UpdatePolicy(ctx context.Context, req RuntimePolicyUpdateRequest) (RuntimePolicyResponse, error) {

	return r.service.UpdatePolicy(ctx, req)

}

func (r *RuntimeBridge) ContextGovernanceSettings(ctx context.Context) (RuntimeContextGovernanceSettingsResponse, error) {

	return r.service.ContextGovernanceSettings(ctx)

}

func (r *RuntimeBridge) SaveContextGovernanceSettings(ctx context.Context, req RuntimeContextGovernanceSettings) (RuntimeContextGovernanceSettingsResponse, error) {

	return r.service.SaveContextGovernanceSettings(ctx, req)

}

func (r *RuntimeBridge) StartRuntimeEventStream(ctx context.Context, req RuntimeEventStreamStartRequest) (RuntimeEventStreamResponse, error) {
	app := application.Get()
	if app == nil {
		return RuntimeEventStreamResponse{}, errors.New("desktop application is not initialized")
	}
	streamID := strings.TrimSpace(req.StreamID)
	if streamID == "" {
		streamID = fmt.Sprintf("runtime-event-stream-%d", time.Now().UnixNano())
	}
	streamCtx, cancel := context.WithCancel(context.Background())
	events, unsubscribe := r.service.SubscribeEvents(streamCtx, req.After)
	r.runtimeEventMu.Lock()
	if previous := r.runtimeEventStreams[streamID]; previous != nil {
		previous()
	}
	r.runtimeEventStreams[streamID] = func() { cancel(); unsubscribe() }
	r.runtimeEventMu.Unlock()
	eventName := "agent-builder:runtime-events"
	go func() {
		defer r.stopRuntimeEventStream(streamID)
		for {
			select {
			case <-streamCtx.Done():
				return
			case event, ok := <-events:
				if !ok {
					return
				}
				app.Event.Emit(eventName, RuntimeEventStreamMessage{StreamID: streamID, Events: []RuntimeEvent{event}})
			}
		}
	}()
	return RuntimeEventStreamResponse{StreamID: streamID, EventName: eventName}, nil
}

func (r *RuntimeBridge) StopRuntimeEventStream(ctx context.Context, req RuntimeEventStreamStopRequest) (bool, error) {
	return r.stopRuntimeEventStream(strings.TrimSpace(req.StreamID)), nil
}

func (r *RuntimeBridge) stopRuntimeEventStream(streamID string) bool {
	r.runtimeEventMu.Lock()
	cancel := r.runtimeEventStreams[streamID]
	delete(r.runtimeEventStreams, streamID)
	r.runtimeEventMu.Unlock()
	if cancel == nil {
		return false
	}
	cancel()
	return true
}

func (r *RuntimeBridge) AuditTurn(ctx context.Context, turnID string) (RuntimeAuditResponse, error) {

	return r.service.AuditTurn(ctx, turnID)

}

func (r *RuntimeBridge) AuditSession(ctx context.Context, sessionID string) (RuntimeAuditResponse, error) {

	return r.service.AuditSession(ctx, sessionID)

}

func (r *RuntimeBridge) ReplayExport(ctx context.Context, req RuntimeReplayExportRequest) (RuntimeReplayExportResponse, error) {

	return r.service.ReplayExport(ctx, req)

}

func (r *RuntimeBridge) Skills(ctx context.Context) (RuntimeSkillsResponse, error) {

	return r.service.Skills(ctx)

}

func (r *RuntimeBridge) Plugins(ctx context.Context) (RuntimePluginsResponse, error) {

	return r.service.Plugins(ctx)

}

func (r *RuntimeBridge) RefreshSkills(ctx context.Context) (RuntimeSkillsResponse, error) {

	return r.service.RefreshSkills(ctx)

}

func (r *RuntimeBridge) CreateSkill(ctx context.Context, req RuntimeSkillCreateRequest) (RuntimeSkillsResponse, error) {

	return r.service.CreateSkill(ctx, req)

}

func (r *RuntimeBridge) AddSkillPath(ctx context.Context, req RuntimeSkillPathRequest) (RuntimeSkillsResponse, error) {

	return r.service.AddSkillPath(ctx, req)

}

func (r *RuntimeBridge) SetSkillEnabled(ctx context.Context, req RuntimeSkillToggleRequest) (RuntimeSkillsResponse, error) {

	return r.service.SetSkillEnabled(ctx, req)

}

func (r *RuntimeBridge) MCPServers(ctx context.Context) (RuntimeMCPServersResponse, error) {

	return r.service.MCPServers(ctx)

}

func (r *RuntimeBridge) SaveMCPServer(ctx context.Context, req RuntimeMCPServerConfigRequest) (RuntimeMCPServersResponse, error) {

	return r.service.SaveMCPServer(ctx, req)

}

func (r *RuntimeBridge) SetMCPServerEnabled(ctx context.Context, req RuntimeMCPServerToggleRequest) (RuntimeMCPServersResponse, error) {

	return r.service.SetMCPServerEnabled(ctx, req)

}

func (r *RuntimeBridge) RefreshMCPServer(ctx context.Context, name string) (RuntimeMCPServersResponse, error) {

	return r.service.RefreshMCPServer(ctx, name)

}

func (r *RuntimeBridge) SetMCPToolEnabled(ctx context.Context, req RuntimeMCPToolToggleRequest) (RuntimeMCPToolsResponse, error) {

	return r.service.SetMCPToolEnabled(ctx, req)

}

func (r *RuntimeBridge) MCPTools(ctx context.Context, name string) (RuntimeMCPToolsResponse, error) {

	return r.service.MCPTools(ctx, name)

}

func (r *RuntimeBridge) MCPResources(ctx context.Context, name string) (RuntimeMCPResourcesResponse, error) {

	return r.service.MCPResources(ctx, name)

}

func (r *RuntimeBridge) MCPPrompts(ctx context.Context, name string) (RuntimeMCPPromptsResponse, error) {

	return r.service.MCPPrompts(ctx, name)

}

func (r *RuntimeBridge) MCPRequests(ctx context.Context, req RuntimeMCPRequestListRequest) (RuntimeMCPRequestsResponse, error) {
	return r.service.MCPRequests(ctx, req)
}

func (r *RuntimeBridge) MCPRequest(ctx context.Context, requestID string) (RuntimeMCPRequestResponse, error) {
	return r.service.MCPRequest(ctx, requestID)
}

func (r *RuntimeBridge) DecideMCPRequest(ctx context.Context, req RuntimeMCPRequestDecision) (RuntimeMCPRequestResponse, error) {
	return r.service.DecideMCPRequest(ctx, req)
}

func (r *RuntimeBridge) RetryMCPServer(ctx context.Context, name string) (RuntimeMCPServersResponse, error) {
	return r.service.RetryMCPServer(ctx, name)
}

func (r *RuntimeBridge) Capabilities(ctx context.Context) (RuntimeCapabilitiesResponse, error) {

	return r.service.Capabilities(ctx)

}

func (r *RuntimeBridge) RefreshCapability(ctx context.Context, capabilityID string) (RuntimeCapabilityResponse, error) {

	return r.service.RefreshCapability(ctx, capabilityID)

}

func (r *RuntimeBridge) SearchTools(ctx context.Context, req RuntimeToolSearchRequest) (RuntimeToolSearchResponse, error) {

	return r.service.SearchTools(ctx, req)

}

func (r *RuntimeBridge) ContextSources(ctx context.Context) (RuntimeContextSourcesResponse, error) {

	return r.service.ContextSources(ctx)

}

func (r *RuntimeBridge) ReadFiles(ctx context.Context, sessionID string) (RuntimeReadFilesResponse, error) {

	return r.service.ReadFiles(ctx, sessionID)

}

func (r *RuntimeBridge) DecidePermission(ctx context.Context, req RuntimePermissionDecision) (RuntimeStatus, error) {

	return r.service.DecidePermission(ctx, req)

}

func (r *RuntimeBridge) Cancel(ctx context.Context) (RuntimeStatus, error) {

	return r.service.Cancel(ctx)

}

func (r *RuntimeBridge) CancelTurn(ctx context.Context, turnID string) (RuntimeStatus, error) {

	return r.service.CancelTurn(ctx, turnID)

}

func (r *RuntimeBridge) MarkInterruptedDone(ctx context.Context, turnID string) (RuntimeTurnResponse, error) {

	return r.service.MarkInterruptedDone(ctx, turnID)

}

func (r *RuntimeBridge) NewChat(ctx context.Context, title string) (RuntimeStatus, error) {
	return r.service.NewChat(ctx, title)
}
