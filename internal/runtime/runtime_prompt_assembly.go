package runtime

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"charm.land/fantasy"
	"github.com/CIPFZ/agent-builder/internal/agent"
	"github.com/CIPFZ/agent-builder/internal/contextmgr"
	"github.com/CIPFZ/agent-builder/internal/db"
	"github.com/CIPFZ/agent-builder/internal/message"
	"github.com/CIPFZ/agent-builder/internal/runtimeapi"
	"github.com/CIPFZ/agent-builder/internal/session"
)

func (r *runtimeSchedulerRecorder) RecordPromptAssembly(ctx context.Context, snapshot agent.PromptAssemblySnapshot) error {
	if r == nil || r.service == nil {
		return nil
	}
	return r.service.recordPromptAssembly(ctx, snapshot)
}

func (r *runtimeSchedulerRecorder) BuildModelInput(ctx context.Context, snapshot agent.ModelInputSnapshot) (agent.ModelInputProjection, error) {
	if r == nil || r.service == nil {
		return agent.ModelInputProjection{Messages: snapshot.Messages}, nil
	}
	return r.service.buildModelInputProjection(ctx, snapshot)
}

// ReactiveCompact implements agent.ReactiveCompactor by delegating to the
// runtime service. The agent loop calls this after fantasy.Stream returns a
// context-length error; the runtime records the attempt in contextmgr,
// enforces the per-session circuit breaker, and executes either a
// microcompact-strengthening projection reduction (attempt 1) or a full
// compact (attempt 2+). Returning CircuitOpen=true tells the agent to stop
// retrying.
func (r *runtimeSchedulerRecorder) ReactiveCompact(ctx context.Context, snapshot agent.ReactiveCompactSnapshot) (agent.ReactiveCompactResult, error) {
	if r == nil || r.service == nil {
		return agent.ReactiveCompactResult{}, nil
	}
	return r.service.runReactiveCompact(ctx, snapshot)
}

func (r *runtimeService) PromptAssembliesByTurn(ctx context.Context, turnID string) (RuntimePromptAssembliesResponse, error) {
	if err := r.ensureStarted(ctx); err != nil {
		return RuntimePromptAssembliesResponse{}, err
	}
	turnID = strings.TrimSpace(turnID)
	if turnID == "" {
		return RuntimePromptAssembliesResponse{}, errors.New("turn id is required")
	}
	if r.promptAssemblies.db == nil {
		return RuntimePromptAssembliesResponse{}, nil
	}
	assemblies, err := r.promptAssemblies.ListByTurn(ctx, turnID)
	if err != nil {
		return RuntimePromptAssembliesResponse{}, err
	}
	if assemblies, err = r.enrichPromptAssembliesWithContext(ctx, assemblies); err != nil {
		return RuntimePromptAssembliesResponse{}, err
	}
	return RuntimePromptAssembliesResponse{Assemblies: assemblies}, nil
}

func (r *runtimeService) PromptAssembliesBySession(ctx context.Context, sessionID string, limit int) (RuntimePromptAssembliesResponse, error) {
	if err := r.ensureStarted(ctx); err != nil {
		return RuntimePromptAssembliesResponse{}, err
	}
	sessionID = strings.TrimSpace(sessionID)
	if sessionID == "" {
		r.mu.Lock()
		sessionID = r.sessionID
		r.mu.Unlock()
	}
	if sessionID == "" {
		return RuntimePromptAssembliesResponse{}, errors.New("session id is required")
	}
	if r.promptAssemblies.db == nil {
		return RuntimePromptAssembliesResponse{}, nil
	}
	assemblies, err := r.promptAssemblies.ListBySession(ctx, sessionID, limit)
	if err != nil {
		return RuntimePromptAssembliesResponse{}, err
	}
	if assemblies, err = r.enrichPromptAssembliesWithContext(ctx, assemblies); err != nil {
		return RuntimePromptAssembliesResponse{}, err
	}
	return RuntimePromptAssembliesResponse{Assemblies: assemblies}, nil
}

func (r *runtimeService) ManualCompact(ctx context.Context, req RuntimeContextActionRequest) (RuntimeManualCompactResponse, error) {
	if err := r.ensureStarted(ctx); err != nil {
		return RuntimeManualCompactResponse{}, err
	}
	req.SessionID = strings.TrimSpace(req.SessionID)
	req.TurnID = strings.TrimSpace(req.TurnID)
	if req.SessionID == "" {
		r.mu.Lock()
		req.SessionID = r.sessionID
		r.mu.Unlock()
	}
	if req.SessionID == "" {
		return RuntimeManualCompactResponse{}, errors.New("session id is required")
	}
	if req.TurnID == "" {
		return RuntimeManualCompactResponse{}, errors.New("turn id is required")
	}
	if r.sessionTurnActive(req.SessionID) {
		return RuntimeManualCompactResponse{}, errors.New("会话正在运行，请等待本轮完成后再压缩")
	}
	if err := r.ensureContextManager(ctx); err != nil {
		return RuntimeManualCompactResponse{}, err
	}
	// Manual compact resets the circuit breaker up front: the user is
	// explicitly asking for a compaction, so whatever streak of auto/reactive
	// failures the session accumulated is irrelevant for this attempt and
	// should not gate the subsequent auto/reactive path either.
	r.resetCompactFailures(req.SessionID)
	result, _, err := r.runManualFullCompact(ctx, req)
	if err != nil {
		return RuntimeManualCompactResponse{}, err
	}
	return RuntimeManualCompactResponse{Boundary: runtimeContextBoundaryFromContext(result.Boundary)}, nil
}

// sessionTurnActive reports whether the session has a turn that is still
// running. Manual compaction is rejected while a turn is active because the
// compact rewrites session.SummaryMessageID which the running turn's message
// selection depends on.
func (r *runtimeService) sessionTurnActive(sessionID string) bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	turnID, ok := r.sessionTurns[sessionID]
	if !ok {
		return false
	}
	state, ok := r.requests[turnID]
	if !ok {
		return false
	}
	return !isFinalRuntimeRequestState(state)
}

// manualFullCompactCallContext carries retry/attempt metadata into the
// compact execution path so failManualFullCompact can emit accurate
// attempt/will_retry/circuit_open fields on compact.failed. Manual and auto
// callers pass zero (attempt=1, willRetry=false); reactive callers pass
// their attempt number plus a hint about whether the outer agent loop will
// try again.
type manualFullCompactCallContext struct {
	Attempt   int
	WillRetry bool
}

func (r *runtimeService) runManualFullCompact(ctx context.Context, req RuntimeContextActionRequest) (contextmgr.CompactResult, message.Message, error) {
	return r.runManualFullCompactWithContext(ctx, req, manualFullCompactCallContext{})
}

