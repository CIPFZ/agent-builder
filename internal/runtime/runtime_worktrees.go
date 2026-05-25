package runtime

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/charmbracelet/crush/internal/permission"
	"github.com/charmbracelet/crush/internal/runtimeapi"
	"github.com/charmbracelet/crush/internal/tools/scheduler"
)

var validRuntimeWorktreeSegment = regexp.MustCompile(`^[a-zA-Z0-9._-]+$`)

func (r *runtimeService) Worktrees(ctx context.Context) (RuntimeWorktreesResponse, error) {
	if err := r.ensureStarted(ctx); err != nil {
		return RuntimeWorktreesResponse{}, err
	}
	items, err := r.worktrees.List(ctx)
	if err != nil {
		return RuntimeWorktreesResponse{}, err
	}
	return RuntimeWorktreesResponse{Worktrees: items}, nil
}

func (r *runtimeService) Worktree(ctx context.Context, id string) (RuntimeWorktreeResponse, error) {
	if err := r.ensureStarted(ctx); err != nil {
		return RuntimeWorktreeResponse{}, err
	}
	wt, err := r.worktrees.Get(ctx, strings.TrimSpace(id))
	if err != nil {
		return RuntimeWorktreeResponse{}, err
	}
	return RuntimeWorktreeResponse{Worktree: wt}, nil
}

func (r *runtimeService) CreateWorktree(ctx context.Context, req RuntimeWorktreeCreateRequest) (RuntimeWorktreeResponse, error) {
	if err := r.ensureWorktreeRuntimeReady(ctx); err != nil {
		return RuntimeWorktreeResponse{}, err
	}
	req = normalizeRuntimeWorktreeCreateRequest(req)
	baseRepo, err := r.resolveWorktreeBaseRepo(ctx, req.BaseRepoPath)
	if err != nil {
		r.recordWorktreePolicyDenied(ctx, RuntimeWorktree{SessionID: req.SessionID, TurnID: req.TurnID, TaskID: req.TaskID, BaseRepoPath: req.BaseRepoPath}, err.Error())
		return RuntimeWorktreeResponse{}, err
	}
	slug, err := runtimeWorktreeSlug(req.Name, firstNonEmpty(req.TaskID, req.TurnID, req.SessionID))
	if err != nil {
		r.recordWorktreePolicyDenied(ctx, RuntimeWorktree{SessionID: req.SessionID, TurnID: req.TurnID, TaskID: req.TaskID, BaseRepoPath: baseRepo}, err.Error())
		return RuntimeWorktreeResponse{}, err
	}
	root := runtimeWorktreeRoot(baseRepo)
	path := filepath.Join(root, slug)
	if err := pathInsideRuntimeWorktreeRoot(root, path); err != nil {
		r.recordWorktreePolicyDenied(ctx, RuntimeWorktree{SessionID: req.SessionID, TurnID: req.TurnID, TaskID: req.TaskID, BaseRepoPath: baseRepo, WorktreePath: path}, err.Error())
		return RuntimeWorktreeResponse{}, err
	}
	ref := firstNonEmpty(req.Ref, "HEAD")
	branch := firstNonEmpty(req.Branch, "agent-builder-"+slug)
	id := runtimeWorktreeID(req.SessionID, req.TurnID, req.TaskID, path)
	wt := RuntimeWorktree{
		ID:             id,
		SessionID:      req.SessionID,
		TurnID:         req.TurnID,
		TaskID:         req.TaskID,
		BaseRepoPath:   baseRepo,
		WorktreePath:   path,
		Branch:         branch,
		Ref:            ref,
		Status:         worktreeStatusCreated,
		PreservePolicy: req.PreservePolicy,
		CleanupPolicy:  req.CleanupPolicy,
		Owner:          "runtime",
		Metadata: map[string]string{
			"root": root,
		},
	}
	if result, denied := r.evaluateWorktreePolicy(ctx, "create", wt); denied {
		wt.Status = worktreeStatusError
		wt.Error = result.Reason
		stored, _ := r.worktrees.Upsert(ctx, wt)
		r.recordWorktreePolicyDenied(ctx, firstNonEmptyWorktree(stored, wt), result.Reason)
		return RuntimeWorktreeResponse{}, errors.New(result.Reason)
	}
	if err := createGitWorktree(ctx, wt); err != nil {
		wt.Status = worktreeStatusError
		wt.Error = err.Error()
		stored, _ := r.worktrees.Upsert(ctx, wt)
		r.recordWorktreeEvent(ctx, runtimeapi.EventWorktreePolicyDenied, "worktree_policy_denied", firstNonEmptyWorktree(stored, wt), err.Error())
		return RuntimeWorktreeResponse{}, err
	}
	stored, err := r.worktrees.Upsert(ctx, wt)
	if err != nil {
		return RuntimeWorktreeResponse{}, err
	}
	r.recordWorktreeEvent(ctx, runtimeapi.EventWorktreeCreated, "worktree_created", stored, "")
	return RuntimeWorktreeResponse{Worktree: stored}, nil
}

