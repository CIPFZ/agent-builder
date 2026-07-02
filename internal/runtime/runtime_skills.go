package runtime

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/CIPFZ/agent-builder/internal/config"
	"github.com/CIPFZ/agent-builder/internal/permission"
	"github.com/CIPFZ/agent-builder/internal/runtimeapi"
	"github.com/CIPFZ/agent-builder/internal/skills"
)

func (r *runtimeService) Skills(ctx context.Context) (RuntimeSkillsResponse, error) {
	if err := r.ensureWorkspaceStarted(ctx, false); err != nil {
		return RuntimeSkillsResponse{}, err
	}
	return r.refreshSkills(ctx, false)
}

func (r *runtimeService) RefreshSkills(ctx context.Context) (RuntimeSkillsResponse, error) {
	if err := r.ensureWorkspaceStarted(ctx, false); err != nil {
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
	layout, err := resolveDesktopLayout()
	if err != nil {
		return RuntimeSkillsResponse{}, err
	}
	desktopCfg, err := loadDesktopSkillConfig(layout)
	if err != nil {
		return RuntimeSkillsResponse{}, err
	}
	disabled := slices.DeleteFunc(slices.Clone(desktopCfg.DisabledSkills), func(existing string) bool {
		return existing == name
	})
	if !req.Enabled {
		disabled = append(disabled, name)
	}
	disabled = normalizeSkillNames(disabled)
	desktopCfg.DisabledSkills = disabled
	if err := saveDesktopSkillConfig(layout, desktopCfg); err != nil {
		return RuntimeSkillsResponse{}, fmt.Errorf("failed to persist disabled skills: %w", err)
	}
	if cfg.Config().Options == nil {
		cfg.Config().Options = &config.Options{}
	}
	runtimeDisabled := slices.DeleteFunc(slices.Clone(cfg.Config().Options.DisabledSkills), func(existing string) bool {
		return existing == name
	})
	if !req.Enabled {
		runtimeDisabled = appendRuntimeSkillName(runtimeDisabled, name)
	}
	cfg.Config().Options.DisabledSkills = runtimeDisabled

	eventType := runtimeapi.EventSkillEnabled
	if !req.Enabled {
		eventType = runtimeapi.EventSkillDisabled
	}
	r.publishRuntimeEvent(runtimeapi.Event{
		ID:        newRuntimeEventID(),
		Type:      eventType,
		CreatedAt: time.Now().UTC().Format(time.RFC3339Nano),
		Payload: map[string]any{
			"name":          name,
			"capability_id": "skill:" + name,
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
	layout, err := resolveDesktopLayout()
	if err != nil {
		return RuntimeSkillsResponse{}, err
	}
	desktopCfg, err := loadDesktopSkillConfig(layout)
	if err != nil {
		return RuntimeSkillsResponse{}, err
	}
	desktopCfg.SkillPaths = appendRuntimeSkillPath(desktopCfg.SkillPaths, path)
	if err := saveDesktopSkillConfig(layout, desktopCfg); err != nil {
		return RuntimeSkillsResponse{}, fmt.Errorf("failed to persist skill paths: %w", err)
	}
	if cfg.Config().Options == nil {
		cfg.Config().Options = &config.Options{}
	}
	cfg.Config().Options.SkillsPaths = appendRuntimeSkillPath(cfg.Config().Options.SkillsPaths, path)
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
	baseDir := strings.TrimSpace(req.Directory)
	if baseDir == "" {
		baseDir = r.desktopSkillsDir()
	} else if !filepath.IsAbs(baseDir) {
		cfg, _, err := r.workspaceConfig(ctx)
		if err != nil {
			return RuntimeSkillsResponse{}, err
		}
		baseDir = filepath.Join(cfg.WorkingDir(), baseDir)
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
	event := runtimeapi.NewEvent(newRuntimeEventID(), runtimeapi.EventSkillDiscoveryStarted, time.Now())
	event.Payload = map[string]any{
		"name": name,
		"path": skillFile,
	}
	r.publishRuntimeEvent(event)
	return r.refreshSkills(ctx, true)
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
	if publish {
		if err := r.runtime.RefreshWorkspaceSkills(ctx, wsID); err != nil {
			return RuntimeSkillsResponse{}, err
		}
	}
	resp := r.runtimeSkillsFromWorkspaceConfig(ws.Cfg, r.desktopSkillPaths()...)
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
			"count":  len(resp.Skills),
			"skills": runtimeSkillEventSummaries(resp.Skills),
		}
		r.publishRuntimeEvent(event)
		for _, skill := range resp.Skills {
			if skill.State == capabilityStateFailed {
				r.publishSkillActivationEvent(runtimeapi.EventSkillActivationFailed, skill, "", skill.Error)
				continue
			}
			if skill.Activation.Included {
				r.publishSkillActivationEvent(runtimeapi.EventSkillActivated, skill, skill.Activation.Reason, "")
			}
		}
	}
	return resp, nil
}

func (r *runtimeService) runtimeSkillsFromWorkspaceConfig(store *config.ConfigStore, extraPaths ...string) RuntimeSkillsResponse {
	return runtimeSkillsFromConfigWithPolicy(store, r.currentPolicyMode(), extraPaths...)
}

func runtimeSkillsFromConfig(store *config.ConfigStore, extraPaths ...string) RuntimeSkillsResponse {
	return runtimeSkillsFromConfigWithPolicy(store, permission.PolicyModeAsk, extraPaths...)
}

func runtimeSkillsFromConfigWithPolicy(store *config.ConfigStore, policyMode permission.PolicyMode, extraPaths ...string) RuntimeSkillsResponse {
	opts := store.Config().Options
	builtin, builtinStates := skills.DiscoverBuiltinWithStates()
	discovered := append([]*skills.Skill(nil), builtin...)
	states := append([]*skills.SkillState(nil), builtinStates...)

	paths := runtimeSkillPaths(store, extraPaths...)
	if len(paths) > 0 {
		expandedPaths := make([]string, 0, len(paths))
		for _, path := range paths {
			expanded := path
			if strings.HasPrefix(expanded, "$") {
				if resolved, err := store.Resolver().ResolveValue(expanded); err == nil {
					expanded = resolved
				}
			}
			expandedPaths = append(expandedPaths, expanded)
		}
		userSkills, userStates := skills.DiscoverWithStates(expandedPaths)
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

	policy := runtimePolicyFromMode(policyMode, 0)
	result := make([]RuntimeSkill, 0, len(states))
	for _, state := range states {
		runtimeSkill := RuntimeSkill{
			Name:         state.Name,
			Path:         state.Path,
			State:        capabilityStateUnloaded,
			Enabled:      true,
			CapabilityID: "skill:" + state.Name,
			Activation: RuntimeSkillActivationMetadata{
				Available: true,
				Included:  true,
				Reason:    "runtime included in prompt",
			},
			ActivationMetadata: RuntimeSkillActivationMetadata{
				Available: true,
				Included:  true,
				Reason:    "runtime included in prompt",
			},
		}
		if state.State == skills.StateError {
			runtimeSkill.State = capabilityStateFailed
			runtimeSkill.Enabled = false
			runtimeSkill.Reason = "skill_diagnostic"
			if state.Err != nil {
				runtimeSkill.Error = state.Err.Error()
				runtimeSkill.Diagnostics = state.Err.Error()
			}
			runtimeSkill.Activation = RuntimeSkillActivationMetadata{Available: false, Included: false, Reason: "failed diagnostics"}
			runtimeSkill.ActivationMetadata = runtimeSkill.Activation
		}
		if skill := skillByName[state.Name]; skill != nil {
			runtimeSkill.Name = skill.Name
			runtimeSkill.Description = skill.Description
			runtimeSkill.Builtin = skill.Builtin
			runtimeSkill.Enabled = !disabledSet[skill.Name] && runtimeSkill.State != capabilityStateFailed
			runtimeSkill.Path = skill.Path
			runtimeSkill.SkillFilePath = skill.SkillFilePath
			runtimeSkill.CapabilityID = "skill:" + skill.Name
			runtimeSkill.AllowedTools = normalizeSkillMetadataList(skill.AllowedTools)
			runtimeSkill.Metadata = cloneSkillMetadata(skill.Metadata)
			if len(runtimeSkill.AllowedTools) > 0 {
				runtimeSkill.PolicyReason = "Skill allowed_tools metadata is preserved for policy inspection and does not expand runtime permissions."
			}
			if !runtimeSkill.Enabled && runtimeSkill.State != capabilityStateFailed {
				runtimeSkill.State = capabilityStateDisabled
				runtimeSkill.Reason = "disabled_skill"
				runtimeSkill.Diagnostics = "Skill is disabled by runtime configuration."
				runtimeSkill.Activation = RuntimeSkillActivationMetadata{Available: true, Included: false, Reason: "excluded by disabled config"}
				runtimeSkill.ActivationMetadata = runtimeSkill.Activation
			}
		}
		if runtimeSkill.CapabilityID == "skill:" {
			runtimeSkill.CapabilityID = "skill:" + runtimeSkill.Name
		}
		if runtimeSkill.State == capabilityStateUnloaded && runtimeSkill.Enabled {
			decision := evaluateRuntimeCapabilityPolicy(policy, RuntimeCapability{
				ID:          runtimeSkill.CapabilityID,
				Kind:        "skill",
				Name:        runtimeSkill.Name,
				Source:      runtimeSkill.Path,
				Enabled:     runtimeSkill.Enabled,
				Risk:        "context",
				Description: runtimeSkill.Description,
				State:       runtimeSkill.State,
			})
			runtimeSkill.PolicyMode = string(decision.Mode)
			runtimeSkill.PolicyRisk = string(decision.Risk)
			runtimeSkill.PolicyReason = firstNonEmpty(runtimeSkill.PolicyReason, decision.Reason)
			if decision.Decision == permission.PolicyDeny {
				runtimeSkill.Enabled = false
				runtimeSkill.State = capabilityStateDisabled
				runtimeSkill.Reason = "policy_denied"
				runtimeSkill.Diagnostics = decision.Reason
				runtimeSkill.Activation = RuntimeSkillActivationMetadata{Available: true, Included: false, Reason: decision.Reason}
				runtimeSkill.ActivationMetadata = runtimeSkill.Activation
			}
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

func runtimeSkillPolicySummary(skill RuntimeSkill) string {
	if len(skill.AllowedTools) > 0 {
		return "read skill activation metadata with allowed_tools policy hints"
	}
	return "read skill activation metadata"
}

func normalizeSkillMetadataList(values []string) []string {
	if len(values) == 0 {
		return nil
	}
	result := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" || slices.Contains(result, value) {
			continue
		}
		result = append(result, value)
	}
	slices.Sort(result)
	return result
}

func cloneSkillMetadata(values map[string]string) map[string]string {
	if len(values) == 0 {
		return nil
	}
	result := make(map[string]string, len(values))
	for key, value := range values {
		result[key] = value
	}
	return result
}

func runtimeSkillEventSummaries(skills []RuntimeSkill) []map[string]any {
	result := make([]map[string]any, 0, len(skills))
	for _, skill := range skills {
		result = append(result, map[string]any{
			"name":          skill.Name,
			"capability_id": skill.CapabilityID,
			"path":          skill.Path,
			"enabled":       skill.Enabled,
			"state":         skill.State,
			"reason":        skill.Reason,
			"error":         skill.Error,
		})
	}
	return result
}

func (r *runtimeService) publishSkillActivationEvent(eventType string, skill RuntimeSkill, reason, errText string) {
	payload := map[string]any{
		"name":          skill.Name,
		"capability_id": firstNonEmpty(skill.CapabilityID, "skill:"+skill.Name),
		"path":          skill.Path,
		"state":         skill.State,
		"reason":        firstNonEmpty(reason, skill.Reason),
	}
	if errText != "" {
		payload["error"] = errText
	}
	r.storeRuntimeEvent(runtimeapi.Event{
		ID:        newRuntimeEventID(),
		Type:      eventType,
		CreatedAt: time.Now().UTC().Format(time.RFC3339Nano),
		Payload:   payload,
	})
}

func (r *runtimeService) desktopSkillsDir() string {
	layout, err := resolveDesktopLayout()
	if err != nil {
		return ""
	}
	return desktopSkillsDir(layout)
}

func (r *runtimeService) desktopSkillPaths() []string {
	layout, err := resolveDesktopLayout()
	if err != nil {
		return nil
	}
	cfg, err := desktopSkillConfigForRuntime(layout)
	if err != nil {
		return nil
	}
	return cfg.SkillPaths
}

func desktopSkillsDir(layout desktopLayout) string {
	return filepath.Join(layout.DataDir, "skills")
}

func runtimeSkillPaths(store *config.ConfigStore, extraPaths ...string) []string {
	var paths []string
	if store.Config().Options != nil {
		paths = slices.Clone(store.Config().Options.SkillsPaths)
	}
	for _, path := range extraPaths {
		paths = appendRuntimeSkillPath(paths, path)
	}
	return paths
}

func appendRuntimeSkillPath(paths []string, path string) []string {
	path = strings.TrimSpace(path)
	if path == "" {
		return paths
	}
	normalized := filepath.Clean(path)
	for _, existing := range paths {
		if strings.EqualFold(filepath.Clean(existing), normalized) {
			return paths
		}
	}
	paths = append(paths, normalized)
	slices.Sort(paths)
	return slices.Compact(paths)
}

func appendRuntimeSkillName(names []string, name string) []string {
	name = strings.TrimSpace(name)
	if name == "" {
		return names
	}
	for _, existing := range names {
		if existing == name {
			return names
		}
	}
	names = append(names, name)
	slices.Sort(names)
	return slices.Compact(names)
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
