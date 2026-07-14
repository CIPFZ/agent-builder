package runtime

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"charm.land/catwalk/pkg/catwalk"
	"github.com/CIPFZ/agent-builder/internal/config"
	"github.com/CIPFZ/agent-builder/internal/modelmeta"
)

func resolveDesktopLayout() (desktopLayout, error) {
	root := strings.TrimSpace(os.Getenv("AGENT_BUILDER_DESKTOP_ROOT"))
	if root == "" {
		exe, err := os.Executable()
		if err != nil {
			return desktopLayout{}, fmt.Errorf("failed to resolve executable path: %w", err)
		}
		root = filepath.Dir(exe)
	}
	root = filepath.Clean(root)
	layout := desktopLayout{
		Root:      root,
		ConfigDir: filepath.Join(root, "config"),
		DataDir:   filepath.Join(root, "data"),
		LogsDir:   filepath.Join(root, "logs"),
	}
	return layout, nil
}

func ensureDesktopLayout(layout desktopLayout) error {
	for _, dir := range []string{layout.ConfigDir, layout.DataDir, layout.LogsDir} {
		if err := os.MkdirAll(dir, 0o700); err != nil {
			return fmt.Errorf("failed to create desktop directory %s: %w", dir, err)
		}
	}
	return nil
}

// loadLocalModelMetadataCache reads discovered model metadata for the local
// provider from the desktop config database, keyed by model id. Missing
// database or table simply yields no metadata.
func validateModelConfig(local RuntimeModelConfig, requireModel bool) error {
	if local.Protocol != "openai" && local.Protocol != "anthropic" {
		return errors.New("protocol must be openai or anthropic")
	}
	if local.APIKey == "" || local.URL == "" || (requireModel && local.Model == "") {
		if requireModel {
			return errors.New("url, apiKey, and model are required")
		}
		return errors.New("url and apiKey are required")
	}
	return nil
}

func discoverModels(ctx context.Context, local RuntimeModelConfig) ([]RuntimeProviderModel, error) {
	if strings.TrimSpace(local.Protocol) == "" || strings.TrimSpace(local.URL) == "" || strings.TrimSpace(local.APIKey) == "" {
		return nil, errors.New("protocol, url, and apiKey are required to fetch models")
	}

	endpoint, headers, err := modelDiscoveryRequest(local)
	if err != nil {
		return nil, err
	}
	requestCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	req, err := http.NewRequestWithContext(requestCtx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create models request: %w", err)
	}
	for key, value := range headers {
		req.Header.Set(key, value)
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch models: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("failed to fetch models: %s", resp.Status)
	}

	var payload map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		return nil, fmt.Errorf("failed to decode models response: %w", err)
	}

	models := make([]RuntimeProviderModel, 0)
	models = append(models, modelsFromDiscoveryValue(payload["data"])...)
	models = append(models, modelsFromDiscoveryValue(payload["models"])...)
	if len(models) == 0 {
		return nil, errors.New("models response did not include any model ids")
	}
	return compactProviderModels(models), nil
}

