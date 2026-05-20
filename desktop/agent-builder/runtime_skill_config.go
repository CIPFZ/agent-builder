package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/charmbracelet/crush/internal/config"
)

type desktopSkillConfig struct {
	SkillPaths     []string `json:"skill_paths,omitempty"`
	DisabledSkills []string `json:"disabled_skills,omitempty"`
}

func loadDesktopSkillConfig(layout desktopLayout) (desktopSkillConfig, error) {
	data, err := os.ReadFile(layout.SkillConfigPath)
	if errors.Is(err, os.ErrNotExist) {
		legacyPath := legacyDesktopSkillConfigPath(layout)
		data, err = os.ReadFile(legacyPath)
		if errors.Is(err, os.ErrNotExist) {
			return desktopSkillConfig{}, nil
		}
		if err == nil {
			cfg, parseErr := parseDesktopSkillConfig(data, legacyPath)
			if parseErr != nil {
				return desktopSkillConfig{}, parseErr
			}
			_ = saveDesktopSkillConfig(layout, cfg)
			return cfg, nil
		}
	}
	if err != nil {
		return desktopSkillConfig{}, fmt.Errorf("failed to read desktop skill config: %w", err)
	}
	return parseDesktopSkillConfig(data, layout.SkillConfigPath)
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
	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to encode desktop skill config: %w", err)
	}
	data = append(data, '\n')
	if err := os.WriteFile(layout.SkillConfigPath, data, 0o600); err != nil {
		return fmt.Errorf("failed to write desktop skill config: %w", err)
	}
	return nil
}

func desktopSkillConfigForRuntime(layout desktopLayout) (desktopSkillConfig, error) {
	cfg, err := loadDesktopSkillConfig(layout)
	if err != nil {
		return desktopSkillConfig{}, err
	}
	cfg.SkillPaths = appendRuntimeSkillPath(cfg.SkillPaths, desktopSkillsDir(layout))
	return cfg, nil
}

func legacyDesktopSkillConfigPath(layout desktopLayout) string {
	return filepath.Join(layout.ConfigDir, "skills.local.json")
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
