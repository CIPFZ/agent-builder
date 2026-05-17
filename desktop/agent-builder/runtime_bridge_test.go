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
	layout := desktopLayout{
		Root:            filepath.Join(root, "desktop", "agent-builder", "bin"),
		ConfigDir:       filepath.Join(root, "desktop", "agent-builder", "bin", "config"),
		DataDir:         filepath.Join(root, "desktop", "agent-builder", "bin", "data"),
		LogsDir:         filepath.Join(root, "desktop", "agent-builder", "bin", "logs"),
		ModelConfigPath: filepath.Join(root, "desktop", "agent-builder", "bin", "config", "model.local.json"),
	}
	got := localModelConfigPaths(layout)
	want := layout.ModelConfigPath
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
	layout := desktopLayout{
		Root:            root,
		ConfigDir:       filepath.Join(root, "config"),
		DataDir:         filepath.Join(root, "data"),
		LogsDir:         filepath.Join(root, "logs"),
		ModelConfigPath: filepath.Join(root, "config", "model.local.json"),
	}
	if err := os.MkdirAll(layout.ConfigDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(layout.ModelConfigPath, []byte(`{
  "protocol": "openai",
  "url": "https://api.deepseek.com",
  "apiKey": "test-key",
  "model": "deepseek-v4-flash",
  "models": ["deepseek-v4-flash"]
}`), 0o600); err != nil {
		t.Fatal(err)
	}

	store := config.NewTestStore(&config.Config{
		Providers: csync.NewMap[string, config.ProviderConfig](),
		Models:    map[config.SelectedModelType]config.SelectedModel{},
		Options:   &config.Options{},
	})

	result := applyLocalModelConfig(store, layout)
	if result.Error != nil {
		t.Fatal(result.Error)
	}
	if !result.Applied {
		t.Fatal("local config was not applied")
	}
	if result.Path != layout.ModelConfigPath {
		t.Fatalf("Path = %s, want %s", result.Path, layout.ModelConfigPath)
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

func TestSaveLocalModelConfigWritesDesktopConfig(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	layout := desktopLayout{
		Root:            root,
		ConfigDir:       filepath.Join(root, "config"),
		DataDir:         filepath.Join(root, "data"),
		LogsDir:         filepath.Join(root, "logs"),
		ModelConfigPath: filepath.Join(root, "config", "model.local.json"),
	}

	err := saveLocalModelConfig(layout, RuntimeModelConfig{
		Protocol: "openai",
		URL:      "https://api.deepseek.com",
		APIKey:   "test-key",
		Model:    "deepseek-v4-flash",
	})
	if err != nil {
		t.Fatal(err)
	}

	data, err := os.ReadFile(layout.ModelConfigPath)
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Contains(localModelConfigPaths(layout), layout.ModelConfigPath) {
		t.Fatal("desktop config path is not searched first")
	}
	if string(data) == "" {
		t.Fatal("config file is empty")
	}
}
