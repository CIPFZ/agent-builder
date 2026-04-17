package model

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"
)

const syntheticClaudeModel = "<synthetic>"

type ClaudeTranscriptOptions struct {
	ParentUUID              *string
	LogicalParentUUID       *string
	SourceToolAssistantUUID string
	Timestamp               string
	IsSidechain             bool
	CWD                     string
	UserType                string
	Entrypoint              string
	Version                 string
	GitBranch               string
	Slug                    string
	AgentID                 string
	TeamName                string
	AgentName               string
	AgentColor              string
	PromptID                string
}

type ClaudeTranscriptMessage struct {
	ParentUUID                *string                    `json:"parentUuid"`
	LogicalParentUUID         *string                    `json:"logicalParentUuid,omitempty"`
	IsSidechain               bool                       `json:"isSidechain"`
	CWD                       string                     `json:"cwd"`
	UserType                  string                     `json:"userType"`
	Entrypoint                string                     `json:"entrypoint,omitempty"`
	SessionID                 string                     `json:"sessionId"`
	Version                   string                     `json:"version"`
	GitBranch                 string                     `json:"gitBranch,omitempty"`
	Slug                      string                     `json:"slug,omitempty"`
	AgentID                   string                     `json:"agentId,omitempty"`
	TeamName                  string                     `json:"teamName,omitempty"`
	AgentName                 string                     `json:"agentName,omitempty"`
	AgentColor                string                     `json:"agentColor,omitempty"`
	PromptID                  string                     `json:"promptId,omitempty"`
	Type                      string                     `json:"type"`
	UUID                      string                     `json:"uuid"`
	Timestamp                 string                     `json:"timestamp"`
	Message                   *ClaudeAPIMessage          `json:"message,omitempty"`
	Subtype                   string                     `json:"subtype,omitempty"`
	Content                   string                     `json:"content,omitempty"`
	IsMeta                    bool                       `json:"isMeta,omitempty"`
	IsVisibleInTranscriptOnly bool                       `json:"isVisibleInTranscriptOnly,omitempty"`
	IsCompactSummary          bool                       `json:"isCompactSummary,omitempty"`
	Level                     string                     `json:"level,omitempty"`
	CompactMetadata           *CompactMetadata           `json:"compactMetadata,omitempty"`
	SourceToolAssistantUUID   string                     `json:"sourceToolAssistantUUID,omitempty"`
	ToolUseResult             any                        `json:"toolUseResult,omitempty"`
	Runtime                   *RuntimeTranscriptMetadata `json:"myclaw,omitempty"`
}

type ClaudeAPIMessage struct {
	ID                string `json:"id,omitempty"`
	Container         any    `json:"container,omitempty"`
	Model             string `json:"model,omitempty"`
	Role              string `json:"role"`
	StopReason        string `json:"stop_reason,omitempty"`
	StopSequence      string `json:"stop_sequence,omitempty"`
	Type              string `json:"type,omitempty"`
	Usage             any    `json:"usage,omitempty"`
	Content           any    `json:"content"`
	ContextManagement any    `json:"context_management,omitempty"`
}

type RuntimeTranscriptMetadata struct {
	Role    string `json:"role,omitempty"`
	Subtype string `json:"subtype,omitempty"`
	Content string `json:"content,omitempty"`
}

type ClaudeTranscriptEntry struct {
	Type       string
	Message    *ClaudeTranscriptMessage
	Raw        map[string]any
	LineNumber int
}

func NewClaudeTranscriptEntry(message ClaudeTranscriptMessage) ClaudeTranscriptEntry {
	return ClaudeTranscriptEntry{Type: message.Type, Message: &message}
}

func NewClaudeMetadataEntry(raw map[string]any) ClaudeTranscriptEntry {
	cloned := cloneAnyMap(raw)
	entryType, _ := cloned["type"].(string)
	return ClaudeTranscriptEntry{Type: entryType, Raw: cloned}
}

func (e ClaudeTranscriptEntry) MarshalJSON() ([]byte, error) {
	if e.Message != nil {
		return json.Marshal(e.Message)
	}
	if e.Raw != nil {
		return json.Marshal(e.Raw)
	}
	return json.Marshal(map[string]any{"type": e.Type})
}

