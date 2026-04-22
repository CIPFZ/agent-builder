package app

import (
	"fmt"
	"path/filepath"
	"strings"

	"myclaw/internal/compaction"
	"myclaw/internal/config"
	"myclaw/internal/llm"
	"myclaw/internal/permissions"
	"myclaw/internal/runtime"
	"myclaw/internal/session"
	"myclaw/internal/workspace"
)

type bootstrapOptions struct {
	FallbackWorkspaceRoots []string
	Compactor              *compaction.Service
}

type runtimeBootstrap struct {
	Sessions *session.Manager
	Policy   permissions.Policy
	Runner   *runtime.Runner
}

func bootstrapRuntime(baseDir string, cfg config.Config, options bootstrapOptions) (*runtimeBootstrap, error) {
	workspaceRoots, err := resolveBootstrapWorkspaceRoots(baseDir, cfg.Permissions.WorkspaceRoots, options.FallbackWorkspaceRoots)
	if err != nil {
		return nil, fmt.Errorf("resolve workspace roots: %w", err)
	}
	policy, err := permissions.SetupPolicy(permissions.Policy{
		Mode:                     permissions.Mode(cfg.Permissions.Mode),
		SubagentMode:             permissions.Mode(cfg.Permissions.SubagentMode),
		PlanMode:                 cfg.Permissions.PlanMode,
		AutoMode:                 cfg.Permissions.AutoMode,
		WorkspaceRoots:           workspaceRoots,
		Rules:                    cfg.Permissions.Rules,
		DangerousCommandPatterns: cfg.Permissions.DangerousCommandPatterns,
	})
	if err != nil {
		return nil, fmt.Errorf("invalid permission policy: %w", err)
	}
	sessions, err := newPersistentSessionManager(defaultSessionStoreRoot(baseDir))
	if err != nil {
		sessions = session.NewManager(nil)
	}
	runner := runtime.NewRunnerWithOptions(sessions, llm.NewClientFromConfig(cfg.LLM), workspace.NewLoader(""), nil, runtime.Options{
		PermissionPolicy:          policy,
		Compactor:                 options.Compactor,
		PermissionUpdatePersister: config.NewPermissionUpdatePersister(baseDir),
		MainLoopModel:             cfg.LLM.Model,
		LLMProvider:               cfg.LLM.Provider,
		ModelCatalog:              llm.NewModelCatalogFromConfig(cfg.LLM),
		DisableMCPPromptSkills:    !cfg.MCP.Skills,
	})
	return &runtimeBootstrap{
		Sessions: sessions,
		Policy:   policy,
		Runner:   runner,
	}, nil
}

func resolveBootstrapWorkspaceRoots(baseDir string, configured []string, fallback []string) ([]string, error) {
	if len(configured) > 0 {
		return configured, nil
	}
	if len(fallback) > 0 {
		resolved := make([]string, 0, len(fallback))
		for _, root := range fallback {
			if strings.TrimSpace(root) == "" {
				continue
			}
			resolved = append(resolved, resolveBootstrapPath(baseDir, root))
		}
		return resolved, nil
	}
	return resolveTUIWorkspaceRoots(baseDir, nil)
}

func resolveBootstrapPath(baseDir, value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return ""
	}
	if filepath.IsAbs(value) {
		return filepath.Clean(value)
	}
	base := baseDir
	if strings.TrimSpace(base) == "" {
		base = "."
	}
	return filepath.Clean(filepath.Join(base, value))
}
