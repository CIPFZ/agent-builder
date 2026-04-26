package tui

import (
	"sort"
	"strings"
	"sync"

	"myclaw/internal/model"
)

type clientStore struct {
	mu              sync.RWMutex
	session         platformStatusSnapshot
	mcp             mcpSnapshot
	tasks           taskPanelSnapshot
	transcript      []transcriptEntry
	events          []string
	diagnostics     diagnosticsState
	activity        activityState
	busy            bool
	approval        *clientApproval
	approvalHistory map[string]approvalView
	lastApprovalID  string
}

type approvalView struct {
	ID           string
	ToolName     string
	ToolInput    string
	Status       string
	Reason       string
	SessionID    string
	RunID        string
	Category     string
	RuleSource   string
	DecisionText string
}

type clientStoreSnapshot struct {
	Transcript  []transcriptEntry
	Events      []string
	Diagnostics diagnosticsState
	Activity    activityState
	Busy        bool
	Approval    *clientApproval
}

func newClientStore() *clientStore {
	return &clientStore{
		transcript:      make([]transcriptEntry, 0, 32),
		events:          []string{"Welcome to myclaw TUI"},
		approvalHistory: make(map[string]approvalView),
	}
}

func (s *clientStore) setSession(snapshot platformStatusSnapshot) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.session = snapshot
}

func (s *clientStore) sessionSnapshot() platformStatusSnapshot {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return clonePlatformStatusSnapshot(s.session)
}

func (s *clientStore) setMCP(snapshot mcpSnapshot) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.mcp = cloneMCPSnapshot(snapshot)
}

func (s *clientStore) mcpSnapshot() mcpSnapshot {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return cloneMCPSnapshot(s.mcp)
}

func (s *clientStore) setTasks(snapshot taskPanelSnapshot) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.tasks = cloneTaskPanelSnapshot(snapshot)
}

func (s *clientStore) taskSnapshot() taskPanelSnapshot {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return cloneTaskPanelSnapshot(s.tasks)
}

func (s *clientStore) snapshot() clientStoreSnapshot {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return clientStoreSnapshot{
		Transcript:  cloneTranscriptEntries(s.transcript),
		Events:      append([]string(nil), s.events...),
		Diagnostics: s.diagnostics,
		Activity:    s.activity,
		Busy:        s.busy,
		Approval:    cloneClientApproval(s.approval),
	}
}