func (e *ClaudeTranscriptEntry) UnmarshalJSON(data []byte) error {
	var raw map[string]any
	if err := json.Unmarshal(data, &raw); err != nil {
		return err
	}
	entryType, _ := raw["type"].(string)
	e.Type = entryType
	if isClaudeTranscriptType(entryType) {
		var message ClaudeTranscriptMessage
		if err := json.Unmarshal(data, &message); err != nil {
			return err
		}
		e.Message = &message
		e.Raw = nil
		return nil
	}
	e.Message = nil
	e.Raw = raw
	return nil
}

func NewClaudeTranscriptMessage(message Message, opts ClaudeTranscriptOptions) ClaudeTranscriptMessage {
	timestamp := opts.Timestamp
	if timestamp == "" && !message.CreatedAt.IsZero() {
		timestamp = message.CreatedAt.Format(time.RFC3339Nano)
	}
	if timestamp == "" {
		timestamp = time.Now().UTC().Format(time.RFC3339Nano)
	}
	logicalParent := opts.LogicalParentUUID
	if logicalParent == nil && message.LogicalParentID != "" {
		value := message.LogicalParentID
		logicalParent = &value
	}

	out := ClaudeTranscriptMessage{
		ParentUUID:                opts.ParentUUID,
		LogicalParentUUID:         logicalParent,
		IsSidechain:               opts.IsSidechain,
		CWD:                       firstNonEmpty(message.CWD, opts.CWD),
		UserType:                  firstNonEmpty(message.UserType, opts.UserType, "external"),
		Entrypoint:                firstNonEmpty(message.Entrypoint, opts.Entrypoint),
		SessionID:                 message.SessionID,
		Version:                   firstNonEmpty(message.Version, opts.Version, "unknown"),
		GitBranch:                 firstNonEmpty(message.GitBranch, opts.GitBranch),
		Slug:                      firstNonEmpty(message.Slug, opts.Slug),
		AgentID:                   firstNonEmpty(message.AgentID, opts.AgentID),
		TeamName:                  firstNonEmpty(message.TeamName, opts.TeamName),
		AgentName:                 firstNonEmpty(message.AgentName, opts.AgentName),
		AgentColor:                firstNonEmpty(message.AgentColor, opts.AgentColor),
		PromptID:                  firstNonEmpty(message.PromptID, opts.PromptID),
		Type:                      claudeTranscriptType(message),
		UUID:                      message.ID,
		Timestamp:                 timestamp,
		IsMeta:                    message.IsMeta,
		IsVisibleInTranscriptOnly: message.IsVisibleInTranscriptOnly,
		IsCompactSummary:          message.IsCompactSummary || message.Role == "summary",
		SourceToolAssistantUUID:   firstNonEmpty(message.SourceToolAssistantUUID, opts.SourceToolAssistantUUID),
		ToolUseResult:             message.ToolUseResult,
	}
	if message.Role == "tool" && out.SourceToolAssistantUUID != "" {
		value := out.SourceToolAssistantUUID
		out.ParentUUID = &value
	}
	if runtime := runtimeTranscriptMetadata(message, out.Type); runtime != nil {
		out.Runtime = runtime
	}

	switch out.Type {
	case "assistant":
		out.Message = &ClaudeAPIMessage{
			ID:                firstNonEmpty(message.ProviderMessageID, message.ID),
			Container:         nil,
			Model:             firstNonEmpty(message.ProviderModel, syntheticClaudeModel),
			Role:              "assistant",
			StopReason:        firstNonEmpty(message.StopReason, "stop_sequence"),
			StopSequence:      message.StopSequence,
			Type:              "message",
			Usage:             message.ProviderUsage,
			Content:           claudeAssistantContent(message),
			ContextManagement: nil,
		}
	case "user":
		out.Message = &ClaudeAPIMessage{
			Role:    "user",
			Content: claudeUserContent(message),
		}
	case "system":
		out.Subtype = message.Subtype
		out.Content = message.Content
		out.Level = message.Level
		out.CompactMetadata = message.CompactMetadata
		if message.Subtype == "compact_boundary" {
			if out.LogicalParentUUID == nil && out.ParentUUID != nil {
				value := *out.ParentUUID
				out.LogicalParentUUID = &value
			}
			out.ParentUUID = nil
		}
	case "attachment":
		out.Subtype = message.Subtype
		out.Content = message.Content
		out.Level = message.Level
	default:
		out.Message = &ClaudeAPIMessage{
			Role:    "user",
			Content: message.Content,
		}
	}
	return out
}

