package config

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"myclaw/internal/permissions"
	"myclaw/internal/session"
)

func TestLoadFromDirLoadsSingleConfigFileAndIgnoresClaudeSettings(t *testing.T) {
	dir := t.TempDir()
	writeTestConfig(t, dir, `{
  "config": {"version": 1},
  "server": {
    "http_addr": "127.0.0.1:19090",
    "ws_path": "/runtime"
  },
  "llm": {
    "active_profile": "claude-main",
    "providers": {
      "anthropic": {
        "protocol": "anthropic",
        "base_url": "https://anthropic.example/v1/messages",
        "api_key": "file-key",
        "enabled": true
      }
    },
    "profiles": {
      "claude-main": {
        "provider": "anthropic",
        "model": "claude-sonnet-4-5",
        "streaming": true
      }
    },
    "routing": {
      "default_profile": "claude-main",
      "agent_profiles": {
        "review": "claude-main"
      }
    }
  },
  "permissions": {
    "mode": "workspace-write",
    "subagent_mode": "ask",
    "workspace_roots": ["."],
    "dangerous_command_patterns": ["rm -rf"]
  },
  "mcp": {
    "enabled": true,
    "skills_enabled": true
  },
  "compact": {
    "verification_mode": true
  }
}`)

	claudeDir := filepath.Join(dir, ".claude")
	if err := os.MkdirAll(claudeDir, 0o755); err != nil {
		t.Fatalf("mkdir .claude: %v", err)
	}
	if err := os.WriteFile(filepath.Join(claudeDir, "settings.json"), []byte(`{
  "permissions": {
    "mode": "danger-full-access"
  }
}`), 0o644); err != nil {
		t.Fatalf("write claude settings: %v", err)
	}
	t.Setenv("MYCLAW_USER_SETTINGS_FILE", filepath.Join(dir, "ignored-user-settings.json"))
	if err := os.WriteFile(filepath.Join(dir, "ignored-user-settings.json"), []byte(`{
  "permissions": {
    "mode": "ask"
  }
}`), 0o644); err != nil {
		t.Fatalf("write ignored user settings: %v", err)
	}

	cfg := LoadFromDir(dir)

	if cfg.HTTPAddr != "127.0.0.1:19090" {
		t.Fatalf("http addr = %q, want file value", cfg.HTTPAddr)
	}
	if cfg.WSPath != "/runtime" {
		t.Fatalf("ws path = %q, want file value", cfg.WSPath)
	}
	if cfg.Permissions.Mode != "workspace-write" {
		t.Fatalf("permission mode = %q, want myclaw.json value", cfg.Permissions.Mode)
	}
	if cfg.LLM.ActiveProfile != "claude-main" {
		t.Fatalf("active profile = %q, want claude-main", cfg.LLM.ActiveProfile)
	}
	if cfg.LLM.Provider != "anthropic" {
		t.Fatalf("resolved provider = %q, want anthropic", cfg.LLM.Provider)
	}
	if cfg.LLM.BaseURL != "https://anthropic.example/v1/messages" {
		t.Fatalf("resolved base_url = %q", cfg.LLM.BaseURL)
	}
	if cfg.LLM.APIKey != "file-key" {
		t.Fatalf("resolved api_key = %q", cfg.LLM.APIKey)
	}
	if cfg.LLM.Model != "claude-sonnet-4-5" {
		t.Fatalf("resolved model = %q", cfg.LLM.Model)
	}
	if !cfg.Compact.VerificationMode {
		t.Fatal("verification mode = false, want true")
	}
	if !cfg.MCP.Enabled || !cfg.MCP.Skills {
		t.Fatalf("mcp config = %#v, want enabled skills", cfg.MCP)
	}
}

func TestLoadFromDirExpandsEnvAndAppliesProviderAndProfileOverrides(t *testing.T) {
	dir := t.TempDir()
	writeTestConfig(t, dir, `{
  "config": {"version": 1},
  "llm": {
    "active_profile": "claude-main",
    "providers": {
      "anthropic": {
        "protocol": "anthropic",
        "base_url": "https://anthropic.example/v1/messages",
        "api_key": "${ANTHROPIC_API_KEY}",
        "enabled": true
      },
      "openai": {
        "protocol": "openai-compatible",
        "base_url": "https://openai.example/v1/chat/completions",
        "api_key": "file-openai-key",
        "enabled": true
      }
    },
    "profiles": {
      "claude-main": {
        "provider": "anthropic",
        "model": "claude-sonnet-4-5"
      },
      "review-gpt": {
        "provider": "openai",
        "model": "gpt-5.1"
      }
    },
    "routing": {
      "default_profile": "claude-main",
      "agent_profiles": {
        "review": "review-gpt"
      }
    }
  }
}`)

	t.Setenv("ANTHROPIC_API_KEY", "expanded-anthropic-key")
	t.Setenv("MYCLAW_LLM_ACTIVE_PROFILE", "review-gpt")
	t.Setenv("MYCLAW_LLM_PROVIDERS__OPENAI__API_KEY", "env-openai-key")
	t.Setenv("MYCLAW_LLM_PROVIDERS__OPENAI__BASE_URL", "https://override.example/v1/chat/completions")
	t.Setenv("MYCLAW_LLM_PROFILES__REVIEW-GPT__MODEL", "gpt-5.2")
	t.Setenv("MYCLAW_LLM_ROUTING__AGENT_PROFILES__FRONTEND", "review-gpt")

	cfg := LoadFromDir(dir)

	if cfg.LLM.Providers["anthropic"].APIKey != "expanded-anthropic-key" {
		t.Fatalf("anthropic api key = %q, want expanded env value", cfg.LLM.Providers["anthropic"].APIKey)
	}
	if cfg.LLM.ActiveProfile != "review-gpt" {
		t.Fatalf("active profile = %q, want env override", cfg.LLM.ActiveProfile)
	}
	if cfg.LLM.APIKey != "env-openai-key" {
		t.Fatalf("resolved api key = %q, want provider env override", cfg.LLM.APIKey)
	}
	if cfg.LLM.BaseURL != "https://override.example/v1/chat/completions" {
		t.Fatalf("resolved base_url = %q, want provider env override", cfg.LLM.BaseURL)
	}
	if cfg.LLM.Model != "gpt-5.2" {
		t.Fatalf("resolved model = %q, want profile env override", cfg.LLM.Model)
	}
	if cfg.LLM.Routing.AgentProfiles["frontend"] != "review-gpt" {
		t.Fatalf("frontend route = %q, want env override", cfg.LLM.Routing.AgentProfiles["frontend"])
	}
}

