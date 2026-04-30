package queryengine

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"myclaw/internal/compaction"
	"myclaw/internal/llm"
	"myclaw/internal/memory"
	"myclaw/internal/prompt"
	"myclaw/internal/session"
	"myclaw/internal/tools"
	"myclaw/internal/workspace"
	"path/filepath"
	"strings"
	"time"
)

func (q *QueryEngine) runModelPass(ctx context.Context, sess session.Session, userMessage session.Message, runID string, sink EventSink) (*textStreamCollector, error) {
	q.ensureMutableMessages(sess.ID)
	q.ensureSessionModelMetadata(sess.ID)
	history := q.Messages(sess.ID)
	if compactor := q.compactorForSession(sess.ID); compactor != nil {
		analysis := compactor.Analyze(history)
		q.recordCompactionAnalysis(analysis)
		if analysis.IsAboveWarningThreshold {
			q.recordCompactionPhase("warning")
			if err := q.emit(sink, Event{
				Type:    "compact.warning",
				Session: sess,
				RunID:   runID,
			}); err != nil {
				return nil, err
			}
			micro := compactor.Microcompact(history)
			if micro.Changed {
				if err := q.sessions.ReplaceMessages(sess.ID, micro.Messages); err != nil {
					return nil, err
				}
				q.replaceMutableMessages(sess.ID, micro.Messages)
				history = micro.Messages
				q.recordCompactionResult(micro)
				if err := q.emit(sink, Event{
					Type:    "compact.micro",
					Session: sess,
					RunID:   runID,
				}); err != nil {
					return nil, err
				}
			}
		}
		if analysis.IsAboveErrorThreshold {
			q.recordCompactionPhase("error")
			if err := q.emit(sink, Event{
				Type:    "compact.error",
				Session: sess,
				RunID:   runID,
			}); err != nil {
				return nil, err
			}
		}
		compactOptions, err := q.sessionMemoryCompactOptions(ctx, sess, analysis)
		if err != nil {
			return nil, err
		}
		result := compactor.CompactWithSessionMemoryOptions(history, q.latestSummaryMemory(sess.ID), q.lastSummarizedMessageID(sess.ID), compactOptions)
		q.recordCompactionResult(result)
		if analysis.IsAboveAutoCompactThreshold && result.Changed {
			q.recordCompactionPhase("auto")
			if err := q.emit(sink, Event{
				Type:    "compact.auto",
				Session: sess,
				RunID:   runID,
			}); err != nil {
				return nil, err
			}
		}
		if analysis.IsAtBlockingLimit && !result.Changed {
			q.recordCompactionPhase("blocked")
			if err := q.emit(sink, Event{
				Type:    "compact.blocked",
				Session: sess,
				RunID:   runID,
			}); err != nil {
				return nil, err
			}
			return nil, fmt.Errorf("context window blocking limit reached")
		}
		if result.Changed {
			compactedWithBoundary := cloneSessionMessages(result.Messages)
			var boundary session.Message
			if result.BoundaryMessage != nil {
				boundary = *result.BoundaryMessage
			} else {
				boundary = q.newCompactBoundary(sess.ID)
				compactedWithBoundary = append(compactedWithBoundary, boundary)
			}
			if err := q.sessions.ReplaceMessages(sess.ID, compactedWithBoundary); err != nil {
				return nil, err
			}
			q.replaceMutableMessages(sess.ID, compactedWithBoundary)
			history = compactedWithBoundary
			q.recordCompactBoundary(boundary)
			_ = q.sessions.UpdateMetadata(sess.ID, func(metadata *session.SessionMetadata) {
				metadata.LastCompactBoundaryID = boundary.ID
				metadata.LastCompactionReason = string(result.Reason)
				metadata.LastCompactedAt = boundary.CreatedAt
				if result.SummaryMessage != nil {
					metadata.LastCompactionSummaryID = result.SummaryMessage.ID
				}
				if result.SummarizedThroughID != "" {
					metadata.LastSummarizedMessageID = result.SummarizedThroughID
				}
			})
			if err := q.emit(sink, Event{
				Type:    "compact.boundary",
				Session: sess,
				RunID:   runID,
				Message: &boundary,
			}); err != nil {
				return nil, err
			}
			if q.snipReplay != nil {
				if replay := q.snipReplay(boundary, q.Messages(sess.ID)); replay != nil && replay.Executed {
					if err := q.sessions.ReplaceMessages(sess.ID, replay.Messages); err != nil {
						return nil, err
					}
					q.replaceMutableMessages(sess.ID, replay.Messages)
					q.recordCompactionReplay(len(replay.Messages))
					if err := q.emit(sink, Event{
						Type:    "compact.replayed",
						Session: sess,
						RunID:   runID,
					}); err != nil {
						return nil, err
					}
					history = q.Messages(sess.ID)
				}
			}
			if result.SummaryMessage != nil && q.memory != nil {
				summary := *result.SummaryMessage
				if items, saved := q.memory.SaveCompactionSummary(sess, summary); saved {
					q.persistMemoryItems(sess.ID, items)
					q.recordCompactionMemorySaved(summary.ID)
					_ = q.emit(sink, Event{Type: "compact.memory_saved", Session: sess, RunID: runID, Message: &summary})
					_ = q.emit(sink, Event{Type: "memory.saved", Session: sess, RunID: runID, Message: &summary})
				}
			}
			if q.postCompactCleanup != nil {
				if cleanup := q.postCompactCleanup(boundary, q.Messages(sess.ID)); cleanup != nil && cleanup.Executed {
					if err := q.sessions.ReplaceMessages(sess.ID, cleanup.Messages); err != nil {
						return nil, err
					}
					q.replaceMutableMessages(sess.ID, cleanup.Messages)
					q.recordCompactionCleanup(len(cleanup.Messages))
					if err := q.emit(sink, Event{
						Type:    "compact.cleaned",
						Session: sess,
						RunID:   runID,
					}); err != nil {
						return nil, err
					}
					history = q.Messages(sess.ID)
				}
			}
		}
	}

	workspaceContext, err := q.workspaceContextForSession(sess)
	if err != nil {
		return nil, err
	}
	contextBuildInput := prompt.BuildInput{
		Session:                 sess,
		History:                 history,
		UserMessage:             userMessage,
		DefaultSystemPrompt:     q.defaultSystemPrompt,
		CustomSystemPrompt:      q.customSystemPrompt,
		AgentSystemPrompt:       q.agentSystemPromptForSession(sess, workspaceContext),
		CoordinatorSystemPrompt: q.coordinatorSystemPrompt,
		ProactiveAgentPrompt:    q.proactiveAgentPrompt,
		AppendSystemPrompt:      q.appendSystemPrompt,
		OverrideSystemPrompt:    q.overrideSystemPrompt,
		UserContextLines:        q.userContextLines(sess, workspaceContext),
		SystemContextLines:      q.systemContextLines(sess, workspaceContext),
		WorkspaceContext:        workspaceContext,
		Tools:                   q.exposedTools(sess.ID, false),
		SessionMemories:         q.memoryLines(sess.ID),
		SessionMemoryItems:      q.memoryItems(sess.ID),
	}
	contextInput, cacheState := q.contextCache.Build(contextBuildInput)
	q.persistContextCacheState(sess.ID, workspaceContext, history, contextBuildInput, cacheState)
	exposedTools := q.exposedTools(sess.ID, false)
	stream := &textStreamCollector{
		sink:           sink,
		session:        sess,
		runID:          runID,
		onDelta:        q.recordAssistantDelta,
		onMessageEnd:   q.clearActiveAssistantText,
		onStreamEvent:  q.recordStreamEvent,
		includePartial: q.includePartialStreamEvents,
	}
	if err := q.emit(sink, Event{
		Type:    "model.request.start",
		Session: sess,
		RunID:   runID,
	}); err != nil {
		return nil, err
	}
	if err := q.client.Stream(ctx, llm.GenerateRequest{
		Session:     sess,
		UserMessage: userMessage,
		History:     history,
		Context:     contextInput,
		Model:       q.mainLoopModelForSessionWithHistory(sess.ID, history),
		Tools:       llmToolDefinitions(exposedTools),
	}, stream); err != nil {
		return nil, err
	}
	if err := q.emit(sink, Event{
		Type:    "model.request.end",
		Session: sess,
		RunID:   runID,
	}); err != nil {
		return nil, err
	}
	return stream, nil
}