func MessageFromClaudeTranscript(entry ClaudeTranscriptMessage, sessionID string) (Message, error) {
	createdAt, err := parseClaudeTimestamp(entry.Timestamp)
	if err != nil {
		return Message{}, err
	}

	message := Message{
		ID:                        entry.UUID,
		SessionID:                 sessionID,
		Role:                      entry.Type,
		Subtype:                   entry.Subtype,
		Content:                   entry.Content,
		IsMeta:                    entry.IsMeta,
		Level:                     entry.Level,
		CompactMetadata:           entry.CompactMetadata,
		IsCompactSummary:          entry.IsCompactSummary,
		IsVisibleInTranscriptOnly: entry.IsVisibleInTranscriptOnly,
		SourceToolAssistantUUID:   entry.SourceToolAssistantUUID,
		ToolUseResult:             entry.ToolUseResult,
		CWD:                       entry.CWD,
		UserType:                  entry.UserType,
		Entrypoint:                entry.Entrypoint,
		Version:                   entry.Version,
		GitBranch:                 entry.GitBranch,
		Slug:                      entry.Slug,
		AgentID:                   entry.AgentID,
		TeamName:                  entry.TeamName,
		AgentName:                 entry.AgentName,
		AgentColor:                entry.AgentColor,
		PromptID:                  entry.PromptID,
		CreatedAt:                 createdAt,
	}
	if entry.LogicalParentUUID != nil {
		message.LogicalParentID = *entry.LogicalParentUUID
	}

	switch entry.Type {
	case "assistant":
		if entry.Message == nil {
			return message, nil
		}
		message.Role = "assistant"
		message.ProviderMessageID = entry.Message.ID
		message.ProviderModel = entry.Message.Model
		message.StopReason = entry.Message.StopReason
		message.StopSequence = entry.Message.StopSequence
		message.ProviderUsage = entry.Message.Usage
		content, blocks, err := parseClaudeMessageContent(entry.Message.Content)
		if err != nil {
			return Message{}, err
		}
		message.Content = content
		message.Blocks = blocks
	case "user":
		if entry.Message == nil {
			message.Role = "user"
			applyRuntimeTranscriptMetadata(&message, entry.Runtime)
			return message, nil
		}
		content, blocks, err := parseClaudeMessageContent(entry.Message.Content)
		if err != nil {
			return Message{}, err
		}
		message.Role = "user"
		if len(blocks) > 0 && blocks[0].Type == MessageBlockToolResult {
			message.Role = "tool"
		} else if entry.IsCompactSummary && strings.HasPrefix(content, "Summary:") {
			message.Role = "summary"
		}
		message.Content = content
		message.Blocks = blocks
	case "system":
		message.Role = "system"
	case "attachment":
		message.Role = "attachment"
	default:
		message.Role = entry.Type
	}

	applyRuntimeTranscriptMetadata(&message, entry.Runtime)
	return message, nil
}

func claudeTranscriptType(message Message) string {
	switch message.Role {
	case "assistant":
		return "assistant"
	case "system":
		return "system"
	case "attachment":
		return "attachment"
	case "summary":
		return "user"
	case "tool":
		return "user"
	default:
		return "user"
	}
}

func NewClaudeTranscriptMessages(messages []Message, parentUUID *string) []ClaudeTranscriptMessage {
	if len(messages) == 0 {
		return nil
	}
	out := make([]ClaudeTranscriptMessage, 0, len(messages))
	currentParent := parentUUID
	for _, message := range messages {
		entry := NewClaudeTranscriptMessage(message, ClaudeTranscriptOptions{ParentUUID: currentParent})
		out = append(out, entry)
		if IsClaudeTranscriptChainParticipant(entry) {
			nextParent := entry.UUID
			currentParent = &nextParent
		}
	}
	return out
}

