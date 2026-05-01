package queryengine

import (
	"context"
	"os/exec"
	"strings"
	"time"

	"myclaw/internal/permissions"
	"myclaw/internal/session"
	"myclaw/internal/workspace"
)

const maxGitStatusChars = 2000
const defaultGitStatusSnapshotTimeout = 2 * time.Second

type gitCommandRunner func(context.Context, string, ...string) (string, bool)

var (
	gitStatusSnapshotTimeout                  = defaultGitStatusSnapshotTimeout
	gitCommandRunnerFunc     gitCommandRunner = runGitCommandWithContext
)

type UserContextProvider interface {
	Lines(session.Session, workspace.Context) []string
}

type UserContextProviderFunc func(session.Session, workspace.Context) []string

func (f UserContextProviderFunc) Lines(sess session.Session, workspaceContext workspace.Context) []string {
	return f(sess, workspaceContext)
}

type SystemContextProvider interface {
	Lines(session.Session, workspace.Context, permissions.Policy) []string
}

type SystemContextProviderFunc func(session.Session, workspace.Context, permissions.Policy) []string

func (f SystemContextProviderFunc) Lines(sess session.Session, workspaceContext workspace.Context, policy permissions.Policy) []string {
	return f(sess, workspaceContext, policy)
}

func defaultUserContextProvider(disableClaudeMd bool) UserContextProvider {
	return UserContextProviderFunc(func(sess session.Session, workspaceContext workspace.Context) []string {
		lines := []string{
			"session_id=" + sess.ID,
			"session_key=" + sess.Key,
			"agent_id=" + sess.AgentID,
			"current_date=" + time.Now().Format("2006-01-02"),
		}
		if !disableClaudeMd {
			if claudeMD, ok := workspaceInstructionContent(workspaceContext); ok {
				lines = append(lines, "claude_md="+claudeMD)
			}
		}
		return lines
	})
}

func defaultSystemContextProvider(systemPromptInjection string, disableGitStatus bool) SystemContextProvider {
	return SystemContextProviderFunc(func(_ session.Session, workspaceContext workspace.Context, policy permissions.Policy) []string {
		lines := []string{
			"permission_mode=" + string(policy.Mode),
		}
		if !disableGitStatus {
			if gitStatus, ok := gitStatusSnapshot(workspaceContext.Root); ok {
				lines = append(lines, "git_status="+gitStatus)
			}
		}
		if injection := strings.TrimSpace(systemPromptInjection); injection != "" {
			lines = append(lines, "cache_breaker=[CACHE_BREAKER: "+injection+"]")
		}
		if policy.PlanMode {
			lines = append(lines, "plan_mode=true")
		}
		if policy.AutoMode {
			lines = append(lines, "auto_mode=true")
		}
		if workspaceContext.Root != "" {
			lines = append(lines, "workspace_root="+workspaceContext.Root)
		}
		if len(policy.WorkspaceRoots) > 0 {
			lines = append(lines, "workspace_roots="+strings.Join(policy.WorkspaceRoots, ","))
		}
		return lines
	})
}

func gitStatusSnapshot(root string) (string, bool) {
	root = strings.TrimSpace(root)
	if root == "" {
		return "", false
	}
	ctx, cancel := context.WithTimeout(context.Background(), gitStatusSnapshotTimeout)
	defer cancel()

	if !gitCommandSucceeded(ctx, root, "rev-parse", "--is-inside-work-tree") {
		return "", false
	}

	branch, ok := runGitCommand(ctx, root, "branch", "--show-current")
	if !ok {
		return "", false
	}
	mainBranch, ok := defaultGitMainBranch(ctx, root, branch)
	if !ok {
		return "", false
	}
	status, ok := runGitCommand(ctx, root, "--no-optional-locks", "status", "--short")
	if !ok {
		return "", false
	}
	log, ok := runGitCommand(ctx, root, "--no-optional-locks", "log", "--oneline", "-n", "5")
	if !ok {
		return "", false
	}
	userName, _ := runGitCommand(ctx, root, "config", "user.name")

	parts := []string{
		"This is the git status at the start of the conversation. Note that this status is a snapshot in time, and will not update during the conversation.",
		"Current branch: " + branch,
		"Main branch (you will usually use this for PRs): " + mainBranch,
	}
	if strings.TrimSpace(userName) != "" {
		parts = append(parts, "Git user: "+userName)
	}
	status = truncateGitStatus(status)
	parts = append(parts,
		"Status:\n"+emptyFallback(status, "(clean)"),
		"Recent commits:\n"+log,
	)
	return strings.Join(parts, "\n\n"), true
}

func defaultGitMainBranch(ctx context.Context, root, currentBranch string) (string, bool) {
	if branch, ok := runGitCommand(ctx, root, "symbolic-ref", "--short", "refs/remotes/origin/HEAD"); ok {
		if idx := strings.LastIndex(branch, "/"); idx >= 0 && idx+1 < len(branch) {
			return strings.TrimSpace(branch[idx+1:]), true
		}
		if strings.TrimSpace(branch) != "" {
			return strings.TrimSpace(branch), true
		}
	}
	if strings.TrimSpace(currentBranch) != "" {
		return strings.TrimSpace(currentBranch), true
	}
	return "", false
}

func gitCommandSucceeded(ctx context.Context, root string, args ...string) bool {
	output, ok := runGitCommand(ctx, root, args...)
	if !ok {
		return false
	}
	return strings.TrimSpace(output) == "true"
}

func runGitCommand(ctx context.Context, root string, args ...string) (string, bool) {
	return gitCommandRunnerFunc(ctx, root, args...)
}

func runGitCommandWithContext(ctx context.Context, root string, args ...string) (string, bool) {
	cmd := exec.CommandContext(ctx, "git", args...)
	cmd.Dir = root
	output, err := cmd.Output()
	if err != nil {
		return "", false
	}
	return strings.TrimSpace(string(output)), true
}

func emptyFallback(value, fallback string) string {
	if strings.TrimSpace(value) == "" {
		return fallback
	}
	return value
}

func truncateGitStatus(status string) string {
	if len(status) <= maxGitStatusChars {
		return status
	}
	return status[:maxGitStatusChars] + "\n... (truncated because it exceeds 2k characters. If you need more information, run \"git status\" using BashTool)"
}

func workspaceFileContent(workspaceContext workspace.Context, name string) (string, bool) {
	for _, file := range workspaceContext.Files {
		if file.Name != name {
			continue
		}
		content := strings.TrimSpace(file.Content)
		if content == "" {
			return "", false
		}
		return content, true
	}
	return "", false
}

func workspaceInstructionContent(workspaceContext workspace.Context) (string, bool) {
	items := make([]string, 0, len(workspaceContext.Files))
	for _, file := range workspaceContext.Files {
		if file.Type != "instruction" {
			continue
		}
		content := strings.TrimSpace(file.Content)
		if content == "" {
			continue
		}
		items = append(items, content)
	}
	if len(items) == 0 {
		return "", false
	}
	return strings.Join(items, "\n\n"), true
}
