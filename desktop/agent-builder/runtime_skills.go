package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/charmbracelet/crush/internal/config"
	"github.com/charmbracelet/crush/internal/runtimeapi"
	"github.com/charmbracelet/crush/internal/skills"
)

func (r *runtimeService) Skills(ctx context.Context) (RuntimeSkillsResponse, error) {
	if err := r.ensureStarted(ctx); err != nil {
		return RuntimeSkillsResponse{}, err
	}
	return r.refreshSkills(ctx, false)
}

func (r *runtimeService) RefreshSkills(ctx context.Context) (RuntimeSkillsResponse, error) {
	if err := r.ensureStarted(ctx); err != nil {
		return RuntimeSkillsResponse{}, err
	}
	return r.refreshSkills(ctx, true)
}

func (r *runtimeService) SetSkillEnabled(ctx context.Context, req RuntimeSkillToggleRequest) (RuntimeSkillsResponse, error) {
	if err := r.ensureStarted(ctx); err != nil {
		return RuntimeSkillsResponse{}, err
	}
	name := strings.TrimSpace(req.Name)
	if name == "" {
		return RuntimeSkillsResponse{}, errors.New("skill name is required")
	}

	r.mu.Lock()
	wsID := r.workspace.ID
	r.mu.Unlock()

	ws, err := r.runtime.GetWorkspace(wsID)
	if err != nil {
		return RuntimeSkillsResponse{}, err
	}
	cfg := ws.Cfg
	if cfg.Config().Options == nil {
		cfg.Config().Options = &config.Options{}
	}
	disabled := slices.DeleteFunc(slices.Clone(cfg.Config().Options.DisabledSkills), func(existing string) bool {
		return existing == name
	})
	if !req.Enabled {
		disabled = append(disabled, name)
	}
	slices.Sort(disabled)
	disabled = slices.Compact(disabled)
	cfg.Config().Options.DisabledSkills = disabled
	if err := cfg.SetConfigField(config.ScopeGlobal, "options.disabled_skills", disabled); err != nil {
		return RuntimeSkillsResponse{}, fmt.Errorf("failed to persist disabled skills: %w", err)
	}

	eventType := runtimeapi.EventSkillEnabled
	if !req.Enabled {
		eventType = runtimeapi.EventSkillDisabled
	}
	r.publishRuntimeEvent(runtimeapi.Event{
		ID:        newRuntimeEventID(),
		Type:      eventType,
		CreatedAt: time.Now().UTC().Format(time.RFC3339Nano),
		Payload: map[string]any{
			"name": name,
		},
	})
	return r.refreshSkills(ctx, true)
}

func (r *runtimeService) AddSkillPath(ctx context.Context, req RuntimeSkillPathRequest) (RuntimeSkillsResponse, error) {
	if err := r.ensureStarted(ctx); err != nil {
		return RuntimeSkillsResponse{}, err
	}
	path := strings.TrimSpace(req.Path)
	if path == "" {
		return RuntimeSkillsResponse{}, errors.New("skill path is required")
	}
	cfg, _, err := r.workspaceConfig(ctx)
	if err != nil {
		return RuntimeSkillsResponse{}, err
	}
	paths, err := r.addSkillPathToConfig(cfg, path)
	if err != nil {
		return RuntimeSkillsResponse{}, err
	}
	if err := cfg.SetConfigField(config.ScopeGlobal, "options.skills_paths", paths); err != nil {
		return RuntimeSkillsResponse{}, fmt.Errorf("failed to persist skill paths: %w", err)
	}
	return r.refreshSkills(ctx, true)
}