func IsClaudeTranscriptMessage(entry ClaudeTranscriptMessage) bool {
	return isClaudeTranscriptType(entry.Type) && entry.UUID != ""
}

func isClaudeTranscriptType(entryType string) bool {
	switch entryType {
	case "user", "assistant", "attachment", "system":
		return true
	default:
		return false
	}
}

func IsClaudeTranscriptChainParticipant(entry ClaudeTranscriptMessage) bool {
	return IsClaudeTranscriptMessage(entry) && entry.Type != "progress"
}

func LatestClaudeTranscriptChain(entries []ClaudeTranscriptMessage) ([]ClaudeTranscriptMessage, error) {
	if len(entries) == 0 {
		return nil, nil
	}
	entries = applyPreservedSegmentRelinks(entries)
	entriesByID := make(map[string]ClaudeTranscriptMessage, len(entries))
	for _, entry := range entries {
		if !IsClaudeTranscriptMessage(entry) {
			continue
		}
		entriesByID[entry.UUID] = entry
	}
	leaf := latestTerminalConversationLeaf(entriesByID, entries)
	if leaf == nil {
		for i := len(entries) - 1; i >= 0; i-- {
			entry := entries[i]
			if !IsClaudeTranscriptMessage(entry) || entry.IsSidechain {
				continue
			}
			leaf = &entry
			break
		}
	}
	if leaf == nil {
		return nil, nil
	}

	var reversed []ClaudeTranscriptMessage
	seen := make(map[string]bool)
	for entry := *leaf; ; {
		if seen[entry.UUID] {
			return nil, fmt.Errorf("cycle in transcript parent chain at %s", entry.UUID)
		}
		seen[entry.UUID] = true
		reversed = append(reversed, entry)
		if entry.ParentUUID == nil || *entry.ParentUUID == "" {
			break
		}
		parent, ok := entriesByID[*entry.ParentUUID]
		if !ok {
			break
		}
		entry = parent
	}
	chain := make([]ClaudeTranscriptMessage, 0, len(reversed))
	for i := len(reversed) - 1; i >= 0; i-- {
		chain = append(chain, reversed[i])
	}
	chain = appendTrailingLegacyCompactBoundary(chain, entries)
	chain = recoverOrphanedParallelToolResults(entriesByID, entries, chain, seen)
	return chain, nil
}

func isClaudeConversationLeaf(entry ClaudeTranscriptMessage) bool {
	return entry.Type == "user" || entry.Type == "assistant"
}

func latestTerminalConversationLeaf(entriesByID map[string]ClaudeTranscriptMessage, entries []ClaudeTranscriptMessage) *ClaudeTranscriptMessage {
	parentUUIDs := make(map[string]bool, len(entries))
	for _, entry := range entries {
		if !IsClaudeTranscriptMessage(entry) || entry.IsSidechain || entry.ParentUUID == nil {
			continue
		}
		parentUUIDs[*entry.ParentUUID] = true
	}
	entryOrder := make(map[string]int, len(entries))
	for i, entry := range entries {
		if entry.UUID != "" {
			entryOrder[entry.UUID] = i
		}
	}
	var leaf *ClaudeTranscriptMessage
	for _, terminal := range entries {
		if !IsClaudeTranscriptMessage(terminal) || terminal.IsSidechain || parentUUIDs[terminal.UUID] {
			continue
		}
		current := terminal
		seen := make(map[string]bool)
		for {
			if seen[current.UUID] {
				break
			}
			seen[current.UUID] = true
			if isClaudeConversationLeaf(current) {
				candidate := current
				if leaf == nil || compareTranscriptLeaf(candidate, *leaf, entryOrder) > 0 {
					leaf = &candidate
				}
				break
			}
			if current.ParentUUID == nil {
				break
			}
			parent, ok := entriesByID[*current.ParentUUID]
			if !ok {
				break
			}
			current = parent
		}
	}
	return leaf
}

