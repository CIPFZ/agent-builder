package config

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"

	"myclaw/internal/permissions"
)

type Config struct {
	HTTPAddr    string
	WSPath      string
	LLM         LLMConfig
	Permissions PermissionConfig
	Compact     CompactConfig
}

type LLMConfig struct {
	Provider string
	BaseURL  string
	APIKey   string
	Model    string
}

type PermissionConfig struct {
	Mode                     string
	SubagentMode             string
	PlanMode                 bool
	AutoMode                 bool
	WorkspaceRoots           []string
	Rules                    []permissions.Rule
	DangerousCommandPatterns []string
}

type CompactConfig struct {
	VerificationMode bool
}

func Default() Config {
	return LoadFromDir(".")
}

func LoadFromDir(dir string) Config {
	cfg := defaultConfig()
	mergeFileConfig(&cfg, dir)
	applyEnvOverrides(&cfg)
	resolvePermissionPaths(&cfg, dir)
	return cfg
}

func defaultConfig() Config {
	return Config{
		HTTPAddr: "127.0.0.1:18080",
		WSPath:   "/ws",
		LLM: LLMConfig{
			Provider: "openai-compatible",
			BaseURL:  envOrDefault("MYCLAW_LLM_BASE_URL", "https://api.longcat.chat/openai/v1/chat/completions"),
			APIKey:   os.Getenv("MYCLAW_LLM_API_KEY"),
			Model:    envOrDefault("MYCLAW_LLM_MODEL", "LongCat-Flash-Chat"),
		},
		Permissions: PermissionConfig{
			Mode:                     envOrDefault("MYCLAW_PERMISSION_MODE", "workspace-write"),
			SubagentMode:             os.Getenv("MYCLAW_PERMISSION_SUBAGENT_MODE"),
			PlanMode:                 envBool("MYCLAW_PERMISSION_PLAN_MODE", false),
			AutoMode:                 envBool("MYCLAW_PERMISSION_AUTO_MODE", false),
			WorkspaceRoots:           splitList(os.Getenv("MYCLAW_PERMISSION_WORKSPACE_ROOTS")),
			Rules:                    parsePermissionRules(os.Getenv("MYCLAW_PERMISSION_RULES_JSON")),
			DangerousCommandPatterns: splitList(envOrDefault("MYCLAW_PERMISSION_DANGEROUS_COMMANDS", "rm -rf,del /f /s,format,shutdown,reboot,powershell -enc")),
		},
		Compact: CompactConfig{
			VerificationMode: envBool("MYCLAW_COMPACT_VERIFICATION_MODE", false),
		},
	}
}

type fileConfig struct {
	HTTPAddr    string               `json:"http_addr"`
	WSPath      string               `json:"ws_path"`
	LLM         fileLLMConfig        `json:"llm"`
	Permissions filePermissionConfig `json:"permissions"`
	Compact     fileCompactConfig    `json:"compact"`
}

type fileLLMConfig struct {
	Provider string `json:"provider"`
	BaseURL  string `json:"base_url"`
	APIKey   string `json:"api_key"`
	Model    string `json:"model"`
}

type filePermissionConfig struct {
	Mode                     string             `json:"mode"`
	SubagentMode             string             `json:"subagent_mode"`
	PlanMode                 *bool              `json:"plan_mode"`
	AutoMode                 *bool              `json:"auto_mode"`
	WorkspaceRoots           []string           `json:"workspace_roots"`
	Rules                    []permissions.Rule `json:"rules"`
	DangerousCommandPatterns []string           `json:"dangerous_command_patterns"`
}

type fileCompactConfig struct {
	VerificationMode *bool `json:"verification_mode"`
}

func mergeFileConfig(cfg *Config, dir string) {
	path := configPath(dir)
	data, err := os.ReadFile(path)
	if err != nil {
		return
	}

	var fileCfg fileConfig
	if err := json.Unmarshal(data, &fileCfg); err != nil {
		return
	}

	if fileCfg.HTTPAddr != "" {
		cfg.HTTPAddr = expandEnv(fileCfg.HTTPAddr)
	}
	if fileCfg.WSPath != "" {
		cfg.WSPath = expandEnv(fileCfg.WSPath)
	}
	if fileCfg.LLM.Provider != "" {
		cfg.LLM.Provider = expandEnv(fileCfg.LLM.Provider)
	}
	if fileCfg.LLM.BaseURL != "" {
		cfg.LLM.BaseURL = expandEnv(fileCfg.LLM.BaseURL)
	}
	if fileCfg.LLM.APIKey != "" {
		cfg.LLM.APIKey = expandEnv(fileCfg.LLM.APIKey)
	}
	if fileCfg.LLM.Model != "" {
		cfg.LLM.Model = expandEnv(fileCfg.LLM.Model)
	}
	if fileCfg.Permissions.Mode != "" {
		cfg.Permissions.Mode = expandEnv(fileCfg.Permissions.Mode)
	}
	if fileCfg.Permissions.SubagentMode != "" {
		cfg.Permissions.SubagentMode = expandEnv(fileCfg.Permissions.SubagentMode)
	}
	if fileCfg.Permissions.PlanMode != nil {
		cfg.Permissions.PlanMode = *fileCfg.Permissions.PlanMode
	}
	if fileCfg.Permissions.AutoMode != nil {
		cfg.Permissions.AutoMode = *fileCfg.Permissions.AutoMode
	}
	if len(fileCfg.Permissions.WorkspaceRoots) > 0 {
		cfg.Permissions.WorkspaceRoots = expandEnvList(fileCfg.Permissions.WorkspaceRoots)
	}
	if len(fileCfg.Permissions.Rules) > 0 {
		cfg.Permissions.Rules = fileCfg.Permissions.Rules
	}
	if len(fileCfg.Permissions.DangerousCommandPatterns) > 0 {
		cfg.Permissions.DangerousCommandPatterns = expandEnvList(fileCfg.Permissions.DangerousCommandPatterns)
	}
	if fileCfg.Compact.VerificationMode != nil {
		cfg.Compact.VerificationMode = *fileCfg.Compact.VerificationMode
	}
}

