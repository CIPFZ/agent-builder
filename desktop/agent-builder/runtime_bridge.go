package main

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"time"

	tea "charm.land/bubbletea/v2"
	"charm.land/catwalk/pkg/catwalk"
	mcptools "github.com/charmbracelet/crush/internal/agent/tools/mcp"
	"github.com/charmbracelet/crush/internal/backend"
	"github.com/charmbracelet/crush/internal/config"
	"github.com/charmbracelet/crush/internal/csync"
	"github.com/charmbracelet/crush/internal/db"
	crushlog "github.com/charmbracelet/crush/internal/log"
	"github.com/charmbracelet/crush/internal/message"
	"github.com/charmbracelet/crush/internal/permission"
	"github.com/charmbracelet/crush/internal/proto"
	"github.com/charmbracelet/crush/internal/pubsub"
	"github.com/charmbracelet/crush/internal/runtimeapi"
	"github.com/charmbracelet/crush/internal/session"
	"github.com/charmbracelet/crush/internal/skills"
	"github.com/charmbracelet/crush/internal/version"
)

// RuntimeService is the transport-neutral runtime boundary used by Wails and
// the local HTTP adapter.
type RuntimeService interface {
	Status(context.Context) (RuntimeStatus, error)
	Models(context.Context) (RuntimeModelsResponse, error)
	GetModelConfig(context.Context) (RuntimeConfigResponse, error)
	SaveModelConfig(context.Context, RuntimeModelConfig) (RuntimeConfigResponse, error)
	VerifyModelConfig(context.Context, RuntimeModelConfig) (RuntimeModelVerifyResponse, error)
	Chat(context.Context, RuntimeChatRequest) (RuntimeChatResponse, error)
	Messages(context.Context) (RuntimeMessagesResponse, error)
	Permissions(context.Context) (RuntimePermissionsResponse, error)
	Events(context.Context) (RuntimeEventsResponse, error)
	EventsEndpoint(context.Context) (RuntimeEventsEndpointResponse, error)
	SubscribeEvents(context.Context) (<-chan RuntimeEvent, func())
	AuditTurn(context.Context, string) (RuntimeAuditResponse, error)
	Skills(context.Context) (RuntimeSkillsResponse, error)
	RefreshSkills(context.Context) (RuntimeSkillsResponse, error)
	SetSkillEnabled(context.Context, RuntimeSkillToggleRequest) (RuntimeSkillsResponse, error)
	MCPServers(context.Context) (RuntimeMCPServersResponse, error)
	RefreshMCPServer(context.Context, string) (RuntimeMCPServersResponse, error)
	MCPTools(context.Context, string) (RuntimeMCPToolsResponse, error)
	MCPResources(context.Context, string) (RuntimeMCPResourcesResponse, error)
	MCPPrompts(context.Context, string) (RuntimeMCPPromptsResponse, error)
	Capabilities(context.Context) (RuntimeCapabilitiesResponse, error)
	DecidePermission(context.Context, RuntimePermissionDecision) (RuntimeStatus, error)
	Cancel(context.Context) (RuntimeStatus, error)
	NewChat(context.Context, string) (RuntimeStatus, error)
}

// RuntimeBridge is the Wails adapter. It intentionally delegates to
// RuntimeService so desktop bindings do not become the business boundary.
type RuntimeBridge struct {
	service RuntimeService
}

