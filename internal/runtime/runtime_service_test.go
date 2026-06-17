package runtime

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/charmbracelet/crush/internal/agent"
	mcptools "github.com/charmbracelet/crush/internal/agent/tools/mcp"
	"github.com/charmbracelet/crush/internal/backend"
	"github.com/charmbracelet/crush/internal/config"
	"github.com/charmbracelet/crush/internal/csync"
	"github.com/charmbracelet/crush/internal/db"
	"github.com/charmbracelet/crush/internal/message"
	"github.com/charmbracelet/crush/internal/permission"
	"github.com/charmbracelet/crush/internal/proto"
	"github.com/charmbracelet/crush/internal/pubsub"
	"github.com/charmbracelet/crush/internal/runtimeapi"
	"github.com/charmbracelet/crush/internal/session"
	"github.com/charmbracelet/crush/internal/tools/scheduler"
)

func TestLocalModelConfigUsesDesktopConfigPath(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	layout := desktopLayout{
		Root:            filepath.Join(root, "desktop", "agent-builder", "bin"),
		ConfigDir:       filepath.Join(root, "desktop", "agent-builder", "bin", "config"),
		DataDir:         filepath.Join(root, "desktop", "agent-builder", "bin", "data"),
		LogsDir:         filepath.Join(root, "desktop", "agent-builder", "bin", "logs"),
		ModelConfigPath: filepath.Join(root, "desktop", "agent-builder", "bin", "config", "model.json"),
	}
	if filepath.Base(layout.ModelConfigPath) != "model.json" {
		t.Fatalf("ModelConfigPath = %s, want model.json", layout.ModelConfigPath)
	}
}

