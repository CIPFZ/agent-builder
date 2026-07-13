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

	"github.com/CIPFZ/agent-builder/internal/apitypes"
	"github.com/CIPFZ/agent-builder/internal/config"
	"github.com/CIPFZ/agent-builder/internal/contextmgr"
	"github.com/CIPFZ/agent-builder/internal/db"
	agentbuilderlog "github.com/CIPFZ/agent-builder/internal/log"
	"github.com/CIPFZ/agent-builder/internal/runtimeapi"
	"github.com/CIPFZ/agent-builder/internal/tools/scheduler"
	"github.com/CIPFZ/agent-builder/internal/version"
	"github.com/CIPFZ/agent-builder/internal/workbench"
)

func (r *runtimeService) workspaceConfig(ctx context.Context) (*config.ConfigStore, string, error) {
	if err := r.ensureWorkspaceStarted(ctx, false); err != nil {
		return nil, "", err
	}
	r.mu.Lock()
	wsID := r.workspace.ID
	r.mu.Unlock()
	ws, err := r.runtime.GetWorkspace(wsID)
	if err != nil {
		return nil, "", err
	}
	return ws.Cfg, wsID, nil
}

func (r *runtimeService) workspaceDB(ctx context.Context) (*sql.DB, error) {
	cfg, _, err := r.workspaceConfig(ctx)
	if err != nil {
		return nil, err
	}
	conn, err := db.Connect(ctx, cfg.Config().Options.DataDirectory)
	if err != nil {
		return nil, err
	}
	return conn, nil
}

func openDesktopDB(ctx context.Context) (*sql.DB, string, error) {
	layout, err := resolveDesktopLayout()
	if err != nil {
		return nil, "", err
	}
	if err := ensureDesktopLayout(layout); err != nil {
		return nil, "", err
	}
	conn, err := db.Connect(ctx, layout.DataDir)
	return conn, layout.DataDir, err
}

// workspaceDBIfStarted returns the workspace database if the runtime has
// already been bootstrapped, or (nil, nil) if it has not. Unlike
// workspaceDB it never calls ensureWorkspaceStarted, so it's safe to call
// from ambient side-effect paths (audit persistence, ref creation, sandbox
// bookkeeping) that must not silently spin up the workbench, permission
// service, and their consumer goroutines. If a workspace has been attached
// but its config still points at an empty data directory, this returns
// (nil, nil) as well — callers treat that as "persistence unavailable" and
// no-op rather than erroring the outer request.
func (r *runtimeService) workspaceDBIfStarted(ctx context.Context) (*sql.DB, error) {
	r.mu.Lock()
	runtimeWorkbench := r.runtime
	var workspaceID string
	if r.workspace != nil {
		workspaceID = r.workspace.ID
	}
	r.mu.Unlock()
	if runtimeWorkbench == nil || workspaceID == "" {
		return nil, nil
	}
	ws, err := runtimeWorkbench.GetWorkspace(workspaceID)
	if err != nil {
		return nil, err
	}
	dataDir := strings.TrimSpace(ws.Cfg.Config().Options.DataDirectory)
	if dataDir == "" {
		return nil, nil
	}
	return db.Connect(ctx, dataDir)
}

func (r *runtimeService) configDB(ctx context.Context) (*sql.DB, error) {
	layout, err := resolveDesktopLayout()
	if err != nil {
		return nil, err
	}
	if err := ensureDesktopLayout(layout); err != nil {
		return nil, err
	}
	conn, err := db.Connect(ctx, layout.DataDir)
	if err != nil {
		return nil, err
	}
	return conn, nil
}

func (r *runtimeService) restart() {
	r.startMu.Lock()
	defer r.startMu.Unlock()

	r.closeRuntimeTerminals("closed", "runtime restarted")

	r.mu.Lock()
	defer r.mu.Unlock()

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
	r.starting = false
	r.sessionID = ""
	r.runtimeCtx = nil
	r.cancel = nil
	r.eventStats = runtimeEventStats{}
	r.requests = make(map[string]runtimeRequestState)
	r.sessionTurns = make(map[string]string)
	r.toolEvents = make(map[string]runtimeToolEventState)
	r.toolCalls = nil
	r.objects = runtimeObjectStore{}
	r.worktrees = runtimeWorktreeStore{}
	r.agentTasks = runtimeAgentTaskStore{}
	r.hookExecutions = runtimeHookExecutionStore{}
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
}

