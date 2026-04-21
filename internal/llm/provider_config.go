package llm

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"myclaw/internal/config"
)

type Protocol string

const (
	ProtocolOpenAICompatible Protocol = "openai-compatible"
	ProtocolAnthropic        Protocol = "anthropic"
)

type ProviderSettings struct {
	Name         string
	Protocol     Protocol
	BaseURL      string
	APIKey       string
	Model        string
	Proxy        ProxySettings
	Headers      map[string]string
	Enabled      bool
	Timeout      time.Duration
	MaxRetries   int
	AuthScheme   string
	APIKeyHeader string
	Organization string
	APIVersion   string
}

type ProfileSettings struct {
	Name      string
	Provider  string
	Model     string
	Streaming bool
}

type ResolvedProfile struct {
	Name      string
	Provider  ProviderSettings
	Model     string
	Streaming bool
}

type providerRoutingSettings struct {
	DefaultProfile string
	GlobalProxy    ProxySettings
	Providers      map[string]ProviderSettings
	Profiles       map[string]ProfileSettings
	AgentProfiles  map[string]string
}

type ProxySettings struct {
	Enabled  bool
	URL      string
	NoProxy  []string
	Explicit bool
}

func (s providerRoutingSettings) HasUsableProfiles() bool {
	if len(s.Profiles) == 0 {
		return false
	}
	for _, profile := range s.Profiles {
		provider, ok := s.Providers[profile.Provider]
		if !ok || !provider.Enabled {
			continue
		}
		if strings.TrimSpace(provider.APIKey) != "" {
			return true
		}
	}
	return false
}

func (s providerRoutingSettings) ResolveProfile(agentType string) (ResolvedProfile, error) {
	if len(s.Profiles) == 0 {
		return ResolvedProfile{}, fmt.Errorf("no llm profiles configured")
	}
	name := strings.TrimSpace(s.AgentProfiles[strings.TrimSpace(agentType)])
	if name == "" {
		name = strings.TrimSpace(s.DefaultProfile)
	}
	if name == "" {
		for profileName := range s.Profiles {
			name = profileName
			break
		}
	}
	profile, ok := s.Profiles[name]
	if !ok {
		return ResolvedProfile{}, fmt.Errorf("llm profile %q not found", name)
	}
	provider, ok := s.Providers[profile.Provider]
	if !ok {
		return ResolvedProfile{}, fmt.Errorf("llm provider %q not found for profile %q", profile.Provider, name)
	}
	if !provider.Enabled {
		return ResolvedProfile{}, fmt.Errorf("llm provider %q is disabled", profile.Provider)
	}
	modelName := strings.TrimSpace(profile.Model)
	if modelName == "" {
		modelName = strings.TrimSpace(provider.Model)
	}
	return ResolvedProfile{
		Name:      name,
		Provider:  provider,
		Model:     modelName,
		Streaming: profile.Streaming,
	}, nil
}

type fileMyclawConfig struct {
	LLM fileLLMSettings `json:"llm"`
}

type fileLLMSettings struct {
	DefaultProfile string                        `json:"default_profile"`
	Proxy          fileProxyConfig               `json:"proxy"`
	Providers      map[string]fileProviderConfig `json:"providers"`
	Profiles       map[string]fileProfileConfig  `json:"profiles"`
	Routing        fileRoutingConfig             `json:"routing"`

	Provider string `json:"provider"`
	BaseURL  string `json:"base_url"`
	APIKey   string `json:"api_key"`
	Model    string `json:"model"`
}

type fileProxyConfig struct {
	Enabled *bool    `json:"enabled"`
	URL     string   `json:"url"`
	NoProxy []string `json:"no_proxy"`
}