func llmToolDefinitions(defs []tools.Definition) []llm.ToolDefinition {
	if len(defs) == 0 {
		return nil
	}
	out := make([]llm.ToolDefinition, 0, len(defs))
	for _, def := range defs {
		if strings.TrimSpace(def.Name) == "" {
			continue
		}
		out = append(out, llm.ToolDefinition{
			Name:        def.Name,
			Description: def.Description,
			InputSchema: cloneAnyMap(def.InputSchema),
		})
	}
	return out
}

func (q *QueryEngine) memoryLines(sessionID string) []string {
	if q.memory == nil {
		return nil
	}
	items := q.memory.List(sessionID)
	if len(items) == 0 {
		if sess, ok := q.sessions.GetByID(sessionID); ok {
			q.memory.RecoverSession(sess)
			items = q.memory.List(sessionID)
		}
	}
	lines := make([]string, 0, len(items))
	for _, item := range items {
		lines = append(lines, item.Content)
	}
	return lines
}

func (q *QueryEngine) memoryItems(sessionID string) []memory.Item {
	if q.memory == nil {
		return nil
	}
	items := q.memory.List(sessionID)
	if len(items) == 0 {
		if sess, ok := q.sessions.GetByID(sessionID); ok {
			q.memory.RecoverSession(sess)
			items = q.memory.List(sessionID)
		}
	}
	return items
}

