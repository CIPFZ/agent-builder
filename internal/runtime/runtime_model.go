package runtime

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"

	"github.com/charmbracelet/crush/internal/config"
	"github.com/charmbracelet/crush/internal/csync"
)

const localProviderID = "local-model"

var errModelConfigMissing = errors.New("model is not configured. Open model settings and save protocol, URL, API key, and model before chatting.")

func (r *runtimeService) Models(ctx context.Context) (RuntimeModelsResponse, error) {
	configured, configuredErr := r.configuredProviderModels(ctx)
	if configuredErr == nil && len(configured) > 0 {
		return RuntimeModelsResponse{Models: configured}, nil
	}
	if err := r.ensureStarted(ctx); err != nil {
		if errors.Is(err, errModelConfigMissing) {
			cfg, cfgErr := r.readConfiguredModel()
			if cfgErr != nil {
				return RuntimeModelsResponse{}, err
			}
			return RuntimeModelsResponse{Models: []RuntimeModel{{
				ID:       cfg.Model,
				Name:     cfg.Model,
				Provider: localProviderID,
				Selected: true,
			}}}, nil
		}
		return RuntimeModelsResponse{}, err
	}

	r.mu.Lock()
	ws := *r.workspace
	r.mu.Unlock()

	workspace, err := r.runtime.GetWorkspace(ws.ID)
	if err != nil {
		return RuntimeModelsResponse{}, err
	}

	selected := workspace.Cfg.Config().Models[config.SelectedModelTypeLarge]
	models := make([]RuntimeModel, 0)
	for _, provider := range workspace.Cfg.Config().EnabledProviders() {
		for _, model := range provider.Models {
			models = append(models, RuntimeModel{
				ID:       model.ID,
				Name:     firstNonEmpty(model.Name, model.ID),
				Provider: provider.ID,
				Selected: selected.Provider == provider.ID && selected.Model == model.ID,
			})
		}
	}

	return RuntimeModelsResponse{Models: models}, nil
}

func (r *runtimeService) configuredProviderModels(ctx context.Context) ([]RuntimeModel, error) {
	store, providerStore, err := r.selectedModelStores(ctx)
	if err != nil {
		return nil, err
	}
	selected, selectedErr := store.Get(ctx, "global", "", "")
	if selectedErr != nil && !errors.Is(selectedErr, errSelectedModelMissing) {
		return nil, selectedErr
	}
	providers, err := providerStore.ListConfigured(ctx)
	if err != nil {
		return nil, err
	}
	models := make([]RuntimeModel, 0, len(providers))
	for _, provider := range providers {
		providerModels := configuredProviderModelIDs(provider)
		if len(providerModels) == 0 {
			continue
		}
		for _, model := range providerModels {
			models = append(models, RuntimeModel{
				ID:                   provider.ID + ":" + model,
				Name:                 model,
				Provider:             firstNonEmpty(provider.Name, provider.ProviderID, provider.ID),
				ProviderID:           provider.ProviderID,
				ConfiguredProviderID: provider.ID,
				ConfiguredProvider:   provider.Name,
				Selected: selectedErr == nil &&
					selected.ConfiguredProviderID == provider.ID &&
					selected.Model == model,
			})
		}
	}
	if selectedErr == nil {
		for i := range models {
			if models[i].Selected {
				return models, nil
			}
		}
	}
	if errors.Is(selectedErr, errSelectedModelMissing) && len(models) > 0 {
		models[0].Selected = true
	}
	return models, nil
}

func configuredProviderModelIDs(provider RuntimeConfiguredProvider) []string {
	return compactModelIDs(append(provider.Models, strings.TrimSpace(provider.DefaultModel)))
}

func (r *runtimeService) GetModelConfig(ctx context.Context) (RuntimeConfigResponse, error) {
	layout, err := resolveDesktopLayout()
	if err != nil {
		return RuntimeConfigResponse{}, err
	}
	if err := ensureDesktopLayout(layout); err != nil {
		return RuntimeConfigResponse{}, err
	}

	local, result := loadLocalModelConfig(layout)
	if result.Error != nil {
		return RuntimeConfigResponse{}, result.Error
	}
	if !result.Applied {
		local = RuntimeModelConfig{
			Protocol: "openai",
		}
	}
	hasAPIKey := result.Applied && local.APIKey != ""
	local.APIKey = ""
	local.HasAPIKey = hasAPIKey
	local.ConfigPath = layout.ModelConfigPath

	return RuntimeConfigResponse{Config: local}, nil
}

func (r *runtimeService) SaveModelConfig(ctx context.Context, req RuntimeModelConfig) (RuntimeConfigResponse, error) {
	layout, err := resolveDesktopLayout()
	if err != nil {
		return RuntimeConfigResponse{}, err
	}

	current, result := loadLocalModelConfig(layout)
	if result.Error != nil {
		return RuntimeConfigResponse{}, result.Error
	}

	next := RuntimeModelConfig{
		Protocol: strings.TrimSpace(req.Protocol),
		URL:      strings.TrimSpace(req.URL),
		APIKey:   strings.TrimSpace(req.APIKey),
		Model:    strings.TrimSpace(req.Model),
		Proxy:    strings.TrimSpace(req.Proxy),
	}
	if next.APIKey == "" {
		next.APIKey = current.APIKey
	}
	discovered, discoverErr := discoverModelIDs(ctx, next)
	if discoverErr == nil && len(discovered) > 0 {
		next.Models = discovered
	} else if next.Model != "" {
		next.Models = mergeModelIDs([]string{next.Model}, req.Models, current.Models)
	}
	if err := validateModelConfig(next, true); err != nil {
		return RuntimeConfigResponse{}, err
	}

	if err := saveLocalModelConfig(layout, next); err != nil {
		return RuntimeConfigResponse{}, err
	}

	r.restart()

	next.APIKey = ""
	next.HasAPIKey = true
	next.ConfigPath = layout.ModelConfigPath
	return RuntimeConfigResponse{Config: next}, nil
}

