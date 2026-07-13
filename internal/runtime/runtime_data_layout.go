package runtime

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

type applicationDataLayout struct {
	Root              string
	DatabaseDir       string
	ProjectsDir       string
	ManagedWorkspaces string
	GlobalDir         string
	CacheDir          string
	BackupsDir        string
}

type projectDataLayout struct {
	Root         string
	MemoryDir    string
	SkillsDir    string
	MCPDir       string
	ObjectsDir   string
	SessionsDir  string
	WorktreesDir string
}

type sessionDataLayout struct {
	Root           string
	EnvironmentDir string
	DownloadsDir   string
	TempDir        string
}

func newApplicationDataLayout(dataDir string) (applicationDataLayout, error) {
	root, err := cleanAbsoluteRoot(dataDir)
	if err != nil {
		return applicationDataLayout{}, fmt.Errorf("invalid application data root: %w", err)
	}
	return applicationDataLayout{
		Root:              root,
		DatabaseDir:       root,
		ProjectsDir:       filepath.Join(root, "projects"),
		ManagedWorkspaces: filepath.Join(root, "managed-workspaces"),
		GlobalDir:         filepath.Join(root, "global"),
		CacheDir:          filepath.Join(root, "cache"),
		BackupsDir:        filepath.Join(root, "backups"),
	}, nil
}

func (l applicationDataLayout) Project(projectID string) (projectDataLayout, error) {
	projectID, err := safeDataSegment("project id", projectID)
	if err != nil {
		return projectDataLayout{}, err
	}
	root := filepath.Join(l.ProjectsDir, projectID)
	return projectDataLayout{
		Root:         root,
		MemoryDir:    filepath.Join(root, "memory"),
		SkillsDir:    filepath.Join(root, "skills"),
		MCPDir:       filepath.Join(root, "mcp"),
		ObjectsDir:   filepath.Join(root, "objects"),
		SessionsDir:  filepath.Join(root, "sessions"),
		WorktreesDir: filepath.Join(root, "worktrees"),
	}, nil
}

func (l projectDataLayout) Session(sessionID string) (sessionDataLayout, error) {
	sessionID, err := safeDataSegment("session id", sessionID)
	if err != nil {
		return sessionDataLayout{}, err
	}
	root := filepath.Join(l.SessionsDir, sessionID)
	return sessionDataLayout{
		Root:           root,
		EnvironmentDir: filepath.Join(root, "environment"),
		DownloadsDir:   filepath.Join(root, "downloads"),
		TempDir:        filepath.Join(root, "tmp"),
	}, nil
}

func ensureApplicationDataLayout(layout applicationDataLayout) error {
	for _, dir := range []string{layout.Root, layout.ProjectsDir, layout.ManagedWorkspaces, layout.GlobalDir, layout.CacheDir, layout.BackupsDir} {
		if err := os.MkdirAll(dir, 0o700); err != nil {
			return fmt.Errorf("create application data directory %s: %w", dir, err)
		}
	}
	return nil
}

func ensureProjectDataLayout(layout projectDataLayout) error {
	for _, dir := range []string{layout.Root, layout.MemoryDir, layout.SkillsDir, layout.MCPDir, layout.ObjectsDir, layout.SessionsDir, layout.WorktreesDir} {
		if err := os.MkdirAll(dir, 0o700); err != nil {
			return fmt.Errorf("create project data directory %s: %w", dir, err)
		}
	}
	return nil
}

func cleanAbsoluteRoot(root string) (string, error) {
	root = strings.TrimSpace(root)
	if root == "" {
		return "", errors.New("path is required")
	}
	abs, err := filepath.Abs(root)
	if err != nil {
		return "", err
	}
	return filepath.Clean(abs), nil
}

func safeDataSegment(label, value string) (string, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return "", fmt.Errorf("%s is required", label)
	}
	if value == "." || value == ".." || filepath.IsAbs(value) || strings.ContainsAny(value, `/\\:`) || strings.ContainsRune(value, 0) {
		return "", fmt.Errorf("invalid %s", label)
	}
	return value, nil
}
