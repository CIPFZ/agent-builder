package config

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"

	"myclaw/internal/permissions"
	"myclaw/internal/session"
)

type PermissionUpdatePersister struct {
	dir string
}

func NewPermissionUpdatePersister(dir string) *PermissionUpdatePersister {
	return &PermissionUpdatePersister{dir: dir}
}

func (p *PermissionUpdatePersister) PersistPermissionUpdates(_ context.Context, _ session.Session, updates []permissions.PermissionUpdate) error {
	if len(updates) == 0 {
		return nil
	}
	changed := false
	updatesByPath := make(map[string][]permissions.PermissionUpdate)
	for _, update := range updates {
		if !isPersistentPermissionDestination(update.Destination) {
			continue
		}
		targetPath := p.pathForDestination(update.Destination)
		updatesByPath[targetPath] = append(updatesByPath[targetPath], update)
	}
	for targetPath, targetUpdates := range updatesByPath {
		targetCfg, err := readWritableFileConfig(targetPath)
		if err != nil {
			return err
		}
		targetChanged := false
		for _, update := range targetUpdates {
			if applyPermissionUpdateToFileConfig(&targetCfg, update) {
				targetChanged = true
			}
		}
		if targetChanged {
			if err := writeWritableFileConfig(targetPath, targetCfg); err != nil {
				return err
			}
			if isLocalSettingsPath(targetPath, p.dir) {
				if err := ensureLocalSettingsGitignored(p.dir); err != nil {
					return err
				}
			}
			changed = true
		}
	}
	if !changed {
		return nil
	}
	return nil
}

func (p *PermissionUpdatePersister) pathForDestination(destination permissions.PermissionUpdateDestination) string {
	switch destination {
	case permissions.PermissionUpdateDestinationUserSettings:
		return userSettingsPath()
	case permissions.PermissionUpdateDestinationProjectSettings:
		return projectSettingsPath(p.dir)
	case permissions.PermissionUpdateDestinationLocalSettings:
		return localSettingsPath(p.dir)
	default:
		return configPath(p.dir)
	}
}

func isLocalSettingsPath(path, dir string) bool {
	return filepath.Clean(path) == filepath.Clean(localSettingsPath(dir))
}

func ensureLocalSettingsGitignored(dir string) error {
	path := filepath.Join(settingsBaseDir(dir), ".gitignore")
	const entry = ".claude/settings.local.json"
	data, err := os.ReadFile(path)
	if err != nil && !os.IsNotExist(err) {
		return err
	}
	content := string(data)
	for _, line := range strings.Split(content, "\n") {
		if strings.TrimSpace(line) == entry {
			return nil
		}
	}
	if strings.TrimSpace(content) != "" && !strings.HasSuffix(content, "\n") {
		content += "\n"
	}
	content += entry + "\n"
	return os.WriteFile(path, []byte(content), 0o644)
}

func applyPermissionUpdateToFileConfig(fileCfg *fileConfig, update permissions.PermissionUpdate) bool {
	switch update.Type {
	case permissions.PermissionUpdateAddRules:
		rules := permissionUpdateRulesForConfig(update)
		if len(rules) > 0 {
			fileCfg.Permissions.Rules = append(fileCfg.Permissions.Rules, rules...)
			return true
		}
	case permissions.PermissionUpdateReplaceRules:
		rules := permissionUpdateRulesForConfig(update)
		kept := rulesWithoutPersistentBehavior(fileCfg.Permissions.Rules, update)
		fileCfg.Permissions.Rules = append(kept, rules...)
		return true
	case permissions.PermissionUpdateRemoveRules:
		remove := permissionUpdateRulesForConfig(update)
		fileCfg.Permissions.Rules = removePersistentRules(fileCfg.Permissions.Rules, remove, update)
		return true
	case permissions.PermissionUpdateSetMode:
		if update.Mode != "" {
			fileCfg.Permissions.Mode = string(update.Mode)
			return true
		}
	case permissions.PermissionUpdateAddDirectories:
		fileCfg.Permissions.WorkspaceRoots = appendUniqueConfigStrings(fileCfg.Permissions.WorkspaceRoots, update.Directories...)
		return true
	case permissions.PermissionUpdateRemoveDirectories:
		fileCfg.Permissions.WorkspaceRoots = removeConfigStrings(fileCfg.Permissions.WorkspaceRoots, update.Directories...)
		return true
	}
	return false
}

