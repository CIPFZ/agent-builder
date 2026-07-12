package runtime

import (
	"errors"
	"fmt"
	"strconv"
)

const (
	RuntimeConversationSchemaVersion = 2

	RuntimeConversationScopeFull   = "full"
	RuntimeConversationScopeWindow = "window"

	RuntimeConversationOperationUpsert = "upsert"
	RuntimeConversationOperationDelete = "delete"

	RuntimeConversationPhaseReasoning    = "reasoning"
	RuntimeConversationPhaseIntermediate = "intermediate"
	RuntimeConversationPhaseFinal        = "final"

	RuntimeConversationEntityTurn          = "turn"
	RuntimeConversationEntityMessage       = "message"
	RuntimeConversationEntityAssistantStep = "assistantStep"
	RuntimeConversationEntityToolCall      = "toolCall"
	RuntimeConversationEntityToolResult    = "toolResult"
	RuntimeConversationEntityPermission    = "permission"
	RuntimeConversationEntityTodoPlan      = "todoPlan"
	RuntimeConversationEntityAgentTask     = "agentTask"
	RuntimeConversationEntityNotice        = "notice"
)

// RuntimeConversationEntityMeta is shared by every canonical conversation
// entity. ActivitySequence is immutable; Revision increases on every update.
type RuntimeConversationEntityMeta struct {
	ID               string `json:"id"`
	SessionID        string `json:"sessionId"`
	TurnID           string `json:"turnId,omitempty"`
	ActivitySequence string `json:"activitySequence"`
	Revision         string `json:"revision"`
	CreatedAt        int64  `json:"createdAt"`
	UpdatedAt        int64  `json:"updatedAt"`
}

type RuntimeCanonicalConversationSnapshot struct {
	SchemaVersion  int                             `json:"schemaVersion"`
	SessionID      string                          `json:"sessionId"`
	Cursor         string                          `json:"cursor"`
	Scope          string                          `json:"scope"`
	Window         *RuntimeConversationWindow      `json:"window,omitempty"`
	Turns          []RuntimeCanonicalTurn          `json:"turns"`
	Messages       []RuntimeCanonicalMessage       `json:"messages"`
	AssistantSteps []RuntimeCanonicalAssistantStep `json:"assistantSteps"`
	ToolCalls      []RuntimeCanonicalToolCall      `json:"toolCalls"`
	ToolResults    []RuntimeCanonicalToolResult    `json:"toolResults"`
	Permissions    []RuntimeCanonicalPermission    `json:"permissions"`
	TodoPlans      []RuntimeCanonicalTodoPlan      `json:"todoPlans"`
	AgentTasks     []RuntimeCanonicalAgentTask     `json:"agentTasks"`
	Notices        []RuntimeCanonicalNotice        `json:"notices"`
}

type RuntimeCanonicalConversationSnapshotRequest struct {
	Scope  string `json:"scope,omitempty"`
	Limit  int    `json:"limit,omitempty"`
	Before string `json:"before,omitempty"`
	Around string `json:"around,omitempty"`
}

type RuntimeConversationSearchRequestV2 struct {
	Query string `json:"query"`
	Limit int    `json:"limit,omitempty"`
}
type RuntimeConversationSearchResultV2 struct {
	MessageID string `json:"messageId"`
	TurnID    string `json:"turnId"`
	Role      string `json:"role"`
	Snippet   string `json:"snippet"`
	CreatedAt int64  `json:"createdAt"`
}
type RuntimeConversationSearchResponseV2 struct {
	SchemaVersion int                                 `json:"schemaVersion"`
	SessionID     string                              `json:"sessionId"`
	Results       []RuntimeConversationSearchResultV2 `json:"results"`
}

type RuntimeCanonicalMessageContentResponseV2 struct {
	SchemaVersion int    `json:"schemaVersion"`
	SessionID     string `json:"sessionId"`
	MessageID     string `json:"messageId"`
	Content       string `json:"content"`
}

type RuntimeCanonicalConversationEventsRequestV2 struct {
	After          string `json:"after"`
	LimitRawEvents int    `json:"limitRawEvents,omitempty"`
}

