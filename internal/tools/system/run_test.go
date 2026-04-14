package system

import (
	"testing"

	"myclaw/internal/tools"
)

func TestRunToolAutoClassifierInputReturnsCommand(t *testing.T) {
	tool := NewRunTool(nil)

	classifier, ok := any(tool).(tools.AutoClassifyingTool)
	if !ok {
		t.Fatal("RunTool must expose a Claude-style auto classifier input projection")
	}

	got := classifier.ToAutoClassifierInput("  cat README.md  ")
	if got != "cat README.md" {
		t.Fatalf("expected trimmed command classifier input, got %#v", got)
	}
}
