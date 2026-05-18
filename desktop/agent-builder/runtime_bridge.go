package main

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	tea "charm.land/bubbletea/v2"
	"charm.land/catwalk/pkg/catwalk"
	"github.com/charmbracelet/crush/internal/backend"
	"github.com/charmbracelet/crush/internal/config"
	crushlog "github.com/charmbracelet/crush/internal/log"
	"github.com/charmbracelet/crush/internal/message"
	"github.com/charmbracelet/crush/internal/permission"
	"github.com/charmbracelet/crush/internal/proto"
	"github.com/charmbracelet/crush/internal/pubsub"
	"github.com/charmbracelet/crush/internal/session"
	"github.com/charmbracelet/crush/internal/version"
)

// RuntimeBridge exposes a small desktop-facing API over the real Crush
// runtime. The React UI stays thin; this service owns workspace, session, and
// agent lifecycle.
type RuntimeBridge struct {
	mu          sync.Mutex
	runtime     *backend.Backend
	workspace   *proto.Workspace
	sessionID   string
	runtimeCtx  context.Context
	cancel      context.CancelFunc
	eventStats  runtimeEventStats
	requests    map[string]runtimeRequestState
	permissions map[string]pendingRuntimePermission
	events      []RuntimeEvent
}

type RuntimeStatus struct {
	Ready       bool              `json:"ready"`
	WorkspaceID string            `json:"workspaceId"`
	SessionID   string            `json:"sessionId"`
	WorkingDir  string            `json:"workingDir"`
	Model       string            `json:"model"`
	Provider    string            `json:"provider"`
	Busy        bool              `json:"busy"`
	Usage       RuntimeUsage      `json:"usage"`
	Events      RuntimeEventStats `json:"events"`
	Requests    RuntimeRequests   `json:"requests"`
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
	RequestID string        `json:"requestId"`
	Status    RuntimeStatus `json:"status"`
}

type RuntimeMessage struct {
	ID           string               `json:"id"`
	SessionID    string               `json:"sessionId"`
	Role         string               `json:"role"`
	Content      string               `json:"content"`
	Parts        []RuntimeMessagePart `json:"parts,omitempty"`
	Provider     string               `json:"provider,omitempty"`
	Model        string               `json:"model,omitempty"`
	CreatedAt    int64                `json:"createdAt"`
	UpdatedAt    int64                `json:"updatedAt"`
	Finished     bool                 `json:"finished"`
	FinishReason string               `json:"finishReason,omitempty"`
	Error        string               `json:"error,omitempty"`
}

type RuntimeMessagePart struct {
	Type       string `json:"type"`
	Text       string `json:"text,omitempty"`
	Thinking   string `json:"thinking,omitempty"`
	StartedAt  int64  `json:"startedAt,omitempty"`
	FinishedAt int64  `json:"finishedAt,omitempty"`
	ToolCallID string `json:"toolCallId,omitempty"`
	Name       string `json:"name,omitempty"`
	Input      string `json:"input,omitempty"`
	Finished   bool   `json:"finished,omitempty"`
	Content    string `json:"content,omitempty"`
	Data       string `json:"data,omitempty"`
	MIMEType   string `json:"mimeType,omitempty"`
	Metadata   string `json:"metadata,omitempty"`
	IsError    bool   `json:"isError,omitempty"`
	Reason     string `json:"reason,omitempty"`
	Message    string `json:"message,omitempty"`
	Details    string `json:"details,omitempty"`
}

type RuntimeMessagesResponse struct {
	Messages []RuntimeMessage `json:"messages"`
}

type RuntimePermissionRequest struct {
	ID          string `json:"id"`
	SessionID   string `json:"sessionId"`
	ToolCallID  string `json:"toolCallId"`
	ToolName    string `json:"toolName"`
	Description string `json:"description,omitempty"`
	Action      string `json:"action"`
	Params      any    `json:"params,omitempty"`
	Path        string `json:"path,omitempty"`
	CreatedAt   int64  `json:"createdAt"`
}