func (q *QueryEngine) workspaceContextForSession(sess session.Session) (workspace.Context, error) {
	if q.workspace == nil {
		return workspace.Context{}, nil
	}
	if root := strings.TrimSpace(sess.Metadata.AgentWorktreePath); root != "" {
		return q.workspace.WithRoot(root).Load()
	}
	if root := strings.TrimSpace(sess.Metadata.AgentCWD); root != "" {
		return q.workspace.WithRoot(root).Load()
	}
	return q.workspace.Load()
}

func (q *QueryEngine) agentSystemPromptForSession(sess session.Session, workspaceContext workspace.Context) string {
	agentPrompt := strings.TrimSpace(q.agentSystemPrompt)
	if override := strings.TrimSpace(sess.Metadata.AgentSystemPrompt); override != "" {
		agentPrompt = override
	}
	if agentMemoryPrompt := q.agentMemoryPromptForSession(sess, workspaceContext); agentMemoryPrompt != "" {
		if agentPrompt == "" {
			return agentMemoryPrompt
		}
		return agentPrompt + "\n\n" + agentMemoryPrompt
	}
	return agentPrompt
}

func (q *QueryEngine) agentMemoryPromptForSession(sess session.Session, workspaceContext workspace.Context) string {
	if q.memory == nil {
		return ""
	}
	agentType := strings.TrimSpace(sess.Metadata.AgentType)
	scopeValue := strings.TrimSpace(sess.Metadata.AgentMemoryScope)
	if agentType == "" || scopeValue == "" {
		return ""
	}
	scope := memory.AgentMemoryScope(scopeValue)
	items := q.memory.ListAgent(memory.AgentMemoryRef{
		AgentType: agentType,
		Scope:     scope,
		Namespace: agentMemoryNamespace(scope, workspaceContext.Root, sess.Key),
	})
	return memory.BuildAgentMemoryPrompt(scope, items)
}