type fileProviderConfig struct {
	Protocol     string            `json:"protocol"`
	BaseURL      string            `json:"base_url"`
	APIKey       string            `json:"api_key"`
	Model        string            `json:"model"`
	Proxy        fileProxyConfig   `json:"proxy"`
	Headers      map[string]string `json:"headers"`
	Enabled      *bool             `json:"enabled"`
	TimeoutMS    int               `json:"timeout_ms"`
	MaxRetries   int               `json:"max_retries"`
	AuthScheme   string            `json:"auth_scheme"`
	APIKeyHeader string            `json:"api_key_header"`
	Organization string            `json:"organization"`
	APIVersion   string            `json:"api_version"`
}

type fileProfileConfig struct {
	Provider  string `json:"provider"`
	Model     string `json:"model"`
	Streaming *bool  `json:"streaming"`
}

type fileRoutingConfig struct {
	DefaultProfile string            `json:"default_profile"`
	AgentProfiles  map[string]string `json:"agent_profiles"`
}

func loadProviderRoutingConfig(legacy config.LLMConfig) (providerRoutingSettings, error) {
	settings := providerRoutingSettings{
		Providers:     make(map[string]ProviderSettings),
		Profiles:      make(map[string]ProfileSettings),
		AgentProfiles: make(map[string]string),
	}

	if path := activeMyclawConfigPath(); path != "" {
		if err := mergeMyclawConfigFile(&settings, path); err != nil {
			return providerRoutingSettings{}, err
		}
	}
	if len(settings.Profiles) == 0 {
		mergeLegacyLLMConfig(&settings, legacy)
	}
	applyProviderEnvOverrides(&settings)
	if len(settings.Profiles) == 0 {
		return providerRoutingSettings{}, nil
	}
	return settings, nil
}

func activeMyclawConfigPath() string {
	if override := strings.TrimSpace(os.Getenv("MYCLAW_CONFIG_FILE")); override != "" {
		return override
	}
	return filepath.Join(".", "configs", "myclaw.json")
}

func mergeMyclawConfigFile(settings *providerRoutingSettings, path string) error {
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	var fileCfg fileMyclawConfig
	if err := json.Unmarshal(data, &fileCfg); err != nil {
		return fmt.Errorf("decode myclaw config: %w", err)
	}

	if len(fileCfg.LLM.Providers) == 0 && strings.TrimSpace(fileCfg.LLM.Provider) != "" {
		legacyProvider := ProviderSettings{
			Name:       "default",
			Protocol:   normalizeProtocol(fileCfg.LLM.Provider),
			BaseURL:    expandEnv(fileCfg.LLM.BaseURL),
			APIKey:     expandEnv(fileCfg.LLM.APIKey),
			Model:      expandEnv(fileCfg.LLM.Model),
			Enabled:    true,
			Timeout:    60 * time.Second,
			MaxRetries: 2,
		}
		settings.Providers[legacyProvider.Name] = legacyProvider
		settings.Profiles["default"] = ProfileSettings{
			Name:      "default",
			Provider:  legacyProvider.Name,
			Model:     legacyProvider.Model,
			Streaming: true,
		}
		settings.DefaultProfile = "default"
		return nil
	}

	mergeFileProxySettings(&settings.GlobalProxy, fileCfg.LLM.Proxy)

	for name, provider := range fileCfg.LLM.Providers {
		normalizedName := strings.TrimSpace(name)
		if normalizedName == "" {
			continue
		}
		settings.Providers[normalizedName] = ProviderSettings{
			Name:         normalizedName,
			Protocol:     normalizeProtocol(provider.Protocol),
			BaseURL:      expandEnv(provider.BaseURL),
			APIKey:       expandEnv(provider.APIKey),
			Model:        expandEnv(provider.Model),
			Proxy:        proxySettingsFromFile(provider.Proxy),
			Headers:      expandStringMap(provider.Headers),
			Enabled:      provider.Enabled == nil || *provider.Enabled,
			Timeout:      durationFromMillis(provider.TimeoutMS, 60*time.Second),
			MaxRetries:   intOrDefault(provider.MaxRetries, 2),
			AuthScheme:   expandEnv(provider.AuthScheme),
			APIKeyHeader: expandEnv(provider.APIKeyHeader),
			Organization: expandEnv(provider.Organization),
			APIVersion:   expandEnv(provider.APIVersion),
		}
	}
	for name, profile := range fileCfg.LLM.Profiles {
		normalizedName := strings.TrimSpace(name)
		if normalizedName == "" {
			continue
		}
		settings.Profiles[normalizedName] = ProfileSettings{
			Name:      normalizedName,
			Provider:  strings.TrimSpace(profile.Provider),
			Model:     expandEnv(profile.Model),
			Streaming: profile.Streaming == nil || *profile.Streaming,
		}
	}
	settings.DefaultProfile = firstNonEmpty(strings.TrimSpace(fileCfg.LLM.Routing.DefaultProfile), strings.TrimSpace(fileCfg.LLM.DefaultProfile), settings.DefaultProfile)
	for agentType, profileName := range fileCfg.LLM.Routing.AgentProfiles {
		agentType = strings.TrimSpace(agentType)
		profileName = strings.TrimSpace(profileName)
		if agentType == "" || profileName == "" {
			continue
		}
		settings.AgentProfiles[agentType] = profileName
	}
	return nil
}

