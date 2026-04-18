package tui

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"

	tea "github.com/charmbracelet/bubbletea"

	"myclaw/internal/diagnostics"
	"myclaw/internal/runtime"
	"myclaw/internal/session"
)

type Options struct {
	LLMLabel string
	Logger   *diagnostics.Logger
}

func Run(ctx context.Context, sessions *session.Manager, runner *runtime.Runner, _ any, _ any, options Options) error {
	if runner == nil {
		return errors.New("nil runner")
	}
	bridge := NewRuntimeBridgeWithContext(ctx, sessions, runner, "main", options.Logger)
	model := NewModel(bridge, ModelConfig{
		SessionID:       bridge.session.ID,
		LLMLabel:        options.LLMLabel,
		PromptEditor:    defaultPromptEditor,
		OpenFile:        defaultFileOpener,
		OpenFileAtLine:  defaultFileLocationOpener,
		WorkspaceSearch: defaultWorkspaceSearcher,
		WorkspaceRoots: func() []string {
			cwd, err := os.Getwd()
			if err != nil || strings.TrimSpace(cwd) == "" {
				return nil
			}
			return []string{cwd}
		}(),
		LogPath: func() string {
			if options.Logger == nil {
				return ""
			}
			return options.Logger.Path()
		}(),
	})
	bridge.log("info", "tui", "startup", fmt.Sprintf("starting session %s", bridge.session.ID), "", map[string]any{
		"llm": options.LLMLabel,
		"log_path": func() string {
			if options.Logger == nil {
				return ""
			}
			return options.Logger.Path()
		}(),
	})
	program := tea.NewProgram(model, tea.WithAltScreen(), tea.WithMouseCellMotion())
	bridge.Attach(program.Send)
	_, err := program.Run()
	if err != nil {
		bridge.log("error", "tui", "shutdown.error", err.Error(), "", nil)
	} else {
		bridge.log("info", "tui", "shutdown", "program exited cleanly", "", nil)
	}
	return err
}