func agentMemoryNamespace(scope memory.AgentMemoryScope, workspaceRoot, sessionKey string) string {
	switch scope {
	case memory.AgentMemoryScopeUser:
		return "user"
	case memory.AgentMemoryScopeProject:
		if strings.TrimSpace(workspaceRoot) != "" {
			return workspaceRoot
		}
		return sessionKey
	case memory.AgentMemoryScopeLocal:
		if strings.TrimSpace(workspaceRoot) != "" {
			return workspaceRoot
		}
		return sessionKey
	default:
		return ""
	}
}

func (q *QueryEngine) sessionMemoryCompactOptions(ctx context.Context, sess session.Session, analysis compaction.Analysis) (compaction.SessionMemoryOptions, error) {
	var hookMessages []session.Message
	if q.sessionStartCompactHook != nil {
		messages, err := q.sessionStartCompactHook.ProcessSessionStartCompact(ctx, sess)
		if err != nil {
			return compaction.SessionMemoryOptions{}, err
		}
		hookMessages = append([]session.Message(nil), messages...)
	}
	transcriptPath := ""
	if q.transcriptPathProvider != nil {
		transcriptPath = q.transcriptPathProvider(sess)
	}
	threshold := 0
	if analysis.IsAboveAutoCompactThreshold {
		threshold = analysis.AutoCompactThreshold
	}
	return compaction.SessionMemoryOptions{
		HookMessages:         hookMessages,
		TranscriptPath:       transcriptPath,
		AutoCompactThreshold: threshold,
		InvokedSkills:        q.invokedSkillsForSession(sess),
	}, nil
}

func (q *QueryEngine) invokedSkillsForSession(sess session.Session) []tools.InvokedSkillInfo {
	q.toolContextMu.Lock()
	defer q.toolContextMu.Unlock()
	return tools.GetInvokedSkillsForAgent(q.toolAppStates[sess.ID], sess.AgentID)
}

func (q *QueryEngine) latestSummaryMemory(sessionID string) string {
	if q.memory == nil {
		return ""
	}
	items := q.memory.List(sessionID)
	for i := len(items) - 1; i >= 0; i-- {
		if items[i].Type == memory.TypeSummary {
			return items[i].Content
		}
	}
	if messages, ok := q.sessions.Messages(sessionID); ok {
		for i := len(messages) - 1; i >= 0; i-- {
			message := messages[i]
			if message.Role == "summary" || (message.Role == "user" && message.IsCompactSummary) {
				if strings.TrimSpace(message.Content) != "" {
					return message.Content
				}
			}
		}
	}
	return ""
}

func (q *QueryEngine) lastSummarizedMessageID(sessionID string) string {
	sess, ok := q.sessions.GetByID(sessionID)
	if !ok {
		return ""
	}
	return strings.TrimSpace(sess.Metadata.LastSummarizedMessageID)
}

func (q *QueryEngine) ensureSessionModelMetadata(sessionID string) {
	baseModel := parseUserSpecifiedMainLoopModel(q.mainLoopModel, q.llmProvider)
	if baseModel == "" {
		return
	}
	_ = q.sessions.UpdateMetadata(sessionID, func(metadata *session.SessionMetadata) {
		if strings.TrimSpace(metadata.InitialMainLoopModel) == "" {
			metadata.InitialMainLoopModel = baseModel
		}
	})
}

func (q *QueryEngine) mainLoopModelForSession(sessionID string) string {
	history, _ := q.sessions.Messages(sessionID)
	return q.mainLoopModelForSessionWithHistory(sessionID, history)
}