func (r *runtimeService) runManualFullCompactWithContext(ctx context.Context, req RuntimeContextActionRequest, callCtx manualFullCompactCallContext) (contextmgr.CompactResult, message.Message, error) {
	wsID, err := r.currentWorkspaceID()
	if err != nil {
		return contextmgr.CompactResult{}, message.Message{}, err
	}
	conn, err := r.workspaceDB(ctx)
	if err != nil {
		return contextmgr.CompactResult{}, message.Message{}, err
	}
	now := time.Now().UTC()
	createdAt := now.UnixMilli()
	trigger := strings.TrimSpace(req.Reason)
	if trigger == "" {
		trigger = "manual"
	}
	boundaryID := fmt.Sprintf("ctxbound_full_%s_%s_%d", trigger, strings.ReplaceAll(req.TurnID, ":", "_"), createdAt)
	projectionID := strings.TrimSpace(req.ProjectionID)
	if projectionID == "" {
		projectionID = contextmgr.ProjectionID(req.TurnID, 1)
	}
	started := contextmgr.Boundary{
		ID:           boundaryID,
		SessionID:    req.SessionID,
		TurnID:       req.TurnID,
		ProjectionID: projectionID,
		Kind:         "full",
		Trigger:      trigger,
		Status:       contextmgr.ProjectionStatusStarted,
		CreatedAt:    createdAt,
	}
	started, err = r.contextStore.UpsertBoundary(ctx, started)
	if err != nil {
		return contextmgr.CompactResult{}, message.Message{}, err
	}
	r.emitCompactEvent(runtimeapi.EventCompactStarted, req.SessionID, req.TurnID, map[string]any{
		"boundary_id": boundaryID,
		"kind":        "full",
		"trigger":     trigger,
		"summary":     "context compact started",
	})
	messages, err := r.runtime.ListSessionMessages(ctx, wsID, req.SessionID)
	if err != nil {
		return contextmgr.CompactResult{}, message.Message{}, r.failManualFullCompact(ctx, started, err, callCtx)
	}
	if len(messages) == 0 {
		err = errors.New("manual compact requires at least one message")
		return contextmgr.CompactResult{}, message.Message{}, r.failManualFullCompact(ctx, started, err, callCtx)
	}
	// Only the region after the previous completed full boundary participates
	// in this compact: everything before it is already covered by the
	// previous summary (the summary message itself is carried forward as
	// summarization input for continuity, but is excluded from stats).
	prevSummaryID, _ := r.latestCompletedFullBoundary(ctx, req.SessionID)
	summaryInput := messages
	statsRegion := messages
	if prevSummaryID != "" {
		for i, msg := range messages {
			if msg.ID == prevSummaryID {
				summaryInput = messages[i:]
				statsRegion = messages[i+1:]
				break
			}
		}
	}

	// PreCompact hook: may inject additionalInstructions into the summarizer
	// prompt. Failures are logged but never block the compact.
	instructions := req.Instructions
	if extra := r.runCompactHook(ctx, wsID, "PreCompact", req.SessionID, req.TurnID, boundaryID, trigger); extra != "" {
		if strings.TrimSpace(instructions) == "" {
			instructions = extra
		} else {
			instructions = instructions + "\n\n" + extra
		}
	}

	// Model summarizer with heuristic fallback. The progress ticker fires an
	// ephemeral compact.progress event every 30s while the model call is in
	// flight so the UI can distinguish "still working" from "stalled".
	progressStop := r.startCompactProgress(req.SessionID, req.TurnID, boundaryID)
	summaryText, summaryMode, summaryErr := r.generateCompactSummary(ctx, req, summaryInput, instructions)
	progressStop()
	if summaryErr != nil {
		return contextmgr.CompactResult{}, message.Message{}, r.failManualFullCompact(ctx, started, summaryErr, callCtx)
	}

	wrappedSummary := compactSummaryMessageText(summaryText, trigger)
	sess, err := r.runtime.GetSession(ctx, wsID, req.SessionID)
	if err != nil {
		return contextmgr.CompactResult{}, message.Message{}, r.failManualFullCompact(ctx, started, err, callCtx)
	}
	messageRefs := make([]string, 0, len(statsRegion))
	preTokens := 0
	for _, msg := range statsRegion {
		if msg.ID == "" || msg.IsSummaryMessage || msg.Role == message.System {
			continue
		}
		messageRefs = append(messageRefs, msg.ID)
		preTokens += estimateRuntimeMessageTokens(msg)
	}

	// Reinjection: read_files, todos, loaded skills, running agent tasks.
	// Query failures degrade to "no reinjection" with slog.Warn so the
	// compact itself always succeeds. Refs are recorded on the boundary.
	reinjection := r.buildCompactReinjection(ctx, wsID, req.SessionID, sess)

	summaryParts := []message.ContentPart{message.TextContent{Text: wrappedSummary}}
	summaryParts = append(summaryParts, reinjection.parts...)

	postTokens := estimateRuntimeTokens(wrappedSummary)
	for _, part := range reinjection.parts {
		if text, ok := part.(message.TextContent); ok {
			postTokens += estimateRuntimeTokens(text.Text)
		}
	}

	completed := started
	completed.Status = contextmgr.ProjectionStatusCompleted
	completed.MessageRefs = messageRefs
	completed.ReinjectedRefs = reinjection.refs
	completed.BudgetBefore = &contextmgr.BudgetReport{TotalEstimatedTokens: preTokens}
	completed.BudgetAfter = &contextmgr.BudgetReport{TotalEstimatedTokens: postTokens}

	// The summary message and the completed boundary are written in a single
	// SQLite transaction so readers never observe one without the other.
	tx, err := conn.BeginTx(ctx, nil)
	if err != nil {
		return contextmgr.CompactResult{}, message.Message{}, r.failManualFullCompact(ctx, started, err, callCtx)
	}
	summaryMessage, err := message.NewService(db.New(tx)).Create(ctx, req.SessionID, message.CreateMessageParams{
		Role:             message.User,
		Parts:            summaryParts,
		IsSummaryMessage: true,
		Metadata: map[string]string{
			"synthetic":       "true",
			"compactBoundary": boundaryID,
			"summary_mode":    summaryMode,
			"reinjected":      strconvItoa(len(reinjection.refs)),
		},
	})
	if err != nil {
		_ = tx.Rollback()
		return contextmgr.CompactResult{}, message.Message{}, r.failManualFullCompact(ctx, started, err, callCtx)
	}
	completed.SummaryMessageID = summaryMessage.ID
	completed.SummaryRef = "runtime://messages/" + summaryMessage.ID
	completed.CompletedAt = time.Now().UTC().UnixMilli()
	completed, err = r.contextStore.WithTx(tx).UpsertBoundary(ctx, completed)
	if err != nil {
		_ = tx.Rollback()
		return contextmgr.CompactResult{}, message.Message{}, r.failManualFullCompact(ctx, started, err, callCtx)
	}
	if err := tx.Commit(); err != nil {
		return contextmgr.CompactResult{}, message.Message{}, r.failManualFullCompact(ctx, started, err, callCtx)
	}
	// session.SummaryMessageID moves immediately after the transaction; if it
	// fails the boundary is marked failed so metering never anchors to a
	// summary the read path does not use.
	sess.SummaryMessageID = summaryMessage.ID
	if _, err := r.runtime.SaveSession(ctx, wsID, sess); err != nil {
		return contextmgr.CompactResult{}, message.Message{}, r.failManualFullCompact(ctx, started, err, callCtx)
	}

	// filetracker stale: after a successful compact the next turn should
	// re-read anything it cares about because the model can no longer see
	// the earlier tool results.
	if err := r.runtime.MarkSessionReadFilesStale(ctx, wsID, req.SessionID, "compact_boundary"); err != nil {
		slog.Warn("Compact: failed to mark read files stale (non-fatal)",
			"session_id", req.SessionID, "boundary_id", completed.ID, "error", err)
	}

	// Fire EventContextReinjected once per successful compact so the front
	// end can flag "these refs came back from history". This is the first
	// real emitter of the event (previously orphaned).
	if len(reinjection.refs) > 0 {
		r.storeRuntimeEvent(runtimeapi.Event{
			ID:        newRuntimeEventID(),
			Type:      runtimeapi.EventContextReinjected,
			CreatedAt: time.Now().UTC().Format(time.RFC3339Nano),
			SessionID: req.SessionID,
			TurnID:    req.TurnID,
			Payload: map[string]any{
				"boundary_id":        completed.ID,
				"summary_message_id": summaryMessage.ID,
				"reinjected_refs":    reinjection.refs,
				"summary":            fmt.Sprintf("reinjected %d refs after compact", len(reinjection.refs)),
			},
		})
	}

	// PostCompact hook: fire-and-log. Failures do not roll back the compact.
	r.runCompactHook(ctx, wsID, "PostCompact", req.SessionID, req.TurnID, completed.ID, trigger)

	r.emitCompactEvent(runtimeapi.EventCompactCompleted, req.SessionID, req.TurnID, map[string]any{
		"boundary_id":        completed.ID,
		"kind":               completed.Kind,
		"trigger":            completed.Trigger,
		"pre_tokens":         preTokens,
		"post_tokens":        postTokens,
		"summarized_count":   len(messageRefs),
		"summary_message_id": summaryMessage.ID,
		"summary_mode":       summaryMode,
		"reinjected_count":   len(reinjection.refs),
		"summary":            "context compact completed",
	})
	// Reset the per-session circuit breaker on any successful full compact:
	// a good compact clears whatever streak of failures the session had.
	r.resetCompactFailures(req.SessionID)
	r.publishContextUsageUpdated(ctx, req.SessionID, req.TurnID, "", 0, nil)
	return contextmgr.CompactResult{Boundary: completed}, summaryMessage, nil
}

// generateCompactSummary drives the model summarizer first, falling back to
// the deterministic heuristic when the model path is unavailable or fails.
// The returned mode is "model" for a model-produced summary and
// "heuristic_fallback" for the heuristic path; callers persist it on the
// boundary so we can distinguish quality-of-summary at diagnostics time.
func (r *runtimeService) generateCompactSummary(ctx context.Context, req RuntimeContextActionRequest, messages []message.Message, instructions string) (string, string, error) {
	if len(messages) == 0 {
		return "", "", errors.New("no messages to summarize")
	}
	// The model summarizer is optional: when it is not wired (tests without
	// coordinator, misconfigured workspace) we go straight to the heuristic
	// so the compact still succeeds.
	if r.runtime != nil {
		wsID, wsErr := r.currentWorkspaceID()
		if wsErr == nil {
			modelReq := agent.CompactSummaryRequest{
				SessionID:    req.SessionID,
				TurnID:       req.TurnID,
				Messages:     messages,
				Instructions: instructions,
			}
			result, modelErr := r.runtime.GenerateCompactSummary(ctx, wsID, modelReq)
			if modelErr == nil && strings.TrimSpace(result.Summary) != "" {
				return result.Summary, "model", nil
			}
			if modelErr != nil {
				slog.Warn("Compact: model summarizer failed; falling back to heuristic",
					"session_id", req.SessionID,
					"turn_id", req.TurnID,
					"error", modelErr,
				)
			}
		}
	}
	// Heuristic fallback: never fails on well-formed input.
	summary := buildManualCompactSummary(messages, instructions, r.recentReadFilePaths(ctx, req.SessionID, 10))
	return summary, "heuristic_fallback", nil
}

