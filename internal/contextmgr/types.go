package contextmgr

import (
	"context"
	"time"

	"charm.land/fantasy"
	"github.com/CIPFZ/agent-builder/internal/message"
)

const (
	ProjectionStatusStarted   = "started"
	ProjectionStatusCompleted = "completed"
	ProjectionStatusFailed    = "failed"

	ProjectionSourceMainTurn = "main_turn"
)

type Manager interface {
	BuildModelInput(context.Context, BuildInputRequest) (BuildInputResult, error)
	ReactiveCompact(context.Context, ReactiveCompactRequest) (ReactiveCompactResult, error)
	ManualCompact(context.Context, ManualCompactRequest) (CompactResult, error)
	ManualSnip(context.Context, ManualSnipRequest) (SnipResult, error)
}

type BuildInputRequest struct {
	WorkspaceID       string
	SessionID         string
	TurnID            string
	Step              int
	Provider          string
	Model             string
	Source            string
	Messages          []message.Message
	ModelMessages     []fantasy.Message
	CanonicalMessages int
	ProjectedMessages int
	BudgetBefore      *BudgetReport
	BudgetAfter       *BudgetReport
	ToolResultBudget  ToolResultBudgetConfig
	Microcompact      MicrocompactConfig
	Snip              SnipConfig
	FullCompact       FullCompactConfig
	AutoCompact       AutoCompactConfig
	Now               time.Time
}

type BuildInputResult struct {
	Messages       []message.Message
	ModelMessages  []fantasy.Message
	Projection     Projection
	Boundaries     []Boundary
	Replacements   []ContentReplacement
	Warnings       []Warning
	SnipBoundaries []SnipBoundary
}

type ReactiveCompactRequest struct {
	SessionID    string
	TurnID       string
	ProjectionID string
	Attempt      int
	Error        string
}

type ReactiveCompactResult struct {
	Attempt ReactiveAttempt
}

type ManualCompactRequest struct {
	SessionID    string
	TurnID       string
	ProjectionID string
	Reason       string
}

type CompactResult struct {
	Boundary Boundary
}

type ManualSnipRequest struct {
	SessionID    string
	TurnID       string
	ProjectionID string
	Reason       string
}

type SnipResult struct {
	SnipBoundary SnipBoundary
}

type BudgetReport struct {
	TotalEstimatedTokens int    `json:"totalEstimatedTokens,omitempty"`
	ContextWindow        int    `json:"contextWindow,omitempty"`
	Model                string `json:"model,omitempty"`
}

type ToolResultBudgetConfig struct {
	MaxSingleResultChars int
	MessageBudgetChars   int
}

type MicrocompactConfig struct {
	Enabled              bool
	IdleIntervalMillis   int64
	LastAssistantAt      int64
	ToolResultCountLimit int
	ToolResultTokenLimit int
	KeepRecent           int
}

type FullCompactConfig struct {
	Enabled              bool
	Trigger              string
	PreserveTailMessages int
	ReinjectedMessages   []fantasy.Message
	MaxSummaryRetries    int
}

type AutoCompactConfig struct {
	Enabled                      bool
	EffectiveContextWindowTokens int
	OutputReserveTokens          int
	WarningBufferTokens          int
	AutoCompactBufferTokens      int
	BlockingBufferTokens         int
	MaxConsecutiveFailures       int
	ConsecutiveFailures          int
	HelperCall                   bool
	RecursionGuard               bool
}

type SnipConfig struct {
	Enabled              bool
	Reason               string
	PreserveHeadMessages int
	PreserveTailMessages int
	Summary              string
}

type CompactSummaryRequest struct {
	SessionID string
	TurnID    string
	Messages  []fantasy.Message
}

type CompactSummaryResult struct {
	Summary string
	Ref     string
}

type CompactSummarizer interface {
	SummarizeCompact(context.Context, CompactSummaryRequest) (CompactSummaryResult, error)
}

type Projection struct {
	ID                    string        `json:"id"`
	SessionID             string        `json:"sessionId"`
	TurnID                string        `json:"turnId"`
	Step                  int           `json:"step"`
	Provider              string        `json:"provider,omitempty"`
	Model                 string        `json:"model,omitempty"`
	Source                string        `json:"source"`
	Status                string        `json:"status"`
	CanonicalMessageCount int           `json:"canonicalMessageCount"`
	ProjectedMessageCount int           `json:"projectedMessageCount"`
	BudgetBefore          *BudgetReport `json:"budgetBefore,omitempty"`
	BudgetAfter           *BudgetReport `json:"budgetAfter,omitempty"`
	CreatedAt             int64         `json:"createdAt"`
	CompletedAt           int64         `json:"completedAt,omitempty"`
	Error                 string        `json:"error,omitempty"`
}

