package config

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"myclaw/internal/permissions"
	"myclaw/internal/session"
)

func TestDefaultLoadsPermissionSettingsFromEnv(t *testing.T) {
	t.Setenv("MYCLAW_PERMISSION_MODE", "ask")
	t.Setenv("MYCLAW_PERMISSION_SUBAGENT_MODE", "workspace-write")
	t.Setenv("MYCLAW_PERMISSION_PLAN_MODE", "true")
	t.Setenv("MYCLAW_PERMISSION_AUTO_MODE", "false")
	t.Setenv("MYCLAW_PERMISSION_WORKSPACE_ROOTS", "/workspace/a;/workspace/b")
	t.Setenv("MYCLAW_PERMISSION_DANGEROUS_COMMANDS", "rm -rf,shutdown")

	cfg := Default()

	if cfg.Permissions.Mode != "ask" {
		t.Fatalf("permission mode = %q, want %q", cfg.Permissions.Mode, "ask")
	}
	if cfg.Permissions.SubagentMode != "workspace-write" {
		t.Fatalf("subagent permission mode = %q, want %q", cfg.Permissions.SubagentMode, "workspace-write")
	}
	if !cfg.Permissions.PlanMode {
		t.Fatal("plan mode = false, want true")
	}
	if cfg.Permissions.AutoMode {
		t.Fatal("auto mode = true, want false")
	}
	if len(cfg.Permissions.WorkspaceRoots) != 2 {
		t.Fatalf("workspace root count = %d, want 2", len(cfg.Permissions.WorkspaceRoots))
	}
	if len(cfg.Permissions.DangerousCommandPatterns) != 2 {
		t.Fatalf("dangerous command pattern count = %d, want 2", len(cfg.Permissions.DangerousCommandPatterns))
	}
}

func TestDefaultLoadsPermissionRulesFromEnvJSON(t *testing.T) {
	t.Setenv("MYCLAW_PERMISSION_RULES_JSON", `[{"tool_name":"system.run","action":"deny","match":{"command_contains":["rm -rf"]}},{"tool_name":"system.run","action":"allow","match":{"workdir_prefixes":["/workspace/safe"]}}]`)

	cfg := Default()

	if len(cfg.Permissions.Rules) != 2 {
		t.Fatalf("permission rule count = %d, want 2", len(cfg.Permissions.Rules))
	}
	if cfg.Permissions.Rules[0].ToolName != "system.run" {
		t.Fatalf("first rule tool = %q, want system.run", cfg.Permissions.Rules[0].ToolName)
	}
	if len(cfg.Permissions.Rules[1].Match.WorkDirPrefixes) != 1 {
		t.Fatalf("second rule workdir prefixes = %#v, want one prefix", cfg.Permissions.Rules[1].Match.WorkDirPrefixes)
	}
}

func TestLoadFromDirReadsLLMSettingsFromConfigFile(t *testing.T) {
	dir := t.TempDir()
	configsDir := filepath.Join(dir, "configs")
	if err := os.MkdirAll(configsDir, 0o755); err != nil {
		t.Fatalf("mkdir configs: %v", err)
	}
	raw := `{
  "llm": {
    "provider": "openai-compatible",
    "base_url": "https://example.test/v1/chat/completions",
    "api_key": "file-key",
    "model": "file-model"
  }
}`
	if err := os.WriteFile(filepath.Join(configsDir, "myclaw.json"), []byte(raw), 0o644); err != nil {
		t.Fatalf("write config: %v", err)
	}

	cfg := LoadFromDir(dir)

	if cfg.LLM.Provider != "openai-compatible" {
		t.Fatalf("provider = %q, want %q", cfg.LLM.Provider, "openai-compatible")
	}
	if cfg.LLM.BaseURL != "https://example.test/v1/chat/completions" {
		t.Fatalf("base_url = %q", cfg.LLM.BaseURL)
	}
	if cfg.LLM.APIKey != "file-key" {
		t.Fatalf("api_key = %q", cfg.LLM.APIKey)
	}
	if cfg.LLM.Model != "file-model" {
		t.Fatalf("model = %q", cfg.LLM.Model)
	}
}