// startCompactProgress fires an ephemeral compact.progress event every 30s
// until the returned cancel func is invoked. The event is never persisted
// (registered in EphemeralEventTypes).
func (r *runtimeService) startCompactProgress(sessionID, turnID, boundaryID string) func() {
	start := time.Now()
	var wg sync.WaitGroup
	stop := make(chan struct{})
	wg.Add(1)
	go func() {
		defer wg.Done()
		ticker := time.NewTicker(30 * time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-stop:
				return
			case now := <-ticker.C:
				r.storeRuntimeEvent(runtimeapi.Event{
					ID:        newRuntimeEventID(),
					Type:      runtimeapi.EventCompactProgress,
					CreatedAt: now.UTC().Format(time.RFC3339Nano),
					SessionID: sessionID,
					TurnID:    turnID,
					Payload: map[string]any{
						"boundary_id": boundaryID,
						"phase":       "summarizing",
						"elapsed_ms":  now.Sub(start).Milliseconds(),
					},
				})
			}
		}
	}()
	return func() {
		close(stop)
		wg.Wait()
	}
}

// compactReinjectionResult carries the content parts we append to the
// summary message (the visible "read this back" bundle) and the boundary
// refs recorded for audit.
type compactReinjectionResult struct {
	parts []message.ContentPart
	refs  []string
}

// Budgets follow the plan (§WP2 design 3): 5 files, 5k tok/file, 50k
// total; skills: 5k/each, 25k total.
const (
	compactMaxReinjectedFiles  = 5
	compactMaxTokensPerFile    = 5_000
	compactMaxFileTokensTotal  = 50_000
	compactMaxTokensPerSkill   = 5_000
	compactMaxSkillTokensTotal = 25_000
	compactMaxReinjectedSkills = 8
)

// buildCompactReinjection assembles the "re-injected context bundle" that
// rides along with the summary message. Individual sources fail silently
// (slog.Warn) so a broken read_files table or a missing filesystem entry
// never blocks the compact.
func (r *runtimeService) buildCompactReinjection(ctx context.Context, wsID, sessionID string, sess session.Session) compactReinjectionResult {
	var res compactReinjectionResult

	// read_files: fetch recent, exclude CLAUDE.md/plan by suffix, respect
	// per-file/total token budgets, and re-read from disk so the model sees
	// current content. If the on-disk hash differs from what we recorded,
	// tag the entry stale so the model knows to Read it again.
	fileEntries := r.recentReadFileEntries(ctx, sessionID, compactMaxReinjectedFiles*3)
	var (
		filesUsed  int
		fileTokens int
	)
	for _, entry := range fileEntries {
		if filesUsed >= compactMaxReinjectedFiles || fileTokens >= compactMaxFileTokensTotal {
			break
		}
		lowered := strings.ToLower(entry.Path)
		if strings.HasSuffix(lowered, "claude.md") || strings.Contains(lowered, "/plan/") || strings.HasSuffix(lowered, ".plan.md") {
			continue
		}
		data, readErr := os.ReadFile(entry.Path)
		if readErr != nil {
			slog.Warn("Compact reinjection: cannot read file (skipping)",
				"session_id", sessionID, "path", entry.Path, "error", readErr)
			continue
		}
		text := string(data)
		stale := false
		if entry.ContentHash != "" {
			sum := sha256.Sum256(data)
			cur := "sha256:" + hex.EncodeToString(sum[:])
			if cur != entry.ContentHash {
				stale = true
			}
		}
		toks := estimateRuntimeTokens(text)
		if toks > compactMaxTokensPerFile {
			// Truncate lightly: keep the head, tell the model there's
			// more to Read.
			rounded := compactMaxTokensPerFile * 4
			if rounded > len(text) {
				rounded = len(text)
			}
			text = text[:rounded] + "\n\n[truncated; use view/read tool to see the rest]"
			toks = estimateRuntimeTokens(text)
		}
		if fileTokens+toks > compactMaxFileTokensTotal {
			continue
		}
		staleTag := ""
		if stale {
			staleTag = " (contents changed since last read — re-read for latest)"
		}
		block := fmt.Sprintf("<reinjected-file path=\"%s\"%s>\n%s\n</reinjected-file>", entry.Path, staleTag, text)
		res.parts = append(res.parts, message.TextContent{Text: block})
		res.refs = append(res.refs, "file://"+filepath.ToSlash(entry.Path))
		fileTokens += toks
		filesUsed++
	}

	// todos: only surface incomplete items so the reinjected block matches
	// the "what's still pending" contract.
	if len(sess.Todos) > 0 {
		if todoBlock := formatCompactTodosBlock(sess.Todos); todoBlock != "" {
			res.parts = append(res.parts, message.TextContent{Text: todoBlock})
			res.refs = append(res.refs, "todos")
		}
	}

	// skills: load the head of every currently-active skill so the model
	// remembers the tools it has after the compact boundary.
	if skillsBlock, skillNames := r.compactSkillsBlock(); skillsBlock != "" {
		res.parts = append(res.parts, message.TextContent{Text: skillsBlock})
		for _, name := range skillNames {
			res.refs = append(res.refs, "skill://"+name)
		}
	}

	// agent tasks: list any active/queued runners so the model does not
	// double-schedule the same work.
	if taskBlock := r.compactAgentTaskBlock(ctx, sessionID); taskBlock != "" {
		res.parts = append(res.parts, message.TextContent{Text: taskBlock})
		res.refs = append(res.refs, "agent-tasks://"+sessionID)
	}

	return res
}

// runCompactHook invokes PreCompact or PostCompact hooks via the workbench
// coordinator adapter, logs failures, and returns the aggregate
// additionalInstructions text (empty for PostCompact which doesn't feed
// back into the prompt).
func (r *runtimeService) runCompactHook(ctx context.Context, wsID, event, sessionID, turnID, boundaryID, trigger string) string {
	if r.runtime == nil {
		return ""
	}
	payload := map[string]any{
		"session_id":  sessionID,
		"turn_id":     turnID,
		"boundary_id": boundaryID,
		"trigger":     trigger,
	}
	payloadJSON, _ := json.Marshal(payload)
	result, err := r.runtime.RunCompactHooks(ctx, wsID, event, sessionID, turnID, string(payloadJSON))
	if err != nil {
		slog.Warn("Compact hook execution failed (non-fatal)",
			"event", event, "session_id", sessionID, "turn_id", turnID, "error", err)
		return ""
	}
	if result.Blocked {
		slog.Warn("Compact hook returned block/halt but compact is proceeding",
			"event", event, "reason", result.Reason,
			"session_id", sessionID, "turn_id", turnID)
	}
	return result.AdditionalInstructions
}

// compactSkillsBlock returns a text block previewing every currently loaded
// skill's header content plus the ref list for boundary.ReinjectedRefs.
func (r *runtimeService) compactSkillsBlock() (string, []string) {
	if r.runtime == nil || r.workspace == nil {
		return "", nil
	}
	ws, err := r.runtime.GetWorkspace(r.workspace.ID)
	if err != nil || ws == nil || ws.Skills == nil {
		return "", nil
	}
	activeSkills := ws.Skills.ActiveSkills()
	if len(activeSkills) == 0 {
		return "", nil
	}
	var (
		b       strings.Builder
		names   []string
		total   int
		emitted int
	)
	b.WriteString("<reinjected-skills>\n")
	for _, sk := range activeSkills {
		if emitted >= compactMaxReinjectedSkills || total >= compactMaxSkillTokensTotal {
			break
		}
		body := strings.TrimSpace(sk.Instructions)
		if body == "" {
			continue
		}
		toks := estimateRuntimeTokens(body)
		if toks > compactMaxTokensPerSkill {
			rounded := compactMaxTokensPerSkill * 4
			if rounded > len(body) {
				rounded = len(body)
			}
			body = body[:rounded] + "\n[skill body truncated; use skill loader for full content]"
			toks = estimateRuntimeTokens(body)
		}
		if total+toks > compactMaxSkillTokensTotal {
			continue
		}
		fmt.Fprintf(&b, "## %s\n%s\n\n", sk.Name, body)
		names = append(names, sk.Name)
		total += toks
		emitted++
	}
	b.WriteString("</reinjected-skills>")
	if emitted == 0 {
		return "", nil
	}
	return b.String(), names
}

// compactAgentTaskBlock is a placeholder text summary of running/queued
// agent tasks for the session. Query failures degrade to empty.
func (r *runtimeService) compactAgentTaskBlock(ctx context.Context, sessionID string) string {
	if r.agentTasks.db == nil {
		return ""
	}
	tasks, err := r.agentTasks.ListBySession(ctx, sessionID)
	if err != nil {
		slog.Warn("Compact reinjection: cannot list agent tasks", "session_id", sessionID, "error", err)
		return ""
	}
	if len(tasks) == 0 {
		return ""
	}
	var live []string
	for _, task := range tasks {
		switch strings.ToLower(task.Status) {
		case "running", "queued", "in_progress", "pending":
			live = append(live, fmt.Sprintf("- %s (status=%s)", task.Name, task.Status))
		}
	}
	if len(live) == 0 {
		return ""
	}
	var b strings.Builder
	b.WriteString("<reinjected-agent-tasks>\n")
	for _, line := range live {
		b.WriteString(line)
		b.WriteString("\n")
	}
	b.WriteString("</reinjected-agent-tasks>")
	return b.String()
}