func (r *runtimeService) EnterWorktree(ctx context.Context, id string, req RuntimeWorktreeActionRequest) (RuntimeWorktreeResponse, error) {
	if err := r.ensureWorktreeRuntimeReady(ctx); err != nil {
		return RuntimeWorktreeResponse{}, err
	}
	wt, err := r.worktrees.Get(ctx, strings.TrimSpace(id))
	if err != nil {
		return RuntimeWorktreeResponse{}, err
	}
	if err := r.validateWorktreeOwnership(wt); err != nil {
		r.recordWorktreePolicyDenied(ctx, wt, err.Error())
		return RuntimeWorktreeResponse{}, err
	}
	if result, denied := r.evaluateWorktreePolicy(ctx, "enter", wt); denied {
		r.recordWorktreePolicyDenied(ctx, wt, result.Reason)
		return RuntimeWorktreeResponse{}, errors.New(result.Reason)
	}
	if err := validateRecoverableWorktree(ctx, wt); err != nil {
		wt.Status = worktreeStatusMissing
		if !errors.Is(err, os.ErrNotExist) {
			wt.Status = worktreeStatusError
		}
		wt.Error = err.Error()
		stored, _ := r.worktrees.Upsert(ctx, wt)
		eventType, auditType := worktreeRecoveryEventForStatus(wt.Status)
		r.recordWorktreeEvent(ctx, eventType, auditType, firstNonEmptyWorktree(stored, wt), wt.Error)
		return RuntimeWorktreeResponse{}, errors.New(wt.Error)
	}
	now := time.Now().UnixMilli()
	wt.Status = worktreeStatusEntered
	wt.EnteredAt = now
	wt.SessionID = firstNonEmpty(req.SessionID, wt.SessionID)
	wt.TurnID = firstNonEmpty(req.TurnID, wt.TurnID)
	wt.TaskID = firstNonEmpty(req.TaskID, wt.TaskID)
	stored, err := r.worktrees.Upsert(ctx, wt)
	if err != nil {
		return RuntimeWorktreeResponse{}, err
	}
	if stored.TaskID != "" {
		_ = r.applyWorktreeToTask(ctx, stored)
	}
	r.recordWorktreeEvent(ctx, runtimeapi.EventWorktreeEntered, "worktree_entered", stored, "")
	return RuntimeWorktreeResponse{Worktree: stored}, nil
}

