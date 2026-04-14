package compaction

import (
	"strings"

	"myclaw/internal/model"
)

type Config struct {
	MaxMessages                int
	MaxEstimatedTokens         int
	ContextWindowTokens        int
	WarningBufferTokens        int
	ErrorBufferTokens          int
	AutoCompactBufferTokens    int
	BlockingBufferTokens       int
	PreserveRecentTurns        int
	SummaryPrefix              string
	SessionMemoryMinTokens     int
	SessionMemoryMinTextBlocks int
	SessionMemoryMaxTokens     int
}

type Analysis struct {
	EstimatedTokens             int
	ContextWindowTokens         int
	WarningThreshold            int
	ErrorThreshold              int
	AutoCompactThreshold        int
	BlockingThreshold           int
	IsAboveWarningThreshold     bool
	IsAboveErrorThreshold       bool
	IsAboveAutoCompactThreshold bool
	IsAtBlockingLimit           bool
}

type Reason string

const (
	ReasonNone         Reason = ""
	ReasonMessageLimit Reason = "message-limit"
	ReasonTokenBudget  Reason = "token-budget"
	ReasonMicrocompact Reason = "microcompact"
)

const clearedToolResultMessage = "[Old tool result content cleared]"

var compactableToolResultPrefixes = []string{
	"system.run:",
	"mcp__",
}

type Result struct {
	Changed               bool
	Reason                Reason
	Analysis              Analysis
	OriginalCount         int
	CompactedCount        int
	Messages              []model.Message
	SummaryMessage        *model.Message
	SummarizedThroughID   string
	PostCompactTokenCount int
}

type Service struct {
	cfg Config
}

type SessionMemoryOptions struct {
	HookMessages         []model.Message
	TranscriptPath       string
	AutoCompactThreshold int
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
	if cfg.SessionMemoryMinTokens <= 0 {
		cfg.SessionMemoryMinTokens = 10000
	}
	if cfg.SessionMemoryMinTextBlocks <= 0 {
		cfg.SessionMemoryMinTextBlocks = 5
	}
	if cfg.SessionMemoryMaxTokens <= 0 {
		cfg.SessionMemoryMaxTokens = 40000
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
	return s.compactTraditional(messages)
}

func (s *Service) CompactWithSessionMemory(messages []model.Message, summaryMemory, lastSummarizedMessageID string) Result {
	return s.CompactWithSessionMemoryOptions(messages, summaryMemory, lastSummarizedMessageID, SessionMemoryOptions{})
}

func (s *Service) CompactWithSessionMemoryOptions(messages []model.Message, summaryMemory, lastSummarizedMessageID string, opts SessionMemoryOptions) Result {
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
	trimmedSummary := strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(summaryMemory), s.cfg.SummaryPrefix))
	if trimmedSummary == "" {
		return s.compactTraditional(messages)
	}

	startIndex := s.sessionMemoryStartIndex(messages, lastSummarizedMessageID)
	if startIndex < 0 {
		return s.compactTraditional(messages)
	}

	recent := filterCompactBoundaryMessages(messages[startIndex:])
	summary := model.Message{
		ID:        "summary-1",
		SessionID: firstSessionID(messages),
		Role:      "summary",
		Content:   s.cfg.SummaryPrefix + " " + trimmedSummary,
	}

	compacted := make([]model.Message, 0, 1+len(recent))
	compacted = append(compacted, summary)
	if transcriptNote := compactTranscriptNote(firstSessionID(messages), opts.TranscriptPath); transcriptNote != nil {
		compacted = append(compacted, *transcriptNote)
	}
	compacted = append(compacted, cloneMessages(opts.HookMessages)...)
	compacted = append(compacted, recent...)

	postCompactTokenCount := estimateTokens(compacted)
	if opts.AutoCompactThreshold > 0 && postCompactTokenCount >= opts.AutoCompactThreshold {
		return s.compactTraditional(messages)
	}

	result.Changed = true
	result.Reason = reason
	result.Messages = compacted
	result.CompactedCount = len(compacted)
	result.SummaryMessage = &summary
	result.PostCompactTokenCount = postCompactTokenCount
	if startIndex > 0 {
		result.SummarizedThroughID = messages[startIndex-1].ID
	}
	return result
}

