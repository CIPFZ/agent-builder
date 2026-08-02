package agent

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"strings"
	"unicode/utf8"

	"charm.land/fantasy"
	"github.com/CIPFZ/agent-builder/internal/message"
)

type PromptAssemblyRecorder interface {
	RecordPromptAssembly(context.Context, PromptAssemblySnapshot) error
}

type ModelInputBuilder interface {
	BuildModelInput(context.Context, ModelInputSnapshot) (ModelInputProjection, error)
}

// SessionHistoryProjector rebuilds the provider-facing history from
// canonical messages and the latest completed persisted compact boundary.
// Canonical storage remains untouched.
type SessionHistoryProjector interface {
	ProjectSessionHistory(context.Context, string, []message.Message) ([]message.Message, error)
}

// ReactiveCompactor is invoked by the agent loop after a provider request
// fails with a context-length error. The implementation records the reactive
// attempt, executes the correct recovery action for the current attempt
// number (projection reduction for attempt 1, full compact for attempt 2+),
// and can signal that the session's circuit breaker is open — in which case
// the agent must stop retrying and surface the underlying error.
type ReactiveCompactor interface {
	ReactiveCompact(context.Context, ReactiveCompactSnapshot) (ReactiveCompactResult, error)
}

// ReactiveCompactSnapshot is the input the agent hands to ReactiveCompact
// after a Stream call fails with a context-length error. Messages is the
// prompt-side view of the history passed to the failed model call and is
// used by the runtime to install a per-turn boundary-anchored projection
// (summary + pairing-safe tail) so the next Stream attempt sends less input.
type ReactiveCompactSnapshot struct {
	SessionID string
	TurnID    string
	Step      int
	Provider  string
	Model     string
	Attempt   int
	Error     string
	Messages  []fantasy.Message
}

// ReactiveCompactResult carries what the caller needs to decide whether to
// retry the failed Stream call. When CircuitOpen is true the caller must
// stop retrying — the runtime has already published compact.failed with
// circuit_open=true and further reactive attempts would produce the same
// result until a successful compact resets the counter.
type ReactiveCompactResult struct {
	Action      string
	CircuitOpen bool
}

type ModelInputSnapshot struct {
	SessionID string
	TurnID    string
	Step      int
	Provider  string
	Model     string
	Source    string
	Messages  []fantasy.Message
}

type ModelInputProjection struct {
	ProjectionID          string
	Messages              []fantasy.Message
	CanonicalMessageCount int
	ProjectedMessageCount int
}

type PromptAssemblySnapshot struct {
	SessionID    string
	TurnID       string
	ProjectionID string
	Step         int
	Provider     string
	Model        string
	Sections     []PromptSectionSummary
	System       PromptSystemSummary
	Messages     PromptMessageSummary
	Tools        PromptToolSummary
	Skills       PromptSkillSummary
	MCP          PromptMCPSummary
	CreatedAt    int64
}

type PromptSectionSummary struct {
	ID            string
	Name          string
	Kind          string
	Role          string
	Order         int
	CachePolicy   string
	Source        string
	SourceRefs    []string
	Scope         string
	Hash          string
	Length        int
	TokenEstimate int
	Redacted      bool
	RawStored     bool
	Diagnostics   string
}

type PromptSystemSummary struct {
	Source             string
	Hash               string
	Length             int
	TokenEstimate      int
	PromptPrefix       bool
	PromptPrefixHash   string
	PromptPrefixTokens int
	SourceRefs         []string
	Redacted           bool
}

type PromptMessageSummary struct {
	Count                int
	ByRole               map[string]int
	ToolResultCount      int
	DeliveredToolResults int
	SyntheticToolResults int
	AttachmentCount      int
	ImageCount           int
	TokenEstimate        int
	RawPromptStored      bool
}

type PromptToolSummary struct {
	Selected      []string
	Omitted       []string
	SelectedCount int
	OmittedCount  int
}

type PromptSkillSummary struct {
	AvailableCount   int
	LoadedCount      int
	Names            []string
	LoadedNames      []string
	XMLPresent       bool
	XMLHash          string
	TokenEstimate    int
	RawContentStored bool
}

type PromptMCPSummary struct {
	ServerCount      int
	InstructionCount int
	Servers          []string
	ServerListHash   string
	InstructionHash  string
	TokenEstimate    int
	RawContentStored bool
}

func buildPromptMessageSummary(messages []fantasy.Message, attachments []message.Attachment) PromptMessageSummary {
	summary := PromptMessageSummary{
		Count:           len(messages),
		ByRole:          make(map[string]int),
		AttachmentCount: len(attachments),
		RawPromptStored: false,
	}
	for _, attachment := range attachments {
		if !attachment.IsText() {
			summary.ImageCount++
		}
	}
	for _, msg := range messages {
		role := string(msg.Role)
		if role == "" {
			role = "unknown"
		}
		summary.ByRole[role]++
		for _, part := range msg.Content {
			summary.TokenEstimate += estimatePromptTokens(promptPartSummaryText(part))
			if tr, ok := fantasy.AsMessagePart[fantasy.ToolResultPart](part); ok {
				summary.ToolResultCount++
				if tr.ToolCallID != "" {
					summary.DeliveredToolResults++
				}
				if strings.Contains(strings.ToLower(promptPartSummaryText(part)), "interrupted") {
					summary.SyntheticToolResults++
				}
			}
		}
	}
	return summary
}

func promptPartSummaryText(part fantasy.MessagePart) string {
	switch value := part.(type) {
	case fantasy.TextPart:
		return value.Text
	case fantasy.ToolResultPart:
		switch output := value.Output.(type) {
		case fantasy.ToolResultOutputContentText:
			return output.Text
		case fantasy.ToolResultOutputContentError:
			if output.Error != nil {
				return output.Error.Error()
			}
		case fantasy.ToolResultOutputContentMedia:
			return output.Text
		}
	case fantasy.FilePart:
		return value.Filename + " " + value.MediaType
	}
	return ""
}

func hashPromptText(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return ""
	}
	sum := sha256.Sum256([]byte(value))
	return "sha256:" + hex.EncodeToString(sum[:])
}

func estimatePromptTokens(text string) int {
	text = strings.TrimSpace(text)
	if text == "" {
		return 0
	}
	tokens := (utf8.RuneCountInString(text) + 3) / 4
	if tokens < 1 {
		return 1
	}
	return tokens
}