func mergeLegacyLLMConfig(settings *providerRoutingSettings, legacy config.LLMConfig) {
	provider := ProviderSettings{
		Name:       "legacy-default",
		Protocol:   normalizeProtocol(legacy.Provider),
		BaseURL:    strings.TrimSpace(legacy.BaseURL),
		APIKey:     strings.TrimSpace(legacy.APIKey),
		Model:      strings.TrimSpace(legacy.Model),
		Enabled:    true,
		Timeout:    60 * time.Second,
		MaxRetries: 2,
	}
	if provider.Protocol == "" {
		provider.Protocol = ProtocolOpenAICompatible
	}
	settings.Providers[provider.Name] = provider
	settings.Profiles["legacy-default"] = ProfileSettings{
		Name:      "legacy-default",
		Provider:  provider.Name,
		Model:     provider.Model,
		Streaming: true,
	}
	settings.DefaultProfile = "legacy-default"
}

func applyProviderEnvOverrides(settings *providerRoutingSettings) {
	if settings.Providers == nil {
		settings.Providers = make(map[string]ProviderSettings)
	}
	if settings.Profiles == nil {
		settings.Profiles = make(map[string]ProfileSettings)
	}
	if settings.AgentProfiles == nil {
		settings.AgentProfiles = make(map[string]string)
	}

	if value := strings.TrimSpace(os.Getenv("MYCLAW_LLM_DEFAULT_PROFILE")); value != "" {
		settings.DefaultProfile = value
	}
	for _, env := range os.Environ() {
		key, value, ok := strings.Cut(env, "=")
		if !ok {
			continue
		}
		switch {
		case strings.HasPrefix(key, "MYCLAW_LLM_PROXY__"):
			applyProxyEnvOverride(&settings.GlobalProxy, strings.TrimPrefix(key, "MYCLAW_LLM_PROXY__"), value)
		case strings.HasPrefix(key, "MYCLAW_LLM_PROVIDER_"):
			applyLegacyProviderEnvOverride(settings, strings.TrimPrefix(key, "MYCLAW_LLM_PROVIDER_"), value)
		}
	}
	for name, provider := range settings.Providers {
		key := envKeyPart(name)
		if value := strings.TrimSpace(os.Getenv("MYCLAW_LLM_PROVIDER_" + key + "_PROTOCOL")); value != "" {
			provider.Protocol = normalizeProtocol(value)
		}
		if value := strings.TrimSpace(os.Getenv("MYCLAW_LLM_PROVIDER_" + key + "_BASE_URL")); value != "" {
			provider.BaseURL = value
		}
		if value := os.Getenv("MYCLAW_LLM_PROVIDER_" + key + "_API_KEY"); value != "" {
			provider.APIKey = value
		}
		if value := strings.TrimSpace(os.Getenv("MYCLAW_LLM_PROVIDER_" + key + "_MODEL")); value != "" {
			provider.Model = value
		}
		if value := strings.TrimSpace(os.Getenv("MYCLAW_LLM_PROVIDER_" + key + "_API_VERSION")); value != "" {
			provider.APIVersion = value
		}
		if value := strings.TrimSpace(os.Getenv("MYCLAW_LLM_PROVIDER_" + key + "_AUTH_SCHEME")); value != "" {
			provider.AuthScheme = value
		}
		if value := strings.TrimSpace(os.Getenv("MYCLAW_LLM_PROVIDER_" + key + "_API_KEY_HEADER")); value != "" {
			provider.APIKeyHeader = value
		}
		if value, ok := lookupEnvBool("MYCLAW_LLM_PROVIDER_" + key + "_ENABLED"); ok {
			provider.Enabled = value
		}
		if value, ok := lookupEnvInt("MYCLAW_LLM_PROVIDER_" + key + "_TIMEOUT_MS"); ok {
			provider.Timeout = durationFromMillis(value, provider.Timeout)
		}
		if value, ok := lookupEnvInt("MYCLAW_LLM_PROVIDER_" + key + "_MAX_RETRIES"); ok {
			provider.MaxRetries = value
		}
		if value, ok := os.LookupEnv("MYCLAW_LLM_PROVIDER_" + key + "_PROXY__URL"); ok {
			applyProxyEnvOverride(&provider.Proxy, "URL", value)
		}
		if value, ok := os.LookupEnv("MYCLAW_LLM_PROVIDER_" + key + "_PROXY__ENABLED"); ok {
			applyProxyEnvOverride(&provider.Proxy, "ENABLED", value)
		}
		if value, ok := os.LookupEnv("MYCLAW_LLM_PROVIDER_" + key + "_PROXY__NO_PROXY"); ok {
			applyProxyEnvOverride(&provider.Proxy, "NO_PROXY", value)
		}
		settings.Providers[name] = provider
	}
	for name, profile := range settings.Profiles {
		key := envKeyPart(name)
		if value := strings.TrimSpace(os.Getenv("MYCLAW_LLM_PROFILE_" + key + "_PROVIDER")); value != "" {
			profile.Provider = value
		}
		if value := strings.TrimSpace(os.Getenv("MYCLAW_LLM_PROFILE_" + key + "_MODEL")); value != "" {
			profile.Model = value
		}
		if value, ok := lookupEnvBool("MYCLAW_LLM_PROFILE_" + key + "_STREAMING"); ok {
			profile.Streaming = value
		}
		settings.Profiles[name] = profile
	}
	for _, env := range os.Environ() {
		key, value, ok := strings.Cut(env, "=")
		if !ok || !strings.HasPrefix(key, "MYCLAW_LLM_AGENT_PROFILE_") {
			continue
		}
		agentType := strings.TrimSpace(strings.TrimPrefix(key, "MYCLAW_LLM_AGENT_PROFILE_"))
		if agentType == "" || strings.TrimSpace(value) == "" {
			continue
		}
		settings.AgentProfiles[strings.ToLower(agentType)] = strings.TrimSpace(value)
	}
	normalized := make(map[string]string, len(settings.AgentProfiles))
	for agentType, profileName := range settings.AgentProfiles {
		normalized[strings.ToLower(strings.TrimSpace(agentType))] = strings.TrimSpace(profileName)
	}
	settings.AgentProfiles = normalized
}

