package runtime

type RuntimeStatus struct {
	Ready       bool                        `json:"ready"`
	WorkspaceID string                      `json:"workspaceId"`
	SessionID   string                      `json:"sessionId"`
	WorkingDir  string                      `json:"workingDir"`
	Model       string                      `json:"model"`
	Provider    string                      `json:"provider"`
	Busy        bool                        `json:"busy"`
	Usage       RuntimeUsage                `json:"usage"`
	Events      RuntimeEventStats           `json:"events"`
	Requests    RuntimeRequests             `json:"requests"`
	Action      *RuntimeWriteActionMetadata `json:"action,omitempty"`
}

type RuntimeProject struct {
	ID              string `json:"id"`
	Name            string `json:"name"`
	Path            string `json:"path"`
	IsGitRepository bool   `json:"isGitRepository"`
	Branch          string `json:"branch,omitempty"`
	Current         bool   `json:"current"`
}

type RuntimeOpenProjectRequest struct {
	Path          string `json:"path"`
	CreateMissing bool   `json:"createMissing,omitempty"`
}

type RuntimeCreateProjectRequest struct {
	Name string `json:"name"`
}

type RuntimeRenameProjectRequest struct {
	ProjectID string `json:"projectId,omitempty"`
	Name      string `json:"name"`
}

type RuntimeProjectActionRequest struct {
	ProjectID string `json:"projectId,omitempty"`
}

type RuntimeOpenProjectResponse struct {
	Project RuntimeProject `json:"project"`
	Status  RuntimeStatus  `json:"status"`
}

type RuntimeModel struct {
	ID                   string `json:"id"`
	Name                 string `json:"name"`
	Provider             string `json:"provider"`
	ProviderID           string `json:"providerId,omitempty"`
	ConfiguredProviderID string `json:"configuredProviderId,omitempty"`
	ConfiguredProvider   string `json:"configuredProvider,omitempty"`
	Selected             bool   `json:"selected"`
}

type RuntimeModelsResponse struct {
	Models []RuntimeModel `json:"models"`
}

type RuntimeSelectedModel struct {
	ID                   string `json:"id"`
	ConfiguredProviderID string `json:"configuredProviderId"`
	ProviderID           string `json:"providerId"`
	Model                string `json:"model"`
	Scope                string `json:"scope"`
	ProjectID            string `json:"projectId,omitempty"`
	SessionID            string `json:"sessionId,omitempty"`
	CreatedAt            int64  `json:"createdAt"`
	UpdatedAt            int64  `json:"updatedAt"`
}

type RuntimeSelectedModelRequest struct {
	ConfiguredProviderID string `json:"configuredProviderId"`
	Model                string `json:"model"`
	Scope                string `json:"scope,omitempty"`
	ProjectID            string `json:"projectId,omitempty"`
	SessionID            string `json:"sessionId,omitempty"`
}

type RuntimeSelectedModelResponse struct {
	SelectedModel RuntimeSelectedModel `json:"selectedModel"`
	Status        RuntimeStatus        `json:"status,omitempty"`
}

type RuntimeProviderType struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
}

type RuntimeProviderCatalogItem struct {
	ID                string   `json:"id"`
	Name              string   `json:"name"`
	Type              string   `json:"type"`
	APIEndpoint       string   `json:"apiEndpoint,omitempty"`
	APIKeyTemplate    string   `json:"apiKeyTemplate,omitempty"`
	ModelCount        int      `json:"modelCount"`
	DefaultLargeModel string   `json:"defaultLargeModel,omitempty"`
	DefaultSmallModel string   `json:"defaultSmallModel,omitempty"`
	RequiredFields    []string `json:"requiredFields,omitempty"`
	Notes             []string `json:"notes,omitempty"`
	Configurable      bool     `json:"configurable"`
}

type RuntimeProviderCatalogResponse struct {
	ProviderTypes []RuntimeProviderType        `json:"providerTypes"`
	Providers     []RuntimeProviderCatalogItem `json:"providers"`
}

type RuntimeConfiguredProvider struct {
	ID              string   `json:"id"`
	ProviderID      string   `json:"providerId"`
	Name            string   `json:"name"`
	Remark          string   `json:"remark,omitempty"`
	Protocol        string   `json:"protocol"`
	APIEndpoint     string   `json:"apiEndpoint"`
	APIKeySecretRef string   `json:"apiKeySecretRef,omitempty"`
	APIKey          string   `json:"apiKey,omitempty"`
	HasAPIKey       bool     `json:"hasApiKey"`
	Proxy           string   `json:"proxy,omitempty"`
	DefaultModel    string   `json:"defaultModel,omitempty"`
	Models          []string `json:"models,omitempty"`
	Enabled         bool     `json:"enabled"`
	CreatedAt       int64    `json:"createdAt"`
	UpdatedAt       int64    `json:"updatedAt"`
}

type RuntimeConfiguredProvidersResponse struct {
	Providers []RuntimeConfiguredProvider `json:"providers"`
}

type RuntimeConfiguredProviderRequest struct {
	ID           string   `json:"id,omitempty"`
	ProviderID   string   `json:"providerId"`
	Name         string   `json:"name"`
	Remark       string   `json:"remark,omitempty"`
	Protocol     string   `json:"protocol"`
	APIEndpoint  string   `json:"apiEndpoint"`
	APIKey       string   `json:"apiKey,omitempty"`
	Proxy        string   `json:"proxy,omitempty"`
	DefaultModel string   `json:"defaultModel,omitempty"`
	Models       []string `json:"models,omitempty"`
	Enabled      bool     `json:"enabled"`
}

type RuntimeConfiguredProviderResponse struct {
	Provider RuntimeConfiguredProvider `json:"provider"`
}

type RuntimeProviderModelDiscoveryResponse struct {
	ProviderID string   `json:"providerId"`
	Models     []string `json:"models"`
	Error      string   `json:"error,omitempty"`
}

type RuntimeProviderTestResponse struct {
	OK         bool   `json:"ok"`
	ProviderID string `json:"providerId"`
	Model      string `json:"model,omitempty"`
	DurationMS int64  `json:"durationMs,omitempty"`
	Error      string `json:"error,omitempty"`
}

type RuntimeConfigResponse struct {
	Config RuntimeModelConfig `json:"config"`
}

type RuntimeChatRequest struct {
	Prompt    string `json:"prompt"`
	SessionID string `json:"sessionId,omitempty"`
	ProjectID string `json:"projectId,omitempty"`
	Scope     string `json:"scope,omitempty"`
}

type RuntimeChatResponse struct {
	RequestID       string                  `json:"requestId"`
	TurnID          string                  `json:"turnId"`
	Status          RuntimeStatus           `json:"status"`
	NormalizedInput *RuntimeNormalizedInput `json:"normalizedInput,omitempty"`
}

type RuntimeUserInputRequest struct {
	SessionID string                  `json:"sessionId,omitempty"`
	ProjectID string                  `json:"projectId,omitempty"`
	Scope     string                  `json:"scope,omitempty"`
	Mode      string                  `json:"mode"`
	Items     []RuntimeUserInputItem  `json:"items"`
	Options   RuntimeUserInputOptions `json:"options,omitempty"`
}

type RuntimeUserInputItem struct {
	Type       string            `json:"type"`
	Text       string            `json:"text,omitempty"`
	Data       string            `json:"data,omitempty"`
	MIMEType   string            `json:"mimeType,omitempty"`
	FileName   string            `json:"fileName,omitempty"`
	SourcePath string            `json:"sourcePath,omitempty"`
	Metadata   map[string]string `json:"metadata,omitempty"`
}

type RuntimeUserInputOptions struct {
	IsMeta            bool   `json:"isMeta,omitempty"`
	SkipSlashCommands bool   `json:"skipSlashCommands,omitempty"`
	BridgeOrigin      bool   `json:"bridgeOrigin,omitempty"`
	VoiceSource       string `json:"voiceSource,omitempty"`
	ClientRequestID   string `json:"clientRequestId,omitempty"`
}

type RuntimeNormalizedInput struct {
	ID                   string                   `json:"id"`
	SessionID            string                   `json:"sessionId"`
	ProjectID            string                   `json:"projectId,omitempty"`
	Scope                string                   `json:"scope,omitempty"`
	Mode                 string                   `json:"mode"`
	Prompt               string                   `json:"prompt,omitempty"`
	Messages             []RuntimeMessageDraft    `json:"messages"`
	Attachments          []RuntimeAttachmentDraft `json:"attachments,omitempty"`
	ShouldQuery          bool                     `json:"shouldQuery"`
	Command              *RuntimeInputCommand     `json:"command,omitempty"`
	HookOutcome          *RuntimeInputHookOutcome `json:"hookOutcome,omitempty"`
	ModelOverride        string                   `json:"modelOverride,omitempty"`
	AllowedToolsOverride []string                 `json:"allowedToolsOverride,omitempty"`
	CreatedAt            int64                    `json:"createdAt"`
}

type RuntimeMessageDraft struct {
	Role          string            `json:"role"`
	Content       string            `json:"content,omitempty"`
	Hidden        bool              `json:"hidden,omitempty"`
	Mode          string            `json:"mode,omitempty"`
	ItemTypes     []string          `json:"itemTypes,omitempty"`
	Metadata      map[string]string `json:"metadata,omitempty"`
	AttachmentIDs []string          `json:"attachmentIds,omitempty"`
}

type RuntimeAttachmentDraft struct {
	ID         string            `json:"id"`
	Type       string            `json:"type"`
	MIMEType   string            `json:"mimeType,omitempty"`
	FileName   string            `json:"fileName,omitempty"`
	SourcePath string            `json:"sourcePath,omitempty"`
	Metadata   map[string]string `json:"metadata,omitempty"`
	SizeBytes  int               `json:"sizeBytes,omitempty"`
}

type RuntimeInputCommand struct {
	Name        string            `json:"name"`
	Args        string            `json:"args,omitempty"`
	Known       bool              `json:"known"`
	Runtime     bool              `json:"runtime"`
	ShouldQuery bool              `json:"shouldQuery"`
	ResultText  string            `json:"resultText,omitempty"`
	Strategy    string            `json:"strategy,omitempty"`
	Metadata    map[string]string `json:"metadata,omitempty"`
}

type RuntimeInputHookOutcome struct {
	Status              string            `json:"status"`
	PreventContinuation bool              `json:"preventContinuation,omitempty"`
	Blocking            bool              `json:"blocking,omitempty"`
	Reason              string            `json:"reason,omitempty"`
	Metadata            map[string]string `json:"metadata,omitempty"`
}

type RuntimeReactCallchainResponse struct {
	SessionID            string                      `json:"sessionId"`
	TurnID               string                      `json:"turnId,omitempty"`
	Nodes                []RuntimeReactCallNode      `json:"nodes"`
	Summary              RuntimeReactCallSummary     `json:"summary"`
	Source               RuntimeReactCallSource      `json:"source"`
	ToolResultDeliveries []RuntimeToolResultDelivery `json:"toolResultDeliveries,omitempty"`
}

type RuntimeToolResultDelivery struct {
	ToolCallID          string `json:"toolCallId"`
	ToolResultMessageID string `json:"toolResultMessageId,omitempty"`
	DeliveredToModel    bool   `json:"deliveredToModel"`
	DeliveredAtStep     int    `json:"deliveredAtStep,omitempty"`
	Synthetic           bool   `json:"synthetic,omitempty"`
	Reason              string `json:"reason,omitempty"`
}

type RuntimeReactCallNode struct {
	ID              string            `json:"id"`
	ParentID        string            `json:"parentId,omitempty"`
	Kind            string            `json:"kind"`
	SessionID       string            `json:"sessionId"`
	TurnID          string            `json:"turnId,omitempty"`
	MessageID       string            `json:"messageId,omitempty"`
	ToolCallID      string            `json:"toolCallId,omitempty"`
	PermissionID    string            `json:"permissionId,omitempty"`
	HookExecutionID string            `json:"hookExecutionId,omitempty"`
	Sequence        int               `json:"sequence"`
	Status          string            `json:"status,omitempty"`
	FinishReason    string            `json:"finishReason,omitempty"`
	Title           string            `json:"title,omitempty"`
	Summary         string            `json:"summary,omitempty"`
	Error           string            `json:"error,omitempty"`
	StartedAt       int64             `json:"startedAt,omitempty"`
	FinishedAt      int64             `json:"finishedAt,omitempty"`
	Evidence        map[string]string `json:"evidence,omitempty"`
}

type RuntimeReactCallSummary struct {
	HasFinalAssistant          bool                        `json:"hasFinalAssistant"`
	FinalAssistantMessageID    string                      `json:"finalAssistantMessageId,omitempty"`
	FinalAssistantEmpty        bool                        `json:"finalAssistantEmpty,omitempty"`
	LastAssistantFinishReason  string                      `json:"lastAssistantFinishReason,omitempty"`
	ToolCallCount              int                         `json:"toolCallCount"`
	PermissionCount            int                         `json:"permissionCount"`
	HookCount                  int                         `json:"hookCount"`
	StopReason                 string                      `json:"stopReason,omitempty"`
	StopReasonMessage          string                      `json:"stopReasonMessage,omitempty"`
	MissingEvidence            []string                    `json:"missingEvidence,omitempty"`
	ToolResultDeliveries       []RuntimeToolResultDelivery `json:"toolResultDeliveries,omitempty"`
	DeliveredToolResultCount   int                         `json:"deliveredToolResultCount,omitempty"`
	UndeliveredToolResultCount int                         `json:"undeliveredToolResultCount,omitempty"`
}

