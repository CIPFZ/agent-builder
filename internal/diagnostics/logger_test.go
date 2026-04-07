package diagnostics

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestLoggerWritesJSONLRecord(t *testing.T) {
	path := filepath.Join(t.TempDir(), "logs", "myclaw.jsonl")
	logger, err := NewLogger(path)
	if err != nil {
		t.Fatalf("NewLogger: %v", err)
	}
	defer logger.Close()

	if err := logger.Log(Entry{
		Level:     "info",
		Component: "tui",
		Event:     "startup",
		Message:   "ready",
		SessionID: "main-1",
		RunID:     "run-1",
		Fields: map[string]any{
			"llm": "openai-compatible",
		},
	}); err != nil {
		t.Fatalf("Log: %v", err)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}

	var record map[string]any
	if err := json.Unmarshal(data, &record); err != nil {
		t.Fatalf("Unmarshal: %v\nraw=%s", err, string(data))
	}
	if record["component"] != "tui" {
		t.Fatalf("component = %#v, want tui", record["component"])
	}
	if record["event"] != "startup" {
		t.Fatalf("event = %#v, want startup", record["event"])
	}
	if record["message"] != "ready" {
		t.Fatalf("message = %#v, want ready", record["message"])
	}
	fields, ok := record["fields"].(map[string]any)
	if !ok || fields["llm"] != "openai-compatible" {
		t.Fatalf("fields = %#v, want llm field", record["fields"])
	}
}