func normalizeProtocol(raw string) Protocol {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "", "openai", "openai-compatible":
		return ProtocolOpenAICompatible
	case "anthropic":
		return ProtocolAnthropic
	default:
		return Protocol(strings.TrimSpace(raw))
	}
}

func expandStringMap(values map[string]string) map[string]string {
	if len(values) == 0 {
		return nil
	}
	out := make(map[string]string, len(values))
	for key, value := range values {
		out[key] = expandEnv(value)
	}
	return out
}

func proxySettingsFromFile(src fileProxyConfig) ProxySettings {
	var out ProxySettings
	mergeFileProxySettings(&out, src)
	return out
}

func mergeFileProxySettings(dst *ProxySettings, src fileProxyConfig) {
	if src.Enabled != nil {
		dst.Enabled = *src.Enabled
		dst.Explicit = true
	}
	if src.URL != "" {
		dst.URL = expandEnv(src.URL)
		dst.Explicit = true
	}
	if len(src.NoProxy) > 0 {
		dst.NoProxy = splitList(strings.Join(expandEnvList(src.NoProxy), ","))
		dst.Explicit = true
	}
}

func applyProxyEnvOverride(dst *ProxySettings, key, value string) {
	dst.Explicit = true
	switch strings.ToUpper(strings.TrimSpace(key)) {
	case "ENABLED":
		dst.Enabled = parseEnvBool(value, dst.Enabled)
	case "URL":
		dst.URL = strings.TrimSpace(value)
	case "NO_PROXY":
		dst.NoProxy = splitList(value)
	}
}

