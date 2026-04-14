package queryengine

import (
	"os"
	"strings"

	"myclaw/internal/permissions"
)

const (
	defaultOpusMainLoopModel      = "claude-opus-4-6"
	defaultSonnetMainLoopModel    = "claude-sonnet-4-6"
	thirdPartySonnetMainLoopModel = "claude-sonnet-4-5"
	defaultHaikuMainLoopModel     = "claude-haiku-4-5"
)

func parseUserSpecifiedMainLoopModel(model string, provider string) string {
	normalizedModel, has1mTag := normalizeMainLoopModelInput(model)
	if normalizedModel == "" {
		return ""
	}

	switch normalizedModel {
	case "opusplan", "sonnet":
		return withOptional1mTag(defaultSonnetModel(provider), has1mTag)
	case "haiku":
		return withOptional1mTag(defaultHaikuModel(), has1mTag)
	case "opus", "best":
		return withOptional1mTag(defaultOpusModel(), has1mTag)
	default:
		return withOptional1mTag(strings.TrimSpace(model), has1mTag)
	}
}

func defaultOpusModel() string {
	if value := strings.TrimSpace(os.Getenv("ANTHROPIC_DEFAULT_OPUS_MODEL")); value != "" {
		return value
	}
	return defaultOpusMainLoopModel
}

func defaultSonnetModel(provider string) string {
	if value := strings.TrimSpace(os.Getenv("ANTHROPIC_DEFAULT_SONNET_MODEL")); value != "" {
		return value
	}
	if provider = strings.TrimSpace(provider); provider != "" && !strings.EqualFold(provider, "firstParty") {
		return thirdPartySonnetMainLoopModel
	}
	return defaultSonnetMainLoopModel
}

func defaultHaikuModel() string {
	if value := strings.TrimSpace(os.Getenv("ANTHROPIC_DEFAULT_HAIKU_MODEL")); value != "" {
		return value
	}
	return defaultHaikuMainLoopModel
}

func smallFastModel() string {
	if value := strings.TrimSpace(os.Getenv("ANTHROPIC_SMALL_FAST_MODEL")); value != "" {
		return value
	}
	return defaultHaikuModel()
}

func resolveRuntimeMainLoopModel(model string, policy permissions.Policy) string {
	return resolveRuntimeMainLoopModelWithProviderAndContext(model, policy, "", false)
}

func resolveRuntimeMainLoopModelWithContext(model string, policy permissions.Policy, exceeds200kTokens bool) string {
	return resolveRuntimeMainLoopModelWithProviderAndContext(model, policy, "", exceeds200kTokens)
}

func resolveRuntimeMainLoopModelWithProviderAndContext(model string, policy permissions.Policy, provider string, exceeds200kTokens bool) string {
	normalizedModel, has1mTag := normalizeMainLoopModelInput(model)
	if normalizedModel == "" {
		return ""
	}
	resolvedModel := parseUserSpecifiedMainLoopModel(model, provider)
	if !policy.PlanMode {
		return resolvedModel
	}

	switch normalizedModel {
	case "opusplan":
		if exceeds200kTokens {
			return resolvedModel
		}
		return withOptional1mTag(defaultOpusModel(), has1mTag)
	case "haiku":
		return withOptional1mTag(defaultSonnetModel(provider), has1mTag)
	default:
		return resolvedModel
	}
}

func normalizeMainLoopModelInput(model string) (string, bool) {
	trimmed := strings.TrimSpace(model)
	if trimmed == "" {
		return "", false
	}
	lower := strings.ToLower(trimmed)
	has1mTag := strings.HasSuffix(lower, "[1m]")
	if has1mTag {
		lower = strings.TrimSpace(strings.TrimSuffix(lower, "[1m]"))
	}
	return lower, has1mTag
}

func withOptional1mTag(model string, has1mTag bool) string {
	model = strings.TrimSpace(model)
	if model == "" || !has1mTag {
		return model
	}
	if strings.HasSuffix(strings.ToLower(model), "[1m]") {
		return model
	}
	return model + "[1m]"
}
