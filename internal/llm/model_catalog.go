package llm

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"path"
	"sort"
	"strings"
	"sync"

	"myclaw/internal/compaction"
	"myclaw/internal/config"
)

type ModelDescriptor struct {
	ID                  string
	DisplayName         string
	Description         string
	ProviderName        string
	ProfileName         string
	ContextWindowTokens int
	MaxOutputTokens     int
	Source              string
}

type ModelCatalog interface {
	ListModels(context.Context) ([]ModelDescriptor, error)
	ValidateModel(context.Context, string) error
	DescribeModel(string) (ModelDescriptor, bool)
}

type staticModelCatalog struct {
	models map[string]ModelDescriptor
	order  []string
}

func NewStaticModelCatalog(models []ModelDescriptor) ModelCatalog {
	index := make(map[string]ModelDescriptor, len(models))
	order := make([]string, 0, len(models))
	for _, model := range models {
		model.ID = strings.TrimSpace(model.ID)
		if model.ID == "" {
			continue
		}
		key := strings.ToLower(model.ID)
		if _, exists := index[key]; exists {
			continue
		}
		index[key] = model
		order = append(order, key)
	}
	return &staticModelCatalog{models: index, order: order}
}

func (c *staticModelCatalog) ListModels(_ context.Context) ([]ModelDescriptor, error) {
	models := make([]ModelDescriptor, 0, len(c.order))
	for _, key := range c.order {
		models = append(models, c.models[key])
	}
	return models, nil
}

func (c *staticModelCatalog) ValidateModel(_ context.Context, model string) error {
	if _, ok := c.DescribeModel(model); ok {
		return nil
	}
	return fmt.Errorf("model %q is not available", strings.TrimSpace(model))
}

func (c *staticModelCatalog) DescribeModel(model string) (ModelDescriptor, bool) {
	descriptor, ok := c.models[strings.ToLower(strings.TrimSpace(model))]
	return descriptor, ok
}

type configuredModelCatalog struct {
	settings      providerRoutingSettings
	activeProfile string

	mu    sync.RWMutex
	cache map[string]ModelDescriptor
}

func NewModelCatalogFromConfig(cfg config.LLMConfig) ModelCatalog {
	settings := catalogSettingsFromConfig(cfg)
	if len(settings.Profiles) == 0 {
		var err error
		settings, err = loadProviderRoutingConfig(cfg)
		if err != nil {
			return NewStaticModelCatalog(nil)
		}
	}
	catalog := &configuredModelCatalog{
		settings:      settings,
		activeProfile: strings.TrimSpace(cfg.ActiveProfile),
		cache:         make(map[string]ModelDescriptor),
	}
	for _, descriptor := range configuredModelDescriptors(settings) {
		catalog.cache[strings.ToLower(descriptor.ID)] = descriptor
	}
	return catalog
}