func (r *runtimeService) ExitWorktree(ctx context.Context, id string, req RuntimeWorktreeActionRequest) (RuntimeWorktreeResponse, error) {
	if err := r.ensureWorktreeRuntimeReady(ctx); err != nil {
		return RuntimeWorktreeResponse{}, err
	}
	wt, err := r.worktrees.Get(ctx, strings.TrimSpace(id))
	if err != nil {
		return RuntimeWorktreeResponse{}, err
	}
	if err := r.validateWorktreeOwnership(wt); err != nil {
		r.recordWorktreePolicyDenied(ctx, wt, err.Error())
		return RuntimeWorktreeResponse{}, err
	}
	if result, denied := r.evaluateWorktreePolicy(ctx, "exit", wt); denied {
		r.recordWorktreePolicyDenied(ctx, wt, result.Reason)
		return RuntimeWorktreeResponse{}, errors.New(result.Reason)
	}
	wt.ExitedAt = time.Now().UnixMilli()
	if req.PreservePolicy != "" {
		wt.PreservePolicy = normalizeWorktreePreservePolicy(req.PreservePolicy)
	}
	if shouldPreserveWorktree(wt, "") {
		wt.Status = worktreeStatusPreserved
	} else {
		wt.Status = worktreeStatusCleanupPending
	}
	stored, err := r.worktrees.Upsert(ctx, wt)
	if err != nil {
		return RuntimeWorktreeResponse{}, err
	}
	if stored.TaskID != "" {
		_ = r.clearWorktreeFromTask(ctx, stored)
	}
	r.recordWorktreeEvent(ctx, runtimeapi.EventWorktreeExited, "worktree_exited", stored, "")
	if stored.Status == worktreeStatusCleanupPending && stored.CleanupPolicy == worktreeCleanupExit {
		return r.CleanupWorktree(ctx, stored.ID, req)
	}
	return RuntimeWorktreeResponse{Worktree: stored}, nil
}

func (r *runtimeService) CleanupWorktree(ctx context.Context, id string, _ RuntimeWorktreeActionRequest) (RuntimeWorktreeResponse, error) {
	if err := r.ensureWorktreeRuntimeReady(ctx); err != nil {
		return RuntimeWorktreeResponse{}, err
	}
	wt, err := r.worktrees.Get(ctx, strings.TrimSpace(id))
	if err != nil {
		return RuntimeWorktreeResponse{}, err
	}
	if err := r.validateWorktreeOwnership(wt); err != nil {
		r.recordWorktreePolicyDenied(ctx, wt, err.Error())
		return RuntimeWorktreeResponse{}, err
	}
	if result, denied := r.evaluateWorktreePolicy(ctx, "cleanup", wt); denied {
		r.recordWorktreePolicyDenied(ctx, wt, result.Reason)
		return RuntimeWorktreeResponse{}, errors.New(result.Reason)
	}
	if shouldPreserveWorktree(wt, "") {
		wt.Status = worktreeStatusPreserved
		stored, upsertErr := r.worktrees.Upsert(ctx, wt)
		if upsertErr != nil {
			return RuntimeWorktreeResponse{}, upsertErr
		}
		r.recordWorktreeEvent(ctx, runtimeapi.EventWorktreePreserved, "worktree_preserved", stored, "preserve policy retained worktree")
		return RuntimeWorktreeResponse{Worktree: stored}, nil
	}
	wt.Status = worktreeStatusCleaning
	wt.Error = ""
	if stored, err := r.worktrees.Upsert(ctx, wt); err == nil {
		wt = stored
	}
	if err := removeGitWorktree(ctx, wt); err != nil {
		if errors.Is(err, errRuntimeWorktreeMissingPath) {
			wt.Status = worktreeStatusMissing
			wt.Error = err.Error()
			stored, _ := r.worktrees.Upsert(ctx, wt)
			r.recordWorktreeEvent(ctx, runtimeapi.EventWorktreeMissingPath, "worktree_missing_path", firstNonEmptyWorktree(stored, wt), wt.Error)
			return RuntimeWorktreeResponse{}, err
		}
		wt.Status = worktreeStatusCleanupFailed
		wt.Error = err.Error()
		stored, _ := r.worktrees.Upsert(ctx, wt)
		r.recordWorktreeEvent(ctx, runtimeapi.EventWorktreeCleanupFailed, "worktree_cleanup_failed", firstNonEmptyWorktree(stored, wt), err.Error())
		return RuntimeWorktreeResponse{}, err
	}
	wt.Status = worktreeStatusCleaned
	wt.CleanedAt = time.Now().UnixMilli()
	wt.Error = ""
	stored, err := r.worktrees.Upsert(ctx, wt)
	if err != nil {
		return RuntimeWorktreeResponse{}, err
	}
	r.recordWorktreeEvent(ctx, runtimeapi.EventWorktreeCleaned, "worktree_cleaned", stored, "")
	return RuntimeWorktreeResponse{Worktree: stored}, nil
}

