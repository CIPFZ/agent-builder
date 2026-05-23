package runtime

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/charmbracelet/crush/internal/backend"
	"github.com/charmbracelet/crush/internal/config"
	"github.com/charmbracelet/crush/internal/csync"
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
	if found.State != "normal" {
		t.Fatalf("State = %q, want normal", found.State)
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
	if byID["skill:crush-config"].Enabled {
		t.Fatalf("disabled skill capability = %#v", byID["skill:crush-config"])
	}
	if byID["mcp:docs:search_docs"].Kind != "mcp_tool" {
		t.Fatalf("mcp tool capability missing: %#v", byID["mcp:docs:search_docs"])
	}
	if byID["mcp_resource:docs:docs://intro"].Kind != "mcp_resource" {
		t.Fatalf("mcp resource capability missing: %#v", byID["mcp_resource:docs:docs://intro"])
	}
	if byID["mcp_prompt:docs:summarize"].Kind != "mcp_prompt" {
		t.Fatalf("mcp prompt capability missing: %#v", byID["mcp_prompt:docs:summarize"])
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
		if skill.State == "error" && strings.Contains(skill.Path, "SKILL.md") && skill.Error != "" {
			return
		}
	}
	t.Fatalf("invalid skill diagnostic missing from %#v", resp.Skills)
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
	dataDir := filepath.Join(workingDir, ".crush")
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
			"finished": {StartedAt: now - 2000, Finished: true},
			"running":  {StartedAt: now - 1000},
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
	chatCalls           int
	statusCalls         int
	recoveryStatusCalls int
	skillsCalls         int
	mcpServerCalls      int
	status              RuntimeStatus
	recoveryStatus      RuntimeRecoveryStatus
	skills              RuntimeSkillsResponse
	mcpServers          RuntimeMCPServersResponse
	capabilities        RuntimeCapabilitiesResponse
	savedMCPServer      RuntimeMCPServerConfigRequest
	toggledMCPServer    RuntimeMCPServerToggleRequest
	toggledMCPTool      RuntimeMCPToolToggleRequest
	selectedSession     string
	renamedSession      RuntimeSessionUpdateRequest
	deletedSession      string
	messageSession      string
	createdSkill        RuntimeSkillCreateRequest
	addedSkillPath      string
	cancelledTurn       string
	turn                RuntimeTurnResponse
	turns               RuntimeTurnsResponse
	turnsStatus         string
	toolCall            RuntimeToolCallResponse
	toolCalls           RuntimeToolCallsResponse
}

func (s *recordingRuntimeService) Status(context.Context) (RuntimeStatus, error) {
	s.statusCalls++
	return s.status, nil
}

func (s *recordingRuntimeService) RecoveryStatus(context.Context) (RuntimeRecoveryStatus, error) {
	s.recoveryStatusCalls++
	return s.recoveryStatus, nil
}

func (s *recordingRuntimeService) Models(context.Context) (RuntimeModelsResponse, error) {
	return RuntimeModelsResponse{}, nil
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

func (s *recordingRuntimeService) Chat(context.Context, RuntimeChatRequest) (RuntimeChatResponse, error) {
	s.chatCalls++
	return RuntimeChatResponse{RequestID: "request-1", TurnID: "request-1", Status: s.status}, nil
}

func (s *recordingRuntimeService) Turn(context.Context, string) (RuntimeTurnResponse, error) {
	return s.turn, nil
}

func (s *recordingRuntimeService) Turns(_ context.Context, status string) (RuntimeTurnsResponse, error) {
	s.turnsStatus = status
	return s.turns, nil
}

func (s *recordingRuntimeService) ToolCall(context.Context, string) (RuntimeToolCallResponse, error) {
	return s.toolCall, nil
}

func (s *recordingRuntimeService) TurnToolCalls(context.Context, string) (RuntimeToolCallsResponse, error) {
	return s.toolCalls, nil
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

func (s *recordingRuntimeService) Messages(context.Context) (RuntimeMessagesResponse, error) {
	return RuntimeMessagesResponse{}, nil
}

func (s *recordingRuntimeService) Permissions(context.Context) (RuntimePermissionsResponse, error) {
	return RuntimePermissionsResponse{}, nil
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

func (s *recordingRuntimeService) Skills(context.Context) (RuntimeSkillsResponse, error) {
	s.skillsCalls++
	return s.skills, nil
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

func (s *recordingRuntimeService) Capabilities(context.Context) (RuntimeCapabilitiesResponse, error) {
	return s.capabilities, nil
}

func (s *recordingRuntimeService) APIEndpoint(context.Context) (RuntimeAPIEndpointResponse, error) {
	return RuntimeAPIEndpointResponse{URL: "http://127.0.0.1:1", Token: "token"}, nil
}

func (s *recordingRuntimeService) DecidePermission(context.Context, RuntimePermissionDecision) (RuntimeStatus, error) {
	return RuntimeStatus{}, nil
}

func (s *recordingRuntimeService) Cancel(context.Context) (RuntimeStatus, error) {
	return RuntimeStatus{}, nil
}

func (s *recordingRuntimeService) CancelTurn(_ context.Context, turnID string) (RuntimeStatus, error) {
	s.cancelledTurn = turnID
	return RuntimeStatus{}, nil
}

func (s *recordingRuntimeService) NewChat(context.Context, string) (RuntimeStatus, error) {
	return s.status, nil
}