func catalogSettingsFromConfig(cfg config.LLMConfig) providerRoutingSettings {
	settings := providerRoutingSettings{
		DefaultProfile: strings.TrimSpace(cfg.DefaultProfile),
		GlobalProxy: ProxySettings{
			Enabled:  cfg.Proxy.Enabled,
			URL:      strings.TrimSpace(cfg.Proxy.URL),
			NoProxy:  append([]string(nil), cfg.Proxy.NoProxy...),
			Explicit: cfg.Proxy.Explicit,
		},
		Providers:     make(map[string]ProviderSettings, len(cfg.Providers)),
		Profiles:      make(map[string]ProfileSettings, len(cfg.Profiles)),
		AgentProfiles: make(map[string]string, len(cfg.Routing.AgentProfiles)),
	}
	for name, provider := range cfg.Providers {
		trimmedName := strings.TrimSpace(name)
		if trimmedName == "" {
			continue
		}
		settings.Providers[trimmedName] = ProviderSettings{
			Name:         trimmedName,
			Protocol:     normalizeProtocol(provider.Protocol),
			BaseURL:      strings.TrimSpace(provider.BaseURL),
			APIKey:       strings.TrimSpace(provider.APIKey),
			Model:        strings.TrimSpace(provider.Model),
			Proxy:        ProxySettings{Enabled: provider.Proxy.Enabled, URL: strings.TrimSpace(provider.Proxy.URL), NoProxy: append([]string(nil), provider.Proxy.NoProxy...), Explicit: provider.Proxy.Explicit},
			Headers:      cloneStringMap(provider.Headers),
			Enabled:      provider.Enabled,
			Timeout:      durationFromMillis(provider.TimeoutMs, 60_000_000_000),
			MaxRetries:   intOrDefault(provider.MaxRetries, 2),
			AuthScheme:   strings.TrimSpace(provider.AuthScheme),
			Organization: strings.TrimSpace(provider.Organization),
			APIVersion:   strings.TrimSpace(provider.APIVersion),
		}
	}
	for name, profile := range cfg.Profiles {
		trimmedName := strings.TrimSpace(name)
		if trimmedName == "" {
			continue
		}
		settings.Profiles[trimmedName] = ProfileSettings{
			Name:      trimmedName,
			Provider:  strings.TrimSpace(profile.Provider),
			Model:     strings.TrimSpace(profile.Model),
			Streaming: profile.Streaming,
		}
	}
	for agentType, profileName := range cfg.Routing.AgentProfiles {
		agentType = strings.ToLower(strings.TrimSpace(agentType))
		profileName = strings.TrimSpace(profileName)
		if agentType == "" || profileName == "" {
			continue
		}
		settings.AgentProfiles[agentType] = profileName
	}
	if settings.DefaultProfile == "" {
		settings.DefaultProfile = strings.TrimSpace(cfg.Routing.DefaultProfile)
	}
	return settings
}

func cloneStringMap(input map[string]string) map[string]string {
	if len(input) == 0 {
		return nil
	}
	out := make(map[string]string, len(input))
	for key, value := range input {
		out[key] = value
	}
	return out
}

func (c *configuredModelCatalog) ListModels(ctx context.Context) ([]ModelDescriptor, error) {
	models := make(map[string]ModelDescriptor)
	for _, descriptor := range configuredModelDescriptors(c.settings) {
		models[strings.ToLower(descriptor.ID)] = descriptor
	}

	for _, provider := range providerList(c.settings) {
		discovered, err := discoverProviderModels(ctx, c.settings.GlobalProxy, provider)
		if err != nil {
			continue
		}
		for _, descriptor := range discovered {
			key := strings.ToLower(descriptor.ID)
			current, exists := models[key]
			if exists {
				if current.ProfileName != "" && descriptor.ProfileName == "" {
					descriptor.ProfileName = current.ProfileName
				}
				if current.ProviderName != "" && descriptor.ProviderName == "" {
					descriptor.ProviderName = current.ProviderName
				}
			}
			if profileName := profileNameForModel(c.settings, descriptor.ID); profileName != "" {
				descriptor.ProfileName = profileName
			}
			models[key] = descriptor
		}
	}

	out := make([]ModelDescriptor, 0, len(models))
	for _, descriptor := range models {
		out = append(out, descriptor)
	}
	sort.SliceStable(out, func(i, j int) bool {
		left := out[i]
		right := out[j]
		if left.ProfileName == c.activeProfile && right.ProfileName != c.activeProfile {
			return true
		}
		if right.ProfileName == c.activeProfile && left.ProfileName != c.activeProfile {
			return false
		}
		if left.ProfileName != "" && right.ProfileName == "" {
			return true
		}
		if right.ProfileName != "" && left.ProfileName == "" {
			return false
		}
		return strings.ToLower(left.ID) < strings.ToLower(right.ID)
	})

	c.mu.Lock()
	defer c.mu.Unlock()
	for _, descriptor := range out {
		c.cache[strings.ToLower(descriptor.ID)] = descriptor
	}
	return out, nil
}

