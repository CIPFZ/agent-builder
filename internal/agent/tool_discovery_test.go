package agent

import (
	"context"
	"errors"
	"testing"

	"charm.land/fantasy"
	"github.com/stretchr/testify/require"
)

type recordingDiscoveryRecorder struct {
	result SchedulerToolDisclosureResult
	err    error
	req    SchedulerToolDisclosureRequest
}

func (r *recordingDiscoveryRecorder) SelectToolsForTurn(_ context.Context, req SchedulerToolDisclosureRequest) (SchedulerToolDisclosureResult, error) {
	r.req = req
	return r.result, r.err
}

func (r *recordingDiscoveryRecorder) SearchToolsForAgent(context.Context, SchedulerToolSearchRequest) (SchedulerToolSearchResult, error) {
	return SchedulerToolSearchResult{}, nil
}

func TestSelectToolsForPreparedStepKeepsSelectedAndBaseTools(t *testing.T) {
	t.Parallel()

	inputs := []string{ToolSearchToolName, "view", "write", "web_search"}
	fantasyTools := makeFakeAgentTools(inputs...)
	recorder := &recordingDiscoveryRecorder{result: SchedulerToolDisclosureResult{Selected: []string{ToolSearchToolName, "view", "web_search"}}}

	got := selectToolsForPreparedStep(t.Context(), fantasyTools, recorder, "session-1", "turn-1")

	require.Equal(t, []string{ToolSearchToolName, "view", "web_search"}, toolNames(got))
	require.Equal(t, "session-1", recorder.req.SessionID)
	require.Equal(t, "turn-1", recorder.req.TurnID)
	require.Len(t, recorder.req.Tools, len(inputs))
}

func TestSelectToolsForPreparedStepFailsClosedToBaseTools(t *testing.T) {
	t.Parallel()

	fantasyTools := makeFakeAgentTools(ToolSearchToolName, "view", "write", "web_search")
	recorder := &recordingDiscoveryRecorder{err: errors.New("discovery unavailable")}

	got := selectToolsForPreparedStep(t.Context(), fantasyTools, recorder, "session-1", "turn-1")

	require.Equal(t, []string{ToolSearchToolName, "view", "write"}, toolNames(got))
}

func TestSelectToolsForPreparedStepEmptySelectionFailsClosedToBaseTools(t *testing.T) {
	t.Parallel()

	fantasyTools := makeFakeAgentTools(ToolSearchToolName, "view", "write", "web_search")
	recorder := &recordingDiscoveryRecorder{}

	got := selectToolsForPreparedStep(t.Context(), fantasyTools, recorder, "session-1", "turn-1")

	require.Equal(t, []string{ToolSearchToolName, "view", "write"}, toolNames(got))
}

func makeFakeAgentTools(names ...string) []fantasy.AgentTool {
	tools := make([]fantasy.AgentTool, 0, len(names))
	for _, name := range names {
		tools = append(tools, &fakeTool{name: name})
	}
	return tools
}

func toolNames(tools []fantasy.AgentTool) []string {
	names := make([]string, 0, len(tools))
	for _, tool := range tools {
		names = append(names, tool.Info().Name)
	}
	return names
}
