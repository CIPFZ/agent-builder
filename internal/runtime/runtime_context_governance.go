package runtime

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/CIPFZ/agent-builder/internal/config"
)

const contextGovernanceSettingKey = "context_governance"

// RuntimeContextGovernanceModelOverride overrides context governance
// settings for a single model within a provider.
type RuntimeContextGovernanceModelOverride struct {
	AutoCompactPercent *float64 `json:"autoCompactPercent,omitempty"`
}

// RuntimeContextGovernanceProviderOverride overrides context governance
// settings for every model of a given provider, with optional per-model
// overrides nested inside.
type RuntimeContextGovernanceProviderOverride struct {
	AutoCompactPercent *float64                                         `json:"autoCompactPercent,omitempty"`
	Models             map[string]RuntimeContextGovernanceModelOverride `json:"models,omitempty"`
}

// RuntimeContextGovernanceSettings is the Wails-facing shape of the
// "contextGovernance" config section. Every field is optional; unset fields
// fall back to documented defaults (enabled=true, pct=nil(auto),
// microcompact=true, keepRecent=5, summaryModel=session).
type RuntimeContextGovernanceSettings struct {
	AutoCompactEnabled      *bool                                               `json:"autoCompactEnabled,omitempty"`
	AutoCompactPercent      *float64                                            `json:"autoCompactPercent,omitempty"`
	MicrocompactEnabled     *bool                                               `json:"microcompactEnabled,omitempty"`
	MicrocompactIdleMinutes int                                                 `json:"microcompactIdleMinutes,omitempty"`
	MicrocompactKeepRecent  int                                                 `json:"microcompactKeepRecent,omitempty"`
	SessionMemoryEnabled    *bool                                               `json:"sessionMemoryEnabled,omitempty"`
	SummaryModel            string                                              `json:"summaryModel,omitempty"`
	ProviderOverrides       map[string]RuntimeContextGovernanceProviderOverride `json:"providerOverrides,omitempty"`
}

// RuntimeContextGovernanceSettingsResponse wraps the persisted settings.
type RuntimeContextGovernanceSettingsResponse struct {
	Settings RuntimeContextGovernanceSettings `json:"settings"`
}

// ContextGovernanceSettings returns the currently persisted context
// governance settings (raw, not resolved against a specific model). Desktop
// settings have one authority: SQLite application_settings.
func (r *runtimeService) ContextGovernanceSettings(ctx context.Context) (RuntimeContextGovernanceSettingsResponse, error) {
	settings, err := r.loadRuntimeContextGovernanceSettings(ctx)
	if err != nil {
		return RuntimeContextGovernanceSettingsResponse{}, err
	}
	return RuntimeContextGovernanceSettingsResponse{Settings: settings}, nil
}

// SaveContextGovernanceSettings validates and persists the given context
// governance settings (the PUT body is the whole settings object), then
// returns the settings as they were saved.
func (r *runtimeService) SaveContextGovernanceSettings(ctx context.Context, req RuntimeContextGovernanceSettings) (RuntimeContextGovernanceSettingsResponse, error) {
	next := req.toConfig()
	if err := config.ValidateContextGovernanceConfig(next); err != nil {
		return RuntimeContextGovernanceSettingsResponse{}, err
	}
	normalized := runtimeContextGovernanceFromConfig(&next)
	raw, err := json.Marshal(normalized)
	if err != nil {
		return RuntimeContextGovernanceSettingsResponse{}, fmt.Errorf("encode context governance settings: %w", err)
	}
	conn, err := r.configDB(ctx)
	if err != nil {
		return RuntimeContextGovernanceSettingsResponse{}, err
	}
	if _, err := conn.ExecContext(ctx, `INSERT INTO application_settings (key, value_json, updated_at) VALUES (?, ?, ?)
ON CONFLICT(key) DO UPDATE SET value_json = excluded.value_json, updated_at = excluded.updated_at`, contextGovernanceSettingKey, string(raw), time.Now().UnixMilli()); err != nil {
		return RuntimeContextGovernanceSettingsResponse{}, fmt.Errorf("save context governance settings: %w", err)
	}
	return RuntimeContextGovernanceSettingsResponse{
		Settings: normalized,
	}, nil
}

func (r *runtimeService) loadRuntimeContextGovernanceSettings(ctx context.Context) (RuntimeContextGovernanceSettings, error) {
	conn, err := r.configDB(ctx)
	if err != nil {
		return RuntimeContextGovernanceSettings{}, err
	}
	var raw string
	err = conn.QueryRowContext(ctx, `SELECT value_json FROM application_settings WHERE key = ?`, contextGovernanceSettingKey).Scan(&raw)
	if errors.Is(err, sql.ErrNoRows) {
		return RuntimeContextGovernanceSettings{}, nil
	}
	if err != nil {
		return RuntimeContextGovernanceSettings{}, fmt.Errorf("load context governance settings: %w", err)
	}
	var settings RuntimeContextGovernanceSettings
	if err := json.Unmarshal([]byte(raw), &settings); err != nil {
		return RuntimeContextGovernanceSettings{}, fmt.Errorf("decode context governance settings: %w", err)
	}
	if err := config.ValidateContextGovernanceConfig(settings.toConfig()); err != nil {
		return RuntimeContextGovernanceSettings{}, fmt.Errorf("stored context governance settings: %w", err)
	}
	return settings, nil
}

