package runtime

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/charmbracelet/crush/internal/agent/prompt"
	"github.com/charmbracelet/crush/internal/backend"
	"github.com/charmbracelet/crush/internal/config"
	"github.com/charmbracelet/crush/internal/db"
	"github.com/charmbracelet/crush/internal/proto"
	"github.com/charmbracelet/crush/internal/runtimeapi"
)

func TestRuntimeContextEventsAndAuditSummary(t *testing.T) {
	t.Parallel()

	service := newRuntimeService()
	sources := []RuntimeContextSource{
		{ID: "managed:coder", Kind: "managed", Name: "Coder defaults", Enabled: true, State: "loaded"},
		{ID: "project:/work/AGENTS.md", Kind: "project", Name: "AGENTS.md", Path: "/work/AGENTS.md", Enabled: true, State: "loaded", TokenEstimate: 4},
		{ID: "local:/work/AGENTS.local.md", Kind: "local", Name: "AGENTS.local.md", Path: "/work/AGENTS.local.md", Enabled: true, State: "unavailable", Reason: "missing"},
		{ID: "file:/work/broken.md", Kind: "file", Name: "broken.md", Path: "/work/broken.md", Enabled: true, State: "failed", Error: "read failed"},
	}
	summary := service.recordTurnContextSources("session-1", "turn-1", sources)
	if summary.AvailableCount != 4 || len(summary.Injected) != 2 || len(summary.Skipped) != 1 || len(summary.Failed) != 1 || summary.TokenEstimate != 4 {
		t.Fatalf("summary = %#v", summary)
	}
	events, err := service.Events(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if !slices.ContainsFunc(events.Events, func(event RuntimeEvent) bool {
		return event.Type == runtimeapi.EventContextLoaded && event.TurnID == "turn-1" && event.Payload["source_id"] == "project:/work/AGENTS.md"
	}) {
		t.Fatalf("context loaded event missing: %#v", events.Events)
	}
	if !slices.ContainsFunc(events.Events, func(event RuntimeEvent) bool {
		return event.Type == runtimeapi.EventContextFailed && event.Payload["source_id"] == "file:/work/broken.md"
	}) {
		t.Fatalf("context failed event missing: %#v", events.Events)
	}

	payload, err := auditPayload(auditEntry{RequestID: "turn-1", Event: "started", ContextSummary: &summary})
	if err != nil {
		t.Fatal(err)
	}
	parsed := runtimeTurnContextSummaryFromPayload(payload["context_summary"].(map[string]any))
	if parsed.AvailableCount != 4 || len(parsed.Injected) != 2 || len(parsed.Failed) != 1 {
		t.Fatalf("parsed = %#v", parsed)
	}
}

func TestPostCompactReinjectionRecordsSkippedAndFailedContextSources(t *testing.T) {
	t.Parallel()

	service := newRuntimeService()
	sources := []RuntimeContextSource{
		{ID: "project:/work/AGENTS.md", Kind: "project", Name: "AGENTS.md", Path: "/work/AGENTS.md", Enabled: true, State: "loaded", TokenEstimate: 4},
		{ID: "local:/work/AGENTS.local.md", Kind: "local", Name: "AGENTS.local.md", Path: "/work/AGENTS.local.md", Enabled: true, State: "unavailable", Reason: "missing"},
		{ID: "file:/work/broken.md", Kind: "file", Name: "broken.md", Path: "/work/broken.md", Enabled: true, State: "failed", Error: "Authorization: Bearer secret"},
	}
	for _, source := range sources {
		ref := reinjectedRefFromContextSource(source)
		switch source.State {
		case "loaded":
			ref.Status = compactStatusCompleted
			service.publishReinjectionEvent(runtimeapi.EventContextReinjected, "session-1", "turn-1", "compact-full", ref)
		case "failed":
			ref.Status = compactStatusFailed
			ref.Error = source.Error
			service.publishReinjectionEvent(runtimeapi.EventContextSourceFailed, "session-1", "turn-1", "compact-full", ref)
		default:
			ref.Status = compactStatusSkipped
			ref.Reason = source.Reason
			service.publishReinjectionEvent(runtimeapi.EventContextSourceSkipped, "session-1", "turn-1", "compact-full", ref)
		}
	}
	events, err := service.Events(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if !slices.ContainsFunc(events.Events, func(event RuntimeEvent) bool { return event.Type == runtimeapi.EventContextSourceSkipped }) {
		t.Fatalf("context.source.skipped missing: %#v", events.Events)
	}
	if !slices.ContainsFunc(events.Events, func(event RuntimeEvent) bool {
		return event.Type == runtimeapi.EventContextSourceFailed && event.Payload["error"] == "[REDACTED]"
	}) {
		t.Fatalf("context.source.failed redaction missing: %#v", events.Events)
	}
}

func TestContextSourceAuditDoesNotIncludeRawContent(t *testing.T) {
	t.Parallel()

	source := runtimeContextSource(prompt.ContextSource{
		ID:             "project:agents",
		Kind:           prompt.ContextSourceProject,
		Name:           "AGENTS.md",
		Path:           "/work/AGENTS.md",
		Enabled:        true,
		State:          prompt.ContextStateLoaded,
		ContentSummary: "short summary",
		Content:        "PRIVATE RAW INSTRUCTIONS",
	})
	summary := runtimeTurnContextSummary([]RuntimeContextSource{source})
	payload, err := auditPayload(auditEntry{Event: "started", ContextSummary: &summary})
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(mustJSON(t, payload)), "PRIVATE RAW INSTRUCTIONS") {
		t.Fatalf("raw context content leaked: %#v", payload)
	}
}

func TestRuntimeContextSourcesAPIUsesMetadataOnly(t *testing.T) {
	t.Parallel()

	dataDir := t.TempDir()
	if _, err := db.Connect(context.Background(), dataDir); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Release(dataDir) })

	service := newRuntimeService()
	workspace := t.TempDir()
	writeRuntimeFile(t, filepath.Join(workspace, "AGENTS.md"), "private project instructions")
	serviceWithWorkspace(t, service, workspace, dataDir)

	resp, err := service.ContextSources(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if !slices.ContainsFunc(resp.Sources, func(source RuntimeContextSource) bool {
		return source.Name == "AGENTS.md" && source.State == "loaded" && source.ContentSummary != ""
	}) {
		t.Fatalf("context sources missing metadata summary: %#v", resp.Sources)
	}
	data, err := json.Marshal(resp)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(data), "private project instructions") {
		t.Fatalf("context API leaked raw content: %s", data)
	}
}

func mustJSON(t *testing.T, value any) []byte {
	t.Helper()
	data, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	return data
}

func serviceWithWorkspace(t *testing.T, service *runtimeService, workingDir, dataDir string) {
	t.Helper()
	cfg := config.NewRuntimeConfig(workingDir, dataDir, false)
	cfg.Options.AutoLSP = ptr(false)
	cfg.SetupAgents()
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
}

func writeRuntimeFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}