func (r *runtimeService) DiscoverModelConfig(ctx context.Context, req RuntimeModelConfig) (RuntimeModelDiscoveryResponse, error) {
	layout, err := resolveDesktopLayout()
	if err != nil {
		return RuntimeModelDiscoveryResponse{}, err
	}
	current, result := loadLocalModelConfig(layout)
	if result.Error != nil {
		return RuntimeModelDiscoveryResponse{}, result.Error
	}
	cfg := RuntimeModelConfig{
		Protocol: strings.TrimSpace(req.Protocol),
		URL:      strings.TrimSpace(req.URL),
		APIKey:   strings.TrimSpace(req.APIKey),
		Model:    strings.TrimSpace(req.Model),
		Proxy:    strings.TrimSpace(req.Proxy),
	}
	if cfg.APIKey == "" {
		cfg.APIKey = current.APIKey
	}
	if err := validateModelConfig(cfg, false); err != nil {
		return RuntimeModelDiscoveryResponse{}, err
	}
	models, err := discoverModelIDs(ctx, cfg)
	if err != nil {
		return RuntimeModelDiscoveryResponse{
			Protocol: cfg.Protocol,
			Model:    cfg.Model,
			Error:    err.Error(),
		}, nil
	}
	selected := cfg.Model
	if selected != "" && !slices.Contains(models, selected) {
		selected = ""
	}
	return RuntimeModelDiscoveryResponse{
		Protocol: cfg.Protocol,
		Model:    selected,
		Models:   models,
	}, nil
}

func (r *runtimeService) VerifyModelConfig(ctx context.Context, req RuntimeModelConfig) (RuntimeModelVerifyResponse, error) {
	layout, err := resolveDesktopLayout()
	if err != nil {
		return RuntimeModelVerifyResponse{}, err
	}
	current, result := loadLocalModelConfig(layout)
	if result.Error != nil {
		return RuntimeModelVerifyResponse{}, result.Error
	}
	cfg := RuntimeModelConfig{
		Protocol: strings.TrimSpace(req.Protocol),
		URL:      strings.TrimSpace(req.URL),
		APIKey:   strings.TrimSpace(req.APIKey),
		Model:    strings.TrimSpace(req.Model),
		Proxy:    strings.TrimSpace(req.Proxy),
		Models:   req.Models,
	}
	if cfg.APIKey == "" {
		cfg.APIKey = current.APIKey
	}
	discovered, discoverErr := discoverModelIDs(ctx, cfg)
	if discoverErr == nil && len(discovered) > 0 {
		cfg.Models = discovered
		if cfg.Model == "" || !slices.Contains(cfg.Models, cfg.Model) {
			cfg.Model = cfg.Models[0]
		}
	} else if cfg.Model != "" {
		cfg.Models = mergeModelIDs([]string{cfg.Model}, req.Models)
	}
	if err := validateModelConfig(cfg, true); err != nil {
		return RuntimeModelVerifyResponse{}, err
	}
	store := config.NewTestStore(&config.Config{
		Providers: csync.NewMap[string, config.ProviderConfig](),
		Models:    map[config.SelectedModelType]config.SelectedModel{},
		Options:   &config.Options{},
	})
	applyModelConfig(store, cfg)
	provider, ok := store.Config().Providers.Get(localProviderID)
	if !ok {
		return RuntimeModelVerifyResponse{}, errors.New("model provider was not configured")
	}
	if err := provider.TestConnection(store.Resolver()); err != nil {
		return RuntimeModelVerifyResponse{
			OK:       false,
			Protocol: cfg.Protocol,
			Model:    cfg.Model,
			Models:   cfg.Models,
			Error:    err.Error(),
		}, nil
	}
	if discoverErr != nil {
		return RuntimeModelVerifyResponse{
			OK:       false,
			Protocol: cfg.Protocol,
			Model:    cfg.Model,
			Models:   cfg.Models,
			Error:    fmt.Sprintf("connected, but failed to fetch models: %v", discoverErr),
		}, nil
	}
	return RuntimeModelVerifyResponse{
		OK:       true,
		Protocol: cfg.Protocol,
		Model:    cfg.Model,
		Models:   cfg.Models,
	}, nil
}

func (r *runtimeService) readConfiguredModel() (RuntimeModelConfig, error) {
	layout, err := resolveDesktopLayout()
	if err != nil {
		return RuntimeModelConfig{}, err
	}
	local, result := loadLocalModelConfig(layout)
	if result.Error != nil {
		return RuntimeModelConfig{}, result.Error
	}
	if !result.Applied {
		return RuntimeModelConfig{}, errModelConfigMissing
	}
	return local, nil
}