func TestLoadFromDirInterpolatesEnvVarsInConfigFile(t *testing.T) {
	dir := t.TempDir()
	configsDir := filepath.Join(dir, "configs")
	if err := os.MkdirAll(configsDir, 0o755); err != nil {
		t.Fatalf("mkdir configs: %v", err)
	}
	t.Setenv("MY_REAL_KEY", "env-key")
	raw := `{
  "llm": {
    "api_key": "${MY_REAL_KEY}"
  }
}`
	if err := os.WriteFile(filepath.Join(configsDir, "myclaw.json"), []byte(raw), 0o644); err != nil {
		t.Fatalf("write config: %v", err)
	}

	cfg := LoadFromDir(dir)

	if cfg.LLM.APIKey != "env-key" {
		t.Fatalf("api_key = %q, want %q", cfg.LLM.APIKey, "env-key")
	}
}

func TestLoadFromDirEnvOverridesConfigFile(t *testing.T) {
	dir := t.TempDir()
	configsDir := filepath.Join(dir, "configs")
	if err := os.MkdirAll(configsDir, 0o755); err != nil {
		t.Fatalf("mkdir configs: %v", err)
	}
	raw := `{
  "llm": {
    "base_url": "https://example.test/v1/chat/completions",
    "api_key": "file-key",
    "model": "file-model"
  }
}`
	if err := os.WriteFile(filepath.Join(configsDir, "myclaw.json"), []byte(raw), 0o644); err != nil {
		t.Fatalf("write config: %v", err)
	}

	t.Setenv("MYCLAW_LLM_API_KEY", "env-key")
	t.Setenv("MYCLAW_LLM_MODEL", "env-model")

	cfg := LoadFromDir(dir)

	if cfg.LLM.APIKey != "env-key" {
		t.Fatalf("api_key = %q, want env override", cfg.LLM.APIKey)
	}
	if cfg.LLM.Model != "env-model" {
		t.Fatalf("model = %q, want env override", cfg.LLM.Model)
	}
	if cfg.LLM.BaseURL != "https://example.test/v1/chat/completions" {
		t.Fatalf("base_url = %q, want file value", cfg.LLM.BaseURL)
	}
}

func TestLoadFromDirUsesAnthropicModelEnvWhenMyclawModelUnset(t *testing.T) {
	dir := t.TempDir()
	configsDir := filepath.Join(dir, "configs")
	if err := os.MkdirAll(configsDir, 0o755); err != nil {
		t.Fatalf("mkdir configs: %v", err)
	}
	raw := `{
  "llm": {
    "model": "file-model"
  }
}`
	if err := os.WriteFile(filepath.Join(configsDir, "myclaw.json"), []byte(raw), 0o644); err != nil {
		t.Fatalf("write config: %v", err)
	}

	t.Setenv("ANTHROPIC_MODEL", "anthropic-env-model")

	cfg := LoadFromDir(dir)

	if cfg.LLM.Model != "anthropic-env-model" {
		t.Fatalf("model = %q, want ANTHROPIC_MODEL override", cfg.LLM.Model)
	}
}

func TestLoadFromDirPrefersMyclawModelEnvOverAnthropicModel(t *testing.T) {
	t.Setenv("MYCLAW_LLM_MODEL", "myclaw-env-model")
	t.Setenv("ANTHROPIC_MODEL", "anthropic-env-model")

	cfg := Default()

	if cfg.LLM.Model != "myclaw-env-model" {
		t.Fatalf("model = %q, want MYCLAW_LLM_MODEL to win", cfg.LLM.Model)
	}
}

