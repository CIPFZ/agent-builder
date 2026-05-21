package runtime

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"slices"

	"github.com/charmbracelet/crush/internal/config"
)

type desktopMCPConfig struct {
	Servers config.MCPs `json:"servers,omitempty"`
}

func loadDesktopMCPConfig(layout desktopLayout) (desktopMCPConfig, error) {
	data, err := os.ReadFile(layout.MCPConfigPath)
	if errors.Is(err, os.ErrNotExist) {
		return desktopMCPConfig{}, nil
	}
	if err != nil {
		return desktopMCPConfig{}, fmt.Errorf("failed to read desktop mcp config: %w", err)
	}
	var cfg desktopMCPConfig
	if err := json.Unmarshal(data, &cfg); err != nil {
		return desktopMCPConfig{}, fmt.Errorf("failed to parse desktop mcp config %s: %w", layout.MCPConfigPath, err)
	}
	cfg.Servers = normalizeMCPs(cfg.Servers)
	return cfg, nil
}

func saveDesktopMCPConfig(layout desktopLayout, cfg desktopMCPConfig) error {
	if err := ensureDesktopLayout(layout); err != nil {
		return err
	}
	cfg.Servers = normalizeMCPs(cfg.Servers)
	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to encode desktop mcp config: %w", err)
	}
	data = append(data, '\n')
	if err := os.WriteFile(layout.MCPConfigPath, data, 0o600); err != nil {
		return fmt.Errorf("failed to write desktop mcp config: %w", err)
	}
	return nil
}

func applyDesktopMCPConfigToStore(store *config.ConfigStore, layout desktopLayout) error {
	cfg, err := loadDesktopMCPConfig(layout)
	if err != nil {
		return err
	}
	if len(cfg.Servers) == 0 {
		return nil
	}
	if store.Config().MCP == nil {
		store.Config().MCP = config.MCPs{}
	}
	for name, server := range cfg.Servers {
		store.Config().MCP[name] = server
	}
	return nil
}

func normalizeMCPs(values config.MCPs) config.MCPs {
	if len(values) == 0 {
		return nil
	}
	result := make(config.MCPs, len(values))
	names := make([]string, 0, len(values))
	for name := range values {
		names = append(names, name)
	}
	slices.Sort(names)
	for _, name := range names {
		server := values[name]
		server.Args = sortedUniqueStrings(server.Args)
		server.EnabledTools = sortedUniqueStrings(server.EnabledTools)
		server.DisabledTools = sortedUniqueStrings(server.DisabledTools)
		server.Env = cloneStringMap(server.Env)
		server.Headers = cloneStringMap(server.Headers)
		result[name] = server
	}
	return result
}
