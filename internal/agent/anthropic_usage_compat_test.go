package agent

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"charm.land/fantasy"
	"charm.land/fantasy/providers/anthropic"
	"github.com/stretchr/testify/require"
)

func TestAnthropicUsageCompatibilityCapturesDeltaInputAndCache(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		events := []string{
			"event: message_start\ndata: {\"type\":\"message_start\",\"message\":{\"id\":\"msg_test\",\"type\":\"message\",\"role\":\"assistant\",\"model\":\"MiniMax-M3\",\"content\":[],\"stop_reason\":null,\"usage\":{\"input_tokens\":0,\"output_tokens\":0}}}\n\n",
			"event: content_block_start\ndata: {\"type\":\"content_block_start\",\"index\":0,\"content_block\":{\"type\":\"text\",\"text\":\"\"}}\n\n",
			"event: content_block_delta\ndata: {\"type\":\"content_block_delta\",\"index\":0,\"delta\":{\"type\":\"text_delta\",\"text\":\"hello\"}}\n\n",
			"event: content_block_stop\ndata: {\"type\":\"content_block_stop\",\"index\":0}\n\n",
			"event: message_delta\ndata: {\"type\":\"message_delta\",\"delta\":{\"stop_reason\":\"end_turn\",\"stop_sequence\":null},\"usage\":{\"input_tokens\":120,\"output_tokens\":7,\"cache_read_input_tokens\":30,\"cache_creation_input_tokens\":4}}\n\n",
			"event: message_stop\ndata: {\"type\":\"message_stop\"}\n\n",
		}
		for _, event := range events {
			_, _ = fmt.Fprint(w, event)
		}
	}))
	defer server.Close()

	base, err := anthropic.New(
		anthropic.WithAPIKey("test-key"),
		anthropic.WithBaseURL(server.URL),
		anthropic.WithHTTPClient(newAnthropicUsageHTTPClient(http.DefaultClient)),
	)
	require.NoError(t, err)
	provider := anthropicUsageProvider{Provider: base}
	model, err := provider.LanguageModel(context.Background(), "MiniMax-M3")
	require.NoError(t, err)
	stream, err := model.Stream(context.Background(), fantasy.Call{Prompt: fantasy.Prompt{{
		Role:    fantasy.MessageRoleUser,
		Content: []fantasy.MessagePart{fantasy.TextPart{Text: "hello"}},
	}}})
	require.NoError(t, err)

	var usage fantasy.Usage
	stream(func(part fantasy.StreamPart) bool {
		if part.Type == fantasy.StreamPartTypeFinish {
			usage = part.Usage
		}
		return true
	})
	require.Equal(t, int64(120), usage.InputTokens)
	require.Equal(t, int64(7), usage.OutputTokens)
	require.Equal(t, int64(30), usage.CacheReadTokens)
	require.Equal(t, int64(4), usage.CacheCreationTokens)
	require.Equal(t, int64(127), usage.TotalTokens)
}
