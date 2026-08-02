package runtime

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
	"time"
)

const (
	runtimeRunSourceUserPrompt = "user_prompt"
	runtimeRunSourceBackfill   = "backfill"

	runtimeRunSessionRolePrimary = "primary"
	runtimeRunSessionRoleChild   = "child"

	runtimeRunCheckpointStatusResumable = "resumable"
)

var (
	errRuntimeRunNotFound           = errors.New("runtime run not found")
	errRuntimeRunCheckpointNotFound = errors.New("runtime run checkpoint not found")
)

type runtimeRunStore struct {
	db *sql.DB
}

func newRuntimeRunStore(db *sql.DB) runtimeRunStore {
	return runtimeRunStore{db: db}
}

func (s runtimeRunStore) EnsureForSession(ctx context.Context, workspaceID, sessionID, objective, source string) (RuntimeRun, error) {
	if s.db == nil {
		return RuntimeRun{}, errors.New("runtime run database is not available")
	}
	workspaceID = strings.TrimSpace(workspaceID)
	sessionID = strings.TrimSpace(sessionID)
	if workspaceID == "" {
		return RuntimeRun{}, errors.New("run workspace id is required")
	}
	if sessionID == "" {
		return RuntimeRun{}, errors.New("run session id is required")
	}
	if existing, err := s.GetBySession(ctx, sessionID); err == nil {
		return existing, nil
	} else if !errors.Is(err, errRuntimeRunNotFound) {
		return RuntimeRun{}, err
	}
	runID := runtimeGeneratedRunID()
	if strings.TrimSpace(source) == runtimeRunSourceBackfill {
		runID = runtimeRunProjectionID(sessionID)
	}
	now := time.Now().UnixMilli()
	run := RuntimeRun{
		ID:               runID,
		WorkspaceID:      workspaceID,
		PrimarySessionID: sessionID,
		SessionIDs:       []string{sessionID},
		Objective:        strings.TrimSpace(objective),
		Status:           runtimeRunStatusActive,
		Source:           firstNonEmpty(strings.TrimSpace(source), runtimeRunSourceUserPrompt),
		CreatedAt:        now,
		UpdatedAt:        now,
	}
	if run.Source == runtimeRunSourceBackfill {
		run.Status = runtimeRunStatusCompleted
	}
	return s.Upsert(ctx, run)
}

func (s runtimeRunStore) UpsertFromProjection(ctx context.Context, projection RuntimeRunProjection, source string) (RuntimeRun, error) {
	if s.db == nil {
		return RuntimeRun{}, errors.New("runtime run database is not available")
	}
	sessionID := strings.TrimSpace(projection.PrimarySessionID)
	if sessionID == "" {
		return RuntimeRun{}, errors.New("run primary session id is required")
	}
	runID := strings.TrimSpace(projection.ID)
	var existing RuntimeRun
	existing, err := s.GetBySession(ctx, sessionID)
	if err == nil {
		runID = existing.ID
	} else if errors.Is(err, errRuntimeRunNotFound) && strings.TrimSpace(source) == runtimeRunSourceBackfill {
		runID = runtimeRunProjectionID(sessionID)
	} else if err != nil && !errors.Is(err, errRuntimeRunNotFound) {
		return RuntimeRun{}, err
	}
	run := RuntimeRun{
		ID:               runID,
		WorkspaceID:      projection.WorkspaceID,
		PrimarySessionID: sessionID,
		SessionIDs:       appendUniqueStrings(nil, projection.SessionIDs...),
		Objective:        projection.Objective,
		Status:           projection.Status,
		Source:           firstNonEmpty(existing.Source, strings.TrimSpace(source), runtimeRunSourceBackfill),
		Checkpoints:      projection.Checkpoints,
		CreatedAt:        projection.CreatedAt,
		UpdatedAt:        projection.UpdatedAt,
		FinishedAt:       projection.FinishedAt,
	}
	if run.Objective == "" {
		run.Objective = existing.Objective
	}
	if run.CreatedAt == 0 {
		run.CreatedAt = existing.CreatedAt
	}
	if run.ID == "" {
		run.ID = runtimeRunProjectionID(sessionID)
	}
	if len(run.SessionIDs) == 0 {
		run.SessionIDs = []string{sessionID}
	}
	if existing.ID != "" && projection.Status == runtimeRunStatusCompleted && !runtimeRunProjectionHasEvidence(projection) {
		switch existing.Status {
		case runtimeRunStatusActive, runtimeRunStatusWaitingUser, runtimeRunStatusInterrupted:
			run.Status = existing.Status
			run.FinishedAt = existing.FinishedAt
		}
	}
	return s.Upsert(ctx, run)
}