type RuntimeReactCallSource struct {
	SessionActivityParity bool `json:"sessionActivityParity"`
	UsesMessages          bool `json:"usesMessages"`
	UsesToolCalls         bool `json:"usesToolCalls"`
	UsesPermissions       bool `json:"usesPermissions"`
	UsesHooks             bool `json:"usesHooks"`
	EventsAreRefreshOnly  bool `json:"eventsAreRefreshOnly"`
}

type RuntimeTerminal struct {
	ID         string   `json:"id"`
	ProjectID  string   `json:"projectId"`
	SessionID  string   `json:"sessionId"`
	Title      string   `json:"title,omitempty"`
	CWD        string   `json:"cwd"`
	InitialCWD string   `json:"initialCwd,omitempty"`
	Shell      string   `json:"shell"`
	ShellPath  string   `json:"shellPath,omitempty"`
	ShellArgs  []string `json:"shellArgs,omitempty"`
	Columns    int      `json:"columns,omitempty"`
	Rows       int      `json:"rows,omitempty"`
	Status     string   `json:"status"`
	ExitCode   *int     `json:"exitCode,omitempty"`
	CreatedAt  int64    `json:"createdAt"`
	UpdatedAt  int64    `json:"updatedAt"`
}

type RuntimeTerminalCreateRequest struct {
	SessionID string   `json:"sessionId"`
	ID        string   `json:"id,omitempty"`
	CWD       string   `json:"cwd,omitempty"`
	ProfileID string   `json:"profileId,omitempty"`
	ShellPath string   `json:"shellPath,omitempty"`
	ShellArgs []string `json:"shellArgs,omitempty"`
	Columns   int      `json:"columns,omitempty"`
	Rows      int      `json:"rows,omitempty"`
}

type RuntimeTerminalInputRequest struct {
	Data      string `json:"data,omitempty"`
	BinaryB64 string `json:"binaryBase64,omitempty"`
}

type RuntimeTerminalResizeRequest struct {
	Columns int `json:"columns"`
	Rows    int `json:"rows"`
}

type RuntimeTerminalEvent struct {
	TerminalID string `json:"terminalId"`
	Sequence   int64  `json:"sequence"`
	Data       string `json:"data,omitempty"`
	BinaryB64  string `json:"binaryBase64,omitempty"`
	Final      bool   `json:"final,omitempty"`
	Status     string `json:"status,omitempty"`
	ExitCode   *int   `json:"exitCode,omitempty"`
	Error      string `json:"error,omitempty"`
}

type RuntimeTerminalResponse struct {
	Terminal RuntimeTerminal `json:"terminal"`
}

type RuntimeSessionTerminalsResponse struct {
	SessionID string            `json:"sessionId"`
	Terminals []RuntimeTerminal `json:"terminals"`
}

type RuntimeTerminalStreamRequest struct {
	Type      string `json:"type"`
	Data      string `json:"data,omitempty"`
	BinaryB64 string `json:"binaryBase64,omitempty"`
	Columns   int    `json:"columns,omitempty"`
	Rows      int    `json:"rows,omitempty"`
	Sequence  int64  `json:"sequence,omitempty"`
}

type RuntimeTerminalStreamMessage struct {
	Type   string                 `json:"type"`
	Events []RuntimeTerminalEvent `json:"events,omitempty"`
	Error  string                 `json:"error,omitempty"`
}

type RuntimeTurn struct {
	ID                       string                     `json:"id"`
	SessionID                string                     `json:"sessionId"`
	Status                   string                     `json:"status"`
	UserMessageID            string                     `json:"userMessageId,omitempty"`
	LatestAssistantMessageID string                     `json:"latestAssistantMessageId,omitempty"`
	StartedAt                int64                      `json:"startedAt"`
	FinishedAt               int64                      `json:"finishedAt,omitempty"`
	DurationMS               int64                      `json:"durationMs,omitempty"`
	Provider                 string                     `json:"provider,omitempty"`
	Model                    string                     `json:"model,omitempty"`
	PromptPreview            string                     `json:"promptPreview,omitempty"`
	UsageBefore              RuntimeUsage               `json:"usageBefore,omitempty"`
	UsageAfter               RuntimeUsage               `json:"usageAfter,omitempty"`
	UsageDelta               RuntimeUsage               `json:"usageDelta,omitempty"`
	LatestMessageID          string                     `json:"latestMessageId,omitempty"`
	LatestAssistant          RuntimeMessage             `json:"latestAssistant,omitempty"`
	Error                    string                     `json:"error,omitempty"`
	Diagnostics              RuntimeTurnDiagnostics     `json:"diagnostics,omitempty"`
	Interrupted              *RuntimeInterruptedSummary `json:"interrupted,omitempty"`
}

type RuntimeTurnResponse struct {
	Turn   RuntimeTurn                 `json:"turn"`
	Action *RuntimeWriteActionMetadata `json:"action,omitempty"`
}

type RuntimeTurnsResponse struct {
	Turns []RuntimeTurn `json:"turns"`
}

type RuntimeRunsResponse struct {
	Runs []RuntimeRun `json:"runs"`
}

type RuntimeRunSummariesResponse struct {
	Runs   []RuntimeRunSummary     `json:"runs"`
	Source RuntimeRunSummarySource `json:"source"`
}

type RuntimeRunSummaryResponse struct {
	Run    RuntimeRunSummary       `json:"run"`
	Source RuntimeRunSummarySource `json:"source"`
}

type RuntimeRunCheckpointMarkersResponse struct {
	Markers []RuntimeRunCheckpointMarker     `json:"markers"`
	Source  RuntimeRunCheckpointMarkerSource `json:"source"`
}

type RuntimeRunCheckpointMarkerResponse struct {
	Marker RuntimeRunCheckpointMarker       `json:"marker"`
	Source RuntimeRunCheckpointMarkerSource `json:"source"`
}

type RuntimeRunResponse struct {
	Run        RuntimeRun                  `json:"run"`
	Projection RuntimeRunProjection        `json:"projection,omitempty"`
	Action     *RuntimeWriteActionMetadata `json:"action,omitempty"`
}

type RuntimeRunResumeResponse struct {
	RunID        string                      `json:"runId"`
	CheckpointID string                      `json:"checkpointId"`
	SessionID    string                      `json:"sessionId"`
	TurnID       string                      `json:"turnId"`
	Chat         RuntimeChatResponse         `json:"chat"`
	Run          RuntimeRunResponse          `json:"run"`
	Action       *RuntimeWriteActionMetadata `json:"action,omitempty"`
}

type RuntimeRun struct {
	ID               string                 `json:"id"`
	WorkspaceID      string                 `json:"workspaceId"`
	PrimarySessionID string                 `json:"primarySessionId"`
	SessionIDs       []string               `json:"sessionIds,omitempty"`
	Objective        string                 `json:"objective,omitempty"`
	Status           string                 `json:"status"`
	Source           string                 `json:"source"`
	Checkpoints      []RuntimeRunCheckpoint `json:"checkpoints,omitempty"`
	CreatedAt        int64                  `json:"createdAt"`
	UpdatedAt        int64                  `json:"updatedAt"`
	FinishedAt       int64                  `json:"finishedAt,omitempty"`
	DiscardedAt      int64                  `json:"discardedAt,omitempty"`
}

type RuntimeRunSummary struct {
	ID               string   `json:"id"`
	WorkspaceID      string   `json:"workspaceId"`
	PrimarySessionID string   `json:"primarySessionId"`
	SessionIDs       []string `json:"sessionIds,omitempty"`
	Objective        string   `json:"objective,omitempty"`
	Source           string   `json:"source"`
	CreatedAt        int64    `json:"createdAt"`
	UpdatedAt        int64    `json:"updatedAt"`
}

type RuntimeRunSummarySource struct {
	Kind                           string   `json:"kind"`
	ReadOnly                       bool     `json:"readOnly"`
	SummaryOnly                    bool     `json:"summaryOnly"`
	PersistedRunAuthority          bool     `json:"persistedRunAuthority"`
	ProjectionRequiredForLifecycle bool     `json:"projectionRequiredForLifecycle"`
	ExcludedEvidence               []string `json:"excludedEvidence,omitempty"`
}

type RuntimeRunCheckpointMarker struct {
	RunID          string   `json:"runId"`
	CheckpointID   string   `json:"checkpointId"`
	TurnID         string   `json:"turnId,omitempty"`
	AcknowledgedAt int64    `json:"acknowledgedAt,omitempty"`
	DiscardedAt    int64    `json:"discardedAt,omitempty"`
	ResumedTurnIDs []string `json:"resumedTurnIds,omitempty"`
}

type RuntimeRunCheckpointMarkerSource struct {
	Kind                             string   `json:"kind"`
	ReadOnly                         bool     `json:"readOnly"`
	MarkerOnly                       bool     `json:"markerOnly"`
	PersistedRunAuthority            bool     `json:"persistedRunAuthority"`
	ProjectionRequiredForEligibility bool     `json:"projectionRequiredForEligibility"`
	ExcludedEvidence                 []string `json:"excludedEvidence,omitempty"`
}

type RuntimeRunProjectionRequest struct {
	SessionID string `json:"sessionId"`
	Limit     int    `json:"limit,omitempty"`
	Cursor    string `json:"cursor,omitempty"`
}

type RuntimeRunProjectionResponse struct {
	Run RuntimeRunProjection `json:"run"`
}

type RuntimeRunTransitionHistoryRequest struct {
	RunID     string `json:"runId,omitempty"`
	SessionID string `json:"sessionId,omitempty"`
	TurnID    string `json:"turnId,omitempty"`
	Cursor    string `json:"cursor,omitempty"`
	Limit     int    `json:"limit,omitempty"`
}

type RuntimeRunTransitionHistoryResponse struct {
	Transitions []RuntimeRunTransition            `json:"transitions"`
	Window      RuntimeActivityWindow             `json:"window"`
	Source      RuntimeRunTransitionHistorySource `json:"source"`
}

type RuntimeRunSchedulerPreflightRequest struct {
	RunID     string `json:"runId,omitempty"`
	SessionID string `json:"sessionId,omitempty"`
	TurnID    string `json:"turnId"`
}

type RuntimeRunSchedulerPreflightResponse struct {
	CanSchedule bool                               `json:"canSchedule"`
	Reason      string                             `json:"reason,omitempty"`
	RunID       string                             `json:"runId,omitempty"`
	SessionID   string                             `json:"sessionId,omitempty"`
	TurnID      string                             `json:"turnId,omitempty"`
	Source      RuntimeRunSchedulerPreflightSource `json:"source"`
}

type RuntimeRunSchedulerPreflightSource struct {
	Kind         string   `json:"kind"`
	ReadOnly     bool     `json:"readOnly"`
	StartsWorker bool     `json:"startsWorker"`
	Evidence     []string `json:"evidence,omitempty"`
}

type RuntimeRunSchedulerPlanRequest struct {
	RunID        string `json:"runId,omitempty"`
	SessionID    string `json:"sessionId,omitempty"`
	Mode         string `json:"mode,omitempty"`
	TurnID       string `json:"turnId,omitempty"`
	CheckpointID string `json:"checkpointId,omitempty"`
	TaskID       string `json:"taskId,omitempty"`
	Cursor       string `json:"cursor,omitempty"`
	Limit        int    `json:"limit,omitempty"`
}

type RuntimeRunSchedulerPlanResponse struct {
	Plan   RuntimeRunSchedulerPlan       `json:"plan"`
	Source RuntimeRunSchedulerPlanSource `json:"source"`
}

type RuntimeRunSchedulerPlan struct {
	RunID               string                        `json:"runId"`
	PrimarySessionID    string                        `json:"primarySessionId"`
	SessionIDs          []string                      `json:"sessionIds,omitempty"`
	Objective           string                        `json:"objective,omitempty"`
	StatusFromRunDetail string                        `json:"statusFromRunDetail,omitempty"`
	Items               []RuntimeRunSchedulerPlanItem `json:"items,omitempty"`
	CancellationScope   string                        `json:"cancellationScope,omitempty"`
	DiagnosticsRoute    string                        `json:"diagnosticsRoute,omitempty"`
	RefreshTargets      []string                      `json:"refreshTargets,omitempty"`
	ActivityWindow      RuntimeActivityWindow         `json:"activityWindow,omitempty"`
}

type RuntimeRunSchedulerPlanItem struct {
	ID                string                       `json:"id"`
	Kind              string                       `json:"kind"`
	OrderKey          string                       `json:"orderKey,omitempty"`
	SessionID         string                       `json:"sessionId,omitempty"`
	TurnID            string                       `json:"turnId,omitempty"`
	CheckpointID      string                       `json:"checkpointId,omitempty"`
	TaskID            string                       `json:"taskId,omitempty"`
	CanSchedule       bool                         `json:"canSchedule"`
	PreflightReason   string                       `json:"preflightReason,omitempty"`
	OwnershipVerified bool                         `json:"ownershipVerified,omitempty"`
	RequiredPreflight bool                         `json:"requiredPreflight"`
	RefreshTargets    []string                     `json:"refreshTargets,omitempty"`
	CancellationScope string                       `json:"cancellationScope,omitempty"`
	DiagnosticsRoute  string                       `json:"diagnosticsRoute,omitempty"`
	TaskScope         RuntimeRunSchedulerTaskScope `json:"taskScope,omitempty"`
}

