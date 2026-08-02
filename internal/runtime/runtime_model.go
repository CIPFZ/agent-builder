package runtime

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"

	"github.com/CIPFZ/agent-builder/internal/config"
)

const localProviderID = "local-model"

var errModelConfigMissing = errors.New("model is not configured; open model settings and save protocol, URL, API key, and model before chatting")

func (r *runtimeService) Models(ctx context.Context) (RuntimeModelsResponse, error) {
	configured, configuredErr := r.configuredProviderModels(ctx)
	if configuredErr == nil && len(configured) > 0 {
		return RuntimeModelsResponse{Models: configured}, nil
	}
	if err := r.ensureStarted(ctx); err != nil {
		if errors.Is(err, errSelectedModelMissing) {
			return RuntimeModelsResponse{}, nil
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
	return compactModelIDs(append(providerModelIDs(provider.Models), strings.TrimSpace(provider.DefaultModel)))
}

func runtimeProviderModelsFromIDs(ids []string) []RuntimeProviderModel {
	models := make([]RuntimeProviderModel, 0, len(ids))
	for _, id := range compactModelIDs(ids) {
		models = append(models, RuntimeProviderModel{ID: id})
	}
	return models
}

func (r *runtimeService) GetModelConfig(ctx context.Context) (RuntimeConfigResponse, error) {
	providers, err := r.ConfiguredProviders(ctx)
	if err != nil {
		return RuntimeConfigResponse{}, err
	}
	if len(providers.Providers) == 0 {
		return RuntimeConfigResponse{Config: RuntimeModelConfig{Protocol: "openai"}}, nil
	}
	provider := providers.Providers[0]
	return RuntimeConfigResponse{Config: RuntimeModelConfig{Protocol: provider.Protocol, URL: provider.APIEndpoint, Model: provider.DefaultModel, Models: providerModelIDs(provider.Models), Proxy: provider.Proxy, HasAPIKey: provider.HasAPIKey}}, nil
}

func (r *runtimeService) SaveModelConfig(ctx context.Context, req RuntimeModelConfig) (RuntimeConfigResponse, error) {
	next := RuntimeModelConfig{
		Protocol: strings.TrimSpace(req.Protocol),
		URL:      strings.TrimSpace(req.URL),
		APIKey:   strings.TrimSpace(req.APIKey),
		Model:    strings.TrimSpace(req.Model),
		Proxy:    strings.TrimSpace(req.Proxy),
	}
	discovered, discoverErr := discoverModels(ctx, next)
	if discoverErr == nil && len(discovered) > 0 {
		next.Models = providerModelIDs(discovered)
	} else if next.Model != "" {
		next.Models = mergeModelIDs([]string{next.Model}, req.Models)
	}
	if err := validateModelConfig(next, true); err != nil {
		return RuntimeConfigResponse{}, err
	}

	_, err := r.SaveConfiguredProvider(ctx, RuntimeConfiguredProviderRequest{ProviderID: next.Protocol, Name: "Default Provider", Protocol: next.Protocol, APIEndpoint: next.URL, APIKey: next.APIKey, Proxy: next.Proxy, DefaultModel: next.Model, Models: runtimeProviderModelsFromIDs(next.Models), Enabled: true})
	if err != nil {
		return RuntimeConfigResponse{}, err
	}
	return r.GetModelConfig(ctx)
}

func (r *runtimeService) DiscoverModelConfig(ctx context.Context, req RuntimeModelConfig) (RuntimeModelDiscoveryResponse, error) {
	cfg := RuntimeModelConfig{
		Protocol: strings.TrimSpace(req.Protocol),
		URL:      strings.TrimSpace(req.URL),
		APIKey:   strings.TrimSpace(req.APIKey),
		Model:    strings.TrimSpace(req.Model),
		Proxy:    strings.TrimSpace(req.Proxy),
	}
	if err := validateModelConfig(cfg, false); err != nil {
		return RuntimeModelDiscoveryResponse{}, err
	}
	discovered, err := discoverModels(ctx, cfg)
	if err != nil {
		return RuntimeModelDiscoveryResponse{
			Protocol: cfg.Protocol,
			Model:    cfg.Model,
			Error:    err.Error(),
		}, nil
	}
	models := providerModelIDs(discovered)
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
	cfg := RuntimeModelConfig{
		Protocol: strings.TrimSpace(req.Protocol),
		URL:      strings.TrimSpace(req.URL),
		APIKey:   strings.TrimSpace(req.APIKey),
		Model:    strings.TrimSpace(req.Model),
		Proxy:    strings.TrimSpace(req.Proxy),
		Models:   req.Models,
	}
	discovered, discoverErr := discoverModels(ctx, cfg)
	if discoverErr == nil && len(discovered) > 0 {
		cfg.Models = providerModelIDs(discovered)
		if cfg.Model == "" || !slices.Contains(cfg.Models, cfg.Model) {
			cfg.Model = cfg.Models[0]
		}
	} else if cfg.Model != "" {
		cfg.Models = mergeModelIDs([]string{cfg.Model}, req.Models)
	}
	if err := validateModelConfig(cfg, true); err != nil {
		return RuntimeModelVerifyResponse{}, err
	}
	if err := testProviderConnection(ctx, RuntimeConfiguredProvider{
		Protocol:    cfg.Protocol,
		APIEndpoint: cfg.URL,
		Proxy:       cfg.Proxy,
	}, cfg.APIKey, cfg.Model); err != nil {
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
