package runtime

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"

	"charm.land/catwalk/pkg/catwalk"
	"github.com/charmbracelet/crush/internal/config"
)

var errSelectedModelMissing = errors.New("model is not selected. Configure a provider and select a model before chatting.")

type runtimeSelectedModelStore struct {
	db *sql.DB
}

func newRuntimeSelectedModelStore(db *sql.DB) runtimeSelectedModelStore {
	return runtimeSelectedModelStore{db: db}
}

func (s runtimeSelectedModelStore) Get(ctx context.Context, scope, projectID, sessionID string) (RuntimeSelectedModel, error) {
	if s.db == nil {
		return RuntimeSelectedModel{}, errors.New("selected model database is not available")
	}
	scope = normalizeSelectedModelScope(scope)
	row := s.db.QueryRowContext(ctx, `
SELECT id, configured_provider_id, provider_id, model, scope, project_id, session_id, created_at, updated_at
FROM selected_models
WHERE scope = ? AND COALESCE(project_id, '') = ? AND COALESCE(session_id, '') = ?
ORDER BY updated_at DESC
LIMIT 1`, scope, strings.TrimSpace(projectID), strings.TrimSpace(sessionID))
	selected, err := scanSelectedModel(row)
	if errors.Is(err, sql.ErrNoRows) {
		return RuntimeSelectedModel{}, errSelectedModelMissing
	}
	return selected, err
}

func (s runtimeSelectedModelStore) Upsert(ctx context.Context, req RuntimeSelectedModelRequest, provider RuntimeConfiguredProvider) (RuntimeSelectedModel, error) {
	if s.db == nil {
		return RuntimeSelectedModel{}, errors.New("selected model database is not available")
	}
	selected := RuntimeSelectedModel{
		ConfiguredProviderID: strings.TrimSpace(req.ConfiguredProviderID),
		ProviderID:           strings.TrimSpace(provider.ProviderID),
		Model:                strings.TrimSpace(req.Model),
		Scope:                normalizeSelectedModelScope(req.Scope),
		ProjectID:            strings.TrimSpace(req.ProjectID),
		SessionID:            strings.TrimSpace(req.SessionID),
	}
	if selected.ConfiguredProviderID == "" {
		return RuntimeSelectedModel{}, errors.New("configuredProviderId is required")
	}
	if selected.ProviderID == "" {
		return RuntimeSelectedModel{}, errors.New("providerId is required")
	}
	if selected.Model == "" {
		return RuntimeSelectedModel{}, errors.New("model is required")
	}
	if selected.Scope == "project" && selected.ProjectID == "" {
		return RuntimeSelectedModel{}, errors.New("projectId is required for project scope")
	}
	if selected.Scope == "session" && selected.SessionID == "" {
		return RuntimeSelectedModel{}, errors.New("sessionId is required for session scope")
	}
	selected.ID = selectedModelID(selected.Scope, selected.ProjectID, selected.SessionID)
	now := time.Now().UTC().UnixMilli()
	selected.CreatedAt = now
	selected.UpdatedAt = now
	_, err := s.db.ExecContext(ctx, `
INSERT INTO selected_models (
    id, configured_provider_id, provider_id, model, scope, project_id, session_id, created_at, updated_at
) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)
ON CONFLICT(id) DO UPDATE SET
    configured_provider_id = excluded.configured_provider_id,
    provider_id = excluded.provider_id,
    model = excluded.model,
    scope = excluded.scope,
    project_id = excluded.project_id,
    session_id = excluded.session_id,
    updated_at = excluded.updated_at`,
		selected.ID,
		selected.ConfiguredProviderID,
		selected.ProviderID,
		selected.Model,
		selected.Scope,
		nullableString(selected.ProjectID),
		nullableString(selected.SessionID),
		selected.CreatedAt,
		selected.UpdatedAt,
	)
	if err != nil {
		return RuntimeSelectedModel{}, fmt.Errorf("failed to save selected model: %w", err)
	}
	return s.Get(ctx, selected.Scope, selected.ProjectID, selected.SessionID)
}

func (s runtimeSelectedModelStore) DeleteForConfiguredProvider(ctx context.Context, configuredProviderID string) error {
	if s.db == nil {
		return errors.New("selected model database is not available")
	}
	_, err := s.db.ExecContext(ctx, `DELETE FROM selected_models WHERE configured_provider_id = ?`, strings.TrimSpace(configuredProviderID))
	if err != nil {
		return fmt.Errorf("failed to delete selected model: %w", err)
	}
	return nil
}

func scanSelectedModel(scanner interface {
	Scan(dest ...any) error
}) (RuntimeSelectedModel, error) {
	var selected RuntimeSelectedModel
	var projectID, sessionID sql.NullString
	if err := scanner.Scan(
		&selected.ID,
		&selected.ConfiguredProviderID,
		&selected.ProviderID,
		&selected.Model,
		&selected.Scope,
		&projectID,
		&sessionID,
		&selected.CreatedAt,
		&selected.UpdatedAt,
	); err != nil {
		return RuntimeSelectedModel{}, err
	}
	selected.ProjectID = projectID.String
	selected.SessionID = sessionID.String
	return selected, nil
}

func (r *runtimeService) SelectedModel(ctx context.Context) (RuntimeSelectedModelResponse, error) {
	selected, err := r.currentSelectedModel(ctx)
	if err != nil {
		return RuntimeSelectedModelResponse{}, err
	}
	return RuntimeSelectedModelResponse{SelectedModel: selected}, nil
}