func rulesWithoutPersistentBehavior(rules []permissions.Rule, update permissions.PermissionUpdate) []permissions.Rule {
	out := make([]permissions.Rule, 0, len(rules))
	source := string(permissionUpdateConfigRuleSource(update.Destination))
	for _, rule := range rules {
		if rule.Source == source && rule.Action == update.Behavior {
			continue
		}
		out = append(out, rule)
	}
	return out
}

func removePersistentRules(existing, remove []permissions.Rule, update permissions.PermissionUpdate) []permissions.Rule {
	out := make([]permissions.Rule, 0, len(existing))
	source := string(permissionUpdateConfigRuleSource(update.Destination))
	for _, rule := range existing {
		if rule.Source == source && containsConfigRule(remove, rule) {
			continue
		}
		out = append(out, rule)
	}
	return out
}

func containsConfigRule(rules []permissions.Rule, candidate permissions.Rule) bool {
	for _, rule := range rules {
		if rule.ToolName != candidate.ToolName || rule.Action != candidate.Action {
			continue
		}
		if !sameConfigStrings(rule.Match.CommandContains, candidate.Match.CommandContains) {
			continue
		}
		if !sameConfigStrings(rule.Match.WorkDirPrefixes, candidate.Match.WorkDirPrefixes) {
			continue
		}
		return true
	}
	return false
}

func appendUniqueConfigStrings(existing []string, values ...string) []string {
	out := append([]string(nil), existing...)
	for _, value := range values {
		if value == "" || containsConfigString(out, value) {
			continue
		}
		out = append(out, value)
	}
	return out
}

func removeConfigStrings(existing []string, remove ...string) []string {
	out := make([]string, 0, len(existing))
	for _, value := range existing {
		if containsConfigString(remove, value) {
			continue
		}
		out = append(out, value)
	}
	return out
}

func containsConfigString(values []string, candidate string) bool {
	for _, value := range values {
		if value == candidate {
			return true
		}
	}
	return false
}

func sameConfigStrings(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for idx := range a {
		if a[idx] != b[idx] {
			return false
		}
	}
	return true
}

func readWritableFileConfig(path string) (fileConfig, error) {
	var fileCfg fileConfig
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return fileCfg, nil
		}
		return fileConfig{}, err
	}
	if len(data) == 0 {
		return fileCfg, nil
	}
	if err := json.Unmarshal(data, &fileCfg); err != nil {
		return fileConfig{}, err
	}
	return fileCfg, nil
}

func writeWritableFileConfig(path string, fileCfg fileConfig) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(fileCfg, "", "  ")
	if err != nil {
		return err
	}
	data = append(data, '\n')
	return os.WriteFile(path, data, 0o644)
}

func isPersistentPermissionDestination(destination permissions.PermissionUpdateDestination) bool {
	switch destination {
	case permissions.PermissionUpdateDestinationUserSettings,
		permissions.PermissionUpdateDestinationProjectSettings,
		permissions.PermissionUpdateDestinationLocalSettings:
		return true
	default:
		return false
	}
}

func permissionUpdateRulesForConfig(update permissions.PermissionUpdate) []permissions.Rule {
	if update.Behavior == "" {
		return nil
	}
	rules := make([]permissions.Rule, 0, len(update.Rules))
	for _, value := range update.Rules {
		if value.ToolName == "" {
			continue
		}
		rule := permissions.Rule{
			ToolName: value.ToolName,
			Action:   update.Behavior,
			Source:   string(permissionUpdateConfigRuleSource(update.Destination)),
		}
		if value.RuleContent != "" {
			rule.Match.CommandContains = []string{value.RuleContent}
		}
		rules = append(rules, rule)
	}
	return rules
}

func permissionUpdateConfigRuleSource(destination permissions.PermissionUpdateDestination) permissions.RuleSource {
	switch destination {
	case permissions.PermissionUpdateDestinationUserSettings:
		return permissions.RuleSourceConfig
	case permissions.PermissionUpdateDestinationProjectSettings:
		return permissions.RuleSourceProject
	case permissions.PermissionUpdateDestinationLocalSettings:
		return permissions.RuleSourceLocal
	default:
		return permissions.RuleSourceSession
	}
}