func (s *clientStore) applyEvent(event clientEvent) clientStoreSnapshot {
	s.mu.Lock()
	defer s.mu.Unlock()

	transcriptLen := len(s.transcript)
	s.diagnostics.LastEvent = event.Type
	s.diagnostics.EventCount++
	if event.Type != "" && event.Type != "assistant.delta" {
		s.events = appendBoundedEvent(s.events, event.Type, 200)
	}
	if event.Session.ID != "" && s.diagnostics.SessionID == "" {
		s.diagnostics.SessionID = event.Session.ID
	}

	switch event.Type {
	case "assistant.delta":
		message := ""
		if event.Message != nil {
			message = event.Message.Content
		}
		if message == "" && event.Tool != nil {
			message = event.Tool.ProgressMessage
		}
		if lastFinalAssistantAlreadyContains(s.transcript, message) {
			break
		}
		if len(s.transcript) == 0 || s.transcript[len(s.transcript)-1].Role != "assistant" || !s.transcript[len(s.transcript)-1].Streaming {
			s.transcript = append(s.transcript, transcriptEntry{Role: "assistant", Content: message, Streaming: true})
		} else {
			s.transcript[len(s.transcript)-1].Content += message
		}
	case "message.created":
		if event.Message != nil {
			if entry, ok := specialTranscriptEntryFromClientMessage(*event.Message); ok {
				s.transcript = append(s.transcript, entry)
				break
			}
			switch event.Message.Role {
			case "user":
				if !transcriptHasMessageID(s.transcript, event.Message.ID) && !lastTranscriptMessageMatches(s.transcript, "user", event.Message.Content) {
					s.transcript = append(s.transcript, transcriptEntry{
						MessageID: event.Message.ID,
						Role:      "user",
						Content:   event.Message.Content,
						Blocks:    cloneClientMessageBlocks(event.Message.Blocks),
					})
				}
			case "assistant":
				if len(s.transcript) > 0 && s.transcript[len(s.transcript)-1].Role == "assistant" && s.transcript[len(s.transcript)-1].Streaming {
					s.transcript[len(s.transcript)-1].MessageID = event.Message.ID
					s.transcript[len(s.transcript)-1].Content = event.Message.Content
					s.transcript[len(s.transcript)-1].Streaming = false
					s.transcript[len(s.transcript)-1].Blocks = cloneClientMessageBlocks(event.Message.Blocks)
				} else if !transcriptHasMessageID(s.transcript, event.Message.ID) && !transcriptMessageExistsSinceLastUser(s.transcript, "assistant", event.Message.Content) {
					s.transcript = append(s.transcript, transcriptEntry{
						MessageID: event.Message.ID,
						Role:      "assistant",
						Content:   event.Message.Content,
						Blocks:    cloneClientMessageBlocks(event.Message.Blocks),
					})
				}
				s.busy = false
				s.activity.Label = "Idle"
			case "tool":
				if entry, ok := transcriptEntryFromClientMessage(*event.Message); ok {
					s.transcript = append(s.transcript, entry)
				} else {
					s.transcript = append(s.transcript, transcriptEntry{Role: "tool", Content: event.Message.Content})
				}
			case "system":
				s.transcript = append(s.transcript, transcriptEntry{Kind: messageKindSystem, Role: "system", Content: event.Message.Content})
			}
		}
	case "tool.called":
		s.applyToolCalledLocked(event)
	case "tool.progress":
		s.applyToolProgressLocked(event.Tool)
	case "tool.result":
		if event.Tool != nil && strings.TrimSpace(event.Tool.ToolName) != "" {
			s.activity.Label = "Tool finished: " + event.Tool.ToolName
		}
		s.applyToolResultLocked(event)
	case "permission.required":
		s.approval = cloneClientApproval(nil)
		if event.Tool != nil {
			s.approval = cloneClientApproval(event.Tool.Approval)
		}
		if s.approval != nil {
			s.recordApprovalLocked(*s.approval)
			s.activity.Label = "Awaiting approval: " + strings.TrimSpace(s.approval.ToolName+" "+s.approval.ToolInput)
		}
		s.busy = false
	case "approval.updated":
		if event.Tool != nil && event.Tool.Approval != nil {
			s.recordApprovalLocked(*event.Tool.Approval)
		}
		s.approval = nil
		s.activity.Label = "Idle"
	case "run.error":
		if event.Error != "" {
			s.diagnostics.LastError = event.Error
			s.activity.Label = "Run error"
			if !lastTranscriptSpecialMessageMatches(s.transcript, messageKindError, event.Error) {
				s.transcript = append(s.transcript, transcriptEntry{Kind: messageKindError, Role: "system", Content: event.Error})
			}
			s.busy = false
		}
	case "compact.boundary":
		s.transcript = append(s.transcript, transcriptEntry{Kind: messageKindCompact, Role: "system", Content: "Conversation compacted"})
	case "compact.cleaned":
		s.transcript = append(s.transcript, transcriptEntry{Kind: messageKindCompact, Role: "system", Content: "Compaction cleanup completed"})
	case "compact.memory_saved":
		s.transcript = append(s.transcript, transcriptEntry{Kind: messageKindCompact, Role: "system", Content: "Session memory saved"})
	case "agent.lifecycle.start":
		s.busy = true
		if s.activity.Label == "" {
			s.activity.Label = "Running turn"
		}
	case "agent.lifecycle.end":
		s.busy = false
		s.activity.Label = "Idle"
	case "subagent.updated", "subagent.completed", "orchestration.updated", "orchestration.plan_step.updated":
		s.applyTaskEventLocked(event)
	}

	_ = transcriptLen
	return clientStoreSnapshot{
		Transcript:  cloneTranscriptEntries(s.transcript),
		Events:      append([]string(nil), s.events...),
		Diagnostics: s.diagnostics,
		Activity:    s.activity,
		Busy:        s.busy,
		Approval:    cloneClientApproval(s.approval),
	}
}