func (r *runtimeService) CreateSkill(ctx context.Context, req RuntimeSkillCreateRequest) (RuntimeSkillsResponse, error) {
	if err := r.ensureStarted(ctx); err != nil {
		return RuntimeSkillsResponse{}, err
	}
	name, err := validateRuntimeSkillName(req.Name)
	if err != nil {
		return RuntimeSkillsResponse{}, err
	}
	description := strings.TrimSpace(req.Description)
	if description == "" {
		return RuntimeSkillsResponse{}, errors.New("skill description is required")
	}
	instructions := strings.TrimSpace(req.Instructions)
	if instructions == "" {
		return RuntimeSkillsResponse{}, errors.New("skill instructions are required")
	}
	cfg, _, err := r.workspaceConfig(ctx)
	if err != nil {
		return RuntimeSkillsResponse{}, err
	}
	workingDir := cfg.WorkingDir()
	baseDir := strings.TrimSpace(req.Directory)
	if baseDir == "" {
		baseDir = filepath.Join(workingDir, ".agents", "skills")
	}
	if !filepath.IsAbs(baseDir) {
		baseDir = filepath.Join(workingDir, baseDir)
	}
	baseDir = filepath.Clean(baseDir)
	skillDir := filepath.Join(baseDir, name)
	if err := os.MkdirAll(skillDir, 0o755); err != nil {
		return RuntimeSkillsResponse{}, fmt.Errorf("failed to create skill directory: %w", err)
	}
	skillFile := filepath.Join(skillDir, skills.SkillFileName)
	if _, err := os.Stat(skillFile); err == nil {
		return RuntimeSkillsResponse{}, fmt.Errorf("skill %s already exists at %s", name, skillFile)
	} else if !errors.Is(err, os.ErrNotExist) {
		return RuntimeSkillsResponse{}, fmt.Errorf("failed to inspect skill file: %w", err)
	}
	content := formatRuntimeSkillMarkdown(name, description, instructions)
	if err := os.WriteFile(skillFile, []byte(content), 0o644); err != nil {
		return RuntimeSkillsResponse{}, fmt.Errorf("failed to write skill file: %w", err)
	}
	paths, err := r.addSkillPathToConfig(cfg, baseDir)
	if err != nil {
		return RuntimeSkillsResponse{}, err
	}
	if err := cfg.SetConfigField(config.ScopeGlobal, "options.skills_paths", paths); err != nil {
		return RuntimeSkillsResponse{}, fmt.Errorf("failed to persist skill paths: %w", err)
	}
	event := runtimeapi.NewEvent(newRuntimeEventID(), runtimeapi.EventSkillDiscoveryStarted, time.Now())
	event.Payload = map[string]any{
		"name": name,
		"path": skillFile,
	}
	r.publishRuntimeEvent(event)
	return r.refreshSkills(ctx, true)
}

func (r *runtimeService) addSkillPathToConfig(cfg *config.ConfigStore, path string) ([]string, error) {
	if cfg.Config().Options == nil {
		cfg.Config().Options = &config.Options{}
	}
	normalized := filepath.Clean(path)
	paths := slices.Clone(cfg.Config().Options.SkillsPaths)
	seen := false
	for _, existing := range paths {
		if strings.EqualFold(filepath.Clean(existing), normalized) {
			seen = true
			break
		}
	}
	if !seen {
		paths = append(paths, normalized)
	}
	slices.Sort(paths)
	paths = slices.Compact(paths)
	cfg.Config().Options.SkillsPaths = paths
	return paths, nil
}

func (r *runtimeService) refreshSkills(ctx context.Context, publish bool) (RuntimeSkillsResponse, error) {
	if publish {
		r.publishRuntimeEvent(runtimeapi.NewEvent(newRuntimeEventID(), runtimeapi.EventSkillDiscoveryStarted, time.Now()))
	}

	r.mu.Lock()
	wsID := r.workspace.ID
	r.mu.Unlock()

	ws, err := r.runtime.GetWorkspace(wsID)
	if err != nil {
		return RuntimeSkillsResponse{}, err
	}
	resp := runtimeSkillsFromConfig(ws.Cfg)
	if publish {
		eventType := runtimeapi.EventSkillDiscoveryCompleted
		for _, skill := range resp.Skills {
			if skill.State == "error" {
				eventType = runtimeapi.EventSkillDiscoveryFailed
				break
			}
		}
		event := runtimeapi.NewEvent(newRuntimeEventID(), eventType, time.Now())
		event.Payload = map[string]any{
			"count": len(resp.Skills),
		}
		r.publishRuntimeEvent(event)
	}
	return resp, nil
}

