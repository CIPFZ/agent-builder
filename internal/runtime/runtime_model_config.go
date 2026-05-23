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
	"github.com/charmbracelet/crush/internal/config"
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
	layout.ModelConfigPath = filepath.Join(layout.ConfigDir, "model.json")
	layout.SkillConfigPath = filepath.Join(layout.ConfigDir, "skills.json")
	layout.MCPConfigPath = filepath.Join(layout.ConfigDir, "mcp.json")
	layout.PolicyConfigPath = filepath.Join(layout.ConfigDir, "policy.json")
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

func loadLocalModelConfig(layout desktopLayout) (RuntimeModelConfig, localModelConfigResult) {
	result := localModelConfigResult{}
	path := filepath.Clean(layout.ModelConfigPath)
	result.CheckedPaths = append(result.CheckedPaths, path)
	data, err := os.ReadFile(path)
	if err != nil {
		return RuntimeModelConfig{}, result
	}

	var local RuntimeModelConfig
	if err := json.Unmarshal(data, &local); err != nil {
		result.Error = fmt.Errorf("failed to parse local model config %s: %w", path, err)
		return RuntimeModelConfig{}, result
	}
	if local.Model == "" && len(local.Models) > 0 {
		local.Model = local.Models[0]
	}
	if local.Model != "" && len(local.Models) == 0 {
		local.Models = []string{local.Model}
	}
	if err := validateModelConfig(local, false); err != nil {
		result.Error = fmt.Errorf("invalid local model config %s: %w", path, err)
		return RuntimeModelConfig{}, result
	}

	result.Applied = true
	result.Path = path
	result.Config = local
	return local, result
}

func applyLocalModelConfig(store *config.ConfigStore, layout desktopLayout) localModelConfigResult {
	local, result := loadLocalModelConfig(layout)
	if result.Error != nil || !result.Applied {
		return result
	}
	applyModelConfig(store, local)
	return result
}

func applyDesktopProxy(result localModelConfigResult) {
	if !result.Applied {
		return
	}

	proxy := strings.TrimSpace(result.Config.Proxy)
	if proxy == "" {
		_ = os.Unsetenv("HTTP_PROXY")
		_ = os.Unsetenv("HTTPS_PROXY")
		_ = os.Unsetenv("http_proxy")
		_ = os.Unsetenv("https_proxy")
		return
	}

	_ = os.Setenv("HTTP_PROXY", proxy)
	_ = os.Setenv("HTTPS_PROXY", proxy)
	_ = os.Setenv("http_proxy", proxy)
	_ = os.Setenv("https_proxy", proxy)
}

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

func saveLocalModelConfig(layout desktopLayout, local RuntimeModelConfig) error {
	if err := ensureDesktopLayout(layout); err != nil {
		return err
	}
	local.ConfigPath = ""
	local.HasAPIKey = false
	if local.Model != "" && len(local.Models) == 0 {
		local.Models = []string{local.Model}
	}
	data, err := json.MarshalIndent(local, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to encode local model config: %w", err)
	}
	data = append(data, '\n')
	if err := os.WriteFile(layout.ModelConfigPath, data, 0o600); err != nil {
		return fmt.Errorf("failed to write local model config: %w", err)
	}
	return nil
}

func discoverModelIDs(ctx context.Context, local RuntimeModelConfig) ([]string, error) {
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

	var payload struct {
		Data []struct {
			ID          string `json:"id"`
			DisplayName string `json:"display_name"`
		} `json:"data"`
		Models []struct {
			ID   string `json:"id"`
			Name string `json:"name"`
		} `json:"models"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		return nil, fmt.Errorf("failed to decode models response: %w", err)
	}

	models := make([]string, 0, len(payload.Data)+len(payload.Models))
	for _, model := range payload.Data {
		models = append(models, firstNonEmpty(strings.TrimSpace(model.ID), strings.TrimSpace(model.DisplayName)))
	}
	for _, model := range payload.Models {
		models = append(models, firstNonEmpty(strings.TrimSpace(model.ID), strings.TrimSpace(model.Name)))
	}
	models = compactModelIDs(models)
	if len(models) == 0 {
		return nil, errors.New("models response did not include any model ids")
	}
	return models, nil
}

func modelDiscoveryRequest(local RuntimeModelConfig) (string, map[string]string, error) {
	baseURL := normalizeModelBaseURL(local.Protocol, local.URL)
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

func applyModelConfig(store *config.ConfigStore, local RuntimeModelConfig) {
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
		models = append(models, catwalk.Model{
			ID:               model,
			Name:             model,
			ContextWindow:    64000,
			DefaultMaxTokens: 4096,
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
