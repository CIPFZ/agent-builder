package config

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func floatPtr(v float64) *float64 { return &v }
func boolPtr(v bool) *bool        { return &v }

func TestContextGovernanceFor_Defaults(t *testing.T) {
	t.Parallel()

	var cfg *ContextGovernanceConfig
	resolved := cfg.ContextGovernanceFor("openai", "gpt-4o")
	require.True(t, resolved.AutoCompactEnabled)
	require.Nil(t, resolved.AutoCompactPercent)
	require.True(t, resolved.MicrocompactEnabled)
	require.Equal(t, DefaultContextGovernanceMicrocompactKeepRecent, resolved.MicrocompactKeepRecent)
	require.Equal(t, ContextGovernanceSummaryModelSession, resolved.SummaryModel)
}

func TestContextGovernanceFor_GlobalOverride(t *testing.T) {
	t.Parallel()

	cfg := &ContextGovernanceConfig{
		AutoCompactEnabled:     boolPtr(false),
		AutoCompactPercent:     floatPtr(0.6),
		MicrocompactEnabled:    boolPtr(false),
		MicrocompactKeepRecent: 8,
		SummaryModel:           ContextGovernanceSummaryModelSmall,
	}
	resolved := cfg.ContextGovernanceFor("openai", "gpt-4o")
	require.False(t, resolved.AutoCompactEnabled)
	require.NotNil(t, resolved.AutoCompactPercent)
	require.InDelta(t, 0.6, *resolved.AutoCompactPercent, 0.0001)
	require.False(t, resolved.MicrocompactEnabled)
	require.Equal(t, 8, resolved.MicrocompactKeepRecent)
	require.Equal(t, ContextGovernanceSummaryModelSmall, resolved.SummaryModel)
}

// TestContextGovernanceFor_OverrideChain asserts the model > provider >
// global precedence: a model-level override wins over its provider's
// override, which wins over the global setting.
func TestContextGovernanceFor_OverrideChain(t *testing.T) {
	t.Parallel()

	cfg := &ContextGovernanceConfig{
		AutoCompactPercent: floatPtr(0.9),
		ProviderOverrides: map[string]ContextGovernanceProviderOverride{
			"openai": {
				AutoCompactPercent: floatPtr(0.7),
				Models: map[string]ContextGovernanceModelOverride{
					"gpt-4o": {AutoCompactPercent: floatPtr(0.3)},
				},
			},
		},
	}

	// Model-level override wins.
	resolved := cfg.ContextGovernanceFor("openai", "gpt-4o")
	require.NotNil(t, resolved.AutoCompactPercent)
	require.InDelta(t, 0.3, *resolved.AutoCompactPercent, 0.0001)

	// Provider-level override wins for a model without its own override.
	resolved = cfg.ContextGovernanceFor("openai", "gpt-4o-mini")
	require.NotNil(t, resolved.AutoCompactPercent)
	require.InDelta(t, 0.7, *resolved.AutoCompactPercent, 0.0001)

	// Falls back to the global value for an unrelated provider.
	resolved = cfg.ContextGovernanceFor("anthropic", "claude")
	require.NotNil(t, resolved.AutoCompactPercent)
	require.InDelta(t, 0.9, *resolved.AutoCompactPercent, 0.0001)
}

func TestValidateContextGovernanceConfig(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		cfg     ContextGovernanceConfig
		wantErr bool
	}{
		{name: "empty is valid", cfg: ContextGovernanceConfig{}},
		{name: "valid percent", cfg: ContextGovernanceConfig{AutoCompactPercent: floatPtr(0.5)}},
		{name: "percent too low", cfg: ContextGovernanceConfig{AutoCompactPercent: floatPtr(0.01)}, wantErr: true},
		{name: "percent too high", cfg: ContextGovernanceConfig{AutoCompactPercent: floatPtr(0.99)}, wantErr: true},
		{name: "percent at lower bound", cfg: ContextGovernanceConfig{AutoCompactPercent: floatPtr(0.05)}},
		{name: "percent at upper bound", cfg: ContextGovernanceConfig{AutoCompactPercent: floatPtr(0.95)}},
		{name: "invalid summary model", cfg: ContextGovernanceConfig{SummaryModel: "huge"}, wantErr: true},
		{name: "valid summary model small", cfg: ContextGovernanceConfig{SummaryModel: "small"}},
		{name: "negative keep recent", cfg: ContextGovernanceConfig{MicrocompactKeepRecent: -1}, wantErr: true},
		{
			name: "invalid provider override percent",
			cfg: ContextGovernanceConfig{
				ProviderOverrides: map[string]ContextGovernanceProviderOverride{
					"openai": {AutoCompactPercent: floatPtr(1.5)},
				},
			},
			wantErr: true,
		},
		{
			name: "invalid model override percent",
			cfg: ContextGovernanceConfig{
				ProviderOverrides: map[string]ContextGovernanceProviderOverride{
					"openai": {Models: map[string]ContextGovernanceModelOverride{
						"gpt-4o": {AutoCompactPercent: floatPtr(0)},
					}},
				},
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			err := ValidateContextGovernanceConfig(tt.cfg)
			if tt.wantErr {
				require.Error(t, err)
				require.True(t, errors.Is(err, ErrContextGovernanceInvalid))
			} else {
				require.NoError(t, err)
			}
		})
	}
}

func TestConfigStore_SaveContextGovernance_PersistsAndReloadsInMemory(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	globalPath := filepath.Join(dir, "agent-builder.json")
	store := &ConfigStore{
		config:         &Config{},
		globalDataPath: globalPath,
	}

	pct := 0.42
	err := store.SaveContextGovernance(ContextGovernanceConfig{
		AutoCompactEnabled: boolPtr(false),
		AutoCompactPercent: &pct,
		SummaryModel:       ContextGovernanceSummaryModelSmall,
	})
	require.NoError(t, err)

	// In-memory config reflects the save immediately (workingDir is empty in
	// this test, so ConfigStore.autoReload is a no-op and this assertion
	// exercises the direct in-memory update rather than a reload round trip).
	require.NotNil(t, store.Config().ContextGovernance)
	require.False(t, *store.Config().ContextGovernance.AutoCompactEnabled)
	require.InDelta(t, 0.42, *store.Config().ContextGovernance.AutoCompactPercent, 0.0001)

	// The global config file on disk carries the same values under the
	// "contextGovernance" key.
	data, err := os.ReadFile(globalPath)
	require.NoError(t, err)
	var onDisk struct {
		ContextGovernance ContextGovernanceConfig `json:"contextGovernance"`
	}
	require.NoError(t, json.Unmarshal(data, &onDisk))
	require.NotNil(t, onDisk.ContextGovernance.AutoCompactEnabled)
	require.False(t, *onDisk.ContextGovernance.AutoCompactEnabled)
	require.InDelta(t, 0.42, *onDisk.ContextGovernance.AutoCompactPercent, 0.0001)
	require.Equal(t, ContextGovernanceSummaryModelSmall, onDisk.ContextGovernance.SummaryModel)
}

func TestConfigStore_SaveContextGovernance_RejectsInvalidPercent(t *testing.T) {
	t.Parallel()

	store := &ConfigStore{
		config:         &Config{},
		globalDataPath: filepath.Join(t.TempDir(), "agent-builder.json"),
	}

	pct := 3.0
	err := store.SaveContextGovernance(ContextGovernanceConfig{AutoCompactPercent: &pct})
	require.Error(t, err)
	require.True(t, errors.Is(err, ErrContextGovernanceInvalid))
	// A rejected save must not mutate the in-memory config.
	require.Nil(t, store.Config().ContextGovernance)
}
