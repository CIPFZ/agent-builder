package runtime

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/charmbracelet/crush/internal/runtimeapi"
)

const (
	runtimeRunCheckpointActionSourceKind = "run_checkpoint"
	runtimeRunSummarySourceKind          = "persisted_run_summary"

	runtimeRunCheckpointActionAcknowledge = "acknowledge_checkpoint"
	runtimeRunCheckpointActionDiscard     = "discard_checkpoint"
	runtimeRunCheckpointActionResume      = "resume_checkpoint"

	runtimeRunCheckpointActionReasonAcknowledged = "checkpoint_acknowledged"
	runtimeRunCheckpointActionReasonDiscarded    = "checkpoint_discarded"
	runtimeRunCheckpointActionReasonResumed      = "checkpoint_resume_started"
)

func runtimeRunSummarySource() RuntimeRunSummarySource {
	return RuntimeRunSummarySource{
		Kind:                           runtimeRunSummarySourceKind,
		ReadOnly:                       true,
		SummaryOnly:                    true,
		PersistedRunAuthority:          true,
		ProjectionRequiredForLifecycle: true,
		ExcludedEvidence: []string{
			"status",
			"finished_at",
			"checkpoints",
			"diagnostics",
			"artifacts",
			"permissions",
			"mcp_actionability",
			"interrupted_summaries",
			"scheduler_details",
			"transition_interpretation",
		},
	}
}

func runtimeRunSummaryFromRun(run RuntimeRun) RuntimeRunSummary {
	return RuntimeRunSummary{
		ID:               run.ID,
		WorkspaceID:      run.WorkspaceID,
		PrimarySessionID: run.PrimarySessionID,
		SessionIDs:       append([]string(nil), run.SessionIDs...),
		Objective:        run.Objective,
		Source:           run.Source,
		CreatedAt:        run.CreatedAt,
		UpdatedAt:        run.UpdatedAt,
	}
}

func (r *runtimeService) Runs(ctx context.Context) (RuntimeRunsResponse, error) {
	if err := r.ensureStarted(ctx); err != nil {
		return RuntimeRunsResponse{}, err
	}
	if r.runs.db == nil {
		return RuntimeRunsResponse{}, errors.New("runtime run database is not available")
	}
	if err := r.backfillRuntimeRuns(ctx); err != nil {
		return RuntimeRunsResponse{}, err
	}
	runs, err := r.runs.List(ctx)
	if err != nil {
		return RuntimeRunsResponse{}, err
	}
	return RuntimeRunsResponse{Runs: runs}, nil
}

func (r *runtimeService) RunSummaries(ctx context.Context) (RuntimeRunSummariesResponse, error) {
	if err := r.ensureStarted(ctx); err != nil {
		return RuntimeRunSummariesResponse{}, err
	}
	if r.runs.db == nil {
		return RuntimeRunSummariesResponse{}, errors.New("runtime run database is not available")
	}
	runs, err := r.runs.List(ctx)
	if err != nil {
		return RuntimeRunSummariesResponse{}, err
	}
	summaries := make([]RuntimeRunSummary, 0, len(runs))
	for _, run := range runs {
		summaries = append(summaries, runtimeRunSummaryFromRun(run))
	}
	return RuntimeRunSummariesResponse{Runs: summaries, Source: runtimeRunSummarySource()}, nil
}

func (r *runtimeService) RunSummary(ctx context.Context, id string) (RuntimeRunSummaryResponse, error) {
	if err := r.ensureStarted(ctx); err != nil {
		return RuntimeRunSummaryResponse{}, err
	}
	id = strings.TrimSpace(id)
	if id == "" {
		return RuntimeRunSummaryResponse{}, errors.New("run id is required")
	}
	if r.runs.db == nil {
		return RuntimeRunSummaryResponse{}, errors.New("runtime run database is not available")
	}
	run, err := r.runs.Get(ctx, id)
	if err != nil {
		return RuntimeRunSummaryResponse{}, err
	}
	return RuntimeRunSummaryResponse{Run: runtimeRunSummaryFromRun(run), Source: runtimeRunSummarySource()}, nil
}