type RuntimeRunSchedulerTaskScope struct {
	AllowedTools     []string `json:"allowedTools,omitempty"`
	CapabilityScope  []string `json:"capabilityScope,omitempty"`
	CWD              string   `json:"cwd,omitempty"`
	Worktree         string   `json:"worktree,omitempty"`
	Role             string   `json:"role,omitempty"`
	Provider         string   `json:"provider,omitempty"`
	Model            string   `json:"model,omitempty"`
	ParentToolCallID string   `json:"parentToolCallId,omitempty"`
	ChildSessionID   string   `json:"childSessionId,omitempty"`
}

type RuntimeRunSchedulerPlanSource struct {
	Kind                  string   `json:"kind"`
	ReadOnly              bool     `json:"readOnly"`
	StartsWorker          bool     `json:"startsWorker"`
	SessionActivityParity bool     `json:"sessionActivityParity"`
	Evidence              []string `json:"evidence,omitempty"`
}

type RuntimeRunSchedulerExecuteTaskRequest struct {
	RunID  string `json:"runId,omitempty"`
	TaskID string `json:"taskId,omitempty"`
}

type RuntimeRunSchedulerExecuteTaskResponse struct {
	Accepted         bool                                 `json:"accepted"`
	ExecutionStarted bool                                 `json:"executionStarted"`
	Reason           string                               `json:"reason,omitempty"`
	Plan             RuntimeRunSchedulerPlanResponse      `json:"plan,omitempty"`
	Task             RuntimeAgentTask                     `json:"task,omitempty"`
	RefreshTargets   []string                             `json:"refreshTargets,omitempty"`
	Source           RuntimeRunSchedulerExecuteTaskSource `json:"source"`
	Action           *RuntimeWriteActionMetadata          `json:"action,omitempty"`
}

type RuntimeRunSchedulerExecuteTaskSource struct {
	Kind                  string   `json:"kind"`
	Action                string   `json:"action"`
	BackendOnly           bool     `json:"backendOnly"`
	StartsWorker          bool     `json:"startsWorker"`
	IdempotentByTaskID    bool     `json:"idempotentByTaskId"`
	SessionActivityParity bool     `json:"sessionActivityParity"`
	Evidence              []string `json:"evidence,omitempty"`
}

type RuntimeWriteActionMetadata struct {
	Accepted       bool                     `json:"accepted"`
	Reason         string                   `json:"reason,omitempty"`
	RefreshTargets []string                 `json:"refreshTargets,omitempty"`
	Source         RuntimeWriteActionSource `json:"source"`
}

type RuntimeWriteActionSource struct {
	Kind                  string   `json:"kind"`
	Action                string   `json:"action"`
	BackendOnly           bool     `json:"backendOnly"`
	StartsWorker          bool     `json:"startsWorker"`
	IdempotentBy          string   `json:"idempotentBy,omitempty"`
	SessionActivityParity bool     `json:"sessionActivityParity"`
	Evidence              []string `json:"evidence,omitempty"`
}

type RuntimeAgentTaskExecutionRequest struct {
	RunID                   string   `json:"runId"`
	TaskID                  string   `json:"taskId"`
	ParentSessionID         string   `json:"parentSessionId"`
	ParentTurnID            string   `json:"parentTurnId"`
	ParentToolCallID        string   `json:"parentToolCallId,omitempty"`
	ChildSessionID          string   `json:"childSessionId,omitempty"`
	Title                   string   `json:"title,omitempty"`
	Kind                    string   `json:"kind,omitempty"`
	Role                    string   `json:"role,omitempty"`
	Name                    string   `json:"name,omitempty"`
	Prompt                  string   `json:"prompt,omitempty"`
	PromptSummary           string   `json:"promptSummary,omitempty"`
	Provider                string   `json:"provider,omitempty"`
	Model                   string   `json:"model,omitempty"`
	AllowedTools            []string `json:"allowedTools,omitempty"`
	CapabilityScope         []string `json:"capabilityScope,omitempty"`
	CWD                     string   `json:"cwd,omitempty"`
	Worktree                string   `json:"worktree,omitempty"`
	StartedAt               int64    `json:"startedAt,omitempty"`
	StartAlreadyRecorded    bool     `json:"startAlreadyRecorded"`
	BackendOnly             bool     `json:"backendOnly"`
	EventPayloadRefreshOnly bool     `json:"eventPayloadRefreshOnly"`
}

type RuntimeAgentTaskExecutionResult struct {
	TaskID             string   `json:"taskId"`
	Status             string   `json:"status,omitempty"`
	Terminal           bool     `json:"terminal"`
	RefreshTargets     []string `json:"refreshTargets,omitempty"`
	ArtifactRefs       []string `json:"artifactRefs,omitempty"`
	ResultSummary      string   `json:"resultSummary,omitempty"`
	Error              string   `json:"error,omitempty"`
	NoStaleResume      bool     `json:"noStaleResume"`
	CompletionOnlyRefs bool     `json:"completionOnlyRefs"`
}

type RuntimeRunTransitionHistorySource struct {
	Kind                  string   `json:"kind"`
	ReadOnly              bool     `json:"readOnly"`
	AuditOnly             bool     `json:"auditOnly"`
	SessionActivityParity bool     `json:"sessionActivityParity"`
	Evidence              []string `json:"evidence,omitempty"`
}

type RuntimeRunProjection struct {
	ID                   string                     `json:"id"`
	WorkspaceID          string                     `json:"workspaceId,omitempty"`
	SessionIDs           []string                   `json:"sessionIds"`
	PrimarySessionID     string                     `json:"primarySessionId"`
	Objective            string                     `json:"objective,omitempty"`
	Status               string                     `json:"status"`
	TurnIDs              []string                   `json:"turnIds,omitempty"`
	TaskIDs              []string                   `json:"taskIds,omitempty"`
	ToolCallIDs          []string                   `json:"toolCallIds,omitempty"`
	PermissionRequestIDs []string                   `json:"permissionRequestIds,omitempty"`
	ExpectedArtifacts    []string                   `json:"expectedArtifacts,omitempty"`
	ProducedArtifacts    []string                   `json:"producedArtifacts,omitempty"`
	VerifiedArtifacts    []string                   `json:"verifiedArtifacts,omitempty"`
	Checkpoints          []RuntimeRunCheckpoint     `json:"checkpoints,omitempty"`
	Diagnostics          RuntimeRunDiagnostics      `json:"diagnostics,omitempty"`
	Interrupted          *RuntimeInterruptedSummary `json:"interrupted,omitempty"`
	UserActions          RuntimeRunUserActions      `json:"userActions,omitempty"`
	EvidenceCursor       string                     `json:"evidenceCursor,omitempty"`
	ActivityWindow       RuntimeActivityWindow      `json:"activityWindow,omitempty"`
	Source               RuntimeRunProjectionSource `json:"source"`
	CreatedAt            int64                      `json:"createdAt,omitempty"`
	UpdatedAt            int64                      `json:"updatedAt,omitempty"`
	FinishedAt           int64                      `json:"finishedAt,omitempty"`
}

type RuntimeRunCheckpoint struct {
	ID             string   `json:"id"`
	TurnID         string   `json:"turnId,omitempty"`
	TaskID         string   `json:"taskId,omitempty"`
	Status         string   `json:"status"`
	Summary        string   `json:"summary,omitempty"`
	ArtifactRefs   []string `json:"artifactRefs,omitempty"`
	CreatedAt      int64    `json:"createdAt,omitempty"`
	AcknowledgedAt int64    `json:"acknowledgedAt,omitempty"`
	DiscardedAt    int64    `json:"discardedAt,omitempty"`
	ResumedTurnIDs []string `json:"resumedTurnIds,omitempty"`
	ResumeEligible bool     `json:"resumeEligible,omitempty"`
}

type RuntimeRunDiagnostics struct {
	TurnCount                  int                     `json:"turnCount,omitempty"`
	TaskCount                  int                     `json:"taskCount,omitempty"`
	ToolCallCount              int                     `json:"toolCallCount,omitempty"`
	PermissionRequestCount     int                     `json:"permissionRequestCount,omitempty"`
	InterruptedTurnCount       int                     `json:"interruptedTurnCount,omitempty"`
	FailedTurnCount            int                     `json:"failedTurnCount,omitempty"`
	CancelledTurnCount         int                     `json:"cancelledTurnCount,omitempty"`
	RunningTurnCount           int                     `json:"runningTurnCount,omitempty"`
	WaitingPermissionTurnCount int                     `json:"waitingPermissionTurnCount,omitempty"`
	TerminalPermissionCounts   RuntimePermissionCounts `json:"terminalPermissionCounts,omitempty"`
	ArtifactCounts             RuntimeArtifactCounts   `json:"artifactCounts,omitempty"`
	ToolCountsByStatus         map[string]int          `json:"toolCountsByStatus,omitempty"`
	Warning                    string                  `json:"warning,omitempty"`
	WarningReason              string                  `json:"warningReason,omitempty"`
}

type RuntimeRunUserActions struct {
	Resume  []RuntimeRunUserAction `json:"resume,omitempty"`
	Discard []RuntimeRunUserAction `json:"discard,omitempty"`
}

type RuntimeRunUserAction struct {
	ID      string `json:"id"`
	TurnID  string `json:"turnId,omitempty"`
	Kind    string `json:"kind"`
	Label   string `json:"label,omitempty"`
	Enabled bool   `json:"enabled"`
	Reason  string `json:"reason,omitempty"`
}

type RuntimeRunProjectionSource struct {
	Kind                  string   `json:"kind"`
	ReadOnly              bool     `json:"readOnly"`
	SessionActivityParity bool     `json:"sessionActivityParity"`
	Evidence              []string `json:"evidence,omitempty"`
}

type RuntimeTurnDiagnostics struct {
	TurnID                    string                    `json:"turnId,omitempty"`
	SessionID                 string                    `json:"sessionId,omitempty"`
	Status                    string                    `json:"status,omitempty"`
	StartedAt                 int64                     `json:"startedAt,omitempty"`
	FinishedAt                int64                     `json:"finishedAt,omitempty"`
	DurationMS                int64                     `json:"durationMs,omitempty"`
	RunningDurationMS         int64                     `json:"runningDurationMs,omitempty"`
	ComputedAt                int64                     `json:"computedAt,omitempty"`
	ExpectedArtifacts         []string                  `json:"expectedArtifacts,omitempty"`
	ProducedArtifacts         []string                  `json:"producedArtifacts,omitempty"`
	VerifiedArtifacts         []string                  `json:"verifiedArtifacts,omitempty"`
	UnverifiedArtifacts       []string                  `json:"unverifiedArtifacts,omitempty"`
	MissingArtifacts          []string                  `json:"missingArtifacts,omitempty"`
	ArtifactVerificationAt    int64                     `json:"artifactVerificationAt,omitempty"`
	ArtifactCounts            RuntimeArtifactCounts     `json:"artifactCounts,omitempty"`
	ArtifactConfidenceSummary RuntimeArtifactConfidence `json:"artifactConfidenceSummary,omitempty"`
	ToolCountsByStatus        map[string]int            `json:"toolCountsByStatus,omitempty"`
	ToolCountsByKind          map[string]int            `json:"toolCountsByKind,omitempty"`
	FailedToolCount           int                       `json:"failedToolCount,omitempty"`
	DeniedToolCount           int                       `json:"deniedToolCount,omitempty"`
	CancelledToolCount        int                       `json:"cancelledToolCount,omitempty"`
	NonzeroExitShellCount     int                       `json:"nonzeroExitShellCount,omitempty"`
	PermissionCounts          RuntimePermissionCounts   `json:"permissionCounts,omitempty"`
	LastToolID                string                    `json:"lastToolId,omitempty"`
	LastToolStatus            string                    `json:"lastToolStatus,omitempty"`
	LastToolTitle             string                    `json:"lastToolTitle,omitempty"`
	LastRuntimeEventAt        int64                     `json:"lastRuntimeEventAt,omitempty"`
	LastRuntimeEventSequence  int64                     `json:"lastRuntimeEventSequence,omitempty"`
	Warning                   string                    `json:"warning,omitempty"`
	WarningReason             string                    `json:"warningReason,omitempty"`
	WarningSource             string                    `json:"warningSource,omitempty"`
}

type RuntimeArtifactCounts struct {
	Expected             int `json:"expected,omitempty"`
	Produced             int `json:"produced,omitempty"`
	Verified             int `json:"verified,omitempty"`
	Missing              int `json:"missing,omitempty"`
	LocalDeliverables    int `json:"localDeliverables,omitempty"`
	RuntimeRefs          int `json:"runtimeRefs,omitempty"`
	ProducedMetadataRefs int `json:"producedMetadataRefs,omitempty"`
	StructuredRefs       int `json:"structuredRefs,omitempty"`
}

type RuntimeArtifactConfidence struct {
	LocalVerifiedFile       int `json:"localVerifiedFile,omitempty"`
	ProducedToolMetadata    int `json:"producedToolMetadata,omitempty"`
	RuntimeOutputRefs       int `json:"runtimeOutputRefs,omitempty"`
	StructuredMCPCustomRefs int `json:"structuredMcpCustomRefs,omitempty"`
	UnknownNotDetected      int `json:"unknownNotDetected,omitempty"`
}