func applyEnvOverrides(cfg *Config) {
	cfg.HTTPAddr = envOrDefault("MYCLAW_HTTP_ADDR", cfg.HTTPAddr)
	cfg.WSPath = envOrDefault("MYCLAW_WS_PATH", cfg.WSPath)
	cfg.LLM.Provider = envOrDefault("MYCLAW_LLM_PROVIDER", cfg.LLM.Provider)
	cfg.LLM.BaseURL = envOrDefault("MYCLAW_LLM_BASE_URL", cfg.LLM.BaseURL)
	if value := os.Getenv("MYCLAW_LLM_API_KEY"); value != "" {
		cfg.LLM.APIKey = value
	}
	cfg.LLM.Model = envOrDefault("MYCLAW_LLM_MODEL", cfg.LLM.Model)
	cfg.Permissions.Mode = envOrDefault("MYCLAW_PERMISSION_MODE", cfg.Permissions.Mode)
	if value := os.Getenv("MYCLAW_PERMISSION_SUBAGENT_MODE"); value != "" {
		cfg.Permissions.SubagentMode = value
	}
	cfg.Permissions.PlanMode = envBool("MYCLAW_PERMISSION_PLAN_MODE", cfg.Permissions.PlanMode)
	cfg.Permissions.AutoMode = envBool("MYCLAW_PERMISSION_AUTO_MODE", cfg.Permissions.AutoMode)
	if value := splitList(os.Getenv("MYCLAW_PERMISSION_WORKSPACE_ROOTS")); len(value) > 0 {
		cfg.Permissions.WorkspaceRoots = value
	}
	if value := parsePermissionRules(os.Getenv("MYCLAW_PERMISSION_RULES_JSON")); len(value) > 0 {
		cfg.Permissions.Rules = value
	}
	if value := splitList(os.Getenv("MYCLAW_PERMISSION_DANGEROUS_COMMANDS")); len(value) > 0 {
		cfg.Permissions.DangerousCommandPatterns = value
	}
	cfg.Compact.VerificationMode = envBool("MYCLAW_COMPACT_VERIFICATION_MODE", cfg.Compact.VerificationMode)
}

func configPath(dir string) string {
	if override := os.Getenv("MYCLAW_CONFIG_FILE"); override != "" {
		return override
	}
	base := dir
	if strings.TrimSpace(base) == "" {
		base = "."
	}
	return filepath.Join(base, "configs", "myclaw.json")
}

func expandEnv(value string) string {
	return os.ExpandEnv(value)
}

func expandEnvList(values []string) []string {
	items := make([]string, 0, len(values))
	for _, value := range values {
		items = append(items, expandEnv(value))
	}
	return items
}

func envOrDefault(key, fallback string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return fallback
}

func envBool(key string, fallback bool) bool {
	value, ok := os.LookupEnv(key)
	if !ok || strings.TrimSpace(value) == "" {
		return fallback
	}
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "1", "true", "yes", "on":
		return true
	case "0", "false", "no", "off":
		return false
	default:
		return fallback
	}
}

func splitList(raw string) []string {
	if raw == "" {
		return nil
	}
	fields := strings.FieldsFunc(raw, func(r rune) bool {
		return r == ';' || r == ','
	})
	items := make([]string, 0, len(fields))
	for _, field := range fields {
		value := strings.TrimSpace(field)
		if value == "" {
			continue
		}
		items = append(items, value)
	}
	return items
}

func parsePermissionRules(raw string) []permissions.Rule {
	if strings.TrimSpace(raw) == "" {
		return nil
	}
	var rules []permissions.Rule
	if err := json.Unmarshal([]byte(raw), &rules); err != nil {
		return nil
	}
	return rules
}

func resolvePermissionPaths(cfg *Config, dir string) {
	base := dir
	if strings.TrimSpace(base) == "" {
		base = "."
	}
	if abs, err := filepath.Abs(base); err == nil {
		base = abs
	}
	cfg.Permissions.WorkspaceRoots = resolveRelativePaths(base, cfg.Permissions.WorkspaceRoots)
	for idx := range cfg.Permissions.Rules {
		cfg.Permissions.Rules[idx].Match.WorkDirPrefixes = resolveRelativePaths(base, cfg.Permissions.Rules[idx].Match.WorkDirPrefixes)
	}
}

func resolveRelativePaths(base string, values []string) []string {
	if len(values) == 0 {
		return nil
	}
	items := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		if !filepath.IsAbs(value) {
			value = filepath.Join(base, value)
		}
		items = append(items, filepath.Clean(value))
	}
	return items
}