func (r *runtimeService) TaskEffectiveScope(ctx context.Context, taskID string) (RuntimeEffectiveScopeResponse, error) {
	if err := r.ensureWorktreeRuntimeReady(ctx); err != nil {
		return RuntimeEffectiveScopeResponse{}, err
	}
	task, err := r.agentTasks.Get(ctx, strings.TrimSpace(taskID))
	if err != nil {
		return RuntimeEffectiveScopeResponse{}, err
	}
	scope := r.effectiveScopeForTask(ctx, task)
	return RuntimeEffectiveScopeResponse{Scope: scope}, nil
}

func (r *runtimeService) ensureWorktreeRuntimeReady(ctx context.Context) error {
	if r.worktrees.db != nil {
		return nil
	}
	return r.ensureStarted(ctx)
}

func (r *runtimeService) effectiveScopeForTask(ctx context.Context, task RuntimeAgentTask) RuntimeEffectiveScope {
	scope := RuntimeEffectiveScope{
		SessionID:    task.ChildSessionID,
		TurnID:       task.ParentTurnID,
		TaskID:       task.ID,
		BaseCWD:      task.CWD,
		EffectiveCWD: firstNonEmpty(task.Worktree, task.CWD),
		WorktreePath: task.Worktree,
	}
	if task.Worktree != "" && r.worktrees.db != nil {
		if items, err := r.worktrees.ListByTask(ctx, task.ID); err == nil {
			for i := len(items) - 1; i >= 0; i-- {
				if items[i].WorktreePath == task.Worktree || items[i].Status == worktreeStatusEntered {
					scope.WorktreeID = items[i].ID
					wt := items[i]
					scope.Worktree = &wt
					break
				}
			}
		}
	}
	return scope
}

func normalizeRuntimeWorktreeCreateRequest(req RuntimeWorktreeCreateRequest) RuntimeWorktreeCreateRequest {
	req.SessionID = strings.TrimSpace(req.SessionID)
	req.TurnID = strings.TrimSpace(req.TurnID)
	req.TaskID = strings.TrimSpace(req.TaskID)
	req.BaseRepoPath = strings.TrimSpace(req.BaseRepoPath)
	req.Branch = strings.TrimSpace(req.Branch)
	req.Ref = strings.TrimSpace(req.Ref)
	req.Name = strings.TrimSpace(req.Name)
	req.PreservePolicy = normalizeWorktreePreservePolicy(req.PreservePolicy)
	req.CleanupPolicy = normalizeWorktreeCleanupPolicy(req.CleanupPolicy)
	return req
}

func normalizeWorktreePreservePolicy(value string) string {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case worktreePreserveNever:
		return worktreePreserveNever
	case worktreePreserveOnExit:
		return worktreePreserveOnExit
	case worktreePreserveAlways:
		return worktreePreserveAlways
	default:
		return worktreePreserveOnFailure
	}
}

func normalizeWorktreeCleanupPolicy(value string) string {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case worktreeCleanupExit:
		return worktreeCleanupExit
	default:
		return worktreeCleanupManual
	}
}

func (r *runtimeService) resolveWorktreeBaseRepo(ctx context.Context, requested string) (string, error) {
	base := strings.TrimSpace(requested)
	if base == "" {
		r.mu.Lock()
		if r.workspace != nil {
			base = r.workspace.Path
		}
		r.mu.Unlock()
	}
	if base == "" {
		return "", errors.New("base repo path is required")
	}
	baseAbs, err := filepath.Abs(base)
	if err != nil {
		return "", fmt.Errorf("failed to resolve base repo path: %w", err)
	}
	baseAbs = filepath.Clean(baseAbs)
	if err := requireGitRepo(ctx, baseAbs); err != nil {
		return "", err
	}
	return baseAbs, nil
}

