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
	if strings.TrimSpace(req.ProjectID) != "" && req.ProjectID != status.WorkspaceID {
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

	layout, err := resolveDesktopLayout()
	if err != nil {
		return RuntimeOpenProjectResponse{}, err
	}
	oldDataDir := runtimeProjectDataDir(layout.DataDir, oldPath)
	newDataDir := runtimeProjectDataDir(layout.DataDir, newPath)

	r.startMu.Lock()
	r.closeRuntimeTerminals("closed", "project renamed")
	r.mu.Lock()
	if r.runtime != nil && r.workspace != nil {
		r.runtime.DeleteWorkspace(r.workspace.ID)
	}
	if r.cancel != nil {
		r.cancel()
	}
	r.runtime = nil
	r.workspace = nil
	r.runtimeConfigured = false
	r.runtimeConfigKnown = false
	r.runtimeCtx = nil
	r.cancel = nil
	r.starting = false
	r.mu.Unlock()
	r.startMu.Unlock()
	if err := db.Release(oldDataDir); err != nil {
		return RuntimeOpenProjectResponse{}, fmt.Errorf("failed to release project database: %w", err)
	}

	if err := os.Rename(oldPath, newPath); err != nil {
		_, _ = r.OpenProject(ctx, RuntimeOpenProjectRequest{Path: oldPath})
		return RuntimeOpenProjectResponse{}, fmt.Errorf("failed to rename project directory: %w", err)
	}
	if err := renameRuntimeProjectDataDir(oldDataDir, newDataDir); err != nil {
		_ = os.Rename(newPath, oldPath)
		_, _ = r.OpenProject(ctx, RuntimeOpenProjectRequest{Path: oldPath})
		return RuntimeOpenProjectResponse{}, err
	}
	return r.OpenProject(ctx, RuntimeOpenProjectRequest{Path: newPath})
}

func (r *runtimeService) OpenProjectInExplorer(ctx context.Context, req RuntimeProjectActionRequest) (RuntimeOpenProjectResponse, error) {
	status, err := r.Status(ctx)
	if err != nil {
		return RuntimeOpenProjectResponse{}, err
	}
	if strings.TrimSpace(req.ProjectID) != "" && req.ProjectID != status.WorkspaceID {
		return RuntimeOpenProjectResponse{}, fmt.Errorf("project %s is not the active project", req.ProjectID)
	}
	projectPath, err := normalizeRuntimeProjectPath(status.WorkingDir)
	if err != nil {
		return RuntimeOpenProjectResponse{}, err
	}
	if err := requireRuntimeProjectDirectory(projectPath); err != nil {
		return RuntimeOpenProjectResponse{}, err
	}
	if err := runtimeOpenPathInFileManager(projectPath); err != nil {
		return RuntimeOpenProjectResponse{}, err
	}
	return RuntimeOpenProjectResponse{
		Project: runtimeProjectFromStatus(status),
		Status:  status,
	}, nil
}

