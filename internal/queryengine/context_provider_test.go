package queryengine

import (
	"context"
	"strings"
	"testing"
	"time"

	"myclaw/internal/permissions"
	"myclaw/internal/session"
	"myclaw/internal/workspace"
)

func TestGitStatusSnapshotTimeoutDoesNotBlockSystemContext(t *testing.T) {
	restore := overrideGitCommandRunnerForTest(t, 20*time.Millisecond, func(ctx context.Context, _ string, _ ...string) (string, bool) {
		<-ctx.Done()
		return "", false
	})
	defer restore()

	start := time.Now()
	got, ok := gitStatusSnapshot("C:/repo")
	if ok {
		t.Fatalf("gitStatusSnapshot ok = true, want false with hanging git command; got %q", got)
	}
	if elapsed := time.Since(start); elapsed > 500*time.Millisecond {
		t.Fatalf("gitStatusSnapshot elapsed = %s, want bounded timeout", elapsed)
	}
}

func TestSystemContextProviderSkipsGitStatusWhenGitCommandsHang(t *testing.T) {
	restore := overrideGitCommandRunnerForTest(t, 20*time.Millisecond, func(ctx context.Context, _ string, _ ...string) (string, bool) {
		<-ctx.Done()
		return "", false
	})
	defer restore()

	provider := defaultSystemContextProvider("", false)
	lines := provider.Lines(
		session.Session{ID: "sess-1"},
		workspace.Context{Root: "C:/repo"},
		permissions.Policy{Mode: permissions.ModeWorkspaceWrite},
	)

	if containsLineWithPrefix(lines, "git_status=") {
		t.Fatalf("system context lines = %#v, did not want git_status when git commands hang", lines)
	}
	if !containsLine(lines, "permission_mode=workspace-write") {
		t.Fatalf("system context lines = %#v, want non-git context preserved", lines)
	}
	if !containsLine(lines, "workspace_root=C:/repo") {
		t.Fatalf("system context lines = %#v, want workspace root preserved", lines)
	}
}

func TestGitStatusSnapshotStillFormatsSuccessfulOutput(t *testing.T) {
	restore := overrideGitCommandRunnerForTest(t, time.Second, func(_ context.Context, _ string, args ...string) (string, bool) {
		command := strings.Join(args, " ")
		switch command {
		case "rev-parse --is-inside-work-tree":
			return "true", true
		case "branch --show-current":
			return "feature/git-timeout", true
		case "symbolic-ref --short refs/remotes/origin/HEAD":
			return "origin/main", true
		case "--no-optional-locks status --short":
			return " M internal/queryengine/context_provider.go", true
		case "--no-optional-locks log --oneline -n 5":
			return "abc1234 add git status timeout", true
		case "config user.name":
			return "Test User", true
		default:
			t.Fatalf("unexpected git command: %s", command)
			return "", false
		}
	})
	defer restore()

	got, ok := gitStatusSnapshot("C:/repo")
	if !ok {
		t.Fatal("gitStatusSnapshot ok = false, want successful snapshot")
	}
	for _, want := range []string{
		"This is the git status at the start of the conversation.",
		"Current branch: feature/git-timeout",
		"Main branch (you will usually use this for PRs): main",
		"Git user: Test User",
		"Status:\n M internal/queryengine/context_provider.go",
		"Recent commits:\nabc1234 add git status timeout",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("git status snapshot = %q, want substring %q", got, want)
		}
	}
}

func TestGitStatusSnapshotSkipsWhenGitCommandFails(t *testing.T) {
	restore := overrideGitCommandRunnerForTest(t, time.Second, func(_ context.Context, _ string, args ...string) (string, bool) {
		if strings.Join(args, " ") == "rev-parse --is-inside-work-tree" {
			return "true", true
		}
		return "", false
	})
	defer restore()

	got, ok := gitStatusSnapshot("C:/repo")
	if ok {
		t.Fatalf("gitStatusSnapshot ok = true, want false after git command failure; got %q", got)
	}
}

func TestGitStatusSnapshotStillTruncatesStatus(t *testing.T) {
	longStatus := strings.Repeat("M file.txt\n", 260)
	restore := overrideGitCommandRunnerForTest(t, time.Second, func(_ context.Context, _ string, args ...string) (string, bool) {
		command := strings.Join(args, " ")
		switch command {
		case "rev-parse --is-inside-work-tree":
			return "true", true
		case "branch --show-current":
			return "main", true
		case "symbolic-ref --short refs/remotes/origin/HEAD":
			return "origin/main", true
		case "--no-optional-locks status --short":
			return longStatus, true
		case "--no-optional-locks log --oneline -n 5":
			return "abc1234 initial commit", true
		case "config user.name":
			return "", false
		default:
			return "", false
		}
	})
	defer restore()

	got, ok := gitStatusSnapshot("C:/repo")
	if !ok {
		t.Fatal("gitStatusSnapshot ok = false, want successful snapshot")
	}
	if !strings.Contains(got, "... (truncated because it exceeds 2k characters. If you need more information, run \"git status\" using BashTool)") {
		t.Fatalf("git status snapshot = %q, want truncation marker", got)
	}
}

func overrideGitCommandRunnerForTest(t *testing.T, timeout time.Duration, runner gitCommandRunner) func() {
	t.Helper()

	previousRunner := gitCommandRunnerFunc
	previousTimeout := gitStatusSnapshotTimeout
	gitCommandRunnerFunc = runner
	gitStatusSnapshotTimeout = timeout

	return func() {
		gitCommandRunnerFunc = previousRunner
		gitStatusSnapshotTimeout = previousTimeout
	}
}

func containsLine(values []string, want string) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}

func containsLineWithPrefix(values []string, prefix string) bool {
	for _, value := range values {
		if strings.HasPrefix(value, prefix) {
			return true
		}
	}
	return false
}
