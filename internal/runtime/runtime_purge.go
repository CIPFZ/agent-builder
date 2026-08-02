package runtime

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

const (
	runtimeDeleteModeHard = "hard"
	runtimeDeleteModeSoft = "soft"
)

func runtimeDeleteMode(ctx context.Context, db *sql.DB) string {
	if db == nil {
		return runtimeDeleteModeHard
	}
	var value string
	err := db.QueryRowContext(ctx, `SELECT value FROM runtime_settings WHERE key = 'delete_mode'`).Scan(&value)
	if err != nil {
		return runtimeDeleteModeHard
	}
	if strings.TrimSpace(value) == runtimeDeleteModeSoft {
		return runtimeDeleteModeSoft
	}
	return runtimeDeleteModeHard
}

func purgeRuntimeSession(ctx context.Context, db *sql.DB, sessionID string, dataDir string) error {
	sessionID = strings.TrimSpace(sessionID)
	if sessionID == "" {
		return nil
	}
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback() //nolint:errcheck
	objects, err := purgeRuntimeSessionTx(ctx, tx, sessionID, dataDir)
	if err != nil {
		return err
	}
	if err := tx.Commit(); err != nil {
		return err
	}
	deleteRuntimeObjectFiles(objects)
	return nil
}

func purgeRuntimeProject(ctx context.Context, db *sql.DB, projectID string, dataDir string) error {
	projectID = strings.TrimSpace(projectID)
	if projectID == "" {
		return nil
	}
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback() //nolint:errcheck
	rows, err := tx.QueryContext(ctx, `SELECT id FROM sessions WHERE project_id = ?`, projectID)
	if err != nil {
		return err
	}
	var sessionIDs []string
	if err := consumeRuntimeRows(rows, func(rows *sql.Rows) error {
		var id string
		if err := rows.Scan(&id); err != nil {
			return err
		}
		sessionIDs = append(sessionIDs, id)
		return nil
	}); err != nil {
		return err
	}
	var allRefs []string
	for _, sessionID := range sessionIDs {
		refs, err := purgeRuntimeSessionTx(ctx, tx, sessionID, dataDir)
		if err != nil {
			return err
		}
		allRefs = append(allRefs, refs...)
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM project_memory_injections WHERE memory_id IN (SELECT id FROM project_memory_records WHERE project_id = ?)`, projectID); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM project_memory_records WHERE project_id = ?`, projectID); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM projects WHERE id = ?`, projectID); err != nil {
		return err
	}
	if err := tx.Commit(); err != nil {
		return err
	}
	deleteRuntimeObjectFiles(allRefs)
	if dataLayout, err := newApplicationDataLayout(dataDir); err == nil {
		if projectLayout, err := dataLayout.Project(projectID); err == nil {
			_ = os.RemoveAll(projectLayout.Root)
		}
	}
	return nil
}

func purgeRuntimeSessionTx(ctx context.Context, tx *sql.Tx, sessionID string, dataDir string) ([]string, error) {
	refs, err := runtimeObjectStoragePathsForSession(ctx, tx, sessionID, dataDir)
	if err != nil {
		return nil, err
	}
	statements := []string{
		`DELETE FROM runtime_recovery_links WHERE source_turn_id IN (SELECT id FROM runtime_turns WHERE session_id = ?) OR resumed_turn_id IN (SELECT id FROM runtime_turns WHERE session_id = ?)`,
		`DELETE FROM runtime_context_circuit_states WHERE session_id = ?`,
		`DELETE FROM runtime_context_reactive_attempts WHERE session_id = ?`,
		`DELETE FROM runtime_context_warnings WHERE session_id = ?`,
		`DELETE FROM runtime_context_reinjections WHERE session_id = ?`,
		`DELETE FROM runtime_context_read_state_snapshots WHERE session_id = ?`,
		`DELETE FROM runtime_context_snip_boundaries WHERE session_id = ?`,
		`DELETE FROM runtime_context_content_replacements WHERE session_id = ?`,
		`DELETE FROM runtime_context_boundaries WHERE session_id = ?`,
		`DELETE FROM runtime_session_memory_revisions WHERE session_id = ?`,
		`DELETE FROM runtime_context_projection_messages WHERE session_id = ?`,
		`DELETE FROM runtime_context_projections WHERE session_id = ?`,
		`DELETE FROM runtime_prompt_assemblies WHERE session_id = ?`,
		`DELETE FROM runtime_user_inputs WHERE session_id = ?`,
		`DELETE FROM runtime_run_transitions WHERE session_id = ?`,
		`DELETE FROM runtime_run_checkpoints WHERE session_id = ?`,
		`DELETE FROM runtime_run_sessions WHERE session_id = ?`,
		`DELETE FROM runtime_runs WHERE primary_session_id = ?`,
		`DELETE FROM runtime_hook_executions WHERE session_id = ?`,
		`DELETE FROM runtime_sandbox_decisions WHERE session_id = ?`,
		`DELETE FROM runtime_mcp_requests WHERE session_id = ?`,
		`DELETE FROM runtime_events WHERE session_id = ?`,
		`DELETE FROM objects WHERE session_id = ?`,
		`DELETE FROM runtime_worktrees WHERE session_id = ?`,
		`DELETE FROM runtime_permission_requests WHERE session_id = ?`,
		`DELETE FROM runtime_tool_calls WHERE session_id = ?`,
		`DELETE FROM runtime_agent_task_results WHERE task_id IN (SELECT id FROM runtime_agent_tasks WHERE parent_session_id = ? OR child_session_id = ?)`,
		`DELETE FROM runtime_agent_task_messages WHERE task_id IN (SELECT id FROM runtime_agent_tasks WHERE parent_session_id = ? OR child_session_id = ?)`,
		`DELETE FROM runtime_agent_tasks WHERE parent_session_id = ? OR child_session_id = ?`,
		`DELETE FROM runtime_turns WHERE session_id = ?`,
		`DELETE FROM runtime_audit_events WHERE session_id = ?`,
		`DELETE FROM read_files WHERE session_id = ?`,
		`DELETE FROM files WHERE session_id = ?`,
		`DELETE FROM message_search_fts WHERE session_id = ?`,
		`DELETE FROM messages WHERE session_id = ?`,
		`DELETE FROM sessions WHERE id = ? OR parent_session_id = ?`,
	}
	for _, stmt := range statements {
		if strings.Count(stmt, "?") == 2 {
			if _, err := tx.ExecContext(ctx, stmt, sessionID, sessionID); err != nil {
				return nil, fmt.Errorf("purging session %s: %w", sessionID, err)
			}
			continue
		}
		if _, err := tx.ExecContext(ctx, stmt, sessionID); err != nil {
			return nil, fmt.Errorf("purging session %s: %w", sessionID, err)
		}
	}
	return refs, nil
}

func deleteRuntimeObjectFiles(paths []string) {
	for _, path := range paths {
		_ = os.Remove(path)
	}
}

func runtimeObjectStoragePathsForSession(ctx context.Context, tx *sql.Tx, sessionID string, dataDir string) ([]string, error) {
	if strings.TrimSpace(dataDir) == "" {
		return nil, nil
	}
	rows, err := tx.QueryContext(ctx, `SELECT project_id, storage_path FROM objects WHERE session_id = ? AND storage_kind = 'file' AND COALESCE(storage_path, '') <> ''`, sessionID)
	if err != nil {
		return nil, err
	}
	defer rows.Close() //nolint:errcheck
	var paths []string
	dataLayout, err := newApplicationDataLayout(dataDir)
	if err != nil {
		return nil, err
	}
	for rows.Next() {
		var projectID, rel string
		if err := rows.Scan(&projectID, &rel); err != nil {
			return nil, err
		}
		projectLayout, err := dataLayout.Project(projectID)
		if err != nil {
			continue
		}
		rootAbs, err := filepath.Abs(projectLayout.ObjectsDir)
		if err != nil {
			continue
		}
		cleanRel := filepath.Clean(filepath.FromSlash(rel))
		if cleanRel == "." || filepath.IsAbs(cleanRel) || strings.HasPrefix(cleanRel, ".."+string(filepath.Separator)) || cleanRel == ".." {
			continue
		}
		abs, err := filepath.Abs(filepath.Join(rootAbs, cleanRel))
		if err != nil {
			continue
		}
		if abs == rootAbs || strings.HasPrefix(abs, rootAbs+string(filepath.Separator)) {
			paths = append(paths, abs)
		}
	}
	return paths, rows.Err()
}
