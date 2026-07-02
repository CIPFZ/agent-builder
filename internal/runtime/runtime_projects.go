package runtime

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	goruntime "runtime"
	"strings"

	"github.com/CIPFZ/agent-builder/internal/db"
)

var runtimeOpenPathInFileManager = defaultRuntimeOpenPathInFileManager

func (r *runtimeService) CreateProject(ctx context.Context, req RuntimeCreateProjectRequest) (RuntimeOpenProjectResponse, error) {
	name, err := normalizeRuntimeProjectName(req.Name)
	if err != nil {
		return RuntimeOpenProjectResponse{}, err
	}
	layout, err := resolveDesktopLayout()
	if err != nil {
		return RuntimeOpenProjectResponse{}, err
	}
	projectRoot := filepath.Join(layout.DataDir, "projects")
	projectPath := filepath.Join(projectRoot, name)
	if err := ensureRuntimeProjectPathUnderRoot(projectRoot, projectPath); err != nil {
		return RuntimeOpenProjectResponse{}, err
	}
	if err := os.MkdirAll(projectRoot, 0o700); err != nil {
		return RuntimeOpenProjectResponse{}, fmt.Errorf("failed to create projects directory: %w", err)
	}
	if _, err := os.Stat(projectPath); err == nil {
		return RuntimeOpenProjectResponse{}, fmt.Errorf("project directory already exists: %s", projectPath)
	} else if err != nil && !os.IsNotExist(err) {
		return RuntimeOpenProjectResponse{}, fmt.Errorf("failed to read project directory: %w", err)
	}
	return r.OpenProject(ctx, RuntimeOpenProjectRequest{Path: projectPath, CreateMissing: true})
}

func (r *runtimeService) RenameProject(ctx context.Context, req RuntimeRenameProjectRequest) (RuntimeOpenProjectResponse, error) {
	name, err := normalizeRuntimeProjectName(req.Name)
	if err != nil {
		return RuntimeOpenProjectResponse{}, err
	}
	status, err := r.Status(ctx)
	if err != nil {
		return RuntimeOpenProjectResponse{}, err
	}
	r.mu.Lock()
	activeProjectID := r.activeProjectID
	r.mu.Unlock()
	if strings.TrimSpace(req.ProjectID) != "" && req.ProjectID != activeProjectID {
		return RuntimeOpenProjectResponse{}, fmt.Errorf("project %s is not the active project", req.ProjectID)
	}
	oldPath, err := normalizeRuntimeProjectPath(status.WorkingDir)
	if err != nil {
		return RuntimeOpenProjectResponse{}, err
	}
	if err := requireRuntimeProjectDirectory(oldPath); err != nil {
		return RuntimeOpenProjectResponse{}, err
	}
	newPath := filepath.Join(filepath.Dir(oldPath), name)
	if err := ensureRuntimeProjectPathUnderRoot(filepath.Dir(oldPath), newPath); err != nil {
		return RuntimeOpenProjectResponse{}, err
	}
	if filepath.Clean(newPath) == filepath.Clean(oldPath) {
		return RuntimeOpenProjectResponse{
			Project: runtimeProjectFromStatus(status),
			Status:  status,
		}, nil
	}
	if _, err := os.Stat(newPath); err == nil {
		return RuntimeOpenProjectResponse{}, fmt.Errorf("project directory already exists: %s", newPath)
	} else if err != nil && !os.IsNotExist(err) {
		return RuntimeOpenProjectResponse{}, fmt.Errorf("failed to read project directory: %w", err)
	}

	r.closeRuntimeTerminals("closed", "project renamed")
	if err := os.Rename(oldPath, newPath); err != nil {
		_, _ = r.OpenProject(ctx, RuntimeOpenProjectRequest{Path: oldPath})
		return RuntimeOpenProjectResponse{}, fmt.Errorf("failed to rename project directory: %w", err)
	}
	return r.OpenProject(ctx, RuntimeOpenProjectRequest{Path: newPath})
}