func (s runtimeRunStore) LinkTurn(ctx context.Context, runID, sessionID, turnID string, startedAt int64) (RuntimeRun, error) {
	if s.db == nil {
		return RuntimeRun{}, errors.New("runtime run database is not available")
	}
	runID = strings.TrimSpace(runID)
	sessionID = strings.TrimSpace(sessionID)
	turnID = strings.TrimSpace(turnID)
	if runID == "" {
		return RuntimeRun{}, errors.New("run id is required")
	}
	if sessionID == "" {
		return RuntimeRun{}, errors.New("run session id is required")
	}
	if turnID == "" {
		return RuntimeRun{}, errors.New("run turn id is required")
	}
	now := time.Now().UnixMilli()
	if startedAt <= 0 {
		startedAt = now
	}
	result, err := s.db.ExecContext(ctx, `
UPDATE runtime_run_sessions
SET turn_id = ?
WHERE run_id = ? AND session_id = ?`, turnID, runID, sessionID)
	if err != nil {
		return RuntimeRun{}, fmt.Errorf("failed to link runtime run turn: %w", err)
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return RuntimeRun{}, fmt.Errorf("failed to inspect runtime run turn link: %w", err)
	}
	if affected == 0 {
		if _, err := s.Get(ctx, runID); errors.Is(err, errRuntimeRunNotFound) {
			return RuntimeRun{}, err
		} else if err != nil {
			return RuntimeRun{}, err
		}
		return RuntimeRun{}, errors.New("runtime run session link not found")
	}
	return s.writeRuntimeRunStatus(ctx, runtimeRunStatusWriteRequest{
		RunID:        runID,
		SessionID:    sessionID,
		Status:       runtimeRunStatusActive,
		Source:       runtimeRunTransitionSourceTurnStarted,
		Reason:       "turn linked to runtime run",
		EvidenceKind: runtimeRunStatusWriteEvidenceTurn,
		TurnID:       turnID,
		Timestamp:    startedAt,
	})
}

func runtimeRunSessionLinkedToTurn(ctx context.Context, store runtimeRunStore, runID, sessionID, turnID string) bool {
	if store.db == nil {
		return false
	}
	runID = strings.TrimSpace(runID)
	sessionID = strings.TrimSpace(sessionID)
	turnID = strings.TrimSpace(turnID)
	if runID == "" || sessionID == "" || turnID == "" {
		return false
	}
	var linkedTurnID string
	if err := store.db.QueryRowContext(ctx, `
SELECT COALESCE(turn_id, '')
FROM runtime_run_sessions
WHERE run_id = ? AND session_id = ?`, runID, sessionID).Scan(&linkedTurnID); err != nil {
		return false
	}
	return linkedTurnID == turnID
}