// runtimeService owns workspace, session, and agent lifecycle.
type runtimeService struct {
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
	eventStream *runtimeSSEServer
	httpAPI     *runtimeHTTPServer
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

type RuntimeEvent = runtimeapi.Event

type RuntimeEventsResponse struct {
	Events []RuntimeEvent `json:"events"`
}

type RuntimeEventsEndpointResponse struct {
	URL string `json:"url"`
}

type RuntimeSkill struct {
	Name          string `json:"name"`
	Description   string `json:"description,omitempty"`
	Builtin       bool   `json:"builtin"`
	Enabled       bool   `json:"enabled"`
	Path          string `json:"path,omitempty"`
	SkillFilePath string `json:"skill_file_path,omitempty"`
	State         string `json:"state"`
	Error         string `json:"error,omitempty"`
}

type RuntimeSkillsResponse struct {
	Skills []RuntimeSkill `json:"skills"`
}

type RuntimeSkillToggleRequest struct {
	Name    string `json:"name"`
	Enabled bool   `json:"enabled"`
}

type RuntimeMCPCounts struct {
	Tools     int `json:"tools"`
	Prompts   int `json:"prompts"`
	Resources int `json:"resources"`
}

type RuntimeMCPServer struct {
	Name          string            `json:"name"`
	Type          string            `json:"type"`
	URL           string            `json:"url,omitempty"`
	Command       string            `json:"command,omitempty"`
	Args          []string          `json:"args,omitempty"`
	Disabled      bool              `json:"disabled"`
	State         string            `json:"state"`
	Counts        RuntimeMCPCounts  `json:"counts"`
	Error         string            `json:"error,omitempty"`
	Env           map[string]string `json:"env,omitempty"`
	Headers       map[string]string `json:"headers,omitempty"`
	EnabledTools  []string          `json:"enabled_tools,omitempty"`
	DisabledTools []string          `json:"disabled_tools,omitempty"`
}

type RuntimeMCPServersResponse struct {
	Servers []RuntimeMCPServer `json:"servers"`
}

type RuntimeMCPTool struct {
	Server      string `json:"server"`
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
	Enabled     bool   `json:"enabled"`
	InputSchema any    `json:"input_schema,omitempty"`
}

type RuntimeMCPToolsResponse struct {
	Tools []RuntimeMCPTool `json:"tools"`
}

type RuntimeMCPResource struct {
	Server      string `json:"server"`
	URI         string `json:"uri"`
	Name        string `json:"name,omitempty"`
	Description string `json:"description,omitempty"`
	MIMEType    string `json:"mime_type,omitempty"`
}

type RuntimeMCPResourcesResponse struct {
	Resources []RuntimeMCPResource `json:"resources"`
}

type RuntimeMCPPrompt struct {
	Server      string `json:"server"`
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
}

type RuntimeMCPPromptsResponse struct {
	Prompts []RuntimeMCPPrompt `json:"prompts"`
}

type RuntimeCapability struct {
	ID          string `json:"id"`
	Kind        string `json:"kind"`
	Name        string `json:"name"`
	Source      string `json:"source,omitempty"`
	Enabled     bool   `json:"enabled"`
	Risk        string `json:"risk"`
	Description string `json:"description,omitempty"`
}

type RuntimeCapabilitiesResponse struct {
	Capabilities []RuntimeCapability `json:"capabilities"`
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

type RuntimeModelVerifyResponse struct {
	OK       bool   `json:"ok"`
	Protocol string `json:"protocol"`
	Model    string `json:"model"`
	Error    string `json:"error,omitempty"`
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
		service: NewRuntimeService(),
	}
}

func NewRuntimeService() RuntimeService {
	return newRuntimeService()
}

func newRuntimeService() *runtimeService {
	service := &runtimeService{
		requests:    make(map[string]runtimeRequestState),
		permissions: make(map[string]pendingRuntimePermission),
		eventStream: newRuntimeSSEServer(),
	}
	service.httpAPI = newRuntimeHTTPServer(service)
	return service
}

func (r *RuntimeBridge) Status(ctx context.Context) (RuntimeStatus, error) {
	return r.service.Status(ctx)
}

func (r *RuntimeBridge) Models(ctx context.Context) (RuntimeModelsResponse, error) {
	return r.service.Models(ctx)
}

func (r *RuntimeBridge) GetModelConfig(ctx context.Context) (RuntimeConfigResponse, error) {
	return r.service.GetModelConfig(ctx)
}

func (r *RuntimeBridge) SaveModelConfig(ctx context.Context, req RuntimeModelConfig) (RuntimeConfigResponse, error) {
	return r.service.SaveModelConfig(ctx, req)
}

func (r *RuntimeBridge) VerifyModelConfig(ctx context.Context, req RuntimeModelConfig) (RuntimeModelVerifyResponse, error) {
	return r.service.VerifyModelConfig(ctx, req)
}

func (r *RuntimeBridge) Chat(ctx context.Context, req RuntimeChatRequest) (RuntimeChatResponse, error) {
	return r.service.Chat(ctx, req)
}

func (r *RuntimeBridge) Messages(ctx context.Context) (RuntimeMessagesResponse, error) {
	return r.service.Messages(ctx)
}

func (r *RuntimeBridge) Permissions(ctx context.Context) (RuntimePermissionsResponse, error) {
	return r.service.Permissions(ctx)
}

func (r *RuntimeBridge) Events(ctx context.Context) (RuntimeEventsResponse, error) {
	return r.service.Events(ctx)
}

func (r *RuntimeBridge) EventsEndpoint(ctx context.Context) (RuntimeEventsEndpointResponse, error) {
	return r.service.EventsEndpoint(ctx)
}

func (r *RuntimeBridge) AuditTurn(ctx context.Context, turnID string) (RuntimeAuditResponse, error) {
	return r.service.AuditTurn(ctx, turnID)
}

func (r *RuntimeBridge) Skills(ctx context.Context) (RuntimeSkillsResponse, error) {
	return r.service.Skills(ctx)
}

func (r *RuntimeBridge) RefreshSkills(ctx context.Context) (RuntimeSkillsResponse, error) {
	return r.service.RefreshSkills(ctx)
}

func (r *RuntimeBridge) SetSkillEnabled(ctx context.Context, req RuntimeSkillToggleRequest) (RuntimeSkillsResponse, error) {
	return r.service.SetSkillEnabled(ctx, req)
}

func (r *RuntimeBridge) MCPServers(ctx context.Context) (RuntimeMCPServersResponse, error) {
	return r.service.MCPServers(ctx)
}

func (r *RuntimeBridge) RefreshMCPServer(ctx context.Context, name string) (RuntimeMCPServersResponse, error) {
	return r.service.RefreshMCPServer(ctx, name)
}

func (r *RuntimeBridge) MCPTools(ctx context.Context, name string) (RuntimeMCPToolsResponse, error) {
	return r.service.MCPTools(ctx, name)
}

func (r *RuntimeBridge) MCPResources(ctx context.Context, name string) (RuntimeMCPResourcesResponse, error) {
	return r.service.MCPResources(ctx, name)
}

func (r *RuntimeBridge) MCPPrompts(ctx context.Context, name string) (RuntimeMCPPromptsResponse, error) {
	return r.service.MCPPrompts(ctx, name)
}

func (r *RuntimeBridge) Capabilities(ctx context.Context) (RuntimeCapabilitiesResponse, error) {
	return r.service.Capabilities(ctx)
}

func (r *RuntimeBridge) APIEndpoint(ctx context.Context) (RuntimeAPIEndpointResponse, error) {
	service, ok := r.service.(interface {
		APIEndpoint(context.Context) (RuntimeAPIEndpointResponse, error)
	})
	if !ok {
		return RuntimeAPIEndpointResponse{}, errors.New("runtime service does not expose an HTTP API endpoint")
	}
	return service.APIEndpoint(ctx)
}

func (r *RuntimeBridge) DecidePermission(ctx context.Context, req RuntimePermissionDecision) (RuntimeStatus, error) {
	return r.service.DecidePermission(ctx, req)
}

func (r *RuntimeBridge) Cancel(ctx context.Context) (RuntimeStatus, error) {
	return r.service.Cancel(ctx)
}

func (r *RuntimeBridge) NewChat(ctx context.Context, title string) (RuntimeStatus, error) {
	return r.service.NewChat(ctx, title)
}

func (r *runtimeService) Status(ctx context.Context) (RuntimeStatus, error) {
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

func (r *runtimeService) Models(ctx context.Context) (RuntimeModelsResponse, error) {
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

func (r *runtimeService) GetModelConfig(ctx context.Context) (RuntimeConfigResponse, error) {
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

func (r *runtimeService) SaveModelConfig(ctx context.Context, req RuntimeModelConfig) (RuntimeConfigResponse, error) {
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

func (r *runtimeService) VerifyModelConfig(_ context.Context, req RuntimeModelConfig) (RuntimeModelVerifyResponse, error) {
	cfg := RuntimeModelConfig{
		Protocol: strings.TrimSpace(req.Protocol),
		URL:      strings.TrimSpace(req.URL),
		APIKey:   strings.TrimSpace(req.APIKey),
		Model:    strings.TrimSpace(req.Model),
		Proxy:    strings.TrimSpace(req.Proxy),
	}
	if cfg.Model != "" {
		cfg.Models = []string{cfg.Model}
	}
	if err := validateModelConfig(cfg); err != nil {
		return RuntimeModelVerifyResponse{}, err
	}
	store := config.NewTestStore(&config.Config{
		Providers: csync.NewMap[string, config.ProviderConfig](),
		Models:    map[config.SelectedModelType]config.SelectedModel{},
		Options:   &config.Options{},
	})
	applyModelConfig(store, cfg)
	provider, ok := store.Config().Providers.Get(localProviderID)
	if !ok {
		return RuntimeModelVerifyResponse{}, errors.New("model provider was not configured")
	}
	if err := provider.TestConnection(store.Resolver()); err != nil {
		return RuntimeModelVerifyResponse{
			OK:       false,
			Protocol: cfg.Protocol,
			Model:    cfg.Model,
			Error:    err.Error(),
		}, nil
	}
	return RuntimeModelVerifyResponse{
		OK:       true,
		Protocol: cfg.Protocol,
		Model:    cfg.Model,
	}, nil
}

func (r *runtimeService) Chat(ctx context.Context, req RuntimeChatRequest) (RuntimeChatResponse, error) {
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

func (r *runtimeService) Messages(ctx context.Context) (RuntimeMessagesResponse, error) {
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

func (r *runtimeService) Permissions(ctx context.Context) (RuntimePermissionsResponse, error) {
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

func (r *runtimeService) Events(ctx context.Context) (RuntimeEventsResponse, error) {
	if err := r.ensureStarted(ctx); err != nil {
		return RuntimeEventsResponse{}, err
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	events := make([]RuntimeEvent, len(r.events))
	copy(events, r.events)
	return RuntimeEventsResponse{Events: events}, nil
}

func (r *runtimeService) EventsEndpoint(_ context.Context) (RuntimeEventsEndpointResponse, error) {
	if err := r.ensureEventStream(); err != nil {
		return RuntimeEventsEndpointResponse{}, err
	}
	return RuntimeEventsEndpointResponse{URL: r.eventStream.URL()}, nil
}

func (r *runtimeService) SubscribeEvents(_ context.Context) (<-chan RuntimeEvent, func()) {
	events := make(chan RuntimeEvent, 64)
	if r.eventStream == nil {
		r.eventStream = newRuntimeSSEServer()
	}
	r.eventStream.addSubscriber(events)
	return events, func() {
		r.eventStream.removeSubscriber(events)
	}
}

func (r *runtimeService) AuditTurn(ctx context.Context, turnID string) (RuntimeAuditResponse, error) {
	db, err := r.workspaceDB(ctx)
	if err != nil {
		return RuntimeAuditResponse{}, err
	}
	return newRuntimeAuditStore(db).ListTurn(ctx, strings.TrimSpace(turnID))
}

func (r *runtimeService) Skills(ctx context.Context) (RuntimeSkillsResponse, error) {
	if err := r.ensureStarted(ctx); err != nil {
		return RuntimeSkillsResponse{}, err
	}
	return r.refreshSkills(ctx, false)
}

func (r *runtimeService) RefreshSkills(ctx context.Context) (RuntimeSkillsResponse, error) {
	if err := r.ensureStarted(ctx); err != nil {
		return RuntimeSkillsResponse{}, err
	}
	return r.refreshSkills(ctx, true)
}

func (r *runtimeService) SetSkillEnabled(ctx context.Context, req RuntimeSkillToggleRequest) (RuntimeSkillsResponse, error) {
	if err := r.ensureStarted(ctx); err != nil {
		return RuntimeSkillsResponse{}, err
	}
	name := strings.TrimSpace(req.Name)
	if name == "" {
		return RuntimeSkillsResponse{}, errors.New("skill name is required")
	}

	r.mu.Lock()
	wsID := r.workspace.ID
	r.mu.Unlock()

	ws, err := r.runtime.GetWorkspace(wsID)
	if err != nil {
		return RuntimeSkillsResponse{}, err
	}
	cfg := ws.Cfg
	if cfg.Config().Options == nil {
		cfg.Config().Options = &config.Options{}
	}
	disabled := slices.DeleteFunc(slices.Clone(cfg.Config().Options.DisabledSkills), func(existing string) bool {
		return existing == name
	})
	if !req.Enabled {
		disabled = append(disabled, name)
	}
	slices.Sort(disabled)
	disabled = slices.Compact(disabled)
	cfg.Config().Options.DisabledSkills = disabled
	if err := cfg.SetConfigField(config.ScopeGlobal, "options.disabled_skills", disabled); err != nil {
		return RuntimeSkillsResponse{}, fmt.Errorf("failed to persist disabled skills: %w", err)
	}

	eventType := runtimeapi.EventSkillEnabled
	if !req.Enabled {
		eventType = runtimeapi.EventSkillDisabled
	}
	r.publishRuntimeEvent(runtimeapi.Event{
		ID:        newRuntimeEventID(),
		Type:      eventType,
		CreatedAt: time.Now().UTC().Format(time.RFC3339Nano),
		Payload: map[string]any{
			"name": name,
		},
	})
	return r.refreshSkills(ctx, true)
}

func (r *runtimeService) MCPServers(ctx context.Context) (RuntimeMCPServersResponse, error) {
	cfg, _, err := r.workspaceConfig(ctx)
	if err != nil {
		return RuntimeMCPServersResponse{}, err
	}
	return runtimeMCPServersFromConfig(cfg), nil
}

func (r *runtimeService) RefreshMCPServer(ctx context.Context, name string) (RuntimeMCPServersResponse, error) {
	cfg, wsID, err := r.workspaceConfig(ctx)
	if err != nil {
		return RuntimeMCPServersResponse{}, err
	}
	name = strings.TrimSpace(name)
	if name == "" {
		return RuntimeMCPServersResponse{}, errors.New("mcp server name is required")
	}
	r.runtime.RefreshMCPTools(ctx, wsID, name)
	r.runtime.MCPRefreshPrompts(ctx, wsID, name)
	r.runtime.MCPRefreshResources(ctx, wsID, name)
	r.publishRuntimeEvent(runtimeapi.Event{
		ID:        newRuntimeEventID(),
		Type:      runtimeapi.EventMCPToolsUpdated,
		CreatedAt: time.Now().UTC().Format(time.RFC3339Nano),
		Payload: map[string]any{
			"name": name,
		},
	})
	return runtimeMCPServersFromConfig(cfg), nil
}

func (r *runtimeService) MCPTools(ctx context.Context, name string) (RuntimeMCPToolsResponse, error) {
	cfg, _, err := r.workspaceConfig(ctx)
	if err != nil {
		return RuntimeMCPToolsResponse{}, err
	}
	return runtimeMCPToolsFromConfig(cfg, strings.TrimSpace(name)), nil
}

func (r *runtimeService) MCPResources(ctx context.Context, name string) (RuntimeMCPResourcesResponse, error) {
	if _, _, err := r.workspaceConfig(ctx); err != nil {
		return RuntimeMCPResourcesResponse{}, err
	}
	return runtimeMCPResources(strings.TrimSpace(name)), nil
}

func (r *runtimeService) MCPPrompts(ctx context.Context, name string) (RuntimeMCPPromptsResponse, error) {
	if _, _, err := r.workspaceConfig(ctx); err != nil {
		return RuntimeMCPPromptsResponse{}, err
	}
	return runtimeMCPPrompts(strings.TrimSpace(name)), nil
}

func (r *runtimeService) Capabilities(ctx context.Context) (RuntimeCapabilitiesResponse, error) {
	cfg, _, err := r.workspaceConfig(ctx)
	if err != nil {
		return RuntimeCapabilitiesResponse{}, err
	}
	skills := runtimeSkillsFromConfig(cfg)
	tools := runtimeMCPToolsFromConfig(cfg, "")
	resources := runtimeMCPResources("")
	prompts := runtimeMCPPrompts("")
	return runtimeCapabilities(cfg, skills, tools, resources, prompts), nil
}

func (r *runtimeService) workspaceConfig(ctx context.Context) (*config.ConfigStore, string, error) {
	if err := r.ensureStarted(ctx); err != nil {
		return nil, "", err
	}
	r.mu.Lock()
	wsID := r.workspace.ID
	r.mu.Unlock()
	ws, err := r.runtime.GetWorkspace(wsID)
	if err != nil {
		return nil, "", err
	}
	return ws.Cfg, wsID, nil
}

func (r *runtimeService) workspaceDB(ctx context.Context) (*sql.DB, error) {
	cfg, _, err := r.workspaceConfig(ctx)
	if err != nil {
		return nil, err
	}
	conn, err := db.Connect(ctx, cfg.Config().Options.DataDirectory)
	if err != nil {
		return nil, err
	}
	return conn, nil
}

func (r *runtimeService) refreshSkills(ctx context.Context, publish bool) (RuntimeSkillsResponse, error) {
	if publish {
		r.publishRuntimeEvent(runtimeapi.NewEvent(newRuntimeEventID(), runtimeapi.EventSkillDiscoveryStarted, time.Now()))
	}

	r.mu.Lock()
	wsID := r.workspace.ID
	r.mu.Unlock()

	ws, err := r.runtime.GetWorkspace(wsID)
	if err != nil {
		return RuntimeSkillsResponse{}, err
	}
	resp := runtimeSkillsFromConfig(ws.Cfg)
	if publish {
		eventType := runtimeapi.EventSkillDiscoveryCompleted
		for _, skill := range resp.Skills {
			if skill.State == "error" {
				eventType = runtimeapi.EventSkillDiscoveryFailed
				break
			}
		}
		event := runtimeapi.NewEvent(newRuntimeEventID(), eventType, time.Now())
		event.Payload = map[string]any{
			"count": len(resp.Skills),
		}
		r.publishRuntimeEvent(event)
	}
	return resp, nil
}

func (r *runtimeService) APIEndpoint(_ context.Context) (RuntimeAPIEndpointResponse, error) {
	if r.httpAPI == nil {
		r.httpAPI = newRuntimeHTTPServer(r)
	}
	if err := r.httpAPI.Start(); err != nil {
		return RuntimeAPIEndpointResponse{}, err
	}
	return RuntimeAPIEndpointResponse{
		URL:   r.httpAPI.URL(),
		Token: r.httpAPI.Token(),
	}, nil
}

func (r *runtimeService) DecidePermission(ctx context.Context, req RuntimePermissionDecision) (RuntimeStatus, error) {
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

func (r *runtimeService) Cancel(ctx context.Context) (RuntimeStatus, error) {
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

func (r *runtimeService) runChat(ctx context.Context, requestID, wsID, sessionID, prompt string, start time.Time, usageBefore RuntimeUsage, provider, model string) {
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

func (r *runtimeService) readConfiguredModel() (RuntimeModelConfig, error) {
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

func (r *runtimeService) NewChat(ctx context.Context, title string) (RuntimeStatus, error) {
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

func (r *runtimeService) restart() {
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

func (r *runtimeService) ensureStarted(ctx context.Context) error {
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

func (r *runtimeService) consumeRuntimeEvents(ctx context.Context, workspaceID string) {
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

func (r *runtimeService) consumeDesktopPermissions(ctx context.Context, workspaceID string, permissions permission.Service) {
	events := permissions.Subscribe(ctx)
	for {
		select {
		case event, ok := <-events:
			if !ok {
				return
			}
			perm := event.Payload
			runtimePerm := toRuntimePermissionRequest(perm)
			var runtimeEvent RuntimeEvent
			now := time.Now()
			r.mu.Lock()
			r.permissions[perm.ID] = pendingRuntimePermission{
				Permission: runtimePerm,
				Raw:        perm,
			}
			r.eventStats.permissionEvents++
			r.eventStats.lastEventAt = now.UnixMilli()
			runtimeEvent = r.appendRuntimeEventLocked(newPermissionRuntimeEvent(now, runtimePerm))
			r.mu.Unlock()
			r.publishRuntimeEvent(runtimeEvent)

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

func (r *runtimeService) recordRuntimeEvent(event pubsub.Event[tea.Msg]) {
	r.mu.Lock()

	var runtimeEvent RuntimeEvent
	now := time.Now()
	r.eventStats.lastEventAt = now.UnixMilli()
	switch payload := event.Payload.(type) {
	case pubsub.Event[message.Message]:
		r.eventStats.messageEvents++
		runtimeEvent = newMessageRuntimeEvent(now, toProtoMessage(payload.Payload))
		runtimeEvent = r.appendRuntimeEventLocked(runtimeEvent)
		if payload.Payload.Role == message.Assistant {
			r.eventStats.assistantEvents++
		}
	case pubsub.Event[proto.Message]:
		r.eventStats.messageEvents++
		runtimeEvent = newMessageRuntimeEvent(now, payload.Payload)
		runtimeEvent = r.appendRuntimeEventLocked(runtimeEvent)
		if payload.Payload.Role == proto.Assistant {
			r.eventStats.assistantEvents++
		}
	case pubsub.Event[proto.Session]:
		r.eventStats.sessionEvents++
		runtimeEvent = newSessionRuntimeEvent(now, payload.Payload.ID, payload.Payload.Title)
		runtimeEvent = r.appendRuntimeEventLocked(runtimeEvent)
	case pubsub.Event[session.Session]:
		r.eventStats.sessionEvents++
		runtimeEvent = newSessionRuntimeEvent(now, payload.Payload.ID, payload.Payload.Title)
		runtimeEvent = r.appendRuntimeEventLocked(runtimeEvent)
	case pubsub.Event[permission.PermissionRequest]:
		r.eventStats.permissionEvents++
	case pubsub.Event[proto.PermissionRequest]:
		r.eventStats.permissionEvents++
	default:
		r.eventStats.otherEvents++
	}
	r.mu.Unlock()
	r.publishRuntimeEvent(runtimeEvent)
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

func (r *runtimeService) latestFinishedAssistantMessage(ctx context.Context, workspaceID, sessionID string) (proto.Message, error) {
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

func (r *runtimeService) sessionUsage(ctx context.Context, workspaceID, sessionID string) (RuntimeUsage, error) {
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

func (r *runtimeService) runtimeRequestsLocked() RuntimeRequests {
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

func (r *runtimeService) appendRuntimeEventLocked(event RuntimeEvent) RuntimeEvent {
	if event.ID == "" {
		event.ID = newRuntimeEventID()
	}
	if event.CreatedAt == "" {
		event.CreatedAt = time.Now().UTC().Format(time.RFC3339Nano)
	}
	r.events = append(r.events, event)
	if len(r.events) > runtimeEventLimit {
		r.events = r.events[len(r.events)-runtimeEventLimit:]
	}
	return event
}

func (r *runtimeService) ensureEventStream() error {
	if r.eventStream == nil {
		r.eventStream = newRuntimeSSEServer()
	}
	return r.eventStream.Start()
}

func (r *runtimeService) publishRuntimeEvent(event RuntimeEvent) {
	if event.Type == "" || r.eventStream == nil {
		return
	}
	if err := r.ensureEventStream(); err != nil {
		slog.Error("Failed to start desktop runtime SSE stream", "error", err)
		return
	}
	r.eventStream.Publish(event)
}

func newMessageRuntimeEvent(createdAt time.Time, msg proto.Message) RuntimeEvent {
	eventType := runtimeapi.EventMessageUpdated
	if msg.FinishPart() != nil {
		eventType = runtimeapi.EventMessageCompleted
	}
	event := runtimeapi.NewEvent(newRuntimeEventID(), eventType, createdAt)
	event.SessionID = msg.SessionID
	event.MessageID = msg.ID
	event.Payload = map[string]any{
		"role":    string(msg.Role),
		"summary": preview(msg.Content().Text, 160),
	}
	return event
}

func newSessionRuntimeEvent(createdAt time.Time, sessionID, title string) RuntimeEvent {
	event := runtimeapi.NewEvent(newRuntimeEventID(), runtimeapi.EventSessionUpdated, createdAt)
	event.SessionID = sessionID
	event.Payload = map[string]any{
		"title": title,
	}
	return event
}

func newPermissionRuntimeEvent(createdAt time.Time, perm RuntimePermissionRequest) RuntimeEvent {
	event := runtimeapi.NewEvent(newRuntimeEventID(), runtimeapi.EventPermissionRequested, createdAt)
	event.SessionID = perm.SessionID
	event.ToolCallID = perm.ToolCallID
	event.Payload = map[string]any{
		"permission_id": perm.ID,
		"tool_name":     perm.ToolName,
		"action":        perm.Action,
		"description":   perm.Description,
		"path":          perm.Path,
		"summary":       perm.ToolName + ":" + perm.Action,
	}
	return event
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

func runtimeSkillsFromConfig(store *config.ConfigStore) RuntimeSkillsResponse {
	opts := store.Config().Options
	builtin, builtinStates := skills.DiscoverBuiltinWithStates()
	discovered := append([]*skills.Skill(nil), builtin...)
	states := append([]*skills.SkillState(nil), builtinStates...)

	if opts != nil && len(opts.SkillsPaths) > 0 {
		paths := make([]string, 0, len(opts.SkillsPaths))
		for _, path := range opts.SkillsPaths {
			expanded := path
			if strings.HasPrefix(expanded, "$") {
				if resolved, err := store.Resolver().ResolveValue(expanded); err == nil {
					expanded = resolved
				}
			}
			paths = append(paths, expanded)
		}
		userSkills, userStates := skills.DiscoverWithStates(paths)
		discovered = append(discovered, userSkills...)
		states = append(states, userStates...)
	}

	allSkills := skills.Deduplicate(discovered)
	states = skills.DeduplicateStates(states)
	skillByName := make(map[string]*skills.Skill, len(allSkills))
	for _, skill := range allSkills {
		skillByName[skill.Name] = skill
	}

	var disabled []string
	if opts != nil {
		disabled = opts.DisabledSkills
	}
	disabledSet := make(map[string]bool, len(disabled))
	for _, name := range disabled {
		disabledSet[name] = true
	}

	result := make([]RuntimeSkill, 0, len(states))
	for _, state := range states {
		runtimeSkill := RuntimeSkill{
			Name:  state.Name,
			Path:  state.Path,
			State: "normal",
		}
		if state.State == skills.StateError {
			runtimeSkill.State = "error"
			if state.Err != nil {
				runtimeSkill.Error = state.Err.Error()
			}
		}
		if skill := skillByName[state.Name]; skill != nil {
			runtimeSkill.Name = skill.Name
			runtimeSkill.Description = skill.Description
			runtimeSkill.Builtin = skill.Builtin
			runtimeSkill.Enabled = !disabledSet[skill.Name] && runtimeSkill.State != "error"
			runtimeSkill.Path = skill.Path
			runtimeSkill.SkillFilePath = skill.SkillFilePath
		}
		result = append(result, runtimeSkill)
	}

	slices.SortStableFunc(result, func(a, b RuntimeSkill) int {
		if c := strings.Compare(strings.ToLower(a.Name), strings.ToLower(b.Name)); c != 0 {
			return c
		}
		return strings.Compare(strings.ToLower(a.Path), strings.ToLower(b.Path))
	})
	return RuntimeSkillsResponse{Skills: result}
}

func runtimeMCPServersFromConfig(store *config.ConfigStore) RuntimeMCPServersResponse {
	states := mcptools.GetStates()
	servers := make([]RuntimeMCPServer, 0, len(store.Config().MCP))
	for _, item := range store.Config().MCP.Sorted() {
		cfg := item.MCP
		state := "disabled"
		var counts RuntimeMCPCounts
		var errorText string
		if !cfg.Disabled {
			state = "starting"
			if info, ok := states[item.Name]; ok {
				state = info.State.String()
				counts = RuntimeMCPCounts{
					Tools:     info.Counts.Tools,
					Prompts:   info.Counts.Prompts,
					Resources: info.Counts.Resources,
				}
				if info.Error != nil {
					errorText = info.Error.Error()
				}
			}
		}
		servers = append(servers, RuntimeMCPServer{
			Name:          item.Name,
			Type:          string(cfg.Type),
			URL:           redactURL(cfg.URL),
			Command:       cfg.Command,
			Args:          slices.Clone(cfg.Args),
			Disabled:      cfg.Disabled,
			State:         state,
			Counts:        counts,
			Error:         errorText,
			Env:           redactMap(cfg.Env),
			Headers:       redactMap(cfg.Headers),
			EnabledTools:  slices.Clone(cfg.EnabledTools),
			DisabledTools: slices.Clone(cfg.DisabledTools),
		})
	}
	return RuntimeMCPServersResponse{Servers: servers}
}

func runtimeMCPToolsFromConfig(store *config.ConfigStore, server string) RuntimeMCPToolsResponse {
	var tools []RuntimeMCPTool
	for name, serverTools := range mcptools.Tools() {
		if server != "" && name != server {
			continue
		}
		cfg := store.Config().MCP[name]
		for _, tool := range serverTools {
			tools = append(tools, RuntimeMCPTool{
				Server:      name,
				Name:        tool.Name,
				Description: tool.Description,
				Enabled:     mcpToolEnabled(cfg, tool.Name),
				InputSchema: tool.InputSchema,
			})
		}
	}
	slices.SortStableFunc(tools, func(a, b RuntimeMCPTool) int {
		if c := strings.Compare(a.Server, b.Server); c != 0 {
			return c
		}
		return strings.Compare(a.Name, b.Name)
	})
	return RuntimeMCPToolsResponse{Tools: tools}
}

func runtimeMCPResources(server string) RuntimeMCPResourcesResponse {
	var resources []RuntimeMCPResource
	for name, serverResources := range mcptools.Resources() {
		if server != "" && name != server {
			continue
		}
		for _, resource := range serverResources {
			resources = append(resources, RuntimeMCPResource{
				Server:      name,
				URI:         resource.URI,
				Name:        resource.Name,
				Description: resource.Description,
				MIMEType:    resource.MIMEType,
			})
		}
	}
	return RuntimeMCPResourcesResponse{Resources: resources}
}

func runtimeMCPPrompts(server string) RuntimeMCPPromptsResponse {
	var prompts []RuntimeMCPPrompt
	for name, serverPrompts := range mcptools.Prompts() {
		if server != "" && name != server {
			continue
		}
		for _, prompt := range serverPrompts {
			prompts = append(prompts, RuntimeMCPPrompt{
				Server:      name,
				Name:        prompt.Name,
				Description: prompt.Description,
			})
		}
	}
	return RuntimeMCPPromptsResponse{Prompts: prompts}
}

func runtimeCapabilities(
	store *config.ConfigStore,
	skills RuntimeSkillsResponse,
	mcpTools RuntimeMCPToolsResponse,
	mcpResources RuntimeMCPResourcesResponse,
	mcpPrompts RuntimeMCPPromptsResponse,
) RuntimeCapabilitiesResponse {
	var capabilities []RuntimeCapability
	disabledTools := map[string]bool{}
	if store.Config().Options != nil {
		for _, name := range store.Config().Options.DisabledTools {
			disabledTools[name] = true
		}
	}
	for _, tool := range builtinToolCapabilities() {
		tool.Enabled = !disabledTools[tool.Name]
		capabilities = append(capabilities, tool)
	}
	for _, skill := range skills.Skills {
		capabilities = append(capabilities, RuntimeCapability{
			ID:          "skill:" + skill.Name,
			Kind:        "skill",
			Name:        skill.Name,
			Source:      skill.Path,
			Enabled:     skill.Enabled,
			Risk:        "context",
			Description: skill.Description,
		})
	}
	for _, tool := range mcpTools.Tools {
		capabilities = append(capabilities, RuntimeCapability{
			ID:          "mcp:" + tool.Server + ":" + tool.Name,
			Kind:        "mcp_tool",
			Name:        tool.Name,
			Source:      tool.Server,
			Enabled:     tool.Enabled,
			Risk:        "external",
			Description: tool.Description,
		})
	}
	for _, resource := range mcpResources.Resources {
		capabilities = append(capabilities, RuntimeCapability{
			ID:          "mcp_resource:" + resource.Server + ":" + resource.URI,
			Kind:        "mcp_resource",
			Name:        firstNonEmpty(resource.Name, resource.URI),
			Source:      resource.Server,
			Enabled:     true,
			Risk:        "read",
			Description: resource.Description,
		})
	}
	for _, prompt := range mcpPrompts.Prompts {
		capabilities = append(capabilities, RuntimeCapability{
			ID:          "mcp_prompt:" + prompt.Server + ":" + prompt.Name,
			Kind:        "mcp_prompt",
			Name:        prompt.Name,
			Source:      prompt.Server,
			Enabled:     true,
			Risk:        "context",
			Description: prompt.Description,
		})
	}
	slices.SortStableFunc(capabilities, func(a, b RuntimeCapability) int {
		if c := strings.Compare(a.Kind, b.Kind); c != 0 {
			return c
		}
		return strings.Compare(a.ID, b.ID)
	})
	return RuntimeCapabilitiesResponse{Capabilities: capabilities}
}

func builtinToolCapabilities() []RuntimeCapability {
	return []RuntimeCapability{
		{ID: "builtin:bash", Kind: "builtin_tool", Name: "bash", Enabled: true, Risk: "write", Description: "Run shell commands."},
		{ID: "builtin:crush_info", Kind: "builtin_tool", Name: "crush_info", Enabled: true, Risk: "read", Description: "Inspect runtime configuration."},
		{ID: "builtin:crush_logs", Kind: "builtin_tool", Name: "crush_logs", Enabled: true, Risk: "read", Description: "Inspect runtime logs."},
		{ID: "builtin:diagnostics", Kind: "builtin_tool", Name: "diagnostics", Enabled: true, Risk: "read", Description: "Read LSP diagnostics."},
		{ID: "builtin:download", Kind: "builtin_tool", Name: "download", Enabled: true, Risk: "write", Description: "Download a URL to a file."},
		{ID: "builtin:edit", Kind: "builtin_tool", Name: "edit", Enabled: true, Risk: "write", Description: "Edit a file."},
		{ID: "builtin:fetch", Kind: "builtin_tool", Name: "fetch", Enabled: true, Risk: "read", Description: "Fetch URL content."},
		{ID: "builtin:glob", Kind: "builtin_tool", Name: "glob", Enabled: true, Risk: "read", Description: "Find files by glob."},
		{ID: "builtin:grep", Kind: "builtin_tool", Name: "grep", Enabled: true, Risk: "read", Description: "Search file contents."},
		{ID: "builtin:job_kill", Kind: "builtin_tool", Name: "job_kill", Enabled: true, Risk: "write", Description: "Stop a background job."},
		{ID: "builtin:job_output", Kind: "builtin_tool", Name: "job_output", Enabled: true, Risk: "read", Description: "Read background job output."},
		{ID: "builtin:list_mcp_resources", Kind: "builtin_tool", Name: "list_mcp_resources", Enabled: true, Risk: "read", Description: "List MCP resources."},
		{ID: "builtin:ls", Kind: "builtin_tool", Name: "ls", Enabled: true, Risk: "read", Description: "List directory contents."},
		{ID: "builtin:lsp_restart", Kind: "builtin_tool", Name: "lsp_restart", Enabled: true, Risk: "write", Description: "Restart an LSP server."},
		{ID: "builtin:multiedit", Kind: "builtin_tool", Name: "multiedit", Enabled: true, Risk: "write", Description: "Apply multiple file edits."},
		{ID: "builtin:read_mcp_resource", Kind: "builtin_tool", Name: "read_mcp_resource", Enabled: true, Risk: "read", Description: "Read an MCP resource."},
		{ID: "builtin:references", Kind: "builtin_tool", Name: "references", Enabled: true, Risk: "read", Description: "Find symbol references."},
		{ID: "builtin:sourcegraph", Kind: "builtin_tool", Name: "sourcegraph", Enabled: true, Risk: "read", Description: "Search Sourcegraph."},
		{ID: "builtin:todos", Kind: "builtin_tool", Name: "todos", Enabled: true, Risk: "write", Description: "Track todo items."},
		{ID: "builtin:view", Kind: "builtin_tool", Name: "view", Enabled: true, Risk: "read", Description: "Read files."},
		{ID: "builtin:web_fetch", Kind: "builtin_tool", Name: "web_fetch", Enabled: true, Risk: "read", Description: "Fetch web content."},
		{ID: "builtin:web_search", Kind: "builtin_tool", Name: "web_search", Enabled: true, Risk: "read", Description: "Search the web."},
		{ID: "builtin:write", Kind: "builtin_tool", Name: "write", Enabled: true, Risk: "write", Description: "Write a file."},
	}
}

func mcpToolEnabled(cfg config.MCPConfig, name string) bool {
	if len(cfg.EnabledTools) > 0 && !slices.Contains(cfg.EnabledTools, name) {
		return false
	}
	return !slices.Contains(cfg.DisabledTools, name)
}

func redactMap(values map[string]string) map[string]string {
	if len(values) == 0 {
		return nil
	}
	redacted := make(map[string]string, len(values))
	for key, value := range values {
		if shouldRedact(key, value) {
			redacted[key] = "[REDACTED]"
			continue
		}
		redacted[key] = value
	}
	return redacted
}

func redactURL(raw string) string {
	if raw == "" {
		return ""
	}
	if strings.Contains(raw, "@") {
		return "[REDACTED_URL]"
	}
	return raw
}

func shouldRedact(key, value string) bool {
	normalized := strings.ToLower(key + " " + value)
	for _, marker := range []string{"authorization", "api_key", "apikey", "token", "secret", "password", "bearer "} {
		if strings.Contains(normalized, marker) {
			return true
		}
	}
	return false
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

func (r *runtimeService) writeAudit(entry auditEntry) {
	r.writeRuntimeAuditEvent(context.Background(), entry)

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

func (r *runtimeService) writeRuntimeAuditEvent(ctx context.Context, entry auditEntry) {
	db, err := r.workspaceDB(ctx)
	if err != nil {
		slog.Debug("Runtime audit database unavailable", "error", err)
		return
	}
	payload, err := auditPayload(entry)
	if err != nil {
		slog.Error("Failed to prepare runtime audit payload", "error", err)
		return
	}
	event := RuntimeAuditEvent{
		ID:        newRuntimeEventID(),
		SessionID: entry.SessionID,
		TurnID:    firstNonEmpty(entry.RequestID, entry.SessionID),
		Type:      entry.Event,
		CreatedAt: firstNonEmpty(entry.Timestamp, time.Now().UTC().Format(time.RFC3339Nano)),
		Payload:   payload,
	}
	if err := newRuntimeAuditStore(db).Append(ctx, event); err != nil {
		slog.Error("Failed to write runtime audit event", "error", err)
	}
}

func auditPayload(entry auditEntry) (map[string]any, error) {
	data, err := json.Marshal(entry)
	if err != nil {
		return nil, err
	}
	var payload map[string]any
	if err := json.Unmarshal(data, &payload); err != nil {
		return nil, err
	}
	return payload, nil
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

func newRuntimeEventID() string {
	return "evt_" + newRequestID()
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