func compareTranscriptLeaf(a, b ClaudeTranscriptMessage, order map[string]int) int {
	if a.Timestamp > b.Timestamp {
		return 1
	}
	if a.Timestamp < b.Timestamp {
		return -1
	}
	return order[a.UUID] - order[b.UUID]
}

func appendTrailingLegacyCompactBoundary(chain []ClaudeTranscriptMessage, entries []ClaudeTranscriptMessage) []ClaudeTranscriptMessage {
	if len(chain) == 0 {
		return chain
	}
	tail := chain[len(chain)-1]
	for _, entry := range entries {
		if !IsClaudeTranscriptMessage(entry) || entry.Type != "system" || entry.IsSidechain || entry.ParentUUID == nil {
			continue
		}
		if *entry.ParentUUID != tail.UUID {
			continue
		}
		if entry.Subtype == "compact_boundary" || entry.Content == "[compact_boundary]" || entry.Content == "Conversation compacted" {
			chain = append(chain, entry)
			tail = entry
		}
	}
	return chain
}

func applyPreservedSegmentRelinks(entries []ClaudeTranscriptMessage) []ClaudeTranscriptMessage {
	lastSegBoundaryIdx := -1
	absoluteLastBoundaryIdx := -1
	var lastSeg *CompactPreservedSegment
	for i := range entries {
		entry := entries[i]
		if !isCompactBoundaryEntry(entry) {
			continue
		}
		absoluteLastBoundaryIdx = i
		if entry.CompactMetadata != nil && entry.CompactMetadata.PreservedSegment != nil {
			seg := *entry.CompactMetadata.PreservedSegment
			lastSeg = &seg
			lastSegBoundaryIdx = i
		}
	}
	if lastSeg == nil || absoluteLastBoundaryIdx < 0 {
		return entries
	}

	out := append([]ClaudeTranscriptMessage(nil), entries...)
	indexByID := make(map[string]int, len(out))
	for i, entry := range out {
		if entry.UUID != "" {
			indexByID[entry.UUID] = i
		}
	}

	preserved := make(map[string]bool)
	segIsLive := lastSegBoundaryIdx == absoluteLastBoundaryIdx
	if segIsLive {
		seen := make(map[string]bool)
		currentID := lastSeg.TailID
		reachedHead := false
		for currentID != "" {
			if seen[currentID] {
				break
			}
			seen[currentID] = true
			idx, ok := indexByID[currentID]
			if !ok {
				break
			}
			preserved[currentID] = true
			if currentID == lastSeg.HeadID {
				reachedHead = true
				break
			}
			parent := out[idx].ParentUUID
			if parent == nil {
				break
			}
			currentID = *parent
		}
		if !reachedHead {
			return entries
		}
		if idx, ok := indexByID[lastSeg.HeadID]; ok {
			anchor := lastSeg.AnchorID
			out[idx].ParentUUID = &anchor
		}
		for i := range out {
			if out[i].ParentUUID == nil || *out[i].ParentUUID != lastSeg.AnchorID || out[i].UUID == lastSeg.HeadID {
				continue
			}
			tail := lastSeg.TailID
			out[i].ParentUUID = &tail
		}
		for uuid := range preserved {
			idx, ok := indexByID[uuid]
			if !ok || out[idx].Type != "assistant" || out[idx].Message == nil {
				continue
			}
			out[idx].Message.Usage = map[string]any{
				"input_tokens":                0,
				"output_tokens":               0,
				"cache_creation_input_tokens": 0,
				"cache_read_input_tokens":     0,
			}
		}
	}

	pruned := make([]ClaudeTranscriptMessage, 0, len(out))
	for i, entry := range out {
		if i < absoluteLastBoundaryIdx && !preserved[entry.UUID] {
			continue
		}
		pruned = append(pruned, entry)
	}
	return pruned
}

func isCompactBoundaryEntry(entry ClaudeTranscriptMessage) bool {
	return entry.Type == "system" && (entry.Subtype == "compact_boundary" || entry.Content == "[compact_boundary]" || entry.Content == "Conversation compacted")
}