func TestLoadFromDirResolvesRelativePermissionPaths(t *testing.T) {
	dir := t.TempDir()
	writeTestConfig(t, dir, `{
  "config": {"version": 1},
  "llm": {
    "active_profile": "claude-main",
    "providers": {
      "anthropic": {
        "protocol": "anthropic",
        "base_url": "https://anthropic.example/v1/messages",
        "api_key": "file-key",
        "enabled": true
      }
    },
    "profiles": {
      "claude-main": {
        "provider": "anthropic",
        "model": "claude-sonnet-4-5"
      }
    }
  },
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
}`)

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

func TestLoadFromDirPanicsOnInvalidRoutingProfile(t *testing.T) {
	dir := t.TempDir()
	writeTestConfig(t, dir, `{
  "config": {"version": 1},
  "llm": {
    "active_profile": "claude-main",
    "providers": {
      "anthropic": {
        "protocol": "anthropic",
        "base_url": "https://anthropic.example/v1/messages",
        "api_key": "file-key",
        "enabled": true
      }
    },
    "profiles": {
      "claude-main": {
        "provider": "anthropic",
        "model": "claude-sonnet-4-5"
      }
    },
    "routing": {
      "default_profile": "missing-profile"
    }
  }
}`)

	defer func() {
		if recover() == nil {
			t.Fatal("LoadFromDir did not panic on invalid routing profile")
		}
	}()

	_ = LoadFromDir(dir)
}

func TestPermissionUpdatePersisterWritesToConfiguredMyclawJSON(t *testing.T) {
	dir := t.TempDir()
	configPath := filepath.Join(dir, "configs", "myclaw.json")
	t.Setenv("MYCLAW_CONFIG_FILE", configPath)
	writeTestConfig(t, dir, `{
  "config": {"version": 1},
  "llm": {
    "active_profile": "claude-main",
    "providers": {
      "anthropic": {
        "protocol": "anthropic",
        "base_url": "https://anthropic.example/v1/messages",
        "api_key": "file-key",
        "enabled": true
      }
    },
    "profiles": {
      "claude-main": {
        "provider": "anthropic",
        "model": "claude-sonnet-4-5"
      }
    }
  },
  "permissions": {
    "mode": "workspace-write"
  }
}`)

	persister := NewPermissionUpdatePersister(dir)
	err := persister.PersistPermissionUpdates(context.Background(), session.Session{ID: "main-1"}, []permissions.PermissionUpdate{
		{
			Type:        permissions.PermissionUpdateSetMode,
			Destination: permissions.PermissionUpdateDestinationUserSettings,
			Mode:        permissions.ModeAsk,
		},
		{
			Type:        permissions.PermissionUpdateAddRules,
			Destination: permissions.PermissionUpdateDestinationUserSettings,
			Behavior:    permissions.ActionAllow,
			Rules: []permissions.PermissionRuleValue{{
				ToolName:    "system.run",
				RuleContent: "go test ./...",
			}},
		},
	})
	if err != nil {
		t.Fatalf("persist permission updates: %v", err)
	}

	cfg := LoadFromDir(dir)
	if cfg.Permissions.Mode != string(permissions.ModeAsk) {
		t.Fatalf("mode = %q, want ask", cfg.Permissions.Mode)
	}
	if len(cfg.Permissions.Rules) != 1 {
		t.Fatalf("rules = %#v, want one persisted rule", cfg.Permissions.Rules)
	}
	if got := cfg.Permissions.Rules[0].Match.CommandContains[0]; got != "go test ./..." {
		t.Fatalf("command match = %q, want persisted command", got)
	}
}

func writeTestConfig(t *testing.T, dir, raw string) {
	t.Helper()
	configDir := filepath.Join(dir, "configs")
	if err := os.MkdirAll(configDir, 0o755); err != nil {
		t.Fatalf("mkdir configs: %v", err)
	}
	if err := os.WriteFile(filepath.Join(configDir, "myclaw.json"), []byte(raw), 0o644); err != nil {
		t.Fatalf("write config: %v", err)
	}
}
