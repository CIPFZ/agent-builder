package app

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"myclaw/internal/config"
	"myclaw/internal/llm"
	"myclaw/internal/permissions"
	"myclaw/internal/runtime"
	"myclaw/internal/session"
	"myclaw/internal/workspace"
)

func TestNewDaemonHandlerServesWebUI(t *testing.T) {
	cfg := config.Config{
		HTTPAddr: "127.0.0.1:0",
		WSPath:   "/ws",
	}

	handler, _ := newDaemonHandler(cfg, io.Discard)

	req := httptest.NewRequest(http.MethodGet, "/ui", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("/ui status = %d, want 200", rec.Code)
	}
	if got := rec.Header().Get("Content-Type"); !strings.Contains(got, "text/html") {
		t.Fatalf("/ui content-type = %q, want html", got)
	}
	body := rec.Body.String()
	if !strings.Contains(body, "MyClaw Control Panel") {
		t.Fatalf("/ui body missing title: %q", body)
	}
	if !strings.Contains(body, "\"/ws\"") {
		t.Fatalf("/ui body missing ws path: %q", body)
	}
	if !strings.Contains(body, `type: "req"`) {
		t.Fatalf("/ui body missing req protocol type: %q", body)
	}
	if !strings.Contains(body, "Session Status") {
		t.Fatalf("/ui body missing session panel: %q", body)
	}
	if !strings.Contains(body, "Approval Actions") {
		t.Fatalf("/ui body missing approval panel: %q", body)
	}
	if !strings.Contains(body, "approval_list") {
		t.Fatalf("/ui body missing approval action wiring: %q", body)
	}
	if !strings.Contains(body, "session-id-value") {
		t.Fatalf("/ui body missing session info slots: %q", body)
	}
	if !strings.Contains(body, "approval-items") {
		t.Fatalf("/ui body missing approval list container: %q", body)
	}
	if !strings.Contains(body, "plan-summary-value") {
		t.Fatalf("/ui body missing plan summary slot: %q", body)
	}
	if !strings.Contains(body, "history-summary-value") {
		t.Fatalf("/ui body missing history summary slot: %q", body)
	}
	if !strings.Contains(body, "transcript-items") {
		t.Fatalf("/ui body missing transcript container: %q", body)
	}
	if !strings.Contains(body, "updateSessionInfo") {
		t.Fatalf("/ui body missing session update logic: %q", body)
	}
	if !strings.Contains(body, "renderApprovals") {
		t.Fatalf("/ui body missing approval render logic: %q", body)
	}
	if !strings.Contains(body, "renderPlanSummary") {
		t.Fatalf("/ui body missing plan summary render logic: %q", body)
	}
	if !strings.Contains(body, "renderHistorySummary") {
		t.Fatalf("/ui body missing history summary render logic: %q", body)
	}
	if !strings.Contains(body, "renderTranscriptMessage") {
		t.Fatalf("/ui body missing transcript render logic: %q", body)
	}
	if !strings.Contains(body, "applyAssistantDelta") {
		t.Fatalf("/ui body missing assistant delta logic: %q", body)
	}
}

func TestNewDaemonHandlerKeepsRootTextEndpoint(t *testing.T) {
	cfg := config.Config{
		HTTPAddr: "127.0.0.1:0",
		WSPath:   "/ws",
	}

	handler, _ := newDaemonHandler(cfg, io.Discard)

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("/ status = %d, want 200", rec.Code)
	}
	if got := rec.Header().Get("Content-Type"); !strings.Contains(got, "text/plain") {
		t.Fatalf("/ content-type = %q, want text/plain", got)
	}
	if body := rec.Body.String(); !strings.Contains(body, "myclawd is running") {
		t.Fatalf("/ body = %q, want running text", body)
	}
}

func TestNewDaemonHandlerBootstrapsDaemonRuntimeOnlyOnce(t *testing.T) {
	original := daemonBootstrapRuntime
	t.Cleanup(func() { daemonBootstrapRuntime = original })

	var calls atomic.Int32
	daemonBootstrapRuntime = func(_ string, _ config.Config, _ bootstrapOptions) (*runtimeBootstrap, error) {
		calls.Add(1)
		sessions := session.NewManager(nil)
		runner := runtime.NewRunnerWithOptions(
			sessions,
			llm.NewMockClient(),
			workspace.NewLoader(""),
			nil,
			runtime.Options{
				PermissionPolicy: permissions.Policy{Mode: permissions.ModeDangerFullAccess},
			},
		)
		return &runtimeBootstrap{
			Sessions: sessions,
			Policy:   permissions.Policy{Mode: permissions.ModeDangerFullAccess},
			Runner:   runner,
		}, nil
	}

	cfg := config.Config{
		HTTPAddr: "127.0.0.1:0",
		WSPath:   "/ws",
	}
	_, gatewayServer := newDaemonHandler(cfg, io.Discard)
	if gatewayServer == nil {
		t.Fatal("expected daemon handler to return gateway server")
	}
	if got := calls.Load(); got != 1 {
		t.Fatalf("bootstrap runtime calls = %d, want exactly 1", got)
	}
}

func TestLLMClientFromRuntimeConfigReturnsUnavailableWithoutAPIKey(t *testing.T) {
	cfg := config.Config{
		LLM: config.LLMConfig{
			Provider: "default",
			BaseURL:  "https://example.invalid/v1/chat/completions",
			APIKey:   "",
			Model:    "LongCat-Flash-Chat",
		},
	}

	client := LLMClientFromRuntimeConfig(cfg)
	if _, ok := client.(*llm.UnavailableClient); !ok {
		t.Fatalf("client = %T, want *llm.UnavailableClient", client)
	}
}
