package app

import (
	"strings"

	"myclaw/internal/config"
	"myclaw/internal/llm"
)

func LoadRuntimeConfig(dir string) config.Config {
	cfg := config.Default()
	defer func() {
		cfg = normalizeRuntimeConfig(cfg)
	}()
	func() {
		defer func() {
			if recover() != nil {
				cfg = config.Default()
			}
		}()
		cfg = config.LoadFromDir(dir)
	}()
	return normalizeRuntimeConfig(cfg)
}

func normalizeRuntimeConfig(cfg config.Config) config.Config {
	if strings.TrimSpace(cfg.LLM.APIKey) == "" {
		cfg.LLM.Provider = "mock"
		cfg.LLM.BaseURL = "mock://builtin"
	}
	return cfg
}

func LLMClientFromRuntimeConfig(cfg config.Config) llm.Client {
	if strings.TrimSpace(cfg.LLM.APIKey) == "" || strings.EqualFold(cfg.LLM.Provider, "mock") {
		return llm.NewMockClient()
	}
	return llm.NewClientFromConfig(cfg.LLM)
}
