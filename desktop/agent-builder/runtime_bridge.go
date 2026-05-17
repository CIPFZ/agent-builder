package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/user"
	"path/filepath"
	"runtime"
	"strings"
	"sync"

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

type RuntimeChatRequest struct {
	Prompt string `json:"prompt"`
}

type RuntimeChatResponse struct {
	Provider string `json:"provider"`
	Content  string `json:"content"`
	Model    string `json:"model"`
}

type localModelConfig struct {
	Protocol string   `json:"protocol"`
	URL      string   `json:"url"`
	APIKey   string   `json:"apiKey"`
	Models   []string `json:"models"`
}

type localModelConfigResult struct {
	Applied      bool
	Path         string
	CheckedPaths []string
	Error        error
}

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

	if err := r.runtime.SendMessage(ctx, wsID, proto.AgentMessage{
		SessionID: sessionID,
		Prompt:    prompt,
	}); err != nil {
		return RuntimeChatResponse{}, fmt.Errorf("failed to send message to Crush agent: %w", err)
	}

	msg, err := r.waitForAssistant(ctx, wsID, sessionID)
	if err != nil {
		return RuntimeChatResponse{}, err
	}

	return RuntimeChatResponse{
		Provider: msg.Provider,
		Content:  msg.Content().String(),
		Model:    msg.Model,
	}, nil
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

	workingDir, err := os.Getwd()
	if err != nil {
		return fmt.Errorf("failed to resolve working directory: %w", err)
	}
	workingDir = filepath.Clean(workingDir)

	store, err := config.Init(workingDir, "", false)
	if err != nil {
		return fmt.Errorf("failed to initialize Crush config: %w", err)
	}
	localResult := applyLocalModelConfig(store, workingDir)
	if localResult.Error != nil {
		return localResult.Error
	}
	if !store.Config().IsConfigured() {
		return fmt.Errorf(
			"Crush has no configured provider. Configure a provider in crush.json or create an Agent Builder local model config. Checked: %s",
			strings.Join(localResult.CheckedPaths, ", "),
		)
	}

	logFile := filepath.Join(store.Config().Options.DataDirectory, "logs", "agent-builder.log")
	crushlog.Setup(logFile, false)

	ctx, cancel := context.WithCancel(context.Background())
	r.cancel = cancel
	r.runtime = backend.New(ctx, store, nil)

	wsRuntime, ws, err := r.runtime.CreateWorkspace(proto.Workspace{
		Path:    workingDir,
		Version: version.Version,
		Env:     os.Environ(),
	})
	if err != nil {
		cancel()
		r.runtime = nil
		r.cancel = nil
		return fmt.Errorf("failed to create Crush workspace: %w", err)
	}
	workspaceLocalResult := applyLocalModelConfig(wsRuntime.Cfg, workingDir)
	if workspaceLocalResult.Error != nil {
		return workspaceLocalResult.Error
	}
	if !wsRuntime.Cfg.Config().IsConfigured() {
		return fmt.Errorf(
			"Crush workspace has no configured provider. Configure a provider in crush.json or create an Agent Builder local model config. Checked: %s",
			strings.Join(workspaceLocalResult.CheckedPaths, ", "),
		)
	}
	wsRuntime.Cfg.SetupAgents()
	r.workspace = &ws

	if err := r.runtime.InitAgent(ctx, ws.ID); err != nil {
		return fmt.Errorf("failed to initialize Crush coder agent: %w", err)
	}
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

func (r *RuntimeBridge) waitForAssistant(ctx context.Context, workspaceID, sessionID string) (proto.Message, error) {
	return r.latestAssistantMessage(ctx, workspaceID, sessionID)
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

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if value != "" {
			return value
		}
	}
	return ""
}

func applyLocalModelConfig(store *config.ConfigStore, workingDir string) localModelConfigResult {
	result := localModelConfigResult{}
	for _, path := range localModelConfigPaths(workingDir) {
		result.CheckedPaths = append(result.CheckedPaths, path)
		data, err := os.ReadFile(path)
		if err != nil {
			continue
		}

		var local localModelConfig
		if err := json.Unmarshal(data, &local); err != nil {
			result.Error = fmt.Errorf("failed to parse local model config %s: %w", path, err)
			return result
		}
		if local.APIKey == "" || local.URL == "" || len(local.Models) == 0 {
			result.Error = fmt.Errorf("local model config %s requires url, apiKey, and models", path)
			return result
		}

		applyModelConfig(store, local)
		result.Applied = true
		result.Path = path
		return result
	}
	return result
}

func applyModelConfig(store *config.ConfigStore, local localModelConfig) {
	if store.Config().Options == nil {
		store.Config().Options = &config.Options{}
	}
	autoLSP := false
	store.Config().Options.AutoLSP = &autoLSP

	providerType := catwalk.TypeOpenAICompat
	if local.Protocol == "anthropic" {
		providerType = catwalk.TypeAnthropic
	}

	models := make([]catwalk.Model, 0, len(local.Models))
	for _, model := range local.Models {
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

	providerID := "deepseek-local"
	store.Config().Providers.Set(providerID, config.ProviderConfig{
		ID:      providerID,
		Name:    "DeepSeek Local",
		BaseURL: local.URL,
		Type:    providerType,
		APIKey:  local.APIKey,
		Models:  models,
	})

	selected := config.SelectedModel{
		Provider:  providerID,
		Model:     models[0].ID,
		MaxTokens: models[0].DefaultMaxTokens,
	}
	store.Config().Models[config.SelectedModelTypeLarge] = selected
	store.Config().Models[config.SelectedModelTypeSmall] = selected
}

func localModelConfigPaths(workingDir string) []string {
	var paths []string
	add := func(path string) {
		if path == "" {
			return
		}
		path = filepath.Clean(path)
		for _, existing := range paths {
			if strings.EqualFold(existing, path) {
				return
			}
		}
		paths = append(paths, path)
	}

	appendRelativeCandidates := func(root string) {
		if root == "" {
			return
		}
		add(filepath.Join(root, "agent-builder.local.json"))
		add(filepath.Join(root, "client", "server", "deepseek.local.json"))
	}

	for current := workingDir; current != ""; current = filepath.Dir(current) {
		appendRelativeCandidates(current)
		if filepath.VolumeName(current) == current || filepath.Dir(current) == current {
			break
		}
	}
	if exe, err := os.Executable(); err == nil {
		for current := filepath.Dir(exe); ; current = filepath.Dir(current) {
			appendRelativeCandidates(current)
			if filepath.VolumeName(current) == current || filepath.Dir(current) == current {
				break
			}
		}
	}

	if appData := os.Getenv("APPDATA"); appData != "" {
		add(filepath.Join(appData, "AgentBuilder", "model.local.json"))
	}
	if localAppData := os.Getenv("LOCALAPPDATA"); localAppData != "" {
		add(filepath.Join(localAppData, "AgentBuilder", "model.local.json"))
	}
	if home, err := os.UserHomeDir(); err == nil {
		add(filepath.Join(home, ".agent-builder", "model.local.json"))
	}
	if currentUser, err := user.Current(); err == nil && currentUser.HomeDir != "" {
		add(filepath.Join(currentUser.HomeDir, ".agent-builder", "model.local.json"))
	}
	if runtime.GOOS != "windows" {
		if xdg := os.Getenv("XDG_CONFIG_HOME"); xdg != "" {
			add(filepath.Join(xdg, "agent-builder", "model.local.json"))
		}
	}
	return paths
}