func (q *QueryEngine) mainLoopModelForSessionWithHistory(sessionID string, history []session.Message) string {
	policy := q.PermissionPolicyForSession(sessionID)
	exceeds200kTokens := estimateMessagesTokens(history) > 200000
	sess, ok := q.sessions.GetByID(sessionID)
	if ok {
		if override := strings.TrimSpace(sess.Metadata.MainLoopModelOverride); override != "" {
			return resolveRuntimeMainLoopModelWithProviderAndContext(override, policy, q.llmProvider, exceeds200kTokens)
		}
	}
	return resolveRuntimeMainLoopModelWithProviderAndContext(strings.TrimSpace(q.mainLoopModel), policy, q.llmProvider, exceeds200kTokens)
}

func (q *QueryEngine) userContextLines(sess session.Session, workspaceContext workspace.Context) []string {
	if q.userContextProvider == nil {
		return nil
	}
	return q.userContextProvider.Lines(sess, workspaceContext)
}

func (q *QueryEngine) systemContextLines(sess session.Session, workspaceContext workspace.Context) []string {
	if q.systemContextProvider == nil {
		return q.readFileContextLines(sess)
	}
	lines := q.systemContextProvider.Lines(sess, workspaceContext, q.PermissionPolicyForSession(sess.ID))
	lines = append(lines, q.readFileContextLines(sess)...)
	return lines
}

func (q *QueryEngine) readFileContextLines(sess session.Session) []string {
	current, ok := q.sessions.GetByID(sess.ID)
	if ok {
		sess = current
	}
	if len(sess.Metadata.ReadFiles) == 0 {
		return nil
	}
	lines := make([]string, 0, len(sess.Metadata.ReadFiles))
	for _, item := range sess.Metadata.ReadFiles {
		fresh, err := readFileMetadata(item.Path, item.ToolUseID)
		if err != nil {
			lines = append(lines, "read_file_stale="+filepath.ToSlash(item.Path)+": "+err.Error())
			continue
		}
		if item.Hash != "" && fresh.Hash != item.Hash {
			lines = append(lines, "read_file_stale="+filepath.ToSlash(item.Path)+": content changed")
			continue
		}
		lines = append(lines, fmt.Sprintf("read_file=%s hash=%s size=%d", filepath.ToSlash(item.Path), fresh.Hash, fresh.Size))
	}
	return lines
}

func (q *QueryEngine) persistContextCacheState(sessionID string, workspaceContext workspace.Context, history []session.Message, input prompt.BuildInput, state prompt.CacheState) {
	if strings.TrimSpace(state.Key) == "" {
		return
	}
	now := time.Now().UTC()
	_ = q.sessions.UpdateMetadata(sessionID, func(metadata *session.SessionMetadata) {
		metadata.ContextCache.Key = state.Key
		metadata.ContextCache.WorkspaceHash = workspaceContext.Fingerprint
		metadata.ContextCache.HistoryHash = hashMessages(history)
		metadata.ContextCache.MemoryHash = hashMemoryItems(input.SessionMemoryItems)
		if state.Hit {
			metadata.ContextCache.LastCacheHitAt = now
		} else {
			metadata.ContextCache.LastRebuiltAt = now
		}
	})
}

func hashMessages(messages []session.Message) string {
	h := sha256.New()
	for _, msg := range messages {
		h.Write([]byte(msg.ID))
		h.Write([]byte{0})
		h.Write([]byte(msg.Role))
		h.Write([]byte{0})
		h.Write([]byte(msg.Content))
		h.Write([]byte{0})
	}
	return hex.EncodeToString(h.Sum(nil))
}

func hashMemoryItems(items []memory.Item) string {
	h := sha256.New()
	for _, item := range items {
		h.Write([]byte(item.ID))
		h.Write([]byte{0})
		h.Write([]byte(item.Content))
		h.Write([]byte{0})
	}
	return hex.EncodeToString(h.Sum(nil))
}

func (q *QueryEngine) findMessageByID(sessionID, messageID string) (session.Message, bool) {
	messages, ok := q.sessions.Messages(sessionID)
	if !ok {
		return session.Message{}, false
	}
	for _, message := range messages {
		if message.ID == messageID {
			return message, true
		}
	}
	return session.Message{}, false
}

