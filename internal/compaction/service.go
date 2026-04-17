package compaction

import (
	"crypto/rand"
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"myclaw/internal/model"
	"myclaw/internal/tools"
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
const maxSessionMemorySectionLength = 2000

var compactableToolResultPrefixes = []string{
	"system.run:",
	"mcp__",
}

type Result struct {
	Changed                   bool
	Reason                    Reason
	Analysis                  Analysis
	OriginalCount             int
	CompactedCount            int
	Messages                  []model.Message
	BoundaryMessage           *model.Message
	SummaryMessage            *model.Message
	Attachments               []model.Message
	HookMessages              []model.Message
	SummarizedThroughID       string
	PostCompactTokenCount     int
	TruePostCompactTokenCount int
}

type Service struct {
	cfg Config
}

type SessionMemoryOptions struct {
	HookMessages         []model.Message
	TranscriptPath       string
	AutoCompactThreshold int
	PlanAttachment       *model.Message
	InvokedSkills        []tools.InvokedSkillInfo
	SessionMemoryPath    string
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
	sessionID := firstSessionID(messages)
	summary := sessionMemorySummaryMessage(sessionID, trimmedSummary, opts)
	boundary := compactBoundaryMessage(sessionID, analysis.EstimatedTokens, messages, len(messages[:startIndex]))
	boundary.CompactMetadata.PreCompactDiscoveredTools = extractDiscoveredToolNames(messages)
	annotateBoundaryWithPreservedSegment(&boundary, summary.ID, recent)

	attachments := make([]model.Message, 0, 1+len(opts.InvokedSkills))
	if opts.PlanAttachment != nil {
		attachments = append(attachments, cloneMessage(*opts.PlanAttachment))
	}
	if len(opts.InvokedSkills) > 0 {
		attachments = append(attachments, tools.BuildInvokedSkillsAttachmentMessage(newUUID(), sessionID, opts.InvokedSkills))
	}

	compacted := make([]model.Message, 0, 2+len(recent)+len(attachments)+len(opts.HookMessages))
	compacted = append(compacted, boundary, summary)
	compacted = append(compacted, recent...)
	compacted = append(compacted, attachments...)
	compacted = append(compacted, cloneMessages(opts.HookMessages)...)

	postCompactTokenCount := estimateTokens(compacted)
	if opts.AutoCompactThreshold > 0 && postCompactTokenCount >= opts.AutoCompactThreshold {
		return s.compactTraditional(messages)
	}

	result.Changed = true
	result.Reason = reason
	result.Messages = compacted
	result.CompactedCount = len(compacted)
	result.BoundaryMessage = &boundary
	result.SummaryMessage = &summary
	result.Attachments = attachments
	result.HookMessages = cloneMessages(opts.HookMessages)
	result.PostCompactTokenCount = postCompactTokenCount
	result.TruePostCompactTokenCount = postCompactTokenCount
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
		contentTokens := roughTokenCount(msg.Content)
		blockTokens := 0
		for _, block := range msg.Blocks {
			blockTokens += estimateBlockTokens(block)
		}
		if blockTokens > 0 && strings.TrimSpace(msg.Content) == strings.TrimSpace(messageBlocksText(msg.Blocks)) {
			if blockTokens > contentTokens {
				total += blockTokens
			} else {
				total += contentTokens
			}
		} else {
			total += contentTokens + blockTokens
		}
		if msg.CompactMetadata != nil {
			if encoded, err := json.Marshal(msg.CompactMetadata); err == nil {
				total += roughTokenCount(string(encoded))
			}
		}
	}
	return total
}

func messageBlocksText(blocks []model.MessageBlock) string {
	var parts []string
	for _, block := range blocks {
		switch block.Type {
		case model.MessageBlockText, model.MessageBlockThinking:
			if block.Text != "" {
				parts = append(parts, block.Text)
			}
		case model.MessageBlockToolResult:
			if block.Content != "" {
				parts = append(parts, block.Content)
			}
		case model.MessageBlockToolUse:
			if block.Name != "" {
				parts = append(parts, block.Name)
			}
		default:
			if block.Text != "" {
				parts = append(parts, block.Text)
			}
		}
	}
	return strings.Join(parts, "\n")
}

func estimateBlockTokens(block model.MessageBlock) int {
	if block.Raw != nil {
		if encoded, err := json.Marshal(block.Raw); err == nil {
			return roughTokenCount(string(encoded))
		}
	}
	total := roughTokenCount(block.Text) + roughTokenCount(block.Content) + roughTokenCount(block.Name) + roughTokenCount(block.Input)
	if block.InputObject != nil {
		if encoded, err := json.Marshal(block.InputObject); err == nil {
			total += roughTokenCount(string(encoded))
		}
	}
	return total
}

