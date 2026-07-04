package config

import (
	"errors"
	"fmt"
	"strings"
)

// ErrContextGovernanceInvalid wraps every validation error returned by
// ValidateContextGovernanceConfig / ValidateContextGovernanceAutoCompactPercent
// so callers (e.g. the runtime HTTP layer) can map it to a 400 response via
// errors.Is without string matching.
var ErrContextGovernanceInvalid = errors.New("invalid context governance settings")

// Context governance summary model selectors. "session" follows whatever
// model the session is currently using; "small" always uses the configured
// small model so summarization stays cheap regardless of the session model.
const (
	ContextGovernanceSummaryModelSession = "session"
	ContextGovernanceSummaryModelSmall   = "small"
)

// DefaultContextGovernanceMicrocompactKeepRecent is the number of most
// recent messages microcompact always keeps untouched when no override is
// configured.
const DefaultContextGovernanceMicrocompactKeepRecent = 5

// Auto-compact trigger percent overrides must stay within this range: below
// 5% compaction would fire almost immediately, above 95% there is no room
// left for the model's own output.
const (
	MinContextGovernanceAutoCompactPercent = 0.05
	MaxContextGovernanceAutoCompactPercent = 0.95
)

// ContextGovernanceModelOverride overrides context governance settings for a
// single model within a provider. Only autoCompactPercent is overridable at
// the model level today; other fields fall back to the provider/global
// value.
type ContextGovernanceModelOverride struct {
	AutoCompactPercent *float64 `json:"autoCompactPercent,omitempty" jsonschema:"description=Auto compact trigger as a fraction of the model's context window (0.05-0.95). Empty inherits the provider/global value.,minimum=0.05,maximum=0.95"`
}

// ContextGovernanceProviderOverride overrides context governance settings
// for every model of a given provider, with optional per-model overrides
// nested inside.
type ContextGovernanceProviderOverride struct {
	AutoCompactPercent *float64                                  `json:"autoCompactPercent,omitempty" jsonschema:"description=Auto compact trigger as a fraction of the context window (0.05-0.95) for every model of this provider. Empty inherits the global value.,minimum=0.05,maximum=0.95"`
	Models             map[string]ContextGovernanceModelOverride `json:"models,omitempty" jsonschema:"description=Per-model overrides keyed by model id"`
}

// ContextGovernanceConfig is the global "contextGovernance" config section.
// Every field is optional; ContextGovernanceFor resolves the effective
// settings for a given provider/model with documented defaults.
type ContextGovernanceConfig struct {
	AutoCompactEnabled     *bool                                        `json:"autoCompactEnabled,omitempty" jsonschema:"description=Whether the runtime is allowed to auto-compact the conversation when the context window fills up,default=true"`
	AutoCompactPercent     *float64                                     `json:"autoCompactPercent,omitempty" jsonschema:"description=Global auto compact trigger as a fraction of the context window (0.05-0.95). Empty lets the runtime derive the trigger from the model's context window.,minimum=0.05,maximum=0.95"`
	MicrocompactEnabled    *bool                                        `json:"microcompactEnabled,omitempty" jsonschema:"description=Whether microcompact (tool-result trimming within a step) is enabled,default=true"`
	MicrocompactKeepRecent int                                          `json:"microcompactKeepRecent,omitempty" jsonschema:"description=Number of most recent messages microcompact always keeps untouched,default=5"`
	SummaryModel           string                                       `json:"summaryModel,omitempty" jsonschema:"description=Model used to generate compact summaries,enum=session,enum=small,default=session"`
	ProviderOverrides      map[string]ContextGovernanceProviderOverride `json:"providerOverrides,omitempty" jsonschema:"description=Per-provider (and nested per-model) overrides keyed by provider id"`
}

// ResolvedContextGovernance is the fully-defaulted view of context
// governance settings for a specific provider/model pair.
type ResolvedContextGovernance struct {
	AutoCompactEnabled bool
	// AutoCompactPercent is nil when the trigger should be derived from the
	// model's context window formula (no override in effect).
	AutoCompactPercent     *float64
	MicrocompactEnabled    bool
	MicrocompactKeepRecent int
	SummaryModel           string
}

