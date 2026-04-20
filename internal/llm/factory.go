package llm

import (
	"context"
	"fmt"
	"strings"

	"myclaw/internal/config"
)

func NewClientFromConfig(cfg config.LLMConfig) Client {
	settings, err := loadProviderRoutingConfig(cfg)
	if err == nil && settings.HasUsableProfiles() {
		return &RoutingClient{
			settings: settings,
			clients:  make(map[string]Client),
		}
	}
	if cfg.APIKey == "" {
		return NewMockClient()
	}

	return NewOpenAICompatibleClient(OpenAICompatibleConfig{
		BaseURL: cfg.BaseURL,
		APIKey:  cfg.APIKey,
		Model:   cfg.Model,
	})
}

type RoutingClient struct {
	settings providerRoutingSettings
	clients  map[string]Client
}

func (c *RoutingClient) Stream(ctx context.Context, req GenerateRequest, handler StreamHandler) error {
	resolved, err := c.settings.ResolveProfile(req.Session.Metadata.AgentType)
	if err != nil {
		return err
	}
	client, err := c.clientForProfile(resolved)
	if err != nil {
		return err
	}
	if strings.TrimSpace(req.Model) == "" {
		req.Model = resolved.Model
	}
	return client.Stream(ctx, req, handler)
}

func (c *RoutingClient) clientForProfile(profile ResolvedProfile) (Client, error) {
	if existing, ok := c.clients[profile.Name]; ok {
		return existing, nil
	}
	provider := profile.Provider
	if strings.TrimSpace(provider.APIKey) == "" {
		return nil, fmt.Errorf("llm provider %q for profile %q is missing api key", provider.Name, profile.Name)
	}
	var client Client
	switch provider.Protocol {
	case ProtocolAnthropic:
		client = NewAnthropicClient(AnthropicConfig{
			BaseURL:      provider.BaseURL,
			APIKey:       provider.APIKey,
			Model:        profile.Model,
			APIVersion:   provider.APIVersion,
			Headers:      provider.Headers,
			Timeout:      provider.Timeout,
			MaxRetries:   provider.MaxRetries,
			APIKeyHeader: provider.APIKeyHeader,
		})
	case ProtocolOpenAICompatible:
		client = NewOpenAICompatibleClient(OpenAICompatibleConfig{
			BaseURL:      provider.BaseURL,
			APIKey:       provider.APIKey,
			Model:        profile.Model,
			Headers:      provider.Headers,
			Timeout:      provider.Timeout,
			MaxRetries:   provider.MaxRetries,
			AuthScheme:   provider.AuthScheme,
			APIKeyHeader: provider.APIKeyHeader,
			Organization: provider.Organization,
		})
	default:
		return nil, fmt.Errorf("unsupported llm protocol %q", provider.Protocol)
	}
	c.clients[profile.Name] = client
	return client, nil
}