func runtimeWorktreeSlug(name, seed string) (string, error) {
	slug := strings.TrimSpace(name)
	if slug == "" {
		sum := sha256.Sum256([]byte(firstNonEmpty(seed, fmt.Sprint(time.Now().UnixNano()))))
		slug = "wt-" + hex.EncodeToString(sum[:4])
	}
	slug = strings.ReplaceAll(slug, "\\", "/")
	parts := strings.Split(slug, "/")
	if len(parts) == 0 || len(slug) > 64 {
		return "", fmt.Errorf("invalid worktree name %q", slug)
	}
	for _, part := range parts {
		if part == "" || part == "." || part == ".." || !validRuntimeWorktreeSegment.MatchString(part) {
			return "", fmt.Errorf("invalid worktree name %q", slug)
		}
	}
	return strings.ReplaceAll(slug, "/", "+"), nil
}

func runtimeWorktreeID(sessionID, turnID, taskID, path string) string {
	sum := sha256.Sum256([]byte(sessionID + "\x00" + turnID + "\x00" + taskID + "\x00" + filepath.Clean(path)))
	return "wt_" + hex.EncodeToString(sum[:8])
}

func runtimeWorktreeRoot(baseRepo string) string {
	return filepath.Join(baseRepo, ".agent-builder", "worktrees")
}

func pathInsideRuntimeWorktreeRoot(root, target string) error {
	rootAbs, err := filepath.Abs(root)
	if err != nil {
		return fmt.Errorf("failed to resolve worktree root: %w", err)
	}
	targetAbs, err := filepath.Abs(target)
	if err != nil {
		return fmt.Errorf("failed to resolve worktree path: %w", err)
	}
	rootClean := strings.ToLower(filepath.Clean(rootAbs))
	targetClean := strings.ToLower(filepath.Clean(targetAbs))
	if targetClean == rootClean || strings.HasPrefix(targetClean, rootClean+string(filepath.Separator)) {
		return nil
	}
	return fmt.Errorf("worktree path %s is outside runtime-owned root %s", targetAbs, rootAbs)
}

func requireGitRepo(ctx context.Context, path string) error {
	result, err := runGit(ctx, path, "rev-parse", "--show-toplevel")
	if err != nil {
		return fmt.Errorf("base repo path is not a git repository: %w", err)
	}
	if strings.TrimSpace(result) == "" {
		return errors.New("base repo path is not a git repository")
	}
	return nil
}

func createGitWorktree(ctx context.Context, wt RuntimeWorktree) error {
	if err := os.MkdirAll(filepath.Dir(wt.WorktreePath), 0o755); err != nil {
		return fmt.Errorf("failed to create worktree root: %w", err)
	}
	if _, err := os.Stat(wt.WorktreePath); err == nil {
		return fmt.Errorf("worktree path already exists and is not a git worktree: %s", wt.WorktreePath)
	} else if !os.IsNotExist(err) {
		return fmt.Errorf("failed to inspect worktree path: %w", err)
	}
	_, err := runGit(ctx, wt.BaseRepoPath, "worktree", "add", "-B", wt.Branch, wt.WorktreePath, firstNonEmpty(wt.Ref, "HEAD"))
	if err != nil {
		return fmt.Errorf("failed to create git worktree: %w", err)
	}
	return nil
}

func removeGitWorktree(ctx context.Context, wt RuntimeWorktree) error {
	if err := validateRuntimeOwnedWorktreeForCleanup(ctx, wt); err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil
		}
		return err
	}
	if _, err := runGit(ctx, wt.BaseRepoPath, "worktree", "remove", "--force", wt.WorktreePath); err != nil {
		return fmt.Errorf("failed to remove git worktree: %w", err)
	}
	_, _ = runGit(ctx, wt.BaseRepoPath, "worktree", "prune")
	return nil
}