func (r *runtimeService) Run(ctx context.Context, id string) (RuntimeRunResponse, error) {
	if err := r.ensureStarted(ctx); err != nil {
		return RuntimeRunResponse{}, err
	}
	id = strings.TrimSpace(id)
	if id == "" {
		return RuntimeRunResponse{}, errors.New("run id is required")
	}
	if r.runs.db == nil {
		return RuntimeRunResponse{}, errors.New("runtime run database is not available")
	}
	run, err := r.runs.Get(ctx, id)
	if errors.Is(err, errRuntimeRunNotFound) && strings.HasPrefix(id, "run:session:") {
		sessionID := strings.TrimPrefix(id, "run:session:")
		if _, backfillErr := r.backfillRuntimeRunSession(ctx, sessionID); backfillErr != nil {
			return RuntimeRunResponse{}, backfillErr
		}
		run, err = r.runs.Get(ctx, id)
	}
	if err != nil {
		return RuntimeRunResponse{}, err
	}
	projection, err := r.RunProjection(ctx, RuntimeRunProjectionRequest{SessionID: run.PrimarySessionID})
	if err != nil {
		return RuntimeRunResponse{}, fmt.Errorf("failed to build runtime run projection parity payload: %w", err)
	}
	projection.Run.ID = run.ID
	run, err = r.runs.Get(ctx, run.ID)
	if err != nil {
		return RuntimeRunResponse{}, err
	}
	return RuntimeRunResponse{Run: run, Projection: projection.Run}, nil
}

func (r *runtimeService) AcknowledgeRunCheckpoint(ctx context.Context, runID, checkpointID string) (RuntimeRunResponse, error) {
	if err := r.ensureStarted(ctx); err != nil {
		return RuntimeRunResponse{}, err
	}
	if r.runs.db == nil {
		return RuntimeRunResponse{}, errors.New("runtime run database is not available")
	}
	if _, err := r.runs.AcknowledgeCheckpoint(ctx, runID, checkpointID); err != nil {
		return RuntimeRunResponse{}, err
	}
	resp, err := r.Run(ctx, runID)
	if err != nil {
		return RuntimeRunResponse{}, err
	}
	return withRuntimeRunCheckpointAction(resp, runtimeRunCheckpointActionAcknowledge, runtimeRunCheckpointActionReasonAcknowledged), nil
}

func (r *runtimeService) DiscardRunCheckpoint(ctx context.Context, runID, checkpointID string) (RuntimeRunResponse, error) {
	if err := r.ensureStarted(ctx); err != nil {
		return RuntimeRunResponse{}, err
	}
	if r.runs.db == nil {
		return RuntimeRunResponse{}, errors.New("runtime run database is not available")
	}
	if _, err := r.runs.DiscardCheckpoint(ctx, runID, checkpointID); err != nil {
		return RuntimeRunResponse{}, err
	}
	resp, err := r.Run(ctx, runID)
	if err != nil {
		return RuntimeRunResponse{}, err
	}
	return withRuntimeRunCheckpointAction(resp, runtimeRunCheckpointActionDiscard, runtimeRunCheckpointActionReasonDiscarded), nil
}

func withRuntimeRunCheckpointAction(resp RuntimeRunResponse, action, reason string) RuntimeRunResponse {
	resp.Action = &RuntimeWriteActionMetadata{
		Accepted:       true,
		Reason:         reason,
		RefreshTargets: runtimeRunSchedulerRefreshTargets(),
		Source: RuntimeWriteActionSource{
			Kind:                  runtimeRunCheckpointActionSourceKind,
			Action:                action,
			BackendOnly:           true,
			StartsWorker:          false,
			IdempotentBy:          "run_id+checkpoint_id",
			SessionActivityParity: true,
			Evidence: []string{
				"runtime_runs",
				"runtime_run_checkpoints",
				"runtime_run_projection",
				"runtime_run_scheduler_plan",
				"session_activity",
			},
		},
	}
	return resp
}

