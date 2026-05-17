package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"charm.land/catwalk/pkg/catwalk"
	"github.com/charmbracelet/crush/internal/backend"
	"github.com/charmbracelet/crush/internal/config"
	crushlog "github.com/charmbracelet/crush/internal/log"
	"github.com/charmbracelet/crush/internal/message"
	"github.com/charmbracelet/crush/internal/proto"
	"github.com/charmbracelet/crush/internal/version"
)

// RuntimeBridge exposes a small desktop-facing API over the real Crush
// runtime. The React UI stays thin; this service owns workspace, session, and
// agent lifecycle.
type RuntimeBridge struct {
	mu        sync.Mutex
	runtime   *backend.Backend
	workspace *proto.Workspace
	sessionID string
	cancel    context.CancelFunc
}

type RuntimeStatus struct {
	Ready       bool   `json:"ready"`
	WorkspaceID string `json:"workspaceId"`
	SessionID   string `json:"sessionId"`
	WorkingDir  string `json:"workingDir"`
	Model       string `json:"model"`
	Provider    string `json:"provider"`
	Busy        bool   `json:"busy"`
}

type RuntimeModel struct {
	ID       string `json:"id"`
	Name     string `json:"name"`
	Provider string `json:"provider"`
	Selected bool   `json:"selected"`
}

type RuntimeModelsResponse struct {
	Models []RuntimeModel `json:"models"`
}

type RuntimeConfigResponse struct {
	Config RuntimeModelConfig `json:"config"`
}

type RuntimeChatRequest struct {
	Prompt string `json:"prompt"`
}

type RuntimeChatResponse struct {
	Message RuntimeMessage `json:"message"`
	Status  RuntimeStatus  `json:"status"`
}

type RuntimeMessage struct {
	ID        string `json:"id"`
	SessionID string `json:"sessionId"`
	Role      string `json:"role"`
	Content   string `json:"content"`
	Provider  string `json:"provider,omitempty"`
	Model     string `json:"model,omitempty"`
	CreatedAt int64  `json:"createdAt"`
	UpdatedAt int64  `json:"updatedAt"`
	Finished  bool   `json:"finished"`
	Error     string `json:"error,omitempty"`
}

type RuntimeMessagesResponse struct {
	Messages []RuntimeMessage `json:"messages"`
}

type RuntimeModelConfig struct {
	Protocol   string   `json:"protocol"`
	URL        string   `json:"url"`
	APIKey     string   `json:"apiKey,omitempty"`
	Model      string   `json:"model"`
	Proxy      string   `json:"proxy,omitempty"`
	Models     []string `json:"models,omitempty"`
	HasAPIKey  bool     `json:"hasApiKey"`
	ConfigPath string   `json:"configPath,omitempty"`
}

type localModelConfigResult struct {
	Applied      bool
	Path         string
	CheckedPaths []string
	Error        error
}

type desktopLayout struct {
	Root            string
	ConfigDir       string
	DataDir         string
	LogsDir         string
	ModelConfigPath string
}

const localProviderID = "local-model"
const chatTimeout = 2 * time.Minute

var errModelConfigMissing = errors.New("model is not configured. Open model settings and save protocol, URL, API key, and model before chatting.")

func NewRuntimeBridge() *RuntimeBridge {
	return &RuntimeBridge{}
}

func (r *RuntimeBridge) Status(ctx context.Context) (RuntimeStatus, error) {
	if err := r.ensureStarted(ctx); err != nil {
		return RuntimeStatus{}, err
	}

	r.mu.Lock()
	ws := *r.workspace
	sessionID := r.sessionID
	r.mu.Unlock()

	info, err := r.runtime.GetAgentInfo(ws.ID)
	if err != nil {
		return RuntimeStatus{}, err
	}

	return RuntimeStatus{
		Ready:       info.IsReady,
		WorkspaceID: ws.ID,
		SessionID:   sessionID,
		WorkingDir:  ws.Path,
		Model:       info.ModelCfg.Model,
		Provider:    info.ModelCfg.Provider,
		Busy:        info.IsBusy,
	}, nil
}