func runtimeSkillsFromConfig(store *config.ConfigStore) RuntimeSkillsResponse {
	opts := store.Config().Options
	builtin, builtinStates := skills.DiscoverBuiltinWithStates()
	discovered := append([]*skills.Skill(nil), builtin...)
	states := append([]*skills.SkillState(nil), builtinStates...)

	if opts != nil && len(opts.SkillsPaths) > 0 {
		paths := make([]string, 0, len(opts.SkillsPaths))
		for _, path := range opts.SkillsPaths {
			expanded := path
			if strings.HasPrefix(expanded, "$") {
				if resolved, err := store.Resolver().ResolveValue(expanded); err == nil {
					expanded = resolved
				}
			}
			paths = append(paths, expanded)
		}
		userSkills, userStates := skills.DiscoverWithStates(paths)
		discovered = append(discovered, userSkills...)
		states = append(states, userStates...)
	}

	allSkills := skills.Deduplicate(discovered)
	states = skills.DeduplicateStates(states)
	skillByName := make(map[string]*skills.Skill, len(allSkills))
	for _, skill := range allSkills {
		skillByName[skill.Name] = skill
	}

	var disabled []string
	if opts != nil {
		disabled = opts.DisabledSkills
	}
	disabledSet := make(map[string]bool, len(disabled))
	for _, name := range disabled {
		disabledSet[name] = true
	}

	result := make([]RuntimeSkill, 0, len(states))
	for _, state := range states {
		runtimeSkill := RuntimeSkill{
			Name:  state.Name,
			Path:  state.Path,
			State: "normal",
		}
		if state.State == skills.StateError {
			runtimeSkill.State = "error"
			if state.Err != nil {
				runtimeSkill.Error = state.Err.Error()
			}
		}
		if skill := skillByName[state.Name]; skill != nil {
			runtimeSkill.Name = skill.Name
			runtimeSkill.Description = skill.Description
			runtimeSkill.Builtin = skill.Builtin
			runtimeSkill.Enabled = !disabledSet[skill.Name] && runtimeSkill.State != "error"
			runtimeSkill.Path = skill.Path
			runtimeSkill.SkillFilePath = skill.SkillFilePath
		}
		result = append(result, runtimeSkill)
	}

	slices.SortStableFunc(result, func(a, b RuntimeSkill) int {
		if c := strings.Compare(strings.ToLower(a.Name), strings.ToLower(b.Name)); c != 0 {
			return c
		}
		return strings.Compare(strings.ToLower(a.Path), strings.ToLower(b.Path))
	})
	return RuntimeSkillsResponse{Skills: result}
}

func validateRuntimeSkillName(value string) (string, error) {
	name := strings.TrimSpace(value)
	if name == "" {
		return "", errors.New("skill name is required")
	}
	for _, char := range name {
		if char >= 'a' && char <= 'z' || char >= '0' && char <= '9' || char == '_' || char == '-' {
			continue
		}
		return "", fmt.Errorf("skill name %q must use only lowercase letters, numbers, underscore, or dash", name)
	}
	return name, nil
}

func formatRuntimeSkillMarkdown(name, description, instructions string) string {
	description = strings.ReplaceAll(strings.TrimSpace(description), "\r\n", "\n")
	instructions = strings.ReplaceAll(strings.TrimSpace(instructions), "\r\n", "\n")
	return fmt.Sprintf("---\nname: %s\ndescription: %s\n---\n\n%s\n", name, description, instructions)
}
