package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"charm.land/fantasy"
	"github.com/charmbracelet/crush/internal/agent/tools"
)

const ToolSearchToolName = "tool_search"

type toolSearchParams struct {
	Query      string `json:"query" description:"Search query, or select:<tool_name> for direct selection"`
	MaxResults int    `json:"max_results,omitempty" description:"Maximum number of tool matches to return"`
}

type toolSearchTool struct {
	recorder ToolDiscoveryRecorder
	opts     fantasy.ProviderOptions
}

func newToolSearchTool(recorder ToolDiscoveryRecorder) fantasy.AgentTool {
	return &toolSearchTool{recorder: recorder}
}

func (t *toolSearchTool) Info() fantasy.ToolInfo {
	return fantasy.ToolInfo{
		Name:        ToolSearchToolName,
		Description: "Search and select deferred runtime tools by name, source, or capability. Use select:<tool_name> to select a known tool.",
		Parameters: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"query": map[string]any{
					"type":        "string",
					"description": "Search query, or select:<tool_name> for direct selection.",
				},
				"max_results": map[string]any{
					"type":        "integer",
					"description": "Maximum number of tool matches to return.",
				},
			},
		},
		Required: []string{"query"},
		Parallel: true,
	}
}

func (t *toolSearchTool) Run(ctx context.Context, call fantasy.ToolCall) (fantasy.ToolResponse, error) {
	var params toolSearchParams
	if err := json.Unmarshal([]byte(call.Input), &params); err != nil {
		return fantasy.NewTextErrorResponse("invalid parameters: " + err.Error()), nil
	}
	if t.recorder == nil {
		return fantasy.NewTextErrorResponse("tool discovery is unavailable"), nil
	}
	result, err := t.recorder.SearchToolsForAgent(ctx, SchedulerToolSearchRequest{
		Query:      params.Query,
		MaxResults: params.MaxResults,
		SessionID:  tools.GetSessionFromContext(ctx),
		TurnID:     tools.GetTurnFromContext(ctx),
		ToolCallID: call.ID,
	})
	if err != nil {
		return fantasy.NewTextErrorResponse(err.Error()), nil
	}
	if result.Message != "" {
		return fantasy.NewTextResponse(result.Message), nil
	}
	if len(result.Matches) == 0 {
		return fantasy.NewTextResponse(fmt.Sprintf("No matching runtime tools found for %q.", result.Query)), nil
	}
	return fantasy.NewTextResponse(fmt.Sprintf("Selected runtime tools: %s", strings.Join(result.Matches, ", "))), nil
}

func (t *toolSearchTool) ProviderOptions() fantasy.ProviderOptions {
	return t.opts
}

func (t *toolSearchTool) SetProviderOptions(opts fantasy.ProviderOptions) {
	t.opts = opts
}
