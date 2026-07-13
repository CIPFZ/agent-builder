package runtime

import (
	"context"
	"encoding/json"
	"fmt"
	"slices"
	"time"

	"github.com/CIPFZ/agent-builder/internal/config"
	"github.com/CIPFZ/agent-builder/internal/db"
)

type desktopMCPConfig struct {
	Servers config.MCPs `json:"servers,omitempty"`
}

func loadDesktopMCPConfig(layout desktopLayout) (desktopMCPConfig, error) {
	if err := ensureDesktopLayout(layout); err != nil {
		return desktopMCPConfig{}, err
	}
	conn, err := db.Connect(context.Background(), layout.DataDir)
	if err != nil {
		return desktopMCPConfig{}, err
	}
	defer db.Release(layout.DataDir) //nolint:errcheck
	rows, err := conn.Query(`SELECT name, config_json FROM mcp_servers WHERE scope = 'global' AND project_id = '' ORDER BY name`)
	if err != nil {
		return desktopMCPConfig{}, fmt.Errorf("load mcp servers: %w", err)
	}
	defer rows.Close() //nolint:errcheck
	cfg := desktopMCPConfig{Servers: config.MCPs{}}
	for rows.Next() {
		var name, configJSON string
		if err := rows.Scan(&name, &configJSON); err != nil {
			return desktopMCPConfig{}, err
		}
		var server config.MCPConfig
		if err := json.Unmarshal([]byte(configJSON), &server); err != nil {
			return desktopMCPConfig{}, fmt.Errorf("decode mcp server %s: %w", name, err)
		}
		cfg.Servers[name] = server
	}
	cfg.Servers = normalizeMCPs(cfg.Servers)
	return cfg, rows.Err()
}

func saveDesktopMCPConfig(layout desktopLayout, cfg desktopMCPConfig) error {
	if err := ensureDesktopLayout(layout); err != nil {
		return err
	}
	cfg.Servers = normalizeMCPs(cfg.Servers)
	conn, err := db.Connect(context.Background(), layout.DataDir)
	if err != nil {
		return err
	}
	defer db.Release(layout.DataDir) //nolint:errcheck
	tx, err := conn.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback() //nolint:errcheck
	if _, err := tx.Exec(`DELETE FROM mcp_servers WHERE scope = 'global' AND project_id = ''`); err != nil {
		return err
	}
	now := time.Now().UnixMilli()
	for name, server := range cfg.Servers {
		data, err := json.Marshal(server)
		if err != nil {
			return fmt.Errorf("encode mcp server %s: %w", name, err)
		}
		if _, err := tx.Exec(`INSERT INTO mcp_servers (name, scope, project_id, config_json, updated_at) VALUES (?, 'global', '', ?, ?)`, name, string(data), now); err != nil {
			return err
		}
	}
	return tx.Commit()
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
