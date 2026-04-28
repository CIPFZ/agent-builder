package config

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"myclaw/internal/permissions"
)

type Config struct {
	Version     int
	HTTPAddr    string
	WSPath      string
	Server      ServerConfig
	Runtime     RuntimeConfig
	LLM         LLMConfig
	Permissions PermissionConfig
	Compact     CompactConfig
	MCP         MCPConfig
}

type ServerConfig struct {
	HTTPAddr         string
	WSPath           string
	RequestTimeoutMs int
	RetryMaxRetries  int
	RetryBaseDelayMs int
	RetryMaxDelayMs  int
}

type RuntimeConfig struct {
	MaxTurns int
}

type LLMConfig struct {
	DefaultProfile string
	ActiveProfile  string
	Provider       string
	BaseURL        string
	APIKey         string
	Model          string
	Proxy          LLMProxyConfig
	Providers      map[string]LLMProviderConfig
	Profiles       map[string]LLMProfileConfig
	Routing        LLMRoutingConfig
}

type LLMProxyConfig struct {
	Enabled  bool
	URL      string
	NoProxy  []string
	Explicit bool
}

type LLMProviderConfig struct {
	Protocol     string
	BaseURL      string
	APIKey       string
	Model        string
	Proxy        LLMProxyConfig
	Headers      map[string]string
	Enabled      bool
	TimeoutMs    int
	MaxRetries   int
	AuthScheme   string
	Organization string
	APIVersion   string
}

type LLMProfileConfig struct {
	Provider  string
	Model     string
	BaseURL   string
	APIKey    string
	Streaming bool
}