func recoverOrphanedParallelToolResults(entriesByID map[string]ClaudeTranscriptMessage, entries []ClaudeTranscriptMessage, chain []ClaudeTranscriptMessage, seen map[string]bool) []ClaudeTranscriptMessage {
	if len(chain) == 0 {
		return chain
	}
	anchorByProviderID := make(map[string]ClaudeTranscriptMessage)
	for _, entry := range chain {
		if entry.Type == "assistant" && entry.Message != nil && entry.Message.ID != "" {
			anchorByProviderID[entry.Message.ID] = entry
		}
	}
	if len(anchorByProviderID) == 0 {
		return chain
	}
	siblingsByProviderID := make(map[string][]ClaudeTranscriptMessage)
	toolResultsByAssistant := make(map[string][]ClaudeTranscriptMessage)
	for _, entry := range entries {
		if entry.Type == "assistant" && entry.Message != nil && entry.Message.ID != "" {
			siblingsByProviderID[entry.Message.ID] = append(siblingsByProviderID[entry.Message.ID], entry)
			continue
		}
		if entry.Type == "user" && entry.ParentUUID != nil && claudeEntryHasToolResult(entry) {
			toolResultsByAssistant[*entry.ParentUUID] = append(toolResultsByAssistant[*entry.ParentUUID], entry)
		}
	}

	inserts := make(map[string][]ClaudeTranscriptMessage)
	processed := make(map[string]bool)
	for _, entry := range chain {
		if entry.Type != "assistant" || entry.Message == nil || entry.Message.ID == "" || processed[entry.Message.ID] {
			continue
		}
		providerID := entry.Message.ID
		processed[providerID] = true
		group := siblingsByProviderID[providerID]
		var orphanedSiblings []ClaudeTranscriptMessage
		for _, sibling := range group {
			if !seen[sibling.UUID] {
				orphanedSiblings = append(orphanedSiblings, sibling)
				seen[sibling.UUID] = true
			}
		}
		var orphanedToolResults []ClaudeTranscriptMessage
		for _, sibling := range group {
			for _, toolResult := range toolResultsByAssistant[sibling.UUID] {
				if !seen[toolResult.UUID] {
					orphanedToolResults = append(orphanedToolResults, toolResult)
					seen[toolResult.UUID] = true
				}
			}
		}
		if len(orphanedSiblings) == 0 && len(orphanedToolResults) == 0 {
			continue
		}
		sortTranscriptEntriesByTimestamp(orphanedSiblings)
		sortTranscriptEntriesByTimestamp(orphanedToolResults)
		anchor := anchorByProviderID[providerID]
		if _, ok := entriesByID[anchor.UUID]; ok {
			inserts[anchor.UUID] = append(inserts[anchor.UUID], orphanedSiblings...)
			inserts[anchor.UUID] = append(inserts[anchor.UUID], orphanedToolResults...)
		}
	}
	if len(inserts) == 0 {
		return chain
	}
	out := make([]ClaudeTranscriptMessage, 0, len(chain))
	for _, entry := range chain {
		out = append(out, entry)
		out = append(out, inserts[entry.UUID]...)
	}
	return out
}

func claudeEntryHasToolResult(entry ClaudeTranscriptMessage) bool {
	if entry.Message == nil {
		return false
	}
	_, blocks, err := parseClaudeMessageContent(entry.Message.Content)
	if err != nil {
		return false
	}
	for _, block := range blocks {
		if block.Type == MessageBlockToolResult {
			return true
		}
	}
	return false
}

func sortTranscriptEntriesByTimestamp(entries []ClaudeTranscriptMessage) {
	for i := 1; i < len(entries); i++ {
		for j := i; j > 0 && entries[j].Timestamp < entries[j-1].Timestamp; j-- {
			entries[j], entries[j-1] = entries[j-1], entries[j]
		}
	}
}

