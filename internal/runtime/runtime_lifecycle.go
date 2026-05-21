package runtime

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/charmbracelet/crush/internal/backend"
	"github.com/charmbracelet/crush/internal/config"
	"github.com/charmbracelet/crush/internal/db"
	crushlog "github.com/charmbracelet/crush/internal/log"
	"github.com/charmbracelet/crush/internal/proto"
	"github.com/charmbracelet/crush/internal/version"
)

func (r *runtimeService) workspaceConfig(ctx context.Context) (*config.ConfigStore, string, error) {
	if err := r.ensureStarted(ctx); err != nil {
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

func (r *runtimeService) APIEndpoint(_ context.Context) (RuntimeAPIEndpointResponse, error) {
	if r.httpAPI == nil {
		r.httpAPI = newRuntimeHTTPServer(r)
	}
	if err := r.httpAPI.Start(); err != nil {
		return RuntimeAPIEndpointResponse{}, err
	}
	return RuntimeAPIEndpointResponse{
		URL:   r.httpAPI.URL(),
		Token: r.httpAPI.Token(),
	}, nil
}

func (r *runtimeService) restart() {
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
	r.sessionID = ""
	r.runtimeCtx = nil
	r.cancel = nil
	r.eventStats = runtimeEventStats{}
	r.requests = make(map[string]runtimeRequestState)
	r.sessionTurns = make(map[string]string)
	r.toolEvents = make(map[string]runtimeToolEventState)
	r.permissions = make(map[string]pendingRuntimePermission)
	r.events = nil
}

func (r *runtimeService) ensureStarted(ctx context.Context) error {
	r.mu.Lock()
	if r.runtime != nil && r.workspace != nil {
		r.mu.Unlock()
		return nil
	}
	r.mu.Unlock()

	r.mu.Lock()
	defer r.mu.Unlock()
	if r.runtime != nil && r.workspace != nil {
		return nil
	}

	layout, err := resolveDesktopLayout()
	if err != nil {
		return err
	}
	if err := ensureDesktopLayout(layout); err != nil {
		return err
	}
	augmentDesktopPath(layout)

	workingDir, err := os.Getwd()
	if err != nil {
		return fmt.Errorf("failed to resolve working directory: %w", err)
	}
	workingDir = filepath.Clean(workingDir)

	cfg := config.NewRuntimeConfig(workingDir, layout.DataDir, false)
	store := config.NewRuntimeStore(workingDir, cfg, layout.ModelConfigPath)
	localResult := applyLocalModelConfig(store, layout)
	if localResult.Error != nil {
		return localResult.Error
	}
	if !store.Config().IsConfigured() {
		return errModelConfigMissing
	}
	if err := applyDesktopSkillConfigToStore(store, layout); err != nil {
		return err
	}
	if err := applyDesktopMCPConfigToStore(store, layout); err != nil {
		return err
	}
	store.Config().SetupAgents()
	applyDesktopProxy(localResult)

	logFile := filepath.Join(layout.LogsDir, "agent-builder.log")
	crushlog.Setup(logFile, false)
	logConfiguredModel(store)

	runtimeCtx, cancel := context.WithCancel(context.Background())
	r.runtimeCtx = runtimeCtx
	r.cancel = cancel
	r.runtime = backend.New(runtimeCtx, store, nil)

	wsRuntime, ws, err := r.runtime.CreateWorkspace(proto.Workspace{
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
		return fmt.Errorf("failed to create Crush workspace: %w", err)
	}
	workspaceLocalResult := applyLocalModelConfig(wsRuntime.Cfg, layout)
	if workspaceLocalResult.Error != nil {
		return workspaceLocalResult.Error
	}
	if !wsRuntime.Cfg.Config().IsConfigured() {
		return errModelConfigMissing
	}
	if err := applyDesktopSkillConfigToStore(wsRuntime.Cfg, layout); err != nil {
		return err
	}
	if err := applyDesktopMCPConfigToStore(wsRuntime.Cfg, layout); err != nil {
		return err
	}
	wsRuntime.Cfg.SetupAgents()
	r.workspace = &ws
	go r.consumeRuntimeEvents(runtimeCtx, ws.ID)
	go r.consumeDesktopPermissions(runtimeCtx, ws.ID, wsRuntime.Permissions)

	if err := r.runtime.UpdateAgent(runtimeCtx, ws.ID); err != nil {
		return fmt.Errorf("failed to update Crush agent model: %w", err)
	}

	last, listErr := r.runtime.ListSessions(ctx, ws.ID)
	if listErr == nil && len(last) > 0 {
		r.sessionID = last[0].ID
	} else if listErr != nil {
		return fmt.Errorf("failed to restore Crush sessions: %w", listErr)
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