func (c *configuredModelCatalog) ValidateModel(ctx context.Context, model string) error {
	model = strings.TrimSpace(model)
	if model == "" {
		return fmt.Errorf("model name is empty")
	}
	if _, ok := c.DescribeModel(model); ok {
		return nil
	}
	models, err := c.ListModels(ctx)
	if err != nil {
		return err
	}
	for _, descriptor := range models {
		if strings.EqualFold(descriptor.ID, model) {
			return nil
		}
	}
	return fmt.Errorf("model %q is not available", model)
}

func (c *configuredModelCatalog) DescribeModel(model string) (ModelDescriptor, bool) {
	model = strings.TrimSpace(model)
	if model == "" {
		return ModelDescriptor{}, false
	}
	c.mu.RLock()
	descriptor, ok := c.cache[strings.ToLower(model)]
	c.mu.RUnlock()
	if ok {
		return descriptor, true
	}
	if heuristic, ok := heuristicModelDescriptor(model); ok {
		return heuristic, true
	}
	return ModelDescriptor{}, false
}

func ClaudeCompactionConfig(base compaction.Config, model string, catalog ModelCatalog) compaction.Config {
	descriptor, ok := describeModelWithFallback(catalog, model)
	if !ok || descriptor.ContextWindowTokens <= 0 {
		return base
	}
	reservedOutput := descriptor.MaxOutputTokens
	if reservedOutput <= 0 {
		reservedOutput = 20000
	}
	effectiveWindow := descriptor.ContextWindowTokens - reservedOutput
	if effectiveWindow <= 0 {
		return base
	}
	base.ContextWindowTokens = effectiveWindow
	base.WarningBufferTokens = 33000
	base.ErrorBufferTokens = 33000
	base.AutoCompactBufferTokens = 13000
	base.BlockingBufferTokens = 3000
	return base
}

func describeModelWithFallback(catalog ModelCatalog, model string) (ModelDescriptor, bool) {
	if catalog != nil {
		if descriptor, ok := catalog.DescribeModel(model); ok {
			return descriptor, true
		}
	}
	return heuristicModelDescriptor(model)
}

func configuredModelDescriptors(settings providerRoutingSettings) []ModelDescriptor {
	models := make(map[string]ModelDescriptor)
	for profileName, profile := range settings.Profiles {
		model := strings.TrimSpace(profile.Model)
		if model == "" {
			if provider, ok := settings.Providers[profile.Provider]; ok {
				model = strings.TrimSpace(provider.Model)
			}
		}
		if model == "" {
			continue
		}
		descriptor, _ := heuristicModelDescriptor(model)
		descriptor.ID = model
		descriptor.DisplayName = catalogFirstNonEmpty(descriptor.DisplayName, model)
		descriptor.ProviderName = profile.Provider
		descriptor.ProfileName = profileName
		descriptor.Source = "configured"
		models[strings.ToLower(model)] = descriptor
	}
	out := make([]ModelDescriptor, 0, len(models))
	for _, descriptor := range models {
		out = append(out, descriptor)
	}
	return out
}