type RuntimePermissionCounts struct {
	Pending   int `json:"pending,omitempty"`
	Allowed   int `json:"allowed,omitempty"`
	Denied    int `json:"denied,omitempty"`
	Expired   int `json:"expired,omitempty"`
	Cancelled int `json:"cancelled,omitempty"`
}

type RuntimeInterruptedSummary struct {
	TurnID                   string                        `json:"turnId,omitempty"`
	SessionID                string                        `json:"sessionId,omitempty"`
	Status                   string                        `json:"status,omitempty"`
	StartedAt                int64                         `json:"startedAt,omitempty"`
	InterruptedAt            int64                         `json:"interruptedAt,omitempty"`
	DurationMS               int64                         `json:"durationMs,omitempty"`
	Reason                   string                        `json:"reason,omitempty"`
	Source                   string                        `json:"source,omitempty"`
	LastCompletedTool        RuntimeInterruptedToolSummary `json:"lastCompletedTool,omitempty"`
	LastFailedTool           RuntimeInterruptedToolSummary `json:"lastFailedTool,omitempty"`
	PendingTool              RuntimeInterruptedToolSummary `json:"pendingTool,omitempty"`
	ExpectedArtifacts        []string                      `json:"expectedArtifacts,omitempty"`
	ProducedArtifacts        []string                      `json:"producedArtifacts,omitempty"`
	VerifiedArtifacts        []string                      `json:"verifiedArtifacts,omitempty"`
	MissingArtifacts         []string                      `json:"missingArtifacts,omitempty"`
	ArtifactCounts           RuntimeArtifactCounts         `json:"artifactCounts,omitempty"`
	PermissionCounts         RuntimePermissionCounts       `json:"permissionCounts,omitempty"`
	FailedToolCount          int                           `json:"failedToolCount,omitempty"`
	DeniedToolCount          int                           `json:"deniedToolCount,omitempty"`
	CancelledToolCount       int                           `json:"cancelledToolCount,omitempty"`
	NonzeroExitShellCount    int                           `json:"nonzeroExitShellCount,omitempty"`
	LastRuntimeEventAt       int64                         `json:"lastRuntimeEventAt,omitempty"`
	LastRuntimeEventSequence int64                         `json:"lastRuntimeEventSequence,omitempty"`
	SummaryText              string                        `json:"summaryText,omitempty"`
}

type RuntimeInterruptedToolSummary struct {
	ID            string                 `json:"id,omitempty"`
	Name          string                 `json:"name,omitempty"`
	Source        string                 `json:"source,omitempty"`
	Status        string                 `json:"status,omitempty"`
	StartedAt     int64                  `json:"startedAt,omitempty"`
	FinishedAt    int64                  `json:"finishedAt,omitempty"`
	Command       string                 `json:"command,omitempty"`
	WorkingDir    string                 `json:"workingDir,omitempty"`
	ExitCode      *int                   `json:"exitCode,omitempty"`
	Target        string                 `json:"target,omitempty"`
	Targets       []string               `json:"targets,omitempty"`
	StdoutExcerpt string                 `json:"stdoutExcerpt,omitempty"`
	StderrExcerpt string                 `json:"stderrExcerpt,omitempty"`
	FailureReason string                 `json:"failureReason,omitempty"`
	ArtifactRefs  []string               `json:"artifactRefs,omitempty"`
	DiffRefs      []string               `json:"diffRefs,omitempty"`
	Display       RuntimeToolCallDisplay `json:"display,omitempty"`
}

type RuntimeToolCall struct {
	ID                             string                 `json:"id"`
	SessionID                      string                 `json:"sessionId"`
	TurnID                         string                 `json:"turnId"`
	MessageID                      string                 `json:"messageId,omitempty"`
	Name                           string                 `json:"name"`
	Source                         string                 `json:"source"`
	CapabilityID                   string                 `json:"capabilityId,omitempty"`
	JobID                          string                 `json:"jobId,omitempty"`
	Command                        string                 `json:"command,omitempty"`
	Risk                           string                 `json:"risk,omitempty"`
	PolicyReason                   string                 `json:"policyReason,omitempty"`
	PolicyMode                     string                 `json:"policyMode,omitempty"`
	PolicyProfile                  string                 `json:"policyProfile,omitempty"`
	PolicyHeadless                 bool                   `json:"policyHeadless,omitempty"`
	PolicyHeadlessReason           string                 `json:"policyHeadlessReason,omitempty"`
	PolicyRuleID                   string                 `json:"policyRuleId,omitempty"`
	PolicyRuleSource               string                 `json:"policyRuleSource,omitempty"`
	PolicyScopeKind                string                 `json:"policyScopeKind,omitempty"`
	PolicyScopeValue               string                 `json:"policyScopeValue,omitempty"`
	PolicyTargetSummary            string                 `json:"policyTargetSummary,omitempty"`
	ShellRisk                      string                 `json:"shellRisk,omitempty"`
	ShellReason                    string                 `json:"shellReason,omitempty"`
	SandboxDecisionID              string                 `json:"sandboxDecisionId,omitempty"`
	SandboxMode                    string                 `json:"sandboxMode,omitempty"`
	SandboxStatus                  string                 `json:"sandboxStatus,omitempty"`
	SandboxExecutor                string                 `json:"sandboxExecutor,omitempty"`
	SandboxReason                  string                 `json:"sandboxReason,omitempty"`
	SandboxError                   string                 `json:"sandboxError,omitempty"`
	ExitCode                       int                    `json:"exitCode,omitempty"`
	JobStatus                      string                 `json:"jobStatus,omitempty"`
	JobStartedAt                   int64                  `json:"jobStartedAt,omitempty"`
	JobFinishedAt                  int64                  `json:"jobFinishedAt,omitempty"`
	Status                         string                 `json:"status"`
	InputSummary                   string                 `json:"inputSummary,omitempty"`
	OutputSummary                  string                 `json:"outputSummary,omitempty"`
	ModelContent                   string                 `json:"modelContent,omitempty"`
	Structured                     string                 `json:"structuredOutput,omitempty"`
	Stdout                         string                 `json:"stdout,omitempty"`
	Stderr                         string                 `json:"stderr,omitempty"`
	OutputRefs                     []string               `json:"outputRefs,omitempty"`
	ArtifactRefs                   []string               `json:"artifactRefs,omitempty"`
	DiffRefs                       []string               `json:"diffRefs,omitempty"`
	IsError                        bool                   `json:"isError,omitempty"`
	Compacted                      bool                   `json:"compacted,omitempty"`
	CompactRef                     string                 `json:"compactRef,omitempty"`
	CompactBoundaryID              string                 `json:"compactBoundaryId,omitempty"`
	CompactOriginalEstimatedTokens int                    `json:"compactOriginalEstimatedTokens,omitempty"`
	CompactedAt                    int64                  `json:"compactedAt,omitempty"`
	StartedAt                      int64                  `json:"startedAt"`
	FinishedAt                     int64                  `json:"finishedAt,omitempty"`
	Error                          string                 `json:"error,omitempty"`
	Display                        RuntimeToolCallDisplay `json:"display,omitempty"`
}

type RuntimeToolCallDisplay struct {
	Kind            string   `json:"kind,omitempty"`
	Title           string   `json:"title,omitempty"`
	Detail          string   `json:"detail,omitempty"`
	Target          string   `json:"target,omitempty"`
	PrimaryTarget   string   `json:"primaryTarget,omitempty"`
	Targets         []string `json:"targets,omitempty"`
	WorkingDir      string   `json:"workingDir,omitempty"`
	Command         string   `json:"command,omitempty"`
	ExitCode        *int     `json:"exitCode,omitempty"`
	DurationMS      int64    `json:"durationMs,omitempty"`
	StdoutExcerpt   string   `json:"stdoutExcerpt,omitempty"`
	StderrExcerpt   string   `json:"stderrExcerpt,omitempty"`
	InputExcerpt    string   `json:"inputExcerpt,omitempty"`
	OutputExcerpt   string   `json:"outputExcerpt,omitempty"`
	FailureReason   string   `json:"failureReason,omitempty"`
	ArtifactCount   int      `json:"artifactCount,omitempty"`
	DiffCount       int      `json:"diffCount,omitempty"`
	ArtifactRefs    []string `json:"artifactRefs,omitempty"`
	DiffRefs        []string `json:"diffRefs,omitempty"`
	ArtifactSummary string   `json:"artifactSummary,omitempty"`
	DiffSummary     string   `json:"diffSummary,omitempty"`
}

type RuntimeRef struct {
	ID                string `json:"id"`
	URI               string `json:"uri"`
	SessionID         string `json:"sessionId"`
	TurnID            string `json:"turnId,omitempty"`
	ToolCallID        string `json:"toolCallId,omitempty"`
	TaskID            string `json:"taskId,omitempty"`
	Kind              string `json:"kind"`
	MediaType         string `json:"mediaType,omitempty"`
	ContentType       string `json:"contentType,omitempty"`
	SizeBytes         int64  `json:"sizeBytes"`
	EstimatedTokens   int    `json:"estimatedTokens"`
	Preview           string `json:"preview,omitempty"`
	Summary           string `json:"summary,omitempty"`
	StorageKind       string `json:"storageKind"`
	StoragePath       string `json:"storagePath,omitempty"`
	InlinePayload     string `json:"-"`
	RedactionStatus   string `json:"redactionStatus"`
	SandboxDecisionID string `json:"sandboxDecisionId,omitempty"`
	SandboxMode       string `json:"sandboxMode,omitempty"`
	SandboxStatus     string `json:"sandboxStatus,omitempty"`
	CreatedAt         int64  `json:"createdAt"`
	CanReadContent    bool   `json:"canReadContent"`
}

type RuntimeRefListRequest struct {
	SessionID  string `json:"sessionId,omitempty"`
	TurnID     string `json:"turnId,omitempty"`
	ToolCallID string `json:"toolCallId,omitempty"`
	TaskID     string `json:"taskId,omitempty"`
	Kind       string `json:"kind,omitempty"`
}

type RuntimeRefResponse struct {
	Ref RuntimeRef `json:"ref"`
}

type RuntimeRefsResponse struct {
	Refs []RuntimeRef `json:"refs"`
}

type RuntimeRefContentResponse struct {
	Ref       RuntimeRef `json:"ref"`
	Content   string     `json:"content,omitempty"`
	Redacted  bool       `json:"redacted,omitempty"`
	Truncated bool       `json:"truncated,omitempty"`
}

type RuntimeToolCallResponse struct {
	ToolCall RuntimeToolCall `json:"toolCall"`
}

type RuntimeToolCallsResponse struct {
	ToolCalls []RuntimeToolCall `json:"toolCalls"`
}

type RuntimeHook struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Source      string `json:"source"`
	Event       string `json:"event"`
	Matcher     string `json:"matcher,omitempty"`
	Enabled     bool   `json:"enabled"`
	Status      string `json:"status"`
	Diagnostics string `json:"diagnostics,omitempty"`
	Reason      string `json:"reason,omitempty"`
	Timeout     int    `json:"timeout,omitempty"`
}

type RuntimeHookExecution struct {
	ID                string `json:"id"`
	HookID            string `json:"hookId"`
	HookName          string `json:"hookName,omitempty"`
	HookSource        string `json:"hookSource,omitempty"`
	Event             string `json:"event"`
	Status            string `json:"status"`
	SessionID         string `json:"sessionId,omitempty"`
	TurnID            string `json:"turnId,omitempty"`
	ToolCallID        string `json:"toolCallId,omitempty"`
	TaskID            string `json:"taskId,omitempty"`
	CapabilityID      string `json:"capabilityId,omitempty"`
	MCPServer         string `json:"mcpServer,omitempty"`
	Skill             string `json:"skill,omitempty"`
	ContextRef        string `json:"contextRef,omitempty"`
	PolicyMode        string `json:"policyMode,omitempty"`
	PolicyProfile     string `json:"policyProfile,omitempty"`
	PolicyRule        string `json:"policyRule,omitempty"`
	PolicyDecision    string `json:"policyDecision,omitempty"`
	PolicyReason      string `json:"policyReason,omitempty"`
	Headless          bool   `json:"headless,omitempty"`
	HeadlessReason    string `json:"headlessReason,omitempty"`
	SandboxDecisionID string `json:"sandboxDecisionId,omitempty"`
	SandboxStatus     string `json:"sandboxStatus,omitempty"`
	ScopeKind         string `json:"scopeKind,omitempty"`
	ScopeValue        string `json:"scopeValue,omitempty"`
	Reason            string `json:"reason,omitempty"`
	Error             string `json:"error,omitempty"`
	InputSummary      string `json:"inputSummary,omitempty"`
	OutputSummary     string `json:"outputSummary,omitempty"`
	ContextSummary    string `json:"contextSummary,omitempty"`
	InputRewritten    bool   `json:"inputRewritten,omitempty"`
	ContextInjected   bool   `json:"contextInjected,omitempty"`
	Redacted          bool   `json:"redacted"`
	StartedAt         int64  `json:"startedAt"`
	CompletedAt       int64  `json:"completedAt,omitempty"`
	DurationMS        int64  `json:"durationMs,omitempty"`
}

type RuntimeHooksResponse struct {
	Hooks       []RuntimeHook `json:"hooks"`
	Diagnostics []string      `json:"diagnostics,omitempty"`
}

