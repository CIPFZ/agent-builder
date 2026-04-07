package tools

import (
	"context"
	"strings"

	"myclaw/internal/session"
)

type TextUpperTool struct{}

func NewTextUpperTool() *TextUpperTool {
	return &TextUpperTool{}
}

func (t *TextUpperTool) Definition() Definition {
	return Definition{
		Name:        "text.upper",
		Description: "Convert the given input text to uppercase.",
		Aliases:     []string{"uppercase"},
		Enabled:     true,
		ReadOnly:    true,
	}
}

func (t *TextUpperTool) Invoke(_ context.Context, _ session.Session, input string) (string, error) {
	return strings.ToUpper(input), nil
}

func (t *TextUpperTool) IsEnabled() bool {
	return true
}

func (t *TextUpperTool) IsReadOnly(_ string) bool {
	return true
}

func (t *TextUpperTool) IsDestructive(_ string) bool {
	return false
}

func (t *TextUpperTool) ShouldDefer() bool {
	return false
}

func (t *TextUpperTool) AlwaysLoad() bool {
	return false
}

func (t *TextUpperTool) PromptDescription() string {
	return "Convert the given input text to uppercase."
}

func (t *TextUpperTool) SearchHint() string {
	return "uppercase text"
}
