package runtime

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/CIPFZ/agent-builder/internal/modelmeta"
)

var (
	errConfiguredProviderNotFound  = errors.New("configured provider not found")
	errConfiguredProviderDuplicate = errors.New("configured provider name already exists")
)

type runtimeProviderSettingsStore struct {
	db *sql.DB
}

func newRuntimeProviderSettingsStore(db *sql.DB) runtimeProviderSettingsStore {
	return runtimeProviderSettingsStore{db: db}
}

func (s runtimeProviderSettingsStore) ensureConfiguredProviderColumns(ctx context.Context) error {
	if s.db == nil {
		return errors.New("provider settings database is not available")
	}
	if _, err := s.db.ExecContext(ctx, `ALTER TABLE configured_providers ADD COLUMN models_json TEXT NOT NULL DEFAULT '[]'`); err != nil {
		if !strings.Contains(strings.ToLower(err.Error()), "duplicate column") {
			return fmt.Errorf("failed to ensure configured provider models column: %w", err)
		}
	}
	if _, err := s.db.ExecContext(ctx, `ALTER TABLE configured_providers ADD COLUMN default_context_window INTEGER`); err != nil {
		if !strings.Contains(strings.ToLower(err.Error()), "duplicate column") {
			return fmt.Errorf("failed to ensure configured provider default context window column: %w", err)
		}
	}
	if _, err := s.db.ExecContext(ctx, `
CREATE TABLE IF NOT EXISTS model_metadata_cache (
    provider_id TEXT NOT NULL,
    model_id TEXT NOT NULL,
    context_window INTEGER,
    max_output_tokens INTEGER,
    source TEXT NOT NULL,
    fetched_at INTEGER NOT NULL,
    PRIMARY KEY (provider_id, model_id)
)`); err != nil {
		return fmt.Errorf("failed to ensure model metadata cache table: %w", err)
	}
	return nil
}

func (s runtimeProviderSettingsStore) SyncCatalog(ctx context.Context, providers []RuntimeProviderCatalogItem) error {
	if s.db == nil {
		return errors.New("provider settings database is not available")
	}
	now := time.Now().UTC().UnixMilli()
	for _, provider := range providers {
		if strings.TrimSpace(provider.ID) == "" {
			continue
		}
		if _, err := s.db.ExecContext(ctx, `
INSERT INTO provider_catalog (
    id, name, type, api_endpoint, api_key_template, model_count,
    default_large_model, default_small_model, required_fields_json, notes_json,
    configurable, created_at, updated_at
) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
ON CONFLICT(id) DO UPDATE SET
    name = excluded.name,
    type = excluded.type,
    api_endpoint = excluded.api_endpoint,
    api_key_template = excluded.api_key_template,
    model_count = excluded.model_count,
    default_large_model = excluded.default_large_model,
    default_small_model = excluded.default_small_model,
    required_fields_json = excluded.required_fields_json,
    notes_json = excluded.notes_json,
    configurable = excluded.configurable,
    updated_at = excluded.updated_at`,
			provider.ID,
			firstNonEmpty(provider.Name, provider.ID),
			provider.Type,
			nullableString(provider.APIEndpoint),
			nullableString(provider.APIKeyTemplate),
			provider.ModelCount,
			nullableString(provider.DefaultLargeModel),
			nullableString(provider.DefaultSmallModel),
			mustMarshalStringSlice(provider.RequiredFields),
			mustMarshalStringSlice(provider.Notes),
			boolInt(provider.Configurable),
			now,
			now,
		); err != nil {
			return fmt.Errorf("failed to sync provider catalog %s: %w", provider.ID, err)
		}
	}
	return nil
}