type RuntimeCanonicalConversationEventsResponseV2 struct {
	SchemaVersion    int                                `json:"schemaVersion"`
	SessionID        string                             `json:"sessionId"`
	AfterCursor      string                             `json:"afterCursor"`
	Cursor           string                             `json:"cursor"`
	Events           []RuntimeConversationEntityEventV2 `json:"events"`
	SnapshotRequired bool                               `json:"snapshotRequired,omitempty"`
	Reason           string                             `json:"reason,omitempty"`
}

type RuntimeCanonicalConversationEventBatchV2 struct {
	SchemaVersion    int                                `json:"schemaVersion"`
	SessionID        string                             `json:"sessionId"`
	AfterCursor      string                             `json:"afterCursor"`
	Cursor           string                             `json:"cursor"`
	Events           []RuntimeConversationEntityEventV2 `json:"events"`
	SnapshotRequired bool                               `json:"snapshotRequired,omitempty"`
	Reason           string                             `json:"reason,omitempty"`
}

type RuntimeCanonicalConversationStreamStartRequestV2 struct {
	SessionID string `json:"sessionId"`
	StreamID  string `json:"streamId,omitempty"`
	After     string `json:"after"`
}
type RuntimeCanonicalConversationStreamStopRequestV2 struct {
	StreamID string `json:"streamId"`
}
type RuntimeCanonicalConversationStreamResponseV2 struct {
	StreamID  string `json:"streamId"`
	EventName string `json:"eventName"`
}
type RuntimeCanonicalConversationStreamMessageV2 struct {
	StreamID  string `json:"streamId"`
	Lifecycle string `json:"lifecycle,omitempty"`
	RuntimeCanonicalConversationEventBatchV2
}

type RuntimeConversationWindow struct {
	TurnIDs       []string `json:"turnIds,omitempty"`
	BeforeCursor  string   `json:"beforeCursor,omitempty"`
	AfterCursor   string   `json:"afterCursor,omitempty"`
	HasMoreBefore bool     `json:"hasMoreBefore,omitempty"`
}

type RuntimeCanonicalTurn struct {
	RuntimeConversationEntityMeta
	Status         string `json:"status"`
	UserMessageID  string `json:"userMessageId,omitempty"`
	FinalMessageID string `json:"finalMessageId,omitempty"`
	StartedAt      int64  `json:"startedAt,omitempty"`
	FinishedAt     int64  `json:"finishedAt,omitempty"`
	Error          string `json:"error,omitempty"`
}

type RuntimeCanonicalMessage struct {
	RuntimeConversationEntityMeta
	Role             string `json:"role"`
	Phase            string `json:"phase,omitempty"`
	AssistantStepID  string `json:"assistantStepId,omitempty"`
	Status           string `json:"status"`
	Content          string `json:"content,omitempty"`
	ContentLength    int    `json:"contentLength,omitempty"`
	ContentTruncated bool   `json:"contentTruncated,omitempty"`
	ClientRequestID  string `json:"clientRequestId,omitempty"`
	Error            string `json:"error,omitempty"`
}

type RuntimeCanonicalAssistantStep struct {
	RuntimeConversationEntityMeta
	MessageID  string `json:"messageId"`
	Index      int    `json:"index"`
	Status     string `json:"status"`
	StartedAt  int64  `json:"startedAt,omitempty"`
	FinishedAt int64  `json:"finishedAt,omitempty"`
}

type RuntimeCanonicalToolCall struct {
	RuntimeConversationEntityMeta
	MessageID        string   `json:"messageId,omitempty"`
	AssistantStepID  string   `json:"assistantStepId,omitempty"`
	ParentToolCallID string   `json:"parentToolCallId,omitempty"`
	RoundID          string   `json:"roundId,omitempty"`
	Name             string   `json:"name"`
	Source           string   `json:"source"`
	Kind             string   `json:"kind,omitempty"`
	Status           string   `json:"status"`
	InputJSON        string   `json:"inputJson,omitempty"`
	Command          string   `json:"command,omitempty"`
	Targets          []string `json:"targets,omitempty"`
	WorkingDir       string   `json:"workingDir,omitempty"`
	Risk             string   `json:"risk,omitempty"`
	ResultIDs        []string `json:"resultIds,omitempty"`
	StartedAt        int64    `json:"startedAt,omitempty"`
	FinishedAt       int64    `json:"finishedAt,omitempty"`
	ExitCode         *int     `json:"exitCode,omitempty"`
	Error            string   `json:"error,omitempty"`
}