func (s runtimeRunStore) Upsert(ctx context.Context, run RuntimeRun) (RuntimeRun, error) {
	if s.db == nil {
		return RuntimeRun{}, errors.New("runtime run database is not available")
	}
	run.ID = strings.TrimSpace(run.ID)
	run.WorkspaceID = strings.TrimSpace(run.WorkspaceID)
	run.PrimarySessionID = strings.TrimSpace(run.PrimarySessionID)
	if run.ID == "" {
		return RuntimeRun{}, errors.New("run id is required")
	}
	if run.WorkspaceID == "" {
		return RuntimeRun{}, errors.New("run workspace id is required")
	}
	if run.PrimarySessionID == "" {
		return RuntimeRun{}, errors.New("run primary session id is required")
	}
	if run.Status == "" {
		run.Status = runtimeRunStatusCompleted
	}
	if run.Source == "" {
		run.Source = runtimeRunSourceBackfill
	}
	now := time.Now().UnixMilli()
	if run.CreatedAt == 0 {
		run.CreatedAt = now
	}
	if run.UpdatedAt == 0 {
		run.UpdatedAt = run.CreatedAt
	}
	_, err := s.db.ExecContext(ctx, `
INSERT INTO runtime_runs (
    id, workspace_id, primary_session_id, objective, status,
    created_at, updated_at, finished_at, discarded_at, source, metadata_json
) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
ON CONFLICT(id) DO UPDATE SET
    workspace_id = COALESCE(NULLIF(excluded.workspace_id, ''), runtime_runs.workspace_id),
    primary_session_id = COALESCE(NULLIF(excluded.primary_session_id, ''), runtime_runs.primary_session_id),
    objective = COALESCE(NULLIF(excluded.objective, ''), runtime_runs.objective),
    status = excluded.status,
    updated_at = excluded.updated_at,
    finished_at = excluded.finished_at,
    discarded_at = COALESCE(excluded.discarded_at, runtime_runs.discarded_at),
    source = COALESCE(NULLIF(excluded.source, ''), runtime_runs.source),
    metadata_json = COALESCE(excluded.metadata_json, runtime_runs.metadata_json)`,
		run.ID,
		run.WorkspaceID,
		run.PrimarySessionID,
		nullableString(run.Objective),
		run.Status,
		run.CreatedAt,
		run.UpdatedAt,
		nullableInt64(run.FinishedAt),
		nullableInt64(run.DiscardedAt),
		run.Source,
		nil,
	)
	if err != nil {
		return RuntimeRun{}, fmt.Errorf("failed to upsert runtime run: %w", err)
	}
	if err := s.upsertSessionLinks(ctx, run); err != nil {
		return RuntimeRun{}, err
	}
	if err := s.upsertCheckpoints(ctx, run); err != nil {
		return RuntimeRun{}, err
	}
	return s.Get(ctx, run.ID)
}

func (s runtimeRunStore) Get(ctx context.Context, id string) (RuntimeRun, error) {
	if s.db == nil {
		return RuntimeRun{}, errors.New("runtime run database is not available")
	}
	row := s.db.QueryRowContext(ctx, runtimeRunSelectSQL()+` WHERE id = ?`, strings.TrimSpace(id))
	run, err := scanRuntimeRun(row)
	if errors.Is(err, sql.ErrNoRows) {
		return RuntimeRun{}, errRuntimeRunNotFound
	}
	if err != nil {
		return RuntimeRun{}, err
	}
	return s.hydrateRunChildren(ctx, run)
}

func (s runtimeRunStore) GetBySession(ctx context.Context, sessionID string) (RuntimeRun, error) {
	if s.db == nil {
		return RuntimeRun{}, errors.New("runtime run database is not available")
	}
	row := s.db.QueryRowContext(ctx, runtimeRunSelectSQL()+`
WHERE id IN (SELECT run_id FROM runtime_run_sessions WHERE session_id = ?)
ORDER BY updated_at DESC
LIMIT 1`, strings.TrimSpace(sessionID))
	run, err := scanRuntimeRun(row)
	if errors.Is(err, sql.ErrNoRows) {
		return RuntimeRun{}, errRuntimeRunNotFound
	}
	if err != nil {
		return RuntimeRun{}, err
	}
	return s.hydrateRunChildren(ctx, run)
}

func (s runtimeRunStore) List(ctx context.Context) ([]RuntimeRun, error) {
	if s.db == nil {
		return nil, errors.New("runtime run database is not available")
	}
	rows, err := s.db.QueryContext(ctx, runtimeRunSelectSQL()+` ORDER BY updated_at DESC`)
	if err != nil {
		return nil, fmt.Errorf("failed to list runtime runs: %w", err)
	}
	defer rows.Close() //nolint:errcheck
	var runs []RuntimeRun
	for rows.Next() {
		run, err := scanRuntimeRun(rows)
		if err != nil {
			return nil, err
		}
		runs = append(runs, run)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("failed to iterate runtime runs: %w", err)
	}
	if err := rows.Close(); err != nil {
		return nil, fmt.Errorf("failed to close runtime run rows: %w", err)
	}
	for i := range runs {
		run, err := s.hydrateRunChildren(ctx, runs[i])
		if err != nil {
			return nil, err
		}
		runs[i] = run
	}
	return runs, nil
}

func (s runtimeRunStore) ListCheckpointMarkers(ctx context.Context, runID string) ([]RuntimeRunCheckpointMarker, error) {
	run, err := s.Get(ctx, runID)
	if err != nil {
		return nil, err
	}
	markers := make([]RuntimeRunCheckpointMarker, 0, len(run.Checkpoints))
	for _, checkpoint := range run.Checkpoints {
		markers = append(markers, runtimeRunCheckpointMarkerFromCheckpoint(run.ID, checkpoint))
	}
	return markers, nil
}

