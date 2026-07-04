package runtime

import (
	"context"
	"database/sql"
	"os"
	"path/filepath"
	"testing"

	"github.com/CIPFZ/agent-builder/internal/db"
	"github.com/CIPFZ/agent-builder/internal/modelmeta"
)

func providerSettingsStoreForTest(t *testing.T) (runtimeProviderSettingsStore, *sql.DB) {
	t.Helper()
	dataDir := filepath.Join(t.TempDir(), "provider-settings")
	if err := os.MkdirAll(dataDir, 0o700); err != nil {
		t.Fatal(err)
	}
	conn, err := db.Connect(context.Background(), dataDir)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Release(dataDir) })
	store := newRuntimeProviderSettingsStore(conn)
	if err := store.ensureConfiguredProviderColumns(context.Background()); err != nil {
		t.Fatal(err)
	}
	return store, conn
}

func TestEnrichConfiguredProviderModelsOverrideSemantics(t *testing.T) {
	t.Parallel()

	store, _ := providerSettingsStoreForTest(t)
	ctx := context.Background()

	// Only max output filled: the window must NOT become a user override —
	// the user value stays empty, the resolved window comes from the builtin
	// tier, and the user's max output is respected in the resolved value.
	provider, err := store.enrichConfiguredProviderModels(ctx, RuntimeConfiguredProvider{
		ID:         "provider-1",
		ProviderID: "deepseek",
		Models: []RuntimeProviderModel{
			{ID: "deepseek-chat", MaxOutputTokens: 8000},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	model := provider.Models[0]
	if model.Source == modelmeta.SourceUserOverride {
		t.Fatalf("max-output-only edit must not lock the window as user_override: %#v", model)
	}
	if model.ContextWindow != 0 {
		t.Fatalf("user context window must stay empty, got %d", model.ContextWindow)
	}
	if model.ResolvedContextWindow != 128000 {
		t.Fatalf("resolved context window = %d, want 128000 (builtin)", model.ResolvedContextWindow)
	}
	if model.ResolvedMaxOutputTokens != 8000 {
		t.Fatalf("resolved max output = %d, want user value 8000", model.ResolvedMaxOutputTokens)
	}

	// Window explicitly filled: source is user_override and the explicit
	// value is both the user value and the resolved value.
	provider, err = store.enrichConfiguredProviderModels(ctx, RuntimeConfiguredProvider{
		ID:         "provider-1",
		ProviderID: "deepseek",
		Models: []RuntimeProviderModel{
			{ID: "deepseek-chat", ContextWindow: 200000},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	model = provider.Models[0]
	if model.Source != modelmeta.SourceUserOverride {
		t.Fatalf("explicit window must be user_override, got %q", model.Source)
	}
	if model.ContextWindow != 200000 || model.ResolvedContextWindow != 200000 {
		t.Fatalf("window values = user %d resolved %d, want 200000/200000", model.ContextWindow, model.ResolvedContextWindow)
	}
}

func TestMarshalProviderModelsPersistsOnlyUserValues(t *testing.T) {
	t.Parallel()

	raw := mustMarshalProviderModels([]RuntimeProviderModel{{
		ID:                      "deepseek-chat",
		DisplayName:             "DeepSeek Chat",
		ContextWindow:           0,
		MaxOutputTokens:         8000,
		ResolvedContextWindow:   128000,
		ResolvedMaxOutputTokens: 8000,
		Source:                  "builtin",
	}})
	models := unmarshalProviderModels(raw)
	if len(models) != 1 {
		t.Fatalf("models = %#v", models)
	}
	stored := models[0]
	if stored.ContextWindow != 0 || stored.MaxOutputTokens != 8000 {
		t.Fatalf("stored user values = %#v", stored)
	}
	if stored.ResolvedContextWindow != 0 || stored.ResolvedMaxOutputTokens != 0 || stored.Source != "" {
		t.Fatalf("resolved values leaked into models_json: %#v", stored)
	}
}
