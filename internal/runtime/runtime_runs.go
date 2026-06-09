package runtime

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/charmbracelet/crush/internal/runtimeapi"
)

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
	return r.Run(ctx, runID)
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
	return r.Run(ctx, runID)
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
	return RuntimeRunResumeResponse{
		RunID:        run.ID,
		CheckpointID: checkpoint.ID,
		SessionID:    run.PrimarySessionID,
		TurnID:       chat.TurnID,
		Chat:         chat,
		Run:          refreshed,
	}, nil
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
	return r.runs.UpsertFromProjection(ctx, projection.Run, runtimeRunSourceBackfill)
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
