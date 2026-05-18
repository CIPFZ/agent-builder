package main

import (
	"bufio"
	"context"
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/charmbracelet/crush/internal/config"
	"github.com/charmbracelet/crush/internal/csync"
	"github.com/charmbracelet/crush/internal/permission"
	"github.com/charmbracelet/crush/internal/proto"
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
		ToolCallID:  "tool-1",
		ToolName:    "bash",
		Description: "Run a command",
		Action:      "execute",
		Params:      map[string]any{"command": "pwd"},
		Path:        "C:\\work",
	}

	runtimePerm := toRuntimePermissionRequest(perm)
	if runtimePerm.ID != perm.ID || runtimePerm.ToolName != perm.ToolName || runtimePerm.Action != perm.Action {
		t.Fatalf("runtime permission mapping failed: %#v", runtimePerm)
	}
	if runtimePerm.CreatedAt == 0 {
		t.Fatal("runtime permission CreatedAt was not set")
	}

	protoPerm := toProtoPermissionRequest(perm)
	if protoPerm.ID != perm.ID || protoPerm.ToolCallID != perm.ToolCallID || protoPerm.Path != perm.Path {
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
	if event.Type != "message.created" || event.MessageID != "message-1" {
		t.Fatalf("event = %#v", event)
	}
	if len(service.events) != 1 {
		t.Fatalf("stored events = %d, want 1", len(service.events))
	}
}

func TestRuntimeBridgeDelegatesToRuntimeService(t *testing.T) {
	t.Parallel()

	service := &recordingRuntimeService{}
	bridge := &RuntimeBridge{service: service}

	if _, err := bridge.Chat(context.Background(), RuntimeChatRequest{Prompt: "hello"}); err != nil {
		t.Fatal(err)
	}
	if service.chatCalls != 1 {
		t.Fatalf("chatCalls = %d, want 1", service.chatCalls)
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
	chatCalls   int
	statusCalls int
	status      RuntimeStatus
}

func (s *recordingRuntimeService) Status(context.Context) (RuntimeStatus, error) {
	s.statusCalls++
	return s.status, nil
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

func (s *recordingRuntimeService) Chat(context.Context, RuntimeChatRequest) (RuntimeChatResponse, error) {
	s.chatCalls++
	return RuntimeChatResponse{RequestID: "request-1"}, nil
}

func (s *recordingRuntimeService) Messages(context.Context) (RuntimeMessagesResponse, error) {
	return RuntimeMessagesResponse{}, nil
}

func (s *recordingRuntimeService) Permissions(context.Context) (RuntimePermissionsResponse, error) {
	return RuntimePermissionsResponse{}, nil
}

func (s *recordingRuntimeService) Events(context.Context) (RuntimeEventsResponse, error) {
	return RuntimeEventsResponse{}, nil
}

func (s *recordingRuntimeService) EventsEndpoint(context.Context) (RuntimeEventsEndpointResponse, error) {
	return RuntimeEventsEndpointResponse{}, nil
}

func (s *recordingRuntimeService) DecidePermission(context.Context, RuntimePermissionDecision) (RuntimeStatus, error) {
	return RuntimeStatus{}, nil
}

func (s *recordingRuntimeService) Cancel(context.Context) (RuntimeStatus, error) {
	return RuntimeStatus{}, nil
}

func (s *recordingRuntimeService) NewChat(context.Context, string) (RuntimeStatus, error) {
	return RuntimeStatus{}, nil
}