func (r *runtimeService) ensureStarted(ctx context.Context) error {
	return r.ensureWorkspaceStarted(ctx, true)
}

func (r *runtimeService) ensureWorkspaceStarted(ctx context.Context, requireConfigured bool) error {
	r.mu.Lock()
	if !r.starting && r.runtime != nil && r.workspace != nil {
		configured := r.runtimeConfigured
		configKnown := r.runtimeConfigKnown
		r.mu.Unlock()
		if requireConfigured && configKnown && !configured {
			return errSelectedModelMissing
		}
		return nil
	}
	r.mu.Unlock()

	r.startMu.Lock()
	defer r.startMu.Unlock()

	r.mu.Lock()
	if !r.starting && r.runtime != nil && r.workspace != nil {
		configured := r.runtimeConfigured
		configKnown := r.runtimeConfigKnown
		r.mu.Unlock()
		if requireConfigured && configKnown && !configured {
			return errSelectedModelMissing
		}
		return nil
	}
	r.starting = true
	r.mu.Unlock()
	defer func() {
		r.mu.Lock()
		r.starting = false
		r.mu.Unlock()
	}()

	layout, err := resolveDesktopLayout()
	if err != nil {
		return err
	}
	if err := ensureDesktopLayout(layout); err != nil {
		return err
	}
	augmentDesktopPath(layout)

	r.mu.Lock()
	workingDir := strings.TrimSpace(r.projectPath)
	r.mu.Unlock()
	if workingDir == "" {
		workingDir = runtimeDefaultWorkingDir()
		if workingDir == "" {
			return fmt.Errorf("failed to resolve working directory")
		}
	}
	workingDir = filepath.Clean(workingDir)
	cfg := config.NewRuntimeConfig(workingDir, layout.DataDir, false)
	store := config.NewRuntimeStore(workingDir, cfg)
	_, _, selectedModelErr := r.applySelectedConfiguredModel(ctx, store)
	if requireConfigured && !store.Config().IsConfigured() {
		if errors.Is(selectedModelErr, errSelectedModelMissing) {
			return errSelectedModelMissing
		}
		return errModelConfigMissing
	}
	if err := applyDesktopSkillConfigToStore(store, layout); err != nil {
		return err
	}
	if err := applyDesktopMCPConfigToStore(store, layout); err != nil {
		return err
	}
	store.Config().SetupAgents()

	logFile := filepath.Join(layout.LogsDir, "agent-builder.log")
	agentbuilderlog.Setup(logFile, false)
	logConfiguredModel(store)

	runtimeCtx, cancel := context.WithCancel(context.Background())
	r.runtimeCtx = runtimeCtx
	r.cancel = cancel
	recorder := &runtimeSchedulerRecorder{service: r}
	r.runtime = workbench.NewWithSchedulerRecorder(runtimeCtx, store, nil, recorder)

	wsRuntime, ws, err := r.runtime.CreateWorkspace(apitypes.Workspace{
		Path:    workingDir,
		DataDir: layout.DataDir,
		Version: version.Version,
		Config:  store.Config(),
		Env:     os.Environ(),
	})
	if err != nil {
		cancel()
		r.runtime = nil
		r.cancel = nil
		return fmt.Errorf("failed to create Agent Builder workspace: %w", err)
	}
	_, _, workspaceSelectedModelErr := r.applySelectedConfiguredModel(ctx, wsRuntime.Cfg)
	workspaceConfigured := wsRuntime.Cfg.Config().IsConfigured()
	if requireConfigured && !workspaceConfigured {
		if errors.Is(workspaceSelectedModelErr, errSelectedModelMissing) {
			return errSelectedModelMissing
		}
		return errModelConfigMissing
	}
	if err := applyDesktopSkillConfigToStore(wsRuntime.Cfg, layout); err != nil {
		return err
	}
	if err := applyDesktopMCPConfigToStore(wsRuntime.Cfg, layout); err != nil {
		return err
	}
	wsRuntime.Cfg.SetupAgents()
	settingsDB, err := db.Connect(ctx, layout.DataDir)
	if err != nil {
		return err
	}
	policy, err := loadRuntimePolicy(ctx, settingsDB)
	_ = db.Release(layout.DataDir)
	if err != nil {
		return err
	}
	policyMode, err := normalizeRuntimePolicyMode(policy.Mode)
	if err != nil {
		return err
	}
	wsRuntime.Permissions.SetPolicy(runtimePermissionPolicy(policy), policyMode)
	r.policy = policy
	r.workspace = &ws
	r.runtimeConfigured = workspaceConfigured
	r.runtimeConfigKnown = true
	go r.consumeRuntimeEvents(runtimeCtx, ws.ID)
	go r.consumeDesktopPermissions(runtimeCtx, ws.ID, wsRuntime.Permissions)
	go r.consumePermissionPolicyApplications(runtimeCtx, ws.ID, wsRuntime.Permissions)
	go r.consumeTodoUpdates(runtimeCtx)

	if workspaceConfigured {
		if err := r.runtime.UpdateAgent(runtimeCtx, ws.ID); err != nil {
			return fmt.Errorf("failed to update Agent Builder agent model: %w", err)
		}
	}

	conn, err := db.Connect(ctx, wsRuntime.Cfg.Config().Options.DataDirectory)
	if err != nil {
		return fmt.Errorf("failed to connect runtime state store: %w", err)
	}
	startedAt := time.Now().UTC()
	r.turns = newRuntimeTurnStore(conn)
	r.userInputs = newRuntimeUserInputStore(conn)
	r.toolCalls = scheduler.New(NewRuntimeToolCallStoreForDB(conn))
	r.objects = newRuntimeObjectStore(conn, wsRuntime.Cfg.Config().Options.DataDirectory)
	r.contextStore = contextmgr.NewSQLStore(conn)
	r.contextManager = contextmgr.NewManager(contextmgr.ManagerOptions{Store: r.contextStore})
	r.promptAssemblies = newRuntimePromptAssemblyStore(conn)
	r.worktrees = newRuntimeWorktreeStore(conn)
	r.agentTasks = newRuntimeAgentTaskStore(conn)
	r.hookExecutions = newRuntimeHookExecutionStore(conn)
	r.eventStore = newRuntimeEventStore(conn)
	r.permissionStore = newRuntimePermissionStore(conn)
	r.mcpRequestStore = newRuntimeMCPRequestStore(conn)
	r.runs = newRuntimeRunStore(conn)
	r.transitions = newRuntimeRunTransitionStore(conn)
	r.recoveryLinks = newRuntimeRecoveryLinkStore(conn)
	r.installWorkbenchAgentTaskRunner(r.runtime, ws.ID)
	if maxSequence, err := r.eventStore.MaxSequence(ctx); err != nil {
		return fmt.Errorf("failed to recover runtime event sequence: %w", err)
	} else if maxSequence > r.nextEventSequence {
		r.nextEventSequence = maxSequence
	}
	if err := r.ensureAgentRolesLoaded(ctx); err != nil {
		return fmt.Errorf("failed to load runtime agent roles: %w", err)
	}
	interrupted, err := r.turns.InterruptUnfinished(ctx)
	if err != nil {
		return fmt.Errorf("failed to recover runtime turns: %w", err)
	}
	interruptedTasks, err := r.agentTasks.InterruptUnfinished(ctx)
	if err != nil {
		return fmt.Errorf("failed to recover runtime agent tasks: %w", err)
	}
	interruptedHooks, err := r.hookExecutions.InterruptRunning(ctx, startedAt.UnixMilli())
	if err != nil {
		return fmt.Errorf("failed to recover runtime hook executions: %w", err)
	}
	interruptedToolCalls, err := cancelUnfinishedRuntimeToolCalls(ctx, r.toolCalls, conn)
	if err != nil {
		return fmt.Errorf("failed to recover runtime tool calls: %w", err)
	}
	recoveredWorktrees, err := r.recoverWorktrees(ctx)
	if err != nil {
		return fmt.Errorf("failed to recover runtime worktrees: %w", err)
	}
	expiredPermissions, err := r.expireInvalidPendingPermissions(ctx)
	if err != nil {
		return fmt.Errorf("failed to recover runtime permissions: %w", err)
	}
	cancelledMCPRequests, err := r.mcpRequestStore.CancelActionableOnStartup(ctx, "runtime restarted; stale MCP request is no longer actionable")
	if err != nil {
		return fmt.Errorf("failed to recover runtime mcp requests: %w", err)
	}
	r.recovery = runtimeRecoveryRecord{
		startedAt:          startedAt,
		interruptedTurns:   append([]RuntimeTurn(nil), interrupted...),
		interruptedTasks:   append([]RuntimeAgentTask(nil), interruptedTasks...),
		worktrees:          append([]RuntimeWorktree(nil), recoveredWorktrees...),
		interruptedHooks:   append([]RuntimeHookExecution(nil), interruptedHooks...),
		expiredPermissions: append([]RuntimePermissionRequest(nil), expiredPermissions...),
	}
	for _, turn := range interrupted {
		r.storeRuntimeEvent(runtimeapi.Event{
			ID:        newRuntimeEventID(),
			Type:      runtimeapi.EventTurnInterrupted,
			CreatedAt: time.Now().UTC().Format(time.RFC3339Nano),
			SessionID: turn.SessionID,
			TurnID:    turn.ID,
			Payload: map[string]any{
				"status": turnStatusInterrupted,
				"error":  turn.Error,
			},
		})
		_ = newRuntimeAuditStore(conn).Append(ctx, RuntimeAuditEvent{
			ID:        newRuntimeEventID(),
			SessionID: turn.SessionID,
			TurnID:    turn.ID,
			Type:      turnStatusInterrupted,
			CreatedAt: time.Now().UTC().Format(time.RFC3339Nano),
			Payload: map[string]any{
				"request_id":   turn.ID,
				"event":        turnStatusInterrupted,
				"workspace_id": ws.ID,
				"session_id":   turn.SessionID,
				"provider":     turn.Provider,
				"model":        turn.Model,
				"error":        turn.Error,
			},
		})
	}
	for _, call := range interruptedToolCalls {
		r.storeRuntimeEvent(runtimeToolCallEvent(runtimeapi.EventToolCallCancelled, call, map[string]any{
			"name":    call.Name,
			"summary": "runtime restarted",
			"status":  string(call.Status),
		}))
		_ = newRuntimeAuditStore(conn).Append(ctx, RuntimeAuditEvent{
			ID:         newRuntimeEventID(),
			SessionID:  call.SessionID,
			TurnID:     call.TurnID,
			ToolCallID: call.ID,
			Type:       "tool_call_cancelled",
			CreatedAt:  time.Now().UTC().Format(time.RFC3339Nano),
			Payload: map[string]any{
				"event":      "tool_call_cancelled",
				"reason":     "runtime restarted",
				"status":     string(call.Status),
				"tool_name":  call.Name,
				"tool_input": call.InputSummary,
			},
		})
	}
	for _, task := range interruptedTasks {
		r.storeRuntimeEvent(runtimeAgentTaskEvent(runtimeapi.EventTaskInterrupted, task))
		_ = newRuntimeAuditStore(conn).Append(ctx, RuntimeAuditEvent{
			ID:         newRuntimeEventID(),
			SessionID:  task.ParentSessionID,
			TurnID:     task.ParentTurnID,
			ToolCallID: task.ParentToolCallID,
			Type:       "task_interrupted",
			CreatedAt:  startedAt.Format(time.RFC3339Nano),
			Payload: redactRuntimePayload(map[string]any{
				"event":               "task_interrupted",
				"workspace_id":        ws.ID,
				"task_id":             task.ID,
				"parent_turn_id":      task.ParentTurnID,
				"parent_session_id":   task.ParentSessionID,
				"parent_tool_call_id": task.ParentToolCallID,
				"child_session_id":    task.ChildSessionID,
				"kind":                task.Kind,
				"role":                task.Role,
				"name":                task.Name,
				"provider":            task.Provider,
				"model":               task.Model,
				"capability_scope":    task.CapabilityScope,
				"allowed_tools":       task.AllowedTools,
				"result_summary":      task.ResultSummary,
				"error":               task.Error,
			}),
		})
	}
	for _, hook := range interruptedHooks {
		r.recordHookExecutionEvent(runtimeapi.EventHookExecutionFailed, hook)
		r.auditHookExecution(ctx, hook, "hook_execution_interrupted")
	}
	for _, wt := range recoveredWorktrees {
		eventType, auditType := worktreeRecoveryEventForStatus(wt.Status)
		r.recordWorktreeEvent(ctx, eventType, auditType, wt, wt.Error)
	}
	for _, perm := range expiredPermissions {
		r.storeRuntimeEvent(runtimeapi.Event{
			ID:         newRuntimeEventID(),
			Type:       runtimeapi.EventPermissionDecided,
			CreatedAt:  startedAt.Format(time.RFC3339Nano),
			SessionID:  perm.SessionID,
			TurnID:     perm.TurnID,
			ToolCallID: perm.ToolCallID,
			Payload: map[string]any{
				"permission_id": perm.ID,
				"tool_name":     perm.ToolName,
				"action":        perm.Action,
				"path":          perm.Path,
				"risk":          perm.Risk,
				"status":        perm.Status,
			},
		})
		_ = newRuntimeAuditStore(conn).Append(ctx, RuntimeAuditEvent{
			ID:           newRuntimeEventID(),
			SessionID:    perm.SessionID,
			TurnID:       perm.TurnID,
			ToolCallID:   perm.ToolCallID,
			PermissionID: perm.ID,
			Type:         "permission_" + perm.Status,
			CreatedAt:    startedAt.Format(time.RFC3339Nano),
			Payload: map[string]any{
				"permission_id": perm.ID,
				"tool_name":     perm.ToolName,
				"action":        perm.Action,
				"path":          perm.Path,
				"risk":          perm.Risk,
				"status":        perm.Status,
				"workspace_id":  ws.ID,
				"session_id":    perm.SessionID,
				"turn_id":       perm.TurnID,
				"tool_call_id":  perm.ToolCallID,
			},
		})
	}
	for _, req := range cancelledMCPRequests {
		r.publishMCPRequestLifecycle(req)
		r.writeMCPRequestAudit(req)
	}
	for _, turn := range interrupted {
		r.recordRunTurnTransition(ctx, runtimeRunTransitionSourceStartupRecovery, turn, "", runtimeRunStatusInterrupted, "runtime startup recovery interrupted unfinished turn")
	}
	for _, task := range interruptedTasks {
		r.recordRunTaskTransition(ctx, runtimeRunTransitionSourceStartupRecovery, task, "", runtimeRunStatusInterrupted, "runtime startup recovery interrupted unfinished task")
	}

	last, listErr := r.runtime.ListSessions(ctx, ws.ID)
	if listErr == nil && len(last) > 0 {
		r.sessionID = last[0].ID
	} else if listErr != nil {
		return fmt.Errorf("failed to restore Agent Builder sessions: %w", listErr)
	}
	return nil
}

func augmentDesktopPath(layout desktopLayout) {
	candidates := []string{
		filepath.Join(layout.Root, "tools"),
		filepath.Join(layout.Root, "bin"),
	}
	current := os.Getenv("PATH")
	parts := filepath.SplitList(current)
	for _, candidate := range candidates {
		if _, err := os.Stat(candidate); err != nil {
			continue
		}
		seen := false
		for _, part := range parts {
			if strings.EqualFold(filepath.Clean(part), filepath.Clean(candidate)) {
				seen = true
				break
			}
		}
		if !seen {
			parts = append([]string{candidate}, parts...)
		}
	}
	if len(parts) > 0 {
		_ = os.Setenv("PATH", strings.Join(parts, string(os.PathListSeparator)))
	}
}