type RuntimePermissionsResponse struct {
	Permissions []RuntimePermissionRequest `json:"permissions"`
}

type RuntimePermissionDecision struct {
	PermissionID string `json:"permissionId"`
	Action       string `json:"action"`
}

type RuntimeRequests struct {
	ActiveRequestID  string `json:"activeRequestId,omitempty"`
	ActiveStartedAt  int64  `json:"activeStartedAt,omitempty"`
	ActiveDurationMS int64  `json:"activeDurationMs,omitempty"`
	Running          int    `json:"running"`
}

type RuntimeUsage struct {
	PromptTokens     int64   `json:"promptTokens"`
	CompletionTokens int64   `json:"completionTokens"`
	TotalTokens      int64   `json:"totalTokens"`
	Cost             float64 `json:"cost"`
}

type RuntimeEventStats struct {
	LastEventAt      int64 `json:"lastEventAt"`
	MessageEvents    int64 `json:"messageEvents"`
	SessionEvents    int64 `json:"sessionEvents"`
	OtherEvents      int64 `json:"otherEvents"`
	AssistantEvents  int64 `json:"assistantEvents"`
	PermissionEvents int64 `json:"permissionEvents"`
}

type RuntimeEvent struct {
	Type      string `json:"type"`
	Role      string `json:"role,omitempty"`
	SessionID string `json:"sessionId,omitempty"`
	MessageID string `json:"messageId,omitempty"`
	CreatedAt int64  `json:"createdAt"`
	Summary   string `json:"summary,omitempty"`
}

type RuntimeEventsResponse struct {
	Events []RuntimeEvent `json:"events"`
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
	Config       RuntimeModelConfig
	Error        error
}

type desktopLayout struct {
	Root            string
	ConfigDir       string
	DataDir         string
	LogsDir         string
	ModelConfigPath string
}

type runtimeRequestState struct {
	StartedAt int64  `json:"startedAt"`
	Finished  bool   `json:"finished"`
	Error     string `json:"error,omitempty"`
}

type pendingRuntimePermission struct {
	Permission RuntimePermissionRequest
	Raw        permission.PermissionRequest
}

type runtimeEventStats struct {
	lastEventAt      int64
	messageEvents    int64
	sessionEvents    int64
	otherEvents      int64
	assistantEvents  int64
	permissionEvents int64
}

const runtimeEventLimit = 200

type auditEntry struct {
	RequestID             string        `json:"request_id"`
	Event                 string        `json:"event"`
	Timestamp             string        `json:"timestamp"`
	WorkspaceID           string        `json:"workspace_id,omitempty"`
	SessionID             string        `json:"session_id,omitempty"`
	Provider              string        `json:"provider,omitempty"`
	Model                 string        `json:"model,omitempty"`
	PromptLength          int           `json:"prompt_length,omitempty"`
	PromptPreview         string        `json:"prompt_preview,omitempty"`
	ResponseLength        int           `json:"response_length,omitempty"`
	ResponsePreview       string        `json:"response_preview,omitempty"`
	DurationMS            int64         `json:"duration_ms,omitempty"`
	FinishReason          string        `json:"finish_reason,omitempty"`
	UsageBefore           *RuntimeUsage `json:"usage_before,omitempty"`
	UsageAfter            *RuntimeUsage `json:"usage_after,omitempty"`
	UsageDelta            *RuntimeUsage `json:"usage_delta,omitempty"`
	Error                 string        `json:"error,omitempty"`
	LatestAssistantID     string        `json:"latest_assistant_id,omitempty"`
	LatestAssistantFinish bool          `json:"latest_assistant_finished,omitempty"`
	PermissionTool        string        `json:"permission_tool,omitempty"`
	PermissionAction      string        `json:"permission_action,omitempty"`
	PermissionPath        string        `json:"permission_path,omitempty"`
	PermissionPolicy      string        `json:"permission_policy,omitempty"`
}

const localProviderID = "local-model"
const auditPreviewLimit = 600
const runtimePartPreviewLimit = 4000