func applyLegacyProviderEnvOverride(settings *providerRoutingSettings, suffix, value string) {
	parts := strings.SplitN(suffix, "_PROXY__", 2)
	if len(parts) != 2 {
		return
	}
	name := strings.TrimSpace(strings.ToLower(strings.NewReplacer("_", "-", ".", "-").Replace(parts[0])))
	provider, ok := settings.Providers[name]
	if !ok {
		return
	}
	applyProxyEnvOverride(&provider.Proxy, parts[1], value)
	settings.Providers[name] = provider
}

func splitList(raw string) []string {
	if strings.TrimSpace(raw) == "" {
		return nil
	}
	fields := strings.FieldsFunc(raw, func(r rune) bool {
		return r == ';' || r == ','
	})
	out := make([]string, 0, len(fields))
	for _, field := range fields {
		field = strings.TrimSpace(field)
		if field != "" {
			out = append(out, field)
		}
	}
	return out
}

func expandEnvList(values []string) []string {
	if len(values) == 0 {
		return nil
	}
	out := make([]string, 0, len(values))
	for _, value := range values {
		out = append(out, expandEnv(value))
	}
	return out
}

func parseEnvBool(raw string, fallback bool) bool {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "1", "true", "yes", "on":
		return true
	case "0", "false", "no", "off":
		return false
	default:
		return fallback
	}
}

func envKeyPart(raw string) string {
	replacer := strings.NewReplacer("-", "_", ".", "_", " ", "_")
	return strings.ToUpper(replacer.Replace(strings.TrimSpace(raw)))
}

func durationFromMillis(value int, fallback time.Duration) time.Duration {
	if value <= 0 {
		return fallback
	}
	return time.Duration(value) * time.Millisecond
}

func intOrDefault(value int, fallback int) int {
	if value <= 0 {
		return fallback
	}
	return value
}

func lookupEnvBool(key string) (bool, bool) {
	raw, ok := os.LookupEnv(key)
	if !ok {
		return false, false
	}
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "1", "true", "yes", "on":
		return true, true
	case "0", "false", "no", "off":
		return false, true
	default:
		return false, false
	}
}

func lookupEnvInt(key string) (int, bool) {
	raw, ok := os.LookupEnv(key)
	if !ok || strings.TrimSpace(raw) == "" {
		return 0, false
	}
	value, err := strconv.Atoi(strings.TrimSpace(raw))
	if err != nil {
		return 0, false
	}
	return value, true
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}

func expandEnv(value string) string {
	return os.ExpandEnv(value)
}
