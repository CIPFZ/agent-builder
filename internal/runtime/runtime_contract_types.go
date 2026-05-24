package runtime

type RuntimeStatus struct {
	Ready       bool              `json:"ready"`
	WorkspaceID string            `json:"workspaceId"`
	SessionID   string            `json:"sessionId"`
	WorkingDir  string            `json:"workingDir"`
	Model       string            `json:"model"`
	Provider    string            `json:"provider"`
	Busy        bool              `json:"busy"`
	Usage       RuntimeUsage      `json:"usage"`
	Events      RuntimeEventStats `json:"events"`
	Requests    RuntimeRequests   `json:"requests"`
}

type RuntimeModel struct {
	ID       string `json:"id"`
	Name     string `json:"name"`
	Provider string `json:"provider"`
	Selected bool   `json:"selected"`
}

type RuntimeModelsResponse struct {
	Models []RuntimeModel `json:"models"`
}

type RuntimeConfigResponse struct {
	Config RuntimeModelConfig `json:"config"`
}

type RuntimeChatRequest struct {
	Prompt    string `json:"prompt"`
	SessionID string `json:"sessionId,omitempty"`
}

type RuntimeChatResponse struct {
	RequestID string        `json:"requestId"`
	TurnID    string        `json:"turnId"`
	Status    RuntimeStatus `json:"status"`
}

type RuntimeTurn struct {
	ID                       string         `json:"id"`
	SessionID                string         `json:"sessionId"`
	Status                   string         `json:"status"`
	UserMessageID            string         `json:"userMessageId,omitempty"`
	LatestAssistantMessageID string         `json:"latestAssistantMessageId,omitempty"`
	StartedAt                int64          `json:"startedAt"`
	FinishedAt               int64          `json:"finishedAt,omitempty"`
	DurationMS               int64          `json:"durationMs,omitempty"`
	Provider                 string         `json:"provider,omitempty"`
	Model                    string         `json:"model,omitempty"`
	PromptPreview            string         `json:"promptPreview,omitempty"`
	UsageBefore              RuntimeUsage   `json:"usageBefore,omitempty"`
	UsageAfter               RuntimeUsage   `json:"usageAfter,omitempty"`
	UsageDelta               RuntimeUsage   `json:"usageDelta,omitempty"`
	LatestMessageID          string         `json:"latestMessageId,omitempty"`
	LatestAssistant          RuntimeMessage `json:"latestAssistant,omitempty"`
	Error                    string         `json:"error,omitempty"`
}

type RuntimeTurnResponse struct {
	Turn RuntimeTurn `json:"turn"`
}

type RuntimeTurnsResponse struct {
	Turns []RuntimeTurn `json:"turns"`
}

type RuntimeToolCall struct {
	ID                             string `json:"id"`
	SessionID                      string `json:"sessionId"`
	TurnID                         string `json:"turnId"`
	MessageID                      string `json:"messageId,omitempty"`
	Name                           string `json:"name"`
	Source                         string `json:"source"`
	CapabilityID                   string `json:"capabilityId,omitempty"`
	JobID                          string `json:"jobId,omitempty"`
	Command                        string `json:"command,omitempty"`
	Risk                           string `json:"risk,omitempty"`
	PolicyReason                   string `json:"policyReason,omitempty"`
	PolicyMode                     string `json:"policyMode,omitempty"`
	PolicyProfile                  string `json:"policyProfile,omitempty"`
	PolicyRuleID                   string `json:"policyRuleId,omitempty"`
	PolicyRuleSource               string `json:"policyRuleSource,omitempty"`
	PolicyScopeKind                string `json:"policyScopeKind,omitempty"`
	PolicyScopeValue               string `json:"policyScopeValue,omitempty"`
	PolicyTargetSummary            string `json:"policyTargetSummary,omitempty"`
	ShellRisk                      string `json:"shellRisk,omitempty"`
	ShellReason                    string `json:"shellReason,omitempty"`
	ExitCode                       int    `json:"exitCode,omitempty"`
	JobStatus                      string `json:"jobStatus,omitempty"`
	JobStartedAt                   int64  `json:"jobStartedAt,omitempty"`
	JobFinishedAt                  int64  `json:"jobFinishedAt,omitempty"`
	Status                         string `json:"status"`
	InputSummary                   string `json:"inputSummary,omitempty"`
	OutputSummary                  string `json:"outputSummary,omitempty"`
	ModelContent                   string `json:"modelContent,omitempty"`
	Structured                     string `json:"structuredOutput,omitempty"`
	Stdout                         string `json:"stdout,omitempty"`
	Stderr                         string `json:"stderr,omitempty"`
	IsError                        bool   `json:"isError,omitempty"`
	Compacted                      bool   `json:"compacted,omitempty"`
	CompactRef                     string `json:"compactRef,omitempty"`
	CompactBoundaryID              string `json:"compactBoundaryId,omitempty"`
	CompactOriginalEstimatedTokens int    `json:"compactOriginalEstimatedTokens,omitempty"`
	CompactedAt                    int64  `json:"compactedAt,omitempty"`
	StartedAt                      int64  `json:"startedAt"`
	FinishedAt                     int64  `json:"finishedAt,omitempty"`
	Error                          string `json:"error,omitempty"`
}

type RuntimeToolCallResponse struct {
	ToolCall RuntimeToolCall `json:"toolCall"`
}