type RuntimeCanonicalAgentTaskMessage struct {
	ID                string   `json:"id"`
	Direction         string   `json:"direction"`
	Kind              string   `json:"kind"`
	Status            string   `json:"status"`
	Sequence          int64    `json:"sequence,omitempty"`
	ContentSummary    string   `json:"contentSummary,omitempty"`
	RelatedToolCallID string   `json:"relatedToolCallId,omitempty"`
	RelatedMessageID  string   `json:"relatedMessageId,omitempty"`
	ArtifactRefs      []string `json:"artifactRefs,omitempty"`
	CreatedAt         int64    `json:"createdAt,omitempty"`
	DeliveredAt       int64    `json:"deliveredAt,omitempty"`
	ProcessedAt       int64    `json:"processedAt,omitempty"`
	Error             string   `json:"error,omitempty"`
}

type RuntimeCanonicalToolResult struct {
	RuntimeConversationEntityMeta
	ToolCallID       string   `json:"toolCallId"`
	Ordinal          int      `json:"ordinal"`
	Status           string   `json:"status"`
	ContentPreview   string   `json:"contentPreview,omitempty"`
	ErrorPreview     string   `json:"errorPreview,omitempty"`
	OutputRefs       []string `json:"outputRefs,omitempty"`
	ArtifactRefs     []string `json:"artifactRefs,omitempty"`
	DiffRefs         []string `json:"diffRefs,omitempty"`
	DeliveredToModel bool     `json:"deliveredToModel,omitempty"`
}

type RuntimeCanonicalPermission struct {
	RuntimeConversationEntityMeta
	ToolCallID          string `json:"toolCallId"`
	Status              string `json:"status"`
	Description         string `json:"description,omitempty"`
	Action              string `json:"action,omitempty"`
	Path                string `json:"path,omitempty"`
	Target              string `json:"target,omitempty"`
	Risk                string `json:"risk,omitempty"`
	PolicyMode          string `json:"policyMode,omitempty"`
	PolicyReason        string `json:"policyReason,omitempty"`
	PolicyRuleID        string `json:"policyRuleId,omitempty"`
	PolicyRuleSource    string `json:"policyRuleSource,omitempty"`
	PolicyScopeKind     string `json:"policyScopeKind,omitempty"`
	PolicyScopeValue    string `json:"policyScopeValue,omitempty"`
	PolicyTargetSummary string `json:"policyTargetSummary,omitempty"`
	Reason              string `json:"reason,omitempty"`
	Decision            string `json:"decision,omitempty"`
	RequestedAt         int64  `json:"requestedAt,omitempty"`
	DecidedAt           int64  `json:"decidedAt,omitempty"`
}

type RuntimeCanonicalTodoPlan struct {
	RuntimeConversationEntityMeta
	OwnerTurnID string                     `json:"ownerTurnId"`
	Status      string                     `json:"status"`
	Items       []RuntimeCanonicalTodoItem `json:"items"`
}

type RuntimeCanonicalTodoItem struct {
	ID         string `json:"id"`
	Order      int    `json:"order"`
	Status     string `json:"status"`
	Content    string `json:"content"`
	ActiveForm string `json:"activeForm,omitempty"`
}