func (s *clientStore) applyApproval(view approvalView) {
	if strings.TrimSpace(view.ID) == "" {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.approvalHistory[view.ID] = view
	s.lastApprovalID = view.ID
}

func (s *clientStore) latestApproval() (approvalView, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if s.lastApprovalID == "" {
		return approvalView{}, false
	}
	view, ok := s.approvalHistory[s.lastApprovalID]
	return view, ok
}

func (s *clientStore) clearApproval(id string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.approval != nil && s.approval.ID == id {
		s.approval = nil
	}
}

func (s *clientStore) appendUserMessage(text string) clientStoreSnapshot {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.transcript = append(s.transcript, transcriptEntry{Role: "user", Content: text})
	s.busy = true
	s.activity.Label = "Running turn"
	return clientStoreSnapshot{
		Transcript:  cloneTranscriptEntries(s.transcript),
		Events:      append([]string(nil), s.events...),
		Diagnostics: s.diagnostics,
		Activity:    s.activity,
		Busy:        s.busy,
		Approval:    cloneClientApproval(s.approval),
	}
}

func lastTranscriptMessageMatches(entries []transcriptEntry, role, content string) bool {
	if len(entries) == 0 {
		return false
	}
	last := entries[len(entries)-1]
	return last.Kind == "" && last.Role == role && strings.TrimSpace(last.Content) == strings.TrimSpace(content)
}

func lastTranscriptSpecialMessageMatches(entries []transcriptEntry, kind, content string) bool {
	if len(entries) == 0 {
		return false
	}
	last := entries[len(entries)-1]
	return last.Kind == kind && strings.TrimSpace(last.Content) == strings.TrimSpace(content)
}

func transcriptHasMessageID(entries []transcriptEntry, id string) bool {
	id = strings.TrimSpace(id)
	if id == "" {
		return false
	}
	for _, entry := range entries {
		if entry.MessageID == id {
			return true
		}
	}
	return false
}

func transcriptMessageExistsSinceLastUser(entries []transcriptEntry, role, content string) bool {
	content = strings.TrimSpace(content)
	for i := len(entries) - 1; i >= 0; i-- {
		entry := entries[i]
		if entry.Role == "user" && entry.Kind == "" {
			return false
		}
		if entry.Kind == "" && entry.Role == role && strings.TrimSpace(entry.Content) == content {
			return true
		}
	}
	return false
}

func lastFinalAssistantAlreadyContains(entries []transcriptEntry, delta string) bool {
	delta = strings.TrimSpace(delta)
	if delta == "" || len(entries) == 0 {
		return false
	}
	last := entries[len(entries)-1]
	return last.Kind == "" && last.Role == "assistant" && !last.Streaming && strings.Contains(strings.TrimSpace(last.Content), delta)
}

func (s *clientStore) applyBridgeError(err error) clientStoreSnapshot {
	s.mu.Lock()
	defer s.mu.Unlock()
	if err != nil {
		s.events = append(s.events, "error: "+err.Error())
		s.busy = false
		s.diagnostics.LastError = err.Error()
	}
	return clientStoreSnapshot{
		Transcript:  cloneTranscriptEntries(s.transcript),
		Events:      append([]string(nil), s.events...),
		Diagnostics: s.diagnostics,
		Activity:    s.activity,
		Busy:        s.busy,
		Approval:    cloneClientApproval(s.approval),
	}
}

func (s *clientStore) applyToolCalledLocked(event clientEvent) {
	if event.Tool == nil {
		return
	}
	entry := transcriptEntry{
		Role:            "tool",
		ToolUseID:       toolEventID(event),
		ToolName:        event.Tool.ToolName,
		ToolInput:       event.Tool.ToolInput,
		ToolInputObject: cloneAnyMap(event.Tool.ToolInputObject),
		ToolStatus:      toolStatusRunning,
		Content:         "Running " + event.Tool.ToolName + "...",
	}
	s.activity.Label = "Running tool: " + strings.TrimSpace(event.Tool.ToolName+" "+event.Tool.ToolInput)
	if index := findToolEntryIndexIn(s.transcript, entry.ToolUseID, entry.ToolName, entry.ToolInput, true); index >= 0 {
		s.transcript[index] = entry
		return
	}
	s.transcript = append(s.transcript, entry)
}

func (s *clientStore) applyToolProgressLocked(progress *clientToolEvent) {
	if progress == nil {
		return
	}
	index := findToolEntryIndexIn(s.transcript, progress.ToolUseID, "", "", true)
	if index < 0 {
		s.transcript = append(s.transcript, transcriptEntry{
			Role:                "tool",
			ToolUseID:           strings.TrimSpace(progress.ToolUseID),
			ToolStatus:          toolStatusRunning,
			ToolProgressType:    progress.ProgressType,
			ToolProgressMessage: progress.ProgressMessage,
			ToolProgressOutput:  progressOutput(progress),
		})
		return
	}
	entry := &s.transcript[index]
	entry.ToolProgressType = progress.ProgressType
	entry.ToolProgressMessage = progress.ProgressMessage
	if output := progressOutput(progress); output != "" {
		entry.ToolProgressOutput = output
	}
	if entry.ToolStatus == "" || entry.ToolStatus == "called" {
		entry.ToolStatus = toolStatusRunning
	}
}

func (s *clientStore) applyToolResultLocked(event clientEvent) {
	if event.Tool == nil {
		return
	}
	toolUseID, resultContent, isError := toolResultFromEvent(event)
	if toolUseID == "" {
		toolUseID = toolEventID(event)
	}
	status := toolStatusSucceeded
	if isError {
		status = toolStatusFailed
	}
	index := findToolEntryIndexIn(s.transcript, toolUseID, event.Tool.ToolName, event.Tool.ToolInput, true)
	if index < 0 {
		s.transcript = append(s.transcript, transcriptEntry{
			Role:            "tool",
			ToolUseID:       toolUseID,
			ToolName:        event.Tool.ToolName,
			ToolInput:       event.Tool.ToolInput,
			ToolInputObject: cloneAnyMap(event.Tool.ToolInputObject),
			ToolStatus:      status,
			ToolError:       isError,
			Content:         resultContent,
		})
		return
	}
	entry := &s.transcript[index]
	entry.ToolName = valueOrDefault(event.Tool.ToolName, entry.ToolName)
	if event.Tool.ToolInput != "" {
		entry.ToolInput = event.Tool.ToolInput
	}
	if event.Tool.ToolInputObject != nil {
		entry.ToolInputObject = cloneAnyMap(event.Tool.ToolInputObject)
	}
	if toolUseID != "" {
		entry.ToolUseID = toolUseID
	}
	entry.ToolStatus = status
	entry.ToolError = isError
	entry.Content = resultContent
}

func (s *clientStore) applyTaskEventLocked(event clientEvent) {
	var update *clientTaskUpdate
	if event.Tool != nil {
		if event.Tool.SubagentUpdate != nil {
			update = event.Tool.SubagentUpdate
		}
		if update == nil && event.Tool.OrchestrationRun != nil {
			update = event.Tool.OrchestrationRun
		}
	}
	if update == nil || strings.TrimSpace(update.RunID) == "" {
		return
	}

	index := -1
	for i := range s.tasks.Tasks {
		if s.tasks.Tasks[i].RunID == update.RunID {
			index = i
			break
		}
	}
	task := taskSnapshot{
		RunID:             update.RunID,
		Label:             update.Label,
		Status:            update.Status,
		ParentSessionID:   update.ParentSessionID,
		ChildSessionID:    update.ChildSessionID,
		ChildSessionKey:   update.ChildSessionKey,
		Output:            update.Output,
		Error:             update.Error,
		LastEvent:         update.LastEvent,
		Message:           update.Message,
		NextAction:        update.NextAction,
		RecommendedRole:   update.RecommendedRole,
		RecommendedAction: update.RecommendedAction,
		DecisionPriority:  update.DecisionPriority,
		DecisionReason:    update.DecisionReason,
		AutoExecutable:    update.AutoExecutable,
	}
	if index >= 0 {
		task = mergeTaskSnapshot(s.tasks.Tasks[index], task)
		s.tasks.Tasks[index] = task
	} else {
		s.tasks.Tasks = append(s.tasks.Tasks, task)
	}
	s.recountTasksLocked()
}

func (s *clientStore) recountTasksLocked() {
	s.tasks.RunningCount = 0
	s.tasks.CompletedCount = 0
	s.tasks.FailedCount = 0
	s.tasks.StoppedCount = 0
	for _, task := range s.tasks.Tasks {
		switch task.Status {
		case "running":
			s.tasks.RunningCount++
		case "completed":
			s.tasks.CompletedCount++
		case "failed":
			s.tasks.FailedCount++
		case "stopped":
			s.tasks.StoppedCount++
		}
	}
	sort.SliceStable(s.tasks.Tasks, func(i, j int) bool {
		return s.tasks.Tasks[i].RunID < s.tasks.Tasks[j].RunID
	})
}

func (s *clientStore) recordApprovalLocked(approval clientApproval) {
	s.approvalHistory[approval.ID] = approvalView{
		ID:           approval.ID,
		ToolName:     approval.ToolName,
		ToolInput:    approval.ToolInput,
		Status:       approval.Status,
		Reason:       approval.Reason,
		SessionID:    approval.SessionID,
		RunID:        approval.RunID,
		Category:     approval.Category,
		RuleSource:   approval.RuleSource,
		DecisionText: approval.DecisionReason,
	}
	s.lastApprovalID = approval.ID
}

func findToolEntryIndexIn(entries []transcriptEntry, toolUseID, toolName, toolInput string, runningOnly bool) int {
	toolUseID = strings.TrimSpace(toolUseID)
	if toolUseID != "" {
		for i := len(entries) - 1; i >= 0; i-- {
			entry := entries[i]
			if entry.Role == "tool" && entry.ToolUseID == toolUseID && (!runningOnly || isRunningToolStatus(entry.ToolStatus)) {
				return i
			}
		}
	}
	for i := len(entries) - 1; i >= 0; i-- {
		entry := entries[i]
		if entry.Role != "tool" || (runningOnly && !isRunningToolStatus(entry.ToolStatus)) {
			continue
		}
		if toolName != "" && entry.ToolName != toolName {
			continue
		}
		if toolInput != "" && entry.ToolInput != toolInput {
			continue
		}
		return i
	}
	return -1
}

func mergeTaskSnapshot(base, update taskSnapshot) taskSnapshot {
	if update.Label != "" {
		base.Label = update.Label
	}
	if update.Status != "" {
		base.Status = update.Status
	}
	if update.ParentSessionID != "" {
		base.ParentSessionID = update.ParentSessionID
	}
	if update.ChildSessionID != "" {
		base.ChildSessionID = update.ChildSessionID
	}
	if update.ChildSessionKey != "" {
		base.ChildSessionKey = update.ChildSessionKey
	}
	if update.Output != "" {
		base.Output = update.Output
	}
	if update.Error != "" {
		base.Error = update.Error
	}
	if update.LastEvent != "" {
		base.LastEvent = update.LastEvent
	}
	if update.Message != "" {
		base.Message = update.Message
	}
	if update.NextAction != "" {
		base.NextAction = update.NextAction
	}
	if update.RecommendedRole != "" {
		base.RecommendedRole = update.RecommendedRole
	}
	if update.RecommendedAction != "" {
		base.RecommendedAction = update.RecommendedAction
	}
	if update.DecisionPriority != "" {
		base.DecisionPriority = update.DecisionPriority
	}
	if update.DecisionReason != "" {
		base.DecisionReason = update.DecisionReason
	}
	base.AutoExecutable = update.AutoExecutable
	return base
}

func clonePlatformStatusSnapshot(snapshot platformStatusSnapshot) platformStatusSnapshot {
	snapshot.WorkspaceRoots = append([]string(nil), snapshot.WorkspaceRoots...)
	snapshot.AvailableModels = append([]platformModelOption(nil), snapshot.AvailableModels...)
	return snapshot
}

func cloneMCPSnapshot(snapshot mcpSnapshot) mcpSnapshot {
	out := mcpSnapshot{Servers: make([]mcpServerSnapshot, 0, len(snapshot.Servers))}
	for _, server := range snapshot.Servers {
		out.Servers = append(out.Servers, mcpServerSnapshot{
			Name:          server.Name,
			TransportType: server.TransportType,
			Endpoint:      server.Endpoint,
			Enabled:       server.Enabled,
			Tools:         append([]string(nil), server.Tools...),
			Prompts:       append([]string(nil), server.Prompts...),
			Resources:     append([]string(nil), server.Resources...),
		})
	}
	return out
}

func cloneTaskPanelSnapshot(snapshot taskPanelSnapshot) taskPanelSnapshot {
	out := snapshot
	out.Tasks = append([]taskSnapshot(nil), snapshot.Tasks...)
	return out
}

func cloneTranscriptEntries(entries []transcriptEntry) []transcriptEntry {
	out := make([]transcriptEntry, 0, len(entries))
	for _, entry := range entries {
		cloned := entry
		cloned.Blocks = cloneModelMessageBlocks(entry.Blocks)
		cloned.ToolInputObject = cloneAnyMap(entry.ToolInputObject)
		out = append(out, cloned)
	}
	return out
}

func cloneModelMessageBlocks(blocks []model.MessageBlock) []model.MessageBlock {
	if len(blocks) == 0 {
		return nil
	}
	out := make([]model.MessageBlock, 0, len(blocks))
	for _, block := range blocks {
		cloned := block
		cloned.InputObject = cloneAnyMap(block.InputObject)
		if block.Raw != nil {
			cloned.Raw = cloneAnyMap(block.Raw)
		}
		out = append(out, cloned)
	}
	return out
}

func cloneClientApproval(approval *clientApproval) *clientApproval {
	if approval == nil {
		return nil
	}
	cloned := *approval
	cloned.ToolInputObject = cloneAnyMap(approval.ToolInputObject)
	return &cloned
}
