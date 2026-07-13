package runtime

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

type RuntimeProjectStorageDiagnostics struct {
	ProjectID      string   `json:"projectId"`
	Root           string   `json:"root"`
	ObjectCount    int      `json:"objectCount"`
	ObjectBytes    int64    `json:"objectBytes"`
	MissingObjects []string `json:"missingObjects,omitempty"`
	OrphanFiles    []string `json:"orphanFiles,omitempty"`
}

func (r *runtimeService) ProjectStorageDiagnostics(ctx context.Context, projectID string) (RuntimeProjectStorageDiagnostics, error) {
	store, err := r.projectStore(ctx)
	if err != nil {
		return RuntimeProjectStorageDiagnostics{}, err
	}
	projectID = strings.TrimSpace(firstNonEmpty(projectID, r.activeProjectID))
	if _, err := store.GetActive(ctx, projectID); err != nil {
		return RuntimeProjectStorageDiagnostics{}, err
	}
	appLayout, err := newApplicationDataLayout(store.dataDir)
	if err != nil {
		return RuntimeProjectStorageDiagnostics{}, err
	}
	projectLayout, err := appLayout.Project(projectID)
	if err != nil {
		return RuntimeProjectStorageDiagnostics{}, err
	}
	report := RuntimeProjectStorageDiagnostics{ProjectID: projectID, Root: projectLayout.Root}
	objectStore := newRuntimeObjectStore(store.db, store.dataDir)
	rows, err := store.db.QueryContext(ctx, `SELECT id, storage_kind, COALESCE(storage_path, ''), size_bytes FROM objects WHERE project_id = ?`, projectID)
	if err != nil {
		return report, fmt.Errorf("list project objects: %w", err)
	}
	referenced := map[string]struct{}{}
	for rows.Next() {
		var id, storageKind, storagePath string
		var size int64
		if err := rows.Scan(&id, &storageKind, &storagePath, &size); err != nil {
			rows.Close() //nolint:errcheck
			return report, err
		}
		report.ObjectCount++
		report.ObjectBytes += size
		if storageKind != runtimeObjectStorageFile {
			continue
		}
		abs, err := objectStore.resolveStoragePath(projectID, storagePath)
		if err != nil {
			report.MissingObjects = append(report.MissingObjects, id)
			continue
		}
		referenced[filepath.Clean(abs)] = struct{}{}
		if info, err := os.Stat(abs); err != nil || info.IsDir() {
			report.MissingObjects = append(report.MissingObjects, id)
		}
	}
	if err := rows.Close(); err != nil {
		return report, err
	}
	if err := filepath.WalkDir(projectLayout.ObjectsDir, func(path string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil || entry.IsDir() {
			return walkErr
		}
		if _, ok := referenced[filepath.Clean(path)]; !ok {
			rel, _ := filepath.Rel(projectLayout.ObjectsDir, path)
			report.OrphanFiles = append(report.OrphanFiles, filepath.ToSlash(rel))
		}
		return nil
	}); err != nil && !os.IsNotExist(err) {
		return report, err
	}
	return report, rows.Err()
}