type RuntimeHookExecutionsRequest struct {
	SessionID  string `json:"sessionId,omitempty"`
	TurnID     string `json:"turnId,omitempty"`
	ToolCallID string `json:"toolCallId,omitempty"`
	TaskID     string `json:"taskId,omitempty"`
	Event      string `json:"event,omitempty"`
	Status     string `json:"status,omitempty"`
}

type RuntimeHookExecutionResponse struct {
	Execution RuntimeHookExecution `json:"execution"`
}

type RuntimeHookExecutionsResponse struct {
	Executions []RuntimeHookExecution `json:"executions"`
}

type RuntimeAgentTask struct {
	ID                 string                  `json:"id"`
	ParentTurnID       string                  `json:"parentTurnId,omitempty"`
	ParentSessionID    string                  `json:"parentSessionId"`
	ParentToolCallID   string                  `json:"parentToolCallId,omitempty"`
	ChildSessionID     string                  `json:"childSessionId,omitempty"`
	Title              string                  `json:"title"`
	Kind               string                  `json:"kind"`
	Role               string                  `json:"role,omitempty"`
	Name               string                  `json:"name,omitempty"`
	PromptSummary      string                  `json:"promptSummary,omitempty"`
	Model              string                  `json:"model,omitempty"`
	Provider           string                  `json:"provider,omitempty"`
	AllowedTools       []string                `json:"allowedTools,omitempty"`
	CapabilityScope    []string                `json:"capabilityScope,omitempty"`
	CWD                string                  `json:"cwd,omitempty"`
	Worktree           string                  `json:"worktree,omitempty"`
	Status             string                  `json:"status"`
	Progress           int                     `json:"progress"`
	ResultSummary      string                  `json:"resultSummary,omitempty"`
	ArtifactRefs       []string                `json:"artifactRefs,omitempty"`
	StartedAt          int64                   `json:"startedAt"`
	UpdatedAt          int64                   `json:"updatedAt"`
	FinishedAt         int64                   `json:"finishedAt,omitempty"`
	Error              string                  `json:"error,omitempty"`
	CancellationDetail string                  `json:"cancellationDetail,omitempty"`
	Result             *RuntimeAgentTaskResult `json:"result,omitempty"`
}

type RuntimeAgentTaskResponse struct {
	Task     RuntimeAgentTask            `json:"task"`
	Messages []RuntimeAgentTaskMessage   `json:"messages,omitempty"`
	Result   *RuntimeAgentTaskResult     `json:"result,omitempty"`
	Action   *RuntimeWriteActionMetadata `json:"action,omitempty"`
}

type RuntimeAgentTasksResponse struct {
	Tasks []RuntimeAgentTask `json:"tasks"`
}

type RuntimeWorktree struct {
	ID             string            `json:"id"`
	SessionID      string            `json:"sessionId"`
	TurnID         string            `json:"turnId,omitempty"`
	TaskID         string            `json:"taskId,omitempty"`
	BaseRepoPath   string            `json:"baseRepoPath"`
	WorktreePath   string            `json:"worktreePath"`
	Branch         string            `json:"branch"`
	Ref            string            `json:"ref,omitempty"`
	Status         string            `json:"status"`
	PreservePolicy string            `json:"preservePolicy"`
	CleanupPolicy  string            `json:"cleanupPolicy"`
	CreatedAt      int64             `json:"createdAt"`
	EnteredAt      int64             `json:"enteredAt,omitempty"`
	ExitedAt       int64             `json:"exitedAt,omitempty"`
	CleanedAt      int64             `json:"cleanedAt,omitempty"`
	UpdatedAt      int64             `json:"updatedAt"`
	Error          string            `json:"error,omitempty"`
	Owner          string            `json:"owner,omitempty"`
	Metadata       map[string]string `json:"metadata,omitempty"`
}

type RuntimeWorktreesResponse struct {
	Worktrees []RuntimeWorktree `json:"worktrees"`
}

type RuntimeWorktreeResponse struct {
	Worktree RuntimeWorktree `json:"worktree"`
}

type RuntimeWorktreeCreateRequest struct {
	SessionID      string `json:"sessionId,omitempty"`
	TurnID         string `json:"turnId,omitempty"`
	TaskID         string `json:"taskId,omitempty"`
	BaseRepoPath   string `json:"baseRepoPath,omitempty"`
	Branch         string `json:"branch,omitempty"`
	Ref            string `json:"ref,omitempty"`
	Name           string `json:"name,omitempty"`
	PreservePolicy string `json:"preservePolicy,omitempty"`
	CleanupPolicy  string `json:"cleanupPolicy,omitempty"`
}

type RuntimeWorktreeActionRequest struct {
	SessionID      string `json:"sessionId,omitempty"`
	TurnID         string `json:"turnId,omitempty"`
	TaskID         string `json:"taskId,omitempty"`
	PreservePolicy string `json:"preservePolicy,omitempty"`
}

type RuntimeEffectiveScope struct {
	SessionID    string           `json:"sessionId,omitempty"`
	TurnID       string           `json:"turnId,omitempty"`
	TaskID       string           `json:"taskId,omitempty"`
	BaseCWD      string           `json:"baseCwd,omitempty"`
	EffectiveCWD string           `json:"effectiveCwd,omitempty"`
	WorktreeID   string           `json:"worktreeId,omitempty"`
	WorktreePath string           `json:"worktreePath,omitempty"`
	Worktree     *RuntimeWorktree `json:"worktree,omitempty"`
	Sandbox      string           `json:"sandbox,omitempty"`
	Remote       string           `json:"remote,omitempty"`
}

type RuntimeSandboxDecision struct {
	ID             string   `json:"id"`
	SessionID      string   `json:"sessionId"`
	TurnID         string   `json:"turnId,omitempty"`
	ToolCallID     string   `json:"toolCallId,omitempty"`
	TaskID         string   `json:"taskId,omitempty"`
	Mode           string   `json:"mode"`
	Status         string   `json:"status"`
	Executor       string   `json:"executor,omitempty"`
	CWD            string   `json:"cwd,omitempty"`
	WorktreeID     string   `json:"worktreeId,omitempty"`
	WorktreePath   string   `json:"worktreePath,omitempty"`
	CommandSummary string   `json:"commandSummary,omitempty"`
	PolicyMode     string   `json:"policyMode,omitempty"`
	PolicyProfile  string   `json:"policyProfile,omitempty"`
	PolicyRule     string   `json:"policyRule,omitempty"`
	Reason         string   `json:"reason,omitempty"`
	Error          string   `json:"error,omitempty"`
	AllowedPaths   []string `json:"allowedPaths,omitempty"`
	DeniedPaths    []string `json:"deniedPaths,omitempty"`
	NetworkAllowed bool     `json:"networkAllowed,omitempty"`
	NetworkReason  string   `json:"networkReason,omitempty"`
	CreatedAt      int64    `json:"createdAt"`
	CompletedAt    int64    `json:"completedAt,omitempty"`
}

type RuntimeSandboxDecisionListRequest struct {
	SessionID  string `json:"sessionId,omitempty"`
	TurnID     string `json:"turnId,omitempty"`
	ToolCallID string `json:"toolCallId,omitempty"`
	TaskID     string `json:"taskId,omitempty"`
}

type RuntimeSandboxDecisionResponse struct {
	Decision RuntimeSandboxDecision `json:"decision"`
}

type RuntimeSandboxDecisionsResponse struct {
	Decisions []RuntimeSandboxDecision `json:"decisions"`
}

type RuntimeEffectiveScopeResponse struct {
	Scope RuntimeEffectiveScope `json:"scope"`
}

type RuntimeAgentRoleDefinition struct {
	ID              string            `json:"id"`
	Name            string            `json:"name"`
	Title           string            `json:"title,omitempty"`
	Description     string            `json:"description,omitempty"`
	PromptSummary   string            `json:"promptSummary,omitempty"`
	AllowedTools    []string          `json:"allowedTools,omitempty"`
	CapabilityScope []string          `json:"capabilityScope,omitempty"`
	Model           string            `json:"model,omitempty"`
	Provider        string            `json:"provider,omitempty"`
	CWD             string            `json:"cwd,omitempty"`
	Worktree        string            `json:"worktree,omitempty"`
	Risk            string            `json:"risk,omitempty"`
	PolicyMetadata  map[string]string `json:"policyMetadata,omitempty"`
	Source          string            `json:"source,omitempty"`
	CreatedAt       int64             `json:"createdAt,omitempty"`
	UpdatedAt       int64             `json:"updatedAt,omitempty"`
}

type RuntimeAgentRolesResponse struct {
	Roles []RuntimeAgentRoleDefinition `json:"roles"`
}

type RuntimeAgentRoleResponse struct {
	Role RuntimeAgentRoleDefinition `json:"role"`
}

type RuntimeAgentTaskMessage struct {
	ID                string         `json:"id"`
	TaskID            string         `json:"taskId"`
	ParentTaskID      string         `json:"parentTaskId,omitempty"`
	ParentTurnID      string         `json:"parentTurnId,omitempty"`
	ParentSessionID   string         `json:"parentSessionId,omitempty"`
	ChildSessionID    string         `json:"childSessionId,omitempty"`
	Direction         string         `json:"direction"`
	Kind              string         `json:"kind"`
	Status            string         `json:"status"`
	Sequence          int64          `json:"sequence,omitempty"`
	ContentSummary    string         `json:"contentSummary,omitempty"`
	Payload           map[string]any `json:"payload,omitempty"`
	RelatedToolCallID string         `json:"relatedToolCallId,omitempty"`
	RelatedMessageID  string         `json:"relatedMessageId,omitempty"`
	ArtifactRefs      []string       `json:"artifactRefs,omitempty"`
	CreatedAt         int64          `json:"createdAt"`
	DeliveredAt       int64          `json:"deliveredAt,omitempty"`
	ProcessedAt       int64          `json:"processedAt,omitempty"`
	Error             string         `json:"error,omitempty"`
}

type RuntimeAgentTaskMessagesResponse struct {
	Messages []RuntimeAgentTaskMessage `json:"messages"`
}

type RuntimeAgentTaskMessageCreateRequest struct {
	Direction         string         `json:"direction"`
	Kind              string         `json:"kind"`
	ContentSummary    string         `json:"contentSummary,omitempty"`
	Payload           map[string]any `json:"payload,omitempty"`
	RelatedToolCallID string         `json:"relatedToolCallId,omitempty"`
	RelatedMessageID  string         `json:"relatedMessageId,omitempty"`
	ArtifactRefs      []string       `json:"artifactRefs,omitempty"`
	Status            string         `json:"status,omitempty"`
	Error             string         `json:"error,omitempty"`
}

type RuntimeAgentTaskMessageResponse struct {
	Message RuntimeAgentTaskMessage `json:"message"`
}

type RuntimeAgentTaskResult struct {
	TaskID              string   `json:"taskId"`
	Status              string   `json:"status"`
	Summary             string   `json:"summary,omitempty"`
	ErrorDetail         string   `json:"errorDetail,omitempty"`
	CancellationDetail  string   `json:"cancellationDetail,omitempty"`
	ArtifactRefs        []string `json:"artifactRefs,omitempty"`
	RelatedMessageRefs  []string `json:"relatedMessageRefs,omitempty"`
	RelatedToolCallRefs []string `json:"relatedToolCallRefs,omitempty"`
	CompactBoundaryRefs []string `json:"compactBoundaryRefs,omitempty"`
	CreatedAt           int64    `json:"createdAt"`
	UpdatedAt           int64    `json:"updatedAt"`
}

type RuntimeAgentTaskResultResponse struct {
	Result RuntimeAgentTaskResult `json:"result"`
}

type RuntimeTodo struct {
	Content    string `json:"content"`
	Status     string `json:"status"`
	ActiveForm string `json:"activeForm,omitempty"`
}

type RuntimeTodoSummary struct {
	SessionID  string        `json:"sessionId"`
	TurnID     string        `json:"turnId,omitempty"`
	Todos      []RuntimeTodo `json:"todos"`
	Pending    int           `json:"pending"`
	InProgress int           `json:"inProgress"`
	Completed  int           `json:"completed"`
	Total      int           `json:"total"`
	UpdatedAt  int64         `json:"updatedAt,omitempty"`
}

type RuntimeTodosResponse struct {
	Summary RuntimeTodoSummary `json:"summary"`
}

type RuntimeMessage struct {
	ID           string               `json:"id"`
	SessionID    string               `json:"sessionId"`
	Role         string               `json:"role"`
	Content      string               `json:"content"`
	Parts        []RuntimeMessagePart `json:"parts,omitempty"`
	Provider     string               `json:"provider,omitempty"`
	Model        string               `json:"model,omitempty"`
	CreatedAt    int64                `json:"createdAt"`
	UpdatedAt    int64                `json:"updatedAt"`
	Finished     bool                 `json:"finished"`
	FinishReason string               `json:"finishReason,omitempty"`
	Error        string               `json:"error,omitempty"`
}

