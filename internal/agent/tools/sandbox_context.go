package tools

import "context"

type sandboxContextKey struct{}

type SandboxContextMetadata struct {
	DecisionID string
	Mode       string
	Status     string
	Executor   string
	Reason     string
	Error      string
}

func WithSandboxMetadata(ctx context.Context, meta SandboxContextMetadata) context.Context {
	return context.WithValue(ctx, sandboxContextKey{}, meta)
}

func SandboxMetadataFromContext(ctx context.Context) (SandboxContextMetadata, bool) {
	meta, ok := ctx.Value(sandboxContextKey{}).(SandboxContextMetadata)
	return meta, ok
}