type RuntimeCanonicalAgentTask struct {
	RuntimeConversationEntityMeta
	ParentToolCallID  string                             `json:"parentToolCallId,omitempty"`
	ParentTaskID      string                             `json:"parentTaskId,omitempty"`
	ChildSessionID    string                             `json:"childSessionId,omitempty"`
	TeamID            string                             `json:"teamId,omitempty"`
	TeamRole          string                             `json:"teamRole,omitempty"`
	Title             string                             `json:"title,omitempty"`
	Kind              string                             `json:"kind,omitempty"`
	Name              string                             `json:"name,omitempty"`
	PromptSummary     string                             `json:"promptSummary,omitempty"`
	Model             string                             `json:"model,omitempty"`
	Provider          string                             `json:"provider,omitempty"`
	AllowedTools      []string                           `json:"allowedTools,omitempty"`
	CapabilityScope   []string                           `json:"capabilityScope,omitempty"`
	CWD               string                             `json:"cwd,omitempty"`
	Worktree          string                             `json:"worktree,omitempty"`
	Status            string                             `json:"status"`
	Progress          int                                `json:"progress,omitempty"`
	ResultSummary     string                             `json:"resultSummary,omitempty"`
	ArtifactRefs      []string                           `json:"artifactRefs,omitempty"`
	Dependencies      []string                           `json:"dependencies,omitempty"`
	ResultRefs        []string                           `json:"resultRefs,omitempty"`
	OutputRefs        []string                           `json:"outputRefs,omitempty"`
	StartedAt         int64                              `json:"startedAt,omitempty"`
	FinishedAt        int64                              `json:"finishedAt,omitempty"`
	Error             string                             `json:"error,omitempty"`
	Messages          []RuntimeCanonicalAgentTaskMessage `json:"messages,omitempty"`
	MessageCount      int                                `json:"messageCount,omitempty"`
	MessagesTruncated bool                               `json:"messagesTruncated,omitempty"`
}

type RuntimeCanonicalNotice struct {
	RuntimeConversationEntityMeta
	Kind     string   `json:"kind"`
	Status   string   `json:"status,omitempty"`
	Summary  string   `json:"summary,omitempty"`
	Refs     []string `json:"refs,omitempty"`
	DataJSON string   `json:"dataJson,omitempty"`
}

// RuntimeConversationEntityEventV2 is deliberately typed: exactly one entity
// pointer is populated for an upsert. Delete events carry only identity and a
// tombstone reason.
type RuntimeConversationEntityEventV2 struct {
	SchemaVersion   int                            `json:"schemaVersion"`
	ID              string                         `json:"id"`
	SessionID       string                         `json:"sessionId"`
	TurnID          string                         `json:"turnId,omitempty"`
	Sequence        string                         `json:"sequence"`
	CreatedAt       int64                          `json:"createdAt"`
	EntityType      string                         `json:"entityType"`
	EntityID        string                         `json:"entityId"`
	Operation       string                         `json:"operation"`
	Revision        string                         `json:"revision"`
	TombstoneReason string                         `json:"tombstoneReason,omitempty"`
	Turn            *RuntimeCanonicalTurn          `json:"turn,omitempty"`
	Message         *RuntimeCanonicalMessage       `json:"message,omitempty"`
	AssistantStep   *RuntimeCanonicalAssistantStep `json:"assistantStep,omitempty"`
	ToolCall        *RuntimeCanonicalToolCall      `json:"toolCall,omitempty"`
	ToolResult      *RuntimeCanonicalToolResult    `json:"toolResult,omitempty"`
	Permission      *RuntimeCanonicalPermission    `json:"permission,omitempty"`
	TodoPlan        *RuntimeCanonicalTodoPlan      `json:"todoPlan,omitempty"`
	AgentTask       *RuntimeCanonicalAgentTask     `json:"agentTask,omitempty"`
	Notice          *RuntimeCanonicalNotice        `json:"notice,omitempty"`
}

func (s RuntimeCanonicalConversationSnapshot) Validate() error {
	if s.SchemaVersion != RuntimeConversationSchemaVersion {
		return fmt.Errorf("unsupported canonical conversation schema version %d", s.SchemaVersion)
	}
	if s.SessionID == "" || !validDecimal(s.Cursor) {
		return errors.New("canonical conversation snapshot requires session id and decimal cursor")
	}
	if s.Scope != RuntimeConversationScopeFull && s.Scope != RuntimeConversationScopeWindow {
		return fmt.Errorf("invalid canonical conversation scope %q", s.Scope)
	}
	if s.Scope == RuntimeConversationScopeWindow && s.Window == nil {
		return errors.New("window snapshot requires window metadata")
	}
	if s.Scope == RuntimeConversationScopeFull && s.Window != nil {
		return errors.New("full snapshot must not carry window metadata")
	}
	if s.Turns == nil || s.Messages == nil || s.AssistantSteps == nil || s.ToolCalls == nil || s.ToolResults == nil || s.Permissions == nil || s.TodoPlans == nil || s.AgentTasks == nil || s.Notices == nil {
		return errors.New("canonical conversation collections must be arrays, not null")
	}
	return nil
}