func (s *Service) compactTraditional(messages []model.Message) Result {
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
	result.SummarizedThroughID = older[len(older)-1].ID
	return result
}

func (s *Service) Microcompact(messages []model.Message) Result {
	result := Result{
		Reason:        ReasonNone,
		Analysis:      s.Analyze(messages),
		OriginalCount: len(messages),
		Messages:      cloneMessages(messages),
	}
	if s.cfg.PreserveRecentTurns >= len(messages) {
		result.CompactedCount = len(result.Messages)
		return result
	}

	cutoff := len(messages) - s.cfg.PreserveRecentTurns
	changed := false
	compacted := cloneMessages(messages)
	for i := 0; i < cutoff; i++ {
		if compacted[i].Role != "tool" {
			continue
		}
		cleared, ok := microcompactToolMessage(compacted[i])
		if !ok {
			continue
		}
		compacted[i] = cleared
		changed = true
	}

	result.Messages = compacted
	result.CompactedCount = len(compacted)
	if changed {
		result.Changed = true
		result.Reason = ReasonMicrocompact
	}
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

func filterCompactBoundaryMessages(messages []model.Message) []model.Message {
	out := make([]model.Message, 0, len(messages))
	for _, msg := range messages {
		if msg.Role == "system" && msg.Content == "[compact_boundary]" {
			continue
		}
		out = append(out, msg)
	}
	return out
}

func firstSessionID(messages []model.Message) string {
	if len(messages) == 0 {
		return ""
	}
	return messages[0].SessionID
}

func compactTranscriptNote(sessionID, transcriptPath string) *model.Message {
	transcriptPath = strings.TrimSpace(transcriptPath)
	if transcriptPath == "" {
		return nil
	}
	return &model.Message{
		ID:        "compact-transcript-note",
		SessionID: sessionID,
		Role:      "system",
		Content:   "Previous transcript before compact is available at: " + transcriptPath,
	}
}

func microcompactToolMessage(msg model.Message) (model.Message, bool) {
	content := strings.TrimSpace(msg.Content)
	if content == "" {
		return msg, false
	}
	for _, prefix := range compactableToolResultPrefixes {
		if strings.HasPrefix(content, prefix) {
			if strings.HasSuffix(content, clearedToolResultMessage) {
				return msg, false
			}
			msg.Content = prefix + " " + clearedToolResultMessage
			msg.Content = strings.ReplaceAll(msg.Content, ":  ", ": ")
			return msg, true
		}
	}
	return msg, false
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}

func (s *Service) sessionMemoryStartIndex(messages []model.Message, lastSummarizedMessageID string) int {
	if len(messages) == 0 {
		return 0
	}

	lastSummarizedIndex := -1
	if strings.TrimSpace(lastSummarizedMessageID) != "" {
		found := false
		for i, msg := range messages {
			if msg.ID == lastSummarizedMessageID {
				lastSummarizedIndex = i
				found = true
				break
			}
		}
		if !found {
			return -1
		}
	}

	startIndex := len(messages)
	if lastSummarizedIndex >= 0 {
		startIndex = lastSummarizedIndex + 1
	}

	totalTokens := 0
	textBlocks := 0
	for i := startIndex; i < len(messages); i++ {
		totalTokens += estimateTokens([]model.Message{messages[i]})
		if hasTextBlocks(messages[i]) {
			textBlocks++
		}
	}
	if totalTokens >= s.cfg.SessionMemoryMaxTokens {
		return s.adjustToBoundaryFloor(messages, startIndex)
	}
	if totalTokens >= s.cfg.SessionMemoryMinTokens && textBlocks >= s.cfg.SessionMemoryMinTextBlocks {
		return s.adjustToBoundaryFloor(messages, startIndex)
	}

	floor := 0
	for i := len(messages) - 1; i >= 0; i-- {
		if messages[i].Role == "system" && messages[i].Content == "[compact_boundary]" {
			floor = i + 1
			break
		}
	}

	for i := startIndex - 1; i >= floor; i-- {
		totalTokens += estimateTokens([]model.Message{messages[i]})
		if hasTextBlocks(messages[i]) {
			textBlocks++
		}
		startIndex = i
		if totalTokens >= s.cfg.SessionMemoryMaxTokens {
			break
		}
		if totalTokens >= s.cfg.SessionMemoryMinTokens && textBlocks >= s.cfg.SessionMemoryMinTextBlocks {
			break
		}
	}
	return s.adjustToBoundaryFloor(messages, startIndex)
}

func (s *Service) adjustToBoundaryFloor(messages []model.Message, startIndex int) int {
	if startIndex < 0 {
		return 0
	}
	if startIndex > len(messages) {
		return len(messages)
	}
	if startIndex == 0 || startIndex >= len(messages) {
		return startIndex
	}

	adjusted := startIndex
	toolResultIDs := make(map[string]struct{})
	for i := startIndex; i < len(messages); i++ {
		for _, id := range toolResultIDsForMessage(messages[i]) {
			toolResultIDs[id] = struct{}{}
		}
	}
	if len(toolResultIDs) > 0 {
		for i := adjusted; i < len(messages); i++ {
			for _, id := range toolUseIDsForMessage(messages[i]) {
				delete(toolResultIDs, id)
			}
		}
		for i := adjusted - 1; i >= 0 && len(toolResultIDs) > 0; i-- {
			for _, id := range toolUseIDsForMessage(messages[i]) {
				if _, ok := toolResultIDs[id]; ok {
					adjusted = i
					delete(toolResultIDs, id)
				}
			}
		}
	}

	providerIDs := make(map[string]struct{})
	for i := adjusted; i < len(messages); i++ {
		if messages[i].Role != "assistant" {
			continue
		}
		if id := strings.TrimSpace(messages[i].ProviderMessageID); id != "" {
			providerIDs[id] = struct{}{}
		}
	}
	for i := adjusted - 1; i >= 0; i-- {
		if messages[i].Role != "assistant" {
			continue
		}
		if _, ok := providerIDs[strings.TrimSpace(messages[i].ProviderMessageID)]; ok && strings.TrimSpace(messages[i].ProviderMessageID) != "" {
			adjusted = i
		}
	}

	if adjusted == startIndex && messages[startIndex].Role == "tool" && messages[startIndex-1].Role == "assistant" {
		return startIndex - 1
	}
	return adjusted
}

func hasTextBlocks(message model.Message) bool {
	if len(message.Blocks) > 0 {
		for _, block := range message.Blocks {
			if block.Type == model.MessageBlockText && strings.TrimSpace(block.Text+block.Content) != "" {
				return true
			}
		}
		return false
	}
	if strings.TrimSpace(message.Content) == "" {
		return false
	}
	switch message.Role {
	case "user", "assistant", "summary":
		return true
	default:
		return false
	}
}

func toolResultIDsForMessage(message model.Message) []string {
	if message.Role == "tool" {
		return []string{message.ID}
	}
	if message.Role != "user" {
		return nil
	}
	ids := make([]string, 0, len(message.Blocks))
	for _, block := range message.Blocks {
		if block.Type == model.MessageBlockToolResult && strings.TrimSpace(block.ToolUseID) != "" {
			ids = append(ids, strings.TrimSpace(block.ToolUseID))
		}
	}
	return ids
}

func toolUseIDsForMessage(message model.Message) []string {
	if message.Role != "assistant" {
		return nil
	}
	ids := make([]string, 0, len(message.Blocks))
	for _, block := range message.Blocks {
		if block.Type == model.MessageBlockToolUse && strings.TrimSpace(block.ID) != "" {
			ids = append(ids, strings.TrimSpace(block.ID))
		}
	}
	return ids
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
