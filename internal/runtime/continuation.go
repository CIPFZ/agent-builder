package runtime

import (
	"fmt"
	"sort"
	"strings"

	"myclaw/internal/agent"
	"myclaw/internal/approval"
	"myclaw/internal/session"
)

type ContinuationSnapshot struct {
	SessionID           string
	SessionKey          string
	AgentID             string
	IsMain              bool
	Status              session.ContinuationStatus
	ReadyForPrompt      bool
	ResumeFromMessageID string
	ResumeFromRole      string
	HasCompaction       bool
	RecoveryError       string
	PendingApproval     *approval.Request
	Tasks               []TaskSnapshot
}

type TaskSnapshot struct {
	RunID           string
	ParentSessionID string
	Label           string
	Prompt          string
	Status          string
	ChildSessionID  string
	ChildSessionKey string
	Attempt         int
	LastAction      string
	Output          string
	OutputFile      string
	Error           string
	ControlMessages []string
}

func (r *Runner) ContinuationSnapshot(sessionID string) (ContinuationSnapshot, error) {
	if r == nil || r.sessions == nil {
		return ContinuationSnapshot{}, fmt.Errorf("runtime sessions are not configured")
	}
	sess, ok := r.sessions.GetByID(sessionID)
	if !ok {
		return ContinuationSnapshot{}, fmt.Errorf("session %q not found", sessionID)
	}
	return r.continuationSnapshotForSession(sess), nil
}

func (r *Runner) continuationSnapshotForSession(sess session.Session) ContinuationSnapshot {
	state, ok := r.sessions.ContinuationState(sess.ID)
	if !ok {
		state = session.ContinuationState{
			Status:         session.ContinuationStatusAwaitingAssistant,
			ReadyForPrompt: false,
		}
	}
	snapshot := ContinuationSnapshot{
		SessionID:           sess.ID,
		SessionKey:          sess.Key,
		AgentID:             sess.AgentID,
		IsMain:              sess.IsMain,
		Status:              state.Status,
		ReadyForPrompt:      state.ReadyForPrompt,
		ResumeFromMessageID: state.ResumeFromMessageID,
		ResumeFromRole:      state.ResumeFromRole,
		HasCompaction:       state.HasCompaction,
	}
	if !ok {
		snapshot.RecoveryError = "session recovery state is unavailable"
	}
	if stateError := r.stateError(); stateError != "" {
		snapshot.ReadyForPrompt = false
		snapshot.RecoveryError = appendRecoveryError(snapshot.RecoveryError, stateError)
	}
	if corruptPendingApprovalMetadata(sess.Metadata) {
		snapshot.Status = session.ContinuationStatusAwaitingApproval
		snapshot.ReadyForPrompt = false
		snapshot.ResumeFromMessageID = sess.Metadata.PendingApprovalID
		snapshot.ResumeFromRole = "approval"
		snapshot.RecoveryError = "pending approval metadata is incomplete or inconsistent"
	}
	if pending, ok := r.pendingApprovalForSession(sess.ID); ok {
		snapshot.PendingApproval = cloneApprovalRequest(pending)
		snapshot.Status = session.ContinuationStatusAwaitingApproval
		snapshot.ReadyForPrompt = false
		snapshot.ResumeFromMessageID = pending.ID
		snapshot.ResumeFromRole = "approval"
	}
	snapshot.Tasks = r.taskSnapshotsForSession(sess.ID)
	return snapshot
}

func corruptPendingApprovalMetadata(metadata session.SessionMetadata) bool {
	if strings.TrimSpace(metadata.PendingApprovalID) == "" {
		return false
	}
	if metadata.PendingApprovalStatus != string(approval.StatusPending) {
		return true
	}
	if strings.TrimSpace(metadata.PendingApprovalToolName) == "" {
		return true
	}
	return false
}

func (r *Runner) taskSnapshotsForSession(sessionID string) []TaskSnapshot {
	if r == nil || r.options.AgentManager == nil {
		return nil
	}
	runs := r.options.AgentManager.List()
	out := make([]TaskSnapshot, 0, len(runs))
	for _, run := range runs {
		if run.ParentSessionID != sessionID {
			continue
		}
		out = append(out, taskSnapshotFromRun(run))
	}
	sort.SliceStable(out, func(i, j int) bool {
		return out[i].RunID < out[j].RunID
	})
	return out
}

func taskSnapshotFromRun(run agent.Run) TaskSnapshot {
	return TaskSnapshot{
		RunID:           run.ID,
		ParentSessionID: run.ParentSessionID,
		Label:           run.Label,
		Prompt:          run.Prompt,
		Status:          string(run.Status),
		ChildSessionID:  run.ChildSessionID,
		ChildSessionKey: run.ChildSessionKey,
		Attempt:         run.Attempt,
		LastAction:      string(run.LastAction),
		Output:          run.Output,
		OutputFile:      run.OutputFile,
		Error:           run.ErrorSummary,
		ControlMessages: append([]string(nil), run.ControlMessages...),
	}
}