func (r *RuntimeBridge) Models(ctx context.Context) (RuntimeModelsResponse, error) {
	if err := r.ensureStarted(ctx); err != nil {
		if errors.Is(err, errModelConfigMissing) {
			cfg, cfgErr := r.readConfiguredModel()
			if cfgErr != nil {
				return RuntimeModelsResponse{}, err
			}
			return RuntimeModelsResponse{Models: []RuntimeModel{{
				ID:       cfg.Model,
				Name:     cfg.Model,
				Provider: localProviderID,
				Selected: true,
			}}}, nil
		}
		return RuntimeModelsResponse{}, err
	}

	r.mu.Lock()
	ws := *r.workspace
	r.mu.Unlock()

	workspace, err := r.runtime.GetWorkspace(ws.ID)
	if err != nil {
		return RuntimeModelsResponse{}, err
	}

	selected := workspace.Cfg.Config().Models[config.SelectedModelTypeLarge]
	models := make([]RuntimeModel, 0)
	for _, provider := range workspace.Cfg.Config().EnabledProviders() {
		for _, model := range provider.Models {
			models = append(models, RuntimeModel{
				ID:       model.ID,
				Name:     firstNonEmpty(model.Name, model.ID),
				Provider: provider.ID,
				Selected: selected.Provider == provider.ID && selected.Model == model.ID,
			})
		}
	}

	return RuntimeModelsResponse{Models: models}, nil
}

func (r *RuntimeBridge) GetModelConfig(ctx context.Context) (RuntimeConfigResponse, error) {
	layout, err := resolveDesktopLayout()
	if err != nil {
		return RuntimeConfigResponse{}, err
	}
	if err := ensureDesktopLayout(layout); err != nil {
		return RuntimeConfigResponse{}, err
	}

	local, result := loadLocalModelConfig(layout)
	if result.Error != nil {
		return RuntimeConfigResponse{}, result.Error
	}
	if !result.Applied {
		local = RuntimeModelConfig{
			Protocol: "openai",
		}
	}
	hasAPIKey := result.Applied && local.APIKey != ""
	local.APIKey = ""
	local.HasAPIKey = hasAPIKey
	local.ConfigPath = layout.ModelConfigPath

	return RuntimeConfigResponse{Config: local}, nil
}

func (r *RuntimeBridge) SaveModelConfig(ctx context.Context, req RuntimeModelConfig) (RuntimeConfigResponse, error) {
	layout, err := resolveDesktopLayout()
	if err != nil {
		return RuntimeConfigResponse{}, err
	}

	current, result := loadLocalModelConfig(layout)
	if result.Error != nil {
		return RuntimeConfigResponse{}, result.Error
	}

	next := RuntimeModelConfig{
		Protocol: strings.TrimSpace(req.Protocol),
		URL:      strings.TrimSpace(req.URL),
		APIKey:   strings.TrimSpace(req.APIKey),
		Model:    strings.TrimSpace(req.Model),
		Proxy:    strings.TrimSpace(req.Proxy),
	}
	if next.APIKey == "" {
		next.APIKey = current.APIKey
	}
	if next.Model != "" {
		next.Models = []string{next.Model}
	}
	if err := validateModelConfig(next); err != nil {
		return RuntimeConfigResponse{}, err
	}

	if err := saveLocalModelConfig(layout, next); err != nil {
		return RuntimeConfigResponse{}, err
	}

	r.restart()

	next.APIKey = ""
	next.HasAPIKey = true
	next.ConfigPath = layout.ModelConfigPath
	return RuntimeConfigResponse{Config: next}, nil
}

func (r *RuntimeBridge) Chat(ctx context.Context, req RuntimeChatRequest) (RuntimeChatResponse, error) {
	prompt := strings.TrimSpace(req.Prompt)
	if prompt == "" {
		return RuntimeChatResponse{}, errors.New("prompt is required")
	}
	if err := r.ensureStarted(ctx); err != nil {
		return RuntimeChatResponse{}, err
	}

	r.mu.Lock()
	wsID := r.workspace.ID
	sessionID := r.sessionID
	r.mu.Unlock()

	chatCtx, cancel := context.WithTimeout(ctx, chatTimeout)
	defer cancel()

	start := time.Now()
	slog.Info("Desktop chat started", "workspace_id", wsID, "session_id", sessionID, "prompt_len", len(prompt))
	if err := r.runtime.SendMessage(chatCtx, wsID, proto.AgentMessage{
		SessionID: sessionID,
		Prompt:    prompt,
	}); err != nil {
		slog.Error("Desktop chat failed", "workspace_id", wsID, "session_id", sessionID, "duration", time.Since(start).String(), "error", err)
		return RuntimeChatResponse{}, fmt.Errorf("failed to send message to Crush agent: %w", err)
	}

	msg, err := r.waitForAssistant(chatCtx, wsID, sessionID)
	if err != nil {
		slog.Error("Desktop chat response unavailable", "workspace_id", wsID, "session_id", sessionID, "duration", time.Since(start).String(), "error", err)
		return RuntimeChatResponse{}, err
	}
	slog.Info("Desktop chat finished", "workspace_id", wsID, "session_id", sessionID, "provider", msg.Provider, "model", msg.Model, "duration", time.Since(start).String(), "content_len", len(msg.Content().String()))

	status, err := r.Status(ctx)
	if err != nil {
		return RuntimeChatResponse{}, err
	}

	return RuntimeChatResponse{
		Message: toRuntimeMessage(msg),
		Status:  status,
	}, nil
}

