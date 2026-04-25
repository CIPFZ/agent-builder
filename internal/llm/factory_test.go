package llm

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"myclaw/internal/config"
	"myclaw/internal/model"
	"myclaw/internal/prompt"
	"myclaw/internal/session"
)

func TestLoadProviderRoutingConfigFromMyclawConfigResolvesProfilesAndAgentRouting(t *testing.T) {
	cfgPath := filepath.Join(t.TempDir(), "myclaw.json")
	if err := os.WriteFile(cfgPath, []byte(`{
  "llm": {
    "default_profile": "claude-main",
    "providers": {
      "openai": {
        "protocol": "openai-compatible",
        "base_url": "https://openai.example/v1/chat/completions",
        "api_key": "openai-key",
        "enabled": true
      },
      "anthropic": {
        "protocol": "anthropic",
        "base_url": "https://anthropic.example/v1/messages",
        "api_key": "anthropic-key",
        "api_version": "2023-06-01",
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
}`), 0o600); err != nil {
		t.Fatalf("write config: %v", err)
	}

	t.Setenv("MYCLAW_CONFIG_FILE", cfgPath)

	settings, err := loadProviderRoutingConfig(config.LLMConfig{})
	if err != nil {
		t.Fatalf("load provider config: %v", err)
	}

	review, err := settings.ResolveProfile("review")
	if err != nil {
		t.Fatalf("resolve review profile: %v", err)
	}
	if review.Name != "review-gpt" || review.Provider.Protocol != ProtocolOpenAICompatible || review.Model != "gpt-5.1" {
		t.Fatalf("review profile = %#v, want review-gpt on openai-compatible/gpt-5.1", review)
	}

	worker, err := settings.ResolveProfile("worker")
	if err != nil {
		t.Fatalf("resolve worker profile: %v", err)
	}
	if worker.Name != "claude-main" || worker.Provider.Protocol != ProtocolAnthropic || worker.Model != "claude-sonnet-4-5" {
		t.Fatalf("worker profile = %#v, want default claude-main on anthropic/claude-sonnet-4-5", worker)
	}
}

func TestLoadProviderRoutingConfigEnvironmentOverridesProviderSecrets(t *testing.T) {
	cfgPath := filepath.Join(t.TempDir(), "myclaw.json")
	if err := os.WriteFile(cfgPath, []byte(`{
  "llm": {
    "proxy": {
      "enabled": true,
      "url": "http://global-proxy.example:8080"
    },
    "default_profile": "claude-main",
    "providers": {
      "anthropic": {
        "protocol": "anthropic",
        "base_url": "https://anthropic.example/v1/messages",
        "api_key": "file-key",
        "api_version": "2023-06-01",
        "enabled": true,
        "proxy": {
          "enabled": true,
          "url": "socks5://provider-proxy.example:1080"
        }
      }
    },
    "profiles": {
      "claude-main": {
        "provider": "anthropic",
        "model": "claude-sonnet-4-5"
      }
    }
  }
}`), 0o600); err != nil {
		t.Fatalf("write config: %v", err)
	}

	t.Setenv("MYCLAW_CONFIG_FILE", cfgPath)
	t.Setenv("MYCLAW_LLM_PROVIDER_ANTHROPIC_API_KEY", "env-key")
	t.Setenv("MYCLAW_LLM_PROVIDER_ANTHROPIC_BASE_URL", "https://override.example/v1/messages")
	t.Setenv("MYCLAW_LLM_PROXY__URL", "http://env-global-proxy.example:8888")
	t.Setenv("MYCLAW_LLM_PROVIDER_ANTHROPIC_PROXY__URL", "socks5://env-provider-proxy.example:1081")

	settings, err := loadProviderRoutingConfig(config.LLMConfig{})
	if err != nil {
		t.Fatalf("load provider config: %v", err)
	}
	resolved, err := settings.ResolveProfile("")
	if err != nil {
		t.Fatalf("resolve default profile: %v", err)
	}
	if resolved.Provider.APIKey != "env-key" {
		t.Fatalf("api key = %q, want env override", resolved.Provider.APIKey)
	}
	if resolved.Provider.BaseURL != "https://override.example/v1/messages" {
		t.Fatalf("base url = %q, want env override", resolved.Provider.BaseURL)
	}
	if resolved.Provider.Proxy.URL != "socks5://env-provider-proxy.example:1081" {
		t.Fatalf("proxy = %#v, want provider env override", resolved.Provider.Proxy)
	}
	if settings.GlobalProxy.URL != "http://env-global-proxy.example:8888" {
		t.Fatalf("global proxy = %#v, want env override", settings.GlobalProxy)
	}
}