func (q *QueryEngine) ensureMutableMessages(sessionID string) {
	q.msgMu.RLock()
	_, ok := q.messages[sessionID]
	q.msgMu.RUnlock()
	if ok {
		return
	}
	snapshot, ok := q.sessions.RecoverySnapshot(sessionID)
	if !ok {
		q.replaceMutableMessages(sessionID, nil)
		return
	}
	q.replaceMutableMessages(sessionID, snapshot.Continuation)
	q.restoreStateFromSession(snapshot)
}

func (q *QueryEngine) ensureUserMessageTracked(sessionID string, msg session.Message) {
	q.msgMu.RLock()
	items := q.messages[sessionID]
	for _, existing := range items {
		if existing.ID == msg.ID {
			q.msgMu.RUnlock()
			return
		}
	}
	q.msgMu.RUnlock()
	q.appendMutableMessage(sessionID, msg)
}

func (q *QueryEngine) maybeInjectSkillListingAttachment(sess session.Session) error {
	if _, ok := q.tools.InspectWithPolicy("Skill", "", q.PermissionPolicyForSession(sess.ID)); !ok {
		return nil
	}
	skills := q.skillListingCommands()
	if len(skills) == 0 {
		return nil
	}
	hasExistingListing := q.hasSkillListingAttachment(sess.ID)

	q.toolContextMu.Lock()
	appState := q.toolAppStates[sess.ID]
	if appState == nil {
		appState = make(map[string]any)
	}
	sent := stringSetFromAny(appState["sentSkillListingNames"])
	if len(sent) == 0 && hasExistingListing {
		for _, skill := range skills {
			if skill.Name != "" {
				sent[skill.Name] = struct{}{}
			}
		}
		appState["sentSkillListingNames"] = stringSetKeys(sent)
		q.toolAppStates[sess.ID] = appState
		q.toolContextMu.Unlock()
		return nil
	}
	isInitial := len(sent) == 0
	newSkills := make([]tools.SkillCommand, 0, len(skills))
	for _, skill := range skills {
		if skill.Name == "" || skill.DisableModelInvocation {
			continue
		}
		if _, ok := sent[skill.Name]; ok {
			continue
		}
		newSkills = append(newSkills, skill)
		sent[skill.Name] = struct{}{}
	}
	if len(newSkills) == 0 {
		q.toolAppStates[sess.ID] = appState
		q.toolContextMu.Unlock()
		return nil
	}
	appState["sentSkillListingNames"] = stringSetKeys(sent)
	q.toolAppStates[sess.ID] = appState
	q.toolContextMu.Unlock()

	message := tools.BuildSkillListingAttachmentMessage("", sess.ID, newSkills, 0, isInitial)
	appended, err := q.sessions.AppendModelMessage(sess.ID, message)
	if err != nil {
		return err
	}
	q.appendMutableMessage(sess.ID, appended)
	return nil
}

func (q *QueryEngine) skillListingCommands() []tools.SkillCommand {
	bundled := tools.GetBundledSkills()
	builtinPlugin := tools.GetBuiltinPluginSkillCommands()
	local := tools.GetDynamicSkills()
	base := make([]tools.SkillCommand, 0, len(bundled)+len(builtinPlugin)+len(local))
	base = append(base, bundled...)
	base = append(base, builtinPlugin...)
	base = append(base, local...)
	base = tools.FilterSkillListingCommands(base)
	mcp := tools.FilterSkillListingCommands(tools.MCPSkillCommands(q.mcpSkills))
	if len(base) == 0 {
		return dedupeSkillCommands(mcp)
	}
	if len(mcp) == 0 {
		return dedupeSkillCommands(base)
	}
	out := make([]tools.SkillCommand, 0, len(base)+len(mcp))
	seen := make(map[string]struct{}, len(base)+len(mcp))
	for _, skill := range base {
		if skill.Name == "" {
			continue
		}
		if _, ok := seen[skill.Name]; ok {
			continue
		}
		seen[skill.Name] = struct{}{}
		out = append(out, skill)
	}
	for _, skill := range mcp {
		if skill.Name == "" {
			continue
		}
		if _, ok := seen[skill.Name]; ok {
			continue
		}
		seen[skill.Name] = struct{}{}
		out = append(out, skill)
	}
	return out
}