var errModelConfigMissing = errors.New("model is not configured. Open model settings and save protocol, URL, API key, and model before chatting.")

func NewRuntimeBridge() *RuntimeBridge {
	return &RuntimeBridge{
		requests:    make(map[string]runtimeRequestState),
		permissions: make(map[string]pendingRuntimePermission),
	}
}

func (r *RuntimeBridge) Status(ctx context.Context) (RuntimeStatus, error) {
	if err := r.ensureStarted(ctx); err != nil {
		return RuntimeStatus{}, err
	}

	r.mu.Lock()
	ws := *r.workspace
	sessionID := r.sessionID
	events := r.eventStats.snapshot()
	requests := r.runtimeRequestsLocked()
	r.mu.Unlock()

	info, err := r.runtime.GetAgentInfo(ws.ID)
	if err != nil {
		return RuntimeStatus{}, err
	}
	usage, err := r.sessionUsage(ctx, ws.ID, sessionID)
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
		Usage:       usage,
		Events:      events,
		Requests:    requests,
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
	runCtx := r.runtimeCtx
	if runCtx == nil {
		runCtx = context.Background()
	}
	r.mu.Unlock()

	requestID := newRequestID()
	start := time.Now()
	usageBefore, err := r.sessionUsage(ctx, wsID, sessionID)
	if err != nil {
		return RuntimeChatResponse{}, err
	}

	r.mu.Lock()
	r.requests[requestID] = runtimeRequestState{StartedAt: start.UnixMilli()}
	r.mu.Unlock()

	status, err := r.Status(ctx)
	if err != nil {
		return RuntimeChatResponse{}, err
	}

	slog.Info("Desktop chat queued", "request_id", requestID, "workspace_id", wsID, "session_id", sessionID, "prompt_len", len(prompt))
	r.writeAudit(auditEntry{
		RequestID:     requestID,
		Event:         "started",
		Timestamp:     start.Format(time.RFC3339Nano),
		WorkspaceID:   wsID,
		SessionID:     sessionID,
		Provider:      status.Provider,
		Model:         status.Model,
		PromptLength:  len(prompt),
		PromptPreview: preview(prompt, auditPreviewLimit),
		UsageBefore:   &usageBefore,
	})

	go r.runChat(runCtx, requestID, wsID, sessionID, prompt, start, usageBefore, status.Provider, status.Model)

	return RuntimeChatResponse{
		RequestID: requestID,
		Status:    status,
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
		if msg.IsSummaryMessage || msg.Role == message.System {
			continue
		}
		runtimeMessage := toRuntimeMessage(toProtoMessage(msg))
		if !isDisplayableRuntimeMessage(runtimeMessage) {
			continue
		}
		runtimeMessages = append(runtimeMessages, runtimeMessage)
	}

	return RuntimeMessagesResponse{Messages: runtimeMessages}, nil
}

