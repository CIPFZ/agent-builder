package agent

import (
	"context"
	"encoding/json"
	"errors"
	"strings"

	"charm.land/fantasy"
	"charm.land/fantasy/jsonrepair"
)

func repairToolCallJSON(ctx context.Context, options fantasy.ToolCallRepairOptions) (*fantasy.ToolCallContent, error) {
	_ = ctx
	if strings.TrimSpace(options.OriginalToolCall.Input) == "" {
		return nil, errors.New("tool call input is empty")
	}
	if json.Valid([]byte(options.OriginalToolCall.Input)) {
		return nil, errors.New("tool call input is already valid JSON")
	}
	repairedInput, err := jsonrepair.RepairJSON(options.OriginalToolCall.Input, jsonrepair.WithStreamStable())
	if err != nil {
		return nil, err
	}
	if strings.TrimSpace(repairedInput) == "" || repairedInput == options.OriginalToolCall.Input {
		return nil, errors.New("tool call JSON repair produced no changes")
	}
	var decoded any
	if err := json.Unmarshal([]byte(repairedInput), &decoded); err != nil {
		return nil, err
	}
	repaired := options.OriginalToolCall
	repaired.Input = repairedInput
	return &repaired, nil
}