func (r *runtimeService) ResumeRunCheckpoint(ctx context.Context, runID, checkpointID string) (RuntimeRunResumeResponse, error) {
	if err := r.ensureStarted(ctx); err != nil {
		return RuntimeRunResumeResponse{}, err
	}
	if r.runs.db == nil {
		return RuntimeRunResumeResponse{}, errors.New("runtime run database is not available")
	}
	runID = strings.TrimSpace(runID)
	checkpointID = strings.TrimSpace(checkpointID)
	if runID == "" {
		return RuntimeRunResumeResponse{}, errors.New("run id is required")
	}
	if checkpointID == "" {
		return RuntimeRunResumeResponse{}, errors.New("checkpoint id is required")
	}
	run, err := r.runs.Get(ctx, runID)
	if err != nil {
		return RuntimeRunResumeResponse{}, err
	}
	checkpoint, ok := runtimeRunCheckpointByID(run.Checkpoints, checkpointID)
	if !ok {
		return RuntimeRunResumeResponse{}, errRuntimeRunCheckpointNotFound
	}
	if !checkpoint.ResumeEligible {
		return RuntimeRunResumeResponse{}, errors.New("checkpoint is not eligible for resume")
	}
	prompt := runtimeRunResumePrompt(run, checkpoint)
	chat, err := r.Chat(ctx, RuntimeChatRequest{SessionID: run.PrimarySessionID, Prompt: prompt})
	if err != nil {
		return RuntimeRunResumeResponse{}, err
	}
	if _, err := r.runs.LinkCheckpointResume(ctx, run.ID, checkpoint.ID, chat.TurnID); err != nil {
		return RuntimeRunResumeResponse{}, err
	}
	r.recordCheckpointResumeTransition(ctx, run, checkpoint, chat.TurnID)
	r.writeAudit(auditEntry{
		RequestID:     chat.TurnID,
		Event:         "run_checkpoint_resumed",
		Timestamp:     time.Now().UTC().Format(time.RFC3339Nano),
		WorkspaceID:   run.WorkspaceID,
		SessionID:     run.PrimarySessionID,
		PromptPreview: preview(prompt, auditPreviewLimit),
		Extra: map[string]any{
			"run_id":          run.ID,
			"checkpoint_id":   checkpoint.ID,
			"source_turn_id":  checkpoint.TurnID,
			"source_task_id":  checkpoint.TaskID,
			"resumed_turn_id": chat.TurnID,
			"artifact_refs":   checkpoint.ArtifactRefs,
		},
	})
	r.storeRuntimeEvent(runtimeapi.Event{
		ID:        newRuntimeEventID(),
		Type:      "run.checkpoint.resumed",
		CreatedAt: time.Now().UTC().Format(time.RFC3339Nano),
		SessionID: run.PrimarySessionID,
		TurnID:    chat.TurnID,
		Payload: map[string]any{
			"run_id":          run.ID,
			"checkpoint_id":   checkpoint.ID,
			"resumed_turn_id": chat.TurnID,
		},
	})
	refreshed, err := r.Run(ctx, run.ID)
	if err != nil {
		return RuntimeRunResumeResponse{}, err
	}
	resp := RuntimeRunResumeResponse{
		RunID:        run.ID,
		CheckpointID: checkpoint.ID,
		SessionID:    run.PrimarySessionID,
		TurnID:       chat.TurnID,
		Chat:         chat,
		Run:          refreshed,
	}
	return withRuntimeRunCheckpointResumeAction(resp), nil
}

func withRuntimeRunCheckpointResumeAction(resp RuntimeRunResumeResponse) RuntimeRunResumeResponse {
	resp.Action = &RuntimeWriteActionMetadata{
		Accepted:       true,
		Reason:         runtimeRunCheckpointActionReasonResumed,
		RefreshTargets: runtimeRunSchedulerRefreshTargets(),
		Source: RuntimeWriteActionSource{
			Kind:                  runtimeRunCheckpointActionSourceKind,
			Action:                runtimeRunCheckpointActionResume,
			BackendOnly:           true,
			StartsWorker:          true,
			SessionActivityParity: true,
			Evidence: []string{
				"runtime_runs",
				"runtime_run_checkpoints",
				"runtime_turns",
				"runtime_messages",
				"runtime_run_transitions",
				"runtime_events",
				"runtime_audit",
				"runtime_run_projection",
				"runtime_run_scheduler_plan",
				"session_activity",
			},
		},
	}
	return resp
}

