package runtime

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/google/uuid"
)

var errRuntimeProjectNotFound = errors.New("runtime project not found")

type runtimeProjectRecord struct {
	ID              string
	Name            string
	Path            string
	CanonicalPath   string
	GitRoot         string
	Branch          string
	IsGitRepository bool
	ExistsOnDisk    bool
	CreatedAt       int64
	UpdatedAt       int64
	LastOpenedAt    int64
	DeletedAt       int64
}

type runtimeProjectStore struct {
	db      *sql.DB
	dataDir string
}

func newRuntimeProjectStore(db *sql.DB, dataDir string) runtimeProjectStore {
	return runtimeProjectStore{db: db, dataDir: filepath.Clean(dataDir)}
}

func (s runtimeProjectStore) UpsertActiveByPath(ctx context.Context, path string) (runtimeProjectRecord, error) {
	if s.db == nil {
		return runtimeProjectRecord{}, errors.New("runtime project database is not available")
	}
	path, err := normalizeRuntimeProjectPath(path)
	if err != nil {
		return runtimeProjectRecord{}, err
	}
	canonicalPath := runtimeProjectCanonicalPath(path)
	if existing, err := s.getActiveByCanonicalPath(ctx, canonicalPath); err == nil {
		return s.MarkOpened(ctx, existing.ID)
	} else if !errors.Is(err, errRuntimeProjectNotFound) {
		return runtimeProjectRecord{}, err
	}
	now := time.Now().UnixMilli()
	record := runtimeProjectRecord{
		ID:              uuid.NewString(),
		Name:            runtimeProjectName(path),
		Path:            path,
		CanonicalPath:   canonicalPath,
		GitRoot:         runtimeProjectGitRoot(path),
		Branch:          runtimeProjectBranch(path),
		IsGitRepository: runtimeProjectIsGitRepository(path),
		ExistsOnDisk:    runtimeProjectExistsOnDisk(path),
		CreatedAt:       now,
		UpdatedAt:       now,
		LastOpenedAt:    now,
	}
	_, err = s.db.ExecContext(ctx, `
INSERT INTO projects (
    id, name, path, canonical_path, git_root, branch,
    is_git_repository, exists_on_disk, created_at, updated_at, last_opened_at
) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		record.ID, record.Name, record.Path, record.CanonicalPath,
		nullableString(record.GitRoot), nullableString(record.Branch),
		runtimeProjectBoolInt(record.IsGitRepository), runtimeProjectBoolInt(record.ExistsOnDisk),
		record.CreatedAt, record.UpdatedAt, record.LastOpenedAt,
	)
	if err != nil {
		return runtimeProjectRecord{}, fmt.Errorf("failed to create project record: %w", err)
	}
	return s.Get(ctx, record.ID)
}

func (s runtimeProjectStore) Get(ctx context.Context, id string) (runtimeProjectRecord, error) {
	id = strings.TrimSpace(id)
	if id == "" {
		return runtimeProjectRecord{}, errRuntimeProjectNotFound
	}
	row := s.db.QueryRowContext(ctx, `
SELECT id, name, path, canonical_path, COALESCE(git_root, ''), COALESCE(branch, ''),
    is_git_repository, exists_on_disk, created_at, updated_at, COALESCE(last_opened_at, 0), COALESCE(deleted_at, 0)
FROM projects
WHERE id = ?`, id)
	record, err := scanRuntimeProjectRecord(row)
	if errors.Is(err, sql.ErrNoRows) {
		return runtimeProjectRecord{}, errRuntimeProjectNotFound
	}
	return record, err
}

func (s runtimeProjectStore) GetActive(ctx context.Context, id string) (runtimeProjectRecord, error) {
	record, err := s.Get(ctx, id)
	if err != nil {
		return runtimeProjectRecord{}, err
	}
	if record.DeletedAt > 0 {
		return runtimeProjectRecord{}, errRuntimeProjectNotFound
	}
	return record, nil
}

func (s runtimeProjectStore) ListActive(ctx context.Context) ([]runtimeProjectRecord, error) {
	rows, err := s.db.QueryContext(ctx, `
SELECT id, name, path, canonical_path, COALESCE(git_root, ''), COALESCE(branch, ''),
    is_git_repository, exists_on_disk, created_at, updated_at, COALESCE(last_opened_at, 0), COALESCE(deleted_at, 0)
FROM projects
WHERE deleted_at IS NULL
ORDER BY COALESCE(last_opened_at, updated_at) DESC, name ASC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close() //nolint:errcheck
	var records []runtimeProjectRecord
	for rows.Next() {
		record, err := scanRuntimeProjectRecord(rows)
		if err != nil {
			return nil, err
		}
		records = append(records, record)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return records, nil
}

func (s runtimeProjectStore) MarkOpened(ctx context.Context, id string) (runtimeProjectRecord, error) {
	record, err := s.GetActive(ctx, id)
	if err != nil {
		return runtimeProjectRecord{}, err
	}
	now := time.Now().UnixMilli()
	_, err = s.db.ExecContext(ctx, `
UPDATE projects
SET name = ?, branch = ?, is_git_repository = ?, exists_on_disk = ?, updated_at = ?, last_opened_at = ?
WHERE id = ? AND deleted_at IS NULL`,
		runtimeProjectName(record.Path),
		nullableString(runtimeProjectBranch(record.Path)),
		runtimeProjectBoolInt(runtimeProjectIsGitRepository(record.Path)),
		runtimeProjectBoolInt(runtimeProjectExistsOnDisk(record.Path)),
		now, now, id,
	)
	if err != nil {
		return runtimeProjectRecord{}, err
	}
	return s.GetActive(ctx, id)
}

func (s runtimeProjectStore) SoftDelete(ctx context.Context, id string) error {
	now := time.Now().UnixMilli()
	result, err := s.db.ExecContext(ctx, `
UPDATE projects SET deleted_at = ?, updated_at = ? WHERE id = ? AND deleted_at IS NULL`, now, now, strings.TrimSpace(id))
	if err != nil {
		return err
	}
	affected, _ := result.RowsAffected()
	if affected == 0 {
		return errRuntimeProjectNotFound
	}
	_, err = s.db.ExecContext(ctx, `
UPDATE sessions SET deleted_at = ?, status = 'deleted' WHERE project_id = ? AND deleted_at IS NULL`, now, strings.TrimSpace(id))
	return err
}

func (s runtimeProjectStore) getActiveByCanonicalPath(ctx context.Context, canonicalPath string) (runtimeProjectRecord, error) {
	row := s.db.QueryRowContext(ctx, `
SELECT id, name, path, canonical_path, COALESCE(git_root, ''), COALESCE(branch, ''),
    is_git_repository, exists_on_disk, created_at, updated_at, COALESCE(last_opened_at, 0), COALESCE(deleted_at, 0)
FROM projects
WHERE canonical_path = ? AND deleted_at IS NULL
LIMIT 1`, canonicalPath)
	record, err := scanRuntimeProjectRecord(row)
	if errors.Is(err, sql.ErrNoRows) {
		return runtimeProjectRecord{}, errRuntimeProjectNotFound
	}
	return record, err
}

type runtimeProjectScanner interface {
	Scan(dest ...any) error
}

func scanRuntimeProjectRecord(scanner runtimeProjectScanner) (runtimeProjectRecord, error) {
	var record runtimeProjectRecord
	var isGit, exists int64
	if err := scanner.Scan(
		&record.ID,
		&record.Name,
		&record.Path,
		&record.CanonicalPath,
		&record.GitRoot,
		&record.Branch,
		&isGit,
		&exists,
		&record.CreatedAt,
		&record.UpdatedAt,
		&record.LastOpenedAt,
		&record.DeletedAt,
	); err != nil {
		return runtimeProjectRecord{}, err
	}
	record.IsGitRepository = isGit != 0
	record.ExistsOnDisk = exists != 0
	return record, nil
}

func runtimeProjectCanonicalPath(path string) string {
	cleaned := filepath.Clean(path)
	if evaluated, err := filepath.EvalSymlinks(cleaned); err == nil {
		cleaned = filepath.Clean(evaluated)
	}
	return strings.ToLower(cleaned)
}

func runtimeProjectExistsOnDisk(path string) bool {
	info, err := os.Stat(path)
	return err == nil && info.IsDir()
}

func runtimeProjectGitRoot(path string) string {
	dir := filepath.Clean(path)
	for {
		if info, err := os.Stat(filepath.Join(dir, ".git")); err == nil && info.IsDir() {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return ""
		}
		dir = parent
	}
}

func runtimeProjectRecordToDTO(record runtimeProjectRecord, currentID string) RuntimeProject {
	return RuntimeProject{
		ID:              record.ID,
		Name:            record.Name,
		Path:            record.Path,
		CanonicalPath:   record.CanonicalPath,
		IsGitRepository: record.IsGitRepository,
		Branch:          record.Branch,
		Current:         record.ID != "" && record.ID == currentID,
		ExistsOnDisk:    record.ExistsOnDisk,
		CreatedAt:       record.CreatedAt,
		UpdatedAt:       record.UpdatedAt,
		LastOpenedAt:    record.LastOpenedAt,
		DeletedAt:       record.DeletedAt,
	}
}

func runtimeProjectBoolInt(value bool) int {
	if value {
		return 1
	}
	return 0
}