// recentReadFileEntry is a slim view over the read_files row used by the
// reinjection budget accounting.
type recentReadFileEntry struct {
	Path        string
	ContentHash string
}

// recentReadFileEntries returns absolute paths + stored content hashes for
// the most recent reads in the session. It uses the workspace working dir
// to reconstruct absolute paths (mirroring filetracker.ListReadFiles).
func (r *runtimeService) recentReadFileEntries(ctx context.Context, sessionID string, limit int) []recentReadFileEntry {
	if limit <= 0 {
		return nil
	}
	conn := r.turns.db
	if conn == nil {
		var err error
		if conn, err = r.workspaceDB(ctx); err != nil {
			return nil
		}
	}
	files, err := db.New(conn).ListSessionReadFiles(ctx, sessionID)
	if err != nil {
		return nil
	}
	base := ""
	if r.workspace != nil {
		base = r.workspace.Path
	}
	if base == "" {
		base, _ = os.Getwd()
	}
	out := make([]recentReadFileEntry, 0, minInt(len(files), limit))
	for _, file := range files {
		path := strings.TrimSpace(file.Path)
		if path == "" {
			continue
		}
		if !filepath.IsAbs(path) && base != "" {
			path = filepath.Join(base, path)
		}
		out = append(out, recentReadFileEntry{
			Path:        path,
			ContentHash: file.ContentHash,
		})
		if len(out) >= limit {
			break
		}
	}
	return out
}

func formatCompactTodosBlock(todos []session.Todo) string {
	var live []session.Todo
	for _, t := range todos {
		if t.Status != session.TodoStatusCompleted {
			live = append(live, t)
		}
	}
	if len(live) == 0 {
		return ""
	}
	var b strings.Builder
	b.WriteString("<reinjected-todos>\n")
	for _, t := range live {
		fmt.Fprintf(&b, "- [%s] %s\n", t.Status, t.Content)
	}
	b.WriteString("</reinjected-todos>")
	return b.String()
}

func strconvItoa(v int) string {
	return fmt.Sprintf("%d", v)
}

// recentReadFilePaths returns up to limit most recently read file paths for
// the session from the read_files table.
func (r *runtimeService) recentReadFilePaths(ctx context.Context, sessionID string, limit int) []string {
	if limit <= 0 {
		return nil
	}
	conn := r.turns.db
	if conn == nil {
		var err error
		if conn, err = r.workspaceDB(ctx); err != nil {
			return nil
		}
	}
	files, err := db.New(conn).ListSessionReadFiles(ctx, sessionID)
	if err != nil {
		return nil
	}
	paths := make([]string, 0, minInt(len(files), limit))
	for _, file := range files {
		path := strings.TrimSpace(file.Path)
		if path == "" {
			continue
		}
		paths = append(paths, path)
		if len(paths) >= limit {
			break
		}
	}
	return paths
}

// compactCircuitBreakerThreshold is the number of consecutive auto/reactive
// full-compact failures per session after which the runtime opens the
// circuit and stops attempting further compaction until a successful
// (or manual) compact resets the counter.
const compactCircuitBreakerThreshold = 3

// isCompactCircuitOpen reports whether the session's consecutive
// full-compact failure counter has reached the configured threshold.
func (r *runtimeService) isCompactCircuitOpen(sessionID string) bool {
	r.compactFailureMu.Lock()
	defer r.compactFailureMu.Unlock()
	return r.compactFailures[sessionID] >= compactCircuitBreakerThreshold
}

// incrementCompactFailure bumps the per-session counter and returns the new
// value so callers can decide whether the circuit is now open.
func (r *runtimeService) incrementCompactFailure(sessionID string) int {
	r.compactFailureMu.Lock()
	defer r.compactFailureMu.Unlock()
	if r.compactFailures == nil {
		r.compactFailures = make(map[string]int)
	}
	r.compactFailures[sessionID]++
	return r.compactFailures[sessionID]
}

// resetCompactFailures clears the counter for the session; called on any
// successful full compact and when the user manually /compacts.
func (r *runtimeService) resetCompactFailures(sessionID string) {
	r.compactFailureMu.Lock()
	defer r.compactFailureMu.Unlock()
	delete(r.compactFailures, sessionID)
}

func (r *runtimeService) failManualFullCompact(ctx context.Context, boundary contextmgr.Boundary, compactErr error, callCtx manualFullCompactCallContext) error {
	failed := boundary
	failed.Status = contextmgr.ProjectionStatusFailed
	failed.Error = compactErr.Error()
	failed.CompletedAt = time.Now().UTC().UnixMilli()
	_, _ = r.contextStore.UpsertBoundary(ctx, failed)

	// Circuit-breaker accounting. Manual failures do NOT increment the
	// counter (the user explicitly asked for it and gets an immediate error
	// they can act on). Auto and reactive failures count toward the
	// threshold so a broken provider or a summarizer stuck in a loop stops
	// burning tokens indefinitely.
	trigger := strings.ToLower(strings.TrimSpace(boundary.Trigger))
	circuitOpen := false
	switch trigger {
	case "auto", "reactive":
		count := r.incrementCompactFailure(boundary.SessionID)
		circuitOpen = count >= compactCircuitBreakerThreshold
	default:
		circuitOpen = r.isCompactCircuitOpen(boundary.SessionID)
	}

	attempt := callCtx.Attempt
	if attempt <= 0 {
		attempt = 1
	}
	willRetry := callCtx.WillRetry && !circuitOpen

	r.emitCompactEvent(runtimeapi.EventCompactFailed, boundary.SessionID, boundary.TurnID, map[string]any{
		"boundary_id":  boundary.ID,
		"kind":         boundary.Kind,
		"trigger":      boundary.Trigger,
		"attempt":      attempt,
		"will_retry":   willRetry,
		"circuit_open": circuitOpen,
		"error":        compactErr.Error(),
		"summary":      compactErr.Error(),
	})
	return compactErr
}

func (r *runtimeService) emitCompactEvent(eventType, sessionID, turnID string, payload map[string]any) {
	r.storeRuntimeEvent(runtimeapi.Event{
		ID:        newRuntimeEventID(),
		Type:      eventType,
		CreatedAt: time.Now().UTC().Format(time.RFC3339Nano),
		SessionID: sessionID,
		TurnID:    turnID,
		Payload:   payload,
	})
}

func (r *runtimeService) currentWorkspaceID() (string, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.workspace == nil || strings.TrimSpace(r.workspace.ID) == "" {
		return "", errors.New("runtime workspace is not available")
	}
	return r.workspace.ID, nil
}

func (r *runtimeService) ManualSnip(ctx context.Context, req RuntimeContextActionRequest) (RuntimeManualSnipResponse, error) {
	if err := r.ensureStarted(ctx); err != nil {
		return RuntimeManualSnipResponse{}, err
	}
	req.SessionID = strings.TrimSpace(req.SessionID)
	req.TurnID = strings.TrimSpace(req.TurnID)
	if req.SessionID == "" {
		r.mu.Lock()
		req.SessionID = r.sessionID
		r.mu.Unlock()
	}
	if req.SessionID == "" {
		return RuntimeManualSnipResponse{}, errors.New("session id is required")
	}
	if req.TurnID == "" {
		return RuntimeManualSnipResponse{}, errors.New("turn id is required")
	}
	if err := r.ensureContextManager(ctx); err != nil {
		return RuntimeManualSnipResponse{}, err
	}
	result, err := r.contextManager.ManualSnip(ctx, contextmgr.ManualSnipRequest{
		SessionID:    req.SessionID,
		TurnID:       req.TurnID,
		ProjectionID: req.ProjectionID,
		Reason:       req.Reason,
	})
	if err != nil {
		return RuntimeManualSnipResponse{}, err
	}
	return RuntimeManualSnipResponse{SnipBoundary: runtimeSnipBoundaryFromContext(result.SnipBoundary)}, nil
}