func roughTokenCount(content string) int {
	return (len(content) + 3) / 4
}

func cloneMessages(messages []model.Message) []model.Message {
	out := make([]model.Message, len(messages))
	for i, message := range messages {
		out[i] = cloneMessage(message)
	}
	return out
}

func cloneMessage(message model.Message) model.Message {
	if len(message.Blocks) > 0 {
		message.Blocks = append([]model.MessageBlock(nil), message.Blocks...)
		for i := range message.Blocks {
			if message.Blocks[i].InputObject != nil {
				message.Blocks[i].InputObject = cloneAnyMap(message.Blocks[i].InputObject)
			}
			if message.Blocks[i].Raw != nil {
				message.Blocks[i].Raw = cloneAnyMap(message.Blocks[i].Raw)
			}
		}
	}
	if message.CompactMetadata != nil {
		metadata := *message.CompactMetadata
		metadata.PreCompactDiscoveredTools = append([]string(nil), message.CompactMetadata.PreCompactDiscoveredTools...)
		if message.CompactMetadata.PreservedSegment != nil {
			segment := *message.CompactMetadata.PreservedSegment
			metadata.PreservedSegment = &segment
		}
		message.CompactMetadata = &metadata
	}
	return message
}

func cloneAnyMap(input map[string]any) map[string]any {
	if input == nil {
		return nil
	}
	out := make(map[string]any, len(input))
	for key, value := range input {
		out[key] = value
	}
	return out
}

func filterCompactBoundaryMessages(messages []model.Message) []model.Message {
	out := make([]model.Message, 0, len(messages))
	for _, msg := range messages {
		if isCompactBoundaryMessage(msg) {
			continue
		}
		out = append(out, cloneMessage(msg))
	}
	return out
}

func firstSessionID(messages []model.Message) string {
	if len(messages) == 0 {
		return ""
	}
	return messages[0].SessionID
}

func sessionMemorySummaryMessage(sessionID, content string, opts SessionMemoryOptions) model.Message {
	truncatedContent, wasTruncated := truncateSessionMemoryForCompact(content)
	content = getCompactUserSummaryMessage(truncatedContent, true, strings.TrimSpace(opts.TranscriptPath), true)
	if wasTruncated {
		if sessionMemoryPath := strings.TrimSpace(opts.SessionMemoryPath); sessionMemoryPath != "" {
			content += "\n\nSome session memory sections were truncated for length. The full session memory can be viewed at: " + sessionMemoryPath
		}
	}
	return model.Message{
		ID:                        newUUID(),
		SessionID:                 sessionID,
		Role:                      "user",
		Content:                   content,
		IsCompactSummary:          true,
		IsVisibleInTranscriptOnly: true,
	}
}

func compactBoundaryMessage(sessionID string, preTokens int, messages []model.Message, messagesSummarized int) model.Message {
	boundary := model.Message{
		ID:        newUUID(),
		SessionID: sessionID,
		Role:      "system",
		Subtype:   "compact_boundary",
		Content:   "Conversation compacted",
		Level:     "info",
		CompactMetadata: &model.CompactMetadata{
			Trigger:            "auto",
			PreTokens:          preTokens,
			MessagesSummarized: messagesSummarized,
		},
	}
	if len(messages) > 0 {
		boundary.LogicalParentID = messages[len(messages)-1].ID
	}
	return boundary
}

func annotateBoundaryWithPreservedSegment(boundary *model.Message, anchorID string, messagesToKeep []model.Message) {
	if boundary == nil || len(messagesToKeep) == 0 {
		return
	}
	if boundary.CompactMetadata == nil {
		boundary.CompactMetadata = &model.CompactMetadata{}
	}
	boundary.CompactMetadata.PreservedSegment = &model.CompactPreservedSegment{
		HeadID:   messagesToKeep[0].ID,
		AnchorID: anchorID,
		TailID:   messagesToKeep[len(messagesToKeep)-1].ID,
	}
}

func isCompactBoundaryMessage(message model.Message) bool {
	return message.Role == "system" && (message.Subtype == "compact_boundary" || message.Content == "[compact_boundary]")
}