func (e RuntimeConversationEntityEventV2) Validate() error {
	if e.SchemaVersion != RuntimeConversationSchemaVersion || e.ID == "" || e.SessionID == "" || e.EntityID == "" || !validDecimal(e.Sequence) || !validDecimal(e.Revision) {
		return errors.New("canonical conversation event has an invalid envelope")
	}
	payloads := map[string]bool{
		RuntimeConversationEntityTurn: e.Turn != nil, RuntimeConversationEntityMessage: e.Message != nil,
		RuntimeConversationEntityAssistantStep: e.AssistantStep != nil, RuntimeConversationEntityToolCall: e.ToolCall != nil,
		RuntimeConversationEntityToolResult: e.ToolResult != nil, RuntimeConversationEntityPermission: e.Permission != nil,
		RuntimeConversationEntityTodoPlan: e.TodoPlan != nil, RuntimeConversationEntityAgentTask: e.AgentTask != nil,
		RuntimeConversationEntityNotice: e.Notice != nil,
	}
	if _, ok := payloads[e.EntityType]; !ok {
		return fmt.Errorf("invalid canonical conversation entity type %q", e.EntityType)
	}
	count := 0
	for _, present := range payloads {
		if present {
			count++
		}
	}
	if e.Operation == RuntimeConversationOperationDelete {
		if count != 0 {
			return errors.New("canonical delete event must not carry a payload")
		}
		return nil
	}
	if e.Operation != RuntimeConversationOperationUpsert || count != 1 || !payloads[e.EntityType] {
		return errors.New("canonical upsert event must carry exactly its typed payload")
	}
	meta := e.payloadMeta()
	if meta == nil || meta.ID != e.EntityID || meta.SessionID != e.SessionID || meta.Revision != e.Revision || (e.TurnID != "" && meta.TurnID != e.TurnID) {
		return errors.New("canonical event envelope does not match payload identity")
	}
	return nil
}

func (e RuntimeConversationEntityEventV2) payloadMeta() *RuntimeConversationEntityMeta {
	switch e.EntityType {
	case RuntimeConversationEntityTurn:
		if e.Turn != nil {
			return &e.Turn.RuntimeConversationEntityMeta
		}
	case RuntimeConversationEntityMessage:
		if e.Message != nil {
			return &e.Message.RuntimeConversationEntityMeta
		}
	case RuntimeConversationEntityAssistantStep:
		if e.AssistantStep != nil {
			return &e.AssistantStep.RuntimeConversationEntityMeta
		}
	case RuntimeConversationEntityToolCall:
		if e.ToolCall != nil {
			return &e.ToolCall.RuntimeConversationEntityMeta
		}
	case RuntimeConversationEntityToolResult:
		if e.ToolResult != nil {
			return &e.ToolResult.RuntimeConversationEntityMeta
		}
	case RuntimeConversationEntityPermission:
		if e.Permission != nil {
			return &e.Permission.RuntimeConversationEntityMeta
		}
	case RuntimeConversationEntityTodoPlan:
		if e.TodoPlan != nil {
			return &e.TodoPlan.RuntimeConversationEntityMeta
		}
	case RuntimeConversationEntityAgentTask:
		if e.AgentTask != nil {
			return &e.AgentTask.RuntimeConversationEntityMeta
		}
	case RuntimeConversationEntityNotice:
		if e.Notice != nil {
			return &e.Notice.RuntimeConversationEntityMeta
		}
	}
	return nil
}

func validDecimal(value string) bool {
	if value == "" {
		return false
	}
	parsed, err := strconv.ParseUint(value, 10, 64)
	return err == nil && strconv.FormatUint(parsed, 10) == value
}