func (r *runtimeService) OpenProjectInExplorer(ctx context.Context, req RuntimeProjectActionRequest) (RuntimeOpenProjectResponse, error) {
	store, err := r.projectStore(ctx)
	if err != nil {
		return RuntimeOpenProjectResponse{}, err
	}
	projectID := strings.TrimSpace(req.ProjectID)
	if projectID == "" {
		r.mu.Lock()
		projectID = r.activeProjectID
		r.mu.Unlock()
	}
	project, err := store.GetActive(ctx, projectID)
	if err != nil {
		return RuntimeOpenProjectResponse{}, err
	}
	if err := requireRuntimeProjectDirectory(project.Path); err != nil {
		return RuntimeOpenProjectResponse{}, err
	}
	if err := runtimeOpenPathInFileManager(project.Path); err != nil {
		return RuntimeOpenProjectResponse{}, err
	}
	status, err := r.Status(ctx)
	if err != nil {
		return RuntimeOpenProjectResponse{}, err
	}
	r.mu.Lock()
	activeID := r.activeProjectID
	r.mu.Unlock()
	return RuntimeOpenProjectResponse{
		Project: runtimeProjectRecordToDTO(project, activeID),
		Status:  status,
	}, nil
}

func (r *runtimeService) RemoveProject(ctx context.Context, req RuntimeProjectActionRequest) (RuntimeOpenProjectResponse, error) {
	store, err := r.projectStore(ctx)
	if err != nil {
		return RuntimeOpenProjectResponse{}, err
	}
	projectID := strings.TrimSpace(req.ProjectID)
	if projectID == "" {
		r.mu.Lock()
		projectID = r.activeProjectID
		r.mu.Unlock()
	}
	project, err := store.GetActive(ctx, projectID)
	if err != nil {
		return RuntimeOpenProjectResponse{}, err
	}

	r.mu.Lock()
	wasActive := r.activeProjectID == project.ID
	if wasActive {
		r.projectPath = ""
		r.activeProjectID = ""
		r.sessionID = ""
	}
	r.mu.Unlock()

	mode := runtimeDeleteMode(ctx, store.db)
	if mode == runtimeDeleteModeSoft {
		if err := store.SoftDelete(ctx, project.ID); err != nil {
			return RuntimeOpenProjectResponse{}, err
		}
	} else {
		if err := purgeRuntimeProject(ctx, store.db, project.ID, filepath.Join(store.dataDir, "runtime_refs")); err != nil {
			return RuntimeOpenProjectResponse{}, fmt.Errorf("failed to purge project data: %w", err)
		}
	}

	nextStatus, err := r.Status(ctx)
	if err != nil {
		return RuntimeOpenProjectResponse{}, err
	}
	if wasActive && filepath.Clean(nextStatus.WorkingDir) == filepath.Clean(project.Path) {
		nextStatus.WorkingDir = runtimeDefaultWorkingDir()
		nextStatus.ExplicitProject = false
	}
	return RuntimeOpenProjectResponse{
		Project: runtimeProjectFromStatus(nextStatus),
		Status:  nextStatus,
	}, nil
}

func (r *runtimeService) OpenProject(ctx context.Context, req RuntimeOpenProjectRequest) (RuntimeOpenProjectResponse, error) {
	projectPath, err := normalizeRuntimeProjectPath(req.Path)
	if err != nil {
		return RuntimeOpenProjectResponse{}, err
	}
	if req.CreateMissing {
		if err := os.MkdirAll(projectPath, 0o755); err != nil {
			return RuntimeOpenProjectResponse{}, fmt.Errorf("failed to create project directory: %w", err)
		}
	}
	if err := requireRuntimeProjectDirectory(projectPath); err != nil {
		return RuntimeOpenProjectResponse{}, err
	}
	store, err := r.projectStore(ctx)
	if err != nil {
		return RuntimeOpenProjectResponse{}, err
	}
	project, err := store.UpsertActiveByPath(ctx, projectPath)
	if err != nil {
		return RuntimeOpenProjectResponse{}, err
	}

	r.mu.Lock()
	r.projectPath = projectPath
	r.activeProjectID = project.ID
	r.sessionID = ""
	r.mu.Unlock()

	status, err := r.Status(ctx)
	if err != nil {
		return RuntimeOpenProjectResponse{}, err
	}
	return RuntimeOpenProjectResponse{
		Project: runtimeProjectRecordToDTO(project, project.ID),
		Status:  status,
	}, nil
}