type LLMRoutingConfig struct {
	DefaultProfile string
	AgentProfiles  map[string]string
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

type MCPServerConfig struct {
	Name                    string
	Enabled                 bool
	Type                    string
	BaseURL                 string
	URL                     string
	Command                 string
	Args                    []string
	Env                     map[string]string
	Headers                 map[string]string
	HeadersHelper           string
	AuthURL                 string
	AuthScope               string
	AuthResourceMetadataURL string
	AuthChallenge           map[string]string
}

type MCPConfig struct {
	Enabled bool
	Skills  bool
	Servers []MCPServerConfig
}

func Default() Config {
	cfg, err := loadFromDir(".")
	if err == nil {
		return cfg
	}
	cfg = defaultConfig()
	if strings.Contains(err.Error(), "resolved llm api_key") {
		cfg.LLM.Provider = "default"
		if provider, ok := cfg.LLM.Providers["default"]; ok {
			cfg.LLM.BaseURL = provider.BaseURL
			cfg.LLM.APIKey = provider.APIKey
		}
		if profile, ok := cfg.LLM.Profiles["default"]; ok {
			cfg.LLM.Model = profile.Model
		}
	}
	return cfg
}

func LoadFromDir(dir string) Config {
	cfg, err := loadFromDir(dir)
	if err != nil {
		panic(err)
	}
	return cfg
}

func loadFromDir(dir string) (Config, error) {
	cfg := defaultConfig()
	if err := mergeFileConfig(&cfg, configPath(dir)); err != nil {
		return Config{}, err
	}
	applyEnvOverrides(&cfg)
	resolvePermissionPaths(&cfg, dir)
	resolveMCPPaths(&cfg, dir)
	if err := validateAndResolve(&cfg); err != nil {
		return Config{}, err
	}
	return cfg, nil
}

func defaultConfig() Config {
	defaultProviderName := "default"
	defaultProfileName := "default"
	defaultProvider := LLMProviderConfig{
		Protocol:   "openai-compatible",
		BaseURL:    "https://api.longcat.chat/openai/v1/chat/completions",
		APIKey:     os.Getenv("MYCLAW_LLM_API_KEY"),
		Enabled:    true,
		TimeoutMs:  60000,
		MaxRetries: 2,
		Headers:    map[string]string{},
	}
	defaultProfile := LLMProfileConfig{
		Provider:  defaultProviderName,
		Model:     firstNonEmptyEnv([]string{"MYCLAW_LLM_MODEL", "ANTHROPIC_MODEL"}, "LongCat-Flash-Chat"),
		Streaming: true,
	}
	cfg := Config{
		Version:  1,
		HTTPAddr: "127.0.0.1:18080",
		WSPath:   "/ws",
		Server: ServerConfig{
			HTTPAddr:         "127.0.0.1:18080",
			WSPath:           "/ws",
			RequestTimeoutMs: 300000,
			RetryMaxRetries:  3,
			RetryBaseDelayMs: 500,
			RetryMaxDelayMs:  4000,
		},
		Runtime: RuntimeConfig{
			MaxTurns: 100,
		},
		LLM: LLMConfig{
			DefaultProfile: defaultProfileName,
			ActiveProfile:  defaultProfileName,
			Providers: map[string]LLMProviderConfig{
				defaultProviderName: defaultProvider,
			},
			Profiles: map[string]LLMProfileConfig{
				defaultProfileName: defaultProfile,
			},
			Routing: LLMRoutingConfig{
				DefaultProfile: defaultProfileName,
				AgentProfiles:  map[string]string{},
			},
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
		MCP: MCPConfig{
			Enabled: true,
			Skills:  envBool("MYCLAW_MCP_SKILLS", true),
		},
	}
	cfg.LLM.Provider = defaultProviderName
	cfg.LLM.BaseURL = defaultProvider.BaseURL
	cfg.LLM.APIKey = defaultProvider.APIKey
	cfg.LLM.Model = defaultProfile.Model
	return cfg
}

type fileConfig struct {
	Config      fileRuntimeConfig    `json:"config"`
	Server      fileServerConfig     `json:"server"`
	Runtime     fileRuntimeControls  `json:"runtime"`
	HTTPAddr    string               `json:"http_addr"`
	WSPath      string               `json:"ws_path"`
	LLM         fileLLMConfig        `json:"llm"`
	Permissions filePermissionConfig `json:"permissions"`
	Compact     fileCompactConfig    `json:"compact"`
	MCP         fileMCPConfig        `json:"mcp"`
}

type fileRuntimeConfig struct {
	Version int `json:"version"`
}

type fileServerConfig struct {
	HTTPAddr         string `json:"http_addr"`
	WSPath           string `json:"ws_path"`
	RequestTimeoutMs *int   `json:"request_timeout_ms"`
	RetryMaxRetries  *int   `json:"retry_max_retries"`
	RetryBaseDelayMs *int   `json:"retry_base_delay_ms"`
	RetryMaxDelayMs  *int   `json:"retry_max_delay_ms"`
}

type fileRuntimeControls struct {
	MaxTurns *int `json:"max_turns"`
}

type fileLLMConfig struct {
	DefaultProfile string                           `json:"default_profile"`
	ActiveProfile  string                           `json:"active_profile"`
	Proxy          fileLLMProxyConfig               `json:"proxy"`
	Providers      map[string]fileLLMProviderConfig `json:"providers"`
	Profiles       map[string]fileLLMProfileConfig  `json:"profiles"`
	Routing        fileLLMRoutingConfig             `json:"routing"`
}

type fileLLMProxyConfig struct {
	Enabled *bool    `json:"enabled"`
	URL     string   `json:"url"`
	NoProxy []string `json:"no_proxy"`
}

type fileLLMProviderConfig struct {
	Protocol     string             `json:"protocol"`
	BaseURL      string             `json:"base_url"`
	APIKey       string             `json:"api_key"`
	Model        string             `json:"model"`
	Proxy        fileLLMProxyConfig `json:"proxy"`
	Headers      map[string]string  `json:"headers"`
	Enabled      *bool              `json:"enabled"`
	TimeoutMs    *int               `json:"timeout_ms"`
	MaxRetries   *int               `json:"max_retries"`
	AuthScheme   string             `json:"auth_scheme"`
	Organization string             `json:"organization"`
	APIVersion   string             `json:"api_version"`
}

type fileLLMProfileConfig struct {
	Provider  string `json:"provider"`
	Model     string `json:"model"`
	BaseURL   string `json:"base_url"`
	APIKey    string `json:"api_key"`
	Streaming *bool  `json:"streaming"`
}

type fileLLMRoutingConfig struct {
	DefaultProfile string            `json:"default_profile"`
	AgentProfiles  map[string]string `json:"agent_profiles"`
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

type fileMCPConfig struct {
	Enabled       *bool                 `json:"enabled"`
	Skills        *bool                 `json:"skills"`
	SkillsEnabled *bool                 `json:"skills_enabled"`
	Servers       []fileMCPServerConfig `json:"servers"`
}

type fileMCPServerConfig struct {
	Name                    string            `json:"name"`
	Enabled                 *bool             `json:"enabled"`
	Type                    string            `json:"type"`
	BaseURL                 string            `json:"base_url"`
	URL                     string            `json:"url"`
	Command                 string            `json:"command"`
	Args                    []string          `json:"args"`
	Env                     map[string]string `json:"env"`
	Headers                 map[string]string `json:"headers"`
	HeadersHelper           string            `json:"headers_helper"`
	AuthURL                 string            `json:"auth_url"`
	AuthScope               string            `json:"auth_scope"`
	AuthResourceMetadataURL string            `json:"auth_resource_metadata_url"`
	AuthChallenge           map[string]string `json:"auth_challenge"`
}

func mergeFileConfig(cfg *Config, path string) error {
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}

	var fileCfg fileConfig
	if err := json.Unmarshal(data, &fileCfg); err != nil {
		return fmt.Errorf("decode %s: %w", path, err)
	}

	if fileCfg.Config.Version != 0 {
		cfg.Version = fileCfg.Config.Version
	}
	if fileCfg.Server.HTTPAddr != "" {
		cfg.HTTPAddr = expandEnv(fileCfg.Server.HTTPAddr)
		cfg.Server.HTTPAddr = cfg.HTTPAddr
	}
	if fileCfg.Server.WSPath != "" {
		cfg.WSPath = expandEnv(fileCfg.Server.WSPath)
		cfg.Server.WSPath = cfg.WSPath
	}
	if fileCfg.Server.RequestTimeoutMs != nil {
		cfg.Server.RequestTimeoutMs = *fileCfg.Server.RequestTimeoutMs
	}
	if fileCfg.Server.RetryMaxRetries != nil {
		cfg.Server.RetryMaxRetries = *fileCfg.Server.RetryMaxRetries
	}
	if fileCfg.Server.RetryBaseDelayMs != nil {
		cfg.Server.RetryBaseDelayMs = *fileCfg.Server.RetryBaseDelayMs
	}
	if fileCfg.Server.RetryMaxDelayMs != nil {
		cfg.Server.RetryMaxDelayMs = *fileCfg.Server.RetryMaxDelayMs
	}
	if fileCfg.Runtime.MaxTurns != nil {
		cfg.Runtime.MaxTurns = *fileCfg.Runtime.MaxTurns
	}
	if fileCfg.HTTPAddr != "" {
		cfg.HTTPAddr = expandEnv(fileCfg.HTTPAddr)
		cfg.Server.HTTPAddr = cfg.HTTPAddr
	}
	if fileCfg.WSPath != "" {
		cfg.WSPath = expandEnv(fileCfg.WSPath)
		cfg.Server.WSPath = cfg.WSPath
	}
	if fileCfg.LLM.DefaultProfile != "" {
		cfg.LLM.DefaultProfile = normalizeMapKey(expandEnv(fileCfg.LLM.DefaultProfile))
	}
	if fileCfg.LLM.ActiveProfile != "" {
		cfg.LLM.ActiveProfile = normalizeMapKey(expandEnv(fileCfg.LLM.ActiveProfile))
	}
	mergeFileProxyConfig(&cfg.LLM.Proxy, fileCfg.LLM.Proxy)
	for name, provider := range fileCfg.LLM.Providers {
		key := normalizeMapKey(name)
		merged := cfg.LLM.Providers[key]
		if merged.Headers == nil {
			merged.Headers = map[string]string{}
		}
		mergeFileProxyConfig(&merged.Proxy, provider.Proxy)
		if provider.Protocol != "" {
			merged.Protocol = expandEnv(provider.Protocol)
		}
		if provider.BaseURL != "" {
			merged.BaseURL = expandEnv(provider.BaseURL)
		}
		if provider.APIKey != "" {
			merged.APIKey = expandEnv(provider.APIKey)
		}
		if provider.Model != "" {
			merged.Model = expandEnv(provider.Model)
		}
		for headerKey, headerValue := range provider.Headers {
			merged.Headers[headerKey] = expandEnv(headerValue)
		}
		if provider.Enabled != nil {
			merged.Enabled = *provider.Enabled
		}
		if provider.TimeoutMs != nil {
			merged.TimeoutMs = *provider.TimeoutMs
		}
		if provider.MaxRetries != nil {
			merged.MaxRetries = *provider.MaxRetries
		}
		if provider.AuthScheme != "" {
			merged.AuthScheme = expandEnv(provider.AuthScheme)
		}
		if provider.Organization != "" {
			merged.Organization = expandEnv(provider.Organization)
		}
		if provider.APIVersion != "" {
			merged.APIVersion = expandEnv(provider.APIVersion)
		}
		cfg.LLM.Providers[key] = merged
	}
	for name, profile := range fileCfg.LLM.Profiles {
		key := normalizeMapKey(name)
		merged := cfg.LLM.Profiles[key]
		if profile.Provider != "" {
			merged.Provider = normalizeMapKey(expandEnv(profile.Provider))
		}
		if profile.Model != "" {
			merged.Model = expandEnv(profile.Model)
		}
		if profile.BaseURL != "" {
			merged.BaseURL = expandEnv(profile.BaseURL)
		}
		if profile.APIKey != "" {
			merged.APIKey = expandEnv(profile.APIKey)
		}
		if profile.Streaming != nil {
			merged.Streaming = *profile.Streaming
		}
		cfg.LLM.Profiles[key] = merged
	}
	if fileCfg.LLM.Routing.DefaultProfile != "" {
		cfg.LLM.Routing.DefaultProfile = normalizeMapKey(expandEnv(fileCfg.LLM.Routing.DefaultProfile))
	}
	if len(fileCfg.LLM.Routing.AgentProfiles) > 0 {
		if cfg.LLM.Routing.AgentProfiles == nil {
			cfg.LLM.Routing.AgentProfiles = map[string]string{}
		}
		for agentType, profileName := range fileCfg.LLM.Routing.AgentProfiles {
			cfg.LLM.Routing.AgentProfiles[normalizeMapKey(agentType)] = normalizeMapKey(expandEnv(profileName))
		}
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
		cfg.Permissions.Rules = append([]permissions.Rule(nil), fileCfg.Permissions.Rules...)
	}
	if len(fileCfg.Permissions.DangerousCommandPatterns) > 0 {
		cfg.Permissions.DangerousCommandPatterns = expandEnvList(fileCfg.Permissions.DangerousCommandPatterns)
	}
	if fileCfg.Compact.VerificationMode != nil {
		cfg.Compact.VerificationMode = *fileCfg.Compact.VerificationMode
	}
	if fileCfg.MCP.Enabled != nil {
		cfg.MCP.Enabled = *fileCfg.MCP.Enabled
	}
	if fileCfg.MCP.Skills != nil {
		cfg.MCP.Skills = *fileCfg.MCP.Skills
	}
	if fileCfg.MCP.SkillsEnabled != nil {
		cfg.MCP.Skills = *fileCfg.MCP.SkillsEnabled
	}
	if len(fileCfg.MCP.Servers) > 0 {
		cfg.MCP.Servers = mergeFileMCPServers(fileCfg.MCP.Servers)
	}
	return nil
}

func applyEnvOverrides(cfg *Config) {
	cfg.HTTPAddr = envOrDefault("MYCLAW_HTTP_ADDR", cfg.HTTPAddr)
	cfg.WSPath = envOrDefault("MYCLAW_WS_PATH", cfg.WSPath)
	cfg.Server.HTTPAddr = cfg.HTTPAddr
	cfg.Server.WSPath = cfg.WSPath
	if value := os.Getenv("MYCLAW_SERVER_REQUEST_TIMEOUT_MS"); value != "" {
		cfg.Server.RequestTimeoutMs = parseInt(value, cfg.Server.RequestTimeoutMs)
	}
	if value := os.Getenv("MYCLAW_SERVER_RETRY_MAX_RETRIES"); value != "" {
		cfg.Server.RetryMaxRetries = parseInt(value, cfg.Server.RetryMaxRetries)
	}
	if value := os.Getenv("MYCLAW_SERVER_RETRY_BASE_DELAY_MS"); value != "" {
		cfg.Server.RetryBaseDelayMs = parseInt(value, cfg.Server.RetryBaseDelayMs)
	}
	if value := os.Getenv("MYCLAW_SERVER_RETRY_MAX_DELAY_MS"); value != "" {
		cfg.Server.RetryMaxDelayMs = parseInt(value, cfg.Server.RetryMaxDelayMs)
	}
	if value := os.Getenv("MYCLAW_RUNTIME_MAX_TURNS"); value != "" {
		cfg.Runtime.MaxTurns = parseInt(value, cfg.Runtime.MaxTurns)
	}

	if value := os.Getenv("MYCLAW_LLM_DEFAULT_PROFILE"); value != "" {
		cfg.LLM.DefaultProfile = normalizeMapKey(value)
	}
	if value := os.Getenv("MYCLAW_LLM_ACTIVE_PROFILE"); value != "" {
		cfg.LLM.ActiveProfile = normalizeMapKey(value)
	}

	for _, entry := range os.Environ() {
		key, value, ok := strings.Cut(entry, "=")
		if !ok {
			continue
		}
		switch {
		case strings.HasPrefix(key, "MYCLAW_LLM_PROXY__"):
			applyProxyEnvOverride(&cfg.LLM.Proxy, strings.TrimPrefix(key, "MYCLAW_LLM_PROXY__"), value)
		case strings.HasPrefix(key, "MYCLAW_LLM_PROVIDERS__"):
			applyProviderEnvOverride(cfg, strings.TrimPrefix(key, "MYCLAW_LLM_PROVIDERS__"), value)
		case strings.HasPrefix(key, "MYCLAW_LLM_PROFILES__"):
			applyProfileEnvOverride(cfg, strings.TrimPrefix(key, "MYCLAW_LLM_PROFILES__"), value)
		case strings.HasPrefix(key, "MYCLAW_LLM_ROUTING__"):
			applyRoutingEnvOverride(cfg, strings.TrimPrefix(key, "MYCLAW_LLM_ROUTING__"), value)
		}
	}

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
	cfg.MCP.Enabled = envBool("MYCLAW_MCP_ENABLED", cfg.MCP.Enabled)
	if value, ok := os.LookupEnv("MYCLAW_MCP_SKILLS_ENABLED"); ok && strings.TrimSpace(value) != "" {
		cfg.MCP.Skills = envBool("MYCLAW_MCP_SKILLS_ENABLED", cfg.MCP.Skills)
	} else {
		cfg.MCP.Skills = envBool("MYCLAW_MCP_SKILLS", cfg.MCP.Skills)
	}
}

func mergeFileMCPServers(servers []fileMCPServerConfig) []MCPServerConfig {
	merged := make([]MCPServerConfig, 0, len(servers))
	for _, server := range servers {
		entry := MCPServerConfig{
			Name:                    expandEnv(server.Name),
			Enabled:                 true,
			Type:                    expandEnv(server.Type),
			BaseURL:                 expandEnv(server.BaseURL),
			URL:                     expandEnv(server.URL),
			Command:                 expandEnv(server.Command),
			Args:                    expandEnvList(server.Args),
			Env:                     expandEnvMap(server.Env),
			Headers:                 expandEnvMap(server.Headers),
			HeadersHelper:           expandEnv(server.HeadersHelper),
			AuthURL:                 expandEnv(server.AuthURL),
			AuthScope:               expandEnv(server.AuthScope),
			AuthResourceMetadataURL: expandEnv(server.AuthResourceMetadataURL),
			AuthChallenge:           expandEnvMap(server.AuthChallenge),
		}
		if server.Enabled != nil {
			entry.Enabled = *server.Enabled
		}
		merged = append(merged, entry)
	}
	return merged
}

func expandEnvMap(values map[string]string) map[string]string {
	if len(values) == 0 {
		return nil
	}
	expanded := make(map[string]string, len(values))
	for key, value := range values {
		expanded[key] = expandEnv(value)
	}
	return expanded
}

func resolveMCPPaths(cfg *Config, dir string) {
	for index := range cfg.MCP.Servers {
		cfg.MCP.Servers[index].Command = resolveMCPPath(dir, cfg.MCP.Servers[index].Command)
		cfg.MCP.Servers[index].HeadersHelper = resolveMCPHelperPath(dir, cfg.MCP.Servers[index].HeadersHelper)
	}
}

func resolveMCPPath(baseDir, value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return ""
	}
	if filepath.IsAbs(value) {
		return filepath.Clean(value)
	}
	if !strings.ContainsAny(value, `/\`) {
		return value
	}
	if strings.TrimSpace(baseDir) == "" {
		baseDir = "."
	}
	return filepath.Clean(filepath.Join(baseDir, value))
}

func resolveMCPHelperPath(baseDir, value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return ""
	}
	if filepath.IsAbs(value) {
		return filepath.Clean(value)
	}
	if strings.Contains(value, " ") || !strings.ContainsAny(value, `/\`) {
		return value
	}
	if strings.TrimSpace(baseDir) == "" {
		baseDir = "."
	}
	return filepath.Clean(filepath.Join(baseDir, value))
}

func validateMCPConfig(cfg *MCPConfig) error {
	if cfg == nil || len(cfg.Servers) == 0 {
		return nil
	}
	seen := make(map[string]struct{}, len(cfg.Servers))
	for index := range cfg.Servers {
		path := fmt.Sprintf("mcp.servers[%d]", index)
		server := &cfg.Servers[index]
		server.Name = strings.TrimSpace(server.Name)
		server.Type = normalizeMCPServerTransport(server.Type, *server)
		server.BaseURL = strings.TrimSpace(server.BaseURL)
		server.URL = strings.TrimSpace(server.URL)
		server.Command = strings.TrimSpace(server.Command)
		server.HeadersHelper = strings.TrimSpace(server.HeadersHelper)
		server.AuthURL = strings.TrimSpace(server.AuthURL)
		server.AuthScope = strings.TrimSpace(server.AuthScope)
		server.AuthResourceMetadataURL = strings.TrimSpace(server.AuthResourceMetadataURL)
		if server.Name == "" {
			return fmt.Errorf("%s.name must not be empty", path)
		}
		key := strings.ToLower(server.Name)
		if _, exists := seen[key]; exists {
			return fmt.Errorf("%s.name %q duplicates another MCP server", path, server.Name)
		}
		seen[key] = struct{}{}
		if !server.Enabled {
			continue
		}
		switch server.Type {
		case "stdio":
			if server.Command == "" {
				return fmt.Errorf("%s.command must not be empty for stdio servers", path)
			}
		case "http", "streamable_http", "sse":
			if strings.TrimSpace(firstNonEmpty(server.BaseURL, server.URL)) == "" {
				return fmt.Errorf("%s.base_url or %s.url must not be empty for %s servers", path, path, server.Type)
			}
		default:
			return fmt.Errorf("%s.type must be one of stdio, http, streamable_http, or sse", path)
		}
	}
	return nil
}

func normalizeMCPServerTransport(value string, server MCPServerConfig) string {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "":
		switch {
		case strings.TrimSpace(server.Command) != "":
			return "stdio"
		case strings.TrimSpace(firstNonEmpty(server.BaseURL, server.URL)) != "":
			return "streamable_http"
		default:
			return ""
		}
	case "stdio", "http", "streamable_http", "sse":
		return strings.ToLower(strings.TrimSpace(value))
	default:
		return strings.ToLower(strings.TrimSpace(value))
	}
}

func applyProviderEnvOverride(cfg *Config, path, value string) {
	parts := strings.Split(path, "__")
	if len(parts) < 2 {
		return
	}
	name := normalizeMapKey(parts[0])
	provider := cfg.LLM.Providers[name]
	if provider.Headers == nil {
		provider.Headers = map[string]string{}
	}
	switch strings.ToUpper(parts[1]) {
	case "PROXY":
		if len(parts) < 3 {
			return
		}
		applyProxyEnvOverride(&provider.Proxy, strings.Join(parts[2:], "__"), value)
	case "PROTOCOL":
		provider.Protocol = value
	case "BASE_URL":
		provider.BaseURL = value
	case "API_KEY":
		provider.APIKey = value
	case "MODEL":
		provider.Model = value
	case "ENABLED":
		provider.Enabled = parseBool(value, provider.Enabled)
	case "TIMEOUT_MS":
		provider.TimeoutMs = parseInt(value, provider.TimeoutMs)
	case "MAX_RETRIES":
		provider.MaxRetries = parseInt(value, provider.MaxRetries)
	case "AUTH_SCHEME":
		provider.AuthScheme = value
	case "ORGANIZATION":
		provider.Organization = value
	case "API_VERSION":
		provider.APIVersion = value
	case "HEADERS":
		if len(parts) == 3 {
			provider.Headers[parts[2]] = value
		}
	default:
		return
	}
	cfg.LLM.Providers[name] = provider
}

func applyProxyEnvOverride(proxyCfg *LLMProxyConfig, path, value string) {
	proxyCfg.Explicit = true
	switch strings.ToUpper(strings.TrimSpace(path)) {
	case "ENABLED":
		proxyCfg.Enabled = parseBool(value, proxyCfg.Enabled)
	case "URL":
		proxyCfg.URL = strings.TrimSpace(value)
	case "NO_PROXY":
		proxyCfg.NoProxy = splitList(value)
	}
}

func applyProfileEnvOverride(cfg *Config, path, value string) {
	parts := strings.Split(path, "__")
	if len(parts) < 2 {
		return
	}
	name := normalizeMapKey(parts[0])
	profile := cfg.LLM.Profiles[name]
	switch strings.ToUpper(parts[1]) {
	case "PROVIDER":
		profile.Provider = normalizeMapKey(value)
	case "MODEL":
		profile.Model = value
	case "BASE_URL":
		profile.BaseURL = value
	case "API_KEY":
		profile.APIKey = value
	case "STREAMING":
		profile.Streaming = parseBool(value, profile.Streaming)
	default:
		return
	}
	cfg.LLM.Profiles[name] = profile
}

func applyRoutingEnvOverride(cfg *Config, path, value string) {
	parts := strings.Split(path, "__")
	if len(parts) == 0 {
		return
	}
	switch strings.ToUpper(parts[0]) {
	case "DEFAULT_PROFILE":
		cfg.LLM.Routing.DefaultProfile = normalizeMapKey(value)
	case "AGENT_PROFILES":
		if len(parts) != 2 {
			return
		}
		if cfg.LLM.Routing.AgentProfiles == nil {
			cfg.LLM.Routing.AgentProfiles = map[string]string{}
		}
		cfg.LLM.Routing.AgentProfiles[normalizeMapKey(parts[1])] = normalizeMapKey(value)
	}
}

func validateAndResolve(cfg *Config) error {
	if cfg.Version <= 0 {
		return fmt.Errorf("config.version must be greater than 0")
	}
	if strings.TrimSpace(cfg.HTTPAddr) == "" {
		return fmt.Errorf("server.http_addr must not be empty")
	}
	if strings.TrimSpace(cfg.WSPath) == "" {
		return fmt.Errorf("server.ws_path must not be empty")
	}
	if cfg.Server.RequestTimeoutMs <= 0 {
		cfg.Server.RequestTimeoutMs = 300000
	}
	if cfg.Server.RetryMaxRetries < 0 {
		cfg.Server.RetryMaxRetries = 0
	}
	if cfg.Server.RetryBaseDelayMs <= 0 {
		cfg.Server.RetryBaseDelayMs = 500
	}
	if cfg.Server.RetryMaxDelayMs <= 0 {
		cfg.Server.RetryMaxDelayMs = cfg.Server.RetryBaseDelayMs
	}
	if cfg.Runtime.MaxTurns <= 0 {
		cfg.Runtime.MaxTurns = 100
	}
	if len(cfg.LLM.Providers) == 0 {
		return fmt.Errorf("llm.providers must define at least one provider")
	}
	if len(cfg.LLM.Profiles) == 0 {
		return fmt.Errorf("llm.profiles must define at least one profile")
	}

	if cfg.LLM.DefaultProfile == "" {
		cfg.LLM.DefaultProfile = cfg.LLM.Routing.DefaultProfile
	}
	if cfg.LLM.Routing.DefaultProfile == "" {
		cfg.LLM.Routing.DefaultProfile = cfg.LLM.DefaultProfile
	}
	if cfg.LLM.ActiveProfile == "" {
		cfg.LLM.ActiveProfile = cfg.LLM.DefaultProfile
	}
	if cfg.LLM.ActiveProfile == "" {
		cfg.LLM.ActiveProfile = cfg.LLM.Routing.DefaultProfile
	}
	activeProfileName := normalizeMapKey(cfg.LLM.ActiveProfile)
	cfg.LLM.ActiveProfile = activeProfileName
	cfg.LLM.DefaultProfile = normalizeMapKey(cfg.LLM.DefaultProfile)
	cfg.LLM.Routing.DefaultProfile = normalizeMapKey(cfg.LLM.Routing.DefaultProfile)

	if _, ok := cfg.LLM.Profiles[cfg.LLM.Routing.DefaultProfile]; !ok {
		return fmt.Errorf("llm.routing.default_profile %q does not exist", cfg.LLM.Routing.DefaultProfile)
	}
	for agentType, profileName := range cfg.LLM.Routing.AgentProfiles {
		if _, ok := cfg.LLM.Profiles[normalizeMapKey(profileName)]; !ok {
			return fmt.Errorf("llm.routing.agent_profiles.%s references unknown profile %q", agentType, profileName)
		}
	}
	if err := validateProxyConfig("llm.proxy", cfg.LLM.Proxy); err != nil {
		return err
	}
	for providerName, providerCfg := range cfg.LLM.Providers {
		if err := validateProxyConfig("llm.providers."+providerName+".proxy", providerCfg.Proxy); err != nil {
			return err
		}
	}
	if err := validateMCPConfig(&cfg.MCP); err != nil {
		return err
	}

	profile, ok := cfg.LLM.Profiles[activeProfileName]
	if !ok {
		return fmt.Errorf("llm.active_profile %q does not exist", activeProfileName)
	}
	providerName := normalizeMapKey(profile.Provider)
	if providerName == "" {
		return fmt.Errorf("llm.profiles.%s.provider must not be empty", activeProfileName)
	}
	provider, ok := cfg.LLM.Providers[providerName]
	if !ok {
		return fmt.Errorf("llm.profiles.%s references unknown provider %q", activeProfileName, providerName)
	}
	if !provider.Enabled {
		return fmt.Errorf("llm.providers.%s is disabled", providerName)
	}
	if strings.TrimSpace(provider.Protocol) == "" {
		return fmt.Errorf("llm.providers.%s.protocol must not be empty", providerName)
	}

	cfg.LLM.Provider = providerName
	cfg.LLM.BaseURL = firstNonEmpty(profile.BaseURL, provider.BaseURL)
	cfg.LLM.APIKey = firstNonEmpty(profile.APIKey, provider.APIKey)
	cfg.LLM.Model = firstNonEmpty(os.Getenv("MYCLAW_LLM_MODEL"), profile.Model, provider.Model)

	if value := os.Getenv("MYCLAW_LLM_BASE_URL"); value != "" {
		cfg.LLM.BaseURL = value
	}
	if value := os.Getenv("MYCLAW_LLM_API_KEY"); value != "" {
		cfg.LLM.APIKey = value
	}

	if strings.TrimSpace(cfg.LLM.BaseURL) == "" {
		return fmt.Errorf("resolved llm base_url for profile %q is empty", activeProfileName)
	}
	if strings.TrimSpace(cfg.LLM.APIKey) == "" {
		if strings.EqualFold(provider.Protocol, "openai-compatible") || strings.EqualFold(provider.Protocol, "mock") {
			cfg.LLM.APIKey = ""
		} else {
			return fmt.Errorf("resolved llm api_key for profile %q is empty", activeProfileName)
		}
	}
	if strings.TrimSpace(cfg.LLM.Model) == "" {
		return fmt.Errorf("resolved llm model for profile %q is empty", activeProfileName)
	}

	cfg.Server.HTTPAddr = cfg.HTTPAddr
	cfg.Server.WSPath = cfg.WSPath
	return nil
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

func userSettingsPath() string {
	if override := os.Getenv("MYCLAW_CONFIG_FILE"); override != "" {
		return override
	}
	return filepath.Join(".", "configs", "myclaw.json")
}

func projectSettingsPath(dir string) string {
	return configPath(dir)
}

func localSettingsPath(dir string) string {
	return configPath(dir)
}

func settingsBaseDir(dir string) string {
	base := dir
	if strings.TrimSpace(base) == "" {
		base = "."
	}
	return base
}

func firstNonEmptyEnv(keys []string, fallback string) string {
	for _, key := range keys {
		if value := os.Getenv(key); value != "" {
			return value
		}
	}
	return fallback
}

func envBool(key string, fallback bool) bool {
	value, ok := os.LookupEnv(key)
	if !ok || strings.TrimSpace(value) == "" {
		return fallback
	}
	return parseBool(value, fallback)
}

func parseBool(value string, fallback bool) bool {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "1", "true", "yes", "on":
		return true
	case "0", "false", "no", "off":
		return false
	default:
		return fallback
	}
}

func parseInt(value string, fallback int) int {
	parsed, err := strconv.Atoi(strings.TrimSpace(value))
	if err != nil {
		return fallback
	}
	return parsed
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

func normalizeMapKey(value string) string {
	return strings.ToLower(strings.TrimSpace(value))
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}

func mergeFileProxyConfig(dst *LLMProxyConfig, src fileLLMProxyConfig) {
	if src.Enabled != nil {
		dst.Enabled = *src.Enabled
		dst.Explicit = true
	}
	if src.URL != "" {
		dst.URL = expandEnv(src.URL)
		dst.Explicit = true
	}
	if len(src.NoProxy) > 0 {
		dst.NoProxy = expandEnvList(src.NoProxy)
		dst.Explicit = true
	}
}

func validateProxyConfig(path string, cfg LLMProxyConfig) error {
	if !cfg.Explicit {
		return nil
	}
	if !cfg.Enabled {
		return nil
	}
	if strings.TrimSpace(cfg.URL) == "" {
		return fmt.Errorf("%s.url must not be empty when proxy is enabled", path)
	}
	lower := strings.ToLower(strings.TrimSpace(cfg.URL))
	switch {
	case strings.HasPrefix(lower, "http://"),
		strings.HasPrefix(lower, "https://"),
		strings.HasPrefix(lower, "socks5://"),
		strings.HasPrefix(lower, "socks5h://"):
		return nil
	default:
		return fmt.Errorf("%s.url must use http, https, socks5, or socks5h", path)
	}
}