func validateRuntimeOwnedWorktreeForCleanup(ctx context.Context, wt RuntimeWorktree) error {
	root := runtimeWorktreeRoot(wt.BaseRepoPath)
	if err := pathInsideRuntimeWorktreeRoot(root, wt.WorktreePath); err != nil {
		return err
	}
	if wt.Owner != "" && wt.Owner != "runtime" {
		return fmt.Errorf("worktree %s is not runtime-owned", wt.ID)
	}
	if err := requireGitRepo(ctx, wt.BaseRepoPath); err != nil {
		return fmt.Errorf("cleanup refused because base repo is unavailable: %w", err)
	}
	if _, err := os.Stat(wt.WorktreePath); os.IsNotExist(err) {
		return errRuntimeWorktreeMissingPath
	} else if err != nil {
		return fmt.Errorf("failed to inspect worktree path before cleanup: %w", err)
	}
	if _, err := runGit(ctx, wt.WorktreePath, "rev-parse", "--is-inside-work-tree"); err != nil {
		return fmt.Errorf("cleanup refused because path is not a git worktree: %w", err)
	}
	return nil
}

func validateRecoverableWorktree(ctx context.Context, wt RuntimeWorktree) error {
	if err := validateRuntimeOwnedWorktreeForCleanup(ctx, wt); err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return os.ErrNotExist
		}
		return err
	}
	return nil
}

func (r *runtimeService) markWorktreeCleanupFailure(ctx context.Context, wt RuntimeWorktree, err error) (RuntimeWorktreeResponse, error) {
	if err == nil {
		return RuntimeWorktreeResponse{}, nil
	}
	if errors.Is(err, errRuntimeWorktreeMissingPath) {
		wt.Status = worktreeStatusMissing
	} else {
		wt.Status = worktreeStatusCleanupFailed
	}
	wt.Error = err.Error()
	stored, _ := r.worktrees.Upsert(ctx, wt)
	eventType, auditType := worktreeRecoveryEventForStatus(stored.Status)
	if stored.Status == worktreeStatusCleanupFailed {
		eventType = runtimeapi.EventWorktreeCleanupFailed
		auditType = "worktree_cleanup_failed"
	}
	if stored.Status == worktreeStatusMissing {
		eventType = runtimeapi.EventWorktreeMissingPath
		auditType = "worktree_missing_path"
	}
	r.recordWorktreeEvent(ctx, eventType, auditType, firstNonEmptyWorktree(stored, wt), wt.Error)
	return RuntimeWorktreeResponse{}, err
}

func runGit(ctx context.Context, dir string, args ...string) (string, error) {
	cmd := exec.CommandContext(ctx, "git", args...)
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), "GIT_TERMINAL_PROMPT=0", "GIT_ASKPASS=")
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	cmd.Stdin = nil
	if err := cmd.Run(); err != nil {
		msg := strings.TrimSpace(stderr.String())
		if msg == "" {
			msg = err.Error()
		}
		return stdout.String(), errors.New(msg)
	}
	return stdout.String(), nil
}

func (r *runtimeService) validateWorktreeOwnership(wt RuntimeWorktree) error {
	if wt.Owner != "" && wt.Owner != "runtime" {
		return fmt.Errorf("worktree %s is not runtime-owned", wt.ID)
	}
	root := runtimeWorktreeRoot(wt.BaseRepoPath)
	return pathInsideRuntimeWorktreeRoot(root, wt.WorktreePath)
}

func (r *runtimeService) evaluateWorktreePolicy(_ context.Context, action string, wt RuntimeWorktree) (permission.PolicyResult, bool) {
	r.mu.Lock()
	policyConfig := r.policy
	r.mu.Unlock()
	policy := runtimePermissionPolicy(policyConfig)
	result := policy.Evaluate(scheduler.ToolCall{
		ID:           "worktree:" + wt.ID + ":" + action,
		SessionID:    wt.SessionID,
		TurnID:       wt.TurnID,
		Name:         "worktree_" + action,
		Source:       scheduler.ToolSourceBuiltin,
		CapabilityID: "builtin:worktree_" + action,
		InputSummary: worktreePolicyInput(action, wt),
	})
	result.TargetSummary = firstNonEmpty(result.TargetSummary, wt.WorktreePath)
	if result.Reason == "" {
		result.Reason = "Runtime worktree " + action + " policy decision."
	}
	r.recordWorktreePolicyDecision(action, wt, result)
	if result.Decision == permission.PolicyDeny {
		return result, true
	}
	if result.Decision == permission.PolicyAsk && result.Headless {
		result.Decision = permission.PolicyDeny
		if result.HeadlessReason == "" {
			result.HeadlessReason = "Worktree action requires approval in a non-interactive runtime policy profile."
		}
		result.Reason = strings.TrimSpace(result.Reason + " " + result.HeadlessReason)
		r.recordWorktreePolicyDecision(action, wt, result)
		return result, true
	}
	return result, false
}