func TestRuntimeOpenProjectCreatesSwitchesAndClosesTerminals(t *testing.T) {
	root := runtimeDevTestRoot(t, "open-project")
	t.Setenv("AGENT_BUILDER_DESKTOP_ROOT", root)
	writeRuntimeDevModelConfig(t, root, "http://127.0.0.1:1")
	oldProject := filepath.Join(t.TempDir(), "old-project")
	newProject := filepath.Join(t.TempDir(), "new-project")
	if err := os.MkdirAll(oldProject, 0o755); err != nil {
		t.Fatal(err)
	}

	service := newRuntimeService()
	opened, err := service.OpenProject(context.Background(), RuntimeOpenProjectRequest{Path: oldProject})
	if err != nil {
		t.Fatal(err)
	}
	if opened.Project.Path != oldProject || !opened.Project.Current || opened.Status.WorkingDir != oldProject {
		t.Fatalf("opened project = %#v", opened)
	}
	service.mu.Lock()
	workspaceID := service.workspace.ID
	service.mu.Unlock()
	sess, err := service.runtime.CreateSession(context.Background(), workspaceID, "Terminal owner")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := service.CreateTerminal(context.Background(), RuntimeTerminalCreateRequest{SessionID: sess.ID, ID: "term-project-switch"}); err != nil {
		t.Fatal(err)
	}

	opened, err = service.OpenProject(context.Background(), RuntimeOpenProjectRequest{Path: newProject, CreateMissing: true})
	if err != nil {
		t.Fatal(err)
	}
	if opened.Project.Path != newProject || opened.Project.Name != filepath.Base(newProject) || opened.Status.WorkingDir != newProject {
		t.Fatalf("switched project = %#v", opened)
	}
	if _, err := os.Stat(newProject); err != nil {
		t.Fatalf("new project was not created: %v", err)
	}
	sessions, err := service.Sessions(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(sessions.Sessions) != 0 {
		t.Fatalf("new project should not list old project sessions: %#v", sessions.Sessions)
	}
	if _, err := service.WriteTerminalInput(context.Background(), "term-project-switch", RuntimeTerminalInputRequest{Data: terminalTestCommand("echo stale")}); err == nil {
		t.Fatal("terminal should be closed after project switch")
	}
	service.mu.Lock()
	terminalCount := len(service.terminalsByID)
	ownershipCount := len(service.terminalIDsBySession)
	service.mu.Unlock()
	if terminalCount != 0 || ownershipCount != 0 {
		t.Fatalf("terminal maps after project switch = terminals:%d ownership:%d, want 0/0", terminalCount, ownershipCount)
	}
}

func TestRuntimeCreateProjectUsesDesktopDataProjectsDirectory(t *testing.T) {
	root := runtimeDevTestRoot(t, "create-project")
	t.Setenv("AGENT_BUILDER_DESKTOP_ROOT", root)
	writeRuntimeDevModelConfig(t, root, "http://127.0.0.1:1")

	service := newRuntimeService()
	created, err := service.CreateProject(context.Background(), RuntimeCreateProjectRequest{Name: "Blank Project"})
	if err != nil {
		t.Fatal(err)
	}

	wantPath := filepath.Join(root, "data", "projects", "Blank Project")
	if created.Project.Path != wantPath || created.Status.WorkingDir != wantPath {
		t.Fatalf("created project path = %#v status=%#v, want %s", created.Project, created.Status, wantPath)
	}
	if _, err := os.Stat(wantPath); err != nil {
		t.Fatalf("project directory was not created: %v", err)
	}
}

func TestRuntimeCreateProjectRejectsInvalidOrExistingName(t *testing.T) {
	root := runtimeDevTestRoot(t, "create-project-invalid")
	t.Setenv("AGENT_BUILDER_DESKTOP_ROOT", root)
	writeRuntimeDevModelConfig(t, root, "http://127.0.0.1:1")

	service := newRuntimeService()
	for _, name := range []string{"", ".", "..", "nested/project", `nested\project`, "bad:name", "trailing.", "CON", "nul.txt", "COM1", "LPT9"} {
		if _, err := service.CreateProject(context.Background(), RuntimeCreateProjectRequest{Name: name}); err == nil {
			t.Fatalf("CreateProject(%q) succeeded, want error", name)
		}
	}

	existing := filepath.Join(root, "data", "projects", "Existing")
	if err := os.MkdirAll(existing, 0o755); err != nil {
		t.Fatal(err)
	}
	if _, err := service.CreateProject(context.Background(), RuntimeCreateProjectRequest{Name: "Existing"}); err == nil {
		t.Fatal("CreateProject existing directory succeeded, want error")
	}
}

func TestRuntimeCreateProjectSucceedsWithoutConfiguredModel(t *testing.T) {
	root := runtimeDevTestRoot(t, "create-project-no-model")
	t.Setenv("AGENT_BUILDER_DESKTOP_ROOT", root)

	service := newRuntimeService()
	created, err := service.CreateProject(context.Background(), RuntimeCreateProjectRequest{Name: "No Model Project"})
	if err != nil {
		t.Fatal(err)
	}

	wantPath := filepath.Join(root, "data", "projects", "No Model Project")
	if created.Status.Ready {
		t.Fatalf("created status ready = true, want false without configured model")
	}
	if created.Project.Path != wantPath || created.Status.WorkingDir != wantPath || created.Project.ID == "" {
		t.Fatalf("created project = %#v status=%#v, want path %s with fallback ID", created.Project, created.Status, wantPath)
	}

	status, err := service.Status(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if status.Ready || status.WorkingDir != wantPath || status.WorkspaceID != created.Project.ID {
		t.Fatalf("fallback status = %#v, created project ID %q", status, created.Project.ID)
	}
}

func TestRuntimeRenameProjectRenamesDirectoryMigratesSessionsAndClosesTerminals(t *testing.T) {
	root := runtimeDevTestRoot(t, "rename-project")
	t.Setenv("AGENT_BUILDER_DESKTOP_ROOT", root)
	writeRuntimeDevModelConfig(t, root, "http://127.0.0.1:1")

	service := newRuntimeService()
	created, err := service.CreateProject(context.Background(), RuntimeCreateProjectRequest{Name: "Before Rename"})
	if err != nil {
		t.Fatal(err)
	}
	oldPath := created.Project.Path
	oldDataDir := runtimeProjectDataDir(filepath.Join(root, "data"), oldPath)

	service.mu.Lock()
	workspaceID := service.workspace.ID
	service.mu.Unlock()
	session, err := service.runtime.CreateSession(context.Background(), workspaceID, "Keep me")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := service.CreateTerminal(context.Background(), RuntimeTerminalCreateRequest{SessionID: session.ID, ID: "rename-terminal"}); err != nil {
		t.Fatal(err)
	}

	renamed, err := service.RenameProject(context.Background(), RuntimeRenameProjectRequest{
		ProjectID: created.Project.ID,
		Name:      "After Rename",
	})
	if err != nil {
		t.Fatal(err)
	}

	wantPath := filepath.Join(root, "data", "projects", "After Rename")
	if renamed.Project.Name != "After Rename" || renamed.Project.Path != wantPath || renamed.Status.WorkingDir != wantPath {
		t.Fatalf("renamed project = %#v status=%#v, want path %s", renamed.Project, renamed.Status, wantPath)
	}
	if _, err := os.Stat(oldPath); !os.IsNotExist(err) {
		t.Fatalf("old project path still exists or stat failed unexpectedly: %v", err)
	}
	if _, err := os.Stat(wantPath); err != nil {
		t.Fatalf("renamed project path missing: %v", err)
	}
	if _, err := os.Stat(oldDataDir); !os.IsNotExist(err) {
		t.Fatalf("old project data dir still exists or stat failed unexpectedly: %v", err)
	}
	if _, err := os.Stat(runtimeProjectDataDir(filepath.Join(root, "data"), wantPath)); err != nil {
		t.Fatalf("renamed project data dir missing: %v", err)
	}

	sessions, err := service.Sessions(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(sessions.Sessions) != 1 || sessions.Sessions[0].ID != session.ID || sessions.Sessions[0].Title != "Keep me" {
		t.Fatalf("sessions after rename = %#v", sessions.Sessions)
	}
	if _, err := service.WriteTerminalInput(context.Background(), "rename-terminal", RuntimeTerminalInputRequest{Data: terminalTestCommand("echo stale")}); err == nil {
		t.Fatal("terminal should be closed after project rename")
	}
	service.mu.Lock()
	terminalCount := len(service.terminalsByID)
	ownershipCount := len(service.terminalIDsBySession)
	service.mu.Unlock()
	if terminalCount != 0 || ownershipCount != 0 {
		t.Fatalf("terminal maps after project rename = terminals:%d ownership:%d, want 0/0", terminalCount, ownershipCount)
	}
}

func TestRuntimeRenameProjectRejectsInvalidExistingOrNonCurrentProject(t *testing.T) {
	root := runtimeDevTestRoot(t, "rename-project-invalid")
	t.Setenv("AGENT_BUILDER_DESKTOP_ROOT", root)
	writeRuntimeDevModelConfig(t, root, "http://127.0.0.1:1")

	service := newRuntimeService()
	created, err := service.CreateProject(context.Background(), RuntimeCreateProjectRequest{Name: "Current"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := service.RenameProject(context.Background(), RuntimeRenameProjectRequest{ProjectID: created.Project.ID, Name: "bad/name"}); err == nil {
		t.Fatal("RenameProject accepted invalid name")
	}
	if _, err := service.RenameProject(context.Background(), RuntimeRenameProjectRequest{ProjectID: "not-current", Name: "Other"}); err == nil {
		t.Fatal("RenameProject accepted non-current project id")
	}
	existing := filepath.Join(root, "data", "projects", "Existing")
	if err := os.MkdirAll(existing, 0o755); err != nil {
		t.Fatal(err)
	}
	if _, err := service.RenameProject(context.Background(), RuntimeRenameProjectRequest{ProjectID: created.Project.ID, Name: "Existing"}); err == nil {
		t.Fatal("RenameProject overwrote existing directory")
	}
}

func TestRuntimeOpenProjectInExplorerUsesCurrentProjectPath(t *testing.T) {
	root := runtimeDevTestRoot(t, "open-project-explorer")
	t.Setenv("AGENT_BUILDER_DESKTOP_ROOT", root)
	writeRuntimeDevModelConfig(t, root, "http://127.0.0.1:1")

	service := newRuntimeService()
	created, err := service.CreateProject(context.Background(), RuntimeCreateProjectRequest{Name: "Explorer Project"})
	if err != nil {
		t.Fatal(err)
	}

	var openedPath string
	previous := runtimeOpenPathInFileManager
	runtimeOpenPathInFileManager = func(path string) error {
		openedPath = path
		return nil
	}
	t.Cleanup(func() {
		runtimeOpenPathInFileManager = previous
	})

	response, err := service.OpenProjectInExplorer(context.Background(), RuntimeProjectActionRequest{ProjectID: created.Project.ID})
	if err != nil {
		t.Fatal(err)
	}
	if openedPath != created.Project.Path {
		t.Fatalf("opened path = %q, want %q", openedPath, created.Project.Path)
	}
	if response.Project.ID != created.Project.ID || response.Status.WorkingDir != created.Project.Path {
		t.Fatalf("response = %#v", response)
	}
}

func TestRuntimeOpenProjectInExplorerRejectsNonCurrentProject(t *testing.T) {
	root := runtimeDevTestRoot(t, "open-project-explorer-invalid")
	t.Setenv("AGENT_BUILDER_DESKTOP_ROOT", root)
	writeRuntimeDevModelConfig(t, root, "http://127.0.0.1:1")

	service := newRuntimeService()
	if _, err := service.CreateProject(context.Background(), RuntimeCreateProjectRequest{Name: "Explorer Project"}); err != nil {
		t.Fatal(err)
	}

	called := false
	previous := runtimeOpenPathInFileManager
	runtimeOpenPathInFileManager = func(path string) error {
		called = true
		return nil
	}
	t.Cleanup(func() {
		runtimeOpenPathInFileManager = previous
	})

	if _, err := service.OpenProjectInExplorer(context.Background(), RuntimeProjectActionRequest{ProjectID: "not-current"}); err == nil {
		t.Fatal("OpenProjectInExplorer accepted non-current project id")
	}
	if called {
		t.Fatal("OpenProjectInExplorer called file manager for non-current project")
	}
}

func TestRuntimeRemoveProjectArchivesAppDataKeepsProjectDirectoryAndClosesTerminals(t *testing.T) {
	root := runtimeDevTestRoot(t, "remove-project")
	t.Setenv("AGENT_BUILDER_DESKTOP_ROOT", root)
	writeRuntimeDevModelConfig(t, root, "http://127.0.0.1:1")

	service := newRuntimeService()
	created, err := service.CreateProject(context.Background(), RuntimeCreateProjectRequest{Name: "Remove Me"})
	if err != nil {
		t.Fatal(err)
	}
	projectPath := created.Project.Path
	projectDataDir := runtimeProjectDataDir(filepath.Join(root, "data"), projectPath)

	service.mu.Lock()
	workspaceID := service.workspace.ID
	service.mu.Unlock()
	session, err := service.runtime.CreateSession(context.Background(), workspaceID, "Remove terminal owner")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := service.CreateTerminal(context.Background(), RuntimeTerminalCreateRequest{SessionID: session.ID, ID: "remove-terminal"}); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(projectDataDir); err != nil {
		t.Fatalf("project data dir missing before remove: %v", err)
	}

	removed, err := service.RemoveProject(context.Background(), RuntimeProjectActionRequest{ProjectID: created.Project.ID})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(projectPath); err != nil {
		t.Fatalf("project directory should be kept: %v", err)
	}
	if _, err := os.Stat(projectDataDir); !os.IsNotExist(err) {
		t.Fatalf("project data dir still exists or stat failed unexpectedly: %v", err)
	}
	if removed.Status.WorkingDir == projectPath {
		t.Fatalf("remove response still points at removed project: %#v", removed.Status)
	}
	if _, err := service.WriteTerminalInput(context.Background(), "remove-terminal", RuntimeTerminalInputRequest{Data: terminalTestCommand("echo stale")}); err == nil {
		t.Fatal("terminal should be closed after project remove")
	}
	service.mu.Lock()
	terminalCount := len(service.terminalsByID)
	ownershipCount := len(service.terminalIDsBySession)
	service.mu.Unlock()
	if terminalCount != 0 || ownershipCount != 0 {
		t.Fatalf("terminal maps after project remove = terminals:%d ownership:%d, want 0/0", terminalCount, ownershipCount)
	}
}

func TestRuntimeRemoveProjectRejectsNonCurrentProject(t *testing.T) {
	root := runtimeDevTestRoot(t, "remove-project-invalid")
	t.Setenv("AGENT_BUILDER_DESKTOP_ROOT", root)
	writeRuntimeDevModelConfig(t, root, "http://127.0.0.1:1")

	service := newRuntimeService()
	if _, err := service.CreateProject(context.Background(), RuntimeCreateProjectRequest{Name: "Remove Current"}); err != nil {
		t.Fatal(err)
	}
	if _, err := service.RemoveProject(context.Background(), RuntimeProjectActionRequest{ProjectID: "not-current"}); err == nil {
		t.Fatal("RemoveProject accepted non-current project id")
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
		ModelConfigPath: filepath.Join(root, "config", "model.json"),
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

func TestApplyModelConfigSelectsConfiguredModelFromDiscoveredList(t *testing.T) {
	t.Parallel()

	store := config.NewTestStore(&config.Config{
		Providers: csync.NewMap[string, config.ProviderConfig](),
		Models:    map[config.SelectedModelType]config.SelectedModel{},
		Options:   &config.Options{},
	})

	applyModelConfig(store, RuntimeModelConfig{
		Protocol: "openai",
		URL:      "https://api.example.com",
		APIKey:   "test-key",
		Model:    "deepseek-v4-pro",
		Models:   []string{"deepseek-v4-flash", "deepseek-v4-pro"},
	})

	selected := store.Config().Models[config.SelectedModelTypeLarge]
	if selected.Model != "deepseek-v4-pro" {
		t.Fatalf("selected model = %q, want configured model", selected.Model)
	}
}

func TestSelectedModelStorePersistsGlobalSelection(t *testing.T) {
	t.Parallel()

	dataDir := filepath.Join(t.TempDir(), "runtime-state")
	if err := os.MkdirAll(dataDir, 0o700); err != nil {
		t.Fatal(err)
	}
	conn, err := db.Connect(context.Background(), dataDir)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_ = conn.Close()
		_ = db.Release(dataDir)
	})

	providerStore := newRuntimeProviderSettingsStore(conn)
	if err := providerStore.SyncCatalog(context.Background(), embeddedProviderCatalog()); err != nil {
		t.Fatal(err)
	}
	provider, err := providerStore.UpsertConfigured(context.Background(), RuntimeConfiguredProviderRequest{
		ID:           "deepseek-main",
		ProviderID:   "deepseek",
		Name:         "DeepSeek",
		Protocol:     "openai-compat",
		APIEndpoint:  "https://api.deepseek.com",
		APIKey:       "test-key",
		DefaultModel: "deepseek-v4-flash",
		Enabled:      true,
	})
	if err != nil {
		t.Fatal(err)
	}

	selectedStore := newRuntimeSelectedModelStore(conn)
	selected, err := selectedStore.Upsert(context.Background(), RuntimeSelectedModelRequest{
		ConfiguredProviderID: provider.ID,
		Model:                "deepseek-v4-pro",
		Scope:                "global",
	}, provider)
	if err != nil {
		t.Fatal(err)
	}
	if selected.ID != "global" || selected.ConfiguredProviderID != provider.ID || selected.Model != "deepseek-v4-pro" {
		t.Fatalf("selected model = %#v", selected)
	}

	loaded, err := selectedStore.Get(context.Background(), "global", "", "")
	if err != nil {
		t.Fatal(err)
	}
	if loaded.Model != "deepseek-v4-pro" || loaded.ProviderID != "deepseek" {
		t.Fatalf("loaded selected model = %#v", loaded)
	}
}

func TestSaveConfiguredProviderMaintainsGlobalSelectedModel(t *testing.T) {
	root := runtimeDevTestRoot(t, "selected-provider-save")
	t.Setenv("AGENT_BUILDER_DESKTOP_ROOT", root)
	service := newRuntimeService()

	provider, err := service.SaveConfiguredProvider(context.Background(), RuntimeConfiguredProviderRequest{
		ID:           "custom-main",
		ProviderID:   "custom",
		Name:         "Custom Main",
		Protocol:     "openai-compat",
		APIEndpoint:  "http://127.0.0.1:9999",
		APIKey:       "test-key",
		DefaultModel: "model-a",
		Enabled:      true,
	})
	if err != nil {
		t.Fatal(err)
	}
	if provider.Provider.ID != "custom-main" {
		t.Fatalf("provider = %#v", provider.Provider)
	}

	selected, err := service.SelectedModel(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if selected.SelectedModel.ConfiguredProviderID != "custom-main" || selected.SelectedModel.Model != "model-a" {
		t.Fatalf("selected model after create = %#v", selected.SelectedModel)
	}

	if _, err := service.SaveConfiguredProvider(context.Background(), RuntimeConfiguredProviderRequest{
		ID:           "custom-main",
		ProviderID:   "custom",
		Name:         "Custom Main",
		Protocol:     "openai-compat",
		APIEndpoint:  "http://127.0.0.1:9998",
		DefaultModel: "model-b",
		Enabled:      true,
	}); err != nil {
		t.Fatal(err)
	}

	selected, err = service.SelectedModel(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if selected.SelectedModel.ConfiguredProviderID != "custom-main" || selected.SelectedModel.Model != "model-b" {
		t.Fatalf("selected model after update = %#v", selected.SelectedModel)
	}
}

func TestSaveConfiguredProviderAlwaysNormalizesProviderEnabled(t *testing.T) {
	root := runtimeDevTestRoot(t, "provider-enabled-normalized")
	t.Setenv("AGENT_BUILDER_DESKTOP_ROOT", root)
	service := newRuntimeService()

	provider, err := service.SaveConfiguredProvider(context.Background(), RuntimeConfiguredProviderRequest{
		ID:           "custom-main",
		ProviderID:   "custom",
		Name:         "Custom Main",
		Protocol:     "openai-compat",
		APIEndpoint:  "http://127.0.0.1:9999",
		APIKey:       "test-key",
		DefaultModel: "model-a",
		Enabled:      false,
	})
	if err != nil {
		t.Fatal(err)
	}
	if !provider.Provider.Enabled {
		t.Fatalf("provider enabled = false after save: %#v", provider.Provider)
	}

	selected, err := service.SelectedModel(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if selected.SelectedModel.ConfiguredProviderID != "custom-main" || selected.SelectedModel.Model != "model-a" {
		t.Fatalf("selected model after disabled request = %#v", selected.SelectedModel)
	}
}

func TestConfiguredProvidersExposeSavedAPIKeyForSettingsEdit(t *testing.T) {
	root := runtimeDevTestRoot(t, "provider-edit-api-key")
	t.Setenv("AGENT_BUILDER_DESKTOP_ROOT", root)
	service := newRuntimeService()

	saved, err := service.SaveConfiguredProvider(context.Background(), RuntimeConfiguredProviderRequest{
		ID:           "custom-main",
		ProviderID:   "custom",
		Name:         "Custom Main",
		Protocol:     "openai-compat",
		APIEndpoint:  "http://127.0.0.1:9999",
		APIKey:       "test-secret-key",
		DefaultModel: "model-a",
	})
	if err != nil {
		t.Fatal(err)
	}
	if saved.Provider.APIKey != "test-secret-key" || !saved.Provider.HasAPIKey {
		t.Fatalf("saved provider api key = %q has=%v", saved.Provider.APIKey, saved.Provider.HasAPIKey)
	}

	configured, err := service.ConfiguredProviders(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(configured.Providers) != 1 {
		t.Fatalf("providers = %#v", configured.Providers)
	}
	if configured.Providers[0].APIKey != "test-secret-key" || !configured.Providers[0].HasAPIKey {
		t.Fatalf("listed provider api key = %q has=%v", configured.Providers[0].APIKey, configured.Providers[0].HasAPIKey)
	}
}

func TestConfiguredProviderModelsExposeSavedModelList(t *testing.T) {
	root := runtimeDevTestRoot(t, "provider-saved-model-list")
	t.Setenv("AGENT_BUILDER_DESKTOP_ROOT", root)
	service := newRuntimeService()

	if _, err := service.SaveConfiguredProvider(context.Background(), RuntimeConfiguredProviderRequest{
		ID:           "deepseek-main",
		ProviderID:   "deepseek",
		Name:         "DeepSeek",
		Protocol:     "openai-compat",
		APIEndpoint:  "https://api.deepseek.com/v1",
		APIKey:       "test-secret-key",
		DefaultModel: "deepseek-v4-flash",
		Models:       []string{"deepseek-v4-flash", "deepseek-v4-pro"},
	}); err != nil {
		t.Fatal(err)
	}

	models, err := service.Models(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(models.Models) != 2 {
		t.Fatalf("models = %#v", models.Models)
	}
	if models.Models[0].Name != "deepseek-v4-flash" || !models.Models[0].Selected {
		t.Fatalf("first model = %#v", models.Models[0])
	}
	if models.Models[1].Name != "deepseek-v4-pro" || models.Models[1].ConfiguredProviderID != "deepseek-main" {
		t.Fatalf("second model = %#v", models.Models[1])
	}

	if _, err := service.SaveSelectedModel(context.Background(), RuntimeSelectedModelRequest{
		ConfiguredProviderID: "deepseek-main",
		Model:                "deepseek-v4-pro",
		Scope:                "global",
	}); err != nil {
		t.Fatal(err)
	}
	models, err = service.Models(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if !models.Models[1].Selected {
		t.Fatalf("selected models = %#v", models.Models)
	}
}

func TestSaveConfiguredProviderRejectsDuplicateName(t *testing.T) {
	root := runtimeDevTestRoot(t, "provider-duplicate-name")
	t.Setenv("AGENT_BUILDER_DESKTOP_ROOT", root)
	service := newRuntimeService()

	if _, err := service.SaveConfiguredProvider(context.Background(), RuntimeConfiguredProviderRequest{
		ID:           "custom-main",
		ProviderID:   "custom",
		Name:         "Custom Main",
		Protocol:     "openai-compat",
		APIEndpoint:  "https://API.example.com/v1/",
		APIKey:       "test-key",
		DefaultModel: "model-a",
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := service.SaveConfiguredProvider(context.Background(), RuntimeConfiguredProviderRequest{
		ID:           "custom-duplicate",
		ProviderID:   "custom",
		Name:         "custom main",
		Protocol:     "openai-compat",
		APIEndpoint:  "https://api.example.com/v1",
		APIKey:       "test-key",
		DefaultModel: "model-a",
	}); !errors.Is(err, errConfiguredProviderDuplicate) {
		t.Fatalf("duplicate save error = %v, want %v", err, errConfiguredProviderDuplicate)
	}
	if _, err := service.SaveConfiguredProvider(context.Background(), RuntimeConfiguredProviderRequest{
		ID:           "custom-main",
		ProviderID:   "custom",
		Name:         "Custom Main Renamed",
		Protocol:     "openai-compat",
		APIEndpoint:  "https://api.example.com/v1",
		DefaultModel: "model-b",
	}); err != nil {
		t.Fatalf("same provider update failed: %v", err)
	}
	if _, err := service.SaveConfiguredProvider(context.Background(), RuntimeConfiguredProviderRequest{
		ID:           "custom-secondary",
		ProviderID:   "custom",
		Name:         "Custom Secondary",
		Protocol:     "openai-compat",
		APIEndpoint:  "https://api.example.com/v1",
		APIKey:       "different-test-key",
		DefaultModel: "model-a",
	}); err != nil {
		t.Fatalf("same provider endpoint with a different name failed: %v", err)
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
		ModelConfigPath: filepath.Join(root, "config", "model.json"),
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
		ModelConfigPath: filepath.Join(root, "config", "model.json"),
	}

	err := saveLocalModelConfig(layout, RuntimeModelConfig{
		Protocol: "openai",
		URL:      "https://api.example.com",
		APIKey:   "test-key",
		Model:    "example-chat",
		Models:   []string{"example-chat", "example-reasoner"},
	})
	if err != nil {
		t.Fatal(err)
	}

	data, err := os.ReadFile(layout.ModelConfigPath)
	if err != nil {
		t.Fatal(err)
	}
	if string(data) == "" {
		t.Fatal("config file is empty")
	}
	var saved RuntimeModelConfig
	if err := json.Unmarshal(data, &saved); err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(saved.Models, []string{"example-chat", "example-reasoner"}) {
		t.Fatalf("Models = %#v", saved.Models)
	}
}

func TestDiscoverModelIDsOpenAICompatible(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/models" {
			t.Fatalf("Path = %s", r.URL.Path)
		}
		if r.Header.Get("Authorization") != "Bearer test-key" {
			t.Fatalf("Authorization = %q", r.Header.Get("Authorization"))
		}
		_, _ = w.Write([]byte(`{"data":[{"id":"deepseek-chat"},{"id":"deepseek-reasoner"},{"id":"deepseek-chat"}]}`))
	}))
	t.Cleanup(server.Close)

	models, err := discoverModelIDs(context.Background(), RuntimeModelConfig{
		Protocol: "openai",
		URL:      server.URL,
		APIKey:   "test-key",
	})
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(models, []string{"deepseek-chat", "deepseek-reasoner"}) {
		t.Fatalf("models = %#v", models)
	}
}

func TestVerifyModelConfigRejectsIncompleteConfig(t *testing.T) {
	t.Parallel()

	service := newRuntimeService()
	_, err := service.VerifyModelConfig(context.Background(), RuntimeModelConfig{
		Protocol: "openai",
		Model:    "test-model",
	})
	if err == nil {
		t.Fatal("expected validation error")
	}
}

func TestDiscoverModelConfigDoesNotRequireModel(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/models" {
			t.Fatalf("Path = %s", r.URL.Path)
		}
		if r.Header.Get("Authorization") != "Bearer test-key" {
			t.Fatalf("Authorization = %q", r.Header.Get("Authorization"))
		}
		_, _ = w.Write([]byte(`{"data":[{"id":"model-a"},{"id":"model-b"}]}`))
	}))
	t.Cleanup(server.Close)

	service := newRuntimeService()
	resp, err := service.DiscoverModelConfig(context.Background(), RuntimeModelConfig{
		Protocol: "openai",
		URL:      server.URL,
		APIKey:   "test-key",
	})
	if err != nil {
		t.Fatal(err)
	}
	if resp.Error != "" || !slices.Equal(resp.Models, []string{"model-a", "model-b"}) || resp.Model != "" {
		t.Fatalf("discovery response = %#v", resp)
	}
}

func TestRuntimeMessagePartsExposeToolActivity(t *testing.T) {
	t.Parallel()

	msg := proto.Message{
		ID:        "assistant-1",
		SessionID: "session-1",
		Role:      proto.Assistant,
		Parts: []proto.ContentPart{
			proto.ReasoningContent{Thinking: "Need to inspect files."},
			proto.ToolCall{ID: "tool-1", Name: "ls", Input: `{"path":"."}`, Finished: true},
			proto.Finish{Reason: proto.FinishReasonToolUse},
		},
	}

	got := toRuntimeMessage(msg)
	if !isDisplayableRuntimeMessage(got) {
		t.Fatal("tool-use assistant message should be displayable")
	}
	if got.FinishReason != string(proto.FinishReasonToolUse) {
		t.Fatalf("FinishReason = %q", got.FinishReason)
	}
	if len(got.Parts) != 3 {
		t.Fatalf("Parts len = %d, want 3", len(got.Parts))
	}
	if got.Parts[0].Type != "reasoning" || got.Parts[0].Thinking == "" {
		t.Fatalf("reasoning part not exposed: %#v", got.Parts[0])
	}
	if got.Parts[1].Type != "tool_call" || got.Parts[1].ToolCallID != "tool-1" || got.Parts[1].Name != "ls" {
		t.Fatalf("tool call part not exposed: %#v", got.Parts[1])
	}
}

func TestRuntimeSkillsFromConfigIncludesBuiltinAndDisabledState(t *testing.T) {
	t.Parallel()

	store := config.NewTestStore(&config.Config{
		Options: &config.Options{
			DisabledSkills: []string{"crush-config"},
		},
	})

	resp := runtimeSkillsFromConfig(store)
	var found *RuntimeSkill
	for i := range resp.Skills {
		if resp.Skills[i].Name == "crush-config" {
			found = &resp.Skills[i]
			break
		}
	}
	if found == nil {
		t.Fatal("builtin crush-config skill was not exposed")
	}
	if !found.Builtin {
		t.Fatalf("Builtin = false for %#v", found)
	}
	if found.Enabled {
		t.Fatalf("Enabled = true for disabled skill %#v", found)
	}
	if found.State != capabilityStateDisabled {
		t.Fatalf("State = %q, want disabled", found.State)
	}
	if found.Activation.Included {
		t.Fatalf("Activation included disabled skill %#v", found.Activation)
	}
}

func TestRuntimeMCPServersFromConfigRedactsSecrets(t *testing.T) {
	t.Parallel()

	store := config.NewTestStore(&config.Config{
		MCP: config.MCPs{
			"docs": {
				Type:    config.MCPHttp,
				URL:     "https://user:secret@example.com/mcp",
				Headers: map[string]string{"Authorization": "Bearer secret-token", "X-Team": "docs"},
				Env:     map[string]string{"API_TOKEN": "secret-token", "MODE": "test"},
			},
		},
		Options: &config.Options{},
	})

	resp := runtimeMCPServersFromConfig(store)
	if len(resp.Servers) != 1 {
		t.Fatalf("servers = %#v", resp.Servers)
	}
	server := resp.Servers[0]
	if server.URL != "[REDACTED_URL]" {
		t.Fatalf("URL = %q, want redacted", server.URL)
	}
	if server.Headers["Authorization"] != "[REDACTED]" {
		t.Fatalf("Authorization header was not redacted: %#v", server.Headers)
	}
	if server.Headers["X-Team"] != "docs" {
		t.Fatalf("non-secret header was redacted: %#v", server.Headers)
	}
	if server.Env["API_TOKEN"] != "[REDACTED]" || server.Env["MODE"] != "test" {
		t.Fatalf("env redaction failed: %#v", server.Env)
	}
}

func TestRuntimePolicyLoadSaveAndUpdate(t *testing.T) {
	root := t.TempDir()
	t.Setenv("AGENT_BUILDER_DESKTOP_ROOT", root)
	service := newRuntimeService()

	resp, err := service.GetPolicy(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if resp.Policy.Mode != "ask" {
		t.Fatalf("default policy = %#v", resp.Policy)
	}

	updated, err := service.UpdatePolicy(context.Background(), RuntimePolicyUpdateRequest{Mode: "full_access"})
	if err != nil {
		t.Fatal(err)
	}
	if updated.Policy.Mode != "full_access" || updated.Policy.Description == "" || !slices.Contains(updated.Policy.Modes, "full_access") {
		t.Fatalf("updated policy = %#v", updated.Policy)
	}

	loaded, err := loadRuntimePolicy(desktopLayout{
		ConfigDir:        filepath.Join(root, "config"),
		DataDir:          filepath.Join(root, "data"),
		LogsDir:          filepath.Join(root, "logs"),
		PolicyConfigPath: filepath.Join(root, "config", "policy.json"),
	})
	if err != nil {
		t.Fatal(err)
	}
	if loaded.Mode != "full_access" {
		t.Fatalf("loaded policy = %#v", loaded)
	}
}

func TestRuntimePolicyApplicationEventAndAudit(t *testing.T) {
	dataDir := filepath.Join(t.TempDir(), "runtime-state")
	if err := os.MkdirAll(dataDir, 0o700); err != nil {
		t.Fatal(err)
	}
	conn, err := db.Connect(context.Background(), dataDir)
	if err != nil {
		t.Fatal(err)
	}
	service := newRuntimeService()
	service.turns = newRuntimeTurnStore(conn)
	service.toolCalls = scheduler.New(NewRuntimeToolCallStoreForDB(conn))
	service.permissionStore = newRuntimePermissionStore(conn)
	workingDir := t.TempDir()
	cfg := config.NewRuntimeConfig(workingDir, dataDir, false)
	cfg.Options.AutoLSP = ptr(false)
	store := config.NewRuntimeStore(workingDir, cfg)
	runtimeBackend := backend.New(context.Background(), store, nil)
	_, workspace, err := runtimeBackend.CreateWorkspace(proto.Workspace{
		Path:    workingDir,
		DataDir: dataDir,
		Config:  store.Config(),
	})
	if err != nil {
		t.Fatal(err)
	}
	service.runtime = runtimeBackend
	service.workspace = &proto.Workspace{ID: workspace.ID, Path: workspace.Path}

	permissions := permission.NewPermissionService(t.TempDir(), false, nil)
	permissions.SetPolicyMode(permission.PolicyModePlan)
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(func() {
		cancel()
		runtimeBackend.DeleteWorkspace(workspace.ID)
		_ = conn.Close()
		_ = db.Release(dataDir)
	})
	go service.consumePermissionPolicyApplications(ctx, workspace.ID, permissions)
	time.Sleep(10 * time.Millisecond)

	granted, err := permissions.Request(context.Background(), permission.CreatePermissionRequest{
		SessionID:   "session-1",
		TurnID:      "turn-1",
		ToolCallID:  "tool-1",
		ToolName:    "bash",
		Action:      "execute",
		Description: `{"command":"go test ./..."}`,
		Path:        t.TempDir(),
	})
	if err != nil {
		t.Fatal(err)
	}
	if granted {
		t.Fatal("plan mode should deny execute tool calls")
	}

	var events []RuntimeEvent
	for i := 0; i < 20; i++ {
		history, _ := service.Events(context.Background())
		events = history.Events
		if slices.ContainsFunc(events, func(event RuntimeEvent) bool {
			return event.Type == runtimeapi.EventPermissionPolicyApplied && event.ToolCallID == "tool-1"
		}) {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	if !slices.ContainsFunc(events, func(event RuntimeEvent) bool {
		return event.Type == runtimeapi.EventPermissionPolicyApplied && event.ToolCallID == "tool-1" && event.Payload["mode"] == permission.PolicyModePlan
	}) {
		t.Fatalf("policy applied event missing from %#v", events)
	}

	var audit RuntimeAuditResponse
	for i := 0; i < 20; i++ {
		audit, err = newRuntimeAuditStore(conn).ListTurn(context.Background(), "turn-1")
		if err != nil {
			t.Fatal(err)
		}
		if slices.ContainsFunc(audit.Events, func(event RuntimeAuditEvent) bool {
			return event.Type == "permission_policy_applied" && event.Payload["policy_mode"] == string(permission.PolicyModePlan)
		}) {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	if !slices.ContainsFunc(audit.Events, func(event RuntimeAuditEvent) bool {
		return event.Type == "permission_policy_applied" && event.Payload["policy_mode"] == string(permission.PolicyModePlan)
	}) {
		t.Fatalf("policy audit missing from %#v", audit.Events)
	}
}

func TestRuntimeSchedulerAskCreatesRecoverablePermission(t *testing.T) {
	dataDir := filepath.Join(t.TempDir(), "runtime-state")
	t.Cleanup(db.ResetPool)
	if err := os.MkdirAll(dataDir, 0o700); err != nil {
		t.Fatal(err)
	}
	conn, err := db.Connect(context.Background(), dataDir)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_ = db.Release(dataDir)
	})

	service := newRuntimeService()
	service.policy = runtimePolicyFromMode(permission.PolicyModeAutoRead, 0)
	service.turns = newRuntimeTurnStore(conn)
	service.toolCalls = scheduler.New(NewRuntimeToolCallStoreForDB(conn))
	service.permissionStore = newRuntimePermissionStore(conn)
	workingDir := t.TempDir()
	cfg := config.NewRuntimeConfig(workingDir, dataDir, false)
	cfg.Options.AutoLSP = ptr(false)
	store := config.NewRuntimeStore(workingDir, cfg)
	runtimeBackend := backend.New(context.Background(), store, nil)
	_, workspace, err := runtimeBackend.CreateWorkspace(proto.Workspace{
		Path:    workingDir,
		DataDir: dataDir,
		Config:  store.Config(),
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		runtimeBackend.DeleteWorkspace(workspace.ID)
		_ = db.Release(dataDir)
	})
	service.runtime = runtimeBackend
	service.workspace = &proto.Workspace{ID: workspace.ID, Path: workspace.Path}
	if _, err := service.turns.Upsert(context.Background(), RuntimeTurn{
		ID:        "turn-1",
		SessionID: "session-1",
		Status:    turnStatusRunning,
		StartedAt: time.Now().UnixMilli(),
	}); err != nil {
		t.Fatal(err)
	}
	ws, err := runtimeBackend.GetWorkspace(workspace.ID)
	if err != nil {
		t.Fatal(err)
	}
	ws.Permissions.SetPolicyMode(permission.PolicyModeAutoRead)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go service.consumeDesktopPermissions(ctx, workspace.ID, ws.Permissions)
	time.Sleep(10 * time.Millisecond)

	recorder := runtimeSchedulerRecorder{service: service}
	decisionCh := make(chan agent.SchedulerToolPolicyDecision, 1)
	errCh := make(chan error, 1)
	go func() {
		decision, err := recorder.EvaluateToolCall(context.Background(), agent.SchedulerToolCall{
			ID:           "tool-ask",
			SessionID:    "session-1",
			TurnID:       "turn-1",
			Name:         "bash",
			Source:       "shell",
			CapabilityID: "builtin:bash",
			InputSummary: `{"command":"go test ./..."}`,
		})
		if err != nil {
			errCh <- err
			return
		}
		decisionCh <- decision
	}()

	var pending []RuntimePermissionRequest
	for i := 0; i < 50; i++ {
		resp, err := service.Permissions(context.Background())
		if err != nil {
			t.Fatal(err)
		}
		if len(resp.Permissions) > 0 {
			pending = resp.Permissions
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	if len(pending) != 1 {
		t.Fatalf("pending permissions = %#v", pending)
	}
	perm := pending[0]
	if perm.ToolCallID != "tool-ask" || perm.TurnID != "turn-1" || perm.Risk != string(permission.RiskExecute) || perm.PolicyMode != string(permission.PolicyModeAutoRead) || perm.Decision != string(permission.PolicyAsk) {
		t.Fatalf("permission metadata = %#v", perm)
	}
	if perm.PolicyProfile != string(permission.PolicyProfileDefault) || perm.PolicyHeadless {
		t.Fatalf("interactive permission profile = %#v", perm)
	}
	decisionStatus, err := service.DecidePermission(context.Background(), RuntimePermissionDecision{PermissionID: perm.ID, Action: string(proto.PermissionAllow)})
	if err != nil {
		t.Fatal(err)
	}
	if decisionStatus.Action == nil || !decisionStatus.Action.Accepted || decisionStatus.Action.Source.Kind != runtimePermissionDecisionActionSourceKind || decisionStatus.Action.Source.Action != string(proto.PermissionAllow) || decisionStatus.Action.Source.IdempotentBy != "permission_id" {
		t.Fatalf("permission decision action metadata = %#v", decisionStatus.Action)
	}
	if len(decisionStatus.Action.RefreshTargets) == 0 || decisionStatus.Action.Source.StartsWorker || !decisionStatus.Action.Source.BackendOnly || !decisionStatus.Action.Source.SessionActivityParity {
		t.Fatalf("permission decision source/refresh metadata = %#v", decisionStatus.Action)
	}
	plainStatus, err := service.Status(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if plainStatus.Action != nil {
		t.Fatalf("plain status should not carry decision action metadata: %#v", plainStatus.Action)
	}

	select {
	case err := <-errCh:
		t.Fatal(err)
	case decision := <-decisionCh:
		if decision.Decision != string(permission.PolicyAllow) || decision.Risk != string(permission.RiskExecute) || decision.Mode != string(permission.PolicyModeAutoRead) {
			t.Fatalf("decision = %#v", decision)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for policy ask decision")
	}

	after, err := service.Permissions(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(after.Permissions) != 0 {
		t.Fatalf("permission should no longer be pending: %#v", after.Permissions)
	}
}

func TestRuntimeSchedulerFullAccessAllowsWithoutPendingPermission(t *testing.T) {
	dataDir := filepath.Join(t.TempDir(), "runtime-state")
	if err := os.MkdirAll(dataDir, 0o700); err != nil {
		t.Fatal(err)
	}
	conn, err := db.Connect(context.Background(), dataDir)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_ = db.Release(dataDir)
	})

	service := newRuntimeService()
	service.policy = runtimePolicyFromMode(permission.PolicyModeFullAccess, 0)
	service.turns = newRuntimeTurnStore(conn)
	service.toolCalls = scheduler.New(NewRuntimeToolCallStoreForDB(conn))
	service.permissionStore = newRuntimePermissionStore(conn)
	if _, err := service.turns.Upsert(context.Background(), RuntimeTurn{
		ID:        "turn-full-access",
		SessionID: "session-full-access",
		Status:    turnStatusRunning,
		StartedAt: time.Now().UnixMilli(),
	}); err != nil {
		t.Fatal(err)
	}

	recorder := runtimeSchedulerRecorder{service: service}
	decision, err := recorder.EvaluateToolCall(context.Background(), agent.SchedulerToolCall{
		ID:           "tool-full-access",
		SessionID:    "session-full-access",
		TurnID:       "turn-full-access",
		Name:         "bash",
		Source:       "shell",
		CapabilityID: "builtin:bash",
		InputSummary: `{"command":"ping baidu.com"}`,
	})
	if err != nil {
		t.Fatal(err)
	}
	if decision.Decision != string(permission.PolicyAllow) || decision.Mode != string(permission.PolicyModeFullAccess) || decision.Risk != string(permission.RiskExecute) {
		t.Fatalf("decision = %#v", decision)
	}

	pending, err := service.permissionStore.List(context.Background(), permissionStatusPending)
	if err != nil {
		t.Fatal(err)
	}
	if len(pending) != 0 {
		t.Fatalf("full_access should not create pending permissions: %#v", pending)
	}
	permissions, err := service.permissionStore.ListBySession(context.Background(), "session-full-access")
	if err != nil {
		t.Fatal(err)
	}
	if len(permissions) != 0 {
		t.Fatalf("full_access should not persist permission requests: %#v", permissions)
	}
	service.decrementRunningToolGuard("turn-full-access")
}

func TestRuntimeSessionActivityRestoresToolCallsAndPermissionHistory(t *testing.T) {
	dataDir := filepath.Join(t.TempDir(), "runtime-state")
	if err := os.MkdirAll(dataDir, 0o700); err != nil {
		t.Fatal(err)
	}
	conn, err := db.Connect(context.Background(), dataDir)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_ = db.Release(dataDir)
	})

	service := newRuntimeService()
	service.turns = newRuntimeTurnStore(conn)
	service.toolCalls = scheduler.New(NewRuntimeToolCallStoreForDB(conn))
	service.permissionStore = newRuntimePermissionStore(conn)
	service.policy = runtimePolicyFromMode(permission.PolicyModeAsk, time.Now().UnixMilli())

	if _, err := service.turns.Upsert(context.Background(), RuntimeTurn{
		ID:         "turn-activity",
		SessionID:  "session-activity",
		Status:     turnStatusFailed,
		StartedAt:  time.Now().Add(-time.Second).UnixMilli(),
		FinishedAt: time.Now().UnixMilli(),
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := service.toolCalls.CreateCall(context.Background(), scheduler.ToolCallRequest{
		ID:           "tool-activity",
		SessionID:    "session-activity",
		TurnID:       "turn-activity",
		Name:         "bash",
		Source:       scheduler.ToolSourceShell,
		InputSummary: `{"command":"ping baidu.com"}`,
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := service.toolCalls.CompleteCall(context.Background(), scheduler.ToolCallResult{
		ToolCallID:    "tool-activity",
		Status:        scheduler.ToolCallDenied,
		OutputSummary: "Permission denied.",
		IsError:       true,
		Error:         "Permission denied.",
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := service.permissionStore.Upsert(context.Background(), RuntimePermissionRequest{
		ID:         "perm-activity",
		SessionID:  "session-activity",
		TurnID:     "turn-activity",
		ToolCallID: "tool-activity",
		ToolName:   "bash",
		Action:     "execute",
		Risk:       "execute",
		PolicyMode: "ask",
		Status:     permissionStatusDenied,
		CreatedAt:  time.Now().Add(-time.Second).UnixMilli(),
		DecidedAt:  time.Now().UnixMilli(),
	}); err != nil {
		t.Fatal(err)
	}

	turns, err := service.turns.ListBySession(context.Background(), "session-activity")
	if err != nil {
		t.Fatal(err)
	}
	calls, err := service.toolCalls.ListCalls(context.Background(), "turn-activity")
	if err != nil {
		t.Fatal(err)
	}
	permissions, err := service.permissionStore.ListBySession(context.Background(), "session-activity")
	if err != nil {
		t.Fatal(err)
	}
	if len(turns) != 1 || turns[0].Status != turnStatusFailed {
		t.Fatalf("turns = %#v", turns)
	}
	if len(calls) != 1 || calls[0].Status != scheduler.ToolCallDenied {
		t.Fatalf("calls = %#v", calls)
	}
	if len(permissions) != 1 || permissions[0].Status != permissionStatusDenied {
		t.Fatalf("permissions = %#v", permissions)
	}
}

func TestRuntimeSchedulerHeadlessAskFailsClosed(t *testing.T) {
	t.Parallel()

	h := newRuntimeScenarioHarness(t)
	h.seedTurn("session-headless", "turn-headless")
	h.service.policy = runtimePolicyFromParts(permission.PolicyModeAutoRead, "headless", nil, time.Now().UnixMilli())
	recorder := runtimeSchedulerRecorder{service: h.service}
	decision, err := recorder.EvaluateToolCall(h.ctx, agent.SchedulerToolCall{
		ID:           "tool-headless",
		SessionID:    "session-headless",
		TurnID:       "turn-headless",
		Name:         "bash",
		Source:       string(scheduler.ToolSourceShell),
		CapabilityID: "builtin:bash",
		InputSummary: `{"command":"go test ./..."}`,
	})
	if err != nil {
		t.Fatal(err)
	}
	if decision.Decision != string(permission.PolicyDeny) || decision.Profile != string(permission.PolicyProfileHeadless) || !decision.Headless {
		t.Fatalf("headless decision = %#v", decision)
	}
	replay := h.replay("turn-headless")
	if !slices.ContainsFunc(replay.Summary.PolicyDecisions, func(item RuntimeReplayPolicyDecision) bool {
		return item.ToolCallID == "tool-headless" && item.Headless && item.HeadlessReason != "" && item.Profile == string(permission.PolicyProfileHeadless)
	}) {
		t.Fatalf("headless replay = %#v", replay.Summary.PolicyDecisions)
	}
	h.assertEventType(runtimeapi.EventSchedulerDeadlockPrevented)
}

func TestRuntimeSchedulerHeadlessDeterministicAllowAndDeny(t *testing.T) {
	t.Parallel()

	h := newRuntimeScenarioHarness(t)
	h.seedTurn("session-headless", "turn-headless")
	allow := h.evaluatePolicy(permission.PolicyModeAutoRead, []RuntimePolicyRule{{
		ID: "allow-read-headless", Decision: string(permission.PolicyAllow), Source: "test", BuiltinTool: "view",
	}}, agent.SchedulerToolCall{
		ID: "tool-read", SessionID: "session-headless", TurnID: "turn-headless", Name: "view", Source: string(scheduler.ToolSourceBuiltin), CapabilityID: "builtin:view", InputSummary: `{"file_path":"README.md"}`,
	})
	if allow.Decision != string(permission.PolicyAllow) || allow.Profile != "scenario" {
		t.Fatalf("headless deterministic allow = %#v", allow)
	}
	h.service.policy = runtimePolicyFromParts(permission.PolicyModeAutoRead, "headless", []RuntimePolicyRule{{
		ID: "deny-write-headless", Decision: string(permission.PolicyDeny), Source: "test", BuiltinTool: "write",
	}}, time.Now().UnixMilli())
	recorder := runtimeSchedulerRecorder{service: h.service}
	deny, err := recorder.EvaluateToolCall(h.ctx, agent.SchedulerToolCall{
		ID: "tool-write", SessionID: "session-headless", TurnID: "turn-headless", Name: "write", Source: string(scheduler.ToolSourceBuiltin), CapabilityID: "builtin:write", InputSummary: `{"file_path":"x","content":"y"}`,
	})
	if err != nil {
		t.Fatal(err)
	}
	if deny.Decision != string(permission.PolicyDeny) || deny.RuleID != "deny-write-headless" || !deny.Headless {
		t.Fatalf("headless deterministic deny = %#v", deny)
	}
}

func TestRuntimeMCPConfigFromRequestPreservesArgsAndRedactsResponse(t *testing.T) {
	t.Parallel()

	name, cfg, err := runtimeMCPConfigFromRequest(RuntimeMCPServerConfigRequest{
		Name:          "docs",
		Type:          "http",
		URL:           "https://example.com/mcp",
		Args:          []string{"--token", "$TOKEN"},
		EnabledTools:  []string{"search", "search"},
		DisabledTools: []string{"write"},
		Headers:       map[string]string{"Authorization": "Bearer secret", "X-Team": "docs"},
		Env:           map[string]string{"API_TOKEN": "secret", "MODE": "test"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if name != "docs" {
		t.Fatalf("name = %q", name)
	}
	if got := strings.Join(cfg.Args, " "); got != "--token $TOKEN" {
		t.Fatalf("args order changed: %#v", cfg.Args)
	}
	if len(cfg.EnabledTools) != 1 || cfg.EnabledTools[0] != "search" {
		t.Fatalf("enabled tools = %#v", cfg.EnabledTools)
	}

	resp := runtimeMCPServersFromConfig(config.NewTestStore(&config.Config{
		MCP:     config.MCPs{name: cfg},
		Options: &config.Options{},
	}))
	server := resp.Servers[0]
	if server.Headers["Authorization"] != "[REDACTED]" || server.Headers["X-Team"] != "docs" {
		t.Fatalf("headers were not redacted correctly: %#v", server.Headers)
	}
	if server.Env["API_TOKEN"] != "[REDACTED]" || server.Env["MODE"] != "test" {
		t.Fatalf("env was not redacted correctly: %#v", server.Env)
	}
}

func TestRuntimeMCPConfigFromRequestValidatesNameAndRequiredFields(t *testing.T) {
	t.Parallel()

	if _, _, err := runtimeMCPConfigFromRequest(RuntimeMCPServerConfigRequest{Name: "bad/name", Type: "http", URL: "https://example.com"}); err == nil {
		t.Fatal("expected invalid name error")
	}
	if _, _, err := runtimeMCPConfigFromRequest(RuntimeMCPServerConfigRequest{Name: "docs", Type: "http"}); err == nil {
		t.Fatal("expected missing url error")
	}
	if _, _, err := runtimeMCPConfigFromRequest(RuntimeMCPServerConfigRequest{Name: "docs", Type: "stdio"}); err == nil {
		t.Fatal("expected missing command error")
	}
}

func TestDesktopMCPConfigIsSeparateFromCrushConfig(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	layout := desktopLayout{
		Root:            root,
		ConfigDir:       filepath.Join(root, "config"),
		DataDir:         filepath.Join(root, "data"),
		LogsDir:         filepath.Join(root, "logs"),
		ModelConfigPath: filepath.Join(root, "config", "model.json"),
		SkillConfigPath: filepath.Join(root, "config", "skills.json"),
		MCPConfigPath:   filepath.Join(root, "config", "mcp.json"),
	}
	store := config.NewTestStore(&config.Config{
		MCP: config.MCPs{
			"project": {Type: config.MCPHttp, URL: "https://project.example.com/mcp"},
		},
		Options: &config.Options{},
	})
	if err := saveDesktopMCPConfig(layout, desktopMCPConfig{
		Servers: config.MCPs{
			"desktop": {Type: config.MCPStdio, Command: "npx", Args: []string{"server", "server"}},
		},
	}); err != nil {
		t.Fatal(err)
	}

	if err := applyDesktopMCPConfigToStore(store, layout); err != nil {
		t.Fatal(err)
	}
	if _, ok := store.Config().MCP["project"]; !ok {
		t.Fatalf("project mcp server removed: %#v", store.Config().MCP)
	}
	desktop, ok := store.Config().MCP["desktop"]
	if !ok {
		t.Fatalf("desktop mcp server missing: %#v", store.Config().MCP)
	}
	if desktop.Command != "npx" || !slices.Equal(desktop.Args, []string{"server"}) {
		t.Fatalf("desktop mcp server was not normalized: %#v", desktop)
	}
	if _, err := os.Stat(layout.MCPConfigPath); err != nil {
		t.Fatalf("desktop mcp config was not written: %v", err)
	}
}

func TestRuntimeSkillNameValidation(t *testing.T) {
	t.Parallel()

	if got, err := validateRuntimeSkillName("my-skill_1"); err != nil || got != "my-skill_1" {
		t.Fatalf("validateRuntimeSkillName valid = %q, %v", got, err)
	}
	if _, err := validateRuntimeSkillName("My Skill"); err == nil {
		t.Fatal("validateRuntimeSkillName accepted invalid skill name")
	}
}

func TestRuntimeCapabilitiesIncludeToolsSkillsAndMCP(t *testing.T) {
	t.Parallel()

	store := config.NewTestStore(&config.Config{
		Options: &config.Options{
			DisabledTools:  []string{"bash"},
			DisabledSkills: []string{"crush-config"},
		},
	})
	resp := runtimeCapabilities(
		store,
		RuntimeSkillsResponse{Skills: []RuntimeSkill{{Name: "crush-config", Enabled: false, Builtin: true, State: "normal"}}},
		RuntimeMCPToolsResponse{Tools: []RuntimeMCPTool{{Server: "docs", Name: "search_docs", Enabled: true}}},
		RuntimeMCPResourcesResponse{Resources: []RuntimeMCPResource{{Server: "docs", URI: "docs://intro"}}},
		RuntimeMCPPromptsResponse{Prompts: []RuntimeMCPPrompt{{Server: "docs", Name: "summarize"}}},
	)

	byID := make(map[string]RuntimeCapability)
	for _, capability := range resp.Capabilities {
		byID[capability.ID] = capability
	}
	if byID["builtin:bash"].Enabled {
		t.Fatalf("disabled builtin tool capability = %#v", byID["builtin:bash"])
	}
	if byID["builtin:bash"].State != capabilityStateDisabled {
		t.Fatalf("disabled builtin state = %#v", byID["builtin:bash"])
	}
	if byID["skill:crush-config"].Enabled {
		t.Fatalf("disabled skill capability = %#v", byID["skill:crush-config"])
	}
	if byID["skill:crush-config"].State != capabilityStateDisabled {
		t.Fatalf("disabled skill state = %#v", byID["skill:crush-config"])
	}
	if byID["mcp:docs:search_docs"].Kind != "mcp_tool" {
		t.Fatalf("mcp tool capability missing: %#v", byID["mcp:docs:search_docs"])
	}
	if byID["mcp:docs:search_docs"].State != capabilityStateUnloaded {
		t.Fatalf("mcp tool should stay unloaded until refresh/load: %#v", byID["mcp:docs:search_docs"])
	}
	if byID["mcp_resource:docs:docs://intro"].Kind != "mcp_resource" {
		t.Fatalf("mcp resource capability missing: %#v", byID["mcp_resource:docs:docs://intro"])
	}
	if byID["mcp_resource:docs:docs://intro"].State != capabilityStateUnloaded {
		t.Fatalf("mcp resource should be metadata-only unloaded: %#v", byID["mcp_resource:docs:docs://intro"])
	}
	if byID["mcp_prompt:docs:summarize"].Kind != "mcp_prompt" {
		t.Fatalf("mcp prompt capability missing: %#v", byID["mcp_prompt:docs:summarize"])
	}
	if byID["mcp:docs:search_docs"].CapabilityID == "" || byID["mcp:docs:search_docs"].SchemaDigest == "" || byID["mcp:docs:search_docs"].SchemaSummary == "" {
		t.Fatalf("search metadata missing: %#v", byID["mcp:docs:search_docs"])
	}
	if strings.Contains(fmt.Sprint(byID["mcp:docs:search_docs"]), "Authorization") {
		t.Fatalf("search metadata leaked raw config: %#v", byID["mcp:docs:search_docs"])
	}
}

func TestRuntimeCapabilityStateNormalization(t *testing.T) {
	t.Parallel()

	if got := normalizeCapabilityState("unknown", true); got != capabilityStateUnloaded {
		t.Fatalf("unknown state = %q, want unloaded", got)
	}
	if got := normalizeCapabilityState(capabilityStateLoaded, false); got != capabilityStateDisabled {
		t.Fatalf("disabled state = %q, want disabled", got)
	}

	capability := applyCapabilityLoadRecord(RuntimeCapability{
		ID:      "skill:test",
		Enabled: true,
		State:   capabilityStateUnloaded,
	}, map[string]runtimeCapabilityLoadRecord{
		"skill:test": {State: capabilityStateFailed, Error: "boom", Diagnostics: "failed", Reason: "refresh_failed"},
	})
	if capability.State != capabilityStateFailed || capability.Error != "boom" || capability.Diagnostics != "failed" {
		t.Fatalf("capability load record not applied: %#v", capability)
	}
}

func TestCapabilityRefreshPathIDAllowsEncodedSlashIDs(t *testing.T) {
	t.Parallel()

	id := capabilityRefreshPathID("/v1/capabilities/mcp_resource%3Adocs%3Adocs%3A%2F%2Fintro/refresh")
	if id != "mcp_resource:docs:docs://intro" {
		t.Fatalf("capability refresh path id = %q", id)
	}
}

func TestRuntimeMCPServerStateNormalization(t *testing.T) {
	t.Parallel()

	cases := []struct {
		state mcptools.State
		want  string
	}{
		{mcptools.StateDisabled, mcpServerStateDisabled},
		{mcptools.StateStarting, mcpServerStateLoading},
		{mcptools.StateConnected, mcpServerStateConnected},
		{mcptools.StateError, mcpServerStateFailed},
	}
	for _, tc := range cases {
		got, _ := normalizeMCPServerState(tc.state)
		if got != tc.want {
			t.Fatalf("normalizeMCPServerState(%v) = %q, want %q", tc.state, got, tc.want)
		}
	}
}

func TestRuntimeMCPConfigRedactionCoversSecrets(t *testing.T) {
	t.Parallel()

	store := config.NewTestStore(&config.Config{
		MCP: config.MCPs{
			"docs": {
				Type:    config.MCPHttp,
				URL:     "https://user:password@example.com/mcp?token=secret",
				Command: "node --proxy-authorization secret",
				Args:    []string{"--token=secret"},
				Env:     map[string]string{"OPENAI_API_KEY": "sk-secret", "MODE": "test"},
				Headers: map[string]string{"Authorization": "Bearer secret", "X-Team": "docs"},
			},
		},
	})

	resp := runtimeMCPServersFromConfig(store)
	data, err := json.Marshal(resp)
	if err != nil {
		t.Fatal(err)
	}
	text := string(data)
	for _, leaked := range []string{"password", "token=secret", "sk-secret", "Bearer secret", "proxy-authorization secret", "--token=secret"} {
		if strings.Contains(text, leaked) {
			t.Fatalf("mcp server response leaked %q: %s", leaked, text)
		}
	}
	if !strings.Contains(text, "X-Team") || !strings.Contains(text, "docs") {
		t.Fatalf("non-secret metadata was over-redacted: %s", text)
	}
}

func TestRuntimeCapabilityPolicySummaryBlocksExternalInPlanMode(t *testing.T) {
	t.Parallel()

	service := newRuntimeService()
	service.policy = runtimePolicyFromMode(permission.PolicyModePlan, 0)
	decision := service.evaluateCapabilityLoadPolicy(RuntimeCapability{
		ID:      "mcp:docs:search",
		Kind:    "mcp_tool",
		Name:    "search",
		Enabled: true,
		Risk:    "external",
	})
	if decision.Decision != permission.PolicyDeny || decision.Risk != permission.RiskNetwork {
		t.Fatalf("external capability decision = %#v, want network deny", decision)
	}
}

func TestRuntimeToolSearchFiltersDisabledDeniedAndAudits(t *testing.T) {
	t.Parallel()

	service := newRuntimeService()
	service.policy = runtimePolicyFromMode(permission.PolicyModePlan, 0)
	store := config.NewTestStore(&config.Config{
		Options: &config.Options{DisabledTools: []string{"write"}},
	})
	service.runtime = nil
	service.workspace = nil

	resp := runtimeCapabilities(
		store,
		RuntimeSkillsResponse{Skills: []RuntimeSkill{{Name: "docs", Enabled: true, Builtin: true, State: "normal", Description: "read docs"}}},
		RuntimeMCPToolsResponse{Tools: []RuntimeMCPTool{{Server: "docs", Name: "search_docs", Description: "search docs", Enabled: true}}},
		RuntimeMCPResourcesResponse{},
		RuntimeMCPPromptsResponse{},
	)
	results, omitted := service.filterAndScoreToolSearch("docs", resp.Capabilities, 10)
	if !slices.ContainsFunc(results, func(result RuntimeToolSearchResult) bool {
		return result.Name == "docs"
	}) {
		t.Fatalf("skill search result missing: %#v", results)
	}
	if slices.ContainsFunc(results, func(result RuntimeToolSearchResult) bool {
		return result.Name == "search_docs"
	}) {
		t.Fatalf("plan mode exposed policy-denied mcp tool: %#v", results)
	}
	if !slices.ContainsFunc(omitted, func(item RuntimeToolSearchOmission) bool {
		return item.Name == "search_docs" && item.Reason == "policy_denied"
	}) {
		t.Fatalf("policy-denied omission missing: %#v", omitted)
	}
	if !slices.ContainsFunc(omitted, func(item RuntimeToolSearchOmission) bool {
		return item.Name == "write" && item.Reason == "disabled_tool"
	}) {
		t.Fatalf("disabled omission missing: %#v", omitted)
	}
}

func TestRuntimeToolSearchRepeatGuardrailEmitsEvent(t *testing.T) {
	t.Parallel()

	service := newRuntimeService()
	for i := 0; i < maxConsecutiveSameToolSearches; i++ {
		blocked, reason := service.preventRepeatedToolSearch("turn-1", "docs")
		if blocked {
			t.Fatalf("search %d blocked early: %s", i, reason)
		}
	}
	blocked, reason := service.preventRepeatedToolSearch("turn-1", "docs")
	if !blocked || reason != "repeat_search" {
		t.Fatalf("repeat guard = %v %q", blocked, reason)
	}
	service.recordDeadlockPrevented("session-1", "turn-1", "tool-1", reason, "repeat")
	events, err := service.Events(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if !slices.ContainsFunc(events.Events, func(event RuntimeEvent) bool {
		return event.Type == runtimeapi.EventSchedulerDeadlockPrevented && event.Payload["reason"] == reason
	}) {
		t.Fatalf("deadlock event missing: %#v", events.Events)
	}
}

func TestRuntimeToolSearchMaxSearchesGuardrailEmitsReplayFact(t *testing.T) {
	t.Parallel()

	h := newRuntimeScenarioHarness(t)
	h.seedTurn("session-search-max", "turn-search-max")
	for i := 0; i < maxToolSearchesPerTurn; i++ {
		blocked, reason := h.service.preventRepeatedToolSearch("turn-search-max", "docs"+string(rune('a'+i)))
		if blocked {
			t.Fatalf("search %d blocked early: %s", i, reason)
		}
	}
	blocked, reason := h.service.preventRepeatedToolSearch("turn-search-max", "overflow")
	if !blocked || reason != "max_searches_per_turn" {
		t.Fatalf("max search guard = %v %q", blocked, reason)
	}
	h.service.recordDeadlockPrevented("session-search-max", "turn-search-max", "tool-search", reason, "max searches")
	replay := h.replay("turn-search-max")
	if !slices.Contains(replay.Summary.ToolDiscovery.GuardrailReasons, "max_searches_per_turn") {
		t.Fatalf("guardrail replay missing: %#v", replay.Summary.ToolDiscovery)
	}
}

func TestRuntimeToolSearchGuardrailRecordsSearchAndDeadlockReplay(t *testing.T) {
	t.Parallel()

	h := newRuntimeScenarioHarness(t)
	h.seedTurn("session-repeat", "turn-repeat")
	for i := 0; i < maxConsecutiveSameToolSearches; i++ {
		blocked, reason := h.service.preventRepeatedToolSearch("turn-repeat", "docs")
		if blocked {
			t.Fatalf("search %d blocked early: %s", i, reason)
		}
	}
	resp, err := h.service.searchTools(h.ctx, RuntimeToolSearchRequest{
		Query:      "docs",
		SessionID:  "session-repeat",
		TurnID:     "turn-repeat",
		ToolCallID: "tool-search",
	})
	if err != nil {
		t.Fatal(err)
	}
	if resp.Guardrail != "repeat_search" {
		t.Fatalf("guardrail response = %#v", resp)
	}
	replay := h.replay("turn-repeat")
	if !slices.ContainsFunc(replay.Summary.ToolSearches, func(item RuntimeReplayToolSearch) bool {
		return item.Query == "docs" && item.Guardrail == "repeat_search"
	}) {
		t.Fatalf("tool search guardrail replay missing: %#v", replay.Summary.ToolSearches)
	}
	if replay.Summary.EventCounts[runtimeapi.EventSchedulerDeadlockPrevented] == 0 {
		t.Fatalf("deadlock event count missing: %#v", replay.Summary.EventCounts)
	}
}

func TestRuntimeToolSearchRecursionGuardAndUnavailableOmissions(t *testing.T) {
	t.Parallel()

	service := newRuntimeService()
	caps := []RuntimeCapability{
		{ID: "builtin:tool_search", Kind: "builtin_tool", Name: agent.ToolSearchToolName, Enabled: true, Risk: "read", Description: "Search tools.", State: capabilityStateLoaded},
		{ID: "builtin:failed", Kind: "builtin_tool", Name: "failed", Enabled: true, Risk: "read", Description: "failed tool.", State: capabilityStateFailed, Reason: "load_failed"},
		{ID: "builtin:unavailable", Kind: "builtin_tool", Name: "unavailable", Enabled: true, Risk: "read", Description: "unavailable tool.", State: capabilityStateUnavailable, Reason: "server_missing"},
	}
	results, omitted := service.filterAndScoreToolSearch("select:tool_search,failed,unavailable", caps, 10)
	if len(results) != 0 {
		t.Fatalf("guarded capabilities leaked into search: %#v", results)
	}
	for id, reason := range map[string]string{"builtin:tool_search": "recursion_guard", "builtin:failed": "load_failed", "builtin:unavailable": "server_missing"} {
		if !slices.ContainsFunc(omitted, func(item RuntimeToolSearchOmission) bool {
			return item.ID == id && item.Reason == reason
		}) {
			t.Fatalf("missing omission %s=%s in %#v", id, reason, omitted)
		}
	}
}

func TestRuntimeSchedulerRunningToolGuardCleanup(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		name     string
		finish   func(context.Context, runtimeSchedulerRecorder, agent.SchedulerToolCallResult) error
		expected scheduler.ToolCallStatus
	}{
		{
			name: "success",
			finish: func(ctx context.Context, recorder runtimeSchedulerRecorder, result agent.SchedulerToolCallResult) error {
				return recorder.ToolCallCompleted(ctx, result)
			},
			expected: scheduler.ToolCallCompleted,
		},
		{
			name: "failure",
			finish: func(ctx context.Context, recorder runtimeSchedulerRecorder, result agent.SchedulerToolCallResult) error {
				result.IsError = true
				result.Error = "boom"
				return recorder.ToolCallFailed(ctx, result)
			},
			expected: scheduler.ToolCallFailed,
		},
		{
			name: "cancel",
			finish: func(ctx context.Context, recorder runtimeSchedulerRecorder, result agent.SchedulerToolCallResult) error {
				result.Cancelled = true
				return recorder.ToolCallCancelled(ctx, result)
			},
			expected: scheduler.ToolCallCancelled,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			service := newRuntimeService()
			service.policy = runtimePolicyFromMode(permission.PolicyModeAutoRead, 0)
			recorder := runtimeSchedulerRecorder{service: service}
			call := agent.SchedulerToolCall{
				ID:           "tool-" + tc.name,
				SessionID:    "session-" + tc.name,
				TurnID:       "turn-" + tc.name,
				Name:         "view",
				Source:       "builtin",
				CapabilityID: "builtin:view",
				InputSummary: `{"file_path":"README.md"}`,
			}
			decision, err := recorder.EvaluateToolCall(context.Background(), call)
			if err != nil {
				t.Fatal(err)
			}
			if decision.Decision != string(permission.PolicyAllow) {
				t.Fatalf("decision = %#v", decision)
			}
			if service.toolDiscovery.RunningByTurn[call.TurnID] != 1 {
				t.Fatalf("running count after allow = %d", service.toolDiscovery.RunningByTurn[call.TurnID])
			}
			if err := recorder.ToolCallStarted(context.Background(), call); err != nil {
				t.Fatal(err)
			}
			if err := tc.finish(context.Background(), recorder, agent.SchedulerToolCallResult{
				ToolCallID:          call.ID,
				SessionID:           call.SessionID,
				TurnID:              call.TurnID,
				Name:                call.Name,
				Source:              call.Source,
				ModelVisibleContent: "done",
			}); err != nil {
				t.Fatal(err)
			}
			if service.toolDiscovery.RunningByTurn[call.TurnID] != 0 {
				t.Fatalf("running count leaked = %d", service.toolDiscovery.RunningByTurn[call.TurnID])
			}
			stored, err := service.toolCalls.GetCall(context.Background(), call.ID)
			if err != nil {
				t.Fatal(err)
			}
			if stored.Status != tc.expected {
				t.Fatalf("stored status = %s, want %s", stored.Status, tc.expected)
			}
		})
	}
}

func TestRecordToolCallsFromMessageWaitsForToolResult(t *testing.T) {
	t.Parallel()

	service := newRuntimeService()
	service.toolCalls = scheduler.New(scheduler.NewMemoryStore())

	assistant := proto.Message{
		ID:        "assistant-1",
		SessionID: "session-1",
		Role:      proto.Assistant,
		Parts: []proto.ContentPart{
			proto.ToolCall{
				ID:       "tool-1",
				Name:     "write",
				Input:    `{"file_path":"report.md","content":"ok"}`,
				Finished: true,
			},
		},
	}
	service.recordToolCallsFromMessage(context.Background(), assistant, "turn-1", time.Now())

	call, err := service.toolCalls.GetCall(context.Background(), "tool-1")
	if err != nil {
		t.Fatal(err)
	}
	if call.Status != scheduler.ToolCallRunning || !call.FinishedAt.IsZero() {
		t.Fatalf("tool call after input finish = %#v, want running without finished time", call)
	}

	result := proto.Message{
		ID:        "tool-result-1",
		SessionID: "session-1",
		Role:      proto.Tool,
		Parts: []proto.ContentPart{
			proto.ToolResult{
				ToolCallID: "tool-1",
				Name:       "write",
				Content:    "File successfully written: C:/Users/ytq/Desktop/report.md",
			},
		},
	}
	service.recordToolCallsFromMessage(context.Background(), result, "turn-1", time.Now())

	call, err = service.toolCalls.GetCall(context.Background(), "tool-1")
	if err != nil {
		t.Fatal(err)
	}
	if call.Status != scheduler.ToolCallCompleted || call.OutputSummary == "" || call.FinishedAt.IsZero() {
		t.Fatalf("tool call after result = %#v, want completed with output and finished time", call)
	}
}

func TestRuntimeCapabilitiesAreMetadataOnlyForSkillsResourcesAndPrompts(t *testing.T) {
	t.Parallel()

	store := config.NewTestStore(&config.Config{})
	resp := runtimeCapabilities(
		store,
		RuntimeSkillsResponse{Skills: []RuntimeSkill{{Name: "docs", Enabled: true, Builtin: true, State: "normal", Description: "heavy skill"}}},
		RuntimeMCPToolsResponse{},
		RuntimeMCPResourcesResponse{Resources: []RuntimeMCPResource{{Server: "docs", URI: "docs://intro", Description: "resource"}}},
		RuntimeMCPPromptsResponse{Prompts: []RuntimeMCPPrompt{{Server: "docs", Name: "summarize", Description: "prompt"}}},
	)

	byID := make(map[string]RuntimeCapability)
	for _, capability := range resp.Capabilities {
		byID[capability.ID] = capability
	}
	for _, id := range []string{"skill:docs", "mcp_resource:docs:docs://intro", "mcp_prompt:docs:summarize"} {
		if byID[id].State != capabilityStateUnloaded {
			t.Fatalf("%s state = %#v, want unloaded metadata", id, byID[id])
		}
		if byID[id].Diagnostics != "" || byID[id].Error != "" {
			t.Fatalf("%s should not include heavy diagnostics on clean metadata: %#v", id, byID[id])
		}
	}
}

func TestRuntimeCapabilityRefreshRecordsFailedDiagnosticAndEvent(t *testing.T) {
	t.Parallel()

	service := newRuntimeService()
	capability := RuntimeCapability{
		ID:      "mcp:docs:search",
		Kind:    "mcp_tool",
		Name:    "search",
		Source:  "docs",
		Enabled: true,
		State:   capabilityStateFailed,
		Reason:  "refresh_failed",
		Error:   "Authorization: Bearer secret",
	}

	service.recordCapabilityLoad(capability, capabilityStateFailed, capability.Reason, capability.Error, 12)
	events, err := service.Events(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	var found bool
	for _, event := range events.Events {
		if event.Type == runtimeapi.EventCapabilityFailed && event.Payload["capability_id"] == "mcp:docs:search" {
			found = true
			if strings.Contains(fmt.Sprint(event.Payload["error"]), "secret") {
				t.Fatalf("capability event leaked secret: %#v", event.Payload)
			}
			break
		}
	}
	if !found {
		t.Fatalf("capability failed event missing: %#v", events.Events)
	}
}

func TestRuntimeMCPRefreshDeniedRecordsEventsAndCapabilities(t *testing.T) {
	t.Parallel()

	service := newRuntimeService()
	service.policy = runtimePolicyFromMode(permission.PolicyModeDenyAll, 0)
	store := config.NewTestStore(&config.Config{
		MCP: config.MCPs{
			"docs": {Type: config.MCPHttp, URL: "https://example.com/mcp"},
		},
	})

	if err := service.refreshMCPServerLifecycle(context.Background(), store, "workspace", "docs", "test"); err != nil {
		t.Fatal(err)
	}

	events, err := service.Events(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	var serverFailed bool
	var capabilityFailed bool
	for _, event := range events.Events {
		switch event.Type {
		case runtimeapi.EventMCPServerFailed:
			serverFailed = event.Payload["name"] == "docs" && event.Payload["state"] == mcpServerStateFailed
		case runtimeapi.EventCapabilityFailed:
			capabilityFailed = strings.Contains(fmt.Sprint(event.Payload["capability_id"]), "mcp")
		}
	}
	if !serverFailed {
		t.Fatalf("mcp.server.failed event missing: %#v", events.Events)
	}
	if !capabilityFailed {
		t.Fatalf("capability.failed event missing: %#v", events.Events)
	}
}

func TestRuntimeMCPResourceAndPromptInventoryFiltersPolicyDenied(t *testing.T) {
	t.Parallel()

	service := newRuntimeService()
	service.policy = runtimePolicyFromParts(permission.PolicyModeAutoRead, "test", []RuntimePolicyRule{
		{ID: "deny-resource", Decision: string(permission.PolicyDeny), MCPResource: "docs://secret", Reason: "resource denied"},
		{ID: "deny-prompt", Decision: string(permission.PolicyDeny), MCPPrompt: "summarize", Reason: "prompt denied"},
	}, 0)

	resources := service.filterMCPResourcesByPolicy(RuntimeMCPResourcesResponse{Resources: []RuntimeMCPResource{
		{Server: "docs", URI: "docs://intro"},
		{Server: "docs", URI: "docs://secret"},
	}})
	if len(resources.Resources) != 1 || resources.Resources[0].URI != "docs://intro" {
		t.Fatalf("resource policy filter = %#v", resources.Resources)
	}
	prompts := service.filterMCPPromptsByPolicy(RuntimeMCPPromptsResponse{Prompts: []RuntimeMCPPrompt{
		{Server: "docs", Name: "summarize"},
		{Server: "docs", Name: "draft"},
	}})
	if len(prompts.Prompts) != 1 || prompts.Prompts[0].Name != "draft" {
		t.Fatalf("prompt policy filter = %#v", prompts.Prompts)
	}
	events, err := service.Events(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if !slices.ContainsFunc(events.Events, func(event RuntimeEvent) bool {
		return event.Type == runtimeapi.EventMCPCapabilityDenied && event.Payload["name"] == "docs://secret"
	}) {
		t.Fatalf("mcp capability denied event missing: %#v", events.Events)
	}
}

func TestRuntimeMCPCapabilityAllowedAppliesAgentTaskScope(t *testing.T) {
	t.Parallel()

	dataDir := t.TempDir()
	conn, err := db.Connect(context.Background(), dataDir)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_ = db.Release(dataDir)
	})
	service := newRuntimeService()
	service.agentTasks = newRuntimeAgentTaskStore(conn)
	if _, err := service.agentTasks.Upsert(context.Background(), RuntimeAgentTask{
		ID:              "task-1",
		ParentSessionID: "session-parent",
		ParentTurnID:    "turn-parent",
		ChildSessionID:  "session-child",
		Status:          agentTaskStatusRunning,
		AllowedTools:    []string{"safe_tool"},
		CapabilityScope: []string{"mcp:docs:safe_tool"},
		StartedAt:       time.Now().UnixMilli(),
		UpdatedAt:       time.Now().UnixMilli(),
	}); err != nil {
		t.Fatal(err)
	}
	recorder := runtimeSchedulerRecorder{service: service}

	allowed := recorder.CapabilityAllowed(context.Background(), agent.SchedulerToolMetadata{
		SessionID:    "session-child",
		TurnID:       "turn-child",
		Name:         "safe_tool",
		Source:       "mcp",
		CapabilityID: "mcp:docs:safe_tool",
		Description:  "safe",
	})
	if !allowed {
		t.Fatal("expected scoped mcp tool to be exposed")
	}
	denied := recorder.CapabilityAllowed(context.Background(), agent.SchedulerToolMetadata{
		SessionID:    "session-child",
		TurnID:       "turn-child",
		Name:         "secret_tool",
		Source:       "mcp",
		CapabilityID: "mcp:docs:secret_tool",
		Description:  "secret",
	})
	if denied {
		t.Fatal("expected agent task scope to hide denied mcp tool")
	}
	events, err := service.Events(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if !slices.ContainsFunc(events.Events, func(event RuntimeEvent) bool {
		return event.Type == runtimeapi.EventMCPCapabilityDenied && event.Payload["scope"] == "agent_task"
	}) {
		t.Fatalf("agent task scoped mcp denial event missing: %#v", events.Events)
	}
}

func TestRuntimeMCPLazyLifecycleEventsAreRedacted(t *testing.T) {
	t.Parallel()

	service := newRuntimeService()
	service.publishMCPServerEvent(runtimeapi.EventMCPServerLazyStarted, "docs", mcpServerStateLoading, "", "capability_refresh")
	service.publishMCPServerEvent(runtimeapi.EventMCPServerLazyFailed, "docs", mcpServerStateFailed, "Authorization: Bearer secret", "connect_failed")

	events, err := service.Events(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	var started, failed bool
	for _, event := range events.Events {
		switch event.Type {
		case runtimeapi.EventMCPServerLazyStarted:
			started = true
		case runtimeapi.EventMCPServerLazyFailed:
			failed = true
			if strings.Contains(fmt.Sprint(event.Payload["error"]), "secret") {
				t.Fatalf("lazy failure leaked secret: %#v", event.Payload)
			}
		}
	}
	if !started || !failed {
		t.Fatalf("lazy lifecycle events missing: %#v", events.Events)
	}
}

func TestRuntimeSkillsFromConfigIncludesInvalidSkillDiagnostics(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	skillDir := filepath.Join(root, "bad-skill")
	if err := os.MkdirAll(skillDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(skillDir, "SKILL.md"), []byte("missing frontmatter"), 0o644); err != nil {
		t.Fatal(err)
	}
	store := config.NewTestStore(&config.Config{
		Options: &config.Options{
			SkillsPaths: []string{root},
		},
	})

	resp := runtimeSkillsFromConfig(store)
	for _, skill := range resp.Skills {
		if skill.State == capabilityStateFailed && strings.Contains(skill.Path, "SKILL.md") && skill.Error != "" && skill.Diagnostics != "" {
			return
		}
	}
	t.Fatalf("invalid skill diagnostic missing from %#v", resp.Skills)
}

func TestRuntimeSkillsFromConfigNormalizesActivationMetadata(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	skillDir := filepath.Join(root, "metadata-skill")
	if err := os.MkdirAll(skillDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(skillDir, "SKILL.md"), []byte(`---
name: metadata-skill
description: Runtime activation metadata skill.
allowed_tools:
  - view
  - bash
---

Use this skill from the desktop runtime.
`), 0o644); err != nil {
		t.Fatal(err)
	}
	store := config.NewTestStore(&config.Config{
		Options: &config.Options{
			SkillsPaths: []string{root},
		},
	})

	resp := runtimeSkillsFromConfig(store)
	for _, skill := range resp.Skills {
		if skill.Name != "metadata-skill" {
			continue
		}
		if skill.State != capabilityStateUnloaded || !skill.Enabled || skill.CapabilityID != "skill:metadata-skill" {
			t.Fatalf("skill state metadata = %#v", skill)
		}
		if got := strings.Join(skill.AllowedTools, ","); got != "bash,view" {
			t.Fatalf("allowed tools = %q", got)
		}
		if !skill.Activation.Included || skill.Activation.Reason == "" {
			t.Fatalf("activation metadata missing: %#v", skill.Activation)
		}
		if !strings.Contains(skill.PolicyReason, "does not expand runtime permissions") {
			t.Fatalf("policy hook reason missing: %#v", skill)
		}
		return
	}
	t.Fatalf("metadata skill missing from %#v", resp.Skills)
}

func TestRuntimeSkillsFromConfigDisabledSkillExcludedFromActivation(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	skillDir := filepath.Join(root, "disabled-skill")
	if err := os.MkdirAll(skillDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(skillDir, "SKILL.md"), []byte(`---
name: disabled-skill
description: Disabled skill.
---

Should not be activated.
`), 0o644); err != nil {
		t.Fatal(err)
	}
	store := config.NewTestStore(&config.Config{
		Options: &config.Options{
			SkillsPaths:    []string{root},
			DisabledSkills: []string{"disabled-skill"},
		},
	})

	resp := runtimeSkillsFromConfig(store)
	for _, skill := range resp.Skills {
		if skill.Name != "disabled-skill" {
			continue
		}
		if skill.Enabled || skill.State != capabilityStateDisabled || skill.Activation.Included {
			t.Fatalf("disabled skill should be excluded: %#v", skill)
		}
		summary := runtimeTurnSkillSummary(resp.Skills, "ask")
		for _, item := range summary.Excluded {
			if item.Name == "disabled-skill" && item.Reason == "excluded by disabled config" {
				return
			}
		}
		t.Fatalf("disabled skill missing from excluded summary: %#v", summary)
	}
	t.Fatalf("disabled skill missing from %#v", resp.Skills)
}

func TestRuntimeSkillsWithDenyAllPolicyExcludedFromActivation(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	skillDir := filepath.Join(root, "policy-skill")
	if err := os.MkdirAll(skillDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(skillDir, "SKILL.md"), []byte(`---
name: policy-skill
description: Policy gated skill.
---

Should not be activated when deny_all is active.
`), 0o644); err != nil {
		t.Fatal(err)
	}
	store := config.NewTestStore(&config.Config{
		Options: &config.Options{
			SkillsPaths: []string{root},
		},
	})

	resp := runtimeSkillsFromConfigWithPolicy(store, permission.PolicyModeDenyAll)
	for _, skill := range resp.Skills {
		if skill.Name != "policy-skill" {
			continue
		}
		if skill.Enabled || skill.State != capabilityStateDisabled || skill.Reason != "policy_denied" || skill.Activation.Included {
			t.Fatalf("deny_all skill should be policy excluded: %#v", skill)
		}
		if skill.PolicyMode != string(permission.PolicyModeDenyAll) || skill.PolicyRisk != string(permission.RiskRead) {
			t.Fatalf("policy metadata missing: %#v", skill)
		}
		return
	}
	t.Fatalf("policy skill missing from %#v", resp.Skills)
}

func TestRuntimeSkillAllowedToolsDoesNotGrantPermission(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	skillDir := filepath.Join(root, "writer-skill")
	if err := os.MkdirAll(skillDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(skillDir, "SKILL.md"), []byte(`---
name: writer-skill
description: Writer skill.
allowed_tools:
  - write
---

Use write when policy allows it.
`), 0o644); err != nil {
		t.Fatal(err)
	}
	store := config.NewTestStore(&config.Config{
		Options: &config.Options{SkillsPaths: []string{root}},
	})

	resp := runtimeSkillsFromConfigWithPolicy(store, permission.PolicyModeAutoRead)
	var found RuntimeSkill
	for _, skill := range resp.Skills {
		if skill.Name == "writer-skill" {
			found = skill
			break
		}
	}
	if found.Name == "" || !found.Enabled || !slices.Contains(found.AllowedTools, "write") {
		t.Fatalf("skill metadata missing: %#v", found)
	}
	policy := permission.NewScopedPermissionPolicy(permission.PolicyModeAutoRead, "test", nil)
	decision := policy.Evaluate(scheduler.ToolCall{
		Name:         "write",
		Source:       scheduler.ToolSourceBuiltin,
		CapabilityID: "builtin:write",
		InputSummary: `{"file_path":"out.txt"}`,
	})
	if decision.Decision == permission.PolicyAllow {
		t.Fatalf("skill allowed_tools must not grant write permission: %#v", decision)
	}
}

func TestRuntimeToolSearchFiltersPolicyDeniedSkillsResourcesAndPrompts(t *testing.T) {
	t.Parallel()

	service := newRuntimeService()
	service.policy = runtimePolicyFromParts(permission.PolicyModeAutoRead, "test", []RuntimePolicyRule{
		{ID: "deny-skill", Decision: string(permission.PolicyDeny), Skill: "deploy", Reason: "skill denied"},
		{ID: "deny-resource", Decision: string(permission.PolicyDeny), MCPResource: "docs://secret", Reason: "resource denied"},
		{ID: "deny-prompt", Decision: string(permission.PolicyDeny), MCPPrompt: "summarize", Reason: "prompt denied"},
	}, 0)
	caps := []RuntimeCapability{
		{ID: "skill:deploy", Kind: "skill", Name: "deploy", Enabled: true, Risk: "context", Description: "deploy docs", State: capabilityStateUnloaded},
		{ID: "mcp_resource:docs:docs://secret", Kind: "mcp_resource", Name: "docs://secret", Source: "docs", Enabled: true, Risk: "read", Description: "secret docs", State: capabilityStateUnloaded},
		{ID: "mcp_prompt:docs:summarize", Kind: "mcp_prompt", Name: "summarize", Source: "docs", Enabled: true, Risk: "context", Description: "summary prompt", State: capabilityStateUnloaded},
	}

	results, omitted := service.filterAndScoreToolSearch("select:deploy,docs://secret,summarize", caps, 10)
	if len(results) != 0 {
		t.Fatalf("policy denied metadata capabilities leaked into search: %#v", results)
	}
	for _, id := range []string{"skill:deploy", "mcp_resource:docs:docs://secret", "mcp_prompt:docs:summarize"} {
		if !slices.ContainsFunc(omitted, func(item RuntimeToolSearchOmission) bool {
			return item.ID == id && item.Reason == "policy_denied"
		}) {
			t.Fatalf("missing policy denied omission for %s: %#v", id, omitted)
		}
	}
}

func TestRuntimeSkillsFromConfigIncludesDesktopManagedPath(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	skillDir := filepath.Join(root, "managed-skill")
	if err := os.MkdirAll(skillDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(skillDir, "SKILL.md"), []byte(`---
name: managed-skill
description: Runtime managed skill.
---

Use this skill from the desktop runtime.
`), 0o644); err != nil {
		t.Fatal(err)
	}
	store := config.NewTestStore(&config.Config{
		Options: &config.Options{},
	})

	resp := runtimeSkillsFromConfig(store, root)
	for _, skill := range resp.Skills {
		if skill.Name == "managed-skill" && skill.Enabled && strings.HasPrefix(skill.SkillFilePath, root) {
			if len(store.Config().Options.SkillsPaths) != 0 {
				t.Fatalf("desktop path was persisted to config: %#v", store.Config().Options.SkillsPaths)
			}
			return
		}
	}
	t.Fatalf("desktop managed skill missing from %#v", resp.Skills)
}

func TestDesktopSkillConfigIsSeparateFromCrushConfig(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	layout := desktopLayout{
		Root:            root,
		ConfigDir:       filepath.Join(root, "config"),
		DataDir:         filepath.Join(root, "data"),
		LogsDir:         filepath.Join(root, "logs"),
		ModelConfigPath: filepath.Join(root, "config", "model.json"),
		SkillConfigPath: filepath.Join(root, "config", "skills.json"),
	}
	store := config.NewTestStore(&config.Config{
		Options: &config.Options{
			SkillsPaths:    []string{"project-skills"},
			DisabledSkills: []string{"project-disabled"},
		},
	})
	if err := saveDesktopSkillConfig(layout, desktopSkillConfig{
		SkillPaths:     []string{filepath.Join(root, "user-skills")},
		DisabledSkills: []string{"desktop-disabled"},
	}); err != nil {
		t.Fatal(err)
	}

	if err := applyDesktopSkillConfigToStore(store, layout); err != nil {
		t.Fatal(err)
	}
	if !slices.Contains(store.Config().Options.SkillsPaths, "project-skills") {
		t.Fatalf("project skill path removed: %#v", store.Config().Options.SkillsPaths)
	}
	if !slices.Contains(store.Config().Options.SkillsPaths, filepath.Join(root, "user-skills")) {
		t.Fatalf("desktop skill path missing: %#v", store.Config().Options.SkillsPaths)
	}
	if !slices.Contains(store.Config().Options.SkillsPaths, desktopSkillsDir(layout)) {
		t.Fatalf("managed skill path missing: %#v", store.Config().Options.SkillsPaths)
	}
	if !slices.Contains(store.Config().Options.DisabledSkills, "project-disabled") ||
		!slices.Contains(store.Config().Options.DisabledSkills, "desktop-disabled") {
		t.Fatalf("disabled skills not merged in memory: %#v", store.Config().Options.DisabledSkills)
	}
}

func TestRefreshSkillsPublishesDiscoveryEvents(t *testing.T) {
	t.Parallel()

	service := newRuntimeService()
	runtime, workspace := backendForSkillTest(t)
	service.runtime = runtime
	service.workspace = &proto.Workspace{ID: workspace.ID}
	service.sessionID = "session-1"

	events, unsubscribe := service.SubscribeEvents(context.Background())
	defer unsubscribe()

	if _, err := service.RefreshSkills(context.Background()); err != nil {
		t.Fatal(err)
	}

	seenStarted := false
	seenCompleted := false
	for i := 0; i < 2; i++ {
		select {
		case event := <-events:
			if event.Type == "skill.discovery.started" {
				seenStarted = true
			}
			if event.Type == "skill.discovery.completed" {
				seenCompleted = true
			}
		case <-time.After(time.Second):
			t.Fatal("timed out waiting for skill discovery events")
		}
	}
	if !seenStarted || !seenCompleted {
		t.Fatalf("skill discovery events missing: started=%v completed=%v", seenStarted, seenCompleted)
	}
}

func TestRuntimeNewChatUsesDraftSessionAndPreservesRecents(t *testing.T) {
	t.Parallel()

	service := newRuntimeService()
	runtime, workspace := backendForSkillTest(t)
	first, err := runtime.CreateSession(context.Background(), workspace.ID, "First chat")
	if err != nil {
		t.Fatal(err)
	}
	service.runtime = runtime
	service.workspace = &proto.Workspace{ID: workspace.ID}
	service.sessionID = first.ID

	status, err := service.NewChat(context.Background(), "Second chat")
	if err != nil {
		t.Fatal(err)
	}
	if status.SessionID != "" {
		t.Fatalf("new chat should enter draft mode without selecting a session: %#v", status)
	}

	sessions, err := service.Sessions(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(sessions.Sessions) != 1 {
		t.Fatalf("sessions = %#v, want only previous persisted session", sessions.Sessions)
	}
	for _, session := range sessions.Sessions {
		if session.Active {
			t.Fatalf("draft mode should not mark persisted sessions active: %#v", sessions.Sessions)
		}
	}

	if _, err := service.SelectSession(context.Background(), first.ID); err != nil {
		t.Fatal(err)
	}
	selected, err := service.Sessions(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	for _, session := range selected.Sessions {
		if session.ID == first.ID && !session.Active {
			t.Fatalf("selected session is not active: %#v", selected.Sessions)
		}
	}
}

func TestRuntimeCreateSessionPersistsAndSelectsSession(t *testing.T) {
	t.Parallel()

	service := newRuntimeService()
	runtimeBackend, workspace := backendForSkillTest(t)
	service.runtime = runtimeBackend
	service.workspace = &proto.Workspace{ID: workspace.ID}

	created, err := service.CreateSession(context.Background(), RuntimeSessionCreateRequest{Title: "Pinned chat"})
	if err != nil {
		t.Fatal(err)
	}
	if created.Session.ID == "" || created.Session.Title != "Pinned chat" || !created.Session.Active {
		t.Fatalf("created session = %#v", created.Session)
	}
	status, err := service.Status(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if status.SessionID != created.Session.ID {
		t.Fatalf("status session = %q, want %q", status.SessionID, created.Session.ID)
	}
	sessions, err := service.Sessions(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(sessions.Sessions) != 1 || sessions.Sessions[0].ID != created.Session.ID || !sessions.Sessions[0].Active {
		t.Fatalf("sessions = %#v", sessions.Sessions)
	}
}

func TestRuntimeCreateSessionPersistsOwnership(t *testing.T) {
	t.Parallel()

	service := newRuntimeService()
	runtimeBackend, workspace := backendForSkillTest(t)
	service.runtime = runtimeBackend
	service.workspace = &proto.Workspace{ID: workspace.ID}

	projectSession, err := service.CreateSession(context.Background(), RuntimeSessionCreateRequest{
		Title:     "Project chat",
		ProjectID: workspace.ID,
		Scope:     "project",
	})
	if err != nil {
		t.Fatal(err)
	}
	if projectSession.Session.Scope != "project" || projectSession.Session.ProjectID != workspace.ID {
		t.Fatalf("project session ownership = %#v", projectSession.Session)
	}

	standaloneSession, err := service.CreateSession(context.Background(), RuntimeSessionCreateRequest{
		Title: "Standalone chat",
		Scope: "standalone",
	})
	if err != nil {
		t.Fatal(err)
	}
	if standaloneSession.Session.Scope != "standalone" || standaloneSession.Session.ProjectID != "" {
		t.Fatalf("standalone session ownership = %#v", standaloneSession.Session)
	}

	sessions, err := service.Sessions(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	byID := map[string]RuntimeSession{}
	for _, sess := range sessions.Sessions {
		byID[sess.ID] = sess
	}
	if byID[projectSession.Session.ID].Scope != "project" || byID[projectSession.Session.ID].ProjectID != workspace.ID {
		t.Fatalf("listed project session ownership = %#v", byID[projectSession.Session.ID])
	}
	if byID[standaloneSession.Session.ID].Scope != "standalone" || byID[standaloneSession.Session.ID].ProjectID != "" {
		t.Fatalf("listed standalone session ownership = %#v", byID[standaloneSession.Session.ID])
	}
}

func TestRuntimeDeleteActiveSessionReturnsToDraft(t *testing.T) {
	t.Parallel()

	service := newRuntimeService()
	runtime, workspace := backendForSkillTest(t)
	first, err := runtime.CreateSession(context.Background(), workspace.ID, "First chat")
	if err != nil {
		t.Fatal(err)
	}
	second, err := runtime.CreateSession(context.Background(), workspace.ID, "Second chat")
	if err != nil {
		t.Fatal(err)
	}
	service.runtime = runtime
	service.workspace = &proto.Workspace{ID: workspace.ID}
	service.sessionID = first.ID

	resp, err := service.DeleteSession(context.Background(), first.ID)
	if err != nil {
		t.Fatal(err)
	}
	if service.sessionID != "" {
		t.Fatalf("active session after delete = %q, want draft", service.sessionID)
	}
	if len(resp.Sessions) != 1 || resp.Sessions[0].ID != second.ID || resp.Sessions[0].Active {
		t.Fatalf("sessions after delete = %#v", resp.Sessions)
	}
	status, err := service.Status(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if status.SessionID != "" || status.Usage.TotalTokens != 0 {
		t.Fatalf("status after delete = %#v", status)
	}
}

func TestRuntimeChatRenamesDefaultSessionTitle(t *testing.T) {
	t.Parallel()

	service := newRuntimeService()
	runtime, workspace := backendForSkillTest(t)
	sess, err := runtime.CreateSession(context.Background(), workspace.ID, "Untitled Session")
	if err != nil {
		t.Fatal(err)
	}
	service.runtime = runtime
	service.workspace = &proto.Workspace{ID: workspace.ID}
	service.sessionID = sess.ID

	if err := service.ensureSessionTitle(context.Background(), workspace.ID, sess.ID, "Summarize runtime session behavior in one line."); err != nil {
		t.Fatal(err)
	}
	renamed, err := runtime.GetSession(context.Background(), workspace.ID, sess.ID)
	if err != nil {
		t.Fatal(err)
	}
	if renamed.Title == "Untitled Session" || renamed.Title == "" {
		t.Fatalf("session was not renamed: %#v", renamed)
	}
	if renamed.Title != "Summarize runtime session behavior in one line." {
		t.Fatalf("title = %q", renamed.Title)
	}
}

func backendForSkillTest(t *testing.T) (*backend.Backend, proto.Workspace) {
	t.Helper()
	workingDir := t.TempDir()
	dataRoot, err := os.MkdirTemp("", "agent-builder-runtime-state-*")
	if err != nil {
		t.Fatal(err)
	}
	dataDir := filepath.Join(dataRoot, "runtime-state")
	if err := os.MkdirAll(dataDir, 0o700); err != nil {
		t.Fatal(err)
	}
	cfg := config.NewRuntimeConfig(workingDir, dataDir, false)
	cfg.Options.AutoLSP = ptr(false)
	store := config.NewRuntimeStore(workingDir, cfg)
	runtime := backend.New(context.Background(), store, nil)
	_, workspace, err := runtime.CreateWorkspace(proto.Workspace{
		Path:    workingDir,
		DataDir: dataDir,
		Config:  store.Config(),
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		runtime.DeleteWorkspace(workspace.ID)
		for i := 0; i < 20; i++ {
			if err := os.RemoveAll(dataRoot); err == nil {
				return
			}
			time.Sleep(25 * time.Millisecond)
		}
	})
	return runtime, workspace
}

func ptr[T any](value T) *T {
	return &value
}

func TestRuntimeMessagePartsExposeToolResults(t *testing.T) {
	t.Parallel()

	msg := proto.Message{
		ID:        "tool-result-1",
		SessionID: "session-1",
		Role:      proto.Tool,
		Parts: []proto.ContentPart{
			proto.ToolResult{ToolCallID: "tool-1", Name: "ls", Content: "file.txt", IsError: false},
		},
	}

	got := toRuntimeMessage(msg)
	if !isDisplayableRuntimeMessage(got) {
		t.Fatal("tool result message should be displayable")
	}
	if got.Role != "tool" {
		t.Fatalf("Role = %q, want tool", got.Role)
	}
	if len(got.Parts) != 1 {
		t.Fatalf("Parts len = %d, want 1", len(got.Parts))
	}
	if got.Parts[0].Type != "tool_result" || got.Parts[0].ToolCallID != "tool-1" || got.Parts[0].Content != "file.txt" {
		t.Fatalf("tool result part not exposed: %#v", got.Parts[0])
	}
}

func TestIsDisplayableRuntimeMessageSkipsEmptyAssistantMessages(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		msg  RuntimeMessage
		want bool
	}{
		{
			name: "user text",
			msg: RuntimeMessage{
				Role:    "user",
				Content: "hello",
			},
			want: true,
		},
		{
			name: "assistant final text",
			msg: RuntimeMessage{
				Role:         "assistant",
				Content:      "done",
				Finished:     true,
				FinishReason: string(proto.FinishReasonEndTurn),
			},
			want: true,
		},
		{
			name: "empty assistant tool-use finish without parts",
			msg: RuntimeMessage{
				Role:         "assistant",
				Finished:     true,
				FinishReason: string(proto.FinishReasonToolUse),
			},
			want: false,
		},
		{
			name: "assistant tool call without text",
			msg: RuntimeMessage{
				Role:         "assistant",
				Parts:        []RuntimeMessagePart{{Type: "tool_call", ToolCallID: "tool-1", Name: "ls"}},
				Finished:     true,
				FinishReason: string(proto.FinishReasonToolUse),
			},
			want: true,
		},
		{
			name: "assistant error without text",
			msg: RuntimeMessage{
				Role:         "assistant",
				Finished:     true,
				FinishReason: string(proto.FinishReasonError),
				Error:        "provider error",
			},
			want: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := isDisplayableRuntimeMessage(tt.msg); got != tt.want {
				t.Fatalf("isDisplayableRuntimeMessage() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestRuntimePermissionRequestMapping(t *testing.T) {
	t.Parallel()

	perm := permission.PermissionRequest{
		ID:          "perm-1",
		SessionID:   "session-1",
		TurnID:      "turn-1",
		ToolCallID:  "tool-1",
		ToolName:    "bash",
		Description: "Run a command",
		Action:      "execute",
		Params:      map[string]any{"command": "pwd"},
		Path:        "C:\\work",
		Risk:        permission.RiskExecute,
		Status:      "pending",
		CreatedAt:   123,
	}

	runtimePerm := toRuntimePermissionRequest(perm)
	if runtimePerm.ID != perm.ID || runtimePerm.ToolName != perm.ToolName || runtimePerm.Action != perm.Action {
		t.Fatalf("runtime permission mapping failed: %#v", runtimePerm)
	}
	if runtimePerm.TurnID != "turn-1" || runtimePerm.Risk != "execute" || runtimePerm.Status != "pending" || runtimePerm.CreatedAt != 123 {
		t.Fatalf("runtime permission metadata failed: %#v", runtimePerm)
	}

	protoPerm := toProtoPermissionRequest(perm)
	if protoPerm.ID != perm.ID || protoPerm.ToolCallID != perm.ToolCallID || protoPerm.Path != perm.Path || protoPerm.TurnID != "turn-1" {
		t.Fatalf("proto permission mapping failed: %#v", protoPerm)
	}
}

func TestRuntimeRequestsLockedReportsActiveRequest(t *testing.T) {
	t.Parallel()

	now := time.Now().UnixMilli()
	service := &runtimeService{
		requests: map[string]runtimeRequestState{
			"cancelled":        {StartedAt: now - 4000, Status: turnStatusCancelled, Cancelled: true},
			"finished":         {StartedAt: now - 3000, Finished: true},
			"finished-at-only": {StartedAt: now - 2000, FinishedAt: now - 1000},
			"running":          {StartedAt: now - 1000},
		},
	}

	got := service.runtimeRequestsLocked()
	if got.Running != 1 {
		t.Fatalf("Running = %d, want 1", got.Running)
	}
	if got.ActiveRequestID != "running" {
		t.Fatalf("ActiveRequestID = %q, want running", got.ActiveRequestID)
	}
	if got.ActiveDurationMS <= 0 {
		t.Fatalf("ActiveDurationMS = %d, want positive", got.ActiveDurationMS)
	}
}

func TestAppendRuntimeEventLockedReturnsPublishEvent(t *testing.T) {
	t.Parallel()

	service := newRuntimeService()
	event := service.appendRuntimeEventLocked(RuntimeEvent{
		Type:      "message.created",
		SessionID: "session-1",
		MessageID: "message-1",
		Payload: map[string]any{
			"role":    "assistant",
			"summary": "hello",
		},
	})

	if event.ID == "" {
		t.Fatal("ID was not assigned")
	}
	if event.CreatedAt == "" {
		t.Fatal("CreatedAt was not assigned")
	}
	if event.Sequence != 1 {
		t.Fatalf("Sequence = %d, want 1", event.Sequence)
	}
	if event.Type != "message.created" || event.MessageID != "message-1" {
		t.Fatalf("event = %#v", event)
	}
	if len(service.events) != 1 {
		t.Fatalf("stored events = %d, want 1", len(service.events))
	}
}

func TestRuntimeEventsAfterCursorAndSnapshotRequired(t *testing.T) {
	t.Parallel()

	service := newRuntimeService()
	for i := 0; i < runtimeEventLimit+2; i++ {
		service.storeRuntimeEvent(RuntimeEvent{
			Type:      runtimeapi.EventMessageCreated,
			CreatedAt: time.Now().UTC().Format(time.RFC3339Nano),
			MessageID: fmt.Sprintf("message-%d", i),
		})
	}

	history, err := service.Events(context.Background(), service.events[len(service.events)-2].Sequence)
	if err != nil {
		t.Fatal(err)
	}
	if history.SnapshotRequired {
		t.Fatal("recent cursor should not require snapshot")
	}
	if len(history.Events) != 1 {
		t.Fatalf("events after recent cursor = %d, want 1", len(history.Events))
	}

	old, err := service.Events(context.Background(), 1)
	if err != nil {
		t.Fatal(err)
	}
	if !old.SnapshotRequired {
		t.Fatal("old cursor should require snapshot")
	}
	if old.FirstSequence <= 1 || old.LastSequence == 0 {
		t.Fatalf("sequence bounds = %d/%d", old.FirstSequence, old.LastSequence)
	}
}

func TestRecordRuntimeEventEmitsTurnScopedToolEvents(t *testing.T) {
	t.Parallel()

	service := newRuntimeService()
	service.sessionTurns["session-1"] = "turn-1"
	msg := message.Message{
		ID:        "message-1",
		SessionID: "session-1",
		Role:      message.Assistant,
		CreatedAt: time.Now().Add(-time.Second).UnixMilli(),
		UpdatedAt: time.Now().UnixMilli(),
		Parts: []message.ContentPart{
			message.ToolCall{ID: "tool-1", Name: "bash", Input: `{"command":"pwd"}`, Finished: true},
			message.ToolResult{ToolCallID: "tool-1", Name: "bash", Content: "C:/work"},
		},
	}

	service.recordRuntimeEvent(pubsub.Event[any]{
		Payload: pubsub.Event[message.Message]{Payload: msg},
	})

	var types []string
	for _, event := range service.events {
		types = append(types, event.Type)
		if event.TurnID != "turn-1" {
			t.Fatalf("event %s TurnID = %q, want turn-1", event.Type, event.TurnID)
		}
	}
	want := []string{
		runtimeapi.EventMessageUpdated,
		runtimeapi.EventToolCallStarted,
		runtimeapi.EventToolCallCompleted,
		runtimeapi.EventToolCallOutput,
	}
	if !slices.Equal(types, want) {
		t.Fatalf("event types = %#v, want %#v", types, want)
	}
	calls, err := service.TurnToolCalls(context.Background(), "turn-1")
	if err != nil {
		t.Fatal(err)
	}
	if len(calls.ToolCalls) != 1 || calls.ToolCalls[0].ID != "tool-1" || calls.ToolCalls[0].Status != "completed" {
		t.Fatalf("tool calls = %#v", calls.ToolCalls)
	}

	service.recordRuntimeEvent(pubsub.Event[any]{
		Payload: pubsub.Event[message.Message]{Payload: msg},
	})
	if len(service.events) != len(want)+1 {
		t.Fatalf("duplicate tool events were emitted: %#v", service.events)
	}
}

func TestRecordToolCallsBackfillDoesNotDowngradeSchedulerFinalState(t *testing.T) {
	t.Parallel()

	service := newRuntimeService()
	if _, err := service.toolCalls.CreateCall(context.Background(), scheduler.ToolCallRequest{
		ID:           "tool-1",
		SessionID:    "session-1",
		TurnID:       "turn-1",
		MessageID:    "message-1",
		Name:         "bash",
		Source:       scheduler.ToolSourceShell,
		InputSummary: "scheduler input",
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := service.toolCalls.CompleteCall(context.Background(), scheduler.ToolCallResult{
		ToolCallID:    "tool-1",
		Status:        scheduler.ToolCallFailed,
		OutputSummary: "scheduler failure",
		IsError:       true,
		Error:         "boom",
	}); err != nil {
		t.Fatal(err)
	}

	msg := proto.Message{
		ID:        "message-1",
		SessionID: "session-1",
		Role:      proto.Assistant,
		Parts: []proto.ContentPart{
			proto.ToolCall{ID: "tool-1", Name: "bash", Input: `{"command":"pwd"}`, Finished: false},
			proto.ToolResult{ToolCallID: "tool-1", Name: "bash", Content: "message success"},
		},
	}
	service.recordToolCallsFromMessage(context.Background(), msg, "turn-1", time.Now())

	call, err := service.toolCalls.GetCall(context.Background(), "tool-1")
	if err != nil {
		t.Fatal(err)
	}
	if call.Status != scheduler.ToolCallFailed || call.OutputSummary != "scheduler failure" || call.Error != "boom" {
		t.Fatalf("backfill downgraded scheduler state: %#v", call)
	}
}

func TestRuntimeToolCallCarriesCapabilityID(t *testing.T) {
	t.Parallel()

	service := newRuntimeService()
	if _, err := service.toolCalls.CreateCall(context.Background(), scheduler.ToolCallRequest{
		ID:           "tool-1",
		SessionID:    "session-1",
		TurnID:       "turn-1",
		Name:         "mcp_docs_search",
		Source:       scheduler.ToolSourceMCP,
		CapabilityID: "mcp:docs:search",
	}); err != nil {
		t.Fatal(err)
	}
	resp, err := service.ToolCall(context.Background(), "tool-1")
	if err != nil {
		t.Fatal(err)
	}
	if resp.ToolCall.Source != "mcp" || resp.ToolCall.CapabilityID != "mcp:docs:search" {
		t.Fatalf("tool call = %#v", resp.ToolCall)
	}
}

func TestRuntimeToolCallCarriesShellJobMetadata(t *testing.T) {
	t.Parallel()

	service := newRuntimeService()
	if _, err := service.toolCalls.CreateCall(context.Background(), scheduler.ToolCallRequest{
		ID:           "tool-1",
		SessionID:    "session-1",
		TurnID:       "turn-1",
		Name:         "bash",
		Source:       scheduler.ToolSourceShell,
		CapabilityID: "shell:bash",
		JobID:        "ABC",
		Command:      "go test ./...",
		Risk:         "execute",
		PolicyReason: "allowed",
		JobStatus:    "running",
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := service.toolCalls.CompleteCall(context.Background(), scheduler.ToolCallResult{
		ToolCallID:    "tool-1",
		Status:        scheduler.ToolCallCompleted,
		OutputSummary: "ok",
		Stdout:        "ok",
		ExitCode:      0,
		JobStatus:     "completed",
	}); err != nil {
		t.Fatal(err)
	}
	resp, err := service.ToolCall(context.Background(), "tool-1")
	if err != nil {
		t.Fatal(err)
	}
	call := resp.ToolCall
	if call.Source != "shell" || call.CapabilityID != "shell:bash" || call.JobID != "ABC" || call.Command != "go test ./..." || call.Risk != "execute" || call.Stdout != "ok" || call.JobStatus != "completed" {
		t.Fatalf("tool call shell metadata = %#v", call)
	}
	if call.Display.Kind != "shell" || call.Display.Title != "已运行 1 条命令" || call.Display.Command != "go test ./..." || call.Display.Detail != "go test ./..." {
		t.Fatalf("tool call display metadata = %#v", call.Display)
	}
}

func TestRuntimeToolCallDisplayExtractsFileTarget(t *testing.T) {
	t.Parallel()

	service := newRuntimeService()
	if _, err := service.toolCalls.CreateCall(context.Background(), scheduler.ToolCallRequest{
		ID:           "tool-read",
		SessionID:    "session-1",
		TurnID:       "turn-1",
		Name:         "view",
		Source:       scheduler.ToolSourceBuiltin,
		CapabilityID: "builtin:view",
		InputSummary: `{"file_path":"go.mod","limit":200}`,
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := service.toolCalls.CompleteCall(context.Background(), scheduler.ToolCallResult{
		ToolCallID:    "tool-read",
		Status:        scheduler.ToolCallCompleted,
		OutputSummary: "module github.com/charmbracelet/crush",
	}); err != nil {
		t.Fatal(err)
	}
	resp, err := service.ToolCall(context.Background(), "tool-read")
	if err != nil {
		t.Fatal(err)
	}
	if resp.ToolCall.Display.Kind != "file_read" || resp.ToolCall.Display.Title != "已读取文件" || resp.ToolCall.Display.Target != "go.mod" || resp.ToolCall.Display.Detail != "go.mod" {
		t.Fatalf("tool call file display metadata = %#v", resp.ToolCall.Display)
	}
}

func TestRuntimeToolCallDisplayKeepsViewAsReadWhenPathContainsWrite(t *testing.T) {
	t.Parallel()

	service := newRuntimeService()
	if _, err := service.toolCalls.CreateCall(context.Background(), scheduler.ToolCallRequest{
		ID:           "tool-view-write-path",
		SessionID:    "session-1",
		TurnID:       "turn-1",
		Name:         "view",
		Source:       scheduler.ToolSourceBuiltin,
		CapabilityID: "builtin:view",
		InputSummary: `{"file_path":"tmp/runtime-dev/long-write-smoke.md"}`,
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := service.toolCalls.CompleteCall(context.Background(), scheduler.ToolCallResult{
		ToolCallID:    "tool-view-write-path",
		Status:        scheduler.ToolCallCompleted,
		OutputSummary: "# Long Write Smoke Test",
	}); err != nil {
		t.Fatal(err)
	}
	resp, err := service.ToolCall(context.Background(), "tool-view-write-path")
	if err != nil {
		t.Fatal(err)
	}
	if resp.ToolCall.Display.Kind != "file_read" || resp.ToolCall.Display.Target != "tmp/runtime-dev/long-write-smoke.md" {
		t.Fatalf("view display metadata = %#v", resp.ToolCall.Display)
	}
}

func TestRuntimeToolCallDisplayKeepsTodosGenericWhenContentMentionsWrite(t *testing.T) {
	t.Parallel()

	service := newRuntimeService()
	if _, err := service.toolCalls.CreateCall(context.Background(), scheduler.ToolCallRequest{
		ID:           "tool-todospan",
		SessionID:    "session-1",
		TurnID:       "turn-1",
		Name:         "todospan",
		Source:       scheduler.ToolSourceBuiltin,
		CapabilityID: "builtin:todospan",
		InputSummary: `{"todos":[{"content":"Write the report","status":"in_progress"}]}`,
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := service.toolCalls.CompleteCall(context.Background(), scheduler.ToolCallResult{
		ToolCallID:    "tool-todospan",
		Status:        scheduler.ToolCallCompleted,
		OutputSummary: "Todo list updated successfully.",
	}); err != nil {
		t.Fatal(err)
	}
	resp, err := service.ToolCall(context.Background(), "tool-todospan")
	if err != nil {
		t.Fatal(err)
	}
	if resp.ToolCall.Display.Kind != "generic" {
		t.Fatalf("todo display metadata = %#v", resp.ToolCall.Display)
	}
}

func TestRuntimeToolCallDisplayClassifiesFileSearch(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		name   string
		input  string
		target string
	}{
		{name: "glob", input: `{"pattern":"**/*.go"}`, target: "**/*.go"},
		{name: "ls", input: `{"path":"internal/runtime"}`, target: "internal/runtime"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			service := newRuntimeService()
			if _, err := service.toolCalls.CreateCall(context.Background(), scheduler.ToolCallRequest{
				ID:           "tool-" + tc.name,
				SessionID:    "session-1",
				TurnID:       "turn-1",
				Name:         tc.name,
				Source:       scheduler.ToolSourceBuiltin,
				CapabilityID: "builtin:" + tc.name,
				InputSummary: tc.input,
			}); err != nil {
				t.Fatal(err)
			}
			if _, err := service.toolCalls.CompleteCall(context.Background(), scheduler.ToolCallResult{
				ToolCallID:    "tool-" + tc.name,
				Status:        scheduler.ToolCallCompleted,
				OutputSummary: "internal/runtime/runtime_tool_calls.go",
			}); err != nil {
				t.Fatal(err)
			}
			resp, err := service.ToolCall(context.Background(), "tool-"+tc.name)
			if err != nil {
				t.Fatal(err)
			}
			if resp.ToolCall.Display.Kind != "file_search" || resp.ToolCall.Display.Title != "已搜索文件" || resp.ToolCall.Display.Target != tc.target || resp.ToolCall.Display.Detail != tc.target {
				t.Fatalf("tool call search display metadata = %#v", resp.ToolCall.Display)
			}
		})
	}
}

func TestRuntimeToolCallDisplayExtractsTargetFromTruncatedJSON(t *testing.T) {
	t.Parallel()

	service := newRuntimeService()
	if _, err := service.toolCalls.CreateCall(context.Background(), scheduler.ToolCallRequest{
		ID:           "tool-write",
		SessionID:    "session-1",
		TurnID:       "turn-1",
		Name:         "write",
		Source:       scheduler.ToolSourceBuiltin,
		CapabilityID: "builtin:write",
		InputSummary: `{"file_path":"C:/repo/docs/report.md","content":"` + strings.Repeat("内容", 120),
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := service.toolCalls.CompleteCall(context.Background(), scheduler.ToolCallResult{
		ToolCallID: "tool-write",
		Status:     scheduler.ToolCallFailed,
		Error:      "truncated input",
		IsError:    true,
	}); err != nil {
		t.Fatal(err)
	}
	resp, err := service.ToolCall(context.Background(), "tool-write")
	if err != nil {
		t.Fatal(err)
	}
	if resp.ToolCall.Display.Kind != "file_write" || resp.ToolCall.Display.Target != "C:/repo/docs/report.md" || resp.ToolCall.Display.Detail != "C:/repo/docs/report.md" {
		t.Fatalf("truncated target display metadata = %#v", resp.ToolCall.Display)
	}
}

func TestRuntimeToolCallDisplayShellDetailMetadata(t *testing.T) {
	t.Parallel()

	path := filepath.Join(t.TempDir(), "shell-created.md")
	started := time.UnixMilli(1000)
	finished := time.UnixMilli(2750)
	call := toRuntimeToolCall(scheduler.ToolCall{
		ID:           "tool-shell",
		SessionID:    "session-1",
		TurnID:       "turn-1",
		Name:         "bash",
		Source:       scheduler.ToolSourceShell,
		Command:      `Set-Content -LiteralPath "` + path + `" -Value ok`,
		InputSummary: `{"command":"ignored","cwd":"C:/work/repo"}`,
		Stdout:       strings.Repeat("stdout line\n", 300),
		Stderr:       "stderr line",
		ArtifactRefs: []string{path},
		ExitCode:     7,
		Status:       scheduler.ToolCallFailed,
		StartedAt:    started,
		FinishedAt:   finished,
	})
	display := call.Display
	if display.Kind != "shell" || display.Command != `Set-Content -LiteralPath "`+path+`" -Value ok` || display.WorkingDir != "C:/work/repo" {
		t.Fatalf("shell display metadata = %#v", display)
	}
	if display.ExitCode == nil || *display.ExitCode != 7 || display.DurationMS != 1750 {
		t.Fatalf("shell exit/duration = %#v", display)
	}
	if display.PrimaryTarget != path || !slices.Contains(display.Targets, path) || display.ArtifactCount != 1 {
		t.Fatalf("shell target/artifact metadata = %#v", display)
	}
	if !strings.Contains(display.StdoutExcerpt, "... truncated ...") || !strings.Contains(display.StdoutExcerpt, "stdout line") || display.StderrExcerpt != "stderr line" {
		t.Fatalf("shell excerpts = stdout %q stderr %q", display.StdoutExcerpt, display.StderrExcerpt)
	}
	if display.FailureReason != "stderr line" {
		t.Fatalf("shell failure reason = %q", display.FailureReason)
	}
	nonzeroCompleted := toRuntimeToolCall(scheduler.ToolCall{
		ID:         "tool-shell-nonzero",
		SessionID:  "session-1",
		TurnID:     "turn-1",
		Name:       "bash",
		Source:     scheduler.ToolSourceShell,
		Command:    "exit 7",
		Stderr:     "nonzero stderr",
		ExitCode:   7,
		Status:     scheduler.ToolCallCompleted,
		StartedAt:  started,
		FinishedAt: finished,
	})
	if nonzeroCompleted.Display.FailureReason != "nonzero stderr" {
		t.Fatalf("nonzero shell failure reason = %#v", nonzeroCompleted.Display)
	}
	success := toRuntimeToolCall(scheduler.ToolCall{
		ID:         "tool-shell-ok",
		SessionID:  "session-1",
		TurnID:     "turn-1",
		Name:       "bash",
		Source:     scheduler.ToolSourceShell,
		Command:    "echo ok",
		ExitCode:   0,
		Status:     scheduler.ToolCallCompleted,
		StartedAt:  started,
		FinishedAt: finished,
	})
	if success.Display.ExitCode == nil || *success.Display.ExitCode != 0 {
		t.Fatalf("shell success exit code missing: %#v", success.Display)
	}
}

func TestRuntimeToolCallDisplayFileKindsAndTargets(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		name      string
		input     string
		wantKind  string
		want      string
		artifacts int
	}{
		{name: "write", input: `{"file_path":"tmp/runtime-dev/read-report.md","mode":"append"}`, wantKind: "file_write", want: "tmp/runtime-dev/read-report.md", artifacts: 1},
		{name: "view", input: `{"file_path":"tmp/runtime-dev/write-report.md"}`, wantKind: "file_read", want: "tmp/runtime-dev/write-report.md"},
		{name: "glob", input: `{"pattern":"**/*write*.go"}`, wantKind: "file_search", want: "**/*write*.go"},
		{name: "grep", input: `{"query":"read","path":"internal/runtime"}`, wantKind: "file_search", want: "internal/runtime"},
		{name: "multiedit", input: `{"file_path":"client/src/App.tsx","edits":[{"old":"a","new":"b"}]}`, wantKind: "file_edit", want: "client/src/App.tsx"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			call := toRuntimeToolCall(scheduler.ToolCall{
				ID:           "tool-" + tc.name,
				SessionID:    "session-1",
				TurnID:       "turn-1",
				Name:         tc.name,
				Source:       scheduler.ToolSourceBuiltin,
				InputSummary: tc.input,
				Status:       scheduler.ToolCallCompleted,
				StartedAt:    time.UnixMilli(1000),
				FinishedAt:   time.UnixMilli(1100),
			})
			if call.Display.Kind != tc.wantKind || call.Display.PrimaryTarget != tc.want || call.Display.Target != tc.want || !slices.Contains(call.Display.Targets, tc.want) {
				t.Fatalf("display metadata = %#v, want kind %s target %s", call.Display, tc.wantKind, tc.want)
			}
			if call.Display.ArtifactCount != tc.artifacts {
				t.Fatalf("artifact count = %d, want %d: %#v", call.Display.ArtifactCount, tc.artifacts, call.Display)
			}
		})
	}
}

func TestRuntimeToolCallDisplayCountsDiffAndStructuredArtifacts(t *testing.T) {
	t.Parallel()

	path := filepath.Join(t.TempDir(), "mcp-created.md")
	mcpCall := toRuntimeToolCall(scheduler.ToolCall{
		ID:         "tool-mcp",
		SessionID:  "session-1",
		TurnID:     "turn-1",
		Name:       "mcp_docs_writer",
		Source:     scheduler.ToolSourceMCP,
		Structured: `{"artifact_refs":[{"path":"` + strings.ReplaceAll(path, `\`, `\\`) + `"}]}`,
		Status:     scheduler.ToolCallCompleted,
		StartedAt:  time.UnixMilli(1000),
		FinishedAt: time.UnixMilli(1100),
	})
	if mcpCall.Display.Kind != "generic" || mcpCall.Display.PrimaryTarget != path || mcpCall.Display.ArtifactCount != 1 {
		t.Fatalf("mcp display metadata = %#v", mcpCall.Display)
	}

	editCall := toRuntimeToolCall(scheduler.ToolCall{
		ID:           "tool-edit",
		SessionID:    "session-1",
		TurnID:       "turn-1",
		Name:         "apply_patch",
		Source:       scheduler.ToolSourceBuiltin,
		InputSummary: `{"file_path":"internal/runtime/runtime_tool_calls.go"}`,
		DiffRefs:     []string{"ref-diff-1", "ref-diff-2"},
		Status:       scheduler.ToolCallCompleted,
		StartedAt:    time.UnixMilli(1000),
		FinishedAt:   time.UnixMilli(1100),
	})
	if editCall.Display.Kind != "file_edit" || editCall.Display.DiffCount != 2 || editCall.Display.DiffSummary != "2 diff refs" {
		t.Fatalf("edit display metadata = %#v", editCall.Display)
	}
}

func TestRuntimeToolCallDisplayShellCommandKeywordsStayShell(t *testing.T) {
	t.Parallel()

	call := toRuntimeToolCall(scheduler.ToolCall{
		ID:           "tool-shell",
		SessionID:    "session-1",
		TurnID:       "turn-1",
		Name:         "bash",
		Source:       scheduler.ToolSourceShell,
		Command:      `echo write > C:\tmp\read-output.md`,
		InputSummary: `{"command":"echo write > C:\\tmp\\read-output.md"}`,
		Status:       scheduler.ToolCallCompleted,
		StartedAt:    time.UnixMilli(1000),
		FinishedAt:   time.UnixMilli(1100),
	})
	if call.Display.Kind != "shell" {
		t.Fatalf("shell command keyword changed kind: %#v", call.Display)
	}
}

func TestRuntimeSessionActivityPreservesGroupedToolDisplayMetadata(t *testing.T) {
	dataDir := filepath.Join(t.TempDir(), "runtime-state")
	if err := os.MkdirAll(dataDir, 0o700); err != nil {
		t.Fatal(err)
	}
	conn, err := db.Connect(context.Background(), dataDir)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_ = db.Release(dataDir)
	})

	service := newRuntimeService()
	service.turns = newRuntimeTurnStore(conn)
	service.toolCalls = scheduler.New(NewRuntimeToolCallStoreForDB(conn))
	service.permissionStore = newRuntimePermissionStore(conn)
	workingDir := t.TempDir()
	cfg := config.NewRuntimeConfig(workingDir, dataDir, false)
	cfg.Options.AutoLSP = ptr(false)
	store := config.NewRuntimeStore(workingDir, cfg)
	runtimeBackend := backend.New(context.Background(), store, nil)
	_, workspace, err := runtimeBackend.CreateWorkspace(proto.Workspace{
		Path:    workingDir,
		DataDir: dataDir,
		Config:  store.Config(),
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		runtimeBackend.DeleteWorkspace(workspace.ID)
	})
	service.runtime = runtimeBackend
	service.workspace = &proto.Workspace{ID: workspace.ID, Path: workspace.Path}
	providerStore := newRuntimeProviderSettingsStore(conn)
	if err := service.syncProviderCatalog(context.Background(), providerStore); err != nil {
		t.Fatal(err)
	}
	provider, err := providerStore.UpsertConfigured(context.Background(), RuntimeConfiguredProviderRequest{
		ID:           "provider-test",
		ProviderID:   "custom",
		Name:         "Test Provider",
		Protocol:     "openai-compat",
		APIEndpoint:  "http://127.0.0.1:9",
		APIKey:       "test-key",
		DefaultModel: "test-model",
		Enabled:      true,
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := newRuntimeSelectedModelStore(conn).Upsert(context.Background(), RuntimeSelectedModelRequest{
		ConfiguredProviderID: provider.ID,
		Model:                provider.DefaultModel,
		Scope:                "global",
	}, provider); err != nil {
		t.Fatal(err)
	}
	sess, err := runtimeBackend.CreateSession(context.Background(), workspace.ID, "Tool metadata")
	if err != nil {
		t.Fatal(err)
	}

	started := time.UnixMilli(1000)
	if _, err := service.turns.Upsert(context.Background(), RuntimeTurn{
		ID:         "turn-tools",
		SessionID:  sess.ID,
		Status:     turnStatusCompleted,
		StartedAt:  started.UnixMilli(),
		FinishedAt: started.Add(3 * time.Second).UnixMilli(),
	}); err != nil {
		t.Fatal(err)
	}
	for _, req := range []scheduler.ToolCallRequest{
		{ID: "tool-write", SessionID: sess.ID, TurnID: "turn-tools", Name: "write", Source: scheduler.ToolSourceBuiltin, InputSummary: `{"file_path":"tmp/runtime-dev/group-write.md"}`},
		{ID: "tool-shell-fail", SessionID: sess.ID, TurnID: "turn-tools", Name: "bash", Source: scheduler.ToolSourceShell, Command: "exit 7", InputSummary: `{"cwd":"C:/work/repo"}`},
		{ID: "tool-edit", SessionID: sess.ID, TurnID: "turn-tools", Name: "apply_patch", Source: scheduler.ToolSourceBuiltin, InputSummary: `{"file_path":"tmp/runtime-dev/group-edit.md"}`},
	} {
		if _, err := service.toolCalls.CreateCall(context.Background(), req); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := service.toolCalls.CompleteCall(context.Background(), scheduler.ToolCallResult{
		ToolCallID:   "tool-write",
		Status:       scheduler.ToolCallCompleted,
		ArtifactRefs: []string{"tmp/runtime-dev/group-write.md"},
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := service.toolCalls.CompleteCall(context.Background(), scheduler.ToolCallResult{
		ToolCallID: "tool-shell-fail",
		Status:     scheduler.ToolCallFailed,
		ExitCode:   7,
		Stderr:     "boom",
		Error:      "boom",
		IsError:    true,
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := service.toolCalls.CompleteCall(context.Background(), scheduler.ToolCallResult{
		ToolCallID: "tool-edit",
		Status:     scheduler.ToolCallCompleted,
		DiffRefs:   []string{"diff-1"},
	}); err != nil {
		t.Fatal(err)
	}

	activity, err := service.SessionActivity(context.Background(), sess.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(activity.ToolCalls) != 3 {
		t.Fatalf("tool calls = %#v", activity.ToolCalls)
	}
	byID := map[string]RuntimeToolCall{}
	for _, call := range activity.ToolCalls {
		byID[call.ID] = call
	}
	if byID["tool-write"].Status != string(scheduler.ToolCallCompleted) || byID["tool-write"].Display.PrimaryTarget != "tmp/runtime-dev/group-write.md" || byID["tool-write"].Display.ArtifactCount == 0 {
		t.Fatalf("write metadata lost: %#v", byID["tool-write"])
	}
	if byID["tool-shell-fail"].Status != string(scheduler.ToolCallFailed) || byID["tool-shell-fail"].Display.ExitCode == nil || *byID["tool-shell-fail"].Display.ExitCode != 7 || byID["tool-shell-fail"].Display.FailureReason != "boom" {
		t.Fatalf("failed shell metadata lost: %#v", byID["tool-shell-fail"])
	}
	if byID["tool-edit"].Status != string(scheduler.ToolCallCompleted) || byID["tool-edit"].Display.DiffCount != 1 {
		t.Fatalf("edit metadata lost: %#v", byID["tool-edit"])
	}
}

func TestRuntimeToolCallRedactsShellSecrets(t *testing.T) {
	t.Parallel()

	service := newRuntimeService()
	if _, err := service.toolCalls.CreateCall(context.Background(), scheduler.ToolCallRequest{
		ID:        "tool-1",
		SessionID: "session-1",
		TurnID:    "turn-1",
		Name:      "bash",
		Source:    scheduler.ToolSourceShell,
		Command:   `echo api_key=sk-secret`,
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := service.toolCalls.CompleteCall(context.Background(), scheduler.ToolCallResult{
		ToolCallID:    "tool-1",
		Status:        scheduler.ToolCallCompleted,
		OutputSummary: "Authorization: Bearer secret-token",
		Stdout:        "token=secret-token",
	}); err != nil {
		t.Fatal(err)
	}
	resp, err := service.ToolCall(context.Background(), "tool-1")
	if err != nil {
		t.Fatal(err)
	}
	data, err := json.Marshal(resp.ToolCall)
	if err != nil {
		t.Fatal(err)
	}
	text := string(data)
	if strings.Contains(text, "sk-secret") || strings.Contains(text, "secret-token") {
		t.Fatalf("runtime tool call leaked shell secret: %s", text)
	}
}

func TestRuntimeSchedulerDeniedShellRecordsFinalToolCall(t *testing.T) {
	t.Parallel()

	service := newRuntimeService()
	service.policy = runtimePolicyFromMode(permission.PolicyModePlan, 0)
	recorder := runtimeSchedulerRecorder{service: service}

	decision, err := recorder.EvaluateToolCall(context.Background(), agent.SchedulerToolCall{
		ID:           "tool-denied",
		SessionID:    "session-1",
		TurnID:       "turn-1",
		Name:         "bash",
		Source:       "shell",
		CapabilityID: "shell:bash",
		InputSummary: `{"command":"go test ./..."}`,
	})
	if err != nil {
		t.Fatal(err)
	}
	if decision.Decision != string(permission.PolicyDeny) || decision.Risk != string(permission.RiskExecute) {
		t.Fatalf("decision = %#v", decision)
	}
	if err := recorder.ToolCallFailed(context.Background(), agent.SchedulerToolCallResult{
		ToolCallID:              "tool-denied",
		SessionID:               "session-1",
		TurnID:                  "turn-1",
		Name:                    "bash",
		Source:                  "shell",
		Command:                 "go test ./...",
		Risk:                    decision.Risk,
		PolicyReason:            decision.Reason,
		StructuredOutputSummary: "policy=deny risk=execute mode=plan",
		Error:                   decision.Reason,
		IsError:                 true,
		Status:                  string(scheduler.ToolCallDenied),
	}); err != nil {
		t.Fatal(err)
	}
	call, err := service.toolCalls.GetCall(context.Background(), "tool-denied")
	if err != nil {
		t.Fatal(err)
	}
	if call.Status != scheduler.ToolCallDenied || call.FinishedAt.IsZero() || !call.IsError {
		t.Fatalf("denied tool call not final: %#v", call)
	}
	events, err := service.Events(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if !slices.ContainsFunc(events.Events, func(event RuntimeEvent) bool {
		return event.Type == runtimeapi.EventToolCallFailed && event.ToolCallID == "tool-denied" && event.Payload["denied"] == true
	}) {
		t.Fatalf("denied failed event missing: %#v", events.Events)
	}
}

func TestRuntimeSchedulerBackgroundJobOutputEmitsTaskProgress(t *testing.T) {
	t.Parallel()

	service := newRuntimeService()
	recorder := runtimeSchedulerRecorder{service: service}

	if err := recorder.ToolCallStarted(context.Background(), agent.SchedulerToolCall{
		ID:           "tool-job",
		SessionID:    "session-1",
		TurnID:       "turn-1",
		Name:         "bash",
		Source:       "shell",
		CapabilityID: "shell:bash",
		JobID:        "job-1",
		Command:      "sleep 10",
		Risk:         string(permission.RiskExecute),
		JobStatus:    "running",
	}); err != nil {
		t.Fatal(err)
	}
	if err := recorder.ToolCallOutput(context.Background(), agent.SchedulerToolCallResult{
		ToolCallID:              "tool-job",
		SessionID:               "session-1",
		TurnID:                  "turn-1",
		Name:                    "bash",
		Source:                  "shell",
		JobID:                   "job-1",
		Command:                 "sleep 10",
		Risk:                    string(permission.RiskExecute),
		JobStatus:               "running",
		StructuredOutputSummary: `{"status":"running"}`,
		Stdout:                  "partial",
	}); err != nil {
		t.Fatal(err)
	}

	events, err := service.Events(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if !slices.ContainsFunc(events.Events, func(event RuntimeEvent) bool {
		return event.Type == runtimeapi.EventTaskProgress && event.ToolCallID == "tool-job" && event.Payload["job_id"] == "job-1"
	}) {
		t.Fatalf("task.progress event missing: %#v", events.Events)
	}
	call, err := service.ToolCall(context.Background(), "tool-job")
	if err != nil {
		t.Fatal(err)
	}
	if call.ToolCall.JobID != "job-1" || call.ToolCall.JobStatus != "running" || call.ToolCall.Stdout != "partial" {
		t.Fatalf("job metadata missing: %#v", call.ToolCall)
	}
}

func TestRuntimeAgentTaskRecorderEmitsEventsAndAudit(t *testing.T) {
	dataDir := t.TempDir()
	t.Cleanup(db.ResetPool)
	conn, err := db.Connect(context.Background(), dataDir)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_ = db.Release(dataDir)
	})

	service := newRuntimeService()
	service.agentTasks = newRuntimeAgentTaskStore(conn)
	workingDir := t.TempDir()
	cfg := config.NewRuntimeConfig(workingDir, dataDir, false)
	cfg.Options.AutoLSP = ptr(false)
	store := config.NewRuntimeStore(workingDir, cfg)
	runtimeBackend := backend.New(context.Background(), store, nil)
	_, workspace, err := runtimeBackend.CreateWorkspace(proto.Workspace{
		Path:    workingDir,
		DataDir: dataDir,
		Config:  store.Config(),
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		runtimeBackend.DeleteWorkspace(workspace.ID)
		_ = db.Release(dataDir)
	})
	service.runtime = runtimeBackend
	service.workspace = &proto.Workspace{ID: workspace.ID, Path: workspace.Path}

	recorder := runtimeSchedulerRecorder{service: service}
	record := agent.AgentTaskRecord{
		ID:               "task-1",
		ParentTurnID:     "turn-1",
		ParentSessionID:  "session-parent",
		ParentToolCallID: "tool-1",
		ChildSessionID:   "session-child",
		Title:            "Fetch Analysis",
		Kind:             agentTaskKindAgenticFetch,
		Role:             "fetch",
		Name:             "agentic_fetch",
		PromptSummary:    "fetch prompt with secret token=abc",
		Provider:         "openai",
		Model:            "gpt-test",
		AllowedTools:     []string{"web_fetch"},
		CapabilityScope:  []string{"network"},
		Progress:         10,
	}
	if err := recorder.AgentTaskStarted(context.Background(), record); err != nil {
		t.Fatal(err)
	}
	record.ResultSummary = "done"
	if err := recorder.AgentTaskCompleted(context.Background(), record); err != nil {
		t.Fatal(err)
	}

	task, err := service.AgentTask(context.Background(), "task-1")
	if err != nil {
		t.Fatal(err)
	}
	if task.Task.Status != agentTaskStatusCompleted || task.Task.ParentToolCallID != "tool-1" || task.Task.ChildSessionID != "session-child" {
		t.Fatalf("task = %#v", task.Task)
	}
	events, err := service.Events(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if !slices.ContainsFunc(events.Events, func(event RuntimeEvent) bool {
		return event.Type == runtimeapi.EventTaskStarted && event.Payload["task_id"] == "task-1" && event.ToolCallID == "tool-1"
	}) {
		t.Fatalf("task.started event missing: %#v", events.Events)
	}
	if !slices.ContainsFunc(events.Events, func(event RuntimeEvent) bool {
		return event.Type == runtimeapi.EventTaskCompleted && event.Payload["status"] == agentTaskStatusCompleted
	}) {
		t.Fatalf("task.completed event missing: %#v", events.Events)
	}
	audit, err := newRuntimeAuditStore(conn).ListTurn(context.Background(), "turn-1")
	if err != nil {
		t.Fatal(err)
	}
	if !slices.ContainsFunc(audit.Events, func(event RuntimeAuditEvent) bool {
		return event.Type == "task_completed" && event.ToolCallID == "tool-1" && event.Payload["agent_task"] != nil
	}) {
		t.Fatalf("task audit missing: %#v", audit.Events)
	}
	summary := service.auditTurnSummary(context.Background(), "turn-1", audit.Events)
	if len(summary.Tasks) != 1 || summary.Tasks[0].ID != "task-1" {
		t.Fatalf("audit summary tasks = %#v", summary.Tasks)
	}
}

func TestRuntimeCancelAgentTaskMarksFinalAndAuditsLimitation(t *testing.T) {
	t.Parallel()

	dataDir := t.TempDir()
	conn, err := db.Connect(context.Background(), dataDir)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_ = db.Release(dataDir)
	})

	service := newRuntimeService()
	service.agentTasks = newRuntimeAgentTaskStore(conn)
	if _, err := service.agentTasks.Upsert(context.Background(), RuntimeAgentTask{
		ID:               "task-1",
		ParentTurnID:     "turn-1",
		ParentSessionID:  "session-parent",
		ParentToolCallID: "tool-1",
		ChildSessionID:   "session-child",
		Title:            "Agent",
		Kind:             agentTaskKindSubagent,
		Status:           agentTaskStatusRunning,
	}); err != nil {
		t.Fatal(err)
	}
	resp, err := service.CancelAgentTask(context.Background(), "task-1")
	if err != nil {
		t.Fatal(err)
	}
	if resp.Task.Status != agentTaskStatusCancelled || resp.Task.FinishedAt == 0 {
		t.Fatalf("cancelled task = %#v", resp.Task)
	}
	if resp.Action == nil || !resp.Action.Accepted || resp.Action.Source.Kind != runtimeAgentTaskCancelSourceKind || resp.Action.Source.Action != runtimeAgentTaskCancelAction || resp.Action.Source.IdempotentBy != "task_id" {
		t.Fatalf("cancel action metadata = %#v", resp.Action)
	}
	if len(resp.Action.RefreshTargets) == 0 || !resp.Action.Source.BackendOnly || resp.Action.Source.StartsWorker || !resp.Action.Source.SessionActivityParity {
		t.Fatalf("cancel action source/refresh metadata = %#v", resp.Action)
	}
	events, err := service.Events(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if !slices.ContainsFunc(events.Events, func(event RuntimeEvent) bool {
		return event.Type == runtimeapi.EventTaskCancelled && event.Payload["task_id"] == "task-1"
	}) {
		t.Fatalf("task.cancelled missing: %#v", events.Events)
	}
}

func TestRuntimeCancelAgentTaskTerminalizesEvidenceAndPreservesOwnership(t *testing.T) {
	t.Parallel()

	dataDir := t.TempDir()
	conn, err := db.Connect(context.Background(), dataDir)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_ = db.Release(dataDir)
	})

	service := newRuntimeService()
	service.agentTasks = newRuntimeAgentTaskStore(conn)
	service.turns = newRuntimeTurnStore(conn)
	service.runs = newRuntimeRunStore(conn)
	service.eventStore = newRuntimeEventStore(conn)

	run, err := service.runs.EnsureForSession(context.Background(), "workspace-1", "session-parent", "cancel task", runtimeRunSourceUserPrompt)
	if err != nil {
		t.Fatal(err)
	}
	turn, err := service.turns.Upsert(context.Background(), RuntimeTurn{
		ID:        "turn-parent",
		SessionID: "session-parent",
		Status:    turnStatusRunning,
		StartedAt: 1000,
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := service.runs.LinkTurn(context.Background(), run.ID, "session-parent", turn.ID, turn.StartedAt); err != nil {
		t.Fatal(err)
	}
	task, err := service.agentTasks.Upsert(context.Background(), RuntimeAgentTask{
		ID:               "task-cancel",
		ParentTurnID:     turn.ID,
		ParentSessionID:  "session-parent",
		ParentToolCallID: "tool-parent",
		ChildSessionID:   "session-child",
		Title:            "Agent",
		Kind:             agentTaskKindSubagent,
		Role:             "reviewer",
		Provider:         "provider-1",
		Model:            "model-1",
		AllowedTools:     []string{"view"},
		CapabilityScope:  []string{"C:/work/project"},
		CWD:              "C:/work/project",
		Worktree:         "worktree-1",
		Status:           agentTaskStatusRunning,
		Progress:         25,
		StartedAt:        1100,
	})
	if err != nil {
		t.Fatal(err)
	}

	resp, err := service.CancelAgentTask(context.Background(), task.ID)
	if err != nil {
		t.Fatal(err)
	}
	if resp.Task.Status != agentTaskStatusCancelled || resp.Task.Progress != 100 || resp.Task.FinishedAt == 0 {
		t.Fatalf("cancelled task = %#v", resp.Task)
	}
	if resp.Task.ParentSessionID != task.ParentSessionID || resp.Task.ParentTurnID != task.ParentTurnID || resp.Task.ParentToolCallID != task.ParentToolCallID || resp.Task.ChildSessionID != task.ChildSessionID {
		t.Fatalf("ownership links changed: task=%#v original=%#v", resp.Task, task)
	}
	if resp.Task.Role != task.Role || resp.Task.Provider != task.Provider || resp.Task.Model != task.Model || resp.Task.CWD != task.CWD || resp.Task.Worktree != task.Worktree {
		t.Fatalf("task scope changed: task=%#v original=%#v", resp.Task, task)
	}
	if resp.Result == nil || resp.Result.TaskID != task.ID || resp.Result.Status != agentTaskStatusCancelled || resp.Result.CancellationDetail == "" || len(resp.Result.ArtifactRefs) != 0 {
		t.Fatalf("cancel result = %#v", resp.Result)
	}
	if resp.Action == nil || !resp.Action.Accepted || resp.Action.Reason != resp.Result.CancellationDetail {
		t.Fatalf("cancel action metadata = %#v result=%#v", resp.Action, resp.Result)
	}
	if !slices.Contains(resp.Action.Source.Evidence, "runtime_agent_tasks") ||
		!slices.Contains(resp.Action.Source.Evidence, "runtime_agent_task_results") ||
		!slices.Contains(resp.Action.Source.Evidence, "runtime_agent_task_messages") ||
		!slices.Contains(resp.Action.Source.Evidence, "session_activity") {
		t.Fatalf("cancel action evidence = %#v", resp.Action.Source.Evidence)
	}

	refreshedTask, err := service.agentTasks.Get(context.Background(), task.ID)
	if err != nil {
		t.Fatal(err)
	}
	if refreshedTask.Status != agentTaskStatusCancelled || refreshedTask.ParentSessionID != task.ParentSessionID || refreshedTask.ParentTurnID != task.ParentTurnID || refreshedTask.ParentToolCallID != task.ParentToolCallID || refreshedTask.ChildSessionID != task.ChildSessionID {
		t.Fatalf("persisted cancelled task = %#v", refreshedTask)
	}
	refreshedResult, err := newRuntimeAgentTaskResultStore(conn).Get(context.Background(), task.ID)
	if err != nil {
		t.Fatal(err)
	}
	if refreshedResult.Status != agentTaskStatusCancelled || refreshedResult.CancellationDetail == "" || len(refreshedResult.ArtifactRefs) != 0 {
		t.Fatalf("persisted cancel result = %#v", refreshedResult)
	}
	messages, err := newRuntimeAgentTaskMessageStore(conn).ListByTask(context.Background(), task.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(messages) != 1 {
		t.Fatalf("messages = %#v", messages)
	}
	msg := messages[0]
	if msg.Direction != taskMessageDirectionParentToChild || msg.Kind != taskMessageKindControl || msg.Status != taskMessageStatusProcessed || msg.RelatedToolCallID != task.ParentToolCallID {
		t.Fatalf("cancel message = %#v", msg)
	}
	if msg.ParentSessionID != task.ParentSessionID || msg.ParentTurnID != task.ParentTurnID || msg.ChildSessionID != task.ChildSessionID {
		t.Fatalf("message ownership links changed: %#v", msg)
	}

	events, err := service.Events(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if !slices.ContainsFunc(events.Events, func(event RuntimeEvent) bool {
		return event.Type == runtimeapi.EventTaskCancelled &&
			event.SessionID == task.ParentSessionID &&
			event.TurnID == task.ParentTurnID &&
			event.ToolCallID == task.ParentToolCallID &&
			event.Payload["task_id"] == task.ID &&
			event.Payload["status"] == agentTaskStatusCancelled
	}) {
		t.Fatalf("task.cancelled event missing: %#v", events.Events)
	}
	if slices.ContainsFunc(events.Events, func(event RuntimeEvent) bool {
		return event.Type == runtimeapi.EventTaskArtifactCreated
	}) {
		t.Fatalf("cancel without completed artifact output created artifact event: %#v", events.Events)
	}
}

func TestRuntimeCancelAgentTaskAlreadyFinalDoesNotRewriteEvidence(t *testing.T) {
	t.Parallel()

	dataDir := t.TempDir()
	conn, err := db.Connect(context.Background(), dataDir)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_ = db.Release(dataDir)
	})

	service := newRuntimeService()
	service.agentTasks = newRuntimeAgentTaskStore(conn)
	service.turns = newRuntimeTurnStore(conn)
	service.eventStore = newRuntimeEventStore(conn)
	task, err := service.agentTasks.Upsert(context.Background(), RuntimeAgentTask{
		ID:               "task-final",
		ParentTurnID:     "turn-parent",
		ParentSessionID:  "session-parent",
		ParentToolCallID: "tool-parent",
		ChildSessionID:   "session-child",
		Title:            "Agent",
		Kind:             agentTaskKindSubagent,
		Status:           agentTaskStatusCompleted,
		Progress:         100,
		ResultSummary:    "completed output",
		ArtifactRefs:     []string{"runtime://refs/task-output"},
		StartedAt:        1000,
		FinishedAt:       2000,
	})
	if err != nil {
		t.Fatal(err)
	}
	result, err := service.upsertAgentTaskResult(context.Background(), RuntimeAgentTaskResult{
		TaskID:       task.ID,
		Status:       agentTaskStatusCompleted,
		Summary:      "completed output",
		ArtifactRefs: []string{"runtime://refs/task-output"},
	})
	if err != nil {
		t.Fatal(err)
	}

	resp, err := service.CancelAgentTask(context.Background(), task.ID)
	if err != nil {
		t.Fatal(err)
	}
	if resp.Task.Status != agentTaskStatusCompleted || resp.Task.FinishedAt != task.FinishedAt || resp.Task.ResultSummary != task.ResultSummary {
		t.Fatalf("final task rewritten = %#v original=%#v", resp.Task, task)
	}
	if resp.Action == nil || resp.Action.Accepted || resp.Action.Reason != runtimeAgentTaskCancelReasonAlreadyFinal || resp.Action.Source.IdempotentBy != "task_id" {
		t.Fatalf("final cancel action metadata = %#v", resp.Action)
	}
	if len(resp.Task.ArtifactRefs) != 1 || resp.Task.ArtifactRefs[0] != "runtime://refs/task-output" {
		t.Fatalf("final task artifact refs rewritten = %#v", resp.Task.ArtifactRefs)
	}
	refreshedResult, err := newRuntimeAgentTaskResultStore(conn).Get(context.Background(), task.ID)
	if err != nil {
		t.Fatal(err)
	}
	if refreshedResult.Status != result.Status || refreshedResult.Summary != result.Summary || refreshedResult.CancellationDetail != "" || len(refreshedResult.ArtifactRefs) != 1 || refreshedResult.ArtifactRefs[0] != result.ArtifactRefs[0] {
		t.Fatalf("final result rewritten = %#v original=%#v", refreshedResult, result)
	}
	messages, err := newRuntimeAgentTaskMessageStore(conn).ListByTask(context.Background(), task.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(messages) != 1 || messages[0].Status != taskMessageStatusRejected || messages[0].Kind != taskMessageKindControl || messages[0].Error == "" {
		t.Fatalf("final cancel rejection message = %#v", messages)
	}
	events, err := service.Events(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if slices.ContainsFunc(events.Events, func(event RuntimeEvent) bool {
		return event.Type == runtimeapi.EventTaskCancelled
	}) {
		t.Fatalf("final cancel emitted task.cancelled: %#v", events.Events)
	}
}

func TestRuntimeAgentTaskFollowUpDeliveryAndRejection(t *testing.T) {
	t.Parallel()

	dataDir := t.TempDir()
	conn, err := db.Connect(context.Background(), dataDir)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_ = db.Release(dataDir)
	})

	service := newRuntimeService()
	service.agentTasks = newRuntimeAgentTaskStore(conn)
	service.turns = newRuntimeTurnStore(conn)
	service.eventStore = newRuntimeEventStore(conn)
	if _, err := service.agentTasks.Upsert(context.Background(), RuntimeAgentTask{
		ID:              "task-1",
		ParentTurnID:    "turn-1",
		ParentSessionID: "session-parent",
		ChildSessionID:  "session-child",
		Title:           "Agent",
		Kind:            agentTaskKindSubagent,
		Status:          agentTaskStatusRunning,
	}); err != nil {
		t.Fatal(err)
	}

	resp, err := service.SendAgentTaskFollowUp(context.Background(), "task-1", RuntimeAgentTaskMessageCreateRequest{
		ContentSummary: "continue",
	})
	if err == nil {
		t.Fatal("expected delivery error without runtime backend")
	}
	if resp.Message.Status != taskMessageStatusRejected || resp.Message.Sequence != 1 || resp.Message.Error == "" {
		t.Fatalf("rejected message = %#v", resp.Message)
	}
	if _, err := service.agentTasks.Upsert(context.Background(), RuntimeAgentTask{
		ID:              "task-1",
		ParentTurnID:    "turn-1",
		ParentSessionID: "session-parent",
		Status:          agentTaskStatusCompleted,
	}); err != nil {
		t.Fatal(err)
	}
	finalResp, err := service.SendAgentTaskFollowUp(context.Background(), "task-1", RuntimeAgentTaskMessageCreateRequest{
		ContentSummary: "too late",
	})
	if err == nil {
		t.Fatal("expected final task rejection")
	}
	if finalResp.Message.Status != taskMessageStatusRejected || finalResp.Message.Sequence != 2 {
		t.Fatalf("final rejected message = %#v", finalResp.Message)
	}
	events, err := service.ReplayExport(context.Background(), RuntimeReplayExportRequest{TurnID: "turn-1"})
	if err != nil {
		t.Fatal(err)
	}
	if !slices.ContainsFunc(events.Summary.AgentTaskMessages, func(msg RuntimeAgentTaskMessage) bool {
		return msg.TaskID == "task-1" && msg.Status == taskMessageStatusRejected && msg.Sequence == 1
	}) {
		t.Fatalf("replay messages = %#v", events.Summary.AgentTaskMessages)
	}
}

func TestRuntimeAgentTaskToolOutputUsesRefs(t *testing.T) {
	t.Parallel()

	dataDir := t.TempDir()
	conn, err := db.Connect(context.Background(), dataDir)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_ = db.Release(dataDir)
	})

	service := newRuntimeService()
	service.agentTasks = newRuntimeAgentTaskStore(conn)
	service.refs = newRuntimeRefStore(conn, dataDir)
	service.turns = newRuntimeTurnStore(conn)
	if _, err := service.agentTasks.Upsert(context.Background(), RuntimeAgentTask{
		ID:               "task-1",
		ParentTurnID:     "turn-1",
		ParentSessionID:  "session-parent",
		ParentToolCallID: "tool-1",
		Title:            "Agent",
		Status:           agentTaskStatusCompleted,
		ResultSummary:    strings.Repeat("x", runtimePartPreviewLimit*2),
		ArtifactRefs:     []string{"artifact-a"},
	}); err != nil {
		t.Fatal(err)
	}
	_, err = service.createRuntimeRef(context.Background(), runtimeRefCreateRequest{
		SessionID:   "session-parent",
		TurnID:      "turn-1",
		ToolCallID:  "tool-1",
		TaskID:      "task-1",
		Kind:        runtimeRefKindTaskArtifact,
		MediaType:   "text/plain",
		ContentType: "task_output",
		Payload:     []byte(strings.Repeat("large-output", 200)),
		Summary:     "large task output",
	})
	if err != nil {
		t.Fatal(err)
	}
	out, err := service.GetAgentTaskOutputForTool(context.Background(), agent.AgentTaskToolOutputRequest{TaskID: "task-1"})
	if err != nil {
		t.Fatal(err)
	}
	if len(out.OutputRefs) == 0 {
		t.Fatalf("expected output refs: %#v", out)
	}
	if len(out.Summary) > runtimePartPreviewLimit {
		t.Fatalf("summary not previewed: len=%d", len(out.Summary))
	}
}

func TestRecordRuntimeEventConvertsSessionAndPermissionPayloads(t *testing.T) {
	t.Parallel()

	service := newRuntimeService()
	service.recordRuntimeEvent(pubsub.Event[any]{
		Payload: pubsub.Event[session.Session]{
			Payload: session.Session{ID: "session-1", Title: "Runtime session"},
		},
	})
	service.recordRuntimeEvent(pubsub.Event[any]{
		Payload: pubsub.Event[permission.PermissionRequest]{
			Payload: permission.PermissionRequest{ID: "perm-1"},
		},
	})

	if len(service.events) != 1 {
		t.Fatalf("events = %#v, want one session runtime event", service.events)
	}
	if service.events[0].Type != runtimeapi.EventSessionUpdated || service.events[0].SessionID != "session-1" {
		t.Fatalf("session event = %#v", service.events[0])
	}
	if service.eventStats.sessionEvents != 1 || service.eventStats.permissionEvents != 1 {
		t.Fatalf("event stats = %#v", service.eventStats)
	}
}

func TestTurnRuntimeEventsCarryUsageAndStatus(t *testing.T) {
	t.Parallel()

	usage := RuntimeUsage{PromptTokens: 3, CompletionTokens: 4, TotalTokens: 7, Cost: 0.01}
	delta := RuntimeUsage{PromptTokens: 1, CompletionTokens: 2, TotalTokens: 3, Cost: 0.001}
	usageEvent := newUsageRuntimeEvent(time.Now(), "turn-1", "session-1", usage, delta)
	if usageEvent.Type != runtimeapi.EventUsageUpdated || usageEvent.TurnID != "turn-1" {
		t.Fatalf("usage event = %#v", usageEvent)
	}
	turnEvent := newTurnFinishedRuntimeEvent(time.Now(), "turn-1", "session-1", "failed", 10*time.Millisecond, "provider", "model", delta, "boom")
	if turnEvent.Type != runtimeapi.EventTurnFailed || turnEvent.Payload["error"] != "boom" {
		t.Fatalf("turn event = %#v", turnEvent)
	}
}

func TestRuntimeSSEServerPublishesRuntimeEvents(t *testing.T) {
	t.Parallel()

	stream := newRuntimeSSEServer()
	if err := stream.Start(); err != nil {
		t.Fatal(err)
	}
	defer stream.Close(context.Background()) //nolint:errcheck

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, stream.URL(), nil)
	if err != nil {
		t.Fatal(err)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("StatusCode = %d", resp.StatusCode)
	}

	stream.Publish(RuntimeEvent{
		ID:        "event-1",
		Type:      "message.created",
		SessionID: "session-1",
		MessageID: "message-1",
		CreatedAt: time.Now().UTC().Format(time.RFC3339Nano),
		Payload: map[string]any{
			"role":    "assistant",
			"summary": "hello",
		},
	})

	scanner := bufio.NewScanner(resp.Body)
	for scanner.Scan() {
		line := scanner.Text()
		if !strings.HasPrefix(line, "data: ") {
			continue
		}
		var event RuntimeEvent
		if err := json.Unmarshal([]byte(strings.TrimPrefix(line, "data: ")), &event); err != nil {
			t.Fatal(err)
		}
		if event.Type != "message.created" || event.MessageID != "message-1" {
			t.Fatalf("event = %#v", event)
		}
		return
	}
	if err := scanner.Err(); err != nil {
		t.Fatal(err)
	}
	t.Fatal("runtime SSE event was not received")
}

type recordingRuntimeService struct {
	chatCalls                  int
	statusCalls                int
	recoveryStatusCalls        int
	skillsCalls                int
	mcpServerCalls             int
	status                     RuntimeStatus
	recoveryStatus             RuntimeRecoveryStatus
	openProjectReq             RuntimeOpenProjectRequest
	createProjectReq           RuntimeCreateProjectRequest
	renameProjectReq           RuntimeRenameProjectRequest
	openProjectInExplorerReq   RuntimeProjectActionRequest
	removeProjectReq           RuntimeProjectActionRequest
	openProject                RuntimeOpenProjectResponse
	createSessionReq           RuntimeSessionCreateRequest
	skills                     RuntimeSkillsResponse
	plugins                    RuntimePluginsResponse
	mcpServers                 RuntimeMCPServersResponse
	mcpRequests                RuntimeMCPRequestsResponse
	mcpRequest                 RuntimeMCPRequestResponse
	mcpRequestDecision         RuntimeMCPRequestDecision
	capabilities               RuntimeCapabilitiesResponse
	contextSources             RuntimeContextSourcesResponse
	refreshedCapability        string
	toolSearchQuery            string
	savedMCPServer             RuntimeMCPServerConfigRequest
	toggledMCPServer           RuntimeMCPServerToggleRequest
	toggledMCPTool             RuntimeMCPToolToggleRequest
	selectedSession            string
	renamedSession             RuntimeSessionUpdateRequest
	deletedSession             string
	messageSession             string
	activitySession            string
	activityWindowSession      string
	activityWindowCursor       string
	activityWindowLimit        int
	activity                   RuntimeSessionActivityResponse
	activityWindow             RuntimeSessionActivityWindowResponse
	turnActivityID             string
	turnActivity               RuntimeTurnActivityResponse
	runProjectionRequest       RuntimeRunProjectionRequest
	runProjection              RuntimeRunProjectionResponse
	transitionHistoryReq       RuntimeRunTransitionHistoryRequest
	transitionHistory          RuntimeRunTransitionHistoryResponse
	runSchedulerPlanReq        RuntimeRunSchedulerPlanRequest
	runSchedulerPlan           RuntimeRunSchedulerPlanResponse
	executeRunID               string
	executeTaskID              string
	executeRunTask             RuntimeRunSchedulerExecuteTaskResponse
	runs                       RuntimeRunsResponse
	runSummaries               RuntimeRunSummariesResponse
	runSummary                 RuntimeRunSummaryResponse
	runSummaryID               string
	runCheckpointMarkers       RuntimeRunCheckpointMarkersResponse
	runCheckpointMarker        RuntimeRunCheckpointMarkerResponse
	runCheckpointMarkersID     string
	runCheckpointMarkerRunID   string
	runCheckpointMarkerID      string
	run                        RuntimeRunResponse
	runID                      string
	ackRunID                   string
	ackCheckpointID            string
	discardRunID               string
	discardCheckpointID        string
	resumeRunID                string
	resumeCheckpointID         string
	resume                     RuntimeRunResumeResponse
	createdSkill               RuntimeSkillCreateRequest
	addedSkillPath             string
	cancelledTurn              string
	markInterruptedDoneTurn    string
	turn                       RuntimeTurnResponse
	turns                      RuntimeTurnsResponse
	turnsStatus                string
	reactCallchain             RuntimeReactCallchainResponse
	reactCallchainTurnID       string
	sessionReactCallchain      RuntimeReactCallchainResponse
	sessionReactCallchainID    string
	sessionReactCallchainLimit int
	toolCall                   RuntimeToolCallResponse
	toolCalls                  RuntimeToolCallsResponse
	hooks                      RuntimeHooksResponse
	hookExecution              RuntimeHookExecutionResponse
	hookExecutions             RuntimeHookExecutionsResponse
	hookExecutionsReq          RuntimeHookExecutionsRequest
	sandboxDecision            RuntimeSandboxDecisionResponse
	sandboxDecisions           RuntimeSandboxDecisionsResponse
	ref                        RuntimeRefResponse
	refs                       RuntimeRefsResponse
	refContent                 RuntimeRefContentResponse
	compactBoundaries          RuntimeCompactBoundariesResponse
	worktrees                  RuntimeWorktreesResponse
	worktree                   RuntimeWorktreeResponse
	worktreeCreate             RuntimeWorktreeCreateRequest
	worktreeAction             RuntimeWorktreeActionRequest
	worktreeActionID           string
	effectiveScope             RuntimeEffectiveScopeResponse
	replayExport               RuntimeReplayExportResponse
	replayExportRequest        RuntimeReplayExportRequest
	agentTask                  RuntimeAgentTaskResponse
	agentTasks                 RuntimeAgentTasksResponse
	agentRoles                 RuntimeAgentRolesResponse
	agentRole                  RuntimeAgentRoleResponse
	agentTaskMessages          RuntimeAgentTaskMessagesResponse
	agentTaskMessage           RuntimeAgentTaskMessageResponse
	agentTaskResult            RuntimeAgentTaskResultResponse
	cancelledTask              string
	todos                      RuntimeTodosResponse
	todoSession                string
	todoTurn                   string
	policy                     RuntimePolicyResponse
	policyCalls                int
	updatedPolicyMode          string
	updatedPolicyRules         []RuntimePolicyRule
	updatedPolicyProfile       string
	permissionDecision         RuntimePermissionDecision
	terminalResponse           RuntimeTerminalResponse
	sessionTerminals           RuntimeSessionTerminalsResponse
	sessionTerminalsID         string
	createdTerminal            RuntimeTerminalCreateRequest
	executedTerminalID         string
	executedTerminalInput      RuntimeTerminalInputRequest
	terminalInputSeen          chan RuntimeTerminalInputRequest
	resizedTerminalID          string
	resizedTerminal            RuntimeTerminalResizeRequest
	terminalResizeSeen         chan RuntimeTerminalResizeRequest
	terminalEvents             chan RuntimeTerminalEvent
	deletedTerminalID          string
}

func (s *recordingRuntimeService) Status(context.Context) (RuntimeStatus, error) {
	s.statusCalls++
	return s.status, nil
}

func (s *recordingRuntimeService) RecoveryStatus(context.Context) (RuntimeRecoveryStatus, error) {
	s.recoveryStatusCalls++
	return s.recoveryStatus, nil
}

func (s *recordingRuntimeService) OpenProject(_ context.Context, req RuntimeOpenProjectRequest) (RuntimeOpenProjectResponse, error) {
	s.openProjectReq = req
	return s.openProject, nil
}

func (s *recordingRuntimeService) CreateProject(_ context.Context, req RuntimeCreateProjectRequest) (RuntimeOpenProjectResponse, error) {
	s.createProjectReq = req
	return s.openProject, nil
}

func (s *recordingRuntimeService) RenameProject(_ context.Context, req RuntimeRenameProjectRequest) (RuntimeOpenProjectResponse, error) {
	s.renameProjectReq = req
	return s.openProject, nil
}

func (s *recordingRuntimeService) OpenProjectInExplorer(_ context.Context, req RuntimeProjectActionRequest) (RuntimeOpenProjectResponse, error) {
	s.openProjectInExplorerReq = req
	return s.openProject, nil
}

func (s *recordingRuntimeService) RemoveProject(_ context.Context, req RuntimeProjectActionRequest) (RuntimeOpenProjectResponse, error) {
	s.removeProjectReq = req
	return s.openProject, nil
}

func (s *recordingRuntimeService) Models(context.Context) (RuntimeModelsResponse, error) {
	return RuntimeModelsResponse{}, nil
}

func (s *recordingRuntimeService) SelectedModel(context.Context) (RuntimeSelectedModelResponse, error) {
	return RuntimeSelectedModelResponse{}, nil
}

func (s *recordingRuntimeService) SaveSelectedModel(context.Context, RuntimeSelectedModelRequest) (RuntimeSelectedModelResponse, error) {
	return RuntimeSelectedModelResponse{}, nil
}

func (s *recordingRuntimeService) ProviderCatalog(context.Context) (RuntimeProviderCatalogResponse, error) {
	return RuntimeProviderCatalogResponse{}, nil
}

func (s *recordingRuntimeService) ConfiguredProviders(context.Context) (RuntimeConfiguredProvidersResponse, error) {
	return RuntimeConfiguredProvidersResponse{}, nil
}

func (s *recordingRuntimeService) SaveConfiguredProvider(context.Context, RuntimeConfiguredProviderRequest) (RuntimeConfiguredProviderResponse, error) {
	return RuntimeConfiguredProviderResponse{}, nil
}

func (s *recordingRuntimeService) DeleteConfiguredProvider(context.Context, string) (RuntimeConfiguredProvidersResponse, error) {
	return RuntimeConfiguredProvidersResponse{}, nil
}

func (s *recordingRuntimeService) DiscoverConfiguredProviderModels(context.Context, string) (RuntimeProviderModelDiscoveryResponse, error) {
	return RuntimeProviderModelDiscoveryResponse{ProviderID: "provider-1", Models: []string{"test-model"}}, nil
}

func (s *recordingRuntimeService) TestConfiguredProvider(context.Context, string) (RuntimeProviderTestResponse, error) {
	return RuntimeProviderTestResponse{OK: true, ProviderID: "provider-1", Model: "test-model"}, nil
}

func (s *recordingRuntimeService) MeasureConfiguredProviderLatency(context.Context, string) (RuntimeProviderTestResponse, error) {
	return RuntimeProviderTestResponse{OK: true, ProviderID: "provider-1", Model: "test-model", DurationMS: 12}, nil
}

func (s *recordingRuntimeService) GetModelConfig(context.Context) (RuntimeConfigResponse, error) {
	return RuntimeConfigResponse{}, nil
}

func (s *recordingRuntimeService) SaveModelConfig(context.Context, RuntimeModelConfig) (RuntimeConfigResponse, error) {
	return RuntimeConfigResponse{}, nil
}

func (s *recordingRuntimeService) DiscoverModelConfig(context.Context, RuntimeModelConfig) (RuntimeModelDiscoveryResponse, error) {
	return RuntimeModelDiscoveryResponse{Protocol: "openai", Models: []string{"test-model"}}, nil
}

func (s *recordingRuntimeService) VerifyModelConfig(context.Context, RuntimeModelConfig) (RuntimeModelVerifyResponse, error) {
	return RuntimeModelVerifyResponse{OK: true, Model: "test-model", Protocol: "openai"}, nil
}

func (s *recordingRuntimeService) CreateTerminal(_ context.Context, req RuntimeTerminalCreateRequest) (RuntimeTerminalResponse, error) {
	s.createdTerminal = req
	return s.terminalResponse, nil
}

func (s *recordingRuntimeService) SessionTerminals(_ context.Context, sessionID string) (RuntimeSessionTerminalsResponse, error) {
	s.sessionTerminalsID = sessionID
	if s.sessionTerminals.SessionID == "" {
		s.sessionTerminals.SessionID = sessionID
	}
	return s.sessionTerminals, nil
}

func (s *recordingRuntimeService) WriteTerminalInput(_ context.Context, terminalID string, req RuntimeTerminalInputRequest) (RuntimeTerminalResponse, error) {
	s.executedTerminalID = terminalID
	s.executedTerminalInput = req
	if s.terminalInputSeen != nil {
		s.terminalInputSeen <- req
	}
	return s.terminalResponse, nil
}

func (s *recordingRuntimeService) ResizeTerminal(_ context.Context, terminalID string, req RuntimeTerminalResizeRequest) (RuntimeTerminalResponse, error) {
	s.resizedTerminalID = terminalID
	s.resizedTerminal = req
	if s.terminalResizeSeen != nil {
		s.terminalResizeSeen <- req
	}
	return s.terminalResponse, nil
}

func (s *recordingRuntimeService) SubscribeTerminalEvents(context.Context, string, ...int64) (<-chan RuntimeTerminalEvent, func()) {
	if s.terminalEvents != nil {
		return s.terminalEvents, func() {}
	}
	ch := make(chan RuntimeTerminalEvent)
	close(ch)
	return ch, func() {}
}

func (s *recordingRuntimeService) DeleteTerminal(_ context.Context, terminalID string) (RuntimeTerminalResponse, error) {
	s.deletedTerminalID = terminalID
	return s.terminalResponse, nil
}

func (s *recordingRuntimeService) Chat(context.Context, RuntimeChatRequest) (RuntimeChatResponse, error) {
	s.chatCalls++
	return RuntimeChatResponse{RequestID: "request-1", TurnID: "request-1", Status: s.status}, nil
}

func (s *recordingRuntimeService) Turn(_ context.Context, turnID string) (RuntimeTurnResponse, error) {
	if s.turn.Turn.ID == "" {
		s.turn.Turn.ID = turnID
	}
	return s.turn, nil
}

func (s *recordingRuntimeService) Turns(_ context.Context, status string) (RuntimeTurnsResponse, error) {
	s.turnsStatus = status
	return s.turns, nil
}

func (s *recordingRuntimeService) ReactCallchain(_ context.Context, turnID string) (RuntimeReactCallchainResponse, error) {
	s.reactCallchainTurnID = turnID
	if s.reactCallchain.TurnID == "" {
		s.reactCallchain.TurnID = turnID
	}
	return s.reactCallchain, nil
}

func (s *recordingRuntimeService) SessionReactCallchain(_ context.Context, sessionID string, limit int) (RuntimeReactCallchainResponse, error) {
	s.sessionReactCallchainID = sessionID
	s.sessionReactCallchainLimit = limit
	if s.sessionReactCallchain.SessionID == "" {
		s.sessionReactCallchain.SessionID = sessionID
	}
	return s.sessionReactCallchain, nil
}

func (s *recordingRuntimeService) Runs(context.Context) (RuntimeRunsResponse, error) {
	return s.runs, nil
}

func (s *recordingRuntimeService) RunSummaries(context.Context) (RuntimeRunSummariesResponse, error) {
	return s.runSummaries, nil
}

func (s *recordingRuntimeService) RunSummary(_ context.Context, id string) (RuntimeRunSummaryResponse, error) {
	s.runSummaryID = id
	return s.runSummary, nil
}

func (s *recordingRuntimeService) RunCheckpointMarkers(_ context.Context, runID string) (RuntimeRunCheckpointMarkersResponse, error) {
	s.runCheckpointMarkersID = runID
	return s.runCheckpointMarkers, nil
}

func (s *recordingRuntimeService) RunCheckpointMarker(_ context.Context, runID, checkpointID string) (RuntimeRunCheckpointMarkerResponse, error) {
	s.runCheckpointMarkerRunID = runID
	s.runCheckpointMarkerID = checkpointID
	return s.runCheckpointMarker, nil
}

func (s *recordingRuntimeService) Run(_ context.Context, id string) (RuntimeRunResponse, error) {
	s.runID = id
	return s.run, nil
}

func (s *recordingRuntimeService) AcknowledgeRunCheckpoint(_ context.Context, runID, checkpointID string) (RuntimeRunResponse, error) {
	s.ackRunID = runID
	s.ackCheckpointID = checkpointID
	return withRuntimeRunCheckpointAction(s.run, runtimeRunCheckpointActionAcknowledge, runtimeRunCheckpointActionReasonAcknowledged), nil
}

func (s *recordingRuntimeService) DiscardRunCheckpoint(_ context.Context, runID, checkpointID string) (RuntimeRunResponse, error) {
	s.discardRunID = runID
	s.discardCheckpointID = checkpointID
	return withRuntimeRunCheckpointAction(s.run, runtimeRunCheckpointActionDiscard, runtimeRunCheckpointActionReasonDiscarded), nil
}

func (s *recordingRuntimeService) ResumeRunCheckpoint(_ context.Context, runID, checkpointID string) (RuntimeRunResumeResponse, error) {
	s.resumeRunID = runID
	s.resumeCheckpointID = checkpointID
	if s.resume.RunID == "" {
		s.resume.RunID = runID
		s.resume.CheckpointID = checkpointID
		s.resume.TurnID = "turn-resume"
	}
	return withRuntimeRunCheckpointResumeAction(s.resume), nil
}

func (s *recordingRuntimeService) ToolCall(context.Context, string) (RuntimeToolCallResponse, error) {
	return s.toolCall, nil
}

func (s *recordingRuntimeService) TurnToolCalls(context.Context, string) (RuntimeToolCallsResponse, error) {
	return s.toolCalls, nil
}

func (s *recordingRuntimeService) Hooks(context.Context) (RuntimeHooksResponse, error) {
	return s.hooks, nil
}

func (s *recordingRuntimeService) HookExecutions(_ context.Context, req RuntimeHookExecutionsRequest) (RuntimeHookExecutionsResponse, error) {
	s.hookExecutionsReq = req
	return s.hookExecutions, nil
}

func (s *recordingRuntimeService) HookExecution(context.Context, string) (RuntimeHookExecutionResponse, error) {
	return s.hookExecution, nil
}

func (s *recordingRuntimeService) SandboxDecision(context.Context, string) (RuntimeSandboxDecisionResponse, error) {
	return s.sandboxDecision, nil
}

func (s *recordingRuntimeService) SandboxDecisions(context.Context, RuntimeSandboxDecisionListRequest) (RuntimeSandboxDecisionsResponse, error) {
	return s.sandboxDecisions, nil
}

func (s *recordingRuntimeService) Refs(context.Context, RuntimeRefListRequest) (RuntimeRefsResponse, error) {
	return s.refs, nil
}

func (s *recordingRuntimeService) Ref(context.Context, string) (RuntimeRefResponse, error) {
	return s.ref, nil
}

func (s *recordingRuntimeService) ReadRefContent(context.Context, string) (RuntimeRefContentResponse, error) {
	return s.refContent, nil
}

func (s *recordingRuntimeService) TurnCompactBoundaries(context.Context, string) (RuntimeCompactBoundariesResponse, error) {
	return s.compactBoundaries, nil
}

func (s *recordingRuntimeService) SessionCompactBoundaries(context.Context, string) (RuntimeCompactBoundariesResponse, error) {
	return s.compactBoundaries, nil
}

func (s *recordingRuntimeService) Worktrees(context.Context) (RuntimeWorktreesResponse, error) {
	return s.worktrees, nil
}

func (s *recordingRuntimeService) Worktree(context.Context, string) (RuntimeWorktreeResponse, error) {
	return s.worktree, nil
}

func (s *recordingRuntimeService) CreateWorktree(_ context.Context, req RuntimeWorktreeCreateRequest) (RuntimeWorktreeResponse, error) {
	s.worktreeCreate = req
	return s.worktree, nil
}

func (s *recordingRuntimeService) EnterWorktree(_ context.Context, id string, req RuntimeWorktreeActionRequest) (RuntimeWorktreeResponse, error) {
	s.worktreeActionID = id
	s.worktreeAction = req
	return s.worktree, nil
}

func (s *recordingRuntimeService) ExitWorktree(_ context.Context, id string, req RuntimeWorktreeActionRequest) (RuntimeWorktreeResponse, error) {
	s.worktreeActionID = id
	s.worktreeAction = req
	return s.worktree, nil
}

func (s *recordingRuntimeService) CleanupWorktree(_ context.Context, id string, req RuntimeWorktreeActionRequest) (RuntimeWorktreeResponse, error) {
	s.worktreeActionID = id
	s.worktreeAction = req
	return s.worktree, nil
}

func (s *recordingRuntimeService) AgentTask(context.Context, string) (RuntimeAgentTaskResponse, error) {
	return s.agentTask, nil
}

func (s *recordingRuntimeService) TaskEffectiveScope(context.Context, string) (RuntimeEffectiveScopeResponse, error) {
	return s.effectiveScope, nil
}

func (s *recordingRuntimeService) TurnAgentTasks(context.Context, string) (RuntimeAgentTasksResponse, error) {
	return s.agentTasks, nil
}

func (s *recordingRuntimeService) CancelAgentTask(_ context.Context, taskID string) (RuntimeAgentTaskResponse, error) {
	s.cancelledTask = taskID
	if s.agentTask.Task.ID == "" {
		s.agentTask.Task = RuntimeAgentTask{ID: taskID, Status: agentTaskStatusCancelled}
	}
	return withRuntimeAgentTaskCancelAction(s.agentTask, true, "test cancellation requested"), nil
}

func (s *recordingRuntimeService) AgentRoles(context.Context) (RuntimeAgentRolesResponse, error) {
	return s.agentRoles, nil
}

func (s *recordingRuntimeService) AgentRole(context.Context, string) (RuntimeAgentRoleResponse, error) {
	return s.agentRole, nil
}

func (s *recordingRuntimeService) AgentTaskMessages(context.Context, string) (RuntimeAgentTaskMessagesResponse, error) {
	return s.agentTaskMessages, nil
}

func (s *recordingRuntimeService) CreateAgentTaskMessage(context.Context, string, RuntimeAgentTaskMessageCreateRequest) (RuntimeAgentTaskMessageResponse, error) {
	return s.agentTaskMessage, nil
}

func (s *recordingRuntimeService) SendAgentTaskFollowUp(context.Context, string, RuntimeAgentTaskMessageCreateRequest) (RuntimeAgentTaskMessageResponse, error) {
	return s.agentTaskMessage, nil
}

func (s *recordingRuntimeService) AgentTaskResult(context.Context, string) (RuntimeAgentTaskResultResponse, error) {
	return s.agentTaskResult, nil
}

func (s *recordingRuntimeService) SessionTodos(_ context.Context, sessionID string) (RuntimeTodosResponse, error) {
	s.todoSession = sessionID
	return s.todos, nil
}

func (s *recordingRuntimeService) TurnTodos(_ context.Context, turnID string) (RuntimeTodosResponse, error) {
	s.todoTurn = turnID
	return s.todos, nil
}

func (s *recordingRuntimeService) Sessions(context.Context) (RuntimeSessionsResponse, error) {
	return RuntimeSessionsResponse{Sessions: []RuntimeSession{
		{ID: "session-1", Title: "Test chat", Active: s.status.SessionID == "session-1"},
		{ID: "session-2", Title: "Other chat", Active: s.status.SessionID == "session-2"},
	}}, nil
}

func (s *recordingRuntimeService) Session(context.Context, string) (RuntimeSessionResponse, error) {
	return RuntimeSessionResponse{Session: RuntimeSession{ID: s.status.SessionID, Title: "Test chat", Active: true}}, nil
}

func (s *recordingRuntimeService) CreateSession(_ context.Context, req RuntimeSessionCreateRequest) (RuntimeSessionResponse, error) {
	s.createSessionReq = req
	sessionID := firstNonEmpty(s.status.SessionID, "session-created")
	return RuntimeSessionResponse{Session: RuntimeSession{ID: sessionID, Title: firstNonEmpty(req.Title, "New chat"), Active: true}}, nil
}

func (s *recordingRuntimeService) SelectSession(_ context.Context, sessionID string) (RuntimeStatus, error) {
	s.selectedSession = sessionID
	return s.status, nil
}

func (s *recordingRuntimeService) RenameSession(_ context.Context, req RuntimeSessionUpdateRequest) (RuntimeSessionsResponse, error) {
	s.renamedSession = req
	return RuntimeSessionsResponse{}, nil
}

func (s *recordingRuntimeService) DeleteSession(_ context.Context, sessionID string) (RuntimeSessionsResponse, error) {
	s.deletedSession = sessionID
	return RuntimeSessionsResponse{}, nil
}

func (s *recordingRuntimeService) SessionMessages(_ context.Context, sessionID string) (RuntimeMessagesResponse, error) {
	s.messageSession = sessionID
	return RuntimeMessagesResponse{}, nil
}

func (s *recordingRuntimeService) SessionActivity(_ context.Context, sessionID string) (RuntimeSessionActivityResponse, error) {
	s.activitySession = sessionID
	if s.activity.SessionID == "" {
		s.activity.SessionID = sessionID
	}
	return s.activity, nil
}

func (s *recordingRuntimeService) SessionActivityWindow(_ context.Context, sessionID string, limit int) (RuntimeSessionActivityWindowResponse, error) {
	return s.SessionActivityCursorWindow(context.Background(), sessionID, "", limit)
}

func (s *recordingRuntimeService) SessionActivityCursorWindow(_ context.Context, sessionID string, cursor string, limit int) (RuntimeSessionActivityWindowResponse, error) {
	s.activityWindowSession = sessionID
	s.activityWindowCursor = cursor
	s.activityWindowLimit = limit
	if s.activityWindow.SessionID == "" {
		s.activityWindow.SessionID = sessionID
	}
	return s.activityWindow, nil
}

func (s *recordingRuntimeService) TurnActivity(_ context.Context, turnID string) (RuntimeTurnActivityResponse, error) {
	s.turnActivityID = turnID
	if s.turnActivity.TurnID == "" {
		s.turnActivity.TurnID = turnID
	}
	return s.turnActivity, nil
}

func (s *recordingRuntimeService) RunProjection(_ context.Context, req RuntimeRunProjectionRequest) (RuntimeRunProjectionResponse, error) {
	s.runProjectionRequest = req
	if s.runProjection.Run.PrimarySessionID == "" {
		s.runProjection.Run.PrimarySessionID = req.SessionID
	}
	return s.runProjection, nil
}

func (s *recordingRuntimeService) RunTransitionHistory(_ context.Context, req RuntimeRunTransitionHistoryRequest) (RuntimeRunTransitionHistoryResponse, error) {
	s.transitionHistoryReq = req
	return s.transitionHistory, nil
}

func (s *recordingRuntimeService) RunSchedulerPlan(_ context.Context, req RuntimeRunSchedulerPlanRequest) (RuntimeRunSchedulerPlanResponse, error) {
	s.runSchedulerPlanReq = req
	return s.runSchedulerPlan, nil
}

func (s *recordingRuntimeService) ExecuteRunTask(_ context.Context, runID, taskID string) (RuntimeRunSchedulerExecuteTaskResponse, error) {
	s.executeRunID = runID
	s.executeTaskID = taskID
	if s.executeRunTask.Source.Kind == "" {
		s.executeRunTask.Source = RuntimeRunSchedulerExecuteTaskSource{
			Kind:                  runtimeRunSchedulerExecuteTaskSourceKind,
			Action:                runtimeRunSchedulerExecuteTaskAction,
			BackendOnly:           true,
			StartsWorker:          false,
			IdempotentByTaskID:    true,
			SessionActivityParity: true,
		}
	}
	return withRuntimeRunSchedulerExecuteTaskAction(s.executeRunTask), nil
}

func (s *recordingRuntimeService) Messages(context.Context) (RuntimeMessagesResponse, error) {
	return RuntimeMessagesResponse{}, nil
}

func (s *recordingRuntimeService) Permissions(context.Context) (RuntimePermissionsResponse, error) {
	return RuntimePermissionsResponse{}, nil
}

func (s *recordingRuntimeService) GetPolicy(context.Context) (RuntimePolicyResponse, error) {
	s.policyCalls++
	return s.policy, nil
}

func (s *recordingRuntimeService) UpdatePolicy(_ context.Context, req RuntimePolicyUpdateRequest) (RuntimePolicyResponse, error) {
	s.updatedPolicyMode = req.Mode
	s.updatedPolicyRules = req.Rules
	s.updatedPolicyProfile = req.Profile
	return RuntimePolicyResponse{Policy: RuntimePolicy{Mode: req.Mode, Rules: req.Rules, Profile: req.Profile}}, nil
}

func (s *recordingRuntimeService) Events(context.Context, ...int64) (RuntimeEventsResponse, error) {
	return RuntimeEventsResponse{}, nil
}

func (s *recordingRuntimeService) EventsEndpoint(context.Context) (RuntimeEventsEndpointResponse, error) {
	return RuntimeEventsEndpointResponse{}, nil
}

func (s *recordingRuntimeService) SubscribeEvents(context.Context, ...int64) (<-chan RuntimeEvent, func()) {
	events := make(chan RuntimeEvent)
	return events, func() {
		close(events)
	}
}

func (s *recordingRuntimeService) AuditTurn(context.Context, string) (RuntimeAuditResponse, error) {
	return RuntimeAuditResponse{}, nil
}

func (s *recordingRuntimeService) AuditSession(context.Context, string) (RuntimeAuditResponse, error) {
	return RuntimeAuditResponse{}, nil
}

func (s *recordingRuntimeService) ReplayExport(_ context.Context, req RuntimeReplayExportRequest) (RuntimeReplayExportResponse, error) {
	s.replayExportRequest = req
	return s.replayExport, nil
}

func (s *recordingRuntimeService) Skills(context.Context) (RuntimeSkillsResponse, error) {
	s.skillsCalls++
	return s.skills, nil
}

func (s *recordingRuntimeService) Plugins(context.Context) (RuntimePluginsResponse, error) {
	return s.plugins, nil
}

func (s *recordingRuntimeService) RefreshSkills(context.Context) (RuntimeSkillsResponse, error) {
	return RuntimeSkillsResponse{}, nil
}

func (s *recordingRuntimeService) CreateSkill(_ context.Context, req RuntimeSkillCreateRequest) (RuntimeSkillsResponse, error) {
	s.createdSkill = req
	return RuntimeSkillsResponse{}, nil
}

func (s *recordingRuntimeService) AddSkillPath(_ context.Context, req RuntimeSkillPathRequest) (RuntimeSkillsResponse, error) {
	s.addedSkillPath = req.Path
	return RuntimeSkillsResponse{}, nil
}

func (s *recordingRuntimeService) SetSkillEnabled(context.Context, RuntimeSkillToggleRequest) (RuntimeSkillsResponse, error) {
	return RuntimeSkillsResponse{}, nil
}

func (s *recordingRuntimeService) MCPServers(context.Context) (RuntimeMCPServersResponse, error) {
	s.mcpServerCalls++
	return s.mcpServers, nil
}

func (s *recordingRuntimeService) SaveMCPServer(_ context.Context, req RuntimeMCPServerConfigRequest) (RuntimeMCPServersResponse, error) {
	s.savedMCPServer = req
	return RuntimeMCPServersResponse{Servers: []RuntimeMCPServer{{Name: req.Name, Type: req.Type, URL: redactURL(req.URL), Headers: redactMap(req.Headers), Env: redactMap(req.Env)}}}, nil
}

func (s *recordingRuntimeService) SetMCPServerEnabled(_ context.Context, req RuntimeMCPServerToggleRequest) (RuntimeMCPServersResponse, error) {
	s.toggledMCPServer = req
	return RuntimeMCPServersResponse{}, nil
}

func (s *recordingRuntimeService) RefreshMCPServer(context.Context, string) (RuntimeMCPServersResponse, error) {
	return RuntimeMCPServersResponse{}, nil
}

func (s *recordingRuntimeService) SetMCPToolEnabled(_ context.Context, req RuntimeMCPToolToggleRequest) (RuntimeMCPToolsResponse, error) {
	s.toggledMCPTool = req
	return RuntimeMCPToolsResponse{}, nil
}

func (s *recordingRuntimeService) MCPTools(context.Context, string) (RuntimeMCPToolsResponse, error) {
	return RuntimeMCPToolsResponse{}, nil
}

func (s *recordingRuntimeService) MCPResources(context.Context, string) (RuntimeMCPResourcesResponse, error) {
	return RuntimeMCPResourcesResponse{}, nil
}

func (s *recordingRuntimeService) MCPPrompts(context.Context, string) (RuntimeMCPPromptsResponse, error) {
	return RuntimeMCPPromptsResponse{}, nil
}

func (s *recordingRuntimeService) MCPRequests(context.Context, RuntimeMCPRequestListRequest) (RuntimeMCPRequestsResponse, error) {
	return s.mcpRequests, nil
}

func (s *recordingRuntimeService) MCPRequest(context.Context, string) (RuntimeMCPRequestResponse, error) {
	return s.mcpRequest, nil
}

func (s *recordingRuntimeService) DecideMCPRequest(_ context.Context, req RuntimeMCPRequestDecision) (RuntimeMCPRequestResponse, error) {
	s.mcpRequestDecision = req
	return withRuntimeMCPRequestDecisionAction(s.mcpRequest, true, req.Action, runtimeMCPRequestDecisionReasonAccepted), nil
}

func (s *recordingRuntimeService) RetryMCPServer(context.Context, string) (RuntimeMCPServersResponse, error) {
	return s.mcpServers, nil
}

func (s *recordingRuntimeService) Capabilities(context.Context) (RuntimeCapabilitiesResponse, error) {
	return s.capabilities, nil
}

func (s *recordingRuntimeService) RefreshCapability(_ context.Context, id string) (RuntimeCapabilityResponse, error) {
	s.refreshedCapability = id
	return RuntimeCapabilityResponse{Capability: RuntimeCapability{ID: id, Kind: "skill", Name: "crush-config", Enabled: true, State: "loaded"}}, nil
}

func (s *recordingRuntimeService) SearchTools(_ context.Context, req RuntimeToolSearchRequest) (RuntimeToolSearchResponse, error) {
	s.toolSearchQuery = req.Query
	return RuntimeToolSearchResponse{Query: req.Query}, nil
}

func (s *recordingRuntimeService) ContextSources(context.Context) (RuntimeContextSourcesResponse, error) {
	return s.contextSources, nil
}

func (s *recordingRuntimeService) ReadFiles(context.Context, string) (RuntimeReadFilesResponse, error) {
	return RuntimeReadFilesResponse{}, nil
}

func (s *recordingRuntimeService) APIEndpoint(context.Context) (RuntimeAPIEndpointResponse, error) {
	return RuntimeAPIEndpointResponse{URL: "http://127.0.0.1:1", Token: "token"}, nil
}

func (s *recordingRuntimeService) ServeHTTP(context.Context, string, string) (RuntimeAPIEndpointResponse, error) {
	return RuntimeAPIEndpointResponse{URL: "http://127.0.0.1:1", Token: "token"}, nil
}

func (s *recordingRuntimeService) DecidePermission(_ context.Context, req RuntimePermissionDecision) (RuntimeStatus, error) {
	s.permissionDecision = req
	return withRuntimePermissionDecisionAction(s.status, proto.PermissionAction(req.Action)), nil
}

func (s *recordingRuntimeService) Cancel(context.Context) (RuntimeStatus, error) {
	return RuntimeStatus{}, nil
}

func (s *recordingRuntimeService) CancelTurn(_ context.Context, turnID string) (RuntimeStatus, error) {
	s.cancelledTurn = turnID
	return withRuntimeTurnAction(s.status, runtimeTurnActionCancel, runtimeTurnActionReasonCancelled), nil
}

func (s *recordingRuntimeService) MarkInterruptedDone(_ context.Context, turnID string) (RuntimeTurnResponse, error) {
	s.markInterruptedDoneTurn = turnID
	return withRuntimeTurnResponseAction(s.turn, runtimeTurnActionMarkInterruptedDone, runtimeTurnActionReasonInterruptedMarkedDone), nil
}

func (s *recordingRuntimeService) NewChat(context.Context, string) (RuntimeStatus, error) {
	return s.status, nil
}