// ContextGovernanceFor resolves the effective context governance settings
// for the given provider/model, applying the model > provider > global
// override chain on top of the documented defaults (enabled=true, pct=nil,
// microcompact=true, keepRecent=5, summaryModel=session). c may be nil.
func (c *ContextGovernanceConfig) ContextGovernanceFor(providerID, modelID string) ResolvedContextGovernance {
	resolved := ResolvedContextGovernance{
		AutoCompactEnabled:     true,
		AutoCompactPercent:     nil,
		MicrocompactEnabled:    true,
		MicrocompactKeepRecent: DefaultContextGovernanceMicrocompactKeepRecent,
		SummaryModel:           ContextGovernanceSummaryModelSession,
	}
	if c == nil {
		return resolved
	}
	if c.AutoCompactEnabled != nil {
		resolved.AutoCompactEnabled = *c.AutoCompactEnabled
	}
	if c.AutoCompactPercent != nil {
		resolved.AutoCompactPercent = c.AutoCompactPercent
	}
	if c.MicrocompactEnabled != nil {
		resolved.MicrocompactEnabled = *c.MicrocompactEnabled
	}
	if c.MicrocompactKeepRecent > 0 {
		resolved.MicrocompactKeepRecent = c.MicrocompactKeepRecent
	}
	if strings.TrimSpace(c.SummaryModel) != "" {
		resolved.SummaryModel = c.SummaryModel
	}

	providerID = strings.TrimSpace(providerID)
	modelID = strings.TrimSpace(modelID)
	if providerID == "" || c.ProviderOverrides == nil {
		return resolved
	}
	providerOverride, ok := c.ProviderOverrides[providerID]
	if !ok {
		return resolved
	}
	if providerOverride.AutoCompactPercent != nil {
		resolved.AutoCompactPercent = providerOverride.AutoCompactPercent
	}
	if modelID != "" && providerOverride.Models != nil {
		if modelOverride, ok := providerOverride.Models[modelID]; ok && modelOverride.AutoCompactPercent != nil {
			resolved.AutoCompactPercent = modelOverride.AutoCompactPercent
		}
	}
	return resolved
}

// ValidateContextGovernanceAutoCompactPercent rejects percentages outside
// the [0.05, 0.95] range.
func ValidateContextGovernanceAutoCompactPercent(pct float64) error {
	if pct < MinContextGovernanceAutoCompactPercent || pct > MaxContextGovernanceAutoCompactPercent {
		return fmt.Errorf("%w: autoCompactPercent must be between %.2f and %.2f", ErrContextGovernanceInvalid, MinContextGovernanceAutoCompactPercent, MaxContextGovernanceAutoCompactPercent)
	}
	return nil
}

// ValidateContextGovernanceConfig validates a whole contextGovernance
// section: every configured autoCompactPercent (global, per-provider,
// per-model) must fall within [0.05, 0.95] and summaryModel, when set, must
// be one of the known selectors.
func ValidateContextGovernanceConfig(cfg ContextGovernanceConfig) error {
	if cfg.AutoCompactPercent != nil {
		if err := ValidateContextGovernanceAutoCompactPercent(*cfg.AutoCompactPercent); err != nil {
			return err
		}
	}
	switch strings.TrimSpace(cfg.SummaryModel) {
	case "", ContextGovernanceSummaryModelSession, ContextGovernanceSummaryModelSmall:
	default:
		return fmt.Errorf("%w: summaryModel must be %q or %q", ErrContextGovernanceInvalid, ContextGovernanceSummaryModelSession, ContextGovernanceSummaryModelSmall)
	}
	if cfg.MicrocompactKeepRecent < 0 {
		return fmt.Errorf("%w: microcompactKeepRecent must not be negative", ErrContextGovernanceInvalid)
	}
	for providerID, providerOverride := range cfg.ProviderOverrides {
		if providerOverride.AutoCompactPercent != nil {
			if err := ValidateContextGovernanceAutoCompactPercent(*providerOverride.AutoCompactPercent); err != nil {
				return fmt.Errorf("provider %s: %w", providerID, err)
			}
		}
		for modelID, modelOverride := range providerOverride.Models {
			if modelOverride.AutoCompactPercent != nil {
				if err := ValidateContextGovernanceAutoCompactPercent(*modelOverride.AutoCompactPercent); err != nil {
					return fmt.Errorf("provider %s model %s: %w", providerID, modelID, err)
				}
			}
		}
	}
	return nil
}

// SaveContextGovernance validates and persists the given context governance
// settings to the global config file (JSON key "contextGovernance"), then
// updates the in-memory config so callers observe the change immediately
// even if auto-reload is disabled (e.g. in tests).
func (s *ConfigStore) SaveContextGovernance(cfg ContextGovernanceConfig) error {
	if err := ValidateContextGovernanceConfig(cfg); err != nil {
		return err
	}
	normalized := cfg
	s.config.ContextGovernance = &normalized
	if err := s.SetConfigField(ScopeGlobal, "contextGovernance", normalized); err != nil {
		return fmt.Errorf("failed to persist context governance settings: %w", err)
	}
	return nil
}