func TestLoadFromDirReadsPermissionRunModesFromConfigFile(t *testing.T) {
	dir := t.TempDir()
	configsDir := filepath.Join(dir, "configs")
	if err := os.MkdirAll(configsDir, 0o755); err != nil {
		t.Fatalf("mkdir configs: %v", err)
	}
	raw := `{
  "permissions": {
    "mode": "workspace-write",
    "plan_mode": true,
    "auto_mode": false,
    "workspace_roots": ["C:/repo"]
  }
}`
	if err := os.WriteFile(filepath.Join(configsDir, "myclaw.json"), []byte(raw), 0o644); err != nil {
		t.Fatalf("write config: %v", err)
	}

	cfg := LoadFromDir(dir)

	if !cfg.Permissions.PlanMode {
		t.Fatal("plan mode = false, want true")
	}
	if cfg.Permissions.AutoMode {
		t.Fatal("auto mode = true, want false")
	}
}

func TestLoadFromDirResolvesRelativePermissionWorkspaceRootsAndRulePrefixes(t *testing.T) {
	dir := t.TempDir()
	configsDir := filepath.Join(dir, "configs")
	if err := os.MkdirAll(configsDir, 0o755); err != nil {
		t.Fatalf("mkdir configs: %v", err)
	}
	raw := `{
  "permissions": {
    "mode": "workspace-write",
    "workspace_roots": ["."],
    "rules": [
      {
        "tool_name": "system.run",
        "action": "allow",
        "match": {
          "workdir_prefixes": ["scripts"]
        }
      }
    ]
  }
}`
	if err := os.WriteFile(filepath.Join(configsDir, "myclaw.json"), []byte(raw), 0o644); err != nil {
		t.Fatalf("write config: %v", err)
	}

	cfg := LoadFromDir(dir)

	if len(cfg.Permissions.WorkspaceRoots) != 1 {
		t.Fatalf("workspace roots = %#v, want one resolved root", cfg.Permissions.WorkspaceRoots)
	}
	if cfg.Permissions.WorkspaceRoots[0] != dir {
		t.Fatalf("workspace root = %q, want %q", cfg.Permissions.WorkspaceRoots[0], dir)
	}
	if len(cfg.Permissions.Rules) != 1 {
		t.Fatalf("rules = %#v, want one rule", cfg.Permissions.Rules)
	}
	wantPrefix := filepath.Join(dir, "scripts")
	if got := cfg.Permissions.Rules[0].Match.WorkDirPrefixes[0]; got != wantPrefix {
		t.Fatalf("rule workdir prefix = %q, want %q", got, wantPrefix)
	}
}

func TestLoadFromDirReadsCompactVerificationModeFromConfigFile(t *testing.T) {
	dir := t.TempDir()
	configsDir := filepath.Join(dir, "configs")
	if err := os.MkdirAll(configsDir, 0o755); err != nil {
		t.Fatalf("mkdir configs: %v", err)
	}
	raw := `{
  "compact": {
    "verification_mode": true
  }
}`
	if err := os.WriteFile(filepath.Join(configsDir, "myclaw.json"), []byte(raw), 0o644); err != nil {
		t.Fatalf("write config: %v", err)
	}

	cfg := LoadFromDir(dir)

	if !cfg.Compact.VerificationMode {
		t.Fatal("compact verification mode = false, want true")
	}
}

func TestPermissionUpdatePersisterAddsUserSettingRuleToConfigFile(t *testing.T) {
	dir := t.TempDir()
	userSettings := filepath.Join(dir, "user-settings.json")
	t.Setenv("MYCLAW_USER_SETTINGS_FILE", userSettings)
	persister := NewPermissionUpdatePersister(dir)

	if err := persister.PersistPermissionUpdates(context.Background(), session.Session{ID: "main-1"}, []permissions.PermissionUpdate{{
		Type:        permissions.PermissionUpdateAddRules,
		Destination: permissions.PermissionUpdateDestinationUserSettings,
		Behavior:    permissions.ActionAllow,
		Rules: []permissions.PermissionRuleValue{{
			ToolName:    "system.run",
			RuleContent: "go test",
		}},
	}}); err != nil {
		t.Fatalf("persist permission update: %v", err)
	}

	data, err := os.ReadFile(userSettings)
	if err != nil {
		t.Fatalf("read config: %v", err)
	}
	var decoded struct {
		Permissions struct {
			Rules []permissions.Rule `json:"rules"`
		} `json:"permissions"`
	}
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("decode config: %v", err)
	}
	if len(decoded.Permissions.Rules) != 1 {
		t.Fatalf("rules = %#v, want one persisted rule", decoded.Permissions.Rules)
	}
	rule := decoded.Permissions.Rules[0]
	if rule.ToolName != "system.run" || rule.Action != permissions.ActionAllow || rule.Source != string(permissions.RuleSourceConfig) {
		t.Fatalf("rule = %#v, want userSettings allow rule", rule)
	}
	if len(rule.Match.CommandContains) != 1 || rule.Match.CommandContains[0] != "go test" {
		t.Fatalf("rule match = %#v, want command content", rule.Match)
	}
}