func (s runtimeRunStore) GetCheckpointMarker(ctx context.Context, runID, checkpointID string) (RuntimeRunCheckpointMarker, error) {
	checkpointID = strings.TrimSpace(checkpointID)
	if checkpointID == "" {
		return RuntimeRunCheckpointMarker{}, errors.New("checkpoint id is required")
	}
	markers, err := s.ListCheckpointMarkers(ctx, runID)
	if err != nil {
		return RuntimeRunCheckpointMarker{}, err
	}
	for _, marker := range markers {
		if marker.CheckpointID == checkpointID {
			return marker, nil
		}
	}
	return RuntimeRunCheckpointMarker{}, errRuntimeRunCheckpointNotFound
}

func (s runtimeRunStore) AcknowledgeCheckpoint(ctx context.Context, runID, checkpointID string) (RuntimeRun, error) {
	return s.markCheckpoint(ctx, runID, checkpointID, "acknowledged_at")
}

func (s runtimeRunStore) DiscardCheckpoint(ctx context.Context, runID, checkpointID string) (RuntimeRun, error) {
	return s.markCheckpoint(ctx, runID, checkpointID, "discarded_at")
}

func (s runtimeRunStore) LinkCheckpointResume(ctx context.Context, runID, checkpointID, turnID string) (RuntimeRun, error) {
	if s.db == nil {
		return RuntimeRun{}, errors.New("runtime run database is not available")
	}
	runID = strings.TrimSpace(runID)
	checkpointID = strings.TrimSpace(checkpointID)
	turnID = strings.TrimSpace(turnID)
	if runID == "" {
		return RuntimeRun{}, errors.New("run id is required")
	}
	if checkpointID == "" {
		return RuntimeRun{}, errors.New("checkpoint id is required")
	}
	if turnID == "" {
		return RuntimeRun{}, errors.New("resume turn id is required")
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return RuntimeRun{}, fmt.Errorf("failed to begin runtime run checkpoint resume link: %w", err)
	}
	defer tx.Rollback() //nolint:errcheck
	var raw sql.NullString
	err = tx.QueryRowContext(ctx, `SELECT metadata_json FROM runtime_run_checkpoints WHERE run_id = ? AND id = ?`, runID, checkpointID).Scan(&raw)
	if errors.Is(err, sql.ErrNoRows) {
		var existingRunID string
		if getErr := tx.QueryRowContext(ctx, `SELECT id FROM runtime_runs WHERE id = ?`, runID).Scan(&existingRunID); errors.Is(getErr, sql.ErrNoRows) {
			return RuntimeRun{}, errRuntimeRunNotFound
		} else if getErr != nil {
			return RuntimeRun{}, getErr
		}
		return RuntimeRun{}, errRuntimeRunCheckpointNotFound
	}
	if err != nil {
		return RuntimeRun{}, fmt.Errorf("failed to read runtime run checkpoint metadata: %w", err)
	}
	metadata := decodeJSONMap(raw.String)
	if metadata == nil {
		metadata = map[string]any{}
	}
	metadata["resumedTurnIds"] = appendUniqueStrings(stringSliceFromMap(metadata, "resumedTurnIds"), turnID)
	encoded, err := encodeJSONMap(metadata)
	if err != nil {
		return RuntimeRun{}, err
	}
	if _, err := tx.ExecContext(ctx, `UPDATE runtime_run_checkpoints SET metadata_json = ? WHERE run_id = ? AND id = ?`, encoded, runID, checkpointID); err != nil {
		return RuntimeRun{}, fmt.Errorf("failed to link runtime run checkpoint resume turn: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return RuntimeRun{}, fmt.Errorf("failed to commit runtime run checkpoint resume link: %w", err)
	}
	return s.Get(ctx, runID)
}

func (s runtimeRunStore) markCheckpoint(ctx context.Context, runID, checkpointID, column string) (RuntimeRun, error) {
	if s.db == nil {
		return RuntimeRun{}, errors.New("runtime run database is not available")
	}
	runID = strings.TrimSpace(runID)
	checkpointID = strings.TrimSpace(checkpointID)
	if runID == "" {
		return RuntimeRun{}, errors.New("run id is required")
	}
	if checkpointID == "" {
		return RuntimeRun{}, errors.New("checkpoint id is required")
	}
	if column != "acknowledged_at" && column != "discarded_at" {
		return RuntimeRun{}, errors.New("unsupported checkpoint marker")
	}
	now := time.Now().UnixMilli()
	result, err := s.db.ExecContext(ctx, fmt.Sprintf(`
UPDATE runtime_run_checkpoints
SET %[1]s = COALESCE(%[1]s, ?)
WHERE run_id = ? AND id = ?`, column), now, runID, checkpointID)
	if err != nil {
		return RuntimeRun{}, fmt.Errorf("failed to mark runtime run checkpoint: %w", err)
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return RuntimeRun{}, fmt.Errorf("failed to inspect runtime run checkpoint marker: %w", err)
	}
	if affected == 0 {
		if _, err := s.Get(ctx, runID); errors.Is(err, errRuntimeRunNotFound) {
			return RuntimeRun{}, err
		} else if err != nil {
			return RuntimeRun{}, err
		}
		return RuntimeRun{}, errRuntimeRunCheckpointNotFound
	}
	return s.Get(ctx, runID)
}

func (s runtimeRunStore) upsertSessionLinks(ctx context.Context, run RuntimeRun) error {
	sessionIDs := appendUniqueStrings([]string{run.PrimarySessionID}, run.SessionIDs...)
	for _, sessionID := range sessionIDs {
		role := runtimeRunSessionRoleChild
		if sessionID == run.PrimarySessionID {
			role = runtimeRunSessionRolePrimary
		}
		if _, err := s.db.ExecContext(ctx, `
INSERT INTO runtime_run_sessions (run_id, session_id, role, task_id, turn_id, worktree_id, created_at)
VALUES (?, ?, ?, NULL, NULL, NULL, ?)
ON CONFLICT(run_id, session_id) DO UPDATE SET
    role = CASE
        WHEN runtime_run_sessions.role = 'primary' THEN runtime_run_sessions.role
        ELSE excluded.role
    END`,
			run.ID, sessionID, role, firstPositiveInt64(run.CreatedAt, time.Now().UnixMilli()),
		); err != nil {
			return fmt.Errorf("failed to upsert runtime run session link: %w", err)
		}
	}
	return nil
}

func (s runtimeRunStore) upsertCheckpoints(ctx context.Context, run RuntimeRun) error {
	for _, checkpoint := range run.Checkpoints {
		if strings.TrimSpace(checkpoint.ID) == "" {
			continue
		}
		artifactRefs, err := encodeStringSlice(checkpoint.ArtifactRefs)
		if err != nil {
			return err
		}
		if _, err := s.db.ExecContext(ctx, `
INSERT INTO runtime_run_checkpoints (
    id, run_id, session_id, turn_id, task_id, status, summary,
    artifact_refs_json, diagnostic_refs_json, created_at, acknowledged_at,
    discarded_at, metadata_json
) VALUES (?, ?, ?, ?, ?, ?, ?, ?, NULL, ?, NULL, NULL, NULL)
ON CONFLICT(id) DO UPDATE SET
    status = excluded.status,
    summary = COALESCE(NULLIF(excluded.summary, ''), runtime_run_checkpoints.summary),
    artifact_refs_json = COALESCE(excluded.artifact_refs_json, runtime_run_checkpoints.artifact_refs_json)`,
			checkpoint.ID,
			run.ID,
			run.PrimarySessionID,
			nullableString(checkpoint.TurnID),
			nullableString(checkpoint.TaskID),
			firstNonEmpty(checkpoint.Status, runtimeRunCheckpointStatusResumable),
			nullableString(checkpoint.Summary),
			artifactRefs,
			firstPositiveInt64(checkpoint.CreatedAt, run.UpdatedAt, run.CreatedAt),
		); err != nil {
			return fmt.Errorf("failed to upsert runtime run checkpoint: %w", err)
		}
	}
	return nil
}

func (s runtimeRunStore) hydrateRunChildren(ctx context.Context, run RuntimeRun) (RuntimeRun, error) {
	sessionRows, err := s.db.QueryContext(ctx, `SELECT session_id FROM runtime_run_sessions WHERE run_id = ? ORDER BY created_at ASC`, run.ID)
	if err != nil {
		return RuntimeRun{}, fmt.Errorf("failed to list runtime run sessions: %w", err)
	}
	defer sessionRows.Close() //nolint:errcheck
	run.SessionIDs = nil
	for sessionRows.Next() {
		var sessionID string
		if err := sessionRows.Scan(&sessionID); err != nil {
			return RuntimeRun{}, err
		}
		run.SessionIDs = appendUniqueString(run.SessionIDs, sessionID)
	}
	if err := sessionRows.Err(); err != nil {
		return RuntimeRun{}, fmt.Errorf("failed to iterate runtime run sessions: %w", err)
	}
	checkpointRows, err := s.db.QueryContext(ctx, `
SELECT id, turn_id, task_id, status, summary, artifact_refs_json, created_at, acknowledged_at, discarded_at, metadata_json
FROM runtime_run_checkpoints
WHERE run_id = ?
ORDER BY created_at ASC`, run.ID)
	if err != nil {
		return RuntimeRun{}, fmt.Errorf("failed to list runtime run checkpoints: %w", err)
	}
	defer checkpointRows.Close() //nolint:errcheck
	run.Checkpoints = nil
	for checkpointRows.Next() {
		var checkpoint RuntimeRunCheckpoint
		var turnID, taskID, summary, artifactRefs sql.NullString
		var acknowledgedAt, discardedAt sql.NullInt64
		var metadata sql.NullString
		if err := checkpointRows.Scan(&checkpoint.ID, &turnID, &taskID, &checkpoint.Status, &summary, &artifactRefs, &checkpoint.CreatedAt, &acknowledgedAt, &discardedAt, &metadata); err != nil {
			return RuntimeRun{}, err
		}
		checkpoint.TurnID = turnID.String
		checkpoint.TaskID = taskID.String
		checkpoint.Summary = summary.String
		checkpoint.ArtifactRefs = decodeStringSlice(artifactRefs.String)
		if acknowledgedAt.Valid {
			checkpoint.AcknowledgedAt = acknowledgedAt.Int64
		}
		if discardedAt.Valid {
			checkpoint.DiscardedAt = discardedAt.Int64
		}
		checkpoint.ResumedTurnIDs = stringSliceFromMap(decodeJSONMap(metadata.String), "resumedTurnIds")
		checkpoint.ResumeEligible = (checkpoint.Status == runtimeRunCheckpointStatusResumable || checkpoint.Status == turnStatusInterrupted || checkpoint.Status == agentTaskStatusInterrupted) &&
			checkpoint.AcknowledgedAt == 0 &&
			checkpoint.DiscardedAt == 0
		run.Checkpoints = append(run.Checkpoints, checkpoint)
	}
	if err := checkpointRows.Err(); err != nil {
		return RuntimeRun{}, fmt.Errorf("failed to iterate runtime run checkpoints: %w", err)
	}
	return run, nil
}

func runtimeRunSelectSQL() string {
	return `
SELECT id, workspace_id, primary_session_id, objective, status,
    created_at, updated_at, finished_at, discarded_at, source
FROM runtime_runs`
}

type runtimeRunScanner interface {
	Scan(dest ...any) error
}

func scanRuntimeRun(scanner runtimeRunScanner) (RuntimeRun, error) {
	var run RuntimeRun
	var objective, source sql.NullString
	var finishedAt, discardedAt sql.NullInt64
	if err := scanner.Scan(
		&run.ID,
		&run.WorkspaceID,
		&run.PrimarySessionID,
		&objective,
		&run.Status,
		&run.CreatedAt,
		&run.UpdatedAt,
		&finishedAt,
		&discardedAt,
		&source,
	); err != nil {
		return RuntimeRun{}, err
	}
	run.Objective = objective.String
	run.Source = source.String
	if finishedAt.Valid {
		run.FinishedAt = finishedAt.Int64
	}
	if discardedAt.Valid {
		run.DiscardedAt = discardedAt.Int64
	}
	return run, nil
}

func runtimeGeneratedRunID() string {
	var bytes [12]byte
	if _, err := rand.Read(bytes[:]); err == nil {
		return "run_" + hex.EncodeToString(bytes[:])
	}
	return "run_" + strings.ReplaceAll(newRuntimeEventID(), "-", "")
}

func runtimeRunProjectionHasEvidence(projection RuntimeRunProjection) bool {
	if len(projection.Checkpoints) > 0 ||
		len(projection.ProducedArtifacts) > 0 ||
		len(projection.VerifiedArtifacts) > 0 ||
		len(projection.ExpectedArtifacts) > 0 {
		return true
	}
	diag := projection.Diagnostics
	return diag.TurnCount > 0 ||
		diag.TaskCount > 0 ||
		diag.ToolCallCount > 0 ||
		diag.PermissionRequestCount > 0
}