func (r *runtimeService) recordPromptAssembly(ctx context.Context, snapshot agent.PromptAssemblySnapshot) error {
	if snapshot.SessionID == "" || snapshot.TurnID == "" {
		return nil
	}
	if r.promptAssemblies.db == nil {
		db, err := r.workspaceDB(ctx)
		if err != nil {
			return err
		}
		r.promptAssemblies = newRuntimePromptAssemblyStore(db)
	}
	contextSources := []RuntimeContextSource{}
	var contextSummary RuntimeTurnContextSummary
	if contextResp, err := r.ContextSources(ctx); err == nil {
		contextSources = contextResp.Sources
		contextSources = append(contextSources, r.memoryContextSourcesForTurn(ctx, snapshot.SessionID, snapshot.TurnID)...)
		contextSummary = runtimeTurnContextSummary(contextSources)
	} else {
		contextSources = append(contextSources, RuntimeContextSource{
			ID:             "context:sources",
			Kind:           "context_source",
			Name:           "Context source inventory",
			Enabled:        true,
			State:          capabilityStateFailed,
			Reason:         "context_sources_unavailable",
			Error:          err.Error(),
			ContentSummary: "Runtime could not load context source inventory before model call.",
		})
		contextSummary = runtimeTurnContextSummary(contextSources)
	}
	var compact []RuntimeCompactBoundary
	if boundaries, err := r.contextStore.ListBoundariesByTurn(ctx, snapshot.TurnID); err == nil {
		compact = runtimeCompactBoundariesFromContext(boundaries)
	}
	promptLength := snapshot.Messages.TokenEstimate * 4
	budget := r.computeRuntimeBudget(ctx, snapshot.SessionID, snapshot.TurnID, snapshot.Model, promptLength, &contextSummary)
	projectionID := snapshot.ProjectionID
	if projectionID == "" {
		if projection, err := r.recordNoopContextProjection(ctx, snapshot, budget); err == nil {
			projectionID = projection.ID
		} else {
			return err
		}
	}
	disclosure := r.toolDisclosureBudget(snapshot.TurnID)
	tools := RuntimePromptToolSummary{
		Selected:       append([]string(nil), snapshot.Tools.Selected...),
		Omitted:        append([]string(nil), snapshot.Tools.Omitted...),
		SelectedCount:  firstPositiveInt(snapshot.Tools.SelectedCount, len(snapshot.Tools.Selected)),
		OmittedCount:   firstPositiveInt(snapshot.Tools.OmittedCount, len(snapshot.Tools.Omitted)),
		SelectedBudget: disclosure.Selected,
		OmittedBudget:  disclosure.Omitted,
	}
	if calls, err := r.TurnToolCalls(ctx, snapshot.TurnID); err == nil {
		tools.ResultCount = len(calls.ToolCalls)
		for _, call := range calls.ToolCalls {
			if len(call.OutputRefs) > 0 || strings.TrimSpace(call.OutputSummary) != "" && strings.Contains(call.OutputSummary, "persisted") {
				tools.PersistedResults++
			}
			if call.Compacted || call.CompactRef != "" {
				tools.CompactedResults++
			}
		}
	}
	assembly := RuntimePromptAssembly{
		ID:           runtimePromptAssemblyID(snapshot.TurnID, snapshot.Step),
		SessionID:    snapshot.SessionID,
		TurnID:       snapshot.TurnID,
		ProjectionID: projectionID,
		Step:         firstPositiveInt(snapshot.Step, 1),
		Provider:     snapshot.Provider,
		Model:        snapshot.Model,
		Sections:     runtimePromptSectionSummaries(snapshot.Sections),
		System: RuntimePromptSystemSummary{
			Source:             snapshot.System.Source,
			Hash:               snapshot.System.Hash,
			Length:             snapshot.System.Length,
			TokenEstimate:      snapshot.System.TokenEstimate,
			PromptPrefix:       snapshot.System.PromptPrefix,
			PromptPrefixHash:   snapshot.System.PromptPrefixHash,
			PromptPrefixTokens: snapshot.System.PromptPrefixTokens,
			SourceRefs:         append([]string(nil), snapshot.System.SourceRefs...),
			Redacted:           true,
		},
		Messages: RuntimePromptMessageSummary{
			Count:                snapshot.Messages.Count,
			ByRole:               cloneStringIntMap(snapshot.Messages.ByRole),
			ToolResultCount:      snapshot.Messages.ToolResultCount,
			DeliveredToolResults: snapshot.Messages.DeliveredToolResults,
			SyntheticToolResults: snapshot.Messages.SyntheticToolResults,
			AttachmentCount:      snapshot.Messages.AttachmentCount,
			ImageCount:           snapshot.Messages.ImageCount,
			TokenEstimate:        snapshot.Messages.TokenEstimate,
			RawPromptStored:      false,
		},
		Tools: tools,
		Skills: RuntimePromptSkillSummary{
			AvailableCount:   snapshot.Skills.AvailableCount,
			LoadedCount:      snapshot.Skills.LoadedCount,
			Names:            append([]string(nil), snapshot.Skills.Names...),
			LoadedNames:      append([]string(nil), snapshot.Skills.LoadedNames...),
			XMLPresent:       snapshot.Skills.XMLPresent,
			XMLHash:          snapshot.Skills.XMLHash,
			TokenEstimate:    snapshot.Skills.TokenEstimate,
			RawContentStored: false,
		},
		MCP: RuntimePromptMCPSummary{
			ServerCount:      snapshot.MCP.ServerCount,
			InstructionCount: snapshot.MCP.InstructionCount,
			Servers:          append([]string(nil), snapshot.MCP.Servers...),
			ServerListHash:   snapshot.MCP.ServerListHash,
			InstructionHash:  snapshot.MCP.InstructionHash,
			TokenEstimate:    snapshot.MCP.TokenEstimate,
			RawContentStored: false,
		},
		ContextSources: contextSources,
		Compact:        compact,
		Budget:         budget,
		CreatedAt:      firstNonZeroInt64(snapshot.CreatedAt, time.Now().UTC().UnixMilli()),
	}
	stored, err := r.promptAssemblies.Upsert(ctx, assembly)
	if err != nil {
		return err
	}
	r.storeRuntimeEvent(runtimeapi.Event{
		ID:        newRuntimeEventID(),
		Type:      runtimeapi.EventPromptAssemblyRecorded,
		CreatedAt: time.Now().UTC().Format(time.RFC3339Nano),
		SessionID: stored.SessionID,
		TurnID:    stored.TurnID,
		Payload: map[string]any{
			"assembly_id":            stored.ID,
			"projection_id":          stored.ProjectionID,
			"step":                   stored.Step,
			"provider":               stored.Provider,
			"model":                  stored.Model,
			"total_estimated_tokens": stored.Budget.TotalEstimatedTokens,
			"section_count":          len(stored.Sections),
			"context_source_count":   len(stored.ContextSources),
			"selected_tool_count":    stored.Tools.SelectedCount,
			"omitted_tool_count":     stored.Tools.OmittedCount,
			"compact_boundary_count": len(stored.Compact),
			"summary":                fmt.Sprintf("step %d prompt assembly recorded", stored.Step),
		},
	})
	return nil
}

func runtimePromptSectionSummaries(sections []agent.PromptSectionSummary) []RuntimePromptSectionSummary {
	if len(sections) == 0 {
		return nil
	}
	out := make([]RuntimePromptSectionSummary, 0, len(sections))
	for _, section := range sections {
		out = append(out, RuntimePromptSectionSummary{
			ID:            section.ID,
			Name:          section.Name,
			Kind:          section.Kind,
			Role:          section.Role,
			Order:         section.Order,
			CachePolicy:   section.CachePolicy,
			Source:        section.Source,
			SourceRefs:    append([]string(nil), section.SourceRefs...),
			Scope:         section.Scope,
			Hash:          section.Hash,
			Length:        section.Length,
			TokenEstimate: section.TokenEstimate,
			Redacted:      true,
			RawStored:     false,
			Diagnostics:   section.Diagnostics,
		})
	}
	return out
}

