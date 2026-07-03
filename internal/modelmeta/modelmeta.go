package modelmeta

import (
	"sort"
	"strings"

	"charm.land/catwalk/pkg/catwalk"
)

const (
	SourceUserOverride    = "user_override"
	SourceProviderDefault = "provider_default"
	SourceDiscovered      = "discovered"
	SourceBuiltin         = "builtin"
	SourceFallback        = "fallback"

	MinContextWindow      = 16000
	MaxContextWindow      = 10000000
	FallbackContextWindow = 128000
	FallbackMaxOutput     = 4096
)

type ModelLimits struct {
	ContextWindow   int
	MaxOutputTokens int
	Source          string
}

type ResolveRequest struct {
	ProviderID                   string
	ModelID                      string
	UserContextWindow            int
	UserMaxOutputTokens          int
	ProviderDefaultContextWindow int
	DiscoveredContextWindow      int
	DiscoveredMaxOutputTokens    int
	Catalog                      []catwalk.Model
}

type builtinLimit struct {
	id              string
	contextWindow   int
	maxOutputTokens int
}

var supplementalLimits = []builtinLimit{
	{id: "deepseek-chat", contextWindow: 128000},
	{id: "deepseek-reasoner", contextWindow: 128000},
	{id: "qwen-long", contextWindow: 1000000},
	{id: "qwen", contextWindow: 128000},
	{id: "glm-4", contextWindow: 128000},
	{id: "kimi", contextWindow: 256000},
	{id: "moonshot", contextWindow: 256000},
	{id: "gpt-4.1", contextWindow: 128000},
	{id: "gpt-4o", contextWindow: 128000},
	{id: "o1", contextWindow: 200000},
	{id: "o3", contextWindow: 200000},
	{id: "o4", contextWindow: 200000},
	{id: "gemini-2", contextWindow: 1000000},
	{id: "gemini-2.0", contextWindow: 1000000},
	{id: "gemini-2.5", contextWindow: 1000000},
}

func Resolve(req ResolveRequest) ModelLimits {
	if validContextWindow(req.UserContextWindow) {
		return ModelLimits{
			ContextWindow:   req.UserContextWindow,
			MaxOutputTokens: firstPositive(req.UserMaxOutputTokens, req.DiscoveredMaxOutputTokens, builtinMaxOutput(req), FallbackMaxOutput),
			Source:          SourceUserOverride,
		}
	}
	if validContextWindow(req.ProviderDefaultContextWindow) {
		return ModelLimits{
			ContextWindow:   req.ProviderDefaultContextWindow,
			MaxOutputTokens: firstPositive(req.UserMaxOutputTokens, req.DiscoveredMaxOutputTokens, builtinMaxOutput(req), FallbackMaxOutput),
			Source:          SourceProviderDefault,
		}
	}
	if validContextWindow(req.DiscoveredContextWindow) {
		return ModelLimits{
			ContextWindow:   req.DiscoveredContextWindow,
			MaxOutputTokens: firstPositive(req.UserMaxOutputTokens, req.DiscoveredMaxOutputTokens, builtinMaxOutput(req), FallbackMaxOutput),
			Source:          SourceDiscovered,
		}
	}
	if builtin := resolveBuiltin(req); validContextWindow(builtin.ContextWindow) {
		builtin.MaxOutputTokens = firstPositive(req.UserMaxOutputTokens, req.DiscoveredMaxOutputTokens, builtin.MaxOutputTokens, FallbackMaxOutput)
		builtin.Source = SourceBuiltin
		return builtin
	}
	return ModelLimits{
		ContextWindow:   FallbackContextWindow,
		MaxOutputTokens: firstPositive(req.UserMaxOutputTokens, req.DiscoveredMaxOutputTokens, FallbackMaxOutput),
		Source:          SourceFallback,
	}
}

func validContextWindow(value int) bool {
	return value >= MinContextWindow && value <= MaxContextWindow
}

func firstPositive(values ...int) int {
	for _, value := range values {
		if value > 0 {
			return value
		}
	}
	return 0
}

func resolveBuiltin(req ResolveRequest) ModelLimits {
	modelID := strings.ToLower(strings.TrimSpace(req.ModelID))
	candidates := make([]builtinLimit, 0, len(req.Catalog)+len(supplementalLimits))
	for _, model := range req.Catalog {
		id := strings.TrimSpace(model.ID)
		if id == "" {
			continue
		}
		candidates = append(candidates, builtinLimit{
			id:              id,
			contextWindow:   int(model.ContextWindow),
			maxOutputTokens: int(model.DefaultMaxTokens),
		})
	}
	candidates = append(candidates, supplementalLimits...)
	sort.SliceStable(candidates, func(i, j int) bool {
		return len(candidates[i].id) > len(candidates[j].id)
	})
	for _, candidate := range candidates {
		id := strings.ToLower(strings.TrimSpace(candidate.id))
		if id == "" || !validContextWindow(candidate.contextWindow) {
			continue
		}
		if modelID == id || strings.Contains(modelID, id) {
			return ModelLimits{
				ContextWindow:   candidate.contextWindow,
				MaxOutputTokens: candidate.maxOutputTokens,
				Source:          SourceBuiltin,
			}
		}
	}
	return ModelLimits{}
}

func builtinMaxOutput(req ResolveRequest) int {
	return resolveBuiltin(req).MaxOutputTokens
}