func (r *runtimeService) Projects(ctx context.Context) (RuntimeProjectsResponse, error) {
	store, err := r.projectStore(ctx)
	if err != nil {
		return RuntimeProjectsResponse{}, err
	}
	records, err := store.ListActive(ctx)
	if err != nil {
		return RuntimeProjectsResponse{}, err
	}
	r.mu.Lock()
	activeID := r.activeProjectID
	r.mu.Unlock()
	projects := make([]RuntimeProject, 0, len(records))
	for _, record := range records {
		projects = append(projects, runtimeProjectRecordToDTO(record, activeID))
	}
	return RuntimeProjectsResponse{Projects: projects}, nil
}

func (r *runtimeService) SidebarProjection(ctx context.Context) (RuntimeSidebarProjectionResponse, error) {
	projects, err := r.Projects(ctx)
	if err != nil {
		return RuntimeSidebarProjectionResponse{}, err
	}
	sessions, err := r.Sessions(ctx)
	if err != nil {
		return RuntimeSidebarProjectionResponse{}, err
	}
	r.mu.Lock()
	activeProjectID := r.activeProjectID
	activeSessionID := r.sessionID
	r.mu.Unlock()
	return RuntimeSidebarProjectionResponse{
		Projects:         projects.Projects,
		Sessions:         sessions.Sessions,
		CurrentProjectID: activeProjectID,
		ActiveSessionID:  activeSessionID,
	}, nil
}

func (r *runtimeService) projectStore(ctx context.Context) (runtimeProjectStore, error) {
	layout, err := resolveDesktopLayout()
	if err != nil {
		return runtimeProjectStore{}, err
	}
	if err := ensureDesktopLayout(layout); err != nil {
		return runtimeProjectStore{}, err
	}
	conn, err := db.Connect(ctx, layout.DataDir)
	if err != nil {
		return runtimeProjectStore{}, err
	}
	return newRuntimeProjectStore(conn, layout.DataDir), nil
}

func runtimeFallbackWorkspaceID(projectPath string) string {
	sum := sha256.Sum256([]byte(strings.ToLower(filepath.Clean(projectPath))))
	return "project-" + hex.EncodeToString(sum[:8])
}

func runtimeDefaultWorkingDir() string {
	workingDir, err := os.Getwd()
	if err != nil {
		return ""
	}
	return filepath.Clean(workingDir)
}

func normalizeRuntimeProjectPath(path string) (string, error) {
	path = strings.TrimSpace(path)
	if path == "" {
		return "", errors.New("project path is required")
	}
	cleaned := filepath.Clean(path)
	if !filepath.IsAbs(cleaned) {
		abs, err := filepath.Abs(cleaned)
		if err != nil {
			return "", fmt.Errorf("failed to resolve project path: %w", err)
		}
		cleaned = abs
	}
	return cleaned, nil
}

func normalizeRuntimeProjectName(name string) (string, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return "", errors.New("project name is required")
	}
	if name == "." || name == ".." {
		return "", errors.New("project name cannot be . or ..")
	}
	if strings.ContainsAny(name, `/\:*?"<>|`) {
		return "", errors.New("project name cannot contain path separators or Windows reserved characters")
	}
	if strings.HasSuffix(name, ".") {
		return "", errors.New("project name cannot end with a dot")
	}
	if isWindowsReservedProjectName(name) {
		return "", errors.New("project name cannot use a Windows reserved device name")
	}
	if filepath.Clean(name) != name {
		return "", errors.New("project name must be a single folder name")
	}
	return name, nil
}