func (r *RuntimeBridge) Messages(ctx context.Context) (RuntimeMessagesResponse, error) {
	if err := r.ensureStarted(ctx); err != nil {
		return RuntimeMessagesResponse{}, err
	}

	r.mu.Lock()
	wsID := r.workspace.ID
	sessionID := r.sessionID
	r.mu.Unlock()

	messages, err := r.runtime.ListSessionMessages(ctx, wsID, sessionID)
	if err != nil {
		return RuntimeMessagesResponse{}, fmt.Errorf("failed to list Crush session messages: %w", err)
	}

	runtimeMessages := make([]RuntimeMessage, 0, len(messages))
	for _, msg := range messages {
		if msg.IsSummaryMessage || msg.Role == message.Tool || msg.Role == message.System {
			continue
		}
		runtimeMessages = append(runtimeMessages, toRuntimeMessage(toProtoMessage(msg)))
	}

	return RuntimeMessagesResponse{Messages: runtimeMessages}, nil
}

func (r *RuntimeBridge) readConfiguredModel() (RuntimeModelConfig, error) {
	layout, err := resolveDesktopLayout()
	if err != nil {
		return RuntimeModelConfig{}, err
	}
	local, result := loadLocalModelConfig(layout)
	if result.Error != nil {
		return RuntimeModelConfig{}, result.Error
	}
	if !result.Applied {
		return RuntimeModelConfig{}, errModelConfigMissing
	}
	return local, nil
}

func (r *RuntimeBridge) NewChat(ctx context.Context, title string) (RuntimeStatus, error) {
	if err := r.ensureStarted(ctx); err != nil {
		return RuntimeStatus{}, err
	}

	r.mu.Lock()
	wsID := r.workspace.ID
	r.mu.Unlock()

	sess, err := r.runtime.CreateSession(ctx, wsID, firstNonEmpty(strings.TrimSpace(title), "New chat"))
	if err != nil {
		return RuntimeStatus{}, fmt.Errorf("failed to create session: %w", err)
	}

	r.mu.Lock()
	r.sessionID = sess.ID
	r.mu.Unlock()

	return r.Status(ctx)
}

func (r *RuntimeBridge) restart() {
	r.mu.Lock()
	defer r.mu.Unlock()

	if r.runtime != nil && r.workspace != nil {
		r.runtime.DeleteWorkspace(r.workspace.ID)
	}
	if r.cancel != nil {
		r.cancel()
	}
	r.runtime = nil
	r.workspace = nil
	r.sessionID = ""
	r.cancel = nil
}