func (r *RuntimeBridge) Permissions(ctx context.Context) (RuntimePermissionsResponse, error) {
	if err := r.ensureStarted(ctx); err != nil {
		return RuntimePermissionsResponse{}, err
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	permissions := make([]RuntimePermissionRequest, 0, len(r.permissions))
	for _, pending := range r.permissions {
		permissions = append(permissions, pending.Permission)
	}
	return RuntimePermissionsResponse{Permissions: permissions}, nil
}

func (r *RuntimeBridge) Events(ctx context.Context) (RuntimeEventsResponse, error) {
	if err := r.ensureStarted(ctx); err != nil {
		return RuntimeEventsResponse{}, err
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	events := make([]RuntimeEvent, len(r.events))
	copy(events, r.events)
	return RuntimeEventsResponse{Events: events}, nil
}

func (r *RuntimeBridge) DecidePermission(ctx context.Context, req RuntimePermissionDecision) (RuntimeStatus, error) {
	if err := r.ensureStarted(ctx); err != nil {
		return RuntimeStatus{}, err
	}

	action := proto.PermissionAction(strings.TrimSpace(req.Action))
	if action != proto.PermissionAllow && action != proto.PermissionAllowForSession && action != proto.PermissionDeny {
		return RuntimeStatus{}, fmt.Errorf("invalid permission action: %s", req.Action)
	}

	r.mu.Lock()
	wsID := r.workspace.ID
	pending, ok := r.permissions[req.PermissionID]
	r.mu.Unlock()
	if !ok {
		return RuntimeStatus{}, fmt.Errorf("permission request %s is not pending", req.PermissionID)
	}

	if err := r.runtime.GrantPermission(wsID, proto.PermissionGrant{
		Permission: toProtoPermissionRequest(pending.Raw),
		Action:     action,
	}); err != nil {
		return RuntimeStatus{}, fmt.Errorf("failed to decide permission: %w", err)
	}

	r.mu.Lock()
	delete(r.permissions, req.PermissionID)
	r.mu.Unlock()

	r.writeAudit(auditEntry{
		Event:            "permission_decided",
		Timestamp:        time.Now().Format(time.RFC3339Nano),
		WorkspaceID:      wsID,
		SessionID:        pending.Permission.SessionID,
		PermissionTool:   pending.Permission.ToolName,
		PermissionAction: pending.Permission.Action,
		PermissionPath:   pending.Permission.Path,
		PermissionPolicy: string(action),
	})

	return r.Status(ctx)
}

func (r *RuntimeBridge) Cancel(ctx context.Context) (RuntimeStatus, error) {
	if err := r.ensureStarted(ctx); err != nil {
		return RuntimeStatus{}, err
	}

	r.mu.Lock()
	wsID := r.workspace.ID
	sessionID := r.sessionID
	r.mu.Unlock()

	if err := r.runtime.CancelSession(wsID, sessionID); err != nil {
		return RuntimeStatus{}, fmt.Errorf("failed to cancel session: %w", err)
	}
	r.writeAudit(auditEntry{
		Event:       "cancel_requested",
		Timestamp:   time.Now().Format(time.RFC3339Nano),
		WorkspaceID: wsID,
		SessionID:   sessionID,
	})
	return r.Status(ctx)
}

func (r *RuntimeBridge) runChat(ctx context.Context, requestID, wsID, sessionID, prompt string, start time.Time, usageBefore RuntimeUsage, provider, model string) {
	err := r.runtime.SendMessage(ctx, wsID, proto.AgentMessage{
		SessionID: sessionID,
		Prompt:    prompt,
	})
	duration := time.Since(start)
	usageAfter, usageErr := r.sessionUsage(context.Background(), wsID, sessionID)
	if usageErr != nil {
		slog.Error("Desktop chat usage unavailable", "request_id", requestID, "workspace_id", wsID, "session_id", sessionID, "error", usageErr)
	}
	assistant, assistantErr := r.latestFinishedAssistantMessage(context.Background(), wsID, sessionID)
	if assistantErr != nil {
		slog.Warn("Desktop chat assistant message unavailable", "request_id", requestID, "workspace_id", wsID, "session_id", sessionID, "error", assistantErr)
	}

	r.mu.Lock()
	state := r.requests[requestID]
	state.Finished = true
	if err != nil {
		state.Error = err.Error()
	}
	r.requests[requestID] = state
	r.mu.Unlock()

	entry := auditEntry{
		RequestID:     requestID,
		Timestamp:     time.Now().Format(time.RFC3339Nano),
		WorkspaceID:   wsID,
		SessionID:     sessionID,
		Provider:      provider,
		Model:         model,
		DurationMS:    duration.Milliseconds(),
		PromptLength:  len(prompt),
		PromptPreview: preview(prompt, auditPreviewLimit),
	}
	usageDelta := usageAfter.Sub(usageBefore)
	entry.UsageBefore = &usageBefore
	entry.UsageAfter = &usageAfter
	entry.UsageDelta = &usageDelta
	if assistantErr == nil {
		runtimeMsg := toRuntimeMessage(assistant)
		entry.LatestAssistantID = runtimeMsg.ID
		entry.LatestAssistantFinish = runtimeMsg.Finished
		entry.FinishReason = runtimeMsg.FinishReason
		entry.ResponseLength = len(runtimeMsg.Content)
		entry.ResponsePreview = preview(runtimeMsg.Content, auditPreviewLimit)
	}
	if err != nil {
		entry.Event = "failed"
		entry.Error = err.Error()
		slog.Error("Desktop chat failed", "request_id", requestID, "workspace_id", wsID, "session_id", sessionID, "duration", duration.String(), "error", err)
	} else {
		entry.Event = "finished"
		slog.Info("Desktop chat finished", "request_id", requestID, "workspace_id", wsID, "session_id", sessionID, "provider", provider, "model", model, "duration", duration.String(), "content_len", entry.ResponseLength, "finish_reason", entry.FinishReason)
	}
	r.writeAudit(entry)
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
	r.runtimeCtx = nil
	r.cancel = nil
	r.eventStats = runtimeEventStats{}
	r.requests = make(map[string]runtimeRequestState)
	r.permissions = make(map[string]pendingRuntimePermission)
	r.events = nil
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
	applyDesktopProxy(localResult)

	logFile := filepath.Join(layout.LogsDir, "agent-builder.log")
	crushlog.Setup(logFile, false)
	logConfiguredModel(store)

	runtimeCtx, cancel := context.WithCancel(context.Background())
	r.runtimeCtx = runtimeCtx
	r.cancel = cancel
	r.runtime = backend.New(runtimeCtx, store, nil)

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
	go r.consumeRuntimeEvents(runtimeCtx, ws.ID)
	go r.consumeDesktopPermissions(runtimeCtx, ws.ID, wsRuntime.Permissions)

	if err := r.runtime.UpdateAgent(runtimeCtx, ws.ID); err != nil {
		return fmt.Errorf("failed to update Crush agent model: %w", err)
	}

	sess, err := r.runtime.CreateSession(ctx, ws.ID, "Desktop chat")
	if err != nil {
		return fmt.Errorf("failed to create Crush session: %w", err)
	}
	r.sessionID = sess.ID
	return nil
}

func (r *RuntimeBridge) consumeRuntimeEvents(ctx context.Context, workspaceID string) {
	events, err := r.runtime.SubscribeEvents(ctx, workspaceID)
	if err != nil {
		slog.Error("Failed to subscribe to Crush runtime events", "workspace_id", workspaceID, "error", err)
		return
	}
	for {
		select {
		case event, ok := <-events:
			if !ok {
				return
			}
			r.recordRuntimeEvent(event)
		case <-ctx.Done():
			return
		}
	}
}

func (r *RuntimeBridge) consumeDesktopPermissions(ctx context.Context, workspaceID string, permissions permission.Service) {
	events := permissions.Subscribe(ctx)
	for {
		select {
		case event, ok := <-events:
			if !ok {
				return
			}
			perm := event.Payload
			runtimePerm := toRuntimePermissionRequest(perm)
			r.mu.Lock()
			r.permissions[perm.ID] = pendingRuntimePermission{
				Permission: runtimePerm,
				Raw:        perm,
			}
			r.eventStats.permissionEvents++
			r.eventStats.lastEventAt = time.Now().UnixMilli()
			r.appendRuntimeEventLocked(RuntimeEvent{
				Type:      "permission_requested",
				SessionID: perm.SessionID,
				MessageID: perm.ToolCallID,
				CreatedAt: time.Now().UnixMilli(),
				Summary:   perm.ToolName + ":" + perm.Action,
			})
			r.mu.Unlock()

			slog.Info("Desktop permission requested", "workspace_id", workspaceID, "session_id", perm.SessionID, "tool", perm.ToolName, "action", perm.Action, "path", perm.Path)
			r.writeAudit(auditEntry{
				Event:            "permission_requested",
				Timestamp:        time.Now().Format(time.RFC3339Nano),
				WorkspaceID:      workspaceID,
				SessionID:        perm.SessionID,
				PermissionTool:   perm.ToolName,
				PermissionAction: perm.Action,
				PermissionPath:   perm.Path,
				PermissionPolicy: "ask",
			})
		case <-ctx.Done():
			return
		}
	}
}

func (r *RuntimeBridge) recordRuntimeEvent(event pubsub.Event[tea.Msg]) {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.eventStats.lastEventAt = time.Now().UnixMilli()
	switch payload := event.Payload.(type) {
	case pubsub.Event[message.Message]:
		r.eventStats.messageEvents++
		r.appendRuntimeEventLocked(RuntimeEvent{
			Type:      "message",
			Role:      string(payload.Payload.Role),
			SessionID: payload.Payload.SessionID,
			MessageID: payload.Payload.ID,
			CreatedAt: r.eventStats.lastEventAt,
			Summary:   preview(payload.Payload.Content().Text, 160),
		})
		if payload.Payload.Role == message.Assistant {
			r.eventStats.assistantEvents++
		}
	case pubsub.Event[proto.Message]:
		r.eventStats.messageEvents++
		r.appendRuntimeEventLocked(RuntimeEvent{
			Type:      "message",
			Role:      string(payload.Payload.Role),
			SessionID: payload.Payload.SessionID,
			MessageID: payload.Payload.ID,
			CreatedAt: r.eventStats.lastEventAt,
			Summary:   preview(payload.Payload.Content().Text, 160),
		})
		if payload.Payload.Role == proto.Assistant {
			r.eventStats.assistantEvents++
		}
	case pubsub.Event[proto.Session]:
		r.eventStats.sessionEvents++
		r.appendRuntimeEventLocked(RuntimeEvent{
			Type:      "session",
			SessionID: payload.Payload.ID,
			CreatedAt: r.eventStats.lastEventAt,
			Summary:   payload.Payload.Title,
		})
	case pubsub.Event[session.Session]:
		r.eventStats.sessionEvents++
		r.appendRuntimeEventLocked(RuntimeEvent{
			Type:      "session",
			SessionID: payload.Payload.ID,
			CreatedAt: r.eventStats.lastEventAt,
			Summary:   payload.Payload.Title,
		})
	case pubsub.Event[permission.PermissionRequest]:
		r.eventStats.permissionEvents++
	case pubsub.Event[proto.PermissionRequest]:
		r.eventStats.permissionEvents++
	default:
		r.eventStats.otherEvents++
	}
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

func (r *RuntimeBridge) latestFinishedAssistantMessage(ctx context.Context, workspaceID, sessionID string) (proto.Message, error) {
	msgs, err := r.runtime.ListSessionMessages(ctx, workspaceID, sessionID)
	if err != nil {
		return proto.Message{}, fmt.Errorf("failed to read session messages: %w", err)
	}
	for i := len(msgs) - 1; i >= 0; i-- {
		if msgs[i].Role == message.Assistant && msgs[i].FinishPart() != nil {
			return toProtoMessage(msgs[i]), nil
		}
	}
	return proto.Message{}, errors.New("finished assistant response is not available")
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
	var finishReason string
	finished := false
	for _, part := range msg.Parts {
		finish, ok := part.(proto.Finish)
		if !ok {
			continue
		}
		finished = true
		finishReason = string(finish.Reason)
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
		ID:           msg.ID,
		SessionID:    msg.SessionID,
		Role:         string(msg.Role),
		Content:      content,
		Parts:        toRuntimeMessageParts(msg),
		Provider:     msg.Provider,
		Model:        msg.Model,
		CreatedAt:    msg.CreatedAt,
		UpdatedAt:    msg.UpdatedAt,
		Finished:     finished,
		FinishReason: finishReason,
		Error:        finishError,
	}
}

func toRuntimeMessageParts(msg proto.Message) []RuntimeMessagePart {
	parts := make([]RuntimeMessagePart, 0, len(msg.Parts))
	for _, part := range msg.Parts {
		switch p := part.(type) {
		case proto.TextContent:
			parts = append(parts, RuntimeMessagePart{
				Type: "text",
				Text: p.Text,
			})
		case proto.ReasoningContent:
			parts = append(parts, RuntimeMessagePart{
				Type:       "reasoning",
				Thinking:   p.Thinking,
				StartedAt:  p.StartedAt,
				FinishedAt: p.FinishedAt,
			})
		case proto.ToolCall:
			parts = append(parts, RuntimeMessagePart{
				Type:       "tool_call",
				ToolCallID: p.ID,
				Name:       p.Name,
				Input:      preview(p.Input, runtimePartPreviewLimit),
				Finished:   p.Finished,
			})
		case proto.ToolResult:
			parts = append(parts, RuntimeMessagePart{
				Type:       "tool_result",
				ToolCallID: p.ToolCallID,
				Name:       p.Name,
				Content:    preview(p.Content, runtimePartPreviewLimit),
				Data:       preview(p.Data, runtimePartPreviewLimit),
				MIMEType:   p.MIMEType,
				Metadata:   preview(p.Metadata, runtimePartPreviewLimit),
				IsError:    p.IsError,
			})
		case proto.Finish:
			parts = append(parts, RuntimeMessagePart{
				Type:    "finish",
				Reason:  string(p.Reason),
				Message: p.Message,
				Details: p.Details,
			})
		case proto.ImageURLContent:
			parts = append(parts, RuntimeMessagePart{
				Type: "image_url",
				Text: p.URL,
			})
		case proto.BinaryContent:
			parts = append(parts, RuntimeMessagePart{
				Type:     "binary",
				Text:     p.Path,
				MIMEType: p.MIMEType,
			})
		}
	}
	return parts
}

func isDisplayableRuntimeMessage(msg RuntimeMessage) bool {
	if msg.Role == string(message.User) {
		return strings.TrimSpace(msg.Content) != ""
	}
	if msg.Role != string(message.Assistant) && msg.Role != string(message.Tool) {
		return false
	}
	if strings.TrimSpace(msg.Content) != "" || msg.Error != "" {
		return true
	}
	for _, part := range msg.Parts {
		switch part.Type {
		case "reasoning":
			if strings.TrimSpace(part.Thinking) != "" {
				return true
			}
		case "tool_call", "tool_result":
			return true
		}
	}
	return msg.Finished && msg.FinishReason == string(proto.FinishReasonError)
}

func (r *RuntimeBridge) sessionUsage(ctx context.Context, workspaceID, sessionID string) (RuntimeUsage, error) {
	sess, err := r.runtime.GetSession(ctx, workspaceID, sessionID)
	if err != nil {
		return RuntimeUsage{}, fmt.Errorf("failed to read Crush session usage: %w", err)
	}
	return RuntimeUsage{
		PromptTokens:     sess.PromptTokens,
		CompletionTokens: sess.CompletionTokens,
		TotalTokens:      sess.PromptTokens + sess.CompletionTokens,
		Cost:             sess.Cost,
	}, nil
}

func (u RuntimeUsage) Sub(before RuntimeUsage) RuntimeUsage {
	return RuntimeUsage{
		PromptTokens:     u.PromptTokens - before.PromptTokens,
		CompletionTokens: u.CompletionTokens - before.CompletionTokens,
		TotalTokens:      u.TotalTokens - before.TotalTokens,
		Cost:             u.Cost - before.Cost,
	}
}

func (s runtimeEventStats) snapshot() RuntimeEventStats {
	return RuntimeEventStats{
		LastEventAt:      s.lastEventAt,
		MessageEvents:    s.messageEvents,
		SessionEvents:    s.sessionEvents,
		OtherEvents:      s.otherEvents,
		AssistantEvents:  s.assistantEvents,
		PermissionEvents: s.permissionEvents,
	}
}

func (r *RuntimeBridge) runtimeRequestsLocked() RuntimeRequests {
	var out RuntimeRequests
	now := time.Now().UnixMilli()
	for requestID, state := range r.requests {
		if state.Finished {
			continue
		}
		out.Running++
		if out.ActiveStartedAt == 0 || state.StartedAt < out.ActiveStartedAt {
			out.ActiveRequestID = requestID
			out.ActiveStartedAt = state.StartedAt
			out.ActiveDurationMS = now - state.StartedAt
		}
	}
	return out
}

func (r *RuntimeBridge) appendRuntimeEventLocked(event RuntimeEvent) {
	if event.CreatedAt == 0 {
		event.CreatedAt = time.Now().UnixMilli()
	}
	r.events = append(r.events, event)
	if len(r.events) > runtimeEventLimit {
		r.events = r.events[len(r.events)-runtimeEventLimit:]
	}
}

func toRuntimePermissionRequest(perm permission.PermissionRequest) RuntimePermissionRequest {
	return RuntimePermissionRequest{
		ID:          perm.ID,
		SessionID:   perm.SessionID,
		ToolCallID:  perm.ToolCallID,
		ToolName:    perm.ToolName,
		Description: perm.Description,
		Action:      perm.Action,
		Params:      perm.Params,
		Path:        perm.Path,
		CreatedAt:   time.Now().UnixMilli(),
	}
}

func toProtoPermissionRequest(perm permission.PermissionRequest) proto.PermissionRequest {
	return proto.PermissionRequest{
		ID:          perm.ID,
		SessionID:   perm.SessionID,
		ToolCallID:  perm.ToolCallID,
		ToolName:    perm.ToolName,
		Description: perm.Description,
		Action:      perm.Action,
		Params:      perm.Params,
		Path:        perm.Path,
	}
}

func (r *RuntimeBridge) writeAudit(entry auditEntry) {
	layout, err := resolveDesktopLayout()
	if err != nil {
		slog.Error("Failed to resolve desktop audit path", "error", err)
		return
	}
	if err := ensureDesktopLayout(layout); err != nil {
		slog.Error("Failed to create desktop audit directory", "error", err)
		return
	}
	if entry.Timestamp == "" {
		entry.Timestamp = time.Now().Format(time.RFC3339Nano)
	}
	path := filepath.Join(layout.LogsDir, "agent-builder-audit.jsonl")
	file, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o600)
	if err != nil {
		slog.Error("Failed to open desktop audit log", "path", path, "error", err)
		return
	}
	defer file.Close() //nolint:errcheck

	data, err := json.Marshal(entry)
	if err != nil {
		slog.Error("Failed to encode desktop audit entry", "error", err)
		return
	}
	if _, err := file.Write(append(data, '\n')); err != nil {
		slog.Error("Failed to write desktop audit entry", "path", path, "error", err)
	}
}

func preview(value string, limit int) string {
	value = strings.TrimSpace(value)
	if len(value) <= limit {
		return value
	}
	if limit <= 3 {
		return value[:limit]
	}
	return value[:limit-3] + "..."
}

func newRequestID() string {
	var data [8]byte
	if _, err := rand.Read(data[:]); err != nil {
		return fmt.Sprintf("%d", time.Now().UnixNano())
	}
	return fmt.Sprintf("%d-%s", time.Now().UnixMilli(), hex.EncodeToString(data[:]))
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
		result.Config = local
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

func applyDesktopProxy(result localModelConfigResult) {
	if !result.Applied {
		return
	}

	proxy := strings.TrimSpace(result.Config.Proxy)
	if proxy == "" {
		_ = os.Unsetenv("HTTP_PROXY")
		_ = os.Unsetenv("HTTPS_PROXY")
		_ = os.Unsetenv("http_proxy")
		_ = os.Unsetenv("https_proxy")
		return
	}

	_ = os.Setenv("HTTP_PROXY", proxy)
	_ = os.Setenv("HTTPS_PROXY", proxy)
	_ = os.Setenv("http_proxy", proxy)
	_ = os.Setenv("https_proxy", proxy)
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
		"has_proxy", hasDesktopProxy(),
	)
}

func hasDesktopProxy() bool {
	return os.Getenv("HTTPS_PROXY") != "" || os.Getenv("HTTP_PROXY") != "" || os.Getenv("https_proxy") != "" || os.Getenv("http_proxy") != ""
}