func (s runtimeProviderSettingsStore) ListCatalog(ctx context.Context) ([]RuntimeProviderCatalogItem, error) {
	if s.db == nil {
		return nil, errors.New("provider settings database is not available")
	}
	rows, err := s.db.QueryContext(ctx, `
SELECT id, name, type, api_endpoint, api_key_template, model_count,
       default_large_model, default_small_model, required_fields_json, notes_json,
       configurable
FROM provider_catalog
ORDER BY name ASC, id ASC`)
	if err != nil {
		return nil, fmt.Errorf("failed to list provider catalog: %w", err)
	}
	defer rows.Close() //nolint:errcheck

	var providers []RuntimeProviderCatalogItem
	for rows.Next() {
		var provider RuntimeProviderCatalogItem
		var apiEndpoint, apiKeyTemplate, defaultLargeModel, defaultSmallModel sql.NullString
		var requiredFieldsJSON, notesJSON string
		var configurable int
		if err := rows.Scan(
			&provider.ID,
			&provider.Name,
			&provider.Type,
			&apiEndpoint,
			&apiKeyTemplate,
			&provider.ModelCount,
			&defaultLargeModel,
			&defaultSmallModel,
			&requiredFieldsJSON,
			&notesJSON,
			&configurable,
		); err != nil {
			return nil, fmt.Errorf("failed to scan provider catalog: %w", err)
		}
		provider.APIEndpoint = apiEndpoint.String
		provider.APIKeyTemplate = apiKeyTemplate.String
		provider.DefaultLargeModel = defaultLargeModel.String
		provider.DefaultSmallModel = defaultSmallModel.String
		provider.RequiredFields = unmarshalStringSlice(requiredFieldsJSON)
		provider.Notes = unmarshalStringSlice(notesJSON)
		provider.Configurable = configurable != 0
		providers = append(providers, provider)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return providers, nil
}

func (s runtimeProviderSettingsStore) ListConfigured(ctx context.Context) ([]RuntimeConfiguredProvider, error) {
	if s.db == nil {
		return nil, errors.New("provider settings database is not available")
	}
	if err := s.ensureConfiguredProviderColumns(ctx); err != nil {
		return nil, err
	}
	rows, err := s.db.QueryContext(ctx, configuredProviderSelectSQL()+` ORDER BY updated_at DESC, name ASC`)
	if err != nil {
		return nil, fmt.Errorf("failed to list configured providers: %w", err)
	}
	defer rows.Close() //nolint:errcheck

	var providers []RuntimeConfiguredProvider
	for rows.Next() {
		provider, err := scanConfiguredProvider(rows)
		if err != nil {
			return nil, err
		}
		providers = append(providers, provider)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	if err := rows.Close(); err != nil {
		return nil, err
	}
	for i := range providers {
		provider, err := s.enrichConfiguredProviderModels(ctx, providers[i])
		if err != nil {
			return nil, err
		}
		providers[i] = provider
	}
	return providers, nil
}

func (s runtimeProviderSettingsStore) ListConfiguredSecrets(ctx context.Context) ([]RuntimeConfiguredProvider, error) {
	if s.db == nil {
		return nil, errors.New("provider settings database is not available")
	}
	if err := s.ensureConfiguredProviderColumns(ctx); err != nil {
		return nil, err
	}
	rows, err := s.db.QueryContext(ctx, configuredProviderSecretSelectSQL()+` ORDER BY updated_at DESC, name ASC`)
	if err != nil {
		return nil, fmt.Errorf("failed to list configured providers: %w", err)
	}
	defer rows.Close() //nolint:errcheck

	var providers []RuntimeConfiguredProvider
	for rows.Next() {
		provider, _, err := scanConfiguredProviderSecret(rows)
		if err != nil {
			return nil, err
		}
		providers = append(providers, provider)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	if err := rows.Close(); err != nil {
		return nil, err
	}
	for i := range providers {
		provider, err := s.enrichConfiguredProviderModels(ctx, providers[i])
		if err != nil {
			return nil, err
		}
		providers[i] = provider
	}
	return providers, nil
}

func (s runtimeProviderSettingsStore) UpsertConfigured(ctx context.Context, req RuntimeConfiguredProviderRequest) (RuntimeConfiguredProvider, error) {
	if s.db == nil {
		return RuntimeConfiguredProvider{}, errors.New("provider settings database is not available")
	}
	if err := s.ensureConfiguredProviderColumns(ctx); err != nil {
		return RuntimeConfiguredProvider{}, err
	}
	provider, err := normalizeConfiguredProviderRequest(req)
	if err != nil {
		return RuntimeConfiguredProvider{}, err
	}
	if provider.ID == "" {
		provider.ID = "provider_" + newRequestID()
	}
	now := time.Now().UTC().UnixMilli()
	if provider.CreatedAt == 0 {
		provider.CreatedAt = now
	}
	provider.UpdatedAt = now
	secretRef := strings.TrimSpace(provider.APIKeySecretRef)
	if secretRef == "" && strings.TrimSpace(req.APIKey) != "" {
		secretRef = "sqlite:" + provider.ID + ":api_key"
	}
	_, err = s.db.ExecContext(ctx, `
INSERT INTO configured_providers (
    id, provider_id, name, remark, protocol, api_endpoint, api_key, api_key_secret_ref,
    proxy, default_model, models_json, default_context_window, enabled, created_at, updated_at
) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
ON CONFLICT(id) DO UPDATE SET
    provider_id = excluded.provider_id,
    name = excluded.name,
    remark = excluded.remark,
    protocol = excluded.protocol,
    api_endpoint = excluded.api_endpoint,
    api_key = COALESCE(excluded.api_key, configured_providers.api_key),
    api_key_secret_ref = COALESCE(excluded.api_key_secret_ref, configured_providers.api_key_secret_ref),
    proxy = excluded.proxy,
    default_model = excluded.default_model,
    models_json = excluded.models_json,
    default_context_window = excluded.default_context_window,
    enabled = excluded.enabled,
    updated_at = excluded.updated_at`,
		provider.ID,
		provider.ProviderID,
		provider.Name,
		nullableString(provider.Remark),
		provider.Protocol,
		provider.APIEndpoint,
		nullableString(strings.TrimSpace(req.APIKey)),
		nullableString(secretRef),
		nullableString(provider.Proxy),
		nullableString(provider.DefaultModel),
		mustMarshalProviderModels(provider.Models),
		nullableInt(provider.DefaultContextWindow),
		boolInt(provider.Enabled),
		provider.CreatedAt,
		provider.UpdatedAt,
	)
	if err != nil {
		return RuntimeConfiguredProvider{}, fmt.Errorf("failed to save configured provider: %w", err)
	}
	return s.GetConfigured(ctx, provider.ID)
}

func (s runtimeProviderSettingsStore) GetConfigured(ctx context.Context, id string) (RuntimeConfiguredProvider, error) {
	if s.db == nil {
		return RuntimeConfiguredProvider{}, errors.New("provider settings database is not available")
	}
	if err := s.ensureConfiguredProviderColumns(ctx); err != nil {
		return RuntimeConfiguredProvider{}, err
	}
	row := s.db.QueryRowContext(ctx, configuredProviderSelectSQL()+` WHERE id = ?`, strings.TrimSpace(id))
	provider, err := scanConfiguredProvider(row)
	if errors.Is(err, sql.ErrNoRows) {
		return RuntimeConfiguredProvider{}, errConfiguredProviderNotFound
	}
	return provider, err
}

func (s runtimeProviderSettingsStore) DeleteConfigured(ctx context.Context, id string) error {
	if s.db == nil {
		return errors.New("provider settings database is not available")
	}
	result, err := s.db.ExecContext(ctx, `DELETE FROM configured_providers WHERE id = ?`, strings.TrimSpace(id))
	if err != nil {
		return fmt.Errorf("failed to delete configured provider: %w", err)
	}
	if rows, _ := result.RowsAffected(); rows == 0 {
		return errConfiguredProviderNotFound
	}
	return nil
}

func (r *runtimeService) ConfiguredProviders(ctx context.Context) (RuntimeConfiguredProvidersResponse, error) {
	store, err := r.providerSettingsStore(ctx)
	if err != nil {
		return RuntimeConfiguredProvidersResponse{}, err
	}
	providers, err := store.ListConfiguredSecrets(ctx)
	if err != nil {
		return RuntimeConfiguredProvidersResponse{}, err
	}
	return RuntimeConfiguredProvidersResponse{Providers: providers}, nil
}

func (r *runtimeService) SaveConfiguredProvider(ctx context.Context, req RuntimeConfiguredProviderRequest) (RuntimeConfiguredProviderResponse, error) {
	store, err := r.providerSettingsStore(ctx)
	if err != nil {
		return RuntimeConfiguredProviderResponse{}, err
	}
	if err := r.syncProviderCatalog(ctx, store); err != nil {
		return RuntimeConfiguredProviderResponse{}, err
	}
	if err := ensureConfiguredProviderNameIsUnique(ctx, store, req); err != nil {
		return RuntimeConfiguredProviderResponse{}, err
	}
	provider, err := store.UpsertConfigured(ctx, req)
	if err != nil {
		return RuntimeConfiguredProviderResponse{}, err
	}
	if err := r.syncSelectedModelAfterProviderSave(ctx, store, provider); err != nil {
		return RuntimeConfiguredProviderResponse{}, err
	}
	r.restart()
	provider, _, err = store.GetConfiguredSecret(ctx, provider.ID)
	if err != nil {
		return RuntimeConfiguredProviderResponse{}, err
	}
	return RuntimeConfiguredProviderResponse{Provider: provider}, nil
}

func ensureConfiguredProviderNameIsUnique(ctx context.Context, store runtimeProviderSettingsStore, req RuntimeConfiguredProviderRequest) error {
	candidate, err := normalizeConfiguredProviderRequest(req)
	if err != nil {
		return err
	}
	providers, err := store.ListConfigured(ctx)
	if err != nil {
		return err
	}
	candidateName := configuredProviderNameKey(candidate.Name)
	for _, existing := range providers {
		if existing.ID == candidate.ID {
			continue
		}
		if configuredProviderNameKey(existing.Name) == candidateName {
			return errConfiguredProviderDuplicate
		}
	}
	return nil
}

func (r *runtimeService) DeleteConfiguredProvider(ctx context.Context, id string) (RuntimeConfiguredProvidersResponse, error) {
	store, err := r.providerSettingsStore(ctx)
	if err != nil {
		return RuntimeConfiguredProvidersResponse{}, err
	}
	selectedStore := newRuntimeSelectedModelStore(store.db)
	if err := selectedStore.DeleteForConfiguredProvider(ctx, id); err != nil {
		return RuntimeConfiguredProvidersResponse{}, err
	}
	if err := store.DeleteConfigured(ctx, id); err != nil {
		return RuntimeConfiguredProvidersResponse{}, err
	}
	if err := r.ensureGlobalSelectedModel(ctx, store); err != nil {
		return RuntimeConfiguredProvidersResponse{}, err
	}
	r.restart()
	providers, err := store.ListConfiguredSecrets(ctx)
	if err != nil {
		return RuntimeConfiguredProvidersResponse{}, err
	}
	return RuntimeConfiguredProvidersResponse{Providers: providers}, nil
}

func (r *runtimeService) syncSelectedModelAfterProviderSave(ctx context.Context, store runtimeProviderSettingsStore, provider RuntimeConfiguredProvider) error {
	selectedStore := newRuntimeSelectedModelStore(store.db)
	selected, err := selectedStore.Get(ctx, "global", "", "")
	if errors.Is(err, errSelectedModelMissing) {
		if strings.TrimSpace(provider.DefaultModel) == "" {
			return nil
		}
		_, err = selectedStore.Upsert(ctx, RuntimeSelectedModelRequest{
			ConfiguredProviderID: provider.ID,
			Model:                provider.DefaultModel,
			Scope:                "global",
		}, provider)
		return err
	}
	if err != nil {
		return err
	}
	if selected.ConfiguredProviderID != provider.ID {
		return nil
	}
	if strings.TrimSpace(provider.DefaultModel) == "" {
		if err := selectedStore.DeleteForConfiguredProvider(ctx, provider.ID); err != nil {
			return err
		}
		return r.ensureGlobalSelectedModel(ctx, store)
	}
	if selected.Model == provider.DefaultModel {
		return nil
	}
	_, err = selectedStore.Upsert(ctx, RuntimeSelectedModelRequest{
		ConfiguredProviderID: provider.ID,
		Model:                provider.DefaultModel,
		Scope:                "global",
	}, provider)
	return err
}

func (r *runtimeService) ensureGlobalSelectedModel(ctx context.Context, store runtimeProviderSettingsStore) error {
	selectedStore := newRuntimeSelectedModelStore(store.db)
	if _, err := selectedStore.Get(ctx, "global", "", ""); err == nil {
		return nil
	} else if !errors.Is(err, errSelectedModelMissing) {
		return err
	}
	providers, err := store.ListConfigured(ctx)
	if err != nil {
		return err
	}
	for _, provider := range providers {
		if strings.TrimSpace(provider.DefaultModel) == "" {
			continue
		}
		_, err = selectedStore.Upsert(ctx, RuntimeSelectedModelRequest{
			ConfiguredProviderID: provider.ID,
			Model:                provider.DefaultModel,
			Scope:                "global",
		}, provider)
		return err
	}
	return nil
}

func (r *runtimeService) providerSettingsStore(ctx context.Context) (runtimeProviderSettingsStore, error) {
	conn, err := r.configDB(ctx)
	if err != nil {
		return runtimeProviderSettingsStore{}, err
	}
	return newRuntimeProviderSettingsStore(conn), nil
}

func (r *runtimeService) syncProviderCatalog(ctx context.Context, store runtimeProviderSettingsStore) error {
	return store.SyncCatalog(ctx, embeddedProviderCatalog())
}

func configuredProviderSelectSQL() string {
	return `
SELECT id, provider_id, name, remark, protocol, api_endpoint, api_key_secret_ref,
       proxy, default_model, models_json, default_context_window, enabled, created_at, updated_at
FROM configured_providers`
}

func configuredProviderSecretSelectSQL() string {
	return `
SELECT id, provider_id, name, remark, protocol, api_endpoint, api_key_secret_ref,
       proxy, default_model, models_json, default_context_window, enabled, created_at, updated_at, api_key
FROM configured_providers`
}

func scanConfiguredProvider(scanner interface {
	Scan(dest ...any) error
},
) (RuntimeConfiguredProvider, error) {
	var provider RuntimeConfiguredProvider
	var remark, secretRef, proxy, defaultModel sql.NullString
	var modelsJSON string
	var defaultContextWindow sql.NullInt64
	var enabled int
	if err := scanner.Scan(
		&provider.ID,
		&provider.ProviderID,
		&provider.Name,
		&remark,
		&provider.Protocol,
		&provider.APIEndpoint,
		&secretRef,
		&proxy,
		&defaultModel,
		&modelsJSON,
		&defaultContextWindow,
		&enabled,
		&provider.CreatedAt,
		&provider.UpdatedAt,
	); err != nil {
		return RuntimeConfiguredProvider{}, err
	}
	provider.Remark = remark.String
	provider.APIKeySecretRef = secretRef.String
	provider.HasAPIKey = secretRef.Valid && secretRef.String != ""
	provider.Proxy = proxy.String
	provider.DefaultModel = defaultModel.String
	provider.Models = compactProviderModels(append(unmarshalProviderModels(modelsJSON), RuntimeProviderModel{ID: provider.DefaultModel}))
	provider.DefaultContextWindow = int(defaultContextWindow.Int64)
	provider.Enabled = enabled != 0
	return provider, nil
}

func normalizeConfiguredProviderRequest(req RuntimeConfiguredProviderRequest) (RuntimeConfiguredProvider, error) {
	provider := RuntimeConfiguredProvider{
		ID:                   strings.TrimSpace(req.ID),
		ProviderID:           strings.TrimSpace(req.ProviderID),
		Name:                 strings.TrimSpace(req.Name),
		Remark:               strings.TrimSpace(req.Remark),
		Protocol:             strings.TrimSpace(req.Protocol),
		APIEndpoint:          strings.TrimSpace(req.APIEndpoint),
		APIKeySecretRef:      "",
		Proxy:                strings.TrimSpace(req.Proxy),
		DefaultModel:         strings.TrimSpace(req.DefaultModel),
		Models:               compactProviderModels(append(req.Models, RuntimeProviderModel{ID: strings.TrimSpace(req.DefaultModel)})),
		DefaultContextWindow: req.DefaultContextWindow,
		Enabled:              true,
	}
	if provider.ProviderID == "" {
		return RuntimeConfiguredProvider{}, errors.New("providerId is required")
	}
	if provider.Name == "" {
		return RuntimeConfiguredProvider{}, errors.New("name is required")
	}
	if provider.Protocol == "" {
		return RuntimeConfiguredProvider{}, errors.New("protocol is required")
	}
	if provider.Protocol != "openai-compat" && provider.Protocol != "anthropic" {
		return RuntimeConfiguredProvider{}, errors.New("protocol must be openai-compat or anthropic")
	}
	if provider.APIEndpoint == "" {
		return RuntimeConfiguredProvider{}, errors.New("apiEndpoint is required")
	}
	if provider.DefaultContextWindow != 0 && (provider.DefaultContextWindow < modelmeta.MinContextWindow || provider.DefaultContextWindow > modelmeta.MaxContextWindow) {
		return RuntimeConfiguredProvider{}, fmt.Errorf("defaultContextWindow must be between %d and %d", modelmeta.MinContextWindow, modelmeta.MaxContextWindow)
	}
	for _, model := range provider.Models {
		if model.ContextWindow != 0 && (model.ContextWindow < modelmeta.MinContextWindow || model.ContextWindow > modelmeta.MaxContextWindow) {
			return RuntimeConfiguredProvider{}, fmt.Errorf("contextWindow for %s must be between %d and %d", model.ID, modelmeta.MinContextWindow, modelmeta.MaxContextWindow)
		}
	}
	return provider, nil
}

func configuredProviderNameKey(name string) string {
	return strings.ToLower(strings.TrimSpace(name))
}

func (s runtimeProviderSettingsStore) enrichConfiguredProviderModels(ctx context.Context, provider RuntimeConfiguredProvider) (RuntimeConfiguredProvider, error) {
	metadata, err := s.ListModelMetadata(ctx, provider.ID)
	if err != nil {
		return RuntimeConfiguredProvider{}, err
	}
	metadataByID := make(map[string]RuntimeProviderModel, len(metadata))
	for _, model := range metadata {
		metadataByID[model.ID] = model
	}
	models := compactProviderModels(provider.Models)
	for i := range models {
		cached := metadataByID[models[i].ID]
		limits := modelmeta.Resolve(modelmeta.ResolveRequest{
			ProviderID:                   provider.ProviderID,
			ModelID:                      models[i].ID,
			UserContextWindow:            models[i].ContextWindow,
			UserMaxOutputTokens:          models[i].MaxOutputTokens,
			ProviderDefaultContextWindow: provider.DefaultContextWindow,
			DiscoveredContextWindow:      cached.ContextWindow,
			DiscoveredMaxOutputTokens:    cached.MaxOutputTokens,
			Catalog:                      configuredProviderCatalogModels(provider.ProviderID),
		})
		// ContextWindow / MaxOutputTokens keep the user's explicit input only
		// (0 = not set). Resolved values live in the dedicated fields, and
		// Source reflects the resolution tier that actually provided the
		// window — user_override only when the user really set the window.
		models[i].ResolvedContextWindow = limits.ContextWindow
		models[i].ResolvedMaxOutputTokens = limits.MaxOutputTokens
		models[i].Source = limits.Source
	}
	provider.Models = models
	return provider, nil
}

func (s runtimeProviderSettingsStore) UpsertModelMetadata(ctx context.Context, providerID string, models []RuntimeProviderModel) error {
	if s.db == nil {
		return errors.New("provider settings database is not available")
	}
	if err := s.ensureConfiguredProviderColumns(ctx); err != nil {
		return err
	}
	now := time.Now().UTC().UnixMilli()
	for _, model := range compactProviderModels(models) {
		if strings.TrimSpace(model.ID) == "" {
			continue
		}
		_, err := s.db.ExecContext(ctx, `
INSERT INTO model_metadata_cache (
    provider_id, model_id, context_window, max_output_tokens, source, fetched_at
) VALUES (?, ?, ?, ?, ?, ?)
ON CONFLICT(provider_id, model_id) DO UPDATE SET
    context_window = excluded.context_window,
    max_output_tokens = excluded.max_output_tokens,
    source = excluded.source,
    fetched_at = excluded.fetched_at`,
			strings.TrimSpace(providerID),
			model.ID,
			nullableInt(model.ContextWindow),
			nullableInt(model.MaxOutputTokens),
			modelmeta.SourceDiscovered,
			now,
		)
		if err != nil {
			return fmt.Errorf("failed to cache model metadata for %s: %w", model.ID, err)
		}
	}
	return nil
}

func (s runtimeProviderSettingsStore) ListModelMetadata(ctx context.Context, providerID string) ([]RuntimeProviderModel, error) {
	if s.db == nil {
		return nil, errors.New("provider settings database is not available")
	}
	rows, err := s.db.QueryContext(ctx, `
SELECT model_id, context_window, max_output_tokens, source
FROM model_metadata_cache
WHERE provider_id = ?`, strings.TrimSpace(providerID))
	if err != nil {
		return nil, fmt.Errorf("failed to list model metadata: %w", err)
	}
	defer rows.Close() //nolint:errcheck

	var models []RuntimeProviderModel
	for rows.Next() {
		var model RuntimeProviderModel
		var contextWindow, maxOutputTokens sql.NullInt64
		if err := rows.Scan(&model.ID, &contextWindow, &maxOutputTokens, &model.Source); err != nil {
			return nil, fmt.Errorf("failed to scan model metadata: %w", err)
		}
		model.ContextWindow = int(contextWindow.Int64)
		model.MaxOutputTokens = int(maxOutputTokens.Int64)
		models = append(models, model)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return models, nil
}

func (s runtimeProviderSettingsStore) GetConfiguredSecret(ctx context.Context, id string) (RuntimeConfiguredProvider, string, error) {
	if s.db == nil {
		return RuntimeConfiguredProvider{}, "", errors.New("provider settings database is not available")
	}
	if err := s.ensureConfiguredProviderColumns(ctx); err != nil {
		return RuntimeConfiguredProvider{}, "", err
	}
	row := s.db.QueryRowContext(ctx, configuredProviderSecretSelectSQL()+` WHERE id = ?`, strings.TrimSpace(id))
	provider, apiKey, err := scanConfiguredProviderSecret(row)
	if errors.Is(err, sql.ErrNoRows) {
		return RuntimeConfiguredProvider{}, "", errConfiguredProviderNotFound
	}
	if err != nil {
		return RuntimeConfiguredProvider{}, "", err
	}
	provider, err = s.enrichConfiguredProviderModels(ctx, provider)
	return provider, apiKey, err
}

func scanConfiguredProviderSecret(scanner interface {
	Scan(dest ...any) error
},
) (RuntimeConfiguredProvider, string, error) {
	var provider RuntimeConfiguredProvider
	var remark, secretRef, proxy, defaultModel, apiKey sql.NullString
	var modelsJSON string
	var defaultContextWindow sql.NullInt64
	var enabled int
	if err := scanner.Scan(
		&provider.ID,
		&provider.ProviderID,
		&provider.Name,
		&remark,
		&provider.Protocol,
		&provider.APIEndpoint,
		&secretRef,
		&proxy,
		&defaultModel,
		&modelsJSON,
		&defaultContextWindow,
		&enabled,
		&provider.CreatedAt,
		&provider.UpdatedAt,
		&apiKey,
	); err != nil {
		return RuntimeConfiguredProvider{}, "", err
	}
	provider.Remark = remark.String
	provider.APIKeySecretRef = secretRef.String
	provider.APIKey = apiKey.String
	provider.HasAPIKey = (secretRef.Valid && secretRef.String != "") || (apiKey.Valid && apiKey.String != "")
	provider.Proxy = proxy.String
	provider.DefaultModel = defaultModel.String
	provider.Models = compactProviderModels(append(unmarshalProviderModels(modelsJSON), RuntimeProviderModel{ID: provider.DefaultModel}))
	provider.DefaultContextWindow = int(defaultContextWindow.Int64)
	provider.Enabled = enabled != 0
	return provider, apiKey.String, nil
}

func (r *runtimeService) DiscoverConfiguredProviderModels(ctx context.Context, id string) (RuntimeProviderModelDiscoveryResponse, error) {
	provider, apiKey, err := r.configuredProviderWithSecret(ctx, id)
	if err != nil {
		return RuntimeProviderModelDiscoveryResponse{}, err
	}
	models, err := discoverProviderModelIDs(ctx, provider, apiKey)
	if err != nil {
		fallback := compactProviderModels([]RuntimeProviderModel{{ID: provider.DefaultModel}})
		return RuntimeProviderModelDiscoveryResponse{ProviderID: provider.ID, Models: fallback, Error: err.Error()}, nil
	}
	if err := r.cacheDiscoveredProviderModels(ctx, provider.ID, models); err != nil {
		return RuntimeProviderModelDiscoveryResponse{}, err
	}
	return RuntimeProviderModelDiscoveryResponse{ProviderID: provider.ID, Models: models}, nil
}

func (r *runtimeService) TestConfiguredProvider(ctx context.Context, id string) (RuntimeProviderTestResponse, error) {
	return r.testConfiguredProvider(ctx, id)
}

func (r *runtimeService) MeasureConfiguredProviderLatency(ctx context.Context, id string) (RuntimeProviderTestResponse, error) {
	return r.testConfiguredProvider(ctx, id)
}

func (r *runtimeService) configuredProviderWithSecret(ctx context.Context, id string) (RuntimeConfiguredProvider, string, error) {
	store, err := r.providerSettingsStore(ctx)
	if err != nil {
		return RuntimeConfiguredProvider{}, "", err
	}
	provider, apiKey, err := store.GetConfiguredSecret(ctx, id)
	if err != nil {
		return RuntimeConfiguredProvider{}, "", err
	}
	if strings.TrimSpace(apiKey) == "" {
		return RuntimeConfiguredProvider{}, "", errors.New("api key is required")
	}
	return provider, apiKey, nil
}

func (r *runtimeService) testConfiguredProvider(ctx context.Context, id string) (RuntimeProviderTestResponse, error) {
	provider, apiKey, err := r.configuredProviderWithSecret(ctx, id)
	if err != nil {
		return RuntimeProviderTestResponse{}, err
	}
	model := strings.TrimSpace(provider.DefaultModel)
	if model == "" {
		model = "deepseek-chat"
	}
	started := time.Now()
	err = testProviderConnection(ctx, provider, apiKey, model)
	durationMS := time.Since(started).Milliseconds()
	if err != nil {
		return RuntimeProviderTestResponse{OK: false, ProviderID: provider.ID, Model: model, DurationMS: durationMS, Error: err.Error()}, nil
	}
	return RuntimeProviderTestResponse{OK: true, ProviderID: provider.ID, Model: model, DurationMS: durationMS}, nil
}

func discoverProviderModelIDs(ctx context.Context, provider RuntimeConfiguredProvider, apiKey string) ([]RuntimeProviderModel, error) {
	protocol, baseURL := normalizeConfiguredProviderProtocolURL(provider)
	if protocol != "openai" && protocol != "anthropic" {
		return nil, errors.New("protocol must be openai-compat or anthropic")
	}
	models, err := discoverModels(ctx, RuntimeModelConfig{
		Protocol: protocol,
		URL:      baseURL,
		APIKey:   apiKey,
		Model:    provider.DefaultModel,
		Proxy:    provider.Proxy,
	})
	if err != nil {
		return nil, err
	}
	return models, nil
}

func (r *runtimeService) cacheDiscoveredProviderModels(ctx context.Context, providerID string, models []RuntimeProviderModel) error {
	store, err := r.providerSettingsStore(ctx)
	if err != nil {
		return err
	}
	return store.UpsertModelMetadata(ctx, providerID, models)
}

func testProviderConnection(ctx context.Context, provider RuntimeConfiguredProvider, apiKey string, model string) error {
	protocol, baseURL := normalizeConfiguredProviderProtocolURL(provider)
	baseURL = normalizeProviderAPIV1BaseURL(protocol, baseURL)
	requestCtx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()

	switch protocol {
	case "openai":
		endpoint := strings.TrimRight(baseURL, "/") + "/chat/completions"
		payload := map[string]any{
			"model":      model,
			"max_tokens": 1,
			"messages":   []map[string]string{{"role": "user", "content": "ping"}},
		}
		return postProviderJSON(requestCtx, endpoint, map[string]string{"Authorization": "Bearer " + apiKey}, payload)
	case "anthropic":
		endpoint := strings.TrimRight(baseURL, "/") + "/messages"
		payload := map[string]any{
			"model":      model,
			"max_tokens": 1,
			"messages":   []map[string]string{{"role": "user", "content": "ping"}},
		}
		return postProviderJSON(requestCtx, endpoint, map[string]string{"x-api-key": apiKey, "anthropic-version": "2023-06-01"}, payload)
	default:
		return errors.New("protocol must be openai-compat or anthropic")
	}
}

func postProviderJSON(ctx context.Context, endpoint string, headers map[string]string, payload map[string]any) error {
	if _, err := url.ParseRequestURI(endpoint); err != nil {
		return fmt.Errorf("invalid api endpoint: %w", err)
	}
	data, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(data))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	for key, value := range headers {
		req.Header.Set(key, value)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return fmt.Errorf("failed to connect to provider: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("provider returned %s", resp.Status)
	}
	return nil
}

func normalizeConfiguredProviderProtocolURL(provider RuntimeConfiguredProvider) (string, string) {
	protocol := "openai"
	if provider.Protocol == "anthropic" {
		protocol = "anthropic"
	}
	baseURL := normalizeModelBaseURL(protocol, provider.APIEndpoint)
	return protocol, baseURL
}

func nullableInt(value int) any {
	if value == 0 {
		return nil
	}
	return value
}

// mustMarshalProviderModels persists only the user's explicit values:
// resolved values and resolution source are runtime projections and must
// never be written back into models_json.
func mustMarshalProviderModels(values []RuntimeProviderModel) string {
	models := compactProviderModels(values)
	stored := make([]RuntimeProviderModel, 0, len(models))
	for _, model := range models {
		stored = append(stored, RuntimeProviderModel{
			ID:              model.ID,
			DisplayName:     model.DisplayName,
			ContextWindow:   model.ContextWindow,
			MaxOutputTokens: model.MaxOutputTokens,
		})
	}
	data, err := json.Marshal(stored)
	if err != nil {
		return "[]"
	}
	return string(data)
}

func unmarshalProviderModels(raw string) []RuntimeProviderModel {
	if strings.TrimSpace(raw) == "" {
		return nil
	}
	var models []RuntimeProviderModel
	if err := json.Unmarshal([]byte(raw), &models); err == nil {
		return models
	}
	var ids []string
	if err := json.Unmarshal([]byte(raw), &ids); err != nil {
		return nil
	}
	converted := make([]RuntimeProviderModel, 0, len(ids))
	for _, id := range ids {
		converted = append(converted, RuntimeProviderModel{ID: id})
	}
	return converted
}

func compactProviderModels(models []RuntimeProviderModel) []RuntimeProviderModel {
	seen := map[string]bool{}
	compact := make([]RuntimeProviderModel, 0, len(models))
	for _, model := range models {
		model.ID = strings.TrimSpace(model.ID)
		model.DisplayName = strings.TrimSpace(model.DisplayName)
		model.Source = strings.TrimSpace(model.Source)
		if model.ID == "" || seen[model.ID] {
			continue
		}
		seen[model.ID] = true
		compact = append(compact, model)
	}
	return compact
}

func providerModelIDs(models []RuntimeProviderModel) []string {
	ids := make([]string, 0, len(models))
	for _, model := range models {
		ids = append(ids, model.ID)
	}
	return compactModelIDs(ids)
}

func mustMarshalStringSlice(values []string) string {
	data, err := json.Marshal(values)
	if err != nil {
		return "[]"
	}
	return string(data)
}

func unmarshalStringSlice(raw string) []string {
	if strings.TrimSpace(raw) == "" {
		return nil
	}
	var values []string
	if err := json.Unmarshal([]byte(raw), &values); err != nil {
		return nil
	}
	return values
}
