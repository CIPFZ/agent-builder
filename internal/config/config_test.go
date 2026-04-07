package config

import (
	"os"
	"path/filepath"
	"testing"
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