func modelsFromDiscoveryValue(value any) []RuntimeProviderModel {
	items, ok := value.([]any)
	if !ok {
		return nil
	}
	models := make([]RuntimeProviderModel, 0, len(items))
	for _, item := range items {
		raw, ok := item.(map[string]any)
		if !ok {
			continue
		}
		model := RuntimeProviderModel{
			ID:              firstNonEmpty(stringFromAny(raw["id"]), stringFromAny(raw["name"]), stringFromAny(raw["display_name"])),
			DisplayName:     firstNonEmpty(stringFromAny(raw["display_name"]), stringFromAny(raw["name"])),
			ContextWindow:   firstPositiveIntFromMap(raw, "context_length", "contextLength", "max_input_tokens", "input_token_limit"),
			MaxOutputTokens: firstPositiveIntFromMap(raw, "max_output_tokens", "maxOutputTokens", "max_tokens", "output_token_limit"),
			Source:          modelmeta.SourceDiscovered,
		}
		if nested, ok := raw["model_info"].(map[string]any); ok {
			model.ContextWindow = firstPositiveModelInt(model.ContextWindow, firstPositiveIntFromMap(nested, "context_length", "contextLength", "max_input_tokens", "input_token_limit"))
			model.MaxOutputTokens = firstPositiveModelInt(model.MaxOutputTokens, firstPositiveIntFromMap(nested, "max_output_tokens", "maxOutputTokens", "max_tokens", "output_token_limit"))
		}
		if nested, ok := raw["details"].(map[string]any); ok {
			model.ContextWindow = firstPositiveModelInt(model.ContextWindow, firstPositiveIntFromMap(nested, "context_length", "contextLength", "max_input_tokens", "input_token_limit"))
			model.MaxOutputTokens = firstPositiveModelInt(model.MaxOutputTokens, firstPositiveIntFromMap(nested, "max_output_tokens", "maxOutputTokens", "max_tokens", "output_token_limit"))
		}
		if strings.TrimSpace(model.ID) != "" {
			models = append(models, model)
		}
	}
	return models
}

func stringFromAny(value any) string {
	switch typed := value.(type) {
	case string:
		return strings.TrimSpace(typed)
	default:
		return ""
	}
}

func firstPositiveIntFromMap(values map[string]any, keys ...string) int {
	for _, key := range keys {
		if value := intFromAny(values[key]); value > 0 {
			return value
		}
	}
	return 0
}

func firstPositiveModelInt(values ...int) int {
	for _, value := range values {
		if value > 0 {
			return value
		}
	}
	return 0
}

func intFromAny(value any) int {
	switch typed := value.(type) {
	case float64:
		return int(typed)
	case int:
		return typed
	case json.Number:
		parsed, _ := typed.Int64()
		return int(parsed)
	case string:
		var parsed int
		if _, err := fmt.Sscanf(strings.TrimSpace(typed), "%d", &parsed); err == nil {
			return parsed
		}
	}
	return 0
}

func modelDiscoveryRequest(local RuntimeModelConfig) (string, map[string]string, error) {
	baseURL := normalizeProviderAPIV1BaseURL(local.Protocol, local.URL)
	if _, err := url.ParseRequestURI(baseURL); err != nil {
		return "", nil, fmt.Errorf("invalid model base url: %w", err)
	}
	headers := map[string]string{}
	switch local.Protocol {
	case "openai":
		headers["Authorization"] = "Bearer " + local.APIKey
		return strings.TrimRight(baseURL, "/") + "/models", headers, nil
	case "anthropic":
		headers["x-api-key"] = local.APIKey
		headers["anthropic-version"] = "2023-06-01"
		return strings.TrimRight(baseURL, "/") + "/models", headers, nil
	default:
		return "", nil, errors.New("protocol must be openai or anthropic")
	}
}

func mergeModelIDs(groups ...[]string) []string {
	models := make([]string, 0)
	for _, group := range groups {
		models = append(models, group...)
	}
	return compactModelIDs(models)
}

func compactModelIDs(models []string) []string {
	seen := map[string]bool{}
	compact := make([]string, 0, len(models))
	for _, model := range models {
		model = strings.TrimSpace(model)
		if model == "" || seen[model] {
			continue
		}
		seen[model] = true
		compact = append(compact, model)
	}
	return compact
}

