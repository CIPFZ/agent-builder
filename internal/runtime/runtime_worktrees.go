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

	"github.com/charmbracelet/crush/internal/runtimeapi"
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
	if err := r.ensureStarted(ctx); err != nil {
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
	if err := r.ensureStarted(ctx); err != nil {
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
	if _, err := os.Stat(wt.WorktreePath); err != nil {
		wt.Status = worktreeStatusMissing
		wt.Error = "worktree path is missing"
		stored, _ := r.worktrees.Upsert(ctx, wt)
		r.recordWorktreeEvent(ctx, runtimeapi.EventWorktreePolicyDenied, "worktree_policy_denied", firstNonEmptyWorktree(stored, wt), wt.Error)
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
	if err := r.ensureStarted(ctx); err != nil {
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
	if err := r.ensureStarted(ctx); err != nil {
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
	if shouldPreserveWorktree(wt, "") {
		wt.Status = worktreeStatusPreserved
		stored, upsertErr := r.worktrees.Upsert(ctx, wt)
		if upsertErr != nil {
			return RuntimeWorktreeResponse{}, upsertErr
		}
		r.recordWorktreeEvent(ctx, runtimeapi.EventWorktreeExited, "worktree_preserved", stored, "preserve policy retained worktree")
		return RuntimeWorktreeResponse{Worktree: stored}, nil
	}
	if err := removeGitWorktree(ctx, wt); err != nil {
		wt.Status = worktreeStatusCleanupPending
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
	if err := r.ensureStarted(ctx); err != nil {
		return RuntimeEffectiveScopeResponse{}, err
	}
	task, err := r.agentTasks.Get(ctx, strings.TrimSpace(taskID))
	if err != nil {
		return RuntimeEffectiveScopeResponse{}, err
	}
	scope := r.effectiveScopeForTask(ctx, task)
	return RuntimeEffectiveScopeResponse{Scope: scope}, nil
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
	root := runtimeWorktreeRoot(wt.BaseRepoPath)
	if err := pathInsideRuntimeWorktreeRoot(root, wt.WorktreePath); err != nil {
		return err
	}
	if _, err := os.Stat(wt.WorktreePath); os.IsNotExist(err) {
		return nil
	} else if err != nil {
		return fmt.Errorf("failed to inspect worktree path before cleanup: %w", err)
	}
	if _, err := runGit(ctx, wt.WorktreePath, "rev-parse", "--is-inside-work-tree"); err != nil {
		return fmt.Errorf("cleanup refused because path is not a git worktree: %w", err)
	}
	if _, err := runGit(ctx, wt.BaseRepoPath, "worktree", "remove", "--force", wt.WorktreePath); err != nil {
		return fmt.Errorf("failed to remove git worktree: %w", err)
	}
	_, _ = runGit(ctx, wt.BaseRepoPath, "worktree", "prune")
	return nil
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

func (r *runtimeService) applyWorktreeToTask(ctx context.Context, wt RuntimeWorktree) error {
	task, err := r.agentTasks.Get(ctx, wt.TaskID)
	if err != nil {
		return err
	}
	task.Worktree = wt.WorktreePath
	task.CWD = firstNonEmpty(task.CWD, wt.BaseRepoPath)
	_, err = r.agentTasks.Upsert(ctx, task)
	return err
}

func (r *runtimeService) clearWorktreeFromTask(ctx context.Context, wt RuntimeWorktree) error {
	task, err := r.agentTasks.Get(ctx, wt.TaskID)
	if err != nil {
		return err
	}
	if task.Worktree == wt.WorktreePath {
		task.Worktree = ""
	}
	_, err = r.agentTasks.Upsert(ctx, task)
	return err
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
		"summary":         wt.Status + " " + wt.WorktreePath,
	}
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
