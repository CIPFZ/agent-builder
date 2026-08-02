package runtime

import (
	"context"
	"encoding/json"
	"fmt"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/CIPFZ/agent-builder/internal/config"
	"github.com/CIPFZ/agent-builder/internal/db"
)

type desktopSkillConfig struct {
	SkillPaths     []string `json:"skill_paths,omitempty"`
	DisabledSkills []string `json:"disabled_skills,omitempty"`
}

func loadDesktopSkillConfig(layout desktopLayout) (desktopSkillConfig, error) {
	if err := ensureDesktopLayout(layout); err != nil {
		return desktopSkillConfig{}, err
	}
	ctx := context.Background()
	conn, err := db.Connect(ctx, layout.DataDir)
	if err != nil {
		return desktopSkillConfig{}, err
	}
	defer db.Release(layout.DataDir) //nolint:errcheck
	rows, err := conn.QueryContext(ctx, `SELECT path, name, enabled, source FROM skill_registrations WHERE scope = 'global' AND project_id = '' ORDER BY id`)
	if err != nil {
		return desktopSkillConfig{}, fmt.Errorf("load skill registrations: %w", err)
	}
	defer rows.Close() //nolint:errcheck
	var cfg desktopSkillConfig
	for rows.Next() {
		var path, name, source string
		var enabled int
		if err := rows.Scan(&path, &name, &enabled, &source); err != nil {
			return desktopSkillConfig{}, err
		}
		if source == "external_path" && path != "" {
			cfg.SkillPaths = append(cfg.SkillPaths, path)
		}
		if enabled == 0 && name != "" {
			cfg.DisabledSkills = append(cfg.DisabledSkills, name)
		}
	}
	return desktopSkillConfig{SkillPaths: normalizeStringList(cfg.SkillPaths), DisabledSkills: normalizeSkillNames(cfg.DisabledSkills)}, rows.Err()
}

func parseDesktopSkillConfig(data []byte, path string) (desktopSkillConfig, error) {
	var cfg desktopSkillConfig
	if err := json.Unmarshal(data, &cfg); err != nil {
		return desktopSkillConfig{}, fmt.Errorf("failed to parse desktop skill config %s: %w", path, err)
	}
	cfg.SkillPaths = normalizeStringList(cfg.SkillPaths)
	cfg.DisabledSkills = normalizeSkillNames(cfg.DisabledSkills)
	return cfg, nil
}

func saveDesktopSkillConfig(layout desktopLayout, cfg desktopSkillConfig) error {
	if err := ensureDesktopLayout(layout); err != nil {
		return err
	}
	cfg.SkillPaths = normalizeStringList(cfg.SkillPaths)
	cfg.DisabledSkills = normalizeSkillNames(cfg.DisabledSkills)
	ctx := context.Background()
	conn, err := db.Connect(ctx, layout.DataDir)
	if err != nil {
		return err
	}
	defer db.Release(layout.DataDir) //nolint:errcheck
	tx, err := conn.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback() //nolint:errcheck
	if _, err := tx.ExecContext(ctx, `DELETE FROM skill_registrations WHERE scope = 'global' AND project_id = ''`); err != nil {
		return err
	}
	now := time.Now().UnixMilli()
	for _, path := range cfg.SkillPaths {
		id := "global:path:" + filepath.ToSlash(path)
		if _, err := tx.ExecContext(ctx, `INSERT INTO skill_registrations (id, scope, project_id, path, name, enabled, source, updated_at) VALUES (?, 'global', '', ?, '', 1, 'external_path', ?)`, id, path, now); err != nil {
			return err
		}
	}
	for _, name := range cfg.DisabledSkills {
		if _, err := tx.ExecContext(ctx, `INSERT INTO skill_registrations (id, scope, project_id, path, name, enabled, source, updated_at) VALUES (?, 'global', '', '', ?, 0, 'builtin', ?)`, "global:name:"+name, name, now); err != nil {
			return err
		}
	}
	return tx.Commit()
}

func desktopSkillConfigForRuntime(layout desktopLayout) (desktopSkillConfig, error) {
	cfg, err := loadDesktopSkillConfig(layout)
	if err != nil {
		return desktopSkillConfig{}, err
	}
	cfg.SkillPaths = appendRuntimeSkillPath(cfg.SkillPaths, desktopSkillsDir(layout))
	return cfg, nil
}

func applyDesktopSkillConfigToStore(store *config.ConfigStore, layout desktopLayout) error {
	cfg, err := desktopSkillConfigForRuntime(layout)
	if err != nil {
		return err
	}
	if store.Config().Options == nil {
		store.Config().Options = &config.Options{}
	}
	for _, path := range cfg.SkillPaths {
		store.Config().Options.SkillsPaths = appendRuntimeSkillPath(store.Config().Options.SkillsPaths, path)
	}
	for _, name := range cfg.DisabledSkills {
		store.Config().Options.DisabledSkills = appendRuntimeSkillName(store.Config().Options.DisabledSkills, name)
	}
	return nil
}

func normalizeStringList(values []string) []string {
	result := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		result = append(result, filepath.Clean(value))
	}
	slices.Sort(result)
	return slices.Compact(result)
}

func normalizeSkillNames(values []string) []string {
	result := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		result = append(result, value)
	}
	slices.Sort(result)
	return slices.Compact(result)
}