func runtimeTranscriptMetadata(message Message, transcriptType string) *RuntimeTranscriptMetadata {
	runtimeRole := ""
	if message.Role != "" && message.Role != transcriptType {
		runtimeRole = message.Role
	}
	runtimeSubtype := ""
	if message.Subtype != "" && transcriptType != "system" && transcriptType != "attachment" {
		runtimeSubtype = message.Subtype
	}
	runtimeContent := ""
	if shouldPreserveRuntimeContent(message, transcriptType) {
		runtimeContent = message.Content
	}
	if runtimeRole == "" && runtimeSubtype == "" && runtimeContent == "" {
		return nil
	}
	return &RuntimeTranscriptMetadata{Role: runtimeRole, Subtype: runtimeSubtype, Content: runtimeContent}
}

func shouldPreserveRuntimeContent(message Message, transcriptType string) bool {
	if message.Content == "" {
		return false
	}
	if message.Role == "tool" {
		return true
	}
	if message.Role == "summary" {
		return true
	}
	if transcriptType == "user" && len(message.Blocks) > 0 && textFromMessageBlocks(message.Blocks) != message.Content {
		return true
	}
	return false
}

func applyRuntimeTranscriptMetadata(message *Message, runtime *RuntimeTranscriptMetadata) {
	if runtime == nil {
		return
	}
	if runtime.Role != "" {
		message.Role = runtime.Role
	}
	if runtime.Subtype != "" {
		message.Subtype = runtime.Subtype
	}
	if runtime.Content != "" {
		message.Content = runtime.Content
	}
}

func RuntimeMessagesFromClaudeTranscriptEntries(entries []ClaudeTranscriptMessage, sessionID string) ([]Message, bool) {
	chain, err := LatestClaudeTranscriptChain(entries)
	if err != nil || len(chain) == 0 {
		return nil, false
	}
	messages := make([]Message, 0, len(chain))
	for _, entry := range chain {
		message, err := MessageFromClaudeTranscript(entry, sessionID)
		if err != nil {
			return nil, false
		}
		messages = append(messages, message)
	}
	return messages, true
}

func claudeAssistantContent(message Message) []MessageBlock {
	if len(message.Blocks) > 0 {
		return append([]MessageBlock(nil), message.Blocks...)
	}
	return []MessageBlock{{Type: MessageBlockText, Text: nonEmptyContent(message.Content)}}
}

func claudeUserContent(message Message) any {
	if len(message.Blocks) > 0 {
		return append([]MessageBlock(nil), message.Blocks...)
	}
	return nonEmptyContent(message.Content)
}

func nonEmptyContent(content string) string {
	if content == "" {
		return "(no content)"
	}
	return content
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if value != "" {
			return value
		}
	}
	return ""
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

func parseClaudeTimestamp(timestamp string) (time.Time, error) {
	if timestamp == "" {
		return time.Time{}, nil
	}
	parsed, err := time.Parse(time.RFC3339Nano, timestamp)
	if err == nil {
		return parsed, nil
	}
	parsed, err = time.Parse(time.RFC3339, timestamp)
	if err == nil {
		return parsed, nil
	}
	return time.Time{}, fmt.Errorf("parse claude transcript timestamp %q: %w", timestamp, err)
}

func parseClaudeMessageContent(content any) (string, []MessageBlock, error) {
	switch value := content.(type) {
	case nil:
		return "", nil, nil
	case string:
		return value, nil, nil
	case []MessageBlock:
		blocks := append([]MessageBlock(nil), value...)
		return textFromMessageBlocks(blocks), blocks, nil
	default:
		data, err := json.Marshal(value)
		if err != nil {
			return "", nil, err
		}
		var contentString string
		if err := json.Unmarshal(data, &contentString); err == nil {
			return contentString, nil, nil
		}
		var blocks []MessageBlock
		if err := json.Unmarshal(data, &blocks); err != nil {
			return "", nil, err
		}
		return textFromMessageBlocks(blocks), blocks, nil
	}
}

func textFromMessageBlocks(blocks []MessageBlock) string {
	var parts []string
	for _, block := range blocks {
		switch block.Type {
		case MessageBlockText:
			if block.Text != "" {
				parts = append(parts, block.Text)
			}
		case MessageBlockThinking:
			if block.Text != "" {
				parts = append(parts, block.Text)
			}
		case MessageBlockToolResult:
			if block.Content != "" {
				parts = append(parts, block.Content)
			}
		case MessageBlockToolUse:
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
