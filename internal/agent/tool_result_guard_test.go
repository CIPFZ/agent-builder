package agent

import (
	"os"
	"strings"
	"testing"

	"github.com/CIPFZ/agent-builder/internal/config"
	"github.com/CIPFZ/agent-builder/internal/message"
	"github.com/stretchr/testify/require"
)

func newTestGuard(tmpDir string) *ToolResultGuard {
	cfg := config.DefaultToolResultGuardConfig()
	cfg.ResultsDir = tmpDir
	cfg.MaxResultChars = 100 // small threshold for testing
	return NewToolResultGuard(cfg, "test-session")
}

func TestGuard_ExemptTool_PassesThrough(t *testing.T) {
	g := newTestGuard(t.TempDir())
	g.config.ExemptTools = []string{"view", "ls"}

	result := message.ToolResult{
		ToolCallID: "toolu_001",
		Name:       "view",
		Content:    strings.Repeat("x", 10000),
	}

	processed := g.Process(result)
	require.Equal(t, result.Content, processed.Content)
	require.Empty(t, processed.StoredPath)
	require.Empty(t, processed.TruncatedBy)
}

func TestGuard_SingleResultTruncation(t *testing.T) {
	g := newTestGuard(t.TempDir())

	result := message.ToolResult{
		ToolCallID: "toolu_001",
		Name:       "bash",
		Content:    strings.Repeat("line of output text\n", 50),
	}

	processed := g.Process(result)
	require.Contains(t, processed.Content, "<persisted-output>")
	require.NotEmpty(t, processed.StoredPath)
	require.Equal(t, "single", processed.TruncatedBy)
	require.Greater(t, processed.OriginalSize, int64(0))

	_, err := os.Stat(processed.StoredPath)
	require.NoError(t, err)
}

func TestGuard_UnderThreshold_NoTruncation(t *testing.T) {
	g := newTestGuard(t.TempDir())

	result := message.ToolResult{
		ToolCallID: "toolu_001",
		Name:       "bash",
		Content:    "short output",
	}

	processed := g.Process(result)
	require.Equal(t, "short output", processed.Content)
	require.Empty(t, processed.StoredPath)
	require.Equal(t, int64(0), processed.OriginalSize)
}

func TestGuard_TurnBudget_SpillsLargestResult(t *testing.T) {
	g := newTestGuard(t.TempDir())
	g.config.TurnBudget = 50

	r1 := message.ToolResult{
		ToolCallID: "toolu_001",
		Name:       "grep",
		Content:    "short",
	}
	p1 := g.Process(r1)
	require.Empty(t, p1.StoredPath)

	r2 := message.ToolResult{
		ToolCallID: "toolu_002",
		Name:       "bash",
		Content:    strings.Repeat("x", 50),
	}
	p2 := g.Process(r2)
	require.NotEmpty(t, p2.StoredPath)
	require.Contains(t, p2.Content, "<persisted-output>")
	require.Equal(t, "turn_budget", p2.TruncatedBy)
}

func TestGuard_ResetTurn(t *testing.T) {
	g := newTestGuard(t.TempDir())
	g.config.TurnBudget = 50

	r1 := message.ToolResult{
		ToolCallID: "toolu_001",
		Name:       "bash",
		Content:    "a somewhat long result that exceeds the turn budget threshold",
	}
	g.Process(r1)

	g.ResetTurn()
	require.Equal(t, 0, g.turnTotal)

	r2 := message.ToolResult{
		ToolCallID: "toolu_002",
		Name:       "bash",
		Content:    "another result",
	}
	p2 := g.Process(r2)
	require.Empty(t, p2.TruncatedBy)
}

func TestGuard_Disabled_SkipsAllProcessing(t *testing.T) {
	cfg := config.DefaultToolResultGuardConfig()
	cfg.Enabled = false
	cfg.MaxResultChars = 10
	g := NewToolResultGuard(cfg, "test-session")

	result := message.ToolResult{
		ToolCallID: "toolu_001",
		Name:       "bash",
		Content:    strings.Repeat("very long output that would be truncated\n", 100),
	}

	processed := g.Process(result)
	require.Equal(t, result.Content, processed.Content)
	require.Empty(t, processed.StoredPath)
}