func (r *runtimeService) SaveSelectedModel(ctx context.Context, req RuntimeSelectedModelRequest) (RuntimeSelectedModelResponse, error) {
	store, providerStore, err := r.selectedModelStores(ctx)
	if err != nil {
		return RuntimeSelectedModelResponse{}, err
	}
	provider, _, err := providerStore.GetConfiguredSecret(ctx, req.ConfiguredProviderID)
	if err != nil {
		return RuntimeSelectedModelResponse{}, err
	}
	selected, err := store.Upsert(ctx, req, provider)
	if err != nil {
		return RuntimeSelectedModelResponse{}, err
	}
	r.restart()
	return RuntimeSelectedModelResponse{SelectedModel: selected}, nil
}

func (r *runtimeService) selectedModelStores(ctx context.Context) (runtimeSelectedModelStore, runtimeProviderSettingsStore, error) {
	conn, err := r.configDB(ctx)
	if err != nil {
		return runtimeSelectedModelStore{}, runtimeProviderSettingsStore{}, err
	}
	return newRuntimeSelectedModelStore(conn), newRuntimeProviderSettingsStore(conn), nil
}

func (r *runtimeService) currentSelectedModel(ctx context.Context) (RuntimeSelectedModel, error) {
	store, providerStore, err := r.selectedModelStores(ctx)
	if err != nil {
		return RuntimeSelectedModel{}, err
	}
	selected, err := store.Get(ctx, "global", "", "")
	if err == nil {
		return selected, nil
	}
	if !errors.Is(err, errSelectedModelMissing) {
		return RuntimeSelectedModel{}, err
	}
	providers, listErr := providerStore.ListConfigured(ctx)
	if listErr != nil {
		return RuntimeSelectedModel{}, listErr
	}
	for _, provider := range providers {
		if strings.TrimSpace(provider.DefaultModel) == "" {
			continue
		}
		return store.Upsert(ctx, RuntimeSelectedModelRequest{
			ConfiguredProviderID: provider.ID,
			Model:                provider.DefaultModel,
			Scope:                "global",
		}, provider)
	}
	return RuntimeSelectedModel{}, errSelectedModelMissing
}

func (r *runtimeService) applySelectedConfiguredModel(ctx context.Context, store *config.ConfigStore) (RuntimeSelectedModel, localModelConfigResult, error) {
	selected, err := r.currentSelectedModel(ctx)
	if err != nil {
		return RuntimeSelectedModel{}, localModelConfigResult{}, err
	}
	providerStore, err := r.providerSettingsStore(ctx)
	if err != nil {
		return RuntimeSelectedModel{}, localModelConfigResult{}, err
	}
	provider, apiKey, err := providerStore.GetConfiguredSecret(ctx, selected.ConfiguredProviderID)
	if err != nil {
		return RuntimeSelectedModel{}, localModelConfigResult{}, err
	}
	if strings.TrimSpace(apiKey) == "" {
		return RuntimeSelectedModel{}, localModelConfigResult{}, errors.New("selected configured provider api key is required")
	}
	model := strings.TrimSpace(selected.Model)
	if model == "" {
		model = strings.TrimSpace(provider.DefaultModel)
	}
	if model == "" {
		return RuntimeSelectedModel{}, localModelConfigResult{}, errors.New("selected model is required")
	}
	applyConfiguredProviderModel(store, provider, apiKey, model)
	return selected, localModelConfigResult{
		Applied: true,
		Config: RuntimeModelConfig{
			Protocol: configuredProviderModelProtocol(provider),
			URL:      provider.APIEndpoint,
			APIKey:   apiKey,
			Model:    model,
			Models:   []string{model},
			Proxy:    provider.Proxy,
		},
	}, nil
}

func applyConfiguredProviderModel(store *config.ConfigStore, provider RuntimeConfiguredProvider, apiKey, modelID string) {
	if store.Config().Options == nil {
		store.Config().Options = &config.Options{}
	}
	autoLSP := false
	store.Config().Options.AutoLSP = &autoLSP
	protocol, baseURL := normalizeConfiguredProviderProtocolURL(provider)
	providerType := catwalk.TypeOpenAICompat
	if protocol == "anthropic" {
		providerType = catwalk.TypeAnthropic
	}
	model := catwalk.Model{
		ID:               modelID,
		Name:             modelID,
		ContextWindow:    64000,
		DefaultMaxTokens: 4096,
	}
	providerID := provider.ID
	store.Config().Providers.Set(providerID, config.ProviderConfig{
		ID:      providerID,
		Name:    firstNonEmpty(provider.Name, provider.ProviderID, provider.ID),
		BaseURL: baseURL,
		Type:    providerType,
		APIKey:  apiKey,
		Models:  []catwalk.Model{model},
	})
	selected := config.SelectedModel{
		Provider:  providerID,
		Model:     model.ID,
		MaxTokens: model.DefaultMaxTokens,
	}
	store.Config().Models[config.SelectedModelTypeLarge] = selected
	store.Config().Models[config.SelectedModelTypeSmall] = selected
}

func configuredProviderModelProtocol(provider RuntimeConfiguredProvider) string {
	if provider.Protocol == "anthropic" {
		return "anthropic"
	}
	return "openai"
}

func normalizeSelectedModelScope(scope string) string {
	switch strings.TrimSpace(scope) {
	case "project", "session":
		return strings.TrimSpace(scope)
	default:
		return "global"
	}
}

func selectedModelID(scope, projectID, sessionID string) string {
	scope = normalizeSelectedModelScope(scope)
	switch scope {
	case "project":
		return "project:" + strings.TrimSpace(projectID)
	case "session":
		return "session:" + strings.TrimSpace(sessionID)
	default:
		return "global"
	}
}