func (r *RuntimeBridge) ensureStarted(ctx context.Context) error {
	r.mu.Lock()
	if r.runtime != nil && r.workspace != nil && r.sessionID != "" {
		r.mu.Unlock()
		return nil
	}
	r.mu.Unlock()

	r.mu.Lock()
	defer r.mu.Unlock()
	if r.runtime != nil && r.workspace != nil && r.sessionID != "" {
		return nil
	}

	layout, err := resolveDesktopLayout()
	if err != nil {
		return err
	}
	if err := ensureDesktopLayout(layout); err != nil {
		return err
	}
	augmentDesktopPath(layout)

	workingDir, err := os.Getwd()
	if err != nil {
		return fmt.Errorf("failed to resolve working directory: %w", err)
	}
	workingDir = filepath.Clean(workingDir)

	cfg := config.NewRuntimeConfig(workingDir, layout.DataDir, false)
	store := config.NewRuntimeStore(workingDir, cfg, layout.ModelConfigPath)
	localResult := applyLocalModelConfig(store, layout)
	if localResult.Error != nil {
		return localResult.Error
	}
	if !store.Config().IsConfigured() {
		return errModelConfigMissing
	}
	store.Config().SetupAgents()

	logFile := filepath.Join(layout.LogsDir, "agent-builder.log")
	crushlog.Setup(logFile, false)
	logConfiguredModel(store)

	ctx, cancel := context.WithCancel(context.Background())
	r.cancel = cancel
	r.runtime = backend.New(ctx, store, nil)

	wsRuntime, ws, err := r.runtime.CreateWorkspace(proto.Workspace{
		Path:    workingDir,
		DataDir: layout.DataDir,
		Version: version.Version,
		Config:  store.Config(),
		Env:     os.Environ(),
	})
	if err != nil {
		cancel()
		r.runtime = nil
		r.cancel = nil
		return fmt.Errorf("failed to create Crush workspace: %w", err)
	}
	workspaceLocalResult := applyLocalModelConfig(wsRuntime.Cfg, layout)
	if workspaceLocalResult.Error != nil {
		return workspaceLocalResult.Error
	}
	if !wsRuntime.Cfg.Config().IsConfigured() {
		return errModelConfigMissing
	}
	wsRuntime.Cfg.SetupAgents()
	r.workspace = &ws

	if err := r.runtime.UpdateAgent(ctx, ws.ID); err != nil {
		return fmt.Errorf("failed to update Crush agent model: %w", err)
	}

	sess, err := r.runtime.CreateSession(ctx, ws.ID, "Desktop chat")
	if err != nil {
		return fmt.Errorf("failed to create Crush session: %w", err)
	}
	r.sessionID = sess.ID
	return nil
}

func augmentDesktopPath(layout desktopLayout) {
	candidates := []string{
		filepath.Join(layout.Root, "tools"),
		filepath.Join(layout.Root, "bin"),
	}
	current := os.Getenv("PATH")
	parts := filepath.SplitList(current)
	for _, candidate := range candidates {
		if _, err := os.Stat(candidate); err != nil {
			continue
		}
		seen := false
		for _, part := range parts {
			if strings.EqualFold(filepath.Clean(part), filepath.Clean(candidate)) {
				seen = true
				break
			}
		}
		if !seen {
			parts = append([]string{candidate}, parts...)
		}
	}
	if len(parts) > 0 {
		_ = os.Setenv("PATH", strings.Join(parts, string(os.PathListSeparator)))
	}
}

func (r *RuntimeBridge) waitForAssistant(ctx context.Context, workspaceID, sessionID string) (proto.Message, error) {
	ticker := time.NewTicker(200 * time.Millisecond)
	defer ticker.Stop()

	for {
		msg, err := r.latestAssistantMessage(ctx, workspaceID, sessionID)
		if err == nil {
			return msg, nil
		}

		select {
		case <-ctx.Done():
			return proto.Message{}, fmt.Errorf("timed out waiting for assistant response: %w", ctx.Err())
		case <-ticker.C:
		}
	}
}

func (r *RuntimeBridge) latestAssistantMessage(ctx context.Context, workspaceID, sessionID string) (proto.Message, error) {
	msgs, err := r.runtime.ListSessionMessages(ctx, workspaceID, sessionID)
	if err != nil {
		return proto.Message{}, fmt.Errorf("failed to read session messages after timeout: %w", err)
	}
	for i := len(msgs) - 1; i >= 0; i-- {
		if msgs[i].Role == message.Assistant {
			return toProtoMessage(msgs[i]), nil
		}
	}
	return proto.Message{}, errors.New("timed out waiting for assistant response")
}