func worktreePolicyInput(action string, wt RuntimeWorktree) string {
	return fmt.Sprintf(`{"action":%q,"path":%q,"cwd":%q,"working_dir":%q,"base_repo_path":%q,"task_id":%q}`,
		action,
		wt.WorktreePath,
		wt.WorktreePath,
		wt.WorktreePath,
		wt.BaseRepoPath,
		wt.TaskID,
	)
}

func (r *runtimeService) recordWorktreePolicyDecision(action string, wt RuntimeWorktree, result permission.PolicyResult) {
	call := RuntimeEvent{
		ID:        newRuntimeEventID(),
		Type:      runtimeapi.EventPermissionPolicyApplied,
		CreatedAt: time.Now().UTC().Format(time.RFC3339Nano),
		SessionID: wt.SessionID,
		TurnID:    wt.TurnID,
		Payload: map[string]any{
			"tool_name":           "worktree_" + action,
			"capability_id":       "builtin:worktree_" + action,
			"decision":            result.Decision,
			"risk":                result.Risk,
			"reason":              result.Reason,
			"mode":                result.Mode,
			"profile":             result.Profile,
			"headless":            result.Headless,
			"headless_reason":     result.HeadlessReason,
			"matched_rule_id":     result.RuleID,
			"matched_rule_source": result.RuleSource,
			"scope_kind":          result.RuleScopeKind,
			"scope_value":         result.RuleScopeValue,
			"target_summary":      result.TargetSummary,
			"worktree_id":         wt.ID,
			"summary":             "worktree " + action + " policy " + string(result.Decision),
		},
	}
	r.storeRuntimeEvent(call)
	if r.canWriteWorktreeAudit() {
		r.writeAudit(auditEntry{
			RequestID:            wt.TurnID,
			Event:                "permission_policy_applied",
			Timestamp:            time.Now().UTC().Format(time.RFC3339Nano),
			SessionID:            wt.SessionID,
			PermissionTool:       "worktree_" + action,
			PermissionAction:     action,
			PermissionPath:       wt.WorktreePath,
			PermissionPolicy:     string(result.Decision),
			PermissionRisk:       string(result.Risk),
			PermissionReason:     result.Reason,
			PolicyMode:           string(result.Mode),
			PolicyProfile:        result.Profile,
			PolicyHeadless:       result.Headless,
			PolicyHeadlessReason: result.HeadlessReason,
			PolicyRuleID:         result.RuleID,
			PolicyRuleSource:     result.RuleSource,
			PolicyScopeKind:      result.RuleScopeKind,
			PolicyScopeValue:     result.RuleScopeValue,
			PolicyTargetSummary:  result.TargetSummary,
			CapabilityID:         "builtin:worktree_" + action,
			AgentTask:            worktreeAuditTask(wt),
			Extra: map[string]any{
				"worktree":        wt,
				"headless":        result.Headless,
				"headless_reason": result.HeadlessReason,
			},
		})
	}
}

func (r *runtimeService) canWriteWorktreeAudit() bool {
	if r == nil || r.runtime == nil || r.workspace == nil {
		return false
	}
	_, err := r.runtime.GetWorkspace(r.workspace.ID)
	return err == nil
}

func shouldPreserveWorktree(wt RuntimeWorktree, failure string) bool {
	switch wt.PreservePolicy {
	case worktreePreserveAlways, worktreePreserveOnExit:
		return true
	case worktreePreserveOnFailure:
		return failure != "" || wt.Error != ""
	default:
		return false
	}
}

var errRuntimeWorktreeMissingPath = errors.New("worktree path is missing")

