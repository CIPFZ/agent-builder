package system

import (
	"context"

	"myclaw/internal/sandbox"
	"myclaw/internal/session"
	"myclaw/internal/tools"
)

var _ tools.Tool = (*RunTool)(nil)

type RunTool struct {
	router *sandbox.Router
}

func NewRunTool(router *sandbox.Router) *RunTool {
	if router == nil {
		router = sandbox.NewRouter(nil, nil)
	}
	return &RunTool{router: router}
}

func (t *RunTool) Definition() tools.Definition {
	return tools.Definition{
		Name:        "system.run",
		Description: "Run a shell command on the host system and return stdout and stderr.",
		Enabled:     true,
		ReadOnly:    false,
		Destructive: true,
	}
}

func (t *RunTool) Invoke(ctx context.Context, sess session.Session, input string) (string, error) {
	return t.router.Run(ctx, sess, input)
}

func (t *RunTool) IsEnabled() bool {
	return true
}

func (t *RunTool) IsReadOnly(_ string) bool {
	return false
}

func (t *RunTool) IsDestructive(_ string) bool {
	return true
}

func (t *RunTool) ShouldDefer() bool {
	return false
}

func (t *RunTool) AlwaysLoad() bool {
	return false
}

func (t *RunTool) PromptDescription() string {
	return "Run a shell command on the host system and return stdout and stderr."
}

func (t *RunTool) SearchHint() string {
	return "shell command"
}