func toProtoMessage(msg message.Message) proto.Message {
	out := proto.Message{
		ID:        msg.ID,
		SessionID: msg.SessionID,
		Role:      proto.MessageRole(msg.Role),
		Model:     msg.Model,
		Provider:  msg.Provider,
		CreatedAt: msg.CreatedAt,
		UpdatedAt: msg.UpdatedAt,
	}

	for _, part := range msg.Parts {
		switch p := part.(type) {
		case message.TextContent:
			out.Parts = append(out.Parts, proto.TextContent{Text: p.Text})
		case message.ReasoningContent:
			out.Parts = append(out.Parts, proto.ReasoningContent{
				Thinking:   p.Thinking,
				Signature:  p.Signature,
				StartedAt:  p.StartedAt,
				FinishedAt: p.FinishedAt,
			})
		case message.ToolCall:
			out.Parts = append(out.Parts, proto.ToolCall{
				ID:       p.ID,
				Name:     p.Name,
				Input:    p.Input,
				Finished: p.Finished,
			})
		case message.ToolResult:
			out.Parts = append(out.Parts, proto.ToolResult{
				ToolCallID: p.ToolCallID,
				Name:       p.Name,
				Content:    p.Content,
				Data:       p.Data,
				MIMEType:   p.MIMEType,
				Metadata:   p.Metadata,
				IsError:    p.IsError,
			})
		case message.Finish:
			out.Parts = append(out.Parts, proto.Finish{
				Reason:  proto.FinishReason(p.Reason),
				Time:    p.Time,
				Message: p.Message,
				Details: p.Details,
			})
		case message.ImageURLContent:
			out.Parts = append(out.Parts, proto.ImageURLContent{
				URL:    p.URL,
				Detail: p.Detail,
			})
		case message.BinaryContent:
			out.Parts = append(out.Parts, proto.BinaryContent{
				Path:     p.Path,
				MIMEType: p.MIMEType,
				Data:     p.Data,
			})
		}
	}

	return out
}

func assistantContent(msg proto.Message) string {
	content := strings.TrimSpace(msg.Content().String())
	if content != "" {
		return msg.Content().String()
	}

	for _, part := range msg.Parts {
		finish, ok := part.(proto.Finish)
		if !ok || finish.Reason != proto.FinishReasonError {
			continue
		}
		switch {
		case finish.Message != "" && finish.Details != "":
			return finish.Message + ": " + finish.Details
		case finish.Message != "":
			return finish.Message
		case finish.Details != "":
			return finish.Details
		default:
			return "Provider returned an error without details. Check logs for more information."
		}
	}

	return msg.Content().String()
}

func toRuntimeMessage(msg proto.Message) RuntimeMessage {
	content := msg.Content().String()
	var finishError string
	finished := false
	for _, part := range msg.Parts {
		finish, ok := part.(proto.Finish)
		if !ok {
			continue
		}
		finished = true
		if finish.Reason != proto.FinishReasonError {
			continue
		}
		switch {
		case finish.Message != "" && finish.Details != "":
			finishError = finish.Message + ": " + finish.Details
		case finish.Message != "":
			finishError = finish.Message
		case finish.Details != "":
			finishError = finish.Details
		default:
			finishError = "Provider returned an error without details. Check logs for more information."
		}
	}
	if strings.TrimSpace(content) == "" && finishError != "" {
		content = finishError
	}

	return RuntimeMessage{
		ID:        msg.ID,
		SessionID: msg.SessionID,
		Role:      string(msg.Role),
		Content:   content,
		Provider:  msg.Provider,
		Model:     msg.Model,
		CreatedAt: msg.CreatedAt,
		UpdatedAt: msg.UpdatedAt,
		Finished:  finished,
		Error:     finishError,
	}
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if value != "" {
			return value
		}
	}
	return ""
}

func resolveDesktopLayout() (desktopLayout, error) {
	root := strings.TrimSpace(os.Getenv("AGENT_BUILDER_DESKTOP_ROOT"))
	if root == "" {
		exe, err := os.Executable()
		if err != nil {
			return desktopLayout{}, fmt.Errorf("failed to resolve executable path: %w", err)
		}
		root = filepath.Dir(exe)
	}
	root = filepath.Clean(root)
	layout := desktopLayout{
		Root:      root,
		ConfigDir: filepath.Join(root, "config"),
		DataDir:   filepath.Join(root, "data"),
		LogsDir:   filepath.Join(root, "logs"),
	}
	layout.ModelConfigPath = filepath.Join(layout.ConfigDir, "model.local.json")
	return layout, nil
}

func ensureDesktopLayout(layout desktopLayout) error {
	for _, dir := range []string{layout.ConfigDir, layout.DataDir, layout.LogsDir} {
		if err := os.MkdirAll(dir, 0o700); err != nil {
			return fmt.Errorf("failed to create desktop directory %s: %w", dir, err)
		}
	}
	return nil
}

