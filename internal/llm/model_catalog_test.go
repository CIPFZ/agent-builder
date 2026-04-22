package llm

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"myclaw/internal/config"
)

func TestModelCatalogListModelsIncludesConfiguredAndDiscoveredOpenAIModels(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/models" {
			t.Fatalf("path = %q, want /v1/models", r.URL.Path)
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"data": []map[string]any{
				{
					"id":                "gpt-5",
					"owned_by":          "openai",
					"context_window":    400000,
					"max_output_tokens": 128000,
				},
				{
					"id":                "gpt-4.1-mini",
					"owned_by":          "openai",
					"context_window":    200000,
					"max_output_tokens": 32768,
				},
			},
		})
	}))
	defer server.Close()

	catalog := NewModelCatalogFromConfig(config.LLMConfig{
		ActiveProfile: "gpt-main",
		Providers: map[string]config.LLMProviderConfig{
			"openai": {
				Protocol:   "openai-compatible",
				BaseURL:    server.URL + "/v1/chat/completions",
				APIKey:     "test-key",
				Enabled:    true,
				TimeoutMs:  1000,
				MaxRetries: 0,
			},
		},
		Profiles: map[string]config.LLMProfileConfig{
			"gpt-main": {
				Provider:  "openai",
				Model:     "gpt-5",
				Streaming: true,
			},
		},
	})

	models, err := catalog.ListModels(context.Background())
	if err != nil {
		t.Fatalf("ListModels: %v", err)
	}
	if len(models) < 2 {
		t.Fatalf("models = %#v, want discovered models", models)
	}
	if models[0].ID != "gpt-5" {
		t.Fatalf("first model = %#v, want gpt-5 first", models[0])
	}
	if models[0].ContextWindowTokens != 400000 {
		t.Fatalf("gpt-5 = %#v, want context window metadata", models[0])
	}
	if models[0].ProfileName != "gpt-main" {
		t.Fatalf("gpt-5 = %#v, want active profile attached", models[0])
	}
}

func TestModelCatalogValidateModelUsesDiscoveredInventory(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{
			"data": []map[string]any{
				{"id": "MiniMax-M2.7", "context_window": 1000000, "max_output_tokens": 32000},
			},
		})
	}))
	defer server.Close()

	catalog := NewModelCatalogFromConfig(config.LLMConfig{
		ActiveProfile: "minimax-main",
		Providers: map[string]config.LLMProviderConfig{
			"minimax": {
				Protocol:   "anthropic",
				BaseURL:    server.URL + "/v1/messages",
				APIKey:     "test-key",
				Enabled:    true,
				TimeoutMs:  1000,
				MaxRetries: 0,
				APIVersion: "2023-06-01",
			},
		},
		Profiles: map[string]config.LLMProfileConfig{
			"minimax-main": {
				Provider:  "minimax",
				Model:     "MiniMax-M2.7",
				Streaming: true,
			},
		},
	})

	if err := catalog.ValidateModel(context.Background(), "MiniMax-M2.7"); err != nil {
		t.Fatalf("ValidateModel(MiniMax-M2.7): %v", err)
	}
	if err := catalog.ValidateModel(context.Background(), "missing-model"); err == nil {
		t.Fatal("ValidateModel(missing-model) = nil, want error")
	}
}

func TestModelCatalogFallsBackToConfiguredModelsWhenRemoteDiscoveryUnavailable(t *testing.T) {
	t.Parallel()

	catalog := NewModelCatalogFromConfig(config.LLMConfig{
		ActiveProfile: "worker",
		Providers: map[string]config.LLMProviderConfig{
			"custom": {
				Protocol:   "openai-compatible",
				BaseURL:    "https://example.invalid/v1/chat/completions",
				APIKey:     "test-key",
				Enabled:    true,
				TimeoutMs:  1,
				MaxRetries: 0,
			},
		},
		Profiles: map[string]config.LLMProfileConfig{
			"worker": {
				Provider:  "custom",
				Model:     "custom-coder",
				Streaming: true,
			},
		},
	})

	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()

	models, err := catalog.ListModels(ctx)
	if err != nil {
		t.Fatalf("ListModels: %v", err)
	}
	if len(models) != 1 || models[0].ID != "custom-coder" {
		t.Fatalf("models = %#v, want configured fallback model", models)
	}
}