type RuntimeMessagePart struct {
	Type             string `json:"type"`
	Text             string `json:"text,omitempty"`
	Thinking         string `json:"thinking,omitempty"`
	StartedAt        int64  `json:"startedAt,omitempty"`
	FinishedAt       int64  `json:"finishedAt,omitempty"`
	ToolCallID       string `json:"toolCallId,omitempty"`
	Name             string `json:"name,omitempty"`
	Input            string `json:"input,omitempty"`
	Finished         bool   `json:"finished,omitempty"`
	Content          string `json:"content,omitempty"`
	Data             string `json:"data,omitempty"`
	MIMEType         string `json:"mimeType,omitempty"`
	Metadata         string `json:"metadata,omitempty"`
	IsError          bool   `json:"isError,omitempty"`
	DeliveredToModel bool   `json:"deliveredToModel,omitempty"`
	DeliveredAtStep  int    `json:"deliveredAtStep,omitempty"`
	DeliveryReason   string `json:"deliveryReason,omitempty"`
	Reason           string `json:"reason,omitempty"`
	Message          string `json:"message,omitempty"`
	Details          string `json:"details,omitempty"`
	StoredPath       string `json:"storedPath,omitempty"`
	OriginalSize     int64  `json:"originalSize,omitempty"`
	TruncatedBy      string `json:"truncatedBy,omitempty"`
}

type RuntimeSession struct {
	ID               string       `json:"id"`
	Title            string       `json:"title"`
	ProjectID        string       `json:"projectId,omitempty"`
	Scope            string       `json:"scope,omitempty"`
	MessageCount     int64        `json:"messageCount"`
	PromptTokens     int64        `json:"promptTokens"`
	CompletionTokens int64        `json:"completionTokens"`
	Cost             float64      `json:"cost"`
	CreatedAt        int64        `json:"createdAt"`
	UpdatedAt        int64        `json:"updatedAt"`
	Active           bool         `json:"active"`
	Usage            RuntimeUsage `json:"usage"`
}

type RuntimeSessionsResponse struct {
	Sessions []RuntimeSession `json:"sessions"`
}

type RuntimeSessionResponse struct {
	Session RuntimeSession `json:"session"`
}

type RuntimeSessionCreateRequest struct {
	Title     string `json:"title"`
	ProjectID string `json:"projectId,omitempty"`
	Scope     string `json:"scope,omitempty"`
}

type RuntimeSessionUpdateRequest struct {
	SessionID string `json:"sessionId"`
	Title     string `json:"title"`
}

type RuntimeMessagesResponse struct {
	Messages []RuntimeMessage `json:"messages"`
}

type RuntimeSessionActivityResponse struct {
	SessionID   string                     `json:"sessionId"`
	Messages    []RuntimeMessage           `json:"messages"`
	Turns       []RuntimeTurn              `json:"turns"`
	ToolCalls   []RuntimeToolCall          `json:"toolCalls"`
	Permissions []RuntimePermissionRequest `json:"permissions"`
	Policy      RuntimePolicy              `json:"policy"`
}

type RuntimeActivityWindow struct {
	Limit         int    `json:"limit,omitempty"`
	Cursor        string `json:"cursor,omitempty"`
	FirstCursor   string `json:"firstCursor,omitempty"`
	LastCursor    string `json:"lastCursor,omitempty"`
	HasMoreBefore bool   `json:"hasMoreBefore,omitempty"`
	HasMoreAfter  bool   `json:"hasMoreAfter,omitempty"`
	EvidenceCount int    `json:"evidenceCount,omitempty"`
	FromStart     bool   `json:"fromStart,omitempty"`
	ToEnd         bool   `json:"toEnd,omitempty"`
}

type RuntimeSessionActivityWindowResponse struct {
	SessionID   string                     `json:"sessionId"`
	Messages    []RuntimeMessage           `json:"messages"`
	Turns       []RuntimeTurn              `json:"turns"`
	ToolCalls   []RuntimeToolCall          `json:"toolCalls"`
	Permissions []RuntimePermissionRequest `json:"permissions"`
	Events      []RuntimeEvent             `json:"events,omitempty"`
	Policy      RuntimePolicy              `json:"policy"`
	Window      RuntimeActivityWindow      `json:"window"`
}

type RuntimeTurnActivityResponse struct {
	SessionID   string                     `json:"sessionId"`
	TurnID      string                     `json:"turnId"`
	Messages    []RuntimeMessage           `json:"messages"`
	Turns       []RuntimeTurn              `json:"turns"`
	ToolCalls   []RuntimeToolCall          `json:"toolCalls"`
	Permissions []RuntimePermissionRequest `json:"permissions"`
	Events      []RuntimeEvent             `json:"events,omitempty"`
	Policy      RuntimePolicy              `json:"policy"`
}

type RuntimePermissionRequest struct {
	ID                   string `json:"id"`
	SessionID            string `json:"sessionId"`
	TurnID               string `json:"turnId,omitempty"`
	ToolCallID           string `json:"toolCallId"`
	ToolName             string `json:"toolName"`
	Description          string `json:"description,omitempty"`
	Action               string `json:"action"`
	Params               any    `json:"params,omitempty"`
	Path                 string `json:"path,omitempty"`
	Target               string `json:"target,omitempty"`
	Risk                 string `json:"risk,omitempty"`
	PolicyMode           string `json:"policyMode,omitempty"`
	PolicyReason         string `json:"policyReason,omitempty"`
	PolicyProfile        string `json:"policyProfile,omitempty"`
	PolicyHeadless       bool   `json:"policyHeadless,omitempty"`
	PolicyHeadlessReason string `json:"policyHeadlessReason,omitempty"`
	PolicyRuleID         string `json:"policyRuleId,omitempty"`
	PolicyRuleSource     string `json:"policyRuleSource,omitempty"`
	PolicyScopeKind      string `json:"policyScopeKind,omitempty"`
	PolicyScopeValue     string `json:"policyScopeValue,omitempty"`
	PolicyTargetSummary  string `json:"policyTargetSummary,omitempty"`
	Decision             string `json:"decision,omitempty"`
	Reason               string `json:"reason,omitempty"`
	Status               string `json:"status,omitempty"`
	CreatedAt            int64  `json:"createdAt"`
	DecidedAt            int64  `json:"decidedAt,omitempty"`
}

type RuntimePermissionsResponse struct {
	Permissions []RuntimePermissionRequest `json:"permissions"`
}

type RuntimePermissionDecision struct {
	PermissionID string `json:"permissionId"`
	Action       string `json:"action"`
}

type RuntimeMCPRequest struct {
	ID                   string `json:"id"`
	Kind                 string `json:"kind"`
	Server               string `json:"server"`
	CapabilityID         string `json:"capabilityId,omitempty"`
	SessionID            string `json:"sessionId,omitempty"`
	TurnID               string `json:"turnId,omitempty"`
	Status               string `json:"status"`
	Prompt               string `json:"prompt,omitempty"`
	Description          string `json:"description,omitempty"`
	ResponseSummary      string `json:"responseSummary,omitempty"`
	PolicyMode           string `json:"policyMode,omitempty"`
	PolicyProfile        string `json:"policyProfile,omitempty"`
	PolicyDecision       string `json:"policyDecision,omitempty"`
	PolicyReason         string `json:"policyReason,omitempty"`
	PolicyRisk           string `json:"policyRisk,omitempty"`
	PolicyRuleID         string `json:"policyRuleId,omitempty"`
	PolicyRuleSource     string `json:"policyRuleSource,omitempty"`
	PolicyScopeKind      string `json:"policyScopeKind,omitempty"`
	PolicyScopeValue     string `json:"policyScopeValue,omitempty"`
	PolicyTargetSummary  string `json:"policyTargetSummary,omitempty"`
	PolicyHeadless       bool   `json:"policyHeadless,omitempty"`
	PolicyHeadlessReason string `json:"policyHeadlessReason,omitempty"`
	CreatedAt            int64  `json:"createdAt"`
	UpdatedAt            int64  `json:"updatedAt"`
	ExpiresAt            int64  `json:"expiresAt,omitempty"`
	CompletedAt          int64  `json:"completedAt,omitempty"`
	Error                string `json:"error,omitempty"`
	Redacted             bool   `json:"redacted"`
}

type RuntimeMCPRequestsResponse struct {
	Requests []RuntimeMCPRequest `json:"requests"`
}

type RuntimeMCPRequestResponse struct {
	Request RuntimeMCPRequest           `json:"request"`
	Action  *RuntimeWriteActionMetadata `json:"action,omitempty"`
}

type RuntimeMCPRequestListRequest struct {
	Kind   string `json:"kind,omitempty"`
	Status string `json:"status,omitempty"`
	Server string `json:"server,omitempty"`
}

type RuntimeMCPRequestDecision struct {
	RequestID       string `json:"requestId"`
	Action          string `json:"action"`
	ResponseSummary string `json:"responseSummary,omitempty"`
	Error           string `json:"error,omitempty"`
}

type RuntimePolicy struct {
	Mode        string                    `json:"mode"`
	Modes       []string                  `json:"modes"`
	Profile     string                    `json:"profile,omitempty"`
	Rules       []RuntimePolicyRule       `json:"rules,omitempty"`
	Diagnostics []RuntimePolicyDiagnostic `json:"diagnostics,omitempty"`
	Description string                    `json:"description,omitempty"`
	UpdatedAt   int64                     `json:"updatedAt,omitempty"`
}

type RuntimePolicyResponse struct {
	Policy RuntimePolicy `json:"policy"`
}

type RuntimePolicyUpdateRequest struct {
	Mode    string              `json:"mode"`
	Profile string              `json:"profile,omitempty"`
	Rules   []RuntimePolicyRule `json:"rules,omitempty"`
}

type RuntimePolicyRule struct {
	ID            string `json:"id"`
	Decision      string `json:"decision"`
	Source        string `json:"source,omitempty"`
	Reason        string `json:"reason,omitempty"`
	Tool          string `json:"tool,omitempty"`
	CapabilityID  string `json:"capabilityId,omitempty"`
	BuiltinTool   string `json:"builtinTool,omitempty"`
	MCPServer     string `json:"mcpServer,omitempty"`
	MCPTool       string `json:"mcpTool,omitempty"`
	MCPResource   string `json:"mcpResource,omitempty"`
	MCPPrompt     string `json:"mcpPrompt,omitempty"`
	Skill         string `json:"skill,omitempty"`
	Subagent      string `json:"subagent,omitempty"`
	TaskScope     string `json:"taskScope,omitempty"`
	CWDPrefix     string `json:"cwdPrefix,omitempty"`
	PathPrefix    string `json:"pathPrefix,omitempty"`
	ShellPrefix   string `json:"shellPrefix,omitempty"`
	ShellRegex    string `json:"shellRegex,omitempty"`
	PolicyMode    string `json:"policyMode,omitempty"`
	PolicyProfile string `json:"policyProfile,omitempty"`
	ScopeKind     string `json:"scopeKind,omitempty"`
	ScopeValue    string `json:"scopeValue,omitempty"`
	Precedence    int    `json:"precedence,omitempty"`
}

type RuntimePolicyDiagnostic struct {
	RuleID string `json:"ruleId,omitempty"`
	Level  string `json:"level"`
	Reason string `json:"reason"`
}

type RuntimeRequests struct {
	ActiveRequestID  string `json:"activeRequestId,omitempty"`
	ActiveStartedAt  int64  `json:"activeStartedAt,omitempty"`
	ActiveDurationMS int64  `json:"activeDurationMs,omitempty"`
	SessionRequestID string `json:"sessionRequestId,omitempty"`
	SessionStartedAt int64  `json:"sessionStartedAt,omitempty"`
	SessionBusy      bool   `json:"sessionBusy,omitempty"`
	Running          int    `json:"running"`
}

type RuntimeUsage struct {
	PromptTokens     int64   `json:"promptTokens"`
	CompletionTokens int64   `json:"completionTokens"`
	TotalTokens      int64   `json:"totalTokens"`
	Cost             float64 `json:"cost"`
}

type RuntimeEventStats struct {
	LastEventAt      int64 `json:"lastEventAt"`
	MessageEvents    int64 `json:"messageEvents"`
	SessionEvents    int64 `json:"sessionEvents"`
	OtherEvents      int64 `json:"otherEvents"`
	AssistantEvents  int64 `json:"assistantEvents"`
	PermissionEvents int64 `json:"permissionEvents"`
}

type RuntimeEventsResponse struct {
	Events           []RuntimeEvent `json:"events"`
	SnapshotRequired bool           `json:"snapshot_required,omitempty"`
	FirstSequence    int64          `json:"first_sequence,omitempty"`
	LastSequence     int64          `json:"last_sequence,omitempty"`
}

type RuntimeReplayExportRequest struct {
	SessionID string `json:"sessionId,omitempty"`
	TurnID    string `json:"turnId,omitempty"`
	After     int64  `json:"after,omitempty"`
}

type RuntimeReplayExportResponse struct {
	SessionID        string                     `json:"sessionId,omitempty"`
	TurnID           string                     `json:"turnId,omitempty"`
	GeneratedAt      string                     `json:"generatedAt"`
	Source           string                     `json:"source"`
	SnapshotRequired bool                       `json:"snapshotRequired,omitempty"`
	FirstSequence    int64                      `json:"firstSequence,omitempty"`
	LastSequence     int64                      `json:"lastSequence,omitempty"`
	Events           []RuntimeEvent             `json:"events"`
	Audit            []RuntimeAuditEvent        `json:"audit"`
	Summary          RuntimeReplayExportSummary `json:"summary"`
}

