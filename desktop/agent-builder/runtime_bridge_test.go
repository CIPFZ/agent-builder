package main

import (
	"os"
	"path/filepath"
	"slices"
	"testing"

	"github.com/charmbracelet/crush/internal/config"
	"github.com/charmbracelet/crush/internal/csync"
)

func TestLocalModelConfigPathsIncludeWorkingDirectoryConfig(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	workingDir := filepath.Join(root, "desktop", "agent-builder", "bin")
	got := localModelConfigPaths(workingDir)
	want := filepath.Join(workingDir, "client", "server", "deepseek.local.json")
	rootWant := filepath.Join(root, "client", "server", "deepseek.local.json")

	if !slices.Contains(got, want) {
		t.Fatalf("localModelConfigPaths() missing %s in %#v", want, got)
	}
	if !slices.Contains(got, rootWant) {
		t.Fatalf("localModelConfigPaths() missing %s in %#v", rootWant, got)
	}
}

func TestApplyLocalModelConfigConfiguresProvider(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	configDir := filepath.Join(root, "client", "server")
	if err := os.MkdirAll(configDir, 0o755); err != nil {
		t.Fatal(err)
	}
	configPath := filepath.Join(configDir, "deepseek.local.json")
	if err := os.WriteFile(configPath, []byte(`{
  "protocol": "openai",
  "url": "https://api.deepseek.com",
  "apiKey": "test-key",
  "models": ["deepseek-v4-flash"]
}`), 0o600); err != nil {
		t.Fatal(err)
	}

	store := config.NewTestStore(&config.Config{
		Providers: csync.NewMap[string, config.ProviderConfig](),
		Models:    map[config.SelectedModelType]config.SelectedModel{},
		Options:   &config.Options{},
	})

	result := applyLocalModelConfig(store, root)
	if result.Error != nil {
		t.Fatal(result.Error)
	}
	if !result.Applied {
		t.Fatal("local config was not applied")
	}
	if result.Path != configPath {
		t.Fatalf("Path = %s, want %s", result.Path, configPath)
	}

	provider, ok := store.Config().Providers.Get("deepseek-local")
	if !ok {
		t.Fatal("deepseek-local provider was not configured")
	}
	if provider.APIKey != "test-key" {
		t.Fatal("api key was not applied")
	}
	selected := store.Config().Models[config.SelectedModelTypeLarge]
	if selected.Provider != "deepseek-local" || selected.Model != "deepseek-v4-flash" {
		t.Fatalf("selected model = %#v", selected)
	}
}