func TestPermissionUpdatePersisterWritesProjectAndLocalSettingsSeparately(t *testing.T) {
	dir := t.TempDir()
	userSettings := filepath.Join(dir, "user-settings.json")
	t.Setenv("MYCLAW_USER_SETTINGS_FILE", userSettings)
	persister := NewPermissionUpdatePersister(dir)

	if err := persister.PersistPermissionUpdates(context.Background(), session.Session{ID: "main-1"}, []permissions.PermissionUpdate{
		{
			Type:        permissions.PermissionUpdateSetMode,
			Destination: permissions.PermissionUpdateDestinationUserSettings,
			Mode:        permissions.ModeAsk,
		},
		{
			Type:        permissions.PermissionUpdateSetMode,
			Destination: permissions.PermissionUpdateDestinationProjectSettings,
			Mode:        permissions.ModeWorkspaceWrite,
		},
		{
			Type:        permissions.PermissionUpdateAddRules,
			Destination: permissions.PermissionUpdateDestinationLocalSettings,
			Behavior:    permissions.ActionDeny,
			Rules: []permissions.PermissionRuleValue{{
				ToolName: "system.run",
			}},
		},
	}); err != nil {
		t.Fatalf("persist permission updates: %v", err)
	}

	if _, err := os.Stat(userSettings); err != nil {
		t.Fatalf("user settings not written: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, ".claude", "settings.json")); err != nil {
		t.Fatalf("project settings not written: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, ".claude", "settings.local.json")); err != nil {
		t.Fatalf("local settings not written: %v", err)
	}
	gitignore, err := os.ReadFile(filepath.Join(dir, ".gitignore"))
	if err != nil {
		t.Fatalf("read gitignore: %v", err)
	}
	if !strings.Contains(string(gitignore), ".claude/settings.local.json") {
		t.Fatalf("gitignore = %q, want local settings entry", string(gitignore))
	}
}

func TestLoadFromDirMergesClaudeStyleSettingsSourcesInPrecedenceOrder(t *testing.T) {
	dir := t.TempDir()
	userSettings := filepath.Join(dir, "user-settings.json")
	t.Setenv("MYCLAW_USER_SETTINGS_FILE", userSettings)
	if err := os.WriteFile(userSettings, []byte(`{
  "permissions": {
    "mode": "ask",
    "rules": [{"tool_name":"text.upper","source":"config","action":"allow"}]
  }
}`), 0o644); err != nil {
		t.Fatalf("write user settings: %v", err)
	}
	projectDir := filepath.Join(dir, ".claude")
	if err := os.MkdirAll(projectDir, 0o755); err != nil {
		t.Fatalf("mkdir project settings: %v", err)
	}
	if err := os.WriteFile(filepath.Join(projectDir, "settings.json"), []byte(`{
  "permissions": {
    "mode": "workspace-write",
    "workspace_roots": ["."],
    "rules": [{"tool_name":"system.run","source":"project","action":"allow"}]
  }
}`), 0o644); err != nil {
		t.Fatalf("write project settings: %v", err)
	}
	if err := os.WriteFile(filepath.Join(projectDir, "settings.local.json"), []byte(`{
  "permissions": {
    "mode": "danger-full-access",
    "rules": [{"tool_name":"agent.task","source":"local","action":"deny"}]
  }
}`), 0o644); err != nil {
		t.Fatalf("write local settings: %v", err)
	}

	cfg := LoadFromDir(dir)

	if cfg.Permissions.Mode != string(permissions.ModeDangerFullAccess) {
		t.Fatalf("mode = %q, want local settings override", cfg.Permissions.Mode)
	}
	if len(cfg.Permissions.Rules) != 3 {
		t.Fatalf("rules = %#v, want merged rules from user, project, local", cfg.Permissions.Rules)
	}
}

func TestPermissionUpdatePersisterReplacesAndRemovesRules(t *testing.T) {
	dir := t.TempDir()
	userSettings := filepath.Join(dir, "user-settings.json")
	t.Setenv("MYCLAW_USER_SETTINGS_FILE", userSettings)
	raw := `{
  "permissions": {
    "rules": [
      {"tool_name":"system.run","source":"config","action":"allow","match":{"command_contains":["old"]}},
      {"tool_name":"text.upper","source":"config","action":"deny"}
    ]
  }
}`
	if err := os.WriteFile(userSettings, []byte(raw), 0o644); err != nil {
		t.Fatalf("write config: %v", err)
	}

	persister := NewPermissionUpdatePersister(dir)
	if err := persister.PersistPermissionUpdates(context.Background(), session.Session{ID: "main-1"}, []permissions.PermissionUpdate{
		{
			Type:        permissions.PermissionUpdateReplaceRules,
			Destination: permissions.PermissionUpdateDestinationUserSettings,
			Behavior:    permissions.ActionAllow,
			Rules: []permissions.PermissionRuleValue{{
				ToolName:    "system.run",
				RuleContent: "new",
			}},
		},
		{
			Type:        permissions.PermissionUpdateRemoveRules,
			Destination: permissions.PermissionUpdateDestinationUserSettings,
			Behavior:    permissions.ActionDeny,
			Rules: []permissions.PermissionRuleValue{{
				ToolName: "text.upper",
			}},
		},
	}); err != nil {
		t.Fatalf("persist permission updates: %v", err)
	}

	cfg := LoadFromDir(dir)
	if len(cfg.Permissions.Rules) != 1 {
		t.Fatalf("rules = %#v, want only replacement allow rule", cfg.Permissions.Rules)
	}
	rule := cfg.Permissions.Rules[0]
	if rule.ToolName != "system.run" || rule.Action != permissions.ActionAllow {
		t.Fatalf("rule = %#v, want replacement allow rule", rule)
	}
	if len(rule.Match.CommandContains) != 1 || rule.Match.CommandContains[0] != "new" {
		t.Fatalf("rule match = %#v, want new rule content", rule.Match)
	}
}

func TestPermissionUpdatePersisterPersistsModeAndDirectories(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("MYCLAW_USER_SETTINGS_FILE", filepath.Join(dir, "user-settings.json"))
	persister := NewPermissionUpdatePersister(dir)

	if err := persister.PersistPermissionUpdates(context.Background(), session.Session{ID: "main-1"}, []permissions.PermissionUpdate{
		{
			Type:        permissions.PermissionUpdateSetMode,
			Destination: permissions.PermissionUpdateDestinationProjectSettings,
			Mode:        permissions.ModeAsk,
		},
		{
			Type:        permissions.PermissionUpdateAddDirectories,
			Destination: permissions.PermissionUpdateDestinationProjectSettings,
			Directories: []string{"C:/repo", "C:/repo/tools"},
		},
		{
			Type:        permissions.PermissionUpdateRemoveDirectories,
			Destination: permissions.PermissionUpdateDestinationProjectSettings,
			Directories: []string{"C:/repo/tools"},
		},
	}); err != nil {
		t.Fatalf("persist permission updates: %v", err)
	}

	cfg := LoadFromDir(dir)
	if cfg.Permissions.Mode != string(permissions.ModeAsk) {
		t.Fatalf("mode = %q, want ask", cfg.Permissions.Mode)
	}
	if len(cfg.Permissions.WorkspaceRoots) != 1 || filepath.ToSlash(cfg.Permissions.WorkspaceRoots[0]) != "C:/repo" {
		t.Fatalf("workspace roots = %#v, want only C:/repo", cfg.Permissions.WorkspaceRoots)
	}
}