func TestNewClientFromConfigRoutesRequestsByAgentType(t *testing.T) {
	var openAIPayload openAIChatRequest
	var anthropicPayload anthropicMessagesRequest

	openAIServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer r.Body.Close()
		if err := json.NewDecoder(r.Body).Decode(&openAIPayload); err != nil {
			t.Fatalf("decode openai payload: %v", err)
		}
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = io.WriteString(w, "data: [DONE]\n\n")
	}))
	defer openAIServer.Close()

	anthropicServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer r.Body.Close()
		if err := json.NewDecoder(r.Body).Decode(&anthropicPayload); err != nil {
			t.Fatalf("decode anthropic payload: %v", err)
		}
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = io.WriteString(w, "event: message_stop\n")
		_, _ = io.WriteString(w, "data: {\"type\":\"message_stop\"}\n\n")
	}))
	defer anthropicServer.Close()

	cfgPath := filepath.Join(t.TempDir(), "myclaw.json")
	if err := os.WriteFile(cfgPath, []byte(`{
  "llm": {
    "default_profile": "worker-openai",
    "providers": {
      "openai": {
        "protocol": "openai-compatible",
        "base_url": "`+openAIServer.URL+`",
        "api_key": "openai-key",
        "enabled": true
      },
      "anthropic": {
        "protocol": "anthropic",
        "base_url": "`+anthropicServer.URL+`",
        "api_key": "anthropic-key",
        "api_version": "2023-06-01",
        "enabled": true
      }
    },
    "profiles": {
      "worker-openai": {
        "provider": "openai",
        "model": "gpt-5.1"
      },
      "review-claude": {
        "provider": "anthropic",
        "model": "claude-sonnet-4-5"
      }
    },
    "routing": {
      "default_profile": "worker-openai",
      "agent_profiles": {
        "review": "review-claude"
      }
    }
  }
}`), 0o600); err != nil {
		t.Fatalf("write config: %v", err)
	}
	t.Setenv("MYCLAW_CONFIG_FILE", cfgPath)

	client := NewClientFromConfig(config.LLMConfig{})
	if err := client.Stream(t.Context(), GenerateRequest{
		Session: session.Session{
			ID:      "sess-1",
			Key:     "review",
			AgentID: "agent-review",
			Metadata: model.SessionMetadata{
				AgentType: "review",
			},
		},
		UserMessage: session.Message{ID: "user-1", Role: "user", Content: "review this", CreatedAt: time.Now().UTC()},
		Context:     prompt.Context{SystemPrompt: "system", UserInput: "review this"},
	}, discardStreamHandler{}); err != nil {
		t.Fatalf("review stream: %v", err)
	}
	if anthropicPayload.Model != "claude-sonnet-4-5" {
		t.Fatalf("anthropic model = %q, want review routing profile model", anthropicPayload.Model)
	}

	if err := client.Stream(t.Context(), GenerateRequest{
		Session: session.Session{
			ID:      "sess-2",
			Key:     "worker",
			AgentID: "agent-worker",
			Metadata: model.SessionMetadata{
				AgentType: "worker",
			},
		},
		UserMessage: session.Message{ID: "user-2", Role: "user", Content: "work", CreatedAt: time.Now().UTC()},
		Context:     prompt.Context{SystemPrompt: "system", UserInput: "work"},
	}, discardStreamHandler{}); err != nil {
		t.Fatalf("worker stream: %v", err)
	}
	if openAIPayload.Model != "gpt-5.1" {
		t.Fatalf("openai model = %q, want default worker profile model", openAIPayload.Model)
	}
}

func TestNewClientFromConfigWithoutAPIKeyReturnsUnavailableClient(t *testing.T) {
	client := NewClientFromConfig(config.LLMConfig{
		Provider: "openai-compatible",
		BaseURL:  "https://example.invalid/v1/chat/completions",
		Model:    "test-model",
	})

	if _, ok := client.(*UnavailableClient); !ok {
		t.Fatalf("client = %T, want *UnavailableClient", client)
	}
}