func (r *runtimeService) buildModelInputProjection(ctx context.Context, snapshot agent.ModelInputSnapshot) (agent.ModelInputProjection, error) {
	if err := r.ensureContextManager(ctx); err != nil {
		return agent.ModelInputProjection{}, err
	}
	if !isCompactGuardedTurn(snapshot.TurnID, snapshot.Source) {
		state, hasState := r.compactStateForTurn(snapshot.SessionID, snapshot.TurnID)
		switch {
		case hasState && state.HasProjection:
			// A full compact already happened in this turn: every subsequent
			// step keeps projecting summary + pairing-safe tail so the model
			// never falls back to the pre-compact history.
			snapshot.Messages = applyCompactTurnProjection(snapshot.SessionID, snapshot.TurnID, snapshot.Messages, state)
		case hasState && state.AutoCompactAttempted:
			// At most one auto compact per turn.
		default:
			if usage, usageErr := r.computeContextUsage(ctx, snapshot.SessionID, snapshot.TurnID, snapshot.Model, 0, nil); usageErr == nil && usage.AutoCompactEnabled && usage.AutoCompactAt > 0 && usage.UsedTokens >= usage.AutoCompactAt {
				r.markAutoCompactAttempted(snapshot.SessionID, snapshot.TurnID)
				// Circuit breaker: after N consecutive auto/reactive failures
				// we stop trying automatic compaction for this session so a
				// broken provider does not burn tokens on a loop. The user's
				// next /compact resets the counter.
				if r.isCompactCircuitOpen(snapshot.SessionID) {
					r.emitCompactEvent(runtimeapi.EventCompactFailed, snapshot.SessionID, snapshot.TurnID, map[string]any{
						"kind":         "full",
						"trigger":      "auto",
						"attempt":      1,
						"will_retry":   false,
						"circuit_open": true,
						"summary":      "auto compact skipped (circuit open)",
					})
				} else {
					limits := r.currentRuntimeModelLimits(ctx, snapshot.SessionID, snapshot.Model)
					gov := r.contextGovernanceFor(ctx, snapshot.SessionID, snapshot.Model)
					thresholds := contextThresholds(limits.ContextWindow, limits.MaxOutputTokens, gov.AutoCompactPercent)
					compactReq := RuntimeContextActionRequest{
						SessionID:    snapshot.SessionID,
						TurnID:       snapshot.TurnID,
						ProjectionID: contextmgr.ProjectionID(snapshot.TurnID, firstPositiveInt(snapshot.Step, 1)),
						Reason:       "auto",
						Instructions: "请直接继续当前任务，不要复述摘要、不要向用户提问。",
					}
					if compact, summaryMessage, compactErr := r.runManualFullCompact(ctx, compactReq); compactErr == nil {
						compactState := runtimeTurnCompactState{
							SummaryMessages:      summaryMessage.ToAIMessage(),
							TailStartIndex:       pairSafeTailStart(snapshot.Messages, compactKeepTailMessages),
							SnapshotLen:          len(snapshot.Messages),
							HasProjection:        true,
							AutoCompactAttempted: true,
						}
						r.setCompactTurnState(snapshot.SessionID, snapshot.TurnID, compactState)
						snapshot.Messages = applyCompactTurnProjection(snapshot.SessionID, snapshot.TurnID, snapshot.Messages, compactState)
						r.warnIfAutoCompactIneffective(ctx, snapshot, compact.Boundary.ID)
					} else if usage.UsedTokens >= thresholds.BlockingAt {
						return agent.ModelInputProjection{}, fmt.Errorf("上下文超出模型限制,请手动压缩(/compact)或新建会话: %w", compactErr)
					}
				}
			}
		}
	}
	// Project memory is injected after the boundary-anchored projection so it
	// is always prepended as a system message at the head of the final
	// sequence and never participates in compaction statistics.
	if messages, err := r.injectProjectMemory(ctx, agentModelInputSnapshotLite{
		SessionID: snapshot.SessionID,
		TurnID:    snapshot.TurnID,
		Step:      firstPositiveInt(snapshot.Step, 1),
	}, snapshot.Messages); err == nil {
		snapshot.Messages = messages
	}
	limits := r.currentRuntimeModelLimits(ctx, snapshot.SessionID, snapshot.Model)
	gov := r.contextGovernanceFor(ctx, snapshot.SessionID, snapshot.Model)
	thresholds := contextThresholds(limits.ContextWindow, limits.MaxOutputTokens, gov.AutoCompactPercent)
	// WP4 §2: reactive attempt-1 installs a per-turn microcompact override
	// (stronger KeepRecent, halved token limit). We consult the turn state
	// AFTER any auto compact ran so the override survives the projection
	// installation.
	microKeep := gov.MicrocompactKeepRecent
	microTokenLimit := maxInt(1, thresholds.EffectiveWindow/4)
	if state, hasState := r.compactStateForTurn(snapshot.SessionID, snapshot.TurnID); hasState {
		if state.ReactiveMicroKeepRecent > 0 {
			microKeep = state.ReactiveMicroKeepRecent
		}
		if state.ReactiveMicroTokenDivisor > 1 {
			microTokenLimit = maxInt(1, microTokenLimit/state.ReactiveMicroTokenDivisor)
		}
	}
	result, err := r.contextManager.BuildModelInput(ctx, contextmgr.BuildInputRequest{
		SessionID:     snapshot.SessionID,
		TurnID:        snapshot.TurnID,
		Step:          firstPositiveInt(snapshot.Step, 1),
		Provider:      snapshot.Provider,
		Model:         snapshot.Model,
		Source:        firstNonEmpty(snapshot.Source, contextmgr.ProjectionSourceMainTurn),
		ModelMessages: snapshot.Messages,
		Microcompact: contextmgr.MicrocompactConfig{
			Enabled:              gov.MicrocompactEnabled,
			KeepRecent:           microKeep,
			ToolResultTokenLimit: microTokenLimit,
		},
	})
	if err != nil {
		return agent.ModelInputProjection{}, err
	}
	projected := result.ModelMessages
	if projected == nil {
		projected = snapshot.Messages
	}
	return agent.ModelInputProjection{
		ProjectionID:          result.Projection.ID,
		Messages:              projected,
		CanonicalMessageCount: result.Projection.CanonicalMessageCount,
		ProjectedMessageCount: result.Projection.ProjectedMessageCount,
	}, nil
}

func isCompactGuardedTurn(turnID, source string) bool {
	turnID = strings.TrimSpace(turnID)
	source = strings.TrimSpace(source)
	return strings.HasPrefix(turnID, "helper_") ||
		strings.HasPrefix(turnID, "summary_") ||
		strings.HasPrefix(turnID, "compact_") ||
		source == "summary" ||
		source == "compact"
}

// applyCompactTurnProjection assembles the boundary-anchored projection:
// leading system messages + compact summary + the pairing-safe tail starting
// at the registered index. Fantasy messages only append within a turn so the
// index stays stable; if the snapshot shape changed unexpectedly the
// projection is skipped with a warning.
func applyCompactTurnProjection(sessionID, turnID string, messages []fantasy.Message, state runtimeTurnCompactState) []fantasy.Message {
	if !state.HasProjection {
		return messages
	}
	if state.TailStartIndex < 0 || state.TailStartIndex > len(messages) || len(messages) < state.SnapshotLen {
		slog.Warn("Compact turn projection skipped: snapshot shape changed",
			"session_id", sessionID,
			"turn_id", turnID,
			"tail_start", state.TailStartIndex,
			"snapshot_len", state.SnapshotLen,
			"message_count", len(messages),
		)
		return messages
	}
	leading := leadingSystemMessageCount(messages)
	if leading > state.TailStartIndex {
		leading = state.TailStartIndex
	}
	tail := trimTrailingDanglingToolCalls(messages[state.TailStartIndex:])
	out := make([]fantasy.Message, 0, leading+len(state.SummaryMessages)+len(tail))
	out = append(out, messages[:leading]...)
	out = append(out, state.SummaryMessages...)
	out = append(out, tail...)
	return out
}

func (r *runtimeService) warnIfAutoCompactIneffective(ctx context.Context, snapshot agent.ModelInputSnapshot, boundaryID string) {
	usage, err := r.computeContextUsage(ctx, snapshot.SessionID, snapshot.TurnID, snapshot.Model, 0, nil)
	if err != nil || usage.AutoCompactAt <= 0 || usage.UsedTokens < usage.AutoCompactAt {
		return
	}
	_, _ = r.contextStore.UpsertWarning(ctx, contextmgr.Warning{
		ID:        "ctxwarn_ineffective_" + boundaryID,
		SessionID: snapshot.SessionID,
		TurnID:    snapshot.TurnID,
		Code:      "auto_compact_ineffective",
		Message:   fmt.Sprintf("auto compact left used tokens at %d (threshold %d); skipping further auto compaction this turn", usage.UsedTokens, usage.AutoCompactAt),
		Severity:  "warning",
	})
	slog.Warn("Auto compact ineffective",
		"session_id", snapshot.SessionID,
		"turn_id", snapshot.TurnID,
		"used_tokens", usage.UsedTokens,
		"auto_compact_at", usage.AutoCompactAt,
	)
}

