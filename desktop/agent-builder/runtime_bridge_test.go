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

	if !slices.Contains(got, want) {
		t.Fatalf("localModelConfigPaths() missing %s in %#v", want, got)
	}
	if len(got) != 1 {
		t.Fatalf("localModelConfigPaths() = %#v, want only %s", got, want)
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
  "url": "https://api.example.com",
  "apiKey": "test-key",
  "model": "example-chat",
  "proxy": "http://127.0.0.1:7890",
  "models": ["example-chat"]
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
	if result.Config.Proxy != "http://127.0.0.1:7890" {
		t.Fatalf("Proxy = %s", result.Config.Proxy)
	}

	provider, ok := store.Config().Providers.Get(localProviderID)
	if !ok {
		t.Fatal("local provider was not configured")
	}
	if provider.APIKey != "test-key" {
		t.Fatal("api key was not applied")
	}
	selected := store.Config().Models[config.SelectedModelTypeLarge]
	if selected.Provider != localProviderID || selected.Model != "example-chat" {
		t.Fatalf("selected model = %#v", selected)
	}
}

func TestApplyDesktopProxySetsAndClearsProxyEnvironment(t *testing.T) {
	t.Setenv("HTTP_PROXY", "")
	t.Setenv("HTTPS_PROXY", "")
	t.Setenv("http_proxy", "")
	t.Setenv("https_proxy", "")

	applyDesktopProxy(localModelConfigResult{
		Applied: true,
		Config:  RuntimeModelConfig{Proxy: "http://127.0.0.1:7890"},
	})

	if os.Getenv("HTTP_PROXY") != "http://127.0.0.1:7890" {
		t.Fatalf("HTTP_PROXY = %q", os.Getenv("HTTP_PROXY"))
	}
	if os.Getenv("HTTPS_PROXY") != "http://127.0.0.1:7890" {
		t.Fatalf("HTTPS_PROXY = %q", os.Getenv("HTTPS_PROXY"))
	}

	applyDesktopProxy(localModelConfigResult{Applied: true})
	if os.Getenv("HTTP_PROXY") != "" || os.Getenv("HTTPS_PROXY") != "" || os.Getenv("http_proxy") != "" || os.Getenv("https_proxy") != "" {
		t.Fatal("proxy environment was not cleared")
	}
}

func TestLocalModelConfigIgnoresLegacyFile(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	layout := desktopLayout{
		Root:            root,
		ConfigDir:       filepath.Join(root, "config"),
		DataDir:         filepath.Join(root, "data"),
		LogsDir:         filepath.Join(root, "logs"),
		ModelConfigPath: filepath.Join(root, "config", "model.local.json"),
	}
	legacyDir := filepath.Join(root, "client", "server")
	if err := os.MkdirAll(legacyDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(legacyDir, "deepseek.local.json"), []byte("\xef\xbb\xbf{}"), 0o600); err != nil {
		t.Fatal(err)
	}

	_, result := loadLocalModelConfig(layout)
	if result.Error != nil {
		t.Fatal(result.Error)
	}
	if result.Applied {
		t.Fatal("legacy model config should not be loaded")
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
		URL:      "https://api.example.com",
		APIKey:   "test-key",
		Model:    "example-chat",
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
