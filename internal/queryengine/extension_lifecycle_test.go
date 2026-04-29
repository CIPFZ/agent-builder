package queryengine

import (
	"context"
	"testing"

	"myclaw/internal/llm"
	"myclaw/internal/session"
	"myclaw/internal/tools"
)

func TestToolLifecycleDisabledBlocksRuntimeExecutionBoundary(t *testing.T) {
	engine := New(Config{
		Client: llm.NewMockClient(),
		ToolRegistry: tools.NewRegistry(queryengineLifecycleProbeTool{
			name:   "DeniedLifecycleTool",
			source: "dynamic",
		}),
		ExtensionLifecycle: []tools.ExtensionLifecycleRecord{{
			Type:   tools.ExtensionTypeTool,
			Source: "dynamic",
			Name:   "DeniedLifecycleTool",
			State:  tools.ExtensionStateDisabled,
		}},
	})

	def, ok := engine.tools.InspectWithPolicy("DeniedLifecycleTool", "", engine.PermissionPolicyForSession(""))
	if !ok {
		t.Fatalf("tool missing from registry")
	}
	if !engine.toolLifecycleDisabled(def) {
		t.Fatalf("tool lifecycle disabled boundary returned false")
	}
}

type queryengineLifecycleProbeTool struct {
	name   string
	source string
}

func (t queryengineLifecycleProbeTool) Definition() tools.Definition {
	return tools.Definition{Name: t.name, Source: t.source}
}

func (t queryengineLifecycleProbeTool) Invoke(context.Context, session.Session, string) (string, error) {
	return "ok", nil
}

func (t queryengineLifecycleProbeTool) IsEnabled() bool           { return true }
func (t queryengineLifecycleProbeTool) IsReadOnly(string) bool    { return true }
func (t queryengineLifecycleProbeTool) IsDestructive(string) bool { return false }
func (t queryengineLifecycleProbeTool) ShouldDefer() bool         { return false }
func (t queryengineLifecycleProbeTool) AlwaysLoad() bool          { return false }
func (t queryengineLifecycleProbeTool) PromptDescription() string { return "" }
func (t queryengineLifecycleProbeTool) SearchHint() string        { return "" }
