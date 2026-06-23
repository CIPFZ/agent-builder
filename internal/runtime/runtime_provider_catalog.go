package runtime

import (
	"context"
	"sort"
	"strings"

	"charm.land/catwalk/pkg/catwalk"
	"charm.land/catwalk/pkg/embedded"
	"github.com/CIPFZ/agent-builder/internal/agent/hyper"
)

func (r *runtimeService) ProviderCatalog(ctx context.Context) (RuntimeProviderCatalogResponse, error) {
	store, err := r.providerSettingsStore(ctx)
	if err != nil {
		return RuntimeProviderCatalogResponse{}, err
	}
	if err := r.syncProviderCatalog(ctx, store); err != nil {
		return RuntimeProviderCatalogResponse{}, err
	}
	items, err := store.ListCatalog(ctx)
	if err != nil {
		return RuntimeProviderCatalogResponse{}, err
	}
	return RuntimeProviderCatalogResponse{
		ProviderTypes: runtimeProviderTypes(),
		Providers:     items,
	}, nil
}

func embeddedProviderCatalog() []RuntimeProviderCatalogItem {
	providers := append([]catwalk.Provider{}, embedded.GetAll()...)
	providers = append(providers, hyper.Embedded())

	items := make([]RuntimeProviderCatalogItem, 0, len(providers)+1)
	for _, provider := range providers {
		id := string(provider.ID)
		items = append(items, RuntimeProviderCatalogItem{
			ID:                id,
			Name:              firstNonEmpty(provider.Name, id),
			Type:              string(provider.Type),
			APIEndpoint:       provider.APIEndpoint,
			APIKeyTemplate:    provider.APIKey,
			ModelCount:        len(provider.Models),
			DefaultLargeModel: provider.DefaultLargeModelID,
			DefaultSmallModel: provider.DefaultSmallModelID,
			RequiredFields:    providerRequiredFields(provider),
			Notes:             providerNotes(provider),
			Configurable:      providerConfigurable(provider),
		})
	}
	items = append(items, RuntimeProviderCatalogItem{
		ID:             "custom",
		Name:           "Custom",
		Type:           string(catwalk.TypeOpenAICompat),
		ModelCount:     0,
		RequiredFields: []string{"apiKey", "apiEndpoint", "protocol"},
		Notes:          []string{"用于配置 OpenAI 兼容或 Anthropic 兼容的自定义 API。"},
		Configurable:   true,
	})

	sort.Slice(items, func(i, j int) bool {
		if items[i].Name == items[j].Name {
			return items[i].ID < items[j].ID
		}
		return items[i].Name < items[j].Name
	})
	return items
}

func runtimeProviderTypes() []RuntimeProviderType {
	return []RuntimeProviderType{
		{ID: string(catwalk.TypeOpenAI), Name: "OpenAI", Description: "OpenAI Responses / Chat Completions 协议"},
		{ID: string(catwalk.TypeOpenAICompat), Name: "OpenAI 兼容", Description: "兼容 /v1/chat/completions 或 /v1/responses 的服务商"},
		{ID: string(catwalk.TypeOpenRouter), Name: "OpenRouter", Description: "OpenRouter 聚合服务"},
		{ID: string(catwalk.TypeVercel), Name: "Vercel AI Gateway", Description: "Vercel AI Gateway 协议"},
		{ID: string(catwalk.TypeAnthropic), Name: "Anthropic", Description: "Anthropic Messages 协议"},
		{ID: string(catwalk.TypeGoogle), Name: "Google Gemini", Description: "Google Generative Language API"},
		{ID: string(catwalk.TypeAzure), Name: "Azure OpenAI", Description: "Azure OpenAI endpoint / deployment 协议"},
		{ID: string(catwalk.TypeBedrock), Name: "Amazon Bedrock", Description: "AWS Bedrock Runtime 协议"},
		{ID: string(catwalk.TypeVertexAI), Name: "Google Vertex AI", Description: "Vertex AI 项目与区域配置"},
		{ID: hyper.Name, Name: hyper.DisplayName, Description: "Charm Hyper 托管代理"},
	}
}

func providerRequiredFields(provider catwalk.Provider) []string {
	switch provider.ID {
	case catwalk.InferenceProviderAzure:
		return []string{"apiKey", "apiEndpoint", "apiVersion"}
	case catwalk.InferenceProviderVertexAI:
		return []string{"project", "location"}
	case catwalk.InferenceProviderBedrock, catwalk.InferenceProviderBedrockEurope:
		return []string{"awsCredentials", "region"}
	case catwalk.InferenceProvider("hyper"):
		return []string{"apiKey"}
	}

	if provider.APIEndpoint == "" && provider.Type == catwalk.TypeOpenAICompat {
		return []string{"apiKey", "apiEndpoint", "models"}
	}
	if provider.APIKey != "" {
		return []string{"apiKey"}
	}
	return nil
}

func providerNotes(provider catwalk.Provider) []string {
	switch provider.ID {
	case catwalk.InferenceProviderVertexAI:
		return []string{"需要 VERTEXAI_PROJECT 与 VERTEXAI_LOCATION。"}
	case catwalk.InferenceProviderAzure:
		return []string{"需要 Azure endpoint 与 AZURE_OPENAI_API_VERSION。"}
	case catwalk.InferenceProviderBedrock, catwalk.InferenceProviderBedrockEurope:
		return []string{"可使用 AWS 环境变量或配置文件凭据。"}
	case catwalk.InferenceProviderCopilot:
		return []string{"依赖 GitHub Copilot OAuth，当前设置页先展示能力。"}
	case catwalk.InferenceProvider("hyper"):
		return []string{"由 Charm Hyper 托管 endpoint。"}
	default:
		if strings.HasPrefix(provider.APIKey, "$") {
			return []string{"API Key 模板来自上游内置 provider，可在用户配置中保存实际密钥。"}
		}
		return nil
	}
}

func providerConfigurable(provider catwalk.Provider) bool {
	switch provider.ID {
	case catwalk.InferenceProviderSynthetic:
		return false
	default:
		return true
	}
}