func (r *runtimeService) RemoveProject(ctx context.Context, req RuntimeProjectActionRequest) (RuntimeOpenProjectResponse, error) {
	status, err := r.Status(ctx)
	if err != nil {
		return RuntimeOpenProjectResponse{}, err
	}
	if strings.TrimSpace(req.ProjectID) != "" && req.ProjectID != status.WorkspaceID {
		return RuntimeOpenProjectResponse{}, fmt.Errorf("project %s is not the active project", req.ProjectID)
	}
	projectPath, err := normalizeRuntimeProjectPath(status.WorkingDir)
	if err != nil {
		return RuntimeOpenProjectResponse{}, err
	}
	if err := requireRuntimeProjectDirectory(projectPath); err != nil {
		return RuntimeOpenProjectResponse{}, err
	}
	layout, err := resolveDesktopLayout()
	if err != nil {
		return RuntimeOpenProjectResponse{}, err
	}
	projectDataRoot := filepath.Join(layout.DataDir, "projects")
	projectDataDir := runtimeProjectDataDir(layout.DataDir, projectPath)
	if err := ensureRuntimeProjectPathUnderRoot(projectDataRoot, projectDataDir); err != nil {
		return RuntimeOpenProjectResponse{}, err
	}

	r.startMu.Lock()
	r.closeRuntimeTerminals("closed", "project removed")
	r.mu.Lock()
	if r.runtime != nil && r.workspace != nil {
		r.runtime.DeleteWorkspace(r.workspace.ID)
	}
	if r.cancel != nil {
		r.cancel()
	}
	r.runtime = nil
	r.workspace = nil
	r.runtimeConfigured = false
	r.runtimeConfigKnown = false
	r.projectPath = ""
	r.starting = false
	r.sessionID = ""
	r.runtimeCtx = nil
	r.cancel = nil
	r.eventStats = runtimeEventStats{}
	r.requests = make(map[string]runtimeRequestState)
	r.sessionTurns = make(map[string]string)
	r.toolEvents = make(map[string]runtimeToolEventState)
	r.toolCalls = nil
	r.refs = runtimeRefStore{}
	r.compactBoundaries = runtimeCompactBoundaryStore{}
	r.worktrees = runtimeWorktreeStore{}
	r.sandboxDecisions = runtimeSandboxDecisionStore{}
	r.hookExecutions = runtimeHookExecutionStore{}
	r.agentTasks = runtimeAgentTaskStore{}
	r.turns = runtimeTurnStore{}
	r.userInputs = runtimeUserInputStore{}
	r.eventStore = runtimeEventStore{}
	r.permissionStore = runtimePermissionStore{}
	r.mcpRequestStore = runtimeMCPRequestStore{}
	r.runs = runtimeRunStore{}
	r.transitions = runtimeRunTransitionStore{}
	r.permissions = make(map[string]pendingRuntimePermission)
	r.policy = defaultRuntimePolicy()
	r.capabilityLoads = make(map[string]runtimeCapabilityLoadRecord)
	r.terminalsByID = make(map[string]*runtimeTerminalState)
	r.terminalIDsBySession = make(map[string]map[string]struct{})
	r.recovery = runtimeRecoveryRecord{}
	r.events = nil
	r.mu.Unlock()
	r.startMu.Unlock()
	if err := db.Release(projectDataDir); err != nil {
		return RuntimeOpenProjectResponse{}, fmt.Errorf("failed to release project database: %w", err)
	}
	if err := os.RemoveAll(projectDataDir); err != nil {
		return RuntimeOpenProjectResponse{}, fmt.Errorf("failed to remove project application data: %w", err)
	}

	nextStatus, err := r.Status(ctx)
	if err != nil {
		return RuntimeOpenProjectResponse{}, err
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

	r.startMu.Lock()
	r.closeRuntimeTerminals("closed", "project switched")
	r.mu.Lock()
	if r.runtime != nil && r.workspace != nil {
		r.runtime.DeleteWorkspace(r.workspace.ID)
	}
	if r.cancel != nil {
		r.cancel()
	}
	r.runtime = nil
	r.workspace = nil
	r.runtimeConfigured = false
	r.runtimeConfigKnown = false
	r.projectPath = projectPath
	r.starting = false
	r.sessionID = ""
	r.runtimeCtx = nil
	r.cancel = nil
	r.eventStats = runtimeEventStats{}
	r.requests = make(map[string]runtimeRequestState)
	r.sessionTurns = make(map[string]string)
	r.toolEvents = make(map[string]runtimeToolEventState)
	r.toolCalls = nil
	r.refs = runtimeRefStore{}
	r.compactBoundaries = runtimeCompactBoundaryStore{}
	r.worktrees = runtimeWorktreeStore{}
	r.sandboxDecisions = runtimeSandboxDecisionStore{}
	r.hookExecutions = runtimeHookExecutionStore{}
	r.agentTasks = runtimeAgentTaskStore{}
	r.turns = runtimeTurnStore{}
	r.userInputs = runtimeUserInputStore{}
	r.eventStore = runtimeEventStore{}
	r.permissionStore = runtimePermissionStore{}
	r.mcpRequestStore = runtimeMCPRequestStore{}
	r.runs = runtimeRunStore{}
	r.transitions = runtimeRunTransitionStore{}
	r.permissions = make(map[string]pendingRuntimePermission)
	r.policy = defaultRuntimePolicy()
	r.capabilityLoads = make(map[string]runtimeCapabilityLoadRecord)
	r.terminalsByID = make(map[string]*runtimeTerminalState)
	r.terminalIDsBySession = make(map[string]map[string]struct{})
	r.recovery = runtimeRecoveryRecord{}
	r.events = nil
	r.mu.Unlock()
	r.startMu.Unlock()

	status, err := r.Status(ctx)
	if err != nil {
		return RuntimeOpenProjectResponse{}, err
	}
	return RuntimeOpenProjectResponse{
		Project: runtimeProjectFromStatus(status),
		Status:  status,
	}, nil
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
		IsGitRepository: runtimeProjectIsGitRepository(path),
		Branch:          runtimeProjectBranch(path),
		Current:         true,
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

func runtimeProjectDataDir(root, projectPath string) string {
	sum := sha256.Sum256([]byte(strings.ToLower(filepath.Clean(projectPath))))
	name := runtimeProjectName(projectPath)
	name = strings.Map(func(r rune) rune {
		switch {
		case r >= 'a' && r <= 'z':
			return r
		case r >= 'A' && r <= 'Z':
			return r
		case r >= '0' && r <= '9':
			return r
		case r == '-' || r == '_':
			return r
		default:
			return '-'
		}
	}, name)
	name = strings.Trim(name, "-")
	if name == "" {
		name = "project"
	}
	return filepath.Join(root, "projects", name+"-"+hex.EncodeToString(sum[:8]))
}