func runtimeContextGovernanceFromConfig(cfg *config.ContextGovernanceConfig) RuntimeContextGovernanceSettings {
	if cfg == nil {
		return RuntimeContextGovernanceSettings{}
	}
	out := RuntimeContextGovernanceSettings{
		AutoCompactEnabled:      cfg.AutoCompactEnabled,
		AutoCompactPercent:      cfg.AutoCompactPercent,
		MicrocompactEnabled:     cfg.MicrocompactEnabled,
		MicrocompactIdleMinutes: cfg.MicrocompactIdleMinutes,
		MicrocompactKeepRecent:  cfg.MicrocompactKeepRecent,
		SessionMemoryEnabled:    cfg.SessionMemoryEnabled,
		SummaryModel:            cfg.SummaryModel,
	}
	if len(cfg.ProviderOverrides) > 0 {
		out.ProviderOverrides = make(map[string]RuntimeContextGovernanceProviderOverride, len(cfg.ProviderOverrides))
		for providerID, override := range cfg.ProviderOverrides {
			out.ProviderOverrides[providerID] = runtimeContextGovernanceProviderOverrideFromConfig(override)
		}
	}
	return out
}

func runtimeContextGovernanceProviderOverrideFromConfig(override config.ContextGovernanceProviderOverride) RuntimeContextGovernanceProviderOverride {
	out := RuntimeContextGovernanceProviderOverride{
		AutoCompactPercent: override.AutoCompactPercent,
	}
	if len(override.Models) > 0 {
		out.Models = make(map[string]RuntimeContextGovernanceModelOverride, len(override.Models))
		for modelID, modelOverride := range override.Models {
			out.Models[modelID] = RuntimeContextGovernanceModelOverride{AutoCompactPercent: modelOverride.AutoCompactPercent}
		}
	}
	return out
}

func (s RuntimeContextGovernanceSettings) toConfig() config.ContextGovernanceConfig {
	out := config.ContextGovernanceConfig{
		AutoCompactEnabled:      s.AutoCompactEnabled,
		AutoCompactPercent:      s.AutoCompactPercent,
		MicrocompactEnabled:     s.MicrocompactEnabled,
		MicrocompactIdleMinutes: s.MicrocompactIdleMinutes,
		MicrocompactKeepRecent:  s.MicrocompactKeepRecent,
		SessionMemoryEnabled:    s.SessionMemoryEnabled,
		SummaryModel:            strings.TrimSpace(s.SummaryModel),
	}
	if len(s.ProviderOverrides) > 0 {
		out.ProviderOverrides = make(map[string]config.ContextGovernanceProviderOverride, len(s.ProviderOverrides))
		for providerID, override := range s.ProviderOverrides {
			out.ProviderOverrides[providerID] = override.toConfig()
		}
	}
	return out
}

func (o RuntimeContextGovernanceProviderOverride) toConfig() config.ContextGovernanceProviderOverride {
	out := config.ContextGovernanceProviderOverride{AutoCompactPercent: o.AutoCompactPercent}
	if len(o.Models) > 0 {
		out.Models = make(map[string]config.ContextGovernanceModelOverride, len(o.Models))
		for modelID, override := range o.Models {
			out.Models[modelID] = config.ContextGovernanceModelOverride{AutoCompactPercent: override.AutoCompactPercent}
		}
	}
	return out
}

// contextGovernanceFor resolves the effective context governance settings
// for the given session/model, applying the model > provider > global
// override chain. It never fails: if the workspace config or selected
// provider can't be resolved, the documented defaults are returned so
// callers (auto-compact judgment, microcompact config) always get a usable
// value.
func (r *runtimeService) contextGovernanceFor(ctx context.Context, sessionID, model string) config.ResolvedContextGovernance {
	var governance *config.ContextGovernanceConfig
	if settings, err := r.loadRuntimeContextGovernanceSettings(ctx); err == nil {
		cfg := settings.toConfig()
		governance = &cfg
	}
	providerID, modelID := r.resolveRuntimeGovernanceProviderModel(ctx, sessionID, model)
	return governance.ContextGovernanceFor(providerID, modelID)
}

// resolveRuntimeGovernanceProviderModel resolves the provider/model id pair
// used to look up provider/model-level context governance overrides. It
// mirrors the session > project > global selection chain used elsewhere for
// model limits (see selectedConfiguredModelLimits) but only needs the
// provider id, not full model metadata.
func (r *runtimeService) resolveRuntimeGovernanceProviderModel(ctx context.Context, sessionID, model string) (providerID, modelID string) {
	modelID = strings.TrimSpace(model)
	store, providerStore, err := r.selectedModelStores(ctx)
	if err != nil {
		return "", modelID
	}
	r.mu.Lock()
	projectID := r.activeProjectID
	r.mu.Unlock()
	var selected RuntimeSelectedModel
	found := false
	if trimmed := strings.TrimSpace(sessionID); trimmed != "" {
		if candidate, getErr := store.Get(ctx, "session", "", trimmed); getErr == nil {
			selected, found = candidate, true
		}
	}
	if !found && strings.TrimSpace(projectID) != "" {
		if candidate, getErr := store.Get(ctx, "project", strings.TrimSpace(projectID), ""); getErr == nil {
			selected, found = candidate, true
		}
	}
	if !found {
		if candidate, getErr := store.Get(ctx, "global", "", ""); getErr == nil {
			selected, found = candidate, true
		}
	}
	if !found {
		return "", modelID
	}
	provider, err := providerStore.GetConfigured(ctx, selected.ConfiguredProviderID)
	if err != nil {
		return "", modelID
	}
	if modelID == "" {
		modelID = strings.TrimSpace(selected.Model)
	}
	return strings.TrimSpace(provider.ProviderID), modelID
}