func (r *runtimeService) applyWorktreeToTask(ctx context.Context, wt RuntimeWorktree) error {
	task, err := r.agentTasks.Get(ctx, wt.TaskID)
	if err != nil {
		return err
	}
	task.CWD = firstNonEmpty(task.CWD, wt.BaseRepoPath)
	if _, err := r.agentTasks.Upsert(ctx, task); err != nil {
		return err
	}
	_, err = r.agentTasks.SetWorktree(ctx, wt.TaskID, wt.WorktreePath)
	return err
}

func (r *runtimeService) clearWorktreeFromTask(ctx context.Context, wt RuntimeWorktree) error {
	task, err := r.agentTasks.Get(ctx, wt.TaskID)
	if err != nil {
		return err
	}
	if task.Worktree == wt.WorktreePath {
		_, err = r.agentTasks.SetWorktree(ctx, wt.TaskID, "")
		return err
	}
	return nil
}

func (r *runtimeService) recordWorktreePolicyDenied(ctx context.Context, wt RuntimeWorktree, reason string) {
	r.recordWorktreeEvent(ctx, runtimeapi.EventWorktreePolicyDenied, "worktree_policy_denied", wt, reason)
}

func (r *runtimeService) recordWorktreeEvent(_ context.Context, eventType, auditType string, wt RuntimeWorktree, errText string) {
	payload := runtimeWorktreeEventPayload(wt)
	if errText != "" {
		payload["error"] = errText
	}
	r.storeRuntimeEvent(runtimeapi.Event{
		ID:        newRuntimeEventID(),
		Type:      eventType,
		CreatedAt: time.Now().UTC().Format(time.RFC3339Nano),
		SessionID: wt.SessionID,
		TurnID:    wt.TurnID,
		Payload:   payload,
	})
	r.writeAudit(auditEntry{
		RequestID:        wt.TurnID,
		Event:            auditType,
		Timestamp:        time.Now().UTC().Format(time.RFC3339Nano),
		SessionID:        wt.SessionID,
		AgentTask:        worktreeAuditTask(wt),
		Error:            errText,
		Extra:            map[string]any{"worktree": wt},
		PermissionReason: firstNonEmpty(errText, "runtime worktree lifecycle"),
	})
}

func runtimeWorktreeEventPayload(wt RuntimeWorktree) map[string]any {
	pathSummary := pathSafeSummary(wt.WorktreePath)
	return map[string]any{
		"id":              wt.ID,
		"session_id":      wt.SessionID,
		"turn_id":         wt.TurnID,
		"task_id":         wt.TaskID,
		"base_repo_path":  wt.BaseRepoPath,
		"worktree_path":   wt.WorktreePath,
		"branch":          wt.Branch,
		"ref":             wt.Ref,
		"status":          wt.Status,
		"preserve_policy": wt.PreservePolicy,
		"cleanup_policy":  wt.CleanupPolicy,
		"created_at":      wt.CreatedAt,
		"entered_at":      wt.EnteredAt,
		"exited_at":       wt.ExitedAt,
		"cleaned_at":      wt.CleanedAt,
		"updated_at":      wt.UpdatedAt,
		"owner":           wt.Owner,
		"path_summary":    pathSummary,
		"summary":         wt.Status + " " + pathSummary,
	}
}

func pathSafeSummary(path string) string {
	if strings.TrimSpace(path) == "" {
		return ""
	}
	return filepath.Clean(path)
}

func worktreeAuditTask(wt RuntimeWorktree) *RuntimeAgentTask {
	if wt.TaskID == "" {
		return nil
	}
	return &RuntimeAgentTask{
		ID:              wt.TaskID,
		ParentTurnID:    wt.TurnID,
		ParentSessionID: wt.SessionID,
		CWD:             wt.BaseRepoPath,
		Worktree:        wt.WorktreePath,
	}
}

func firstNonEmptyWorktree(values ...RuntimeWorktree) RuntimeWorktree {
	for _, value := range values {
		if value.ID != "" {
			return value
		}
	}
	if len(values) > 0 {
		return values[0]
	}
	return RuntimeWorktree{}
}
