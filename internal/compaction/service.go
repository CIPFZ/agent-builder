package compaction

import (
	"strings"

	"myclaw/internal/model"
)

type Config struct {
	MaxMessages         int
	MaxEstimatedTokens  int
	ContextWindowTokens int
	WarningBufferTokens int
	ErrorBufferTokens   int
	AutoCompactBufferTokens int
	BlockingBufferTokens int
	PreserveRecentTurns int
	SummaryPrefix       string
}

type Analysis struct {
	EstimatedTokens            int
	ContextWindowTokens        int
	WarningThreshold           int
	ErrorThreshold             int
	AutoCompactThreshold       int
	BlockingThreshold          int
	IsAboveWarningThreshold    bool
	IsAboveErrorThreshold      bool
	IsAboveAutoCompactThreshold bool
	IsAtBlockingLimit          bool
}

type Reason string

const (
	ReasonNone         Reason = ""
	ReasonMessageLimit Reason = "message-limit"
	ReasonTokenBudget  Reason = "token-budget"
)

type Result struct {
	Changed        bool
	Reason         Reason
	Analysis       Analysis
	OriginalCount  int
	CompactedCount int
	Messages       []model.Message
	SummaryMessage *model.Message
}

type Service struct {
	cfg Config
}

func NewService(cfg Config) *Service {
	if cfg.MaxMessages <= 0 {
		cfg.MaxMessages = 100
	}
	if cfg.PreserveRecentTurns <= 0 {
		cfg.PreserveRecentTurns = 2
	}
	if cfg.SummaryPrefix == "" {
		cfg.SummaryPrefix = "Summary:"
	}
	if cfg.WarningBufferTokens <= 0 {
		cfg.WarningBufferTokens = 20
	}
	if cfg.ErrorBufferTokens <= 0 {
		cfg.ErrorBufferTokens = 20
	}
	if cfg.AutoCompactBufferTokens <= 0 {
		cfg.AutoCompactBufferTokens = 13
	}
	if cfg.BlockingBufferTokens <= 0 {
		cfg.BlockingBufferTokens = 3
	}
	return &Service{cfg: cfg}
}

func (s *Service) Analyze(messages []model.Message) Analysis {
	estimated := estimateTokens(messages)
	window := s.cfg.ContextWindowTokens
	if window <= 0 {
		return Analysis{
			EstimatedTokens:     estimated,
			ContextWindowTokens: 0,
		}
	}
	warning := max(0, window-s.cfg.WarningBufferTokens)
	errThreshold := max(0, window-s.cfg.ErrorBufferTokens)
	autoCompact := max(0, window-s.cfg.AutoCompactBufferTokens)
	blocking := max(0, window-s.cfg.BlockingBufferTokens)
	return Analysis{
		EstimatedTokens:             estimated,
		ContextWindowTokens:         window,
		WarningThreshold:            warning,
		ErrorThreshold:              errThreshold,
		AutoCompactThreshold:        autoCompact,
		BlockingThreshold:           blocking,
		IsAboveWarningThreshold:     estimated >= warning && window > 0,
		IsAboveErrorThreshold:       estimated >= errThreshold && window > 0,
		IsAboveAutoCompactThreshold: estimated >= autoCompact && window > 0,
		IsAtBlockingLimit:           estimated >= blocking && window > 0,
	}
}

func (s *Service) CompactIfNeeded(messages []model.Message) ([]model.Message, bool) {
	result := s.Compact(messages)
	return result.Messages, result.Changed
}

func (s *Service) Compact(messages []model.Message) Result {
	analysis := s.Analyze(messages)
	result := Result{
		Reason:        ReasonNone,
		Analysis:      analysis,
		OriginalCount: len(messages),
		Messages:      cloneMessages(messages),
	}
	reason := s.compactionReason(messages)
	if reason == ReasonNone {
		result.CompactedCount = len(result.Messages)
		return result
	}
	if s.cfg.PreserveRecentTurns >= len(messages) {
		result.CompactedCount = len(result.Messages)
		return result
	}

	cutoff := len(messages) - s.cfg.PreserveRecentTurns
	older := messages[:cutoff]
	recent := messages[cutoff:]

	summaryParts := make([]string, 0, len(older))
	for _, msg := range older {
		content := strings.TrimSpace(msg.Content)
		if content == "" {
			continue
		}
		if msg.Role == "system" {
			continue
		}
		if msg.Role == "summary" {
			content = strings.TrimSpace(strings.TrimPrefix(content, s.cfg.SummaryPrefix))
		}
		summaryParts = append(summaryParts, content)
	}
	summary := model.Message{
		ID:        "summary-1",
		SessionID: firstSessionID(messages),
		Role:      "summary",
		Content:   s.cfg.SummaryPrefix + " " + strings.Join(summaryParts, " | "),
	}

	compacted := make([]model.Message, 0, 1+len(recent))
	compacted = append(compacted, summary)
	compacted = append(compacted, cloneMessages(recent)...)

	result.Changed = true
	result.Reason = reason
	result.Messages = compacted
	result.CompactedCount = len(compacted)
	result.SummaryMessage = &summary
	return result
}

func (s *Service) overTokenBudget(messages []model.Message) bool {
	if s.cfg.MaxEstimatedTokens <= 0 {
		return false
	}
	return estimateTokens(messages) > s.cfg.MaxEstimatedTokens
}

func estimateTokens(messages []model.Message) int {
	total := 0
	for _, msg := range messages {
		total += (len(msg.Content) + 3) / 4
	}
	return total
}

func cloneMessages(messages []model.Message) []model.Message {
	out := make([]model.Message, len(messages))
	copy(out, messages)
	return out
}

func firstSessionID(messages []model.Message) string {
	if len(messages) == 0 {
		return ""
	}
	return messages[0].SessionID
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}

func (s *Service) compactionReason(messages []model.Message) Reason {
	if len(messages) > s.cfg.MaxMessages {
		return ReasonMessageLimit
	}
	if s.overTokenBudget(messages) {
		return ReasonTokenBudget
	}
	return ReasonNone
}
