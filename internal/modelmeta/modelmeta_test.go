package modelmeta

import (
	"testing"

	"charm.land/catwalk/pkg/catwalk"
)

func TestResolvePriorityChain(t *testing.T) {
	catalog := []catwalk.Model{{
		ID:               "catalog-model",
		ContextWindow:    64000,
		DefaultMaxTokens: 2048,
	}}

	tests := []struct {
		name   string
		req    ResolveRequest
		want   int
		source string
	}{
		{
			name: "user override wins",
			req: ResolveRequest{
				ModelID:                      "catalog-model",
				UserContextWindow:            32000,
				ProviderDefaultContextWindow: 48000,
				DiscoveredContextWindow:      96000,
				Catalog:                      catalog,
			},
			want:   32000,
			source: SourceUserOverride,
		},
		{
			name: "provider default beats discovered",
			req: ResolveRequest{
				ModelID:                      "catalog-model",
				ProviderDefaultContextWindow: 48000,
				DiscoveredContextWindow:      96000,
				Catalog:                      catalog,
			},
			want:   48000,
			source: SourceProviderDefault,
		},
		{
			name: "discovered beats builtin",
			req: ResolveRequest{
				ModelID:                 "catalog-model",
				DiscoveredContextWindow: 96000,
				Catalog:                 catalog,
			},
			want:   96000,
			source: SourceDiscovered,
		},
		{
			name: "builtin catalog beats fallback",
			req: ResolveRequest{
				ModelID: "catalog-model",
				Catalog: catalog,
			},
			want:   64000,
			source: SourceBuiltin,
		},
		{
			name: "supplemental longest substring match",
			req: ResolveRequest{
				ModelID: "provider/qwen-long-plus",
			},
			want:   1000000,
			source: SourceBuiltin,
		},
		{
			name: "fallback",
			req: ResolveRequest{
				ModelID: "unknown-model",
			},
			want:   FallbackContextWindow,
			source: SourceFallback,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := Resolve(tt.req)
			if got.ContextWindow != tt.want || got.Source != tt.source {
				t.Fatalf("Resolve() = %#v, want context=%d source=%s", got, tt.want, tt.source)
			}
		})
	}
}

func TestResolveSkipsOutOfRangeValues(t *testing.T) {
	got := Resolve(ResolveRequest{
		ModelID:                      "deepseek-chat",
		UserContextWindow:            15999,
		ProviderDefaultContextWindow: 10000001,
		DiscoveredContextWindow:      32000,
	})
	if got.ContextWindow != 32000 || got.Source != SourceDiscovered {
		t.Fatalf("Resolve() = %#v, want discovered 32000", got)
	}
}
