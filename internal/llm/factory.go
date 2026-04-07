package llm

import "myclaw/internal/config"

func NewClientFromConfig(cfg config.LLMConfig) Client {
	if cfg.APIKey == "" {
		return NewMockClient()
	}

	return NewOpenAICompatibleClient(OpenAICompatibleConfig{
		BaseURL: cfg.BaseURL,
		APIKey:  cfg.APIKey,
		Model:   cfg.Model,
	})
}