type ProjectionMessage struct {
	ID                 string `json:"id"`
	ProjectionID       string `json:"projectionId"`
	SessionID          string `json:"sessionId"`
	TurnID             string `json:"turnId"`
	Sequence           int    `json:"sequence"`
	Role               string `json:"role"`
	CanonicalMessageID string `json:"canonicalMessageId,omitempty"`
	Status             string `json:"status"`
	ReplacementID      string `json:"replacementId,omitempty"`
	TokenEstimate      int    `json:"tokenEstimate,omitempty"`
	ContentRef         string `json:"contentRef,omitempty"`
	Summary            string `json:"summary,omitempty"`
	CreatedAt          int64  `json:"createdAt"`
}

type Boundary struct {
	ID               string        `json:"id"`
	SessionID        string        `json:"sessionId"`
	TurnID           string        `json:"turnId,omitempty"`
	ProjectionID     string        `json:"projectionId,omitempty"`
	Kind             string        `json:"kind"`
	Trigger          string        `json:"trigger"`
	Status           string        `json:"status"`
	SummaryMessageID string        `json:"summaryMessageId,omitempty"`
	SummaryRef       string        `json:"summaryRef,omitempty"`
	MessageRefs      []string      `json:"messageRefs,omitempty"`
	ToolCallRefs     []string      `json:"toolCallRefs,omitempty"`
	ReinjectedRefs   []string      `json:"reinjectedRefs,omitempty"`
	BudgetBefore     *BudgetReport `json:"budgetBefore,omitempty"`
	BudgetAfter      *BudgetReport `json:"budgetAfter,omitempty"`
	CreatedAt        int64         `json:"createdAt"`
	CompletedAt      int64         `json:"completedAt,omitempty"`
	Error            string        `json:"error,omitempty"`
}

type ContentReplacement struct {
	ID                         string `json:"id"`
	SessionID                  string `json:"sessionId"`
	TurnID                     string `json:"turnId,omitempty"`
	ProjectionID               string `json:"projectionId,omitempty"`
	ToolCallID                 string `json:"toolCallId,omitempty"`
	ToolName                   string `json:"toolName,omitempty"`
	Kind                       string `json:"kind"`
	OriginalRef                string `json:"originalRef,omitempty"`
	ReplacementText            string `json:"replacementText"`
	OriginalSizeBytes          int64  `json:"originalSizeBytes,omitempty"`
	OriginalEstimatedTokens    int    `json:"originalEstimatedTokens,omitempty"`
	ReplacementEstimatedTokens int    `json:"replacementEstimatedTokens,omitempty"`
	Reason                     string `json:"reason,omitempty"`
	CreatedAt                  int64  `json:"createdAt"`
}

type SnipBoundary struct {
	ID                 string   `json:"id"`
	SessionID          string   `json:"sessionId"`
	TurnID             string   `json:"turnId,omitempty"`
	ProjectionID       string   `json:"projectionId,omitempty"`
	RemovedMessageRefs []string `json:"removedMessageRefs,omitempty"`
	PreservedHeadRef   string   `json:"preservedHeadRef,omitempty"`
	PreservedTailRef   string   `json:"preservedTailRef,omitempty"`
	SummaryRef         string   `json:"summaryRef,omitempty"`
	Reason             string   `json:"reason,omitempty"`
	CreatedAt          int64    `json:"createdAt"`
}

type ReadStateSnapshot struct {
	ID           string `json:"id"`
	SessionID    string `json:"sessionId"`
	TurnID       string `json:"turnId,omitempty"`
	ProjectionID string `json:"projectionId,omitempty"`
	StateJSON    string `json:"stateJson"`
	CreatedAt    int64  `json:"createdAt"`
}

type Reinjection struct {
	ID            string `json:"id"`
	SessionID     string `json:"sessionId"`
	TurnID        string `json:"turnId,omitempty"`
	ProjectionID  string `json:"projectionId,omitempty"`
	Kind          string `json:"kind"`
	Ref           string `json:"ref,omitempty"`
	Status        string `json:"status"`
	Reason        string `json:"reason,omitempty"`
	TokenEstimate int    `json:"tokenEstimate,omitempty"`
	CreatedAt     int64  `json:"createdAt"`
}

type Warning struct {
	ID           string `json:"id"`
	SessionID    string `json:"sessionId"`
	TurnID       string `json:"turnId,omitempty"`
	ProjectionID string `json:"projectionId,omitempty"`
	Code         string `json:"code"`
	Message      string `json:"message"`
	Severity     string `json:"severity"`
	CreatedAt    int64  `json:"createdAt"`
}

type ReactiveAttempt struct {
	ID           string `json:"id"`
	SessionID    string `json:"sessionId"`
	TurnID       string `json:"turnId"`
	ProjectionID string `json:"projectionId,omitempty"`
	Attempt      int    `json:"attempt"`
	Action       string `json:"action"`
	Status       string `json:"status"`
	Error        string `json:"error,omitempty"`
	CreatedAt    int64  `json:"createdAt"`
	CompletedAt  int64  `json:"completedAt,omitempty"`
}
