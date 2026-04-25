package tui

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"

	tea "github.com/charmbracelet/bubbletea"

	"myclaw/internal/diagnostics"
)

type Options struct {
	LLMLabel string
	Logger   *diagnostics.Logger
}

func Run(ctx context.Context, myclawdURL string, options Options) error {
	if strings.TrimSpace(myclawdURL) == "" {
		return errors.New("empty myclawd url")
	}
	store := newClientStore()
	bridge := NewMyclawdClient(ctx, myclawdURL, "main", store, protocolLoggerAdapter{logger: options.Logger})
	model := NewModel(bridge, ModelConfig{
		SessionID:       bridge.PlatformStatusSnapshot().SessionID,
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
	if err := bridge.Start(); err != nil {
		return err
	}
	defer bridge.Close()
	bridge.log("info", "startup", fmt.Sprintf("connected to %s", myclawdURL), map[string]any{
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
		bridge.log("error", "shutdown.error", err.Error(), nil)
	} else {
		bridge.log("info", "shutdown", "program exited cleanly", nil)
	}
	return err
}

type protocolLoggerAdapter struct {
	logger *diagnostics.Logger
}

func (a protocolLoggerAdapter) Log(level, component, event, message string, fields map[string]any) {
	if a.logger == nil {
		return
	}
	_ = a.logger.Log(diagnostics.Entry{
		Level:     level,
		Component: component,
		Event:     event,
		Message:   message,
		Fields:    fields,
	})
}