type RuntimeReplayExportSummary struct {
	CompactBoundaries  []RuntimeCompactBoundary      `json:"compactBoundaries,omitempty"`
	Budget             *RuntimeBudgetReport          `json:"budget,omitempty"`
	Worktrees          []RuntimeWorktree             `json:"worktrees,omitempty"`
	ToolSearches       []RuntimeReplayToolSearch     `json:"toolSearches,omitempty"`
	ToolDiscovery      RuntimeReplayToolDiscovery    `json:"toolDiscovery,omitempty"`
	Capabilities       RuntimeReplayLifecycle        `json:"capabilities,omitempty"`
	Skills             RuntimeReplayLifecycle        `json:"skills,omitempty"`
	MCP                RuntimeReplayLifecycle        `json:"mcp,omitempty"`
	MCPRequests        []RuntimeReplayMCPRequest     `json:"mcpRequests,omitempty"`
	AgentTaskMessages  []RuntimeAgentTaskMessage     `json:"agentTaskMessages,omitempty"`
	AgentTaskResults   []RuntimeAgentTaskResult      `json:"agentTaskResults,omitempty"`
	AgentTaskArtifacts []string                      `json:"agentTaskArtifacts,omitempty"`
	OutputRefs         []RuntimeRef                  `json:"outputRefs,omitempty"`
	ArtifactRefs       []RuntimeRef                  `json:"artifactRefs,omitempty"`
	CompactOutputRefs  []RuntimeRef                  `json:"compactOutputRefs,omitempty"`
	TaskArtifactRefs   []RuntimeRef                  `json:"taskArtifactRefs,omitempty"`
	PolicyDecisions    []RuntimeReplayPolicyDecision `json:"policyDecisions,omitempty"`
	SandboxDecisions   []RuntimeSandboxDecision      `json:"sandboxDecisions,omitempty"`
	Hooks              []RuntimeHookExecution        `json:"hooks,omitempty"`
	PermissionEvents   []RuntimeReplayPermission     `json:"permissionEvents,omitempty"`
	ReadFiles          []RuntimeReadFileState        `json:"readFiles,omitempty"`
	ToolCalls          []RuntimeToolCall             `json:"toolCalls,omitempty"`
	Recovery           RuntimeReplayRecovery         `json:"recovery,omitempty"`
	EventCounts        map[string]int                `json:"eventCounts,omitempty"`
	AuditCounts        map[string]int                `json:"auditCounts,omitempty"`
	Redacted           bool                          `json:"redacted"`
}

type RuntimeReplayLifecycle struct {
	Started  []string `json:"started,omitempty"`
	Allowed  []string `json:"allowed,omitempty"`
	Denied   []string `json:"denied,omitempty"`
	Loaded   []string `json:"loaded,omitempty"`
	Failed   []string `json:"failed,omitempty"`
	Disabled []string `json:"disabled,omitempty"`
	Updated  []string `json:"updated,omitempty"`
}

type RuntimeReplayToolSearch struct {
	Query        string                        `json:"query,omitempty"`
	Selected     []string                      `json:"selected,omitempty"`
	OmittedCount int                           `json:"omittedCount,omitempty"`
	BudgetImpact RuntimeToolSchemaBudgetImpact `json:"budgetImpact,omitempty"`
	Guardrail    string                        `json:"guardrail,omitempty"`
	Reason       string                        `json:"reason,omitempty"`
}

type RuntimeReplayToolDiscovery struct {
	Selected         []string                      `json:"selected,omitempty"`
	Omitted          []string                      `json:"omitted,omitempty"`
	Denied           []string                      `json:"denied,omitempty"`
	GuardrailReasons []string                      `json:"guardrailReasons,omitempty"`
	BudgetImpact     RuntimeToolSchemaBudgetImpact `json:"budgetImpact,omitempty"`
}

type RuntimeReplayPolicyDecision struct {
	ToolCallID        string `json:"toolCallId,omitempty"`
	ToolName          string `json:"toolName,omitempty"`
	Decision          string `json:"decision,omitempty"`
	Risk              string `json:"risk,omitempty"`
	Reason            string `json:"reason,omitempty"`
	Mode              string `json:"mode,omitempty"`
	Profile           string `json:"profile,omitempty"`
	MatchedRuleID     string `json:"matchedRuleId,omitempty"`
	MatchedRuleSource string `json:"matchedRuleSource,omitempty"`
	ScopeKind         string `json:"scopeKind,omitempty"`
	ScopeValue        string `json:"scopeValue,omitempty"`
	ShellRisk         string `json:"shellRisk,omitempty"`
	ShellReason       string `json:"shellReason,omitempty"`
	Headless          bool   `json:"headless,omitempty"`
	HeadlessReason    string `json:"headlessReason,omitempty"`
}

type RuntimeReplayPermission struct {
	PermissionID string `json:"permissionId,omitempty"`
	ToolCallID   string `json:"toolCallId,omitempty"`
	ToolName     string `json:"toolName,omitempty"`
	Action       string `json:"action,omitempty"`
	Decision     string `json:"decision,omitempty"`
	Status       string `json:"status,omitempty"`
	Risk         string `json:"risk,omitempty"`
	Reason       string `json:"reason,omitempty"`
}

type RuntimeReplayMCPRequest struct {
	RequestID      string `json:"requestId,omitempty"`
	Kind           string `json:"kind,omitempty"`
	Server         string `json:"server,omitempty"`
	CapabilityID   string `json:"capabilityId,omitempty"`
	SessionID      string `json:"sessionId,omitempty"`
	TurnID         string `json:"turnId,omitempty"`
	Status         string `json:"status,omitempty"`
	Decision       string `json:"decision,omitempty"`
	Error          string `json:"error,omitempty"`
	PolicyDecision string `json:"policyDecision,omitempty"`
	PolicyMode     string `json:"policyMode,omitempty"`
	PolicyProfile  string `json:"policyProfile,omitempty"`
	PolicyReason   string `json:"policyReason,omitempty"`
	Redacted       bool   `json:"redacted"`
}

type RuntimeReplayRecovery struct {
	SnapshotRequired   bool  `json:"snapshotRequired,omitempty"`
	PendingPermissions int   `json:"pendingPermissions,omitempty"`
	PendingMCPRequests int   `json:"pendingMcpRequests,omitempty"`
	ActiveTurns        int   `json:"activeTurns,omitempty"`
	InterruptedTurns   int   `json:"interruptedTurns,omitempty"`
	LastEventSequence  int64 `json:"lastEventSequence,omitempty"`
}

type RuntimeCompactBoundary struct {
	ID             string                      `json:"id"`
	SessionID      string                      `json:"sessionId"`
	TurnID         string                      `json:"turnId,omitempty"`
	Kind           string                      `json:"kind"`
	Trigger        string                      `json:"trigger"`
	Status         string                      `json:"status"`
	BudgetBefore   *RuntimeBudgetReport        `json:"budgetBefore,omitempty"`
	BudgetAfter    *RuntimeBudgetReport        `json:"budgetAfter,omitempty"`
	SummaryRef     string                      `json:"summaryRef,omitempty"`
	MessageRefs    []string                    `json:"messageRefs,omitempty"`
	ToolCallRefs   []RuntimeCompactToolCallRef `json:"toolCallRefs,omitempty"`
	ReinjectedRefs []RuntimeReinjectedRef      `json:"reinjectedRefs,omitempty"`
	Error          string                      `json:"error,omitempty"`
	CreatedAt      int64                       `json:"createdAt"`
	CompletedAt    int64                       `json:"completedAt,omitempty"`
}

type RuntimeCompactToolCallRef struct {
	ToolCallID      string `json:"toolCallId"`
	Name            string `json:"name,omitempty"`
	Ref             string `json:"ref,omitempty"`
	EstimatedTokens int    `json:"estimatedTokens,omitempty"`
	Replacement     string `json:"replacement,omitempty"`
	Preserved       bool   `json:"preserved,omitempty"`
	Reason          string `json:"reason,omitempty"`
}

type RuntimeReinjectedRef struct {
	ID             string `json:"id"`
	Kind           string `json:"kind"`
	Name           string `json:"name,omitempty"`
	Path           string `json:"path,omitempty"`
	URI            string `json:"uri,omitempty"`
	Ref            string `json:"ref,omitempty"`
	Status         string `json:"status"`
	Reason         string `json:"reason,omitempty"`
	Error          string `json:"error,omitempty"`
	ContentSummary string `json:"contentSummary,omitempty"`
	TokenEstimate  int    `json:"tokenEstimate,omitempty"`
}

type RuntimeCompactBoundariesResponse struct {
	Boundaries []RuntimeCompactBoundary `json:"boundaries"`
}

type RuntimeBudgetReport struct {
	SessionID            string              `json:"sessionId,omitempty"`
	TurnID               string              `json:"turnId,omitempty"`
	Model                string              `json:"model,omitempty"`
	ContextWindow        int                 `json:"contextWindow,omitempty"`
	InputBudget          RuntimeBudgetBucket `json:"inputBudget"`
	Messages             RuntimeBudgetBucket `json:"messages"`
	ContextSources       RuntimeBudgetBucket `json:"contextSources"`
	ToolSchemas          RuntimeBudgetBucket `json:"toolSchemas"`
	Skills               RuntimeBudgetBucket `json:"skills"`
	MCP                  RuntimeBudgetBucket `json:"mcp"`
	ToolOutputs          RuntimeBudgetBucket `json:"toolOutputs"`
	SelectedToolSchemas  RuntimeBudgetBucket `json:"selectedToolSchemas"`
	OmittedToolSchemas   RuntimeBudgetBucket `json:"omittedToolSchemas"`
	TotalEstimatedTokens int                 `json:"totalEstimatedTokens"`
	UpdatedAt            int64               `json:"updatedAt"`
}

type RuntimeBudgetBucket struct {
	Count           int `json:"count"`
	EstimatedTokens int `json:"estimatedTokens"`
}

type RuntimeEventsEndpointResponse struct {
	URL   string `json:"url"`
	Token string `json:"token,omitempty"`
}

type RuntimeRecoveryStatus struct {
	RuntimeStartedAt   string                     `json:"runtime_started_at"`
	LastEventSequence  int64                      `json:"last_event_sequence"`
	ActiveTurns        []RuntimeTurn              `json:"active_turns"`
	InterruptedTurns   []RuntimeTurn              `json:"interrupted_turns"`
	InterruptedTasks   []RuntimeAgentTask         `json:"interrupted_tasks,omitempty"`
	CompactBoundaries  []RuntimeCompactBoundary   `json:"compact_boundaries,omitempty"`
	Worktrees          []RuntimeWorktree          `json:"worktrees,omitempty"`
	HookExecutions     []RuntimeHookExecution     `json:"hook_executions,omitempty"`
	PendingPermissions []RuntimePermissionRequest `json:"pending_permissions"`
	PendingMCPRequests []RuntimeMCPRequest        `json:"pending_mcp_requests,omitempty"`
	SnapshotRequired   bool                       `json:"snapshot_required,omitempty"`
}

type RuntimeAuditTurnSummary struct {
	TurnID                   string                     `json:"turn_id"`
	SessionID                string                     `json:"session_id,omitempty"`
	Provider                 string                     `json:"provider,omitempty"`
	Model                    string                     `json:"model,omitempty"`
	PromptPreview            string                     `json:"prompt_preview,omitempty"`
	UsageBefore              RuntimeUsage               `json:"usage_before,omitempty"`
	UsageAfter               RuntimeUsage               `json:"usage_after,omitempty"`
	UsageDelta               RuntimeUsage               `json:"usage_delta,omitempty"`
	FinalStatus              string                     `json:"final_status,omitempty"`
	LatestAssistantMessageID string                     `json:"latest_assistant_id,omitempty"`
	ToolCalls                []RuntimeToolCall          `json:"tool_calls,omitempty"`
	Tasks                    []RuntimeAgentTask         `json:"tasks,omitempty"`
	Worktrees                []RuntimeWorktree          `json:"worktrees,omitempty"`
	SandboxDecisions         []RuntimeSandboxDecision   `json:"sandbox_decisions,omitempty"`
	Hooks                    []RuntimeHookExecution     `json:"hooks,omitempty"`
	TaskMessages             []RuntimeAgentTaskMessage  `json:"task_messages,omitempty"`
	TaskResults              []RuntimeAgentTaskResult   `json:"task_results,omitempty"`
	Permissions              []map[string]any           `json:"permissions,omitempty"`
	Skills                   *RuntimeTurnSkillSummary   `json:"skills,omitempty"`
	Context                  *RuntimeTurnContextSummary `json:"context,omitempty"`
	Budget                   *RuntimeBudgetReport       `json:"budget,omitempty"`
	Compact                  []RuntimeCompactBoundary   `json:"compact,omitempty"`
	Errors                   []string                   `json:"errors,omitempty"`
	StartedAt                int64                      `json:"started_at,omitempty"`
	FinishedAt               int64                      `json:"finished_at,omitempty"`
	CreatedAt                string                     `json:"created_at,omitempty"`
	UpdatedAt                string                     `json:"updated_at,omitempty"`
}