func loadLocalModelConfig(layout desktopLayout) (RuntimeModelConfig, localModelConfigResult) {
	result := localModelConfigResult{}
	for _, path := range localModelConfigPaths(layout) {
		result.CheckedPaths = append(result.CheckedPaths, path)
		data, err := os.ReadFile(path)
		if err != nil {
			continue
		}

		var local RuntimeModelConfig
		if err := json.Unmarshal(data, &local); err != nil {
			result.Error = fmt.Errorf("failed to parse local model config %s: %w", path, err)
			return RuntimeModelConfig{}, result
		}
		if local.Model == "" && len(local.Models) > 0 {
			local.Model = local.Models[0]
		}
		if local.Model != "" && len(local.Models) == 0 {
			local.Models = []string{local.Model}
		}
		if err := validateModelConfig(local); err != nil {
			result.Error = fmt.Errorf("invalid local model config %s: %w", path, err)
			return RuntimeModelConfig{}, result
		}

		if path != layout.ModelConfigPath {
			_ = saveLocalModelConfig(layout, local)
		}

		result.Applied = true
		result.Path = path
		return local, result
	}
	return RuntimeModelConfig{}, result
}

func applyLocalModelConfig(store *config.ConfigStore, layout desktopLayout) localModelConfigResult {
	local, result := loadLocalModelConfig(layout)
	if result.Error != nil || !result.Applied {
		return result
	}
	applyModelConfig(store, local)
	return result
}

func validateModelConfig(local RuntimeModelConfig) error {
	if local.Protocol != "openai" && local.Protocol != "anthropic" {
		return errors.New("protocol must be openai or anthropic")
	}
	if local.APIKey == "" || local.URL == "" || local.Model == "" {
		return errors.New("url, apiKey, and model are required")
	}
	return nil
}

func saveLocalModelConfig(layout desktopLayout, local RuntimeModelConfig) error {
	if err := ensureDesktopLayout(layout); err != nil {
		return err
	}
	local.ConfigPath = ""
	local.HasAPIKey = false
	if local.Model != "" {
		local.Models = []string{local.Model}
	}
	data, err := json.MarshalIndent(local, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to encode local model config: %w", err)
	}
	data = append(data, '\n')
	if err := os.WriteFile(layout.ModelConfigPath, data, 0o600); err != nil {
		return fmt.Errorf("failed to write local model config: %w", err)
	}
	return nil
}

func applyModelConfig(store *config.ConfigStore, local RuntimeModelConfig) {
	if store.Config().Options == nil {
		store.Config().Options = &config.Options{}
	}
	autoLSP := false
	store.Config().Options.AutoLSP = &autoLSP

	providerType := catwalk.TypeOpenAICompat
	if local.Protocol == "anthropic" {
		providerType = catwalk.TypeAnthropic
	}

	modelIDs := local.Models
	if len(modelIDs) == 0 && local.Model != "" {
		modelIDs = []string{local.Model}
	}
	models := make([]catwalk.Model, 0, len(modelIDs))
	for _, model := range modelIDs {
		if model == "" {
			continue
		}
		models = append(models, catwalk.Model{
			ID:               model,
			Name:             model,
			ContextWindow:    64000,
			DefaultMaxTokens: 4096,
		})
	}
	if len(models) == 0 {
		return
	}

	baseURL := normalizeModelBaseURL(local.Protocol, local.URL)
	store.Config().Providers.Set(localProviderID, config.ProviderConfig{
		ID:      localProviderID,
		Name:    "Local Model",
		BaseURL: baseURL,
		Type:    providerType,
		APIKey:  local.APIKey,
		Models:  models,
	})

	selected := config.SelectedModel{
		Provider:  localProviderID,
		Model:     models[0].ID,
		MaxTokens: models[0].DefaultMaxTokens,
	}
	store.Config().Models[config.SelectedModelTypeLarge] = selected
	store.Config().Models[config.SelectedModelTypeSmall] = selected
}

func normalizeModelBaseURL(protocol, rawURL string) string {
	trimmed := strings.TrimRight(strings.TrimSpace(rawURL), "/")
	if protocol == "openai" && !strings.HasSuffix(trimmed, "/v1") {
		return trimmed + "/v1"
	}
	return trimmed
}

func localModelConfigPaths(layout desktopLayout) []string {
	return []string{filepath.Clean(layout.ModelConfigPath)}
}

func logConfiguredModel(store *config.ConfigStore) {
	selected := store.Config().Models[config.SelectedModelTypeLarge]
	provider, _ := store.Config().Providers.Get(selected.Provider)
	slog.Info(
		"Desktop model configured",
		"provider", selected.Provider,
		"protocol", string(provider.Type),
		"model", selected.Model,
		"base_url", provider.BaseURL,
		"has_api_key", provider.APIKey != "",
	)
}
