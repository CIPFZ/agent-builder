package agent

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"charm.land/fantasy"
	"charm.land/fantasy/providers/openaicompat"
	"github.com/stretchr/testify/require"
)

func TestOpenAIUsageCompatibilityCapturesUsageWithoutTotal(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		events := []string{
			"data: {\"id\":\"chatcmpl-test\",\"object\":\"chat.completion.chunk\",\"created\":1,\"model\":\"test\",\"choices\":[{\"index\":0,\"delta\":{\"role\":\"assistant\",\"content\":\"hello\"},\"finish_reason\":null}]}\n\n",
			"data: {\"id\":\"chatcmpl-test\",\"object\":\"chat.completion.chunk\",\"created\":1,\"model\":\"test\",\"choices\":[{\"index\":0,\"delta\":{},\"finish_reason\":\"stop\"}]}\n\n",
			"data: {\"id\":\"chatcmpl-test\",\"object\":\"chat.completion.chunk\",\"created\":1,\"model\":\"test\",\"choices\":[],\"usage\":{\"prompt_tokens\":120,\"completion_tokens\":7,\"prompt_tokens_details\":{\"cached_tokens\":30},\"completion_tokens_details\":{\"reasoning_tokens\":2}}}\n\n",
			"data: [DONE]\n\n",
		}
		for _, event := range events {
			_, _ = fmt.Fprint(w, event)
		}
	}))
	defer server.Close()

	base, err := openaicompat.New(
		openaicompat.WithAPIKey("test-key"),
		openaicompat.WithBaseURL(server.URL),
		openaicompat.WithHTTPClient(newOpenAIUsageHTTPClient(http.DefaultClient)),
	)
	require.NoError(t, err)
	provider := openAIUsageProvider{Provider: base}
	model, err := provider.LanguageModel(context.Background(), "test")
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
	require.Equal(t, int64(90), usage.InputTokens)
	require.Equal(t, int64(7), usage.OutputTokens)
	require.Equal(t, int64(30), usage.CacheReadTokens)
	require.Equal(t, int64(2), usage.ReasoningTokens)
	require.Equal(t, int64(127), usage.TotalTokens)
}

func TestOpenAIUsageCompatibilityCapturesResponsesUsage(t *testing.T) {
	tracker := &openAIUsageTracker{}
	body := &openAIUsageBody{ReadCloser: http.NoBody, tracker: tracker}
	body.captureLine([]byte(`data: {"type":"response.completed","response":{"usage":{"input_tokens":150,"output_tokens":20,"total_tokens":170,"input_tokens_details":{"cached_tokens":40},"output_tokens_details":{"reasoning_tokens":5}}}}`))
	usage := tracker.snapshot()
	require.Equal(t, int64(110), usage.InputTokens)
	require.Equal(t, int64(20), usage.OutputTokens)
	require.Equal(t, int64(40), usage.CacheReadTokens)
	require.Equal(t, int64(5), usage.ReasoningTokens)
	require.Equal(t, int64(170), usage.TotalTokens)
}

func TestOpenAIUsageCompatibilityInfersPromptFromTotalAndOutput(t *testing.T) {
	usage := (openAIWireUsage{TotalTokens: 100, CompletionTokens: 25}).fantasyUsage()
	require.Equal(t, int64(75), usage.InputTokens)
	require.Equal(t, int64(25), usage.OutputTokens)
	require.Equal(t, int64(100), usage.TotalTokens)
}