func providerList(settings providerRoutingSettings) []ProviderSettings {
	out := make([]ProviderSettings, 0, len(settings.Providers))
	for _, provider := range settings.Providers {
		if !provider.Enabled || strings.TrimSpace(provider.APIKey) == "" || strings.TrimSpace(provider.BaseURL) == "" {
			continue
		}
		out = append(out, provider)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out
}

func profileNameForModel(settings providerRoutingSettings, model string) string {
	for name, profile := range settings.Profiles {
		if strings.EqualFold(strings.TrimSpace(profile.Model), model) {
			return name
		}
	}
	return ""
}

func discoverProviderModels(ctx context.Context, globalProxy ProxySettings, provider ProviderSettings) ([]ModelDescriptor, error) {
	client, err := newHTTPClient(provider.Timeout, globalProxy, provider.Proxy)
	if err != nil {
		return nil, err
	}
	endpoint, err := modelsEndpoint(provider)
	if err != nil {
		return nil, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, err
	}
	for key, value := range provider.Headers {
		if strings.TrimSpace(key) != "" && strings.TrimSpace(value) != "" {
			req.Header.Set(key, value)
		}
	}
	applyProviderAuthHeaders(req, provider)
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= http.StatusBadRequest {
		data, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return nil, fmt.Errorf("discover models: %s: %s", resp.Status, strings.TrimSpace(string(data)))
	}
	switch provider.Protocol {
	case ProtocolAnthropic:
		return parseAnthropicModels(resp.Body, provider.Name)
	default:
		return parseOpenAIModels(resp.Body, provider.Name)
	}
}

func modelsEndpoint(provider ProviderSettings) (string, error) {
	raw := strings.TrimSpace(provider.BaseURL)
	parsed, err := url.Parse(raw)
	if err != nil {
		return "", err
	}
	if strings.TrimSpace(parsed.Scheme) == "" || strings.TrimSpace(parsed.Host) == "" {
		return "", fmt.Errorf("invalid provider base url %q", raw)
	}
	parsed.Path = joinProviderPath(parsed.Path, "models")
	return parsed.String(), nil
}

func joinProviderPath(currentPath, leaf string) string {
	currentPath = strings.TrimSpace(currentPath)
	if currentPath == "" || currentPath == "/" {
		return "/" + strings.TrimPrefix(leaf, "/")
	}
	lower := strings.ToLower(currentPath)
	switch {
	case strings.HasSuffix(lower, "/chat/completions"):
		currentPath = currentPath[:len(currentPath)-len("/chat/completions")]
	case strings.HasSuffix(lower, "/messages"):
		currentPath = currentPath[:len(currentPath)-len("/messages")]
	}
	currentPath = strings.TrimSuffix(currentPath, "/")
	if strings.HasSuffix(strings.ToLower(currentPath), "/v1") {
		return currentPath + "/" + strings.TrimPrefix(leaf, "/")
	}
	return path.Join(currentPath, leaf)
}

func applyProviderAuthHeaders(req *http.Request, provider ProviderSettings) {
	if req == nil {
		return
	}
	switch provider.Protocol {
	case ProtocolAnthropic:
		keyHeader := strings.TrimSpace(provider.APIKeyHeader)
		if keyHeader == "" {
			keyHeader = "x-api-key"
		}
		req.Header.Set(keyHeader, provider.APIKey)
		apiVersion := strings.TrimSpace(provider.APIVersion)
		if apiVersion == "" {
			apiVersion = "2023-06-01"
		}
		req.Header.Set("anthropic-version", apiVersion)
	default:
		keyHeader := strings.TrimSpace(provider.APIKeyHeader)
		if keyHeader == "" {
			keyHeader = "Authorization"
		}
		authScheme := strings.TrimSpace(provider.AuthScheme)
		if authScheme == "" {
			authScheme = "Bearer"
		}
		if strings.EqualFold(keyHeader, "Authorization") {
			req.Header.Set(keyHeader, authScheme+" "+provider.APIKey)
		} else {
			req.Header.Set(keyHeader, provider.APIKey)
		}
		if provider.Organization != "" {
			req.Header.Set("OpenAI-Organization", provider.Organization)
		}
	}
}

func parseOpenAIModels(r io.Reader, providerName string) ([]ModelDescriptor, error) {
	var payload struct {
		Data []struct {
			ID              string `json:"id"`
			OwnedBy         string `json:"owned_by"`
			ContextWindow   int    `json:"context_window"`
			MaxOutputTokens int    `json:"max_output_tokens"`
		} `json:"data"`
	}
	if err := json.NewDecoder(r).Decode(&payload); err != nil {
		return nil, err
	}
	models := make([]ModelDescriptor, 0, len(payload.Data))
	for _, item := range payload.Data {
		if strings.TrimSpace(item.ID) == "" {
			continue
		}
		descriptor, _ := heuristicModelDescriptor(item.ID)
		descriptor.ID = item.ID
		descriptor.DisplayName = catalogFirstNonEmpty(descriptor.DisplayName, item.ID)
		descriptor.ProviderName = providerName
		descriptor.Source = "discovered"
		if item.ContextWindow > 0 {
			descriptor.ContextWindowTokens = item.ContextWindow
		}
		if item.MaxOutputTokens > 0 {
			descriptor.MaxOutputTokens = item.MaxOutputTokens
		}
		models = append(models, descriptor)
	}
	return models, nil
}

func parseAnthropicModels(r io.Reader, providerName string) ([]ModelDescriptor, error) {
	var payload struct {
		Data []struct {
			ID              string `json:"id"`
			DisplayName     string `json:"display_name"`
			ContextWindow   int    `json:"context_window"`
			MaxOutputTokens int    `json:"max_output_tokens"`
		} `json:"data"`
	}
	if err := json.NewDecoder(r).Decode(&payload); err != nil {
		return nil, err
	}
	models := make([]ModelDescriptor, 0, len(payload.Data))
	for _, item := range payload.Data {
		if strings.TrimSpace(item.ID) == "" {
			continue
		}
		descriptor, _ := heuristicModelDescriptor(item.ID)
		descriptor.ID = item.ID
		descriptor.DisplayName = catalogFirstNonEmpty(strings.TrimSpace(item.DisplayName), descriptor.DisplayName, item.ID)
		descriptor.ProviderName = providerName
		descriptor.Source = "discovered"
		if item.ContextWindow > 0 {
			descriptor.ContextWindowTokens = item.ContextWindow
		}
		if item.MaxOutputTokens > 0 {
			descriptor.MaxOutputTokens = item.MaxOutputTokens
		}
		models = append(models, descriptor)
	}
	return models, nil
}

func heuristicModelDescriptor(model string) (ModelDescriptor, bool) {
	model = strings.TrimSpace(model)
	if model == "" {
		return ModelDescriptor{}, false
	}
	lower := strings.ToLower(model)
	descriptor := ModelDescriptor{
		ID:          model,
		DisplayName: model,
	}
	switch {
	case strings.Contains(lower, "minimax-m2.7"):
		descriptor.ContextWindowTokens = 204800
		descriptor.MaxOutputTokens = 32000
	case strings.Contains(lower, "gpt-5"):
		descriptor.ContextWindowTokens = 400000
		descriptor.MaxOutputTokens = 128000
	case strings.Contains(lower, "gpt-4.1"):
		descriptor.ContextWindowTokens = 1047576
		descriptor.MaxOutputTokens = 32768
	case strings.Contains(lower, "gemini-2.5-pro"):
		descriptor.ContextWindowTokens = 1048576
		descriptor.MaxOutputTokens = 65536
	case strings.Contains(lower, "[1m]"):
		descriptor.ContextWindowTokens = 1000000
		descriptor.MaxOutputTokens = 20000
	case strings.Contains(lower, "claude-opus-4-6"):
		descriptor.ContextWindowTokens = 200000
		descriptor.MaxOutputTokens = 32000
	case strings.Contains(lower, "claude-sonnet-4-6"), strings.Contains(lower, "claude-sonnet-4-5"):
		descriptor.ContextWindowTokens = 200000
		descriptor.MaxOutputTokens = 64000
	case strings.Contains(lower, "claude-haiku-4-5"):
		descriptor.ContextWindowTokens = 200000
		descriptor.MaxOutputTokens = 64000
	default:
		return ModelDescriptor{}, false
	}
	return descriptor, true
}

func minPositive(value, ceiling int) int {
	switch {
	case value <= 0:
		return ceiling
	case ceiling <= 0:
		return value
	case value < ceiling:
		return value
	default:
		return ceiling
	}
}

func catalogFirstNonEmpty(values ...string) string {
	for _, value := range values {
		if trimmed := strings.TrimSpace(value); trimmed != "" {
			return trimmed
		}
	}
	return ""
}