type RuntimeContextSource struct {
	ID             string   `json:"id"`
	Kind           string   `json:"kind"`
	Name           string   `json:"name"`
	Path           string   `json:"path,omitempty"`
	URI            string   `json:"uri,omitempty"`
	Scope          string   `json:"scope,omitempty"`
	Enabled        bool     `json:"enabled"`
	State          string   `json:"state"`
	Reason         string   `json:"reason,omitempty"`
	Diagnostics    string   `json:"diagnostics,omitempty"`
	Error          string   `json:"error,omitempty"`
	ContentSummary string   `json:"content_summary,omitempty"`
	TokenEstimate  int      `json:"token_estimate,omitempty"`
	LoadedAt       string   `json:"loaded_at,omitempty"`
	Provenance     string   `json:"provenance,omitempty"`
	ParentID       string   `json:"parent_id,omitempty"`
	RuleGlobs      []string `json:"rule_globs,omitempty"`
	SizeBytes      int64    `json:"size_bytes,omitempty"`
	MTimeUnix      int64    `json:"mtime_unix,omitempty"`
	ContentHash    string   `json:"content_hash,omitempty"`
}

type RuntimeContextSourcesResponse struct {
	Sources []RuntimeContextSource `json:"sources"`
}

type RuntimeReadFileState struct {
	SessionID     string `json:"sessionId"`
	TurnID        string `json:"turnId,omitempty"`
	ToolCallID    string `json:"toolCallId,omitempty"`
	Path          string `json:"path"`
	ReadAt        int64  `json:"readAt"`
	SizeBytes     int64  `json:"sizeBytes,omitempty"`
	ContentHash   string `json:"contentHash,omitempty"`
	MTimeUnix     int64  `json:"mtimeUnix,omitempty"`
	Offset        int64  `json:"offset,omitempty"`
	Limit         int64  `json:"limit,omitempty"`
	Partial       bool   `json:"partial,omitempty"`
	TokenEstimate int    `json:"tokenEstimate,omitempty"`
	State         string `json:"state"`
	Reason        string `json:"reason,omitempty"`
	Diagnostics   string `json:"diagnostics,omitempty"`
}

type RuntimeReadFilesResponse struct {
	Files []RuntimeReadFileState `json:"files"`
}

type RuntimeTurnContextSummary struct {
	AvailableCount int                    `json:"available_count"`
	Available      []RuntimeContextSource `json:"available,omitempty"`
	Loaded         []RuntimeContextSource `json:"loaded,omitempty"`
	Injected       []RuntimeContextSource `json:"injected,omitempty"`
	Skipped        []RuntimeContextSource `json:"skipped,omitempty"`
	Failed         []RuntimeContextSource `json:"failed,omitempty"`
	TokenEstimate  int                    `json:"token_estimate,omitempty"`
}

type RuntimeAuditResponse struct {
	Summary RuntimeAuditTurnSummary `json:"summary,omitempty"`
	Events  []RuntimeAuditEvent     `json:"events"`
}

type RuntimeSkill struct {
	Name               string                         `json:"name"`
	Description        string                         `json:"description,omitempty"`
	Builtin            bool                           `json:"builtin"`
	Enabled            bool                           `json:"enabled"`
	Path               string                         `json:"path,omitempty"`
	SkillFilePath      string                         `json:"skill_file_path,omitempty"`
	State              string                         `json:"state"`
	Diagnostics        string                         `json:"diagnostics,omitempty"`
	Error              string                         `json:"error,omitempty"`
	Reason             string                         `json:"reason,omitempty"`
	AllowedTools       []string                       `json:"allowed_tools,omitempty"`
	Activation         RuntimeSkillActivationMetadata `json:"activation,omitempty"`
	ActivationMetadata RuntimeSkillActivationMetadata `json:"activation_metadata,omitempty"`
	Metadata           map[string]string              `json:"metadata,omitempty"`
	CapabilityID       string                         `json:"capability_id,omitempty"`
	PolicyMode         string                         `json:"policy_mode,omitempty"`
	PolicyRisk         string                         `json:"policy_risk,omitempty"`
	PolicyReason       string                         `json:"policy_reason,omitempty"`
}

type RuntimeSkillsResponse struct {
	Skills []RuntimeSkill `json:"skills"`
}

type RuntimePlugin struct {
	ID            string   `json:"id"`
	Name          string   `json:"name"`
	Description   string   `json:"description,omitempty"`
	Category      string   `json:"category"`
	Source        string   `json:"source"`
	Kind          string   `json:"kind"`
	Icon          string   `json:"icon,omitempty"`
	Enabled       bool     `json:"enabled"`
	State         string   `json:"state"`
	Diagnostics   string   `json:"diagnostics,omitempty"`
	Reason        string   `json:"reason,omitempty"`
	Error         string   `json:"error,omitempty"`
	Skills        []string `json:"skills,omitempty"`
	MCPServers    []string `json:"mcp_servers,omitempty"`
	ToolCount     int      `json:"tool_count,omitempty"`
	ResourceCount int      `json:"resource_count,omitempty"`
	PromptCount   int      `json:"prompt_count,omitempty"`
}

type RuntimePluginsResponse struct {
	Plugins []RuntimePlugin `json:"plugins"`
}

type RuntimeSkillActivationMetadata struct {
	Available bool   `json:"available"`
	Included  bool   `json:"included"`
	Reason    string `json:"reason,omitempty"`
}

type RuntimeTurnSkillSummary struct {
	AvailableCount int                    `json:"available_count"`
	Available      []RuntimeSkillTurnItem `json:"available,omitempty"`
	Activated      []RuntimeSkillTurnItem `json:"activated,omitempty"`
	Excluded       []RuntimeSkillTurnItem `json:"excluded,omitempty"`
	Failed         []RuntimeSkillTurnItem `json:"failed,omitempty"`
	PolicyMode     string                 `json:"policy_mode,omitempty"`
	PolicyRisk     string                 `json:"policy_risk,omitempty"`
	PolicyReason   string                 `json:"policy_reason,omitempty"`
	SourcePaths    []string               `json:"source_paths,omitempty"`
}

type RuntimeSkillTurnItem struct {
	Name          string   `json:"name"`
	CapabilityID  string   `json:"capability_id,omitempty"`
	Builtin       bool     `json:"builtin"`
	Path          string   `json:"path,omitempty"`
	SkillFilePath string   `json:"skill_file_path,omitempty"`
	State         string   `json:"state,omitempty"`
	Reason        string   `json:"reason,omitempty"`
	Error         string   `json:"error,omitempty"`
	AllowedTools  []string `json:"allowed_tools,omitempty"`
}

type RuntimeSkillCreateRequest struct {
	Name         string `json:"name"`
	Description  string `json:"description"`
	Instructions string `json:"instructions"`
	Directory    string `json:"directory,omitempty"`
}

type RuntimeSkillPathRequest struct {
	Path string `json:"path"`
}

type RuntimeSkillToggleRequest struct {
	Name    string `json:"name"`
	Enabled bool   `json:"enabled"`
}

type RuntimeMCPCounts struct {
	Tools     int `json:"tools"`
	Prompts   int `json:"prompts"`
	Resources int `json:"resources"`
}

type RuntimeMCPServer struct {
	Name          string            `json:"name"`
	Type          string            `json:"type"`
	URL           string            `json:"url,omitempty"`
	Command       string            `json:"command,omitempty"`
	Args          []string          `json:"args,omitempty"`
	Disabled      bool              `json:"disabled"`
	State         string            `json:"state"`
	Counts        RuntimeMCPCounts  `json:"counts"`
	Diagnostics   string            `json:"diagnostics,omitempty"`
	Reason        string            `json:"reason,omitempty"`
	Error         string            `json:"error,omitempty"`
	Env           map[string]string `json:"env,omitempty"`
	Headers       map[string]string `json:"headers,omitempty"`
	EnabledTools  []string          `json:"enabled_tools,omitempty"`
	DisabledTools []string          `json:"disabled_tools,omitempty"`
}

type RuntimeMCPServersResponse struct {
	Servers []RuntimeMCPServer `json:"servers"`
}

type RuntimeMCPServerConfigRequest struct {
	Name          string            `json:"name"`
	Type          string            `json:"type"`
	URL           string            `json:"url,omitempty"`
	Command       string            `json:"command,omitempty"`
	Args          []string          `json:"args,omitempty"`
	Disabled      bool              `json:"disabled"`
	EnabledTools  []string          `json:"enabled_tools,omitempty"`
	DisabledTools []string          `json:"disabled_tools,omitempty"`
	Env           map[string]string `json:"env,omitempty"`
	Headers       map[string]string `json:"headers,omitempty"`
}

type RuntimeMCPServerToggleRequest struct {
	Name    string `json:"name"`
	Enabled bool   `json:"enabled"`
}

type RuntimeMCPTool struct {
	Server      string `json:"server"`
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
	Enabled     bool   `json:"enabled"`
	InputSchema any    `json:"input_schema,omitempty"`
}

type RuntimeMCPToolsResponse struct {
	Tools []RuntimeMCPTool `json:"tools"`
}

type RuntimeMCPToolToggleRequest struct {
	Server  string `json:"server"`
	Tool    string `json:"tool"`
	Enabled bool   `json:"enabled"`
}

type RuntimeMCPResource struct {
	Server      string `json:"server"`
	URI         string `json:"uri"`
	Name        string `json:"name,omitempty"`
	Description string `json:"description,omitempty"`
	MIMEType    string `json:"mime_type,omitempty"`
}

type RuntimeMCPResourcesResponse struct {
	Resources []RuntimeMCPResource `json:"resources"`
}

type RuntimeMCPPrompt struct {
	Server      string `json:"server"`
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
}

type RuntimeMCPPromptsResponse struct {
	Prompts []RuntimeMCPPrompt `json:"prompts"`
}

type RuntimeCapability struct {
	ID            string `json:"id"`
	Kind          string `json:"kind"`
	Name          string `json:"name"`
	Source        string `json:"source,omitempty"`
	Enabled       bool   `json:"enabled"`
	Risk          string `json:"risk"`
	Description   string `json:"description,omitempty"`
	State         string `json:"state"`
	Diagnostics   string `json:"diagnostics,omitempty"`
	Error         string `json:"error,omitempty"`
	Reason        string `json:"reason,omitempty"`
	CapabilityID  string `json:"capabilityId,omitempty"`
	SchemaDigest  string `json:"schemaDigest,omitempty"`
	SchemaSummary string `json:"schemaSummary,omitempty"`
	SearchText    string `json:"searchText,omitempty"`
}

type RuntimeCapabilitiesResponse struct {
	Capabilities []RuntimeCapability `json:"capabilities"`
}

type RuntimeCapabilityResponse struct {
	Capability RuntimeCapability `json:"capability"`
}

type RuntimeToolSearchRequest struct {
	Query      string `json:"query"`
	MaxResults int    `json:"maxResults,omitempty"`
	TurnID     string `json:"turnId,omitempty"`
	SessionID  string `json:"sessionId,omitempty"`
	ToolCallID string `json:"toolCallId,omitempty"`
	Source     string `json:"source,omitempty"`
}

type RuntimeToolSearchResult struct {
	ID            string `json:"id"`
	Kind          string `json:"kind"`
	Name          string `json:"name"`
	Source        string `json:"source,omitempty"`
	Description   string `json:"description,omitempty"`
	Risk          string `json:"risk,omitempty"`
	CapabilityID  string `json:"capabilityId,omitempty"`
	SchemaDigest  string `json:"schemaDigest,omitempty"`
	SchemaSummary string `json:"schemaSummary,omitempty"`
	State         string `json:"state,omitempty"`
	Score         int    `json:"score,omitempty"`
}

type RuntimeToolSearchOmission struct {
	ID     string `json:"id"`
	Kind   string `json:"kind,omitempty"`
	Name   string `json:"name,omitempty"`
	Source string `json:"source,omitempty"`
	Reason string `json:"reason"`
	Risk   string `json:"risk,omitempty"`
	State  string `json:"state,omitempty"`
}

type RuntimeToolSchemaBudgetImpact struct {
	Selected RuntimeBudgetBucket `json:"selected"`
	Omitted  RuntimeBudgetBucket `json:"omitted"`
}

type RuntimeToolSearchResponse struct {
	Query            string                        `json:"query"`
	Results          []RuntimeToolSearchResult     `json:"results"`
	Omitted          []RuntimeToolSearchOmission   `json:"omitted,omitempty"`
	Total            int                           `json:"total"`
	BudgetImpact     RuntimeToolSchemaBudgetImpact `json:"budgetImpact"`
	Guardrail        string                        `json:"guardrail,omitempty"`
	GuardrailError   string                        `json:"guardrailError,omitempty"`
	MaxResults       int                           `json:"maxResults,omitempty"`
	MaxResultsReason string                        `json:"maxResultsReason,omitempty"`
}

type RuntimeModelConfig struct {
	Protocol   string   `json:"protocol"`
	URL        string   `json:"url"`
	APIKey     string   `json:"apiKey,omitempty"`
	Model      string   `json:"model"`
	Proxy      string   `json:"proxy,omitempty"`
	Models     []string `json:"models,omitempty"`
	HasAPIKey  bool     `json:"hasApiKey"`
	ConfigPath string   `json:"configPath,omitempty"`
}

type RuntimeModelVerifyResponse struct {
	OK       bool     `json:"ok"`
	Protocol string   `json:"protocol"`
	Model    string   `json:"model"`
	Models   []string `json:"models,omitempty"`
	Error    string   `json:"error,omitempty"`
}

type RuntimeModelDiscoveryResponse struct {
	Protocol string   `json:"protocol"`
	Model    string   `json:"model,omitempty"`
	Models   []string `json:"models"`
	Error    string   `json:"error,omitempty"`
}