func (r *runtimeService) backfillRuntimeRuns(ctx context.Context) error {
	r.mu.Lock()
	wsID := r.workspace.ID
	r.mu.Unlock()
	sessions, err := r.runtime.ListSessions(ctx, wsID)
	if err != nil {
		return fmt.Errorf("failed to list sessions for runtime run backfill: %w", err)
	}
	for _, sess := range sessions {
		if _, err := r.backfillRuntimeRunSession(ctx, sess.ID); err != nil {
			return err
		}
	}
	return nil
}

func (r *runtimeService) backfillRuntimeRunSession(ctx context.Context, sessionID string) (RuntimeRun, error) {
	sessionID = strings.TrimSpace(sessionID)
	if sessionID == "" {
		return RuntimeRun{}, errors.New("session id is required")
	}
	projection, err := r.RunProjection(ctx, RuntimeRunProjectionRequest{SessionID: sessionID})
	if err != nil {
		return RuntimeRun{}, err
	}
	if run, err := r.runs.GetBySession(ctx, sessionID); err == nil {
		return run, nil
	}
	return r.runs.UpsertFromProjection(ctx, projection.Run, runtimeRunSourceBackfill)
}

func (r *runtimeService) reconcileRuntimeRunForSession(ctx context.Context, sessionID string) (RuntimeRunProjection, error) {
	if r.runs.db == nil || strings.TrimSpace(sessionID) == "" {
		return RuntimeRunProjection{}, nil
	}
	resp, err := r.RunProjection(ctx, RuntimeRunProjectionRequest{SessionID: sessionID})
	if err != nil {
		slog.Warn("Failed to reconcile runtime run", "session_id", sessionID, "error", err)
		return RuntimeRunProjection{}, err
	}
	return resp.Run, nil
}

func (r *runtimeService) reconcileRuntimeRunForTerminalTask(ctx context.Context, task RuntimeAgentTask) {
	if !isFinalAgentTaskStatus(task.Status) || strings.TrimSpace(task.ParentSessionID) == "" || r.runs.db == nil {
		return
	}
	r.mu.Lock()
	runtimeReady := r.runtime != nil
	workspaceReady := r.workspace != nil
	r.mu.Unlock()
	if !runtimeReady || !workspaceReady {
		return
	}
	if _, err := r.reconcileRuntimeRunForSession(ctx, task.ParentSessionID); err != nil {
		slog.Warn("Failed to reconcile runtime run for terminal task", "task_id", task.ID, "session_id", task.ParentSessionID, "error", err)
	}
}

func runtimeRunCheckpointByID(checkpoints []RuntimeRunCheckpoint, checkpointID string) (RuntimeRunCheckpoint, bool) {
	for _, checkpoint := range checkpoints {
		if checkpoint.ID == checkpointID {
			return checkpoint, true
		}
	}
	return RuntimeRunCheckpoint{}, false
}

func runtimeRunResumePrompt(run RuntimeRun, checkpoint RuntimeRunCheckpoint) string {
	parts := []string{
		"Resume the interrupted work from the structured runtime checkpoint below.",
		"",
		"Rules:",
		"- Treat this as a new explicit user turn.",
		"- Do not replay previous tool calls or assume stale permission/MCP requests are actionable.",
		"- Use current workspace state and verify any artifact before relying on it.",
		"",
		"Runtime checkpoint:",
		"- run_id: " + run.ID,
		"- checkpoint_id: " + checkpoint.ID,
		"- session_id: " + run.PrimarySessionID,
	}
	if checkpoint.TurnID != "" {
		parts = append(parts, "- source_turn_id: "+checkpoint.TurnID)
	}
	if checkpoint.TaskID != "" {
		parts = append(parts, "- source_task_id: "+checkpoint.TaskID)
	}
	if checkpoint.Summary != "" {
		parts = append(parts, "- summary: "+preview(redactRuntimeString("summary", checkpoint.Summary), auditPreviewLimit))
	}
	if len(checkpoint.ArtifactRefs) > 0 {
		parts = append(parts, "- artifact_refs: "+strings.Join(checkpoint.ArtifactRefs, ", "))
	}
	return strings.Join(parts, "\n")
}
