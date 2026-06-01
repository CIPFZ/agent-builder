package agent

import (
	"context"
	"encoding/json"
	"testing"

	"charm.land/fantasy"
)

func TestRepairToolCallJSONRepairsTruncatedObject(t *testing.T) {
	t.Parallel()

	repaired, err := repairToolCallJSON(context.Background(), fantasy.ToolCallRepairOptions{
		OriginalToolCall: fantasy.ToolCallContent{
			ToolCallID: "tool-1",
			ToolName:   "write",
			Input:      `{"file_path":"report.md","content":"ok"`,
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if repaired == nil {
		t.Fatal("expected repaired tool call")
	}
	var params map[string]string
	if err := json.Unmarshal([]byte(repaired.Input), &params); err != nil {
		t.Fatalf("repaired input is not valid JSON: %v", err)
	}
	if params["file_path"] != "report.md" || params["content"] != "ok" {
		t.Fatalf("repaired params = %#v", params)
	}
}

func TestRepairToolCallJSONLeavesValidInputAlone(t *testing.T) {
	t.Parallel()

	repaired, err := repairToolCallJSON(context.Background(), fantasy.ToolCallRepairOptions{
		OriginalToolCall: fantasy.ToolCallContent{
			ToolCallID: "tool-1",
			ToolName:   "write",
			Input:      `{"file_path":"report.md","content":"ok"}`,
		},
	})
	if err == nil || repaired != nil {
		t.Fatalf("valid input repair = %#v, %v; want no repair", repaired, err)
	}
}