func isWindowsReservedProjectName(name string) bool {
	base := strings.ToUpper(strings.TrimSpace(name))
	if dot := strings.IndexByte(base, '.'); dot >= 0 {
		base = base[:dot]
	}
	switch base {
	case "CON", "PRN", "AUX", "NUL":
		return true
	}
	if len(base) == 4 {
		prefix := base[:3]
		suffix := base[3]
		return (prefix == "COM" || prefix == "LPT") && suffix >= '1' && suffix <= '9'
	}
	return false
}

func ensureRuntimeProjectPathUnderRoot(root, path string) error {
	root = filepath.Clean(root)
	path = filepath.Clean(path)
	rel, err := filepath.Rel(root, path)
	if err != nil {
		return fmt.Errorf("failed to verify project path: %w", err)
	}
	if rel == "." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) || rel == ".." || filepath.IsAbs(rel) {
		return fmt.Errorf("project path must stay under projects directory: %s", root)
	}
	return nil
}

func requireRuntimeProjectDirectory(path string) error {
	info, err := os.Stat(path)
	if err != nil {
		if os.IsNotExist(err) {
			return fmt.Errorf("project directory does not exist: %s", path)
		}
		return fmt.Errorf("failed to read project directory: %w", err)
	}
	if !info.IsDir() {
		return fmt.Errorf("project path is not a directory: %s", path)
	}
	return nil
}

func defaultRuntimeOpenPathInFileManager(path string) error {
	switch goruntime.GOOS {
	case "windows":
		return exec.Command("explorer.exe", path).Start()
	case "darwin":
		return exec.Command("open", path).Start()
	default:
		return exec.Command("xdg-open", path).Start()
	}
}

func renameRuntimeProjectDataDir(oldDataDir, newDataDir string) error {
	oldDataDir = filepath.Clean(oldDataDir)
	newDataDir = filepath.Clean(newDataDir)
	if oldDataDir == newDataDir {
		return nil
	}
	if _, err := os.Stat(oldDataDir); err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return fmt.Errorf("failed to read project data directory: %w", err)
	}
	if _, err := os.Stat(newDataDir); err == nil {
		return fmt.Errorf("project data directory already exists: %s", newDataDir)
	} else if err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("failed to read target project data directory: %w", err)
	}
	if err := os.MkdirAll(filepath.Dir(newDataDir), 0o700); err != nil {
		return fmt.Errorf("failed to create project data parent directory: %w", err)
	}
	if err := os.Rename(oldDataDir, newDataDir); err != nil {
		return fmt.Errorf("failed to rename project data directory: %w", err)
	}
	return nil
}

func runtimeProjectFromStatus(status RuntimeStatus) RuntimeProject {
	path := filepath.Clean(status.WorkingDir)
	return RuntimeProject{
		ID:              status.WorkspaceID,
		Name:            runtimeProjectName(path),
		Path:            path,
		CanonicalPath:   runtimeProjectCanonicalPath(path),
		IsGitRepository: runtimeProjectIsGitRepository(path),
		Branch:          runtimeProjectBranch(path),
		Current:         true,
		ExistsOnDisk:    runtimeProjectExistsOnDisk(path),
	}
}

func runtimeProjectName(path string) string {
	name := filepath.Base(filepath.Clean(path))
	if strings.TrimSpace(name) == "." || strings.TrimSpace(name) == string(filepath.Separator) {
		return filepath.Clean(path)
	}
	return name
}

func runtimeProjectIsGitRepository(path string) bool {
	info, err := os.Stat(filepath.Join(path, ".git"))
	return err == nil && info.IsDir()
}

func runtimeProjectBranch(path string) string {
	data, err := os.ReadFile(filepath.Join(path, ".git", "HEAD"))
	if err != nil {
		return ""
	}
	head := strings.TrimSpace(string(data))
	if branch, ok := strings.CutPrefix(head, "ref: refs/heads/"); ok {
		return strings.TrimSpace(branch)
	}
	if len(head) >= 7 {
		return head[:7]
	}
	return head
}