func getCompactUserSummaryMessage(summary string, suppressFollowUpQuestions bool, transcriptPath string, recentMessagesPreserved bool) string {
	formattedSummary := formatCompactSummary(summary)
	baseSummary := "This session is being continued from a previous conversation that ran out of context. The summary below covers the earlier portion of the conversation.\n\n" + formattedSummary
	if transcriptPath != "" {
		baseSummary += "\n\nIf you need specific details from before compaction (like exact code snippets, error messages, or content you generated), read the full transcript at: " + transcriptPath
	}
	if recentMessagesPreserved {
		baseSummary += "\n\nRecent messages are preserved verbatim."
	}
	if suppressFollowUpQuestions {
		baseSummary += "\nContinue the conversation from where it left off without asking the user any further questions. Resume directly - do not acknowledge the summary, do not recap what was happening, do not preface with \"I'll continue\" or similar. Pick up the last task as if the break never happened."
	}
	return baseSummary
}

func formatCompactSummary(summary string) string {
	formatted := summary
	if start := strings.Index(formatted, "<analysis>"); start >= 0 {
		if end := strings.Index(formatted[start:], "</analysis>"); end >= 0 {
			formatted = formatted[:start] + formatted[start+end+len("</analysis>"):]
		}
	}
	if start := strings.Index(formatted, "<summary>"); start >= 0 {
		if end := strings.Index(formatted[start:], "</summary>"); end >= 0 {
			inner := strings.TrimSpace(formatted[start+len("<summary>") : start+end])
			formatted = formatted[:start] + "Summary:\n" + inner + formatted[start+end+len("</summary>"):]
		}
	}
	for strings.Contains(formatted, "\n\n\n") {
		formatted = strings.ReplaceAll(formatted, "\n\n\n", "\n\n")
	}
	return strings.TrimSpace(formatted)
}

func truncateSessionMemoryForCompact(content string) (string, bool) {
	lines := strings.Split(content, "\n")
	maxCharsPerSection := maxSessionMemorySectionLength * 4
	output := make([]string, 0, len(lines))
	currentHeader := ""
	currentLines := make([]string, 0)
	wasTruncated := false

	flush := func() {
		flushed, truncated := flushSessionMemorySection(currentHeader, currentLines, maxCharsPerSection)
		output = append(output, flushed...)
		wasTruncated = wasTruncated || truncated
	}

	for _, line := range lines {
		if strings.HasPrefix(line, "# ") {
			flush()
			currentHeader = line
			currentLines = currentLines[:0]
			continue
		}
		currentLines = append(currentLines, line)
	}
	flush()
	return strings.Join(output, "\n"), wasTruncated
}

func flushSessionMemorySection(sectionHeader string, sectionLines []string, maxCharsPerSection int) ([]string, bool) {
	if sectionHeader == "" {
		return append([]string(nil), sectionLines...), false
	}
	sectionContent := strings.Join(sectionLines, "\n")
	if len(sectionContent) <= maxCharsPerSection {
		out := make([]string, 0, 1+len(sectionLines))
		out = append(out, sectionHeader)
		out = append(out, sectionLines...)
		return out, false
	}

	charCount := 0
	kept := []string{sectionHeader}
	for _, line := range sectionLines {
		if charCount+len(line)+1 > maxCharsPerSection {
			break
		}
		kept = append(kept, line)
		charCount += len(line) + 1
	}
	kept = append(kept, "\n[... section truncated for length ...]")
	return kept, true
}

func newUUID() string {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		return ""
	}
	b[6] = (b[6] & 0x0f) | 0x40
	b[8] = (b[8] & 0x3f) | 0x80
	return fmt.Sprintf("%08x-%04x-%04x-%04x-%012x", b[0:4], b[4:6], b[6:8], b[8:10], b[10:16])
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

func extractDiscoveredToolNames(messages []model.Message) []string {
	discovered := make(map[string]struct{})
	for _, message := range messages {
		if message.CompactMetadata != nil {
			for _, name := range message.CompactMetadata.PreCompactDiscoveredTools {
				name = strings.TrimSpace(name)
				if name != "" {
					discovered[name] = struct{}{}
				}
			}
		}
		if message.Role != "user" {
			continue
		}
		for _, block := range message.Blocks {
			extractToolReferences(discovered, block.Raw)
		}
	}
	names := make([]string, 0, len(discovered))
	for name := range discovered {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

func extractToolReferences(discovered map[string]struct{}, value any) {
	switch typed := value.(type) {
	case map[string]any:
		if typed["type"] == "tool_reference" {
			if name := strings.TrimSpace(fmt.Sprint(typed["tool_name"])); name != "" && name != "<nil>" {
				discovered[name] = struct{}{}
			}
		}
		for _, child := range typed {
			extractToolReferences(discovered, child)
		}
	case []any:
		for _, child := range typed {
			extractToolReferences(discovered, child)
		}
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
		if isCompactBoundaryMessage(messages[i]) {
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