// runReactiveCompact drives the runtime side of the agent's post-error
// recovery loop. It (a) records a contextmgr.ReactiveAttempt row so the
// diagnostics panel sees the retry, (b) short-circuits when the per-session
// circuit breaker is open, (c) intensifies microcompact for attempt 1, and
// (d) runs a full compact anchored on the failing turn for attempt 2+. The
// caller (internal/agent/agent.go) uses CircuitOpen to break out of the
// retry loop; the friendly error is composed by the caller so the runtime
// stays transport-neutral.
func (r *runtimeService) runReactiveCompact(ctx context.Context, snapshot agent.ReactiveCompactSnapshot) (agent.ReactiveCompactResult, error) {
	sessionID := strings.TrimSpace(snapshot.SessionID)
	turnID := strings.TrimSpace(snapshot.TurnID)
	if sessionID == "" || turnID == "" {
		return agent.ReactiveCompactResult{}, errors.New("reactive compact requires session and turn ids")
	}
	projectionID := contextmgr.ProjectionID(turnID, firstPositiveInt(snapshot.Step, 1))

	// Record the attempt regardless of circuit state so diagnostics show
	// each recovery attempt the agent tried.
	if err := r.ensureContextManager(ctx); err == nil && r.contextManager != nil {
		_, _ = r.contextManager.ReactiveCompact(ctx, contextmgr.ReactiveCompactRequest{
			SessionID:    sessionID,
			TurnID:       turnID,
			ProjectionID: projectionID,
			Attempt:      snapshot.Attempt,
			Error:        snapshot.Error,
		})
	}

	if r.isCompactCircuitOpen(sessionID) {
		attempt := snapshot.Attempt
		if attempt <= 0 {
			attempt = 1
		}
		r.emitCompactEvent(runtimeapi.EventCompactFailed, sessionID, turnID, map[string]any{
			"kind":         "full",
			"trigger":      "reactive",
			"attempt":      attempt,
			"will_retry":   false,
			"circuit_open": true,
			"summary":      "reactive compact skipped (circuit open)",
			"error":        snapshot.Error,
		})
		return agent.ReactiveCompactResult{CircuitOpen: true}, nil
	}

	if snapshot.Attempt <= 1 {
		// Attempt 1: strengthen microcompact for this turn only. Halve the
		// tool-result token limit and clamp KeepRecent=1 so the next Stream
		// call re-issues the same history through a tighter projection.
		r.setReactiveProjectionOverride(sessionID, turnID, 1, 2)
		return agent.ReactiveCompactResult{Action: "projection_reduction"}, nil
	}

	// Attempt 2+: full compact under the reactive trigger. WillRetry is
	// true if the agent will still have another attempt to spare after this
	// one succeeds/fails and the circuit does not open.
	callCtx := manualFullCompactCallContext{
		Attempt:   snapshot.Attempt,
		WillRetry: snapshot.Attempt < maxRuntimeReactiveAttempts,
	}
	compactReq := RuntimeContextActionRequest{
		SessionID:    sessionID,
		TurnID:       turnID,
		ProjectionID: projectionID,
		Reason:       "reactive",
		Instructions: "上下文超出模型限制，已执行反应式压缩。请直接继续当前任务，不要复述摘要、不要向用户提问。",
	}
	_, summaryMessage, err := r.runManualFullCompactWithContext(ctx, compactReq, callCtx)
	if err != nil {
		// failManualFullCompact already emitted compact.failed with real
		// attempt / will_retry / circuit_open fields, and incremented the
		// circuit-breaker counter.
		return agent.ReactiveCompactResult{CircuitOpen: r.isCompactCircuitOpen(sessionID)}, err
	}
	// Install the boundary-anchored turn projection so the next PrepareStep
	// in this turn sends summary + pairing-safe tail instead of the full
	// history. Also mark AutoCompactAttempted so the auto path skips —
	// there's nothing more to compact this turn.
	if summaryMessage.ID != "" && len(snapshot.Messages) > 0 {
		state := runtimeTurnCompactState{
			SummaryMessages:      summaryMessage.ToAIMessage(),
			TailStartIndex:       pairSafeTailStart(snapshot.Messages, compactKeepTailMessages),
			SnapshotLen:          len(snapshot.Messages),
			HasProjection:        true,
			AutoCompactAttempted: true,
		}
		// Preserve any reactive microcompact override that was already
		// installed on attempt 1 so the intensified projection stays in
		// effect if subsequent steps re-enter buildModelInputProjection.
		if prev, ok := r.compactStateForTurn(sessionID, turnID); ok {
			state.ReactiveMicroKeepRecent = prev.ReactiveMicroKeepRecent
			state.ReactiveMicroTokenDivisor = prev.ReactiveMicroTokenDivisor
		}
		r.setCompactTurnState(sessionID, turnID, state)
	}
	return agent.ReactiveCompactResult{Action: "full_compact"}, nil
}

// maxRuntimeReactiveAttempts mirrors the agent-side cap so failManualFullCompact
// can populate will_retry deterministically without cross-package leakage.
const maxRuntimeReactiveAttempts = 3

func compactTurnStateKey(sessionID, turnID string) string {
	return sessionID + "\x00" + turnID
}

func (r *runtimeService) compactStateForTurn(sessionID, turnID string) (runtimeTurnCompactState, bool) {
	r.compactTurnMu.Lock()
	defer r.compactTurnMu.Unlock()
	state, ok := r.compactTurnStates[compactTurnStateKey(sessionID, turnID)]
	return state, ok
}

func (r *runtimeService) setCompactTurnState(sessionID, turnID string, state runtimeTurnCompactState) {
	r.compactTurnMu.Lock()
	defer r.compactTurnMu.Unlock()
	if r.compactTurnStates == nil {
		r.compactTurnStates = make(map[string]runtimeTurnCompactState)
	}
	r.compactTurnStates[compactTurnStateKey(sessionID, turnID)] = state
}

func (r *runtimeService) markAutoCompactAttempted(sessionID, turnID string) {
	r.compactTurnMu.Lock()
	defer r.compactTurnMu.Unlock()
	if r.compactTurnStates == nil {
		r.compactTurnStates = make(map[string]runtimeTurnCompactState)
	}
	key := compactTurnStateKey(sessionID, turnID)
	state := r.compactTurnStates[key]
	state.AutoCompactAttempted = true
	r.compactTurnStates[key] = state
}

// setReactiveProjectionOverride installs (or refreshes) the microcompact
// intensification used by reactive attempt 1. buildModelInputProjection
// reads these fields when constructing MicrocompactConfig on subsequent
// PrepareStep calls in this turn, without touching global governance.
func (r *runtimeService) setReactiveProjectionOverride(sessionID, turnID string, keepRecent, tokenDivisor int) {
	r.compactTurnMu.Lock()
	defer r.compactTurnMu.Unlock()
	if r.compactTurnStates == nil {
		r.compactTurnStates = make(map[string]runtimeTurnCompactState)
	}
	key := compactTurnStateKey(sessionID, turnID)
	state := r.compactTurnStates[key]
	if keepRecent > 0 {
		state.ReactiveMicroKeepRecent = keepRecent
	}
	if tokenDivisor > 1 {
		state.ReactiveMicroTokenDivisor = tokenDivisor
	}
	r.compactTurnStates[key] = state
}

func (r *runtimeService) clearCompactTurnState(sessionID, turnID string) {
	r.compactTurnMu.Lock()
	defer r.compactTurnMu.Unlock()
	delete(r.compactTurnStates, compactTurnStateKey(sessionID, turnID))
}

func (r *runtimeService) clearCompactStatesForSession(sessionID string) {
	r.compactTurnMu.Lock()
	defer r.compactTurnMu.Unlock()
	prefix := sessionID + "\x00"
	for key := range r.compactTurnStates {
		if strings.HasPrefix(key, prefix) {
			delete(r.compactTurnStates, key)
		}
	}
}

func (r *runtimeService) enrichPromptAssembliesWithContext(ctx context.Context, assemblies []RuntimePromptAssembly) ([]RuntimePromptAssembly, error) {
	if len(assemblies) == 0 {
		return assemblies, nil
	}
	if err := r.ensureContextManager(ctx); err != nil {
		return nil, err
	}
	for i := range assemblies {
		projectionID := strings.TrimSpace(assemblies[i].ProjectionID)
		if projectionID == "" {
			continue
		}
		boundaries, err := r.contextStore.ListBoundariesByProjection(ctx, projectionID)
		if err != nil {
			return nil, err
		}
		for _, boundary := range boundaries {
			assemblies[i].ContextBoundaries = append(assemblies[i].ContextBoundaries, runtimeContextBoundaryFromContext(boundary))
		}
		snipBoundaries, err := r.contextStore.ListSnipBoundariesByProjection(ctx, projectionID)
		if err != nil {
			return nil, err
		}
		for _, snip := range snipBoundaries {
			assemblies[i].SnipBoundaries = append(assemblies[i].SnipBoundaries, runtimeSnipBoundaryFromContext(snip))
		}
		replacements, err := r.contextStore.ListContentReplacementsByProjection(ctx, projectionID)
		if err != nil {
			return nil, err
		}
		for _, replacement := range replacements {
			assemblies[i].Replacements = append(assemblies[i].Replacements, runtimeContentReplacementFromContext(replacement))
		}
		attempts, err := r.contextStore.ListReactiveAttemptsByTurn(ctx, assemblies[i].TurnID)
		if err != nil {
			return nil, err
		}
		for _, attempt := range attempts {
			if attempt.ProjectionID == "" || attempt.ProjectionID == projectionID {
				assemblies[i].ReactiveAttempts = append(assemblies[i].ReactiveAttempts, runtimeReactiveAttemptFromContext(attempt))
			}
		}
	}
	return assemblies, nil
}

func (r *runtimeService) ensureContextManager(ctx context.Context) error {
	if r.contextManager != nil {
		return nil
	}
	db, err := r.workspaceDB(ctx)
	if err != nil {
		return err
	}
	r.contextStore = contextmgr.NewSQLStore(db)
	r.contextManager = contextmgr.NewManager(contextmgr.ManagerOptions{Store: r.contextStore})
	return nil
}

func (r *runtimeService) recordNoopContextProjection(ctx context.Context, snapshot agent.PromptAssemblySnapshot, budget RuntimeBudgetReport) (contextmgr.Projection, error) {
	if err := r.ensureContextManager(ctx); err != nil {
		return contextmgr.Projection{}, err
	}
	result, err := r.contextManager.BuildModelInput(ctx, contextmgr.BuildInputRequest{
		SessionID:         snapshot.SessionID,
		TurnID:            snapshot.TurnID,
		Step:              firstPositiveInt(snapshot.Step, 1),
		Provider:          snapshot.Provider,
		Model:             snapshot.Model,
		Source:            contextmgr.ProjectionSourceMainTurn,
		CanonicalMessages: snapshot.Messages.Count,
		ProjectedMessages: snapshot.Messages.Count,
		BudgetBefore:      runtimeBudgetToContextBudget(budget),
		BudgetAfter:       runtimeBudgetToContextBudget(budget),
	})
	if err != nil {
		return contextmgr.Projection{}, err
	}
	return result.Projection, nil
}

func runtimeBudgetToContextBudget(report RuntimeBudgetReport) *contextmgr.BudgetReport {
	return &contextmgr.BudgetReport{
		TotalEstimatedTokens: report.TotalEstimatedTokens,
		ContextWindow:        report.ContextWindow,
		Model:                report.Model,
	}
}

