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