type RuntimeToolCallsResponse struct {
	ToolCalls []RuntimeToolCall `json:"toolCalls"`
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
	Task     RuntimeAgentTask          `json:"task"`
	Messages []RuntimeAgentTaskMessage `json:"messages,omitempty"`
	Result   *RuntimeAgentTaskResult   `json:"result,omitempty"`
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
	ContentSummary    string         `json:"contentSummary,omitempty"`
	Payload           map[string]any `json:"payload,omitempty"`
	RelatedToolCallID string         `json:"relatedToolCallId,omitempty"`
	RelatedMessageID  string         `json:"relatedMessageId,omitempty"`
	ArtifactRefs      []string       `json:"artifactRefs,omitempty"`
	CreatedAt         int64          `json:"createdAt"`
	DeliveredAt       int64          `json:"deliveredAt,omitempty"`
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
	Type       string `json:"type"`
	Text       string `json:"text,omitempty"`
	Thinking   string `json:"thinking,omitempty"`
	StartedAt  int64  `json:"startedAt,omitempty"`
	FinishedAt int64  `json:"finishedAt,omitempty"`
	ToolCallID string `json:"toolCallId,omitempty"`
	Name       string `json:"name,omitempty"`
	Input      string `json:"input,omitempty"`
	Finished   bool   `json:"finished,omitempty"`
	Content    string `json:"content,omitempty"`
	Data       string `json:"data,omitempty"`
	MIMEType   string `json:"mimeType,omitempty"`
	Metadata   string `json:"metadata,omitempty"`
	IsError    bool   `json:"isError,omitempty"`
	Reason     string `json:"reason,omitempty"`
	Message    string `json:"message,omitempty"`
	Details    string `json:"details,omitempty"`
}

type RuntimeSession struct {
	ID               string       `json:"id"`
	Title            string       `json:"title"`
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

type RuntimeSessionUpdateRequest struct {
	SessionID string `json:"sessionId"`
	Title     string `json:"title"`
}

type RuntimeMessagesResponse struct {
	Messages []RuntimeMessage `json:"messages"`
}

type RuntimePermissionRequest struct {
	ID                  string `json:"id"`
	SessionID           string `json:"sessionId"`
	TurnID              string `json:"turnId,omitempty"`
	ToolCallID          string `json:"toolCallId"`
	ToolName            string `json:"toolName"`
	Description         string `json:"description,omitempty"`
	Action              string `json:"action"`
	Params              any    `json:"params,omitempty"`
	Path                string `json:"path,omitempty"`
	Target              string `json:"target,omitempty"`
	Risk                string `json:"risk,omitempty"`
	PolicyMode          string `json:"policyMode,omitempty"`
	PolicyReason        string `json:"policyReason,omitempty"`
	PolicyProfile       string `json:"policyProfile,omitempty"`
	PolicyRuleID        string `json:"policyRuleId,omitempty"`
	PolicyRuleSource    string `json:"policyRuleSource,omitempty"`
	PolicyScopeKind     string `json:"policyScopeKind,omitempty"`
	PolicyScopeValue    string `json:"policyScopeValue,omitempty"`
	PolicyTargetSummary string `json:"policyTargetSummary,omitempty"`
	Decision            string `json:"decision,omitempty"`
	Reason              string `json:"reason,omitempty"`
	Status              string `json:"status,omitempty"`
	CreatedAt           int64  `json:"createdAt"`
	DecidedAt           int64  `json:"decidedAt,omitempty"`
}

type RuntimePermissionsResponse struct {
	Permissions []RuntimePermissionRequest `json:"permissions"`
}

type RuntimePermissionDecision struct {
	PermissionID string `json:"permissionId"`
	Action       string `json:"action"`
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
	AgentTaskMessages  []RuntimeAgentTaskMessage     `json:"agentTaskMessages,omitempty"`
	AgentTaskResults   []RuntimeAgentTaskResult      `json:"agentTaskResults,omitempty"`
	AgentTaskArtifacts []string                      `json:"agentTaskArtifacts,omitempty"`
	PolicyDecisions    []RuntimeReplayPolicyDecision `json:"policyDecisions,omitempty"`
	PermissionEvents   []RuntimeReplayPermission     `json:"permissionEvents,omitempty"`
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

type RuntimeReplayRecovery struct {
	SnapshotRequired   bool  `json:"snapshotRequired,omitempty"`
	PendingPermissions int   `json:"pendingPermissions,omitempty"`
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
	PendingPermissions []RuntimePermissionRequest `json:"pending_permissions"`
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
	ID             string `json:"id"`
	Kind           string `json:"kind"`
	Name           string `json:"name"`
	Path           string `json:"path,omitempty"`
	URI            string `json:"uri,omitempty"`
	Scope          string `json:"scope,omitempty"`
	Enabled        bool   `json:"enabled"`
	State          string `json:"state"`
	Reason         string `json:"reason,omitempty"`
	Diagnostics    string `json:"diagnostics,omitempty"`
	Error          string `json:"error,omitempty"`
	ContentSummary string `json:"content_summary,omitempty"`
	TokenEstimate  int    `json:"token_estimate,omitempty"`
	LoadedAt       string `json:"loaded_at,omitempty"`
}

type RuntimeContextSourcesResponse struct {
	Sources []RuntimeContextSource `json:"sources"`
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