// applyModelConfig registers the local model provider. The catwalk catalog
// and any cached discovered metadata are passed in so the modelmeta
// resolution chain can use its discovered/builtin tiers instead of always
// falling back to the default window.
func applyModelConfig(store *config.ConfigStore, local RuntimeModelConfig, catalog []catwalk.Model, metadata map[string]RuntimeProviderModel) {
	if store.Config().Options == nil {
		store.Config().Options = &config.Options{}
	}
	autoLSP := false
	store.Config().Options.AutoLSP = &autoLSP

	providerType := catwalk.TypeOpenAICompat
	if local.Protocol == "anthropic" {
		providerType = catwalk.TypeAnthropic
	}

	modelIDs := local.Models
	if len(modelIDs) == 0 && local.Model != "" {
		modelIDs = []string{local.Model}
	}
	models := make([]catwalk.Model, 0, len(modelIDs))
	for _, model := range modelIDs {
		if model == "" {
			continue
		}
		cached := metadata[model]
		limits := modelmeta.Resolve(modelmeta.ResolveRequest{
			ProviderID:                localProviderID,
			ModelID:                   model,
			DiscoveredContextWindow:   cached.ContextWindow,
			DiscoveredMaxOutputTokens: cached.MaxOutputTokens,
			Catalog:                   catalog,
		})
		models = append(models, catwalk.Model{
			ID:               model,
			Name:             model,
			ContextWindow:    int64(limits.ContextWindow),
			DefaultMaxTokens: int64(limits.MaxOutputTokens),
		})
	}
	if len(models) == 0 {
		return
	}

	baseURL := normalizeModelBaseURL(local.Protocol, local.URL)
	store.Config().Providers.Set(localProviderID, config.ProviderConfig{
		ID:      localProviderID,
		Name:    "Local Model",
		BaseURL: baseURL,
		Type:    providerType,
		APIKey:  local.APIKey,
		Models:  models,
	})

	selectedID := strings.TrimSpace(local.Model)
	if selectedID == "" || !slices.ContainsFunc(models, func(model catwalk.Model) bool {
		return model.ID == selectedID
	}) {
		selectedID = models[0].ID
	}

	selectedModel := models[0]
	for _, model := range models {
		if model.ID == selectedID {
			selectedModel = model
			break
		}
	}
	selected := config.SelectedModel{
		Provider:  localProviderID,
		Model:     selectedModel.ID,
		MaxTokens: selectedModel.DefaultMaxTokens,
	}
	store.Config().Models[config.SelectedModelTypeLarge] = selected
	store.Config().Models[config.SelectedModelTypeSmall] = selected
}

func normalizeModelBaseURL(protocol, rawURL string) string {
	trimmed := strings.TrimRight(strings.TrimSpace(rawURL), "/")
	if protocol == "openai" && !strings.HasSuffix(trimmed, "/v1") {
		return trimmed + "/v1"
	}
	if protocol == "anthropic" {
		return strings.TrimSuffix(trimmed, "/v1")
	}
	return trimmed
}

// normalizeProviderAPIV1BaseURL returns the base used by direct REST calls.
// Anthropic SDKs expect the service root and append /v1 themselves, while
// discovery and connection probes address /v1 endpoints directly.
func normalizeProviderAPIV1BaseURL(protocol, rawURL string) string {
	trimmed := strings.TrimRight(strings.TrimSpace(rawURL), "/")
	if (protocol == "openai" || protocol == "anthropic") && !strings.HasSuffix(trimmed, "/v1") {
		return trimmed + "/v1"
	}
	return trimmed
}

func logConfiguredModel(store *config.ConfigStore) {
	selected := store.Config().Models[config.SelectedModelTypeLarge]
	provider, _ := store.Config().Providers.Get(selected.Provider)
	slog.Info(
		"Desktop model configured",
		"provider", selected.Provider,
		"protocol", string(provider.Type),
		"model", selected.Model,
		"base_url", provider.BaseURL,
		"has_api_key", provider.APIKey != "",
		"has_proxy", hasDesktopProxy(),
	)
}

func hasDesktopProxy() bool {
	return os.Getenv("HTTP_PROXY") != "" ||
		os.Getenv("HTTPS_PROXY") != "" ||
		os.Getenv("http_proxy") != "" ||
		os.Getenv("https_proxy") != ""
}