func runtimeContextBoundaryFromContext(boundary contextmgr.Boundary) RuntimeContextBoundary {
	return RuntimeContextBoundary{
		ID:               boundary.ID,
		SessionID:        boundary.SessionID,
		TurnID:           boundary.TurnID,
		ProjectionID:     boundary.ProjectionID,
		Kind:             boundary.Kind,
		Trigger:          boundary.Trigger,
		Status:           boundary.Status,
		SummaryMessageID: boundary.SummaryMessageID,
		SummaryRef:       boundary.SummaryRef,
		MessageRefs:      append([]string(nil), boundary.MessageRefs...),
		ToolCallRefs:     append([]string(nil), boundary.ToolCallRefs...),
		ReinjectedRefs:   append([]string(nil), boundary.ReinjectedRefs...),
		BudgetBefore:     runtimeBudgetFromContextBudget(boundary.BudgetBefore),
		BudgetAfter:      runtimeBudgetFromContextBudget(boundary.BudgetAfter),
		CreatedAt:        boundary.CreatedAt,
		CompletedAt:      boundary.CompletedAt,
		Error:            boundary.Error,
	}
}

func runtimeSnipBoundaryFromContext(snip contextmgr.SnipBoundary) RuntimeSnipBoundary {
	return RuntimeSnipBoundary{
		ID:                 snip.ID,
		SessionID:          snip.SessionID,
		TurnID:             snip.TurnID,
		ProjectionID:       snip.ProjectionID,
		RemovedMessageRefs: append([]string(nil), snip.RemovedMessageRefs...),
		PreservedHeadRef:   snip.PreservedHeadRef,
		PreservedTailRef:   snip.PreservedTailRef,
		SummaryRef:         snip.SummaryRef,
		Reason:             snip.Reason,
		CreatedAt:          snip.CreatedAt,
	}
}

func runtimeContentReplacementFromContext(replacement contextmgr.ContentReplacement) RuntimeContentReplacement {
	return RuntimeContentReplacement{
		ID:                         replacement.ID,
		SessionID:                  replacement.SessionID,
		TurnID:                     replacement.TurnID,
		ProjectionID:               replacement.ProjectionID,
		ToolCallID:                 replacement.ToolCallID,
		ToolName:                   replacement.ToolName,
		Kind:                       replacement.Kind,
		OriginalRef:                replacement.OriginalRef,
		ReplacementEstimatedTokens: replacement.ReplacementEstimatedTokens,
		OriginalEstimatedTokens:    replacement.OriginalEstimatedTokens,
		OriginalSizeBytes:          replacement.OriginalSizeBytes,
		Reason:                     replacement.Reason,
		CreatedAt:                  replacement.CreatedAt,
	}
}

func runtimeReactiveAttemptFromContext(attempt contextmgr.ReactiveAttempt) RuntimeReactiveCompactAttempt {
	return RuntimeReactiveCompactAttempt{
		ID:           attempt.ID,
		SessionID:    attempt.SessionID,
		TurnID:       attempt.TurnID,
		ProjectionID: attempt.ProjectionID,
		Attempt:      attempt.Attempt,
		Action:       attempt.Action,
		Status:       attempt.Status,
		Error:        attempt.Error,
		CreatedAt:    attempt.CreatedAt,
		CompletedAt:  attempt.CompletedAt,
	}
}

func runtimeBudgetFromContextBudget(report *contextmgr.BudgetReport) *RuntimeBudgetReport {
	if report == nil {
		return nil
	}
	return &RuntimeBudgetReport{
		ContextWindow:        report.ContextWindow,
		Model:                report.Model,
		TotalEstimatedTokens: report.TotalEstimatedTokens,
	}
}

func runtimePromptAssemblyID(turnID string, step int) string {
	if step <= 0 {
		step = 1
	}
	return fmt.Sprintf("prompt_%s_step_%03d", strings.ReplaceAll(turnID, ":", "_"), step)
}

func cloneStringIntMap(in map[string]int) map[string]int {
	if len(in) == 0 {
		return nil
	}
	out := make(map[string]int, len(in))
	for key, value := range in {
		out[key] = value
	}
	return out
}

func firstPositiveInt(values ...int) int {
	for _, value := range values {
		if value > 0 {
			return value
		}
	}
	return 0
}

func buildManualCompactSummary(messages []message.Message, instructions string, readFiles []string) string {
	var userMessages []string
	var errors []string
	var currentWork []string
	for _, msg := range messages {
		if msg.IsSummaryMessage || msg.Role == message.System {
			continue
		}
		text := compactMessageText(msg)
		if text == "" {
			continue
		}
		switch msg.Role {
		case message.User:
			userMessages = append(userMessages, compactClip(text, 600))
			currentWork = append(currentWork, compactClip(text, 300))
		case message.Assistant:
			if strings.Contains(strings.ToLower(text), "error") || strings.Contains(text, "失败") {
				errors = append(errors, compactClip(text, 400))
			}
		case message.Tool:
			for _, part := range msg.Parts {
				if result, ok := part.(message.ToolResult); ok && result.IsError {
					errors = append(errors, compactClip(result.Content, 400))
				}
			}
		}
	}
	files := make([]string, 0, len(readFiles))
	for _, path := range readFiles {
		path = strings.TrimSpace(path)
		if path == "" {
			continue
		}
		files = append(files, compactClip(path, 240))
		if len(files) >= 10 {
			break
		}
	}
	var b strings.Builder
	b.WriteString("## Primary Request and Intent\n")
	if len(userMessages) > 0 {
		b.WriteString("- Preserve and continue the user's latest requested task.\n")
	} else {
		b.WriteString("- Continue the current conversation from the compacted context.\n")
	}
	b.WriteString("\n## Key Technical Concepts\n- Context compaction, append-only transcript, runtime projection, and session continuity.\n")
	b.WriteString("\n## Files and Code Sections\n")
	writeCompactList(&b, files, "- No specific file list was available from the compacted messages.")
	b.WriteString("\n## Errors and Fixes\n")
	writeCompactList(&b, errors, "- No explicit errors were captured in the compacted messages.")
	b.WriteString("\n## Problem Solving\n- Earlier conversation content has been compacted into this summary so the next model input can continue without replaying every prior message.\n")
	b.WriteString("\n## All User Messages\n")
	writeCompactList(&b, userMessages, "- No user message text was available.")
	b.WriteString("\n## Pending Tasks\n")
	if trimmed := strings.TrimSpace(instructions); trimmed != "" {
		b.WriteString("- Manual compact instructions: ")
		b.WriteString(compactClip(trimmed, 1000))
		b.WriteString("\n")
	} else {
		b.WriteString("- Continue from the current task state without asking the user to restate prior context.\n")
	}
	b.WriteString("\n## Current Work\n")
	if len(currentWork) > 0 {
		b.WriteString("- Latest user direction: ")
		b.WriteString(currentWork[len(currentWork)-1])
		b.WriteString("\n")
	} else {
		b.WriteString("- Resume the conversation after this compact summary.\n")
	}
	b.WriteString("\n## Optional Next Step\n")
	if len(userMessages) > 0 {
		b.WriteString("- Continue with the latest user request: \"")
		b.WriteString(compactClip(userMessages[len(userMessages)-1], 500))
		b.WriteString("\"\n")
	} else {
		b.WriteString("- Continue the current task using the compact summary above.\n")
	}
	return strings.TrimSpace(b.String())
}

func compactSummaryMessageText(summary, trigger string) string {
	prefix := "本会话由早前对话压缩延续，以下摘要覆盖此前内容。"
	suffix := "如需早前细节，可查看完整会话记录。"
	if trigger == "auto" {
		suffix += " 请直接继续当前任务，不要复述摘要、不要向用户提问。"
	}
	return prefix + "\n\n" + strings.TrimSpace(summary) + "\n\n" + suffix
}

func compactMessageText(msg message.Message) string {
	var parts []string
	for _, part := range msg.Parts {
		switch p := part.(type) {
		case message.TextContent:
			parts = append(parts, p.Text)
		case message.ImageURLContent:
			parts = append(parts, "[image]")
		case message.BinaryContent:
			parts = append(parts, "[document]")
		case message.ToolCall:
			parts = append(parts, "tool_call:"+p.Name)
		case message.ToolResult:
			parts = append(parts, firstNonEmpty(p.Content, p.Data, p.Metadata))
		}
	}
	return strings.TrimSpace(strings.Join(parts, "\n"))
}

func writeCompactList(b *strings.Builder, values []string, fallback string) {
	if len(values) == 0 {
		b.WriteString(fallback)
		b.WriteString("\n")
		return
	}
	limit := minInt(len(values), 20)
	start := maxInt(0, len(values)-limit)
	for _, value := range values[start:] {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		b.WriteString("- ")
		b.WriteString(value)
		b.WriteString("\n")
	}
}

func compactClip(value string, limit int) string {
	value = strings.Join(strings.Fields(strings.TrimSpace(value)), " ")
	if limit <= 0 || len([]rune(value)) <= limit {
		return value
	}
	runes := []rune(value)
	return string(runes[:limit]) + "..."
}
