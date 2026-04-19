package runtime

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

type AgentWorktree struct {
	Path       string
	Branch     string
	HeadCommit string
	GitRoot    string
}

type WorktreeManager interface {
	Create(context.Context, string, string) (AgentWorktree, error)
	HasChanges(context.Context, string, string) (bool, error)
	Remove(context.Context, AgentWorktree) error
}

type gitWorktreeManager struct{}

func newGitWorktreeManager() WorktreeManager {
	return gitWorktreeManager{}
}

func (gitWorktreeManager) Create(ctx context.Context, baseDir, slug string) (AgentWorktree, error) {
	baseDir = filepath.Clean(strings.TrimSpace(baseDir))
	if baseDir == "" {
		return AgentWorktree{}, fmt.Errorf("worktree base directory is required")
	}
	headCommit, err := gitOutput(ctx, baseDir, "rev-parse", "HEAD")
	if err != nil {
		return AgentWorktree{}, err
	}
	worktreeDir := filepath.Join(baseDir, ".claude", "worktrees")
	if err := os.MkdirAll(worktreeDir, 0o755); err != nil {
		return AgentWorktree{}, err
	}
	flattened := strings.NewReplacer("/", "+", "\\", "+", ":", "_").Replace(strings.TrimSpace(slug))
	if flattened == "" {
		flattened = "agent-worktree"
	}
	branch := "worktree-" + flattened
	path := filepath.Join(worktreeDir, flattened)
	if _, err := os.Stat(path); err == nil {
		return AgentWorktree{Path: path, Branch: branch, HeadCommit: headCommit, GitRoot: baseDir}, nil
	}
	if _, err := gitCombinedOutput(ctx, baseDir, "worktree", "add", "-B", branch, path, "HEAD"); err != nil {
		return AgentWorktree{}, err
	}
	return AgentWorktree{Path: path, Branch: branch, HeadCommit: headCommit, GitRoot: baseDir}, nil
}

func (gitWorktreeManager) HasChanges(ctx context.Context, worktreePath, headCommit string) (bool, error) {
	status, err := gitOutput(ctx, worktreePath, "status", "--porcelain")
	if err != nil {
		return false, err
	}
	if strings.TrimSpace(status) != "" {
		return true, nil
	}
	revList, err := gitOutput(ctx, worktreePath, "rev-list", "--count", headCommit+"..HEAD")
	if err != nil {
		return false, err
	}
	return strings.TrimSpace(revList) != "0", nil
}

func (gitWorktreeManager) Remove(ctx context.Context, worktree AgentWorktree) error {
	if strings.TrimSpace(worktree.GitRoot) == "" || strings.TrimSpace(worktree.Path) == "" {
		return fmt.Errorf("worktree path and git root are required for cleanup")
	}
	if _, err := gitCombinedOutput(ctx, worktree.GitRoot, "worktree", "remove", "--force", worktree.Path); err != nil {
		return err
	}
	if strings.TrimSpace(worktree.Branch) != "" {
		if _, err := gitCombinedOutput(ctx, worktree.GitRoot, "branch", "-D", worktree.Branch); err != nil {
			return err
		}
	}
	return nil
}

func gitOutput(ctx context.Context, dir string, args ...string) (string, error) {
	out, err := gitCombinedOutput(ctx, dir, args...)
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(out)), nil
}

func gitCombinedOutput(ctx context.Context, dir string, args ...string) ([]byte, error) {
	cmd := exec.CommandContext(ctx, "git", args...)
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	if err != nil {
		return nil, fmt.Errorf("git %s in %s failed: %w: %s", strings.Join(args, " "), dir, err, strings.TrimSpace(string(out)))
	}
	return out, nil
}