func cloneApprovalRequest(request approval.Request) *approval.Request {
	cloned := request
	cloned.ToolInputObject = cloneAnyMap(request.ToolInputObject)
	cloned.ContentBlocks = cloneAnyMaps(request.ContentBlocks)
	return &cloned
}

func agentRunFromMetadata(metadata session.AgentRunMetadata) agent.Run {
	status := agent.Status(metadata.Status)
	if status == "" {
		status = agent.StatusStopped
	}
	return agent.Run{
		ID:              metadata.ID,
		ParentSessionID: metadata.ParentSessionID,
		ParentAgentID:   metadata.ParentAgentID,
		ChildSessionID:  metadata.ChildSessionID,
		ChildSessionKey: metadata.ChildSessionKey,
		Label:           metadata.Label,
		Prompt:          metadata.Prompt,
		AllowedTools:    append([]string(nil), metadata.AllowedTools...),
		Model:           metadata.Model,
		Effort:          metadata.Effort,
		Status:          status,
		LastAction:      agent.ControlAction(metadata.LastAction),
		Attempt:         metadata.Attempt,
		Output:          metadata.Output,
		OutputFile:      metadata.OutputFile,
		ErrorSummary:    metadata.ErrorSummary,
		ControlMessages: append([]string(nil), metadata.ControlMessages...),
		CreatedAt:       metadata.CreatedAt,
		StartedAt:       metadata.StartedAt,
		UpdatedAt:       metadata.UpdatedAt,
		CompletedAt:     metadata.CompletedAt,
		LastActionAt:    metadata.LastActionAt,
	}
}

func agentRunMetadataFromRun(run agent.Run) session.AgentRunMetadata {
	return session.AgentRunMetadata{
		ID:              run.ID,
		ParentSessionID: run.ParentSessionID,
		ParentAgentID:   run.ParentAgentID,
		ChildSessionID:  run.ChildSessionID,
		ChildSessionKey: run.ChildSessionKey,
		Label:           run.Label,
		Prompt:          run.Prompt,
		AllowedTools:    append([]string(nil), run.AllowedTools...),
		Model:           run.Model,
		Effort:          run.Effort,
		Status:          string(run.Status),
		LastAction:      string(run.LastAction),
		Attempt:         run.Attempt,
		Output:          run.Output,
		OutputFile:      run.OutputFile,
		ErrorSummary:    run.ErrorSummary,
		ControlMessages: append([]string(nil), run.ControlMessages...),
		CreatedAt:       run.CreatedAt,
		StartedAt:       run.StartedAt,
		UpdatedAt:       run.UpdatedAt,
		CompletedAt:     run.CompletedAt,
		LastActionAt:    run.LastActionAt,
	}
}

func rehydrateAgentRuns(sessions *session.Manager, manager *agent.Manager) []string {
	if sessions == nil || manager == nil {
		return nil
	}
	var errs []string
	for _, sess := range sessions.ListSessions() {
		for _, run := range sess.Metadata.AgentRuns {
			if strings.TrimSpace(run.ID) == "" {
				errs = append(errs, fmt.Sprintf("session %s has agent run metadata without id", sess.ID))
				continue
			}
			if err := manager.Restore(agentRunFromMetadata(run)); err != nil {
				errs = append(errs, err.Error())
			}
		}
	}
	return errs
}

func (r *Runner) persistAgentRun(run agent.Run) {
	if r == nil || r.sessions == nil || strings.TrimSpace(run.ParentSessionID) == "" {
		return
	}
	metadata := agentRunMetadataFromRun(run)
	if err := r.sessions.UpdateMetadata(run.ParentSessionID, func(sessionMetadata *session.SessionMetadata) {
		index := -1
		for i := range sessionMetadata.AgentRuns {
			if sessionMetadata.AgentRuns[i].ID == metadata.ID {
				index = i
				break
			}
		}
		if index >= 0 {
			sessionMetadata.AgentRuns[index] = metadata
		} else {
			sessionMetadata.AgentRuns = append(sessionMetadata.AgentRuns, metadata)
		}
	}); err != nil {
		r.recordStateError(fmt.Sprintf("persist agent run %s: %v", run.ID, err))
	}
}

func (r *Runner) recordStateError(message string) {
	if r == nil || strings.TrimSpace(message) == "" {
		return
	}
	r.stateMu.Lock()
	defer r.stateMu.Unlock()
	r.stateErrors = append(r.stateErrors, message)
}

func (r *Runner) stateError() string {
	if r == nil {
		return ""
	}
	r.stateMu.RLock()
	defer r.stateMu.RUnlock()
	return strings.Join(r.stateErrors, "; ")
}

func appendRecoveryError(current, next string) string {
	current = strings.TrimSpace(current)
	next = strings.TrimSpace(next)
	if current == "" {
		return next
	}
	if next == "" {
		return current
	}
	return current + "; " + next
}
