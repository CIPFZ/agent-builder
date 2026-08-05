package agent

import (
	"context"
	"encoding/json"
	"strings"

	"charm.land/fantasy"
)

// ModelResourceGovernor is supplied by the Runtime boundary. Agent owns where
// a provider request begins and ends; Runtime owns the cross-Session budget.
type ModelResourceGovernor interface {
	AcquireModel(context.Context, int64) (release func(), err error)
}

type modelResourceGovernorContextKey struct{}
type toolResourceGovernorContextKey struct{}

type ToolResourceGovernor interface {
	AcquireTool(context.Context, string, int64) (release func(), err error)
}

const (
	ToolResourceHeavy   = "heavy_tool"
	ToolResourceShell   = "shell_process"
	ToolResourceBrowser = "browser_worker"
)

func WithModelResourceGovernor(ctx context.Context, governor ModelResourceGovernor) context.Context {
	if governor == nil {
		return ctx
	}
	return context.WithValue(ctx, modelResourceGovernorContextKey{}, governor)
}

func WithToolResourceGovernor(ctx context.Context, governor ToolResourceGovernor) context.Context {
	if governor == nil {
		return ctx
	}
	return context.WithValue(ctx, toolResourceGovernorContextKey{}, governor)
}

func acquireToolResource(ctx context.Context, source, name string, inputBytes int64) (func(), error) {
	class := toolResourceClass(source, name)
	if class == "" {
		return func() {}, nil
	}
	governor, _ := ctx.Value(toolResourceGovernorContextKey{}).(ToolResourceGovernor)
	if governor == nil {
		return func() {}, nil
	}
	return governor.AcquireTool(ctx, class, inputBytes)
}

func toolResourceClass(source, name string) string {
	lowerSource := strings.ToLower(strings.TrimSpace(source))
	lowerName := strings.ToLower(strings.TrimSpace(name))
	if lowerName == "job_output" || lowerName == "job_kill" {
		return ""
	}
	if strings.Contains(lowerName, "browser") || strings.Contains(lowerName, "computer") || strings.Contains(lowerName, "playwright") || strings.Contains(lowerName, "chrome") {
		if lowerSource == "mcp" {
			return ToolResourceHeavy
		}
		return ToolResourceBrowser
	}
	if lowerSource == "shell" || lowerName == "bash" || lowerName == "shell" {
		return ToolResourceShell
	}
	switch lowerName {
	case "write", "edit", "multiedit", "download", "patch", "apply_patch":
		return ToolResourceHeavy
	}
	if lowerSource == "mcp" {
		return ToolResourceHeavy
	}
	return ""
}

func governLanguageModel(ctx context.Context, model fantasy.LanguageModel) fantasy.LanguageModel {
	governor, _ := ctx.Value(modelResourceGovernorContextKey{}).(ModelResourceGovernor)
	if governor == nil || model == nil {
		return model
	}
	return governedLanguageModel{LanguageModel: model, governor: governor}
}

type governedLanguageModel struct {
	fantasy.LanguageModel
	governor ModelResourceGovernor
}

func (m governedLanguageModel) Generate(ctx context.Context, call fantasy.Call) (*fantasy.Response, error) {
	release, err := m.governor.AcquireModel(ctx, modelCallPayloadBytes(call))
	if err != nil {
		return nil, err
	}
	defer release()
	return m.LanguageModel.Generate(ctx, call)
}

func (m governedLanguageModel) Stream(ctx context.Context, call fantasy.Call) (fantasy.StreamResponse, error) {
	release, err := m.governor.AcquireModel(ctx, modelCallPayloadBytes(call))
	if err != nil {
		return nil, err
	}
	stream, err := m.LanguageModel.Stream(ctx, call)
	if err != nil {
		release()
		return nil, err
	}
	return func(yield func(fantasy.StreamPart) bool) {
		defer release()
		stream(yield)
	}, nil
}

func (m governedLanguageModel) GenerateObject(ctx context.Context, call fantasy.ObjectCall) (*fantasy.ObjectResponse, error) {
	release, err := m.governor.AcquireModel(ctx, modelObjectCallPayloadBytes(call))
	if err != nil {
		return nil, err
	}
	defer release()
	return m.LanguageModel.GenerateObject(ctx, call)
}

func (m governedLanguageModel) StreamObject(ctx context.Context, call fantasy.ObjectCall) (fantasy.ObjectStreamResponse, error) {
	release, err := m.governor.AcquireModel(ctx, modelObjectCallPayloadBytes(call))
	if err != nil {
		return nil, err
	}
	stream, err := m.LanguageModel.StreamObject(ctx, call)
	if err != nil {
		release()
		return nil, err
	}
	return func(yield func(fantasy.ObjectStreamPart) bool) {
		defer release()
		stream(yield)
	}, nil
}

func modelCallPayloadBytes(call fantasy.Call) int64 {
	return encodedModelPayloadBytes(call)
}

func modelObjectCallPayloadBytes(call fantasy.ObjectCall) int64 {
	// RepairText is an in-process callback, not provider payload, and prevents
	// direct JSON encoding. Account only the fields serialized to a provider.
	return encodedModelPayloadBytes(struct {
		Prompt            fantasy.Prompt
		Schema            fantasy.Schema
		SchemaName        string
		SchemaDescription string
		MaxOutputTokens   *int64
		Temperature       *float64
		TopP              *float64
		TopK              *int64
		PresencePenalty   *float64
		FrequencyPenalty  *float64
		Headers           map[string]string
		ProviderOptions   fantasy.ProviderOptions
	}{call.Prompt, call.Schema, call.SchemaName, call.SchemaDescription, call.MaxOutputTokens, call.Temperature, call.TopP, call.TopK, call.PresencePenalty, call.FrequencyPenalty, call.Headers, call.ProviderOptions})
}

func encodedModelPayloadBytes(value any) int64 {
	encoded, err := json.Marshal(value)
	if err != nil {
		// A provider request that cannot be estimated must still consume a
		// non-zero byte lease; ordinary Fantasy calls are JSON encodable.
		return 1
	}
	return max(int64(len(encoded)), 1)
}
