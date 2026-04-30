package queryengine

import (
	"context"
	"fmt"
	runtimecommands "myclaw/internal/commands"
	"myclaw/internal/session"
	"myclaw/internal/tools"
	"strings"
)

type CommandLifecycleResolver interface {
	CommandLifecycleState(session.Session, string) (ExtensionCommand, bool)
}

type ProcessResult struct {
	NormalizedInput string
	ShouldQuery     bool
	ResultText      string
	InputMode       string
	CommandName     string
	Messages        []ImmediateMessage
}

type ImmediateMessage struct {
	Role    string
	Content string
}

type InputProcessor interface {
	Process(context.Context, session.Session, string) (ProcessResult, error)
}

type noopInputProcessor struct{}

type lifecycleInputProcessor struct {
	resolver CommandLifecycleResolver
}

type InputProcessorFunc func(context.Context, session.Session, string) (ProcessResult, error)

func (f InputProcessorFunc) Process(ctx context.Context, sess session.Session, input string) (ProcessResult, error) {
	return f(ctx, sess, input)
}

func (noopInputProcessor) Process(_ context.Context, sess session.Session, input string) (ProcessResult, error) {
	return processRuntimeInput(sess, input, nil)
}

func (p lifecycleInputProcessor) Process(_ context.Context, sess session.Session, input string) (ProcessResult, error) {
	return processRuntimeInput(sess, input, p.resolver)
}

func processRuntimeInput(sess session.Session, input string, resolver CommandLifecycleResolver) (ProcessResult, error) {
	normalized := strings.TrimSpace(input)
	if normalized == "" {
		return ProcessResult{}, nil
	}
	if strings.HasPrefix(normalized, "/") && !strings.ContainsAny(normalized, "\r\n") {
		if resolver != nil {
			if command, ok := resolver.CommandLifecycleState(sess, normalized); ok && command.LifecycleState == tools.ExtensionStateDisabled {
				return ProcessResult{}, fmt.Errorf("slash command %q is disabled by extension lifecycle state disabled", commandNameForError(normalized))
			}
		}
		result, err := runtimecommands.NewDefaultRegistry().Execute(defaultCommandContext(sess), normalized)
		if err != nil {
			return ProcessResult{}, err
		}
		processed := ProcessResult{
			NormalizedInput: result.NormalizedInput,
			ShouldQuery:     result.ShouldQuery,
			ResultText:      result.Output,
			InputMode:       "command",
			CommandName:     result.CommandName,
		}
		if !processed.ShouldQuery {
			processed.NormalizedInput = normalized
		}
		return processed, nil
	}
	return ProcessResult{
		NormalizedInput: normalized,
		ShouldQuery:     true,
		InputMode:       "prompt",
	}, nil
}

func commandNameForError(input string) string {
	body := strings.TrimPrefix(strings.TrimSpace(input), "/")
	name, _, _ := strings.Cut(body, " ")
	name = strings.TrimSpace(name)
	if name == "" {
		return input
	}
	return "/" + strings.ToLower(name)
}

func defaultCommandContext(sess session.Session) runtimecommands.Context {
	modelName := strings.TrimSpace(sess.Metadata.MainLoopModelOverride)
	if modelName == "" {
		modelName = strings.TrimSpace(sess.Metadata.InitialMainLoopModel)
	}
	return runtimecommands.Context{
		PermissionMode:       "default",
		Model:                modelName,
		HasMemory:            true,
		HasResumableSessions: true,
		HasTasks:             true,
		HasMCP:               true,
	}
}