func dedupeSkillCommands(skills []tools.SkillCommand) []tools.SkillCommand {
	if len(skills) == 0 {
		return nil
	}
	out := make([]tools.SkillCommand, 0, len(skills))
	seen := make(map[string]struct{}, len(skills))
	for _, skill := range skills {
		if skill.Name == "" {
			continue
		}
		if _, ok := seen[skill.Name]; ok {
			continue
		}
		seen[skill.Name] = struct{}{}
		out = append(out, skill)
	}
	return out
}

func (q *QueryEngine) maybeInjectDynamicSkillAttachments(sess session.Session, userMessage session.Message) error {
	if _, ok := q.tools.InspectWithPolicy("Skill", "", q.PermissionPolicyForSession(sess.ID)); !ok {
		return nil
	}
	cwd := ""
	if q.workspace != nil {
		if workspaceContext, err := q.workspaceContextForSession(sess); err == nil {
			cwd = workspaceContext.Root
		}
	}
	if strings.TrimSpace(cwd) == "" {
		return nil
	}
	mentioned := extractSkillMentionedFiles(userMessage.Content)
	if len(mentioned) == 0 {
		return nil
	}
	dirs := tools.DiscoverSkillDirsForPaths(mentioned, cwd)
	if len(dirs) == 0 {
		return nil
	}
	added := tools.AddSkillDirectories(dirs)
	for _, dir := range added {
		msg := tools.BuildDynamicSkillAttachmentMessage("", sess.ID, dir, cwd)
		appended, err := q.sessions.AppendModelMessage(sess.ID, msg)
		if err != nil {
			return err
		}
		q.appendMutableMessage(sess.ID, appended)
	}
	return nil
}

func extractSkillMentionedFiles(content string) []string {
	fields := strings.Fields(content)
	out := make([]string, 0)
	for _, field := range fields {
		field = strings.Trim(field, " \t\r\n.,;:!?()[]{}<>\"'")
		if strings.HasPrefix(field, "@") && len(field) > 1 {
			out = append(out, strings.TrimPrefix(field, "@"))
		}
	}
	return out
}

func (q *QueryEngine) hasSkillListingAttachment(sessionID string) bool {
	q.msgMu.RLock()
	defer q.msgMu.RUnlock()
	for _, message := range q.messages[sessionID] {
		if message.Role == "attachment" && message.Subtype == "skill_listing" {
			return true
		}
	}
	return false
}

func (q *QueryEngine) appendMutableMessage(sessionID string, msg session.Message) {
	q.msgMu.Lock()
	q.messages[sessionID] = append(q.messages[sessionID], msg)
	count := len(q.messages[sessionID])
	q.msgMu.Unlock()

	q.stateMu.Lock()
	if q.state.LastSessionID == sessionID {
		q.state.MessageCount = count
	}
	if msg.Role == "user" {
		q.state.LastUserInput = msg.Content
	}
	q.stateMu.Unlock()
}

func (q *QueryEngine) replaceMutableMessages(sessionID string, items []session.Message) {
	cloned := append([]session.Message(nil), items...)
	q.msgMu.Lock()
	q.messages[sessionID] = cloned
	count := len(cloned)
	q.msgMu.Unlock()

	q.stateMu.Lock()
	if q.state.LastSessionID == sessionID {
		q.state.MessageCount = count
	}
	q.stateMu.Unlock()
}
